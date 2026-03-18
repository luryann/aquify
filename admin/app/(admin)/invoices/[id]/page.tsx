'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type InvoiceStatus = 'draft' | 'issued' | 'partially_paid' | 'paid' | 'refunded_partial' | 'refunded_full' | 'voided' | 'written_off'
type PaymentMethod = 'stripe' | 'check' | 'cash' | 'bank_transfer'
type PaymentStatus = 'pending' | 'succeeded' | 'failed'
type LedgerAction = 'invoice_issued' | 'payment_succeeded' | 'payment_failed' | 'refund_processed' | 'invoice_voided' | 'invoice_written_off'

type LineItem = {
  id: string
  description: string
  quantity: number
  unitCents: number
  totalCents: number
  category: string
}

type Payment = {
  id: string
  method: PaymentMethod
  amountCents: number
  status: PaymentStatus
  stripeIntentId: string | null
  note: string | null
  recordedBy: string
  recordedAt: string
}

type Refund = {
  id: string
  amountCents: number
  reason: string
  note: string | null
  createdBy: string
  createdAt: string
  stripeRefundId: string | null
}

type LedgerEntry = {
  id: string
  action: LedgerAction
  amountCents: number
  balanceCents: number
  memo: string
  actorId: string
  occurredAt: string
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_INVOICE = {
  id: 'inv-002',
  invoiceNumber: 'INV-2026-0043',
  household: 'Park Family',
  householdId: 'hh-002',
  primaryContact: 'David Park',
  primaryEmail: 'david.park@example.com',
  totalCents: 72000,
  paidCents: 36000,
  balanceCents: 36000,
  status: 'partially_paid' as InvoiceStatus,
  description: 'Spring season registration',
  createdAt: '2026-03-01T10:00:00Z',
  issuedAt: '2026-03-02T09:00:00Z',
  dueDate: '2026-03-31',
  createdBy: 'Admin Rivera',
  notes: 'Jenna and Lucas — 2 swimmers registered for Spring 2026. Equipment fee waived.',
}

const MOCK_ITEMS: LineItem[] = [
  { id: 'li-001', description: 'Spring Season Registration — Jenna Park',  quantity: 1, unitCents: 28000, totalCents: 28000, category: 'Registration' },
  { id: 'li-002', description: 'Spring Season Registration — Lucas Park',  quantity: 1, unitCents: 28000, totalCents: 28000, category: 'Registration' },
  { id: 'li-003', description: 'USA Swimming Annual Fee — Jenna Park',    quantity: 1, unitCents: 8000,  totalCents: 8000,  category: 'USA Swimming' },
  { id: 'li-004', description: 'USA Swimming Annual Fee — Lucas Park',    quantity: 1, unitCents: 8000,  totalCents: 8000,  category: 'USA Swimming' },
]

const MOCK_PAYMENTS: Payment[] = [
  {
    id: 'pay-001',
    method: 'stripe',
    amountCents: 36000,
    status: 'succeeded',
    stripeIntentId: 'pi_3PxGasClDcKs8Hha0',
    note: null,
    recordedBy: 'Stripe Webhook',
    recordedAt: '2026-03-03T14:22:00Z',
  },
]

const MOCK_REFUNDS: Refund[] = []

const MOCK_LEDGER: LedgerEntry[] = [
  { id: 'le-001', action: 'invoice_issued',    amountCents: 72000,  balanceCents: 72000, memo: 'Invoice INV-2026-0043 issued', actorId: 'Admin Rivera', occurredAt: '2026-03-02T09:00:00Z' },
  { id: 'le-002', action: 'payment_succeeded', amountCents: -36000, balanceCents: 36000, memo: 'Stripe payment pi_3PxGasClDcKs8Hha0', actorId: 'Stripe', occurredAt: '2026-03-03T14:22:00Z' },
]

// ─── Config ───────────────────────────────────────────────────────────────────

const STATUS_CFG: Record<InvoiceStatus, { label: string; color: string; bg: string }> = {
  draft:            { label: 'Draft',          color: 'var(--text-muted)',  bg: 'var(--bg-elevated)' },
  issued:           { label: 'Issued',         color: 'var(--info)',        bg: 'var(--info-bg)' },
  partially_paid:   { label: 'Partial',        color: 'var(--warning)',     bg: 'var(--warning-bg)' },
  paid:             { label: 'Paid',           color: 'var(--success)',     bg: 'var(--success-bg)' },
  refunded_partial: { label: 'Partial Refund', color: 'var(--accent)',      bg: 'var(--accent-subtle)' },
  refunded_full:    { label: 'Refunded',       color: 'var(--accent)',      bg: 'var(--accent-subtle)' },
  voided:           { label: 'Voided',         color: 'var(--text-muted)',  bg: 'var(--bg-elevated)' },
  written_off:      { label: 'Written Off',    color: 'var(--text-muted)',  bg: 'var(--bg-elevated)' },
}

const PAYMENT_METHOD_LABELS: Record<PaymentMethod, string> = {
  stripe:        'Stripe',
  check:         'Check',
  cash:          'Cash',
  bank_transfer: 'Bank Transfer',
}

const PAYMENT_STATUS_CFG: Record<PaymentStatus, { color: string }> = {
  pending:   { color: 'var(--warning)' },
  succeeded: { color: 'var(--success)' },
  failed:    { color: 'var(--error)' },
}

const LEDGER_ACTION_CFG: Record<LedgerAction, { label: string; color: string }> = {
  invoice_issued:      { label: 'Invoice Issued',    color: 'var(--info)' },
  payment_succeeded:   { label: 'Payment Received',  color: 'var(--success)' },
  payment_failed:      { label: 'Payment Failed',    color: 'var(--error)' },
  refund_processed:    { label: 'Refund Processed',  color: 'var(--accent)' },
  invoice_voided:      { label: 'Invoice Voided',    color: 'var(--text-muted)' },
  invoice_written_off: { label: 'Written Off',       color: 'var(--text-muted)' },
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatCents(cents: number) {
  const abs = Math.abs(cents)
  const fmt = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(abs / 100)
  return cents < 0 ? `−${fmt}` : fmt
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function formatDateTime(iso: string) {
  return new Date(iso).toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

// ─── Table Columns ────────────────────────────────────────────────────────────

const ITEM_COLS: Column<LineItem>[] = [
  {
    key: 'description',
    label: 'Description',
    render: (r) => (
      <div>
        <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{r.description}</div>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px', fontFamily: 'var(--font-mono)' }}>{r.category}</div>
      </div>
    ),
  },
  {
    key: 'quantity',
    label: 'Qty',
    align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{r.quantity}</span>,
  },
  {
    key: 'unitCents',
    label: 'Unit Price',
    align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatCents(r.unitCents)}</span>,
  },
  {
    key: 'totalCents',
    label: 'Total',
    align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--text)' }}>{formatCents(r.totalCents)}</span>,
  },
]

const PAYMENT_COLS: Column<Payment>[] = [
  {
    key: 'method',
    label: 'Method',
    render: (r) => <span style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text-subtle)' }}>{PAYMENT_METHOD_LABELS[r.method]}</span>,
  },
  {
    key: 'amountCents',
    label: 'Amount',
    align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--success)' }}>{formatCents(r.amountCents)}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (r) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', fontWeight: 600, color: PAYMENT_STATUS_CFG[r.status].color }}>
        {r.status}
      </span>
    ),
  },
  {
    key: 'stripeIntentId',
    label: 'Reference',
    render: (r) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)' }}>
        {r.stripeIntentId ?? '—'}
      </span>
    ),
  },
  {
    key: 'recordedBy',
    label: 'Recorded By',
    render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{r.recordedBy}</span>,
  },
  {
    key: 'recordedAt',
    label: 'Date',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(r.recordedAt)}</span>,
  },
]

const REFUND_COLS: Column<Refund>[] = [
  {
    key: 'amountCents',
    label: 'Amount',
    align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--accent)' }}>{formatCents(r.amountCents)}</span>,
  },
  {
    key: 'reason',
    label: 'Reason',
    render: (r) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{r.reason}</span>,
  },
  {
    key: 'note',
    label: 'Note',
    render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>{r.note ?? '—'}</span>,
  },
  {
    key: 'createdBy',
    label: 'By',
    render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{r.createdBy}</span>,
  },
  {
    key: 'createdAt',
    label: 'Date',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(r.createdAt)}</span>,
  },
]

const LEDGER_COLS: Column<LedgerEntry>[] = [
  {
    key: 'action',
    label: 'Action',
    render: (r) => {
      const c = LEDGER_ACTION_CFG[r.action]
      return <span style={{ fontSize: '12px', fontWeight: 600, color: c.color, fontFamily: 'var(--font-mono)' }}>{c.label}</span>
    },
  },
  {
    key: 'amountCents',
    label: 'Amount',
    align: 'right',
    render: (r) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: r.amountCents < 0 ? 'var(--success)' : r.action === 'invoice_issued' ? 'var(--text)' : 'var(--accent)' }}>
        {formatCents(r.amountCents)}
      </span>
    ),
  },
  {
    key: 'balanceCents',
    label: 'Balance',
    align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatCents(r.balanceCents)}</span>,
  },
  {
    key: 'memo',
    label: 'Memo',
    render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>{r.memo}</span>,
  },
  {
    key: 'actorId',
    label: 'Actor',
    render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>{r.actorId}</span>,
  },
  {
    key: 'occurredAt',
    label: 'Time',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)' }}>{formatDateTime(r.occurredAt)}</span>,
  },
]

// ─── Lifecycle Actions ────────────────────────────────────────────────────────

function lifecycleActions(status: InvoiceStatus) {
  switch (status) {
    case 'draft':          return [{ label: 'Issue Invoice', icon: 'file-text' as const, primary: true }]
    case 'issued':         return [{ label: 'Record Payment', icon: 'check-circle' as const, primary: true }, { label: 'Void Invoice', icon: 'x' as const, primary: false }]
    case 'partially_paid': return [{ label: 'Record Payment', icon: 'check-circle' as const, primary: true }, { label: 'Create Refund', icon: 'refresh-cw' as const, primary: false }]
    case 'paid':           return [{ label: 'Create Refund', icon: 'refresh-cw' as const, primary: false }]
    default:               return []
  }
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function InvoiceDetailPage() {
  const [activeTab, setActiveTab] = useState('overview')

  const inv = MOCK_INVOICE
  const statusCfg = STATUS_CFG[inv.status]
  const actions = lifecycleActions(inv.status)
  const payProgress = inv.totalCents > 0 ? (inv.paidCents / inv.totalCents) * 100 : 0

  const tabs = [
    { id: 'overview',   label: 'Overview' },
    { id: 'items',      label: 'Line Items', count: MOCK_ITEMS.length },
    { id: 'payments',   label: 'Payments',   count: MOCK_PAYMENTS.length },
    { id: 'refunds',    label: 'Refunds',    count: MOCK_REFUNDS.length },
    { id: 'ledger',     label: 'Ledger',     count: MOCK_LEDGER.length },
  ]

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      {/* Back link */}
      <Link href="/invoices" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--text)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-subtle)')}>
        <Icon name="chevron-left" size={14} /> Invoices
      </Link>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px', gap: '16px' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '6px' }}>
            <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--text)', lineHeight: 1.2 }}>
              {inv.invoiceNumber}
            </h1>
            <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: statusCfg.bg, color: statusCfg.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em', whiteSpace: 'nowrap' as const }}>
              {statusCfg.label}
            </span>
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', display: 'flex', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
            <Link href={`/members/households/${inv.householdId}`} style={{ color: 'var(--text-muted)', textDecoration: 'none' }}
              onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
              onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}>
              {inv.household}
            </Link>
            <span>·</span>
            <span>{inv.primaryContact}</span>
            <span>·</span>
            <span>Issued {inv.issuedAt ? formatDate(inv.issuedAt) : '—'}</span>
            {inv.dueDate && (
              <>
                <span>·</span>
                <span>Due {formatDate(inv.dueDate)}</span>
              </>
            )}
          </div>
        </div>

        <div style={{ display: 'flex', gap: '8px', flexShrink: 0 }}>
          {actions.map((a) => (
            <button
              key={a.label}
              className={a.primary ? 'btn btn-primary' : 'btn btn-secondary'}
              style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '13px' }}
            >
              <Icon name={a.icon} size={13} /> {a.label}
            </button>
          ))}
        </div>
      </div>

      {/* Amount summary bar */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'Total',    value: formatCents(inv.totalCents),   color: 'var(--text)' },
          { label: 'Paid',     value: formatCents(inv.paidCents),    color: 'var(--success)' },
          { label: 'Balance',  value: formatCents(inv.balanceCents), color: inv.balanceCents > 0 ? 'var(--warning)' : 'var(--text-muted)' },
          { label: 'Progress', value: `${Math.round(payProgress)}%`, color: payProgress >= 100 ? 'var(--success)' : 'var(--warning)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Payment progress bar */}
      <div style={{ marginBottom: '28px' }}>
        <div style={{ height: '5px', borderRadius: '3px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
          <div style={{ height: '100%', width: `${payProgress}%`, background: payProgress >= 100 ? 'var(--success)' : 'var(--accent)', borderRadius: '3px', transition: 'width 0.6s var(--ease-out)' }} />
        </div>
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {/* Overview */}
      {activeTab === 'overview' && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Invoice Details</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[
                { label: 'Invoice #',  value: inv.invoiceNumber },
                { label: 'Household', value: inv.household },
                { label: 'Contact',   value: inv.primaryContact },
                { label: 'Email',     value: inv.primaryEmail },
                { label: 'Created',   value: formatDateTime(inv.createdAt) },
                { label: 'Issued',    value: inv.issuedAt ? formatDateTime(inv.issuedAt) : '—' },
                { label: 'Due Date',  value: inv.dueDate ? formatDate(inv.dueDate) : '—' },
                { label: 'Created By',value: inv.createdBy },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600, textAlign: 'right', maxWidth: '60%', wordBreak: 'break-all' }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
            {/* Totals card */}
            <div className="card" style={{ padding: '20px' }}>
              <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Amount Summary</h3>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                {MOCK_ITEMS.map((item) => (
                  <div key={item.id} style={{ display: 'flex', justifyContent: 'space-between', fontSize: '13px' }}>
                    <span style={{ color: 'var(--text-subtle)', flex: 1, paddingRight: '12px' }}>{item.description}</span>
                    <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600, flexShrink: 0 }}>{formatCents(item.totalCents)}</span>
                  </div>
                ))}
                <div style={{ borderTop: '1px solid var(--border)', paddingTop: '10px', display: 'flex', justifyContent: 'space-between', fontSize: '14px', fontWeight: 700 }}>
                  <span style={{ color: 'var(--text)' }}>Total</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>{formatCents(inv.totalCents)}</span>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '13px' }}>
                  <span style={{ color: 'var(--success)' }}>Paid</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--success)', fontWeight: 600 }}>{formatCents(inv.paidCents)}</span>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '14px', fontWeight: 700 }}>
                  <span style={{ color: inv.balanceCents > 0 ? 'var(--warning)' : 'var(--text-muted)' }}>Balance Due</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: inv.balanceCents > 0 ? 'var(--warning)' : 'var(--text-muted)' }}>{formatCents(inv.balanceCents)}</span>
                </div>
              </div>
            </div>

            {/* Notes */}
            {inv.notes && (
              <div className="card" style={{ padding: '20px' }}>
                <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '12px', fontFamily: 'var(--font-mono)' }}>Notes</h3>
                <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7 }}>{inv.notes}</p>
              </div>
            )}

            {/* Description */}
            {inv.description && (
              <div className="card" style={{ padding: '20px' }}>
                <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '12px', fontFamily: 'var(--font-mono)' }}>Description</h3>
                <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7 }}>{inv.description}</p>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Line Items */}
      {activeTab === 'items' && (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{MOCK_ITEMS.length} line items</p>
            {['draft','issued'].includes(inv.status) && (
              <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px' }}>
                <Icon name="plus" size={12} /> Add Item
              </button>
            )}
          </div>
          <Table columns={ITEM_COLS} data={MOCK_ITEMS} emptyTitle="No line items" emptyDescription="Add items to this invoice." />
          <div className="card" style={{ padding: '14px 20px', marginTop: '12px', display: 'flex', justifyContent: 'flex-end' }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', alignItems: 'flex-end', minWidth: '200px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', width: '100%', gap: '24px', fontSize: '14px', fontWeight: 700 }}>
                <span style={{ color: 'var(--text-muted)' }}>Total</span>
                <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>{formatCents(inv.totalCents)}</span>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Payments */}
      {activeTab === 'payments' && (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{MOCK_PAYMENTS.length} payment{MOCK_PAYMENTS.length !== 1 ? 's' : ''} · {formatCents(inv.paidCents)} total collected</p>
            {['issued','partially_paid'].includes(inv.status) && (
              <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px' }}>
                <Icon name="check-circle" size={12} /> Record Payment
              </button>
            )}
          </div>
          {MOCK_PAYMENTS.length === 0
            ? <EmptyState title="No payments" description="Payments will appear here once recorded or received via Stripe." />
            : <Table columns={PAYMENT_COLS} data={MOCK_PAYMENTS} emptyTitle="No payments" emptyDescription="No payments recorded." />
          }
        </div>
      )}

      {/* Refunds */}
      {activeTab === 'refunds' && (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{MOCK_REFUNDS.length} refund{MOCK_REFUNDS.length !== 1 ? 's' : ''}</p>
            {['paid','partially_paid','refunded_partial'].includes(inv.status) && (
              <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px' }}>
                <Icon name="refresh-cw" size={12} /> Create Refund
              </button>
            )}
          </div>
          {MOCK_REFUNDS.length === 0
            ? <EmptyState title="No refunds" description="Refunds will appear here once processed." />
            : <Table columns={REFUND_COLS} data={MOCK_REFUNDS} emptyTitle="No refunds" emptyDescription="No refunds issued." />
          }
        </div>
      )}

      {/* Ledger */}
      {activeTab === 'ledger' && (
        <div>
          <div style={{ fontSize: '13px', color: 'var(--text-subtle)', marginBottom: '12px' }}>
            Append-only ledger for this invoice — all financial events in chronological order
          </div>
          <Table columns={LEDGER_COLS} data={MOCK_LEDGER} emptyTitle="No ledger entries" emptyDescription="Ledger entries are created when this invoice is issued, paid, or refunded." />
        </div>
      )}
    </div>
  )
}
