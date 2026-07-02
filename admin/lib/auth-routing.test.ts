import test from 'node:test'
import assert from 'node:assert/strict'

import { destinationForRole, roleRequiresMFA } from './auth-routing.ts'

test('roleRequiresMFA includes all privileged browser roles', () => {
  assert.equal(roleRequiresMFA('owner'), true)
  assert.equal(roleRequiresMFA('admin'), true)
  assert.equal(roleRequiresMFA('coach'), true)
  assert.equal(roleRequiresMFA('billing'), true)
  assert.equal(roleRequiresMFA(['parent', 'billing']), true)
})

test('roleRequiresMFA leaves parent and swimmer ungated', () => {
  assert.equal(roleRequiresMFA('parent'), false)
  assert.equal(roleRequiresMFA('swimmer'), false)
  assert.equal(roleRequiresMFA('read-only'), false)
})

test('destinationForRole keeps portal routing unchanged', () => {
  assert.equal(destinationForRole('parent'), '/portal/dashboard')
  assert.equal(destinationForRole('swimmer'), '/swim')
  assert.equal(destinationForRole('coach'), '/dashboard')
  assert.equal(destinationForRole(['billing', 'parent']), '/portal/dashboard')
})
