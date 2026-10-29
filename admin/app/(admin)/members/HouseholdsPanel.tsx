'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchHouseholds, fetchSwimmers, createHousehold, deleteHousehold, type HouseholdRecord, type SwimmerRecord } from '@/lib/adminData'
import { normalizeHouseholdStatus, type HouseholdStatus } from '@/lib/household-status'
import { canAccess } from '@/lib/roles'
import { formatDateOnly } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Modal } from '@/components/ui/Modal'

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

function buildColumns(onDelete: (row: HouseholdRow) => void, canDelete: boolean): Column<HouseholdRow>[] {
  return [
    {
      key: 'name',
      label: 'Household',
      sortable: true,
      render: (row) => (
        <div>
          <Link href={`/members/households/${row.id}`} className="u-fw-600 u-fs-fs-base" style={{ color: 'var(--text)', textDecoration: 'none' }}>
            {row.name}
          </Link>
        </div>
      ),
    },
    {
      key: 'contact',
      label: 'Contact',
      sortable: true,
      render: (row) => (
        <div>
          <div className="u-fs-fs-base" style={{ color: 'var(--text)' }}>{row.contact}</div>
          <div className="u-fs-fs-xs u-mt-2px" style={{ color: 'var(--text-muted)' }}>{row.email || 'No email on file'}</div>
        </div>
      ),
    },
    {
      key: 'swimmerCount',
      label: 'Swimmers',
      sortable: true,
      align: 'right',
      render: (row) => <span className="u-ff-font-body u-fs-fs-base" style={{ color: 'var(--text)' }}>{row.swimmerCount}</span>,
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
      render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{formatDateOnly(row.joinedAt)}</span>,
    },
    {
      key: 'id',
      label: '',
      width: '120px',
      render: (row) => (
        <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center' }}>
          <Link href={`/members/households/${row.id}`} className="u-fs-fs-sm u-gap-4px" style={{ color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center' }}>
            View <Icon name="chevron-right" size={12} />
          </Link>
          {canDelete && (
            <button
              onClick={(e) => { e.stopPropagation(); onDelete(row) }}
              className="u-p-2px-4px" style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--error)', display: 'flex', alignItems: 'center', borderRadius: '4px' }}
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

async function loadHouseholdRows(api: ReturnType<typeof useApiClient>, orgId: string) {
  const [households, swimmers] = await Promise.all([fetchHouseholds(api, orgId), fetchSwimmers(api, orgId)])
  return deriveHouseholdRows(households, swimmers)
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

const INPUT_STYLE: React.CSSProperties = { width: '100%' }
const EMPTY_FORM = { household_name: '', primary_email: '', primary_phone_e164: '', status: 'active', notes: '' }

export function HouseholdsPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const canDelete = canAccess(session?.user.roles ?? [], ['admin'])
  const [rows, setRows] = useState<HouseholdRow[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // New Household modal
  const [showNew, setShowNew] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [form, setForm] = useState(EMPTY_FORM)

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
        const nextRows = await loadHouseholdRows(api, session.user.orgId)
        if (!cancelled) setRows(nextRows)
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load households. Try again.'))
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

  const columns = useMemo(() => buildColumns((row) => { setDeleteTarget(row); setDeleteError(null) }, canDelete), [canDelete])

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
      setForm(EMPTY_FORM)
      setRows(await loadHouseholdRows(api, session.user.orgId))
    } catch (err) {
      setFormError(uiErrorMessage(err, 'Couldn’t create household. Try again.'))
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
      setRows(await loadHouseholdRows(api, session.user.orgId))
    } catch (err) {
      setDeleteError(uiErrorMessage(err, 'Couldn’t delete household. Try again.'))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div>
      <div className="u-mb-16px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <button className="btn btn-primary u-gap-6px" style={{ display: 'flex', alignItems: 'center' }} onClick={() => setShowNew(true)}>
          <Icon name="plus" size={14} />
          New household
        </button>
      </div>

      <div className="cols-4 u-gap-12px u-mb-20px" >
        {[
          { label: 'Total', value: counts.total, color: 'var(--text)' },
          { label: 'Active', value: counts.active, color: 'var(--success)' },
          { label: 'Inactive', value: counts.inactive, color: 'var(--text-muted)' },
          { label: 'Archived', value: counts.archived, color: 'var(--text-subtle)' },
        ].map((stat) => (
          <div key={stat.label} className="card u-p-16px-16px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</div>
            <div className="u-fs-fs-2xl u-ff-font-body u-fw-700" style={{ color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <div className="toolbar">
        <div className="toolbar-search">
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input aria-label="Search households or contacts..." type="text" placeholder="Search households or contacts..." value={search} onChange={(e) => setSearch(e.target.value)} className="u-p-8px-12px-8px-32px u-fs-fs-base" style={{ width: '100%' }} />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="u-p-8px-12px u-fs-fs-base" >
          <option value="all">All statuses</option>
          <option value="active">Active</option>
          <option value="inactive">Inactive</option>
          <option value="archived">Archived</option>
        </select>
        <span className="u-fs-fs-sm u-ff-font-body u-ml-auto" style={{ color: 'var(--text-muted)' }}>{filtered.length} households</span>
      </div>

      {loading ? (
        <EmptyState icon="users" title="Loading households" description="Fetching live household data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load households" description={error} />
      ) : (
        <Table columns={columns} data={filtered} pageSize={10} emptyTitle="No households found" emptyDescription="Try adjusting your search or filters." />
      )}

      {/* Delete Confirmation Modal */}
      <Modal open={Boolean(deleteTarget)} onClose={() => setDeleteTarget(null)} title="Delete household" description="This action cannot be undone." width={440}>
<p className="u-fs-fs-base u-mb-20px u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>
              Delete <strong style={{ color: 'var(--text)' }}>{deleteTarget?.name}</strong>? Its swimmers, registrations and invoices are archived with it.
            </p>
{deleteError && <div role="alert" className="form-banner-error u-mb-16px">{deleteError}</div>}
<div className="u-gap-12px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button className="btn" onClick={() => setDeleteTarget(null)} disabled={deleting}>Cancel</button>
              <button
                className="btn"
                style={{ background: 'var(--error)', color: '#fff', border: 'none' }}
                onClick={handleDelete}
                disabled={deleting}
              >
                {deleting ? 'Deleting…' : 'Delete household'}
              </button>
            </div>
</Modal>

      {/* New Household Modal */}
      <Modal open={Boolean(showNew)} onClose={() => setShowNew(false)} title="New household" width={480}>
            {formError && <div role="alert" className="form-banner-error u-mb-16px">{formError}</div>}
            <form onSubmit={handleCreateHousehold} className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div>
                <label className="form-label form-label-required">Household name</label>
                <input aria-label="Smith Family" required style={INPUT_STYLE} value={form.household_name} onChange={(e) => setForm((f) => ({ ...f, household_name: e.target.value }))} placeholder="Smith Family" />
              </div>
              <div className="form-grid">
                <div>
                  <label className="form-label">Primary email</label>
                  <input aria-label="parent@example.com" type="email" value={form.primary_email} onChange={(e) => setForm((f) => ({ ...f, primary_email: e.target.value }))} placeholder="parent@example.com" autoComplete="off" />
                </div>
                <div>
                  <label className="form-label">Phone</label>
                  <input aria-label="+15555550123" type="tel" value={form.primary_phone_e164} onChange={(e) => setForm((f) => ({ ...f, primary_phone_e164: e.target.value }))} placeholder="+15555550123" autoComplete="off" />
                  <p className="field-hint">Include the country code, like +1.</p>
                </div>
              </div>
              <div>
                <label className="form-label">Status</label>
                <select style={INPUT_STYLE} value={form.status} onChange={(e) => setForm((f) => ({ ...f, status: e.target.value }))}>
                  <option value="active">Active</option>
                  <option value="inactive">Inactive</option>
                  <option value="archived">Archived</option>
                </select>
              </div>
              <div>
                <label className="form-label">Notes</label>
                <textarea aria-label="Anything staff should know" style={{ ...INPUT_STYLE, resize: 'vertical' }} rows={3} value={form.notes} onChange={(e) => setForm((f) => ({ ...f, notes: e.target.value }))} placeholder="Anything staff should know" />
              </div>
              <div className="form-actions">
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Creating…' : 'Create household'}</button>
              </div>
            </form>
      </Modal>
    </div>
  )
}
