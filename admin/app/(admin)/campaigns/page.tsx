'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type CampaignStatus = 'draft' | 'sending' | 'sent' | 'failed'

type Campaign = {
  id: string
  subject: string
  status: CampaignStatus
  fromName: string
  fromEmail: string
  totalRecipients: number
  sentCount: number
  failedCount: number
  suppressedCount: number
  openCount: number
  createdBy: string
  createdAt: string
  sentAt: string | null
}

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const MOCK_CAMPAIGNS: Campaign[] = [
  {
    id: 'camp-001', subject: 'Spring 2026 Registration — Action Required',
    status: 'sent', fromName: 'DARE Aquatics', fromEmail: 'noreply@dareaquatics.org',
    totalRecipients: 142, sentCount: 138, failedCount: 1, suppressedCount: 3, openCount: 94,
    createdBy: 'Admin User', createdAt: '2026-02-28T17:00:00Z', sentAt: '2026-03-01T09:05:00Z',
  },
  {
    id: 'camp-002', subject: 'Pool Closure Notice — March 18',
    status: 'sent', fromName: 'DARE Aquatics', fromEmail: 'noreply@dareaquatics.org',
    totalRecipients: 142, sentCount: 140, failedCount: 0, suppressedCount: 2, openCount: 118,
    createdBy: 'Admin User', createdAt: '2026-03-09T09:00:00Z', sentAt: '2026-03-10T08:00:00Z',
  },
  {
    id: 'camp-003', subject: 'State Championships — Parent Travel Packet',
    status: 'sent', fromName: 'Coach Kim', fromEmail: 'coach.kim@dareaquatics.org',
    totalRecipients: 48, sentCount: 46, failedCount: 0, suppressedCount: 2, openCount: 41,
    createdBy: 'Coach Kim', createdAt: '2026-03-11T15:00:00Z', sentAt: '2026-03-12T07:30:00Z',
  },
  {
    id: 'camp-004', subject: 'End-of-Season Banquet — Save the Date',
    status: 'draft', fromName: 'DARE Aquatics', fromEmail: 'noreply@dareaquatics.org',
    totalRecipients: 0, sentCount: 0, failedCount: 0, suppressedCount: 0, openCount: 0,
    createdBy: 'Admin User', createdAt: '2026-03-15T09:00:00Z', sentAt: null,
  },
  {
    id: 'camp-005', subject: 'Annual Fundraiser — Volunteer Sign-Up',
    status: 'draft', fromName: 'DARE Aquatics', fromEmail: 'noreply@dareaquatics.org',
    totalRecipients: 0, sentCount: 0, failedCount: 0, suppressedCount: 0, openCount: 0,
    createdBy: 'Admin User', createdAt: '2026-03-15T11:00:00Z', sentAt: null,
  },
  {
    id: 'camp-006', subject: 'Winter Classic Results — Congrats!',
    status: 'sent', fromName: 'Coach Martinez', fromEmail: 'coach.martinez@dareaquatics.org',
    totalRecipients: 89, sentCount: 87, failedCount: 2, suppressedCount: 0, openCount: 72,
    createdBy: 'Coach Martinez', createdAt: '2026-03-08T16:00:00Z', sentAt: '2026-03-08T17:00:00Z',
  },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

const STATUS_COLORS: Record<CampaignStatus, string> = {
  sent:    'var(--success)',
  sending: 'var(--info)',
  draft:   'var(--warning)',
  failed:  'var(--error)',
}

const STATUS_BG: Record<CampaignStatus, string> = {
  sent:    'var(--success-bg)',
  sending: 'var(--info-bg)',
  draft:   'var(--warning-bg)',
  failed:  'var(--error-bg)',
}

const STATUS_BORDER: Record<CampaignStatus, string> = {
  sent:    'var(--success-border)',
  sending: 'var(--info-border)',
  draft:   'var(--warning-border)',
  failed:  'var(--error-border)',
}

// ─── Components ───────────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: CampaignStatus }) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 5,
      padding: '2px 8px', borderRadius: 4,
      fontSize: 11, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase',
      color: STATUS_COLORS[status],
      background: STATUS_BG[status],
      border: `1px solid ${STATUS_BORDER[status]}`,
    }}>
      {status === 'sending' && (
        <span style={{ width: 5, height: 5, borderRadius: '50%', background: STATUS_COLORS[status], animation: 'pulse 1.2s infinite', display: 'inline-block' }} />
      )}
      {status !== 'sending' && (
        <span style={{ width: 5, height: 5, borderRadius: '50%', background: STATUS_COLORS[status], display: 'inline-block' }} />
      )}
      {status}
    </span>
  )
}

function DeliveryRate({ sent, total }: { sent: number; total: number }) {
  if (total === 0) return <span style={{ color: 'var(--text-disabled)', fontSize: 12 }}>—</span>
  const pct = Math.round((sent / total) * 100)
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 100 }}>
      <div style={{
        flex: 1, height: 4, borderRadius: 2,
        background: 'var(--border)', overflow: 'hidden',
      }}>
        <div style={{
          height: '100%', borderRadius: 2, width: `${pct}%`,
          background: pct === 100 ? 'var(--success)' : pct > 80 ? 'var(--accent)' : 'var(--warning)',
          transition: 'width 600ms ease',
        }} />
      </div>
      <span style={{ fontSize: 12, fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)', flexShrink: 0 }}>
        {pct}%
      </span>
    </div>
  )
}

function StatCard({ label, value, sub }: { label: string; value: string | number; sub?: string }) {
  return (
    <div style={{
      padding: '18px 22px', borderRadius: 10,
      background: 'var(--bg-elevated)', border: '1px solid var(--border)',
      display: 'flex', flexDirection: 'column', gap: 5,
    }}>
      <span style={{ fontSize: 11, fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-muted)' }}>{label}</span>
      <span style={{ fontSize: 28, fontWeight: 700, letterSpacing: '-0.03em', fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>{value}</span>
      {sub && <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{sub}</span>}
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

const STATUS_FILTERS: { value: CampaignStatus | ''; label: string }[] = [
  { value: '',       label: 'All statuses' },
  { value: 'sent',   label: 'Sent' },
  { value: 'sending',label: 'Sending' },
  { value: 'draft',  label: 'Draft' },
  { value: 'failed', label: 'Failed' },
]

export default function CampaignsPage() {
  const [search, setSearch]             = useState('')
  const [statusFilter, setStatusFilter] = useState<CampaignStatus | ''>('')

  const filtered = useMemo(() => {
    return MOCK_CAMPAIGNS.filter(c => {
      if (statusFilter && c.status !== statusFilter) return false
      if (search) {
        const q = search.toLowerCase()
        if (!c.subject.toLowerCase().includes(q) && !c.fromName.toLowerCase().includes(q)) return false
      }
      return true
    })
  }, [search, statusFilter])

  const sentCampaigns  = MOCK_CAMPAIGNS.filter(c => c.status === 'sent').length
  const totalDelivered = MOCK_CAMPAIGNS.reduce((s, c) => s + c.sentCount, 0)
  const totalFailed    = MOCK_CAMPAIGNS.reduce((s, c) => s + c.failedCount, 0)
  const totalSuppressed= MOCK_CAMPAIGNS.reduce((s, c) => s + c.suppressedCount, 0)

  const columns: Column<Campaign>[] = [
    {
      key: 'subject', label: 'Subject', sortable: true,
      render: (row) => (
        <div>
          <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)', marginBottom: 2 }}>
            {row.subject}
          </div>
          <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>
            From: {row.fromName} &lt;{row.fromEmail}&gt;
          </div>
        </div>
      ),
    },
    {
      key: 'status', label: 'Status', sortable: true,
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      key: 'totalRecipients', label: 'Recipients', sortable: true,
      render: (row) => (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, color: row.totalRecipients > 0 ? 'var(--text)' : 'var(--text-disabled)' }}>
          {row.totalRecipients > 0 ? row.totalRecipients.toLocaleString() : '—'}
        </span>
      ),
    },
    {
      key: 'sentCount', label: 'Delivered', sortable: true,
      render: (row) => <DeliveryRate sent={row.sentCount} total={row.totalRecipients} />,
    },
    {
      key: 'openCount', label: 'Opens', sortable: true,
      render: (row) => {
        if (row.totalRecipients === 0) return <span style={{ color: 'var(--text-disabled)', fontSize: 12 }}>—</span>
        const pct = row.sentCount > 0 ? Math.round((row.openCount / row.sentCount) * 100) : 0
        return (
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-subtle)' }}>
            {row.openCount} <span style={{ color: 'var(--text-muted)' }}>({pct}%)</span>
          </span>
        )
      },
    },
    {
      key: 'createdBy', label: 'Created by',
      render: (row) => (
        <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{row.createdBy}</span>
      ),
    },
    {
      key: 'sentAt', label: 'Sent', sortable: true,
      render: (row) => (
        <span style={{ fontSize: 12, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
          {row.sentAt ? formatDate(row.sentAt) : <span style={{ color: 'var(--text-disabled)' }}>—</span>}
        </span>
      ),
    },
    {
      key: 'id', label: '',
      render: (row) => (
        <Link href={`/campaigns/${row.id}`} style={{
          display: 'inline-flex', alignItems: 'center', gap: 4,
          padding: '4px 10px', borderRadius: 5,
          background: 'var(--bg)', border: '1px solid var(--border)',
          color: 'var(--text-subtle)', fontSize: 11, fontWeight: 500,
          textDecoration: 'none', whiteSpace: 'nowrap',
        }}>
          View <Icon name="chevron-right" size={11} />
        </Link>
      ),
    },
  ]

  return (
    <div style={{ padding: '32px 36px', maxWidth: 1400, margin: '0 auto' }}>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 28 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Icon name="mail" size={16} color="var(--accent)" />
            </div>
            <h1 style={{ fontSize: 22, fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--text)', margin: 0 }}>
              Email Campaigns
            </h1>
          </div>
          <p style={{ fontSize: 13, color: 'var(--text-muted)', margin: 0 }}>
            Broadcast emails via Postmark — {MOCK_CAMPAIGNS.length} campaigns total
          </p>
        </div>
        <button style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '9px 16px', borderRadius: 7,
          background: 'var(--accent)', border: 'none',
          color: '#fff', fontSize: 13, fontWeight: 600, cursor: 'pointer',
        }}>
          <Icon name="plus" size={14} /> New Campaign
        </button>
      </div>

      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 14, marginBottom: 28 }}>
        <StatCard label="Sent Campaigns" value={sentCampaigns} />
        <StatCard label="Total Delivered" value={totalDelivered.toLocaleString()} sub="across all campaigns" />
        <StatCard label="Total Failed"    value={totalFailed}   sub="delivery failures" />
        <StatCard label="Suppressed"      value={totalSuppressed} sub="opted out / bounced" />
      </div>

      {/* Filters */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20,
        padding: '14px 16px', borderRadius: 9,
        background: 'var(--bg-elevated)', border: '1px solid var(--border)',
      }}>
        <div style={{ position: 'relative', flex: 1, minWidth: 0 }}>
          <Icon name="search" size={14} color="var(--text-muted)"
            style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' }} />
          <input
            value={search} onChange={e => setSearch(e.target.value)}
            placeholder="Search by subject or sender…"
            style={{
              width: '100%', padding: '7px 10px 7px 32px', borderRadius: 6,
              background: 'var(--bg)', border: '1px solid var(--border)',
              color: 'var(--text)', fontSize: 13, outline: 'none', boxSizing: 'border-box',
            }}
          />
        </div>
        <select
          value={statusFilter} onChange={e => setStatusFilter(e.target.value as CampaignStatus | '')}
          style={{
            padding: '7px 10px', borderRadius: 6,
            background: 'var(--bg)', border: '1px solid var(--border)',
            color: statusFilter ? 'var(--text)' : 'var(--text-muted)', fontSize: 13, cursor: 'pointer',
          }}
        >
          {STATUS_FILTERS.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
        </select>
        {(search || statusFilter) && (
          <button
            onClick={() => { setSearch(''); setStatusFilter('') }}
            style={{
              padding: '7px 12px', borderRadius: 6,
              background: 'transparent', border: '1px solid var(--border)',
              color: 'var(--text-muted)', fontSize: 12, cursor: 'pointer',
            }}
          >
            Clear
          </button>
        )}
      </div>

      {/* Table */}
      {filtered.length === 0 ? (
        <EmptyState icon="mail" title="No campaigns found" description="Try adjusting your filters or create your first campaign." />
      ) : (
        <Table
          data={filtered}
          columns={columns}
          keyField="id"
          defaultSort={{ key: 'createdAt', direction: 'desc' }}
          paginated
          pageSize={15}
        />
      )}
    </div>
  )
}
