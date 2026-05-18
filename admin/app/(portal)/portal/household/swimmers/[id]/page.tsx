'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchSwimmer,
  fetchSwimmerGroups,
  fetchGroups,
  fetchSwimmerBestTimes,
  updateSwimmer,
  type SwimmerRecord,
  type BestTimeRecord,
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
  active:   { label: 'Active',   bg: '#ecfdf5', color: '#059669' },
  inactive: { label: 'Inactive', bg: '#fef2f2', color: '#dc2626' },
  pending:  { label: 'Pending',  bg: '#fffbeb', color: '#d97706' },
}

const EDITABLE_FIELD_NOTE =
  'Fields marked with a pencil icon can be updated by you. Other fields are managed by club staff.'

type Tab = 'profile' | 'performance' | 'history'

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
  const [loading, setLoading] = useState(true)

  // Editable local state (initialized from swimmer data on load)
  const [emergencyNotes, setEmergencyNotes] = useState('')
  const [preferredName, setPreferredName] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saveSuccess, setSaveSuccess] = useState(false)

  useEffect(() => {
    if (!orgId || !swimmerId) { setLoading(false); return }
    setLoading(true)
    Promise.all([
      fetchSwimmer(api, orgId, swimmerId),
      fetchSwimmerGroups(api, orgId),
      fetchGroups(api, orgId),
      fetchSwimmerBestTimes(api, orgId, swimmerId),
    ])
      .then(([sw, sGroups, grps, times]) => {
        setSwimmer(sw)
        setEmergencyNotes(sw.notes ?? '')
        setPreferredName(sw.preferred_name ?? '')

        const link = sGroups.find((sg) => sg.swimmer_id === sw.id && sg.is_primary)
        if (link) {
          const grp = grps.find((g) => g.code === link.group_code)
          setGroupName(grp?.name ?? link.group_code ?? '—')
        }

        setBestTimes(pivotBestTimes(times))
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [api, orgId, swimmerId])

  const [tab, setTab] = useState<Tab>('profile')
  const [editingName, setEditingName] = useState(false)
  const [editingNotes, setEditingNotes] = useState(false)

  async function handleSave() {
    if (!orgId || !swimmerId) return
    setSaving(true)
    setSaveError(null)
    setSaveSuccess(false)
    try {
      const updated = await updateSwimmer(api, orgId, swimmerId, {
        preferred_name: preferredName,
        notes: emergencyNotes,
      })
      setSwimmer(updated)
      setEmergencyNotes(updated.notes ?? '')
      setPreferredName(updated.preferred_name ?? '')
      setEditingName(false)
      setEditingNotes(false)
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
      <nav style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '13px', color: '#9A9693', marginBottom: '28px', animation: 'slideUp 0.3s ease both' }}>
        <Link href="/portal/household" style={{ color: '#9A9693', textDecoration: 'none' }}>
          Household
        </Link>
        <span>/</span>
        <span style={{ color: '#1A1714', fontWeight: 500 }}>
          {loading ? 'Loading…' : fullName}
        </span>
      </nav>

      {/* Header Card */}
      <div style={{ background: '#ffffff', borderRadius: '16px', border: '1px solid rgba(0,0,0,0.07)', boxShadow: '0 2px 12px rgba(0,0,0,0.05)', padding: '28px', marginBottom: '24px', display: 'flex', alignItems: 'center', gap: '20px', animation: 'slideUp 0.35s ease both' }}>
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
            <div style={{ width: '72px', height: '72px', borderRadius: '50%', background: gradient, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: '24px', fontWeight: 700, fontFamily: 'var(--font-mono)', flexShrink: 0, boxShadow: '0 4px 16px rgba(59,130,246,0.3)' }}>
              {initials}
            </div>

            {/* Info */}
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
                <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '26px', color: '#1A1714', margin: 0, lineHeight: 1.1 }}>
                  {fullName}
                </h1>
                <span style={{ padding: '3px 9px', borderRadius: '999px', fontSize: '11.5px', fontWeight: 600, background: statusCfg.bg, color: statusCfg.color, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  {statusCfg.label}
                </span>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '16px', marginTop: '8px', fontSize: '13px', color: '#6B6866' }}>
                {age !== null && <span>Age {age}</span>}
                {swimmer?.gender && <><span>·</span><span>{swimmer.gender}</span></>}
                {groupName !== '—' && <><span>·</span><span>{groupName}</span></>}
                {swimmer?.swimmer_external_id && <><span>·</span><span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px' }}>{swimmer.swimmer_external_id}</span></>}
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
            style={{ flex: 1, padding: '8px 0', borderRadius: '7px', border: 'none', background: tab === t ? '#ffffff' : 'transparent', color: tab === t ? '#1A1714' : '#6B6866', fontSize: '13.5px', fontWeight: tab === t ? 600 : 450, cursor: 'pointer', boxShadow: tab === t ? '0 1px 4px rgba(0,0,0,0.08)' : 'none', transition: 'all 120ms ease', fontFamily: 'var(--font-body)', textTransform: 'capitalize' }}
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
            <Icon name="info" size={15} style={{ color: '#eb5d1e', flexShrink: 0, marginTop: '2px' }} />
            <p style={{ margin: 0, fontSize: '13px', color: '#7A4A28', lineHeight: 1.5 }}>
              {EDITABLE_FIELD_NOTE}
            </p>
          </div>

          {/* Read-only Fields */}
          <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden', marginBottom: '20px' }}>
            <div style={{ padding: '16px 20px 12px', borderBottom: '1px solid rgba(0,0,0,0.06)', display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
                Club-managed fields
              </span>
              <span style={{ padding: '2px 7px', background: 'rgba(0,0,0,0.05)', borderRadius: '4px', fontSize: '10.5px', color: '#9A9693' }}>
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
                { label: 'USS Number',           value: swimmer?.swimmer_external_id ?? '—', mono: true },
                { label: 'Training Group',       value: groupName },
                { label: 'Registration Status',  value: swimmer?.registration_status ?? '—' },
              ].map((field, i, arr) => (
                <div key={field.label} style={{ display: 'flex', padding: '14px 20px', borderBottom: i < arr.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none', gap: '24px', alignItems: 'center' }}>
                  <span style={{ width: '160px', fontSize: '13px', color: '#9A9693', flexShrink: 0 }}>
                    {field.label}
                  </span>
                  <span style={{ fontSize: '13.5px', color: '#6B6866', fontFamily: field.mono ? 'var(--font-mono)' : undefined }}>
                    {field.value}
                  </span>
                </div>
              ))
            )}
          </div>

          {/* Editable Fields */}
          <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden', marginBottom: '20px' }}>
            <div style={{ padding: '16px 20px 12px', borderBottom: '1px solid rgba(0,0,0,0.06)', display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
                Parent-editable fields
              </span>
              <Icon name="edit" size={12} style={{ color: '#eb5d1e' }} />
            </div>

            {/* Preferred Name */}
            <div style={{ padding: '16px 20px', borderBottom: '1px solid rgba(0,0,0,0.05)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: '12px', color: '#9A9693', marginBottom: '4px' }}>Preferred Name / Nickname</div>
                  {editingName ? (
                    <input
                      autoFocus
                      value={preferredName}
                      onChange={(e) => setPreferredName(e.target.value)}
                      style={{ width: '100%', maxWidth: '280px', padding: '7px 11px', border: '1.5px solid #eb5d1e', borderRadius: '8px', fontSize: '14px', color: '#1A1714', fontFamily: 'var(--font-body)', outline: 'none', background: '#fff' }}
                    />
                  ) : (
                    <div style={{ fontSize: '14px', color: '#1A1714', fontWeight: 500 }}>
                      {preferredName || <span style={{ color: '#C0BDBB', fontStyle: 'italic' }}>Not set</span>}
                    </div>
                  )}
                </div>
                <button onClick={() => setEditingName(!editingName)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#eb5d1e', fontSize: '12.5px', display: 'flex', alignItems: 'center', gap: '4px', padding: '4px 8px', borderRadius: '6px', transition: 'background 100ms' }} onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(235,93,30,0.06)')} onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}>
                  <Icon name="edit" size={12} />
                  {editingName ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>

            {/* Emergency Notes */}
            <div style={{ padding: '16px 20px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: '12px', color: '#9A9693', marginBottom: '4px' }}>Medical / Emergency Notes</div>
                  {editingNotes ? (
                    <textarea
                      autoFocus
                      value={emergencyNotes}
                      onChange={(e) => setEmergencyNotes(e.target.value)}
                      rows={4}
                      style={{ width: '100%', padding: '9px 12px', border: '1.5px solid #eb5d1e', borderRadius: '8px', fontSize: '13.5px', color: '#1A1714', fontFamily: 'var(--font-body)', outline: 'none', background: '#fff', resize: 'vertical', lineHeight: 1.5 }}
                    />
                  ) : (
                    <div style={{ fontSize: '13.5px', color: '#1A1714', lineHeight: 1.6, maxWidth: '500px' }}>
                      {emergencyNotes || <span style={{ color: '#C0BDBB', fontStyle: 'italic' }}>No notes added</span>}
                    </div>
                  )}
                  <div style={{ fontSize: '11.5px', color: '#C0BDBB', marginTop: '6px' }}>
                    Visible to coaches and club staff during meets and practices.
                  </div>
                </div>
                <button onClick={() => setEditingNotes(!editingNotes)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#eb5d1e', fontSize: '12.5px', display: 'flex', alignItems: 'center', gap: '4px', padding: '4px 8px', borderRadius: '6px', transition: 'background 100ms', flexShrink: 0 }} onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(235,93,30,0.06)')} onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}>
                  <Icon name="edit" size={12} />
                  {editingNotes ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>
          </div>

          {/* Save Button */}
          {(editingName || editingNotes) && (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '8px', animation: 'slideUp 0.2s ease both' }}>
              {saveError && (
                <p style={{ fontSize: '12px', color: '#dc2626', fontFamily: 'var(--font-mono)', margin: 0 }}>{saveError}</p>
              )}
              {saveSuccess && (
                <p style={{ fontSize: '12px', color: '#059669', fontFamily: 'var(--font-mono)', margin: 0 }}>Changes saved.</p>
              )}
              <div style={{ display: 'flex', gap: '10px' }}>
                <button
                  onClick={() => { setEditingName(false); setEditingNotes(false); setSaveError(null) }}
                  style={{ padding: '10px 20px', background: 'transparent', color: '#6B6866', border: '1px solid rgba(0,0,0,0.12)', borderRadius: '9px', fontSize: '14px', fontWeight: 500, cursor: 'pointer', fontFamily: 'var(--font-body)' }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleSave}
                  disabled={saving}
                  style={{
                    padding: '10px 24px',
                    background: saving ? 'rgba(235,93,30,0.6)' : '#eb5d1e',
                    color: '#fff',
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
          <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden', marginBottom: '20px' }}>
            <div style={{ padding: '16px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span style={{ fontFamily: 'var(--font-display)', fontSize: '17px', color: '#1A1714' }}>Best Times</span>
              <span style={{ fontSize: '11.5px', color: '#9A9693', fontFamily: 'var(--font-mono)' }}>all-time</span>
            </div>

            {/* Table header */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 90px 90px 90px', padding: '10px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)', background: 'rgba(0,0,0,0.02)' }}>
              {['Event', 'SCY', 'SCM', 'LCM'].map((h) => (
                <span key={h} style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.06em', textAlign: h === 'Event' ? 'left' : 'center' }}>
                  {h}
                </span>
              ))}
            </div>

            {loading ? (
              <div style={{ padding: '16px 20px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {[0, 1, 2, 3].map((i) => <Skeleton key={i} h={14} />)}
              </div>
            ) : bestTimes.length === 0 ? (
              <div style={{ padding: '32px 20px', textAlign: 'center', color: '#9A9693', fontSize: '13.5px' }}>
                No best times on record yet.
              </div>
            ) : (
              bestTimes.map((row, i) => (
                <div key={row.event} style={{ display: 'grid', gridTemplateColumns: '1fr 90px 90px 90px', padding: '13px 20px', borderBottom: i < bestTimes.length - 1 ? '1px solid rgba(0,0,0,0.04)' : 'none', alignItems: 'center' }}>
                  <span style={{ fontSize: '14px', color: '#1A1714', fontWeight: 500 }}>{row.event}</span>
                  {[row.scy, row.scm, row.lcm].map((t, j) => (
                    <span key={j} style={{ fontSize: '13.5px', color: t === '—' ? '#C0BDBB' : '#4B7BF5', fontFamily: 'var(--font-mono)', textAlign: 'center', fontWeight: t === '—' ? 400 : 600 }}>
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
          <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden' }}>
            <div style={{ padding: '16px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>
              <span style={{ fontFamily: 'var(--font-display)', fontSize: '17px', color: '#1A1714' }}>
                Activity History
              </span>
            </div>
            <div style={{ padding: '32px 20px', textAlign: 'center', color: '#9A9693', fontSize: '13.5px', lineHeight: 1.6 }}>
              <Icon name="clock" size={20} style={{ display: 'block', margin: '0 auto 10px', color: '#C0BDBB' }} />
              Activity history is managed by club staff and will be available here in a future update.
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
