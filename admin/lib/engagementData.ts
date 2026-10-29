'use client'

// Fetch helpers + types for club events, calendar feeds, wellness check-ins
// and attendance summaries (server/internal/app/{events,wellness,attendance}).
import type { ApiClient } from './api'
import { orgHeaders } from './api'
import { fetchAllPages } from './adminData'

// ── Club events ──────────────────────────────────────────────────────────────

export const CLUB_EVENT_CATEGORIES = ['social', 'fundraiser', 'team_meeting', 'clinic', 'banquet', 'dryland', 'other'] as const
export type ClubEventCategory = (typeof CLUB_EVENT_CATEGORIES)[number]

export const CLUB_EVENT_CATEGORY_LABELS: Record<ClubEventCategory, string> = {
  social: 'Social', fundraiser: 'Fundraiser', team_meeting: 'Team meeting', clinic: 'Clinic',
  banquet: 'Banquet', dryland: 'Dryland', other: 'Other',
}

export type RSVPResponse = 'yes' | 'no' | 'maybe'

export type ClubEventRSVP = {
  id: string
  event_id: string
  household_id: string
  household_name?: string
  swimmer_id?: string
  swimmer_name?: string
  response: RSVPResponse
  guest_count: number
  note?: string
  invoice_id?: string
  billed_quantity: number
  updated_at: string
}

export type ClubEventSummary = { yes: number; no: number; maybe: number; yes_headcount: number; spots_left?: number }

export type ClubEventInput = {
  title: string
  category: ClubEventCategory
  description: string
  location: string
  starts_at: string
  ends_at?: string | null
  all_day: boolean
  audience: 'all' | 'groups'
  group_codes: string[]
  rsvp_enabled: boolean
  rsvp_mode: 'household' | 'swimmer'
  rsvp_deadline?: string | null
  capacity?: number | null
  fee_cents: number
  status: 'scheduled' | 'cancelled'
  public?: boolean
}

export type ClubEventRecord = ClubEventInput & {
  id: string
  org_id: string
  rsvp_open: boolean
  rsvp_summary: ClubEventSummary
  my_rsvps?: ClubEventRSVP[]
  eligible_swimmer_ids?: string[]
  eligible_swimmers?: { id: string; household_id: string; display_name: string }[]
  created_at: string
  updated_at: string
}

export async function fetchClubEvents(api: ApiClient, orgId: string, range?: { from?: string; to?: string }): Promise<ClubEventRecord[]> {
  const qs = new URLSearchParams()
  if (range?.from) qs.set('from', range.from)
  if (range?.to) qs.set('to', range.to)
  const q = qs.toString()
  return fetchAllPages<ClubEventRecord>(api, `/api/v1/club-events${q ? `?${q}` : ''}`, 'events', orgId)
}

export async function createClubEvent(api: ApiClient, orgId: string, body: ClubEventInput): Promise<ClubEventRecord> {
  return (await api.post<{ event: ClubEventRecord }>('/api/v1/club-events', body, orgHeaders(orgId))).event
}

export async function updateClubEvent(api: ApiClient, orgId: string, id: string, body: ClubEventInput): Promise<ClubEventRecord> {
  return (await api.put<{ event: ClubEventRecord }>(`/api/v1/club-events/${id}`, body, orgHeaders(orgId))).event
}

export async function deleteClubEvent(api: ApiClient, orgId: string, id: string): Promise<void> {
  await api.delete(`/api/v1/club-events/${id}`, orgHeaders(orgId))
}

export async function fetchClubEventRSVPs(api: ApiClient, orgId: string, id: string): Promise<{ rsvps: ClubEventRSVP[]; summary: ClubEventSummary }> {
  const res = await api.get<{ rsvps?: ClubEventRSVP[]; summary: ClubEventSummary }>(`/api/v1/club-events/${id}/rsvps`, undefined, orgHeaders(orgId))
  return { rsvps: res.rsvps ?? [], summary: res.summary }
}

export type RSVPBillingOutcome = 'none' | 'charged' | 'unchanged' | 'released' | 'retained_paid' | 'failed'

export async function respondClubEvent(
  api: ApiClient,
  orgId: string,
  id: string,
  body: { household_id?: string; swimmer_id?: string; response: RSVPResponse; guest_count?: number; note?: string },
): Promise<{ rsvp: ClubEventRSVP; billing: RSVPBillingOutcome }> {
  return api.put(`/api/v1/club-events/${id}/rsvps`, body, orgHeaders(orgId))
}

export function clubEventExportPath(id: string): string {
  return `/api/v1/club-events/${id}/rsvps/export`
}

// ── Calendar feed ────────────────────────────────────────────────────────────

export type CalendarFeedStatus = {
  household_id: string
  active: boolean
  feed?: { id: string; created_at: string; last_used_at?: string } | null
  token?: string
  feed_path?: string
}

export async function fetchCalendarFeed(api: ApiClient, orgId: string): Promise<CalendarFeedStatus> {
  return api.get<CalendarFeedStatus>('/api/v1/calendar/feed-token', undefined, orgHeaders(orgId))
}

export async function rotateCalendarFeed(api: ApiClient, orgId: string): Promise<CalendarFeedStatus> {
  return api.post<CalendarFeedStatus>('/api/v1/calendar/feed-token', undefined, orgHeaders(orgId))
}

export async function revokeCalendarFeed(api: ApiClient, orgId: string): Promise<CalendarFeedStatus> {
  return api.delete<CalendarFeedStatus>('/api/v1/calendar/feed-token', orgHeaders(orgId))
}

// ── Wellness ─────────────────────────────────────────────────────────────────

export type WellnessMetrics = {
  sleep_hours: number
  sleep_quality: number
  soreness: number
  energy: number
  stress: number
  mood: number
}

export type WellnessCheckIn = WellnessMetrics & {
  id: string
  swimmer_id: string
  date: string
  note?: string
  readiness: number
  submitted_role: string
  updated_at: string
}

export type WellnessFlag = { code: string; severity: 'warning' | 'alert'; message: string; since: string }

export type WellnessAverages = WellnessMetrics & { count: number; readiness: number }

export type SwimmerWellness = {
  swimmer: { id: string; display_name: string; household_id: string }
  from: string
  to: string
  today: string
  checkins: WellnessCheckIn[]
  today_checkin?: WellnessCheckIn | null
  flags: WellnessFlag[]
  averages: WellnessAverages
}

export type WellnessHeatmap = {
  group: string
  days: string[]
  swimmers: {
    swimmer_id: string
    display_name: string
    cells: { date: string; checkin: WellnessCheckIn | null; readiness: number | null }[]
    flags: WellnessFlag[]
    averages: WellnessAverages
  }[]
  daily: { date: string; count: number; readiness: number | null }[]
  flagged_count: number
  roster_size: number
  checked_in_today: number
}

export async function submitWellnessCheckIn(
  api: ApiClient,
  orgId: string,
  body: WellnessMetrics & { swimmer_id?: string; date?: string; note?: string },
): Promise<{ checkin: WellnessCheckIn; created: boolean }> {
  return api.put('/api/v1/wellness/checkins', body, orgHeaders(orgId))
}

export async function fetchSwimmerWellness(api: ApiClient, orgId: string, swimmerId: string, days = 30): Promise<SwimmerWellness> {
  return api.get<SwimmerWellness>(`/api/v1/wellness/swimmers/${swimmerId}?days=${days}`, undefined, orgHeaders(orgId))
}

export async function fetchWellnessHeatmap(api: ApiClient, orgId: string, group: string, days = 14): Promise<WellnessHeatmap> {
  const qs = new URLSearchParams({ days: String(days) })
  if (group) qs.set('group', group)
  return api.get<WellnessHeatmap>(`/api/v1/wellness/heatmap?${qs}`, undefined, orgHeaders(orgId))
}

// ── Attendance summary ───────────────────────────────────────────────────────

export type AttendanceSummary = {
  swimmer_id: string
  from: string
  to: string
  eligible_practices: number
  present: number
  absent: number
  excused: number
  unmarked: number
  attendance_pct: number | null
  recent: { practice_id: string; date: string; type: string; status: string }[]
}

export async function fetchAttendanceSummary(api: ApiClient, orgId: string, swimmerId: string, days = 90): Promise<AttendanceSummary> {
  const res = await api.get<{ summary: AttendanceSummary }>(`/api/v1/swimmers/${swimmerId}/attendance-summary?days=${days}`, undefined, orgHeaders(orgId))
  return res.summary
}
