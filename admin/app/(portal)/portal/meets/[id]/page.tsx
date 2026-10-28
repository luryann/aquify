'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { parseBackendDate } from '@/lib/dates'
import { ApiError, useApiClient } from '@/lib/api'
import { reportUIError, uiErrorMessage } from '@/lib/frontend-telemetry'
import { isSwimmerEligibleForEvent } from '@/lib/event-eligibility'
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
  const dob = parseBackendDate(s.date_of_birth)
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
    <div className="page">
      <div style={{ height: '20px', width: '160px', background: 'rgba(0,0,0,0.06)', borderRadius: '6px', marginBottom: '24px' }} />
      <div
        className="card" style={{ padding: '24px 24px', marginBottom: '24px' }}
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
          setFilesError(uiErrorMessage(error, 'Couldn’t load meet files. Try again.'))
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
      <div className="page">
        <nav style={{ marginBottom: '20px', fontSize: 'var(--fs-base)' }}>
          <Link href="/portal/meets" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>
            Meets
          </Link>
        </nav>
        {loadError && (
          <div
            style={{
              background: 'var(--warning-bg)',
              border: '1px solid #fdba74',
              borderRadius: '12px',
              padding: '12px 16px',
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              gap: '12px',
              marginBottom: '20px',
            }}
          >
            <div style={{ display: 'flex', gap: '12px', alignItems: 'flex-start' }}>
              <Icon name="alert-circle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
              <p style={{ margin: 0, fontSize: 'var(--fs-sm)', color: 'var(--warning)', lineHeight: 1.5 }}>{loadError}</p>
            </div>
            <button
              type="button"
              onClick={() => setReloadKey((key) => key + 1)}
              style={{ background: 'var(--bg-elevated)', border: '1px solid #fdba74', borderRadius: '8px', cursor: 'pointer', color: 'var(--warning)', fontSize: 'var(--fs-sm)', fontWeight: 600, padding: '8px 16px', whiteSpace: 'nowrap' }}
            >
              Retry
            </button>
          </div>
        )}
        <div
          className="card" style={{ padding: '24px' }}
        >
          <h1 className="page-title">
            {loadError ? 'Meet details are unavailable right now.' : 'Meet not found'}
          </h1>
          <p className="page-subtitle">
            {loadError
              ? 'Refresh, or try again shortly.'
              : <>No meet found for <span style={{ fontFamily: 'var(--font-body)' }}>{meetId || 'this route'}</span>.</>}
          </p>
          <Link href="/portal/meets" style={{ fontSize: 'var(--fs-base)', fontWeight: 600 }}>
            Back to all meets
          </Link>
        </div>
      </div>
    )
  }

  const deadlineStr = meet.commitment_deadline ?? meet.entry_deadline
  const deadlineDate = deadlineStr ? parseBackendDate(deadlineStr) : null
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
  const sessionById = new Map(sessions.map((s) => [s.id, s]))
  const fmtClockTime = (value?: string) => {
    if (!value) return null
    const match = value.match(/^(\d{1,2}):(\d{2})(?::\d{2})?$/)
    if (!match) return value
    const d = new Date(2000, 0, 1, Number(match[1]), Number(match[2]))
    return d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
  }
  const sessionScheduleLabel = (sid: string) => {
    const s = sessionById.get(sid)
    if (!s) return null
    const date = s.session_date
      ? new Date(`${s.session_date}T00:00:00`).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
      : null
    const warmup = fmtClockTime(s.warmup_time)
    const start = fmtClockTime(s.start_time)
    const parts = [date, warmup ? `Warm-up ${warmup}` : null, start ? `Starts ${start}` : null].filter(Boolean)
    return parts.length > 0 ? parts.join(' · ') : null
  }

  const eligibleEvents = swimmer
    ? events.filter((ev) => isSwimmerEligibleForEvent(swimmer, ev, meet?.start_date))
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
            savedNote: st.note.trim(),
            selectedEventSelectionIds: val === 'no' ? {} : prev[swimmerId].selectedEventSelectionIds,
            selectedEvents: val === 'no' ? [] : prev[swimmerId].selectedEvents,
          },
        }))
        setSubmitted((prev) => ({ ...prev, [swimmerId]: true }))
        setTimeout(() => setSubmitted((prev) => ({ ...prev, [swimmerId]: false })), 2500)
      } else {
        setCommitError('Couldn’t update your response because this meet’s commitment wasn’t found. Refresh the page and try again.')
        setTimeout(() => setCommitError(null), 4000)
      }
    } catch (err) {
      reportUIError('portal.meetDetail.commitment.update', err, 'Couldn’t save commitment response. Try again.', {
        orgId,
        meetId,
        swimmerId,
        commitmentId,
      })
    } finally {
      setSubmitting(null)
    }
  }

  // Notes ride along with the yes/no response, so saving a note edited after
  // responding re-sends the current response with the new message.
  const saveNote = async (swimmerId: string) => {
    const st = swimmerStates[swimmerId]
    if (!st?.commitmentId || (st.commitment !== 'yes' && st.commitment !== 'no')) return
    setSubmitting(swimmerId + 'note')
    try {
      const note = st.note.trim()
      await respondToMeetCommitment(api, orgId, meetId, st.commitmentId, {
        response: st.commitment === 'yes' ? 'committed_yes' : 'committed_no',
        ...(note ? { notes: note } : {}),
      })
      setSwimmerStates((prev) => ({ ...prev, [swimmerId]: { ...prev[swimmerId], savedNote: note } }))
      setSubmitted((prev) => ({ ...prev, [swimmerId]: true }))
      setTimeout(() => setSubmitted((prev) => ({ ...prev, [swimmerId]: false })), 2500)
    } catch (err) {
      reportUIError('portal.meetDetail.commitment.note', err, 'Couldn’t save message for coaches. Try again.', {
        orgId,
        meetId,
        swimmerId,
        commitmentId: st.commitmentId,
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
      setSelectionError('Couldn’t update events because this meet’s commitment wasn’t found. Refresh the page and try again.')
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
        reportUIError('portal.meetDetail.selection.update', error, 'Couldn’t save event selections. Try again.', {
          orgId,
          meetId,
          swimmerId: activeSwimmer,
          commitmentId,
          eventId,
        })
        setSelectionError('Couldn’t save event selections. Refresh to see what was saved before trying again.')
      })
      .finally(() => setSelectionSubmitting(null))
  }

  const dateLabel = formatDateRange(meet.start_date, meet.end_date)

  return (
    <div className="page">
      {/* Breadcrumb */}
      <nav
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          fontSize: 'var(--fs-base)',
          color: 'var(--text-muted)',
          marginBottom: '24px',
          animation: 'slideUp 0.3s ease both',
        }}
      >
        <Link href="/portal/meets" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>
          Meets
        </Link>
        <span>/</span>
        <span style={{ color: 'var(--text)', fontWeight: 500 }}>{meet.name}</span>
      </nav>

      {/* Meet header */}
      <div
        className="card" style={{ padding: '24px 24px', marginBottom: '24px', animation: 'slideUp 0.35s ease both' }}
      >
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px', flexWrap: 'wrap' }}>
          <div>
            <h1 className="page-title">
              {meet.name}
            </h1>
            <div style={{ display: 'flex', gap: '16px', flexWrap: 'wrap', fontSize: 'var(--fs-base)', color: 'var(--text-subtle)' }}>
              <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                <Icon name="calendar" size={13} style={{ color: 'var(--text-muted)' }} />
                {dateLabel}
              </span>
              {meet.location && (
                <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                  <Icon name="map-pin" size={13} style={{ color: 'var(--text-muted)' }} />
                  {meet.location}
                </span>
              )}
              <span
                style={{
                  padding: '2px 8px',
                  background: 'rgba(0,0,0,0.05)',
                  borderRadius: '6px',
                  fontFamily: 'var(--font-body)',
                  fontSize: 'var(--fs-xs)',
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
                background: daysUntilDeadline !== null && daysUntilDeadline <= 3 ? 'var(--error-bg)' : 'var(--warning-bg)',
                border: `1px solid ${daysUntilDeadline !== null && daysUntilDeadline <= 3 ? 'rgba(220,38,38,0.2)' : 'rgba(217,119,6,0.2)'}`,
                borderRadius: '12px',
                padding: '12px 16px',
                textAlign: 'center',
              }}
            >
              <div style={{ fontSize: 'var(--fs-sm)', color: daysUntilDeadline !== null && daysUntilDeadline <= 3 ? 'var(--error)' : 'var(--warning)', fontFamily: 'var(--font-body)', marginBottom: '2px' }}>
                Deadline
              </div>
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 700, color: daysUntilDeadline !== null && daysUntilDeadline <= 3 ? 'var(--error)' : '#92400e' }}>
                {deadlineDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
              </div>
              <div style={{ fontSize: 'var(--fs-sm)', fontWeight: 600, color: daysUntilDeadline !== null && daysUntilDeadline <= 3 ? 'var(--error)' : 'var(--warning)', marginTop: '2px' }}>
                {daysUntilDeadline === null ? '' : daysUntilDeadline <= 0 ? 'Closed' : daysUntilDeadline === 1 ? '1 day left' : `${daysUntilDeadline} days left`}
              </div>
            </div>
          )}
        </div>
      </div>

      <div
        className="split-aside"
        style={{
          gap: '20px',
          marginBottom: '24px',
          alignItems: 'start',
          animation: 'slideUp 0.38s ease both',
        }}
      >
        <div
          className="card" style={{ padding: '20px 24px' }}
        >
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px', marginBottom: '12px', flexWrap: 'wrap' }}>
            <div>
              <div style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: 'var(--text)' }}>
                Family Notes and Session Plan
              </div>
            </div>
            <span
              style={{
                padding: '4px 8px',
                borderRadius: '999px',
                background: 'rgba(235,93,30,0.08)',
                color: 'var(--accent)',
                fontSize: 'var(--fs-xs)',
                fontFamily: 'var(--font-body)',
                fontWeight: 600,
              }}
            >
              {events.length} events
            </span>
          </div>

          <div className="cols-3" style={{ gap: '12px', marginBottom: '16px' }}>
            {[
              { label: 'Host', value: meet.host_org ?? '—' },
              { label: 'Course', value: meet.course },
              { label: 'Location', value: meet.location ?? '—' },
            ].map((item) => (
              <div
                key={item.label}
                style={{
                  padding: '12px 12px',
                  borderRadius: '12px',
                  background: 'rgba(0,0,0,0.03)',
                  border: '1px solid rgba(0,0,0,0.05)',
                }}
              >
                <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginBottom: '4px' }}>
                  {item.label}
                </div>
                <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text)', fontWeight: 600, lineHeight: 1.45 }}>
                  {item.value}
                </div>
              </div>
            ))}
          </div>

          {/* Session groups summary */}
          {sessionIds.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {sessionIds.map((sid) => {
                const evCount = events.filter((e) => (e.session_id ?? 'main') === sid).length
                return (
                  <div
                    key={sid}
                    style={{
                      padding: '12px 16px',
                      borderRadius: '12px',
                      background: 'rgba(235,93,30,0.04)',
                      border: '1px solid rgba(235,93,30,0.14)',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
                      <div>
                        <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--text)' }}>
                          {sessionLabel(sid)}
                        </div>
                        {sessionScheduleLabel(sid) ? (
                          <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', marginTop: '2px' }}>
                            {sessionScheduleLabel(sid)}
                          </div>
                        ) : null}
                      </div>
                      <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
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
          className="card" style={{ padding: '20px' }}
        >
          <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--text)', fontFamily: 'var(--font-body)', marginBottom: '8px' }}>
            Meet files
          </div>
          {filesLoading ? (
            <div style={{ padding: '20px 0', display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <div style={{ height: '14px', background: 'rgba(0,0,0,0.05)', borderRadius: '999px' }} />
              <div style={{ height: '14px', width: '76%', background: 'rgba(0,0,0,0.05)', borderRadius: '999px' }} />
            </div>
          ) : filesError ? (
            <div style={{ padding: '12px', borderRadius: '12px', background: 'rgba(235,93,30,0.06)', color: '#b45309', fontSize: 'var(--fs-sm)', lineHeight: 1.55 }}>
              {filesError}
            </div>
          ) : meetFiles.length === 0 ? (
            <div
              style={{
                padding: '32px 12px',
                textAlign: 'center',
                color: 'var(--border-strong)',
                fontSize: 'var(--fs-sm)',
              }}
            >
              <Icon name="file-text" size={22} style={{ color: '#E0DEDC', marginBottom: '8px', display: 'block', margin: '0 auto 8px' }} />
              No meet files yet. Heat sheets and results will appear here once your coach posts them.
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {meetFiles.map((file) => (
                <div
                  key={file.id}
                  style={{
                    padding: '12px',
                    borderRadius: '12px',
                    border: '1px solid rgba(0,0,0,0.08)',
                    background: 'rgba(0,0,0,0.02)',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '12px' }}>
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--text)', wordBreak: 'break-word' }}>{file.filename}</div>
                      <div style={{ marginTop: '4px', fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', lineHeight: 1.5 }}>
                        {formatMeetFileType(file.export_type)} · {formatMeetFileSize(file.size_bytes)}
                      </div>
                    </div>
                    {file.download_url ? (
                      <a
                        href={file.download_url}
                        target="_blank"
                        rel="noreferrer"
                        style={{ color: 'var(--accent)', fontSize: 'var(--fs-sm)', fontWeight: 600, textDecoration: 'none', whiteSpace: 'nowrap' }}
                      >
                        Download
                      </a>
                    ) : (
                      <span style={{ color: 'var(--text-muted)', fontSize: 'var(--fs-sm)', whiteSpace: 'nowrap' }}>
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
                  padding: '8px 16px',
                  borderRadius: '12px',
                  border: isActive ? '2px solid var(--accent)' : '1px solid rgba(0,0,0,0.10)',
                  background: isActive ? 'rgba(235,93,30,0.05)' : 'var(--bg-elevated)',
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
                    color: 'var(--bg-elevated)',
                    fontSize: 'var(--fs-2xs)',
                    fontWeight: 700,
                    fontFamily: 'var(--font-body)',
                  }}
                >
                  {initials}
                </div>
                <span style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: isActive ? 'var(--accent)' : 'var(--text)' }}>
                  {(s.preferred_name ?? s.first_name)}
                </span>
                {/* Commitment badge */}
                <span
                  style={{
                    padding: '2px 6px',
                    borderRadius: '6px',
                    fontSize: 'var(--fs-2xs)',
                    fontWeight: 600,
                    background: commitment === 'yes' ? 'var(--success-bg)' : commitment === 'no' ? 'var(--error-bg)' : 'var(--warning-bg)',
                    color: commitment === 'yes' ? 'var(--success)' : commitment === 'no' ? 'var(--error)' : 'var(--warning)',
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
          className="card" style={{ overflow: 'hidden', marginBottom: '20px', animation: 'slideUp 0.45s ease both' }}
        >
          {/* Header */}
          <div
            style={{
              padding: '20px 24px',
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
                color: 'var(--bg-elevated)',
                fontSize: 'var(--fs-md)',
                fontWeight: 700,
                fontFamily: 'var(--font-body)',
              }}
            >
              {swimmerInitials(swimmer)}
            </div>
            <div>
              <div style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: 'var(--text)' }}>
                {swimmerDisplayName(swimmer)}
              </div>
              <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>
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
                  fontSize: 'var(--fs-base)',
                  color: 'var(--success)',
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
          <div style={{ padding: '20px 24px', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>
            <div
              style={{
                fontSize: 'var(--fs-md)', fontWeight: 600,
                fontFamily: 'var(--font-body)',
                color: 'var(--text)',
                marginBottom: '12px',
              }}
            >
              Are you coming?
            </div>
            <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
              <button
                onClick={() => setCommitment(swimmer.id, 'yes')}
                disabled={submitting !== null || !commitmentOpen || state.commitment === 'closed'}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  padding: '12px 24px',
                  borderRadius: '12px',
                  border: state.commitment === 'yes' ? '2px solid #059669' : '1.5px solid rgba(0,0,0,0.12)',
                  background: state.commitment === 'yes' ? 'var(--success-bg)' : 'var(--bg-elevated)',
                  color: state.commitment === 'yes' ? 'var(--success)' : 'var(--text)',
                  fontWeight: 600,
                  fontSize: 'var(--fs-md)',
                  cursor: 'pointer',
                  fontFamily: 'var(--font-body)',
                  transition: 'all 120ms ease',
                  opacity: submitting !== null || !commitmentOpen || state.commitment === 'closed' ? 0.6 : 1,
                }}
              >
                <Icon name="check-circle" size={16} style={{ color: state.commitment === 'yes' ? 'var(--success)' : 'var(--text-muted)' }} />
                {submitting === swimmer.id + 'yes' ? 'Saving…' : 'Yes, we\'re going'}
              </button>
              <button
                onClick={() => setCommitment(swimmer.id, 'no')}
                disabled={submitting !== null || !commitmentOpen || state.commitment === 'closed'}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  padding: '12px 24px',
                  borderRadius: '12px',
                  border: state.commitment === 'no' ? '2px solid #dc2626' : '1.5px solid rgba(0,0,0,0.12)',
                  background: state.commitment === 'no' ? 'var(--error-bg)' : 'var(--bg-elevated)',
                  color: state.commitment === 'no' ? 'var(--error)' : 'var(--text)',
                  fontWeight: 600,
                  fontSize: 'var(--fs-md)',
                  cursor: 'pointer',
                  fontFamily: 'var(--font-body)',
                  transition: 'all 120ms ease',
                  opacity: submitting !== null || !commitmentOpen || state.commitment === 'closed' ? 0.6 : 1,
                }}
              >
                <Icon name="x-circle" size={16} style={{ color: state.commitment === 'no' ? 'var(--error)' : 'var(--text-muted)' }} />
                {submitting === swimmer.id + 'no' ? 'Saving…' : 'Not this time'}
              </button>
            </div>
            {/* Note for coaches */}
            {commitmentOpen && state.commitment !== 'closed' && (
              <div style={{ marginTop: '16px' }}>
                <label
                  htmlFor={`note-${swimmer.id}`}
                  style={{ display: 'block', fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', marginBottom: '6px', fontFamily: 'var(--font-body)' }}
                >
                  Message for coaches <span style={{ fontWeight: 400 }}>(optional)</span>
                </label>
                <textarea
                  id={`note-${swimmer.id}`}
                  value={state.note}
                  onChange={(e) => setNote(swimmer.id, e.target.value)}
                  placeholder="Any notes for the coach — schedule conflicts, injury considerations, preferred events, etc."
                  rows={3}
                  style={{
                    width: '100%',
                    padding: '12px 12px',
                    borderRadius: '8px',
                    border: '1.5px solid rgba(0,0,0,0.10)',
                    background: '#fafafa',
                    color: 'var(--text)',
                    fontFamily: 'var(--font-body)',
                    fontSize: 'var(--fs-base)',
                    lineHeight: 1.55,
                    resize: 'vertical',
                    outline: 'none',
                    transition: 'border-color 0.15s',
                    boxSizing: 'border-box',
                  }}
                  onFocus={(e) => { e.currentTarget.style.borderColor = 'rgba(235,93,30,0.45)' }}
                  onBlur={(e) => { e.currentTarget.style.borderColor = 'rgba(0,0,0,0.10)' }}
                />
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px', marginTop: '4px' }}>
                  <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', lineHeight: 1.5, margin: 0 }}>
                    {state.commitment === 'yes' || state.commitment === 'no'
                      ? 'This message will be visible to coaches when reviewing commitments.'
                      : 'Sent to coaches with your response above.'}
                  </p>
                  {(state.commitment === 'yes' || state.commitment === 'no') && state.note.trim() !== state.savedNote && (
                    <button
                      type="button"
                      onClick={() => { void saveNote(swimmer.id) }}
                      disabled={submitting === swimmer.id + 'note'}
                      style={{
                        flexShrink: 0,
                        padding: '6px 12px',
                        borderRadius: '8px',
                        border: 'none',
                        background: 'var(--accent)',
                        color: '#fff',
                        fontSize: 'var(--fs-sm)',
                        fontWeight: 600,
                        fontFamily: 'var(--font-body)',
                        cursor: 'pointer',
                        opacity: submitting === swimmer.id + 'note' ? 0.6 : 1,
                      }}
                    >
                      {submitting === swimmer.id + 'note' ? 'Saving…' : 'Save message'}
                    </button>
                  )}
                </div>
              </div>
            )}
            {/* Show note read-only when commitment is closed */}
            {state.commitment === 'closed' && state.note && (
              <div style={{ marginTop: '12px', padding: '12px 12px', background: 'rgba(0,0,0,0.025)', borderRadius: '8px', border: '1px solid rgba(0,0,0,0.07)' }}>
                <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginBottom: '4px' }}>Your note</div>
                <p style={{ fontSize: 'var(--fs-base)', color: '#4B5563', lineHeight: 1.55, margin: 0 }}>{state.note}</p>
              </div>
            )}

            {commitError && (
              <div
                style={{
                  marginTop: '12px',
                  padding: '12px 16px',
                  background: 'rgba(220,38,38,0.06)',
                  border: '1px solid rgba(220,38,38,0.18)',
                  borderRadius: '8px',
                  fontSize: 'var(--fs-sm)',
                  color: 'var(--error)',
                  lineHeight: 1.5,
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: '8px',
                  animation: 'slideUp 0.2s ease both',
                }}
              >
                <Icon name="alert-circle" size={14} style={{ color: 'var(--error)', flexShrink: 0, marginTop: '1px' }} />
                {commitError}
              </div>
            )}
            {selectionError && state.commitment === 'yes' && (
              <div
                style={{
                  marginTop: '12px',
                  padding: '12px 16px',
                  background: 'rgba(220,38,38,0.06)',
                  border: '1px solid rgba(220,38,38,0.18)',
                  borderRadius: '8px',
                  fontSize: 'var(--fs-sm)',
                  color: 'var(--error)',
                  lineHeight: 1.5,
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: '8px',
                  animation: 'slideUp 0.2s ease both',
                }}
              >
                <Icon name="alert-circle" size={14} style={{ color: 'var(--error)', flexShrink: 0, marginTop: '1px' }} />
                {selectionError}
              </div>
            )}
            {!commitmentOpen && state.commitment !== 'closed' && (
              <div
                style={{
                  marginTop: '12px',
                  padding: '12px 16px',
                  background: 'rgba(59,130,246,0.06)',
                  borderRadius: '8px',
                  fontSize: 'var(--fs-sm)',
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
                  padding: '12px 16px',
                  background: 'rgba(0,0,0,0.03)',
                  borderRadius: '8px',
                  fontSize: 'var(--fs-sm)',
                  color: 'var(--text-subtle)',
                  lineHeight: 1.5,
                }}
              >
                This commitment has been reviewed and locked by the coach.
              </div>
            )}
          </div>

          {/* Event selection (only if committed yes) */}
          {commitmentOpen && state.commitment === 'yes' && (
            <div style={{ padding: '20px 24px' }}>
              <div
                style={{
                  fontSize: 'var(--fs-md)', fontWeight: 600,
                  fontFamily: 'var(--font-body)',
                  color: 'var(--text)',
                  marginBottom: '6px',
                }}
              >
                Pick events
              </div>
              <p style={{ margin: '0 0 16px', fontSize: 'var(--fs-base)', color: 'var(--text-muted)', lineHeight: 1.5 }}>
                Select the events {swimmer.preferred_name ?? swimmer.first_name} would like to enter. Coach may adjust selections before the deadline.
              </p>

              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {eligibleSessionGroups.length === 0 ? (
                  <div style={{ padding: '24px', textAlign: 'center', fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>
                    Your coach hasn&apos;t opened event sign-ups for {swimmer.preferred_name ?? swimmer.first_name} yet. Check back before the deadline.
                  </div>
                ) : (
                  eligibleSessionGroups.map((group) => (
                    <div key={group.id} style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          gap: '12px',
                          padding: '12px 12px',
                          borderRadius: '12px',
                          background: 'rgba(0,0,0,0.03)',
                        }}
                      >
                        <div style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--text)' }}>
                          {sessionLabel(group.id)}
                        </div>
                        <span
                          style={{
                            padding: '4px 8px',
                            borderRadius: '999px',
                            background: 'rgba(235,93,30,0.08)',
                            color: 'var(--accent)',
                            fontSize: 'var(--fs-2xs)',
                            fontFamily: 'var(--font-body)',
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
                              padding: '12px 16px',
                              borderRadius: '12px',
                              border: isSelected ? '2px solid var(--accent)' : '1.5px solid rgba(0,0,0,0.09)',
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
                                borderRadius: '6px',
                                border: isSelected ? 'none' : '2px solid rgba(0,0,0,0.15)',
                                background: isSelected ? 'var(--accent)' : 'transparent',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                flexShrink: 0,
                                transition: 'all 120ms ease',
                              }}
                            >
                              {isSelected && <Icon name="check" size={12} style={{ color: 'var(--bg-elevated)' }} />}
                            </div>

                            {/* Event number */}
                            <span
                              style={{
                                width: '28px',
                                fontSize: 'var(--fs-xs)',
                                fontFamily: 'var(--font-body)',
                                color: 'var(--text-muted)',
                                flexShrink: 0,
                                textAlign: 'center',
                              }}
                            >
                              #{ev.event_number ?? '—'}
                            </span>

                            {/* Event name */}
                            <div style={{ flex: 1, minWidth: 0 }}>
                              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--text)' }}>
                                {label}
                              </div>
                              <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>
                                {ageGroup}
                                {ev.requires_qualification && (
                                  <span style={{ marginLeft: '8px' }}>Qualification required</span>
                                )}
                              </div>
                            </div>

                            {/* Entry limit hint */}
                            {ev.entry_limit != null && (
                              <div style={{ textAlign: 'right', flexShrink: 0 }}>
                                <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--border-strong)', fontStyle: 'italic' }}>
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
                    borderRadius: '12px',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    animation: 'slideUp 0.2s ease both',
                  }}
                >
                  <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text)', fontWeight: 500 }}>
                    {state.selectedEvents.length} event{state.selectedEvents.length !== 1 ? 's' : ''} selected
                  </span>
                </div>
              )}

              {/* Coach override notice */}
              <div
                style={{
                  marginTop: '12px',
                  padding: '12px 16px',
                  background: 'rgba(0,0,0,0.03)',
                  borderRadius: '8px',
                  fontSize: 'var(--fs-sm)',
                  color: 'var(--text-muted)',
                  lineHeight: 1.5,
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: '8px',
                }}
              >
                <Icon name="info" size={13} style={{ color: 'var(--border-strong)', marginTop: '1px', flexShrink: 0 }} />
                Coaches may adjust event selections based on eligibility and team strategy. You&apos;ll see any changes reflected here.
              </div>
            </div>
          )}

          {/* Not going state */}
          {state.commitment === 'no' && (
            <div
              style={{
                padding: '20px 24px',
                display: 'flex',
                alignItems: 'center',
                gap: '12px',
              }}
            >
              <Icon name="x-circle" size={18} style={{ color: 'var(--error)' }} />
              <span style={{ fontSize: 'var(--fs-md)', color: 'var(--text-subtle)' }}>
                {swimmer.preferred_name ?? swimmer.first_name} is not attending this meet. You can change this before the deadline.
              </span>
            </div>
          )}
        </div>
      )}

      {/* No swimmers message */}
      {swimmers.length === 0 && (
        <div
          className="card" style={{ padding: '48px', textAlign: 'center', marginBottom: '20px' }}
        >
          <div style={{ fontSize: 'var(--fs-md)', color: 'var(--text-muted)', fontWeight: 500 }}>No swimmers found for your household</div>
          <div style={{ fontSize: 'var(--fs-base)', color: 'var(--border-strong)', marginTop: '6px' }}>
            Contact your club admin to link swimmers to your account.
          </div>
        </div>
      )}

      {/* Fee summary */}
      {totalEvents > 0 && (
        <div
          className="card" style={{ padding: '16px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', animation: 'slideUp 0.5s ease both' }}
        >
          <div>
            <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>Total estimated entry fee</div>
            <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--border-strong)', marginTop: '2px' }}>
              {totalEvents} event{totalEvents !== 1 ? 's' : ''} selected across all swimmers
            </div>
          </div>
          <div
            style={{
              fontSize: 'var(--fs-xl)',
              fontWeight: 700,
              color: 'var(--accent)',
              fontFamily: 'var(--font-body)',
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
