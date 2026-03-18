'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

// ── Stub data ──────────────────────────────────────────────────────────────
const ANNOUNCEMENTS = [
  {
    id: 'ann-001',
    title: 'Season Registration Now Open',
    body: 'Registration for the 2026–2027 season is now open. Please complete your swimmer registrations and submit all required documents by April 15th to secure your spot on the team.',
    category: 'admin' as const,
    pinned: true,
    read: false,
    publishedAt: 'Mar 15, 2026',
    author: 'Club Administration',
  },
  {
    id: 'ann-002',
    title: 'Tri-City Classic — Updated Warm-up Schedule',
    body: 'Warm-up times for the Tri-City Classic have been adjusted. Session A warm-up now begins at 7:00 AM. Please plan your arrival accordingly and check the meet packet for updated heat sheets.',
    category: 'meet' as const,
    pinned: true,
    read: false,
    publishedAt: 'Mar 14, 2026',
    author: 'Head Coach Rivera',
  },
  {
    id: 'ann-003',
    title: 'Pool Closure — Spring Break',
    body: 'The facility will be closed March 28 – April 5 for spring break maintenance. Practice resumes April 6th at regular times. A voluntary open swim session will be held April 4th.',
    category: 'facility' as const,
    pinned: false,
    read: true,
    publishedAt: 'Mar 12, 2026',
    author: 'Aquatics Director',
  },
  {
    id: 'ann-004',
    title: 'Senior Group — Altitude Camp Opportunity',
    body: 'An optional altitude training camp will be held June 10–17 in Colorado Springs. This is a tremendous opportunity for Senior A and Senior B swimmers. Spots are limited. Please express interest by April 1.',
    category: 'training' as const,
    pinned: false,
    read: true,
    publishedAt: 'Mar 10, 2026',
    author: 'Head Coach Rivera',
  },
  {
    id: 'ann-005',
    title: 'Volunteer Sign-Up: Spring Invite',
    body: 'We need parent volunteers for the Spring Regional Invite on April 5th. Roles include timing, concessions, and deck setup. Each family is asked to provide at least one volunteer session.',
    category: 'volunteer' as const,
    pinned: false,
    read: true,
    publishedAt: 'Mar 8, 2026',
    author: 'Parent Committee',
  },
  {
    id: 'ann-006',
    title: 'Updated Billing Schedule for 2026–2027',
    body: 'The billing schedule for the upcoming season has been finalized. Monthly dues will be invoiced on the 1st of each month. Meet fees will be invoiced within 48 hours of entry finalization.',
    category: 'billing' as const,
    pinned: false,
    read: true,
    publishedAt: 'Mar 5, 2026',
    author: 'Club Administration',
  },
]

type NotifChannel = 'email' | 'push'
type NotifEvent = {
  id: string
  label: string
  description: string
  email: boolean
  push: boolean
}

const INITIAL_PREFS: NotifEvent[] = [
  {
    id: 'meet_commitment',
    label: 'Meet Commitment Requests',
    description: 'Notify when a new meet opens for commitment responses.',
    email: true,
    push: true,
  },
  {
    id: 'commitment_deadline',
    label: 'Commitment Deadlines',
    description: 'Remind me 48 hours before a commitment deadline closes.',
    email: true,
    push: true,
  },
  {
    id: 'invoice_issued',
    label: 'New Invoice',
    description: 'Notify when a new invoice is issued to your household.',
    email: true,
    push: false,
  },
  {
    id: 'payment_confirmed',
    label: 'Payment Confirmed',
    description: 'Confirmation email when a payment is successfully processed.',
    email: true,
    push: false,
  },
  {
    id: 'document_requested',
    label: 'Document Requests',
    description: 'Notify when a document is requested or about to expire.',
    email: true,
    push: true,
  },
  {
    id: 'announcements',
    label: 'Club Announcements',
    description: 'Receive new club-wide or group-specific announcements.',
    email: false,
    push: true,
  },
  {
    id: 'meet_updates',
    label: 'Meet Schedule Updates',
    description: 'Notify when meet details, warm-up times, or entry info changes.',
    email: true,
    push: true,
  },
  {
    id: 'entry_approved',
    label: 'Entry Approved',
    description: 'Confirmation when your swimmer\'s entries are approved by the coach.',
    email: true,
    push: false,
  },
]

const CATEGORY_CONFIG: Record<string, { label: string; color: string; bg: string; border: string }> = {
  admin:     { label: 'Admin',     color: '#6b7280', bg: '#f9fafb',  border: '#e5e7eb' },
  meet:      { label: 'Meet',      color: '#1d4ed8', bg: '#eff6ff',  border: '#bfdbfe' },
  facility:  { label: 'Facility',  color: '#7c3aed', bg: '#f5f3ff',  border: '#ddd6fe' },
  training:  { label: 'Training',  color: '#0f766e', bg: '#f0fdfa',  border: '#99f6e4' },
  volunteer: { label: 'Volunteer', color: '#b45309', bg: '#fffbeb',  border: '#fde68a' },
  billing:   { label: 'Billing',   color: '#be123c', bg: '#fff1f2',  border: '#fecdd3' },
}

// ── Components ─────────────────────────────────────────────────────────────
function AnnouncementRow({
  ann,
  onMarkRead,
}: {
  ann: typeof ANNOUNCEMENTS[number]
  onMarkRead: (id: string) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const cat = CATEGORY_CONFIG[ann.category]

  return (
    <div
      style={{
        background: '#ffffff',
        borderRadius: '12px',
        border: `1px solid ${ann.read ? 'rgba(0,0,0,0.08)' : 'rgba(235,93,30,0.25)'}`,
        padding: '18px 20px',
        boxShadow: ann.read ? '0 1px 4px rgba(0,0,0,0.04)' : '0 2px 8px rgba(235,93,30,0.08)',
        transition: 'box-shadow 150ms ease',
        animation: 'slideUp 0.3s var(--ease-out) both',
      }}
    >
      {/* Header row */}
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
        {/* Unread dot */}
        <div
          style={{
            width: '8px',
            height: '8px',
            borderRadius: '50%',
            background: ann.read ? 'transparent' : '#eb5d1e',
            flexShrink: 0,
            marginTop: '6px',
          }}
        />

        <div style={{ flex: 1, minWidth: 0 }}>
          {/* Top row: badges + pin */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '6px', flexWrap: 'wrap' }}>
            <span
              style={{
                fontSize: '10px',
                fontFamily: 'var(--font-mono)',
                fontWeight: 600,
                letterSpacing: '0.04em',
                textTransform: 'uppercase',
                color: cat.color,
                background: cat.bg,
                border: `1px solid ${cat.border}`,
                padding: '2px 7px',
                borderRadius: '999px',
              }}
            >
              {cat.label}
            </span>
            {ann.pinned && (
              <span
                style={{
                  fontSize: '10px',
                  fontFamily: 'var(--font-mono)',
                  fontWeight: 600,
                  letterSpacing: '0.04em',
                  textTransform: 'uppercase',
                  color: '#eb5d1e',
                  background: 'rgba(235,93,30,0.08)',
                  border: '1px solid rgba(235,93,30,0.20)',
                  padding: '2px 7px',
                  borderRadius: '999px',
                }}
              >
                Pinned
              </span>
            )}
            <span
              style={{
                fontSize: '11px',
                fontFamily: 'var(--font-mono)',
                color: '#A09D9B',
                marginLeft: 'auto',
              }}
            >
              {ann.publishedAt}
            </span>
          </div>

          {/* Title */}
          <h3
            style={{
              fontSize: '15px',
              fontWeight: ann.read ? 500 : 700,
              color: '#1A1714',
              marginBottom: '3px',
              lineHeight: 1.3,
            }}
          >
            {ann.title}
          </h3>
          <p style={{ fontSize: '12px', color: '#9A9693', marginBottom: '10px' }}>
            {ann.author}
          </p>

          {/* Body */}
          <p
            style={{
              fontSize: '13.5px',
              color: '#4B4846',
              lineHeight: 1.65,
              overflow: 'hidden',
              display: '-webkit-box',
              WebkitLineClamp: expanded ? 'unset' : 2,
              WebkitBoxOrient: 'vertical' as const,
            }}
          >
            {ann.body}
          </p>
        </div>
      </div>

      {/* Actions */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '10px',
          marginTop: '14px',
          paddingTop: '12px',
          borderTop: '1px solid rgba(0,0,0,0.06)',
        }}
      >
        <button
          onClick={() => setExpanded(!expanded)}
          style={{
            background: 'transparent',
            border: 'none',
            color: '#eb5d1e',
            fontSize: '13px',
            fontWeight: 600,
            cursor: 'pointer',
            padding: '4px 0',
            display: 'flex',
            alignItems: 'center',
            gap: '4px',
          }}
        >
          {expanded ? 'Collapse' : 'Read more'}
          <Icon
            name={expanded ? 'chevron-up' : 'chevron-down'}
            size={13}
          />
        </button>

        {!ann.read && (
          <button
            onClick={() => onMarkRead(ann.id)}
            style={{
              background: 'transparent',
              border: '1px solid rgba(0,0,0,0.12)',
              borderRadius: '7px',
              color: '#6B6866',
              fontSize: '12px',
              fontWeight: 500,
              cursor: 'pointer',
              padding: '4px 12px',
              display: 'flex',
              alignItems: 'center',
              gap: '5px',
              transition: 'border-color 120ms ease, color 120ms ease',
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.color = '#1A1714'
              e.currentTarget.style.borderColor = 'rgba(0,0,0,0.22)'
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.color = '#6B6866'
              e.currentTarget.style.borderColor = 'rgba(0,0,0,0.12)'
            }}
          >
            <Icon name="check" size={12} />
            Mark read
          </button>
        )}
      </div>
    </div>
  )
}

function ToggleSwitch({
  checked,
  onChange,
}: {
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      style={{
        width: '40px',
        height: '22px',
        borderRadius: '999px',
        background: checked ? '#eb5d1e' : 'rgba(0,0,0,0.12)',
        border: 'none',
        cursor: 'pointer',
        position: 'relative',
        transition: 'background 150ms ease',
        flexShrink: 0,
      }}
    >
      <span
        style={{
          position: 'absolute',
          top: '3px',
          left: checked ? '21px' : '3px',
          width: '16px',
          height: '16px',
          borderRadius: '50%',
          background: '#ffffff',
          boxShadow: '0 1px 4px rgba(0,0,0,0.20)',
          transition: 'left 150ms ease',
        }}
      />
    </button>
  )
}

function NotifPrefRow({
  pref,
  onChange,
}: {
  pref: NotifEvent
  onChange: (id: string, channel: NotifChannel, val: boolean) => void
}) {
  return (
    <div
      style={{
        background: '#ffffff',
        borderRadius: '12px',
        border: '1px solid rgba(0,0,0,0.08)',
        padding: '18px 20px',
        display: 'grid',
        gridTemplateColumns: '1fr auto auto',
        alignItems: 'center',
        gap: '20px',
        boxShadow: '0 1px 4px rgba(0,0,0,0.04)',
        animation: 'slideUp 0.3s var(--ease-out) both',
      }}
    >
      <div>
        <p style={{ fontSize: '14px', fontWeight: 600, color: '#1A1714', marginBottom: '3px' }}>
          {pref.label}
        </p>
        <p style={{ fontSize: '12.5px', color: '#6B6866', lineHeight: 1.5 }}>
          {pref.description}
        </p>
      </div>

      {/* Email toggle */}
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '4px' }}>
        <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: '#A09D9B', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
          Email
        </span>
        <ToggleSwitch
          checked={pref.email}
          onChange={(v) => onChange(pref.id, 'email', v)}
        />
      </div>

      {/* Push toggle */}
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '4px' }}>
        <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: '#A09D9B', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
          Push
        </span>
        <ToggleSwitch
          checked={pref.push}
          onChange={(v) => onChange(pref.id, 'push', v)}
        />
      </div>
    </div>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function CommunicationsPage() {
  const [tab, setTab] = useState<'inbox' | 'preferences'>('inbox')
  const [announcements, setAnnouncements] = useState(ANNOUNCEMENTS)
  const [prefs, setPrefs] = useState(INITIAL_PREFS)
  const [categoryFilter, setCategoryFilter] = useState<string>('all')
  const [showUnreadOnly, setShowUnreadOnly] = useState(false)
  const [saved, setSaved] = useState(false)

  const unreadCount = announcements.filter((a) => !a.read).length

  const markRead = (id: string) => {
    setAnnouncements((prev) =>
      prev.map((a) => (a.id === id ? { ...a, read: true } : a))
    )
  }

  const markAllRead = () => {
    setAnnouncements((prev) => prev.map((a) => ({ ...a, read: true })))
  }

  const updatePref = (id: string, channel: NotifChannel, val: boolean) => {
    setPrefs((prev) =>
      prev.map((p) => (p.id === id ? { ...p, [channel]: val } : p))
    )
    setSaved(false)
  }

  const handleSavePrefs = () => {
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  const categories = ['all', ...Array.from(new Set(announcements.map((a) => a.category)))]

  const filtered = announcements.filter((a) => {
    if (showUnreadOnly && a.read) return false
    if (categoryFilter !== 'all' && a.category !== categoryFilter) return false
    return true
  })

  return (
    <div>
      {/* Page header */}
      <div
        style={{
          marginBottom: '32px',
          animation: 'slideUp 0.4s var(--ease-out) both',
        }}
      >
        <h1
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: 'clamp(26px, 4vw, 34px)',
            color: '#1A1714',
            letterSpacing: '-0.02em',
            lineHeight: 1.1,
            marginBottom: '6px',
          }}
        >
          Communications
        </h1>
        <p style={{ fontSize: '14px', color: '#6B6866' }}>
          Club announcements and your notification preferences.
        </p>
      </div>

      {/* Tabs */}
      <div
        style={{
          display: 'flex',
          gap: '4px',
          background: 'rgba(0,0,0,0.05)',
          borderRadius: '10px',
          padding: '4px',
          marginBottom: '28px',
          width: 'fit-content',
          animation: 'slideUp 0.4s var(--ease-out) 0.05s both',
        }}
      >
        {[
          { key: 'inbox',       label: 'Inbox', count: unreadCount },
          { key: 'preferences', label: 'Notifications', count: 0 },
        ].map(({ key, label, count }) => (
          <button
            key={key}
            onClick={() => setTab(key as 'inbox' | 'preferences')}
            style={{
              padding: '7px 16px',
              borderRadius: '7px',
              border: 'none',
              background: tab === key ? '#ffffff' : 'transparent',
              color: tab === key ? '#1A1714' : '#6B6866',
              fontSize: '13.5px',
              fontWeight: tab === key ? 600 : 400,
              cursor: 'pointer',
              boxShadow: tab === key ? '0 1px 4px rgba(0,0,0,0.10)' : 'none',
              transition: 'all 150ms ease',
              display: 'flex',
              alignItems: 'center',
              gap: '7px',
            }}
          >
            {label}
            {count > 0 && (
              <span
                style={{
                  background: '#eb5d1e',
                  color: '#ffffff',
                  fontSize: '10px',
                  fontWeight: 700,
                  fontFamily: 'var(--font-mono)',
                  padding: '1px 6px',
                  borderRadius: '999px',
                  lineHeight: '16px',
                }}
              >
                {count}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* ── INBOX TAB ──────────────────────────────────────────────────── */}
      {tab === 'inbox' && (
        <div style={{ animation: 'slideUp 0.25s var(--ease-out) both' }}>
          {/* Toolbar */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '10px',
              marginBottom: '20px',
              flexWrap: 'wrap',
            }}
          >
            {/* Category filter */}
            <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
              {categories.map((cat) => (
                <button
                  key={cat}
                  onClick={() => setCategoryFilter(cat)}
                  style={{
                    padding: '5px 13px',
                    borderRadius: '999px',
                    border: `1px solid ${categoryFilter === cat ? 'rgba(235,93,30,0.40)' : 'rgba(0,0,0,0.12)'}`,
                    background: categoryFilter === cat ? 'rgba(235,93,30,0.08)' : 'transparent',
                    color: categoryFilter === cat ? '#eb5d1e' : '#6B6866',
                    fontSize: '12px',
                    fontWeight: 500,
                    cursor: 'pointer',
                    transition: 'all 120ms ease',
                    textTransform: 'capitalize',
                  }}
                >
                  {cat === 'all' ? 'All' : CATEGORY_CONFIG[cat]?.label ?? cat}
                </button>
              ))}
            </div>

            {/* Spacer */}
            <div style={{ flex: 1 }} />

            {/* Unread only toggle */}
            <button
              onClick={() => setShowUnreadOnly(!showUnreadOnly)}
              style={{
                padding: '5px 13px',
                borderRadius: '999px',
                border: `1px solid ${showUnreadOnly ? 'rgba(235,93,30,0.40)' : 'rgba(0,0,0,0.12)'}`,
                background: showUnreadOnly ? 'rgba(235,93,30,0.08)' : 'transparent',
                color: showUnreadOnly ? '#eb5d1e' : '#6B6866',
                fontSize: '12px',
                fontWeight: 500,
                cursor: 'pointer',
                transition: 'all 120ms ease',
              }}
            >
              Unread only
            </button>

            {/* Mark all read */}
            {unreadCount > 0 && (
              <button
                onClick={markAllRead}
                style={{
                  padding: '5px 13px',
                  borderRadius: '999px',
                  border: '1px solid rgba(0,0,0,0.12)',
                  background: 'transparent',
                  color: '#6B6866',
                  fontSize: '12px',
                  fontWeight: 500,
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '5px',
                  transition: 'all 120ms ease',
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.color = '#1A1714'
                  e.currentTarget.style.borderColor = 'rgba(0,0,0,0.22)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.color = '#6B6866'
                  e.currentTarget.style.borderColor = 'rgba(0,0,0,0.12)'
                }}
              >
                <Icon name="check" size={12} />
                Mark all read
              </button>
            )}
          </div>

          {/* Announcement list */}
          {filtered.length === 0 ? (
            <div
              style={{
                background: '#ffffff',
                borderRadius: '14px',
                border: '1px solid rgba(0,0,0,0.08)',
                padding: '60px 20px',
                textAlign: 'center',
              }}
            >
              <Icon name="megaphone" size={32} style={{ color: '#D0CCC9', marginBottom: '16px' }} />
              <p style={{ fontSize: '15px', color: '#9A9693', fontWeight: 500 }}>No announcements</p>
              <p style={{ fontSize: '13px', color: '#C0BBB8', marginTop: '4px' }}>
                {showUnreadOnly ? 'All caught up!' : 'Nothing to show for this filter.'}
              </p>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {filtered.map((ann, i) => (
                <div
                  key={ann.id}
                  style={{ animationDelay: `${i * 40}ms`, animation: 'slideUp 0.3s var(--ease-out) both' }}
                >
                  <AnnouncementRow ann={ann} onMarkRead={markRead} />
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* ── PREFERENCES TAB ────────────────────────────────────────────── */}
      {tab === 'preferences' && (
        <div style={{ animation: 'slideUp 0.25s var(--ease-out) both' }}>
          {/* Info banner */}
          <div
            style={{
              background: 'rgba(235,93,30,0.06)',
              border: '1px solid rgba(235,93,30,0.18)',
              borderRadius: '10px',
              padding: '14px 18px',
              display: 'flex',
              gap: '12px',
              alignItems: 'flex-start',
              marginBottom: '24px',
            }}
          >
            <Icon name="info" size={16} style={{ color: '#eb5d1e', marginTop: '1px', flexShrink: 0 }} />
            <div>
              <p style={{ fontSize: '13px', fontWeight: 600, color: '#1A1714', marginBottom: '2px' }}>
                Notification Preferences
              </p>
              <p style={{ fontSize: '12.5px', color: '#6B6866', lineHeight: 1.5 }}>
                Choose how you want to be notified for each event type. Email notifications go to your account address. Push notifications require the mobile app.
              </p>
            </div>
          </div>

          {/* Column headers */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr auto auto',
              gap: '20px',
              padding: '0 20px 8px',
            }}
          >
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#A09D9B', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              Notification type
            </span>
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#A09D9B', textTransform: 'uppercase', letterSpacing: '0.05em', textAlign: 'center', width: '60px' }}>
              Email
            </span>
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#A09D9B', textTransform: 'uppercase', letterSpacing: '0.05em', textAlign: 'center', width: '60px' }}>
              Push
            </span>
          </div>

          {/* Pref rows */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px', marginBottom: '28px' }}>
            {prefs.map((pref, i) => (
              <div
                key={pref.id}
                style={{ animationDelay: `${i * 35}ms`, animation: 'slideUp 0.3s var(--ease-out) both' }}
              >
                <NotifPrefRow pref={pref} onChange={updatePref} />
              </div>
            ))}
          </div>

          {/* Save button */}
          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <button
              onClick={handleSavePrefs}
              style={{
                padding: '10px 24px',
                background: saved ? '#16a34a' : '#1A1714',
                color: '#ffffff',
                border: 'none',
                borderRadius: '9px',
                fontSize: '14px',
                fontWeight: 600,
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: '7px',
                transition: 'background 200ms ease',
              }}
              onMouseEnter={(e) => {
                if (!saved) e.currentTarget.style.background = '#eb5d1e'
              }}
              onMouseLeave={(e) => {
                if (!saved) e.currentTarget.style.background = '#1A1714'
              }}
            >
              {saved ? (
                <>
                  <Icon name="check-circle" size={15} />
                  Saved!
                </>
              ) : (
                <>
                  Save preferences
                </>
              )}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
