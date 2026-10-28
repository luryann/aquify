'use client'

import { useEffect, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchExportsForMeet,
  generateReport,
  type ExportRecord,
  type ReportType,
  type EntryReportGrouping,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

type ExportStatus = 'pending' | 'generating' | 'ready' | 'failed' | 'expired'

type ReportRow = {
  id: string
  reportType: string
  status: ExportStatus
  filename: string
  fileSizeKb: number | null
  createdBy: string
  createdAt: string
  downloadUrl: string | null
}

const STATUS_CFG: Record<ExportStatus, { label: string; color: string; bg: string }> = {
  pending: { label: 'Pending', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  generating: { label: 'Generating', color: 'var(--info)', bg: 'var(--info-bg)' },
  ready: { label: 'Ready', color: 'var(--success)', bg: 'var(--success-bg)' },
  failed: { label: 'Failed', color: 'var(--error)', bg: 'var(--error-bg)' },
  expired: { label: 'Expired', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

const REPORT_TYPE_LABELS: Record<string, string> = {
  eligibility_report: 'Eligibility Report',
  entry_report: 'Entry Report',
  event_report: 'Event Report',
}

const REPORT_TYPE_OPTIONS: { value: ReportType; label: string; hint: string }[] = [
  { value: 'entry_report', label: 'Entry report', hint: 'Every approved entry — swimmer, event, seed time.' },
  { value: 'event_report', label: 'Event report', hint: 'Swimmers grouped under each event, fastest seed first.' },
  { value: 'eligibility_report', label: 'Eligibility report', hint: 'Per event, each swimmer’s sex/age eligibility status.' },
]

const GROUPING_OPTIONS: { value: EntryReportGrouping; label: string }[] = [
  { value: 'session', label: 'By session' },
  { value: 'group', label: 'By training group' },
  { value: 'alphabetical', label: 'Alphabetical' },
]

function reportLabel(reportType: string): string {
  return REPORT_TYPE_LABELS[reportType] ?? reportType.replace(/_/g, ' ')
}

const COLUMNS: Column<ReportRow>[] = [
  { key: 'reportType', label: 'Report', render: (row) => <span style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--text)' }}>{reportLabel(row.reportType)}</span> },
  { key: 'filename', label: 'File', render: (row) => <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', fontFamily: 'var(--font-body)' }}>{row.filename}</span> },
  {
    key: 'status',
    label: 'Status',
    render: (row) => {
      const c = STATUS_CFG[row.status]
      return <span className="badge" style={{ background: c.bg, color: c.color }}>{c.label}</span>
    },
  },
  { key: 'fileSizeKb', label: 'Size', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>{row.fileSizeKb != null ? `${row.fileSizeKb} KB` : '—'}</span> },
  { key: 'createdAt', label: 'Created', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>{new Date(row.createdAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}</span> },
  {
    key: 'id',
    label: '',
    align: 'right',
    render: (row) =>
      row.status === 'ready' && row.downloadUrl ? (
        <a href={row.downloadUrl} target="_blank" rel="noreferrer" className="btn" style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', fontSize: 'var(--fs-sm)', padding: '4px 12px' }}>
          <Icon name="download" size={13} /> PDF
        </a>
      ) : (
        <span style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)' }}>—</span>
      ),
  },
]

const INPUT_STYLE: React.CSSProperties = { width: '100%', padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 'var(--fs-base)', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' }
const LABEL_STYLE: React.CSSProperties = { fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }

export function ReportsPanel({ meetId }: { meetId: string }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<ReportRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showNew, setShowNew] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [genError, setGenError] = useState<string | null>(null)
  const [genForm, setGenForm] = useState<{ report_type: ReportType; grouping: EntryReportGrouping }>({
    report_type: 'entry_report',
    grouping: 'session',
  })

  async function loadReports() {
    if (!session?.user.orgId) return
    const exports = await fetchExportsForMeet(api, session.user.orgId, meetId)
    setRows(mapRows(exports))
  }

  function mapRows(exports: ExportRecord[]): ReportRow[] {
    return exports.map((item) => ({
      id: item.id,
      reportType: item.export_type,
      status: item.status as ExportStatus,
      filename: item.filename,
      fileSizeKb: item.size_bytes ? Math.round(item.size_bytes / 1024) : null,
      createdBy: item.created_by,
      createdAt: item.created_at,
      downloadUrl: item.download_url || null,
    })).sort((a, b) => b.createdAt.localeCompare(a.createdAt))
  }

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
        const exports = await fetchExportsForMeet(api, session.user.orgId, meetId)
        if (!cancelled) setRows(mapRows(exports))
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load reports. Try again.'))
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, session?.user.orgId, meetId])

  async function handleGenerate(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId) return
    setGenerating(true)
    setGenError(null)
    try {
      await generateReport(api, session.user.orgId, meetId, {
        report_type: genForm.report_type,
        ...(genForm.report_type === 'entry_report' ? { grouping: genForm.grouping } : {}),
      })
      setShowNew(false)
      setGenForm({ report_type: 'entry_report', grouping: 'session' })
      await loadReports()
    } catch (err) {
      setGenError(uiErrorMessage(err, 'Couldn’t generate report. Try again.'))
    } finally {
      setGenerating(false)
    }
  }

  const activeReport = REPORT_TYPE_OPTIONS.find((r) => r.value === genForm.report_type)

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '16px' }}>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setShowNew(true)}>
          <Icon name="download" size={14} /> Generate Report
        </button>
      </div>

      {loading ? (
        <EmptyState icon="download" title="Loading reports" description="Fetching generated reports for this meet." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load reports" description={error} />
      ) : (
        <Table columns={COLUMNS} data={rows} pageSize={10} emptyTitle="No reports yet" emptyDescription="Generate a report to see it here." />
      )}

      {showNew && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.6)' }} onClick={(e) => { if (e.target === e.currentTarget) setShowNew(false) }}>
          <div style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', borderRadius: '12px', padding: '24px', width: '100%', maxWidth: '440px', boxShadow: '0 24px 80px rgba(0,0,0,0.4)' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <h2 className="section-title" style={{ margin: 0 }}>Generate report</h2>
              <button onClick={() => setShowNew(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)' }}><Icon name="x" size={18} /></button>
            </div>
            {genError && <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '12px 16px', marginBottom: '16px', fontSize: 'var(--fs-base)' }}>{genError}</div>}
            <form onSubmit={handleGenerate} style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div>
                <label style={LABEL_STYLE}>Report Type *</label>
                <select style={INPUT_STYLE} value={genForm.report_type} onChange={(e) => setGenForm((f) => ({ ...f, report_type: e.target.value as ReportType }))}>
                  {REPORT_TYPE_OPTIONS.map((r) => <option key={r.value} value={r.value}>{r.label}</option>)}
                </select>
                {activeReport && <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', margin: '6px 2px 0' }}>{activeReport.hint}</p>}
              </div>
              {genForm.report_type === 'entry_report' && (
                <div>
                  <label style={LABEL_STYLE}>Group entries by</label>
                  <div style={{ display: 'flex', gap: '6px' }}>
                    {GROUPING_OPTIONS.map((g) => {
                      const active = genForm.grouping === g.value
                      return (
                        <button
                          key={g.value}
                          type="button"
                          onClick={() => setGenForm((f) => ({ ...f, grouping: g.value }))}
                          style={{
                            flex: 1,
                            padding: '8px 6px',
                            fontSize: 'var(--fs-sm)',
                            fontWeight: 600,
                            cursor: 'pointer',
                            borderRadius: 'var(--r-md)',
                            border: `1px solid ${active ? 'var(--accent)' : 'var(--border)'}`,
                            background: active ? 'var(--accent-bg, rgba(235,93,30,0.12))' : 'var(--bg-elevated)',
                            color: active ? 'var(--accent)' : 'var(--text-subtle)',
                          }}
                        >
                          {g.label}
                        </button>
                      )
                    })}
                  </div>
                </div>
              )}
              <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end', marginTop: '4px' }}>
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={generating}>{generating ? 'Generating…' : 'Generate'}</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
