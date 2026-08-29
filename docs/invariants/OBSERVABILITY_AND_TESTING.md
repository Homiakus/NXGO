# Observability and testing invariants

## NXGO-INV-OBS-001 — log channels remain distinct

**MUST NOT:** collapse ListingWindow output, `run_journal` stdout/stderr, NX syslog, Agent structured events and supervisor logs into one undifferentiated text stream.

**MUST:** preserve source, timestamp, run/request/session/epoch correlation and severity; UI/CLI may merge views but underlying provenance remains available.

## NXGO-INV-OBS-002 — severe failures preserve evidence

**MUST NOT:** restart/kill a poisoned or crashed NX worker before attempting bounded collection of available syslog, Agent log, stdout/stderr, crash metadata, active request and fixture/artifact references.

Evidence collection itself must have a timeout and must not block recovery forever.

## NXGO-INV-TEST-001 — controlled NX environment only

**MUST NOT:** use an engineer's arbitrary Customer Defaults, templates, locale, load options, environment variables or mutable Reuse Library as the CI baseline.

**MUST:** NX-backed fixtures record/pin relevant NX build, Agent build, mode, unit system, locale, customer-default/template hashes and test workspace.

## NXGO-INV-TEST-002 — screenshots are not the oracle

**MUST NOT:** declare a CAD/drawing result correct solely because a rendered PNG/PDF looks close to a golden image.

**MUST:** prefer semantic checks first (features, topology, dimensions/associations, BOM, update status, artifact metadata), then vector/text/PDF checks, with visual diff as supplementary evidence.

## NXGO-INV-TEST-003 — support requires real NX

**MUST NOT:** mark a new NX release/build supported because generated code compiles or fake-Agent tests pass.

**MUST:** pass the pinned real-NX compatibility matrix appropriate to the advertised capability set.

## NXGO-INV-TEST-004 — recovery is chaos-tested

Fault injection MUST cover at least:
- kill NX mid-request;
- broken named pipe;
- stale handle after undo/delete/restart;
- failed Builder followed by retry attempt;
- partial assembly load/missing component;
- license unavailable;
- read-only/existing output;
- queue saturation;
- rollback failure;
- simulated session poison;
- wrong Agent/NX version;
- timeout during non-interruptible operation.

The expected assertion is not merely `error != nil`: tests verify correct error taxonomy, handle invalidation, artifact preservation and worker reuse/restart decision.