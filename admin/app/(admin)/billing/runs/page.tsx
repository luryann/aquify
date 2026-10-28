'use client'

import Link from 'next/link'
import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
import {
  commitBillingRun,
  fetchBillingRuns,
  fetchFeeSchedules,
  financeErrorMessage,
  previewBillingRun,
  type BillingRun,
  type FeeSchedule,
  type RunInput,
  type RunPreview,
  type RunResult,
} from '@/lib/financeData'
import { formatDateOnly } from '@/lib/dates'
import { useStaffNames } from '@/lib/useStaffNames'
import { Banner, Field, Money, ScrollTable, formGrid, numStyle, sectionTitle, tableStyle, tdStyle, thStyle, useFinanceContext } from '../financeUi'

function todayISO() {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export default function BillingRunsPage() {
  const { api, orgId } = useFinanceContext()
  const staffName = useStaffNames(api, orgId)
  const [schedules, setSchedules] = useState<FeeSchedule[]>([])
  const [runs, setRuns] = useState<BillingRun[]>([])
  const [loadError, setLoadError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [asOf, setAsOf] = useState(todayISO())
  const [status, setStatus] = useState<'draft' | 'issued'>('draft')
  const [dueDate, setDueDate] = useState('')
  const [selected, setSelected] = useState<string[]>([])
  const [includeAdjustments, setIncludeAdjustments] = useState(true)
  const [preview, setPreview] = useState<RunPreview | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)
  const [result, setResult] = useState<RunResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    if (!orgId) return
    setLoading(true)
    setLoadError(null)
    try {
      const [s, r] = await Promise.all([fetchFeeSchedules(api, orgId), fetchBillingRuns(api, orgId)])
      setSchedules(s.filter((x) => x.active))
      setRuns(r)
    } catch (err) {
      setLoadError(financeErrorMessage(err, 'Billing runs could not be loaded.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId])

  useEffect(() => {
    void load()
  }, [load])

  const input: RunInput = useMemo(
    () => ({ as_of: asOf, invoice_status: status, due_date: dueDate || undefined, schedule_ids: selected.length ? selected : undefined, include_adjustments: includeAdjustments }),
    [asOf, status, dueDate, selected, includeAdjustments],
  )

  // Any change to the run parameters invalidates the reviewed preview.
  useEffect(() => {
    setPreview(null)
  }, [input])

  async function runPreview() {
    setBusy(true)
    setError(null)
    setResult(null)
    try {
      setPreview(await previewBillingRun(api, orgId, input))
    } catch (err) {
      setError(financeErrorMessage(err, 'The preview could not be generated.'))
    } finally {
      setBusy(false)
    }
  }

  async function commit() {
    if (!preview) return
    setBusy(true)
    setError(null)
    try {
      const res = await commitBillingRun(api, orgId, { ...input, expected_fingerprint: preview.fingerprint })
      setResult(res)
      setPreview(null)
      await load()
    } catch (err) {
      setError(financeErrorMessage(err, 'The billing run could not be committed.'))
      // Refresh so the admin reviews what would actually be billed now.
      try {
        setPreview(await previewBillingRun(api, orgId, input))
      } catch {
        /* keep the error banner */
      }
    } finally {
      setBusy(false)
    }
  }

  const toggleSchedule = (id: string) => setSelected((cur) => (cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]))

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Billing runs</h1>
          <p className="page-subtitle">Preview, then commit. Re-running a period never double-bills.</p>
        </div>
      </div>
      {loadError ? <Banner tone="error" onRetry={() => void load()}>{loadError}</Banner> : null}

      <section className="card u-p-20px" >
        <div style={formGrid}>
          <Field label="Bill the period containing">
            <input className="form-input" type="date" value={asOf} onChange={(e) => setAsOf(e.target.value)} />
          </Field>
          <Field label="Create invoices as">
            <select className="form-input" value={status} onChange={(e) => setStatus(e.target.value as 'draft' | 'issued')}>
              <option value="draft">Drafts for review</option>
              <option value="issued">Issued (applies account credit)</option>
            </select>
          </Field>
          <Field label="Due date" hint="Defaults to 15 days after the run date.">
            <input className="form-input" type="date" value={dueDate} onChange={(e) => setDueDate(e.target.value)} />
          </Field>
        </div>
        <fieldset className="u-p-0" style={{ border: 'none', margin: '16px 0 0' }}>
          <legend className="form-label u-mb-8px" >
            Schedules {selected.length === 0 ? '(all active)' : `(${selected.length} selected)`}
          </legend>
          <div className="u-gap-8px" style={{ display: 'flex', flexWrap: 'wrap' }}>
            {schedules.map((s) => (
              <label key={s.id} className={selected.includes(s.id) ? 'badge badge-accent' : 'badge badge-default'} style={{ cursor: 'pointer', padding: '6px 12px', display: 'inline-flex', gap: 6, alignItems: 'center' }}>
                <input type="checkbox" checked={selected.includes(s.id)} onChange={() => toggleSchedule(s.id)} className="u-m-0" />
                {s.name}
              </label>
            ))}
            {!loading && schedules.length === 0 ? (
              <span className="u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>
                No active schedules yet. <Link href="/billing/schedules">Create a fee schedule</Link> first.
              </span>
            ) : null}
          </div>
        </fieldset>
        <label className="u-gap-8px u-fs-fs-base u-mt-16px" style={{ display: 'flex', alignItems: 'center' }}>
          <input type="checkbox" checked={includeAdjustments} onChange={(e) => setIncludeAdjustments(e.target.checked)} />
          Include queued one-off adjustments
        </label>
        <button type="button" className="btn btn-primary u-mt-16px" disabled={busy || !asOf || schedules.length === 0} onClick={() => void runPreview()}>
          {busy && !preview ? 'Building preview…' : 'Preview run'}
        </button>
      </section>

      {error ? <Banner tone="error">{error}</Banner> : null}
      {result ? (
        <Banner tone="success">
          Billed {result.run.invoice_count} {result.run.invoice_count === 1 ? 'household' : 'households'} for {formatDateOnly(result.run.as_of)}: <Money cents={result.run.net_cents} strong />.{' '}
          <Link href="/billing?tab=invoices">Review invoices</Link>
        </Banner>
      ) : null}

      {preview ? (
        <section className="card u-p-20px u-mt-20px" >
          <div className="u-gap-16px" style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', alignItems: 'flex-end' }}>
            <div>
              <h2 style={sectionTitle}>
                {preview.household_count} {preview.household_count === 1 ? 'invoice' : 'invoices'} for {formatDateOnly(preview.as_of)}
              </h2>
              <p className="u-m-0 u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>
                {preview.swimmer_count} swimmers, {preview.charge_count} charges, due {formatDateOnly(preview.due_date)}
              </p>
            </div>
            <dl className="u-m-0 u-fs-fs-base" style={{ display: 'grid', gridTemplateColumns: 'repeat(4, auto)', gap: '2px 20px' }}>
              <dt style={{ color: 'var(--text-subtle)' }}>Gross</dt>
              <dt style={{ color: 'var(--text-subtle)' }}>Discounts</dt>
              <dt style={{ color: 'var(--text-subtle)' }}>Adjustments</dt>
              <dt style={{ color: 'var(--text-subtle)' }}>Invoiced</dt>
              <dd className="u-m-0" ><Money cents={preview.gross_cents} /></dd>
              <dd className="u-m-0" ><Money cents={-preview.discount_cents} /></dd>
              <dd className="u-m-0" ><Money cents={preview.adjustment_cents} /></dd>
              <dd className="u-m-0 u-fs-fs-xl" ><Money cents={preview.net_cents} strong /></dd>
            </dl>
          </div>

          {preview.households.length === 0 ? (
            <p className="u-fs-fs-md u-mt-16px" style={{ color: 'var(--text-subtle)' }}>{preview.skipped.length > 0 ? 'Nothing new to bill for this period. See the swimmers not billed below.' : 'Nothing new to bill for this period. Every eligible charge is already on an invoice.'}</p>
          ) : (
            <ScrollTable>
              <table style={{ ...tableStyle, marginTop: 16 }}>
                <thead>
                  <tr>
                    <th style={thStyle}>Household</th>
                    <th style={{ ...thStyle, textAlign: 'right' }}>Charges</th>
                    <th style={{ ...thStyle, textAlign: 'right' }}>Discounts</th>
                    <th style={{ ...thStyle, textAlign: 'right' }}>Invoice total</th>
                    {status === 'issued' ? <th style={{ ...thStyle, textAlign: 'right' }}>Credit applied</th> : null}
                  </tr>
                </thead>
                <tbody>
                  {preview.households.map((h) => (
                    <Fragment key={h.household_id}>
                      <tr>
                        <td style={tdStyle}>
                          <button
                            type="button"
                            aria-expanded={expanded === h.household_id}
                            onClick={() => setExpanded(expanded === h.household_id ? null : h.household_id)}
                            className="u-p-0 u-fw-600" style={{ background: 'none', border: 'none', color: 'var(--text)', cursor: 'pointer', textAlign: 'left', font: 'inherit' }}
                          >
                            {expanded === h.household_id ? '▾' : '▸'} {h.household_name}
                          </button>
                          <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{h.swimmer_count} {h.swimmer_count === 1 ? 'swimmer' : 'swimmers'}</div>
                        </td>
                        <td style={numStyle}><Money cents={h.gross_cents + h.adjustment_cents} /></td>
                        <td style={numStyle}>{h.discount_cents ? <Money cents={-h.discount_cents} /> : '—'}</td>
                        <td style={numStyle}><Money cents={h.total_cents} strong /></td>
                        {status === 'issued' ? <td style={numStyle}>{h.credit_to_apply_cents ? <Money cents={-h.credit_to_apply_cents} /> : '—'}</td> : null}
                      </tr>
                      {expanded === h.household_id
                        ? h.lines.map((line, i) => (
                            <tr key={`${h.household_id}-${i}`} style={{ background: 'var(--accent-subtle)' }}>
                              <td style={{ ...tdStyle, paddingLeft: 24, color: 'var(--text-subtle)' }} colSpan={3}>{line.description}</td>
                              <td style={numStyle}><Money cents={line.amount_cents} /></td>
                              {status === 'issued' ? <td style={tdStyle} /> : null}
                            </tr>
                          ))
                        : null}
                    </Fragment>
                  ))}
                </tbody>
              </table>
            </ScrollTable>
          )}

          {preview.skipped.length > 0 ? (
            <details className="u-mt-16px u-fs-fs-base" >
              <summary style={{ cursor: 'pointer', color: 'var(--text-subtle)' }}>{preview.skipped.length} not billed, with reasons</summary>
              <ul className="u-pl-20px" style={{ margin: '8px 0 0', color: 'var(--text-subtle)' }}>
                {preview.skipped.map((s, i) => (
                  <li key={i}>
                    {s.household_name || 'Unknown household'}
                    {s.swimmer_name ? `, ${s.swimmer_name}` : ''}
                    {s.schedule_name ? ` (${s.schedule_name})` : ''}: {s.reason}
                  </li>
                ))}
              </ul>
            </details>
          ) : null}

          {preview.households.length > 0 ? (
            <div className="u-gap-12px u-mt-20px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
              <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void commit()}>
                {busy ? 'Committing…' : `Create ${preview.household_count} ${status === 'issued' ? 'issued' : 'draft'} ${preview.household_count === 1 ? 'invoice' : 'invoices'}`}
              </button>
              <span className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>If anything changed since this preview, nothing is billed and you get a fresh preview.</span>
            </div>
          ) : null}
        </section>
      ) : null}

      <section className="u-mt-32px" >
        <h2 style={sectionTitle}>History</h2>
        {loading ? (
          <p className="u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>Loading runs…</p>
        ) : !loadError && runs.length === 0 ? (
          <p className="u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>No billing runs yet. Your first committed run will appear here.</p>
        ) : runs.length > 0 ? (
          <ScrollTable>
            <table style={tableStyle}>
              <thead>
                <tr>
                  <th style={thStyle}>Period</th>
                  <th style={thStyle}>Committed</th>
                  <th style={thStyle}>By</th>
                  <th style={{ ...thStyle, textAlign: 'right' }}>Invoices</th>
                  <th style={{ ...thStyle, textAlign: 'right' }}>Discounts</th>
                  <th style={{ ...thStyle, textAlign: 'right' }}>Credit applied</th>
                  <th style={{ ...thStyle, textAlign: 'right' }}>Total</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((r) => (
                  <tr key={r.id}>
                    <td style={tdStyle}>
                      {formatDateOnly(r.as_of)}
                      <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{r.invoice_status === 'issued' ? 'Issued' : 'Drafts'}</div>
                    </td>
                    <td style={tdStyle}>{new Date(r.created_at).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short' })}</td>
                    <td style={tdStyle}>{r.trigger === 'scheduler' ? 'Automatic' : staffName(r.created_by)}</td>
                    <td style={numStyle}>{r.invoice_count}{r.skipped_count ? <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{r.skipped_count} skipped</div> : null}</td>
                    <td style={numStyle}><Money cents={-r.discount_cents} muted /></td>
                    <td style={numStyle}><Money cents={r.credit_applied_cents} muted /></td>
                    <td style={numStyle}><Money cents={r.net_cents} strong /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </ScrollTable>
        ) : null}
      </section>
    </div>
  )
}
