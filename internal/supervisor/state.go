package supervisor

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrInvalidStateTransition = errors.New("invalid worker state transition")
	ErrWorkerQuarantined      = errors.New("worker is quarantined and cannot accept operations")
	ErrWorkerNotReady         = errors.New("worker is not in a ready state")
)

// WorkerState represents the explicit operational lifecycle state of an NX worker process.
type WorkerState string

const (
	// StateStarting indicates the worker process is launching, establishing IPC named pipe, or completing handshake.
	StateStarting WorkerState = "starting"
	// StateReady indicates the worker is healthy, idle, and ready to accept operations.
	StateReady WorkerState = "ready"
	// StateBusy indicates the worker is currently executing a CAD operation or transaction.
	StateBusy WorkerState = "busy"
	// StateDraining indicates the worker will finish in-flight work and then be recycled/stopped.
	StateDraining WorkerState = "draining"
	// StateDirty indicates an operation failed without completing rollback, but process is still alive.
	StateDirty WorkerState = "dirty"
	// StatePoisoned indicates the worker experienced an unrecoverable or ambiguous failure (e.g. timeout on mutation).
	StatePoisoned WorkerState = "poisoned"
	// StateLost indicates the worker process crashed, terminated unexpectedly, or pipe broke.
	StateLost WorkerState = "lost"
	// StateStopped indicates the worker has been cleanly or intentionally terminated.
	StateStopped WorkerState = "stopped"
)

// QuarantineReasonCode classifies why a worker was placed into quarantine (StatePoisoned).
type QuarantineReasonCode string

const (
	QuarantineNone              QuarantineReasonCode = ""
	QuarantineTimeoutAmbiguity  QuarantineReasonCode = "timeout_ambiguity"
	QuarantineSessionLoss       QuarantineReasonCode = "session_loss"
	QuarantineSyslogCritical    QuarantineReasonCode = "syslog_critical"
	QuarantineMemoryLimit       QuarantineReasonCode = "memory_limit"
	QuarantineProtocolViolation QuarantineReasonCode = "protocol_violation"
	QuarantineManual            QuarantineReasonCode = "manual"
)

// ValidStateTransition returns true if moving from `from` to `to` is an allowed transition.
func ValidStateTransition(from, to WorkerState) bool {
	if from == to {
		return true
	}
	switch from {
	case StateStarting:
		return to == StateReady || to == StatePoisoned || to == StateLost || to == StateStopped
	case StateReady:
		return to == StateBusy || to == StateDraining || to == StateDirty || to == StatePoisoned || to == StateLost || to == StateStopped
	case StateBusy:
		return to == StateReady || to == StateDraining || to == StateDirty || to == StatePoisoned || to == StateLost || to == StateStopped
	case StateDraining:
		return to == StateStopped || to == StatePoisoned || to == StateLost
	case StateDirty:
		return to == StateReady || to == StatePoisoned || to == StateLost || to == StateStopped
	case StatePoisoned:
		return to == StateStopped
	case StateLost:
		return to == StateStopped
	case StateStopped:
		return false
	default:
		return false
	}
}

// StateMachine manages the lifecycle state transitions and quarantine metadata with thread safety.
type StateMachine struct {
	mu             sync.RWMutex
	current        WorkerState
	quarantineCode QuarantineReasonCode
	quarantineErr  error
	history        []StateTransitionRecord
	listeners      []func(record StateTransitionRecord)
}

// StateTransitionRecord captures an audit record of a lifecycle state change.
type StateTransitionRecord struct {
	From   WorkerState          `json:"from"`
	To     WorkerState          `json:"to"`
	Code   QuarantineReasonCode `json:"code,omitempty"`
	Reason string               `json:"reason,omitempty"`
}

// NewStateMachine initializes a state machine starting in StateStarting.
func NewStateMachine() *StateMachine {
	return &StateMachine{
		current: StateStarting,
		history: []StateTransitionRecord{
			{From: "", To: StateStarting, Reason: "initialization"},
		},
	}
}

// Current returns the current WorkerState.
func (sm *StateMachine) Current() WorkerState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current
}

// QuarantineInfo returns the active quarantine reason code and error, if any.
func (sm *StateMachine) QuarantineInfo() (QuarantineReasonCode, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.quarantineCode, sm.quarantineErr
}

// IsUsable returns true if the worker can accept new commands (StateReady or StateBusy).
func (sm *StateMachine) IsUsable() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current == StateReady || sm.current == StateBusy
}

// Transition performs a validated state transition.
func (sm *StateMachine) Transition(to WorkerState, reason error) error {
	sm.mu.Lock()
	if !ValidStateTransition(sm.current, to) {
		current := sm.current
		sm.mu.Unlock()
		return fmt.Errorf("%w: cannot transition from %q to %q", ErrInvalidStateTransition, current, to)
	}

	record := StateTransitionRecord{
		From:   sm.current,
		To:     to,
		Reason: errToString(reason),
	}
	sm.current = to
	sm.history = append(sm.history, record)
	listeners := make([]func(record StateTransitionRecord), len(sm.listeners))
	copy(listeners, sm.listeners)
	sm.mu.Unlock()

	for _, l := range listeners {
		l(record)
	}
	return nil
}

// Quarantine unconditionally transitions the state machine to StatePoisoned with a quarantine code.
func (sm *StateMachine) Quarantine(code QuarantineReasonCode, reason error) {
	sm.mu.Lock()
	from := sm.current
	sm.current = StatePoisoned
	sm.quarantineCode = code
	sm.quarantineErr = reason

	record := StateTransitionRecord{
		From:   from,
		To:     StatePoisoned,
		Code:   code,
		Reason: errToString(reason),
	}
	sm.history = append(sm.history, record)
	listeners := make([]func(record StateTransitionRecord), len(sm.listeners))
	copy(listeners, sm.listeners)
	sm.mu.Unlock()

	for _, l := range listeners {
		l(record)
	}
}

// AddListener registers a callback invoked on each state transition.
func (sm *StateMachine) AddListener(listener func(record StateTransitionRecord)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.listeners = append(sm.listeners, listener)
}

// History returns a snapshot of all state transitions recorded by this machine.
func (sm *StateMachine) History() []StateTransitionRecord {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	copied := make([]StateTransitionRecord, len(sm.history))
	copy(copied, sm.history)
	return copied
}

func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
