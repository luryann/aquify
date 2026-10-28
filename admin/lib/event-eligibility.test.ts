import test from 'node:test'
import assert from 'node:assert/strict'

import { isSwimmerEligibleForEvent, swimmerAgeAsOf } from './event-eligibility.ts'

test('swimmerAgeAsOf uses the meet start date, not today', () => {
  assert.equal(swimmerAgeAsOf('2011-07-21', '2026-12-05'), 15)
  assert.equal(swimmerAgeAsOf('2011-12-06', '2026-12-05'), 14)
  assert.equal(swimmerAgeAsOf('2011-12-05', '2026-12-05'), 15)
  assert.equal(swimmerAgeAsOf('not-a-date', '2026-12-05'), null)
})

test('isSwimmerEligibleForEvent enforces gender and age bounds like the API', () => {
  const leo = { gender: 'male', date_of_birth: '2011-07-21' }
  const maya = { gender: 'female', date_of_birth: '2014-03-09' }
  const boys1314 = { gender: 'M', min_age: 13, max_age: 14 }
  const girls1112 = { gender: 'F', min_age: 11, max_age: 12 }
  const open = { gender: 'O' }

  assert.equal(isSwimmerEligibleForEvent(leo, boys1314, '2026-12-05'), false) // aged up to 15
  assert.equal(isSwimmerEligibleForEvent(leo, boys1314, '2025-12-05'), true)
  assert.equal(isSwimmerEligibleForEvent(maya, girls1112, '2026-12-05'), true)
  assert.equal(isSwimmerEligibleForEvent(maya, boys1314, '2026-12-05'), false)
  assert.equal(isSwimmerEligibleForEvent(leo, open, '2026-12-05'), true)
})
