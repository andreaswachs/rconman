package mock

import (
	"context"

	"github.com/your-org/rconman/internal/rcon"
)

// Mock is a complete mock implementation of the rcon.Client interface
type Mock struct {
	responses map[string]string
	players   []string
	connected bool
	lastCmd   string
}

// New creates a new mock RCON client
func New() *Mock {
	return &Mock{
		responses: make(map[string]string),
		players:   []string{},
		connected: true,
	}
}

// SetResponse sets a response for a given command
func (m *Mock) SetResponse(cmd, resp string) {
	m.responses[cmd] = resp
}

// SetPlayers sets the list of players to return
func (m *Mock) SetPlayers(players []string) {
	m.players = players
}

// SetConnected sets the connected state
func (m *Mock) SetConnected(connected bool) {
	m.connected = connected
}

// Send sends a command and returns the mapped response
func (m *Mock) Send(ctx context.Context, command string) (string, error) {
	m.lastCmd = command
	if !m.connected {
		return "", rcon.ErrNotConnected
	}
	if resp, ok := m.responses[command]; ok {
		return resp, nil
	}
	return "", nil
}

// PlayerList returns the list of players
func (m *Mock) PlayerList(ctx context.Context) ([]string, error) {
	if !m.connected {
		return nil, rcon.ErrNotConnected
	}
	return m.players, nil
}

// IsConnected returns the connection status
func (m *Mock) IsConnected() bool {
	return m.connected
}

// Close closes the mock connection
func (m *Mock) Close() error {
	m.connected = false
	return nil
}

// LastCommand returns the last command sent (for testing)
func (m *Mock) LastCommand() string {
	return m.lastCmd
}
