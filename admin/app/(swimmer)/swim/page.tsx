'use client'

import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'

// ── Design tokens ──────────────────────────────────────────────────────────
const C = {
  accent:   '#00D4AA',
  accentDim: 'rgba(0,212,170,0.12)',
  accentBorder: 'rgba(0,212,170,0.25)',
  text:     '#E4F2FF',
  subtle:   '#7FA8C9',
  muted:    '#4A6B82',
  card:     '#0D2235',
  cardHov:  '#0F2A40',
  border:   'rgba(0,212,170,0.10)',
  warn:     '#F59E0B',
  warnBg:   'rgba(245,158,11,0.10)',
  warnBorder: 'rgba(245,158,11,0.25)',
}

// ── Stub data ──────────────────────────────────────────────────────────────
const SWIMMER = {
  name: 'Emma Chen',
  age: 14,
  group: 'Senior A',
  ussNumber: 'CA2019-0134',
  registrationStatus: 'active',
  initials: 'EC',
}

const UPCOMING_MEETS = [
  {
    id: 'meet-001',
    name: 'Tri-City Classic',
    date: 'Mar 22–23, 2026',
    course: 'SCY',
    location: 'Riverside Aquatic Center',
    commitmentStatus: 'committed' as const,
    events: ['200 Free', '100 Back', '400 IM'],
    path: '/swim/meets/meet-001',
  },
  {
    id: 'meet-002',
    name: 'Regional Spring Invite',
    date: 'Apr 5, 2026',
    course: 'LCM',
    location: 'University Pool',
    commitmentStatus: 'open' as const,
    events: [],
    path: '/swim/meets/meet-002',
  },
]

const RECENT_ANNOUNCEMENTS = [
  {
    id: 'ann-001',
    title: 'Tri-City Classic — Updated Warm-up Schedule',
    publishedAt: 'Mar 14, 2026',
    category: 'meet',
    unread: true,
  },
  {
    id: 'ann-002',
    title: 'Pool Closure — Spring Break (Mar 28 – Apr 5)',
    publishedAt: 'Mar 12, 2026',
    category: 'facility',
    unread: true,
  },
  {
    id: 'ann-003',
    title: 'Altitude Camp Opportunity — Senior Group',
    publishedAt: 'Mar 10, 2026',
    category: 'training',
    unread: false,
  },
]

const BEST_TIMES_PREVIEW = [
  { event: '200 Free', course: 'SCY', time: '1:52.34', standard: 'A', improvementDays: 45 },
  { event: '100 Back', course: 'SCY', time: '59.87',   standard: 'AA', improvementDays: 22 },
  { event: '400 IM',   course: 'SCY', time: '4:42.11', standard: 'B',  improvementDays: 78 },
]

const COMMIT_STATUS = {
  committed: { label: 'Committed',       color: '#00D4AA', bg: 'rgba(0,212,170,0.10)', border: 'rgba(0,212,170,0.25)' },
  pending:   { label: 'Response Needed', color: '#F59E0B', bg: 'rgba(245,158,11,0.10)', border: 'rgba(245,158,11,0.25)' },
  open:      { label: 'Not Yet Open',    color: '#4A6B82', bg: 'rgba(74,107,130,0.10)', border: 'rgba(74,107,130,0.20)' },
}

const STANDARD_COLOR: Record<string, string> = {
  AAA: '#00D4AA',
  AA:  '#3b82f6',
  A:   '#8b5cf6',
  BB:  '#F59E0B',
  B:   '#9A9693',
}

// ── Components ─────────────────────────────────────────────────────────────
function StatCard({ label, value, sub, accent }: { label: string; value: string; sub?: string; accent?: boolean }) {
  return (
    <div
      style={{
        background: C.card,
        border: `1px solid ${accent ? C.accentBorder : C.border}`,
        borderRadius: '12px',
        padding: '18px 20px',
        boxShadow: accent ? `0 0 20px rgba(0,212,170,0.08)` : 'none',
      }}
    >
      <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.muted, letterSpacing: '0.05em', textTransform: 'uppercase', marginBottom: '6px' }}>
        {label}
      </p>
      <p style={{ fontFamily: 'var(--font-display)', fontSize: '28px', color: accent ? C.accent : C.text, letterSpacing: '-0.02em', lineHeight: 1 }}>
        {value}
      </p>
      {sub && (
        <p style={{ fontSize: '12px', color: C.muted, marginTop: '4px' }}>{sub}</p>
      )}
    </div>
  )
}

function MeetCard({ meet }: { meet: typeof UPCOMING_MEETS[number] }) {
  const status = COMMIT_STATUS[meet.commitmentStatus]
  return (
    <div
      style={{
        background: C.card,
        border: `1px solid ${C.border}`,
        borderRadius: '12px',
        padding: '18px 20px',
        display: 'flex',
        flexDirection: 'column',
        gap: '14px',
        transition: 'border-color 150ms ease, box-shadow 150ms ease',
      }}
      onMouseEnter={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.borderColor = C.accentBorder
        el.style.boxShadow = '0 0 24px rgba(0,212,170,0.06)'
      }}
      onMouseLeave={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.borderColor = C.border
        el.style.boxShadow = 'none'
      }}
    >
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '12px' }}>
        <div>
          <p style={{ fontSize: '15px', fontWeight: 600, color: C.text, marginBottom: '3px' }}>
            {meet.name}
          </p>
          <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
            <span style={{ fontSize: '12px', color: C.subtle, display: 'flex', alignItems: 'center', gap: '4px' }}>
              <Icon name="calendar" size={11} />
              {meet.date}
            </span>
            <span style={{ fontSize: '12px', color: C.subtle, display: 'flex', alignItems: 'center', gap: '4px' }}>
              <Icon name="map-pin" size={11} />
              {meet.location}
            </span>
          </div>
        </div>
        <div style={{ display: 'flex', gap: '7px', alignItems: 'center' }}>
          <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', fontWeight: 600, color: '#0D2235', background: C.accent, padding: '2px 7px', borderRadius: '4px', letterSpacing: '0.04em' }}>
            {meet.course}
          </span>
          <span
            style={{
              fontSize: '11px',
              fontWeight: 500,
              color: status.color,
              background: status.bg,
              border: `1px solid ${status.border}`,
              padding: '3px 9px',
              borderRadius: '999px',
              whiteSpace: 'nowrap',
            }}
          >
            {status.label}
          </span>
        </div>
      </div>

      {/* Events */}
      {meet.events.length > 0 && (
        <div>
          <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '6px' }}>
            My Events
          </p>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
            {meet.events.map((ev) => (
              <span
                key={ev}
                style={{
                  fontSize: '12px',
                  color: C.accent,
                  background: C.accentDim,
                  border: `1px solid ${C.accentBorder}`,
                  padding: '3px 10px',
                  borderRadius: '6px',
                  fontFamily: 'var(--font-mono)',
                }}
              >
                {ev}
              </span>
            ))}
          </div>
        </div>
      )}

      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <Link
          href={meet.path}
          style={{
            fontSize: '12px',
            fontWeight: 600,
            color: C.accent,
            textDecoration: 'none',
            display: 'inline-flex',
            alignItems: 'center',
            gap: '5px',
          }}
        >
          Meet details
          <Icon name="chevron-right" size={13} />
        </Link>
      </div>
    </div>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function SwimmerDashboardPage() {
  const { session } = useSession()
  const firstName = session?.user.name?.split(' ')[0] ?? SWIMMER.name.split(' ')[0]

  const now = new Date()
  const hour = now.getHours()
  const greeting = hour < 12 ? 'Good morning' : hour < 17 ? 'Good afternoon' : 'Good evening'

  return (
    <div>
      {/* Hero greeting */}
      <div
        style={{
          marginBottom: '40px',
          animation: 'slideUp 0.45s cubic-bezier(0.16,1,0.3,1) both',
        }}
      >
        {/* Swimmer avatar + name */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '18px', marginBottom: '8px' }}>
          <div
            style={{
              width: '56px',
              height: '56px',
              borderRadius: '50%',
              background: 'linear-gradient(135deg, #00D4AA 0%, #0099CC 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#061623',
              fontSize: '18px',
              fontWeight: 700,
              fontFamily: 'var(--font-mono)',
              boxShadow: '0 0 24px rgba(0,212,170,0.30)',
              flexShrink: 0,
            }}
          >
            {SWIMMER.initials}
          </div>
          <div>
            <p
              style={{
                fontSize: '12px',
                fontFamily: 'var(--font-mono)',
                color: C.muted,
                letterSpacing: '0.05em',
                marginBottom: '4px',
              }}
            >
              {greeting}
            </p>
            <h1
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: 'clamp(26px, 4vw, 36px)',
                color: C.text,
                letterSpacing: '-0.02em',
                lineHeight: 1.1,
              }}
            >
              {firstName}.
            </h1>
          </div>
        </div>
        <p style={{ fontSize: '14px', color: C.subtle, marginTop: '8px', marginLeft: '74px' }}>
          {SWIMMER.group} · USS# {SWIMMER.ussNumber}
        </p>
      </div>

      {/* Stats strip */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(170px, 1fr))',
          gap: '12px',
          marginBottom: '40px',
          animation: 'slideUp 0.45s cubic-bezier(0.16,1,0.3,1) 0.06s both',
        }}
      >
        <StatCard label="Season Events" value="3" sub="Registered events" accent />
        <StatCard label="Next Meet" value="Mar 22" sub="Tri-City Classic" />
        <StatCard label="Best Time (100 Back)" value="59.87" sub="SCY — AA standard" />
        <StatCard label="Registration" value="Active" sub="2025–2026 season" />
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'minmax(0,1fr) 340px',
          gap: '24px',
          alignItems: 'start',
        }}
      >
        {/* Left column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '32px' }}>
          {/* Upcoming meets */}
          <section>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
              <h2
                style={{
                  fontFamily: 'var(--font-display)',
                  fontSize: '20px',
                  color: C.text,
                  letterSpacing: '-0.01em',
                }}
              >
                Upcoming Meets
              </h2>
              <Link
                href="/swim/schedule"
                style={{
                  fontSize: '13px',
                  fontWeight: 500,
                  color: C.accent,
                  textDecoration: 'none',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  transition: 'opacity 120ms ease',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.opacity = '0.7')}
                onMouseLeave={(e) => (e.currentTarget.style.opacity = '1')}
              >
                Full schedule
                <Icon name="chevron-right" size={14} />
              </Link>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {UPCOMING_MEETS.map((meet, i) => (
                <div
                  key={meet.id}
                  style={{ animationDelay: `${0.1 + i * 0.07}s`, animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) both' }}
                >
                  <MeetCard meet={meet} />
                </div>
              ))}
            </div>
          </section>

          {/* Best times preview */}
          <section>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
              <h2
                style={{
                  fontFamily: 'var(--font-display)',
                  fontSize: '20px',
                  color: C.text,
                  letterSpacing: '-0.01em',
                }}
              >
                My Best Times
              </h2>
              <Link
                href="/swim/performance"
                style={{
                  fontSize: '13px',
                  fontWeight: 500,
                  color: C.accent,
                  textDecoration: 'none',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  transition: 'opacity 120ms ease',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.opacity = '0.7')}
                onMouseLeave={(e) => (e.currentTarget.style.opacity = '1')}
              >
                Full performance
                <Icon name="chevron-right" size={14} />
              </Link>
            </div>
            <div
              style={{
                background: C.card,
                border: `1px solid ${C.border}`,
                borderRadius: '12px',
                overflow: 'hidden',
                animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.18s both',
              }}
            >
              {/* Table header */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 80px 100px 80px',
                  padding: '10px 18px',
                  borderBottom: `1px solid ${C.border}`,
                }}
              >
                {['Event', 'Course', 'Best Time', 'Std'].map((h) => (
                  <span
                    key={h}
                    style={{
                      fontSize: '10.5px',
                      fontFamily: 'var(--font-mono)',
                      color: C.muted,
                      textTransform: 'uppercase',
                      letterSpacing: '0.05em',
                    }}
                  >
                    {h}
                  </span>
                ))}
              </div>
              {BEST_TIMES_PREVIEW.map((bt, i) => (
                <div
                  key={bt.event}
                  style={{
                    display: 'grid',
                    gridTemplateColumns: '1fr 80px 100px 80px',
                    padding: '12px 18px',
                    borderBottom: i < BEST_TIMES_PREVIEW.length - 1 ? `1px solid ${C.border}` : 'none',
                    alignItems: 'center',
                    transition: 'background 100ms ease',
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,212,170,0.04)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <span style={{ fontSize: '13.5px', fontWeight: 600, color: C.text }}>{bt.event}</span>
                  <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: C.subtle }}>{bt.course}</span>
                  <span style={{ fontSize: '14px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: C.accent }}>{bt.time}</span>
                  <span
                    style={{
                      fontSize: '11px',
                      fontFamily: 'var(--font-mono)',
                      fontWeight: 700,
                      color: STANDARD_COLOR[bt.standard] ?? C.subtle,
                      background: `${STANDARD_COLOR[bt.standard] ?? C.subtle}15`,
                      padding: '2px 8px',
                      borderRadius: '5px',
                      display: 'inline-block',
                    }}
                  >
                    {bt.standard}
                  </span>
                </div>
              ))}
            </div>
          </section>
        </div>

        {/* Right sidebar — announcements */}
        <div
          style={{
            background: C.card,
            border: `1px solid ${C.border}`,
            borderRadius: '14px',
            overflow: 'hidden',
            animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.12s both',
          }}
        >
          <div
            style={{
              padding: '16px 18px 14px',
              borderBottom: `1px solid ${C.border}`,
            }}
          >
            <p
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: '17px',
                color: C.text,
                marginBottom: '2px',
              }}
            >
              Announcements
            </p>
            <p style={{ fontSize: '12px', color: C.muted }}>
              Latest from the club
            </p>
          </div>

          {RECENT_ANNOUNCEMENTS.map((ann, i) => (
            <div
              key={ann.id}
              style={{
                padding: '14px 18px',
                borderBottom: i < RECENT_ANNOUNCEMENTS.length - 1 ? `1px solid ${C.border}` : 'none',
                display: 'flex',
                gap: '10px',
                alignItems: 'flex-start',
                transition: 'background 100ms ease',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,212,170,0.03)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              {/* Unread dot */}
              <div
                style={{
                  width: '7px',
                  height: '7px',
                  borderRadius: '50%',
                  background: ann.unread ? C.accent : 'transparent',
                  flexShrink: 0,
                  marginTop: '5px',
                }}
              />
              <div>
                <p style={{ fontSize: '13px', fontWeight: 500, color: ann.unread ? C.text : C.subtle, lineHeight: 1.4, marginBottom: '3px' }}>
                  {ann.title}
                </p>
                <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.muted }}>
                  {ann.publishedAt}
                </p>
              </div>
            </div>
          ))}

          <div
            style={{
              padding: '10px 18px',
              borderTop: `1px solid ${C.border}`,
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
            }}
          >
            <Icon name="megaphone" size={12} style={{ color: C.muted }} />
            <span style={{ fontSize: '11.5px', color: C.muted }}>
              Full inbox available via your parent account
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
