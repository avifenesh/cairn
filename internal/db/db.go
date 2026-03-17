// Package db provides the SQLite database layer for Pub v2.
//
// It uses modernc.org/sqlite (pure Go, no CGO) with WAL mode and
// embedded SQL migrations applied in filename order.
package db

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver
)

// DB wraps a *sql.DB with Pub-specific helpers.
type DB struct {
	*sql.DB
	path string
}

// Open creates (or opens) a SQLite database at path and applies
// performance PRAGMAs. Use ":memory:" for an in-memory database.
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("db: open %q: %w", path, err)
	}

	// Single writer connection — WAL mode handles concurrency.
	sqlDB.SetMaxOpenConns(1)

	d := &DB{DB: sqlDB, path: path}

	if err := d.applyPragmas(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("db: pragmas: %w", err)
	}

	slog.Info("database opened", "path", path)
	return d, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	slog.Info("database closing", "path", d.path)
	return d.DB.Close()
}

// applyPragmas sets SQLite performance and safety pragmas.
func (d *DB) applyPragmas() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA cache_size=-64000",
		"PRAGMA mmap_size=268435456",
	}
	for _, p := range pragmas {
		if _, err := d.Exec(p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}
