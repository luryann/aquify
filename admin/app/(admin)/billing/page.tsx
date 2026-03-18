'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Icon } from '@/components/ui/icons'

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const STATS = {
  totalReceivables: 48230,
  collected30d: 12850,
  outstanding: 8740,
  overdue: 3120,
  invoiceCount: 184,
  paidCount: 141,
  openCount: 29,
  overdueCount: 14,
  stripeBalance: 12702,
  lastReconciled: '2026-03-15T00:00:10Z',
  reconciliationMatch: true,
}

type RecentPayment = {
  id: string
  household: string
  amount: number
  method: 'stripe' | 'check' | 'cash'
  date: string
  invoiceId: string
}

const RECENT_PAYMENTS: RecentPayment[] = [
  { id: 'p1', household: 'Allen Family',     amount: 285.00, method: 'stripe', date: '2026-03-15', invoiceId: 'inv-001' },
  { id: 'p2', household: 'Reyes Household',  amount: 195.50, method: 'stripe', date: '2026-03-14', invoiceId: 'inv-002' },
  { id: 'p3', household: 'Torres Family',    amount: 420.00, method: 'check',  date: '2026-03-13', invoiceId: 'inv-003' },
  { id: 'p4', household: 'Park Family',      amount: 340.00, method: 'stripe', date: '2026-03-12', invoiceId: 'inv-004' },
  { id: 'p5', household: 'Williams Family',  amount: 175.00, method: 'cash',   date: '2026-03-11', invoiceId: 'inv-005' },
  { id: 'p6', household: 'Johnson Household',amount: 510.00, method: 'stripe', date: '2026-03-10', invoiceId: 'inv-006' },
]

type OverdueInvoice = {
  id: string
  household: string
  amount: number
  daysOverdue: number
  invoiceId: string
}

const OVERDUE: OverdueInvoice[] = [
  { id: 'o1', household: 'Martinez Family', amount: 890.00, daysOverdue: 47, invoiceId: 'inv-101' },
  { id: 'o2', household: 'Brown Household', amount: 340.00, daysOverdue: 31, invoiceId: 'inv-102' },
  { id: 'o3', household: 'Davis Family',    amount: 520.00, daysOverdue: 18, invoiceId: 'inv-103' },
  { id: 'o4', household: 'Wilson Family',   amount: 395.00, daysOverdue: 12, invoiceId: 'inv-104' },
  { id: 'o5', household: 'Lee Household',   amount: 975.00, daysOverdue: 8,  invoiceId: 'inv-105' },
]

function fmt$(n: number) { return `$${n.toLocaleString('en-US', { minimumFractionDigits: 2 })}` }
const fmtDate = (d: string) => new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })

export default function BillingOverviewPage() {
  const router = useRouter()
  const [runningReconcile, setRunningReconcile] = useState(false)

  async function handleReconcile() {
    setRunningReconcile(true)
    await new Promise((r) => setTimeout(r, 1800))
    setRunningReconcile(false)
  }

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1200px' }}>
      {/* Header */}
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Billing
        </p>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '12px' }}>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em' }}>
            Billing Overview
          </h1>
          <div style={{ display: 'flex', gap: '10px' }}>
            <button
              onClick={handleReconcile}
              disabled={runningReconcile}
              style={{
                padding: '8px 16px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px',
                color: 'var(--text-subtle)', fontSize: '12px', cursor: runningReconcile ? 'not-allowed' : 'pointer',
                fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '7px',
                transition: 'all 0.15s ease',
              }}
              onMouseEnter={(e) => { if (!runningReconcile) { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' } }}
              onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
            >
              {runningReconcile ? (
                <>
                  <div style={{ width: '12px', height: '12px', borderRadius: '50%', border: '2px solid var(--border)', borderTopColor: 'var(--text-subtle)', animation: 'spin 0.8s linear infinite' }} />
                  Reconciling…
                </>
              ) : (
                <>
                  <Icon name="refresh-cw" size={12} />
                  Run Reconciliation
                </>
              )}
            </button>
            <button
              onClick={() => router.push('/invoices')}
              style={{
                padding: '8px 16px', background: 'var(--accent)', border: 'none', borderRadius: '7px',
                color: '#fff', fontSize: '12px', fontWeight: 600, cursor: 'pointer',
                fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '7px',
              }}
              onMouseEnter={(e) => { e.currentTarget.style.background = '#d44f14' }}
              onMouseLeave={(e) => { e.currentTarget.style.background = 'var(--accent)' }}
            >
              <Icon name="file-text" size={12} />
              All Invoices
            </button>
          </div>
        </div>
      </div>

      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>

      {/* Reconciliation status banner */}
      <div style={{
        padding: '12px 16px', marginBottom: '24px', borderRadius: '9px',
        background: STATS.reconciliationMatch ? 'var(--success-bg)' : 'var(--error-bg)',
        border: `1px solid ${STATS.reconciliationMatch ? 'var(--success-border)' : 'var(--error-border)'}`,
        display: 'flex', alignItems: 'center', gap: '12px',
      }}>
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none"
          stroke={STATS.reconciliationMatch ? 'var(--success)' : 'var(--error)'}
          strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
          {STATS.reconciliationMatch
            ? <><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" /><polyline points="22 4 12 14.01 9 11.01" /></>
            : <><circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" /></>
          }
        </svg>
        <div style={{ flex: 1 }}>
          <span style={{ fontSize: '13px', fontWeight: 500, color: STATS.reconciliationMatch ? 'var(--success)' : 'var(--error)' }}>
            {STATS.reconciliationMatch ? 'Stripe reconciliation matched' : 'Reconciliation mismatch detected'}
          </span>
          <span style={{ fontSize: '12px', color: 'var(--text-muted)', marginLeft: '12px', fontFamily: 'var(--font-mono)' }}>
            Last run: {fmtDate(STATS.lastReconciled)}
          </span>
        </div>
        <button
          onClick={() => router.push('/billing/stripe')}
          style={{ padding: '4px 12px', background: 'none', border: `1px solid ${STATS.reconciliationMatch ? 'var(--success-border)' : 'var(--error-border)'}`, borderRadius: '5px', color: STATS.reconciliationMatch ? 'var(--success)' : 'var(--error)', fontSize: '11px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}
        >
          View Stripe
        </button>
      </div>

      {/* KPI grid */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '14px', marginBottom: '28px' }}>
        {[
          { label: 'Total Receivables', value: fmt$(STATS.totalReceivables), sub: `${STATS.invoiceCount} invoices`, accent: false },
          { label: 'Collected (30d)',    value: fmt$(STATS.collected30d),   sub: `${STATS.paidCount} paid`,     accent: true },
          { label: 'Outstanding',       value: fmt$(STATS.outstanding),    sub: `${STATS.openCount} open`,     accent: false },
          { label: 'Overdue',           value: fmt$(STATS.overdue),        sub: `${STATS.overdueCount} invoices`, accent: false, warn: true },
        ].map((k) => (
          <div
            key={k.label}
            className="card-elevated"
            style={{
              padding: '18px 20px',
              borderLeft: k.accent ? '3px solid var(--success)' : k.warn ? '3px solid var(--error)' : '3px solid transparent',
            }}
          >
            <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '8px' }}>
              {k.label}
            </p>
            <p style={{ fontFamily: 'var(--font-display)', fontSize: '22px', color: k.warn ? 'var(--error)' : 'var(--text)', lineHeight: 1, marginBottom: '4px' }}>
              {k.value}
            </p>
            <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
              {k.sub}
            </p>
          </div>
        ))}
      </div>

      {/* Two-column: Recent Payments + Overdue */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '20px' }}>
        {/* Recent Payments */}
        <div className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '16px 18px', borderBottom: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>Recent Payments</h3>
            <button
              onClick={() => router.push('/ledger')}
              style={{ background: 'none', border: 'none', fontSize: '11px', color: 'var(--accent)', cursor: 'pointer', fontFamily: 'var(--font-body)' }}
            >
              View Ledger →
            </button>
          </div>
          <div>
            {RECENT_PAYMENTS.map((p) => (
              <div
                key={p.id}
                style={{
                  padding: '12px 18px', borderBottom: '1px solid var(--border)',
                  display: 'flex', alignItems: 'center', gap: '12px',
                  transition: 'background 0.1s ease', cursor: 'pointer',
                }}
                onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--bg-surface)' }}
                onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                onClick={() => router.push(`/invoices/${p.invoiceId}`)}
              >
                <div style={{
                  width: '32px', height: '32px', borderRadius: '8px', flexShrink: 0,
                  background: p.method === 'stripe' ? 'rgba(99,102,241,0.12)' : 'var(--bg-elevated)',
                  border: '1px solid var(--border)',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                }}>
                  {p.method === 'stripe' ? (
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#818cf8" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                      <rect x="1" y="4" width="22" height="16" rx="2" ry="2" /><line x1="1" y1="10" x2="23" y2="10" />
                    </svg>
                  ) : (
                    <Icon name="dollar-sign" size={12} style={{ color: 'var(--text-muted)' }} />
                  )}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <p style={{ fontSize: '13px', color: 'var(--text)', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {p.household}
                  </p>
                  <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                    {fmtDate(p.date)} · {p.method}
                  </p>
                </div>
                <span style={{ fontSize: '14px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--success)', flexShrink: 0 }}>
                  +{fmt$(p.amount)}
                </span>
              </div>
            ))}
          </div>
        </div>

        {/* Overdue Invoices */}
        <div className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '16px 18px', borderBottom: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>
              Overdue Invoices
              <span style={{ marginLeft: '8px', fontSize: '11px', padding: '2px 7px', borderRadius: '20px', background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error-border)', fontFamily: 'var(--font-mono)', fontWeight: 700 }}>
                {OVERDUE.length}
              </span>
            </h3>
            <button
              onClick={() => router.push('/invoices?status=overdue')}
              style={{ background: 'none', border: 'none', fontSize: '11px', color: 'var(--accent)', cursor: 'pointer', fontFamily: 'var(--font-body)' }}
            >
              View All →
            </button>
          </div>
          <div>
            {OVERDUE.map((o) => (
              <div
                key={o.id}
                style={{
                  padding: '12px 18px', borderBottom: '1px solid var(--border)',
                  display: 'flex', alignItems: 'center', gap: '12px',
                  transition: 'background 0.1s ease', cursor: 'pointer',
                }}
                onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--bg-surface)' }}
                onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                onClick={() => router.push(`/invoices/${o.invoiceId}`)}
              >
                <div style={{ flex: 1, minWidth: 0 }}>
                  <p style={{ fontSize: '13px', color: 'var(--text)', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {o.household}
                  </p>
                  <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--error)' }}>
                    {o.daysOverdue}d overdue
                  </p>
                </div>
                <span style={{ fontSize: '14px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--text)', flexShrink: 0 }}>
                  {fmt$(o.amount)}
                </span>
                <span style={{
                  fontSize: '10px', padding: '2px 7px', borderRadius: '20px',
                  background: o.daysOverdue > 30 ? 'var(--error-bg)' : 'var(--warning-bg)',
                  color: o.daysOverdue > 30 ? 'var(--error)' : 'var(--warning)',
                  border: `1px solid ${o.daysOverdue > 30 ? 'var(--error-border)' : 'var(--warning-border)'}`,
                  fontFamily: 'var(--font-mono)', fontWeight: 600, flexShrink: 0,
                }}>
                  {o.daysOverdue > 30 ? 'CRITICAL' : 'OVERDUE'}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Invoice state distribution */}
      <div className="card-elevated" style={{ padding: '20px 24px', marginTop: '20px' }}>
        <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '16px' }}>Invoice State Distribution</h3>
        <div style={{ display: 'flex', gap: '24px', flexWrap: 'wrap' }}>
          {[
            { label: 'Paid',         count: 141, color: 'var(--success)' },
            { label: 'Issued',       count: 17,  color: 'var(--info)' },
            { label: 'Partially Paid', count: 12, color: 'var(--warning)' },
            { label: 'Overdue',      count: 14,  color: 'var(--error)' },
            { label: 'Draft',        count: 8,   color: 'var(--text-muted)' },
            { label: 'Voided',       count: 3,   color: 'var(--text-disabled)' },
          ].map((s) => (
            <div key={s.label} style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <div style={{ width: '8px', height: '8px', borderRadius: '50%', background: s.color, flexShrink: 0 }} />
              <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{s.label}</span>
              <span style={{ fontSize: '14px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--text)' }}>{s.count}</span>
            </div>
          ))}
        </div>
        {/* Bar */}
        <div style={{ marginTop: '14px', height: '8px', borderRadius: '4px', overflow: 'hidden', display: 'flex', background: 'var(--bg-elevated)' }}>
          {[{ w: 141, c: 'var(--success)' }, { w: 17, c: 'var(--info)' }, { w: 12, c: 'var(--warning)' }, { w: 14, c: 'var(--error)' }, { w: 8, c: 'var(--text-muted)' }, { w: 3, c: 'var(--text-disabled)' }].map((b, i) => (
            <div key={i} style={{ width: `${(b.w / 195) * 100}%`, background: b.c, transition: 'width 0.3s ease' }} />
          ))}
        </div>
      </div>
    </div>
  )
}
