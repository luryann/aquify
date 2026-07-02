import test from 'node:test'
import assert from 'node:assert/strict'

import { canAccess, filterNavItems, hasRole } from './roles.ts'

test('multi-role access checks use the full backend role set', () => {
  assert.equal(canAccess(['parent', 'billing'], ['billing']), true)
  assert.equal(canAccess(['parent', 'swimmer'], ['owner', 'admin']), false)
  assert.equal(hasRole(['swimmer', 'coach'], 'parent'), true)
})

test('navigation filtering preserves items for secondary privileged roles', () => {
  const items = filterNavItems(['parent', 'billing'])
  assert.equal(items.some((item) => item.path === '/billing'), true)
  assert.equal(items.some((item) => item.path === '/portal/dashboard'), false)
})
