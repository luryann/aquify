'use client'

import { useState, type ChangeEvent } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'

// ─── Types ────────────────────────────────────────────────────────────────────

type MeetStatus = 'draft' | 'published' | 'commitment_open' | 'commitment_closed' | 'entry_finalized' | 'completed' | 'archived'

type Session = {
  id: string; name: string; date: string; warmupTime: string; startTime: string
  eventCount: number; section: string; notes?: string
}
type Event = {
  id: string; eventNumber: number; name: string; course: string
  gender: 'M' | 'F' | 'Mixed'; ageGroup: string; entries: number
  seedTime: string | null; sessionId: string
}
type Commitment = {
  id: string; swimmer: string; swimmerId: string; household: string
  status: 'invited' | 'committed_yes' | 'committed_no' | 'locked'
  eventSelections: number; respondedAt: string | null
}
type Entry = {
  id: string; swimmer: string; swimmerId: string; event: string
  seedTime: string; status: 'draft' | 'submitted' | 'approved' | 'exported'
  override: boolean; overrideReason?: string; entryTime?: string
}
type MeetAttachment = {
  id: string; name: string; type: 'pdf' | 'image' | 'file'
  uploadedAt: string; uploadedBy: string; size: string; url: string
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_MEET = {
  id: 'meet-001',
  name: 'Winter Classic',
  host: 'DARE Aquatics',
  location: 'Portland Aquatics Center, 1401 N Hayden Island Dr',
  startDate: '2026-03-25',
  endDate: '2026-03-26',
  status: 'commitment_open' as MeetStatus,
  course: 'SCY',
  entryDeadline: '2026-03-18',
  description: 'Annual Winter Classic hosted by DARE Aquatics. Open to all club swimmers. Standard USA Swimming sanctioned meet. Psych sheets and heat sheets will be posted 48 hours before the meet.\n\nWarm-up procedures will be assigned by lane. Please arrive 15 minutes before your designated warm-up time. All swimmers must check in with their coach before warming up.',
  sanctionNumber: 'OR-2026-WC-001',
  swimmerSelfSignup: true,
  maxEntriesPerSwimmer: 5,
  entryFeePerEvent: 8.00,
}

const MOCK_SESSIONS: Session[] = [
  { id: 'ses-001', name: 'Morning Session', date: '2026-03-25', warmupTime: '07:30', startTime: '09:00', eventCount: 14, section: 'Day 1 — Saturday', notes: 'Warm-up Lane 1-4. Check in with coach at team area.' },
  { id: 'ses-002', name: 'Afternoon Session', date: '2026-03-25', warmupTime: '13:00', startTime: '14:00', eventCount: 12, section: 'Day 1 — Saturday', notes: 'Finals session. Top 8 from morning advance.' },
  { id: 'ses-003', name: 'Morning Session', date: '2026-03-26', warmupTime: '07:30', startTime: '09:00', eventCount: 14, section: 'Day 2 — Sunday', notes: 'Distance events first. All ages.' },
  { id: 'ses-004', name: 'Finals', date: '2026-03-26', warmupTime: '17:00', startTime: '18:00', eventCount: 8, section: 'Day 2 — Sunday', notes: 'Top 8 finalists per event. Coaches must submit scratches by 4:30 PM.' },
]

const MOCK_EVENTS: Event[] = [
  { id: 'ev-001', eventNumber: 1,  name: '200 IM',     course: 'SCY', gender: 'F', ageGroup: '11-12', entries: 0, seedTime: null, sessionId: 'ses-001' },
  { id: 'ev-002', eventNumber: 2,  name: '200 IM',     course: 'SCY', gender: 'M', ageGroup: '11-12', entries: 0, seedTime: null, sessionId: 'ses-001' },
  { id: 'ev-003', eventNumber: 3,  name: '100 Fly',    course: 'SCY', gender: 'F', ageGroup: '13-14', entries: 0, seedTime: null, sessionId: 'ses-001' },
  { id: 'ev-004', eventNumber: 4,  name: '100 Fly',    course: 'SCY', gender: 'M', ageGroup: '13-14', entries: 0, seedTime: null, sessionId: 'ses-001' },
  { id: 'ev-005', eventNumber: 5,  name: '200 Free',   course: 'SCY', gender: 'F', ageGroup: 'Open',  entries: 3, seedTime: null, sessionId: 'ses-001' },
  { id: 'ev-006', eventNumber: 6,  name: '200 Free',   course: 'SCY', gender: 'M', ageGroup: 'Open',  entries: 2, seedTime: null, sessionId: 'ses-001' },
  { id: 'ev-007', eventNumber: 7,  name: '100 Back',   course: 'SCY', gender: 'F', ageGroup: '10&U',  entries: 0, seedTime: null, sessionId: 'ses-001' },
  { id: 'ev-008', eventNumber: 8,  name: '100 Back',   course: 'SCY', gender: 'M', ageGroup: '10&U',  entries: 0, seedTime: null, sessionId: 'ses-001' },
  { id: 'ev-009', eventNumber: 9,  name: '100 Breast', course: 'SCY', gender: 'F', ageGroup: 'Open',  entries: 0, seedTime: null, sessionId: 'ses-002' },
  { id: 'ev-010', eventNumber: 10, name: '100 Breast', course: 'SCY', gender: 'M', ageGroup: 'Open',  entries: 0, seedTime: null, sessionId: 'ses-002' },
  { id: 'ev-011', eventNumber: 11, name: '100 Free',   course: 'SCY', gender: 'F', ageGroup: 'Open',  entries: 4, seedTime: null, sessionId: 'ses-002' },
  { id: 'ev-012', eventNumber: 12, name: '100 Free',   course: 'SCY', gender: 'M', ageGroup: 'Open',  entries: 3, seedTime: null, sessionId: 'ses-002' },
  { id: 'ev-013', eventNumber: 13, name: '400 IM',     course: 'SCY', gender: 'F', ageGroup: 'Open',  entries: 2, seedTime: null, sessionId: 'ses-003' },
  { id: 'ev-014', eventNumber: 14, name: '400 IM',     course: 'SCY', gender: 'M', ageGroup: 'Open',  entries: 1, seedTime: null, sessionId: 'ses-003' },
  { id: 'ev-015', eventNumber: 15, name: '200 Back',   course: 'SCY', gender: 'F', ageGroup: 'Open',  entries: 0, seedTime: null, sessionId: 'ses-003' },
  { id: 'ev-016', eventNumber: 16, name: '200 Back',   course: 'SCY', gender: 'M', ageGroup: 'Open',  entries: 0, seedTime: null, sessionId: 'ses-003' },
  { id: 'ev-017', eventNumber: 17, name: '50 Free',    course: 'SCY', gender: 'F', ageGroup: 'Open',  entries: 5, seedTime: null, sessionId: 'ses-004' },
  { id: 'ev-018', eventNumber: 18, name: '50 Free',    course: 'SCY', gender: 'M', ageGroup: 'Open',  entries: 4, seedTime: null, sessionId: 'ses-004' },
]

const MOCK_COMMITMENTS: Commitment[] = [
  { id: 'com-001', swimmer: 'Caleb Henderson',  swimmerId: 'sw-001', household: 'Henderson Family',  status: 'committed_yes', eventSelections: 3, respondedAt: '2026-03-08' },
  { id: 'com-002', swimmer: 'Mia Henderson',    swimmerId: 'sw-002', household: 'Henderson Family',  status: 'committed_yes', eventSelections: 2, respondedAt: '2026-03-08' },
  { id: 'com-003', swimmer: 'Owen Henderson',   swimmerId: 'sw-003', household: 'Henderson Family',  status: 'committed_no',  eventSelections: 0, respondedAt: '2026-03-08' },
  { id: 'com-004', swimmer: 'Jenna Park',       swimmerId: 'sw-004', household: 'Park Family',        status: 'committed_yes', eventSelections: 4, respondedAt: '2026-03-07' },
  { id: 'com-005', swimmer: 'Lucas Park',       swimmerId: 'sw-005', household: 'Park Family',        status: 'invited',       eventSelections: 0, respondedAt: null },
  { id: 'com-006', swimmer: 'Sofia Torres',     swimmerId: 'sw-006', household: 'Torres Household',   status: 'committed_yes', eventSelections: 5, respondedAt: '2026-03-06' },
  { id: 'com-007', swimmer: 'Ade Okafor',       swimmerId: 'sw-009', household: 'Okafor Household',   status: 'committed_yes', eventSelections: 4, respondedAt: '2026-03-09' },
  { id: 'com-008', swimmer: 'Tolu Okafor',      swimmerId: 'sw-010', household: 'Okafor Household',   status: 'invited',       eventSelections: 0, respondedAt: null },
  { id: 'com-009', swimmer: 'Amara Okafor',     swimmerId: 'sw-011', household: 'Okafor Household',   status: 'committed_yes', eventSelections: 2, respondedAt: '2026-03-09' },
]

const MOCK_ENTRIES: Entry[] = [
  { id: 'ent-001', swimmer: 'Sofia Torres',   swimmerId: 'sw-006', event: '200 Free (F Open)',  seedTime: '2:02.18', status: 'submitted', override: false },
  { id: 'ent-002', swimmer: 'Sofia Torres',   swimmerId: 'sw-006', event: '100 Free (F Open)',  seedTime: '56.40',   status: 'approved',  override: false },
  { id: 'ent-003', swimmer: 'Jenna Park',     swimmerId: 'sw-004', event: '200 IM (F 13-14)',   seedTime: '2:18.44', status: 'submitted', override: false },
  { id: 'ent-004', swimmer: 'Jenna Park',     swimmerId: 'sw-004', event: '100 Fly (F 13-14)',  seedTime: '1:04.88', status: 'approved',  override: false },
  { id: 'ent-005', swimmer: 'Caleb Henderson',swimmerId: 'sw-001', event: '100 Free (M Open)',  seedTime: '52.84',   status: 'draft',     override: false },
  { id: 'ent-006', swimmer: 'Ade Okafor',     swimmerId: 'sw-009', event: '100 Back (M Open)',  seedTime: '56.12',   status: 'submitted', override: true, overrideReason: 'Coach override – time from non-SDIF meet' },
  { id: 'ent-007', swimmer: 'Amara Okafor',   swimmerId: 'sw-011', event: '100 Free (F Open)',  seedTime: '1:01.33', status: 'draft',     override: false },
  { id: 'ent-008', swimmer: 'Mia Henderson',  swimmerId: 'sw-002', event: '50 Free (F Open)',   seedTime: '28.77',   status: 'approved',  override: false },
]

const MOCK_ATTACHMENTS: MeetAttachment[] = [
  { id: 'att-001', name: 'Meet Packet — Winter Classic 2026.pdf', type: 'pdf', uploadedAt: '2026-03-10', uploadedBy: 'Coach Rivera', size: '2.4 MB', url: '#' },
  { id: 'att-002', name: 'Venue Map.pdf', type: 'pdf', uploadedAt: '2026-03-10', uploadedBy: 'Coach Rivera', size: '0.8 MB', url: '#' },
  { id: 'att-003', name: 'Pool Diagram.png', type: 'image', uploadedAt: '2026-03-12', uploadedBy: 'Coach Rivera', size: '340 KB', url: '#' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

const STATUS_CFG: Record<MeetStatus, { label: string; color: string; bg: string }> = {
  draft:             { label: 'Draft',             color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  published:         { label: 'Published',         color: 'var(--info)',       bg: 'var(--info-bg)' },
  commitment_open:   { label: 'Commitments Open',  color: 'var(--warning)',    bg: 'var(--warning-bg)' },
  commitment_closed: { label: 'Commitments Closed',color: 'var(--warning)',    bg: 'var(--warning-bg)' },
  entry_finalized:   { label: 'Entries Finalized', color: 'var(--accent)',     bg: 'var(--accent-subtle)' },
  completed:         { label: 'Completed',         color: 'var(--success)',    bg: 'var(--success-bg)' },
  archived:          { label: 'Archived',          color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

const COMMITMENT_STATUS_CFG: Record<Commitment['status'], { label: string; color: string; bg: string }> = {
  invited:       { label: 'Awaiting',    color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  committed_yes: { label: 'Going',       color: 'var(--success)',    bg: 'var(--success-bg)' },
  committed_no:  { label: 'Not Going',   color: 'var(--error)',      bg: 'var(--error-bg)' },
  locked:        { label: 'Locked',      color: 'var(--accent)',     bg: 'var(--accent-subtle)' },
}

const ENTRY_STATUS_COLORS: Record<Entry['status'], { color: string; bg: string }> = {
  draft:     { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  submitted: { color: 'var(--info)',       bg: 'var(--info-bg)' },
  approved:  { color: 'var(--success)',    bg: 'var(--success-bg)' },
  exported:  { color: 'var(--accent)',     bg: 'var(--accent-subtle)' },
}

function groupBy<T>(arr: T[], key: (item: T) => string): Record<string, T[]> {
  return arr.reduce((acc, item) => {
    const k = key(item)
    if (!acc[k]) acc[k] = []
    acc[k].push(item)
    return acc
  }, {} as Record<string, T[]>)
}

// ─── Sessions Tab with Day Sections ──────────────────────────────────────────

function SessionsTab() {
  const [sessions, setSessions] = useState<Session[]>(MOCK_SESSIONS)
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(['Day 1 — Saturday', 'Day 2 — Sunday']))
  const [showAddSession, setShowAddSession] = useState(false)
  const [sessionDraft, setSessionDraft] = useState({
    name: '',
    date: MOCK_MEET.startDate,
    warmupTime: '',
    startTime: '',
    section: 'Day 1 — Saturday',
    notes: '',
  })
  const sessionsBySection = groupBy(sessions, (s) => s.section)
  const canSaveSession = Boolean(
    sessionDraft.name.trim() &&
    sessionDraft.date &&
    sessionDraft.warmupTime &&
    sessionDraft.startTime &&
    sessionDraft.section.trim(),
  )

  function resetSessionDraft() {
    setSessionDraft({
      name: '',
      date: MOCK_MEET.startDate,
      warmupTime: '',
      startTime: '',
      section: 'Day 1 — Saturday',
      notes: '',
    })
  }

  function handleAddSession() {
    if (!canSaveSession) return
    setSessions((prev) => [
      ...prev,
      {
        id: `ses-${Date.now()}`,
        name: sessionDraft.name.trim(),
        date: sessionDraft.date,
        warmupTime: sessionDraft.warmupTime,
        startTime: sessionDraft.startTime,
        eventCount: 0,
        section: sessionDraft.section.trim(),
        notes: sessionDraft.notes.trim() || undefined,
      },
    ])
    setExpandedSections((prev) => new Set(prev).add(sessionDraft.section.trim()))
    resetSessionDraft()
    setShowAddSession(false)
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{sessions.length} sessions across {Object.keys(sessionsBySection).length} days</p>
        <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setShowAddSession(true)}>
          <Icon name="plus" size={13} /> Add Session
        </button>
      </div>

      {Object.entries(sessionsBySection).map(([section, sessions]) => {
        const isOpen = expandedSections.has(section)
        return (
          <div key={section} style={{ marginBottom: '16px' }}>
            {/* Section header */}
            <button
              onClick={() => {
                const next = new Set(expandedSections)
                if (isOpen) { next.delete(section) } else { next.add(section) }
                setExpandedSections(next)
              }}
              style={{
                display: 'flex', alignItems: 'center', gap: '10px',
                width: '100%', padding: '10px 14px',
                background: 'var(--bg-surface)', border: '1px solid var(--border)',
                borderRadius: isOpen ? 'var(--r-md) var(--r-md) 0 0' : 'var(--r-md)',
                cursor: 'pointer', textAlign: 'left',
                transition: 'background var(--dur-fast)',
              }}
            >
              <Icon name={isOpen ? 'chevron-down' : 'chevron-right'} size={14} style={{ color: 'var(--text-muted)', flexShrink: 0 }} />
              <span style={{ fontWeight: 700, fontSize: '13px', color: 'var(--text)', fontFamily: 'var(--font-display)', letterSpacing: '0.01em' }}>{section}</span>
              <span style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>
                {sessions.length} session{sessions.length !== 1 ? 's' : ''} · {sessions.reduce((a, s) => a + s.eventCount, 0)} events
              </span>
            </button>

            {isOpen && (
              <div style={{ border: '1px solid var(--border)', borderTop: 'none', borderRadius: '0 0 var(--r-md) var(--r-md)', overflow: 'hidden' }}>
                {sessions.map((session, idx) => (
                  <div
                    key={session.id}
                    style={{
                      padding: '14px 16px',
                      background: idx % 2 === 0 ? 'var(--bg)' : 'var(--bg-surface)',
                      borderTop: idx > 0 ? '1px solid var(--border)' : 'none',
                      display: 'grid',
                      gridTemplateColumns: '1fr auto auto auto auto',
                      gap: '16px',
                      alignItems: 'center',
                    }}
                  >
                    <div>
                      <div style={{ fontWeight: 600, fontSize: '13px', color: 'var(--text)', marginBottom: '2px' }}>{session.name}</div>
                      {session.notes && (
                        <div style={{ fontSize: '11px', color: 'var(--text-muted)', lineHeight: 1.5 }}>{session.notes}</div>
                      )}
                    </div>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '2px' }}>Warm-up</div>
                      <div style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text-subtle)' }}>{session.warmupTime}</div>
                    </div>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '2px' }}>Start</div>
                      <div style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--text)' }}>{session.startTime}</div>
                    </div>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '2px' }}>Events</div>
                      <div style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text)' }}>{session.eventCount}</div>
                    </div>
                    <div style={{ display: 'flex', gap: '6px' }}>
                      <button className="btn btn-ghost" style={{ padding: '4px 8px', fontSize: '11px' }}>
                        <Icon name="edit-2" size={11} />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )
      })}

      {/* Add Session Modal */}
      {showAddSession && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <div className="card" style={{ padding: '24px', width: '480px', maxWidth: '90vw' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
              <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '18px', fontWeight: 600 }}>Add Session</h3>
              <button className="btn btn-ghost" onClick={() => setShowAddSession(false)}><Icon name="x" size={16} /></button>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div>
                <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>SESSION NAME</label>
                <input className="input" value={sessionDraft.name} onChange={(e) => setSessionDraft((prev) => ({ ...prev, name: e.target.value }))} placeholder="e.g. Day 1 — Morning" style={{ width: '100%' }} />
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: '10px' }}>
                <div>
                  <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>DATE</label>
                  <input className="input" type="date" value={sessionDraft.date} onChange={(e) => setSessionDraft((prev) => ({ ...prev, date: e.target.value }))} style={{ width: '100%' }} />
                </div>
                <div>
                  <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>WARM-UP</label>
                  <input className="input" type="time" value={sessionDraft.warmupTime} onChange={(e) => setSessionDraft((prev) => ({ ...prev, warmupTime: e.target.value }))} style={{ width: '100%' }} />
                </div>
                <div>
                  <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>START</label>
                  <input className="input" type="time" value={sessionDraft.startTime} onChange={(e) => setSessionDraft((prev) => ({ ...prev, startTime: e.target.value }))} style={{ width: '100%' }} />
                </div>
              </div>
              <div>
                <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>SECTION</label>
                <input className="input" value={sessionDraft.section} onChange={(e) => setSessionDraft((prev) => ({ ...prev, section: e.target.value }))} placeholder="e.g. Day 1 — Saturday" style={{ width: '100%' }} />
              </div>
              <div>
                <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>NOTES (optional)</label>
                <textarea className="input" value={sessionDraft.notes} onChange={(e) => setSessionDraft((prev) => ({ ...prev, notes: e.target.value }))} rows={3} placeholder="Warm-up lane assignments, special instructions..." style={{ width: '100%', resize: 'vertical' }} />
              </div>
            </div>
            <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end', marginTop: '20px' }}>
              <button className="btn btn-secondary" onClick={() => { resetSessionDraft(); setShowAddSession(false) }}>Cancel</button>
              <button className="btn btn-primary" onClick={handleAddSession} disabled={!canSaveSession}>Add Session</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Events Tab with Session Filter ──────────────────────────────────────────

function EventsTab() {
  const [sessionFilter, setSessionFilter] = useState<string>('all')
  const filtered = sessionFilter === 'all' ? MOCK_EVENTS : MOCK_EVENTS.filter((e) => e.sessionId === sessionFilter)

  const SESSION_COLS: Column<Event>[] = [
    { key: 'eventNumber', label: '#', align: 'right', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{r.eventNumber}</span> },
    { key: 'name', label: 'Event', render: (r) => <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{r.name}</span> },
    {
      key: 'gender', label: 'Gender',
      render: (r) => (
        <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: r.gender === 'F' ? 'var(--accent)' : r.gender === 'M' ? 'var(--info)' : 'var(--text-subtle)' }}>
          {r.gender === 'F' ? 'Female' : r.gender === 'M' ? 'Male' : 'Mixed'}
        </span>
      ),
    },
    { key: 'ageGroup', label: 'Age Group', render: (r) => <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', padding: '2px 7px', borderRadius: '8px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', color: 'var(--text-subtle)' }}>{r.ageGroup}</span> },
    { key: 'course', label: 'Course', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)' }}>{r.course}</span> },
    { key: 'entries', label: 'Entries', align: 'right', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: r.entries > 0 ? 'var(--success)' : 'var(--text-muted)' }}>{r.entries > 0 ? r.entries : '—'}</span> },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
        <div style={{ display: 'flex', gap: '6px', alignItems: 'center', flexWrap: 'wrap' }}>
          <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginRight: '4px' }}>Session:</span>
          {[{ id: 'all', label: 'All Sessions' }, ...MOCK_SESSIONS.map((s) => ({ id: s.id, label: s.name + ' (' + s.date + ')' }))].map((opt) => (
            <button
              key={opt.id}
              onClick={() => setSessionFilter(opt.id)}
              style={{
                fontSize: '11px', padding: '3px 10px', borderRadius: '20px', cursor: 'pointer',
                background: sessionFilter === opt.id ? 'var(--accent)' : 'var(--bg-elevated)',
                color: sessionFilter === opt.id ? '#fff' : 'var(--text-subtle)',
                border: `1px solid ${sessionFilter === opt.id ? 'var(--accent)' : 'var(--border)'}`,
                fontFamily: 'var(--font-mono)', transition: 'all var(--dur-fast)',
              }}
            >{opt.label}</button>
          ))}
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Icon name="upload-cloud" size={13} /> Import CL2
          </button>
          <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Icon name="plus" size={13} /> Add Event
          </button>
        </div>
      </div>
      <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '12px' }}>{filtered.length} events shown</p>
      <Table columns={SESSION_COLS} data={filtered} pageSize={20} emptyTitle="No events" emptyDescription="Add events or import a CL2 file to populate the event list." />
    </div>
  )
}

// ─── Entries Tab with Actions ─────────────────────────────────────────────────

function EntriesTab() {
  const [entries, setEntries] = useState<Entry[]>(MOCK_ENTRIES)
  const [editEntry, setEditEntry] = useState<Entry | null>(null)
  const [showAddEntry, setShowAddEntry] = useState(false)
  const [filterStatus, setFilterStatus] = useState<string>('all')
  const [filterSwimmer, setFilterSwimmer] = useState('')
  const [editSeedTime, setEditSeedTime] = useState('')
  const [editReason, setEditReason] = useState('')
  const [auditLog, setAuditLog] = useState<Array<{ id: string; message: string; at: string }>>([])
  const [newEntryDraft, setNewEntryDraft] = useState({
    swimmerId: '',
    eventId: '',
    seedTime: '',
    note: '',
  })

  const displayed = entries
    .filter((e) => filterStatus === 'all' || e.status === filterStatus)
    .filter((e) => !filterSwimmer || e.swimmer.toLowerCase().includes(filterSwimmer.toLowerCase()))

  function approveEntry(id: string) {
    setEntries((prev) => prev.map((e) => e.id === id ? { ...e, status: 'approved' as const } : e))
  }

  function approveAll() {
    setEntries((prev) => prev.map((e) => e.status === 'submitted' ? { ...e, status: 'approved' as const } : e))
  }

  function saveEdit() {
    if (!editEntry) return
    setEntries((prev) => prev.map((e) => e.id === editEntry.id ? { ...e, seedTime: editSeedTime || e.seedTime, override: true, overrideReason: editReason } : e))
    setAuditLog((prev) => [
      {
        id: `audit-${Date.now()}`,
        message: `Override saved for ${editEntry.swimmer} in ${editEntry.event}${editReason ? ` — ${editReason}` : ''}.`,
        at: new Date().toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }),
      },
      ...prev,
    ])
    setEditEntry(null)
    setEditSeedTime('')
    setEditReason('')
  }

  const availableSwimmers = MOCK_COMMITMENTS.filter((commitment) => commitment.status === 'committed_yes')
  const selectedSwimmer = availableSwimmers.find((commitment) => commitment.swimmerId === newEntryDraft.swimmerId)
  const selectedEvent = MOCK_EVENTS.find((event) => event.id === newEntryDraft.eventId)
  const canAddEntry = Boolean(selectedSwimmer && selectedEvent && newEntryDraft.seedTime.trim())

  function handleAddEntry() {
    if (!selectedSwimmer || !selectedEvent || !newEntryDraft.seedTime.trim()) return
    const newEntry: Entry = {
      id: `ent-${Date.now()}`,
      swimmer: selectedSwimmer.swimmer,
      swimmerId: selectedSwimmer.swimmerId,
      event: `${selectedEvent.name} (${selectedEvent.gender} ${selectedEvent.ageGroup})`,
      seedTime: newEntryDraft.seedTime.trim(),
      status: 'draft',
      override: true,
      overrideReason: newEntryDraft.note.trim() || 'Coach-created after commitment review',
    }

    setEntries((prev) => [newEntry, ...prev])
    setAuditLog((prev) => [
      {
        id: `audit-${Date.now() + 1}`,
        message: `Coach-created entry for ${newEntry.swimmer} in ${newEntry.event}${newEntryDraft.note ? ` — ${newEntryDraft.note}` : ''}.`,
        at: new Date().toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }),
      },
      ...prev,
    ])
    setNewEntryDraft({ swimmerId: '', eventId: '', seedTime: '', note: '' })
    setShowAddEntry(false)
  }

  const submittedCount = entries.filter((e) => e.status === 'submitted').length
  const approvedCount = entries.filter((e) => e.status === 'approved').length
  const overrideCount = entries.filter((e) => e.override).length

  return (
    <div>
      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Total Entries', value: entries.length, color: 'var(--text)' },
          { label: 'Submitted', value: submittedCount, color: 'var(--info)' },
          { label: 'Approved', value: approvedCount, color: 'var(--success)' },
          { label: 'Coach Override', value: overrideCount, color: 'var(--warning)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '12px 14px' }}>
            <div style={{ fontSize: '10px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '4px' }}>{s.label}</div>
            <div style={{ fontSize: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Filters + Actions */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '14px', gap: '12px', flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' }}>
          <input
            className="input"
            placeholder="Search swimmer..."
            value={filterSwimmer}
            onChange={(e) => setFilterSwimmer(e.target.value)}
            style={{ width: '180px', fontSize: '12px', padding: '5px 10px' }}
          />
          {['all', 'draft', 'submitted', 'approved', 'exported'].map((s) => (
            <button
              key={s}
              onClick={() => setFilterStatus(s)}
              style={{
                fontSize: '11px', padding: '3px 10px', borderRadius: '20px', cursor: 'pointer',
                background: filterStatus === s ? 'var(--accent)' : 'var(--bg-elevated)',
                color: filterStatus === s ? '#fff' : 'var(--text-subtle)',
                border: `1px solid ${filterStatus === s ? 'var(--accent)' : 'var(--border)'}`,
                fontFamily: 'var(--font-mono)', textTransform: 'capitalize', transition: 'all var(--dur-fast)',
              }}
            >{s === 'all' ? 'All' : s}</button>
          ))}
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setShowAddEntry(true)}>
            <Icon name="plus" size={13} /> Add Entry
          </button>
          <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Icon name="download-cloud" size={13} /> Export SD3
          </button>
          <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={approveAll}>
            <Icon name="check-circle" size={13} /> Approve All Submitted
          </button>
        </div>
      </div>

      {/* Entries list */}
      <div style={{ border: '1px solid var(--border)', borderRadius: 'var(--r-lg)', overflow: 'hidden' }}>
        {/* Table header */}
        <div style={{ display: 'grid', gridTemplateColumns: '1.5fr 1.5fr 100px 100px 80px 120px', padding: '9px 14px', borderBottom: '1px solid var(--border)', background: 'var(--bg-surface)' }}>
          {['Swimmer', 'Event', 'Seed Time', 'Status', 'Override', 'Actions'].map((h) => (
            <span key={h} style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', fontWeight: 600 }}>{h}</span>
          ))}
        </div>
        {displayed.length === 0 && (
          <div style={{ padding: '32px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>No entries match the current filter.</div>
        )}
        {displayed.map((entry, idx) => {
          const sc = ENTRY_STATUS_COLORS[entry.status]
          return (
            <div
              key={entry.id}
              style={{
                display: 'grid', gridTemplateColumns: '1.5fr 1.5fr 100px 100px 80px 120px',
                padding: '12px 14px', alignItems: 'center',
                background: idx % 2 === 0 ? 'var(--bg)' : 'var(--bg-surface)',
                borderTop: idx > 0 ? '1px solid var(--border)' : 'none',
                transition: 'background var(--dur-fast)',
              }}
            >
              <Link href={`/members/swimmers/${entry.swimmerId}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}
                onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
                onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
                {entry.swimmer}
              </Link>
              <span style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{entry.event}</span>
              <div>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--accent)' }}>{entry.seedTime}</span>
              </div>
              <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: sc.bg, color: sc.color, fontFamily: 'var(--font-mono)', textTransform: 'uppercase', width: 'fit-content' }}>{entry.status}</span>
              <div>
                {entry.override ? (
                  <span title={entry.overrideReason} style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', padding: '2px 6px', borderRadius: '6px', background: 'var(--warning-bg)', color: 'var(--warning)', cursor: 'help' }}>OVR</span>
                ) : <span style={{ color: 'var(--text-muted)', fontSize: '12px' }}>—</span>}
              </div>
              <div style={{ display: 'flex', gap: '4px' }}>
                {entry.status === 'submitted' && (
                  <button
                    className="btn btn-ghost"
                    onClick={() => approveEntry(entry.id)}
                    style={{ padding: '3px 8px', fontSize: '11px', color: 'var(--success)', border: '1px solid var(--success)', borderRadius: 'var(--r-sm)' }}
                  >
                    Approve
                  </button>
                )}
                <button
                  className="btn btn-ghost"
                  onClick={() => { setEditEntry(entry); setEditSeedTime(entry.seedTime); setEditReason(entry.overrideReason ?? '') }}
                  style={{ padding: '3px 8px', fontSize: '11px' }}
                >
                  <Icon name="edit-2" size={11} />
                </button>
              </div>
            </div>
          )
        })}
      </div>

      <div className="card" style={{ padding: '16px', marginTop: '16px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px' }}>
          <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>Override Audit Trail</h3>
          <span style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>{auditLog.length} actions</span>
        </div>
        {auditLog.length === 0 ? (
          <p style={{ fontSize: '12px', color: 'var(--text-muted)' }}>Override and coach-created entry actions will appear here.</p>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
            {auditLog.slice(0, 6).map((item) => (
              <div key={item.id} style={{ padding: '10px 12px', borderRadius: '8px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                <div style={{ fontSize: '12px', color: 'var(--text)' }}>{item.message}</div>
                <div style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '4px' }}>{item.at}</div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Edit Entry Modal */}
      {editEntry && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <div className="card" style={{ padding: '24px', width: '440px', maxWidth: '90vw' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
              <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '18px', fontWeight: 600 }}>Edit / Override Entry</h3>
              <button className="btn btn-ghost" onClick={() => setEditEntry(null)}><Icon name="x" size={16} /></button>
            </div>
            <div style={{ marginBottom: '16px', padding: '10px 14px', background: 'var(--bg-surface)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
              <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '2px' }}>{editEntry.swimmer}</div>
              <div style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{editEntry.event}</div>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div>
                <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>SEED TIME</label>
                <input className="input" value={editSeedTime} onChange={(e) => setEditSeedTime(e.target.value)} placeholder="e.g. 52.84 or 1:02.15" style={{ width: '100%' }} />
              </div>
              <div>
                <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>OVERRIDE REASON (required if changing time)</label>
                <textarea className="input" value={editReason} onChange={(e) => setEditReason(e.target.value)} rows={3} placeholder="Explain why time is being overridden..." style={{ width: '100%', resize: 'vertical' }} />
              </div>
            </div>
            <div style={{ marginTop: '6px', padding: '10px 12px', background: 'var(--warning-bg)', borderRadius: 'var(--r-sm)', border: '1px solid var(--warning)', fontSize: '11px', color: 'var(--warning)' }}>
              This override will be logged in the audit trail.
            </div>
            <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end', marginTop: '16px' }}>
              <button className="btn btn-secondary" onClick={() => setEditEntry(null)}>Cancel</button>
              <button className="btn btn-primary" onClick={saveEdit}>Save Override</button>
            </div>
          </div>
        </div>
      )}

      {/* Add Entry Modal */}
      {showAddEntry && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <div className="card" style={{ padding: '24px', width: '480px', maxWidth: '90vw' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
              <div>
                <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '18px', fontWeight: 600, marginBottom: '2px' }}>Add Entry for Swimmer</h3>
                <p style={{ fontSize: '12px', color: 'var(--text-muted)' }}>Sign up a committed swimmer for an additional event.</p>
              </div>
              <button className="btn btn-ghost" onClick={() => setShowAddEntry(false)}><Icon name="x" size={16} /></button>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div>
                <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>SWIMMER</label>
                <select className="input" value={newEntryDraft.swimmerId} onChange={(e) => setNewEntryDraft((prev) => ({ ...prev, swimmerId: e.target.value }))} style={{ width: '100%' }}>
                  <option value="">Select swimmer...</option>
                  {availableSwimmers.map((c) => (
                    <option key={c.swimmerId} value={c.swimmerId}>{c.swimmer}</option>
                  ))}
                </select>
              </div>
              <div>
                <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>EVENT</label>
                <select className="input" value={newEntryDraft.eventId} onChange={(e) => setNewEntryDraft((prev) => ({ ...prev, eventId: e.target.value }))} style={{ width: '100%' }}>
                  <option value="">Select event...</option>
                  {MOCK_EVENTS.map((e) => (
                    <option key={e.id} value={e.id}>#{e.eventNumber} {e.name} ({e.gender === 'M' ? 'Male' : 'Female'} {e.ageGroup})</option>
                  ))}
                </select>
              </div>
              <div>
                <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>SEED TIME</label>
                <input className="input" value={newEntryDraft.seedTime} onChange={(e) => setNewEntryDraft((prev) => ({ ...prev, seedTime: e.target.value }))} placeholder="e.g. 1:02.15 or 58.40" style={{ width: '100%' }} />
              </div>
              <div>
                <label style={{ fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)', display: 'block', marginBottom: '6px' }}>COACH NOTE (optional)</label>
                <input className="input" value={newEntryDraft.note} onChange={(e) => setNewEntryDraft((prev) => ({ ...prev, note: e.target.value }))} placeholder="Reason for manual entry..." style={{ width: '100%' }} />
              </div>
            </div>
            <div style={{ marginTop: '6px', padding: '10px 12px', background: 'var(--info-bg)', borderRadius: 'var(--r-sm)', border: '1px solid var(--info)', fontSize: '11px', color: 'var(--info)' }}>
              This adds an entry even if the swimmer has already committed. The entry will be marked as coach-created.
            </div>
            <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end', marginTop: '16px' }}>
              <button className="btn btn-secondary" onClick={() => setShowAddEntry(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={handleAddEntry} disabled={!canAddEntry}>Add Entry</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Overview Tab ─────────────────────────────────────────────────────────────

function OverviewTab() {
  const [selfSignup, setSelfSignup] = useState(MOCK_MEET.swimmerSelfSignup)
  const [description, setDescription] = useState(MOCK_MEET.description)
  const [editingDesc, setEditingDesc] = useState(false)
  const [attachments, setAttachments] = useState<MeetAttachment[]>(MOCK_ATTACHMENTS)

  function handleAttachmentUpload(event: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.target.files ?? [])
    if (files.length === 0) return

    const uploadedAt = new Date().toISOString().slice(0, 10)
    const nextAttachments = files.map((file) => ({
      id: `att-${Date.now()}-${file.name}`,
      name: file.name,
      type: file.type.startsWith('image/') ? 'image' as const : file.type.includes('pdf') ? 'pdf' as const : 'file' as const,
      uploadedAt,
      uploadedBy: 'Current Admin',
      size: `${(file.size / 1024 / 1024).toFixed(file.size > 1024 * 1024 ? 1 : 2)} MB`,
      url: '#',
    }))

    setAttachments((prev) => [...nextAttachments, ...prev])
    event.target.value = ''
  }

  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
      {/* Left column */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
        {/* Meet details */}
        <div className="card" style={{ padding: '20px' }}>
          <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Meet Details</h3>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            {[
              { label: 'Sanction #', value: MOCK_MEET.sanctionNumber },
              { label: 'Host', value: MOCK_MEET.host },
              { label: 'Course', value: MOCK_MEET.course },
              { label: 'Entry Deadline', value: MOCK_MEET.entryDeadline },
              { label: 'Start Date', value: MOCK_MEET.startDate },
              { label: 'End Date', value: MOCK_MEET.endDate },
              { label: 'Max Events/Swimmer', value: String(MOCK_MEET.maxEntriesPerSwimmer) },
              { label: 'Entry Fee / Event', value: `$${MOCK_MEET.entryFeePerEvent.toFixed(2)}` },
            ].map((row) => (
              <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600 }}>{row.value}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Swimmer self-signup toggle */}
        <div className="card" style={{ padding: '20px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '16px' }}>
            <div>
              <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>Swimmer Self-Signup</h3>
              <p style={{ fontSize: '12px', color: 'var(--text-muted)', lineHeight: 1.5 }}>
                Allow swimmers to sign themselves up for this meet via the swimmer portal. When enabled, swimmers can select their own events within the configured limits.
              </p>
            </div>
            <button
              onClick={() => setSelfSignup(!selfSignup)}
              style={{
                position: 'relative', width: '40px', height: '22px', borderRadius: '11px', flexShrink: 0,
                background: selfSignup ? 'var(--accent)' : 'var(--bg-elevated)',
                border: `1px solid ${selfSignup ? 'var(--accent)' : 'var(--border)'}`,
                cursor: 'pointer', transition: 'all var(--dur-fast)',
              }}
            >
              <div style={{
                position: 'absolute', top: '2px',
                left: selfSignup ? '19px' : '2px',
                width: '16px', height: '16px', borderRadius: '50%',
                background: selfSignup ? '#fff' : 'var(--text-muted)',
                transition: 'left var(--dur-fast)',
              }} />
            </button>
          </div>
          {selfSignup && (
            <div style={{ marginTop: '12px', padding: '10px 12px', background: 'var(--success-bg)', borderRadius: 'var(--r-sm)', border: '1px solid var(--success)', fontSize: '11px', color: 'var(--success)', fontFamily: 'var(--font-mono)' }}>
              ✓ Swimmers can sign up via the swimmer portal
            </div>
          )}
        </div>

        {/* Lifecycle actions */}
        <div className="card" style={{ padding: '20px' }}>
          <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '12px', fontFamily: 'var(--font-mono)' }}>Lifecycle Actions</h3>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
            <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px', justifyContent: 'flex-start' }}>
              <Icon name="check-square" size={13} /> Close Commitments
            </button>
            <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px', justifyContent: 'flex-start' }}>
              <Icon name="clipboard-list" size={13} /> Finalize Entries
            </button>
            <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px', justifyContent: 'flex-start' }}>
              <Icon name="download-cloud" size={13} /> Export SD3 File
            </button>
          </div>
        </div>
      </div>

      {/* Right column */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
        {/* Location */}
        <div className="card" style={{ padding: '20px' }}>
          <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '12px', fontFamily: 'var(--font-mono)' }}>Location</h3>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.6 }}>{MOCK_MEET.location}</p>
        </div>

        {/* Meet description / info */}
        <div className="card" style={{ padding: '20px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)' }}>Meet Info / Description</h3>
            <button className="btn btn-ghost" style={{ padding: '3px 8px', fontSize: '11px', display: 'flex', alignItems: 'center', gap: '4px' }} onClick={() => setEditingDesc(!editingDesc)}>
              <Icon name={editingDesc ? 'check' : 'edit-2'} size={11} /> {editingDesc ? 'Save' : 'Edit'}
            </button>
          </div>
          {editingDesc ? (
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={8}
              className="input"
              style={{ width: '100%', resize: 'vertical', fontSize: '13px', lineHeight: 1.6 }}
            />
          ) : (
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7, whiteSpace: 'pre-line' }}>{description}</p>
          )}
        </div>

        {/* Attachments */}
        <div className="card" style={{ padding: '20px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)' }}>Attachments</h3>
            <label className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '11px', cursor: 'pointer' }}>
              <Icon name="upload-cloud" size={12} /> Upload
              <input type="file" multiple accept=".pdf,image/*,.doc,.docx" style={{ display: 'none' }} onChange={handleAttachmentUpload} />
            </label>
          </div>

          {/* Upload zone */}
          <div style={{
            border: '2px dashed var(--border)', borderRadius: 'var(--r-md)', padding: '20px',
            textAlign: 'center', marginBottom: '14px', background: 'var(--bg-surface)',
            cursor: 'pointer', transition: 'border-color var(--dur-fast)',
          }}
            onMouseEnter={(e) => (e.currentTarget.style.borderColor = 'var(--accent)')}
            onMouseLeave={(e) => (e.currentTarget.style.borderColor = 'var(--border)')}
          >
            <Icon name="upload-cloud" size={24} style={{ color: 'var(--text-muted)', marginBottom: '8px' }} />
            <label style={{ fontSize: '13px', color: 'var(--text-muted)', cursor: 'pointer' }}>
              Drag & drop or <span style={{ color: 'var(--accent)' }}>browse files</span>
              <input type="file" multiple accept=".pdf,image/*,.doc,.docx" style={{ display: 'none' }} onChange={handleAttachmentUpload} />
            </label>
            <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '4px' }}>PDF, PNG, JPG · Max 20MB</p>
          </div>

          {/* File list */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
            {attachments.map((att) => (
              <div key={att.id} style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '10px 12px', background: 'var(--bg-surface)', borderRadius: 'var(--r-sm)', border: '1px solid var(--border)' }}>
                <div style={{ width: '28px', height: '28px', borderRadius: 'var(--r-sm)', background: att.type === 'pdf' ? 'var(--error-bg)' : att.type === 'image' ? 'var(--info-bg)' : 'var(--bg-elevated)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                  <Icon name={att.type === 'pdf' ? 'file-text' : att.type === 'image' ? 'eye' : 'file-text'} size={14} style={{ color: att.type === 'pdf' ? 'var(--error)' : att.type === 'image' ? 'var(--info)' : 'var(--text-muted)' }} />
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{att.name}</div>
                  <div style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>{att.size} · {att.uploadedBy} · {att.uploadedAt}</div>
                </div>
                <a href={att.url} style={{ flexShrink: 0 }}>
                  <button className="btn btn-ghost" style={{ padding: '3px 6px' }}><Icon name="download-cloud" size={12} /></button>
                </a>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function MeetDetailPage() {
  const [activeTab, setActiveTab] = useState('overview')

  const committed = MOCK_COMMITMENTS.filter((c) => c.status === 'committed_yes').length
  const awaiting  = MOCK_COMMITMENTS.filter((c) => c.status === 'invited').length
  const declined  = MOCK_COMMITMENTS.filter((c) => c.status === 'committed_no').length

  const tabs = [
    { id: 'overview',     label: 'Overview' },
    { id: 'sessions',     label: 'Sessions',    count: MOCK_SESSIONS.length },
    { id: 'events',       label: 'Events',      count: MOCK_EVENTS.length },
    { id: 'commitments',  label: 'Commitments', count: MOCK_COMMITMENTS.length },
    { id: 'entries',      label: 'Entries',     count: MOCK_ENTRIES.length },
  ]

  const statusCfg = STATUS_CFG[MOCK_MEET.status]

  const COMMITMENT_COLS: Column<Commitment>[] = [
    {
      key: 'swimmer', label: 'Swimmer', sortable: true,
      render: (r) => (
        <Link href={`/members/swimmers/${r.swimmerId}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
          {r.swimmer}
        </Link>
      ),
    },
    { key: 'household', label: 'Household', render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{r.household}</span> },
    {
      key: 'status', label: 'Response',
      render: (r) => {
        const c = COMMITMENT_STATUS_CFG[r.status]
        return <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', textTransform: 'uppercase' as const }}>{c.label}</span>
      },
    },
    {
      key: 'eventSelections', label: 'Events Selected', align: 'right',
      render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: r.eventSelections > 0 ? 'var(--text)' : 'var(--text-muted)' }}>{r.eventSelections > 0 ? r.eventSelections : '—'}</span>,
    },
    { key: 'respondedAt', label: 'Responded', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{r.respondedAt ?? '—'}</span> },
  ]

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      <Link href="/meets" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--text)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-subtle)')}>
        <Icon name="chevron-left" size={14} /> Meets
      </Link>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '6px' }}>
            <h1 style={{ fontSize: '24px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2 }}>
              {MOCK_MEET.name}
            </h1>
            <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: statusCfg.bg, color: statusCfg.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em', whiteSpace: 'nowrap' }}>
              {statusCfg.label}
            </span>
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', display: 'flex', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
            <span>{MOCK_MEET.host}</span>
            <span>·</span>
            <span>{MOCK_MEET.location.split(',')[0]}</span>
            <span>·</span>
            <span>{MOCK_MEET.startDate} → {MOCK_MEET.endDate}</span>
            <span>·</span>
            <span style={{ padding: '1px 6px', borderRadius: 'var(--r-xs)', border: '1px solid var(--border)', background: 'var(--bg-elevated)' }}>{MOCK_MEET.course}</span>
          </div>
        </div>
        <div style={{ display: 'flex', gap: '8px', flexShrink: 0 }}>
          <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Icon name="download-cloud" size={13} /> Export SD3
          </button>
          <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Icon name="edit-2" size={13} /> Edit Meet
          </button>
        </div>
      </div>

      {/* Quick stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'Sessions',   value: MOCK_SESSIONS.length,    color: 'var(--text)' },
          { label: 'Events',     value: MOCK_EVENTS.length,      color: 'var(--text)' },
          { label: 'Going',      value: committed,               color: 'var(--success)' },
          { label: 'Awaiting',   value: awaiting,                color: 'var(--warning)' },
          { label: 'Declined',   value: declined,                color: 'var(--error)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'overview'    && <OverviewTab />}
      {activeTab === 'sessions'    && <SessionsTab />}
      {activeTab === 'events'      && <EventsTab />}

      {/* Commitments */}
      {activeTab === 'commitments' && (
        <div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px', marginBottom: '20px' }}>
            {[
              { label: 'Going',    value: committed, color: 'var(--success)', bg: 'var(--success-bg)' },
              { label: 'Awaiting', value: awaiting,  color: 'var(--warning)', bg: 'var(--warning-bg)' },
              { label: 'Declined', value: declined,  color: 'var(--error)',   bg: 'var(--error-bg)' },
            ].map((s) => (
              <div key={s.label} style={{ display: 'flex', alignItems: 'center', gap: '12px', padding: '14px 16px', background: s.bg, borderRadius: 'var(--r-lg)', border: `1px solid ${s.color}22` }}>
                <div style={{ fontSize: '28px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
                <div style={{ fontSize: '12px', color: s.color, fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', fontWeight: 600 }}>{s.label}</div>
              </div>
            ))}
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '12px' }}>
            <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Icon name="check-square" size={13} /> Lock All Commitments
            </button>
          </div>
          <Table columns={COMMITMENT_COLS} data={MOCK_COMMITMENTS} emptyTitle="No commitments" emptyDescription="Publish the meet to start collecting commitments from families." />
        </div>
      )}

      {activeTab === 'entries' && <EntriesTab />}
    </div>
  )
}
