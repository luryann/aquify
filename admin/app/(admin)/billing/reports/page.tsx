'use client'

import { useCallback, useEffect, useState } from 'react'
import {
  AGING_BUCKETS,
  agingShares,
  downloadFinanceCsv,
  fetchAgingReport,
  fetchDepositsReport,
  fetchRevenueReport,
  financeErrorMessage,
  formatCents,
  type AgingReport,
  type DepositsReport,
  type RevenueReport,
} from '@/lib/financeData'
import { formatDateOnly } from '@/lib/dates'
import { Banner, Field, Money, ScrollTable, numStyle, sectionTitle, tableStyle, tdStyle, thStyle, useFinanceContext } from '../financeUi'

// Sequential ramp (one hue, light → dark) from the brand accent: older debt reads heavier.
const AGING_RAMP = ['#f4b48a', '#ee8546', '#eb5d1e', '#b8420f', '#7a2a08']

function iso(d: Date) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export default function FinanceReportsPage() {
  const { api, orgId, token } = useFinanceContext()
  const now = new Date()
  const [asOf, setAsOf] = useState(iso(now))
  const [from, setFrom] = useState(iso(new Date(now.getFullYear(), now.getMonth() - 11, 1)))
  const [to, setTo] = useState(iso(now))
  const [period, setPeriod] = useState<'day' | 'week' | 'month'>('month')
  const [aging, setAging] = useState<AgingReport | null>(null)
  const [revenue, setRevenue] = useState<RevenueReport | null>(null)
  const [deposits, setDeposits] = useState<DepositsReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [hoverMonth, setHoverMonth] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!orgId) return
    setLoading(true)
    setError(null)
    try {
      const [a, r, d] = await Promise.all([
        fetchAgingReport(api, orgId, asOf),
        fetchRevenueReport(api, orgId, from, to),
        fetchDepositsReport(api, orgId, from, to, period),
      ])
      setAging(a)
      setRevenue(r)
      setDeposits(d)
    } catch (err) {
      setError(financeErrorMessage(err, 'Finance reports could not be loaded.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId, asOf, from, to, period])

  useEffect(() => {
    void load()
  }, [load])

  async function download(path: string, filename: string) {
    try {
      await downloadFinanceCsv(token, orgId, path, filename)
    } catch (err) {
      setError(financeErrorMessage(err, 'The export could not be downloaded.'))
    }
  }

  const shares = aging ? agingShares(aging.totals) : []
  const maxMonth = Math.max(1, ...(revenue?.by_month.map((m) => m.amount_cents) ?? [0]))
  const hovered = revenue?.by_month.find((m) => m.key === hoverMonth)

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Finance reports</h1>
        </div>
      </div>
      {error ? <Banner tone="error" onRetry={() => void load()}>{error}</Banner> : null}

      <section className="card" style={{ padding: 20 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 12, flexWrap: 'wrap' }}>
          <div>
            <h2 style={sectionTitle}>Receivables aging</h2>
            {aging ? (
              <p style={{ margin: 0, fontSize: 'var(--fs-base)', color: 'var(--text-subtle)' }}>
                <Money cents={aging.totals.total_cents} strong /> open across {aging.rows.length} {aging.rows.length === 1 ? 'household' : 'households'}
              </p>
            ) : null}
          </div>
          <div style={{ display: 'flex', gap: 8, alignItems: 'flex-end', flexWrap: 'wrap' }}>
            <Field label="Aged as of">
              <input className="form-input" type="date" value={asOf} onChange={(e) => setAsOf(e.target.value)} />
            </Field>
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => void download(`/api/v1/billing/reports/aging?as_of=${asOf}&format=csv`, `ar-aging-${asOf}.csv`)}>
              Export aging CSV
            </button>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => void download('/api/v1/billing/reports/outstanding?format=csv', 'outstanding-balances.csv')}>
              Export balances CSV
            </button>
          </div>
        </div>

        {aging && aging.totals.total_cents > 0 ? (
          <>
            <div role="img" aria-label="Open receivables by age" style={{ display: 'flex', gap: 2, height: 28, margin: '20px 0 12px', borderRadius: 4, overflow: 'hidden' }}>
              {AGING_BUCKETS.map((b, i) =>
                shares[i] > 0 ? (
                  <div
                    key={b.key}
                    title={`${b.label}: ${formatCents(aging.totals[b.key])}`}
                    style={{ flex: `${shares[i]} 0 0`, minWidth: 4, background: AGING_RAMP[i] }}
                  />
                ) : null,
              )}
            </div>
            <ul style={{ display: 'flex', flexWrap: 'wrap', gap: '6px 20px', listStyle: 'none', padding: 0, margin: '0 0 16px', fontSize: 'var(--fs-base)' }}>
              {AGING_BUCKETS.map((b, i) => (
                <li key={b.key} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <span aria-hidden style={{ width: 10, height: 10, borderRadius: 4, background: AGING_RAMP[i] }} />
                  <span style={{ color: 'var(--text-subtle)' }}>{b.label}</span>
                  <Money cents={aging.totals[b.key]} />
                </li>
              ))}
            </ul>
            <ScrollTable>
              <table style={tableStyle}>
                <thead>
                  <tr>
                    <th style={thStyle}>Household</th>
                    {AGING_BUCKETS.map((b) => (
                      <th key={b.key} style={{ ...thStyle, textAlign: 'right' }}>{b.label}</th>
                    ))}
                    <th style={{ ...thStyle, textAlign: 'right' }}>Total</th>
                  </tr>
                </thead>
                <tbody>
                  {aging.rows.map((row) => (
                    <tr key={row.household_id || 'none'}>
                      <td style={tdStyle}>
                        {row.household_name}
                        <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>
                          {row.invoice_count} open{row.oldest_due_date ? `, oldest due ${formatDateOnly(row.oldest_due_date)}` : ''}
                        </div>
                      </td>
                      {AGING_BUCKETS.map((b) => (
                        <td key={b.key} style={numStyle}>{row[b.key] ? <Money cents={row[b.key]} /> : <span style={{ color: 'var(--text-muted)' }}>—</span>}</td>
                      ))}
                      <td style={numStyle}><Money cents={row.total_cents} strong /></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </ScrollTable>
          </>
        ) : !loading && aging ? (
          <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', marginTop: 16 }}>Nothing is outstanding. Every issued invoice is paid, voided, or written off.</p>
        ) : null}
      </section>

      <section className="card" style={{ padding: 20, marginTop: 20 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 12, flexWrap: 'wrap' }}>
          <div>
            <h2 style={sectionTitle}>Invoiced revenue</h2>
            {revenue ? (
              <p style={{ margin: 0, fontSize: 'var(--fs-base)', color: 'var(--text-subtle)' }}>
                <Money cents={revenue.total_cents} strong /> issued {formatDateOnly(revenue.from)} – {formatDateOnly(revenue.to)}, net of discounts
              </p>
            ) : null}
          </div>
          <div style={{ display: 'flex', gap: 8, alignItems: 'flex-end', flexWrap: 'wrap' }}>
            <Field label="From">
              <input className="form-input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
            </Field>
            <Field label="To">
              <input className="form-input" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
            </Field>
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => void download(`/api/v1/billing/reports/revenue?from=${from}&to=${to}&format=csv`, `revenue-${from}-${to}.csv`)}>
              Export revenue CSV
            </button>
          </div>
        </div>

        {revenue && revenue.by_month.length > 0 ? (
          <div style={{ position: 'relative', marginTop: 20 }}>
            <div style={{ height: 18, fontSize: 'var(--fs-base)', color: 'var(--text-subtle)' }} aria-live="polite">
              {hovered ? (
                <>
                  {hovered.label}: <Money cents={hovered.amount_cents} strong />
                </>
              ) : (
                'Hover a month for its total'
              )}
            </div>
            <div style={{ display: 'flex', alignItems: 'flex-end', gap: 2, height: 160, borderBottom: '1px solid var(--border-strong)', marginTop: 8 }}>
              {revenue.by_month.map((m) => (
                <button
                  key={m.key}
                  type="button"
                  aria-label={`${m.label}: ${formatCents(m.amount_cents)}`}
                  onMouseEnter={() => setHoverMonth(m.key)}
                  onMouseLeave={() => setHoverMonth(null)}
                  onFocus={() => setHoverMonth(m.key)}
                  onBlur={() => setHoverMonth(null)}
                  style={{ flex: 1, height: '100%', display: 'flex', alignItems: 'flex-end', background: 'none', border: 'none', padding: 0, cursor: 'default' }}
                >
                  <span
                    style={{
                      display: 'block',
                      width: '100%',
                      height: `${Math.max(0, (m.amount_cents / maxMonth) * 100)}%`,
                      minHeight: m.amount_cents > 0 ? 2 : 0,
                      background: hoverMonth === m.key ? 'var(--accent-hover)' : 'var(--accent)',
                      borderRadius: '4px 4px 0 0',
                    }}
                  />
                </button>
              ))}
            </div>
            <div style={{ display: 'flex', gap: 2, fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', marginTop: 6 }}>
              {revenue.by_month.map((m, i) => (
                <span key={m.key} style={{ flex: 1, textAlign: 'center', overflow: 'hidden', whiteSpace: 'nowrap' }}>
                  {i % Math.ceil(revenue.by_month.length / 6) === 0 ? m.label.slice(0, 3) : ''}
                </span>
              ))}
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 280px), 1fr))', gap: 24, marginTop: 20 }}>
              {[
                { title: 'By category', rows: revenue.by_category },
                { title: 'By group', rows: revenue.by_group },
              ].map((block) => (
                <div key={block.title}>
                  <h3 className="card-title" style={{ margin: '0 0 6px' }}>{block.title}</h3>
                  {block.rows.length === 0 ? <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)' }}>No revenue in this range.</p> : null}
                  {block.rows.map((row) => (
                    <div key={row.key} style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--border)', fontSize: 'var(--fs-base)' }}>
                      <span>{row.label}</span>
                      <Money cents={row.amount_cents} />
                    </div>
                  ))}
                </div>
              ))}
            </div>
          </div>
        ) : null}
      </section>

      <section className="card" style={{ padding: 20, marginTop: 20 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 12, flexWrap: 'wrap' }}>
          <div>
            <h2 style={sectionTitle}>Deposits</h2>
            {deposits ? (
              <p style={{ margin: 0, fontSize: 'var(--fs-base)', color: 'var(--text-subtle)' }}>
                <Money cents={deposits.net_cents} strong /> net after <Money cents={deposits.refunds_cents} muted /> in refunds
              </p>
            ) : null}
          </div>
          <div style={{ display: 'flex', gap: 8, alignItems: 'flex-end', flexWrap: 'wrap' }}>
            <Field label="Group by">
              <select className="form-input" value={period} onChange={(e) => setPeriod(e.target.value as 'day' | 'week' | 'month')}>
                <option value="day">Day</option>
                <option value="week">Week</option>
                <option value="month">Month</option>
              </select>
            </Field>
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => void download(`/api/v1/billing/reports/deposits?from=${from}&to=${to}&period=${period}&format=csv`, `deposits-${from}-${to}.csv`)}>
              Export deposits CSV
            </button>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => void download(`/api/v1/billing/reports/quickbooks?kind=invoices&from=${from}&to=${to}`, `quickbooks-invoices-${from}-${to}.csv`)}>
              QuickBooks invoices
            </button>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => void download(`/api/v1/billing/reports/quickbooks?kind=payments&from=${from}&to=${to}`, `quickbooks-payments-${from}-${to}.csv`)}>
              QuickBooks payments
            </button>
          </div>
        </div>
        {deposits && deposits.rows.length > 0 ? (
          <ScrollTable>
            <table style={{ ...tableStyle, marginTop: 16 }}>
              <thead>
                <tr>
                  <th style={thStyle}>Period</th>
                  <th style={{ ...thStyle, textAlign: 'right' }}>Payments</th>
                  <th style={{ ...thStyle, textAlign: 'right' }}>Received</th>
                  <th style={{ ...thStyle, textAlign: 'right' }}>Refunded</th>
                  <th style={{ ...thStyle, textAlign: 'right' }}>Net deposit</th>
                </tr>
              </thead>
              <tbody>
                {deposits.rows.map((row) => (
                  <tr key={row.period_start}>
                    <td style={tdStyle}>{row.period_start === row.period_end ? formatDateOnly(row.period_start) : `${formatDateOnly(row.period_start)} – ${formatDateOnly(row.period_end)}`}</td>
                    <td style={numStyle}>{row.payment_count}</td>
                    <td style={numStyle}><Money cents={row.gross_cents} /></td>
                    <td style={numStyle}>{row.refunds_cents ? <Money cents={row.refunds_cents} muted /> : '—'}</td>
                    <td style={numStyle}><Money cents={row.net_cents} strong /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </ScrollTable>
        ) : !loading ? (
          <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', marginTop: 16 }}>No payments were received in this range.</p>
        ) : null}
      </section>
    </div>
  )
}
