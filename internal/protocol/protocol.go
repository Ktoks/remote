package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

// Packet Types
const (
	TypeStdout = 0x01
	TypeStderr = 0x02
	TypeExit   = 0x03
)

// Packet represents a decoded message.
type Packet struct {
	Type uint8
	Data []byte
	Code uint32 // Used only for TypeExit
}

// Encoder prevents interleaved writes to the socket.
type Encoder struct {
	mu     sync.Mutex
	writer io.Writer
}

// NewEncoder returns a new io.Writer encoder
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{writer: w}
}

// Encode writes a packet to the wire in format: [Type:1][Len:4][Payload:N]
func (e *Encoder) Encode(pType uint8, data []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	header := make([]byte, 5)
	header[0] = pType
	binary.BigEndian.PutUint32(header[1:], uint32(len(data)))

	// Write Header
	if _, err := e.writer.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Write Payload
	if len(data) > 0 {
		if _, err := e.writer.Write(data); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}
	return nil
}

// DecodeLoop reads from the reader and executes callbacks based on packet type.
// It returns when EOF is reached or an error occurs.
func DecodeLoop(r io.Reader, onStdout, onStderr func([]byte), onExit func(int) bool) error {
	header := make([]byte, 5)

	for {
		if _, err := io.ReadFull(r, header); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read header: %w", err)
		}

		pType := header[0]
		pLen := binary.BigEndian.Uint32(header[1:])

		var payload []byte
		if pLen > 0 {
			payload = make([]byte, pLen)
			if _, err := io.ReadFull(r, payload); err != nil {
				return fmt.Errorf("read payload: %w", err)
			}
		}

		switch pType {
		case TypeStdout:
			if onStdout != nil {
				onStdout(payload)
			}
		case TypeStderr:
			if onStderr != nil {
				onStderr(payload)
			}
		case TypeExit:
			code := int(binary.BigEndian.Uint32(payload))
			if onExit != nil {
				shouldStop := onExit(code)
				if shouldStop {
					return nil
				}
			}
		}
	}
}
