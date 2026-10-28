'use client'

import { Suspense, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { Tabs } from '@/components/ui/Tabs'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchMeetCommitments, fetchMeetEntries, fetchMeetEvents, fetchMeets, type MeetRecord } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

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
  commitment_open: { label: 'Commitments open', color: 'var(--warning)', bg: 'var(--warning-bg)' },
  commitment_closed: { label: 'Commitments closed', color: 'var(--warning)', bg: 'var(--warning-bg)' },
  entry_finalized: { label: 'Entries finalized', color: 'var(--accent-text, var(--accent))', bg: 'var(--accent-subtle)' },
  completed: { label: 'Completed', color: 'var(--success)', bg: 'var(--success-bg)' },
  archived: { label: 'Archived', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

function StatusBadge({ status }: { status: MeetStatus }) {
  const c = STATUS_CONFIG[status]
  return <span className="badge" style={{ background: c.bg, color: c.color }}>{c.label}</span>
}

const COURSE_LABELS: Record<MeetCourse, string> = {
  SCY: 'Short Course Yards',
  SCM: 'Short Course Meters',
  LCM: 'Long Course Meters',
}

function CourseBadge({ course }: { course: MeetCourse }) {
  const colors: Record<MeetCourse, string> = { SCY: 'var(--text-subtle)', SCM: 'var(--info)', LCM: 'var(--accent)' }
  return <span title={COURSE_LABELS[course]} className="u-ff-font-body u-fs-fs-xs u-p-2px-6px" style={{ borderRadius: 'var(--r-xs)', border: '1px solid var(--border)', color: colors[course], background: 'var(--bg-elevated)' }}>{course}</span>
}

const COLUMNS: Column<MeetRow>[] = [
  {
    key: 'name',
    label: 'Meet',
    sortable: true,
    render: (row) => (
      <div>
        <Link href={`/meets/${row.id}`} className="u-fw-600 u-fs-fs-base" style={{ color: 'var(--text)', textDecoration: 'none' }}>{row.name}</Link>
        <div className="u-fs-fs-xs u-mt-2px" style={{ color: 'var(--text-muted)' }}>{row.host} · {row.location || 'Location TBD'}</div>
      </div>
    ),
  },
  {
    key: 'startDate',
    label: 'Dates',
    sortable: true,
    render: (row) => (
      <div className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
        <div>{row.startDate || '—'}</div>
        {row.endDate && row.endDate !== row.startDate && <div style={{ color: 'var(--text-muted)' }}>→ {row.endDate}</div>}
      </div>
    ),
  },
  { key: 'course', label: 'Course', render: (row) => <CourseBadge course={row.course} /> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
  { key: 'events', label: 'Events', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: row.events > 0 ? 'var(--text)' : 'var(--text-muted)' }}>{row.events || '—'}</span> },
  { key: 'commitments', label: 'Commitments', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: row.commitments > 0 ? 'var(--text)' : 'var(--text-muted)' }}>{row.commitments || '—'}</span> },
  { key: 'entries', label: 'Entries', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ fontWeight: row.entries > 0 ? 600 : 400, color: row.entries > 0 ? 'var(--accent)' : 'var(--text-muted)' }}>{row.entries || '—'}</span> },
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
          setError(uiErrorMessage(err, 'Couldn’t load meets. Try again.'))
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
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Meets</h1>
        </div>
        <Link href="/meets/new" className="btn btn-primary">
          <Icon name="plus" size={14} /> New meet
        </Link>
      </div>

      {createdNotice === '1' && (
        <div className="card u-p-16px-16px u-mb-20px" style={{ borderColor: 'var(--success-border)', background: 'var(--success-bg)' }}>
          <div className="u-fs-fs-base u-fw-600" style={{ color: 'var(--success)' }}>Meet created.</div>
        </div>
      )}

      <div className="cols-4 u-gap-12px u-mb-20px" >
        {[
          { label: 'Active meets', value: rows.filter((row) => ACTIVE_STATUSES.includes(row.status)).length, color: 'var(--text)' },
          { label: 'Open commitments', value: rows.filter((row) => row.status === 'commitment_open').length, color: 'var(--warning)' },
          { label: 'Finalized', value: rows.filter((row) => row.status === 'entry_finalized').length, color: 'var(--accent-text, var(--accent))' },
          { label: 'Entries this season', value: totalEntries, color: 'var(--success)' },
        ].map((stat) => (
          <div key={stat.label} className="card u-p-16px-16px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</div>
            <div className="u-fs-fs-2xl u-ff-font-body u-fw-700" style={{ color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={viewTabs} active={viewTab} onChange={setViewTab} />

      <div className="toolbar">
        <div className="toolbar-search">
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input aria-label="Search meets..." type="text" placeholder="Search meets..." value={search} onChange={(e) => setSearch(e.target.value)} className="u-p-8px-12px-8px-32px u-fs-fs-base" style={{ width: '100%' }} />
        </div>
        <select value={courseFilter} onChange={(e) => setCourseFilter(e.target.value)} className="u-p-8px-12px u-fs-fs-base" >
          <option value="all">All courses</option>
          <option value="SCY">SCY - Short Course Yards</option>
          <option value="SCM">SCM - Short Course Meters</option>
          <option value="LCM">LCM - Long Course Meters</option>
        </select>
        <span className="u-fs-fs-sm u-ff-font-body u-ml-auto" style={{ color: 'var(--text-muted)' }}>{filtered.length} meets</span>
      </div>

      {loading ? (
        <EmptyState icon="calendar" title="Loading meets" description="Fetching your meets." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load meets" description={error} />
      ) : (
        <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No meets found" emptyDescription="Try adjusting your filters or create a new meet." />
      )}
    </div>
  )
}
