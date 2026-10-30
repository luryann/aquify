'use client'

import { useCallback, useEffect, useState } from 'react'
import { fetchGroups, fetchHouseholds, fetchSeasons, type GroupRecord, type HouseholdRecord, type SeasonRecord } from '@/lib/adminData'
import {
  applyLateFeesNow,
  archiveDiscountRule,
  archiveFeeSchedule,
  createAdjustment,
  describeDiscount,
  describeTier,
  fetchAdjustments,
  fetchBillingSettings,
  fetchDiscountRules,
  fetchFeeSchedules,
  financeErrorMessage,
  formatCents,
  parseDollarsToCents,
  saveBillingSettings,
  saveDiscountRule,
  saveFeeSchedule,
  voidAdjustment,
  type BillingAdjustment,
  type BillingSettings,
  type DiscountRule,
  type FeeSchedule,
  type FeeScheduleInput,
} from '@/lib/financeData'
import { formatDateOnly } from '@/lib/dates'
import { Banner, Field, Money, ScrollTable, formGrid, numStyle, sectionTitle, tableStyle, tdStyle, thStyle, useFinanceContext } from '../financeUi'
import { PageSkeleton } from '@/components/ui/PageSkeleton'

const FREQUENCY_LABEL: Record<string, string> = { monthly: 'Monthly', quarterly: 'Quarterly', annual: 'Annual', one_time: 'One-time' }
const PRORATION_LABEL: Record<string, string> = {
  none: 'Full amount',
  daily: 'By days active',
  half: 'Half if under half the period',
  waive: 'Waive join period',
}

type ScheduleDraft = {
  id?: string
  name: string
  kind: 'dues' | 'registration'
  group_code: string
  season: string
  frequency: FeeScheduleInput['frequency']
  amount: string
  start_date: string
  end_date: string
  proration: FeeScheduleInput['proration']
  auto_bill: boolean
}

const EMPTY_SCHEDULE: ScheduleDraft = {
  name: '', kind: 'dues', group_code: '', season: '', frequency: 'monthly', amount: '', start_date: '', end_date: '', proration: 'daily', auto_bill: false,
}

export default function FeeSchedulesPage() {
  const { api, orgId } = useFinanceContext()
  const [schedules, setSchedules] = useState<FeeSchedule[]>([])
  const [rules, setRules] = useState<DiscountRule[]>([])
  const [adjustments, setAdjustments] = useState<BillingAdjustment[]>([])
  const [settings, setSettings] = useState<BillingSettings | null>(null)
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [seasons, setSeasons] = useState<SeasonRecord[]>([])
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [message, setMessage] = useState<{ tone: 'error' | 'success'; text: string } | null>(null)
  const [draft, setDraft] = useState<ScheduleDraft | null>(null)
  const [ruleDraft, setRuleDraft] = useState({ name: '', min: '2', max: '', type: 'percent' as 'percent' | 'fixed', value: '', applies_to: 'dues' as DiscountRule['applies_to'] })
  const [adjDraft, setAdjDraft] = useState({ household_id: '', amount: '', reason: '' })
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    if (!orgId) return
    setLoading(true)
    setLoadError(null)
    try {
      const [s, r, a, st, g, se, h] = await Promise.all([
        fetchFeeSchedules(api, orgId),
        fetchDiscountRules(api, orgId),
        fetchAdjustments(api, orgId),
        fetchBillingSettings(api, orgId),
        fetchGroups(api, orgId),
        fetchSeasons(api, orgId),
        fetchHouseholds(api, orgId),
      ])
      setSchedules(s)
      setRules(r)
      setAdjustments(a)
      setSettings(st)
      setGroups(g)
      setSeasons(se)
      setHouseholds(h)
    } catch (err) {
      setLoadError(financeErrorMessage(err, 'Fee schedules could not be loaded.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId])

  useEffect(() => {
    void load()
  }, [load])

  async function run<T>(action: () => Promise<T>, success: string | ((result: T) => string)) {
    setBusy(true)
    setMessage(null)
    try {
      const result = await action()
      setMessage({ tone: 'success', text: typeof success === 'function' ? success(result) : success })
      await load()
      return true
    } catch (err) {
      setMessage({ tone: 'error', text: financeErrorMessage(err, 'The change could not be saved.') })
      return false
    } finally {
      setBusy(false)
    }
  }

  async function submitSchedule() {
    if (!draft) return
    const cents = parseDollarsToCents(draft.amount)
    if (!cents || cents <= 0) {
      setMessage({ tone: 'error', text: 'Enter an amount greater than zero, like 150 or 150.00.' })
      return
    }
    const input: FeeScheduleInput = {
      name: draft.name,
      kind: draft.kind,
      group_code: draft.group_code,
      season: draft.kind === 'registration' ? draft.season : '',
      frequency: draft.kind === 'registration' ? 'one_time' : draft.frequency,
      amount_cents: cents,
      start_date: draft.start_date,
      end_date: draft.end_date,
      proration: draft.kind === 'registration' || draft.frequency === 'one_time' ? 'none' : draft.proration,
      auto_bill: draft.auto_bill,
    }
    if (await run(() => saveFeeSchedule(api, orgId, input, draft.id), draft.id ? 'Schedule updated.' : 'Schedule created.')) setDraft(null)
  }

  async function submitRule() {
    const value = ruleDraft.type === 'percent' ? Math.round(Number(ruleDraft.value) * 100) : parseDollarsToCents(ruleDraft.value) ?? 0
    if (!value || value <= 0) {
      setMessage({ tone: 'error', text: 'Enter a discount value greater than zero.' })
      return
    }
    const ok = await run(
      () =>
        saveDiscountRule(api, orgId, {
          name: ruleDraft.name,
          applies_to: ruleDraft.applies_to,
          min_position: Number(ruleDraft.min),
          max_position: ruleDraft.max ? Number(ruleDraft.max) : null,
          discount_type: ruleDraft.type,
          percent_bps: ruleDraft.type === 'percent' ? value : 0,
          amount_cents: ruleDraft.type === 'fixed' ? value : 0,
        }),
      'Discount tier added.',
    )
    if (ok) setRuleDraft({ ...ruleDraft, name: '', value: '' })
  }

  async function submitAdjustment() {
    const cents = parseDollarsToCents(adjDraft.amount)
    if (!cents) {
      setMessage({ tone: 'error', text: 'Enter a non-zero amount. Use a negative amount for a discount.' })
      return
    }
    const ok = await run(() => createAdjustment(api, orgId, { household_id: adjDraft.household_id, amount_cents: cents, reason: adjDraft.reason }), 'Adjustment queued for the next billing run.')
    if (ok) setAdjDraft({ household_id: '', amount: '', reason: '' })
  }

  const householdName = (id: string) => households.find((h) => h.id === id)?.household_name ?? 'Unknown household'
  const activeSchedules = schedules.filter((s) => s.active)
  const archivedSchedules = schedules.filter((s) => !s.active)

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Fee schedules</h1>
        </div>
      </div>

      {loadError ? <Banner tone="error" onRetry={() => void load()}>{loadError}</Banner> : null}
      {message ? <Banner tone={message.tone}>{message.text}</Banner> : null}

      <section className="card u-p-20px u-mb-space-8" >
        <div className="u-gap-12px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', flexWrap: 'wrap' }}>
          <h2 style={sectionTitle}>Dues and registration fees</h2>
          {!draft ? (
            <button type="button" className="btn btn-primary btn-sm" onClick={() => setDraft(EMPTY_SCHEDULE)}>
              New schedule
            </button>
          ) : null}
        </div>

        {draft ? (
          <div className="u-p-16px u-m-16px-0" style={{ border: '1px solid var(--border-strong)', borderRadius: 12 }}>
            <div style={formGrid}>
              <Field label="Name">
                <input aria-label="Silver monthly dues" className="form-input" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} placeholder="Silver monthly dues" />
              </Field>
              <Field label="Type">
                <select className="form-input" value={draft.kind} onChange={(e) => setDraft({ ...draft, kind: e.target.value as ScheduleDraft['kind'] })}>
                  <option value="dues">Recurring dues</option>
                  <option value="registration">Season registration fee</option>
                </select>
              </Field>
              <Field label={draft.kind === 'dues' ? 'Roster group' : 'Group (optional)'}>
                <select className="form-input" value={draft.group_code} onChange={(e) => setDraft({ ...draft, group_code: e.target.value })}>
                  <option value="">{draft.kind === 'dues' ? 'Choose a group' : 'All groups'}</option>
                  {groups.map((g) => (
                    <option key={g.id} value={g.code}>
                      {g.name} ({g.code})
                    </option>
                  ))}
                </select>
              </Field>
              {draft.kind === 'registration' ? (
                <Field label="Season" hint="Matched against the season on each registration.">
                  <input className="form-input" list="finance-seasons" value={draft.season} onChange={(e) => setDraft({ ...draft, season: e.target.value })} />
                  <datalist id="finance-seasons">
                    {seasons.map((s) => (
                      <option key={s.id} value={s.name} />
                    ))}
                  </datalist>
                </Field>
              ) : (
                <Field label="Frequency">
                  <select className="form-input" value={draft.frequency} onChange={(e) => setDraft({ ...draft, frequency: e.target.value as ScheduleDraft['frequency'] })}>
                    {Object.entries(FREQUENCY_LABEL).map(([v, l]) => (
                      <option key={v} value={v}>
                        {l}
                      </option>
                    ))}
                  </select>
                </Field>
              )}
              <Field label="Amount (USD)">
                <input aria-label="150.00" className="form-input" inputMode="decimal" value={draft.amount} onChange={(e) => setDraft({ ...draft, amount: e.target.value })} placeholder="150.00" />
              </Field>
              <Field label="Starts">
                <input className="form-input" type="date" value={draft.start_date} onChange={(e) => setDraft({ ...draft, start_date: e.target.value })} />
              </Field>
              <Field label="Ends (optional)">
                <input className="form-input" type="date" value={draft.end_date} onChange={(e) => setDraft({ ...draft, end_date: e.target.value })} />
              </Field>
              {draft.kind === 'dues' && draft.frequency !== 'one_time' ? (
                <Field label="Mid-period joins">
                  <select className="form-input" value={draft.proration} onChange={(e) => setDraft({ ...draft, proration: e.target.value as ScheduleDraft['proration'] })}>
                    {Object.entries(PRORATION_LABEL).map(([v, l]) => (
                      <option key={v} value={v}>
                        {l}
                      </option>
                    ))}
                  </select>
                </Field>
              ) : null}
            </div>
            <label className="u-gap-8px u-m-16px-0 u-fs-fs-base" style={{ display: 'flex', alignItems: 'center' }}>
              <input type="checkbox" checked={draft.auto_bill} onChange={(e) => setDraft({ ...draft, auto_bill: e.target.checked })} />
              Include in automatic monthly billing
            </label>
            <div className="u-gap-8px" style={{ display: 'flex' }}>
              <button type="button" className="btn btn-primary btn-sm" disabled={busy} onClick={() => void submitSchedule()}>
                {draft.id ? 'Save schedule' : 'Create schedule'}
              </button>
              <button type="button" className="btn btn-ghost btn-sm" onClick={() => setDraft(null)}>
                Cancel
              </button>
            </div>
          </div>
        ) : null}

        {loading ? (
          <PageSkeleton variant="table" noHeader rows={4} />
        ) : !loadError && activeSchedules.length === 0 ? (
          <p className="u-fs-fs-base u-mt-12px" style={{ color: 'var(--text-subtle)' }}>No active schedules. Create one per roster group to start billing dues.</p>
        ) : activeSchedules.length > 0 ? (
          <ScrollTable>
            <table style={{ ...tableStyle, marginTop: 12 }}>
              <thead>
                <tr>
                  <th style={thStyle}>Schedule</th>
                  <th style={thStyle}>Applies to</th>
                  <th style={thStyle}>Billing</th>
                  <th style={{ ...thStyle, textAlign: 'right' }}>Amount</th>
                  <th style={thStyle}>Dates</th>
                  <th style={thStyle} />
                </tr>
              </thead>
              <tbody>
                {activeSchedules.map((s) => (
                  <tr key={s.id}>
                    <td style={tdStyle}>
                      <div className="u-fw-600" >{s.name}</div>
                      {s.auto_bill ? <span className="badge badge-accent u-mt-4px" >Auto-billed</span> : null}
                    </td>
                    <td style={tdStyle}>{s.kind === 'dues' ? `${s.group_code} roster` : `${s.season} registrations${s.group_code ? ` · ${s.group_code}` : ''}`}</td>
                    <td style={tdStyle}>
                      {FREQUENCY_LABEL[s.frequency]}
                      {s.kind === 'dues' && s.frequency !== 'one_time' ? <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{PRORATION_LABEL[s.proration]}</div> : null}
                    </td>
                    <td style={numStyle}>
                      <Money cents={s.amount_cents} strong />
                    </td>
                    <td style={tdStyle}>
                      {formatDateOnly(s.start_date)}
                      {s.end_date ? ` – ${formatDateOnly(s.end_date)}` : ' onward'}
                    </td>
                    <td style={{ ...tdStyle, textAlign: 'right', whiteSpace: 'nowrap' }}>
                      <button
                        type="button"
                        className="btn btn-ghost btn-sm"
                        onClick={() =>
                          setDraft({
                            id: s.id, name: s.name, kind: s.kind, group_code: s.group_code ?? '', season: s.season ?? '', frequency: s.frequency,
                            amount: (s.amount_cents / 100).toFixed(2), start_date: s.start_date, end_date: s.end_date ?? '', proration: s.proration, auto_bill: s.auto_bill,
                          })
                        }
                      >
                        Edit
                      </button>
                      <button type="button" className="btn btn-ghost btn-sm" disabled={busy} onClick={() => void run(() => archiveFeeSchedule(api, orgId, s.id), 'Schedule archived.')}>
                        Archive
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </ScrollTable>
        ) : null}
        {archivedSchedules.length > 0 ? (
          <p className="u-fs-fs-sm u-mt-12px" style={{ color: 'var(--text-subtle)' }}>
            {archivedSchedules.length} archived {archivedSchedules.length === 1 ? 'schedule keeps' : 'schedules keep'} their billing history.
          </p>
        ) : null}
      </section>

      <div className="u-gap-20px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 460px), 1fr))' }}>
        <section className="card u-p-20px" >
          <h2 style={sectionTitle}>Sibling discounts</h2>
          <p className="u-fs-fs-base u-m-0-0-12px" style={{ color: 'var(--text-subtle)' }}>
            Highest-fee swimmer pays full price; later siblings get the best tier. No stacking.
          </p>
          {rules.filter((r) => r.active).map((r) => (
            <div key={r.id} className="u-gap-12px u-p-12px-0" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--border)' }}>
              <div>
                <div className="u-fw-600 u-fs-fs-md" >{describeTier(r)}: {describeDiscount(r)}</div>
                <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
                  {r.name} · {r.applies_to === 'all' ? 'dues and registration' : r.applies_to}
                </div>
              </div>
              <button type="button" className="btn btn-ghost btn-sm" disabled={busy} onClick={() => void run(() => archiveDiscountRule(api, orgId, r.id), 'Tier removed.')}>
                Remove
              </button>
            </div>
          ))}
          <div style={{ ...formGrid, gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', marginTop: 16 }}>
            <Field label="Name">
              <input aria-label="2nd sibling" className="form-input" value={ruleDraft.name} onChange={(e) => setRuleDraft({ ...ruleDraft, name: e.target.value })} placeholder="2nd sibling" />
            </Field>
            <Field label="From swimmer #">
              <input className="form-input" type="number" min={2} value={ruleDraft.min} onChange={(e) => setRuleDraft({ ...ruleDraft, min: e.target.value })} />
            </Field>
            <Field label="Through # (blank = and up)">
              <input className="form-input" type="number" min={2} value={ruleDraft.max} onChange={(e) => setRuleDraft({ ...ruleDraft, max: e.target.value })} />
            </Field>
            <Field label="Discount">
              <select className="form-input" value={ruleDraft.type} onChange={(e) => setRuleDraft({ ...ruleDraft, type: e.target.value as 'percent' | 'fixed' })}>
                <option value="percent">Percent</option>
                <option value="fixed">Fixed amount</option>
              </select>
            </Field>
            <Field label={ruleDraft.type === 'percent' ? 'Percent off' : 'Amount off (USD)'}>
              <input className="form-input" inputMode="decimal" value={ruleDraft.value} onChange={(e) => setRuleDraft({ ...ruleDraft, value: e.target.value })} placeholder={ruleDraft.type === 'percent' ? '10' : '25.00'} />
            </Field>
            <Field label="Applies to">
              <select className="form-input" value={ruleDraft.applies_to} onChange={(e) => setRuleDraft({ ...ruleDraft, applies_to: e.target.value as DiscountRule['applies_to'] })}>
                <option value="dues">Dues</option>
                <option value="registration">Registration</option>
                <option value="all">Both</option>
              </select>
            </Field>
          </div>
          <button type="button" className="btn btn-secondary btn-sm u-mt-12px" disabled={busy || !ruleDraft.name} onClick={() => void submitRule()}>
            Add tier
          </button>
        </section>

        <section className="card u-p-20px" >
          <h2 style={sectionTitle}>One-off adjustments</h2>
          <p className="u-fs-fs-base u-m-0-0-12px" style={{ color: 'var(--text-subtle)' }}>Applied once, on the household&apos;s next billing run.</p>
          {adjustments.map((a) => (
            <div key={a.id} className="u-gap-12px u-p-12px-0" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--border)' }}>
              <div style={{ minWidth: 0 }}>
                <div className="u-fw-600 u-fs-fs-md" >{householdName(a.household_id)}</div>
                <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{a.reason}</div>
              </div>
              <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center' }}>
                <Money cents={a.amount_cents} strong />
                <button type="button" className="btn btn-ghost btn-sm" disabled={busy} onClick={() => void run(() => voidAdjustment(api, orgId, a.id), 'Adjustment voided.')}>
                  Void
                </button>
              </div>
            </div>
          ))}
          <div style={{ ...formGrid, marginTop: 16 }}>
            <Field label="Household">
              <select className="form-input" value={adjDraft.household_id} onChange={(e) => setAdjDraft({ ...adjDraft, household_id: e.target.value })}>
                <option value="">Choose a household</option>
                {households.map((h) => (
                  <option key={h.id} value={h.id}>
                    {h.household_name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Amount (negative = discount)">
              <input aria-label="-20.00" className="form-input" inputMode="decimal" value={adjDraft.amount} onChange={(e) => setAdjDraft({ ...adjDraft, amount: e.target.value })} placeholder="-20.00" />
            </Field>
          </div>
          <Field label="Reason">
            <input aria-label="Volunteer hours credit" className="form-input" value={adjDraft.reason} onChange={(e) => setAdjDraft({ ...adjDraft, reason: e.target.value })} placeholder="Volunteer hours credit" />
          </Field>
          <button type="button" className="btn btn-secondary btn-sm u-mt-12px" disabled={busy || !adjDraft.household_id || !adjDraft.reason.trim()} onClick={() => void submitAdjustment()}>
            Queue adjustment
          </button>
        </section>
      </div>

      {settings ? (
        <section className="card u-p-20px u-mt-20px" >
          <h2 style={sectionTitle}>Late fees and automatic billing</h2>
          <div style={{ ...formGrid, marginTop: 12 }}>
            <label className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center' }}>
              <input type="checkbox" checked={settings.late_fee_enabled} onChange={(e) => setSettings({ ...settings, late_fee_enabled: e.target.checked })} />
              Charge a late fee
            </label>
            <Field label="Grace days after due date">
              <input className="form-input" type="number" min={0} value={settings.late_fee_grace_days} onChange={(e) => setSettings({ ...settings, late_fee_grace_days: Number(e.target.value) })} />
            </Field>
            <Field label="Fee type">
              <select className="form-input" value={settings.late_fee_type} onChange={(e) => setSettings({ ...settings, late_fee_type: e.target.value as BillingSettings['late_fee_type'] })}>
                <option value="flat">Flat amount</option>
                <option value="percent">Percent of balance</option>
              </select>
            </Field>
            {settings.late_fee_type === 'flat' ? (
              <Field label="Fee (USD)">
                <input className="form-input" inputMode="decimal" defaultValue={(settings.late_fee_flat_cents / 100).toFixed(2)} onBlur={(e) => setSettings({ ...settings, late_fee_flat_cents: parseDollarsToCents(e.target.value) ?? 0 })} />
              </Field>
            ) : (
              <Field label="Percent of balance">
                <input className="form-input" inputMode="decimal" defaultValue={settings.late_fee_percent_bps / 100} onBlur={(e) => setSettings({ ...settings, late_fee_percent_bps: Math.round(Number(e.target.value) * 100) })} />
              </Field>
            )}
            <Field label="Cap (USD, 0 = none)">
              <input className="form-input" inputMode="decimal" defaultValue={(settings.late_fee_max_cents / 100).toFixed(2)} onBlur={(e) => setSettings({ ...settings, late_fee_max_cents: parseDollarsToCents(e.target.value) ?? 0 })} />
            </Field>
          </div>
          <div style={{ ...formGrid, marginTop: 16 }}>
            <label className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center' }}>
              <input type="checkbox" checked={settings.auto_run_enabled} onChange={(e) => setSettings({ ...settings, auto_run_enabled: e.target.checked })} />
              Bill auto-billed schedules monthly
            </label>
            <Field label="Run on day of month">
              <input className="form-input" type="number" min={1} max={28} value={settings.auto_run_day} onChange={(e) => setSettings({ ...settings, auto_run_day: Number(e.target.value) })} />
            </Field>
            <Field label="Create invoices as">
              <select className="form-input" value={settings.auto_run_invoice_status} onChange={(e) => setSettings({ ...settings, auto_run_invoice_status: e.target.value as BillingSettings['auto_run_invoice_status'] })}>
                <option value="draft">Drafts for review</option>
                <option value="issued">Issued to families</option>
              </select>
            </Field>
            <Field label="Due after (days)">
              <input className="form-input" type="number" min={0} value={settings.auto_run_due_days} onChange={(e) => setSettings({ ...settings, auto_run_due_days: Number(e.target.value) })} />
            </Field>
          </div>
          <h2 className="u-mt-24px" style={sectionTitle}>Meet fees</h2>
          <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-subtle)' }}>
            Default charges for each swimmer entered in a meet. Per-event fees are multiplied by the number of events the swimmer swims.
          </p>
          <div style={{ ...formGrid, marginTop: 12 }}>
            <Field label="Fee per event (USD)">
              <input className="form-input" inputMode="decimal" defaultValue={(settings.meet_fee_per_event_cents / 100).toFixed(2)} onBlur={(e) => setSettings({ ...settings, meet_fee_per_event_cents: parseDollarsToCents(e.target.value) ?? 0 })} />
            </Field>
            <Field label="Flat fee per swimmer per meet (USD)">
              <input className="form-input" inputMode="decimal" defaultValue={(settings.meet_fee_flat_cents / 100).toFixed(2)} onBlur={(e) => setSettings({ ...settings, meet_fee_flat_cents: parseDollarsToCents(e.target.value) ?? 0 })} />
            </Field>
            <Field label="Create meet invoices as">
              <select className="form-input" value={settings.meet_fee_invoice_status} onChange={(e) => setSettings({ ...settings, meet_fee_invoice_status: e.target.value as BillingSettings['meet_fee_invoice_status'] })}>
                <option value="draft">Drafts for review</option>
                <option value="issued">Issued to families</option>
              </select>
            </Field>
            <Field label="Due after (days)">
              <input className="form-input" type="number" min={0} value={settings.meet_fee_due_days} onChange={(e) => setSettings({ ...settings, meet_fee_due_days: Number(e.target.value) })} />
            </Field>
          </div>
          <div className="u-gap-8px u-mt-16px" style={{ display: 'flex', flexWrap: 'wrap' }}>
            <button type="button" className="btn btn-primary btn-sm" disabled={busy} onClick={() => void run(() => saveBillingSettings(api, orgId, settings), 'Billing settings saved.')}>
              Save settings
            </button>
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              disabled={busy || !settings.late_fee_enabled}
              onClick={() =>
                void run(
                  () => applyLateFeesNow(api, orgId),
                  (result) => `${result.applied_count} late ${result.applied_count === 1 ? 'fee' : 'fees'} assessed (${formatCents(result.total_cents)}).`,
                )
              }
            >
              Assess late fees now
            </button>
          </div>
        </section>
      ) : null}
    </div>
  )
}
