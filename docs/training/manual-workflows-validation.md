# Manual Workflows Validation Checklist — DARE Platform

**Reference:** `docs/decisions/2026-03-15-manual-at-launch-workflows.md`
**Version:** 1.0
**Last Updated:** 2026-03-23
**Environment:** Staging (production-like data seed required before validation)

---

## Purpose

This document provides a step-by-step validation checklist for every manual workflow that must operate correctly at launch. Each workflow must be validated in staging by the named owner before production cutover is approved.

A workflow is **validated** when the named owner has completed all steps and signed off. Incomplete sign-offs are launch-blocking.

---

## Workflow 1: New Season Setup

**Description:** Create a new registration season, configure groups, set up registration periods, and make them available to families.

**Owner:** Club Admin Lead
**Backup Validator:** Product Manager

### Steps to Validate

1. [ ] Log in to the admin portal as an owner or admin.
2. [ ] Navigate to **System → Seasons**.
3. [ ] Click **New Season** (or edit the existing upcoming season).
4. [ ] Fill in:
   - Season name (e.g., "Summer 2026")
   - Start date and end date
   - Status: `upcoming`
5. [ ] Save the season. Verify it appears in the seasons list with correct dates.
6. [ ] Navigate to **Members → Groups**.
7. [ ] Create at least two groups for the season (e.g., "Senior" and "Junior").
   - Set eligibility rules (min age, max age, gender filter if applicable).
   - Set the season association.
8. [ ] Verify the groups appear in the groups list with correct details.
9. [ ] Navigate to **Members → Registrations**.
10. [ ] Verify the registration period is visible with a start and end date.
11. [ ] Switch to the parent portal (use a test parent account).
12. [ ] Verify the registration option is visible with the correct season and groups.

### Pass Criteria

- Season created with correct dates and displays in admin UI.
- At least two groups created with eligibility rules.
- Registration period visible to parents on the portal.
- No 500 errors or unexpected redirects during the flow.

### Sign-Off

| | |
|---|---|
| Validator Name | |
| Date Completed | |
| Environment | Staging |
| Result | Pass / Fail |
| Notes | |

---

## Workflow 2: Meet Setup and Commitment Cycle

**Description:** Create a meet from scratch, open commitments to parents, review and adjust entries, finalize entries, and export an SD3 file.

**Owner:** Club Admin Lead (setup) + Head Coach (commitment review)
**Backup Validator:** Product Manager

### Steps to Validate

1. [ ] Log in as admin/owner.
2. [ ] Navigate to **Meets → New Meet**.
3. [ ] Fill in Basic Info (name, host, location, pool type: LCM, entry deadline).
4. [ ] Optionally upload a test meet package PDF.
5. [ ] Add two sessions (Day 1 and Day 2) with realistic dates, warm-up, and start times.
6. [ ] Add at least 4 events across the sessions (e.g., 100 Free, 200 IM, 50 Back, 100 Fly).
7. [ ] Click **Publish Meet**. Verify status transitions to `published`.
8. [ ] Click **Open Commitments**. Verify status transitions to `commitment_open`.
9. [ ] Switch to the parent portal (test parent account with swimmers in eligible groups).
10. [ ] Verify the meet appears in **Portal → Meets** with commitment buttons.
11. [ ] Commit "Yes" for swimmer 1 and select two events. Include a coach note.
12. [ ] Commit "No" for swimmer 2 (if applicable).
13. [ ] Switch back to admin portal.
14. [ ] Navigate to **Commitments** and filter by the test meet.
15. [ ] Verify both commitments appear with correct statuses.
16. [ ] Click a swimmer to open the drill-down panel. Verify events and note are visible.
17. [ ] As a coach, override one event selection. Enter an audit reason.
18. [ ] Navigate to the meet detail and click **Close Commitments**. Verify status transitions.
19. [ ] Click **Finalize Entries**. Verify status transitions to `entry_finalized`.
20. [ ] Click **Export Entries (SD3)**. Wait for the export to generate.
21. [ ] Download the SD3 file and verify it is a valid SDIF file (non-empty, correct extension).

### Pass Criteria

- Meet created and published successfully.
- Parents can commit and select events from the portal without a swimmer login.
- Coach override is logged in the audit trail with the provided reason.
- Entry finalization locks all entries.
- SD3 export generates and downloads successfully.
- No 500 errors during any step.

### Sign-Off

| | |
|---|---|
| Validator Name | |
| Date Completed | |
| Environment | Staging |
| Result | Pass / Fail |
| Notes | |

---

## Workflow 3: Results Import

**Description:** Upload a meet result file (HY3 format), review and resolve any matching conflicts, apply the import, and verify that swimmer best times are updated.

**Owner:** Club Admin Lead
**Backup Validator:** Head Coach

### Steps to Validate

1. [ ] Obtain a valid test HY3 file (can be a synthetic SD3 file in a ZIP renamed to `.hy3`). Ensure at least one swimmer in the file matches a DARE swimmer by name+DOB.
2. [ ] Log in as admin.
3. [ ] Navigate to **Imports → Upload File**.
4. [ ] Upload the test HY3 file.
5. [ ] Observe status transitions: `uploaded → scanning → parsing → matching`.
6. [ ] If the import reaches `conflicts_pending`: review the Conflicts tab and resolve each conflict by matching the record to the correct swimmer.
7. [ ] Click **Apply Import**.
8. [ ] Verify the import transitions to `applied`.
9. [ ] Navigate to **Members → Swimmers** and open the swimmer detail for a swimmer who had results in the file.
10. [ ] Verify that best times have been updated with the imported times.
11. [ ] Optionally run the performance report to confirm the times appear.

### Pass Criteria

- File uploads and begins processing within 30 seconds.
- Conflict resolution UI presents matches with sufficient swimmer detail to make a correct decision.
- Import applies successfully with no errors.
- Swimmer best times are updated in the swimmer detail view.
- Import audit event is present in the audit log.

### Sign-Off

| | |
|---|---|
| Validator Name | |
| Date Completed | |
| Environment | Staging |
| Result | Pass / Fail |
| Notes | |

---

## Workflow 4: Monthly Billing Cycle

**Description:** Generate invoices for a group of families, send invoice emails, track payments, and run the monthly reconciliation report.

**Owner:** Club Treasurer
**Backup Validator:** Finance Admin

### Steps to Validate

1. [ ] Log in as admin/billing role.
2. [ ] Navigate to **Billing → Invoices → New Invoice**.
3. [ ] Create a test invoice for a test household:
   - Line item: "Monthly dues" — $50.00
   - Due date: 30 days from today
4. [ ] Click **Issue Invoice**. Verify status transitions from `draft` to `issued`.
5. [ ] Click **Send Invoice Email**. Verify the email is sent (check Mailpit or email logs).
6. [ ] Switch to the parent portal (test parent account).
7. [ ] Navigate to **Billing** and verify the invoice is visible with the correct amount.
8. [ ] Initiate a Stripe test payment (use Stripe test card `4242 4242 4242 4242`).
9. [ ] Verify the payment completes and the invoice transitions to `paid`.
10. [ ] Switch back to admin portal.
11. [ ] Verify the payment is recorded in the invoice's Payments tab.
12. [ ] Navigate to **Reports → Payment Reconciliation**.
13. [ ] Generate the report for the current month.
14. [ ] Verify the test payment appears in the report with no discrepancy.

### Pass Criteria

- Invoice created and emailed successfully.
- Parent can view and pay the invoice from the portal.
- Payment is recorded automatically via Stripe webhook.
- Reconciliation report shows $0 discrepancy for the test period.

### Sign-Off

| | |
|---|---|
| Validator Name | |
| Date Completed | |
| Environment | Staging |
| Result | Pass / Fail |
| Notes | |

---

## Workflow 5: New Family Onboarding

**Description:** Onboard a new family — create household, add swimmers, assign groups, complete registration, and make a payment.

**Owner:** Club Admin Lead
**Backup Validator:** Operations Staff

### Steps to Validate

1. [ ] Log in as admin.
2. [ ] Navigate to **Members → Households → Add Household**.
3. [ ] Create a test household: "Test Family", email `testfamily+staging@example.com`.
4. [ ] Navigate to **Members → Swimmers → Add Swimmer**.
5. [ ] Add a swimmer attached to the test household: "Alex Test", DOB 2015-01-01, gender M.
6. [ ] Navigate to **Members → Groups** and assign Alex Test to the appropriate group.
7. [ ] Verify the swimmer appears in the group member list.
8. [ ] Navigate to **Members → Registrations**.
9. [ ] Create or find a pending registration for the test household.
10. [ ] Approve the registration.
11. [ ] Verify the family receives an onboarding email (check Mailpit or email logs).
12. [ ] Switch to the parent portal using the test household's account.
13. [ ] Verify the swimmer appears in the household view.
14. [ ] Verify the swimmer is shown in the meets list as eligible for commitment.
15. [ ] Navigate to **Compliance → Documents** on the admin side.
16. [ ] Create a test document type: "Test Waiver 2026" (required).
17. [ ] Switch to parent portal and upload a test document for "Test Waiver 2026".
18. [ ] Switch back to admin and verify the submission appears for review.
19. [ ] Verify the document and confirm status transitions to `verified`.

### Pass Criteria

- Household and swimmer created without errors.
- Swimmer assignable to a group.
- Registration approval triggers onboarding email.
- Parent portal shows the swimmer and eligible meets without a swimmer account.
- Document upload and verification flow completes end-to-end.

### Sign-Off

| | |
|---|---|
| Validator Name | |
| Date Completed | |
| Environment | Staging |
| Result | Pass / Fail |
| Notes | |

---

## Workflow 6: Data Export for Compliance (GDPR/COPPA)

**Description:** Process a data export request from a family — generate a full data export, deliver it, and log the action.

**Owner:** Club Admin Lead
**Backup Validator:** Lead Engineer (for technical export steps)

### Steps to Validate

1. [ ] Log in as owner (owner role required for compliance actions).
2. [ ] Navigate to **Compliance → Data Requests**.
3. [ ] Create a test export request for the test household created in Workflow 5.
4. [ ] Verify the request appears in the requests list with status `pending`.
5. [ ] Click **Generate Export**.
6. [ ] Wait for the export to complete (status transitions to `ready`).
7. [ ] Download the exported file.
8. [ ] Verify the export contains:
   - Household contact information
   - Swimmer profile data
   - Registration history
   - Invoice and payment history
9. [ ] Mark the request as `delivered` and record the delivery method.
10. [ ] Verify the action is logged in the audit trail with the actor's user ID and timestamp.
11. [ ] Verify the exported data does not contain information from other households (data isolation check).

### Pass Criteria

- Export request created and processed without errors.
- Downloaded export contains all required data categories.
- Export does not contain data for other households.
- Delivery and audit logging complete successfully.

### Sign-Off

| | |
|---|---|
| Validator Name | |
| Date Completed | |
| Environment | Staging |
| Result | Pass / Fail |
| Notes | |

---

## Workflow 7: Season Close

**Description:** Archive completed meets, run final season reports, create a data backup, and transition to the new season.

**Owner:** Club Admin Lead + Club Treasurer
**Backup Validator:** Product Manager

### Steps to Validate

1. [ ] Verify all meets for the season are in `completed` or `archived` state. Any open meets must be completed first.
2. [ ] Verify all invoices for the season are resolved: `paid`, `voided`, or explicitly marked uncollectible with an audit reason.
3. [ ] Navigate to **Reports** and run the following final reports for the season:
   - [ ] Membership Status (CSV) — verify row count matches known active members
   - [ ] Meet Fees (CSV) — verify totals match expected revenue
   - [ ] Invoice Aging (CSV) — confirm no unexpected outstanding balances
   - [ ] Registration Status (CSV) — confirm all registrations are in a terminal state
4. [ ] Save all four reports to a secure location (club file server or designated storage).
5. [ ] Archive meets from the season: navigate to each `completed` meet and click **Archive**.
6. [ ] Navigate to **System → Seasons** and set the current season status to `completed`.
7. [ ] Verify the new season (created in Workflow 1) transitions to `active` or set it manually.
8. [ ] Confirm that the new season's groups are active and visible to parents.
9. [ ] Send a season-close announcement via **Comms → Announcements** if applicable.
10. [ ] Record the season close date and final report archive location in the club's records.

### Pass Criteria

- All four season-end reports generate without error and have reasonable row counts.
- All meets archived successfully.
- Season status updated to `completed`.
- New season is active and visible in the parent portal.
- No orphaned open invoices or uncommitted entries remain.

### Sign-Off

| | |
|---|---|
| Validator Name | |
| Date Completed | |
| Environment | Staging |
| Result | Pass / Fail |
| Notes | |

---

## Summary Sign-Off Table

All seven workflows must show **Pass** before production cutover is approved.

| Workflow | Owner | Status | Sign-Off Date |
|---|---|---|---|
| 1. New Season Setup | Club Admin Lead | Open | |
| 2. Meet Setup and Commitment Cycle | Club Admin Lead + Head Coach | Open | |
| 3. Results Import | Club Admin Lead | Open | |
| 4. Monthly Billing Cycle | Club Treasurer | Open | |
| 5. New Family Onboarding | Club Admin Lead | Open | |
| 6. Data Export for Compliance | Club Admin Lead | Open | |
| 7. Season Close | Club Admin Lead + Club Treasurer | Open | |

**Final approval:** All workflows signed off → Engineering Lead marks STEP 29L validation complete → cutover proceeds per `docs/runbooks/cutover.md`.
