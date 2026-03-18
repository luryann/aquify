'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'

// ── Stub data ──────────────────────────────────────────────────────────────
const SUMMARY = {
  openBalance: 420.00,
  totalPaidThisSeason: 1840.00,
  creditBalance: 25.00,
  nextDueDate: 'Apr 1, 2026',
}

const INVOICES = [
  {
    id: 'inv-001',
    number: 'INV-2026-0012',
    description: 'Season Registration Fee — 2025–2026',
    amount: 420.00,
    status: 'issued' as const,
    dueDate: 'Apr 1, 2026',
    issuedDate: 'Mar 1, 2026',
    items: 2,
  },
  {
    id: 'inv-002',
    number: 'INV-2026-0007',
    description: 'Tri-City Classic Entry Fees',
    amount: 48.00,
    status: 'paid' as const,
    dueDate: 'Mar 15, 2026',
    issuedDate: 'Feb 28, 2026',
    paidDate: 'Mar 10, 2026',
    items: 6,
  },
  {
    id: 'inv-003',
    number: 'INV-2025-0041',
    description: 'Fall Season Registration',
    amount: 850.00,
    status: 'paid' as const,
    dueDate: 'Sep 15, 2025',
    issuedDate: 'Sep 1, 2025',
    paidDate: 'Sep 12, 2025',
    items: 1,
  },
  {
    id: 'inv-004',
    number: 'INV-2025-0053',
    description: 'Regional Spring Invite Entries',
    amount: 64.00,
    status: 'paid' as const,
    dueDate: 'Jan 30, 2026',
    issuedDate: 'Jan 15, 2026',
    paidDate: 'Jan 28, 2026',
    items: 8,
  },
]

const RECENT_PAYMENTS = [
  { date: 'Mar 10, 2026', description: 'Tri-City Classic Entry Fees',   amount: 48.00,   method: 'Visa ····4321' },
  { date: 'Jan 28, 2026', description: 'Regional Spring Invite Entries', amount: 64.00,   method: 'Visa ····4321' },
  { date: 'Sep 12, 2025', description: 'Fall Season Registration',       amount: 850.00,  method: 'ACH Transfer' },
]

// ── Helpers ─────────────────────────────────────────────────────────────────
const STATUS_CONFIG = {
  draft:            { label: 'Draft',     bg: '#f9fafb', color: '#9A9693' },
  issued:           { label: 'Due',       bg: '#fffbeb', color: '#d97706' },
  partially_paid:   { label: 'Partial',   bg: '#eff6ff', color: '#2563eb' },
  paid:             { label: 'Paid',      bg: '#ecfdf5', color: '#059669' },
  refunded_partial: { label: 'Refunded',  bg: '#f5f3ff', color: '#7c3aed' },
  refunded_full:    { label: 'Refunded',  bg: '#f5f3ff', color: '#7c3aed' },
  voided:           { label: 'Voided',    bg: '#f9fafb', color: '#9A9693' },
  written_off:      { label: 'Written Off', bg: '#f9fafb', color: '#9A9693' },
}

// ── Autopay Component ─────────────────────────────────────────────────────────
function AutopayTab() {
  const [autopayEnabled, setAutopayEnabled] = useState(false)
  const [payDay, setPayDay] = useState('1')
  const [payMethod, setPayMethod] = useState('visa-4321')
  const [addCard, setAddCard] = useState(false)

  const PAYMENT_METHODS = [
    { id: 'visa-4321', label: 'Visa ····4321', icon: 'credit-card' as const, expires: '09/27', isDefault: true },
    { id: 'ach',       label: 'ACH Transfer', icon: 'dollar-sign' as const, expires: null,    isDefault: false },
  ]

  const SCHEDULED = autopayEnabled ? [
    { desc: 'Season Registration Fee — 2025–2026', amount: 420.00, date: 'Apr 1, 2026', method: 'Visa ····4321' },
  ] : []

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '20px', animation: 'slideUp 0.3s ease both' }}>
      {/* Enable/disable toggle */}
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

      {/* Autopay settings (shown when enabled) */}
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

      {/* Scheduled payments */}
      {SCHEDULED.length > 0 && (
        <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', overflow: 'hidden' }}>
          <div style={{ padding: '14px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)', fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
            Scheduled Payments
          </div>
          {SCHEDULED.map((s, i) => (
            <div key={i} style={{ display: 'flex', alignItems: 'center', gap: '14px', padding: '14px 20px', borderBottom: '1px solid rgba(0,0,0,0.05)' }}>
              <div style={{ width: '36px', height: '36px', background: '#fffbeb', borderRadius: '8px', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                <Icon name="clock" size={16} style={{ color: '#d97706' }} />
              </div>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: '13px', fontWeight: 500, color: '#1A1714', marginBottom: '2px' }}>{s.desc}</div>
                <div style={{ fontSize: '11.5px', color: '#9A9693' }}>Scheduled {s.date} · {s.method}</div>
              </div>
              <div style={{ textAlign: 'right', flexShrink: 0 }}>
                <div style={{ fontFamily: 'var(--font-mono)', fontWeight: 700, color: '#1A1714', fontSize: '14px' }}>${s.amount.toFixed(2)}</div>
                <div style={{ fontSize: '11px', color: '#d97706', fontWeight: 600 }}>Pending</div>
              </div>
            </div>
          ))}
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

// ── Component ────────────────────────────────────────────────────────────────
export default function BillingPage() {
  const [activeTab, setActiveTab] = useState<'overview' | 'autopay'>('overview')

  return (
    <div>
      {/* Page header */}
      <div style={{ marginBottom: '20px', animation: 'slideUp 0.3s ease both' }}>
        <h1
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: '30px',
            color: '#1A1714',
            margin: '0 0 6px',
            lineHeight: 1.1,
          }}
        >
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
          >{t.label}{t.id === 'autopay' && <span style={{ marginLeft: '6px', fontSize: '10px', background: '#ecfdf5', color: '#059669', padding: '1px 6px', borderRadius: '8px', fontWeight: 600 }}>New</span>}</button>
        ))}
      </div>

      {activeTab === 'autopay' && <AutopayTab />}

      {activeTab === 'overview' && <div>

      {/* Outstanding balance banner */}
      {SUMMARY.openBalance > 0 && (
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
            <div
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: '34px',
                color: '#92400e',
                lineHeight: 1,
                marginBottom: '6px',
              }}
            >
              ${SUMMARY.openBalance.toFixed(2)}
            </div>
            <div style={{ fontSize: '12.5px', color: '#a16207' }}>
              Due by {SUMMARY.nextDueDate}
            </div>
          </div>
          <Link
            href="/portal/billing/invoices/inv-001"
            style={{
              padding: '12px 24px',
              background: '#d97706',
              color: '#fff',
              borderRadius: '10px',
              fontSize: '14px',
              fontWeight: 700,
              textDecoration: 'none',
              flexShrink: 0,
              boxShadow: '0 4px 12px rgba(217,119,6,0.3)',
            }}
          >
            Pay Now
          </Link>
        </div>
      )}

      {/* Stats strip */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(3, 1fr)',
          gap: '12px',
          marginBottom: '28px',
          animation: 'slideUp 0.4s ease both',
        }}
      >
        {[
          { label: 'Paid This Season', value: `$${SUMMARY.totalPaidThisSeason.toLocaleString()}`, color: '#059669' },
          { label: 'Open Balance',     value: `$${SUMMARY.openBalance.toFixed(2)}`,               color: '#d97706' },
          { label: 'Account Credit',   value: `$${SUMMARY.creditBalance.toFixed(2)}`,             color: '#2563eb' },
        ].map((stat) => (
          <div
            key={stat.label}
            style={{
              background: '#ffffff',
              borderRadius: '12px',
              border: '1px solid rgba(0,0,0,0.07)',
              padding: '16px 20px',
              boxShadow: '0 1px 4px rgba(0,0,0,0.04)',
            }}
          >
            <div
              style={{
                fontSize: '22px',
                fontWeight: 700,
                color: stat.color,
                fontFamily: 'var(--font-mono)',
              }}
            >
              {stat.value}
            </div>
            <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '4px' }}>{stat.label}</div>
          </div>
        ))}
      </div>

      {/* Invoices list */}
      <section style={{ marginBottom: '32px', animation: 'slideUp 0.45s ease both' }}>
        <div
          style={{
            fontSize: '11px',
            fontFamily: 'var(--font-mono)',
            color: '#9A9693',
            textTransform: 'uppercase',
            letterSpacing: '0.08em',
            marginBottom: '14px',
          }}
        >
          Invoices
        </div>

        <div
          style={{
            background: '#ffffff',
            borderRadius: '14px',
            border: '1px solid rgba(0,0,0,0.07)',
            boxShadow: '0 1px 6px rgba(0,0,0,0.04)',
            overflow: 'hidden',
          }}
        >
          {/* Table header */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr 100px 100px 100px 36px',
              padding: '10px 20px',
              borderBottom: '1px solid rgba(0,0,0,0.07)',
              background: 'rgba(0,0,0,0.02)',
            }}
          >
            {['Invoice', 'Amount', 'Due', 'Status', ''].map((h) => (
              <span
                key={h}
                style={{
                  fontSize: '11px',
                  fontFamily: 'var(--font-mono)',
                  color: '#9A9693',
                  textTransform: 'uppercase',
                  letterSpacing: '0.06em',
                }}
              >
                {h}
              </span>
            ))}
          </div>

          {/* Rows */}
          {INVOICES.map((inv, i) => {
            const statusCfg = STATUS_CONFIG[inv.status]
            return (
              <Link
                key={inv.id}
                href={`/portal/billing/invoices/${inv.id}`}
                style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 100px 100px 100px 36px',
                  padding: '14px 20px',
                  borderBottom: i < INVOICES.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                  alignItems: 'center',
                  textDecoration: 'none',
                  transition: 'background 100ms',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.015)')}
                onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
              >
                <div>
                  <div style={{ fontSize: '13.5px', color: '#1A1714', fontWeight: 500, lineHeight: 1.3 }}>
                    {inv.description}
                  </div>
                  <div
                    style={{
                      fontSize: '11.5px',
                      color: '#9A9693',
                      fontFamily: 'var(--font-mono)',
                      marginTop: '2px',
                    }}
                  >
                    {inv.number}
                  </div>
                </div>
                <span
                  style={{
                    fontSize: '14px',
                    fontWeight: 700,
                    color: '#1A1714',
                    fontFamily: 'var(--font-mono)',
                  }}
                >
                  ${inv.amount.toFixed(2)}
                </span>
                <span style={{ fontSize: '12.5px', color: '#6B6866' }}>{inv.dueDate}</span>
                <span
                  style={{
                    display: 'inline-block',
                    padding: '3px 9px',
                    borderRadius: '999px',
                    fontSize: '11.5px',
                    fontWeight: 600,
                    background: statusCfg.bg,
                    color: statusCfg.color,
                  }}
                >
                  {statusCfg.label}
                </span>
                <Icon name="chevron-right" size={14} style={{ color: '#C0BDBB' }} />
              </Link>
            )
          })}
        </div>
      </section>

      {/* Payment history */}
      <section style={{ animation: 'slideUp 0.5s ease both' }}>
        <div
          style={{
            fontSize: '11px',
            fontFamily: 'var(--font-mono)',
            color: '#9A9693',
            textTransform: 'uppercase',
            letterSpacing: '0.08em',
            marginBottom: '14px',
          }}
        >
          Recent Payments
        </div>

        <div
          style={{
            background: '#ffffff',
            borderRadius: '12px',
            border: '1px solid rgba(0,0,0,0.07)',
            overflow: 'hidden',
          }}
        >
          {RECENT_PAYMENTS.map((payment, i) => (
            <div
              key={i}
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                padding: '14px 18px',
                borderBottom: i < RECENT_PAYMENTS.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                gap: '12px',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <div
                  style={{
                    width: '32px',
                    height: '32px',
                    borderRadius: '50%',
                    background: '#ecfdf5',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    flexShrink: 0,
                  }}
                >
                  <Icon name="check-circle" size={15} style={{ color: '#059669' }} />
                </div>
                <div>
                  <div style={{ fontSize: '13.5px', color: '#1A1714', fontWeight: 500 }}>
                    {payment.description}
                  </div>
                  <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '2px' }}>
                    {payment.date} · {payment.method}
                  </div>
                </div>
              </div>
              <span
                style={{
                  fontSize: '14px',
                  fontWeight: 700,
                  color: '#059669',
                  fontFamily: 'var(--font-mono)',
                  flexShrink: 0,
                }}
              >
                −${payment.amount.toFixed(2)}
              </span>
            </div>
          ))}
        </div>
      </section>

      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(12px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
      </div>}
    </div>
  )
}
