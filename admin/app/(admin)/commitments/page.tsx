'use client'

import { Suspense, useState, useMemo, useEffect, useCallback, useRef, type CSSProperties } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
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
import { formatDateOnly } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { useConfirm } from '@/components/ui/ConfirmDialog'
import { useDialogA11y } from '@/components/ui/useDialogA11y'
import './page.inline.css'

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
  committed_no:   { label: 'Not going', color: 'var(--error)',       bg: 'var(--error-bg)' },
  locked:         { label: 'Locked',    color: 'var(--accent-text, var(--accent))',      bg: 'var(--accent-subtle)' },
  coach_reviewed: { label: 'Reviewed',  color: 'var(--info)',        bg: 'var(--info-bg)' },
}

const MEET_STATUS_CFG: Record<string, { label: string; color: string }> = {
  draft:             { label: 'Draft',              color: 'var(--text-muted)' },
  published:         { label: 'Published',          color: 'var(--info)' },
  commitment_open:   { label: 'Commitments open',   color: 'var(--warning)' },
  commitment_closed: { label: 'Commitments closed', color: 'var(--warning)' },
  entry_finalized:   { label: 'Entries finalized',  color: 'var(--accent-text, var(--accent))' },
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
  meetCounts,
}: {
  data: SwimmerPanelData
  onClose: () => void
  onRefreshTable: () => void
  /** Commitment statuses for this meet, used to describe what lock-all will do. */
  meetCounts: { total: number; lockable: number }
}) {
  const panelRef = useRef<HTMLDivElement>(null)
  useDialogA11y(panelRef, onClose)
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
      showFeedback('error', uiErrorMessage(err, 'Failed to update selection'))
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
      showFeedback('error', uiErrorMessage(err, 'Failed to approve entry'))
    } finally {
      setApprovingEntry((prev) => { const s = new Set(prev); s.delete(entryId); return s })
    }
  }, [api, orgId, meetId, showFeedback])

  // ── Lock all commitments for this meet ───────────────────────────────────────
  const confirm = useConfirm()
  const handleLock = useCallback(async () => {
    const left = meetCounts.total - meetCounts.lockable
    const ok = await confirm({ title: `Lock commitments for ${row.meetName}?`, message: `Of ${meetCounts.total} commitment${meetCounts.total === 1 ? '' : 's'}, ${meetCounts.lockable} (committed yes or coach reviewed) will be locked and their event selections become entries. ${left} (declined or no response) will be left as they are. Locking cannot be undone; families can no longer change their answers.`, typedPhrase: 'LOCK', confirmLabel: 'Lock commitments', destructive: true })
    if (!ok) return
    setLocking(true)
    try {
      await lockMeetCommitments(api, orgId, meetId)
      showFeedback('success', 'All commitments locked for this meet')
      onRefreshTable()
    } catch (err) {
      showFeedback('error', uiErrorMessage(err, 'Failed to lock commitments'))
    } finally {
      setLocking(false)
    }
  }, [api, orgId, meetId, row.meetName, showFeedback, onRefreshTable, confirm, meetCounts])

  const commitCfg = STATUS_CFG[row.status] ?? STATUS_CFG.invited
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
        style={{ position: 'fixed', inset: 0, background: 'var(--scrim)', zIndex: 200, backdropFilter: 'blur(2px)' }}
        onClick={onClose}
      />

      {/* Slide-over */}
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label="Swimmer commitments"
        tabIndex={-1}
        style={{
          outline: 'none',
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
        <div className="u-p-20px-24px u-gap-12px" style={{ borderBottom: '1px solid var(--border)', display: 'flex', alignItems: 'center', flexShrink: 0 }}>
          <div
            className="u-fs-fs-md u-fw-700 u-ff-font-body" style={{ width: '40px', height: '40px', borderRadius: '50%', background: 'linear-gradient(135deg, var(--accent) 0%, #b84a18 100%)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', flexShrink: 0 }}
          >
            {(swimmer.first_name[0] ?? '') + (swimmer.last_name[0] ?? '')}
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="u-fs-fs-lg u-ff-font-display u-fw-display-weight u-lh-1_2" style={{ color: 'var(--text)' }}>
              {swimmer.first_name} {swimmer.last_name}
            </div>
            <div className="u-fs-fs-sm u-mt-2px" style={{ color: 'var(--text-muted)' }}>
              {row.meetName} · {row.meetDate ? formatDateOnly(row.meetDate) : '—'}
            </div>
          </div>
          <button type="button"
            onClick={onClose}
            className="u-p-4px" style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', borderRadius: '6px', display: 'flex', alignItems: 'center' }}
            title="Close"
          >
            <Icon name="x" size={18} />
          </button>
        </div>

        {/* Inline feedback banner */}
        {feedback && (
          <div className="u-p-12px-24px u-fs-fs-sm u-fw-600 u-ff-font-body u-gap-6px" style={{ background: feedback.type === 'success' ? 'var(--success-bg)' : 'var(--error-bg)', color: feedback.type === 'success' ? 'var(--success)' : 'var(--error)', display: 'flex', alignItems: 'center', flexShrink: 0 }}>
            <Icon name={feedback.type === 'success' ? 'check' : 'alert-circle'} size={13} />
            {feedback.msg}
          </div>
        )}

        {/* Scrollable body */}
        <div className="u-p-20px-24px" style={{ flex: 1, overflowY: 'auto' }}>

          {/* Status & meet info row */}
          <div className="cols-2 u-gap-12px u-mb-20px" >
            <div className="card u-p-12px-16px" >
              <div className="u-fs-fs-sm u-ff-font-body u-mb-4px" style={{ color: 'var(--text-muted)' }}>
                Commitment
              </div>
              <span className="badge" style={{ background: commitCfg.bg, color: commitCfg.color }}>
                {commitCfg.label}
              </span>
            </div>
            <div className="card u-p-12px-16px" >
              <div className="u-fs-fs-sm u-ff-font-body u-mb-4px" style={{ color: 'var(--text-muted)' }}>
                Meet status
              </div>
              <span className="u-fs-fs-sm u-ff-font-body u-fw-600" style={{ color: MEET_STATUS_CFG[row.meetStatus]?.color ?? 'var(--text-muted)' }}>
                {meetStatusLabel}
              </span>
            </div>
          </div>

          {/* Swimmer note */}
          {row.note && (
            <div
              className="card u-p-12px-16px u-mb-20px"
              style={{ borderColor: 'var(--accent-border)', background: 'var(--accent-subtle)' }}
            >
              <div className="u-fs-fs-sm u-ff-font-body u-mb-4px" style={{ color: 'var(--text-muted)' }}>
                Note from family
              </div>
              <p className="u-fs-fs-base u-lh-1_55 u-m-0" style={{ color: 'var(--text-subtle)' }}>{row.note}</p>
            </div>
          )}

          {/* Event selections */}
          <div className="u-mb-20px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-8px" style={{ color: 'var(--text-muted)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span>Event selections</span>
              {selectionsLoading ? (
                <span style={{ color: 'var(--text-muted)' }}>Loading…</span>
              ) : (
                <span style={{ color: selectedCount > 0 ? 'var(--accent)' : 'var(--text-muted)' }}>
                  {selectedCount} selected
                </span>
              )}
            </div>

            {canEditSelections && (
              <p className="u-fs-fs-xs u-mb-12px u-lh-1_5" style={{ color: 'var(--text-muted)' }}>
                Click events below to add or remove them from this swimmer&apos;s selection.
              </p>
            )}

            {events.length === 0 ? (
              <div className="card u-p-24px" style={{ textAlign: 'center' }}>
                <p className="u-fs-fs-base u-m-0" style={{ color: 'var(--text-muted)' }}>No events for this meet yet.</p>
              </div>
            ) : (
              <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
                {sessionGroups.map((group) => (
                  <div key={group.id}>
                    <div className="u-fs-fs-xs u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)', paddingLeft: '2px' }}>
                      {group.id === 'main' ? 'Main Session' : 'Unscheduled'}
                    </div>
                    <div className="u-gap-4px" style={{ display: 'flex', flexDirection: 'column' }}>
                      {group.events.map((ev) => {
                        const isSelected = selections.has(ev.id)
                        const isPending = toggling.has(ev.id)
                        return (
                          <button type="button"
                            key={ev.id}
                            onClick={() => toggleSelection(ev)}
                            disabled={!canEditSelections || isPending || selectionsLoading}
                            title={canEditSelections ? (isSelected ? 'Click to deselect' : 'Click to select') : 'Selections can only be edited when commitments are open'}
                            className="u-gap-12px u-p-12px-12px" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', border: isSelected ? '1.5px solid var(--accent)' : '1px solid var(--border)', background: isSelected ? 'var(--accent-subtle)' : 'var(--bg-surface)', transition: 'all 120ms ease', cursor: canEditSelections ? (isPending ? 'wait' : 'pointer') : 'default', width: '100%', textAlign: 'left', opacity: isPending ? 0.6 : 1 }}
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
                            <span className="u-fs-fs-base" style={{ color: isSelected ? 'var(--text)' : 'var(--text-subtle)', flex: 1, fontWeight: isSelected ? 600 : 400 }}>
                              {eventLabel(ev)}
                            </span>
                            {ev.event_number && (
                              <span className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
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
            <div className="u-mb-20px" >
              <div className="u-fs-fs-sm u-ff-font-body u-mb-12px" style={{ color: 'var(--text-muted)' }}>
                Entry Records ({localEntries.length})
              </div>
              <div className="u-gap-6px" style={{ display: 'flex', flexDirection: 'column' }}>
                {localEntries.map((e) => {
                  const cfg = ENTRY_STATUS_CFG[e.status] ?? { label: e.status, color: 'var(--text-muted)', bg: 'var(--bg-elevated)' }
                  const isApproving = approvingEntry.has(e.id)
                  const entryEvent = events.find((ev) => ev.id === e.meet_event_id)
                  return (
                    <div key={e.id} className="card u-p-8px-12px u-gap-12px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text)', flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {entryEvent ? eventLabel(entryEvent) : `Entry #${e.id.slice(-6)}`}
                      </span>
                      <span className="u-fs-fs-xs u-p-2px-8px u-ff-font-body u-fw-600" style={{ borderRadius: '8px', background: cfg.bg, color: cfg.color, whiteSpace: 'nowrap' }}>
                        {cfg.label}
                      </span>
                      {e.status === 'submitted' && (
                        <button type="button"
                          onClick={() => handleApproveEntry(e.id)}
                          disabled={isApproving}
                          className="u-fs-fs-xs u-p-4px-12px u-fw-600" style={{ borderRadius: '6px', background: isApproving ? 'var(--bg-elevated)' : 'var(--success)', color: isApproving ? 'var(--text-muted)' : '#fff', border: 'none', cursor: isApproving ? 'wait' : 'pointer', whiteSpace: 'nowrap', transition: 'all 150ms ease' }}
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
        <div className="u-p-16px-24px u-gap-8px" style={{ borderTop: '1px solid var(--border)', display: 'flex', flexShrink: 0, background: 'var(--bg)', flexWrap: 'wrap' }}>
          <Link
            href={`/meets/${row.meetId}`}
            className="u-gap-4px u-p-12px u-fs-fs-base u-fw-600" style={{ flex: 1, minWidth: '120px', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', textDecoration: 'none', transition: 'border-color 0.15s' }}
          >
            <Icon name="calendar" size={13} />
            Open Meet
          </Link>
          <Link
            href={`/members/swimmers/${row.swimmerId}`}
            className="u-gap-4px u-p-12px u-fs-fs-base u-fw-600" style={{ flex: 1, minWidth: '120px', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', textDecoration: 'none', transition: 'border-color 0.15s' }}
          >
            <Icon name="user" size={13} />
            Swimmer Profile
          </Link>
          {(row.meetStatus === 'commitment_open' || row.meetStatus === 'commitment_closed') && (
            <button type="button"
              onClick={handleLock}
              disabled={locking}
              className="u-gap-4px u-p-12px u-fs-fs-base u-fw-600" style={{ flex: 1, minWidth: '140px', display: 'flex', alignItems: 'center', justifyContent: 'center', background: locking ? 'var(--bg-elevated)' : 'var(--accent)', border: 'none', borderRadius: '8px', color: locking ? 'var(--text-muted)' : '#fff', cursor: locking ? 'wait' : 'pointer', transition: 'all 0.15s' }}
            >
              <Icon name="shield-check" size={13} />
              {locking ? 'Locking…' : 'Lock All Commitments'}
            </button>
          )}
        </div>
      </div>

    </>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function CommitmentsPage() {
  return (
    <Suspense fallback={null}>
      <CommitmentsPageContent />
    </Suspense>
  )
}

function CommitmentsPageContent() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const searchParams = useSearchParams()
  const lockedMeetId = searchParams.get('meet')

  const [rows, setRows] = useState<CommitmentRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [meetFilter, setMeetFilter] = useState(lockedMeetId ?? 'all')
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
        if (!cancelled) setError(uiErrorMessage(err, 'Commitments couldn’t be loaded. Check your connection and try again.'))
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

    const [events, swimmerEntriesMap] = await Promise.all([
      meetEventsCache.get(row.meetId)
        ?? fetchMeetEvents(api, orgId, row.meetId).catch(() => [] as MeetEventRecord[]).then((fetched) => {
          setMeetEventsCache((prev) => new Map(prev).set(row.meetId, fetched))
          return fetched
        }),
      meetEntriesCache.get(row.meetId)
        ?? fetchMeetEntries(api, orgId, row.meetId).catch(() => [] as EntryRecord[]).then((allEntries) => {
          const bySwimmer = new Map<string, EntryRecord[]>()
          for (const e of allEntries) {
            const bucket = bySwimmer.get(e.swimmer_id) ?? []
            bucket.push(e)
            bySwimmer.set(e.swimmer_id, bucket)
          }
          setMeetEntriesCache((prev) => new Map(prev).set(row.meetId, bySwimmer))
          return bySwimmer
        }),
    ])
    const meetEntriesForSwimmer = swimmerEntriesMap.get(row.swimmerId) ?? []

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
    return [{ value: 'all', label: 'All meets' }, ...[...seen.entries()].map(([v, l]) => ({ value: v, label: l }))]
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
          <button type="button"
            onClick={() => openPanel(r)}
            className="u-p-0" style={{ background: 'none', border: 'none', cursor: 'pointer', textAlign: 'left' }}
          >
            <span
              className="u-fs-fs-base u-fw-600 u-gap-4px" style={{ color: 'var(--text)', textDecoration: 'none', display: 'flex', alignItems: 'center' }}
              onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.color = 'var(--accent)' }}
              onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.color = 'var(--text)' }}
            >
              {r.swimmer}
              <Icon name="chevron-right" size={12} color="var(--text-muted)" />
            </span>
          </button>
          {r.householdId && (
            <div className="u-fs-fs-xs u-mt-1px" style={{ color: 'var(--text-muted)' }}>
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
            <div className="u-fs-fs-2xs u-mt-2px u-gap-4px" style={{ color: 'var(--accent-text, var(--accent))', display: 'flex', alignItems: 'center' }}>
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
            className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)', textDecoration: 'none' }}
            onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
            onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}
          >
            {r.meetName}
          </Link>
          <div className="u-fs-fs-xs u-mt-1px u-ff-font-body" style={{ color: MEET_STATUS_CFG[r.meetStatus]?.color ?? 'var(--text-muted)' }}>
            {MEET_STATUS_CFG[r.meetStatus]?.label ?? r.meetStatus}
          </div>
        </div>
      ),
    },
    {
      key: 'meetDate',
      label: 'Date',
      render: (r) => (
        <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>
          {r.meetDate ? formatDateOnly(r.meetDate) : '—'}
        </span>
      ),
    },
    {
      key: 'status',
      label: 'Status',
      render: (r) => {
        const c = STATUS_CFG[r.status] ?? STATUS_CFG.invited
        return (
          <span className="badge" style={{ background: c.bg, color: c.color }}>
            {c.label}
          </span>
        )
      },
    },
    {
      key: 'id',
      label: '',
      render: (r) => (
        <button type="button"
          onClick={() => openPanel(r)}
          className="u-fs-fs-sm u-gap-4px u-p-2px-4px" style={{ color: 'var(--text-muted)', background: 'none', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', borderRadius: '4px', transition: 'color 0.15s' }}
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
      <div className="page">
        <div className="card u-p-20px" style={{ borderColor: 'var(--error-border)', background: 'var(--error-bg)' }}>
          <div className="row row-between" role="alert">
            <p className="u-fs-fs-base" style={{ color: 'var(--error)' }}>{error}</p>
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => setRefreshKey((k) => k + 1)}>Retry</button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="page">

      {lockedMeetId && (
        <Link href={`/meets/${lockedMeetId}`} className="u-gap-4px u-fs-fs-base u-mb-16px" style={{ display: 'inline-flex', alignItems: 'center', color: 'var(--text-subtle)', textDecoration: 'none' }}>
          <Icon name="chevron-left" size={14} /> Back to meet
        </Link>
      )}

      {/* Page header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">
            Commitments
          </h1>
        </div>
      </div>

      {/* Stats strip */}
      <div className="cols-5 u-gap-12px u-mb-24px" >
        {[
          { label: 'Total',     value: loading ? '—' : total,    color: 'var(--text)' },
          { label: 'Going',     value: loading ? '—' : going,    color: 'var(--success)' },
          { label: 'Not going', value: loading ? '—' : notGoing, color: 'var(--error)' },
          { label: 'Awaiting',  value: loading ? '—' : pending,  color: (pending > 0 && !loading) ? 'var(--warning)' : 'var(--text-muted)' },
          { label: 'Locked',    value: loading ? '—' : locked,   color: 'var(--accent-text, var(--accent))' },
        ].map((s) => (
          <div key={s.label} className="card u-p-16px-16px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{s.label}</div>
            <div className="u-fs-fs-2xl u-ff-font-body u-fw-700" style={{ color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Response rate bar */}
      {!loading && total > 0 && (
        <div className="card u-p-16px-16px u-mb-20px" >
          <div className="u-mb-8px u-fs-fs-sm u-ff-font-body" style={{ display: 'flex', justifyContent: 'space-between', color: 'var(--text-muted)' }}>
            <span>Response rate</span>
            <span>{Math.round(((going + notGoing + locked) / total) * 100)}%</span>
          </div>
          <div style={{ height: '5px', borderRadius: '4px', background: 'var(--bg-elevated)', overflow: 'hidden', display: 'flex' }}>
            <div style={{ height: '100%', width: `${(going / total) * 100}%`, background: 'var(--success)', transition: 'width 0.6s var(--ease-out)' }} />
            <div style={{ height: '100%', width: `${(locked / total) * 100}%`, background: 'var(--accent)', transition: 'width 0.6s var(--ease-out)' }} />
            <div style={{ height: '100%', width: `${(notGoing / total) * 100}%`, background: 'var(--error)', transition: 'width 0.6s var(--ease-out)' }} />
          </div>
          <div className="u-gap-16px u-mt-8px u-fs-fs-xs u-ff-font-body" style={{ display: 'flex', color: 'var(--text-muted)' }}>
            <span style={{ color: 'var(--success)' }}>■ Going ({going})</span>
            <span style={{ color: 'var(--accent-text, var(--accent))' }}>■ Locked ({locked})</span>
            <span style={{ color: 'var(--error)' }}>■ Not Going ({notGoing})</span>
            <span>■ Pending ({pending})</span>
          </div>
        </div>
      )}

      {/* Filters */}
      <div className="u-gap-12px u-mb-16px" style={{ display: 'flex', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 260px' }}>
          <Icon name="search" size={14} color="var(--text-muted)" style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' } as CSSProperties} />
          <input aria-label="Search swimmer, household, or meet"
            type="text"
            placeholder="Search swimmer, household, or meet…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="input-search input u-pl-32px"
            style={{ width: '100%' }}
          />
        </div>
        {lockedMeetId ? (
          <div className="u-gap-8px u-p-0-12px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', borderRadius: 'var(--r-md)', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)' }}>
            <Icon name="calendar" size={13} color="var(--text-muted)" />
            {meetOptions.find((o) => o.value === lockedMeetId)?.label ?? 'This meet'}
            <Link href="/commitments" className="u-fs-fs-xs" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none' }}>Clear</Link>
          </div>
        ) : (
          <select value={meetFilter} onChange={(e) => setMeetFilter(e.target.value)} className="input" style={{ minWidth: '180px' }}>
            {meetOptions.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        )}
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="input" style={{ minWidth: '150px' }}>
          <option value="all">All statuses</option>
          <option value="invited">Awaiting</option>
          <option value="committed_yes">Going</option>
          <option value="committed_no">Not going</option>
          <option value="locked">Locked</option>
        </select>
      </div>

      {/* Result count */}
      {!loading && (
        <div className="u-fs-fs-sm u-ff-font-body u-mb-12px" style={{ color: 'var(--text-muted)' }}>
          {filtered.length} of {total} commitments · click a swimmer name to view their entries
        </div>
      )}

      {/* Loading skeleton */}
      {loading && (
        <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
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
          meetCounts={(() => {
            const mine = rows.filter((r) => r.meetId === panelData.row.meetId)
            return { total: mine.length, lockable: mine.filter((r) => r.status === 'committed_yes' || r.status === 'coach_reviewed').length }
          })()}
          onRefreshTable={() => {
            // Re-trigger the main data load so commitment statuses update after locking
            setPanelData(null)
            setMeetEntriesCache(new Map())
            setRefreshKey((k) => k + 1)
          }}
        />
      )}

          </div>
  )
}
