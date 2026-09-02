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

func TestCanonicalCompiledHostMigrationLaneIsWired(t *testing.T) {
	root := repoRootForCompiledHostTest(t)
	bootstrap := readRepoFile(t, root, "agent/bundle/CompiledHostBootstrap.cs")
	host := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/EntryPoint.cs")
	geometry := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/EntryPoint.Geometry.cs")
	project := readRepoFile(t, root, "agent/NXGO.Agent.NXHost/NXGO.Agent.NXHost.csproj")
	build := readRepoFile(t, root, "scripts/build-agent.ps1")

	for _, marker := range []string{
		"NXGO_AGENT_BIN",
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

	if !strings.Contains(project, "<ProjectReference") || !strings.Contains(project, "NXGO.Agent.Core") {
		t.Fatal("NXHost must consume NXGO.Agent.Core through a ProjectReference")
	}
	if !strings.Contains(project, "System.Web.Extensions") {
		t.Fatal("canonical NXHost must use the framework JSON serializer rather than manual JSON slicing")
	}
	for _, marker := range []string{
		"using NXGO.Agent.Core;",
		"new NxExecutor()",
		"new NamedPipeRequestServer",
		"HandleRegistry<TaggedObject>",
		"new RequestJournal",
		"JavaScriptSerializer",
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
		"Registry.Register(body, \"Body\", requestId)",
		"StartObjectRelease",
	} {
		if !strings.Contains(geometry, marker) {
			t.Errorf("canonical Geometry adapter missing Core/lifetime marker %q", marker)
		}
	}
	for _, forbidden := range []string{
		"public sealed class NxExecutor",
		"public sealed class NamedPipeRequestServer",
		"private static string ExtractJsonString",
		"IndexOf(\"\\\"request_id\\\"\")",
	} {
		if strings.Contains(host, forbidden) || strings.Contains(geometry, forbidden) {
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
		!strings.Contains(build, "NXGO.Agent.Core.dll") ||
		!strings.Contains(build, "NXGO.Agent.NXHost.dll") {
		t.Fatal("build-agent.ps1 must build and verify canonical Core/NXHost outputs")
	}
}
