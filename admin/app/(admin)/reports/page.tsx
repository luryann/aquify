'use client'

import { useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  downloadReportArtifact,
  fetchInvoiceAgingReport,
  fetchMeetFeesReport,
  fetchMembershipStatusReport,
  fetchPaymentReconciliationReport,
  fetchRegistrationStatusReport,
  type ReportArtifactRecord,
} from '@/lib/adminData'

type ReportKey = 'membership-status' | 'meet-fees' | 'invoice-aging' | 'payment-reconciliation' | 'registration-status'

type ReportSummary = {
  key: ReportKey
  title: string
  category: string
  description: string
  rowCount: number
  generatedAt: string
  accent: string
}

const REPORTS: Array<{ key: ReportKey; title: string; category: string; description: string; accent: string }> = [
  { key: 'membership-status', title: 'Membership Status', category: 'Members', description: 'Live roster, household, and primary group coverage.', accent: 'var(--accent)' },
  { key: 'meet-fees', title: 'Meet Fees', category: 'Billing', description: 'Issued meet-fee invoice lines grouped from the persisted billing data.', accent: 'var(--warning)' },
  { key: 'invoice-aging', title: 'Invoice Aging', category: 'Billing', description: 'Outstanding balances bucketed by current aging windows.', accent: 'var(--error)' },
  { key: 'payment-reconciliation', title: 'Payment Reconciliation', category: 'Billing', description: 'Payment, refund, and ledger agreement from the live billing service.', accent: 'var(--success)' },
  { key: 'registration-status', title: 'Registration Status', category: 'Members', description: 'Current season registration and waiver completion view.', accent: 'var(--info)' },
]

function formatDateTime(value: string) {
  return new Date(value).toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
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
          { key: 'membership-status', title: 'Membership Status', category: 'Members', description: 'Live roster, household, and primary group coverage.', rowCount: membership.row_count, generatedAt: membership.generated_at, accent: 'var(--accent)' },
          { key: 'meet-fees', title: 'Meet Fees', category: 'Billing', description: 'Issued meet-fee invoice lines grouped from the persisted billing data.', rowCount: meetFees.row_count, generatedAt: meetFees.generated_at, accent: 'var(--warning)' },
          { key: 'invoice-aging', title: 'Invoice Aging', category: 'Billing', description: 'Outstanding balances bucketed by current aging windows.', rowCount: invoiceAging.row_count, generatedAt: invoiceAging.generated_at, accent: 'var(--error)' },
          { key: 'payment-reconciliation', title: 'Payment Reconciliation', category: 'Billing', description: 'Payment, refund, and ledger agreement from the live billing service.', rowCount: reconciliation.row_count, generatedAt: reconciliation.generated_at, accent: 'var(--success)' },
          { key: 'registration-status', title: 'Registration Status', category: 'Members', description: 'Current season registration and waiver completion view.', rowCount: registration.row_count, generatedAt: registration.generated_at, accent: 'var(--info)' },
        ]

        if (!cancelled) setSummaries(next)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Unable to load live report summaries.')
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
      setError(err instanceof ApiError ? err.message : 'Unable to generate report artifact.')
    } finally {
      setDownloading(null)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="bar-chart-2" title="Sign in required" description="Authenticate to run live reports." /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1180, margin: '0 auto' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Reporting
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', fontWeight: 400, color: 'var(--text)', marginBottom: '6px' }}>
          Live Report Console
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)', maxWidth: '68ch' }}>
          These report cards are backed by the production report endpoints. Row counts and generated timestamps come from the live API, and CSV exports are stored as report artifacts instead of mock downloads.
        </p>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      {loading ? (
        <EmptyState icon="bar-chart-2" title="Loading report definitions" description="Fetching live summaries from the backend reporting pack." />
      ) : summaries.length === 0 ? (
        <EmptyState icon="bar-chart-2" title="No reports available" description="Live report endpoints returned no data for this organisation yet." />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
          {grouped.map(([category, items]) => (
            <section key={category}>
              <div style={{ marginBottom: '12px' }}>
                <h2 style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)' }}>{category}</h2>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: '14px' }}>
                {items.map((report) => {
                  const artifact = artifacts[report.key]
                  return (
                    <div key={report.key} className="card" style={{ padding: '18px 18px 16px', borderTop: `3px solid ${report.accent}` }}>
                      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '12px', marginBottom: '12px' }}>
                        <div>
                          <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: report.accent, textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '4px' }}>
                            {REPORTS.find((item) => item.key === report.key)?.category}
                          </div>
                          <h3 style={{ fontSize: '18px', color: 'var(--text)', marginBottom: '4px' }}>{report.title}</h3>
                          <p style={{ fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.55 }}>{report.description}</p>
                        </div>
                        <div style={{ width: '36px', height: '36px', borderRadius: '10px', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--accent-subtle)', color: 'var(--accent)', flexShrink: 0 }}>
                          <Icon name="file-text" size={16} />
                        </div>
                      </div>

                      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px', marginBottom: '14px' }}>
                        <div style={{ padding: '12px', borderRadius: '8px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                          <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>Rows</div>
                          <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>{report.rowCount}</div>
                        </div>
                        <div style={{ padding: '12px', borderRadius: '8px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                          <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>Generated</div>
                          <div style={{ fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.45 }}>{formatDateTime(report.generatedAt)}</div>
                        </div>
                      </div>

                      <div style={{ display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap' }}>
                        <button
                          onClick={() => void handleDownload(report.key)}
                          disabled={downloading === report.key}
                          style={{
                            padding: '8px 12px',
                            borderRadius: '7px',
                            border: 'none',
                            background: report.accent,
                            color: '#fff',
                            fontSize: '12px',
                            fontWeight: 600,
                            cursor: downloading === report.key ? 'not-allowed' : 'pointer',
                          }}
                        >
                          {downloading === report.key ? 'Generating…' : 'Generate CSV'}
                        </button>
                        {artifact ? (
                          <a
                            href={artifact.download_url}
                            target="_blank"
                            rel="noreferrer"
                            style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none' }}
                          >
                            {artifact.filename} · {formatBytes(artifact.size_bytes)}
                          </a>
                        ) : (
                          <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>Artifact delivery enabled</span>
                        )}
                      </div>
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
