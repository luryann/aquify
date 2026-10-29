'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchGroups, fetchHouseholds, fetchSwimmerGroups, fetchSwimmers, createSwimmer, type GroupRecord, type HouseholdRecord, type SwimmerGroupLinkRecord, type SwimmerRecord } from '@/lib/adminData'
import { parseBackendDate } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Modal } from '@/components/ui/Modal'

type SwimmerStatus = 'active' | 'inactive' | 'pending' | 'archived'

type Gender = 'female' | 'male' | 'non_binary' | 'undisclosed'

const GENDER_SHORT: Record<Gender, string> = { female: 'F', male: 'M', non_binary: 'NB', undisclosed: 'U' }
const GENDER_LABEL: Record<Gender, string> = { female: 'Female', male: 'Male', non_binary: 'Non-binary', undisclosed: 'Undisclosed' }

type SwimmerRow = {
  id: string
  name: string
  ussNumber: string
  age: number
  gender: Gender
  household: string
  householdId: string
  groups: string[]
  status: SwimmerStatus
}

function calcAge(dob: string): number {
  const birthDate = parseBackendDate(dob)
  const now = new Date()
  let age = now.getFullYear() - birthDate.getFullYear()
  const month = now.getMonth() - birthDate.getMonth()
  if (month < 0 || (month === 0 && now.getDate() < birthDate.getDate())) age--
  return Math.max(age, 0)
}

function normalizeGender(value: string): Gender {
  if (value === 'male') return 'male'
  if (value === 'female') return 'female'
  if (value === 'non_binary') return 'non_binary'
  return 'undisclosed'
}

function StatusBadge({ status }: { status: SwimmerStatus }) {
  const cfg: Record<SwimmerStatus, { label: string; color: string; bg: string }> = {
    active: { label: 'Active', color: 'var(--success)', bg: 'var(--success-bg)' },
    inactive: { label: 'Inactive', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    pending: { label: 'Pending', color: 'var(--warning)', bg: 'var(--warning-bg)' },
    archived: { label: 'Archived', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  }
  const c = cfg[status]
  return <span className="badge" style={{ background: c.bg, color: c.color }}>{c.label}</span>
}

const COLUMNS: Column<SwimmerRow>[] = [
  {
    key: 'name',
    label: 'Swimmer',
    sortable: true,
    render: (row) => (
      <div>
        <Link href={`/members/swimmers/${row.id}`} className="u-fw-600 u-fs-fs-base" style={{ color: 'var(--text)', textDecoration: 'none' }}>{row.name}</Link>
        <div className="u-fs-fs-xs u-mt-2px u-ff-font-body" style={{ color: 'var(--text-muted)' }}>{row.ussNumber || 'No external ID'}</div>
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
        <span className="u-ff-font-body u-fs-fs-base" style={{ color: 'var(--text)' }}>{row.age}</span>
        <span className="u-fs-fs-xs u-ml-4px" style={{ color: 'var(--text-muted)' }}>{GENDER_SHORT[row.gender]}</span>
      </div>
    ),
  },
  {
    key: 'household',
    label: 'Household',
    render: (row) => <Link href={`/members/households/${row.householdId}`} className="u-fs-fs-base" style={{ color: 'var(--text-subtle)', textDecoration: 'none' }}>{row.household}</Link>,
  },
  {
    key: 'groups',
    label: 'Groups',
    render: (row) => (
      <div className="u-gap-4px" style={{ display: 'flex', flexWrap: 'wrap' }}>
        {row.groups.length > 0 ? row.groups.map((group) => (
          <span key={group} className="u-fs-fs-xs u-p-2px-8px u-ff-font-body" style={{ borderRadius: '8px', background: 'var(--accent-subtle)', color: 'var(--accent-text, var(--accent))' }}>{group}</span>
        )) : <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>—</span>}
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
    render: (row) => <Link href={`/members/swimmers/${row.id}`} className="u-fs-fs-sm u-gap-4px" style={{ color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center' }}>View <Icon name="chevron-right" size={12} /></Link>,
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
      gender: normalizeGender(swimmer.gender),
      household: householdById.get(swimmer.household_id) ?? swimmer.household_id,
      householdId: swimmer.household_id,
      groups: (groupsBySwimmer[swimmer.id] ?? []).sort(),
      status: (swimmer.registration_status as SwimmerStatus) || 'pending',
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

const INPUT_STYLE: React.CSSProperties = { width: '100%' }
const EMPTY_FORM = { first_name: '', last_name: '', household_id: '', date_of_birth: '', gender: 'female', usa_swimming_id: '', registration_status: 'active' }

export function SwimmersPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<SwimmerRow[]>([])
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [genderFilter, setGenderFilter] = useState('all')
  const [groupFilter, setGroupFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // New Swimmer modal
  const [showNew, setShowNew] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [form, setForm] = useState(EMPTY_FORM)

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
        if (!cancelled) {
          setRows(deriveRows(swimmers, households, groups, swimmerGroups))
          setHouseholds(households)
        }
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load swimmers. Try again.'))
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
    male: rows.filter((row) => row.gender === 'male').length,
    female: rows.filter((row) => row.gender === 'female').length,
  }), [rows])

  async function handleCreateSwimmer(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId) return
    setSubmitting(true)
    setFormError(null)
    try {
      await createSwimmer(api, session.user.orgId, form)
      setShowNew(false)
      setForm(EMPTY_FORM)
      const [swimmers, hhs, grps, swimmerGroups] = await Promise.all([
        fetchSwimmers(api, session.user.orgId),
        fetchHouseholds(api, session.user.orgId),
        fetchGroups(api, session.user.orgId),
        fetchSwimmerGroups(api, session.user.orgId),
      ])
      setRows(deriveRows(swimmers, hhs, grps, swimmerGroups))
      setHouseholds(hhs)
    } catch (err) {
      setFormError(uiErrorMessage(err, 'Couldn’t create swimmer. Try again.'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <div className="u-mb-16px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <button type="button" className="btn btn-primary u-gap-6px" style={{ display: 'flex', alignItems: 'center' }} onClick={() => setShowNew(true)}>
          <Icon name="plus" size={14} />
          Add swimmer
        </button>
      </div>

      <div className="cols-4 u-gap-12px u-mb-20px" >
        {[
          { label: 'Total roster', value: counts.total, color: 'var(--text)' },
          { label: 'Active', value: counts.active, color: 'var(--success)' },
          { label: 'Male', value: counts.male, color: 'var(--info)' },
          { label: 'Female', value: counts.female, color: 'var(--accent-text, var(--accent))' },
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
          <input aria-label="Search swimmers or households..." type="text" placeholder="Search swimmers or households..." value={search} onChange={(e) => setSearch(e.target.value)} className="u-p-8px-12px-8px-32px u-fs-fs-base" style={{ width: '100%' }} />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
          <option value="all">All statuses</option>
          <option value="active">Active</option>
          <option value="pending">Pending</option>
          <option value="inactive">Inactive</option>
          <option value="archived">Archived</option>
        </select>
        <select value={genderFilter} onChange={(e) => setGenderFilter(e.target.value)}>
          <option value="all">All genders</option>
          <option value="female">{GENDER_LABEL.female}</option>
          <option value="male">{GENDER_LABEL.male}</option>
          <option value="non_binary">{GENDER_LABEL.non_binary}</option>
          <option value="undisclosed">{GENDER_LABEL.undisclosed}</option>
        </select>
        <select value={groupFilter} onChange={(e) => setGroupFilter(e.target.value)}>
          <option value="all">All groups</option>
          {groups.map((group) => <option key={group} value={group}>{group}</option>)}
        </select>
        <span className="u-fs-fs-sm u-ff-font-body u-ml-auto" style={{ color: 'var(--text-muted)' }}>{filtered.length} of {rows.length}</span>
      </div>

      {loading ? (
        <EmptyState icon="users" title="Loading swimmers" description="Fetching your roster." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load swimmers" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No swimmers found" emptyDescription="Adjust your filters to see results." />
      )}

      {/* Add Swimmer Modal */}
      <Modal open={Boolean(showNew)} onClose={() => setShowNew(false)} title="Add swimmer" width={500}>
            {formError && <div role="alert" className="form-banner-error u-mb-16px">{formError}</div>}
            <form onSubmit={handleCreateSwimmer} className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div className="cols-2 u-gap-12px" >
                <div>
                  <label className="form-label form-label-required">First name</label>
                  <input aria-label="Jane" required style={INPUT_STYLE} value={form.first_name} onChange={(e) => setForm((f) => ({ ...f, first_name: e.target.value }))} placeholder="Jane" />
                </div>
                <div>
                  <label className="form-label form-label-required">Last name</label>
                  <input aria-label="Smith" required style={INPUT_STYLE} value={form.last_name} onChange={(e) => setForm((f) => ({ ...f, last_name: e.target.value }))} placeholder="Smith" />
                </div>
              </div>
              <div>
                <label className="form-label form-label-required">Household</label>
                <select required style={INPUT_STYLE} value={form.household_id} onChange={(e) => setForm((f) => ({ ...f, household_id: e.target.value }))}>
                  <option value="">Select household…</option>
                  {households.map((hh) => <option key={hh.id} value={hh.id}>{hh.household_name}</option>)}
                </select>
              </div>
              <div className="cols-2 u-gap-12px" >
                <div>
                  <label className="form-label form-label-required">Date of birth</label>
                  <input required type="date" style={INPUT_STYLE} value={form.date_of_birth} onChange={(e) => setForm((f) => ({ ...f, date_of_birth: e.target.value }))} />
                </div>
                <div>
                  <label className="form-label form-label-required">Gender</label>
                  <select required style={INPUT_STYLE} value={form.gender} onChange={(e) => setForm((f) => ({ ...f, gender: e.target.value }))}>
                    <option value="female">Female</option>
                    <option value="male">Male</option>
                    <option value="non_binary">Non-binary</option>
                    <option value="undisclosed">Undisclosed</option>
                  </select>
                </div>
              </div>
              <div>
                <label className="form-label">USA Swimming ID</label>
                <input aria-label="e.g. AB12CD34EF56" style={INPUT_STYLE} value={form.usa_swimming_id} onChange={(e) => setForm((f) => ({ ...f, usa_swimming_id: e.target.value.toUpperCase() }))} placeholder="e.g. AB12CD34EF56" />
              </div>
              <div>
                <label className="form-label">Registration status</label>
                <select style={INPUT_STYLE} value={form.registration_status} onChange={(e) => setForm((f) => ({ ...f, registration_status: e.target.value }))}>
                  <option value="active">Active</option>
                  <option value="pending">Pending</option>
                  <option value="inactive">Inactive</option>
                </select>
              </div>
              <div className="form-actions">
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Adding…' : 'Add swimmer'}</button>
              </div>
            </form>
      </Modal>
    </div>
  )
}
