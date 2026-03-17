package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/avifenesh/cairn/internal/db"
)

// Store provides CRUD operations on the memories SQLite table.
type Store struct {
	db *db.DB
}

// NewStore creates a Store backed by the given database.
func NewStore(d *db.DB) *Store {
	return &Store{db: d}
}

// generateID returns a 16-character hex string from crypto/rand.
func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("memory: generate id: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}

// timeFormat is the ISO-8601 format used for SQLite TEXT timestamps.
const timeFormat = "2006-01-02T15:04:05.000Z"

// Create persists a new memory. If m.ID is empty, one is generated.
func (s *Store) Create(ctx context.Context, m *Memory) error {
	if m.ID == "" {
		id, err := generateID()
		if err != nil {
			return err
		}
		m.ID = id
	}

	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now

	if m.Confidence == 0 {
		m.Confidence = 0.5
	}
	if m.Source == "" {
		m.Source = "agent"
	}
	if m.Status == "" {
		m.Status = StatusProposed
	}
	if m.Scope == "" {
		m.Scope = ScopeGlobal
	}
	if m.Category == "" {
		m.Category = CatFact
	}

	metaJSON, err := json.Marshal(m.Metadata)
	if err != nil {
		return fmt.Errorf("memory: marshal metadata: %w", err)
	}

	var embeddingBlob []byte
	if len(m.Embedding) > 0 {
		embeddingBlob = encodeFloat32s(m.Embedding)
	}

	var lastUsed *string
	if m.LastUsedAt != nil {
		s := m.LastUsedAt.UTC().Format(timeFormat)
		lastUsed = &s
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO memories (id, content, category, scope, status, confidence, source,
			created_at, updated_at, embedding, access_count, last_accessed_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Content, string(m.Category), string(m.Scope), string(m.Status),
		m.Confidence, m.Source,
		m.CreatedAt.UTC().Format(timeFormat), m.UpdatedAt.UTC().Format(timeFormat),
		embeddingBlob, m.UseCount, lastUsed, string(metaJSON),
	)
	if err != nil {
		return fmt.Errorf("memory: create %q: %w", m.ID, err)
	}

	slog.Debug("memory created", "id", m.ID, "category", m.Category, "status", m.Status)
	return nil
}

// Get retrieves a single memory by ID.
func (s *Store) Get(ctx context.Context, id string) (*Memory, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, content, category, scope, status, confidence, source,
			created_at, updated_at, embedding, access_count, last_accessed_at, metadata
		FROM memories WHERE id = ?`, id)

	m, err := scanMemory(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("memory: not found: %s", id)
		}
		return nil, fmt.Errorf("memory: get %q: %w", id, err)
	}
	return m, nil
}

// List returns memories matching the given filters.
func (s *Store) List(ctx context.Context, opts ListOpts) ([]*Memory, error) {
	query := "SELECT id, content, category, scope, status, confidence, source, created_at, updated_at, embedding, access_count, last_accessed_at, metadata FROM memories WHERE 1=1"
	var args []any

	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, string(opts.Status))
	}
	if opts.Category != "" {
		query += " AND category = ?"
		args = append(args, string(opts.Category))
	}
	if opts.Scope != "" {
		query += " AND scope = ?"
		args = append(args, string(opts.Scope))
	}

	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: list: %w", err)
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		m, err := scanMemoryRows(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: list scan: %w", err)
		}
		memories = append(memories, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: list rows: %w", err)
	}
	return memories, nil
}

// Update replaces a memory's content and metadata fields.
func (s *Store) Update(ctx context.Context, m *Memory) error {
	m.UpdatedAt = time.Now().UTC()

	metaJSON, err := json.Marshal(m.Metadata)
	if err != nil {
		return fmt.Errorf("memory: marshal metadata: %w", err)
	}

	var embeddingBlob []byte
	if len(m.Embedding) > 0 {
		embeddingBlob = encodeFloat32s(m.Embedding)
	}

	var lastUsed *string
	if m.LastUsedAt != nil {
		s := m.LastUsedAt.UTC().Format(timeFormat)
		lastUsed = &s
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE memories SET content=?, category=?, scope=?, status=?, confidence=?,
			source=?, updated_at=?, embedding=?, access_count=?, last_accessed_at=?, metadata=?
		WHERE id=?`,
		m.Content, string(m.Category), string(m.Scope), string(m.Status),
		m.Confidence, m.Source, m.UpdatedAt.UTC().Format(timeFormat),
		embeddingBlob, m.UseCount, lastUsed, string(metaJSON), m.ID,
	)
	if err != nil {
		return fmt.Errorf("memory: update %q: %w", m.ID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory: not found: %s", m.ID)
	}
	return nil
}

// Delete removes a memory by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM memories WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("memory: delete %q: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory: not found: %s", id)
	}
	slog.Debug("memory deleted", "id", id)
	return nil
}

// UpdateStatus changes a memory's status (proposed → accepted/rejected).
func (s *Store) UpdateStatus(ctx context.Context, id string, status Status) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := s.db.ExecContext(ctx,
		"UPDATE memories SET status = ?, updated_at = ? WHERE id = ?",
		string(status), now, id,
	)
	if err != nil {
		return fmt.Errorf("memory: update status %q: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory: not found: %s", id)
	}
	slog.Debug("memory status updated", "id", id, "status", status)
	return nil
}

// IncrementUseCount bumps the access counter and last-accessed timestamp.
func (s *Store) IncrementUseCount(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := s.db.ExecContext(ctx,
		"UPDATE memories SET access_count = access_count + 1, last_accessed_at = ?, updated_at = ? WHERE id = ?",
		now, now, id,
	)
	if err != nil {
		return fmt.Errorf("memory: increment use count %q: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory: not found: %s", id)
	}
	return nil
}

// SearchByKeyword finds accepted memories whose content or metadata contains query.
func (s *Store) SearchByKeyword(ctx context.Context, query string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	pattern := "%" + query + "%"

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, content, category, scope, status, confidence, source,
			created_at, updated_at, embedding, access_count, last_accessed_at, metadata
		FROM memories
		WHERE status = 'accepted' AND (content LIKE ? OR metadata LIKE ?)
		ORDER BY access_count DESC
		LIMIT ?`, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: keyword search: %w", err)
	}
	defer rows.Close()

	var results []*Memory
	for rows.Next() {
		m, err := scanMemoryRows(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: keyword search scan: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: keyword search rows: %w", err)
	}
	return results, nil
}

// AllAcceptedWithEmbeddings returns all accepted memories that have a non-nil embedding.
func (s *Store) AllAcceptedWithEmbeddings(ctx context.Context) ([]*Memory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, content, category, scope, status, confidence, source,
			created_at, updated_at, embedding, access_count, last_accessed_at, metadata
		FROM memories
		WHERE status = 'accepted' AND embedding IS NOT NULL
		ORDER BY access_count DESC`)
	if err != nil {
		return nil, fmt.Errorf("memory: all accepted with embeddings: %w", err)
	}
	defer rows.Close()

	var results []*Memory
	for rows.Next() {
		m, err := scanMemoryRows(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: all accepted embeddings scan: %w", err)
		}
		if len(m.Embedding) > 0 {
			results = append(results, m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: all accepted embeddings rows: %w", err)
	}
	return results, nil
}

// OldUnusedMemories returns accepted memories with access_count=0 older than age.
func (s *Store) OldUnusedMemories(ctx context.Context, age time.Duration) ([]*Memory, error) {
	cutoff := time.Now().UTC().Add(-age).Format(timeFormat)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, content, category, scope, status, confidence, source,
			created_at, updated_at, embedding, access_count, last_accessed_at, metadata
		FROM memories
		WHERE status = 'accepted' AND access_count = 0 AND created_at < ?`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("memory: old unused: %w", err)
	}
	defer rows.Close()

	var results []*Memory
	for rows.Next() {
		m, err := scanMemoryRows(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: old unused scan: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: old unused rows: %w", err)
	}
	return results, nil
}

// UpdateConfidence sets the confidence value for a memory.
func (s *Store) UpdateConfidence(ctx context.Context, id string, confidence float64) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := s.db.ExecContext(ctx,
		"UPDATE memories SET confidence = ?, updated_at = ? WHERE id = ?",
		confidence, now, id,
	)
	if err != nil {
		return fmt.Errorf("memory: update confidence %q: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory: not found: %s", id)
	}
	return nil
}

// --- scan helpers ---

// scanner abstracts *sql.Row and *sql.Rows for shared scan logic.
type scanner interface {
	Scan(dest ...any) error
}

func scanMemory(sc scanner) (*Memory, error) {
	var m Memory
	var (
		cat, scope, status string
		createdAt, updatedAt string
		embeddingBlob        []byte
		lastUsed             sql.NullString
		metaStr              sql.NullString
	)

	err := sc.Scan(
		&m.ID, &m.Content, &cat, &scope, &status, &m.Confidence, &m.Source,
		&createdAt, &updatedAt, &embeddingBlob, &m.UseCount, &lastUsed, &metaStr,
	)
	if err != nil {
		return nil, err
	}

	m.Category = Category(cat)
	m.Scope = Scope(scope)
	m.Status = Status(status)

	m.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	m.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)

	if lastUsed.Valid {
		t, _ := time.Parse(timeFormat, lastUsed.String)
		m.LastUsedAt = &t
	}

	if len(embeddingBlob) > 0 {
		m.Embedding = decodeFloat32s(embeddingBlob)
	}

	if metaStr.Valid && metaStr.String != "" {
		m.Metadata = make(map[string]any)
		_ = json.Unmarshal([]byte(metaStr.String), &m.Metadata)
	}

	return &m, nil
}

func scanMemoryRows(rows *sql.Rows) (*Memory, error) {
	return scanMemory(rows)
}

// --- float32 encoding for embedding BLOB ---

// encodeFloat32s converts a []float32 to a little-endian byte slice.
func encodeFloat32s(fs []float32) []byte {
	buf := make([]byte, len(fs)*4)
	for i, f := range fs {
		bits := math.Float32bits(f)
		buf[i*4+0] = byte(bits)
		buf[i*4+1] = byte(bits >> 8)
		buf[i*4+2] = byte(bits >> 16)
		buf[i*4+3] = byte(bits >> 24)
	}
	return buf
}

// decodeFloat32s converts a little-endian byte slice to []float32.
func decodeFloat32s(b []byte) []float32 {
	n := len(b) / 4
	fs := make([]float32, n)
	for i := range n {
		bits := uint32(b[i*4]) | uint32(b[i*4+1])<<8 | uint32(b[i*4+2])<<16 | uint32(b[i*4+3])<<24
		fs[i] = math.Float32frombits(bits)
	}
	return fs
}
