/**
 * k6 Load Test — Parent Portal Concurrent Sessions
 *
 * Simulates 20 concurrent parent users browsing the portal: loading the
 * dashboard, browsing meets and invoices, and viewing the calendar.
 *
 * Usage:
 *   k6 run -e API_BASE_URL=http://localhost:8080 \
 *           -e PARENT_TOKEN=stub-parent \
 *           -e ORG_ID=org-demo \
 *           tests/load/k6/portal-load.js
 */

import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// ---------------------------------------------------------------------------
// Custom metrics
// ---------------------------------------------------------------------------
const errorRate = new Rate('errors');
const dashboardLatency = new Trend('portal_dashboard_latency', true);
const meetsLatency = new Trend('portal_meets_latency', true);
const invoicesLatency = new Trend('portal_invoices_latency', true);
const calendarLatency = new Trend('portal_calendar_latency', true);

// ---------------------------------------------------------------------------
// Test options
// ---------------------------------------------------------------------------
export const options = {
  vus: 20,
  duration: '5m',

  thresholds: {
    // Portal pages may be slightly slower than raw API — p95 under 800ms
    http_req_duration: ['p(95)<800'],
    errors: ['rate<0.01'],
    portal_dashboard_latency: ['p(95)<800'],
    portal_meets_latency: ['p(95)<800'],
    portal_invoices_latency: ['p(95)<800'],
    portal_calendar_latency: ['p(95)<1000'],
  },
};

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------
const BASE_URL = __ENV.API_BASE_URL || 'http://localhost:8080';
const ORG_ID = __ENV.ORG_ID || 'org-demo';
const PARENT_TOKEN = __ENV.PARENT_TOKEN || 'stub-parent';

function portalHeaders() {
  return {
    Authorization: `Bearer ${PARENT_TOKEN}`,
    'X-Org-Id': ORG_ID,
    'Content-Type': 'application/json',
  };
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------
export function setup() {
  const res = http.get(`${BASE_URL}/health/live`);
  if (res.status !== 200) {
    throw new Error(`Health check failed: ${res.status}`);
  }
  return {};
}

// ---------------------------------------------------------------------------
// Default function — parent portal browsing session
// ---------------------------------------------------------------------------
export default function () {
  const headers = portalHeaders();

  // Portal dashboard: fetches session + household + swimmers
  group('portal_dashboard', () => {
    const sessionRes = http.get(
      `${BASE_URL}/api/v1/auth/session`,
      { headers, tags: { endpoint: 'portal_session' } }
    );
    check(sessionRes, { 'session 200': (r) => r.status === 200 });

    const swimmersRes = http.get(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/members/swimmers`,
      { headers, tags: { endpoint: 'portal_dashboard_swimmers' } }
    );
    const ok = check(swimmersRes, { 'dashboard swimmers 200': (r) => r.status === 200 });
    errorRate.add(!ok);
    dashboardLatency.add(swimmersRes.timings.duration);
  });

  sleep(1);

  // Portal meets list
  group('portal_meets', () => {
    const res = http.get(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/meets?limit=20`,
      { headers, tags: { endpoint: 'portal_meets' } }
    );
    const ok = check(res, {
      'meets list 200': (r) => r.status === 200,
    });
    errorRate.add(!ok);
    meetsLatency.add(res.timings.duration);
  });

  sleep(1);

  // Portal invoices
  group('portal_invoices', () => {
    const res = http.get(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/billing/invoices?limit=10`,
      { headers, tags: { endpoint: 'portal_invoices' } }
    );
    const ok = check(res, {
      'invoices 200': (r) => r.status === 200,
    });
    errorRate.add(!ok);
    invoicesLatency.add(res.timings.duration);
  });

  sleep(1);

  // Calendar — fetches meets and practices together
  group('portal_calendar', () => {
    const meetsRes = http.get(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/meets?limit=50`,
      { headers, tags: { endpoint: 'portal_calendar_meets' } }
    );
    check(meetsRes, { 'calendar meets 200': (r) => r.status === 200 });

    const practicesRes = http.get(
      `${BASE_URL}/api/v1/practices?limit=50`,
      { headers, tags: { endpoint: 'portal_calendar_practices' } }
    );
    const ok = check(practicesRes, {
      'calendar practices 200 or 403': (r) => r.status === 200 || r.status === 403,
    });
    errorRate.add(!ok);
    calendarLatency.add(Math.max(meetsRes.timings.duration, practicesRes.timings.duration));
  });

  sleep(2);
}
