# IPC, cancellation, retry and performance invariants

## NXGO-INV-IPC-001 — cancellation is not unsafe thread abort

**MUST NOT:** interpret Go `context` cancellation as permission to abort a running NX thread or tear down arbitrary NXOpen code mid-call.

**MUST:** distinguish operation states:
- queued: cancel safely;
- running/cooperative: request cancellation only through a supported safe mechanism;
- non-interruptible: client deadline may expire while Agent reaches a safe boundary;
- hung/suspect: supervisor may terminate the entire NX worker and invalidate the epoch.

## NXGO-INV-IPC-002 — reconnect never revives old handles

**MUST NOT:** transparently reconnect a client and continue using prior `Part`, `Face`, `Feature`, Builder or other remote proxies.

**MUST:** increment/change session epoch on Agent/NX restart; old handles fail deterministically with stale/session-lost errors.

## NXGO-INV-IPC-003 — no blind retries of mutations

**MUST NOT:** apply generic automatic retry loops to mutating NX operations.

A response can be lost after NX committed the change. Blind retry can create duplicate features, exports or Teamcenter actions.

**MUST:** retry policy depends on operation semantics and known execution state.

## NXGO-INV-IPC-004 — idempotency is explicit

Retryable mutating workflows MUST use request IDs/idempotency keys or deterministic workflow checkpoints. Agent/supervisor SHOULD retain short-lived completion records sufficient to answer duplicate requests without reapplying a committed change.

Automatic replay after worker restart is allowed only for workflows explicitly declared replay-safe from clean inputs.

## NXGO-INV-PERF-001 — no N+1 normal API

**MUST NOT:** design common operations that require one IPC round trip per face/edge/feature/component when NX can evaluate the batch locally.

**MUST:** provide coarse bulk APIs (`Analyze`, batch attributes, assembly inspection, drawing generation, batch export) and keep raw per-object calls as advanced escape hatches.

**Performance gate:** representative 300-component assembly scenarios must compare batch and raw/chatty paths; domain API should remain bounded primarily by NX work, not IPC count.