'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchGroups, fetchRegistrations, fetchUsers, createGroup, type GroupRecord, type RegistrationRecord, type UserRecord } from '@/lib/adminData'
import { canWrite } from '@/lib/roles'

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
  competitive: { label: 'Competitive', color: 'var(--accent)', bg: 'var(--accent-subtle)' },
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
        <Link href={`/members/groups/${row.id}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}>{row.name}</Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginTop: '2px' }}>{row.id}</div>
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
    label: 'Registrations',
    sortable: true,
    align: 'right',
    render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{row.swimmerCount}</span>,
  },
  {
    key: 'coachName',
    label: 'Coach',
    render: (row) => <span style={{ fontSize: '13px', color: row.coachName ? 'var(--text-subtle)' : 'var(--text-muted)' }}>{row.coachName ?? 'Unassigned'}</span>,
  },
  {
    key: 'minAge',
    label: 'Age Range',
    render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--text-subtle)' }}>{row.minAge ?? '—'}-{row.maxAge ?? '+'}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => <span className="badge" style={{ background: row.status === 'active' ? 'var(--success-bg)' : 'var(--bg-elevated)', color: row.status === 'active' ? 'var(--success)' : 'var(--text-muted)' }}>{row.status}</span>,
  },
]

function displayNameForUser(user: UserRecord): string {
  const name = `${user.firstName ?? ''} ${user.lastName ?? ''}`.trim()
  return name || user.email || user.id
}

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

const INPUT_STYLE: React.CSSProperties = { width: '100%', padding: '9px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' }
const LABEL_STYLE: React.CSSProperties = { fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '5px' }

export default function GroupsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const allowWrite = canWrite(session?.user.roles ?? [])
  const [rows, setRows] = useState<GroupRow[]>([])
  const [userNameById, setUserNameById] = useState<Record<string, string>>({})
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('active')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // New Group modal
  const [showNew, setShowNew] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [form, setForm] = useState({ code: '', name: '', description: '', type: 'competitive', min_age: '', max_age: '' })

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
          fetchUsers(api, session.user.orgId, { limit: 200 }).catch(() => ({ users: [] as UserRecord[] })),
        ])
        const userNameById = Object.fromEntries(users.users.map((u) => [u.id, displayNameForUser(u)]))
        if (!cancelled) {
          setUserNameById(userNameById)
          setRows(deriveRows(groups, registrations, userNameById))
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load groups.')
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

  const selectStyle: React.CSSProperties = { padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', cursor: 'pointer' }

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
      })
      setShowNew(false)
      setForm({ code: '', name: '', description: '', type: 'competitive', min_age: '', max_age: '' })
      const [grps, regs] = await Promise.all([fetchGroups(api, session.user.orgId), fetchRegistrations(api, session.user.orgId)])
      setRows(deriveRows(grps, regs, userNameById))
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : 'Failed to create group.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1100px' }}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Groups</h1>
          <p className="page-subtitle">Live training-group inventory and enrollment counts</p>
        </div>
        {allowWrite && (
          <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setShowNew(true)}>
            <Icon name="plus" size={14} /> New Group
          </button>
        )}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Active Groups', value: rows.filter((row) => row.status === 'active').length, color: 'var(--text)' },
          { label: 'Competitive', value: rows.filter((row) => row.type === 'competitive').length, color: 'var(--accent)' },
          { label: 'Developmental', value: rows.filter((row) => row.type === 'developmental').length, color: 'var(--warning)' },
          { label: 'Total Enrolled', value: totalRegistrations, color: 'var(--success)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-body)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-body)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '300px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search groups..." value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Types</option>
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
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginLeft: 'auto' }}>{filtered.length} groups</span>
      </div>

      {loading ? (
        <EmptyState icon="layers" title="Loading groups" description="Fetching live group data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load groups" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No groups found" emptyDescription="Try adjusting your filters." />
      )}

      {/* New Group Modal */}
      {showNew && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.6)' }} onClick={(e) => { if (e.target === e.currentTarget) setShowNew(false) }}>
          <div style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', borderRadius: '14px', padding: '28px', width: '100%', maxWidth: '480px', boxShadow: '0 24px 80px rgba(0,0,0,0.4)' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <h2 style={{ fontSize: '18px', fontFamily: 'var(--font-display)', color: 'var(--text)', margin: 0 }}>New Group</h2>
              <button onClick={() => setShowNew(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)' }}><Icon name="x" size={18} /></button>
            </div>
            {formError && <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>{formError}</div>}
            <form onSubmit={handleCreateGroup} style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
                <div>
                  <label style={LABEL_STYLE}>Code *</label>
                  <input required style={INPUT_STYLE} value={form.code} onChange={(e) => setForm((f) => ({ ...f, code: e.target.value.toUpperCase() }))} placeholder="COMP-A" />
                </div>
                <div>
                  <label style={LABEL_STYLE}>Name *</label>
                  <input required style={INPUT_STYLE} value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="Competitive A" />
                </div>
              </div>
              <div>
                <label style={LABEL_STYLE}>Type</label>
                <select style={INPUT_STYLE} value={form.type} onChange={(e) => setForm((f) => ({ ...f, type: e.target.value }))}>
                  <option value="competitive">Competitive</option>
                  <option value="recreational">Recreational</option>
                  <option value="developmental">Developmental</option>
                  <option value="masters">Masters</option>
                  <option value="other">Other</option>
                </select>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
                <div>
                  <label style={LABEL_STYLE}>Min Age</label>
                  <input type="number" min="0" max="99" style={INPUT_STYLE} value={form.min_age} onChange={(e) => setForm((f) => ({ ...f, min_age: e.target.value }))} placeholder="9" />
                </div>
                <div>
                  <label style={LABEL_STYLE}>Max Age</label>
                  <input type="number" min="0" max="99" style={INPUT_STYLE} value={form.max_age} onChange={(e) => setForm((f) => ({ ...f, max_age: e.target.value }))} placeholder="14" />
                </div>
              </div>
              <div>
                <label style={LABEL_STYLE}>Description</label>
                <textarea style={{ ...INPUT_STYLE, resize: 'vertical' }} rows={2} value={form.description} onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))} placeholder="Optional description..." />
              </div>
              <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '4px' }}>
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Creating…' : 'Create Group'}</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
