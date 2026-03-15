package rcon

import "fmt"

var (
	ErrNotConnected   = fmt.Errorf("RCON client not connected")
	ErrAuthFailed     = fmt.Errorf("RCON authentication failed")
	ErrPacketTooLarge = fmt.Errorf("RCON packet exceeds maximum size")
	ErrInvalidPacket  = fmt.Errorf("RCON invalid packet format")
	ErrTimeout        = fmt.Errorf("RCON request timeout")
)
