'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'

// ─── Types ────────────────────────────────────────────────────────────────────

type GroupType = 'training' | 'age-group' | 'developmental'

type Group = {
  id: string
  name: string
  type: GroupType
  season: string
  swimmerCount: number
  coachName: string | null
  minAge: number | null
  maxAge: number | null
  status: 'active' | 'archived'
  createdAt: string
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_GROUPS: Group[] = [
  { id: 'grp-001', name: 'Senior Elite',       type: 'training',      season: '2024–25', swimmerCount: 18, coachName: 'Coach Williams', minAge: 13, maxAge: null, status: 'active',   createdAt: '2023-08-01' },
  { id: 'grp-002', name: 'Senior B',           type: 'training',      season: '2024–25', swimmerCount: 14, coachName: 'Coach Williams', minAge: 12, maxAge: 15,   status: 'active',   createdAt: '2023-08-01' },
  { id: 'grp-003', name: 'Junior A',           type: 'training',      season: '2024–25', swimmerCount: 22, coachName: 'Coach Martinez', minAge: 10, maxAge: 13,   status: 'active',   createdAt: '2023-08-01' },
  { id: 'grp-004', name: 'Junior B',           type: 'training',      season: '2024–25', swimmerCount: 19, coachName: 'Coach Martinez', minAge: 9,  maxAge: 12,   status: 'active',   createdAt: '2023-08-01' },
  { id: 'grp-005', name: 'Development Gold',   type: 'developmental', season: '2024–25', swimmerCount: 16, coachName: 'Coach Rivera',   minAge: 7,  maxAge: 10,   status: 'active',   createdAt: '2023-08-01' },
  { id: 'grp-006', name: 'Development Silver', type: 'developmental', season: '2024–25', swimmerCount: 20, coachName: 'Coach Rivera',   minAge: 6,  maxAge: 9,    status: 'active',   createdAt: '2023-08-01' },
  { id: 'grp-007', name: '12 & Under Boys',    type: 'age-group',     season: '2024–25', swimmerCount: 31, coachName: null,             minAge: null,maxAge: 12,  status: 'active',   createdAt: '2024-01-01' },
  { id: 'grp-008', name: '12 & Under Girls',   type: 'age-group',     season: '2024–25', swimmerCount: 28, coachName: null,             minAge: null,maxAge: 12,  status: 'active',   createdAt: '2024-01-01' },
  { id: 'grp-009', name: 'Senior Elite 2023',  type: 'training',      season: '2023–24', swimmerCount: 0,  coachName: 'Coach Williams', minAge: 13, maxAge: null, status: 'archived', createdAt: '2022-08-01' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

const TYPE_CONFIG: Record<GroupType, { label: string; color: string; bg: string }> = {
  'training':      { label: 'Training',      color: 'var(--accent)',   bg: 'var(--accent-subtle)' },
  'age-group':     { label: 'Age Group',     color: 'var(--info)',     bg: 'var(--info-bg)' },
  'developmental': { label: 'Developmental', color: 'var(--warning)',  bg: 'var(--warning-bg)' },
}

function TypeBadge({ type }: { type: GroupType }) {
  const c = TYPE_CONFIG[type]
  return (
    <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em' }}>
      {c.label}
    </span>
  )
}

// ─── Columns ──────────────────────────────────────────────────────────────────

const COLUMNS: Column<Group>[] = [
  {
    key: 'name', label: 'Group', sortable: true,
    render: (r) => (
      <div>
        <Link href={`/members/groups/${r.id}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
          {r.name}
        </Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '2px' }}>{r.season}</div>
      </div>
    ),
  },
  { key: 'type',  label: 'Type', render: (r) => <TypeBadge type={r.type} /> },
  {
    key: 'swimmerCount', label: 'Swimmers', sortable: true, align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{r.swimmerCount}</span>,
  },
  {
    key: 'coachName', label: 'Coach',
    render: (r) => <span style={{ fontSize: '13px', color: r.coachName ? 'var(--text-subtle)' : 'var(--text-muted)' }}>{r.coachName ?? '—'}</span>,
  },
  {
    key: 'minAge', label: 'Age Range',
    render: (r) => {
      if (!r.minAge && !r.maxAge) return <span style={{ color: 'var(--text-muted)', fontSize: '12px' }}>Any</span>
      const lo = r.minAge ? `${r.minAge}` : '—'
      const hi = r.maxAge ? `${r.maxAge}` : '+'
      return <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{lo}–{hi}</span>
    },
  },
  {
    key: 'status', label: 'Status',
    render: (r) => (
      <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px',
        background: r.status === 'active' ? 'var(--success-bg)' : 'var(--bg-elevated)',
        color:      r.status === 'active' ? 'var(--success)' : 'var(--text-muted)',
        fontFamily: 'var(--font-mono)', textTransform: 'uppercase' as const, letterSpacing: '0.02em' }}>
        {r.status}
      </span>
    ),
  },
  {
    key: 'id', label: '', width: '60px',
    render: (r) => (
      <Link href={`/members/groups/${r.id}`} style={{ fontSize: '12px', color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}>
        View <Icon name="chevron-right" size={12} />
      </Link>
    ),
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function GroupsPage() {
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('active')

  const filtered = useMemo(() => {
    return MOCK_GROUPS.filter((g) => {
      const matchesSearch = !search || g.name.toLowerCase().includes(search.toLowerCase()) || (g.coachName ?? '').toLowerCase().includes(search.toLowerCase())
      const matchesType   = typeFilter   === 'all' || g.type   === typeFilter
      const matchesStatus = statusFilter === 'all' || g.status === statusFilter
      return matchesSearch && matchesType && matchesStatus
    })
  }, [search, typeFilter, statusFilter])

  const totalSwimmers = MOCK_GROUPS.filter((g) => g.status === 'active').reduce((acc, g) => acc + g.swimmerCount, 0)

  const selectStyle: React.CSSProperties = {
    padding: '8px 12px',
    background: 'var(--bg-elevated)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--r-md)',
    fontSize: '13px',
    color: 'var(--text)',
    outline: 'none',
    cursor: 'pointer',
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1100px' }}>

      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Groups</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Training groups, age groups, and eligibility configurations</p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <Icon name="plus" size={14} /> New Group
        </button>
      </div>

      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Active Groups',  value: MOCK_GROUPS.filter((g) => g.status === 'active').length,  color: 'var(--text)' },
          { label: 'Training',       value: MOCK_GROUPS.filter((g) => g.type === 'training').length,  color: 'var(--accent)' },
          { label: 'Age Group',      value: MOCK_GROUPS.filter((g) => g.type === 'age-group').length, color: 'var(--info)' },
          { label: 'Total Enrolled', value: totalSwimmers,                                            color: 'var(--success)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '300px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search groups..." value={search} onChange={(e) => setSearch(e.target.value)}
            style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Types</option>
          <option value="training">Training</option>
          <option value="age-group">Age Group</option>
          <option value="developmental">Developmental</option>
        </select>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={selectStyle}>
          <option value="active">Active</option>
          <option value="archived">Archived</option>
          <option value="all">All</option>
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>
          {filtered.length} groups
        </span>
      </div>

      <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No groups found" emptyDescription="Try adjusting your filters." />
    </div>
  )
}
