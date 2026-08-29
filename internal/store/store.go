// Package store persists Seatkey's data in SQLite (via the pure-Go
// modernc.org/sqlite driver, so the server ships as a single static binary
// with no cgo and no external database to run).
package store

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

var (
	// ErrNotFound is returned when a lookup by ID or key matches no row.
	ErrNotFound = errors.New("not found")
	// ErrKeyExists is returned on the (astronomically unlikely) collision of
	// a freshly generated license key with an existing one.
	ErrKeyExists = errors.New("license key already exists")
)

// Store wraps the SQLite connection backing all of Seatkey's persistence.
type Store struct {
	db *sql.DB
}

// Open creates (if needed) and migrates the SQLite database at path.
// Use ":memory:" for an ephemeral in-process database, primarily for tests.
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite allows only one writer at a time; a single connection avoids
	// SQLITE_BUSY errors under concurrent requests instead of masking them.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
