package pipe

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestFramedClientServerRoundtrip(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// Server goroutine
	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		// 1. Handshake
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

		// 2. Request / Response
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

	// 1. Handshake
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

	// 2. Call
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

		// Server answers with major version 2
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
}

func TestCallContextCancellation(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// Server does not respond
	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		_, _ = serverFramed.Receive()
		// hang
	}()

	client := NewClient(clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-timeout",
		Op:        "slow.op",
	})
	if err == nil {
		t.Fatalf("expected context deadline error, got nil")
	}
}
