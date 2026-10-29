'use client'

import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { fetchAnnouncements, type AnnouncementRecord } from '@/lib/adminData'
import {
  DEFAULT_PORTAL_COMMUNICATION_PREFERENCES,
  loadPortalCommunicationPreferences,
  portalCommunicationPreferencesChanged,
  portalCommunicationPreferencesErrorMessage,
  savePortalCommunicationPreferences,
  type PortalCommunicationPreference,
} from '@/lib/portal-communications-preferences'
import { formatDateOnly } from '@/lib/dates'
import './page.inline.css'

// ── Category config ─────────────────────────────────────────────────────────
const CATEGORY_CONFIG: Record<string, { label: string; color: string; bg: string }> = {
  all:       { label: 'All',       color: 'var(--text-muted)', bg: 'var(--bg-surface)' },
  meet:      { label: 'Meet',      color: 'var(--info)', bg: 'var(--info-bg)' },
  admin:     { label: 'Admin',     color: 'var(--water)', bg: 'var(--water-subtle)' },
  facility:  { label: 'Facility',  color: 'var(--success)', bg: 'var(--success-bg)' },
  training:  { label: 'Training',  color: 'var(--warning)', bg: 'var(--warning-bg)' },
  billing:   { label: 'Billing',   color: 'var(--error)', bg: 'var(--error-bg)' },
  volunteer: { label: 'Volunteer', color: 'var(--water)', bg: 'var(--water-subtle)' },
}

type NotifChannel = 'email' | 'push'

// ── Helpers ─────────────────────────────────────────────────────────────────
// One canonical audience -> category mapping, used for both the badge and the
// filter so they cannot disagree. The API only emits `all` and `group`; anything
// without its own category (including unknown future audiences) files under Admin.
function audienceToCategory(audience: string): string {
  return audience in CATEGORY_CONFIG && audience !== 'all' ? audience : 'admin'
}

// ── Loading skeleton ─────────────────────────────────────────────────────────
function Skeleton() {
  return (
    <div className="is-930d87f">
      {[1, 2, 3].map((i) => (
        <div
          key={i}
          style={{
            background: 'var(--bg-elevated)',
            borderRadius: '12px',
            border: '1px solid var(--border)',
            padding: '20px 24px',
            animation: 'pulse 1.5s ease-in-out infinite',
            animationDelay: `${i * 0.1}s`,
          }}
        >
          <div className="is-f22a507" />
          <div className="is-42ea615" />
          <div style={{ width: '70%', height: '10px', background: 'var(--bg-surface)', borderRadius: '6px' }} />
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
  const [announcementsError, setAnnouncementsError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'inbox' | 'preferences'>('inbox')
  const [filter, setFilter] = useState('all')
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [prefs, setPrefs] = useState<PortalCommunicationPreference[]>(DEFAULT_PORTAL_COMMUNICATION_PREFERENCES)
  const [savedPrefs, setSavedPrefs] = useState<PortalCommunicationPreference[]>(DEFAULT_PORTAL_COMMUNICATION_PREFERENCES)
  const [prefsLoading, setPrefsLoading] = useState(true)
  const [prefsSaving, setPrefsSaving] = useState(false)
  const [prefsSaved, setPrefsSaved] = useState(false)
  const [prefsError, setPrefsError] = useState<string | null>(null)
  const [announcementsRetryKey, setAnnouncementsRetryKey] = useState(0)

  useEffect(() => {
    if (!orgId) {
      setAnnouncementsError('Your portal session is missing club context. Refresh and sign in again.')
      setLoading(false)
      return
    }
    setLoading(true)
    setAnnouncementsError(null)
    fetchAnnouncements(api, orgId, 'published')
      .then(setAnnouncements)
      .catch((error) => {
        reportUIError('portal.communications.announcements.load', error, 'Couldn’t load announcements. Try again.', {
          orgId,
        })
        setAnnouncementsError('Couldn’t load announcements. Try again.')
      })
      .finally(() => setLoading(false))
  }, [api, orgId, announcementsRetryKey])

  useEffect(() => {
    let cancelled = false
    if (!orgId) {
      setPrefsLoading(false)
      return
    }

    setPrefsLoading(true)
    setPrefsError(null)
    loadPortalCommunicationPreferences(api, orgId)
      .then((nextPrefs) => {
        if (cancelled) return
        setPrefs(nextPrefs)
        setSavedPrefs(nextPrefs)
      })
      .catch((error: unknown) => {
        if (cancelled) return
        setPrefs(DEFAULT_PORTAL_COMMUNICATION_PREFERENCES)
        setSavedPrefs(DEFAULT_PORTAL_COMMUNICATION_PREFERENCES)
        setPrefsError(portalCommunicationPreferencesErrorMessage(error))
      })
      .finally(() => {
        if (!cancelled) setPrefsLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [api, orgId])

  const filtered = announcements.filter((a) => {
    if (filter === 'all') return true
    return audienceToCategory(a.audience) === filter
  })

  const togglePref = (id: string, channel: NotifChannel) => {
    setPrefsSaved(false)
    setPrefs((prev) =>
      prev.map((p) => (p.id === id ? { ...p, [channel]: !p[channel] } : p)),
    )
  }

  const savePrefs = async () => {
    if (!orgId || prefsSaving) return
    setPrefsSaving(true)
    setPrefsError(null)
    setPrefsSaved(false)
    try {
      const nextPrefs = await savePortalCommunicationPreferences(api, orgId, prefs)
      setPrefs(nextPrefs)
      setSavedPrefs(nextPrefs)
      setPrefsSaved(true)
      setTimeout(() => setPrefsSaved(false), 2500)
    } catch (error) {
      setPrefsError(portalCommunicationPreferencesErrorMessage(error))
    } finally {
      setPrefsSaving(false)
    }
  }

  return (
    <div>
      {/* Page header */}
      <div className="page-heading">
        <h1 className="page-title">
          Communications
        </h1>
      </div>

      {/* Tabs */}
      <div
        className="is-b7bb846"
      >
        {(['inbox', 'preferences'] as const).map((t) => (
          <button type="button"
            key={t}
            onClick={() => setActiveTab(t)}
            style={{
              flex: 1,
              padding: '8px 0',
              borderRadius: '8px',
              border: 'none',
              background: activeTab === t ? 'var(--bg-elevated)' : 'transparent',
              color: activeTab === t ? 'var(--text)' : 'var(--text-subtle)',
              fontSize: 'var(--fs-base)',
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
            className="is-d22a75f"
          >
            {Object.entries(CATEGORY_CONFIG).map(([key, cfg]) => (
              <button type="button"
                key={key}
                onClick={() => setFilter(key)}
                style={{
                  padding: '4px 12px',
                  borderRadius: '999px',
                  border: `1px solid ${filter === key ? cfg.color : 'var(--border-strong)'}`,
                  background: filter === key ? cfg.bg : 'var(--bg-elevated)',
                  color: filter === key ? cfg.color : 'var(--text-subtle)',
                  fontSize: 'var(--fs-sm)',
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
          ) : announcementsError ? (
            <div
              className="is-392b6e6"
            >
              <div>
                <p className="is-703ac01">
                  Announcements are unavailable
                </p>
                <p className="is-de107ca">{announcementsError}</p>
              </div>
              <button type="button"
                onClick={() => setAnnouncementsRetryKey((current) => current + 1)}
                className="is-b2368ab"
              >
                Retry
              </button>
            </div>
          ) : filtered.length === 0 ? (
            <div
              className="card is-ae9c012" 
            >
              <Icon name="megaphone" size={32} style={{ color: 'var(--text-muted)', marginBottom: '16px' }} />
              <p className="is-91904b5">
                No announcements
              </p>
              <p className="is-f299ae1">
                {filter === 'all'
                  ? 'Club announcements will appear here when published.'
                  : 'No announcements in this category.'}
              </p>
            </div>
          ) : (
            <div className="is-930d87f">
              {filtered.map((ann, i) => {
                const cat = CATEGORY_CONFIG[audienceToCategory(ann.audience)] ?? CATEGORY_CONFIG.admin
                const expanded = expandedId === ann.id
                return (
                  <div
                    key={ann.id}
                    style={{
                      background: 'var(--bg-elevated)',
                      borderRadius: '12px',
                      border: '1px solid var(--border)',
                      overflow: 'hidden',
                      animation: `slideUp 0.3s ease ${i * 0.06}s both`,
                    }}
                  >
                    {/* Header */}
                    <button type="button"
                      onClick={() => setExpandedId(expanded ? null : ann.id)}
                      className="is-b89ef60 comms-ann-header"
                    >
                      <div style={{ flex: 1 }}>
                        <div
                          className="is-9fcf47c"
                        >
                          <span
                            style={{
                              fontSize: 'var(--fs-2xs)',
                              fontWeight: 600,
                              fontFamily: 'var(--font-body)',
                              color: cat.color,
                              background: cat.bg,
                              padding: '2px 8px',
                              borderRadius: '4px',
                            }}
                          >
                            {cat.label}
                          </span>
                          <span
                            className="is-c9c6d72"
                          >
                            {formatDateOnly(ann.published_at)}
                          </span>
                        </div>
                        <p
                          className="is-90d8fe3"
                        >
                          {ann.title}
                        </p>
                        <p className="is-4f39ab0">
                          {ann.created_by || 'Club Administration'}
                        </p>
                      </div>
                      <div
                        style={{
                          color: 'var(--border-strong)',
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
                        className="is-945cf76"
                      >
                        <p
                          className="is-debe079"
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
        <div style={{ animation: 'slideUp 0.35s ease both' }}>
          {prefsError && (
            <div
              className="is-b705077"
            >
              {prefsError}
            </div>
          )}
          <div
            className="card is-b25ef49" 
          >
            {/* Header */}
            <div
              className="is-0378948"
            >
              <span className="col-head">
                Notification Event
              </span>
              <span className="col-head" style={{ textAlign: 'center' }}>
                Email
              </span>
              <span className="col-head" style={{ textAlign: 'center' }}>
                Push
              </span>
            </div>

            {prefs.map((pref, i) => (
              <div
                key={pref.id}
                style={{
                  padding: '16px 20px',
                  borderBottom: i < prefs.length - 1 ? '1px solid var(--border)' : 'none',
                  display: 'grid',
                  gridTemplateColumns: '1fr 60px 60px',
                  gap: '12px',
                  alignItems: 'center',
                }}
              >
                <div>
                  <p className="is-58c33bb">
                    {pref.label}
                  </p>
                  <p className="is-4f39ab0">{pref.description}</p>
                </div>
                {(['email', 'push'] as NotifChannel[]).map((ch) => (
                  <div key={ch} style={{ display: 'flex', justifyContent: 'center' }}>
                    <button type="button"
                      role="switch"
                      aria-label={`${pref.label} ${ch}`}
                      aria-checked={pref[ch]}
                      onClick={() => togglePref(pref.id, ch)}
                      disabled={prefsLoading || prefsSaving}
                      style={{
                        width: '40px',
                        height: '22px',
                        borderRadius: '999px',
                        background: pref[ch] ? 'var(--accent)' : 'var(--bg-wash-strong)',
                        border: 'none',
                        cursor: prefsLoading || prefsSaving ? 'not-allowed' : 'pointer',
                        opacity: prefsLoading || prefsSaving ? 0.55 : 1,
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
                          background: 'var(--bg-elevated)',
                          boxShadow: 'var(--shadow-sm)',
                          transition: 'left 150ms ease',
                        }}
                      />
                    </button>
                  </div>
                ))}
              </div>
            ))}
          </div>

          <div className="is-e2ff0fd">
            <button type="button"
              onClick={savePrefs}
              disabled={prefsLoading || prefsSaving || !portalCommunicationPreferencesChanged(prefs, savedPrefs)}
              className="comms-save-btn"
            >
              {prefsLoading ? 'Loading…' : prefsSaving ? 'Saving…' : 'Save Preferences'}
            </button>
            {prefsSaved && (
              <div
                className="is-f38032e"
              >
                <Icon name="check-circle" size={15} />
                Saved
              </div>
            )}
          </div>
        </div>
      )}

    </div>
  )
}
