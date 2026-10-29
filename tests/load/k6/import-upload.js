/**
 * k6 Load Test — Meet entry file parse
 *
 * Simulates 10 concurrent users uploading an SD3 file to the meet-creation
 * parser (POST /api/v1/meets/parse-entry-file, multipart field "file").
 *
 * Usage:
 *   k6 run -e API_BASE_URL=http://localhost:8080 \
 *           -e AUTH_TOKEN=stub-admin \
 *           -e ORG_ID=00000000-0000-0000-0000-00000000a001 \
 *           tests/load/k6/import-upload.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// ---------------------------------------------------------------------------
// Custom metrics
// ---------------------------------------------------------------------------
const errorRate = new Rate('errors');
const parseLatency = new Trend('parse_latency', true);

export const options = {
  vus: 10,
  duration: '3m',
  thresholds: {
    errors: ['rate<0.02'],
    parse_latency: ['p(95)<2000'],
  },
};

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------
const BASE_URL = __ENV.API_BASE_URL || 'http://localhost:8080';
const ORG_ID = __ENV.ORG_ID || '00000000-0000-0000-0000-00000000a001';
const AUTH_TOKEN = __ENV.AUTH_TOKEN || 'stub-admin';

function apiHeaders() {
  return {
    Authorization: `Bearer ${AUTH_TOKEN}`,
    'X-Org-Id': ORG_ID,
  };
}

/**
 * Generate a minimal valid SD3/SDIF file body (~1 KB) for upload testing.
 * This is a synthetic payload; actual validation is tested in unit tests.
 */
function generateTestSD3() {
  const lines = [
    'A0SDIF3.0                    TEST CLUB                    ',
    'B1TEST MEET                                    20260315',
    'C1DEMO POOL                  50M LCM           ',
    'D0  1SMITH          JOHN        M19950101USAS  ',
    'E0  1100FR  016250                              ',
    'Z0                                               ',
  ];
  // Pad each line to 160 chars and join with CRLF for a valid SDIF file
  return lines.map((l) => l.padEnd(160, ' ')).join('\r\n');
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------
export function setup() {
  const res = http.get(`${BASE_URL}/api/v1/health/live`);
  if (res.status !== 200) {
    throw new Error(`Health check failed: ${res.status}`);
  }
  return { testFile: generateTestSD3() };
}

// ---------------------------------------------------------------------------
// Default function — one parse request per VU iteration
// ---------------------------------------------------------------------------
export default function (data) {
  const res = http.post(
    `${BASE_URL}/api/v1/meets/parse-entry-file`,
    { file: http.file(data.testFile, `load-test-vu${__VU}-iter${__ITER}.sd3`, 'application/octet-stream') },
    { headers: apiHeaders(), tags: { endpoint: 'parse_entry_file' } }
  );

  const ok = check(res, {
    'parse 200': (r) => r.status === 200,
  });
  errorRate.add(!ok);
  parseLatency.add(res.timings.duration);

  sleep(1);
}
