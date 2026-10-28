import test from 'node:test'
import assert from 'node:assert/strict'
import { INSTALL_DISMISS_KEY, isInstallDismissed, rememberInstallDismissed, shouldOfferInstall } from './install-prompt'

function mem(initial: Record<string, string> = {}) {
  const m = { ...initial }
  return { getItem: (k: string) => m[k] ?? null, setItem: (k: string, v: string) => { m[k] = v } }
}

test('not dismissed by default or without storage', () => {
  assert.equal(isInstallDismissed(mem()), false)
  assert.equal(isInstallDismissed(null), false)
})

test('dismissal snoozes for 90 days then expires', () => {
  const s = mem()
  rememberInstallDismissed(s, 1_000)
  assert.equal(isInstallDismissed(s, 1_000 + 89 * 86_400_000), true)
  assert.equal(isInstallDismissed(s, 1_000 + 91 * 86_400_000), false)
})

test('install is offered only from the third distinct visit day', () => {
  const s = mem()
  const day = 86_400_000
  assert.equal(shouldOfferInstall(s, 0), false)
  assert.equal(shouldOfferInstall(s, 1_000), false, 'same day does not count twice')
  assert.equal(shouldOfferInstall(s, day), false)
  assert.equal(shouldOfferInstall(s, 2 * day), true)
  rememberInstallDismissed(s, 2 * day)
  assert.equal(shouldOfferInstall(s, 3 * day), false)
})

test('garbage values and throwing storage are tolerated', () => {
  assert.equal(isInstallDismissed(mem({ [INSTALL_DISMISS_KEY]: 'nope' })), false)
  const bad = { getItem: () => { throw new Error('x') }, setItem: () => { throw new Error('x') } }
  assert.equal(isInstallDismissed(bad), false)
  assert.doesNotThrow(() => rememberInstallDismissed(bad))
})
