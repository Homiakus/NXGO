package nx_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/supervisor"
	"github.com/Homiakus/NXGO/pkg/nxgo"
)

func getNXHome(t *testing.T) string {
	if runtime.GOOS != "windows" {
		t.Skip("skipping real NX test on non-Windows host")
	}

	if os.Getenv("NXGO_RUN_REAL_NX") == "" {
		t.Skip("skipping real NX test; set NXGO_RUN_REAL_NX=1 or run via 'nxctl test nx'")
	}

	nxHome := os.Getenv("NXGO_NX_HOME")
	if nxHome == "" {
		nxHome = os.Getenv("UGII_BASE_DIR")
	}
	if nxHome == "" {
		installs, err := supervisor.Discover()
		if err != nil || len(installs) == 0 {
			t.Skip("no real Siemens NX installation found; set NXGO_NX_HOME or UGII_BASE_DIR")
		}
		nxHome = installs[0].Home
	}
	return nxHome
}

func startTestWorker(t *testing.T, ctx context.Context) (*supervisor.WorkerProcess, *nxgo.Session) {
	nxHome := getNXHome(t)
	cfg := supervisor.WorkerConfig{
		NXHome:         nxHome,
		StartupTimeout: 45 * time.Second,
	}

	t.Logf("starting real NX Agent worker against %s...", nxHome)
	worker, err := supervisor.StartWorker(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to start real NX Agent worker: %v", err)
	}

	session := nxgo.WrapClient(worker.Client, worker.Manifest.ID, 1, worker.Manifest.NXRelease)
	return worker, session
}

func TestRealNXAgentBootstrapAndSessionQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	// 1. Call nx.ping
	if err := session.Ping(ctx); err != nil {
		t.Fatalf("session.Ping failed: %v", err)
	}
	t.Log("nx.ping passed on bound NX thread!")

	// 2. Call session.info
	info, err := session.Info(ctx)
	if err != nil {
		t.Fatalf("session.Info failed: %v", err)
	}
	if info.ThreadID == 0 {
		t.Fatalf("expected non-zero thread ID, got %d", info.ThreadID)
	}
	t.Logf("session.info: release=%s thread=%d basedir=%s", info.Release, info.ThreadID, info.BaseDir)

	// 3. Graceful shutdown
	t.Log("stopping real NX Agent...")
	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("worker stop failed: %v", err)
	}
	t.Log("real NX Agent stopped cleanly with code 0")
}

func TestRealNXPartOperationsAndObjectRegistry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()
	partFilePath := filepath.Join(tempDir, "nxgo_bracket_test.prt")

	// 1. Create a new part in millimeters
	t.Log("creating new part...")
	part, err := session.NewPart(ctx, partFilePath, "mm")
	if err != nil {
		t.Fatalf("NewPart failed: %v", err)
	}

	if part.Ref.ObjectID == "" {
		t.Fatal("expected non-empty ObjectID in part ref")
	}
	if part.Ref.NativeTag == 0 {
		t.Fatal("expected non-zero NativeTag for Part TaggedObject")
	}
	t.Logf("part created: name=%s units=%s handle=%s native_tag=%d",
		part.Name, part.Units, part.Ref.ObjectID, part.Ref.NativeTag)

	// 2. Query part summary
	t.Log("querying part summary...")
	summary, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("part.Summary failed: %v", err)
	}
	if summary.Units != "Millimeters" {
		t.Fatalf("expected Millimeters, got %s", summary.Units)
	}
	if summary.BodyCount != 0 || summary.FeatureCount != 0 {
		t.Fatalf("expected 0 bodies and features in new part, got bodies=%d features=%d",
			summary.BodyCount, summary.FeatureCount)
	}
	t.Logf("part summary verified: bodies=%d features=%d components=%d",
		summary.BodyCount, summary.FeatureCount, summary.ComponentCount)

	// 3. Save part to disk
	t.Log("saving part...")
	saveResp, err := part.Save(ctx)
	if err != nil {
		t.Fatalf("part.Save failed: %v", err)
	}
	if !saveResp.Saved {
		t.Fatal("expected saveResp.Saved == true")
	}
	t.Logf("part saved: full_path=%s", saveResp.FullPath)

	// 4. Close part
	t.Log("closing part...")
	if err := part.Close(ctx, false); err != nil {
		t.Fatalf("part.Close failed: %v", err)
	}
	t.Log("part closed cleanly")

	// 5. Verify handle invalidation after close (Object Registry check)
	_, err = part.Summary(ctx)
	if err == nil {
		t.Fatal("expected summary query on closed handle to fail, but it succeeded")
	}
	t.Logf("stale handle rejected as expected: %v", err)

	// 6. Re-open saved part from disk
	t.Log("re-opening saved part...")
	reopenedPart, err := session.OpenPart(ctx, partFilePath)
	if err != nil {
		t.Fatalf("session.OpenPart failed: %v", err)
	}
	t.Logf("part reopened: handle=%s native_tag=%d",
		reopenedPart.Ref.ObjectID, reopenedPart.Ref.NativeTag)

	// 7. Verify summary of reopened part
	reopenedSummary, err := reopenedPart.Summary(ctx)
	if err != nil {
		t.Fatalf("reopened summary failed: %v", err)
	}
	if reopenedSummary.Units != "Millimeters" {
		t.Fatalf("expected Millimeters, got %s", reopenedSummary.Units)
	}

	// 8. Close reopened part
	_ = reopenedPart.Close(ctx, false)

	// 9. Clean shutdown
	_ = worker.Stop(ctx)
}

func TestRealNXTransactionRollbackAndCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	tempDir := t.TempDir()
	partFilePath := filepath.Join(tempDir, "nxgo_tx_test.prt")

	// 1. Create new part
	part, err := session.NewPart(ctx, partFilePath, "mm")
	if err != nil {
		t.Fatalf("NewPart failed: %v", err)
	}

	// 2. Begin transaction 1
	t.Log("beginning transaction 1...")
	tx1, err := session.BeginTx(ctx, "test_tx_rollback")
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}
	if tx1.TxID == "" || tx1.MarkID == 0 {
		t.Fatalf("invalid transaction 1: tx_id=%s mark_id=%d", tx1.TxID, tx1.MarkID)
	}
	t.Logf("transaction 1 created: tx_id=%s mark_id=%d", tx1.TxID, tx1.MarkID)

	// 3. Rollback transaction 1
	t.Log("rolling back transaction 1...")
	if err := tx1.Rollback(ctx); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	t.Log("transaction 1 rolled back successfully")

	// 4. Begin transaction 2 and commit
	t.Log("beginning transaction 2...")
	tx2, err := session.BeginTx(ctx, "test_tx_commit")
	if err != nil {
		t.Fatalf("BeginTx 2 failed: %v", err)
	}

	t.Log("committing transaction 2...")
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	t.Log("transaction 2 committed successfully")

	// 5. Clean close
	_ = part.Close(ctx, false)
	_ = worker.Stop(ctx)
}

func TestRealNXStaleHandleRejection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	worker, _ := startTestWorker(t, ctx)
	defer worker.Kill()

	// 1. Query with foreign session ID
	foreignRef := protocol.ObjectHandleWire{
		SessionID: "foreign-session-id-999",
		Epoch:     1,
		ObjectID:  "obj-999",
	}

	reqData, _ := protocol.EncodePayload(protocol.PartSummaryRequest{PartRef: &foreignRef})
	resp, err := worker.Client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-foreign-sess",
		Op:        "part.query_summary",
		Payload:   reqData,
	})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if resp.Status == protocol.StatusOK {
		t.Fatal("expected call with foreign session_id to fail")
	}
	if !strings.Contains(resp.Error.Message, "stale session") {
		t.Fatalf("expected 'stale session' in error message, got: %s", resp.Error.Message)
	}
	t.Logf("foreign session reference rejected: %s", resp.Error.Message)

	// 2. Query with foreign/stale epoch
	foreignEpochRef := protocol.ObjectHandleWire{
		SessionID: worker.Manifest.ID,
		Epoch:     9999,
		ObjectID:  "obj-999",
	}

	reqData2, _ := protocol.EncodePayload(protocol.PartSummaryRequest{PartRef: &foreignEpochRef})
	resp2, err := worker.Client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-foreign-epoch",
		Op:        "part.query_summary",
		Payload:   reqData2,
	})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if resp2.Status == protocol.StatusOK {
		t.Fatal("expected call with stale epoch to fail")
	}
	if !strings.Contains(resp2.Error.Message, "stale epoch") {
		t.Fatalf("expected 'stale epoch' in error message, got: %s", resp2.Error.Message)
	}
	t.Logf("stale epoch reference rejected: %s", resp2.Error.Message)

	_ = worker.Stop(ctx)
}
