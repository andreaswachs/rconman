package rcon

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
)

// Client interface defines the RCON client contract
type Client interface {
	Send(ctx context.Context, command string) (string, error)
	PlayerList(ctx context.Context) ([]string, error)
	IsConnected() bool
	Close() error
}

// RealClient implements the Client interface with actual network connectivity
type RealClient struct {
	host      string
	port      int
	password  string
	conn      net.Conn
	requestID int32
	mu        sync.Mutex
}

// NewRealClient creates a new RCON client (does not connect until first use)
func NewRealClient(host string, port int, password string) (*RealClient, error) {
	return &RealClient{
		host:     host,
		port:     port,
		password: password,
	}, nil
}

// ensureConnected dials and authenticates if not already connected.
// Must be called with mu held.
func (rc *RealClient) ensureConnected(ctx context.Context) error {
	if rc.conn != nil {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", rc.host, rc.port)

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to RCON server at %s: %w", addr, err)
	}

	authPacket := &Packet{
		ID:      1,
		Type:    typeAuth,
		Payload: rc.password,
	}

	data, err := authPacket.Encode()
	if err != nil {
		conn.Close()
		return err
	}

	if _, err := conn.Write(data); err != nil {
		conn.Close()
		return fmt.Errorf("failed to send auth packet: %w", err)
	}

	resp, err := DecodePacket(conn)
	if err != nil {
		conn.Close()
		return err
	}

	if resp.ID == -1 {
		conn.Close()
		return ErrAuthFailed
	}

	rc.conn = conn
	return nil
}

// Send sends a command to the RCON server and returns the response
func (rc *RealClient) Send(ctx context.Context, command string) (string, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if err := rc.ensureConnected(ctx); err != nil {
		return "", err
	}

	if deadline, ok := ctx.Deadline(); ok {
		rc.conn.SetDeadline(deadline) //nolint:errcheck
	}

	rc.requestID++
	packet := &Packet{
		ID:      rc.requestID,
		Type:    typeExec,
		Payload: command,
	}

	data, err := packet.Encode()
	if err != nil {
		return "", err
	}

	if _, err := rc.conn.Write(data); err != nil {
		rc.conn.Close()
		rc.conn = nil
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	resp, err := DecodePacket(rc.conn)
	if err != nil {
		rc.conn.Close()
		rc.conn = nil
		return "", err
	}

	return resp.Payload, nil
}

// PlayerList retrieves the list of players from the server
func (rc *RealClient) PlayerList(ctx context.Context) ([]string, error) {
	response, err := rc.Send(ctx, "list")
	if err != nil {
		return nil, err
	}

	// Minecraft response: "There are N of a max of M players online: p1, p2, ..."
	_, after, found := strings.Cut(response, "players online: ")
	if !found {
		return []string{}, nil
	}

	playerStr := strings.TrimSpace(after)
	if playerStr == "" {
		return []string{}, nil
	}

	parts := strings.Split(playerStr, ", ")
	players := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			players = append(players, p)
		}
	}
	return players, nil
}

// IsConnected returns whether the client is currently connected
func (rc *RealClient) IsConnected() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.conn != nil
}

// Close closes the RCON connection
func (rc *RealClient) Close() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.conn == nil {
		return nil
	}

	err := rc.conn.Close()
	rc.conn = nil
	return err
}
