'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type ImportStatus =
  | 'uploaded'
  | 'scanning'
  | 'parsing'
  | 'matching'
  | 'conflicts_pending'
  | 'applied'
  | 'failed'
  | 'archived'

type FileType = 'SD3' | 'CL2' | 'ZIP' | 'HY3'

type FileImport = {
  id: string
  filename: string
  fileType: FileType
  status: ImportStatus
  rowsParsed: number
  rowsMatched: number
  conflictCount: number
  uploadedBy: string
  uploadedAt: string
  meet: string | null
  fileSizeKb: number
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_IMPORTS: FileImport[] = [
  {
    id: 'imp-001',
    filename: 'winter_classic_results.sd3',
    fileType: 'SD3',
    status: 'conflicts_pending',
    rowsParsed: 412,
    rowsMatched: 398,
    conflictCount: 14,
    uploadedBy: 'Coach Martinez',
    uploadedAt: '2026-03-15T14:22:00Z',
    meet: 'Winter Classic',
    fileSizeKb: 38,
  },
  {
    id: 'imp-002',
    filename: 'spring_invite_entries.sd3',
    fileType: 'SD3',
    status: 'applied',
    rowsParsed: 284,
    rowsMatched: 284,
    conflictCount: 0,
    uploadedBy: 'Admin Rivera',
    uploadedAt: '2026-03-12T09:05:00Z',
    meet: 'Spring Invitational',
    fileSizeKb: 21,
  },
  {
    id: 'imp-003',
    filename: 'feb_results_hy3.zip',
    fileType: 'HY3',
    status: 'applied',
    rowsParsed: 731,
    rowsMatched: 728,
    conflictCount: 3,
    uploadedBy: 'Coach Martinez',
    uploadedAt: '2026-03-08T16:44:00Z',
    meet: 'Polar Plunge',
    fileSizeKb: 95,
  },
  {
    id: 'imp-004',
    filename: 'club_roster_export.cl2',
    fileType: 'CL2',
    status: 'parsing',
    rowsParsed: 0,
    rowsMatched: 0,
    conflictCount: 0,
    uploadedBy: 'Admin Rivera',
    uploadedAt: '2026-03-16T11:30:00Z',
    meet: null,
    fileSizeKb: 14,
  },
  {
    id: 'imp-005',
    filename: 'jan_invitational.sd3',
    fileType: 'SD3',
    status: 'failed',
    rowsParsed: 0,
    rowsMatched: 0,
    conflictCount: 0,
    uploadedBy: 'Coach Kim',
    uploadedAt: '2026-03-02T08:15:00Z',
    meet: 'January Invitational',
    fileSizeKb: 5,
  },
  {
    id: 'imp-006',
    filename: 'dec_championships_all.zip',
    fileType: 'ZIP',
    status: 'archived',
    rowsParsed: 1240,
    rowsMatched: 1235,
    conflictCount: 5,
    uploadedBy: 'Admin Rivera',
    uploadedAt: '2025-12-20T09:00:00Z',
    meet: 'December Championships',
    fileSizeKb: 148,
  },
  {
    id: 'imp-007',
    filename: 'oct_time_trial.sd3',
    fileType: 'SD3',
    status: 'archived',
    rowsParsed: 180,
    rowsMatched: 180,
    conflictCount: 0,
    uploadedBy: 'Coach Kim',
    uploadedAt: '2025-10-18T14:00:00Z',
    meet: 'October Time Trial',
    fileSizeKb: 12,
  },
]

// ─── Config ───────────────────────────────────────────────────────────────────

const STATUS_CFG: Record<ImportStatus, { label: string; color: string; bg: string }> = {
  uploaded:          { label: 'Uploaded',          color: 'var(--text-muted)',   bg: 'var(--bg-elevated)' },
  scanning:          { label: 'Scanning',           color: 'var(--info)',         bg: 'var(--info-bg)' },
  parsing:           { label: 'Parsing',            color: 'var(--info)',         bg: 'var(--info-bg)' },
  matching:          { label: 'Matching',           color: 'var(--warning)',      bg: 'var(--warning-bg)' },
  conflicts_pending: { label: 'Conflicts Pending',  color: 'var(--warning)',      bg: 'var(--warning-bg)' },
  applied:           { label: 'Applied',            color: 'var(--success)',      bg: 'var(--success-bg)' },
  failed:            { label: 'Failed',             color: 'var(--error)',        bg: 'var(--error-bg)' },
  archived:          { label: 'Archived',           color: 'var(--text-muted)',   bg: 'var(--bg-elevated)' },
}

const FILE_TYPE_CFG: Record<FileType, { color: string; bg: string }> = {
  SD3: { color: 'var(--info)',    bg: 'var(--info-bg)' },
  CL2: { color: 'var(--accent)',  bg: 'var(--accent-subtle)' },
  ZIP: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
  HY3: { color: 'var(--success)', bg: 'var(--success-bg)' },
}

const STATUS_FILTER_OPTIONS: { value: string; label: string }[] = [
  { value: 'all',               label: 'All Statuses' },
  { value: 'conflicts_pending', label: 'Conflicts Pending' },
  { value: 'parsing',           label: 'Processing' },
  { value: 'applied',           label: 'Applied' },
  { value: 'failed',            label: 'Failed' },
  { value: 'archived',          label: 'Archived' },
]

const TYPE_FILTER_OPTIONS: { value: string; label: string }[] = [
  { value: 'all', label: 'All Types' },
  { value: 'SD3', label: 'SD3' },
  { value: 'CL2', label: 'CL2' },
  { value: 'ZIP', label: 'ZIP' },
  { value: 'HY3', label: 'HY3' },
]

// ─── Table Columns ────────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: ImportStatus }) {
  const c = STATUS_CFG[status]
  return (
    <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em', whiteSpace: 'nowrap' as const }}>
      {c.label}
    </span>
  )
}

function FileTypeBadge({ type }: { type: FileType }) {
  const c = FILE_TYPE_CFG[type]
  return (
    <span style={{ fontSize: '11px', fontWeight: 700, padding: '2px 7px', borderRadius: '6px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.05em' }}>
      {type}
    </span>
  )
}

function formatDate(iso: string) {
  const d = new Date(iso)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

const COLUMNS: Column<FileImport>[] = [
  {
    key: 'filename',
    label: 'File',
    render: (r) => (
      <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
        <div style={{ width: '30px', height: '30px', borderRadius: 'var(--r-md)', background: FILE_TYPE_CFG[r.fileType].bg, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
          <Icon name="file-text" size={14} color={FILE_TYPE_CFG[r.fileType].color} />
        </div>
        <div>
          <Link
            href={`/imports/${r.id}`}
            style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none', fontFamily: 'var(--font-mono)' }}
            onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
            onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}
          >
            {r.filename}
          </Link>
          {r.meet && (
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '1px' }}>{r.meet}</div>
          )}
        </div>
      </div>
    ),
  },
  {
    key: 'fileType',
    label: 'Type',
    render: (r) => <FileTypeBadge type={r.fileType} />,
  },
  {
    key: 'status',
    label: 'Status',
    render: (r) => <StatusBadge status={r.status} />,
  },
  {
    key: 'rowsParsed',
    label: 'Rows',
    align: 'right',
    render: (r) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: r.rowsParsed > 0 ? 'var(--text)' : 'var(--text-muted)' }}>
        {r.rowsParsed > 0 ? `${r.rowsMatched.toLocaleString()} / ${r.rowsParsed.toLocaleString()}` : '—'}
      </span>
    ),
  },
  {
    key: 'conflictCount',
    label: 'Conflicts',
    align: 'right',
    render: (r) => (
      r.conflictCount > 0
        ? <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', fontWeight: 700, color: 'var(--warning)', background: 'var(--warning-bg)', padding: '2px 8px', borderRadius: '10px' }}>{r.conflictCount}</span>
        : <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>—</span>
    ),
  },
  {
    key: 'uploadedBy',
    label: 'Uploaded By',
    render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{r.uploadedBy}</span>,
  },
  {
    key: 'uploadedAt',
    label: 'Date',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(r.uploadedAt)}</span>,
  },
  {
    key: 'id',
    label: '',
    render: (r) => (
      <Link href={`/imports/${r.id}`} style={{ fontSize: '12px', color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '3px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}>
        {r.status === 'conflicts_pending' ? 'Resolve' : 'View'}
        <Icon name="chevron-right" size={12} />
      </Link>
    ),
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function ImportsPage() {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [typeFilter, setTypeFilter] = useState('all')

  const total        = MOCK_IMPORTS.length
  const processing   = MOCK_IMPORTS.filter((i) => ['scanning','parsing','matching'].includes(i.status)).length
  const conflicts    = MOCK_IMPORTS.filter((i) => i.status === 'conflicts_pending').length
  const applied      = MOCK_IMPORTS.filter((i) => i.status === 'applied').length
  const failed       = MOCK_IMPORTS.filter((i) => i.status === 'failed').length

  const filtered = useMemo(() => {
    return MOCK_IMPORTS.filter((imp) => {
      const q = search.toLowerCase()
      const matchSearch = !q || imp.filename.toLowerCase().includes(q) || (imp.meet?.toLowerCase().includes(q) ?? false) || imp.uploadedBy.toLowerCase().includes(q)
      const matchStatus = statusFilter === 'all' || imp.status === statusFilter
      const matchType   = typeFilter === 'all'   || imp.fileType === typeFilter
      return matchSearch && matchStatus && matchType
    })
  }, [search, statusFilter, typeFilter])

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      {/* Page header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>
            File Imports
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
            SD3, CL2, ZIP, and HY3 result &amp; roster files
          </p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <Icon name="upload-cloud" size={13} /> Upload File
        </button>
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total',      value: total,      color: 'var(--text)' },
          { label: 'Processing', value: processing, color: 'var(--info)' },
          { label: 'Conflicts',  value: conflicts,  color: 'var(--warning)' },
          { label: 'Applied',    value: applied,    color: 'var(--success)' },
          { label: 'Failed',     value: failed,     color: 'var(--error)' },
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
            placeholder="Search by filename, meet, or uploader…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="input"
            style={{ paddingLeft: '32px', width: '100%' }}
          />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="input" style={{ minWidth: '180px' }}>
          {STATUS_FILTER_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <select value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)} className="input" style={{ minWidth: '130px' }}>
          {TYPE_FILTER_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </div>

      {/* Result count */}
      <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '12px' }}>
        {filtered.length} of {total} imports
      </div>

      {/* Table */}
      {filtered.length === 0
        ? <EmptyState title="No imports found" description="Try adjusting your filters or upload a new file." />
        : <Table columns={COLUMNS} data={filtered} pageSize={15} emptyTitle="No imports" emptyDescription="Upload an SD3, CL2, ZIP, or HY3 file to get started." />
      }
    </div>
  )
}
