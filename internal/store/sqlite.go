package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the sql.DB and provides access to typed stores.
type DB struct {
	*sql.DB
}

// NewDB opens a SQLite database and enables WAL mode and foreign keys.
func NewDB(dsn string) (*DB, error) {
	db, err := sql.Open("sqlite3", dsn+"?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &DB{db}, nil
}

// Migrate executes the given SQL migration string.
func (db *DB) Migrate(sql string) error {
	if _, err := db.ExecContext(context.Background(), sql); err != nil {
		return fmt.Errorf("exec migration: %w", err)
	}
	return nil
}
