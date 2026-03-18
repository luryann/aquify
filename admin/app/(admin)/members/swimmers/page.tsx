'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type SwimmerStatus = 'active' | 'inactive' | 'pending'

type Swimmer = {
  id: string
  name: string
  ussNumber: string
  dob: string
  age: number
  gender: 'M' | 'F'
  household: string
  householdId: string
  groups: string[]
  status: SwimmerStatus
  topEvent: string
  topTime: string | null
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_SWIMMERS: Swimmer[] = [
  { id: 'sw-001', name: 'Caleb Henderson',   ussNumber: '185-4821-A', dob: '2012-03-15', age: 12, gender: 'M', household: 'Henderson Family',  householdId: 'hh-001', groups: ['Senior Elite'],       status: 'active',   topEvent: '100 Free',   topTime: '52.84' },
  { id: 'sw-002', name: 'Mia Henderson',     ussNumber: '185-4822-B', dob: '2014-07-22', age: 10, gender: 'F', household: 'Henderson Family',  householdId: 'hh-001', groups: ['Junior A'],            status: 'active',   topEvent: '50 Back',    topTime: '34.20' },
  { id: 'sw-003', name: 'Owen Henderson',    ussNumber: '185-4823-C', dob: '2016-11-08', age: 8,  gender: 'M', household: 'Henderson Family',  householdId: 'hh-001', groups: ['Development Gold'],    status: 'active',   topEvent: '25 Free',    topTime: '18.65' },
  { id: 'sw-004', name: 'Jenna Park',        ussNumber: '185-5001-D', dob: '2010-05-14', age: 14, gender: 'F', household: 'Park Family',        householdId: 'hh-002', groups: ['Senior Elite'],       status: 'active',   topEvent: '200 IM',     topTime: '2:18.44' },
  { id: 'sw-005', name: 'Lucas Park',        ussNumber: '185-5002-E', dob: '2013-09-03', age: 11, gender: 'M', household: 'Park Family',        householdId: 'hh-002', groups: ['Junior A'],            status: 'active',   topEvent: '100 Fly',    topTime: '1:05.32' },
  { id: 'sw-006', name: 'Sofia Torres',      ussNumber: '185-6001-F', dob: '2009-01-20', age: 15, gender: 'F', household: 'Torres Household',   householdId: 'hh-003', groups: ['Senior Elite'],       status: 'active',   topEvent: '200 Free',   topTime: '2:02.18' },
  { id: 'sw-007', name: 'Ethan Kim',         ussNumber: '185-7001-G', dob: '2011-06-30', age: 13, gender: 'M', household: 'Kim Family',         householdId: 'hh-004', groups: ['Junior B'],            status: 'pending',  topEvent: '100 Breast', topTime: null },
  { id: 'sw-008', name: 'Lily Kim',          ussNumber: '185-7002-H', dob: '2013-11-12', age: 11, gender: 'F', household: 'Kim Family',         householdId: 'hh-004', groups: ['Development Silver'],  status: 'pending',  topEvent: '50 Free',    topTime: null },
  { id: 'sw-009', name: 'Ade Okafor',        ussNumber: '185-8001-I', dob: '2008-04-18', age: 16, gender: 'M', household: 'Okafor Household',   householdId: 'hh-005', groups: ['Senior Elite'],       status: 'active',   topEvent: '100 Back',   topTime: '56.12' },
  { id: 'sw-010', name: 'Tolu Okafor',       ussNumber: '185-8002-J', dob: '2010-08-25', age: 14, gender: 'F', household: 'Okafor Household',   householdId: 'hh-005', groups: ['Senior B'],            status: 'active',   topEvent: '200 Breast', topTime: '2:38.77' },
  { id: 'sw-011', name: 'Amara Okafor',      ussNumber: '185-8003-K', dob: '2012-02-14', age: 12, gender: 'F', household: 'Okafor Household',   householdId: 'hh-005', groups: ['Junior A'],            status: 'active',   topEvent: '100 Free',   topTime: '1:04.55' },
  { id: 'sw-012', name: 'Marco Reyes',       ussNumber: '185-9001-L', dob: '2015-07-07', age: 9,  gender: 'M', household: 'Reyes Family',       householdId: 'hh-006', groups: ['Development Gold'],    status: 'active',   topEvent: '25 Back',    topTime: '22.10' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: SwimmerStatus }) {
  const cfg: Record<SwimmerStatus, { label: string; color: string; bg: string }> = {
    active:   { label: 'Active',   color: 'var(--success)',    bg: 'var(--success-bg)' },
    inactive: { label: 'Inactive', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    pending:  { label: 'Pending',  color: 'var(--warning)',    bg: 'var(--warning-bg)' },
  }
  const c = cfg[status]
  return (
    <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.02em' }}>
      {c.label}
    </span>
  )
}

// ─── Columns ──────────────────────────────────────────────────────────────────

const COLUMNS: Column<Swimmer>[] = [
  {
    key: 'name', label: 'Swimmer', sortable: true,
    render: (r) => (
      <div>
        <Link href={`/members/swimmers/${r.id}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
          {r.name}
        </Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px', fontFamily: 'var(--font-mono)' }}>{r.ussNumber}</div>
      </div>
    ),
  },
  {
    key: 'age', label: 'Age', sortable: true, align: 'right',
    render: (r) => (
      <div style={{ textAlign: 'right' }}>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text)' }}>{r.age}</span>
        <span style={{ fontSize: '11px', color: 'var(--text-muted)', marginLeft: '4px' }}>{r.gender === 'M' ? 'M' : 'F'}</span>
      </div>
    ),
  },
  {
    key: 'household', label: 'Household',
    render: (r) => (
      <Link href={`/members/households/${r.householdId}`} style={{ fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-subtle)')}>
        {r.household}
      </Link>
    ),
  },
  {
    key: 'groups', label: 'Group',
    render: (r) => (
      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
        {r.groups.map((g) => (
          <span key={g} style={{ fontSize: '11px', padding: '2px 7px', borderRadius: '8px', background: 'var(--accent-subtle)', color: 'var(--accent)', fontFamily: 'var(--font-mono)' }}>{g}</span>
        ))}
        {r.groups.length === 0 && <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>—</span>}
      </div>
    ),
  },
  { key: 'status', label: 'Status', render: (r) => <StatusBadge status={r.status} /> },
  {
    key: 'topEvent', label: 'Top Event',
    render: (r) => (
      <div>
        <div style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{r.topEvent}</div>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', fontWeight: 600, color: r.topTime ? 'var(--accent)' : 'var(--text-muted)' }}>
          {r.topTime ?? '—'}
        </div>
      </div>
    ),
  },
  {
    key: 'id', label: '', width: '60px',
    render: (r) => (
      <Link href={`/members/swimmers/${r.id}`} style={{ fontSize: '12px', color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}>
        View <Icon name="chevron-right" size={12} />
      </Link>
    ),
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function SwimmersPage() {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [genderFilter, setGenderFilter] = useState('all')

  const GROUPS = [...new Set(MOCK_SWIMMERS.flatMap((s) => s.groups))].sort()
  const [groupFilter, setGroupFilter] = useState('all')

  const filtered = useMemo(() => {
    return MOCK_SWIMMERS.filter((s) => {
      const q = search.toLowerCase()
      const matchesSearch = !search || s.name.toLowerCase().includes(q) || s.ussNumber.toLowerCase().includes(q) || s.household.toLowerCase().includes(q)
      const matchesStatus = statusFilter === 'all' || s.status === statusFilter
      const matchesGender = genderFilter === 'all' || s.gender === genderFilter
      const matchesGroup  = groupFilter  === 'all' || s.groups.includes(groupFilter)
      return matchesSearch && matchesStatus && matchesGender && matchesGroup
    })
  }, [search, statusFilter, genderFilter, groupFilter])

  const counts = useMemo(() => ({
    total:  MOCK_SWIMMERS.length,
    active: MOCK_SWIMMERS.filter((s) => s.status === 'active').length,
    male:   MOCK_SWIMMERS.filter((s) => s.gender === 'M').length,
    female: MOCK_SWIMMERS.filter((s) => s.gender === 'F').length,
  }), [])

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
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>
            Swimmers
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Roster management and performance overview</p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <Icon name="plus" size={14} />
          Add Swimmer
        </button>
      </div>

      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Total Roster', value: counts.total,  color: 'var(--text)' },
          { label: 'Active',       value: counts.active, color: 'var(--success)' },
          { label: 'Male',         value: counts.male,   color: 'var(--info)' },
          { label: 'Female',       value: counts.female, color: 'var(--accent)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: 1, minWidth: '200px', maxWidth: '320px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input
            type="text" placeholder="Search swimmers, USS#..." value={search} onChange={(e) => setSearch(e.target.value)}
            style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }}
          />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Statuses</option>
          <option value="active">Active</option>
          <option value="pending">Pending</option>
          <option value="inactive">Inactive</option>
        </select>
        <select value={genderFilter} onChange={(e) => setGenderFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Genders</option>
          <option value="M">Male</option>
          <option value="F">Female</option>
        </select>
        <select value={groupFilter} onChange={(e) => setGroupFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Groups</option>
          {GROUPS.map((g) => <option key={g} value={g}>{g}</option>)}
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>
          {filtered.length} of {MOCK_SWIMMERS.length}
        </span>
      </div>

      {/* Table */}
      {filtered.length === 0 ? (
        <EmptyState icon="users" title="No swimmers found" description="Try adjusting your search or filters." />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No swimmers found" emptyDescription="Adjust your filters to see results." />
      )}
    </div>
  )
}
