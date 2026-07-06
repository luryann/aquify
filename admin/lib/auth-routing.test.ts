import test from 'node:test'
import assert from 'node:assert/strict'

import { destinationForRole, roleRequiresMFA } from './auth-routing.ts'

test('roleRequiresMFA includes all privileged browser roles', () => {
  assert.equal(roleRequiresMFA('admin'), true)
  assert.equal(roleRequiresMFA('coach'), true)
  assert.equal(roleRequiresMFA(['parent', 'admin']), true)
})

test('roleRequiresMFA leaves parent and swimmer ungated', () => {
  assert.equal(roleRequiresMFA('parent'), false)
  assert.equal(roleRequiresMFA('swimmer'), false)
})

test('destinationForRole keeps portal routing unchanged', () => {
  assert.equal(destinationForRole('parent'), '/portal/dashboard')
  assert.equal(destinationForRole('swimmer'), '/swim')
  assert.equal(destinationForRole('coach'), '/dashboard')
  assert.equal(destinationForRole(['admin', 'parent']), '/portal/dashboard')
})
