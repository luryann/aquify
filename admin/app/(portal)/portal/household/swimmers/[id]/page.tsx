'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { SwimmerSignInToggle } from '@/components/members/SwimmerSignInToggle'
import { GoalsPanel } from '@/components/performance/GoalsPanel'
import { useApiClient } from '@/lib/api'
import { reportUIError, uiErrorMessage } from '@/lib/frontend-telemetry'
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
import { formatDateOnly, parseBackendDate } from '@/lib/dates'
import './page.inline.css'
import { SWIMMER_AVATAR_COLOR } from '@/lib/avatar-colors'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { ShimmerBlock } from '@/components/ui/ShimmerBlock'

// ── Helpers ─────────────────────────────────────────────────────────────────
function calcAge(iso: string | undefined | null): number | null {
  if (!iso) return null
  const d = parseBackendDate(iso)
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
  const value = parseBackendDate(iso)
  if (Number.isNaN(value.getTime())) return '—'
  return value.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function historyAccent(type: string): { bg: string; color: string } {
  switch (type) {
    case 'result':
      return { bg: 'var(--info-bg)', color: 'var(--info)' }
    case 'registration':
      return { bg: 'var(--success-bg)', color: 'var(--success)' }
    case 'waiver':
      return { bg: 'var(--warning-bg)', color: 'var(--warning)' }
    case 'document':
      return { bg: 'var(--accent-subtle)', color: 'var(--accent-text, var(--accent))' }
    case 'commitment':
      return { bg: 'var(--bg-wash-strong)', color: 'var(--text-subtle)' }
    default:
      return { bg: 'var(--bg-wash-strong)', color: 'var(--text-subtle)' }
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
      setLoadError('We could not find that swimmer. Go back to your household and pick a swimmer.')
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
            'We could not load this page. Check your connection and try again.',
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
      setSaveError(uiErrorMessage(err, 'Couldn’t save changes. Try again.'))
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
  const statusCfg = STATUS_CONFIG[statusKey] ?? STATUS_CONFIG.active
  const gradient = SWIMMER_AVATAR_COLOR


  return (
    <div className="page">
      
      {/* Breadcrumb */}
      <nav className="is-90f3963">
        <Link href="/portal/household" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>
          Household
        </Link>
        <span>/</span>
        <span className="is-aaa1da3">
          {loading ? 'Loading…' : fullName}
        </span>
      </nav>

      {loadError && (
        <LoadErrorBanner message={loadError} onRetry={() => setReloadKey((key) => key + 1)} />
      )}

      {/* Header Card */}
      <div className="card is-466f0ca" >
        {loading ? (
          <>
            <ShimmerBlock w={72} h={72} r={36} />
            <div className="is-e933947">
              <ShimmerBlock w="40%" h={26} />
              <ShimmerBlock w="65%" h={14} />
            </div>
          </>
        ) : (
          <>
            {/* Avatar */}
            <div style={{ width: '72px', height: '72px', borderRadius: '50%', background: gradient, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--bg-elevated)', fontSize: 'var(--fs-2xl)', fontWeight: 700, fontFamily: 'var(--font-body)', flexShrink: 0, boxShadow: '0 4px 16px rgba(59,130,246,0.3)' }}>
              {initials}
            </div>

            {/* Info */}
            <div style={{ flex: 1, minWidth: 0 }}>
              <div className="is-c960f60">
                <h1 className="page-title">
                  {fullName}
                </h1>
                <span style={{ padding: '4px 8px', borderRadius: '999px', fontSize: 'var(--fs-xs)', fontWeight: 600, background: statusCfg.bg, color: statusCfg.color, }}>
                  {statusCfg.label}
                </span>
              </div>
              <div className="is-a62a9a1">
                {age !== null && <span>Age {age}</span>}
                {swimmer?.gender && <><span>·</span><span>{swimmer.gender}</span></>}
                {groupName !== '—' && <><span>·</span><span>{groupName}</span></>}
                {swimmer?.swimmer_external_id && <><span>·</span><span className="is-ad077fe">{swimmer.swimmer_external_id}</span></>}
              </div>
            </div>

          </>
        )}
      </div>

      {/* Tabs */}
      <div className="is-fbd0fa8">
        {(['profile', 'performance', 'history'] as Tab[]).map((t) => (
          <button type="button"
            key={t}
            onClick={() => setTab(t)}
            style={{ flex: 1, padding: '8px 0', borderRadius: '8px', border: 'none', background: tab === t ? 'var(--bg-elevated)' : 'transparent', color: tab === t ? 'var(--text)' : 'var(--text-subtle)', fontSize: 'var(--fs-base)', fontWeight: tab === t ? 600 : 450, cursor: 'pointer', boxShadow: tab === t ? 'var(--shadow-xs)' : 'none', transition: 'all 120ms ease', fontFamily: 'var(--font-body)', textTransform: 'capitalize' }}
          >
            {t}
          </button>
        ))}
      </div>

      {/* ── PROFILE TAB ─────────────────────────────────────────────────────── */}
      {tab === 'profile' && (
        <div style={{ animation: 'slideUp 0.3s ease both' }}>
          {/* Edit policy notice */}
          <div className="is-b0803c6">
            <Icon name="info" size={15} style={{ color: 'var(--accent)', flexShrink: 0, marginTop: '2px' }} />
            <p className="is-822be2a">
              {EDITABLE_FIELD_NOTE}
            </p>
          </div>

          {/* Read-only Fields */}
          <div className="card is-b25ef49" >
            <div className="is-1d257ec">
              <span className="is-71cd844">
                Club-managed fields
              </span>
              <span className="is-ed524ad">
                read-only
              </span>
            </div>

            {loading ? (
              <div className="is-824b1f4">
                {[0, 1, 2, 3, 4].map((i) => <ShimmerBlock key={i} w={`${60 + i * 5}%`} h={14} />)}
              </div>
            ) : (
              [
                { label: 'Full name',           value: fullName },
                { label: 'Date of birth',        value: formatDateOnly(swimmer?.date_of_birth) },
                { label: 'Age',                  value: age !== null ? `${age} years old` : '—' },
                { label: 'Gender',               value: swimmer?.gender ?? '—' },
                { label: 'External ID',          value: swimmer?.swimmer_external_id ?? '—', mono: true },
                { label: 'Training group',       value: groupName },
                { label: 'Registration status',  value: swimmer?.registration_status ?? '—' },
              ].map((field, i, arr) => (
                <div key={field.label} style={{ display: 'flex', padding: '16px 20px', borderBottom: i < arr.length - 1 ? '1px solid var(--border)' : 'none', gap: '24px', alignItems: 'center' }}>
                  <span className="is-2d24824">
                    {field.label}
                  </span>
                  <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', fontFamily: field.mono ? 'var(--font-mono)' : undefined }}>
                    {field.value}
                  </span>
                </div>
              ))
            )}
          </div>

          {/* Editable Fields */}
          <div className="card is-b25ef49" >
            <div className="is-1d257ec">
              <span className="is-71cd844">
                Parent-editable fields
              </span>
              <Icon name="edit" size={12} style={{ color: 'var(--accent)' }} />
            </div>

            {/* Preferred Name */}
            <div className="is-541f5a9">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div className="is-8ab28f2">Preferred Name / Nickname</div>
                  {editingName ? (
                    <input aria-label="Preferred name"
                      autoFocus
                      value={preferredName}
                      onChange={(e) => setPreferredName(e.target.value)}
                      className="is-1855096"
                    />
                  ) : (
                    <div className="is-ddcee0b">
                      {preferredName || <span style={{ color: 'var(--border-strong)', fontStyle: 'italic' }}>Not set</span>}
                    </div>
                  )}
                </div>
                <button type="button" onClick={() => setEditingName(!editingName)} className="is-57031ea hv-76554a">
                  <Icon name="edit" size={12} />
                  {editingName ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>

            {/* Bio (swimmer notes field) */}
            <div className="is-401e6d8">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div className="is-8ab28f2">Bio</div>
                  {editingNotes ? (
                    <textarea aria-label="Bio"
                      autoFocus
                      value={emergencyNotes}
                      onChange={(e) => setEmergencyNotes(e.target.value)}
                      rows={4}
                      className="is-0bb82c5"
                    />
                  ) : (
                    <div className="is-6600ab7">
                      {emergencyNotes || <span style={{ color: 'var(--border-strong)', fontStyle: 'italic' }}>No bio yet</span>}
                    </div>
                  )}
                  <div className="is-ed9977d">
                    Visible to coaches and club staff during meets and practices.
                  </div>
                </div>
                <button type="button" onClick={() => setEditingNotes(!editingNotes)} className="is-b347197 hv-76554a">
                  <Icon name="edit" size={12} />
                  {editingNotes ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>

            {/* Emergency Contact + Allergies */}
            <div className="is-b47e2a5">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ flex: 1 }}>
                  <div className="is-76ed46e">Emergency contact</div>
                  {editingEmergencyContact ? (
                    <div className="is-7baf057">
                      <div className="is-6b4ef92">
                        <input aria-label="Contact name"
                          autoFocus
                          value={emergencyContactName}
                          onChange={(e) => setEmergencyContactName(e.target.value)}
                          placeholder="Contact name"
                          className="is-8b9e057"
                        />
                        <input aria-label="Relationship"
                          value={emergencyContactRelationship}
                          onChange={(e) => setEmergencyContactRelationship(e.target.value)}
                          placeholder="Relationship"
                          className="is-8b9e057"
                        />
                      </div>
                      <input aria-label="Phone number"
                        value={emergencyContactPhone}
                        onChange={(e) => setEmergencyContactPhone(e.target.value)}
                        placeholder="Phone number"
                        type="tel"
                        className="is-8b9e057"
                      />
                      <div>
                        <div className="is-0bc3371">Allergies</div>
                        <input aria-label="Allergies"
                          value={allergies}
                          onChange={(e) => setAllergies(e.target.value)}
                          placeholder="e.g. Peanuts, bee stings"
                          className="is-18c465d"
                        />
                      </div>
                    </div>
                  ) : (
                    <div className="is-de462b1">
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
                      <div className="is-c67b0c3">
                        Allergies: {allergies || <span style={{ color: 'var(--border-strong)', fontStyle: 'italic' }}>none noted</span>}
                      </div>
                    </div>
                  )}
                </div>
                <button type="button" onClick={() => setEditingEmergencyContact(!editingEmergencyContact)} className="is-b347197 hv-76554a">
                  <Icon name="edit" size={12} />
                  {editingEmergencyContact ? 'Cancel' : 'Edit'}
                </button>
              </div>
            </div>
          </div>

          {/* Save Button */}
          {(editingName || editingNotes || editingEmergencyContact) && (
            <div className="is-cebe0b0">
              {saveError && (
                <p className="is-194998e">{saveError}</p>
              )}
              {saveSuccess && (
                <p className="is-ace1913">Changes saved.</p>
              )}
              <div className="is-d6316a7">
                <button type="button"
                  onClick={() => { setEditingName(false); setEditingNotes(false); setEditingEmergencyContact(false); setSaveError(null) }}
                  className="is-4dd21a6"
                >
                  Cancel
                </button>
                <button type="button"
                  onClick={handleSave}
                  disabled={saving}
                  style={{
                    padding: '12px 24px',
                    background: saving ? 'rgba(235,93,30,0.6)' : 'var(--accent)',
                    color: 'var(--bg-elevated)',
                    border: 'none',
                    borderRadius: '8px',
                    fontSize: 'var(--fs-md)',
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
          <div className="card is-b25ef49" >
            <div className="is-8f43699">
              <span className="is-d2752fc">Best times</span>
              <span className="is-c9c6d72">all-time</span>
            </div>

            {/* Table header */}
            <div className="is-0f9b25b">
              {['Event', 'SCY', 'SCM', 'LCM'].map((h) => (
                <span key={h} className="col-head" style={{ textAlign: h === 'Event' ? 'left' : 'center' }}>
                  {h}
                </span>
              ))}
            </div>

            {loading ? (
              <div className="is-6455ae6">
                {[0, 1, 2, 3].map((i) => <ShimmerBlock key={i} h={14} />)}
              </div>
            ) : bestTimes.length === 0 ? (
              <div className="is-7f65358">
                No best times on record yet.
              </div>
            ) : (
              bestTimes.map((row, i) => (
                <div key={row.event} style={{ display: 'grid', gridTemplateColumns: '1fr 90px 90px 90px', padding: '12px 20px', borderBottom: i < bestTimes.length - 1 ? '1px solid var(--border)' : 'none', alignItems: 'center' }}>
                  <span className="is-ddcee0b">{row.event}</span>
                  {[row.scy, row.scm, row.lcm].map((t, j) => (
                    <span key={j} style={{ fontSize: 'var(--fs-base)', color: t === '—' ? 'var(--border-strong)' : 'var(--info)', fontFamily: 'var(--font-body)', textAlign: 'center', fontWeight: t === '—' ? 400 : 600 }}>
                      {t}
                    </span>
                  ))}
                </div>
              ))
            )}
          </div>

          <div className="is-0d884ef">
            Best times are updated automatically when meet results are imported. Contact the club to dispute a time.
          </div>
        </div>
      )}

      {/* ── HISTORY TAB ──────────────────────────────────────────────────────── */}
      {tab === 'history' && (
        <div style={{ animation: 'slideUp 0.3s ease both' }}>
          <div className="card" style={{ overflow: 'hidden' }}>
            <div className="is-541f5a9">
              <span className="is-d2752fc">
                Activity History
              </span>
            </div>
            {loading ? (
              <div className="is-824b1f4">
                {[0, 1, 2].map((i) => <ShimmerBlock key={i} h={42} r={10} />)}
              </div>
            ) : activity.length === 0 ? (
              <div className="is-7f23ed8">
                <Icon name="clock" size={20} style={{ display: 'block', margin: '0 auto 12px', color: 'var(--border-strong)' }} />
                No activity has been recorded for this swimmer yet.
              </div>
            ) : (
              activity.map((entry, index) => {
                const accent = historyAccent(entry.type)
                return (
                  <div key={entry.id} style={{ display: 'grid', gridTemplateColumns: '104px 1fr', gap: '16px', padding: '16px 20px', borderBottom: index < activity.length - 1 ? '1px solid var(--border)' : 'none', alignItems: 'start' }}>
                    <div className="is-7dabd43">
                      {fmtShortDate(entry.occurred_at)}
                    </div>
                    <div className="is-d6316a7">
                      <div style={{ width: '10px', height: '10px', borderRadius: '999px', background: accent.bg, border: `2px solid ${accent.color}`, marginTop: '4px', flexShrink: 0 }} />
                      <div style={{ minWidth: 0 }}>
                        <div className="is-9717a47">
                          <span className="is-c8e8844">{entry.title}</span>
                          {entry.status ? (
                            <span style={{ padding: '2px 8px', borderRadius: '999px', background: accent.bg, color: accent.color, fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', }}>
                              {entry.status.replaceAll('_', ' ')}
                            </span>
                          ) : null}
                        </div>
                        {entry.detail ? (
                          <div className="is-e1f2131">
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

      {swimmer ? (
        <div className="is-ef7a9df">
          <SwimmerSignInToggle swimmerId={swimmer.id} disabled={swimmer.portal_access_disabled ?? false} onChange={(next) => setSwimmer((prev) => (prev ? { ...prev, portal_access_disabled: next } : prev))} />
        </div>
      ) : null}

      {swimmerId ? (
        <div className="is-ef7a9df">
          <GoalsPanel swimmerId={swimmerId} />
        </div>
      ) : null}
    </div>
  )
}
