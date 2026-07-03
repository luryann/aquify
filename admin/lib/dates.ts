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

/** Format a start–end date range, collapsing to a single date when equal/absent. */
export function formatDateRange(start?: string | null, end?: string | null): string {
  if (!start) return '—'
  const startLabel = formatDateOnly(start)
  if (!end || end === start) return startLabel
  return `${startLabel} – ${formatDateOnly(end)}`
}
