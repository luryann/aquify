import test from 'node:test'
import assert from 'node:assert/strict'

import { staffLabel } from './useStaffNames.ts'

test('staffLabel only treats the exact id "system" as the system actor', () => {
  const names = { u1: 'Coach Kim' }
  assert.equal(staffLabel(names, 'u1'), 'Coach Kim')
  assert.equal(staffLabel(names, 'system'), 'System')
  assert.equal(staffLabel(names, null), 'System')
  assert.equal(staffLabel(names, 'system-admin-42'), 'A staff member')
  assert.equal(staffLabel({ 'system-admin-42': 'Sam Systems' }, 'system-admin-42'), 'Sam Systems')
})
