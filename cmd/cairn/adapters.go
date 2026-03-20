package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	cairnchannel "github.com/avifenesh/cairn/internal/channel"
	"github.com/avifenesh/cairn/internal/config"
	cairncron "github.com/avifenesh/cairn/internal/cron"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/signal"
	"github.com/avifenesh/cairn/internal/skill"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
)

// memoryAdapter bridges memory.Service to tool.MemoryService.
type memoryAdapter struct {
	svc *memory.Service
}

func (a *memoryAdapter) Create(ctx context.Context, m *tool.MemoryItem) error {
	mem := &memory.Memory{
		Content:  m.Content,
		Category: memory.Category(m.Category),
		Scope:    memory.Scope(m.Scope),
		Source:   m.Source,
	}
	if err := a.svc.Create(ctx, mem); err != nil {
		return err
	}
	m.ID = mem.ID // propagate generated ID back
	return nil
}

func (a *memoryAdapter) Search(ctx context.Context, query string, limit int) ([]tool.MemorySearchResult, error) {
	results, err := a.svc.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]tool.MemorySearchResult, len(results))
	for i, r := range results {
		out[i] = tool.MemorySearchResult{
			Memory: &tool.MemoryItem{
				ID:         r.Memory.ID,
				Content:    r.Memory.Content,
				Category:   string(r.Memory.Category),
				Scope:      string(r.Memory.Scope),
				Status:     string(r.Memory.Status),
				Confidence: r.Memory.Confidence,
				Source:     r.Memory.Source,
			},
			Score: r.Score,
		}
	}
	return out, nil
}

func (a *memoryAdapter) Get(ctx context.Context, id string) (*tool.MemoryItem, error) {
	m, err := a.svc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &tool.MemoryItem{
		ID:         m.ID,
		Content:    m.Content,
		Category:   string(m.Category),
		Scope:      string(m.Scope),
		Status:     string(m.Status),
		Confidence: m.Confidence,
		Source:     m.Source,
	}, nil
}

func (a *memoryAdapter) Accept(ctx context.Context, id string) error {
	return a.svc.Accept(ctx, id)
}

func (a *memoryAdapter) Reject(ctx context.Context, id string) error {
	return a.svc.Reject(ctx, id)
}

func (a *memoryAdapter) Delete(ctx context.Context, id string) error {
	return a.svc.Delete(ctx, id)
}

// eventAdapter bridges signal.EventStore to tool.EventService.
type eventAdapter struct {
	store *signal.EventStore
}

func (a *eventAdapter) List(ctx context.Context, f tool.EventFilter) ([]*tool.StoredEvent, error) {
	events, err := a.store.List(ctx, signal.EventFilter{
		Source:          f.Source,
		Kind:            f.Kind,
		UnreadOnly:      f.UnreadOnly,
		ExcludeArchived: f.ExcludeArchived,
		Limit:           f.Limit,
		Before:          f.Before,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*tool.StoredEvent, len(events))
	for i, ev := range events {
		out[i] = &tool.StoredEvent{
			ID:         ev.ID,
			Source:     ev.Source,
			Kind:       ev.Kind,
			Title:      ev.Title,
			Body:       ev.Body,
			URL:        ev.URL,
			Actor:      ev.Actor,
			GroupKey:   ev.GroupKey,
			Metadata:   ev.Metadata,
			CreatedAt:  ev.CreatedAt,
			ReadAt:     ev.ReadAt,
			ArchivedAt: ev.ArchivedAt,
		}
	}
	return out, nil
}

func (a *eventAdapter) Count(ctx context.Context, f tool.EventFilter) (int, error) {
	return a.store.Count(ctx, signal.EventFilter{
		Source:     f.Source,
		Kind:       f.Kind,
		UnreadOnly: f.UnreadOnly,
	})
}

func (a *eventAdapter) CountBySource(ctx context.Context) (map[string]int, error) {
	return a.store.CountBySource(ctx)
}

func (a *eventAdapter) CountArchivedBySource(ctx context.Context) (map[string]int, error) {
	return a.store.CountArchivedBySource(ctx)
}

func (a *eventAdapter) Archive(ctx context.Context, id string) error {
	return a.store.Archive(ctx, id)
}

func (a *eventAdapter) DeleteByID(ctx context.Context, id string) error {
	return a.store.DeleteByID(ctx, id)
}

func (a *eventAdapter) MarkRead(ctx context.Context, id string) error {
	return a.store.MarkRead(ctx, id)
}

func (a *eventAdapter) MarkAllRead(ctx context.Context) (int, error) {
	return a.store.MarkAllRead(ctx)
}

func (a *eventAdapter) Ingest(ctx context.Context, events []*tool.IngestEvent) ([]*tool.IngestEvent, error) {
	raw := make([]*signal.RawEvent, len(events))
	for i, ev := range events {
		raw[i] = &signal.RawEvent{
			Source:     ev.Source,
			SourceID:   ev.SourceID,
			Kind:       ev.Kind,
			Title:      ev.Title,
			Body:       ev.Body,
			Actor:      ev.Actor,
			OccurredAt: ev.OccurredAt,
			Metadata:   ev.Metadata,
		}
	}
	inserted, err := a.store.Ingest(ctx, raw)
	if err != nil {
		return nil, err
	}
	out := make([]*tool.IngestEvent, len(inserted))
	for i, ev := range inserted {
		out[i] = &tool.IngestEvent{
			Source:     ev.Source,
			SourceID:   ev.SourceID,
			Kind:       ev.Kind,
			Title:      ev.Title,
			Body:       ev.Body,
			Actor:      ev.Actor,
			OccurredAt: ev.OccurredAt,
			Metadata:   ev.Metadata,
		}
	}
	return out, nil
}

// digestAdapter bridges signal.DigestRunner to tool.DigestService.
type digestAdapter struct {
	runner *signal.DigestRunner
}

func (a *digestAdapter) Generate(ctx context.Context) (*tool.DigestResult, error) {
	d, err := a.runner.Generate(ctx)
	if err != nil {
		return nil, err
	}
	return &tool.DigestResult{
		Summary:    d.Summary,
		Highlights: d.Highlights,
		EventCount: d.EventCount,
	}, nil
}

// journalAdapter bridges agent.JournalStore to tool.JournalService.
type journalAdapter struct {
	store *agent.JournalStore
}

func (a *journalAdapter) Recent(ctx context.Context, dur time.Duration) ([]*tool.JournalEntry, error) {
	entries, err := a.store.Recent(ctx, dur)
	if err != nil {
		return nil, err
	}
	out := make([]*tool.JournalEntry, len(entries))
	for i, e := range entries {
		out[i] = &tool.JournalEntry{
			ID:        e.ID,
			Summary:   e.Summary,
			Decisions: e.Decisions,
			Errors:    e.Errors,
			Learnings: e.Learnings,
			Mode:      e.Mode,
			CreatedAt: e.CreatedAt,
		}
	}
	return out, nil
}

// taskAdapter bridges task.Engine to tool.TaskService.
type taskAdapter struct {
	engine *task.Engine
}

func (a *taskAdapter) Submit(ctx context.Context, req *tool.TaskSubmitRequest) (*tool.TaskItem, error) {
	t, err := a.engine.Submit(ctx, &task.SubmitRequest{
		Type:        task.TaskType(req.Type),
		Priority:    task.Priority(req.Priority),
		Description: req.Description,
	})
	if err != nil {
		return nil, err
	}
	return &tool.TaskItem{
		ID:          t.ID,
		Type:        string(t.Type),
		Status:      string(t.Status),
		Description: t.Description,
		Priority:    int(t.Priority),
		CreatedAt:   t.CreatedAt,
	}, nil
}

func (a *taskAdapter) List(ctx context.Context, status, taskType string, limit int) ([]*tool.TaskItem, error) {
	opts := task.ListOpts{Limit: limit}
	if status != "" {
		opts.Status = task.TaskStatus(status)
	}
	if taskType != "" {
		opts.Type = task.TaskType(taskType)
	}
	tasks, err := a.engine.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	out := make([]*tool.TaskItem, len(tasks))
	for i, t := range tasks {
		out[i] = &tool.TaskItem{
			ID:          t.ID,
			Type:        string(t.Type),
			Status:      string(t.Status),
			Description: t.Description,
			Priority:    int(t.Priority),
			Error:       t.Error,
			CreatedAt:   t.CreatedAt,
		}
	}
	return out, nil
}

func (a *taskAdapter) Complete(ctx context.Context, id string, output string) error {
	var raw json.RawMessage
	if output != "" {
		var err error
		raw, err = json.Marshal(output)
		if err != nil {
			return fmt.Errorf("task adapter: marshal output: %w", err)
		}
	}
	return a.engine.Complete(ctx, id, raw)
}

// statusAdapter aggregates system status from multiple services.
type statusAdapter struct {
	tasks       *task.Engine
	events      *signal.EventStore
	memories    *memory.Service
	startedAt   time.Time
	pollerNames []string // populated at startup
}

func (a *statusAdapter) GetStatus(ctx context.Context) (*tool.SystemStatus, error) {
	var errs []string

	// Count active tasks by listing each active status.
	activeTasks := 0
	for _, s := range []task.TaskStatus{task.StatusQueued, task.StatusClaimed, task.StatusRunning} {
		tasks, err := a.tasks.List(ctx, task.ListOpts{Status: s})
		if err != nil {
			errs = append(errs, fmt.Sprintf("tasks(%s): %v", s, err))
		} else {
			activeTasks += len(tasks)
		}
	}

	unreadEvents, err := a.events.Count(ctx, signal.EventFilter{UnreadOnly: true})
	if err != nil {
		errs = append(errs, fmt.Sprintf("events: %v", err))
	}

	memoryCount := 0
	mems, err := a.memories.List(ctx, memory.ListOpts{Status: memory.StatusAccepted})
	if err != nil {
		errs = append(errs, fmt.Sprintf("memories: %v", err))
	} else {
		memoryCount = len(mems)
	}

	// Build poller info from registered names.
	pollers := make([]tool.PollerInfo, len(a.pollerNames))
	for i, name := range a.pollerNames {
		pollers[i] = tool.PollerInfo{Source: name, Active: true}
	}

	status := &tool.SystemStatus{
		Uptime:       time.Since(a.startedAt).Truncate(time.Second).String(),
		ActiveTasks:  activeTasks,
		UnreadEvents: unreadEvents,
		MemoryCount:  memoryCount,
		PollerStatus: pollers,
	}

	if len(errs) > 0 {
		return status, fmt.Errorf("partial status: %s", strings.Join(errs, "; "))
	}
	return status, nil
}

// skillAdapter bridges skill.Service to tool.SkillService.
type skillAdapter struct {
	svc *skill.Service
}

func (a *skillAdapter) Get(name string) *tool.SkillItem {
	sk := a.svc.Get(name)
	if sk == nil {
		return nil
	}
	return skillToItem(sk)
}

func (a *skillAdapter) List() []*tool.SkillItem {
	skills := a.svc.List()
	out := make([]*tool.SkillItem, len(skills))
	for i, sk := range skills {
		out[i] = skillToItem(sk)
	}
	return out
}

func (a *skillAdapter) Create(name, description, content, inclusion string, allowedTools []string) error {
	return a.svc.Create(name, description, content, inclusion, allowedTools)
}

func (a *skillAdapter) Update(name, description, content, inclusion string, allowedTools []string) error {
	return a.svc.Update(name, description, content, inclusion, allowedTools)
}

func (a *skillAdapter) Delete(name string) error {
	return a.svc.Delete(name)
}

func (a *skillAdapter) InstallDir() string {
	return a.svc.InstallDir()
}

func (a *skillAdapter) Refresh() error {
	return a.svc.Discover()
}

func skillToItem(sk *skill.Skill) *tool.SkillItem {
	return &tool.SkillItem{
		Name:         sk.Name,
		Description:  sk.Description,
		Inclusion:    string(sk.Inclusion),
		Content:      sk.Content,
		AllowedTools: sk.AllowedTools,
		Location:     filepath.Dir(sk.Location),
		DisableModel: sk.DisableModel,
	}
}

// notifierAdapter bridges channel.Router to tool.NotifyService.
type notifierAdapter struct {
	router *cairnchannel.Router
}

func (a *notifierAdapter) Notify(ctx context.Context, text string, priority int) {
	a.router.Notify(ctx, &cairnchannel.OutgoingMessage{
		Text:     text,
		Priority: cairnchannel.Priority(priority),
	})
}

func (a *notifierAdapter) SendToChannel(ctx context.Context, channelName, text string, priority int) error {
	return a.router.SendTo(ctx, channelName, &cairnchannel.OutgoingMessage{
		Text:     text,
		Priority: cairnchannel.Priority(priority),
	})
}

// NotifyApproval sends an approval request to channels with approve/deny buttons.
func (a *notifierAdapter) NotifyApproval(ctx context.Context, approvalID, approvalType, description string) {
	text := fmt.Sprintf("**Approval required** [%s]\n\n%s\n\nID: `%s`", approvalType, description, approvalID)
	a.router.Notify(ctx, &cairnchannel.OutgoingMessage{
		Text:     text,
		Priority: cairnchannel.PriorityHigh,
		Actions: []cairnchannel.ActionGroup{
			{
				Label: "Decision",
				Actions: []cairnchannel.Action{
					{ID: "approve:" + approvalID, Label: "Approve", Style: "primary"},
					{ID: "deny:" + approvalID, Label: "Deny", Style: "danger"},
				},
			},
		},
	})
}
func (a *notifierAdapter) FlushDigest(ctx context.Context) int {
	return a.router.FlushDigest(ctx)
}

func (a *notifierAdapter) DigestLen() int {
	return a.router.DigestLen()
}

// cronAdapter bridges cron.Store to tool.CronService.
type cronAdapter struct {
	store *cairncron.Store
}

func (a *cronAdapter) Create(ctx context.Context, name, schedule, instruction string, priority int) (string, error) {
	job := &cairncron.CronJob{
		Enabled:     true,
		Name:        name,
		Schedule:    schedule,
		Instruction: instruction,
		Timezone:    "UTC",
		Priority:    priority,
		CooldownMs:  3600000,
	}
	if err := a.store.Create(ctx, job); err != nil {
		return "", err
	}
	return job.ID, nil
}

func (a *cronAdapter) List(ctx context.Context) ([]tool.CronJobInfo, error) {
	jobs, err := a.store.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]tool.CronJobInfo, len(jobs))
	for i, j := range jobs {
		result[i] = tool.CronJobInfo{
			ID:          j.ID,
			Name:        j.Name,
			Schedule:    j.Schedule,
			Instruction: j.Instruction,
			Timezone:    j.Timezone,
			Enabled:     j.Enabled,
			Priority:    j.Priority,
			LastRun:     j.LastRunAt,
			NextRun:     j.NextRunAt,
		}
	}
	return result, nil
}

func (a *cronAdapter) Delete(ctx context.Context, idOrName string) error {
	// Try by ID first.
	if err := a.store.Delete(ctx, idOrName); err == nil {
		return nil
	}
	// Fall back to name lookup.
	job, err := a.store.GetByName(ctx, idOrName)
	if err != nil {
		return fmt.Errorf("cron job %q not found", idOrName)
	}
	return a.store.Delete(ctx, job.ID)
}

// configAdapter bridges config.Config to tool.ConfigService.
type configAdapter struct {
	cfg *config.Config
}

func (a *configAdapter) PatchConfig(ctx context.Context, changes map[string]any) (map[string]any, error) {
	// Convert map to PatchableConfig JSON and back.
	data, err := json.Marshal(changes)
	if err != nil {
		return nil, err
	}
	var patch config.PatchableConfig
	if err := json.Unmarshal(data, &patch); err != nil {
		return nil, err
	}
	a.cfg.ApplyPatch(patch)
	if err := a.cfg.SaveOverrides(a.cfg.DataDir); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}
	return a.GetConfig(ctx)
}

func (a *configAdapter) GetConfig(_ context.Context) (map[string]any, error) {
	p := a.cfg.GetPatchable()
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return result, nil
}

// --- Channel command handlers ---

// handleMemoriesCommand handles /memories subcommands from channels.
//
//	/memories              — list accepted memories (last 20)
//	/memories proposed     — list proposed memories
//	/memories search <q>   — search memories
//	/memories accept <id>  — accept a proposed memory
//	/memories reject <id>  — reject a proposed memory
//	/memories delete <id>  — delete a memory
//	/memories compact      — run compaction (decay old, reject stale)
//	/memories dedup        — find and remove duplicate memories
func handleMemoriesCommand(ctx context.Context, args string, svc *memory.Service) (*cairnchannel.OutgoingMessage, error) {
	parts := strings.Fields(args)
	sub := ""
	if len(parts) > 0 {
		sub = strings.ToLower(parts[0])
	}

	switch sub {
	case "", "list":
		status := memory.StatusAccepted
		if len(parts) > 1 && strings.ToLower(parts[1]) == "proposed" {
			status = memory.StatusProposed
		}
		mems, err := svc.List(ctx, memory.ListOpts{Status: status, Limit: 20})
		if err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
		}
		if len(mems) == 0 {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("No %s memories.", status)}, nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "**%s memories** (%d):\n\n", status, len(mems))
		for _, m := range mems {
			snippet := m.Content
			if len(snippet) > 80 {
				snippet = snippet[:77] + "..."
			}
			fmt.Fprintf(&b, "`%s` [%s] %s\n", m.ID[:8], m.Category, snippet)
		}
		return &cairnchannel.OutgoingMessage{Text: b.String()}, nil

	case "proposed":
		mems, err := svc.List(ctx, memory.ListOpts{Status: memory.StatusProposed, Limit: 20})
		if err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
		}
		if len(mems) == 0 {
			return &cairnchannel.OutgoingMessage{Text: "No proposed memories."}, nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "**Proposed memories** (%d):\n\n", len(mems))
		for _, m := range mems {
			snippet := m.Content
			if len(snippet) > 80 {
				snippet = snippet[:77] + "..."
			}
			fmt.Fprintf(&b, "`%s` [%s] %.0f%% — %s\n", m.ID[:8], m.Category, m.Confidence*100, snippet)
		}
		b.WriteString("\nUse `/memories accept <id>` or `/memories reject <id>`")
		return &cairnchannel.OutgoingMessage{Text: b.String()}, nil

	case "search":
		if len(parts) < 2 {
			return &cairnchannel.OutgoingMessage{Text: "Usage: `/memories search <query>`"}, nil
		}
		query := strings.Join(parts[1:], " ")
		results, err := svc.Search(ctx, query, 10)
		if err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
		}
		if len(results) == 0 {
			return &cairnchannel.OutgoingMessage{Text: "No matching memories."}, nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "**Search: %s** (%d results):\n\n", query, len(results))
		for _, r := range results {
			snippet := r.Memory.Content
			if len(snippet) > 80 {
				snippet = snippet[:77] + "..."
			}
			fmt.Fprintf(&b, "`%s` [%.0f%%] %s\n", r.Memory.ID[:8], r.Score*100, snippet)
		}
		return &cairnchannel.OutgoingMessage{Text: b.String()}, nil

	case "accept":
		if len(parts) < 2 {
			return &cairnchannel.OutgoingMessage{Text: "Usage: `/memories accept <id>`"}, nil
		}
		id, err := resolveMemoryID(ctx, svc, parts[1])
		if err != nil {
			return &cairnchannel.OutgoingMessage{Text: err.Error()}, nil
		}
		if err := svc.Accept(ctx, id); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Memory `%s` accepted.", parts[1])}, nil

	case "reject":
		if len(parts) < 2 {
			return &cairnchannel.OutgoingMessage{Text: "Usage: `/memories reject <id>`"}, nil
		}
		id, err := resolveMemoryID(ctx, svc, parts[1])
		if err != nil {
			return &cairnchannel.OutgoingMessage{Text: err.Error()}, nil
		}
		if err := svc.Reject(ctx, id); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Memory `%s` rejected.", parts[1])}, nil

	case "delete":
		if len(parts) < 2 {
			return &cairnchannel.OutgoingMessage{Text: "Usage: `/memories delete <id>`"}, nil
		}
		id, err := resolveMemoryID(ctx, svc, parts[1])
		if err != nil {
			return &cairnchannel.OutgoingMessage{Text: err.Error()}, nil
		}
		if err := svc.Delete(ctx, id); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Memory `%s` deleted.", parts[1])}, nil

	case "compact":
		if err := svc.Compact(ctx); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Compaction error: %s", err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: "Memory compaction complete."}, nil

	case "dedup":
		count, err := deduplicateMemories(ctx, svc)
		if err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Dedup error: %s", err)}, nil
		}
		if count == 0 {
			return &cairnchannel.OutgoingMessage{Text: "No duplicates found."}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Removed %d duplicate memories.", count)}, nil

	default:
		return &cairnchannel.OutgoingMessage{Text: "Usage:\n`/memories` — list accepted\n`/memories proposed` — list proposed\n`/memories search <query>`\n`/memories accept <id>`\n`/memories reject <id>`\n`/memories delete <id>`\n`/memories compact`\n`/memories dedup`"}, nil
	}
}

// resolveMemoryID resolves a short prefix to a full memory ID.
func resolveMemoryID(ctx context.Context, svc *memory.Service, prefix string) (string, error) {
	// Try exact match first.
	if m, err := svc.Get(ctx, prefix); err == nil && m != nil {
		return m.ID, nil
	}
	// Search by prefix across all statuses.
	for _, status := range []memory.Status{memory.StatusAccepted, memory.StatusProposed, memory.StatusRejected} {
		mems, err := svc.List(ctx, memory.ListOpts{Status: status, Limit: 100})
		if err != nil {
			continue
		}
		var matches []string
		for _, m := range mems {
			if strings.HasPrefix(m.ID, prefix) {
				matches = append(matches, m.ID)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return "", fmt.Errorf("ambiguous prefix `%s` — matches %d memories", prefix, len(matches))
		}
	}
	return "", fmt.Errorf("no memory found with ID or prefix `%s`", prefix)
}

// deduplicateMemories finds accepted memories with identical content and removes duplicates.
func deduplicateMemories(ctx context.Context, svc *memory.Service) (int, error) {
	mems, err := svc.List(ctx, memory.ListOpts{Status: memory.StatusAccepted, Limit: 500})
	if err != nil {
		return 0, err
	}

	seen := make(map[string]string) // content hash → first ID
	removed := 0
	for _, m := range mems {
		key := strings.TrimSpace(strings.ToLower(m.Content))
		if firstID, exists := seen[key]; exists {
			// Duplicate — delete the newer one, keep the older.
			_ = firstID
			if err := svc.Delete(ctx, m.ID); err == nil {
				removed++
			}
		} else {
			seen[key] = m.ID
		}
	}
	return removed, nil
}

// handlePatchCommand handles /patch subcommands for SOUL.md patches.
//
//	/patch           — show pending patch
//	/patch approve   — approve pending patch
//	/patch deny      — deny pending patch
func handlePatchCommand(_ context.Context, args string, soul *memory.Soul) (*cairnchannel.OutgoingMessage, error) {
	sub := strings.TrimSpace(strings.ToLower(args))

	switch sub {
	case "", "show":
		p := soul.PendingPatch()
		if p == nil {
			return &cairnchannel.OutgoingMessage{Text: "No pending SOUL patch."}, nil
		}
		preview := p.Content
		if len(preview) > 500 {
			preview = preview[:497] + "..."
		}
		text := fmt.Sprintf("**Pending SOUL patch** (`%s`)\nSource: %s\nCreated: %s\n\n```\n%s\n```\n\nUse `/patch approve` or `/patch deny`",
			p.ID[:8], p.Source, p.CreatedAt.Format("2006-01-02 15:04"), preview)
		return &cairnchannel.OutgoingMessage{Text: text}, nil

	case "approve":
		p := soul.PendingPatch()
		if p == nil {
			return &cairnchannel.OutgoingMessage{Text: "No pending SOUL patch to approve."}, nil
		}
		if err := soul.ApprovePatch(p.ID); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("SOUL patch `%s` approved and applied.", p.ID[:8])}, nil

	case "deny", "reject":
		p := soul.PendingPatch()
		if p == nil {
			return &cairnchannel.OutgoingMessage{Text: "No pending SOUL patch to deny."}, nil
		}
		if err := soul.DenyPatch(p.ID); err != nil {
			return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Error: %s", err)}, nil
		}
		return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("SOUL patch `%s` denied.", p.ID[:8])}, nil

	default:
		return &cairnchannel.OutgoingMessage{Text: "Usage:\n`/patch` — show pending\n`/patch approve` — approve\n`/patch deny` — deny"}, nil
	}
}
