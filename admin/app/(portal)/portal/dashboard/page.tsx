'use client'

import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'

// ── Stub data ──────────────────────────────────────────────────────────────
const ALERTS = [
  {
    id: 'inv-001',
    type: 'invoice' as const,
    title: 'Invoice Due',
    body: '$420.00 outstanding',
    detail: 'Season registration fee',
    urgency: 'high' as const,
    cta: 'Pay Now',
    ctaPath: '/portal/billing',
  },
  {
    id: 'commit-001',
    type: 'commitment' as const,
    title: 'Respond to Meet',
    body: 'Tri-City Classic',
    detail: 'Deadline in 2 days — Mar 19',
    urgency: 'medium' as const,
    cta: 'Respond',
    ctaPath: '/portal/meets',
  },
  {
    id: 'doc-001',
    type: 'document' as const,
    title: 'Document Needed',
    body: 'Insurance Card',
    detail: 'Required before Mar 25',
    urgency: 'low' as const,
    cta: 'Upload',
    ctaPath: '/portal/documents',
  },
]

const SWIMMERS = [
  {
    id: 'sw-001',
    name: 'Emma Chen',
    age: 14,
    group: 'Senior A',
    status: 'active' as const,
    gradient: 'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
    initials: 'EC',
    pendingCommits: 1,
    nextMeet: 'Tri-City Classic',
  },
  {
    id: 'sw-002',
    name: 'Noah Chen',
    age: 11,
    group: 'Junior B',
    status: 'active' as const,
    gradient: 'linear-gradient(135deg, #10b981 0%, #059669 100%)',
    initials: 'NC',
    pendingCommits: 0,
    nextMeet: 'Spring Invite',
  },
]

const UPCOMING_MEETS = [
  {
    id: 'meet-001',
    name: 'Tri-City Classic',
    date: 'Mar 22, 2026',
    course: 'SCY',
    location: 'Riverside Aquatic Center',
    commitment: 'pending' as const,
    swimmers: ['Emma Chen'],
  },
  {
    id: 'meet-002',
    name: 'Regional Spring Invite',
    date: 'Apr 5, 2026',
    course: 'LCM',
    location: 'University Pool',
    commitment: 'committed' as const,
    swimmers: ['Emma Chen', 'Noah Chen'],
  },
  {
    id: 'meet-003',
    name: 'Summer Championship',
    date: 'Jun 14, 2026',
    course: 'LCM',
    location: 'Olympic Training Center',
    commitment: 'open' as const,
    swimmers: [],
  },
]

// ── Alert urgency config ────────────────────────────────────────────────────
const URGENCY_CONFIG = {
  high: {
    borderColor: '#ef4444',
    iconBg: '#fef2f2',
    iconColor: '#ef4444',
    badgeBg: '#fef2f2',
    badgeColor: '#dc2626',
    badgeText: 'Action Required',
  },
  medium: {
    borderColor: '#f59e0b',
    iconBg: '#fffbeb',
    iconColor: '#d97706',
    badgeBg: '#fffbeb',
    badgeColor: '#92400e',
    badgeText: 'Due Soon',
  },
  low: {
    borderColor: '#3b82f6',
    iconBg: '#eff6ff',
    iconColor: '#3b82f6',
    badgeBg: '#eff6ff',
    badgeColor: '#1d4ed8',
    badgeText: 'Upcoming',
  },
}

const ALERT_ICON: Record<string, string> = {
  invoice:    'file-text',
  commitment: 'calendar',
  document:   'clipboard-list',
}

// ── Commitment status config ────────────────────────────────────────────────
const COMMIT_STATUS = {
  pending:   { label: 'Response Needed', color: '#d97706', bg: '#fffbeb', border: '#fde68a' },
  committed: { label: 'Committed',       color: '#16a34a', bg: '#f0fdf4', border: '#bbf7d0' },
  open:      { label: 'Not Yet Open',    color: '#6b7280', bg: '#f9fafb', border: '#e5e7eb' },
}

// ── Components ─────────────────────────────────────────────────────────────
function AlertCard({
  alert,
}: {
  alert: typeof ALERTS[number]
}) {
  const cfg = URGENCY_CONFIG[alert.urgency]
  const iconName = ALERT_ICON[alert.type] as 'file-text' | 'calendar' | 'clipboard-list'

  return (
    <div
      style={{
        background: '#ffffff',
        borderRadius: '12px',
        border: '1px solid rgba(0,0,0,0.08)',
        borderLeft: `3px solid ${cfg.borderColor}`,
        padding: '20px',
        boxShadow: '0 2px 8px rgba(0,0,0,0.06)',
        display: 'flex',
        flexDirection: 'column',
        gap: '14px',
        animation: 'slideUp 0.35s var(--ease-out) both',
        transition: 'box-shadow 150ms ease, transform 150ms ease',
      }}
      onMouseEnter={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.boxShadow = '0 6px 20px rgba(0,0,0,0.10)'
        el.style.transform = 'translateY(-1px)'
      }}
      onMouseLeave={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.boxShadow = '0 2px 8px rgba(0,0,0,0.06)'
        el.style.transform = 'translateY(0)'
      }}
    >
      {/* Icon + Badge */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div
          style={{
            width: '38px',
            height: '38px',
            borderRadius: '10px',
            background: cfg.iconBg,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: cfg.iconColor,
          }}
        >
          <Icon name={iconName} size={18} />
        </div>
        <span
          style={{
            fontSize: '10.5px',
            fontWeight: 600,
            fontFamily: 'var(--font-mono)',
            letterSpacing: '0.04em',
            textTransform: 'uppercase',
            color: cfg.badgeColor,
            background: cfg.badgeBg,
            padding: '3px 8px',
            borderRadius: '999px',
          }}
        >
          {cfg.badgeText}
        </span>
      </div>

      {/* Content */}
      <div>
        <p
          style={{
            fontSize: '11px',
            fontFamily: 'var(--font-mono)',
            color: '#A09D9B',
            letterSpacing: '0.04em',
            textTransform: 'uppercase',
            marginBottom: '4px',
          }}
        >
          {alert.title}
        </p>
        <p
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: '20px',
            color: '#1A1714',
            lineHeight: 1.2,
            marginBottom: '4px',
          }}
        >
          {alert.body}
        </p>
        <p style={{ fontSize: '12.5px', color: '#6B6866', lineHeight: 1.5 }}>
          {alert.detail}
        </p>
      </div>

      {/* CTA */}
      <Link
        href={alert.ctaPath}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '6px',
          padding: '8px 14px',
          background: '#1A1714',
          color: '#ffffff',
          borderRadius: '8px',
          fontSize: '13px',
          fontWeight: 600,
          textDecoration: 'none',
          alignSelf: 'flex-start',
          transition: 'background 120ms ease',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.background = '#eb5d1e')}
        onMouseLeave={(e) => (e.currentTarget.style.background = '#1A1714')}
      >
        {alert.cta}
        <Icon name="chevron-right" size={13} />
      </Link>
    </div>
  )
}

function SwimmerCard({ swimmer }: { swimmer: typeof SWIMMERS[number] }) {
  return (
    <div
      style={{
        background: '#ffffff',
        borderRadius: '14px',
        border: '1px solid rgba(0,0,0,0.08)',
        padding: '22px',
        boxShadow: '0 2px 8px rgba(0,0,0,0.05)',
        display: 'flex',
        gap: '16px',
        alignItems: 'flex-start',
        transition: 'box-shadow 150ms ease, transform 150ms ease',
        animation: 'slideUp 0.4s var(--ease-out) 0.1s both',
      }}
      onMouseEnter={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.boxShadow = '0 8px 24px rgba(0,0,0,0.10)'
        el.style.transform = 'translateY(-2px)'
      }}
      onMouseLeave={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.boxShadow = '0 2px 8px rgba(0,0,0,0.05)'
        el.style.transform = 'translateY(0)'
      }}
    >
      {/* Avatar */}
      <div
        style={{
          width: '52px',
          height: '52px',
          borderRadius: '50%',
          background: swimmer.gradient,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#ffffff',
          fontSize: '16px',
          fontWeight: 700,
          fontFamily: 'var(--font-mono)',
          flexShrink: 0,
          boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
        }}
      >
        {swimmer.initials}
      </div>

      {/* Info */}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '2px' }}>
          <p
            style={{
              fontSize: '15px',
              fontWeight: 600,
              color: '#1A1714',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {swimmer.name}
          </p>
          <span
            style={{
              fontSize: '10px',
              fontFamily: 'var(--font-mono)',
              color: '#16a34a',
              background: '#f0fdf4',
              padding: '2px 7px',
              borderRadius: '999px',
              fontWeight: 500,
              flexShrink: 0,
            }}
          >
            Active
          </span>
        </div>
        <p style={{ fontSize: '12px', color: '#6B6866', marginBottom: '12px' }}>
          Age {swimmer.age} · {swimmer.group}
        </p>

        {/* Next meet / pending commit */}
        {swimmer.pendingCommits > 0 ? (
          <div
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '6px',
              background: '#fffbeb',
              border: '1px solid #fde68a',
              borderRadius: '7px',
              padding: '5px 10px',
              fontSize: '12px',
              color: '#92400e',
            }}
          >
            <Icon name="alert-circle" size={13} style={{ color: '#d97706' }} />
            {swimmer.pendingCommits} commitment pending
          </div>
        ) : (
          <div
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '6px',
              background: '#f9fafb',
              border: '1px solid #e5e7eb',
              borderRadius: '7px',
              padding: '5px 10px',
              fontSize: '12px',
              color: '#374151',
            }}
          >
            <Icon name="calendar" size={13} style={{ color: '#6b7280' }} />
            {swimmer.nextMeet}
          </div>
        )}
      </div>

      {/* Arrow */}
      <Link
        href={`/portal/swimmers/${swimmer.id}`}
        aria-label={`View ${swimmer.name}`}
        style={{
          width: '32px',
          height: '32px',
          borderRadius: '8px',
          background: 'rgba(0,0,0,0.04)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#6B6866',
          flexShrink: 0,
          transition: 'background 120ms ease, color 120ms ease',
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.background = '#eb5d1e'
          e.currentTarget.style.color = '#ffffff'
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.background = 'rgba(0,0,0,0.04)'
          e.currentTarget.style.color = '#6B6866'
        }}
      >
        <Icon name="chevron-right" size={15} />
      </Link>
    </div>
  )
}

function MeetRow({ meet }: { meet: typeof UPCOMING_MEETS[number] }) {
  const status = COMMIT_STATUS[meet.commitment]
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: '90px 1fr auto auto',
        alignItems: 'center',
        gap: '16px',
        padding: '16px 20px',
        borderBottom: '1px solid rgba(0,0,0,0.06)',
        transition: 'background 120ms ease',
      }}
      onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.02)')}
      onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
    >
      {/* Date */}
      <div>
        <p
          style={{
            fontSize: '11px',
            fontFamily: 'var(--font-mono)',
            color: '#eb5d1e',
            fontWeight: 500,
            letterSpacing: '0.02em',
          }}
        >
          {meet.date}
        </p>
        <p
          style={{
            fontSize: '10px',
            fontFamily: 'var(--font-mono)',
            color: '#A09D9B',
            marginTop: '2px',
          }}
        >
          {meet.course}
        </p>
      </div>

      {/* Name + location */}
      <div>
        <p style={{ fontSize: '14px', fontWeight: 600, color: '#1A1714', marginBottom: '2px' }}>
          {meet.name}
        </p>
        <p style={{ fontSize: '12px', color: '#9A9693' }}>{meet.location}</p>
      </div>

      {/* Commitment chip */}
      <span
        style={{
          fontSize: '11.5px',
          fontWeight: 500,
          color: status.color,
          background: status.bg,
          border: `1px solid ${status.border}`,
          padding: '4px 10px',
          borderRadius: '999px',
          whiteSpace: 'nowrap',
        }}
      >
        {status.label}
      </span>

      {/* CTA */}
      {meet.commitment === 'pending' ? (
        <Link
          href="/portal/meets"
          style={{
            padding: '6px 14px',
            background: '#eb5d1e',
            color: '#ffffff',
            borderRadius: '7px',
            fontSize: '12.5px',
            fontWeight: 600,
            textDecoration: 'none',
            whiteSpace: 'nowrap',
            transition: 'background 120ms ease',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.background = '#d44f14')}
          onMouseLeave={(e) => (e.currentTarget.style.background = '#eb5d1e')}
        >
          Respond
        </Link>
      ) : (
        <Link
          href="/portal/meets"
          style={{
            padding: '6px 14px',
            background: 'transparent',
            color: '#6B6866',
            border: '1px solid rgba(0,0,0,0.12)',
            borderRadius: '7px',
            fontSize: '12.5px',
            fontWeight: 500,
            textDecoration: 'none',
            whiteSpace: 'nowrap',
            transition: 'border-color 120ms ease, color 120ms ease',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.color = '#1A1714'
            e.currentTarget.style.borderColor = 'rgba(0,0,0,0.24)'
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.color = '#6B6866'
            e.currentTarget.style.borderColor = 'rgba(0,0,0,0.12)'
          }}
        >
          View
        </Link>
      )}
    </div>
  )
}

// ── Section header helper ────────────────────────────────────────────────────
function SectionHeader({
  title,
  action,
  actionPath,
}: {
  title: string
  action?: string
  actionPath?: string
}) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginBottom: '16px',
      }}
    >
      <h2
        style={{
          fontFamily: 'var(--font-display)',
          fontSize: '22px',
          color: '#1A1714',
          letterSpacing: '-0.01em',
          lineHeight: 1.2,
        }}
      >
        {title}
      </h2>
      {action && actionPath && (
        <Link
          href={actionPath}
          style={{
            fontSize: '13px',
            fontWeight: 500,
            color: '#eb5d1e',
            textDecoration: 'none',
            display: 'flex',
            alignItems: 'center',
            gap: '4px',
            transition: 'color 120ms ease',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.color = '#d44f14')}
          onMouseLeave={(e) => (e.currentTarget.style.color = '#eb5d1e')}
        >
          {action}
          <Icon name="chevron-right" size={14} />
        </Link>
      )}
    </div>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function ParentDashboardPage() {
  const { session } = useSession()
  const firstName = session?.user.name?.split(' ')[0] ?? 'there'

  const now = new Date()
  const dateStr = now.toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })

  const hour = now.getHours()
  const greeting = hour < 12 ? 'Good morning' : hour < 17 ? 'Good afternoon' : 'Good evening'

  return (
    <div>
      {/* Page greeting */}
      <div
        style={{
          marginBottom: '36px',
          animation: 'slideUp 0.4s var(--ease-out) both',
        }}
      >
        <p
          style={{
            fontSize: '12px',
            fontFamily: 'var(--font-mono)',
            color: '#A09D9B',
            letterSpacing: '0.05em',
            marginBottom: '6px',
          }}
        >
          {dateStr}
        </p>
        <h1
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: 'clamp(28px, 4vw, 38px)',
            color: '#1A1714',
            letterSpacing: '-0.02em',
            lineHeight: 1.1,
          }}
        >
          {greeting}, {firstName}.
        </h1>
        <p style={{ fontSize: '14px', color: '#6B6866', marginTop: '8px', lineHeight: 1.6 }}>
          Here&apos;s what needs your attention today.
        </p>
      </div>

      {/* Needs Attention */}
      <section style={{ marginBottom: '44px' }}>
        <SectionHeader title="Needs Your Attention" />
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
            gap: '16px',
          }}
        >
          {ALERTS.map((alert, i) => (
            <div
              key={alert.id}
              style={{ animationDelay: `${i * 60}ms`, animation: 'slideUp 0.35s var(--ease-out) both' }}
            >
              <AlertCard alert={alert} />
            </div>
          ))}
        </div>
      </section>

      {/* Swimmers */}
      <section style={{ marginBottom: '44px' }}>
        <SectionHeader title="Your Swimmers" action="View All" actionPath="/portal/household" />
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))',
            gap: '14px',
          }}
        >
          {SWIMMERS.map((swimmer) => (
            <SwimmerCard key={swimmer.id} swimmer={swimmer} />
          ))}
        </div>
      </section>

      {/* Upcoming meets */}
      <section>
        <SectionHeader title="Upcoming Meets" action="See All Meets" actionPath="/portal/meets" />
        <div
          style={{
            background: '#ffffff',
            borderRadius: '14px',
            border: '1px solid rgba(0,0,0,0.08)',
            overflow: 'hidden',
            boxShadow: '0 2px 8px rgba(0,0,0,0.05)',
            animation: 'slideUp 0.45s var(--ease-out) 0.15s both',
          }}
        >
          {UPCOMING_MEETS.map((meet, i) => (
            <div
              key={meet.id}
              style={{
                animation: `slideUp 0.3s var(--ease-out) ${0.2 + i * 0.07}s both`,
              }}
            >
              <MeetRow meet={meet} />
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
