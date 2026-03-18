package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/llm"
)

// JournalEntry represents a single episodic memory from a session or tick.
type JournalEntry struct {
	ID         string   `json:"id"`
	SessionID  string   `json:"sessionId"`
	Summary    string   `json:"summary"`
	Decisions  []string `json:"decisions"`
	Errors     []string `json:"errors"`
	Learnings  []string `json:"learnings"`
	Entities   []string `json:"entities"`
	ToolCount  int      `json:"toolCount"`
	RoundCount int      `json:"roundCount"`
	Mode       string   `json:"mode"`
	DurationMs int64    `json:"durationMs"`
	CreatedAt  time.Time `json:"createdAt"`
}

// JournalStore persists and retrieves journal entries.
type JournalStore struct {
	db *sql.DB
}

// NewJournalStore creates a journal store.
func NewJournalStore(db *sql.DB) *JournalStore {
	return &JournalStore{db: db}
}

const journalTimeFormat = "2006-01-02T15:04:05.000Z"

// Save persists a journal entry.
func (s *JournalStore) Save(ctx context.Context, entry *JournalEntry) error {
	if entry.ID == "" {
		entry.ID = generateJournalID()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	decisions, err := json.Marshal(entry.Decisions)
	if err != nil {
		return fmt.Errorf("journal: marshal decisions: %w", err)
	}
	errors, err := json.Marshal(entry.Errors)
	if err != nil {
		return fmt.Errorf("journal: marshal errors: %w", err)
	}
	learnings, err := json.Marshal(entry.Learnings)
	if err != nil {
		return fmt.Errorf("journal: marshal learnings: %w", err)
	}
	entities, err := json.Marshal(entry.Entities)
	if err != nil {
		return fmt.Errorf("journal: marshal entities: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO session_journal (id, session_id, summary, decisions, errors, learnings, entities,
			tool_count, round_count, mode, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.SessionID, entry.Summary,
		string(decisions), string(errors), string(learnings), string(entities),
		entry.ToolCount, entry.RoundCount, entry.Mode,
		entry.DurationMs, entry.CreatedAt.Format(journalTimeFormat))
	return err
}

// Recent returns journal entries from the last duration, ordered newest first.
func (s *JournalStore) Recent(ctx context.Context, dur time.Duration) ([]*JournalEntry, error) {
	cutoff := time.Now().UTC().Add(-dur).Format(journalTimeFormat)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, summary, decisions, errors, learnings, entities,
			tool_count, round_count, mode, duration_ms, created_at
		FROM session_journal WHERE created_at > ? ORDER BY created_at DESC`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*JournalEntry
	for rows.Next() {
		e, err := scanJournalEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

type journalScanner interface {
	Scan(dest ...any) error
}

func scanJournalEntry(row journalScanner) (*JournalEntry, error) {
	var e JournalEntry
	var decisions, errors, learnings, entities, createdStr string

	if err := row.Scan(&e.ID, &e.SessionID, &e.Summary,
		&decisions, &errors, &learnings, &entities,
		&e.ToolCount, &e.RoundCount, &e.Mode, &e.DurationMs, &createdStr); err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(decisions), &e.Decisions); err != nil {
		slog.Warn("journal: parse decisions", "id", e.ID, "error", err)
	}
	if err := json.Unmarshal([]byte(errors), &e.Errors); err != nil {
		slog.Warn("journal: parse errors", "id", e.ID, "error", err)
	}
	if err := json.Unmarshal([]byte(learnings), &e.Learnings); err != nil {
		slog.Warn("journal: parse learnings", "id", e.ID, "error", err)
	}
	if err := json.Unmarshal([]byte(entities), &e.Entities); err != nil {
		slog.Warn("journal: parse entities", "id", e.ID, "error", err)
	}

	if t, err := time.Parse(journalTimeFormat, createdStr); err == nil {
		e.CreatedAt = t
	}
	return &e, nil
}

func generateJournalID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("j_%x", b)
}

// Journaler generates episodic memory entries from sessions using an LLM.
type Journaler struct {
	store    *JournalStore
	provider llm.Provider
	model    string
}

// NewJournaler creates a session journaler.
func NewJournaler(store *JournalStore, provider llm.Provider, model string) *Journaler {
	return &Journaler{store: store, provider: provider, model: model}
}

// Record creates a journal entry for a completed session. The LLM summarizes
// the session into structured episodic memory. Fire-and-forget - errors are logged.
func (j *Journaler) Record(ctx context.Context, session *Session, duration time.Duration) {
	if j.provider == nil || len(session.Events) == 0 {
		return
	}

	// Build a compact session transcript for the LLM.
	transcript := buildTranscript(session)
	if transcript == "" {
		return
	}

	prompt := fmt.Sprintf(`Analyze this agent session and produce a JSON object with these fields:
- summary: 1-2 sentence summary of what happened
- decisions: array of key decisions made
- errors: array of errors encountered
- learnings: array of things learned or patterns noticed
- entities: array of entities mentioned (repos, people, tools, concepts)

Session (mode: %s, events: %d):
%s

Respond with ONLY valid JSON, no markdown fences.`, session.Mode, len(session.Events), transcript)

	result, err := j.callLLM(ctx, prompt)
	if err != nil {
		slog.Warn("journal: LLM summarization failed", "session", session.ID, "error", err)
		return
	}

	entry := parseJournalResult(result, session, duration)
	if err := j.store.Save(ctx, entry); err != nil {
		slog.Warn("journal: save failed", "session", session.ID, "error", err)
	}
}

func buildTranscript(session *Session) string {
	var b strings.Builder

	for _, ev := range session.Events {
		for _, part := range ev.Parts {
			switch p := part.(type) {
			case TextPart:
				text := p.Text
				if runes := []rune(text); len(runes) > 500 {
					text = string(runes[:500]) + "..."
				}
				fmt.Fprintf(&b, "[%s] %s\n", ev.Author, text)
			case ToolPart:
				if p.Status == ToolCompleted || p.Status == ToolFailed {
					fmt.Fprintf(&b, "[tool:%s] status=%s\n", p.ToolName, p.Status)
				}
			}
		}
	}
	return b.String()
}

func parseJournalResult(result string, session *Session, duration time.Duration) *JournalEntry {
	entry := &JournalEntry{
		SessionID:  session.ID,
		Mode:       string(session.Mode),
		DurationMs: duration.Milliseconds(),
	}

	// Try to parse as JSON.
	var parsed struct {
		Summary   string   `json:"summary"`
		Decisions []string `json:"decisions"`
		Errors    []string `json:"errors"`
		Learnings []string `json:"learnings"`
		Entities  []string `json:"entities"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err == nil {
		entry.Summary = parsed.Summary
		entry.Decisions = parsed.Decisions
		entry.Errors = parsed.Errors
		entry.Learnings = parsed.Learnings
		entry.Entities = parsed.Entities
	} else {
		// Fallback: use raw text as summary.
		entry.Summary = result
	}

	// Count tools and rounds from session.
	for _, ev := range session.Events {
		for _, part := range ev.Parts {
			if tp, ok := part.(ToolPart); ok && (tp.Status == ToolCompleted || tp.Status == ToolFailed) {
				entry.ToolCount++
			}
		}
		if ev.Round > entry.RoundCount {
			entry.RoundCount = ev.Round
		}
	}

	return entry
}

func (j *Journaler) callLLM(ctx context.Context, prompt string) (string, error) {
	req := &llm.Request{
		Model: j.model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: prompt}}},
		},
		MaxTokens: 512,
	}

	ch, err := j.provider.Stream(ctx, req)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	for ev := range ch {
		if td, ok := ev.(llm.TextDelta); ok {
			result.WriteString(td.Text)
		}
	}
	return result.String(), nil
}
