'use client'

import { useState, useEffect, useCallback } from 'react'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchPaymentReconciliationReport, type PaymentReconciliationReportRecord } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

// ─── Helpers ───────────────────────────────────────────────────────────────────

function fmtCents(c: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(c / 100)
}
function fmtDt(d: string) {
  return new Date(d).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

// ─── Config ────────────────────────────────────────────────────────────────────

const TABS = [
  { id: 'reconciliation', label: 'Payment checks' },
  { id: 'config',         label: 'Technical setup' },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function StripePage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)

  const [active, setActive] = useState('reconciliation')
  const [running, setRunning] = useState(false)
  const [loading, setLoading] = useState(true)
  const [report, setReport] = useState<PaymentReconciliationReportRecord | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [lastRefreshed, setLastRefreshed] = useState<Date | null>(null)

  const loadReport = useCallback(async (orgId: string) => {
    try {
      setLoading(true)
      setError(null)
      const r = await fetchPaymentReconciliationReport(api, orgId)
      setReport(r)
      setLastRefreshed(new Date())
    } catch (err) {
      setError(uiErrorMessage(err, 'Check your connection and try again.'))
    } finally {
      setLoading(false)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session?.user.orgId])

  useEffect(() => {
    if (session?.user.orgId) {
      loadReport(session.user.orgId)
    } else {
      setLoading(false)
    }
  }, [session?.user.orgId, loadReport])

  async function handleRunReconcile() {
    if (!session?.user.orgId) return
    setRunning(true)
    await loadReport(session.user.orgId)
    setRunning(false)
  }

  const alertTriggered = report?.stripe_alert_triggered ?? false
  const mismatchCents = report?.stripe_mismatch_cents ?? 0
  const hasStripeData = report?.stripe_reported_cents !== undefined

  return (
    <div className="page">
      
      {/* Header */}
      <div className="u-mb-24px" >
        <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap' }}>
          <h1 className="page-title">
            Stripe
          </h1>
        </div>
      </div>

      <Tabs tabs={TABS} active={active} onChange={setActive} />

      <div style={{ marginTop: '24px' }}>

        {/* ── Reconciliation Tab ── */}
        {active === 'reconciliation' && (
          <div>
            {/* Error banner */}
            {error && (
              <div className="u-p-16px-20px u-mb-20px u-gap-12px" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', display: 'flex', alignItems: 'flex-start' }}>
                <Icon name="alert-circle" size={16} color="var(--error)" />
                <div>
                  <p className="u-fs-fs-base u-fw-600 u-mb-2px" style={{ color: 'var(--error)' }}>Couldn&apos;t load payment check data</p>
                  <p className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{error}</p>
                </div>
              </div>
            )}

            {/* Mismatch alert banner */}
            {!loading && !error && alertTriggered && (
              <div className="u-p-16px-20px u-mb-20px u-gap-12px" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', display: 'flex', alignItems: 'flex-start' }}>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--error)" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="u-mt-1px" style={{ flexShrink: 0 }}>
                  <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
                  <line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" />
                </svg>
                <div>
                  <p className="u-fs-fs-base u-fw-600 u-mb-4px" style={{ color: 'var(--error)' }}>
                    Your records and Stripe don&apos;t match
                  </p>
                  <p className="u-fs-fs-sm u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>
                    There&apos;s a difference of {fmtCents(Math.abs(mismatchCents))} between your records and your Stripe balance. Review recent payments and refunds.
                  </p>
                </div>
              </div>
            )}

            {/* Reconciliation cards */}
            {loading ? (
              <div className="cols-3 u-gap-16px u-mb-24px" >
                {[1,2,3].map((i) => (
                  <div key={i} className="card-elevated u-p-20px-20px" >
                    <div className="u-mb-12px" style={{ height: '10px', width: '60%', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite' }} />
                    <div style={{ height: '28px', width: '80%', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: '0.1s' }} />
                  </div>
                ))}
              </div>
            ) : (
              <div className="cols-3 u-gap-16px u-mb-24px" >
                {[
                  {
                    label: 'Total collected',
                    value: report ? fmtCents(report.net_collected_cents) : '—',
                    sub: `${report?.row_count ?? 0} payments recorded`,
                    color: 'var(--text)',
                  },
                  {
                    label: 'Total refunded',
                    value: report ? fmtCents(report.refund_total_cents) : '—',
                    sub: 'All refunds issued',
                    color: 'var(--text)',
                  },
                  {
                    label: 'Payments needing review',
                    value: report ? report.unreconciled_row_count.toString() : '—',
                    sub: hasStripeData
                      ? (alertTriggered ? `${fmtCents(Math.abs(mismatchCents))} does not match Stripe` : 'Matches Stripe')
                      : 'Stripe balance not provided',
                    color: alertTriggered ? 'var(--error)' : (report?.unreconciled_row_count ?? 0) > 0 ? 'var(--warning)' : 'var(--success)',
                  },
                ].map((c) => (
                  <div key={c.label} className="card-elevated u-p-20px-20px" >
                    <p className="u-fs-fs-sm u-ff-font-body u-mb-8px" style={{ color: 'var(--text-muted)' }}>{c.label}</p>
                    <p className="u-ff-font-body u-fs-fs-2xl u-fw-700 u-mb-4px" style={{ color: c.color }}>{c.value}</p>
                    <p className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>{c.sub}</p>
                  </div>
                ))}
              </div>
            )}

            {/* Run details */}
            <div className="card u-p-20px-24px" >
              <div className="u-mb-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <div>
                  <h3 className="card-title u-mb-4px" >Last payment check</h3>
                  <p className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
                    {report ? `${fmtDt(report.generated_at)}, checked on demand` : lastRefreshed ? fmtDt(lastRefreshed.toISOString()) : '—'}
                  </p>
                </div>
                <button type="button"
                  onClick={handleRunReconcile}
                  disabled={running || loading}
                  className="u-p-8px-16px u-fs-fs-sm u-ff-font-body u-gap-8px" style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text-subtle)', cursor: (running || loading) ? 'not-allowed' : 'pointer', display: 'flex', alignItems: 'center', transition: 'all 0.15s' }}
                  onMouseEnter={(e) => { if (!running && !loading) { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' } }}
                  onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
                >
                  {running ? (
                    <><span className="spinner spinner-sm" aria-hidden />Running…</>
                  ) : (<><Icon name="refresh-cw" size={12} />Refresh</>)}
                </button>
              </div>
              <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
                {[
                  { label: 'Method',      value: 'Compares your records to Stripe' },
                  { label: 'Payments',    value: report ? fmtCents(report.payment_total_cents) : '—' },
                  { label: 'Refunds',     value: report ? fmtCents(report.refund_total_cents) : '—' },
                  { label: 'Status',      value: loading ? 'Checking…' : error ? 'Could not check' : alertTriggered ? 'Does not match Stripe' : 'All payments match Stripe' },
                  { label: 'Schedule',    value: 'Checked automatically every day' },
                ].map((r) => (
                  <div key={r.label} className="u-gap-16px" style={{ display: 'flex' }}>
                    <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)', minWidth: '100px', flexShrink: 0 }}>{r.label}</span>
                    <span className="u-fs-fs-sm u-ff-font-body" style={{ color: r.label === 'Status' && !loading && !error
                        ? alertTriggered ? 'var(--error)' : 'var(--success)'
                        : 'var(--text-subtle)' }}>{r.value}</span>
                  </div>
                ))}
              </div>
            </div>

            {/* Unreconciled rows summary */}
            {!loading && !error && report && report.unreconciled_row_count > 0 && (
              <div className="card u-p-16px-20px u-mt-16px" style={{ borderColor: 'var(--warning-border)', background: 'var(--warning-bg)' }}>
                <h3 className="card-title u-mb-8px" >
                  {report.unreconciled_row_count} Payment{report.unreconciled_row_count !== 1 ? 's' : ''} Need{report.unreconciled_row_count === 1 ? 's' : ''} Review
                </h3>
                <p className="u-fs-fs-sm u-lh-1_6" style={{ color: 'var(--text-muted)' }}>
                  These payments don&apos;t match Stripe. Review them in the{' '}
                  <a href="/billing" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none' }}>Billing overview</a>{' '}
                  or refresh the payment check above for details.
                </p>
              </div>
            )}
          </div>
        )}

        {/* ── Config Tab ── */}
        {active === 'config' && (
          <div>
            <div className="card u-p-24px" >
              <h3 className="card-title u-mb-16px" >Stripe environment variables</h3>
              <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
                {[
                  { key: 'STRIPE_SECRET_KEY',                              required: true },
                  { key: 'STRIPE_WEBHOOK_SECRET',                          required: true },
                  { key: 'STRIPE_CURRENCY',                                required: false },
                  { key: 'STRIPE_RECONCILIATION_ALERT_THRESHOLD_CENTS',    required: false },
                ].map((env) => (
                  <div key={env.key} className="u-gap-12px u-p-12px-16px" style={{ display: 'flex', alignItems: 'center', background: 'var(--bg-elevated)', borderRadius: '8px', border: '1px solid var(--border)' }}>
                    <div style={{ width: '8px', height: '8px', borderRadius: '50%', background: 'var(--text-muted)', flexShrink: 0 }} />
                    <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text)', flex: 1, minWidth: 0, overflowWrap: 'anywhere' }}>{env.key}</span>
                    {env.required && (
                      <span className="u-fs-fs-2xs u-p-1px-6px u-ff-font-body u-fw-600" style={{ background: 'var(--accent-subtle)', color: 'var(--accent-text, var(--accent))', border: '1px solid var(--accent-border)', borderRadius: '4px' }}>
                        REQUIRED
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>

            <div className="card u-p-20px-24px u-mt-16px" >
              <h3 className="card-title u-mb-12px" >Webhook URL</h3>
              <p className="u-fs-fs-sm u-mb-12px u-lh-1_6" style={{ color: 'var(--text-muted)' }}>
                Register this endpoint in your Stripe Dashboard.
              </p>
              <div className="u-p-12px-16px u-gap-12px" style={{ background: 'var(--bg-elevated)', borderRadius: '8px', border: '1px solid var(--border)', display: 'flex', alignItems: 'center' }}>
                <span className="u-ff-font-body u-fs-fs-base" style={{ color: 'var(--text)', flex: 1 }}>
                  POST /api/v1/webhooks/stripe
                </span>
                <span className="u-fs-fs-2xs u-p-2px-8px u-ff-font-body u-fw-600" style={{ background: 'var(--info-bg)', color: 'var(--info)', border: '1px solid var(--info-border)', borderRadius: '4px' }}>
                  PUBLIC · HMAC-VERIFIED
                </span>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
