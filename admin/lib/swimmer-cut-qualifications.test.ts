import test from 'node:test'
import assert from 'node:assert/strict'

import {
  filterApplicableTimeStandards,
  sortCutQualificationCards,
  swimmerAgeOnDate,
} from './swimmer-cut-qualifications.ts'

const swimmer = {
  id: 'sw_1',
  org_id: 'org_1',
  household_id: 'hh_1',
  first_name: 'Ava',
  last_name: 'Lane',
  date_of_birth: '2012-05-20',
  gender: 'female',
  registration_status: 'active',
}

test('swimmerAgeOnDate calculates age from DOB against a reference date', () => {
  assert.equal(swimmerAgeOnDate(swimmer, new Date('2026-05-17T00:00:00Z')), 13)
})

test('filterApplicableTimeStandards keeps only active standards matching swimmer gender and age band', () => {
  const standards = [
    { id: 'a', org_id: 'org_1', name: 'A', season_year: 2026, standard_code: 'A', event_distance: 100, stroke: 'free', course: 'SCY', gender: 'F', time_ms: 60000, is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
    { id: 'b', org_id: 'org_1', name: 'B', season_year: 2026, standard_code: 'AA', event_distance: 100, stroke: 'free', course: 'SCY', gender: 'M', time_ms: 58000, is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
    { id: 'c', org_id: 'org_1', name: 'C', season_year: 2026, standard_code: 'BB', event_distance: 200, stroke: 'free', course: 'SCY', gender: 'F', age_min: 14, time_ms: 140000, is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
    { id: 'd', org_id: 'org_1', name: 'D', season_year: 2026, standard_code: 'B', event_distance: 50, stroke: 'free', course: 'SCY', gender: 'X', time_ms: 32000, is_active: false, created_at: '', updated_at: '', created_by: '', updated_by: '' },
  ]

  const filtered = filterApplicableTimeStandards(standards, swimmer, new Date('2026-05-17T00:00:00Z'))
  assert.deepEqual(filtered.map((item) => item.id), ['a'])
})

test('sortCutQualificationCards prioritizes qualified cards then event order', () => {
  const cards = sortCutQualificationCards([
    {
      standard: { id: 'b', org_id: 'org_1', name: 'AA', season_year: 2026, standard_code: 'AA', event_distance: 200, stroke: 'free', course: 'LCM', gender: 'F', time_ms: 120000, is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
      qualification: { swimmer_id: 'sw_1', standard_id: 'b', standard_code: 'AA', event_distance: 200, stroke: 'free', course: 'LCM', gender: 'F', cut_time_ms: 120000, qualified: false },
    },
    {
      standard: { id: 'a', org_id: 'org_1', name: 'A', season_year: 2026, standard_code: 'A', event_distance: 100, stroke: 'free', course: 'SCY', gender: 'F', time_ms: 60000, is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
      qualification: { swimmer_id: 'sw_1', standard_id: 'a', standard_code: 'A', event_distance: 100, stroke: 'free', course: 'SCY', gender: 'F', cut_time_ms: 60000, qualified: true },
    },
  ])

  assert.deepEqual(cards.map((item) => item.standard.id), ['a', 'b'])
})
