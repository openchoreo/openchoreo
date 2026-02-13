// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package checkpoint

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// Store provides a SQLite-based checkpoint database to track processed events.
// This enables deduplication across pod restarts of the events-collector.
type Store struct {
	db *sql.DB
}

// New creates a new checkpoint store at the given path.
// It creates the database file and events table if they don't exist.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open checkpoint database: %w", err)
	}

	// Enable WAL mode for better concurrent read/write performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	// Create the events table if it doesn't exist
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS events (
			event_uid TEXT PRIMARY KEY,
			timestamp INTEGER NOT NULL
		)
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create events table: %w", err)
	}

	return &Store{db: db}, nil
}

// Exists checks whether an event with the given UID has already been processed.
func (s *Store) Exists(eventUID string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM events WHERE event_uid = ?", eventUID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check event existence: %w", err)
	}
	return count > 0, nil
}

// Add a record for an event with the given UID to the checkpoint DB.
// It returns true if the event was newly inserted,
// false if the event already existed (duplicate)
func (s *Store) Add(eventUID string) (bool, error) {
	result, err := s.db.Exec(
		"INSERT OR IGNORE INTO events (event_uid, timestamp) VALUES (?, ?)",
		eventUID, time.Now().Unix(),
	)
	if err != nil {
		return false, fmt.Errorf("failed to add event to checkpoint: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

// CleanupBefore deletes all records older than the given cutoff time.
// Returns the number of deleted records.
func (s *Store) CleanupBefore(cutoff time.Time) (int64, error) {
	result, err := s.db.Exec("DELETE FROM events WHERE timestamp < ?", cutoff.Unix())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old events: %w", err)
	}
	return result.RowsAffected()
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
