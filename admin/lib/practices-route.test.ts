import test from 'node:test'
import assert from 'node:assert/strict'

import { legacyPracticesRedirectTarget } from './practices-route.ts'

test('legacy /practices route redirects to the live training page', () => {
  assert.equal(legacyPracticesRedirectTarget(), '/training')
})
