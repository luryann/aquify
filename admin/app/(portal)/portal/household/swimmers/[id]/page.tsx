'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  fetchSwimmerActivity,
  fetchSwimmer,
  fetchSwimmerGroups,
  fetchGroups,
  fetchSwimmerBestTimes,
  updateSwimmer,
  type SwimmerRecord,
  type BestTimeRecord,
  type SwimmerActivityRecord,
} from '@/lib/adminData'

// ── Helpers ─────────────────────────────────────────────────────────────────
function fmtDOB(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function calcAge(iso: string | undefined | null): number | null {
  if (!iso) return null
  const d = new Date(iso)
  const now = new Date()
  let age = now.getFullYear() - d.getFullYear()
  const m = now.getMonth() - d.getMonth()
  if (m < 0 || (m === 0 && now.getDate() < d.getDate())) age--
  return age
}

function strokeLabel(stroke: string): string {
  const map: Record<string, string> = {
    freestyle: 'Free', backstroke: 'Back', breaststroke: 'Breast',
    butterfly: 'Fly', im: 'IM', individual_medley: 'IM',
  }
  return map[stroke.toLowerCase()] ?? stroke
}

function eventKey(distance: number, stroke: string): string {
  return `${distance} ${strokeLabel(stroke)}`
}

function fmtTime(ms: number): string {
  const totalSec = ms / 1000
  const mins = Math.floor(totalSec / 60)
  const secs = totalSec - mins * 60
  const secStr = secs.toFixed(2).padStart(5, '0')
  return mins > 0 ? `${mins}:${secStr}` : secStr
}

type TimeRow = { event: string; scy: string; scm: string; lcm: string }

function pivotBestTimes(records: BestTimeRecord[]): TimeRow[] {
  const byEvent = new Map<string, { scy: string; scm: string; lcm: string }>()
  for (const rec of records) {
    const key = eventKey(rec.event_distance, rec.stroke)
    if (!byEvent.has(key)) byEvent.set(key, { scy: '—', scm: '—', lcm: '—' })
    const row = byEvent.get(key)!
    const course = rec.course.toLowerCase()
    if ((course === 'scy' || course === 'scm' || course === 'lcm') && rec.best_time_ms != null) {
      row[course] = fmtTime(rec.best_time_ms)
    }
  }
  return Array.from(byEvent.entries()).map(([event, times]) => ({ event, ...times }))
}

const GRADIENTS = [
  'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
  'linear-gradient(135deg, #10b981 0%, #059669 100%)',
  'linear-gradient(135deg, #f59e0b 0%, #d97706 100%)',
  'linear-gradient(135deg, #ef4444 0%, #dc2626 100%)',
]

function Skeleton({ w = '100%', h = 16, r = 8 }: { w?: string | number; h?: number; r?: number }) {
  return (
    <div
      style={{
        width: w, height: h, borderRadius: r,
        background: 'linear-gradient(90deg, #f0efee 25%, #e6e5e3 50%, #f0efee 75%)',
        backgroundSize: '200% 100%',
        animation: 'shimmer 1.5s infinite',
      }}
    />
  )
}

const STATUS_CONFIG: Record<string, { label: string; bg: string; color: string }> = {
  active:   { label: 'Active',   bg: 'var(--success-bg)', color: 'var(--success)' },
  inactive: { label: 'Inactive', bg: 'var(--error-bg)', color: 'var(--error)' },
  pending:  { label: 'Pending',  bg: 'var(--warning-bg)', color: 'var(--warning)' },
}

const EDITABLE_FIELD_NOTE =
  'Fields marked with a pencil icon can be updated by you. Other fields are managed by club staff.'

type Tab = 'profile' | 'performance' | 'history'

function fmtShortDate(iso?: string): string {
  if (!iso) return '—'
  const value = new Date(iso)
  if (Number.isNaN(value.getTime())) return '—'
  return value.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function historyAccent(type: string): { bg: string; color: string } {
  switch (type) {
    case 'result':
      return { bg: 'rgba(59,130,246,0.12)', color: 'var(--info)' }
    case 'registration':
      return { bg: 'rgba(16,185,129,0.12)', color: 'var(--success)' }
    case 'waiver':
      return { bg: 'rgba(245,158,11,0.15)', color: '#b45309' }
    case 'document':
      return { bg: 'rgba(235,93,30,0.12)', color: '#c2410c' }
    case 'commitment':
      return { bg: 'rgba(107,114,128,0.12)', color: '#4b5563' }
    default:
      return { bg: 'rgba(0,0,0,0.06)', color: 'var(--text-subtle)' }
  }
}

// ── Component ────────────────────────────────────────────────────────────────
export default function SwimmerProfilePage() {
  const params = useParams<{ id: string }>()
  const swimmerId = typeof params?.id === 'string' ? params.id : ''

  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [swimmer, setSwimmer] = useState<SwimmerRecord | null>(null)
  const [groupName, setGroupName] = useState<string>('—')
  const [bestTimes, setBestTimes] = useState<TimeRow[]>([])
  const [activity, setActivity] = useState<SwimmerActivityRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  // Editable local state (initialized from swimmer data on load)
  const [emergencyNotes, setEmergencyNotes] = useState('')
  const [preferredName, setPreferredName] = useState('')
  const [emergencyContactName, setEmergencyContactName] = useState('')
  const [emergencyContactPhone, setEmergencyContactPhone] = useState('')
  const [emergencyContactRelationship, setEmergencyContactRelationship] = useState('')
  const [allergies, setAllergies] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saveSuccess, setSaveSuccess] = useState(false)

  useEffect(() => {
    if (!orgId || !swimmerId) {
      setLoading(false)
      setLoadError('Swimmer detail is unavailable because the swimmer context is missing.')
      return
    }
    setLoading(true)
    setLoadError(null)
    Promise.allSettled([
      fetchSwimmer(api, orgId, swimmerId),
      fetchSwimmerGroups(api, orgId),
      fetchGroups(api, orgId),
      fetchSwimmerBestTimes(api, orgId, swimmerId),
      fetchSwimmerActivity(api, orgId, swimmerId),
    ])
      .then(([swResult, sGroupsResult, grpsResult, timesResult, activityResult]) => {
        if (swResult.status === 'rejected') {
          setSwimmer(null)
          setGroupName('—')
          setBestTimes([])
          setActivity([])
          setLoadError(reportUIError(
            'portal-household-swimmer-load',
            swResult.reason,
            'Swimmer detail data is unavailable right now.',
            { orgId, swimmerId },
          ))
          return
        }

        const sw = swResult.value
        setSwimmer(sw)
        setEmergencyNotes(sw.notes ?? '')
        setPreferredName(sw.preferred_name ?? '')
        setEmergencyContactName(sw.emergency_contact_name ?? '')
        setEmergencyContactPhone(sw.emergency_contact_phone ?? '')
        setEmergencyContactRelationship(sw.emergency_contact_relationship ?? '')
        setAllergies(sw.allergies ?? '')

        if (sGroupsResult.status === 'fulfilled' && grpsResult.status === 'fulfilled') {
          const sGroups = sGroupsResult.value
          const grps = grpsResult.value
          const link = sGroups.find((sg) => sg.swimmer_id === sw.id && sg.is_primary)
          if (link) {
            const grp = grps.find((g) => g.code === link.group_code)
            setGroupName(grp?.name ?? link.group_code ?? '—')
          }
        }

        if (timesResult.status === 'fulfilled') {
          setBestTimes(pivotBestTimes(timesResult.value))
        }
        if (activityResult.status === 'fulfilled') {
          setActivity(activityResult.value)
        }
      })
      .finally(() => setLoading(false))
  }, [api, orgId, swimmerId, reloadKey])

  const [tab, setTab] = useState<Tab>('profile')
  const [editingName, setEditingName] = useState(false)
  const [editingNotes, setEditingNotes] = useState(false)
  const [editingEmergencyContact, setEditingEmergencyContact] = useState(false)

  async function handleSave() {
    if (!orgId || !swimmerId) return
    setSaving(true)
    setSaveError(null)
    setSaveSuccess(false)
    try {
      const updated = await updateSwimmer(api, orgId, swimmerId, {
        preferred_name: preferredName,
        notes: emergencyNotes,
        emergency_contact_name: emergencyContactName,
        emergency_contact_phone: emergencyContactPhone,
        emergency_contact_relationship: emergencyContactRelationship,
        allergies,
      })
      setSwimmer(updated)
      setEmergencyNotes(updated.notes ?? '')
      setPreferredName(updated.preferred_name ?? '')
      setEmergencyContactName(updated.emergency_contact_name ?? '')
      setEmergencyContactPhone(updated.emergency_contact_phone ?? '')
      setEmergencyContactRelationship(updated.emergency_contact_relationship ?? '')
      setAllergies(updated.allergies ?? '')
      setEditingName(false)
      setEditingNotes(false)
      setEditingEmergencyContact(false)
      setSaveSuccess(true)
      setTimeout(() => setSaveSuccess(false), 3000)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save changes.')
    } finally {
      setSaving(false)
    }
  }

  const fullName = swimmer ? `${swimmer.first_name} ${swimmer.last_name}` : '—'
  const initials = swimmer
    ? `${swimmer.first_name[0] ?? ''}${swimmer.last_name[0] ?? ''}`.toUpperCase()
    : '?'
  const age = calcAge(swimmer?.date_of_birth)
  const statusKey = swimmer?.registration_status ?? 'active'
  const statusCfg = STATUS_CONFIG[statusKey] ?? STATUS_CONFIG['active']
  const gradient = GRADIENTS[0]


  return (
    <div style={{ maxWidth: '820px' }}>
      <style>{`
        @keyframes slideUp { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
      `}</style>

      {/* Breadcrumb */}
      <nav style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '13px', color: 'var(--text-muted)', marginBottom: '28px', animation: 'slideUp 0.3s ease both' }}>
        <Link href="/portal/household" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>
          Household
        </Link>
        <span>/</span>
        <span style={{ color: 'var(--text)', fontWeight: 500 }}>
          {loading ? 'Loading…' : fullName}
        </span>
      </nav>

      {loadError && (
        <div
          style={{
            background: 'var(--warning-bg)',
            border: '1px solid #fdba74',
            borderRadius: '10px',
            padding: '12px 16px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: '12px',
            marginBottom: '24px',
          }}
        >
          <div style={{ display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
            <Icon name="alert-circle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
            <p style={{ margin: 0, fontSize: '12.5px', color: 'var(--warning)', lineHeight: 1.5 }}>{loadError}</p>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{ background: 'var(--bg-elevated)', border: '1px solid #fdba74', borderRadius: '8px', cursor: 'pointer', color: 'var(--warning)', fontSize: '12px', fontWeight: 600, padding: '8px 14px', whiteSpace: 'nowrap' }}
          >
            Retry
          </button>
        </div>
      )}

      {/* Header Card */}
      <div className="card" style={{ boxShadow: '0 2px 12px rgba(0,0,0,0.05)', padding: '28px', marginBottom: '24px', display: 'flex', alignItems: 'center', gap: '20px', animation: 'slideUp 0.35s ease both' }}>
        {loading ? (
          <>
            <Skeleton w={72} h={72} r={36} />
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '10px' }}>
              <Skeleton w="40%" h={26} />
              <Skeleton w="65%" h={14} />
            </div>
          </>
        ) : (
          <>
            {/* Avatar */}
            <div style={{ width: '72px', height: '72px', borderRadius: '50%', background: gradient, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--bg-elevated)', fontSize: '24px', fontWeight: 700, fontFamily: 'var(--font-body)', flexShrink: 0, boxShadow: '0 4px 16px rgba(59,130,246,0.3)' }}>
              {initials}
            </div>

            {/* Info */}
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
                <h1 className="page-title">
                  {fullName}
                </h1>
                <span style={{ padding: '3px 9px', borderRadius: '999px', fontSize: '11.5px', fontWeight: 600, background: statusCfg.bg, color: statusCfg.color, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  {statusCfg.label}
                </span>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '16px', marginTop: '8px', fontSize: '13px', color: 'var(--text-subtle)' }}>
                {age !== null && <span>Age {age}</span>}
                {swimmer?.gender && <><span>·</span><span>{swimmer.gender}</span></>}
                {groupName !== '—' && <><span>·</span><span>{groupName}</span></>}
                {swimmer?.swimmer_external_id && <><span>·</span><span style={{ fontFamily: 'var(--font-body)', fontSize: '12px' }}>{swimmer.swimmer_external_id}</span></>}
              </div>
            </div>

          </>
        )}
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', gap: '2px', marginBottom: '24px', background: 'rgba(0,0,0,0.04)', borderRadius: '10px', padding: '4px', animation: 'slideUp 0.4s ease both' }}>
        {(['profile', 'performance', 'history'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{ flex: 1, padding: '8px 0', borderRadius: '7px', border: 'none', background: tab === t ? 'var(--bg-elevated)' : 'transparent', color: tab === t ? 'var(--text)' : 'var(--text-subtle)', fontSize: '13.5px', fontWeight: tab === t ? 600 : 450, cursor: 'pointer', boxShadow: tab === t ? '0 1px 4px rgba(0,0,0,0.08)' : 'none', transition: 'all 120ms ease', fontFamily: 'var(--font-body)', textTransform: 'capitalize' }}
          >
            {t}
          </button>
        ))}
      </div>

      {/* ── PROFILE TAB ─────────────────────────────────────────────────────── */}
      {tab === 'profile' && (
        <div style={{ animation: 'slideUp 0.3s ease both' }}>
          {/* Edit policy notice */}
          <div style={{ background: 'rgba(235,93,30,0.06)', border: '1px solid rgba(235,93,30,0.15)', borderRadius: '10px', padding: '12px 16px', marginBottom: '20px', display: 'flex', alignItems: 'flex-start', gap: '10px' }}>
            <Icon name="info" size={15} style={{ color: 'var(--accent)', flexShrink: 0, marginTop: '2px' }} />
            <p style={{ margin: 0, fontSize: '13px', color: '#7A4A28', lineHeight: 1.5 }}>
              {EDITABLE_FIELD_NOTE}
            </p>
          </div>

          {/* Read-only Fields */}
          <div className="card" style={{ boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden', marginBottom: '20px' }}>
            <div style={{ padding: '16px 20px 12px', borderBottom: '1px solid rgba(0,0,0,0.06)', display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
                Club-managed fields
              </span>
              <span style={{ padding: '2px 7px', background: 'rgba(0,0,0,0.05)', borderRadius: '4px', fontSize: '10.5px', color: 'var(--text-muted)' }}>
                read-only
              </span>
            </div>

            {loading ? (
              <div style={{ padding: '20px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {[0, 1, 2, 3, 4].map((i) => <Skeleton key={i} w={`${60 + i * 5}%`} h={14} />)}
              </div>
            ) : (
              [
                { label: 'Full Name',           value: fullName },
                { label: 'Date of Birth',        value: fmtDOB(swimmer?.date_of_birth) },
                { label: 'Age',                  value: age !== null ? `${age} years old` : '—' },
                { label: 'Gender',               value: swimmer?.gender ?? '—' },
                { label: 'External ID',          value: swimmer?.swimmer_external_id ?? '—', mono: true },
                { label: 'Training Group',       value: groupName },
                { label: 'Registration Status',  value: swimmer?.registration_status ?? '—' },
              ].map((field, i, arr) => (
                <div key={field.label} style={{ display: 'flex', padding: '14px 20px', borderBottom: i < arr.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none', gap: '24px', alignItems: 'center' }}>
                  <span style={{ width: '160px', fontSize: '13px', color: 'var(--text-muted)', flexShrink: 0 }}>
                    {field.label}
                  </span>
                  <span style={{ fontSize: '13.5px', color: 'var(--text-subtle)', fontFamily: field.mono ? 'var(--font-body)' : undefined }}>
                    {field.value}
                  </span>
                </div>
              ))
            )}
          </div>

          {/* Editable Fields */}
          <div className="card" style={{ boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden', marginBottom: '20px' }}>
            <div style={{ padding: '16px 20px 12px', borderBottom: '1px solid rgba(0,0,0,0.06)', display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
                Parent-editable fields
              </span>
              <Icon name="edit" size={12} style={{ color: 'var(--accent)' }} />
            </div>

            {/* Preferred Name */}
            <div style={{ padding: '16px 20px', borderBottom: '1px solid rgba(0,0,0,0.05)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '4px' }}>Preferred Name / Nickname</div>
                  {editingName ? (
                    <input
                      autoFocus
                      value={preferredName}
                      onChange={(e) => setPreferredName(e.target.value)}
                      style={{ width: '100%', maxWidth: '280px', padding: '7px 11px', border: '1.5px solid var(--accent)', borderRadius: '8px', fontSize: '14px', color: 'var(--text)', fontFamily: 'var(--font-body)', outline: 'none', background: 'var(--bg-elevated)' }}
                    />
                  ) : (
                    <div style={{ fontSize: '14px', color: 'var(--text)', fontWeight: 500 }}>
                      {preferredName || <span style={{ color: 'var(--border-strong)', fontStyle: 'italic' }}>Not set</span>}
                    </div>
                  )}
                </div>
                <button onClick={() => setEditingName(!editingName)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--accent)', fontSize: '12.5px', display: 'flex', alignItems: 'center', gap: '4px', padding: '4px 8px', borderRadius: '6px', transition: 'background 100ms' }} onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(235,93,30,0.06)')} onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}>
                  <Icon name="edit" size={12} />
                  {editingName ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>

            {/* Emergency Notes */}
            <div style={{ padding: '16px 20px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '4px' }}>Medical / Emergency Notes</div>
                  {editingNotes ? (
                    <textarea
                      autoFocus
                      value={emergencyNotes}
                      onChange={(e) => setEmergencyNotes(e.target.value)}
                      rows={4}
                      style={{ width: '100%', padding: '9px 12px', border: '1.5px solid var(--accent)', borderRadius: '8px', fontSize: '13.5px', color: 'var(--text)', fontFamily: 'var(--font-body)', outline: 'none', background: 'var(--bg-elevated)', resize: 'vertical', lineHeight: 1.5 }}
                    />
                  ) : (
                    <div style={{ fontSize: '13.5px', color: 'var(--text)', lineHeight: 1.6, maxWidth: '500px' }}>
                      {emergencyNotes || <span style={{ color: 'var(--border-strong)', fontStyle: 'italic' }}>No notes added</span>}
                    </div>
                  )}
                  <div style={{ fontSize: '11.5px', color: 'var(--border-strong)', marginTop: '6px' }}>
                    Visible to coaches and club staff during meets and practices.
                  </div>
                </div>
                <button onClick={() => setEditingNotes(!editingNotes)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--accent)', fontSize: '12.5px', display: 'flex', alignItems: 'center', gap: '4px', padding: '4px 8px', borderRadius: '6px', transition: 'background 100ms', flexShrink: 0 }} onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(235,93,30,0.06)')} onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}>
                  <Icon name="edit" size={12} />
                  {editingNotes ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>

            {/* Emergency Contact + Allergies */}
            <div style={{ padding: '16px 20px', borderTop: '1px solid rgba(0,0,0,0.05)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '10px' }}>Emergency Contact</div>
                  {editingEmergencyContact ? (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px', maxWidth: '440px' }}>
                      <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: '10px' }}>
                        <input
                          autoFocus
                          value={emergencyContactName}
                          onChange={(e) => setEmergencyContactName(e.target.value)}
                          placeholder="Contact name"
                          style={{ padding: '7px 11px', border: '1.5px solid var(--accent)', borderRadius: '8px', fontSize: '13.5px', color: 'var(--text)', fontFamily: 'var(--font-body)', outline: 'none', background: 'var(--bg-elevated)' }}
                        />
                        <input
                          value={emergencyContactRelationship}
                          onChange={(e) => setEmergencyContactRelationship(e.target.value)}
                          placeholder="Relationship"
                          style={{ padding: '7px 11px', border: '1.5px solid var(--accent)', borderRadius: '8px', fontSize: '13.5px', color: 'var(--text)', fontFamily: 'var(--font-body)', outline: 'none', background: 'var(--bg-elevated)' }}
                        />
                      </div>
                      <input
                        value={emergencyContactPhone}
                        onChange={(e) => setEmergencyContactPhone(e.target.value)}
                        placeholder="Phone number"
                        type="tel"
                        style={{ padding: '7px 11px', border: '1.5px solid var(--accent)', borderRadius: '8px', fontSize: '13.5px', color: 'var(--text)', fontFamily: 'var(--font-body)', outline: 'none', background: 'var(--bg-elevated)' }}
                      />
                      <div>
                        <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginBottom: '4px' }}>Allergies</div>
                        <input
                          value={allergies}
                          onChange={(e) => setAllergies(e.target.value)}
                          placeholder="e.g. Peanuts, bee stings"
                          style={{ width: '100%', padding: '7px 11px', border: '1.5px solid var(--accent)', borderRadius: '8px', fontSize: '13.5px', color: 'var(--text)', fontFamily: 'var(--font-body)', outline: 'none', background: 'var(--bg-elevated)' }}
                        />
                      </div>
                    </div>
                  ) : (
                    <div style={{ fontSize: '13.5px', color: 'var(--text)', lineHeight: 1.7 }}>
                      {emergencyContactName || emergencyContactPhone || emergencyContactRelationship ? (
                        <>
                          {emergencyContactName || <span style={{ color: 'var(--border-strong)', fontStyle: 'italic' }}>Name not set</span>}
                          {emergencyContactRelationship && <span style={{ color: 'var(--text-muted)' }}> · {emergencyContactRelationship}</span>}
                          <br />
                          {emergencyContactPhone || <span style={{ color: 'var(--border-strong)', fontStyle: 'italic' }}>Phone not set</span>}
                        </>
                      ) : (
                        <span style={{ color: 'var(--border-strong)', fontStyle: 'italic' }}>No emergency contact on file</span>
                      )}
                      <div style={{ fontSize: '12px', color: 'var(--text-subtle)', marginTop: '6px' }}>
                        Allergies: {allergies || <span style={{ color: 'var(--border-strong)', fontStyle: 'italic' }}>none noted</span>}
                      </div>
                    </div>
                  )}
                </div>
                <button onClick={() => setEditingEmergencyContact(!editingEmergencyContact)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--accent)', fontSize: '12.5px', display: 'flex', alignItems: 'center', gap: '4px', padding: '4px 8px', borderRadius: '6px', transition: 'background 100ms', flexShrink: 0 }} onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(235,93,30,0.06)')} onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}>
                  <Icon name="edit" size={12} />
                  {editingEmergencyContact ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>
          </div>

          {/* Save Button */}
          {(editingName || editingNotes || editingEmergencyContact) && (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '8px', animation: 'slideUp 0.2s ease both' }}>
              {saveError && (
                <p style={{ fontSize: '12px', color: 'var(--error)', fontFamily: 'var(--font-body)', margin: 0 }}>{saveError}</p>
              )}
              {saveSuccess && (
                <p style={{ fontSize: '12px', color: 'var(--success)', fontFamily: 'var(--font-body)', margin: 0 }}>Changes saved.</p>
              )}
              <div style={{ display: 'flex', gap: '10px' }}>
                <button
                  onClick={() => { setEditingName(false); setEditingNotes(false); setEditingEmergencyContact(false); setSaveError(null) }}
                  style={{ padding: '10px 20px', background: 'transparent', color: 'var(--text-subtle)', border: '1px solid rgba(0,0,0,0.12)', borderRadius: '9px', fontSize: '14px', fontWeight: 500, cursor: 'pointer', fontFamily: 'var(--font-body)' }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleSave}
                  disabled={saving}
                  style={{
                    padding: '10px 24px',
                    background: saving ? 'rgba(235,93,30,0.6)' : 'var(--accent)',
                    color: 'var(--bg-elevated)',
                    border: 'none',
                    borderRadius: '9px',
                    fontSize: '14px',
                    fontWeight: 600,
                    cursor: saving ? 'wait' : 'pointer',
                    fontFamily: 'var(--font-body)',
                    transition: 'background 150ms',
                  }}
                >
                  {saving ? 'Saving…' : 'Save Changes'}
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* ── PERFORMANCE TAB ──────────────────────────────────────────────────── */}
      {tab === 'performance' && (
        <div style={{ animation: 'slideUp 0.3s ease both' }}>
          <div className="card" style={{ boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden', marginBottom: '20px' }}>
            <div style={{ padding: '16px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span style={{ fontFamily: 'var(--font-display)', fontSize: '17px', color: 'var(--text)' }}>Best Times</span>
              <span style={{ fontSize: '11.5px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>all-time</span>
            </div>

            {/* Table header */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 90px 90px 90px', padding: '10px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)', background: 'rgba(0,0,0,0.02)' }}>
              {['Event', 'SCY', 'SCM', 'LCM'].map((h) => (
                <span key={h} style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', textAlign: h === 'Event' ? 'left' : 'center' }}>
                  {h}
                </span>
              ))}
            </div>

            {loading ? (
              <div style={{ padding: '16px 20px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {[0, 1, 2, 3].map((i) => <Skeleton key={i} h={14} />)}
              </div>
            ) : bestTimes.length === 0 ? (
              <div style={{ padding: '32px 20px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13.5px' }}>
                No best times on record yet.
              </div>
            ) : (
              bestTimes.map((row, i) => (
                <div key={row.event} style={{ display: 'grid', gridTemplateColumns: '1fr 90px 90px 90px', padding: '13px 20px', borderBottom: i < bestTimes.length - 1 ? '1px solid rgba(0,0,0,0.04)' : 'none', alignItems: 'center' }}>
                  <span style={{ fontSize: '14px', color: 'var(--text)', fontWeight: 500 }}>{row.event}</span>
                  {[row.scy, row.scm, row.lcm].map((t, j) => (
                    <span key={j} style={{ fontSize: '13.5px', color: t === '—' ? 'var(--border-strong)' : '#4B7BF5', fontFamily: 'var(--font-body)', textAlign: 'center', fontWeight: t === '—' ? 400 : 600 }}>
                      {t}
                    </span>
                  ))}
                </div>
              ))
            )}
          </div>

          <div style={{ background: 'rgba(235,93,30,0.04)', border: '1px solid rgba(235,93,30,0.12)', borderRadius: '10px', padding: '12px 16px', fontSize: '12.5px', color: '#7A4A28', lineHeight: 1.5 }}>
            Best times are updated automatically when meet results are imported. Contact the club to dispute a time.
          </div>
        </div>
      )}

      {/* ── HISTORY TAB ──────────────────────────────────────────────────────── */}
      {tab === 'history' && (
        <div style={{ animation: 'slideUp 0.3s ease both' }}>
          <div className="card" style={{ boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden' }}>
            <div style={{ padding: '16px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>
              <span style={{ fontFamily: 'var(--font-display)', fontSize: '17px', color: 'var(--text)' }}>
                Activity History
              </span>
            </div>
            {loading ? (
              <div style={{ padding: '20px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {[0, 1, 2].map((i) => <Skeleton key={i} h={42} r={10} />)}
              </div>
            ) : activity.length === 0 ? (
              <div style={{ padding: '32px 20px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13.5px', lineHeight: 1.6 }}>
                <Icon name="clock" size={20} style={{ display: 'block', margin: '0 auto 10px', color: 'var(--border-strong)' }} />
                No activity has been recorded for this swimmer yet.
              </div>
            ) : (
              activity.map((entry, index) => {
                const accent = historyAccent(entry.type)
                return (
                  <div key={entry.id} style={{ display: 'grid', gridTemplateColumns: '104px 1fr', gap: '14px', padding: '16px 20px', borderBottom: index < activity.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none', alignItems: 'start' }}>
                    <div style={{ fontSize: '11.5px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', paddingTop: '3px' }}>
                      {fmtShortDate(entry.occurred_at)}
                    </div>
                    <div style={{ display: 'flex', gap: '12px' }}>
                      <div style={{ width: '10px', height: '10px', borderRadius: '999px', background: accent.bg, border: `2px solid ${accent.color}`, marginTop: '4px', flexShrink: 0 }} />
                      <div style={{ minWidth: 0 }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap', marginBottom: '4px' }}>
                          <span style={{ fontSize: '13.5px', color: 'var(--text)', fontWeight: 600 }}>{entry.title}</span>
                          {entry.status ? (
                            <span style={{ padding: '2px 8px', borderRadius: '999px', background: accent.bg, color: accent.color, fontSize: '10.5px', fontFamily: 'var(--font-body)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                              {entry.status.replaceAll('_', ' ')}
                            </span>
                          ) : null}
                        </div>
                        {entry.detail ? (
                          <div style={{ fontSize: '12.5px', color: 'var(--text-subtle)', lineHeight: 1.55 }}>
                            {entry.detail}
                          </div>
                        ) : null}
                      </div>
                    </div>
                  </div>
                )
              })
            )}
          </div>
        </div>
      )}
    </div>
  )
}
