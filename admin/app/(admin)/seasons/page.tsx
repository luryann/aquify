'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  createSeason,
  deleteSeason,
  fetchSeasons,
  updateSeason,
  type SeasonRecord,
} from '@/lib/adminData'

type SeasonStatus = SeasonRecord['status']

const STATUS_LABELS: Record<SeasonStatus, string> = {
  active: 'Active',
  upcoming: 'Upcoming',
  completed: 'Completed',
}

const STATUS_COLORS: Record<SeasonStatus, { bg: string; border: string; text: string }> = {
  active:    { bg: 'var(--success-bg)',  border: 'var(--success-border)',  text: 'var(--success)' },
  upcoming:  { bg: 'var(--info-bg)',     border: 'var(--info-border)',     text: 'var(--info)' },
  completed: { bg: 'var(--bg-elevated)', border: 'var(--border)',          text: 'var(--text-muted)' },
}

function StatusBadge({ status }: { status: SeasonStatus }) {
  const c = STATUS_COLORS[status]
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: '5px',
      padding: '3px 10px', borderRadius: '20px',
      background: c.bg, border: `1px solid ${c.border}`,
      fontSize: '11px', fontFamily: 'var(--font-mono)', fontWeight: 600,
      color: c.text, textTransform: 'uppercase', letterSpacing: '0.06em',
    }}>
      <span style={{ width: '5px', height: '5px', borderRadius: '50%', background: c.text, display: 'inline-block' }} />
      {STATUS_LABELS[status]}
    </span>
  )
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <label style={{
      display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)',
      color: 'var(--text-muted)', letterSpacing: '0.06em',
      textTransform: 'uppercase', marginBottom: '7px',
    }}>
      {children}
    </label>
  )
}

const inputStyle = {
  width: '100%', padding: '10px 12px',
  background: 'var(--bg-elevated)', border: '1px solid var(--border)',
  borderRadius: '8px', color: 'var(--text)', fontSize: '13px',
  fontFamily: 'var(--font-body)', outline: 'none',
  transition: 'border-color 0.15s',
} as React.CSSProperties

type SeasonDraft = {
  name: string
  startDate: string
  endDate: string
  status: SeasonStatus
}

function draftFromSeason(season: SeasonRecord): SeasonDraft {
  return {
    name: season.name,
    startDate: season.start_date,
    endDate: season.end_date,
    status: season.status,
  }
}

function formatSeasonDate(value: string) {
  if (!value) return '—'
  return new Date(value + 'T00:00:00').toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })
}

function StatusPicker({ value, onChange }: { value: SeasonStatus; onChange: (s: SeasonStatus) => void }) {
  return (
    <div style={{ display: 'flex', gap: '6px' }}>
      {(['active', 'upcoming', 'completed'] as SeasonStatus[]).map((s) => (
        <button
          key={s}
          onClick={() => onChange(s)}
          style={{
            padding: '6px 14px', borderRadius: '6px', border: `1px solid ${value === s ? 'var(--accent-border)' : 'var(--border)'}`,
            background: value === s ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
            color: value === s ? 'var(--accent)' : 'var(--text-muted)',
            fontSize: '12px', fontFamily: 'var(--font-mono)', cursor: 'pointer',
            fontWeight: value === s ? 600 : 400, transition: 'all 0.15s',
            textTransform: 'uppercase', letterSpacing: '0.05em',
          }}
        >{STATUS_LABELS[s]}</button>
      ))}
    </div>
  )
}

function SeasonCard({
  season,
  accent,
  saving,
  onSave,
  onDelete,
}: {
  season: SeasonRecord
  accent?: boolean
  saving: boolean
  onSave: (draft: SeasonDraft) => Promise<boolean>
  onDelete: () => Promise<void>
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<SeasonDraft>(() => draftFromSeason(season))

  function startEditing() {
    setDraft(draftFromSeason(season))
    setEditing(true)
  }

  async function handleSave() {
    const saved = await onSave(draft)
    if (saved) setEditing(false)
  }

  function u<K extends keyof SeasonDraft>(key: K, value: SeasonDraft[K]) {
    setDraft((d) => ({ ...d, [key]: value }))
  }

  const name = editing ? draft.name : season.name
  const startDate = editing ? draft.startDate : season.start_date
  const endDate = editing ? draft.endDate : season.end_date
  const status = editing ? draft.status : season.status

  return (
    <div className="card" style={{
      padding: '24px',
      border: accent ? '1px solid var(--accent-border)' : '1px solid var(--border)',
      background: accent ? 'rgba(235,93,30,0.03)' : undefined,
      position: 'relative',
    }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '20px' }}>
        <div>
          <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.10em', marginBottom: '6px' }}>
            {season.status === 'active' ? 'Current Season' : season.status === 'upcoming' ? 'Upcoming Season' : 'Past Season'}
          </p>
          {editing ? (
            <input
              value={draft.name}
              onChange={(e) => u('name', e.target.value)}
              style={{ ...inputStyle, fontSize: '18px', fontFamily: 'var(--font-display)', padding: '6px 10px', width: '240px' }}
              onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
              onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
            />
          ) : (
            <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '22px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em' }}>
              {name || <span style={{ color: 'var(--text-muted)' }}>Unnamed Season</span>}
            </h2>
          )}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          {!editing && <StatusBadge status={status} />}
          <button
            onClick={() => editing ? setEditing(false) : startEditing()}
            className="btn btn-ghost"
            style={{ gap: '5px', fontSize: '12px' }}
          >
            <Icon name={editing ? 'x' : 'edit-2'} size={12} />
            {editing ? 'Cancel' : 'Edit'}
          </button>
        </div>
      </div>

      {/* Fields */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px', marginBottom: '16px' }}>
        <div>
          <FieldLabel>Start Date</FieldLabel>
          {editing ? (
            <input type="date" value={draft.startDate} onChange={(e) => u('startDate', e.target.value)}
              style={inputStyle}
              onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
              onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
            />
          ) : (
            <p style={{ fontSize: '14px', color: startDate ? 'var(--text)' : 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
              {formatSeasonDate(startDate)}
            </p>
          )}
        </div>
        <div>
          <FieldLabel>End Date</FieldLabel>
          {editing ? (
            <input type="date" value={draft.endDate} onChange={(e) => u('endDate', e.target.value)}
              style={inputStyle}
              onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
              onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
            />
          ) : (
            <p style={{ fontSize: '14px', color: endDate ? 'var(--text)' : 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
              {formatSeasonDate(endDate)}
            </p>
          )}
        </div>
      </div>

      {editing && (
        <div style={{ marginBottom: '16px' }}>
          <FieldLabel>Status</FieldLabel>
          <StatusPicker value={draft.status} onChange={(s) => u('status', s)} />
        </div>
      )}

      {/* Duration bar (display mode) */}
      {!editing && startDate && endDate && (
        <div style={{ marginBottom: '16px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '5px' }}>
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
              {Math.round((new Date(endDate + 'T00:00:00').getTime() - new Date(startDate + 'T00:00:00').getTime()) / (1000 * 60 * 60 * 24 * 7))} weeks
            </span>
            {status === 'active' && (() => {
              const now = Date.now()
              const start = new Date(startDate + 'T00:00:00').getTime()
              const end = new Date(endDate + 'T00:00:00').getTime()
              const pct = Math.min(100, Math.max(0, Math.round((now - start) / (end - start) * 100)))
              return (
                <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--accent)' }}>
                  {pct}% complete
                </span>
              )
            })()}
          </div>
          {status === 'active' && (() => {
            const now = Date.now()
            const start = new Date(startDate + 'T00:00:00').getTime()
            const end = new Date(endDate + 'T00:00:00').getTime()
            const pct = Math.min(100, Math.max(0, Math.round((now - start) / (end - start) * 100)))
            return (
              <div style={{ height: '4px', borderRadius: '2px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
                <div style={{ height: '100%', width: `${pct}%`, background: 'var(--accent)', borderRadius: '2px', transition: 'width 0.6s ease' }} />
              </div>
            )
          })()}
        </div>
      )}

      {/* Save / delete actions */}
      {editing && (
        <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '8px' }}>
          <button onClick={() => { void onDelete() }} className="btn btn-ghost" disabled={saving} style={{ fontSize: '12px', color: 'var(--danger, #dc2626)', gap: '5px' }}>
            <Icon name="trash-2" size={12} /> Delete
          </button>
          <button onClick={() => { void handleSave() }} className="btn btn-primary" disabled={saving} style={{ fontSize: '13px' }}>
            <Icon name="check" size={13} /> {saving ? 'Saving…' : 'Save Changes'}
          </button>
        </div>
      )}
    </div>
  )
}

const EMPTY_DRAFT: SeasonDraft = { name: '', startDate: '', endDate: '', status: 'upcoming' }

export default function SeasonsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [seasons, setSeasons] = useState<SeasonRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [actionError, setActionError] = useState('')
  const [saving, setSaving] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)

  const [creating, setCreating] = useState(false)
  const [createDraft, setCreateDraft] = useState<SeasonDraft>(EMPTY_DRAFT)

  const [goals, setGoals] = useState('')
  const [goalsSaved, setGoalsSaved] = useState(false)

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!orgId) {
        setLoading(false)
        return
      }
      setLoading(true)
      setLoadError('')
      try {
        const list = await fetchSeasons(api, orgId)
        if (cancelled) return
        setSeasons(list)
      } catch (err) {
        if (cancelled) return
        setLoadError(err instanceof Error ? err.message : 'Failed to load seasons.')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => { cancelled = true }
  }, [api, orgId, reloadKey])

  const activeSeason = useMemo(
    () => seasons.find((s) => s.status === 'active') ?? seasons[0] ?? null,
    [seasons],
  )

  useEffect(() => {
    setGoals(activeSeason?.goals ?? '')
  }, [activeSeason?.id, activeSeason?.goals])

  const replaceSeason = useCallback((updated: SeasonRecord) => {
    setSeasons((current) => current.map((s) => (s.id === updated.id ? updated : s)))
  }, [])

  async function handleSaveSeason(season: SeasonRecord, draft: SeasonDraft): Promise<boolean> {
    setSaving(true)
    setActionError('')
    try {
      const updated = await updateSeason(api, orgId, season.id, {
        name: draft.name,
        start_date: draft.startDate,
        end_date: draft.endDate,
        status: draft.status,
      })
      replaceSeason(updated)
      return true
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to save season.')
      return false
    } finally {
      setSaving(false)
    }
  }

  async function handleDeleteSeason(season: SeasonRecord) {
    setSaving(true)
    setActionError('')
    try {
      await deleteSeason(api, orgId, season.id)
      setSeasons((current) => current.filter((s) => s.id !== season.id))
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to delete season.')
    } finally {
      setSaving(false)
    }
  }

  async function handleCreateSeason() {
    if (!createDraft.name.trim()) {
      setActionError('Season name is required.')
      return
    }
    setSaving(true)
    setActionError('')
    try {
      const created = await createSeason(api, orgId, {
        name: createDraft.name.trim(),
        start_date: createDraft.startDate,
        end_date: createDraft.endDate,
        status: createDraft.status,
      })
      setSeasons((current) => [created, ...current])
      setCreating(false)
      setCreateDraft(EMPTY_DRAFT)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to create season.')
    } finally {
      setSaving(false)
    }
  }

  async function handleSaveGoals() {
    if (!activeSeason) return
    setSaving(true)
    setActionError('')
    try {
      const updated = await updateSeason(api, orgId, activeSeason.id, { goals })
      replaceSeason(updated)
      setGoalsSaved(true)
      setTimeout(() => setGoalsSaved(false), 2000)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to save goals.')
    } finally {
      setSaving(false)
    }
  }

  const timelineSeasons = seasons.filter((s) => s.start_date && s.end_date)

  return (
    <div style={{ padding: '32px', maxWidth: '900px', margin: '0 auto' }}>
      {/* Page header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '32px' }}>
        <div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em', marginBottom: '6px' }}>
            Seasons
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
            Manage season dates, goals, and transition planning.
          </p>
        </div>
        <button onClick={() => { setCreating((c) => !c); setActionError('') }} className="btn btn-primary" style={{ fontSize: '13px', gap: '6px' }}>
          <Icon name={creating ? 'x' : 'plus'} size={13} /> {creating ? 'Cancel' : 'New Season'}
        </button>
      </div>

      {/* Errors */}
      {(loadError || actionError) && (
        <div className="card" style={{ padding: '14px 18px', marginBottom: '20px', border: '1px solid var(--danger-border, #fecaca)', background: 'var(--danger-bg, #fef2f2)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px' }}>
          <p style={{ fontSize: '13px', color: 'var(--danger, #dc2626)' }}>{loadError || actionError}</p>
          {loadError && (
            <button onClick={() => setReloadKey((k) => k + 1)} className="btn btn-secondary" style={{ fontSize: '12px', flexShrink: 0 }}>
              Retry
            </button>
          )}
        </div>
      )}

      {/* Create form */}
      {creating && (
        <div className="card" style={{ padding: '24px', marginBottom: '24px', border: '1px solid var(--accent-border)' }}>
          <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.10em', marginBottom: '16px' }}>
            New Season
          </p>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px', marginBottom: '16px' }}>
            <div style={{ gridColumn: '1 / -1' }}>
              <FieldLabel>Name</FieldLabel>
              <input value={createDraft.name} onChange={(e) => setCreateDraft((d) => ({ ...d, name: e.target.value }))} placeholder="2026–2027 Season" style={inputStyle} />
            </div>
            <div>
              <FieldLabel>Start Date</FieldLabel>
              <input type="date" value={createDraft.startDate} onChange={(e) => setCreateDraft((d) => ({ ...d, startDate: e.target.value }))} style={inputStyle} />
            </div>
            <div>
              <FieldLabel>End Date</FieldLabel>
              <input type="date" value={createDraft.endDate} onChange={(e) => setCreateDraft((d) => ({ ...d, endDate: e.target.value }))} style={inputStyle} />
            </div>
          </div>
          <div style={{ marginBottom: '16px' }}>
            <FieldLabel>Status</FieldLabel>
            <StatusPicker value={createDraft.status} onChange={(s) => setCreateDraft((d) => ({ ...d, status: s }))} />
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <button onClick={() => { void handleCreateSeason() }} className="btn btn-primary" disabled={saving} style={{ fontSize: '13px' }}>
              <Icon name="check" size={13} /> {saving ? 'Creating…' : 'Create Season'}
            </button>
          </div>
        </div>
      )}

      {/* Loading */}
      {loading && (
        <div className="card" style={{ padding: '40px', textAlign: 'center', marginBottom: '24px' }}>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>Loading seasons…</p>
        </div>
      )}

      {/* Empty state */}
      {!loading && !loadError && seasons.length === 0 && !creating && (
        <div className="card" style={{ padding: '40px', textAlign: 'center', marginBottom: '24px' }}>
          <p style={{ fontSize: '14px', color: 'var(--text)', marginBottom: '6px' }}>No seasons yet</p>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>Create your first season to start planning dates and goals.</p>
        </div>
      )}

      {/* Season cards */}
      {!loading && seasons.length > 0 && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '20px', marginBottom: '24px' }}>
          {seasons.map((season) => (
            <SeasonCard
              key={season.id}
              season={season}
              accent={season.status === 'active'}
              saving={saving}
              onSave={(draft) => handleSaveSeason(season, draft)}
              onDelete={() => handleDeleteSeason(season)}
            />
          ))}
        </div>
      )}

      {/* Season Goals */}
      {!loading && activeSeason && (
        <div className="card" style={{ padding: '24px', marginBottom: '24px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
            <div>
              <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.10em', marginBottom: '4px' }}>
                Season Goals
              </p>
              <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
                Program objectives, performance targets, and coaching priorities for {activeSeason.name}.
              </p>
            </div>
            <button
              onClick={() => { void handleSaveGoals() }}
              className="btn btn-secondary"
              disabled={saving}
              style={{ fontSize: '12px', gap: '6px', flexShrink: 0 }}
            >
              {goalsSaved ? (
                <><Icon name="check" size={12} /> Saved</>
              ) : (
                <><Icon name="check" size={12} /> {saving ? 'Saving…' : 'Save Goals'}</>
              )}
            </button>
          </div>
          <textarea
            value={goals}
            onChange={(e) => setGoals(e.target.value)}
            placeholder={'e.g.\n• Qualify 10+ swimmers for sectionals\n• Improve team average in 200 fly by 3%\n• Launch new age group development track\n• Increase meet participation rate to 85%'}
            rows={8}
            style={{
              width: '100%', padding: '12px 14px',
              background: 'var(--bg-elevated)', border: '1px solid var(--border)',
              borderRadius: '8px', color: 'var(--text)', fontSize: '13px',
              fontFamily: 'var(--font-body)', outline: 'none', resize: 'vertical',
              lineHeight: '1.6', transition: 'border-color 0.15s',
            }}
            onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
            onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
          />
        </div>
      )}

      {/* Timeline overview */}
      {!loading && timelineSeasons.length > 0 && (
        <div className="card" style={{ padding: '24px' }}>
          <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.10em', marginBottom: '20px' }}>
            Timeline Overview
          </p>

          {timelineSeasons.map((s, i) => {
            const startDate = new Date(s.start_date + 'T00:00:00')
            const endDate = new Date(s.end_date + 'T00:00:00')
            const startYear = startDate.getFullYear()
            const endYear = endDate.getFullYear()
            const months = Math.round((endDate.getTime() - startDate.getTime()) / (1000 * 60 * 60 * 24 * 30))
            const color = s.status === 'active' ? 'var(--accent)' : s.status === 'upcoming' ? 'var(--info)' : 'var(--text-muted)'

            return (
              <div key={s.id} style={{ display: 'flex', alignItems: 'center', gap: '16px', marginBottom: i === timelineSeasons.length - 1 ? '0' : '14px' }}>
                <div style={{ width: '120px', fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', flexShrink: 0 }}>
                  {startYear} – {endYear}
                </div>
                <div style={{ flex: 1, height: '8px', borderRadius: '4px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
                  <div style={{
                    height: '100%',
                    width: s.status === 'active'
                      ? `${Math.min(100, Math.max(5, Math.round((Date.now() - startDate.getTime()) / (endDate.getTime() - startDate.getTime()) * 100)))}%`
                      : s.status === 'upcoming' ? '5%' : '100%',
                    background: color,
                    borderRadius: '4px',
                    opacity: s.status === 'upcoming' ? 0.4 : 1,
                    transition: 'width 0.6s ease',
                  }} />
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', width: '160px', flexShrink: 0 }}>
                  <span style={{ fontSize: '12px', color: 'var(--text)', fontWeight: 500 }}>{s.name}</span>
                  <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                    {months}mo
                  </span>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
