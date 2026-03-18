export type MeetDocumentType = 'image' | 'doc'

export type MeetDocument = {
  id: string
  name: string
  type: MeetDocumentType
  size: string
  updatedAt: string
  audience: 'family' | 'swimmer' | 'all'
  url: string
}

export type MeetSessionGroup = {
  id: string
  label: string
  warmup: string
  start: string
  notes: string
  eventIds: string[]
}

export type MeetEventOption = {
  id: string
  number: number
  name: string
  gender: 'F' | 'M'
  ageGroup: string
  cut: string | null
  sessionId: string
}

export type MeetRecord = {
  id: string
  name: string
  dateLabel: string
  startDate: string
  endDate: string
  deadlineDate: string
  course: string
  location: string
  host: string
  status: 'published' | 'commitment_open' | 'commitment_closed' | 'completed'
  entryFeePerEvent: number
  notes: string
  overview: string
  selfSignupNote: string
  documents: MeetDocument[]
  sessions: MeetSessionGroup[]
  events: MeetEventOption[]
}

export type HouseholdSwimmer = {
  id: string
  name: string
  initials: string
  gradient: string
  age: number
  gender: 'F' | 'M'
  bestTimes: Record<string, string>
}

export type MeetSignupState = {
  commitment: 'pending' | 'yes' | 'no'
  selectedEvents: string[]
}

export const HOUSEHOLD_SWIMMERS: HouseholdSwimmer[] = [
  {
    id: 'sw-001',
    name: 'Emma Chen',
    initials: 'EC',
    gradient: 'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
    age: 14,
    gender: 'F',
    bestTimes: {
      '200 Free': '2:05.31',
      '100 Back': '1:03.44',
      '100 Fly': '1:05.18',
      '50 Free': '26.44',
      '200 IM': '2:18.50',
      '100 Breast': '1:12.77',
      '400 IM': '4:48.40',
      '200 Back': '2:20.12',
    },
  },
  {
    id: 'sw-002',
    name: 'Noah Chen',
    initials: 'NC',
    gradient: 'linear-gradient(135deg, #10b981 0%, #059669 100%)',
    age: 11,
    gender: 'M',
    bestTimes: {
      '100 Free': '1:02.11',
      '200 Back': '2:41.55',
      '50 Breast': '39.22',
      '50 Free': '29.05',
      '100 IM': '1:15.02',
      '50 Fly': '31.80',
    },
  },
]

export const PRIMARY_SWIMMER_ID = 'sw-001'

export const MEETS: MeetRecord[] = [
  {
    id: 'meet-001',
    name: 'Tri-City Classic',
    dateLabel: 'Mar 22–23, 2026',
    startDate: '2026-03-22',
    endDate: '2026-03-23',
    deadlineDate: '2026-03-19T23:59:59',
    course: 'SCY',
    location: 'Riverside Aquatic Center',
    host: 'Riverside Swim Club',
    status: 'commitment_open',
    entryFeePerEvent: 8.0,
    notes:
      'Bring your team cap. Warmups start at 7:00 AM. Entry deadline is firm with no late additions.',
    overview:
      'Tri-City Classic is an early-season travel meet with Saturday morning and Sunday morning timed finals plus a Saturday sprint session.',
    selfSignupNote: 'Self-signup available until the posted family deadline.',
    documents: [
      {
        id: 'doc-001',
        name: 'Tri-City Classic Meet Packet.html',
        type: 'doc',
        size: '12 KB',
        updatedAt: 'Mar 14',
        audience: 'all',
        url: '/meet-files/tri-city-classic-packet.html',
      },
      {
        id: 'doc-002',
        name: 'Facility Parking Guide.html',
        type: 'doc',
        size: '9 KB',
        updatedAt: 'Mar 14',
        audience: 'all',
        url: '/meet-files/tri-city-classic-parking-guide.html',
      },
      {
        id: 'doc-003',
        name: 'Warm-Up Assignment Sheet.svg',
        type: 'image',
        size: '18 KB',
        updatedAt: 'Mar 16',
        audience: 'all',
        url: '/meet-files/tri-city-classic-warmups.svg',
      },
    ],
    sessions: [
      {
        id: 'sat-am',
        label: 'Saturday AM',
        warmup: '7:00 AM',
        start: '8:15 AM',
        notes: 'Distance check-in closes 30 minutes before the session starts.',
        eventIds: ['ev-01', 'ev-02', 'ev-07'],
      },
      {
        id: 'sat-pm',
        label: 'Saturday PM',
        warmup: '1:00 PM',
        start: '2:00 PM',
        notes: 'Sprint session. Coaches want swimmers on deck 20 minutes before parade.',
        eventIds: ['ev-03', 'ev-04', 'ev-08'],
      },
      {
        id: 'sun-am',
        label: 'Sunday AM',
        warmup: '7:30 AM',
        start: '8:45 AM',
        notes: 'Positive check-in required for the opening 200 IM events.',
        eventIds: ['ev-05', 'ev-06', 'ev-09'],
      },
    ],
    events: [
      { id: 'ev-01', number: 1, name: '200 Free', gender: 'F', ageGroup: '13-14', cut: '2:10.00', sessionId: 'sat-am' },
      { id: 'ev-02', number: 3, name: '100 Back', gender: 'F', ageGroup: '13-14', cut: '1:08.00', sessionId: 'sat-am' },
      { id: 'ev-03', number: 5, name: '100 Fly', gender: 'F', ageGroup: '13-14', cut: '1:10.00', sessionId: 'sat-pm' },
      { id: 'ev-04', number: 7, name: '50 Free', gender: 'F', ageGroup: '13-14', cut: null, sessionId: 'sat-pm' },
      { id: 'ev-05', number: 9, name: '200 IM', gender: 'F', ageGroup: '13-14', cut: '2:28.00', sessionId: 'sun-am' },
      { id: 'ev-06', number: 11, name: '100 Breast', gender: 'F', ageGroup: '13-14', cut: '1:18.00', sessionId: 'sun-am' },
      { id: 'ev-07', number: 2, name: '100 Free', gender: 'M', ageGroup: '11-12', cut: null, sessionId: 'sat-am' },
      { id: 'ev-08', number: 4, name: '200 Back', gender: 'M', ageGroup: '11-12', cut: '2:25.00', sessionId: 'sat-pm' },
      { id: 'ev-09', number: 6, name: '50 Breast', gender: 'M', ageGroup: '11-12', cut: null, sessionId: 'sun-am' },
    ],
  },
  {
    id: 'meet-002',
    name: 'Regional Spring Invite',
    dateLabel: 'Apr 5, 2026',
    startDate: '2026-04-05',
    endDate: '2026-04-05',
    deadlineDate: '2026-03-29T23:59:59',
    course: 'LCM',
    location: 'University Aquatic Center',
    host: 'Pacific Regional Swimming',
    status: 'commitment_open',
    entryFeePerEvent: 10.5,
    notes:
      'This is a long-course meet. Arrive early for parking and expect longer walks from the garage to deck.',
    overview:
      'Regional Spring Invite is a one-day long-course competition with morning prelim-style sessions split by age and a short afternoon finals block.',
    selfSignupNote: 'Families can self-select preliminary events while the commitment window remains open.',
    documents: [
      {
        id: 'doc-101',
        name: 'Regional Spring Invite Packet.html',
        type: 'doc',
        size: '14 KB',
        updatedAt: 'Mar 18',
        audience: 'all',
        url: '/meet-files/regional-spring-invite-packet.html',
      },
      {
        id: 'doc-102',
        name: 'Hotel + Parking Notes.html',
        type: 'doc',
        size: '8 KB',
        updatedAt: 'Mar 18',
        audience: 'family',
        url: '/meet-files/regional-spring-invite-travel-notes.html',
      },
      {
        id: 'doc-103',
        name: 'Session Deck Map.svg',
        type: 'image',
        size: '22 KB',
        updatedAt: 'Mar 18',
        audience: 'all',
        url: '/meet-files/regional-spring-invite-deck-map.svg',
      },
    ],
    sessions: [
      {
        id: 'reg-am',
        label: 'Morning Session',
        warmup: '7:15 AM',
        start: '8:30 AM',
        notes: 'Positive check-in required for the 400 freestyle.',
        eventIds: ['ev-201', 'ev-202', 'ev-205', 'ev-206'],
      },
      {
        id: 'reg-pm',
        label: 'Afternoon Session',
        warmup: '1:15 PM',
        start: '2:15 PM',
        notes: 'Finals block runs oldest to youngest by stroke order.',
        eventIds: ['ev-203', 'ev-204', 'ev-207', 'ev-208'],
      },
    ],
    events: [
      { id: 'ev-201', number: 12, name: '200 Back', gender: 'F', ageGroup: '13-14', cut: '2:40.00', sessionId: 'reg-am' },
      { id: 'ev-202', number: 18, name: '400 IM', gender: 'F', ageGroup: '13-14', cut: '5:18.00', sessionId: 'reg-am' },
      { id: 'ev-203', number: 31, name: '100 Fly', gender: 'F', ageGroup: '13-14', cut: '1:12.50', sessionId: 'reg-pm' },
      { id: 'ev-204', number: 36, name: '50 Free', gender: 'F', ageGroup: '13-14', cut: null, sessionId: 'reg-pm' },
      { id: 'ev-205', number: 14, name: '100 Free', gender: 'M', ageGroup: '11-12', cut: '1:08.50', sessionId: 'reg-am' },
      { id: 'ev-206', number: 20, name: '100 IM', gender: 'M', ageGroup: '11-12', cut: null, sessionId: 'reg-am' },
      { id: 'ev-207', number: 33, name: '50 Fly', gender: 'M', ageGroup: '11-12', cut: null, sessionId: 'reg-pm' },
      { id: 'ev-208', number: 38, name: '50 Breast', gender: 'M', ageGroup: '11-12', cut: '41.50', sessionId: 'reg-pm' },
    ],
  },
  {
    id: 'meet-003',
    name: 'State Junior Championships',
    dateLabel: 'May 2–3, 2026',
    startDate: '2026-05-02',
    endDate: '2026-05-03',
    deadlineDate: '2026-04-24T23:59:59',
    course: 'SCY',
    location: 'State Aquatic Complex',
    host: 'State Swimming',
    status: 'published',
    entryFeePerEvent: 12.0,
    notes:
      'Qualification is stricter for this championship meet and coaches may cap entries before submission.',
    overview:
      'State Junior Championships is a championship-format weekend meet with higher standards, longer sessions, and a tighter family response timeline.',
    selfSignupNote: 'Self-signup opens when the commitment window begins; coach review remains required.',
    documents: [
      {
        id: 'doc-201',
        name: 'Junior Champs Standards.html',
        type: 'doc',
        size: '11 KB',
        updatedAt: 'Mar 18',
        audience: 'all',
        url: '/meet-files/state-junior-champs-standards.html',
      },
      {
        id: 'doc-202',
        name: 'Travel + Team Hotel Sheet.html',
        type: 'doc',
        size: '10 KB',
        updatedAt: 'Mar 18',
        audience: 'family',
        url: '/meet-files/state-junior-champs-travel-sheet.html',
      },
      {
        id: 'doc-203',
        name: 'Championship Timeline.svg',
        type: 'image',
        size: '19 KB',
        updatedAt: 'Mar 18',
        audience: 'all',
        url: '/meet-files/state-junior-champs-timeline.svg',
      },
    ],
    sessions: [
      {
        id: 'state-am',
        label: 'Saturday Prelims',
        warmup: '6:45 AM',
        start: '8:00 AM',
        notes: 'Scratch deadline is 30 minutes after positive check-in closes.',
        eventIds: ['ev-301', 'ev-302'],
      },
      {
        id: 'state-pm',
        label: 'Sunday Championship Block',
        warmup: '7:00 AM',
        start: '8:15 AM',
        notes: 'Finals-qualified athletes will receive deck-side timeline updates from coaches.',
        eventIds: ['ev-303', 'ev-304'],
      },
    ],
    events: [
      { id: 'ev-301', number: 44, name: '200 Free', gender: 'F', ageGroup: '13-14', cut: '2:07.50', sessionId: 'state-am' },
      { id: 'ev-302', number: 48, name: '100 Back', gender: 'F', ageGroup: '13-14', cut: '1:04.20', sessionId: 'state-am' },
      { id: 'ev-303', number: 62, name: '200 IM', gender: 'F', ageGroup: '13-14', cut: '2:21.00', sessionId: 'state-pm' },
      { id: 'ev-304', number: 68, name: '100 Breast', gender: 'F', ageGroup: '13-14', cut: '1:14.00', sessionId: 'state-pm' },
    ],
  },
]

const MEETS_BY_ID = Object.fromEntries(MEETS.map((meet) => [meet.id, meet])) as Record<string, MeetRecord>

const MEET_SIGNUP_STATE_BY_ID: Record<string, Record<string, MeetSignupState>> = {
  'meet-001': {
    'sw-001': { commitment: 'pending', selectedEvents: [] },
    'sw-002': { commitment: 'pending', selectedEvents: [] },
  },
  'meet-002': {
    'sw-001': { commitment: 'yes', selectedEvents: ['ev-201', 'ev-203'] },
    'sw-002': { commitment: 'yes', selectedEvents: ['ev-205'] },
  },
  'meet-003': {
    'sw-001': { commitment: 'pending', selectedEvents: [] },
    'sw-002': { commitment: 'no', selectedEvents: [] },
  },
}

export function getMeetById(id: string): MeetRecord | null {
  return MEETS_BY_ID[id] ?? null
}

export function getFamilyVisibleDocuments(meet: MeetRecord): MeetDocument[] {
  return meet.documents.filter((document) => document.audience !== 'swimmer')
}

export function getSwimmerVisibleDocuments(meet: MeetRecord): MeetDocument[] {
  return meet.documents.filter((document) => document.audience !== 'family')
}

export function getDefaultHouseholdMeetStates(meetId: string): Record<string, MeetSignupState> {
  const preset = MEET_SIGNUP_STATE_BY_ID[meetId] ?? {}
  return Object.fromEntries(
    HOUSEHOLD_SWIMMERS.map((swimmer) => {
      const state = preset[swimmer.id] ?? { commitment: 'pending', selectedEvents: [] }
      return [
        swimmer.id,
        {
          commitment: state.commitment,
          selectedEvents: [...state.selectedEvents],
        },
      ]
    }),
  )
}
