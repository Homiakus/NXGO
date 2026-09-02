package nx_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/supervisor"
	"github.com/Homiakus/NXGO/pkg/nxgo"
)

// TestRealNXCanonicalCompiledHost proves the H4 migration lane end-to-end:
// run_journal -> minimal CompiledHostBootstrap.cs -> NXGO.Agent.NXHost.dll ->
// NXGO.Agent.Core.dll -> shared NxExecutor/RequestJournal/HandleRegistry on the
// NX execution thread. It also runs a geometric oracle through the canonical
// adapter so E3 can prove units and BuilderScope behavior when the self-hosted
// Siemens NX runner is actually executed.
func TestRealNXCanonicalCompiledHost(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("canonical compiled NXHost requires Windows/Siemens NX")
	}
	if os.Getenv("NXGO_RUN_REAL_NX") == "" {
		t.Skip("set NXGO_RUN_REAL_NX=1 or run via the real-NX quality gate")
	}

	repoRoot := repoRootFromTestFile(t)
	agentBin := os.Getenv("NXGO_AGENT_BIN")
	if agentBin == "" {
		agentBin = filepath.Join(repoRoot, "agent", "bin")
	}

	coreDLL := filepath.Join(agentBin, "NXGO.Agent.Core.dll")
	hostDLL := filepath.Join(agentBin, "NXGO.Agent.NXHost.dll")
	missing := ""
	if _, err := os.Stat(coreDLL); err != nil {
		missing = coreDLL
	}
	if _, err := os.Stat(hostDLL); err != nil {
		missing = hostDLL
	}
	if missing != "" {
		if os.Getenv("NXGO_REQUIRE_COMPILED_HOST") == "1" {
			t.Fatalf("canonical compiled Agent output is mandatory but missing: %s; run scripts/build-agent.ps1", missing)
		}
		t.Skipf("canonical compiled Agent not built: %s", missing)
	}

	oldBin, hadBin := os.LookupEnv("NXGO_AGENT_BIN")
	if err := os.Setenv("NXGO_AGENT_BIN", agentBin); err != nil {
		t.Fatalf("set NXGO_AGENT_BIN: %v", err)
	}
	defer func() {
		if hadBin {
			_ = os.Setenv("NXGO_AGENT_BIN", oldBin)
		} else {
			_ = os.Unsetenv("NXGO_AGENT_BIN")
		}
	}()

	bootstrap := filepath.Join(repoRoot, "agent", "bundle", "CompiledHostBootstrap.cs")
	if _, err := os.Stat(bootstrap); err != nil {
		t.Fatalf("compiled-host bootstrap missing: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	worker, err := supervisor.StartWorker(ctx, supervisor.WorkerConfig{
		NXHome:         getNXHome(t),
		JournalPath:    bootstrap,
		StartupTimeout: 45 * time.Second,
	})
	if err != nil {
		t.Fatalf("start canonical compiled NXHost: %v", err)
	}
	defer worker.Kill()

	if worker.Manifest == nil || worker.Manifest.ID == "" {
		t.Fatal("canonical compiled NXHost did not produce a session manifest")
	}

	session := nxgo.WrapClient(worker.Client, worker.Manifest.ID, 1, worker.Manifest.NXRelease)
	if err := session.Ping(ctx); err != nil {
		t.Fatalf("canonical compiled NXHost ping failed: %v", err)
	}
	info, err := session.Info(ctx)
	if err != nil {
		t.Fatalf("canonical compiled NXHost session.info failed: %v", err)
	}
	if info.ThreadID == 0 {
		t.Fatal("canonical compiled NXHost returned zero NX execution thread id")
	}
	if info.SessionID != worker.Manifest.ID {
		t.Fatalf("session identity mismatch: info=%q manifest=%q", info.SessionID, worker.Manifest.ID)
	}
	if info.Epoch != 1 {
		t.Fatalf("unexpected canonical NXHost epoch: got %d want 1", info.Epoch)
	}

	partPath := filepath.Join(t.TempDir(), "nxgo_compiled_host_part.prt")
	part, err := session.NewPart(ctx, partPath, "mm")
	if err != nil {
		t.Fatalf("canonical compiled NXHost part.new failed: %v", err)
	}
	if part.Ref.ObjectID == "" || part.Ref.Generation == 0 {
		t.Fatalf("canonical part handle is incomplete: object=%q generation=%d", part.Ref.ObjectID, part.Ref.Generation)
	}
	if part.Ref.SessionID != worker.Manifest.ID || part.Ref.Epoch != 1 || part.Ref.Kind != "Part" {
		t.Fatalf("canonical part handle identity mismatch: %+v", part.Ref)
	}

	summary, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("canonical compiled NXHost part.summary failed: %v", err)
	}
	if summary.BodyCount != 0 || summary.FeatureCount != 0 {
		t.Fatalf("new canonical part should be empty: bodies=%d features=%d", summary.BodyCount, summary.FeatureCount)
	}

	block, err := part.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 100,
		Width:  50,
		Height: 25,
	})
	if err != nil {
		t.Fatalf("canonical compiled NXHost CreateBlock failed: %v", err)
	}
	if block.Ref.Generation == 0 || block.BodyRef.Generation == 0 {
		t.Fatalf("canonical block handles must carry generation: feature=%+v body=%+v", block.Ref, block.BodyRef)
	}

	mp, err := part.MassProperties(ctx)
	if err != nil {
		t.Fatalf("canonical block MassProperties failed: %v", err)
	}
	if math.Abs(mp.Volume-125000.0) > 0.1 {
		t.Fatalf("canonical metric volume contract mismatch: got %.6f want 125000 mm^3", mp.Volume)
	}
	if math.Abs(mp.Area-17500.0) > 0.1 {
		t.Fatalf("canonical metric area contract mismatch: got %.6f want 17500 mm^2", mp.Area)
	}
	if math.Abs(mp.Centroid[0]-50.0) > 0.1 || math.Abs(mp.Centroid[1]-25.0) > 0.1 || math.Abs(mp.Centroid[2]-12.5) > 0.1 {
		t.Fatalf("canonical metric centroid mismatch: got %+v want [50 25 12.5] mm", mp.Centroid)
	}

	bbox, err := part.BoundingBox(ctx)
	if err != nil {
		t.Fatalf("canonical block BoundingBox failed: %v", err)
	}
	if math.Abs(bbox.Dimensions[0]-100.0) > 0.01 || math.Abs(bbox.Dimensions[1]-50.0) > 0.01 || math.Abs(bbox.Dimensions[2]-25.0) > 0.01 {
		t.Fatalf("canonical bbox unit contract mismatch: got %+v want [100 50 25]", bbox.Dimensions)
	}

	cylinder, err := part.CreateCylinder(ctx, nxgo.CylinderParams{
		Origin:    nxgo.Point3D{200, 0, 0},
		Direction: nxgo.Vector3D{0, 0, 1},
		Diameter:  20,
		Height:    30,
	})
	if err != nil {
		t.Fatalf("canonical compiled NXHost CreateCylinder failed: %v", err)
	}
	if cylinder.Ref.Generation == 0 || cylinder.BodyRef.Generation == 0 {
		t.Fatalf("canonical cylinder handles must carry generation: feature=%+v body=%+v", cylinder.Ref, cylinder.BodyRef)
	}

	bodies, err := part.Bodies(ctx)
	if err != nil {
		t.Fatalf("canonical compiled NXHost Bodies failed: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("canonical geometry expected 2 bodies after block+cylinder, got %d", len(bodies))
	}
	bodyRefs := make([]interface{ ObjectIDString() string }, 0)
	_ = bodyRefs // body handles are released by the SDK aggregation path; direct query handles are released explicitly below.
	for i, body := range bodies {
		if body.Ref.Generation == 0 || body.FaceCount == 0 || body.EdgeCount == 0 {
			t.Fatalf("canonical body %d is incomplete: %+v", i, body)
		}
	}
	// Part.Bodies exposes explicit handles, so callers own them. This also
	// exercises canonical object.release and prevents fixture-induced leaks.
	if err := session.ReleaseObjects(ctx, bodies[0].Ref, bodies[1].Ref); err != nil {
		t.Fatalf("canonical object.release failed: %v", err)
	}

	summary, err = part.Summary(ctx)
	if err != nil {
		t.Fatalf("canonical summary after geometry failed: %v", err)
	}
	if summary.BodyCount != 2 || summary.FeatureCount < 2 {
		t.Fatalf("canonical geometry summary mismatch: bodies=%d features=%d", summary.BodyCount, summary.FeatureCount)
	}

	if err := session.ReleaseObjects(ctx, block.Ref, block.BodyRef, cylinder.Ref, cylinder.BodyRef); err != nil {
		t.Fatalf("canonical feature/body handle cleanup failed: %v", err)
	}

	saved, err := part.Save(ctx)
	if err != nil {
		t.Fatalf("canonical compiled NXHost part.save failed: %v", err)
	}
	if !saved.Saved {
		t.Fatal("canonical compiled NXHost did not report saved=true")
	}

	if err := part.Close(ctx, false); err != nil {
		t.Fatalf("canonical compiled NXHost part.close failed: %v", err)
	}
	if _, err := part.Summary(ctx); err == nil {
		t.Fatal("released canonical Part handle unexpectedly remained resolvable after close")
	}

	reopened, err := session.OpenPart(ctx, partPath)
	if err != nil {
		t.Fatalf("canonical compiled NXHost part.open failed: %v", err)
	}
	if reopened.Ref.Generation == 0 {
		t.Fatal("reopened canonical part is missing generation")
	}
	reopenedSummary, err := reopened.Summary(ctx)
	if err != nil {
		t.Fatalf("canonical reopened part summary failed: %v", err)
	}
	if reopenedSummary.BodyCount != 2 {
		t.Fatalf("canonical reopened part lost geometry: bodies=%d want 2", reopenedSummary.BodyCount)
	}
	if err := reopened.Close(ctx, false); err != nil {
		t.Fatalf("canonical reopened part close failed: %v", err)
	}

	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("canonical compiled NXHost shutdown failed: %v", err)
	}
	t.Logf("canonical Core->NXHost geometry verified: session=%s thread=%d release=%s volume=%.1f area=%.1f bbox=%+v first_handle=%s/%d reopened_handle=%s/%d",
		info.SessionID, info.ThreadID, info.Release, mp.Volume, mp.Area, bbox.Dimensions,
		part.Ref.ObjectID, part.Ref.Generation,
		reopened.Ref.ObjectID, reopened.Ref.Generation)
}

func repoRootFromTestFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve compiled_host_test.go path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolved repository root is invalid: %s: %v", root, err)
	}
	return root
}
