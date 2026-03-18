'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'

// ─── Types ────────────────────────────────────────────────────────────────────

type BestTime = {
  event: string
  course: 'SCY' | 'SCM' | 'LCM'
  time: string
  meet: string
  date: string
  deltaFromStandard: string | null
  standard: string | null
}

type GroupMembership = {
  id: string
  groupName: string
  groupType: string
  assignedAt: string
  status: string
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_SWIMMER = {
  id: 'sw-001',
  name: 'Caleb Henderson',
  ussNumber: '185-4821-A',
  dob: '2012-03-15',
  age: 12,
  gender: 'M',
  status: 'active',
  household: 'Henderson Family',
  householdId: 'hh-001',
  groups: ['Senior Elite'],
  email: null as string | null,
  notes: 'Strong 100 Free specialist. Targeting BB cut by end of season.',
}

const MOCK_BEST_TIMES: BestTime[] = [
  { event: '50 Free',    course: 'SCY', time: '24.55',  meet: 'Winter Invitational 2024', date: '2024-12-14', deltaFromStandard: '-1.45', standard: 'BB' },
  { event: '100 Free',   course: 'SCY', time: '52.84',  meet: 'League Championships 2024', date: '2024-11-02', deltaFromStandard: '-2.16', standard: 'BB' },
  { event: '200 Free',   course: 'SCY', time: '1:58.40',meet: 'Fall Classic 2024',          date: '2024-10-19', deltaFromStandard: null,    standard: null },
  { event: '100 Back',   course: 'SCY', time: '1:00.12',meet: 'Dual Meet — Sharks',         date: '2024-09-28', deltaFromStandard: '3.12',  standard: 'A' },
  { event: '100 Breast', course: 'SCY', time: '1:08.84',meet: 'Fall Classic 2024',          date: '2024-10-19', deltaFromStandard: null,    standard: null },
  { event: '200 IM',     course: 'SCY', time: '2:18.20',meet: 'League Championships 2024', date: '2024-11-02', deltaFromStandard: '5.20',  standard: 'A' },
]

const MOCK_GROUPS: GroupMembership[] = [
  { id: 'gm-001', groupName: 'Senior Elite', groupType: 'Training', assignedAt: '2023-09-01', status: 'active' },
]

const MOCK_HISTORY = [
  { date: '2024-12-14', event: 'Winter Invitational 2024 — Entered 4 events, 3 best times' },
  { date: '2024-11-02', event: 'League Championships — Entered 5 events, new 100 Free best time' },
  { date: '2024-10-19', event: 'Fall Classic — Entered 3 events' },
  { date: '2024-09-01', event: 'Assigned to Senior Elite group' },
  { date: '2023-09-01', event: 'Registration completed for 2023–24 season' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: string }) {
  const c = status === 'active' ? { color: 'var(--success)', bg: 'var(--success-bg)' } : { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' }
  return (
    <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', textTransform: 'uppercase' as const, letterSpacing: '0.02em' }}>
      {status}
    </span>
  )
}

// ─── Best Times Columns ───────────────────────────────────────────────────────

const BT_COLS: Column<BestTime>[] = [
  { key: 'event',  label: 'Event',  render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{r.event}</span> },
  { key: 'course', label: 'Course', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)', padding: '2px 6px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-xs)', border: '1px solid var(--border)' }}>{r.course}</span> },
  {
    key: 'time', label: 'Best Time', align: 'right', sortable: true,
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '14px', fontWeight: 700, color: 'var(--accent)' }}>{r.time}</span>,
  },
  {
    key: 'standard', label: 'Standard',
    render: (r) => r.standard
      ? <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', padding: '2px 7px', borderRadius: '8px', background: 'var(--accent-subtle)', color: 'var(--accent)' }}>{r.standard}</span>
      : <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>—</span>,
  },
  {
    key: 'deltaFromStandard', label: 'Δ Standard', align: 'right',
    render: (r) => {
      if (!r.deltaFromStandard) return <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>—</span>
      const n = parseFloat(r.deltaFromStandard)
      const color = n <= 0 ? 'var(--success)' : 'var(--warning)'
      return <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color, fontWeight: 600 }}>{n > 0 ? `+${r.deltaFromStandard}` : r.deltaFromStandard}s</span>
    },
  },
  { key: 'meet', label: 'Meet', render: (r) => <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{r.meet}</span> },
  { key: 'date', label: 'Date', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{r.date}</span> },
]

const GROUP_COLS: Column<GroupMembership>[] = [
  { key: 'groupName', label: 'Group', render: (r) => <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{r.groupName}</span> },
  { key: 'groupType', label: 'Type', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{r.groupType}</span> },
  { key: 'assignedAt', label: 'Assigned', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{r.assignedAt}</span> },
  {
    key: 'status', label: 'Status',
    render: (r) => <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: 'var(--success-bg)', color: 'var(--success)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase' as const }}>{r.status}</span>,
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function SwimmerDetailPage() {
  const [activeTab, setActiveTab] = useState('profile')

  const tabs = [
    { id: 'profile',     label: 'Profile' },
    { id: 'performance', label: 'Performance', count: MOCK_BEST_TIMES.length },
    { id: 'groups',      label: 'Groups',      count: MOCK_GROUPS.length },
    { id: 'history',     label: 'History',     count: MOCK_HISTORY.length },
  ]

  return (
    <div style={{ padding: '24px', maxWidth: '1100px' }}>

      {/* Back */}
      <Link href="/members/swimmers" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--text)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-subtle)')}>
        <Icon name="chevron-left" size={14} /> Swimmers
      </Link>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
          <div style={{ width: '48px', height: '48px', borderRadius: '50%', background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: '18px', fontWeight: 700, color: 'var(--accent)', fontFamily: 'var(--font-display)', flexShrink: 0 }}>
            {MOCK_SWIMMER.name.split(' ').map((n) => n[0]).join('').slice(0, 2)}
          </div>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
              <h1 style={{ fontSize: '24px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2 }}>
                {MOCK_SWIMMER.name}
              </h1>
              <StatusBadge status={MOCK_SWIMMER.status} />
            </div>
            <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', display: 'flex', alignItems: 'center', gap: '12px' }}>
              <span>{MOCK_SWIMMER.ussNumber}</span>
              <span>·</span>
              <span>Age {MOCK_SWIMMER.age}</span>
              <span>·</span>
              <span>{MOCK_SWIMMER.gender === 'M' ? 'Male' : 'Female'}</span>
              <span>·</span>
              <Link href={`/members/households/${MOCK_SWIMMER.householdId}`} style={{ color: 'var(--accent)', textDecoration: 'none' }}>
                {MOCK_SWIMMER.household}
              </Link>
            </div>
          </div>
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button className="btn btn-secondary"><Icon name="edit-2" size={13} /> Edit</button>
          <button className="btn btn-secondary"><Icon name="bar-chart-2" size={13} /> Performance</button>
        </div>
      </div>

      {/* Quick stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'DOB',            value: MOCK_SWIMMER.dob,             color: 'var(--text)' },
          { label: 'Best Event',     value: '100 Free',                   color: 'var(--accent)' },
          { label: 'Best Time',      value: '52.84',                      color: 'var(--accent)' },
          { label: 'Current Group',  value: MOCK_SWIMMER.groups[0] ?? '—',color: 'var(--text)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '16px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Tabs */}
      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {/* Profile */}
      {activeTab === 'profile' && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Swimmer Info</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[
                { label: 'USS Number',  value: MOCK_SWIMMER.ussNumber },
                { label: 'Date of Birth', value: MOCK_SWIMMER.dob },
                { label: 'Age',          value: `${MOCK_SWIMMER.age}` },
                { label: 'Gender',       value: MOCK_SWIMMER.gender === 'M' ? 'Male' : 'Female' },
                { label: 'Email',        value: MOCK_SWIMMER.email ?? '—' },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600 }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Coach Notes</h3>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7 }}>
              {MOCK_SWIMMER.notes}
            </p>
          </div>
        </div>
      )}

      {/* Performance */}
      {activeTab === 'performance' && (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Short Course Yards (SCY) best times across all events.</p>
            <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Icon name="plus" size={13} /> Manual Override
            </button>
          </div>
          <Table columns={BT_COLS} data={MOCK_BEST_TIMES} emptyTitle="No times recorded" emptyDescription="Times will appear here after results are imported." />
        </div>
      )}

      {/* Groups */}
      {activeTab === 'groups' && (
        <Table columns={GROUP_COLS} data={MOCK_GROUPS} emptyTitle="No group assignments" emptyDescription="This swimmer has not been assigned to any groups." />
      )}

      {/* History */}
      {activeTab === 'history' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '0px' }}>
          {MOCK_HISTORY.map((h, i) => (
            <div key={h.date} style={{ display: 'flex', gap: '16px', padding: '14px 0', borderBottom: i < MOCK_HISTORY.length - 1 ? '1px solid var(--border)' : 'none', alignItems: 'flex-start' }}>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)', flexShrink: 0, paddingTop: '2px', minWidth: '90px' }}>{h.date}</span>
              <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--border-strong)', flexShrink: 0, marginTop: '6px' }} />
              <span style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.5 }}>{h.event}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
