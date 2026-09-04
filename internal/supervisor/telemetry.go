package supervisor

import (
	"sync"
	"time"
)

// WorkerTelemetry captures cumulative and high-watermark metrics for a worker process.
type WorkerTelemetry struct {
	CallCount         int64         `json:"call_count"`
	ErrorCount        int64         `json:"error_count"`
	OpenHandles       int64         `json:"open_handles"`
	PeakHandles       int64         `json:"peak_handles"`
	MemoryBytes       int64         `json:"memory_bytes"`
	PeakMemoryBytes   int64         `json:"peak_memory_bytes"`
	TotalExecutionDur time.Duration `json:"total_execution_dur"`
	StartedAt         time.Time     `json:"started_at"`
	LastActiveAt      time.Time     `json:"last_active_at"`
}

// RecyclePolicy defines operational bounds before a worker is marked for graceful draining and replacement.
type RecyclePolicy struct {
	MaxCallsPerWorker int64         `json:"max_calls_per_worker"`
	MaxMemoryBytes    int64         `json:"max_memory_bytes"`
	MaxLifetime       time.Duration `json:"max_lifetime"`
	MaxErrors         int64         `json:"max_errors"`
}

// TelemetryTracker aggregates real-time resource telemetry across workers.
type TelemetryTracker struct {
	mu     sync.RWMutex
	data   map[string]*WorkerTelemetry
	policy RecyclePolicy
}

// NewTelemetryTracker creates an active tracker.
func NewTelemetryTracker(policy RecyclePolicy) *TelemetryTracker {
	return &TelemetryTracker{
		data:   make(map[string]*WorkerTelemetry),
		policy: policy,
	}
}

// RecordCall registers a completed call duration, error status, and optional memory/handle snapshot.
func (tt *TelemetryTracker) RecordCall(workerID string, dur time.Duration, isError bool, memoryBytes int64, openHandles int64) {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	t, exists := tt.data[workerID]
	if !exists {
		t = &WorkerTelemetry{
			StartedAt:    time.Now().UTC(),
			LastActiveAt: time.Now().UTC(),
		}
		tt.data[workerID] = t
	}

	t.CallCount++
	if isError {
		t.ErrorCount++
	}
	t.TotalExecutionDur += dur
	t.LastActiveAt = time.Now().UTC()

	if memoryBytes > 0 {
		t.MemoryBytes = memoryBytes
		if memoryBytes > t.PeakMemoryBytes {
			t.PeakMemoryBytes = memoryBytes
		}
	}

	if openHandles >= 0 {
		t.OpenHandles = openHandles
		if openHandles > t.PeakHandles {
			t.PeakHandles = openHandles
		}
	}
}

// Telemetry returns a copy of telemetry data for a worker.
func (tt *TelemetryTracker) Telemetry(workerID string) (WorkerTelemetry, bool) {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	t, exists := tt.data[workerID]
	if !exists {
		return WorkerTelemetry{}, false
	}
	return *t, true
}

// ShouldDrain evaluates if a worker has exceeded configured recycling policy limits.
func (tt *TelemetryTracker) ShouldDrain(workerID string) (bool, string) {
	tt.mu.RLock()
	defer tt.mu.RUnlock()

	t, exists := tt.data[workerID]
	if !exists {
		return false, ""
	}

	if tt.policy.MaxCallsPerWorker > 0 && t.CallCount >= tt.policy.MaxCallsPerWorker {
		return true, "max call count exceeded"
	}
	if tt.policy.MaxMemoryBytes > 0 && t.MemoryBytes >= tt.policy.MaxMemoryBytes {
		return true, "memory threshold exceeded"
	}
	if tt.policy.MaxErrors > 0 && t.ErrorCount >= tt.policy.MaxErrors {
		return true, "max error count exceeded"
	}
	if tt.policy.MaxLifetime > 0 && time.Since(t.StartedAt) >= tt.policy.MaxLifetime {
		return true, "max worker lifetime reached"
	}

	return false, ""
}
