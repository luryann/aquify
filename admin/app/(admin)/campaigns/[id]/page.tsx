'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type CampaignStatus = 'draft' | 'sending' | 'sent' | 'failed'
type DeliveryStatus = 'delivered' | 'failed' | 'suppressed' | 'pending' | 'opened' | 'clicked'

type DeliveryRecord = {
  id: string
  recipientEmail: string
  recipientName: string
  status: DeliveryStatus
  postmarkMessageId: string | null
  openedAt: string | null
  deliveredAt: string | null
  failureReason: string | null
}

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const MOCK_CAMPAIGN = {
  id: 'camp-001',
  subject: 'Spring 2026 Registration — Action Required',
  preheader: 'Complete your registration by March 25th to secure priority group placement.',
  status: 'sent' as CampaignStatus,
  fromName: 'DARE Aquatics',
  fromEmail: 'noreply@dareaquatics.org',
  replyTo: 'admin@dareaquatics.org',
  totalRecipients: 142,
  sentCount: 138,
  failedCount: 1,
  suppressedCount: 3,
  openCount: 94,
  clickCount: 38,
  createdBy: 'Admin User',
  createdAt: '2026-02-28T17:00:00Z',
  sentAt: '2026-03-01T09:05:00Z',
  bodyHtml: `<p>Dear {{FirstName}},</p>
<p>Spring 2026 Season registration is <strong>now open</strong>. Please complete your registration by <strong>March 25th</strong> to secure priority group placement for your swimmer(s).</p>
<h3>What to complete:</h3>
<ul>
  <li>Update swimmer and contact information</li>
  <li>Sign the 2026 Code of Conduct waiver</li>
  <li>Pay the annual registration fee ($850/household)</li>
</ul>
<p>Questions? Contact us at admin@dareaquatics.org.</p>`,
  audienceDescription: 'All active households (2025 season)',
}

const MOCK_DELIVERIES: DeliveryRecord[] = [
  { id: 'del-001', recipientEmail: 'parent.chen@email.com',       recipientName: 'David Chen',     status: 'opened',     postmarkMessageId: 'pm_aaa111', openedAt: '2026-03-01T09:24:00Z', deliveredAt: '2026-03-01T09:07:00Z', failureReason: null },
  { id: 'del-002', recipientEmail: 'parent.okafor@email.com',     recipientName: 'Grace Okafor',   status: 'clicked',    postmarkMessageId: 'pm_aaa112', openedAt: '2026-03-01T09:41:00Z', deliveredAt: '2026-03-01T09:07:00Z', failureReason: null },
  { id: 'del-003', recipientEmail: 'parent.martinez@email.com',   recipientName: 'Rosa Martinez',  status: 'opened',     postmarkMessageId: 'pm_aaa113', openedAt: '2026-03-01T11:02:00Z', deliveredAt: '2026-03-01T09:08:00Z', failureReason: null },
  { id: 'del-004', recipientEmail: 'parent.henderson@email.com',  recipientName: 'Luke Henderson', status: 'delivered',  postmarkMessageId: 'pm_aaa114', openedAt: null,                    deliveredAt: '2026-03-01T09:08:00Z', failureReason: null },
  { id: 'del-005', recipientEmail: 'parent.park@email.com',       recipientName: 'Min Park',       status: 'opened',     postmarkMessageId: 'pm_aaa115', openedAt: '2026-03-01T10:15:00Z', deliveredAt: '2026-03-01T09:08:00Z', failureReason: null },
  { id: 'del-006', recipientEmail: 'old.address@defunct.com',     recipientName: 'Unknown',        status: 'failed',     postmarkMessageId: null,        openedAt: null,                    deliveredAt: null,                    failureReason: '550 User does not exist' },
  { id: 'del-007', recipientEmail: 'optout.parent@email.com',     recipientName: 'Jamie Wilson',   status: 'suppressed', postmarkMessageId: null,        openedAt: null,                    deliveredAt: null,                    failureReason: 'Suppression list — opted out' },
  { id: 'del-008', recipientEmail: 'optout2.parent@email.com',    recipientName: 'Sam Torres',     status: 'suppressed', postmarkMessageId: null,        openedAt: null,                    deliveredAt: null,                    failureReason: 'Suppression list — hard bounce' },
  { id: 'del-009', recipientEmail: 'another.parent@email.com',    recipientName: 'Alex Kim',       status: 'delivered',  postmarkMessageId: 'pm_aaa119', openedAt: null,                    deliveredAt: '2026-03-01T09:09:00Z', failureReason: null },
  { id: 'del-010', recipientEmail: 'parent.johnson@email.com',    recipientName: 'Dana Johnson',   status: 'opened',     postmarkMessageId: 'pm_aaa120', openedAt: '2026-03-01T14:30:00Z', deliveredAt: '2026-03-01T09:09:00Z', failureReason: null },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: '2-digit', minute: '2-digit', hour12: true,
  })
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

const DELIVERY_COLORS: Record<DeliveryStatus, string> = {
  opened:     'var(--success)',
  clicked:    'var(--accent)',
  delivered:  'var(--info)',
  failed:     'var(--error)',
  suppressed: 'var(--text-muted)',
  pending:    'var(--warning)',
}

const DELIVERY_BG: Record<DeliveryStatus, string> = {
  opened:     'var(--success-bg)',
  clicked:    'var(--accent-subtle)',
  delivered:  'var(--info-bg)',
  failed:     'var(--error-bg)',
  suppressed: 'var(--border)',
  pending:    'var(--warning-bg)',
}

// ─── Components ───────────────────────────────────────────────────────────────

function CampaignStatusBadge({ status }: { status: CampaignStatus }) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 5,
      padding: '3px 10px', borderRadius: 5,
      fontSize: 11, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase',
      color: STATUS_COLORS[status], background: STATUS_BG[status],
      border: `1px solid ${STATUS_BORDER[status]}`,
    }}>
      <span style={{ width: 5, height: 5, borderRadius: '50%', background: STATUS_COLORS[status], display: 'inline-block' }} />
      {status}
    </span>
  )
}

function DeliveryBadge({ status }: { status: DeliveryStatus }) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 4,
      padding: '2px 7px', borderRadius: 4,
      fontSize: 11, fontWeight: 600, letterSpacing: '0.03em', textTransform: 'uppercase',
      color: DELIVERY_COLORS[status],
      background: DELIVERY_BG[status],
    }}>
      {status}
    </span>
  )
}

function StatCard({ label, value, sub, color }: { label: string; value: string | number; sub?: string; color?: string }) {
  return (
    <div style={{
      padding: '18px 22px', borderRadius: 10,
      background: 'var(--bg-elevated)', border: '1px solid var(--border)',
      display: 'flex', flexDirection: 'column', gap: 5,
    }}>
      <span style={{ fontSize: 11, fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--text-muted)' }}>{label}</span>
      <span style={{ fontSize: 26, fontWeight: 700, letterSpacing: '-0.03em', fontFamily: 'var(--font-mono)', color: color ?? 'var(--text)' }}>{value}</span>
      {sub && <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{sub}</span>}
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function CampaignDetailPage() {
  const camp = MOCK_CAMPAIGN
  const [activeTab, setActiveTab] = useState('overview')

  const deliveryPct = camp.totalRecipients > 0 ? Math.round((camp.sentCount / camp.totalRecipients) * 100) : 0
  const openRate    = camp.sentCount > 0 ? Math.round((camp.openCount / camp.sentCount) * 100) : 0

  const tabs = [
    { id: 'overview',   label: 'Overview' },
    { id: 'deliveries', label: 'Deliveries', count: MOCK_DELIVERIES.length },
    { id: 'preview',    label: 'Preview' },
  ]

  const columns: Column<DeliveryRecord>[] = [
    {
      key: 'recipientName', label: 'Recipient', sortable: true,
      render: (row) => (
        <div>
          <div style={{ fontSize: 13, fontWeight: 500, color: 'var(--text)', marginBottom: 1 }}>{row.recipientName}</div>
          <div style={{ fontSize: 11, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>{row.recipientEmail}</div>
        </div>
      ),
    },
    {
      key: 'status', label: 'Status', sortable: true,
      render: (row) => <DeliveryBadge status={row.status} />,
    },
    {
      key: 'deliveredAt', label: 'Delivered', sortable: true,
      render: (row) => (
        <span style={{ fontSize: 12, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
          {row.deliveredAt ? formatDateTime(row.deliveredAt) : <span style={{ color: 'var(--text-disabled)' }}>—</span>}
        </span>
      ),
    },
    {
      key: 'openedAt', label: 'Opened', sortable: true,
      render: (row) => (
        <span style={{ fontSize: 12, color: row.openedAt ? 'var(--success)' : 'var(--text-disabled)', fontFamily: 'var(--font-mono)' }}>
          {row.openedAt ? formatDateTime(row.openedAt) : '—'}
        </span>
      ),
    },
    {
      key: 'postmarkMessageId', label: 'Message ID',
      render: (row) => (
        row.postmarkMessageId
          ? <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>{row.postmarkMessageId}</span>
          : row.failureReason
            ? <span style={{ fontSize: 11, color: 'var(--error)' }}>{row.failureReason}</span>
            : <span style={{ color: 'var(--text-disabled)', fontSize: 11 }}>—</span>
      ),
    },
  ]

  return (
    <div style={{ padding: '32px 36px', maxWidth: 1100, margin: '0 auto' }}>

      {/* Breadcrumb */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 20 }}>
        <Link href="/campaigns" style={{ fontSize: 12, color: 'var(--text-muted)', textDecoration: 'none' }}>
          Email Campaigns
        </Link>
        <Icon name="chevron-right" size={12} color="var(--text-disabled)" />
        <span style={{ fontSize: 12, color: 'var(--text-subtle)' }}>{camp.subject}</span>
      </div>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16, marginBottom: 20 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ marginBottom: 8 }}>
            <CampaignStatusBadge status={camp.status} />
          </div>
          <h1 style={{ fontSize: 22, fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--text)', margin: '0 0 8px' }}>
            {camp.subject}
          </h1>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 14 }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: 'var(--text-muted)' }}>
              <Icon name="mail" size={12} />
              {camp.fromName} &lt;{camp.fromEmail}&gt;
            </span>
            <span style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: 'var(--text-muted)' }}>
              <Icon name="user" size={12} />
              {camp.createdBy}
            </span>
            {camp.sentAt && (
              <span style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: 'var(--text-muted)' }}>
                <Icon name="clock" size={12} />
                Sent {formatDateTime(camp.sentAt)}
              </span>
            )}
          </div>
        </div>
        {camp.status === 'draft' && (
          <button style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '9px 18px', borderRadius: 7,
            background: 'var(--accent)', border: 'none',
            color: '#fff', fontSize: 13, fontWeight: 600, cursor: 'pointer', flexShrink: 0,
          }}>
            <Icon name="mail" size={14} /> Send Campaign
          </button>
        )}
      </div>

      {/* Delivery progress bar */}
      {camp.status !== 'draft' && (
        <div style={{ marginBottom: 24 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>Delivery progress</span>
            <span style={{ fontSize: 12, fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
              {camp.sentCount}/{camp.totalRecipients} delivered ({deliveryPct}%)
            </span>
          </div>
          <div style={{ height: 5, borderRadius: 3, background: 'var(--border)', overflow: 'hidden' }}>
            <div style={{
              height: '100%', borderRadius: 3,
              width: `${deliveryPct}%`,
              background: 'linear-gradient(90deg, var(--accent), var(--success))',
              transition: 'width 800ms ease',
            }} />
          </div>
        </div>
      )}

      {/* Stats */}
      {camp.status !== 'draft' && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 14, marginBottom: 28 }}>
          <StatCard label="Total Recipients" value={camp.totalRecipients} />
          <StatCard label="Delivered"        value={camp.sentCount}     color="var(--success)"  sub={`${deliveryPct}% delivery rate`} />
          <StatCard label="Opened"           value={camp.openCount}     color="var(--accent)"   sub={`${openRate}% open rate`} />
          <StatCard label="Suppressed"       value={camp.suppressedCount} />
        </div>
      )}

      {/* Tabs */}
      <div style={{ marginBottom: 24 }}>
        <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />
      </div>

      {/* Tab: Overview */}
      {activeTab === 'overview' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <div style={{
            padding: '24px', borderRadius: 10,
            background: 'var(--bg-elevated)', border: '1px solid var(--border)',
          }}>
            <h3 style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', margin: '0 0 16px' }}>
              Campaign Details
            </h3>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px 24px' }}>
              {[
                ['Subject', camp.subject],
                ['Preheader', camp.preheader],
                ['From Name', camp.fromName],
                ['From Email', camp.fromEmail],
                ['Reply-To', camp.replyTo],
                ['Audience', camp.audienceDescription],
                ['Created By', camp.createdBy],
                ['Created At', formatDateTime(camp.createdAt)],
                ...(camp.sentAt ? [['Sent At', formatDateTime(camp.sentAt)]] : []),
              ].map(([label, value]) => (
                <div key={label}>
                  <div style={{ fontSize: 11, color: 'var(--text-muted)', marginBottom: 3 }}>{label}</div>
                  <div style={{ fontSize: 13, color: 'var(--text-subtle)' }}>{value}</div>
                </div>
              ))}
            </div>
          </div>

          {camp.status !== 'draft' && (
            <div style={{
              padding: '24px', borderRadius: 10,
              background: 'var(--bg-elevated)', border: '1px solid var(--border)',
            }}>
              <h3 style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', margin: '0 0 16px' }}>
                Delivery Summary
              </h3>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 12 }}>
                {[
                  { label: 'Total', value: camp.totalRecipients, color: 'var(--text)' },
                  { label: 'Sent',  value: camp.sentCount,       color: 'var(--success)' },
                  { label: 'Opened',value: camp.openCount,       color: 'var(--accent)' },
                  { label: 'Clicked',value: camp.clickCount,     color: 'var(--info)' },
                  { label: 'Failed', value: camp.failedCount,    color: camp.failedCount > 0 ? 'var(--error)' : 'var(--text-muted)' },
                ].map(s => (
                  <div key={s.label} style={{ textAlign: 'center', padding: '12px 8px', borderRadius: 8, background: 'var(--bg)', border: '1px solid var(--border)' }}>
                    <div style={{ fontSize: 22, fontWeight: 700, fontFamily: 'var(--font-mono)', color: s.color }}>{s.value}</div>
                    <div style={{ fontSize: 11, color: 'var(--text-muted)', marginTop: 3 }}>{s.label}</div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Tab: Deliveries */}
      {activeTab === 'deliveries' && (
        MOCK_DELIVERIES.length === 0
          ? <EmptyState icon="mail" title="No delivery records" description="Send the campaign to see delivery tracking." />
          : <Table
              data={MOCK_DELIVERIES}
              columns={columns}
              keyField="id"
              defaultSort={{ key: 'deliveredAt', direction: 'desc' }}
              paginated
              pageSize={20}
            />
      )}

      {/* Tab: Preview */}
      {activeTab === 'preview' && (
        <div style={{
          borderRadius: 10, overflow: 'hidden',
          border: '1px solid var(--border)',
        }}>
          <div style={{
            padding: '14px 20px',
            background: 'var(--bg-elevated)',
            borderBottom: '1px solid var(--border)',
            display: 'flex', gap: 16, alignItems: 'center',
          }}>
            <div style={{ display: 'flex', gap: 6 }}>
              {['#ff5f57', '#febc2e', '#28c840'].map(c => (
                <div key={c} style={{ width: 10, height: 10, borderRadius: '50%', background: c }} />
              ))}
            </div>
            <span style={{ fontSize: 12, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
              {camp.subject}
            </span>
          </div>
          <iframe
            srcDoc={camp.bodyHtml}
            sandbox="allow-same-origin"
            title="Email preview"
            style={{
              width: '100%', border: 'none',
              minHeight: 400, background: '#fff',
              fontSize: 14,
            }}
          />
        </div>
      )}

    </div>
  )
}
