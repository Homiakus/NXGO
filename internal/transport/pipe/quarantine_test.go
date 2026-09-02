package pipe

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestQuarantineHookFiresOnceWithTerminalCause(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		_, _ = serverFramed.Receive()
		// No response: force the caller deadline to make the post-send outcome
		// ambiguous and quarantine the connection.
	}()

	client := NewClient(clientConn)
	hookCalled := make(chan error, 2)
	var hookCount atomic.Int32
	client.SetQuarantineHook(func(err error) {
		hookCount.Add(1)
		hookCalled <- err
	})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-hook-timeout",
		Op:        "feature.create_block",
	})
	if err == nil || !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("expected ErrOutcomeUnknown, got %v", err)
	}

	select {
	case hookErr := <-hookCalled:
		if !errors.Is(hookErr, ErrOutcomeUnknown) {
			t.Fatalf("hook received wrong terminal cause: %v", hookErr)
		}
	case <-time.After(time.Second):
		t.Fatal("quarantine hook was not invoked")
	}

	// Reusing a quarantined client must not fire the lifecycle hook again.
	_, _ = client.Call(context.Background(), &protocol.RequestEnvelope{
		RequestID: "req-hook-reuse",
		Op:        "part.save",
	})
	time.Sleep(20 * time.Millisecond)
	if got := hookCount.Load(); got != 1 {
		t.Fatalf("expected quarantine hook exactly once, got %d", got)
	}
}

func TestHookInstalledAfterQuarantineReceivesExistingCause(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		serverFramed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		_, _ = serverFramed.Receive()
	}()

	client := NewClient(clientConn)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-before-hook",
		Op:        "slow.op",
	})
	if err == nil || !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("expected ErrOutcomeUnknown, got %v", err)
	}

	hookCalled := make(chan error, 1)
	client.SetQuarantineHook(func(hookErr error) {
		hookCalled <- hookErr
	})

	select {
	case hookErr := <-hookCalled:
		if !errors.Is(hookErr, ErrOutcomeUnknown) {
			t.Fatalf("late hook received wrong cause: %v", hookErr)
		}
	case <-time.After(time.Second):
		t.Fatal("late-installed hook did not receive existing quarantine cause")
	}
}
