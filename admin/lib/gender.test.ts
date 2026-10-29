import test from 'node:test'
import assert from 'node:assert/strict'

import { isSwimmerGenderEligibleForEvent, swimmerGenderToEventGender } from './gender.ts'

test('swimmerGenderToEventGender maps known values exactly', () => {
  assert.equal(swimmerGenderToEventGender('female'), 'F')
  assert.equal(swimmerGenderToEventGender(' F '), 'F')
  assert.equal(swimmerGenderToEventGender('Male'), 'M')
  assert.equal(swimmerGenderToEventGender('m'), 'M')
  assert.equal(swimmerGenderToEventGender('non_binary'), 'X')
  assert.equal(swimmerGenderToEventGender('undisclosed'), 'X')
})

test('values that merely start with M or F are not classified as male or female', () => {
  assert.equal(swimmerGenderToEventGender('multi'), 'X')
  assert.equal(swimmerGenderToEventGender('fluid'), 'X')
  assert.equal(isSwimmerGenderEligibleForEvent('multi', 'M'), false)
  assert.equal(isSwimmerGenderEligibleForEvent('female', 'F'), true)
})
