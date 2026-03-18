'use client'

import { useState, useMemo } from 'react'
import { Icon, type IconName } from '@/components/ui/icons'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type AuditAction =
  | 'auth.role.assign'
  | 'billing.invoice.issue'
  | 'billing.invoice.void'
  | 'billing.invoice.write_off'
  | 'billing.refund.create'
  | 'meet.entry.override'
  | 'meet.entry.approve'
  | 'meet.commitments.lock'
  | 'performance.best_time.override'
  | 'import.conflict.resolve'
  | 'members.swimmer.update'
  | 'compliance.data_export.request'
  | 'compliance.data_deletion.request'

type ResourceType = 'invoice' | 'meet' | 'entry' | 'swimmer' | 'import' | 'user' | 'compliance_request'

type AuditEvent = {
  id: string
  action: AuditAction
  resourceType: ResourceType
  resourceId: string
  actorName: string
  actorRole: string
  actorId: string
  description: string
  meta: Record<string, string>
  createdAt: string
}

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const MOCK_EVENTS: AuditEvent[] = [
  { id: 'ae-001', action: 'billing.invoice.issue',      resourceType: 'invoice',           resourceId: 'inv-001', actorName: 'Admin User',    actorRole: 'admin',   actorId: 'usr-001', description: 'Invoice INV-2026-0001 issued to Chen Family',          meta: { amount: '$850.00', household: 'Chen Family' },                     createdAt: '2026-03-01T09:00:00Z' },
  { id: 'ae-002', action: 'billing.refund.create',      resourceType: 'invoice',           resourceId: 'inv-004', actorName: 'Admin User',    actorRole: 'admin',   actorId: 'usr-001', description: 'Refund of $150.00 issued on INV-2026-0004',            meta: { amount: '$150.00', reason: 'Duplicate charge' },                   createdAt: '2026-03-07T16:41:00Z' },
  { id: 'ae-003', action: 'billing.invoice.write_off',  resourceType: 'invoice',           resourceId: 'inv-007', actorName: 'Owner',         actorRole: 'owner',   actorId: 'usr-000', description: 'Invoice INV-2026-0007 written off — financial hardship', meta: { amount: '$850.00', household: 'Wilson Family' },                   createdAt: '2026-03-12T14:00:00Z' },
  { id: 'ae-004', action: 'meet.entry.override',        resourceType: 'entry',             resourceId: 'ent-022', actorName: 'Coach Martinez',actorRole: 'coach',   actorId: 'usr-003', description: 'Entry override for Emma Chen — 100 Free, seed 57.42',   meta: { swimmer: 'Emma Chen', event: '100 Free', seedTime: '57.42' },      createdAt: '2026-03-14T10:30:00Z' },
  { id: 'ae-005', action: 'meet.entry.approve',         resourceType: 'entry',             resourceId: 'ent-023', actorName: 'Coach Kim',     actorRole: 'coach',   actorId: 'usr-004', description: 'Entry approved for Jenna Park — 200 IM',               meta: { swimmer: 'Jenna Park', event: '200 IM' },                           createdAt: '2026-03-14T10:45:00Z' },
  { id: 'ae-006', action: 'meet.commitments.lock',      resourceType: 'meet',              resourceId: 'meet-001',actorName: 'Coach Martinez',actorRole: 'coach',   actorId: 'usr-003', description: 'Commitments locked for Spring Championships',          meta: { meet: 'Spring Championships', totalCommitted: '87' },              createdAt: '2026-03-10T17:00:00Z' },
  { id: 'ae-007', action: 'auth.role.assign',           resourceType: 'user',              resourceId: 'usr-012', actorName: 'Owner',         actorRole: 'owner',   actorId: 'usr-000', description: 'Role billing assigned to billing@dare.org',           meta: { role: 'billing', targetUser: 'billing@dare.org' },                 createdAt: '2026-03-02T08:00:00Z' },
  { id: 'ae-008', action: 'performance.best_time.override', resourceType: 'swimmer',      resourceId: 'swm-005', actorName: 'Coach Martinez',actorRole: 'coach',   actorId: 'usr-003', description: 'Best time override for Ade Okafor — 100 Back 56.12',   meta: { swimmer: 'Ade Okafor', event: '100 Back', time: '56.12' },         createdAt: '2026-03-15T09:15:00Z' },
  { id: 'ae-009', action: 'import.conflict.resolve',    resourceType: 'import',            resourceId: 'imp-001', actorName: 'Coach Kim',     actorRole: 'coach',   actorId: 'usr-004', description: 'Conflict con-001 resolved — matched to Sofia Torres',  meta: { conflict: 'con-001', resolution: 'matched', swimmer: 'Sofia Torres' }, createdAt: '2026-03-15T11:00:00Z' },
  { id: 'ae-010', action: 'members.swimmer.update',     resourceType: 'swimmer',           resourceId: 'swm-003', actorName: 'Admin User',    actorRole: 'admin',   actorId: 'usr-001', description: 'Swimmer profile updated — Sofia Torres',               meta: { field: 'ussId', oldValue: 'UNKNOWN', newValue: '999888777' },      createdAt: '2026-03-15T11:05:00Z' },
  { id: 'ae-011', action: 'compliance.data_export.request', resourceType: 'compliance_request', resourceId: 'dr-001', actorName: 'Admin User', actorRole: 'admin', actorId: 'usr-001', description: 'Data export requested for Wilson Family (GDPR)',        meta: { household: 'Wilson Family', reason: 'GDPR Article 20' },           createdAt: '2026-03-13T09:00:00Z' },
  { id: 'ae-012', action: 'billing.invoice.void',       resourceType: 'invoice',           resourceId: 'inv-005', actorName: 'Admin User',    actorRole: 'admin',   actorId: 'usr-001', description: 'Invoice INV-2026-0005 voided — billed in error',       meta: { amount: '$45.00', reason: 'Billed in error' },                     createdAt: '2026-03-08T10:45:00Z' },
  { id: 'ae-013', action: 'compliance.data_deletion.request', resourceType: 'compliance_request', resourceId: 'dr-002', actorName: 'Owner', actorRole: 'owner', actorId: 'usr-000', description: 'Data deletion request created for former member',       meta: { reason: 'Member request — CCPA' },                                 createdAt: '2026-03-14T14:00:00Z' },
  { id: 'ae-014', action: 'auth.role.assign',           resourceType: 'user',              resourceId: 'usr-015', actorName: 'Owner',         actorRole: 'owner',   actorId: 'usr-000', description: 'Role coach assigned to coach.new@dare.org',           meta: { role: 'coach', targetUser: 'coach.new@dare.org' },                 createdAt: '2026-03-03T10:00:00Z' },
  { id: 'ae-015', action: 'meet.entry.override',        resourceType: 'entry',             resourceId: 'ent-035', actorName: 'Coach Kim',     actorRole: 'coach',   actorId: 'usr-004', description: 'Entry override for Caleb Henderson — 100 Free',        meta: { swimmer: 'Caleb Henderson', event: '100 Free', seedTime: '52.84' }, createdAt: '2026-03-14T11:00:00Z' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  })
}

const ACTION_COLORS: Record<string, string> = {
  'auth':        'var(--info)',
  'billing':     'var(--warning)',
  'meet':        'var(--accent)',
  'performance': 'var(--success)',
  'import':      'var(--text-subtle)',
  'members':     'var(--text-subtle)',
  'compliance':  'var(--error)',
}

const ACTION_BG: Record<string, string> = {
  'auth':        'var(--info-bg)',
  'billing':     'var(--warning-bg)',
  'meet':        'var(--accent-subtle)',
  'performance': 'var(--success-bg)',
  'import':      'var(--border)',
  'members':     'var(--border)',
  'compliance':  'var(--error-bg)',
}

const ACTION_BORDER: Record<string, string> = {
  'auth':        'var(--info-border)',
  'billing':     'var(--warning-border)',
  'meet':        'var(--accent-border)',
  'performance': 'var(--success-border)',
  'import':      'var(--border)',
  'members':     'var(--border)',
  'compliance':  'var(--error-border)',
}

function getActionColor(action: string): string {
  const prefix = action.split('.')[0]
  return ACTION_COLORS[prefix] ?? 'var(--text-muted)'
}

function getActionBg(action: string): string {
  const prefix = action.split('.')[0]
  return ACTION_BG[prefix] ?? 'var(--border)'
}

function getActionBorder(action: string): string {
  const prefix = action.split('.')[0]
  return ACTION_BORDER[prefix] ?? 'var(--border)'
}

function ActionBadge({ action }: { action: AuditAction }) {
  return (
    <span style={{
      display: 'inline-block',
      padding: '2px 7px', borderRadius: 4,
      fontSize: 10, fontWeight: 700, letterSpacing: '0.05em',
      fontFamily: 'var(--font-mono)',
      color: getActionColor(action),
      background: getActionBg(action),
      border: `1px solid ${getActionBorder(action)}`,
      whiteSpace: 'nowrap',
    }}>
      {action}
    </span>
  )
}

const RESOURCE_ICONS: Record<ResourceType, string> = {
  invoice:            'file-text',
  meet:               'calendar',
  entry:              'clipboard-list',
  swimmer:            'users',
  import:             'upload-cloud',
  user:               'user',
  compliance_request: 'shield-check',
}

// ─── Filter helpers ───────────────────────────────────────────────────────────

const ACTION_PREFIXES = [
  { value: '',            label: 'All actions' },
  { value: 'auth',        label: 'Auth / Roles' },
  { value: 'billing',     label: 'Billing' },
  { value: 'meet',        label: 'Meet / Entries' },
  { value: 'performance', label: 'Performance' },
  { value: 'import',      label: 'Imports' },
  { value: 'members',     label: 'Members' },
  { value: 'compliance',  label: 'Compliance' },
]

const RESOURCE_TYPES: { value: ResourceType | ''; label: string }[] = [
  { value: '',                   label: 'All resources' },
  { value: 'invoice',            label: 'Invoice' },
  { value: 'meet',               label: 'Meet' },
  { value: 'entry',              label: 'Entry' },
  { value: 'swimmer',            label: 'Swimmer' },
  { value: 'import',             label: 'Import' },
  { value: 'user',               label: 'User' },
  { value: 'compliance_request', label: 'Compliance Request' },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function AuditLogPage() {
  const [search, setSearch]           = useState('')
  const [actionFilter, setActionFilter] = useState('')
  const [resourceFilter, setResourceFilter] = useState<ResourceType | ''>('')
  const [dateFrom, setDateFrom]       = useState('')
  const [dateTo, setDateTo]           = useState('')
  const [expandedId, setExpandedId]   = useState<string | null>(null)

  const filtered = useMemo(() => {
    return MOCK_EVENTS.filter(e => {
      if (actionFilter && !e.action.startsWith(actionFilter)) return false
      if (resourceFilter && e.resourceType !== resourceFilter) return false
      if (dateFrom && e.createdAt < dateFrom) return false
      if (dateTo && e.createdAt > dateTo + 'T23:59:59Z') return false
      if (search) {
        const q = search.toLowerCase()
        if (!e.description.toLowerCase().includes(q) &&
            !e.actorName.toLowerCase().includes(q) &&
            !e.resourceId.toLowerCase().includes(q)) return false
      }
      return true
    }).sort((a, b) => b.createdAt.localeCompare(a.createdAt))
  }, [search, actionFilter, resourceFilter, dateFrom, dateTo])

  const hasFilter = search || actionFilter || resourceFilter || dateFrom || dateTo

  return (
    <div style={{ padding: '32px 36px', maxWidth: 1300, margin: '0 auto' }}>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 28 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Icon name="shield-check" size={16} color="var(--accent)" />
            </div>
            <h1 style={{ fontSize: 22, fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--text)', margin: 0 }}>
              Audit Log
            </h1>
          </div>
          <p style={{ fontSize: 13, color: 'var(--text-muted)', margin: 0 }}>
            Immutable record of all privileged actions — {MOCK_EVENTS.length} events · 7-year retention
          </p>
        </div>
        <button style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '8px 14px', borderRadius: 7,
          background: 'var(--bg-elevated)', border: '1px solid var(--border)',
          color: 'var(--text-subtle)', fontSize: 12, fontWeight: 500, cursor: 'pointer',
        }}>
          <Icon name="download" size={13} /> Export CSV
        </button>
      </div>

      {/* Filters */}
      <div style={{
        display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 10, marginBottom: 20,
        padding: '14px 16px', borderRadius: 9,
        background: 'var(--bg-elevated)', border: '1px solid var(--border)',
      }}>
        <div style={{ position: 'relative', flex: '1 1 220px', minWidth: 0 }}>
          <Icon name="search" size={14} color="var(--text-muted)"
            style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' }} />
          <input
            value={search} onChange={e => setSearch(e.target.value)}
            placeholder="Search description, actor, resource ID…"
            style={{
              width: '100%', padding: '7px 10px 7px 32px', borderRadius: 6,
              background: 'var(--bg)', border: '1px solid var(--border)',
              color: 'var(--text)', fontSize: 13, outline: 'none', boxSizing: 'border-box',
            }}
          />
        </div>
        <select
          value={actionFilter} onChange={e => setActionFilter(e.target.value)}
          style={{
            padding: '7px 10px', borderRadius: 6,
            background: 'var(--bg)', border: '1px solid var(--border)',
            color: actionFilter ? 'var(--text)' : 'var(--text-muted)', fontSize: 13, cursor: 'pointer',
          }}
        >
          {ACTION_PREFIXES.map(a => <option key={a.value} value={a.value}>{a.label}</option>)}
        </select>
        <select
          value={resourceFilter} onChange={e => setResourceFilter(e.target.value as ResourceType | '')}
          style={{
            padding: '7px 10px', borderRadius: 6,
            background: 'var(--bg)', border: '1px solid var(--border)',
            color: resourceFilter ? 'var(--text)' : 'var(--text-muted)', fontSize: 13, cursor: 'pointer',
          }}
        >
          {RESOURCE_TYPES.map(r => <option key={r.value} value={r.value}>{r.label}</option>)}
        </select>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <input type="date" value={dateFrom} onChange={e => setDateFrom(e.target.value)}
            style={{ padding: '7px 10px', borderRadius: 6, background: 'var(--bg)', border: '1px solid var(--border)', color: dateFrom ? 'var(--text)' : 'var(--text-muted)', fontSize: 13, colorScheme: 'dark' }} />
          <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>–</span>
          <input type="date" value={dateTo} onChange={e => setDateTo(e.target.value)}
            style={{ padding: '7px 10px', borderRadius: 6, background: 'var(--bg)', border: '1px solid var(--border)', color: dateTo ? 'var(--text)' : 'var(--text-muted)', fontSize: 13, colorScheme: 'dark' }} />
        </div>
        {hasFilter && (
          <button
            onClick={() => { setSearch(''); setActionFilter(''); setResourceFilter(''); setDateFrom(''); setDateTo('') }}
            style={{ padding: '7px 12px', borderRadius: 6, background: 'transparent', border: '1px solid var(--border)', color: 'var(--text-muted)', fontSize: 12, cursor: 'pointer' }}
          >
            Clear
          </button>
        )}
      </div>

      {/* Result count */}
      {hasFilter && (
        <div style={{ marginBottom: 12, fontSize: 12, color: 'var(--text-muted)' }}>
          Showing {filtered.length} of {MOCK_EVENTS.length} events
        </div>
      )}

      {/* Log */}
      {filtered.length === 0 ? (
        <EmptyState icon="shield-check" title="No audit events found" description="Try adjusting your filters." />
      ) : (
        <div style={{
          borderRadius: 10, overflow: 'hidden',
          border: '1px solid var(--border)',
          background: 'var(--bg-elevated)',
        }}>
          {/* Column headers */}
          <div style={{
            display: 'grid',
            gridTemplateColumns: '160px 220px 140px 1fr 80px',
            gap: 0, padding: '10px 16px',
            borderBottom: '1px solid var(--border)',
            background: 'var(--bg)',
          }}>
            {['Timestamp', 'Action', 'Resource', 'Description', 'Actor'].map(h => (
              <span key={h} style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                {h}
              </span>
            ))}
          </div>

          {filtered.map((event, i) => {
            const isExpanded = expandedId === event.id
            return (
              <div key={event.id} style={{ borderBottom: i < filtered.length - 1 ? '1px solid var(--border)' : 'none' }}>
                <div
                  onClick={() => setExpandedId(isExpanded ? null : event.id)}
                  style={{
                    display: 'grid',
                    gridTemplateColumns: '160px 220px 140px 1fr 80px',
                    gap: 0, padding: '12px 16px',
                    cursor: 'pointer',
                    background: isExpanded ? 'var(--bg)' : 'transparent',
                    transition: 'background 150ms',
                  }}
                  onMouseEnter={e => { if (!isExpanded) (e.currentTarget as HTMLDivElement).style.background = 'var(--bg-surface)' }}
                  onMouseLeave={e => { if (!isExpanded) (e.currentTarget as HTMLDivElement).style.background = 'transparent' }}
                >
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', lineHeight: '1.4', alignSelf: 'center' }}>
                    {formatDateTime(event.createdAt)}
                  </span>
                  <div style={{ alignSelf: 'center' }}>
                    <ActionBadge action={event.action} />
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6, alignSelf: 'center' }}>
                    <Icon name={RESOURCE_ICONS[event.resourceType] as IconName} size={12} color="var(--text-muted)" />
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>
                      {event.resourceId}
                    </span>
                  </div>
                  <span style={{ fontSize: 12, color: 'var(--text-subtle)', alignSelf: 'center', lineHeight: '1.4' }}>
                    {event.description}
                  </span>
                  <div style={{ alignSelf: 'center' }}>
                    <div style={{ fontSize: 11, color: 'var(--text-subtle)', fontWeight: 500 }}>{event.actorName}</div>
                    <div style={{ fontSize: 10, color: 'var(--text-muted)' }}>{event.actorRole}</div>
                  </div>
                </div>

                {/* Expanded metadata */}
                {isExpanded && (
                  <div style={{
                    padding: '12px 16px 16px 16px',
                    background: 'var(--bg)',
                    borderTop: '1px solid var(--border)',
                  }}>
                    <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
                      <div>
                        <div style={{ fontSize: 10, color: 'var(--text-muted)', marginBottom: 3, textTransform: 'uppercase', letterSpacing: '0.06em' }}>Event ID</div>
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>{event.id}</span>
                      </div>
                      <div>
                        <div style={{ fontSize: 10, color: 'var(--text-muted)', marginBottom: 3, textTransform: 'uppercase', letterSpacing: '0.06em' }}>Actor ID</div>
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>{event.actorId}</span>
                      </div>
                      {Object.entries(event.meta).map(([k, v]) => (
                        <div key={k}>
                          <div style={{ fontSize: 10, color: 'var(--text-muted)', marginBottom: 3, textTransform: 'uppercase', letterSpacing: '0.06em' }}>{k}</div>
                          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-subtle)' }}>{v}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* Immutability notice */}
      <div style={{
        marginTop: 16, padding: '10px 14px', borderRadius: 7,
        background: 'var(--info-bg)', border: '1px solid var(--info-border)',
        display: 'flex', alignItems: 'center', gap: 8,
      }}>
        <Icon name="info" size={13} color="var(--info)" />
        <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
          Audit events are <strong style={{ color: 'var(--text-subtle)' }}>immutable</strong> and retained for a minimum of{' '}
          <strong style={{ color: 'var(--text-subtle)' }}>7 years</strong>.
          Click any row to expand metadata.
        </span>
      </div>
    </div>
  )
}
