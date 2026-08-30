package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	FrameHeaderSize = 4
	DefaultMaxPayloadBytes = 4 * 1024 * 1024
)

var (
	ErrTruncatedFrame = errors.New("truncated frame header")
	ErrInvalidLength  = errors.New("invalid frame length")
	ErrLengthMismatch = errors.New("frame length mismatch")
)

func EncodeFrame(payload []byte) ([]byte, error) {
	if len(payload) > DefaultMaxPayloadBytes {
		return nil, fmt.Errorf("%w: %d > %d", ErrInvalidLength, len(payload), DefaultMaxPayloadBytes)
	}
	frame := make([]byte, FrameHeaderSize+len(payload))
	binary.LittleEndian.PutUint32(frame[:FrameHeaderSize], uint32(len(payload)))
	copy(frame[FrameHeaderSize:], payload)
	return frame, nil
}

func DecodeFrame(frame []byte, maxPayload int) ([]byte, error) {
	if len(frame) < FrameHeaderSize {
		return nil, ErrTruncatedFrame
	}
	if maxPayload <= 0 {
		maxPayload = DefaultMaxPayloadBytes
	}
	length := uint64(binary.LittleEndian.Uint32(frame[:FrameHeaderSize]))
	if length > uint64(maxPayload) {
		return nil, ErrInvalidLength
	}
	if uint64(len(frame)-FrameHeaderSize) != length {
		return nil, ErrLengthMismatch
	}
	payload := make([]byte, int(length))
	copy(payload, frame[FrameHeaderSize:])
	return payload, nil
}
