package rcon

import (
	"encoding/binary"
	"io"
)

const (
	maxPacketSize = 4096
	typeAuth      = 3
	typeExec      = 2
	typeResp      = 0
)

// Packet represents an RCON protocol packet
type Packet struct {
	ID      int32
	Type    int32
	Payload string
}

// Encode serializes the packet to bytes with proper length prefix
func (p *Packet) Encode() ([]byte, error) {
	// Packet format: [length(4 bytes)][id(4 bytes)][type(4 bytes)][payload(variable)][null terminator(1 byte)]
	payloadBytes := []byte(p.Payload)
	bodySize := 4 + 4 + len(payloadBytes) + 1 // id + type + payload + null terminator

	if bodySize > maxPacketSize {
		return nil, ErrPacketTooLarge
	}

	// Total packet size is 4 bytes for length + body size
	totalSize := 4 + bodySize
	result := make([]byte, totalSize)

	// Write length prefix (little-endian)
	binary.LittleEndian.PutUint32(result[0:4], uint32(bodySize))

	// Write id (little-endian)
	binary.LittleEndian.PutUint32(result[4:8], uint32(p.ID))

	// Write type (little-endian)
	binary.LittleEndian.PutUint32(result[8:12], uint32(p.Type))

	// Write payload
	copy(result[12:12+len(payloadBytes)], payloadBytes)

	// Write null terminator
	result[12+len(payloadBytes)] = 0

	return result, nil
}

// DecodePacket reads and decodes a packet from the given reader
func DecodePacket(r io.Reader) (*Packet, error) {
	// Read length prefix (4 bytes)
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return nil, ErrInvalidPacket
	}

	length := binary.LittleEndian.Uint32(lengthBuf)
	if length > maxPacketSize {
		return nil, ErrPacketTooLarge
	}

	// Read the body
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, ErrInvalidPacket
	}

	// Parse id (4 bytes)
	if len(body) < 4 {
		return nil, ErrInvalidPacket
	}
	id := int32(binary.LittleEndian.Uint32(body[0:4]))

	// Parse type (4 bytes)
	if len(body) < 8 {
		return nil, ErrInvalidPacket
	}
	packetType := int32(binary.LittleEndian.Uint32(body[4:8]))

	// Parse payload (rest of body minus null terminator)
	payload := ""
	if len(body) > 9 {
		// Remove the null terminator at the end
		payload = string(body[8 : len(body)-1])
	}

	return &Packet{
		ID:      id,
		Type:    packetType,
		Payload: payload,
	}, nil
}
