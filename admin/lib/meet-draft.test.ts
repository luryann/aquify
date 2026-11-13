import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildCreateMeetPayload,
  dollarsToCents,
  getNextSessionNumber,
  isMeetEventFileName,
  mapParsedSessionsToDraftSessions,
  type MeetCreateDraftState,
} from './meet-draft.ts'
import type { ParsedMeetEventDraftRecord, ParsedMeetSessionDraft } from './adminData.ts'

test('mapParsedSessionsToDraftSessions preserves imported session numbers', () => {
  const parsedSessions: ParsedMeetSessionDraft[] = [
    { session_number: 2, section: 'Day 1', name: 'Morning', session_date: '2026-05-01' },
    { session_number: 4, section: 'Day 2', name: 'Finals', session_date: '2026-05-02' },
  ]

  assert.deepEqual(
    mapParsedSessionsToDraftSessions(parsedSessions).map((session) => session.sessionNumber),
    [2, 4],
  )
})

test('getNextSessionNumber uses the highest imported session number', () => {
  assert.equal(getNextSessionNumber([{ sessionNumber: 2 }, { sessionNumber: 4 }]), 5)
})

test('buildCreateMeetPayload preserves session numbering for sessions and events', () => {
  const draft: MeetCreateDraftState = {
    name: 'Spring Invite',
    course: 'SCY',
    hostClub: 'aquify',
    location: 'Bellevue',
    startDate: '2026-05-01',
    endDate: '2026-05-02',
    commitmentDeadline: '2026-04-20',
    entryDeadline: '2026-04-25',
    maxEntriesPerSwimmer: '3',
    swimmerSelfSignup: true,
    sessions: [
      { id: 's-1', sessionNumber: 2, section: 'Day 1', name: 'Morning', date: '2026-05-01', warmupTime: '07:00', startTime: '08:00' },
      { id: 's-2', sessionNumber: 4, section: 'Day 2', name: 'Finals', date: '2026-05-02', warmupTime: '07:00', startTime: '08:00' },
    ],
  }
  const events: ParsedMeetEventDraftRecord[] = [
    { session_number: 4, event_number: '12', stroke: 'freestyle', distance: 100, is_relay: false },
  ]

  const payload = buildCreateMeetPayload(draft, events)

  assert.deepEqual(payload.sessions?.map((session) => session.session_number), [2, 4])
  assert.equal(payload.events?.[0]?.session_number, 4)
})

test('buildCreateMeetPayload carries host fees from the events file and the per-swimmer fee', () => {
  const draft: MeetCreateDraftState = {
    name: 'Fall Festival',
    course: 'SCY',
    hostClub: 'Host',
    location: 'Aquatic Center',
    startDate: '2026-11-14',
    endDate: '2026-11-15',
    commitmentDeadline: '',
    entryDeadline: '',
    maxEntriesPerSwimmer: '3',
    swimmerFee: '15.50',
    swimmerSelfSignup: true,
    sessions: [],
  }
  const events: ParsedMeetEventDraftRecord[] = [
    { session_number: 1, event_number: '1', stroke: 'freestyle', distance: 200, is_relay: false, entry_fee_cents: 600 },
    { session_number: 1, event_number: '2', stroke: 'freestyle', distance: 50, is_relay: false },
  ]

  const payload = buildCreateMeetPayload(draft, events)

  assert.deepEqual(payload.events?.map((event) => event.entry_fee_cents), [600, 0])
  assert.equal(payload.signup_settings?.swimmer_fee_cents, 1550)
  assert.equal(payload.age_up_date, undefined)
  assert.equal(buildCreateMeetPayload({ ...draft, ageUpDate: '2026-12-31' }, events).age_up_date, '2026-12-31')
})

test('dollarsToCents reads typed amounts and treats blanks or junk as no fee', () => {
  assert.equal(dollarsToCents('35'), 3500)
  assert.equal(dollarsToCents('$1,015.5'), 101550)
  assert.equal(dollarsToCents('6.005'), 601)
  for (const value of ['', '  ', 'abc', '-4', '1.2.3']) assert.equal(dollarsToCents(value), 0)
})

test('isMeetEventFileName accepts only the host event formats', () => {
  for (const name of ['Meet Events.hyv', 'MEET EVENTS.EV3', 'meet-events-002.zip']) assert.equal(isMeetEventFileName(name), true)
  for (const name of ['entries.sd3', 'entries.hy3', 'results.cl2', 'meet-info.pdf', 'hyv']) assert.equal(isMeetEventFileName(name), false)
})
