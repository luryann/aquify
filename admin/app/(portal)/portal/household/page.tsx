'use client'

import Link from 'next/link'
import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError } from '@/lib/frontend-telemetry'
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
import { formatDateOnly, parseBackendDate } from '@/lib/dates'
import './page.inline.css'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { ShimmerBlock } from '@/components/ui/ShimmerBlock'
import { SWIMMER_AVATAR_COLOR } from '@/lib/avatar-colors'

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
  return parseBackendDate(iso).toLocaleDateString('en-US', { month: 'long', year: 'numeric' })
}

function calcAge(dob: string): number {
  const d = parseBackendDate(dob)
  const now = new Date()
  let age = now.getFullYear() - d.getFullYear()
  const m = now.getMonth() - d.getMonth()
  if (m < 0 || (m === 0 && now.getDate() < d.getDate())) age--
  return age
}

function initials(first: string, last: string): string {
  return `${first[0] ?? ''}${last[0] ?? ''}`.toUpperCase()
}

const REG_STATUS_CONFIG = {
  active:   { label: 'Registered', color: 'var(--success)', bg: 'var(--success-bg)', border: 'var(--success-bg)' },
  pending:  { label: 'Pending',    color: 'var(--warning)', bg: 'var(--warning-bg)', border: 'var(--warning-border)' },
  default:  { label: 'Not enrolled', color: 'var(--text-muted)', bg: 'var(--bg-surface)', border: 'var(--border)' },
}

type Tab = 'overview' | 'contacts' | 'swimmers'

// ── Skeleton ─────────────────────────────────────────────────────────────────
// ── Components ─────────────────────────────────────────────────────────────
function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div
      className="is-54e0865"
    >
      <dt
        className="is-c686ed6"
      >
        {label}
      </dt>
      <dd className="is-5a91e9a">
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
        background: 'var(--bg-elevated)',
        border: contact.is_primary ? '1.5px solid rgba(235,93,30,0.25)' : '1px solid var(--border)',
        borderRadius: '12px',
        padding: '20px',
        display: 'flex',
        gap: '16px',
        alignItems: 'flex-start',
      }}
    >
      <div
        style={{
          width: '44px',
          height: '44px',
          borderRadius: '50%',
          background: contact.is_primary
            ? 'var(--accent)'
            : 'var(--text-muted)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'var(--bg-elevated)',
          fontSize: 'var(--fs-md)',
          fontWeight: 700,
          fontFamily: 'var(--font-body)',
          flexShrink: 0,
        }}
      >
        {initials(contact.first_name, contact.last_name)}
      </div>

      <div style={{ flex: 1 }}>
        <div className="is-92d0f5a">
          <p className="is-7ae0218">
            {name}
          </p>
          {contact.is_primary && (
            <span
              className="is-5239d01"
            >
              Primary
            </span>
          )}
        </div>
        <p className="is-76ed46e">
          {contact.relationship}
        </p>

        <div className="is-ef1f3a5">
          {contact.phone_e164 && (
            <a
              href={`tel:${contact.phone_e164}`}
              className="is-9e21610"
            >
              <Icon name="users" size={13} style={{ color: 'var(--text-muted)' }} />
              {fmtPhone(contact.phone_e164)}
            </a>
          )}
          {contact.email && (
            <a
              href={`mailto:${contact.email}`}
              className="is-9e21610"
            >
              <Icon name="mail" size={13} style={{ color: 'var(--text-muted)' }} />
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
  groupName,
}: {
  swimmer: SwimmerRecord
  groupName: string
}) {
  const regKey = swimmer.registration_status === 'active' ? 'active' : swimmer.registration_status === 'pending' ? 'pending' : 'default'
  const regCfg = REG_STATUS_CONFIG[regKey]
  const ini = initials(swimmer.first_name, swimmer.last_name)
  const age = calcAge(swimmer.date_of_birth)

  return (
    <div
      className="card" style={{ overflow: 'hidden' }}
    >
      <div
        className="is-9aded49"
      >
        <div
          style={{
            width: '52px',
            height: '52px',
            borderRadius: '50%',
            background: SWIMMER_AVATAR_COLOR,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: 'var(--bg-elevated)',
            fontSize: 'var(--fs-lg)',
            fontWeight: 700,
            fontFamily: 'var(--font-body)',
            flexShrink: 0,
          }}
        >
          {ini}
        </div>
        <div style={{ flex: 1 }}>
          <p className="is-3f474c1">
            {swimmer.first_name} {swimmer.last_name}
          </p>
          <p className="is-83e0368">
            Age {age} · {swimmer.gender}{groupName ? ` · ${groupName}` : ''}
          </p>
        </div>
        <span
          style={{
            fontSize: 'var(--fs-xs)',
            fontWeight: 600,
            color: regCfg.color,
            background: regCfg.bg,
            border: `1px solid ${regCfg.border}`,
            padding: '4px 12px',
            borderRadius: '999px',
            fontFamily: 'var(--font-body)',
          }}
        >
          {regCfg.label}
        </span>
      </div>

      <dl className="is-8e2cdea">
        <InfoRow label="Date of birth" value={formatDateOnly(swimmer.date_of_birth)} />
        <InfoRow label="External ID"    value={swimmer.swimmer_external_id ?? '—'} />
      </dl>

      <div
        className="is-7fd66e1"
      >
        <Link
          href={`/portal/household/swimmers/${swimmer.id}`}
          className="is-f312e03 hv-dbb904"
        >
          <Icon name="user" size={13} />
          View Profile
        </Link>
        <Link
          href="/portal/meets"
          className="is-b60a01e hv-0ddfb6"
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
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId || !householdId) {
      setLoading(false)
      setLoadError('We could not find your household details. Sign out and back in, or contact your club if this keeps happening.')
      return
    }
    setLoading(true)
    setLoadError(null)
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
      .catch((error) => {
        setHousehold(null)
        setContacts([])
        setSwimmers([])
        setSwimmerGroups([])
        setGroups([])
        setLoadError(reportUIError(
          'portal-household-load',
          error,
          'We could not load this page. Check your connection and try again.',
          { orgId, householdId },
        ))
      })
      .finally(() => setLoading(false))
  }, [api, orgId, householdId, reloadKey])

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
    { id: 'contacts', label: 'Additional contacts', count: contacts.length },
    { id: 'swimmers', label: 'Swimmers', count: swimmers.length },
  ]

  const hhName = household?.household_name ?? '—'
  const firstLetter = hhName.split(' ')[0]?.[0] ?? 'H'

  return (
    <div style={{ animation: 'slideUp 0.35s var(--ease-out) both' }}>
      
      {/* Page header */}
      <div className="is-aa82eef">
        <div
          className="is-2d51e9a"
        >
          {firstLetter}
        </div>

        <div style={{ flex: '1 1 200px', minWidth: 0 }}>
          {loading ? (
            <div className="is-930d87f">
              <ShimmerBlock w="200px" h={28} />
              <ShimmerBlock w="120px" h={18} />
            </div>
          ) : (
            <>
              <h1 className="page-title">
                {hhName}
              </h1>
              <div className="is-c960f60">
                <span
                  className="is-54b1dfb"
                >
                  {household?.status ?? 'Active'}
                </span>
                <span className="is-4f39ab0">
                  Member since {fmtMonthYear(household?.created_at)}
                </span>
              </div>
            </>
          )}
        </div>

        <button type="button"
          disabled
          title="Editing household profile details isn't available yet"
          className="is-b1aebc7"
        >
          <Icon name="edit-2" size={14} />
          Edit Profile
        </button>
      </div>

      {loadError && (
        <LoadErrorBanner message={loadError} onRetry={() => setReloadKey((key) => key + 1)} />
      )}

      {/* Tabs */}
      <div
        className="is-c0a6bbd"
      >
        {tabs.map((tab) => {
          const active = activeTab === tab.id
          return (
            <button type="button"
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              style={{
                padding: '12px 16px',
                background: 'transparent',
                border: 'none',
                borderBottom: active ? '2px solid var(--accent)' : '2px solid transparent',
                cursor: 'pointer',
                fontSize: 'var(--fs-base)',
                fontWeight: active ? 600 : 450,
                color: active ? 'var(--text)' : 'var(--text-subtle)',
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                transition: 'color 120ms ease',
                marginBottom: '-1px',
                whiteSpace: 'nowrap',
                flexShrink: 0,
              }}
            >
              {tab.label}
              {tab.count !== undefined && (
                <span
                  style={{
                    fontSize: 'var(--fs-xs)',
                    fontFamily: 'var(--font-body)',
                    color: active ? 'var(--accent)' : 'var(--text-muted)',
                    background: active ? 'rgba(235,93,30,0.08)' : 'var(--bg-wash-strong)',
                    padding: '1px 8px',
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
          className="is-b9bd260"
        >
          {/* Contact info card */}
          <div
            className="card is-0acd099" 
          >
            <h3 className="card-title is-72d6c38" 
            >
              Contact
            </h3>
            {loading ? (
              <div className="is-930d87f">
                <ShimmerBlock h={14} />
                <ShimmerBlock h={14} />
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
            className="card is-0acd099" 
          >
            <h3 className="card-title is-72d6c38" 
            >
              Membership
            </h3>
            {loading ? (
              <div className="is-930d87f">
                <ShimmerBlock h={14} />
                <ShimmerBlock h={14} />
              </div>
            ) : (
              <div className="is-529a823">
                <div>
                  <p className="is-45e49ab">
                    Status
                  </p>
                  <span className="badge" style={{ color: 'var(--success)', background: 'var(--success-bg)', border: '1px solid var(--success-border)' }}>
                    {household?.status ?? 'Active'}
                  </span>
                </div>
                <div>
                  <p className="is-45e49ab">
                    Member Since
                  </p>
                  <p className="is-7ae0218">
                    {fmtMonthYear(household?.created_at)}
                  </p>
                </div>
                <div>
                  <p className="is-45e49ab">
                    Swimmers
                  </p>
                  <p className="is-7ae0218">{swimmers.length}</p>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {activeTab === 'contacts' && (
        <div
          className="is-b98395c"
        >
          {loading ? (
            [0, 1, 2].map((i) => (
              <div key={i} className="card is-cb387ac" >
                <ShimmerBlock w={44} h={44} r={22} />
                <div className="is-20f80e0">
                  <ShimmerBlock w="60%" h={15} />
                  <ShimmerBlock w="40%" h={12} />
                </div>
              </div>
            ))
          ) : loadError ? null : shouldShowCollectionEmptyState(loading, loadError, contacts.length) ? (
            <div className="is-e37262b">No additional contacts on file. Your primary email and phone are shown under Overview.</div>
          ) : (
            contacts.map((contact) => (
              <ContactCard key={contact.id} contact={contact} />
            ))
          )}

          <button type="button"
            disabled
            title="Adding a new contact isn't available yet"
            className="is-d36ce82"
          >
            <Icon name="plus" size={22} />
            <span className="is-33a9c28">Add contact</span>
          </button>
        </div>
      )}

      {activeTab === 'swimmers' && (
        <div
          className="is-828bf30"
        >
          {loading ? (
            [0, 1].map((i) => (
              <div key={i} className="card is-ebaaa90" >
                <ShimmerBlock w={52} h={52} r={26} />
                <div className="is-20f80e0">
                  <ShimmerBlock w="50%" h={16} />
                  <ShimmerBlock w="70%" h={12} />
                </div>
              </div>
            ))
          ) : loadError ? null : shouldShowCollectionEmptyState(loading, loadError, swimmers.length) ? (
            <div className="is-e37262b">No swimmers found for this household.</div>
          ) : (
            swimmers.map((swimmer) => (
              <SwimmerProfileCard
                key={swimmer.id}
                swimmer={swimmer}
                groupName={getGroupName(swimmer.id)}
              />
            ))
          )}
        </div>
      )}
    </div>
  )
}
