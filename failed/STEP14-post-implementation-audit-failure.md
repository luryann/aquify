# STEP 14 Post-Implementation Audit Failure

**Date:** 2026-03-16
**Step:** STEP 14 — Stripe Integration
**Verdict:** FAIL — Conditions 1 (bugs/edge cases), 2 (code quality), and 3 (security) are not fully met.

---

## Condition Results

| Condition | Result | Reason |
|-----------|--------|--------|
| 1. No bugs / edge cases | **FAIL** | 3 confirmed bugs (Issues 1, 4, 5) |
| 2. No code quality regressions | **FAIL** | 2 quality issues (Issues 3, 6) |
| 3. No security issues | **FAIL** | 2 security-relevant findings (Issues 1, 2) |
| 4. Followed all directions / completeness | **PASS** | All 11 TODO tasks implemented |

---

## Issue 1 — Security + Bug: `hmac.Equal` compares hex strings, not raw bytes

**File:** `server/internal/stripe/client.go`, lines ~189–192
**Severity:** High (security-relevant)

### What went wrong
`VerifyStripeSignature` computes an HMAC-SHA256 digest and hex-encodes it into `expected`. It also reads raw hex strings from the `Stripe-Signature` header into `v1Sigs`. The comparison is:

```go
if hmac.Equal([]byte(sig), []byte(expected)) {
```

Both `sig` and `expected` are hex-encoded strings. `hmac.Equal` is designed to compare **raw MAC byte arrays** of fixed, equal length to guarantee constant-time operation. When comparing two hex strings:
- If `sig` (from the header) is malformed or truncated, it will have a different byte length than `expected` (always 64 hex chars). Go's `hmac.Equal` short-circuits and returns false **without** constant-time guarantees on length mismatch, creating a timing oracle that could be exploited to determine partial HMAC prefix validity.
- Even in the non-attack case, this is semantically wrong: you are comparing string representations rather than cryptographic values.

### How to fix
Decode both sides from hex before comparing. Replace the comparison block in `VerifyStripeSignature`:

```go
import "encoding/hex"

for _, sig := range v1Sigs {
    sigBytes, err := hex.DecodeString(sig)
    if err != nil {
        continue // malformed hex — skip, don't short-circuit
    }
    mac := hmac.New(sha256.New, []byte(sc.webhookSecret))
    mac.Write([]byte(signedPayload))
    expectedBytes := mac.Sum(nil)
    if hmac.Equal(sigBytes, expectedBytes) {
        return nil
    }
}
```

Note: compute `expectedBytes` as raw bytes (not hex-encoded), and `sigBytes` as decoded raw bytes. Both are now always 32 bytes, guaranteeing constant-time comparison regardless of header content.

### What counts as passing
The comparison in `VerifyStripeSignature` must use `hmac.Equal` on two raw `[]byte` values of equal fixed length (32 bytes for SHA-256). The hex-encoded string comparison must be removed.

---

## Issue 2 — Security: Internal error message leaked to Stripe in webhook response body

**File:** `server/internal/app/handlers/stripe.go`, lines ~336–346
**Severity:** Medium (information disclosure)

### What went wrong
When a webhook event is persisted but fails to process, the handler returns HTTP 200 with:

```go
httpx.WriteJSON(w, http.StatusOK, map[string]any{
    "status": "persisted_with_error",
    "error":  processErr.Error(),
})
```

Stripe receives the full internal error string (e.g., `"invoice not found"`, `"payment exceeds invoice balance"`) verbatim. These strings appear in Stripe's dashboard webhook delivery logs and are accessible to Stripe support staff. This is an unnecessary information disclosure of internal domain state.

### How to fix
Remove the `error` field from the response body. Stripe does not read or use the response body; only the status code matters for retry logic. The error is already logged with `slog` on the line above this block.

```go
httpx.WriteJSON(w, http.StatusOK, map[string]any{
    "status": "persisted_with_error",
})
```

### What counts as passing
The `persisted_with_error` response body must not contain any internal error message string. The error must be logged server-side only.

---

## Issue 3 — Code Quality: Misleading comment in `HandleStripeChargeRefunded` creates future deadlock trap

**File:** `server/internal/app/billing/service.go`, line ~1031
**Severity:** Medium (maintenance/correctness)

### What went wrong
The function comment states:

> "It is idempotent via the existing refund dedup logic in `CreateRefund`"

This is false. `HandleStripeChargeRefunded` does **not** call `CreateRefund`. It re-implements the entire refund creation path inline while holding `s.mu.Lock()`. It cannot call `CreateRefund` because `CreateRefund` also acquires `s.mu.Lock()`, which would deadlock. When this code is migrated to a real DB layer, a developer following the comment may attempt to refactor to call `CreateRefund`, resulting in a deadlock under load.

### How to fix
Replace the misleading comment with a correct one that explains why the logic is duplicated:

```go
// HandleStripeChargeRefunded processes a charge.refunded webhook event idempotently.
// NOTE: This method acquires s.mu.Lock() for the entire operation and therefore CANNOT
// delegate to CreateRefund (which also acquires s.mu.Lock()). The refund creation logic
// is intentionally duplicated here. When migrating to a real DB layer, use a shared
// internal helper that does NOT acquire the lock (lock at the service boundary only).
```

### What counts as passing
The comment on `HandleStripeChargeRefunded` must accurately describe why the refund logic is duplicated inline and must warn future developers not to replace it with a call to `CreateRefund` without first refactoring the locking strategy.

---

## Issue 4 — Bug: `RecordWebhookEvent` never populates `OrgID` on `WebhookEventRecord`

**File:** `server/internal/app/billing/service.go`, lines ~892–901
**Severity:** High (data integrity — breaks multi-tenant production wiring)

### What went wrong
`WebhookEventRecord` has an `OrgID` field, and the DB schema's `webhook_events` table has an `org_id` column. However, `RecordWebhookEvent` never sets `OrgID`:

```go
s.webhookEvents[key] = &WebhookEventRecord{
    ID:        newID(),
    Provider:  provider,
    EventID:   eventID,
    EventType: eventType,
    Payload:   cp,
    Processed: false,
    CreatedAt: time.Now().UTC(),
    // OrgID: never set
}
```

The `ListWebhookEvents` method acknowledges this with `_ = orgID` and a comment: "Webhook events are global (not per-org) in the stub." But the `RecordWebhookEvent` method signature does not accept an `orgID` parameter, so there is no way to wire in an org ID even when replacing with a real DB layer without a signature change.

The handler calling `RecordWebhookEvent` (in `stripe.go:318`) does have access to `orgID` from `event.Metadata["org_id"]` — but cannot pass it to the method.

In production with a real DB, this would write `org_id = NULL` to every webhook event row, violating any NOT NULL constraint on `org_id` and breaking multi-tenant isolation.

### How to fix
Add `orgID string` as a parameter to `RecordWebhookEvent` and populate it in the struct:

```go
func (s *Service) RecordWebhookEvent(provider, orgID, eventID, eventType string, payload []byte) (*WebhookEventRecord, error) {
    ...
    s.webhookEvents[key] = &WebhookEventRecord{
        ...
        OrgID: orgID,
        ...
    }
}
```

Update the handler call site to pass `orgID` extracted from `event.Metadata`.

Update `ListWebhookEvents` to filter by `orgID` when non-empty (removing the `_ = orgID` stub).

### What counts as passing
`RecordWebhookEvent` accepts `orgID` as a parameter, populates it on the stored record, and the handler passes the actual `orgID` from the webhook event metadata. `ListWebhookEvents` must filter by `orgID` when non-empty.

---

## Issue 5 — Bug: Missing-metadata `payment_intent.succeeded` silently marked as `processed=true`

**File:** `server/internal/app/handlers/stripe.go`, lines ~378–387
**Severity:** High (billing integrity)

### What went wrong
When a `payment_intent.succeeded` event arrives with no `org_id` or `invoice_id` in metadata, `onPaymentIntentSucceeded` returns `nil` (no error):

```go
if orgID == "" || invoiceID == "" {
    h.logger.Warn("stripe webhook: payment_intent.succeeded missing metadata; cannot link to invoice",
        "payment_intent_id", paymentIntentID)
    return nil // soft failure — cannot route without metadata
}
```

Returning `nil` causes the webhook handler to call `MarkWebhookProcessed("stripe", event.ID, nil)` — marking the event as successfully processed. This means:

1. The event is consumed and will never be reprocessed (idempotency key is spent).
2. Money received cannot be attributed to any invoice.
3. The reconciliation report will show a mismatch (Stripe balance includes this payment; ledger does not) with no trace of why.
4. The event is invisible to operators — it appears as a normal successful webhook.

The same problem exists in `onChargeRefunded` when metadata is missing.

### How to fix
Return a non-nil error so the event is marked `processed=false` and is visible for operator investigation:

```go
if orgID == "" || invoiceID == "" {
    h.logger.Error("stripe webhook: payment_intent.succeeded missing metadata; payment cannot be attributed",
        "payment_intent_id", paymentIntentID)
    return fmt.Errorf("payment_intent.succeeded event %s missing org_id or invoice_id metadata — manual attribution required", paymentIntentID)
}
```

This causes the handler to respond with `HTTP 200` + `"status": "persisted_with_error"` (Stripe will not retry) but marks the event as unprocessed, making it visible in the webhook events list for manual investigation.

Apply the same fix to `onChargeRefunded`.

### What counts as passing
A `payment_intent.succeeded` event with missing metadata must return a non-nil error from `onPaymentIntentSucceeded`, causing the event to be stored as `processed=false`. The handler must still return HTTP 200 to prevent Stripe retry. The same must apply to `onChargeRefunded`. Operators querying `GET /api/v1/stripe/webhook-events` must be able to see and investigate unprocessed events.

---

## Issue 6 — Code Quality: Handler-layer role check duplicates service-layer role check

**File:** `server/internal/app/handlers/stripe.go`, lines ~124–132 and `server/internal/app/billing/service.go`, lines ~260–268
**Severity:** Low (pattern deviation, maintenance risk)

### What went wrong
The `stripe.go` handler defines and calls `requireBillingAccess` to check that the caller is `admin | owner | billing` before calling into the service. The billing service also has `requireBillingRole` performing the same check. Every other handler in the codebase (billing, meets, groups, imports) delegates role enforcement entirely to the service layer and does not re-check at the handler level.

This creates two sources of truth for the same role check. If the billing role policy changes (e.g., `coach` gets billing read access), the change must be made in both places or the handler will block what the service would allow.

### How to fix
Remove the `requireBillingAccess` pre-check from the Stripe handlers. Rely on the service-layer checks exclusively, consistent with all other handlers in the codebase.

### What counts as passing
Stripe handlers do not perform their own role validation before calling into the service. All role enforcement is delegated to the service layer, consistent with the pattern established in `billing`, `meets`, `groups`, and `imports` handlers.

---

## Master List of Files Edited/Created in STEP 14

| File | Operation |
|------|-----------|
| `server/internal/stripe/client.go` | Created |
| `server/internal/app/billing/service.go` | Modified (STEP 14 additions: WebhookEventRecord, stripeIntentIndex, HandleStripe* methods, GetReconciliationReport) |
| `server/internal/app/handlers/stripe.go` | Created |
| `server/internal/app/server.go` | Modified (added Stripe wiring, RegisterPublicRoutes, 11th NewRouterWithServices parameter) |
| `server/internal/app/config.go` | Modified (added StripeConfig struct and env var loading) |
| `server/internal/app/server_stripe_test.go` | Created |
| `docs/planning/STEP14-implementation-plan.md` | Created |
