/**
 * k6 Load Test — Admin Browsing
 *
 * Simulates 50 concurrent admin/coach users browsing core list pages.
 *
 * Usage:
 *   k6 run -e API_BASE_URL=http://localhost:8080 \
 *           -e AUTH_TOKEN=stub-admin-token \
 *           -e ORG_ID=org-demo \
 *           tests/load/k6/admin-browse.js
 */

import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// ---------------------------------------------------------------------------
// Custom metrics
// ---------------------------------------------------------------------------
const errorRate = new Rate('errors');
const householdsLatency = new Trend('households_latency', true);
const swimmersLatency = new Trend('swimmers_latency', true);
const meetsLatency = new Trend('meets_latency', true);
const invoicesLatency = new Trend('invoices_latency', true);
const auditLatency = new Trend('audit_latency', true);

// ---------------------------------------------------------------------------
// Test options
// ---------------------------------------------------------------------------
export const options = {
  vus: 50,
  duration: '5m',

  thresholds: {
    // Overall p95 must be under 500 ms
    http_req_duration: ['p(95)<500'],
    // Error rate under 1%
    errors: ['rate<0.01'],
    // Per-endpoint latency budgets
    households_latency: ['p(95)<500'],
    swimmers_latency: ['p(95)<500'],
    meets_latency: ['p(95)<500'],
    invoices_latency: ['p(95)<500'],
    audit_latency: ['p(95)<800'],
  },
};

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------
const BASE_URL = __ENV.API_BASE_URL || 'http://localhost:8080';
const ORG_ID = __ENV.ORG_ID || 'org-demo';
const DEFAULT_TOKEN = __ENV.AUTH_TOKEN || 'stub-admin-token';

function authHeaders(token) {
  return {
    Authorization: `Bearer ${token}`,
    'X-Org-Id': ORG_ID,
    'Content-Type': 'application/json',
  };
}

// ---------------------------------------------------------------------------
// Setup — verify connectivity and obtain token
// ---------------------------------------------------------------------------
export function setup() {
  const res = http.get(`${BASE_URL}/health/live`);
  if (res.status !== 200) {
    throw new Error(`Health check failed: ${res.status} — is the API running at ${BASE_URL}?`);
  }

  // In a real environment, exchange credentials for a token here.
  // In dev/staging, the stub token is sufficient.
  return { token: DEFAULT_TOKEN };
}

// ---------------------------------------------------------------------------
// Default function — VU workload
// ---------------------------------------------------------------------------
export default function (data) {
  const headers = authHeaders(data.token);

  group('list_households', () => {
    const res = http.get(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/members/households?limit=25`,
      { headers, tags: { endpoint: 'households' } }
    );
    const ok = check(res, {
      'households 200': (r) => r.status === 200,
    });
    errorRate.add(!ok);
    householdsLatency.add(res.timings.duration);
  });

  sleep(1);

  group('list_swimmers', () => {
    const res = http.get(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/members/swimmers?limit=25`,
      { headers, tags: { endpoint: 'swimmers' } }
    );
    const ok = check(res, {
      'swimmers 200': (r) => r.status === 200,
    });
    errorRate.add(!ok);
    swimmersLatency.add(res.timings.duration);
  });

  sleep(1);

  group('list_meets', () => {
    const res = http.get(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/meets?limit=25`,
      { headers, tags: { endpoint: 'meets' } }
    );
    const ok = check(res, {
      'meets 200': (r) => r.status === 200,
    });
    errorRate.add(!ok);
    meetsLatency.add(res.timings.duration);
  });

  sleep(1);

  group('list_invoices', () => {
    const res = http.get(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/billing/invoices?limit=25`,
      { headers, tags: { endpoint: 'invoices' } }
    );
    const ok = check(res, {
      'invoices 200': (r) => r.status === 200,
    });
    errorRate.add(!ok);
    invoicesLatency.add(res.timings.duration);
  });

  sleep(1);

  group('list_audit_events', () => {
    const res = http.get(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/audit?limit=25`,
      { headers, tags: { endpoint: 'audit' } }
    );
    const ok = check(res, {
      'audit 200': (r) => r.status === 200,
    });
    errorRate.add(!ok);
    auditLatency.add(res.timings.duration);
  });

  sleep(1);
}

// ---------------------------------------------------------------------------
// Teardown — log summary
// ---------------------------------------------------------------------------
export function teardown(data) {
  console.log(`Load test complete. Token used: ${data.token.substring(0, 12)}...`);
}
