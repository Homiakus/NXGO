# Audit finding index

This file is the human-readable index for the machine-readable finding
registry in `policy/audit-findings.json`. It deliberately records only the
finding identity and current policy status; the detailed evidence and
remediation criteria remain in `MASTER_PLAN.md` and the linked invariant
documents.

| Finding | Policy status | Primary invariant area |
| --- | --- | --- |
| A-001 | open | IPC sequencing and cancellation |
| A-002 | open | IPC outcome ambiguity |
| A-003 | open | mutation idempotency and recovery |
| A-004 | open | ObjectRef identity |
| A-005 | open | object lifetime and fail-closed resolution |
| A-006 | open | semantic CAD correctness |
| A-007 | open | geometry units and mass properties |
| A-008 | open | builder lifecycle |
| A-009 | open | save and close durability |
| A-010 | mitigated | Agent Core consolidation |
| A-011 | mitigated | canonical production dispatch |
| A-012 | open | protocol and runtime hardening |
| A-013 | mitigated | registry leases and dependent invalidation |
| A-014 | open | local pipe security |
| A-015 | open | release evidence and qualification |
| A-016 | open | NX2512 C# Agent bootstrap |
| F-017 | mitigated | supervisor early-exit diagnostics |

The registry is authoritative for status. A finding marked `mitigated` is not
release proof: the corresponding phase exit gate and retained evidence are
still required.
