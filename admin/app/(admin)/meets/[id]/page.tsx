'use client'

import { Suspense, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useParams, useSearchParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { formatDateOnly, parseBackendDate } from '@/lib/dates'
import { formatSwimTime } from '@/lib/performanceData'
import { useApiClient } from '@/lib/api'
import { ReportsPanel } from './ReportsPanel'
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

type SessionRow = {
  id: string
  name: string
  date: string
  warmupTime: string
  startTime: string
  eventCount: number
}

type EventRow = {
  id: string
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
  { key: 'warmupTime', label: 'Warm-up', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{formatTime(row.warmupTime)}</span> },
  { key: 'startTime', label: 'Start', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text)' }}>{formatTime(row.startTime)}</span> },
  { key: 'eventCount', label: 'Events', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text)' }}>{row.eventCount}</span> },
]

const EVENT_COLS: Column<EventRow>[] = [
  { key: 'eventNumber', label: 'Event', render: (row) => <span className="u-ff-font-body u-fs-fs-sm u-fw-600" style={{ color: 'var(--text)' }}>{row.eventNumber}</span> },
  { key: 'name', label: 'Name', render: (row) => <span className="u-fs-fs-base" style={{ color: 'var(--text)' }}>{row.name}</span> },
  { key: 'sessionName', label: 'Session', render: (row) => <span className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{row.sessionName}</span> },
  { key: 'ageGroup', label: 'Age/Gender', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{row.gender} {row.ageGroup}</span> },
  { key: 'entries', label: 'Entries', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--accent-text, var(--accent))' }}>{row.entries}</span> },
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

// ─── Lifecycle bar ────────────────────────────────────────────────────────────

const LIFECYCLE_STAGES = [
  { key: 'draft', label: 'Draft' },
  { key: 'published', label: 'Published' },
  { key: 'commitment_open', label: 'Commitments open' },
  { key: 'commitment_closed', label: 'Commitments closed' },
  { key: 'entry_finalized', label: 'Entries finalized' },
  { key: 'completed', label: 'Completed' },
  { key: 'archived', label: 'Archived' },
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

function MeetLifecycleBar({
  meet,
  onTransition,
  transitioning,
}: {
  meet: MeetRecord
  onTransition: (toStatus: string) => void
  transitioning: boolean
}) {
  const confirm = useConfirm()
  const stageIndex = LIFECYCLE_STAGES.findIndex((s) => s.key === meet.status)
  const actions = getLifecycleActions(meet.status)

  if (meet.status === 'archived') return null

  return (
    <div className="u-p-16px-20px u-mb-24px u-gap-20px" style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '12px', display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
      {/* Progress track */}
      <div className="u-gap-0" style={{ flex: 1, minWidth: '260px', display: 'flex', alignItems: 'center' }}>
        {LIFECYCLE_STAGES.filter((s) => s.key !== 'archived').map((stage, i, arr) => {
          const idx = LIFECYCLE_STAGES.findIndex((s2) => s2.key === stage.key)
          const isPast = idx < stageIndex
          const isCurrent = idx === stageIndex
          const isLast = i === arr.length - 1
          const dotColor = isPast ? 'var(--success)' : isCurrent ? 'var(--accent)' : 'var(--border)'
          const lineColor = isPast ? 'var(--success)' : 'var(--border)'
          return (
            <div key={stage.key} style={{ display: 'flex', alignItems: 'center', flex: isLast ? 0 : 1, minWidth: 0 }}>
              <div title={stage.label} style={{
                width: isCurrent ? '10px' : '8px',
                height: isCurrent ? '10px' : '8px',
                borderRadius: '50%',
                background: dotColor,
                border: isCurrent ? `2px solid var(--accent)` : 'none',
                boxShadow: isCurrent ? '0 0 0 3px var(--accent-subtle)' : 'none',
                flexShrink: 0,
                transition: 'all 0.2s',
              }} />
              {!isLast && (
                <div style={{ flex: 1, height: '2px', background: lineColor, minWidth: '8px', transition: 'background 0.2s' }} />
              )}
            </div>
          )
        })}
      </div>

      {/* Current label */}
      <div className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)', whiteSpace: 'nowrap', flexShrink: 0 }}>
        {LIFECYCLE_STAGES[stageIndex]?.label ?? meet.status}
      </div>

      {/* Action buttons */}
      {actions.length > 0 && (
        <div className="u-gap-8px" style={{ display: 'flex', flexShrink: 0 }}>
          {actions.map((action) => (
            <button type="button"
              key={action.toStatus}
              disabled={transitioning}
              onClick={async () => {
                if (action.confirm) {
                  const ok = await confirm({ title: `${action.label}?`, message: action.confirm, confirmLabel: action.label, destructive: action.variant === 'danger' })
                  if (!ok) return
                }
                onTransition(action.toStatus)
              }}
              className="u-p-8px-16px u-fs-fs-sm u-fw-600 u-ff-font-body" style={{ borderRadius: '6px', cursor: transitioning ? 'not-allowed' : 'pointer', opacity: transitioning ? 0.6 : 1, transition: 'all 0.15s', border: action.variant === 'ghost' ? '1px solid var(--border)' : 'none', background: action.variant === 'primary'
                  ? 'var(--accent)'
                  : action.variant === 'danger'
                  ? 'var(--error-bg)'
                  : 'transparent', color: action.variant === 'primary'
                  ? '#fff'
                  : action.variant === 'danger'
                  ? 'var(--error)'
                  : 'var(--text-subtle)' }}
            >
              {transitioning ? '…' : action.label}
            </button>
          ))}
        </div>
      )}
    </div>
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
  const [lastRefreshedAt, setLastRefreshedAt] = useState<Date | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

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
        })).sort((a, b) => a.date.localeCompare(b.date)))
        setEvents(rawEvents.map((event) => ({
          id: event.id,
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
        setLastRefreshedAt(new Date())
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
    return <div className="page"><EmptyState icon="calendar" title="Loading meet" description="Fetching meet details." /></div>
  }

  if (error || !meet) {
    return <div className="page"><EmptyState icon="alert-circle" title="Unable to load meet" description={error ?? 'Meet not found.'} /></div>
  }

  return (
    <div className="page">
      <Link href="/meets" className="u-gap-4px u-fs-fs-base u-mb-20px" style={{ display: 'inline-flex', alignItems: 'center', color: 'var(--text-subtle)', textDecoration: 'none' }}>
        <Icon name="chevron-left" size={14} /> Meets
      </Link>

      <div className="page-header">
        <div>
          <div className="u-gap-6px-12px u-mb-4px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
            <h1 className="page-title">{meet.name}</h1>
            <StatusBadge status={meet.status} />
          </div>
          <div className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
            {meet.host_org || 'Host TBD'} · {meet.location || 'Location TBD'}
          </div>
        </div>
      </div>

      <MeetLifecycleBar meet={meet} onTransition={handleTransition} transitioning={transitioning} />

      {transitionError && (
        <div className="u-mb-16px u-p-12px-16px u-fs-fs-base u-ff-font-body" style={{ borderRadius: '6px', background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)' }}>
          {transitionError}
        </div>
      )}

      <div className="cols-4 u-gap-12px u-mb-24px" >
        {[
          { label: 'Course', value: meet.course, color: 'var(--text)' },
          { label: 'Sessions', value: sessions.length.toString(), color: 'var(--text)' },
          { label: 'Events', value: events.length.toString(), color: 'var(--accent-text, var(--accent))' },
          { label: 'Entries', value: entries.length.toString(), color: 'var(--success)' },
        ].map((stat) => (
          <div key={stat.label} className="card u-p-16px-16px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</div>
            <div className="u-fs-fs-xl u-ff-font-body u-fw-700" style={{ color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'overview' ? (
        <div className="cols-2 u-gap-16px" >
          {meet.status === 'entry_finalized' || meet.status === 'completed' || meet.status === 'archived' ? (
            <ResultsImport meetId={meetId} />
          ) : null}
          <div className="card u-p-20px" >
            <h3 className="card-title u-mb-16px" >Dates &amp; host</h3>
            <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
              {[
                { label: 'Start date', value: formatDateOnly(meet.start_date) },
                { label: 'End date', value: formatDateOnly(meet.end_date || meet.start_date) },
                { label: 'Commit deadline', value: formatDateOnly(meet.commitment_deadline) },
                { label: 'Entry deadline', value: formatDateOnly(meet.entry_deadline) },
                { label: 'Host', value: meet.host_org || '—' },
              ].map((row) => (
                <div key={row.label} className="u-fs-fs-base u-pb-12px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--border)' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span className="u-ff-font-body u-fw-600" style={{ color: 'var(--text)' }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="card u-p-20px" >
            <h3 className="card-title u-mb-16px" >Open items</h3>
            <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div className="u-fs-fs-base" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ color: 'var(--text-muted)' }}>Households yet to respond</span>
                <span className="u-ff-font-body u-fw-600" style={{ color: commitments.filter((c) => c.status === 'invited').length > 0 ? 'var(--warning)' : 'var(--text)' }}>
                  {commitments.filter((c) => c.status === 'invited').length}
                </span>
              </div>
              <div className="u-fs-fs-base" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ color: 'var(--text-muted)' }}>Entries awaiting approval</span>
                <span className="u-ff-font-body u-fw-600" style={{ color: entries.filter((e) => e.status === 'submitted').length > 0 ? 'var(--warning)' : 'var(--text)' }}>
                  {entries.filter((e) => e.status === 'submitted').length}
                </span>
              </div>
              <p className="u-fs-fs-xs u-ff-font-body u-mt-4px" style={{ color: 'var(--text-muted)' }}>
                {lastRefreshedAt ? `Updated ${lastRefreshedAt.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}` : 'Waiting for first refresh…'}
              </p>
            </div>
          </div>
        </div>
      ) : null}

      {activeTab === 'sessions' ? <Table columns={SESSION_COLS} data={sessions} emptyTitle="No sessions" emptyDescription="No meet sessions have been created yet." /> : null}
      {activeTab === 'events' && meet ? (
        <div>
          {EDITABLE_EVENT_STATUSES.has(meet.status) ? (
            <AddMeetEventForm
              meetId={meetId}
              course={meet.course || 'SCY'}
              sessions={sessions.map((s) => ({ id: s.id, name: s.name }))}
              suggestedEventNumber={nextEventNumber(events)}
              onCreated={() => setReloadKey((k) => k + 1)}
            />
          ) : null}
          <Table columns={EVENT_COLS} data={events} emptyTitle="No events" emptyDescription="No events have been configured yet." />
        </div>
      ) : null}
      {activeTab === 'commitments' ? (
        <div>
          <div className="u-mb-12px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <Link href={`/commitments?meet=${meetId}`} className="u-fs-fs-sm u-gap-4px" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none', display: 'inline-flex', alignItems: 'center' }}>
              Manage commitments (selections, approvals, lock) <Icon name="chevron-right" size={12} />
            </Link>
          </div>
          <Table columns={COMMITMENT_COLS} data={commitments} emptyTitle="No commitments" emptyDescription="No swimmer commitments are recorded for this meet." />
        </div>
      ) : null}
      {activeTab === 'entries' ? (
        <div>
          {meet && (meet.status === 'entry_finalized' || meet.status === 'completed') && entries.length > 0 ? (
            <EntryFileExport meetId={meetId} onExported={() => setReloadKey((k) => k + 1)} />
          ) : null}
          <div className="u-gap-16px u-mb-12px" style={{ display: 'flex', justifyContent: 'flex-end', flexWrap: 'wrap' }}>
            <Link href={`/performance/relays?meet_id=${meetId}`} className="u-fs-fs-sm u-gap-4px" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none', display: 'inline-flex', alignItems: 'center' }}>
              Build relays <Icon name="chevron-right" size={12} />
            </Link>
            <Link href={`/entries?meet=${meetId}`} className="u-fs-fs-sm u-gap-4px" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none', display: 'inline-flex', alignItems: 'center' }}>
              Manage entries (approve, scratch, add) <Icon name="chevron-right" size={12} />
            </Link>
          </div>
          <Table columns={ENTRY_COLS} data={entries} emptyTitle="No entries" emptyDescription="No entries have been proposed for this meet yet." />
        </div>
      ) : null}
      {activeTab === 'volunteers' ? <MeetVolunteersPanel meetId={meetId} meetStartDate={meet?.start_date} /> : null}
      {activeTab === 'reports' ? <ReportsPanel meetId={meetId} /> : null}
    </div>
  )
}
