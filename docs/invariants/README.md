# NXGO Programming Invariants

Status: **normative**. These rules define how NXGO MUST and MUST NOT be implemented. A change that intentionally violates an invariant requires an accepted ADR and replacement safety mechanism.

Each invariant has a stable ID for code review, tests, log events and CI failures. Example: `NXGO-INV-EXEC-001`.

## Rule format

Every detailed invariant uses:

- **MUST NOT** — forbidden implementation pattern;
- **MUST** — required safe pattern;
- **Why** — failure mode seen in NX/NXOpen practice;
- **Enforcement** — how architecture/tooling prevents the mistake;
- **Tests** — minimum proof that the invariant holds.

## Mandatory invariant catalog

| ID | Short rule | Detail |
|---|---|---|
| NXGO-INV-EXEC-001 | Transport/background threads never call NXOpen directly | [Execution](EXECUTION.md) |
| NXGO-INV-EXEC-002 | NX execution serialization must be reentrancy-aware | [Execution](EXECUTION.md) |
| NXGO-INV-EXEC-003 | Parallel Go work never implies parallel mutation of one NX session | [Execution](EXECUTION.md) |
| NXGO-INV-OBJ-001 | Remote handles are never trusted after object death | [Objects](OBJECTS_AND_LIFETIME.md) |
| NXGO-INV-OBJ-002 | Every remote handle is bound to session epoch/generation | [Objects](OBJECTS_AND_LIFETIME.md) |
| NXGO-INV-OBJ-003 | Siemens/.NET live objects never cross the IPC boundary | [Objects](OBJECTS_AND_LIFETIME.md) |
| NXGO-INV-OBJ-004 | Temporary handles have bounded scopes/leases | [Objects](OBJECTS_AND_LIFETIME.md) |
| NXGO-INV-MUT-001 | Every Builder is destroyed on every exit path | [Mutation](MUTATION_AND_RECOVERY.md) |
| NXGO-INV-MUT-002 | A Builder is never reused after failed Commit/validation | [Mutation](MUTATION_AND_RECOVERY.md) |
| NXGO-INV-MUT-003 | Mutations do not assume Commit means fully updated model | [Mutation](MUTATION_AND_RECOVERY.md) |
| NXGO-INV-MUT-004 | Undo marks are not advertised as cross-system ACID transactions | [Mutation](MUTATION_AND_RECOVERY.md) |
| NXGO-INV-SES-001 | Poisoned/suspect NX sessions are never reused | [Mutation](MUTATION_AND_RECOVERY.md) |
| NXGO-INV-SES-002 | Errors are classified by recovery semantics, not only message/code | [Mutation](MUTATION_AND_RECOVERY.md) |
| NXGO-INV-JRN-001 | Recorded `FindObject` identifiers are not production selectors | [Journals/UI](JOURNALS_AND_UI.md) |
| NXGO-INV-JRN-002 | Recorded journals are recipes, not production source of truth | [Journals/UI](JOURNALS_AND_UI.md) |
| NXGO-INV-UI-001 | Headless capability is never inferred from GUI success | [Journals/UI](JOURNALS_AND_UI.md) |
| NXGO-INV-UI-002 | CI/worker flows do not depend on coordinate/ribbon automation | [Journals/UI](JOURNALS_AND_UI.md) |
| NXGO-INV-UI-003 | Potential modal UI is denied or explicitly modeled in workers | [Journals/UI](JOURNALS_AND_UI.md) |
| NXGO-INV-VER-001 | NX API compatibility is never assumed across releases | [Versions](VERSIONS_AND_RUNTIME.md) |
| NXGO-INV-VER-002 | Generated API is release-manifest driven, not single-version generated | [Versions](VERSIONS_AND_RUNTIME.md) |
| NXGO-INV-VER-003 | Agent runtime/compiler target follows supported NX runtime | [Versions](VERSIONS_AND_RUNTIME.md) |
| NXGO-INV-VER-004 | Long-lived Agent lifecycle does not use unsafe immediate unload | [Versions](VERSIONS_AND_RUNTIME.md) |
| NXGO-INV-VER-005 | Every registered NX callback is unregisterable and owned | [Versions](VERSIONS_AND_RUNTIME.md) |
| NXGO-INV-STATE-001 | Work Part, Display Part and Work Component are never conflated | [Assembly/state](ASSEMBLY_STATE_AND_LICENSES.md) |
| NXGO-INV-ASM-001 | Assembly analysis never silently accepts unknown partial-load completeness | [Assembly/state](ASSEMBLY_STATE_AND_LICENSES.md) |
| NXGO-INV-ASM-002 | `PartLoadStatus`/load issues are never discarded | [Assembly/state](ASSEMBLY_STATE_AND_LICENSES.md) |
| NXGO-INV-CAP-001 | Process startup is not proof that a licensed capability exists | [Assembly/state](ASSEMBLY_STATE_AND_LICENSES.md) |
| NXGO-INV-STATE-002 | Interactive requests do not ignore current NX application/busy state | [Assembly/state](ASSEMBLY_STATE_AND_LICENSES.md) |
| NXGO-INV-TCM-001 | Managed/Teamcenter state is not disguised as native filesystem semantics | [Assembly/state](ASSEMBLY_STATE_AND_LICENSES.md) |
| NXGO-INV-COR-001 | NX success/absence of exception is not proof of engineering correctness | [Correctness](CORRECTNESS_UNITS_IO.md) |
| NXGO-INV-COR-002 | Dimensioned values never cross domain API as unitless numbers | [Correctness](CORRECTNESS_UNITS_IO.md) |
| NXGO-INV-COR-003 | High-level API does not expose NX binding quirks as user obligations | [Correctness](CORRECTNESS_UNITS_IO.md) |
| NXGO-INV-IO-001 | File writes/opens never assume overwrite, Unicode, permission or open-state semantics | [Correctness](CORRECTNESS_UNITS_IO.md) |
| NXGO-INV-IPC-001 | `context` cancellation never force-aborts arbitrary running NX code | [IPC](IPC_DISTRIBUTED_SYSTEMS.md) |
| NXGO-INV-IPC-002 | Session reconnect never revives old object handles | [IPC](IPC_DISTRIBUTED_SYSTEMS.md) |
| NXGO-INV-IPC-003 | Mutating operations are never blindly retried | [IPC](IPC_DISTRIBUTED_SYSTEMS.md) |
| NXGO-INV-IPC-004 | Request idempotency is explicit for retryable mutations/workflows | [IPC](IPC_DISTRIBUTED_SYSTEMS.md) |
| NXGO-INV-PERF-001 | Public API does not require N+1 RPC for normal bulk CAD queries | [IPC](IPC_DISTRIBUTED_SYSTEMS.md) |
| NXGO-INV-GEN-001 | Codegen must model lifecycle/ref/out/callback semantics, not signatures only | [Codegen](CODEGEN_AND_API_BOUNDARIES.md) |
| NXGO-INV-GEN-002 | NXRemotableObject/MarshalByRef internals are not reused as NXGO transport | [Codegen](CODEGEN_AND_API_BOUNDARIES.md) |
| NXGO-INV-OBS-001 | ListingWindow/stdout/syslog/Agent log are not treated as one stream | [Observability/testing](OBSERVABILITY_AND_TESTING.md) |
| NXGO-INV-OBS-002 | Severe NX failures always preserve syslog/crash evidence | [Observability/testing](OBSERVABILITY_AND_TESTING.md) |
| NXGO-INV-TEST-001 | NX-backed CI never runs against uncontrolled user defaults/environment | [Observability/testing](OBSERVABILITY_AND_TESTING.md) |
| NXGO-INV-TEST-002 | Visual golden comparison is never the sole correctness oracle | [Observability/testing](OBSERVABILITY_AND_TESTING.md) |
| NXGO-INV-TEST-003 | New NX release is never declared supported without real-NX matrix | [Observability/testing](OBSERVABILITY_AND_TESTING.md) |
| NXGO-INV-TEST-004 | Recovery behavior is tested with fault/chaos injection | [Observability/testing](OBSERVABILITY_AND_TESTING.md) |

## Audit traceability

The current audit finding registry and its evidence status are indexed in
[Audit findings](AUDIT_FINDINGS.md) and verified against
`policy/audit-findings.json` by `cmd/invariantcheck`.

## Severity

- **P0 invariant**: violation can corrupt NX state, produce wrong CAD output, deadlock, or make recovery unsafe. Merge is blocked.
- **P1 invariant**: violation can create nondeterminism, compatibility defects, misleading results or operational instability. Merge is blocked unless explicitly waived by ADR.
- **P2 invariant**: maintainability/performance guard. Merge should be blocked once automated enforcement exists.

## Enforcement principle

Invariants SHOULD be moved from prose to executable enforcement whenever possible: architecture tests, analyzers, generated wrappers, protocol validation, capability gates, fault injection, NX-backed tests and supervisor state machines. See [Invariant enforcement](../INVARIANT_ENFORCEMENT.md).
