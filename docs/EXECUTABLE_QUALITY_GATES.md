# Executable quality gates

Status: **normative implementation companion** to `ENGINEERING_STANDARD.md` and `TESTING_PLAYBOOK.md`.

The repository now contains executable scaffolding for the documented rules. A written rule is not considered fully enforced until it is mapped to static analysis, runtime guard, negative test, real-NX test, or CI gate.

## Implemented now

### Fast loop

`go run ./cmd/nxctl test fast` executes:

1. `go test -race ./...`;
2. `go vet ./...`;
3. `go run ./cmd/invariantcheck`.

GitHub Actions workflow: `.github/workflows/fast.yml`.

### Invariant gate

`cmd/invariantcheck` currently proves:

- required normative documents and workflows exist;
- the invariant catalog contains at least 40 stable unique IDs;
- public Go roots (`sdk/`, `pkg/` when introduced) do not import cgo or directly mention Siemens NXOpen DLL dependencies.

This checker is intentionally small and dependency-free. It must grow as implementation boundaries become concrete.

### Fake-Agent failure contracts

`internal/fakeagent` provides executable contracts for distributed-system failure semantics before the real Agent exists, including:

- lost response after a committed mutation;
- idempotent replay by request ID;
- poisoned-session rejection;
- replay soak boundedness;
- benchmark entry point.

### Session safety primitives

`internal/sessionhealth` makes terminal `Poisoned`/`Lost` states non-reusable in the current epoch.

`internal/objectref` makes session/epoch identity explicit and rejects stale references.

These packages are initial executable forms of `NXGO-INV-SES-001`, `NXGO-INV-OBJ-002`, `NXGO-INV-IPC-002/003/004`.

### Real Siemens NX smoke

`go run ./cmd/nxctl test nx` is fail-closed and requires:

- Windows;
- explicit `NXGO_NX_HOME`;
- a real `run_journal.exe` under that installation.

`scripts/nx-real-smoke.ps1` launches `tests/nx/smoke.py` through the Siemens runner. The journal imports `NXOpen`, obtains the live NX session, writes to the NX system log and creates an out-of-process marker. The command fails if the marker is absent, so a missing NX installation cannot produce a false green result.

GitHub Actions workflow: `.github/workflows/nx-self-hosted.yml`, intentionally limited to an authorized self-hosted Windows runner.

## Command surface

```text
nxctl test fast
nxctl test nx
nxctl test matrix
nxctl test chaos
nxctl test soak
nxctl test perf
```

`matrix` requires `NXGO_NX_MATRIX` with semicolon-separated installation roots and executes the real-NX smoke against each entry.

Current `chaos`, `soak` and `perf` commands exercise the Fake Agent. They are placeholders for the corresponding real-NX campaigns and MUST NOT be cited as evidence of NX kernel/session recovery until real-NX drivers are added.

## Next enforcement steps

1. Introduce the .NET NX Agent skeleton and architecture tests that prohibit direct NXOpen calls outside `NxExecutor`.
2. Add `BuilderScope<T>` and tests proving destroy/dispose on every exit path.
3. Add structured invariant-compliance metadata mapping all P0/P1 rules to tests.
4. Add a warm real-NX worker/fixture-reset protocol.
5. Add isolated NX process tests for kill, timeout, poison and ambiguous commit-response loss.
6. Add semantic CAD fixture assertions and Check-Mate adapter.
7. Add API-manifest differential tests for 2512 vs 2606.
8. Add mutation campaigns (Go decision logic and C# Agent safety primitives).
9. Add long-running handle/callback/builder leak soak tests.

## Evidence rule

CI/test reports MUST distinguish:

- simulated/fake behavior;
- real NXOpen execution;
- exact NX release/build;
- session health before/after;
- semantic CAD oracle used;
- artifacts captured.

A Fake-Agent pass must never be labeled a Siemens NX integration pass.
