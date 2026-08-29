# Version, runtime and Agent lifecycle invariants

## NXGO-INV-VER-001 — no assumed cross-release API stability

**MUST NOT:** assume code compiled/tested on one NX Continuous Release has identical methods, enums, defaults or behavior on another.

**MUST:** maintain per-release API manifests, diffs, capability adapters and real-NX regression results. Siemens API reporting/local installed metadata is preferred evidence.

## NXGO-INV-VER-002 — codegen is multi-release

**MUST NOT:** regenerate raw bindings from only the newest installed NX and silently replace compatibility knowledge.

**MUST:** preserve versioned manifests (for example 2512 and 2606), canonical symbol IDs and explicit introduced/changed/removed metadata where discoverable.

Generated public/raw contracts must not leak a release-specific signature when a canonical versioned adapter is required.

## NXGO-INV-VER-003 — Agent runtime follows NX support matrix

**MUST NOT:** select .NET/Python/compiler target because it is newest or convenient.

**MUST:** build/load the Agent using a runtime/toolchain supported by the target NX release. The Pure-Go client remains decoupled from this constraint.

`nxctl doctor` MUST report installed NX build, expected Agent flavor/runtime, found NXOpen assemblies and mismatch diagnostics.

## NXGO-INV-VER-004 — long-lived Agent is not immediately unloaded

**MUST NOT:** use immediate library unload for the resident Agent while it owns callbacks, service state, pipe endpoints or NX object registry.

**MUST:** use lifecycle appropriate to a process-resident plugin (normally until NX termination) and keep development hot-reload separate from production lifecycle.

## NXGO-INV-VER-005 — callback ownership is explicit

**MUST NOT:** register callbacks without recording a corresponding unregister/dispose action.

**MUST:** centralize callback subscriptions in an owned registry and remove them on controlled shutdown/reload. Duplicate registration after reload must be detectable.

**Tests:** repeated Agent initialization/shutdown in a development harness must not multiply event delivery.