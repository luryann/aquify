'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  fetchGroups, fetchPracticeAttendance, fetchPractices, setPracticeAttendance,
  type AttendanceRosterEntry, type AttendanceStatus, type GroupRecord, type PracticeRecord,
} from '@/lib/adminData'
import { reportUIError, uiErrorMessage } from '@/lib/frontend-telemetry'
import {
  dueMarks, effectiveStatus, enqueueMark, failMark, isRetryable, loadQueue, pendingForPractice, saveQueue, settleMark,
  type RollCallQueue,
} from '@/lib/roll-call-queue'
import { localToday } from '@/lib/wellness-scale'

const ROSTER_CACHE_PREFIX = 'aquify:roll-call-roster:'

const STATUS_UI: Record<AttendanceStatus, { label: string; key: string; color: string; bg: string; border: string }> = {
  present: { label: 'Present', key: 'P', color: 'var(--success)', bg: 'var(--success-bg)', border: 'var(--success-border)' },
  absent:  { label: 'Absent',  key: 'A', color: 'var(--error)',   bg: 'var(--error-bg)',   border: 'var(--error-border)' },
  excused: { label: 'Excused', key: 'E', color: 'var(--warning)', bg: 'var(--warning-bg)', border: 'var(--warning-border)' },
}

function storage(): Storage | undefined {
  try { return typeof window !== 'undefined' ? window.localStorage : undefined } catch { return undefined }
}

function readRosterCache(practiceId: string): AttendanceRosterEntry[] | null {
  try {
    const raw = storage()?.getItem(ROSTER_CACHE_PREFIX + practiceId)
    return raw ? (JSON.parse(raw) as AttendanceRosterEntry[]) : null
  } catch { return null }
}

function writeRosterCache(practiceId: string, roster: AttendanceRosterEntry[]) {
  try { storage()?.setItem(ROSTER_CACHE_PREFIX + practiceId, JSON.stringify(roster)) } catch { /* cache is best-effort */ }
}

export default function RollCallPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [date, setDate] = useState(localToday())
  const [practices, setPractices] = useState<PracticeRecord[]>([])
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [practiceId, setPracticeId] = useState('')
  const [roster, setRoster] = useState<AttendanceRosterEntry[] | null>(null)
  const [rosterFromCache, setRosterFromCache] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [queue, setQueue] = useState<RollCallQueue>({})
  const [online, setOnline] = useState(true)
  const [syncing, setSyncing] = useState(false)
  const [permanentError, setPermanentError] = useState<string | null>(null)
  const queueRef = useRef<RollCallQueue>({})
  const flushing = useRef(false)
  const practiceIdRef = useRef('')

  const updateQueue = useCallback((fn: (q: RollCallQueue) => RollCallQueue) => {
    const next = fn(queueRef.current)
    queueRef.current = next
    saveQueue(storage(), next)
    setQueue(next)
  }, [])

  useEffect(() => {
    const initial = loadQueue(storage())
    queueRef.current = initial
    setQueue(initial)
    setOnline(typeof navigator === 'undefined' ? true : navigator.onLine)
  }, [])

  const flush = useCallback(async () => {
    if (flushing.current || !orgId) return
    flushing.current = true
    setSyncing(true)
    try {
      for (const mark of dueMarks(queueRef.current, Date.now())) {
        try {
          await setPracticeAttendance(api, orgId, mark.practiceId, mark.swimmerId, mark.status)
          updateQueue((q) => settleMark(q, mark))
          // The server accepted the mark: fold it into the roster snapshot so the
          // row does not fall back to its pre-tap status once the queue entry clears.
          const applyMark = (r: AttendanceRosterEntry[]) => r.map((e) => (e.swimmer_id === mark.swimmerId ? { ...e, status: mark.status } : e))
          if (practiceIdRef.current === mark.practiceId) setRoster((r) => (r ? applyMark(r) : r))
          const cachedRoster = readRosterCache(mark.practiceId)
          if (cachedRoster) writeRosterCache(mark.practiceId, applyMark(cachedRoster))
          setOnline(true)
        } catch (err) {
          const status = err instanceof ApiError ? err.status : undefined
          if (isRetryable(status)) {
            updateQueue((q) => failMark(q, mark, uiErrorMessage(err, 'Network error'), Date.now()))
            if (status === undefined) setOnline(false)
          } else {
            // Permanent rejection (e.g. practice deleted): drop it and say so.
            updateQueue((q) => settleMark(q, mark))
            setPermanentError(reportUIError('roll-call-mark', err, 'A mark was rejected by the server.', { practiceId: mark.practiceId }))
          }
        }
      }
    } finally {
      flushing.current = false
      setSyncing(false)
    }
  }, [api, orgId, updateQueue])

  useEffect(() => {
    const tick = window.setInterval(() => { void flush() }, 3000)
    const onOnline = () => { setOnline(true); void flush() }
    const onOffline = () => setOnline(false)
    window.addEventListener('online', onOnline)
    window.addEventListener('offline', onOffline)
    return () => { window.clearInterval(tick); window.removeEventListener('online', onOnline); window.removeEventListener('offline', onOffline) }
  }, [flush])

  useEffect(() => {
    if (!orgId) return
    setLoadError(null)
    Promise.all([fetchPractices(api, orgId), fetchGroups(api, orgId)])
      .then(([p, g]) => { setPractices(p); setGroups(g) })
      .catch((err) => setLoadError(reportUIError('roll-call-practices', err, 'Practices could not be loaded. Marks you already queued will still sync.')))
  }, [api, orgId, reloadKey])

  useEffect(() => { practiceIdRef.current = practiceId }, [practiceId])

  const dayPractices = useMemo(() => practices.filter((p) => p.date === date && p.type !== 'rest'), [practices, date])
  const groupName = useCallback((id?: string) => {
    if (!id) return 'All groups'
    return groups.find((g) => g.id === id || g.code === id)?.name ?? id
  }, [groups])

  useEffect(() => {
    if (dayPractices.length === 1) setPracticeId(dayPractices[0].id)
    else if (!dayPractices.some((p) => p.id === practiceId)) setPracticeId('')
  }, [dayPractices, practiceId])

  useEffect(() => {
    if (!practiceId || !orgId) { setRoster(null); return }
    let cancelled = false
    const cached = readRosterCache(practiceId)
    if (cached) { setRoster(cached); setRosterFromCache(true) } else setRoster(null)
    fetchPracticeAttendance(api, orgId, practiceId)
      .then((r) => { if (cancelled) return; setRoster(r); setRosterFromCache(false); writeRosterCache(practiceId, r) })
      .catch((err) => {
        if (cancelled) return
        if (!cached) setLoadError(reportUIError('roll-call-roster', err, 'The roster could not be loaded. Reconnect and try again.'))
        else setOnline(false)
      })
    return () => { cancelled = true }
  }, [api, orgId, practiceId, reloadKey])

  function mark(swimmerId: string, status: AttendanceStatus) {
    if (!practiceId) return
    setPermanentError(null)
    updateQueue((q) => enqueueMark(q, practiceId, swimmerId, status, Date.now()))
    void flush()
  }

  function markAllUnmarkedPresent() {
    if (!practiceId || !roster) return
    for (const r of roster) {
      if (!effectiveStatus(queueRef.current, practiceId, r.swimmer_id, r.status)) {
        updateQueue((q) => enqueueMark(q, practiceId, r.swimmer_id, 'present', Date.now()))
      }
    }
    void flush()
  }

  const pending = practiceId ? pendingForPractice(queue, practiceId) : []
  const allPending = Object.keys(queue).length
  const counts = (roster ?? []).reduce((acc, r) => {
    const s = effectiveStatus(queue, practiceId, r.swimmer_id, r.status)
    acc[s ?? 'unmarked'] = (acc[s ?? 'unmarked'] ?? 0) + 1
    return acc
  }, {} as Record<string, number>)

  const syncTone = allPending === 0 ? 'success' : online ? 'warning' : 'error'
  const syncText = allPending === 0
    ? 'All marks saved'
    : online
      ? `${allPending} mark${allPending === 1 ? '' : 's'} syncing${syncing ? '…' : ''}`
      : `Offline — ${allPending} mark${allPending === 1 ? '' : 's'} saved on this device, will sync when wifi returns`

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Roll call</h1>
          <p className="page-subtitle">Tap swimmers as they arrive. Marks sync when wifi returns.</p>
        </div>
        <input type="date" className="input" value={date} onChange={(e) => setDate(e.target.value)} aria-label="Practice date" style={{ minHeight: 44 }} />
      </div>

      <div role="status" aria-live="polite" className={`badge badge-${syncTone}`} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '12px 16px', borderRadius: 12, marginBottom: 16, fontSize: 'var(--fs-base)', textTransform: 'none', letterSpacing: 0 }}>
        <Icon name={allPending === 0 ? 'check-circle' : online ? 'refresh-cw' : 'alert-triangle'} size={16} />
        <span style={{ flex: 1 }}>{syncText}</span>
        {allPending > 0 && <button type="button" className="btn btn-secondary btn-sm" onClick={() => { updateQueue((q) => Object.fromEntries(Object.entries(q).map(([k, m]) => [k, { ...m, nextAttemptAt: 0 }]))); void flush() }}>Retry now</button>}
      </div>
      {permanentError && <p role="alert" style={{ fontSize: 'var(--fs-base)', color: 'var(--error)', marginBottom: 12 }}>{permanentError}</p>}
      {loadError && (
        <p role="alert" style={{ fontSize: 'var(--fs-base)', color: 'var(--warning)', marginBottom: 12 }}>
          {loadError} <button type="button" className="btn btn-ghost btn-sm" onClick={() => setReloadKey((k) => k + 1)}>Retry</button>
        </p>
      )}

      {dayPractices.length === 0 ? (
        <div className="card" style={{ padding: 24, fontSize: 'var(--fs-md)', color: 'var(--text-subtle)' }}>No practices on this date. Schedule one in Training, or pick another day.</div>
      ) : (
        <div role="tablist" aria-label="Practice" style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 16 }}>
          {dayPractices.map((p) => (
            <button key={p.id} role="tab" aria-selected={p.id === practiceId} type="button" onClick={() => setPracticeId(p.id)}
              className={p.id === practiceId ? 'btn btn-primary' : 'btn btn-secondary'} style={{ minHeight: 48, fontSize: 'var(--fs-md)' }}>
              {groupName(p.group_id)}{p.type !== 'water' ? ` · ${p.type}` : ''}
            </button>
          ))}
        </div>
      )}

      {practiceId && roster && (
        <>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, flexWrap: 'wrap', marginBottom: 12 }}>
            <p style={{ fontSize: 'var(--fs-md)', color: 'var(--text-subtle)' }}>
              <strong style={{ color: 'var(--text)' }}>{counts.present ?? 0}</strong> present · {counts.absent ?? 0} absent · {counts.excused ?? 0} excused · {counts.unmarked ?? 0} not marked
              {rosterFromCache ? ' · roster from this device' : ''}
            </p>
            {(counts.unmarked ?? 0) > 0 && <button type="button" className="btn btn-secondary" style={{ minHeight: 44 }} onClick={markAllUnmarkedPresent}>Mark the rest present</button>}
          </div>
          {roster.length === 0 ? (
            <div className="card" style={{ padding: 24, fontSize: 'var(--fs-md)', color: 'var(--text-subtle)' }}>No swimmers are assigned to this group yet.</div>
          ) : (
            <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 8 }}>
              {roster.map((r) => {
                const status = effectiveStatus(queue, practiceId, r.swimmer_id, r.status)
                const isPending = pending.some((m) => m.swimmerId === r.swimmer_id)
                return (
                  <li key={r.swimmer_id} className="card" style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '12px 12px', flexWrap: 'wrap' }}>
                    <span style={{ flex: '1 1 160px', minWidth: 0, fontSize: 'var(--fs-lg)', fontWeight: 600, color: 'var(--text)' }}>
                      {r.swimmer_name}
                      {isPending && <span style={{ display: 'block', fontSize: 'var(--fs-xs)', fontWeight: 500, color: 'var(--text-muted)' }}>Saving…</span>}
                    </span>
                    <span role="group" aria-label={`Attendance for ${r.swimmer_name}`} style={{ display: 'flex', gap: 6 }}>
                      {(Object.keys(STATUS_UI) as AttendanceStatus[]).map((s) => {
                        const ui = STATUS_UI[s]
                        const on = status === s
                        return (
                          <button key={s} type="button" aria-pressed={on} aria-label={ui.label} onClick={() => mark(r.swimmer_id, s)}
                            style={{
                              width: 64, height: 56, borderRadius: 12, cursor: 'pointer', fontSize: 'var(--fs-xl)', fontWeight: 700,
                              border: `2px solid ${on ? ui.color : 'var(--border-strong)'}`, background: on ? ui.bg : 'var(--bg-elevated)',
                              color: on ? ui.color : 'var(--text-muted)', touchAction: 'manipulation',
                            }}>
                            {ui.key}
                          </button>
                        )
                      })}
                    </span>
                  </li>
                )
              })}
            </ul>
          )}
        </>
      )}
      {practiceId && !roster && !loadError && <div className="skeleton" style={{ height: 240, borderRadius: 12 }} />}
    </div>
  )
}
