// Package db owns the SQLite connection and the schema migration that runs
// at startup. SQLite is used as the persistent store: it needs no external
// server, ships a single durable file that survives restarts/redeploys as
// long as the file's directory is on a persistent volume, and is more than
// enough for this assignment's scale (a handful of windows, a few dozen
// media items each).
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

const schema = `
CREATE TABLE IF NOT EXISTS windows (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	cycle_anchor DATETIME NOT NULL,
	created_at   DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS media_items (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	window_id         TEXT NOT NULL REFERENCES windows(id) ON DELETE CASCADE,
	type              TEXT NOT NULL CHECK(type IN ('image','video','blank')),
	url               TEXT NOT NULL DEFAULT '',
	duration_seconds  INTEGER NOT NULL CHECK(duration_seconds > 0),
	position          INTEGER NOT NULL,
	created_at        DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_media_items_window ON media_items(window_id, position);

CREATE TABLE IF NOT EXISTS sync_events (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	media_id          INTEGER,
	type              TEXT NOT NULL,
	url               TEXT NOT NULL,
	duration_seconds  INTEGER NOT NULL,
	starts_at         DATETIME NOT NULL,
	ends_at           DATETIME NOT NULL,
	created_at        DATETIME NOT NULL
);
`

// Open creates (if needed) the database file's parent directory, opens the
// SQLite connection, enables foreign-key enforcement (off by default in
// SQLite), and applies the schema. It is safe to call on every startup —
// every statement is idempotent (CREATE TABLE IF NOT EXISTS).
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	// _foreign_keys=on: without this, SQLite silently ignores the
	// REFERENCES ... ON DELETE CASCADE clause above.
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on", path)
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite only supports one writer at a time; capping the pool avoids
	// "database is locked" errors under concurrent requests.
	conn.SetMaxOpenConns(1)

	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return conn, nil
}
