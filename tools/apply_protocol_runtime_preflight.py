#!/usr/bin/env python3
from pathlib import Path


def replace_once(path: Path, old: str, new: str, label: str) -> None:
    text = path.read_text(encoding='utf-8')
    count = text.count(old)
    if count != 1:
        raise SystemExit(f'{label}: expected exactly one source snippet, got {count}')
    path.write_text(text.replace(old, new, 1), encoding='utf-8')


worker = Path('internal/supervisor/worker.go')
replace_once(
    worker,
    'for _, dll := range []string{"NXGO.Agent.Core.dll", "NXGO.Agent.NXHost.dll"} {',
    'for _, dll := range []string{"Newtonsoft.Json.dll", "NXGO.Protocol.dll", "NXGO.Agent.Core.dll", "NXGO.Agent.NXHost.dll"} {',
    'supervisor canonical runtime list',
)

nxctl = Path('cmd/nxctl/main.go')
replace_once(
    nxctl,
    '''\t\tif _, err := exec.LookPath("dotnet"); err != nil {\n\t\t\treturn errors.New("dotnet SDK is required by the canonical fast gate because NXGO includes the NX-independent Agent core")\n\t\t}\n\t\treturn runCmd(ctx, "dotnet", "test", "agent/NXGO.Agent.Core.Tests/NXGO.Agent.Core.Tests.csproj", "-c", "Release", "--nologo")\n''',
    '''\t\tif _, err := exec.LookPath("dotnet"); err != nil {\n\t\t\treturn errors.New("dotnet SDK is required by the canonical fast gate because NXGO includes the NX-independent Protocol and Agent core")\n\t\t}\n\t\tif err := runCmd(ctx, "dotnet", "test", "agent/NXGO.Protocol.Tests/NXGO.Protocol.Tests.csproj", "-c", "Release", "--nologo"); err != nil { return err }\n\t\treturn runCmd(ctx, "dotnet", "test", "agent/NXGO.Agent.Core.Tests/NXGO.Agent.Core.Tests.csproj", "-c", "Release", "--nologo")\n''',
    'nxctl fast protocol gate',
)

fixture = Path('tests/nx/compiled_host_test.go')
replace_once(
    fixture,
    '''// run_journal -> minimal CompiledHostBootstrap.cs -> NXGO.Agent.NXHost.dll ->\n// NXGO.Agent.Core.dll -> shared NxExecutor/RequestJournal/HandleRegistry on the\n''',
    '''// supervisor canonical mode -> run_journal -> minimal CompiledHostBootstrap.cs ->\n// Newtonsoft.Json.dll -> NXGO.Protocol.dll -> NXGO.Agent.Core.dll ->\n// NXGO.Agent.NXHost.dll -> shared NxExecutor/RequestJournal/HandleRegistry on the\n''',
    'compiled host dependency comment',
)
replace_once(
    fixture,
    '''\tcoreDLL := filepath.Join(agentBin, "NXGO.Agent.Core.dll")\n\thostDLL := filepath.Join(agentBin, "NXGO.Agent.NXHost.dll")\n\tmissing := ""\n\tif _, err := os.Stat(coreDLL); err != nil {\n\t\tmissing = coreDLL\n\t}\n\tif _, err := os.Stat(hostDLL); err != nil {\n\t\tmissing = hostDLL\n\t}\n''',
    '''\tmissing := ""\n\tfor _, dll := range []string{"Newtonsoft.Json.dll", "NXGO.Protocol.dll", "NXGO.Agent.Core.dll", "NXGO.Agent.NXHost.dll"} {\n\t\tpath := filepath.Join(agentBin, dll)\n\t\tif _, err := os.Stat(path); err != nil {\n\t\t\tmissing = path\n\t\t\tbreak\n\t\t}\n\t}\n''',
    'compiled host runtime dependency check',
)
replace_once(
    fixture,
    '''\tworker, err := supervisor.StartWorker(ctx, supervisor.WorkerConfig{\n\t\tNXHome:         getNXHome(t),\n\t\tJournalPath:    bootstrap,\n\t\tStartupTimeout: 45 * time.Second,\n\t})\n''',
    '''\tworker, err := supervisor.StartWorker(ctx, supervisor.WorkerConfig{\n\t\tNXHome:         getNXHome(t),\n\t\tAgentMode:      supervisor.AgentModeCanonical,\n\t\tAgentBin:       agentBin,\n\t\tStartupTimeout: 45 * time.Second,\n\t})\n''',
    'compiled host supervisor canonical mode',
)

for path, markers in {
    worker: ('Newtonsoft.Json.dll', 'NXGO.Protocol.dll'),
    nxctl: ('NXGO.Protocol.Tests/NXGO.Protocol.Tests.csproj', 'NXGO.Agent.Core.Tests/NXGO.Agent.Core.Tests.csproj'),
    fixture: ('supervisor.AgentModeCanonical', 'Newtonsoft.Json.dll', 'NXGO.Protocol.dll'),
}.items():
    text = path.read_text(encoding='utf-8')
    for marker in markers:
        if marker not in text:
            raise SystemExit(f'{path}: required marker missing after patch: {marker}')

print('canonical protocol runtime preflight migration applied')
