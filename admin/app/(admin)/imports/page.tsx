'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchImports, fetchMeets, type ImportRecord, type MeetRecord } from '@/lib/adminData'

type ImportStatus = 'uploaded' | 'scanning' | 'parsing' | 'matching' | 'conflicts_pending' | 'applied' | 'failed' | 'archived'
type FileType = 'SD3' | 'CL2' | 'ZIP' | 'HY3'

type FileImportRow = {
  id: string
  filename: string
  fileType: FileType
  status: ImportStatus
  rowsParsed: number
  conflictCount: number
  uploadedBy: string
  uploadedAt: string
  meet: string | null
  fileSizeKb: number
}

const STATUS_CFG: Record<ImportStatus, { label: string; color: string; bg: string }> = {
  uploaded: { label: 'Uploaded', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  scanning: { label: 'Scanning', color: 'var(--info)', bg: 'var(--info-bg)' },
  parsing: { label: 'Parsing', color: 'var(--info)', bg: 'var(--info-bg)' },
  matching: { label: 'Matching', color: 'var(--warning)', bg: 'var(--warning-bg)' },
  conflicts_pending: { label: 'Conflicts Pending', color: 'var(--warning)', bg: 'var(--warning-bg)' },
  applied: { label: 'Applied', color: 'var(--success)', bg: 'var(--success-bg)' },
  failed: { label: 'Failed', color: 'var(--error)', bg: 'var(--error-bg)' },
  archived: { label: 'Archived', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

const FILE_TYPE_CFG: Record<FileType, { color: string; bg: string }> = {
  SD3: { color: 'var(--info)', bg: 'var(--info-bg)' },
  CL2: { color: 'var(--accent)', bg: 'var(--accent-subtle)' },
  ZIP: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
  HY3: { color: 'var(--success)', bg: 'var(--success-bg)' },
}

const COLUMNS: Column<FileImportRow>[] = [
  {
    key: 'filename',
    label: 'File',
    render: (row) => (
      <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
        <div style={{ width: '30px', height: '30px', borderRadius: 'var(--r-md)', background: FILE_TYPE_CFG[row.fileType].bg, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
          <Icon name="file-text" size={14} color={FILE_TYPE_CFG[row.fileType].color} />
        </div>
        <div>
          <Link href={`/imports/${row.id}`} style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none', fontFamily: 'var(--font-mono)' }}>{row.filename}</Link>
          {row.meet && <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '1px' }}>{row.meet}</div>}
        </div>
      </div>
    ),
  },
  {
    key: 'fileType',
    label: 'Type',
    render: (row) => <span style={{ fontSize: '11px', fontWeight: 700, padding: '2px 7px', borderRadius: '6px', background: FILE_TYPE_CFG[row.fileType].bg, color: FILE_TYPE_CFG[row.fileType].color, fontFamily: 'var(--font-mono)', letterSpacing: '0.05em' }}>{row.fileType}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => {
      const c = STATUS_CFG[row.status]
      return <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em', whiteSpace: 'nowrap' }}>{c.label}</span>
    },
  },
  { key: 'rowsParsed', label: 'Rows', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: row.rowsParsed > 0 ? 'var(--text)' : 'var(--text-muted)' }}>{row.rowsParsed || '—'}</span> },
  { key: 'conflictCount', label: 'Conflicts', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: row.conflictCount > 0 ? 'var(--warning)' : 'var(--text-muted)' }}>{row.conflictCount || '—'}</span> },
  { key: 'uploadedBy', label: 'Uploaded By', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.uploadedBy}</span> },
]

function mapRows(imports: ImportRecord[], meets: MeetRecord[]): FileImportRow[] {
  const meetNameById = new Map(meets.map((meet) => [meet.id, meet.name]))
  return imports.map((item) => ({
    id: item.id,
    filename: item.filename,
    fileType: item.file_type.toUpperCase() as FileType,
    status: item.status as ImportStatus,
    rowsParsed: item.row_count,
    conflictCount: item.conflict_count,
    uploadedBy: item.created_by,
    uploadedAt: item.created_at,
    meet: item.meet_id ? meetNameById.get(item.meet_id) ?? item.meet_id : null,
    fileSizeKb: Math.round((item.declared_size_bytes ?? 0) / 1024),
  })).sort((a, b) => b.uploadedAt.localeCompare(a.uploadedAt))
}

export default function ImportsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<FileImportRow[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [typeFilter, setTypeFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!session?.user.orgId) {
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const [imports, meets] = await Promise.all([fetchImports(api, session.user.orgId), fetchMeets(api, session.user.orgId)])
        if (!cancelled) setRows(mapRows(imports, meets))
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load imports.')
          setRows([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId])

  const filtered = useMemo(() => rows.filter((row) => {
    const query = search.toLowerCase()
    const matchesSearch = !search || row.filename.toLowerCase().includes(query) || (row.meet ?? '').toLowerCase().includes(query) || row.uploadedBy.toLowerCase().includes(query)
    const matchesStatus = statusFilter === 'all' || row.status === statusFilter
    const matchesType = typeFilter === 'all' || row.fileType === typeFilter
    return matchesSearch && matchesStatus && matchesType
  }), [rows, search, statusFilter, typeFilter])

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Imports</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Live file import jobs and conflict state</p>
        </div>
      </div>

      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '320px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search by filename, meet, or uploader…" value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', cursor: 'pointer' }}>
          <option value="all">All Statuses</option>
          {Object.entries(STATUS_CFG).map(([value, cfg]) => <option key={value} value={value}>{cfg.label}</option>)}
        </select>
        <select value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', cursor: 'pointer' }}>
          <option value="all">All Types</option>
          {(['SD3', 'CL2', 'ZIP', 'HY3'] as FileType[]).map((value) => <option key={value} value={value}>{value}</option>)}
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>{filtered.length} imports</span>
      </div>

      {loading ? (
        <EmptyState icon="upload-cloud" title="Loading imports" description="Fetching live import jobs from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load imports" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No imports found" emptyDescription="Try adjusting your filters." />
      )}
    </div>
  )
}
