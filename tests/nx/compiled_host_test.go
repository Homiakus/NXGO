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
// supervisor canonical mode -> run_dotnet_core_nxopen.exe ->
// NXGO.Agent.NXHost.dll -> shared NxExecutor/RequestJournal/HandleRegistry on the
// NX execution thread. It runs geometry, transaction, lifetime and Assembly
// oracles through canonical adapters. E3 is earned only when this fixture is
// actually executed on the self-hosted Siemens NX runner.
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

	missing := ""
	for _, dll := range []string{"Newtonsoft.Json.dll", "NXGO.Protocol.dll", "NXGO.Agent.Core.dll", "NXGO.Agent.NXHost.dll"} {
		path := filepath.Join(agentBin, dll)
		if _, err := os.Stat(path); err != nil {
			missing = path
			break
		}
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

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
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
	for i, body := range bodies {
		if body.Ref.Generation == 0 || body.FaceCount == 0 || body.EdgeCount == 0 {
			t.Fatalf("canonical body %d is incomplete: %+v", i, body)
		}
	}
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

	rollbackTx, err := session.BeginTx(ctx, "canonical_rollback")
	if err != nil {
		t.Fatalf("canonical transaction.begin for rollback failed: %v", err)
	}
	rolledBackFeature, err := part.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 100, 0},
		Length: 10,
		Width:  10,
		Height: 10,
	})
	if err != nil {
		t.Fatalf("canonical CreateBlock inside rollback transaction failed: %v", err)
	}
	if rolledBackFeature.Ref.Generation == 0 {
		t.Fatal("rollback candidate feature is missing generation")
	}
	summaryDuringRollback, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("canonical summary before rollback failed: %v", err)
	}
	if summaryDuringRollback.BodyCount != 3 {
		t.Fatalf("canonical rollback precondition mismatch: bodies=%d want 3", summaryDuringRollback.BodyCount)
	}
	if err := rollbackTx.Rollback(ctx); err != nil {
		t.Fatalf("canonical transaction.rollback failed: %v", err)
	}
	summaryAfterRollback, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("canonical summary after rollback failed: %v", err)
	}
	if summaryAfterRollback.BodyCount != 2 {
		t.Fatalf("canonical rollback did not restore body count: got %d want 2", summaryAfterRollback.BodyCount)
	}

	commitTx, err := session.BeginTx(ctx, "canonical_commit")
	if err != nil {
		t.Fatalf("canonical transaction.begin for commit failed: %v", err)
	}
	committedFeature, err := part.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 200, 0},
		Length: 8,
		Width:  8,
		Height: 8,
	})
	if err != nil {
		t.Fatalf("canonical CreateBlock inside commit transaction failed: %v", err)
	}
	if err := commitTx.Commit(ctx); err != nil {
		t.Fatalf("canonical transaction.commit failed: %v", err)
	}
	summaryAfterCommit, err := part.Summary(ctx)
	if err != nil {
		t.Fatalf("canonical summary after commit failed: %v", err)
	}
	if summaryAfterCommit.BodyCount != 3 {
		t.Fatalf("canonical commit did not preserve body count: got %d want 3", summaryAfterCommit.BodyCount)
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
	if err := session.ReleaseObjects(ctx, block.Ref); err == nil {
		t.Fatal("dependent Feature handle unexpectedly remained live after owning Part close")
	}
	if err := session.ReleaseObjects(ctx, cylinder.BodyRef); err == nil {
		t.Fatal("dependent Body handle unexpectedly remained live after owning Part close")
	}
	if err := session.ReleaseObjects(ctx, rolledBackFeature.Ref); err == nil {
		t.Fatal("rolled-back Feature handle unexpectedly remained live after owning Part close")
	}
	if err := session.ReleaseObjects(ctx, committedFeature.Ref); err == nil {
		t.Fatal("committed Feature handle unexpectedly remained live after owning Part close")
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
	if reopenedSummary.BodyCount != 3 {
		t.Fatalf("canonical reopened part lost committed geometry: bodies=%d want 3", reopenedSummary.BodyCount)
	}
	if err := reopened.Close(ctx, false); err != nil {
		t.Fatalf("canonical reopened part close failed: %v", err)
	}

	// Assembly oracle. Query tree/BOM must be value snapshots (no operational
	// handles), while AddComponent returns persistent handles owned by the
	// assembly Part and Remove consumes exactly that identity.
	assemblyDir := t.TempDir()
	childPath := filepath.Join(assemblyDir, "canonical_child.prt")
	child, err := session.NewPart(ctx, childPath, "mm")
	if err != nil {
		t.Fatalf("canonical child part.new failed: %v", err)
	}
	if _, err := child.CreateBlock(ctx, nxgo.BlockParams{
		Origin: nxgo.Point3D{0, 0, 0},
		Length: 10,
		Width:  10,
		Height: 5,
	}); err != nil {
		t.Fatalf("canonical child geometry failed: %v", err)
	}
	if _, err := child.Save(ctx); err != nil {
		t.Fatalf("canonical child save failed: %v", err)
	}
	if err := child.Close(ctx, false); err != nil {
		t.Fatalf("canonical child close failed: %v", err)
	}

	assemblyPath := filepath.Join(assemblyDir, "canonical_assembly.prt")
	assemblyPart, err := session.NewPart(ctx, assemblyPath, "mm")
	if err != nil {
		t.Fatalf("canonical assembly part.new failed: %v", err)
	}
	componentA, err := assemblyPart.AddComponent(ctx, nxgo.AddComponentParams{
		PartPath:      childPath,
		ComponentName: "CHILD_A",
		Origin:        nxgo.Point3D{0, 0, 0},
	})
	if err != nil {
		t.Fatalf("canonical assembly.add_component A failed: %v", err)
	}
	componentB, err := assemblyPart.AddComponent(ctx, nxgo.AddComponentParams{
		PartPath:      childPath,
		ComponentName: "CHILD_B",
		Origin:        nxgo.Point3D{20, 0, 0},
	})
	if err != nil {
		t.Fatalf("canonical assembly.add_component B failed: %v", err)
	}
	if componentA.Ref.Generation == 0 || componentB.Ref.Generation == 0 {
		t.Fatalf("canonical persistent Component handles require generation: A=%+v B=%+v", componentA.Ref, componentB.Ref)
	}

	tree, err := assemblyPart.ComponentTree(ctx)
	if err != nil {
		t.Fatalf("canonical assembly.query_tree failed: %v", err)
	}
	if len(tree.Children) != 2 {
		t.Fatalf("canonical assembly tree expected 2 children, got %d", len(tree.Children))
	}
	for i, node := range tree.Children {
		if node.Ref.ObjectID != "" || node.Ref.Generation != 0 {
			t.Fatalf("assembly snapshot child %d unexpectedly carries operational handle: %+v", i, node.Ref)
		}
		if node.PrototypePath == "" {
			t.Fatalf("assembly snapshot child %d has empty prototype path", i)
		}
	}

	bom, err := assemblyPart.BOM(ctx)
	if err != nil {
		t.Fatalf("canonical assembly.query_bom failed: %v", err)
	}
	if len(bom) != 1 || bom[0].Quantity != 2 {
		t.Fatalf("canonical BOM expected one child part with quantity 2, got %+v", bom)
	}

	if err := componentB.Remove(ctx); err != nil {
		t.Fatalf("canonical assembly.remove_component failed: %v", err)
	}
	if err := session.ReleaseObjects(ctx, componentB.Ref); err == nil {
		t.Fatal("removed Component handle unexpectedly remained live")
	}
	treeAfterRemove, err := assemblyPart.ComponentTree(ctx)
	if err != nil {
		t.Fatalf("canonical tree after component removal failed: %v", err)
	}
	if len(treeAfterRemove.Children) != 1 {
		t.Fatalf("canonical assembly tree expected 1 child after removal, got %d", len(treeAfterRemove.Children))
	}

	if _, err := assemblyPart.Save(ctx); err != nil {
		t.Fatalf("canonical assembly save failed: %v", err)
	}
	if err := assemblyPart.Close(ctx, false); err != nil {
		t.Fatalf("canonical assembly close failed: %v", err)
	}
	if err := session.ReleaseObjects(ctx, componentA.Ref); err == nil {
		t.Fatal("Component handle unexpectedly survived owning assembly Part close")
	}

	reopenedAssembly, err := session.OpenPart(ctx, assemblyPath)
	if err != nil {
		t.Fatalf("canonical assembly reopen failed: %v", err)
	}
	reopenedTree, err := reopenedAssembly.ComponentTree(ctx)
	if err != nil {
		t.Fatalf("canonical reopened assembly tree failed: %v", err)
	}
	if len(reopenedTree.Children) != 1 {
		t.Fatalf("canonical reopened assembly expected 1 persisted child, got %d", len(reopenedTree.Children))
	}
	if err := reopenedAssembly.Close(ctx, false); err != nil {
		t.Fatalf("canonical reopened assembly close failed: %v", err)
	}

	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("canonical compiled NXHost shutdown failed: %v", err)
	}
	t.Logf("canonical Core->NXHost geometry+transactions+assembly verified: session=%s thread=%d release=%s volume=%.1f area=%.1f bbox=%+v assembly_children=%d",
		info.SessionID, info.ThreadID, info.Release, mp.Volume, mp.Area, bbox.Dimensions, len(reopenedTree.Children))
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
