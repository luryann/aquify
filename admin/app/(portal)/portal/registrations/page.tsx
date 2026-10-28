'use client'

import Link from 'next/link'
import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  fetchDocumentSubmissions,
  fetchDocumentTypes,
  fetchRegistrations,
  fetchSwimmers,
  fetchGroups,
  type DocumentSubmissionRecord,
  type DocumentTypeRecord,
  type RegistrationRecord,
  type SwimmerRecord,
  type GroupRecord,
} from '@/lib/adminData'
import {
  buildPortalRegistrationDocumentRows,
  summarizeRegistrationWaiver,
} from '@/lib/portal-registration-status'
import { parseBackendDate } from '@/lib/dates'

// ── Helpers ─────────────────────────────────────────────────────────────────
function fmtDate(iso?: string): string {
  if (!iso) return '—'
  return parseBackendDate(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
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
  active:   { label: 'Active',   bg: 'var(--success-bg)', color: 'var(--success)' },
  pending:  { label: 'Pending',  bg: 'var(--warning-bg)', color: 'var(--warning)' },
  expired:  { label: 'Expired',  bg: 'var(--bg-surface)', color: 'var(--text-muted)' },
  inactive: { label: 'Inactive', bg: 'var(--error-bg)', color: 'var(--error)' },
}

const DOC_STATUS_CONFIG: Record<string, { label: string; bg: string; color: string }> = {
  verified:  { label: 'Verified',        bg: 'var(--success-bg)', color: 'var(--success)' },
  submitted: { label: 'Under review',    bg: 'var(--warning-bg)', color: 'var(--warning)' },
  pending:   { label: 'Pending',         bg: 'var(--warning-bg)', color: 'var(--warning)' },
  rejected:  { label: 'Resubmit',        bg: 'var(--error-bg)', color: 'var(--error)' },
  expired:   { label: 'Expired',         bg: 'var(--error-bg)', color: 'var(--error)' },
  missing:   { label: 'Upload required', bg: 'var(--error-bg)', color: 'var(--error)' },
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
  const [documentTypes, setDocumentTypes] = useState<DocumentTypeRecord[]>([])
  const [documentSubmissions, setDocumentSubmissions] = useState<DocumentSubmissionRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [retryKey, setRetryKey] = useState(0)

  useEffect(() => {
    if (!orgId || !householdId) {
      setLoadError('Your portal session is missing household context. Refresh and sign in again.')
      setLoading(false)
      return
    }
    setLoading(true)
    setLoadError(null)
    Promise.all([
      fetchRegistrations(api, orgId),
      fetchSwimmers(api, orgId),
      fetchGroups(api, orgId),
      fetchDocumentTypes(api, orgId, true),
      fetchDocumentSubmissions(api, orgId, householdId),
    ])
      .then(([regs, swmrs, grps, docTypes, docSubs]) => {
        const hhSwimmers = swmrs.filter((s) => s.household_id === householdId)
        const hhSwimmerIds = new Set(hhSwimmers.map((s) => s.id))
        const hhRegs = regs.filter((r) => hhSwimmerIds.has(r.swimmer_id))
        setRegistrations(hhRegs)
        setSwimmers(hhSwimmers)
        setGroups(grps)
        setDocumentTypes(docTypes)
        setDocumentSubmissions(docSubs)
        // Auto-expand first active registration
        const first = hhRegs.find((r) => r.status === 'active') ?? hhRegs[0]
        if (first) setExpandedId(first.id)
      })
      .catch((error) => {
        reportUIError('portal.registrations.load', error, 'Couldn’t load registrations. Try again.', {
          orgId,
          householdId,
        })
        setLoadError('Couldn’t load registrations. Try again.')
      })
      .finally(() => setLoading(false))
  }, [api, orgId, householdId, retryKey])

  function getSwimmer(swimmerId: string): SwimmerRecord | undefined {
    return swimmers.find((s) => s.id === swimmerId)
  }

  function getGroup(groupId: string): GroupRecord | undefined {
    return groups.find((g) => g.id === groupId)
  }

  const activeRegs = registrations.filter((r) => r.status === 'active')
  const pendingRegs = registrations.filter((r) => r.status === 'pending')
  const pastRegs = registrations.filter((r) => ['expired', 'inactive'].includes(r.status))

  return (
    <div>
      <style>{`
        @keyframes slideUp { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
      `}</style>

      {/* Page header */}
      <div className="page-heading">
        <h1 className="page-title">
          Registration Center
        </h1>
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: '12px', marginBottom: '24px', animation: 'slideUp 0.4s ease both' }}>
        {loading ? (
          [0, 1, 2, 3].map((i) => (
            <div key={i} className="card" style={{ padding: '16px 20px' }}>
              <Skeleton w="40%" h={22} r={4} />
              <div style={{ marginTop: '8px' }}><Skeleton w="80%" h={12} /></div>
            </div>
          ))
        ) : (
          [
            { label: 'Active registrations', value: String(activeRegs.length), color: 'var(--success)' },
            { label: 'Pending registrations', value: String(pendingRegs.length), color: 'var(--warning)' },
            { label: 'Total swimmers',        value: String(swimmers.length),   color: 'var(--info)' },
            { label: 'Season',               value: activeRegs[0]?.season ?? pendingRegs[0]?.season ?? '—', color: 'var(--accent)' },
          ].map((stat) => (
            <div key={stat.label} className="card" style={{ padding: '16px 20px' }}>
              <div style={{ fontSize: 'var(--fs-2xl)', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-body)' }}>
                {stat.value}
              </div>
              <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '4px' }}>{stat.label}</div>
            </div>
          ))
        )}
      </div>

      {!loading && loadError && (
        <div style={{ marginBottom: '20px', background: 'var(--error-bg)', border: '1px solid rgba(220,38,38,0.18)', borderRadius: '12px', padding: '20px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
          <div>
            <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--error)', marginBottom: '4px' }}>
              Registration data is unavailable
            </div>
            <div style={{ fontSize: 'var(--fs-base)', color: 'var(--error)' }}>{loadError}</div>
          </div>
          <button
            onClick={() => setRetryKey((current) => current + 1)}
            style={{ padding: '8px 16px', borderRadius: '8px', border: 'none', background: 'var(--error)', color: 'var(--bg-elevated)', fontSize: 'var(--fs-base)', fontWeight: 600, cursor: 'pointer', flexShrink: 0 }}
          >
            Retry
          </button>
        </div>
      )}

      {/* Registration cards */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '16px', marginBottom: '32px' }}>
        {loading ? (
          [0, 1].map((i) => (
            <div key={i} className="card" style={{ padding: '20px 24px', display: 'flex', gap: '16px', alignItems: 'center' }}>
              <Skeleton w={48} h={48} r={24} />
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '8px' }}>
                <Skeleton w="40%" h={18} />
                <Skeleton w="60%" h={13} />
              </div>
            </div>
          ))
        ) : loadError ? null : registrations.length === 0 ? (
          <div className="card" style={{ padding: '40px', textAlign: 'center', color: 'var(--text-muted)', fontSize: 'var(--fs-md)' }}>
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
            const waiverSummary = summarizeRegistrationWaiver(reg)
            const documentRows = buildPortalRegistrationDocumentRows({
              registration: reg,
              swimmerId: reg.swimmer_id,
              documentTypes,
              submissions: documentSubmissions,
            })

            return (
              <div
                key={reg.id}
                style={{
                  background: 'var(--bg-elevated)',
                  borderRadius: '12px',
                  border: '1px solid rgba(0,0,0,0.07)',
                  overflow: 'hidden',
                  animation: `slideUp ${0.4 + idx * 0.08}s ease both`,
                }}
              >
                {/* Card header */}
                <div
                  style={{ padding: '20px 24px', display: 'flex', alignItems: 'center', gap: '16px', cursor: 'pointer' }}
                  onClick={() => setExpandedId(isExpanded ? null : reg.id)}
                >
                  <div
                    style={{
                      width: '48px', height: '48px', borderRadius: '50%',
                      background: gradient, display: 'flex', alignItems: 'center', justifyContent: 'center',
                      color: 'var(--bg-elevated)', fontSize: 'var(--fs-lg)', fontWeight: 700, fontFamily: 'var(--font-body)',
                      flexShrink: 0,
                    }}
                  >
                    {ini}
                  </div>

                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <span style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: 'var(--text)', lineHeight: 1.1 }}>
                        {swimmer ? `${swimmer.first_name} ${swimmer.last_name}` : reg.swimmer_id}
                      </span>
                      <span style={{ padding: '2px 8px', borderRadius: '999px', fontSize: 'var(--fs-xs)', fontWeight: 600, background: statusCfg.bg, color: statusCfg.color, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                        {statusCfg.label}
                      </span>
                    </div>
                    <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '4px' }}>
                      {group?.name ?? group?.code ?? 'Group TBD'} · {reg.season}
                    </div>
                  </div>

                  <Icon name={isExpanded ? 'chevron-up' : 'chevron-down'} size={16} style={{ color: 'var(--text-muted)', flexShrink: 0 }} />
                </div>

                {/* Expanded content */}
                {isExpanded && (
                  <div style={{ borderTop: '1px solid rgba(0,0,0,0.06)', padding: '20px 24px' }}>
                    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: '20px', marginBottom: '20px' }}>
                      {/* Enrollment details */}
                      <div>
                        <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '12px' }}>
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
                            <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>{item.label}</span>
                            <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text)', fontWeight: 500 }}>{item.value}</span>
                          </div>
                        ))}
                        {swimmer && (
                          <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid rgba(0,0,0,0.04)' }}>
                            <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>USS #</span>
                            <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text)', fontWeight: 500, fontFamily: 'var(--font-body)' }}>
                              {swimmer.swimmer_external_id ?? '—'}
                            </span>
                          </div>
                        )}
                      </div>

                      {/* Waivers and document status */}
                      <div>
                        <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '12px' }}>
                          Documents & waivers
                        </div>
                        <div style={{ background: 'var(--bg-surface)', borderRadius: '12px', padding: '16px', color: 'var(--text-subtle)', fontSize: 'var(--fs-base)', lineHeight: 1.6 }}>
                          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '12px', marginBottom: '12px' }}>
                            <div>
                              <div style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--text)', marginBottom: '2px' }}>
                                {waiverSummary.accepted ? 'Registration waiver accepted' : 'Registration waiver still pending'}
                              </div>
                              <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>
                                {waiverSummary.accepted
                                  ? `${waiverSummary.count} accepted waiver${waiverSummary.count === 1 ? '' : 's'}${waiverSummary.lastAcceptedAt ? ` • last accepted ${fmtDate(waiverSummary.lastAcceptedAt)}` : ''}`
                                  : 'No registration waiver acceptance is recorded yet.'}
                              </div>
                            </div>
                            <span style={{
                              padding: '4px 8px',
                              borderRadius: '999px',
                              fontSize: 'var(--fs-xs)',
                              fontWeight: 600,
                              background: waiverSummary.accepted ? 'var(--success-bg)' : 'var(--error-bg)',
                              color: waiverSummary.accepted ? 'var(--success)' : 'var(--error)',
                              whiteSpace: 'nowrap',
                            }}>
                              {waiverSummary.accepted ? 'Accepted' : 'Pending'}
                            </span>
                          </div>

                          {waiverSummary.types.length > 0 && (
                            <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginBottom: '12px' }}>
                              {waiverSummary.types.map((type) => (
                                <span key={type} style={{ padding: '2px 8px', borderRadius: '999px', background: 'var(--bg-elevated)', border: '1px solid rgba(0,0,0,0.08)', fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: 'var(--text-subtle)' }}>
                                  {type}
                                </span>
                              ))}
                            </div>
                          )}

                          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginBottom: '12px' }}>
                            {documentRows.length === 0 ? (
                              <div style={{ color: 'var(--text-muted)' }}>No waiver or registration documents are required for this registration yet.</div>
                            ) : (
                              documentRows.map((row) => {
                                const docStatus = DOC_STATUS_CONFIG[row.effectiveStatus]
                                return (
                                  <div key={row.id} className="card" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '12px', padding: '12px 12px' }}>
                                    <div>
                                      <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text)', fontWeight: 600 }}>{row.name}</div>
                                      <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)' }}>
                                        {row.submittedAt ? `Submitted ${fmtDate(row.submittedAt)}` : row.expiresAt ? `Expires ${fmtDate(row.expiresAt)}` : row.appliesTo === 'swimmer' ? 'Swimmer-specific requirement' : 'Household requirement'}
                                      </div>
                                    </div>
                                    <span style={{ padding: '4px 8px', borderRadius: '999px', fontSize: 'var(--fs-xs)', fontWeight: 600, background: docStatus.bg, color: docStatus.color, whiteSpace: 'nowrap' }}>
                                      {docStatus.label}
                                    </span>
                                  </div>
                                )
                              })
                            )}
                          </div>

                          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--text-muted)' }}>
                            <Icon name="clipboard-list" size={16} style={{ color: 'var(--border-strong)' }} />
                            <span>
                              Manage supporting uploads in the{' '}
                              <Link href="/portal/documents" style={{ color: 'var(--accent)', textDecoration: 'none', fontWeight: 500 }}>
                                Documents Center
                              </Link>
                              .
                            </span>
                          </div>
                        </div>
                      </div>
                    </div>

                    <Link
                      href={`/portal/household/swimmers/${reg.swimmer_id}`}
                      style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', fontSize: 'var(--fs-base)', color: 'var(--accent)', textDecoration: 'none', fontWeight: 500 }}
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
          <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '12px' }}>
            Past seasons
          </div>
          <div className="card" style={{ overflow: 'hidden' }}>
            {pastRegs.map((reg, i) => {
              const sw = getSwimmer(reg.swimmer_id)
              const statusCfg = STATUS_CONFIG[reg.status] ?? STATUS_CONFIG['expired']
              return (
                <div
                  key={reg.id}
                  style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '12px 20px', borderBottom: i < pastRegs.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none' }}
                >
                  <div>
                    <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', fontWeight: 500 }}>
                      {sw ? `${sw.first_name} ${sw.last_name}` : '—'}
                    </span>
                    <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', marginLeft: '8px' }}>— {reg.season}</span>
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                    <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>{fmtDate(reg.registration_date)}</span>
                    <span style={{ padding: '2px 8px', borderRadius: '999px', fontSize: 'var(--fs-xs)', background: statusCfg.bg, color: statusCfg.color, fontWeight: 500 }}>
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
