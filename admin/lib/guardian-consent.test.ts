import test from 'node:test'
import assert from 'node:assert/strict'

import { guardianConsentState, isUnderConsentAge } from './guardian-consent.ts'

const now = new Date('2026-11-03T15:00:00Z')

test('under 13 until the 13th birthday', () => {
  assert.equal(isUnderConsentAge('2013-11-04', now), true)
  assert.equal(isUnderConsentAge('2013-11-03', now), false)
  assert.equal(isUnderConsentAge('2010-01-01', now), false)
})

test('unparseable or missing date of birth fails closed', () => {
  assert.equal(isUnderConsentAge('', now), true)
  assert.equal(isUnderConsentAge(undefined, now), true)
  assert.equal(isUnderConsentAge('not-a-date', now), true)
})

test('consent state reflects age and recorded consent', () => {
  assert.equal(guardianConsentState({ date_of_birth: '2016-05-05' }, now), 'needed')
  assert.equal(guardianConsentState({ date_of_birth: '2016-05-05', guardian_consent_at: '2026-10-01T00:00:00Z' }, now), 'given')
  assert.equal(guardianConsentState({ date_of_birth: '2008-05-05' }, now), 'not-needed')
})
