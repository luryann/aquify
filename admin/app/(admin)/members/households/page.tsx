'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchHouseholds, fetchSwimmers, createHousehold, deleteHousehold, type HouseholdRecord, type SwimmerRecord } from '@/lib/adminData'
import { normalizeHouseholdStatus, type HouseholdStatus } from '@/lib/household-status'
import { canWrite } from '@/lib/roles'

type HouseholdRow = {
  id: string
  name: string
  contact: string
  email: string
  swimmerCount: number
  status: HouseholdStatus
  joinedAt: string
}

const STATUS_CONFIG: Record<HouseholdStatus, { label: string; color: string; bg: string }> = {
  active: { label: 'Active', color: 'var(--success)', bg: 'var(--success-bg)' },
  inactive: { label: 'Inactive', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  archived: { label: 'Archived', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

function StatusBadge({ status }: { status: HouseholdStatus }) {
  const cfg = STATUS_CONFIG[status]
  return (
    <span className="badge" style={{ background: cfg.bg, color: cfg.color }}>
      {cfg.label}
    </span>
  )
}

function buildColumns(onDelete: (row: HouseholdRow) => void, allowWrite: boolean): Column<HouseholdRow>[] {
  return [
    {
      key: 'name',
      label: 'Household',
      sortable: true,
      render: (row) => (
        <div>
          <Link href={`/members/households/${row.id}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}>
            {row.name}
          </Link>
          <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px', fontFamily: 'var(--font-body)' }}>{row.id}</div>
        </div>
      ),
    },
    {
      key: 'contact',
      label: 'Contact',
      sortable: true,
      render: (row) => (
        <div>
          <div style={{ fontSize: '13px', color: 'var(--text)' }}>{row.contact}</div>
          <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>{row.email || 'No email on file'}</div>
        </div>
      ),
    },
    {
      key: 'swimmerCount',
      label: 'Swimmers',
      sortable: true,
      align: 'right',
      render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '13px', color: 'var(--text)' }}>{row.swimmerCount}</span>,
    },
    {
      key: 'status',
      label: 'Status',
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      key: 'joinedAt',
      label: 'Created',
      sortable: true,
      render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--text-subtle)' }}>{new Date(row.joinedAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}</span>,
    },
    {
      key: 'id',
      label: '',
      width: '120px',
      render: (row) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Link href={`/members/households/${row.id}`} style={{ fontSize: '12px', color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px' }}>
            View <Icon name="chevron-right" size={12} />
          </Link>
          {allowWrite && (
            <button
              onClick={(e) => { e.stopPropagation(); onDelete(row) }}
              style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--error)', padding: '2px 4px', display: 'flex', alignItems: 'center', borderRadius: '4px' }}
              title="Delete household"
            >
              <Icon name="trash-2" size={13} />
            </button>
          )}
        </div>
      ),
    },
  ]
}

function deriveHouseholdRows(households: HouseholdRecord[], swimmers: SwimmerRecord[]): HouseholdRow[] {
  const swimmerCountByHousehold = swimmers.reduce<Record<string, number>>((acc, swimmer) => {
    acc[swimmer.household_id] = (acc[swimmer.household_id] ?? 0) + 1
    return acc
  }, {})

  return households
    .map((household) => ({
      id: household.id,
      name: household.household_name,
      contact: household.primary_email || household.primary_phone_e164 || household.household_name,
      email: household.primary_email || '',
      swimmerCount: swimmerCountByHousehold[household.id] ?? 0,
      status: normalizeHouseholdStatus(household.status),
      joinedAt: household.created_at,
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

const INPUT_STYLE: React.CSSProperties = { width: '100%', padding: '9px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' }
const LABEL_STYLE: React.CSSProperties = { fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '5px' }

export default function HouseholdsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const allowWrite = canWrite(session?.user.roles ?? [])
  const [rows, setRows] = useState<HouseholdRow[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // New Household modal
  const [showNew, setShowNew] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [form, setForm] = useState({ household_name: '', primary_email: '', primary_phone_e164: '', status: 'active', notes: '' })

  // Delete confirmation
  const [deleteTarget, setDeleteTarget] = useState<HouseholdRow | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!session?.user.orgId) {
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        setError(null)
        const [households, swimmers] = await Promise.all([
          fetchHouseholds(api, session.user.orgId),
          fetchSwimmers(api, session.user.orgId),
        ])
        if (!cancelled) setRows(deriveHouseholdRows(households, swimmers))
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load households.')
          setRows([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId])

  const filtered = useMemo(() => {
    return rows.filter((household) => {
      const query = search.toLowerCase()
      const matchesSearch = !search || household.name.toLowerCase().includes(query) || household.contact.toLowerCase().includes(query) || household.email.toLowerCase().includes(query)
      const matchesStatus = statusFilter === 'all' || household.status === statusFilter
      return matchesSearch && matchesStatus
    })
  }, [rows, search, statusFilter])

  const columns = useMemo(() => buildColumns((row) => { setDeleteTarget(row); setDeleteError(null) }, allowWrite), [allowWrite])

  const counts = useMemo(() => ({
    total: rows.length,
    active: rows.filter((row) => row.status === 'active').length,
    inactive: rows.filter((row) => row.status === 'inactive').length,
    archived: rows.filter((row) => row.status === 'archived').length,
  }), [rows])

  async function handleCreateHousehold(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId) return
    setSubmitting(true)
    setFormError(null)
    try {
      await createHousehold(api, session.user.orgId, form)
      setShowNew(false)
      setForm({ household_name: '', primary_email: '', primary_phone_e164: '', status: 'active', notes: '' })
      // Reload
      const [households, swimmers] = await Promise.all([fetchHouseholds(api, session.user.orgId), fetchSwimmers(api, session.user.orgId)])
      setRows(deriveHouseholdRows(households, swimmers))
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : 'Failed to create household.')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleDelete() {
    if (!deleteTarget || !session?.user.orgId) return
    setDeleting(true)
    setDeleteError(null)
    try {
      await deleteHousehold(api, session.user.orgId, deleteTarget.id)
      setDeleteTarget(null)
      const [households, swimmers] = await Promise.all([fetchHouseholds(api, session.user.orgId), fetchSwimmers(api, session.user.orgId)])
      setRows(deriveHouseholdRows(households, swimmers))
    } catch (err) {
      setDeleteError(err instanceof ApiError ? err.message : 'Failed to delete household.')
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Households</h1>
          <p className="page-subtitle">Live household roster and membership account records</p>
        </div>
        {allowWrite && (
          <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setShowNew(true)}>
            <Icon name="plus" size={14} />
            New Household
          </button>
        )}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Total', value: counts.total, color: 'var(--text)' },
          { label: 'Active', value: counts.active, color: 'var(--success)' },
          { label: 'Inactive', value: counts.inactive, color: 'var(--text-muted)' },
          { label: 'Archived', value: counts.archived, color: 'var(--text-subtle)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-body)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-body)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '360px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search households or contacts..." value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', cursor: 'pointer' }}>
          <option value="all">All Statuses</option>
          <option value="active">Active</option>
          <option value="inactive">Inactive</option>
          <option value="archived">Archived</option>
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginLeft: 'auto' }}>{filtered.length} households</span>
      </div>

      {loading ? (
        <EmptyState icon="users" title="Loading households" description="Fetching live household data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load households" description={error} />
      ) : (
        <Table columns={columns} data={filtered} pageSize={10} emptyTitle="No households found" emptyDescription="Try adjusting your search or filters." />
      )}

      {/* Delete Confirmation Modal */}
      {deleteTarget && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.6)' }} onClick={(e) => { if (e.target === e.currentTarget) setDeleteTarget(null) }}>
          <div style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', borderRadius: '14px', padding: '28px', width: '100%', maxWidth: '440px', boxShadow: '0 24px 80px rgba(0,0,0,0.4)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '16px' }}>
              <div style={{ width: '40px', height: '40px', borderRadius: '10px', background: 'var(--error-bg)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                <Icon name="trash-2" size={18} style={{ color: 'var(--error)' }} />
              </div>
              <div>
                <h2 style={{ fontSize: '16px', fontFamily: 'var(--font-display)', color: 'var(--text)', margin: 0, marginBottom: '2px' }}>Delete Household</h2>
                <p style={{ fontSize: '12px', color: 'var(--text-muted)', margin: 0 }}>This action cannot be undone.</p>
              </div>
            </div>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', marginBottom: '20px', lineHeight: 1.5 }}>
              Are you sure you want to delete <strong style={{ color: 'var(--text)' }}>{deleteTarget.name}</strong>? All associated data will be soft-deleted.
            </p>
            {deleteError && <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>{deleteError}</div>}
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
              <button className="btn" onClick={() => setDeleteTarget(null)} disabled={deleting}>Cancel</button>
              <button
                className="btn"
                style={{ background: 'var(--error)', color: '#fff', border: 'none' }}
                onClick={handleDelete}
                disabled={deleting}
              >
                {deleting ? 'Deleting…' : 'Delete Household'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* New Household Modal */}
      {showNew && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.6)' }} onClick={(e) => { if (e.target === e.currentTarget) setShowNew(false) }}>
          <div style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', borderRadius: '14px', padding: '28px', width: '100%', maxWidth: '480px', boxShadow: '0 24px 80px rgba(0,0,0,0.4)' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <h2 style={{ fontSize: '18px', fontFamily: 'var(--font-display)', color: 'var(--text)', margin: 0 }}>New Household</h2>
              <button onClick={() => setShowNew(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)' }}><Icon name="x" size={18} /></button>
            </div>
            {formError && <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>{formError}</div>}
            <form onSubmit={handleCreateHousehold} style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div>
                <label style={LABEL_STYLE}>Household Name *</label>
                <input required style={INPUT_STYLE} value={form.household_name} onChange={(e) => setForm((f) => ({ ...f, household_name: e.target.value }))} placeholder="Smith Family" />
              </div>
              <div>
                <label style={LABEL_STYLE}>Primary Email</label>
                <input type="email" style={INPUT_STYLE} value={form.primary_email} onChange={(e) => setForm((f) => ({ ...f, primary_email: e.target.value }))} placeholder="parent@example.com" />
              </div>
              <div>
                <label style={LABEL_STYLE}>Phone (E.164)</label>
                <input style={INPUT_STYLE} value={form.primary_phone_e164} onChange={(e) => setForm((f) => ({ ...f, primary_phone_e164: e.target.value }))} placeholder="+15555555555" />
              </div>
              <div>
                <label style={LABEL_STYLE}>Status</label>
                <select style={INPUT_STYLE} value={form.status} onChange={(e) => setForm((f) => ({ ...f, status: e.target.value }))}>
                  <option value="active">Active</option>
                  <option value="inactive">Inactive</option>
                  <option value="archived">Archived</option>
                </select>
              </div>
              <div>
                <label style={LABEL_STYLE}>Notes</label>
                <textarea style={{ ...INPUT_STYLE, resize: 'vertical' }} rows={3} value={form.notes} onChange={(e) => setForm((f) => ({ ...f, notes: e.target.value }))} placeholder="Optional notes..." />
              </div>
              <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '4px' }}>
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Creating…' : 'Create Household'}</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
