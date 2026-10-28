'use client'

import { Suspense, useState, useEffect } from 'react'
import Link from 'next/link'
import { useParams, useSearchParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { formatDateOnly } from '@/lib/dates'
import {
  fetchInvoice,
  fetchInvoiceItems,
  fetchInvoicePayments,
  InvoiceRecord,
  InvoiceItemRecord,
  PaymentRecord,
} from '@/lib/adminData'
import {
  isPortalInvoiceCheckoutDisabled,
  launchPortalInvoiceCheckout,
  portalInvoiceCheckoutErrorMessage,
} from '@/lib/portal-invoice-checkout'

// ── Helpers ─────────────────────────────────────────────────────────────────
const STATUS_CONFIG: Record<string, { label: string; bg: string; color: string }> = {
  draft:            { label: 'Draft',        bg: 'var(--bg-surface)', color: 'var(--text-muted)' },
  issued:           { label: 'Payment due',  bg: 'var(--warning-bg)', color: 'var(--warning)' },
  partially_paid:   { label: 'Partially paid',bg: 'var(--info-bg)', color: 'var(--info)' },
  paid:             { label: 'Paid',         bg: 'var(--success-bg)', color: 'var(--success)' },
  refunded_partial: { label: 'Refunded',     bg: '#f5f3ff', color: '#7c3aed' },
  refunded_full:    { label: 'Refunded',     bg: '#f5f3ff', color: '#7c3aed' },
  voided:           { label: 'Voided',       bg: 'var(--bg-surface)', color: 'var(--text-muted)' },
  written_off:      { label: 'Written off',  bg: 'var(--bg-surface)', color: 'var(--text-muted)' },
}

function centsToDisplay(cents: number): string {
  return (cents / 100).toFixed(2)
}

function formatDate(iso?: string): string {
  return formatDateOnly(iso)
}

type Tab = 'summary' | 'payments' | 'receipts'

// ── Loading skeleton ─────────────────────────────────────────────────────────
function LoadingSkeleton() {
  return (
    <div className="page">
      <div style={{ height: '20px', width: '180px', background: 'rgba(0,0,0,0.06)', borderRadius: '6px', marginBottom: '24px' }} />
      <div
        className="card" style={{ padding: '24px', marginBottom: '20px' }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '24px' }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <div style={{ height: '14px', width: '160px', background: 'rgba(0,0,0,0.06)', borderRadius: '4px' }} />
            <div style={{ height: '24px', width: '300px', background: 'rgba(0,0,0,0.06)', borderRadius: '6px' }} />
            <div style={{ height: '14px', width: '240px', background: 'rgba(0,0,0,0.04)', borderRadius: '4px' }} />
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', alignItems: 'flex-end' }}>
            <div style={{ height: '12px', width: '80px', background: 'rgba(0,0,0,0.04)', borderRadius: '4px' }} />
            <div style={{ height: '36px', width: '100px', background: 'rgba(0,0,0,0.06)', borderRadius: '6px' }} />
          </div>
        </div>
        <div style={{ height: '6px', background: 'rgba(0,0,0,0.06)', borderRadius: '999px', marginBottom: '20px' }} />
        <div style={{ height: '52px', background: 'rgba(0,0,0,0.05)', borderRadius: '12px' }} />
      </div>
      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(12px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  )
}

// ── Component ────────────────────────────────────────────────────────────────
export default function InvoiceDetailPage() {
  return (
    <Suspense fallback={<LoadingSkeleton />}>
      <InvoiceDetailPageContent />
    </Suspense>
  )
}

function InvoiceDetailPageContent() {
  const params = useParams<{ id: string }>()
  const searchParams = useSearchParams()
  const invoiceId = typeof params?.id === 'string' ? params.id : ''

  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [invoice, setInvoice] = useState<InvoiceRecord | null>(null)
  const [items, setItems] = useState<InvoiceItemRecord[]>([])
  const [payments, setPayments] = useState<PaymentRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [tab, setTab] = useState<Tab>('summary')
  const [checkingOut, setCheckingOut] = useState(false)
  const [checkoutError, setCheckoutError] = useState('')
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId || !invoiceId) {
      setLoading(false)
      setLoadError('Invoice detail is unavailable because invoice context is missing.')
      return
    }
    setLoading(true)
    setLoadError(null)
    Promise.all([
      fetchInvoice(api, orgId, invoiceId),
      fetchInvoiceItems(api, orgId, invoiceId),
      fetchInvoicePayments(api, orgId, invoiceId),
    ])
      .then(([inv, its, pmts]) => {
        setInvoice(inv)
        setItems(its)
        setPayments(pmts)
      })
      .catch((error) => {
        setInvoice(null)
        setItems([])
        setPayments([])
        setLoadError(reportUIError(
          'portal-invoice-detail-load',
          error,
          'Invoice detail data is unavailable right now.',
          { orgId, invoiceId },
        ))
      })
      .finally(() => setLoading(false))
  }, [api, orgId, invoiceId, reloadKey])

  if (loading) return <LoadingSkeleton />

  if (!invoice) {
    return (
      <div className="page">
        <nav style={{ marginBottom: '20px', fontSize: 'var(--fs-base)' }}>
          <Link href="/portal/billing" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>
            Billing
          </Link>
        </nav>
        {loadError && (
          <div
            style={{
              background: 'var(--warning-bg)',
              border: '1px solid #fdba74',
              borderRadius: '12px',
              padding: '12px 16px',
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              gap: '12px',
              marginBottom: '20px',
            }}
          >
            <div style={{ display: 'flex', gap: '12px', alignItems: 'flex-start' }}>
              <Icon name="alert-circle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
              <p style={{ margin: 0, fontSize: 'var(--fs-sm)', color: 'var(--warning)', lineHeight: 1.5 }}>{loadError}</p>
            </div>
            <button
              type="button"
              onClick={() => setReloadKey((key) => key + 1)}
              style={{ background: 'var(--bg-elevated)', border: '1px solid #fdba74', borderRadius: '8px', cursor: 'pointer', color: 'var(--warning)', fontSize: 'var(--fs-sm)', fontWeight: 600, padding: '8px 16px', whiteSpace: 'nowrap' }}
            >
              Retry
            </button>
          </div>
        )}
        <div
          className="card" style={{ padding: '48px', textAlign: 'center' }}
        >
          <div style={{ fontSize: 'var(--fs-md)', color: 'var(--text-muted)', fontWeight: 500 }}>
            {loadError ? 'Invoice details are unavailable right now.' : 'Invoice not found'}
          </div>
        </div>
      </div>
    )
  }

  const statusCfg = STATUS_CONFIG[invoice.status] ?? { label: invoice.status, bg: 'var(--bg-surface)', color: 'var(--text-muted)' }
  const total = invoice.total_cents / 100
  const amountPaid = invoice.amount_paid_cents / 100
  const amountDue = invoice.amount_due_cents / 100
  const paidPct = total > 0 ? Math.round((amountPaid / total) * 100) : 0
  const checkoutState = searchParams.get('checkout')
  const checkoutDisabled = isPortalInvoiceCheckoutDisabled(invoice, checkingOut)

  const handlePay = async () => {
    if (!invoice || !orgId || typeof window === 'undefined') return
    setCheckingOut(true)
    setCheckoutError('')
    try {
      await launchPortalInvoiceCheckout({
        api,
        orgId,
        invoice,
        origin: window.location.origin,
        redirect: (url) => window.location.assign(url),
      })
    } catch (error) {
      setCheckingOut(false)
      setCheckoutError(portalInvoiceCheckoutErrorMessage(error))
    }
  }

  return (
    <div className="page">
      {/* Breadcrumb */}
      <nav
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          fontSize: 'var(--fs-base)',
          color: 'var(--text-muted)',
          marginBottom: '24px',
          animation: 'slideUp 0.3s ease both',
        }}
      >
        <Link href="/portal/billing" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>
          Billing
        </Link>
        <span>/</span>
        <span style={{ color: 'var(--text)', fontWeight: 500, fontFamily: 'var(--font-body)', fontSize: 'var(--fs-sm)' }}>
          {invoice.invoice_number}
        </span>
      </nav>

      {/* Invoice header */}
      <div
        className="card" style={{ padding: '24px', marginBottom: '20px', animation: 'slideUp 0.35s ease both' }}
      >
        {checkoutState === 'success' && (
          <div
            style={{
              marginBottom: '16px',
              padding: '12px 16px',
              borderRadius: '12px',
              background: 'var(--info-bg)',
              border: '1px solid rgba(37,99,235,0.15)',
              color: '#1d4ed8',
              fontSize: 'var(--fs-base)',
              lineHeight: 1.5,
            }}
          >
            Stripe returned successfully. If payment confirmation is still in flight, refresh this invoice in a moment to see the updated status.
          </div>
        )}
        {checkoutState === 'cancelled' && (
          <div
            style={{
              marginBottom: '16px',
              padding: '12px 16px',
              borderRadius: '12px',
              background: 'var(--warning-bg)',
              border: '1px solid rgba(217,119,6,0.15)',
              color: '#c2410c',
              fontSize: 'var(--fs-base)',
              lineHeight: 1.5,
            }}
          >
            Checkout was cancelled before payment completed. Your invoice is still open.
          </div>
        )}
        {checkoutError && (
          <div
            style={{
              marginBottom: '16px',
              padding: '12px 16px',
              borderRadius: '12px',
              background: 'var(--error-bg)',
              border: '1px solid rgba(220,38,38,0.15)',
              color: 'var(--error)',
              fontSize: 'var(--fs-base)',
              lineHeight: 1.5,
            }}
          >
            {checkoutError}
          </div>
        )}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '16px', flexWrap: 'wrap', marginBottom: '20px' }}>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
              <h1 className="page-title">
                {invoice.notes ?? invoice.invoice_number}
              </h1>
              <span
                style={{
                  padding: '4px 12px',
                  borderRadius: '999px',
                  fontSize: 'var(--fs-xs)',
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
            <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', marginTop: '4px' }}>
              <span style={{ fontFamily: 'var(--font-mono)' }}>{invoice.invoice_number}</span>
              {invoice.issued_at ? ` · Issued ${formatDate(invoice.issued_at)}` : ''}
              {invoice.due_date ? ` · Due ${formatDate(invoice.due_date)}` : ''}
            </div>
          </div>

          {/* Amount due */}
          <div style={{ textAlign: 'right' }}>
            <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginBottom: '4px' }}>Amount due</div>
            <div
              style={{
                fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)',
                fontSize: 'var(--fs-4xl)',
                color: amountDue > 0 ? 'var(--warning)' : 'var(--success)',
                lineHeight: 1,
              }}
            >
              ${amountDue.toFixed(2)}
            </div>
          </div>
        </div>

        {/* Payment progress bar */}
        {total > 0 && (
          <div style={{ marginBottom: '20px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '6px' }}>
              <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>
                ${amountPaid.toFixed(2)} paid of ${total.toFixed(2)}
              </span>
              <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>{paidPct}%</span>
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
                  background: paidPct === 100 ? 'var(--success)' : 'var(--accent)',
                  borderRadius: '999px',
                  transition: 'width 0.8s ease',
                }}
              />
            </div>
          </div>
        )}

        {/* Pay button */}
        {(invoice.status === 'issued' || invoice.status === 'partially_paid') && (
          <button
            onClick={handlePay}
            disabled={checkoutDisabled}
            className="btn btn-primary"
            style={{
              width: '100%',
              padding: '16px',
              fontSize: 'var(--fs-md)',
              fontWeight: 700,
              justifyContent: 'center',
              gap: '12px',
              transition: 'opacity 120ms, transform 80ms',
              opacity: checkoutDisabled ? 0.8 : 1,
            }}
            onMouseEnter={(e) => { if (!checkoutDisabled) e.currentTarget.style.transform = 'translateY(-1px)' }}
            onMouseLeave={(e) => { e.currentTarget.style.transform = 'translateY(0)' }}
          >
            {checkingOut ? (
              <>
                <span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-base)', opacity: 0.8 }}>⟳</span>
                Redirecting to Checkout…
              </>
            ) : (
              <>
                <Icon name="credit-card" size={18} />
                Pay ${amountDue.toFixed(2)} now
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
          borderRadius: '12px',
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
              borderRadius: '8px',
              border: 'none',
              background: tab === t ? 'var(--bg-elevated)' : 'transparent',
              color: tab === t ? 'var(--text)' : 'var(--text-subtle)',
              fontSize: 'var(--fs-base)',
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
          className="card" style={{ overflow: 'hidden', animation: 'slideUp 0.3s ease both' }}
        >
          {/* Header row */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr 60px 90px 90px',
              padding: '12px 20px',
              borderBottom: '1px solid rgba(0,0,0,0.07)',
              background: 'rgba(0,0,0,0.02)',
            }}
          >
            {['Description', 'Qty', 'Unit', 'Total'].map((h) => (
              <span
                key={h}
                style={{
                  fontSize: 'var(--fs-xs)',
                  fontFamily: 'var(--font-body)',
                  color: 'var(--text-muted)',
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
          {items.length === 0 ? (
            <div style={{ padding: '32px 20px', textAlign: 'center', fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>
              No line items found.
            </div>
          ) : (
            items.map((item, i) => (
              <div
                key={item.id}
                style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 60px 90px 90px',
                  padding: '16px 20px',
                  borderBottom: i < items.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                  alignItems: 'center',
                }}
              >
                <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text)' }}>{item.description}</span>
                <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', textAlign: 'right', fontFamily: 'var(--font-body)' }}>
                  {item.quantity}
                </span>
                <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', textAlign: 'right', fontFamily: 'var(--font-body)' }}>
                  ${centsToDisplay(item.unit_amount_cents)}
                </span>
                <span style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--text)', textAlign: 'right', fontFamily: 'var(--font-body)' }}>
                  ${centsToDisplay(item.total_cents)}
                </span>
              </div>
            ))
          )}

          {/* Totals */}
          <div
            style={{
              padding: '16px 20px',
              borderTop: '2px solid rgba(0,0,0,0.07)',
              display: 'flex',
              justifyContent: 'flex-end',
              gap: '32px',
            }}
          >
            <div style={{ textAlign: 'right' }}>
              <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginBottom: '2px' }}>Subtotal</div>
              <div style={{ fontSize: 'var(--fs-md)', fontFamily: 'var(--font-body)', color: 'var(--text)' }}>
                ${total.toFixed(2)}
              </div>
            </div>
            <div style={{ textAlign: 'right' }}>
              <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginBottom: '2px' }}>Amount due</div>
              <div style={{ fontSize: 'var(--fs-xl)', fontWeight: 700, fontFamily: 'var(--font-body)', color: 'var(--warning)' }}>
                ${amountDue.toFixed(2)}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ── Payments ────────────────────────────────────────────────────────── */}
      {tab === 'payments' && (
        <div style={{ animation: 'slideUp 0.3s ease both' }}>
          {payments.length === 0 ? (
            <div
              className="card" style={{ padding: '48px', textAlign: 'center' }}
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
                  margin: '0 auto 16px',
                }}
              >
                <Icon name="credit-card" size={22} style={{ color: 'var(--border-strong)' }} />
              </div>
              <div style={{ fontSize: 'var(--fs-md)', color: 'var(--text-muted)', fontWeight: 500 }}>No payments recorded yet</div>
              <div style={{ fontSize: 'var(--fs-base)', color: 'var(--border-strong)', marginTop: '6px' }}>
                Payments will appear here once processed.
              </div>
            </div>
          ) : (
            <div className="card" style={{ overflow: 'hidden' }}>
              {payments.map((p, i) => (
                <div
                  key={p.id}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    padding: '16px 20px',
                    borderBottom: i < payments.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                  }}
                >
                  <div>
                    <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text)', fontWeight: 500 }}>
                      {p.method}
                    </div>
                    <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '2px' }}>
                      {p.paid_at ? formatDate(p.paid_at) : formatDate(p.created_at)}
                    </div>
                  </div>
                  <span style={{ fontSize: 'var(--fs-md)', fontWeight: 700, fontFamily: 'var(--font-body)', color: 'var(--success)' }}>
                    −${centsToDisplay(p.amount_cents)}
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
          className="card" style={{ overflow: 'hidden', animation: 'slideUp 0.3s ease both' }}
        >
          {payments.length === 0 ? (
            <div style={{ padding: '48px', textAlign: 'center' }}>
              <div style={{ fontSize: 'var(--fs-md)', color: 'var(--text-muted)', fontWeight: 500 }}>No receipts available</div>
              <div style={{ fontSize: 'var(--fs-base)', color: 'var(--border-strong)', marginTop: '6px' }}>
                Receipts will appear here after payments are processed.
              </div>
            </div>
          ) : (
            payments.filter((p) => p.status === 'succeeded' || p.status === 'paid').map((rec, i, arr) => (
              <div
                key={rec.id}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '16px',
                  padding: '16px 20px',
                  borderBottom: i < arr.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                }}
              >
                <div
                  style={{
                    width: '34px',
                    height: '34px',
                    borderRadius: '8px',
                    background: 'var(--success-bg)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    flexShrink: 0,
                  }}
                >
                  <Icon name="file-text" size={15} style={{ color: 'var(--success)' }} />
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text)', fontWeight: 500 }}>
                    Payment — {rec.method}
                  </div>
                  <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '2px' }}>
                    {rec.paid_at ? formatDate(rec.paid_at) : formatDate(rec.created_at)} · {rec.method}
                    <span
                      style={{
                        marginLeft: '8px',
                        fontFamily: 'var(--font-body)',
                        fontSize: 'var(--fs-2xs)',
                        color: 'var(--border-strong)',
                      }}
                    >
                      #{rec.id.slice(-6)}
                    </span>
                  </div>
                </div>
                <div style={{ textAlign: 'right', flexShrink: 0 }}>
                  <div
                    style={{
                      fontSize: 'var(--fs-md)',
                      fontWeight: 700,
                      fontFamily: 'var(--font-body)',
                      color: 'var(--success)',
                    }}
                  >
                    ${centsToDisplay(rec.amount_cents)}
                  </div>
                  <button
                    style={{
                      marginTop: '4px',
                      background: 'none',
                      border: 'none',
                      cursor: 'pointer',
                      fontSize: 'var(--fs-xs)',
                      color: 'var(--accent)',
                      fontWeight: 500,
                      padding: '0',
                      fontFamily: 'var(--font-body)',
                    }}
                  >
                    Download PDF
                  </button>
                </div>
              </div>
            ))
          )}
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
