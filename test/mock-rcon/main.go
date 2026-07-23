package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
)

const (
	typeAuth   = 3
	typeExec   = 2
	typeResp   = 0
	maxPacket  = 4096
)

func main() {
	listener, err := net.Listen("tcp", "0.0.0.0:25575")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	log.Println("Mock RCON server listening on :25575")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	var authenticated bool

	for {
		pkt, err := readPacket(conn)
		if err != nil {
			if err != io.EOF {
				log.Printf("read error: %v", err)
			}
			return
		}

		switch pkt.Type {
		case typeAuth:
			if pkt.Payload == "rcon-password" || pkt.Payload == "e2e-rcon-password" {
				authenticated = true
				writePacket(conn, pkt.ID, typeAuth, "")
			} else {
				writePacket(conn, -1, typeAuth, "")
			}
		case typeExec:
			if !authenticated {
				writePacket(conn, pkt.ID, typeResp, "not authenticated")
				continue
			}
			resp := handleCommand(pkt.Payload)
			writePacket(conn, pkt.ID, typeResp, resp)
		default:
			writePacket(conn, pkt.ID, typeResp, "unknown packet type")
		}
	}
}

func handleCommand(cmd string) string {
	switch {
	case cmd == "list":
		return "There are 2 of a max of 20 players online: Steve, Alex"
	case cmd == "save-all":
		return "Saved the game"
	case strings.HasPrefix(cmd, "say "):
		return ""
	case strings.HasPrefix(cmd, "give "):
		return "Given item to player"
	case strings.HasPrefix(cmd, "kick "):
		return "Kicked player"
	case strings.HasPrefix(cmd, "stop"):
		return "Stopping the server"
	case strings.HasPrefix(cmd, "difficulty "):
		return "Difficulty set to " + strings.TrimPrefix(cmd, "difficulty ")
	case strings.HasPrefix(cmd, "time set "):
		return "Set the time to " + strings.TrimPrefix(cmd, "time set ")
	default:
		return fmt.Sprintf("Unknown command: %s", cmd)
	}
}

func readPacket(r io.Reader) (*packet, error) {
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return nil, err
	}

	length := binary.LittleEndian.Uint32(lengthBuf)
	if length > maxPacket {
		return nil, fmt.Errorf("packet too large: %d", length)
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}

	if len(body) < 8 {
		return nil, fmt.Errorf("packet body too short")
	}

	pkt := &packet{
		ID:   int32(binary.LittleEndian.Uint32(body[0:4])),
		Type: int32(binary.LittleEndian.Uint32(body[4:8])),
	}
	if len(body) > 9 {
		pkt.Payload = string(body[8 : len(body)-1])
	}

	return pkt, nil
}

func writePacket(w io.Writer, id int32, pktType int32, payload string) {
	payloadBytes := []byte(payload)
	bodySize := 4 + 4 + len(payloadBytes) + 1

	buf := make([]byte, 4+bodySize)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(bodySize))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(id))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(pktType))
	copy(buf[12:12+len(payloadBytes)], payloadBytes)
	buf[12+len(payloadBytes)] = 0

	w.Write(buf)
}

type packet struct {
	ID      int32
	Type    int32
	Payload string
}
