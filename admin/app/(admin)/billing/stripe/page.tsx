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
      setError(uiErrorMessage(err, 'Failed to load reconciliation data'))
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
      <style>{`@keyframes spin { to { transform: rotate(360deg); } } @keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>

      {/* Header */}
      <div style={{ marginBottom: '24px' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '12px' }}>
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
              <div style={{ padding: '16px 20px', marginBottom: '20px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
                <Icon name="alert-circle" size={16} color="var(--error)" />
                <div>
                  <p style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--error)', marginBottom: '2px' }}>Couldn&apos;t load payment check data</p>
                  <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>{error}</p>
                </div>
              </div>
            )}

            {/* Mismatch alert banner */}
            {!loading && !error && alertTriggered && (
              <div style={{ padding: '16px 20px', marginBottom: '20px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--error)" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0, marginTop: '1px' }}>
                  <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
                  <line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" />
                </svg>
                <div>
                  <p style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--error)', marginBottom: '4px' }}>
                    Your records and Stripe don&apos;t match
                  </p>
                  <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', lineHeight: 1.5 }}>
                    There&apos;s a difference of {fmtCents(Math.abs(mismatchCents))} between your records and your Stripe balance. Review recent payments and refunds.
                  </p>
                </div>
              </div>
            )}

            {/* Reconciliation cards */}
            {loading ? (
              <div className="cols-3" style={{ gap: '16px', marginBottom: '24px' }}>
                {[1,2,3].map((i) => (
                  <div key={i} className="card-elevated" style={{ padding: '20px 20px' }}>
                    <div style={{ height: '10px', width: '60%', background: 'var(--bg-elevated)', borderRadius: '4px', marginBottom: '12px', animation: 'pulse 1.5s ease-in-out infinite' }} />
                    <div style={{ height: '28px', width: '80%', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: '0.1s' }} />
                  </div>
                ))}
              </div>
            ) : (
              <div className="cols-3" style={{ gap: '16px', marginBottom: '24px' }}>
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
                  <div key={c.label} className="card-elevated" style={{ padding: '20px 20px' }}>
                    <p style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '8px' }}>{c.label}</p>
                    <p style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-2xl)', fontWeight: 700, color: c.color, marginBottom: '4px' }}>{c.value}</p>
                    <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)' }}>{c.sub}</p>
                  </div>
                ))}
              </div>
            )}

            {/* Run details */}
            <div className="card" style={{ padding: '20px 24px' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
                <div>
                  <h3 className="card-title" style={{ marginBottom: '4px' }}>Last payment check</h3>
                  <p style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)' }}>
                    {report ? `${fmtDt(report.generated_at)}, checked on demand` : lastRefreshed ? fmtDt(lastRefreshed.toISOString()) : '—'}
                  </p>
                </div>
                <button
                  onClick={handleRunReconcile}
                  disabled={running || loading}
                  style={{
                    padding: '8px 16px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px',
                    color: 'var(--text-subtle)', fontSize: 'var(--fs-sm)', cursor: (running || loading) ? 'not-allowed' : 'pointer',
                    fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '8px', transition: 'all 0.15s',
                  }}
                  onMouseEnter={(e) => { if (!running && !loading) { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' } }}
                  onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
                >
                  {running ? (
                    <><div style={{ width: '12px', height: '12px', borderRadius: '50%', border: '2px solid var(--border)', borderTopColor: 'var(--text-subtle)', animation: 'spin 0.8s linear infinite' }} />Running…</>
                  ) : (<><Icon name="refresh-cw" size={12} />Refresh</>)}
                </button>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {[
                  { label: 'Method',      value: 'Compares your records to Stripe' },
                  { label: 'Payments',    value: report ? fmtCents(report.payment_total_cents) : '—' },
                  { label: 'Refunds',     value: report ? fmtCents(report.refund_total_cents) : '—' },
                  { label: 'Status',      value: loading ? 'Checking…' : error ? 'Could not check' : alertTriggered ? 'Does not match Stripe' : 'All payments match Stripe' },
                  { label: 'Schedule',    value: 'Checked automatically every day' },
                ].map((r) => (
                  <div key={r.label} style={{ display: 'flex', gap: '16px' }}>
                    <span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', minWidth: '100px', flexShrink: 0 }}>{r.label}</span>
                    <span style={{
                      fontSize: 'var(--fs-sm)',
                      color: r.label === 'Status' && !loading && !error
                        ? alertTriggered ? 'var(--error)' : 'var(--success)'
                        : 'var(--text-subtle)',
                      fontFamily: 'var(--font-body)',
                    }}>{r.value}</span>
                  </div>
                ))}
              </div>
            </div>

            {/* Unreconciled rows summary */}
            {!loading && !error && report && report.unreconciled_row_count > 0 && (
              <div className="card" style={{ padding: '16px 20px', marginTop: '16px', borderColor: 'var(--warning-border)', background: 'var(--warning-bg)' }}>
                <h3 className="card-title" style={{ marginBottom: '8px' }}>
                  {report.unreconciled_row_count} Payment{report.unreconciled_row_count !== 1 ? 's' : ''} Need{report.unreconciled_row_count === 1 ? 's' : ''} Review
                </h3>
                <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', lineHeight: 1.6 }}>
                  These payments don&apos;t match Stripe. Review them in the{' '}
                  <a href="/billing" style={{ color: 'var(--accent)', textDecoration: 'none' }}>Billing overview</a>{' '}
                  or refresh the payment check above for details.
                </p>
              </div>
            )}
          </div>
        )}

        {/* ── Config Tab ── */}
        {active === 'config' && (
          <div>
            <div className="card" style={{ padding: '24px' }}>
              <h3 className="card-title" style={{ marginBottom: '16px' }}>Stripe environment variables</h3>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {[
                  { key: 'STRIPE_SECRET_KEY',                              required: true },
                  { key: 'STRIPE_WEBHOOK_SECRET',                          required: true },
                  { key: 'STRIPE_CURRENCY',                                required: false },
                  { key: 'STRIPE_RECONCILIATION_ALERT_THRESHOLD_CENTS',    required: false },
                ].map((env) => (
                  <div key={env.key} style={{ display: 'flex', alignItems: 'center', gap: '12px', padding: '12px 16px', background: 'var(--bg-elevated)', borderRadius: '8px', border: '1px solid var(--border)' }}>
                    <div style={{ width: '8px', height: '8px', borderRadius: '50%', background: 'var(--text-muted)', flexShrink: 0 }} />
                    <span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-sm)', color: 'var(--text)', flex: 1, minWidth: 0, overflowWrap: 'anywhere' }}>{env.key}</span>
                    {env.required && (
                      <span style={{ fontSize: 'var(--fs-2xs)', padding: '1px 6px', background: 'var(--accent-subtle)', color: 'var(--accent)', border: '1px solid var(--accent-border)', borderRadius: '4px', fontFamily: 'var(--font-body)', fontWeight: 600 }}>
                        REQUIRED
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>

            <div className="card" style={{ padding: '20px 24px', marginTop: '16px' }}>
              <h3 className="card-title" style={{ marginBottom: '12px' }}>Webhook URL</h3>
              <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginBottom: '12px', lineHeight: 1.6 }}>
                Register this endpoint in your Stripe Dashboard.
              </p>
              <div style={{ padding: '12px 16px', background: 'var(--bg-elevated)', borderRadius: '8px', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', gap: '12px' }}>
                <span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-base)', color: 'var(--text)', flex: 1 }}>
                  POST /api/v1/webhooks/stripe
                </span>
                <span style={{ fontSize: 'var(--fs-2xs)', padding: '2px 8px', background: 'var(--info-bg)', color: 'var(--info)', border: '1px solid var(--info-border)', borderRadius: '4px', fontFamily: 'var(--font-body)', fontWeight: 600 }}>
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
