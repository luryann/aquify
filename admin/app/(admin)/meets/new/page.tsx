'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Icon } from '@/components/ui/icons'

type Course = 'SCY' | 'SCM' | 'LCM'

type MeetSessionDraft = {
  id: string
  section: string
  name: string
  date: string
  warmupTime: string
  startTime: string
}

type MeetDraft = {
  name: string
  course: Course
  hostClub: string
  location: string
  startDate: string
  endDate: string
  commitmentDeadline: string
  entryDeadline: string
  description: string
  ageGroups: string[]
  maxEntriesPerSwimmer: string
  swimmerSelfSignup: boolean
  sessions: MeetSessionDraft[]
}

const STEP_LABELS = ['Basic Info', 'Schedule', 'Entry Rules', 'Review']
const COURSES: Course[] = ['SCY', 'SCM', 'LCM']
const DEFAULT_AGE_GROUPS = ['8 & Under', '9-10', '11-12', '13-14', '15-16', '17-18', 'Open']
export default function NewMeetPage() {
  const router = useRouter()
  const [step, setStep] = useState(0)
  const [saving, setSaving] = useState(false)
  const [draft, setDraft] = useState<MeetDraft>({
    name: '',
    course: 'SCY',
    hostClub: '',
    location: '',
    startDate: '',
    endDate: '',
    commitmentDeadline: '',
    entryDeadline: '',
    description: '',
    ageGroups: ['9-10', '11-12', '13-14', '15-16', '17-18'],
    maxEntriesPerSwimmer: '3',
    swimmerSelfSignup: true,
    sessions: [
      { id: 'session-1', section: 'Day 1', name: 'Morning Session', date: '', warmupTime: '', startTime: '' },
    ],
  })

  function update<K extends keyof MeetDraft>(key: K, value: MeetDraft[K]) {
    setDraft((d) => ({ ...d, [key]: value }))
  }

  function toggleAgeGroup(ag: string) {
    setDraft((d) => ({
      ...d,
      ageGroups: d.ageGroups.includes(ag) ? d.ageGroups.filter((a) => a !== ag) : [...d.ageGroups, ag],
    }))
  }

  function canProceed(): boolean {
    if (step === 0) return !!draft.name && !!draft.course && !!draft.hostClub
    if (step === 1) return !!draft.startDate && !!draft.endDate && draft.sessions.every((session) => session.section && session.name)
    if (step === 2) return draft.ageGroups.length > 0
    return true
  }

  async function handleSave() {
    setSaving(true)
    await new Promise((r) => setTimeout(r, 600))
    setSaving(false)
    router.push('/meets?created=1')
  }

  function updateSession(sessionId: string, key: keyof MeetSessionDraft, value: string) {
    setDraft((current) => ({
      ...current,
      sessions: current.sessions.map((session) =>
        session.id === sessionId ? { ...session, [key]: value } : session,
      ),
    }))
  }

  function addSession() {
    setDraft((current) => ({
      ...current,
      sessions: [
        ...current.sessions,
        {
          id: `session-${current.sessions.length + 1}`,
          section: `Day ${current.sessions.length + 1}`,
          name: 'New Session',
          date: current.startDate,
          warmupTime: '',
          startTime: '',
        },
      ],
    }))
  }

  function removeSession(sessionId: string) {
    setDraft((current) => ({
      ...current,
      sessions: current.sessions.length === 1
        ? current.sessions
        : current.sessions.filter((session) => session.id !== sessionId),
    }))
  }

  return (
    <div style={{ padding: '32px', maxWidth: '760px', margin: '0 auto' }}>
      {/* Header */}
      <div style={{ marginBottom: '32px' }}>
        <button
          onClick={() => router.push('/meets')}
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', fontSize: '12px', fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '16px', padding: 0 }}
          onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--text)' }}
          onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--text-muted)' }}
        >
          <Icon name="arrow-left" size={13} />
          Back to Meets
        </button>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em' }}>
          Create Meet
        </h1>
      </div>

      {/* Step indicator */}
      <div style={{ display: 'flex', gap: '0', marginBottom: '32px' }}>
        {STEP_LABELS.map((label, idx) => (
          <div key={label} style={{ display: 'flex', alignItems: 'center', flex: idx < STEP_LABELS.length - 1 ? '1' : undefined }}>
            <div
              onClick={() => idx < step && setStep(idx)}
              style={{
                display: 'flex', alignItems: 'center', gap: '8px',
                cursor: idx < step ? 'pointer' : 'default',
              }}
            >
              <div style={{
                width: '28px', height: '28px', borderRadius: '50%', flexShrink: 0,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                background: idx < step ? 'var(--accent)' : idx === step ? 'var(--bg-elevated)' : 'var(--bg-surface)',
                border: `2px solid ${idx < step ? 'var(--accent)' : idx === step ? 'var(--accent-border)' : 'var(--border)'}`,
                fontSize: '11px', fontFamily: 'var(--font-mono)', fontWeight: 700,
                color: idx < step ? '#fff' : idx === step ? 'var(--accent)' : 'var(--text-muted)',
                transition: 'all 0.2s ease',
              }}>
                {idx < step ? (
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                ) : idx + 1}
              </div>
              <span style={{ fontSize: '12px', fontWeight: idx === step ? 600 : 400, color: idx === step ? 'var(--text)' : 'var(--text-muted)', whiteSpace: 'nowrap' }}>
                {label}
              </span>
            </div>
            {idx < STEP_LABELS.length - 1 && (
              <div style={{ flex: 1, height: '2px', margin: '0 12px', background: idx < step ? 'var(--accent)' : 'var(--border)', transition: 'background 0.3s ease', minWidth: '24px' }} />
            )}
          </div>
        ))}
      </div>

      {/* Step content */}
      <div className="card" style={{ padding: '32px' }}>
        {step === 0 && (
          <div>
            <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '24px' }}>
              Basic Information
            </h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                  Meet Name <span style={{ color: 'var(--error)' }}>*</span>
                </label>
                <input
                  value={draft.name}
                  onChange={(e) => update('name', e.target.value)}
                  placeholder="e.g. Spring Invitational 2026"
                  style={{ width: '100%', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '14px', fontFamily: 'var(--font-body)', outline: 'none', transition: 'border-color 0.15s' }}
                  onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                  onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                />
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
                <div>
                  <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                    Course <span style={{ color: 'var(--error)' }}>*</span>
                  </label>
                  <div style={{ display: 'flex', gap: '4px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', padding: '3px' }}>
                    {COURSES.map((c) => (
                      <button key={c} onClick={() => update('course', c)}
                        style={{
                          flex: 1, padding: '7px', borderRadius: '6px', border: 'none', cursor: 'pointer',
                          background: draft.course === c ? 'var(--accent)' : 'transparent',
                          color: draft.course === c ? '#fff' : 'var(--text-muted)',
                          fontSize: '12px', fontWeight: 700, fontFamily: 'var(--font-mono)', transition: 'all 0.15s',
                        }}>{c}</button>
                    ))}
                  </div>
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                    Host Club <span style={{ color: 'var(--error)' }}>*</span>
                  </label>
                  <input
                    value={draft.hostClub}
                    onChange={(e) => update('hostClub', e.target.value)}
                    placeholder="e.g. DARE Aquatics"
                    style={{ width: '100%', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none', transition: 'border-color 0.15s' }}
                    onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                    onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                  />
                </div>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                  Venue / Location
                </label>
                <input
                  value={draft.location}
                  onChange={(e) => update('location', e.target.value)}
                  placeholder="e.g. Aquatic Center, 123 Pool Rd, City, ST"
                  style={{ width: '100%', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none', transition: 'border-color 0.15s' }}
                  onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                  onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                  Description
                </label>
                <textarea
                  value={draft.description}
                  onChange={(e) => update('description', e.target.value)}
                  placeholder="Optional notes, special rules, travel info…"
                  rows={3}
                  style={{ width: '100%', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none', resize: 'vertical', transition: 'border-color 0.15s' }}
                  onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                  onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                />
              </div>
            </div>
          </div>
        )}

        {step === 1 && (
          <div>
            <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '24px' }}>
              Schedule & Deadlines
            </h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
                <div>
                  <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                    Start Date <span style={{ color: 'var(--error)' }}>*</span>
                  </label>
                  <input type="date" value={draft.startDate} onChange={(e) => update('startDate', e.target.value)}
                    style={{ width: '100%', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }} />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                    End Date <span style={{ color: 'var(--error)' }}>*</span>
                  </label>
                  <input type="date" value={draft.endDate} onChange={(e) => update('endDate', e.target.value)}
                    style={{ width: '100%', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }} />
                </div>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
                <div>
                  <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                    Commitment Deadline
                  </label>
                  <input type="datetime-local" value={draft.commitmentDeadline} onChange={(e) => update('commitmentDeadline', e.target.value)}
                    style={{ width: '100%', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }} />
                  <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '4px' }}>When parents must respond yes/no by</p>
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                    Entry Deadline
                  </label>
                  <input type="datetime-local" value={draft.entryDeadline} onChange={(e) => update('entryDeadline', e.target.value)}
                    style={{ width: '100%', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }} />
                  <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '4px' }}>Submission deadline to meet host</p>
                </div>
              </div>

              <div style={{ paddingTop: '8px', borderTop: '1px solid var(--border)' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
                  <div>
                    <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '4px' }}>
                      Session Sections
                    </label>
                    <p style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                      Build the day or section structure now so sessions are grouped correctly after creation.
                    </p>
                  </div>
                  <button className="btn btn-secondary" type="button" onClick={addSession}>
                    <Icon name="plus" size={13} /> Add Session
                  </button>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                  {draft.sessions.map((session, index) => (
                    <div key={session.id} style={{ border: '1px solid var(--border)', borderRadius: '10px', padding: '14px', background: 'var(--bg-surface)' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px' }}>
                        <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                          Session {index + 1}
                        </span>
                        {draft.sessions.length > 1 && (
                          <button className="btn btn-ghost" type="button" onClick={() => removeSession(session.id)}>
                            <Icon name="trash-2" size={12} />
                          </button>
                        )}
                      </div>
                      <div style={{ display: 'grid', gridTemplateColumns: '1.1fr 1.2fr 1fr 1fr 1fr', gap: '10px' }}>
                        <input className="input" value={session.section} onChange={(e) => updateSession(session.id, 'section', e.target.value)} placeholder="Day 1" />
                        <input className="input" value={session.name} onChange={(e) => updateSession(session.id, 'name', e.target.value)} placeholder="Morning Session" />
                        <input className="input" type="date" value={session.date} onChange={(e) => updateSession(session.id, 'date', e.target.value)} />
                        <input className="input" type="time" value={session.warmupTime} onChange={(e) => updateSession(session.id, 'warmupTime', e.target.value)} />
                        <input className="input" type="time" value={session.startTime} onChange={(e) => updateSession(session.id, 'startTime', e.target.value)} />
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        )}

        {step === 2 && (
          <div>
            <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '24px' }}>
              Entry Rules
            </h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '10px' }}>
                  Age Groups <span style={{ color: 'var(--error)' }}>*</span>
                </label>
                <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                  {DEFAULT_AGE_GROUPS.map((ag) => (
                    <button
                      key={ag}
                      onClick={() => toggleAgeGroup(ag)}
                      style={{
                        padding: '6px 14px', borderRadius: '20px',
                        background: draft.ageGroups.includes(ag) ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                        border: `1px solid ${draft.ageGroups.includes(ag) ? 'var(--accent-border)' : 'var(--border)'}`,
                        color: draft.ageGroups.includes(ag) ? 'var(--accent)' : 'var(--text-muted)',
                        fontSize: '12px', fontWeight: draft.ageGroups.includes(ag) ? 600 : 400,
                        cursor: 'pointer', fontFamily: 'var(--font-body)', transition: 'all 0.15s',
                      }}
                    >{ag}</button>
                  ))}
                </div>
              </div>
              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                  Max Individual Events per Swimmer
                </label>
                <div style={{ display: 'flex', gap: '4px', width: 'fit-content' }}>
                  {['2', '3', '4', '5', '6', 'Unlimited'].map((n) => (
                    <button key={n} onClick={() => update('maxEntriesPerSwimmer', n)}
                      style={{
                        padding: '7px 14px', borderRadius: '6px', border: `1px solid ${draft.maxEntriesPerSwimmer === n ? 'var(--accent-border)' : 'var(--border)'}`,
                        background: draft.maxEntriesPerSwimmer === n ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                        color: draft.maxEntriesPerSwimmer === n ? 'var(--accent)' : 'var(--text-muted)',
                        fontSize: '12px', fontFamily: 'var(--font-body)', cursor: 'pointer', transition: 'all 0.15s',
                        fontWeight: draft.maxEntriesPerSwimmer === n ? 600 : 400,
                      }}>{n}</button>
                  ))}
                </div>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '7px' }}>
                  Swimmer Self-Signup
                </label>
                <button
                  type="button"
                  onClick={() => update('swimmerSelfSignup', !draft.swimmerSelfSignup)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    width: '100%',
                    padding: '12px 14px',
                    borderRadius: '10px',
                    border: `1px solid ${draft.swimmerSelfSignup ? 'var(--accent-border)' : 'var(--border)'}`,
                    background: draft.swimmerSelfSignup ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                    color: 'var(--text)',
                    cursor: 'pointer',
                  }}
                >
                  <span style={{ fontSize: '13px' }}>
                    {draft.swimmerSelfSignup ? 'Enabled for swimmer portal event selection' : 'Disabled, parent-only commitment flow'}
                  </span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: draft.swimmerSelfSignup ? 'var(--accent)' : 'var(--text-muted)' }}>
                    {draft.swimmerSelfSignup ? 'ON' : 'OFF'}
                  </span>
                </button>
              </div>
            </div>
          </div>
        )}

        {step === 3 && (
          <div>
            <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '20px' }}>
              Review & Create
            </h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              {[
                { label: 'Name',          value: draft.name || '—' },
                { label: 'Course',        value: draft.course },
                { label: 'Host Club',     value: draft.hostClub || '—' },
                { label: 'Location',      value: draft.location || '—' },
                { label: 'Dates',         value: draft.startDate && draft.endDate ? `${draft.startDate} → ${draft.endDate}` : '—' },
                { label: 'Commit By',     value: draft.commitmentDeadline || '—' },
                { label: 'Entry By',      value: draft.entryDeadline || '—' },
                { label: 'Age Groups',    value: draft.ageGroups.join(', ') || '—' },
                { label: 'Max Events',    value: draft.maxEntriesPerSwimmer },
                { label: 'Self-Signup',   value: draft.swimmerSelfSignup ? 'Enabled' : 'Disabled' },
                { label: 'Sessions',      value: `${draft.sessions.length} configured across ${new Set(draft.sessions.map((session) => session.section)).size} sections` },
              ].map((r) => (
                <div key={r.label} style={{ display: 'flex', gap: '16px', padding: '10px 0', borderBottom: '1px solid var(--border)' }}>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', minWidth: '100px', flexShrink: 0, paddingTop: '1px' }}>{r.label}</span>
                  <span style={{ fontSize: '13px', color: 'var(--text)', fontWeight: 500 }}>{r.value}</span>
                </div>
              ))}
            </div>
            <div style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {draft.sessions.map((session) => (
                <div key={session.id} style={{ padding: '12px 14px', borderRadius: '10px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                  <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text)' }}>{session.section} · {session.name}</div>
                  <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '3px' }}>
                    {session.date || 'Date TBD'} · Warm-up {session.warmupTime || 'TBD'} · Start {session.startTime || 'TBD'}
                  </div>
                </div>
              ))}
            </div>
            <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '16px' }}>
              The meet will be created in <strong style={{ color: 'var(--text)' }}>draft</strong> state. Publish it when you&apos;re ready to open commitments.
            </p>
          </div>
        )}
      </div>

      {/* Nav buttons */}
      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '20px' }}>
        <button
          onClick={() => step > 0 ? setStep(step - 1) : router.push('/meets')}
          style={{ padding: '10px 22px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer', fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '7px', transition: 'all 0.15s' }}
          onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' }}
          onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
        >
          <Icon name="arrow-left" size={13} />
          {step === 0 ? 'Cancel' : 'Back'}
        </button>
        <button
          onClick={() => step < 3 ? setStep(step + 1) : handleSave()}
          disabled={!canProceed() || saving}
          style={{
            padding: '10px 28px', borderRadius: '8px', border: 'none',
            background: canProceed() && !saving ? 'var(--accent)' : 'var(--bg-elevated)',
            color: canProceed() && !saving ? '#fff' : 'var(--text-muted)',
            fontSize: '13px', fontWeight: 600, cursor: canProceed() && !saving ? 'pointer' : 'not-allowed',
            fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '8px', transition: 'background 0.15s',
          }}
          onMouseEnter={(e) => { if (canProceed() && !saving) e.currentTarget.style.background = '#d44f14' }}
          onMouseLeave={(e) => { if (canProceed() && !saving) e.currentTarget.style.background = 'var(--accent)' }}
        >
          {saving ? (
            <><div style={{ width: '13px', height: '13px', borderRadius: '50%', border: '2px solid rgba(255,255,255,0.3)', borderTopColor: '#fff', animation: 'spin 0.8s linear infinite' }} />Creating…</>
          ) : step < 3 ? (
            <>Next <Icon name="arrow-right" size={13} /></>
          ) : (
            <>Create Meet <Icon name="check" size={13} /></>
          )}
        </button>
      </div>
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}
