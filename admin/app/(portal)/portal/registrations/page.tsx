'use client'

import Link from 'next/link'
import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchRegistrations,
  fetchSwimmers,
  fetchGroups,
  type RegistrationRecord,
  type SwimmerRecord,
  type GroupRecord,
} from '@/lib/adminData'

// ── Helpers ─────────────────────────────────────────────────────────────────
function fmtDate(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}


const GRADIENTS = [
  'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
  'linear-gradient(135deg, #10b981 0%, #059669 100%)',
  'linear-gradient(135deg, #f59e0b 0%, #d97706 100%)',
  'linear-gradient(135deg, #ef4444 0%, #dc2626 100%)',
]

function Skeleton({ w = '100%', h = 16, r = 8 }: { w?: string | number; h?: number; r?: number }) {
  return (
    <div
      style={{
        width: w, height: h, borderRadius: r,
        background: 'linear-gradient(90deg, #f0efee 25%, #e6e5e3 50%, #f0efee 75%)',
        backgroundSize: '200% 100%',
        animation: 'shimmer 1.5s infinite',
      }}
    />
  )
}

// ── Helpers ─────────────────────────────────────────────────────────────────
const STATUS_CONFIG: Record<string, { label: string; bg: string; color: string }> = {
  active:   { label: 'Active',   bg: '#ecfdf5', color: '#059669' },
  pending:  { label: 'Pending',  bg: '#fffbeb', color: '#d97706' },
  expired:  { label: 'Expired',  bg: '#f9fafb', color: '#9A9693' },
  inactive: { label: 'Inactive', bg: '#fef2f2', color: '#dc2626' },
}

// ── Component ────────────────────────────────────────────────────────────────
export default function RegistrationsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''

  const [registrations, setRegistrations] = useState<RegistrationRecord[]>([])
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [expandedId, setExpandedId] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId || !householdId) { setLoading(false); return }
    setLoading(true)
    Promise.all([
      fetchRegistrations(api, orgId),
      fetchSwimmers(api, orgId),
      fetchGroups(api, orgId),
    ])
      .then(([regs, swmrs, grps]) => {
        const hhSwimmers = swmrs.filter((s) => s.household_id === householdId)
        const hhSwimmerIds = new Set(hhSwimmers.map((s) => s.id))
        const hhRegs = regs.filter((r) => hhSwimmerIds.has(r.swimmer_id))
        setRegistrations(hhRegs)
        setSwimmers(hhSwimmers)
        setGroups(grps)
        // Auto-expand first active registration
        const first = hhRegs.find((r) => r.status === 'active') ?? hhRegs[0]
        if (first) setExpandedId(first.id)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [api, orgId, householdId])

  function getSwimmer(swimmerId: string): SwimmerRecord | undefined {
    return swimmers.find((s) => s.id === swimmerId)
  }

  function getGroup(groupId: string): GroupRecord | undefined {
    return groups.find((g) => g.id === groupId)
  }

  const activeRegs = registrations.filter((r) => r.status === 'active')
  const pastRegs = registrations.filter((r) => ['expired', 'inactive'].includes(r.status))

  return (
    <div>
      <style>{`
        @keyframes slideUp { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
      `}</style>

      {/* Page header */}
      <div style={{ marginBottom: '28px', animation: 'slideUp 0.3s ease both' }}>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: '#1A1714', margin: '0 0 6px', lineHeight: 1.1 }}>
          Registration Center
        </h1>
        <p style={{ margin: 0, fontSize: '14px', color: '#9A9693' }}>
          Enrollment status and USS membership for your swimmers.
        </p>
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px', marginBottom: '28px', animation: 'slideUp 0.4s ease both' }}>
        {loading ? (
          [0, 1, 2].map((i) => (
            <div key={i} style={{ background: '#fff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.07)', padding: '16px 20px' }}>
              <Skeleton w="40%" h={22} r={4} />
              <div style={{ marginTop: '8px' }}><Skeleton w="80%" h={12} /></div>
            </div>
          ))
        ) : (
          [
            { label: 'Active Registrations', value: String(activeRegs.length), color: '#059669' },
            { label: 'Total Swimmers',        value: String(swimmers.length),   color: '#3b82f6' },
            { label: 'Season',               value: activeRegs[0]?.season ?? '—', color: '#eb5d1e' },
          ].map((stat) => (
            <div key={stat.label} style={{ background: '#ffffff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.07)', padding: '16px 20px', boxShadow: '0 1px 4px rgba(0,0,0,0.04)' }}>
              <div style={{ fontSize: '22px', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-mono)' }}>
                {stat.value}
              </div>
              <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '4px' }}>{stat.label}</div>
            </div>
          ))
        )}
      </div>

      {/* Registration cards */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '16px', marginBottom: '32px' }}>
        {loading ? (
          [0, 1].map((i) => (
            <div key={i} style={{ background: '#fff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', padding: '20px 22px', display: 'flex', gap: '16px', alignItems: 'center' }}>
              <Skeleton w={48} h={48} r={24} />
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '8px' }}>
                <Skeleton w="40%" h={18} />
                <Skeleton w="60%" h={13} />
              </div>
            </div>
          ))
        ) : registrations.length === 0 ? (
          <div style={{ background: '#fff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', padding: '40px', textAlign: 'center', color: '#9A9693', fontSize: '14px' }}>
            No registrations found for your household.
          </div>
        ) : (
          registrations.filter((r) => r.status === 'active' || r.status === 'pending').map((reg, idx) => {
            const swimmer = getSwimmer(reg.swimmer_id)
            const group = getGroup(reg.group_id)
            const isExpanded = expandedId === reg.id
            const ini = swimmer ? `${swimmer.first_name[0] ?? ''}${swimmer.last_name[0] ?? ''}`.toUpperCase() : '?'
            const gradient = GRADIENTS[idx % GRADIENTS.length]
            const statusCfg = STATUS_CONFIG[reg.status] ?? STATUS_CONFIG['active']

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
                  style={{ padding: '20px 22px', display: 'flex', alignItems: 'center', gap: '16px', cursor: 'pointer' }}
                  onClick={() => setExpandedId(isExpanded ? null : reg.id)}
                >
                  <div
                    style={{
                      width: '48px', height: '48px', borderRadius: '50%',
                      background: gradient, display: 'flex', alignItems: 'center', justifyContent: 'center',
                      color: '#fff', fontSize: '16px', fontWeight: 700, fontFamily: 'var(--font-mono)',
                      flexShrink: 0, boxShadow: '0 3px 10px rgba(0,0,0,0.15)',
                    }}
                  >
                    {ini}
                  </div>

                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <span style={{ fontFamily: 'var(--font-display)', fontSize: '18px', color: '#1A1714', lineHeight: 1.1 }}>
                        {swimmer ? `${swimmer.first_name} ${swimmer.last_name}` : reg.swimmer_id}
                      </span>
                      <span style={{ padding: '2px 8px', borderRadius: '999px', fontSize: '11px', fontWeight: 600, background: statusCfg.bg, color: statusCfg.color, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                        {statusCfg.label}
                      </span>
                    </div>
                    <div style={{ fontSize: '12.5px', color: '#9A9693', marginTop: '3px' }}>
                      {group?.name ?? group?.code ?? 'Group TBD'} · {reg.season}
                    </div>
                  </div>

                  <Icon name={isExpanded ? 'chevron-up' : 'chevron-down'} size={16} style={{ color: '#9A9693', flexShrink: 0 }} />
                </div>

                {/* Expanded content */}
                {isExpanded && (
                  <div style={{ borderTop: '1px solid rgba(0,0,0,0.06)', padding: '20px 22px' }}>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '20px', marginBottom: '20px' }}>
                      {/* Enrollment details */}
                      <div>
                        <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '12px' }}>
                          Enrollment
                        </div>
                        {[
                          { label: 'Enrolled',  value: fmtDate(reg.registration_date) },
                          { label: 'Season',    value: reg.season },
                          { label: 'Group',     value: group?.name ?? group?.code ?? '—' },
                          { label: 'Status',    value: statusCfg.label },
                        ].map((item) => (
                          <div
                            key={item.label}
                            style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid rgba(0,0,0,0.04)' }}
                          >
                            <span style={{ fontSize: '12.5px', color: '#9A9693' }}>{item.label}</span>
                            <span style={{ fontSize: '12.5px', color: '#1A1714', fontWeight: 500 }}>{item.value}</span>
                          </div>
                        ))}
                        {swimmer && (
                          <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid rgba(0,0,0,0.04)' }}>
                            <span style={{ fontSize: '12.5px', color: '#9A9693' }}>USS #</span>
                            <span style={{ fontSize: '12.5px', color: '#1A1714', fontWeight: 500, fontFamily: 'var(--font-mono)' }}>
                              {swimmer.swimmer_external_id ?? '—'}
                            </span>
                          </div>
                        )}
                      </div>

                      {/* Waivers placeholder */}
                      <div>
                        <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '12px' }}>
                          Documents & Waivers
                        </div>
                        <div style={{ background: '#f9fafb', borderRadius: '10px', padding: '16px', textAlign: 'center', color: '#9A9693', fontSize: '13px', lineHeight: 1.6 }}>
                          <Icon name="clipboard-list" size={20} style={{ display: 'block', margin: '0 auto 8px', color: '#C0BDBB' }} />
                          Manage waivers and documents in the{' '}
                          <Link href="/portal/documents" style={{ color: '#eb5d1e', textDecoration: 'none', fontWeight: 500 }}>
                            Documents Center
                          </Link>
                          .
                        </div>
                      </div>
                    </div>

                    <Link
                      href={`/portal/household/swimmers/${reg.swimmer_id}`}
                      style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', fontSize: '13px', color: '#eb5d1e', textDecoration: 'none', fontWeight: 500 }}
                    >
                      <Icon name="user" size={13} />
                      View swimmer profile
                    </Link>
                  </div>
                )}
              </div>
            )
          })
        )}
      </div>

      {/* Past registrations */}
      {!loading && pastRegs.length > 0 && (
        <div style={{ animation: 'slideUp 0.6s ease both' }}>
          <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '12px' }}>
            Past Seasons
          </div>
          <div style={{ background: '#ffffff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.07)', overflow: 'hidden' }}>
            {pastRegs.map((reg, i) => {
              const sw = getSwimmer(reg.swimmer_id)
              const statusCfg = STATUS_CONFIG[reg.status] ?? STATUS_CONFIG['expired']
              return (
                <div
                  key={reg.id}
                  style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '12px 18px', borderBottom: i < pastRegs.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none' }}
                >
                  <div>
                    <span style={{ fontSize: '13.5px', color: '#6B6866', fontWeight: 500 }}>
                      {sw ? `${sw.first_name} ${sw.last_name}` : '—'}
                    </span>
                    <span style={{ fontSize: '13px', color: '#9A9693', marginLeft: '8px' }}>— {reg.season}</span>
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                    <span style={{ fontSize: '12px', color: '#9A9693' }}>{fmtDate(reg.registration_date)}</span>
                    <span style={{ padding: '2px 8px', borderRadius: '999px', fontSize: '11px', background: statusCfg.bg, color: statusCfg.color, fontWeight: 500 }}>
                      {statusCfg.label}
                    </span>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
