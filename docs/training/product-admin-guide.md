# Product Admin Guide — DARE Platform

**Audience:** Club administrators, operations staff, coaches
**Version:** 1.0
**Last Updated:** 2026-03-23

---

## Overview

DARE is your club's all-in-one operations platform. It replaces TeamUnify/SportsEngine Motion for member management, meet scheduling, billing, communications, and performance tracking. This guide covers the day-to-day tasks that admins and coaches perform.

---

## 1. Member Management

### 1.1 Adding a New Household

1. Navigate to **Members → Households** in the left sidebar.
2. Click **Add Household** in the top-right corner.
3. Fill in the household details:
   - Primary contact name, email, and phone
   - Address
4. Click **Save Household**.
5. The household is created with `active` status. You can now add swimmers to it.

### 1.2 Creating a Swimmer

1. Navigate to **Members → Swimmers**.
2. Click **Add Swimmer**.
3. Fill in:
   - First name, last name
   - Date of birth
   - Gender
   - Household (select the parent household)
   - USA Swimming member ID (if known)
4. Click **Save Swimmer**.

### 1.3 Managing Groups

1. Navigate to **Members → Groups**.
2. To create a group, click **New Group** and enter the group name, description, and eligibility rules (min/max age, gender, registration season).
3. To add swimmers to a group, open the group detail and use the **Add Swimmer** action.
4. Groups drive meet commitment invitations — swimmers must be in a group to receive commitment requests.

### 1.4 Handling Registrations

1. Navigate to **Members → Registrations**.
2. Review pending registrations in the **Pending** tab.
3. To approve: click the registration row and click **Approve**.
4. To reject: click **Reject** and enter a reason (required for audit trail).
5. Approved registrations trigger onboarding emails and billing.

---

## 2. Meet Management

### 2.1 Creating a Meet

1. Navigate to **Meets** and click **New Meet**.
2. Fill in **Basic Info**: meet name, host club, location, pool type (SCY/SCM/LCM), entry deadline.
3. Optionally upload meet package files (PDF, heat sheet) and a meet entry file (.cl2/.sd3/.zip/.hy3). Uploading a meet entry file auto-populates the sessions in the next step.
4. In the **Schedule** step, add sessions using the **Add Session** button. For each session, enter:
   - Day label (e.g., Day 1)
   - Session name
   - Date
   - Warm-up time
   - Start time
5. In the **Events** step, add events per session. Specify distance, stroke, gender, age group, and entry fee.
6. Click **Publish Meet**. The meet is now visible to coaches.

### 2.2 Opening Commitments

1. Open the meet detail page.
2. Click **Open Commitments**. This transitions the meet to `commitment_open` state.
3. Parents will see the meet on their portal with **Yes/No** commitment buttons.
4. You can set a commitment deadline — the system will auto-close commitments at that time.

### 2.3 Reviewing Commitments

1. Navigate to **Commitments** in the sidebar.
2. Use the meet filter to see commitments for a specific meet.
3. Click a swimmer's name to open the detail panel showing their commitment status, family note, and event selections.
4. Coaches can override event selections before entries are locked. Every override is logged in the audit trail with a mandatory reason.

### 2.4 Finalizing Entries

1. From the meet detail, click **Close Commitments** when the commitment window ends.
2. Review the entry list. Make any final coach adjustments.
3. Click **Finalize Entries**. This transitions the meet to `entry_finalized` state and locks all entries.
4. After finalization, entries can be exported as an SD3 file for submission.

### 2.5 Exporting SD3

1. From the finalized meet detail, click **Export Entries (SD3)**.
2. The system generates the SD3 file in the background. You will see a loading indicator.
3. When the export is `ready`, a download button appears. Click to download the `.sd3` file.
4. Submit the SD3 file to the meet host per their instructions.

---

## 3. Import/Export (Meet Results)

### 3.1 Uploading a Meet Result File

1. Navigate to **Imports**.
2. Click **Upload File**.
3. Select your file (.cl2, .sd3, .zip, or .hy3 format).
4. The system uploads the file and begins parsing. Status transitions: `uploaded → scanning → parsing → matching`.
5. If there are no conflicts, the import moves to `applied` automatically.
6. If there are conflicts (swimmer matching ambiguities), the import moves to `conflicts_pending`.

### 3.2 Resolving Import Conflicts

1. Open the import from the **Imports** list.
2. The **Conflicts** tab shows each ambiguous record. For each conflict, you can:
   - **Match**: select the correct swimmer from a search
   - **Auto-match**: accept the system's best suggestion
   - **Reject**: skip this record
3. Once all conflicts are resolved, click **Apply Import**.
4. Best times are updated immediately for all applied results.

### 3.3 Downloading Export Files

1. Navigate to **Exports**.
2. Find the export by meet name or date.
3. Click **Download** to download the SD3 or ZIP file.
4. Exports expire after 7 days. If a download link is expired, re-generate the export from the meet detail.

---

## 4. Billing

### 4.1 Creating an Invoice

1. Navigate to **Invoices**.
2. Click **New Invoice**.
3. Select the household and add line items (description, amount, quantity).
4. Set a due date.
5. Click **Issue Invoice**. The invoice transitions from `draft` to `issued`.
6. Optionally click **Send Invoice Email** to notify the family.

### 4.2 Managing Payment Plans

1. Open an existing invoice.
2. Click **Create Payment Plan**.
3. Specify the installment count and frequency.
4. The system splits the balance into sub-invoices on a schedule.

### 4.3 Processing Refunds

Refunds require admin or owner role plus MFA verification.

1. Open the invoice and find the payment to refund.
2. Click **Refund**.
3. Enter the refund amount and reason.
4. Complete the MFA prompt.
5. For Stripe payments, the refund is processed via Stripe automatically. For manual payments, record the refund manually.

### 4.4 Reviewing the Ledger

1. Navigate to **Billing → Ledger**.
2. The ledger shows all append-only financial events: invoices, payments, refunds, adjustments.
3. Use the date filter and household filter to scope the view.
4. The ledger is immutable — no entries can be deleted or edited.

---

## 5. Communications

### 5.1 Creating an Announcement

1. Navigate to **Comms → Announcements**.
2. Click **New Announcement**.
3. Enter title, body, and target audience (all members, specific group, etc.).
4. Click **Publish** to post immediately, or **Save Draft** to publish later.
5. Published announcements appear in the parent portal **Communications** section.

### 5.2 Building an Email Campaign

1. Navigate to **Comms → Campaigns**.
2. Click **New Campaign**.
3. Set the subject, from name, and HTML/text body.
4. Select recipients (by group or all active households).
5. Click **Send Campaign** or **Schedule** for a future date.
6. Track delivery status and open rates in the campaign detail.

### 5.3 Managing the Suppression List

1. Navigate to **Comms → Suppression**.
2. View all suppressed email addresses (bounced, spam complaints, unsubscribed).
3. To manually suppress an email: click **Add Suppression** and enter the email and reason.
4. To remove a suppression (if the address is valid and the family requests it): click the row and click **Remove Suppression**.

---

## 6. Reports

Navigate to **Reports**. Available reports:

| Report | Description | Export |
|---|---|---|
| Membership Status | Active, inactive, and pending members by group | CSV |
| Meet Fees | Fees collected per meet, per swimmer | CSV |
| Invoice Aging | Outstanding balances by age bucket (30/60/90+ days) | CSV |
| Payment Reconciliation | Stripe vs DARE balance comparison | CSV |
| Registration Status | Registration completion by season and group | CSV |

To run a report:
1. Select the report type.
2. Set date range and filters.
3. Click **Generate Report**.
4. Download the CSV or view the summary table on screen.

---

## 7. Document Center

### 7.1 Managing Document Types

1. Navigate to **Compliance → Documents**.
2. Click the **Document Types** tab.
3. To add a new required document (e.g., "2026 Liability Waiver"): click **New Document Type** and enter name, description, and whether it is required.
4. To archive a document type: click the type and click **Archive**. Existing submissions are retained.

### 7.2 Reviewing Submissions

1. In the **Document Types** tab, click a type to see all submissions.
2. Or use the **Submissions** tab to filter across all types.
3. To verify a submission: open the detail, review the attached file, and click **Verify**.
4. To reject: click **Reject** and enter a reason. The parent will be notified and can re-upload.

---

## 8. Training / Practices

### 8.1 Logging a Practice

1. Navigate to **Training → Schedule**.
2. Click **Log Practice**.
3. Fill in:
   - Date
   - Group
   - Practice type (regular, optional, dryland, etc.)
   - Description
   - Total yardage target
   - Attendance count
   - Per-stroke yardage breakdown (freestyle, backstroke, etc.)
4. Click **Save Practice**.

### 8.2 Viewing Weekly Stats

1. In **Training → Stats**, select the week.
2. Cards show per-group totals: total yardage, attendance, and stroke breakdown.

---

## 9. Tips and Common Mistakes

### Do Not

- **Do not delete a committed entry.** If a swimmer's entry needs to be removed after commitments are locked, use a coach override with an audit reason. Deleting directly bypasses the audit trail.
- **Do not archive an active season while it has open registrations or unpaid invoices.** Complete billing and close registrations first.
- **Do not re-use import files.** If you need to re-import a file, create a new import job. Re-uploading to an existing job that has already been applied will create duplicate results.
- **Do not manually edit the ledger.** The billing ledger is append-only. If an entry is wrong, create a correcting entry (refund or adjustment), not an edit.
- **Do not bypass MFA for refunds.** The MFA prompt on refunds is a security requirement, not an optional step.

### Common Mistakes

- **Wrong org ID in API calls:** If you are using the API directly, always include `X-Org-Id` header. The most common cause of 403 errors is a mismatched org ID.
- **Forgot to finalize entries before exporting SD3:** The export button is only available in `entry_finalized` state. If you cannot export, check the meet lifecycle state.
- **Commitment window closed before all parents responded:** You can reopen commitments briefly (owner/admin only) and notify affected families. Every reopen is logged in the audit trail.
