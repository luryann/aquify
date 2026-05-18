'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchExportsForMeet, fetchMeets, generateExport, type ExportRecord, type MeetRecord } from '@/lib/adminData'

type ExportStatus = 'pending' | 'generating' | 'ready' | 'failed' | 'expired'
type ExportType = 'standard_sd3' | 'extended_sd3' | 'sdif_roster'

type FileExportRow = {
  id: string
  exportType: ExportType
  meet: string
  meetId: string
  status: ExportStatus
  filename: string
  fileSizeKb: number | null
  createdBy: string
  createdAt: string
  expiresAt: string | null
  downloadUrl: string | null
}

const STATUS_CFG: Record<ExportStatus, { label: string; color: string; bg: string }> = {
  pending: { label: 'Pending', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  generating: { label: 'Generating', color: 'var(--info)', bg: 'var(--info-bg)' },
  ready: { label: 'Ready', color: 'var(--success)', bg: 'var(--success-bg)' },
  failed: { label: 'Failed', color: 'var(--error)', bg: 'var(--error-bg)' },
  expired: { label: 'Expired', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

const EXPORT_TYPE_LABELS: Record<ExportType, string> = {
  standard_sd3: 'SD3 Standard',
  extended_sd3: 'SD3 Extended',
  sdif_roster: 'SDIF Roster',
}

const COLUMNS: Column<FileExportRow>[] = [
  {
    key: 'exportType',
    label: 'Export',
    render: (row) => (
      <div>
        <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '2px' }}>{EXPORT_TYPE_LABELS[row.exportType]}</div>
        <Link href={`/meets/${row.meetId}`} style={{ fontSize: '11px', color: 'var(--text-muted)', textDecoration: 'none' }}>{row.meet}</Link>
      </div>
    ),
  },
  {
    key: 'filename',
    label: 'File',
    render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>{row.filename}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => {
      const c = STATUS_CFG[row.status]
      return <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', whiteSpace: 'nowrap' }}>{c.label}</span>
    },
  },
  {
    key: 'fileSizeKb',
    label: 'Size',
    align: 'right',
    render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{row.fileSizeKb != null ? `${row.fileSizeKb} KB` : '—'}</span>,
  },
  {
    key: 'createdBy',
    label: 'Created By',
    render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.createdBy}</span>,
  },
  {
    key: 'createdAt',
    label: 'Created',
    render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{new Date(row.createdAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}</span>,
  },
]

async function loadExports(api: ReturnType<typeof useApiClient>, orgId: string): Promise<FileExportRow[]> {
  const meets = await fetchMeets(api, orgId)
  const exportsByMeet = await Promise.all(meets.map(async (meet) => ({
    meet,
    exports: await fetchExportsForMeet(api, orgId, meet.id).catch(() => []),
  })))

  return exportsByMeet.flatMap(({ meet, exports }) => exports.map((item: ExportRecord) => ({
    id: item.id,
    exportType: item.export_type as ExportType,
    meet: meet.name,
    meetId: meet.id,
    status: item.status as ExportStatus,
    filename: item.filename,
    fileSizeKb: item.size_bytes ? Math.round(item.size_bytes / 1024) : null,
    createdBy: item.created_by,
    createdAt: item.created_at,
    expiresAt: item.expires_at || null,
    downloadUrl: item.download_url || null,
  }))).sort((a, b) => b.createdAt.localeCompare(a.createdAt))
}

const INPUT_STYLE: React.CSSProperties = { width: '100%', padding: '9px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' }
const LABEL_STYLE: React.CSSProperties = { fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '5px' }

export default function ExportsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<FileExportRow[]>([])
  const [meets, setMeets] = useState<MeetRecord[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showNew, setShowNew] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [genError, setGenError] = useState<string | null>(null)
  const [genForm, setGenForm] = useState({ meet_id: '', export_type: 'standard_sd3' })

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId) {
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        setError(null)
        const [exports, meetsData] = await Promise.all([loadExports(api, session.user.orgId), fetchMeets(api, session.user.orgId)])
        if (!cancelled) { setRows(exports); setMeets(meetsData) }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load exports.')
          setRows([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId])

  const filtered = useMemo(() => rows.filter((row) => {
    const query = search.toLowerCase()
    const matchesSearch = !search || row.meet.toLowerCase().includes(query) || row.filename.toLowerCase().includes(query) || row.createdBy.toLowerCase().includes(query)
    const matchesStatus = statusFilter === 'all' || row.status === statusFilter
    return matchesSearch && matchesStatus
  }), [rows, search, statusFilter])

  async function handleGenerate(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId || !genForm.meet_id) return
    setGenerating(true)
    setGenError(null)
    try {
      await generateExport(api, session.user.orgId, genForm.meet_id, { export_type: genForm.export_type })
      setShowNew(false)
      setGenForm({ meet_id: '', export_type: 'standard_sd3' })
      const [exports, meetsData] = await Promise.all([loadExports(api, session.user.orgId), fetchMeets(api, session.user.orgId)])
      setRows(exports); setMeets(meetsData)
    } catch (err) {
      setGenError(err instanceof ApiError ? err.message : 'Failed to generate export.')
    } finally {
      setGenerating(false)
    }
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Exports</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Live export jobs generated from the persisted backend queue</p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setShowNew(true)}>
          <Icon name="download" size={14} /> Generate Export
        </button>
      </div>

      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '320px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search by meet, file, or creator…" value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', cursor: 'pointer' }}>
          <option value="all">All Statuses</option>
          {Object.entries(STATUS_CFG).map(([value, cfg]) => <option key={value} value={value}>{cfg.label}</option>)}
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>{filtered.length} exports</span>
      </div>

      {loading ? (
        <EmptyState icon="download" title="Loading exports" description="Fetching live export jobs from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load exports" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No exports found" emptyDescription="Generate an export from a meet to see it here." />
      )}

      {showNew && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.6)' }} onClick={(e) => { if (e.target === e.currentTarget) setShowNew(false) }}>
          <div style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', borderRadius: '14px', padding: '28px', width: '100%', maxWidth: '420px', boxShadow: '0 24px 80px rgba(0,0,0,0.4)' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <h2 style={{ fontSize: '18px', fontFamily: 'var(--font-display)', color: 'var(--text)', margin: 0 }}>Generate Export</h2>
              <button onClick={() => setShowNew(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)' }}><Icon name="x" size={18} /></button>
            </div>
            {genError && <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>{genError}</div>}
            <form onSubmit={handleGenerate} style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div>
                <label style={LABEL_STYLE}>Meet *</label>
                <select required style={INPUT_STYLE} value={genForm.meet_id} onChange={(e) => setGenForm((f) => ({ ...f, meet_id: e.target.value }))}>
                  <option value="">Select meet…</option>
                  {meets.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
                </select>
              </div>
              <div>
                <label style={LABEL_STYLE}>Export Type *</label>
                <select style={INPUT_STYLE} value={genForm.export_type} onChange={(e) => setGenForm((f) => ({ ...f, export_type: e.target.value }))}>
                  <option value="standard_sd3">SD3 Standard</option>
                  <option value="extended_sd3">SD3 Extended</option>
                  <option value="sdif_roster">SDIF Roster</option>
                </select>
              </div>
              <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '4px' }}>
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={generating || !genForm.meet_id}>{generating ? 'Generating…' : 'Generate'}</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
