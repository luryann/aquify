import test from 'node:test'
import assert from 'node:assert/strict'

import { canAccess, filterNavItems, hasRole } from './roles.ts'

test('multi-role access checks use the full backend role set', () => {
  assert.equal(canAccess(['parent', 'admin'], ['admin']), true)
  assert.equal(canAccess(['parent', 'swimmer'], ['admin']), false)
  assert.equal(hasRole(['swimmer', 'coach'], 'parent'), true)
})

test('navigation filtering preserves items for secondary privileged roles', () => {
  const items = filterNavItems(['parent', 'admin'])
  const billingItem = items.find((item) => item.path === '/billing')
  assert.ok(billingItem)
  assert.equal(items.some((item) => item.path === '/portal/dashboard'), false)
})

test('navigation filtering hides items the role cannot access', () => {
  const items = filterNavItems(['coach'])
  assert.equal(items.some((item) => item.path === '/billing'), false)
  assert.equal(items.some((item) => item.path === '/settings'), false)
  assert.equal(items.some((item) => item.path === '/meets'), true)
  assert.equal(items.every((item) => item.allowedRoles.includes('coach')), true)
})
