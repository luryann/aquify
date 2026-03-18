'use client'

import { useState, useMemo } from 'react'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type LedgerEventType =
  | 'invoice_issued'
  | 'payment_succeeded'
  | 'refund_processed'
  | 'invoice_voided'
  | 'invoice_written_off'

type LedgerEvent = {
  id: string
  eventType: LedgerEventType
  invoiceId: string
  invoiceNumber: string
  householdId: string
  householdName: string
  description: string
  creditCents: number
  debitCents: number
  balanceCents: number
  actorName: string
  createdAt: string
}

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const MOCK_EVENTS: LedgerEvent[] = [
  { id: 'le-001', eventType: 'invoice_issued',     invoiceId: 'inv-001', invoiceNumber: 'INV-2026-0001', householdId: 'hh-001', householdName: 'Chen Family',     description: 'Invoice issued — Annual Registration 2026',     creditCents: 85000,  debitCents: 0,      balanceCents: 85000,   actorName: 'Admin User',    createdAt: '2026-03-01T09:00:00Z' },
  { id: 'le-002', eventType: 'payment_succeeded',  invoiceId: 'inv-001', invoiceNumber: 'INV-2026-0001', householdId: 'hh-001', householdName: 'Chen Family',     description: 'Payment received — Stripe (pi_xxx1234)',        creditCents: 0,      debitCents: 85000,  balanceCents: 0,       actorName: 'Stripe Webhook', createdAt: '2026-03-01T10:14:22Z' },
  { id: 'le-003', eventType: 'invoice_issued',     invoiceId: 'inv-002', invoiceNumber: 'INV-2026-0002', householdId: 'hh-002', householdName: 'Okafor Family',   description: 'Invoice issued — Annual Registration 2026',     creditCents: 85000,  debitCents: 0,      balanceCents: 85000,   actorName: 'Admin User',    createdAt: '2026-03-02T08:30:00Z' },
  { id: 'le-004', eventType: 'payment_succeeded',  invoiceId: 'inv-002', invoiceNumber: 'INV-2026-0002', householdId: 'hh-002', householdName: 'Okafor Family',   description: 'Payment received — Check (#4421)',               creditCents: 0,      debitCents: 42500,  balanceCents: 42500,   actorName: 'billing@dare.org', createdAt: '2026-03-03T14:05:00Z' },
  { id: 'le-005', eventType: 'invoice_issued',     invoiceId: 'inv-003', invoiceNumber: 'INV-2026-0003', householdId: 'hh-003', householdName: 'Martinez Family', description: 'Invoice issued — Spring Championships Entry',   creditCents: 12000,  debitCents: 0,      balanceCents: 12000,   actorName: 'Admin User',    createdAt: '2026-03-05T11:00:00Z' },
  { id: 'le-006', eventType: 'payment_succeeded',  invoiceId: 'inv-003', invoiceNumber: 'INV-2026-0003', householdId: 'hh-003', householdName: 'Martinez Family', description: 'Payment received — Stripe (pi_xxx5678)',        creditCents: 0,      debitCents: 12000,  balanceCents: 0,       actorName: 'Stripe Webhook', createdAt: '2026-03-05T11:22:10Z' },
  { id: 'le-007', eventType: 'invoice_issued',     invoiceId: 'inv-004', invoiceNumber: 'INV-2026-0004', householdId: 'hh-004', householdName: 'Henderson Family','description': 'Invoice issued — Annual Registration 2026',   creditCents: 85000,  debitCents: 0,      balanceCents: 85000,   actorName: 'Admin User',    createdAt: '2026-03-06T09:00:00Z' },
  { id: 'le-008', eventType: 'refund_processed',   invoiceId: 'inv-004', invoiceNumber: 'INV-2026-0004', householdId: 'hh-004', householdName: 'Henderson Family','description': 'Refund issued — Duplicate charge corrected',  creditCents: 15000,  debitCents: 0,      balanceCents: 100000,  actorName: 'Admin User',    createdAt: '2026-03-07T16:41:00Z' },
  { id: 'le-009', eventType: 'invoice_issued',     invoiceId: 'inv-005', invoiceNumber: 'INV-2026-0005', householdId: 'hh-005', householdName: 'Park Family',     description: 'Invoice issued — Equipment/Gear Fee',           creditCents: 4500,   debitCents: 0,      balanceCents: 4500,    actorName: 'Admin User',    createdAt: '2026-03-08T10:00:00Z' },
  { id: 'le-010', eventType: 'invoice_voided',     invoiceId: 'inv-005', invoiceNumber: 'INV-2026-0005', householdId: 'hh-005', householdName: 'Park Family',     description: 'Invoice voided — Billed in error',              creditCents: 0,      debitCents: 4500,   balanceCents: 0,       actorName: 'Admin User',    createdAt: '2026-03-08T10:45:00Z' },
  { id: 'le-011', eventType: 'invoice_issued',     invoiceId: 'inv-006', invoiceNumber: 'INV-2026-0006', householdId: 'hh-006', householdName: 'Thompson Family', description: 'Invoice issued — Late Registration Fee',        creditCents: 5000,   debitCents: 0,      balanceCents: 5000,    actorName: 'Admin User',    createdAt: '2026-03-10T08:00:00Z' },
  { id: 'le-012', eventType: 'payment_succeeded',  invoiceId: 'inv-006', invoiceNumber: 'INV-2026-0006', householdId: 'hh-006', householdName: 'Thompson Family', description: 'Payment received — Cash (receipt #0092)',       creditCents: 0,      debitCents: 5000,   balanceCents: 0,       actorName: 'billing@dare.org', createdAt: '2026-03-10T09:15:00Z' },
  { id: 'le-013', eventType: 'invoice_issued',     invoiceId: 'inv-007', invoiceNumber: 'INV-2026-0007', householdId: 'hh-007', householdName: 'Wilson Family',   description: 'Invoice issued — Annual Registration 2026',     creditCents: 85000,  debitCents: 0,      balanceCents: 85000,   actorName: 'Admin User',    createdAt: '2026-03-11T09:30:00Z' },
  { id: 'le-014', eventType: 'invoice_written_off',invoiceId: 'inv-007', invoiceNumber: 'INV-2026-0007', householdId: 'hh-007', householdName: 'Wilson Family',   description: 'Balance written off — Financial hardship',      creditCents: 0,      debitCents: 85000,  balanceCents: 0,       actorName: 'Owner',         createdAt: '2026-03-12T14:00:00Z' },
  { id: 'le-015', eventType: 'invoice_issued',     invoiceId: 'inv-008', invoiceNumber: 'INV-2026-0008', householdId: 'hh-001', householdName: 'Chen Family',     description: 'Invoice issued — State Qualifier Entry Fee',    creditCents: 8500,   debitCents: 0,      balanceCents: 8500,    actorName: 'Admin User',    createdAt: '2026-03-14T10:00:00Z' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatCents(cents: number): string {
  if (cents === 0) return '—'
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100)
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: '2-digit', minute: '2-digit', hour12: false,
  })
}

const EVENT_TYPE_LABELS: Record<LedgerEventType, string> = {
  invoice_issued:     'Issued',
  payment_succeeded:  'Payment',
  refund_processed:   'Refund',
  invoice_voided:     'Voided',
  invoice_written_off:'Write-Off',
}

const EVENT_TYPE_COLORS: Record<LedgerEventType, string> = {
  invoice_issued:     'var(--info)',
  payment_succeeded:  'var(--success)',
  refund_processed:   'var(--warning)',
  invoice_voided:     'var(--text-muted)',
  invoice_written_off:'var(--error)',
}

const EVENT_TYPE_BG: Record<LedgerEventType, string> = {
  invoice_issued:     'var(--info-bg)',
  payment_succeeded:  'var(--success-bg)',
  refund_processed:   'var(--warning-bg)',
  invoice_voided:     'var(--border)',
  invoice_written_off:'var(--error-bg)',
}

const EVENT_TYPE_BORDER: Record<LedgerEventType, string> = {
  invoice_issued:     'var(--info-border)',
  payment_succeeded:  'var(--success-border)',
  refund_processed:   'var(--warning-border)',
  invoice_voided:     'var(--border)',
  invoice_written_off:'var(--error-border)',
}

// ─── Components ───────────────────────────────────────────────────────────────

function EventTypeBadge({ type }: { type: LedgerEventType }) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 5,
      padding: '2px 8px', borderRadius: 4,
      fontSize: 11, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase',
      color: EVENT_TYPE_COLORS[type],
      background: EVENT_TYPE_BG[type],
      border: `1px solid ${EVENT_TYPE_BORDER[type]}`,
      fontFamily: 'var(--font-mono)',
    }}>
      {EVENT_TYPE_LABELS[type]}
    </span>
  )
}

function StatCard({ label, value, sub, accent }: { label: string; value: string; sub?: string; accent?: boolean }) {
  return (
    <div style={{
      padding: '20px 24px', borderRadius: 10,
      background: 'var(--bg-elevated)',
      border: '1px solid var(--border)',
      display: 'flex', flexDirection: 'column', gap: 6,
    }}>
      <span style={{ fontSize: 11, fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-muted)' }}>
        {label}
      </span>
      <span style={{
        fontSize: 26, fontWeight: 700, letterSpacing: '-0.02em',
        fontFamily: 'var(--font-mono)',
        color: accent ? 'var(--accent)' : 'var(--text)',
      }}>
        {value}
      </span>
      {sub && <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{sub}</span>}
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

const EVENT_TYPES: { value: LedgerEventType | ''; label: string }[] = [
  { value: '', label: 'All event types' },
  { value: 'invoice_issued',     label: 'Invoice Issued' },
  { value: 'payment_succeeded',  label: 'Payment Received' },
  { value: 'refund_processed',   label: 'Refund Processed' },
  { value: 'invoice_voided',     label: 'Invoice Voided' },
  { value: 'invoice_written_off',label: 'Written Off' },
]

export default function LedgerPage() {
  const [search, setSearch]         = useState('')
  const [typeFilter, setTypeFilter] = useState<LedgerEventType | ''>('')
  const [dateFrom, setDateFrom]     = useState('')
  const [dateTo, setDateTo]         = useState('')

  const filtered = useMemo(() => {
    return MOCK_EVENTS.filter(e => {
      if (typeFilter && e.eventType !== typeFilter) return false
      if (search) {
        const q = search.toLowerCase()
        if (!e.householdName.toLowerCase().includes(q) &&
            !e.invoiceNumber.toLowerCase().includes(q) &&
            !e.description.toLowerCase().includes(q)) return false
      }
      if (dateFrom && e.createdAt < dateFrom) return false
      if (dateTo && e.createdAt > dateTo + 'T23:59:59Z') return false
      return true
    })
  }, [search, typeFilter, dateFrom, dateTo])

  const totalCredits = MOCK_EVENTS.reduce((s, e) => s + e.creditCents, 0)
  const totalDebits  = MOCK_EVENTS.reduce((s, e) => s + e.debitCents, 0)
  const netBalance   = totalCredits - totalDebits

  const columns: Column<LedgerEvent>[] = [
    {
      key: 'createdAt', label: 'Timestamp', sortable: true,
      render: (row) => (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>
          {formatDate(row.createdAt)}
        </span>
      ),
    },
    {
      key: 'eventType', label: 'Event', sortable: true,
      render: (row) => <EventTypeBadge type={row.eventType} />,
    },
    {
      key: 'invoiceNumber', label: 'Invoice', sortable: true,
      render: (row) => (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text)' }}>
          {row.invoiceNumber}
        </span>
      ),
    },
    {
      key: 'householdName', label: 'Household', sortable: true,
      render: (row) => (
        <span style={{ fontSize: 13, color: 'var(--text)' }}>{row.householdName}</span>
      ),
    },
    {
      key: 'description', label: 'Description',
      render: (row) => (
        <span style={{ fontSize: 12, color: 'var(--text-subtle)', maxWidth: 320, display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {row.description}
        </span>
      ),
    },
    {
      key: 'creditCents', label: 'Credit', sortable: true,
      render: (row) => (
        <span style={{
          fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 600,
          color: row.creditCents > 0 ? 'var(--success)' : 'var(--text-disabled)',
          textAlign: 'right', display: 'block',
        }}>
          {row.creditCents > 0 ? formatCents(row.creditCents) : '—'}
        </span>
      ),
    },
    {
      key: 'debitCents', label: 'Debit', sortable: true,
      render: (row) => (
        <span style={{
          fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 600,
          color: row.debitCents > 0 ? 'var(--error)' : 'var(--text-disabled)',
          textAlign: 'right', display: 'block',
        }}>
          {row.debitCents > 0 ? formatCents(row.debitCents) : '—'}
        </span>
      ),
    },
    {
      key: 'actorName', label: 'By',
      render: (row) => (
        <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{row.actorName}</span>
      ),
    },
  ]

  return (
    <div style={{ padding: '32px 36px', maxWidth: 1400, margin: '0 auto' }}>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 28 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Icon name="book-open" size={16} color="var(--accent)" />
            </div>
            <h1 style={{ fontSize: 22, fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--text)', margin: 0 }}>
              Billing Ledger
            </h1>
          </div>
          <p style={{ fontSize: 13, color: 'var(--text-muted)', margin: 0 }}>
            Append-only financial event log — {MOCK_EVENTS.length} events
          </p>
        </div>
        <button style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '8px 14px', borderRadius: 7,
          background: 'var(--bg-elevated)', border: '1px solid var(--border)',
          color: 'var(--text-subtle)', fontSize: 12, fontWeight: 500, cursor: 'pointer',
        }}>
          <Icon name="download" size={13} />
          Export CSV
        </button>
      </div>

      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 14, marginBottom: 28 }}>
        <StatCard label="Total Credits"  value={formatCents(totalCredits)} sub="invoices + refunds" />
        <StatCard label="Total Debits"   value={formatCents(totalDebits)}  sub="payments + voids + write-offs" />
        <StatCard label="Net Balance"    value={formatCents(netBalance)}   accent={netBalance < 0} />
        <StatCard label="Total Events"   value={String(MOCK_EVENTS.length)} sub={`${filtered.length} matching filter`} />
      </div>

      {/* Filters */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20,
        padding: '14px 16px', borderRadius: 9,
        background: 'var(--bg-elevated)', border: '1px solid var(--border)',
        flexWrap: 'wrap',
      }}>
        <div style={{ position: 'relative', flex: '1 1 220px', minWidth: 0 }}>
          <Icon name="search" size={14} color="var(--text-muted)"
            style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' }} />
          <input
            value={search} onChange={e => setSearch(e.target.value)}
            placeholder="Search household, invoice, description…"
            style={{
              width: '100%', padding: '7px 10px 7px 32px', borderRadius: 6,
              background: 'var(--bg)', border: '1px solid var(--border)',
              color: 'var(--text)', fontSize: 13, outline: 'none', boxSizing: 'border-box',
            }}
          />
        </div>

        <select
          value={typeFilter} onChange={e => setTypeFilter(e.target.value as LedgerEventType | '')}
          style={{
            padding: '7px 10px', borderRadius: 6, flex: '0 0 auto',
            background: 'var(--bg)', border: '1px solid var(--border)',
            color: typeFilter ? 'var(--text)' : 'var(--text-muted)', fontSize: 13, cursor: 'pointer',
          }}
        >
          {EVENT_TYPES.map(t => <option key={t.value} value={t.value}>{t.label}</option>)}
        </select>

        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <input
            type="date" value={dateFrom} onChange={e => setDateFrom(e.target.value)}
            style={{
              padding: '7px 10px', borderRadius: 6,
              background: 'var(--bg)', border: '1px solid var(--border)',
              color: dateFrom ? 'var(--text)' : 'var(--text-muted)', fontSize: 13,
              colorScheme: 'dark',
            }}
          />
          <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>to</span>
          <input
            type="date" value={dateTo} onChange={e => setDateTo(e.target.value)}
            style={{
              padding: '7px 10px', borderRadius: 6,
              background: 'var(--bg)', border: '1px solid var(--border)',
              color: dateTo ? 'var(--text)' : 'var(--text-muted)', fontSize: 13,
              colorScheme: 'dark',
            }}
          />
        </div>

        {(search || typeFilter || dateFrom || dateTo) && (
          <button
            onClick={() => { setSearch(''); setTypeFilter(''); setDateFrom(''); setDateTo('') }}
            style={{
              padding: '7px 12px', borderRadius: 6,
              background: 'transparent', border: '1px solid var(--border)',
              color: 'var(--text-muted)', fontSize: 12, cursor: 'pointer',
            }}
          >
            Clear
          </button>
        )}
      </div>

      {/* Table */}
      {filtered.length === 0 ? (
        <EmptyState icon="book-open" title="No ledger events" description="Try adjusting your filters." />
      ) : (
        <Table
          data={filtered}
          columns={columns}
          keyField="id"
          defaultSort={{ key: 'createdAt', direction: 'desc' }}
          paginated
          pageSize={20}
        />
      )}

      {/* Immutability notice */}
      <div style={{
        marginTop: 16, padding: '10px 14px', borderRadius: 7,
        background: 'var(--info-bg)', border: '1px solid var(--info-border)',
        display: 'flex', alignItems: 'center', gap: 8,
      }}>
        <Icon name="info" size={13} color="var(--info)" />
        <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
          This ledger is <strong style={{ color: 'var(--text-subtle)' }}>append-only</strong>. Events cannot be edited or deleted.
          Corrections are made through subsequent credit or debit entries.
        </span>
      </div>

    </div>
  )
}
