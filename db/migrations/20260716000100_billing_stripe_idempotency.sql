-- +goose Up
-- Dedicated idempotency key for Stripe-sourced credit movements, replacing the
-- free-text "(Stripe pi_...)" reason suffix. Overpayment credits use the
-- PaymentIntent id; refund reversals use "refund|<pi>|<refund id>".
ALTER TABLE household_credit_entries
  ADD COLUMN stripe_ref TEXT NOT NULL DEFAULT '';

UPDATE household_credit_entries
   SET stripe_ref = substring(reason from '\(Stripe ([^)]+)\)$')
 WHERE kind = 'overpayment' AND reason ~ '\(Stripe [^)]+\)$';

CREATE UNIQUE INDEX household_credit_entries_stripe_ref_uidx
  ON household_credit_entries (org_id, stripe_ref) WHERE stripe_ref <> '';

-- Webhook claims: a lease stops concurrent deliveries of one event from being
-- processed twice; retryable marks transient failures Stripe should redeliver.
ALTER TABLE webhook_events
  ADD COLUMN retryable BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN processing_until TIMESTAMPTZ;

-- +goose Down
ALTER TABLE webhook_events DROP COLUMN processing_until, DROP COLUMN retryable;
DROP INDEX IF EXISTS household_credit_entries_stripe_ref_uidx;
ALTER TABLE household_credit_entries DROP COLUMN stripe_ref;
