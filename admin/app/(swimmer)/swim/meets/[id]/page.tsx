'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchMeet,
  fetchMeetSessions,
  fetchMeetCommitments,
  fetchMeetEvents,
  fetchSwimmers,
  fetchSwimmerBestTimes,
  type MeetRecord,
  type MeetSessionRecord,
  type MeetEventRecord,
  type CommitmentRecord,
  type BestTimeRecord,
} from '@/lib/adminData'

const C = {
  bg: '#061623',
  card: '#0D2235',
  text: '#E4F2FF',
  subtle: '#7FA8C9',
  muted: '#4A6B82',
  accent: '#00D4AA',
  accentDim: 'rgba(0,212,170,0.10)',
  accentBorder: 'rgba(0,212,170,0.22)',
  border: 'rgba(0,212,170,0.10)',
  warning: '#F59E0B',
}

// ── Helpers ──────────────────────────────────────────────────────────────────
function fmtDateLabel(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function fmtTimeFull(isoOrTime?: string): string {
  if (!isoOrTime) return ''
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

function mapCommitStatus(status?: string): 'pending' | 'yes' | 'no' {
  if (status === 'committed_yes') return 'yes'
  if (status === 'committed_no') return 'no'
  return 'pending'
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

  const [meet, setMeet] = useState<MeetRecord | null>(null)
  const [sessions, setSessions] = useState<MeetSessionRecord[]>([])
  const [events, setEvents] = useState<MeetEventRecord[]>([])
  const [bestTimes, setBestTimes] = useState<BestTimeRecord[]>([])
  const [swimmerName, setSwimmerName] = useState<string>('')
  const [swimmerGender, setSwimmerGender] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)

  const [commitment, setCommitment] = useState<'pending' | 'yes' | 'no'>('pending')
  const [selectedEvents, setSelectedEvents] = useState<string[]>([])

  useEffect(() => {
    if (!orgId || !meetId || !householdId) { setLoading(false); return }
    setLoading(true)

    Promise.all([
      fetchMeet(api, orgId, meetId).catch(() => null),
      fetchSwimmers(api, orgId),
    ])
      .then(async ([meetData, allSwimmers]) => {
        if (!meetData) { setNotFound(true); setLoading(false); return }
        setMeet(meetData)

        const hhSwimmer = allSwimmers.find((s) => s.household_id === householdId)
        if (!hhSwimmer) { setLoading(false); return }
        setSwimmerName(`${hhSwimmer.first_name} ${hhSwimmer.last_name}`)
        setSwimmerGender(hhSwimmer.gender ?? '')

        const [meetSessions, meetEvents, commitments, times] = await Promise.all([
          fetchMeetSessions(api, orgId, meetId).catch(() => [] as MeetSessionRecord[]),
          fetchMeetEvents(api, orgId, meetId).catch(() => [] as MeetEventRecord[]),
          fetchMeetCommitments(api, orgId, meetId).catch(() => [] as CommitmentRecord[]),
          fetchSwimmerBestTimes(api, orgId, hhSwimmer.id).catch(() => [] as BestTimeRecord[]),
        ])

        setSessions(meetSessions)
        setEvents(meetEvents)
        setBestTimes(times)

        const myCommitment = commitments.find((c) => c.swimmer_id === hhSwimmer.id)
        setCommitment(mapCommitStatus(myCommitment?.status))
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [api, orgId, meetId, householdId])

  const commitmentOpen = meet?.status === 'commitment_open'
  const deadlineDate = meet?.commitment_deadline ? new Date(meet.commitment_deadline) : null

  const toggleEvent = (eventId: string) => {
    if (!commitmentOpen) return
    setSelectedEvents((prev) =>
      prev.includes(eventId) ? prev.filter((item) => item !== eventId) : [...prev, eventId],
    )
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
        <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '16px', padding: '24px 26px' }}>
          <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.accent, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
            Athlete Meet Center
          </div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: C.text, lineHeight: 1.05, marginBottom: '10px' }}>Meet not found</h1>
          <p style={{ fontSize: '13px', color: C.subtle, lineHeight: 1.7 }}>
            No meet was found for <span style={{ fontFamily: 'var(--font-mono)', color: C.text }}>{meetId || 'this route'}</span>.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div style={{ maxWidth: '980px' }}>
      <style>{`@keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }`}</style>

      <Link href="/swim/schedule" style={{ display: 'inline-flex', alignItems: 'center', gap: '5px', color: C.subtle, textDecoration: 'none', fontSize: '13px', marginBottom: '22px' }}>
        <Icon name="chevron-left" size={14} />
        Schedule
      </Link>

      {/* Meet header */}
      <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '16px', padding: '24px 26px', marginBottom: '20px', boxShadow: '0 16px 40px rgba(0,0,0,0.18)' }}>
        {loading ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <Skeleton w="55%" h={32} r={6} />
            <Skeleton w="70%" h={14} />
          </div>
        ) : (
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: '16px', flexWrap: 'wrap' }}>
            <div>
              <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.accent, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
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
                  <span style={{ fontFamily: 'var(--font-mono)', color: C.text }}>{meet.course}</span>
                )}
              </div>
            </div>

            {deadlineDate && (
              <div style={{ padding: '12px 14px', borderRadius: '12px', background: 'rgba(245,158,11,0.12)', border: '1px solid rgba(245,158,11,0.22)', minWidth: '180px' }}>
                <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: C.warning, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '4px' }}>
                  Commitment Deadline
                </div>
                <div style={{ fontSize: '15px', fontWeight: 700, color: '#fcd34d' }}>
                  {deadlineDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) 290px', gap: '18px', alignItems: 'start' }}>
        {/* Left: commitment + event signup */}
        <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '16px', overflow: 'hidden' }}>
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
                <div style={{ width: '42px', height: '42px', borderRadius: '50%', background: 'linear-gradient(135deg, #00D4AA 0%, #0099CC 100%)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#061623', fontFamily: 'var(--font-mono)', fontWeight: 700 }}>
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
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '12px' }}>
              Commitment
            </div>
            <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
              {[
                { id: 'yes', label: 'I am going', color: C.accent, bg: C.accentDim, border: C.accentBorder },
                { id: 'no', label: 'Not this meet', color: '#fca5a5', bg: 'rgba(239,68,68,0.10)', border: 'rgba(239,68,68,0.16)' },
              ].map((option) => (
                <button
                  key={option.id}
                  onClick={() => {
                    if (!commitmentOpen) return
                    const next = option.id as 'yes' | 'no'
                    setCommitment(next)
                    if (next === 'no') setSelectedEvents([])
                  }}
                  style={{ borderRadius: '10px', border: `1px solid ${commitment === option.id ? option.border : C.border}`, background: commitment === option.id ? option.bg : 'transparent', color: commitment === option.id ? option.color : C.text, padding: '10px 16px', fontSize: '13px', fontWeight: 600, cursor: commitmentOpen ? 'pointer' : 'not-allowed', opacity: commitmentOpen ? 1 : 0.6, fontFamily: 'var(--font-body)' }}
                >
                  {option.label}
                </button>
              ))}
            </div>
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
              <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '12px' }}>
                Session-Split Signup
              </div>

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
                            {session.warmup_at ? `Warm-up ${fmtTimeFull(session.warmup_at)}` : ''}
                            {session.start_at ? ` · Start ${fmtTimeFull(session.start_at)}` : ''}
                          </div>
                        </div>
                        <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: C.accent, background: C.accentDim, border: `1px solid ${C.accentBorder}`, borderRadius: '999px', padding: '3px 8px' }}>
                          {session.events.length} choices
                        </span>
                      </div>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                        {session.events.map((ev) => <EventButton key={ev.id} ev={ev} meet={meet} bestTimes={bestTimes} selectedEvents={selectedEvents} toggleEvent={toggleEvent} />)}
                      </div>
                    </div>
                  ))}

                  {/* Unsessioned events */}
                  {unsessionedEvents.length > 0 && (
                    <div style={{ borderRadius: '14px', border: `1px solid ${C.border}`, background: 'rgba(255,255,255,0.02)', padding: '14px' }}>
                      <div style={{ fontSize: '14px', fontWeight: 700, color: C.text, marginBottom: '10px' }}>Events</div>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                        {unsessionedEvents.map((ev) => <EventButton key={ev.id} ev={ev} meet={meet} bestTimes={bestTimes} selectedEvents={selectedEvents} toggleEvent={toggleEvent} />)}
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
          {/* Meet files placeholder */}
          <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '16px', padding: '18px' }}>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '10px' }}>
              Meet Files
            </div>
            <div style={{ fontSize: '13px', color: C.muted, textAlign: 'center', padding: '10px 0', lineHeight: 1.6 }}>
              <Icon name="file-text" size={18} style={{ display: 'block', margin: '0 auto 8px', color: C.muted }} />
              Files will appear here when available.
            </div>
          </div>

          {/* Summary */}
          <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '16px', padding: '18px' }}>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '10px' }}>
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
    </div>
  )
}

// ── Event row button ─────────────────────────────────────────────────────────
function EventButton({
  ev,
  meet,
  bestTimes,
  selectedEvents,
  toggleEvent,
}: {
  ev: MeetEventRecord
  meet: MeetRecord | null
  bestTimes: BestTimeRecord[]
  selectedEvents: string[]
  toggleEvent: (id: string) => void
}) {
  const selected = selectedEvents.includes(ev.id)
  const btMs = meet ? getBestTimeMs(bestTimes, ev, meet.course) : null
  const btLabel = btMs !== null ? fmtMs(btMs) : 'NT'

  const qualMs = ev.qualifying_time_cents != null
    ? ev.qualifying_time_cents * 10
    : null
  const cutMet = btMs !== null && qualMs !== null ? btMs <= qualMs : null

  return (
    <button
      onClick={() => toggleEvent(ev.id)}
      style={{ textAlign: 'left', borderRadius: '12px', border: `1px solid ${selected ? C.accentBorder : C.border}`, background: selected ? C.accentDim : 'rgba(255,255,255,0.03)', padding: '12px 14px', cursor: 'pointer', color: C.text, fontFamily: 'var(--font-body)' }}
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
          <div style={{ fontSize: '13px', fontFamily: 'var(--font-mono)', color: C.accent, fontWeight: 700 }}>
            {btLabel}
          </div>
          {cutMet !== null && (
            <div style={{ fontSize: '10px', color: cutMet ? C.accent : '#fbbf24' }}>
              {cutMet ? 'meets cut' : 'under cut'}
            </div>
          )}
        </div>
      </div>
    </button>
  )
}
