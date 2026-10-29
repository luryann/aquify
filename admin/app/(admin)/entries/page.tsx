'use client'

import { Suspense, useCallback, useMemo, useRef, useState, useEffect, type CSSProperties } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchMeets,
  fetchMeetEntries,
  fetchMeetEvents,
  fetchSwimmers,
  scratchEntry,
  approveEntry,
  overrideEntry,
  type EntryRecord,
  type MeetEventRecord,
  type MeetRecord,
  type SwimmerRecord,
} from '@/lib/adminData'
import { formatMeetEntriesSaveError, saveMeetEntriesBatch } from '@/lib/meet-entry-save'
import { isSwimmerEligibleForEvent } from '@/lib/event-eligibility'
import { formatEventLabel } from '@/lib/meet-events'
import { formatDateOnly } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { useConfirm, useConfirmReason } from '@/components/ui/ConfirmDialog'
import { useDialogA11y } from '@/components/ui/useDialogA11y'
import './page.inline.css'

// ─── Types ────────────────────────────────────────────────────────────────────

type EntryStatus = 'draft' | 'submitted' | 'approved' | 'rejected' | 'exported' | 'updated_from_results' | 'archived' | 'scratched'

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
  scratched:            { label: 'Scratched', color: 'var(--error)',      bg: 'var(--error-bg)' },
}

const STATUS_FILTER_OPTIONS = [
  { value: 'all',       label: 'All statuses' },
  { value: 'draft',     label: 'Draft' },
  { value: 'submitted', label: 'Submitted' },
  { value: 'approved',  label: 'Approved' },
  { value: 'rejected',  label: 'Rejected' },
  { value: 'exported',  label: 'Exported' },
  { value: 'scratched', label: 'Scratched' },
]

const ENTRY_STATUSES = new Set(['commitment_open', 'commitment_closed', 'entry_finalized', 'completed'])

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatSeedTime(ms: number | undefined): string {
  if (!ms || ms <= 0) return 'NT'
  const totalHundredths = Math.round(ms / 10)
  const h = Math.floor(totalHundredths / 360000)
  const m = Math.floor((totalHundredths % 360000) / 6000)
  const s = Math.floor((totalHundredths % 6000) / 100)
  const cs = totalHundredths % 100
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}.${String(cs).padStart(2, '0')}`
  if (m > 0) return `${m}:${String(s).padStart(2, '0')}.${String(cs).padStart(2, '0')}`
  return `${s}.${String(cs).padStart(2, '0')}`
}

// ─── Add Entry Panel ──────────────────────────────────────────────────────────

function AddEntryPanel({
  swimmers,
  meets,
  api,
  orgId,
  onClose,
  onSaved,
}: {
  swimmers: SwimmerRecord[]
  meets: MeetRecord[]
  api: ReturnType<typeof useApiClient>
  orgId: string
  onClose: () => void
  onSaved: () => void
}) {
  const panelRef = useRef<HTMLDivElement>(null)
  useDialogA11y(panelRef, onClose)
  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [selectedSwimmer, setSelectedSwimmer] = useState<SwimmerRecord | null>(null)
  const [selectedMeet, setSelectedMeet] = useState<MeetRecord | null>(null)
  const [events, setEvents] = useState<MeetEventRecord[]>([])
  const [selectedEvents, setSelectedEvents] = useState<Set<string>>(new Set())
  const [loadingEvents, setLoadingEvents] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const eligibleMeets = meets.filter((m) => ENTRY_STATUSES.has(m.status))

  const handleSelectMeet = async (meet: MeetRecord) => {
    setSelectedMeet(meet)
    setSelectedEvents(new Set())
    setSaveError(null)
    setLoadingEvents(true)
    setEvents(await fetchMeetEvents(api, orgId, meet.id).catch(() => [] as MeetEventRecord[]))
    setLoadingEvents(false)
    setStep(3)
  }

  const toggleEvent = (evId: string) => {
    setSaveError(null)
    setSelectedEvents((prev) => {
      const next = new Set(prev)
      if (next.has(evId)) next.delete(evId)
      else next.add(evId)
      return next
    })
  }

  const handleSave = async () => {
    if (!selectedSwimmer || !selectedMeet || selectedEvents.size === 0) return
    setSaving(true)
    setSaveError(null)
    try {
      await saveMeetEntriesBatch(
        api,
        orgId,
        selectedMeet.id,
        selectedSwimmer.id,
        [...selectedEvents],
      )
      setSaved(true)
      setTimeout(() => {
        onSaved()
        onClose()
      }, 1000)
    } catch (error) {
      setSaved(false)
      setSaveError(formatMeetEntriesSaveError(error, selectedEvents.size))
    } finally {
      setSaving(false)
    }
  }

  // Filter events to what the swimmer can enter (gender + age at the meet), grouped by session
  const eligibleSessionGroups = useMemo(() => {
    const map = new Map<string, MeetEventRecord[]>()
    for (const ev of events) {
      if (selectedSwimmer && !isSwimmerEligibleForEvent(selectedSwimmer, ev, selectedMeet?.start_date)) continue
      const sid = ev.session_id ?? 'main'
      const bucket = map.get(sid) ?? []
      bucket.push(ev)
      map.set(sid, bucket)
    }
    return [...map.entries()].map(([id, evs]) => ({ id, events: evs }))
  }, [events, selectedSwimmer, selectedMeet])

  return (
    <>
      <div
        style={{ position: 'fixed', inset: 0, background: 'var(--scrim)', zIndex: 200, backdropFilter: 'blur(2px)' }}
        onClick={onClose}
      />
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label="Add entry for swimmer"
        tabIndex={-1}
        style={{
          outline: 'none',
          position: 'fixed',
          top: 0, right: 0, bottom: 0,
          width: '540px',
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
        <div className="u-p-20px-24px" style={{ borderBottom: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexShrink: 0 }}>
          <div>
            <div className="u-fs-fs-lg u-ff-font-display u-fw-display-weight u-lh-1_2" style={{ color: 'var(--text)' }}>
              Add entry for swimmer
            </div>
            <div className="u-fs-fs-sm u-mt-2px" style={{ color: 'var(--text-muted)' }}>
              Step {step} of 3 — {step === 1 ? 'Select swimmer' : step === 2 ? 'Select meet' : 'Choose events'}
            </div>
          </div>
          <button type="button" onClick={onClose} className="u-p-4px" style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', borderRadius: '6px', display: 'flex' }}>
            <Icon name="x" size={18} />
          </button>
        </div>

        {/* Step progress */}
        <div className="u-p-12px-24px u-gap-8px" style={{ borderBottom: '1px solid var(--border)', display: 'flex', flexShrink: 0 }}>
          {[
            { n: 1, label: selectedSwimmer ? `${selectedSwimmer.first_name} ${selectedSwimmer.last_name}` : 'Swimmer' },
            { n: 2, label: selectedMeet ? selectedMeet.name : 'Meet' },
            { n: 3, label: `Events${selectedEvents.size > 0 ? ` (${selectedEvents.size})` : ''}` },
          ].map(({ n, label }) => (
            <button type="button"
              key={n}
              onClick={() => { if (n < step || (n === 2 && selectedSwimmer) || (n === 3 && selectedMeet)) setStep(n as 1 | 2 | 3) }}
              className="u-gap-6px u-p-4px-12px u-fs-fs-sm u-ff-font-body" style={{ display: 'flex', alignItems: 'center', borderRadius: '6px', border: 'none', background: step === n ? 'var(--accent-subtle)' : 'transparent', color: step === n ? 'var(--accent)' : step > n ? 'var(--success)' : 'var(--text-muted)', fontWeight: step === n ? 600 : 400, cursor: 'pointer' }}
            >
              <span className="u-fs-fs-2xs u-ff-font-body u-fw-700" style={{ width: '18px', height: '18px', borderRadius: '50%', background: step > n ? 'var(--success)' : step === n ? 'var(--accent)' : 'var(--bg-elevated)', color: step >= n ? '#fff' : 'var(--text-muted)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                {step > n ? '✓' : n}
              </span>
              {label}
            </button>
          ))}
        </div>

        {/* Body */}
        <div className="u-p-20px-24px" style={{ flex: 1, overflowY: 'auto' }}>

          {/* Step 1: Swimmer */}
          {step === 1 && (
            <div>
              <p className="u-fs-fs-base u-mb-16px" style={{ color: 'var(--text-muted)' }}>Select a swimmer to add entries for:</p>
              <div className="u-gap-6px" style={{ display: 'flex', flexDirection: 'column' }}>
                {swimmers.map((sw) => (
                  <button type="button"
                    key={sw.id}
                    onClick={() => { setSelectedSwimmer(sw); setStep(2) }}
                    className="u-gap-12px u-p-12px-16px u-ff-font-body" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-surface)', cursor: 'pointer', textAlign: 'left', transition: 'border-color 0.14s, background 0.14s' }}
                    onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent)'; e.currentTarget.style.background = 'var(--accent-subtle)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.background = 'var(--bg-surface)' }}
                  >
                    <div className="u-fs-fs-xs u-fw-700 u-ff-font-body" style={{ width: '32px', height: '32px', borderRadius: '50%', background: 'linear-gradient(135deg, var(--accent) 0%, #b84a18 100%)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', flexShrink: 0 }}>
                      {(sw.first_name[0] ?? '') + (sw.last_name[0] ?? '')}
                    </div>
                    <div>
                      <div className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{sw.first_name} {sw.last_name}</div>
                      <div className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
                        {sw.gender ?? '—'} · {sw.registration_status}
                      </div>
                    </div>
                    <Icon name="chevron-right" size={14} color="var(--text-muted)" style={{ marginLeft: 'auto', flexShrink: 0 } as CSSProperties} />
                  </button>
                ))}
                {swimmers.length === 0 && (
                  <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>No swimmers found.</p>
                )}
              </div>
            </div>
          )}

          {/* Step 2: Meet */}
          {step === 2 && (
            <div>
              <p className="u-fs-fs-base u-mb-16px" style={{ color: 'var(--text-muted)' }}>Select a meet to add entries to:</p>
              <div className="u-gap-6px" style={{ display: 'flex', flexDirection: 'column' }}>
                {eligibleMeets.map((m) => (
                  <button type="button"
                    key={m.id}
                    onClick={() => handleSelectMeet(m)}
                    className="u-gap-12px u-p-12px-16px u-ff-font-body" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-surface)', cursor: 'pointer', textAlign: 'left', transition: 'border-color 0.14s, background 0.14s' }}
                    onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent)'; e.currentTarget.style.background = 'var(--accent-subtle)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.background = 'var(--bg-surface)' }}
                  >
                    <Icon name="calendar" size={16} color="var(--text-muted)" style={{ flexShrink: 0 } as CSSProperties} />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{m.name}</div>
                      <div className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
                        {m.start_date ? formatDateOnly(m.start_date) : '—'} · {m.status.replace(/_/g, ' ')}
                      </div>
                    </div>
                    <Icon name="chevron-right" size={14} color="var(--text-muted)" style={{ flexShrink: 0 } as CSSProperties} />
                  </button>
                ))}
                {eligibleMeets.length === 0 && (
                  <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>No meets currently accepting entries.</p>
                )}
              </div>
            </div>
          )}

          {/* Step 3: Events */}
          {step === 3 && (
            <div>
              {loadingEvents ? (
                <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
                  {[1,2,3,4].map((i) => (
                    <div key={i} style={{ height: '44px', background: 'var(--bg-elevated)', borderRadius: '8px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: `${i * 0.1}s` }} />
                  ))}
                </div>
              ) : (
                <>
                  <div className="u-mb-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <p className="page-subtitle">
                      Select events for <strong style={{ color: 'var(--text)' }}>{selectedSwimmer?.first_name} {selectedSwimmer?.last_name}</strong>:
                    </p>
                    {selectedEvents.size > 0 && (
                      <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--accent-text, var(--accent))' }}>
                        {selectedEvents.size} selected
                      </span>
                    )}
                  </div>

                  {eligibleSessionGroups.length === 0 ? (
                    <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>No eligible events found for this swimmer.</p>
                  ) : (
                    <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
                      {eligibleSessionGroups.map((group) => (
                        <div key={group.id}>
                          <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>
                            {group.id === 'main' ? 'Main session' : 'Unscheduled'}
                          </div>
                          <div className="u-gap-4px" style={{ display: 'flex', flexDirection: 'column' }}>
                            {group.events.map((ev) => {
                              const isSelected = selectedEvents.has(ev.id)
                              return (
                                <div
                                  key={ev.id}
                                  onClick={() => toggleEvent(ev.id)}
                                  className="u-gap-12px u-p-12px-12px" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', border: isSelected ? '1.5px solid var(--accent)' : '1px solid var(--border)', background: isSelected ? 'var(--accent-subtle)' : 'var(--bg-surface)', cursor: 'pointer', transition: 'all 120ms ease' }}
                                >
                                  <div style={{ width: '16px', height: '16px', borderRadius: '4px', border: isSelected ? '2px solid var(--accent)' : '1.5px solid var(--border)', background: isSelected ? 'var(--accent)' : 'transparent', flexShrink: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                                    {isSelected && <svg viewBox="0 0 10 8" fill="none" width="9" height="9"><path d="M1 4l3 3 5-6" stroke="#fff" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></svg>}
                                  </div>
                                  <span className="u-fs-fs-base" style={{ color: isSelected ? 'var(--text)' : 'var(--text-subtle)', flex: 1, fontWeight: isSelected ? 600 : 400 }}>
                                    {formatEventLabel(ev)}
                                  </span>
                                  {ev.event_number && (
                                    <span className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>#{ev.event_number}</span>
                                  )}
                                </div>
                              )
                            })}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                  {saveError && (
                    <div
                      className="u-mt-16px u-p-12px-12px u-fs-fs-sm" style={{ borderRadius: '8px', border: '1px solid var(--error-border)', background: 'var(--error-bg)', color: 'var(--error)', lineHeight: 1.45 }}
                    >
                      {saveError}
                    </div>
                  )}
                </>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="u-p-16px-24px u-gap-8px" style={{ borderTop: '1px solid var(--border)', display: 'flex', flexShrink: 0, background: 'var(--bg)' }}>
          <button type="button" onClick={onClose} className="btn btn-secondary" style={{ flex: 1 }}>
            Cancel
          </button>
          {step === 3 && (
            <button type="button"
              onClick={handleSave}
              disabled={selectedEvents.size === 0 || saving || saved}
              className={`btn ${saved ? 'btn-success' : 'btn-primary'}`} style={{ flex: 2 }}
            >
              {saved ? (
                <><Icon name="check-circle" size={14} /> Saved!</>
              ) : saving ? (
                <><span className="spinner spinner-sm" aria-hidden /> Saving…</>
              ) : (
                <>Save {selectedEvents.size} {selectedEvents.size === 1 ? 'Entry' : 'Entries'}</>
              )}
            </button>
          )}
        </div>
      </div>

    </>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function EntriesPage() {
  return (
    <Suspense fallback={null}>
      <EntriesPageContent />
    </Suspense>
  )
}

function EntriesPageContent() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const searchParams = useSearchParams()
  const lockedMeetId = searchParams.get('meet')

  const [rows, setRows] = useState<EntryRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [meetFilter, setMeetFilter] = useState(lockedMeetId ?? 'all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [search, setSearch] = useState('')
  const [showAddPanel, setShowAddPanel] = useState(false)

  // Full data for add-entry panel
  const [allSwimmers, setAllSwimmers] = useState<SwimmerRecord[]>([])
  const [allMeets, setAllMeets] = useState<MeetRecord[]>([])

  const loadData = useCallback(async (cancelled: { v: boolean }) => {
    if (!session?.user.orgId) { setLoading(false); return }
    try {
      setLoading(true)
      setError(null)
      const orgId = session.user.orgId

      const [meets, swimmers] = await Promise.all([
        fetchMeets(api, orgId),
        fetchSwimmers(api, orgId),
      ])

      setAllSwimmers(swimmers)
      setAllMeets(meets)

      const swimmerMap = new Map<string, SwimmerRecord>(swimmers.map((s) => [s.id, s]))
      const relevantMeets = meets.filter((m) => ENTRY_STATUSES.has(m.status))

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
            event: formatEventLabel(meetEvent),
            seedTime: formatSeedTime(e.seed_time_ms),
            status: e.status as EntryStatus,
            override: !!e.override_reason,
            overrideNote: e.override_reason ?? null,
          })
        }
      }

      if (!cancelled.v) setRows(combined)
    } catch (err) {
      if (!cancelled.v) setError(uiErrorMessage(err, 'Entries couldn’t be loaded. Check your connection and try again.'))
    } finally {
      if (!cancelled.v) setLoading(false)
    }
  }, [session?.user.orgId, api])

  useEffect(() => {
    const cancelled = { v: false }
    loadData(cancelled)
    return () => { cancelled.v = true }
  }, [loadData])

  const handleApprove = useCallback(async (entryId: string, meetId: string): Promise<boolean> => {
    if (!session?.user.orgId) return false
    try {
      await approveEntry(api, session.user.orgId, meetId, entryId)
      setRows((prev) => prev.map((e) => e.id === entryId ? { ...e, status: 'approved' } : e))
      return true
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t approve entry. Try again.'))
      return false
    }
  }, [api, session?.user.orgId])

  const handleApproveAll = useCallback(async (pending: { id: string; meetId: string }[]) => {
    setError(null)
    const results = await Promise.all(pending.map((e) => handleApprove(e.id, e.meetId)))
    const failed = results.filter((ok) => !ok).length
    if (failed > 0) setError(`${failed} of ${pending.length} entries could not be approved. Refresh to see their current status.`)
  }, [handleApprove])

  const confirm = useConfirm()
  const confirmReason = useConfirmReason()
  const [overrideDraft, setOverrideDraft] = useState<{ entryId: string; meetId: string; events: MeetEventRecord[]; eventId: string; reason: string } | null>(null)
  // The API has no separate "rejected" state: rejecting a submitted entry
  // scratches it (kept on record, excluded from reports and exports).
  const handleReject = useCallback(async (entryId: string, meetId: string) => {
    if (!session?.user.orgId) return
    const ok = await confirm({ title: 'Reject this entry?', message: 'It will be scratched and excluded from reports and exports. It stays on record.', confirmLabel: 'Reject entry', destructive: true, count: 1 })
    if (!ok) return
    void scratchEntry(api, session.user.orgId, meetId, entryId, 'Rejected during coach review')
      .then(() => setRows((prev) => prev.map((e) => e.id === entryId ? { ...e, status: 'scratched' } : e)))
      .catch((err) => setError(uiErrorMessage(err, 'Couldn’t reject entry. Try again.')))
  }, [api, session?.user.orgId, confirm])

  const handleScratch = useCallback(async (entryId: string, meetId: string) => {
    if (!session?.user.orgId) return
    const reason = await confirmReason({ title: 'Scratch this entry?', message: 'It stays on record but is removed from reports and exports.', confirmLabel: 'Scratch entry', destructive: true, count: 1, reasonLabel: 'Reason', reasonRequired: true })
    if (reason === null) return
    void scratchEntry(api, session.user.orgId, meetId, entryId, reason)
      .then(() => setRows((prev) => prev.map((e) => e.id === entryId ? { ...e, status: 'scratched' } : e)))
      .catch((err) => setError(uiErrorMessage(err, 'Couldn’t scratch entry. Try again.')))
  }, [api, session?.user.orgId, confirmReason])

  const handleOverride = useCallback((entryId: string, meetId: string) => {
    const orgId = session?.user.orgId
    if (!orgId) return
    void fetchMeetEvents(api, orgId, meetId)
      .then((meetEvents) => {
        if (meetEvents.length === 0) {
          setError('This meet has no events to override into.')
          return
        }
        setOverrideDraft({ entryId, meetId, events: meetEvents, eventId: meetEvents[0].id, reason: '' })
      })
      .catch((err) => setError(uiErrorMessage(err, 'Couldn’t override entry. Try again.')))
  }, [api, session?.user.orgId])

  const submitOverride = useCallback(() => {
    const orgId = session?.user.orgId
    const d = overrideDraft
    if (!orgId || !d || !d.reason.trim()) return
    const target = d.events.find((ev) => ev.id === d.eventId)
    if (!target) return
    const reason = d.reason.trim()
    setOverrideDraft(null)
    void overrideEntry(api, orgId, d.meetId, d.entryId, { new_meet_event_id: target.id, reason })
      .then((updated) => setRows((prev) => prev.map((e) => e.id === d.entryId
        ? { ...e, event: formatEventLabel(target), override: true, overrideNote: updated.override_reason ?? reason }
        : e)))
      .catch((err) => setError(uiErrorMessage(err, 'Couldn’t override entry. Try again.')))
  }, [api, session?.user.orgId, overrideDraft])

  // Stats
  const total     = rows.length
  const submitted = rows.filter((e) => e.status === 'submitted').length
  const approved  = rows.filter((e) => e.status === 'approved').length
  const draft     = rows.filter((e) => e.status === 'draft').length
  const overrides = rows.filter((e) => e.override).length

  const meetOptions = useMemo(() => {
    const seen = new Map<string, string>()
    for (const r of rows) seen.set(r.meetId, r.meetName)
    return [{ value: 'all', label: 'All meets' }, ...[...seen.entries()].map(([v, l]) => ({ value: v, label: l }))]
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

  // Keyboard triage: / filter, j/k move, a approve, r reject (still asks to confirm).
  const searchRef = useRef<HTMLInputElement>(null)
  const [focusedId, setFocusedId] = useState<string | null>(null)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return
      const t = e.target as HTMLElement | null
      if (t && (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName))) return
      if (document.querySelector('[role="dialog"]')) return
      const idx = filtered.findIndex((r) => r.id === focusedId)
      if (e.key === '/') { e.preventDefault(); searchRef.current?.focus() }
      else if (e.key === 'j') { setFocusedId(filtered[Math.min(idx + 1, filtered.length - 1)]?.id ?? null) }
      else if (e.key === 'k') { setFocusedId(filtered[Math.max(idx - 1, 0)]?.id ?? null) }
      else if ((e.key === 'a' || e.key === 'r') && idx >= 0) {
        const row = filtered[idx]
        if (row.status !== 'submitted') return
        if (e.key === 'a') void handleApprove(row.id, row.meetId)
        else void handleReject(row.id, row.meetId)
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [filtered, focusedId, handleApprove, handleReject])

  const columns = useMemo<Column<EntryRow>[]>(() => [
    {
      key: 'swimmer',
      label: 'Swimmer',
      sortable: true,
      render: (r) => (
        <Link href={`/members/swimmers/${r.swimmerId}`} className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)', textDecoration: 'none' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
          {r.swimmer}
        </Link>
      ),
    },
    {
      key: 'event',
      label: 'Event',
      render: (r) => (
        <span className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{r.event}</span>
      ),
    },
    {
      key: 'seedTime',
      label: 'Seed time',
      align: 'right',
      render: (r) => (
        <span className="u-ff-font-body u-fs-fs-base u-fw-700" style={{ color: r.seedTime === 'NT' ? 'var(--text-muted)' : 'var(--accent)' }}>
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
          <Link href={`/meets/${r.meetId}`} className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)', textDecoration: 'none' }}
            onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
            onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
            {r.meetName}
          </Link>
          {r.meetDate && (
            <div className="u-fs-fs-xs u-ff-font-body u-mt-1px" style={{ color: 'var(--text-muted)' }}>
              {formatDateOnly(r.meetDate)}
            </div>
          )}
        </div>
      ),
    },
    {
      key: 'status',
      label: 'Status',
      render: (r) => {
        const c = STATUS_CFG[r.status] ?? STATUS_CFG.draft
        return (
          <div className="u-gap-4px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
            <span className="badge" style={{ background: c.bg, color: c.color }}>
              {c.label}
            </span>
            {r.override && (
              <span
                title={r.overrideNote ? `Seed time changed by hand: ${r.overrideNote}` : 'Seed time changed by hand'}
                className="u-fs-fs-2xs u-fw-700 u-p-2px-6px u-ff-font-body" style={{ borderRadius: '8px', background: 'var(--accent-subtle)', color: 'var(--accent-text, var(--accent))' }}
              >
                CHANGED
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
        // Entries can be scratched until they are exported/finalized.
        const scratchable = r.status === 'draft' || r.status === 'submitted' || r.status === 'approved'
        return (
          <div className="u-gap-6px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', flexWrap: 'wrap' }}>
            {r.status === 'submitted' && (
              <>
                <button type="button" className="btn btn-sm u-gap-4px" style={{ background: 'var(--success-bg)', color: 'var(--success)', border: '1px solid var(--success-border)', display: 'flex', alignItems: 'center' }} onClick={() => { void handleApprove(r.id, r.meetId) }}>
                  <Icon name="check-circle" size={11} /> Approve
                </button>
                <button type="button" className="btn btn-sm btn-secondary u-gap-4px" style={{ color: 'var(--error)', display: 'flex', alignItems: 'center' }} onClick={() => handleReject(r.id, r.meetId)}>
                  <Icon name="x" size={11} /> Reject
                </button>
              </>
            )}
            {scratchable && (
              <button type="button"
                className="btn btn-sm btn-secondary u-gap-4px"
                style={{ display: 'flex', alignItems: 'center' }}
                onClick={() => handleOverride(r.id, r.meetId)}
                title="Change this entry's event (coach override, reason required)"
              >
                <Icon name="edit" size={11} /> Override
              </button>
            )}
            {scratchable && (
              <button type="button"
                className="btn btn-sm btn-secondary u-gap-4px"
                style={{ color: 'var(--error)', display: 'flex', alignItems: 'center' }}
                onClick={() => handleScratch(r.id, r.meetId)}
                title="Scratch this entry"
              >
                <Icon name="trash-2" size={11} /> Scratch
              </button>
            )}
            <Link href={`/meets/${r.meetId}`} className="u-fs-fs-sm u-gap-4px" style={{ color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center' }}
              onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
              onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}>
              Meet <Icon name="chevron-right" size={12} />
            </Link>
          </div>
        )
      },
    },
  ], [handleApprove, handleReject, handleScratch, handleOverride])

  if (error) {
    return (
      <div className="page">
        <div className="card u-p-20px" style={{ borderColor: 'var(--error-border)', background: 'var(--error-bg)' }}>
          <div className="row row-between" role="alert">
            <p className="u-fs-fs-base" style={{ color: 'var(--error)' }}>{error}</p>
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => void loadData({ v: false })}>Retry</button>
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
            Entries
          </h1>
        </div>
        <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center' }}>
          {pendingVisible.length > 0 && (
            <button type="button"
              className="btn btn-sm btn-secondary u-gap-6px"
              style={{ display: 'flex', alignItems: 'center' }}
              onClick={() => { void handleApproveAll(pendingVisible) }}
            >
              <Icon name="check-circle" size={13} /> Approve All ({pendingVisible.length})
            </button>
          )}
          <button type="button"
            className="btn btn-primary u-gap-6px"
            style={{ display: 'flex', alignItems: 'center' }}
            onClick={() => setShowAddPanel(true)}
          >
            <Icon name="plus" size={13} /> Add entry
          </button>
        </div>
      </div>

      {/* Stats strip */}
      <div className="cols-4 u-gap-12px u-mb-24px" >
        {[
          { label: 'Total',        value: loading ? '—' : total,     color: 'var(--text)' },
          { label: 'Needs review', value: loading ? '—' : submitted, color: (submitted > 0 && !loading) ? 'var(--warning)' : 'var(--text-muted)' },
          { label: 'Approved',     value: loading ? '—' : approved,  color: 'var(--success)' },
          { label: 'Overrides',    value: loading ? '—' : overrides, color: (overrides > 0 && !loading) ? 'var(--accent)' : 'var(--text-muted)' },
        ].map((s) => (
          <div key={s.label} className="card u-p-16px-16px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{s.label}</div>
            <div className="u-fs-fs-2xl u-ff-font-body u-fw-700" style={{ color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Pending banner */}
      {!loading && submitted > 0 && (
        <div className="card u-p-16px-16px u-mb-20px u-gap-12px" style={{ borderColor: 'var(--warning-border)', background: 'var(--warning-bg)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'center' }}>
            <Icon name="alert-circle" size={16} color="var(--warning)" />
            <span className="u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>
              <strong style={{ color: 'var(--warning)' }}>{submitted} entr{submitted === 1 ? 'y' : 'ies'}</strong> {submitted === 1 ? 'is' : 'are'} pending coach approval across all meets
            </span>
          </div>
          {draft > 0 && (
            <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
              {draft} draft{draft !== 1 ? 's' : ''} not yet submitted
            </span>
          )}
        </div>
      )}

      {/* Filters */}
      <div className="toolbar">
        <div className="toolbar-search">
          <Icon name="search" size={14} color="var(--text-muted)" style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' } as CSSProperties} />
          <input aria-label="Search swimmer, event, or meet" ref={searchRef} type="text" placeholder="Search swimmer, event, or meet…" value={search} onChange={(e) => setSearch(e.target.value)} className="input-search input u-pl-32px" style={{ width: '100%' }} />
        </div>
        {lockedMeetId ? (
          <div className="u-gap-8px u-p-0-12px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', borderRadius: 'var(--r-md)', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)' }}>
            <Icon name="calendar" size={13} color="var(--text-muted)" />
            {meetOptions.find((o) => o.value === lockedMeetId)?.label ?? 'This meet'}
            <Link href="/entries" className="u-fs-fs-xs" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none' }}>Clear</Link>
          </div>
        ) : (
          <select value={meetFilter} onChange={(e) => setMeetFilter(e.target.value)} className="input" style={{ minWidth: '180px' }}>
            {meetOptions.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        )}
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="input" style={{ minWidth: '150px' }}>
          {STATUS_FILTER_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </div>

      {!loading && (
        <div className="u-fs-fs-sm u-ff-font-body u-mb-12px" style={{ color: 'var(--text-muted)' }}>
          {filtered.length} of {total} entries.{' '}
          Keyboard shortcuts: <span className="kbd-hint">/</span> filter, <span className="kbd-hint">j</span> <span className="kbd-hint">k</span> move, <span className="kbd-hint">a</span> approve, <span className="kbd-hint">r</span> reject.
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

      {!loading && (
        filtered.length === 0
          ? <EmptyState title="No entries found" description={total === 0 ? "No entries exist yet. Use 'Add entry' to add one, or entries are created when commitments are locked." : "Try adjusting your filters."} />
          : <Table columns={columns} data={filtered} keyField="id" highlightKey={focusedId ?? undefined} pageSize={20} emptyTitle="No entries" emptyDescription="No entries match the current filters." />
      )}

      {/* Add entry panel */}
      {showAddPanel && session?.user.orgId && (
        <AddEntryPanel
          swimmers={allSwimmers}
          meets={allMeets}
          api={api}
          orgId={session.user.orgId}
          onClose={() => setShowAddPanel(false)}
          onSaved={() => {
            const cancelled = { v: false }
            loadData(cancelled)
          }}
        />
      )}
      <Modal
        open={!!overrideDraft}
        onClose={() => setOverrideDraft(null)}
        title="Override entry event"
        width={440}
        footer={
          <>
            <button type="button" className="btn btn-secondary" onClick={() => setOverrideDraft(null)}>Cancel</button>
            <button type="button" className="btn btn-primary" disabled={!overrideDraft?.reason.trim()} onClick={submitOverride}>Override entry</button>
          </>
        }
      >
        {overrideDraft && (
          <div className="stack">
            <div>
              <label htmlFor="override-event" className="form-label">Change to event</label>
              <select id="override-event" className="form-input" style={{ width: '100%' }} value={overrideDraft.eventId} onChange={(e) => setOverrideDraft({ ...overrideDraft, eventId: e.target.value })}>
                {overrideDraft.events.map((ev) => <option key={ev.id} value={ev.id}>{formatEventLabel(ev)}</option>)}
              </select>
            </div>
            <div>
              <label htmlFor="override-reason" className="form-label">Reason</label>
              <textarea id="override-reason" className="form-input" rows={3} style={{ width: '100%' }} data-autofocus="" value={overrideDraft.reason} onChange={(e) => setOverrideDraft({ ...overrideDraft, reason: e.target.value })} />
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}
