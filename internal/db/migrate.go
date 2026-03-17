package db

import (
	"embed"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies all pending SQL migrations embedded in the binary.
// Migrations are sorted by filename (e.g. 001_init.sql, 002_foo.sql)
// and each is executed inside a transaction. Already-applied versions
// (tracked in schema_migrations) are skipped.
func (d *DB) Migrate() error {
	// Ensure the tracking table exists (idempotent).
	if _, err := d.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER PRIMARY KEY,
			applied_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)
	`); err != nil {
		return fmt.Errorf("migrate: create tracking table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("migrate: read embedded dir: %w", err)
	}

	// Sort by name to guarantee order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := parseVersion(entry.Name())
		if err != nil {
			return fmt.Errorf("migrate: parse version from %q: %w", entry.Name(), err)
		}

		// Check if already applied.
		var exists int
		if err := d.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&exists); err != nil {
			return fmt.Errorf("migrate: check version %d: %w", version, err)
		}
		if exists > 0 {
			slog.Debug("migration already applied", "version", version, "file", entry.Name())
			continue
		}

		// Read and execute migration.
		content, err := migrationsFS.ReadFile(path.Join("migrations", entry.Name()))
		if err != nil {
			return fmt.Errorf("migrate: read %q: %w", entry.Name(), err)
		}

		tx, err := d.Begin()
		if err != nil {
			return fmt.Errorf("migrate: begin tx for version %d: %w", version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate: exec %q: %w", entry.Name(), err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate: record version %d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrate: commit version %d: %w", version, err)
		}

		slog.Info("migration applied", "version", version, "file", entry.Name())
	}

	return nil
}

// parseVersion extracts the leading integer from a migration filename
// like "001_init.sql" -> 1.
func parseVersion(filename string) (int, error) {
	base := strings.TrimSuffix(filename, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) == 0 {
		return 0, fmt.Errorf("no version prefix in %q", filename)
	}
	v, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid version prefix %q: %w", parts[0], err)
	}
	return v, nil
}
