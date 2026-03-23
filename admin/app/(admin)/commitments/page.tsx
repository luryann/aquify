'use client'

import { useState, useMemo, useEffect } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchMeets,
  fetchMeetCommitments,
  fetchSwimmers,
  fetchHouseholds,
  type CommitmentRecord,
  type SwimmerRecord,
  type HouseholdRecord,
} from '@/lib/adminData'

// ─── Types ────────────────────────────────────────────────────────────────────

type CommitmentStatus = 'invited' | 'committed_yes' | 'committed_no' | 'locked' | 'coach_reviewed'

type CommitmentRow = {
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
}

// ─── Config ───────────────────────────────────────────────────────────────────

const STATUS_CFG: Record<string, { label: string; color: string; bg: string }> = {
  invited:        { label: 'Awaiting',     color: 'var(--text-muted)',  bg: 'var(--bg-elevated)' },
  committed_yes:  { label: 'Going',        color: 'var(--success)',     bg: 'var(--success-bg)' },
  committed_no:   { label: 'Not Going',    color: 'var(--error)',       bg: 'var(--error-bg)' },
  locked:         { label: 'Locked',       color: 'var(--accent)',      bg: 'var(--accent-subtle)' },
  coach_reviewed: { label: 'Reviewed',     color: 'var(--info)',        bg: 'var(--info-bg)' },
}

const MEET_STATUS_CFG: Record<string, { label: string; color: string }> = {
  draft:             { label: 'Draft',              color: 'var(--text-muted)' },
  published:         { label: 'Published',          color: 'var(--info)' },
  commitment_open:   { label: 'Commitments Open',   color: 'var(--warning)' },
  commitment_closed: { label: 'Commitments Closed', color: 'var(--warning)' },
  entry_finalized:   { label: 'Entries Finalized',  color: 'var(--accent)' },
  completed:         { label: 'Completed',          color: 'var(--success)' },
  archived:          { label: 'Archived',           color: 'var(--text-muted)' },
}

// Fetch commitments for meets that have commitment data
const COMMITMENT_STATUSES = new Set(['commitment_open', 'commitment_closed', 'entry_finalized', 'completed'])

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function CommitmentsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)

  const [rows, setRows] = useState<CommitmentRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [meetFilter, setMeetFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [search, setSearch] = useState('')

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    let cancelled = false
    async function run() {
      if (!session?.user.orgId) { setLoading(false); return }
      try {
        setLoading(true)
        setError(null)
        const orgId = session.user.orgId

        const [allMeets, allSwimmers, allHouseholds] = await Promise.all([
          fetchMeets(api, orgId),
          fetchSwimmers(api, orgId),
          fetchHouseholds(api, orgId),
        ])

        const swimmerMap = new Map<string, SwimmerRecord>(allSwimmers.map((s) => [s.id, s]))
        const householdMap = new Map<string, HouseholdRecord>(allHouseholds.map((h) => [h.id, h]))

        // Only fetch commitments for meets in relevant statuses
        const relevantMeets = allMeets.filter((m) => COMMITMENT_STATUSES.has(m.status))

        const commitmentResults = await Promise.all(
          relevantMeets.map((m) =>
            fetchMeetCommitments(api, orgId, m.id).catch(() => [] as CommitmentRecord[])
          )
        )

        const combined: CommitmentRow[] = []
        for (let i = 0; i < relevantMeets.length; i++) {
          const meet = relevantMeets[i]
          const commitments = commitmentResults[i]
          for (const c of commitments) {
            const swimmer = swimmerMap.get(c.swimmer_id)
            const household = swimmer ? householdMap.get(swimmer.household_id) : undefined
            combined.push({
              id: c.id,
              swimmer: swimmer ? `${swimmer.first_name} ${swimmer.last_name}` : c.swimmer_id,
              swimmerId: c.swimmer_id,
              household: household?.household_name ?? '—',
              householdId: swimmer?.household_id ?? '',
              meetId: meet.id,
              meetName: meet.name,
              meetDate: meet.start_date ?? '',
              meetStatus: meet.status,
              status: c.status as CommitmentStatus,
            })
          }
        }

        if (!cancelled) setRows(combined)
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load commitments')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    run()
    return () => { cancelled = true }
  }, [session?.user.orgId])

  // Stats
  const total    = rows.length
  const going    = rows.filter((r) => r.status === 'committed_yes').length
  const notGoing = rows.filter((r) => r.status === 'committed_no').length
  const pending  = rows.filter((r) => r.status === 'invited').length
  const locked   = rows.filter((r) => r.status === 'locked' || r.status === 'coach_reviewed').length

  // Meet options
  const meetOptions = useMemo(() => {
    const seen = new Map<string, string>()
    for (const r of rows) seen.set(r.meetId, r.meetName)
    return [{ value: 'all', label: 'All Meets' }, ...[...seen.entries()].map(([v, l]) => ({ value: v, label: l }))]
  }, [rows])

  const filtered = useMemo(() => {
    return rows.filter((r) => {
      const q = search.toLowerCase()
      const matchSearch = !q
        || r.swimmer.toLowerCase().includes(q)
        || r.household.toLowerCase().includes(q)
        || r.meetName.toLowerCase().includes(q)
      const matchMeet   = meetFilter   === 'all' || r.meetId === meetFilter
      const matchStatus = statusFilter === 'all' || r.status === statusFilter
      return matchSearch && matchMeet && matchStatus
    })
  }, [rows, search, meetFilter, statusFilter])

  const columns = useMemo<Column<CommitmentRow>[]>(() => [
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
          {r.householdId && (
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
          )}
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
          {r.meetDate ? formatDate(r.meetDate) : '—'}
        </span>
      ),
    },
    {
      key: 'status',
      label: 'Status',
      render: (r) => {
        const c = STATUS_CFG[r.status] ?? STATUS_CFG['invited']
        return (
          <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', whiteSpace: 'nowrap' as const }}>
            {c.label}
          </span>
        )
      },
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
  ], [])

  if (error) {
    return (
      <div style={{ padding: '24px', maxWidth: '1200px' }}>
        <div className="card" style={{ padding: '20px', borderLeft: '3px solid var(--error)' }}>
          <p style={{ color: 'var(--error)', fontSize: '13px' }}>Failed to load commitments: {error}</p>
        </div>
      </div>
    )
  }

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
          { label: 'Total',     value: loading ? '—' : total,    color: 'var(--text)' },
          { label: 'Going',     value: loading ? '—' : going,    color: 'var(--success)' },
          { label: 'Not Going', value: loading ? '—' : notGoing, color: 'var(--error)' },
          { label: 'Awaiting',  value: loading ? '—' : pending,  color: (pending > 0 && !loading) ? 'var(--warning)' : 'var(--text-muted)' },
          { label: 'Locked',    value: loading ? '—' : locked,   color: 'var(--accent)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Response rate bar */}
      {!loading && total > 0 && (
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
          {meetOptions.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
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
      {!loading && (
        <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '12px' }}>
          {filtered.length} of {total} commitments
        </div>
      )}

      {/* Loading skeleton */}
      {loading && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
          {[1,2,3,4,5].map((i) => (
            <div key={i} style={{ height: '44px', background: 'var(--bg-elevated)', borderRadius: '6px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: `${i * 0.1}s` }} />
          ))}
        </div>
      )}

      {/* Table */}
      {!loading && (
        filtered.length === 0
          ? <EmptyState title="No commitments found" description={total === 0 ? "No commitments exist yet. Commitments are created when meets reach commitment_open status." : "Try adjusting your filters."} />
          : <Table columns={columns} data={filtered} pageSize={20} emptyTitle="No commitments" emptyDescription="No commitments match the current filters." />
      )}

      <style>{`@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>
    </div>
  )
}
