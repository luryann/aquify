'use client'

import { useEffect, useState, type CSSProperties, type Dispatch, type SetStateAction } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  fetchHouseholdAutopay,
  fetchInvoicePayments,
  fetchInvoicesForHousehold,
  optOutHouseholdAutopay,
  upsertHouseholdAutopay,
  type HouseholdAutopayStatusRecord,
  type InvoiceRecord,
  type PaymentRecord,
} from '@/lib/adminData'
import {
  DEFAULT_AUTOPAY_FORM,
  autopayErrorMessage,
  autopayFormFromStatus,
  autopayPayloadFromForm,
  formatAutopayMethod,
  type AutopayFormState,
} from '@/lib/autopay'

function fmtDate(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function cents(n: number): string {
  return (n / 100).toFixed(2)
}

function Skeleton({ w = '100%', h = 16, r = 8 }: { w?: string | number; h?: number; r?: number }) {
  return (
    <div
      style={{
        width: w,
        height: h,
        borderRadius: r,
        background: 'linear-gradient(90deg, #f0efee 25%, #e6e5e3 50%, #f0efee 75%)',
        backgroundSize: '200% 100%',
        animation: 'shimmer 1.5s infinite',
      }}
    />
  )
}

const STATUS_CONFIG: Record<string, { label: string; bg: string; color: string }> = {
  draft: { label: 'Draft', bg: '#f9fafb', color: '#9A9693' },
  issued: { label: 'Due', bg: '#fffbeb', color: '#d97706' },
  partially_paid: { label: 'Partial', bg: '#eff6ff', color: '#2563eb' },
  paid: { label: 'Paid', bg: '#ecfdf5', color: '#059669' },
  refunded_partial: { label: 'Refunded', bg: '#f5f3ff', color: '#7c3aed' },
  refunded_full: { label: 'Refunded', bg: '#f5f3ff', color: '#7c3aed' },
  voided: { label: 'Voided', bg: '#f9fafb', color: '#9A9693' },
  written_off: { label: 'Written Off', bg: '#f9fafb', color: '#9A9693' },
}

function fieldLabel(label: string) {
  return (
    <label style={{ fontSize: '12px', color: '#6B6866', display: 'block', marginBottom: '6px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
      {label}
    </label>
  )
}

function inputStyle(): CSSProperties {
  return {
    width: '100%',
    padding: '10px 12px',
    borderRadius: '10px',
    border: '1px solid rgba(0,0,0,0.1)',
    background: '#fff',
    fontSize: '13px',
    color: '#1A1714',
    outline: 'none',
  }
}

function AutopayTab(props: {
  status: HouseholdAutopayStatusRecord | null
  form: AutopayFormState
  setForm: Dispatch<SetStateAction<AutopayFormState>>
  loading: boolean
  saving: boolean
  error: string
  loadError: string
  success: string
  onSave: () => void
  onOptOut: () => void
  onRetry: () => void
}) {
  const { status, form, setForm, loading, saving, error, loadError, success, onSave, onOptOut, onRetry } = props
  const methodSummary = formatAutopayMethod(status)
  const nextCharge = status?.next_charge_date
    ? `${fmtDate(status.next_charge_date)}${typeof status.next_charge_amount_cents === 'number' ? ` • $${cents(status.next_charge_amount_cents)}` : ''}`
    : 'No scheduled charge yet'

  if (loading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
        <div style={{ background: '#fff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', padding: '20px 24px' }}>
          <Skeleton w="40%" h={18} />
          <div style={{ marginTop: '10px' }}><Skeleton w="75%" h={12} /></div>
        </div>
        <div style={{ background: '#fff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', padding: '20px 24px' }}>
          <Skeleton w="28%" h={16} />
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '14px', marginTop: '18px' }}>
            <Skeleton h={42} />
            <Skeleton h={42} />
            <Skeleton h={42} />
            <Skeleton h={42} />
          </div>
        </div>
      </div>
    )
  }

  if (loadError) {
    return (
      <div style={{ background: '#fff5f5', border: '1px solid rgba(220,38,38,0.16)', borderRadius: '14px', padding: '20px 22px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
        <div>
          <div style={{ fontWeight: 700, fontSize: '15px', color: '#991b1b', marginBottom: '4px' }}>
            Autopay settings are unavailable
          </div>
          <div style={{ fontSize: '13px', color: '#b91c1c', lineHeight: 1.6 }}>
            {loadError}
          </div>
        </div>
        <button
          onClick={onRetry}
          style={{ padding: '11px 16px', borderRadius: '10px', border: 'none', background: '#991b1b', color: '#fff', fontSize: '13px', fontWeight: 700, cursor: 'pointer', flexShrink: 0 }}
        >
          Retry
        </button>
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '18px', animation: 'slideUp 0.3s ease both' }}>
      <div
        style={{
          background: status?.enabled ? 'linear-gradient(135deg, #ecfdf5, #d1fae5)' : '#ffffff',
          border: `1px solid ${status?.enabled ? 'rgba(5,150,105,0.2)' : 'rgba(0,0,0,0.08)'}`,
          borderRadius: '14px',
          padding: '20px 24px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: '16px',
        }}
      >
        <div style={{ flex: 1 }}>
          <div style={{ fontWeight: 700, fontSize: '15px', color: '#1A1714', marginBottom: '4px' }}>
            {status?.enabled ? 'Autopay enabled' : 'Autopay paused'}
          </div>
          <div style={{ fontSize: '13px', color: '#6B6866', lineHeight: 1.6 }}>
            {status?.enabled
              ? `Charges will use ${methodSummary}. Next scheduled attempt: ${nextCharge}.`
              : 'Save a payment method and select when invoices should be charged automatically.'}
          </div>
        </div>
        <span
          style={{
            width: 'fit-content',
            padding: '6px 10px',
            borderRadius: '999px',
            fontSize: '11px',
            fontWeight: 700,
            fontFamily: 'var(--font-mono)',
            background: status?.enabled ? '#059669' : '#e5e7eb',
            color: status?.enabled ? '#fff' : '#4b5563',
            flexShrink: 0,
          }}
        >
          {status?.enabled ? 'ACTIVE' : 'OFF'}
        </span>
      </div>

      {error ? (
        <div style={{ background: '#fef2f2', border: '1px solid rgba(220,38,38,0.15)', borderRadius: '12px', padding: '12px 14px', fontSize: '13px', color: '#b91c1c' }}>
          {error}
        </div>
      ) : null}
      {success ? (
        <div style={{ background: '#ecfdf5', border: '1px solid rgba(5,150,105,0.15)', borderRadius: '12px', padding: '12px 14px', fontSize: '13px', color: '#065f46' }}>
          {success}
        </div>
      ) : null}

      <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', padding: '20px 24px' }}>
        <div style={{ fontWeight: 600, fontSize: '14px', color: '#1A1714', marginBottom: '16px' }}>Saved payment method</div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '14px' }}>
          <div>
            {fieldLabel('Payment Type')}
            <select
              value={form.paymentMethodType}
              onChange={(event) => setForm((current) => ({ ...current, paymentMethodType: event.target.value === 'ach' ? 'ach' : 'card' }))}
              style={inputStyle()}
            >
              <option value="card">Card</option>
              <option value="ach">ACH</option>
            </select>
          </div>
          <div>
            {fieldLabel('Saved Label')}
            <input
              value={form.paymentMethodLabel}
              onChange={(event) => setForm((current) => ({ ...current, paymentMethodLabel: event.target.value }))}
              placeholder={form.paymentMethodType === 'ach' ? 'Checking account' : 'Visa •••• 4242'}
              style={inputStyle()}
            />
          </div>
          <div>
            {fieldLabel(form.paymentMethodType === 'ach' ? 'Bank Name' : 'Card Brand')}
            <input
              value={form.paymentMethodBrand}
              onChange={(event) => setForm((current) => ({ ...current, paymentMethodBrand: event.target.value }))}
              placeholder={form.paymentMethodType === 'ach' ? 'Chase Bank' : 'Visa'}
              style={inputStyle()}
            />
          </div>
          <div>
            {fieldLabel(form.paymentMethodType === 'ach' ? 'Reference' : 'Last 4')}
            <input
              value={form.paymentMethodType === 'ach' ? form.paymentMethodExternalRef : form.paymentMethodLast4}
              onChange={(event) => setForm((current) => (
                form.paymentMethodType === 'ach'
                  ? { ...current, paymentMethodExternalRef: event.target.value }
                  : { ...current, paymentMethodLast4: event.target.value }
              ))}
              placeholder={form.paymentMethodType === 'ach' ? 'bank_12345' : '4242'}
              style={inputStyle()}
            />
          </div>
          {form.paymentMethodType === 'card' ? (
            <>
              <div>
                {fieldLabel('Expiry Month')}
                <input
                  value={form.paymentMethodExpMonth}
                  onChange={(event) => setForm((current) => ({ ...current, paymentMethodExpMonth: event.target.value }))}
                  placeholder="09"
                  style={inputStyle()}
                />
              </div>
              <div>
                {fieldLabel('Expiry Year')}
                <input
                  value={form.paymentMethodExpYear}
                  onChange={(event) => setForm((current) => ({ ...current, paymentMethodExpYear: event.target.value }))}
                  placeholder="2028"
                  style={inputStyle()}
                />
              </div>
            </>
          ) : null}
        </div>
      </div>

      <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', padding: '20px 24px' }}>
        <div style={{ fontWeight: 600, fontSize: '14px', color: '#1A1714', marginBottom: '14px' }}>Charge timing</div>
        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          {[
            { value: 0, label: 'On due date' },
            { value: 3, label: '3 days early' },
            { value: 7, label: '7 days early' },
          ].map((option) => (
            <button
              key={option.value}
              onClick={() => setForm((current) => ({ ...current, triggerDaysBeforeDue: option.value as 0 | 3 | 7 }))}
              style={{
                padding: '8px 14px',
                borderRadius: '999px',
                border: `1px solid ${form.triggerDaysBeforeDue === option.value ? '#1A1714' : 'rgba(0,0,0,0.08)'}`,
                background: form.triggerDaysBeforeDue === option.value ? '#1A1714' : '#f7f6f4',
                color: form.triggerDaysBeforeDue === option.value ? '#fff' : '#6B6866',
                fontSize: '12.5px',
                cursor: 'pointer',
              }}
            >
              {option.label}
            </button>
          ))}
        </div>
        <div style={{ marginTop: '16px', padding: '12px 14px', background: '#f0fdf4', border: '1px solid rgba(5,150,105,0.15)', borderRadius: '8px', fontSize: '12px', color: '#065f46', lineHeight: 1.5 }}>
          Next scheduled attempt: <strong>{nextCharge}</strong>
        </div>
      </div>

      <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
        <button
          onClick={onSave}
          disabled={saving}
          style={{
            padding: '12px 18px',
            borderRadius: '10px',
            border: 'none',
            background: '#1A1714',
            color: '#fff',
            fontSize: '13px',
            fontWeight: 700,
            cursor: saving ? 'default' : 'pointer',
            opacity: saving ? 0.7 : 1,
          }}
        >
          {saving ? 'Saving…' : status?.enabled ? 'Update autopay' : 'Enable autopay'}
        </button>
        {status?.enabled ? (
          <button
            onClick={onOptOut}
            disabled={saving}
            style={{
              padding: '12px 18px',
              borderRadius: '10px',
              border: '1px solid rgba(185,28,28,0.16)',
              background: '#fff5f5',
              color: '#b91c1c',
              fontSize: '13px',
              fontWeight: 700,
              cursor: saving ? 'default' : 'pointer',
              opacity: saving ? 0.7 : 1,
            }}
          >
            Opt out
          </button>
        ) : null}
      </div>

      <div style={{ background: '#eff6ff', border: '1px solid rgba(37,99,235,0.15)', borderRadius: '12px', padding: '16px 20px', display: 'flex', gap: '12px' }}>
        <Icon name="info" size={16} style={{ color: '#2563eb', flexShrink: 0, marginTop: '1px' }} />
        <div style={{ fontSize: '13px', color: '#1e40af', lineHeight: 1.6 }}>
          This page stores billing metadata only. Live card numbers are still handled by Stripe and are not stored in the app runtime.
        </div>
      </div>
    </div>
  )
}

export default function BillingPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''

  const [invoices, setInvoices] = useState<InvoiceRecord[]>([])
  const [payments, setPayments] = useState<Array<PaymentRecord & { invoiceNumber: string }>>([])
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<'overview' | 'autopay'>('overview')
  const [autopayStatus, setAutopayStatus] = useState<HouseholdAutopayStatusRecord | null>(null)
  const [autopayForm, setAutopayForm] = useState<AutopayFormState>(DEFAULT_AUTOPAY_FORM)
  const [autopayLoading, setAutopayLoading] = useState(true)
  const [autopaySaving, setAutopaySaving] = useState(false)
  const [billingError, setBillingError] = useState('')
  const [autopayLoadError, setAutopayLoadError] = useState('')
  const [autopayError, setAutopayError] = useState('')
  const [autopaySuccess, setAutopaySuccess] = useState('')
  const [retryKey, setRetryKey] = useState(0)

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!orgId || !householdId) {
        setBillingError('Billing is unavailable because household context is missing.')
        setAutopayLoadError('Autopay is unavailable because household context is missing.')
        setLoading(false)
        setAutopayLoading(false)
        return
      }

      setLoading(true)
      setAutopayLoading(true)
      setBillingError('')
      setAutopayLoadError('')
      setAutopayError('')

      const [invoiceResult, autopayResult] = await Promise.allSettled([
        fetchInvoicesForHousehold(api, orgId, householdId),
        fetchHouseholdAutopay(api, orgId, householdId),
      ])

      if (cancelled) return

      if (invoiceResult.status === 'fulfilled') {
        const invs = invoiceResult.value
        setInvoices(invs)
        const paidInvs = invs.filter((inv) => ['paid', 'partially_paid'].includes(inv.status)).slice(0, 5)
        if (paidInvs.length > 0) {
          const paymentResults = await Promise.allSettled(
            paidInvs.map((inv) =>
              fetchInvoicePayments(api, orgId, inv.id)
                .then((pmts) => pmts.map((payment) => ({ ...payment, invoiceNumber: inv.invoice_number }))),
            ),
          )
          if (!cancelled) {
            const paymentArrays = paymentResults
              .filter((result): result is PromiseFulfilledResult<Array<PaymentRecord & { invoiceNumber: string }>> => result.status === 'fulfilled')
              .map((result) => result.value)
            const hadPaymentFailure = paymentResults.some((result) => result.status === 'rejected')
            if (hadPaymentFailure) {
              reportUIError('portal.billing.payments.load', new Error('One or more invoice payment reads failed.'), 'Unable to load recent payment activity right now.', {
                orgId,
                householdId,
                invoiceIds: paidInvs.map((invoice) => invoice.id),
              })
              setBillingError('Unable to load recent payment activity right now.')
            }
            setPayments(
              paymentArrays
                .flat()
                .sort((a, b) => new Date(b.paid_at ?? b.created_at).getTime() - new Date(a.paid_at ?? a.created_at).getTime())
                .slice(0, 5),
            )
          }
        } else {
          setPayments([])
        }
      } else if (!cancelled) {
        reportUIError('portal.billing.invoices.load', invoiceResult.reason, 'Unable to load billing activity right now.', {
          orgId,
          householdId,
        })
        setBillingError('Unable to load billing activity right now.')
      }

      if (autopayResult.status === 'fulfilled') {
        setAutopayStatus(autopayResult.value)
        setAutopayForm(autopayFormFromStatus(autopayResult.value))
      } else if (!cancelled) {
        reportUIError('portal.billing.autopay.load', autopayResult.reason, 'Unable to load autopay settings right now.', {
          orgId,
          householdId,
        })
        setAutopayStatus(null)
        setAutopayForm(DEFAULT_AUTOPAY_FORM)
        setAutopayLoadError('Unable to load autopay settings right now.')
      }

      if (!cancelled) {
        setLoading(false)
        setAutopayLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, orgId, householdId, retryKey])

  async function saveAutopay() {
    if (!orgId || !householdId) return
    setAutopaySaving(true)
    setAutopayError('')
    setAutopaySuccess('')
    try {
      const status = await upsertHouseholdAutopay(api, orgId, householdId, autopayPayloadFromForm(autopayForm))
      setAutopayStatus(status)
      setAutopayForm(autopayFormFromStatus(status))
      setAutopaySuccess('Autopay settings saved.')
    } catch (error) {
      setAutopayError(autopayErrorMessage(error))
    } finally {
      setAutopaySaving(false)
    }
  }

  async function disableAutopay() {
    if (!orgId || !householdId) return
    setAutopaySaving(true)
    setAutopayError('')
    setAutopaySuccess('')
    try {
      const status = await optOutHouseholdAutopay(api, orgId, householdId)
      setAutopayStatus(status)
      setAutopayForm(autopayFormFromStatus(status))
      setAutopaySuccess('Autopay has been turned off.')
    } catch (error) {
      setAutopayError(autopayErrorMessage(error, 'Unable to opt out of autopay right now.'))
    } finally {
      setAutopaySaving(false)
    }
  }

  const openBalance = invoices
    .filter((inv) => ['issued', 'partially_paid'].includes(inv.status))
    .reduce((sum, inv) => sum + inv.amount_due_cents, 0) / 100
  const totalPaid = invoices.reduce((sum, inv) => sum + inv.amount_paid_cents, 0) / 100
  const nextDueInvoice = invoices
    .filter((inv) => ['issued', 'partially_paid'].includes(inv.status) && inv.due_date)
    .sort((a, b) => new Date(a.due_date!).getTime() - new Date(b.due_date!).getTime())[0]
  const firstDueInvoiceId = nextDueInvoice?.id ?? ''

  return (
    <div>
      <style>{`
        @keyframes slideUp { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
      `}</style>

      <div style={{ marginBottom: '20px', animation: 'slideUp 0.3s ease both' }}>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: '#1A1714', margin: '0 0 6px', lineHeight: 1.1 }}>
          Billing
        </h1>
        <p style={{ margin: 0, fontSize: '14px', color: '#9A9693' }}>
          Invoices, payment history, and autopay settings.
        </p>
      </div>

      <div style={{ display: 'flex', gap: '2px', background: 'rgba(0,0,0,0.04)', borderRadius: '10px', padding: '3px', marginBottom: '24px', width: 'fit-content' }}>
        {[{ id: 'overview', label: 'Overview' }, { id: 'autopay', label: 'Autopay' }].map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id as 'overview' | 'autopay')}
            style={{
              padding: '6px 18px',
              borderRadius: '8px',
              border: 'none',
              cursor: 'pointer',
              background: activeTab === tab.id ? '#ffffff' : 'transparent',
              color: activeTab === tab.id ? '#1A1714' : '#6B6866',
              fontWeight: activeTab === tab.id ? 600 : 400,
              fontSize: '13px',
              boxShadow: activeTab === tab.id ? '0 1px 4px rgba(0,0,0,0.08)' : 'none',
            }}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === 'autopay' ? (
        <AutopayTab
          status={autopayStatus}
          form={autopayForm}
          setForm={setAutopayForm}
          loading={autopayLoading}
          saving={autopaySaving}
          error={autopayError}
          loadError={autopayLoadError}
          success={autopaySuccess}
          onSave={saveAutopay}
          onOptOut={disableAutopay}
          onRetry={() => setRetryKey((current) => current + 1)}
        />
      ) : (
        <div>
          {billingError ? (
            <div style={{ background: '#fef2f2', border: '1px solid rgba(220,38,38,0.15)', borderRadius: '12px', padding: '14px 16px', marginBottom: '18px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
              <div style={{ fontSize: '13px', color: '#b91c1c' }}>
                {billingError}
              </div>
              <button
                onClick={() => setRetryKey((current) => current + 1)}
                style={{ padding: '9px 14px', borderRadius: '9px', border: 'none', background: '#991b1b', color: '#fff', fontSize: '12.5px', fontWeight: 700, cursor: 'pointer', flexShrink: 0 }}
              >
                Retry
              </button>
            </div>
          ) : null}

          {!loading && !billingError && openBalance > 0 && (
            <div
              style={{
                background: 'linear-gradient(135deg, #fef3c7 0%, #fffbeb 100%)',
                border: '1px solid rgba(217,119,6,0.25)',
                borderRadius: '14px',
                padding: '20px 24px',
                marginBottom: '24px',
                display: 'flex',
                alignItems: 'center',
                gap: '20px',
              }}
            >
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: '13px', color: '#92400e', fontWeight: 500, marginBottom: '4px' }}>Outstanding Balance</div>
                <div style={{ fontFamily: 'var(--font-display)', fontSize: '34px', color: '#92400e', lineHeight: 1, marginBottom: '6px' }}>
                  ${openBalance.toFixed(2)}
                </div>
                {nextDueInvoice?.due_date ? <div style={{ fontSize: '12.5px', color: '#a16207' }}>Due by {fmtDate(nextDueInvoice.due_date)}</div> : null}
              </div>
              <Link
                href={firstDueInvoiceId ? `/portal/billing/invoices/${firstDueInvoiceId}` : '/portal/billing'}
                style={{ padding: '12px 24px', background: '#d97706', color: '#fff', borderRadius: '10px', fontSize: '14px', fontWeight: 700, textDecoration: 'none', flexShrink: 0, boxShadow: '0 4px 12px rgba(217,119,6,0.3)' }}
              >
                Pay Now
              </Link>
            </div>
          )}

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px', marginBottom: '28px' }}>
            {loading ? (
              [0, 1, 2].map((i) => (
                <div key={i} style={{ background: '#fff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.07)', padding: '16px 20px' }}>
                  <Skeleton w="60%" h={22} r={4} />
                  <div style={{ marginTop: '8px' }}><Skeleton w="80%" h={12} /></div>
                </div>
              ))
            ) : billingError ? (
              [0, 1, 2].map((i) => (
                <div key={i} style={{ background: '#fff7f7', borderRadius: '12px', border: '1px solid rgba(220,38,38,0.08)', padding: '16px 20px' }}>
                  <div style={{ fontSize: '12px', color: '#b91c1c', fontWeight: 600 }}>Unavailable</div>
                  <div style={{ fontSize: '12px', color: '#ef4444', marginTop: '4px' }}>Retry to load billing totals.</div>
                </div>
              ))
            ) : (
              [
                { label: 'Paid This Season', value: `$${totalPaid.toLocaleString()}`, color: '#059669' },
                { label: 'Open Balance', value: `$${openBalance.toFixed(2)}`, color: '#d97706' },
                { label: 'Invoices', value: String(invoices.length), color: '#2563eb' },
              ].map((stat) => (
                <div key={stat.label} style={{ background: '#ffffff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.07)', padding: '16px 20px', boxShadow: '0 1px 4px rgba(0,0,0,0.04)' }}>
                  <div style={{ fontSize: '22px', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-mono)' }}>{stat.value}</div>
                  <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '4px' }}>{stat.label}</div>
                </div>
              ))
            )}
          </div>

          <section style={{ marginBottom: '32px' }}>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '14px' }}>
              Invoices
            </div>

            <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden' }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 100px 100px 100px 36px', padding: '10px 20px', borderBottom: '1px solid rgba(0,0,0,0.07)', background: 'rgba(0,0,0,0.02)' }}>
                {['Invoice', 'Amount', 'Due', 'Status', ''].map((header) => (
                  <span key={header} style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                    {header}
                  </span>
                ))}
              </div>

              {loading ? (
                [0, 1, 2].map((i) => (
                  <div key={i} style={{ display: 'grid', gridTemplateColumns: '1fr 100px 100px 100px 36px', padding: '14px 20px', borderBottom: '1px solid rgba(0,0,0,0.05)', alignItems: 'center', gap: '12px' }}>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                      <Skeleton w="70%" h={14} />
                      <Skeleton w="40%" h={11} />
                    </div>
                    <Skeleton w="80%" h={14} />
                    <Skeleton w="80%" h={13} />
                    <Skeleton w={60} h={24} r={999} />
                    <Skeleton w={14} h={14} />
                  </div>
                ))
              ) : billingError ? null : invoices.length === 0 ? (
                <div style={{ padding: '32px', textAlign: 'center', color: '#9A9693', fontSize: '14px' }}>No invoices found.</div>
              ) : (
                invoices.map((inv, index) => {
                  const statusCfg = STATUS_CONFIG[inv.status] ?? STATUS_CONFIG.draft
                  return (
                    <Link
                      key={inv.id}
                      href={`/portal/billing/invoices/${inv.id}`}
                      style={{
                        display: 'grid',
                        gridTemplateColumns: '1fr 100px 100px 100px 36px',
                        padding: '14px 20px',
                        borderBottom: index < invoices.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                        alignItems: 'center',
                        textDecoration: 'none',
                      }}
                    >
                      <div>
                        <div style={{ fontSize: '13.5px', color: '#1A1714', fontWeight: 500, lineHeight: 1.3 }}>
                          {inv.notes ?? `Invoice ${inv.invoice_number}`}
                        </div>
                        <div style={{ fontSize: '11.5px', color: '#9A9693', fontFamily: 'var(--font-mono)', marginTop: '2px' }}>
                          {inv.invoice_number}
                        </div>
                      </div>
                      <span style={{ fontSize: '14px', fontWeight: 700, color: '#1A1714', fontFamily: 'var(--font-mono)' }}>
                        ${cents(inv.total_cents)}
                      </span>
                      <span style={{ fontSize: '12.5px', color: '#6B6866' }}>{fmtDate(inv.due_date)}</span>
                      <span style={{ display: 'inline-block', padding: '3px 9px', borderRadius: '999px', fontSize: '11.5px', fontWeight: 600, background: statusCfg.bg, color: statusCfg.color }}>
                        {statusCfg.label}
                      </span>
                      <Icon name="chevron-right" size={14} style={{ color: '#C0BDBB' }} />
                    </Link>
                  )
                })
              )}
            </div>
          </section>

          <section>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '14px' }}>
              Recent Payments
            </div>

            <div style={{ background: '#ffffff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.07)', overflow: 'hidden' }}>
              {loading ? (
                [0, 1, 2].map((i) => (
                  <div key={i} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 18px', borderBottom: i < 2 ? '1px solid rgba(0,0,0,0.05)' : 'none', gap: '12px' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                      <Skeleton w={32} h={32} r={16} />
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                        <Skeleton w={160} h={14} />
                        <Skeleton w={120} h={12} />
                      </div>
                    </div>
                    <Skeleton w={60} h={14} />
                  </div>
                ))
              ) : billingError ? null : payments.length === 0 ? (
                <div style={{ padding: '24px', textAlign: 'center', color: '#9A9693', fontSize: '14px' }}>No payment history yet.</div>
              ) : (
                payments.map((payment, index) => (
                  <div
                    key={payment.id}
                    style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 18px', borderBottom: index < payments.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none', gap: '12px' }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                      <div style={{ width: '32px', height: '32px', borderRadius: '50%', background: '#ecfdf5', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                        <Icon name="check-circle" size={15} style={{ color: '#059669' }} />
                      </div>
                      <div>
                        <div style={{ fontSize: '13.5px', color: '#1A1714', fontWeight: 500 }}>{payment.invoiceNumber}</div>
                        <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '2px' }}>
                          {fmtDate(payment.paid_at ?? payment.created_at)} · {payment.method}
                        </div>
                      </div>
                    </div>
                    <span style={{ fontSize: '14px', fontWeight: 700, color: '#059669', fontFamily: 'var(--font-mono)', flexShrink: 0 }}>
                      -${cents(payment.amount_cents)}
                    </span>
                  </div>
                ))
              )}
            </div>
          </section>
        </div>
      )}
    </div>
  )
}
