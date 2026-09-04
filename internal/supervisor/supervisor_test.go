package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestSupervisor_QuarantineEnforcement(t *testing.T) {
	sup := NewSupervisor(WorkerConfig{})
	wp := &WorkerProcess{
		Config: WorkerConfig{PipeName: "test-wp"},
		sm:     NewStateMachine(),
	}
	_ = wp.Transition(StateReady, nil)
	if err := sup.Register(wp); err != nil {
		t.Fatalf("failed to register worker: %v", err)
	}

	// Manually quarantine worker
	wp.Quarantine(QuarantineTimeoutAmbiguity, errors.New("timeout on extrude"))

	// Next call must fail closed with ErrWorkerQuarantined
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := sup.InterceptCall(ctx, wp, &protocol.RequestEnvelope{Op: "test"})
	if !errors.Is(err, ErrWorkerQuarantined) {
		t.Fatalf("expected ErrWorkerQuarantined, got %v", err)
	}
}

func TestSupervisor_OutcomeUnknownInterception(t *testing.T) {
	sup := NewSupervisor(WorkerConfig{})
	wp := &WorkerProcess{
		Config: WorkerConfig{PipeName: "test-wp-intercept"},
		sm:     NewStateMachine(),
	}
	_ = wp.Transition(StateReady, nil)
	_ = sup.Register(wp)

	// InterceptCall when Client is nil should quarantine for protocol violation
	ctx := context.Background()
	_, err := sup.InterceptCall(ctx, wp, &protocol.RequestEnvelope{Op: "geom.extrude"})
	if err == nil {
		t.Fatalf("expected error on nil client")
	}

	if wp.State() != StatePoisoned {
		t.Fatalf("expected worker to be quarantined to StatePoisoned, got %s", wp.State())
	}
	if wp.QuarantineCode() != QuarantineProtocolViolation {
		t.Fatalf("expected QuarantineProtocolViolation, got %s", wp.QuarantineCode())
	}
}
