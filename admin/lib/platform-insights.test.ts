import test from 'node:test'
import assert from 'node:assert/strict'

import { attentionItems, healthChecks, mergeActivity, statusCounts } from './platform-insights.ts'
import type { PlatformOrg } from './tenancyData.ts'

const NOW = Date.parse('2026-11-13T12:00:00Z')
const org = (over: Partial<PlatformOrg>): PlatformOrg => ({ id: 'o', name: 'Club', status: 'active', created_at: '2026-01-01T00:00:00Z', ...over })

test('attention lists only clubs that need an operator, most urgent first', () => {
  const orgs = [
    org({ id: 'a', name: 'Active' }),
    org({ id: 'p', name: 'Platform' }),
    org({ id: 's', name: 'Paused', status: 'suspended' }),
    org({ id: 'c', name: 'Closing', status: 'offboarding', offboarding_at: '2026-11-10T12:00:00Z' }),
    org({ id: 'd', name: 'Done waiting', status: 'offboarding', offboarding_at: '2026-10-01T12:00:00Z' }),
    org({ id: 'x', name: 'Stuck', status: 'provisioning', created_at: '2026-11-13T11:00:00Z' }),
    org({ id: 'n', name: 'Fresh', status: 'provisioning', created_at: '2026-11-13T11:55:00Z' }),
  ]
  const items = attentionItems(orgs, 'p', NOW)
  assert.deepEqual(items.map((i) => i.org.name), ['Done waiting', 'Stuck', 'Closing', 'Paused'])
  assert.match(items[2].detail, /^11 days left/)
})

test('status counts skip the platform org', () => {
  const c = statusCounts([org({ id: 'p' }), org({ id: 'a' }), org({ id: 'b', status: 'suspended' })], 'p')
  assert.equal(c.active, 1)
  assert.equal(c.suspended, 1)
})

test('activity from every club merges newest first and carries the club name', () => {
  const ev = (id: string, at: string) => ({ id, action: 'platform.org.update', actor_user_id: 'u', occurred_at: at, resource_id: 'r' })
  const merged = mergeActivity([
    { org: { id: '1', name: 'One' }, events: [ev('a', '2026-11-01T00:00:00Z'), ev('c', '2026-11-03T00:00:00Z')] },
    { org: { id: '2', name: 'Two' }, events: [ev('b', '2026-11-02T00:00:00Z')] },
  ])
  assert.deepEqual(merged.map((m) => `${m.id}:${m.orgName}`), ['c:One', 'b:Two', 'a:One'])
})

test('health checks name failures first and explain disabled services', () => {
  const rows = healthChecks({ database: 'ready', redis: 'disabled', storage: 'connection refused', stripe: 'ready (stub)' })
  assert.deepEqual(rows.map((r) => [r.name, r.state]), [['File storage', 'down'], ['Job queue (Redis)', 'off'], ['Database', 'ok'], ['Payments (Stripe)', 'ok']])
  assert.equal(rows[1].detail, 'Not turned on')
  assert.equal(rows[3].detail, 'stub')
})
