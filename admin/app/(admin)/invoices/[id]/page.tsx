'use client'

import { useEffect, useMemo, useState, type FormEvent } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { canAccess } from '@/lib/roles'
import { formatDateOnly, parseBackendDate } from '@/lib/dates'
import {
  fetchHousehold,
  fetchInvoice,
  fetchInvoiceItems,
  fetchInvoicePayments,
  fetchInvoiceRefunds,
  fetchLedgerEventsForInvoice,
  fetchSwimmers,
  createRefund,
  downloadReportArtifact,
  addInvoiceItem,
  issueInvoice,
  voidInvoice,
  writeOffInvoice,
  recordInvoicePayment,
  type InvoiceItemRecord,
  type InvoiceRecord,
  type LedgerEventRecord,
  type PaymentRecord,
  type RefundRecord,
  type HouseholdRecord,
  type SwimmerRecord,
} from '@/lib/adminData'

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100)
}

const formatDate = formatDateOnly

function formatDateTime(value?: string | null) {
  if (!value) return '—'
  const parsed = parseBackendDate(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function StatusBadge({ status }: { status: string }) {
  const cfg: Record<string, { color: string; bg: string }> = {
    draft: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    issued: { color: 'var(--info)', bg: 'var(--info-bg)' },
    partially_paid: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
    paid: { color: 'var(--success)', bg: 'var(--success-bg)' },
    refunded_partial: { color: 'var(--accent)', bg: 'var(--accent-subtle)' },
    refunded_full: { color: 'var(--accent)', bg: 'var(--accent-subtle)' },
    voided: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    written_off: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    pending: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
    succeeded: { color: 'var(--success)', bg: 'var(--success-bg)' },
    failed: { color: 'var(--error)', bg: 'var(--error-bg)' },
    processed: { color: 'var(--success)', bg: 'var(--success-bg)' },
  }
  const colors = cfg[status] ?? { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' }
  return <span className="badge" style={{ background: colors.bg, color: colors.color }}>{status.replaceAll('_', ' ')}</span>
}

function buildItemCols(swimmerNameById: Map<string, string>): Column<InvoiceItemRecord>[] {
  return [
    {
      key: 'description',
      label: 'Description',
      render: (row) => (
        <div>
          <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{row.description}</div>
          <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px', fontFamily: 'var(--font-body)' }}>{row.category}</div>
        </div>
      ),
    },
    {
      key: 'swimmer_id',
      label: 'Swimmer',
      render: (row) => (
        <span style={{ fontSize: '12px', color: row.swimmer_id ? 'var(--text-subtle)' : 'var(--text-muted)' }}>
          {row.swimmer_id ? (swimmerNameById.get(row.swimmer_id) ?? 'Unknown swimmer') : 'Household-wide'}
        </span>
      ),
    },
    { key: 'quantity', label: 'Qty', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--text-subtle)' }}>{row.quantity}</span> },
    { key: 'unit_amount_cents', label: 'Unit Price', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatCents(row.unit_amount_cents)}</span> },
    { key: 'total_cents', label: 'Total', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '13px', fontWeight: 700, color: 'var(--text)' }}>{formatCents(row.total_cents)}</span> },
  ]
}

const PAYMENT_COLS: Column<PaymentRecord>[] = [
  { key: 'method', label: 'Method', render: (row) => <span style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-subtle)' }}>{row.method}</span> },
  { key: 'amount_cents', label: 'Amount', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '13px', fontWeight: 700, color: 'var(--success)' }}>{formatCents(row.amount_cents)}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
  { key: 'stripe_payment_intent_id', label: 'Reference', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '11px', color: 'var(--text-muted)' }}>{row.stripe_payment_intent_id || row.external_ref || '—'}</span> },
  { key: 'created_by', label: 'Recorded By', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.created_by}</span> },
  { key: 'created_at', label: 'Date', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(row.paid_at || row.created_at)}</span> },
]

const REFUND_COLS: Column<RefundRecord>[] = [
  { key: 'amount_cents', label: 'Amount', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '13px', fontWeight: 700, color: 'var(--accent)' }}>{formatCents(row.amount_cents)}</span> },
  { key: 'reason', label: 'Reason', render: (row) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{row.reason}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
  { key: 'created_by', label: 'By', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.created_by}</span> },
  { key: 'created_at', label: 'Date', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(row.created_at)}</span> },
]

const LEDGER_COLS: Column<LedgerEventRecord>[] = [
  { key: 'event_type', label: 'Action', render: (row) => <span style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text)' }}>{row.event_type.replaceAll('_', ' ')}</span> },
  { key: 'amount_cents', label: 'Amount', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: row.amount_cents < 0 ? 'var(--success)' : 'var(--text)' }}>{formatCents(row.amount_cents)}</span> },
  { key: 'description', label: 'Memo', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.description || '—'}</span> },
  { key: 'actor_id', label: 'Actor', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '11px', color: 'var(--text-muted)' }}>{row.actor_id || 'system'}</span> },
  { key: 'created_at', label: 'Occurred', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDateTime(row.created_at)}</span> },
]

const INPUT_STYLE = { width: '100%', padding: '9px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' as const }
const LABEL_STYLE = { fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase' as const, letterSpacing: '0.05em', display: 'block', marginBottom: '5px' }

function IssueRefundPanel({
  api, orgId, invoiceId, payments, onIssued,
}: {
  api: ReturnType<typeof useApiClient>
  orgId: string
  invoiceId: string
  payments: PaymentRecord[]
  onIssued: () => void
}) {
  const eligiblePayments = useMemo(
    () => payments.filter((p) => p.status === 'succeeded' || p.status === 'processed'),
    [payments],
  )
  const [open, setOpen] = useState(false)
  const [paymentId, setPaymentId] = useState('')
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const selectedPayment = eligiblePayments.find((p) => p.id === paymentId)

  function openForm() {
    setOpen(true)
    setError(null)
    const first = eligiblePayments[0]
    if (first) {
      setPaymentId(first.id)
      setAmount((first.amount_cents / 100).toFixed(2))
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!selectedPayment) {
      setError('Select a payment to refund.')
      return
    }
    const amountCents = Math.round(parseFloat(amount) * 100)
    if (!Number.isFinite(amountCents) || amountCents <= 0) {
      setError('Enter a valid refund amount.')
      return
    }
    if (amountCents > selectedPayment.amount_cents) {
      setError('Refund amount cannot exceed the payment amount.')
      return
    }
    if (!reason.trim()) {
      setError('A reason is required.')
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      await createRefund(api, orgId, invoiceId, {
        payment_id: selectedPayment.id,
        amount_cents: amountCents,
        reason: reason.trim(),
      })
      setOpen(false)
      setPaymentId('')
      setAmount('')
      setReason('')
      onIssued()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to issue refund.')
    } finally {
      setSubmitting(false)
    }
  }

  if (eligiblePayments.length === 0) {
    return null
  }

  return (
    <div className="card" style={{ padding: '16px', marginBottom: '16px' }}>
      {!open ? (
        <button type="button" className="btn btn-primary" onClick={openForm}>
          Issue refund
        </button>
      ) : (
        <form onSubmit={(e) => void handleSubmit(e)} style={{ display: 'grid', gap: '14px' }}>
          {error && (
            <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '10px 14px', fontSize: '13px' }}>
              {error}
            </div>
          )}
          <div>
            <label style={LABEL_STYLE}>Payment to refund *</label>
            <select
              style={INPUT_STYLE}
              value={paymentId}
              onChange={(e) => {
                setPaymentId(e.target.value)
                const p = eligiblePayments.find((row) => row.id === e.target.value)
                if (p) setAmount((p.amount_cents / 100).toFixed(2))
              }}
            >
              {eligiblePayments.map((p) => (
                <option key={p.id} value={p.id}>
                  {formatCents(p.amount_cents)} · {p.method} · {formatDate(p.paid_at || p.created_at)}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label style={LABEL_STYLE}>Refund amount *</label>
            <input
              type="number"
              step="0.01"
              min="0.01"
              max={selectedPayment ? selectedPayment.amount_cents / 100 : undefined}
              style={INPUT_STYLE}
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
            />
          </div>
          <div>
            <label style={LABEL_STYLE}>Reason *</label>
            <textarea
              style={{ ...INPUT_STYLE, resize: 'vertical' }}
              rows={2}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="E.g. overpayment, duplicate charge, season cancellation…"
            />
          </div>
          <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
            <button type="button" className="btn" onClick={() => setOpen(false)} disabled={submitting}>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary" disabled={submitting}>
              {submitting ? 'Issuing…' : 'Issue refund'}
            </button>
          </div>
        </form>
      )}
    </div>
  )
}

const ITEM_CATEGORIES = ['program_fee', 'meet_fee', 'equipment', 'manual', 'late_fee', 'credit'] as const

function AddInvoiceItemPanel({
  api, orgId, invoiceId, onAdded,
}: {
  api: ReturnType<typeof useApiClient>
  orgId: string
  invoiceId: string
  onAdded: () => void
}) {
  const [open, setOpen] = useState(false)
  const [description, setDescription] = useState('')
  const [quantity, setQuantity] = useState('1')
  const [unitAmount, setUnitAmount] = useState('')
  const [category, setCategory] = useState<(typeof ITEM_CATEGORIES)[number]>('manual')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const qty = Number.parseInt(quantity, 10)
    const unitCents = Math.round(parseFloat(unitAmount) * 100)
    if (!description.trim()) {
      setError('A description is required.')
      return
    }
    if (!Number.isFinite(qty) || qty <= 0) {
      setError('Enter a valid quantity.')
      return
    }
    if (!Number.isFinite(unitCents) || unitCents <= 0) {
      setError('Enter a valid unit price.')
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      await addInvoiceItem(api, orgId, invoiceId, {
        description: description.trim(),
        quantity: qty,
        unit_amount_cents: unitCents,
        category,
      })
      setOpen(false)
      setDescription('')
      setQuantity('1')
      setUnitAmount('')
      setCategory('manual')
      onAdded()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to add invoice item.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="card" style={{ padding: '16px', marginBottom: '16px' }}>
      {!open ? (
        <button type="button" className="btn btn-primary" onClick={() => { setOpen(true); setError(null) }}>
          Add item
        </button>
      ) : (
        <form onSubmit={(e) => void handleSubmit(e)} style={{ display: 'grid', gap: '14px' }}>
          {error && (
            <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '10px 14px', fontSize: '13px' }}>
              {error}
            </div>
          )}
          <div>
            <label style={LABEL_STYLE}>Description *</label>
            <input style={INPUT_STYLE} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="E.g. Fall program fee" />
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '14px' }}>
            <div>
              <label style={LABEL_STYLE}>Quantity *</label>
              <input type="number" min="1" step="1" style={INPUT_STYLE} value={quantity} onChange={(e) => setQuantity(e.target.value)} />
            </div>
            <div>
              <label style={LABEL_STYLE}>Unit price *</label>
              <input type="number" min="0.01" step="0.01" style={INPUT_STYLE} value={unitAmount} onChange={(e) => setUnitAmount(e.target.value)} placeholder="0.00" />
            </div>
          </div>
          <div>
            <label style={LABEL_STYLE}>Category *</label>
            <select style={INPUT_STYLE} value={category} onChange={(e) => setCategory(e.target.value as (typeof ITEM_CATEGORIES)[number])}>
              {ITEM_CATEGORIES.map((c) => <option key={c} value={c}>{c.replaceAll('_', ' ')}</option>)}
            </select>
          </div>
          <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
            <button type="button" className="btn" onClick={() => setOpen(false)} disabled={submitting}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Adding…' : 'Add item'}</button>
          </div>
        </form>
      )}
    </div>
  )
}

const PAYMENT_METHODS = ['check', 'cash', 'ach', 'stripe', 'other'] as const

function RecordPaymentPanel({
  api, orgId, invoiceId, onRecorded,
}: {
  api: ReturnType<typeof useApiClient>
  orgId: string
  invoiceId: string
  onRecorded: () => void
}) {
  const [open, setOpen] = useState(false)
  const [amount, setAmount] = useState('')
  const [method, setMethod] = useState<(typeof PAYMENT_METHODS)[number]>('check')
  const [externalRef, setExternalRef] = useState('')
  const [notes, setNotes] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const amountCents = Math.round(parseFloat(amount) * 100)
    if (!Number.isFinite(amountCents) || amountCents <= 0) {
      setError('Enter a valid payment amount.')
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      await recordInvoicePayment(api, orgId, invoiceId, {
        amount_cents: amountCents,
        method,
        external_ref: externalRef.trim() || undefined,
        notes: notes.trim() || undefined,
      })
      setOpen(false)
      setAmount('')
      setExternalRef('')
      setNotes('')
      onRecorded()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to record payment.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="card" style={{ padding: '16px', marginBottom: '16px' }}>
      {!open ? (
        <button type="button" className="btn btn-primary" onClick={() => { setOpen(true); setError(null) }}>
          Record offline payment
        </button>
      ) : (
        <form onSubmit={(e) => void handleSubmit(e)} style={{ display: 'grid', gap: '14px' }}>
          {error && (
            <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '10px 14px', fontSize: '13px' }}>
              {error}
            </div>
          )}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '14px' }}>
            <div>
              <label style={LABEL_STYLE}>Amount *</label>
              <input type="number" min="0.01" step="0.01" style={INPUT_STYLE} value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="0.00" />
            </div>
            <div>
              <label style={LABEL_STYLE}>Method *</label>
              <select style={INPUT_STYLE} value={method} onChange={(e) => setMethod(e.target.value as (typeof PAYMENT_METHODS)[number])}>
                {PAYMENT_METHODS.map((m) => <option key={m} value={m}>{m}</option>)}
              </select>
            </div>
          </div>
          <div>
            <label style={LABEL_STYLE}>Reference</label>
            <input style={INPUT_STYLE} value={externalRef} onChange={(e) => setExternalRef(e.target.value)} placeholder="Check #, transaction ID, etc." />
          </div>
          <div>
            <label style={LABEL_STYLE}>Notes</label>
            <textarea style={{ ...INPUT_STYLE, resize: 'vertical' }} rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
          </div>
          <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
            <button type="button" className="btn" onClick={() => setOpen(false)} disabled={submitting}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Recording…' : 'Record payment'}</button>
          </div>
        </form>
      )}
    </div>
  )
}

function truncateId(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}…` : id
}

function CopyableId({ id }: { id: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      type="button"
      title={id}
      onClick={() => {
        void navigator.clipboard.writeText(id).then(() => {
          setCopied(true)
          setTimeout(() => setCopied(false), 1500)
        })
      }}
      style={{
        display: 'inline-flex', alignItems: 'center', gap: '4px',
        background: 'none', border: 'none', cursor: 'pointer', padding: 0,
        fontFamily: 'var(--font-body)', fontSize: '12px', color: 'inherit',
      }}
    >
      {copied ? 'Copied!' : truncateId(id)}
      <Icon name="copy" size={11} />
    </button>
  )
}

export default function InvoiceDetailPage() {
  const params = useParams<{ id: string }>()
  const invoiceId = typeof params?.id === 'string' ? params.id : ''
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [activeTab, setActiveTab] = useState('items')
  const [invoice, setInvoice] = useState<InvoiceRecord | null>(null)
  const [household, setHousehold] = useState<HouseholdRecord | null>(null)
  const [items, setItems] = useState<InvoiceItemRecord[]>([])
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [payments, setPayments] = useState<PaymentRecord[]>([])
  const [refunds, setRefunds] = useState<RefundRecord[]>([])
  const [ledger, setLedger] = useState<LedgerEventRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [downloading, setDownloading] = useState(false)
  const [downloadError, setDownloadError] = useState<string | null>(null)
  const [lifecycleBusy, setLifecycleBusy] = useState(false)
  const [lifecycleError, setLifecycleError] = useState<string | null>(null)

  async function handleLifecycleAction(action: 'issue' | 'void' | 'write-off', confirmMessage: string) {
    if (!session?.user.orgId || !invoiceId) return
    if (!window.confirm(confirmMessage)) return
    setLifecycleBusy(true)
    setLifecycleError(null)
    try {
      if (action === 'issue') await issueInvoice(api, session.user.orgId, invoiceId)
      else if (action === 'void') await voidInvoice(api, session.user.orgId, invoiceId)
      else await writeOffInvoice(api, session.user.orgId, invoiceId)
      setReloadKey((k) => k + 1)
    } catch (err) {
      setLifecycleError(err instanceof ApiError ? err.message : `Failed to ${action} invoice.`)
    } finally {
      setLifecycleBusy(false)
    }
  }

  async function handleDownload() {
    if (!session?.user.orgId || !invoiceId) return
    setDownloading(true)
    setDownloadError(null)
    try {
      const artifact = await downloadReportArtifact(
        api,
        session.user.orgId,
        `/api/v1/reports/invoice-detail?invoice_id=${invoiceId}&format=csv`,
      )
      if (artifact?.download_url) {
        window.open(artifact.download_url, '_blank', 'noopener,noreferrer')
      }
    } catch (err) {
      setDownloadError(err instanceof ApiError ? err.message : 'Unable to generate invoice export.')
    } finally {
      setDownloading(false)
    }
  }

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!session?.user.orgId || !invoiceId) {
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        setError(null)
        const nextInvoice = await fetchInvoice(api, session.user.orgId, invoiceId)
        const [nextHousehold, nextItems, nextPayments, nextRefunds, nextLedger, nextSwimmers] = await Promise.all([
          nextInvoice.household_id ? fetchHousehold(api, session.user.orgId, nextInvoice.household_id) : Promise.resolve(null),
          fetchInvoiceItems(api, session.user.orgId, invoiceId),
          fetchInvoicePayments(api, session.user.orgId, invoiceId),
          fetchInvoiceRefunds(api, session.user.orgId, invoiceId),
          fetchLedgerEventsForInvoice(api, session.user.orgId, invoiceId),
          fetchSwimmers(api, session.user.orgId),
        ])

        if (cancelled) return

        setInvoice(nextInvoice)
        setHousehold(nextHousehold)
        setItems(nextItems)
        setPayments(nextPayments)
        setRefunds(nextRefunds)
        setLedger(nextLedger)
        setSwimmers(nextSwimmers)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load invoice details.')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, invoiceId, session?.user.orgId, reloadKey])

  const tabs = useMemo(() => [
    { id: 'items', label: 'Items', count: items.length },
    { id: 'payments', label: 'Payments', count: payments.length },
    { id: 'refunds', label: 'Refunds', count: refunds.length },
    { id: 'ledger', label: 'History', count: ledger.length },
  ], [items.length, ledger.length, payments.length, refunds.length])

  const swimmerNameById = useMemo(
    () => new Map(swimmers.map((s) => [s.id, `${s.first_name} ${s.last_name}`])),
    [swimmers],
  )
  const itemCols = useMemo(() => buildItemCols(swimmerNameById), [swimmerNameById])

  if (loading) {
    return <div style={{ padding: '24px', maxWidth: '1200px' }}><EmptyState icon="credit-card" title="Loading invoice" description="Fetching live invoice detail data from the API." /></div>
  }

  if (error || !invoice) {
    return <div style={{ padding: '24px', maxWidth: '1200px' }}><EmptyState icon="alert-circle" title="Unable to load invoice" description={error ?? 'Invoice not found.'} /></div>
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <Link href="/billing?tab=invoices" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}>
        <Icon name="chevron-left" size={14} /> Invoices
      </Link>

      <div className="page-header">
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
            <h1 className="page-title">{invoice.invoice_number}</h1>
            <StatusBadge status={invoice.status} />
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', display: 'flex', alignItems: 'center', gap: '6px' }}>
            <CopyableId id={invoice.id} /> · {household ? <Link href={`/members/households/${household.id}`} style={{ color: 'var(--accent)', textDecoration: 'none', fontFamily: 'var(--font-body)' }}>{household.household_name}</Link> : 'Unassigned household'}
          </div>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '6px' }}>
          <div style={{ display: 'flex', gap: '8px' }}>
            {invoice.status === 'draft' && (
              <button type="button" className="btn btn-primary" disabled={lifecycleBusy} onClick={() => void handleLifecycleAction('issue', 'Issue this invoice? The balance will become due and it can no longer have items added.')}>
                {lifecycleBusy ? '…' : 'Issue'}
              </button>
            )}
            {invoice.status === 'issued' && invoice.amount_paid_cents === 0 && (
              <button type="button" className="btn btn-secondary" disabled={lifecycleBusy} onClick={() => void handleLifecycleAction('void', 'Void this invoice? This cannot be undone.')}>
                {lifecycleBusy ? '…' : 'Void'}
              </button>
            )}
            {(invoice.status === 'issued' || invoice.status === 'partially_paid') && canAccess(session?.user.roles ?? [], ['admin']) && (
              <button type="button" className="btn btn-secondary" style={{ color: 'var(--error)' }} disabled={lifecycleBusy} onClick={() => void handleLifecycleAction('write-off', 'Write off the remaining balance on this invoice? This cannot be undone.')}>
                {lifecycleBusy ? '…' : 'Write Off'}
              </button>
            )}
            <button type="button" className="btn" onClick={() => void handleDownload()} disabled={downloading} style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Icon name="download-cloud" size={14} />
              {downloading ? 'Preparing…' : 'Download CSV'}
            </button>
          </div>
          {lifecycleError && <span style={{ fontSize: '12px', color: 'var(--error)' }}>{lifecycleError}</span>}
          {downloadError && <span style={{ fontSize: '12px', color: 'var(--error)' }}>{downloadError}</span>}
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'Total', value: formatCents(invoice.total_cents), color: 'var(--text)' },
          { label: 'Paid', value: formatCents(invoice.amount_paid_cents), color: 'var(--success)' },
          { label: 'Balance', value: formatCents(invoice.amount_due_cents), color: invoice.amount_due_cents > 0 ? 'var(--warning)' : 'var(--success)' },
          { label: 'Due Date', value: formatDate(invoice.due_date), color: 'var(--text-subtle)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-body)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '18px', fontFamily: 'var(--font-body)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'items' ? (
        <>
          {invoice.status === 'draft' && (
            <AddInvoiceItemPanel
              api={api}
              orgId={session?.user.orgId ?? ''}
              invoiceId={invoiceId}
              onAdded={() => setReloadKey((k) => k + 1)}
            />
          )}
          <Table columns={itemCols} data={items} emptyTitle="No invoice items" emptyDescription="No line items are attached to this invoice." />
        </>
      ) : null}
      {activeTab === 'payments' ? (
        <>
          {(invoice.status === 'issued' || invoice.status === 'partially_paid') && (
            <RecordPaymentPanel
              api={api}
              orgId={session?.user.orgId ?? ''}
              invoiceId={invoiceId}
              onRecorded={() => setReloadKey((k) => k + 1)}
            />
          )}
          <Table columns={PAYMENT_COLS} data={payments} emptyTitle="No payments" emptyDescription="No payments have been recorded for this invoice yet." />
        </>
      ) : null}
      {activeTab === 'refunds' ? (
        <>
          {canAccess(session?.user.roles ?? [], ['admin']) && (
            <IssueRefundPanel
              api={api}
              orgId={session?.user.orgId ?? ''}
              invoiceId={invoiceId}
              payments={payments}
              onIssued={() => setReloadKey((k) => k + 1)}
            />
          )}
          <Table columns={REFUND_COLS} data={refunds} emptyTitle="No refunds" emptyDescription="No refunds have been recorded for this invoice." />
        </>
      ) : null}
      {activeTab === 'ledger' ? <Table columns={LEDGER_COLS} data={ledger} emptyTitle="No ledger activity" emptyDescription="No ledger events were returned for this invoice." /> : null}
    </div>
  )
}
