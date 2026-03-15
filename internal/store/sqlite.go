package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store with SQLite backend.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-backed store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create table if not exists
	schema := `
	CREATE TABLE IF NOT EXISTS command_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		user_email TEXT NOT NULL,
		server_id TEXT NOT NULL,
		command TEXT NOT NULL,
		response TEXT NOT NULL,
		duration_ms INTEGER NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_command_logs_timestamp ON command_logs(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_command_logs_server_id ON command_logs(server_id);
	CREATE INDEX IF NOT EXISTS idx_command_logs_user_email ON command_logs(user_email);
	`

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// RecordCommand logs a command execution.
func (s *SQLiteStore) RecordCommand(ctx context.Context, email, serverID, command, response string, durationMS int64) error {
	query := `
	INSERT INTO command_logs (timestamp, user_email, server_id, command, response, duration_ms)
	VALUES (datetime('now'), ?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecContext(ctx, query, email, serverID, command, response, durationMS)
	return err
}

// GetLogs retrieves recent log entries.
func (s *SQLiteStore) GetLogs(ctx context.Context, limit int) ([]CommandLog, error) {
	query := `
	SELECT id, timestamp, user_email, server_id, command, response, duration_ms
	FROM command_logs
	ORDER BY timestamp DESC
	LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []CommandLog
	for rows.Next() {
		var log CommandLog
		var timestamp string
		if err := rows.Scan(&log.ID, &timestamp, &log.UserEmail, &log.ServerID, &log.Command, &log.Response, &log.DurationMS); err != nil {
			return nil, err
		}
		log.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestamp)
		logs = append(logs, log)
	}

	return logs, rows.Err()
}

// PruneOlderThan deletes log entries older than the specified duration.
func (s *SQLiteStore) PruneOlderThan(ctx context.Context, age time.Duration) error {
	query := `DELETE FROM command_logs WHERE timestamp < datetime('now', '-' || ? || ' seconds')`
	_, err := s.db.ExecContext(ctx, query, int64(age.Seconds()))
	return err
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
