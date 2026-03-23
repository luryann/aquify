'use client'

import { useCallback, useMemo, useState, useEffect, type CSSProperties } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchMeets,
  fetchMeetEntries,
  fetchMeetEvents,
  fetchSwimmers,
  type EntryRecord,
  type MeetEventRecord,
  type SwimmerRecord,
} from '@/lib/adminData'

// ─── Types ────────────────────────────────────────────────────────────────────

type EntryStatus = 'draft' | 'submitted' | 'approved' | 'rejected' | 'exported' | 'updated_from_results' | 'archived'

type EntryRow = {
  id: string
  swimmer: string
  swimmerId: string
  meetId: string
  meetName: string
  meetDate: string
  event: string
  seedTime: string
  status: EntryStatus
  override: boolean
  overrideNote: string | null
}

// ─── Config ───────────────────────────────────────────────────────────────────

const STATUS_CFG: Record<string, { label: string; color: string; bg: string }> = {
  draft:                { label: 'Draft',     color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  submitted:            { label: 'Submitted', color: 'var(--warning)',    bg: 'var(--warning-bg)' },
  approved:             { label: 'Approved',  color: 'var(--success)',    bg: 'var(--success-bg)' },
  rejected:             { label: 'Rejected',  color: 'var(--error)',      bg: 'var(--error-bg)' },
  exported:             { label: 'Exported',  color: 'var(--info)',       bg: 'var(--info-bg)' },
  updated_from_results: { label: 'Updated',   color: 'var(--info)',       bg: 'var(--info-bg)' },
  archived:             { label: 'Archived',  color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

const STATUS_FILTER_OPTIONS = [
  { value: 'all',       label: 'All Statuses' },
  { value: 'draft',     label: 'Draft' },
  { value: 'submitted', label: 'Submitted' },
  { value: 'approved',  label: 'Approved' },
  { value: 'rejected',  label: 'Rejected' },
  { value: 'exported',  label: 'Exported' },
]

const STROKE_LABELS: Record<string, string> = {
  free: 'Free', back: 'Back', breast: 'Breast', fly: 'Fly', im: 'IM', medley: 'Medley',
}

const ENTRY_STATUSES = new Set(['commitment_open', 'commitment_closed', 'entry_finalized', 'completed'])

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function formatSeedTime(hundredths: number | undefined): string {
  if (!hundredths || hundredths <= 0) return 'NT'
  const totalHundredths = Math.round(hundredths)
  const h = Math.floor(totalHundredths / 360000)
  const m = Math.floor((totalHundredths % 360000) / 6000)
  const s = Math.floor((totalHundredths % 6000) / 100)
  const cs = totalHundredths % 100
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}.${String(cs).padStart(2, '0')}`
  if (m > 0) return `${m}:${String(s).padStart(2, '0')}.${String(cs).padStart(2, '0')}`
  return `${s}.${String(cs).padStart(2, '0')}`
}

function formatEvent(ev: MeetEventRecord | undefined): string {
  if (!ev) return '—'
  const dist = ev.distance ?? ''
  const stroke = STROKE_LABELS[ev.stroke?.toLowerCase() ?? ''] ?? ev.stroke ?? ''
  const gender = ev.gender ? (ev.gender === 'M' ? 'M' : ev.gender === 'F' ? 'F' : ev.gender) : ''
  const age = ev.age_group ? ` ${ev.age_group}` : ' Open'
  return [dist, stroke, gender ? `(${gender}${age})` : ''].filter(Boolean).join(' ')
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function EntriesPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)

  const [rows, setRows] = useState<EntryRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [meetFilter, setMeetFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [search, setSearch] = useState('')

  const handleApprove = useCallback((entryId: string) => {
    setRows((prev) => prev.map((e) => e.id === entryId ? { ...e, status: 'approved' } : e))
  }, [])

  const handleReject = useCallback((entryId: string) => {
    if (!window.confirm('Reject this submitted entry?')) return
    setRows((prev) => prev.map((e) => e.id === entryId ? { ...e, status: 'rejected' } : e))
  }, [])

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    let cancelled = false
    async function run() {
      if (!session?.user.orgId) { setLoading(false); return }
      try {
        setLoading(true)
        setError(null)
        const orgId = session.user.orgId

        const [allMeets, allSwimmers] = await Promise.all([
          fetchMeets(api, orgId),
          fetchSwimmers(api, orgId),
        ])

        const swimmerMap = new Map<string, SwimmerRecord>(allSwimmers.map((s) => [s.id, s]))
        const relevantMeets = allMeets.filter((m) => ENTRY_STATUSES.has(m.status))

        // Fetch entries and events for each meet in parallel
        const [entryResults, eventResults] = await Promise.all([
          Promise.all(relevantMeets.map((m) => fetchMeetEntries(api, orgId, m.id).catch(() => [] as EntryRecord[]))),
          Promise.all(relevantMeets.map((m) => fetchMeetEvents(api, orgId, m.id).catch(() => [] as MeetEventRecord[]))),
        ])

        const combined: EntryRow[] = []
        for (let i = 0; i < relevantMeets.length; i++) {
          const meet = relevantMeets[i]
          const entries = entryResults[i]
          const eventsMap = new Map<string, MeetEventRecord>(eventResults[i].map((ev) => [ev.id, ev]))

          for (const e of entries) {
            const swimmer = swimmerMap.get(e.swimmer_id)
            const meetEvent = e.meet_event_id ? eventsMap.get(e.meet_event_id) : undefined
            combined.push({
              id: e.id,
              swimmer: swimmer ? `${swimmer.first_name} ${swimmer.last_name}` : e.swimmer_id,
              swimmerId: e.swimmer_id,
              meetId: meet.id,
              meetName: meet.name,
              meetDate: meet.start_date ?? '',
              event: formatEvent(meetEvent),
              seedTime: formatSeedTime(e.seed_time_cents),
              status: e.status as EntryStatus,
              override: !!e.override_reason,
              overrideNote: e.override_reason ?? null,
            })
          }
        }

        if (!cancelled) setRows(combined)
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load entries')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    run()
    return () => { cancelled = true }
  }, [session?.user.orgId])

  // Stats
  const total     = rows.length
  const submitted = rows.filter((e) => e.status === 'submitted').length
  const approved  = rows.filter((e) => e.status === 'approved').length
  const draft     = rows.filter((e) => e.status === 'draft').length
  const overrides = rows.filter((e) => e.override).length

  // Meet options
  const meetOptions = useMemo(() => {
    const seen = new Map<string, string>()
    for (const r of rows) seen.set(r.meetId, r.meetName)
    return [{ value: 'all', label: 'All Meets' }, ...[...seen.entries()].map(([v, l]) => ({ value: v, label: l }))]
  }, [rows])

  const filtered = useMemo(() => {
    return rows.filter((e) => {
      const q = search.toLowerCase()
      const matchSearch = !q
        || e.swimmer.toLowerCase().includes(q)
        || e.event.toLowerCase().includes(q)
        || e.meetName.toLowerCase().includes(q)
      const matchMeet   = meetFilter   === 'all' || e.meetId === meetFilter
      const matchStatus = statusFilter === 'all' || e.status === statusFilter
      return matchSearch && matchMeet && matchStatus
    })
  }, [rows, search, meetFilter, statusFilter])

  const pendingVisible = filtered.filter((e) => e.status === 'submitted')

  const columns = useMemo<Column<EntryRow>[]>(() => [
    {
      key: 'swimmer',
      label: 'Swimmer',
      sortable: true,
      render: (r) => (
        <Link
          href={`/members/swimmers/${r.swimmerId}`}
          style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}
        >
          {r.swimmer}
        </Link>
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
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: r.seedTime === 'NT' ? 'var(--text-muted)' : 'var(--accent)' }}>
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
          {r.meetDate && (
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '1px' }}>
              {formatDate(r.meetDate)}
            </div>
          )}
        </div>
      ),
    },
    {
      key: 'status',
      label: 'Status',
      render: (r) => {
        const c = STATUS_CFG[r.status] ?? STATUS_CFG['draft']
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

  if (error) {
    return (
      <div style={{ padding: '24px', maxWidth: '1200px' }}>
        <div className="card" style={{ padding: '20px', borderLeft: '3px solid var(--error)' }}>
          <p style={{ color: 'var(--error)', fontSize: '13px' }}>Failed to load entries: {error}</p>
        </div>
      </div>
    )
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <style>{`@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>

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
            onClick={() => {
              for (const e of pendingVisible) handleApprove(e.id)
            }}
          >
            <Icon name="check-circle" size={13} /> Approve All ({pendingVisible.length})
          </button>
        )}
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total',        value: loading ? '—' : total,     color: 'var(--text)' },
          { label: 'Needs Review', value: loading ? '—' : submitted, color: (submitted > 0 && !loading) ? 'var(--warning)' : 'var(--text-muted)' },
          { label: 'Approved',     value: loading ? '—' : approved,  color: 'var(--success)' },
          { label: 'Overrides',    value: loading ? '—' : overrides, color: (overrides > 0 && !loading) ? 'var(--accent)' : 'var(--text-muted)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Pending banner */}
      {!loading && submitted > 0 && (
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
          {meetOptions.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="input" style={{ minWidth: '150px' }}>
          {STATUS_FILTER_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </div>

      {/* Result count */}
      {!loading && (
        <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '12px' }}>
          {filtered.length} of {total} entries
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
          ? <EmptyState title="No entries found" description={total === 0 ? "No entries exist yet. Entries are created when commitments are locked and event selections are finalized." : "Try adjusting your filters."} />
          : <Table columns={columns} data={filtered} pageSize={20} emptyTitle="No entries" emptyDescription="No entries match the current filters." />
      )}
    </div>
  )
}
