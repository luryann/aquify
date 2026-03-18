'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'

// ─── Types ────────────────────────────────────────────────────────────────────

type AnnouncementStatus = 'draft' | 'published' | 'archived'
type Audience = 'all' | 'coaches' | 'parents' | 'swimmers' | 'admin'

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const MOCK_ANNOUNCEMENT = {
  id: 'ann-001',
  title: 'Spring Season Registration Now Open',
  status: 'published' as AnnouncementStatus,
  audience: 'all' as Audience,
  pinned: true,
  authorName: 'Coach Martinez',
  authorRole: 'coach',
  body: `Registration for the 2026 Spring Season is now open. All returning families should complete their registration by **March 25th** to lock in priority group placement.

**What you need to complete:**
- Update swimmer contact information
- Sign the 2026 Code of Conduct waiver
- Pay the annual registration fee ($850 per household)

**New for 2026:**
We've added an optional equipment package this year, which includes two practice suits, a team cap, and a mesh gear bag. This can be added during registration checkout.

If you have any questions, please contact the front office at admin@dareaquatics.org or speak with your group's coach at the next practice.

We look forward to an amazing season!

— Coach Martinez & the DARE Coaching Staff`,
  publishedAt: '2026-03-01T09:00:00Z',
  archivedAt:  null,
  createdAt:   '2026-02-28T18:00:00Z',
  updatedAt:   '2026-03-01T08:55:00Z',
  notifyViaPush:  true,
  notifyViaEmail: false,
  groups: ['Senior Elite', 'Senior Prep', 'Junior A', 'Junior B', 'Age Group'],
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', {
    weekday: 'long', month: 'long', day: 'numeric', year: 'numeric',
  })
}

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString('en-US', {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: '2-digit', minute: '2-digit', hour12: true,
  })
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
  all:     'All Members',
  coaches: 'Coaches Only',
  parents: 'Parents Only',
  swimmers:'Swimmers Only',
  admin:   'Admin Only',
}

// ─── Components ───────────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: AnnouncementStatus }) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 5,
      padding: '3px 10px', borderRadius: 5,
      fontSize: 11, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase',
      color: STATUS_COLORS[status],
      background: STATUS_BG[status],
      border: `1px solid ${STATUS_BORDER[status]}`,
    }}>
      <span style={{ width: 5, height: 5, borderRadius: '50%', background: STATUS_COLORS[status], display: 'inline-block' }} />
      {status}
    </span>
  )
}

function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <label style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', cursor: 'pointer', gap: 12 }}>
      <span style={{ fontSize: 13, color: 'var(--text-subtle)' }}>{label}</span>
      <input
        type="checkbox"
        checked={checked}
        onChange={e => onChange(e.target.checked)}
        style={{ position: 'absolute', opacity: 0, width: 0, height: 0 }}
      />
      <div
        role="switch"
        aria-checked={checked}
        tabIndex={0}
        onKeyDown={e => { if (e.key === ' ' || e.key === 'Enter') { e.preventDefault(); onChange(!checked) } }}
        style={{
          width: 36, height: 20, borderRadius: 10, position: 'relative', flexShrink: 0,
          background: checked ? 'var(--accent)' : 'var(--border)',
          transition: 'background 200ms', cursor: 'pointer', outline: 'none',
        }}
      >
        <div style={{
          position: 'absolute', top: 2, left: checked ? 18 : 2,
          width: 16, height: 16, borderRadius: '50%',
          background: '#fff', transition: 'left 200ms',
          boxShadow: '0 1px 3px rgba(0,0,0,0.3)',
        }} />
      </div>
    </label>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function AnnouncementDetailPage() {
  const ann = MOCK_ANNOUNCEMENT
  const [activeTab, setActiveTab] = useState('content')
  const [pinned, setPinned]             = useState(ann.pinned)
  const [notifyPush, setNotifyPush]     = useState(ann.notifyViaPush)
  const [notifyEmail, setNotifyEmail]   = useState(ann.notifyViaEmail)
  const [status, setStatus]             = useState<AnnouncementStatus>(ann.status)

  const tabs = [
    { id: 'content', label: 'Content' },
    { id: 'settings', label: 'Settings' },
  ]

  function handlePublish() { setStatus('published') }
  function handleArchive() { setStatus('archived') }

  return (
    <div style={{ padding: '32px 36px', maxWidth: 900, margin: '0 auto' }}>

      {/* Breadcrumb */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 20 }}>
        <Link href="/announcements" style={{ fontSize: 12, color: 'var(--text-muted)', textDecoration: 'none' }}>
          Announcements
        </Link>
        <Icon name="chevron-right" size={12} color="var(--text-disabled)" />
        <span style={{ fontSize: 12, color: 'var(--text-subtle)' }}>{ann.title}</span>
      </div>

      {/* Header */}
      <div style={{ marginBottom: 28 }}>
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16, marginBottom: 12 }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8, flexWrap: 'wrap' }}>
              <StatusBadge status={status} />
              {pinned && (
                <span style={{
                  display: 'inline-flex', alignItems: 'center', gap: 4,
                  padding: '2px 8px', borderRadius: 4, fontSize: 11, fontWeight: 600,
                  color: 'var(--accent)', background: 'var(--accent-subtle)',
                  border: '1px solid var(--accent-border)',
                }}>
                  <Icon name="activity" size={10} /> Pinned
                </span>
              )}
            </div>
            <h1 style={{ fontSize: 24, fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--text)', margin: '0 0 10px' }}>
              {ann.title}
            </h1>
            <div style={{ display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}>
              <span style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: 'var(--text-muted)' }}>
                <Icon name="user" size={12} />
                {ann.authorName} <span style={{ color: 'var(--text-disabled)' }}>({ann.authorRole})</span>
              </span>
              <span style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: 'var(--text-muted)' }}>
                <Icon name="users" size={12} />
                {AUDIENCE_LABELS[ann.audience]}
              </span>
              {ann.publishedAt && (
                <span style={{ display: 'flex', alignItems: 'center', gap: 5, fontSize: 12, color: 'var(--text-muted)' }}>
                  <Icon name="clock" size={12} />
                  Published {formatDateTime(ann.publishedAt)}
                </span>
              )}
            </div>
          </div>

          {/* Actions */}
          <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
            {status === 'draft' && (
              <>
                <button
                  onClick={handlePublish}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 6,
                    padding: '8px 16px', borderRadius: 7,
                    background: 'var(--accent)', border: 'none',
                    color: '#fff', fontSize: 13, fontWeight: 600, cursor: 'pointer',
                  }}
                >
                  <Icon name="check-circle" size={14} /> Publish
                </button>
                <button
                  onClick={handleArchive}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 6,
                    padding: '8px 14px', borderRadius: 7,
                    background: 'var(--bg-elevated)', border: '1px solid var(--border)',
                    color: 'var(--text-subtle)', fontSize: 13, cursor: 'pointer',
                  }}
                >
                  Archive
                </button>
              </>
            )}
            {status === 'published' && (
              <>
                <button style={{
                  display: 'flex', alignItems: 'center', gap: 6,
                  padding: '8px 14px', borderRadius: 7,
                  background: 'var(--bg-elevated)', border: '1px solid var(--border)',
                  color: 'var(--text-subtle)', fontSize: 13, cursor: 'pointer',
                }}>
                  <Icon name="edit-2" size={13} /> Edit
                </button>
                <button
                  onClick={handleArchive}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 6,
                    padding: '8px 14px', borderRadius: 7,
                    background: 'var(--bg-elevated)', border: '1px solid var(--border)',
                    color: 'var(--text-muted)', fontSize: 13, cursor: 'pointer',
                  }}
                >
                  Archive
                </button>
              </>
            )}
            {status === 'archived' && (
              <span style={{ fontSize: 12, color: 'var(--text-disabled)', padding: '8px 4px' }}>
                Archived {ann.archivedAt ? formatDateTime(ann.archivedAt) : ''}
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div style={{ marginBottom: 24 }}>
        <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />
      </div>

      {/* Tab: Content */}
      {activeTab === 'content' && (
        <div style={{
          padding: '28px 32px', borderRadius: 10,
          background: 'var(--bg-elevated)', border: '1px solid var(--border)',
        }}>
          <div style={{
            fontSize: 14, lineHeight: 1.8, color: 'var(--text-subtle)',
            whiteSpace: 'pre-wrap', fontFamily: 'var(--font-body)',
          }}>
            {ann.body.split(/\*\*(.*?)\*\*/g).map((part, i) =>
              i % 2 === 1
                ? <strong key={i} style={{ color: 'var(--text)', fontWeight: 600 }}>{part}</strong>
                : part
            )}
          </div>
          <div style={{
            marginTop: 24, paddingTop: 20, borderTop: '1px solid var(--border)',
            display: 'flex', justifyContent: 'space-between', alignItems: 'center',
          }}>
            <div style={{ display: 'flex', gap: 20 }}>
              <span style={{ fontSize: 11, color: 'var(--text-disabled)' }}>
                Created {formatDate(ann.createdAt)}
              </span>
              <span style={{ fontSize: 11, color: 'var(--text-disabled)' }}>
                Last updated {formatDateTime(ann.updatedAt)}
              </span>
            </div>
            {status !== 'archived' && (
              <button style={{
                display: 'flex', alignItems: 'center', gap: 5,
                padding: '6px 12px', borderRadius: 5,
                background: 'transparent', border: '1px solid var(--border)',
                color: 'var(--text-subtle)', fontSize: 12, cursor: 'pointer',
              }}>
                <Icon name="edit-2" size={12} /> Edit content
              </button>
            )}
          </div>
        </div>
      )}

      {/* Tab: Settings */}
      {activeTab === 'settings' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <div style={{
            padding: '24px', borderRadius: 10,
            background: 'var(--bg-elevated)', border: '1px solid var(--border)',
          }}>
            <h3 style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)', margin: '0 0 18px', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
              Audience
            </h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              <div>
                <label style={{ display: 'block', fontSize: 12, color: 'var(--text-muted)', marginBottom: 6 }}>Target audience</label>
                <select
                  defaultValue={ann.audience}
                  disabled={status === 'archived'}
                  style={{
                    padding: '8px 12px', borderRadius: 6, width: '100%', maxWidth: 300,
                    background: 'var(--bg)', border: '1px solid var(--border)',
                    color: 'var(--text)', fontSize: 13,
                    opacity: status === 'archived' ? 0.5 : 1,
                  }}
                >
                  <option value="all">All Members</option>
                  <option value="parents">Parents Only</option>
                  <option value="coaches">Coaches Only</option>
                  <option value="swimmers">Swimmers Only</option>
                  <option value="admin">Admin Only</option>
                </select>
              </div>
              <div>
                <label style={{ display: 'block', fontSize: 12, color: 'var(--text-muted)', marginBottom: 8 }}>Groups</label>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                  {ann.groups.map(g => (
                    <span key={g} style={{
                      padding: '4px 10px', borderRadius: 5,
                      background: 'var(--bg)', border: '1px solid var(--border)',
                      fontSize: 12, color: 'var(--text-subtle)',
                    }}>
                      {g}
                    </span>
                  ))}
                  <span style={{
                    padding: '4px 10px', borderRadius: 5,
                    background: 'var(--bg)', border: '1px dashed var(--border)',
                    fontSize: 12, color: 'var(--text-disabled)', cursor: 'pointer',
                  }}>
                    + All groups
                  </span>
                </div>
              </div>
            </div>
          </div>

          <div style={{
            padding: '24px', borderRadius: 10,
            background: 'var(--bg-elevated)', border: '1px solid var(--border)',
          }}>
            <h3 style={{ fontSize: 13, fontWeight: 600, color: 'var(--text)', margin: '0 0 18px', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
              Display Options
            </h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <Toggle checked={pinned} onChange={setPinned} label="Pin to top of member portal" />
              <div style={{ height: 1, background: 'var(--border)' }} />
              <Toggle checked={notifyPush} onChange={setNotifyPush} label="Send push notification when published" />
              <Toggle checked={notifyEmail} onChange={setNotifyEmail} label="Send email notification when published" />
            </div>
          </div>

          {status !== 'archived' && (
            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button style={{
                padding: '9px 20px', borderRadius: 7,
                background: 'var(--accent)', border: 'none',
                color: '#fff', fontSize: 13, fontWeight: 600, cursor: 'pointer',
              }}>
                Save Settings
              </button>
            </div>
          )}
        </div>
      )}

    </div>
  )
}
