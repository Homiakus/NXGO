# Executable quality gates

Status: **normative implementation companion** to `ENGINEERING_STANDARD.md` and `TESTING_PLAYBOOK.md`.

The repository now contains executable scaffolding for the documented rules. A written rule is not considered fully enforced until it is mapped to static analysis, runtime guard, negative test, real-NX test, or CI gate.

## Implemented now

### Fast loop

`go run ./cmd/nxctl test fast` executes:

1. `go test -race ./...`;
2. `CGO_ENABLED=0 go test ./...`;
3. `go vet ./...`;
4. `go run ./cmd/invariantcheck`.

GitHub Actions workflow: `.github/workflows/fast.yml` calls this canonical command so local and CI behavior do not drift.

### Invariant gate

`cmd/invariantcheck` currently proves:

- required normative documents and workflows exist;
- the invariant catalog contains at least 40 stable unique IDs;
- `policy/invariant-compliance.json` is valid, references only known invariant IDs and points enforced mechanisms at real repository paths;
- public Go roots (`sdk/`, `pkg/` when introduced) do not import cgo or directly mention Siemens NXOpen DLL dependencies.

The compliance file distinguishes `enforced`, `partially_enforced`, and `planned`. A partially enforced Fake-Agent rule MUST NOT be promoted to `enforced` merely because the simulated path passes.

### Model/state-machine and fuzz checks

`internal/sessionhealth/model_test.go` exhaustively explores short event sequences through the session-health automaton and proves that `Poisoned`/`Lost` states cannot re-enter service in-place.

`internal/objectref/objectref_fuzz_test.go` defines a native Go fuzz target proving that a handle created in one session epoch is never valid in another epoch.

Run a bounded fuzz campaign with:

```text
go run ./cmd/nxctl test fuzz
```

`NXGO_FUZZTIME` controls duration and defaults to `30s`. `.github/workflows/campaigns.yml` runs a longer scheduled/manual campaign.

### Fake-Agent failure contracts

`internal/fakeagent` provides executable contracts for distributed-system failure semantics before the real Agent exists, including:

- lost response after a committed mutation;
- idempotent replay by request ID;
- poisoned-session rejection;
- replay soak boundedness;
- benchmark entry point.

These are simulation tests. They validate NXGO protocol/recovery logic, not Siemens NX kernel behavior.

### Session safety primitives

`internal/sessionhealth` makes terminal `Poisoned`/`Lost` states non-reusable in the current epoch.

`internal/objectref` makes session/epoch/generation identity explicit and rejects stale references.

These packages are initial executable forms of `NXGO-INV-SES-001`, `NXGO-INV-OBJ-002`, `NXGO-INV-IPC-002/003/004`.

### Real Siemens NX smoke

`go run ./cmd/nxctl test nx` is fail-closed and requires:

- Windows;
- explicit `NXGO_NX_HOME`;
- a real `run_journal.exe` under that installation.

`scripts/nx-real-smoke.ps1` launches `tests/nx/smoke.py` through the Siemens runner. The journal imports `NXOpen`, obtains the live NX session, writes to the NX system log and creates an out-of-process marker. The command fails if the marker is absent, so a missing NX installation cannot produce a false green result.

GitHub Actions workflow: `.github/workflows/nx-self-hosted.yml`, intentionally limited to an authorized self-hosted Windows runner.

This smoke proves only that real NX/NXOpen execution is reachable. It does not yet prove the future NXGO Agent, safe executor, semantic CAD behavior, recovery model or supported release matrix.

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

Current `chaos`, `soak` and `perf` commands exercise the Fake Agent. They are placeholders for the corresponding real-NX campaigns and MUST NOT be cited as evidence of NX kernel/session recovery until real-NX drivers are added.

## CI workflows

- `.github/workflows/fast.yml` — required fast/no-NX quality gate on pushes/PRs.
- `.github/workflows/campaigns.yml` — scheduled/manual fuzz + simulated chaos/soak/performance campaigns.
- `.github/workflows/nx-self-hosted.yml` — manual real-NX smoke on an authorized self-hosted Windows runner.

## Current verified evidence

The first `fast-quality-gates` run after introducing the executable infrastructure completed successfully on GitHub Actions, including race-enabled tests, `go vet`, invariant checking and Fake-Agent chaos. Real Siemens NX execution has **not** been claimed from public CI because the required licensed Windows runner is external to the repository.

## Next enforcement steps

1. Introduce the .NET NX Agent skeleton and architecture tests that prohibit direct NXOpen calls outside `NxExecutor`.
2. Add `BuilderScope<T>` and tests proving destroy/dispose on every exit path.
3. Expand invariant-compliance metadata toward every implemented P0/P1 rule.
4. Add a warm real-NX worker/fixture-reset protocol.
5. Add isolated NX process tests for kill, timeout, poison and ambiguous commit-response loss.
6. Add semantic CAD fixture assertions and Check-Mate adapter.
7. Add API-manifest differential tests for 2512 vs 2606.
8. Add mutation campaigns (Go decision logic and C# Agent safety primitives).
9. Add metamorphic CAD tests and direct-NXOpen/NXGO differential cases.
10. Add long-running handle/callback/builder leak soak tests.

## Evidence rule

CI/test reports MUST distinguish:

- simulated/fake behavior;
- real NXOpen execution;
- exact NX release/build;
- session health before/after;
- semantic CAD oracle used;
- artifacts captured.

A Fake-Agent pass must never be labeled a Siemens NX integration pass.
