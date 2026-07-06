'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchHouseholds, fetchRegistrations, fetchSwimmers, fetchGroups, createRegistration, type HouseholdRecord, type RegistrationRecord, type SwimmerRecord, type GroupRecord } from '@/lib/adminData'

type RegistrationStatus = 'pending' | 'active' | 'completed' | 'rejected' | 'withdrawn'
type WaiverStatus = 'pending' | 'accepted'

type RegistrationRow = {
  id: string
  household: string
  householdId: string
  season: string
  submittedAt: string
  status: RegistrationStatus
  waiverStatus: WaiverStatus
  swimmerCount: number
  notes: string | null
}

const STATUS_CONFIG: Record<RegistrationStatus, { label: string; color: string; bg: string }> = {
  pending: { label: 'Pending', color: 'var(--warning)', bg: 'var(--warning-bg)' },
  active: { label: 'Active', color: 'var(--success)', bg: 'var(--success-bg)' },
  completed: { label: 'Completed', color: 'var(--info)', bg: 'var(--info-bg)' },
  rejected: { label: 'Rejected', color: 'var(--error)', bg: 'var(--error-bg)' },
  withdrawn: { label: 'Withdrawn', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

const WAIVER_CONFIG: Record<WaiverStatus, { label: string; color: string }> = {
  pending: { label: 'Pending', color: 'var(--warning)' },
  accepted: { label: 'Signed', color: 'var(--success)' },
}

const COLUMNS: Column<RegistrationRow>[] = [
  {
    key: 'household',
    label: 'Household',
    sortable: true,
    render: (row) => <Link href={`/members/households/${row.householdId}`} style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}>{row.household}</Link>,
  },
  {
    key: 'season',
    label: 'Season',
    render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--text-subtle)', padding: '2px 7px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-xs)', border: '1px solid var(--border)' }}>{row.season}</span>,
  },
  {
    key: 'submittedAt',
    label: 'Submitted',
    sortable: true,
    render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--text-subtle)' }}>{row.submittedAt ? new Date(row.submittedAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : '—'}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => {
      const c = STATUS_CONFIG[row.status]
      return <span className="badge" style={{ background: c.bg, color: c.color }}>{c.label}</span>
    },
  },
  {
    key: 'waiverStatus',
    label: 'Waiver',
    render: (row) => {
      const c = WAIVER_CONFIG[row.waiverStatus]
      const icon = row.waiverStatus === 'accepted' ? 'check-circle' : 'clock'
      return <div style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '12px', color: c.color }}><Icon name={icon} size={13} />{c.label}</div>
    },
  },
  {
    key: 'swimmerCount',
    label: 'Swimmers',
    align: 'right',
    render: (row) => <span style={{ fontFamily: 'var(--font-body)', fontSize: '13px', color: 'var(--text)' }}>{row.swimmerCount}</span>,
  },
]

function deriveRows(registrations: RegistrationRecord[], households: HouseholdRecord[]): RegistrationRow[] {
  const householdById = new Map(households.map((household) => [household.id, household.household_name]))
  const grouped = registrations.reduce<Record<string, RegistrationRecord[]>>((acc, registration) => {
    const key = `${registration.household_id}:${registration.season}`
    acc[key] = [...(acc[key] ?? []), registration]
    return acc
  }, {})

  return Object.values(grouped)
    .map((group) => {
      const representative = [...group].sort((a, b) => b.registration_date.localeCompare(a.registration_date))[0]
      const activeWaiver = (group.every((registration) => registration.waiver_accepted) ? 'accepted' : 'pending') as WaiverStatus
      return {
        id: representative.id,
        household: householdById.get(representative.household_id) ?? representative.household_id,
        householdId: representative.household_id,
        season: representative.season,
        submittedAt: representative.registration_date,
        status: representative.status as RegistrationStatus,
        waiverStatus: activeWaiver,
        swimmerCount: group.length,
        notes: representative.notes || null,
      }
    })
    .sort((a, b) => b.submittedAt.localeCompare(a.submittedAt))
}

const INPUT_STYLE: React.CSSProperties = { width: '100%', padding: '9px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' }
const LABEL_STYLE: React.CSSProperties = { fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '5px' }

export function RegistrationsPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [rows, setRows] = useState<RegistrationRow[]>([])
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [groupsList, setGroupsList] = useState<GroupRecord[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [seasonFilter, setSeasonFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // New Registration modal
  const [showNew, setShowNew] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [form, setForm] = useState({ household_id: '', swimmer_id: '', group_id: '', season: new Date().getFullYear().toString() + '-' + (new Date().getFullYear() + 1).toString().slice(-2) })

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
        const [registrations, householdsData, swimmersData, groupsData] = await Promise.all([
          fetchRegistrations(api, session.user.orgId),
          fetchHouseholds(api, session.user.orgId),
          fetchSwimmers(api, session.user.orgId),
          fetchGroups(api, session.user.orgId),
        ])
        if (!cancelled) {
          setRows(deriveRows(registrations, householdsData))
          setHouseholds(householdsData)
          setSwimmers(swimmersData)
          setGroupsList(groupsData)
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load registrations.')
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

  const seasons = useMemo(() => [...new Set(rows.map((row) => row.season))].sort().reverse(), [rows])
  const filtered = useMemo(() => rows.filter((row) => {
    const query = search.toLowerCase()
    const matchesSearch = !search || row.household.toLowerCase().includes(query)
    const matchesStatus = statusFilter === 'all' || row.status === statusFilter
    const matchesSeason = seasonFilter === 'all' || row.season === seasonFilter
    return matchesSearch && matchesStatus && matchesSeason
  }), [rows, search, statusFilter, seasonFilter])

  const counts = useMemo(() => {
    const base = seasonFilter === 'all' ? rows : rows.filter((row) => row.season === seasonFilter)
    return {
      total: base.length,
      active: base.filter((row) => row.status === 'active').length,
      pending: base.filter((row) => row.status === 'pending').length,
      swimmers: base.reduce((acc, row) => acc + row.swimmerCount, 0),
    }
  }, [rows, seasonFilter])

  const selectStyle: React.CSSProperties = { padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', cursor: 'pointer' }

  const filteredSwimmers = useMemo(() => form.household_id ? swimmers.filter((s) => s.household_id === form.household_id) : swimmers, [swimmers, form.household_id])

  async function handleCreateRegistration(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId) return
    setSubmitting(true)
    setFormError(null)
    try {
      await createRegistration(api, session.user.orgId, { household_id: form.household_id, swimmer_id: form.swimmer_id, group_id: form.group_id, season: form.season })
      setShowNew(false)
      setForm({ household_id: '', swimmer_id: '', group_id: '', season: new Date().getFullYear().toString() + '-' + (new Date().getFullYear() + 1).toString().slice(-2) })
      const [registrations, householdsData] = await Promise.all([fetchRegistrations(api, session.user.orgId), fetchHouseholds(api, session.user.orgId)])
      setRows(deriveRows(registrations, householdsData))
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : 'Failed to create registration.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '16px' }}>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setShowNew(true)}>
          <Icon name="plus" size={14} /> New Registration
        </button>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Total', value: counts.total, color: 'var(--text)' },
          { label: 'Active', value: counts.active, color: 'var(--success)' },
          { label: 'Pending', value: counts.pending, color: 'var(--warning)' },
          { label: 'Swimmers', value: counts.swimmers, color: 'var(--accent)' },
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
          <input type="text" placeholder="Search households..." value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={seasonFilter} onChange={(e) => setSeasonFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Seasons</option>
          {seasons.map((season) => <option key={season} value={season}>{season}</option>)}
        </select>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Statuses</option>
          <option value="pending">Pending</option>
          <option value="active">Active</option>
          <option value="completed">Completed</option>
          <option value="rejected">Rejected</option>
          <option value="withdrawn">Withdrawn</option>
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginLeft: 'auto' }}>{filtered.length} registrations</span>
      </div>

      {loading ? (
        <EmptyState icon="clipboard-list" title="Loading registrations" description="Fetching live registration data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load registrations" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No registrations found" emptyDescription="Try adjusting your season or status filter." />
      )}

      {/* New Registration Modal */}
      {showNew && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.6)' }} onClick={(e) => { if (e.target === e.currentTarget) setShowNew(false) }}>
          <div style={{ background: 'var(--bg-card)', border: '1px solid var(--border)', borderRadius: '14px', padding: '28px', width: '100%', maxWidth: '480px', boxShadow: '0 24px 80px rgba(0,0,0,0.4)' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <h2 style={{ fontSize: '18px', fontFamily: 'var(--font-display)', color: 'var(--text)', margin: 0 }}>New Registration</h2>
              <button onClick={() => setShowNew(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)' }}><Icon name="x" size={18} /></button>
            </div>
            {formError && <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>{formError}</div>}
            <form onSubmit={handleCreateRegistration} style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div>
                <label style={LABEL_STYLE}>Household *</label>
                <select required style={INPUT_STYLE} value={form.household_id} onChange={(e) => setForm((f) => ({ ...f, household_id: e.target.value, swimmer_id: '' }))}>
                  <option value="">Select household…</option>
                  {households.map((h) => <option key={h.id} value={h.id}>{h.household_name}</option>)}
                </select>
              </div>
              <div>
                <label style={LABEL_STYLE}>Swimmer *</label>
                <select required style={INPUT_STYLE} value={form.swimmer_id} onChange={(e) => setForm((f) => ({ ...f, swimmer_id: e.target.value }))}>
                  <option value="">Select swimmer…</option>
                  {filteredSwimmers.map((s) => <option key={s.id} value={s.id}>{s.first_name} {s.last_name}</option>)}
                </select>
              </div>
              <div>
                <label style={LABEL_STYLE}>Group *</label>
                <select required style={INPUT_STYLE} value={form.group_id} onChange={(e) => setForm((f) => ({ ...f, group_id: e.target.value }))}>
                  <option value="">Select group…</option>
                  {groupsList.map((g) => <option key={g.id} value={g.id}>{g.name} ({g.code})</option>)}
                </select>
              </div>
              <div>
                <label style={LABEL_STYLE}>Season *</label>
                <input required style={INPUT_STYLE} value={form.season} onChange={(e) => setForm((f) => ({ ...f, season: e.target.value }))} placeholder="2025-26" />
              </div>
              <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '4px' }}>
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Creating…' : 'Create Registration'}</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
