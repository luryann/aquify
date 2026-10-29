// Pure helpers for club events (form drafts, date handling, calendar keys).
import type { ClubEventInput, ClubEventRecord, ClubEventRSVP, ClubEventSummary } from './engagementData'
import { localInputToISO } from './dates.ts'

const pad = (n: number) => String(n).padStart(2, '0')

/** YYYY-MM-DD for a Date in the viewer's local time zone. */
function localDateKey(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** ISO timestamp -> value for <input type="datetime-local"> in local time. */
export function isoToLocalInput(iso?: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${localDateKey(d)}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export { localInputToISO }

/** All-day events carry UTC-midnight timestamps; the date part is the day. */
export function allDayISO(date: string): string {
  return `${date}T00:00:00Z`
}

/** Calendar date (YYYY-MM-DD) an event falls on for the viewer. */
export function clubEventDateKey(e: Pick<ClubEventRecord, 'starts_at' | 'all_day'>): string {
  if (e.all_day) return e.starts_at.slice(0, 10)
  return localDateKey(new Date(e.starts_at))
}

export function formatClubEventWhen(e: Pick<ClubEventRecord, 'starts_at' | 'ends_at' | 'all_day'>): string {
  if (e.all_day) {
    const start = new Date(`${e.starts_at.slice(0, 10)}T00:00:00`)
    const label = start.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
    const endDay = e.ends_at?.slice(0, 10)
    if (endDay && endDay !== e.starts_at.slice(0, 10)) {
      return `${label} – ${new Date(`${endDay}T00:00:00`).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}`
    }
    return `${label}, all day`
  }
  const start = new Date(e.starts_at)
  const day = start.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
  const t = (d: Date) => d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
  return e.ends_at ? `${day}, ${t(start)} – ${t(new Date(e.ends_at))}` : `${day}, ${t(start)}`
}

export function formatFee(cents: number): string {
  return `$${(cents / 100).toFixed(cents % 100 === 0 ? 0 : 2)}`
}

export type ClubEventDraft = {
  title: string
  category: ClubEventInput['category']
  description: string
  location: string
  allDay: boolean
  startDate: string
  endDate: string
  startsAt: string
  endsAt: string
  audience: 'all' | 'groups'
  groupCodes: string[]
  rsvpEnabled: boolean
  rsvpMode: 'household' | 'swimmer'
  rsvpDeadline: string
  capacity: string
  fee: string
  status: 'scheduled' | 'cancelled'
  isPublic: boolean
}

export function emptyClubEventDraft(): ClubEventDraft {
  return {
    title: '', category: 'social', description: '', location: '', allDay: false,
    startDate: '', endDate: '', startsAt: '', endsAt: '', audience: 'all', groupCodes: [],
    rsvpEnabled: true, rsvpMode: 'household', rsvpDeadline: '', capacity: '', fee: '', status: 'scheduled', isPublic: false,
  }
}

export function draftFromClubEvent(e: ClubEventRecord): ClubEventDraft {
  return {
    title: e.title, category: e.category, description: e.description ?? '', location: e.location ?? '',
    allDay: e.all_day,
    startDate: e.all_day ? e.starts_at.slice(0, 10) : '',
    endDate: e.all_day && e.ends_at ? e.ends_at.slice(0, 10) : '',
    startsAt: e.all_day ? '' : isoToLocalInput(e.starts_at),
    endsAt: e.all_day ? '' : isoToLocalInput(e.ends_at),
    audience: e.audience, groupCodes: e.group_codes ?? [],
    rsvpEnabled: e.rsvp_enabled, rsvpMode: e.rsvp_mode,
    rsvpDeadline: isoToLocalInput(e.rsvp_deadline),
    capacity: e.capacity ? String(e.capacity) : '',
    fee: e.fee_cents ? (e.fee_cents / 100).toFixed(2) : '',
    status: e.status, isPublic: e.public ?? false,
  }
}

/** Validates a draft and builds the API body; returns an error string instead. */
export function clubEventInputFromDraft(d: ClubEventDraft): ClubEventInput | string {
  if (!d.title.trim()) return 'Give the event a title.'
  let startsAt: string | null
  let endsAt: string | null = null
  if (d.allDay) {
    if (!d.startDate) return 'Pick the event date.'
    startsAt = allDayISO(d.startDate)
    if (d.endDate) {
      if (d.endDate < d.startDate) return 'The end date is before the start date.'
      endsAt = allDayISO(d.endDate)
    }
  } else {
    startsAt = localInputToISO(d.startsAt)
    if (!startsAt) return 'Pick a start date and time.'
    endsAt = localInputToISO(d.endsAt)
    if (endsAt && endsAt < startsAt) return 'The end time is before the start time.'
  }
  if (d.audience === 'groups' && d.groupCodes.length === 0) return 'Choose at least one group, or invite everyone.'
  const capacity = d.capacity.trim() ? Number.parseInt(d.capacity, 10) : null
  if (capacity !== null && (!Number.isFinite(capacity) || capacity < 1)) return 'Capacity must be a whole number above zero.'
  const feeCents = d.fee.trim() ? Math.round(Number.parseFloat(d.fee) * 100) : 0
  if (!Number.isFinite(feeCents) || feeCents < 0) return 'The fee must be a dollar amount.'
  if (feeCents > 0 && !d.rsvpEnabled) return 'Turn on RSVPs to charge a fee.'
  const rsvpDeadline = d.rsvpEnabled ? localInputToISO(d.rsvpDeadline) : null
  if (rsvpDeadline && rsvpDeadline > startsAt) return 'The RSVP deadline must be before the event starts.'
  return {
    title: d.title.trim(), category: d.category, description: d.description.trim(), location: d.location.trim(),
    starts_at: startsAt, ends_at: endsAt, all_day: d.allDay, audience: d.audience,
    group_codes: d.audience === 'groups' ? d.groupCodes : [],
    rsvp_enabled: d.rsvpEnabled, rsvp_mode: d.rsvpMode, rsvp_deadline: rsvpDeadline,
    capacity: d.rsvpEnabled ? capacity : null, fee_cents: feeCents, status: d.status,
    public: d.audience === 'all' && d.isPublic,
  }
}

/** The household-level or per-swimmer RSVP a viewer already gave. */
export function findMyRSVP(rsvps: ClubEventRSVP[] | undefined, swimmerId?: string): ClubEventRSVP | undefined {
  return (rsvps ?? []).find((r) => (swimmerId ? r.swimmer_id === swimmerId : !r.swimmer_id))
}

export const RSVP_LABELS = { yes: 'Going', maybe: 'Maybe', no: 'Not going' } as const

export function billingNotice(outcome: string, feeCents: number): string | null {
  switch (outcome) {
    case 'charged': return `An invoice for the ${formatFee(feeCents)} per-person fee is in Billing.`
    case 'released': return 'The unpaid event invoice was cancelled.'
    case 'retained_paid': return 'Your payment is kept on file. Contact the club about a refund.'
    case 'failed': return 'RSVP saved, but the fee invoice could not be created. The club has been notified.'
    default: return null
  }
}

/** Applies one changed RSVP to an event's summary so counts update without a refetch. */
export function applyRSVPToSummary(summary: ClubEventSummary, prev: ClubEventRSVP | undefined, next: ClubEventRSVP): ClubEventSummary {
  const out = { ...summary }
  const bump = (r: ClubEventRSVP, sign: 1 | -1) => {
    out[r.response] = Math.max(0, out[r.response] + sign)
    if (r.response === 'yes') {
      const heads = 1 + Math.max(0, r.guest_count || 0)
      out.yes_headcount = Math.max(0, out.yes_headcount + sign * heads)
      if (out.spots_left !== undefined && out.spots_left !== null) out.spots_left = Math.max(0, out.spots_left - sign * heads)
    }
  }
  if (prev) bump(prev, -1)
  bump(next, 1)
  return out
}
