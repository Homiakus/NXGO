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

// TestRealNXCanonicalObjectRefFailClosed proves H3 requirements on real NX:
// 1. A valid work/display part exists with an established CAD body.
// 2. An invalid PartRef (foreign session / unknown object ID) fails closed before mutation.
// 3. A handle with wrong Kind (e.g. body ref passed where part is expected) fails closed.
// 4. A handle with wrong/stale generation fails closed before mutation.
// 5. Semantic verification: throughout all invalid attempts, the valid work part
//    in NX remains strictly untouched (zero unintended CAD changes).
func TestRealNXCanonicalObjectRefFailClosed(t *testing.T) {
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
	partPath := filepath.Join(t.TempDir(), "nxgo_failclosed_part.prt")
	part, err := session.NewPart(ctx, partPath, "mm")
	if err != nil {
		t.Fatalf("create test part: %v", err)
	}

	// Create one valid block so the part has baseline CAD state:
	validBlock, err := part.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 50,
		Width:  50,
		Height: 50,
	})
	if err != nil {
		t.Fatalf("create baseline valid block: %v", err)
	}

	baselineSummary, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("get baseline summary: %v", err)
	}
	if baselineSummary.BodyCount != 1 || baselineSummary.FeatureCount < 1 {
		t.Fatalf("precondition failed: expected 1 body, got %+v", baselineSummary)
	}

	// Case 1: Foreign/invalid session handle
	foreignRef := protocol.ObjectHandleWire{
		SessionID:  "foreign-session-9999",
		Epoch:      1,
		ObjectID:   "99999",
		Generation: 1,
		Kind:       "Part",
	}
	payloadForeign, _ := protocol.EncodePayload(protocol.FeatureCreateBlockRequest{
		PartRef: &foreignRef,
		Units:   "mm",
		Origin:  nxgo.Point3D{100, 0, 0},
		Length:  10, Width: 10, Height: 10,
	})
	resp1, err := worker.Client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-failclosed-foreign",
		Op:        "feature.create_block",
		Payload:   payloadForeign,
	})
	if err != nil {
		t.Fatalf("expected protocol error response, got transport error: %v", err)
	}
	if resp1.Status != protocol.StatusError {
		t.Fatalf("foreign session handle must fail closed, got status: %s", resp1.Status)
	}

	// Case 2: Wrong Kind (pass valid block BodyRef where PartRef is required)
	wrongKindRef := protocol.ObjectHandleWire{
		SessionID:  part.Ref.SessionID,
		Epoch:      part.Ref.Epoch,
		ObjectID:   validBlock.BodyRef.ObjectID,
		Generation: validBlock.BodyRef.Generation,
		Kind:       "Body", // Wrong kind: expected Part
	}
	payloadWrongKind, _ := protocol.EncodePayload(protocol.FeatureCreateBlockRequest{
		PartRef: &wrongKindRef,
		Units:   "mm",
		Origin:  nxgo.Point3D{200, 0, 0},
		Length:  10, Width: 10, Height: 10,
	})
	resp2, err := worker.Client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-failclosed-wrong-kind",
		Op:        "feature.create_block",
		Payload:   payloadWrongKind,
	})
	if err != nil {
		t.Fatalf("expected protocol error response, got transport error: %v", err)
	}
	if resp2.Status != protocol.StatusError {
		t.Fatalf("wrong kind handle must fail closed, got status: %s", resp2.Status)
	}

	// Case 3: Stale generation on the valid part's object ID
	staleGenRef := protocol.ObjectHandleWire{
		SessionID:  part.Ref.SessionID,
		Epoch:      part.Ref.Epoch,
		ObjectID:   part.Ref.ObjectID,
		Generation: part.Ref.Generation + 500, // Non-matching generation
		Kind:       "Part",
	}
	payloadStaleGen, _ := protocol.EncodePayload(protocol.FeatureCreateBlockRequest{
		PartRef: &staleGenRef,
		Units:   "mm",
		Origin:  nxgo.Point3D{300, 0, 0},
		Length:  10, Width: 10, Height: 10,
	})
	resp3, err := worker.Client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-failclosed-stale-gen",
		Op:        "feature.create_block",
		Payload:   payloadStaleGen,
	})
	if err != nil {
		t.Fatalf("expected protocol error response, got transport error: %v", err)
	}
	if resp3.Status != protocol.StatusError {
		t.Fatalf("stale generation handle must fail closed, got status: %s", resp3.Status)
	}

	// Verify semantic postcondition on real NX:
	// None of the invalid mutation attempts must have fallen back to the work part!
	afterSummary, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("query summary after invalid mutations: %v", err)
	}
	if afterSummary.BodyCount != baselineSummary.BodyCount || afterSummary.FeatureCount != baselineSummary.FeatureCount {
		t.Fatalf("work part was mutated by invalid reference requests! baseline=%+v after=%+v", baselineSummary, afterSummary)
	}

	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("worker shutdown failed: %v", err)
	}
	t.Logf("real-NX fail-closed ObjectRef verified: foreign session, wrong kind and stale generation rejected with zero CAD mutations")
}
