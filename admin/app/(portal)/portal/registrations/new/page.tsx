'use client'

import { useEffect, useMemo, useState } from 'react'
import { useRouter } from 'next/navigation'

import { ApiError, useApiClient } from '@/lib/api'
import {
  acceptRegistrationWaiver,
  createRegistration,
  fetchDocumentTypes,
  fetchGroups,
  fetchRegistrations,
  fetchSwimmers,
  type DocumentTypeRecord,
  type GroupRecord,
  type RegistrationRecord,
  type SwimmerRecord,
} from '@/lib/adminData'
import {
  deriveEligiblePortalRegistrationSwimmers,
  derivePortalRegistrationPrograms,
  derivePortalRegistrationSeasons,
  derivePortalRegistrationWaivers,
} from '@/lib/portal-registration-wizard'
import { useSession } from '@/lib/session'

const STEP_LABELS = ['Select Program', 'Select Swimmers', 'Review Waivers', 'Confirm']

function calculateAge(dateOfBirth: string): number | null {
  const dob = new Date(dateOfBirth)
  if (Number.isNaN(dob.getTime())) return null
  const now = new Date()
  let age = now.getFullYear() - dob.getFullYear()
  const monthDelta = now.getMonth() - dob.getMonth()
  if (monthDelta < 0 || (monthDelta === 0 && now.getDate() < dob.getDate())) age -= 1
  return age
}

function formatApiError(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback
}

export default function NewRegistrationPage() {
  const router = useRouter()
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''

  const [step, setStep] = useState(0)
  const [selectedSeason, setSelectedSeason] = useState('')
  const [selectedProgramId, setSelectedProgramId] = useState('')
  const [selectedSwimmers, setSelectedSwimmers] = useState<Set<string>>(new Set())
  const [waiverAccepted, setWaiverAccepted] = useState<Record<string, boolean>>({})
  const [expandedWaiver, setExpandedWaiver] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [registrations, setRegistrations] = useState<RegistrationRecord[]>([])
  const [documentTypes, setDocumentTypes] = useState<DocumentTypeRecord[]>([])

  useEffect(() => {
    let cancelled = false
    async function load() {
      if (!orgId || !householdId) {
        setLoading(false)
        setError('Your session is missing household access. Sign out and sign back in to continue.')
        return
      }
      try {
        setLoading(true)
        setError('')
        const [allSwimmers, allGroups, allRegistrations, allDocumentTypes] = await Promise.all([
          fetchSwimmers(api, orgId),
          fetchGroups(api, orgId),
          fetchRegistrations(api, orgId),
          fetchDocumentTypes(api, orgId, true),
        ])
        if (cancelled) return
        setSwimmers(allSwimmers.filter((swimmer) => swimmer.household_id === householdId))
        setGroups(allGroups)
        setRegistrations(allRegistrations)
        setDocumentTypes(allDocumentTypes)
      } catch (loadError) {
        if (cancelled) return
        setError(formatApiError(loadError, 'Unable to load registration options right now.'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [api, householdId, orgId])

  const seasonOptions = useMemo(() => derivePortalRegistrationSeasons(registrations), [registrations])
  const programOptions = useMemo(() => derivePortalRegistrationPrograms(groups), [groups])
  const waiverOptions = useMemo(() => derivePortalRegistrationWaivers(documentTypes), [documentTypes])
  const swimmerOptions = useMemo(
    () => deriveEligiblePortalRegistrationSwimmers(swimmers, registrations, selectedSeason, selectedProgramId),
    [registrations, selectedProgramId, selectedSeason, swimmers],
  )

  useEffect(() => {
    if (!selectedSeason && seasonOptions[0]) {
      setSelectedSeason(seasonOptions[0].id)
    }
  }, [selectedSeason, seasonOptions])

  useEffect(() => {
    setSelectedSwimmers((current) => {
      const validIds = new Set(swimmerOptions.filter((swimmer) => !swimmer.alreadyRegistered).map((swimmer) => swimmer.id))
      const next = new Set([...current].filter((id) => validIds.has(id)))
      return next.size === current.size ? current : next
    })
  }, [swimmerOptions])

  function toggleSwimmer(id: string) {
    setSelectedSwimmers((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
    setError('')
  }

  function canProceed(): boolean {
    if (step === 0) return Boolean(selectedSeason && selectedProgramId)
    if (step === 1) return selectedSwimmers.size > 0
    if (step === 2) return waiverOptions.filter((waiver) => waiver.required).every((waiver) => waiverAccepted[waiver.id])
    return true
  }

  async function handleSubmit() {
    if (!orgId || !householdId || !selectedProgramId || !selectedSeason || selectedSwimmers.size === 0) return
    setSubmitting(true)
    setError('')
    try {
      const selectedWaivers = waiverOptions.filter((waiver) => waiverAccepted[waiver.id])
      const swimmerIds = [...selectedSwimmers]
      const swimmerName = (id: string) => {
        const s = swimmerOptions.find((swimmer) => swimmer.id === id)
        return s ? `${s.first_name} ${s.last_name}` : id
      }

      const registrationResults = await Promise.allSettled(
        swimmerIds.map((swimmerId) => createRegistration(api, orgId, {
          household_id: householdId,
          swimmer_id: swimmerId,
          group_id: selectedProgramId,
          season: selectedSeason,
        })),
      )

      const createdRegistrations = registrationResults
        .map((result, i) => (result.status === 'fulfilled' ? { swimmerId: swimmerIds[i], registration: result.value } : null))
        .filter((entry): entry is { swimmerId: string; registration: Awaited<ReturnType<typeof createRegistration>> } => entry !== null)
      const failedSwimmerNames = registrationResults
        .map((result, i) => (result.status === 'rejected' ? swimmerName(swimmerIds[i]) : null))
        .filter((name): name is string => name !== null)

      const waiverTasks = createdRegistrations.flatMap(({ swimmerId, registration }) =>
        selectedWaivers.map((waiver) => ({
          swimmerId,
          run: () => acceptRegistrationWaiver(api, orgId, registration.id, { waiver_type: waiver.waiverType }),
        })),
      )
      const waiverResults = await Promise.allSettled(waiverTasks.map((task) => task.run()))
      const failedWaiverSwimmerNames = [
        ...new Set(
          waiverResults
            .map((result, i) => (result.status === 'rejected' ? swimmerName(waiverTasks[i].swimmerId) : null))
            .filter((name): name is string => name !== null),
        ),
      ]

      if (failedSwimmerNames.length > 0 || failedWaiverSwimmerNames.length > 0) {
        const parts: string[] = []
        if (createdRegistrations.length > 0) {
          parts.push(`Registered ${createdRegistrations.length} of ${swimmerIds.length} swimmer(s) successfully.`)
        }
        if (failedSwimmerNames.length > 0) {
          parts.push(`Failed to register: ${failedSwimmerNames.join(', ')}.`)
        }
        if (failedWaiverSwimmerNames.length > 0) {
          parts.push(`Waiver acceptance failed for: ${failedWaiverSwimmerNames.join(', ')} (registration was created; please retry accepting waivers from the Registrations page).`)
        }
        setError(parts.join(' '))
        return
      }

      setSubmitted(true)
    } catch (submitError) {
      setError(formatApiError(submitError, 'Unable to submit this registration right now.'))
    } finally {
      setSubmitting(false)
    }
  }

  const selectedProgram = programOptions.find((program) => program.id === selectedProgramId) ?? null
  const selectedSeasonLabel = seasonOptions.find((season) => season.id === selectedSeason)?.label ?? selectedSeason
  const selectedSwimmerRecords = swimmerOptions.filter((swimmer) => selectedSwimmers.has(swimmer.id))

  if (submitted) {
    return (
      <div style={{ padding: '48px 24px', maxWidth: '560px', margin: '0 auto', textAlign: 'center' }}>
        <div style={{ width: '64px', height: '64px', borderRadius: '50%', background: 'var(--success-bg)', border: '1px solid var(--success-border)', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 24px' }}>
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="var(--success)" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
            <polyline points="20 6 9 17 4 12" />
          </svg>
        </div>
        <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '26px', color: 'var(--text)', marginBottom: '10px' }}>Registration submitted</h2>
        <p style={{ fontSize: '14px', color: 'var(--text-subtle)', lineHeight: 1.7, marginBottom: '28px' }}>
          Your {selectedSeasonLabel} registration for <strong style={{ color: 'var(--text)' }}>{selectedProgram?.name ?? 'the selected program'}</strong> is now pending review.
        </p>
        <button
          onClick={() => router.push('/portal/registrations')}
          className="btn btn-primary"
          style={{ padding: '11px 28px', fontSize: '14px' }}
        >
          Back to Registrations
        </button>
      </div>
    )
  }

  return (
    <div style={{ padding: '32px 24px 80px', maxWidth: '720px', margin: '0 auto' }}>
      <button
        onClick={() => step > 0 ? setStep(step - 1) : router.push('/portal/registrations')}
        style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', fontSize: '13px', fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '20px', padding: 0 }}
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
          <line x1="19" y1="12" x2="5" y2="12" /><polyline points="12 19 5 12 12 5" />
        </svg>
        {step === 0 ? 'Back to Registrations' : 'Previous step'}
      </button>

      <h1 className="page-title">
        New Registration
      </h1>

      <div style={{ display: 'flex', gap: '8px', marginBottom: '28px' }}>
        {STEP_LABELS.map((label, idx) => (
          <div key={label} style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '6px' }}>
            <div style={{ height: '3px', borderRadius: '2px', background: idx <= step ? 'var(--accent)' : 'var(--border)', transition: 'background 0.3s ease' }} />
            <span style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: idx === step ? 'var(--text)' : 'var(--text-muted)', fontWeight: idx === step ? 600 : 400, letterSpacing: '0.02em' }}>
              {label}
            </span>
          </div>
        ))}
      </div>

      {error && (
        <div style={{ marginBottom: '18px', padding: '12px 14px', borderRadius: '10px', background: 'var(--error-bg)', border: '1px solid rgba(220,38,38,0.15)', color: 'var(--error)', fontSize: '13px', lineHeight: 1.5 }}>
          {error}
        </div>
      )}

      {loading ? (
        <div style={{ padding: '28px 0', color: 'var(--text-subtle)', fontSize: '14px' }}>Loading registration options…</div>
      ) : (
        <>
          {step === 0 && (
            <div>
              <p className="page-subtitle">
                Choose the season and active program you&apos;d like to register for.
              </p>

              <div style={{ marginBottom: '20px' }}>
                <div style={{ fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '10px' }}>Season</div>
                <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
                  {seasonOptions.map((season) => (
                    <button
                      key={season.id}
                      onClick={() => setSelectedSeason(season.id)}
                      style={{
                        padding: '10px 14px',
                        borderRadius: '999px',
                        border: `1px solid ${selectedSeason === season.id ? 'var(--accent)' : 'var(--border)'}`,
                        background: selectedSeason === season.id ? 'rgba(235,93,30,0.08)' : 'var(--bg-elevated)',
                        color: selectedSeason === season.id ? '#c2410c' : 'var(--text-subtle)',
                        cursor: 'pointer',
                        fontSize: '13px',
                        fontWeight: 600,
                      }}
                    >
                      {season.label}
                    </button>
                  ))}
                </div>
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {programOptions.map((program) => (
                  <button
                    key={program.id}
                    onClick={() => setSelectedProgramId(program.id)}
                    style={{
                      padding: '18px 20px',
                      textAlign: 'left',
                      borderRadius: '12px',
                      cursor: 'pointer',
                      background: selectedProgramId === program.id ? 'rgba(235,93,30,0.06)' : 'var(--bg-elevated)',
                      border: `2px solid ${selectedProgramId === program.id ? 'var(--accent)' : 'var(--border)'}`,
                      transition: 'all 0.2s ease',
                      width: '100%',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '12px' }}>
                      <div>
                        <p style={{ fontSize: '15px', fontWeight: 600, color: 'var(--text)', marginBottom: '4px', fontFamily: '"Plus Jakarta Sans", system-ui, sans-serif' }}>{program.name}</p>
                        <p style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
                          {program.group.code.toUpperCase()} · {program.type}
                        </p>
                      </div>
                      <div style={{ textAlign: 'right', flexShrink: 0, color: 'var(--text-muted)', fontSize: '12px' }}>
                        {program.group.min_age != null || program.group.max_age != null
                          ? `Ages ${program.group.min_age ?? '—'}-${program.group.max_age ?? '—'}`
                          : 'All ages'}
                      </div>
                    </div>
                    {program.description && (
                      <p style={{ marginTop: '10px', marginBottom: 0, color: 'var(--text-subtle)', fontSize: '13px', lineHeight: 1.55 }}>
                        {program.description}
                      </p>
                    )}
                  </button>
                ))}
              </div>
            </div>
          )}

          {step === 1 && (
            <div>
              <p className="page-subtitle">
                Select which swimmers to enroll in <strong style={{ color: 'var(--text)' }}>{selectedProgram?.name}</strong> for <strong style={{ color: 'var(--text)' }}>{selectedSeasonLabel}</strong>.
              </p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                {swimmerOptions.map((swimmer) => {
                  const age = calculateAge(swimmer.date_of_birth)
                  return (
                    <button
                      key={swimmer.id}
                      onClick={() => !swimmer.alreadyRegistered && toggleSwimmer(swimmer.id)}
                      disabled={swimmer.alreadyRegistered}
                      style={{
                        padding: '16px 18px',
                        textAlign: 'left',
                        borderRadius: '10px',
                        cursor: swimmer.alreadyRegistered ? 'not-allowed' : 'pointer',
                        background: swimmer.alreadyRegistered ? '#F5F3F0' : selectedSwimmers.has(swimmer.id) ? 'rgba(235,93,30,0.06)' : 'var(--bg-elevated)',
                        border: `2px solid ${swimmer.alreadyRegistered ? 'var(--border)' : selectedSwimmers.has(swimmer.id) ? 'var(--accent)' : 'var(--border)'}`,
                        width: '100%',
                        transition: 'all 0.2s ease',
                        opacity: swimmer.alreadyRegistered ? 0.7 : 1,
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
                        <div style={{
                          width: '36px', height: '36px', borderRadius: '50%', flexShrink: 0,
                          background: selectedSwimmers.has(swimmer.id) && !swimmer.alreadyRegistered ? 'var(--accent)' : 'var(--border)',
                          display: 'flex', alignItems: 'center', justifyContent: 'center',
                          fontSize: '14px', fontWeight: 700, color: selectedSwimmers.has(swimmer.id) && !swimmer.alreadyRegistered ? 'var(--bg-elevated)' : 'var(--text-muted)',
                          fontFamily: '"Instrument Serif", Georgia, serif',
                        }}>
                          {`${swimmer.first_name[0] ?? ''}${swimmer.last_name[0] ?? ''}`.toUpperCase()}
                        </div>
                        <div style={{ flex: 1 }}>
                          <p style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)', marginBottom: '2px', fontFamily: '"Plus Jakarta Sans", system-ui, sans-serif' }}>
                            {swimmer.first_name} {swimmer.last_name}
                          </p>
                          <p style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
                            {age != null ? `Age ${age}` : 'Age unavailable'} · {swimmer.registration_status}
                          </p>
                        </div>
                        {swimmer.alreadyRegistered && (
                          <span style={{ fontSize: '10px', padding: '2px 8px', background: 'var(--border)', color: 'var(--text-muted)', borderRadius: '20px', fontFamily: 'var(--font-body)', fontWeight: 600, flexShrink: 0 }}>
                            ALREADY REGISTERED
                          </span>
                        )}
                      </div>
                    </button>
                  )
                })}
              </div>
            </div>
          )}

          {step === 2 && (
            <div>
              <p className="page-subtitle">
                Review and accept the live registration and waiver requirements for this enrollment.
              </p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {waiverOptions.map((waiver) => (
                  <div
                    key={waiver.id}
                    style={{
                      borderRadius: '10px',
                      overflow: 'hidden',
                      border: `1px solid ${waiverAccepted[waiver.id] ? 'rgba(22,163,74,0.3)' : 'var(--border)'}`,
                      background: waiverAccepted[waiver.id] ? 'rgba(22,163,74,0.04)' : 'var(--bg-elevated)',
                    }}
                  >
                    <button
                      onClick={() => setExpandedWaiver(expandedWaiver === waiver.id ? null : waiver.id)}
                      style={{ width: '100%', padding: '14px 16px', background: 'none', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '12px', textAlign: 'left' }}
                    >
                      <div style={{
                        width: '20px', height: '20px', borderRadius: '50%', flexShrink: 0,
                        background: waiverAccepted[waiver.id] ? 'var(--success)' : '#F0EDEA',
                        border: `2px solid ${waiverAccepted[waiver.id] ? 'var(--success)' : 'var(--border)'}`,
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                      }}>
                        {waiverAccepted[waiver.id] && (
                          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth={3} strokeLinecap="round" strokeLinejoin="round">
                            <polyline points="20 6 9 17 4 12" />
                          </svg>
                        )}
                      </div>
                      <div style={{ flex: 1 }}>
                        <p style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)', fontFamily: '"Plus Jakarta Sans", system-ui, sans-serif' }}>
                          {waiver.title} {waiver.required && <span style={{ color: 'var(--error)' }}>*</span>}
                        </p>
                        <p style={{ margin: '4px 0 0', fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
                          {waiver.appliesTo}
                        </p>
                      </div>
                    </button>

                    {expandedWaiver === waiver.id && (
                      <div style={{ padding: '0 16px 16px' }}>
                        <div style={{ padding: '12px', background: 'var(--bg)', borderRadius: '8px', marginBottom: '12px' }}>
                          <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7, margin: 0 }}>{waiver.body}</p>
                        </div>
                        <label style={{ display: 'flex', alignItems: 'flex-start', gap: '10px', cursor: 'pointer' }}>
                          <input
                            type="checkbox"
                            checked={!!waiverAccepted[waiver.id]}
                            onChange={(event) => setWaiverAccepted((prev) => ({ ...prev, [waiver.id]: event.target.checked }))}
                            style={{ marginTop: '2px', accentColor: 'var(--accent)', width: '16px', height: '16px', cursor: 'pointer' }}
                          />
                          <span style={{ fontSize: '13px', color: 'var(--text)', lineHeight: 1.5 }}>
                            I have read and agree to the <strong>{waiver.title}</strong>
                            {!waiver.required && <span style={{ color: 'var(--text-muted)' }}> (optional)</span>}
                          </span>
                        </label>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          {step === 3 && (
            <div>
              <p className="page-subtitle">
                Review your registration summary before submitting.
              </p>
              <div className="card" style={{ overflow: 'hidden', marginBottom: '20px' }}>
                <div style={{ padding: '16px 18px', borderBottom: '1px solid #E5E1DC', background: 'var(--bg)' }}>
                  <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>Program</p>
                  <p style={{ fontSize: '15px', fontWeight: 600, color: 'var(--text)', marginTop: '2px' }}>{selectedProgram?.name}</p>
                  <p style={{ fontSize: '12px', color: 'var(--text-muted)', margin: '4px 0 0' }}>{selectedSeasonLabel}</p>
                </div>
                <div style={{ padding: '16px 18px', borderBottom: '1px solid #E5E1DC' }}>
                  <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>Swimmers</p>
                  {selectedSwimmerRecords.map((swimmer) => (
                    <div key={swimmer.id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                      <span style={{ fontSize: '13px', color: 'var(--text)' }}>{swimmer.first_name} {swimmer.last_name}</span>
                      <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--text-subtle)' }}>{swimmer.registration_status}</span>
                    </div>
                  ))}
                </div>
                <div style={{ padding: '16px 18px' }}>
                  <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>Accepted Requirements</p>
                  <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                    {waiverOptions.filter((waiver) => waiverAccepted[waiver.id]).map((waiver) => (
                      <span key={waiver.id} style={{ padding: '4px 9px', borderRadius: '999px', background: 'var(--warning-bg)', color: '#c2410c', fontSize: '11px', fontWeight: 600 }}>
                        {waiver.title}
                      </span>
                    ))}
                  </div>
                </div>
              </div>
              <div style={{ padding: '12px 14px', background: 'rgba(37,99,235,0.06)', border: '1px solid rgba(37,99,235,0.15)', borderRadius: '8px', fontSize: '12px', color: 'var(--info)' }}>
                Registrations are created immediately in pending status, and accepted waivers are written to the same live registration records shown in the portal and admin views.
              </div>
            </div>
          )}
        </>
      )}

      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '28px' }}>
        <button
          onClick={() => step > 0 ? setStep(step - 1) : router.push('/portal/registrations')}
          className="btn btn-secondary"
          style={{ padding: '11px 22px' }}
        >
          {step === 0 ? 'Cancel' : '← Back'}
        </button>
        <button
          onClick={() => step < 3 ? setStep(step + 1) : void handleSubmit()}
          disabled={loading || !canProceed() || submitting}
          className="btn btn-primary"
          style={{ padding: '11px 28px' }}
        >
          {submitting ? 'Submitting…' : step < 3 ? 'Continue →' : 'Submit Registration'}
        </button>
      </div>
    </div>
  )
}
