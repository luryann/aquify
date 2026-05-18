/**
 * k6 Load Test — Import File Upload Flow
 *
 * Simulates 10 concurrent users uploading SD3 meet result files through the
 * full import pipeline: create job → get presigned upload URL → PUT file →
 * finalize import.
 *
 * Usage:
 *   k6 run -e API_BASE_URL=http://localhost:8080 \
 *           -e AUTH_TOKEN=stub-admin-token \
 *           -e ORG_ID=org-demo \
 *           tests/load/k6/import-upload.js
 */

import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// ---------------------------------------------------------------------------
// Custom metrics
// ---------------------------------------------------------------------------
const errorRate = new Rate('errors');
const createJobLatency = new Trend('create_job_latency', true);
const uploadLatency = new Trend('upload_latency', true);
const finalizeLatency = new Trend('finalize_latency', true);
const e2eLatency = new Trend('import_e2e_latency', true);

// ---------------------------------------------------------------------------
// Test options
// ---------------------------------------------------------------------------
export const options = {
  vus: 10,
  duration: '3m',

  thresholds: {
    // End-to-end p95 under 3 seconds
    import_e2e_latency: ['p(95)<3000'],
    // Error rate under 2%
    errors: ['rate<0.02'],
    // Individual step thresholds
    create_job_latency: ['p(95)<500'],
    upload_latency: ['p(95)<2000'],
    finalize_latency: ['p(95)<800'],
  },
};

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------
const BASE_URL = __ENV.API_BASE_URL || 'http://localhost:8080';
const ORG_ID = __ENV.ORG_ID || 'org-demo';
const AUTH_TOKEN = __ENV.AUTH_TOKEN || 'stub-admin-token';

function apiHeaders() {
  return {
    Authorization: `Bearer ${AUTH_TOKEN}`,
    'X-Org-Id': ORG_ID,
    'Content-Type': 'application/json',
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
  const res = http.get(`${BASE_URL}/health/live`);
  if (res.status !== 200) {
    throw new Error(`Health check failed: ${res.status}`);
  }
  return { testFile: generateTestSD3() };
}

// ---------------------------------------------------------------------------
// Default function — full import pipeline per VU iteration
// ---------------------------------------------------------------------------
export default function (data) {
  const vuId = __VU;
  const iterationId = __ITER;
  const iterStart = Date.now();

  let importId = null;
  let uploadUrl = null;
  let allOk = true;

  // Step 1: Create import job
  group('create_import_job', () => {
    const payload = JSON.stringify({
      filename: `load-test-vu${vuId}-iter${iterationId}.sd3`,
      content_type: 'application/octet-stream',
      file_size: data.testFile.length,
      source_type: 'sd3',
    });

    const res = http.post(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/imports`,
      payload,
      { headers: apiHeaders(), tags: { endpoint: 'create_import' } }
    );

    const ok = check(res, {
      'create job 201': (r) => r.status === 201,
      'has import id': (r) => {
        try {
          const body = JSON.parse(r.body);
          return !!body.id;
        } catch {
          return false;
        }
      },
      'has upload url': (r) => {
        try {
          const body = JSON.parse(r.body);
          return !!body.upload_url;
        } catch {
          return false;
        }
      },
    });

    errorRate.add(!ok);
    createJobLatency.add(res.timings.duration);

    if (ok) {
      try {
        const body = JSON.parse(res.body);
        importId = body.id;
        uploadUrl = body.upload_url;
      } catch {
        allOk = false;
      }
    } else {
      allOk = false;
    }
  });

  if (!allOk || !importId || !uploadUrl) {
    errorRate.add(1);
    return;
  }

  sleep(0.5);

  // Step 2: Upload file to presigned URL
  group('upload_file', () => {
    const uploadRes = http.put(uploadUrl, data.testFile, {
      headers: {
        'Content-Type': 'application/octet-stream',
        'Content-Length': String(data.testFile.length),
      },
      tags: { endpoint: 'upload_file' },
    });

    // Presigned S3/MinIO PUTs return 200 or 204
    const ok = check(uploadRes, {
      'upload 200 or 204': (r) => r.status === 200 || r.status === 204,
    });

    errorRate.add(!ok);
    uploadLatency.add(uploadRes.timings.duration);
    if (!ok) allOk = false;
  });

  if (!allOk) {
    errorRate.add(1);
    return;
  }

  sleep(0.5);

  // Step 3: Finalize import (triggers parsing)
  group('finalize_import', () => {
    const finalizeRes = http.post(
      `${BASE_URL}/api/v1/orgs/${ORG_ID}/imports/${importId}/finalize`,
      null,
      { headers: apiHeaders(), tags: { endpoint: 'finalize_import' } }
    );

    const ok = check(finalizeRes, {
      'finalize 200 or 202': (r) => r.status === 200 || r.status === 202,
    });

    errorRate.add(!ok);
    finalizeLatency.add(finalizeRes.timings.duration);
  });

  // Record end-to-end elapsed time
  e2eLatency.add(Date.now() - iterStart);

  sleep(2);
}
