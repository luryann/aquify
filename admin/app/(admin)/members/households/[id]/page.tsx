'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'

// ─── Types ────────────────────────────────────────────────────────────────────

type Swimmer = {
  id: string
  name: string
  ussNumber: string
  dob: string
  gender: 'M' | 'F'
  groups: string[]
  status: string
}

type Contact = {
  id: string
  name: string
  role: string
  email: string
  phone: string
  isPrimary: boolean
}

type Invoice = {
  id: string
  description: string
  amount: number
  status: 'draft' | 'issued' | 'paid' | 'voided' | 'overdue'
  issuedAt: string
  dueAt: string
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_HOUSEHOLD = {
  id: 'hh-001',
  name: 'Henderson Family',
  status: 'active' as const,
  address: '4821 Maple Drive, Portland, OR 97201',
  balance: -240.00,
  creditOnFile: 500.00,
  orgId: 'dare-aquatics',
  createdAt: '2023-09-01',
  notes: 'Scholarship recipient — reduced registration fee applies.',
}

const MOCK_SWIMMERS: Swimmer[] = [
  { id: 'sw-001', name: 'Caleb Henderson',   ussNumber: '185-4821-A', dob: '2012-03-15', gender: 'M', groups: ['Senior Elite'],       status: 'active' },
  { id: 'sw-002', name: 'Mia Henderson',     ussNumber: '185-4822-B', dob: '2014-07-22', gender: 'F', groups: ['Junior A'],            status: 'active' },
  { id: 'sw-003', name: 'Owen Henderson',    ussNumber: '185-4823-C', dob: '2016-11-08', gender: 'M', groups: ['Development Gold'],    status: 'active' },
]

const MOCK_CONTACTS: Contact[] = [
  { id: 'ct-001', name: 'Sarah Henderson', role: 'Parent / Guardian', email: 'sarah@example.com', phone: '(503) 555-0142', isPrimary: true },
  { id: 'ct-002', name: 'Tom Henderson',   role: 'Parent / Guardian', email: 'tom@example.com',   phone: '(503) 555-0143', isPrimary: false },
]

const MOCK_INVOICES: Invoice[] = [
  { id: 'inv-001', description: '2024–25 Registration Fee',   amount: 450.00, status: 'paid',   issuedAt: '2024-09-01', dueAt: '2024-09-30' },
  { id: 'inv-002', description: 'November Monthly Dues',      amount: 240.00, status: 'overdue',issuedAt: '2024-11-01', dueAt: '2024-11-15' },
  { id: 'inv-003', description: 'Winter Invitational Entry',  amount: 65.00,  status: 'issued', issuedAt: '2024-12-01', dueAt: '2024-12-20' },
]

// ─── Status helpers ───────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: string }) {
  const cfg: Record<string, { color: string; bg: string }> = {
    active:   { color: 'var(--success)',  bg: 'var(--success-bg)' },
    inactive: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    pending:  { color: 'var(--warning)',  bg: 'var(--warning-bg)' },
    paid:     { color: 'var(--success)',  bg: 'var(--success-bg)' },
    issued:   { color: 'var(--info)',     bg: 'var(--info-bg)' },
    overdue:  { color: 'var(--error)',    bg: 'var(--error-bg)' },
    draft:    { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    voided:   { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  }
  const c = cfg[status] ?? { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' }
  return (
    <span style={{
      fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px',
      background: c.bg, color: c.color, fontFamily: 'var(--font-mono)',
      letterSpacing: '0.02em', textTransform: 'uppercase',
    }}>
      {status}
    </span>
  )
}

// ─── Sub-tables ───────────────────────────────────────────────────────────────

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
  { key: 'dob', label: 'DOB', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{r.dob}</span> },
  { key: 'gender', label: 'Gender', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{r.gender === 'M' ? 'Male' : 'Female'}</span> },
  {
    key: 'groups', label: 'Group',
    render: (r) => (
      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
        {r.groups.map((g) => (
          <span key={g} style={{ fontSize: '11px', padding: '2px 7px', borderRadius: '8px', background: 'var(--accent-subtle)', color: 'var(--accent)', fontFamily: 'var(--font-mono)' }}>{g}</span>
        ))}
      </div>
    ),
  },
  { key: 'status', label: 'Status', render: (r) => <StatusBadge status={r.status} /> },
]

const CONTACT_COLS: Column<Contact>[] = [
  {
    key: 'name', label: 'Contact', sortable: true,
    render: (r) => (
      <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
        <div style={{ width: '30px', height: '30px', borderRadius: '50%', background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: '12px', fontWeight: 700, color: 'var(--accent)', flexShrink: 0 }}>
          {r.name.split(' ').map((n) => n[0]).join('').slice(0, 2)}
        </div>
        <div>
          <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>
            {r.name}
            {r.isPrimary && <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', background: 'var(--accent-subtle)', color: 'var(--accent)', padding: '1px 6px', borderRadius: '8px', marginLeft: '6px', fontWeight: 600 }}>PRIMARY</span>}
          </div>
          <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>{r.role}</div>
        </div>
      </div>
    ),
  },
  { key: 'email', label: 'Email', render: (r) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>{r.email}</span> },
  { key: 'phone', label: 'Phone', render: (r) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>{r.phone}</span> },
]

const INVOICE_COLS: Column<Invoice>[] = [
  { key: 'id', label: 'Invoice', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{r.id}</span> },
  { key: 'description', label: 'Description', render: (r) => <span style={{ fontSize: '13px', color: 'var(--text)' }}>{r.description}</span> },
  {
    key: 'amount', label: 'Amount', align: 'right', sortable: true,
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>${r.amount.toFixed(2)}</span>,
  },
  { key: 'status', label: 'Status', render: (r) => <StatusBadge status={r.status} /> },
  { key: 'dueAt', label: 'Due', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{r.dueAt}</span> },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function HouseholdDetailPage() {
  const [activeTab, setActiveTab] = useState('overview')

  const tabs = [
    { id: 'overview',  label: 'Overview' },
    { id: 'swimmers',  label: 'Swimmers',  count: MOCK_SWIMMERS.length },
    { id: 'contacts',  label: 'Contacts',  count: MOCK_CONTACTS.length },
    { id: 'invoices',  label: 'Invoices',  count: MOCK_INVOICES.length },
  ]

  const balance = MOCK_HOUSEHOLD.balance
  const balanceColor = balance < 0 ? 'var(--error)' : balance > 0 ? 'var(--success)' : 'var(--text-muted)'
  const balanceText = balance < 0
    ? `-$${Math.abs(balance).toFixed(2)}`
    : balance > 0
    ? `+$${balance.toFixed(2)}`
    : '$0.00'

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      {/* Back link */}
      <Link href="/members/households" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--text)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-subtle)')}>
        <Icon name="chevron-left" size={14} />
        Households
      </Link>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
            <h1 style={{ fontSize: '24px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2 }}>
              {MOCK_HOUSEHOLD.name}
            </h1>
            <StatusBadge status={MOCK_HOUSEHOLD.status} />
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
            {MOCK_HOUSEHOLD.id} · {MOCK_HOUSEHOLD.address}
          </div>
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button className="btn btn-secondary">
            <Icon name="edit-2" size={13} />
            Edit
          </button>
          <button className="btn btn-secondary">
            <Icon name="mail" size={13} />
            Message
          </button>
        </div>
      </div>

      {/* Quick stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'Balance',        value: balanceText,                        color: balanceColor },
          { label: 'Swimmers',       value: MOCK_SWIMMERS.length.toString(),    color: 'var(--text)' },
          { label: 'Contacts',       value: MOCK_CONTACTS.length.toString(),    color: 'var(--text)' },
          { label: 'Member Since',   value: MOCK_HOUSEHOLD.createdAt,           color: 'var(--text-subtle)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>
              {s.label}
            </div>
            <div style={{ fontSize: '18px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>
              {s.value}
            </div>
          </div>
        ))}
      </div>

      {/* Tabs */}
      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {/* Tab content */}
      {activeTab === 'overview' && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>
              Account Info
            </h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[
                { label: 'Household ID',     value: MOCK_HOUSEHOLD.id },
                { label: 'Account Balance',  value: balanceText, color: balanceColor },
                { label: 'Credit on File',   value: `$${MOCK_HOUSEHOLD.creditOnFile.toFixed(2)}` },
                { label: 'Organization',     value: MOCK_HOUSEHOLD.orgId },
                { label: 'Created',          value: MOCK_HOUSEHOLD.createdAt },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: (row as { color?: string }).color ?? 'var(--text)', fontWeight: 600 }}>
                    {row.value}
                  </span>
                </div>
              ))}
            </div>
          </div>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>
              Notes
            </h3>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.6 }}>
              {MOCK_HOUSEHOLD.notes || 'No notes recorded.'}
            </p>
          </div>
        </div>
      )}

      {activeTab === 'swimmers' && (
        <Table columns={SWIMMER_COLS} data={MOCK_SWIMMERS} emptyTitle="No swimmers" emptyDescription="No swimmers are linked to this household." />
      )}

      {activeTab === 'contacts' && (
        <Table columns={CONTACT_COLS} data={MOCK_CONTACTS} emptyTitle="No contacts" emptyDescription="No contacts are linked to this household." />
      )}

      {activeTab === 'invoices' && (
        <div>
          <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '12px' }}>
            <Link href="/billing/invoices" style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px' }}>
              View all invoices <Icon name="external-link" size={12} />
            </Link>
          </div>
          <Table columns={INVOICE_COLS} data={MOCK_INVOICES} emptyTitle="No invoices" emptyDescription="No invoices have been issued to this household." />
        </div>
      )}
    </div>
  )
}
