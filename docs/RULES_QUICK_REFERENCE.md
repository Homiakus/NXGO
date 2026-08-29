# NXGO engineering rules — quick reference

Status: **normative summary**. This file is the daily checklist. Detailed rationale and enforcement live in [ENGINEERING_STANDARD.md](ENGINEERING_STANDARD.md), [TESTING_PLAYBOOK.md](TESTING_PLAYBOOK.md), and the [invariant catalog](invariants/README.md).

## 1. Architecture

1. Public Go SDK MUST remain Pure Go and MUST NOT reference Siemens binaries.
2. NXOpen/NXOpen.UF calls MUST stay inside the NX Agent boundary.
3. Transport DTOs, Siemens types, generated raw types and public domain types MUST remain distinct layers.
4. High-level APIs MUST hide NX Builder lifecycle, session/thread restrictions, version adapters and object registry details.
5. Raw/UF/journal escape hatches MUST NOT dictate high-level API design.
6. Architectural exceptions require an ADR and replacement safety mechanism.

## 2. NX execution

1. Transport/background threads MUST NOT call NXOpen directly (`NXGO-INV-EXEC-001`).
2. One NX session is serialized through a reentrancy-aware NX executor by default.
3. Go goroutines MAY work concurrently outside NX, but MUST NOT imply concurrent mutation of one NX session.
4. Interactive state, Work Part, Display Part and Work Component MUST be modeled explicitly.
5. Headless-safe, graphics-required, interactive-required and managed-mode capabilities MUST be distinguished.

## 3. NX objects and mutation

1. Live Siemens/.NET objects MUST NOT cross IPC.
2. Every remote object handle MUST carry session identity/epoch and become invalid after restart.
3. Temporary handles MUST have bounded scope/lease and cleanup.
4. Every Builder MUST be destroyed on every path.
5. A Builder MUST NOT be reused after failed validation/Commit.
6. Mutation code MUST account for NX update semantics; Commit alone is not proof of updated model state.
7. Undo marks MUST NOT be presented as cross-system ACID transactions.
8. Suspect/poisoned sessions MUST be quarantined/recycled, never silently reused.

## 4. Public Go API

1. Prefer domain intent over NX implementation mechanics.
2. Blocking/remote operations accept `context.Context`.
3. Dimensioned values MUST use explicit unit-aware types; naked `float64` is forbidden for public dimensional inputs.
4. Normal workflows SHOULD be coarse/bulk operations; avoid N+1 RPC.
5. Public errors MUST have stable NXGO semantics and preserve native diagnostic context.
6. Mutation retry MUST be explicit and idempotency-aware; blind retries are forbidden.
7. `Part`, `Assembly`, `Drawing`, etc. returned by SDK are remote proxies, not durable identities across sessions.
8. High-level operations MUST define semantic postconditions where engineering correctness can be checked.

## 5. Agent/C# code

1. All NX calls pass through the NX-safe executor.
2. Builder use goes through common safe scopes/helpers.
3. NX callbacks/subscriptions have explicit owners and unregister paths.
4. Release-specific behavior lives in adapters/capabilities, not scattered conditionals.
5. Agent target runtime/compiler follows the supported NX release requirements.
6. Long-lived Agent code MUST use an unload lifecycle compatible with registered callbacks/services.
7. Native NX exceptions are classified into recovery semantics: ordinary, rollback-required, suspect, poisoned, crash.

## 6. Protocol/IPC

1. Protocol is typed and explicitly versioned.
2. Unknown compatible fields/capabilities are tolerated according to protocol version rules.
3. Old handles are never revived by reconnect.
4. Cancellation of a Go context MUST NOT force-abort arbitrary running NX code.
5. Every request has correlation identity; retryable mutations additionally have idempotency identity.
6. Local IPC is secured to the intended user/session by default.
7. Batch messages MUST be bounded; malformed/oversized input must fail closed.

## 7. Assemblies, files and licenses

1. Partial assembly load MUST never be silently treated as complete.
2. `PartLoadStatus`/load warnings MUST be preserved and surfaced.
3. NX startup does not prove that a feature license is available.
4. Native filesystem and Teamcenter-managed semantics MUST remain separate.
5. File APIs MUST define overwrite, existing-open-part, permission, encoding/path and atomic-output behavior.
6. Release artifacts SHOULD be staged and atomically promoted after validation.

## 8. Code generation

1. Generated code is never manually edited.
2. Generation is deterministic and traceable to a specific NX release/build manifest.
3. Scanner/codegen MUST model lifecycle, `ref/out`, arrays, enums, callbacks and disposable/transient classes—not signatures only.
4. NX API compatibility MUST NOT be assumed between Continuous Release builds.
5. A new NX release is supported only after manifest diff + compile + real-NX regression matrix.

## 9. Observability

1. NX syslog, Agent structured log, runner log, stdout/stderr and journal output remain distinguishable sources.
2. All sources share run/request/test correlation IDs.
3. Fatal/suspect failures MUST preserve diagnostics before worker recycle.
4. Logs MUST NOT become the only correctness oracle; semantic state must be asserted.
5. Sensitive paths/data are redacted according to security policy.

## 10. Testing — mandatory philosophy

1. Mock-only confidence is insufficient for NX-dependent behavior.
2. Real NX MUST be part of the testing loop for affected NX behavior.
3. Tests use the cheapest valid layer first, then real NX where the behavior crosses the NX boundary.
4. Semantic CAD checks are primary; screenshots/visual goldens are supplementary.
5. Every recovery mechanism requires a negative/fault test.
6. Critical decision logic (error classification, retries, units, session health, validators) SHOULD be mutation-tested.
7. Parsers/protocol boundaries SHOULD be fuzz-tested.
8. Stateful lifecycle logic SHOULD use property/model-based testing.
9. Version compatibility SHOULD use differential testing across supported NX releases.
10. Long-lived Agent/worker behavior requires soak/resource-leak testing.

## 11. Required testing loops

- **Fast loop**: formatting, static analysis, unit/table/property tests, protocol/fake Agent, selected fuzz corpus.
- **Warm NX loop**: affected real-NX integration tests against a reusable controlled worker.
- **Isolated NX loop**: one fresh NX process for crash, poison, startup, license, modal/UI and destructive recovery cases.
- **Matrix loop**: supported pinned NX builds/modes/units.
- **Nightly/release loop**: mutation, extended fuzz, differential, chaos, soak, performance and compatibility campaigns.

See [TESTING_PLAYBOOK.md](TESTING_PLAYBOOK.md).

## 12. Merge blockers

A change MUST NOT merge when any applicable condition is true:

- P0/P1 invariant violation;
- new NX behavior has no real-NX test;
- mutation can be retried but idempotency semantics are undefined;
- public dimensional input is unitless;
- severe NX failure can leave a worker marked healthy;
- returned handles can survive/revive across session epoch change;
- generated/raw NX detail leaks into public domain API without explicit design review;
- assembly-wide result ignores incomplete loading;
- correctness is asserted only by screenshot/PDF pixels;
- new supported NX release has not passed the real-NX compatibility matrix;
- changed behavior lacks diagnostics/correlation needed to reproduce failure.

## 13. Definition of done

Use [DEFINITION_OF_DONE.md](DEFINITION_OF_DONE.md). In short: code + tests + real-NX evidence where applicable + invariant compliance + diagnostics + docs + deterministic cleanup + compatibility impact are one deliverable, not separate follow-up work.