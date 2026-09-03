package agentbundle

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRootForCompiledHostTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve compiled host test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readRepoFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func csharpMethodBody(t *testing.T, source, name string) string {
	t.Helper()
	start := strings.Index(source, "private static Task<byte[]> "+name)
	if start < 0 {
		t.Fatalf("canonical NXHost missing method %s", name)
	}
	rest := source[start+1:]
	end := strings.Index(rest, "\n    private static ")
	if end < 0 {
		return source[start:]
	}
	return source[start : start+1+end]
}

func requireExecutionTimeResolve(t *testing.T, source, method, resolveMarker string, mutation bool) {
	t.Helper()
	body := csharpMethodBody(t, source, method)
	executor := strings.Index(body, "executor.EnqueueTracked")
	resolve := strings.Index(body, resolveMarker)
	if executor < 0 || resolve < 0 || resolve < executor {
		t.Errorf("%s must resolve opaque handles inside queued NX execution", method)
		return
	}
	if mutation {
		started := strings.Index(body, "Journal.MarkStarted(requestId);")
		if started < 0 || resolve > started {
			t.Errorf("%s must resolve/revalidate handles before Journal.MarkStarted", method)
		}
	}
}

func TestCanonicalCompiledHostMigrationLaneIsWired(t *testing.T) {
	root := repoRootForCompiledHostTest(t)
	bootstrap := readRepoFile(t, root, "agent/bundle/CompiledHostBootstrap.cs")
	host := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/EntryPoint.cs")
	geometry := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/EntryPoint.Geometry.cs")
	transactions := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/EntryPoint.Transactions.cs")
	assembly := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/EntryPoint.Assembly.cs")
	registry := readRepoFile(t, root, "agent/NXGO.Agent.Core/HandleRegistry.cs")
	project := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/NXGO.Agent.NXHost.csproj")
	build := readRepoFile(t, root, "scripts/build-agent.ps1")

	for _, marker := range []string{
		"NXGO_AGENT_BIN",
		"Newtonsoft.Json.dll",
		"NXGO.Protocol.dll",
		"NXGO.Agent.Core.dll",
		"NXGO.Agent.NXHost.dll",
		"NXGO.Agent.NXHost.EntryPoint",
		"Assembly.LoadFrom",
	} {
		if !strings.Contains(bootstrap, marker) {
			t.Errorf("compiled bootstrap missing marker %q", marker)
		}
	}
	if strings.Contains(bootstrap, "class NxExecutor") || strings.Contains(bootstrap, "class ObjectRegistry") {
		t.Fatal("compiled bootstrap must remain a loader only; runtime primitives cannot be duplicated there")
	}

	if !strings.Contains(project, "<ProjectReference") || !strings.Contains(project, "NXGO.Agent.Core") || !strings.Contains(project, "NXGO.Protocol") {
		t.Fatal("NXHost must consume NXGO.Agent.Core and NXGO.Protocol through ProjectReferences")
	}
	if strings.Contains(project, "System.Web.Extensions") {
		t.Fatal("canonical NXHost must not retain the legacy JavaScriptSerializer dependency")
	}

	for _, marker := range []string{
		"using NXGO.Agent.Core;",
		"EntryPoint.Run",
		"new NxExecutor()",
		"new NamedPipeRequestServer",
		"HandleRegistry<TaggedObject>",
		"new RequestJournal",
		"JsonWireCodec",
		"WireMessageProbeDto",
		"RequestEnvelopeDto",
		"ResponseEnvelopeDto",
		"Journal.Admit(requestId, operation, Wire.Serialize(requestPayload), request.CorrelationId, request.TxId)",
		"private static readonly RequestJournal Journal = LoadJournal(JournalStore)",
		"private static void PersistJournalOrThrow()",
		"Journal.MarkCommitted(requestId, response); PersistJournalOrThrow();",
		"Journal.MarkOutcomeUnknown(requestId, ex.GetType().Name + \": \" + ex.Message); PersistJournalOrThrow();",
		"ProtocolMajor = 2",
		"part.new",
		"part.open",
		"part.save",
		"part.close",
		"part.query_summary",
		"feature.create_block",
		"feature.create_cylinder",
		"part.query_bodies",
		"geometry.query_mass_properties",
		"geometry.query_bounding_box",
		"object.release",
		"transaction.begin",
		"transaction.commit",
		"transaction.rollback",
		"assembly.add_component",
		"assembly.query_tree",
		"assembly.query_bom",
		"assembly.remove_component",
		"PreStartErrorCategory",
		"part.Save(BasePart.SaveComponents.True, BasePart.CloseAfterSave.False);",
		"part.Close(BasePart.CloseWholeTree.False, BasePart.CloseModified.CloseModified, null!);",
	} {
		if !strings.Contains(host, marker) {
			t.Errorf("canonical NXHost missing Core/v2/router marker %q", marker)
		}
	}

	for _, marker := range []string{
		"BuilderScope<NXOpen.Features.BlockFeatureBuilder>",
		"BuilderScope<NXOpen.Features.CylinderBuilder>",
		"GeometryUnitContract",
		"ContractFor(body).NormalizeBoundingBox",
		"MaxProducedHandlesPerRequest = 256",
		"ownerObjectId: partHandle.ObjectId",
		"leaseScopeId: requestId",
		"leaseScopeLimit: MaxProducedHandlesPerRequest",
		"StartObjectRelease",
		"GetString(payload, \"lease_scope_id\", \"\")",
		"Registry.ReleaseScope(leaseScopeId)",
	} {
		if !strings.Contains(geometry, marker) {
			t.Errorf("canonical Geometry adapter missing Core/lifetime marker %q", marker)
		}
	}

	for _, marker := range []string{
		"UndoTransactionLedger<Session.UndoMarkId>",
		"Transactions.EnsureCanBegin();",
		"Transactions.Take(txId);",
		"session.SetUndoMark",
		"session.DeleteUndoMark",
		"session.UndoToMark",
		"Journal.MarkStarted(requestId);",
	} {
		if !strings.Contains(transactions, marker) {
			t.Errorf("canonical Transaction adapter missing Core/NX marker %q", marker)
		}
	}
	for _, method := range []string{"StartTransactionCommit", "StartTransactionRollback"} {
		body := csharpMethodBody(t, transactions, method)
		claim := strings.Index(body, "Transactions.Take(txId);")
		started := strings.Index(body, "Journal.MarkStarted(requestId);")
		if claim < 0 || started < 0 || claim > started {
			t.Errorf("%s must claim the Core transaction before marking NX mutation started", method)
		}
	}
	beginBody := csharpMethodBody(t, transactions, "StartTransactionBegin")
	preflight := strings.Index(beginBody, "Transactions.EnsureCanBegin();")
	started := strings.Index(beginBody, "Journal.MarkStarted(requestId);")
	if preflight < 0 || started < 0 || preflight > started {
		t.Fatal("transaction.begin capacity preflight must happen before journal started state")
	}

	for _, marker := range []string{
		"MaxAssemblySnapshotNodes = 16384",
		"MaxAssemblySnapshotDepth = 64",
		"ownerObjectId: partHandle.ObjectId",
		"Registry.ResolveOwned(componentHandle, partHandle, \"Component\")",
		"Tree/BOM are snapshots, not object-identity APIs",
		"SerializeComponentSnapshot",
		"CollectBOMSnapshot",
		"assembly snapshot node count exceeds canonical safety limit",
	} {
		if !strings.Contains(assembly, marker) {
			t.Errorf("canonical Assembly adapter missing safety/snapshot marker %q", marker)
		}
	}
	if strings.Contains(assembly, "component.OwningPart") {
		t.Fatal("Assembly adapter must not validate ownership by touching NXOpen objects on the transport thread")
	}
	closeBody := csharpMethodBody(t, host, "StartPartClose")
	if savePos, closePos := strings.Index(closeBody, "part.Save("), strings.Index(closeBody, "part.Close("); savePos < 0 || closePos < 0 || savePos > closePos {
		t.Fatal("part.close with save must save before closing and must not suppress save failures")
	}
	if !strings.Contains(registry, "ResolveOwned(ObjectHandleToken token, ObjectHandleToken owner") {
		t.Fatal("Core registry must expose ownership validation for cross-object operations")
	}

	// Tree/BOM are deliberately value snapshots. Registry allocation is only
	// allowed in the add-component mutation, never while traversing query data.
	for _, method := range []string{"StartAssemblyQueryTree", "StartAssemblyQueryBOM"} {
		body := csharpMethodBody(t, assembly, method)
		if strings.Contains(body, "Registry.Register(") {
			t.Errorf("%s must not allocate operational handles while reading an assembly snapshot", method)
		}
	}

	// A wire handle may be parsed on the transport thread, but its native target
	// must be resolved again on the serialized NX execution thread. Mutations do
	// that before MarkStarted, so a queued close can make the later operation a
	// deterministic failed-before-start rather than touching a closed NX object.
	requireExecutionTimeResolve(t, host, "StartPartSave", "Registry.Resolve(handle, \"Part\")", true)
	requireExecutionTimeResolve(t, host, "StartPartClose", "Registry.Resolve(handle, \"Part\")", true)
	requireExecutionTimeResolve(t, host, "StartPartSummary", "Registry.Resolve(handle, \"Part\")", false)
	requireExecutionTimeResolve(t, geometry, "StartCreateBlock", "Registry.Resolve(partHandle, \"Part\")", true)
	requireExecutionTimeResolve(t, geometry, "StartCreateCylinder", "Registry.Resolve(partHandle, \"Part\")", true)
	requireExecutionTimeResolve(t, geometry, "StartQueryBodies", "Registry.Resolve(partHandle, \"Part\")", false)
	requireExecutionTimeResolve(t, geometry, "StartMassProperties", "Registry.Resolve(bodyHandle, \"Body\")", false)
	requireExecutionTimeResolve(t, geometry, "StartBoundingBox", "Registry.Resolve(bodyHandle, \"Body\")", false)
	requireExecutionTimeResolve(t, assembly, "StartAssemblyAddComponent", "Registry.Resolve(partHandle, \"Part\")", true)
	requireExecutionTimeResolve(t, assembly, "StartAssemblyRemoveComponent", "Registry.ResolveOwned(componentHandle, partHandle, \"Component\")", true)
	requireExecutionTimeResolve(t, assembly, "StartAssemblyQueryTree", "Registry.Resolve(partHandle, \"Part\")", false)
	requireExecutionTimeResolve(t, assembly, "StartAssemblyQueryBOM", "Registry.Resolve(partHandle, \"Part\")", false)

	for _, forbidden := range []string{
		"public sealed class NxExecutor",
		"public sealed class NamedPipeRequestServer",
		"private static string ExtractJsonString",
		"IndexOf(\"\\\"request_id\\\"\")",
	} {
		if strings.Contains(host, forbidden) || strings.Contains(geometry, forbidden) || strings.Contains(transactions, forbidden) || strings.Contains(assembly, forbidden) {
			t.Errorf("canonical NXHost reintroduced duplicated/manual runtime primitive: %q", forbidden)
		}
	}
	for _, forbidden := range []string{
		"massProps[0] * 1000000.0",
		"minMax[0] / 1000.0",
		"int units = 3",
	} {
		if strings.Contains(geometry, forbidden) {
			t.Errorf("canonical Geometry adapter reintroduced pre-hardening unit conversion: %q", forbidden)
		}
	}

	if !strings.Contains(build, "dotnet build $hostProject") ||
		!strings.Contains(build, "NXGO.Protocol.dll") ||
		!strings.Contains(build, "Newtonsoft.Json.dll") ||
		!strings.Contains(build, "NXGO.Agent.Core.dll") ||
		!strings.Contains(build, "NXGO.Agent.NXHost.dll") {
		t.Fatal("build-agent.ps1 must build and verify canonical Protocol/Core/NXHost runtime outputs")
	}
	for _, forbidden := range []string{
		"AgentWorker.dll",
		"Compiling transitional legacy AgentWorker",
		"Framework64\\v4.0.30319\\csc.exe",
		"$legacySrc",
	} {
		if strings.Contains(build, forbidden) {
			t.Errorf("build-agent.ps1 must not compile legacy AgentWorker: %q", forbidden)
		}
	}
}

func TestCanonicalHostUsesDeclaredMutationClassMap(t *testing.T) {
	root := repoRootForCompiledHostTest(t)
	host := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/EntryPoint.cs")
	if !strings.Contains(host, "MutationClasses") || !strings.Contains(host, "return MutationClasses.ContainsKey(operation);") {
		t.Fatal("canonical host must derive journal admission from the declared mutation class map")
	}
	for _, op := range []string{"part.new", "feature.create_block", "transaction.commit", "assembly.add_component", "drafting.export_pdf"} {
		if !strings.Contains(host, "[\""+op+"\"] = MutationOutcomeClass.") {
			t.Errorf("mutation operation %q has no declared outcome class", op)
		}
	}
	for _, op := range []string{"nx.ping", "session.info", "part.query_summary", "part.query_bodies", "geometry.query_mass_properties", "geometry.query_bounding_box", "assembly.query_tree", "assembly.query_bom", "drafting.query_sheets"} {
		if !strings.Contains(host, "\""+op+"\"") {
			t.Errorf("read-only operation %q is missing from the canonical operation registry", op)
		}
	}
}
