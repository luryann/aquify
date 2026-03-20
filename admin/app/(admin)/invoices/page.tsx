'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchHouseholds, fetchInvoicesForHousehold, type InvoiceRecord } from '@/lib/adminData'

type InvoiceStatus = 'draft' | 'issued' | 'partially_paid' | 'paid' | 'refunded_partial' | 'refunded_full' | 'voided' | 'written_off'

type InvoiceRow = {
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

const STATUS_CFG: Record<InvoiceStatus, { label: string; color: string; bg: string }> = {
  draft: { label: 'Draft', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  issued: { label: 'Issued', color: 'var(--info)', bg: 'var(--info-bg)' },
  partially_paid: { label: 'Partial', color: 'var(--warning)', bg: 'var(--warning-bg)' },
  paid: { label: 'Paid', color: 'var(--success)', bg: 'var(--success-bg)' },
  refunded_partial: { label: 'Partial Refund', color: 'var(--accent)', bg: 'var(--accent-subtle)' },
  refunded_full: { label: 'Refunded', color: 'var(--accent)', bg: 'var(--accent-subtle)' },
  voided: { label: 'Voided', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  written_off: { label: 'Written Off', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100)
}

function isOverdue(invoice: InvoiceRow) {
  if (!invoice.dueDate) return false
  if (['paid', 'refunded_full', 'voided', 'written_off'].includes(invoice.status)) return false
  return new Date(invoice.dueDate) < new Date()
}

const COLUMNS: Column<InvoiceRow>[] = [
  {
    key: 'invoiceNumber',
    label: 'Invoice',
    render: (row) => <Link href={`/invoices/${row.id}`} style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--text)', textDecoration: 'none' }}>{row.invoiceNumber}</Link>,
  },
  {
    key: 'household',
    label: 'Household',
    sortable: true,
    render: (row) => <Link href={`/members/households/${row.householdId}`} style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}>{row.household}</Link>,
  },
  {
    key: 'description',
    label: 'Description',
    render: (row) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)', maxWidth: '240px', display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{row.description ?? '—'}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => {
      const c = STATUS_CFG[row.status]
      return <div style={{ display: 'flex', gap: '5px', alignItems: 'center', flexWrap: 'wrap' }}>
        <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', whiteSpace: 'nowrap' }}>{c.label}</span>
        {isOverdue(row) && <span style={{ fontSize: '10px', fontWeight: 700, padding: '2px 6px', borderRadius: '8px', background: 'var(--error-bg)', color: 'var(--error)', fontFamily: 'var(--font-mono)' }}>OVERDUE</span>}
      </div>
    },
  },
  {
    key: 'totalCents',
    label: 'Amount',
    align: 'right',
    render: (row) => (
      <div style={{ textAlign: 'right' }}>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--text)' }}>{formatCents(row.totalCents)}</div>
        {row.paidCents > 0 && row.paidCents < row.totalCents && <div style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--success)' }}>{formatCents(row.paidCents)} paid</div>}
      </div>
    ),
  },
  { key: 'issuedAt', label: 'Issued', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{row.issuedAt ? new Date(row.issuedAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : '—'}</span> },
  { key: 'dueDate', label: 'Due', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: isOverdue(row) ? 'var(--error)' : 'var(--text-muted)' }}>{row.dueDate ?? '—'}</span> },
]

async function loadInvoices(api: ReturnType<typeof useApiClient>, orgId: string): Promise<InvoiceRow[]> {
  const households = await fetchHouseholds(api, orgId)
  const invoicesByHousehold = await Promise.all(households.map(async (household) => ({
    household,
    invoices: await fetchInvoicesForHousehold(api, orgId, household.id).catch(() => []),
  })))

  return invoicesByHousehold.flatMap(({ household, invoices }) => invoices.map((invoice: InvoiceRecord) => ({
    id: invoice.id,
    invoiceNumber: invoice.invoice_number,
    household: household.household_name,
    householdId: household.id,
    totalCents: invoice.total_cents,
    paidCents: invoice.amount_paid_cents,
    status: invoice.status as InvoiceStatus,
    issuedAt: invoice.issued_at || null,
    dueDate: invoice.due_date || null,
    createdAt: invoice.created_at,
    description: invoice.notes || null,
  }))).sort((a, b) => b.createdAt.localeCompare(a.createdAt))
}

export default function InvoicesPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<InvoiceRow[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId) {
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        setError(null)
        const invoices = await loadInvoices(api, session.user.orgId)
        if (!cancelled) setRows(invoices)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load invoices.')
          setRows([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId])

  const filtered = useMemo(() => rows.filter((row) => {
    const query = search.toLowerCase()
    const matchesSearch = !search || row.household.toLowerCase().includes(query) || row.invoiceNumber.toLowerCase().includes(query) || (row.description ?? '').toLowerCase().includes(query)
    const matchesStatus = statusFilter === 'all' || row.status === statusFilter
    return matchesSearch && matchesStatus
  }), [rows, search, statusFilter])

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Invoices</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Live billing invoices aggregated across persisted household accounts</p>
        </div>
      </div>

      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '320px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search invoices or households…" value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', cursor: 'pointer' }}>
          <option value="all">All Statuses</option>
          {Object.entries(STATUS_CFG).map(([value, cfg]) => <option key={value} value={value}>{cfg.label}</option>)}
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>{filtered.length} invoices</span>
      </div>

      {loading ? (
        <EmptyState icon="file-text" title="Loading invoices" description="Fetching live invoice data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load invoices" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No invoices found" emptyDescription="Issue an invoice to see it here." />
      )}
    </div>
  )
}
