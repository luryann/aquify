'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type ExportStatus = 'pending' | 'generating' | 'ready' | 'failed' | 'expired'
type ExportType = 'SD3_STANDARD' | 'SD3_EXTENDED' | 'SDIF_ROSTER'

type FileExport = {
  id: string
  exportType: ExportType
  meet: string
  meetId: string
  status: ExportStatus
  format: string
  rowCount: number | null
  fileSizeKb: number | null
  createdBy: string
  createdAt: string
  completedAt: string | null
  expiresAt: string | null
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_EXPORTS: FileExport[] = [
  {
    id: 'exp-001',
    exportType: 'SD3_STANDARD',
    meet: 'Spring Invitational',
    meetId: 'meet-002',
    status: 'ready',
    format: 'SD3',
    rowCount: 218,
    fileSizeKb: 18,
    createdBy: 'Coach Martinez',
    createdAt: '2026-03-14T10:15:00Z',
    completedAt: '2026-03-14T10:15:42Z',
    expiresAt: '2026-04-13T10:15:42Z',
  },
  {
    id: 'exp-002',
    exportType: 'SD3_EXTENDED',
    meet: 'Winter Classic',
    meetId: 'meet-001',
    status: 'ready',
    format: 'SD3',
    rowCount: 412,
    fileSizeKb: 38,
    createdBy: 'Admin Rivera',
    createdAt: '2026-03-12T14:30:00Z',
    completedAt: '2026-03-12T14:31:05Z',
    expiresAt: '2026-04-11T14:31:05Z',
  },
  {
    id: 'exp-003',
    exportType: 'SDIF_ROSTER',
    meet: 'Spring Invitational',
    meetId: 'meet-002',
    status: 'generating',
    format: 'SD3',
    rowCount: null,
    fileSizeKb: null,
    createdBy: 'Coach Kim',
    createdAt: '2026-03-16T11:45:00Z',
    completedAt: null,
    expiresAt: null,
  },
  {
    id: 'exp-004',
    exportType: 'SD3_STANDARD',
    meet: 'Polar Plunge',
    meetId: 'meet-003',
    status: 'failed',
    format: 'SD3',
    rowCount: null,
    fileSizeKb: null,
    createdBy: 'Coach Martinez',
    createdAt: '2026-03-08T09:00:00Z',
    completedAt: null,
    expiresAt: null,
  },
  {
    id: 'exp-005',
    exportType: 'SD3_STANDARD',
    meet: 'December Championships',
    meetId: 'meet-004',
    status: 'expired',
    format: 'SD3',
    rowCount: 1240,
    fileSizeKb: 112,
    createdBy: 'Admin Rivera',
    createdAt: '2025-12-22T08:00:00Z',
    completedAt: '2025-12-22T08:00:48Z',
    expiresAt: '2026-01-21T08:00:48Z',
  },
  {
    id: 'exp-006',
    exportType: 'SDIF_ROSTER',
    meet: 'Winter Classic',
    meetId: 'meet-001',
    status: 'ready',
    format: 'SD3',
    rowCount: 84,
    fileSizeKb: 7,
    createdBy: 'Admin Rivera',
    createdAt: '2026-03-01T09:30:00Z',
    completedAt: '2026-03-01T09:30:15Z',
    expiresAt: '2026-03-31T09:30:15Z',
  },
]

// ─── Config ───────────────────────────────────────────────────────────────────

const STATUS_CFG: Record<ExportStatus, { label: string; color: string; bg: string }> = {
  pending:    { label: 'Pending',    color: 'var(--text-muted)',   bg: 'var(--bg-elevated)' },
  generating: { label: 'Generating', color: 'var(--info)',         bg: 'var(--info-bg)' },
  ready:      { label: 'Ready',      color: 'var(--success)',      bg: 'var(--success-bg)' },
  failed:     { label: 'Failed',     color: 'var(--error)',        bg: 'var(--error-bg)' },
  expired:    { label: 'Expired',    color: 'var(--text-muted)',   bg: 'var(--bg-elevated)' },
}

const EXPORT_TYPE_LABELS: Record<ExportType, string> = {
  SD3_STANDARD: 'SD3 Standard',
  SD3_EXTENDED: 'SD3 Extended',
  SDIF_ROSTER:  'SDIF Roster',
}

const STATUS_FILTER_OPTIONS = [
  { value: 'all',        label: 'All Statuses' },
  { value: 'generating', label: 'Generating' },
  { value: 'ready',      label: 'Ready' },
  { value: 'failed',     label: 'Failed' },
  { value: 'expired',    label: 'Expired' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function isExpiringSoon(expiresAt: string | null) {
  if (!expiresAt) return false
  const days = (new Date(expiresAt).getTime() - Date.now()) / (1000 * 60 * 60 * 24)
  return days > 0 && days < 7
}

// ─── Table Columns ────────────────────────────────────────────────────────────

const COLUMNS: Column<FileExport>[] = [
  {
    key: 'exportType',
    label: 'Export',
    render: (r) => (
      <div>
        <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '2px' }}>{EXPORT_TYPE_LABELS[r.exportType]}</div>
        <Link href={`/meets/${r.meetId}`} style={{ fontSize: '11px', color: 'var(--text-muted)', textDecoration: 'none' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}>
          {r.meet}
        </Link>
      </div>
    ),
  },
  {
    key: 'format',
    label: 'Format',
    render: (r) => (
      <span style={{ fontSize: '11px', fontWeight: 700, padding: '2px 7px', borderRadius: '6px', background: 'var(--info-bg)', color: 'var(--info)', fontFamily: 'var(--font-mono)', letterSpacing: '0.05em' }}>
        {r.format}
      </span>
    ),
  },
  {
    key: 'status',
    label: 'Status',
    render: (r) => {
      const c = STATUS_CFG[r.status]
      return (
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', whiteSpace: 'nowrap' as const }}>
            {c.label}
          </span>
          {r.status === 'generating' && (
            <span style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--info)', display: 'inline-block', animation: 'pulse 1.5s ease-in-out infinite' }} />
          )}
        </div>
      )
    },
  },
  {
    key: 'rowCount',
    label: 'Rows',
    align: 'right',
    render: (r) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: r.rowCount != null ? 'var(--text)' : 'var(--text-muted)' }}>
        {r.rowCount != null ? r.rowCount.toLocaleString() : '—'}
      </span>
    ),
  },
  {
    key: 'fileSizeKb',
    label: 'Size',
    align: 'right',
    render: (r) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>
        {r.fileSizeKb != null ? `${r.fileSizeKb} KB` : '—'}
      </span>
    ),
  },
  {
    key: 'createdBy',
    label: 'Created By',
    render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{r.createdBy}</span>,
  },
  {
    key: 'createdAt',
    label: 'Created',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(r.createdAt)}</span>,
  },
  {
    key: 'expiresAt',
    label: 'Expires',
    render: (r) => {
      if (!r.expiresAt) return <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>—</span>
      const soon = isExpiringSoon(r.expiresAt)
      return (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: soon ? 'var(--warning)' : 'var(--text-muted)', fontWeight: soon ? 600 : 400 }}>
          {formatDate(r.expiresAt)}
          {soon && ' ⚠'}
        </span>
      )
    },
  },
  {
    key: 'id',
    label: '',
    render: (r) => {
      if (r.status === 'ready') {
        return (
          <button className="btn btn-secondary" style={{ fontSize: '12px', padding: '4px 12px', display: 'flex', alignItems: 'center', gap: '4px' }}>
            <Icon name="download" size={12} /> Download
          </button>
        )
      }
      if (r.status === 'failed') {
        return (
          <button className="btn btn-secondary" style={{ fontSize: '12px', padding: '4px 12px', display: 'flex', alignItems: 'center', gap: '4px', color: 'var(--error)' }}>
            <Icon name="refresh-cw" size={12} /> Retry
          </button>
        )
      }
      return <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>—</span>
    },
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function ExportsPage() {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')

  const total      = MOCK_EXPORTS.length
  const generating = MOCK_EXPORTS.filter((e) => e.status === 'generating').length
  const ready      = MOCK_EXPORTS.filter((e) => e.status === 'ready').length
  const failed     = MOCK_EXPORTS.filter((e) => e.status === 'failed').length

  const expiringSoon = MOCK_EXPORTS.filter((e) => isExpiringSoon(e.expiresAt)).length

  const filtered = useMemo(() => {
    return MOCK_EXPORTS.filter((exp) => {
      const q = search.toLowerCase()
      const matchSearch = !q
        || exp.meet.toLowerCase().includes(q)
        || EXPORT_TYPE_LABELS[exp.exportType].toLowerCase().includes(q)
        || exp.createdBy.toLowerCase().includes(q)
      const matchStatus = statusFilter === 'all' || exp.status === statusFilter
      return matchSearch && matchStatus
    })
  }, [search, statusFilter])

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      {/* Page header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>
            File Exports
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
            SD3 and SDIF export history — download meet entry files
          </p>
        </div>
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total',          value: total,        color: 'var(--text)' },
          { label: 'Generating',     value: generating,   color: 'var(--info)' },
          { label: 'Ready',          value: ready,        color: 'var(--success)' },
          { label: 'Failed',         value: failed,       color: 'var(--error)' },
          { label: 'Expiring Soon',  value: expiringSoon, color: expiringSoon > 0 ? 'var(--warning)' : 'var(--text-muted)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 280px' }}>
          <Icon name="search" size={14} color="var(--text-muted)" style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' } as React.CSSProperties} />
          <input
            type="text"
            placeholder="Search by meet, type, or creator…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="input"
            style={{ paddingLeft: '32px', width: '100%' }}
          />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="input" style={{ minWidth: '160px' }}>
          {STATUS_FILTER_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </div>

      {/* Result count */}
      <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '12px' }}>
        {filtered.length} of {total} exports
      </div>

      {/* Table */}
      {filtered.length === 0
        ? <EmptyState title="No exports found" description="Try adjusting your filters, or generate an export from a meet." />
        : <Table columns={COLUMNS} data={filtered} pageSize={15} emptyTitle="No exports" emptyDescription="Generate an SD3 or SDIF export from a meet page." />
      }

      {/* CSS for pulse animation */}
      <style>{`
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50%       { opacity: 0.3; }
        }
      `}</style>
    </div>
  )
}
