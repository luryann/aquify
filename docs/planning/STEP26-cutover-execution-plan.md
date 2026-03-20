# STEP 26 — Cutover Execution Plan

Date: 2026-03-18
Owner: Codex
Status: In progress

## Goal

Turn `TODO.md` section `26) Cutover execution plan` into an operator-ready launch
packet that combines:

- an executable cutover runbook
- signoff and rollback templates
- repeatable smoke/certification commands
- explicit hypercare monitoring references

## Inputs reviewed

- `design-language-guide.html`
- `docs/planning/Developer Handoff Master Document.md`
- `docs/planning/Plan.md`
- `docs/planning/STEP24-migration-prep-plan.md`
- `docs/planning/STEP25-observability-plan.md`
- `docs/planning/release-gate-checklist.md`
- `docs/runbooks/on-call.md`
- `CLAUDE.md`
- `TODO.md`
- `Makefile`

## Constraints and assumptions

- The repository can prepare the cutover workflow, but it cannot truthfully mark
  production-only actions as executed from local source control alone.
- Migration rehearsal should reuse the Step 24 migration CLI rather than invent a
  second cutover-only importer path.
- Certification should reuse the Step 23 regression suite already in the repo.
- Hypercare should point to the Step 25 dashboards and alerts rather than
  duplicate observability definitions.

## Deliverables for this step

### 1. Operator runbooks

- `docs/runbooks/cutover.md`
- `docs/runbooks/hypercare.md`

### 2. Templates

- `docs/migration/cutover-signoff-template.md`

### 3. Repo automation

- `scripts/cutover-rehearsal.sh`
- `scripts/cutover-smoke.sh`
- `scripts/cutover-certification.sh`
- `scripts/cutover-billing-check.sh`
- `Makefile` targets that wrap those scripts

### 4. Documentation updates

- `README.md` command references
- `TODO.md` status note for Step 26
- `CLAUDE.md` runtime notes for Step 26 artifacts

## TODO.md mapping

| TODO item | Repo artifact / command |
|---|---|
| Freeze legacy data-change window for cutover period | `docs/runbooks/cutover.md` communication + freeze checklist |
| Run final migration rehearsal | `make cutover-rehearsal` |
| Execute pre-cutover backup | `docs/runbooks/cutover.md` backup checklist |
| Execute production migration | `docs/runbooks/cutover.md` migration sequence |
| Validate migration checklist completion | `docs/planning/release-gate-checklist.md` + cutover checklist |
| Run smoke tests for critical user flows | `make cutover-smoke` |
| Run billing reconciliation check | `make cutover-billing-check` |
| Run import/export certification fixture suite | `make cutover-certify` |
| Collect Product / Engineering / Operations / Finance signoff | `docs/migration/cutover-signoff-template.md` |
| Switch traffic/DNS to new platform | `docs/runbooks/cutover.md` DNS switch checklist |
| Monitor first 24h hypercare dashboard | `docs/runbooks/hypercare.md` |

## Verification target

- `make cutover-rehearsal`
- `make cutover-certify`
- `scripts/cutover-smoke.sh` usage output without required env vars
- `scripts/cutover-billing-check.sh` usage output without required env vars

## Remaining real-world execution blockers

- a scheduled production cutover window
- provisioned production/staging credentials
- real stakeholder approvals and signatures
- live DNS / infrastructure access
