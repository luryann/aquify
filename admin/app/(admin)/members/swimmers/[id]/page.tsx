'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useParams, useRouter } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Modal } from '@/components/ui/Modal'
import { Tabs } from '@/components/ui/Tabs'
import { SwimmerEngagementCard } from '@/components/engagement/SwimmerEngagementCard'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import {
  deleteSwimmer,
  fetchGroups,
  fetchHousehold,
  fetchSwimmer,
  fetchSwimmerBestTimes,
  fetchSwimmerGroups,
  fetchSwimmerProgression,
  type BestTimeRecord,
  type GroupRecord,
  type HouseholdRecord,
  type ProgressionResultRecord,
  type SwimmerGroupLinkRecord,
  type SwimmerRecord,
} from '@/lib/adminData'
import { ageFromDob, formatDateOnly } from '@/lib/dates'
import { formatSwimTime } from '@/lib/performanceData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

type BestTimeRow = {
  event: string
  course: string
  time: string
  updatedAt: string
  override: string | null
}

type GroupMembershipRow = {
  id: string
  groupName: string
  groupType: string
  assignedAt: string
  status: string
}

function StatusBadge({ status }: { status: string }) {
  const colors = status === 'active'
    ? { color: 'var(--success)', bg: 'var(--success-bg)' }
    : { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' }
  return (
    <span className="badge" style={{ background: colors.bg, color: colors.color }}>
      {status}
    </span>
  )
}

const BT_COLS: Column<BestTimeRow>[] = [
  { key: 'event', label: 'Event', render: (row) => <span className="u-ff-font-body u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{row.event}</span> },
  { key: 'course', label: 'Course', render: (row) => <span className="u-ff-font-body u-fs-fs-sm u-p-2px-6px" style={{ color: 'var(--text-subtle)', background: 'var(--bg-elevated)', borderRadius: 'var(--r-xs)', border: '1px solid var(--border)' }}>{row.course}</span> },
  { key: 'time', label: 'Best time', align: 'right', sortable: true, render: (row) => <span className="u-ff-font-body u-fs-fs-md u-fw-700" style={{ color: 'var(--accent-text, var(--accent))' }}>{row.time}</span> },
  { key: 'override', label: 'Source', render: (row) => <span className="u-fs-fs-sm" style={{ color: row.override ? 'var(--warning)' : 'var(--text-muted)' }}>{row.override ?? 'Imported result'}</span> },
  { key: 'updatedAt', label: 'Updated', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{formatDateOnly(row.updatedAt)}</span> },
]

const GROUP_COLS: Column<GroupMembershipRow>[] = [
  { key: 'groupName', label: 'Group', render: (row) => <span className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{row.groupName}</span> },
  { key: 'groupType', label: 'Type', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{row.groupType}</span> },
  { key: 'assignedAt', label: 'Assigned', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{formatDateOnly(row.assignedAt)}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
]

function buildBestTimeRows(bestTimes: BestTimeRecord[]): BestTimeRow[] {
  return bestTimes
    .map((bestTime) => ({
      event: `${bestTime.event_distance} ${bestTime.stroke}`,
      course: bestTime.course,
      time: formatSwimTime(bestTime.best_time_ms),
      updatedAt: bestTime.recorded_at || bestTime.updated_at,
      override: bestTime.is_manual_override ? (bestTime.override_reason || 'Manual override') : null,
    }))
    .sort((a, b) => a.event.localeCompare(b.event))
}

function buildGroupRows(groups: GroupRecord[], links: SwimmerGroupLinkRecord[]): GroupMembershipRow[] {
  const groupByCode = new Map(groups.map((group) => [group.code, group]))
  return links
    .map((link) => {
      const group = groupByCode.get(link.group_code)
      return {
        id: link.id,
        groupName: group?.name || link.group_code,
        groupType: group?.type || 'unknown',
        assignedAt: link.assigned_at || '',
        status: group?.is_active ? 'active' : 'archived',
      }
    })
    .sort((a, b) => a.groupName.localeCompare(b.groupName))
}

function buildHistoryRows(results: ProgressionResultRecord[]) {
  return results
    .map((result) => ({
      id: result.id,
      date: result.updated_at || result.created_at,
      event: `${result.distance} ${result.stroke} ${result.course}`,
      detail: result.fin_time_ms != null ? `Finished in ${formatSwimTime(result.fin_time_ms)}` : (result.override_reason || 'Recorded progression result'),
    }))
    .sort((a, b) => b.date.localeCompare(a.date))
}

export default function SwimmerDetailPage() {
  const params = useParams<{ id: string }>()
  const router = useRouter()
  const swimmerId = typeof params?.id === 'string' ? params.id : ''
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [activeTab, setActiveTab] = useState('profile')
  const [swimmer, setSwimmer] = useState<SwimmerRecord | null>(null)
  const [household, setHousehold] = useState<HouseholdRecord | null>(null)
  const [bestTimes, setBestTimes] = useState<BestTimeRow[]>([])
  const [groupMemberships, setGroupMemberships] = useState<GroupMembershipRow[]>([])
  const [history, setHistory] = useState<Array<{ id: string; date: string; event: string; detail: string }>>([])
  const [loading, setLoading] = useState(true)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [deleteNameConfirm, setDeleteNameConfirm] = useState('')

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!session?.user.orgId || !swimmerId) {
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        setError(null)
        const nextSwimmer = await fetchSwimmer(api, session.user.orgId, swimmerId)
        const [nextHousehold, groups, swimmerGroups, nextBestTimes, progression] = await Promise.all([
          fetchHousehold(api, session.user.orgId, nextSwimmer.household_id).catch((err) => {
            if (err instanceof ApiError && err.isNotFound()) return null
            throw err
          }),
          fetchGroups(api, session.user.orgId),
          fetchSwimmerGroups(api, session.user.orgId),
          fetchSwimmerBestTimes(api, session.user.orgId, swimmerId),
          fetchSwimmerProgression(api, session.user.orgId, swimmerId),
        ])

        if (cancelled) return

        setSwimmer(nextSwimmer)
        setHousehold(nextHousehold)
        setBestTimes(buildBestTimeRows(nextBestTimes))
        setGroupMemberships(buildGroupRows(groups, swimmerGroups.filter((link) => link.swimmer_id === swimmerId)))
        setHistory(buildHistoryRows(progression))
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load swimmer details. Try again.'))
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId, swimmerId])

  const tabs = useMemo(() => [
    { id: 'profile', label: 'Profile' },
    { id: 'performance', label: 'Performance', count: bestTimes.length },
    { id: 'groups', label: 'Groups', count: groupMemberships.length },
    { id: 'history', label: 'History', count: history.length },
  ], [bestTimes.length, groupMemberships.length, history.length])

  const bestEvent = bestTimes[0]
  const canDelete = session?.user.roles?.includes('admin') ?? false

  async function handleDeleteSwimmer() {
    if (!session?.user.orgId || !swimmer) return

    setDeleting(true)
    setDeleteError(null)
    try {
      await deleteSwimmer(api, session.user.orgId, swimmer.id)
      router.push('/members?tab=swimmers')
    } catch (err) {
      setDeleteError(uiErrorMessage(err, 'Couldn’t delete swimmer. Try again.'))
      setShowDeleteConfirm(false)
    } finally {
      setDeleting(false)
    }
  }

  if (loading) {
    return <div className="page"><EmptyState icon="users" title="Loading swimmer" description="Fetching live swimmer detail data from the API." /></div>
  }

  if (error || !swimmer) {
    return <div className="page"><EmptyState icon="alert-circle" title="Unable to load swimmer" description={error ?? 'Swimmer not found.'} /></div>
  }

  const swimmerName = `${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim()

  return (
    <div className="page">
      <Link href="/members?tab=swimmers" className="u-gap-4px u-fs-fs-base u-mb-20px" style={{ display: 'inline-flex', alignItems: 'center', color: 'var(--text-subtle)', textDecoration: 'none' }}>
        <Icon name="chevron-left" size={14} /> Swimmers
      </Link>

      <div className="page-header">
        <div className="u-gap-16px" style={{ display: 'flex', alignItems: 'center' }}>
          <div className="u-fs-fs-xl u-fw-display-weight u-ff-font-display" style={{ width: '48px', height: '48px', borderRadius: '50%', background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--accent-text, var(--accent))', flexShrink: 0 }}>
            {swimmerName.split(' ').map((part) => part[0]).join('').slice(0, 2)}
          </div>
          <div>
            <div className="u-gap-6px-12px u-mb-4px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
              <h1 className="page-title">{swimmerName}</h1>
              <StatusBadge status={swimmer.registration_status} />
            </div>
            <div className="u-fs-fs-sm u-ff-font-body u-gap-12px" style={{ color: 'var(--text-muted)', display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
              <span>{swimmer.usa_swimming_id || swimmer.swimmer_external_id || 'No USA-S ID'}</span>
              <span>·</span>
              <span>Age {ageFromDob(swimmer.date_of_birth)}</span>
              <span>·</span>
              <span>{swimmer.gender}</span>
              {household ? (
                <>
                  <span>·</span>
                  <Link href={`/members/households/${household.id}`} style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none' }}>
                    {household.household_name}
                  </Link>
                </>
              ) : (
                <>
                  <span>·</span>
                  <span style={{ color: 'var(--warning)' }}>Household deleted</span>
                </>
              )}
            </div>
          </div>
        </div>
        {canDelete ? (
          <button type="button" className="btn btn-danger" onClick={() => { setShowDeleteConfirm(true); setDeleteNameConfirm(''); setDeleteError(null) }} disabled={deleting}>
            Delete Swimmer
          </button>
        ) : null}
      </div>

      <Modal open={Boolean(showDeleteConfirm)} onClose={() => setShowDeleteConfirm(false)} title="Delete swimmer" width={460}>
<p className="u-fs-fs-base u-mb-8px u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>
              This permanently removes <strong style={{ color: 'var(--text)' }}>{swimmerName}</strong> from the roster. It only succeeds if no registrations, meet entries, results, best times, document submissions, or invoice items are still linked to this swimmer — otherwise the delete will be blocked and those dependencies will be listed. This action <strong style={{ color: 'var(--error)' }}>cannot be undone</strong>.
            </p>
<p className="u-fs-fs-sm u-mb-16px" style={{ color: 'var(--text-muted)' }}>
              Type the swimmer&apos;s name to confirm:
            </p>
<input
              type="text"
              className="u-mb-16px" style={{ width: '100%' }}
              placeholder={swimmerName}
              value={deleteNameConfirm}
              onChange={(e) => setDeleteNameConfirm(e.target.value)}
            />
<div className="u-gap-8px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button type="button" className="btn u-fs-fs-base" onClick={() => setShowDeleteConfirm(false)}>Cancel</button>
              <button type="button"
                className="btn u-fs-fs-base"
                style={{ background: 'var(--error)', color: '#fff', border: 'none' }}
                disabled={deleteNameConfirm !== swimmerName || deleting}
                onClick={() => { void handleDeleteSwimmer() }}
              >
                {deleting ? 'Deleting…' : 'Delete permanently'}
              </button>
            </div>
</Modal>

      {deleteError ? (
        <div className="card u-p-16px-16px u-mb-20px" style={{ borderColor: 'var(--error-border)', background: 'var(--error-bg)' }}>
          <div className="u-fs-fs-sm u-fw-600 u-ff-font-body u-mb-4px" style={{ color: 'var(--error)' }}>Delete blocked</div>
          <div className="u-fs-fs-base" style={{ color: 'var(--text)' }}>{deleteError}</div>
        </div>
      ) : null}

      <div className="cols-4 u-gap-12px u-mb-24px" >
        {[
          { label: 'DOB', value: formatDateOnly(swimmer.date_of_birth), color: 'var(--text)' },
          { label: 'Best event', value: bestEvent?.event || '—', color: 'var(--accent-text, var(--accent))' },
          { label: 'Best time', value: bestEvent?.time || 'No time yet', color: 'var(--accent-text, var(--accent))' },
          { label: 'Current group', value: groupMemberships[0]?.groupName || '—', color: 'var(--text)' },
        ].map((stat) => (
          <div key={stat.label} className="card u-p-16px-16px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</div>
            <div className="u-fs-fs-lg u-ff-font-body u-fw-700" style={{ color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <SwimmerEngagementCard api={api} orgId={session?.user.orgId ?? ''} swimmerId={swimmerId} />

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'profile' ? (
        <div className="cols-2 u-gap-16px" >
          <div className="card u-p-20px" >
            <h3 className="card-title u-mb-16px" >Profile</h3>
            <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
              {[
                { label: 'USA Swimming ID', value: swimmer.usa_swimming_id || '—' },
                { label: 'External ID', value: swimmer.swimmer_external_id || '—' },
                { label: 'Date of birth', value: formatDateOnly(swimmer.date_of_birth) },
                { label: 'Age', value: ageFromDob(swimmer.date_of_birth) },
                { label: 'Gender', value: swimmer.gender },
                { label: 'Medical flags', value: swimmer.medical_flags_redacted ? 'Hidden from you (medical staff only)' : (swimmer.medical_flags?.join(', ') || 'None') },
              ].map((row) => (
                <div key={row.label} className="u-fs-fs-base u-pb-12px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--border)' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span className="u-ff-font-body u-fw-600" style={{ color: 'var(--text)' }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="card u-p-20px" >
            <h3 className="card-title u-mb-16px" >Coach notes</h3>
            <p className="u-fs-fs-base u-lh-1_7" style={{ color: 'var(--text-subtle)' }}>
              {swimmer.notes || 'No coach notes recorded yet.'}
            </p>
          </div>
        </div>
      ) : null}

      {activeTab === 'performance' ? (
        <div>
          <Table columns={BT_COLS} data={bestTimes} emptyTitle="No times recorded" emptyDescription="Times will appear here after results are imported." />
        </div>
      ) : null}

      {activeTab === 'groups' ? (
        <Table columns={GROUP_COLS} data={groupMemberships} emptyTitle="No group assignments" emptyDescription="This swimmer has not been assigned to any groups." />
      ) : null}

      {activeTab === 'history' ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '0px' }}>
          {history.length ? history.map((entry, index) => (
            <div key={entry.id} className="u-gap-16px u-p-16px-0" style={{ display: 'flex', borderBottom: index < history.length - 1 ? '1px solid var(--border)' : 'none', alignItems: 'flex-start' }}>
              <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)', flexShrink: 0, paddingTop: '2px', minWidth: '90px' }}>{formatDateOnly(entry.date)}</span>
              <div className="u-mt-6px" style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--border-strong)', flexShrink: 0 }} />
              <div>
                <div className="u-fs-fs-base u-lh-1_5 u-fw-600" style={{ color: 'var(--text)' }}>{entry.event}</div>
                <div className="u-fs-fs-sm u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>{entry.detail}</div>
              </div>
            </div>
          )) : <EmptyState icon="bar-chart-2" title="No progression history" description="Progression entries will appear after results or manual performance records are added." />}
        </div>
      ) : null}
    </div>
  )
}
