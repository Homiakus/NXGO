package pipe

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestMalformedFrameQuarantinesAsProtocolViolation(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		_, err := serverFramed.Receive()
		if err != nil {
			return
		}

		// Announce a response larger than the negotiated/default maximum without
		// sending a payload. The client must classify this as protocol-fatal,
		// not as an ordinary retryable transport timeout.
		header := make([]byte, protocol.FrameHeaderSize)
		binary.LittleEndian.PutUint32(header, uint32(protocol.DefaultMaxPayloadBytes+1))
		_, _ = serverConn.Write(header)
	}()

	client := NewClient(clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-malformed-frame",
		Op:        "nx.ping",
	})
	if err == nil || !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation, got %v", err)
	}
}

func TestConnectionLossCompletesAllPendingCalls(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	const n = 8
	serverReceivedAll := make(chan struct{})
	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		for i := 0; i < n; i++ {
			if _, err := serverFramed.Receive(); err != nil {
				_ = serverConn.Close()
				return
			}
		}
		close(serverReceivedAll)
		_ = serverConn.Close()
	}()

	client := NewClient(clientConn)
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := client.Call(ctx, &protocol.RequestEnvelope{
				RequestID: "req-pending-" + string(rune('A'+i)),
				Op:        "nx.ping",
			})
			results <- err
		}()
	}

	select {
	case <-serverReceivedAll:
	case <-time.After(time.Second):
		t.Fatal("server did not receive all pending calls")
	}

	for i := 0; i < n; i++ {
		select {
		case err := <-results:
			if err == nil || !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatalf("pending call did not receive terminal transport error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("pending call remained blocked after connection loss")
		}
	}
}
