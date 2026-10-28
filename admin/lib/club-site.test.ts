import test from 'node:test'
import assert from 'node:assert/strict'
import { excerpt, formatMeetDates, groupByMonth, resolveRecordFilter, groupRecordsByAge, formatEventDay, eventName } from './club-site.ts'
import type { PublicRecord } from './publicSiteData.ts'

const rec = (course: string, gender: string, age: string, distance = 50, stroke = 'freestyle'): PublicRecord => ({
  label: '', course, gender, age_group: age, distance, stroke, time_ms: 25000, swimmer_name: 'A B', swim_date: '2025-01-01',
})

test('excerpt trims on a word boundary', () => {
  assert.equal(excerpt('short'), 'short')
  const out = excerpt('word '.repeat(80), 40)
  assert.ok(out.endsWith('…') && out.length <= 41)
})

test('meet dates collapse within a month', () => {
  assert.equal(formatMeetDates({ start_date: '2026-03-07', end_date: '2026-03-09' }), 'Mar 7–9, 2026')
  assert.equal(formatMeetDates({ start_date: '2026-03-07', end_date: '2026-03-07' }), 'Mar 7, 2026')
  assert.equal(formatMeetDates({ start_date: '2026-03-30', end_date: '2026-04-01' }), 'Mar 30 – Apr 1, 2026')
  assert.equal(formatMeetDates({}), 'Dates to be announced')
})

test('groupByMonth keeps order', () => {
  const g = groupByMonth([{ d: '2026-03-01' }, { d: '2026-03-20' }, { d: '2026-04-02' }], (x) => x.d)
  assert.deepEqual(g.map((x) => [x.key, x.items.length]), [['2026-03', 2], ['2026-04', 1]])
})

test('record filter falls back to available course and gender', () => {
  const records = [rec('SCM', 'M', '10U'), rec('SCM', 'M', '11-12')]
  assert.deepEqual(resolveRecordFilter(records, 'XXX', 'F'), { course: 'SCM', gender: 'M' })
  assert.deepEqual(resolveRecordFilter([], undefined, undefined), { course: 'SCY', gender: 'F' })
  const grouped = groupRecordsByAge(records, { course: 'SCM', gender: 'M' })
  assert.deepEqual(grouped.map((g) => g.ageGroup), ['10U', '11-12'])
})

test('all-day events keep their calendar day', () => {
  assert.equal(formatEventDay({ starts_at: '2026-05-02T00:00:00Z', all_day: true }, { day: 'numeric' }), '2')
})

test('event name capitalises stroke', () => {
  assert.equal(eventName({ distance: 100, stroke: 'butterfly' }), '100 Butterfly')
})
