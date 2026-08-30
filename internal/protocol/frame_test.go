package protocol

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
)

func TestFrameGoldenPing(t *testing.T) {
	frame, err := EncodeFrame([]byte("ping"))
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(frame); got != "0400000070696e67" {
		t.Fatalf("golden frame mismatch: %s", got)
	}
	payload, err := DecodeFrame(frame, DefaultMaxPayloadBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, []byte("ping")) {
		t.Fatalf("payload mismatch: %q", payload)
	}
}

func TestDecodeRejectsMismatch(t *testing.T) {
	_, err := DecodeFrame([]byte{4, 0, 0, 0, 'p'}, DefaultMaxPayloadBytes)
	if !errors.Is(err, ErrLengthMismatch) {
		t.Fatalf("expected ErrLengthMismatch, got %v", err)
	}
}

func FuzzDecodeFrame(f *testing.F) {
	f.Add([]byte{4, 0, 0, 0, 'p', 'i', 'n', 'g'})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		payload, err := DecodeFrame(data, 64*1024)
		if err != nil {
			return
		}
		frame, err := EncodeFrame(payload)
		if err != nil {
			t.Fatalf("re-encode accepted payload: %v", err)
		}
		decoded, err := DecodeFrame(frame, 64*1024)
		if err != nil {
			t.Fatalf("round trip decode: %v", err)
		}
		if !bytes.Equal(decoded, payload) {
			t.Fatal("round trip payload mismatch")
		}
	})
}
