'use client'

import { useEffect, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { Modal } from '@/components/ui/Modal'
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
  { key: 'reportType', label: 'Report', render: (row) => <span className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{reportLabel(row.reportType)}</span> },
  { key: 'filename', label: 'File', render: (row) => <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>{row.filename}</span> },
  {
    key: 'status',
    label: 'Status',
    render: (row) => {
      const c = STATUS_CFG[row.status]
      return <span className="badge" style={{ background: c.bg, color: c.color }}>{c.label}</span>
    },
  },
  { key: 'fileSizeKb', label: 'Size', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{row.fileSizeKb != null ? `${row.fileSizeKb} KB` : '—'}</span> },
  { key: 'createdAt', label: 'Created', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{new Date(row.createdAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}</span> },
  {
    key: 'id',
    label: '',
    align: 'right',
    render: (row) =>
      row.status === 'ready' && row.downloadUrl ? (
        <a href={row.downloadUrl} target="_blank" rel="noreferrer" className="btn u-gap-6px u-fs-fs-sm u-p-4px-12px" style={{ display: 'inline-flex', alignItems: 'center' }}>
          <Icon name="download" size={13} /> PDF
        </a>
      ) : (
        <span className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>—</span>
      ),
  },
]

const INPUT_STYLE: React.CSSProperties = { width: '100%' }

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
      <div className="u-mb-16px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <button className="btn btn-primary u-gap-6px" style={{ display: 'flex', alignItems: 'center' }} onClick={() => setShowNew(true)}>
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

      <Modal open={Boolean(showNew)} onClose={() => setShowNew(false)} title="Generate report" width={440}>
            {genError && <div role="alert" className="form-banner-error u-mb-16px">{genError}</div>}
            <form onSubmit={handleGenerate} className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div>
                <label className="form-label form-label-required">Report type</label>
                <select style={INPUT_STYLE} value={genForm.report_type} onChange={(e) => setGenForm((f) => ({ ...f, report_type: e.target.value as ReportType }))}>
                  {REPORT_TYPE_OPTIONS.map((r) => <option key={r.value} value={r.value}>{r.label}</option>)}
                </select>
                {activeReport && <p className="u-fs-fs-xs" style={{ color: 'var(--text-muted)', margin: '6px 2px 0' }}>{activeReport.hint}</p>}
              </div>
              {genForm.report_type === 'entry_report' && (
                <div>
                  <label className="form-label">Group entries by</label>
                  <div className="u-gap-6px" style={{ display: 'flex' }}>
                    {GROUPING_OPTIONS.map((g) => {
                      const active = genForm.grouping === g.value
                      return (
                        <button
                          key={g.value}
                          type="button"
                          onClick={() => setGenForm((f) => ({ ...f, grouping: g.value }))}
                          className="u-p-8px-6px u-fs-fs-sm u-fw-600" style={{ flex: 1, cursor: 'pointer', borderRadius: 'var(--r-md)', border: `1px solid ${active ? 'var(--accent)' : 'var(--border)'}`, background: active ? 'var(--accent-bg, rgba(235,93,30,0.12))' : 'var(--bg-elevated)', color: active ? 'var(--accent)' : 'var(--text-subtle)' }}
                        >
                          {g.label}
                        </button>
                      )
                    })}
                  </div>
                </div>
              )}
              <div className="form-actions" style={{ display: 'flex', justifyContent: 'flex-end' }}>
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={generating}>{generating ? 'Generating…' : 'Generate'}</button>
              </div>
            </form>
      </Modal>
    </div>
  )
}
