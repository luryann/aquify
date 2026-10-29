'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { ageFromDob, formatClockTime, formatDateOnly, parseBackendDate } from '@/lib/dates'
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
import './page.inline.css'
import { formatMeetFileSize, formatMeetFileType } from '@/lib/meet-files'
import { formatStroke } from '@/lib/meet-events'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'

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

const SWIMMER_GRADIENTS = [
  'var(--accent)',
  'var(--info)',
  'var(--warning)',
  'var(--success)',
  'var(--text-muted)',
]

function eventLabel(ev: MeetEventRecord): string {
  const parts: string[] = []
  if (ev.distance) parts.push(String(ev.distance))
  if (ev.stroke) parts.push(formatStroke(ev.stroke))
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

// ── Yes / No response button ─────────────────────────────────────────────────
const RESPONSE_BUTTON = {
  yes: { tone: 'success', icon: 'check-circle', label: 'Yes, we\'re going' },
  no: { tone: 'error', icon: 'x-circle', label: 'Not this time' },
} as const

function ResponseButton({ answer, selected, disabled, busy, onClick }: {
  answer: 'yes' | 'no'
  selected: boolean
  disabled: boolean
  busy: boolean
  onClick: () => void
}) {
  const { tone, icon, label } = RESPONSE_BUTTON[answer]
  return (
    <button type="button"
      onClick={onClick}
      disabled={disabled}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        padding: '12px 24px',
        borderRadius: '12px',
        border: selected ? `2px solid var(--${tone})` : '1.5px solid var(--border-strong)',
        background: selected ? `var(--${tone}-bg)` : 'var(--bg-elevated)',
        color: selected ? `var(--${tone})` : 'var(--text)',
        fontWeight: 600,
        fontSize: 'var(--fs-md)',
        cursor: 'pointer',
        fontFamily: 'var(--font-body)',
        transition: 'all 120ms ease',
        opacity: disabled ? 0.6 : 1,
      }}
    >
      <Icon name={icon} size={16} style={{ color: selected ? `var(--${tone})` : 'var(--text-muted)' }} />
      {busy ? 'Saving…' : label}
    </button>
  )
}

// ── Loading skeleton ─────────────────────────────────────────────────────────
function LoadingSkeleton() {
  return (
    <div className="page">
      <div className="is-8718669" />
      <div
        className="card is-c7147b4" 
      >
        <div className="is-12c6d3c" />
        <div className="is-cb387ac">
          <div style={{ height: '14px', width: '120px', background: 'var(--bg-wash)', borderRadius: '4px' }} />
          <div style={{ height: '14px', width: '140px', background: 'var(--bg-wash)', borderRadius: '4px' }} />
        </div>
      </div>
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
      setLoadError('We could not find your household details. Sign out and back in, or contact your club if this keeps happening.')
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
          'We could not load this page. Check your connection and try again.',
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
        <nav className="is-781ef11">
          <Link href="/portal/meets" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>
            Meets
          </Link>
        </nav>
        {loadError && (
          <LoadErrorBanner message={loadError} onRetry={() => setReloadKey((key) => key + 1)} />
        )}
        <div
          className="card is-0acd099" 
        >
          <h1 className="page-title">
            {loadError ? 'Meet details are unavailable right now.' : 'Meet not found'}
          </h1>
          <p className="page-subtitle">
            {loadError
              ? 'Refresh, or try again shortly.'
              : <>No meet found for <span className="is-b88ab11">{meetId || 'this route'}</span>.</>}
          </p>
          <Link href="/portal/meets" className="is-71dc476">
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

  const deadlineUrgent = daysUntilDeadline !== null && daysUntilDeadline <= 3
  const deadlineTone = deadlineUrgent ? 'error' : 'warning'

  const commitmentOpen = meet.status === 'commitment_open'

  const swimmer = swimmers.find((s) => s.id === activeSwimmer)
  const state = activeSwimmer ? swimmerStates[activeSwimmer] : undefined

  // Group events by session_id for display
  const sessionIds = Array.from(new Set(events.map((e) => e.session_id ?? 'main')))
  const sessionNameById = new Map(sessions.map((s) => [s.id, s.name?.trim() || `Session ${s.session_number}`]))
  const sessionLabel = (sid: string) => (sid === 'main' ? 'Main Session' : sessionNameById.get(sid) ?? `Session ${sid.slice(-4)}`)
  const sessionById = new Map(sessions.map((s) => [s.id, s]))
  const sessionScheduleLabel = (sid: string) => {
    const s = sessionById.get(sid)
    if (!s) return null
    const date = s.session_date
      ? new Date(`${s.session_date}T00:00:00`).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
      : null
    const warmup = formatClockTime(s.warmup_time)
    const start = formatClockTime(s.start_time)
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

  const flashSaved = (swimmerId: string) => {
    setSubmitted((prev) => ({ ...prev, [swimmerId]: true }))
    setTimeout(() => setSubmitted((prev) => ({ ...prev, [swimmerId]: false })), 2500)
  }

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
        flashSaved(swimmerId)
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
      flashSaved(swimmerId)
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
        className="is-90f3963"
      >
        <Link href="/portal/meets" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>
          Meets
        </Link>
        <span>/</span>
        <span className="is-aaa1da3">{meet.name}</span>
      </nav>

      {/* Meet header */}
      <div
        className="card is-3316b93" 
      >
        <div className="is-421ada3">
          <div>
            <h1 className="page-title">
              {meet.name}
            </h1>
            <div className="is-cc4c7a9">
              <span className="is-df7e073">
                <Icon name="calendar" size={13} style={{ color: 'var(--text-muted)' }} />
                {dateLabel}
              </span>
              {meet.location && (
                <span className="is-df7e073">
                  <Icon name="map-pin" size={13} style={{ color: 'var(--text-muted)' }} />
                  {meet.location}
                </span>
              )}
              <span
                className="is-a921358"
              >
                {meet.course}
              </span>
            </div>
          </div>

          {/* Deadline urgency */}
          {deadlineDate && (
            <div
              style={{
                background: `var(--${deadlineTone}-bg)`,
                border: `1px solid var(--${deadlineTone}-border)`,
                borderRadius: '12px',
                padding: '12px 16px',
                textAlign: 'center',
              }}
            >
              <div style={{ fontSize: 'var(--fs-sm)', color: `var(--${deadlineTone})`, fontFamily: 'var(--font-body)', marginBottom: '2px' }}>
                Deadline
              </div>
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 700, color: `var(--${deadlineTone})` }}>
                {formatDateOnly(deadlineStr)}
              </div>
              <div style={{ fontSize: 'var(--fs-sm)', fontWeight: 600, color: `var(--${deadlineTone})`, marginTop: '2px' }}>
                {daysUntilDeadline === null ? '' : daysUntilDeadline <= 0 ? 'Closed' : daysUntilDeadline === 1 ? '1 day left' : `${daysUntilDeadline} days left`}
              </div>
            </div>
          )}
        </div>
      </div>

      <div
        className="split-aside is-73a36ce"
        
      >
        <div
          className="card is-b516f27" 
        >
          <div className="is-20f84a7">
            <div>
              <div className="is-f3c1003">
                Family Notes and Session Plan
              </div>
            </div>
            <span
              className="is-04277ae"
            >
              {events.length} events
            </span>
          </div>

          <div className="cols-3 is-13a4e6f" >
            {[
              { label: 'Host', value: meet.host_org ?? '—' },
              { label: 'Course', value: meet.course },
              { label: 'Location', value: meet.location ?? '—' },
            ].map((item) => (
              <div
                key={item.label}
                className="is-682e071"
              >
                <div className="is-ab10191">
                  {item.label}
                </div>
                <div className="is-97b07bb">
                  {item.value}
                </div>
              </div>
            ))}
          </div>

          {/* Session groups summary */}
          {sessionIds.length > 0 && (
            <div className="is-930d87f">
              {sessionIds.map((sid) => {
                const evCount = events.filter((e) => (e.session_id ?? 'main') === sid).length
                return (
                  <div
                    key={sid}
                    className="is-1b16ebc"
                  >
                    <div className="is-cc98d52">
                      <div>
                        <div className="is-7ae0218">
                          {sessionLabel(sid)}
                        </div>
                        {sessionScheduleLabel(sid) ? (
                          <div className="is-35ca940">
                            {sessionScheduleLabel(sid)}
                          </div>
                        ) : null}
                      </div>
                      <div className="is-c9c6d72">
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
          className="card is-f8d354e" 
        >
          <div className="is-172dd23">
            Meet files
          </div>
          {filesLoading ? (
            <div className="is-7923d16">
              <div style={{ height: '14px', background: 'var(--bg-wash)', borderRadius: '999px' }} />
              <div style={{ height: '14px', width: '76%', background: 'var(--bg-wash)', borderRadius: '999px' }} />
            </div>
          ) : filesError ? (
            <div className="is-bdc139e">
              {filesError}
            </div>
          ) : meetFiles.length === 0 ? (
            <div
              className="is-ac91f09"
            >
              <Icon name="file-text" size={22} style={{ color: 'var(--border-strong)', marginBottom: '8px', display: 'block', margin: '0 auto 8px' }} />
              No meet files yet. Heat sheets and results will appear here once your coach posts them.
            </div>
          ) : (
            <div className="is-930d87f">
              {meetFiles.map((file) => (
                <div
                  key={file.id}
                  className="is-45d5cf2"
                >
                  <div className="is-ece61a4">
                    <div style={{ minWidth: 0 }}>
                      <div className="is-80e95a3">{file.filename}</div>
                      <div className="is-9d94312">
                        {formatMeetFileType(file.export_type)} · {formatMeetFileSize(file.size_bytes)}
                      </div>
                    </div>
                    {file.download_url ? (
                      <a
                        href={file.download_url}
                        target="_blank"
                        rel="noreferrer"
                        className="is-df8a870"
                      >
                        Download
                      </a>
                    ) : (
                      <span className="is-672e8ce">
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
          className="is-284324c"
        >
          {swimmers.map((s, idx) => {
            const st = swimmerStates[s.id]
            const isActive = activeSwimmer === s.id
            const gradient = SWIMMER_GRADIENTS[idx % SWIMMER_GRADIENTS.length]
            const initials = swimmerInitials(s)
            const commitment = st?.commitment ?? 'pending'
            return (
              <button type="button"
                key={s.id}
                onClick={() => setActiveSwimmer(s.id)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  padding: '8px 16px',
                  borderRadius: '12px',
                  border: isActive ? '2px solid var(--accent)' : '1px solid var(--border)',
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
          className="card is-41b831d" 
        >
          {/* Header */}
          <div
            className="is-e957292"
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
              <div className="is-f3c1003">
                {swimmerDisplayName(swimmer)}
              </div>
              <div className="is-4f39ab0">
                Age {ageFromDob(swimmer.date_of_birth)} · {swimmer.gender === 'F' ? 'Female' : swimmer.gender === 'M' ? 'Male' : swimmer.gender}
              </div>
            </div>
            {submitted[swimmer.id] && (
              <div
                className="is-4d1e6c2"
              >
                <Icon name="check-circle" size={15} />
                Saved
              </div>
            )}
          </div>

          {/* Commitment step */}
          <div className="is-cd9e4a1">
            <div
              className="is-a1a186f"
            >
              Are you coming?
            </div>
            <div className="is-18d4ed1">
              {(['yes', 'no'] as const).map((answer) => (
                <ResponseButton
                  key={answer}
                  answer={answer}
                  selected={state.commitment === answer}
                  disabled={submitting !== null || !commitmentOpen || state.commitment === 'closed'}
                  busy={submitting === swimmer.id + answer}
                  onClick={() => setCommitment(swimmer.id, answer)}
                />
              ))}
            </div>
            {/* Note for coaches */}
            {commitmentOpen && state.commitment !== 'closed' && (
              <div className="is-d265a6b">
                <label
                  htmlFor={`note-${swimmer.id}`}
                  className="is-0ee4bcd"
                >
                  Message for coaches <span className="is-1c653fb">(optional)</span>
                </label>
                <textarea
                  id={`note-${swimmer.id}`}
                  value={state.note}
                  onChange={(e) => setNote(swimmer.id, e.target.value)}
                  placeholder="Any notes for the coach — schedule conflicts, injury considerations, preferred events, etc."
                  rows={3}
                  className="is-098874b"
                />
                <div className="is-f4a9600">
                  <p className="is-cb38578">
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
              <div className="is-ac6c66f">
                <div className="is-ab10191">Your note</div>
                <p className="is-11fb03e">{state.note}</p>
              </div>
            )}

            {commitError && (
              <div
                className="is-0130177"
              >
                <Icon name="alert-circle" size={14} style={{ color: 'var(--error)', flexShrink: 0, marginTop: '1px' }} />
                {commitError}
              </div>
            )}
            {selectionError && state.commitment === 'yes' && (
              <div
                className="is-0130177"
              >
                <Icon name="alert-circle" size={14} style={{ color: 'var(--error)', flexShrink: 0, marginTop: '1px' }} />
                {selectionError}
              </div>
            )}
            {!commitmentOpen && state.commitment !== 'closed' && (
              <div
                className="is-c7c6692"
              >
                Commitment actions unlock when this meet moves into the open family response window.
              </div>
            )}
            {state.commitment === 'closed' && (
              <div
                className="is-d06bba4"
              >
                This commitment has been reviewed and locked by the coach.
              </div>
            )}
          </div>

          {/* Event selection (only if committed yes) */}
          {commitmentOpen && state.commitment === 'yes' && (
            <div className="is-b516f27">
              <div
                className="is-4daa4b9"
              >
                Pick events
              </div>
              <p className="is-b19fd90">
                Select the events {swimmer.preferred_name ?? swimmer.first_name} would like to enter. Coach may adjust selections before the deadline.
              </p>

              <div className="is-355eedd">
                {eligibleSessionGroups.length === 0 ? (
                  <div className="is-f17b1ee">
                    Your coach hasn&apos;t opened event sign-ups for {swimmer.preferred_name ?? swimmer.first_name} yet. Check back before the deadline.
                  </div>
                ) : (
                  eligibleSessionGroups.map((group) => (
                    <div key={group.id} className="is-355eedd">
                      <div
                        className="is-a2954f3"
                      >
                        <div className="is-199518b">
                          {sessionLabel(group.id)}
                        </div>
                        <span
                          className="is-20d0acf"
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
                              border: isSelected ? '2px solid var(--accent)' : '1.5px solid var(--border)',
                              background: isSelected ? 'rgba(235,93,30,0.04)' : 'var(--bg-surface)',
                              cursor: selectionSubmitting ? 'progress' : 'pointer',
                              opacity: selectionSubmitting === `${swimmer.id}:${ev.id}` ? 0.65 : 1,
                              transition: 'all 120ms ease',
                            }}
                            onMouseEnter={(e) => {
                              if (!isSelected) e.currentTarget.style.borderColor = 'rgba(235,93,30,0.35)'
                            }}
                            onMouseLeave={(e) => {
                              if (!isSelected) e.currentTarget.style.borderColor = 'var(--border)'
                            }}
                          >
                            {/* Checkbox */}
                            <div
                              style={{
                                width: '20px',
                                height: '20px',
                                borderRadius: '6px',
                                border: isSelected ? 'none' : '2px solid var(--border-strong)',
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
                              className="is-857db60"
                            >
                              #{ev.event_number ?? '—'}
                            </span>

                            {/* Event name */}
                            <div style={{ flex: 1, minWidth: 0 }}>
                              <div className="is-7ae0218">
                                {label}
                              </div>
                              <div className="is-4f39ab0">
                                {ageGroup}
                                {ev.requires_qualification && (
                                  <span className="is-c937f3b">Qualification required</span>
                                )}
                              </div>
                            </div>

                            {/* Entry limit hint */}
                            {ev.entry_limit != null && (
                              <div style={{ textAlign: 'right', flexShrink: 0 }}>
                                <span className="is-279d25a">
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
                  className="is-c8bfb14"
                >
                  <span className="is-df04e21">
                    {state.selectedEvents.length} event{state.selectedEvents.length !== 1 ? 's' : ''} selected
                  </span>
                </div>
              )}

              {/* Coach override notice */}
              <div
                className="is-3b11ba6"
              >
                <Icon name="info" size={13} style={{ color: 'var(--border-strong)', marginTop: '1px', flexShrink: 0 }} />
                Coaches may adjust event selections based on eligibility and team strategy. You&apos;ll see any changes reflected here.
              </div>
            </div>
          )}

          {/* Not going state */}
          {state.commitment === 'no' && (
            <div
              className="is-2e726da"
            >
              <Icon name="x-circle" size={18} style={{ color: 'var(--error)' }} />
              <span className="is-a462b23">
                {swimmer.preferred_name ?? swimmer.first_name} is not attending this meet. You can change this before the deadline.
              </span>
            </div>
          )}
        </div>
      )}

      {/* No swimmers message */}
      {swimmers.length === 0 && (
        <div
          className="card is-a176e40" 
        >
          <div className="is-5e8f68c">No swimmers found for your household</div>
          <div className="is-87e0fe4">
            Contact your club admin to link swimmers to your account.
          </div>
        </div>
      )}

      {/* Fee summary */}
      {totalEvents > 0 && (
        <div
          className="card is-f8dbd06" 
        >
          <div>
            <div className="is-8fb176a">Total estimated entry fee</div>
            <div className="is-09688c5">
              {totalEvents} event{totalEvents !== 1 ? 's' : ''} selected across all swimmers
            </div>
          </div>
          <div
            className="is-cc881b3"
          >
            {totalEvents} event{totalEvents !== 1 ? 's' : ''}
          </div>
        </div>
      )}

          </div>
  )
}
