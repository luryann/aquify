'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type CommitmentStatus = 'invited' | 'committed_yes' | 'committed_no' | 'locked'

type Commitment = {
  id: string
  swimmer: string
  swimmerId: string
  household: string
  householdId: string
  meetId: string
  meetName: string
  meetDate: string
  meetStatus: string
  status: CommitmentStatus
  eventSelections: number
  respondedAt: string | null
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_COMMITMENTS: Commitment[] = [
  // Winter Classic (commitment_open)
  { id: 'com-001', swimmer: 'Caleb Henderson',  swimmerId: 'sw-001', household: 'Henderson Family', householdId: 'hh-001', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2025-01-25', meetStatus: 'commitment_open', status: 'committed_yes', eventSelections: 3, respondedAt: '2025-01-08' },
  { id: 'com-002', swimmer: 'Mia Henderson',    swimmerId: 'sw-002', household: 'Henderson Family', householdId: 'hh-001', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2025-01-25', meetStatus: 'commitment_open', status: 'committed_yes', eventSelections: 2, respondedAt: '2025-01-08' },
  { id: 'com-003', swimmer: 'Owen Henderson',   swimmerId: 'sw-003', household: 'Henderson Family', householdId: 'hh-001', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2025-01-25', meetStatus: 'commitment_open', status: 'committed_no',  eventSelections: 0, respondedAt: '2025-01-08' },
  { id: 'com-004', swimmer: 'Jenna Park',       swimmerId: 'sw-004', household: 'Park Family',       householdId: 'hh-002', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2025-01-25', meetStatus: 'commitment_open', status: 'committed_yes', eventSelections: 4, respondedAt: '2025-01-07' },
  { id: 'com-005', swimmer: 'Lucas Park',       swimmerId: 'sw-005', household: 'Park Family',       householdId: 'hh-002', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2025-01-25', meetStatus: 'commitment_open', status: 'invited',       eventSelections: 0, respondedAt: null },
  { id: 'com-006', swimmer: 'Sofia Torres',     swimmerId: 'sw-006', household: 'Torres Household',  householdId: 'hh-003', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2025-01-25', meetStatus: 'commitment_open', status: 'committed_yes', eventSelections: 5, respondedAt: '2025-01-06' },
  { id: 'com-007', swimmer: 'Ade Okafor',       swimmerId: 'sw-009', household: 'Okafor Household',  householdId: 'hh-004', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2025-01-25', meetStatus: 'commitment_open', status: 'committed_yes', eventSelections: 4, respondedAt: '2025-01-09' },
  { id: 'com-008', swimmer: 'Tolu Okafor',      swimmerId: 'sw-010', household: 'Okafor Household',  householdId: 'hh-004', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2025-01-25', meetStatus: 'commitment_open', status: 'invited',       eventSelections: 0, respondedAt: null },
  { id: 'com-009', swimmer: 'Amara Okafor',     swimmerId: 'sw-011', household: 'Okafor Household',  householdId: 'hh-004', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2025-01-25', meetStatus: 'commitment_open', status: 'committed_yes', eventSelections: 2, respondedAt: '2025-01-09' },
  // Spring Dual (entry_finalized — locked commitments)
  { id: 'com-010', swimmer: 'Caleb Henderson',  swimmerId: 'sw-001', household: 'Henderson Family', householdId: 'hh-001', meetId: 'meet-004', meetName: 'Spring Dual — Sharks', meetDate: '2025-02-22', meetStatus: 'entry_finalized', status: 'locked',        eventSelections: 3, respondedAt: '2025-02-10' },
  { id: 'com-011', swimmer: 'Jenna Park',       swimmerId: 'sw-004', household: 'Park Family',       householdId: 'hh-002', meetId: 'meet-004', meetName: 'Spring Dual — Sharks', meetDate: '2025-02-22', meetStatus: 'entry_finalized', status: 'locked',        eventSelections: 4, respondedAt: '2025-02-09' },
  { id: 'com-012', swimmer: 'Sofia Torres',     swimmerId: 'sw-006', household: 'Torres Household',  householdId: 'hh-003', meetId: 'meet-004', meetName: 'Spring Dual — Sharks', meetDate: '2025-02-22', meetStatus: 'entry_finalized', status: 'locked',        eventSelections: 5, respondedAt: '2025-02-08' },
  { id: 'com-013', swimmer: 'Ade Okafor',       swimmerId: 'sw-009', household: 'Okafor Household',  householdId: 'hh-004', meetId: 'meet-004', meetName: 'Spring Dual — Sharks', meetDate: '2025-02-22', meetStatus: 'entry_finalized', status: 'locked',        eventSelections: 3, respondedAt: '2025-02-10' },
  { id: 'com-014', swimmer: 'Hannah Lee',       swimmerId: 'sw-031', household: 'Lee Family',        householdId: 'hh-007', meetId: 'meet-004', meetName: 'Spring Dual — Sharks', meetDate: '2025-02-22', meetStatus: 'entry_finalized', status: 'committed_no',  eventSelections: 0, respondedAt: '2025-02-11' },
  // Rose City Invitational (published — no responses yet)
  { id: 'com-015', swimmer: 'Caleb Henderson',  swimmerId: 'sw-001', household: 'Henderson Family', householdId: 'hh-001', meetId: 'meet-002', meetName: 'Rose City Invitational', meetDate: '2025-02-08', meetStatus: 'published', status: 'invited', eventSelections: 0, respondedAt: null },
  { id: 'com-016', swimmer: 'Jenna Park',       swimmerId: 'sw-004', household: 'Park Family',       householdId: 'hh-002', meetId: 'meet-002', meetName: 'Rose City Invitational', meetDate: '2025-02-08', meetStatus: 'published', status: 'invited', eventSelections: 0, respondedAt: null },
]

// ─── Config ───────────────────────────────────────────────────────────────────

const STATUS_CFG: Record<CommitmentStatus, { label: string; color: string; bg: string }> = {
  invited:       { label: 'Awaiting',  color: 'var(--text-muted)',  bg: 'var(--bg-elevated)' },
  committed_yes: { label: 'Going',     color: 'var(--success)',     bg: 'var(--success-bg)' },
  committed_no:  { label: 'Not Going', color: 'var(--error)',       bg: 'var(--error-bg)' },
  locked:        { label: 'Locked',    color: 'var(--accent)',      bg: 'var(--accent-subtle)' },
}

const MEET_STATUS_CFG: Record<string, { label: string; color: string }> = {
  draft:             { label: 'Draft',             color: 'var(--text-muted)' },
  published:         { label: 'Published',         color: 'var(--info)' },
  commitment_open:   { label: 'Commitments Open',  color: 'var(--warning)' },
  commitment_closed: { label: 'Commitments Closed',color: 'var(--warning)' },
  entry_finalized:   { label: 'Entries Finalized', color: 'var(--accent)' },
  completed:         { label: 'Completed',         color: 'var(--success)' },
  archived:          { label: 'Archived',          color: 'var(--text-muted)' },
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

// ─── Derived: meet options ─────────────────────────────────────────────────────

const MEET_OPTIONS = (() => {
  const seen = new Map<string, string>()
  for (const c of MOCK_COMMITMENTS) seen.set(c.meetId, c.meetName)
  return [{ value: 'all', label: 'All Meets' }, ...[...seen.entries()].map(([v, l]) => ({ value: v, label: l }))]
})()

// ─── Table Columns ────────────────────────────────────────────────────────────

const COLUMNS: Column<Commitment>[] = [
  {
    key: 'swimmer',
    label: 'Swimmer',
    sortable: true,
    render: (r) => (
      <div>
        <Link
          href={`/members/swimmers/${r.swimmerId}`}
          style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}
        >
          {r.swimmer}
        </Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '1px' }}>
          <Link
            href={`/members/households/${r.householdId}`}
            style={{ color: 'var(--text-muted)', textDecoration: 'none' }}
            onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
            onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}
          >
            {r.household}
          </Link>
        </div>
      </div>
    ),
  },
  {
    key: 'meetName',
    label: 'Meet',
    sortable: true,
    render: (r) => (
      <div>
        <Link
          href={`/meets/${r.meetId}`}
          style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}
        >
          {r.meetName}
        </Link>
        <div style={{ fontSize: '11px', color: MEET_STATUS_CFG[r.meetStatus]?.color ?? 'var(--text-muted)', marginTop: '1px', fontFamily: 'var(--font-mono)' }}>
          {MEET_STATUS_CFG[r.meetStatus]?.label ?? r.meetStatus}
        </div>
      </div>
    ),
  },
  {
    key: 'meetDate',
    label: 'Date',
    render: (r) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>
        {formatDate(r.meetDate)}
      </span>
    ),
  },
  {
    key: 'status',
    label: 'Status',
    render: (r) => {
      const c = STATUS_CFG[r.status]
      return (
        <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', whiteSpace: 'nowrap' as const }}>
          {c.label}
        </span>
      )
    },
  },
  {
    key: 'eventSelections',
    label: 'Events Selected',
    align: 'right',
    render: (r) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: r.eventSelections > 0 ? 'var(--text)' : 'var(--text-muted)' }}>
        {r.eventSelections > 0 ? r.eventSelections : '—'}
      </span>
    ),
  },
  {
    key: 'respondedAt',
    label: 'Responded',
    render: (r) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>
        {r.respondedAt ? formatDate(r.respondedAt) : <span style={{ color: 'var(--warning)', fontStyle: 'italic' }}>Pending</span>}
      </span>
    ),
  },
  {
    key: 'id',
    label: '',
    render: (r) => (
      <Link
        href={`/meets/${r.meetId}`}
        style={{ fontSize: '12px', color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '3px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}
      >
        Meet <Icon name="chevron-right" size={12} />
      </Link>
    ),
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function CommitmentsPage() {
  const [meetFilter, setMeetFilter]     = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [search, setSearch]             = useState('')

  // Stats
  const total      = MOCK_COMMITMENTS.length
  const going      = MOCK_COMMITMENTS.filter((c) => c.status === 'committed_yes').length
  const notGoing   = MOCK_COMMITMENTS.filter((c) => c.status === 'committed_no').length
  const pending    = MOCK_COMMITMENTS.filter((c) => c.status === 'invited').length
  const locked     = MOCK_COMMITMENTS.filter((c) => c.status === 'locked').length

  const filtered = useMemo(() => {
    return MOCK_COMMITMENTS.filter((c) => {
      const q = search.toLowerCase()
      const matchSearch = !q
        || c.swimmer.toLowerCase().includes(q)
        || c.household.toLowerCase().includes(q)
        || c.meetName.toLowerCase().includes(q)
      const matchMeet   = meetFilter   === 'all' || c.meetId === meetFilter
      const matchStatus = statusFilter === 'all' || c.status === statusFilter
      return matchSearch && matchMeet && matchStatus
    })
  }, [search, meetFilter, statusFilter])

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      {/* Page header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>
            Commitments
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
            Cross-meet commitment tracker — all swimmer responses in one view
          </p>
        </div>
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total',      value: total,    color: 'var(--text)' },
          { label: 'Going',      value: going,    color: 'var(--success)' },
          { label: 'Not Going',  value: notGoing, color: 'var(--error)' },
          { label: 'Awaiting',   value: pending,  color: pending > 0 ? 'var(--warning)' : 'var(--text-muted)' },
          { label: 'Locked',     value: locked,   color: 'var(--accent)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Response rate bar */}
      {total > 0 && (
        <div className="card" style={{ padding: '14px 16px', marginBottom: '20px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '8px', fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
            <span>Response rate</span>
            <span>{Math.round(((going + notGoing + locked) / total) * 100)}%</span>
          </div>
          <div style={{ height: '5px', borderRadius: '3px', background: 'var(--bg-elevated)', overflow: 'hidden', display: 'flex' }}>
            <div style={{ height: '100%', width: `${(going / total) * 100}%`, background: 'var(--success)', transition: 'width 0.6s var(--ease-out)' }} />
            <div style={{ height: '100%', width: `${(locked / total) * 100}%`, background: 'var(--accent)', transition: 'width 0.6s var(--ease-out)' }} />
            <div style={{ height: '100%', width: `${(notGoing / total) * 100}%`, background: 'var(--error)', transition: 'width 0.6s var(--ease-out)' }} />
          </div>
          <div style={{ display: 'flex', gap: '16px', marginTop: '8px', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
            <span style={{ color: 'var(--success)' }}>■ Going ({going})</span>
            <span style={{ color: 'var(--accent)' }}>■ Locked ({locked})</span>
            <span style={{ color: 'var(--error)' }}>■ Not Going ({notGoing})</span>
            <span>■ Pending ({pending})</span>
          </div>
        </div>
      )}

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 260px' }}>
          <Icon name="search" size={14} color="var(--text-muted)" style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' } as React.CSSProperties} />
          <input
            type="text"
            placeholder="Search swimmer, household, or meet…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="input"
            style={{ paddingLeft: '32px', width: '100%' }}
          />
        </div>
        <select value={meetFilter} onChange={(e) => setMeetFilter(e.target.value)} className="input" style={{ minWidth: '180px' }}>
          {MEET_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="input" style={{ minWidth: '150px' }}>
          <option value="all">All Statuses</option>
          <option value="invited">Awaiting</option>
          <option value="committed_yes">Going</option>
          <option value="committed_no">Not Going</option>
          <option value="locked">Locked</option>
        </select>
      </div>

      {/* Result count */}
      <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '12px' }}>
        {filtered.length} of {total} commitments
      </div>

      {/* Table */}
      {filtered.length === 0
        ? <EmptyState title="No commitments found" description="Try adjusting your filters or check the meet detail page to send commitment requests." />
        : <Table columns={COLUMNS} data={filtered} pageSize={20} emptyTitle="No commitments" emptyDescription="No commitments match the current filters." />
      }
    </div>
  )
}
