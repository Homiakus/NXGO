# Engineering correctness, units and I/O invariants

## NXGO-INV-COR-001 — no-exception is not correctness

**MUST NOT:** treat a successful NXOpen return/Commit as sufficient proof that the requested engineering intent occurred.

**MUST:** high-level mutating operations define semantic postconditions appropriate to the operation: expected body/feature count, topology relationship, volume/mass bounds, drawing association/update state, BOM consistency, created artifact metadata, etc.

Postcondition failure is an NXGO operation failure even if NX raised no exception.

## NXGO-INV-COR-002 — no unitless engineering dimensions

**MUST NOT:** expose ambiguous `float64` for dimensional values in the domain/workflow API when units matter.

**MUST:** use typed quantities or explicit units (`MM`, `Inch`, angle units, mass units, tolerances) and normalize protocol representation to value + dimension/unit.

Generated raw API MAY expose native representation but is clearly lower-level and must not dictate high-level types.

**Tests:** round-trip mm/inch fixtures and deliberately wrong-unit cases; no silent 25.4x conversion is acceptable.

## NXGO-INV-COR-003 — binding quirks stay behind adapters

**MUST NOT:** require ordinary Go callers to know that a specific NX property expects expression text, a float instead of integer, a hidden nonzero tolerance default, or a release-specific enum quirk.

**MUST:** normalize these behaviors in release/domain adapters, validate inputs before NX invocation and expose native quirks only in raw API.

## NXGO-INV-IO-001 — file behavior is explicit

**MUST NOT:** assume overwrite is allowed, file is not already open, directory is writable, path encoding is supported, or native/managed path semantics are interchangeable.

**MUST:** define policies for existing files, temporary export, atomic promotion, open-part conflicts, permission errors and supported path character sets. File side effects must be included in workflow compensation design.

**Tests:** existing target, read-only target, open part, missing directory, Unicode path capability, export failure after successful model mutation.