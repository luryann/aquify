'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'

// ─── Types ─────────────────────────────────────────────────────────────────────

type WebhookEvent = {
  id: string
  stripeEventId: string
  type: string
  processedAt: string
  status: 'processed' | 'failed' | 'ignored'
  payload: Record<string, string>
}

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const RECONCILIATION = {
  ledgerTotal: 12850.00,
  stripeBalance: 12702.50,
  difference: 147.50,
  alertThresholdCents: 100,
  alertTriggered: true,
  lastRunAt: '2026-03-15T00:00:10Z',
  runningStatus: 'idle' as const,
}

const WEBHOOK_EVENTS: WebhookEvent[] = [
  { id: 'we1', stripeEventId: 'evt_1P2ABC', type: 'payment_intent.succeeded', processedAt: '2026-03-15T14:22:10Z', status: 'processed', payload: { amount: '28500', currency: 'usd', customer: 'cus_1ABC' } },
  { id: 'we2', stripeEventId: 'evt_1P2DEF', type: 'payment_intent.succeeded', processedAt: '2026-03-14T09:45:33Z', status: 'processed', payload: { amount: '19550', currency: 'usd', customer: 'cus_2DEF' } },
  { id: 'we3', stripeEventId: 'evt_1P2GHI', type: 'charge.refunded',          processedAt: '2026-03-13T16:10:00Z', status: 'processed', payload: { amount: '5000',  currency: 'usd', customer: 'cus_3GHI' } },
  { id: 'we4', stripeEventId: 'evt_1P2JKL', type: 'payment_intent.succeeded', processedAt: '2026-03-12T11:20:45Z', status: 'processed', payload: { amount: '42000', currency: 'usd', customer: 'cus_4JKL' } },
  { id: 'we5', stripeEventId: 'evt_1P2MNO', type: 'payment_intent.payment_failed', processedAt: '2026-03-11T08:55:12Z', status: 'processed', payload: { amount: '34000', currency: 'usd', customer: 'cus_5MNO' } },
  { id: 'we6', stripeEventId: 'evt_1P2PQR', type: 'payment_intent.succeeded', processedAt: '2026-03-10T17:30:22Z', status: 'failed',    payload: { amount: '17500', currency: 'usd', customer: 'cus_6PQR' } },
  { id: 'we7', stripeEventId: 'evt_1P2STU', type: 'checkout.session.completed', processedAt: '2026-03-09T13:15:00Z', status: 'ignored',  payload: { amount: '51000', currency: 'usd', customer: 'cus_7STU' } },
]

const EVENT_TYPE_COLORS: Record<string, string> = {
  'payment_intent.succeeded': 'var(--success)',
  'payment_intent.payment_failed': 'var(--error)',
  'charge.refunded': 'var(--warning)',
  'checkout.session.completed': 'var(--info)',
}

const STATUS_COLORS: Record<string, { bg: string; border: string; text: string }> = {
  processed: { bg: 'var(--success-bg)', border: 'var(--success-border)', text: 'var(--success)' },
  failed:    { bg: 'var(--error-bg)',   border: 'var(--error-border)',   text: 'var(--error)' },
  ignored:   { bg: 'var(--bg-elevated)', border: 'var(--border)',        text: 'var(--text-muted)' },
}

function fmtCents(c: number | string) {
  const n = typeof c === 'string' ? parseInt(c, 10) : c
  return `$${(n / 100).toLocaleString('en-US', { minimumFractionDigits: 2 })}`
}
function fmt$(n: number) { return `$${n.toLocaleString('en-US', { minimumFractionDigits: 2 })}` }
function fmtDt(d: string) { return new Date(d).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) }

const TABS = [
  { id: 'reconciliation', label: 'Reconciliation' },
  { id: 'webhooks',       label: 'Webhook Events', count: WEBHOOK_EVENTS.length },
  { id: 'config',         label: 'Configuration' },
]

export default function StripePage() {
  const [active, setActive] = useState('reconciliation')
  const activeTab = active
  const [running, setRunning] = useState(false)
  const [expandedEvent, setExpandedEvent] = useState<string | null>(null)

  async function handleRunReconcile() {
    setRunning(true)
    await new Promise((r) => setTimeout(r, 2000))
    setRunning(false)
  }

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1100px' }}>
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>

      {/* Header */}
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Billing · Stripe
        </p>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '12px' }}>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em' }}>
            Stripe Integration
          </h1>
          {/* Stripe-esque badge */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '6px 14px', background: 'rgba(99,91,255,0.08)', border: '1px solid rgba(99,91,255,0.2)', borderRadius: '20px' }}>
            <div style={{ width: '8px', height: '8px', borderRadius: '50%', background: '#635bff', boxShadow: '0 0 8px rgba(99,91,255,0.5)' }} />
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#635bff', fontWeight: 600, letterSpacing: '0.04em' }}>
              STRIPE CONNECTED
            </span>
          </div>
        </div>
      </div>

      <Tabs tabs={TABS} active={active} onChange={setActive} />

      <div style={{ marginTop: '24px' }}>
        {/* ── Reconciliation Tab ── */}
        {activeTab === 'reconciliation' && (
          <div>
            {/* Alert banner */}
            {RECONCILIATION.alertTriggered && (
              <div style={{
                padding: '14px 18px', marginBottom: '20px', borderRadius: '9px',
                background: 'var(--error-bg)', border: '1px solid var(--error-border)',
                display: 'flex', alignItems: 'flex-start', gap: '12px',
              }}>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--error)" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0, marginTop: '1px' }}>
                  <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
                  <line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" />
                </svg>
                <div>
                  <p style={{ fontSize: '13px', fontWeight: 600, color: 'var(--error)', marginBottom: '4px' }}>
                    Reconciliation mismatch exceeds threshold
                  </p>
                  <p style={{ fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.5 }}>
                    Difference of {fmt$(RECONCILIATION.difference)} exceeds the alert threshold of {fmtCents(RECONCILIATION.alertThresholdCents)}.
                    Review recent payments and refunds for discrepancies.
                  </p>
                </div>
              </div>
            )}

            {/* Reconciliation cards */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '16px', marginBottom: '24px' }}>
              {[
                { label: 'Ledger Total (Stripe)',   value: fmt$(RECONCILIATION.ledgerTotal),   sub: 'Sum of payment_succeeded events', color: 'var(--text)' },
                { label: 'Stripe Balance',          value: fmt$(RECONCILIATION.stripeBalance), sub: 'FetchBalance() from Stripe API',  color: 'var(--text)' },
                { label: 'Discrepancy',             value: fmt$(RECONCILIATION.difference),    sub: `Threshold: ${fmtCents(RECONCILIATION.alertThresholdCents)}`, color: RECONCILIATION.alertTriggered ? 'var(--error)' : 'var(--success)' },
              ].map((c) => (
                <div key={c.label} className="card-elevated" style={{ padding: '18px 20px' }}>
                  <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '8px' }}>{c.label}</p>
                  <p style={{ fontFamily: 'var(--font-mono)', fontSize: '22px', fontWeight: 700, color: c.color, marginBottom: '4px' }}>{c.value}</p>
                  <p style={{ fontSize: '11px', color: 'var(--text-muted)' }}>{c.sub}</p>
                </div>
              ))}
            </div>

            {/* Run details */}
            <div className="card" style={{ padding: '20px 24px' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
                <div>
                  <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>Last Reconciliation Run</h3>
                  <p style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                    {fmtDt(RECONCILIATION.lastRunAt)} · nightly job (00:00 UTC)
                  </p>
                </div>
                <button
                  onClick={handleRunReconcile}
                  disabled={running}
                  style={{
                    padding: '8px 16px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '7px',
                    color: 'var(--text-subtle)', fontSize: '12px', cursor: running ? 'not-allowed' : 'pointer',
                    fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '8px', transition: 'all 0.15s',
                  }}
                  onMouseEnter={(e) => { if (!running) { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' } }}
                  onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
                >
                  {running ? (
                    <><div style={{ width: '12px', height: '12px', borderRadius: '50%', border: '2px solid var(--border)', borderTopColor: 'var(--text-subtle)', animation: 'spin 0.8s linear infinite' }} />Running…</>
                  ) : (<><Icon name="refresh-cw" size={12} />Run Now</>)}
                </button>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {[
                  { label: 'Method',           value: 'payment_succeeded ledger events (method=stripe)' },
                  { label: 'Alert Threshold',  value: fmtCents(RECONCILIATION.alertThresholdCents) },
                  { label: 'Status',           value: RECONCILIATION.alertTriggered ? '⚠ ALERT TRIGGERED' : '✓ MATCH' },
                  { label: 'Schedule',         value: 'Every 24h (WORKER_RECONCILE_INTERVAL)' },
                ].map((r) => (
                  <div key={r.label} style={{ display: 'flex', gap: '16px' }}>
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', minWidth: '120px', flexShrink: 0 }}>{r.label}</span>
                    <span style={{ fontSize: '12px', color: r.label === 'Status' && RECONCILIATION.alertTriggered ? 'var(--error)' : 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>{r.value}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        {/* ── Webhooks Tab ── */}
        {activeTab === 'webhooks' && (
          <div>
            <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>
              Webhook endpoint: <span style={{ color: 'var(--text-subtle)' }}>POST /api/v1/webhooks/stripe</span> · HMAC-verified · idempotent
            </p>
            <div className="card" style={{ overflow: 'hidden' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid var(--border)' }}>
                    {['Stripe Event ID', 'Type', 'Status', 'Processed At', ''].map((h) => (
                      <th key={h} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                        {h}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {WEBHOOK_EVENTS.map((e) => {
                    const sc = STATUS_COLORS[e.status]
                    const tc = EVENT_TYPE_COLORS[e.type] ?? 'var(--text-subtle)'
                    return (
                      <>
                        <tr
                          key={e.id}
                          onClick={() => setExpandedEvent(expandedEvent === e.id ? null : e.id)}
                          style={{ borderBottom: expandedEvent === e.id ? 'none' : '1px solid var(--border)', cursor: 'pointer', transition: 'background 0.1s ease' }}
                          onMouseEnter={(ev) => { ev.currentTarget.style.background = 'var(--bg-surface)' }}
                          onMouseLeave={(ev) => { ev.currentTarget.style.background = 'transparent' }}
                        >
                          <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>
                            {e.stripeEventId}
                          </td>
                          <td style={{ padding: '11px 14px' }}>
                            <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: tc }}>{e.type}</span>
                          </td>
                          <td style={{ padding: '11px 14px' }}>
                            <span style={{ fontSize: '10px', padding: '2px 7px', borderRadius: '20px', fontFamily: 'var(--font-mono)', fontWeight: 600, letterSpacing: '0.04em', background: sc.bg, color: sc.text, border: `1px solid ${sc.border}` }}>
                              {e.status.toUpperCase()}
                            </span>
                          </td>
                          <td style={{ padding: '11px 14px', fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
                            {fmtDt(e.processedAt)}
                          </td>
                          <td style={{ padding: '11px 14px', textAlign: 'right' }}>
                            <Icon name={expandedEvent === e.id ? 'chevron-up' : 'chevron-down'} size={13} style={{ color: 'var(--text-muted)' }} />
                          </td>
                        </tr>
                        {expandedEvent === e.id && (
                          <tr key={`${e.id}-exp`} style={{ borderBottom: '1px solid var(--border)' }}>
                            <td colSpan={5} style={{ padding: '0 14px 14px 14px' }}>
                              <div style={{ background: 'var(--bg-elevated)', borderRadius: '7px', padding: '12px', fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-subtle)' }}>
                                {Object.entries(e.payload).map(([k, v]) => (
                                  <div key={k} style={{ display: 'flex', gap: '16px', marginBottom: '4px' }}>
                                    <span style={{ color: 'var(--text-muted)', minWidth: '80px' }}>{k}:</span>
                                    <span>{v}</span>
                                  </div>
                                ))}
                              </div>
                            </td>
                          </tr>
                        )}
                      </>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* ── Config Tab ── */}
        {activeTab === 'config' && (
          <div>
            <div className="card" style={{ padding: '24px' }}>
              <h3 style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)', marginBottom: '16px' }}>Stripe Environment Variables</h3>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {[
                  { key: 'STRIPE_SECRET_KEY',            value: 'sk_live_●●●●●●●●●●●●●●●●●●●●',    required: true,  set: true },
                  { key: 'STRIPE_WEBHOOK_SECRET',        value: 'whsec_●●●●●●●●●●●●●●●●●●●●',       required: true,  set: true },
                  { key: 'STRIPE_CURRENCY',              value: 'usd',                               required: false, set: true },
                  { key: 'STRIPE_RECONCILIATION_ALERT_THRESHOLD_CENTS', value: '100',                required: false, set: true },
                ].map((env) => (
                  <div key={env.key} style={{ display: 'flex', alignItems: 'center', gap: '12px', padding: '10px 14px', background: 'var(--bg-elevated)', borderRadius: '7px', border: '1px solid var(--border)' }}>
                    <div style={{ width: '8px', height: '8px', borderRadius: '50%', background: env.set ? 'var(--success)' : 'var(--error)', flexShrink: 0 }} />
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text)', flex: 1 }}>{env.key}</span>
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{env.value}</span>
                    {env.required && (
                      <span style={{ fontSize: '10px', padding: '1px 6px', background: 'var(--accent-subtle)', color: 'var(--accent)', border: '1px solid var(--accent-border)', borderRadius: '4px', fontFamily: 'var(--font-mono)', fontWeight: 600 }}>
                        REQUIRED
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>

            <div className="card" style={{ padding: '20px 24px', marginTop: '16px' }}>
              <h3 style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)', marginBottom: '12px' }}>Webhook Endpoint</h3>
              <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '12px', lineHeight: 1.6 }}>
                Register this endpoint in your Stripe Dashboard to receive events. HMAC signature verification is enforced on every request.
              </p>
              <div style={{ padding: '10px 14px', background: 'var(--bg-elevated)', borderRadius: '7px', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', gap: '12px' }}>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text)', flex: 1 }}>
                  POST /api/v1/webhooks/stripe
                </span>
                <span style={{ fontSize: '10px', padding: '2px 7px', background: 'var(--info-bg)', color: 'var(--info)', border: '1px solid var(--info-border)', borderRadius: '4px', fontFamily: 'var(--font-mono)', fontWeight: 600 }}>
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
