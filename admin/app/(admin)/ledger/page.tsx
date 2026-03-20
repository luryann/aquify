'use client'

import { useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchHouseholds, fetchInvoicesForHousehold, fetchLedgerEvents, type HouseholdRecord, type InvoiceRecord, type LedgerEventRecord } from '@/lib/adminData'

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
  { key: 'createdAt', label: 'Timestamp', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>{new Date(row.createdAt).toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })}</span> },
  { key: 'eventType', label: 'Event', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--accent)' }}>{row.eventType}</span> },
  { key: 'invoiceNumber', label: 'Invoice', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text)' }}>{row.invoiceNumber || row.invoiceId || '—'}</span> },
  { key: 'householdName', label: 'Household', render: (row) => <span style={{ fontSize: 13, color: 'var(--text)' }}>{row.householdName || '—'}</span> },
  { key: 'description', label: 'Description', render: (row) => <span style={{ fontSize: 12, color: 'var(--text-subtle)', maxWidth: 320, display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{row.description || '—'}</span> },
  { key: 'creditCents', label: 'Credit', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 600, color: row.creditCents > 0 ? 'var(--success)' : 'var(--text-disabled)', textAlign: 'right', display: 'block' }}>{row.creditCents > 0 ? formatCents(row.creditCents) : '—'}</span> },
  { key: 'debitCents', label: 'Debit', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 600, color: row.debitCents > 0 ? 'var(--error)' : 'var(--text-disabled)', textAlign: 'right', display: 'block' }}>{row.debitCents > 0 ? formatCents(row.debitCents) : '—'}</span> },
]

async function loadLedgerRows(api: ReturnType<typeof useApiClient>, orgId: string): Promise<LedgerEventRow[]> {
  const households = await fetchHouseholds(api, orgId)
  const invoicesByHousehold = await Promise.all(households.map(async (household) => ({
    household,
    invoices: await fetchInvoicesForHousehold(api, orgId, household.id).catch(() => []),
  })))
  const events = await fetchLedgerEvents(api, orgId)

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

export default function LedgerPage() {
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
          setError(err instanceof ApiError ? err.message : 'Failed to load ledger events.')
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
    if (dateFrom && row.createdAt < dateFrom) return false
    if (dateTo && row.createdAt > `${dateTo}T23:59:59Z`) return false
    return !search || row.householdName.toLowerCase().includes(query) || row.invoiceNumber.toLowerCase().includes(query) || row.description.toLowerCase().includes(query)
  }), [rows, search, dateFrom, dateTo])

  const totalCredits = rows.reduce((sum, row) => sum + row.creditCents, 0)
  const totalDebits = rows.reduce((sum, row) => sum + row.debitCents, 0)

  return (
    <div style={{ padding: '32px 36px', maxWidth: 1400, margin: '0 auto' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 28 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Icon name="book-open" size={16} color="var(--accent)" />
            </div>
            <h1 style={{ fontSize: 22, fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--text)', margin: 0 }}>Billing Ledger</h1>
          </div>
          <p style={{ fontSize: 13, color: 'var(--text-muted)', margin: 0 }}>Append-only financial event log backed by the live billing service</p>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12, marginBottom: 20 }}>
        {[
          { label: 'Events', value: rows.length.toString(), sub: 'Current org scope' },
          { label: 'Credits', value: formatCents(totalCredits), sub: 'Invoice and refund postings', accent: true },
          { label: 'Debits', value: formatCents(totalDebits), sub: 'Payments, voids, and write-offs' },
        ].map((stat) => (
          <div key={stat.label} style={{ padding: '20px 24px', borderRadius: 10, background: 'var(--bg-elevated)', border: '1px solid var(--border)', display: 'flex', flexDirection: 'column', gap: 6 }}>
            <span style={{ fontSize: 11, fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-muted)' }}>{stat.label}</span>
            <span style={{ fontSize: 26, fontWeight: 700, letterSpacing: '-0.02em', fontFamily: 'var(--font-mono)', color: stat.accent ? 'var(--accent)' : 'var(--text)' }}>{stat.value}</span>
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{stat.sub}</span>
          </div>
        ))}
      </div>

      <div style={{ display: 'flex', gap: 10, marginBottom: 16, alignItems: 'center', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: 1, minWidth: 260 }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search household, invoice, description…" value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 13, color: 'var(--text)', outline: 'none' }} />
        </div>
        <input type="date" value={dateFrom} onChange={(e) => setDateFrom(e.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 13, color: 'var(--text)', outline: 'none' }} />
        <input type="date" value={dateTo} onChange={(e) => setDateTo(e.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 13, color: 'var(--text)', outline: 'none' }} />
        <span style={{ fontSize: 12, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>{filtered.length} events</span>
      </div>

      {loading ? (
        <EmptyState icon="book-open" title="Loading ledger events" description="Fetching live ledger data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load ledger events" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={15} emptyTitle="No ledger events found" emptyDescription="Try adjusting the filters or date range." />
      )}
    </div>
  )
}
