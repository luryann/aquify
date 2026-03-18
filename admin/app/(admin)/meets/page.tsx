'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { Tabs } from '@/components/ui/Tabs'

// ─── Types ────────────────────────────────────────────────────────────────────

type MeetStatus =
  | 'draft'
  | 'published'
  | 'commitment_open'
  | 'commitment_closed'
  | 'entry_finalized'
  | 'completed'
  | 'archived'

type MeetCourse = 'SCY' | 'SCM' | 'LCM'

type Meet = {
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

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_MEETS: Meet[] = [
  // Upcoming / active
  { id: 'meet-001', name: 'Winter Classic',           host: 'DARE Aquatics',  location: 'Portland Aquatics Center',  startDate: '2026-03-22', endDate: '2026-03-23', status: 'commitment_open',   course: 'SCY', entries: 0,   commitments: 47, events: 24 },
  { id: 'meet-002', name: 'Rose City Invitational',   host: 'Rose City Swim', location: 'Mt. Hood Aquatics',         startDate: '2026-04-19', endDate: '2026-04-20', status: 'published',         course: 'SCY', entries: 0,   commitments: 0,  events: 20 },
  { id: 'meet-003', name: 'NW Age Group Champs',      host: 'PNSO',           location: 'King County Aquatics',      startDate: '2026-05-15', endDate: '2026-05-17', status: 'draft',             course: 'SCY', entries: 0,   commitments: 0,  events: 0  },
  { id: 'meet-004', name: 'Spring Dual — Sharks',     host: 'DARE Aquatics',  location: 'DARE Pool',                 startDate: '2026-04-05', endDate: '2026-04-05', status: 'entry_finalized',   course: 'SCY', entries: 89,  commitments: 52, events: 18 },
  // Completed
  { id: 'meet-005', name: 'League Championships',     host: 'Portland League',location: 'Sellwood Pool',             startDate: '2024-11-02', endDate: '2024-11-03', status: 'completed',         course: 'SCY', entries: 124, commitments: 68, events: 30 },
  { id: 'meet-006', name: 'Fall Classic',             host: 'DARE Aquatics',  location: 'DARE Pool',                 startDate: '2024-10-19', endDate: '2024-10-20', status: 'completed',         course: 'SCY', entries: 96,  commitments: 55, events: 24 },
  { id: 'meet-007', name: 'Season Opener',            host: 'Willamette Swim',location: 'Williamette University',    startDate: '2024-09-28', endDate: '2024-09-29', status: 'archived',          course: 'SCY', entries: 78,  commitments: 44, events: 20 },
  { id: 'meet-008', name: 'Summer League Final',      host: 'Portland League',location: 'Portland Community Pool',   startDate: '2024-08-10', endDate: '2024-08-11', status: 'archived',          course: 'LCM', entries: 210, commitments: 92, events: 40 },
]

const CREATED_MEETS_KEY = 'aquify:created-meets'

// ─── Helpers ──────────────────────────────────────────────────────────────────

const STATUS_CONFIG: Record<MeetStatus, { label: string; color: string; bg: string; short: string }> = {
  draft:               { label: 'Draft',               color: 'var(--text-muted)', bg: 'var(--bg-elevated)', short: 'draft' },
  published:           { label: 'Published',           color: 'var(--info)',       bg: 'var(--info-bg)',     short: 'published' },
  commitment_open:     { label: 'Commitments Open',    color: 'var(--warning)',    bg: 'var(--warning-bg)', short: 'comm. open' },
  commitment_closed:   { label: 'Commitments Closed',  color: 'var(--warning)',    bg: 'var(--warning-bg)', short: 'comm. closed' },
  entry_finalized:     { label: 'Entries Finalized',   color: 'var(--accent)',     bg: 'var(--accent-subtle)', short: 'finalized' },
  completed:           { label: 'Completed',           color: 'var(--success)',    bg: 'var(--success-bg)', short: 'done' },
  archived:            { label: 'Archived',            color: 'var(--text-muted)', bg: 'var(--bg-elevated)', short: 'archived' },
}

function StatusBadge({ status }: { status: MeetStatus }) {
  const c = STATUS_CONFIG[status]
  return (
    <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em', whiteSpace: 'nowrap' as const }}>
      {c.label}
    </span>
  )
}

function CourseBadge({ course }: { course: MeetCourse }) {
  const colors: Record<MeetCourse, string> = { SCY: 'var(--text-subtle)', SCM: 'var(--info)', LCM: 'var(--accent)' }
  return (
    <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', padding: '2px 6px', borderRadius: 'var(--r-xs)', border: '1px solid var(--border)', color: colors[course], background: 'var(--bg-elevated)' }}>
      {course}
    </span>
  )
}

// ─── Columns ──────────────────────────────────────────────────────────────────

const COLUMNS: Column<Meet>[] = [
  {
    key: 'name', label: 'Meet', sortable: true,
    render: (r) => (
      <div>
        <Link href={`/meets/${r.id}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
          {r.name}
        </Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>{r.host} · {r.location}</div>
      </div>
    ),
  },
  {
    key: 'startDate', label: 'Dates', sortable: true,
    render: (r) => (
      <div style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>
        <div>{r.startDate}</div>
        {r.endDate !== r.startDate && <div style={{ color: 'var(--text-muted)' }}>→ {r.endDate}</div>}
      </div>
    ),
  },
  { key: 'course', label: 'Course', render: (r) => <CourseBadge course={r.course} /> },
  { key: 'status', label: 'Status', render: (r) => <StatusBadge status={r.status} /> },
  {
    key: 'events', label: 'Events', align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{r.events > 0 ? r.events : '—'}</span>,
  },
  {
    key: 'commitments', label: 'Commitments', align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: r.commitments > 0 ? 'var(--text)' : 'var(--text-muted)' }}>{r.commitments > 0 ? r.commitments : '—'}</span>,
  },
  {
    key: 'entries', label: 'Entries', align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', fontWeight: r.entries > 0 ? 600 : 400, color: r.entries > 0 ? 'var(--accent)' : 'var(--text-muted)' }}>{r.entries > 0 ? r.entries : '—'}</span>,
  },
  {
    key: 'id', label: '', width: '60px',
    render: (r) => (
      <Link href={`/meets/${r.id}`} style={{ fontSize: '12px', color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}>
        View <Icon name="chevron-right" size={12} />
      </Link>
    ),
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

const ACTIVE_STATUSES: MeetStatus[] = ['draft', 'published', 'commitment_open', 'commitment_closed', 'entry_finalized']
const PAST_STATUSES:   MeetStatus[] = ['completed', 'archived']

export default function MeetsPage() {
  const [meets, setMeets] = useState<Meet[]>(MOCK_MEETS)
  const [viewTab, setViewTab]     = useState('upcoming')
  const [search, setSearch]       = useState('')
  const [courseFilter, setCourse] = useState('all')

  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(CREATED_MEETS_KEY)
      if (!raw) return
      const created = JSON.parse(raw) as Meet[]
      if (Array.isArray(created) && created.length > 0) {
        setMeets([...created, ...MOCK_MEETS])
      }
    } catch {
      window.localStorage.removeItem(CREATED_MEETS_KEY)
    }
  }, [])

  const viewTabs = [
    { id: 'upcoming', label: 'Upcoming / Active', count: meets.filter((m) => ACTIVE_STATUSES.includes(m.status)).length },
    { id: 'past',     label: 'Past',              count: meets.filter((m) => PAST_STATUSES.includes(m.status)).length },
    { id: 'all',      label: 'All',               count: meets.length },
  ]

  const filtered = useMemo(() => {
    return meets.filter((m) => {
      const q = search.toLowerCase()
      const matchesSearch = !search || m.name.toLowerCase().includes(q) || m.host.toLowerCase().includes(q) || m.location.toLowerCase().includes(q)
      const matchesCourse = courseFilter === 'all' || m.course === courseFilter
      const matchesView   = viewTab === 'all'
        ? true
        : viewTab === 'upcoming'
        ? ACTIVE_STATUSES.includes(m.status)
        : PAST_STATUSES.includes(m.status)
      return matchesSearch && matchesCourse && matchesView
    })
  }, [meets, search, courseFilter, viewTab])

  const totalEntries = meets.filter((m) => m.status === 'completed').reduce((a, m) => a + m.entries, 0)

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Meets</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Manage swim meets, events, commitments, and entries</p>
        </div>
        <Link href="/meets/new" className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px', textDecoration: 'none' }}>
          <Icon name="plus" size={14} /> New Meet
        </Link>
      </div>

      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Active Meets',    value: meets.filter((m) => ACTIVE_STATUSES.includes(m.status)).length, color: 'var(--text)' },
          { label: 'Open Commitments',value: meets.filter((m) => m.status === 'commitment_open').length,     color: 'var(--warning)' },
          { label: 'Finalized',        value: meets.filter((m) => m.status === 'entry_finalized').length,    color: 'var(--accent)' },
          { label: 'Total Entries (Season)', value: totalEntries,                                                  color: 'var(--success)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* View tabs */}
      <Tabs tabs={viewTabs} active={viewTab} onChange={setViewTab} />

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '320px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search meets..." value={search} onChange={(e) => setSearch(e.target.value)}
            style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={courseFilter} onChange={(e) => setCourse(e.target.value)}
          style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', cursor: 'pointer' }}>
          <option value="all">All Courses</option>
          <option value="SCY">SCY</option>
          <option value="SCM">SCM</option>
          <option value="LCM">LCM</option>
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>
          {filtered.length} meets
        </span>
      </div>

      <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No meets found" emptyDescription="Try adjusting your filters or create a new meet." />
    </div>
  )
}
