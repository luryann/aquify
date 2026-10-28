'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchHouseholds, fetchInvoicesForHousehold, createInvoice, type InvoiceRecord, type HouseholdRecord } from '@/lib/adminData'
import { formatDateOnly, parseBackendDate } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

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
  refunded_partial: { label: 'Partial refund', color: 'var(--accent)', bg: 'var(--accent-subtle)' },
  refunded_full: { label: 'Refunded', color: 'var(--accent)', bg: 'var(--accent-subtle)' },
  voided: { label: 'Voided', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  written_off: { label: 'Written off', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100)
}

function isOverdue(invoice: InvoiceRow) {
  if (!invoice.dueDate) return false
  if (['paid', 'refunded_full', 'voided', 'written_off'].includes(invoice.status)) return false
  return parseBackendDate(invoice.dueDate) < new Date()
}

const COLUMNS: Column<InvoiceRow>[] = [
  {
    key: 'invoiceNumber',
    label: 'Invoice',
    render: (row) => <Link href={`/invoices/${row.id}`} style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-base)', fontWeight: 700, color: 'var(--text)', textDecoration: 'none' }}>{row.invoiceNumber}</Link>,
  },
  {
    key: 'household',
    label: 'Household',
    sortable: true,
    render: (row) => <Link href={`/members/households/${row.householdId}`} style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}>{row.household}</Link>,
  },
  {
    key: 'description',
    label: 'Description',
    render: (row) => <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', maxWidth: '240px', display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{row.description ?? '—'}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => {
      const c = STATUS_CFG[row.status]
      return <div style={{ display: 'flex', gap: '4px', alignItems: 'center', flexWrap: 'wrap' }}>
        <span className="badge" style={{ background: c.bg, color: c.color }}>{c.label}</span>
        {isOverdue(row) && <span style={{ fontSize: 'var(--fs-2xs)', fontWeight: 700, padding: '2px 6px', borderRadius: '8px', background: 'var(--error-bg)', color: 'var(--error)', fontFamily: 'var(--font-body)' }}>OVERDUE</span>}
      </div>
    },
  },
  {
    key: 'totalCents',
    label: 'Amount',
    align: 'right',
    render: (row) => (
      <div style={{ textAlign: 'right' }}>
        <div style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-base)', fontWeight: 700, color: 'var(--text)' }}>{formatCents(row.totalCents)}</div>
        {row.paidCents > 0 && row.paidCents < row.totalCents && <div style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-xs)', color: 'var(--success)' }}>{formatCents(row.paidCents)} paid</div>}
      </div>
    ),
  },
  { key: 'issuedAt', label: 'Issued', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>{formatDateOnly(row.issuedAt)}</span> },
  { key: 'dueDate', label: 'Due', render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-sm)', color: isOverdue(row) ? 'var(--error)' : 'var(--text-muted)' }}>{formatDateOnly(row.dueDate)}</span> },
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

const INPUT_STYLE: React.CSSProperties = { width: '100%', padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 'var(--fs-base)', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' }
const LABEL_STYLE: React.CSSProperties = { fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }

export function InvoicesPanel() {
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
          setError(uiErrorMessage(err, 'Couldn’t load invoices. Try again.'))
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
      setFormError(uiErrorMessage(err, 'Couldn’t create invoice. Try again.'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <div className="toolbar">
        <div className="toolbar-search">
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search invoices or households…" value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 'var(--fs-base)', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 'var(--fs-base)', color: 'var(--text)', outline: 'none', cursor: 'pointer' }}>
          <option value="all">All statuses</option>
          {Object.entries(STATUS_CFG).map(([value, cfg]) => <option key={value} value={value}>{cfg.label}</option>)}
        </select>
        <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginLeft: 'auto' }}>{filtered.length} invoices</span>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setShowNew(true)}>
          <Icon name="plus" size={14} /> New Invoice
        </button>
      </div>

      {loading ? (
        <EmptyState icon="file-text" title="Loading invoices" description="Fetching invoice data." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load invoices" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No invoices found" emptyDescription="Issue an invoice to see it here." />
      )}

      {showNew && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.6)' }} onClick={(e) => { if (e.target === e.currentTarget) setShowNew(false) }}>
          <div style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', borderRadius: '12px', padding: '24px', width: '100%', maxWidth: '440px', boxShadow: '0 24px 80px rgba(0,0,0,0.4)' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <h2 className="section-title" style={{ margin: 0 }}>New invoice</h2>
              <button onClick={() => setShowNew(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)' }}><Icon name="x" size={18} /></button>
            </div>
            {formError && <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '12px 16px', marginBottom: '16px', fontSize: 'var(--fs-base)' }}>{formError}</div>}
            <form onSubmit={handleCreateInvoice} style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
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
                <label style={LABEL_STYLE}>Due date</label>
                <input type="date" style={INPUT_STYLE} value={form.due_date} onChange={(e) => setForm((f) => ({ ...f, due_date: e.target.value }))} />
              </div>
              <div>
                <label style={LABEL_STYLE}>Notes</label>
                <textarea style={{ ...INPUT_STYLE, resize: 'vertical' }} rows={3} value={form.notes} onChange={(e) => setForm((f) => ({ ...f, notes: e.target.value }))} placeholder="Optional notes…" />
              </div>
              <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end', marginTop: '4px' }}>
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
