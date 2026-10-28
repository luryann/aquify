import type { ApiClient } from '@/lib/api'

// Team records, top times, goals, relay builder and progress reports.
// Backend: server/internal/app/handlers/performance_extensions.go.

function org(orgId: string) {
  return { 'X-Org-Id': orgId }
}

function qs(params: Record<string, string | number | undefined | null>) {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') q.set(k, String(v))
  }
  const s = q.toString()
  return s ? `?${s}` : ''
}

export type Course = 'SCY' | 'SCM' | 'LCM'
export type IndividualStroke = 'free' | 'back' | 'breast' | 'fly' | 'im'
export const COURSES: Course[] = ['SCY', 'SCM', 'LCM']
export const STROKES: IndividualStroke[] = ['free', 'back', 'breast', 'fly', 'im']
export const AGE_GROUPS = ['8U', '9-10', '11-12', '13-14', '15-16', '17-18', 'OPEN'] as const
export type AgeGroup = (typeof AGE_GROUPS)[number]

export const STROKE_LABELS: Record<string, string> = { free: 'Free', back: 'Back', breast: 'Breast', fly: 'Fly', im: 'IM' }
export const AGE_GROUP_LABELS: Record<string, string> = {
  '8U': '8 & under', '9-10': '9–10', '11-12': '11–12', '13-14': '13–14', '15-16': '15–16', '17-18': '17–18', OPEN: 'Open',
}

export const EVENT_DISTANCES: Record<IndividualStroke, number[]> = {
  free: [50, 100, 200, 400, 500, 800, 1000, 1500, 1650],
  back: [50, 100, 200],
  breast: [50, 100, 200],
  fly: [50, 100, 200],
  im: [100, 200, 400],
}

/** Formats milliseconds as SS.hh or M:SS.hh. */
export function formatSwimTime(ms?: number | null): string {
  if (ms == null || !Number.isFinite(ms)) return 'NT'
  const hundredths = Math.round(ms / 10)
  const minutes = Math.floor(hundredths / 6000)
  const seconds = Math.floor((hundredths % 6000) / 100)
  const frac = hundredths % 100
  const ss = minutes > 0 ? String(seconds).padStart(2, '0') : String(seconds)
  return `${minutes > 0 ? `${minutes}:` : ''}${ss}.${String(frac).padStart(2, '0')}`
}

/** Parses "58.12", "1:02.34" or "1:02" into milliseconds; null when invalid. */
export function parseSwimTime(raw: string): number | null {
  const v = raw.trim()
  const m = /^(?:(\d{1,2}):)?(\d{1,2})(?:\.(\d{1,2}))?$/.exec(v)
  if (!m) return null
  const minutes = m[1] ? Number(m[1]) : 0
  const seconds = Number(m[2])
  if (m[1] && seconds >= 60) return null
  const frac = m[3] ? Number(m[3].padEnd(2, '0')) : 0
  const ms = (minutes * 60 + seconds) * 1000 + frac * 10
  return ms > 0 ? ms : null
}

/** "+1.23" for a drop (faster), "−0.40" when slower. */
export function formatDrop(ms: number): string {
  const sign = ms >= 0 ? '−' : '+'
  return `${sign}${formatSwimTime(Math.abs(ms))}`
}

export function genderLabel(gender: string, ageGroup: string): string {
  if (gender === 'X') return 'Mixed'
  if (ageGroup === 'OPEN') return gender === 'F' ? 'Women' : 'Men'
  return gender === 'F' ? 'Girls' : 'Boys'
}

export function eventLabel(distance: number, stroke: string, course?: string) {
  return `${distance} ${STROKE_LABELS[stroke] ?? stroke}${course ? ` ${course}` : ''}`
}

// ─── Types ───────────────────────────────────────────────────────────────────

export type RecordEntry = {
  id: string
  course: Course
  gender: 'M' | 'F'
  age_group: AgeGroup
  distance: number
  stroke: IndividualStroke
  time_ms: number
  swimmer_id?: string
  swimmer_name: string
  swimmer_age?: number
  swim_date: string
  meet_id?: string
  meet_name?: string
  result_id?: string
  source: 'result' | 'manual'
  status: 'accepted' | 'pending' | 'rejected'
  locked: boolean
  verified: boolean
  verified_by?: string
  notes?: string
}

export type TeamRecord = {
  course: Course
  gender: 'M' | 'F'
  age_group: AgeGroup
  distance: number
  stroke: IndividualStroke
  label: string
  record: RecordEntry | null
  previous?: RecordEntry | null
  history_count: number
  pending_count: number
}

export type RecordHistory = {
  label: string
  current?: RecordEntry | null
  progression: RecordEntry[]
  pending: RecordEntry[]
  rejected: RecordEntry[]
}

export type RecordBreak = {
  entry_id: string
  label: string
  age_group: AgeGroup
  time_ms: number
  previous_time_ms?: number
  previous_holder?: string
  swimmer_name: string
  status: 'accepted' | 'pending'
}

export type TopTimeEntry = {
  rank: number
  swimmer_id: string
  swimmer_name: string
  age?: number
  time_ms: number
  swim_date: string
  meet_id?: string
  meet_name?: string
  result_id: string
}

export type TopTimesList = {
  course: Course
  gender: 'M' | 'F'
  age_group: AgeGroup
  distance: number
  stroke: IndividualStroke
  label: string
  entries: TopTimeEntry[]
}

export type Goal = {
  id: string
  swimmer_id: string
  distance: number
  stroke: IndividualStroke
  course: Course
  target_time_ms: number
  baseline_time_ms?: number
  deadline?: string
  notes?: string
  status: 'active' | 'archived'
  best_time_ms: number | null
  gap_ms?: number
  progress_percent?: number
  achieved: boolean
  days_remaining?: number
  overdue: boolean
}

export type RelayLeg = {
  leg: number
  stroke: string
  swimmer_id: string
  swimmer_name: string
  gender?: string
  time_ms: number
  locked: boolean
}

export type RelayTeam = { label: string; legs: RelayLeg[]; total_ms: number }

export type RelayPoolSwimmer = { swimmer_id: string; name: string; gender: string; age?: number; times: Record<string, number> }

export type RelayProposal = {
  course: Course
  gender: 'M' | 'F' | 'X'
  age_group: string
  relay_type: 'free' | 'medley'
  distance: number
  leg_distance: number
  as_of: string
  meet_id?: string
  teams: RelayTeam[]
  warnings: string[]
  pool: RelayPoolSwimmer[]
}

export type RelayLock = { team: number; leg: number; swimmer_id: string }

export type RelayOptimizeRequest = {
  meet_id?: string
  course?: string
  gender: 'M' | 'F' | 'X'
  age_group: string
  relay_type: 'free' | 'medley'
  distance: number
  teams: number
  locks: RelayLock[]
  excluded_swimmer_ids: string[]
}

export type RelayLineup = {
  id: string
  meet_id: string
  name: string
  course: Course
  gender: 'M' | 'F' | 'X'
  age_group: string
  relay_type: 'free' | 'medley'
  distance: number
  teams: RelayTeam[]
  notes?: string
  created_at: string
}

export type ProgressEvent = { distance: number; stroke: string; course: string; before_ms: number; after_ms: number; drop_ms: number; percent: number }

export type SwimmerProgress = {
  swimmer_id: string
  swimmer_name: string
  group_codes: string[]
  events: ProgressEvent[]
  events_compared: number
  events_improved: number
  new_events: number
  avg_percent: number
  total_drop_ms: number
}

export type ProgressReport = {
  period_a: { from: string; to: string }
  period_b: { from: string; to: string }
  swimmers: SwimmerProgress[]
  most_improved: SwimmerProgress[]
}

// ─── Records ─────────────────────────────────────────────────────────────────

export async function fetchTeamRecords(api: ApiClient, orgId: string, params: { course?: string; gender?: string; age_group?: string } = {}) {
  const res = await api.get<{ records?: TeamRecord[] }>(`/api/v1/performance/records${qs(params)}`, undefined, org(orgId))
  return res.records ?? []
}

export async function fetchRecordHistory(api: ApiClient, orgId: string, key: { course: string; gender: string; age_group: string; distance: number; stroke: string }) {
  const res = await api.get<{ history: RecordHistory }>(`/api/v1/performance/records/history${qs(key)}`, undefined, org(orgId))
  return res.history
}

export async function fetchPendingRecords(api: ApiClient, orgId: string) {
  const res = await api.get<{ entries?: RecordEntry[] }>('/api/v1/performance/records/pending', undefined, org(orgId))
  return res.entries ?? []
}

export type ManualRecordInput = {
  course: string; gender: string; age_group: string; distance: number; stroke: string; time_ms: number
  swimmer_name: string; swim_date: string; meet_name?: string; notes?: string; locked?: boolean; verified?: boolean
}

export async function createManualRecord(api: ApiClient, orgId: string, input: ManualRecordInput) {
  const res = await api.post<{ entry: RecordEntry }>('/api/v1/performance/records', input, org(orgId))
  return res.entry
}

export async function updateRecordEntry(api: ApiClient, orgId: string, entryId: string, patch: Partial<Pick<RecordEntry, 'locked' | 'verified' | 'notes'>>) {
  const res = await api.patch<{ entry: RecordEntry }>(`/api/v1/performance/records/${encodeURIComponent(entryId)}`, patch, org(orgId))
  return res.entry
}

export async function decideRecordEntry(api: ApiClient, orgId: string, entryId: string, decision: 'approve' | 'reject') {
  const res = await api.post<{ entry: RecordEntry }>(`/api/v1/performance/records/${encodeURIComponent(entryId)}/${decision}`, {}, org(orgId))
  return res.entry
}

export async function deleteManualRecord(api: ApiClient, orgId: string, entryId: string) {
  await api.delete(`/api/v1/performance/records/${encodeURIComponent(entryId)}`, org(orgId))
}

export async function rebuildRecords(api: ApiClient, orgId: string) {
  const res = await api.post<{ summary: { entries_created: number; entries_removed: number; slots_skipped_locked: number } }>('/api/v1/performance/records/rebuild', {}, org(orgId))
  return res.summary
}

export async function draftRecordAnnouncement(api: ApiClient, orgId: string, entryId: string) {
  const res = await api.post<{ announcement: { id: string } }>(`/api/v1/performance/records/${encodeURIComponent(entryId)}/announcement`, {}, org(orgId))
  return res.announcement
}

// ─── Top times ───────────────────────────────────────────────────────────────

export async function fetchTopTimes(api: ApiClient, orgId: string, params: { course: string; gender?: string; age_group?: string; distance?: number; stroke?: string; from?: string; to?: string; limit?: number }) {
  const res = await api.get<{ lists?: TopTimesList[] }>(`/api/v1/performance/top-times${qs(params)}`, undefined, org(orgId))
  return res.lists ?? []
}

// ─── Goals ───────────────────────────────────────────────────────────────────

export async function fetchSwimmerGoals(api: ApiClient, orgId: string, swimmerId: string) {
  const res = await api.get<{ goals?: Goal[] }>(`/api/v1/swimmers/${encodeURIComponent(swimmerId)}/goals`, undefined, org(orgId))
  return res.goals ?? []
}

export type GoalInput = { distance: number; stroke: string; course: string; target_time_ms: number; deadline?: string; notes?: string }

export async function createSwimmerGoal(api: ApiClient, orgId: string, swimmerId: string, input: GoalInput) {
  const res = await api.post<{ goal: Goal }>(`/api/v1/swimmers/${encodeURIComponent(swimmerId)}/goals`, input, org(orgId))
  return res.goal
}

export async function deleteSwimmerGoal(api: ApiClient, orgId: string, swimmerId: string, goalId: string) {
  await api.delete(`/api/v1/swimmers/${encodeURIComponent(swimmerId)}/goals/${encodeURIComponent(goalId)}`, org(orgId))
}

// ─── Relays ──────────────────────────────────────────────────────────────────

export async function optimizeRelays(api: ApiClient, orgId: string, input: RelayOptimizeRequest) {
  const res = await api.post<{ proposal: RelayProposal }>('/api/v1/performance/relays/optimize', input, org(orgId))
  return res.proposal
}

export async function fetchRelayLineups(api: ApiClient, orgId: string, meetId: string) {
  const res = await api.get<{ lineups?: RelayLineup[] }>(`/api/v1/performance/relay-lineups${qs({ meet_id: meetId, limit: 200 })}`, undefined, org(orgId))
  return res.lineups ?? []
}

export async function saveRelayLineup(api: ApiClient, orgId: string, proposal: RelayProposal, meetId: string, name: string) {
  const body = {
    meet_id: meetId, name, course: proposal.course, gender: proposal.gender, age_group: proposal.age_group,
    relay_type: proposal.relay_type, distance: proposal.distance,
    teams: proposal.teams.map((t) => ({ label: t.label, total_ms: t.total_ms, legs: t.legs })),
  }
  const res = await api.post<{ lineup: RelayLineup }>('/api/v1/performance/relay-lineups', body, org(orgId))
  return res.lineup
}

export async function deleteRelayLineup(api: ApiClient, orgId: string, lineupId: string) {
  await api.delete(`/api/v1/performance/relay-lineups/${encodeURIComponent(lineupId)}`, org(orgId))
}

// ─── Progress ────────────────────────────────────────────────────────────────

export async function fetchProgressReport(api: ApiClient, orgId: string, params: { from_a: string; to_a: string; from_b: string; to_b: string; group_code?: string; course?: string; limit?: number }) {
  const res = await api.get<{ report: ProgressReport }>(`/api/v1/performance/progress-report${qs(params)}`, undefined, org(orgId))
  return res.report
}
