import { parseBackendDate } from './dates.ts'
import type { PublicEvent, PublicMeet, PublicRecord } from './publicSiteData.ts'

/** Short plain-text teaser for a news body. */
export function excerpt(body: string, max = 180): string {
  const flat = body.replace(/\s+/g, ' ').trim()
  if (flat.length <= max) return flat
  const cut = flat.slice(0, max)
  const lastSpace = cut.lastIndexOf(' ')
  return `${(lastSpace > max * 0.6 ? cut.slice(0, lastSpace) : cut).replace(/[.,;:!?-]+$/, '')}…`
}

/** All-day events are stored at UTC midnight and must not shift across time zones. */
function eventDate(e: Pick<PublicEvent, 'starts_at' | 'all_day'>): { date: Date; timeZone?: string } {
  return { date: new Date(e.starts_at), timeZone: e.all_day ? 'UTC' : undefined }
}

export function formatEventDay(e: Pick<PublicEvent, 'starts_at' | 'all_day'>, opts: Intl.DateTimeFormatOptions): string {
  const { date, timeZone } = eventDate(e)
  return date.toLocaleDateString('en-US', { ...opts, timeZone })
}

export function formatEventTime(e: Pick<PublicEvent, 'starts_at' | 'all_day'>): string {
  if (e.all_day) return 'All day'
  return new Date(e.starts_at).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
}

export function formatMeetDates(m: Pick<PublicMeet, 'start_date' | 'end_date'>): string {
  if (!m.start_date) return 'Dates to be announced'
  const start = parseBackendDate(m.start_date)
  const end = m.end_date && m.end_date !== m.start_date ? parseBackendDate(m.end_date) : null
  const month = (d: Date) => d.toLocaleDateString('en-US', { month: 'short' })
  if (!end) return start.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
  if (start.getMonth() === end.getMonth() && start.getFullYear() === end.getFullYear()) {
    return `${month(start)} ${start.getDate()}–${end.getDate()}, ${end.getFullYear()}`
  }
  return `${month(start)} ${start.getDate()} – ${month(end)} ${end.getDate()}, ${end.getFullYear()}`
}

export function monthKey(iso: string): string {
  return iso.slice(0, 7)
}

export function monthLabel(key: string): string {
  return parseBackendDate(`${key}-01`).toLocaleDateString('en-US', { month: 'long', year: 'numeric' })
}

/** Groups items by YYYY-MM, preserving input order within and across groups. */
export function groupByMonth<T>(items: T[], dateOf: (item: T) => string | undefined): { key: string; items: T[] }[] {
  const out: { key: string; items: T[] }[] = []
  for (const item of items) {
    const d = dateOf(item)
    if (!d) continue
    const key = monthKey(d)
    const last = out[out.length - 1]
    if (last && last.key === key) last.items.push(item)
    else out.push({ key, items: [item] })
  }
  return out
}

export const COURSE_LABELS: Record<string, string> = { SCY: 'Short course yards', SCM: 'Short course meters', LCM: 'Long course meters' }
export const GENDER_LABELS: Record<string, string> = { F: 'Girls and women', M: 'Boys and men' }

export type RecordFilter = { course: string; gender: string }

/** Picks a valid course/gender pair, defaulting to the first course that has records. */
export function resolveRecordFilter(records: PublicRecord[], course?: string, gender?: string): RecordFilter {
  const courses = Array.from(new Set(records.map((r) => r.course)))
  const c = course && courses.includes(course) ? course : courses[0] ?? 'SCY'
  const genders = Array.from(new Set(records.filter((r) => r.course === c).map((r) => r.gender)))
  const g = gender && genders.includes(gender) ? gender : genders.includes('F') ? 'F' : genders[0] ?? 'F'
  return { course: c, gender: g }
}

/** Records for a filter, grouped by age group in the order the server sent them. */
export function groupRecordsByAge(records: PublicRecord[], f: RecordFilter): { ageGroup: string; records: PublicRecord[] }[] {
  const out: { ageGroup: string; records: PublicRecord[] }[] = []
  for (const r of records) {
    if (r.course !== f.course || r.gender !== f.gender) continue
    const last = out[out.length - 1]
    if (last && last.ageGroup === r.age_group) last.records.push(r)
    else out.push({ ageGroup: r.age_group, records: [r] })
  }
  return out
}

/** Strips a trailing age-group prefix/suffix so rows read as just the event ("100 Freestyle"). */
export function eventName(r: Pick<PublicRecord, 'distance' | 'stroke'>): string {
  const stroke = r.stroke.replace(/_/g, ' ')
  return `${r.distance} ${stroke.charAt(0).toUpperCase()}${stroke.slice(1)}`
}
