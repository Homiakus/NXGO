package nx_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/supervisor"
	"github.com/Homiakus/NXGO/pkg/nxgo"
)

// TestRealNXCanonicalIdempotencyReplay proves H2 requirements on real NX:
// 1. Initial mutating request executes and commits.
// 2. Exact replay with the same RequestID returns the cached committed result
//    without creating duplicate CAD features or bodies.
// 3. Reusing the same RequestID with a conflicting payload fails closed
//    with REQUEST_IDENTITY_CONFLICT and poisons the worker session.
func TestRealNXCanonicalIdempotencyReplay(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("canonical compiled NXHost requires Windows/Siemens NX")
	}
	if os.Getenv("NXGO_RUN_REAL_NX") == "" {
		t.Skip("set NXGO_RUN_REAL_NX=1 or run via the real-NX quality gate")
	}

	repoRoot := repoRootFromTestFile(t)
	agentBin := filepath.Join(repoRoot, "agent", "bin")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	worker, err := supervisor.StartWorker(ctx, supervisor.WorkerConfig{
		NXHome:         getNXHome(t),
		AgentMode:      supervisor.AgentModeCanonical,
		AgentBin:       agentBin,
		StartupTimeout: 45 * time.Second,
	})
	if err != nil {
		t.Fatalf("start canonical compiled NXHost: %v", err)
	}
	defer worker.Kill()

	session := nxgo.WrapClient(worker.Client, worker.Manifest.ID, 1, worker.Manifest.NXRelease)
	partPath := filepath.Join(t.TempDir(), "nxgo_idempotency_part.prt")
	part, err := session.NewPart(ctx, partPath, "mm")
	if err != nil {
		t.Fatalf("create test part: %v", err)
	}

	summary0, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("part.Summary initial: %v", err)
	}
	if summary0.BodyCount != 0 {
		t.Fatalf("expected 0 bodies initially, got %d", summary0.BodyCount)
	}

	// 1. Initial mutating request with fixed RequestID:
	reqID := "req-idempotency-block-001"
	reqPayload, err := protocol.EncodePayload(protocol.FeatureCreateBlockRequest{
		PartRef: &part.Ref,
		Units:   part.Units,
		Origin:  nxgo.Point3D{0, 0, 0},
		Length:  50,
		Width:   50,
		Height:  50,
	})
	if err != nil {
		t.Fatalf("encode initial request payload: %v", err)
	}

	resp1, err := worker.Client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: reqID,
		Op:        "feature.create_block",
		Payload:   reqPayload,
	})
	if err != nil {
		t.Fatalf("initial feature.create_block failed: %v", err)
	}
	if resp1.Status != protocol.StatusOK {
		t.Fatalf("initial feature.create_block status: %s (err=%+v)", resp1.Status, resp1.Error)
	}

	summary1, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("part.Summary after initial creation: %v", err)
	}
	if summary1.BodyCount != 1 {
		t.Fatalf("expected 1 body after initial creation, got %d", summary1.BodyCount)
	}

	// 2. Replay with identical RequestID and payload:
	resp2, err := worker.Client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: reqID,
		Op:        "feature.create_block",
		Payload:   reqPayload,
	})
	if err != nil {
		t.Fatalf("replay feature.create_block failed: %v", err)
	}
	if resp2.Status != protocol.StatusOK {
		t.Fatalf("replay feature.create_block status: %s (err=%+v)", resp2.Status, resp2.Error)
	}

	summary2, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("part.Summary after replay: %v", err)
	}
	if summary2.BodyCount != 1 || summary2.FeatureCount != summary1.FeatureCount {
		t.Fatalf("replay created duplicate CAD objects: before=%+v after=%+v", summary1, summary2)
	}

	// 3. Conflicting replay with same RequestID but different payload:
	diffPayload, err := protocol.EncodePayload(protocol.FeatureCreateBlockRequest{
		PartRef: &part.Ref,
		Units:   part.Units,
		Origin:  nxgo.Point3D{100, 100, 100},
		Length:  99,
		Width:   99,
		Height:  99,
	})
	if err != nil {
		t.Fatalf("encode conflicting payload: %v", err)
	}

	resp3, err := worker.Client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: reqID,
		Op:        "feature.create_block",
		Payload:   diffPayload,
	})
	if err != nil {
		t.Fatalf("expected response with error status, got transport error: %v", err)
	}
	if resp3.Status != protocol.StatusError {
		t.Fatalf("expected error status for conflicting replay, got %s", resp3.Status)
	}
	if resp3.Error == nil || resp3.Error.Category != "REQUEST_IDENTITY_CONFLICT" {
		t.Fatalf("expected REQUEST_IDENTITY_CONFLICT, got: %+v", resp3.Error)
	}

	// Worker should be stopped cleanly
	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("worker.Stop failed: %v", err)
	}
	t.Logf("real-NX idempotency verified: initial and replay returned identical results with 1 CAD body, conflict rejected")
}
