# Journal, headless and UI invariants

## NXGO-INV-JRN-001 — recorded object IDs are not selectors

**MUST NOT:** ship production automation that depends on recorded `FindObject("...")`, journal handles or other selection identifiers tied to one recorded model unless the identifier is an intentionally managed stable project identifier.

**MUST:** replace journal selection with semantic selectors, explicit attributes, stable NXGO IDs or domain-specific selection rules.

**Why:** recorded journals are sticky to the model/session they were recorded against and commonly fail on other parts.

## NXGO-INV-JRN-002 — journals are discovery artifacts

**MUST NOT:** treat raw recorded journal code as clean production implementation or mechanically concatenate recorded journals without normalization.

**MUST:** use journals to discover call sequences, then remove transient setup, brittle selectors, duplicate variables, GUI-specific state and Builder noise before implementing an Agent recipe/domain API.

A future journal-to-recipe tool MUST flag unresolved `FindObject`, UI coordinates, transient object names and release-specific calls.

## NXGO-INV-UI-001 — GUI success does not imply headless support

**MUST NOT:** mark a capability worker/headless-safe solely because it succeeds in interactive NX.

**MUST:** capabilities declare execution requirements such as `HEADLESS_SAFE`, `GRAPHICS_REQUIRED`, `INTERACTIVE_REQUIRED`, `USER_INTERACTION_REQUIRED`, `MANAGED_MODE_REQUIRED`.

## NXGO-INV-UI-002 — no coordinate/ribbon automation in core CI

**MUST NOT:** use screen coordinates, ribbon positions, DPI-sensitive clicks or generic desktop macro automation as the primary implementation of an NXGO domain operation.

Such techniques MAY exist only as an explicitly unstable UI fallback with separate capability/security classification.

## NXGO-INV-UI-003 — modal UI is controlled

**MUST NOT:** allow unattended workers to invoke operations known to open file dialogs, message boxes or user prompts without a deterministic suppression/answer strategy.

**MUST:** worker policy denies interactive-required calls by default. Interactive attach mode may allow them and reports session state.

**Tests:** simulate/trigger modal-capable path and verify worker rejects or supervisor identifies the non-progress state rather than hanging indefinitely.