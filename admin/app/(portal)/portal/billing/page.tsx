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
import './page.inline.css'

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
        background: 'linear-gradient(90deg, var(--bg-elevated) 25%, var(--border-strong) 50%, var(--bg-elevated) 75%)',
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
  refunded_partial: { label: 'Refunded', bg: 'var(--water-subtle)', color: 'var(--water)' },
  refunded_full: { label: 'Refunded', bg: 'var(--water-subtle)', color: 'var(--water)' },
  voided: { label: 'Voided', bg: 'var(--bg-surface)', color: 'var(--text-muted)' },
  written_off: { label: 'Written off', bg: 'var(--bg-surface)', color: 'var(--text-muted)' },
}

function fieldLabel(label: string) {
  return (
    <label className="is-4ecc633">
      {label}
    </label>
  )
}

function inputStyle(): CSSProperties {
  return { width: '100%' }
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
      <div className="is-8ef476f">
        <div className="card is-b516f27" >
          <Skeleton w="40%" h={18} />
          <div className="is-9c9cee2"><Skeleton w="75%" h={12} /></div>
        </div>
        <div className="card is-b516f27" >
          <Skeleton w="28%" h={16} />
          <div className="cols-2 is-e31d992" >
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
      <div className="is-d95d6c7">
        <div>
          <div className="is-4f68acd">
            Autopay settings are unavailable
          </div>
          <div className="is-e150e31">
            {loadError}
          </div>
        </div>
        <button type="button"
          onClick={onRetry}
          className="is-1618f1c"
        >
          Retry
        </button>
      </div>
    )
  }

  return (
    <div className="is-4a32804">
      <div
        style={{
          background: status?.enabled ? 'var(--success-bg)' : 'var(--bg-elevated)',
          border: `1px solid ${status?.enabled ? 'var(--success-border)' : 'var(--border)'}`,
          borderRadius: '12px',
          padding: '20px 24px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: '16px',
        }}
      >
        <div style={{ flex: 1 }}>
          <div className="is-4a62c9a">
            {status?.enabled ? 'Autopay enabled' : 'Autopay paused'}
          </div>
          <div className="is-d2e2a79">
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
            color: status?.enabled ? 'var(--bg-elevated)' : 'var(--text-muted)',
            flexShrink: 0,
          }}
        >
          {status?.enabled ? 'ACTIVE' : 'OFF'}
        </span>
      </div>

      {error ? (
        <div className="is-7774b87">
          {error}
        </div>
      ) : null}
      {success ? (
        <div className="is-b444239">
          {success}
        </div>
      ) : null}

      <div className="card is-b516f27" >
        <div className="is-6996b9b">Saved payment method</div>
        <div className="cols-2 is-f192209" >
          <div>
            {fieldLabel('Payment Type')}
            <select aria-label="Payment type"
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
            <input aria-label="Saved label"
              value={form.paymentMethodLabel}
              onChange={(event) => setForm((current) => ({ ...current, paymentMethodLabel: event.target.value }))}
              placeholder={form.paymentMethodType === 'ach' ? 'Checking account' : 'Visa •••• 4242'}
              style={inputStyle()}
            />
          </div>
          <div>
            {fieldLabel(form.paymentMethodType === 'ach' ? 'Bank Name' : 'Card Brand')}
            <input aria-label="Bank name or card brand"
              value={form.paymentMethodBrand}
              onChange={(event) => setForm((current) => ({ ...current, paymentMethodBrand: event.target.value }))}
              placeholder={form.paymentMethodType === 'ach' ? 'Chase Bank' : 'Visa'}
              style={inputStyle()}
            />
          </div>
          <div>
            {fieldLabel(form.paymentMethodType === 'ach' ? 'Reference' : 'Last 4')}
            <input aria-label="Reference or last 4"
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
                <input aria-label="Expiry month"
                  value={form.paymentMethodExpMonth}
                  onChange={(event) => setForm((current) => ({ ...current, paymentMethodExpMonth: event.target.value }))}
                  placeholder="09"
                  style={inputStyle()}
                />
              </div>
              <div>
                {fieldLabel('Expiry Year')}
                <input aria-label="Expiry year"
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

      <div className="card is-b516f27" >
        <div className="is-6996b9b">Charge timing</div>
        <div className="is-51144f2">
          {[
            { value: 0, label: 'On due date' },
            { value: 3, label: '3 days early' },
            { value: 7, label: '7 days early' },
          ].map((option) => (
            <button type="button"
              key={option.value}
              onClick={() => setForm((current) => ({ ...current, triggerDaysBeforeDue: option.value as 0 | 3 | 7 }))}
              style={{
                padding: '8px 16px',
                borderRadius: '999px',
                border: `1px solid ${form.triggerDaysBeforeDue === option.value ? 'var(--text)' : 'var(--border)'}`,
                background: form.triggerDaysBeforeDue === option.value ? 'var(--text)' : 'var(--bg-surface)',
                color: form.triggerDaysBeforeDue === option.value ? 'var(--bg-elevated)' : 'var(--text-subtle)',
                fontSize: 'var(--fs-sm)',
                cursor: 'pointer',
              }}
            >
              {option.label}
            </button>
          ))}
        </div>
        <div className="is-2a2a818">
          Next scheduled attempt: <strong>{nextCharge}</strong>
        </div>
      </div>

      <div className="is-18d4ed1">
        <button type="button"
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
          <button type="button"
            onClick={onOptOut}
            disabled={saving}
            style={{
              padding: '12px 20px',
              borderRadius: '12px',
              border: '1px solid rgba(185,28,28,0.16)',
              background: 'var(--error-bg)',
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

      <div className="is-be964b2">
        <Icon name="info" size={16} style={{ color: 'var(--info)', flexShrink: 0, marginTop: '1px' }} />
        <div className="is-5120349">
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
        setBillingError('We could not find your household details. Sign out and back in, or contact your club if this keeps happening.')
        setAutopayLoadError('We could not find your household details. Sign out and back in, or contact your club if this keeps happening.')
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

      <div className="is-bf3e85c">
        {[{ id: 'overview', label: 'Overview' }, { id: 'autopay', label: 'Autopay' }].map((tab) => (
          <button type="button"
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
            <div className="is-487d05a">
              <div className="is-de107ca">
                {billingError}
              </div>
              <button type="button"
                onClick={() => setRetryKey((current) => current + 1)}
                className="is-9ce49de"
              >
                Retry
              </button>
            </div>
          ) : null}

          {!loading && !billingError && openBalance > 0 && (
            <div
              className="is-c0206d4"
            >
              <div style={{ flex: 1 }}>
                <div className="is-fc1270f">Outstanding balance</div>
                <div className="is-3e1851c">
                  ${openBalance.toFixed(2)}
                </div>
                {nextDueInvoice?.due_date ? <div className="is-743adc2">Due by {fmtDate(nextDueInvoice.due_date)}</div> : null}
              </div>
              <Link
                href={firstDueInvoiceId ? `/portal/billing/invoices/${firstDueInvoiceId}` : '/portal/billing'}
                className="is-294ff90"
              >
                Pay Now
              </Link>
            </div>
          )}

          <div className="cols-3 is-7a1f33f" >
            {loading ? (
              [0, 1, 2].map((i) => (
                <div key={i} className="card is-401e6d8" >
                  <Skeleton w="60%" h={22} r={4} />
                  <div className="is-36ca0af"><Skeleton w="80%" h={12} /></div>
                </div>
              ))
            ) : billingError ? (
              [0, 1, 2].map((i) => (
                <div key={i} className="is-cea1a42">
                  <div className="is-94d1cfe">Unavailable</div>
                  <div className="is-8b90288">Retry to load billing totals.</div>
                </div>
              ))
            ) : (
              [
                { label: 'Paid this season', value: `$${totalPaid.toLocaleString()}`, color: 'var(--success)' },
                { label: 'Open balance', value: `$${openBalance.toFixed(2)}`, color: 'var(--warning)' },
                { label: 'Invoices', value: String(invoices.length), color: 'var(--info)' },
              ].map((stat) => (
                <div key={stat.label} className="card is-401e6d8" >
                  <div style={{ fontSize: 'var(--fs-2xl)', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-body)' }}>{stat.value}</div>
                  <div className="is-dd721e0">{stat.label}</div>
                </div>
              ))
            )}
          </div>

          <section className="is-6a59054">
            <div className="is-483d59c">
              Invoices
            </div>

            <div className="card is-f27afbb" >
              <div className="inv-row inv-row-head is-ec41ca3" >
                {['Invoice', 'Amount', 'Due', 'Status', ''].map((header) => (
                  <span key={header} className="col-head">
                    {header}
                  </span>
                ))}
              </div>

              {loading ? (
                [0, 1, 2].map((i) => (
                  <div key={i} className="inv-row is-541f5a9" >
                    <div className="is-9808a21">
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
                <div className="is-3f7bfac">No invoices found.</div>
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
                        borderBottom: index < invoices.length - 1 ? '1px solid var(--border)' : 'none',
                        alignItems: 'center',
                        textDecoration: 'none',
                      }}
                    >
                      <div>
                        <div className="is-632f865">
                          {inv.notes ?? `Invoice ${inv.invoice_number}`}
                        </div>
                        <div className="is-8d70815">
                          {inv.invoice_number}
                        </div>
                      </div>
                      <span className="is-dbeeba1">
                        ${cents(inv.total_cents)}
                      </span>
                      <span className="is-83e0368">{fmtDate(inv.due_date)}</span>
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
            <div className="is-483d59c">
              Recent payments
            </div>

            <div className="card" style={{ overflow: 'hidden' }}>
              {loading ? (
                [0, 1, 2].map((i) => (
                  <div key={i} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 20px', borderBottom: i < 2 ? '1px solid var(--border)' : 'none', gap: '12px' }}>
                    <div className="is-e2ff0fd">
                      <Skeleton w={32} h={32} r={16} />
                      <div className="is-9808a21">
                        <Skeleton w={160} h={14} />
                        <Skeleton w={120} h={12} />
                      </div>
                    </div>
                    <Skeleton w={60} h={14} />
                  </div>
                ))
              ) : billingError ? null : payments.length === 0 ? (
                <div className="is-3d754c5">No payment history yet.</div>
              ) : (
                payments.map((payment, index) => (
                  <div
                    key={payment.id}
                    style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 20px', borderBottom: index < payments.length - 1 ? '1px solid var(--border)' : 'none', gap: '12px' }}
                  >
                    <div className="is-e2ff0fd">
                      <div style={{ width: '32px', height: '32px', borderRadius: '50%', background: 'var(--success-bg)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                        <Icon name="check-circle" size={15} style={{ color: 'var(--success)' }} />
                      </div>
                      <div>
                        <div className="is-df04e21">{payment.invoiceNumber}</div>
                        <div className="is-7536cb1">
                          {fmtDate(payment.paid_at ?? payment.created_at)} · {payment.method}
                        </div>
                      </div>
                    </div>
                    <span className="is-f8109cc">
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
