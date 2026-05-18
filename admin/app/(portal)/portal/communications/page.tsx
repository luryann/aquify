'use client'

import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchAnnouncements, type AnnouncementRecord } from '@/lib/adminData'

// ── Category config ─────────────────────────────────────────────────────────
const CATEGORY_CONFIG: Record<string, { label: string; color: string; bg: string }> = {
  all:       { label: 'All',       color: '#6b7280', bg: '#f9fafb' },
  meet:      { label: 'Meet',      color: '#2563eb', bg: '#eff6ff' },
  admin:     { label: 'Admin',     color: '#7c3aed', bg: '#f5f3ff' },
  facility:  { label: 'Facility',  color: '#059669', bg: '#ecfdf5' },
  training:  { label: 'Training',  color: '#d97706', bg: '#fffbeb' },
  billing:   { label: 'Billing',   color: '#dc2626', bg: '#fef2f2' },
  volunteer: { label: 'Volunteer', color: '#0891b2', bg: '#ecfeff' },
}

type NotifChannel = 'email' | 'push'
type NotifEvent = {
  id: string
  label: string
  description: string
  email: boolean
  push: boolean
}

const INITIAL_PREFS: NotifEvent[] = [
  { id: 'meet_commitment', label: 'Meet Commitment Requests', description: 'When a coach sends a new meet invitation', email: true, push: true },
  { id: 'invoice_due',     label: 'Invoice Due Reminders',   description: 'When an invoice is due or overdue',         email: true, push: true },
  { id: 'announcement',   label: 'Club Announcements',       description: 'General announcements from club staff',     email: true, push: false },
  { id: 'doc_reminder',   label: 'Document Reminders',       description: 'When a required document is expiring',      email: true, push: false },
  { id: 'entry_update',   label: 'Entry Updates',            description: 'When your swimmer\'s meet entries change',  email: false, push: true },
]

// ── Helpers ─────────────────────────────────────────────────────────────────
function fmtDate(dateStr?: string): string {
  if (!dateStr) return '—'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return dateStr
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function audienceToCategory(audience: string): string {
  if (audience === 'all') return 'admin'
  if (audience === 'parents') return 'admin'
  return audience
}

// ── Loading skeleton ─────────────────────────────────────────────────────────
function Skeleton() {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
      {[1, 2, 3].map((i) => (
        <div
          key={i}
          style={{
            background: '#fff',
            borderRadius: '14px',
            border: '1px solid rgba(0,0,0,0.07)',
            padding: '20px 24px',
            animation: 'pulse 1.5s ease-in-out infinite',
            animationDelay: `${i * 0.1}s`,
          }}
        >
          <div style={{ width: '60%', height: '14px', background: '#f0eeec', borderRadius: '6px', marginBottom: '10px' }} />
          <div style={{ width: '90%', height: '10px', background: '#f5f4f2', borderRadius: '6px', marginBottom: '6px' }} />
          <div style={{ width: '70%', height: '10px', background: '#f5f4f2', borderRadius: '6px' }} />
        </div>
      ))}
    </div>
  )
}

// ── Main page ────────────────────────────────────────────────────────────────
export default function CommunicationsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [announcements, setAnnouncements] = useState<AnnouncementRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<'inbox' | 'preferences'>('inbox')
  const [filter, setFilter] = useState('all')
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [prefs, setPrefs] = useState<NotifEvent[]>(INITIAL_PREFS)
  const [savedPrefs, setSavedPrefs] = useState(false)

  useEffect(() => {
    if (!orgId) { setLoading(false); return }
    setLoading(true)
    fetchAnnouncements(api, orgId, 'published')
      .then(setAnnouncements)
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [api, orgId])

  const filtered = announcements.filter((a) => {
    if (filter === 'all') return true
    return audienceToCategory(a.audience) === filter || a.audience === filter
  })

  const togglePref = (id: string, channel: NotifChannel) => {
    setPrefs((prev) =>
      prev.map((p) => (p.id === id ? { ...p, [channel]: !p[channel] } : p)),
    )
  }

  const savePrefs = async () => {
    await new Promise((r) => setTimeout(r, 500))
    setSavedPrefs(true)
    setTimeout(() => setSavedPrefs(false), 2500)
  }

  return (
    <div>
      {/* Page header */}
      <div style={{ marginBottom: '28px', animation: 'slideUp 0.35s ease both' }}>
        <h1
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: 'clamp(24px, 3vw, 32px)',
            color: '#1A1714',
            letterSpacing: '-0.02em',
            lineHeight: 1.1,
            marginBottom: '6px',
          }}
        >
          Communications
        </h1>
        <p style={{ fontSize: '14px', color: '#6B6866' }}>
          Announcements and notification settings from your club.
        </p>
      </div>

      {/* Tabs */}
      <div
        style={{
          display: 'flex',
          gap: '2px',
          marginBottom: '28px',
          background: 'rgba(0,0,0,0.04)',
          borderRadius: '10px',
          padding: '4px',
          maxWidth: '340px',
          animation: 'slideUp 0.38s ease both',
        }}
      >
        {(['inbox', 'preferences'] as const).map((t) => (
          <button
            key={t}
            onClick={() => setActiveTab(t)}
            style={{
              flex: 1,
              padding: '9px 0',
              borderRadius: '7px',
              border: 'none',
              background: activeTab === t ? '#ffffff' : 'transparent',
              color: activeTab === t ? '#1A1714' : '#6B6866',
              fontSize: '13.5px',
              fontWeight: activeTab === t ? 600 : 450,
              cursor: 'pointer',
              boxShadow: activeTab === t ? '0 1px 4px rgba(0,0,0,0.08)' : 'none',
              transition: 'all 120ms ease',
              fontFamily: 'var(--font-body)',
              textTransform: 'capitalize',
            }}
          >
            {t === 'inbox' ? 'Announcements' : 'Preferences'}
          </button>
        ))}
      </div>

      {/* ── INBOX TAB ──────────────────────────────────────────────────────── */}
      {activeTab === 'inbox' && (
        <div>
          {/* Category filter pills */}
          <div
            style={{
              display: 'flex',
              gap: '8px',
              flexWrap: 'wrap',
              marginBottom: '20px',
              animation: 'slideUp 0.4s ease both',
            }}
          >
            {Object.entries(CATEGORY_CONFIG).map(([key, cfg]) => (
              <button
                key={key}
                onClick={() => setFilter(key)}
                style={{
                  padding: '5px 12px',
                  borderRadius: '999px',
                  border: `1px solid ${filter === key ? cfg.color : 'rgba(0,0,0,0.12)'}`,
                  background: filter === key ? cfg.bg : '#ffffff',
                  color: filter === key ? cfg.color : '#6B6866',
                  fontSize: '12.5px',
                  fontWeight: filter === key ? 600 : 450,
                  cursor: 'pointer',
                  transition: 'all 120ms ease',
                  fontFamily: 'var(--font-body)',
                }}
              >
                {cfg.label}
              </button>
            ))}
          </div>

          {/* Loading / content */}
          {loading ? (
            <Skeleton />
          ) : filtered.length === 0 ? (
            <div
              style={{
                background: '#fff',
                borderRadius: '14px',
                border: '1px solid rgba(0,0,0,0.07)',
                padding: '56px 32px',
                textAlign: 'center',
                animation: 'slideUp 0.3s ease both',
              }}
            >
              <Icon name="megaphone" size={32} style={{ color: '#D4D1CE', marginBottom: '14px' }} />
              <p style={{ fontSize: '15px', color: '#9A9693', fontWeight: 500, marginBottom: '6px' }}>
                No announcements
              </p>
              <p style={{ fontSize: '13px', color: '#C0BDBB' }}>
                {filter === 'all'
                  ? 'Club announcements will appear here when published.'
                  : 'No announcements in this category.'}
              </p>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {filtered.map((ann, i) => {
                const cat = CATEGORY_CONFIG[audienceToCategory(ann.audience)] ?? CATEGORY_CONFIG.admin
                const expanded = expandedId === ann.id
                return (
                  <div
                    key={ann.id}
                    style={{
                      background: '#ffffff',
                      borderRadius: '14px',
                      border: '1px solid rgba(0,0,0,0.07)',
                      boxShadow: '0 1px 6px rgba(0,0,0,0.04)',
                      overflow: 'hidden',
                      animation: `slideUp 0.3s ease ${i * 0.06}s both`,
                    }}
                  >
                    {/* Header */}
                    <button
                      onClick={() => setExpandedId(expanded ? null : ann.id)}
                      style={{
                        width: '100%',
                        display: 'flex',
                        alignItems: 'flex-start',
                        justifyContent: 'space-between',
                        gap: '16px',
                        padding: '18px 20px',
                        background: 'none',
                        border: 'none',
                        cursor: 'pointer',
                        textAlign: 'left',
                        transition: 'background 100ms ease',
                      }}
                      onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.015)')}
                      onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                    >
                      <div style={{ flex: 1 }}>
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: '8px',
                            marginBottom: '5px',
                            flexWrap: 'wrap',
                          }}
                        >
                          <span
                            style={{
                              fontSize: '10.5px',
                              fontWeight: 600,
                              fontFamily: 'var(--font-mono)',
                              textTransform: 'uppercase',
                              letterSpacing: '0.05em',
                              color: cat.color,
                              background: cat.bg,
                              padding: '2px 7px',
                              borderRadius: '4px',
                            }}
                          >
                            {cat.label}
                          </span>
                          <span
                            style={{
                              fontSize: '11px',
                              color: '#A09D9B',
                              fontFamily: 'var(--font-mono)',
                            }}
                          >
                            {fmtDate(ann.published_at)}
                          </span>
                        </div>
                        <p
                          style={{
                            fontSize: '15px',
                            fontWeight: 600,
                            color: '#1A1714',
                            marginBottom: '3px',
                            lineHeight: 1.3,
                          }}
                        >
                          {ann.title}
                        </p>
                        <p style={{ fontSize: '12.5px', color: '#9A9693' }}>
                          {ann.created_by || 'Club Administration'}
                        </p>
                      </div>
                      <div
                        style={{
                          color: '#C0BDBB',
                          transition: 'transform 200ms ease',
                          transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
                          flexShrink: 0,
                          paddingTop: '2px',
                        }}
                      >
                        <Icon name="chevron-down" size={16} />
                      </div>
                    </button>

                    {/* Expanded body */}
                    {expanded && (
                      <div
                        style={{
                          padding: '0 20px 20px',
                          borderTop: '1px solid rgba(0,0,0,0.06)',
                          animation: 'slideUp 0.2s ease both',
                        }}
                      >
                        <p
                          style={{
                            fontSize: '14px',
                            color: '#3D3A38',
                            lineHeight: 1.65,
                            marginTop: '16px',
                          }}
                        >
                          {ann.body}
                        </p>
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>
      )}

      {/* ── PREFERENCES TAB ────────────────────────────────────────────────── */}
      {activeTab === 'preferences' && (
        <div style={{ maxWidth: '640px', animation: 'slideUp 0.35s ease both' }}>
          <div
            style={{
              background: '#ffffff',
              borderRadius: '14px',
              border: '1px solid rgba(0,0,0,0.07)',
              boxShadow: '0 1px 6px rgba(0,0,0,0.04)',
              overflow: 'hidden',
              marginBottom: '20px',
            }}
          >
            {/* Header */}
            <div
              style={{
                padding: '16px 20px',
                borderBottom: '1px solid rgba(0,0,0,0.06)',
                display: 'grid',
                gridTemplateColumns: '1fr 60px 60px',
                gap: '12px',
                alignItems: 'center',
              }}
            >
              <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.07em' }}>
                Notification Event
              </span>
              <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.05em', textAlign: 'center' }}>
                Email
              </span>
              <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.05em', textAlign: 'center' }}>
                Push
              </span>
            </div>

            {prefs.map((pref, i) => (
              <div
                key={pref.id}
                style={{
                  padding: '16px 20px',
                  borderBottom: i < prefs.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                  display: 'grid',
                  gridTemplateColumns: '1fr 60px 60px',
                  gap: '12px',
                  alignItems: 'center',
                }}
              >
                <div>
                  <p style={{ fontSize: '14px', fontWeight: 500, color: '#1A1714', marginBottom: '2px' }}>
                    {pref.label}
                  </p>
                  <p style={{ fontSize: '12px', color: '#9A9693' }}>{pref.description}</p>
                </div>
                {(['email', 'push'] as NotifChannel[]).map((ch) => (
                  <div key={ch} style={{ display: 'flex', justifyContent: 'center' }}>
                    <button
                      onClick={() => togglePref(pref.id, ch)}
                      style={{
                        width: '40px',
                        height: '22px',
                        borderRadius: '999px',
                        background: pref[ch] ? '#eb5d1e' : 'rgba(0,0,0,0.12)',
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
                          left: pref[ch] ? '20px' : '3px',
                          width: '16px',
                          height: '16px',
                          borderRadius: '50%',
                          background: '#ffffff',
                          boxShadow: '0 1px 3px rgba(0,0,0,0.2)',
                          transition: 'left 150ms ease',
                        }}
                      />
                    </button>
                  </div>
                ))}
              </div>
            ))}
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <button
              onClick={savePrefs}
              style={{
                padding: '10px 24px',
                background: '#1A1714',
                color: '#fff',
                border: 'none',
                borderRadius: '9px',
                fontSize: '14px',
                fontWeight: 600,
                cursor: 'pointer',
                fontFamily: 'var(--font-body)',
                transition: 'background 120ms ease',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = '#eb5d1e')}
              onMouseLeave={(e) => (e.currentTarget.style.background = '#1A1714')}
            >
              Save Preferences
            </button>
            {savedPrefs && (
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '6px',
                  color: '#059669',
                  fontSize: '13px',
                  fontWeight: 500,
                  animation: 'slideUp 0.2s ease both',
                }}
              >
                <Icon name="check-circle" size={15} />
                Saved
              </div>
            )}
          </div>
        </div>
      )}

      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(10px); }
          to   { opacity: 1; transform: translateY(0); }
        }
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50%       { opacity: 0.5; }
        }
      `}</style>
    </div>
  )
}
