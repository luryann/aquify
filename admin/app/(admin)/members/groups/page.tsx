'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchGroups, fetchRegistrations, type GroupRecord, type RegistrationRecord } from '@/lib/adminData'

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
  return <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em' }}>{c.label}</span>
}

const COLUMNS: Column<GroupRow>[] = [
  {
    key: 'name',
    label: 'Group',
    sortable: true,
    render: (row) => (
      <div>
        <Link href={`/members/groups/${row.id}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}>{row.name}</Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '2px' }}>{row.id}</div>
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
    render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{row.swimmerCount}</span>,
  },
  {
    key: 'coachName',
    label: 'Coach',
    render: (row) => <span style={{ fontSize: '13px', color: row.coachName ? 'var(--text-subtle)' : 'var(--text-muted)' }}>{row.coachName ?? 'Unassigned'}</span>,
  },
  {
    key: 'minAge',
    label: 'Age Range',
    render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{row.minAge ?? '—'}-{row.maxAge ?? '+'}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: row.status === 'active' ? 'var(--success-bg)' : 'var(--bg-elevated)', color: row.status === 'active' ? 'var(--success)' : 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.02em' }}>{row.status}</span>,
  },
]

function deriveRows(groups: GroupRecord[], registrations: RegistrationRecord[]): GroupRow[] {
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
      coachName: group.coach_user_id || null,
      minAge: group.min_age ?? null,
      maxAge: group.max_age ?? null,
      status: (group.is_active ? 'active' : 'archived') as 'active' | 'archived',
      createdAt: group.created_at,
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

export default function GroupsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<GroupRow[]>([])
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('active')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

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
        const [groups, registrations] = await Promise.all([
          fetchGroups(api, session.user.orgId),
          fetchRegistrations(api, session.user.orgId),
        ])
        if (!cancelled) setRows(deriveRows(groups, registrations))
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

  return (
    <div style={{ padding: '24px', maxWidth: '1100px' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Groups</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Live training-group inventory and enrollment counts</p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <Icon name="plus" size={14} /> New Group
        </button>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Active Groups', value: rows.filter((row) => row.status === 'active').length, color: 'var(--text)' },
          { label: 'Competitive', value: rows.filter((row) => row.type === 'competitive').length, color: 'var(--accent)' },
          { label: 'Developmental', value: rows.filter((row) => row.type === 'developmental').length, color: 'var(--warning)' },
          { label: 'Total Enrolled', value: totalRegistrations, color: 'var(--success)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
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
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>{filtered.length} groups</span>
      </div>

      {loading ? (
        <EmptyState icon="layers" title="Loading groups" description="Fetching live group data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load groups" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No groups found" emptyDescription="Try adjusting your filters." />
      )}
    </div>
  )
}
