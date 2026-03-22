'use client'

import Link from 'next/link'
import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchHousehold,
  fetchHouseholdContacts,
  fetchSwimmers,
  fetchSwimmerGroups,
  fetchGroups,
  type HouseholdRecord,
  type ContactRecord,
  type SwimmerRecord,
  type SwimmerGroupLinkRecord,
  type GroupRecord,
} from '@/lib/adminData'

// ── Helpers ─────────────────────────────────────────────────────────────────
function fmtPhone(e164?: string): string {
  if (!e164) return '—'
  const d = e164.replace(/\D/g, '')
  if (d.length === 11 && d[0] === '1') {
    return `(${d.slice(1, 4)}) ${d.slice(4, 7)}-${d.slice(7)}`
  }
  return e164
}

function fmtMonthYear(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('en-US', { month: 'long', year: 'numeric' })
}

function fmtDOB(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function calcAge(dob: string): number {
  const d = new Date(dob)
  const now = new Date()
  let age = now.getFullYear() - d.getFullYear()
  const m = now.getMonth() - d.getMonth()
  if (m < 0 || (m === 0 && now.getDate() < d.getDate())) age--
  return age
}

function initials(first: string, last: string): string {
  return `${first[0] ?? ''}${last[0] ?? ''}`.toUpperCase()
}

const GRADIENTS = [
  'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
  'linear-gradient(135deg, #10b981 0%, #059669 100%)',
  'linear-gradient(135deg, #f59e0b 0%, #d97706 100%)',
  'linear-gradient(135deg, #ef4444 0%, #dc2626 100%)',
  'linear-gradient(135deg, #06b6d4 0%, #0891b2 100%)',
]

const REG_STATUS_CONFIG = {
  active:   { label: 'Registered', color: '#16a34a', bg: '#f0fdf4', border: '#bbf7d0' },
  pending:  { label: 'Pending',    color: '#d97706', bg: '#fffbeb', border: '#fde68a' },
  default:  { label: 'Not Enrolled', color: '#6b7280', bg: '#f9fafb', border: '#e5e7eb' },
}

type Tab = 'overview' | 'contacts' | 'swimmers'

// ── Skeleton ─────────────────────────────────────────────────────────────────
function Skeleton({ w = '100%', h = 16, r = 8 }: { w?: string | number; h?: number; r?: number }) {
  return (
    <div
      style={{
        width: w,
        height: h,
        borderRadius: r,
        background: 'linear-gradient(90deg, #f0efee 25%, #e6e5e3 50%, #f0efee 75%)',
        backgroundSize: '200% 100%',
        animation: 'shimmer 1.5s infinite',
      }}
    />
  )
}

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

function ContactCard({ contact }: { contact: ContactRecord }) {
  const name = `${contact.first_name} ${contact.last_name}`
  return (
    <div
      style={{
        background: '#ffffff',
        border: contact.is_primary ? '1.5px solid rgba(235,93,30,0.25)' : '1px solid rgba(0,0,0,0.08)',
        borderRadius: '12px',
        padding: '20px',
        display: 'flex',
        gap: '16px',
        alignItems: 'flex-start',
        boxShadow: contact.is_primary
          ? '0 2px 12px rgba(235,93,30,0.08)'
          : '0 1px 6px rgba(0,0,0,0.04)',
      }}
    >
      <div
        style={{
          width: '44px',
          height: '44px',
          borderRadius: '50%',
          background: contact.is_primary
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
        {initials(contact.first_name, contact.last_name)}
      </div>

      <div style={{ flex: 1 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '2px' }}>
          <p style={{ fontSize: '15px', fontWeight: 600, color: '#1A1714' }}>
            {name}
          </p>
          {contact.is_primary && (
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
          {contact.phone_e164 && (
            <a
              href={`tel:${contact.phone_e164}`}
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
              {fmtPhone(contact.phone_e164)}
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

function SwimmerProfileCard({
  swimmer,
  idx,
  groupName,
}: {
  swimmer: SwimmerRecord
  idx: number
  groupName: string
}) {
  const regKey = swimmer.registration_status === 'active' ? 'active' : swimmer.registration_status === 'pending' ? 'pending' : 'default'
  const regCfg = REG_STATUS_CONFIG[regKey]
  const gradient = GRADIENTS[idx % GRADIENTS.length]
  const ini = initials(swimmer.first_name, swimmer.last_name)
  const age = calcAge(swimmer.date_of_birth)

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
            background: gradient,
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
          {ini}
        </div>
        <div style={{ flex: 1 }}>
          <p style={{ fontSize: '16px', fontWeight: 700, color: '#1A1714', marginBottom: '3px' }}>
            {swimmer.first_name} {swimmer.last_name}
          </p>
          <p style={{ fontSize: '12.5px', color: '#6B6866' }}>
            Age {age} · {swimmer.gender}{groupName ? ` · ${groupName}` : ''}
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

      <dl style={{ padding: '4px 22px 12px' }}>
        <InfoRow label="Date of Birth" value={fmtDOB(swimmer.date_of_birth)} />
        <InfoRow label="USS Number"    value={swimmer.swimmer_external_id ?? '—'} />
      </dl>

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
          href={`/portal/household/swimmers/${swimmer.id}`}
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
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''

  const [household, setHousehold] = useState<HouseholdRecord | null>(null)
  const [contacts, setContacts] = useState<ContactRecord[]>([])
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [swimmerGroups, setSwimmerGroups] = useState<SwimmerGroupLinkRecord[]>([])
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<Tab>('overview')

  useEffect(() => {
    if (!orgId || !householdId) { setLoading(false); return }
    setLoading(true)
    Promise.all([
      fetchHousehold(api, orgId, householdId),
      fetchHouseholdContacts(api, orgId, householdId),
      fetchSwimmers(api, orgId),
      fetchSwimmerGroups(api, orgId),
      fetchGroups(api, orgId),
    ])
      .then(([hh, ctcts, swmrs, sGroups, grps]) => {
        setHousehold(hh)
        setContacts(ctcts)
        setSwimmers(swmrs.filter((s) => s.household_id === householdId))
        setSwimmerGroups(sGroups)
        setGroups(grps)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [api, orgId, householdId])

  function getGroupName(swimmerId: string): string {
    const primaryLink = swimmerGroups.find(
      (sg) => sg.swimmer_id === swimmerId && sg.is_primary
    ) ?? swimmerGroups.find((sg) => sg.swimmer_id === swimmerId)
    if (!primaryLink) return ''
    const grp = groups.find((g) => g.code === primaryLink.group_code)
    return grp?.name ?? primaryLink.group_code
  }

  const tabs: { id: Tab; label: string; count?: number }[] = [
    { id: 'overview', label: 'Overview' },
    { id: 'contacts', label: 'Contacts', count: contacts.length },
    { id: 'swimmers', label: 'Swimmers', count: swimmers.length },
  ]

  const hhName = household?.household_name ?? '—'
  const firstLetter = hhName.split(' ')[0]?.[0] ?? 'H'

  return (
    <div style={{ animation: 'slideUp 0.35s var(--ease-out) both' }}>
      <style>{`
        @keyframes slideUp { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
      `}</style>

      {/* Page header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: '20px', marginBottom: '32px' }}>
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
          {firstLetter}
        </div>

        <div style={{ flex: 1 }}>
          {loading ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              <Skeleton w="200px" h={28} />
              <Skeleton w="120px" h={18} />
            </div>
          ) : (
            <>
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
                {hhName}
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
                  {household?.status ?? 'Active'}
                </span>
                <span style={{ fontSize: '12.5px', color: '#9A9693' }}>
                  Member since {fmtMonthYear(household?.created_at)}
                </span>
              </div>
            </>
          )}
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
            {loading ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                <Skeleton h={14} />
                <Skeleton h={14} />
              </div>
            ) : (
              <dl>
                <InfoRow label="Email" value={household?.primary_email ?? '—'} />
                <InfoRow label="Phone" value={fmtPhone(household?.primary_phone_e164)} />
              </dl>
            )}
          </div>

          {/* Membership card */}
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
              Membership
            </h3>
            {loading ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                <Skeleton h={14} />
                <Skeleton h={14} />
              </div>
            ) : (
              <div style={{ display: 'flex', gap: '24px', flexWrap: 'wrap' }}>
                <div>
                  <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#A09D9B', letterSpacing: '0.04em', textTransform: 'uppercase', marginBottom: '4px' }}>
                    Status
                  </p>
                  <span style={{ fontSize: '13px', fontWeight: 600, color: '#16a34a', background: '#f0fdf4', border: '1px solid #bbf7d0', padding: '4px 12px', borderRadius: '999px' }}>
                    {household?.status ?? 'Active'}
                  </span>
                </div>
                <div>
                  <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#A09D9B', letterSpacing: '0.04em', textTransform: 'uppercase', marginBottom: '4px' }}>
                    Member Since
                  </p>
                  <p style={{ fontSize: '15px', fontWeight: 600, color: '#1A1714' }}>
                    {fmtMonthYear(household?.created_at)}
                  </p>
                </div>
                <div>
                  <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#A09D9B', letterSpacing: '0.04em', textTransform: 'uppercase', marginBottom: '4px' }}>
                    Swimmers
                  </p>
                  <p style={{ fontSize: '15px', fontWeight: 600, color: '#1A1714' }}>{swimmers.length}</p>
                </div>
                <div>
                  <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#A09D9B', letterSpacing: '0.04em', textTransform: 'uppercase', marginBottom: '4px' }}>
                    Household ID
                  </p>
                  <p style={{ fontSize: '13px', fontFamily: 'var(--font-mono)', color: '#6B6866' }}>
                    {household?.id ?? '—'}
                  </p>
                </div>
              </div>
            )}
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
          {loading ? (
            [0, 1, 2].map((i) => (
              <div key={i} style={{ background: '#fff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.08)', padding: '20px', display: 'flex', gap: '16px' }}>
                <Skeleton w={44} h={44} r={22} />
                <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '8px' }}>
                  <Skeleton w="60%" h={15} />
                  <Skeleton w="40%" h={12} />
                </div>
              </div>
            ))
          ) : contacts.length === 0 ? (
            <div style={{ color: '#9A9693', fontSize: '14px', padding: '20px 0' }}>No contacts on file.</div>
          ) : (
            contacts.map((contact) => (
              <ContactCard key={contact.id} contact={contact} />
            ))
          )}

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
          {loading ? (
            [0, 1].map((i) => (
              <div key={i} style={{ background: '#fff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.08)', padding: '20px 22px', display: 'flex', gap: '16px', alignItems: 'center' }}>
                <Skeleton w={52} h={52} r={26} />
                <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '8px' }}>
                  <Skeleton w="50%" h={16} />
                  <Skeleton w="70%" h={12} />
                </div>
              </div>
            ))
          ) : swimmers.length === 0 ? (
            <div style={{ color: '#9A9693', fontSize: '14px', padding: '20px 0' }}>No swimmers found for this household.</div>
          ) : (
            swimmers.map((swimmer, idx) => (
              <SwimmerProfileCard
                key={swimmer.id}
                swimmer={swimmer}
                idx={idx}
                groupName={getGroupName(swimmer.id)}
              />
            ))
          )}
        </div>
      )}
    </div>
  )
}
