// Package db opens the SQLite database, runs migrations, and provides the
// query layer. It uses the pure-Go modernc.org/sqlite driver so the binary
// stays CGO-free and statically linkable.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps *sql.DB with Archivarr's query methods.
type DB struct {
	*sql.DB
}

// Open opens (creating if needed) the SQLite database at path with WAL mode,
// a busy timeout, and foreign-key enforcement applied to every connection.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating db dir: %w", err)
		}
	}

	// A rollback journal (not WAL) is used deliberately: WAL needs shared-memory
	// mmap that bind-mounted filesystems (e.g. Docker Desktop) don't support, and
	// it would lock the file exclusively, preventing external tools (DB Browser,
	// etc.) from reading it. busy_timeout covers brief write contention.
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	// Single connection: this is a low-traffic, single-user app, so serializing
	// access removes lock contention entirely and keeps behavior identical
	// across every filesystem.
	sqldb.SetMaxOpenConns(1)
	if err := sqldb.Ping(); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("connecting to sqlite: %w", err)
	}
	return &DB{sqldb}, nil
}
