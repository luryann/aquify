'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type InvoiceStatus =
  | 'draft'
  | 'issued'
  | 'partially_paid'
  | 'paid'
  | 'refunded_partial'
  | 'refunded_full'
  | 'voided'
  | 'written_off'

type Invoice = {
  id: string
  invoiceNumber: string
  household: string
  householdId: string
  totalCents: number
  paidCents: number
  status: InvoiceStatus
  issuedAt: string | null
  dueDate: string | null
  createdAt: string
  description: string | null
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_INVOICES: Invoice[] = [
  {
    id: 'inv-001',
    invoiceNumber: 'INV-2026-0042',
    household: 'Henderson Family',
    householdId: 'hh-001',
    totalCents: 85000,
    paidCents: 85000,
    status: 'paid',
    issuedAt: '2026-03-01T09:00:00Z',
    dueDate: '2026-03-31',
    createdAt: '2026-02-28T14:00:00Z',
    description: 'Spring season registration + meet fees',
  },
  {
    id: 'inv-002',
    invoiceNumber: 'INV-2026-0043',
    household: 'Park Family',
    householdId: 'hh-002',
    totalCents: 72000,
    paidCents: 36000,
    status: 'partially_paid',
    issuedAt: '2026-03-02T09:00:00Z',
    dueDate: '2026-03-31',
    createdAt: '2026-03-01T10:00:00Z',
    description: 'Spring season registration',
  },
  {
    id: 'inv-003',
    invoiceNumber: 'INV-2026-0044',
    household: 'Torres Household',
    householdId: 'hh-003',
    totalCents: 120000,
    paidCents: 0,
    status: 'issued',
    issuedAt: '2026-03-05T09:00:00Z',
    dueDate: '2026-03-31',
    createdAt: '2026-03-04T11:00:00Z',
    description: 'Spring season + equipment fee',
  },
  {
    id: 'inv-004',
    invoiceNumber: 'INV-2026-0045',
    household: 'Okafor Household',
    householdId: 'hh-004',
    totalCents: 64500,
    paidCents: 0,
    status: 'issued',
    issuedAt: '2026-03-06T09:00:00Z',
    dueDate: '2026-03-31',
    createdAt: '2026-03-05T08:30:00Z',
    description: 'Spring season — 3 swimmers',
  },
  {
    id: 'inv-005',
    invoiceNumber: 'INV-2026-0039',
    household: 'Chen Family',
    householdId: 'hh-005',
    totalCents: 48000,
    paidCents: 0,
    status: 'draft',
    issuedAt: null,
    dueDate: null,
    createdAt: '2026-03-15T16:00:00Z',
    description: 'New member registration fees',
  },
  {
    id: 'inv-006',
    invoiceNumber: 'INV-2026-0038',
    household: 'Kim Family',
    householdId: 'hh-006',
    totalCents: 55000,
    paidCents: 55000,
    status: 'refunded_partial',
    issuedAt: '2026-02-15T09:00:00Z',
    dueDate: '2026-03-15',
    createdAt: '2026-02-14T09:00:00Z',
    description: 'Winter season + equipment',
  },
  {
    id: 'inv-007',
    invoiceNumber: 'INV-2025-0101',
    household: 'Lee Family',
    householdId: 'hh-007',
    totalCents: 36000,
    paidCents: 0,
    status: 'written_off',
    issuedAt: '2025-09-01T09:00:00Z',
    dueDate: '2025-09-30',
    createdAt: '2025-09-01T09:00:00Z',
    description: 'Fall season — uncollectible',
  },
  {
    id: 'inv-008',
    invoiceNumber: 'INV-2026-0041',
    household: 'Rivera Household',
    householdId: 'hh-008',
    totalCents: 22000,
    paidCents: 0,
    status: 'voided',
    issuedAt: '2026-02-20T09:00:00Z',
    dueDate: '2026-03-20',
    createdAt: '2026-02-20T09:00:00Z',
    description: 'Duplicate invoice — voided',
  },
]

// ─── Config ───────────────────────────────────────────────────────────────────

const STATUS_CFG: Record<InvoiceStatus, { label: string; color: string; bg: string }> = {
  draft:             { label: 'Draft',            color: 'var(--text-muted)',   bg: 'var(--bg-elevated)' },
  issued:            { label: 'Issued',           color: 'var(--info)',         bg: 'var(--info-bg)' },
  partially_paid:    { label: 'Partial',          color: 'var(--warning)',      bg: 'var(--warning-bg)' },
  paid:              { label: 'Paid',             color: 'var(--success)',      bg: 'var(--success-bg)' },
  refunded_partial:  { label: 'Partial Refund',   color: 'var(--accent)',       bg: 'var(--accent-subtle)' },
  refunded_full:     { label: 'Refunded',         color: 'var(--accent)',       bg: 'var(--accent-subtle)' },
  voided:            { label: 'Voided',           color: 'var(--text-muted)',   bg: 'var(--bg-elevated)' },
  written_off:       { label: 'Written Off',      color: 'var(--text-muted)',   bg: 'var(--bg-elevated)' },
}

const STATUS_FILTER_OPTIONS = [
  { value: 'all',             label: 'All Statuses' },
  { value: 'draft',           label: 'Draft' },
  { value: 'issued',          label: 'Issued' },
  { value: 'partially_paid',  label: 'Partial' },
  { value: 'paid',            label: 'Paid' },
  { value: 'refunded_partial',label: 'Partial Refund' },
  { value: 'refunded_full',   label: 'Refunded' },
  { value: 'voided',          label: 'Voided' },
  { value: 'written_off',     label: 'Written Off' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100)
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function isOverdue(invoice: Invoice) {
  if (!invoice.dueDate) return false
  if (['paid','refunded_full','voided','written_off'].includes(invoice.status)) return false
  return new Date(invoice.dueDate) < new Date()
}

// ─── Table Columns ────────────────────────────────────────────────────────────

const COLUMNS: Column<Invoice>[] = [
  {
    key: 'invoiceNumber',
    label: 'Invoice',
    render: (r) => (
      <Link href={`/invoices/${r.id}`} style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--text)', textDecoration: 'none' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
        {r.invoiceNumber}
      </Link>
    ),
  },
  {
    key: 'household',
    label: 'Household',
    sortable: true,
    render: (r) => (
      <Link href={`/members/households/${r.householdId}`} style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
        {r.household}
      </Link>
    ),
  },
  {
    key: 'description',
    label: 'Description',
    render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)', maxWidth: '200px', display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{r.description ?? '—'}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (r) => {
      const c = STATUS_CFG[r.status]
      const overdue = isOverdue(r)
      return (
        <div style={{ display: 'flex', gap: '5px', alignItems: 'center', flexWrap: 'wrap' }}>
          <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', whiteSpace: 'nowrap' as const }}>
            {c.label}
          </span>
          {overdue && (
            <span style={{ fontSize: '10px', fontWeight: 700, padding: '2px 6px', borderRadius: '8px', background: 'var(--error-bg)', color: 'var(--error)', fontFamily: 'var(--font-mono)' }}>
              OVERDUE
            </span>
          )}
        </div>
      )
    },
  },
  {
    key: 'totalCents',
    label: 'Amount',
    align: 'right',
    render: (r) => (
      <div style={{ textAlign: 'right' }}>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--text)' }}>{formatCents(r.totalCents)}</div>
        {r.paidCents > 0 && r.paidCents < r.totalCents && (
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--success)' }}>{formatCents(r.paidCents)} paid</div>
        )}
      </div>
    ),
  },
  {
    key: 'issuedAt',
    label: 'Issued',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{r.issuedAt ? formatDate(r.issuedAt) : '—'}</span>,
  },
  {
    key: 'dueDate',
    label: 'Due',
    render: (r) => {
      if (!r.dueDate) return <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>—</span>
      const overdue = isOverdue(r)
      return (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: overdue ? 'var(--error)' : 'var(--text-muted)', fontWeight: overdue ? 600 : 400 }}>
          {formatDate(r.dueDate)}
        </span>
      )
    },
  },
  {
    key: 'id',
    label: '',
    render: (r) => (
      <Link href={`/invoices/${r.id}`} style={{ fontSize: '12px', color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '3px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}>
        View <Icon name="chevron-right" size={12} />
      </Link>
    ),
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function InvoicesPage() {
  const [search, setSearch]       = useState('')
  const [statusFilter, setStatus] = useState('all')

  const totalIssued    = MOCK_INVOICES.filter((i) => !['draft','voided','written_off'].includes(i.status)).reduce((s, i) => s + i.totalCents, 0)
  const paidAmount     = MOCK_INVOICES.filter((i) => i.status === 'paid').reduce((s, i) => s + i.totalCents, 0)
  const outstandingAmt = MOCK_INVOICES.filter((i) => ['issued','partially_paid'].includes(i.status)).reduce((s, i) => s + (i.totalCents - i.paidCents), 0)
  const overdueCount   = MOCK_INVOICES.filter(isOverdue).length

  const filtered = useMemo(() => {
    return MOCK_INVOICES.filter((inv) => {
      const q = search.toLowerCase()
      const matchSearch = !q
        || inv.household.toLowerCase().includes(q)
        || inv.invoiceNumber.toLowerCase().includes(q)
        || (inv.description?.toLowerCase().includes(q) ?? false)
      const matchStatus = statusFilter === 'all' || inv.status === statusFilter
      return matchSearch && matchStatus
    })
  }, [search, statusFilter])

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      {/* Page header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>
            Invoices
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
            Household billing — dues, fees, and registrations
          </p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <Icon name="plus" size={13} /> New Invoice
        </button>
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Open Balance',  value: formatCents(outstandingAmt), color: outstandingAmt > 0 ? 'var(--warning)' : 'var(--text)' },
          { label: 'Collected',     value: formatCents(paidAmount),      color: 'var(--success)' },
          { label: 'Total Issued',  value: formatCents(totalIssued),     color: 'var(--text)' },
          { label: 'Overdue',       value: overdueCount.toString(),      color: overdueCount > 0 ? 'var(--error)' : 'var(--text-muted)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 280px' }}>
          <Icon name="search" size={14} color="var(--text-muted)" style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' } as React.CSSProperties} />
          <input
            type="text"
            placeholder="Search by household, invoice #, or description…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="input"
            style={{ paddingLeft: '32px', width: '100%' }}
          />
        </div>
        <select value={statusFilter} onChange={(e) => setStatus(e.target.value)} className="input" style={{ minWidth: '160px' }}>
          {STATUS_FILTER_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </div>

      {/* Result count */}
      <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '12px' }}>
        {filtered.length} of {MOCK_INVOICES.length} invoices
      </div>

      {/* Table */}
      {filtered.length === 0
        ? <EmptyState title="No invoices found" description="Try adjusting your filters or create a new invoice." />
        : <Table columns={COLUMNS} data={filtered} pageSize={15} emptyTitle="No invoices" emptyDescription="Create an invoice from a household page." />
      }
    </div>
  )
}
