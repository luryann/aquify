'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import {
  fetchHousehold,
  fetchInvoice,
  fetchInvoiceItems,
  fetchInvoicePayments,
  fetchInvoiceRefunds,
  fetchLedgerEventsForInvoice,
  type InvoiceItemRecord,
  type InvoiceRecord,
  type LedgerEventRecord,
  type PaymentRecord,
  type RefundRecord,
  type HouseholdRecord,
} from '@/lib/adminData'

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100)
}

function formatDate(value?: string | null) {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function formatDateTime(value?: string | null) {
  if (!value) return '—'
  const parsed = new Date(value)
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
  return <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', fontWeight: 700, padding: '2px 8px', borderRadius: '10px', background: colors.bg, color: colors.color }}>{status.replaceAll('_', ' ')}</span>
}

const ITEM_COLS: Column<InvoiceItemRecord>[] = [
  {
    key: 'description',
    label: 'Description',
    render: (row) => (
      <div>
        <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{row.description}</div>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px', fontFamily: 'var(--font-mono)' }}>{row.category}</div>
      </div>
    ),
  },
  { key: 'quantity', label: 'Qty', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{row.quantity}</span> },
  { key: 'unit_amount_cents', label: 'Unit Price', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatCents(row.unit_amount_cents)}</span> },
  { key: 'total_cents', label: 'Total', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--text)' }}>{formatCents(row.total_cents)}</span> },
]

const PAYMENT_COLS: Column<PaymentRecord>[] = [
  { key: 'method', label: 'Method', render: (row) => <span style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-subtle)' }}>{row.method}</span> },
  { key: 'amount_cents', label: 'Amount', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--success)' }}>{formatCents(row.amount_cents)}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
  { key: 'stripe_payment_intent_id', label: 'Reference', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)' }}>{row.stripe_payment_intent_id || row.external_ref || '—'}</span> },
  { key: 'created_by', label: 'Recorded By', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.created_by}</span> },
  { key: 'created_at', label: 'Date', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(row.paid_at || row.created_at)}</span> },
]

const REFUND_COLS: Column<RefundRecord>[] = [
  { key: 'amount_cents', label: 'Amount', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--accent)' }}>{formatCents(row.amount_cents)}</span> },
  { key: 'reason', label: 'Reason', render: (row) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{row.reason}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
  { key: 'created_by', label: 'By', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.created_by}</span> },
  { key: 'created_at', label: 'Date', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(row.created_at)}</span> },
]

const LEDGER_COLS: Column<LedgerEventRecord>[] = [
  { key: 'event_type', label: 'Action', render: (row) => <span style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text)' }}>{row.event_type.replaceAll('_', ' ')}</span> },
  { key: 'amount_cents', label: 'Amount', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: row.amount_cents < 0 ? 'var(--success)' : 'var(--text)' }}>{formatCents(row.amount_cents)}</span> },
  { key: 'description', label: 'Memo', render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{row.description || '—'}</span> },
  { key: 'actor_id', label: 'Actor', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)' }}>{row.actor_id || 'system'}</span> },
  { key: 'created_at', label: 'Occurred', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDateTime(row.created_at)}</span> },
]

export default function InvoiceDetailPage() {
  const params = useParams<{ id: string }>()
  const invoiceId = typeof params?.id === 'string' ? params.id : ''
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [activeTab, setActiveTab] = useState('items')
  const [invoice, setInvoice] = useState<InvoiceRecord | null>(null)
  const [household, setHousehold] = useState<HouseholdRecord | null>(null)
  const [items, setItems] = useState<InvoiceItemRecord[]>([])
  const [payments, setPayments] = useState<PaymentRecord[]>([])
  const [refunds, setRefunds] = useState<RefundRecord[]>([])
  const [ledger, setLedger] = useState<LedgerEventRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

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
        const [nextHousehold, nextItems, nextPayments, nextRefunds, nextLedger] = await Promise.all([
          nextInvoice.household_id ? fetchHousehold(api, session.user.orgId, nextInvoice.household_id) : Promise.resolve(null),
          fetchInvoiceItems(api, session.user.orgId, invoiceId),
          fetchInvoicePayments(api, session.user.orgId, invoiceId),
          fetchInvoiceRefunds(api, session.user.orgId, invoiceId),
          fetchLedgerEventsForInvoice(api, session.user.orgId, invoiceId),
        ])

        if (cancelled) return

        setInvoice(nextInvoice)
        setHousehold(nextHousehold)
        setItems(nextItems)
        setPayments(nextPayments)
        setRefunds(nextRefunds)
        setLedger(nextLedger)
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
  }, [api, invoiceId, session?.user.orgId])

  const tabs = useMemo(() => [
    { id: 'items', label: 'Items', count: items.length },
    { id: 'payments', label: 'Payments', count: payments.length },
    { id: 'refunds', label: 'Refunds', count: refunds.length },
    { id: 'ledger', label: 'Ledger', count: ledger.length },
  ], [items.length, ledger.length, payments.length, refunds.length])

  if (loading) {
    return <div style={{ padding: '24px', maxWidth: '1200px' }}><EmptyState icon="credit-card" title="Loading invoice" description="Fetching live invoice detail data from the API." /></div>
  }

  if (error || !invoice) {
    return <div style={{ padding: '24px', maxWidth: '1200px' }}><EmptyState icon="alert-circle" title="Unable to load invoice" description={error ?? 'Invoice not found.'} /></div>
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <Link href="/invoices" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}>
        <Icon name="chevron-left" size={14} /> Invoices
      </Link>

      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
            <h1 style={{ fontSize: '24px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2 }}>{invoice.invoice_number}</h1>
            <StatusBadge status={invoice.status} />
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
            {invoice.id} · {household ? <Link href={`/members/households/${household.id}`} style={{ color: 'var(--accent)', textDecoration: 'none' }}>{household.household_name}</Link> : 'Unassigned household'}
          </div>
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
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '18px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'items' ? <Table columns={ITEM_COLS} data={items} emptyTitle="No invoice items" emptyDescription="No line items are attached to this invoice." /> : null}
      {activeTab === 'payments' ? <Table columns={PAYMENT_COLS} data={payments} emptyTitle="No payments" emptyDescription="No payments have been recorded for this invoice yet." /> : null}
      {activeTab === 'refunds' ? <Table columns={REFUND_COLS} data={refunds} emptyTitle="No refunds" emptyDescription="No refunds have been recorded for this invoice." /> : null}
      {activeTab === 'ledger' ? <Table columns={LEDGER_COLS} data={ledger} emptyTitle="No ledger activity" emptyDescription="No ledger events were returned for this invoice." /> : null}
    </div>
  )
}
