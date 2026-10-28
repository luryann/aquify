'use client'

import { Suspense, useCallback, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { fetchMeets, type MeetRecord } from '@/lib/adminData'
import {
  AGE_GROUPS, AGE_GROUP_LABELS, COURSES, STROKE_LABELS,
  deleteRelayLineup, fetchRelayLineups, formatSwimTime, optimizeRelays, saveRelayLineup,
  type RelayLineup, type RelayLock, type RelayProposal,
} from '@/lib/performanceData'
import { FIELD_STYLE, FilterRow, LoadErrorBanner, Notice, Segmented } from '@/components/performance/PerfControls'
import { relayLegLabel, toggleLock } from '@/lib/relay-builder'

export default function RelaysPage() {
  return (
    <Suspense fallback={null}>
      <RelayBuilder />
    </Suspense>
  )
}

function RelayBuilder() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const params = useSearchParams()
  const [meets, setMeets] = useState<MeetRecord[]>([])
  const [meetId, setMeetId] = useState(params.get('meet_id') ?? '')
  const [course, setCourse] = useState('SCY')
  const [gender, setGender] = useState<'F' | 'M' | 'X'>('F')
  const [ageGroup, setAgeGroup] = useState('OPEN')
  const [relayType, setRelayType] = useState<'free' | 'medley'>('medley')
  const [distance, setDistance] = useState(200)
  const [teams, setTeams] = useState(3)
  const [locks, setLocks] = useState<RelayLock[]>([])
  const [excluded, setExcluded] = useState<string[]>([])
  const [proposal, setProposal] = useState<RelayProposal | null>(null)
  const [lineups, setLineups] = useState<RelayLineup[]>([])
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [name, setName] = useState('')

  useEffect(() => {
    if (!orgId) return
    fetchMeets(api, orgId).then(setMeets).catch((err) => setError(reportUIError('performance.relays.meets', err, 'Meets could not be loaded.')))
  }, [api, orgId])

  const meet = meets.find((m) => m.id === meetId)
  useEffect(() => {
    if (meet && ['SCY', 'SCM', 'LCM'].includes(meet.course)) setCourse(meet.course)
  }, [meet])

  const loadLineups = useCallback(async () => {
    if (!orgId || !meetId) { setLineups([]); return }
    try {
      setLineups(await fetchRelayLineups(api, orgId, meetId))
    } catch (err) {
      setError(reportUIError('performance.relays.lineups', err, 'Saved lineups could not be loaded.'))
    }
  }, [api, orgId, meetId])
  useEffect(() => { void loadLineups() }, [loadLineups])

  const run = useCallback(async (nextLocks = locks, nextExcluded = excluded) => {
    if (!orgId) return
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      setProposal(await optimizeRelays(api, orgId, {
        meet_id: meetId || undefined, course, gender, age_group: ageGroup, relay_type: relayType, distance, teams,
        locks: nextLocks, excluded_swimmer_ids: nextExcluded,
      }))
    } catch (err) {
      setError(reportUIError('performance.relays.optimize', err, 'Relays could not be built.'))
    } finally {
      setBusy(false)
    }
  }, [api, orgId, meetId, course, gender, ageGroup, relayType, distance, teams, locks, excluded])

  const resetConstraints = () => { setLocks([]); setExcluded([]); setProposal(null) }

  const lockLeg = (team: number, leg: number, swimmerId: string) => {
    const next = toggleLock(locks, { team, leg, swimmer_id: swimmerId })
    setLocks(next)
    void run(next, excluded)
  }
  const exclude = (swimmerId: string) => {
    const next = excluded.includes(swimmerId) ? excluded.filter((id) => id !== swimmerId) : [...excluded, swimmerId]
    const nextLocks = locks.filter((l) => l.swimmer_id !== swimmerId)
    setExcluded(next)
    setLocks(nextLocks)
    void run(nextLocks, next)
  }

  const save = async () => {
    if (!proposal || !meetId) return
    setBusy(true)
    try {
      await saveRelayLineup(api, orgId, proposal, meetId, name.trim())
      setNotice(`Saved lineup for ${meet?.name ?? 'the meet'}.`)
      setName('')
      await loadLineups()
    } catch (err) {
      setError(reportUIError('performance.relays.save', err, 'The lineup could not be saved.'))
    } finally {
      setBusy(false)
    }
  }

  const excludedNames = useMemo(() => {
    const byId = new Map((proposal?.pool ?? []).map((p) => [p.swimmer_id, p.name]))
    return excluded.map((id) => ({ id, name: byId.get(id) ?? 'Swimmer' }))
  }, [excluded, proposal])

  const distances = relayType === 'medley' ? [200, 400] : [200, 400, 800]

  return (
    <div className="page">
      <h1 className="page-title">Relay builder</h1>
      <p className="page-subtitle u-mb-24px" >Fastest A, B and C relays from flat-start best times. Lock or exclude swimmers and the rest re-solves.</p>

      <FilterRow>
        <select aria-label="Meet" value={meetId} onChange={(e) => { setMeetId(e.target.value); resetConstraints() }} style={{ ...FIELD_STYLE, maxWidth: '100%' }}>
          <option value="">No meet (ages as of today)</option>
          {meets.map((m) => <option key={m.id} value={m.id}>{m.name}{m.start_date ? ` (${m.start_date})` : ''}</option>)}
        </select>
        <Segmented label="Relay" value={relayType} onChange={(v) => { setRelayType(v); if (v === 'medley' && distance === 800) setDistance(200); resetConstraints() }} options={[{ value: 'medley', label: 'Medley' }, { value: 'free', label: 'Free' }]} />
        <Segmented label="Distance" value={String(distance)} onChange={(v) => { setDistance(Number(v)); resetConstraints() }} options={distances.map((d) => ({ value: String(d), label: String(d) }))} />
        <Segmented label="Gender" value={gender} onChange={(v) => { setGender(v); resetConstraints() }} options={[{ value: 'F', label: 'Girls' }, { value: 'M', label: 'Boys' }, { value: 'X', label: 'Mixed' }]} />
        <select aria-label="Age group" value={ageGroup} onChange={(e) => { setAgeGroup(e.target.value); resetConstraints() }} style={FIELD_STYLE}>
          {AGE_GROUPS.map((g) => <option key={g} value={g}>{AGE_GROUP_LABELS[g]}</option>)}
        </select>
        <Segmented label="Course" value={course} onChange={(v) => { setCourse(v); resetConstraints() }} options={COURSES.map((c) => ({ value: c, label: c }))} />
        <select aria-label="Relays to build" value={teams} onChange={(e) => setTeams(Number(e.target.value))} style={FIELD_STYLE}>
          {[1, 2, 3, 4, 5, 6].map((n) => <option key={n} value={n}>{n === 1 ? 'A relay only' : `A–${String.fromCharCode(64 + n)} relays`}</option>)}
        </select>
        <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void run()}>{busy ? 'Building…' : 'Build relays'}</button>
      </FilterRow>

      {error ? <LoadErrorBanner message={error} onRetry={() => void run()} /> : null}
      {notice ? <Notice tone="success">{notice}</Notice> : null}

      {proposal ? (
        <>
          <p className="u-fs-fs-base u-m-0-0-16px" style={{ color: 'var(--text-muted)' }}>
            {proposal.pool.length} eligible swimmer{proposal.pool.length === 1 ? '' : 's'} with {proposal.leg_distance} {proposal.course} times, ages as of {proposal.as_of}.
            {mixedNote(proposal.gender)}
          </p>
          {proposal.warnings.map((w) => <Notice key={w} tone="warning">{w}</Notice>)}
          <div className="u-gap-16px u-mb-20px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 320px), 1fr))' }}>
            {proposal.teams.map((team, ti) => (
              <section key={team.label} className="card u-p-16px-16px" aria-label={`Relay ${team.label}`}>
                <div className="u-mb-8px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
                  <h2 className="section-title">{team.label}</h2>
                  <span className="u-ff-font-display u-fw-display-weight u-fs-fs-3xl" style={{ color: 'var(--accent-text, var(--accent))', fontVariantNumeric: 'tabular-nums' }}>{formatSwimTime(team.total_ms)}</span>
                </div>
                <ol className="u-m-0 u-p-0" style={{ listStyle: 'none' }}>
                  {team.legs.map((leg) => (
                    <li key={leg.leg} className="u-gap-8px u-p-8px-0 u-fs-fs-base" style={{ display: 'grid', gridTemplateColumns: '64px minmax(0, 1fr) auto', alignItems: 'center', borderTop: '1px solid var(--border)' }}>
                      <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{relayLegLabel(proposal.relay_type, leg.leg)}</span>
                      <span style={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {leg.swimmer_name}{proposal.gender === 'X' ? <span style={{ color: 'var(--text-muted)' }}> ({leg.gender})</span> : null}
                        <span style={{ color: 'var(--text-muted)', fontVariantNumeric: 'tabular-nums' }}> {formatSwimTime(leg.time_ms)}</span>
                      </span>
                      <span className="u-gap-4px" style={{ display: 'flex' }}>
                        <button type="button" className={`btn btn-sm ${leg.locked ? 'btn-secondary' : 'btn-ghost'}`} aria-pressed={leg.locked} disabled={busy} onClick={() => lockLeg(ti, leg.leg, leg.swimmer_id)} title={leg.locked ? 'Unlock this leg' : 'Keep this swimmer on this leg'}>{leg.locked ? 'Locked' : 'Lock'}</button>
                        <button type="button" className="btn btn-sm btn-ghost" disabled={busy} onClick={() => exclude(leg.swimmer_id)} title="Leave this swimmer out and rebuild">Leave out</button>
                      </span>
                    </li>
                  ))}
                </ol>
              </section>
            ))}
          </div>

          {excludedNames.length > 0 ? (
            <p className="u-fs-fs-base u-m-0-0-16px u-gap-6px" style={{ color: 'var(--text-subtle)', display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}>
              Left out:
              {excludedNames.map((x) => (
                <button key={x.id} type="button" className="badge badge-default" style={{ cursor: 'pointer' }} onClick={() => exclude(x.id)} title="Put back in the pool">{x.name} ✕</button>
              ))}
            </p>
          ) : null}

          {proposal.teams.length > 0 ? (
            meetId ? (
              <div className="u-gap-8px u-mb-24px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}>
                <input aria-label="Lineup name" placeholder={`${gender === 'X' ? 'Mixed' : gender === 'F' ? 'Girls' : 'Boys'} ${AGE_GROUP_LABELS[ageGroup]} ${distance} ${relayType === 'free' ? 'Free' : 'Medley'} Relay`} value={name} onChange={(e) => setName(e.target.value)} style={{ ...FIELD_STYLE, flex: '1 1 240px', maxWidth: '420px' }} />
                <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void save()}>Save lineup to meet</button>
              </div>
            ) : <Notice>Choose a meet to save this lineup.</Notice>
          ) : null}
        </>
      ) : null}

      {meetId ? (
        <section aria-labelledby="saved-lineups">
          <h2 id="saved-lineups" className="section-title" style={{ margin: '8px 0 12px' }}>
            Saved lineups for {meet?.name ?? 'this meet'}
          </h2>
          {meet ? <p className="u-m-0-0-12px u-fs-fs-base" ><Link href={`/meets/${meet.id}`} style={{ color: 'var(--accent-text, var(--accent))' }}>Back to {meet.name}</Link></p> : null}
          {lineups.length === 0 ? (
            <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>No lineups saved for this meet yet.</p>
          ) : (
            <div className="u-gap-12px" style={{ display: 'grid' }}>
              {lineups.map((l) => (
                <details key={l.id} className="card u-p-12px-16px" >
                  <summary className="u-gap-8px u-fs-fs-md" style={{ cursor: 'pointer', display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap' }}>
                    <span className="u-fw-600" >{l.name}</span>
                    <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{l.teams.map((t) => `${t.label} ${formatSwimTime(t.total_ms)}`).join(' / ')}</span>
                  </summary>
                  <div className="u-gap-12px u-mt-12px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 240px), 1fr))' }}>
                    {l.teams.map((t) => (
                      <div key={t.label} className="u-fs-fs-base" >
                        <strong>{t.label}</strong>
                        <ol className="u-m-4px-0-0 u-pl-20px" >
                          {t.legs.map((leg) => <li key={leg.leg}>{leg.swimmer_name} <span style={{ color: 'var(--text-muted)' }}>{STROKE_LABELS[leg.stroke] ?? leg.stroke} {formatSwimTime(leg.time_ms)}</span></li>)}
                        </ol>
                      </div>
                    ))}
                  </div>
                  <button type="button" className="btn btn-sm btn-danger u-mt-12px" onClick={() => void deleteRelayLineup(api, orgId, l.id).then(loadLineups).catch((err) => setError(reportUIError('performance.relays.delete', err, 'The lineup could not be deleted.')))}>Delete lineup</button>
                </details>
              ))}
            </div>
          )}
        </section>
      ) : null}
    </div>
  )
}

function mixedNote(gender: string) {
  return gender === 'X' ? ' Mixed relays use two men and two women.' : ''
}
