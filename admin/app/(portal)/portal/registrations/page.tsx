'use client'

import Link from 'next/link'
import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

// ── Stub data ──────────────────────────────────────────────────────────────
const REGISTRATIONS = [
  {
    id: 'reg-001',
    swimmer: {
      id: 'sw-001',
      name: 'Emma Chen',
      age: 14,
      group: 'Senior A',
      gradient: 'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
      initials: 'EC',
    },
    season: '2025–2026',
    status: 'active' as const,
    enrolledDate: 'Sep 3, 2025',
    expiresDate: 'Aug 31, 2026',
    waivers: [
      { name: 'Liability Waiver',     status: 'signed' as const,   signedDate: 'Sep 3, 2025' },
      { name: 'Photo Release',        status: 'signed' as const,   signedDate: 'Sep 3, 2025' },
      { name: 'Medical Authorization',status: 'signed' as const,   signedDate: 'Sep 3, 2025' },
    ],
    program: 'Competitive Team — Senior Level',
    ussExpiry: 'Sep 1, 2026',
    ussStatus: 'current' as const,
  },
  {
    id: 'reg-002',
    swimmer: {
      id: 'sw-002',
      name: 'Noah Chen',
      age: 11,
      group: 'Junior B',
      gradient: 'linear-gradient(135deg, #10b981 0%, #059669 100%)',
      initials: 'NC',
    },
    season: '2025–2026',
    status: 'active' as const,
    enrolledDate: 'Sep 5, 2025',
    expiresDate: 'Aug 31, 2026',
    waivers: [
      { name: 'Liability Waiver',     status: 'signed' as const,   signedDate: 'Sep 5, 2025' },
      { name: 'Photo Release',        status: 'signed' as const,   signedDate: 'Sep 5, 2025' },
      { name: 'Medical Authorization',status: 'pending' as const,  signedDate: null },
    ],
    program: 'Competitive Team — Junior Level',
    ussExpiry: 'Sep 1, 2026',
    ussStatus: 'current' as const,
  },
]

const PAST_REGISTRATIONS = [
  { swimmer: 'Emma Chen', season: '2024–2025', status: 'expired' as const, enrolledDate: 'Sep 1, 2024' },
  { swimmer: 'Emma Chen', season: '2023–2024', status: 'expired' as const, enrolledDate: 'Sep 1, 2023' },
]

// ── Helpers ─────────────────────────────────────────────────────────────────
const STATUS_CONFIG = {
  active:   { label: 'Active',   bg: '#ecfdf5', color: '#059669' },
  pending:  { label: 'Pending',  bg: '#fffbeb', color: '#d97706' },
  expired:  { label: 'Expired',  bg: '#f9fafb', color: '#9A9693' },
  inactive: { label: 'Inactive', bg: '#fef2f2', color: '#dc2626' },
}

const WAIVER_CONFIG = {
  signed:  { label: 'Signed',  bg: '#ecfdf5', color: '#059669', icon: 'check-circle' as const },
  pending: { label: 'Required', bg: '#fffbeb', color: '#d97706', icon: 'alert-circle' as const },
}

// ── Component ────────────────────────────────────────────────────────────────
export default function RegistrationsPage() {
  const [expandedId, setExpandedId] = useState<string | null>('reg-001')

  const pendingWaiverCount = REGISTRATIONS.flatMap((r) =>
    r.waivers.filter((w) => w.status === 'pending')
  ).length

  return (
    <div>
      {/* Page header */}
      <div
        style={{
          marginBottom: '28px',
          animation: 'slideUp 0.3s ease both',
        }}
      >
        <h1
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: '30px',
            color: '#1A1714',
            margin: '0 0 6px',
            lineHeight: 1.1,
          }}
        >
          Registration Center
        </h1>
        <p style={{ margin: 0, fontSize: '14px', color: '#9A9693' }}>
          Enrollment status, waivers, and USS membership for your swimmers.
        </p>
      </div>

      {/* Pending waiver alert */}
      {pendingWaiverCount > 0 && (
        <div
          style={{
            background: '#fffbeb',
            border: '1px solid rgba(217,119,6,0.25)',
            borderRadius: '12px',
            padding: '14px 18px',
            marginBottom: '24px',
            display: 'flex',
            alignItems: 'center',
            gap: '12px',
            animation: 'slideUp 0.35s ease both',
          }}
        >
          <div
            style={{
              width: '34px',
              height: '34px',
              borderRadius: '50%',
              background: 'rgba(217,119,6,0.12)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0,
            }}
          >
            <Icon name="alert-circle" size={17} style={{ color: '#d97706' }} />
          </div>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: '14px', fontWeight: 600, color: '#92400e' }}>
              {pendingWaiverCount} waiver{pendingWaiverCount !== 1 ? 's' : ''} require your signature
            </div>
            <div style={{ fontSize: '12.5px', color: '#a16207', marginTop: '2px' }}>
              Unsigned waivers may affect your swimmer&apos;s ability to participate in meets.
            </div>
          </div>
          <Link
            href="/portal/documents"
            style={{
              padding: '8px 16px',
              background: '#d97706',
              color: '#fff',
              borderRadius: '8px',
              fontSize: '13px',
              fontWeight: 600,
              textDecoration: 'none',
              flexShrink: 0,
            }}
          >
            Sign Now
          </Link>
        </div>
      )}

      {/* Stats strip */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(3, 1fr)',
          gap: '12px',
          marginBottom: '28px',
          animation: 'slideUp 0.4s ease both',
        }}
      >
        {[
          { label: 'Active Registrations', value: '2', color: '#059669' },
          { label: 'Waivers Complete',      value: `${REGISTRATIONS.flatMap(r => r.waivers).filter(w => w.status === 'signed').length}/${REGISTRATIONS.flatMap(r => r.waivers).length}`, color: '#3b82f6' },
          { label: 'Current Season',        value: '2025–26', color: '#eb5d1e' },
        ].map((stat) => (
          <div
            key={stat.label}
            style={{
              background: '#ffffff',
              borderRadius: '12px',
              border: '1px solid rgba(0,0,0,0.07)',
              padding: '16px 20px',
              boxShadow: '0 1px 4px rgba(0,0,0,0.04)',
            }}
          >
            <div style={{ fontSize: '22px', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-mono)' }}>
              {stat.value}
            </div>
            <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '4px' }}>{stat.label}</div>
          </div>
        ))}
      </div>

      {/* Registration cards */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '16px', marginBottom: '32px' }}>
        {REGISTRATIONS.map((reg, idx) => {
          const isExpanded = expandedId === reg.id
          const allSigned = reg.waivers.every((w) => w.status === 'signed')

          return (
            <div
              key={reg.id}
              style={{
                background: '#ffffff',
                borderRadius: '14px',
                border: '1px solid rgba(0,0,0,0.07)',
                boxShadow: '0 2px 12px rgba(0,0,0,0.05)',
                overflow: 'hidden',
                animation: `slideUp ${0.4 + idx * 0.08}s ease both`,
              }}
            >
              {/* Card header */}
              <div
                style={{
                  padding: '20px 22px',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '16px',
                  cursor: 'pointer',
                }}
                onClick={() => setExpandedId(isExpanded ? null : reg.id)}
              >
                {/* Avatar */}
                <div
                  style={{
                    width: '48px',
                    height: '48px',
                    borderRadius: '50%',
                    background: reg.swimmer.gradient,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: '#fff',
                    fontSize: '16px',
                    fontWeight: 700,
                    fontFamily: 'var(--font-mono)',
                    flexShrink: 0,
                    boxShadow: '0 3px 10px rgba(0,0,0,0.15)',
                  }}
                >
                  {reg.swimmer.initials}
                </div>

                {/* Swimmer info */}
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <span
                      style={{
                        fontFamily: 'var(--font-display)',
                        fontSize: '18px',
                        color: '#1A1714',
                        lineHeight: 1.1,
                      }}
                    >
                      {reg.swimmer.name}
                    </span>
                    <span
                      style={{
                        padding: '2px 8px',
                        borderRadius: '999px',
                        fontSize: '11px',
                        fontWeight: 600,
                        background: STATUS_CONFIG[reg.status].bg,
                        color: STATUS_CONFIG[reg.status].color,
                        textTransform: 'uppercase',
                        letterSpacing: '0.05em',
                      }}
                    >
                      {STATUS_CONFIG[reg.status].label}
                    </span>
                  </div>
                  <div style={{ fontSize: '12.5px', color: '#9A9693', marginTop: '3px' }}>
                    {reg.program} · {reg.season}
                  </div>
                </div>

                {/* Right side */}
                <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flexShrink: 0 }}>
                  {!allSigned && (
                    <span
                      style={{
                        padding: '4px 10px',
                        background: '#fffbeb',
                        border: '1px solid rgba(217,119,6,0.2)',
                        borderRadius: '999px',
                        fontSize: '11.5px',
                        color: '#d97706',
                        fontWeight: 500,
                      }}
                    >
                      Waiver needed
                    </span>
                  )}
                  <Icon
                    name={isExpanded ? 'chevron-up' : 'chevron-down'}
                    size={16}
                    style={{ color: '#9A9693' }}
                  />
                </div>
              </div>

              {/* Expanded content */}
              {isExpanded && (
                <div
                  style={{
                    borderTop: '1px solid rgba(0,0,0,0.06)',
                    padding: '20px 22px',
                  }}
                >
                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns: '1fr 1fr',
                      gap: '20px',
                      marginBottom: '20px',
                    }}
                  >
                    {/* Enrollment details */}
                    <div>
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
                        Enrollment
                      </div>
                      {[
                        { label: 'Enrolled',    value: reg.enrolledDate },
                        { label: 'Expires',     value: reg.expiresDate },
                        { label: 'USS Expiry',  value: reg.ussExpiry },
                        { label: 'USS Status',  value: reg.ussStatus === 'current' ? '✓ Current' : '⚠ Expiring' },
                      ].map((item) => (
                        <div
                          key={item.label}
                          style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            padding: '6px 0',
                            borderBottom: '1px solid rgba(0,0,0,0.04)',
                          }}
                        >
                          <span style={{ fontSize: '12.5px', color: '#9A9693' }}>{item.label}</span>
                          <span style={{ fontSize: '12.5px', color: '#1A1714', fontWeight: 500 }}>{item.value}</span>
                        </div>
                      ))}
                    </div>

                    {/* Waivers */}
                    <div>
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
                        Waivers
                      </div>
                      {reg.waivers.map((waiver) => {
                        const cfg = WAIVER_CONFIG[waiver.status]
                        return (
                          <div
                            key={waiver.name}
                            style={{
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'space-between',
                              padding: '8px 10px',
                              marginBottom: '6px',
                              background: cfg.bg,
                              borderRadius: '8px',
                              border: `1px solid ${cfg.color}22`,
                            }}
                          >
                            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                              <Icon name={cfg.icon} size={13} style={{ color: cfg.color, flexShrink: 0 }} />
                              <span style={{ fontSize: '12.5px', color: '#1A1714' }}>{waiver.name}</span>
                            </div>
                            <div style={{ textAlign: 'right' }}>
                              {waiver.status === 'signed' ? (
                                <span style={{ fontSize: '11px', color: cfg.color, fontWeight: 500 }}>
                                  {waiver.signedDate}
                                </span>
                              ) : (
                                <Link
                                  href="/portal/documents"
                                  style={{
                                    fontSize: '12px',
                                    fontWeight: 600,
                                    color: cfg.color,
                                    textDecoration: 'none',
                                    padding: '2px 8px',
                                    background: 'rgba(217,119,6,0.1)',
                                    borderRadius: '5px',
                                  }}
                                >
                                  Sign →
                                </Link>
                              )}
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  </div>

                  {/* View profile link */}
                  <Link
                    href={`/portal/household/swimmers/${reg.swimmer.id}`}
                    style={{
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: '6px',
                      fontSize: '13px',
                      color: '#eb5d1e',
                      textDecoration: 'none',
                      fontWeight: 500,
                    }}
                  >
                    <Icon name="user" size={13} />
                    View {reg.swimmer.name}&apos;s profile
                  </Link>
                </div>
              )}
            </div>
          )
        })}
      </div>

      {/* Past registrations */}
      <div style={{ animation: 'slideUp 0.6s ease both' }}>
        <div
          style={{
            fontSize: '11px',
            fontFamily: 'var(--font-mono)',
            color: '#9A9693',
            textTransform: 'uppercase',
            letterSpacing: '0.08em',
            marginBottom: '12px',
          }}
        >
          Past Seasons
        </div>
        <div
          style={{
            background: '#ffffff',
            borderRadius: '12px',
            border: '1px solid rgba(0,0,0,0.07)',
            overflow: 'hidden',
          }}
        >
          {PAST_REGISTRATIONS.map((reg, i) => (
            <div
              key={i}
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                padding: '12px 18px',
                borderBottom: i < PAST_REGISTRATIONS.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
              }}
            >
              <div>
                <span style={{ fontSize: '13.5px', color: '#6B6866', fontWeight: 500 }}>{reg.swimmer}</span>
                <span style={{ fontSize: '13px', color: '#9A9693', marginLeft: '8px' }}>— {reg.season}</span>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                <span style={{ fontSize: '12px', color: '#9A9693' }}>{reg.enrolledDate}</span>
                <span
                  style={{
                    padding: '2px 8px',
                    borderRadius: '999px',
                    fontSize: '11px',
                    background: STATUS_CONFIG[reg.status].bg,
                    color: STATUS_CONFIG[reg.status].color,
                    fontWeight: 500,
                  }}
                >
                  {STATUS_CONFIG[reg.status].label}
                </span>
              </div>
            </div>
          ))}
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
