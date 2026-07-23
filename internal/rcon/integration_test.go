package rcon

import (
	"context"
	"net"
	"testing"
	"time"
)

func startMockRCON(t *testing.T, password string) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleMockRCON(conn, password)
		}
	}()

	addr := ln.Addr().String()
	return addr, func() { ln.Close() }
}

func handleMockRCON(conn net.Conn, password string) {
	defer conn.Close()
	var authed bool

	for {
		pkt, err := readMockPacket(conn)
		if err != nil {
			return
		}

		switch pkt.Type {
		case 3: // auth
			if pkt.Payload == password {
				authed = true
				writeMockPacket(conn, pkt.ID, 3, "")
			} else {
				writeMockPacket(conn, -1, 3, "")
			}
		case 2: // exec
			if !authed {
				writeMockPacket(conn, pkt.ID, 0, "not authenticated")
				continue
			}
			switch pkt.Payload {
			case "list":
				writeMockPacket(conn, pkt.ID, 0, "There are 2 of a max of 20 players online: Steve, Alex")
			case "save-all":
				writeMockPacket(conn, pkt.ID, 0, "Saved the game")
			default:
				writeMockPacket(conn, pkt.ID, 0, "ok: "+pkt.Payload)
			}
		}
	}
}

func readMockPacket(r interface{ Read([]byte) (int, error) }) (*mockPacket, error) {
	lenBuf := make([]byte, 4)
	if _, err := readFull(r, lenBuf); err != nil {
		return nil, err
	}
	length := uint32(lenBuf[0]) | uint32(lenBuf[1])<<8 | uint32(lenBuf[2])<<16 | uint32(lenBuf[3])<<24
	body := make([]byte, length)
	if _, err := readFull(r, body); err != nil {
		return nil, err
	}
	pkt := &mockPacket{
		ID:   int32(uint32(body[0]) | uint32(body[1])<<8 | uint32(body[2])<<16 | uint32(body[3])<<24),
		Type: int32(uint32(body[4]) | uint32(body[5])<<8 | uint32(body[6])<<16 | uint32(body[7])<<24),
	}
	if len(body) > 9 {
		pkt.Payload = string(body[8 : len(body)-1])
	}
	return pkt, nil
}

func readFull(r interface{ Read([]byte) (int, error) }, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func writeMockPacket(w net.Conn, id int32, pktType int32, payload string) {
	payloadBytes := []byte(payload)
	bodySize := 4 + 4 + len(payloadBytes) + 1
	buf := make([]byte, 4+bodySize)
	buf[0] = byte(bodySize)
	buf[1] = byte(bodySize >> 8)
	buf[2] = byte(bodySize >> 16)
	buf[3] = byte(bodySize >> 24)
	buf[4] = byte(uint32(id))
	buf[5] = byte(uint32(id) >> 8)
	buf[6] = byte(uint32(id) >> 16)
	buf[7] = byte(uint32(id) >> 24)
	buf[8] = byte(uint32(pktType))
	buf[9] = byte(uint32(pktType) >> 8)
	buf[10] = byte(uint32(pktType) >> 16)
	buf[11] = byte(uint32(pktType) >> 24)
	copy(buf[12:12+len(payloadBytes)], payloadBytes)
	buf[12+len(payloadBytes)] = 0
	w.Write(buf)
}

type mockPacket struct {
	ID      int32
	Type    int32
	Payload string
}

func TestRealClientSendAndPlayerList(t *testing.T) {
	addr, cleanup := startMockRCON(t, "testpass")
	defer cleanup()

	host, port, _ := net.SplitHostPort(addr)
	p, _ := net.LookupPort("tcp", port)

	client, err := NewRealClient(host, p, "testpass")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Send(ctx, "save-all")
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if resp != "Saved the game" {
		t.Errorf("expected 'Saved the game', got %q", resp)
	}

	players, err := client.PlayerList(ctx)
	if err != nil {
		t.Fatalf("player list failed: %v", err)
	}
	if len(players) != 2 {
		t.Fatalf("expected 2 players, got %d", len(players))
	}
	if players[0] != "Steve" || players[1] != "Alex" {
		t.Errorf("expected Steve, Alex, got %v", players)
	}
}

func TestRealClientAuthFailure(t *testing.T) {
	addr, cleanup := startMockRCON(t, "correctpass")
	defer cleanup()

	host, port, _ := net.SplitHostPort(addr)
	p, _ := net.LookupPort("tcp", port)

	client, err := NewRealClient(host, p, "wrongpass")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.Send(ctx, "list")
	if err != ErrAuthFailed {
		t.Errorf("expected ErrAuthFailed, got %v", err)
	}
}

func TestRealClientReconnect(t *testing.T) {
	addr, cleanup := startMockRCON(t, "testpass")
	defer cleanup()

	host, port, _ := net.SplitHostPort(addr)
	p, _ := net.LookupPort("tcp", port)

	client, err := NewRealClient(host, p, "testpass")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First command succeeds
	_, err = client.Send(ctx, "save-all")
	if err != nil {
		t.Fatalf("first send failed: %v", err)
	}

	// Simulate connection drop — close the underlying conn
	client.mu.Lock()
	if client.conn != nil {
		client.conn.Close()
		client.conn = nil
	}
	client.mu.Unlock()

	// Next command should reconnect and succeed
	resp, err := client.Send(ctx, "save-all")
	if err != nil {
		t.Fatalf("reconnect send failed: %v", err)
	}
	if resp != "Saved the game" {
		t.Errorf("expected 'Saved the game', got %q", resp)
	}
}
