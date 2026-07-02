import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildSwimmerCutsViewModel,
  describeCutQualification,
} from './swimmer-performance-cuts.ts'

const baseCard = {
  standard: {
    id: 'std_1',
    org_id: 'org_1',
    name: 'A Standard',
    season_year: 2026,
    standard_code: 'A',
    event_distance: 100,
    stroke: 'free',
    course: 'SCY',
    gender: 'F',
    time_ms: 60000,
    is_active: true,
    created_at: '',
    updated_at: '',
    created_by: '',
    updated_by: '',
  },
  qualification: {
    swimmer_id: 'sw_1',
    standard_id: 'std_1',
    standard_code: 'A',
    event_distance: 100,
    stroke: 'free',
    course: 'SCY',
    gender: 'F',
    cut_time_ms: 60000,
    qualified: false,
  },
}

test('buildSwimmerCutsViewModel returns loading state while qualifications are loading', () => {
  const model = buildSwimmerCutsViewModel({ loading: true, cards: [] })
  assert.equal(model.state, 'loading')
  assert.match(model.message, /Fetching active time standards/i)
})

test('buildSwimmerCutsViewModel returns error state for API failures', () => {
  const model = buildSwimmerCutsViewModel({ loading: false, error: 'Request failed.', cards: [] })
  assert.equal(model.state, 'error')
  assert.equal(model.message, 'Request failed.')
})

test('buildSwimmerCutsViewModel returns empty state when no applicable standards exist', () => {
  const model = buildSwimmerCutsViewModel({ loading: false, cards: [] })
  assert.equal(model.state, 'empty')
  assert.match(model.message, /no active standards matching your age/i)
})

test('buildSwimmerCutsViewModel summarizes ready state counts', () => {
  const model = buildSwimmerCutsViewModel({
    loading: false,
    cards: [
      {
        ...baseCard,
        qualification: { ...baseCard.qualification, qualified: true, best_time_ms: 59800, margin_ms: 200 },
      },
      {
        ...baseCard,
        standard: { ...baseCard.standard, id: 'std_2' },
        qualification: { ...baseCard.qualification, standard_id: 'std_2', best_time_ms: 61234, margin_ms: -1234 },
      },
    ],
  })

  assert.equal(model.state, 'ready')
  assert.equal(model.qualifiedCount, 1)
  assert.equal(model.totalCount, 2)
})

test('describeCutQualification reports qualified swimmers with positive margin', () => {
  const status = describeCutQualification({
    ...baseCard,
    qualification: { ...baseCard.qualification, qualified: true, best_time_ms: 59800, margin_ms: 200 },
  })

  assert.equal(status.badge, 'Qualified')
  assert.equal(status.tone, 'success')
  assert.match(status.detail, /Under the cut by 0\.20s\./)
})

test('describeCutQualification reports non-qualified swimmers with remaining margin', () => {
  const status = describeCutQualification({
    ...baseCard,
    qualification: { ...baseCard.qualification, best_time_ms: 61234, margin_ms: -1234 },
  })

  assert.equal(status.badge, 'Not Yet Qualified')
  assert.equal(status.tone, 'warning')
  assert.match(status.detail, /1\.23s away from the cut\./)
})

test('describeCutQualification reports no-data state when no best time exists', () => {
  const status = describeCutQualification(baseCard)
  assert.equal(status.badge, 'No Time Yet')
  assert.equal(status.tone, 'muted')
  assert.match(status.detail, /No recorded best time/i)
})
