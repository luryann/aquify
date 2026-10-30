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
import { Modal } from '@/components/ui/Modal'
import { formatCents } from '@/lib/financeData'
import { PageSkeleton } from '@/components/ui/PageSkeleton'

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
  refunded_partial: { label: 'Partial refund', color: 'var(--accent-text, var(--accent))', bg: 'var(--accent-subtle)' },
  refunded_full: { label: 'Refunded', color: 'var(--accent-text, var(--accent))', bg: 'var(--accent-subtle)' },
  voided: { label: 'Voided', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  written_off: { label: 'Written off', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
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
    render: (row) => <Link href={`/invoices/${row.id}`} className="u-ff-font-body u-fs-fs-base u-fw-700" style={{ color: 'var(--text)', textDecoration: 'none' }}>{row.invoiceNumber}</Link>,
  },
  {
    key: 'household',
    label: 'Household',
    sortable: true,
    render: (row) => <Link href={`/members/households/${row.householdId}`} className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)', textDecoration: 'none' }}>{row.household}</Link>,
  },
  {
    key: 'description',
    label: 'Description',
    render: (row) => <span className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)', maxWidth: '240px', display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{row.description ?? '—'}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => {
      const c = STATUS_CFG[row.status]
      return <div className="u-gap-4px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
        <span className="badge" style={{ background: c.bg, color: c.color }}>{c.label}</span>
        {isOverdue(row) && <span className="u-fs-fs-2xs u-fw-700 u-p-2px-6px u-ff-font-body" style={{ borderRadius: '8px', background: 'var(--error-bg)', color: 'var(--error)' }}>OVERDUE</span>}
      </div>
    },
  },
  {
    key: 'totalCents',
    label: 'Amount',
    align: 'right',
    render: (row) => (
      <div style={{ textAlign: 'right' }}>
        <div className="u-ff-font-body u-fs-fs-base u-fw-700" style={{ color: 'var(--text)' }}>{formatCents(row.totalCents)}</div>
        {row.paidCents > 0 && row.paidCents < row.totalCents && <div className="u-ff-font-body u-fs-fs-xs" style={{ color: 'var(--success)' }}>{formatCents(row.paidCents)} paid</div>}
      </div>
    ),
  },
  { key: 'issuedAt', label: 'Issued', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{formatDateOnly(row.issuedAt)}</span> },
  { key: 'dueDate', label: 'Due', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: isOverdue(row) ? 'var(--error)' : 'var(--text-muted)' }}>{formatDateOnly(row.dueDate)}</span> },
]

async function loadInvoices(api: ReturnType<typeof useApiClient>, orgId: string): Promise<{ rows: InvoiceRow[]; households: HouseholdRecord[] }> {
  const households = await fetchHouseholds(api, orgId)
  const invoicesByHousehold = await Promise.all(households.map(async (household) => ({
    household,
    invoices: await fetchInvoicesForHousehold(api, orgId, household.id).catch(() => []),
  })))

  const rows = invoicesByHousehold.flatMap(({ household, invoices }) => invoices.map((invoice: InvoiceRecord) => ({
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
  return { rows, households }
}

const INPUT_STYLE: React.CSSProperties = { width: '100%' }

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
        const next = await loadInvoices(api, session.user.orgId)
        if (!cancelled) { setRows(next.rows); setHouseholds(next.households) }
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
      const next = await loadInvoices(api, session.user.orgId)
      setRows(next.rows); setHouseholds(next.households)
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
          <input aria-label="Search invoices or households" type="text" placeholder="Search invoices or households…" value={search} onChange={(e) => setSearch(e.target.value)} className="u-p-8px-12px-8px-32px u-fs-fs-base" style={{ width: '100%' }} />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="u-p-8px-12px u-fs-fs-base" >
          <option value="all">All statuses</option>
          {Object.entries(STATUS_CFG).map(([value, cfg]) => <option key={value} value={value}>{cfg.label}</option>)}
        </select>
        <span className="u-fs-fs-sm u-ff-font-body u-ml-auto" style={{ color: 'var(--text-muted)' }}>{filtered.length} invoices</span>
        <button type="button" className="btn btn-primary u-gap-6px" style={{ display: 'flex', alignItems: 'center' }} onClick={() => setShowNew(true)}>
          <Icon name="plus" size={14} /> New invoice
        </button>
      </div>

      {loading ? (
        <PageSkeleton variant="table" noHeader rows={4} />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load invoices" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No invoices found" emptyDescription="Issue an invoice to see it here." />
      )}

      <Modal open={Boolean(showNew)} onClose={() => setShowNew(false)} title="New invoice" width={440}>
            {formError && <div role="alert" className="form-banner-error u-mb-16px">{formError}</div>}
            <form onSubmit={handleCreateInvoice} className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div>
                <label className="form-label form-label-required">Invoice number</label>
                <input aria-label="INV-2026-001" required style={INPUT_STYLE} value={form.invoice_number} onChange={(e) => setForm((f) => ({ ...f, invoice_number: e.target.value }))} placeholder="INV-2026-001" />
              </div>
              <div>
                <label className="form-label">Household</label>
                <select style={INPUT_STYLE} value={form.household_id} onChange={(e) => setForm((f) => ({ ...f, household_id: e.target.value }))}>
                  <option value="">No household</option>
                  {households.map((h) => <option key={h.id} value={h.id}>{h.household_name}</option>)}
                </select>
              </div>
              <div>
                <label className="form-label">Due date</label>
                <input type="date" style={INPUT_STYLE} value={form.due_date} onChange={(e) => setForm((f) => ({ ...f, due_date: e.target.value }))} />
              </div>
              <div>
                <label className="form-label">Notes</label>
                <textarea aria-label="Optional notes" style={{ ...INPUT_STYLE, resize: 'vertical' }} rows={3} value={form.notes} onChange={(e) => setForm((f) => ({ ...f, notes: e.target.value }))} placeholder="Optional notes…" />
              </div>
              <div className="form-actions">
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Creating…' : 'Create invoice'}</button>
              </div>
            </form>
      </Modal>
    </div>
  )
}
