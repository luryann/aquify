'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'

// ─── Types ────────────────────────────────────────────────────────────────────

type Swimmer = { id: string; name: string; ussNumber: string; age: number; gender: 'M' | 'F'; status: string; joinedGroup: string }
type EligibilityRule = { id: string; ruleType: string; operator: string; value: string; description: string; active: boolean }

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_GROUP = {
  id: 'grp-001',
  name: 'Senior Elite',
  type: 'training' as const,
  season: '2024–25',
  description: 'Top training group for competitive swimmers targeting national qualifying standards. Practice 5–6 days per week.',
  coachName: 'Coach Williams',
  coachId: 'usr-coach-001',
  minAge: 13,
  maxAge: null as number | null,
  status: 'active' as const,
  createdAt: '2023-08-01',
}

const MOCK_SWIMMERS: Swimmer[] = [
  { id: 'sw-001', name: 'Caleb Henderson',  ussNumber: '185-4821-A', age: 12, gender: 'M', status: 'active', joinedGroup: '2023-09-01' },
  { id: 'sw-004', name: 'Jenna Park',       ussNumber: '185-5001-D', age: 14, gender: 'F', status: 'active', joinedGroup: '2022-09-01' },
  { id: 'sw-006', name: 'Sofia Torres',     ussNumber: '185-6001-F', age: 15, gender: 'F', status: 'active', joinedGroup: '2021-09-01' },
  { id: 'sw-009', name: 'Ade Okafor',       ussNumber: '185-8001-I', age: 16, gender: 'M', status: 'active', joinedGroup: '2020-09-01' },
]

const MOCK_RULES: EligibilityRule[] = [
  { id: 'rule-001', ruleType: 'age',          operator: 'gte', value: '13',     description: 'Swimmer must be at least 13 years old',  active: true },
  { id: 'rule-002', ruleType: 'best_time',    operator: 'lte', value: '57.99',  description: '100 Free SCY under 58 seconds (boys)',    active: true },
  { id: 'rule-003', ruleType: 'best_time',    operator: 'lte', value: '1:03.99',description: '100 Free SCY under 1:04 (girls)',         active: true },
  { id: 'rule-004', ruleType: 'registration', operator: 'eq',  value: 'active', description: 'Active registration for current season', active: true },
]

// ─── Columns ──────────────────────────────────────────────────────────────────

const SWIMMER_COLS: Column<Swimmer>[] = [
  {
    key: 'name', label: 'Swimmer', sortable: true,
    render: (r) => (
      <div>
        <Link href={`/members/swimmers/${r.id}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
          {r.name}
        </Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '2px' }}>{r.ussNumber}</div>
      </div>
    ),
  },
  { key: 'age', label: 'Age', align: 'right', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text)' }}>{r.age}</span> },
  { key: 'gender', label: 'Gender', render: (r) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{r.gender === 'M' ? 'Male' : 'Female'}</span> },
  {
    key: 'status', label: 'Status',
    render: (r) => (
      <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: 'var(--success-bg)', color: 'var(--success)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase' as const }}>
        {r.status}
      </span>
    ),
  },
  { key: 'joinedGroup', label: 'Since', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{r.joinedGroup}</span> },
]

const RULE_COLS: Column<EligibilityRule>[] = [
  { key: 'ruleType', label: 'Type', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', padding: '2px 8px', borderRadius: '8px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', color: 'var(--text-subtle)' }}>{r.ruleType}</span> },
  { key: 'operator', label: 'Operator', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--info)' }}>{r.operator}</span> },
  { key: 'value', label: 'Value', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{r.value}</span> },
  { key: 'description', label: 'Description', render: (r) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{r.description}</span> },
  {
    key: 'active', label: 'Status',
    render: (r) => (
      <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px',
        background: r.active ? 'var(--success-bg)' : 'var(--bg-elevated)',
        color:      r.active ? 'var(--success)' : 'var(--text-muted)',
        fontFamily: 'var(--font-mono)', textTransform: 'uppercase' as const }}>
        {r.active ? 'Active' : 'Disabled'}
      </span>
    ),
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function GroupDetailPage() {
  const [activeTab, setActiveTab] = useState('members')

  const tabs = [
    { id: 'members',     label: 'Members',           count: MOCK_SWIMMERS.length },
    { id: 'eligibility', label: 'Eligibility Rules',  count: MOCK_RULES.length },
    { id: 'settings',    label: 'Settings' },
  ]

  return (
    <div style={{ padding: '24px', maxWidth: '1100px' }}>

      <Link href="/members/groups" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--text)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-subtle)')}>
        <Icon name="chevron-left" size={14} /> Groups
      </Link>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
            <h1 style={{ fontSize: '24px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2 }}>
              {MOCK_GROUP.name}
            </h1>
            <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: 'var(--accent-subtle)', color: 'var(--accent)', fontFamily: 'var(--font-mono)', letterSpacing: '0.02em' }}>
              Training
            </span>
            <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: 'var(--success-bg)', color: 'var(--success)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase' as const }}>
              Active
            </span>
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
            {MOCK_GROUP.id} · Season {MOCK_GROUP.season}
          </div>
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button className="btn btn-secondary"><Icon name="plus" size={13} /> Add Swimmer</button>
          <button className="btn btn-secondary"><Icon name="edit-2" size={13} /> Edit</button>
        </div>
      </div>

      {/* Quick stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'Swimmers',  value: MOCK_SWIMMERS.length.toString(), color: 'var(--text)' },
          { label: 'Coach',     value: MOCK_GROUP.coachName ?? '—',     color: 'var(--text-subtle)' },
          { label: 'Age Range', value: `${MOCK_GROUP.minAge ?? '—'}+`,  color: 'var(--text)' },
          { label: 'Season',    value: MOCK_GROUP.season,               color: 'var(--text)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '16px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'members' && (
        <div>
          <div style={{ fontSize: '13px', color: 'var(--text-subtle)', marginBottom: '16px' }}>
            {MOCK_GROUP.description}
          </div>
          <Table columns={SWIMMER_COLS} data={MOCK_SWIMMERS} emptyTitle="No members" emptyDescription="No swimmers have been assigned to this group." />
        </div>
      )}

      {activeTab === 'eligibility' && (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Rules evaluated when checking swimmer eligibility for this group.</p>
            <button className="btn btn-secondary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Icon name="plus" size={13} /> Add Rule
            </button>
          </div>
          <Table columns={RULE_COLS} data={MOCK_RULES} emptyTitle="No rules" emptyDescription="No eligibility rules configured." />
        </div>
      )}

      {activeTab === 'settings' && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Group Settings</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[
                { label: 'Group ID',    value: MOCK_GROUP.id },
                { label: 'Type',        value: 'Training' },
                { label: 'Season',      value: MOCK_GROUP.season },
                { label: 'Coach',       value: MOCK_GROUP.coachName ?? '—' },
                { label: 'Min Age',     value: MOCK_GROUP.minAge ? `${MOCK_GROUP.minAge}` : '—' },
                { label: 'Max Age',     value: MOCK_GROUP.maxAge ? `${MOCK_GROUP.maxAge}` : 'No limit' },
                { label: 'Created',     value: MOCK_GROUP.createdAt },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600 }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Description</h3>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7 }}>{MOCK_GROUP.description}</p>

            <div style={{ marginTop: '20px', paddingTop: '16px', borderTop: '1px solid var(--border)' }}>
              <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--error)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '12px', fontFamily: 'var(--font-mono)' }}>Danger Zone</h3>
              <button className="btn btn-danger" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <Icon name="trash-2" size={13} /> Archive Group
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
