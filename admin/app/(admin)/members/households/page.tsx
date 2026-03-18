'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type HouseholdStatus = 'active' | 'inactive' | 'pending' | 'suspended'

type Household = {
  id: string
  name: string
  primaryContact: string
  email: string
  swimmerCount: number
  status: HouseholdStatus
  balance: number
  joinedAt: string
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_HOUSEHOLDS: Household[] = [
  { id: 'hh-001', name: 'Henderson Family',  primaryContact: 'Sarah Henderson',   email: 'sarah@example.com',   swimmerCount: 3, status: 'active',    balance: -240.00, joinedAt: '2023-09-01' },
  { id: 'hh-002', name: 'Park Family',        primaryContact: 'James Park',         email: 'jpark@example.com',   swimmerCount: 2, status: 'active',    balance: 0,        joinedAt: '2023-09-15' },
  { id: 'hh-003', name: 'Torres Household',   primaryContact: 'Maria Torres',       email: 'mtorres@example.com', swimmerCount: 1, status: 'active',    balance: 125.00,   joinedAt: '2022-08-20' },
  { id: 'hh-004', name: 'Kim Family',         primaryContact: 'David Kim',          email: 'dkim@example.com',    swimmerCount: 2, status: 'pending',   balance: -850.00, joinedAt: '2024-01-10' },
  { id: 'hh-005', name: 'Okafor Household',   primaryContact: 'Chidi Okafor',       email: 'cokafor@example.com', swimmerCount: 4, status: 'active',    balance: 0,        joinedAt: '2021-09-01' },
  { id: 'hh-006', name: 'Reyes Family',       primaryContact: 'Elena Reyes',        email: 'ereyes@example.com',  swimmerCount: 2, status: 'active',    balance: -60.00,  joinedAt: '2023-02-14' },
  { id: 'hh-007', name: 'Nguyen Household',   primaryContact: 'Linh Nguyen',        email: 'lnguyen@example.com', swimmerCount: 1, status: 'inactive',  balance: 0,        joinedAt: '2020-09-01' },
  { id: 'hh-008', name: 'Patel Family',       primaryContact: 'Priya Patel',        email: 'ppatel@example.com',  swimmerCount: 3, status: 'active',    balance: 500.00,   joinedAt: '2022-01-05' },
  { id: 'hh-009', name: 'Müller Household',   primaryContact: 'Hans Müller',        email: 'hmuller@example.com', swimmerCount: 2, status: 'active',    balance: -120.00, joinedAt: '2023-07-22' },
  { id: 'hh-010', name: 'Jackson Family',     primaryContact: 'Denise Jackson',     email: 'djackson@example.com',swimmerCount: 2, status: 'suspended', balance: -1200.00,joinedAt: '2022-11-30' },
  { id: 'hh-011', name: 'Tanaka Household',   primaryContact: 'Keiko Tanaka',       email: 'ktanaka@example.com', swimmerCount: 1, status: 'active',    balance: 75.00,    joinedAt: '2024-02-01' },
  { id: 'hh-012', name: 'O\'Brien Family',    primaryContact: 'Patrick O\'Brien',   email: 'pobrien@example.com', swimmerCount: 3, status: 'active',    balance: -350.00, joinedAt: '2021-08-15' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

const STATUS_CONFIG: Record<HouseholdStatus, { label: string; color: string; bg: string }> = {
  active:    { label: 'Active',    color: 'var(--success)',  bg: 'var(--success-bg)' },
  inactive:  { label: 'Inactive',  color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  pending:   { label: 'Pending',   color: 'var(--warning)',  bg: 'var(--warning-bg)' },
  suspended: { label: 'Suspended', color: 'var(--error)',    bg: 'var(--error-bg)' },
}

function StatusBadge({ status }: { status: HouseholdStatus }) {
  const cfg = STATUS_CONFIG[status]
  return (
    <span style={{
      fontSize: '11px',
      fontWeight: 600,
      padding: '2px 8px',
      borderRadius: '10px',
      background: cfg.bg,
      color: cfg.color,
      fontFamily: 'var(--font-mono)',
      letterSpacing: '0.02em',
      textTransform: 'uppercase',
    }}>
      {cfg.label}
    </span>
  )
}

function formatBalance(cents: number) {
  const abs = Math.abs(cents)
  const formatted = abs.toLocaleString('en-US', { style: 'currency', currency: 'USD' })
  if (cents < 0) return { text: `-${formatted}`, color: 'var(--error)' }
  if (cents > 0) return { text: `+${formatted}`, color: 'var(--success)' }
  return { text: '—', color: 'var(--text-muted)' }
}

// ─── Columns ──────────────────────────────────────────────────────────────────

const COLUMNS: Column<Household>[] = [
  {
    key: 'name',
    label: 'Household',
    sortable: true,
    render: (row) => (
      <div>
        <Link
          href={`/members/households/${row.id}`}
          style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}
        >
          {row.name}
        </Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px', fontFamily: 'var(--font-mono)' }}>
          {row.id}
        </div>
      </div>
    ),
  },
  {
    key: 'primaryContact',
    label: 'Primary Contact',
    sortable: true,
    render: (row) => (
      <div>
        <div style={{ fontSize: '13px', color: 'var(--text)' }}>{row.primaryContact}</div>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>{row.email}</div>
      </div>
    ),
  },
  {
    key: 'swimmerCount',
    label: 'Swimmers',
    sortable: true,
    align: 'right',
    render: (row) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text)' }}>
        {row.swimmerCount}
      </span>
    ),
  },
  {
    key: 'status',
    label: 'Status',
    render: (row) => <StatusBadge status={row.status} />,
  },
  {
    key: 'balance',
    label: 'Balance',
    sortable: true,
    align: 'right',
    render: (row) => {
      const { text, color } = formatBalance(row.balance)
      return (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color, fontWeight: 600 }}>
          {text}
        </span>
      )
    },
  },
  {
    key: 'joinedAt',
    label: 'Joined',
    sortable: true,
    render: (row) => (
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>
        {new Date(row.joinedAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
      </span>
    ),
  },
  {
    key: 'id',
    label: '',
    width: '80px',
    render: (row) => (
      <Link
        href={`/members/households/${row.id}`}
        style={{
          fontSize: '12px',
          color: 'var(--text-muted)',
          textDecoration: 'none',
          display: 'flex',
          alignItems: 'center',
          gap: '4px',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}
      >
        View <Icon name="chevron-right" size={12} />
      </Link>
    ),
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function HouseholdsPage() {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')

  const filtered = useMemo(() => {
    return MOCK_HOUSEHOLDS.filter((h) => {
      const matchesSearch =
        !search ||
        h.name.toLowerCase().includes(search.toLowerCase()) ||
        h.primaryContact.toLowerCase().includes(search.toLowerCase()) ||
        h.email.toLowerCase().includes(search.toLowerCase())
      const matchesStatus = statusFilter === 'all' || h.status === statusFilter
      return matchesSearch && matchesStatus
    })
  }, [search, statusFilter])

  const counts = useMemo(() => ({
    total:     MOCK_HOUSEHOLDS.length,
    active:    MOCK_HOUSEHOLDS.filter((h) => h.status === 'active').length,
    pending:   MOCK_HOUSEHOLDS.filter((h) => h.status === 'pending').length,
    suspended: MOCK_HOUSEHOLDS.filter((h) => h.status === 'suspended').length,
  }), [])

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>

      {/* Page header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{
            fontSize: '22px',
            fontFamily: 'var(--font-display)',
            fontWeight: 600,
            color: 'var(--text)',
            lineHeight: 1.2,
            marginBottom: '4px',
          }}>
            Households
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
            Manage member households and account balances
          </p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <Icon name="plus" size={14} />
          New Household
        </button>
      </div>

      {/* Stats strip */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(4, 1fr)',
        gap: '12px',
        marginBottom: '20px',
      }}>
        {[
          { label: 'Total',     value: counts.total,     color: 'var(--text)' },
          { label: 'Active',    value: counts.active,    color: 'var(--success)' },
          { label: 'Pending',   value: counts.pending,   color: 'var(--warning)' },
          { label: 'Suspended', value: counts.suspended, color: 'var(--error)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>
              {stat.label}
            </div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>
              {stat.value}
            </div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '360px' }}>
          <Icon
            name="search"
            size={14}
            style={{
              position: 'absolute',
              left: '10px',
              top: '50%',
              transform: 'translateY(-50%)',
              color: 'var(--text-muted)',
              pointerEvents: 'none',
            }}
          />
          <input
            type="text"
            placeholder="Search households, contacts..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{
              width: '100%',
              padding: '8px 12px 8px 32px',
              background: 'var(--bg-elevated)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--r-md)',
              fontSize: '13px',
              color: 'var(--text)',
              outline: 'none',
            }}
          />
        </div>

        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          style={{
            padding: '8px 12px',
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--r-md)',
            fontSize: '13px',
            color: 'var(--text)',
            outline: 'none',
            cursor: 'pointer',
          }}
        >
          <option value="all">All Statuses</option>
          <option value="active">Active</option>
          <option value="pending">Pending</option>
          <option value="inactive">Inactive</option>
          <option value="suspended">Suspended</option>
        </select>

        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>
          {filtered.length} of {MOCK_HOUSEHOLDS.length}
        </span>
      </div>

      {/* Table */}
      {filtered.length === 0 ? (
        <EmptyState
          icon="users"
          title="No households found"
          description="Try adjusting your search or filter criteria."
        />
      ) : (
        <Table
          columns={COLUMNS}
          data={filtered}
          pageSize={10}
          emptyTitle="No households found"
          emptyDescription="Try adjusting your search or filter criteria."
        />
      )}
    </div>
  )
}
