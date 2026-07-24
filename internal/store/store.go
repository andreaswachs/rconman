package store

import (
	"context"
	"time"
)

// CommandLog represents a recorded command execution.
type CommandLog struct {
	ID         int64
	Timestamp  time.Time
	UserEmail  string
	ServerID   string
	Command    string
	Response   string
	DurationMS int64
}

// StoredTemplate represents a command template stored in the database.
type StoredTemplate struct {
	ID          int64
	ServerID    string
	Category    string
	Name        string
	Description string
	Command     string
	Params      string // JSON array of config.TemplateParam
}

// ServerSettings holds per-server jail/unjail coordinates.
type ServerSettings struct {
	ServerID string
	JailX   string
	JailY   string
	JailZ   string
	UnjailX string
	UnjailY string
	UnjailZ string
}

// Store defines the interface for persistence.
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

	// GetTemplates returns all command templates for a server, grouped by category.
	GetTemplates(ctx context.Context, serverID string) ([]StoredTemplate, error)

	// CreateTemplate inserts a new command template.
	CreateTemplate(ctx context.Context, t StoredTemplate) (int64, error)

	// UpdateTemplate updates an existing command template by ID.
	UpdateTemplate(ctx context.Context, t StoredTemplate) error

	// DeleteTemplate removes a command template by ID.
	DeleteTemplate(ctx context.Context, id int64) error

	// GetServerSettings returns jail/unjail coordinates for a server.
	GetServerSettings(ctx context.Context, serverID string) (ServerSettings, error)

	// SaveServerSettings saves jail/unjail coordinates for a server.
	SaveServerSettings(ctx context.Context, s ServerSettings) error
}
