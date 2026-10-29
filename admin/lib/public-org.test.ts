import test from 'node:test'
import assert from 'node:assert/strict'

import { LOCAL_DEV_ORG_ID } from './localDev.ts'
import { resolvePublicOrg } from './public-org.ts'

test('resolvePublicOrg prefers an explicit org_id query parameter', () => {
  const resolved = resolvePublicOrg({
    searchParams: new URLSearchParams('org_id=org-query&club_code=dare2026'),
    host: 'portal.example.com',
    clubCode: 'ignored',
  })

  assert.deepEqual(resolved, {
    orgId: 'org-query',
    clubCode: 'IGNORED',
    source: 'query_org_id',
  })
})

test('resolvePublicOrg falls back to local dev org on localhost', () => {
  const resolved = resolvePublicOrg({
    host: 'localhost',
    config: { defaultOrgId: 'org-demo' },
  })

  assert.equal(resolved.orgId, LOCAL_DEV_ORG_ID)
  assert.equal(resolved.source, 'local_dev')
})

test('resolvePublicOrg does not resolve the local dev org in a production build', () => {
  const resolved = resolvePublicOrg({
    host: 'localhost',
    config: { defaultOrgId: '', production: true },
  })

  assert.equal(resolved.orgId, '')
  assert.equal(resolved.source, 'none')
})

test('resolvePublicOrg resolves club code query parameters to tenant org ids', () => {
  const resolved = resolvePublicOrg({
    searchParams: new URLSearchParams('club_code=dare2026'),
    host: 'portal.example.com',
    config: {
      clubCodeOrgMap: {
        dare2026: 'org-dare',
      },
    },
  })

  assert.deepEqual(resolved, {
    orgId: 'org-dare',
    clubCode: 'DARE2026',
    source: 'query_club_code',
  })
})

test('resolvePublicOrg resolves production hosts to tenant org ids', () => {
  const resolved = resolvePublicOrg({
    host: 'bluefish.example.com',
    config: {
      hostOrgMap: {
        'bluefish.example.com': 'org-bluefish',
      },
    },
  })

  assert.deepEqual(resolved, {
    orgId: 'org-bluefish',
    clubCode: '',
    source: 'host',
  })
})

test('resolvePublicOrg ignores shared demo defaults on production hosts', () => {
  const resolved = resolvePublicOrg({
    host: 'portal.example.com',
    config: { defaultOrgId: 'org-demo' },
  })

  assert.equal(resolved.orgId, '')
  assert.equal(resolved.source, 'none')
})

test('resolvePublicOrg allows explicit non-demo defaults on production hosts', () => {
  const resolved = resolvePublicOrg({
    host: 'portal.example.com',
    config: { defaultOrgId: 'org-club-001' },
  })

  assert.equal(resolved.orgId, 'org-club-001')
  assert.equal(resolved.source, 'default')
})

test('resolvePublicOrg returns none when no club context is resolvable', () => {
  const resolved = resolvePublicOrg({ host: 'aquify.example.com' })

  assert.equal(resolved.orgId, '')
  assert.equal(resolved.source, 'none')
})
