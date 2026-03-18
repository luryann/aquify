'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'

// ── Stub data ──────────────────────────────────────────────────────────────
const SWIMMER = {
  id: 'sw-001',
  name: 'Emma Chen',
  dob: 'Feb 12, 2012',
  age: 14,
  gender: 'Female',
  ussNumber: 'ABC123456X',
  group: 'Senior A',
  coach: 'Coach Martinez',
  status: 'active' as const,
  gradient: 'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
  initials: 'EC',
  registrationStatus: 'registered' as const,
  season: '2025–2026',
  // editable fields
  jerseySize: 'YL',
  emergencyNotes: 'Mild chlorine sensitivity — use non-scented sunscreen. No known drug allergies. Carries an inhaler (mild asthma, rarely needed).',
  preferredName: 'Em',
}

const BEST_TIMES = [
  { event: '50 Free',    scy: '26.44', scm: '28.91', lcm: '29.15' },
  { event: '100 Free',   scy: '57.82', scm: '1:02.44', lcm: '1:03.77' },
  { event: '200 Free',   scy: '2:05.31', scm: '2:15.88', lcm: '2:18.22' },
  { event: '100 Back',   scy: '1:03.44', scm: '1:09.01', lcm: '1:10.55' },
  { event: '100 Breast', scy: '1:12.77', scm: '—', lcm: '—' },
  { event: '100 Fly',    scy: '1:05.18', scm: '1:11.33', lcm: '—' },
  { event: '200 IM',     scy: '2:18.50', scm: '2:30.11', lcm: '—' },
]

const HISTORY = [
  { date: 'Mar 8, 2026',  event: 'Assigned to Senior A group',        actor: 'Coach Martinez' },
  { date: 'Feb 1, 2026',  event: 'Registration approved (2025–2026)', actor: 'Admin' },
  { date: 'Sep 12, 2025', event: 'Season waiver accepted',            actor: 'Sarah Chen' },
  { date: 'Sep 3, 2025',  event: 'Joined Junior A → Senior A moved',  actor: 'Coach Martinez' },
]

// ── Helpers ─────────────────────────────────────────────────────────────────
const STATUS_CONFIG = {
  active:   { label: 'Active',   bg: '#ecfdf5', color: '#059669' },
  inactive: { label: 'Inactive', bg: '#fef2f2', color: '#dc2626' },
  pending:  { label: 'Pending',  bg: '#fffbeb', color: '#d97706' },
}

const EDITABLE_FIELD_NOTE =
  'Fields marked with a pencil icon can be updated by you. Other fields are managed by club staff.'

type Tab = 'profile' | 'performance' | 'history'

// ── Component ────────────────────────────────────────────────────────────────
export default function SwimmerProfilePage() {
  const [tab, setTab] = useState<Tab>('profile')
  const [jerseySize, setJerseySize] = useState(SWIMMER.jerseySize)
  const [preferredName, setPreferredName] = useState(SWIMMER.preferredName)
  const [emergencyNotes, setEmergencyNotes] = useState(SWIMMER.emergencyNotes)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [editingJersey, setEditingJersey] = useState(false)
  const [editingName, setEditingName] = useState(false)
  const [editingNotes, setEditingNotes] = useState(false)

  const statusCfg = STATUS_CONFIG[SWIMMER.status]

  const handleSave = async () => {
    setSaving(true)
    await new Promise((r) => setTimeout(r, 700))
    setSaving(false)
    setSaved(true)
    setEditingJersey(false)
    setEditingName(false)
    setEditingNotes(false)
    setTimeout(() => setSaved(false), 2500)
  }

  return (
    <div style={{ maxWidth: '820px' }}>
      {/* Breadcrumb */}
      <nav
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          fontSize: '13px',
          color: '#9A9693',
          marginBottom: '28px',
          animation: 'slideUp 0.3s ease both',
        }}
      >
        <Link href="/portal/household" style={{ color: '#9A9693', textDecoration: 'none' }}>
          Household
        </Link>
        <span>/</span>
        <span style={{ color: '#1A1714', fontWeight: 500 }}>Emma Chen</span>
      </nav>

      {/* Header Card */}
      <div
        style={{
          background: '#ffffff',
          borderRadius: '16px',
          border: '1px solid rgba(0,0,0,0.07)',
          boxShadow: '0 2px 12px rgba(0,0,0,0.05)',
          padding: '28px',
          marginBottom: '24px',
          display: 'flex',
          alignItems: 'center',
          gap: '20px',
          animation: 'slideUp 0.35s ease both',
        }}
      >
        {/* Avatar */}
        <div
          style={{
            width: '72px',
            height: '72px',
            borderRadius: '50%',
            background: SWIMMER.gradient,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#fff',
            fontSize: '24px',
            fontWeight: 700,
            fontFamily: 'var(--font-mono)',
            flexShrink: 0,
            boxShadow: '0 4px 16px rgba(59,130,246,0.3)',
          }}
        >
          {SWIMMER.initials}
        </div>

        {/* Info */}
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
            <h1
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: '26px',
                color: '#1A1714',
                margin: 0,
                lineHeight: 1.1,
              }}
            >
              {SWIMMER.name}
            </h1>
            <span
              style={{
                padding: '3px 9px',
                borderRadius: '999px',
                fontSize: '11.5px',
                fontWeight: 600,
                background: statusCfg.bg,
                color: statusCfg.color,
                textTransform: 'uppercase',
                letterSpacing: '0.05em',
              }}
            >
              {statusCfg.label}
            </span>
          </div>
          <div
            style={{
              display: 'flex',
              flexWrap: 'wrap',
              gap: '16px',
              marginTop: '8px',
              fontSize: '13px',
              color: '#6B6866',
            }}
          >
            <span>Age {SWIMMER.age}</span>
            <span>·</span>
            <span>{SWIMMER.gender}</span>
            <span>·</span>
            <span>{SWIMMER.group}</span>
            <span>·</span>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px' }}>{SWIMMER.ussNumber}</span>
          </div>
          <div style={{ marginTop: '6px', fontSize: '12.5px', color: '#9A9693' }}>
            Season {SWIMMER.season} · Registered
          </div>
        </div>

        {/* Save feedback */}
        {saved && (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              fontSize: '13px',
              color: '#059669',
              fontWeight: 500,
              animation: 'slideUp 0.2s ease both',
            }}
          >
            <Icon name="check-circle" size={15} />
            Saved
          </div>
        )}
      </div>

      {/* Tabs */}
      <div
        style={{
          display: 'flex',
          gap: '2px',
          marginBottom: '24px',
          background: 'rgba(0,0,0,0.04)',
          borderRadius: '10px',
          padding: '4px',
          animation: 'slideUp 0.4s ease both',
        }}
      >
        {(['profile', 'performance', 'history'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              flex: 1,
              padding: '8px 0',
              borderRadius: '7px',
              border: 'none',
              background: tab === t ? '#ffffff' : 'transparent',
              color: tab === t ? '#1A1714' : '#6B6866',
              fontSize: '13.5px',
              fontWeight: tab === t ? 600 : 450,
              cursor: 'pointer',
              boxShadow: tab === t ? '0 1px 4px rgba(0,0,0,0.08)' : 'none',
              transition: 'all 120ms ease',
              fontFamily: 'var(--font-body)',
              textTransform: 'capitalize',
            }}
          >
            {t}
          </button>
        ))}
      </div>

      {/* ── PROFILE TAB ─────────────────────────────────────────────────────── */}
      {tab === 'profile' && (
        <div style={{ animation: 'slideUp 0.3s ease both' }}>
          {/* Edit policy notice */}
          <div
            style={{
              background: 'rgba(235,93,30,0.06)',
              border: '1px solid rgba(235,93,30,0.15)',
              borderRadius: '10px',
              padding: '12px 16px',
              marginBottom: '20px',
              display: 'flex',
              alignItems: 'flex-start',
              gap: '10px',
            }}
          >
            <Icon name="info" size={15} style={{ color: '#eb5d1e', flexShrink: 0, marginTop: '2px' }} />
            <p style={{ margin: 0, fontSize: '13px', color: '#7A4A28', lineHeight: 1.5 }}>
              {EDITABLE_FIELD_NOTE}
            </p>
          </div>

          {/* Read-only Fields */}
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
            <div
              style={{
                padding: '16px 20px 12px',
                borderBottom: '1px solid rgba(0,0,0,0.06)',
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
              }}
            >
              <span
                style={{
                  fontSize: '11px',
                  fontFamily: 'var(--font-mono)',
                  color: '#9A9693',
                  textTransform: 'uppercase',
                  letterSpacing: '0.08em',
                }}
              >
                Club-managed fields
              </span>
              <span
                style={{
                  padding: '2px 7px',
                  background: 'rgba(0,0,0,0.05)',
                  borderRadius: '4px',
                  fontSize: '10.5px',
                  color: '#9A9693',
                }}
              >
                read-only
              </span>
            </div>

            {[
              { label: 'Full Name', value: SWIMMER.name },
              { label: 'Date of Birth', value: SWIMMER.dob },
              { label: 'Gender', value: SWIMMER.gender },
              { label: 'USS Number', value: SWIMMER.ussNumber, mono: true },
              { label: 'Training Group', value: `${SWIMMER.group} (Coach ${SWIMMER.coach})` },
              { label: 'Registration Status', value: 'Registered — 2025–2026 Season' },
            ].map((field, i) => (
              <div
                key={field.label}
                style={{
                  display: 'flex',
                  padding: '14px 20px',
                  borderBottom: i < 5 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                  gap: '24px',
                  alignItems: 'center',
                }}
              >
                <span
                  style={{
                    width: '160px',
                    fontSize: '13px',
                    color: '#9A9693',
                    flexShrink: 0,
                  }}
                >
                  {field.label}
                </span>
                <span
                  style={{
                    fontSize: '13.5px',
                    color: '#6B6866',
                    fontFamily: field.mono ? 'var(--font-mono)' : undefined,
                  }}
                >
                  {field.value}
                </span>
              </div>
            ))}
          </div>

          {/* Editable Fields */}
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
            <div
              style={{
                padding: '16px 20px 12px',
                borderBottom: '1px solid rgba(0,0,0,0.06)',
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
              }}
            >
              <span
                style={{
                  fontSize: '11px',
                  fontFamily: 'var(--font-mono)',
                  color: '#9A9693',
                  textTransform: 'uppercase',
                  letterSpacing: '0.08em',
                }}
              >
                Parent-editable fields
              </span>
              <Icon name="edit" size={12} style={{ color: '#eb5d1e' }} />
            </div>

            {/* Preferred Name */}
            <div
              style={{
                padding: '16px 20px',
                borderBottom: '1px solid rgba(0,0,0,0.05)',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: '12px', color: '#9A9693', marginBottom: '4px' }}>
                    Preferred Name / Nickname
                  </div>
                  {editingName ? (
                    <input
                      autoFocus
                      value={preferredName}
                      onChange={(e) => setPreferredName(e.target.value)}
                      style={{
                        width: '100%',
                        maxWidth: '280px',
                        padding: '7px 11px',
                        border: '1.5px solid #eb5d1e',
                        borderRadius: '8px',
                        fontSize: '14px',
                        color: '#1A1714',
                        fontFamily: 'var(--font-body)',
                        outline: 'none',
                        background: '#fff',
                      }}
                    />
                  ) : (
                    <div style={{ fontSize: '14px', color: '#1A1714', fontWeight: 500 }}>
                      {preferredName}
                    </div>
                  )}
                </div>
                <button
                  onClick={() => setEditingName(!editingName)}
                  style={{
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    color: '#eb5d1e',
                    fontSize: '12.5px',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                    padding: '4px 8px',
                    borderRadius: '6px',
                    transition: 'background 100ms',
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(235,93,30,0.06)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="edit" size={12} />
                  {editingName ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>

            {/* Jersey Size */}
            <div
              style={{
                padding: '16px 20px',
                borderBottom: '1px solid rgba(0,0,0,0.05)',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: '12px', color: '#9A9693', marginBottom: '4px' }}>Jersey Size</div>
                  {editingJersey ? (
                    <select
                      value={jerseySize}
                      onChange={(e) => setJerseySize(e.target.value)}
                      autoFocus
                      style={{
                        padding: '7px 11px',
                        border: '1.5px solid #eb5d1e',
                        borderRadius: '8px',
                        fontSize: '14px',
                        color: '#1A1714',
                        fontFamily: 'var(--font-body)',
                        outline: 'none',
                        background: '#fff',
                        cursor: 'pointer',
                      }}
                    >
                      {['YS', 'YM', 'YL', 'YXL', 'AS', 'AM', 'AL', 'AXL', 'AXXL'].map((s) => (
                        <option key={s} value={s}>{s}</option>
                      ))}
                    </select>
                  ) : (
                    <div style={{ fontSize: '14px', color: '#1A1714', fontWeight: 500 }}>{jerseySize}</div>
                  )}
                </div>
                <button
                  onClick={() => setEditingJersey(!editingJersey)}
                  style={{
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    color: '#eb5d1e',
                    fontSize: '12.5px',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                    padding: '4px 8px',
                    borderRadius: '6px',
                    transition: 'background 100ms',
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(235,93,30,0.06)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="edit" size={12} />
                  {editingJersey ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>

            {/* Emergency Notes */}
            <div style={{ padding: '16px 20px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: '12px', color: '#9A9693', marginBottom: '4px' }}>
                    Medical / Emergency Notes
                  </div>
                  {editingNotes ? (
                    <textarea
                      autoFocus
                      value={emergencyNotes}
                      onChange={(e) => setEmergencyNotes(e.target.value)}
                      rows={4}
                      style={{
                        width: '100%',
                        padding: '9px 12px',
                        border: '1.5px solid #eb5d1e',
                        borderRadius: '8px',
                        fontSize: '13.5px',
                        color: '#1A1714',
                        fontFamily: 'var(--font-body)',
                        outline: 'none',
                        background: '#fff',
                        resize: 'vertical',
                        lineHeight: 1.5,
                      }}
                    />
                  ) : (
                    <div
                      style={{
                        fontSize: '13.5px',
                        color: '#1A1714',
                        lineHeight: 1.6,
                        maxWidth: '500px',
                      }}
                    >
                      {emergencyNotes || (
                        <span style={{ color: '#C0BDbb', fontStyle: 'italic' }}>No notes added</span>
                      )}
                    </div>
                  )}
                  <div style={{ fontSize: '11.5px', color: '#C0BDBB', marginTop: '6px' }}>
                    Visible to coaches and club staff during meets and practices.
                  </div>
                </div>
                <button
                  onClick={() => setEditingNotes(!editingNotes)}
                  style={{
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    color: '#eb5d1e',
                    fontSize: '12.5px',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                    padding: '4px 8px',
                    borderRadius: '6px',
                    transition: 'background 100ms',
                    flexShrink: 0,
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(235,93,30,0.06)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="edit" size={12} />
                  {editingNotes ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>
          </div>

          {/* Save Button */}
          {(editingJersey || editingName || editingNotes) && (
            <div
              style={{
                display: 'flex',
                justifyContent: 'flex-end',
                animation: 'slideUp 0.2s ease both',
              }}
            >
              <button
                onClick={handleSave}
                disabled={saving}
                style={{
                  padding: '10px 24px',
                  background: '#eb5d1e',
                  color: '#fff',
                  border: 'none',
                  borderRadius: '9px',
                  fontSize: '14px',
                  fontWeight: 600,
                  cursor: saving ? 'wait' : 'pointer',
                  opacity: saving ? 0.7 : 1,
                  transition: 'opacity 120ms',
                  fontFamily: 'var(--font-body)',
                }}
              >
                {saving ? 'Saving…' : 'Save Changes'}
              </button>
            </div>
          )}
        </div>
      )}

      {/* ── PERFORMANCE TAB ──────────────────────────────────────────────────── */}
      {tab === 'performance' && (
        <div style={{ animation: 'slideUp 0.3s ease both' }}>
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
            <div
              style={{
                padding: '16px 20px',
                borderBottom: '1px solid rgba(0,0,0,0.06)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
              }}
            >
              <span
                style={{
                  fontFamily: 'var(--font-display)',
                  fontSize: '17px',
                  color: '#1A1714',
                }}
              >
                Best Times
              </span>
              <span
                style={{
                  fontSize: '11.5px',
                  color: '#9A9693',
                  fontFamily: 'var(--font-mono)',
                }}
              >
                as of Mar 2026
              </span>
            </div>

            {/* Table header */}
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr 90px 90px 90px',
                padding: '10px 20px',
                borderBottom: '1px solid rgba(0,0,0,0.06)',
                background: 'rgba(0,0,0,0.02)',
              }}
            >
              {['Event', 'SCY', 'SCM', 'LCM'].map((h) => (
                <span
                  key={h}
                  style={{
                    fontSize: '11px',
                    fontFamily: 'var(--font-mono)',
                    color: '#9A9693',
                    textTransform: 'uppercase',
                    letterSpacing: '0.06em',
                    textAlign: h === 'Event' ? 'left' : 'center',
                  }}
                >
                  {h}
                </span>
              ))}
            </div>

            {/* Rows */}
            {BEST_TIMES.map((row, i) => (
              <div
                key={row.event}
                style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 90px 90px 90px',
                  padding: '13px 20px',
                  borderBottom: i < BEST_TIMES.length - 1 ? '1px solid rgba(0,0,0,0.04)' : 'none',
                  alignItems: 'center',
                }}
              >
                <span style={{ fontSize: '14px', color: '#1A1714', fontWeight: 500 }}>
                  {row.event}
                </span>
                {[row.scy, row.scm, row.lcm].map((t, j) => (
                  <span
                    key={j}
                    style={{
                      fontSize: '13.5px',
                      color: t === '—' ? '#C0BDBB' : '#4B7BF5',
                      fontFamily: 'var(--font-mono)',
                      textAlign: 'center',
                      fontWeight: t === '—' ? 400 : 600,
                    }}
                  >
                    {t}
                  </span>
                ))}
              </div>
            ))}
          </div>

          <div
            style={{
              background: 'rgba(235,93,30,0.04)',
              border: '1px solid rgba(235,93,30,0.12)',
              borderRadius: '10px',
              padding: '12px 16px',
              fontSize: '12.5px',
              color: '#7A4A28',
              lineHeight: 1.5,
            }}
          >
            Best times are updated automatically when meet results are imported. Contact the club to dispute a time.
          </div>
        </div>
      )}

      {/* ── HISTORY TAB ──────────────────────────────────────────────────────── */}
      {tab === 'history' && (
        <div style={{ animation: 'slideUp 0.3s ease both' }}>
          <div
            style={{
              background: '#ffffff',
              borderRadius: '14px',
              border: '1px solid rgba(0,0,0,0.07)',
              boxShadow: '0 1px 6px rgba(0,0,0,0.04)',
              overflow: 'hidden',
            }}
          >
            <div style={{ padding: '16px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>
              <span
                style={{
                  fontFamily: 'var(--font-display)',
                  fontSize: '17px',
                  color: '#1A1714',
                }}
              >
                Activity History
              </span>
            </div>

            {HISTORY.map((entry, i) => (
              <div
                key={i}
                style={{
                  display: 'flex',
                  gap: '16px',
                  padding: '14px 20px',
                  borderBottom: i < HISTORY.length - 1 ? '1px solid rgba(0,0,0,0.04)' : 'none',
                  alignItems: 'flex-start',
                }}
              >
                {/* Dot timeline */}
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', paddingTop: '4px' }}>
                  <div
                    style={{
                      width: '8px',
                      height: '8px',
                      borderRadius: '50%',
                      background: '#D4D1CE',
                      flexShrink: 0,
                    }}
                  />
                  {i < HISTORY.length - 1 && (
                    <div
                      style={{
                        width: '1px',
                        flex: 1,
                        background: 'rgba(0,0,0,0.07)',
                        marginTop: '4px',
                        minHeight: '24px',
                      }}
                    />
                  )}
                </div>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: '13.5px', color: '#1A1714', lineHeight: 1.4 }}>
                    {entry.event}
                  </div>
                  <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '3px' }}>
                    {entry.date} · {entry.actor}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(12px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  )
}
