import test from 'node:test'
import assert from 'node:assert/strict'

import type { ApiClient } from './api.ts'
import { ApiError } from './api.ts'
import {
  acceptRegistrationWaiver,
  archiveAnnouncement,
  createAnnouncement,
  createCampaign,
  createDataDeletionRequest,
  createDataExportRequest,
  createMeetEntry,
  createSuppression,
  createTimeStandard,
  deleteDLQTask,
  deleteTimeStandard,
  downloadReportArtifact,
  fetchAnnouncement,
  fetchAnnouncements,
  fetchAuditEvents,
  fetchCampaign,
  fetchCampaignDeliveries,
  fetchCampaigns,
  fetchDLQTasks,
  fetchDataDeletionRequests,
  fetchDataExportRequests,
  fetchInvoiceAgingReport,
  fetchHouseholdAutopay,
  fetchHouseholdAutopayStatuses,
  fetchInvoices,
  fetchJobMetrics,
  fetchJobs,
  fetchMembershipStatusReport,
  fetchPaymentReconciliationReport,
  fetchPayments,
  fetchPerformanceBestTimes,
  fetchPerformanceProgression,
  fetchQueueStats,
  fetchRegistrationStatusReport,
  fetchRuntimeSettings,
  fetchSwimmerActivity,
  fetchSuppression,
  fetchSwimmers,
  fetchSwimmerBestTimes,
  fetchSwimmerProgression,
  fetchTimeStandards,
  fetchUsers,
  publishAnnouncement,
  removeSuppression,
  respondToMeetCommitment,
  retryDLQTask,
  sendCampaign,
  optOutHouseholdAutopay,
  upsertHouseholdAutopay,
  updateTimeStandard,
} from './adminData.ts'

type RecordedCall = {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE'
  path: string
  body?: unknown
  headers?: Record<string, string>
}

function createMockApi() {
  const calls: RecordedCall[] = []
  const responses = new Map<string, unknown>()

  function key(method: RecordedCall['method'], path: string) {
    return `${method} ${path}`
  }

  const api = {
    get: async <T>(path: string, _signal?: AbortSignal, headers?: Record<string, string>) => {
      calls.push({ method: 'GET', path, headers })
      return responses.get(key('GET', path)) as T
    },
    post: async <T>(path: string, body?: unknown, headers?: Record<string, string>) => {
      calls.push({ method: 'POST', path, body, headers })
      return responses.get(key('POST', path)) as T
    },
    put: async <T>(path: string, body?: unknown, headers?: Record<string, string>) => {
      calls.push({ method: 'PUT', path, body, headers })
      return responses.get(key('PUT', path)) as T
    },
    delete: async <T>(path: string, headers?: Record<string, string>) => {
      calls.push({ method: 'DELETE', path, headers })
      return responses.get(key('DELETE', path)) as T
    },
    calls,
    respond(method: RecordedCall['method'], path: string, value: unknown) {
      responses.set(key(method, path), value)
    },
  }

  return api
}

test('Step 29E reports helpers hit live report endpoints with org-scoped headers', async () => {
  const mock = createMockApi()
  mock.respond('GET', '/api/v1/reports/membership-status', { report: { generated_at: '2026-03-20T00:00:00Z', row_count: 1, rows: [] } })
  mock.respond('GET', '/api/v1/reports/invoice-aging?as_of=2026-03-20', { report: { generated_at: '2026-03-20T00:00:00Z', as_of_date: '2026-03-20', row_count: 0, outstanding_count: 0, total_outstanding_cents: 0, rows: [] } })
  mock.respond('GET', '/api/v1/reports/payment-reconciliation?stripe_reported_cents=4200', { report: { generated_at: '2026-03-20T00:00:00Z', row_count: 0, payment_total_cents: 0, refund_total_cents: 0, net_collected_cents: 0, unreconciled_row_count: 0, stripe_reported_cents: 4200, rows: [] } })
  mock.respond('GET', '/api/v1/reports/registration-status', { report: { generated_at: '2026-03-20T00:00:00Z', row_count: 0, rows: [] } })
  mock.respond('GET', '/api/v1/reports/membership-status/latest-artifact', {})

  const api = mock as unknown as ApiClient
  const orgId = 'org_live'

  assert.equal((await fetchMembershipStatusReport(api, orgId)).row_count, 1)
  assert.equal((await fetchInvoiceAgingReport(api, orgId, '2026-03-20')).as_of_date, '2026-03-20')
  assert.equal((await fetchPaymentReconciliationReport(api, orgId, 4200)).stripe_reported_cents, 4200)
  assert.equal((await fetchRegistrationStatusReport(api, orgId)).row_count, 0)
  assert.equal(await downloadReportArtifact(api, orgId, '/api/v1/reports/membership-status/latest-artifact'), null)

  assert.deepEqual(mock.calls, [
    { method: 'GET', path: '/api/v1/reports/membership-status', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/reports/invoice-aging?as_of=2026-03-20', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/reports/payment-reconciliation?stripe_reported_cents=4200', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/reports/registration-status', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/reports/membership-status/latest-artifact', headers: { 'X-Org-Id': orgId } },
  ])
})

test('Step 29E users helper preserves source metadata for stubbed development user lists', async () => {
  const mock = createMockApi()
  const orgId = 'org_users'
  mock.respond('GET', '/api/v1/users?role=coach&limit=25', {
    status: 'ok',
    count: 1,
    source: 'stub',
    is_stubbed: true,
    users: [
      {
        id: 'user_coach',
        username: 'coach@example.com',
        email: 'coach@example.com',
        firstName: 'Coach',
        lastName: 'Person',
        enabled: true,
        emailVerified: true,
        roles: ['coach'],
      },
    ],
  })

  const api = mock as unknown as ApiClient
  const result = await fetchUsers(api, orgId, { role: 'coach', limit: 25 })

  assert.equal(result.count, 1)
  assert.equal(result.source, 'stub')
  assert.equal(result.isStubbed, true)
  assert.equal(result.users[0]?.roles?.[0], 'coach')
  assert.deepEqual(mock.calls, [
    { method: 'GET', path: '/api/v1/users?role=coach&limit=25', headers: { 'X-Org-Id': orgId } },
  ])
})

test('Step 29E audit and jobs helpers cover the live operational admin endpoints', async () => {
  const mock = createMockApi()
  mock.respond('GET', '/api/v1/audit-events?action=announcement.publish', { events: [{ id: 'evt_1', action: 'announcement.publish' }], next_cursor: 'next_1' })
  mock.respond('GET', '/api/v1/jobs', { tasks: [{ id: 'job_1', type: 'billing_reconcile' }] })
  mock.respond('GET', '/api/v1/jobs/queues', { queues: [{ queue: 'default', pending: 1, active: 0, retrying: 0, completed: 3, failed: 0, dead_letter: 0 }] })
  mock.respond('GET', '/api/v1/jobs/metrics', { by_type: { billing_reconcile: { enqueued: 1, succeeded: 1, failed: 0, dlq: 0 } }, recent_executions: [] })
  mock.respond('GET', '/api/v1/jobs/dlq', { dead_letter_tasks: [{ id: 'dlq_1', type: 'send_email' }] })
  mock.respond('POST', '/api/v1/jobs/dlq/dlq_1/retry', undefined)
  mock.respond('DELETE', '/api/v1/jobs/dlq/dlq_1', undefined)

  const api = mock as unknown as ApiClient

  assert.equal((await fetchAuditEvents(api, 'org_ops', { action: 'announcement.publish' })).next_cursor, 'next_1')
  assert.equal((await fetchJobs(api, 'org_ops'))[0]?.id, 'job_1')
  assert.equal((await fetchQueueStats(api, 'org_ops'))[0]?.queue, 'default')
  assert.equal((await fetchJobMetrics(api, 'org_ops')).by_type.billing_reconcile?.succeeded, 1)
  assert.equal((await fetchDLQTasks(api, 'org_ops'))[0]?.id, 'dlq_1')
  await retryDLQTask(api, 'org_ops', 'dlq_1')
  await deleteDLQTask(api, 'org_ops', 'dlq_1')

  assert.deepEqual(mock.calls, [
    { method: 'GET', path: '/api/v1/audit-events?action=announcement.publish', headers: { 'X-Org-Id': 'org_ops' } },
    { method: 'GET', path: '/api/v1/jobs', headers: { 'X-Org-Id': 'org_ops' } },
    { method: 'GET', path: '/api/v1/jobs/queues', headers: { 'X-Org-Id': 'org_ops' } },
    { method: 'GET', path: '/api/v1/jobs/metrics', headers: { 'X-Org-Id': 'org_ops' } },
    { method: 'GET', path: '/api/v1/jobs/dlq', headers: { 'X-Org-Id': 'org_ops' } },
    { method: 'POST', path: '/api/v1/jobs/dlq/dlq_1/retry', body: undefined, headers: { 'X-Org-Id': 'org_ops' } },
    { method: 'DELETE', path: '/api/v1/jobs/dlq/dlq_1', headers: { 'X-Org-Id': 'org_ops' } },
  ])
})

test('billing autopay helpers use the live household autopay endpoints', async () => {
  const mock = createMockApi()
  const orgId = 'org_billing'

  mock.respond('GET', '/api/v1/households/hh_1/autopay', { autopay: { household_id: 'hh_1', enabled: true, trigger_days_before_due: 3 } })
  mock.respond('GET', '/api/v1/autopay/households?household_id=hh_1&household_id=hh_2', {
    autopay: [
      { household_id: 'hh_1', enabled: true, trigger_days_before_due: 3 },
      { household_id: 'hh_2', enabled: false, trigger_days_before_due: 0 },
    ],
  })
  mock.respond('PUT', '/api/v1/households/hh_1/autopay', { autopay: { household_id: 'hh_1', enabled: true, trigger_days_before_due: 7 } })
  mock.respond('POST', '/api/v1/households/hh_1/autopay/opt-out', { autopay: { household_id: 'hh_1', enabled: false, trigger_days_before_due: 7 } })

  const api = mock as unknown as ApiClient

  assert.equal((await fetchHouseholdAutopay(api, orgId, 'hh_1')).enabled, true)
  assert.equal((await fetchHouseholdAutopayStatuses(api, orgId, ['hh_1', 'hh_2'])).length, 2)
  assert.equal((await upsertHouseholdAutopay(api, orgId, 'hh_1', {
    payment_method_type: 'card',
    payment_method_label: 'Visa •••• 4242',
    trigger_days_before_due: 7,
  })).trigger_days_before_due, 7)
  assert.equal((await optOutHouseholdAutopay(api, orgId, 'hh_1')).enabled, false)

  assert.deepEqual(mock.calls, [
    { method: 'GET', path: '/api/v1/households/hh_1/autopay', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/autopay/households?household_id=hh_1&household_id=hh_2', headers: { 'X-Org-Id': orgId } },
    {
      method: 'PUT',
      path: '/api/v1/households/hh_1/autopay',
      body: {
        payment_method_type: 'card',
        payment_method_label: 'Visa •••• 4242',
        trigger_days_before_due: 7,
      },
      headers: { 'X-Org-Id': orgId },
    },
    { method: 'POST', path: '/api/v1/households/hh_1/autopay/opt-out', body: undefined, headers: { 'X-Org-Id': orgId } },
  ])
})

test('portal registration helpers use live registration and waiver routes', async () => {
  const mock = createMockApi()
  const orgId = 'org_portal_reg'

  mock.respond('POST', '/api/v1/registrations/reg_1/waivers', {
    waiver: {
      id: 'wvr_1',
      org_id: orgId,
      registration_id: 'reg_1',
      waiver_type: 'code of conduct agreement',
      accepted_at: '2026-05-25T18:00:00Z',
      accepted_by_user_id: 'usr_parent',
      created_at: '2026-05-25T18:00:00Z',
    },
  })

  const api = mock as unknown as ApiClient
  const waiver = await acceptRegistrationWaiver(api, orgId, 'reg_1', {
    waiver_type: 'code of conduct agreement',
  })

  assert.equal(waiver.registration_id, 'reg_1')
  assert.equal(waiver.waiver_type, 'code of conduct agreement')
  assert.deepEqual(mock.calls, [
    {
      method: 'POST',
      path: '/api/v1/registrations/reg_1/waivers',
      body: { waiver_type: 'code of conduct agreement' },
      headers: { 'X-Org-Id': orgId },
    },
  ])
})

test('Step 29E comms helpers preserve live list, detail, and publish/archive flows', async () => {
  const mock = createMockApi()
  mock.respond('GET', '/api/v1/announcements?status=draft', { announcements: [{ id: 'ann_1', title: 'Spring Update' }] })
  mock.respond('GET', '/api/v1/announcements/ann_1', { id: 'ann_1', title: 'Spring Update', status: 'draft' })
  mock.respond('POST', '/api/v1/announcements', { id: 'ann_2', title: 'New Draft', status: 'draft' })
  mock.respond('POST', '/api/v1/announcements/ann_1/publish', { id: 'ann_1', status: 'published' })
  mock.respond('POST', '/api/v1/announcements/ann_1/archive', { id: 'ann_1', status: 'archived' })
  mock.respond('GET', '/api/v1/email-campaigns', { campaigns: [{ id: 'cmp_1', name: 'Spring Campaign' }] })
  mock.respond('GET', '/api/v1/email-campaigns/cmp_1', { id: 'cmp_1', subject: 'Go team' })
  mock.respond('POST', '/api/v1/email-campaigns', { id: 'cmp_2', subject: 'Summer Invite' })
  mock.respond('POST', '/api/v1/email-campaigns/cmp_1/send', { id: 'cmp_1', status: 'sent', sent_count: 2 })
  mock.respond('GET', '/api/v1/email-campaigns/cmp_1/deliveries', { deliveries: [{ id: 'del_1', recipient_email: 'parent@example.com' }] })

  const api = mock as unknown as ApiClient
  const orgId = 'org_comms'

  assert.equal((await fetchAnnouncements(api, orgId, 'draft'))[0]?.id, 'ann_1')
  assert.equal((await fetchAnnouncement(api, orgId, 'ann_1')).id, 'ann_1')
  assert.equal((await createAnnouncement(api, orgId, { title: 'New Draft', body: 'Body', audience: 'all' })).id, 'ann_2')
  assert.equal((await publishAnnouncement(api, orgId, 'ann_1')).status, 'published')
  assert.equal((await archiveAnnouncement(api, orgId, 'ann_1')).status, 'archived')
  assert.equal((await fetchCampaigns(api, orgId))[0]?.id, 'cmp_1')
  assert.equal((await fetchCampaign(api, orgId, 'cmp_1')).id, 'cmp_1')
  assert.equal((await createCampaign(api, orgId, {
    name: 'Summer Invite',
    subject: 'Summer Invite',
    body_html: '<p>Hello</p>',
    from_name: 'Aquify',
    from_email: 'team@example.com',
    audience: 'all',
  })).id, 'cmp_2')
  assert.equal((await sendCampaign(api, orgId, 'cmp_1')).status, 'sent')
  assert.equal((await fetchCampaignDeliveries(api, orgId, 'cmp_1'))[0]?.id, 'del_1')

  assert.deepEqual(mock.calls, [
    { method: 'GET', path: '/api/v1/announcements?status=draft', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/announcements/ann_1', headers: { 'X-Org-Id': orgId } },
    { method: 'POST', path: '/api/v1/announcements', body: { title: 'New Draft', body: 'Body', audience: 'all' }, headers: { 'X-Org-Id': orgId } },
    { method: 'POST', path: '/api/v1/announcements/ann_1/publish', body: undefined, headers: { 'X-Org-Id': orgId } },
    { method: 'POST', path: '/api/v1/announcements/ann_1/archive', body: undefined, headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/email-campaigns', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/email-campaigns/cmp_1', headers: { 'X-Org-Id': orgId } },
    {
      method: 'POST',
      path: '/api/v1/email-campaigns',
      body: {
        name: 'Summer Invite',
        subject: 'Summer Invite',
        body_html: '<p>Hello</p>',
        from_name: 'Aquify',
        from_email: 'team@example.com',
        audience: 'all',
      },
      headers: { 'X-Org-Id': orgId },
    },
    {
      method: 'POST',
      path: '/api/v1/email-campaigns/cmp_1/send',
      body: undefined,
      headers: { 'X-Org-Id': orgId },
    },
    { method: 'GET', path: '/api/v1/email-campaigns/cmp_1/deliveries', headers: { 'X-Org-Id': orgId } },
  ])
})

test('Step 29E compliance and suppression helpers keep admin mutations on live endpoints', async () => {
  const mock = createMockApi()
  mock.respond('GET', '/api/v1/suppression', { suppression: [{ id: 'sup_1', email: 'optout@example.com' }] })
  mock.respond('POST', '/api/v1/suppression', { id: 'sup_2', email: 'new@example.com', reason: 'bounce' })
  mock.respond('DELETE', '/api/v1/suppression/sup_1', undefined)
  mock.respond('GET', '/api/v1/compliance/data-export-requests', { requests: [{ id: 'exp_1', household_id: 'hh_1' }] })
  mock.respond('POST', '/api/v1/compliance/data-export-requests', { id: 'exp_2', household_id: 'hh_2' })
  mock.respond('GET', '/api/v1/compliance/data-deletion-requests', { requests: [{ id: 'del_1', household_id: 'hh_1' }] })
  mock.respond('POST', '/api/v1/compliance/data-deletion-requests', { id: 'del_2', household_id: 'hh_2', reason: 'requested by household' })

  const api = mock as unknown as ApiClient
  const orgId = 'org_compliance'

  assert.equal((await fetchSuppression(api, orgId))[0]?.id, 'sup_1')
  assert.equal((await createSuppression(api, orgId, { email: 'new@example.com', reason: 'bounce' })).id, 'sup_2')
  await removeSuppression(api, orgId, 'sup_1')
  assert.equal((await fetchDataExportRequests(api, orgId))[0]?.id, 'exp_1')
  assert.equal((await createDataExportRequest(api, orgId, 'hh_2')).id, 'exp_2')
  assert.equal((await fetchDataDeletionRequests(api, orgId))[0]?.id, 'del_1')
  assert.equal((await createDataDeletionRequest(api, orgId, { household_id: 'hh_2', reason: 'requested by household' })).id, 'del_2')

  assert.deepEqual(mock.calls, [
    { method: 'GET', path: '/api/v1/suppression', headers: { 'X-Org-Id': orgId } },
    { method: 'POST', path: '/api/v1/suppression', body: { email: 'new@example.com', reason: 'bounce' }, headers: { 'X-Org-Id': orgId } },
    { method: 'DELETE', path: '/api/v1/suppression/sup_1', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/compliance/data-export-requests', headers: { 'X-Org-Id': orgId } },
    { method: 'POST', path: '/api/v1/compliance/data-export-requests', body: { household_id: 'hh_2' }, headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/compliance/data-deletion-requests', headers: { 'X-Org-Id': orgId } },
    {
      method: 'POST',
      path: '/api/v1/compliance/data-deletion-requests',
      body: { household_id: 'hh_2', reason: 'requested by household' },
      headers: { 'X-Org-Id': orgId },
    },
  ])
})

test('Step 29E performance and settings helpers keep live admin screens attached to backend state', async () => {
  const mock = createMockApi()
  mock.respond('GET', '/api/v1/settings/runtime', { settings: { app_env: 'development' } })
  mock.respond('GET', '/api/v1/performance/best-times', { best_times: [{ id: 'bt_org_1', swimmer_id: 'sw_1' }] })
  mock.respond('GET', '/api/v1/performance/progression', { results: [{ id: 'pr_org_1', swimmer_id: 'sw_1' }] })
  mock.respond('GET', '/api/v1/swimmers/sw_1/best-times', { best_times: [{ id: 'bt_1', swimmer_id: 'sw_1' }] })
  mock.respond('GET', '/api/v1/swimmers/sw_1/progression', { results: [{ id: 'pr_1', swimmer_id: 'sw_1' }] })
  mock.respond('GET', '/api/v1/swimmers/sw_1/activity', { activities: [{ id: 'act_1', type: 'result', occurred_at: '2026-05-19T00:00:00Z', title: '100 SCY Freestyle' }] })
  mock.respond('GET', '/api/v1/time-standards?course=SCY&season_year=2026', { time_standards: [{ id: 'ts_1', standard_code: 'A' }] })
  mock.respond('POST', '/api/v1/time-standards', { time_standard: { id: 'ts_2', standard_code: 'B' } })
  mock.respond('PUT', '/api/v1/time-standards/ts_2', { time_standard: { id: 'ts_2', is_active: false } })
  mock.respond('DELETE', '/api/v1/time-standards/ts_2', undefined)

  const api = mock as unknown as ApiClient
  const orgId = 'org_perf'

  assert.equal((await fetchRuntimeSettings(api, orgId)).app_env, 'development')
  assert.equal((await fetchPerformanceBestTimes(api, orgId))[0]?.id, 'bt_org_1')
  assert.equal((await fetchPerformanceProgression(api, orgId))[0]?.id, 'pr_org_1')
  assert.equal((await fetchSwimmerBestTimes(api, orgId, 'sw_1'))[0]?.id, 'bt_1')
  assert.equal((await fetchSwimmerProgression(api, orgId, 'sw_1'))[0]?.id, 'pr_1')
  assert.equal((await fetchSwimmerActivity(api, orgId, 'sw_1'))[0]?.id, 'act_1')
  assert.equal((await fetchTimeStandards(api, orgId, { course: 'SCY', season_year: '2026' }))[0]?.id, 'ts_1')
  assert.equal((await createTimeStandard(api, orgId, {
    name: '2026 A Standard',
    season_year: 2026,
    standard_code: 'B',
    event_distance: 100,
    stroke: 'free',
    course: 'SCY',
    gender: 'F',
    time_ms: 65000,
    is_active: true,
  })).id, 'ts_2')
  assert.equal((await updateTimeStandard(api, orgId, 'ts_2', { is_active: false })).id, 'ts_2')
  await deleteTimeStandard(api, orgId, 'ts_2')

  assert.deepEqual(mock.calls, [
    { method: 'GET', path: '/api/v1/settings/runtime', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/performance/best-times', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/performance/progression', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/swimmers/sw_1/best-times', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/swimmers/sw_1/progression', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/swimmers/sw_1/activity', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/time-standards?course=SCY&season_year=2026', headers: { 'X-Org-Id': orgId } },
    {
      method: 'POST',
      path: '/api/v1/time-standards',
      body: {
        name: '2026 A Standard',
        season_year: 2026,
        standard_code: 'B',
        event_distance: 100,
        stroke: 'free',
        course: 'SCY',
        gender: 'F',
        time_ms: 65000,
        is_active: true,
      },
      headers: { 'X-Org-Id': orgId },
    },
    { method: 'PUT', path: '/api/v1/time-standards/ts_2', body: { is_active: false }, headers: { 'X-Org-Id': orgId } },
    { method: 'DELETE', path: '/api/v1/time-standards/ts_2', headers: { 'X-Org-Id': orgId } },
  ])
})

test('Step 29E billing helpers support org-wide invoice and payment reads without household fan-out', async () => {
  const mock = createMockApi()
  mock.respond('GET', '/api/v1/invoices', { invoices: [{ id: 'inv_1', invoice_number: 'INV-1' }] })
  mock.respond('GET', '/api/v1/payments', { payments: [{ id: 'pay_1', invoice_id: 'inv_1' }] })

  const api = mock as unknown as ApiClient
  const orgId = 'org_billing'

  assert.equal((await fetchInvoices(api, orgId))[0]?.id, 'inv_1')
  assert.equal((await fetchPayments(api, orgId))[0]?.id, 'pay_1')

  assert.deepEqual(mock.calls, [
    { method: 'GET', path: '/api/v1/invoices', headers: { 'X-Org-Id': orgId } },
    { method: 'GET', path: '/api/v1/payments', headers: { 'X-Org-Id': orgId } },
  ])
})

test('meet mutation helpers target the live /api/v1/meets route shape and backend request bodies', async () => {
  const mock = createMockApi()
  mock.respond('POST', '/api/v1/meets/meet_1/entries', {
    entry: { id: 'entry_1', meet_id: 'meet_1', swimmer_id: 'sw_1', meet_event_id: 'event_1', status: 'draft' },
  })
  mock.respond('POST', '/api/v1/meets/meet_1/commitments/cmt_1/respond', {
    commitment: { id: 'cmt_1', meet_id: 'meet_1', swimmer_id: 'sw_1', status: 'committed_yes', note: 'Can do prelims only.' },
  })

  const api = mock as unknown as ApiClient
  const orgId = 'org_meets'

  assert.equal((await createMeetEntry(api, orgId, 'meet_1', {
    swimmer_id: 'sw_1',
    meet_event_id: 'event_1',
  })).id, 'entry_1')
  assert.equal((await respondToMeetCommitment(api, orgId, 'meet_1', 'cmt_1', {
    response: 'committed_yes',
    notes: 'Can do prelims only.',
  })).id, 'cmt_1')

  assert.deepEqual(mock.calls, [
    {
      method: 'POST',
      path: '/api/v1/meets/meet_1/entries',
      body: { swimmer_id: 'sw_1', meet_event_id: 'event_1' },
      headers: { 'X-Org-Id': orgId },
    },
    {
      method: 'POST',
      path: '/api/v1/meets/meet_1/commitments/cmt_1/respond',
      body: { response: 'committed_yes', notes: 'Can do prelims only.' },
      headers: { 'X-Org-Id': orgId },
    },
  ])
})

test('fetchSwimmers returns empty list on role-based forbidden response', async () => {
  const api = {
    get: async () => {
      throw new ApiError(403, 'forbidden', 'You are not allowed to list swimmers.')
    },
  } as unknown as ApiClient

  const swimmers = await fetchSwimmers(api, 'org_demo')
  assert.deepEqual(swimmers, [])
})

test('fetchSwimmers rethrows non-forbidden 403 responses', async () => {
  const api = {
    get: async () => {
      throw new ApiError(403, 'scope_violation', 'Principal org_id must match X-Org-Id scope.')
    },
  } as unknown as ApiClient

  await assert.rejects(
    fetchSwimmers(api, 'org_demo'),
    (error: unknown) =>
      error instanceof ApiError &&
      error.status === 403 &&
      error.code === 'scope_violation',
  )
})
