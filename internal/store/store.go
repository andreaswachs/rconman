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

	// GetDesiredState returns the desired state (1=running, 0=stopped) for a server.
	GetDesiredState(ctx context.Context, serverID string) (int, error)

	// SetDesiredState sets the desired state for a server.
	SetDesiredState(ctx context.Context, serverID string, state int) error

	// GetAllDesiredStates returns a map of server_id to desired state.
	GetAllDesiredStates(ctx context.Context) (map[string]int, error)
}
