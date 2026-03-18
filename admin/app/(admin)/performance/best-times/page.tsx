'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

type Course = 'SCY' | 'SCM' | 'LCM'
type BestTime = {
  id: string
  event: string
  course: Course
  timeSeconds: number
  achievedAt: string
  meetName: string
  source: 'import' | 'manual'
  isOverride: boolean
}

type Swimmer = {
  id: string
  name: string
  age: number
  gender: 'M' | 'F'
  group: string
  ussNumber: string
  bestTimes: BestTime[]
}

function fmtTime(sec: number): string {
  if (sec < 60) return sec.toFixed(2)
  const m = Math.floor(sec / 60)
  const s = (sec % 60).toFixed(2).padStart(5, '0')
  return `${m}:${s}`
}

const MOCK_SWIMMERS: Swimmer[] = [
  {
    id: 'sw1', name: 'Marcus Allen', age: 14, gender: 'M', group: 'Senior', ussNumber: '2014010100041A',
    bestTimes: [
      { id: 'bt1', event: '100 Free', course: 'SCY', timeSeconds: 50.23, achievedAt: '2026-02-14', meetName: 'Winter Invitational', source: 'import', isOverride: false },
      { id: 'bt2', event: '200 Free', course: 'SCY', timeSeconds: 110.45, achievedAt: '2026-01-20', meetName: 'Club Championships', source: 'import', isOverride: false },
      { id: 'bt3', event: '100 Back', course: 'SCY', timeSeconds: 58.99, achievedAt: '2025-11-05', meetName: 'Fall Classic', source: 'import', isOverride: false },
      { id: 'bt4', event: '200 IM',   course: 'SCY', timeSeconds: 118.32, achievedAt: '2026-02-14', meetName: 'Winter Invitational', source: 'import', isOverride: false },
      { id: 'bt5', event: '100 Free', course: 'LCM', timeSeconds: 55.10, achievedAt: '2025-07-12', meetName: 'Summer Nationals Qualifier', source: 'import', isOverride: false },
    ],
  },
  {
    id: 'sw2', name: 'Sofia Reyes', age: 12, gender: 'F', group: 'Junior A', ussNumber: '2014030200142B',
    bestTimes: [
      { id: 'bt6', event: '100 Fly',  course: 'SCY', timeSeconds: 65.88, achievedAt: '2026-01-20', meetName: 'Club Championships', source: 'import', isOverride: false },
      { id: 'bt7', event: '200 Fly',  course: 'SCY', timeSeconds: 143.21, achievedAt: '2025-12-10', meetName: 'Holiday Classic', source: 'import', isOverride: false },
      { id: 'bt8', event: '100 Free', course: 'SCY', timeSeconds: 61.34, achievedAt: '2026-02-14', meetName: 'Winter Invitational', source: 'import', isOverride: false },
      { id: 'bt9', event: '100 Fly',  course: 'SCY', timeSeconds: 64.50, achievedAt: '2026-01-05', meetName: 'Manual entry', source: 'manual', isOverride: true },
    ],
  },
  {
    id: 'sw3', name: 'James Okonkwo', age: 16, gender: 'M', group: 'Senior', ussNumber: '2010050300283C',
    bestTimes: [
      { id: 'bt10', event: '200 Breast', course: 'SCY', timeSeconds: 133.67, achievedAt: '2026-02-14', meetName: 'Winter Invitational', source: 'import', isOverride: false },
      { id: 'bt11', event: '100 Breast', course: 'SCY', timeSeconds: 61.22, achievedAt: '2026-01-20', meetName: 'Club Championships', source: 'import', isOverride: false },
      { id: 'bt12', event: '200 IM',     course: 'SCY', timeSeconds: 112.89, achievedAt: '2026-02-14', meetName: 'Winter Invitational', source: 'import', isOverride: false },
    ],
  },
]

const ALL_COURSES: Course[] = ['SCY', 'SCM', 'LCM']

export default function BestTimesPage() {
  const [searchSwimmer, setSearchSwimmer] = useState('')
  const [selectedSwimmer, setSelectedSwimmer] = useState<Swimmer | null>(null)
  const [course, setCourse] = useState<Course>('SCY')
  const [showOverrideModal, setShowOverrideModal] = useState(false)
  const [overrideTarget, setOverrideTarget] = useState<BestTime | null>(null)
  const [overrideTime, setOverrideTime] = useState('')
  const [overrideReason, setOverrideReason] = useState('')

  const filteredSwimmers = MOCK_SWIMMERS.filter((s) =>
    !searchSwimmer || s.name.toLowerCase().includes(searchSwimmer.toLowerCase()) ||
    s.ussNumber.toLowerCase().includes(searchSwimmer.toLowerCase())
  )

  const courseTimes = selectedSwimmer
    ? selectedSwimmer.bestTimes.filter((t) => t.course === course)
    : []

  function handleOverride(bt: BestTime) {
    setOverrideTarget(bt)
    setOverrideTime(fmtTime(bt.timeSeconds))
    setOverrideReason('')
    setShowOverrideModal(true)
  }

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1200px' }}>
      {/* Header */}
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Performance
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em' }}>
          Best Times Lookup
        </h1>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: '20px', alignItems: 'start' }}>
        {/* Swimmer list panel */}
        <div className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '14px 16px', borderBottom: '1px solid var(--border)' }}>
            <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '8px' }}>
              Select Swimmer
            </p>
            <div style={{ position: 'relative' }}>
              <Icon name="search" size={12} style={{ position: 'absolute', left: '9px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
              <input
                placeholder="Search name or USS#…"
                value={searchSwimmer}
                onChange={(e) => setSearchSwimmer(e.target.value)}
                style={{
                  width: '100%', paddingLeft: '28px', paddingRight: '8px', height: '32px',
                  background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '5px',
                  color: 'var(--text)', fontSize: '12px', fontFamily: 'var(--font-body)', outline: 'none',
                }}
              />
            </div>
          </div>
          <div style={{ maxHeight: '500px', overflowY: 'auto' }}>
            {filteredSwimmers.map((s) => (
              <button
                key={s.id}
                onClick={() => setSelectedSwimmer(s)}
                style={{
                  width: '100%', padding: '12px 16px', textAlign: 'left', background: 'none',
                  border: 'none', borderBottom: '1px solid var(--border)', cursor: 'pointer',
                  backgroundColor: selectedSwimmer?.id === s.id ? 'var(--accent-subtle)' : 'transparent',
                  transition: 'background-color 0.1s ease',
                }}
                onMouseEnter={(e) => { if (selectedSwimmer?.id !== s.id) e.currentTarget.style.backgroundColor = 'var(--bg-surface)' }}
                onMouseLeave={(e) => { if (selectedSwimmer?.id !== s.id) e.currentTarget.style.backgroundColor = 'transparent' }}
              >
                <div style={{ fontSize: '13px', fontWeight: 500, color: selectedSwimmer?.id === s.id ? 'var(--accent)' : 'var(--text)', marginBottom: '3px' }}>
                  {s.name}
                </div>
                <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                  {s.group} · {s.gender === 'M' ? '♂' : '♀'} {s.age}yo · {s.bestTimes.length} times
                </div>
              </button>
            ))}
          </div>
        </div>

        {/* Times panel */}
        {selectedSwimmer ? (
          <div>
            {/* Swimmer header */}
            <div className="card-elevated" style={{ padding: '20px 24px', marginBottom: '16px' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '8px' }}>
                <div>
                  <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '4px' }}>
                    {selectedSwimmer.name}
                  </h2>
                  <p style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                    USS# {selectedSwimmer.ussNumber} · {selectedSwimmer.group} · {selectedSwimmer.gender === 'M' ? 'Male' : 'Female'}, Age {selectedSwimmer.age}
                  </p>
                </div>
                <span style={{ fontSize: '22px', fontFamily: 'var(--font-display)', color: 'var(--text)' }}>
                  {selectedSwimmer.bestTimes.length} times recorded
                </span>
              </div>
            </div>

            {/* Course tabs */}
            <div style={{ display: 'flex', gap: '4px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', padding: '3px', marginBottom: '16px', width: 'fit-content' }}>
              {ALL_COURSES.map((c) => {
                const count = selectedSwimmer.bestTimes.filter((t) => t.course === c).length
                return (
                  <button
                    key={c}
                    onClick={() => setCourse(c)}
                    style={{
                      padding: '5px 16px', borderRadius: '5px', border: 'none', cursor: 'pointer',
                      background: course === c ? 'var(--bg-elevated)' : 'transparent',
                      color: course === c ? 'var(--text)' : 'var(--text-muted)',
                      fontSize: '12px', fontWeight: course === c ? 600 : 400, fontFamily: 'var(--font-body)',
                      display: 'flex', alignItems: 'center', gap: '6px',
                      boxShadow: course === c ? 'var(--shadow-xs)' : 'none',
                      transition: 'all 0.15s ease',
                    }}
                  >
                    {c}
                    <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: course === c ? 'var(--text-muted)' : 'var(--text-disabled)' }}>
                      {count}
                    </span>
                  </button>
                )
              })}
            </div>

            {/* Times table */}
            <div className="card" style={{ overflow: 'hidden' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid var(--border)' }}>
                    {['Event', 'Best Time', 'Date', 'Meet', 'Source', ''].map((h) => (
                      <th key={h} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                        {h}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {courseTimes.length === 0 ? (
                    <tr>
                      <td colSpan={6} style={{ padding: '40px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>
                        No {course} times recorded for {selectedSwimmer.name}.
                      </td>
                    </tr>
                  ) : (
                    courseTimes.map((bt) => (
                      <tr
                        key={bt.id}
                        style={{ borderBottom: '1px solid var(--border)', transition: 'background 0.1s ease' }}
                        onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--bg-surface)' }}
                        onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                      >
                        <td style={{ padding: '11px 14px', fontSize: '13px', fontWeight: 500, color: 'var(--text)' }}>
                          {bt.event}
                        </td>
                        <td style={{ padding: '11px 14px' }}>
                          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '15px', fontWeight: 700, color: 'var(--text)' }}>
                            {fmtTime(bt.timeSeconds)}
                          </span>
                          {bt.isOverride && (
                            <span style={{ marginLeft: '8px', fontSize: '10px', padding: '1px 6px', borderRadius: '20px', background: 'var(--warning-bg)', color: 'var(--warning)', border: '1px solid var(--warning-border)', fontFamily: 'var(--font-mono)', fontWeight: 600 }}>
                              OVERRIDE
                            </span>
                          )}
                        </td>
                        <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>
                          {bt.achievedAt}
                        </td>
                        <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>
                          {bt.meetName}
                        </td>
                        <td style={{ padding: '11px 14px' }}>
                          <span style={{
                            fontSize: '10px', padding: '2px 7px', borderRadius: '20px', fontFamily: 'var(--font-mono)', fontWeight: 600,
                            background: bt.source === 'manual' ? 'var(--warning-bg)' : 'var(--bg-elevated)',
                            color: bt.source === 'manual' ? 'var(--warning)' : 'var(--text-muted)',
                            border: `1px solid ${bt.source === 'manual' ? 'var(--warning-border)' : 'var(--border)'}`,
                          }}>
                            {bt.source === 'manual' ? 'MANUAL' : 'IMPORT'}
                          </span>
                        </td>
                        <td style={{ padding: '11px 14px', textAlign: 'right' }}>
                          <button
                            onClick={() => handleOverride(bt)}
                            style={{
                              padding: '4px 10px', background: 'var(--bg-elevated)', border: '1px solid var(--border)',
                              borderRadius: '5px', color: 'var(--text-muted)', fontSize: '11px', cursor: 'pointer',
                              fontFamily: 'var(--font-body)', transition: 'all 0.15s ease',
                              display: 'inline-flex', alignItems: 'center', gap: '5px',
                            }}
                            onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent-border)'; e.currentTarget.style.color = 'var(--accent)' }}
                            onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-muted)' }}
                          >
                            <Icon name="edit" size={11} />
                            Override
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        ) : (
          <div className="card" style={{ padding: '64px', textAlign: 'center' }}>
            <div style={{ width: '48px', height: '48px', borderRadius: '12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 16px' }}>
              <Icon name="users" size={20} style={{ color: 'var(--text-muted)' }} />
            </div>
            <p style={{ fontSize: '14px', color: 'var(--text-subtle)', marginBottom: '6px' }}>Select a swimmer</p>
            <p style={{ fontSize: '12px', color: 'var(--text-muted)' }}>Choose a swimmer from the panel to view their best times.</p>
          </div>
        )}
      </div>

      {/* Override modal */}
      {showOverrideModal && overrideTarget && selectedSwimmer && (
        <div
          style={{ position: 'fixed', inset: 0, background: 'var(--bg-overlay)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
          onClick={(e) => { if (e.target === e.currentTarget) setShowOverrideModal(false) }}
        >
          <div style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '14px', padding: '32px', width: '440px', boxShadow: 'var(--shadow-lg)' }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '6px' }}>
              Manual Override
            </h3>
            <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '20px' }}>
              Overriding {overrideTarget.event} ({overrideTarget.course}) for {selectedSwimmer.name}.
              This action will be audit-logged with your identity.
            </p>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '14px', marginBottom: '20px' }}>
              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '6px' }}>New Time</label>
                <input
                  value={overrideTime}
                  onChange={(e) => setOverrideTime(e.target.value)}
                  placeholder="e.g. 56.23 or 1:02.45"
                  style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text)', fontSize: '14px', fontFamily: 'var(--font-mono)', outline: 'none' }}
                />
              </div>
              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '6px' }}>Reason (required)</label>
                <textarea
                  value={overrideReason}
                  onChange={(e) => setOverrideReason(e.target.value)}
                  placeholder="Explain why this time is being manually overridden…"
                  rows={3}
                  style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none', resize: 'vertical' }}
                />
              </div>
            </div>

            <div style={{ padding: '10px 12px', background: 'var(--warning-bg)', border: '1px solid var(--warning-border)', borderRadius: '7px', fontSize: '12px', color: 'var(--warning)', marginBottom: '20px' }}>
              ⚠ This override will replace the current best time and be recorded in the audit log.
            </div>

            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
              <button onClick={() => setShowOverrideModal(false)} style={{ padding: '8px 18px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Cancel</button>
              <button
                onClick={() => setShowOverrideModal(false)}
                disabled={!overrideTime || !overrideReason}
                style={{ padding: '8px 18px', background: overrideTime && overrideReason ? 'var(--accent)' : 'var(--bg-elevated)', border: `1px solid ${overrideTime && overrideReason ? 'transparent' : 'var(--border)'}`, borderRadius: '7px', color: overrideTime && overrideReason ? '#fff' : 'var(--text-muted)', fontSize: '13px', fontWeight: 600, cursor: overrideTime && overrideReason ? 'pointer' : 'not-allowed', fontFamily: 'var(--font-body)' }}
              >
                Apply Override
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
