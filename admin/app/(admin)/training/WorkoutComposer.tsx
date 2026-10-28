'use client'

import { useEffect, useRef, useState } from 'react'
import { ApiError, type ApiClient } from '@/lib/api'
import { parseWorkoutText, type Course, type ParsedWorkout, type WorkoutInput } from '@/lib/trainingData'
import { formatDistance, formatDuration, parseTags } from '@/lib/workout-format'
import { WorkoutSheet, ZoneBar } from '@/components/training/WorkoutSheet'

export type ComposerInitial = { title: string; course: Course; tags: string[]; text: string }

const EXAMPLE = `Warmup:
400 choice
8x50 kick w/ fins @1:00
Main set
3 x {4x100 free @1:30 EN2, 100 easy} rest 1:00
6x50 fly @:55 SP2
Cooldown: 200 easy`

const fieldStyle: React.CSSProperties = {
  width: '100%', padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)',
  borderRadius: '8px', color: 'var(--text)', fontSize: 'var(--fs-base)', fontFamily: 'var(--font-body)', outline: 'none',
}

function Label({ children, htmlFor }: { children: React.ReactNode; htmlFor: string }) {
  return <label htmlFor={htmlFor} style={{ display: 'block', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginBottom: '6px' }}>{children}</label>
}

export function errorText(error: unknown, fallback: string): string {
  if (error instanceof ApiError) return error.message
  if (error instanceof Error && error.message) return error.message
  return fallback
}

/**
 * Quick-entry workout editor: coaches type shorthand, the server parses it
 * into structured sets and computes totals, and the live preview shows
 * exactly what will be saved. Lines the parser cannot read block saving.
 */
export function WorkoutComposer({
  api, orgId, initial, saveLabel, onSave, onCancel, showTags = true,
}: {
  api: ApiClient
  orgId: string
  initial: ComposerInitial
  saveLabel: string
  onSave: (input: WorkoutInput) => Promise<void>
  onCancel?: () => void
  showTags?: boolean
}) {
  const [title, setTitle] = useState(initial.title)
  const [course, setCourse] = useState<Course>(initial.course)
  const [tags, setTags] = useState(initial.tags.join(', '))
  const [text, setText] = useState(initial.text)
  const [parsed, setParsed] = useState<ParsedWorkout | null>(null)
  const [parseError, setParseError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const seq = useRef(0)

  useEffect(() => {
    if (!text.trim()) {
      setParsed(null)
      return
    }
    const id = ++seq.current
    const timer = setTimeout(async () => {
      try {
        const res = await parseWorkoutText(api, orgId, text, course)
        if (id === seq.current) { setParsed(res); setParseError(null) }
      } catch (error) {
        if (id === seq.current) setParseError(errorText(error, 'Couldn’t read this workout. Try again.'))
      }
    }, 350)
    return () => clearTimeout(timer)
  }, [api, orgId, text, course])

  const blocking = !parsed || parsed.errors.length > 0 || parsed.sections.length === 0
  const canSave = !!title.trim() && !blocking && !saving

  async function handleSave() {
    if (!parsed || !canSave) return
    setSaving(true)
    setSaveError(null)
    try {
      await onSave({ title: title.trim(), course, tags: parseTags(tags), sections: parsed.sections })
    } catch (error) {
      setSaveError(errorText(error, 'Couldn’t save the workout. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="composer-grid" style={{ display: 'grid', gap: '20px' }}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', minWidth: 0 }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) 96px', gap: '12px' }}>
          <div>
            <Label htmlFor="wk-title">Title</Label>
            <input id="wk-title" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Threshold Tuesday" style={fieldStyle} maxLength={160} />
          </div>
          <div>
            <Label htmlFor="wk-course">Course</Label>
            <select id="wk-course" value={course} onChange={(e) => setCourse(e.target.value as Course)} style={fieldStyle}>
              <option value="SCY">SCY</option>
              <option value="SCM">SCM</option>
              <option value="LCM">LCM</option>
            </select>
          </div>
        </div>
        {showTags && (
          <div>
            <Label htmlFor="wk-tags">Tags, comma separated</Label>
            <input id="wk-tags" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="threshold, senior, taper" style={fieldStyle} />
          </div>
        )}
        <div>
          <Label htmlFor="wk-text">Sets</Label>
          <textarea
            id="wk-text" value={text} onChange={(e) => setText(e.target.value)} rows={14} spellCheck={false}
            placeholder={EXAMPLE}
            style={{ ...fieldStyle, fontFamily: 'var(--font-mono)', fontSize: 'var(--fs-base)', lineHeight: 1.6, resize: 'vertical' }}
          />
          <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '6px', lineHeight: 1.5 }}>
            One set per line: <code>8x100 free @1:30 EN2</code>. Start a section with <code>Warmup:</code>, <code>Main</code>, <code>Kick</code>, <code>Cooldown</code>.
            Group rounds with <code>3 x {'{'}4x100, 100 easy{'}'}</code> or <code>3 rounds:</code> and indented lines.
          </p>
        </div>
        {(parsed?.errors.length ?? 0) > 0 && (
          <ul role="alert" style={{ margin: 0, padding: '12px 12px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', listStyle: 'none', fontSize: 'var(--fs-sm)', color: 'var(--error)', display: 'flex', flexDirection: 'column', gap: '4px' }}>
            {parsed!.errors.map((e, i) => (
              <li key={i}>{e.line > 0 ? `Line ${e.line}: ` : ''}{e.message}{e.text ? ` (“${e.text}”)` : ''}</li>
            ))}
          </ul>
        )}
        {(parseError || saveError) && <p role="alert" style={{ fontSize: 'var(--fs-sm)', color: 'var(--error)' }}>{saveError ?? parseError}</p>}
        <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
          {onCancel && <button type="button" className="btn btn-secondary" onClick={onCancel}>Cancel</button>}
          <button type="button" className="btn btn-primary" onClick={handleSave} disabled={!canSave}>
            {saving ? 'Saving…' : saveLabel}
          </button>
        </div>
      </div>

      <div style={{ minWidth: 0, border: '1px solid var(--border)', borderRadius: '12px', padding: '16px', background: 'var(--bg-surface)' }}>
        {parsed && parsed.sections.length > 0 ? (
          <>
            <div style={{ display: 'flex', gap: '20px', flexWrap: 'wrap', alignItems: 'baseline', marginBottom: '12px' }}>
              <span style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-3xl)', color: 'var(--text)', lineHeight: 1 }}>
                {formatDistance(parsed.totals.distance, parsed.totals.unit)}
              </span>
              <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)' }}>about {formatDuration(parsed.totals.estimated_seconds)}</span>
              <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)' }}>{parsed.totals.lengths} lengths</span>
            </div>
            <div style={{ marginBottom: '16px' }}><ZoneBar byZone={parsed.totals.by_zone} /></div>
            {parsed.totals.warnings.length > 0 && (
              <ul style={{ margin: '0 0 16px', padding: '8px 12px', listStyle: 'none', fontSize: 'var(--fs-sm)', color: 'var(--warning)', background: 'var(--warning-bg)', borderRadius: '8px' }}>
                {parsed.totals.warnings.map((w, i) => <li key={i}>{w}</li>)}
              </ul>
            )}
            <WorkoutSheet workout={parsed} />
          </>
        ) : (
          <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>
            The structured workout, distance and zone mix appear here as you type.
          </p>
        )}
      </div>
      <style>{`@media (min-width: 960px) { .composer-grid { grid-template-columns: minmax(0, 5fr) minmax(0, 6fr); } }`}</style>
    </div>
  )
}
