'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'

// ── Stub data ──────────────────────────────────────────────────────────────
const INVOICE = {
  id: 'inv-001',
  number: 'INV-2026-0012',
  status: 'issued' as const,
  description: 'Season Registration Fee — 2025–2026',
  issuedDate: 'Mar 1, 2026',
  dueDate: 'Apr 1, 2026',
  total: 420.00,
  amountPaid: 0,
  amountDue: 420.00,
  household: 'Chen Family',
  items: [
    { description: 'Competitive Team Registration — Emma Chen', qty: 1, unitPrice: 250.00, total: 250.00 },
    { description: 'Competitive Team Registration — Noah Chen',  qty: 1, unitPrice: 170.00, total: 170.00 },
  ],
}

const PAYMENTS: { date: string; amount: number; method: string; status: string; receiptId: string }[] = []

const RECEIPTS = [
  { id: 'rec-7811', date: 'Mar 10, 2026', description: 'Payment — Tri-City Classic',  amount: 48.00,  method: 'Visa ····4321' },
  { id: 'rec-6502', date: 'Jan 28, 2026', description: 'Payment — Spring Invite',      amount: 64.00,  method: 'Visa ····4321' },
  { id: 'rec-4120', date: 'Sep 12, 2025', description: 'Payment — Fall Registration',  amount: 850.00, method: 'ACH Transfer' },
]

// ── Helpers ─────────────────────────────────────────────────────────────────
const STATUS_CONFIG = {
  draft:            { label: 'Draft',        bg: '#f9fafb', color: '#9A9693' },
  issued:           { label: 'Payment Due',  bg: '#fffbeb', color: '#d97706' },
  partially_paid:   { label: 'Partially Paid',bg: '#eff6ff', color: '#2563eb' },
  paid:             { label: 'Paid',         bg: '#ecfdf5', color: '#059669' },
  refunded_partial: { label: 'Refunded',     bg: '#f5f3ff', color: '#7c3aed' },
  refunded_full:    { label: 'Refunded',     bg: '#f5f3ff', color: '#7c3aed' },
  voided:           { label: 'Voided',       bg: '#f9fafb', color: '#9A9693' },
  written_off:      { label: 'Written Off',  bg: '#f9fafb', color: '#9A9693' },
}

type Tab = 'summary' | 'payments' | 'receipts'

// ── Component ────────────────────────────────────────────────────────────────
export default function InvoiceDetailPage() {
  const [tab, setTab] = useState<Tab>('summary')
  const [checkingOut, setCheckingOut] = useState(false)

  const statusCfg = STATUS_CONFIG[INVOICE.status]
  const paidPct = INVOICE.total > 0 ? Math.round((INVOICE.amountPaid / INVOICE.total) * 100) : 0

  const handlePay = async () => {
    setCheckingOut(true)
    await new Promise((r) => setTimeout(r, 1200))
    setCheckingOut(false)
    alert('Stripe Checkout would launch here in production.')
  }

  return (
    <div style={{ maxWidth: '780px' }}>
      {/* Breadcrumb */}
      <nav
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          fontSize: '13px',
          color: '#9A9693',
          marginBottom: '24px',
          animation: 'slideUp 0.3s ease both',
        }}
      >
        <Link href="/portal/billing" style={{ color: '#9A9693', textDecoration: 'none' }}>
          Billing
        </Link>
        <span>/</span>
        <span style={{ color: '#1A1714', fontWeight: 500, fontFamily: 'var(--font-mono)', fontSize: '12px' }}>
          {INVOICE.number}
        </span>
      </nav>

      {/* Invoice header */}
      <div
        style={{
          background: '#ffffff',
          borderRadius: '16px',
          border: '1px solid rgba(0,0,0,0.07)',
          boxShadow: '0 2px 12px rgba(0,0,0,0.05)',
          padding: '28px',
          marginBottom: '20px',
          animation: 'slideUp 0.35s ease both',
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '16px', flexWrap: 'wrap', marginBottom: '20px' }}>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '6px' }}>
              <span
                style={{
                  fontFamily: 'var(--font-mono)',
                  fontSize: '12px',
                  color: '#9A9693',
                  letterSpacing: '0.06em',
                }}
              >
                {INVOICE.number}
              </span>
              <span
                style={{
                  padding: '3px 10px',
                  borderRadius: '999px',
                  fontSize: '11.5px',
                  fontWeight: 600,
                  background: statusCfg.bg,
                  color: statusCfg.color,
                  textTransform: 'uppercase',
                  letterSpacing: '0.05em',
                }}
              >
                {statusCfg.label}
              </span>
            </div>
            <h1
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: '22px',
                color: '#1A1714',
                margin: '0 0 4px',
                lineHeight: 1.2,
              }}
            >
              {INVOICE.description}
            </h1>
            <div style={{ fontSize: '13px', color: '#9A9693' }}>
              {INVOICE.household} · Issued {INVOICE.issuedDate} · Due {INVOICE.dueDate}
            </div>
          </div>

          {/* Amount due */}
          <div style={{ textAlign: 'right' }}>
            <div style={{ fontSize: '12px', color: '#9A9693', marginBottom: '4px' }}>Amount Due</div>
            <div
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: '32px',
                color: INVOICE.amountDue > 0 ? '#d97706' : '#059669',
                lineHeight: 1,
              }}
            >
              ${INVOICE.amountDue.toFixed(2)}
            </div>
          </div>
        </div>

        {/* Payment progress bar */}
        {INVOICE.total > 0 && (
          <div style={{ marginBottom: '20px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '6px' }}>
              <span style={{ fontSize: '12px', color: '#9A9693' }}>
                ${INVOICE.amountPaid.toFixed(2)} paid of ${INVOICE.total.toFixed(2)}
              </span>
              <span style={{ fontSize: '12px', color: '#9A9693' }}>{paidPct}%</span>
            </div>
            <div
              style={{
                height: '6px',
                background: 'rgba(0,0,0,0.06)',
                borderRadius: '999px',
                overflow: 'hidden',
              }}
            >
              <div
                style={{
                  height: '100%',
                  width: `${paidPct}%`,
                  background: paidPct === 100 ? '#059669' : '#eb5d1e',
                  borderRadius: '999px',
                  transition: 'width 0.8s ease',
                }}
              />
            </div>
          </div>
        )}

        {/* Pay button */}
        {(INVOICE.status === 'issued' || INVOICE.status === 'partially_paid') && (
          <button
            onClick={handlePay}
            disabled={checkingOut}
            style={{
              width: '100%',
              padding: '14px',
              background: '#eb5d1e',
              color: '#fff',
              border: 'none',
              borderRadius: '11px',
              fontSize: '15px',
              fontWeight: 700,
              cursor: checkingOut ? 'wait' : 'pointer',
              fontFamily: 'var(--font-body)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: '10px',
              boxShadow: '0 4px 14px rgba(235,93,30,0.35)',
              transition: 'opacity 120ms, transform 80ms',
              opacity: checkingOut ? 0.8 : 1,
            }}
            onMouseEnter={(e) => { if (!checkingOut) e.currentTarget.style.transform = 'translateY(-1px)' }}
            onMouseLeave={(e) => { e.currentTarget.style.transform = 'translateY(0)' }}
          >
            {checkingOut ? (
              <>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', opacity: 0.8 }}>⟳</span>
                Redirecting to Checkout…
              </>
            ) : (
              <>
                <Icon name="credit-card" size={18} />
                Pay ${INVOICE.amountDue.toFixed(2)} via Stripe
              </>
            )}
          </button>
        )}
      </div>

      {/* Tabs */}
      <div
        style={{
          display: 'flex',
          gap: '2px',
          marginBottom: '20px',
          background: 'rgba(0,0,0,0.04)',
          borderRadius: '10px',
          padding: '4px',
          animation: 'slideUp 0.4s ease both',
        }}
      >
        {(['summary', 'payments', 'receipts'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              flex: 1,
              padding: '8px 0',
              borderRadius: '7px',
              border: 'none',
              background: tab === t ? '#ffffff' : 'transparent',
              color: tab === t ? '#1A1714' : '#6B6866',
              fontSize: '13.5px',
              fontWeight: tab === t ? 600 : 450,
              cursor: 'pointer',
              boxShadow: tab === t ? '0 1px 4px rgba(0,0,0,0.08)' : 'none',
              transition: 'all 120ms ease',
              fontFamily: 'var(--font-body)',
              textTransform: 'capitalize',
            }}
          >
            {t === 'summary' ? 'Line Items' : t === 'payments' ? 'Payments' : 'Receipt History'}
          </button>
        ))}
      </div>

      {/* ── Line Items ──────────────────────────────────────────────────────── */}
      {tab === 'summary' && (
        <div
          style={{
            background: '#ffffff',
            borderRadius: '14px',
            border: '1px solid rgba(0,0,0,0.07)',
            boxShadow: '0 1px 6px rgba(0,0,0,0.04)',
            overflow: 'hidden',
            animation: 'slideUp 0.3s ease both',
          }}
        >
          {/* Header row */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr 60px 90px 90px',
              padding: '10px 20px',
              borderBottom: '1px solid rgba(0,0,0,0.07)',
              background: 'rgba(0,0,0,0.02)',
            }}
          >
            {['Description', 'Qty', 'Unit', 'Total'].map((h) => (
              <span
                key={h}
                style={{
                  fontSize: '11px',
                  fontFamily: 'var(--font-mono)',
                  color: '#9A9693',
                  textTransform: 'uppercase',
                  letterSpacing: '0.06em',
                  textAlign: h === 'Description' ? 'left' : 'right',
                }}
              >
                {h}
              </span>
            ))}
          </div>

          {/* Items */}
          {INVOICE.items.map((item, i) => (
            <div
              key={i}
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr 60px 90px 90px',
                padding: '14px 20px',
                borderBottom: i < INVOICE.items.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                alignItems: 'center',
              }}
            >
              <span style={{ fontSize: '13.5px', color: '#1A1714' }}>{item.description}</span>
              <span style={{ fontSize: '13.5px', color: '#6B6866', textAlign: 'right', fontFamily: 'var(--font-mono)' }}>
                {item.qty}
              </span>
              <span style={{ fontSize: '13.5px', color: '#6B6866', textAlign: 'right', fontFamily: 'var(--font-mono)' }}>
                ${item.unitPrice.toFixed(2)}
              </span>
              <span style={{ fontSize: '14px', fontWeight: 600, color: '#1A1714', textAlign: 'right', fontFamily: 'var(--font-mono)' }}>
                ${item.total.toFixed(2)}
              </span>
            </div>
          ))}

          {/* Totals */}
          <div
            style={{
              padding: '14px 20px',
              borderTop: '2px solid rgba(0,0,0,0.07)',
              display: 'flex',
              justifyContent: 'flex-end',
              gap: '32px',
            }}
          >
            <div style={{ textAlign: 'right' }}>
              <div style={{ fontSize: '12px', color: '#9A9693', marginBottom: '2px' }}>Subtotal</div>
              <div style={{ fontSize: '14px', fontFamily: 'var(--font-mono)', color: '#1A1714' }}>
                ${INVOICE.total.toFixed(2)}
              </div>
            </div>
            <div style={{ textAlign: 'right' }}>
              <div style={{ fontSize: '12px', color: '#9A9693', marginBottom: '2px' }}>Amount Due</div>
              <div style={{ fontSize: '18px', fontWeight: 700, fontFamily: 'var(--font-mono)', color: '#d97706' }}>
                ${INVOICE.amountDue.toFixed(2)}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ── Payments ────────────────────────────────────────────────────────── */}
      {tab === 'payments' && (
        <div style={{ animation: 'slideUp 0.3s ease both' }}>
          {PAYMENTS.length === 0 ? (
            <div
              style={{
                background: '#ffffff',
                borderRadius: '14px',
                border: '1px solid rgba(0,0,0,0.07)',
                padding: '48px',
                textAlign: 'center',
              }}
            >
              <div
                style={{
                  width: '48px',
                  height: '48px',
                  borderRadius: '50%',
                  background: 'rgba(0,0,0,0.04)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  margin: '0 auto 14px',
                }}
              >
                <Icon name="credit-card" size={22} style={{ color: '#C0BDBB' }} />
              </div>
              <div style={{ fontSize: '15px', color: '#9A9693', fontWeight: 500 }}>No payments recorded yet</div>
              <div style={{ fontSize: '13px', color: '#C0BDBB', marginTop: '6px' }}>
                Payments will appear here once processed.
              </div>
            </div>
          ) : (
            <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', overflow: 'hidden' }}>
              {PAYMENTS.map((p, i) => (
                <div
                  key={i}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    padding: '14px 20px',
                    borderBottom: i < PAYMENTS.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                  }}
                >
                  <div>
                    <div style={{ fontSize: '13.5px', color: '#1A1714', fontWeight: 500 }}>
                      {p.method}
                    </div>
                    <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '2px' }}>{p.date}</div>
                  </div>
                  <span style={{ fontSize: '15px', fontWeight: 700, fontFamily: 'var(--font-mono)', color: '#059669' }}>
                    −${p.amount.toFixed(2)}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* ── Receipt History ──────────────────────────────────────────────────── */}
      {tab === 'receipts' && (
        <div
          style={{
            background: '#ffffff',
            borderRadius: '14px',
            border: '1px solid rgba(0,0,0,0.07)',
            overflow: 'hidden',
            animation: 'slideUp 0.3s ease both',
          }}
        >
          {RECEIPTS.map((rec, i) => (
            <div
              key={rec.id}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '14px',
                padding: '14px 20px',
                borderBottom: i < RECEIPTS.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
              }}
            >
              <div
                style={{
                  width: '34px',
                  height: '34px',
                  borderRadius: '8px',
                  background: '#ecfdf5',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0,
                }}
              >
                <Icon name="file-text" size={15} style={{ color: '#059669' }} />
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: '13.5px', color: '#1A1714', fontWeight: 500 }}>{rec.description}</div>
                <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '2px' }}>
                  {rec.date} · {rec.method}
                  <span
                    style={{
                      marginLeft: '8px',
                      fontFamily: 'var(--font-mono)',
                      fontSize: '10.5px',
                      color: '#C0BDBB',
                    }}
                  >
                    #{rec.id}
                  </span>
                </div>
              </div>
              <div style={{ textAlign: 'right', flexShrink: 0 }}>
                <div
                  style={{
                    fontSize: '14px',
                    fontWeight: 700,
                    fontFamily: 'var(--font-mono)',
                    color: '#059669',
                  }}
                >
                  ${rec.amount.toFixed(2)}
                </div>
                <button
                  style={{
                    marginTop: '3px',
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    fontSize: '11.5px',
                    color: '#eb5d1e',
                    fontWeight: 500,
                    padding: '0',
                    fontFamily: 'var(--font-body)',
                  }}
                >
                  Download PDF
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(12px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  )
}
