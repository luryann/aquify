'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { UnlinkedSwimmerNotice } from '@/components/swimmer/UnlinkedSwimmerNotice'
import { isSwimmerEligibleForEvent } from '@/lib/event-eligibility'
import {
  fetchMeet,
  fetchMeetSessions,
  fetchMeetCommitments,
  fetchCommitmentEventSelections,
  fetchMeetEvents,
  fetchExportsForMeet,
  fetchSwimmers,
  fetchSwimmerBestTimes,
  type MeetRecord,
  type MeetSessionRecord,
  type MeetEventRecord,
  type BestTimeRecord,
  type ExportRecord,
} from '@/lib/adminData'
import { buildSwimmerMeetDetailState } from '@/lib/swimmer-meet-detail-state'
import './page.inline.css'
import { formatMeetFileSize, formatMeetFileType } from '@/lib/meet-files'
import { formatStroke } from '@/lib/meet-events'
import { formatClockTime, formatDateOnly } from '@/lib/dates'
import { formatSwimTime } from '@/lib/performanceData'

const C = {
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
function eventName(ev: MeetEventRecord): string {
  const dist = ev.distance ? `${ev.distance} ` : ''
  const stroke = formatStroke(ev.stroke)
  return `${dist}${stroke}`.trim() || `Event #${ev.event_number ?? '—'}`
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

function Skeleton({ w = '100%', h = 16, r = 8 }: { w?: string | number; h?: number; r?: number }) {
  return (
    <div
      style={{
        width: w, height: h, borderRadius: r,
        background: 'linear-gradient(90deg, rgba(255,255,255,0.05) 25%, rgba(255,255,255,0.10) 50%, rgba(255,255,255,0.05) 75%)',
        backgroundSize: '200% 100%',
        animation: 'shimmerSweep 1.5s infinite',
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
  const [swimmerName, setSwimmerName] = useState<string>('')
  const [swimmerGender, setSwimmerGender] = useState<string>('')
  const [swimmerDob, setSwimmerDob] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [filesLoading, setFilesLoading] = useState(true)
  const [filesError, setFilesError] = useState<string | null>(null)

  const [commitment, setCommitment] = useState<'pending' | 'yes' | 'no'>('pending')
  const [selectedEvents, setSelectedEvents] = useState<string[]>([])
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
      setSwimmerName('')
      setSwimmerGender('')
      setSwimmerDob('')
      setCommitment('pending')
      setSelectedEvents([])

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
                'Couldn’t load meet files. Try again.',
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
        setSwimmerName(`${hhSwimmer.first_name} ${hhSwimmer.last_name}`)
        setSwimmerGender(hhSwimmer.gender ?? '')
        setSwimmerDob(hhSwimmer.date_of_birth ?? '')
        setCommitment(swimmerState.commitment)
        setSelectedEvents(swimmerState.selectedEvents)
      } catch (error) {
        if (cancelled) return
        if (error instanceof ApiError && error.status === 404) {
          setNotFound(true)
          return
        }
        reportUIError(
          'swimmer.meetDetail.load',
          error,
          'Couldn’t load meet details. Try again.',
          { orgId, meetId, householdId },
        )
        setLoadError('Couldn’t load meet details. Try again.')
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

  // Group events by session, filtered by swimmer gender and age at the meet
  const sessionedEvents = sessions.map((session) => ({
    ...session,
    events: events.filter(
      (ev) => ev.session_id === session.id && (!swimmerGender || isSwimmerEligibleForEvent({ gender: swimmerGender, date_of_birth: swimmerDob }, ev, meet?.start_date)),
    ),
  })).filter((s) => s.events.length > 0)

  // If no sessions, show all events
  const unsessionedEvents = events.filter(
    (ev) => !ev.session_id && (!swimmerGender || isSwimmerEligibleForEvent({ gender: swimmerGender, date_of_birth: swimmerDob }, ev, meet?.start_date)),
  )

  const swimmerInitials = swimmerName
    ? swimmerName.split(' ').map((p) => p[0] ?? '').join('').toUpperCase().slice(0, 2)
    : '?'

  if (!loading && notFound) {
    return (
      <div className="page">
                <Link href="/swim/schedule" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', color: C.subtle, textDecoration: 'none', fontSize: 'var(--fs-base)', marginBottom: '24px' }}>
          <Icon name="chevron-left" size={14} />
          Schedule
        </Link>
        <div className="card is-653b4fd" >
          <h1 className="page-title">Meet not found</h1>
          <p style={{ fontSize: 'var(--fs-base)', color: C.subtle, lineHeight: 1.7 }}>
            No meet was found for <span style={{ fontFamily: 'var(--font-body)', color: C.text }}>{meetId || 'this route'}</span>.
          </p>
        </div>
      </div>
    )
  }

  if (!loading && loadError) {
    return (
      <div className="page">
                <Link href="/swim/schedule" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', color: C.subtle, textDecoration: 'none', fontSize: 'var(--fs-base)', marginBottom: '24px' }}>
          <Icon name="chevron-left" size={14} />
          Schedule
        </Link>
        <div className="card is-653b4fd" >
          <h1 className="page-title">
            We could not load this page.
          </h1>
          <p style={{ fontSize: 'var(--fs-base)', color: C.subtle, lineHeight: 1.7, marginBottom: '20px' }}>
            {loadError}
          </p>
          <button type="button"
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
    <div className="page">
      
      <Link href="/swim/schedule" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', color: C.subtle, textDecoration: 'none', fontSize: 'var(--fs-base)', marginBottom: '24px' }}>
        <Icon name="chevron-left" size={14} />
        Back
      </Link>

      {/* Meet header */}
      <div className="card is-657abde" >
        {loading ? (
          <div className="is-930d87f">
            <Skeleton w="55%" h={32} r={6} />
            <Skeleton w="70%" h={14} />
          </div>
        ) : (
          <div className="is-7602e17">
            <div>
              <h1 className="page-title">
                {meet?.name ?? '—'}
              </h1>
              <div style={{ display: 'flex', gap: '16px', flexWrap: 'wrap', fontSize: 'var(--fs-base)', color: C.subtle }}>
                {meet?.start_date && (
                  <span className="is-df7e073">
                    <Icon name="calendar" size={12} />
                    {formatDateOnly(meet.start_date)}
                  </span>
                )}
                {meet?.location && (
                  <span className="is-df7e073">
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
              <div className="is-29f8506">
                <div style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: C.warning, marginBottom: '4px' }}>
                  Commitment deadline
                </div>
                <div className="is-f9b35ea">
                  {deadlineDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {!loading && noLinkedSwimmer && <UnlinkedSwimmerNotice />}

      {!noLinkedSwimmer && (
      <div className="split-aside is-ac2cd18" >
        {/* Left: commitment + event signup */}
        <div className="card" style={{ overflow: 'hidden' }}>
          {/* Swimmer identity */}
          <div style={{ padding: '20px 20px', borderBottom: `1px solid ${C.border}` }}>
            {loading ? (
              <div className="is-bc6560f">
                <Skeleton w={42} h={42} r={21} />
                <div className="is-20f80e0">
                  <Skeleton w="45%" h={18} />
                  <Skeleton w="60%" h={12} />
                </div>
              </div>
            ) : (
              <div className="is-e2ff0fd">
                <div className="is-1b340d2">
                  {swimmerInitials}
                </div>
                <div style={{ flex: 1 }}>
                  <div style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: C.text }}>{swimmerName}</div>
                  <div style={{ fontSize: 'var(--fs-sm)', color: C.subtle }}>Session-split event signup</div>
                </div>
              </div>
            )}
          </div>

          {/* Commitment buttons */}
          <div style={{ padding: '20px 20px', borderBottom: `1px solid ${C.border}` }}>
            <div className="is-15297e7">
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: C.text }}>
                Commitment
              </div>
              <span style={{
                fontSize: 'var(--fs-xs)', fontWeight: 600, padding: '4px 12px', borderRadius: '999px',
                color: commitment === 'yes' ? C.accent : commitment === 'no' ? 'var(--error)' : 'var(--warning)',
                background: commitment === 'yes' ? C.accentDim : commitment === 'no' ? 'var(--error-bg)' : 'var(--warning-bg)',
                border: `1px solid ${commitment === 'yes' ? C.accentBorder : commitment === 'no' ? 'var(--error-border)' : 'var(--warning-border)'}`,
              }}>
                {commitment === 'yes' ? 'Going' : commitment === 'no' ? 'Not going' : 'Waiting for your answer'}
              </span>
            </div>
            <div style={{ fontSize: 'var(--fs-sm)', color: C.muted, lineHeight: 1.6 }}>
              Your parent or guardian answers meet commitments and picks events for you. Ask them if this answer needs to change.
            </div>
            {!commitmentOpen && (
              <div className="is-8b5fc35">
                {meet?.status === 'published'
                  ? 'This meet is visible now, but self-signup stays read-only until the commitment window opens.'
                  : 'The commitment window for this meet is closed.'}
              </div>
            )}
          </div>

          {/* Event signup */}
          {commitmentOpen && commitment !== 'no' && (
            <div className="is-e58c8a9">
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: C.text, marginBottom: '6px' }}>
                Events
              </div>
              <div style={{ fontSize: 'var(--fs-xs)', color: C.muted, marginBottom: '12px', lineHeight: 1.5 }}>
                &ldquo;Cut&rdquo; is the qualifying time for an event — your best time shows whether you&rsquo;ve met it.
              </div>
              {loading ? (
                <div className="is-930d87f">
                  <Skeleton h={80} r={12} />
                  <Skeleton h={80} r={12} />
                </div>
              ) : sessionedEvents.length === 0 && unsessionedEvents.length === 0 ? (
                <div style={{ fontSize: 'var(--fs-base)', color: C.muted, textAlign: 'center', padding: '16px 0' }}>
                  No events available for this meet yet.
                </div>
              ) : (
                <div className="is-930d87f">
                  {/* Sessioned events */}
                  {sessionedEvents.map((session) => (
                    <div key={session.id} style={{ borderRadius: '12px', border: `1px solid ${C.border}`, background: 'rgba(255,255,255,0.02)', padding: '16px' }}>
                      <div className="is-2b01013">
                        <div>
                          <div style={{ fontSize: 'var(--fs-md)', fontWeight: 700, color: C.text }}>{session.name}</div>
                          <div style={{ fontSize: 'var(--fs-sm)', color: C.subtle }}>
                            {session.warmup_time ? `Warm-up ${formatClockTime(session.warmup_time)}` : ''}
                            {session.start_time ? ` · Start ${formatClockTime(session.start_time)}` : ''}
                          </div>
                        </div>
                        <span style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', color: C.accent, background: C.accentDim, border: `1px solid ${C.accentBorder}`, borderRadius: '999px', padding: '4px 8px' }}>
                          {session.events.length} choices
                        </span>
                      </div>
                      <div className="is-355eedd">
                        {session.events.map((ev) => <EventButton key={ev.id} ev={ev} meet={meet} bestTimes={bestTimes} selectedEvents={selectedEvents} />)}
                      </div>
                    </div>
                  ))}

                  {/* Unsessioned events */}
                  {unsessionedEvents.length > 0 && (
                    <div style={{ borderRadius: '12px', border: `1px solid ${C.border}`, background: 'rgba(255,255,255,0.02)', padding: '16px' }}>
                    <div style={{ fontSize: 'var(--fs-md)', fontWeight: 700, color: C.text, marginBottom: '12px' }}>Events</div>
                    <div className="is-355eedd">
                        {unsessionedEvents.map((ev) => <EventButton key={ev.id} ev={ev} meet={meet} bestTimes={bestTimes} selectedEvents={selectedEvents} />)}
                    </div>
                  </div>
                )}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Right: summary */}
        <div className="is-8ef476f">
          {/* Meet files */}
          <div className="card is-f8d354e" >
            <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: C.text, marginBottom: '12px' }}>
              Meet files
            </div>
            {filesLoading ? (
              <div className="is-457efd7">
                <Skeleton h={14} r={999} />
                <Skeleton w="72%" h={14} r={999} />
              </div>
            ) : filesError ? (
              <div className="is-aaece7c">
                {filesError}
              </div>
            ) : meetFiles.length === 0 ? (
              <div style={{ fontSize: 'var(--fs-base)', color: C.muted, textAlign: 'center', padding: '12px 0', lineHeight: 1.6 }}>
                <Icon name="file-text" size={18} style={{ display: 'block', margin: '0 auto 8px', color: C.muted }} />
                No meet files available yet.
              </div>
            ) : (
              <div className="is-930d87f">
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
                    <div className="is-ef3773e">
                      <div style={{ minWidth: 0 }}>
                        <div style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: C.text, wordBreak: 'break-word' }}>{file.filename}</div>
                        <div style={{ marginTop: '4px', fontSize: 'var(--fs-xs)', color: C.subtle, lineHeight: 1.5 }}>
                          {formatMeetFileType(file.export_type)} · {formatMeetFileSize(file.size_bytes)}
                        </div>
                      </div>
                      {file.download_url ? (
                        <a
                          href={file.download_url}
                          target="_blank"
                          rel="noreferrer"
                          style={{ color: C.accent, fontSize: 'var(--fs-sm)', fontWeight: 600, textDecoration: 'none', whiteSpace: 'nowrap' }}
                        >
                          Download
                        </a>
                      ) : (
                        <span style={{ color: C.subtle, fontSize: 'var(--fs-sm)', whiteSpace: 'nowrap' }}>
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
          <div className="card is-f8d354e" >
            <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: C.text, marginBottom: '6px' }}>
              {selectedEvents.length} selected event{selectedEvents.length !== 1 ? 's' : ''}
            </div>
            <div style={{ fontSize: 'var(--fs-sm)', color: C.subtle, lineHeight: 1.6 }}>
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
}: {
  ev: MeetEventRecord
  meet: MeetRecord | null
  bestTimes: BestTimeRecord[]
  selectedEvents: string[]
}) {
  const selected = selectedEvents.includes(ev.id)
  const btMs = meet ? getBestTimeMs(bestTimes, ev, meet.course) : null
  const btLabel = btMs !== null ? formatSwimTime(btMs) : 'No time yet'

  const qualMs = ev.qualifying_time_cents != null
    ? ev.qualifying_time_cents * 10
    : null
  const cutMet = btMs !== null && qualMs !== null ? btMs <= qualMs : null

  return (
    <div
      aria-label={selected ? 'Selected by your parent or guardian' : undefined}
      style={{ textAlign: 'left', borderRadius: '12px', border: `1px solid ${selected ? C.accentBorder : C.border}`, background: selected ? C.accentDim : 'rgba(255,255,255,0.03)', padding: '12px 16px', color: C.text, fontFamily: 'var(--font-body)' }}
    >
      <div className="is-9fcafc6">
        <div>
          <div className="is-dd1df4f">
            {ev.event_number ? `#${ev.event_number} ` : ''}{eventName(ev)}
          </div>
          <div style={{ fontSize: 'var(--fs-xs)', color: C.subtle }}>
            {ev.age_group ?? ''}
            {qualMs !== null && (
              <span style={{ marginLeft: ev.age_group ? '8px' : 0 }}>
                Cut {formatSwimTime(qualMs)}
              </span>
            )}
          </div>
        </div>
        <div style={{ textAlign: 'right' }}>
          <div style={{ fontSize: 'var(--fs-base)', fontFamily: 'var(--font-body)', color: C.accent, fontWeight: 700 }}>
            {btLabel}
          </div>
          {cutMet !== null && (
            <div style={{ fontSize: 'var(--fs-2xs)', color: cutMet ? C.accent : 'var(--warning)' }}>
              {cutMet ? 'meets cut' : 'under cut'}
            </div>
          )}
        </div>
      </div>
      {selected && (
        <div style={{ marginTop: '6px', fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', fontWeight: 600, color: C.accent }}>
          Chosen for you
        </div>
      )}
    </div>
  )
}
