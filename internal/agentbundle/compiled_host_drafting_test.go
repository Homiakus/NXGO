package agentbundle

import (
	"strings"
	"testing"
)

func TestCanonicalCompiledHostDraftingLaneIsWired(t *testing.T) {
	root := repoRootForCompiledHostTest(t)
	host := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/EntryPoint.cs")
	drafting := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/EntryPoint.Drafting.cs")

	for _, marker := range []string{
		"drafting.create_sheet",
		"drafting.query_sheets",
		"drafting.export_pdf",
	} {
		if !strings.Contains(host, marker) {
			t.Errorf("canonical NXHost router/capabilities missing Drafting marker %q", marker)
		}
	}
	for _, marker := range []string{
		"MaxDraftingSnapshotSheets = 1024",
		"ownerObjectId: partHandle.ObjectId",
		"value snapshots",
		"currently supports only black_and_white color mode",
		"requested drawing sheets were not found",
		"SelectDrawingSheets(part, requestedSheets)",
		"NX produced an empty PDF artifact",
		"if (!File.Exists(outputPath))",
		"var size = new FileInfo(outputPath).Length",
	} {
		if !strings.Contains(drafting, marker) {
			t.Errorf("canonical Drafting adapter missing safety marker %q", marker)
		}
	}

	create := csharpMethodBody(t, drafting, "StartDraftingCreateSheet")
	createResolve := strings.Index(create, "Registry.Resolve(partHandle, \"Part\")")
	createStarted := strings.Index(create, "Journal.MarkStarted(requestId);")
	createCapacity := strings.Index(create, "Registry.Count >= Registry.Capacity")
	createExecutor := strings.Index(create, "executor.EnqueueTracked")
	if createResolve < createExecutor || createResolve > createStarted {
		t.Fatal("create_sheet must revalidate Part on the NX execution thread before MarkStarted")
	}
	if createCapacity < 0 || createCapacity > createStarted {
		t.Fatal("create_sheet must fail registry-capacity preflight before NX mutation start")
	}

	query := csharpMethodBody(t, drafting, "StartDraftingQuerySheets")
	if strings.Contains(query, "Registry.Register(") {
		t.Fatal("query_sheets must remain a value snapshot and never consume registry handles")
	}
	requireExecutionTimeResolve(t, drafting, "StartDraftingQuerySheets", "Registry.Resolve(partHandle, \"Part\")", false)

	export := csharpMethodBody(t, drafting, "StartDraftingExportPdf")
	exportResolve := strings.Index(export, "Registry.Resolve(partHandle, \"Part\")")
	exportSelect := strings.Index(export, "SelectDrawingSheets(part, requestedSheets)")
	exportStarted := strings.Index(export, "Journal.MarkStarted(requestId);")
	exportCommit := strings.Index(export, "scope.CommitOnce")
	exportExecutor := strings.Index(export, "executor.EnqueueTracked")
	if exportResolve < exportExecutor || exportResolve > exportStarted {
		t.Fatal("export_pdf must revalidate Part at execution time before MarkStarted")
	}
	if exportSelect < 0 || exportSelect > exportStarted {
		t.Fatal("export_pdf must resolve the requested sheet set before mutation/artifact start")
	}
	if exportCommit < 0 || exportStarted > exportCommit {
		t.Fatal("export_pdf must record started state before PrintPDFBuilder commit")
	}
}
