'use client'

import { useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  downloadReportArtifact,
  fetchInvoiceAgingReport,
  fetchMeetFeesReport,
  fetchMembershipStatusReport,
  fetchPaymentReconciliationReport,
  fetchRegistrationStatusReport,
  type ReportArtifactRecord,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { formatDateTime } from '@/lib/dates'
import { PageSkeleton } from '@/components/ui/PageSkeleton'

type ReportKey = 'membership-status' | 'meet-fees' | 'invoice-aging' | 'payment-reconciliation' | 'registration-status'

type ReportSummary = {
  key: ReportKey
  title: string
  category: string
  description: string
  rowCount: number
  generatedAt: string
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function csvPath(key: ReportKey) {
  switch (key) {
    case 'membership-status':
      return '/api/v1/reports/membership-status?format=csv'
    case 'meet-fees':
      return '/api/v1/reports/meet-fees?format=csv'
    case 'invoice-aging':
      return '/api/v1/reports/invoice-aging?format=csv'
    case 'payment-reconciliation':
      return '/api/v1/reports/payment-reconciliation?format=csv'
    case 'registration-status':
      return '/api/v1/reports/registration-status?format=csv'
  }
}

export default function ReportsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [summaries, setSummaries] = useState<ReportSummary[]>([])
  const [artifacts, setArtifacts] = useState<Record<string, ReportArtifactRecord | null>>({})
  const [loading, setLoading] = useState(true)
  const [downloading, setDownloading] = useState<string | null>(null)
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
        const [membership, meetFees, invoiceAging, reconciliation, registration] = await Promise.all([
          fetchMembershipStatusReport(api, session.user.orgId),
          fetchMeetFeesReport(api, session.user.orgId),
          fetchInvoiceAgingReport(api, session.user.orgId),
          fetchPaymentReconciliationReport(api, session.user.orgId),
          fetchRegistrationStatusReport(api, session.user.orgId),
        ])

        const next: ReportSummary[] = [
          { key: 'membership-status', title: 'Membership status', category: 'Members', description: 'Roster, household, and primary group coverage.', rowCount: membership.row_count, generatedAt: membership.generated_at },
          { key: 'meet-fees', title: 'Meet fees', category: 'Billing', description: 'Issued meet-fee invoice lines grouped by household.', rowCount: meetFees.row_count, generatedAt: meetFees.generated_at },
          { key: 'invoice-aging', title: 'Invoice aging', category: 'Billing', description: 'Outstanding balances bucketed by current aging windows.', rowCount: invoiceAging.row_count, generatedAt: invoiceAging.generated_at },
          { key: 'payment-reconciliation', title: 'Payment check', category: 'Billing', description: 'Confirms payments, refunds, and account totals all match up.', rowCount: reconciliation.row_count, generatedAt: reconciliation.generated_at },
          { key: 'registration-status', title: 'Registration status', category: 'Members', description: 'Current season registration and waiver completion view.', rowCount: registration.row_count, generatedAt: registration.generated_at },
        ]

        if (!cancelled) setSummaries(next)
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load report summaries. Try again.'))
          setSummaries([])
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

  const grouped = useMemo(() => {
    const map = new Map<string, ReportSummary[]>()
    for (const summary of summaries) {
      const existing = map.get(summary.category) ?? []
      existing.push(summary)
      map.set(summary.category, existing)
    }
    return Array.from(map.entries())
  }, [summaries])

  async function handleDownload(key: ReportKey) {
    if (!session?.user.orgId) return
    setDownloading(key)
    setError(null)
    try {
      const artifact = await downloadReportArtifact(api, session.user.orgId, csvPath(key))
      setArtifacts((current) => ({ ...current, [key]: artifact }))
      if (artifact?.download_url) {
        window.open(artifact.download_url, '_blank', 'noopener,noreferrer')
      }
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t generate report artifact. Try again.'))
    } finally {
      setDownloading(null)
    }
  }

  if (!session) {
    return <div className="u-p-32px" ><EmptyState icon="bar-chart-2" title="Sign in required" description="Sign in to run reports." /></div>
  }

  return (
    <div className="page">
      <div className="page-heading">
        <h1 className="page-title">
          Reports
        </h1>
      </div>

      {error ? (
        <div className="u-mb-20px u-p-12px-16px u-fs-fs-sm" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)' }}>
          {error}
        </div>
      ) : null}

      {loading ? (
        <PageSkeleton variant="table" noHeader rows={4} />
      ) : summaries.length === 0 ? (
        <EmptyState icon="bar-chart-2" title="No reports available" description="No report data is available for this organization yet." />
      ) : (
        <div className="stack">
          {grouped.map(([category, items]) => (
            <section key={category}>
              <h2 className="section-title u-mb-12px" >{category}</h2>
              <div className="card u-p-0" >
                {items.map((report, i) => {
                  const artifact = artifacts[report.key]
                  return (
                    <div
                      key={report.key}
                      className="report-row"
                      style={{ borderTop: i === 0 ? 'none' : '1px solid var(--border)' }}
                    >
                      <div style={{ minWidth: 0 }}>
                        <h3 className="card-title">{report.title}</h3>
                        <p className="u-fs-fs-sm u-mt-2px" style={{ color: 'var(--text-subtle)' }}>{report.description}</p>
                        {artifact ? (
                          <a
                            href={artifact.download_url}
                            target="_blank"
                            rel="noreferrer"
                            className="u-mt-4px u-fs-fs-sm" style={{ display: 'inline-block' }}
                          >
                            {artifact.filename} · {formatBytes(artifact.size_bytes)}
                          </a>
                        ) : null}
                      </div>
                      <div className="report-stat">
                        <span>Records</span>
                        <strong className="tabular">{report.rowCount}</strong>
                      </div>
                      <div className="report-stat">
                        <span>Generated</span>
                        <strong>{formatDateTime(report.generatedAt)}</strong>
                      </div>
                      <button
                        type="button"
                        className="btn btn-secondary btn-sm"
                        onClick={() => void handleDownload(report.key)}
                        disabled={downloading === report.key}
                      >
                        <Icon name="download" size={14} />
                        {downloading === report.key ? 'Generating…' : 'Download CSV'}
                      </button>
                    </div>
                  )
                })}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}
