'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

type ExportStatus = 'pending' | 'processing' | 'ready' | 'failed' | 'expired'

type DataExportRequest = {
  id: string
  householdId: string
  householdName: string
  requestedBy: string
  requestedAt: string
  completedAt: string | null
  status: ExportStatus
  downloadUrl: string | null
  expiresAt: string | null
  scope: string[]
}

const MOCK_REQUESTS: DataExportRequest[] = [
  {
    id: 'dex1', householdId: 'hh1', householdName: 'Allen Family',
    requestedBy: 'parent (linda.allen@example.com)', requestedAt: '2026-03-14T10:30:00Z',
    completedAt: '2026-03-14T10:35:12Z', status: 'ready',
    downloadUrl: '/api/v1/compliance/data-export-requests/dex1/download',
    expiresAt: '2026-04-13T10:35:12Z',
    scope: ['profile', 'swimmers', 'registrations', 'invoices', 'payments', 'commitments'],
  },
  {
    id: 'dex2', householdId: 'hh4', householdName: 'Park Family',
    requestedBy: 'admin (jessica.park@admin)', requestedAt: '2026-03-12T14:00:00Z',
    completedAt: '2026-03-12T14:08:44Z', status: 'ready',
    downloadUrl: '/api/v1/compliance/data-export-requests/dex2/download',
    expiresAt: '2026-04-11T14:08:44Z',
    scope: ['profile', 'swimmers', 'invoices'],
  },
  {
    id: 'dex3', householdId: 'hh7', householdName: 'Martinez Family',
    requestedBy: 'parent (carlos.martinez@example.com)', requestedAt: '2026-03-15T09:00:00Z',
    completedAt: null, status: 'processing',
    downloadUrl: null, expiresAt: null,
    scope: ['profile', 'swimmers', 'registrations', 'invoices', 'payments', 'commitments', 'audit_events'],
  },
  {
    id: 'dex4', householdId: 'hh2', householdName: 'Reyes Household',
    requestedBy: 'parent (sofia.reyes@example.com)', requestedAt: '2026-02-28T16:20:00Z',
    completedAt: '2026-03-01T08:00:00Z', status: 'expired',
    downloadUrl: null, expiresAt: '2026-03-31T08:00:00Z',
    scope: ['profile', 'swimmers'],
  },
]

const STATUS_CONFIG: Record<ExportStatus, { label: string; bg: string; border: string; color: string }> = {
  pending:    { label: 'Pending',    bg: 'var(--bg-elevated)', border: 'var(--border)',        color: 'var(--text-muted)' },
  processing: { label: 'Processing', bg: 'var(--info-bg)',     border: 'var(--info-border)',   color: 'var(--info)' },
  ready:      { label: 'Ready',      bg: 'var(--success-bg)',  border: 'var(--success-border)', color: 'var(--success)' },
  failed:     { label: 'Failed',     bg: 'var(--error-bg)',    border: 'var(--error-border)',   color: 'var(--error)' },
  expired:    { label: 'Expired',    bg: 'var(--bg-elevated)', border: 'var(--border)',        color: 'var(--text-disabled)' },
}

function fmtDt(d: string) { return new Date(d).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) }
function fmtDate(d: string) { return new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) }

export default function DataExportsPage() {
  const [statusFilter, setStatusFilter] = useState<ExportStatus | 'ALL'>('ALL')

  const filtered = MOCK_REQUESTS.filter((r) => statusFilter === 'ALL' || r.status === statusFilter)

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1100px' }}>
      {/* Header */}
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Compliance
        </p>
        <div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em', marginBottom: '6px' }}>
            Data Export Requests
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
            Household data portability requests (GDPR Art. 20 / CCPA). Exports include all data held for the requesting household.
          </p>
        </div>
      </div>

      {/* GDPR note */}
      <div style={{ padding: '12px 16px', marginBottom: '20px', background: 'var(--accent-subtle)', border: '1px solid var(--accent-border)', borderRadius: '8px', display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
        <Icon name="shield-check" size={14} style={{ color: 'var(--accent)', marginTop: '1px', flexShrink: 0 }} />
        <p style={{ fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.6, margin: 0 }}>
          Exports are retained for <strong style={{ color: 'var(--text)' }}>30 days</strong> (COMPLIANCE_EXPORT_TTL_DAYS) before automatic expiration.
          All export requests are audit-logged with actor and timestamp. Processing is handled asynchronously via the <code style={{ fontFamily: 'var(--font-mono)', fontSize: '11px' }}>compliance:data_export</code> job queue.
        </p>
      </div>

      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total Requests', value: MOCK_REQUESTS.length.toString() },
          { label: 'Ready',          value: MOCK_REQUESTS.filter(r => r.status === 'ready').length.toString() },
          { label: 'Processing',     value: MOCK_REQUESTS.filter(r => r.status === 'processing').length.toString() },
          { label: 'Expired',        value: MOCK_REQUESTS.filter(r => r.status === 'expired').length.toString() },
        ].map((s) => (
          <div key={s.label} className="card-elevated" style={{ padding: '14px 16px' }}>
            <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{s.label}</p>
            <p style={{ fontFamily: 'var(--font-display)', fontSize: '22px', color: 'var(--text)' }}>{s.value}</p>
          </div>
        ))}
      </div>

      {/* Status filter */}
      <div style={{ display: 'flex', gap: '4px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', padding: '3px', width: 'fit-content', marginBottom: '16px' }}>
        {(['ALL', 'pending', 'processing', 'ready', 'failed', 'expired'] as const).map((s) => (
          <button key={s} onClick={() => setStatusFilter(s)}
            style={{
              padding: '4px 14px', borderRadius: '5px', border: 'none', cursor: 'pointer',
              background: statusFilter === s ? 'var(--bg-elevated)' : 'transparent',
              color: statusFilter === s ? 'var(--text)' : 'var(--text-muted)',
              fontSize: '11px', fontWeight: statusFilter === s ? 600 : 400, fontFamily: 'var(--font-body)',
              boxShadow: statusFilter === s ? 'var(--shadow-xs)' : 'none', transition: 'all 0.15s ease',
              textTransform: 'capitalize',
            }}
          >{s === 'ALL' ? 'All' : s}</button>
        ))}
      </div>

      {/* Table */}
      <div className="card" style={{ overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ borderBottom: '1px solid var(--border)' }}>
              {['Household', 'Requested By', 'Requested', 'Status', 'Expires', 'Scope', ''].map((h) => (
                <th key={h} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 ? (
              <tr><td colSpan={7} style={{ padding: '48px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>No export requests match this filter.</td></tr>
            ) : (
              filtered.map((req) => {
                const sc = STATUS_CONFIG[req.status]
                return (
                  <tr key={req.id}
                    style={{ borderBottom: '1px solid var(--border)', transition: 'background 0.1s ease' }}
                    onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--bg-surface)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                  >
                    <td style={{ padding: '11px 14px', fontSize: '13px', fontWeight: 500, color: 'var(--text)' }}>
                      {req.householdName}
                    </td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>
                      {req.requestedBy}
                    </td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
                      {fmtDt(req.requestedAt)}
                    </td>
                    <td style={{ padding: '11px 14px' }}>
                      <span style={{ fontSize: '10px', padding: '2px 7px', borderRadius: '20px', fontFamily: 'var(--font-mono)', fontWeight: 600, letterSpacing: '0.04em', background: sc.bg, color: sc.color, border: `1px solid ${sc.border}` }}>
                        {sc.label.toUpperCase()}
                      </span>
                    </td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', fontFamily: 'var(--font-mono)', color: req.expiresAt ? 'var(--text-subtle)' : 'var(--text-muted)' }}>
                      {req.expiresAt ? fmtDate(req.expiresAt) : '—'}
                    </td>
                    <td style={{ padding: '11px 14px' }}>
                      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
                        {req.scope.slice(0, 3).map((s) => (
                          <span key={s} style={{ fontSize: '10px', padding: '1px 6px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '4px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                            {s}
                          </span>
                        ))}
                        {req.scope.length > 3 && (
                          <span style={{ fontSize: '10px', padding: '1px 6px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '4px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                            +{req.scope.length - 3}
                          </span>
                        )}
                      </div>
                    </td>
                    <td style={{ padding: '11px 14px', textAlign: 'right' }}>
                      {req.status === 'ready' && req.downloadUrl ? (
                        <a href={req.downloadUrl}
                          style={{ padding: '4px 12px', background: 'var(--success-bg)', border: '1px solid var(--success-border)', borderRadius: '5px', color: 'var(--success)', fontSize: '11px', textDecoration: 'none', fontFamily: 'var(--font-body)', display: 'inline-flex', alignItems: 'center', gap: '5px', fontWeight: 600 }}>
                          <Icon name="download" size={11} />
                          Download
                        </a>
                      ) : req.status === 'processing' ? (
                        <div style={{ display: 'flex', alignItems: 'center', gap: '6px', justifyContent: 'flex-end' }}>
                          <div style={{ width: '12px', height: '12px', borderRadius: '50%', border: '2px solid var(--border)', borderTopColor: 'var(--info)', animation: 'spin 0.8s linear infinite' }} />
                          <span style={{ fontSize: '11px', color: 'var(--info)', fontFamily: 'var(--font-mono)' }}>generating…</span>
                        </div>
                      ) : null}
                    </td>
                  </tr>
                )
              })
            )}
          </tbody>
        </table>
      </div>
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}
