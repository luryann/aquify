'use client'

import { useState, useMemo, useEffect, useCallback, useRef, type CSSProperties } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, type ApiClient } from '@/lib/api'
import {
  fetchMeets,
  fetchMeetCommitments,
  fetchMeetEntries,
  fetchMeetEvents,
  fetchSwimmers,
  fetchHouseholds,
  fetchCommitmentEventSelections,
  addCommitmentEventSelection,
  deleteCommitmentEventSelection,
  approveEntry,
  lockMeetCommitments,
  type CommitmentRecord,
  type SwimmerRecord,
  type HouseholdRecord,
  type MeetEventRecord,
  type EntryRecord,
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
  note?: string
}

type SwimmerPanelData = {
  row: CommitmentRow
  swimmer: SwimmerRecord
  events: MeetEventRecord[]
  entries: EntryRecord[]
  api: ApiClient
  orgId: string
}

// ─── Config ───────────────────────────────────────────────────────────────────

const STATUS_CFG: Record<string, { label: string; color: string; bg: string }> = {
  invited:        { label: 'Awaiting',  color: 'var(--text-muted)',  bg: 'var(--bg-elevated)' },
  committed_yes:  { label: 'Going',     color: 'var(--success)',     bg: 'var(--success-bg)' },
  committed_no:   { label: 'Not Going', color: 'var(--error)',       bg: 'var(--error-bg)' },
  locked:         { label: 'Locked',    color: 'var(--accent)',      bg: 'var(--accent-subtle)' },
  coach_reviewed: { label: 'Reviewed',  color: 'var(--info)',        bg: 'var(--info-bg)' },
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

const ENTRY_STATUS_CFG: Record<string, { label: string; color: string; bg: string }> = {
  draft:                { label: 'Draft',     color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  submitted:            { label: 'Submitted', color: 'var(--warning)',    bg: 'var(--warning-bg)' },
  approved:             { label: 'Approved',  color: 'var(--success)',    bg: 'var(--success-bg)' },
  rejected:             { label: 'Rejected',  color: 'var(--error)',      bg: 'var(--error-bg)' },
  exported:             { label: 'Exported',  color: 'var(--info)',       bg: 'var(--info-bg)' },
  updated_from_results: { label: 'Updated',   color: 'var(--info)',       bg: 'var(--info-bg)' },
  archived:             { label: 'Archived',  color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

const COMMITMENT_STATUSES = new Set(['commitment_open', 'commitment_closed', 'entry_finalized', 'completed'])

const STROKE_LABELS: Record<string, string> = {
  free: 'Freestyle', back: 'Backstroke', breast: 'Breaststroke', fly: 'Butterfly', im: 'IM', medley: 'Medley',
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function eventLabel(ev: MeetEventRecord): string {
  const dist = ev.distance ? `${ev.distance}` : ''
  const stroke = STROKE_LABELS[ev.stroke?.toLowerCase() ?? ''] ?? ev.stroke ?? ''
  const age = ev.age_group ? ` · ${ev.age_group}` : ''
  const gender = ev.gender ? ` (${ev.gender === 'F' ? 'Girls' : ev.gender === 'M' ? 'Boys' : ev.gender})` : ''
  return [dist, stroke].filter(Boolean).join(' ') + gender + age || `Event #${ev.event_number ?? ev.id.slice(-4)}`
}

// ─── Swimmer detail panel ─────────────────────────────────────────────────────

function SwimmerPanel({
  data,
  onClose,
  onRefreshTable,
}: {
  data: SwimmerPanelData
  onClose: () => void
  onRefreshTable: () => void
}) {
  const { row, swimmer, events, api, orgId } = data
  const commitmentId = row.id
  const meetId = row.meetId

  // ── Panel-local state ────────────────────────────────────────────────────────
  const [selections, setSelections] = useState<Map<string, string>>(new Map()) // meetEventId → selectionId
  const [selectionsLoading, setSelectionsLoading] = useState(true)
  const [toggling, setToggling] = useState<Set<string>>(new Set())             // meetEventId → pending
  const [localEntries, setLocalEntries] = useState<EntryRecord[]>(data.entries)
  const [approvingEntry, setApprovingEntry] = useState<Set<string>>(new Set()) // entryId → pending
  const [locking, setLocking] = useState(false)
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)
  const feedbackTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Flash feedback for 3 seconds — clears on unmount to avoid setState-on-unmounted-component
  useEffect(() => () => { if (feedbackTimerRef.current) clearTimeout(feedbackTimerRef.current) }, [])
  const showFeedback = useCallback((type: 'success' | 'error', msg: string) => {
    if (feedbackTimerRef.current) clearTimeout(feedbackTimerRef.current)
    setFeedback({ type, msg })
    feedbackTimerRef.current = setTimeout(() => setFeedback(null), 3000)
  }, [])

  // ── Load event selections on mount ──────────────────────────────────────────
  useEffect(() => {
    let cancelled = false
    setSelectionsLoading(true)
    fetchCommitmentEventSelections(api, orgId, meetId, commitmentId)
      .then((sels) => {
        if (cancelled) return
        const map = new Map<string, string>()
        for (const s of sels) map.set(s.meet_event_id, s.id)
        setSelections(map)
      })
      .catch(() => {
        // Selections unavailable (e.g., commitment not in commitment_open) — fall back to entries
        if (cancelled) return
        const map = new Map<string, string>()
        for (const e of data.entries) {
          if (e.meet_event_id) map.set(e.meet_event_id, e.id)
        }
        setSelections(map)
      })
      .finally(() => { if (!cancelled) setSelectionsLoading(false) })
    return () => { cancelled = true }
  }, [api, orgId, meetId, commitmentId, data.entries])

  // ── Toggle event selection ───────────────────────────────────────────────────
  const canEditSelections = row.meetStatus === 'commitment_open' && row.status === 'committed_yes'

  const toggleSelection = useCallback(async (ev: MeetEventRecord) => {
    if (!canEditSelections) return
    const eventId = ev.id
    setToggling((prev) => new Set(prev).add(eventId))
    try {
      const selId = selections.get(eventId)
      if (selId) {
        // Deselect
        await deleteCommitmentEventSelection(api, orgId, meetId, commitmentId, selId)
        setSelections((prev) => { const m = new Map(prev); m.delete(eventId); return m })
      } else {
        // Select
        const sel = await addCommitmentEventSelection(api, orgId, meetId, commitmentId, eventId)
        setSelections((prev) => new Map(prev).set(sel.meet_event_id, sel.id))
      }
    } catch (err) {
      showFeedback('error', err instanceof Error ? err.message : 'Failed to update selection')
    } finally {
      setToggling((prev) => { const s = new Set(prev); s.delete(eventId); return s })
    }
  }, [api, orgId, meetId, commitmentId, selections, canEditSelections, showFeedback])

  // ── Approve entry ────────────────────────────────────────────────────────────
  const handleApproveEntry = useCallback(async (entryId: string) => {
    setApprovingEntry((prev) => new Set(prev).add(entryId))
    try {
      const updated = await approveEntry(api, orgId, meetId, entryId)
      setLocalEntries((prev) => prev.map((e) => e.id === entryId ? { ...e, status: updated.status } : e))
      showFeedback('success', 'Entry approved')
    } catch (err) {
      showFeedback('error', err instanceof Error ? err.message : 'Failed to approve entry')
    } finally {
      setApprovingEntry((prev) => { const s = new Set(prev); s.delete(entryId); return s })
    }
  }, [api, orgId, meetId, showFeedback])

  // ── Lock all commitments for this meet ───────────────────────────────────────
  const handleLock = useCallback(async () => {
    if (!window.confirm(`Lock ALL commitments for "${row.meetName}"? This cannot be undone and will prevent further changes.`)) return
    setLocking(true)
    try {
      await lockMeetCommitments(api, orgId, meetId)
      showFeedback('success', 'All commitments locked for this meet')
      onRefreshTable()
    } catch (err) {
      showFeedback('error', err instanceof Error ? err.message : 'Failed to lock commitments')
    } finally {
      setLocking(false)
    }
  }, [api, orgId, meetId, row.meetName, showFeedback, onRefreshTable])

  const commitCfg = STATUS_CFG[row.status] ?? STATUS_CFG['invited']
  const meetStatusLabel = MEET_STATUS_CFG[row.meetStatus]?.label ?? row.meetStatus

  // Group events by session
  const sessionGroups = useMemo(() => {
    const map = new Map<string, MeetEventRecord[]>()
    for (const ev of events) {
      const sid = ev.session_id ?? 'main'
      const bucket = map.get(sid) ?? []
      bucket.push(ev)
      map.set(sid, bucket)
    }
    return [...map.entries()].map(([id, evs]) => ({ id, events: evs }))
  }, [events])

  const selectedCount = selections.size

  return (
    <>
      {/* Backdrop */}
      <div
        style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.45)', zIndex: 200, backdropFilter: 'blur(2px)' }}
        onClick={onClose}
      />

      {/* Slide-over */}
      <div
        style={{
          position: 'fixed',
          top: 0,
          right: 0,
          bottom: 0,
          width: '560px',
          maxWidth: '100vw',
          background: 'var(--bg)',
          borderLeft: '1px solid var(--border)',
          zIndex: 201,
          display: 'flex',
          flexDirection: 'column',
          animation: 'slideInRight 0.25s cubic-bezier(0.16,1,0.3,1) both',
          overflow: 'hidden',
        }}
      >
        {/* Header */}
        <div style={{ padding: '18px 22px', borderBottom: '1px solid var(--border)', display: 'flex', alignItems: 'center', gap: '12px', flexShrink: 0 }}>
          <div
            style={{
              width: '40px',
              height: '40px',
              borderRadius: '50%',
              background: 'linear-gradient(135deg, var(--accent) 0%, #b84a18 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#fff',
              fontSize: '14px',
              fontWeight: 700,
              fontFamily: 'var(--font-mono)',
              flexShrink: 0,
            }}
          >
            {(swimmer.first_name[0] ?? '') + (swimmer.last_name[0] ?? '')}
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: '16px', fontFamily: 'var(--font-display)', color: 'var(--text)', lineHeight: 1.2 }}>
              {swimmer.first_name} {swimmer.last_name}
            </div>
            <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '2px' }}>
              {row.meetName} · {row.meetDate ? formatDate(row.meetDate) : '—'}
            </div>
          </div>
          <button
            onClick={onClose}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: '4px', borderRadius: '6px', display: 'flex', alignItems: 'center' }}
            title="Close"
          >
            <Icon name="x" size={18} />
          </button>
        </div>

        {/* Inline feedback banner */}
        {feedback && (
          <div style={{
            padding: '10px 22px',
            background: feedback.type === 'success' ? 'var(--success-bg)' : 'var(--error-bg)',
            color: feedback.type === 'success' ? 'var(--success)' : 'var(--error)',
            fontSize: '12px',
            fontWeight: 600,
            fontFamily: 'var(--font-mono)',
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
            flexShrink: 0,
          }}>
            <Icon name={feedback.type === 'success' ? 'check' : 'alert-circle'} size={13} />
            {feedback.msg}
          </div>
        )}

        {/* Scrollable body */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '20px 22px' }}>

          {/* Status & meet info row */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px', marginBottom: '20px' }}>
            <div className="card" style={{ padding: '12px 14px' }}>
              <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '5px' }}>
                Commitment
              </div>
              <span style={{ fontSize: '12px', fontWeight: 700, padding: '3px 10px', borderRadius: '10px', background: commitCfg.bg, color: commitCfg.color, fontFamily: 'var(--font-mono)' }}>
                {commitCfg.label}
              </span>
            </div>
            <div className="card" style={{ padding: '12px 14px' }}>
              <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '5px' }}>
                Meet Status
              </div>
              <span style={{ fontSize: '12px', color: MEET_STATUS_CFG[row.meetStatus]?.color ?? 'var(--text-muted)', fontFamily: 'var(--font-mono)', fontWeight: 600 }}>
                {meetStatusLabel}
              </span>
            </div>
          </div>

          {/* Swimmer note */}
          {row.note && (
            <div
              className="card"
              style={{ padding: '12px 14px', marginBottom: '20px', borderLeft: '3px solid var(--accent)' }}
            >
              <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '5px' }}>
                Note from family
              </div>
              <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.55, margin: 0 }}>{row.note}</p>
            </div>
          )}

          {/* Event selections */}
          <div style={{ marginBottom: '20px' }}>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '8px', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span>Event Selections</span>
              {selectionsLoading ? (
                <span style={{ color: 'var(--text-muted)' }}>Loading…</span>
              ) : (
                <span style={{ color: selectedCount > 0 ? 'var(--accent)' : 'var(--text-muted)' }}>
                  {selectedCount} selected
                </span>
              )}
            </div>

            {canEditSelections && (
              <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginBottom: '10px', lineHeight: 1.5 }}>
                Click events below to add or remove them from this swimmer&apos;s selection.
              </p>
            )}

            {events.length === 0 ? (
              <div className="card" style={{ padding: '24px', textAlign: 'center' }}>
                <p style={{ fontSize: '13px', color: 'var(--text-muted)', margin: 0 }}>No events for this meet yet.</p>
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                {sessionGroups.map((group) => (
                  <div key={group.id}>
                    <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '6px', paddingLeft: '2px' }}>
                      {group.id === 'main' ? 'Main Session' : `Session ${group.id.slice(-4)}`}
                    </div>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '5px' }}>
                      {group.events.map((ev) => {
                        const isSelected = selections.has(ev.id)
                        const isPending = toggling.has(ev.id)
                        return (
                          <button
                            key={ev.id}
                            onClick={() => toggleSelection(ev)}
                            disabled={!canEditSelections || isPending || selectionsLoading}
                            title={canEditSelections ? (isSelected ? 'Click to deselect' : 'Click to select') : 'Selections can only be edited when commitments are open'}
                            style={{
                              display: 'flex',
                              alignItems: 'center',
                              gap: '10px',
                              padding: '10px 12px',
                              borderRadius: '8px',
                              border: isSelected ? '1.5px solid var(--accent)' : '1px solid var(--border)',
                              background: isSelected ? 'var(--accent-subtle)' : 'var(--bg-surface)',
                              transition: 'all 120ms ease',
                              cursor: canEditSelections ? (isPending ? 'wait' : 'pointer') : 'default',
                              width: '100%',
                              textAlign: 'left',
                              opacity: isPending ? 0.6 : 1,
                            }}
                          >
                            <div
                              style={{
                                width: '16px',
                                height: '16px',
                                borderRadius: '4px',
                                border: isSelected ? '2px solid var(--accent)' : '1.5px solid var(--border)',
                                background: isSelected ? 'var(--accent)' : 'transparent',
                                flexShrink: 0,
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                              }}
                            >
                              {isSelected && !isPending && (
                                <svg viewBox="0 0 10 8" fill="none" width="9" height="9">
                                  <path d="M1 4l3 3 5-6" stroke="#fff" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                                </svg>
                              )}
                              {isPending && (
                                <div style={{ width: '8px', height: '8px', borderRadius: '50%', border: '1.5px solid var(--accent)', borderTopColor: 'transparent', animation: 'spin 0.6s linear infinite' }} />
                              )}
                            </div>
                            <span style={{ fontSize: '13px', color: isSelected ? 'var(--text)' : 'var(--text-subtle)', flex: 1, fontWeight: isSelected ? 600 : 400 }}>
                              {eventLabel(ev)}
                            </span>
                            {ev.event_number && (
                              <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                                #{ev.event_number}
                              </span>
                            )}
                          </button>
                        )
                      })}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Entry records with approve actions */}
          {localEntries.length > 0 && (
            <div style={{ marginBottom: '20px' }}>
              <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '10px' }}>
                Entry Records ({localEntries.length})
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                {localEntries.map((e) => {
                  const cfg = ENTRY_STATUS_CFG[e.status] ?? { label: e.status, color: 'var(--text-muted)', bg: 'var(--bg-elevated)' }
                  const isApproving = approvingEntry.has(e.id)
                  return (
                    <div key={e.id} className="card" style={{ padding: '9px 12px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '10px' }}>
                      <span style={{ fontSize: '12px', color: 'var(--text)', fontFamily: 'var(--font-mono)', flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        Entry #{e.id.slice(-6)}
                      </span>
                      <span style={{ fontSize: '11px', padding: '2px 8px', borderRadius: '8px', background: cfg.bg, color: cfg.color, fontFamily: 'var(--font-mono)', fontWeight: 600, whiteSpace: 'nowrap' }}>
                        {cfg.label}
                      </span>
                      {e.status === 'submitted' && (
                        <button
                          onClick={() => handleApproveEntry(e.id)}
                          disabled={isApproving}
                          style={{
                            fontSize: '11px',
                            padding: '3px 10px',
                            borderRadius: '6px',
                            background: isApproving ? 'var(--bg-elevated)' : 'var(--success)',
                            color: isApproving ? 'var(--text-muted)' : '#fff',
                            border: 'none',
                            cursor: isApproving ? 'wait' : 'pointer',
                            fontWeight: 600,
                            whiteSpace: 'nowrap',
                            transition: 'all 150ms ease',
                          }}
                        >
                          {isApproving ? '…' : 'Approve'}
                        </button>
                      )}
                    </div>
                  )
                })}
              </div>
            </div>
          )}
        </div>

        {/* Footer actions */}
        <div style={{ padding: '14px 22px', borderTop: '1px solid var(--border)', display: 'flex', gap: '8px', flexShrink: 0, background: 'var(--bg)', flexWrap: 'wrap' }}>
          <Link
            href={`/meets/${row.meetId}`}
            style={{ flex: 1, minWidth: '120px', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '5px', padding: '10px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '8px', fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none', transition: 'border-color 0.15s' }}
          >
            <Icon name="calendar" size={13} />
            Open Meet
          </Link>
          <Link
            href={`/members/swimmers/${row.swimmerId}`}
            style={{ flex: 1, minWidth: '120px', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '5px', padding: '10px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '8px', fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none', transition: 'border-color 0.15s' }}
          >
            <Icon name="user" size={13} />
            Swimmer Profile
          </Link>
          {(row.meetStatus === 'commitment_open' || row.meetStatus === 'commitment_closed') && (
            <button
              onClick={handleLock}
              disabled={locking}
              style={{
                flex: 1,
                minWidth: '140px',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: '5px',
                padding: '10px',
                background: locking ? 'var(--bg-elevated)' : 'var(--accent)',
                border: 'none',
                borderRadius: '8px',
                fontSize: '13px',
                fontWeight: 600,
                color: locking ? 'var(--text-muted)' : '#fff',
                cursor: locking ? 'wait' : 'pointer',
                transition: 'all 0.15s',
              }}
            >
              <Icon name="shield-check" size={13} />
              {locking ? 'Locking…' : 'Lock All Commitments'}
            </button>
          )}
        </div>
      </div>

      <style>{`
        @keyframes slideInRight {
          from { transform: translateX(100%); opacity: 0; }
          to   { transform: translateX(0);    opacity: 1; }
        }
        @keyframes spin {
          to { transform: rotate(360deg); }
        }
      `}</style>
    </>
  )
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
  const [refreshKey, setRefreshKey] = useState(0)

  // Panel state
  const [panelData, setPanelData] = useState<SwimmerPanelData | null>(null)
  const [panelLoading, setPanelLoading] = useState(false)

  // Full data for panel lookups
  const [allSwimmersMap, setAllSwimmersMap] = useState<Map<string, SwimmerRecord>>(new Map())
  const [meetEventsCache, setMeetEventsCache] = useState<Map<string, MeetEventRecord[]>>(new Map())
  const [meetEntriesCache, setMeetEntriesCache] = useState<Map<string, Map<string, EntryRecord[]>>>(new Map())
  const openPanelLoadId = useRef(0) // monotonic counter to discard stale concurrent opens

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
        setAllSwimmersMap(swimmerMap)

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
              note: c.note,
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
  }, [session?.user.orgId, api, refreshKey])

  const openPanel = useCallback(async (row: CommitmentRow) => {
    const swimmer = allSwimmersMap.get(row.swimmerId)
    if (!swimmer || !session?.user.orgId) return
    const orgId = session.user.orgId

    // Increment load ID; if another open starts before this one resolves, we discard our result
    const myLoadId = ++openPanelLoadId.current
    setPanelLoading(true)
    setPanelData(null)

    // Check event cache
    let events = meetEventsCache.get(row.meetId)
    if (!events) {
      events = await fetchMeetEvents(api, orgId, row.meetId).catch(() => [] as MeetEventRecord[])
      setMeetEventsCache((prev) => new Map(prev).set(row.meetId, events!))
    }

    // Check entries cache
    let meetEntriesForSwimmer: EntryRecord[] = []
    let swimmerEntriesMap = meetEntriesCache.get(row.meetId)
    if (!swimmerEntriesMap) {
      const allEntries = await fetchMeetEntries(api, orgId, row.meetId).catch(() => [] as EntryRecord[])
      swimmerEntriesMap = new Map<string, EntryRecord[]>()
      for (const e of allEntries) {
        const bucket = swimmerEntriesMap.get(e.swimmer_id) ?? []
        bucket.push(e)
        swimmerEntriesMap.set(e.swimmer_id, bucket)
      }
      setMeetEntriesCache((prev) => new Map(prev).set(row.meetId, swimmerEntriesMap!))
    }
    meetEntriesForSwimmer = swimmerEntriesMap.get(row.swimmerId) ?? []

    if (myLoadId !== openPanelLoadId.current) return // stale — a newer open superseded us
    setPanelLoading(false)
    setPanelData({ row, swimmer, events, entries: meetEntriesForSwimmer, api, orgId })
  }, [api, session?.user.orgId, allSwimmersMap, meetEventsCache, meetEntriesCache])

  // Stats
  const total    = rows.length
  const going    = rows.filter((r) => r.status === 'committed_yes').length
  const notGoing = rows.filter((r) => r.status === 'committed_no').length
  const pending  = rows.filter((r) => r.status === 'invited').length
  const locked   = rows.filter((r) => r.status === 'locked' || r.status === 'coach_reviewed').length

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
          <button
            onClick={() => openPanel(r)}
            style={{ background: 'none', border: 'none', padding: 0, cursor: 'pointer', textAlign: 'left' }}
          >
            <span
              style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px' }}
              onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.color = 'var(--accent)' }}
              onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.color = 'var(--text)' }}
            >
              {r.swimmer}
              <Icon name="chevron-right" size={12} color="var(--text-muted)" />
            </span>
          </button>
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
          {r.note && (
            <div style={{ fontSize: '10px', color: 'var(--accent)', marginTop: '2px', display: 'flex', alignItems: 'center', gap: '3px' }}>
              <Icon name="info" size={10} />
              has note
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
        <button
          onClick={() => openPanel(r)}
          style={{ fontSize: '12px', color: 'var(--text-muted)', background: 'none', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '3px', padding: '2px 4px', borderRadius: '4px', transition: 'color 0.15s' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}
        >
          {panelLoading ? 'Loading…' : 'View'} <Icon name="chevron-right" size={12} />
        </button>
      ),
    },
  ], [openPanel, panelLoading])

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
            Cross-meet commitment tracker — click a swimmer to view their entries and event selections
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
          <Icon name="search" size={14} color="var(--text-muted)" style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' } as CSSProperties} />
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
          {filtered.length} of {total} commitments · click a swimmer name to view their entries
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

      {/* Swimmer panel */}
      {panelData && (
        <SwimmerPanel
          data={panelData}
          onClose={() => setPanelData(null)}
          onRefreshTable={() => {
            // Re-trigger the main data load so commitment statuses update after locking
            setPanelData(null)
            setMeetEntriesCache(new Map())
            setRefreshKey((k) => k + 1)
          }}
        />
      )}

      <style>{`@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>
    </div>
  )
}
