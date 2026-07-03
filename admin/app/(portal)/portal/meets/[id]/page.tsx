'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { parseBackendDate } from '@/lib/dates'
import { ApiError, useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  addCommitmentEventSelection,
  deleteCommitmentEventSelection,
  fetchMeet,
  fetchCommitmentEventSelections,
  fetchMeetCommitments,
  fetchMeetEvents,
  fetchMeetSessions,
  fetchExportsForMeet,
  fetchSwimmers,
  respondToMeetCommitment,
  type CommitmentEventSelectionRecord,
  type ExportRecord,
  MeetRecord,
  MeetEventRecord,
  MeetSessionRecord,
  SwimmerRecord,
} from '@/lib/adminData'
import {
  buildPortalSwimmerStates,
  canEditPortalMeetSelections,
  type PortalSwimmerCommitmentState,
  withAddedPortalMeetSelection,
  withRemovedPortalMeetSelection,
} from '@/lib/portal-meet-event-selections'

// ── Types ────────────────────────────────────────────────────────────────────
type SwimmerState = PortalSwimmerCommitmentState

// ── Helpers ──────────────────────────────────────────────────────────────────
function formatDateRange(start?: string, end?: string): string {
  if (!start) return '—'
  const s = parseBackendDate(start)
  const opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' }
  if (!end || end === start) return s.toLocaleDateString('en-US', { ...opts, year: 'numeric' })
  const e = parseBackendDate(end)
  if (s.getFullYear() === e.getFullYear()) {
    return `${s.toLocaleDateString('en-US', opts)}–${e.toLocaleDateString('en-US', { ...opts, year: 'numeric' })}`
  }
  return `${s.toLocaleDateString('en-US', { ...opts, year: 'numeric' })}–${e.toLocaleDateString('en-US', { ...opts, year: 'numeric' })}`
}

function swimmerInitials(s: SwimmerRecord): string {
  const first = (s.preferred_name ?? s.first_name)[0] ?? ''
  const last = s.last_name[0] ?? ''
  return (first + last).toUpperCase()
}

function swimmerDisplayName(s: SwimmerRecord): string {
  return `${s.preferred_name ?? s.first_name} ${s.last_name}`
}

function swimmerAge(s: SwimmerRecord): number {
  const dob = new Date(s.date_of_birth)
  const now = new Date()
  let age = now.getFullYear() - dob.getFullYear()
  const m = now.getMonth() - dob.getMonth()
  if (m < 0 || (m === 0 && now.getDate() < dob.getDate())) age--
  return age
}

const SWIMMER_GRADIENTS = [
  'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
  'linear-gradient(135deg, #10b981 0%, #059669 100%)',
  'linear-gradient(135deg, #f59e0b 0%, #d97706 100%)',
  'linear-gradient(135deg, #ec4899 0%, #a855f7 100%)',
  'linear-gradient(135deg, #06b6d4 0%, #3b82f6 100%)',
]

function eventLabel(ev: MeetEventRecord): string {
  const parts: string[] = []
  if (ev.distance) parts.push(String(ev.distance))
  if (ev.stroke) parts.push(ev.stroke.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()))
  return parts.length > 0 ? parts.join(' ') : `Event #${ev.event_number ?? ev.id.slice(-4)}`
}

function eventAgeGroup(ev: MeetEventRecord): string {
  const parts: string[] = []
  if (ev.gender) parts.push(ev.gender === 'F' ? 'Girls' : ev.gender === 'M' ? 'Boys' : ev.gender === 'X' ? 'Mixed' : ev.gender)
  if (ev.age_group) {
    parts.push(ev.age_group)
  } else if (ev.min_age != null && ev.max_age != null) {
    parts.push(`${ev.min_age}-${ev.max_age}`)
  } else if (ev.min_age != null) {
    parts.push(`${ev.min_age}&O`)
  } else if (ev.max_age != null) {
    parts.push(`Under ${ev.max_age}`)
  }
  return parts.join(' ')
}

function formatMeetFileSize(sizeBytes?: number): string {
  if (!sizeBytes) return 'Unknown size'
  if (sizeBytes < 1024) return `${sizeBytes} B`
  return `${Math.round(sizeBytes / 1024)} KB`
}

function formatMeetFileType(exportType: string): string {
  if (exportType === 'eligibility_report') return 'Eligibility report'
  if (exportType === 'entry_report') return 'Entry report'
  if (exportType === 'event_report') return 'Event report'
  return exportType.replace(/_/g, ' ')
}

// ── Loading skeleton ─────────────────────────────────────────────────────────
function LoadingSkeleton() {
  return (
    <div style={{ maxWidth: '860px' }}>
      <div style={{ height: '20px', width: '160px', background: 'rgba(0,0,0,0.06)', borderRadius: '6px', marginBottom: '24px' }} />
      <div
        style={{
          background: '#ffffff',
          borderRadius: '16px',
          border: '1px solid rgba(0,0,0,0.07)',
          padding: '24px 28px',
          marginBottom: '24px',
        }}
      >
        <div style={{ height: '28px', width: '280px', background: 'rgba(0,0,0,0.06)', borderRadius: '8px', marginBottom: '12px' }} />
        <div style={{ display: 'flex', gap: '16px' }}>
          <div style={{ height: '14px', width: '120px', background: 'rgba(0,0,0,0.04)', borderRadius: '4px' }} />
          <div style={{ height: '14px', width: '140px', background: 'rgba(0,0,0,0.04)', borderRadius: '4px' }} />
        </div>
      </div>
      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(12px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  )
}

// ── Component ────────────────────────────────────────────────────────────────
export default function MeetDetailPage() {
  const params = useParams<{ id: string }>()
  const meetId = typeof params?.id === 'string' ? params.id : ''

  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''

  const [meet, setMeet] = useState<MeetRecord | null>(null)
  const [events, setEvents] = useState<MeetEventRecord[]>([])
  const [sessions, setSessions] = useState<MeetSessionRecord[]>([])
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [meetFiles, setMeetFiles] = useState<ExportRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [filesLoading, setFilesLoading] = useState(true)
  const [filesError, setFilesError] = useState<string | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  const [swimmerStates, setSwimmerStates] = useState<Record<string, SwimmerState>>({})
  const [activeSwimmer, setActiveSwimmer] = useState<string>('')
  const [submitting, setSubmitting] = useState<string | null>(null)
  const [selectionSubmitting, setSelectionSubmitting] = useState<string | null>(null)
  const [submitted, setSubmitted] = useState<Record<string, boolean>>({})
  const [commitError, setCommitError] = useState<string | null>(null)
  const [selectionError, setSelectionError] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId || !meetId) {
      setLoading(false)
      setFilesLoading(false)
      setLoadError('Meet detail is unavailable because meet context is missing.')
      return
    }
    setLoading(true)
    setFilesLoading(true)
    setFilesError(null)
    setLoadError(null)
    Promise.all([
      fetchMeet(api, orgId, meetId),
      fetchMeetCommitments(api, orgId, meetId),
      fetchMeetEvents(api, orgId, meetId),
      fetchSwimmers(api, orgId),
      fetchMeetSessions(api, orgId, meetId).catch(() => [] as MeetSessionRecord[]),
      fetchExportsForMeet(api, orgId, meetId).catch((error) => {
        if (error instanceof ApiError && error.isForbidden()) {
          setFilesError('Meet files become available when this meet is tied to your household commitments.')
        } else {
          setFilesError(error instanceof ApiError ? error.message : 'Failed to load meet files.')
        }
        return [] as ExportRecord[]
      }),
    ])
      .then(async ([m, cmts, evs, swmrs, sess, files]) => {
        setMeet(m)
        setEvents(evs)
        setSessions(sess)
        setMeetFiles(files)

        // Filter to household swimmers
        const householdSwimmers = householdId
          ? swmrs.filter((s) => s.household_id === householdId)
          : swmrs

        setSwimmers(householdSwimmers)

        const selectionsByCommitmentId: Record<string, CommitmentEventSelectionRecord[]> = {}
        await Promise.all(
          cmts.map(async (commitment) => {
            if (!commitment.id) return
            selectionsByCommitmentId[commitment.id] = await fetchCommitmentEventSelections(api, orgId, meetId, commitment.id).catch(
              () => [] as CommitmentEventSelectionRecord[],
            )
          }),
        )

        const initialStates = buildPortalSwimmerStates(householdSwimmers, cmts, selectionsByCommitmentId)
        setSwimmerStates(initialStates)
        if (householdSwimmers.length > 0) setActiveSwimmer(householdSwimmers[0].id)
      })
      .catch((error) => {
        setMeet(null)
        setEvents([])
        setSwimmers([])
        setMeetFiles([])
        setSwimmerStates({})
        setActiveSwimmer('')
        setLoadError(reportUIError(
          'portal-meet-detail-load',
          error,
          'Meet detail data is unavailable right now.',
          { orgId, householdId, meetId },
        ))
      })
      .finally(() => {
        setLoading(false)
        setFilesLoading(false)
      })
  }, [api, orgId, householdId, meetId, reloadKey])

  if (loading) return <LoadingSkeleton />

  if (!meet) {
    return (
      <div style={{ maxWidth: '720px' }}>
        <nav style={{ marginBottom: '20px', fontSize: '13px' }}>
          <Link href="/portal/meets" style={{ color: '#9A9693', textDecoration: 'none' }}>
            Meets
          </Link>
        </nav>
        {loadError && (
          <div
            style={{
              background: '#fff7ed',
              border: '1px solid #fdba74',
              borderRadius: '10px',
              padding: '12px 16px',
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              gap: '12px',
              marginBottom: '20px',
            }}
          >
            <div style={{ display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
              <Icon name="alert-circle" size={15} style={{ color: '#ea580c', marginTop: '1px', flexShrink: 0 }} />
              <p style={{ margin: 0, fontSize: '12.5px', color: '#9a3412', lineHeight: 1.5 }}>{loadError}</p>
            </div>
            <button
              type="button"
              onClick={() => setReloadKey((key) => key + 1)}
              style={{ background: '#ffffff', border: '1px solid #fdba74', borderRadius: '8px', cursor: 'pointer', color: '#9a3412', fontSize: '12px', fontWeight: 600, padding: '8px 14px', whiteSpace: 'nowrap' }}
            >
              Retry
            </button>
          </div>
        )}
        <div
          style={{
            background: '#ffffff',
            borderRadius: '16px',
            border: '1px solid rgba(0,0,0,0.07)',
            boxShadow: '0 2px 12px rgba(0,0,0,0.05)',
            padding: '28px',
          }}
        >
          <div style={{ fontSize: '11px', color: '#9A9693', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '10px' }}>
            Meet Center
          </div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', color: '#1A1714', margin: '0 0 10px' }}>
            {loadError ? 'Meet details are unavailable right now.' : 'Meet not found'}
          </h1>
          <p style={{ fontSize: '14px', color: '#6B6866', lineHeight: 1.6, marginBottom: '16px' }}>
            {loadError
              ? 'Refresh this page or return to the meet list once the service is available again.'
              : <>We couldn&apos;t find a meet for <span style={{ fontFamily: 'var(--font-mono)' }}>{meetId || 'this route'}</span>. Return to the meet list to open an available invite.</>}
          </p>
          <Link href="/portal/meets" style={{ fontSize: '13px', fontWeight: 600 }}>
            Back to all meets
          </Link>
        </div>
      </div>
    )
  }

  const deadlineStr = meet.commitment_deadline ?? meet.entry_deadline
  const deadlineDate = deadlineStr ? new Date(deadlineStr) : null
  const now = new Date()
  const daysUntilDeadline = deadlineDate
    ? Math.max(0, Math.ceil((deadlineDate.getTime() - now.getTime()) / 86400000))
    : null

  const commitmentOpen = meet.status === 'commitment_open'

  const swimmer = swimmers.find((s) => s.id === activeSwimmer)
  const state = activeSwimmer ? swimmerStates[activeSwimmer] : undefined

  // Group events by session_id for display
  const sessionIds = Array.from(new Set(events.map((e) => e.session_id ?? 'main')))
  const sessionNameById = new Map(sessions.map((s) => [s.id, s.name?.trim() || `Session ${s.session_number}`]))
  const sessionLabel = (sid: string) => (sid === 'main' ? 'Main Session' : sessionNameById.get(sid) ?? `Session ${sid.slice(-4)}`)

  const eligibleEvents = swimmer
    ? events.filter((ev) => !ev.gender || ev.gender.toLowerCase() === 'mixed' || ev.gender === swimmer.gender)
    : []

  const eligibleSessionGroups = sessionIds
    .map((sid) => ({
      id: sid,
      events: eligibleEvents.filter((e) => (e.session_id ?? 'main') === sid),
    }))
    .filter((g) => g.events.length > 0)

  const totalEvents = Object.values(swimmerStates).reduce(
    (acc, s) => acc + s.selectedEvents.length, 0
  )

  const setCommitment = async (swimmerId: string, val: 'yes' | 'no') => {
    const st = swimmerStates[swimmerId]
    if (!st) return
    const commitmentId = st.commitmentId

    setSubmitting(swimmerId + val)
    try {
      if (commitmentId) {
        const body: { response: 'committed_yes' | 'committed_no'; notes?: string } = {
          response: val === 'yes' ? 'committed_yes' : 'committed_no',
        }
        if (st.note.trim()) body.notes = st.note.trim()
        await respondToMeetCommitment(api, orgId, meetId, commitmentId, body)
        setSwimmerStates((prev) => ({
          ...prev,
          [swimmerId]: {
            ...prev[swimmerId],
            commitment: val,
            selectedEventSelectionIds: val === 'no' ? {} : prev[swimmerId].selectedEventSelectionIds,
            selectedEvents: val === 'no' ? [] : prev[swimmerId].selectedEvents,
          },
        }))
        setSubmitted((prev) => ({ ...prev, [swimmerId]: true }))
        setTimeout(() => setSubmitted((prev) => ({ ...prev, [swimmerId]: false })), 2500)
      } else {
        setCommitError('Cannot update commitment — no commitment record found. Please refresh and try again.')
        setTimeout(() => setCommitError(null), 4000)
      }
    } catch (err) {
      reportUIError('portal.meetDetail.commitment.update', err, 'Failed to save commitment response.', {
        orgId,
        meetId,
        swimmerId,
        commitmentId,
      })
    } finally {
      setSubmitting(null)
    }
  }

  const setNote = (swimmerId: string, note: string) => {
    setSwimmerStates((prev) => ({
      ...prev,
      [swimmerId]: { ...prev[swimmerId], note },
    }))
  }

  const toggleEvent = (eventId: string) => {
    if (!activeSwimmer) return
    const currentState = swimmerStates[activeSwimmer]
    if (!canEditPortalMeetSelections(commitmentOpen, currentState)) return

    const commitmentId = currentState.commitmentId
    if (!commitmentId) {
      setSelectionError('Cannot update event selections without a commitment record. Please refresh and try again.')
      return
    }

    const requestKey = `${activeSwimmer}:${eventId}`
    setSelectionSubmitting(requestKey)
    setSelectionError(null)

    const existingSelectionId = currentState.selectedEventSelectionIds[eventId]
    const request = existingSelectionId
      ? deleteCommitmentEventSelection(api, orgId, meetId, commitmentId, existingSelectionId).then(() => ({ remove: true as const }))
      : addCommitmentEventSelection(api, orgId, meetId, commitmentId, eventId).then((selection) => ({ remove: false as const, selection }))

    request
      .then((result) => {
        setSwimmerStates((prev) => {
          const swimmerState = prev[activeSwimmer]
          if (!swimmerState) return prev
          return {
            ...prev,
            [activeSwimmer]: result.remove
              ? withRemovedPortalMeetSelection(swimmerState, eventId)
              : withAddedPortalMeetSelection(swimmerState, result.selection),
          }
        })
      })
      .catch((error) => {
        reportUIError('portal.meetDetail.selection.update', error, 'Failed to save event selections.', {
          orgId,
          meetId,
          swimmerId: activeSwimmer,
          commitmentId,
          eventId,
        })
        setSelectionError('Failed to save event selections. Refresh the page to confirm the current saved events before trying again.')
      })
      .finally(() => setSelectionSubmitting(null))
  }

  const dateLabel = formatDateRange(meet.start_date, meet.end_date)

  return (
    <div style={{ maxWidth: '860px' }}>
      {/* Breadcrumb */}
      <nav
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          fontSize: '13px',
          color: '#9A9693',
          marginBottom: '24px',
          animation: 'slideUp 0.3s ease both',
        }}
      >
        <Link href="/portal/meets" style={{ color: '#9A9693', textDecoration: 'none' }}>
          Meets
        </Link>
        <span>/</span>
        <span style={{ color: '#1A1714', fontWeight: 500 }}>{meet.name}</span>
      </nav>

      {/* Meet header */}
      <div
        style={{
          background: '#ffffff',
          borderRadius: '16px',
          border: '1px solid rgba(0,0,0,0.07)',
          boxShadow: '0 2px 12px rgba(0,0,0,0.05)',
          padding: '24px 28px',
          marginBottom: '24px',
          animation: 'slideUp 0.35s ease both',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px', flexWrap: 'wrap' }}>
          <div>
            <h1
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: '26px',
                color: '#1A1714',
                margin: '0 0 8px',
                lineHeight: 1.1,
              }}
            >
              {meet.name}
            </h1>
            <div style={{ display: 'flex', gap: '16px', flexWrap: 'wrap', fontSize: '13px', color: '#6B6866' }}>
              <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                <Icon name="calendar" size={13} style={{ color: '#9A9693' }} />
                {dateLabel}
              </span>
              {meet.location && (
                <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                  <Icon name="map-pin" size={13} style={{ color: '#9A9693' }} />
                  {meet.location}
                </span>
              )}
              <span
                style={{
                  padding: '2px 8px',
                  background: 'rgba(0,0,0,0.05)',
                  borderRadius: '5px',
                  fontFamily: 'var(--font-mono)',
                  fontSize: '11px',
                }}
              >
                {meet.course}
              </span>
            </div>
          </div>

          {/* Deadline urgency */}
          {deadlineDate && (
            <div
              style={{
                background: daysUntilDeadline !== null && daysUntilDeadline <= 3 ? '#fef2f2' : '#fffbeb',
                border: `1px solid ${daysUntilDeadline !== null && daysUntilDeadline <= 3 ? 'rgba(220,38,38,0.2)' : 'rgba(217,119,6,0.2)'}`,
                borderRadius: '10px',
                padding: '10px 16px',
                textAlign: 'center',
              }}
            >
              <div style={{ fontSize: '11px', color: daysUntilDeadline !== null && daysUntilDeadline <= 3 ? '#dc2626' : '#d97706', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '2px' }}>
                Deadline
              </div>
              <div style={{ fontSize: '14px', fontWeight: 700, color: daysUntilDeadline !== null && daysUntilDeadline <= 3 ? '#dc2626' : '#92400e' }}>
                {deadlineDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
              </div>
              <div style={{ fontSize: '12px', fontWeight: 600, color: daysUntilDeadline !== null && daysUntilDeadline <= 3 ? '#dc2626' : '#d97706', marginTop: '2px' }}>
                {daysUntilDeadline === null ? '' : daysUntilDeadline <= 0 ? 'Closed' : daysUntilDeadline === 1 ? '1 day left' : `${daysUntilDeadline} days left`}
              </div>
            </div>
          )}
        </div>
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'minmax(0, 1fr) 280px',
          gap: '18px',
          marginBottom: '24px',
          alignItems: 'start',
          animation: 'slideUp 0.38s ease both',
        }}
      >
        <div
          style={{
            background: '#ffffff',
            borderRadius: '14px',
            border: '1px solid rgba(0,0,0,0.07)',
            boxShadow: '0 2px 12px rgba(0,0,0,0.05)',
            padding: '18px 22px',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px', marginBottom: '12px', flexWrap: 'wrap' }}>
            <div>
              <div style={{ fontSize: '11px', color: '#9A9693', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '5px' }}>
                Meet Info
              </div>
              <div style={{ fontFamily: 'var(--font-display)', fontSize: '19px', color: '#1A1714' }}>
                Family Notes and Session Plan
              </div>
            </div>
            <span
              style={{
                padding: '4px 9px',
                borderRadius: '999px',
                background: 'rgba(235,93,30,0.08)',
                color: '#eb5d1e',
                fontSize: '11px',
                fontFamily: 'var(--font-mono)',
                fontWeight: 600,
              }}
            >
              {events.length} events
            </span>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: '10px', marginBottom: '16px' }}>
            {[
              { label: 'Host', value: meet.host_org ?? '—' },
              { label: 'Course', value: meet.course },
              { label: 'Location', value: meet.location ?? '—' },
            ].map((item) => (
              <div
                key={item.label}
                style={{
                  padding: '11px 12px',
                  borderRadius: '10px',
                  background: 'rgba(0,0,0,0.03)',
                  border: '1px solid rgba(0,0,0,0.05)',
                }}
              >
                <div style={{ fontSize: '10px', color: '#9A9693', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '4px' }}>
                  {item.label}
                </div>
                <div style={{ fontSize: '13px', color: '#1A1714', fontWeight: 600, lineHeight: 1.45 }}>
                  {item.value}
                </div>
              </div>
            ))}
          </div>

          {/* Session groups summary */}
          {sessionIds.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {sessionIds.map((sid) => {
                const evCount = events.filter((e) => (e.session_id ?? 'main') === sid).length
                return (
                  <div
                    key={sid}
                    style={{
                      padding: '12px 14px',
                      borderRadius: '12px',
                      background: 'rgba(235,93,30,0.04)',
                      border: '1px solid rgba(235,93,30,0.14)',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: '10px', alignItems: 'center', flexWrap: 'wrap' }}>
                      <div style={{ fontSize: '14px', fontWeight: 600, color: '#1A1714' }}>
                        {sessionLabel(sid)}
                      </div>
                      <div style={{ fontSize: '11px', color: '#9A9693', fontFamily: 'var(--font-mono)' }}>
                        {evCount} event{evCount !== 1 ? 's' : ''}
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>

        {/* Meet files */}
        <div
          style={{
            background: '#ffffff',
            borderRadius: '14px',
            border: '1px solid rgba(0,0,0,0.07)',
            boxShadow: '0 2px 12px rgba(0,0,0,0.05)',
            padding: '18px',
          }}
        >
          <div style={{ fontSize: '11px', color: '#9A9693', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
            Meet Files
          </div>
          {filesLoading ? (
            <div style={{ padding: '20px 0', display: 'flex', flexDirection: 'column', gap: '10px' }}>
              <div style={{ height: '14px', background: 'rgba(0,0,0,0.05)', borderRadius: '999px' }} />
              <div style={{ height: '14px', width: '76%', background: 'rgba(0,0,0,0.05)', borderRadius: '999px' }} />
            </div>
          ) : filesError ? (
            <div style={{ padding: '12px', borderRadius: '10px', background: 'rgba(235,93,30,0.06)', color: '#b45309', fontSize: '12.5px', lineHeight: 1.55 }}>
              {filesError}
            </div>
          ) : meetFiles.length === 0 ? (
            <div
              style={{
                padding: '32px 12px',
                textAlign: 'center',
                color: '#C0BDBB',
                fontSize: '12.5px',
              }}
            >
              <Icon name="file-text" size={22} style={{ color: '#E0DEDC', marginBottom: '8px', display: 'block', margin: '0 auto 8px' }} />
              No meet files available yet
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {meetFiles.map((file) => (
                <div
                  key={file.id}
                  style={{
                    padding: '12px',
                    borderRadius: '10px',
                    border: '1px solid rgba(0,0,0,0.08)',
                    background: 'rgba(0,0,0,0.02)',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '10px' }}>
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontSize: '13px', fontWeight: 600, color: '#1A1714', wordBreak: 'break-word' }}>{file.filename}</div>
                      <div style={{ marginTop: '4px', fontSize: '11.5px', color: '#9A9693', lineHeight: 1.5 }}>
                        {formatMeetFileType(file.export_type)} · {formatMeetFileSize(file.size_bytes)}
                      </div>
                    </div>
                    {file.download_url ? (
                      <a
                        href={file.download_url}
                        target="_blank"
                        rel="noreferrer"
                        style={{ color: '#eb5d1e', fontSize: '12px', fontWeight: 600, textDecoration: 'none', whiteSpace: 'nowrap' }}
                      >
                        Download
                      </a>
                    ) : (
                      <span style={{ color: '#9A9693', fontSize: '12px', whiteSpace: 'nowrap' }}>
                        {file.status === 'ready' ? 'Link pending' : file.status.replace(/_/g, ' ')}
                      </span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Swimmer tabs */}
      {swimmers.length > 0 && (
        <div
          style={{
            display: 'flex',
            gap: '8px',
            marginBottom: '20px',
            animation: 'slideUp 0.4s ease both',
            flexWrap: 'wrap',
          }}
        >
          {swimmers.map((s, idx) => {
            const st = swimmerStates[s.id]
            const isActive = activeSwimmer === s.id
            const gradient = SWIMMER_GRADIENTS[idx % SWIMMER_GRADIENTS.length]
            const initials = swimmerInitials(s)
            const commitment = st?.commitment ?? 'pending'
            return (
              <button
                key={s.id}
                onClick={() => setActiveSwimmer(s.id)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  padding: '8px 14px',
                  borderRadius: '10px',
                  border: isActive ? '2px solid #eb5d1e' : '1px solid rgba(0,0,0,0.10)',
                  background: isActive ? 'rgba(235,93,30,0.05)' : '#ffffff',
                  cursor: 'pointer',
                  fontFamily: 'var(--font-body)',
                  transition: 'all 120ms ease',
                }}
              >
                <div
                  style={{
                    width: '26px',
                    height: '26px',
                    borderRadius: '50%',
                    background: gradient,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: '#fff',
                    fontSize: '9px',
                    fontWeight: 700,
                    fontFamily: 'var(--font-mono)',
                  }}
                >
                  {initials}
                </div>
                <span style={{ fontSize: '13.5px', fontWeight: 600, color: isActive ? '#eb5d1e' : '#1A1714' }}>
                  {(s.preferred_name ?? s.first_name)}
                </span>
                {/* Commitment badge */}
                <span
                  style={{
                    padding: '2px 6px',
                    borderRadius: '5px',
                    fontSize: '10.5px',
                    fontWeight: 600,
                    background: commitment === 'yes' ? '#ecfdf5' : commitment === 'no' ? '#fef2f2' : '#fffbeb',
                    color: commitment === 'yes' ? '#059669' : commitment === 'no' ? '#dc2626' : '#d97706',
                  }}
                >
                  {commitment === 'yes' ? 'Going' : commitment === 'no' ? 'Not Going' : commitment === 'closed' ? 'Locked' : 'Pending'}
                </span>
              </button>
            )
          })}
        </div>
      )}

      {/* Commitment card for active swimmer */}
      {swimmer && state && (
        <div
          style={{
            background: '#ffffff',
            borderRadius: '14px',
            border: '1px solid rgba(0,0,0,0.07)',
            boxShadow: '0 2px 12px rgba(0,0,0,0.05)',
            overflow: 'hidden',
            marginBottom: '20px',
            animation: 'slideUp 0.45s ease both',
          }}
        >
          {/* Header */}
          <div
            style={{
              padding: '18px 22px',
              borderBottom: '1px solid rgba(0,0,0,0.06)',
              display: 'flex',
              alignItems: 'center',
              gap: '12px',
            }}
          >
            <div
              style={{
                width: '40px',
                height: '40px',
                borderRadius: '50%',
                background: SWIMMER_GRADIENTS[swimmers.indexOf(swimmer) % SWIMMER_GRADIENTS.length],
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#fff',
                fontSize: '14px',
                fontWeight: 700,
                fontFamily: 'var(--font-mono)',
              }}
            >
              {swimmerInitials(swimmer)}
            </div>
            <div>
              <div style={{ fontFamily: 'var(--font-display)', fontSize: '18px', color: '#1A1714' }}>
                {swimmerDisplayName(swimmer)}
              </div>
              <div style={{ fontSize: '12.5px', color: '#9A9693' }}>
                Age {swimmerAge(swimmer)} · {swimmer.gender === 'F' ? 'Female' : swimmer.gender === 'M' ? 'Male' : swimmer.gender}
              </div>
            </div>
            {submitted[swimmer.id] && (
              <div
                style={{
                  marginLeft: 'auto',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '6px',
                  fontSize: '13px',
                  color: '#059669',
                  fontWeight: 500,
                  animation: 'slideUp 0.2s ease both',
                }}
              >
                <Icon name="check-circle" size={15} />
                Saved
              </div>
            )}
          </div>

          {/* Commitment step */}
          <div style={{ padding: '20px 22px', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>
            <div
              style={{
                fontSize: '11px',
                fontFamily: 'var(--font-mono)',
                color: '#9A9693',
                textTransform: 'uppercase',
                letterSpacing: '0.07em',
                marginBottom: '12px',
              }}
            >
              Step 1 — Commitment
            </div>
            <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
              <button
                onClick={() => setCommitment(swimmer.id, 'yes')}
                disabled={submitting !== null || !commitmentOpen || state.commitment === 'closed'}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  padding: '11px 22px',
                  borderRadius: '10px',
                  border: state.commitment === 'yes' ? '2px solid #059669' : '1.5px solid rgba(0,0,0,0.12)',
                  background: state.commitment === 'yes' ? '#ecfdf5' : '#ffffff',
                  color: state.commitment === 'yes' ? '#059669' : '#1A1714',
                  fontWeight: 600,
                  fontSize: '14px',
                  cursor: 'pointer',
                  fontFamily: 'var(--font-body)',
                  transition: 'all 120ms ease',
                  opacity: submitting !== null || !commitmentOpen || state.commitment === 'closed' ? 0.6 : 1,
                }}
              >
                <Icon name="check-circle" size={16} style={{ color: state.commitment === 'yes' ? '#059669' : '#9A9693' }} />
                {submitting === swimmer.id + 'yes' ? 'Saving…' : 'Yes, we\'re going'}
              </button>
              <button
                onClick={() => setCommitment(swimmer.id, 'no')}
                disabled={submitting !== null || !commitmentOpen || state.commitment === 'closed'}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  padding: '11px 22px',
                  borderRadius: '10px',
                  border: state.commitment === 'no' ? '2px solid #dc2626' : '1.5px solid rgba(0,0,0,0.12)',
                  background: state.commitment === 'no' ? '#fef2f2' : '#ffffff',
                  color: state.commitment === 'no' ? '#dc2626' : '#1A1714',
                  fontWeight: 600,
                  fontSize: '14px',
                  cursor: 'pointer',
                  fontFamily: 'var(--font-body)',
                  transition: 'all 120ms ease',
                  opacity: submitting !== null || !commitmentOpen || state.commitment === 'closed' ? 0.6 : 1,
                }}
              >
                <Icon name="x-circle" size={16} style={{ color: state.commitment === 'no' ? '#dc2626' : '#9A9693' }} />
                {submitting === swimmer.id + 'no' ? 'Saving…' : 'Not this time'}
              </button>
            </div>
            {/* Note for coaches */}
            {commitmentOpen && state.commitment !== 'closed' && (
              <div style={{ marginTop: '14px' }}>
                <label
                  htmlFor={`note-${swimmer.id}`}
                  style={{ display: 'block', fontSize: '11px', fontWeight: 600, color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '6px', fontFamily: 'var(--font-mono)' }}
                >
                  Message for coaches <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>(optional)</span>
                </label>
                <textarea
                  id={`note-${swimmer.id}`}
                  value={state.note}
                  onChange={(e) => setNote(swimmer.id, e.target.value)}
                  placeholder="Any notes for the coach — schedule conflicts, injury considerations, preferred events, etc."
                  rows={3}
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: '9px',
                    border: '1.5px solid rgba(0,0,0,0.10)',
                    background: '#fafafa',
                    color: '#1A1714',
                    fontFamily: 'var(--font-body)',
                    fontSize: '13px',
                    lineHeight: 1.55,
                    resize: 'vertical',
                    outline: 'none',
                    transition: 'border-color 0.15s',
                    boxSizing: 'border-box',
                  }}
                  onFocus={(e) => { e.currentTarget.style.borderColor = 'rgba(235,93,30,0.45)' }}
                  onBlur={(e) => { e.currentTarget.style.borderColor = 'rgba(0,0,0,0.10)' }}
                />
                <p style={{ fontSize: '11px', color: '#9A9693', marginTop: '5px', lineHeight: 1.5 }}>
                  This message will be visible to coaches when reviewing commitments.
                </p>
              </div>
            )}
            {/* Show note read-only when commitment is closed */}
            {state.commitment === 'closed' && state.note && (
              <div style={{ marginTop: '12px', padding: '10px 12px', background: 'rgba(0,0,0,0.025)', borderRadius: '8px', border: '1px solid rgba(0,0,0,0.07)' }}>
                <div style={{ fontSize: '10.5px', color: '#9A9693', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '4px' }}>Your note</div>
                <p style={{ fontSize: '13px', color: '#4B5563', lineHeight: 1.55, margin: 0 }}>{state.note}</p>
              </div>
            )}

            {commitError && (
              <div
                style={{
                  marginTop: '12px',
                  padding: '11px 14px',
                  background: 'rgba(220,38,38,0.06)',
                  border: '1px solid rgba(220,38,38,0.18)',
                  borderRadius: '8px',
                  fontSize: '12.5px',
                  color: '#dc2626',
                  lineHeight: 1.5,
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: '8px',
                  animation: 'slideUp 0.2s ease both',
                }}
              >
                <Icon name="alert-circle" size={14} style={{ color: '#dc2626', flexShrink: 0, marginTop: '1px' }} />
                {commitError}
              </div>
            )}
            {selectionError && state.commitment === 'yes' && (
              <div
                style={{
                  marginTop: '12px',
                  padding: '11px 14px',
                  background: 'rgba(220,38,38,0.06)',
                  border: '1px solid rgba(220,38,38,0.18)',
                  borderRadius: '8px',
                  fontSize: '12.5px',
                  color: '#dc2626',
                  lineHeight: 1.5,
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: '8px',
                  animation: 'slideUp 0.2s ease both',
                }}
              >
                <Icon name="alert-circle" size={14} style={{ color: '#dc2626', flexShrink: 0, marginTop: '1px' }} />
                {selectionError}
              </div>
            )}
            {!commitmentOpen && state.commitment !== 'closed' && (
              <div
                style={{
                  marginTop: '12px',
                  padding: '10px 14px',
                  background: 'rgba(59,130,246,0.06)',
                  borderRadius: '8px',
                  fontSize: '12.5px',
                  color: '#4B7BF5',
                  lineHeight: 1.5,
                }}
              >
                Commitment actions unlock when this meet moves into the open family response window.
              </div>
            )}
            {state.commitment === 'closed' && (
              <div
                style={{
                  marginTop: '12px',
                  padding: '10px 14px',
                  background: 'rgba(0,0,0,0.03)',
                  borderRadius: '8px',
                  fontSize: '12.5px',
                  color: '#6B6866',
                  lineHeight: 1.5,
                }}
              >
                This commitment has been reviewed and locked by the coach.
              </div>
            )}
          </div>

          {/* Event selection (only if committed yes) */}
          {commitmentOpen && state.commitment === 'yes' && (
            <div style={{ padding: '20px 22px' }}>
              <div
                style={{
                  fontSize: '11px',
                  fontFamily: 'var(--font-mono)',
                  color: '#9A9693',
                  textTransform: 'uppercase',
                  letterSpacing: '0.07em',
                  marginBottom: '6px',
                }}
              >
                Step 2 — Select Events
              </div>
              <p style={{ margin: '0 0 16px', fontSize: '13px', color: '#9A9693', lineHeight: 1.5 }}>
                Select the events {swimmer.preferred_name ?? swimmer.first_name} would like to enter. Coach may adjust selections before the deadline.
              </p>

              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {eligibleSessionGroups.length === 0 ? (
                  <div style={{ padding: '24px', textAlign: 'center', fontSize: '13.5px', color: '#9A9693' }}>
                    No eligible events found for this swimmer.
                  </div>
                ) : (
                  eligibleSessionGroups.map((group) => (
                    <div key={group.id} style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          gap: '10px',
                          padding: '10px 12px',
                          borderRadius: '10px',
                          background: 'rgba(0,0,0,0.03)',
                        }}
                      >
                        <div style={{ fontSize: '13px', fontWeight: 600, color: '#1A1714' }}>
                          {sessionLabel(group.id)}
                        </div>
                        <span
                          style={{
                            padding: '3px 8px',
                            borderRadius: '999px',
                            background: 'rgba(235,93,30,0.08)',
                            color: '#eb5d1e',
                            fontSize: '10.5px',
                            fontFamily: 'var(--font-mono)',
                            fontWeight: 600,
                          }}
                        >
                          {group.events.length} options
                        </span>
                      </div>

                      {group.events.map((ev) => {
                        const isSelected = state.selectedEvents.includes(ev.id)
                        const label = eventLabel(ev)
                        const ageGroup = eventAgeGroup(ev)

                        return (
                          <div
                            key={ev.id}
                            onClick={() => toggleEvent(ev.id)}
                            style={{
                              display: 'flex',
                              alignItems: 'center',
                              gap: '12px',
                              padding: '12px 14px',
                              borderRadius: '10px',
                              border: isSelected ? '2px solid #eb5d1e' : '1.5px solid rgba(0,0,0,0.09)',
                              background: isSelected ? 'rgba(235,93,30,0.04)' : '#fafafa',
                              cursor: selectionSubmitting ? 'progress' : 'pointer',
                              opacity: selectionSubmitting === `${swimmer.id}:${ev.id}` ? 0.65 : 1,
                              transition: 'all 120ms ease',
                            }}
                            onMouseEnter={(e) => {
                              if (!isSelected) e.currentTarget.style.borderColor = 'rgba(235,93,30,0.35)'
                            }}
                            onMouseLeave={(e) => {
                              if (!isSelected) e.currentTarget.style.borderColor = 'rgba(0,0,0,0.09)'
                            }}
                          >
                            {/* Checkbox */}
                            <div
                              style={{
                                width: '20px',
                                height: '20px',
                                borderRadius: '5px',
                                border: isSelected ? 'none' : '2px solid rgba(0,0,0,0.15)',
                                background: isSelected ? '#eb5d1e' : 'transparent',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                flexShrink: 0,
                                transition: 'all 120ms ease',
                              }}
                            >
                              {isSelected && <Icon name="check" size={12} style={{ color: '#fff' }} />}
                            </div>

                            {/* Event number */}
                            <span
                              style={{
                                width: '28px',
                                fontSize: '11.5px',
                                fontFamily: 'var(--font-mono)',
                                color: '#9A9693',
                                flexShrink: 0,
                                textAlign: 'center',
                              }}
                            >
                              #{ev.event_number ?? '—'}
                            </span>

                            {/* Event name */}
                            <div style={{ flex: 1, minWidth: 0 }}>
                              <div style={{ fontSize: '14px', fontWeight: 600, color: '#1A1714' }}>
                                {label}
                              </div>
                              <div style={{ fontSize: '12px', color: '#9A9693' }}>
                                {ageGroup}
                                {ev.requires_qualification && (
                                  <span style={{ marginLeft: '8px' }}>Qualification required</span>
                                )}
                              </div>
                            </div>

                            {/* Entry limit hint */}
                            {ev.entry_limit != null && (
                              <div style={{ textAlign: 'right', flexShrink: 0 }}>
                                <span style={{ fontSize: '12px', color: '#C0BDBB', fontStyle: 'italic' }}>
                                  Limit {ev.entry_limit}
                                </span>
                              </div>
                            )}
                          </div>
                        )
                      })}
                    </div>
                  ))
                )}
              </div>

              {/* Summary */}
              {state.selectedEvents.length > 0 && (
                <div
                  style={{
                    marginTop: '16px',
                    padding: '12px 16px',
                    background: 'rgba(235,93,30,0.06)',
                    borderRadius: '10px',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    animation: 'slideUp 0.2s ease both',
                  }}
                >
                  <span style={{ fontSize: '13.5px', color: '#1A1714', fontWeight: 500 }}>
                    {state.selectedEvents.length} event{state.selectedEvents.length !== 1 ? 's' : ''} selected
                  </span>
                </div>
              )}

              {/* Coach override notice */}
              <div
                style={{
                  marginTop: '12px',
                  padding: '10px 14px',
                  background: 'rgba(0,0,0,0.03)',
                  borderRadius: '8px',
                  fontSize: '12.5px',
                  color: '#9A9693',
                  lineHeight: 1.5,
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: '8px',
                }}
              >
                <Icon name="info" size={13} style={{ color: '#C0BDBB', marginTop: '1px', flexShrink: 0 }} />
                Coaches may adjust event selections based on eligibility and team strategy. You&apos;ll see any changes reflected here.
              </div>
            </div>
          )}

          {/* Not going state */}
          {state.commitment === 'no' && (
            <div
              style={{
                padding: '20px 22px',
                display: 'flex',
                alignItems: 'center',
                gap: '12px',
              }}
            >
              <Icon name="x-circle" size={18} style={{ color: '#dc2626' }} />
              <span style={{ fontSize: '14px', color: '#6B6866' }}>
                {swimmer.preferred_name ?? swimmer.first_name} is not attending this meet. You can change this before the deadline.
              </span>
            </div>
          )}
        </div>
      )}

      {/* No swimmers message */}
      {swimmers.length === 0 && (
        <div
          style={{
            background: '#ffffff',
            borderRadius: '14px',
            border: '1px solid rgba(0,0,0,0.07)',
            padding: '48px',
            textAlign: 'center',
            marginBottom: '20px',
          }}
        >
          <div style={{ fontSize: '15px', color: '#9A9693', fontWeight: 500 }}>No swimmers found for your household</div>
          <div style={{ fontSize: '13px', color: '#C0BDBB', marginTop: '6px' }}>
            Contact your club admin to link swimmers to your account.
          </div>
        </div>
      )}

      {/* Fee summary */}
      {totalEvents > 0 && (
        <div
          style={{
            background: '#ffffff',
            borderRadius: '12px',
            border: '1px solid rgba(0,0,0,0.07)',
            padding: '16px 22px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            animation: 'slideUp 0.5s ease both',
          }}
        >
          <div>
            <div style={{ fontSize: '13px', color: '#9A9693' }}>Total estimated entry fee</div>
            <div style={{ fontSize: '11.5px', color: '#C0BDBB', marginTop: '2px' }}>
              {totalEvents} event{totalEvents !== 1 ? 's' : ''} selected across all swimmers
            </div>
          </div>
          <div
            style={{
              fontSize: '18px',
              fontWeight: 700,
              color: '#eb5d1e',
              fontFamily: 'var(--font-mono)',
            }}
          >
            {totalEvents} event{totalEvents !== 1 ? 's' : ''}
          </div>
        </div>
      )}

      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(12px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  )
}
