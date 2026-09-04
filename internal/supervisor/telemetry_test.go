package supervisor_test

import (
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/supervisor"
)

func TestTelemetryTracker_RecordAndDrainPolicy(t *testing.T) {
	policy := supervisor.RecyclePolicy{
		MaxCallsPerWorker: 5,
		MaxMemoryBytes:    100 * 1024 * 1024, // 100MB
		MaxErrors:         2,
		MaxLifetime:       time.Minute,
	}

	tracker := supervisor.NewTelemetryTracker(policy)
	workerID := "worker-test-1"

	// Record calls under threshold
	for i := 0; i < 4; i++ {
		tracker.RecordCall(workerID, 10*time.Millisecond, false, 50*1024*1024, 10)
	}

	shouldDrain, reason := tracker.ShouldDrain(workerID)
	if shouldDrain {
		t.Fatalf("expected worker not to drain before threshold, got reason: %s", reason)
	}

	tel, exists := tracker.Telemetry(workerID)
	if !exists || tel.CallCount != 4 {
		t.Fatalf("expected 4 calls recorded, got %+v", tel)
	}
	if tel.PeakMemoryBytes != 50*1024*1024 {
		t.Fatalf("expected peak memory 50MB, got %d", tel.PeakMemoryBytes)
	}

	// 5th call hits threshold
	tracker.RecordCall(workerID, 10*time.Millisecond, false, 50*1024*1024, 10)
	shouldDrain, reason = tracker.ShouldDrain(workerID)
	if !shouldDrain || reason != "max call count exceeded" {
		t.Fatalf("expected drain due to max call count, got drain=%v, reason=%s", shouldDrain, reason)
	}
}

func TestTelemetryTracker_MemoryAndErrorDrain(t *testing.T) {
	policy := supervisor.RecyclePolicy{
		MaxMemoryBytes: 200 * 1024 * 1024,
		MaxErrors:      2,
	}

	tracker := supervisor.NewTelemetryTracker(policy)
	workerID := "worker-test-mem"

	tracker.RecordCall(workerID, 10*time.Millisecond, true, 100*1024*1024, 5)
	shouldDrain, _ := tracker.ShouldDrain(workerID)
	if shouldDrain {
		t.Fatalf("should not drain after 1 error")
	}

	tracker.RecordCall(workerID, 10*time.Millisecond, true, 150*1024*1024, 5)
	shouldDrain, reason := tracker.ShouldDrain(workerID)
	if !shouldDrain || reason != "max error count exceeded" {
		t.Fatalf("expected drain due to max error count, got drain=%v, reason=%s", shouldDrain, reason)
	}
}
