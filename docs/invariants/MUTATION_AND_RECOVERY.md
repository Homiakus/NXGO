# Mutation, Builder and recovery invariants

## NXGO-INV-MUT-001 — Builder destruction is unconditional

**MUST NOT:** return, throw, cancel or serialize a response while a created NX Builder is left undisposed/undestroyed.

**MUST:** wrap builder lifetime in Agent-owned `try/finally`/scope logic. The public Go API never owns Builder cleanup.

```text
CreateBuilder -> configure -> Validate/Commit -> finally Destroy
```

This applies on success, NXException, validation failure, cancellation observation and protocol serialization failure.

## NXGO-INV-MUT-002 — failed Builder is single-attempt

**MUST NOT:** mutate parameters and retry `Commit` on a Builder after its commit/validation failed unless Siemens explicitly documents reuse as safe for that exact API and release.

**MUST:** destroy failed Builder and create a fresh one for a retry.

## NXGO-INV-MUT-003 — Commit is not final model state

**MUST NOT:** assume `Commit()` alone means downstream geometry is updated and safe to consume.

**MUST:** each high-level mutation recipe explicitly defines required update behavior (`UpdateManager`/equivalent), then validates postconditions.

A recipe is incomplete without: undo boundary where applicable -> build -> commit -> update -> semantic validation -> cleanup.

## NXGO-INV-MUT-004 — Undo is not distributed ACID

**MUST NOT:** document `Transaction` as guaranteeing rollback of filesystem, Teamcenter, network, database, export or arbitrary external side effects.

**MUST:** distinguish model rollback from workflow compensation. External artifacts should be produced to temporary locations and promoted atomically after model validation when possible.

## NXGO-INV-SES-001 — poisoned sessions are destroyed

**MUST NOT:** continue normal work after an error classified as kernel/modeler fatal, NX fatal error, corrupted update state, failed critical rollback or equivalent `SESSION_POISONED` condition.

**MUST:** stop accepting mutations, preserve diagnostics, terminate the worker and start a fresh NX process before subsequent jobs.

## NXGO-INV-SES-002 — recovery semantics are first-class

**MUST NOT:** map all NX failures to a generic `error` and let callers guess whether retry/reuse is safe.

**MUST:** classify failures at least along:
- recoverable operation error;
- transaction abort;
- session busy/invalid state;
- session suspect;
- session poisoned;
- process crash;
- license/capability failure;
- infrastructure/protocol failure.

Errors exposed to Go include `Retryable`, `Recoverable`, `Poisoned`, operation/request/session IDs and NX code when available.

**Tests:** inject normal NX exception, rollback failure, simulated kernel poison and process death; assert different supervisor actions.