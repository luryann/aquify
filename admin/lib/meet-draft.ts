import type { CreateMeetPayload, ParsedMeetEventDraftRecord, ParsedMeetSessionDraft } from './adminData.ts'

export type Course = 'SCY' | 'SCM' | 'LCM'

export type MeetSessionDraft = {
  id: string
  sessionNumber: number
  section: string
  name: string
  date: string
  warmupTime: string
  startTime: string
}

export type MeetCreateDraftState = {
  name: string
  course: Course
  hostClub: string
  location: string
  startDate: string
  endDate: string
  commitmentDeadline: string
  entryDeadline: string
  maxEntriesPerSwimmer: string
  swimmerSelfSignup: boolean
  sessions: MeetSessionDraft[]
}

export function maxEntriesPayloadValue(value: string): number {
  return value === 'Unlimited' ? 6 : Number(value || '3')
}

export function mapParsedSessionsToDraftSessions(parsedSessions: ParsedMeetSessionDraft[]): MeetSessionDraft[] {
  return parsedSessions.map((session, idx) => ({
    id: `parsed-${idx + 1}`,
    sessionNumber: session.session_number || idx + 1,
    section: session.section,
    name: session.name,
    date: session.session_date ?? '',
    warmupTime: session.warmup_time ?? '',
    startTime: session.start_time ?? '',
  }))
}

export function getNextSessionNumber(sessions: Array<Pick<MeetSessionDraft, 'sessionNumber'>>): number {
  return sessions.reduce((max, session) => Math.max(max, session.sessionNumber), 0) + 1
}

export function buildCreateMeetPayload(
  draft: MeetCreateDraftState,
  parsedEvents: ParsedMeetEventDraftRecord[],
): CreateMeetPayload {
  return {
    name: draft.name,
    course: draft.course,
    host_org: draft.hostClub,
    location: draft.location,
    start_date: draft.startDate || undefined,
    end_date: draft.endDate || undefined,
    commitment_deadline: draft.commitmentDeadline || undefined,
    entry_deadline: draft.entryDeadline || undefined,
    sessions: draft.sessions.map((session) => ({
      session_number: session.sessionNumber,
      name: session.name || session.section,
      session_date: session.date || undefined,
      warmup_time: session.warmupTime || undefined,
      start_time: session.startTime || undefined,
    })),
    events: parsedEvents.map((event) => ({
      session_number: event.session_number || undefined,
      event_number: event.event_number,
      stroke: event.stroke,
      distance: event.distance,
      course: event.course || draft.course,
      gender: event.gender || undefined,
      min_age: event.min_age,
      max_age: event.max_age,
      entry_fee_cents: 0,
      is_relay: event.is_relay,
    })),
    signup_settings: {
      self_signup_enabled: draft.swimmerSelfSignup,
      max_entries_per_swimmer: maxEntriesPayloadValue(draft.maxEntriesPerSwimmer),
    },
  }
}
