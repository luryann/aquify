'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { UnlinkedSwimmerNotice } from '@/components/swimmer/UnlinkedSwimmerNotice'
import {
  addCommitmentEventSelection,
  deleteCommitmentEventSelection,
  fetchMeet,
  fetchMeetSessions,
  fetchMeetCommitments,
  fetchCommitmentEventSelections,
  fetchMeetEvents,
  fetchExportsForMeet,
  respondToMeetCommitment,
  fetchSwimmers,
  fetchSwimmerBestTimes,
  type MeetRecord,
  type MeetSessionRecord,
  type MeetEventRecord,
  type BestTimeRecord,
  type ExportRecord,
} from '@/lib/adminData'
import {
  buildSwimmerMeetDetailState,
  canEditSwimmerMeetSelections,
  type SwimmerMeetDetailState,
  withAddedSwimmerMeetSelection,
  withRemovedSwimmerMeetSelection,
} from '@/lib/swimmer-meet-detail-state'

const C = {
  bg: 'var(--bg)',
  card: 'var(--bg-surface)',
  text: 'var(--text)',
  subtle: 'var(--text-subtle)',
  muted: 'var(--text-muted)',
  accent: 'var(--accent)',
  accentDim: 'var(--border)',
  accentBorder: 'var(--accent-border)',
  border: 'var(--border)',
  warning: 'var(--warning)',
}

// ── Helpers ──────────────────────────────────────────────────────────────────
function fmtDateLabel(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function fmtTimeFull(isoOrTime?: string): string {
  if (!isoOrTime) return ''
  const plainTime = isoOrTime.match(/^(\d{1,2}):(\d{2})(?::\d{2})?$/)
  if (plainTime) {
    const d = new Date(2000, 0, 1, Number(plainTime[1]), Number(plainTime[2]))
    return d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
  }
  try {
    const d = new Date(isoOrTime)
    return d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
  } catch {
    return isoOrTime
  }
}

function strokeLabel(stroke?: string): string {
  if (!stroke) return ''
  const map: Record<string, string> = {
    freestyle: 'Freestyle', backstroke: 'Backstroke', breaststroke: 'Breaststroke',
    butterfly: 'Butterfly', im: 'IM', individual_medley: 'IM',
  }
  return map[stroke.toLowerCase()] ?? stroke
}

function eventName(ev: MeetEventRecord): string {
  const dist = ev.distance ? `${ev.distance} ` : ''
  const stroke = strokeLabel(ev.stroke)
  return `${dist}${stroke}`.trim() || `Event #${ev.event_number ?? '—'}`
}

function fmtMs(ms: number): string {
  const totalSec = ms / 1000
  const mins = Math.floor(totalSec / 60)
  const secs = totalSec - mins * 60
  const secStr = secs.toFixed(2).padStart(5, '0')
  return mins > 0 ? `${mins}:${secStr}` : secStr
}

function getBestTimeMs(times: BestTimeRecord[], ev: MeetEventRecord, course: string): number | null {
  const match = times.find(
    (t) =>
      t.event_distance === ev.distance &&
      t.stroke?.toLowerCase() === ev.stroke?.toLowerCase() &&
      t.course?.toLowerCase() === course.toLowerCase(),
  )
  return match?.best_time_ms ?? null
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

function Skeleton({ w = '100%', h = 16, r = 8 }: { w?: string | number; h?: number; r?: number }) {
  return (
    <div
      style={{
        width: w, height: h, borderRadius: r,
        background: 'linear-gradient(90deg, rgba(255,255,255,0.05) 25%, rgba(255,255,255,0.10) 50%, rgba(255,255,255,0.05) 75%)',
        backgroundSize: '200% 100%',
        animation: 'shimmer 1.5s infinite',
      }}
    />
  )
}

// ── Page ─────────────────────────────────────────────────────────────────────
export default function SwimmerMeetDetailPage() {
  const params = useParams<{ id: string }>()
  const meetId = typeof params?.id === 'string' ? params.id : ''

  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''
  const mySwimmerId = session?.user.swimmerId ?? ''

  const [meet, setMeet] = useState<MeetRecord | null>(null)
  const [sessions, setSessions] = useState<MeetSessionRecord[]>([])
  const [events, setEvents] = useState<MeetEventRecord[]>([])
  const [meetFiles, setMeetFiles] = useState<ExportRecord[]>([])
  const [bestTimes, setBestTimes] = useState<BestTimeRecord[]>([])
  const [swimmerId, setSwimmerId] = useState<string | null>(null)
  const [swimmerName, setSwimmerName] = useState<string>('')
  const [swimmerGender, setSwimmerGender] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [filesLoading, setFilesLoading] = useState(true)
  const [filesError, setFilesError] = useState<string | null>(null)

  const [commitment, setCommitment] = useState<'pending' | 'yes' | 'no'>('pending')
  const [commitmentId, setCommitmentId] = useState<string | null>(null)
  const [selectedEvents, setSelectedEvents] = useState<string[]>([])
  const [selectedEventSelectionIds, setSelectedEventSelectionIds] = useState<Record<string, string>>({})
  const [submittingCommitment, setSubmittingCommitment] = useState<'yes' | 'no' | null>(null)
  const [selectionSubmitting, setSelectionSubmitting] = useState<string | null>(null)
  const [commitError, setCommitError] = useState<string | null>(null)
  const [selectionError, setSelectionError] = useState<string | null>(null)
  const [retryKey, setRetryKey] = useState(0)
  const [noLinkedSwimmer, setNoLinkedSwimmer] = useState(false)

  useEffect(() => {
    if (!orgId || !meetId || !householdId) {
      setLoadError('Your swimmer portal session is missing meet or household context. Refresh and sign in again.')
      setLoading(false)
      setFilesLoading(false)
      return
    }
    setNoLinkedSwimmer(!mySwimmerId)

    let cancelled = false

    async function loadMeetDetail() {
      setLoading(true)
      setLoadError(null)
      setNotFound(false)
      setFilesLoading(true)
      setFilesError(null)
      setMeet(null)
      setSessions([])
      setEvents([])
      setMeetFiles([])
      setBestTimes([])
      setSwimmerId(null)
      setSwimmerName('')
      setSwimmerGender('')
      setCommitment('pending')
      setCommitmentId(null)
      setSelectedEvents([])
      setSelectedEventSelectionIds({})
      setCommitError(null)
      setSelectionError(null)

      try {
        // Meet name/date/location come from the meet record itself, not the
        // swimmer link — always fetch it so an unlinked account still sees
        // basic meet info instead of a blank page. Sessions/events/roster are
        // only rendered behind a linked swimmer, so fetch those only when
        // there's a swimmer to fetch them for.
        const [meetData, allSwimmers] = await Promise.all([
          fetchMeet(api, orgId, meetId),
          mySwimmerId ? fetchSwimmers(api, orgId) : Promise.resolve([]),
        ])
        if (cancelled) return
        setMeet(meetData)

        if (!mySwimmerId) return

        const hhSwimmer = allSwimmers.find((s) => s.id === mySwimmerId)
        if (!hhSwimmer) {
          setLoadError('Your linked swimmer record could not be loaded for this meet. Refresh and try again.')
          return
        }

        const [meetSessions, meetEvents, commitments, times, files] = await Promise.all([
          fetchMeetSessions(api, orgId, meetId),
          fetchMeetEvents(api, orgId, meetId),
          fetchMeetCommitments(api, orgId, meetId),
          fetchSwimmerBestTimes(api, orgId, hhSwimmer.id),
          fetchExportsForMeet(api, orgId, meetId).catch((error) => {
            if (error instanceof ApiError && error.isForbidden()) {
              setFilesError('Meet files become available when this meet is tied to your swimmer household.')
            } else {
              setFilesError(reportUIError(
                'swimmer.meetDetail.files.load',
                error,
                'Failed to load meet files.',
                { orgId, meetId, householdId },
              ))
            }
            return [] as ExportRecord[]
          }),
        ])
        if (cancelled) return

        const myCommitment = commitments.find((c) => c.swimmer_id === hhSwimmer.id)
        const persistedSelections = myCommitment
          ? await fetchCommitmentEventSelections(api, orgId, meetId, myCommitment.id)
          : []
        if (cancelled) return

        const swimmerState = buildSwimmerMeetDetailState(myCommitment, persistedSelections)
        setSessions(meetSessions)
        setEvents(meetEvents)
        setMeetFiles(files)
        setBestTimes(times)
        setSwimmerId(hhSwimmer.id)
        setSwimmerName(`${hhSwimmer.first_name} ${hhSwimmer.last_name}`)
        setSwimmerGender(hhSwimmer.gender ?? '')
        setCommitment(swimmerState.commitment)
        setCommitmentId(swimmerState.commitmentId)
        setSelectedEvents(swimmerState.selectedEvents)
        setSelectedEventSelectionIds(swimmerState.selectedEventSelectionIds)
      } catch (error) {
        if (cancelled) return
        if (error instanceof ApiError && error.status === 404) {
          setNotFound(true)
          return
        }
        reportUIError(
          'swimmer.meetDetail.load',
          error,
          'Unable to load meet details right now. Please try again.',
          { orgId, meetId, householdId },
        )
        setLoadError('Unable to load meet details right now. Please try again.')
      } finally {
        if (!cancelled) {
          setLoading(false)
          setFilesLoading(false)
        }
      }
    }

    void loadMeetDetail()

    return () => {
      cancelled = true
    }
  }, [api, orgId, meetId, householdId, mySwimmerId, retryKey])

  const commitmentOpen = meet?.status === 'commitment_open'
  const deadlineDate = meet?.commitment_deadline ? new Date(meet.commitment_deadline) : null

  const toggleCommitment = async (next: 'yes' | 'no') => {
    if (!commitmentOpen) return
    setCommitError(null)

    if (!swimmerId) {
      setCommitError('Cannot update this meet commitment until the swimmer record is available.')
      return
    }

    const currentCommitmentId = commitmentId
    if (!currentCommitmentId) {
      const meetCommitments = await fetchMeetCommitments(api, orgId, meetId)
      const myCommitment = meetCommitments.find((item) => item.swimmer_id === swimmerId)
      if (!myCommitment) {
        setCommitError('Cannot update commitment because no backend commitment record was found. Refresh and try again.')
        return
      }
      setCommitmentId(myCommitment.id)
      setSubmittingCommitment(next)
      try {
        await respondToMeetCommitment(api, orgId, meetId, myCommitment.id, {
          response: next === 'yes' ? 'committed_yes' : 'committed_no',
        })
        setCommitment(next)
        if (next === 'no') {
          setSelectedEvents([])
          setSelectedEventSelectionIds({})
        }
      } catch (error) {
        reportUIError('swimmer.meetDetail.commitment.update', error, 'Failed to save your commitment response.', {
          orgId,
          meetId,
          commitmentId: myCommitment.id,
          swimmerId,
        })
        setCommitError('Failed to save your commitment response. Refresh to confirm the current saved state before trying again.')
      } finally {
        setSubmittingCommitment(null)
      }
      return
    }

    setSubmittingCommitment(next)
    try {
      await respondToMeetCommitment(api, orgId, meetId, currentCommitmentId, {
        response: next === 'yes' ? 'committed_yes' : 'committed_no',
      })
      setCommitment(next)
      if (next === 'no') {
        setSelectedEvents([])
        setSelectedEventSelectionIds({})
      }
    } catch (error) {
      reportUIError('swimmer.meetDetail.commitment.update', error, 'Failed to save your commitment response.', {
        orgId,
        meetId,
        commitmentId: currentCommitmentId,
        swimmerId,
      })
      setCommitError('Failed to save your commitment response. Refresh to confirm the current saved state before trying again.')
    } finally {
      setSubmittingCommitment(null)
    }
  }

  const toggleEvent = (eventId: string) => {
    const swimmerState: SwimmerMeetDetailState = {
      commitment,
      commitmentId,
      selectedEvents,
      selectedEventSelectionIds,
    }
    if (!canEditSwimmerMeetSelections(commitmentOpen, swimmerState)) return

    const currentCommitmentId = swimmerState.commitmentId
    if (!currentCommitmentId) {
      setSelectionError('Cannot update event selections without a commitment record. Please refresh and try again.')
      return
    }

    setSelectionSubmitting(eventId)
    setSelectionError(null)

    const existingSelectionId = selectedEventSelectionIds[eventId]
    const request = existingSelectionId
      ? deleteCommitmentEventSelection(api, orgId, meetId, currentCommitmentId, existingSelectionId).then(() => ({ remove: true as const }))
      : addCommitmentEventSelection(api, orgId, meetId, currentCommitmentId, eventId).then((selection) => ({ remove: false as const, selection }))

    request
      .then((result) => {
        const currentState: SwimmerMeetDetailState = {
          commitment,
          commitmentId: currentCommitmentId,
          selectedEvents,
          selectedEventSelectionIds,
        }
        const nextState = result.remove
          ? withRemovedSwimmerMeetSelection(currentState, eventId)
          : withAddedSwimmerMeetSelection(currentState, result.selection)
        setSelectedEvents(nextState.selectedEvents)
        setSelectedEventSelectionIds(nextState.selectedEventSelectionIds)
      })
      .catch((error) => {
        reportUIError('swimmer.meetDetail.selection.update', error, 'Failed to save event selections.', {
          orgId,
          meetId,
          commitmentId: currentCommitmentId,
          eventId,
          swimmerId,
        })
        setSelectionError('Failed to save event selections. Refresh the page to confirm the current saved events before trying again.')
      })
      .finally(() => setSelectionSubmitting(null))
  }

  // Group events by session, filtered by swimmer gender
  const sessionedEvents = sessions.map((session) => ({
    ...session,
    events: events.filter(
      (ev) =>
        ev.session_id === session.id &&
        (!ev.gender || !swimmerGender || ev.gender.toLowerCase() === swimmerGender.toLowerCase() || ev.gender.toLowerCase() === 'mixed'),
    ),
  })).filter((s) => s.events.length > 0)

  // If no sessions, show all events
  const unsessionedEvents = events.filter(
    (ev) =>
      !ev.session_id &&
      (!ev.gender || !swimmerGender || ev.gender.toLowerCase() === swimmerGender.toLowerCase() || ev.gender.toLowerCase() === 'mixed'),
  )

  const swimmerInitials = swimmerName
    ? swimmerName.split(' ').map((p) => p[0] ?? '').join('').toUpperCase().slice(0, 2)
    : '?'

  if (!loading && notFound) {
    return (
      <div style={{ maxWidth: '720px' }}>
        <style>{`@keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }`}</style>
        <Link href="/swim/schedule" style={{ display: 'inline-flex', alignItems: 'center', gap: '5px', color: C.subtle, textDecoration: 'none', fontSize: '13px', marginBottom: '22px' }}>
          <Icon name="chevron-left" size={14} />
          Schedule
        </Link>
        <div className="card" style={{ padding: '24px 26px' }}>
          <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: C.accent, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
            Athlete Meet Center
          </div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: C.text, lineHeight: 1.05, marginBottom: '10px' }}>Meet not found</h1>
          <p style={{ fontSize: '13px', color: C.subtle, lineHeight: 1.7 }}>
            No meet was found for <span style={{ fontFamily: 'var(--font-body)', color: C.text }}>{meetId || 'this route'}</span>.
          </p>
        </div>
      </div>
    )
  }

  if (!loading && loadError) {
    return (
      <div style={{ maxWidth: '720px' }}>
        <style>{`@keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }`}</style>
        <Link href="/swim/schedule" style={{ display: 'inline-flex', alignItems: 'center', gap: '5px', color: C.subtle, textDecoration: 'none', fontSize: '13px', marginBottom: '22px' }}>
          <Icon name="chevron-left" size={14} />
          Schedule
        </Link>
        <div className="card" style={{ padding: '24px 26px' }}>
          <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--error)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
            Athlete Meet Center
          </div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: C.text, lineHeight: 1.05, marginBottom: '10px' }}>
            Meet details are unavailable right now.
          </h1>
          <p style={{ fontSize: '13px', color: C.subtle, lineHeight: 1.7, marginBottom: '18px' }}>
            {loadError}
          </p>
          <button
            onClick={() => setRetryKey((current) => current + 1)}
            className="btn"
            style={{ background: 'var(--error)', color: '#fff', border: 'none' }}
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

  return (
    <div style={{ maxWidth: '980px' }}>
      <style>{`@keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }`}</style>

      <Link href="/swim/schedule" style={{ display: 'inline-flex', alignItems: 'center', gap: '5px', color: C.subtle, textDecoration: 'none', fontSize: '13px', marginBottom: '22px' }}>
        <Icon name="chevron-left" size={14} />
        Back
      </Link>

      {/* Meet header */}
      <div className="card" style={{ padding: '24px 26px', marginBottom: '20px', boxShadow: '0 16px 40px rgba(0,0,0,0.18)' }}>
        {loading ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <Skeleton w="55%" h={32} r={6} />
            <Skeleton w="70%" h={14} />
          </div>
        ) : (
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: '16px', flexWrap: 'wrap' }}>
            <div>
              <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: C.accent, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
                Athlete Meet Center
              </div>
              <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: C.text, lineHeight: 1.05, marginBottom: '10px' }}>
                {meet?.name ?? '—'}
              </h1>
              <div style={{ display: 'flex', gap: '14px', flexWrap: 'wrap', fontSize: '13px', color: C.subtle }}>
                {meet?.start_date && (
                  <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                    <Icon name="calendar" size={12} />
                    {fmtDateLabel(meet.start_date)}
                  </span>
                )}
                {meet?.location && (
                  <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                    <Icon name="map-pin" size={12} />
                    {meet.location}
                  </span>
                )}
                {meet?.course && (
                  <span style={{ fontFamily: 'var(--font-body)', color: C.text }}>{meet.course}</span>
                )}
              </div>
            </div>

            {deadlineDate && (
              <div style={{ padding: '12px 14px', borderRadius: '12px', background: 'var(--warning-bg)', border: '1px solid rgba(245,158,11,0.22)', minWidth: '180px' }}>
                <div style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: C.warning, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '4px' }}>
                  Commitment Deadline
                </div>
                <div style={{ fontSize: '15px', fontWeight: 700, color: 'var(--warning)' }}>
                  {deadlineDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {!loading && noLinkedSwimmer && <UnlinkedSwimmerNotice />}

      {!noLinkedSwimmer && (
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) 290px', gap: '18px', alignItems: 'start' }}>
        {/* Left: commitment + event signup */}
        <div className="card" style={{ overflow: 'hidden' }}>
          {/* Swimmer identity */}
          <div style={{ padding: '18px 20px', borderBottom: `1px solid ${C.border}` }}>
            {loading ? (
              <div style={{ display: 'flex', gap: '12px', alignItems: 'center' }}>
                <Skeleton w={42} h={42} r={21} />
                <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '8px' }}>
                  <Skeleton w="45%" h={18} />
                  <Skeleton w="60%" h={12} />
                </div>
              </div>
            ) : (
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <div style={{ width: '42px', height: '42px', borderRadius: '50%', background: 'linear-gradient(135deg, var(--accent) 0%, var(--accent-2) 100%)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--bg)', fontFamily: 'var(--font-body)', fontWeight: 700 }}>
                  {swimmerInitials}
                </div>
                <div style={{ flex: 1 }}>
                  <div style={{ fontFamily: 'var(--font-display)', fontSize: '20px', color: C.text }}>{swimmerName}</div>
                  <div style={{ fontSize: '12px', color: C.subtle }}>Session-split event signup</div>
                </div>
              </div>
            )}
          </div>

          {/* Commitment buttons */}
          <div style={{ padding: '18px 20px', borderBottom: `1px solid ${C.border}` }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '10px', marginBottom: '12px' }}>
              <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                Commitment
              </div>
              <span style={{
                fontSize: '11px', fontWeight: 600, padding: '4px 10px', borderRadius: '999px',
                color: commitment === 'yes' ? C.accent : commitment === 'no' ? 'var(--error)' : 'var(--warning)',
                background: commitment === 'yes' ? C.accentDim : commitment === 'no' ? 'var(--error-bg)' : 'var(--warning-bg)',
                border: `1px solid ${commitment === 'yes' ? C.accentBorder : commitment === 'no' ? 'var(--error-border)' : 'var(--warning-border)'}`,
              }}>
                {commitment === 'yes' ? 'Going' : commitment === 'no' ? 'Not going' : 'Waiting for your answer'}
              </span>
            </div>
            <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
              {[
                { id: 'yes', label: 'I am going', color: C.accent, bg: C.accentDim, border: C.accentBorder },
                { id: 'no', label: 'Not this meet', color: 'var(--error)', bg: 'var(--error-bg)', border: 'var(--error-border)' },
              ].map((option) => (
                <button
                  key={option.id}
                  onClick={() => void toggleCommitment(option.id as 'yes' | 'no')}
                  style={{ borderRadius: '10px', border: `1px solid ${commitment === option.id ? option.border : C.border}`, background: commitment === option.id ? option.bg : 'transparent', color: commitment === option.id ? option.color : C.text, padding: '10px 16px', fontSize: '13px', fontWeight: 600, cursor: commitmentOpen && !submittingCommitment ? 'pointer' : 'not-allowed', opacity: commitmentOpen && !submittingCommitment ? 1 : 0.6, fontFamily: 'var(--font-body)' }}
                  disabled={!commitmentOpen || submittingCommitment !== null}
                >
                  {option.label}
                </button>
              ))}
            </div>
            {commitError && (
              <div style={{ marginTop: '12px', fontSize: '12px', color: 'var(--error)', lineHeight: 1.6 }}>
                {commitError}
              </div>
            )}
            {!commitmentOpen && (
              <div style={{ marginTop: '12px', fontSize: '12px', color: '#93c5fd', lineHeight: 1.6 }}>
                {meet?.status === 'published'
                  ? 'This meet is visible now, but self-signup stays read-only until the commitment window opens.'
                  : 'The commitment window for this meet is closed.'}
              </div>
            )}
          </div>

          {/* Event signup */}
          {commitmentOpen && commitment !== 'no' && (
            <div style={{ padding: '18px 20px' }}>
              <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '6px' }}>
                Session-Split Signup
              </div>
              <div style={{ fontSize: '11.5px', color: C.muted, marginBottom: '12px', lineHeight: 1.5 }}>
                &ldquo;Cut&rdquo; is the qualifying time for an event — your best time shows whether you&rsquo;ve met it.
              </div>
              {selectionError && (
                <div style={{ marginBottom: '12px', fontSize: '12px', color: 'var(--error)', lineHeight: 1.6 }}>
                  {selectionError}
                </div>
              )}

              {loading ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                  <Skeleton h={80} r={12} />
                  <Skeleton h={80} r={12} />
                </div>
              ) : sessionedEvents.length === 0 && unsessionedEvents.length === 0 ? (
                <div style={{ fontSize: '13px', color: C.muted, textAlign: 'center', padding: '16px 0' }}>
                  No events available for this meet yet.
                </div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                  {/* Sessioned events */}
                  {sessionedEvents.map((session) => (
                    <div key={session.id} style={{ borderRadius: '14px', border: `1px solid ${C.border}`, background: 'rgba(255,255,255,0.02)', padding: '14px' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '10px', flexWrap: 'wrap' }}>
                        <div>
                          <div style={{ fontSize: '14px', fontWeight: 700, color: C.text }}>{session.name}</div>
                          <div style={{ fontSize: '12px', color: C.subtle }}>
                            {session.warmup_time ? `Warm-up ${fmtTimeFull(session.warmup_time)}` : ''}
                            {session.start_time ? ` · Start ${fmtTimeFull(session.start_time)}` : ''}
                          </div>
                        </div>
                        <span style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: C.accent, background: C.accentDim, border: `1px solid ${C.accentBorder}`, borderRadius: '999px', padding: '3px 8px' }}>
                          {session.events.length} choices
                        </span>
                      </div>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                        {session.events.map((ev) => <EventButton key={ev.id} ev={ev} meet={meet} bestTimes={bestTimes} selectedEvents={selectedEvents} toggleEvent={toggleEvent} selectionSubmitting={selectionSubmitting === ev.id} />)}
                      </div>
                    </div>
                  ))}

                  {/* Unsessioned events */}
                  {unsessionedEvents.length > 0 && (
                    <div style={{ borderRadius: '14px', border: `1px solid ${C.border}`, background: 'rgba(255,255,255,0.02)', padding: '14px' }}>
                    <div style={{ fontSize: '14px', fontWeight: 700, color: C.text, marginBottom: '10px' }}>Events</div>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                        {unsessionedEvents.map((ev) => <EventButton key={ev.id} ev={ev} meet={meet} bestTimes={bestTimes} selectedEvents={selectedEvents} toggleEvent={toggleEvent} selectionSubmitting={selectionSubmitting === ev.id} />)}
                    </div>
                  </div>
                )}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Right: summary */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {/* Meet files */}
          <div className="card" style={{ padding: '18px' }}>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '10px' }}>
              Meet Files
            </div>
            {filesLoading ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px', padding: '10px 0' }}>
                <Skeleton h={14} r={999} />
                <Skeleton w="72%" h={14} r={999} />
              </div>
            ) : filesError ? (
              <div style={{ fontSize: '12px', color: 'var(--warning)', lineHeight: 1.6 }}>
                {filesError}
              </div>
            ) : meetFiles.length === 0 ? (
              <div style={{ fontSize: '13px', color: C.muted, textAlign: 'center', padding: '10px 0', lineHeight: 1.6 }}>
                <Icon name="file-text" size={18} style={{ display: 'block', margin: '0 auto 8px', color: C.muted }} />
                No meet files available yet.
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                {meetFiles.map((file) => (
                  <div
                    key={file.id}
                    style={{
                      borderRadius: '12px',
                      border: `1px solid ${C.border}`,
                      background: 'rgba(255,255,255,0.03)',
                      padding: '12px',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: '10px', alignItems: 'flex-start' }}>
                      <div style={{ minWidth: 0 }}>
                        <div style={{ fontSize: '13px', fontWeight: 600, color: C.text, wordBreak: 'break-word' }}>{file.filename}</div>
                        <div style={{ marginTop: '4px', fontSize: '11.5px', color: C.subtle, lineHeight: 1.5 }}>
                          {formatMeetFileType(file.export_type)} · {formatMeetFileSize(file.size_bytes)}
                        </div>
                      </div>
                      {file.download_url ? (
                        <a
                          href={file.download_url}
                          target="_blank"
                          rel="noreferrer"
                          style={{ color: C.accent, fontSize: '12px', fontWeight: 600, textDecoration: 'none', whiteSpace: 'nowrap' }}
                        >
                          Download
                        </a>
                      ) : (
                        <span style={{ color: C.subtle, fontSize: '12px', whiteSpace: 'nowrap' }}>
                          {file.status === 'ready' ? 'Link pending' : file.status.replace(/_/g, ' ')}
                        </span>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Summary */}
          <div className="card" style={{ padding: '18px' }}>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '10px' }}>
              Summary
            </div>
            <div style={{ fontSize: '14px', fontWeight: 600, color: C.text, marginBottom: '6px' }}>
              {selectedEvents.length} selected event{selectedEvents.length !== 1 ? 's' : ''}
            </div>
            <div style={{ fontSize: '12px', color: C.subtle, lineHeight: 1.6 }}>
              Coaches can still adjust entries before final submission.
            </div>
          </div>
        </div>
      </div>
      )}
    </div>
  )
}

// ── Event row button ─────────────────────────────────────────────────────────
function EventButton({
  ev,
  meet,
  bestTimes,
  selectedEvents,
  selectionSubmitting,
  toggleEvent,
}: {
  ev: MeetEventRecord
  meet: MeetRecord | null
  bestTimes: BestTimeRecord[]
  selectedEvents: string[]
  selectionSubmitting: boolean
  toggleEvent: (id: string) => void
}) {
  const selected = selectedEvents.includes(ev.id)
  const btMs = meet ? getBestTimeMs(bestTimes, ev, meet.course) : null
  const btLabel = btMs !== null ? fmtMs(btMs) : 'No time yet'

  const qualMs = ev.qualifying_time_cents != null
    ? ev.qualifying_time_cents * 10
    : null
  const cutMet = btMs !== null && qualMs !== null ? btMs <= qualMs : null

  return (
    <button
      onClick={() => toggleEvent(ev.id)}
      disabled={selectionSubmitting}
      style={{ textAlign: 'left', borderRadius: '12px', border: `1px solid ${selected ? C.accentBorder : C.border}`, background: selected ? C.accentDim : 'rgba(255,255,255,0.03)', padding: '12px 14px', cursor: selectionSubmitting ? 'progress' : 'pointer', opacity: selectionSubmitting ? 0.65 : 1, color: C.text, fontFamily: 'var(--font-body)' }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center' }}>
        <div>
          <div style={{ fontWeight: 600, fontSize: '14px' }}>
            {ev.event_number ? `#${ev.event_number} ` : ''}{eventName(ev)}
          </div>
          <div style={{ fontSize: '11.5px', color: C.subtle }}>
            {ev.age_group ?? ''}
            {qualMs !== null && (
              <span style={{ marginLeft: ev.age_group ? '8px' : 0 }}>
                Cut {fmtMs(qualMs)}
              </span>
            )}
          </div>
        </div>
        <div style={{ textAlign: 'right' }}>
          <div style={{ fontSize: '13px', fontFamily: 'var(--font-body)', color: C.accent, fontWeight: 700 }}>
            {btLabel}
          </div>
          {cutMet !== null && (
            <div style={{ fontSize: '10px', color: cutMet ? C.accent : 'var(--warning)' }}>
              {cutMet ? 'meets cut' : 'under cut'}
            </div>
          )}
        </div>
      </div>
    </button>
  )
}
