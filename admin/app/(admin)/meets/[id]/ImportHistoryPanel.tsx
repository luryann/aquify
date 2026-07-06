'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchImports } from '@/lib/adminData'
import { IMPORT_FILE_TYPE_CFG, normalizeImportFileType, type DisplayImportFileType } from '@/lib/importFileTypes'

type ImportStatus = 'uploaded' | 'scanning' | 'parsing' | 'matching' | 'conflicts_pending' | 'applied' | 'failed' | 'archived'

type ImportRow = {
  id: string
  filename: string
  fileType: DisplayImportFileType
  status: ImportStatus
  rowsParsed: number
  conflictCount: number
  uploadedBy: string
  uploadedAt: string
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

const COLUMNS: Column<ImportRow>[] = [
  {
    key: 'filename',
    label: 'File',
    render: (row) => (
      <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
        <div style={{ width: '30px', height: '30px', borderRadius: 'var(--r-md)', background: IMPORT_FILE_TYPE_CFG[row.fileType].bg, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
          <Icon name="file-text" size={14} color={IMPORT_FILE_TYPE_CFG[row.fileType].color} />
        </div>
        <Link href={`/imports/${row.id}`} style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none', fontFamily: 'var(--font-body)' }}>{row.filename}</Link>
      </div>
    ),
  },
  {
    key: 'fileType',
    label: 'Type',
    render: (row) => <span style={{ fontSize: '11px', fontWeight: 700, padding: '2px 7px', borderRadius: '6px', background: IMPORT_FILE_TYPE_CFG[row.fileType].bg, color: IMPORT_FILE_TYPE_CFG[row.fileType].color, fontFamily: 'var(--font-body)', letterSpacing: '0.05em' }}>{row.fileType}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => {
      const c = STATUS_CFG[row.status]
      return <span className="badge" style={{ background: c.bg, color: c.color }}>{c.label}</span>
    },
  },
  { key: 'rowsParsed', label: 'Rows', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: row.rowsParsed > 0 ? 'var(--text)' : 'var(--text-muted)' }}>{row.rowsParsed || '—'}</span> },
  { key: 'conflictCount', label: 'Conflicts', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: row.conflictCount > 0 ? 'var(--warning)' : 'var(--text-muted)' }}>{row.conflictCount || '—'}</span> },
  { key: 'uploadedBy', label: 'Uploaded By', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.uploadedBy}</span> },
]

export function ImportHistoryPanel({ meetId }: { meetId: string }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<ImportRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

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
        const imports = await fetchImports(api, session.user.orgId)
        if (cancelled) return
        const scoped = imports
          .filter((item) => item.meet_id === meetId)
          .map((item) => ({
            id: item.id,
            filename: item.filename,
            fileType: normalizeImportFileType(item.file_type),
            status: item.status as ImportStatus,
            rowsParsed: item.row_count,
            conflictCount: item.conflict_count,
            uploadedBy: item.created_by,
            uploadedAt: item.created_at,
          }))
          .sort((a, b) => b.uploadedAt.localeCompare(a.uploadedAt))
        setRows(scoped)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load import history.')
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
  }, [api, session?.user.orgId, meetId])

  const hint = useMemo(() => 'Files parsed through this meet’s entry-file import. Start a new import from the meet creation flow.', [])

  return (
    <div>
      <p style={{ fontSize: '12px', color: 'var(--text-subtle)', marginBottom: '16px' }}>{hint}</p>
      {loading ? (
        <EmptyState icon="upload-cloud" title="Loading import history" description="Fetching import records for this meet." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load import history" description={error} />
      ) : (
        <Table columns={COLUMNS} data={rows} pageSize={10} emptyTitle="No imports for this meet" emptyDescription="Files parsed while creating or updating this meet will show up here." />
      )}
    </div>
  )
}
