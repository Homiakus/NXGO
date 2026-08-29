# `nxctl` CLI specification

`nxctl` is the Go supervisor/operator interface. It is useful independently of the Go SDK and is the process owner for worker/test mode.

## 1. Commands

### Discovery

```text
nxctl installations
nxctl status
nxctl sessions
```

Shows exact release/build, install path, Agent availability, PID, mode and health.

### Start/stop

```text
nxctl start --version 2606 --mode worker
nxctl attach --pid 12345
nxctl stop --session <id>
```

Stop refuses to terminate user-owned interactive NX unless explicitly forced.

### Logs

```text
nxctl logs --follow
nxctl logs --session <id> --level warn
nxctl diagnose --run <run-id>
```

Merged log stream labels source (`nx`, `agent`, `runner`).

### Tests

```text
nxctl test
nxctl test --nx 2512,2606
nxctl test --case drawing/basic
```

Produces machine-readable and human reports plus failure artifact bundles.

### API inspection

```text
nxctl api scan --version 2606
nxctl api diff 2512 2606
nxctl api find "projected view"
nxctl api inspect NXOpen.Drawings.ProjectedViewBuilder
```

### Doctor

```text
nxctl doctor
```

Checks:

- supported NX installations;
- Programming Tools/NXOpen availability;
- expected assemblies;
- Agent installation;
- writable temp/log directories;
- license startup smoke test when requested;
- protocol compatibility.

## 2. Output

Human output is concise; `--json` returns stable machine-readable schemas. Errors return documented exit codes.

## 3. Exit codes

Suggested:

```text
0 success
2 usage/configuration
10 NX not found
11 Agent unavailable
12 incompatible versions
20 NX operation failed
21 NX crash/fatal error
22 timeout
23 license unavailable
30 test failure
40 infrastructure failure
```

## 4. Configuration precedence

1. CLI flags;
2. environment variables;
3. project config;
4. user config;
5. discovered defaults.

`nxctl config explain` SHOULD show the resolved value and origin for diagnostics.

## 5. Worker ownership

Supervisor records a process manifest containing PID, NX build, workspace, endpoint, startup time and ownership. It uses job/process containment mechanisms where practical so orphan worker cleanup is reliable.

## 6. UX rule

Every command SHOULD support `--json` before v1 so automation does not need to scrape terminal text.