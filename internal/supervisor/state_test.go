package supervisor

import (
	"errors"
	"testing"
)

func TestStateMachine_InitialState(t *testing.T) {
	sm := NewStateMachine()
	if sm.Current() != StateStarting {
		t.Fatalf("expected initial state StateStarting, got %s", sm.Current())
	}
	if sm.IsUsable() {
		t.Fatalf("expected initial state not to be usable")
	}
	code, err := sm.QuarantineInfo()
	if code != QuarantineNone || err != nil {
		t.Fatalf("expected no quarantine info initially, got code=%s, err=%v", code, err)
	}
}

func TestStateMachine_ValidTransitions(t *testing.T) {
	sm := NewStateMachine()

	// Starting -> Ready
	if err := sm.Transition(StateReady, nil); err != nil {
		t.Fatalf("failed starting -> ready: %v", err)
	}
	if sm.Current() != StateReady || !sm.IsUsable() {
		t.Fatalf("expected StateReady and usable, got %s", sm.Current())
	}

	// Ready -> Busy
	if err := sm.Transition(StateBusy, nil); err != nil {
		t.Fatalf("failed ready -> busy: %v", err)
	}
	if sm.Current() != StateBusy || !sm.IsUsable() {
		t.Fatalf("expected StateBusy and usable, got %s", sm.Current())
	}

	// Busy -> Ready
	if err := sm.Transition(StateReady, nil); err != nil {
		t.Fatalf("failed busy -> ready: %v", err)
	}

	// Ready -> Draining
	if err := sm.Transition(StateDraining, nil); err != nil {
		t.Fatalf("failed ready -> draining: %v", err)
	}
	if sm.IsUsable() {
		t.Fatalf("expected draining state not to be usable")
	}

	// Draining -> Stopped
	if err := sm.Transition(StateStopped, nil); err != nil {
		t.Fatalf("failed draining -> stopped: %v", err)
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	sm := NewStateMachine()

	// Starting -> Busy is invalid
	err := sm.Transition(StateBusy, nil)
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("expected ErrInvalidStateTransition for starting -> busy, got %v", err)
	}

	// Starting -> Stopped is valid
	if err := sm.Transition(StateStopped, nil); err != nil {
		t.Fatalf("expected starting -> stopped to be valid, got %v", err)
	}

	// Stopped -> Ready is invalid (Stopped is terminal)
	err = sm.Transition(StateReady, nil)
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("expected ErrInvalidStateTransition from stopped, got %v", err)
	}
}

func TestStateMachine_Quarantine(t *testing.T) {
	sm := NewStateMachine()
	_ = sm.Transition(StateReady, nil)
	_ = sm.Transition(StateBusy, nil)

	qErr := errors.New("timeout on extrude mutation")
	sm.Quarantine(QuarantineTimeoutAmbiguity, qErr)

	if sm.Current() != StatePoisoned {
		t.Fatalf("expected StatePoisoned, got %s", sm.Current())
	}
	if sm.IsUsable() {
		t.Fatalf("expected poisoned worker not to be usable")
	}

	code, err := sm.QuarantineInfo()
	if code != QuarantineTimeoutAmbiguity || !errors.Is(err, qErr) {
		t.Fatalf("expected quarantine code %s and error %v, got code=%s, err=%v",
			QuarantineTimeoutAmbiguity, qErr, code, err)
	}

	// Poisoned -> Stopped is valid
	if err := sm.Transition(StateStopped, nil); err != nil {
		t.Fatalf("expected poisoned -> stopped to be valid, got %v", err)
	}
}

func TestStateMachine_ListenersAndHistory(t *testing.T) {
	sm := NewStateMachine()
	var transitions []StateTransitionRecord
	sm.AddListener(func(record StateTransitionRecord) {
		transitions = append(transitions, record)
	})

	_ = sm.Transition(StateReady, nil)
	_ = sm.Transition(StateBusy, nil)
	sm.Quarantine(QuarantineSyslogCritical, errors.New("access violation in libpart"))

	history := sm.History()
	if len(history) != 4 { // init + ready + busy + quarantine
		t.Fatalf("expected 4 history entries, got %d", len(history))
	}
	if len(transitions) != 3 {
		t.Fatalf("expected 3 listener events, got %d", len(transitions))
	}
	if transitions[2].Code != QuarantineSyslogCritical {
		t.Fatalf("expected QuarantineSyslogCritical in last transition, got %s", transitions[2].Code)
	}
}
