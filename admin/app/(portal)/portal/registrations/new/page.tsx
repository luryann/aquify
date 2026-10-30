'use client'

import { useEffect, useMemo, useState } from 'react'
import { useRouter } from 'next/navigation'

import { useApiClient } from '@/lib/api'
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
import { parseBackendDate } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import './page.inline.css'
import { PageSkeleton } from '@/components/ui/PageSkeleton'

const STEP_LABELS = ['Select Program', 'Select Swimmers', 'Review Waivers', 'Confirm']

function calculateAge(dateOfBirth: string): number | null {
  const dob = parseBackendDate(dateOfBirth)
  if (Number.isNaN(dob.getTime())) return null
  const now = new Date()
  let age = now.getFullYear() - dob.getFullYear()
  const monthDelta = now.getMonth() - dob.getMonth()
  if (monthDelta < 0 || (monthDelta === 0 && now.getDate() < dob.getDate())) age -= 1
  return age
}

function formatApiError(error: unknown, fallback: string): string {
  return uiErrorMessage(error, fallback)
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
        setError(formatApiError(loadError, 'Couldn’t load registration options. Try again.'))
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
        return s ? `${s.first_name} ${s.last_name}` : 'a swimmer'
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
        // Swimmers that did register must drop out of the selection, or a retry
        // would try to create their registrations a second time.
        if (createdRegistrations.length > 0) {
          void fetchRegistrations(api, orgId).then(setRegistrations).catch(() => {})
        }
        return
      }

      setSubmitted(true)
    } catch (submitError) {
      setError(formatApiError(submitError, 'Couldn’t submit this registration. Try again.'))
    } finally {
      setSubmitting(false)
    }
  }

  const selectedProgram = programOptions.find((program) => program.id === selectedProgramId) ?? null
  const selectedSeasonLabel = seasonOptions.find((season) => season.id === selectedSeason)?.label ?? selectedSeason
  const selectedSwimmerRecords = swimmerOptions.filter((swimmer) => selectedSwimmers.has(swimmer.id))

  if (submitted) {
    return (
      <div className="is-2ca52bf">
        <div className="is-997c999">
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="var(--success)" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
            <polyline points="20 6 9 17 4 12" />
          </svg>
        </div>
        <h2 className="is-8edcabb">Registration submitted</h2>
        <p className="is-07d54ce">
          Your {selectedSeasonLabel} registration for <strong style={{ color: 'var(--text)' }}>{selectedProgram?.name ?? 'the selected program'}</strong> is now pending review.
        </p>
        <button type="button"
          onClick={() => router.push('/portal/registrations')}
          className="btn btn-primary is-a2d7fde"
          
        >
          Back to Registrations
        </button>
      </div>
    )
  }

  return (
    <div className="page">
      <button type="button"
        onClick={() => step > 0 ? setStep(step - 1) : router.push('/portal/registrations')}
        className="is-05b71a8"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
          <line x1="19" y1="12" x2="5" y2="12" /><polyline points="12 19 5 12 12 5" />
        </svg>
        {step === 0 ? 'Back to Registrations' : 'Previous step'}
      </button>

      <h1 className="page-title">
        New registration
      </h1>

      <div className="is-107b010">
        {STEP_LABELS.map((label, idx) => (
          <div key={label} className="is-ffdac66">
            <div style={{ height: '3px', borderRadius: '2px', background: idx <= step ? 'var(--accent)' : 'var(--border)', transition: 'background 0.3s ease' }} />
            <span style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', color: idx === step ? 'var(--text)' : 'var(--text-muted)', fontWeight: idx === step ? 600 : 400, }}>
              {label}
            </span>
          </div>
        ))}
      </div>

      {error && (
        <div className="is-b602fb5">
          {error}
        </div>
      )}

      {loading ? (
        <PageSkeleton variant="list" noHeader rows={4} />
      ) : (
        <>
          {step === 0 && (
            <div>

              <div className="is-b75fad0">
                <div className="is-906dc5d">Season</div>
                <div className="is-18d4ed1">
                  {seasonOptions.map((season) => (
                    <button type="button"
                      key={season.id}
                      onClick={() => setSelectedSeason(season.id)}
                      style={{
                        padding: '12px 16px',
                        borderRadius: '999px',
                        border: `1px solid ${selectedSeason === season.id ? 'var(--accent)' : 'var(--border)'}`,
                        background: selectedSeason === season.id ? 'rgba(235,93,30,0.08)' : 'var(--bg-elevated)',
                        color: selectedSeason === season.id ? 'var(--accent-text, var(--accent))' : 'var(--text-subtle)',
                        cursor: 'pointer',
                        fontSize: 'var(--fs-base)',
                        fontWeight: 600,
                      }}
                    >
                      {season.label}
                    </button>
                  ))}
                </div>
              </div>

              <div className="is-930d87f">
                {programOptions.map((program) => (
                  <button type="button"
                    key={program.id}
                    onClick={() => setSelectedProgramId(program.id)}
                    style={{
                      padding: '20px 20px',
                      textAlign: 'left',
                      borderRadius: '12px',
                      cursor: 'pointer',
                      background: selectedProgramId === program.id ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                      border: `2px solid ${selectedProgramId === program.id ? 'var(--accent)' : 'var(--border)'}`,
                      transition: 'all 0.2s ease',
                      width: '100%',
                    }}
                  >
                    <div className="is-ece61a4">
                      <div>
                        <p className="is-6fb5cb4">{program.name}</p>
                        <p className="is-915a8b5">
                          {program.group.code.toUpperCase()} · {program.type}
                        </p>
                      </div>
                      <div className="is-3457e2a">
                        {program.group.min_age != null || program.group.max_age != null
                          ? `Ages ${program.group.min_age ?? '—'}-${program.group.max_age ?? '—'}`
                          : 'All ages'}
                      </div>
                    </div>
                    {program.description && (
                      <p className="is-0791cd0">
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
                Choose swimmers for <strong style={{ color: 'var(--text)' }}>{selectedProgram?.name}</strong>, {selectedSeasonLabel}.
              </p>
              <div className="is-930d87f">
                {swimmerOptions.map((swimmer) => {
                  const age = calculateAge(swimmer.date_of_birth)
                  return (
                    <button type="button"
                      key={swimmer.id}
                      onClick={() => !swimmer.alreadyRegistered && toggleSwimmer(swimmer.id)}
                      disabled={swimmer.alreadyRegistered}
                      style={{
                        padding: '16px 20px',
                        textAlign: 'left',
                        borderRadius: '12px',
                        cursor: swimmer.alreadyRegistered ? 'not-allowed' : 'pointer',
                        background: swimmer.alreadyRegistered ? 'var(--bg-surface)' : selectedSwimmers.has(swimmer.id) ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                        border: `2px solid ${swimmer.alreadyRegistered ? 'var(--border)' : selectedSwimmers.has(swimmer.id) ? 'var(--accent)' : 'var(--border)'}`,
                        width: '100%',
                        transition: 'all 0.2s ease',
                        opacity: swimmer.alreadyRegistered ? 0.7 : 1,
                      }}
                    >
                      <div className="is-c9a7335">
                        <div style={{
                          width: '36px', height: '36px', borderRadius: '50%', flexShrink: 0,
                          background: selectedSwimmers.has(swimmer.id) && !swimmer.alreadyRegistered ? 'var(--accent)' : 'var(--border)',
                          display: 'flex', alignItems: 'center', justifyContent: 'center',
                          fontSize: 'var(--fs-md)', fontWeight: 'var(--display-weight)', color: selectedSwimmers.has(swimmer.id) && !swimmer.alreadyRegistered ? 'var(--bg-elevated)' : 'var(--text-muted)',
                          fontFamily: 'var(--font-display)',
                        }}>
                          {`${swimmer.first_name[0] ?? ''}${swimmer.last_name[0] ?? ''}`.toUpperCase()}
                        </div>
                        <div style={{ flex: 1 }}>
                          <p className="is-2776d14">
                            {swimmer.first_name} {swimmer.last_name}
                          </p>
                          <p className="is-915a8b5">
                            {age != null ? `Age ${age}` : 'Age unavailable'} · {swimmer.registration_status}
                          </p>
                        </div>
                        {swimmer.alreadyRegistered && (
                          <span className="is-2b57a88">
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
                Review and accept to continue.
              </p>
              <div className="is-930d87f">
                {waiverOptions.map((waiver) => (
                  <div
                    key={waiver.id}
                    style={{
                      borderRadius: '12px',
                      overflow: 'hidden',
                      border: `1px solid ${waiverAccepted[waiver.id] ? 'rgba(22,163,74,0.3)' : 'var(--border)'}`,
                      background: waiverAccepted[waiver.id] ? 'rgba(22,163,74,0.04)' : 'var(--bg-elevated)',
                    }}
                  >
                    <button type="button"
                      aria-expanded={expandedWaiver === waiver.id}
                      onClick={() => setExpandedWaiver(expandedWaiver === waiver.id ? null : waiver.id)}
                      className="is-60a388a"
                    >
                      <div style={{
                        width: '20px', height: '20px', borderRadius: '50%', flexShrink: 0,
                        background: waiverAccepted[waiver.id] ? 'var(--success)' : 'var(--bg-wash-strong)',
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
                        <p className="is-d7b1039">
                          {waiver.title} {waiver.required && <span style={{ color: 'var(--error)' }}>*</span>}
                        </p>
                        <p className="is-f36bffb">
                          {waiver.appliesTo}
                        </p>
                      </div>
                    </button>

                    {expandedWaiver === waiver.id && (
                      <div className="is-1586e86">
                        <div className="is-690e2d2">
                          <p className="is-87a56b3">{waiver.body}</p>
                        </div>
                        <label className="is-68cd3d9">
                          <input
                            type="checkbox"
                            checked={!!waiverAccepted[waiver.id]}
                            onChange={(event) => setWaiverAccepted((prev) => ({ ...prev, [waiver.id]: event.target.checked }))}
                            className="is-67c1d5d"
                          />
                          <span className="is-71a6eb4">
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
              <div className="card is-b25ef49" >
                <div className="is-9b28c7d">
                  <p className="is-703129f">Program</p>
                  <p className="is-0bf6ab9">{selectedProgram?.name}</p>
                  <p className="is-47d4bd3">{selectedSeasonLabel}</p>
                </div>
                <div className="is-296acb1">
                  <p className="is-dd4d726">Swimmers</p>
                  {selectedSwimmerRecords.map((swimmer) => (
                    <div key={swimmer.id} className="is-3893aaf">
                      <span className="is-302d13d">{swimmer.first_name} {swimmer.last_name}</span>
                      <span className="is-cee8ee9">{swimmer.registration_status}</span>
                    </div>
                  ))}
                </div>
                <div className="is-401e6d8">
                  <p className="is-dd4d726">Accepted requirements</p>
                  <div className="is-51144f2">
                    {waiverOptions.filter((waiver) => waiverAccepted[waiver.id]).map((waiver) => (
                      <span key={waiver.id} className="is-b6ac006">
                        {waiver.title}
                      </span>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          )}
        </>
      )}

      <div className="is-1584c56">
        <button type="button"
          onClick={() => step > 0 ? setStep(step - 1) : router.push('/portal/registrations')}
          className="btn btn-secondary is-ab97eba"
          
        >
          {step === 0 ? 'Cancel' : '← Back'}
        </button>
        <button type="button"
          onClick={() => step < 3 ? setStep(step + 1) : void handleSubmit()}
          disabled={loading || !canProceed() || submitting}
          className="btn btn-primary is-ab97eba"
          
        >
          {submitting ? 'Submitting…' : step < 3 ? 'Continue →' : 'Submit Registration'}
        </button>
      </div>
    </div>
  )
}
