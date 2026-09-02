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

	if !strings.Contains(project, `ProjectReference Include="..\NXGO.Agent.Core\NXGO.Agent.Core.csproj"`) {
		t.Fatal("NXHost must consume NXGO.Agent.Core through a ProjectReference")
	}
	for _, marker := range []string{
		"using NXGO.Agent.Core;",
		"new NxExecutor()",
		"new NamedPipeRequestServer",
		"ProtocolMajor = 2",
		"session.info",
	} {
		if !strings.Contains(host, marker) {
			t.Errorf("canonical NXHost missing Core/v2 marker %q", marker)
		}
	}
	if strings.Contains(host, "public sealed class NxExecutor") || strings.Contains(host, "public sealed class NamedPipeRequestServer") {
		t.Fatal("NXHost must not reimplement Agent.Core transport/executor primitives")
	}

	if !strings.Contains(build, "dotnet build $hostProject") ||
		!strings.Contains(build, "NXGO.Agent.Core.dll") ||
		!strings.Contains(build, "NXGO.Agent.NXHost.dll") {
		t.Fatal("build-agent.ps1 must build and verify canonical Core/NXHost outputs")
	}
}
