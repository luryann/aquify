'use client'

import { useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { formatDateOnly } from '@/lib/dates'
import {
  fetchHouseholds,
  fetchHouseholdAutopayStatuses,
  optOutHouseholdAutopay,
  upsertHouseholdAutopay,
  type HouseholdAutopayStatusRecord,
  type HouseholdRecord,
} from '@/lib/adminData'
import {
  DEFAULT_AUTOPAY_FORM,
  autopayErrorMessage,
  autopayFormFromStatus,
  autopayPayloadFromForm,
  formatAutopayMethod,
  mergeHouseholdAutopayRows,
  type AutopayFormState,
  type HouseholdAutopayRow,
} from '@/lib/autopay'
import { FIELD_INPUT_STYLE } from '@/lib/form-styles'
import { PageSkeleton } from '@/components/ui/PageSkeleton'

function formatCurrencyCents(value?: number) {
  return typeof value === 'number' ? `$${(value / 100).toFixed(2)}` : '—'
}

export function AutopayPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [statuses, setStatuses] = useState<HouseholdAutopayStatusRecord[]>([])
  const [selectedHouseholdId, setSelectedHouseholdId] = useState('')
  const [form, setForm] = useState<AutopayFormState>(DEFAULT_AUTOPAY_FORM)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!orgId) {
        setLoading(false)
        return
      }

      setLoading(true)
      setError('')
      try {
        const householdList = await fetchHouseholds(api, orgId)
        const autopayStatuses = await fetchHouseholdAutopayStatuses(api, orgId, householdList.map((household) => household.id))
        if (cancelled) return
        setHouseholds(householdList)
        setStatuses(autopayStatuses)
        setSelectedHouseholdId(householdList[0]?.id ?? '')
      } catch (loadError) {
        if (!cancelled) setError(autopayErrorMessage(loadError, 'Couldn’t load autopay households. Try again.'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, orgId])

  useEffect(() => {
    if (!selectedHouseholdId) {
      setForm(DEFAULT_AUTOPAY_FORM)
      return
    }
    const status = statuses.find((entry) => entry.household_id === selectedHouseholdId) ?? null
    setForm(autopayFormFromStatus(status))
  }, [selectedHouseholdId, statuses])

  const rows = mergeHouseholdAutopayRows(households, statuses)
  const selectedRow = rows.find((row) => row.householdId === selectedHouseholdId) ?? null
  const enabledCount = rows.filter((row) => row.enabled).length
  const scheduledCount = rows.filter((row) => row.enabled && row.nextChargeDate).length

  function handleSelect(row: HouseholdAutopayRow) {
    setSelectedHouseholdId(row.householdId)
    setForm(autopayFormFromStatus(row.status))
    setError('')
    setSuccess('')
  }

  async function refreshStatuses(nextSelected = selectedHouseholdId) {
    const autopayStatuses = await fetchHouseholdAutopayStatuses(api, orgId, households.map((household) => household.id))
    setStatuses(autopayStatuses)
    const selectedStatus = autopayStatuses.find((entry) => entry.household_id === nextSelected) ?? null
    setForm(autopayFormFromStatus(selectedStatus))
  }

  async function mutateAutopay(mutate: () => Promise<unknown>, successMessage: string, failureMessage?: string) {
    if (!selectedHouseholdId || !orgId) return
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      await mutate()
      await refreshStatuses(selectedHouseholdId)
      setSuccess(successMessage)
    } catch (saveError) {
      setError(autopayErrorMessage(saveError, failureMessage))
    } finally {
      setSaving(false)
    }
  }

  const saveAutopay = () => mutateAutopay(
    () => upsertHouseholdAutopay(api, orgId, selectedHouseholdId, autopayPayloadFromForm(form)),
    'Autopay settings saved.',
  )

  const disableAutopay = () => mutateAutopay(
    () => optOutHouseholdAutopay(api, orgId, selectedHouseholdId),
    'Autopay has been turned off for this household.',
    'Couldn’t opt this household out. Try again.',
  )

  return (
    <div>
      {error ? (
        <div className="u-p-12px-16px u-mb-20px u-fs-fs-sm u-ff-font-body" style={{ background: 'var(--danger-bg)', border: '1px solid var(--danger-border)', borderRadius: '8px', color: 'var(--danger)' }}>
          {error}
        </div>
      ) : null}
      {success ? (
        <div className="u-p-12px-16px u-mb-20px u-fs-fs-sm u-ff-font-body" style={{ background: 'var(--success-bg)', border: '1px solid rgba(5,150,105,0.15)', borderRadius: '8px', color: 'var(--success)' }}>
          {success}
        </div>
      ) : null}

      <div className="cols-3 u-gap-12px u-mb-20px" >
        {[
          { label: 'Autopay enabled', value: enabledCount, sub: `${rows.length} tracked households`, color: 'var(--accent-text, var(--accent))' },
          { label: 'Next charge batch', value: scheduledCount, sub: 'households with upcoming charges', color: 'var(--success)' },
          { label: 'Manual review', value: rows.length - enabledCount, sub: 'households still on manual billing', color: 'var(--text)' },
        ].map((stat) => (
          <div key={stat.label} className="card u-p-16px-20px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</div>
            <div className="u-fs-fs-2xl u-ff-font-body u-fw-700" style={{ color: stat.color }}>{loading ? '—' : stat.value}</div>
            <div className="u-fs-fs-xs u-mt-2px" style={{ color: 'var(--text-muted)' }}>{stat.sub}</div>
          </div>
        ))}
      </div>

      <div className="split-pair u-gap-16px" >
        <div className="card u-p-16px" style={{ overflowX: 'auto' }}>
          <div className="u-gap-12px u-pb-12px" style={{ display: 'grid', gridTemplateColumns: '1.3fr 1fr 1fr 0.8fr 0.8fr', borderBottom: '1px solid var(--border)', minWidth: '560px' }}>
            {['Household', 'Saved Method', 'Next Scheduled Charge', 'Status', 'Actions'].map((header) => (
              <div key={header} className="col-head">
                {header}
              </div>
            ))}
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {loading ? (
              <PageSkeleton variant="table" noHeader rows={4} />
            ) : rows.length === 0 ? (
              <div className="u-p-24px u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>No households available.</div>
            ) : (
              rows.map((row) => (
                <div key={row.householdId} className="u-gap-12px u-p-16px-0" style={{ display: 'grid', gridTemplateColumns: '1.3fr 1fr 1fr 0.8fr 0.8fr', alignItems: 'center', borderBottom: '1px solid var(--border)', minWidth: '560px' }}>
                  <div>
                    <div className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{row.householdName}</div>
                  </div>
                  <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{row.methodLabel}</div>
                  <div>
                    <div className="u-fs-fs-base" style={{ color: 'var(--text)' }}>{formatDateOnly(row.nextChargeDate)}</div>
                    <div className="u-fs-fs-xs u-ff-font-body u-mt-4px" style={{ color: 'var(--text-muted)' }}>{formatCurrencyCents(row.nextChargeAmountCents)}</div>
                  </div>
                  <span className="badge" style={{ width: 'fit-content', background: row.enabled ? 'var(--success-bg)' : 'var(--warning-bg)', color: row.enabled ? 'var(--success)' : 'var(--warning)' }}>
                    {row.enabled ? 'Enabled' : 'Paused'}
                  </span>
                  <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center' }}>
                    <button type="button" className="btn btn-secondary" onClick={() => handleSelect(row)}>
                      {selectedHouseholdId === row.householdId ? 'Editing' : 'Manage'}
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="card u-p-20px" >
          <h2 className="section-title u-mb-4px" >
            {selectedRow?.householdName ?? 'Choose a household'}
          </h2>
          <p className="u-fs-fs-sm u-mb-16px" style={{ color: 'var(--text-muted)' }}>
            {selectedRow ? (selectedRow.enabled ? 'Autopay enabled' : 'Manual billing') : 'Select a row to edit.'}
          </p>

          {selectedRow ? (
            <div className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div className="u-p-12px-16px" style={{ borderRadius: '12px', background: selectedRow.enabled ? 'var(--success-bg)' : 'var(--surface-subtle)', border: '1px solid var(--border)' }}>
                <div className="u-fs-fs-sm u-fw-600 u-mb-4px" style={{ color: 'var(--text)' }}>{formatAutopayMethod(selectedRow.status)}</div>
                <div className="u-fs-fs-xs u-lh-1_5" style={{ color: 'var(--text-muted)' }}>
                  Next charge: {formatDateOnly(selectedRow.nextChargeDate)} · {formatCurrencyCents(selectedRow.nextChargeAmountCents)}
                </div>
              </div>

              <div>
                <label className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                  Payment type
                </label>
                <select value={form.paymentMethodType} onChange={(event) => setForm((current) => ({ ...current, paymentMethodType: event.target.value === 'ach' ? 'ach' : 'card' }))} style={FIELD_INPUT_STYLE}>
                  <option value="card">Card</option>
                  <option value="ach">ACH</option>
                </select>
              </div>

              <div>
                <label className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                  Saved label
                </label>
                <input aria-label="Visa •••• 4242" value={form.paymentMethodLabel} onChange={(event) => setForm((current) => ({ ...current, paymentMethodLabel: event.target.value }))} style={FIELD_INPUT_STYLE} placeholder="Visa •••• 4242" />
              </div>

              <div className="cols-2 u-gap-12px" >
                <div>
                  <label className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                    {form.paymentMethodType === 'ach' ? 'Bank Name' : 'Brand'}
                  </label>
                  <input value={form.paymentMethodBrand} onChange={(event) => setForm((current) => ({ ...current, paymentMethodBrand: event.target.value }))} style={FIELD_INPUT_STYLE} placeholder={form.paymentMethodType === 'ach' ? 'Chase Bank' : 'Visa'} />
                </div>
                <div>
                  <label className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                    {form.paymentMethodType === 'ach' ? 'Reference' : 'Last 4'}
                  </label>
                  <input
                    value={form.paymentMethodType === 'ach' ? form.paymentMethodExternalRef : form.paymentMethodLast4}
                    onChange={(event) => setForm((current) => (
                      current.paymentMethodType === 'ach'
                        ? { ...current, paymentMethodExternalRef: event.target.value }
                        : { ...current, paymentMethodLast4: event.target.value }
                    ))}
                    style={FIELD_INPUT_STYLE}
                    placeholder={form.paymentMethodType === 'ach' ? 'bank_12345' : '4242'}
                  />
                </div>
              </div>

              {form.paymentMethodType === 'card' ? (
                <div className="cols-2 u-gap-12px" >
                  <div>
                    <label className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                      Expiry month
                    </label>
                    <input aria-label="09" value={form.paymentMethodExpMonth} onChange={(event) => setForm((current) => ({ ...current, paymentMethodExpMonth: event.target.value }))} style={FIELD_INPUT_STYLE} placeholder="09" />
                  </div>
                  <div>
                    <label className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                      Expiry year
                    </label>
                    <input aria-label="2028" value={form.paymentMethodExpYear} onChange={(event) => setForm((current) => ({ ...current, paymentMethodExpYear: event.target.value }))} style={FIELD_INPUT_STYLE} placeholder="2028" />
                  </div>
                </div>
              ) : null}

              <div>
                <label className="u-fs-fs-sm u-ff-font-body u-mb-8px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                  Charge timing
                </label>
                <p className="u-fs-fs-xs u-mb-8px u-lh-1_5" style={{ color: 'var(--text-muted)' }}>
                  Charging early means the family&apos;s card or bank account is billed a few days before the invoice is actually due.
                </p>
                <div className="u-gap-8px" style={{ display: 'flex', flexWrap: 'wrap' }}>
                  {[
                    { value: 0, label: 'Due date' },
                    { value: 3, label: '3 days early' },
                    { value: 7, label: '7 days early' },
                  ].map((option) => (
                    <button type="button"
                      key={option.value}
                      onClick={() => setForm((current) => ({ ...current, triggerDaysBeforeDue: option.value as 0 | 3 | 7 }))}
                      className="u-p-8px-12px u-fs-fs-sm" style={{ borderRadius: '999px', border: `1px solid ${form.triggerDaysBeforeDue === option.value ? 'var(--text)' : 'var(--border)'}`, background: form.triggerDaysBeforeDue === option.value ? 'var(--text)' : '#fff', color: form.triggerDaysBeforeDue === option.value ? '#fff' : 'var(--text-muted)', cursor: 'pointer' }}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
              </div>

              <div className="u-gap-8px" style={{ display: 'flex', flexWrap: 'wrap' }}>
                <button type="button" className="btn btn-primary" onClick={saveAutopay} disabled={saving}>
                  {saving ? 'Saving…' : selectedRow.enabled ? 'Update Autopay' : 'Enable Autopay'}
                </button>
                {selectedRow.enabled ? (
                  <button type="button" className="btn btn-secondary" onClick={disableAutopay} disabled={saving}>
                    Opt Out
                  </button>
                ) : null}
              </div>
            </div>
          ) : (
            <p className="page-subtitle">Select a household to edit.</p>
          )}
        </div>
      </div>
    </div>
  )
}
