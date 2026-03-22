'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchInvoicesForHousehold,
  fetchInvoicePayments,
  type InvoiceRecord,
  type PaymentRecord,
} from '@/lib/adminData'

// ── Helpers ──────────────────────────────────────────────────────────────────
function fmtDate(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function cents(n: number): string {
  return (n / 100).toFixed(2)
}

// ── Skeleton ─────────────────────────────────────────────────────────────────
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

// ── Status config ─────────────────────────────────────────────────────────────
const STATUS_CONFIG: Record<string, { label: string; bg: string; color: string }> = {
  draft:            { label: 'Draft',     bg: '#f9fafb', color: '#9A9693' },
  issued:           { label: 'Due',       bg: '#fffbeb', color: '#d97706' },
  partially_paid:   { label: 'Partial',   bg: '#eff6ff', color: '#2563eb' },
  paid:             { label: 'Paid',      bg: '#ecfdf5', color: '#059669' },
  refunded_partial: { label: 'Refunded',  bg: '#f5f3ff', color: '#7c3aed' },
  refunded_full:    { label: 'Refunded',  bg: '#f5f3ff', color: '#7c3aed' },
  voided:           { label: 'Voided',    bg: '#f9fafb', color: '#9A9693' },
  written_off:      { label: 'Written Off', bg: '#f9fafb', color: '#9A9693' },
}

// ── Autopay Component (local state only — no backend endpoint yet) ──────────
function AutopayTab() {
  const [autopayEnabled, setAutopayEnabled] = useState(false)
  const [payDay, setPayDay] = useState('due')
  const [payMethod, setPayMethod] = useState('visa-4321')
  const [addCard, setAddCard] = useState(false)

  const PAYMENT_METHODS = [
    { id: 'visa-4321', label: 'Visa ····4321', icon: 'credit-card' as const, expires: '09/27', isDefault: true },
    { id: 'ach',       label: 'ACH Transfer', icon: 'dollar-sign' as const, expires: null,    isDefault: false },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '20px', animation: 'slideUp 0.3s ease both' }}>
      <div
        style={{
          background: autopayEnabled ? 'linear-gradient(135deg, #ecfdf5, #d1fae5)' : '#ffffff',
          border: `1px solid ${autopayEnabled ? 'rgba(5,150,105,0.2)' : 'rgba(0,0,0,0.08)'}`,
          borderRadius: '14px',
          padding: '20px 24px',
          display: 'flex',
          alignItems: 'center',
          gap: '16px',
          transition: 'all 200ms ease',
        }}
      >
        <div style={{ flex: 1 }}>
          <div style={{ fontWeight: 700, fontSize: '15px', color: '#1A1714', marginBottom: '4px' }}>
            Automatic Payments
          </div>
          <div style={{ fontSize: '13px', color: '#6B6866', lineHeight: 1.5 }}>
            {autopayEnabled
              ? 'Autopay is active. Invoices will be paid automatically on the due date using your saved payment method.'
              : 'Enable autopay to automatically pay invoices on their due date. You can disable at any time.'}
          </div>
        </div>
        <button
          onClick={() => setAutopayEnabled(!autopayEnabled)}
          style={{
            position: 'relative', width: '48px', height: '26px', borderRadius: '13px',
            background: autopayEnabled ? '#059669' : '#e5e7eb',
            border: 'none', cursor: 'pointer', flexShrink: 0,
            transition: 'background 200ms ease',
          }}
        >
          <div style={{
            position: 'absolute', top: '3px',
            left: autopayEnabled ? '25px' : '3px',
            width: '20px', height: '20px', borderRadius: '50%',
            background: '#fff', boxShadow: '0 1px 3px rgba(0,0,0,0.2)',
            transition: 'left 200ms ease',
          }} />
        </button>
      </div>

      {autopayEnabled && (
        <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', padding: '20px 24px' }}>
          <div style={{ fontWeight: 600, fontSize: '14px', color: '#1A1714', marginBottom: '16px' }}>Autopay Settings</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
            <div>
              <label style={{ fontSize: '12px', color: '#6B6866', display: 'block', marginBottom: '6px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Payment Method</label>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {PAYMENT_METHODS.map((m) => (
                  <button
                    key={m.id}
                    onClick={() => setPayMethod(m.id)}
                    style={{
                      display: 'flex', alignItems: 'center', gap: '12px',
                      padding: '12px 14px', borderRadius: '10px',
                      border: `1.5px solid ${payMethod === m.id ? '#059669' : 'rgba(0,0,0,0.09)'}`,
                      background: payMethod === m.id ? '#ecfdf5' : '#fafaf8',
                      cursor: 'pointer', textAlign: 'left', transition: 'all 150ms ease',
                    }}
                  >
                    <div style={{ width: '32px', height: '22px', background: '#e5e7eb', borderRadius: '4px', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                      <Icon name={m.icon} size={14} style={{ color: '#6B6866' }} />
                    </div>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontWeight: 600, fontSize: '13px', color: '#1A1714' }}>{m.label}</div>
                      {m.expires && <div style={{ fontSize: '11px', color: '#9A9693' }}>Expires {m.expires}</div>}
                    </div>
                    {payMethod === m.id && (
                      <Icon name="check-circle" size={16} style={{ color: '#059669', flexShrink: 0 }} />
                    )}
                  </button>
                ))}
                {!addCard && (
                  <button
                    onClick={() => setAddCard(true)}
                    style={{
                      display: 'flex', alignItems: 'center', gap: '8px',
                      padding: '10px 14px', borderRadius: '10px',
                      border: '1.5px dashed rgba(0,0,0,0.12)',
                      background: 'transparent', cursor: 'pointer', fontSize: '13px', color: '#6B6866',
                    }}
                  >
                    <Icon name="plus" size={14} /> Add new payment method
                  </button>
                )}
              </div>
            </div>
            <div>
              <label style={{ fontSize: '12px', color: '#6B6866', display: 'block', marginBottom: '6px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Auto-pay Trigger</label>
              <div style={{ display: 'flex', gap: '8px' }}>
                {[{ v: 'due', l: 'On due date' }, { v: '3', l: '3 days early' }, { v: '7', l: '7 days early' }].map((opt) => (
                  <button
                    key={opt.v}
                    onClick={() => setPayDay(opt.v)}
                    style={{
                      padding: '7px 14px', borderRadius: '8px', cursor: 'pointer',
                      background: payDay === opt.v ? '#1A1714' : '#f4f4f4',
                      color: payDay === opt.v ? '#fff' : '#6B6866',
                      border: `1px solid ${payDay === opt.v ? '#1A1714' : 'rgba(0,0,0,0.08)'}`,
                      fontSize: '12.5px', fontWeight: 500, transition: 'all 150ms ease',
                    }}
                  >{opt.l}</button>
                ))}
              </div>
            </div>
          </div>
          <div style={{ marginTop: '16px', padding: '12px 14px', background: '#f0fdf4', border: '1px solid rgba(5,150,105,0.15)', borderRadius: '8px', fontSize: '12px', color: '#065f46', lineHeight: 1.5 }}>
            You will receive an email reminder 3 days before any automatic payment is processed. You can pause or cancel autopay at any time before the charge occurs.
          </div>
        </div>
      )}

      {!autopayEnabled && (
        <div style={{ background: '#eff6ff', border: '1px solid rgba(37,99,235,0.15)', borderRadius: '12px', padding: '16px 20px', display: 'flex', gap: '12px' }}>
          <Icon name="info" size={16} style={{ color: '#2563eb', flexShrink: 0, marginTop: '1px' }} />
          <div style={{ fontSize: '13px', color: '#1e40af', lineHeight: 1.6 }}>
            Enabling autopay ensures invoices are never missed. All charges use Stripe&apos;s secure payment processing. Your card details are never stored on our servers.
          </div>
        </div>
      )}
    </div>
  )
}

// ── Component ─────────────────────────────────────────────────────────────────
export default function BillingPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''

  const [invoices, setInvoices] = useState<InvoiceRecord[]>([])
  const [payments, setPayments] = useState<Array<PaymentRecord & { invoiceNumber: string }>>([])
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<'overview' | 'autopay'>('overview')

  useEffect(() => {
    if (!orgId || !householdId) { setLoading(false); return }
    setLoading(true)
    fetchInvoicesForHousehold(api, orgId, householdId)
      .then(async (invs) => {
        setInvoices(invs)
        // Fetch payments for recent paid invoices
        const paidInvs = invs
          .filter((inv) => ['paid', 'partially_paid'].includes(inv.status))
          .slice(0, 5)
        if (paidInvs.length > 0) {
          const arrays = await Promise.all(
            paidInvs.map((inv) =>
              fetchInvoicePayments(api, orgId, inv.id)
                .then((pmts) => pmts.map((p) => ({ ...p, invoiceNumber: inv.invoice_number })))
                .catch(() => [])
            )
          )
          const flat = arrays
            .flat()
            .sort((a, b) => new Date(b.paid_at ?? b.created_at).getTime() - new Date(a.paid_at ?? a.created_at).getTime())
            .slice(0, 5)
          setPayments(flat)
        }
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [api, orgId, householdId])

  // Derived summary
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

      {/* Page header */}
      <div style={{ marginBottom: '20px', animation: 'slideUp 0.3s ease both' }}>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: '#1A1714', margin: '0 0 6px', lineHeight: 1.1 }}>
          Billing
        </h1>
        <p style={{ margin: 0, fontSize: '14px', color: '#9A9693' }}>
          Invoices, payment history, and autopay settings.
        </p>
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', gap: '2px', background: 'rgba(0,0,0,0.04)', borderRadius: '10px', padding: '3px', marginBottom: '24px', width: 'fit-content' }}>
        {[{ id: 'overview', label: 'Overview' }, { id: 'autopay', label: 'Autopay' }].map((t) => (
          <button
            key={t.id}
            onClick={() => setActiveTab(t.id as 'overview' | 'autopay')}
            style={{
              padding: '6px 18px', borderRadius: '8px', border: 'none', cursor: 'pointer',
              background: activeTab === t.id ? '#ffffff' : 'transparent',
              color: activeTab === t.id ? '#1A1714' : '#6B6866',
              fontWeight: activeTab === t.id ? 600 : 400,
              fontSize: '13px',
              boxShadow: activeTab === t.id ? '0 1px 4px rgba(0,0,0,0.08)' : 'none',
              transition: 'all 150ms ease',
            }}
          >
            {t.label}
            {t.id === 'autopay' && (
              <span style={{ marginLeft: '6px', fontSize: '10px', background: '#ecfdf5', color: '#059669', padding: '1px 6px', borderRadius: '8px', fontWeight: 600 }}>New</span>
            )}
          </button>
        ))}
      </div>

      {activeTab === 'autopay' && <AutopayTab />}

      {activeTab === 'overview' && (
        <div>
          {/* Outstanding balance banner */}
          {!loading && openBalance > 0 && (
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
                animation: 'slideUp 0.35s ease both',
              }}
            >
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: '13px', color: '#92400e', fontWeight: 500, marginBottom: '4px' }}>
                  Outstanding Balance
                </div>
                <div style={{ fontFamily: 'var(--font-display)', fontSize: '34px', color: '#92400e', lineHeight: 1, marginBottom: '6px' }}>
                  ${openBalance.toFixed(2)}
                </div>
                {nextDueInvoice?.due_date && (
                  <div style={{ fontSize: '12.5px', color: '#a16207' }}>
                    Due by {fmtDate(nextDueInvoice.due_date)}
                  </div>
                )}
              </div>
              <Link
                href={firstDueInvoiceId ? `/portal/billing/invoices/${firstDueInvoiceId}` : '/portal/billing'}
                style={{ padding: '12px 24px', background: '#d97706', color: '#fff', borderRadius: '10px', fontSize: '14px', fontWeight: 700, textDecoration: 'none', flexShrink: 0, boxShadow: '0 4px 12px rgba(217,119,6,0.3)' }}
              >
                Pay Now
              </Link>
            </div>
          )}

          {/* Stats strip */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px', marginBottom: '28px', animation: 'slideUp 0.4s ease both' }}>
            {loading ? (
              [0, 1, 2].map((i) => (
                <div key={i} style={{ background: '#fff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.07)', padding: '16px 20px' }}>
                  <Skeleton w="60%" h={22} r={4} />
                  <div style={{ marginTop: '8px' }}><Skeleton w="80%" h={12} /></div>
                </div>
              ))
            ) : (
              [
                { label: 'Paid This Season', value: `$${totalPaid.toLocaleString()}`, color: '#059669' },
                { label: 'Open Balance',     value: `$${openBalance.toFixed(2)}`,     color: '#d97706' },
                { label: 'Invoices',         value: String(invoices.length),           color: '#2563eb' },
              ].map((stat) => (
                <div key={stat.label} style={{ background: '#ffffff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.07)', padding: '16px 20px', boxShadow: '0 1px 4px rgba(0,0,0,0.04)' }}>
                  <div style={{ fontSize: '22px', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-mono)' }}>
                    {stat.value}
                  </div>
                  <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '4px' }}>{stat.label}</div>
                </div>
              ))
            )}
          </div>

          {/* Invoices list */}
          <section style={{ marginBottom: '32px', animation: 'slideUp 0.45s ease both' }}>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '14px' }}>
              Invoices
            </div>

            <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden' }}>
              {/* Table header */}
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 100px 100px 100px 36px', padding: '10px 20px', borderBottom: '1px solid rgba(0,0,0,0.07)', background: 'rgba(0,0,0,0.02)' }}>
                {['Invoice', 'Amount', 'Due', 'Status', ''].map((h) => (
                  <span key={h} style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                    {h}
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
              ) : invoices.length === 0 ? (
                <div style={{ padding: '32px', textAlign: 'center', color: '#9A9693', fontSize: '14px' }}>No invoices found.</div>
              ) : (
                invoices.map((inv, i) => {
                  const statusCfg = STATUS_CONFIG[inv.status] ?? STATUS_CONFIG['draft']
                  return (
                    <Link
                      key={inv.id}
                      href={`/portal/billing/invoices/${inv.id}`}
                      style={{
                        display: 'grid', gridTemplateColumns: '1fr 100px 100px 100px 36px',
                        padding: '14px 20px',
                        borderBottom: i < invoices.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                        alignItems: 'center', textDecoration: 'none', transition: 'background 100ms',
                      }}
                      onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.015)')}
                      onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
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

          {/* Payment history */}
          <section style={{ animation: 'slideUp 0.5s ease both' }}>
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
              ) : payments.length === 0 ? (
                <div style={{ padding: '24px', textAlign: 'center', color: '#9A9693', fontSize: '14px' }}>No payment history yet.</div>
              ) : (
                payments.map((payment, i) => (
                  <div
                    key={payment.id}
                    style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 18px', borderBottom: i < payments.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none', gap: '12px' }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                      <div style={{ width: '32px', height: '32px', borderRadius: '50%', background: '#ecfdf5', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                        <Icon name="check-circle" size={15} style={{ color: '#059669' }} />
                      </div>
                      <div>
                        <div style={{ fontSize: '13.5px', color: '#1A1714', fontWeight: 500 }}>
                          {payment.invoiceNumber}
                        </div>
                        <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '2px' }}>
                          {fmtDate(payment.paid_at ?? payment.created_at)} · {payment.method}
                        </div>
                      </div>
                    </div>
                    <span style={{ fontSize: '14px', fontWeight: 700, color: '#059669', fontFamily: 'var(--font-mono)', flexShrink: 0 }}>
                      −${cents(payment.amount_cents)}
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
