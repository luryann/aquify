'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

type Course = 'SCY' | 'SCM' | 'LCM'
type Standard = {
  id: string
  name: string
  course: Course
  season: string
  ageMin: number | null
  ageMax: number | null
  gender: 'M' | 'F' | 'X'
  event: string
  timeSeconds: number
  cutType: 'A' | 'B' | 'BB' | 'AA' | 'AAA' | 'AAAA' | 'Nationals' | 'Custom'
  active: boolean
  meetId: string | null
}

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const MOCK_STANDARDS: Standard[] = [
  { id: 's1',  name: '2025-26 A Cut',    course: 'SCY', season: '2025-26', ageMin: 10, ageMax: 12, gender: 'M', event: '100 Free',    timeSeconds: 56.29,  cutType: 'A',      active: true,  meetId: null },
  { id: 's2',  name: '2025-26 B Cut',    course: 'SCY', season: '2025-26', ageMin: 10, ageMax: 12, gender: 'M', event: '100 Free',    timeSeconds: 60.99,  cutType: 'B',      active: true,  meetId: null },
  { id: 's3',  name: '2025-26 A Cut',    course: 'SCY', season: '2025-26', ageMin: 10, ageMax: 12, gender: 'F', event: '100 Free',    timeSeconds: 58.49,  cutType: 'A',      active: true,  meetId: null },
  { id: 's4',  name: '2025-26 A Cut',    course: 'SCY', season: '2025-26', ageMin: 13, ageMax: 14, gender: 'M', event: '200 IM',      timeSeconds: 122.39, cutType: 'AA',     active: true,  meetId: null },
  { id: 's5',  name: '2025-26 AA Cut',   course: 'SCY', season: '2025-26', ageMin: 13, ageMax: 14, gender: 'F', event: '200 Fly',     timeSeconds: 131.79, cutType: 'AA',     active: true,  meetId: null },
  { id: 's6',  name: '2025-26 AAA Cut',  course: 'SCY', season: '2025-26', ageMin: 15, ageMax: 18, gender: 'M', event: '400 IM',      timeSeconds: 264.89, cutType: 'AAA',    active: true,  meetId: null },
  { id: 's7',  name: 'Spring Invite',    course: 'SCM', season: '2025-26', ageMin: null, ageMax: null, gender: 'M', event: '50 Back', timeSeconds: 32.10,  cutType: 'Custom', active: true,  meetId: 'm1' },
  { id: 's8',  name: 'Spring Invite',    course: 'SCM', season: '2025-26', ageMin: null, ageMax: null, gender: 'F', event: '50 Back', timeSeconds: 35.60,  cutType: 'Custom', active: true,  meetId: 'm1' },
  { id: 's9',  name: '2024-25 A Cut',    course: 'LCM', season: '2024-25', ageMin: 11, ageMax: 12, gender: 'F', event: '100 Breast',  timeSeconds: 80.49,  cutType: 'A',      active: false, meetId: null },
  { id: 's10', name: '2024-25 BB Cut',   course: 'SCY', season: '2024-25', ageMin: 8,  ageMax: 10, gender: 'F', event: '50 Free',     timeSeconds: 33.99,  cutType: 'BB',     active: false, meetId: null },
]

function fmtTime(sec: number): string {
  if (sec < 60) return sec.toFixed(2)
  const m = Math.floor(sec / 60)
  const s = (sec % 60).toFixed(2).padStart(5, '0')
  return `${m}:${s}`
}

const COURSES: Course[] = ['SCY', 'SCM', 'LCM']

export default function TimeStandardsPage() {
  const [search, setSearch] = useState('')
  const [course, setCourse] = useState<Course | 'ALL'>('ALL')
  const [season, setSeason] = useState('ALL')
  const [showActive, setShowActive] = useState(true)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [showCreateModal, setShowCreateModal] = useState(false)

  const seasons = ['ALL', ...Array.from(new Set(MOCK_STANDARDS.map((s) => s.season)))]

  const filtered = MOCK_STANDARDS.filter((s) => {
    if (showActive && !s.active) return false
    if (course !== 'ALL' && s.course !== course) return false
    if (season !== 'ALL' && s.season !== season) return false
    if (search) {
      const q = search.toLowerCase()
      return s.name.toLowerCase().includes(q) || s.event.toLowerCase().includes(q) || s.cutType.toLowerCase().includes(q)
    }
    return true
  })

  function toggleSelect(id: string) {
    setSelectedIds((prev) => {
      const n = new Set(prev)
      if (n.has(id)) { n.delete(id) } else { n.add(id) }
      return n
    })
  }

  const cutColors: Record<string, string> = {
    B: '#60a5fa', BB: '#60a5fa', A: '#4ade80', AA: '#4ade80',
    AAA: '#fbbf24', AAAA: '#fbbf24', Nationals: 'var(--accent)', Custom: 'var(--text-subtle)',
  }

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1200px' }}>
      {/* Header */}
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Performance
        </p>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px', flexWrap: 'wrap' }}>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em' }}>
            Time Standards
          </h1>
          <button
            onClick={() => setShowCreateModal(true)}
            style={{
              padding: '8px 18px', background: 'var(--accent)', border: 'none', borderRadius: '7px',
              color: '#fff', fontSize: '13px', fontWeight: 600, cursor: 'pointer', fontFamily: 'var(--font-body)',
              display: 'flex', alignItems: 'center', gap: '7px', letterSpacing: '-0.01em',
            }}
            onMouseEnter={(e) => { e.currentTarget.style.background = '#d44f14' }}
            onMouseLeave={(e) => { e.currentTarget.style.background = 'var(--accent)' }}
          >
            <Icon name="plus" size={14} />
            Add Standard
          </button>
        </div>
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total Standards', value: MOCK_STANDARDS.length.toString() },
          { label: 'Active', value: MOCK_STANDARDS.filter(s => s.active).toString().length === 0 ? MOCK_STANDARDS.filter(s => s.active).length.toString() : MOCK_STANDARDS.filter(s => s.active).length.toString() },
          { label: 'Courses', value: '3 (SCY / SCM / LCM)' },
          { label: 'Seasons', value: Array.from(new Set(MOCK_STANDARDS.map(s => s.season))).length.toString() },
        ].map((stat) => (
          <div key={stat.label} className="card-elevated" style={{ padding: '14px 16px' }}>
            <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>
              {stat.label}
            </p>
            <p style={{ fontSize: '20px', fontFamily: 'var(--font-display)', color: 'var(--text)', lineHeight: 1 }}>
              {stat.value}
            </p>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '20px', flexWrap: 'wrap', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, minWidth: '200px', maxWidth: '300px' }}>
          <Icon name="search" size={13} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input
            placeholder="Search events, cuts…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{
              width: '100%', paddingLeft: '30px', paddingRight: '10px', height: '34px',
              background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '6px',
              color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none',
            }}
          />
        </div>

        {/* Course filter */}
        <div style={{ display: 'flex', gap: '4px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '6px', padding: '3px' }}>
          {(['ALL', ...COURSES] as const).map((c) => (
            <button key={c} onClick={() => setCourse(c)}
              style={{
                padding: '4px 12px', borderRadius: '4px', border: 'none', cursor: 'pointer',
                background: course === c ? 'var(--bg-elevated)' : 'transparent',
                color: course === c ? 'var(--text)' : 'var(--text-muted)',
                fontSize: '12px', fontWeight: course === c ? 600 : 400, fontFamily: 'var(--font-body)',
                boxShadow: course === c ? 'var(--shadow-xs)' : 'none',
                transition: 'all 0.15s ease',
              }}
            >{c}</button>
          ))}
        </div>

        {/* Season filter */}
        <select
          value={season}
          onChange={(e) => setSeason(e.target.value)}
          style={{
            height: '34px', padding: '0 10px', background: 'var(--bg-surface)',
            border: '1px solid var(--border)', borderRadius: '6px',
            color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none',
          }}
        >
          {seasons.map((s) => <option key={s} value={s}>{s === 'ALL' ? 'All Seasons' : s}</option>)}
        </select>

        {/* Active toggle */}
        <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer', fontSize: '13px', color: 'var(--text-subtle)' }}>
          <div
            onClick={() => setShowActive(!showActive)}
            style={{
              width: '36px', height: '20px', borderRadius: '10px',
              background: showActive ? 'var(--accent)' : 'var(--bg-elevated)',
              border: '1px solid ' + (showActive ? 'transparent' : 'var(--border)'),
              position: 'relative', cursor: 'pointer', transition: 'background 0.2s',
            }}
          >
            <div style={{
              position: 'absolute', top: '2px', left: showActive ? '18px' : '2px',
              width: '14px', height: '14px', borderRadius: '50%', background: '#fff',
              transition: 'left 0.2s', boxShadow: 'var(--shadow-xs)',
            }} />
          </div>
          Active only
        </label>

        {selectedIds.size > 0 && (
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center', marginLeft: 'auto' }}>
            <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
              {selectedIds.size} selected
            </span>
            <button style={{ padding: '5px 12px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '5px', color: 'var(--error)', fontSize: '12px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}>
              Delete
            </button>
          </div>
        )}
      </div>

      {/* Table */}
      <div className="card" style={{ overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ borderBottom: '1px solid var(--border)' }}>
              <th style={{ width: '36px', padding: '10px 12px' }}>
                <input type="checkbox"
                  checked={selectedIds.size === filtered.length && filtered.length > 0}
                  onChange={() => {
                    if (selectedIds.size === filtered.length) setSelectedIds(new Set())
                    else setSelectedIds(new Set(filtered.map(s => s.id)))
                  }}
                  style={{ cursor: 'pointer' }}
                />
              </th>
              {['Event', 'Cut', 'Course', 'Age Group', 'Gender', 'Standard Time', 'Season', 'Status', ''].map((h) => (
                <th key={h} style={{ padding: '10px 12px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', whiteSpace: 'nowrap' }}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 ? (
              <tr>
                <td colSpan={9} style={{ padding: '48px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>
                  No standards match the current filters.
                </td>
              </tr>
            ) : (
              filtered.map((s) => (
                <tr
                  key={s.id}
                  style={{
                    borderBottom: '1px solid var(--border)',
                    background: selectedIds.has(s.id) ? 'var(--accent-subtle)' : 'transparent',
                    transition: 'background 0.1s ease',
                  }}
                  onMouseEnter={(e) => { if (!selectedIds.has(s.id)) e.currentTarget.style.background = 'var(--bg-surface)' }}
                  onMouseLeave={(e) => { if (!selectedIds.has(s.id)) e.currentTarget.style.background = 'transparent' }}
                >
                  <td style={{ padding: '10px 12px' }}>
                    <input type="checkbox" checked={selectedIds.has(s.id)}
                      onChange={() => toggleSelect(s.id)} style={{ cursor: 'pointer' }} />
                  </td>
                  <td style={{ padding: '10px 12px', fontSize: '13px', color: 'var(--text)', fontWeight: 500 }}>
                    {s.event}
                  </td>
                  <td style={{ padding: '10px 12px' }}>
                    <span style={{
                      fontSize: '11px', fontWeight: 700, padding: '2px 8px', borderRadius: '20px',
                      background: 'rgba(0,0,0,0.2)', color: cutColors[s.cutType] ?? 'var(--text-subtle)',
                      fontFamily: 'var(--font-mono)', letterSpacing: '0.04em',
                    }}>
                      {s.cutType}
                    </span>
                  </td>
                  <td style={{ padding: '10px 12px' }}>
                    <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', padding: '2px 7px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '4px', color: 'var(--text-subtle)' }}>
                      {s.course}
                    </span>
                  </td>
                  <td style={{ padding: '10px 12px', fontSize: '13px', color: 'var(--text-subtle)' }}>
                    {s.ageMin && s.ageMax ? `${s.ageMin}–${s.ageMax}` : 'Open'}
                  </td>
                  <td style={{ padding: '10px 12px', fontSize: '13px', color: 'var(--text-subtle)' }}>
                    {s.gender === 'M' ? 'Male' : s.gender === 'F' ? 'Female' : 'Mixed'}
                  </td>
                  <td style={{ padding: '10px 12px', fontFamily: 'var(--font-mono)', fontSize: '14px', fontWeight: 600, color: 'var(--text)' }}>
                    {fmtTime(s.timeSeconds)}
                  </td>
                  <td style={{ padding: '10px 12px', fontSize: '12px', color: 'var(--text-muted)' }}>
                    {s.season}
                  </td>
                  <td style={{ padding: '10px 12px' }}>
                    <span style={{
                      fontSize: '10px', padding: '2px 7px', borderRadius: '20px',
                      background: s.active ? 'var(--success-bg)' : 'var(--bg-elevated)',
                      color: s.active ? 'var(--success)' : 'var(--text-muted)',
                      border: `1px solid ${s.active ? 'var(--success-border)' : 'var(--border)'}`,
                      fontFamily: 'var(--font-mono)', fontWeight: 600, letterSpacing: '0.04em',
                    }}>
                      {s.active ? 'ACTIVE' : 'ARCHIVED'}
                    </span>
                  </td>
                  <td style={{ padding: '10px 12px', textAlign: 'right' }}>
                    <button style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: '4px', borderRadius: '4px', transition: 'color 0.15s' }}
                      onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--text)' }}
                      onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--text-muted)' }}>
                      <Icon name="edit" size={13} />
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Count */}
      <p style={{ marginTop: '12px', fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
        Showing {filtered.length} of {MOCK_STANDARDS.length} standards
      </p>

      {/* Create modal stub */}
      {showCreateModal && (
        <div
          style={{
            position: 'fixed', inset: 0, background: 'var(--bg-overlay)', zIndex: 1000,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}
          onClick={(e) => { if (e.target === e.currentTarget) setShowCreateModal(false) }}
        >
          <div style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '14px', padding: '32px', width: '480px', boxShadow: 'var(--shadow-lg)' }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '20px' }}>
              Add Time Standard
            </h3>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '14px', marginBottom: '20px' }}>
              {[['Event', 'e.g. 100 Free'], ['Standard Time', 'e.g. 56.29'], ['Season', 'e.g. 2025-26'], ['Cut Type', 'A / B / AA…']].map(([lbl, ph]) => (
                <div key={lbl}>
                  <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '6px' }}>{lbl}</label>
                  <input placeholder={ph} style={{ width: '100%', padding: '8px 10px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '6px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }} />
                </div>
              ))}
            </div>
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
              <button onClick={() => setShowCreateModal(false)} style={{ padding: '8px 18px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Cancel</button>
              <button onClick={() => setShowCreateModal(false)} style={{ padding: '8px 18px', background: 'var(--accent)', border: 'none', borderRadius: '7px', color: '#fff', fontSize: '13px', fontWeight: 600, cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Save Standard</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
