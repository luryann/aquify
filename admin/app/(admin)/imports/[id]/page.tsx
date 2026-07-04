'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import {
  fetchImport,
  fetchImportConflicts,
  fetchImportReport,
  fetchMeets,
  type ImportConflictRecord,
  type ImportRecord,
  type ImportReportRecord,
} from '@/lib/adminData'

type ImportErrorRow = {
  row: string
  code: string
  message: string
}

function formatDateTime(value?: string | null) {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function formatBytes(value?: number) {
  if (!value) return '—'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / (1024 * 1024)).toFixed(1)} MB`
}

function StatusBadge({ status }: { status: string }) {
  const cfg: Record<string, { color: string; bg: string }> = {
    uploaded: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    scanning: { color: 'var(--info)', bg: 'var(--info-bg)' },
    parsing: { color: 'var(--info)', bg: 'var(--info-bg)' },
    matching: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
    conflicts_pending: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
    applied: { color: 'var(--success)', bg: 'var(--success-bg)' },
    failed: { color: 'var(--error)', bg: 'var(--error-bg)' },
    archived: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    open: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
    resolved_manual: { color: 'var(--success)', bg: 'var(--success-bg)' },
    resolved_auto: { color: 'var(--success)', bg: 'var(--success-bg)' },
    rejected: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    closed: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  }
  const colors = cfg[status] ?? { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' }
  return <span className="badge" style={{ background: colors.bg, color: colors.color }}>{status.replaceAll('_', ' ')}</span>
}

const CONFLICT_COLS: Column<ImportConflictRecord>[] = [
  { key: 'row_number', label: 'Row', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '11px', color: 'var(--text-muted)' }}>{row.row_number ?? '—'}</span> },
  { key: 'conflict_type', label: 'Type', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '11px', color: 'var(--text)' }}>{row.conflict_type}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
  { key: 'raw_data', label: 'Raw Data', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '10px', color: 'var(--text-muted)' }}>{row.raw_data || '—'}</span> },
  { key: 'resolved_swimmer_id', label: 'Resolved To', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '11px', color: 'var(--text-subtle)' }}>{row.resolved_swimmer_id || '—'}</span> },
]

const ERROR_COLS: Column<ImportErrorRow>[] = [
  { key: 'row', label: 'Row', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '11px', color: 'var(--text-muted)' }}>{row.row}</span> },
  { key: 'code', label: 'Code', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '11px', color: 'var(--text)' }}>{row.code}</span> },
  { key: 'message', label: 'Message', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.message}</span> },
]

export default function ImportDetailPage() {
  const params = useParams<{ id: string }>()
  const importId = typeof params?.id === 'string' ? params.id : ''
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [activeTab, setActiveTab] = useState('overview')
  const [fileImport, setFileImport] = useState<ImportRecord | null>(null)
  const [report, setReport] = useState<ImportReportRecord | null>(null)
  const [conflicts, setConflicts] = useState<ImportConflictRecord[]>([])
  const [meetName, setMeetName] = useState<string>('—')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!session?.user.orgId || !importId) {
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        setError(null)
        const [nextImport, nextConflicts, nextReport, meets] = await Promise.all([
          fetchImport(api, session.user.orgId, importId),
          fetchImportConflicts(api, session.user.orgId, importId),
          fetchImportReport(api, session.user.orgId, importId),
          fetchMeets(api, session.user.orgId),
        ])

        if (cancelled) return

        setFileImport(nextImport)
        setConflicts(nextConflicts)
        setReport(nextReport)
        const meet = meets.find((candidate) => candidate.id === nextImport.meet_id)
        setMeetName(meet?.name || '—')
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load import details.')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, importId, session?.user.orgId])

  const tabs = useMemo(() => [
    { id: 'overview', label: 'Overview' },
    { id: 'conflicts', label: 'Conflicts', count: conflicts.length },
    { id: 'errors', label: 'Report Errors', count: report?.errors?.length ?? 0 },
  ], [conflicts.length, report?.errors?.length])

  const errorRows = useMemo(() => (report?.errors ?? []).map((entry) => ({
    row: entry.row_number?.toString() || '—',
    code: entry.error_code,
    message: entry.error_message,
  })), [report])

  if (loading) {
    return <div style={{ padding: '24px', maxWidth: '1200px' }}><EmptyState icon="upload" title="Loading import" description="Fetching live import detail data from the API." /></div>
  }

  if (error || !fileImport) {
    return <div style={{ padding: '24px', maxWidth: '1200px' }}><EmptyState icon="alert-circle" title="Unable to load import" description={error ?? 'Import not found.'} /></div>
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <Link href="/imports" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}>
        <Icon name="chevron-left" size={14} /> Imports
      </Link>

      <div className="page-header">
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
            <h1 className="page-title">{fileImport.filename}</h1>
            <StatusBadge status={fileImport.status} />
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
            {fileImport.id} · {fileImport.file_type.toUpperCase()} · Meet: {meetName}
          </div>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'Rows', value: `${report?.total_rows ?? fileImport.row_count}`, color: 'var(--text)' },
          { label: 'Matched', value: `${report?.matched_rows ?? Math.max((report?.total_rows ?? fileImport.row_count) - fileImport.error_count - fileImport.conflict_count, 0)}`, color: 'var(--success)' },
          { label: 'Conflicts', value: `${conflicts.length || fileImport.conflict_count}`, color: 'var(--warning)' },
          { label: 'File Size', value: formatBytes(fileImport.declared_size_bytes), color: 'var(--text-subtle)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-body)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '18px', fontFamily: 'var(--font-body)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'overview' ? (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-body)' }}>Import Metadata</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[
                { label: 'Uploaded', value: formatDateTime(fileImport.created_at) },
                { label: 'Created By', value: fileImport.created_by },
                { label: 'Scan Status', value: fileImport.status === 'uploaded' ? 'Pending' : 'Processed' },
                { label: 'Storage Key', value: fileImport.id },
                { label: 'Meet', value: meetName },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span style={{ fontFamily: 'var(--font-body)', color: 'var(--text)', fontWeight: 600 }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-body)' }}>Import Report</h3>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7 }}>
              This view uses the persisted import summary plus the live conflict queue from the backend. Open the Conflicts and Errors tabs below for the actionable records returned by the API.
            </p>
          </div>
        </div>
      ) : null}

      {activeTab === 'conflicts' ? (
        <Table columns={CONFLICT_COLS} data={conflicts} emptyTitle="No conflicts" emptyDescription="This import has no unresolved or recorded conflicts." />
      ) : null}

      {activeTab === 'errors' ? (
        <Table columns={ERROR_COLS} data={errorRows} emptyTitle="No report errors" emptyDescription="The persisted import report does not contain parse or validation errors." />
      ) : null}
    </div>
  )
}
