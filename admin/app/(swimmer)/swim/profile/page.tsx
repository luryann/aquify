'use client'

import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchSwimmers,
  fetchSwimmerGroups,
  fetchGroups,
  type SwimmerRecord,
  type GroupRecord,
} from '@/lib/adminData'

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

// ── Helpers ────────────────────────────────────────────────────────────────
function calcAge(dob: string): number {
  const d = new Date(dob)
  const now = new Date()
  let age = now.getFullYear() - d.getFullYear()
  const m = now.getMonth() - d.getMonth()
  if (m < 0 || (m === 0 && now.getDate() < d.getDate())) age--
  return age
}

function fmtDob(dob: string): string {
  return new Date(dob).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

// ── Skeleton ───────────────────────────────────────────────────────────────
function SkeletonLine({ width = '100%', height = '14px' }: { width?: string; height?: string }) {
  return (
    <div style={{ width, height, background: 'rgba(0,212,170,0.08)', borderRadius: '6px', animation: 'pulse 1.5s ease-in-out infinite' }} />
  )
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
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''

  const [swimmer, setSwimmer] = useState<SwimmerRecord | null>(null)
  const [group, setGroup] = useState<GroupRecord | null>(null)
  const [loading, setLoading] = useState(true)
  const [bio, setBio] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (!orgId) { setLoading(false); return }

    Promise.all([
      fetchSwimmers(api, orgId),
      fetchSwimmerGroups(api, orgId),
      fetchGroups(api, orgId),
    ]).then(([swimmers, swimmerGroupLinks, groups]) => {
      // Resolve swimmer
      const mySwimmer = householdId
        ? (swimmers.find(s => s.household_id === householdId) ?? null)
        : null
      setSwimmer(mySwimmer)

      if (mySwimmer) {
        // Find primary group link for this swimmer
        const myLinks = swimmerGroupLinks.filter(l => l.swimmer_id === mySwimmer.id)
        const primaryLink = myLinks.find(l => l.is_primary) ?? myLinks[0] ?? null
        if (primaryLink) {
          const matchedGroup = groups.find(g => g.code === primaryLink.group_code) ?? null
          setGroup(matchedGroup)
        }
      }
    }).catch(console.error).finally(() => setLoading(false))
  }, [api, orgId, householdId])

  const handleSave = () => {
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  const initials = swimmer
    ? `${swimmer.first_name[0]}${swimmer.last_name[0]}`.toUpperCase()
    : (session?.user.avatarInitials ?? 'SW')

  const displayName = swimmer
    ? `${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`
    : (session?.user.name ?? 'Swimmer')

  return (
    <div>
      <style>{`
        @keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.45} }
        @keyframes slideUp { from{opacity:0;transform:translateY(12px)} to{opacity:1;transform:translateY(0)} }
      `}</style>

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
          {loading ? '…' : initials}
        </div>

        <div style={{ flex: 1 }}>
          {loading ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px', marginTop: '4px' }}>
              <SkeletonLine width="200px" height="28px" />
              <SkeletonLine width="140px" height="14px" />
            </div>
          ) : (
            <>
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
                {displayName}
              </h1>
              <p style={{ fontSize: '13px', color: C.subtle, marginBottom: '10px' }}>
                {group ? group.name : 'No group assigned'}
                {swimmer?.swimmer_external_id ? ` · USS# ${swimmer.swimmer_external_id}` : ''}
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
                    textTransform: 'capitalize',
                  }}
                >
                  {swimmer?.registration_status ?? 'Unknown'}
                </span>
              </div>
            </>
          )}
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
            {loading ? (
              <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '14px', padding: '22px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
                <SkeletonLine width="120px" height="18px" />
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '18px' }}>
                  {[1,2,3,4].map(i => <SkeletonLine key={i} height="40px" />)}
                </div>
              </div>
            ) : (
              <FieldGroup label="Club Information">
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '18px' }}>
                  <ReadOnlyField label="Group" value={group?.name ?? '—'} />
                  <ReadOnlyField label="Group Code" value={group?.code ?? '—'} />
                  <ReadOnlyField label="Registration" value={swimmer?.registration_status ?? '—'} />
                  <ReadOnlyField label="External ID" value={swimmer?.swimmer_external_id ?? '—'} />
                </div>
              </FieldGroup>
            )}
          </div>

          {/* Personal info */}
          <div style={{ animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.12s both' }}>
            {loading ? (
              <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '14px', padding: '22px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
                <SkeletonLine width="160px" height="18px" />
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '18px' }}>
                  {[1,2,3,4,5,6].map(i => <SkeletonLine key={i} height="40px" />)}
                </div>
              </div>
            ) : swimmer ? (
              <FieldGroup label="Personal Information">
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '18px' }}>
                  <ReadOnlyField label="First Name" value={swimmer.first_name} />
                  <ReadOnlyField label="Last Name" value={swimmer.last_name} />
                  {swimmer.preferred_name && (
                    <ReadOnlyField label="Preferred Name" value={swimmer.preferred_name} />
                  )}
                  <ReadOnlyField
                    label="Date of Birth"
                    value={swimmer.date_of_birth ? fmtDob(swimmer.date_of_birth) : '—'}
                  />
                  <ReadOnlyField
                    label="Age"
                    value={swimmer.date_of_birth ? String(calcAge(swimmer.date_of_birth)) : '—'}
                  />
                  <ReadOnlyField
                    label="Gender"
                    value={swimmer.gender
                      ? swimmer.gender.charAt(0).toUpperCase() + swimmer.gender.slice(1)
                      : '—'}
                  />
                </div>
                {swimmer.medical_flags && (
                  <div>
                    <ReadOnlyField label="Medical Notes" value={swimmer.medical_flags?.join('; ') ?? ''} />
                  </div>
                )}
              </FieldGroup>
            ) : (
              <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '14px', padding: '24px', textAlign: 'center' }}>
                <p style={{ fontSize: '14px', color: C.muted }}>No swimmer profile found for your account.</p>
              </div>
            )}
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
                <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '17px', color: C.text, letterSpacing: '-0.01em' }}>
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
          {/* Group card */}
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
              My Group
            </h3>
            {loading ? (
              <SkeletonLine />
            ) : group ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '10px 12px', background: C.accentDim, border: `1px solid ${C.accentBorder}`, borderRadius: '8px' }}>
                  <span style={{ width: '6px', height: '6px', borderRadius: '50%', background: C.accent, flexShrink: 0 }} />
                  <div>
                    <p style={{ fontSize: '14px', fontWeight: 600, color: C.text }}>{group.name}</p>
                    <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.muted }}>{group.code}</p>
                  </div>
                </div>
                {(group.min_age != null || group.max_age != null) && (
                  <p style={{ fontSize: '11.5px', color: C.muted }}>
                    Age range: {group.min_age ?? '—'} – {group.max_age ?? '—'}
                  </p>
                )}
              </div>
            ) : (
              <p style={{ fontSize: '13px', color: C.muted }}>No group assigned yet.</p>
            )}
          </div>

          {/* Notes / Equipment sidebar */}
          <div
            style={{
              background: 'rgba(0,0,0,0.20)',
              border: `1px solid ${C.border}`,
              borderRadius: '14px',
              padding: '18px',
              animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.14s both',
            }}
          >
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '16px', color: C.text, marginBottom: '10px' }}>
              Notes
            </h3>
            {loading ? (
              <SkeletonLine />
            ) : swimmer?.notes ? (
              <p style={{ fontSize: '13px', color: C.subtle, lineHeight: 1.6 }}>{swimmer.notes}</p>
            ) : (
              <p style={{ fontSize: '13px', color: C.muted, lineHeight: 1.5 }}>No notes on file.</p>
            )}
            <p style={{ fontSize: '11.5px', color: C.muted, marginTop: '10px', lineHeight: 1.5 }}>
              To update equipment sizes or emergency notes, ask a parent to update via the parent portal.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
