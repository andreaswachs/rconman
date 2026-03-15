package rcon

import (
	"context"
	"testing"
)

// Verify that Client interface is implemented by MockClient
var _ Client = (*MockClient)(nil)

// MockClient is a test implementation of the Client interface
type MockClient struct {
	responses map[string]string
	players   []string
	connected bool
	lastCmd   string
}

// NewMockClient creates a new mock client for testing
func NewMockClient() *MockClient {
	return &MockClient{
		responses: make(map[string]string),
		players:   []string{},
		connected: true,
	}
}

// SetResponse sets a response for a given command
func (m *MockClient) SetResponse(cmd, resp string) {
	m.responses[cmd] = resp
}

// SetPlayers sets the list of players to return
func (m *MockClient) SetPlayers(players []string) {
	m.players = players
}

// SetConnected sets the connected state
func (m *MockClient) SetConnected(connected bool) {
	m.connected = connected
}

// Send sends a command and returns the mapped response
func (m *MockClient) Send(ctx context.Context, command string) (string, error) {
	m.lastCmd = command
	if !m.connected {
		return "", ErrNotConnected
	}
	if resp, ok := m.responses[command]; ok {
		return resp, nil
	}
	return "", nil
}

// PlayerList returns the list of players
func (m *MockClient) PlayerList(ctx context.Context) ([]string, error) {
	if !m.connected {
		return nil, ErrNotConnected
	}
	return m.players, nil
}

// IsConnected returns the connection status
func (m *MockClient) IsConnected() bool {
	return m.connected
}

// Close closes the mock connection
func (m *MockClient) Close() error {
	m.connected = false
	return nil
}

// LastCommand returns the last command sent (for testing)
func (m *MockClient) LastCommand() string {
	return m.lastCmd
}

// TestMockClientSend tests that MockClient.Send returns mapped responses
func TestMockClientSend(t *testing.T) {
	mock := NewMockClient()
	mock.SetResponse("say Hello", "Hello")

	resp, err := mock.Send(context.Background(), "say Hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "Hello" {
		t.Fatalf("expected 'Hello', got '%s'", resp)
	}
}

// TestMockClientNotConnected tests that disconnected state returns error
func TestMockClientNotConnected(t *testing.T) {
	mock := NewMockClient()
	mock.SetConnected(false)

	_, err := mock.Send(context.Background(), "say Hello")
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

// TestClientInterface verifies the Client interface exists
func TestClientInterface(t *testing.T) {
	mock := NewMockClient()
	var c Client = mock
	if c == nil {
		t.Fatal("expected Client interface to be implemented")
	}
}

// TestPacketEncode tests packet encoding
func TestPacketEncode(t *testing.T) {
	packet := &Packet{
		ID:      1,
		Type:    typeExec,
		Payload: "say test",
	}

	data, err := packet.Encode()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(data) < 4 {
		t.Fatalf("expected packet data to be at least 4 bytes, got %d", len(data))
	}
}

// TestPacketEncodeTooLarge tests that oversized packets return error
func TestPacketEncodeTooLarge(t *testing.T) {
	// Create a payload larger than maxPacketSize
	largePayload := make([]byte, maxPacketSize+1)
	packet := &Packet{
		ID:      1,
		Type:    typeExec,
		Payload: string(largePayload),
	}

	_, err := packet.Encode()
	if err != ErrPacketTooLarge {
		t.Fatalf("expected ErrPacketTooLarge, got %v", err)
	}
}
