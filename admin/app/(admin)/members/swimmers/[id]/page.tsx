'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useParams, useRouter } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
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

function formatDate(value?: string | null) {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function formatMs(ms?: number | null) {
  if (ms == null) return 'NT'
  const totalHundredths = Math.round(ms / 10)
  const minutes = Math.floor(totalHundredths / 6000)
  const seconds = Math.floor((totalHundredths % 6000) / 100)
  const hundredths = totalHundredths % 100
  if (minutes > 0) return `${minutes}:${seconds.toString().padStart(2, '0')}.${hundredths.toString().padStart(2, '0')}`
  return `${seconds}.${hundredths.toString().padStart(2, '0')}`
}

function computeAge(dob?: string) {
  if (!dob) return '—'
  const birth = new Date(dob)
  if (Number.isNaN(birth.getTime())) return '—'
  const now = new Date()
  let age = now.getFullYear() - birth.getFullYear()
  const birthdayPassed = now.getMonth() > birth.getMonth() || (now.getMonth() === birth.getMonth() && now.getDate() >= birth.getDate())
  if (!birthdayPassed) age -= 1
  return `${age}`
}

function StatusBadge({ status }: { status: string }) {
  const colors = status === 'active'
    ? { color: 'var(--success)', bg: 'var(--success-bg)' }
    : { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' }
  return (
    <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: colors.bg, color: colors.color, fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.02em' }}>
      {status}
    </span>
  )
}

const BT_COLS: Column<BestTimeRow>[] = [
  { key: 'event', label: 'Event', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{row.event}</span> },
  { key: 'course', label: 'Course', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)', padding: '2px 6px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-xs)', border: '1px solid var(--border)' }}>{row.course}</span> },
  { key: 'time', label: 'Best Time', align: 'right', sortable: true, render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '14px', fontWeight: 700, color: 'var(--accent)' }}>{row.time}</span> },
  { key: 'override', label: 'Source', render: (row) => <span style={{ fontSize: '12px', color: row.override ? 'var(--warning)' : 'var(--text-muted)' }}>{row.override ?? 'Imported result'}</span> },
  { key: 'updatedAt', label: 'Updated', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(row.updatedAt)}</span> },
]

const GROUP_COLS: Column<GroupMembershipRow>[] = [
  { key: 'groupName', label: 'Group', render: (row) => <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{row.groupName}</span> },
  { key: 'groupType', label: 'Type', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{row.groupType}</span> },
  { key: 'assignedAt', label: 'Assigned', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatDate(row.assignedAt)}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
]

function buildBestTimeRows(bestTimes: BestTimeRecord[]): BestTimeRow[] {
  return bestTimes
    .map((bestTime) => ({
      event: `${bestTime.event_distance} ${bestTime.stroke}`,
      course: bestTime.course,
      time: formatMs(bestTime.best_time_ms),
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
      detail: result.fin_time_ms != null ? `Finished in ${formatMs(result.fin_time_ms)}` : (result.override_reason || 'Recorded progression result'),
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
          setError(err instanceof ApiError ? err.message : 'Failed to load swimmer details.')
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
  const canDelete = session?.user.role === 'owner' || session?.user.role === 'admin'

  async function handleDeleteSwimmer() {
    if (!session?.user.orgId || !swimmer) return
    const swimmerName = `${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim()
    if (!window.confirm(`Delete swimmer "${swimmerName}"? This will only succeed if no related registrations, entries, results, or other linked records still exist.`)) {
      return
    }

    setDeleting(true)
    setDeleteError(null)
    try {
      await deleteSwimmer(api, session.user.orgId, swimmer.id)
      router.push('/members/swimmers')
    } catch (err) {
      setDeleteError(err instanceof ApiError ? err.message : 'Failed to delete swimmer.')
    } finally {
      setDeleting(false)
    }
  }

  if (loading) {
    return <div style={{ padding: '24px', maxWidth: '1100px' }}><EmptyState icon="users" title="Loading swimmer" description="Fetching live swimmer detail data from the API." /></div>
  }

  if (error || !swimmer) {
    return <div style={{ padding: '24px', maxWidth: '1100px' }}><EmptyState icon="alert-circle" title="Unable to load swimmer" description={error ?? 'Swimmer not found.'} /></div>
  }

  const swimmerName = `${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim()

  return (
    <div style={{ padding: '24px', maxWidth: '1100px' }}>
      <Link href="/members/swimmers" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}>
        <Icon name="chevron-left" size={14} /> Swimmers
      </Link>

      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
          <div style={{ width: '48px', height: '48px', borderRadius: '50%', background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: '18px', fontWeight: 700, color: 'var(--accent)', fontFamily: 'var(--font-display)', flexShrink: 0 }}>
            {swimmerName.split(' ').map((part) => part[0]).join('').slice(0, 2)}
          </div>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
              <h1 style={{ fontSize: '24px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2 }}>{swimmerName}</h1>
              <StatusBadge status={swimmer.registration_status} />
            </div>
            <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
              <span>{swimmer.usa_swimming_id || swimmer.swimmer_external_id || 'No USA-S ID'}</span>
              <span>·</span>
              <span>Age {computeAge(swimmer.date_of_birth)}</span>
              <span>·</span>
              <span>{swimmer.gender}</span>
              {household ? (
                <>
                  <span>·</span>
                  <Link href={`/members/households/${household.id}`} style={{ color: 'var(--accent)', textDecoration: 'none' }}>
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
          <button className="btn btn-danger" onClick={handleDeleteSwimmer} disabled={deleting}>
            {deleting ? 'Deleting…' : 'Delete Swimmer'}
          </button>
        ) : null}
      </div>

      {deleteError ? (
        <div className="card" style={{ padding: '14px 16px', marginBottom: '20px', borderColor: 'var(--error-border)', background: 'var(--error-bg)' }}>
          <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--error)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '4px' }}>Delete Blocked</div>
          <div style={{ fontSize: '13px', color: 'var(--text)' }}>{deleteError}</div>
        </div>
      ) : null}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'DOB', value: formatDate(swimmer.date_of_birth), color: 'var(--text)' },
          { label: 'Best Event', value: bestEvent?.event || '—', color: 'var(--accent)' },
          { label: 'Best Time', value: bestEvent?.time || 'NT', color: 'var(--accent)' },
          { label: 'Current Group', value: groupMemberships[0]?.groupName || '—', color: 'var(--text)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '16px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'profile' ? (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Swimmer Info</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[
                { label: 'USA Swimming ID', value: swimmer.usa_swimming_id || '—' },
                { label: 'External ID', value: swimmer.swimmer_external_id || '—' },
                { label: 'Date of Birth', value: formatDate(swimmer.date_of_birth) },
                { label: 'Age', value: computeAge(swimmer.date_of_birth) },
                { label: 'Gender', value: swimmer.gender },
                { label: 'Medical Flags', value: swimmer.medical_flags_redacted ? 'Restricted' : (swimmer.medical_flags?.join(', ') || 'None') },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600 }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Coach Notes</h3>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7 }}>
              {swimmer.notes || 'No coach notes recorded yet.'}
            </p>
          </div>
        </div>
      ) : null}

      {activeTab === 'performance' ? (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Best times are loaded from the live performance service for this swimmer.</p>
          </div>
          <Table columns={BT_COLS} data={bestTimes} emptyTitle="No times recorded" emptyDescription="Times will appear here after results are imported." />
        </div>
      ) : null}

      {activeTab === 'groups' ? (
        <Table columns={GROUP_COLS} data={groupMemberships} emptyTitle="No group assignments" emptyDescription="This swimmer has not been assigned to any groups." />
      ) : null}

      {activeTab === 'history' ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '0px' }}>
          {history.length ? history.map((entry, index) => (
            <div key={entry.id} style={{ display: 'flex', gap: '16px', padding: '14px 0', borderBottom: index < history.length - 1 ? '1px solid var(--border)' : 'none', alignItems: 'flex-start' }}>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)', flexShrink: 0, paddingTop: '2px', minWidth: '90px' }}>{formatDate(entry.date)}</span>
              <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--border-strong)', flexShrink: 0, marginTop: '6px' }} />
              <div>
                <div style={{ fontSize: '13px', color: 'var(--text)', lineHeight: 1.5, fontWeight: 600 }}>{entry.event}</div>
                <div style={{ fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.5 }}>{entry.detail}</div>
              </div>
            </div>
          )) : <EmptyState icon="bar-chart-2" title="No progression history" description="Progression entries will appear after results or manual performance records are added." />}
        </div>
      ) : null}
    </div>
  )
}
