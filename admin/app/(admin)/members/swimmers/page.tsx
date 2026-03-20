'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchGroups, fetchHouseholds, fetchSwimmerGroups, fetchSwimmers, type GroupRecord, type HouseholdRecord, type SwimmerGroupLinkRecord, type SwimmerRecord } from '@/lib/adminData'

type SwimmerStatus = 'active' | 'inactive' | 'pending' | 'archived'

type SwimmerRow = {
  id: string
  name: string
  ussNumber: string
  age: number
  gender: 'M' | 'F' | 'X'
  household: string
  householdId: string
  groups: string[]
  status: SwimmerStatus
}

function calcAge(dob: string): number {
  const birthDate = new Date(dob)
  const now = new Date()
  let age = now.getFullYear() - birthDate.getFullYear()
  const month = now.getMonth() - birthDate.getMonth()
  if (month < 0 || (month === 0 && now.getDate() < birthDate.getDate())) age--
  return Math.max(age, 0)
}

function genderLabel(value: string): 'M' | 'F' | 'X' {
  if (value === 'male') return 'M'
  if (value === 'female') return 'F'
  return 'X'
}

function StatusBadge({ status }: { status: SwimmerStatus }) {
  const cfg: Record<SwimmerStatus, { label: string; color: string; bg: string }> = {
    active: { label: 'Active', color: 'var(--success)', bg: 'var(--success-bg)' },
    inactive: { label: 'Inactive', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    pending: { label: 'Pending', color: 'var(--warning)', bg: 'var(--warning-bg)' },
    archived: { label: 'Archived', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  }
  const c = cfg[status]
  return <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.02em' }}>{c.label}</span>
}

const COLUMNS: Column<SwimmerRow>[] = [
  {
    key: 'name',
    label: 'Swimmer',
    sortable: true,
    render: (row) => (
      <div>
        <Link href={`/members/swimmers/${row.id}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}>{row.name}</Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px', fontFamily: 'var(--font-mono)' }}>{row.ussNumber || 'No external ID'}</div>
      </div>
    ),
  },
  {
    key: 'age',
    label: 'Age',
    sortable: true,
    align: 'right',
    render: (row) => (
      <div style={{ textAlign: 'right' }}>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text)' }}>{row.age}</span>
        <span style={{ fontSize: '11px', color: 'var(--text-muted)', marginLeft: '4px' }}>{row.gender}</span>
      </div>
    ),
  },
  {
    key: 'household',
    label: 'Household',
    render: (row) => <Link href={`/members/households/${row.householdId}`} style={{ fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none' }}>{row.household}</Link>,
  },
  {
    key: 'groups',
    label: 'Groups',
    render: (row) => (
      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
        {row.groups.length > 0 ? row.groups.map((group) => (
          <span key={group} style={{ fontSize: '11px', padding: '2px 7px', borderRadius: '8px', background: 'var(--accent-subtle)', color: 'var(--accent)', fontFamily: 'var(--font-mono)' }}>{group}</span>
        )) : <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>—</span>}
      </div>
    ),
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => <StatusBadge status={row.status} />,
  },
  {
    key: 'id',
    label: '',
    width: '60px',
    render: (row) => <Link href={`/members/swimmers/${row.id}`} style={{ fontSize: '12px', color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px' }}>View <Icon name="chevron-right" size={12} /></Link>,
  },
]

function deriveRows(swimmers: SwimmerRecord[], households: HouseholdRecord[], groups: GroupRecord[], links: SwimmerGroupLinkRecord[]): SwimmerRow[] {
  const householdById = new Map(households.map((household) => [household.id, household.household_name]))
  const groupNameByCode = new Map(groups.map((group) => [group.code, group.name]))
  const groupsBySwimmer = links.reduce<Record<string, string[]>>((acc, link) => {
    const groupName = groupNameByCode.get(link.group_code) ?? link.group_code
    acc[link.swimmer_id] = [...(acc[link.swimmer_id] ?? []), groupName]
    return acc
  }, {})

  return swimmers
    .map((swimmer) => ({
      id: swimmer.id,
      name: [swimmer.preferred_name || swimmer.first_name, swimmer.last_name].filter(Boolean).join(' '),
      ussNumber: swimmer.swimmer_external_id || '',
      age: calcAge(swimmer.date_of_birth),
      gender: genderLabel(swimmer.gender),
      household: householdById.get(swimmer.household_id) ?? swimmer.household_id,
      householdId: swimmer.household_id,
      groups: (groupsBySwimmer[swimmer.id] ?? []).sort(),
      status: (swimmer.registration_status as SwimmerStatus) || 'pending',
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

export default function SwimmersPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<SwimmerRow[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [genderFilter, setGenderFilter] = useState('all')
  const [groupFilter, setGroupFilter] = useState('all')
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
        const [swimmers, households, groups, swimmerGroups] = await Promise.all([
          fetchSwimmers(api, session.user.orgId),
          fetchHouseholds(api, session.user.orgId),
          fetchGroups(api, session.user.orgId),
          fetchSwimmerGroups(api, session.user.orgId),
        ])
        if (!cancelled) setRows(deriveRows(swimmers, households, groups, swimmerGroups))
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load swimmers.')
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

  const groups = useMemo(() => [...new Set(rows.flatMap((row) => row.groups))].sort(), [rows])
  const filtered = useMemo(() => {
    return rows.filter((row) => {
      const query = search.toLowerCase()
      const matchesSearch = !search || row.name.toLowerCase().includes(query) || row.ussNumber.toLowerCase().includes(query) || row.household.toLowerCase().includes(query)
      const matchesStatus = statusFilter === 'all' || row.status === statusFilter
      const matchesGender = genderFilter === 'all' || row.gender === genderFilter
      const matchesGroup = groupFilter === 'all' || row.groups.includes(groupFilter)
      return matchesSearch && matchesStatus && matchesGender && matchesGroup
    })
  }, [rows, search, statusFilter, genderFilter, groupFilter])

  const counts = useMemo(() => ({
    total: rows.length,
    active: rows.filter((row) => row.status === 'active').length,
    male: rows.filter((row) => row.gender === 'M').length,
    female: rows.filter((row) => row.gender === 'F').length,
  }), [rows])

  const selectStyle: React.CSSProperties = { padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', cursor: 'pointer' }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Swimmers</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Live roster membership and current group assignment data</p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <Icon name="plus" size={14} />
          Add Swimmer
        </button>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Total Roster', value: counts.total, color: 'var(--text)' },
          { label: 'Active', value: counts.active, color: 'var(--success)' },
          { label: 'Male', value: counts.male, color: 'var(--info)' },
          { label: 'Female', value: counts.female, color: 'var(--accent)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: 1, minWidth: '200px', maxWidth: '320px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search swimmers or households..." value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Statuses</option>
          <option value="active">Active</option>
          <option value="pending">Pending</option>
          <option value="inactive">Inactive</option>
          <option value="archived">Archived</option>
        </select>
        <select value={genderFilter} onChange={(e) => setGenderFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Genders</option>
          <option value="M">Male</option>
          <option value="F">Female</option>
          <option value="X">Other / Undisclosed</option>
        </select>
        <select value={groupFilter} onChange={(e) => setGroupFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Groups</option>
          {groups.map((group) => <option key={group} value={group}>{group}</option>)}
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>{filtered.length} of {rows.length}</span>
      </div>

      {loading ? (
        <EmptyState icon="users" title="Loading swimmers" description="Fetching live roster data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load swimmers" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No swimmers found" emptyDescription="Adjust your filters to see results." />
      )}
    </div>
  )
}
