'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchHouseholds, fetchInvoicesForHousehold, createInvoice, type InvoiceRecord, type HouseholdRecord } from '@/lib/adminData'

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

const INPUT_STYLE: React.CSSProperties = { width: '100%', padding: '9px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' }
const LABEL_STYLE: React.CSSProperties = { fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '5px' }

export default function InvoicesPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<InvoiceRow[]>([])
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showNew, setShowNew] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [form, setForm] = useState({ household_id: '', invoice_number: '', due_date: '', notes: '' })

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
        const [invoices, householdsData] = await Promise.all([loadInvoices(api, session.user.orgId), fetchHouseholds(api, session.user.orgId)])
        if (!cancelled) { setRows(invoices); setHouseholds(householdsData) }
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

  async function handleCreateInvoice(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId) return
    setSubmitting(true)
    setFormError(null)
    try {
      await createInvoice(api, session.user.orgId, { household_id: form.household_id || undefined, invoice_number: form.invoice_number, due_date: form.due_date || undefined, notes: form.notes || undefined })
      setShowNew(false)
      setForm({ household_id: '', invoice_number: '', due_date: '', notes: '' })
      const [invoices, householdsData] = await Promise.all([loadInvoices(api, session.user.orgId), fetchHouseholds(api, session.user.orgId)])
      setRows(invoices); setHouseholds(householdsData)
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : 'Failed to create invoice.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Invoices</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Live billing invoices aggregated across persisted household accounts</p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setShowNew(true)}>
          <Icon name="plus" size={14} /> New Invoice
        </button>
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

      {showNew && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.6)' }} onClick={(e) => { if (e.target === e.currentTarget) setShowNew(false) }}>
          <div style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', borderRadius: '14px', padding: '28px', width: '100%', maxWidth: '440px', boxShadow: '0 24px 80px rgba(0,0,0,0.4)' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <h2 style={{ fontSize: '18px', fontFamily: 'var(--font-display)', color: 'var(--text)', margin: 0 }}>New Invoice</h2>
              <button onClick={() => setShowNew(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)' }}><Icon name="x" size={18} /></button>
            </div>
            {formError && <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>{formError}</div>}
            <form onSubmit={handleCreateInvoice} style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div>
                <label style={LABEL_STYLE}>Invoice Number *</label>
                <input required style={INPUT_STYLE} value={form.invoice_number} onChange={(e) => setForm((f) => ({ ...f, invoice_number: e.target.value }))} placeholder="INV-2026-001" />
              </div>
              <div>
                <label style={LABEL_STYLE}>Household</label>
                <select style={INPUT_STYLE} value={form.household_id} onChange={(e) => setForm((f) => ({ ...f, household_id: e.target.value }))}>
                  <option value="">No household</option>
                  {households.map((h) => <option key={h.id} value={h.id}>{h.household_name}</option>)}
                </select>
              </div>
              <div>
                <label style={LABEL_STYLE}>Due Date</label>
                <input type="date" style={INPUT_STYLE} value={form.due_date} onChange={(e) => setForm((f) => ({ ...f, due_date: e.target.value }))} />
              </div>
              <div>
                <label style={LABEL_STYLE}>Notes</label>
                <textarea style={{ ...INPUT_STYLE, resize: 'vertical' }} rows={3} value={form.notes} onChange={(e) => setForm((f) => ({ ...f, notes: e.target.value }))} placeholder="Optional notes…" />
              </div>
              <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '4px' }}>
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Creating…' : 'Create Invoice'}</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
