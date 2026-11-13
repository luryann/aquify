'use client'

import { Suspense, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useParams, useSearchParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { formatDateOnly, formatDeadline, parseBackendDate } from '@/lib/dates'
import { formatSwimTime } from '@/lib/performanceData'
import { useApiClient } from '@/lib/api'
import { ReportsPanel } from './ReportsPanel'
import { EventsBySession } from './EventsBySession'
import { AddMeetEventForm } from './AddMeetEventForm'
import { EntryFileExport } from './EntryFileExport'
import { ResultsImport } from './ResultsImport'
import { MeetVolunteersPanel } from '@/components/volunteer/MeetVolunteersPanel'
import { formatAgeGroup, formatStroke } from '@/lib/meet-events'
import {
  fetchHouseholds,
  fetchMeet,
  fetchMeetCommitments,
  fetchMeetEntries,
  fetchMeetEvents,
  fetchMeetSessions,
  fetchSwimmers,
  lockMeetCommitments,
  openMeetCommitments,
  transitionMeet,
  type MeetRecord,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { useConfirm } from '@/components/ui/ConfirmDialog'
import { PageSkeleton } from '@/components/ui/PageSkeleton'
import './meet-detail.css'

type SessionRow = {
  id: string
  name: string
  date: string
  warmupTime: string
  startTime: string
  eventCount: number
  number: number
}

type EventRow = {
  id: string
  sessionId: string
  eventNumber: string
  name: string
  course: string
  gender: string
  ageGroup: string
  entries: number
  sessionName: string
}

type CommitmentRow = {
  id: string
  swimmer: string
  household: string
  status: string
}

type EntryRow = {
  id: string
  swimmer: string
  event: string
  seedTime: string
  status: string
}

function formatTime(value?: string | null) {
  if (!value) return '—'
  // Bare clock times ("13:55" or "13:55:00") come from time-only fields.
  const clock = /^(\d{1,2}):(\d{2})(?::\d{2})?$/.exec(value.trim())
  if (clock) {
    const h = Number(clock[1])
    return `${h % 12 || 12}:${clock[2]} ${h < 12 ? 'AM' : 'PM'}`
  }
  const parsed = parseBackendDate(value)
  if (!Number.isNaN(parsed.getTime())) {
    return parsed.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
  }
  return value
}

function countBy<T>(items: T[], key: (item: T) => string | null | undefined): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const item of items) {
    const k = key(item)
    if (k) counts[k] = (counts[k] ?? 0) + 1
  }
  return counts
}

function StatusBadge({ status }: { status: string }) {
  const cfg: Record<string, { label: string; color: string; bg: string }> = {
    draft: { label: 'Draft', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    published: { label: 'Published', color: 'var(--info)', bg: 'var(--info-bg)' },
    commitment_open: { label: 'Commitments open', color: 'var(--warning)', bg: 'var(--warning-bg)' },
    commitment_closed: { label: 'Commitments closed', color: 'var(--warning)', bg: 'var(--warning-bg)' },
    entry_finalized: { label: 'Entries finalized', color: 'var(--accent-text, var(--accent))', bg: 'var(--accent-subtle)' },
    completed: { label: 'Completed', color: 'var(--success)', bg: 'var(--success-bg)' },
    archived: { label: 'Archived', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    invited: { label: 'Invited', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    committed_yes: { label: 'Committed yes', color: 'var(--success)', bg: 'var(--success-bg)' },
    committed_no: { label: 'Committed no', color: 'var(--error)', bg: 'var(--error-bg)' },
    locked: { label: 'Locked', color: 'var(--accent-text, var(--accent))', bg: 'var(--accent-subtle)' },
    submitted: { label: 'Submitted', color: 'var(--info)', bg: 'var(--info-bg)' },
    approved: { label: 'Approved', color: 'var(--success)', bg: 'var(--success-bg)' },
  }
  const colors = cfg[status] ?? { label: status, color: 'var(--text-muted)', bg: 'var(--bg-elevated)' }
  return <span className="badge" style={{ background: colors.bg, color: colors.color }}>{colors.label}</span>
}

const SESSION_COLS: Column<SessionRow>[] = [
  { key: 'name', label: 'Session', render: (row) => <span className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{row.name}</span> },
  { key: 'date', label: 'Date', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{formatDateOnly(row.date)}</span> },
  { key: 'warmupTime', label: 'Warm-up', render: (row) => <span className="u-fs-fs-sm plain-digits" style={{ color: 'var(--text-muted)' }}>{formatTime(row.warmupTime)}</span> },
  { key: 'startTime', label: 'Start', render: (row) => <span className="u-fs-fs-sm plain-digits" style={{ color: 'var(--text)' }}>{formatTime(row.startTime)}</span> },
  { key: 'eventCount', label: 'Events', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text)' }}>{row.eventCount}</span> },
]

const COMMITMENT_COLS: Column<CommitmentRow>[] = [
  { key: 'swimmer', label: 'Swimmer', render: (row) => <span className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{row.swimmer}</span> },
  { key: 'household', label: 'Household', render: (row) => <span className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{row.household}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
]

const ENTRY_COLS: Column<EntryRow>[] = [
  { key: 'swimmer', label: 'Swimmer', render: (row) => <span className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{row.swimmer}</span> },
  { key: 'event', label: 'Event', render: (row) => <span className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{row.event}</span> },
  { key: 'seedTime', label: 'Seed', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--accent-text, var(--accent))' }}>{row.seedTime}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
]

// ─── Lifecycle ────────────────────────────────────────────────────────────────

const LIFECYCLE_STAGES = [
  { key: 'draft', label: 'Draft' },
  { key: 'published', label: 'Published' },
  { key: 'commitment_open', label: 'Commitments open' },
  { key: 'commitment_closed', label: 'Commitments closed' },
  { key: 'entry_finalized', label: 'Entries finalized' },
  { key: 'completed', label: 'Completed' },
]

type LifecycleAction = {
  label: string
  toStatus: string
  variant: 'primary' | 'danger' | 'ghost'
  confirm?: string
}

function getLifecycleActions(status: string): LifecycleAction[] {
  switch (status) {
    case 'draft':
      return [{ label: 'Publish meet', toStatus: 'published', variant: 'primary' }]
    case 'published':
      return [
        { label: 'Open commitments', toStatus: 'commitment_open', variant: 'primary' },
        { label: 'Revert to draft', toStatus: 'draft', variant: 'danger', confirm: 'Revert this meet back to draft? It will no longer be visible to swimmers.' },
      ]
    case 'commitment_open':
      return [{ label: 'Close commitments', toStatus: 'commitment_closed', variant: 'primary', confirm: 'Close commitments? Families will no longer be able to respond, and event selections for swimmers who are going become entries for coach review.' }]
    case 'commitment_closed':
      return [{ label: 'Finalize entries', toStatus: 'entry_finalized', variant: 'primary' }]
    case 'entry_finalized':
      return [{ label: 'Mark complete', toStatus: 'completed', variant: 'primary', confirm: 'Mark this meet as completed?' }]
    case 'completed':
      return [{ label: 'Archive', toStatus: 'archived', variant: 'ghost', confirm: 'Archive this meet? This cannot be undone.' }]
    default:
      return []
  }
}

function LifecycleActions({ status, onTransition, transitioning }: { status: string; onTransition: (toStatus: string) => void; transitioning: boolean }) {
  const confirm = useConfirm()
  const actions = getLifecycleActions(status)
  if (actions.length === 0) return null
  return (
    <div className="md-actions">
      {[...actions].reverse().map((action) => (
        <button
          type="button"
          key={action.toStatus}
          disabled={transitioning}
          className={action.variant === 'primary' ? 'btn btn-primary' : action.variant === 'danger' ? 'btn btn-ghost' : 'btn btn-secondary'}
          style={action.variant === 'danger' ? { color: 'var(--error)' } : undefined}
          onClick={async () => {
            if (action.confirm) {
              const ok = await confirm({ title: `${action.label}?`, message: action.confirm, confirmLabel: action.label, destructive: action.variant === 'danger' })
              if (!ok) return
            }
            onTransition(action.toStatus)
          }}
        >
          {transitioning && action.variant === 'primary' ? <span className="spinner spinner-sm" aria-hidden /> : null}
          {action.label}
        </button>
      ))}
    </div>
  )
}

function StageSteps({ status }: { status: string }) {
  const current = LIFECYCLE_STAGES.findIndex((s) => s.key === status)
  if (current < 0) return null // archived
  return (
    <ol className="md-steps" aria-label="Meet stage">
      {LIFECYCLE_STAGES.map((stage, i) => (
        <li key={stage.key} className="md-step" data-state={i < current ? 'done' : i === current ? 'current' : 'todo'} aria-current={i === current ? 'step' : undefined}>
          {stage.label}
        </li>
      ))}
    </ol>
  )
}

function StackBar({ parts }: { parts: { label: string; value: number; color: string }[] }) {
  const total = parts.reduce((n, p) => n + p.value, 0)
  if (total === 0) return null
  return (
    <>
      <div className="md-bar" role="img" aria-label={parts.map((p) => `${p.value} ${p.label}`).join(', ')}>
        {parts.filter((p) => p.value > 0).map((p) => <span key={p.label} style={{ flex: p.value, background: p.color }} />)}
      </div>
      <div className="md-legend">
        {parts.map((p) => <span key={p.label}><i style={{ background: p.color }} /><b>{p.value}</b>{p.label}</span>)}
      </div>
    </>
  )
}


// Events can be added until entries are finalized; after that the event list
// is what was exported to the host and must not drift.
const EDITABLE_EVENT_STATUSES = new Set(['draft', 'published', 'commitment_open', 'commitment_closed'])

function nextEventNumber(events: EventRow[]): string {
  const max = events.reduce((acc, event) => {
    const n = parseInt(event.eventNumber, 10)
    return Number.isFinite(n) && n > acc ? n : acc
  }, 0)
  return String(max + 1)
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function MeetDetailPage() {
  return (
    <Suspense fallback={null}>
      <MeetDetailPageContent />
    </Suspense>
  )
}

function MeetDetailPageContent() {
  const params = useParams<{ id: string }>()
  const meetId = typeof params?.id === 'string' ? params.id : ''
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const searchParams = useSearchParams()
  const [activeTab, setActiveTab] = useState(searchParams.get('tab') ?? 'overview')
  const [meet, setMeet] = useState<MeetRecord | null>(null)
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [events, setEvents] = useState<EventRow[]>([])
  const [commitments, setCommitments] = useState<CommitmentRow[]>([])
  const [entries, setEntries] = useState<EntryRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [transitioning, setTransitioning] = useState(false)
  const [transitionError, setTransitionError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [showAddEvent, setShowAddEvent] = useState(false)

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!session?.user.orgId || !meetId) {
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        setError(null)
        const [nextMeet, rawSessions, rawEvents, rawCommitments, rawEntries, swimmers, households] = await Promise.all([
          fetchMeet(api, session.user.orgId, meetId),
          fetchMeetSessions(api, session.user.orgId, meetId),
          fetchMeetEvents(api, session.user.orgId, meetId),
          fetchMeetCommitments(api, session.user.orgId, meetId),
          fetchMeetEntries(api, session.user.orgId, meetId),
          fetchSwimmers(api, session.user.orgId),
          fetchHouseholds(api, session.user.orgId),
        ])

        if (cancelled) return

        const swimmerById = new Map(swimmers.map((swimmer) => [swimmer.id, swimmer]))
        const householdById = new Map(households.map((household) => [household.id, household.household_name]))
        const sessionById = new Map(rawSessions.map((session) => [session.id, session]))
        const eventById = new Map(rawEvents.map((event) => [event.id, event]))
        const swimmerName = (swimmerId: string) => {
          const swimmer = swimmerById.get(swimmerId)
          return swimmer ? `${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim() : swimmerId
        }
        const eventCountBySession = countBy(rawEvents, (event) => event.session_id)
        const entryCountByEvent = countBy(rawEntries, (entry) => entry.meet_event_id)

        setMeet(nextMeet)
        setSessions(rawSessions.map((meetSession) => ({
          id: meetSession.id,
          name: meetSession.name,
          date: meetSession.session_date || meetSession.scheduled_date || '',
          warmupTime: meetSession.warmup_time || meetSession.warmup_at || '',
          startTime: meetSession.start_time || meetSession.start_at || '',
          eventCount: eventCountBySession[meetSession.id] ?? 0,
          number: meetSession.session_number ?? meetSession.session_order ?? 0,
        })).sort((a, b) => a.number - b.number || a.date.localeCompare(b.date) || a.startTime.localeCompare(b.startTime)))
        setEvents(rawEvents.map((event) => ({
          id: event.id,
          sessionId: event.session_id || '',
          eventNumber: event.event_number?.toString() || '—',
          name: [event.distance, formatStroke(event.stroke)].filter(Boolean).join(' ') || 'Meet event',
          course: event.course || nextMeet.course,
          gender: event.gender || 'X',
          ageGroup: formatAgeGroup(event.min_age, event.max_age, event.age_group),
          entries: entryCountByEvent[event.id] ?? 0,
          sessionName: sessionById.get(event.session_id || '')?.name || 'Unscheduled',
        })).sort((a, b) => a.eventNumber.localeCompare(b.eventNumber, undefined, { numeric: true })))
        setCommitments(rawCommitments.map((commitment) => {
          const swimmer = swimmerById.get(commitment.swimmer_id)
          return {
            id: commitment.id,
            swimmer: swimmerName(commitment.swimmer_id),
            household: swimmer ? (householdById.get(swimmer.household_id) || 'Unknown household') : 'Unknown household',
            status: commitment.status,
          }
        }).sort((a, b) => a.swimmer.localeCompare(b.swimmer)))
        setEntries(rawEntries.map((entry) => {
          const event = eventById.get(entry.meet_event_id ?? '')
          return {
            id: entry.id,
            swimmer: swimmerName(entry.swimmer_id),
            event: event ? `${event.distance || ''} ${formatStroke(event.stroke)} ${event.gender || ''} ${formatAgeGroup(event.min_age, event.max_age, event.age_group)}`.trim() : 'Meet event',
            seedTime: formatSwimTime(entry.seed_time_ms),
            status: entry.status,
          }
        }).sort((a, b) => a.swimmer.localeCompare(b.swimmer)))
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load meet details. Try again.'))
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, meetId, session?.user.orgId, reloadKey])

  const confirm = useConfirm()
  async function handleTransition(toStatus: string) {
    if (!session?.user.orgId || !meetId) return
    if (toStatus === 'entry_finalized') {
      const pending = entries.filter((entry) => entry.status === 'submitted').length
      if (pending > 0) {
        const ok = await confirm({ title: 'Finalize with pending entries?', message: 'Entries still awaiting approval will not be in the SD3 entry file.', count: pending, countNoun: 'pending entr', confirmLabel: 'Finalize anyway' })
        if (!ok) return
      }
    }
    setTransitioning(true)
    setTransitionError(null)
    try {
      let updated: MeetRecord
      if (toStatus === 'commitment_open') {
        updated = await openMeetCommitments(api, session.user.orgId, meetId)
      } else if (toStatus === 'commitment_closed') {
        await lockMeetCommitments(api, session.user.orgId, meetId)
        updated = await fetchMeet(api, session.user.orgId, meetId)
      } else {
        updated = await transitionMeet(api, session.user.orgId, meetId, toStatus)
      }
      setMeet(updated)
      // Opening/closing commitments creates or locks commitment records, so
      // refetch the tabs rather than showing stale counts.
      setReloadKey((k) => k + 1)
    } catch (err) {
      setTransitionError(uiErrorMessage(err, 'Transition failed.'))
    } finally {
      setTransitioning(false)
    }
  }

  const tabs = useMemo(() => [
    { id: 'overview', label: 'Overview' },
    { id: 'sessions', label: 'Sessions', count: sessions.length },
    { id: 'events', label: 'Events', count: events.length },
    { id: 'commitments', label: 'Commitments', count: commitments.length },
    { id: 'entries', label: 'Entries', count: entries.length },
    { id: 'volunteers', label: 'Volunteers' },
    { id: 'reports', label: 'Reports' },
  ], [commitments.length, entries.length, events.length, sessions.length])

  if (loading) {
    return <PageSkeleton variant="detail" />
  }

  if (error || !meet) {
    return <div className="page"><EmptyState icon="alert-circle" title="Unable to load meet" description={error ?? 'Meet not found.'} /></div>
  }

  const invited = commitments.filter((c) => c.status === 'invited').length
  const yes = commitments.filter((c) => c.status === 'committed_yes' || c.status === 'coach_reviewed' || c.status === 'locked').length
  const no = commitments.filter((c) => c.status === 'committed_no').length
  const pendingEntries = entries.filter((e) => e.status === 'submitted').length
  const approvedEntries = entries.filter((e) => e.status === 'approved').length
  const exportedEntries = entries.filter((e) => e.status === 'exported').length
  const resultsOpen = meet.status === 'entry_finalized' || meet.status === 'completed' || meet.status === 'archived'

  const todo: { count?: number; text: string; tab: string }[] = []
  if (events.length === 0 && EDITABLE_EVENT_STATUSES.has(meet.status)) todo.push({ text: 'Add events so families can sign up', tab: 'events' })
  if (meet.status === 'commitment_open' && invited > 0) todo.push({ count: invited, text: `${invited === 1 ? 'swimmer has' : 'swimmers have'} not responded`, tab: 'commitments' })
  if (pendingEntries > 0) todo.push({ count: pendingEntries, text: `${pendingEntries === 1 ? 'entry needs' : 'entries need'} approval`, tab: 'entries' })
  if ((meet.status === 'entry_finalized' || meet.status === 'completed') && approvedEntries > 0) todo.push({ count: approvedEntries, text: 'approved, not yet sent to the host', tab: 'entries' })

  const dateRows: { label: string; value: string | null }[] = [
    { label: 'Dates', value: meet.start_date ? (meet.end_date && meet.end_date !== meet.start_date ? `${formatDateOnly(meet.start_date)} – ${formatDateOnly(meet.end_date)}` : formatDateOnly(meet.start_date)) : null },
    { label: 'Commitments due', value: meet.commitment_deadline ? formatDeadline(meet.commitment_deadline) : null },
    { label: 'Entries due', value: meet.entry_deadline ? formatDeadline(meet.entry_deadline) : null },
    { label: 'Age-up date', value: meet.age_up_date ? formatDateOnly(meet.age_up_date) : null },
  ]

  return (
    <div className="page">
      <Link href="/meets" className="btn btn-ghost u-fs-fs-sm u-gap-6px u-mb-8px" style={{ marginLeft: 'calc(var(--space-2) * -1)' }}>
        <Icon name="chevron-left" size={14} /> Meets
      </Link>

      <div className="page-header">
        <div>
          <div className="u-gap-6px-12px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
            <h1 className="page-title">{meet.name}</h1>
            <StatusBadge status={meet.status} />
          </div>
          <div className="md-meta">
            <span>{meet.course}</span>
            {meet.host_org ? <span>{meet.host_org}</span> : null}
            {meet.location ? <span>{meet.location}</span> : null}
          </div>
        </div>
        <LifecycleActions status={meet.status} onTransition={handleTransition} transitioning={transitioning} />
      </div>

      <StageSteps status={meet.status} />

      {transitionError && (
        <div role="alert" className="u-mb-16px u-p-12px-16px u-fs-fs-base" style={{ borderRadius: '8px', background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error-border)' }}>
          {transitionError}
        </div>
      )}

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'overview' ? (
        <div className="split-aside u-gap-16px">
          <div className="stack" style={{ gap: 'var(--space-4)', minWidth: 0 }}>
            <div className="card">
              <h2 className="card-title u-mb-12px">Needs attention</h2>
              {todo.length > 0 ? (
                <ul className="md-todo">
                  {todo.map((item) => (
                    <li key={item.text}>
                      <button type="button" className="md-todo-item" onClick={() => setActiveTab(item.tab)}>
                        {item.count !== undefined ? <span className="md-todo-count">{item.count}</span> : null}
                        <span className="md-todo-text">{item.text}</span>
                        <Icon name="chevron-right" size={14} className="md-todo-go" />
                      </button>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="md-clear"><Icon name="check-circle" size={15} /> All clear</p>
              )}
            </div>

            {commitments.length > 0 ? (
              <div className="card">
                <h2 className="card-title u-mb-12px">Commitments</h2>
                <StackBar parts={[
                  { label: 'going', value: yes, color: 'var(--success)' },
                  { label: 'not going', value: no, color: 'var(--error)' },
                  { label: 'no response', value: invited, color: 'var(--border-strong, var(--border))' },
                ]} />
              </div>
            ) : null}

            {entries.length > 0 ? (
              <div className="card">
                <h2 className="card-title u-mb-12px">Entries</h2>
                <StackBar parts={[
                  { label: 'to review', value: pendingEntries, color: 'var(--warning)' },
                  { label: 'approved', value: approvedEntries, color: 'var(--success)' },
                  { label: 'exported', value: exportedEntries, color: 'var(--info)' },
                ]} />
              </div>
            ) : null}

            {resultsOpen ? <ResultsImport meetId={meetId} /> : null}
          </div>

          <div className="card">
            <dl className="md-facts">
              {dateRows.map((row) => (
                <FactRow key={row.label} label={row.label} value={row.value} />
              ))}
            </dl>
          </div>
        </div>
      ) : null}

      {activeTab === 'sessions' ? <Table columns={SESSION_COLS} data={sessions} emptyTitle="No sessions" /> : null}
      {activeTab === 'events' && meet ? (
        <div>
          {EDITABLE_EVENT_STATUSES.has(meet.status) && (events.length === 0 || showAddEvent) ? (
            <AddMeetEventForm
              meetId={meetId}
              course={meet.course || 'SCY'}
              sessions={sessions.map((s) => ({ id: s.id, name: s.name }))}
              suggestedEventNumber={nextEventNumber(events)}
              onCreated={() => { setShowAddEvent(false); setReloadKey((k) => k + 1) }}
            />
          ) : null}
          <EventsBySession
            events={events}
            sessions={sessions}
            formatTime={formatTime}
            toolbarExtra={EDITABLE_EVENT_STATUSES.has(meet.status) ? (
              <button type="button" className="btn btn-secondary u-fs-fs-sm u-gap-6px" aria-expanded={showAddEvent} onClick={() => setShowAddEvent((v) => !v)}>
                <Icon name={showAddEvent ? 'x' : 'plus'} size={13} /> {showAddEvent ? 'Cancel' : 'Add event'}
              </button>
            ) : null}
          />
        </div>
      ) : null}
      {activeTab === 'commitments' ? (
        <div>
          <div className="md-toolbar">
            <Link href={`/commitments?meet=${meetId}`} className="btn btn-secondary u-fs-fs-sm">Manage commitments <Icon name="chevron-right" size={12} /></Link>
          </div>
          <Table columns={COMMITMENT_COLS} data={commitments} emptyTitle="No commitments" />
        </div>
      ) : null}
      {activeTab === 'entries' ? (
        <div>
          <EntryFileExport
            meetId={meetId}
            meetStatus={meet.status}
            approvedEntryCount={approvedEntries + exportedEntries}
            onExported={() => setReloadKey((k) => k + 1)}
            actions={(
              <>
                <Link href={`/performance/relays?meet_id=${meetId}`} className="btn btn-secondary u-fs-fs-sm">Build relays</Link>
                <Link href={`/entries?meet=${meetId}`} className="btn btn-secondary u-fs-fs-sm">Manage entries</Link>
              </>
            )}
          />
          <Table columns={ENTRY_COLS} data={entries} emptyTitle="No entries" />
        </div>
      ) : null}
      {activeTab === 'volunteers' ? <MeetVolunteersPanel meetId={meetId} meetStartDate={meet?.start_date} /> : null}
      {activeTab === 'reports' ? <ReportsPanel meetId={meetId} /> : null}
    </div>
  )
}

function FactRow({ label, value }: { label: string; value: string | null }) {
  return (
    <>
      <dt>{label}</dt>
      <dd data-empty={!value}>{value ?? 'Not set'}</dd>
    </>
  )
}

