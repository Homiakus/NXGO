# Executable quality gates

Status: **normative implementation companion** to `ENGINEERING_STANDARD.md` and `TESTING_PLAYBOOK.md`.

The repository contains executable enforcement for a growing subset of the documented rules. A written rule is not considered fully enforced until it is mapped to static analysis, runtime guard, negative test, real-NX test, or CI gate.

## Implemented now

### Fast loop

`go run ./cmd/nxctl test fast` executes:

1. `go test -race ./...`;
2. `CGO_ENABLED=0 go test ./...`;
3. `go vet ./...`;
4. `go run ./cmd/invariantcheck`.

GitHub Actions workflow `.github/workflows/fast.yml` calls this canonical Go command and additionally runs the NX-independent .NET Agent contract suite:

```text
dotnet test agent/NXGO.Agent.Core.Tests/NXGO.Agent.Core.Tests.csproj -c Release
```

This keeps local/CI Go behavior aligned while proving Agent safety primitives without redistributing Siemens binaries.

### Invariant gate

`cmd/invariantcheck` currently proves:

- required normative documents and workflows exist;
- the invariant catalog contains at least 40 stable unique IDs;
- `policy/invariant-compliance.json` is valid, references only known invariant IDs and points enforced mechanisms at real repository paths;
- public Go roots (`sdk/`, `pkg/` when introduced) do not import cgo or directly mention Siemens NXOpen DLL dependencies.

The compliance file distinguishes `enforced`, `partially_enforced`, and `planned`. A partially enforced core/Fake-Agent rule MUST NOT be promoted to full real-NX evidence merely because simulation or NX-independent contracts pass.

### NX-independent .NET Agent core

`agent/NXGO.Agent.Core` targets `netstandard2.0` and contains no Siemens assembly references. Ordinary GitHub Actions therefore compiles and tests the most failure-sensitive bridge mechanics before an NX machine is involved.

Implemented primitives:

- `NxExecutor` — transport/background threads enqueue work; only the explicitly bound execution thread may drain/execute it;
- `BuilderScope<T>` — one commit attempt per builder scope and unconditional caller-supplied destroy action on disposal;
- `SessionHealthState` — non-healthy sessions are not reusable;
- `FrameCodec` — bounded 4-byte little-endian framing;
- `NamedPipeRequestServer` — local IPC server whose NX-dependent handlers are required to enqueue through `NxExecutor`.

The contract suite proves:

- successful and failed builder paths both destroy resources;
- the same failed builder scope cannot be committed again;
- queued work does not execute on the producer/transport thread;
- wrong-thread queue draining is rejected;
- queued cancellation is observed before execution;
- terminal/suspect session state is not reusable;
- framing rejects malformed length data;
- Go and C# share the same `ping` golden frame (`04 00 00 00 70 69 6e 67`);
- a real local named-pipe round trip succeeds without NX.

This is executable evidence for the **core boundary** of `NXGO-INV-EXEC-001/002`, `NXGO-INV-MUT-001/002` and `NXGO-INV-SES-001`. Real NX adapters still require pinned NX evidence before those rules can be claimed end-to-end.

### Dedicated NX worker host skeleton

`agent/NXGO.Agent.NXHost` targets .NET 8 and is launched by the installed NX2512 `NXBIN\managed_core\run_dotnet_core_nxopen.exe` against its matching `NXGO.Agent.NXHost.dll`. NXOpen references come only from the installed `managed_core` directory supplied through `NXGO_NX_MANAGED`.

The current worker-host design:

```text
NX entry thread
    -> run_dotnet_core_nxopen.exe NXGO.Agent.NXHost.dll
    -> Program.Main -> Session.GetSession()
    -> NxExecutor.BindToCurrentThread()
    -> start named-pipe transport on background task
    -> drain queued NX work on NX entry thread
```

It intentionally supports **dedicated worker mode only**. It MUST NOT be reused as an interactive attach implementation because an infinite worker pump would monopolize the interactive NX entry thread. Interactive attach remains blocked until a Siemens-supported main-thread callback/pump adapter is implemented and tested.

The bootstrap operations `ping`, `nx.ping`, and `shutdown` are implementation scaffolding, not the stable public protocol. The typed/protobuf contract described in `PROTOCOL.md` remains the public target.

`scripts/build-agent.ps1` fails closed unless `NXGO_NX_MANAGED` points at a real installed NX managed assembly directory. No Siemens binary is committed.

### Model/state-machine and fuzz checks

`internal/sessionhealth/model_test.go` exhaustively explores short event sequences through the session-health automaton and proves that `Poisoned`/`Lost` states cannot re-enter service in-place.

`internal/objectref/objectref_fuzz_test.go` defines a native Go fuzz target proving that a handle created in one session epoch is never valid in another epoch.

`internal/protocol/frame_test.go` adds a cross-language framing golden and fuzz target.

Run a bounded fuzz campaign with:

```text
go run ./cmd/nxctl test fuzz
```

`NXGO_FUZZTIME` controls duration and defaults to `30s`. `.github/workflows/campaigns.yml` runs a longer scheduled/manual campaign.

### Fake-Agent failure contracts

`internal/fakeagent` provides executable contracts for distributed-system failure semantics before the production Agent exists, including:

- lost response after a committed mutation;
- idempotent replay by request ID;
- poisoned-session rejection;
- replay soak boundedness;
- benchmark entry point.

These are simulation tests. They validate NXGO protocol/recovery logic, not Siemens NX kernel behavior.

### Go session/object safety primitives

`internal/sessionhealth` makes terminal `Poisoned`/`Lost` states non-reusable in the current epoch.

`internal/objectref` makes session/epoch/generation identity explicit and rejects stale references.

These packages are executable forms of `NXGO-INV-SES-001`, `NXGO-INV-OBJ-002`, and parts of `NXGO-INV-IPC-002/003/004`.

### Real Siemens NX smoke

`go run ./cmd/nxctl test nx` is fail-closed and requires:

- Windows;
- explicit `NXGO_NX_HOME`;
- a real `run_journal.exe` under that installation.

`scripts/nx-real-smoke.ps1` launches `tests/nx/smoke.py` through the Siemens runner. The journal imports `NXOpen`, obtains the live NX session, writes to the NX system log and creates an out-of-process marker. The command fails if the marker is absent, so a missing NX installation cannot produce a false green result.

GitHub Actions workflow `.github/workflows/nx-self-hosted.yml` is intentionally limited to an authorized self-hosted Windows runner.

This smoke proves only that real NX/NXOpen execution is reachable. Until the new NXHost is built and exercised on a pinned licensed installation, it does **not** prove Agent startup, safe executor behavior inside NX, semantic CAD behavior, recovery, or release compatibility.

## Command surface

```text
nxctl test fast
nxctl test fuzz
nxctl test nx
nxctl test matrix
nxctl test chaos
nxctl test soak
nxctl test perf
```

`matrix` requires `NXGO_NX_MATRIX` with semicolon-separated installation roots and executes the real-NX smoke against each entry.

Current `chaos`, `soak` and `perf` commands exercise the Fake Agent. They MUST NOT be cited as evidence of NX kernel/session recovery until real-NX drivers are added.

## CI workflows

- `.github/workflows/fast.yml` — required Go + NX-independent .NET Agent quality gate on pushes/PRs;
- `.github/workflows/campaigns.yml` — scheduled/manual fuzz + simulated chaos/soak/performance campaigns;
- `.github/workflows/nx-self-hosted.yml` — manual real-NX smoke on an authorized self-hosted Windows runner.

## Current verified evidence

Verified on GitHub Actions:

- Go race-enabled tests;
- Pure-Go `CGO_ENABLED=0` tests;
- `go vet`;
- invariant/compliance checking;
- Fake-Agent chaos contract;
- `NXGO.Agent.Core` compilation as `netstandard2.0`;
- .NET Agent Core contracts, including named-pipe roundtrip and executor/builder safety;
- cross-language bootstrap-frame golden.

Real Siemens NX Agent execution has **not** been claimed from public CI because the required licensed Windows/NX runner is external to the repository.

## Next enforcement steps

1. Build and run `NXGO.Agent.NXHost` on the pinned authorized NX worker and prove `nx.ping` executes on the bound NX thread.
2. Add a typed request/response envelope, handshake, protocol/capability negotiation and stable errors; retire bootstrap string payloads.
3. Add architecture/static analysis that rejects NXOpen references outside approved NXHost/release-adapter namespaces.
4. Implement explicit callback reentrancy/call-depth policy for the executor.
5. Add real NX `BuilderScope<T>` adapters and first model mutation recipe with update + semantic postcondition.
6. Add a warm real-NX worker/fixture-reset protocol.
7. Add isolated NX tests for kill, timeout, poison and ambiguous commit-response loss.
8. Add semantic CAD fixture assertions and Check-Mate adapter.
9. Add API-manifest differential tests for 2512 vs 2606.
10. Add mutation campaigns for Go decision logic and C# Agent safety primitives.
11. Add metamorphic CAD tests and direct-NXOpen/NXGO differential cases.
12. Add long-running handle/callback/builder leak soak tests.

## Evidence rule

CI/test reports MUST distinguish:

- simulated/fake behavior;
- NX-independent Agent-core behavior;
- real NXOpen execution;
- exact NX release/build;
- session health before/after;
- semantic CAD oracle used;
- artifacts captured.

A Fake-Agent or Agent-Core pass must never be labeled a Siemens NX integration pass.
