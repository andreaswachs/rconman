package store

import (
	"context"
	"time"
)

// CommandLog represents a recorded command execution.
type CommandLog struct {
	ID        int64
	Timestamp time.Time
	UserEmail string
	ServerID  string
	Command   string
	Response  string
	DurationMS int64
}

// Store defines the interface for command logging persistence.
type Store interface {
	// RecordCommand stores a command execution log.
	RecordCommand(ctx context.Context, email, serverID, command, response string, durationMS int64) error

	// GetLogs retrieves the most recent command logs up to the specified limit.
	GetLogs(ctx context.Context, limit int) ([]CommandLog, error)

	// PruneOlderThan deletes command logs older than the specified age.
	PruneOlderThan(ctx context.Context, age time.Duration) error
}
