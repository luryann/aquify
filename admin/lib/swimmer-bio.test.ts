import test from 'node:test'
import assert from 'node:assert/strict'

import {
  SWIMMER_BIO_MAX_LENGTH,
  normalizeSwimmerBio,
  validateSwimmerBio,
} from './swimmer-bio.ts'

test('normalizeSwimmerBio trims surrounding whitespace before save', () => {
  assert.equal(normalizeSwimmerBio('  relay lead-off specialist  '), 'relay lead-off specialist')
})

test('validateSwimmerBio accepts empty and max-length bios', () => {
  assert.equal(validateSwimmerBio(''), null)
  assert.equal(validateSwimmerBio('a'.repeat(SWIMMER_BIO_MAX_LENGTH)), null)
})

test('validateSwimmerBio rejects bios longer than the configured max length', () => {
  assert.equal(validateSwimmerBio('a'.repeat(SWIMMER_BIO_MAX_LENGTH + 1)), 'Bio must be 400 characters or fewer.')
})
