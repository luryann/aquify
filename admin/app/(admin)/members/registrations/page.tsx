'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'

// ─── Types ────────────────────────────────────────────────────────────────────

type RegistrationStatus = 'pending' | 'active' | 'completed' | 'rejected' | 'withdrawn'
type WaiverStatus = 'pending' | 'accepted' | 'expired'

type Registration = {
  id: string
  household: string
  householdId: string
  season: string
  submittedAt: string
  status: RegistrationStatus
  waiverStatus: WaiverStatus
  swimmerCount: number
  total: number
  notes: string | null
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_REGISTRATIONS: Registration[] = [
  { id: 'reg-001', household: 'Henderson Family',  householdId: 'hh-001', season: '2024–25', submittedAt: '2024-08-15', status: 'active',    waiverStatus: 'accepted', swimmerCount: 3, total: 450.00, notes: null },
  { id: 'reg-002', household: 'Park Family',        householdId: 'hh-002', season: '2024–25', submittedAt: '2024-08-18', status: 'active',    waiverStatus: 'accepted', swimmerCount: 2, total: 350.00, notes: null },
  { id: 'reg-003', household: 'Torres Household',   householdId: 'hh-003', season: '2024–25', submittedAt: '2024-08-20', status: 'active',    waiverStatus: 'accepted', swimmerCount: 1, total: 200.00, notes: null },
  { id: 'reg-004', household: 'Kim Family',         householdId: 'hh-004', season: '2024–25', submittedAt: '2024-09-02', status: 'pending',   waiverStatus: 'pending',  swimmerCount: 2, total: 350.00, notes: 'Awaiting payment confirmation.' },
  { id: 'reg-005', household: 'Okafor Household',   householdId: 'hh-005', season: '2024–25', submittedAt: '2024-08-10', status: 'active',    waiverStatus: 'accepted', swimmerCount: 4, total: 600.00, notes: null },
  { id: 'reg-006', household: 'Reyes Family',       householdId: 'hh-006', season: '2024–25', submittedAt: '2024-08-22', status: 'active',    waiverStatus: 'accepted', swimmerCount: 2, total: 350.00, notes: null },
  { id: 'reg-007', household: 'Patel Family',       householdId: 'hh-008', season: '2024–25', submittedAt: '2024-08-30', status: 'active',    waiverStatus: 'accepted', swimmerCount: 3, total: 450.00, notes: null },
  { id: 'reg-008', household: 'Müller Household',   householdId: 'hh-009', season: '2024–25', submittedAt: '2024-09-05', status: 'active',    waiverStatus: 'accepted', swimmerCount: 2, total: 350.00, notes: null },
  { id: 'reg-009', household: 'Tanaka Household',   householdId: 'hh-011', season: '2024–25', submittedAt: '2024-09-10', status: 'active',    waiverStatus: 'accepted', swimmerCount: 1, total: 200.00, notes: null },
  { id: 'reg-010', household: 'O\'Brien Family',    householdId: 'hh-012', season: '2024–25', submittedAt: '2024-08-12', status: 'active',    waiverStatus: 'accepted', swimmerCount: 3, total: 450.00, notes: null },
  { id: 'reg-011', household: 'Jackson Family',     householdId: 'hh-010', season: '2024–25', submittedAt: '2024-09-01', status: 'rejected',  waiverStatus: 'expired',  swimmerCount: 2, total: 0,      notes: 'Rejected due to outstanding balance.' },
  // Prior season
  { id: 'reg-012', household: 'Henderson Family',  householdId: 'hh-001', season: '2023–24', submittedAt: '2023-08-20', status: 'completed', waiverStatus: 'accepted', swimmerCount: 3, total: 420.00, notes: null },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

const STATUS_CONFIG: Record<RegistrationStatus, { label: string; color: string; bg: string }> = {
  pending:   { label: 'Pending',   color: 'var(--warning)',    bg: 'var(--warning-bg)' },
  active:    { label: 'Active',    color: 'var(--success)',    bg: 'var(--success-bg)' },
  completed: { label: 'Completed', color: 'var(--info)',       bg: 'var(--info-bg)' },
  rejected:  { label: 'Rejected',  color: 'var(--error)',      bg: 'var(--error-bg)' },
  withdrawn: { label: 'Withdrawn', color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
}

const WAIVER_CONFIG: Record<WaiverStatus, { label: string; color: string }> = {
  pending:  { label: 'Pending',  color: 'var(--warning)' },
  accepted: { label: 'Signed',   color: 'var(--success)' },
  expired:  { label: 'Expired',  color: 'var(--error)' },
}

// ─── Columns ──────────────────────────────────────────────────────────────────

const COLUMNS: Column<Registration>[] = [
  { key: 'id', label: 'ID', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)' }}>{r.id}</span> },
  {
    key: 'household', label: 'Household', sortable: true,
    render: (r) => (
      <Link href={`/members/households/${r.householdId}`} style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
        {r.household}
      </Link>
    ),
  },
  { key: 'season', label: 'Season', render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)', padding: '2px 7px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-xs)', border: '1px solid var(--border)' }}>{r.season}</span> },
  { key: 'submittedAt', label: 'Submitted', sortable: true, render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{r.submittedAt}</span> },
  {
    key: 'status', label: 'Status',
    render: (r) => {
      const c = STATUS_CONFIG[r.status]
      return <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: c.bg, color: c.color, fontFamily: 'var(--font-mono)', textTransform: 'uppercase' as const, letterSpacing: '0.02em' }}>{c.label}</span>
    },
  },
  {
    key: 'waiverStatus', label: 'Waiver',
    render: (r) => {
      const c = WAIVER_CONFIG[r.waiverStatus]
      return (
        <div style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '12px', color: c.color }}>
          <Icon name={r.waiverStatus === 'accepted' ? 'check-circle' : r.waiverStatus === 'expired' ? 'alert-circle' : 'clock'} size={13} />
          {c.label}
        </div>
      )
    },
  },
  {
    key: 'swimmerCount', label: 'Swimmers', align: 'right',
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text)' }}>{r.swimmerCount}</span>,
  },
  {
    key: 'total', label: 'Total', align: 'right', sortable: true,
    render: (r) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 600, color: r.total > 0 ? 'var(--text)' : 'var(--text-muted)' }}>{r.total > 0 ? `$${r.total.toFixed(2)}` : '—'}</span>,
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function RegistrationsPage() {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [seasonFilter, setSeasonFilter] = useState('2024–25')

  const seasons = [...new Set(MOCK_REGISTRATIONS.map((r) => r.season))].sort().reverse()

  const filtered = useMemo(() => {
    return MOCK_REGISTRATIONS.filter((r) => {
      const q = search.toLowerCase()
      const matchesSearch = !search || r.household.toLowerCase().includes(q)
      const matchesStatus = statusFilter === 'all' || r.status === statusFilter
      const matchesSeason = seasonFilter === 'all' || r.season === seasonFilter
      return matchesSearch && matchesStatus && matchesSeason
    })
  }, [search, statusFilter, seasonFilter])

  const counts = useMemo(() => {
    const base = MOCK_REGISTRATIONS.filter((r) => r.season === seasonFilter || seasonFilter === 'all')
    return {
      total:   base.length,
      active:  base.filter((r) => r.status === 'active').length,
      pending: base.filter((r) => r.status === 'pending').length,
      revenue: base.reduce((acc, r) => acc + r.total, 0),
    }
  }, [seasonFilter])

  const selectStyle: React.CSSProperties = {
    padding: '8px 12px',
    background: 'var(--bg-elevated)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--r-md)',
    fontSize: '13px',
    color: 'var(--text)',
    outline: 'none',
    cursor: 'pointer',
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Registrations</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Season membership registrations and waiver tracking</p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <Icon name="plus" size={14} /> New Registration
        </button>
      </div>

      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Total',    value: counts.total,                                                      color: 'var(--text)' },
          { label: 'Active',   value: counts.active,                                                     color: 'var(--success)' },
          { label: 'Pending',  value: counts.pending,                                                    color: 'var(--warning)' },
          { label: 'Revenue',  value: `$${counts.revenue.toLocaleString('en-US', { minimumFractionDigits: 2 })}`, color: 'var(--accent)' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '300px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search households..." value={search} onChange={(e) => setSearch(e.target.value)}
            style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <select value={seasonFilter} onChange={(e) => setSeasonFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Seasons</option>
          {seasons.map((s) => <option key={s} value={s}>{s}</option>)}
        </select>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Statuses</option>
          <option value="pending">Pending</option>
          <option value="active">Active</option>
          <option value="completed">Completed</option>
          <option value="rejected">Rejected</option>
          <option value="withdrawn">Withdrawn</option>
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>
          {filtered.length} registrations
        </span>
      </div>

      <Table columns={COLUMNS} data={filtered} pageSize={10} emptyTitle="No registrations found" emptyDescription="Try adjusting your season or status filter." />
    </div>
  )
}
