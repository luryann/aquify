'use client'

import { useEffect, useState, type CSSProperties, type Dispatch, type SetStateAction } from 'react'
import { PortalStatement } from './PortalStatement'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { parseBackendDate, formatDateOnly } from '@/lib/dates'
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
  return formatDateOnly(iso)
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
  draft: { label: 'Draft', bg: 'var(--bg-surface)', color: 'var(--text-muted)' },
  issued: { label: 'Due', bg: 'var(--warning-bg)', color: 'var(--warning)' },
  partially_paid: { label: 'Partial', bg: 'var(--info-bg)', color: 'var(--info)' },
  paid: { label: 'Paid', bg: 'var(--success-bg)', color: 'var(--success)' },
  refunded_partial: { label: 'Refunded', bg: '#f5f3ff', color: '#7c3aed' },
  refunded_full: { label: 'Refunded', bg: '#f5f3ff', color: '#7c3aed' },
  voided: { label: 'Voided', bg: 'var(--bg-surface)', color: 'var(--text-muted)' },
  written_off: { label: 'Written off', bg: 'var(--bg-surface)', color: 'var(--text-muted)' },
}

function fieldLabel(label: string) {
  return (
    <label style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', display: 'block', marginBottom: '6px', fontFamily: 'var(--font-body)' }}>
      {label}
    </label>
  )
}

function inputStyle(): CSSProperties {
  return {
    width: '100%',
    padding: '12px 12px',
    borderRadius: '12px',
    border: '1px solid rgba(0,0,0,0.1)',
    background: 'var(--bg-elevated)',
    fontSize: 'var(--fs-base)',
    color: 'var(--text)',
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
        <div className="card" style={{ padding: '20px 24px' }}>
          <Skeleton w="40%" h={18} />
          <div style={{ marginTop: '12px' }}><Skeleton w="75%" h={12} /></div>
        </div>
        <div className="card" style={{ padding: '20px 24px' }}>
          <Skeleton w="28%" h={16} />
          <div className="cols-2" style={{ gap: '16px', marginTop: '20px' }}>
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
      <div style={{ background: '#fff5f5', border: '1px solid rgba(220,38,38,0.16)', borderRadius: '12px', padding: '20px 24px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
        <div>
          <div style={{ fontWeight: 700, fontSize: 'var(--fs-md)', color: 'var(--error)', marginBottom: '4px' }}>
            Autopay settings are unavailable
          </div>
          <div style={{ fontSize: 'var(--fs-base)', color: 'var(--error)', lineHeight: 1.6 }}>
            {loadError}
          </div>
        </div>
        <button
          onClick={onRetry}
          style={{ padding: '12px 16px', borderRadius: '12px', border: 'none', background: 'var(--error)', color: 'var(--bg-elevated)', fontSize: 'var(--fs-base)', fontWeight: 700, cursor: 'pointer', flexShrink: 0 }}
        >
          Retry
        </button>
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '20px', animation: 'slideUp 0.3s ease both' }}>
      <div
        style={{
          background: status?.enabled ? 'var(--success-bg)' : 'var(--bg-elevated)',
          border: `1px solid ${status?.enabled ? 'rgba(5,150,105,0.2)' : 'rgba(0,0,0,0.08)'}`,
          borderRadius: '12px',
          padding: '20px 24px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: '16px',
        }}
      >
        <div style={{ flex: 1 }}>
          <div style={{ fontWeight: 700, fontSize: 'var(--fs-md)', color: 'var(--text)', marginBottom: '4px' }}>
            {status?.enabled ? 'Autopay enabled' : 'Autopay paused'}
          </div>
          <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', lineHeight: 1.6 }}>
            {status?.enabled
              ? `Charges will use ${methodSummary}. Next scheduled attempt: ${nextCharge}.`
              : 'Save a payment method and select when invoices should be charged automatically.'}
          </div>
        </div>
        <span
          style={{
            width: 'fit-content',
            padding: '6px 12px',
            borderRadius: '999px',
            fontSize: 'var(--fs-xs)',
            fontWeight: 700,
            fontFamily: 'var(--font-body)',
            background: status?.enabled ? 'var(--success)' : 'var(--border)',
            color: status?.enabled ? 'var(--bg-elevated)' : '#4b5563',
            flexShrink: 0,
          }}
        >
          {status?.enabled ? 'ACTIVE' : 'OFF'}
        </span>
      </div>

      {error ? (
        <div style={{ background: 'var(--error-bg)', border: '1px solid rgba(220,38,38,0.15)', borderRadius: '12px', padding: '12px 16px', fontSize: 'var(--fs-base)', color: 'var(--error)' }}>
          {error}
        </div>
      ) : null}
      {success ? (
        <div style={{ background: 'var(--success-bg)', border: '1px solid rgba(5,150,105,0.15)', borderRadius: '12px', padding: '12px 16px', fontSize: 'var(--fs-base)', color: '#065f46' }}>
          {success}
        </div>
      ) : null}

      <div className="card" style={{ padding: '20px 24px' }}>
        <div style={{ fontWeight: 600, fontSize: 'var(--fs-md)', color: 'var(--text)', marginBottom: '16px' }}>Saved payment method</div>
        <div className="cols-2" style={{ gap: '16px' }}>
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

      <div className="card" style={{ padding: '20px 24px' }}>
        <div style={{ fontWeight: 600, fontSize: 'var(--fs-md)', color: 'var(--text)', marginBottom: '16px' }}>Charge timing</div>
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
                padding: '8px 16px',
                borderRadius: '999px',
                border: `1px solid ${form.triggerDaysBeforeDue === option.value ? 'var(--text)' : 'rgba(0,0,0,0.08)'}`,
                background: form.triggerDaysBeforeDue === option.value ? 'var(--text)' : '#f7f6f4',
                color: form.triggerDaysBeforeDue === option.value ? 'var(--bg-elevated)' : 'var(--text-subtle)',
                fontSize: 'var(--fs-sm)',
                cursor: 'pointer',
              }}
            >
              {option.label}
            </button>
          ))}
        </div>
        <div style={{ marginTop: '16px', padding: '12px 16px', background: 'var(--success-bg)', border: '1px solid rgba(5,150,105,0.15)', borderRadius: '8px', fontSize: 'var(--fs-sm)', color: '#065f46', lineHeight: 1.5 }}>
          Next scheduled attempt: <strong>{nextCharge}</strong>
        </div>
      </div>

      <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
        <button
          onClick={onSave}
          disabled={saving}
          style={{
            padding: '12px 20px',
            borderRadius: '12px',
            border: 'none',
            background: 'var(--text)',
            color: 'var(--bg-elevated)',
            fontSize: 'var(--fs-base)',
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
              padding: '12px 20px',
              borderRadius: '12px',
              border: '1px solid rgba(185,28,28,0.16)',
              background: '#fff5f5',
              color: 'var(--error)',
              fontSize: 'var(--fs-base)',
              fontWeight: 700,
              cursor: saving ? 'default' : 'pointer',
              opacity: saving ? 0.7 : 1,
            }}
          >
            Opt out
          </button>
        ) : null}
      </div>

      <div style={{ background: 'var(--info-bg)', border: '1px solid rgba(37,99,235,0.15)', borderRadius: '12px', padding: '16px 20px', display: 'flex', gap: '12px' }}>
        <Icon name="info" size={16} style={{ color: 'var(--info)', flexShrink: 0, marginTop: '1px' }} />
        <div style={{ fontSize: 'var(--fs-base)', color: '#1e40af', lineHeight: 1.6 }}>
          This page shows billing summary details only. Your card number is handled directly by Stripe and is never stored here.
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
              reportUIError('portal.billing.payments.load', new Error('One or more invoice payment reads failed.'), 'Couldn’t load recent payment activity. Try again.', {
                orgId,
                householdId,
                invoiceIds: paidInvs.map((invoice) => invoice.id),
              })
              setBillingError('Couldn’t load recent payment activity. Try again.')
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
        reportUIError('portal.billing.invoices.load', invoiceResult.reason, 'Couldn’t load billing activity. Try again.', {
          orgId,
          householdId,
        })
        setBillingError('Couldn’t load billing activity. Try again.')
      }

      if (autopayResult.status === 'fulfilled') {
        setAutopayStatus(autopayResult.value)
        setAutopayForm(autopayFormFromStatus(autopayResult.value))
      } else if (!cancelled) {
        reportUIError('portal.billing.autopay.load', autopayResult.reason, 'Couldn’t load autopay settings. Try again.', {
          orgId,
          householdId,
        })
        setAutopayStatus(null)
        setAutopayForm(DEFAULT_AUTOPAY_FORM)
        setAutopayLoadError('Couldn’t load autopay settings. Try again.')
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
      setAutopayError(autopayErrorMessage(error, 'Couldn’t opt out of autopay. Try again.'))
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
    .sort((a, b) => parseBackendDate(a.due_date!).getTime() - parseBackendDate(b.due_date!).getTime())[0]
  const firstDueInvoiceId = nextDueInvoice?.id ?? ''

  return (
    <div>
      <style>{`
        @keyframes slideUp { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
      `}</style>

      <div className="page-heading">
        <h1 className="page-title">
          Billing
        </h1>
      </div>

      <div style={{ display: 'flex', gap: '2px', background: 'rgba(0,0,0,0.04)', borderRadius: '12px', padding: '4px', marginBottom: '24px', width: 'fit-content' }}>
        {[{ id: 'overview', label: 'Overview' }, { id: 'autopay', label: 'Autopay' }].map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id as 'overview' | 'autopay')}
            style={{
              padding: '6px 20px',
              borderRadius: '8px',
              border: 'none',
              cursor: 'pointer',
              background: activeTab === tab.id ? 'var(--bg-elevated)' : 'transparent',
              color: activeTab === tab.id ? 'var(--text)' : 'var(--text-subtle)',
              fontWeight: activeTab === tab.id ? 600 : 400,
              fontSize: 'var(--fs-base)',
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
            <div style={{ background: 'var(--error-bg)', border: '1px solid rgba(220,38,38,0.15)', borderRadius: '12px', padding: '16px 16px', marginBottom: '20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
              <div style={{ fontSize: 'var(--fs-base)', color: 'var(--error)' }}>
                {billingError}
              </div>
              <button
                onClick={() => setRetryKey((current) => current + 1)}
                style={{ padding: '8px 16px', borderRadius: '8px', border: 'none', background: 'var(--error)', color: 'var(--bg-elevated)', fontSize: 'var(--fs-sm)', fontWeight: 700, cursor: 'pointer', flexShrink: 0 }}
              >
                Retry
              </button>
            </div>
          ) : null}

          {!loading && !billingError && openBalance > 0 && (
            <div
              style={{
                background: 'var(--warning-bg)',
                border: '1px solid rgba(217,119,6,0.25)',
                borderRadius: '12px',
                padding: '20px 24px',
                marginBottom: '24px',
                display: 'flex',
                alignItems: 'center',
                gap: '20px',
              }}
            >
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 'var(--fs-base)', color: '#92400e', fontWeight: 500, marginBottom: '4px' }}>Outstanding balance</div>
                <div style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-4xl)', color: '#92400e', lineHeight: 1, marginBottom: '6px' }}>
                  ${openBalance.toFixed(2)}
                </div>
                {nextDueInvoice?.due_date ? <div style={{ fontSize: 'var(--fs-sm)', color: '#a16207' }}>Due by {fmtDate(nextDueInvoice.due_date)}</div> : null}
              </div>
              <Link
                href={firstDueInvoiceId ? `/portal/billing/invoices/${firstDueInvoiceId}` : '/portal/billing'}
                style={{ padding: '12px 24px', background: 'var(--warning)', color: 'var(--bg-elevated)', borderRadius: '12px', fontSize: 'var(--fs-md)', fontWeight: 700, textDecoration: 'none', flexShrink: 0, boxShadow: '0 4px 12px rgba(217,119,6,0.3)' }}
              >
                Pay Now
              </Link>
            </div>
          )}

          <div className="cols-3" style={{ gap: '12px', marginBottom: '24px' }}>
            {loading ? (
              [0, 1, 2].map((i) => (
                <div key={i} className="card" style={{ padding: '16px 20px' }}>
                  <Skeleton w="60%" h={22} r={4} />
                  <div style={{ marginTop: '8px' }}><Skeleton w="80%" h={12} /></div>
                </div>
              ))
            ) : billingError ? (
              [0, 1, 2].map((i) => (
                <div key={i} style={{ background: '#fff7f7', borderRadius: '12px', border: '1px solid rgba(220,38,38,0.08)', padding: '16px 20px' }}>
                  <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--error)', fontWeight: 600 }}>Unavailable</div>
                  <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--error)', marginTop: '4px' }}>Retry to load billing totals.</div>
                </div>
              ))
            ) : (
              [
                { label: 'Paid this season', value: `$${totalPaid.toLocaleString()}`, color: 'var(--success)' },
                { label: 'Open balance', value: `$${openBalance.toFixed(2)}`, color: 'var(--warning)' },
                { label: 'Invoices', value: String(invoices.length), color: 'var(--info)' },
              ].map((stat) => (
                <div key={stat.label} className="card" style={{ padding: '16px 20px' }}>
                  <div style={{ fontSize: 'var(--fs-2xl)', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-body)' }}>{stat.value}</div>
                  <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '4px' }}>{stat.label}</div>
                </div>
              ))
            )}
          </div>

          <section style={{ marginBottom: 'var(--space-8)' }}>
            <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '16px' }}>
              Invoices
            </div>

            <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
              <div className="inv-row inv-row-head" style={{ padding: '12px 20px', borderBottom: '1px solid rgba(0,0,0,0.07)', background: 'rgba(0,0,0,0.02)' }}>
                {['Invoice', 'Amount', 'Due', 'Status', ''].map((header) => (
                  <span key={header} className="col-head">
                    {header}
                  </span>
                ))}
              </div>

              {loading ? (
                [0, 1, 2].map((i) => (
                  <div key={i} className="inv-row" style={{ padding: '16px 20px', borderBottom: '1px solid rgba(0,0,0,0.05)' }}>
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
                <div style={{ padding: '32px', textAlign: 'center', color: 'var(--text-muted)', fontSize: 'var(--fs-md)' }}>No invoices found.</div>
              ) : (
                invoices.map((inv, index) => {
                  const statusCfg = STATUS_CONFIG[inv.status] ?? STATUS_CONFIG.draft
                  return (
                    <Link
                      key={inv.id}
                      href={`/portal/billing/invoices/${inv.id}`}
                      className="inv-row"
                      style={{
                        padding: '16px 20px',
                        borderBottom: index < invoices.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                        alignItems: 'center',
                        textDecoration: 'none',
                      }}
                    >
                      <div>
                        <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text)', fontWeight: 500, lineHeight: 1.3 }}>
                          {inv.notes ?? `Invoice ${inv.invoice_number}`}
                        </div>
                        <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginTop: '2px' }}>
                          {inv.invoice_number}
                        </div>
                      </div>
                      <span style={{ fontSize: 'var(--fs-md)', fontWeight: 700, color: 'var(--text)', fontFamily: 'var(--font-body)' }}>
                        ${cents(inv.total_cents)}
                      </span>
                      <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>{fmtDate(inv.due_date)}</span>
                      <span style={{ display: 'inline-block', justifySelf: 'start', padding: '4px 8px', borderRadius: '999px', fontSize: 'var(--fs-xs)', fontWeight: 600, background: statusCfg.bg, color: statusCfg.color }}>
                        {statusCfg.label}
                      </span>
                      <Icon name="chevron-right" size={14} style={{ color: 'var(--border-strong)' }} />
                    </Link>
                  )
                })
              )}
            </div>
          </section>

          <section>
            <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '16px' }}>
              Recent payments
            </div>

            <div className="card" style={{ overflow: 'hidden' }}>
              {loading ? (
                [0, 1, 2].map((i) => (
                  <div key={i} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 20px', borderBottom: i < 2 ? '1px solid rgba(0,0,0,0.05)' : 'none', gap: '12px' }}>
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
                <div style={{ padding: '24px', textAlign: 'center', color: 'var(--text-muted)', fontSize: 'var(--fs-md)' }}>No payment history yet.</div>
              ) : (
                payments.map((payment, index) => (
                  <div
                    key={payment.id}
                    style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 20px', borderBottom: index < payments.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none', gap: '12px' }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                      <div style={{ width: '32px', height: '32px', borderRadius: '50%', background: 'var(--success-bg)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                        <Icon name="check-circle" size={15} style={{ color: 'var(--success)' }} />
                      </div>
                      <div>
                        <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text)', fontWeight: 500 }}>{payment.invoiceNumber}</div>
                        <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '2px' }}>
                          {fmtDate(payment.paid_at ?? payment.created_at)} · {payment.method}
                        </div>
                      </div>
                    </div>
                    <span style={{ fontSize: 'var(--fs-md)', fontWeight: 700, color: 'var(--success)', fontFamily: 'var(--font-body)', flexShrink: 0 }}>
                      -${cents(payment.amount_cents)}
                    </span>
                  </div>
                ))
              )}
            </div>
          </section>

          <PortalStatement api={api} orgId={orgId} householdId={householdId} />
        </div>
      )}
    </div>
  )
}
