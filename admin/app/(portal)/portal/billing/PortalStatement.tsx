'use client'

import { useCallback, useEffect, useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { formatDateOnly } from '@/lib/dates'
import { fetchStatement, fetchUpcomingCharges, financeErrorMessage, formatCents, type Statement, type UpcomingCharges } from '@/lib/financeData'
import './PortalStatement.inline.css'

const headingStyle = { fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', margin: '0 0 16px' } as const
const rowStyle = { padding: '12px 20px', borderBottom: '1px solid var(--border)', fontSize: 'var(--fs-base)' } as const

/** Household account statement (charges, payments, credits, running balance) plus upcoming recurring charges. */
export function PortalStatement({ api, orgId, householdId }: { api: ApiClient; orgId: string; householdId: string }) {
  const [statement, setStatement] = useState<Statement | null>(null)
  const [upcoming, setUpcoming] = useState<UpcomingCharges | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    if (!orgId || !householdId) return
    setLoading(true)
    setError(null)
    try {
      const [s, u] = await Promise.all([fetchStatement(api, orgId, householdId), fetchUpcomingCharges(api, orgId, householdId)])
      setStatement(s)
      setUpcoming(u)
    } catch (err) {
      setError(financeErrorMessage(err, 'Your statement could not be loaded.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId, householdId])

  useEffect(() => {
    void load()
  }, [load])

  const lines = statement ? [...statement.lines].reverse() : []

  return (
    <>
      <section className="is-8dd12b4">
        <div className="is-347f2f9">
          <h2 style={headingStyle}>Account statement</h2>
          {statement ? (
            <div className="is-8fb176a">
              Balance <strong style={{ color: 'var(--text)', fontVariantNumeric: 'tabular-nums' }}>{formatCents(statement.closing_balance_cents)}</strong>
              {statement.credit_balance_cents > 0 ? <> · {formatCents(statement.credit_balance_cents)} account credit</> : null}
            </div>
          ) : null}
        </div>
        {error ? (
          <div role="alert" className="is-67f161c">
            <span>{error}</span>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => void load()}>
              Retry
            </button>
          </div>
        ) : (
          <div className="card is-f27afbb" >
            {loading ? (
              <div className="is-13c4a2f">Loading statement…</div>
            ) : lines.length === 0 ? (
              <div className="is-3d754c5">No charges or payments yet.</div>
            ) : (
              lines.map((line, i) => (
                <div key={i} className="ledger-row" style={rowStyle}>
                  <span style={{ color: 'var(--text-muted)' }}>{formatDateOnly(line.date, { month: 'short', day: 'numeric', year: 'numeric' })}</span>
                  <span style={{ color: line.informational ? 'var(--text-muted)' : 'var(--text)', minWidth: 0, overflowWrap: 'anywhere' }}>{line.description}</span>
                  <span style={{ fontVariantNumeric: 'tabular-nums', fontWeight: 600, color: line.informational ? 'var(--text-muted)' : line.amount_cents < 0 ? 'var(--success)' : 'var(--text)' }}>
                    {line.informational ? `(${formatCents(Math.abs(line.display_amount_cents))})` : formatCents(line.amount_cents)}
                  </span>
                  <span style={{ fontVariantNumeric: 'tabular-nums', color: 'var(--text-muted)', minWidth: 72, textAlign: 'right' }}>{formatCents(line.balance_cents)}</span>
                </div>
              ))
            )}
          </div>
        )}
      </section>

      {upcoming && upcoming.charges.length > 0 ? (
        <section className="is-8dd12b4">
          <h2 style={headingStyle}>Upcoming charges</h2>
          <div className="card is-f27afbb" >
            {upcoming.charges.map((c, i) => (
              <div key={i} className="is-98991f7">
                <div style={{ minWidth: 0 }}>
                  <div className="is-aaa1da3">
                    {c.schedule_name} for {c.swimmer_name}
                  </div>
                  <div className="is-a4e789a">
                    {formatDateOnly(c.period_start)} – {formatDateOnly(c.period_end)}
                    {c.discount_cents > 0 ? `, includes ${formatCents(c.discount_cents)} sibling discount` : ''}
                  </div>
                </div>
                <span className="is-e298452">{formatCents(c.net_cents)}</span>
              </div>
            ))}
            <div className="is-ecbf34d">
              Estimated through {formatDateOnly(upcoming.through)}: {formatCents(upcoming.total_cents)}.
              {upcoming.credit_balance_cents > 0 ? ` Your ${formatCents(upcoming.credit_balance_cents)} account credit applies automatically.` : ''}
            </div>
          </div>
        </section>
      ) : null}
    </>
  )
}
