'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import {
  HOUSEHOLD_SWIMMERS,
  getDefaultHouseholdMeetStates,
  getFamilyVisibleDocuments,
  getMeetById,
} from '@/lib/meetCenter'

const SWIMMERS = HOUSEHOLD_SWIMMERS

type SwimmerState = {
  commitment: 'pending' | 'yes' | 'no'
  selectedEvents: string[]
}

// ── Component ────────────────────────────────────────────────────────────────
export default function MeetDetailPage() {
  const params = useParams<{ id: string }>()
  const meetId = typeof params?.id === 'string' ? params.id : ''
  const MEET = getMeetById(meetId)
  const initialSwimmerStates = getDefaultHouseholdMeetStates(meetId)
  const [swimmerStates, setSwimmerStates] = useState<Record<string, SwimmerState>>(initialSwimmerStates)
  const [activeSwimmer, setActiveSwimmer] = useState<string>(SWIMMERS[0].id)
  const [submitting, setSubmitting] = useState<string | null>(null)
  const [submitted, setSubmitted] = useState<Record<string, boolean>>({})

  useEffect(() => {
    setSwimmerStates(getDefaultHouseholdMeetStates(meetId))
    setActiveSwimmer(SWIMMERS[0].id)
    setSubmitting(null)
    setSubmitted({})
  }, [meetId])

  if (!MEET) {
    return (
      <div style={{ maxWidth: '720px' }}>
        <nav style={{ marginBottom: '20px', fontSize: '13px' }}>
          <Link href="/portal/meets" style={{ color: '#9A9693', textDecoration: 'none' }}>
            Meets
          </Link>
        </nav>
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
            Meet not found
          </h1>
          <p style={{ fontSize: '14px', color: '#6B6866', lineHeight: 1.6, marginBottom: '16px' }}>
            We couldn&apos;t find a meet for <span style={{ fontFamily: 'var(--font-mono)' }}>{meetId || 'this route'}</span>.
            Return to the meet list to open an available invite.
          </p>
          <Link href="/portal/meets" style={{ fontSize: '13px', fontWeight: 600 }}>
            Back to all meets
          </Link>
        </div>
      </div>
    )
  }

  const EVENTS = MEET.events
  const SESSIONS = MEET.sessions
  const DOCUMENTS = getFamilyVisibleDocuments(MEET)
  const deadlineDate = new Date(MEET.deadlineDate)
  const now = new Date()
  const daysUntilDeadline = Math.max(
    0,
    Math.ceil((deadlineDate.getTime() - now.getTime()) / 86400000),
  )

  const swimmer = SWIMMERS.find((s) => s.id === activeSwimmer)!
  const state = swimmerStates[activeSwimmer]
  const sessionById = Object.fromEntries(SESSIONS.map((session) => [session.id, session]))

  const eligibleEvents = EVENTS.filter((ev) => ev.gender === swimmer.gender)
  const eligibleSessions = SESSIONS
    .map((session) => ({
      ...session,
      events: eligibleEvents.filter((event) => event.sessionId === session.id),
    }))
    .filter((session) => session.events.length > 0)
  const commitmentOpen = MEET.status === 'commitment_open'

  const setCommitment = async (swimmerId: string, val: 'yes' | 'no') => {
    setSubmitting(swimmerId + val)
    await new Promise((r) => setTimeout(r, 600))
    setSwimmerStates((prev) => ({
      ...prev,
      [swimmerId]: {
        ...prev[swimmerId],
        commitment: val,
        selectedEvents: val === 'no' ? [] : prev[swimmerId].selectedEvents,
      },
    }))
    setSubmitting(null)
    setSubmitted((prev) => ({ ...prev, [swimmerId]: true }))
    setTimeout(() => setSubmitted((prev) => ({ ...prev, [swimmerId]: false })), 2500)
  }

  const toggleEvent = (eventId: string) => {
    setSwimmerStates((prev) => {
      const current = prev[activeSwimmer].selectedEvents
      const next = current.includes(eventId)
        ? current.filter((e) => e !== eventId)
        : [...current, eventId]
      return { ...prev, [activeSwimmer]: { ...prev[activeSwimmer], selectedEvents: next } }
    })
  }

  const totalEvents = Object.values(swimmerStates).reduce(
    (acc, s) => acc + s.selectedEvents.length, 0
  )
  const estimatedFee = totalEvents * MEET.entryFeePerEvent

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
        <span style={{ color: '#1A1714', fontWeight: 500 }}>{MEET.name}</span>
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
              {MEET.name}
            </h1>
            <div style={{ display: 'flex', gap: '16px', flexWrap: 'wrap', fontSize: '13px', color: '#6B6866' }}>
              <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                <Icon name="calendar" size={13} style={{ color: '#9A9693' }} />
                {MEET.dateLabel}
              </span>
              <span style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                <Icon name="map-pin" size={13} style={{ color: '#9A9693' }} />
                {MEET.location}
              </span>
              <span
                style={{
                  padding: '2px 8px',
                  background: 'rgba(0,0,0,0.05)',
                  borderRadius: '5px',
                  fontFamily: 'var(--font-mono)',
                  fontSize: '11px',
                }}
              >
                {MEET.course}
              </span>
            </div>
          </div>

          {/* Deadline urgency */}
          <div
            style={{
              background: daysUntilDeadline <= 3 ? '#fef2f2' : '#fffbeb',
              border: `1px solid ${daysUntilDeadline <= 3 ? 'rgba(220,38,38,0.2)' : 'rgba(217,119,6,0.2)'}`,
              borderRadius: '10px',
              padding: '10px 16px',
              textAlign: 'center',
            }}
          >
            <div style={{ fontSize: '11px', color: daysUntilDeadline <= 3 ? '#dc2626' : '#d97706', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '2px' }}>
              Deadline
            </div>
            <div style={{ fontSize: '14px', fontWeight: 700, color: daysUntilDeadline <= 3 ? '#dc2626' : '#92400e' }}>
              {deadlineDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
            </div>
            <div style={{ fontSize: '12px', fontWeight: 600, color: daysUntilDeadline <= 3 ? '#dc2626' : '#d97706', marginTop: '2px' }}>
              {daysUntilDeadline <= 0 ? 'Closed' : daysUntilDeadline === 1 ? '1 day left' : `${daysUntilDeadline} days left`}
            </div>
          </div>
        </div>

        {/* Notes */}
        {MEET.notes && (
          <div
            style={{
              marginTop: '16px',
              padding: '10px 14px',
              background: 'rgba(0,0,0,0.03)',
              borderRadius: '8px',
              fontSize: '13px',
              color: '#6B6866',
              lineHeight: 1.5,
              borderLeft: '3px solid rgba(235,93,30,0.25)',
            }}
          >
            <strong style={{ color: '#1A1714', fontWeight: 600 }}>Note:</strong> {MEET.notes}
          </div>
        )}
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
              {DOCUMENTS.length} files
            </span>
          </div>

          <p style={{ fontSize: '13px', color: '#6B6866', lineHeight: 1.7, marginBottom: '16px' }}>
            {MEET.overview}
          </p>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: '10px', marginBottom: '16px' }}>
            {[
              { label: 'Host', value: MEET.host },
              { label: 'Course', value: MEET.course },
              { label: 'Location', value: MEET.location },
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

          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
            {SESSIONS.map((session) => (
              <div
                key={session.id}
                style={{
                  padding: '12px 14px',
                  borderRadius: '12px',
                  background: 'rgba(235,93,30,0.04)',
                  border: '1px solid rgba(235,93,30,0.14)',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: '10px', alignItems: 'center', marginBottom: '6px', flexWrap: 'wrap' }}>
                  <div style={{ fontSize: '14px', fontWeight: 600, color: '#1A1714' }}>{session.label}</div>
                  <div style={{ fontSize: '11px', color: '#9A9693', fontFamily: 'var(--font-mono)' }}>
                    Warm-up {session.warmup} · Start {session.start}
                  </div>
                </div>
                <div style={{ fontSize: '12.5px', color: '#6B6866', lineHeight: 1.55 }}>{session.notes}</div>
              </div>
            ))}
          </div>
        </div>

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
          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
            {DOCUMENTS.map((doc) => (
              <a
                key={doc.id}
                href={doc.url}
                target="_blank"
                rel="noreferrer"
                style={{
                  textDecoration: 'none',
                  color: 'inherit',
                  display: 'block',
                }}
              >
                <div
                  style={{
                    padding: '11px 12px',
                    borderRadius: '10px',
                    border: '1px solid rgba(0,0,0,0.08)',
                    background:
                      doc.type === 'image'
                        ? 'rgba(59,130,246,0.05)'
                        : 'rgba(235,93,30,0.04)',
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
                      background: '#ffffff',
                      border: '1px solid rgba(0,0,0,0.06)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      color: doc.type === 'image' ? '#3b82f6' : '#eb5d1e',
                      flexShrink: 0,
                    }}
                  >
                    <Icon name={doc.type === 'image' ? 'eye' : 'file-text'} size={14} />
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: '12.5px', fontWeight: 600, color: '#1A1714', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {doc.name}
                    </div>
                    <div style={{ fontSize: '10.5px', color: '#9A9693', fontFamily: 'var(--font-mono)' }}>
                      {doc.size} · updated {doc.updatedAt}
                    </div>
                  </div>
                  <Icon name="download-cloud" size={14} style={{ color: '#9A9693', flexShrink: 0 }} />
                </div>
              </a>
            ))}
          </div>
        </div>
      </div>

      {/* Swimmer tabs */}
      <div
        style={{
          display: 'flex',
          gap: '8px',
          marginBottom: '20px',
          animation: 'slideUp 0.4s ease both',
          flexWrap: 'wrap',
        }}
      >
        {SWIMMERS.map((s) => {
          const st = swimmerStates[s.id]
          const isActive = activeSwimmer === s.id
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
                  background: s.gradient,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: '#fff',
                  fontSize: '9px',
                  fontWeight: 700,
                  fontFamily: 'var(--font-mono)',
                }}
              >
                {s.initials}
              </div>
              <span style={{ fontSize: '13.5px', fontWeight: 600, color: isActive ? '#eb5d1e' : '#1A1714' }}>
                {s.name.split(' ')[0]}
              </span>
              {/* Commitment badge */}
              <span
                style={{
                  padding: '2px 6px',
                  borderRadius: '5px',
                  fontSize: '10.5px',
                  fontWeight: 600,
                  background: st.commitment === 'yes' ? '#ecfdf5' : st.commitment === 'no' ? '#fef2f2' : '#fffbeb',
                  color: st.commitment === 'yes' ? '#059669' : st.commitment === 'no' ? '#dc2626' : '#d97706',
                }}
              >
                {st.commitment === 'yes' ? 'Going' : st.commitment === 'no' ? 'Not Going' : 'Pending'}
              </span>
            </button>
          )
        })}
      </div>

      {/* Commitment card for active swimmer */}
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
              background: swimmer.gradient,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#fff',
              fontSize: '14px',
              fontWeight: 700,
              fontFamily: 'var(--font-mono)',
            }}
          >
            {swimmer.initials}
          </div>
          <div>
            <div style={{ fontFamily: 'var(--font-display)', fontSize: '18px', color: '#1A1714' }}>
              {swimmer.name}
            </div>
            <div style={{ fontSize: '12.5px', color: '#9A9693' }}>
              Age {swimmer.age} · {swimmer.gender === 'F' ? 'Female' : 'Male'}
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
              disabled={submitting !== null || !commitmentOpen}
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
                opacity: submitting !== null || !commitmentOpen ? 0.6 : 1,
              }}
            >
              <Icon name="check-circle" size={16} style={{ color: state.commitment === 'yes' ? '#059669' : '#9A9693' }} />
              {submitting === swimmer.id + 'yes' ? 'Saving…' : 'Yes, we\'re going'}
            </button>
            <button
              onClick={() => setCommitment(swimmer.id, 'no')}
              disabled={submitting !== null || !commitmentOpen}
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
                opacity: submitting !== null || !commitmentOpen ? 0.6 : 1,
              }}
            >
              <Icon name="x-circle" size={16} style={{ color: state.commitment === 'no' ? '#dc2626' : '#9A9693' }} />
              {submitting === swimmer.id + 'no' ? 'Saving…' : 'Not this time'}
            </button>
          </div>
          {!commitmentOpen && (
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
              Select the events {swimmer.name.split(' ')[0]} would like to enter. Coach may adjust selections before the deadline.
            </p>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {eligibleSessions.map((session) => (
                <div key={session.id} style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
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
                    <div>
                      <div style={{ fontSize: '13px', fontWeight: 600, color: '#1A1714' }}>{session.label}</div>
                      <div style={{ fontSize: '11.5px', color: '#9A9693' }}>
                        Warm-up {session.warmup} · Start {session.start}
                      </div>
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
                      {session.events.length} options
                    </span>
                  </div>

                  {session.events.map((ev) => {
                const isSelected = state.selectedEvents.includes(ev.id)
                const bestTime = swimmer.bestTimes[ev.name as keyof typeof swimmer.bestTimes]
                const cutMet = bestTime && ev.cut
                  ? parseTimeToMs(bestTime) <= parseTimeToMs(ev.cut)
                  : null

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
                      cursor: 'pointer',
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
                      #{ev.number}
                    </span>

                    {/* Event name */}
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontSize: '14px', fontWeight: 600, color: '#1A1714' }}>
                          {ev.name}
                        </div>
                        <div style={{ fontSize: '12px', color: '#9A9693' }}>
                        {ev.ageGroup}
                        <span style={{ marginLeft: '8px' }}>{sessionById[ev.sessionId]?.label}</span>
                        {ev.cut && <span style={{ marginLeft: '8px' }}>Cut: {ev.cut}</span>}
                        </div>
                      </div>

                    {/* Best time + cut eligibility hint */}
                    <div style={{ textAlign: 'right', flexShrink: 0 }}>
                      {bestTime ? (
                        <>
                          <div
                            style={{
                              fontSize: '13.5px',
                              fontFamily: 'var(--font-mono)',
                              color: '#4B7BF5',
                              fontWeight: 600,
                            }}
                          >
                            {bestTime}
                          </div>
                          {ev.cut && (
                            <div
                              style={{
                                fontSize: '10.5px',
                                color: cutMet ? '#059669' : '#d97706',
                                fontWeight: 500,
                                marginTop: '2px',
                              }}
                            >
                              {cutMet ? '✓ Meets cut' : '⚠ Under cut time'}
                            </div>
                          )}
                        </>
                      ) : (
                        <span style={{ fontSize: '12px', color: '#C0BDBB', fontStyle: 'italic' }}>
                          No best time
                        </span>
                      )}
                    </div>
                    </div>
                  )
                  })}
                </div>
              ))}
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
                <span style={{ fontSize: '13.5px', color: '#eb5d1e', fontWeight: 600, fontFamily: 'var(--font-mono)' }}>
                  Est. ${(state.selectedEvents.length * MEET.entryFeePerEvent).toFixed(2)}
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
              {swimmer.name.split(' ')[0]} is not attending this meet. You can change this before the deadline.
            </span>
          </div>
        )}
      </div>

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
              ${MEET.entryFeePerEvent}/event × {totalEvents} event{totalEvents !== 1 ? 's' : ''} across all swimmers
            </div>
          </div>
          <div
            style={{
              fontSize: '22px',
              fontWeight: 700,
              color: '#1A1714',
              fontFamily: 'var(--font-mono)',
            }}
          >
            ${estimatedFee.toFixed(2)}
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

// ── Time comparison utility ──────────────────────────────────────────────────
function parseTimeToMs(t: string): number {
  const parts = t.split(':')
  if (parts.length === 1) return parseFloat(t) * 1000
  const [min, sec] = parts
  return parseInt(min) * 60000 + parseFloat(sec) * 1000
}
