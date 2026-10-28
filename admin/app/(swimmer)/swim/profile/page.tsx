'use client'

import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError, uiErrorMessage } from '@/lib/frontend-telemetry'
import { UnlinkedSwimmerNotice } from '@/components/swimmer/UnlinkedSwimmerNotice'
import {
  fetchSwimmers,
  fetchSwimmerGroups,
  fetchGroups,
  updateSwimmer,
  type SwimmerRecord,
  type GroupRecord,
} from '@/lib/adminData'
import { SWIMMER_BIO_MAX_LENGTH, normalizeSwimmerBio, validateSwimmerBio } from '@/lib/swimmer-bio'
import './page.inline.css'

// ── Design tokens ──────────────────────────────────────────────────────────
const C = {
  accent:       'var(--accent)',
  accentDim:    'var(--border)',
  accentBorder: 'var(--accent-border)',
  text:         'var(--text)',
  subtle:       'var(--text-subtle)',
  muted:        'var(--text-muted)',
  card:         'var(--bg-surface)',
  border:       'var(--border)',
  inputBg:      'var(--border-strong)',
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
    <div style={{ width, height, background: 'var(--border)', borderRadius: '6px', animation: 'pulse 1.5s ease-in-out infinite' }} />
  )
}

// ── Components ─────────────────────────────────────────────────────────────
function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p
        style={{
          fontSize: 'var(--fs-sm)',
          fontFamily: 'var(--font-body)',
          color: C.muted,
          marginBottom: '4px',
        }}
      >
        {label}
      </p>
      <p style={{ fontSize: 'var(--fs-md)', color: C.text, fontWeight: 500, lineHeight: 1.4 }}>
        {value}
      </p>
    </div>
  )
}

function FieldGroup({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="card is-f4ffd4e"
      
    >
      <h3 className="section-title" style={{ paddingBottom: '16px', borderBottom: `1px solid ${C.border}`, margin: '-4px 0 0' }}
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
  const swimmerId = session?.user.swimmerId ?? ''

  const [swimmer, setSwimmer] = useState<SwimmerRecord | null>(null)
  const [group, setGroup] = useState<GroupRecord | null>(null)
  const [loading, setLoading] = useState(true)
  const [bio, setBio] = useState('')
  const [bioSaving, setBioSaving] = useState(false)
  const [bioError, setBioError] = useState<string | null>(null)
  const [bioSaved, setBioSaved] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [noLinkedSwimmer, setNoLinkedSwimmer] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId) {
      setLoading(false)
      setLoadError('We could not find your club. Sign out and back in to try again.')
      return
    }

    let active = true
    setLoading(true)
    setLoadError(null)

    Promise.all([
      fetchSwimmers(api, orgId),
      fetchSwimmerGroups(api, orgId),
      fetchGroups(api, orgId),
    ]).then(([swimmers, swimmerGroupLinks, groups]) => {
      if (!active) return
      // Resolve "which swimmer is me" by the stable swimmer_id on the
      // session, not by guessing a household roster entry.
      const mySwimmer = swimmerId
        ? (swimmers.find(s => s.id === swimmerId) ?? null)
        : null
      setSwimmer(mySwimmer)
      setNoLinkedSwimmer(!swimmerId)
      setBio(mySwimmer?.notes ?? '')

      if (mySwimmer) {
        // Find primary group link for this swimmer
        const myLinks = swimmerGroupLinks.filter(l => l.swimmer_id === mySwimmer.id)
        const primaryLink = myLinks.find(l => l.is_primary) ?? myLinks[0] ?? null
        if (primaryLink) {
          const matchedGroup = groups.find(g => g.code === primaryLink.group_code) ?? null
          setGroup(matchedGroup)
        }
      }
    }).catch((error) => {
      if (!active) return
      setSwimmer(null)
      setGroup(null)
      setBio('')
      setLoadError(reportUIError(
        'swimmer-profile-load',
        error,
        'We could not load this page. Check your connection and try again.',
        { orgId, householdId },
      ))
    }).finally(() => { if (active) setLoading(false) })

    return () => { active = false }
  }, [api, orgId, householdId, swimmerId, reloadKey])

  const saveBio = async () => {
    if (!swimmer || !orgId || loading || bioSaving) return

    const validationError = validateSwimmerBio(bio)
    if (validationError) {
      setBioError(validationError)
      setBioSaved(false)
      return
    }

    setBioSaving(true)
    setBioError(null)
    setBioSaved(false)
    try {
      const updated = await updateSwimmer(api, orgId, swimmer.id, { notes: normalizeSwimmerBio(bio) })
      setSwimmer(updated)
      setBio(updated.notes ?? '')
      setBioSaved(true)
    } catch (error) {
      setBioError(uiErrorMessage(error, 'Couldn’t save bio. Try again.'))
    } finally {
      setBioSaving(false)
    }
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
        className="is-ce0e9fe"
      >
        {/* Avatar */}
        <div
          className="is-81e81d8"
        >
          {loading ? '…' : initials}
        </div>

        <div style={{ flex: 1 }}>
          {loading ? (
            <div className="is-c284387">
              <SkeletonLine width="200px" height="28px" />
              <SkeletonLine width="140px" height="14px" />
            </div>
          ) : (
            <>
              <h1 className="page-title"
              >
                {displayName}
              </h1>
              <p style={{ fontSize: 'var(--fs-base)', color: C.subtle, marginBottom: '12px' }}>
                {group ? group.name : 'No group assigned'}
                {swimmer?.swimmer_external_id ? ` · USS# ${swimmer.swimmer_external_id}` : ''}
              </p>
              <div className="is-51144f2">
                <span
                  className="is-74e24b2"
                >
                  {swimmer?.registration_status ?? 'Not set'}
                </span>
              </div>
            </>
          )}
        </div>
      </div>

      {/* Read-only notice */}
      <div
        style={{
          background: 'var(--border)',
          border: `1px solid ${C.accentBorder}`,
          borderRadius: '12px',
          padding: '12px 16px',
          display: 'flex',
          gap: '12px',
          alignItems: 'flex-start',
          marginBottom: '24px',
          animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.05s both',
        }}
      >
        <Icon name="info" size={15} style={{ color: C.accent, marginTop: '1px', flexShrink: 0 }} />
        <p style={{ fontSize: 'var(--fs-sm)', color: C.subtle, lineHeight: 1.5 }}>
          Most profile fields are managed by the club or your parent account. You can update your bio below. To change other information, please ask a parent to update it via the parent portal.
        </p>
      </div>

      {loadError && (
        <div
          className="is-ef8d534"
        >
          <div className="is-730b6a7">
            <Icon name="alert-circle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
            <p style={{ margin: 0, fontSize: 'var(--fs-sm)', color: C.text, lineHeight: 1.5 }}>{loadError}</p>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{
              padding: '8px 16px',
              borderRadius: '8px',
              border: '1px solid rgba(245,158,11,0.25)',
              background: 'transparent',
              color: C.text,
              fontSize: 'var(--fs-sm)',
              fontWeight: 600,
              cursor: 'pointer',
              whiteSpace: 'nowrap',
            }}
          >
            Retry
          </button>
        </div>
      )}

      {!loading && !loadError && noLinkedSwimmer && <UnlinkedSwimmerNotice />}

      <div
        className="split-aside is-15cde2f"
        
      >
        {/* Main column */}
        <div className="is-8ef476f">
          {/* Club info */}
          <div style={{ animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.08s both' }}>
            {loading ? (
              <div className="card is-c9491b0" >
                <SkeletonLine width="120px" height="18px" />
                <div className="cols-2 is-ac2cd18" >
                  {[1,2,3,4].map(i => <SkeletonLine key={i} height="40px" />)}
                </div>
              </div>
            ) : (
              <FieldGroup label="Club information">
                <div className="cols-2 is-ac2cd18" >
                  <ReadOnlyField label="Group" value={group?.name ?? '—'} />
                  <ReadOnlyField label="Group code" value={group?.code ?? '—'} />
                  <ReadOnlyField label="Registration" value={swimmer?.registration_status ?? '—'} />
                  <ReadOnlyField label="External ID" value={swimmer?.swimmer_external_id ?? '—'} />
                </div>
              </FieldGroup>
            )}
          </div>

          {/* Personal info */}
          <div style={{ animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.12s both' }}>
            {loading ? (
              <div className="card is-c9491b0" >
                <SkeletonLine width="160px" height="18px" />
                <div className="cols-2 is-ac2cd18" >
                  {[1,2,3,4,5,6].map(i => <SkeletonLine key={i} height="40px" />)}
                </div>
              </div>
            ) : swimmer ? (
              <FieldGroup label="Personal information">
                <div className="cols-2 is-ac2cd18" >
                  <ReadOnlyField label="First name" value={swimmer.first_name} />
                  <ReadOnlyField label="Last name" value={swimmer.last_name} />
                  {swimmer.preferred_name && (
                    <ReadOnlyField label="Preferred name" value={swimmer.preferred_name} />
                  )}
                  <ReadOnlyField
                    label="Date of birth"
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
                    <ReadOnlyField label="Medical notes" value={swimmer.medical_flags?.join('; ') ?? ''} />
                  </div>
                )}
                {swimmer.allergies && (
                  <div>
                    <ReadOnlyField label="Allergies" value={swimmer.allergies} />
                  </div>
                )}
                {(swimmer.emergency_contact_name || swimmer.emergency_contact_phone) && (
                  <div className="cols-2 is-ac2cd18" >
                    <ReadOnlyField label="Emergency contact" value={swimmer.emergency_contact_name || '—'} />
                    <ReadOnlyField label="Emergency phone" value={swimmer.emergency_contact_phone || '—'} />
                    {swimmer.emergency_contact_relationship && (
                      <ReadOnlyField label="Relationship" value={swimmer.emergency_contact_relationship} />
                    )}
                  </div>
                )}
              </FieldGroup>
            ) : shouldShowCollectionEmptyState(loading, loadError, swimmer ? 1 : 0) ? (
              noLinkedSwimmer ? null : (
                <div className="card is-0830bd2" >
                  <p style={{ fontSize: 'var(--fs-md)', color: C.muted }}>No swimmer profile found for your account.</p>
                </div>
              )
            ) : null}
          </div>

          {/* Bio — editable */}
          <div style={{ animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.16s both' }}>
            <div className="card is-0acd099"
              
            >
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px', paddingBottom: '16px', borderBottom: `1px solid ${C.border}` }}>
                <h3 className="section-title">
                  My Bio
                </h3>
                <span
                  style={{
                    fontSize: 'var(--fs-2xs)',
                    fontFamily: 'var(--font-body)',
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

              <p style={{ fontSize: 'var(--fs-sm)', color: C.muted, marginBottom: '12px', lineHeight: 1.5 }}>
                Share a little about yourself — this appears on your team profile.
              </p>

              <textarea aria-label="Bio"
                disabled={loading || !swimmer || bioSaving}
                value={bio}
                onChange={(e) => {
                  setBio(e.target.value)
                  if (bioError) setBioError(null)
                  if (bioSaved) setBioSaved(false)
                }}
                placeholder="Tell your team about yourself. Your goals, favorite events, or anything you'd like to share..."
                maxLength={SWIMMER_BIO_MAX_LENGTH}
                rows={4}
                style={{ width: '100%',
                  lineHeight: 1.6,
                  resize: 'vertical' }}
              />

              {bioError && (
                <div className="is-e53b553">
                  {bioError}
                </div>
              )}

              <div className="is-7d1aea8">
                <span style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: C.muted }}>
                  {bio.length}/{SWIMMER_BIO_MAX_LENGTH}
                </span>
                <div className="is-e2ff0fd">
                  {bioSaved && (
                    <span style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: C.accent }}>
                      Saved
                    </span>
                  )}
                  <button type="button"
                    onClick={() => void saveBio()}
                    disabled={loading || !swimmer || bioSaving}
                    style={{
                      padding: '8px 20px',
                      background: bioSaving ? 'var(--border)' : C.accentDim,
                      color: bioSaving ? C.muted : C.accent,
                      border: `1px solid ${bioSaving ? 'var(--accent-subtle)' : C.accentBorder}`,
                      borderRadius: '8px',
                      fontSize: 'var(--fs-base)',
                      fontWeight: 700,
                      cursor: loading || !swimmer || bioSaving ? 'not-allowed' : 'pointer',
                      opacity: loading || !swimmer ? 0.65 : 1,
                      display: 'flex',
                      alignItems: 'center',
                      gap: '6px',
                    }}
                  >
                    {bioSaving ? 'Saving...' : 'Save Bio'}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Sidebar */}
        <div className="is-8ef476f">
          {/* Group card */}
          <div className="card is-b67ba5f"
            
          >
            <h3 className="section-title" style={{ marginBottom: '16px', paddingBottom: '12px', borderBottom: `1px solid ${C.border}` }}
            >
              My Group
            </h3>
            {loading ? (
              <SkeletonLine />
            ) : group ? (
              <div className="is-930d87f">
                <div style={{ display: 'flex', alignItems: 'center', gap: '12px', padding: '12px 12px', background: C.accentDim, border: `1px solid ${C.accentBorder}`, borderRadius: '8px' }}>
                  <span style={{ width: '6px', height: '6px', borderRadius: '50%', background: C.accent, flexShrink: 0 }} />
                  <div>
                    <p style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: C.text }}>{group.name}</p>
                    <p style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: C.muted }}>{group.code}</p>
                  </div>
                </div>
                {(group.min_age != null || group.max_age != null) && (
                  <p style={{ fontSize: 'var(--fs-xs)', color: C.muted }}>
                    Age range: {group.min_age ?? '—'} – {group.max_age ?? '—'}
                  </p>
                )}
              </div>
            ) : (
              <p style={{ fontSize: 'var(--fs-base)', color: C.muted }}>No group assigned yet.</p>
            )}
          </div>

          {/* Saved bio sidebar */}
          <div
            style={{
              background: 'rgba(0,0,0,0.20)',
              border: `1px solid ${C.border}`,
              borderRadius: '12px',
              padding: '20px',
              animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.14s both',
            }}
          >
            <h3 className="section-title is-72d6c38" >
              Saved bio
            </h3>
            {loading ? (
              <SkeletonLine />
            ) : swimmer?.notes ? (
              <p style={{ fontSize: 'var(--fs-base)', color: C.subtle, lineHeight: 1.6 }}>{swimmer.notes}</p>
            ) : (
              <p style={{ fontSize: 'var(--fs-base)', color: C.muted, lineHeight: 1.5 }}>No bio saved yet.</p>
            )}
            <p style={{ fontSize: 'var(--fs-xs)', color: C.muted, marginTop: '12px', lineHeight: 1.5 }}>
              This panel reflects the currently saved bio text from your swimmer profile record.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
