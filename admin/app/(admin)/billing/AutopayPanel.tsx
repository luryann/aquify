'use client'

import { useEffect, useState, type CSSProperties } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
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

function formatCurrencyCents(value?: number) {
  return typeof value === 'number' ? `$${(value / 100).toFixed(2)}` : '—'
}

function formatDate(value?: string) {
  return value ? new Date(value).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : '—'
}

function inputStyle(): CSSProperties {
  return {
    width: '100%',
    padding: '12px 12px',
    borderRadius: '12px',
    border: '1px solid var(--border)',
    background: 'var(--bg-elevated)',
    fontSize: 'var(--fs-base)',
    color: 'var(--text)',
  }
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
        const nextSelected = householdList[0]?.id ?? ''
        setSelectedHouseholdId(nextSelected)
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

  async function saveAutopay() {
    if (!selectedHouseholdId || !orgId) return
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      await upsertHouseholdAutopay(api, orgId, selectedHouseholdId, autopayPayloadFromForm(form))
      await refreshStatuses(selectedHouseholdId)
      setSuccess('Autopay settings saved.')
    } catch (saveError) {
      setError(autopayErrorMessage(saveError))
    } finally {
      setSaving(false)
    }
  }

  async function disableAutopay() {
    if (!selectedHouseholdId || !orgId) return
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      await optOutHouseholdAutopay(api, orgId, selectedHouseholdId)
      await refreshStatuses(selectedHouseholdId)
      setSuccess('Autopay has been turned off for this household.')
    } catch (saveError) {
      setError(autopayErrorMessage(saveError, 'Couldn’t opt this household out. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      {error ? (
        <div style={{ padding: '12px 16px', marginBottom: '20px', background: 'var(--danger-bg)', border: '1px solid var(--danger-border)', borderRadius: '8px', fontSize: 'var(--fs-sm)', color: 'var(--danger)', fontFamily: 'var(--font-body)' }}>
          {error}
        </div>
      ) : null}
      {success ? (
        <div style={{ padding: '12px 16px', marginBottom: '20px', background: 'var(--success-bg)', border: '1px solid rgba(5,150,105,0.15)', borderRadius: '8px', fontSize: 'var(--fs-sm)', color: 'var(--success)', fontFamily: 'var(--font-body)' }}>
          {success}
        </div>
      ) : null}

      <div className="cols-3" style={{ gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Autopay enabled', value: enabledCount, sub: `${rows.length} tracked households`, color: 'var(--accent)' },
          { label: 'Next charge batch', value: scheduledCount, sub: 'households with upcoming charges', color: 'var(--success)' },
          { label: 'Manual review', value: rows.length - enabledCount, sub: 'households still on manual billing', color: 'var(--text)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '16px 20px' }}>
            <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: 'var(--fs-2xl)', fontFamily: 'var(--font-body)', fontWeight: 700, color: stat.color }}>{loading ? '—' : stat.value}</div>
            <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', marginTop: '2px' }}>{stat.sub}</div>
          </div>
        ))}
      </div>

      <div className="split-pair" style={{ gap: '16px' }}>
        <div className="card" style={{ padding: '16px', overflowX: 'auto' }}>
          <div style={{ display: 'grid', gridTemplateColumns: '1.3fr 1fr 1fr 0.8fr 0.8fr', gap: '12px', paddingBottom: '12px', borderBottom: '1px solid var(--border)', minWidth: '560px' }}>
            {['Household', 'Saved Method', 'Next Scheduled Charge', 'Status', 'Actions'].map((header) => (
              <div key={header} className="col-head">
                {header}
              </div>
            ))}
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {loading ? (
              <div style={{ padding: '24px', color: 'var(--text-muted)', fontSize: 'var(--fs-base)' }}>Loading households…</div>
            ) : rows.length === 0 ? (
              <div style={{ padding: '24px', color: 'var(--text-muted)', fontSize: 'var(--fs-base)' }}>No households available.</div>
            ) : (
              rows.map((row) => (
                <div key={row.householdId} style={{ display: 'grid', gridTemplateColumns: '1.3fr 1fr 1fr 0.8fr 0.8fr', gap: '12px', alignItems: 'center', padding: '16px 0', borderBottom: '1px solid var(--border)', minWidth: '560px' }}>
                  <div>
                    <div style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--text)' }}>{row.householdName}</div>
                  </div>
                  <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>{row.methodLabel}</div>
                  <div>
                    <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text)' }}>{formatDate(row.nextChargeDate)}</div>
                    <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginTop: '4px' }}>{formatCurrencyCents(row.nextChargeAmountCents)}</div>
                  </div>
                  <span className="badge" style={{ width: 'fit-content', background: row.enabled ? 'var(--success-bg)' : 'var(--warning-bg)', color: row.enabled ? 'var(--success)' : 'var(--warning)' }}>
                    {row.enabled ? 'Enabled' : 'Paused'}
                  </span>
                  <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                    <button className="btn btn-secondary" onClick={() => handleSelect(row)}>
                      {selectedHouseholdId === row.householdId ? 'Editing' : 'Manage'}
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="card" style={{ padding: '20px' }}>
          <h2 className="section-title" style={{ marginBottom: '4px' }}>
            {selectedRow?.householdName ?? 'Choose a household'}
          </h2>
          <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginBottom: '16px' }}>
            {selectedRow ? (selectedRow.enabled ? 'Autopay enabled' : 'Manual billing') : 'Select a row to edit.'}
          </p>

          {selectedRow ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div style={{ padding: '12px 16px', borderRadius: '12px', background: selectedRow.enabled ? 'var(--success-bg)' : 'var(--surface-subtle)', border: '1px solid var(--border)' }}>
                <div style={{ fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>{formatAutopayMethod(selectedRow.status)}</div>
                <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', lineHeight: 1.5 }}>
                  Next charge: {formatDate(selectedRow.nextChargeDate)} · {formatCurrencyCents(selectedRow.nextChargeAmountCents)}
                </div>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '6px' }}>
                  Payment type
                </label>
                <select value={form.paymentMethodType} onChange={(event) => setForm((current) => ({ ...current, paymentMethodType: event.target.value === 'ach' ? 'ach' : 'card' }))} style={inputStyle()}>
                  <option value="card">Card</option>
                  <option value="ach">ACH</option>
                </select>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '6px' }}>
                  Saved label
                </label>
                <input value={form.paymentMethodLabel} onChange={(event) => setForm((current) => ({ ...current, paymentMethodLabel: event.target.value }))} style={inputStyle()} placeholder="Visa •••• 4242" />
              </div>

              <div className="cols-2" style={{ gap: '12px' }}>
                <div>
                  <label style={{ display: 'block', fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '6px' }}>
                    {form.paymentMethodType === 'ach' ? 'Bank Name' : 'Brand'}
                  </label>
                  <input value={form.paymentMethodBrand} onChange={(event) => setForm((current) => ({ ...current, paymentMethodBrand: event.target.value }))} style={inputStyle()} placeholder={form.paymentMethodType === 'ach' ? 'Chase Bank' : 'Visa'} />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '6px' }}>
                    {form.paymentMethodType === 'ach' ? 'Reference' : 'Last 4'}
                  </label>
                  <input
                    value={form.paymentMethodType === 'ach' ? form.paymentMethodExternalRef : form.paymentMethodLast4}
                    onChange={(event) => setForm((current) => (
                      current.paymentMethodType === 'ach'
                        ? { ...current, paymentMethodExternalRef: event.target.value }
                        : { ...current, paymentMethodLast4: event.target.value }
                    ))}
                    style={inputStyle()}
                    placeholder={form.paymentMethodType === 'ach' ? 'bank_12345' : '4242'}
                  />
                </div>
              </div>

              {form.paymentMethodType === 'card' ? (
                <div className="cols-2" style={{ gap: '12px' }}>
                  <div>
                    <label style={{ display: 'block', fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '6px' }}>
                      Expiry month
                    </label>
                    <input value={form.paymentMethodExpMonth} onChange={(event) => setForm((current) => ({ ...current, paymentMethodExpMonth: event.target.value }))} style={inputStyle()} placeholder="09" />
                  </div>
                  <div>
                    <label style={{ display: 'block', fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '6px' }}>
                      Expiry year
                    </label>
                    <input value={form.paymentMethodExpYear} onChange={(event) => setForm((current) => ({ ...current, paymentMethodExpYear: event.target.value }))} style={inputStyle()} placeholder="2028" />
                  </div>
                </div>
              ) : null}

              <div>
                <label style={{ display: 'block', fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '8px' }}>
                  Charge timing
                </label>
                <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', marginBottom: '8px', lineHeight: 1.5 }}>
                  Charging early means the family&apos;s card or bank account is billed a few days before the invoice is actually due.
                </p>
                <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                  {[
                    { value: 0, label: 'Due date' },
                    { value: 3, label: '3 days early' },
                    { value: 7, label: '7 days early' },
                  ].map((option) => (
                    <button
                      key={option.value}
                      onClick={() => setForm((current) => ({ ...current, triggerDaysBeforeDue: option.value as 0 | 3 | 7 }))}
                      style={{
                        padding: '8px 12px',
                        borderRadius: '999px',
                        border: `1px solid ${form.triggerDaysBeforeDue === option.value ? 'var(--text)' : 'var(--border)'}`,
                        background: form.triggerDaysBeforeDue === option.value ? 'var(--text)' : '#fff',
                        color: form.triggerDaysBeforeDue === option.value ? '#fff' : 'var(--text-muted)',
                        fontSize: 'var(--fs-sm)',
                        cursor: 'pointer',
                      }}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
              </div>

              <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                <button className="btn btn-primary" onClick={saveAutopay} disabled={saving}>
                  {saving ? 'Saving…' : selectedRow.enabled ? 'Update Autopay' : 'Enable Autopay'}
                </button>
                {selectedRow.enabled ? (
                  <button className="btn btn-secondary" onClick={disableAutopay} disabled={saving}>
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
