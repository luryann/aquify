'use client'

import Link from 'next/link'
import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

// ── Stub data ──────────────────────────────────────────────────────────────
const HOUSEHOLD = {
  id: 'hh-001',
  name: 'Chen Family',
  address: {
    street: '4821 Lakeview Drive',
    city: 'Sacramento',
    state: 'CA',
    zip: '95814',
  },
  phone: '(916) 555-0187',
  email: 'chen.family@gmail.com',
  memberSince: 'September 2021',
  status: 'active' as const,
}

const CONTACTS = [
  {
    id: 'c-001',
    name: 'Sarah Chen',
    relationship: 'Primary Parent',
    phone: '(916) 555-0187',
    email: 'sarah.chen@gmail.com',
    isPrimary: true,
  },
  {
    id: 'c-002',
    name: 'David Chen',
    relationship: 'Secondary Parent',
    phone: '(916) 555-0244',
    email: 'david.chen@gmail.com',
    isPrimary: false,
  },
  {
    id: 'c-003',
    name: 'Margaret Chen',
    relationship: 'Emergency Contact',
    phone: '(916) 555-0311',
    email: '',
    isPrimary: false,
  },
]

const SWIMMERS = [
  {
    id: 'sw-001',
    name: 'Emma Chen',
    dob: 'Feb 12, 2012',
    age: 14,
    gender: 'Female',
    ussNumber: 'ABC123456X',
    group: 'Senior A',
    status: 'active' as const,
    gradient: 'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
    initials: 'EC',
    registrationStatus: 'registered' as const,
    season: '2025–2026',
  },
  {
    id: 'sw-002',
    name: 'Noah Chen',
    dob: 'Jul 3, 2015',
    age: 11,
    gender: 'Male',
    ussNumber: 'DEF789012Y',
    group: 'Junior B',
    status: 'active' as const,
    gradient: 'linear-gradient(135deg, #10b981 0%, #059669 100%)',
    initials: 'NC',
    registrationStatus: 'registered' as const,
    season: '2025–2026',
  },
]

const REG_STATUS_CONFIG = {
  registered:  { label: 'Registered', color: '#16a34a', bg: '#f0fdf4', border: '#bbf7d0' },
  pending:     { label: 'Pending',    color: '#d97706', bg: '#fffbeb', border: '#fde68a' },
  unregistered:{ label: 'Not Enrolled', color: '#6b7280', bg: '#f9fafb', border: '#e5e7eb' },
}

type Tab = 'overview' | 'contacts' | 'swimmers'

// ── Components ─────────────────────────────────────────────────────────────
function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: '140px 1fr',
        gap: '12px',
        padding: '12px 0',
        borderBottom: '1px solid rgba(0,0,0,0.05)',
        alignItems: 'baseline',
      }}
    >
      <dt
        style={{
          fontSize: '11.5px',
          fontFamily: 'var(--font-mono)',
          color: '#A09D9B',
          letterSpacing: '0.03em',
          textTransform: 'uppercase',
          fontWeight: 500,
        }}
      >
        {label}
      </dt>
      <dd style={{ fontSize: '14px', color: '#1A1714', fontWeight: 450 }}>
        {value}
      </dd>
    </div>
  )
}

function ContactCard({ contact }: { contact: typeof CONTACTS[number] }) {
  return (
    <div
      style={{
        background: '#ffffff',
        border: contact.isPrimary ? '1.5px solid rgba(235,93,30,0.25)' : '1px solid rgba(0,0,0,0.08)',
        borderRadius: '12px',
        padding: '20px',
        display: 'flex',
        gap: '16px',
        alignItems: 'flex-start',
        boxShadow: contact.isPrimary
          ? '0 2px 12px rgba(235,93,30,0.08)'
          : '0 1px 6px rgba(0,0,0,0.04)',
      }}
    >
      {/* Avatar */}
      <div
        style={{
          width: '44px',
          height: '44px',
          borderRadius: '50%',
          background: contact.isPrimary
            ? 'linear-gradient(135deg, #eb5d1e 0%, #f4823a 100%)'
            : 'linear-gradient(135deg, #9ca3af 0%, #6b7280 100%)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#ffffff',
          fontSize: '14px',
          fontWeight: 700,
          fontFamily: 'var(--font-mono)',
          flexShrink: 0,
          boxShadow: '0 2px 8px rgba(0,0,0,0.12)',
        }}
      >
        {contact.name.split(' ').map((w) => w[0]).join('').slice(0, 2).toUpperCase()}
      </div>

      {/* Info */}
      <div style={{ flex: 1 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '2px' }}>
          <p style={{ fontSize: '15px', fontWeight: 600, color: '#1A1714' }}>
            {contact.name}
          </p>
          {contact.isPrimary && (
            <span
              style={{
                fontSize: '10px',
                fontFamily: 'var(--font-mono)',
                fontWeight: 600,
                color: '#eb5d1e',
                background: 'rgba(235,93,30,0.10)',
                padding: '2px 7px',
                borderRadius: '999px',
                letterSpacing: '0.04em',
                textTransform: 'uppercase',
              }}
            >
              Primary
            </span>
          )}
        </div>
        <p style={{ fontSize: '12px', color: '#9A9693', marginBottom: '12px' }}>
          {contact.relationship}
        </p>

        <div style={{ display: 'flex', flexDirection: 'column', gap: '5px' }}>
          {contact.phone && (
            <a
              href={`tel:${contact.phone}`}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '7px',
                fontSize: '13px',
                color: '#374151',
                textDecoration: 'none',
              }}
            >
              <Icon name="users" size={13} style={{ color: '#9ca3af' }} />
              {contact.phone}
            </a>
          )}
          {contact.email && (
            <a
              href={`mailto:${contact.email}`}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '7px',
                fontSize: '13px',
                color: '#374151',
                textDecoration: 'none',
              }}
            >
              <Icon name="mail" size={13} style={{ color: '#9ca3af' }} />
              {contact.email}
            </a>
          )}
        </div>
      </div>
    </div>
  )
}

function SwimmerProfileCard({ swimmer }: { swimmer: typeof SWIMMERS[number] }) {
  const regCfg = REG_STATUS_CONFIG[swimmer.registrationStatus]

  return (
    <div
      style={{
        background: '#ffffff',
        border: '1px solid rgba(0,0,0,0.08)',
        borderRadius: '14px',
        overflow: 'hidden',
        boxShadow: '0 2px 8px rgba(0,0,0,0.06)',
      }}
    >
      {/* Card header */}
      <div
        style={{
          padding: '20px 22px',
          display: 'flex',
          gap: '16px',
          alignItems: 'center',
          borderBottom: '1px solid rgba(0,0,0,0.06)',
        }}
      >
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
            fontSize: '17px',
            fontWeight: 700,
            fontFamily: 'var(--font-mono)',
            flexShrink: 0,
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
          }}
        >
          {swimmer.initials}
        </div>
        <div style={{ flex: 1 }}>
          <p style={{ fontSize: '16px', fontWeight: 700, color: '#1A1714', marginBottom: '3px' }}>
            {swimmer.name}
          </p>
          <p style={{ fontSize: '12.5px', color: '#6B6866' }}>
            Age {swimmer.age} · {swimmer.gender} · {swimmer.group}
          </p>
        </div>
        <span
          style={{
            fontSize: '11px',
            fontWeight: 600,
            color: regCfg.color,
            background: regCfg.bg,
            border: `1px solid ${regCfg.border}`,
            padding: '4px 10px',
            borderRadius: '999px',
            fontFamily: 'var(--font-mono)',
            letterSpacing: '0.03em',
          }}
        >
          {regCfg.label}
        </span>
      </div>

      {/* Card body */}
      <dl style={{ padding: '4px 22px 12px' }}>
        <InfoRow label="Date of Birth"  value={swimmer.dob} />
        <InfoRow label="USS Number"     value={swimmer.ussNumber} />
        <InfoRow label="Season"         value={swimmer.season} />
      </dl>

      {/* Footer */}
      <div
        style={{
          padding: '12px 22px',
          borderTop: '1px solid rgba(0,0,0,0.05)',
          background: '#fafaf9',
          display: 'flex',
          gap: '8px',
        }}
      >
        <Link
          href={`/portal/swimmers/${swimmer.id}`}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '6px',
            fontSize: '12.5px',
            fontWeight: 600,
            color: '#1A1714',
            background: 'rgba(0,0,0,0.06)',
            padding: '6px 12px',
            borderRadius: '7px',
            textDecoration: 'none',
            transition: 'background 120ms ease',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.10)')}
          onMouseLeave={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.06)')}
        >
          <Icon name="user" size={13} />
          View Profile
        </Link>
        <Link
          href="/portal/meets"
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '6px',
            fontSize: '12.5px',
            fontWeight: 600,
            color: '#6B6866',
            padding: '6px 12px',
            borderRadius: '7px',
            textDecoration: 'none',
            transition: 'background 120ms ease, color 120ms ease',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.background = 'rgba(0,0,0,0.04)'
            e.currentTarget.style.color = '#1A1714'
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.background = 'transparent'
            e.currentTarget.style.color = '#6B6866'
          }}
        >
          <Icon name="calendar" size={13} />
          Meets
        </Link>
      </div>
    </div>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function HouseholdProfilePage() {
  const [activeTab, setActiveTab] = useState<Tab>('overview')

  const tabs: { id: Tab; label: string; count?: number }[] = [
    { id: 'overview',  label: 'Overview' },
    { id: 'contacts',  label: 'Contacts',  count: CONTACTS.length },
    { id: 'swimmers',  label: 'Swimmers',  count: SWIMMERS.length },
  ]

  return (
    <div style={{ animation: 'slideUp 0.35s var(--ease-out) both' }}>
      {/* Page header */}
      <div
        style={{
          display: 'flex',
          alignItems: 'flex-start',
          gap: '20px',
          marginBottom: '32px',
        }}
      >
        {/* Household avatar */}
        <div
          style={{
            width: '64px',
            height: '64px',
            borderRadius: '18px',
            background: 'linear-gradient(135deg, #eb5d1e 0%, #f59e0b 100%)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#ffffff',
            fontSize: '22px',
            fontWeight: 700,
            fontFamily: 'var(--font-mono)',
            flexShrink: 0,
            boxShadow: '0 4px 16px rgba(235,93,30,0.30)',
          }}
        >
          {HOUSEHOLD.name.split(' ')[0]?.[0] ?? 'H'}
        </div>

        <div style={{ flex: 1 }}>
          <h1
            style={{
              fontFamily: 'var(--font-display)',
              fontSize: '28px',
              color: '#1A1714',
              letterSpacing: '-0.02em',
              lineHeight: 1.1,
              marginBottom: '6px',
            }}
          >
            {HOUSEHOLD.name}
          </h1>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
            <span
              style={{
                fontSize: '11px',
                fontFamily: 'var(--font-mono)',
                fontWeight: 600,
                color: '#16a34a',
                background: '#f0fdf4',
                border: '1px solid #bbf7d0',
                padding: '3px 9px',
                borderRadius: '999px',
                letterSpacing: '0.03em',
                textTransform: 'uppercase',
              }}
            >
              Active
            </span>
            <span style={{ fontSize: '12.5px', color: '#9A9693' }}>
              Member since {HOUSEHOLD.memberSince}
            </span>
          </div>
        </div>

        <button
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
            padding: '8px 16px',
            background: '#ffffff',
            border: '1px solid rgba(0,0,0,0.12)',
            borderRadius: '9px',
            fontSize: '13px',
            fontWeight: 600,
            color: '#1A1714',
            cursor: 'pointer',
            flexShrink: 0,
            boxShadow: '0 1px 4px rgba(0,0,0,0.06)',
            transition: 'box-shadow 120ms ease, border-color 120ms ease',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.boxShadow = '0 3px 10px rgba(0,0,0,0.10)'
            e.currentTarget.style.borderColor = 'rgba(0,0,0,0.20)'
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.boxShadow = '0 1px 4px rgba(0,0,0,0.06)'
            e.currentTarget.style.borderColor = 'rgba(0,0,0,0.12)'
          }}
        >
          <Icon name="edit-2" size={14} />
          Edit Profile
        </button>
      </div>

      {/* Tabs */}
      <div
        style={{
          display: 'flex',
          gap: '2px',
          borderBottom: '1px solid rgba(0,0,0,0.08)',
          marginBottom: '28px',
        }}
      >
        {tabs.map((tab) => {
          const active = activeTab === tab.id
          return (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              style={{
                padding: '10px 16px',
                background: 'transparent',
                border: 'none',
                borderBottom: active ? '2px solid #eb5d1e' : '2px solid transparent',
                cursor: 'pointer',
                fontSize: '13.5px',
                fontWeight: active ? 600 : 450,
                color: active ? '#1A1714' : '#6B6866',
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                transition: 'color 120ms ease',
                marginBottom: '-1px',
              }}
            >
              {tab.label}
              {tab.count !== undefined && (
                <span
                  style={{
                    fontSize: '11px',
                    fontFamily: 'var(--font-mono)',
                    color: active ? '#eb5d1e' : '#A09D9B',
                    background: active ? 'rgba(235,93,30,0.08)' : 'rgba(0,0,0,0.06)',
                    padding: '1px 7px',
                    borderRadius: '999px',
                    fontWeight: 600,
                  }}
                >
                  {tab.count}
                </span>
              )}
            </button>
          )
        })}
      </div>

      {/* Tab content */}
      {activeTab === 'overview' && (
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: '1fr 1fr',
            gap: '20px',
            animation: 'slideUp 0.25s var(--ease-out) both',
          }}
        >
          {/* Contact info card */}
          <div
            style={{
              background: '#ffffff',
              borderRadius: '14px',
              border: '1px solid rgba(0,0,0,0.08)',
              padding: '24px',
              boxShadow: '0 2px 8px rgba(0,0,0,0.05)',
            }}
          >
            <h3
              style={{
                fontSize: '13px',
                fontFamily: 'var(--font-mono)',
                fontWeight: 600,
                color: '#A09D9B',
                letterSpacing: '0.08em',
                textTransform: 'uppercase',
                marginBottom: '16px',
              }}
            >
              Contact Information
            </h3>
            <dl>
              <InfoRow label="Email"   value={HOUSEHOLD.email} />
              <InfoRow label="Phone"   value={HOUSEHOLD.phone} />
            </dl>
          </div>

          {/* Address card */}
          <div
            style={{
              background: '#ffffff',
              borderRadius: '14px',
              border: '1px solid rgba(0,0,0,0.08)',
              padding: '24px',
              boxShadow: '0 2px 8px rgba(0,0,0,0.05)',
            }}
          >
            <h3
              style={{
                fontSize: '13px',
                fontFamily: 'var(--font-mono)',
                fontWeight: 600,
                color: '#A09D9B',
                letterSpacing: '0.08em',
                textTransform: 'uppercase',
                marginBottom: '16px',
              }}
            >
              Mailing Address
            </h3>
            <dl>
              <InfoRow label="Street" value={HOUSEHOLD.address.street} />
              <InfoRow label="City"   value={`${HOUSEHOLD.address.city}, ${HOUSEHOLD.address.state} ${HOUSEHOLD.address.zip}`} />
            </dl>
          </div>

          {/* Membership card */}
          <div
            style={{
              background: '#ffffff',
              borderRadius: '14px',
              border: '1px solid rgba(0,0,0,0.08)',
              padding: '24px',
              boxShadow: '0 2px 8px rgba(0,0,0,0.05)',
              gridColumn: '1 / -1',
            }}
          >
            <h3
              style={{
                fontSize: '13px',
                fontFamily: 'var(--font-mono)',
                fontWeight: 600,
                color: '#A09D9B',
                letterSpacing: '0.08em',
                textTransform: 'uppercase',
                marginBottom: '16px',
              }}
            >
              Membership
            </h3>
            <div style={{ display: 'flex', gap: '28px', flexWrap: 'wrap' }}>
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
                  Status
                </p>
                <span
                  style={{
                    fontSize: '13px',
                    fontWeight: 600,
                    color: '#16a34a',
                    background: '#f0fdf4',
                    border: '1px solid #bbf7d0',
                    padding: '4px 12px',
                    borderRadius: '999px',
                  }}
                >
                  Active
                </span>
              </div>
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
                  Member Since
                </p>
                <p style={{ fontSize: '15px', fontWeight: 600, color: '#1A1714' }}>
                  {HOUSEHOLD.memberSince}
                </p>
              </div>
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
                  Swimmers
                </p>
                <p style={{ fontSize: '15px', fontWeight: 600, color: '#1A1714' }}>
                  {SWIMMERS.length}
                </p>
              </div>
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
                  Household ID
                </p>
                <p
                  style={{
                    fontSize: '13px',
                    fontFamily: 'var(--font-mono)',
                    color: '#6B6866',
                  }}
                >
                  {HOUSEHOLD.id}
                </p>
              </div>
            </div>
          </div>
        </div>
      )}

      {activeTab === 'contacts' && (
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))',
            gap: '16px',
            animation: 'slideUp 0.25s var(--ease-out) both',
          }}
        >
          {CONTACTS.map((contact) => (
            <ContactCard key={contact.id} contact={contact} />
          ))}

          {/* Add contact CTA */}
          <button
            style={{
              background: 'transparent',
              border: '1.5px dashed rgba(0,0,0,0.15)',
              borderRadius: '12px',
              padding: '24px',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              gap: '8px',
              cursor: 'pointer',
              color: '#9A9693',
              transition: 'border-color 120ms ease, color 120ms ease, background 120ms ease',
              minHeight: '120px',
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.borderColor = '#eb5d1e'
              e.currentTarget.style.color = '#eb5d1e'
              e.currentTarget.style.background = 'rgba(235,93,30,0.03)'
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.borderColor = 'rgba(0,0,0,0.15)'
              e.currentTarget.style.color = '#9A9693'
              e.currentTarget.style.background = 'transparent'
            }}
          >
            <Icon name="plus" size={22} />
            <span style={{ fontSize: '13px', fontWeight: 500 }}>Add Contact</span>
          </button>
        </div>
      )}

      {activeTab === 'swimmers' && (
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(360px, 1fr))',
            gap: '16px',
            animation: 'slideUp 0.25s var(--ease-out) both',
          }}
        >
          {SWIMMERS.map((swimmer) => (
            <SwimmerProfileCard key={swimmer.id} swimmer={swimmer} />
          ))}
        </div>
      )}
    </div>
  )
}
