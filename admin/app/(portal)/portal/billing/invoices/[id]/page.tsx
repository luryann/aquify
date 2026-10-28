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
import './page.inline.css'

// ── Helpers ─────────────────────────────────────────────────────────────────
const STATUS_CONFIG: Record<string, { label: string; bg: string; color: string }> = {
  draft:            { label: 'Draft',        bg: 'var(--bg-surface)', color: 'var(--text-muted)' },
  issued:           { label: 'Payment due',  bg: 'var(--warning-bg)', color: 'var(--warning)' },
  partially_paid:   { label: 'Partially paid',bg: 'var(--info-bg)', color: 'var(--info)' },
  paid:             { label: 'Paid',         bg: 'var(--success-bg)', color: 'var(--success)' },
  refunded_partial: { label: 'Refunded',     bg: 'var(--water-subtle)', color: 'var(--water)' },
  refunded_full:    { label: 'Refunded',     bg: 'var(--water-subtle)', color: 'var(--water)' },
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
      <div className="is-3256332" />
      <div
        className="card is-bb717d2" 
      >
        <div className="is-dfe8511">
          <div className="is-930d87f">
            <div style={{ height: '14px', width: '160px', background: 'var(--bg-wash-strong)', borderRadius: '4px' }} />
            <div style={{ height: '24px', width: '300px', background: 'var(--bg-wash-strong)', borderRadius: '6px' }} />
            <div style={{ height: '14px', width: '240px', background: 'var(--bg-wash)', borderRadius: '4px' }} />
          </div>
          <div className="is-8d95465">
            <div style={{ height: '12px', width: '80px', background: 'var(--bg-wash)', borderRadius: '4px' }} />
            <div style={{ height: '36px', width: '100px', background: 'var(--bg-wash-strong)', borderRadius: '6px' }} />
          </div>
        </div>
        <div className="is-592a9d2" />
        <div style={{ height: '52px', background: 'var(--bg-wash)', borderRadius: '12px' }} />
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
      setLoadError('We could not find your household details. Sign out and back in, or contact your club if this keeps happening.')
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
          'We could not load this page. Check your connection and try again.',
          { orgId, invoiceId },
        ))
      })
      .finally(() => setLoading(false))
  }, [api, orgId, invoiceId, reloadKey])

  if (loading) return <LoadingSkeleton />

  if (!invoice) {
    return (
      <div className="page">
        <nav className="is-781ef11">
          <Link href="/portal/billing" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>
            Billing
          </Link>
        </nav>
        {loadError && (
          <div
            className="is-469b891"
          >
            <div className="is-730b6a7">
              <Icon name="alert-circle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
              <p className="is-51a00ce">{loadError}</p>
            </div>
            <button
              type="button"
              onClick={() => setReloadKey((key) => key + 1)}
              className="is-889d44c"
            >
              Retry
            </button>
          </div>
        )}
        <div
          className="card is-52b05e3" 
        >
          <div className="is-5e8f68c">
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
        className="is-90f3963"
      >
        <Link href="/portal/billing" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>
          Billing
        </Link>
        <span>/</span>
        <span className="is-7604dc9">
          {invoice.invoice_number}
        </span>
      </nav>

      {/* Invoice header */}
      <div
        className="card is-e878bde" 
      >
        {checkoutState === 'success' && (
          <div
            className="is-72a24ef"
          >
            Stripe returned successfully. If payment confirmation is still in flight, refresh this invoice in a moment to see the updated status.
          </div>
        )}
        {checkoutState === 'cancelled' && (
          <div
            className="is-bfa258f"
          >
            Checkout was cancelled before payment completed. Your invoice is still open.
          </div>
        )}
        {checkoutError && (
          <div
            className="is-a85471b"
          >
            {checkoutError}
          </div>
        )}
        <div className="is-5324aad">
          <div>
            <div className="is-c960f60">
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
                }}
              >
                {statusCfg.label}
              </span>
            </div>
            <div className="is-b2d8026">
              <span className="is-d5bdeb0">{invoice.invoice_number}</span>
              {invoice.issued_at ? ` · Issued ${formatDate(invoice.issued_at)}` : ''}
              {invoice.due_date ? ` · Due ${formatDate(invoice.due_date)}` : ''}
            </div>
          </div>

          {/* Amount due */}
          <div style={{ textAlign: 'right' }}>
            <div className="is-8ab28f2">Amount due</div>
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
          <div className="is-b75fad0">
            <div className="is-0c50af4">
              <span className="is-4f39ab0">
                ${amountPaid.toFixed(2)} paid of ${total.toFixed(2)}
              </span>
              <span className="is-4f39ab0">{paidPct}%</span>
            </div>
            <div
              style={{
                height: '6px',
                background: 'var(--bg-wash-strong)',
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
          <button type="button"
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
                <span className="is-7604705">⟳</span>
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
        className="is-1a3aa72"
      >
        {(['summary', 'payments', 'receipts'] as Tab[]).map((t) => (
          <button type="button"
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
            className="is-e9d91ee"
          >
            {['Description', 'Qty', 'Unit', 'Total'].map((h) => (
              <span
                key={h}
                style={{
                  fontSize: 'var(--fs-xs)',
                  fontFamily: 'var(--font-body)',
                  color: 'var(--text-muted)',
                  textAlign: h === 'Description' ? 'left' : 'right',
                }}
              >
                {h}
              </span>
            ))}
          </div>

          {/* Items */}
          {items.length === 0 ? (
            <div className="is-e20bace">
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
                  borderBottom: i < items.length - 1 ? '1px solid var(--border)' : 'none',
                  alignItems: 'center',
                }}
              >
                <span className="is-302d13d">{item.description}</span>
                <span className="is-8c28f85">
                  {item.quantity}
                </span>
                <span className="is-8c28f85">
                  ${centsToDisplay(item.unit_amount_cents)}
                </span>
                <span className="is-b9b90e9">
                  ${centsToDisplay(item.total_cents)}
                </span>
              </div>
            ))
          )}

          {/* Totals */}
          <div
            className="is-2c597dc"
          >
            <div style={{ textAlign: 'right' }}>
              <div className="is-c8f1267">Subtotal</div>
              <div className="is-3b18e73">
                ${total.toFixed(2)}
              </div>
            </div>
            <div style={{ textAlign: 'right' }}>
              <div className="is-c8f1267">Amount due</div>
              <div className="is-5348860">
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
              className="card is-52b05e3" 
            >
              <div
                className="is-af12721"
              >
                <Icon name="credit-card" size={22} style={{ color: 'var(--border-strong)' }} />
              </div>
              <div className="is-5e8f68c">No payments recorded yet</div>
              <div className="is-87e0fe4">
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
                    borderBottom: i < payments.length - 1 ? '1px solid var(--border)' : 'none',
                  }}
                >
                  <div>
                    <div className="is-df04e21">
                      {p.method}
                    </div>
                    <div className="is-7536cb1">
                      {p.paid_at ? formatDate(p.paid_at) : formatDate(p.created_at)}
                    </div>
                  </div>
                  <span className="is-c3600c0">
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
            <div className="is-52b05e3">
              <div className="is-5e8f68c">No receipts available</div>
              <div className="is-87e0fe4">
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
                  borderBottom: i < arr.length - 1 ? '1px solid var(--border)' : 'none',
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
                  <div className="is-df04e21">
                    Payment — {rec.method}
                  </div>
                  <div className="is-7536cb1">
                    {rec.paid_at ? formatDate(rec.paid_at) : formatDate(rec.created_at)} · {rec.method}
                    <span
                      className="is-abbb76c"
                    >
                      #{rec.id.slice(-6)}
                    </span>
                  </div>
                </div>
                <div style={{ textAlign: 'right', flexShrink: 0 }}>
                  <div
                    className="is-c3600c0"
                  >
                    ${centsToDisplay(rec.amount_cents)}
                  </div>
                  <button type="button"
                    className="is-0eb0eac"
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
