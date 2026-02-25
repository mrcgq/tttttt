package transport

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// readFrame reads a single WebSocket frame from the reader.
// Server frames are expected to be unmasked (per RFC 6455).
func readFrame(r io.Reader) (opcode byte, payload []byte, fin bool, err error) {
	header := make([]byte, 2)
	if _, err = io.ReadFull(r, header); err != nil {
		return 0, nil, false, fmt.Errorf("ws: read frame header: %w", err)
	}

	fin = (header[0] & 0x80) != 0
	opcode = header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
	length := uint64(header[1] & 0x7F)

	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err = io.ReadFull(r, ext); err != nil {
			return 0, nil, false, fmt.Errorf("ws: read ext length 16: %w", err)
		}
		length = uint64(binary.BigEndian.Uint16(ext))

	case 127:
		ext := make([]byte, 8)
		if _, err = io.ReadFull(r, ext); err != nil {
			return 0, nil, false, fmt.Errorf("ws: read ext length 64: %w", err)
		}
		length = binary.BigEndian.Uint64(ext)
	}

	var maskKey [4]byte
	if masked {
		if _, err = io.ReadFull(r, maskKey[:]); err != nil {
			return 0, nil, false, fmt.Errorf("ws: read mask key: %w", err)
		}
	}

	if length > 64*1024*1024 {
		return 0, nil, false, fmt.Errorf("ws: frame too large: %d bytes", length)
	}

	payload = make([]byte, length)
	if length > 0 {
		if _, err = io.ReadFull(r, payload); err != nil {
			return 0, nil, false, fmt.Errorf("ws: read payload: %w", err)
		}
	}

	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return opcode, payload, fin, nil
}

// writeFrameBytes writes a WebSocket frame with explicit FIN control.
// Client frames MUST be masked (per RFC 6455).
func writeFrameBytes(w net.Conn, finFlag bool, opcode byte, payload []byte) (int, error) {
	length := len(payload)

	headerSize := 2 + 4
	if length >= 126 && length < 65536 {
		headerSize += 2
	} else if length >= 65536 {
		headerSize += 8
	}

	frame := make([]byte, 0, headerSize+length)

	firstByte := opcode & 0x0F
	if finFlag {
		firstByte |= 0x80
	}
	frame = append(frame, firstByte)

	switch {
	case length < 126:
		frame = append(frame, 0x80|byte(length))
	case length < 65536:
		frame = append(frame, 0x80|126)
		ext := make([]byte, 2)
		binary.BigEndian.PutUint16(ext, uint16(length))
		frame = append(frame, ext...)
	default:
		frame = append(frame, 0x80|127)
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(length))
		frame = append(frame, ext...)
	}

	var maskKey [4]byte
	if _, err := rand.Read(maskKey[:]); err != nil {
		return 0, fmt.Errorf("ws: generate mask key: %w", err)
	}
	frame = append(frame, maskKey[:]...)

	masked := make([]byte, length)
	for i := 0; i < length; i++ {
		masked[i] = payload[i] ^ maskKey[i%4]
	}
	frame = append(frame, masked...)

	n, err := w.Write(frame)
	if err != nil {
		return 0, fmt.Errorf("ws: write frame: %w", err)
	}

	_ = n
	return length, nil
}

// writeFrame writes a WebSocket frame with FIN=1 (final frame).
func writeFrame(w net.Conn, opcode byte, payload []byte) (int, error) {
	return writeFrameBytes(w, true, opcode, payload)
}

// WriteCloseFrame sends a WebSocket close frame with a status code.
func WriteCloseFrame(w net.Conn, statusCode uint16) (int, error) {
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload, statusCode)
	return writeFrame(w, 0x08, payload)
}
