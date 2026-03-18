'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

// ── Design tokens ──────────────────────────────────────────────────────────
const C = {
  accent:       '#00D4AA',
  accentDim:    'rgba(0,212,170,0.10)',
  accentBorder: 'rgba(0,212,170,0.22)',
  text:         '#E4F2FF',
  subtle:       '#7FA8C9',
  muted:        '#4A6B82',
  card:         '#0D2235',
  border:       'rgba(0,212,170,0.10)',
  inputBg:      'rgba(0,0,0,0.25)',
  inputBorder:  'rgba(0,212,170,0.18)',
}

// ── Stub data ──────────────────────────────────────────────────────────────
const SWIMMER_PROFILE = {
  // Read-only (managed by club)
  legalName:         'Emma Rachel Chen',
  preferredName:     'Emma',
  dob:               'Aug 14, 2011',
  age:               14,
  gender:            'Female',
  ussNumber:         'CA2019-0134',
  registrationYear:  '2025–2026',
  registrationStatus:'Active',
  group:             'Senior A',
  coachName:         'Coach Rivera',
  clubName:          'Aquify Swim Club',

  // Parent-editable (via parent portal)
  jerseySize:        'M',
  emergencyNotes:    'Mild penicillin allergy — carry EpiPen. Contact mom first.',

  // Swimmer-editable (limited)
  bio:               '',
  preferredEvents:   ['100 Back', '200 Free', '400 IM'],
}

// ── Components ─────────────────────────────────────────────────────────────
function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p
        style={{
          fontSize: '10.5px',
          fontFamily: 'var(--font-mono)',
          color: C.muted,
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
          marginBottom: '4px',
        }}
      >
        {label}
      </p>
      <p style={{ fontSize: '14px', color: C.text, fontWeight: 500, lineHeight: 1.4 }}>
        {value}
      </p>
    </div>
  )
}

function FieldGroup({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div
      style={{
        background: C.card,
        border: `1px solid ${C.border}`,
        borderRadius: '14px',
        padding: '22px',
        display: 'flex',
        flexDirection: 'column',
        gap: '18px',
      }}
    >
      <h3
        style={{
          fontFamily: 'var(--font-display)',
          fontSize: '17px',
          color: C.text,
          letterSpacing: '-0.01em',
          paddingBottom: '14px',
          borderBottom: `1px solid ${C.border}`,
          margin: '-4px 0 0',
        }}
      >
        {label}
      </h3>
      {children}
    </div>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function SwimmerProfilePage() {
  const [bio, setBio] = useState(SWIMMER_PROFILE.bio)
  const [saved, setSaved] = useState(false)

  const handleSave = () => {
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  return (
    <div>
      {/* Header */}
      <div
        style={{
          display: 'flex',
          alignItems: 'flex-start',
          gap: '20px',
          marginBottom: '36px',
          animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) both',
        }}
      >
        {/* Avatar */}
        <div
          style={{
            width: '72px',
            height: '72px',
            borderRadius: '50%',
            background: 'linear-gradient(135deg, #00D4AA 0%, #0099CC 100%)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#061623',
            fontSize: '24px',
            fontWeight: 700,
            fontFamily: 'var(--font-mono)',
            boxShadow: '0 0 32px rgba(0,212,170,0.28)',
            flexShrink: 0,
          }}
        >
          EC
        </div>

        <div style={{ flex: 1 }}>
          <h1
            style={{
              fontFamily: 'var(--font-display)',
              fontSize: 'clamp(24px, 4vw, 32px)',
              color: C.text,
              letterSpacing: '-0.02em',
              lineHeight: 1.1,
              marginBottom: '4px',
            }}
          >
            {SWIMMER_PROFILE.preferredName} Chen
          </h1>
          <p style={{ fontSize: '13px', color: C.subtle, marginBottom: '10px' }}>
            {SWIMMER_PROFILE.group} · USS# {SWIMMER_PROFILE.ussNumber}
          </p>
          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
            <span
              style={{
                fontSize: '11px',
                fontFamily: 'var(--font-mono)',
                color: '#00D4AA',
                background: 'rgba(0,212,170,0.12)',
                border: '1px solid rgba(0,212,170,0.25)',
                padding: '3px 10px',
                borderRadius: '999px',
                fontWeight: 600,
              }}
            >
              {SWIMMER_PROFILE.registrationStatus}
            </span>
            <span
              style={{
                fontSize: '11px',
                fontFamily: 'var(--font-mono)',
                color: C.subtle,
                background: 'rgba(127,168,201,0.10)',
                border: '1px solid rgba(127,168,201,0.15)',
                padding: '3px 10px',
                borderRadius: '999px',
              }}
            >
              {SWIMMER_PROFILE.registrationYear}
            </span>
          </div>
        </div>
      </div>

      {/* Read-only notice */}
      <div
        style={{
          background: 'rgba(0,212,170,0.06)',
          border: `1px solid ${C.accentBorder}`,
          borderRadius: '10px',
          padding: '12px 16px',
          display: 'flex',
          gap: '10px',
          alignItems: 'flex-start',
          marginBottom: '24px',
          animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.05s both',
        }}
      >
        <Icon name="info" size={15} style={{ color: C.accent, marginTop: '1px', flexShrink: 0 }} />
        <p style={{ fontSize: '12.5px', color: C.subtle, lineHeight: 1.5 }}>
          Most profile fields are managed by the club or your parent account. You can update your bio below. To change other information, please ask a parent to update it via the parent portal.
        </p>
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'minmax(0,1fr) 300px',
          gap: '20px',
          alignItems: 'start',
        }}
      >
        {/* Main column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {/* Club info */}
          <div style={{ animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.08s both' }}>
            <FieldGroup label="Club Information">
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 1fr',
                  gap: '18px',
                }}
              >
                <ReadOnlyField label="Club" value={SWIMMER_PROFILE.clubName} />
                <ReadOnlyField label="Group" value={SWIMMER_PROFILE.group} />
                <ReadOnlyField label="Coach" value={SWIMMER_PROFILE.coachName} />
                <ReadOnlyField label="Registration" value={SWIMMER_PROFILE.registrationYear} />
              </div>
            </FieldGroup>
          </div>

          {/* Personal info */}
          <div style={{ animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.12s both' }}>
            <FieldGroup label="Personal Information">
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 1fr',
                  gap: '18px',
                }}
              >
                <ReadOnlyField label="Legal Name" value={SWIMMER_PROFILE.legalName} />
                <ReadOnlyField label="Preferred Name" value={SWIMMER_PROFILE.preferredName} />
                <ReadOnlyField label="Date of Birth" value={SWIMMER_PROFILE.dob} />
                <ReadOnlyField label="Age" value={String(SWIMMER_PROFILE.age)} />
                <ReadOnlyField label="Gender" value={SWIMMER_PROFILE.gender} />
                <ReadOnlyField label="USS Number" value={SWIMMER_PROFILE.ussNumber} />
              </div>
            </FieldGroup>
          </div>

          {/* Bio — editable */}
          <div style={{ animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.16s both' }}>
            <div
              style={{
                background: C.card,
                border: `1px solid ${C.accentBorder}`,
                borderRadius: '14px',
                padding: '22px',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px', paddingBottom: '14px', borderBottom: `1px solid ${C.border}` }}>
                <h3
                  style={{
                    fontFamily: 'var(--font-display)',
                    fontSize: '17px',
                    color: C.text,
                    letterSpacing: '-0.01em',
                  }}
                >
                  My Bio
                </h3>
                <span
                  style={{
                    fontSize: '10px',
                    fontFamily: 'var(--font-mono)',
                    color: C.accent,
                    background: C.accentDim,
                    border: `1px solid ${C.accentBorder}`,
                    padding: '2px 8px',
                    borderRadius: '999px',
                  }}
                >
                  Editable
                </span>
              </div>

              <p style={{ fontSize: '12.5px', color: C.muted, marginBottom: '12px', lineHeight: 1.5 }}>
                Share a little about yourself — this appears on your team profile.
              </p>

              <textarea
                value={bio}
                onChange={(e) => setBio(e.target.value)}
                placeholder="Tell your team about yourself. Your goals, favorite events, or anything you'd like to share..."
                maxLength={400}
                rows={4}
                style={{
                  width: '100%',
                  background: C.inputBg,
                  border: `1px solid ${C.inputBorder}`,
                  borderRadius: '8px',
                  padding: '12px 14px',
                  fontSize: '13.5px',
                  color: C.text,
                  fontFamily: 'var(--font-body)',
                  lineHeight: 1.6,
                  resize: 'vertical',
                  outline: 'none',
                  boxSizing: 'border-box',
                  transition: 'border-color 150ms ease',
                }}
                onFocus={(e) => (e.currentTarget.style.borderColor = C.accent)}
                onBlur={(e) => (e.currentTarget.style.borderColor = C.inputBorder)}
              />

              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: '12px' }}>
                <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.muted }}>
                  {bio.length}/400
                </span>
                <button
                  onClick={handleSave}
                  style={{
                    padding: '8px 20px',
                    background: saved ? 'rgba(0,212,170,0.20)' : C.accent,
                    color: saved ? C.accent : '#061623',
                    border: `1px solid ${saved ? C.accentBorder : 'transparent'}`,
                    borderRadius: '8px',
                    fontSize: '13px',
                    fontWeight: 700,
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '6px',
                    transition: 'all 200ms ease',
                  }}
                  onMouseEnter={(e) => { if (!saved) e.currentTarget.style.background = '#00b896' }}
                  onMouseLeave={(e) => { if (!saved) e.currentTarget.style.background = C.accent }}
                >
                  {saved ? (
                    <>
                      <Icon name="check-circle" size={14} />
                      Saved
                    </>
                  ) : (
                    'Save Bio'
                  )}
                </button>
              </div>
            </div>
          </div>
        </div>

        {/* Sidebar */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {/* Preferred events */}
          <div
            style={{
              background: C.card,
              border: `1px solid ${C.border}`,
              borderRadius: '14px',
              padding: '18px',
              animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.10s both',
            }}
          >
            <h3
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: '16px',
                color: C.text,
                marginBottom: '14px',
                paddingBottom: '12px',
                borderBottom: `1px solid ${C.border}`,
              }}
            >
              Preferred Events
            </h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {SWIMMER_PROFILE.preferredEvents.map((ev) => (
                <div
                  key={ev}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '10px',
                    padding: '8px 12px',
                    background: C.accentDim,
                    border: `1px solid ${C.accentBorder}`,
                    borderRadius: '8px',
                  }}
                >
                  <span
                    style={{
                      width: '6px',
                      height: '6px',
                      borderRadius: '50%',
                      background: C.accent,
                      flexShrink: 0,
                    }}
                  />
                  <span style={{ fontSize: '13px', fontWeight: 500, color: C.text }}>{ev}</span>
                </div>
              ))}
            </div>
            <p style={{ fontSize: '11.5px', color: C.muted, marginTop: '10px', lineHeight: 1.5 }}>
              Preferred events are set by your coach.
            </p>
          </div>

          {/* Parent contact info notice */}
          <div
            style={{
              background: 'rgba(0,0,0,0.20)',
              border: `1px solid ${C.border}`,
              borderRadius: '14px',
              padding: '18px',
              animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.14s both',
            }}
          >
            <h3
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: '16px',
                color: C.text,
                marginBottom: '10px',
              }}
            >
              Equipment
            </h3>
            <ReadOnlyField label="Jersey Size" value={SWIMMER_PROFILE.jerseySize} />
            <p style={{ fontSize: '11.5px', color: C.muted, marginTop: '10px', lineHeight: 1.5 }}>
              To update equipment sizes or emergency notes, ask a parent to update via the parent portal.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
