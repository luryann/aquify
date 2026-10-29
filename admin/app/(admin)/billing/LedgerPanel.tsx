'use client'

import { useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchHouseholds, fetchInvoicesForHousehold, fetchLedgerEvents, type HouseholdRecord, type InvoiceRecord, type LedgerEventRecord } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

type LedgerEventRow = {
  id: string
  eventType: string
  invoiceId: string
  invoiceNumber: string
  householdName: string
  description: string
  creditCents: number
  debitCents: number
  actorName: string
  createdAt: string
}

function formatCents(cents: number) {
  if (cents === 0) return '—'
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100)
}

const COLUMNS: Column<LedgerEventRow>[] = [
  { key: 'createdAt', label: 'Timestamp', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>{new Date(row.createdAt).toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })}</span> },
  { key: 'eventType', label: 'Event', render: (row) => <span className="u-ff-font-body u-fs-fs-xs" style={{ color: 'var(--accent-text, var(--accent))' }}>{row.eventType}</span> },
  { key: 'invoiceNumber', label: 'Invoice', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text)' }}>{row.invoiceNumber || row.invoiceId || '—'}</span> },
  { key: 'householdName', label: 'Household', render: (row) => <span className="u-fs-fs-base" style={{ color: 'var(--text)' }}>{row.householdName || '—'}</span> },
  { key: 'description', label: 'Description', render: (row) => <span className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)', maxWidth: 320, display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{row.description || '—'}</span> },
  { key: 'creditCents', label: 'Billed', render: (row) => <span className="u-ff-font-body u-fs-fs-base u-fw-600" style={{ color: row.creditCents > 0 ? 'var(--success)' : 'var(--text-muted)', textAlign: 'right', display: 'block' }}>{row.creditCents > 0 ? formatCents(row.creditCents) : '—'}</span> },
  { key: 'debitCents', label: 'Paid or Removed', render: (row) => <span className="u-ff-font-body u-fs-fs-base u-fw-600" style={{ color: row.debitCents > 0 ? 'var(--error)' : 'var(--text-muted)', textAlign: 'right', display: 'block' }}>{row.debitCents > 0 ? formatCents(row.debitCents) : '—'}</span> },
]

async function loadLedgerRows(api: ReturnType<typeof useApiClient>, orgId: string): Promise<LedgerEventRow[]> {
  const [invoicesByHousehold, events] = await Promise.all([
    fetchHouseholds(api, orgId).then((households) => Promise.all(households.map(async (household) => ({
      household,
      invoices: await fetchInvoicesForHousehold(api, orgId, household.id).catch(() => []),
    })))),
    fetchLedgerEvents(api, orgId),
  ])

  const invoiceMap = new Map<string, InvoiceRecord>()
  const householdByInvoice = new Map<string, HouseholdRecord>()
  for (const { household, invoices } of invoicesByHousehold) {
    for (const invoice of invoices) {
      invoiceMap.set(invoice.id, invoice)
      householdByInvoice.set(invoice.id, household)
    }
  }

  return events.map((event: LedgerEventRecord) => {
    const invoice = event.invoice_id ? invoiceMap.get(event.invoice_id) : undefined
    const household = event.invoice_id ? householdByInvoice.get(event.invoice_id) : undefined
    const isCredit = ['invoice_issued', 'refund_processed'].includes(event.event_type)
    return {
      id: event.id,
      eventType: event.event_type,
      invoiceId: event.invoice_id || '',
      invoiceNumber: invoice?.invoice_number || '',
      householdName: household?.household_name || '',
      description: event.description || '',
      creditCents: isCredit ? Math.max(event.amount_cents, 0) : 0,
      debitCents: isCredit ? 0 : Math.max(event.amount_cents, 0),
      actorName: event.actor_id || 'system',
      createdAt: event.created_at,
    }
  }).sort((a, b) => b.createdAt.localeCompare(a.createdAt))
}

export function LedgerPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<LedgerEventRow[]>([])
  const [search, setSearch] = useState('')
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')
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
        const next = await loadLedgerRows(api, session.user.orgId)
        if (!cancelled) setRows(next)
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load ledger events. Try again.'))
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

  const query = search.toLowerCase()
  const filtered = useMemo(() => rows.filter((row) => {
    if (dateFrom && row.createdAt < dateFrom) return false
    if (dateTo && row.createdAt > `${dateTo}T23:59:59Z`) return false
    return !search || row.householdName.toLowerCase().includes(query) || row.invoiceNumber.toLowerCase().includes(query) || row.description.toLowerCase().includes(query)
  }), [rows, search, query, dateFrom, dateTo])

  const totalCredits = rows.reduce((sum, row) => sum + row.creditCents, 0)
  const totalDebits = rows.reduce((sum, row) => sum + row.debitCents, 0)

  return (
    <div>
      <div className="cols-3 u-gap-12px u-mb-20px" >
        {[
          { label: 'Transactions', value: rows.length.toString(), sub: 'For your club' },
          { label: 'Money billed', value: formatCents(totalCredits), sub: 'Invoices and refunds', accent: true },
          { label: 'Payments and write-offs', value: formatCents(totalDebits), sub: 'Payments, voided charges, and write-offs' },
        ].map((stat) => (
          <div key={stat.label} className="u-p-20px-24px u-gap-6px" style={{ borderRadius: 12, background: 'var(--bg-elevated)', border: '1px solid var(--border)', display: 'flex', flexDirection: 'column' }}>
            <span className="u-fs-fs-sm u-fw-600" style={{ color: 'var(--text-muted)' }}>{stat.label}</span>
            <span className="u-fs-fs-3xl u-fw-700 u-ff-font-body" style={{ letterSpacing: '-0.02em', color: stat.accent ? 'var(--accent)' : 'var(--text)' }}>{stat.value}</span>
            <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{stat.sub}</span>
          </div>
        ))}
      </div>

      <div className="toolbar">
        <div className="toolbar-search">
          <Icon name="search" size={14} style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input aria-label="Search household, invoice, description" type="text" placeholder="Search household, invoice, description…" value={search} onChange={(e) => setSearch(e.target.value)} className="u-p-8px-12px-8px-32px u-fs-fs-base" style={{ width: '100%' }} />
        </div>
        <input type="date" value={dateFrom} onChange={(e) => setDateFrom(e.target.value)} className="u-p-8px-12px u-fs-fs-base"  />
        <input type="date" value={dateTo} onChange={(e) => setDateTo(e.target.value)} className="u-p-8px-12px u-fs-fs-base"  />
        <span className="u-fs-fs-sm u-ff-font-body u-ml-auto" style={{ color: 'var(--text-muted)' }}>{filtered.length} events</span>
      </div>

      {loading ? (
        <EmptyState icon="book-open" title="Loading ledger events" description="Fetching ledger data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load ledger events" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={15} emptyTitle="No transactions found" emptyDescription="Try adjusting the filters or date range." />
      )}
    </div>
  )
}
