'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchGroups, fetchRegistrations, fetchStaff, createGroup, type GroupRecord, type RegistrationRecord, type StaffRecord } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Modal } from '@/components/ui/Modal'

function formatAgeRange(minAge: number | null | undefined, maxAge: number | null | undefined) {
  if (minAge == null && maxAge == null) return 'All ages'
  if (minAge == null) return `Up to ${maxAge}`
  if (maxAge == null) return `${minAge}+`
  return `${minAge}-${maxAge}`
}

type GroupType = 'competitive' | 'recreational' | 'developmental' | 'masters' | 'other'

type GroupRow = {
  id: string
  name: string
  type: GroupType
  swimmerCount: number
  coachName: string | null
  minAge: number | null
  maxAge: number | null
  status: 'active' | 'archived'
  createdAt: string
}

const TYPE_CONFIG: Record<GroupType, { label: string; color: string; bg: string }> = {
  competitive: { label: 'Competitive', color: 'var(--accent-text, var(--accent))', bg: 'var(--accent-subtle)' },
  recreational: { label: 'Recreational', color: 'var(--info)', bg: 'var(--info-bg)' },
  developmental: { label: 'Developmental', color: 'var(--warning)', bg: 'var(--warning-bg)' },
  masters: { label: 'Masters', color: 'var(--success)', bg: 'var(--success-bg)' },
  other: { label: 'Other', color: 'var(--text-subtle)', bg: 'var(--bg-elevated)' },
}

function TypeBadge({ type }: { type: GroupType }) {
  const c = TYPE_CONFIG[type]
  return <span className="badge" style={{ background: c.bg, color: c.color }}>{c.label}</span>
}

const COLUMNS: Column<GroupRow>[] = [
  {
    key: 'name',
    label: 'Group',
    sortable: true,
    render: (row) => (
      <div>
        <Link href={`/members/groups/${row.id}`} className="u-fw-600 u-fs-fs-base" style={{ color: 'var(--text)', textDecoration: 'none' }}>{row.name}</Link>
      </div>
    ),
  },
  {
    key: 'type',
    label: 'Type',
    render: (row) => <TypeBadge type={row.type} />,
  },
  {
    key: 'swimmerCount',
    label: 'Swimmers',
    sortable: true,
    align: 'right',
    render: (row) => <span className="u-ff-font-body u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{row.swimmerCount}</span>,
  },
  {
    key: 'coachName',
    label: 'Coach',
    render: (row) => <span className="u-fs-fs-base" style={{ color: row.coachName ? 'var(--text-subtle)' : 'var(--text-muted)' }}>{row.coachName ?? 'Unassigned'}</span>,
  },
  {
    key: 'minAge',
    label: 'Age range',
    render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{formatAgeRange(row.minAge, row.maxAge)}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => <span className="badge" style={{ background: row.status === 'active' ? 'var(--success-bg)' : 'var(--bg-elevated)', color: row.status === 'active' ? 'var(--success)' : 'var(--text-muted)' }}>{row.status}</span>,
  },
]

function deriveRows(groups: GroupRecord[], registrations: RegistrationRecord[], userNameById: Record<string, string>): GroupRow[] {
  const registrationCountByGroup = registrations.reduce<Record<string, number>>((acc, registration) => {
    if (registration.status !== 'withdrawn' && registration.status !== 'rejected') {
      acc[registration.group_id] = (acc[registration.group_id] ?? 0) + 1
    }
    return acc
  }, {})

  return groups
    .map((group) => ({
      id: group.id,
      name: group.name,
      type: (group.type as GroupType) || 'other',
      swimmerCount: registrationCountByGroup[group.id] ?? 0,
      coachName: group.coach_user_id ? (userNameById[group.coach_user_id] ?? group.coach_user_id) : null,
      minAge: group.min_age ?? null,
      maxAge: group.max_age ?? null,
      status: (group.is_active ? 'active' : 'archived') as 'active' | 'archived',
      createdAt: group.created_at,
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

const INPUT_STYLE: React.CSSProperties = { width: '100%' }

export function GroupsPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<GroupRow[]>([])
  const [userNameById, setUserNameById] = useState<Record<string, string>>({})
  const [staff, setStaff] = useState<StaffRecord[]>([])
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('active')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // New Group modal
  const [showNew, setShowNew] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [form, setForm] = useState({ code: '', name: '', description: '', type: 'competitive', min_age: '', max_age: '', coach_user_id: '' })

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
        const [groups, registrations, users] = await Promise.all([
          fetchGroups(api, session.user.orgId),
          fetchRegistrations(api, session.user.orgId),
          fetchStaff(api, session.user.orgId).catch(() => [] as StaffRecord[]),
        ])
        const userNameById = Object.fromEntries(users.map((u) => [u.id, u.name]))
        if (!cancelled) {
          setUserNameById(userNameById)
          setStaff(users)
          setRows(deriveRows(groups, registrations, userNameById))
        }
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load groups. Try again.'))
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

  const filtered = useMemo(() => rows.filter((group) => {
    const matchesSearch = !search || group.name.toLowerCase().includes(search.toLowerCase()) || (group.coachName ?? '').toLowerCase().includes(search.toLowerCase())
    const matchesType = typeFilter === 'all' || group.type === typeFilter
    const matchesStatus = statusFilter === 'all' || group.status === statusFilter
    return matchesSearch && matchesType && matchesStatus
  }), [rows, search, typeFilter, statusFilter])

  const totalRegistrations = rows.filter((row) => row.status === 'active').reduce((acc, row) => acc + row.swimmerCount, 0)

  const selectStyle: React.CSSProperties = {}

  async function handleCreateGroup(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId) return
    setSubmitting(true)
    setFormError(null)
    try {
      await createGroup(api, session.user.orgId, {
        code: form.code,
        name: form.name,
        description: form.description,
        type: form.type,
        min_age: form.min_age ? parseInt(form.min_age) : undefined,
        max_age: form.max_age ? parseInt(form.max_age) : undefined,
        coach_user_id: form.coach_user_id || undefined,
      })
      setShowNew(false)
      setForm({ code: '', name: '', description: '', type: 'competitive', min_age: '', max_age: '', coach_user_id: '' })
      const [grps, regs] = await Promise.all([fetchGroups(api, session.user.orgId), fetchRegistrations(api, session.user.orgId)])
      setRows(deriveRows(grps, regs, userNameById))
    } catch (err) {
      setFormError(uiErrorMessage(err, 'Couldn’t create group. Try again.'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <div className="u-mb-16px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <button type="button" className="btn btn-primary u-gap-6px" style={{ display: 'flex', alignItems: 'center' }} onClick={() => setShowNew(true)}>
          <Icon name="plus" size={14} /> New group
        </button>
      </div>

      <div className="cols-4 u-gap-12px u-mb-20px" >
        {[
          { label: 'Active groups', value: rows.filter((row) => row.status === 'active').length, color: 'var(--text)' },
          { label: 'Competitive', value: rows.filter((row) => row.type === 'competitive').length, color: 'var(--accent-text, var(--accent))' },
          { label: 'Developmental', value: rows.filter((row) => row.type === 'developmental').length, color: 'var(--warning)' },
          { label: 'Total enrolled', value: totalRegistrations, color: 'var(--success)' },
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
          <input aria-label="Search groups..." type="text" placeholder="Search groups..." value={search} onChange={(e) => setSearch(e.target.value)} className="u-p-8px-12px-8px-32px u-fs-fs-base" style={{ width: '100%' }} />
        </div>
        <select value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)} style={selectStyle}>
          <option value="all">All types</option>
          <option value="competitive">Competitive</option>
          <option value="recreational">Recreational</option>
          <option value="developmental">Developmental</option>
          <option value="masters">Masters</option>
          <option value="other">Other</option>
        </select>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={selectStyle}>
          <option value="active">Active</option>
          <option value="archived">Archived</option>
          <option value="all">All</option>
        </select>
        <span className="u-fs-fs-sm u-ff-font-body u-ml-auto" style={{ color: 'var(--text-muted)' }}>{filtered.length} groups</span>
      </div>

      {loading ? (
        <EmptyState icon="layers" title="Loading groups" description="Fetching live group data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load groups" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No groups found" emptyDescription="Try adjusting your filters." />
      )}

      {/* New Group Modal */}
      <Modal open={Boolean(showNew)} onClose={() => setShowNew(false)} title="New group" width={480}>
            {formError && <div role="alert" className="form-banner-error u-mb-16px">{formError}</div>}
            <form onSubmit={handleCreateGroup} className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div className="cols-2 u-gap-12px" >
                <div>
                  <label className="form-label form-label-required">Code</label>
                  <input aria-label="COMP-A" required style={INPUT_STYLE} value={form.code} onChange={(e) => setForm((f) => ({ ...f, code: e.target.value.toUpperCase() }))} placeholder="COMP-A" />
                </div>
                <div>
                  <label className="form-label form-label-required">Name</label>
                  <input aria-label="Competitive A" required style={INPUT_STYLE} value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="Competitive A" />
                </div>
              </div>
              <div className="cols-2 u-gap-12px" >
                <div>
                  <label className="form-label">Type</label>
                  <select style={INPUT_STYLE} value={form.type} onChange={(e) => setForm((f) => ({ ...f, type: e.target.value }))}>
                    <option value="competitive">Competitive</option>
                    <option value="recreational">Recreational</option>
                    <option value="developmental">Developmental</option>
                    <option value="masters">Masters</option>
                    <option value="other">Other</option>
                  </select>
                </div>
                <div>
                  <label className="form-label">Coach</label>
                  <select style={INPUT_STYLE} value={form.coach_user_id} onChange={(e) => setForm((f) => ({ ...f, coach_user_id: e.target.value }))}>
                    <option value="">Unassigned</option>
                    {staff.map((member) => (
                      <option key={member.id} value={member.id}>{member.name}</option>
                    ))}
                  </select>
                </div>
              </div>
              <div className="cols-2 u-gap-12px" >
                <div>
                  <label className="form-label">Min age</label>
                  <input aria-label="9" type="number" min="0" max="99" style={INPUT_STYLE} value={form.min_age} onChange={(e) => setForm((f) => ({ ...f, min_age: e.target.value }))} placeholder="9" />
                </div>
                <div>
                  <label className="form-label">Max age</label>
                  <input aria-label="14" type="number" min="0" max="99" style={INPUT_STYLE} value={form.max_age} onChange={(e) => setForm((f) => ({ ...f, max_age: e.target.value }))} placeholder="14" />
                </div>
              </div>
              <div>
                <label className="form-label">Description</label>
                <textarea aria-label="Optional description..." style={{ ...INPUT_STYLE, resize: 'vertical' }} rows={2} value={form.description} onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))} placeholder="Optional description..." />
              </div>
              <div className="form-actions">
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Creating…' : 'Create group'}</button>
              </div>
            </form>
      </Modal>
    </div>
  )
}
