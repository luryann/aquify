# Legacy Export Inventory

This document defines the minimum legacy exports required to run Step 24 migration prep and dry-runs.

## Required export set

| Export | Source system | Format | Owner | Provenance / notes |
| --- | --- | --- | --- | --- |
| Households and contacts | TeamUnify / SportsEngine Motion | CSV | Operations lead | Parent account is the launch-critical source of truth for household operations. |
| Swimmers roster | TeamUnify / SportsEngine Motion | CSV | Membership lead | Must include swimmer external IDs when available. |
| Groups / roster groups | TeamUnify / SportsEngine Motion | CSV | Coaching lead | Needed to preserve training-group assignment and registration eligibility context. |
| Historical best times | TeamUnify / SportsEngine Motion or derived reporting export | CSV | Head coach | Used when full result-history migration is deferred or incomplete. |
| Open invoices | TeamUnify / SportsEngine Motion / billing export | CSV | Finance lead | Must include invoice number, due date, outstanding amount, and household linkage. |
| Payment history | TeamUnify / SportsEngine Motion / billing export | CSV | Finance lead | Must include invoice linkage and payment date/history for reconciliation. |
| Meet history / entries / results | TeamUnify / SportsEngine Motion | CSV or native meet-file exports | Operations lead | Used for later parity checks and extended historical migration. |
| Results archive payloads | Hy-Tek / Meet Manager ecosystem | `.sd3`, `.cl2`, `.hy3`, `.zip` | Meet operations lead | Retain raw native files for re-import validation and historical audit. |
| HY3 results inventory | Hy-Tek / Meet Manager ecosystem | `.hy3` | Meet operations lead | Required explicitly for migration readiness. Provenance: exported from Hy-Tek-compatible results workflows; keep owner, season, and originating meet metadata alongside each archive because HY3 is treated as a ZIP container with source provenance. |
| Known bad-data samples | Legacy source exports + operator notes | CSV plus notes | Data migration lead | Preserve representative malformed rows for remediation rehearsal. |

## Required directory shape for the new migration CLI

The Step 24 migration CLI expects:

```text
<input-dir>/
  households.csv
  swimmers.csv
  groups.csv
  historical_best_times.csv
  open_invoices.csv
  payment_history.csv
```

Optional legacy evidence files that should still be preserved for dry-run packages:

```text
<input-dir>/evidence/
  known-bad-data-notes.md
  hy3/
  raw-results/
```

## Ownership checkpoints

- Operations lead supplies household, meet-history, and native meet-file exports.
- Membership lead validates household-to-swimmer linkage completeness.
- Coaching lead validates groups and best-time expectations.
- Finance lead validates open invoices and payment history exports.
- Data migration lead owns the final dry-run packet, reconciliation output, and remediation log.
