package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestVersionNegotiation(t *testing.T) {
	server := Version{Major: 1, Minor: 2}

	// Same major, same or different minor -> should succeed
	if err := NegotiateVersion(Version{Major: 1, Minor: 0}, server); err != nil {
		t.Fatalf("expected compatible minor version, got %v", err)
	}
	if err := NegotiateVersion(Version{Major: 1, Minor: 5}, server); err != nil {
		t.Fatalf("expected compatible minor version, got %v", err)
	}

	// Major version mismatch -> must fail closed
	if err := NegotiateVersion(Version{Major: 2, Minor: 0}, server); err == nil {
		t.Fatalf("expected error on major version mismatch, got nil")
	}
}

func TestHandshakeRequestValidation(t *testing.T) {
	req := HandshakeRequest{
		ProtocolVersion: Version{Major: 1, Minor: 0},
		SDKVersion:      "v0.1.0",
		ClientPID:       1234,
		Nonce:           "random-nonce-123",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("valid handshake failed validation: %v", err)
	}

	invalidReq := req
	invalidReq.Nonce = ""
	if err := invalidReq.Validate(); err == nil {
		t.Fatalf("expected error on empty nonce")
	}
}

func TestRequestEnvelopeValidationAndRoundtrip(t *testing.T) {
	req := RequestEnvelope{
		RequestID:     "req-100",
		CorrelationID: "corr-100",
		Op:            "part.open",
		TimeoutMs:     5000,
		Payload:       json.RawMessage(`{"path":"C:\\test.prt"}`),
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("valid request failed validation: %v", err)
	}

	data, err := EncodePayload(req)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	decoded, err := DecodePayload[RequestEnvelope](data)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.RequestID != req.RequestID || decoded.Op != req.Op {
		t.Fatalf("decoded mismatch: got %+v, want %+v", decoded, req)
	}

	// Validation edge cases
	noID := req
	noID.RequestID = ""
	if err := noID.Validate(); err == nil {
		t.Fatalf("expected error on empty request_id")
	}

	noOp := req
	noOp.Op = ""
	if err := noOp.Validate(); err == nil {
		t.Fatalf("expected error on empty op")
	}
}

func TestResponseEnvelopeAndErrorValidation(t *testing.T) {
	errEnv := &ErrorEnvelope{
		Category:      ErrCategoryNXException,
		NXErrorCode:   1001,
		Message:       "Part file not found",
		Op:            "part.open",
		Recoverable:   false,
		SessionHealth: "healthy",
		CorrelationID: "corr-100",
	}
	if err := errEnv.Validate(); err != nil {
		t.Fatalf("valid error envelope failed validation: %v", err)
	}

	resp := ResponseEnvelope{
		RequestID: "req-100",
		Status:    StatusError,
		Error:     errEnv,
		Timing: TimingData{
			ExecutionMs:   15,
			TotalDuration: 20,
		},
	}

	data, err := EncodePayload(resp)
	if err != nil {
		t.Fatalf("encode response failed: %v", err)
	}

	decoded, err := DecodePayload[ResponseEnvelope](data)
	if err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if decoded.Status != StatusError || decoded.Error == nil || decoded.Error.SessionHealth != "healthy" {
		t.Fatalf("decoded response mismatch: %+v", decoded)
	}

	// Invalid session health
	invalidErr := *errEnv
	invalidErr.SessionHealth = "corrupted_unknown"
	if err := invalidErr.Validate(); err == nil {
		t.Fatalf("expected error on invalid session health")
	}
}

func TestStreamEventEncoding(t *testing.T) {
	evt := StreamEvent{
		Kind:          StreamKindLog,
		CorrelationID: "corr-100",
		Sequence:      1,
		Timestamp:     time.Now().UTC(),
		Payload:       json.RawMessage(`{"level":"info","msg":"opening part"}`),
		LossMarker:    false,
	}

	data, err := EncodePayload(evt)
	if err != nil {
		t.Fatalf("encode stream event failed: %v", err)
	}

	decoded, err := DecodePayload[StreamEvent](data)
	if err != nil {
		t.Fatalf("decode stream event failed: %v", err)
	}

	if decoded.Kind != StreamKindLog || decoded.Sequence != 1 {
		t.Fatalf("decoded stream event mismatch: %+v", decoded)
	}
}

func TestFramedProtocolRoundtrip(t *testing.T) {
	req := RequestEnvelope{
		RequestID: "req-framed-1",
		Op:        "nx.ping",
		Payload:   json.RawMessage(`{}`),
	}

	payload, err := EncodePayload(req)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}

	frame, err := EncodeFrame(payload)
	if err != nil {
		t.Fatalf("encode frame: %v", err)
	}

	decodedPayload, err := DecodeFrame(frame, DefaultMaxPayloadBytes)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}

	decodedReq, err := DecodePayload[RequestEnvelope](decodedPayload)
	if err != nil {
		t.Fatalf("decode req from frame: %v", err)
	}

	if decodedReq.RequestID != "req-framed-1" || decodedReq.Op != "nx.ping" {
		t.Fatalf("mismatch after framed roundtrip: %+v", decodedReq)
	}
}

func BenchmarkProtocolEncodeRequest(b *testing.B) {
	req := RequestEnvelope{
		RequestID:     "req-bench-1",
		CorrelationID: "corr-bench-1",
		Op:            "part.open",
		TimeoutMs:     5000,
		Payload:       json.RawMessage(`{"path":"C:\\models\\test.prt"}`),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload, err := EncodePayload(req)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := EncodeFrame(payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProtocolDecodeRequest(b *testing.B) {
	req := RequestEnvelope{
		RequestID:     "req-bench-1",
		CorrelationID: "corr-bench-1",
		Op:            "part.open",
		TimeoutMs:     5000,
		Payload:       json.RawMessage(`{"path":"C:\\models\\test.prt"}`),
	}
	payload, _ := EncodePayload(req)
	frame, _ := EncodeFrame(payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decodedPayload, err := DecodeFrame(frame, DefaultMaxPayloadBytes)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := DecodePayload[RequestEnvelope](decodedPayload); err != nil {
			b.Fatal(err)
		}
	}
}

