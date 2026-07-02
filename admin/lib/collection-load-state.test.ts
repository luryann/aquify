import test from 'node:test'
import assert from 'node:assert/strict'

import { shouldShowCollectionContent, shouldShowCollectionEmptyState } from './collection-load-state.ts'

test('collection empty state is suppressed when a load error exists', () => {
  assert.equal(shouldShowCollectionEmptyState(false, 'Failed to load data.', 0), false)
  assert.equal(shouldShowCollectionContent(false, 'Failed to load data.', 3), false)
})

test('collection helpers only show content or empty state when loading has completed cleanly', () => {
  assert.equal(shouldShowCollectionEmptyState(false, null, 0), true)
  assert.equal(shouldShowCollectionContent(false, null, 2), true)
  assert.equal(shouldShowCollectionEmptyState(true, null, 0), false)
})
