-- +goose Up
CREATE TABLE webhook_events (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id          UUID,  -- nullable; some webhooks arrive before org resolution

  provider        TEXT NOT NULL DEFAULT 'stripe',
  event_id        TEXT NOT NULL,   -- Stripe event ID — idempotency key
  event_type      TEXT NOT NULL,   -- e.g. payment_intent.succeeded

  payload         JSONB NOT NULL,

  processed       BOOLEAN NOT NULL DEFAULT FALSE,
  processed_at    TIMESTAMPTZ,
  error_message   TEXT,

  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT webhook_events_provider_event_unique UNIQUE (provider, event_id)
);

CREATE INDEX webhook_events_provider_idx    ON webhook_events (provider);
CREATE INDEX webhook_events_processed_idx   ON webhook_events (processed) WHERE NOT processed;
CREATE INDEX webhook_events_created_at_idx  ON webhook_events (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS webhook_events;
