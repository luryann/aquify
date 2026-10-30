'use client'

import { useState, useEffect, useRef, useCallback } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError } from '@/lib/frontend-telemetry'
import { fetchMeets, fetchPractices, fetchGroups, type MeetRecord, type PracticeRecord, type GroupRecord } from '@/lib/adminData'
import { fetchClubEvents, type ClubEventRecord } from '@/lib/engagementData'
import { clubEventDateKey, formatClubEventWhen } from '@/lib/club-events'
import { CalendarSubscribeCard } from '@/components/engagement/CalendarSubscribeCard'
import { fetchTrainingSchedule, type Workout } from '@/lib/trainingData'
import { addDaysISO } from '@/lib/workout-format'
import { WorkoutSheet } from '@/components/training/WorkoutSheet'
import './page.inline.css'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { Modal } from '@/components/ui/Modal'
import { localToday } from '@/lib/dates'

// ── Types ───────────────────────────────────────────────────────────────────
type EventType = 'meet' | 'practice' | 'deadline' | 'holiday' | 'event'

type CalEvent = {
  id: string
  title: string
  type: EventType
  date: string          // YYYY-MM-DD
  time?: string
  location?: string
  detail?: string
  linkPath?: string
  workout?: Workout
}

// ── Helpers ─────────────────────────────────────────────────────────────────
function dateOnly(iso?: string): string {
  if (!iso) return ''
  return iso.split('T')[0]
}

function deadlineCountdownText(iso: string): string {
  const days = Math.ceil((new Date(iso).getTime() - Date.now()) / 86400000)
  if (days <= 0) return 'Respond yes/no for each swimmer by tonight.'
  if (days === 1) return 'Respond yes/no for each swimmer — 1 day left.'
  return `Respond yes/no for each swimmer — ${days} days left.`
}

function meetsToCalEvents(meets: MeetRecord[]): CalEvent[] {
  const events: CalEvent[] = []
  for (const meet of meets) {
    if (meet.start_date) {
      events.push({
        id: `meet-${meet.id}`,
        title: meet.name,
        type: 'meet',
        date: dateOnly(meet.start_date),
        location: meet.location ?? undefined,
        detail: meet.host_org ? `Hosted by ${meet.host_org}` : undefined,
        linkPath: `/portal/meets/${meet.id}`,
      })
      if (meet.end_date && dateOnly(meet.end_date) !== dateOnly(meet.start_date)) {
        events.push({
          id: `meet-${meet.id}-end`,
          title: `${meet.name} — Day 2`,
          type: 'meet',
          date: dateOnly(meet.end_date),
          location: meet.location ?? undefined,
          linkPath: `/portal/meets/${meet.id}`,
        })
      }
    }
    if (meet.commitment_deadline && meet.status === 'commitment_open') {
      events.push({
        id: `deadline-${meet.id}`,
        title: `Commitment Deadline — ${meet.name}`,
        type: 'deadline',
        date: dateOnly(meet.commitment_deadline),
        detail: deadlineCountdownText(meet.commitment_deadline),
        linkPath: `/portal/meets/${meet.id}`,
      })
    }
  }
  return events
}


function practicesToCalEvents(practices: PracticeRecord[], groupNameById: Map<string, string>, workouts: Map<string, Workout>): CalEvent[] {
  return practices.map((p) => {
    const groupName = p.group_id ? groupNameById.get(p.group_id) : undefined
    const label = groupName ?? 'All groups'
    return {
      id: `practice-${p.id}`,
      title: p.description ? `Practice — ${label} — ${p.description.slice(0, 60)}` : `Practice — ${label}`,
      type: 'practice' as EventType,
      date: p.date,
      detail: p.yardage_target > 0 ? `${p.yardage_target.toLocaleString()} yds target` : undefined,
      workout: workouts.get(p.id),
    }
  })
}

function clubEventsToCalEvents(list: ClubEventRecord[]): CalEvent[] {
  return list.map((e) => ({
    id: `club-${e.id}`,
    title: e.status === 'cancelled' ? `Cancelled: ${e.title}` : e.title,
    type: 'event' as EventType,
    date: clubEventDateKey(e),
    time: formatClubEventWhen(e).split(', ').slice(2).join(', ') || undefined,
    location: e.location || undefined,
    detail: e.rsvp_enabled && e.status !== 'cancelled' ? (e.my_rsvps?.length ? 'You have replied. Change it on the Events page.' : 'RSVP on the Events page.') : undefined,
    linkPath: `/portal/events#event-${e.id}`,
  }))
}

// ── Config ─────────────────────────────────────────────────────────────────
const EVENT_STYLE: Record<EventType, { dot: string; bg: string; border: string; color: string; label: string }> = {
  meet:     { dot: 'var(--accent)', bg: 'var(--accent-subtle)', border: 'var(--accent-border)', color: 'var(--accent)', label: 'Meet' },
  practice: { dot: 'var(--info)', bg: 'var(--info-bg)', border: 'var(--info-border)', color: 'var(--info)', label: 'Practice' },
  deadline: { dot: 'var(--error)', bg: 'var(--error-bg)', border: 'var(--error-border)', color: 'var(--error)', label: 'Deadline' },
  holiday:  { dot: 'var(--text-muted)', bg: 'var(--bg-surface)', border: 'var(--border)', color: 'var(--text-muted)', label: 'Holiday' },
  event:    { dot: 'var(--warning)', bg: 'var(--warning-bg)', border: 'var(--warning-border)', color: 'var(--warning)', label: 'Event' },
}

// ── Calendar helpers ────────────────────────────────────────────────────────
function getDaysInMonth(year: number, month: number) {
  return new Date(year, month + 1, 0).getDate()
}

function getFirstDayOfWeek(year: number, month: number) {
  return new Date(year, month, 1).getDay() // 0=Sun
}

function toDateStr(year: number, month: number, day: number) {
  const m = String(month + 1).padStart(2, '0')
  const d = String(day).padStart(2, '0')
  return `${year}-${m}-${d}`
}

const MONTH_NAMES = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
]
const DAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

// ── Sub-components ──────────────────────────────────────────────────────────
function EventChip({ ev }: { ev: CalEvent }) {
  const s = EVENT_STYLE[ev.type]
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
        padding: '4px 8px',
        borderRadius: '6px',
        background: s.bg,
        border: `1px solid ${s.border}`,
        fontSize: 'var(--fs-xs)',
        fontWeight: 500,
        color: s.color,
        overflow: 'hidden',
        whiteSpace: 'nowrap',
        textOverflow: 'ellipsis',
        cursor: 'default',
      }}
    >
      <span
        style={{
          width: '6px',
          height: '6px',
          borderRadius: '50%',
          background: s.dot,
          flexShrink: 0,
        }}
      />
      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
        {ev.title}
      </span>
    </div>
  )
}

const UPCOMING_MAX = 15

function NavArrowButton({ direction, onClick }: { direction: 'prev' | 'next'; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} aria-label={direction === 'prev' ? 'Previous month' : 'Next month'} className="cal-nav-btn">
      <Icon name={direction === 'prev' ? 'chevron-left' : 'chevron-right'} size={15} />
    </button>
  )
}

// Rows stay two lines at most: long practice titles clamp instead of stacking
// one word per line in the narrow side column.
const clampLines = (lines: number): React.CSSProperties => ({
  display: '-webkit-box',
  WebkitLineClamp: lines,
  WebkitBoxOrient: 'vertical',
  overflow: 'hidden',
})

function upcomingDateLabel(date: string): { day: string; weekday: string } {
  const [y, m, d] = date.split('-').map(Number)
  const local = new Date(y, (m ?? 1) - 1, d ?? 1)
  return {
    day: local.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
    weekday: local.toLocaleDateString('en-US', { weekday: 'short' }),
  }
}

function UpcomingEventRow({ ev }: { ev: CalEvent }) {
  const s = EVENT_STYLE[ev.type]
  const when = upcomingDateLabel(ev.date)
  const meta = [ev.time, ev.location].filter(Boolean).join(' · ')
  const content = (
    <div
      title={ev.title}
      className="is-955ade5 upcoming-row"
    >
      <div className="is-508b578">
        <span className="is-2dd2301">{when.day}</span>
        <span className="is-c454211">{when.weekday}</span>
      </div>
      <div style={{ minWidth: 0 }}>
        <p className="is-fdeb8cd">
          <span aria-label={s.label} style={{ width: '8px', height: '8px', borderRadius: '50%', background: s.dot, flexShrink: 0, transform: 'translateY(-1px)' }} />
          <span style={clampLines(2)}>{ev.title}</span>
        </p>
        {meta && (
          <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-subtle)', marginTop: '2px', ...clampLines(1) }}>{meta}</p>
        )}
        {ev.detail && (
          <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', marginTop: '2px', ...clampLines(1) }}>{ev.detail}</p>
        )}
      </div>
    </div>
  )

  if (ev.linkPath) {
    return <Link href={ev.linkPath} style={{ textDecoration: 'none', display: 'block' }}>{content}</Link>
  }
  return content
}

// Fixed-height list: scrolls inside the card so the sidebar never grows the page.
// The bottom fade hints at more rows and disappears once the end is reached.
function UpcomingScroller({ events }: { events: CalEvent[] }) {
  const ref = useRef<HTMLDivElement>(null)
  const [atEnd, setAtEnd] = useState(true)
  const sync = useCallback(() => {
    const el = ref.current
    if (el) setAtEnd(el.scrollHeight - el.scrollTop - el.clientHeight < 4)
  }, [])
  useEffect(sync, [events, sync])
  return (
    <div
      ref={ref}
      onScroll={sync}
      className="upcoming-scroll"
      data-at-end={atEnd}
      tabIndex={0}
      role="region"
      aria-label="Upcoming events"
    >
      {events.map((ev) => (
        <UpcomingEventRow key={ev.id} ev={ev} />
      ))}
    </div>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function CalendarPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [events, setEvents] = useState<CalEvent[]>([])
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [clubCalEvents, setClubCalEvents] = useState<CalEvent[]>([])
  const [clubError, setClubError] = useState<string | null>(null)

  // Club events load independently so a failure there never blanks meets/practices.
  useEffect(() => {
    if (!orgId) return
    setClubError(null)
    fetchClubEvents(api, orgId)
      .then((list) => setClubCalEvents(clubEventsToCalEvents(list)))
      .catch((error) => {
        setClubCalEvents([])
        setClubError(reportUIError('portal-calendar-club-events', error, 'Club events could not be loaded.', { orgId }))
      })
  }, [api, orgId, reloadKey])

  useEffect(() => {
    if (!orgId) {
      setLoadError('We could not find your club. Sign out and back in to try again.')
      return
    }
    setLoadError(null)
    Promise.all([
      fetchMeets(api, orgId),
      fetchPractices(api, orgId),
      fetchGroups(api, orgId),
      // Structured sets for nearby practices; the calendar still renders if unavailable.
      fetchTrainingSchedule(api, orgId, addDaysISO(localToday(), -30), addDaysISO(localToday(), 90)).catch(() => []),
    ])
      .then(([meets, practices, groups, scheduleItems]) => {
        const groupNameById = new Map<string, string>(groups.map((g: GroupRecord) => [g.id, g.name]))
        const workoutByPractice = new Map<string, Workout>()
        for (const item of scheduleItems) if (item.workout) workoutByPractice.set(item.practice.id, item.workout)
        setEvents([...meetsToCalEvents(meets), ...practicesToCalEvents(practices, groupNameById, workoutByPractice)])
      })
      .catch((error) => {
        setEvents([])
        setLoadError(reportUIError(
          'portal-calendar-load',
          error,
          'We could not load this page. Check your connection and try again.',
          { orgId },
        ))
      })
  }, [api, orgId, reloadKey])

  const today = new Date()
  const [viewYear, setViewYear] = useState(today.getFullYear())
  const [viewMonth, setViewMonth] = useState(today.getMonth())
  const [selectedDate, setSelectedDate] = useState<string | null>(null)
  const [view, setView] = useState<'month' | 'week'>('month')
  const [shareOpen, setShareOpen] = useState(false)
  const [typeFilter, setTypeFilter] = useState<EventType | 'all'>('all')

  // Responsive: track viewport width to reflow sidebar below calendar on mobile.
  // Start with desktop-default (1024) to avoid SSR/hydration mismatch; update
  // after mount. A brief layout switch on first paint is acceptable and expected.
  const [windowWidth, setWindowWidth] = useState<number>(1024)
  useEffect(() => {
    setWindowWidth(window.innerWidth)
    const handleResize = () => setWindowWidth(window.innerWidth)
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])
  const isMobile = windowWidth < 720

  const daysInMonth = getDaysInMonth(viewYear, viewMonth)
  const firstDow = getFirstDayOfWeek(viewYear, viewMonth)

  // Events indexed by date string
  const allEvents = [...events, ...clubCalEvents]
  const eventsByDate = allEvents.reduce<Record<string, CalEvent[]>>((acc, ev) => {
    if (!acc[ev.date]) acc[ev.date] = []
    acc[ev.date].push(ev)
    return acc
  }, {})

  const goToPrev = () => {
    if (viewMonth === 0) { setViewYear(y => y - 1); setViewMonth(11) }
    else setViewMonth(m => m - 1)
  }
  const goToNext = () => {
    if (viewMonth === 11) { setViewYear(y => y + 1); setViewMonth(0) }
    else setViewMonth(m => m + 1)
  }
  const goToToday = () => {
    setViewYear(today.getFullYear())
    setViewMonth(today.getMonth())
    setSelectedDate(toDateStr(today.getFullYear(), today.getMonth(), today.getDate()))
  }

  const todayStr = toDateStr(today.getFullYear(), today.getMonth(), today.getDate())

  // Upcoming events from today
  const upcomingEvents = allEvents
    .filter((ev) => ev.date >= todayStr)
    .filter((ev) => typeFilter === 'all' || ev.type === typeFilter)
    .sort((a, b) => a.date.localeCompare(b.date))
    .slice(0, UPCOMING_MAX)

  const visibleEventsOn = (dateStr: string) =>
    (eventsByDate[dateStr] ?? []).filter((ev) => typeFilter === 'all' || ev.type === typeFilter)

  const selectedEvents = selectedDate ? visibleEventsOn(selectedDate) : []

  // Week view helpers — derive weekStart from selectedDate if available,
  // otherwise anchor to the first week of the currently viewed month.
  // This ensures month navigation + week view stay in sync.
  const weekStart = (() => {
    if (selectedDate) {
      const d = new Date(selectedDate + 'T12:00:00')
      d.setDate(d.getDate() - d.getDay())
      return d
    }
    // Default: Sunday of the week containing the 1st of viewMonth
    const d = new Date(viewYear, viewMonth, 1)
    d.setDate(d.getDate() - d.getDay())
    return d
  })()

  const weekDays = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(weekStart)
    d.setDate(weekStart.getDate() + i)
    return {
      date: toDateStr(d.getFullYear(), d.getMonth(), d.getDate()),
      label: DAY_LABELS[d.getDay()],
      num: d.getDate(),
      isToday: toDateStr(d.getFullYear(), d.getMonth(), d.getDate()) === todayStr,
    }
  })

  return (
    <div>
      {/* Page header */}
      <div
        className="is-f7be90b"
      >
        <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
          <h1 className="page-title">
            Calendar
          </h1>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setShareOpen(true)}>
            <Icon name="calendar" size={13} /> Share
          </button>
        </div>

        {/* View toggle */}
        <div
          className="is-ec25a69"
        >
          {(['month', 'week'] as const).map((v) => (
            <button type="button"
              key={v}
              onClick={() => setView(v)}
              style={{
                padding: '4px 16px',
                borderRadius: '6px',
                border: 'none',
                background: view === v ? 'var(--bg-elevated)' : 'transparent',
                color: view === v ? 'var(--text)' : 'var(--text-subtle)',
                fontSize: 'var(--fs-sm)',
                fontWeight: view === v ? 600 : 400,
                cursor: 'pointer',
                boxShadow: view === v ? '0 1px 3px rgba(0,0,0,0.08)' : 'none',
                transition: 'all 120ms ease',
                textTransform: 'capitalize',
              }}
            >
              {v}
            </button>
          ))}
        </div>
      </div>

      {clubError && !loadError && (
        <p role="alert" className="is-bf585c4">
          {clubError}{' '}
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setReloadKey((key) => key + 1)}>Retry</button>
        </p>
      )}

      {loadError && (
        <LoadErrorBanner message={loadError} onRetry={() => setReloadKey((key) => key + 1)} />
      )}

      {/* Legend + type filter */}
      <div
        className="is-63beafb"
      >
        {(['all', 'meet', 'practice', 'deadline', 'event', 'holiday'] as const).map((type) => {
          const active = typeFilter === type
          const s = type !== 'all' ? EVENT_STYLE[type] : null
          return (
            <button type="button"
              key={type}
              onClick={() => setTypeFilter(type)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                padding: '4px 12px',
                borderRadius: '999px',
                border: `1px solid ${active ? (s ? s.border : 'var(--border-strong)') : 'var(--border)'}`,
                background: active ? (s ? s.bg : 'var(--bg-wash-strong)') : 'transparent',
                color: active ? (s ? s.color : 'var(--text)') : 'var(--text-subtle)',
                fontSize: 'var(--fs-sm)',
                fontWeight: 500,
                cursor: 'pointer',
                transition: 'all 120ms ease',
              }}
            >
              {s && (
                <span
                  style={{
                    width: '7px',
                    height: '7px',
                    borderRadius: '50%',
                    background: s.dot,
                    flexShrink: 0,
                  }}
                />
              )}
              <span style={{ textTransform: 'capitalize' }}>
                {type === 'all' ? 'All types' : EVENT_STYLE[type as EventType].label}
              </span>
            </button>
          )
        })}
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: isMobile ? '1fr' : 'minmax(0, 1fr) minmax(240px, 280px)',
          gap: '20px',
          animation: 'slideUp 0.4s var(--ease-out) 0.1s both',
          alignItems: 'start',
        }}
      >
        {/* ── MAIN CALENDAR ─────────────────────────────────────────── */}
        <div
          className="card" style={{ overflowX: 'auto', overflowY: 'hidden', minWidth: 0 }}
        >
          {/* Month nav */}
          <div
            className="is-044825a"
          >
            <NavArrowButton direction="prev" onClick={goToPrev} />

            <div className="is-e2ff0fd">
              <h2 className="section-title"
              >
                {MONTH_NAMES[viewMonth]} {viewYear}
              </h2>
              <button type="button"
                onClick={goToToday}
                className="is-68b99f7 cal-today-btn"
              >
                Today
              </button>
            </div>

            <NavArrowButton direction="next" onClick={goToNext} />
          </div>

          {isMobile ? (
            /* ── MOBILE AGENDA VIEW ─────────────────────────────────── */
            /* The 7-column grid can't reflow to a narrow screen without
               either truncating every cell or forcing horizontal scroll —
               a stacked day-by-day list is the usable mobile equivalent. */
            <div style={{ display: 'flex', flexDirection: 'column' }}>
              {(view === 'month'
                ? Array.from({ length: daysInMonth }, (_, i) => toDateStr(viewYear, viewMonth, i + 1))
                : weekDays.map((wd) => wd.date)
              ).map((dateStr) => {
                const dayEvents = visibleEventsOn(dateStr)
                if (view === 'month' && dayEvents.length === 0) return null
                const isToday = dateStr === todayStr
                const isSelected = dateStr === selectedDate
                const d = new Date(dateStr + 'T12:00:00')

                return (
                  <div
                    key={dateStr}
                    onClick={() => setSelectedDate(dateStr === selectedDate ? null : dateStr)}
                    style={{
                      padding: '12px 16px',
                      borderBottom: '1px solid var(--border)',
                      cursor: 'pointer',
                      background: isSelected ? 'rgba(235,93,30,0.04)' : 'transparent',
                      display: 'flex',
                      gap: '12px',
                      alignItems: 'flex-start',
                    }}
                  >
                    <div
                      style={{
                        width: '34px',
                        height: '34px',
                        borderRadius: '50%',
                        flexShrink: 0,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        fontSize: 'var(--fs-base)',
                        fontWeight: isToday ? 700 : 400,
                        color: isToday ? 'var(--bg-elevated)' : isSelected ? 'var(--accent)' : 'var(--text)',
                        background: isToday ? 'var(--accent)' : 'transparent',
                      }}
                    >
                      {d.getDate()}
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <p className="is-f45a7a7">
                        {d.toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric' })}
                      </p>
                      {dayEvents.length === 0 ? (
                        <span className="is-4f39ab0">No events</span>
                      ) : (
                        <div className="is-ef1f3a5">
                          {dayEvents.map((ev) => (
                            <EventChip key={ev.id} ev={ev} />
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          ) : view === 'month' ? (
            <>
              {/* Day-of-week headers */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(7, minmax(0, 1fr))',
                  borderBottom: '1px solid var(--border)',
                  minWidth: '560px',
                }}
              >
                {DAY_LABELS.map((d) => (
                  <div
                    key={d}
                    className="is-b587c69"
                  >
                    {d}
                  </div>
                ))}
              </div>

              {/* Day grid */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(7, minmax(0, 1fr))',
                  minWidth: '560px',
                }}
              >
                {/* Empty cells before first day */}
                {Array.from({ length: firstDow }, (_, i) => (
                  <div
                    key={`empty-${i}`}
                    style={{
                      minHeight: '172px',
                      borderRight: '1px solid var(--border)',
                      borderBottom: '1px solid var(--border)',
                      background: 'var(--bg-wash)',
                    }}
                  />
                ))}

                {/* Day cells */}
                {Array.from({ length: daysInMonth }, (_, i) => {
                  const day = i + 1
                  const dateStr = toDateStr(viewYear, viewMonth, day)
                  const dayEvents = visibleEventsOn(dateStr)
                  const isToday = dateStr === todayStr
                  const isSelected = dateStr === selectedDate
                  const colIndex = (firstDow + i) % 7

                  return (
                    <div
                      key={day}
                      onClick={() => setSelectedDate(dateStr === selectedDate ? null : dateStr)}
                      className="cal-day"
                      data-selected={isSelected}
                      style={{
                        minHeight: '172px',
                        borderRight: colIndex < 6 ? '1px solid var(--border)' : 'none',
                        borderBottom: '1px solid var(--border)',
                        padding: '12px 8px 8px',
                        position: 'relative',
                      }}
                    >
                      {/* Day number */}
                      <div
                        style={{
                          width: '26px',
                          height: '26px',
                          borderRadius: '50%',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontSize: 'var(--fs-base)',
                          fontWeight: isToday ? 700 : 400,
                          color: isToday ? 'var(--bg-elevated)' : isSelected ? 'var(--accent)' : 'var(--text)',
                          background: isToday ? 'var(--accent)' : 'transparent',
                          marginBottom: '6px',
                        }}
                      >
                        {day}
                      </div>

                      {/* Event dots / chips */}
                      <div className="is-32bee05">
                        {dayEvents.slice(0, 3).map((ev) => (
                          <EventChip key={ev.id} ev={ev} />
                        ))}
                        {dayEvents.length > 3 && (
                          <span
                            className="is-446b8e8"
                          >
                            +{dayEvents.length - 3} more
                          </span>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            </>
          ) : (
            /* ── WEEK VIEW ──────────────────────────────────────────── */
            <>
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(7, minmax(0, 1fr))',
                  borderBottom: '1px solid var(--border)',
                  minWidth: '560px',
                }}
              >
                {weekDays.map((wd) => (
                  <div
                    key={wd.date}
                    className="is-04e0c56"
                  >
                    <p
                      className="is-98a7b6f"
                    >
                      {wd.label}
                    </p>
                    <div
                      style={{
                        width: '30px',
                        height: '30px',
                        borderRadius: '50%',
                        background: wd.isToday ? 'var(--accent)' : 'transparent',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        margin: '0 auto',
                        fontSize: 'var(--fs-md)',
                        fontWeight: wd.isToday ? 700 : 400,
                        color: wd.isToday ? 'var(--bg-elevated)' : 'var(--text)',
                      }}
                    >
                      {wd.num}
                    </div>
                  </div>
                ))}
              </div>

              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(7, minmax(0, 1fr))',
                  minHeight: '340px',
                  minWidth: '560px',
                }}
              >
                {weekDays.map((wd, idx) => {
                  const dayEvents = visibleEventsOn(wd.date)
                  return (
                    <div
                      key={wd.date}
                      style={{
                        padding: '12px 6px',
                        borderRight: idx < 6 ? '1px solid var(--border)' : 'none',
                        display: 'flex',
                        flexDirection: 'column',
                        gap: '4px',
                        background: wd.isToday ? 'rgba(235,93,30,0.03)' : 'transparent',
                      }}
                    >
                      {dayEvents.map((ev) => (
                        <EventChip key={ev.id} ev={ev} />
                      ))}
                      {dayEvents.length === 0 && (
                        <span className="is-5bb4634">
                          —
                        </span>
                      )}
                    </div>
                  )
                })}
              </div>
            </>
          )}
        </div>

        {/* Calendar subscription lives behind the Share button so it is not always on screen */}
        <Modal open={shareOpen} onClose={() => setShareOpen(false)} title="Add to your phone’s calendar">
          <CalendarSubscribeCard api={api} orgId={orgId} />
        </Modal>

        {/* ── SIDEBAR ──────────────────────────────────────────────── */}
        <div className="is-8ef476f">
          {/* Selected day detail */}
          {selectedDate && selectedEvents.length > 0 && (
            <div
              className="card" style={{ overflow: 'hidden', animation: 'slideUp 0.2s var(--ease-out) both' }}
            >
              <div
                className="is-703aadc"
              >
                <p
                  className="is-f3c1003"
                >
                  {new Date(selectedDate + 'T12:00:00').toLocaleDateString('en-US', {
                    weekday: 'long', month: 'long', day: 'numeric',
                  })}
                </p>
              </div>
              {selectedEvents.map((ev) => (
                <div key={ev.id}>
                  <UpcomingEventRow ev={ev} />
                  {ev.workout && (
                    <div className="is-4810ca8">
                      <WorkoutSheet workout={ev.workout} />
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Upcoming events */}
          <div
            className="card is-f27afbb" 
          >
            <div className="is-703aadc">
              <h2 className="card-title">Upcoming</h2>
            </div>

            {loadError ? null : shouldShowCollectionEmptyState(false, loadError, upcomingEvents.length) ? (
              <div className="is-9d9263e">
                <p className="is-8fb176a">No upcoming events</p>
              </div>
            ) : (
              <UpcomingScroller events={upcomingEvents} />
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
