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
import { localToday } from '@/lib/wellness-scale'
import { Banner, Field, Money, ScrollTable, numStyle, sectionTitle, tableStyle, tdStyle, thStyle, useFinanceContext } from '../financeUi'

// Sequential ramp (one hue, light → dark) from the brand accent: older debt reads heavier.
const AGING_RAMP = ['#f4b48a', '#ee8546', '#eb5d1e', '#b8420f', '#7a2a08']

export default function FinanceReportsPage() {
  const { api, orgId, token } = useFinanceContext()
  const now = new Date()
  const [asOf, setAsOf] = useState(localToday(now))
  const [from, setFrom] = useState(localToday(new Date(now.getFullYear(), now.getMonth() - 11, 1)))
  const [to, setTo] = useState(localToday(now))
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

      <section className="card u-p-20px" >
        <div className="u-gap-12px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', flexWrap: 'wrap' }}>
          <div>
            <h2 style={sectionTitle}>Receivables aging</h2>
            {aging ? (
              <p className="u-m-0 u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>
                <Money cents={aging.totals.total_cents} strong /> open across {aging.rows.length} {aging.rows.length === 1 ? 'household' : 'households'}
              </p>
            ) : null}
          </div>
          <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'flex-end', flexWrap: 'wrap' }}>
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
            <div role="img" aria-label="Open receivables by age" className="u-gap-2px" style={{ display: 'flex', height: 28, margin: '20px 0 12px', borderRadius: 4, overflow: 'hidden' }}>
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
            <ul className="u-p-0 u-m-0-0-16px u-fs-fs-base" style={{ display: 'flex', flexWrap: 'wrap', gap: '6px 20px', listStyle: 'none' }}>
              {AGING_BUCKETS.map((b, i) => (
                <li key={b.key} className="u-gap-8px" style={{ display: 'flex', alignItems: 'center' }}>
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
                        <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
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
          <p className="u-fs-fs-base u-mt-16px" style={{ color: 'var(--text-subtle)' }}>Nothing is outstanding. Every issued invoice is paid, voided, or written off.</p>
        ) : null}
      </section>

      <section className="card u-p-20px u-mt-20px" >
        <div className="u-gap-12px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', flexWrap: 'wrap' }}>
          <div>
            <h2 style={sectionTitle}>Invoiced revenue</h2>
            {revenue ? (
              <p className="u-m-0 u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>
                <Money cents={revenue.total_cents} strong /> issued {formatDateOnly(revenue.from)} – {formatDateOnly(revenue.to)}, net of discounts
              </p>
            ) : null}
          </div>
          <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'flex-end', flexWrap: 'wrap' }}>
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
          <div className="u-mt-20px" style={{ position: 'relative' }}>
            <div className="u-fs-fs-base" style={{ height: 18, color: 'var(--text-subtle)' }} aria-live="polite">
              {hovered ? (
                <>
                  {hovered.label}: <Money cents={hovered.amount_cents} strong />
                </>
              ) : (
                'Hover a month for its total'
              )}
            </div>
            <div className="u-gap-2px u-mt-8px" style={{ display: 'flex', alignItems: 'flex-end', height: 160, borderBottom: '1px solid var(--border-strong)' }}>
              {revenue.by_month.map((m) => (
                <button
                  key={m.key}
                  type="button"
                  aria-label={`${m.label}: ${formatCents(m.amount_cents)}`}
                  onMouseEnter={() => setHoverMonth(m.key)}
                  onMouseLeave={() => setHoverMonth(null)}
                  onFocus={() => setHoverMonth(m.key)}
                  onBlur={() => setHoverMonth(null)}
                  className="u-p-0" style={{ flex: 1, height: '100%', display: 'flex', alignItems: 'flex-end', background: 'none', border: 'none', cursor: 'default' }}
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
            <div className="u-gap-2px u-fs-fs-xs u-mt-6px" style={{ display: 'flex', color: 'var(--text-muted)' }}>
              {revenue.by_month.map((m, i) => (
                <span key={m.key} style={{ flex: 1, textAlign: 'center', overflow: 'hidden', whiteSpace: 'nowrap' }}>
                  {i % Math.ceil(revenue.by_month.length / 6) === 0 ? m.label.slice(0, 3) : ''}
                </span>
              ))}
            </div>
            <div className="u-gap-24px u-mt-20px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 280px), 1fr))' }}>
              {[
                { title: 'By category', rows: revenue.by_category },
                { title: 'By group', rows: revenue.by_group },
              ].map((block) => (
                <div key={block.title}>
                  <h3 className="card-title u-m-0-0-6px" >{block.title}</h3>
                  {block.rows.length === 0 ? <p className="u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>No revenue in this range.</p> : null}
                  {block.rows.map((row) => (
                    <div key={row.key} className="u-fs-fs-base" style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--border)' }}>
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

      <section className="card u-p-20px u-mt-20px" >
        <div className="u-gap-12px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', flexWrap: 'wrap' }}>
          <div>
            <h2 style={sectionTitle}>Deposits</h2>
            {deposits ? (
              <p className="u-m-0 u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>
                <Money cents={deposits.net_cents} strong /> net after <Money cents={deposits.refunds_cents} muted /> in refunds
              </p>
            ) : null}
          </div>
          <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'flex-end', flexWrap: 'wrap' }}>
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
          <p className="u-fs-fs-base u-mt-16px" style={{ color: 'var(--text-subtle)' }}>No payments were received in this range.</p>
        ) : null}
      </section>
    </div>
  )
}
