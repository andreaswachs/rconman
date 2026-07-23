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

	CREATE TABLE IF NOT EXISTS server_desired_state (
		server_id TEXT PRIMARY KEY,
		desired_state INTEGER NOT NULL DEFAULT 1,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS command_templates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id TEXT NOT NULL,
		category TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		command TEXT NOT NULL,
		params TEXT NOT NULL DEFAULT '[]',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_command_templates_server_id ON command_templates(server_id);
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

// GetDesiredState returns the desired state (1=running, 0=stopped) for a server.
func (s *SQLiteStore) GetDesiredState(ctx context.Context, serverID string) (int, error) {
	var state int
	err := s.db.QueryRowContext(ctx,
		`SELECT desired_state FROM server_desired_state WHERE server_id = ?`, serverID,
	).Scan(&state)
	if err == sql.ErrNoRows {
		return 1, nil // default: running
	}
	return state, err
}

// SetDesiredState sets the desired state for a server.
func (s *SQLiteStore) SetDesiredState(ctx context.Context, serverID string, state int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO server_desired_state (server_id, desired_state, updated_at)
		 VALUES (?, ?, datetime('now'))
		 ON CONFLICT(server_id) DO UPDATE SET desired_state = excluded.desired_state, updated_at = datetime('now')`,
		serverID, state)
	return err
}

// GetAllDesiredStates returns a map of server_id to desired state.
func (s *SQLiteStore) GetAllDesiredStates(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT server_id, desired_state FROM server_desired_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var id string
		var state int
		if err := rows.Scan(&id, &state); err != nil {
			return nil, err
		}
		result[id] = state
	}
	return result, rows.Err()
}

// GetTemplates returns all command templates for a server.
func (s *SQLiteStore) GetTemplates(ctx context.Context, serverID string) ([]StoredTemplate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, server_id, category, name, description, command, params
		 FROM command_templates WHERE server_id = ? ORDER BY category, id`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []StoredTemplate
	for rows.Next() {
		var t StoredTemplate
		if err := rows.Scan(&t.ID, &t.ServerID, &t.Category, &t.Name, &t.Description, &t.Command, &t.Params); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// CreateTemplate inserts a new command template.
func (s *SQLiteStore) CreateTemplate(ctx context.Context, t StoredTemplate) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO command_templates (server_id, category, name, description, command, params)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.ServerID, t.Category, t.Name, t.Description, t.Command, t.Params)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateTemplate updates an existing command template by ID.
func (s *SQLiteStore) UpdateTemplate(ctx context.Context, t StoredTemplate) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE command_templates SET category=?, name=?, description=?, command=?, params=?, updated_at=datetime('now')
		 WHERE id=?`,
		t.Category, t.Name, t.Description, t.Command, t.Params, t.ID)
	return err
}

// DeleteTemplate removes a command template by ID.
func (s *SQLiteStore) DeleteTemplate(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM command_templates WHERE id=?`, id)
	return err
}
