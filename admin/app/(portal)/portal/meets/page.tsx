'use client'

import Link from 'next/link'
import { Icon } from '@/components/ui/icons'

// ── Stub data ──────────────────────────────────────────────────────────────
type MeetStatus = 'draft' | 'published' | 'commitment_open' | 'commitment_closed' | 'entry_finalized' | 'completed' | 'archived'
type CommitmentStatus = 'pending' | 'yes' | 'no' | 'open' | 'closed'

type MeetCard = {
  id: string
  name: string
  date: string
  deadlineDate: string
  course: string
  location: string
  status: MeetStatus
  swimmers: Array<{ id: string; name: string; initials: string; gradient: string; commitment: CommitmentStatus }>
}

const MEETS: MeetCard[] = [
  {
    id: 'meet-001',
    name: 'Tri-City Classic',
    date: 'Mar 22, 2026',
    deadlineDate: '2026-03-19T23:59:59',
    course: 'SCY',
    location: 'Riverside Aquatic Center',
    status: 'commitment_open' as const,
    swimmers: [
      { id: 'sw-001', name: 'Emma Chen', initials: 'EC', gradient: 'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)', commitment: 'pending' as const },
      { id: 'sw-002', name: 'Noah Chen',  initials: 'NC', gradient: 'linear-gradient(135deg, #10b981 0%, #059669 100%)', commitment: 'pending' as const },
    ],
  },
  {
    id: 'meet-002',
    name: 'Regional Spring Invite',
    date: 'Apr 5, 2026',
    deadlineDate: '2026-04-01T23:59:59',
    course: 'LCM',
    location: 'University Aquatic Center',
    status: 'commitment_open' as const,
    swimmers: [
      { id: 'sw-001', name: 'Emma Chen', initials: 'EC', gradient: 'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)', commitment: 'yes' as const },
      { id: 'sw-002', name: 'Noah Chen',  initials: 'NC', gradient: 'linear-gradient(135deg, #10b981 0%, #059669 100%)', commitment: 'yes' as const },
    ],
  },
  {
    id: 'meet-003',
    name: 'State Junior Championships',
    date: 'May 2, 2026',
    deadlineDate: '2026-04-24T23:59:59',
    course: 'SCY',
    location: 'State Aquatic Complex',
    status: 'published' as const,
    swimmers: [
      { id: 'sw-001', name: 'Emma Chen', initials: 'EC', gradient: 'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)', commitment: 'open' as const },
    ],
  },
  {
    id: 'meet-004',
    name: 'Summer Championship Series',
    date: 'Jun 14, 2026',
    deadlineDate: '2026-06-05T23:59:59',
    course: 'LCM',
    location: 'Olympic Training Center',
    status: 'draft' as const,
    swimmers: [],
  },
  {
    id: 'meet-005',
    name: 'Valley Winter Classic',
    date: 'Jan 18, 2026',
    deadlineDate: '2026-01-12T23:59:59',
    course: 'SCY',
    location: 'Valley Aquatic Center',
    status: 'completed' as const,
    swimmers: [
      { id: 'sw-001', name: 'Emma Chen', initials: 'EC', gradient: 'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)', commitment: 'yes' as const },
    ],
  },
]

// ── Helpers ─────────────────────────────────────────────────────────────────
const COMMITMENT_CONFIG = {
  pending: { label: 'Respond',   bg: '#fffbeb', color: '#d97706', border: 'rgba(217,119,6,0.2)' },
  yes:     { label: 'Going',     bg: '#ecfdf5', color: '#059669', border: 'rgba(5,150,105,0.2)' },
  no:      { label: 'Not Going', bg: '#fef2f2', color: '#dc2626', border: 'rgba(220,38,38,0.2)' },
  open:    { label: 'Opens Soon',bg: '#f0f9ff', color: '#0284c7', border: 'rgba(2,132,199,0.2)' },
  closed:  { label: 'Closed',    bg: '#f9fafb', color: '#9A9693', border: 'rgba(0,0,0,0.1)' },
}

const MEET_STATUS_LABEL: Record<string, string> = {
  draft:              'Draft',
  published:          'Opens Soon',
  commitment_open:    'Commitment Open',
  commitment_closed:  'Commitment Closed',
  entry_finalized:    'Entries Finalized',
  completed:          'Completed',
  archived:           'Archived',
}

function DeadlineCountdown({ days }: { days: number }) {
  if (days < 0) return null
  const urgent = days <= 3
  const warning = days <= 7

  const color = urgent ? '#dc2626' : warning ? '#d97706' : '#6B6866'
  const bg    = urgent ? '#fef2f2' : warning ? '#fffbeb' : 'rgba(0,0,0,0.04)'

  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '5px',
        padding: '3px 9px',
        borderRadius: '999px',
        fontSize: '12px',
        fontWeight: urgent || warning ? 600 : 400,
        color,
        background: bg,
      }}
    >
      {urgent && <Icon name="alert-circle" size={12} style={{ color }} />}
      {days === 0
        ? 'Due today'
        : days === 1
        ? '1 day left'
        : `${days} days left`}
    </span>
  )
}

// ── Component ────────────────────────────────────────────────────────────────
export default function MeetsPage() {
  const meetsWithDeadlines = MEETS.map((meet) => ({
    ...meet,
    daysUntilDeadline: Math.ceil((new Date(meet.deadlineDate).getTime() - Date.now()) / 86400000),
  }))
  const activeMeets    = meetsWithDeadlines.filter((m) => m.status === 'commitment_open')
  const upcomingMeets  = meetsWithDeadlines.filter((m) => m.status === 'entry_finalized')
  const pastMeets      = meetsWithDeadlines.filter((m) => ['completed', 'archived'].includes(m.status))
  const pendingRespond = meetsWithDeadlines.filter((m) =>
    m.swimmers.some((s) => s.commitment === 'pending') && m.status === 'commitment_open'
  ).length

  return (
    <div>
      {/* Page header */}
      <div style={{ marginBottom: '28px', animation: 'slideUp 0.3s ease both' }}>
        <h1
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: '30px',
            color: '#1A1714',
            margin: '0 0 6px',
            lineHeight: 1.1,
          }}
        >
          Meets
        </h1>
        <p style={{ margin: 0, fontSize: '14px', color: '#9A9693' }}>
          Commitment deadlines and meet schedules for your household.
        </p>
      </div>

      {/* Action needed banner */}
      {pendingRespond > 0 && (
        <div
          style={{
            background: 'linear-gradient(135deg, rgba(235,93,30,0.08) 0%, rgba(235,93,30,0.04) 100%)',
            border: '1px solid rgba(235,93,30,0.2)',
            borderRadius: '12px',
            padding: '16px 20px',
            marginBottom: '24px',
            display: 'flex',
            alignItems: 'center',
            gap: '14px',
            animation: 'slideUp 0.35s ease both',
          }}
        >
          <div
            style={{
              width: '40px',
              height: '40px',
              borderRadius: '50%',
              background: 'rgba(235,93,30,0.12)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0,
            }}
          >
            <Icon name="calendar" size={18} style={{ color: '#eb5d1e' }} />
          </div>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: '14px', fontWeight: 600, color: '#8B3312' }}>
              {pendingRespond} meet{pendingRespond !== 1 ? 's' : ''} need{pendingRespond === 1 ? 's' : ''} your response
            </div>
            <div style={{ fontSize: '12.5px', color: '#b8541e', marginTop: '2px' }}>
              Respond before the deadline to secure spots for your swimmers.
            </div>
          </div>
        </div>
      )}

      {/* Active meets (commitment open / opens soon) */}
      {activeMeets.length > 0 && (
        <section style={{ marginBottom: '32px' }}>
          <div
            style={{
              fontSize: '11px',
              fontFamily: 'var(--font-mono)',
              color: '#9A9693',
              textTransform: 'uppercase',
              letterSpacing: '0.08em',
              marginBottom: '14px',
              animation: 'slideUp 0.4s ease both',
            }}
          >
            Needs Action
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            {activeMeets.map((meet, idx) => {
              const hasUrgent = meet.swimmers.some((s) => s.commitment === 'pending') &&
                                meet.daysUntilDeadline <= 3

              return (
                <div
                  key={meet.id}
                  style={{
                    background: '#ffffff',
                    borderRadius: '14px',
                    border: `1px solid ${hasUrgent ? 'rgba(220,38,38,0.15)' : 'rgba(0,0,0,0.07)'}`,
                    boxShadow: hasUrgent
                      ? '0 2px 16px rgba(220,38,38,0.08)'
                      : '0 2px 12px rgba(0,0,0,0.05)',
                    overflow: 'hidden',
                    animation: `slideUp ${0.4 + idx * 0.07}s ease both`,
                  }}
                >
                  {/* Top bar */}
                  <div
                    style={{
                      padding: '18px 22px',
                      display: 'flex',
                      alignItems: 'flex-start',
                      gap: '16px',
                    }}
                  >
                    {/* Date block */}
                    <div
                      style={{
                        width: '52px',
                        borderRadius: '10px',
                        background: hasUrgent ? 'rgba(220,38,38,0.07)' : 'rgba(235,93,30,0.07)',
                        border: `1px solid ${hasUrgent ? 'rgba(220,38,38,0.15)' : 'rgba(235,93,30,0.15)'}`,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        padding: '8px 4px',
                        flexShrink: 0,
                        textAlign: 'center',
                      }}
                    >
                      <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: '#eb5d1e', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                        {meet.date.split(' ')[0]}
                      </span>
                      <span style={{ fontSize: '22px', fontWeight: 700, color: '#1A1714', lineHeight: 1.1, fontFamily: 'var(--font-display)' }}>
                        {meet.date.split(' ')[1].replace(',', '')}
                      </span>
                    </div>

                    {/* Meet info */}
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                        <span
                          style={{
                            fontFamily: 'var(--font-display)',
                            fontSize: '19px',
                            color: '#1A1714',
                            lineHeight: 1.1,
                          }}
                        >
                          {meet.name}
                        </span>
                        <span
                          style={{
                            padding: '2px 8px',
                            background: 'rgba(0,0,0,0.05)',
                            borderRadius: '4px',
                            fontSize: '11px',
                            fontFamily: 'var(--font-mono)',
                            color: '#6B6866',
                          }}
                        >
                          {meet.course}
                        </span>
                      </div>
                      <div
                        style={{
                          fontSize: '12.5px',
                          color: '#9A9693',
                          marginTop: '4px',
                          display: 'flex',
                          alignItems: 'center',
                          gap: '8px',
                          flexWrap: 'wrap',
                        }}
                      >
                        <span>{meet.location}</span>
                        <span>·</span>
                        <span>Deadline: {new Date(meet.deadlineDate).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}</span>
                        <DeadlineCountdown days={meet.daysUntilDeadline} />
                      </div>

                      {/* Per-swimmer commitment status */}
                      <div
                        style={{
                          display: 'flex',
                          gap: '8px',
                          marginTop: '12px',
                          flexWrap: 'wrap',
                        }}
                      >
                        {meet.swimmers.map((swimmer) => {
                          const cfg = COMMITMENT_CONFIG[swimmer.commitment]
                          return (
                            <div
                              key={swimmer.id}
                              style={{
                                display: 'flex',
                                alignItems: 'center',
                                gap: '7px',
                                padding: '5px 10px',
                                background: cfg.bg,
                                border: `1px solid ${cfg.border}`,
                                borderRadius: '8px',
                              }}
                            >
                              <div
                                style={{
                                  width: '20px',
                                  height: '20px',
                                  borderRadius: '50%',
                                  background: swimmer.gradient,
                                  display: 'flex',
                                  alignItems: 'center',
                                  justifyContent: 'center',
                                  color: '#fff',
                                  fontSize: '8px',
                                  fontWeight: 700,
                                  fontFamily: 'var(--font-mono)',
                                  flexShrink: 0,
                                }}
                              >
                                {swimmer.initials}
                              </div>
                              <span style={{ fontSize: '12.5px', color: '#1A1714', fontWeight: 500 }}>
                                {swimmer.name.split(' ')[0]}
                              </span>
                              <span
                                style={{
                                  fontSize: '11.5px',
                                  color: cfg.color,
                                  fontWeight: 600,
                                }}
                              >
                                {cfg.label}
                              </span>
                            </div>
                          )
                        })}
                      </div>
                    </div>

                    {/* CTA */}
                    <Link
                      href={`/portal/meets/${meet.id}`}
                      style={{
                        padding: '9px 18px',
                        background: hasUrgent ? '#dc2626' : '#eb5d1e',
                        color: '#fff',
                        borderRadius: '9px',
                        fontSize: '13.5px',
                        fontWeight: 600,
                        textDecoration: 'none',
                        flexShrink: 0,
                        transition: 'opacity 120ms',
                      }}
                    >
                      {meet.swimmers.some((s) => s.commitment === 'pending')
                        ? 'Respond'
                        : 'View Meet'}
                    </Link>
                  </div>
                </div>
              )
            })}
          </div>
        </section>
      )}

      {/* Upcoming confirmed */}
      {upcomingMeets.length > 0 && (
        <section style={{ marginBottom: '32px' }}>
          <div
            style={{
              fontSize: '11px',
              fontFamily: 'var(--font-mono)',
              color: '#9A9693',
              textTransform: 'uppercase',
              letterSpacing: '0.08em',
              marginBottom: '14px',
            }}
          >
            Entries Confirmed
          </div>
          <MeetList meets={upcomingMeets} />
        </section>
      )}

      {/* Announced meets (published, commitment not yet open) */}
      {meetsWithDeadlines.filter((m) => m.status === 'published').length > 0 && (
        <section style={{ marginBottom: '32px' }}>
          <div
            style={{
              fontSize: '11px',
              fontFamily: 'var(--font-mono)',
              color: '#9A9693',
              textTransform: 'uppercase',
              letterSpacing: '0.08em',
              marginBottom: '14px',
            }}
          >
            Announced
          </div>
          <MeetList meets={meetsWithDeadlines.filter((m) => m.status === 'published')} />
        </section>
      )}

      {/* Past meets */}
      {pastMeets.length > 0 && (
        <section>
          <div
            style={{
              fontSize: '11px',
              fontFamily: 'var(--font-mono)',
              color: '#9A9693',
              textTransform: 'uppercase',
              letterSpacing: '0.08em',
              marginBottom: '14px',
            }}
          >
            Past Meets
          </div>
          <div
            style={{
              background: '#ffffff',
              borderRadius: '12px',
              border: '1px solid rgba(0,0,0,0.07)',
              overflow: 'hidden',
            }}
          >
            {pastMeets.map((meet, i) => (
              <Link
                key={meet.id}
                href={`/portal/meets/${meet.id}`}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '14px 18px',
                  borderBottom: i < pastMeets.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                  textDecoration: 'none',
                  transition: 'background 100ms',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.02)')}
                onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
              >
                <div>
                  <span style={{ fontSize: '14px', color: '#6B6866', fontWeight: 500 }}>{meet.name}</span>
                  <span style={{ fontSize: '13px', color: '#9A9693', marginLeft: '8px' }}>{meet.date}</span>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                  <span style={{ fontSize: '12px', color: '#9A9693', fontFamily: 'var(--font-mono)' }}>
                    {meet.course}
                  </span>
                  <span
                    style={{
                      padding: '2px 8px',
                      background: 'rgba(0,0,0,0.05)',
                      borderRadius: '5px',
                      fontSize: '11.5px',
                      color: '#9A9693',
                    }}
                  >
                    {MEET_STATUS_LABEL[meet.status]}
                  </span>
                  <Icon name="chevron-right" size={14} style={{ color: '#C0BDBB' }} />
                </div>
              </Link>
            ))}
          </div>
        </section>
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

// ── Simple meet list for non-urgent meets ────────────────────────────────────
function MeetList({ meets }: { meets: Array<MeetCard & { daysUntilDeadline: number }> }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
      {meets.map((meet) => (
        <Link
          key={meet.id}
          href={`/portal/meets/${meet.id}`}
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            background: '#ffffff',
            borderRadius: '12px',
            border: '1px solid rgba(0,0,0,0.07)',
            padding: '16px 20px',
            textDecoration: 'none',
            gap: '16px',
            transition: 'box-shadow 120ms ease',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.boxShadow = '0 4px 16px rgba(0,0,0,0.08)')}
          onMouseLeave={(e) => (e.currentTarget.style.boxShadow = 'none')}
        >
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
              <span style={{ fontSize: '15px', color: '#1A1714', fontWeight: 500 }}>{meet.name}</span>
              <span
                style={{
                  padding: '1px 7px',
                  background: 'rgba(0,0,0,0.05)',
                  borderRadius: '4px',
                  fontSize: '11px',
                  fontFamily: 'var(--font-mono)',
                  color: '#6B6866',
                }}
              >
                {meet.course}
              </span>
            </div>
            <div style={{ fontSize: '12.5px', color: '#9A9693', marginTop: '3px' }}>
              {meet.date} · {meet.location}
            </div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexShrink: 0 }}>
            <DeadlineCountdown days={meet.daysUntilDeadline} />
            <Icon name="chevron-right" size={15} style={{ color: '#C0BDBB' }} />
          </div>
        </Link>
      ))}
    </div>
  )
}
