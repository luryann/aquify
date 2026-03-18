'use client'

import { useCallback, useMemo, useState, type CSSProperties } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type EntryStatus = 'draft' | 'submitted' | 'approved' | 'rejected' | 'exported'

type Entry = {
  id: string
  swimmer: string
  swimmerId: string
  household: string
  householdId: string
  meetId: string
  meetName: string
  meetDate: string
  event: string
  seedTime: string
  status: EntryStatus
  override: boolean
  overrideNote: string | null
  proposedAt: string
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_ENTRIES: Entry[] = [
  // Winter Classic — submitted entries needing approval
  { id: 'ent-001', swimmer: 'Sofia Torres',    swimmerId: 'sw-006', household: 'Torres Household',  householdId: 'hh-003', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2026-03-22', event: '200 Free (F Open)',   seedTime: '2:02.18', status: 'submitted', override: false, overrideNote: null,                             proposedAt: '2026-03-10T14:00:00Z' },
  { id: 'ent-002', swimmer: 'Sofia Torres',    swimmerId: 'sw-006', household: 'Torres Household',  householdId: 'hh-003', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2026-03-22', event: '100 Free (F Open)',   seedTime: '56.40',   status: 'submitted', override: false, overrideNote: null,                             proposedAt: '2026-03-10T14:00:00Z' },
  { id: 'ent-003', swimmer: 'Jenna Park',      swimmerId: 'sw-004', household: 'Park Family',       householdId: 'hh-002', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2026-03-22', event: '200 IM (F 13-14)',    seedTime: '2:18.44', status: 'submitted', override: false, overrideNote: null,                             proposedAt: '2026-03-10T14:01:00Z' },
  { id: 'ent-004', swimmer: 'Jenna Park',      swimmerId: 'sw-004', household: 'Park Family',       householdId: 'hh-002', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2026-03-22', event: '100 Fly (F 13-14)',   seedTime: '1:04.88', status: 'submitted', override: false, overrideNote: null,                             proposedAt: '2026-03-10T14:01:00Z' },
  { id: 'ent-005', swimmer: 'Caleb Henderson', swimmerId: 'sw-001', household: 'Henderson Family',  householdId: 'hh-001', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2026-03-22', event: '100 Free (M Open)',   seedTime: '52.84',   status: 'submitted', override: false, overrideNote: null,                             proposedAt: '2026-03-10T14:02:00Z' },
  { id: 'ent-006', swimmer: 'Ade Okafor',      swimmerId: 'sw-009', household: 'Okafor Household',  householdId: 'hh-004', meetId: 'meet-001', meetName: 'Winter Classic',        meetDate: '2026-03-22', event: '100 Back (M Open)',   seedTime: '56.12',   status: 'submitted', override: true,  overrideNote: 'Coach override: seed time updated from March results', proposedAt: '2026-03-10T14:03:00Z' },
  // Spring Dual — approved entries
  { id: 'ent-007', swimmer: 'Caleb Henderson', swimmerId: 'sw-001', household: 'Henderson Family',  householdId: 'hh-001', meetId: 'meet-004', meetName: 'Spring Dual — Sharks', meetDate: '2026-04-05', event: '100 Free (M Open)',   seedTime: '52.40',   status: 'approved',  override: false, overrideNote: null,                             proposedAt: '2026-03-20T10:00:00Z' },
  { id: 'ent-008', swimmer: 'Jenna Park',      swimmerId: 'sw-004', household: 'Park Family',       householdId: 'hh-002', meetId: 'meet-004', meetName: 'Spring Dual — Sharks', meetDate: '2026-04-05', event: '200 IM (F 13-14)',    seedTime: '2:17.11', status: 'approved',  override: false, overrideNote: null,                             proposedAt: '2026-03-20T10:01:00Z' },
  { id: 'ent-009', swimmer: 'Sofia Torres',    swimmerId: 'sw-006', household: 'Torres Household',  householdId: 'hh-003', meetId: 'meet-004', meetName: 'Spring Dual — Sharks', meetDate: '2026-04-05', event: '200 Free (F Open)',   seedTime: '2:01.44', status: 'approved',  override: false, overrideNote: null,                             proposedAt: '2026-03-20T10:02:00Z' },
  // Rose City Invitational — draft entries
  { id: 'ent-010', swimmer: 'Amara Okafor',    swimmerId: 'sw-011', household: 'Okafor Household',  householdId: 'hh-004', meetId: 'meet-002', meetName: 'Rose City Invitational', meetDate: '2026-04-19', event: '100 Breast (F Open)', seedTime: '1:12.33', status: 'draft',     override: false, overrideNote: null,                             proposedAt: '2026-04-01T09:00:00Z' },
  { id: 'ent-011', swimmer: 'Mia Henderson',   swimmerId: 'sw-002', household: 'Henderson Family',  householdId: 'hh-001', meetId: 'meet-002', meetName: 'Rose City Invitational', meetDate: '2026-04-19', event: '100 Back (F 11-12)', seedTime: '1:08.22', status: 'draft',     override: false, overrideNote: null,                             proposedAt: '2026-04-01T09:01:00Z' },
]

// ─── Config ───────────────────────────────────────────────────────────────────

const STATUS_CFG: Record<EntryStatus, { label: string; color: string; bg: string }> = {
  draft:     { label: 'Draft',     color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  submitted: { label: 'Submitted', color: 'var(--warning)',    bg: 'var(--warning-bg)' },
  approved:  { label: 'Approved',  color: 'var(--success)',    bg: 'var(--success-bg)' },
  rejected:  { label: 'Rejected',  color: 'var(--error)',      bg: 'var(--error-bg)' },
  exported:  { label: 'Exported',  color: 'var(--info)',       bg: 'var(--info-bg)' },
}

const STATUS_FILTER_OPTIONS = [
  { value: 'all',       label: 'All Statuses' },
  { value: 'draft',     label: 'Draft' },
  { value: 'submitted', label: 'Submitted' },
  { value: 'approved',  label: 'Approved' },
  { value: 'rejected',  label: 'Rejected' },
  { value: 'exported',  label: 'Exported' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

// ─── Derived: meet options ─────────────────────────────────────────────────────

const MEET_OPTIONS = (() => {
  const seen = new Map<string, string>()
  for (const e of MOCK_ENTRIES) seen.set(e.meetId, e.meetName)
  return [{ value: 'all', label: 'All Meets' }, ...[...seen.entries()].map(([v, l]) => ({ value: v, label: l }))]
})()

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function EntriesPage() {
  const [entries, setEntries]           = useState<Entry[]>(MOCK_ENTRIES)
  const [meetFilter, setMeetFilter]     = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [search, setSearch]             = useState('')

  const handleApprove = useCallback((entryId: string) => {
    setEntries((prev) => prev.map((e) => e.id === entryId ? { ...e, status: 'approved' } : e))
  }, [])

  const handleReject = useCallback((entryId: string) => {
    if (!window.confirm('Reject this submitted entry? The swimmer entry will remain visible as rejected.')) return
    setEntries((prev) => prev.map((e) => e.id === entryId ? { ...e, status: 'rejected' } : e))
  }, [])

  function handleApproveAll(ids: string[]) {
    setEntries((prev) => prev.map((e) => ids.includes(e.id) ? { ...e, status: 'approved' } : e))
  }

  // Stats
  const total     = entries.length
  const submitted = entries.filter((e) => e.status === 'submitted').length
  const approved  = entries.filter((e) => e.status === 'approved').length
  const draft     = entries.filter((e) => e.status === 'draft').length
  const overrides = entries.filter((e) => e.override).length

  const filtered = useMemo(() => {
    return entries.filter((e) => {
      const q = search.toLowerCase()
      const matchSearch = !q
        || e.swimmer.toLowerCase().includes(q)
        || e.event.toLowerCase().includes(q)
        || e.meetName.toLowerCase().includes(q)
        || e.household.toLowerCase().includes(q)
      const matchMeet   = meetFilter   === 'all' || e.meetId   === meetFilter
      const matchStatus = statusFilter === 'all' || e.status   === statusFilter
      return matchSearch && matchMeet && matchStatus
    })
  }, [entries, search, meetFilter, statusFilter])

  const pendingVisible = filtered.filter((e) => e.status === 'submitted')

  // Build table columns inside the component so approve/reject closures work
  const columns = useMemo<Column<Entry>[]>(() => [
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
      key: 'event',
      label: 'Event',
      render: (r) => (
        <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{r.event}</span>
      ),
    },
    {
      key: 'seedTime',
      label: 'Seed Time',
      align: 'right',
      render: (r) => (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--accent)' }}>
          {r.seedTime}
        </span>
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
          <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '1px' }}>
            {formatDate(r.meetDate)}
          </div>
        </div>
      ),
    },
    {
      key: 'status',
      label: 'Status',
      render: (r) => {
        const c = STATUS_CFG[r.status]
        return (
          <div style={{ display: 'flex', gap: '5px', alignItems: 'center', flexWrap: 'wrap' }}>
            <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', whiteSpace: 'nowrap' as const }}>
              {c.label}
            </span>
            {r.override && (
              <span style={{ fontSize: '10px', fontWeight: 700, padding: '2px 6px', borderRadius: '8px', background: 'var(--accent-subtle)', color: 'var(--accent)', fontFamily: 'var(--font-mono)' }}>
                OVERRIDE
              </span>
            )}
          </div>
        )
      },
    },
    {
      key: 'proposedAt',
      label: 'Proposed',
      render: (r) => (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>
          {formatDate(r.proposedAt)}
        </span>
      ),
    },
    {
      key: 'id',
      label: '',
      render: (r) => {
        if (r.status !== 'submitted') {
          return (
            <Link
              href={`/meets/${r.meetId}`}
              style={{ fontSize: '12px', color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '3px' }}
              onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
              onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}
            >
              Meet <Icon name="chevron-right" size={12} />
            </Link>
          )
        }
        return (
          <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
            <button
              className="btn btn-sm"
              style={{ background: 'var(--success-bg)', color: 'var(--success)', border: '1px solid var(--success-border)', display: 'flex', alignItems: 'center', gap: '4px' }}
              onClick={() => handleApprove(r.id)}
            >
              <Icon name="check-circle" size={11} /> Approve
            </button>
            <button
              className="btn btn-sm btn-secondary"
              style={{ color: 'var(--error)', display: 'flex', alignItems: 'center', gap: '4px' }}
              onClick={() => handleReject(r.id)}
            >
              <Icon name="x" size={11} /> Reject
            </button>
          </div>
        )
      },
    },
  ], [handleApprove, handleReject])

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      {/* Page header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>
            Entry Approvals
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
            Cross-meet entry approval queue — review and approve submitted swimmer entries
          </p>
        </div>
        {pendingVisible.length > 0 && (
          <button
            className="btn btn-primary"
            style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
            onClick={() => handleApproveAll(pendingVisible.map((e) => e.id))}
          >
            <Icon name="check-circle" size={13} /> Approve All ({pendingVisible.length})
          </button>
        )}
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total',       value: total,     color: 'var(--text)' },
          { label: 'Needs Review', value: submitted, color: submitted > 0 ? 'var(--warning)' : 'var(--text-muted)' },
          { label: 'Approved',    value: approved,  color: 'var(--success)' },
          { label: 'Overrides',   value: overrides, color: overrides > 0 ? 'var(--accent)' : 'var(--text-muted)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Pending banner */}
      {submitted > 0 && (
        <div
          className="card"
          style={{ padding: '14px 16px', marginBottom: '20px', borderLeft: '3px solid var(--warning)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px' }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <Icon name="alert-circle" size={16} color="var(--warning)" />
            <span style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
              <strong style={{ color: 'var(--warning)' }}>{submitted} entr{submitted === 1 ? 'y' : 'ies'}</strong> {submitted === 1 ? 'is' : 'are'} pending coach approval across all meets
            </span>
          </div>
          {draft > 0 && (
            <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
              {draft} draft{draft !== 1 ? 's' : ''} not yet submitted
            </span>
          )}
        </div>
      )}

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 260px' }}>
          <Icon name="search" size={14} color="var(--text-muted)" style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' } as CSSProperties} />
          <input
            type="text"
            placeholder="Search swimmer, event, or meet…"
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
          {STATUS_FILTER_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </div>

      {/* Result count */}
      <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '12px' }}>
        {filtered.length} of {total} entries
      </div>

      {/* Table */}
      {filtered.length === 0
        ? <EmptyState title="No entries found" description="Try adjusting your filters. Entries are proposed when commitments are locked and event selections are finalized." />
        : <Table columns={columns} data={filtered} pageSize={20} emptyTitle="No entries" emptyDescription="No entries match the current filters." />
      }
    </div>
  )
}
