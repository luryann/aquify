'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type ImportStatus = 'uploaded' | 'scanning' | 'parsing' | 'matching' | 'conflicts_pending' | 'applied' | 'failed' | 'archived'
type ConflictType = 'unknown_athlete' | 'ambiguous_match' | 'bad_event_code' | 'duplicate_row'
type ConflictStatus = 'open' | 'resolved_manual' | 'resolved_auto' | 'rejected' | 'closed'
type FileType = 'SD3' | 'CL2' | 'ZIP' | 'HY3'

type ImportRow = {
  id: string
  lineNumber: number
  recordType: string
  rawData: string
  parseStatus: 'matched' | 'unmatched' | 'conflict' | 'skipped'
  swimmerName: string | null
  event: string | null
  time: string | null
}

type Conflict = {
  id: string
  lineNumber: number
  conflictType: ConflictType
  status: ConflictStatus
  rawName: string
  rawDob: string | null
  rawGender: string | null
  rawUssId: string | null
  candidates: { id: string; name: string; dob: string; ussId: string; similarity: number }[]
  resolvedTo: string | null
  resolvedBy: string | null
  resolvedAt: string | null
  notes: string | null
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_IMPORT = {
  id: 'imp-001',
  filename: 'winter_classic_results.sd3',
  fileType: 'SD3' as FileType,
  status: 'conflicts_pending' as ImportStatus,
  meet: 'Winter Classic',
  meetId: 'meet-001',
  fileSizeKb: 38,
  uploadedBy: 'Coach Martinez',
  uploadedAt: '2026-03-15T14:22:00Z',
  processedAt: '2026-03-15T14:22:45Z',
  rowsParsed: 412,
  rowsMatched: 398,
  rowsConflict: 14,
  rowsSkipped: 0,
  sdifVersion: '3',
  orgCode: 'DARE',
}

const MOCK_ROWS: ImportRow[] = [
  { id: 'row-001', lineNumber: 12, recordType: 'D0', rawData: 'D01234567Jones       Emma         F20120310', parseStatus: 'matched',   swimmerName: 'Emma Jones',     event: '100 Free (F 13-14)', time: '57.42' },
  { id: 'row-002', lineNumber: 14, recordType: 'D0', rawData: 'D0987654321Park      Jenna        F20110815', parseStatus: 'matched',   swimmerName: 'Jenna Park',     event: '200 IM (F 13-14)',  time: '2:18.44' },
  { id: 'row-003', lineNumber: 16, recordType: 'D0', rawData: 'D0UNKNOWN  Torres     Sofia        F20091201', parseStatus: 'conflict',  swimmerName: 'Sofia Torres',   event: '200 Free (F Open)', time: '2:02.18' },
  { id: 'row-004', lineNumber: 18, recordType: 'D0', rawData: 'D0111111111Henderson  Caleb        M20120504', parseStatus: 'matched',   swimmerName: 'Caleb Henderson',event: '100 Free (M Open)', time: '52.84' },
  { id: 'row-005', lineNumber: 20, recordType: 'D0', rawData: 'D0222222222Okafor     Ade          M20110920', parseStatus: 'matched',   swimmerName: 'Ade Okafor',     event: '100 Back (M Open)', time: '56.12' },
  { id: 'row-006', lineNumber: 22, recordType: 'D0', rawData: 'D0333333333Smith      J            M20130101', parseStatus: 'conflict',  swimmerName: null,             event: null,                time: null },
  { id: 'row-007', lineNumber: 24, recordType: 'D0', rawData: 'D0444444444Lee        Hannah       F20120820', parseStatus: 'matched',   swimmerName: 'Hannah Lee',     event: '100 Breast (F Open)',time: '1:12.33' },
  { id: 'row-008', lineNumber: 26, recordType: 'E0', rawData: 'E0BADCODE  FREE100                          ', parseStatus: 'conflict',  swimmerName: null,             event: null,                time: null },
]

const MOCK_CONFLICTS: Conflict[] = [
  {
    id: 'con-001',
    lineNumber: 16,
    conflictType: 'ambiguous_match',
    status: 'open',
    rawName: 'Torres, Sofia',
    rawDob: '2009-12-01',
    rawGender: 'F',
    rawUssId: 'UNKNOWN',
    candidates: [
      { id: 'sw-006', name: 'Sofia Torres',       dob: '2009-12-01', ussId: 'OR-206-TRSF', similarity: 0.97 },
      { id: 'sw-042', name: 'Sofia-Mae Torres',   dob: '2009-11-30', ussId: 'WA-042-TRSM', similarity: 0.74 },
    ],
    resolvedTo: null,
    resolvedBy: null,
    resolvedAt: null,
    notes: null,
  },
  {
    id: 'con-002',
    lineNumber: 22,
    conflictType: 'unknown_athlete',
    status: 'open',
    rawName: 'Smith, J',
    rawDob: '2013-01-01',
    rawGender: 'M',
    rawUssId: '333333333',
    candidates: [],
    resolvedTo: null,
    resolvedBy: null,
    resolvedAt: null,
    notes: null,
  },
  {
    id: 'con-003',
    lineNumber: 26,
    conflictType: 'bad_event_code',
    status: 'open',
    rawName: null as unknown as string,
    rawDob: null,
    rawGender: null,
    rawUssId: null,
    candidates: [],
    resolvedTo: null,
    resolvedBy: null,
    resolvedAt: null,
    notes: 'Unrecognized SDIF event code "BADCODE" on record type E0',
  },
  {
    id: 'con-004',
    lineNumber: 58,
    conflictType: 'ambiguous_match',
    status: 'resolved_manual',
    rawName: 'Lee, Hannah',
    rawDob: '2012-08-20',
    rawGender: 'F',
    rawUssId: '444444444',
    candidates: [
      { id: 'sw-031', name: 'Hannah Lee',   dob: '2012-08-20', ussId: 'OR-031-LEHF', similarity: 0.99 },
      { id: 'sw-078', name: 'Hannah Grace Lee', dob: '2012-08-21', ussId: 'OR-078-LEGH', similarity: 0.88 },
    ],
    resolvedTo: 'sw-031',
    resolvedBy: 'Coach Martinez',
    resolvedAt: '2026-03-15T15:10:00Z',
    notes: 'Confirmed via USS# cross-check',
  },
]

// ─── Config ───────────────────────────────────────────────────────────────────

const IMPORT_STATUS_CFG: Record<ImportStatus, { label: string; color: string; bg: string }> = {
  uploaded:          { label: 'Uploaded',          color: 'var(--text-muted)',  bg: 'var(--bg-elevated)' },
  scanning:          { label: 'Scanning',           color: 'var(--info)',        bg: 'var(--info-bg)' },
  parsing:           { label: 'Parsing',            color: 'var(--info)',        bg: 'var(--info-bg)' },
  matching:          { label: 'Matching',           color: 'var(--warning)',     bg: 'var(--warning-bg)' },
  conflicts_pending: { label: 'Conflicts Pending',  color: 'var(--warning)',     bg: 'var(--warning-bg)' },
  applied:           { label: 'Applied',            color: 'var(--success)',     bg: 'var(--success-bg)' },
  failed:            { label: 'Failed',             color: 'var(--error)',       bg: 'var(--error-bg)' },
  archived:          { label: 'Archived',           color: 'var(--text-muted)',  bg: 'var(--bg-elevated)' },
}

const CONFLICT_TYPE_CFG: Record<ConflictType, { label: string; color: string; bg: string }> = {
  unknown_athlete: { label: 'Unknown Athlete',  color: 'var(--error)',   bg: 'var(--error-bg)' },
  ambiguous_match: { label: 'Ambiguous Match',  color: 'var(--warning)', bg: 'var(--warning-bg)' },
  bad_event_code:  { label: 'Bad Event Code',   color: 'var(--error)',   bg: 'var(--error-bg)' },
  duplicate_row:   { label: 'Duplicate Row',    color: 'var(--info)',    bg: 'var(--info-bg)' },
}

const CONFLICT_STATUS_CFG: Record<ConflictStatus, { label: string; color: string; bg: string }> = {
  open:             { label: 'Open',            color: 'var(--warning)',     bg: 'var(--warning-bg)' },
  resolved_manual:  { label: 'Resolved',        color: 'var(--success)',     bg: 'var(--success-bg)' },
  resolved_auto:    { label: 'Auto-Resolved',   color: 'var(--success)',     bg: 'var(--success-bg)' },
  rejected:         { label: 'Rejected',        color: 'var(--text-muted)',  bg: 'var(--bg-elevated)' },
  closed:           { label: 'Closed',          color: 'var(--text-muted)',  bg: 'var(--bg-elevated)' },
}

const ROW_PARSE_CFG: Record<ImportRow['parseStatus'], { color: string }> = {
  matched:   { color: 'var(--success)' },
  unmatched: { color: 'var(--text-muted)' },
  conflict:  { color: 'var(--warning)' },
  skipped:   { color: 'var(--text-muted)' },
}

// ─── Row Columns ──────────────────────────────────────────────────────────────

const ROW_COLS: Column<ImportRow>[] = [
  {
    key: 'lineNumber',
    label: 'Line',
    align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)' }}>{r.lineNumber}</span>,
  },
  {
    key: 'recordType',
    label: 'Rec',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', padding: '1px 6px', borderRadius: '4px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', color: 'var(--text-subtle)' }}>{r.recordType}</span>,
  },
  {
    key: 'parseStatus',
    label: 'Status',
    render: (r) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', fontWeight: 600, color: ROW_PARSE_CFG[r.parseStatus].color }}>
        {r.parseStatus}
      </span>
    ),
  },
  {
    key: 'swimmerName',
    label: 'Swimmer',
    render: (r) => <span style={{ fontSize: '13px', color: r.swimmerName ? 'var(--text)' : 'var(--text-muted)', fontWeight: r.swimmerName ? 600 : 400 }}>{r.swimmerName ?? '—'}</span>,
  },
  {
    key: 'event',
    label: 'Event',
    render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{r.event ?? '—'}</span>,
  },
  {
    key: 'time',
    label: 'Time',
    align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: r.time ? 'var(--accent)' : 'var(--text-muted)' }}>{r.time ?? '—'}</span>,
  },
  {
    key: 'rawData',
    label: 'Raw',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '10px', color: 'var(--text-muted)', overflow: 'hidden', display: 'block', maxWidth: '260px', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{r.rawData}</span>,
  },
]

// ─── Conflict Card ────────────────────────────────────────────────────────────

function ConflictCard({ conflict, onResolve, onReject }: {
  conflict: Conflict
  onResolve: (conflictId: string, swimmerIdOrAction: string) => void
  onReject: (conflictId: string) => void
}) {
  const typeCfg    = CONFLICT_TYPE_CFG[conflict.conflictType]
  const statusCfg  = CONFLICT_STATUS_CFG[conflict.status]
  const isOpen     = conflict.status === 'open'

  return (
    <div
      className="card"
      style={{
        padding: '18px 20px',
        borderLeft: `3px solid ${isOpen ? typeCfg.color : statusCfg.color}`,
        opacity: isOpen ? 1 : 0.65,
      }}
    >
      {/* Header row */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '14px', gap: '12px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', fontWeight: 700, padding: '2px 8px', borderRadius: '10px', background: typeCfg.bg, color: typeCfg.color }}>
            {typeCfg.label}
          </span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: statusCfg.bg, color: statusCfg.color }}>
            {statusCfg.label}
          </span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)', padding: '2px 6px', borderRadius: '4px', background: 'var(--bg-elevated)', border: '1px solid var(--border)' }}>
            Line {conflict.lineNumber}
          </span>
        </div>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)', flexShrink: 0 }}>{conflict.id}</span>
      </div>

      {/* Raw data */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))', gap: '10px', marginBottom: '14px' }}>
        {conflict.rawName && (
          <div>
            <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '2px' }}>Name in file</div>
            <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{conflict.rawName}</div>
          </div>
        )}
        {conflict.rawDob && (
          <div>
            <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '2px' }}>DOB</div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{conflict.rawDob}</div>
          </div>
        )}
        {conflict.rawGender && (
          <div>
            <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '2px' }}>Gender</div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{conflict.rawGender}</div>
          </div>
        )}
        {conflict.rawUssId && (
          <div>
            <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '2px' }}>USS# in file</div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{conflict.rawUssId}</div>
          </div>
        )}
      </div>

      {/* Notes */}
      {conflict.notes && (
        <div style={{ padding: '8px 12px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)', marginBottom: '14px' }}>
          <span style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>{conflict.notes}</span>
        </div>
      )}

      {/* Candidates */}
      {conflict.candidates.length > 0 && (
        <div style={{ marginBottom: '14px' }}>
          <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
            Candidate Matches
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
            {conflict.candidates.map((c) => (
              <div
                key={c.id}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '10px 12px',
                  background: conflict.resolvedTo === c.id ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                  borderRadius: 'var(--r-md)',
                  border: `1px solid ${conflict.resolvedTo === c.id ? 'var(--accent)' : 'var(--border)'}`,
                  gap: '12px',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: '16px', flexWrap: 'wrap', flex: 1 }}>
                  <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{c.name}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)' }}>DOB {c.dob}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)' }}>USS# {c.ussId}</span>
                  <span style={{
                    fontFamily: 'var(--font-mono)', fontSize: '11px', fontWeight: 700,
                    color: c.similarity >= 0.9 ? 'var(--success)' : c.similarity >= 0.75 ? 'var(--warning)' : 'var(--error)',
                  }}>
                    {Math.round(c.similarity * 100)}% match
                  </span>
                </div>
                {isOpen && (
                  <button
                    className="btn btn-secondary"
                    style={{ fontSize: '12px', padding: '4px 12px', flexShrink: 0 }}
                    onClick={() => onResolve(conflict.id, c.id)}
                  >
                    Select
                  </button>
                )}
                {!isOpen && conflict.resolvedTo === c.id && (
                  <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--success)' }}>✓ Resolved</span>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Resolution info */}
      {!isOpen && conflict.resolvedBy && (
        <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', borderTop: '1px solid var(--border)', paddingTop: '10px', marginTop: '4px' }}>
          Resolved by <strong>{conflict.resolvedBy}</strong> on {new Date(conflict.resolvedAt!).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
        </div>
      )}

      {/* Actions */}
      {isOpen && conflict.candidates.length === 0 && (
        <div style={{ display: 'flex', gap: '8px', marginTop: '4px' }}>
          <button
            className="btn btn-secondary"
            style={{ fontSize: '12px', display: 'flex', alignItems: 'center', gap: '5px' }}
            onClick={() => onReject(conflict.id)}
          >
            <Icon name="x" size={12} /> Reject Row
          </button>
        </div>
      )}
      {isOpen && conflict.candidates.length > 0 && (
        <div style={{ display: 'flex', gap: '8px', marginTop: '4px' }}>
          <button
            className="btn btn-secondary"
            style={{ fontSize: '12px', display: 'flex', alignItems: 'center', gap: '5px', color: 'var(--error)' }}
            onClick={() => onReject(conflict.id)}
          >
            <Icon name="x" size={12} /> Reject Row
          </button>
        </div>
      )}
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function ImportDetailPage() {
  const [activeTab, setActiveTab] = useState('overview')
  const [conflicts, setConflicts] = useState<Conflict[]>(MOCK_CONFLICTS)
  const [conflictFilter, setConflictFilter] = useState<'all' | 'open' | 'resolved'>('all')

  const imp = MOCK_IMPORT
  const statusCfg = IMPORT_STATUS_CFG[imp.status]

  const openConflicts     = conflicts.filter((c) => c.status === 'open').length
  const resolvedConflicts = conflicts.filter((c) => ['resolved_manual','resolved_auto'].includes(c.status)).length
  const allResolved       = openConflicts === 0

  function handleResolve(conflictId: string, swimmerId: string) {
    setConflicts((prev) =>
      prev.map((c) =>
        c.id === conflictId
          ? { ...c, status: 'resolved_manual', resolvedTo: swimmerId, resolvedBy: 'Coach Martinez', resolvedAt: new Date().toISOString() }
          : c
      )
    )
  }

  function handleReject(conflictId: string) {
    setConflicts((prev) =>
      prev.map((c) =>
        c.id === conflictId
          ? { ...c, status: 'rejected', resolvedBy: 'Coach Martinez', resolvedAt: new Date().toISOString() }
          : c
      )
    )
  }

  const filteredConflicts = conflicts.filter((c) => {
    if (conflictFilter === 'open')     return c.status === 'open'
    if (conflictFilter === 'resolved') return ['resolved_manual','resolved_auto','rejected'].includes(c.status)
    return true
  })

  const tabs = [
    { id: 'overview',   label: 'Overview' },
    { id: 'rows',       label: 'Rows',      count: imp.rowsParsed },
    { id: 'conflicts',  label: 'Conflicts', count: openConflicts },
    { id: 'report',     label: 'Report' },
  ]

  const FILE_TYPE_CFG: Record<string, { color: string; bg: string }> = {
    SD3: { color: 'var(--info)',    bg: 'var(--info-bg)' },
    CL2: { color: 'var(--accent)',  bg: 'var(--accent-subtle)' },
    ZIP: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
    HY3: { color: 'var(--success)', bg: 'var(--success-bg)' },
  }
  const ftCfg = FILE_TYPE_CFG[imp.fileType]

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      {/* Back link */}
      <Link href="/imports" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--text)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-subtle)')}>
        <Icon name="chevron-left" size={14} /> Imports
      </Link>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '6px' }}>
            <div style={{ width: '32px', height: '32px', borderRadius: 'var(--r-md)', background: ftCfg.bg, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Icon name="file-text" size={15} color={ftCfg.color} />
            </div>
            <h1 style={{ fontSize: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--text)', lineHeight: 1.2 }}>
              {imp.filename}
            </h1>
            <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: statusCfg.bg, color: statusCfg.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em', whiteSpace: 'nowrap' as const }}>
              {statusCfg.label}
            </span>
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', display: 'flex', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
            <span>{imp.fileType}</span>
            <span>·</span>
            {imp.meet && (
              <>
                <Link href={`/meets/${imp.meetId}`} style={{ color: 'var(--text-muted)', textDecoration: 'none' }}
                  onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
                  onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}>
                  {imp.meet}
                </Link>
                <span>·</span>
              </>
            )}
            <span>Uploaded by {imp.uploadedBy}</span>
            <span>·</span>
            <span>{new Date(imp.uploadedAt).toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })}</span>
          </div>
        </div>
        <div style={{ display: 'flex', gap: '8px', flexShrink: 0 }}>
          <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Icon name="download" size={13} /> Download Report
          </button>
          {imp.status === 'conflicts_pending' && allResolved && (
            <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Icon name="check-circle" size={13} /> Apply Import
            </button>
          )}
        </div>
      </div>

      {/* Quick stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'Rows Parsed',  value: imp.rowsParsed,   color: 'var(--text)' },
          { label: 'Matched',      value: imp.rowsMatched,  color: 'var(--success)' },
          { label: 'Conflicts',    value: imp.rowsConflict, color: 'var(--warning)' },
          { label: 'Open',         value: openConflicts,    color: openConflicts > 0 ? 'var(--warning)' : 'var(--text-muted)' },
          { label: 'Resolved',     value: resolvedConflicts,color: resolvedConflicts > 0 ? 'var(--success)' : 'var(--text-muted)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {/* Overview */}
      {activeTab === 'overview' && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>File Details</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[
                { label: 'Import ID',     value: imp.id },
                { label: 'Filename',      value: imp.filename },
                { label: 'Format',        value: imp.fileType },
                { label: 'SDIF Version',  value: imp.sdifVersion },
                { label: 'Org Code',      value: imp.orgCode },
                { label: 'File Size',     value: `${imp.fileSizeKb} KB` },
                { label: 'Uploaded By',   value: imp.uploadedBy },
                { label: 'Uploaded At',   value: new Date(imp.uploadedAt).toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' }) },
                { label: 'Processed At',  value: new Date(imp.processedAt).toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' }) },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600, textAlign: 'right', maxWidth: '60%', wordBreak: 'break-all' }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
            <div className="card" style={{ padding: '20px' }}>
              <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Parse Summary</h3>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                {[
                  { label: 'Total Rows',   value: imp.rowsParsed,   color: 'var(--text)' },
                  { label: 'Matched',      value: imp.rowsMatched,  color: 'var(--success)' },
                  { label: 'Conflicts',    value: imp.rowsConflict, color: 'var(--warning)' },
                  { label: 'Skipped',      value: imp.rowsSkipped,  color: 'var(--text-muted)' },
                ].map((row) => (
                  <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <span style={{ fontSize: '13px', color: 'var(--text-muted)' }}>{row.label}</span>
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: '15px', fontWeight: 700, color: row.color }}>{row.value}</span>
                  </div>
                ))}
                {/* Match rate bar */}
                <div style={{ marginTop: '8px' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px', fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                    <span>Match rate</span>
                    <span>{Math.round((imp.rowsMatched / imp.rowsParsed) * 100)}%</span>
                  </div>
                  <div style={{ height: '4px', borderRadius: '2px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
                    <div style={{ height: '100%', width: `${(imp.rowsMatched / imp.rowsParsed) * 100}%`, background: 'var(--success)', borderRadius: '2px', transition: 'width 0.6s var(--ease-out)' }} />
                  </div>
                </div>
              </div>
            </div>

            {imp.status === 'conflicts_pending' && (
              <div className="card" style={{ padding: '20px', borderLeft: '3px solid var(--warning)' }}>
                <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--warning)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '10px', fontFamily: 'var(--font-mono)' }}>Action Required</h3>
                <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.6, marginBottom: '14px' }}>
                  {openConflicts} conflict{openConflicts !== 1 ? 's' : ''} require manual resolution before this import can be applied.
                </p>
                <button className="btn btn-primary" style={{ fontSize: '13px', display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setActiveTab('conflicts')}>
                  <Icon name="alert-circle" size={13} /> Review Conflicts
                </button>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Rows */}
      {activeTab === 'rows' && (
        <div>
          <div style={{ fontSize: '13px', color: 'var(--text-subtle)', marginBottom: '12px' }}>
            Showing {MOCK_ROWS.length} of {imp.rowsParsed} rows — sample display (full row data requires database query)
          </div>
          <Table columns={ROW_COLS} data={MOCK_ROWS} pageSize={20} emptyTitle="No rows" emptyDescription="No rows parsed from this file." />
        </div>
      )}

      {/* Conflicts */}
      {activeTab === 'conflicts' && (
        <div>
          {/* Conflict filter tabs */}
          <div style={{ display: 'flex', gap: '8px', marginBottom: '20px' }}>
            {([['all','All'], ['open','Open'], ['resolved','Resolved']] as const).map(([v, l]) => (
              <button
                key={v}
                onClick={() => setConflictFilter(v)}
                style={{
                  padding: '5px 14px',
                  borderRadius: 'var(--r-md)',
                  border: '1px solid var(--border)',
                  background: conflictFilter === v ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                  color: conflictFilter === v ? 'var(--accent)' : 'var(--text-subtle)',
                  fontSize: '12px',
                  fontWeight: conflictFilter === v ? 600 : 400,
                  cursor: 'pointer',
                  transition: 'all var(--dur-fast)',
                }}
              >
                {l}
                {v === 'open' && openConflicts > 0 && (
                  <span style={{ marginLeft: '6px', fontFamily: 'var(--font-mono)', fontSize: '11px', fontWeight: 700, color: 'var(--warning)' }}>{openConflicts}</span>
                )}
              </button>
            ))}
            <div style={{ flex: 1 }} />
            {allResolved && imp.status === 'conflicts_pending' && (
              <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px' }}>
                <Icon name="check-circle" size={12} /> Apply Import
              </button>
            )}
          </div>

          {filteredConflicts.length === 0
            ? <EmptyState title="No conflicts" description={conflictFilter === 'open' ? 'All conflicts have been resolved.' : 'No conflicts found.'} />
            : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {filteredConflicts.map((c) => (
                  <ConflictCard key={c.id} conflict={c} onResolve={handleResolve} onReject={handleReject} />
                ))}
              </div>
            )
          }
        </div>
      )}

      {/* Report */}
      {activeTab === 'report' && (
        <div className="card" style={{ padding: '20px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)' }}>Import Report</h3>
            <button className="btn btn-secondary" style={{ fontSize: '12px', display: 'flex', alignItems: 'center', gap: '5px' }}>
              <Icon name="download" size={12} /> Download .txt
            </button>
          </div>
          <pre style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.8, overflowX: 'auto', background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', padding: '16px', border: '1px solid var(--border)' }}>
{`DARE Platform Import Report
Generated: ${new Date().toISOString()}
File: ${imp.filename}
Format: ${imp.fileType} (SDIF v${imp.sdifVersion})
Meet: ${imp.meet ?? 'N/A'}
Org: ${imp.orgCode}

--- SUMMARY ---
Total rows parsed:  ${imp.rowsParsed.toString().padStart(6)}
Matched:            ${imp.rowsMatched.toString().padStart(6)}
Conflicts:          ${imp.rowsConflict.toString().padStart(6)}
Skipped:            ${imp.rowsSkipped.toString().padStart(6)}
Match rate:         ${Math.round((imp.rowsMatched / imp.rowsParsed) * 100)}%

--- CONFLICTS (sorted by line number) ---
${conflicts.map((c) =>
  `  [${c.lineNumber.toString().padStart(4)}]  ${c.conflictType.padEnd(20)}  ${c.status.padEnd(18)}  ${c.rawName ?? '(no name)'}`
).join('\n')}

--- END OF REPORT ---`}
          </pre>
        </div>
      )}

    </div>
  )
}
