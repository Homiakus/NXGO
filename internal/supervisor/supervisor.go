package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

var (
	ErrSupervisorStopped = errors.New("supervisor has been stopped")
	ErrWorkerNotFound    = errors.New("worker not tracked by supervisor")
)

// Supervisor oversees worker lifecycles, health transitions, quarantine enforcement, and automatic recycling.
type Supervisor struct {
	mu          sync.RWMutex
	workers     map[string]*WorkerProcess
	stopped     bool
	defaultCfg  WorkerConfig
	onRecycle   []func(oldWorker, newWorker *WorkerProcess)
}

// NewSupervisor creates an active supervisor.
func NewSupervisor(defaultCfg WorkerConfig) *Supervisor {
	return &Supervisor{
		workers:    make(map[string]*WorkerProcess),
		defaultCfg: defaultCfg,
	}
}

// Register adds an existing WorkerProcess under supervisor management.
func (s *Supervisor) Register(wp *WorkerProcess) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return ErrSupervisorStopped
	}
	if wp == nil {
		return errors.New("nil worker process")
	}
	id := wp.Config.PipeName
	if wp.Manifest != nil && wp.Manifest.ID != "" {
		id = wp.Manifest.ID
	}
	s.workers[id] = wp

	// Wire state transition hooks
	if wp.sm != nil {
		wp.sm.AddListener(func(record StateTransitionRecord) {
			if record.To == StatePoisoned || record.To == StateLost {
				// Worker became unusable
			}
		})
	}

	return nil
}

// InterceptCall executes a transport call against a worker, automatically inspecting for ErrOutcomeUnknown
// or protocol errors and quarantining the worker immediately to avoid state divergence.
func (s *Supervisor) InterceptCall(ctx context.Context, wp *WorkerProcess, req *protocol.RequestEnvelope) (*protocol.ResponseEnvelope, error) {
	if wp == nil {
		return nil, errors.New("nil worker process")
	}

	if !wp.StateMachine().IsUsable() {
		code, err := wp.StateMachine().QuarantineInfo()
		return nil, fmt.Errorf("%w: state=%s, code=%s, reason=%v", ErrWorkerQuarantined, wp.State(), code, err)
	}

	if err := wp.Transition(StateBusy, nil); err != nil {
		return nil, fmt.Errorf("worker not ready for execution: %w", err)
	}
	defer func() {
		if wp.State() == StateBusy {
			_ = wp.Transition(StateReady, nil)
		}
	}()

	if wp.Client == nil {
		wp.Quarantine(QuarantineProtocolViolation, errors.New("worker client is nil"))
		return nil, ErrWorkerQuarantined
	}

	resp, err := wp.Client.Call(ctx, req)
	if err != nil {
		if errors.Is(err, pipe.ErrOutcomeUnknown) {
			wp.Quarantine(QuarantineTimeoutAmbiguity, err)
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			// Mutation call timeout/cancellation
			wp.Quarantine(QuarantineTimeoutAmbiguity, fmt.Errorf("call cancelled/timed out: %w", err))
		}
		return nil, err
	}

	return resp, nil
}

// SafeRecycle safely terminates a poisoned, dirty, or draining worker and starts a fresh replacement.
func (s *Supervisor) SafeRecycle(ctx context.Context, wp *WorkerProcess) (*WorkerProcess, error) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil, ErrSupervisorStopped
	}
	s.mu.Unlock()

	if wp == nil {
		return nil, errors.New("nil worker to recycle")
	}

	// 1. Drain / Kill old worker safely
	_ = wp.Kill()

	// 2. Derive config for new replacement worker
	cfg := wp.Config
	if cfg.PipeName != "" {
		// Allocate fresh unique pipe name to avoid collision
		cfg.PipeName = fmt.Sprintf("%s-r", cfg.PipeName)
	}

	// 3. Start replacement worker
	newWorker, err := StartWorker(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to launch replacement worker during recycle: %w", err)
	}

	// 4. Update supervisor registry
	s.mu.Lock()
	oldID := wp.Config.PipeName
	if wp.Manifest != nil && wp.Manifest.ID != "" {
		oldID = wp.Manifest.ID
	}
	delete(s.workers, oldID)

	newID := newWorker.Config.PipeName
	if newWorker.Manifest != nil && newWorker.Manifest.ID != "" {
		newID = newWorker.Manifest.ID
	}
	s.workers[newID] = newWorker

	callbacks := make([]func(oldWorker, newWorker *WorkerProcess), len(s.onRecycle))
	copy(callbacks, s.onRecycle)
	s.mu.Unlock()

	for _, cb := range callbacks {
		cb(wp, newWorker)
	}

	return newWorker, nil
}

// OnRecycle registers a callback invoked when a worker is recycled.
func (s *Supervisor) OnRecycle(callback func(oldWorker, newWorker *WorkerProcess)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onRecycle = append(s.onRecycle, callback)
}

// StopAll stops all managed workers and halts the supervisor.
func (s *Supervisor) StopAll(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	workers := make([]*WorkerProcess, 0, len(s.workers))
	for _, wp := range s.workers {
		workers = append(workers, wp)
	}
	s.workers = make(map[string]*WorkerProcess)
	s.mu.Unlock()

	var firstErr error
	for _, wp := range workers {
		if err := wp.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
