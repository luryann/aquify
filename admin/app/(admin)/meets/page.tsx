'use client'

import { Suspense, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { Tabs } from '@/components/ui/Tabs'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchMeetCommitments, fetchMeetEntries, fetchMeetEvents, fetchMeets, type MeetRecord } from '@/lib/adminData'
import { canWrite } from '@/lib/roles'

type MeetStatus = 'draft' | 'published' | 'commitment_open' | 'commitment_closed' | 'entry_finalized' | 'completed' | 'archived'
type MeetCourse = 'SCY' | 'SCM' | 'LCM'

type MeetRow = {
  id: string
  name: string
  host: string
  location: string
  startDate: string
  endDate: string
  status: MeetStatus
  course: MeetCourse
  entries: number
  commitments: number
  events: number
}

const STATUS_CONFIG: Record<MeetStatus, { label: string; color: string; bg: string }> = {
  draft: { label: 'Draft', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  published: { label: 'Published', color: 'var(--info)', bg: 'var(--info-bg)' },
  commitment_open: { label: 'Commitments Open', color: 'var(--warning)', bg: 'var(--warning-bg)' },
  commitment_closed: { label: 'Commitments Closed', color: 'var(--warning)', bg: 'var(--warning-bg)' },
  entry_finalized: { label: 'Entries Finalized', color: 'var(--accent)', bg: 'var(--accent-subtle)' },
  completed: { label: 'Completed', color: 'var(--success)', bg: 'var(--success-bg)' },
  archived: { label: 'Archived', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

function StatusBadge({ status }: { status: MeetStatus }) {
  const c = STATUS_CONFIG[status]
  return <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em', whiteSpace: 'nowrap' }}>{c.label}</span>
}

function CourseBadge({ course }: { course: MeetCourse }) {
  const colors: Record<MeetCourse, string> = { SCY: 'var(--text-subtle)', SCM: 'var(--info)', LCM: 'var(--accent)' }
  return <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', padding: '2px 6px', borderRadius: 'var(--r-xs)', border: '1px solid var(--border)', color: colors[course], background: 'var(--bg-elevated)' }}>{course}</span>
}

const COLUMNS: Column<MeetRow>[] = [
  {
    key: 'name',
    label: 'Meet',
    sortable: true,
    render: (row) => (
      <div>
        <Link href={`/meets/${row.id}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}>{row.name}</Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>{row.host} · {row.location || 'Location TBD'}</div>
      </div>
    ),
  },
  {
    key: 'startDate',
    label: 'Dates',
    sortable: true,
    render: (row) => (
      <div style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>
        <div>{row.startDate || '—'}</div>
        {row.endDate && row.endDate !== row.startDate && <div style={{ color: 'var(--text-muted)' }}>→ {row.endDate}</div>}
      </div>
    ),
  },
  { key: 'course', label: 'Course', render: (row) => <CourseBadge course={row.course} /> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
  { key: 'events', label: 'Events', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: row.events > 0 ? 'var(--text)' : 'var(--text-muted)' }}>{row.events || '—'}</span> },
  { key: 'commitments', label: 'Commitments', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: row.commitments > 0 ? 'var(--text)' : 'var(--text-muted)' }}>{row.commitments || '—'}</span> },
  { key: 'entries', label: 'Entries', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', fontWeight: row.entries > 0 ? 600 : 400, color: row.entries > 0 ? 'var(--accent)' : 'var(--text-muted)' }}>{row.entries || '—'}</span> },
]

const ACTIVE_STATUSES: MeetStatus[] = ['draft', 'published', 'commitment_open', 'commitment_closed', 'entry_finalized']
const PAST_STATUSES: MeetStatus[] = ['completed', 'archived']

async function buildMeetRows(api: ReturnType<typeof useApiClient>, orgId: string, meets: MeetRecord[]): Promise<MeetRow[]> {
  const details = await Promise.all(meets.map(async (meet) => {
    const [events, commitments, entries] = await Promise.all([
      fetchMeetEvents(api, orgId, meet.id).catch(() => []),
      fetchMeetCommitments(api, orgId, meet.id).catch(() => []),
      fetchMeetEntries(api, orgId, meet.id).catch(() => []),
    ])

    return {
      id: meet.id,
      name: meet.name,
      host: meet.host_org || 'Host TBD',
      location: meet.location || '',
      startDate: meet.start_date || '',
      endDate: meet.end_date || meet.start_date || '',
      status: meet.status as MeetStatus,
      course: (meet.course as MeetCourse) || 'SCY',
      events: events.length,
      commitments: commitments.length,
      entries: entries.length,
    }
  }))

  return details.sort((a, b) => (b.startDate || '').localeCompare(a.startDate || ''))
}

export default function MeetsPage() {
  return (
    <Suspense fallback={null}>
      <MeetsPageContent />
    </Suspense>
  )
}

function MeetsPageContent() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const allowWrite = canWrite(session?.user.roles ?? [])
  const searchParams = useSearchParams()
  const [rows, setRows] = useState<MeetRow[]>([])
  const [viewTab, setViewTab] = useState('upcoming')
  const [search, setSearch] = useState('')
  const [courseFilter, setCourseFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const createdNotice = searchParams.get('created')

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
        const meets = await fetchMeets(api, session.user.orgId)
        const builtRows = await buildMeetRows(api, session.user.orgId, meets)
        if (!cancelled) setRows(builtRows)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load meets.')
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

  const viewTabs = [
    { id: 'upcoming', label: 'Upcoming / Active', count: rows.filter((row) => ACTIVE_STATUSES.includes(row.status)).length },
    { id: 'past', label: 'Past', count: rows.filter((row) => PAST_STATUSES.includes(row.status)).length },
    { id: 'all', label: 'All', count: rows.length },
  ]

  const filtered = useMemo(() => rows.filter((row) => {
    const query = search.toLowerCase()
    const matchesSearch = !search || row.name.toLowerCase().includes(query) || row.host.toLowerCase().includes(query) || row.location.toLowerCase().includes(query)
    const matchesCourse = courseFilter === 'all' || row.course === courseFilter
    const matchesView = viewTab === 'all' || (viewTab === 'upcoming' ? ACTIVE_STATUSES.includes(row.status) : PAST_STATUSES.includes(row.status))
    return matchesSearch && matchesCourse && matchesView
  }), [rows, search, courseFilter, viewTab])

  const totalEntries = rows.reduce((acc, row) => acc + row.entries, 0)

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Meets</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Live meet operations from persisted backend state</p>
        </div>
        {allowWrite && (
          <Link href="/meets/new" className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px', textDecoration: 'none' }}>
            <Icon name="plus" size={14} /> New Meet
          </Link>
        )}
      </div>

      {createdNotice === '1' && (
        <div className="card" style={{ padding: '14px 16px', marginBottom: '20px', borderColor: 'var(--success-border)', background: 'var(--success-bg)' }}>
          <div style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--success)', marginBottom: '4px', letterSpacing: '0.04em' }}>LIVE DATA ENABLED</div>
          <div style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.6 }}>The meet list now reloads from persisted API data. Newly created meets appear once the create flow posts to the backend.</div>
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Active Meets', value: rows.filter((row) => ACTIVE_STATUSES.includes(row.status)).length, color: 'var(--text)' },
          { label: 'Open Commitments', value: rows.filter((row) => row.status === 'commitment_open').length, color: 'var(--warning)' },
          { label: 'Finalized', value: rows.filter((row) => row.status === 'entry_finalized').length, color: 'var(--accent)' },
          { label: 'Total Entries (Season)', value: totalEntries, color: 'var(--success)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={viewTabs} active={viewTab} onChange={setViewTab} />

      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '320px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search meets..." value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={courseFilter} onChange={(e) => setCourseFilter(e.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', cursor: 'pointer' }}>
          <option value="all">All Courses</option>
          <option value="SCY">SCY</option>
          <option value="SCM">SCM</option>
          <option value="LCM">LCM</option>
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>{filtered.length} meets</span>
      </div>

      {loading ? (
        <EmptyState icon="calendar" title="Loading meets" description="Fetching live meet data from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load meets" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No meets found" emptyDescription="Try adjusting your filters or create a new meet." />
      )}
    </div>
  )
}
