import assert from 'node:assert/strict'
import test from 'node:test'
import { proxyAPIRequest } from './api-proxy'

test('API proxy resolves runtime upstream, streams bodies and preserves auth cookies and redirects', async () => {
  const oldOrigin = process.env.API_INTERNAL_URL
  const oldFetch = globalThis.fetch
  try {
    process.env.API_INTERNAL_URL = 'http://api-one:8080'
    const calls: string[] = []
    globalThis.fetch = async (url, options) => {
      calls.push(String(url))
      assert.equal(new Headers(options?.headers).get('cookie'), 'aquify_access_token=session')
      assert.equal(new Headers(options?.headers).get('x-csrf-token'), 'csrf')
      assert.equal(options?.redirect, 'manual')
      assert.equal(await new Response(options?.body).text(), 'payload')
      return new Response(null, { status: 302, headers: [
        ['location', '/portal'],
        ['set-cookie', 'aquify_access_token=next; HttpOnly; Path=/'],
        ['set-cookie', 'aquify_csrf_token=next-csrf; Path=/'],
      ] })
    }
    const request = () => new Request('https://app.example/api/v1/auth/session?refresh=true', {
      method: 'POST', body: 'payload', headers: { cookie: 'aquify_access_token=session', 'x-csrf-token': 'csrf' },
    })
    const response = await proxyAPIRequest(request())
    assert.equal(response.status, 302)
    assert.equal(response.headers.get('location'), '/portal')
    assert.equal(response.headers.getSetCookie().length, 2)
    process.env.API_INTERNAL_URL = 'http://api-two:9090'
    await proxyAPIRequest(request())
    assert.deepEqual(calls, ['http://api-one:8080/api/v1/auth/session?refresh=true', 'http://api-two:9090/api/v1/auth/session?refresh=true'])
  } finally {
    globalThis.fetch = oldFetch
    if (oldOrigin === undefined) delete process.env.API_INTERNAL_URL
    else process.env.API_INTERNAL_URL = oldOrigin
  }
})

test('API proxy returns a gateway error when the upstream is unavailable', async () => {
  const oldFetch = globalThis.fetch
  globalThis.fetch = async () => { throw new Error('connection refused') }
  try {
    const response = await proxyAPIRequest(new Request('https://app.example/api/v1/meets'))
    assert.equal(response.status, 502)
  } finally { globalThis.fetch = oldFetch }
})
