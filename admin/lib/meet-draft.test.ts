import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildCreateMeetPayload,
  getNextSessionNumber,
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
    hostClub: 'DARE',
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
