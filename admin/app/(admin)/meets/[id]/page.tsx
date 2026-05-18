'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
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

function formatDate(value?: string | null) {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function formatTime(value?: string | null) {
  if (!value) return '—'
  const parsed = new Date(value)
  if (!Number.isNaN(parsed.getTime())) {
    return parsed.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
  }
  return value
}

function formatSeedTime(cents?: number) {
  if (cents == null) return 'NT'
  const seconds = cents / 100
  if (seconds >= 60) {
    const minutes = Math.floor(seconds / 60)
    const remainder = (seconds % 60).toFixed(2).padStart(5, '0')
    return `${minutes}:${remainder}`
  }
  return seconds.toFixed(2)
}

function formatAgeGroup(minAge?: number, maxAge?: number, fallback?: string) {
  if (fallback) return fallback
  if (minAge != null && maxAge != null) return `${minAge}-${maxAge}`
  if (minAge != null) return `${minAge}&O`
  if (maxAge != null) return `Under ${maxAge}`
  return 'Open'
}

function StatusBadge({ status }: { status: string }) {
  const cfg: Record<string, { label: string; color: string; bg: string }> = {
    draft: { label: 'Draft', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    published: { label: 'Published', color: 'var(--info)', bg: 'var(--info-bg)' },
    commitment_open: { label: 'Commitments Open', color: 'var(--warning)', bg: 'var(--warning-bg)' },
    commitment_closed: { label: 'Commitments Closed', color: 'var(--warning)', bg: 'var(--warning-bg)' },
    entry_finalized: { label: 'Entries Finalized', color: 'var(--accent)', bg: 'var(--accent-subtle)' },
    completed: { label: 'Completed', color: 'var(--success)', bg: 'var(--success-bg)' },
    archived: { label: 'Archived', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    invited: { label: 'Invited', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    committed_yes: { label: 'Committed Yes', color: 'var(--success)', bg: 'var(--success-bg)' },
    committed_no: { label: 'Committed No', color: 'var(--error)', bg: 'var(--error-bg)' },
    locked: { label: 'Locked', color: 'var(--accent)', bg: 'var(--accent-subtle)' },
    submitted: { label: 'Submitted', color: 'var(--info)', bg: 'var(--info-bg)' },
    approved: { label: 'Approved', color: 'var(--success)', bg: 'var(--success-bg)' },
  }
  const colors = cfg[status] ?? { label: status, color: 'var(--text-muted)', bg: 'var(--bg-elevated)' }
  return <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: colors.bg, color: colors.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em' }}>{colors.label}</span>
}

const SESSION_COLS: Column<SessionRow>[] = [
  { key: 'name', label: 'Session', render: (row) => <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{row.name}</span> },
  { key: 'date', label: 'Date', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatDate(row.date)}</span> },
  { key: 'warmupTime', label: 'Warm-up', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatTime(row.warmupTime)}</span> },
  { key: 'startTime', label: 'Start', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text)' }}>{formatTime(row.startTime)}</span> },
  { key: 'eventCount', label: 'Events', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text)' }}>{row.eventCount}</span> },
]

const EVENT_COLS: Column<EventRow>[] = [
  { key: 'eventNumber', label: 'Event', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', fontWeight: 600, color: 'var(--text)' }}>{row.eventNumber}</span> },
  { key: 'name', label: 'Name', render: (row) => <span style={{ fontSize: '13px', color: 'var(--text)' }}>{row.name}</span> },
  { key: 'sessionName', label: 'Session', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.sessionName}</span> },
  { key: 'ageGroup', label: 'Age/Gender', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{row.gender} {row.ageGroup}</span> },
  { key: 'entries', label: 'Entries', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--accent)' }}>{row.entries}</span> },
]

const COMMITMENT_COLS: Column<CommitmentRow>[] = [
  { key: 'swimmer', label: 'Swimmer', render: (row) => <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{row.swimmer}</span> },
  { key: 'household', label: 'Household', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.household}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
]

const ENTRY_COLS: Column<EntryRow>[] = [
  { key: 'swimmer', label: 'Swimmer', render: (row) => <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{row.swimmer}</span> },
  { key: 'event', label: 'Event', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.event}</span> },
  { key: 'seedTime', label: 'Seed', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--accent)' }}>{row.seedTime}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
]

// ─── Lifecycle bar ────────────────────────────────────────────────────────────

const LIFECYCLE_STAGES = [
  { key: 'draft', label: 'Draft' },
  { key: 'published', label: 'Published' },
  { key: 'commitment_open', label: 'Commitments Open' },
  { key: 'commitment_closed', label: 'Commitments Closed' },
  { key: 'entry_finalized', label: 'Entries Finalized' },
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
      return [{ label: 'Publish Meet', toStatus: 'published', variant: 'primary' }]
    case 'published':
      return [
        { label: 'Open Commitments', toStatus: 'commitment_open', variant: 'primary' },
        { label: 'Revert to Draft', toStatus: 'draft', variant: 'danger', confirm: 'Revert this meet back to draft? It will no longer be visible to swimmers.' },
      ]
    case 'commitment_open':
      return [{ label: 'Close Commitments', toStatus: 'commitment_closed', variant: 'primary', confirm: 'Close commitments? Swimmers will no longer be able to respond.' }]
    case 'commitment_closed':
      return [{ label: 'Finalize Entries', toStatus: 'entry_finalized', variant: 'primary' }]
    case 'entry_finalized':
      return [{ label: 'Mark Complete', toStatus: 'completed', variant: 'primary', confirm: 'Mark this meet as completed?' }]
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
  const stageIndex = LIFECYCLE_STAGES.findIndex((s) => s.key === meet.status)
  const actions = getLifecycleActions(meet.status)

  if (meet.status === 'archived') return null

  return (
    <div style={{
      background: 'var(--bg-elevated)',
      border: '1px solid var(--border)',
      borderRadius: '10px',
      padding: '16px 20px',
      marginBottom: '24px',
      display: 'flex',
      alignItems: 'center',
      gap: '20px',
      flexWrap: 'wrap',
    }}>
      {/* Progress track */}
      <div style={{ flex: 1, minWidth: '260px', display: 'flex', alignItems: 'center', gap: 0 }}>
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
      <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', whiteSpace: 'nowrap', flexShrink: 0 }}>
        {LIFECYCLE_STAGES[stageIndex]?.label ?? meet.status}
      </div>

      {/* Action buttons */}
      {actions.length > 0 && (
        <div style={{ display: 'flex', gap: '8px', flexShrink: 0 }}>
          {actions.map((action) => (
            <button
              key={action.toStatus}
              disabled={transitioning}
              onClick={() => {
                if (action.confirm) {
                  if (!window.confirm(action.confirm)) return
                }
                onTransition(action.toStatus)
              }}
              style={{
                padding: '7px 14px',
                borderRadius: '6px',
                fontSize: '12px',
                fontWeight: 600,
                fontFamily: 'var(--font-mono)',
                cursor: transitioning ? 'not-allowed' : 'pointer',
                opacity: transitioning ? 0.6 : 1,
                transition: 'all 0.15s',
                border: action.variant === 'ghost' ? '1px solid var(--border)' : 'none',
                background: action.variant === 'primary'
                  ? 'var(--accent)'
                  : action.variant === 'danger'
                  ? 'var(--error-bg)'
                  : 'transparent',
                color: action.variant === 'primary'
                  ? '#fff'
                  : action.variant === 'danger'
                  ? 'var(--error)'
                  : 'var(--text-subtle)',
              }}
            >
              {transitioning ? '…' : action.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function MeetDetailPage() {
  const params = useParams<{ id: string }>()
  const meetId = typeof params?.id === 'string' ? params.id : ''
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [activeTab, setActiveTab] = useState('overview')
  const [meet, setMeet] = useState<MeetRecord | null>(null)
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [events, setEvents] = useState<EventRow[]>([])
  const [commitments, setCommitments] = useState<CommitmentRow[]>([])
  const [entries, setEntries] = useState<EntryRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [transitioning, setTransitioning] = useState(false)
  const [transitionError, setTransitionError] = useState<string | null>(null)

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
        const entryCountByEvent = rawEntries.reduce<Record<string, number>>((acc, entry) => {
          if (entry.meet_event_id) acc[entry.meet_event_id] = (acc[entry.meet_event_id] ?? 0) + 1
          return acc
        }, {})

        setMeet(nextMeet)
        setSessions(rawSessions.map((meetSession) => ({
          id: meetSession.id,
          name: meetSession.name,
          date: meetSession.session_date || meetSession.scheduled_date || '',
          warmupTime: meetSession.warmup_time || meetSession.warmup_at || '',
          startTime: meetSession.start_time || meetSession.start_at || '',
          eventCount: rawEvents.filter((event) => event.session_id === meetSession.id).length,
        })).sort((a, b) => a.date.localeCompare(b.date)))
        setEvents(rawEvents.map((event) => ({
          id: event.id,
          eventNumber: event.event_number?.toString() || '—',
          name: [event.distance, event.stroke].filter(Boolean).join(' ') || 'Meet event',
          course: nextMeet.course,
          gender: event.gender || 'X',
          ageGroup: formatAgeGroup(event.min_age, event.max_age, event.age_group),
          entries: entryCountByEvent[event.id] ?? 0,
          sessionName: sessionById.get(event.session_id || '')?.name || 'Unscheduled',
        })).sort((a, b) => a.eventNumber.localeCompare(b.eventNumber, undefined, { numeric: true })))
        setCommitments(rawCommitments.map((commitment) => {
          const swimmer = swimmerById.get(commitment.swimmer_id)
          return {
            id: commitment.id,
            swimmer: swimmer ? `${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim() : commitment.swimmer_id,
            household: swimmer ? (householdById.get(swimmer.household_id) || 'Unknown household') : 'Unknown household',
            status: commitment.status,
          }
        }).sort((a, b) => a.swimmer.localeCompare(b.swimmer)))
        setEntries(rawEntries.map((entry) => {
          const swimmer = swimmerById.get(entry.swimmer_id)
          const event = rawEvents.find((candidate) => candidate.id === entry.meet_event_id)
          return {
            id: entry.id,
            swimmer: swimmer ? `${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim() : entry.swimmer_id,
            event: event ? `${event.distance || ''} ${event.stroke || ''} ${event.gender || ''} ${formatAgeGroup(event.min_age, event.max_age, event.age_group)}`.trim() : 'Meet event',
            seedTime: formatSeedTime(entry.seed_time_cents),
            status: entry.status,
          }
        }).sort((a, b) => a.swimmer.localeCompare(b.swimmer)))
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load meet details.')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, meetId, session?.user.orgId])

  async function handleTransition(toStatus: string) {
    if (!session?.user.orgId || !meetId) return
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
    } catch (err) {
      setTransitionError(err instanceof Error ? err.message : 'Transition failed.')
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
  ], [commitments.length, entries.length, events.length, sessions.length])

  if (loading) {
    return <div style={{ padding: '24px', maxWidth: '1200px' }}><EmptyState icon="calendar" title="Loading meet" description="Fetching live meet detail data from the API." /></div>
  }

  if (error || !meet) {
    return <div style={{ padding: '24px', maxWidth: '1200px' }}><EmptyState icon="alert-circle" title="Unable to load meet" description={error ?? 'Meet not found.'} /></div>
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <Link href="/meets" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}>
        <Icon name="chevron-left" size={14} /> Meets
      </Link>

      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
            <h1 style={{ fontSize: '24px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2 }}>{meet.name}</h1>
            <StatusBadge status={meet.status} />
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
            {meet.id} · {meet.host_org || 'Host TBD'} · {meet.location || 'Location TBD'}
          </div>
        </div>
      </div>

      <MeetLifecycleBar meet={meet} onTransition={handleTransition} transitioning={transitioning} />

      {transitionError && (
        <div style={{ marginBottom: '16px', padding: '10px 14px', borderRadius: '6px', background: 'var(--error-bg)', color: 'var(--error)', fontSize: '13px', fontFamily: 'var(--font-mono)', border: '1px solid var(--error)' }}>
          {transitionError}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'Course', value: meet.course, color: 'var(--text)' },
          { label: 'Sessions', value: sessions.length.toString(), color: 'var(--text)' },
          { label: 'Events', value: events.length.toString(), color: 'var(--accent)' },
          { label: 'Entries', value: entries.length.toString(), color: 'var(--success)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '18px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'overview' ? (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Meet Details</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[
                { label: 'Start Date', value: formatDate(meet.start_date) },
                { label: 'End Date', value: formatDate(meet.end_date || meet.start_date) },
                { label: 'Commit Deadline', value: formatDate(meet.commitment_deadline) },
                { label: 'Entry Deadline', value: formatDate(meet.entry_deadline) },
                { label: 'Host', value: meet.host_org || '—' },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600 }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Operations Snapshot</h3>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7 }}>
              This detail view is backed by persisted meet, session, event, commitment, and entry data, and the operational counts and tables below are all live.
            </p>
          </div>
        </div>
      ) : null}

      {activeTab === 'sessions' ? <Table columns={SESSION_COLS} data={sessions} emptyTitle="No sessions" emptyDescription="No meet sessions have been created yet." /> : null}
      {activeTab === 'events' ? <Table columns={EVENT_COLS} data={events} emptyTitle="No events" emptyDescription="No events have been configured yet." /> : null}
      {activeTab === 'commitments' ? <Table columns={COMMITMENT_COLS} data={commitments} emptyTitle="No commitments" emptyDescription="No swimmer commitments are recorded for this meet." /> : null}
      {activeTab === 'entries' ? <Table columns={ENTRY_COLS} data={entries} emptyTitle="No entries" emptyDescription="No entries have been proposed for this meet yet." /> : null}
    </div>
  )
}
