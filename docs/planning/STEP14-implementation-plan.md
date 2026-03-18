# STEP 14: Stripe Integration — Implementation Plan

## Overview
Wire in Stripe as the payment processor. This step adds:
- A Stripe client interface + in-memory stub (follows StubStorage pattern)
- Webhook event persistence with idempotency/dedup protection
- Handlers for creating PaymentIntents and CheckoutSessions
- A public (unauthenticated) webhook endpoint with Stripe signature verification
- Idempotent handling of `payment_intent.succeeded`, `payment_intent.payment_failed`, and `charge.refunded`
- An on-demand reconciliation endpoint comparing ledger Stripe totals to a Stripe balance report
- Alert trigger (logged + returned) when mismatch exceeds configured threshold

## Design Decisions

### No external SDK dependency (consistent with StubStorage pattern)
- Define a `stripe.Client` interface under `server/internal/stripe/`
- `StubClient` implements the interface in-memory for tests/dev
- Webhook signature verification is implemented from scratch using `crypto/hmac` + SHA-256
  (Stripe's algorithm is fully documented and straightforward)
- A real SDK-backed client can be dropped in later without touching handlers or services

### Webhook endpoint is public (no auth middleware)
- Stripe POSTs to `/api/v1/webhooks/stripe` directly; it cannot provide Bearer tokens
- Security is enforced via `Stripe-Signature` header HMAC verification
- Uses `RegisterPublicRoutes` pattern (same as auth handlers)

### Idempotency key
- `webhook_events` table schema uses UNIQUE(provider, event_id)
- In-memory: map key `"stripe|<event_id>"`
- If a duplicate arrives → respond 200 OK immediately without re-processing

### Payment state linkage
- When creating a PaymentIntent, `metadata` includes `invoice_id` and `org_id`
- Webhook handler reads those fields back from event metadata
- `billing.Service` gains a secondary index `stripeIntentIndex map[string]*Payment` (paymentIntentID → Payment)

### Reconciliation
- Endpoint: `GET /api/v1/stripe/reconciliation` (admin/billing only)
- Sums all `payment_succeeded` ledger events for payments with `method=stripe`
- Calls `StripeClient.FetchBalanceCents()` to get Stripe's view of net charges
- Returns `ReconciliationReport` with mismatch amount and `alert_triggered` flag
- In the stub, `FetchBalanceCents()` returns a configurable value (default 0)

## New Files
1. `server/internal/stripe/client.go` — Client interface, types, StubClient, VerifyStripeSignature
2. `server/internal/app/handlers/stripe.go` — StripeHandlers, routes
3. `server/internal/app/server_stripe_test.go` — Tests

## Modified Files
1. `server/internal/app/billing/service.go` — WebhookEventRecord type + new methods
2. `server/internal/app/config.go` — StripeConfig struct + env loading
3. `server/internal/app/server.go` — Wire StripeHandlers (public + protected routes)
4. `TODO.md` — Mark tasks complete

## New API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/stripe/payment-intents` | protected (billing) | Create Stripe PaymentIntent |
| POST | `/api/v1/stripe/checkout-sessions` | protected (billing) | Create Stripe Checkout Session |
| POST | `/api/v1/webhooks/stripe` | **public** (sig-verified) | Receive Stripe webhook |
| GET | `/api/v1/stripe/webhook-events` | protected (admin/billing) | List webhook events |
| GET | `/api/v1/stripe/reconciliation` | protected (admin/billing) | Run reconciliation report |

## Billing Service Additions

### New type: WebhookEventRecord
```
id, org_id, provider, event_id, event_type, payload (JSON),
processed, processed_at, error_message, created_at
```

### New service methods
- `RecordWebhookEvent(provider, eventID, eventType string, payload []byte) (isNew bool, err error)`
- `MarkWebhookProcessed(providerEventKey string, processErr error)`
- `HandleStripePaymentIntentSucceeded(orgID, paymentIntentID, invoiceID string, amountCents int, actorID string) (*Payment, *LedgerEvent, error)`
- `HandleStripePaymentIntentFailed(paymentIntentID string) error`
- `HandleStripeChargeRefunded(orgID, paymentIntentID, invoiceID string, amountCents int, stripeRefundID, reason, actorID string) (*Refund, *LedgerEvent, error)`
- `ListWebhookEvents(orgID string, principal *Principal) ([]*WebhookEventRecord, error)`
- `GetReconciliationReport(orgID string, principal *Principal, stripeTotalCents, alertThresholdCents int) (*ReconciliationReport, error)`

## Stripe Config Env Vars
- `STRIPE_SECRET_KEY` — Stripe API secret key (sk_live_... or sk_test_...)
- `STRIPE_WEBHOOK_SECRET` — Endpoint webhook signing secret (whsec_...)
- `STRIPE_CURRENCY` — default currency, e.g. "usd"
- `STRIPE_RECONCILIATION_ALERT_THRESHOLD_CENTS` — mismatch threshold for alert (default: 100)

## Webhook Event Flow

```
POST /api/v1/webhooks/stripe
  1. Read raw body (must read before any other body processing)
  2. Verify Stripe-Signature header via HMAC-SHA256
  3. Parse event ID + type from payload
  4. Check for duplicate (billing.Service.RecordWebhookEvent)
     → if duplicate: return 200 OK {"status":"already_processed"}
  5. Persist webhook event (processed=false)
  6. Dispatch by event.Type:
     - payment_intent.succeeded → HandleStripePaymentIntentSucceeded
     - payment_intent.payment_failed → HandleStripePaymentIntentFailed
     - charge.refunded → HandleStripeChargeRefunded
     - (others) → no-op
  7. Mark webhook event processed (or failed)
  8. Return 200 OK {"status":"processed"}
```

## Testing Strategy
- `TestStripeWebhookDuplicateReplay` — second identical event returns 200 but is a no-op
- `TestStripeWebhookPaymentIntentSucceeded` — records payment + ledger event in billing
- `TestStripeWebhookPaymentIntentFailed` — marks payment failed
- `TestStripeWebhookChargeRefunded` — processes refund in billing
- `TestStripeWebhookInvalidSignature` — real client rejects bad sig; stub skips
- `TestCreatePaymentIntentSuccess` — endpoint creates PI and returns client_secret
- `TestCreatePaymentIntentForbiddenForParent` — role check
- `TestStripeReconciliationAdminOnly` — role check + returns report
- `TestStripeWebhookEventsAdminOnly` — role check + list

## Progress Tracking
- [x] Planning doc created
- [ ] stripe/client.go
- [ ] billing service additions
- [ ] handlers/stripe.go
- [ ] config.go
- [ ] server.go
- [ ] server_stripe_test.go
- [ ] TODO.md updated
