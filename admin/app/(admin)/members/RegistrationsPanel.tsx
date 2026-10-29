'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchHouseholds, fetchRegistrations, fetchSwimmers, fetchGroups, createRegistration, transitionRegistration, type HouseholdRecord, type RegistrationRecord, type SwimmerRecord, type GroupRecord } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { formatDateOnly } from '@/lib/dates'
import { useConfirm } from '@/components/ui/ConfirmDialog'
import { Modal } from '@/components/ui/Modal'

type RegistrationStatus = 'pending' | 'active' | 'inactive' | 'completed' | 'rejected' | 'withdrawn'
type WaiverStatus = 'pending' | 'accepted'

type RegistrationRow = {
  id: string
  household: string
  householdId: string
  swimmer: string
  swimmerId: string
  program: string
  groupId: string
  season: string
  submittedAt: string
  status: RegistrationStatus
  waiverStatus: WaiverStatus
  notes: string | null
}

// Mirrors server/internal/app/groups/service.go's validTransitions map.
const VALID_TRANSITIONS: Record<string, string[]> = {
  pending: ['active', 'withdrawn'],
  active: ['inactive', 'withdrawn'],
  inactive: ['active', 'withdrawn'],
}

// Button labels are the action, not the destination status.
const TRANSITION_LABELS: Record<string, Record<string, string>> = {
  pending: { active: 'Approve', withdrawn: 'Withdraw' },
  active: { inactive: 'Deactivate', withdrawn: 'Withdraw' },
  inactive: { active: 'Reactivate', withdrawn: 'Withdraw' },
}

const STATUS_CONFIG: Record<RegistrationStatus, { label: string; color: string; bg: string }> = {
  pending: { label: 'Pending', color: 'var(--warning)', bg: 'var(--warning-bg)' },
  active: { label: 'Active', color: 'var(--success)', bg: 'var(--success-bg)' },
  inactive: { label: 'Inactive', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  completed: { label: 'Completed', color: 'var(--info)', bg: 'var(--info-bg)' },
  rejected: { label: 'Rejected', color: 'var(--error)', bg: 'var(--error-bg)' },
  withdrawn: { label: 'Withdrawn', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

const WAIVER_CONFIG: Record<WaiverStatus, { label: string; color: string }> = {
  pending: { label: 'Pending', color: 'var(--warning)' },
  accepted: { label: 'Signed', color: 'var(--success)' },
}

function buildColumns(
  onTransition: (row: RegistrationRow, toStatus: string) => void,
  transitioningId: string | null,
): Column<RegistrationRow>[] {
  return [
  {
    key: 'swimmer',
    label: 'Swimmer',
    sortable: true,
    render: (row) => <Link href={`/members/swimmers/${row.swimmerId}`} className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)', textDecoration: 'none' }}>{row.swimmer}</Link>,
  },
  {
    key: 'household',
    label: 'Household',
    sortable: true,
    render: (row) => <Link href={`/members/households/${row.householdId}`} className="u-fs-fs-base" style={{ color: 'var(--text-subtle)', textDecoration: 'none' }}>{row.household}</Link>,
  },
  {
    key: 'program',
    label: 'Program',
    sortable: true,
    render: (row) => <Link href={`/members/groups/${row.groupId}`} className="u-fs-fs-base" style={{ color: 'var(--text-subtle)', textDecoration: 'none' }}>{row.program}</Link>,
  },
  {
    key: 'season',
    label: 'Season',
    render: (row) => <span className="u-ff-font-body u-fs-fs-sm u-p-2px-8px" style={{ color: 'var(--text-subtle)', background: 'var(--bg-elevated)', borderRadius: 'var(--r-xs)', border: '1px solid var(--border)' }}>{row.season}</span>,
  },
  {
    key: 'submittedAt',
    label: 'Submitted',
    sortable: true,
    render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{formatDateOnly(row.submittedAt)}</span>,
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
      return <div className="u-gap-4px u-fs-fs-sm" style={{ display: 'flex', alignItems: 'center', color: c.color }}><Icon name={icon} size={13} />{c.label}</div>
    },
  },
  {
    key: 'id',
    label: '',
    render: (row) => {
      const options = VALID_TRANSITIONS[row.status] ?? []
      if (options.length === 0) return null
      const busy = transitioningId === row.id
      return (
        <div className="u-gap-6px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
          {options.map((toStatus) => (
            <button type="button"
              key={toStatus}
              className="btn btn-sm btn-secondary"
              disabled={busy}
              onClick={() => onTransition(row, toStatus)}
              title={`Move ${row.swimmer}'s registration to ${toStatus}`}
            >
              {busy ? '…' : TRANSITION_LABELS[row.status]?.[toStatus] ?? toStatus}
            </button>
          ))}
        </div>
      )
    },
  },
  ]
}

function deriveRows(
  registrations: RegistrationRecord[],
  households: HouseholdRecord[],
  swimmers: SwimmerRecord[],
  groups: GroupRecord[],
): RegistrationRow[] {
  const householdById = new Map(households.map((household) => [household.id, household.household_name]))
  const swimmerById = new Map(swimmers.map((swimmer) => [swimmer.id, `${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim()]))
  const groupById = new Map(groups.map((group) => [group.id, group.name]))
  return registrations
    .map((registration) => ({
      id: registration.id,
      household: householdById.get(registration.household_id) ?? 'Unknown household',
      householdId: registration.household_id,
      swimmer: swimmerById.get(registration.swimmer_id) ?? 'Unknown swimmer',
      swimmerId: registration.swimmer_id,
      program: groupById.get(registration.group_id) ?? 'Unknown program',
      groupId: registration.group_id,
      season: registration.season,
      submittedAt: registration.registration_date || registration.created_at || '',
      status: registration.status as RegistrationStatus,
      waiverStatus: (registration.waiver_accepted ? 'accepted' : 'pending') as WaiverStatus,
      notes: registration.notes || null,
    }))
    .sort((a, b) => b.submittedAt.localeCompare(a.submittedAt))
}

const INPUT_STYLE: React.CSSProperties = { width: '100%' }

function emptyForm() {
  const year = new Date().getFullYear()
  return { household_id: '', swimmer_id: '', group_id: '', season: `${year}-${(year + 1).toString().slice(-2)}` }
}

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
  const [form, setForm] = useState(emptyForm)

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
          setRows(deriveRows(registrations, householdsData, swimmersData, groupsData))
          setHouseholds(householdsData)
          setSwimmers(swimmersData)
          setGroupsList(groupsData)
        }
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load registrations. Try again.'))
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
    const matchesSearch = !search || row.household.toLowerCase().includes(query) || row.swimmer.toLowerCase().includes(query) || row.program.toLowerCase().includes(query)
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
      swimmers: new Set(base.map((row) => row.swimmerId)).size,
    }
  }, [rows, seasonFilter])

  const filteredSwimmers = useMemo(() => form.household_id ? swimmers.filter((s) => s.household_id === form.household_id) : swimmers, [swimmers, form.household_id])

  const [transitioningId, setTransitioningId] = useState<string | null>(null)
  const [transitionError, setTransitionError] = useState<string | null>(null)

  const confirm = useConfirm()
  const handleTransition = useCallback(async (row: RegistrationRow, toStatus: string) => {
    const orgId = session?.user.orgId
    if (!orgId) return
    const label = TRANSITION_LABELS[row.status]?.[toStatus] ?? toStatus
    if (!(await confirm({ title: `${label} registration?`, message: `${row.swimmer}'s ${row.program} registration will change status.`, confirmLabel: label }))) return
    setTransitioningId(row.id)
    setTransitionError(null)
    try {
      await transitionRegistration(api, orgId, row.id, toStatus)
    } catch (err) {
      setTransitionError(uiErrorMessage(err, 'Couldn’t update registration. Try again.'))
    }
    try {
      const registrations = await fetchRegistrations(api, orgId)
      setRows(deriveRows(registrations, households, swimmers, groupsList))
    } catch {
      // Keep the existing rows if the refresh fails; the transition itself already ran.
    } finally {
      setTransitioningId(null)
    }
  }, [api, session?.user.orgId, households, swimmers, groupsList, confirm])

  const columns = useMemo(() => buildColumns(handleTransition, transitioningId), [handleTransition, transitioningId])

  async function handleCreateRegistration(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId) return
    setSubmitting(true)
    setFormError(null)
    try {
      await createRegistration(api, session.user.orgId, { household_id: form.household_id, swimmer_id: form.swimmer_id, group_id: form.group_id, season: form.season })
      setShowNew(false)
      setForm(emptyForm())
      const registrations = await fetchRegistrations(api, session.user.orgId)
      setRows(deriveRows(registrations, households, swimmers, groupsList))
    } catch (err) {
      setFormError(uiErrorMessage(err, 'Couldn’t create registration. Try again.'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <div className="u-mb-16px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <button type="button" className="btn btn-primary u-gap-6px" style={{ display: 'flex', alignItems: 'center' }} onClick={() => setShowNew(true)}>
          <Icon name="plus" size={14} /> New registration
        </button>
      </div>

      <div className="cols-4 u-gap-12px u-mb-20px" >
        {[
          { label: 'Total', value: counts.total, color: 'var(--text)' },
          { label: 'Active', value: counts.active, color: 'var(--success)' },
          { label: 'Pending', value: counts.pending, color: 'var(--warning)' },
          { label: 'Swimmers', value: counts.swimmers, color: 'var(--accent-text, var(--accent))' },
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
          <input aria-label="Search households..." type="text" placeholder="Search households..." value={search} onChange={(e) => setSearch(e.target.value)} className="u-p-8px-12px-8px-32px u-fs-fs-base" style={{ width: '100%' }} />
        </div>
        <select value={seasonFilter} onChange={(e) => setSeasonFilter(e.target.value)}>
          <option value="all">All seasons</option>
          {seasons.map((season) => <option key={season} value={season}>{season}</option>)}
        </select>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
          <option value="all">All statuses</option>
          <option value="pending">Pending</option>
          <option value="active">Active</option>
          <option value="completed">Completed</option>
          <option value="rejected">Rejected</option>
          <option value="withdrawn">Withdrawn</option>
        </select>
        <span className="u-fs-fs-sm u-ff-font-body u-ml-auto" style={{ color: 'var(--text-muted)' }}>{filtered.length} registrations</span>
      </div>

      {transitionError && (
        <div className="u-p-12px-16px u-mb-16px u-fs-fs-base" style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px' }}>
          {transitionError}
        </div>
      )}

      {loading ? (
        <EmptyState icon="clipboard-list" title="Loading registrations" description="Fetching live registration data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load registrations" description={error} />
      ) : (
        <Table columns={columns} data={filtered} pageSize={10} emptyTitle="No registrations found" emptyDescription="Try adjusting your season or status filter." />
      )}

      {/* New Registration Modal */}
      <Modal open={Boolean(showNew)} onClose={() => setShowNew(false)} title="New registration" width={480}>
            {formError && <div role="alert" className="form-banner-error u-mb-16px">{formError}</div>}
            <form onSubmit={handleCreateRegistration} className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div>
                <label className="form-label form-label-required">Household</label>
                <select required style={INPUT_STYLE} value={form.household_id} onChange={(e) => setForm((f) => ({ ...f, household_id: e.target.value, swimmer_id: '' }))}>
                  <option value="">Select household…</option>
                  {households.map((h) => <option key={h.id} value={h.id}>{h.household_name}</option>)}
                </select>
              </div>
              <div>
                <label className="form-label form-label-required">Swimmer</label>
                <select required style={INPUT_STYLE} value={form.swimmer_id} onChange={(e) => setForm((f) => ({ ...f, swimmer_id: e.target.value }))}>
                  <option value="">Select swimmer…</option>
                  {filteredSwimmers.map((s) => <option key={s.id} value={s.id}>{s.first_name} {s.last_name}</option>)}
                </select>
              </div>
              <div>
                <label className="form-label form-label-required">Group</label>
                <select required style={INPUT_STYLE} value={form.group_id} onChange={(e) => setForm((f) => ({ ...f, group_id: e.target.value }))}>
                  <option value="">Select group…</option>
                  {groupsList.map((g) => <option key={g.id} value={g.id}>{g.name} ({g.code})</option>)}
                </select>
              </div>
              <div>
                <label className="form-label form-label-required">Season</label>
                <input aria-label="2025-26" required style={INPUT_STYLE} value={form.season} onChange={(e) => setForm((f) => ({ ...f, season: e.target.value }))} placeholder="2025-26" />
              </div>
              <div className="form-actions">
                <button type="button" className="btn" onClick={() => setShowNew(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? 'Creating…' : 'Create Registration'}</button>
              </div>
            </form>
      </Modal>
    </div>
  )
}
