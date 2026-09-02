package pipe

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestFramedClientServerRoundtrip(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)

		hsBytes, err := serverFramed.Receive()
		if err != nil {
			return
		}
		hsReq, err := protocol.DecodePayload[protocol.HandshakeRequest](hsBytes)
		if err != nil {
			return
		}
		if hsReq.Nonce != "nonce-1" {
			return
		}

		hsResp := protocol.HandshakeResponse{
			ProtocolVersion: protocol.Version{Major: 1, Minor: 0},
			AgentVersion:    "v0.1.0",
			SessionID:       "sess-100",
			Epoch:           1,
			MaxPayloadBytes: protocol.DefaultMaxPayloadBytes,
		}
		respBytes, _ := protocol.EncodePayload(hsResp)
		_ = serverFramed.Send(respBytes)

		reqBytes, err := serverFramed.Receive()
		if err != nil {
			return
		}
		req, err := protocol.DecodePayload[protocol.RequestEnvelope](reqBytes)
		if err != nil {
			return
		}

		resp := protocol.ResponseEnvelope{
			RequestID: req.RequestID,
			Status:    protocol.StatusOK,
			Payload:   json.RawMessage(`{"result":"pong"}`),
			Timing: protocol.TimingData{
				ExecutionMs: 5,
			},
		}
		resBytes, _ := protocol.EncodePayload(resp)
		_ = serverFramed.Send(resBytes)
	}()

	client := NewClient(clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hsResp, err := client.Handshake(ctx, &protocol.HandshakeRequest{
		ProtocolVersion: protocol.Version{Major: 1, Minor: 0},
		SDKVersion:      "v0.1.0",
		ClientPID:       1234,
		Nonce:           "nonce-1",
	})
	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	if hsResp.SessionID != "sess-100" || hsResp.Epoch != 1 {
		t.Fatalf("unexpected handshake response: %+v", hsResp)
	}

	resp, err := client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-1",
		Op:        "nx.ping",
	})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if resp.RequestID != "req-1" || resp.Status != protocol.StatusOK {
		t.Fatalf("unexpected call response: %+v", resp)
	}
}

func TestHandshakeRejectsMajorVersionMismatch(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		hsBytes, err := serverFramed.Receive()
		if err != nil {
			return
		}
		_, _ = protocol.DecodePayload[protocol.HandshakeRequest](hsBytes)

		hsResp := protocol.HandshakeResponse{
			ProtocolVersion: protocol.Version{Major: 2, Minor: 0},
			AgentVersion:    "v2.0.0",
			SessionID:       "sess-200",
			Epoch:           2,
		}
		respBytes, _ := protocol.EncodePayload(hsResp)
		_ = serverFramed.Send(respBytes)
	}()

	client := NewClient(clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.Handshake(ctx, &protocol.HandshakeRequest{
		ProtocolVersion: protocol.Version{Major: 1, Minor: 0},
		SDKVersion:      "v0.1.0",
		ClientPID:       1234,
		Nonce:           "nonce-v2",
	})
	if err == nil {
		t.Fatalf("expected error on major version mismatch")
	}
	if !errors.Is(err, ErrHandshakeFailed) {
		t.Fatalf("expected ErrHandshakeFailed, got %v", err)
	}
}

func TestCallContextCancellationQuarantinesConnection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		_, _ = serverFramed.Receive()
		// Intentionally never respond. The client must close the connection when
		// the caller deadline expires because the remote execution outcome is
		// ambiguous after the request has been sent.
	}()

	client := NewClient(clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-timeout",
		Op:        "slow.op",
	})
	if err == nil {
		t.Fatalf("expected timeout/quarantine error, got nil")
	}
	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("expected ErrOutcomeUnknown, got %v", err)
	}

	_, err = client.Call(context.Background(), &protocol.RequestEnvelope{
		RequestID: "req-after-timeout",
		Op:        "nx.ping",
	})
	if err == nil || !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("expected quarantined connection to reject reuse, got %v", err)
	}
}

func TestLateResponseCannotSatisfyNextRequest(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	received := make(chan string, 2)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)

		reqBytes, err := serverFramed.Receive()
		if err != nil {
			return
		}
		req, err := protocol.DecodePayload[protocol.RequestEnvelope](reqBytes)
		if err != nil {
			return
		}
		received <- req.RequestID

		// Deliver A only after the caller has timed out. A correct client has
		// already quarantined and closed the stream, so this response can never
		// be consumed by a later request B.
		time.Sleep(80 * time.Millisecond)
		late := protocol.ResponseEnvelope{RequestID: req.RequestID, Status: protocol.StatusOK}
		lateBytes, _ := protocol.EncodePayload(late)
		_ = serverFramed.Send(lateBytes)

		// If B is incorrectly sent on the same connection, record it.
		nextBytes, err := serverFramed.Receive()
		if err != nil {
			return
		}
		next, err := protocol.DecodePayload[protocol.RequestEnvelope](nextBytes)
		if err == nil {
			received <- next.RequestID
		}
	}()

	client := NewClient(clientConn)
	ctxA, cancelA := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancelA()

	_, err := client.Call(ctxA, &protocol.RequestEnvelope{
		RequestID: "req-A",
		Op:        "feature.create_block",
	})
	if err == nil || !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("expected request A to quarantine with unknown outcome, got %v", err)
	}

	_, err = client.Call(context.Background(), &protocol.RequestEnvelope{
		RequestID: "req-B",
		Op:        "part.save",
	})
	if err == nil || !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("expected request B to be rejected before send, got %v", err)
	}

	select {
	case id := <-received:
		if id != "req-A" {
			t.Fatalf("expected server to receive request A first, got %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive request A")
	}

	select {
	case id := <-received:
		t.Fatalf("request %q reached server after connection quarantine", id)
	case <-serverDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("server did not observe connection closure after quarantine")
	}
}

func TestUnexpectedResponseRequestIDQuarantinesConnection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		reqBytes, err := serverFramed.Receive()
		if err != nil {
			return
		}
		_, err = protocol.DecodePayload[protocol.RequestEnvelope](reqBytes)
		if err != nil {
			return
		}

		wrong := protocol.ResponseEnvelope{
			RequestID: "different-request-id",
			Status:    protocol.StatusOK,
		}
		wrongBytes, _ := protocol.EncodePayload(wrong)
		_ = serverFramed.Send(wrongBytes)
	}()

	client := NewClient(clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "expected-request-id",
		Op:        "nx.ping",
	})
	if err == nil {
		t.Fatal("expected protocol violation")
	}
	if !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation, got %v", err)
	}

	_, err = client.Call(context.Background(), &protocol.RequestEnvelope{
		RequestID: "req-after-protocol-violation",
		Op:        "nx.ping",
	})
	if err == nil || !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("expected protocol-violating connection to remain quarantined, got %v", err)
	}
}

func TestConcurrentCallsRouteResponsesByRequestID(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		requests := make([]*protocol.RequestEnvelope, 0, 2)
		for len(requests) < 2 {
			b, err := serverFramed.Receive()
			if err != nil {
				return
			}
			req, err := protocol.DecodePayload[protocol.RequestEnvelope](b)
			if err != nil {
				return
			}
			requests = append(requests, req)
		}

		// Respond in reverse receive order. The client must route by RequestID,
		// not by whichever Call happens to be waiting first.
		for i := len(requests) - 1; i >= 0; i-- {
			resp := protocol.ResponseEnvelope{
				RequestID: requests[i].RequestID,
				Status:    protocol.StatusOK,
				Payload:   json.RawMessage(`{"ok":true}`),
			}
			b, _ := protocol.EncodePayload(resp)
			if err := serverFramed.Send(b); err != nil {
				return
			}
		}
	}()

	client := NewClient(clientConn)
	type result struct {
		id  string
		err error
	}
	results := make(chan result, 2)
	for _, id := range []string{"req-1", "req-2"} {
		id := id
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			resp, err := client.Call(ctx, &protocol.RequestEnvelope{RequestID: id, Op: "nx.ping"})
			if err != nil {
				results <- result{err: err}
				return
			}
			results <- result{id: resp.RequestID}
		}()
	}

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case res := <-results:
			if res.err != nil {
				t.Fatalf("concurrent call failed: %v", res.err)
			}
			seen[res.id] = true
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for concurrent call")
		}
	}
	if !seen["req-1"] || !seen["req-2"] {
		t.Fatalf("responses were not routed by RequestID: %+v", seen)
	}
}
