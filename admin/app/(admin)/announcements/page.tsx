'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type AnnouncementStatus = 'draft' | 'published' | 'archived'
type Audience = 'all' | 'coaches' | 'parents' | 'swimmers' | 'admin'

type Announcement = {
  id: string
  title: string
  status: AnnouncementStatus
  audience: Audience
  pinned: boolean
  authorName: string
  authorRole: string
  bodyPreview: string
  publishedAt: string | null
  archivedAt:  string | null
  createdAt:   string
}

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const MOCK_ANNOUNCEMENTS: Announcement[] = [
  {
    id: 'ann-001', title: 'Spring Season Registration Now Open',
    status: 'published', audience: 'all', pinned: true,
    authorName: 'Coach Martinez', authorRole: 'coach',
    bodyPreview: 'Registration for the 2026 Spring Season is now open. All returning families should complete their registration by March 25th…',
    publishedAt: '2026-03-01T09:00:00Z', archivedAt: null, createdAt: '2026-02-28T18:00:00Z',
  },
  {
    id: 'ann-002', title: 'Pool Closure — March 18th (Maintenance)',
    status: 'published', audience: 'all', pinned: true,
    authorName: 'Admin User', authorRole: 'admin',
    bodyPreview: 'The Aquatic Center will be closed on Tuesday, March 18th for scheduled lane line maintenance. All practices are cancelled that day…',
    publishedAt: '2026-03-10T14:30:00Z', archivedAt: null, createdAt: '2026-03-09T10:00:00Z',
  },
  {
    id: 'ann-003', title: 'State Championships — Travel Information',
    status: 'published', audience: 'parents', pinned: false,
    authorName: 'Coach Kim', authorRole: 'coach',
    bodyPreview: 'Please review the attached travel itinerary for the State Championships in Sacramento. Carpooling sign-ups close March 20th…',
    publishedAt: '2026-03-12T08:00:00Z', archivedAt: null, createdAt: '2026-03-11T16:00:00Z',
  },
  {
    id: 'ann-004', title: 'Updated Code of Conduct — 2026',
    status: 'draft', audience: 'all', pinned: false,
    authorName: 'Admin User', authorRole: 'admin',
    bodyPreview: 'An updated Code of Conduct has been prepared for the 2026 season. Please review and provide feedback before we publish…',
    publishedAt: null, archivedAt: null, createdAt: '2026-03-14T11:00:00Z',
  },
  {
    id: 'ann-005', title: 'End-of-Season Banquet Details',
    status: 'draft', audience: 'all', pinned: false,
    authorName: 'Admin User', authorRole: 'admin',
    bodyPreview: 'The end-of-season banquet will be held on May 10th at the Community Center. RSVP details coming soon…',
    publishedAt: null, archivedAt: null, createdAt: '2026-03-15T09:30:00Z',
  },
  {
    id: 'ann-006', title: 'Winter Season — Final Practice Schedule',
    status: 'archived', audience: 'all', pinned: false,
    authorName: 'Coach Martinez', authorRole: 'coach',
    bodyPreview: 'The final practice schedule for the 2025 Winter Season is published. Please check your group assignment…',
    publishedAt: '2025-11-01T08:00:00Z', archivedAt: '2026-01-15T00:00:00Z', createdAt: '2025-10-30T12:00:00Z',
  },
  {
    id: 'ann-007', title: 'Holiday Break — Pool Hours',
    status: 'archived', audience: 'all', pinned: false,
    authorName: 'Admin User', authorRole: 'admin',
    bodyPreview: 'Pool hours during the holiday break (Dec 22 – Jan 5) will be reduced. See schedule below…',
    publishedAt: '2025-12-18T09:00:00Z', archivedAt: '2026-01-06T00:00:00Z', createdAt: '2025-12-17T10:00:00Z',
  },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

const STATUS_COLORS: Record<AnnouncementStatus, string> = {
  published: 'var(--success)',
  draft:     'var(--warning)',
  archived:  'var(--text-muted)',
}

const STATUS_BG: Record<AnnouncementStatus, string> = {
  published: 'var(--success-bg)',
  draft:     'var(--warning-bg)',
  archived:  'var(--border)',
}

const STATUS_BORDER: Record<AnnouncementStatus, string> = {
  published: 'var(--success-border)',
  draft:     'var(--warning-border)',
  archived:  'var(--border)',
}

const AUDIENCE_LABELS: Record<Audience, string> = {
  all:     'Everyone',
  coaches: 'Coaches',
  parents: 'Parents',
  swimmers:'Swimmers',
  admin:   'Admin',
}

// ─── Components ───────────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: AnnouncementStatus }) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 5,
      padding: '2px 8px', borderRadius: 4,
      fontSize: 11, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase',
      color: STATUS_COLORS[status], background: STATUS_BG[status],
      border: `1px solid ${STATUS_BORDER[status]}`,
    }}>
      <span style={{ width: 5, height: 5, borderRadius: '50%', background: STATUS_COLORS[status], display: 'inline-block' }} />
      {status}
    </span>
  )
}

function StatCard({ label, value, color }: { label: string; value: number; color?: string }) {
  return (
    <div style={{
      padding: '18px 22px', borderRadius: 10,
      background: 'var(--bg-elevated)', border: '1px solid var(--border)',
      display: 'flex', flexDirection: 'column', gap: 5,
    }}>
      <span style={{ fontSize: 11, fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-muted)' }}>{label}</span>
      <span style={{ fontSize: 28, fontWeight: 700, letterSpacing: '-0.03em', fontFamily: 'var(--font-mono)', color: color ?? 'var(--text)' }}>{value}</span>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

const STATUS_FILTERS: { value: AnnouncementStatus | ''; label: string }[] = [
  { value: '', label: 'All statuses' },
  { value: 'published', label: 'Published' },
  { value: 'draft',     label: 'Draft' },
  { value: 'archived',  label: 'Archived' },
]

export default function AnnouncementsPage() {
  const [search, setSearch]           = useState('')
  const [statusFilter, setStatusFilter] = useState<AnnouncementStatus | ''>('')

  const filtered = useMemo(() => {
    return MOCK_ANNOUNCEMENTS.filter(a => {
      if (statusFilter && a.status !== statusFilter) return false
      if (search) {
        const q = search.toLowerCase()
        if (!a.title.toLowerCase().includes(q) && !a.authorName.toLowerCase().includes(q)) return false
      }
      return true
    })
  }, [search, statusFilter])

  const published = MOCK_ANNOUNCEMENTS.filter(a => a.status === 'published').length
  const drafts    = MOCK_ANNOUNCEMENTS.filter(a => a.status === 'draft').length
  const archived  = MOCK_ANNOUNCEMENTS.filter(a => a.status === 'archived').length
  const pinned    = MOCK_ANNOUNCEMENTS.filter(a => a.pinned).length

  const columns: Column<Announcement>[] = [
    {
      key: 'title', label: 'Title', sortable: true,
      render: (row) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {row.pinned && (
            <span title="Pinned" style={{
              width: 16, height: 16, borderRadius: 3,
              background: 'var(--accent-subtle)',
              display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
            }}>
              <Icon name="activity" size={10} color="var(--accent)" />
            </span>
          )}
          <div style={{ minWidth: 0 }}>
            <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)', marginBottom: 2, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 360 }}>
              {row.title}
            </div>
            <div style={{ fontSize: 11, color: 'var(--text-muted)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 360 }}>
              {row.bodyPreview}
            </div>
          </div>
        </div>
      ),
    },
    {
      key: 'status', label: 'Status', sortable: true,
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      key: 'audience', label: 'Audience',
      render: (row) => (
        <span style={{ fontSize: 12, color: 'var(--text-subtle)', fontWeight: 500 }}>
          {AUDIENCE_LABELS[row.audience]}
        </span>
      ),
    },
    {
      key: 'authorName', label: 'Author', sortable: true,
      render: (row) => (
        <div>
          <div style={{ fontSize: 12, color: 'var(--text)' }}>{row.authorName}</div>
          <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>{row.authorRole}</div>
        </div>
      ),
    },
    {
      key: 'publishedAt', label: 'Published', sortable: true,
      render: (row) => (
        <span style={{ fontSize: 12, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
          {row.publishedAt ? formatDate(row.publishedAt) : <span style={{ color: 'var(--text-disabled)' }}>—</span>}
        </span>
      ),
    },
    {
      key: 'id', label: '',
      render: (row) => (
        <Link href={`/announcements/${row.id}`} style={{
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
    <div style={{ padding: '32px 36px', maxWidth: 1300, margin: '0 auto' }}>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 28 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Icon name="megaphone" size={16} color="var(--accent)" />
            </div>
            <h1 style={{ fontSize: 22, fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--text)', margin: 0 }}>
              Announcements
            </h1>
          </div>
          <p style={{ fontSize: 13, color: 'var(--text-muted)', margin: 0 }}>
            Broadcast messages to club members and parents
          </p>
        </div>
        <button style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '9px 16px', borderRadius: 7,
          background: 'var(--accent)', border: 'none',
          color: '#fff', fontSize: 13, fontWeight: 600, cursor: 'pointer',
        }}>
          <Icon name="plus" size={14} />
          New Announcement
        </button>
      </div>

      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 14, marginBottom: 28 }}>
        <StatCard label="Published"  value={published} color="var(--success)" />
        <StatCard label="Drafts"     value={drafts}    color="var(--warning)" />
        <StatCard label="Archived"   value={archived}  />
        <StatCard label="Pinned"     value={pinned}    color="var(--accent)" />
      </div>

      {/* Pinned announcements callout */}
      {filtered.some(a => a.pinned && a.status === 'published') && !statusFilter && !search && (
        <div style={{
          marginBottom: 20, padding: '10px 14px', borderRadius: 7,
          background: 'var(--accent-subtle)', border: '1px solid var(--accent-border)',
          display: 'flex', alignItems: 'center', gap: 8,
        }}>
          <Icon name="activity" size={13} color="var(--accent)" />
          <span style={{ fontSize: 12, color: 'var(--text-subtle)' }}>
            <strong style={{ color: 'var(--text)', fontWeight: 600 }}>{pinned} pinned</strong> announcement{pinned !== 1 ? 's' : ''} currently visible at the top of the member portal.
          </span>
        </div>
      )}

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
            placeholder="Search by title or author…"
            style={{
              width: '100%', padding: '7px 10px 7px 32px', borderRadius: 6,
              background: 'var(--bg)', border: '1px solid var(--border)',
              color: 'var(--text)', fontSize: 13, outline: 'none', boxSizing: 'border-box',
            }}
          />
        </div>
        <select
          value={statusFilter} onChange={e => setStatusFilter(e.target.value as AnnouncementStatus | '')}
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
        <EmptyState
          icon="megaphone" title="No announcements found"
          description={search || statusFilter ? 'Try adjusting your filters.' : 'Create your first announcement to get started.'}
        />
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
