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
  type MeetRecord,
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
  free: 'Freestyle', back: 'Backstroke', breast: 'Breaststroke', fly: 'Butterfly', im: 'IM', medley: 'Medley',
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
  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [selectedSwimmer, setSelectedSwimmer] = useState<SwimmerRecord | null>(null)
  const [selectedMeet, setSelectedMeet] = useState<MeetRecord | null>(null)
  const [events, setEvents] = useState<MeetEventRecord[]>([])
  const [selectedEvents, setSelectedEvents] = useState<Set<string>>(new Set())
  const [loadingEvents, setLoadingEvents] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  const eligibleMeets = meets.filter((m) => ENTRY_STATUSES.has(m.status))

  const loadEvents = async (meet: MeetRecord) => {
    setLoadingEvents(true)
    const evs = await fetchMeetEvents(api, orgId, meet.id).catch(() => [] as MeetEventRecord[])
    setEvents(evs)
    setLoadingEvents(false)
  }

  const handleSelectMeet = async (meet: MeetRecord) => {
    setSelectedMeet(meet)
    setSelectedEvents(new Set())
    await loadEvents(meet)
    setStep(3)
  }

  const toggleEvent = (evId: string) => {
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
    try {
      // Create entries for each selected event
      await Promise.all(
        [...selectedEvents].map((evId) =>
          api.post(
            `/api/v1/orgs/${orgId}/meets/${selectedMeet.id}/entries`,
            { swimmer_id: selectedSwimmer.id, meet_event_id: evId, status: 'draft' },
            { 'X-Org-Id': orgId }
          ).catch((e: unknown) => console.warn('Entry create failed:', e))
        )
      )
      setSaved(true)
      setTimeout(() => {
        onSaved()
        onClose()
      }, 1000)
    } finally {
      setSaving(false)
    }
  }

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

  // Filter events to swimmer's gender
  const eligibleEvents = useMemo(() => {
    if (!selectedSwimmer) return events
    return events.filter((ev) => !ev.gender || ev.gender.toLowerCase() === 'mixed' || ev.gender === selectedSwimmer.gender)
  }, [events, selectedSwimmer])

  const eligibleSessionGroups = sessionGroups
    .map((g) => ({ ...g, events: g.events.filter((e) => eligibleEvents.includes(e)) }))
    .filter((g) => g.events.length > 0)

  return (
    <>
      <div
        style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.45)', zIndex: 200, backdropFilter: 'blur(2px)' }}
        onClick={onClose}
      />
      <div
        style={{
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
        <div style={{ padding: '18px 22px', borderBottom: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexShrink: 0 }}>
          <div>
            <div style={{ fontSize: '16px', fontFamily: 'var(--font-display)', color: 'var(--text)', lineHeight: 1.2 }}>
              Add Entry for Swimmer
            </div>
            <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '2px' }}>
              Step {step} of 3 — {step === 1 ? 'Select swimmer' : step === 2 ? 'Select meet' : 'Choose events'}
            </div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: '4px', borderRadius: '6px', display: 'flex' }}>
            <Icon name="x" size={18} />
          </button>
        </div>

        {/* Step progress */}
        <div style={{ padding: '12px 22px', borderBottom: '1px solid var(--border)', display: 'flex', gap: '8px', flexShrink: 0 }}>
          {[
            { n: 1, label: selectedSwimmer ? `${selectedSwimmer.first_name} ${selectedSwimmer.last_name}` : 'Swimmer' },
            { n: 2, label: selectedMeet ? selectedMeet.name : 'Meet' },
            { n: 3, label: `Events${selectedEvents.size > 0 ? ` (${selectedEvents.size})` : ''}` },
          ].map(({ n, label }) => (
            <button
              key={n}
              onClick={() => { if (n < step || (n === 2 && selectedSwimmer) || (n === 3 && selectedMeet)) setStep(n as 1 | 2 | 3) }}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                padding: '5px 10px',
                borderRadius: '6px',
                border: 'none',
                background: step === n ? 'var(--accent-subtle)' : 'transparent',
                color: step === n ? 'var(--accent)' : step > n ? 'var(--success)' : 'var(--text-muted)',
                fontSize: '12px',
                fontWeight: step === n ? 600 : 400,
                cursor: 'pointer',
                fontFamily: 'var(--font-body)',
              }}
            >
              <span style={{ width: '18px', height: '18px', borderRadius: '50%', background: step > n ? 'var(--success)' : step === n ? 'var(--accent)' : 'var(--bg-elevated)', color: step >= n ? '#fff' : 'var(--text-muted)', fontSize: '10px', fontFamily: 'var(--font-mono)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: 700, flexShrink: 0 }}>
                {step > n ? '✓' : n}
              </span>
              {label}
            </button>
          ))}
        </div>

        {/* Body */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '20px 22px' }}>

          {/* Step 1: Swimmer */}
          {step === 1 && (
            <div>
              <p style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '14px' }}>Select a swimmer to add entries for:</p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                {swimmers.map((sw) => (
                  <button
                    key={sw.id}
                    onClick={() => { setSelectedSwimmer(sw); setStep(2) }}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: '10px',
                      padding: '11px 14px',
                      borderRadius: '9px',
                      border: '1px solid var(--border)',
                      background: 'var(--bg-surface)',
                      cursor: 'pointer',
                      textAlign: 'left',
                      fontFamily: 'var(--font-body)',
                      transition: 'border-color 0.14s, background 0.14s',
                    }}
                    onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent)'; e.currentTarget.style.background = 'var(--accent-subtle)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.background = 'var(--bg-surface)' }}
                  >
                    <div style={{ width: '32px', height: '32px', borderRadius: '50%', background: 'linear-gradient(135deg, var(--accent) 0%, #b84a18 100%)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: '11px', fontWeight: 700, fontFamily: 'var(--font-mono)', flexShrink: 0 }}>
                      {(sw.first_name[0] ?? '') + (sw.last_name[0] ?? '')}
                    </div>
                    <div>
                      <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{sw.first_name} {sw.last_name}</div>
                      <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                        {sw.gender ?? '—'} · {sw.registration_status}
                      </div>
                    </div>
                    <Icon name="chevron-right" size={14} color="var(--text-muted)" style={{ marginLeft: 'auto', flexShrink: 0 } as CSSProperties} />
                  </button>
                ))}
                {swimmers.length === 0 && (
                  <p style={{ fontSize: '13px', color: 'var(--text-muted)', fontStyle: 'italic' }}>No swimmers found.</p>
                )}
              </div>
            </div>
          )}

          {/* Step 2: Meet */}
          {step === 2 && (
            <div>
              <p style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '14px' }}>Select a meet to add entries to:</p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                {eligibleMeets.map((m) => (
                  <button
                    key={m.id}
                    onClick={() => handleSelectMeet(m)}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: '10px',
                      padding: '11px 14px',
                      borderRadius: '9px',
                      border: '1px solid var(--border)',
                      background: 'var(--bg-surface)',
                      cursor: 'pointer',
                      textAlign: 'left',
                      fontFamily: 'var(--font-body)',
                      transition: 'border-color 0.14s, background 0.14s',
                    }}
                    onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent)'; e.currentTarget.style.background = 'var(--accent-subtle)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.background = 'var(--bg-surface)' }}
                  >
                    <Icon name="calendar" size={16} color="var(--text-muted)" style={{ flexShrink: 0 } as CSSProperties} />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{m.name}</div>
                      <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                        {m.start_date ? formatDate(m.start_date) : '—'} · {m.status.replace(/_/g, ' ')}
                      </div>
                    </div>
                    <Icon name="chevron-right" size={14} color="var(--text-muted)" style={{ flexShrink: 0 } as CSSProperties} />
                  </button>
                ))}
                {eligibleMeets.length === 0 && (
                  <p style={{ fontSize: '13px', color: 'var(--text-muted)', fontStyle: 'italic' }}>No meets currently accepting entries.</p>
                )}
              </div>
            </div>
          )}

          {/* Step 3: Events */}
          {step === 3 && (
            <div>
              {loadingEvents ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                  {[1,2,3,4].map((i) => (
                    <div key={i} style={{ height: '44px', background: 'var(--bg-elevated)', borderRadius: '8px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: `${i * 0.1}s` }} />
                  ))}
                </div>
              ) : (
                <>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '14px' }}>
                    <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
                      Select events for <strong style={{ color: 'var(--text)' }}>{selectedSwimmer?.first_name} {selectedSwimmer?.last_name}</strong>:
                    </p>
                    {selectedEvents.size > 0 && (
                      <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--accent)' }}>
                        {selectedEvents.size} selected
                      </span>
                    )}
                  </div>

                  {eligibleSessionGroups.length === 0 ? (
                    <p style={{ fontSize: '13px', color: 'var(--text-muted)', fontStyle: 'italic' }}>No eligible events found for this swimmer.</p>
                  ) : (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                      {eligibleSessionGroups.map((group) => (
                        <div key={group.id}>
                          <div style={{ fontSize: '10.5px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '6px' }}>
                            {group.id === 'main' ? 'Main Session' : `Session ${group.id.slice(-4)}`}
                          </div>
                          <div style={{ display: 'flex', flexDirection: 'column', gap: '5px' }}>
                            {group.events.map((ev) => {
                              const isSelected = selectedEvents.has(ev.id)
                              return (
                                <div
                                  key={ev.id}
                                  onClick={() => toggleEvent(ev.id)}
                                  style={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    gap: '10px',
                                    padding: '10px 12px',
                                    borderRadius: '8px',
                                    border: isSelected ? '1.5px solid var(--accent)' : '1px solid var(--border)',
                                    background: isSelected ? 'var(--accent-subtle)' : 'var(--bg-surface)',
                                    cursor: 'pointer',
                                    transition: 'all 120ms ease',
                                  }}
                                >
                                  <div style={{ width: '16px', height: '16px', borderRadius: '4px', border: isSelected ? '2px solid var(--accent)' : '1.5px solid var(--border)', background: isSelected ? 'var(--accent)' : 'transparent', flexShrink: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                                    {isSelected && <svg viewBox="0 0 10 8" fill="none" width="9" height="9"><path d="M1 4l3 3 5-6" stroke="#fff" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></svg>}
                                  </div>
                                  <span style={{ fontSize: '13px', color: isSelected ? 'var(--text)' : 'var(--text-subtle)', flex: 1, fontWeight: isSelected ? 600 : 400 }}>
                                    {formatEvent(ev)}
                                  </span>
                                  {ev.event_number && (
                                    <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>#{ev.event_number}</span>
                                  )}
                                </div>
                              )
                            })}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div style={{ padding: '14px 22px', borderTop: '1px solid var(--border)', display: 'flex', gap: '8px', flexShrink: 0, background: 'var(--bg)' }}>
          <button onClick={onClose} style={{ flex: 1, padding: '10px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '8px', fontSize: '13px', fontWeight: 600, color: 'var(--text)', cursor: 'pointer' }}>
            Cancel
          </button>
          {step === 3 && (
            <button
              onClick={handleSave}
              disabled={selectedEvents.size === 0 || saving || saved}
              style={{ flex: 2, padding: '10px', background: saved ? 'var(--success)' : 'var(--accent)', border: 'none', borderRadius: '8px', fontSize: '13px', fontWeight: 600, color: '#fff', cursor: selectedEvents.size === 0 || saving || saved ? 'not-allowed' : 'pointer', opacity: selectedEvents.size === 0 ? 0.5 : 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px', transition: 'background 0.2s' }}
            >
              {saved ? (
                <><Icon name="check-circle" size={14} /> Saved!</>
              ) : saving ? (
                <><span style={{ width: '14px', height: '14px', border: '2px solid rgba(255,255,255,0.3)', borderTopColor: '#fff', borderRadius: '50%', animation: 'spin 0.65s linear infinite', display: 'inline-block' }} /> Saving…</>
              ) : (
                <>Save {selectedEvents.size} {selectedEvents.size === 1 ? 'Entry' : 'Entries'}</>
              )}
            </button>
          )}
        </div>
      </div>

      <style>{`
        @keyframes slideInRight {
          from { transform: translateX(100%); opacity: 0; }
          to   { transform: translateX(0);    opacity: 1; }
        }
        @keyframes spin { to { transform: rotate(360deg); } }
      `}</style>
    </>
  )
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
            event: formatEvent(meetEvent),
            seedTime: formatSeedTime(e.seed_time_cents),
            status: e.status as EntryStatus,
            override: !!e.override_reason,
            overrideNote: e.override_reason ?? null,
          })
        }
      }

      if (!cancelled.v) setRows(combined)
    } catch (err) {
      if (!cancelled.v) setError(err instanceof Error ? err.message : 'Failed to load entries')
    } finally {
      if (!cancelled.v) setLoading(false)
    }
  }, [session?.user.orgId, api])

  useEffect(() => {
    const cancelled = { v: false }
    loadData(cancelled)
    return () => { cancelled.v = true }
  }, [loadData])

  const handleApprove = useCallback((entryId: string) => {
    setRows((prev) => prev.map((e) => e.id === entryId ? { ...e, status: 'approved' } : e))
  }, [])

  const handleReject = useCallback((entryId: string) => {
    if (!window.confirm('Reject this submitted entry?')) return
    setRows((prev) => prev.map((e) => e.id === entryId ? { ...e, status: 'rejected' } : e))
  }, [])

  // Stats
  const total     = rows.length
  const submitted = rows.filter((e) => e.status === 'submitted').length
  const approved  = rows.filter((e) => e.status === 'approved').length
  const draft     = rows.filter((e) => e.status === 'draft').length
  const overrides = rows.filter((e) => e.override).length

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
        <Link href={`/members/swimmers/${r.swimmerId}`} style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}
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
          <Link href={`/meets/${r.meetId}`} style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}
            onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
            onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
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
            <Link href={`/meets/${r.meetId}`} style={{ fontSize: '12px', color: 'var(--text-muted)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '3px' }}
              onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
              onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}>
              Meet <Icon name="chevron-right" size={12} />
            </Link>
          )
        }
        return (
          <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
            <button className="btn btn-sm" style={{ background: 'var(--success-bg)', color: 'var(--success)', border: '1px solid var(--success-border)', display: 'flex', alignItems: 'center', gap: '4px' }} onClick={() => handleApprove(r.id)}>
              <Icon name="check-circle" size={11} /> Approve
            </button>
            <button className="btn btn-sm btn-secondary" style={{ color: 'var(--error)', display: 'flex', alignItems: 'center', gap: '4px' }} onClick={() => handleReject(r.id)}>
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
            Cross-meet entry approval queue — review submitted entries or add entries for a swimmer
          </p>
        </div>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
          {pendingVisible.length > 0 && (
            <button
              className="btn btn-sm btn-secondary"
              style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
              onClick={() => { for (const e of pendingVisible) handleApprove(e.id) }}
            >
              <Icon name="check-circle" size={13} /> Approve All ({pendingVisible.length})
            </button>
          )}
          <button
            className="btn btn-primary"
            style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
            onClick={() => setShowAddPanel(true)}
          >
            <Icon name="plus" size={13} /> Add Entry
          </button>
        </div>
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
        <div className="card" style={{ padding: '14px 16px', marginBottom: '20px', borderLeft: '3px solid var(--warning)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px' }}>
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
          <input type="text" placeholder="Search swimmer, event, or meet…" value={search} onChange={(e) => setSearch(e.target.value)} className="input" style={{ paddingLeft: '32px', width: '100%' }} />
        </div>
        <select value={meetFilter} onChange={(e) => setMeetFilter(e.target.value)} className="input" style={{ minWidth: '180px' }}>
          {meetOptions.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="input" style={{ minWidth: '150px' }}>
          {STATUS_FILTER_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </div>

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

      {!loading && (
        filtered.length === 0
          ? <EmptyState title="No entries found" description={total === 0 ? "No entries exist yet. Use 'Add Entry' to add one, or entries are created when commitments are locked." : "Try adjusting your filters."} />
          : <Table columns={columns} data={filtered} pageSize={20} emptyTitle="No entries" emptyDescription="No entries match the current filters." />
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
    </div>
  )
}
