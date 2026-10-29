import test from 'node:test'
import assert from 'node:assert/strict'

import { requestLogout, safeRedirectTarget } from './logout-flow.ts'

const origin = 'https://app.example.com'

test('safeRedirectTarget allows relative paths and rejects protocol-relative ones', () => {
  assert.deepEqual(safeRedirectTarget('/login', { origin }), { target: '/login', topLevel: false })
  assert.deepEqual(safeRedirectTarget('//evil.example', { origin }), { target: '/login', topLevel: false })
  assert.deepEqual(safeRedirectTarget('', { origin }), { target: '/login', topLevel: false })
})

test('safeRedirectTarget rejects non-http schemes and insecure remote hosts', () => {
  assert.equal(safeRedirectTarget('javascript:alert(1)', { origin }).target, '/login')
  assert.equal(safeRedirectTarget('http://idp.example.com/logout', { origin }).target, '/login')
  assert.equal(safeRedirectTarget('http://localhost:8081/logout', { origin }).topLevel, true)
})

test('safeRedirectTarget honours same-origin and the IdP allowlist', () => {
  assert.equal(safeRedirectTarget('https://app.example.com/x', { origin, allowedOrigins: 'https://idp.example.com' }).topLevel, true)
  assert.equal(safeRedirectTarget('https://idp.example.com/logout?x=1', { origin, allowedOrigins: 'https://idp.example.com' }).topLevel, true)
  assert.equal(safeRedirectTarget('https://evil.example.com/', { origin, allowedOrigins: 'https://idp.example.com' }).target, '/login')
})

test('requestLogout sends the CSRF header and prefers the IdP logout url', async () => {
  let seen: RequestInit | undefined
  const fetchImpl = (async (_url: string, init?: RequestInit) => {
    seen = init
    return new Response(JSON.stringify({ logout_url: 'https://idp.example.com/logout', redirect_url: '/login' }), { status: 200 })
  }) as unknown as typeof fetch
  const result = await requestLogout({ fetchImpl, csrfToken: 'tok', origin })
  assert.deepEqual(result, { target: 'https://idp.example.com/logout', topLevel: true })
  assert.equal((seen?.headers as Record<string, string>)['X-CSRF-Token'], 'tok')
})

test('requestLogout surfaces the server message on failure', async () => {
  const fetchImpl = (async () => new Response(JSON.stringify({ message: 'CSRF token missing.' }), { status: 403 })) as unknown as typeof fetch
  await assert.rejects(() => requestLogout({ fetchImpl, origin }), /CSRF token missing/)
})

test('requestLogout falls back to /login when the server sends an unsafe url', async () => {
  const fetchImpl = (async () => new Response(JSON.stringify({ logout_url: 'javascript:alert(1)' }), { status: 200 })) as unknown as typeof fetch
  assert.deepEqual(await requestLogout({ fetchImpl, origin }), { target: '/login', topLevel: false })
})
