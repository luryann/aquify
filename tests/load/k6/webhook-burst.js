/**
 * k6 Load Test — Stripe Webhook Burst
 *
 * Simulates a burst of 20 concurrent VUs posting Stripe webhook events to
 * validate ingestion throughput and low-latency processing under load.
 *
 * NOTE: This test uses a synthetic Stripe-Signature header. In staging, set
 * WEBHOOK_SECRET to the actual Stripe test webhook signing secret so that
 * HMAC verification passes. In development with stub validation disabled,
 * the test secret below is sufficient.
 *
 * Usage:
 *   k6 run -e API_BASE_URL=http://localhost:8080 \
 *           -e WEBHOOK_SECRET=whsec_test_secret \
 *           tests/load/k6/webhook-burst.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import crypto from 'k6/crypto';

// ---------------------------------------------------------------------------
// Custom metrics
// ---------------------------------------------------------------------------
const errorRate = new Rate('errors');
const webhookLatency = new Trend('webhook_latency', true);

// ---------------------------------------------------------------------------
// Test options
// ---------------------------------------------------------------------------
export const options = {
  vus: 20,
  duration: '1m',

  thresholds: {
    // p99 must be under 200ms — webhooks must be acknowledged quickly
    webhook_latency: ['p(99)<200'],
    http_req_duration: ['p(99)<200'],
    // Near-zero error rate required for payment reliability
    errors: ['rate<0.001'],
  },
};

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------
const BASE_URL = __ENV.API_BASE_URL || 'http://localhost:8080';
const WEBHOOK_SECRET = __ENV.WEBHOOK_SECRET || 'whsec_test_secret';

/**
 * Build a minimal Stripe payment_intent.payment_failed webhook payload. The
 * intent is unknown to the ledger, so the API verifies the signature, records
 * the event and acknowledges it with 200 without touching invoices — which is
 * what this ingestion-throughput test measures.
 */
function buildWebhookPayload(vuId, iterationId) {
  const eventId = `evt_load_${vuId}_${iterationId}_${Date.now()}`;
  const paymentIntentId = `pi_load_${vuId}_${iterationId}`;

  return JSON.stringify({
    id: eventId,
    object: 'event',
    api_version: '2023-10-16',
    created: Math.floor(Date.now() / 1000),
    type: 'payment_intent.payment_failed',
    livemode: false,
    data: {
      object: {
        id: paymentIntentId,
        object: 'payment_intent',
        amount: 15000,
        currency: 'usd',
        status: 'succeeded',
        metadata: {
          org_id: '00000000-0000-0000-0000-00000000a001',
          invoice_id: `inv-load-${vuId}`,
        },
      },
    },
  });
}

/**
 * Compute a Stripe-compatible webhook signature.
 *
 * Stripe signature format:
 *   t=<unix_timestamp>,v1=<HMAC-SHA256(secret, "t.payload")>
 *
 * k6's crypto.hmac returns a hex-encoded digest when encoding='hex'.
 */
function buildStripeSignature(payload, secret) {
  const timestamp = Math.floor(Date.now() / 1000);
  const signedPayload = `${timestamp}.${payload}`;
  // Stripe uses HMAC-SHA256 with the raw secret (after stripping the "whsec_" prefix)
  const rawSecret = secret.startsWith('whsec_') ? secret.slice(6) : secret;
  const signature = crypto.hmac('sha256', rawSecret, signedPayload, 'hex');
  return `t=${timestamp},v1=${signature}`;
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------
export function setup() {
  const res = http.get(`${BASE_URL}/api/v1/health/live`);
  if (res.status !== 200) {
    throw new Error(`Health check failed: ${res.status}`);
  }
  return {};
}

// ---------------------------------------------------------------------------
// Default function
// ---------------------------------------------------------------------------
export default function () {
  const payload = buildWebhookPayload(__VU, __ITER);
  const signature = buildStripeSignature(payload, WEBHOOK_SECRET);

  const res = http.post(
    `${BASE_URL}/api/v1/webhooks/stripe`,
    payload,
    {
      headers: {
        'Content-Type': 'application/json',
        'Stripe-Signature': signature,
      },
      tags: { endpoint: 'stripe_webhook' },
    }
  );

  const ok = check(res, {
    // Stripe webhooks should be acknowledged with 200
    'webhook 200': (r) => r.status === 200,
    // Response must be fast — Stripe retries if response takes > 30s
    'response time < 200ms': (r) => r.timings.duration < 200,
  });

  errorRate.add(!ok);
  webhookLatency.add(res.timings.duration);

  // No sleep — burst test, maximize throughput
}
