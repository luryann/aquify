'use client'

import { useState, useRef, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'

type SeasonStatus = 'active' | 'upcoming' | 'completed'

type Season = {
  name: string
  startDate: string
  endDate: string
  status: SeasonStatus
  goals: string
}

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

function SeasonCard({
  title,
  season,
  onChange,
  accent,
}: {
  title: string
  season: Season
  onChange: (updates: Partial<Season>) => void
  accent?: boolean
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<Season>(season)
  const savedRef = useRef(season)
  useEffect(() => { savedRef.current = season }, [season])

  function handleSave() {
    onChange(draft)
    setEditing(false)
  }

  function handleCancel() {
    setDraft(savedRef.current)
    setEditing(false)
  }

  function u<K extends keyof Season>(key: K, value: Season[K]) {
    setDraft((d) => ({ ...d, [key]: value }))
  }

  const displaySeason = editing ? draft : season

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
            {title}
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
              {displaySeason.name || <span style={{ color: 'var(--text-muted)' }}>Unnamed Season</span>}
            </h2>
          )}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          {!editing && <StatusBadge status={displaySeason.status} />}
          <button
            onClick={() => editing ? handleCancel() : setEditing(true)}
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
            <p style={{ fontSize: '14px', color: displaySeason.startDate ? 'var(--text)' : 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
              {displaySeason.startDate
                ? new Date(displaySeason.startDate + 'T00:00:00').toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })
                : '—'}
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
            <p style={{ fontSize: '14px', color: displaySeason.endDate ? 'var(--text)' : 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
              {displaySeason.endDate
                ? new Date(displaySeason.endDate + 'T00:00:00').toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })
                : '—'}
            </p>
          )}
        </div>
      </div>

      {editing && (
        <div style={{ marginBottom: '16px' }}>
          <FieldLabel>Status</FieldLabel>
          <div style={{ display: 'flex', gap: '6px' }}>
            {(['active', 'upcoming', 'completed'] as SeasonStatus[]).map((s) => (
              <button
                key={s}
                onClick={() => u('status', s)}
                style={{
                  padding: '6px 14px', borderRadius: '6px', border: `1px solid ${draft.status === s ? 'var(--accent-border)' : 'var(--border)'}`,
                  background: draft.status === s ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                  color: draft.status === s ? 'var(--accent)' : 'var(--text-muted)',
                  fontSize: '12px', fontFamily: 'var(--font-mono)', cursor: 'pointer',
                  fontWeight: draft.status === s ? 600 : 400, transition: 'all 0.15s',
                  textTransform: 'uppercase', letterSpacing: '0.05em',
                }}
              >{STATUS_LABELS[s]}</button>
            ))}
          </div>
        </div>
      )}

      {/* Duration bar (display mode) */}
      {!editing && displaySeason.startDate && displaySeason.endDate && (
        <div style={{ marginBottom: '16px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '5px' }}>
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
              {Math.round((new Date(displaySeason.endDate + 'T00:00:00').getTime() - new Date(displaySeason.startDate + 'T00:00:00').getTime()) / (1000 * 60 * 60 * 24 * 7))} weeks
            </span>
            {displaySeason.status === 'active' && (() => {
              const now = Date.now()
              const start = new Date(displaySeason.startDate + 'T00:00:00').getTime()
              const end = new Date(displaySeason.endDate + 'T00:00:00').getTime()
              const pct = Math.min(100, Math.max(0, Math.round((now - start) / (end - start) * 100)))
              return (
                <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--accent)' }}>
                  {pct}% complete
                </span>
              )
            })()}
          </div>
          {displaySeason.status === 'active' && (() => {
            const now = Date.now()
            const start = new Date(displaySeason.startDate + 'T00:00:00').getTime()
            const end = new Date(displaySeason.endDate + 'T00:00:00').getTime()
            const pct = Math.min(100, Math.max(0, Math.round((now - start) / (end - start) * 100)))
            return (
              <div style={{ height: '4px', borderRadius: '2px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
                <div style={{ height: '100%', width: `${pct}%`, background: 'var(--accent)', borderRadius: '2px', transition: 'width 0.6s ease' }} />
              </div>
            )
          })()}
        </div>
      )}

      {/* Save button */}
      {editing && (
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '8px' }}>
          <button onClick={handleSave} className="btn btn-primary" style={{ fontSize: '13px' }}>
            <Icon name="check" size={13} /> Save Changes
          </button>
        </div>
      )}
    </div>
  )
}

export default function SeasonsPage() {
  const [currentSeason, setCurrentSeason] = useState<Season>({
    name: '2025–2026 Season',
    startDate: '2025-09-01',
    endDate: '2026-06-30',
    status: 'active',
    goals: '',
  })

  const [nextSeason, setNextSeason] = useState<Season>({
    name: '2026–2027 Season',
    startDate: '2026-09-01',
    endDate: '2027-06-30',
    status: 'upcoming',
    goals: '',
  })

  const [goals, setGoals] = useState('')
  const [goalsSaved, setGoalsSaved] = useState(false)

  function handleSaveGoals() {
    setCurrentSeason((s) => ({ ...s, goals }))
    setGoalsSaved(true)
    setTimeout(() => setGoalsSaved(false), 2000)
  }

  return (
    <div style={{ padding: '32px', maxWidth: '900px', margin: '0 auto' }}>
      {/* Page header */}
      <div style={{ marginBottom: '32px' }}>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em', marginBottom: '6px' }}>
          Seasons
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
          Manage season dates, goals, and transition planning.
        </p>
      </div>

      {/* Season cards */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '20px', marginBottom: '24px' }}>
        <SeasonCard
          title="Current Season"
          season={currentSeason}
          onChange={(u) => setCurrentSeason((s) => ({ ...s, ...u }))}
          accent
        />
        <SeasonCard
          title="Next Season"
          season={nextSeason}
          onChange={(u) => setNextSeason((s) => ({ ...s, ...u }))}
        />
      </div>

      {/* Season Goals */}
      <div className="card" style={{ padding: '24px', marginBottom: '24px' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
          <div>
            <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.10em', marginBottom: '4px' }}>
              Season Goals
            </p>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
              Program objectives, performance targets, and coaching priorities for {currentSeason.name}.
            </p>
          </div>
          <button
            onClick={handleSaveGoals}
            className="btn btn-secondary"
            style={{ fontSize: '12px', gap: '6px', flexShrink: 0 }}
          >
            {goalsSaved ? (
              <><Icon name="check" size={12} /> Saved</>
            ) : (
              <><Icon name="check" size={12} /> Save Goals</>
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

      {/* Timeline overview */}
      <div className="card" style={{ padding: '24px' }}>
        <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.10em', marginBottom: '20px' }}>
          Timeline Overview
        </p>

        {[
          { label: currentSeason.name, start: currentSeason.startDate, end: currentSeason.endDate, status: currentSeason.status },
          { label: nextSeason.name, start: nextSeason.startDate, end: nextSeason.endDate, status: nextSeason.status },
        ].filter(s => s.start && s.end).map((s, i) => {
          const startDate = new Date(s.start + 'T00:00:00')
          const endDate = new Date(s.end + 'T00:00:00')
          const startYear = startDate.getFullYear()
          const endYear = endDate.getFullYear()
          const months = Math.round((endDate.getTime() - startDate.getTime()) / (1000 * 60 * 60 * 24 * 30))
          const color = s.status === 'active' ? 'var(--accent)' : s.status === 'upcoming' ? 'var(--info)' : 'var(--text-muted)'

          return (
            <div key={i} style={{ display: 'flex', alignItems: 'center', gap: '16px', marginBottom: i === 0 ? '14px' : '0' }}>
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
                <span style={{ fontSize: '12px', color: 'var(--text)', fontWeight: 500 }}>{s.label}</span>
                <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                  {months}mo
                </span>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
