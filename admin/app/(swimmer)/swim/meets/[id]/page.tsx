'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import {
  HOUSEHOLD_SWIMMERS,
  PRIMARY_SWIMMER_ID,
  getDefaultHouseholdMeetStates,
  getMeetById,
  getSwimmerVisibleDocuments,
} from '@/lib/meetCenter'

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

const SWIMMER = HOUSEHOLD_SWIMMERS.find((swimmer) => swimmer.id === PRIMARY_SWIMMER_ID) ?? HOUSEHOLD_SWIMMERS[0]

function parseTimeToMs(t: string): number {
  const parts = t.split(':')
  if (parts.length === 1) return parseFloat(t) * 1000
  const [min, sec] = parts
  return parseInt(min, 10) * 60000 + parseFloat(sec) * 1000
}

export default function SwimmerMeetDetailPage() {
  const params = useParams<{ id: string }>()
  const meetId = typeof params?.id === 'string' ? params.id : ''
  const meet = getMeetById(meetId)
  const initialState = getDefaultHouseholdMeetStates(meetId)[SWIMMER.id]
  const [commitment, setCommitment] = useState<'pending' | 'yes' | 'no'>(initialState.commitment)
  const [selectedEvents, setSelectedEvents] = useState<string[]>(initialState.selectedEvents)

  useEffect(() => {
    const nextState = getDefaultHouseholdMeetStates(meetId)[SWIMMER.id]
    setCommitment(nextState.commitment)
    setSelectedEvents(nextState.selectedEvents)
  }, [meetId])

  if (!meet) {
    return (
      <div style={{ maxWidth: '720px' }}>
        <Link
          href="/swim/schedule"
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '5px',
            color: C.subtle,
            textDecoration: 'none',
            fontSize: '13px',
            marginBottom: '22px',
          }}
        >
          <Icon name="chevron-left" size={14} />
          Schedule
        </Link>
        <div
          style={{
            background: C.card,
            border: `1px solid ${C.border}`,
            borderRadius: '16px',
            padding: '24px 26px',
          }}
        >
          <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.accent, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
            Athlete Meet Center
          </div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: C.text, lineHeight: 1.05, marginBottom: '10px' }}>
            Meet not found
          </h1>
          <p style={{ fontSize: '13px', color: C.subtle, lineHeight: 1.7 }}>
            No swimmer meet detail was found for <span style={{ fontFamily: 'var(--font-mono)', color: C.text }}>{meetId || 'this route'}</span>.
          </p>
        </div>
      </div>
    )
  }

  const sessions = meet.sessions.map((session) => ({
    ...session,
    events: meet.events.filter(
      (event) => event.sessionId === session.id && event.gender === SWIMMER.gender,
    ),
  })).filter((session) => session.events.length > 0)

  const deadlineDate = new Date(meet.deadlineDate)
  const documents = getSwimmerVisibleDocuments(meet)
  const commitmentOpen = meet.status === 'commitment_open'

  const toggleEvent = (eventId: string) => {
    if (!commitmentOpen) return
    setSelectedEvents((prev) =>
      prev.includes(eventId) ? prev.filter((item) => item !== eventId) : [...prev, eventId],
    )
  }

  return (
    <div style={{ maxWidth: '980px' }}>
      <Link
        href="/swim/schedule"
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '5px',
          color: C.subtle,
          textDecoration: 'none',
          fontSize: '13px',
          marginBottom: '22px',
        }}
      >
        <Icon name="chevron-left" size={14} />
        Schedule
      </Link>

      <div
        style={{
          background: C.card,
          border: `1px solid ${C.border}`,
          borderRadius: '16px',
          padding: '24px 26px',
          marginBottom: '20px',
          boxShadow: '0 16px 40px rgba(0,0,0,0.18)',
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: '16px', flexWrap: 'wrap' }}>
          <div>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.accent, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
              Athlete Meet Center
            </div>
            <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: C.text, lineHeight: 1.05, marginBottom: '10px' }}>
              {meet.name}
            </h1>
            <div style={{ display: 'flex', gap: '14px', flexWrap: 'wrap', fontSize: '13px', color: C.subtle }}>
              <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                <Icon name="calendar" size={12} />
                {meet.dateLabel}
              </span>
              <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                <Icon name="map-pin" size={12} />
                {meet.location}
              </span>
              <span style={{ fontFamily: 'var(--font-mono)', color: C.text }}>{meet.course}</span>
            </div>
          </div>

          <div
            style={{
              padding: '12px 14px',
              borderRadius: '12px',
              background: 'rgba(245,158,11,0.12)',
              border: '1px solid rgba(245,158,11,0.22)',
              minWidth: '180px',
            }}
          >
            <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: C.warning, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '4px' }}>
              Deadline
            </div>
            <div style={{ fontSize: '15px', fontWeight: 700, color: '#fcd34d' }}>
              {deadlineDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
            </div>
            <div style={{ fontSize: '12px', color: '#fde68a', marginTop: '2px' }}>{meet.selfSignupNote}</div>
          </div>
        </div>

        <p style={{ marginTop: '16px', fontSize: '13px', color: C.subtle, lineHeight: 1.7 }}>
          {meet.overview}
        </p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) 290px', gap: '18px', alignItems: 'start' }}>
        <div
          style={{
            background: C.card,
            border: `1px solid ${C.border}`,
            borderRadius: '16px',
            overflow: 'hidden',
          }}
        >
          <div style={{ padding: '18px 20px', borderBottom: `1px solid ${C.border}` }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
              <div
                style={{
                  width: '42px',
                  height: '42px',
                  borderRadius: '50%',
                  background: 'linear-gradient(135deg, #00D4AA 0%, #0099CC 100%)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: '#061623',
                  fontFamily: 'var(--font-mono)',
                  fontWeight: 700,
                }}
              >
                {SWIMMER.initials}
              </div>
              <div style={{ flex: 1 }}>
                <div style={{ fontFamily: 'var(--font-display)', fontSize: '20px', color: C.text }}>{SWIMMER.name}</div>
                <div style={{ fontSize: '12px', color: C.subtle }}>Age {SWIMMER.age} · Session-split event signup</div>
              </div>
            </div>
          </div>

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
                  style={{
                    borderRadius: '10px',
                    border: `1px solid ${commitment === option.id ? option.border : C.border}`,
                    background: commitment === option.id ? option.bg : 'transparent',
                    color: commitment === option.id ? option.color : C.text,
                    padding: '10px 16px',
                    fontSize: '13px',
                    fontWeight: 600,
                    cursor: commitmentOpen ? 'pointer' : 'not-allowed',
                    opacity: commitmentOpen ? 1 : 0.6,
                  }}
                >
                  {option.label}
                </button>
              ))}
            </div>
            {!commitmentOpen && (
              <div style={{ marginTop: '12px', fontSize: '12px', color: '#93c5fd', lineHeight: 1.6 }}>
                This meet is visible now, but self-signup stays read-only until the commitment window opens.
              </div>
            )}
          </div>

          {commitmentOpen && commitment !== 'no' && (
            <div style={{ padding: '18px 20px' }}>
              <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '12px' }}>
                Session-Split Signup
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {sessions.map((session) => (
                  <div
                    key={session.id}
                    style={{
                      borderRadius: '14px',
                      border: `1px solid ${C.border}`,
                      background: 'rgba(255,255,255,0.02)',
                      padding: '14px',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '10px', flexWrap: 'wrap' }}>
                      <div>
                        <div style={{ fontSize: '14px', fontWeight: 700, color: C.text }}>{session.label}</div>
                        <div style={{ fontSize: '12px', color: C.subtle }}>
                          Warm-up {session.warmup} · Start {session.start}
                        </div>
                      </div>
                      <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: C.accent, background: C.accentDim, border: `1px solid ${C.accentBorder}`, borderRadius: '999px', padding: '3px 8px' }}>
                        {session.events.length} choices
                      </span>
                    </div>

                    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                      {session.events.map((event) => {
                        const bestTime = SWIMMER.bestTimes[event.name as keyof typeof SWIMMER.bestTimes]
                        const cutMet = bestTime && event.cut ? parseTimeToMs(bestTime) <= parseTimeToMs(event.cut) : null
                        const selected = selectedEvents.includes(event.id)

                        return (
                          <button
                            key={event.id}
                            onClick={() => toggleEvent(event.id)}
                            style={{
                              textAlign: 'left',
                              borderRadius: '12px',
                              border: `1px solid ${selected ? C.accentBorder : C.border}`,
                              background: selected ? C.accentDim : 'rgba(255,255,255,0.03)',
                              padding: '12px 14px',
                              cursor: 'pointer',
                              color: C.text,
                            }}
                          >
                            <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center' }}>
                              <div>
                                <div style={{ fontWeight: 600, fontSize: '14px' }}>#{event.number} {event.name}</div>
                                <div style={{ fontSize: '11.5px', color: C.subtle }}>
                                  {event.ageGroup}
                                  {event.cut && <span style={{ marginLeft: '8px' }}>Cut {event.cut}</span>}
                                </div>
                              </div>
                              <div style={{ textAlign: 'right' }}>
                                <div style={{ fontSize: '13px', fontFamily: 'var(--font-mono)', color: C.accent, fontWeight: 700 }}>
                                  {bestTime ?? 'NT'}
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
                      })}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          <div
            style={{
              background: C.card,
              border: `1px solid ${C.border}`,
              borderRadius: '16px',
              padding: '18px',
            }}
          >
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '10px' }}>
              Meet Files
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {documents.map((doc) => (
                <a key={doc.id} href={doc.url} target="_blank" rel="noreferrer" style={{ textDecoration: 'none', color: 'inherit' }}>
                  <div
                    style={{
                      borderRadius: '12px',
                      border: `1px solid ${C.border}`,
                      background: 'rgba(255,255,255,0.03)',
                      padding: '12px',
                      display: 'flex',
                      alignItems: 'center',
                      gap: '10px',
                    }}
                  >
                    <div
                      style={{
                        width: '30px',
                        height: '30px',
                        borderRadius: '8px',
                        background: 'rgba(255,255,255,0.06)',
                        border: `1px solid ${C.border}`,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: doc.type === 'image' ? '#93c5fd' : C.accent,
                        flexShrink: 0,
                      }}
                    >
                      <Icon name={doc.type === 'image' ? 'eye' : 'file-text'} size={14} />
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontSize: '12.5px', fontWeight: 600, color: C.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {doc.name}
                      </div>
                      <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: C.muted }}>
                        {doc.size} · updated {doc.updatedAt}
                      </div>
                    </div>
                    <Icon name="download-cloud" size={14} style={{ color: C.subtle }} />
                  </div>
                </a>
              ))}
            </div>
          </div>

          <div
            style={{
              background: C.card,
              border: `1px solid ${C.border}`,
              borderRadius: '16px',
              padding: '18px',
            }}
          >
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.subtle, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '10px' }}>
              Summary
            </div>
            <div style={{ fontSize: '14px', fontWeight: 600, color: C.text, marginBottom: '6px' }}>
              {selectedEvents.length} selected event{selectedEvents.length !== 1 ? 's' : ''}
            </div>
            <div style={{ fontSize: '12px', color: C.subtle, lineHeight: 1.6 }}>
              Estimated entry cost ${(
                selectedEvents.length * meet.entryFeePerEvent
              ).toFixed(2)}. Coaches can still adjust entries before final submission.
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
