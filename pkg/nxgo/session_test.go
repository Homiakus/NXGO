package nxgo_test

import (
	"testing"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/pkg/nxgo"
)

func TestWrapClientSessionFields(t *testing.T) {
	session := nxgo.WrapClient(nil, "sess-test-123", 1, "v2512")
	if session.SessionID() != "sess-test-123" {
		t.Fatalf("expected session ID sess-test-123, got %s", session.SessionID())
	}
	if session.Epoch() != 1 {
		t.Fatalf("expected epoch 1, got %d", session.Epoch())
	}
	if session.NXRelease() != "v2512" {
		t.Fatalf("expected release v2512, got %s", session.NXRelease())
	}
}

func TestProtocolTypedPayloadEncoding(t *testing.T) {
	// Verify JSON serialization round-trips for transactions and parts
	txReq := protocol.TransactionBeginRequest{Name: "test-tx"}
	b, err := protocol.EncodePayload(txReq)
	if err != nil {
		t.Fatalf("encode txReq failed: %v", err)
	}

	decoded, err := protocol.DecodePayload[protocol.TransactionBeginRequest](b)
	if err != nil {
		t.Fatalf("decode txReq failed: %v", err)
	}
	if decoded.Name != "test-tx" {
		t.Fatalf("expected test-tx, got %s", decoded.Name)
	}
}
