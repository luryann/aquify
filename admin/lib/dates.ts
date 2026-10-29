// Shared date formatting helpers.
//
// Backend date-only fields (meet start_date/end_date, due dates, session dates)
// are serialized as `YYYY-MM-DD`. `new Date("2026-08-15")` parses that as UTC
// midnight, which then renders as the *previous* day in negative-offset
// timezones (e.g. US Pacific) — the classic off-by-one date bug. Appending a
// local time component forces local-midnight parsing so the calendar date is
// preserved. Full ISO datetime strings (with a `T`) are passed through as-is.

const DATE_ONLY = /^\d{4}-\d{2}-\d{2}$/

/** Parse a backend date string without shifting date-only values across a TZ. */
export function parseBackendDate(value: string): Date {
  return new Date(DATE_ONLY.test(value.trim()) ? `${value.trim()}T00:00:00` : value)
}

const DEFAULT_OPTS: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric', year: 'numeric' }

/** Format a date-only (or ISO) string; returns `fallback` for empty/invalid. */
export function formatDateOnly(
  value?: string | null,
  opts: Intl.DateTimeFormatOptions = DEFAULT_OPTS,
  fallback = '—',
): string {
  if (!value) return fallback
  const parsed = parseBackendDate(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleDateString('en-US', opts)
}

/** Whole-year age on `asOf` for a date-of-birth string; '—' when missing or invalid. */
export function ageFromDob(dob?: string | null, asOf: Date = new Date()): string {
  if (!dob) return '—'
  const birth = parseBackendDate(dob)
  if (Number.isNaN(birth.getTime())) return '—'
  let age = asOf.getFullYear() - birth.getFullYear()
  const birthdayPassed = asOf.getMonth() > birth.getMonth() || (asOf.getMonth() === birth.getMonth() && asOf.getDate() >= birth.getDate())
  if (!birthdayPassed) age -= 1
  return `${age}`
}

/** <input type="datetime-local"> value ("2026-03-07T08:00") -> ISO timestamp in the viewer's zone; null when blank or invalid. */
export function localInputToISO(value: string): string | null {
  if (!value) return null
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? null : d.toISOString()
}

/** Format a start–end date range, collapsing to a single date when equal/absent. */
export function formatDateRange(start?: string | null, end?: string | null): string {
  if (!start) return '—'
  const startLabel = formatDateOnly(start)
  if (!end || end === start) return startLabel
  return `${startLabel} – ${formatDateOnly(end)}`
}

const DATE_TIME_OPTS: Intl.DateTimeFormatOptions = {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
}

/** Format an ISO timestamp for display (local time); '—' when missing. */
export function formatDateTime(value?: string | null): string {
  if (!value) return '—'
  return new Date(value).toLocaleString('en-US', DATE_TIME_OPTS)
}

/** "Jul 4" style label for a date-only or ISO string. */
export function formatShortDate(value: string): string {
  return parseBackendDate(value).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

/** "3:05 PM" for a "HH:MM[:SS]" clock string or an ISO timestamp; '' when missing, the input itself when unparseable. */
export function formatClockTime(value?: string | null): string {
  if (!value) return ''
  const plain = value.match(/^(\d{1,2}):(\d{2})(?::\d{2})?$/)
  const date = plain ? new Date(2000, 0, 1, Number(plain[1]), Number(plain[2])) : new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
}

/** Today's date (YYYY-MM-DD) in the viewer's timezone; `toISOString()` would give the UTC date, which is tomorrow for US evenings. */
export function localToday(now = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${now.getFullYear()}-${p(now.getMonth() + 1)}-${p(now.getDate())}`
}
