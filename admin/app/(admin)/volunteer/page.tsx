'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchSeasons, type SeasonRecord } from '@/lib/adminData'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  addVolunteerCredit,
  billVolunteerBuyout,
  fetchSeasonLedger,
  fetchVolunteerJobs,
  fetchVolunteerRequirements,
  fetchVolunteerSettings,
  updateVolunteerSettings,
  upsertVolunteerRequirement,
  type VolunteerJobSummary,
  type VolunteerJobView,
  type VolunteerLedgerRow,
  type VolunteerRequirement,
  type VolunteerSettings,
} from '@/lib/volunteerData'
import { fillPercent, formatCents, formatHours } from '@/lib/volunteer-state'
import { ComplianceBadge, HoursMeter, InlineNotice } from '@/components/volunteer/VolunteerPrimitives'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { NewJobsForm } from '@/components/volunteer/NewJobsForm'
import { StaffJobBoard } from '@/components/volunteer/StaffJobBoard'
import { useConfirm } from '@/components/ui/ConfirmDialog'

const th: React.CSSProperties = {
  textAlign: 'left', padding: '12px 12px', fontSize: 'var(--fs-sm)', fontWeight: 600,
  color: 'var(--text-muted)', borderBottom: '1px solid var(--border)', whiteSpace: 'nowrap',
}
const td: React.CSSProperties = { padding: '12px 12px', fontSize: 'var(--fs-base)', borderBottom: '1px solid var(--border)', verticalAlign: 'middle' }

export default function VolunteerAdminPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const isAdmin = (session?.user.roles ?? []).includes('admin')

  const [jobs, setJobs] = useState<VolunteerJobView[]>([])
  const [summary, setSummary] = useState<VolunteerJobSummary | null>(null)
  const [jobsError, setJobsError] = useState<string | null>(null)
  const [jobsLoading, setJobsLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [jobsKey, setJobsKey] = useState(0)

  const [seasons, setSeasons] = useState<SeasonRecord[]>([])
  const [seasonId, setSeasonId] = useState('')
  const [ledger, setLedger] = useState<VolunteerLedgerRow[]>([])
  const [ledgerError, setLedgerError] = useState<string | null>(null)
  const [ledgerLoading, setLedgerLoading] = useState(false)
  const [ledgerKey, setLedgerKey] = useState(0)
  const [actionMsg, setActionMsg] = useState<{ tone: 'error' | 'success'; text: string } | null>(null)
  const [busyRow, setBusyRow] = useState<string | null>(null)
  const [creditFor, setCreditFor] = useState<string | null>(null)
  const [creditHours, setCreditHours] = useState('')
  const [creditReason, setCreditReason] = useState('')

  const [settings, setSettings] = useState<VolunteerSettings | null>(null)
  const [seasonReq, setSeasonReq] = useState<VolunteerRequirement | null>(null)
  const [reqHours, setReqHours] = useState('')
  const [reqRate, setReqRate] = useState('')
  const [cutoff, setCutoff] = useState('')
  const [rulesMsg, setRulesMsg] = useState<{ tone: 'error' | 'success'; text: string } | null>(null)

  const reloadJobs = useCallback(() => setJobsKey((k) => k + 1), [])
  const reloadLedger = useCallback(() => setLedgerKey((k) => k + 1), [])

  useEffect(() => {
    if (!orgId) return
    let cancelled = false
    setJobsLoading(true)
    fetchVolunteerJobs(api, orgId, { upcoming: true })
      .then((res) => { if (!cancelled) { setJobs(res.jobs); setSummary(res.summary); setJobsError(null) } })
      .catch((err) => { if (!cancelled) setJobsError(reportUIError('volunteer.jobs', err, 'Couldn’t load volunteer jobs. Try again.')) })
      .finally(() => { if (!cancelled) setJobsLoading(false) })
    return () => { cancelled = true }
  }, [api, orgId, jobsKey])

  useEffect(() => {
    if (!orgId) return
    fetchSeasons(api, orgId)
      .then((list) => {
        setSeasons(list)
        setSeasonId((cur) => cur || (list.find((s) => s.status === 'active') ?? list[0])?.id || '')
      })
      .catch((err) => setLedgerError(reportUIError('volunteer.seasons', err, 'Couldn’t load seasons. Try again.')))
    fetchVolunteerSettings(api, orgId)
      .then((st) => { setSettings(st); setCutoff(String(st.cancellation_cutoff_hours)) })
      .catch(() => setSettings(null))
  }, [api, orgId])

  useEffect(() => {
    if (!orgId || !seasonId) return
    let cancelled = false
    setLedgerLoading(true)
    Promise.all([fetchSeasonLedger(api, orgId, seasonId), fetchVolunteerRequirements(api, orgId, seasonId)])
      .then(([rows, reqs]) => {
        if (cancelled) return
        setLedger(rows)
        setLedgerError(null)
        setSeasonReq(reqs.find((r) => !r.household_id) ?? null)
      })
      .catch((err) => { if (!cancelled) setLedgerError(reportUIError('volunteer.ledger', err, 'Couldn’t load household hours. Try again.')) })
      .finally(() => { if (!cancelled) setLedgerLoading(false) })
    return () => { cancelled = true }
  }, [api, orgId, seasonId, ledgerKey])

  // Defaults come from org settings, so derive the form values here instead of
  // refetching the ledger every time settings load or are saved.
  useEffect(() => {
    setReqHours(String(seasonReq?.required_hours ?? settings?.default_required_hours ?? 0))
    setReqRate(((seasonReq?.buyout_cents_per_hour ?? settings?.default_buyout_cents_per_hour ?? 0) / 100).toFixed(2))
  }, [seasonReq, settings])

  const totals = useMemo(() => {
    const short = ledger.filter((r) => r.status === 'short').length
    const due = ledger.reduce((sum, r) => sum + r.buyout_due_cents, 0)
    return { short, due }
  }, [ledger])

  const confirm = useConfirm()

  async function billBuyout(row: VolunteerLedgerRow) {
    const ok = await confirm({
      title: `Bill ${row.household_name}?`,
      message: `${formatCents(row.buyout_due_cents)} for ${formatHours(row.remaining_hours)} short. This creates and issues an invoice.`,
      confirmLabel: 'Bill household',
    })
    if (!ok) return
    setBusyRow(row.household_id)
    setActionMsg(null)
    try {
      const inv = await billVolunteerBuyout(api, orgId, { season_id: row.season_id, household_id: row.household_id, issue: true })
      setActionMsg({ tone: 'success', text: `Invoice ${inv.invoice_number} issued to ${row.household_name}.` })
      reloadLedger()
    } catch (err) {
      setActionMsg({ tone: 'error', text: reportUIError('volunteer.buyout', err, 'Couldn’t bill the buyout. Try again.') })
    } finally {
      setBusyRow(null)
    }
  }

  async function submitCredit(row: VolunteerLedgerRow) {
    const hours = Number(creditHours)
    if (!hours || !creditReason.trim()) return
    setBusyRow(row.household_id)
    setActionMsg(null)
    try {
      await addVolunteerCredit(api, orgId, { season_id: row.season_id, household_id: row.household_id, hours, reason: creditReason.trim() })
      setActionMsg({ tone: 'success', text: `Credited ${formatHours(hours)} to ${row.household_name}.` })
      setCreditFor(null)
      setCreditHours('')
      setCreditReason('')
      reloadLedger()
    } catch (err) {
      setActionMsg({ tone: 'error', text: reportUIError('volunteer.credit', err, 'Couldn’t credit hours. Try again.') })
    } finally {
      setBusyRow(null)
    }
  }

  async function saveRules(e: React.FormEvent) {
    e.preventDefault()
    setRulesMsg(null)
    try {
      if (seasonId) {
        await upsertVolunteerRequirement(api, orgId, {
          season_id: seasonId,
          required_hours: Number(reqHours) || 0,
          buyout_cents_per_hour: Math.round((Number(reqRate) || 0) * 100),
        })
      }
      const st = await updateVolunteerSettings(api, orgId, {
        cancellation_cutoff_hours: Number(cutoff) || 0,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
      })
      setSettings(st)
      setRulesMsg({ tone: 'success', text: 'Rules saved.' })
      reloadLedger()
    } catch (err) {
      setRulesMsg({ tone: 'error', text: reportUIError('volunteer.rules', err, 'Couldn’t save the rules. Try again.') })
    }
  }

  const seasonName = seasons.find((s) => s.id === seasonId)?.name ?? 'this season'

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Volunteers</h1>
        </div>
        <button type="button" className="btn btn-primary" onClick={() => setShowForm((v) => !v)}>{showForm ? 'Close' : 'Post jobs'}</button>
      </div>

      {showForm ? (
        <div className="u-mb-24px" >
          <p className="u-fs-fs-base u-m-0-0-12px" style={{ color: 'var(--text-muted)' }}>
            For meet shifts, use the Volunteers tab on the meet so jobs stay attached to it.
          </p>
          <NewJobsForm api={api} orgId={orgId} onCreated={() => { setShowForm(false); reloadJobs() }} />
        </div>
      ) : null}

      <section className="u-mb-space-8" >
        <div className="u-gap-12px u-mb-16px" style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', flexWrap: 'wrap' }}>
          <h2 className="section-title">Upcoming shifts</h2>
          {summary && summary.total_capacity > 0 ? (
            <span className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>
              <strong className="u-ff-font-mono" style={{ color: 'var(--text)' }}>{fillPercent(summary.filled_spots, summary.total_capacity)}%</strong> filled · {summary.open_spots} open spots
            </span>
          ) : null}
        </div>
        {jobsError ? <LoadErrorBanner message={jobsError} onRetry={reloadJobs} /> : null}
        {!jobsLoading && !jobsError && jobs.length === 0 ? (
          <p className="card u-fs-fs-md u-m-0" style={{ color: 'var(--text-muted)' }}>
            No upcoming shifts. Post jobs for a club event above, or open a <Link href="/meets" style={{ color: 'var(--accent-text, var(--accent))' }}>meet</Link> and add its standard crew.
          </p>
        ) : null}
        {!jobsLoading && !jobsError && jobs.length > 0 ? (
          <StaffJobBoard api={api} orgId={orgId} jobs={jobs} onChanged={() => { reloadJobs(); reloadLedger() }} />
        ) : null}
      </section>

      <section>
        <div className="u-gap-12px u-mb-16px" style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', flexWrap: 'wrap' }}>
          <h2 className="section-title">Household hours</h2>
          <select className="form-input" aria-label="Season" value={seasonId} onChange={(e) => setSeasonId(e.target.value)} style={{ width: 'auto', minWidth: '180px' }}>
            {seasons.length === 0 ? <option value="">No seasons</option> : null}
            {seasons.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
        </div>

        {isAdmin && seasonId ? (
          <form onSubmit={saveRules} className="card u-gap-12px u-mb-16px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-end' }}>
            <label className="form-field" style={{ flex: '1 1 140px' }}>
              <span className="form-label">Hours required in {seasonName}</span>
              <input className="form-input" type="number" min={0} step={0.5} value={reqHours} onChange={(e) => setReqHours(e.target.value)} />
            </label>
            <label className="form-field" style={{ flex: '1 1 140px' }}>
              <span className="form-label">Buyout per missing hour ($)</span>
              <input className="form-input" type="number" min={0} step={0.01} value={reqRate} onChange={(e) => setReqRate(e.target.value)} />
            </label>
            <label className="form-field" style={{ flex: '1 1 140px' }}>
              <span className="form-label">Parents can cancel until (hours before)</span>
              <input className="form-input" type="number" min={0} max={720} value={cutoff} onChange={(e) => setCutoff(e.target.value)} />
            </label>
            <button type="submit" className="btn btn-secondary">Save rules</button>
            {rulesMsg ? <div style={{ flexBasis: '100%' }}><InlineNotice tone={rulesMsg.tone}>{rulesMsg.text}</InlineNotice></div> : null}
          </form>
        ) : null}

        {actionMsg ? <InlineNotice tone={actionMsg.tone}>{actionMsg.text}</InlineNotice> : null}
        {ledgerError ? <LoadErrorBanner message={ledgerError} onRetry={reloadLedger} /> : null}
        {!ledgerLoading && !ledgerError && seasonId && ledger.length === 0 ? (
          <p className="card u-fs-fs-md u-m-0" style={{ color: 'var(--text-muted)' }}>No active households yet.</p>
        ) : null}
        {!ledgerError && ledger.length > 0 ? (
          <>
            <p className="u-fs-fs-base u-m-0-0-12px" style={{ color: 'var(--text-muted)' }}>
              {totals.short} {totals.short === 1 ? 'household is' : 'households are'} short · {formatCents(totals.due)} in unbilled buyouts
            </p>
            <div className="card table-scroll u-p-0" style={{ overflowX: 'auto', position: 'relative' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: '760px' }}>
                <thead>
                  <tr>
                    <th style={th}>Household</th>
                    <th style={{ ...th, width: '28%' }}>Progress</th>
                    <th style={th}>Earned</th>
                    <th style={th}>Remaining</th>
                    <th style={th}>Status</th>
                    <th style={th}>Buyout</th>
                    <th style={th}><span className="sr-only">Actions</span></th>
                  </tr>
                </thead>
                <tbody>
                  {ledger.map((row) => (
                    <tr key={row.household_id}>
                      <td style={td}>
                        <Link href={`/members/households/${row.household_id}`} className="u-fw-600" style={{ color: 'var(--text)' }}>{row.household_name}</Link>
                        {row.requirement_source === 'household' ? <span className="u-fs-fs-xs" style={{ display: 'block', color: 'var(--text-muted)' }}>Custom requirement</span> : null}
                      </td>
                      <td style={td}><HoursMeter compact required={row.required_hours} earned={row.earned_hours} scheduled={row.scheduled_hours} /></td>
                      <td style={{ ...td, fontFamily: 'var(--font-mono)' }}>{row.earned_hours} / {row.required_hours}</td>
                      <td style={{ ...td, fontFamily: 'var(--font-mono)' }}>{row.remaining_hours}</td>
                      <td style={td}><ComplianceBadge status={row.status} /></td>
                      <td style={{ ...td, fontFamily: 'var(--font-mono)' }}>
                        {row.buyout?.invoice_id ? (
                          isAdmin ? <Link href={`/invoices/${row.buyout.invoice_id}`} style={{ color: 'var(--accent-text, var(--accent))' }}>{formatCents(row.buyout.amount_cents)}</Link> : formatCents(row.buyout.amount_cents)
                        ) : row.buyout_due_cents > 0 ? formatCents(row.buyout_due_cents) : '—'}
                      </td>
                      <td style={{ ...td, whiteSpace: 'nowrap', textAlign: 'right' }}>
                        {creditFor === row.household_id ? (
                          <span className="u-gap-6px" style={{ display: 'inline-flex', alignItems: 'center' }}>
                            <input className="form-input" aria-label="Hours" type="number" step={0.25} placeholder="Hrs" value={creditHours} onChange={(e) => setCreditHours(e.target.value)} style={{ width: '70px' }} />
                            <input className="form-input" aria-label="Reason" placeholder="Reason" value={creditReason} onChange={(e) => setCreditReason(e.target.value)} style={{ width: '140px' }} />
                            <button type="button" className="btn btn-primary btn-sm" disabled={busyRow !== null || !Number(creditHours) || !creditReason.trim()} onClick={() => submitCredit(row)}>Save</button>
                            <button type="button" className="btn btn-ghost btn-sm" onClick={() => setCreditFor(null)}>Cancel</button>
                          </span>
                        ) : (
                          <span className="u-gap-6px" style={{ display: 'inline-flex' }}>
                            <button type="button" className="btn btn-ghost btn-sm" onClick={() => setCreditFor(row.household_id)}>Credit hours</button>
                            {isAdmin && row.buyout_due_cents > 0 && !row.buyout ? (
                              <button type="button" className="btn btn-secondary btn-sm" disabled={busyRow !== null} onClick={() => billBuyout(row)}>
                                {busyRow === row.household_id ? 'Billing…' : 'Bill buyout'}
                              </button>
                            ) : null}
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        ) : null}
      </section>
    </div>
  )
}
