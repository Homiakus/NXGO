# NX state, assemblies, licenses and Teamcenter invariants

## NXGO-INV-STATE-001 — Work/Display/Component are distinct

**MUST NOT:** expose a single ambiguous `CurrentPart` as the basis of mutating high-level operations.

**MUST:** model `DisplayPart`, `WorkPart` and `WorkComponent` distinctly; high-level mutations either take an explicit target or document a deterministic target-resolution policy.

After part open/switch operations the Agent re-resolves context rather than trusting cached references.

## NXGO-INV-ASM-001 — partial-load completeness is explicit

**MUST NOT:** return authoritative assembly-wide mass/BOM/topology/count results when required components may be partially/unloaded without marking completeness.

**MUST:** operations declare load policy (`RequireFullLoad`, `AllowPartial`, explicit component set) and return completeness metadata where partial analysis is meaningful.

## NXGO-INV-ASM-002 — load status is never discarded

**MUST NOT:** interpret `Open` as binary success/failure and discard `PartLoadStatus` or equivalent warnings.

**MUST:** surface normalized `LoadIssue[]`, `Complete`/`Partial` state and strict-mode `ErrPartialLoad` when incomplete loading violates the operation contract.

## NXGO-INV-CAP-001 — startup is not license capability

**MUST NOT:** infer CAM, advanced assemblies, routing, validation or other module availability merely because NX launched.

**MUST:** capability/license checks occur at handshake and/or operation boundary as appropriate; missing licenses map to stable typed errors and never to generic internal failures.

## NXGO-INV-STATE-002 — interactive NX state is respected

**MUST NOT:** inject arbitrary operations into an interactive session while NX is inside incompatible commands, sketch editing, drawing state, modal interaction or other blocked application state.

**MUST:** model session state and operation policy: `FAIL_IF_BUSY`, `WAIT_UNTIL_IDLE`, `INTERACTIVE_SAFE`, `WORKER_ONLY`.

## NXGO-INV-TCM-001 — managed mode has its own semantics

**MUST NOT:** pretend Teamcenter/managed objects are ordinary filesystem paths with native-mode open/save semantics.

**MUST:** isolate managed-mode contracts (item/revision/dataset, revision/load rules, permissions/check-out state, managed load issues) behind explicit capabilities and APIs.

Native mode tests MUST NOT be used as proof of managed-mode correctness.