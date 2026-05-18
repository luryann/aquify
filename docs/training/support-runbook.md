# Support Incident Playbook — DARE Platform

**Audience:** Tier 1 support staff, club administrators, on-call engineers
**Version:** 1.0
**Last Updated:** 2026-03-23

---

## How to Use This Playbook

Each incident section follows this format:
1. **Symptoms** — what the user reports
2. **Diagnostic steps** — how to investigate
3. **Resolution** — how to fix it
4. **Escalation** — when and how to involve engineering

Use the incident number to reference in tickets and Slack threads (e.g., "following INC-3 webhook playbook").

---

## INC-1: Import Fails or Stuck in Parsing State

### Symptoms

- Admin uploaded a file but the import status shows `parsing`, `scanning`, or `matching` for more than 10 minutes.
- Import status shows `failed`.
- Error message in the import detail: "parsing error", "unsupported format", or no message at all.
- The import was applied but some swimmers are not showing updated best times.

### Common Causes

- Corrupt or truncated file
- Wrong encoding (import expects UTF-8 or ASCII for SD3/CL2)
- File extension does not match actual format (e.g., `.sd3` file that is actually a ZIP)
- HY3 inner payload has conflicting SD3 and CL2 files
- Meet file has swimmer IDs that do not exist in DARE

### Diagnostic Steps

1. **Check import status via the admin UI:**
   - Navigate to **Imports** and open the stuck import.
   - Note the `status` field and any error message in the **Errors** tab.

2. **Check import status via the API:**
   ```bash
   curl -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "X-Org-Id: $ORG_ID" \
        https://api.dare.club/api/v1/orgs/$ORG_ID/imports/$IMPORT_ID
   ```
   Look for `"errors"` array in the response.

3. **Check the background job queue for failures:**
   ```bash
   curl -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "X-Org-Id: $ORG_ID" \
        https://api.dare.club/api/v1/admin/jobs/dlq
   ```
   Look for a `parse_import` task with the import ID in the payload.

4. **Check API logs for parser errors:**
   ```bash
   docker compose logs dare-api | grep '"import_id":"<import_id>"'
   docker compose logs dare-worker | grep '"import_id":"<import_id>"'
   ```

### Resolution

**For a failed import with known errors:**
1. Download the error report from the import detail (if available).
2. Identify the specific line/record causing the failure.
3. Guide the user to:
   - Correct the file in their source system.
   - Upload a new import (do not re-use the failed import job).
4. After uploading the corrected file, the new import should proceed normally.

**For a stuck import (no error, no progress):**
1. Check if a background worker is running: `docker compose ps dare-worker`
2. If the worker is not running: `docker compose restart dare-worker`
3. The stuck import should resume processing within 1–2 minutes.
4. If it does not resume, the import may be orphaned. Escalate to engineering.

**For a DLQ task:**
1. Retry the task via the API:
   ```bash
   curl -X POST \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "X-Org-Id: $ORG_ID" \
        https://api.dare.club/api/v1/admin/jobs/dlq/$TASK_ID/retry
   ```
2. Monitor the import status for the next 2 minutes.

### Escalation

Escalate to engineering if:
- The parser exits with a 500 error on a valid file.
- The worker is crashing repeatedly (check restart count in `docker compose ps`).
- The import is stuck after a worker restart.
- Multiple imports are stuck simultaneously.

Include in the escalation: the import ID, the file name, the error message (if any), and the DLQ task ID (if applicable).

---

## INC-2: Parent Can't Log In

### Symptoms

- Parent reports they cannot access the portal.
- Browser shows 401 Unauthorized, redirect loop, or a blank white screen after login.
- Parent reports "MFA code is not working".
- Parent reports "I never receive the email to log in".

### Diagnostic Steps

**For 401 / redirect loop:**
1. Confirm which environment the parent is on (staging vs production URL).
2. Check if the parent's Keycloak account is active:
   - Keycloak admin console → Users → search by email.
   - Verify `Enabled = ON` and `Email Verified = ON`.
3. Check if the parent's session is stuck:
   ```bash
   # Check audit log for recent login events for this user
   curl -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "X-Org-Id: $ORG_ID" \
        "https://api.dare.club/api/v1/orgs/$ORG_ID/audit?user_id=<user_id>&limit=10"
   ```

**For MFA issues:**
1. Ask if the parent is using a TOTP authenticator app or backup codes.
2. If TOTP: verify the app's clock is synchronized (time drift > 30s causes TOTP failures).
3. If backup codes: confirm they are using an unused code.
4. Check enrolled MFA factors:
   ```bash
   curl -H "Authorization: Bearer $PARENT_TOKEN" \
        https://api.dare.club/api/v1/auth/mfa/factors
   ```

**For email not received:**
1. Check the Postmark activity log for the email address.
2. Check DARE suppression list: **Comms → Suppression** — search for the parent's email.

### Resolution

**Account disabled or not verified:**
1. In Keycloak admin, enable the account and set `Email Verified = ON`.
2. Ask the parent to try logging in again.

**Stuck session:**
1. In Keycloak admin, open the user's Sessions tab and click **Logout All Sessions**.
2. Ask the parent to clear browser cookies and try again.

**MFA TOTP issues:**
1. If the parent can use backup codes: have them log in with a backup code and reconfigure their authenticator app.
2. If no backup codes available: in Keycloak admin, remove the TOTP factor for the user. The parent will be prompted to re-enroll on next login.

**Email in suppression list:**
1. Remove the suppression: **Comms → Suppression** → find row → **Remove Suppression**.
2. Trigger a new invite email via Keycloak admin (Reset Password action).

**Auth service down:**
If Keycloak is unreachable or returning 500 errors, escalate to engineering immediately. This is a P0 incident.

### Escalation

Escalate to engineering if:
- Keycloak admin console is unreachable.
- Multiple parents report the same login failure simultaneously (possible auth service outage).
- The OIDC redirect is looping (300+ redirects in browser network tab).
- Parent's account appears correct in Keycloak but login still fails.

---

## INC-3: Stripe Payment Webhook Missed

### Symptoms

- Parent reports payment went through on their credit card but invoice still shows `issued` in the portal.
- Stripe Dashboard shows the payment as `succeeded` with a charge ID.
- DARE ledger shows no payment record for the invoice.

### Diagnostic Steps

1. **Confirm the payment in Stripe:**
   - Log in to the Stripe Dashboard.
   - Navigate to Payments → All Transactions.
   - Search by parent email or card last 4.
   - Note the `payment_intent_id` (format: `pi_XXXX`).

2. **Check webhook delivery in Stripe:**
   - Navigate to Stripe Dashboard → Developers → Webhooks.
   - Find your DARE webhook endpoint.
   - Under **Recent Deliveries**, look for a `payment_intent.succeeded` event for the payment intent ID.
   - If the delivery shows "Failed" or has no entry, the webhook was missed.

3. **Check DARE audit log for the webhook event:**
   ```bash
   curl -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "X-Org-Id: $ORG_ID" \
        "https://api.dare.club/api/v1/orgs/$ORG_ID/audit?category=billing&limit=50"
   ```
   Search for the payment intent ID in the event metadata.

4. **Check the webhook ingestion health:**
   ```bash
   curl https://api.dare.club/health/ready
   ```
   The API must be healthy for webhooks to be received.

### Resolution

**For a single missed webhook:**
1. Open the invoice in DARE.
2. Click **Record Payment**.
3. Enter:
   - Amount: same as the Stripe charge amount
   - Method: `stripe`
   - Reference: the Stripe `payment_intent_id` (pi_XXXX)
   - Payment date: the Stripe charge date
4. The invoice transitions to `paid`.
5. Notify the parent that their payment has been recorded.

**To trigger a retry from Stripe:**
- In Stripe Dashboard → Developers → Webhooks → Recent Deliveries, find the failed delivery and click **Resend**.
- This replays the webhook. Verify DARE processes it without creating a duplicate (idempotency key check).

### Escalation

Escalate to engineering if:
- Multiple consecutive payments are missed (possible webhook endpoint failure).
- The Stripe webhook endpoint returns 500 or is unreachable.
- The manual payment recording fails with an error.
- Webhook signature verification is failing (check API logs for `invalid webhook signature`).

---

## INC-4: Billing Discrepancy

### Symptoms

- Parent calls to report the invoice total does not match what was discussed or what they expected.
- Admin reports the ledger balance does not add up.
- Reconciliation report shows a non-zero discrepancy.

### Diagnostic Steps

1. **Pull the ledger events for the household:**
   ```bash
   curl -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "X-Org-Id: $ORG_ID" \
        "https://api.dare.club/api/v1/orgs/$ORG_ID/billing/ledger?household_id=<household_id>"
   ```

2. **Compare ledger events with invoice line items:**
   - Open the invoice in the admin UI.
   - Review each line item: description, amount, and date.
   - Verify the total matches the sum of line items.

3. **Compare with Stripe charges:**
   - In Stripe Dashboard, search by parent email.
   - List all charges for the past 90 days.
   - Compare with DARE payment records.

4. **Run the reconciliation report for the relevant period:**
   - Navigate to **Reports → Payment Reconciliation**.
   - Set date range to cover the disputed period.
   - Look for unreconciled rows.

### Resolution

**For a line item error (wrong amount on invoice):**
1. If the invoice is still `draft`: edit the line item directly.
2. If the invoice is `issued` or later: create a correcting invoice (credit memo) with a negative line item for the error amount, with a clear description ("Credit: billing correction for invoice INV-XXXX").

**For a duplicate charge:**
- See Section 8.2 of the Finance Guide (Duplicate Payment).

**For a Stripe vs DARE discrepancy:**
- See INC-3 resolution or manually reconcile as described in Finance Guide Section 4.

### Escalation

Involve Finance (treasurer/finance admin) if:
- The discrepancy is greater than $10.
- The discrepancy involves a refund that was processed in Stripe but not recorded in DARE.
- The parent is requesting a formal explanation or credit.

Escalate to engineering if:
- The ledger shows events that cannot be explained by any user action in the audit log.
- The reconciliation report fails to generate.
- Ledger totals are mathematically inconsistent (balance ≠ sum of events).

---

## INC-5: Document Verification Stuck

### Symptoms

- Parent uploaded a document but admin cannot see it in the **Compliance → Documents** review queue.
- Document submission shows `pending` even though the parent says they uploaded a file.
- Admin clicks **Verify** but nothing happens or an error appears.

### Diagnostic Steps

1. **Check the submission status via the API:**
   ```bash
   curl -H "Authorization: Bearer $ADMIN_TOKEN" \
        -H "X-Org-Id: $ORG_ID" \
        "https://api.dare.club/api/v1/documents?household_id=<household_id>"
   ```
   Look for the submission and its `status` field.

2. **Check if the file was attached:**
   - A submission in `pending` state has no file attached (the parent created the submission but did not complete the upload).
   - A submission in `submitted` state has a file attached and is ready for review.
   - If the submission is still `pending`, the parent did not complete the upload flow.

3. **Check object storage connectivity:**
   ```bash
   # MinIO/S3 health check
   curl https://minio.dare.club/minio/health/live
   ```
   If storage is down, file uploads will fail silently on the parent's side.

4. **Check API logs for upload errors:**
   ```bash
   docker compose logs dare-api | grep '"document_submission_id":"<submission_id>"'
   ```

### Resolution

**Parent did not complete the upload (submission stuck in `pending`):**
1. Contact the parent and ask them to:
   - Navigate to **Portal → Documents**.
   - Find the document type that requires submission.
   - Click **Upload Document** (or the existing pending submission) and complete the file upload.
2. After upload, the submission transitions to `submitted` and appears in the admin review queue.

**Admin can't see the submission (display issue):**
1. Ask the admin to hard-refresh the page (Ctrl+Shift+R / Cmd+Shift+R).
2. Verify the admin is filtering to the correct document type and household.

**Object storage (MinIO/S3) is down:**
1. Check MinIO health: `docker compose ps minio`
2. Restart if needed: `docker compose restart minio`
3. Ask the parent to retry the upload after confirming storage is healthy.
4. Escalate to engineering if MinIO cannot be restarted.

### Escalation

Escalate to engineering if:
- MinIO/S3 is unreachable and cannot be restarted.
- File uploads are failing with 500 errors.
- A submission shows `submitted` with a file ID but the file cannot be downloaded (broken presigned URL or missing object).
- Multiple households report the same upload issue simultaneously.
