# NXGO Architecture FMEA and Risk Control

Status: **normative architecture-risk register**  
Machine source of truth: `policy/architecture-risks.json`  
Planning source of truth: `MASTER_PLAN.md`

## 1. Purpose

NXGO wraps a stateful external CAD engine across a process boundary. A defect can therefore be more dangerous than a normal library bug: a timeout can hide a committed mutation, a stale object reference can target the wrong CAD object, a release-specific runtime change can break the Agent after compilation succeeds, and a save/export failure can leave engineering artifacts inconsistent.

This document applies an FMEA-style method to the architecture itself. It is used to:

- identify architectural failure modes before feature expansion;
- quantify prioritization with Severity / Occurrence / Detection scores;
- connect each risk to concrete controls, tests, evidence and roadmap work;
- preserve residual-risk visibility after a mitigation lands;
- make architecture-risk review executable through `cmd/invariantcheck`.

The score is an engineering prioritization aid, not a safety certification or a substitute for domain-specific regulatory risk management.

## 2. Scoring model

Each risk has two score sets:

- **inherent** — expected exposure without the listed NXGO controls;
- **residual** — exposure after controls currently implemented and evidenced.

Each dimension uses a 1–10 scale:

- **Severity (S)** — engineering consequence if the failure occurs. A score of 10 means possible silent or uncontrolled wrong CAD mutation/data result, not merely a recoverable API error.
- **Occurrence (O)** — likelihood under realistic supported workloads and release conditions.
- **Detection (D)** — likelihood the failure escapes current prevention/detection controls. Higher means harder to detect before engineering impact.

`RPN = S × O × D`

Planning bands:

| Residual RPN | Planning meaning |
|---:|---|
| `>= 250` | Critical — release/architecture gate unless explicitly accepted by ADR |
| `150..249` | High — must have an active remediation workstream and exit criteria |
| `80..149` | Medium — tracked with evidence and review triggers |
| `< 80` | Low residual — remains visible as `watch` when severity is intrinsically high |

Severity is not reduced merely because controls exist. Controls normally reduce occurrence and/or improve detection.

## 3. Status model

- `open` — important control is absent or incomplete and a remediation plan is required;
- `mitigating` — controls exist but a material gap remains;
- `watch` — current controls/evidence reduce residual exposure, but the architectural failure mode must remain monitored because future changes can reintroduce it;
- `accepted` — risk is consciously accepted with a decision record; used sparingly;
- `closed` — failure mode is no longer applicable to the architecture, not merely "fixed once".

For this project, high-severity boundary risks are normally `watch`, not `closed`, because changes in NX versions, transport, object lifecycle or recovery can reintroduce them.

## 4. Architecture FMEA register

The full causal/control/action detail lives in `policy/architecture-risks.json`. This table is the human review index and is checked against that file by `cmd/invariantcheck`.

| Risk ID | Area | Failure mode | Inherent S/O/D/RPN | Residual S/O/D/RPN | Status | Primary plan |
|---|---|---|---:|---:|---|---|
| RISK-ARCH-001 | transport / outcome | Ambiguous mutation outcome is retried as if it definitely failed | 10/6/6/360 | 10/2/3/60 | watch | H1, H2, D6 |
| RISK-ARCH-002 | object lifetime | Stale/foreign handle resolves or mutates the wrong NX object | 10/5/7/350 | 10/2/2/40 | watch | H3, D1-D3 |
| RISK-ARCH-003 | execution model | NXOpen executes on an unsupported thread or unsafe reentrant context | 10/5/6/300 | 10/3/3/90 | watch | H4, D6 |
| RISK-ARCH-004 | transaction / recovery | Partial mutation or incomplete update leaves a poisoned session reusable | 10/6/6/360 | 10/4/4/160 | mitigating | H5.2, H5.3 |
| RISK-ARCH-005 | runtime compatibility | NX release/load-context change breaks bootstrap or type identity | 9/5/6/270 | 9/3/4/108 | watch | H6.1, release qualification |
| RISK-ARCH-006 | semantic correctness | Unit/semantic drift produces plausible but wrong engineering values | 10/5/5/250 | 10/2/3/60 | watch | H5.1, D2-D4 |
| RISK-ARCH-007 | code generation | Raw binding/capability map drifts from actual NX API surface | 8/5/6/240 | 8/4/5/160 | mitigating | API scanner |
| RISK-ARCH-008 | resource lifecycle | Handles/journal/queue/native resources grow without bound | 7/6/4/168 | 7/2/2/28 | watch | H3, H6.3, D6 |
| RISK-ARCH-009 | security boundary | Unintended local process connects to or impersonates the worker pipe | 9/4/7/252 | 9/3/4/108 | mitigating | H6.2, D6 |
| RISK-ARCH-010 | data durability | Save/export is reported successful before durable artifact publication | 9/4/5/180 | 9/3/3/81 | mitigating | H5.4, D4 |
| RISK-ARCH-011 | verification | Fake/unit evidence overstates production or release safety | 9/5/7/315 | 9/2/3/54 | watch | evidence policy, H4, H6.1 |
| RISK-ARCH-012 | API contract | SDK accepts parameters/abstractions not faithfully implemented by backend | 8/5/6/240 | 8/2/3/48 | watch | H5.2, domain phases |
| RISK-ARCH-013 | performance / resilience | Fine-grained IPC and long serialized work cause timeout/recycle cascades | 8/6/5/240 | 8/3/4/96 | mitigating | H6.4, D2/D3/D6 |
| RISK-ARCH-014 | headless automation | Modal UI/journal state blocks unattended execution | 8/4/6/192 | 8/3/5/120 | mitigating | H6.3, D4/future domains |
| RISK-ARCH-015 | distributed transaction | NX and external-system side effects diverge with no shared rollback | 9/4/7/252 | 9/4/6/216 | open | workflow plane, H5.4, future PLM |
| RISK-ARCH-016 | capability / NX state | License or partial-load state is mistaken for complete capability/model state | 9/5/5/225 | 9/3/4/108 | mitigating | H5, H6.1, D3 |

## 5. Highest current residual risks

### RISK-ARCH-015 — external side effects / distributed transaction — RPN 216

NX undo marks cannot roll back Teamcenter/Designcenter writes, remote storage publication or other external side effects. The architecture already states that transactions are session-local, but a future PLM/publishing workflow needs an explicit saga/compensation model before write support is admitted.

Required before external-system write support:

1. durable workflow-step identity;
2. explicit ordering of NX commit vs external publication;
3. idempotency key or compensation for every external side effect;
4. crash injection between each step;
5. recovery state that distinguishes `NX committed / external not published`, `external published / response lost`, and `compensation failed`.

### RISK-ARCH-004 — partial mutation/update/rollback — RPN 160

The transaction shape is strong, but `MASTER_PLAN.md` still carries incomplete UpdateManager/update recipe work for mutating operations. New builder-backed APIs must not rely on generic cleanup alone; each one needs a specific update/rollback/postcondition recipe and a health-state result when recovery cannot be proven.

### RISK-ARCH-007 — raw generation / capability drift — RPN 160

Generated bindings dramatically expand coverage, but they also increase the probability of a false compatibility claim if overload identity, dispatch generation and release evidence are not tied together. Capability must mean "binding + dispatch + qualified release evidence", not just "scanner saw a symbol".

## 6. Risk-control layers

NXGO should not depend on a single prevention mechanism. A high-severity risk is controlled through several layers:

```text
public contract / validation
        |
        v
protocol identity / typed envelopes
        |
        v
Agent admission / capability / journal
        |
        v
NX-safe executor / transaction / object registry
        |
        v
semantic postcondition / health classification
        |
        v
supervisor quarantine / recycle / artifact retention
        |
        v
real-NX evidence and compatibility matrix
```

A control is considered strong only when it is both implemented and testable at the layer where the failure can actually happen.

## 7. Planning integration rules

`MASTER_PLAN.md` is the only implementation roadmap. FMEA does not create a second backlog.

Every `open` or `mitigating` risk must have:

- at least one `plan_refs` entry;
- explicit next actions;
- measurable acceptance criteria;
- evidence paths;
- review triggers;
- an occurrence/detection score that is reduced only when evidence justifies it.

Every architecture-affecting plan task and PR must answer:

1. Which `RISK-ARCH-*` IDs can this change increase, decrease or create?
2. Does the change alter S/O/D or status?
3. What evidence justifies a reduced score?
4. Does a new failure mode require a new risk entry and possibly a new invariant/ADR?

A feature is not "done" if it lowers code/test debt while increasing an untracked architecture risk.

## 8. Merge and release rules

- A new architecture risk with residual RPN `>= 150` must enter `MASTER_PLAN.md` in the same PR.
- `open`/`mitigating` risks must not disappear from the plan because an implementation commit landed; the score/status changes only with evidence.
- `accepted` or `closed` high-severity risks require a decision record and retained evidence.
- An architecture change that alters transport, executor, request journal, object identity, transaction/update semantics, runtime loader, capability generation or external publication must explicitly review the corresponding risk IDs.
- Release evidence must be tied to the actual NX release/build; simulated evidence cannot lower a real-NX occurrence/detection score by itself.

## 9. Review cadence and triggers

The register is reviewed:

- on every architecture-changing PR;
- before a new NX release is added to the supported matrix;
- before a new major domain (CAM, routing, CAE, PLM integration, broad PMI) is unfrozen;
- after any production incident, hung/poisoned session, unexplained CAD mutation or artifact mismatch;
- after any failed chaos/soak/matrix campaign that changes the likelihood or detectability of a failure mode.

Scores are not calendar theater. If no architecture/evidence changed, the score should normally remain unchanged.

## 10. Adding a new risk

1. Allocate the next `RISK-ARCH-###` ID.
2. Describe the failure mode, effect and concrete causes.
3. Score inherent S/O/D.
4. List current controls and evidence paths.
5. Score residual S/O/D conservatively.
6. Add a `MASTER_PLAN.md` work item/phase reference.
7. Define next actions, acceptance criteria and review triggers.
8. Add the human-readable row here.
9. Run `go run ./cmd/invariantcheck` (or `go run ./cmd/nxctl test fast`).

The checker rejects duplicate IDs, invalid scores/RPN math, missing planning/evidence/action fields, plan omissions for active risks, and docs/register status drift.
