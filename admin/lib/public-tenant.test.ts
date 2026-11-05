import test from 'node:test'
import assert from 'node:assert/strict'

import { normalizeClubSlug, resolveClubSlug, type TenantFetch } from './public-tenant.ts'

const ok = (body: unknown): TenantFetch => async () => ({ ok: true, json: async () => body })

test('the ?club= link wins and is normalised', async () => {
  const never: TenantFetch = async () => { throw new Error('must not call the server') }
  assert.equal(await resolveClubSlug({ searchParams: new URLSearchParams('club=Aqua-Club'), host: 'x.example.org', fetchImpl: never }), 'aqua-club')
})

test('invalid slugs are rejected rather than forwarded', () => {
  for (const bad of ['', 'a', 'has space', 'UP_PER', '../etc', 'x'.repeat(41)]) assert.equal(normalizeClubSlug(bad), '', bad)
})

test('a verified custom domain resolves through the server', async () => {
  let asked = ''
  const f: TenantFetch = async (url) => { asked = url; return { ok: true, json: async () => ({ slug: 'bluefish', name: 'Bluefish' }) } }
  assert.equal(await resolveClubSlug({ host: 'Swim.Bluefish.org', fetchImpl: f }), 'bluefish')
  assert.equal(asked, '/api/v1/public/tenant?host=swim.bluefish.org')
})

test('unknown hosts, localhost, IPs, errors and junk responses resolve to nothing', async () => {
  assert.equal(await resolveClubSlug({ host: 'unknown.example.org', fetchImpl: async () => ({ ok: false, json: async () => ({}) }) }), '')
  assert.equal(await resolveClubSlug({ host: 'localhost', fetchImpl: ok({ slug: 'x-club' }) }), '')
  assert.equal(await resolveClubSlug({ host: '127.0.0.1', fetchImpl: ok({ slug: 'x-club' }) }), '')
  assert.equal(await resolveClubSlug({ host: 'a.example.org', fetchImpl: async () => { throw new Error('offline') } }), '')
  assert.equal(await resolveClubSlug({ host: 'a.example.org', fetchImpl: ok({ slug: '../../etc' }) }), '')
  assert.equal(await resolveClubSlug({ host: 'a.example.org', fetchImpl: ok({ slug: 42 }) }), '')
})
