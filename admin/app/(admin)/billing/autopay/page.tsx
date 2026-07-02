'use client'

import { useEffect, useState, type CSSProperties } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
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
    padding: '10px 12px',
    borderRadius: '10px',
    border: '1px solid var(--border)',
    background: '#fff',
    fontSize: '13px',
    color: 'var(--text)',
  }
}

export default function BillingAutopayPage() {
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
        if (!cancelled) setError(autopayErrorMessage(loadError, 'Unable to load autopay households right now.'))
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
      setError(autopayErrorMessage(saveError, 'Unable to opt this household out right now.'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={{ padding: '32px', maxWidth: '1180px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '24px', gap: '16px', flexWrap: 'wrap' }}>
        <div>
          <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
            Billing
          </p>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', marginBottom: '4px' }}>
            Autopay
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
            Manage household autopay enrollment, saved billing methods, next scheduled charges, and opt-out handling.
          </p>
        </div>
        <Link href="/billing" className="btn btn-secondary" style={{ textDecoration: 'none', display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
          <Icon name="arrow-left" size={13} /> Back to Billing
        </Link>
      </div>

      {error ? (
        <div style={{ padding: '10px 14px', marginBottom: '18px', background: 'var(--danger-bg)', border: '1px solid var(--danger-border)', borderRadius: '7px', fontSize: '12px', color: 'var(--danger)', fontFamily: 'var(--font-mono)' }}>
          {error}
        </div>
      ) : null}
      {success ? (
        <div style={{ padding: '10px 14px', marginBottom: '18px', background: 'var(--success-bg)', border: '1px solid rgba(5,150,105,0.15)', borderRadius: '7px', fontSize: '12px', color: 'var(--success)', fontFamily: 'var(--font-mono)' }}>
          {success}
        </div>
      ) : null}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Autopay Enabled', value: enabledCount, sub: `${rows.length} tracked households`, color: 'var(--accent)' },
          { label: 'Next Charge Batch', value: scheduledCount, sub: 'households with upcoming charges', color: 'var(--success)' },
          { label: 'Manual Review', value: rows.length - enabledCount, sub: 'households still on manual billing', color: 'var(--text)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '16px 18px' }}>
            <div style={{ fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '24px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{loading ? '—' : stat.value}</div>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>{stat.sub}</div>
          </div>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1.35fr 1fr', gap: '16px', alignItems: 'start' }}>
        <div className="card" style={{ padding: '16px' }}>
          <div style={{ display: 'grid', gridTemplateColumns: '1.3fr 1fr 1fr 0.8fr 0.8fr', gap: '12px', paddingBottom: '10px', borderBottom: '1px solid var(--border)' }}>
            {['Household', 'Saved Method', 'Next Scheduled Charge', 'Status', 'Actions'].map((header) => (
              <div key={header} style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                {header}
              </div>
            ))}
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {loading ? (
              <div style={{ padding: '24px', color: 'var(--text-muted)', fontSize: '13px' }}>Loading households…</div>
            ) : rows.length === 0 ? (
              <div style={{ padding: '24px', color: 'var(--text-muted)', fontSize: '13px' }}>No households available.</div>
            ) : (
              rows.map((row) => (
                <div key={row.householdId} style={{ display: 'grid', gridTemplateColumns: '1.3fr 1fr 1fr 0.8fr 0.8fr', gap: '12px', alignItems: 'center', padding: '14px 0', borderBottom: '1px solid var(--border)' }}>
                  <div>
                    <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{row.householdName}</div>
                    <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '3px' }}>{row.householdId}</div>
                  </div>
                  <div style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.methodLabel}</div>
                  <div>
                    <div style={{ fontSize: '13px', color: 'var(--text)' }}>{formatDate(row.nextChargeDate)}</div>
                    <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '3px' }}>{formatCurrencyCents(row.nextChargeAmountCents)}</div>
                  </div>
                  <span style={{ width: 'fit-content', fontSize: '11px', fontWeight: 600, padding: '3px 10px', borderRadius: '999px', background: row.enabled ? 'var(--success-bg)' : 'var(--warning-bg)', color: row.enabled ? 'var(--success)' : 'var(--warning)', fontFamily: 'var(--font-mono)' }}>
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

        <div className="card" style={{ padding: '18px' }}>
          <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>
            Selected Household
          </div>
          <h2 style={{ fontSize: '18px', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>
            {selectedRow?.householdName ?? 'Choose a household'}
          </h2>
          <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '16px' }}>
            {selectedRow ? `${selectedRow.householdId} · ${selectedRow.enabled ? 'Autopay enabled' : 'Manual billing'}` : 'Select a row to edit autopay settings.'}
          </p>

          {selectedRow ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div style={{ padding: '12px 14px', borderRadius: '10px', background: selectedRow.enabled ? 'var(--success-bg)' : 'var(--surface-subtle)', border: '1px solid var(--border)' }}>
                <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>{formatAutopayMethod(selectedRow.status)}</div>
                <div style={{ fontSize: '11px', color: 'var(--text-muted)', lineHeight: 1.5 }}>
                  Next charge: {formatDate(selectedRow.nextChargeDate)} · {formatCurrencyCents(selectedRow.nextChargeAmountCents)}
                </div>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '6px', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                  Payment Type
                </label>
                <select value={form.paymentMethodType} onChange={(event) => setForm((current) => ({ ...current, paymentMethodType: event.target.value === 'ach' ? 'ach' : 'card' }))} style={inputStyle()}>
                  <option value="card">Card</option>
                  <option value="ach">ACH</option>
                </select>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '6px', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                  Saved Label
                </label>
                <input value={form.paymentMethodLabel} onChange={(event) => setForm((current) => ({ ...current, paymentMethodLabel: event.target.value }))} style={inputStyle()} placeholder="Visa •••• 4242" />
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
                <div>
                  <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '6px', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                    {form.paymentMethodType === 'ach' ? 'Bank Name' : 'Brand'}
                  </label>
                  <input value={form.paymentMethodBrand} onChange={(event) => setForm((current) => ({ ...current, paymentMethodBrand: event.target.value }))} style={inputStyle()} placeholder={form.paymentMethodType === 'ach' ? 'Chase Bank' : 'Visa'} />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '6px', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
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
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
                  <div>
                    <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '6px', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                      Expiry Month
                    </label>
                    <input value={form.paymentMethodExpMonth} onChange={(event) => setForm((current) => ({ ...current, paymentMethodExpMonth: event.target.value }))} style={inputStyle()} placeholder="09" />
                  </div>
                  <div>
                    <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '6px', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                      Expiry Year
                    </label>
                    <input value={form.paymentMethodExpYear} onChange={(event) => setForm((current) => ({ ...current, paymentMethodExpYear: event.target.value }))} style={inputStyle()} placeholder="2028" />
                  </div>
                </div>
              ) : null}

              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '8px', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                  Charge Timing
                </label>
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
                        fontSize: '12px',
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
            <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
              Select a household from the table to edit its billing automation settings.
            </p>
          )}
        </div>
      </div>
    </div>
  )
}
