'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'

type Season = { id: string; name: string; startDate: string; endDate: string; fee: number; openSpots: number }
type Swimmer = { id: string; name: string; age: number; group: string | null; currentSeason: boolean }
type Waiver = { id: string; title: string; required: boolean; body: string }

const SEASONS: Season[] = [
  { id: 'ss1', name: '2025–26 Competitive Season', startDate: 'Sep 1, 2025', endDate: 'Aug 31, 2026', fee: 480, openSpots: 12 },
  { id: 'ss2', name: '2026 Summer Splash (Short Course)', startDate: 'Jun 1, 2026', endDate: 'Aug 15, 2026', fee: 280, openSpots: 8 },
]

const MY_SWIMMERS: Swimmer[] = [
  { id: 'sw1', name: 'Marcus Allen', age: 14, group: 'Senior', currentSeason: true },
  { id: 'sw2', name: 'Lily Allen', age: 10, group: null, currentSeason: false },
]

const WAIVERS: Waiver[] = [
  {
    id: 'w1', title: 'Code of Conduct Agreement', required: true,
    body: 'I agree to abide by the DARE Aquatics Code of Conduct, which includes respectful behavior toward coaches, teammates, and officials at all times. I understand that violations may result in suspension or removal from the team.',
  },
  {
    id: 'w2', title: 'Medical Release & Liability Waiver', required: true,
    body: 'I authorize DARE Aquatics staff to obtain emergency medical treatment for my child in the event I cannot be reached. I understand the risks associated with competitive swimming and release DARE Aquatics from liability for injuries arising from normal participation.',
  },
  {
    id: 'w3', title: 'Photo & Media Release', required: false,
    body: 'I grant DARE Aquatics permission to use photographs and videos of my child for promotional, educational, and fundraising purposes. This includes social media, newsletters, and website content.',
  },
]

const STEP_LABELS = ['Select Program', 'Select Swimmers', 'Review Waivers', 'Confirm']

export default function NewRegistrationPage() {
  const router = useRouter()
  const [step, setStep] = useState(0)
  const [selectedSeason, setSelectedSeason] = useState<string | null>(null)
  const [selectedSwimmers, setSelectedSwimmers] = useState<Set<string>>(new Set())
  const [waiverAccepted, setWaiverAccepted] = useState<Record<string, boolean>>({})
  const [expandedWaiver, setExpandedWaiver] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [submitted, setSubmitted] = useState(false)

  function toggleSwimmer(id: string) {
    setSelectedSwimmers((prev) => { const n = new Set(prev); if (n.has(id)) { n.delete(id) } else { n.add(id) } return n })
  }

  function canProceed(): boolean {
    if (step === 0) return !!selectedSeason
    if (step === 1) return selectedSwimmers.size > 0
    if (step === 2) return WAIVERS.filter(w => w.required).every(w => waiverAccepted[w.id])
    return true
  }

  async function handleSubmit() {
    setSubmitting(true)
    await new Promise((r) => setTimeout(r, 1500))
    setSubmitting(false)
    setSubmitted(true)
  }

  const season = SEASONS.find(s => s.id === selectedSeason)
  const totalFee = selectedSwimmers.size * (season?.fee ?? 0)

  if (submitted) {
    return (
      <div style={{ padding: '48px 24px', maxWidth: '560px', margin: '0 auto', textAlign: 'center' }}>
        <div style={{ width: '64px', height: '64px', borderRadius: '50%', background: 'var(--success-bg)', border: '1px solid var(--success-border)', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 24px' }}>
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="var(--success)" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
            <polyline points="20 6 9 17 4 12" />
          </svg>
        </div>
        <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '26px', color: 'var(--text)', marginBottom: '10px' }}>Registration submitted!</h2>
        <p style={{ fontSize: '14px', color: 'var(--text-subtle)', lineHeight: 1.7, marginBottom: '28px' }}>
          Your registration for <strong style={{ color: 'var(--text)' }}>{season?.name}</strong> has been submitted for review.
          You&apos;ll receive a confirmation once it&apos;s approved by the club.
        </p>
        <button
          onClick={() => router.push('/portal/registrations')}
          style={{ padding: '11px 28px', background: '#eb5d1e', border: 'none', borderRadius: '10px', color: '#fff', fontSize: '14px', fontWeight: 600, cursor: 'pointer', fontFamily: 'var(--font-body)' }}
        >
          Back to Registrations
        </button>
      </div>
    )
  }

  return (
    <div style={{ padding: '32px 24px 80px', maxWidth: '640px', margin: '0 auto' }}>
      {/* Header */}
      <button
        onClick={() => step > 0 ? setStep(step - 1) : router.push('/portal/registrations')}
        style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#9A9693', fontSize: '13px', fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '20px', padding: 0 }}
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
          <line x1="19" y1="12" x2="5" y2="12" /><polyline points="12 19 5 12 12 5" />
        </svg>
        {step === 0 ? 'Back to Registrations' : 'Previous step'}
      </button>

      <h1 style={{ fontFamily: '"Instrument Serif", Georgia, serif', fontSize: '28px', fontWeight: 400, color: '#111111', letterSpacing: '-0.01em', marginBottom: '24px' }}>
        New Registration
      </h1>

      {/* Step indicator */}
      <div style={{ display: 'flex', gap: '8px', marginBottom: '28px' }}>
        {STEP_LABELS.map((label, idx) => (
          <div key={label} style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '6px' }}>
            <div style={{ height: '3px', borderRadius: '2px', background: idx <= step ? '#eb5d1e' : '#E5E1DC', transition: 'background 0.3s ease' }} />
            <span style={{ fontSize: '10px', fontFamily: '"JetBrains Mono", monospace', color: idx === step ? '#111111' : '#A09D9B', fontWeight: idx === step ? 600 : 400, letterSpacing: '0.02em' }}>
              {label}
            </span>
          </div>
        ))}
      </div>

      {/* Step 0: Select Program */}
      {step === 0 && (
        <div>
          <p style={{ fontSize: '14px', color: '#6B6866', marginBottom: '20px', lineHeight: 1.6 }}>
            Choose the program or season you&apos;d like to enroll your swimmer(s) in.
          </p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            {SEASONS.map((s) => (
              <button
                key={s.id}
                onClick={() => setSelectedSeason(s.id)}
                style={{
                  padding: '18px 20px', textAlign: 'left', borderRadius: '12px', cursor: 'pointer',
                  background: selectedSeason === s.id ? 'rgba(235,93,30,0.06)' : '#FFFFFF',
                  border: `2px solid ${selectedSeason === s.id ? '#eb5d1e' : '#E5E1DC'}`,
                  transition: 'all 0.2s ease', width: '100%',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '12px' }}>
                  <div>
                    <p style={{ fontSize: '15px', fontWeight: 600, color: '#111111', marginBottom: '4px', fontFamily: '"Plus Jakarta Sans", system-ui, sans-serif' }}>{s.name}</p>
                    <p style={{ fontSize: '12px', color: '#9A9693', fontFamily: '"JetBrains Mono", monospace' }}>
                      {s.startDate} — {s.endDate}
                    </p>
                  </div>
                  <div style={{ textAlign: 'right', flexShrink: 0 }}>
                    <p style={{ fontFamily: '"Instrument Serif", Georgia, serif', fontSize: '18px', color: '#111111', marginBottom: '2px' }}>
                      ${s.fee}/swimmer
                    </p>
                    <p style={{ fontSize: '11px', color: s.openSpots < 5 ? '#d97706' : '#9A9693', fontFamily: '"JetBrains Mono", monospace' }}>
                      {s.openSpots} spots open
                    </p>
                  </div>
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Step 1: Select Swimmers */}
      {step === 1 && (
        <div>
          <p style={{ fontSize: '14px', color: '#6B6866', marginBottom: '20px', lineHeight: 1.6 }}>
            Select which swimmers to enroll in <strong style={{ color: '#111111' }}>{season?.name}</strong>.
          </p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
            {MY_SWIMMERS.map((sw) => (
              <button
                key={sw.id}
                onClick={() => !sw.currentSeason && toggleSwimmer(sw.id)}
                disabled={sw.currentSeason}
                style={{
                  padding: '16px 18px', textAlign: 'left', borderRadius: '10px', cursor: sw.currentSeason ? 'not-allowed' : 'pointer',
                  background: sw.currentSeason ? '#F5F3F0' : selectedSwimmers.has(sw.id) ? 'rgba(235,93,30,0.06)' : '#FFFFFF',
                  border: `2px solid ${sw.currentSeason ? '#E5E1DC' : selectedSwimmers.has(sw.id) ? '#eb5d1e' : '#E5E1DC'}`,
                  width: '100%', transition: 'all 0.2s ease', opacity: sw.currentSeason ? 0.6 : 1,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
                  <div style={{
                    width: '36px', height: '36px', borderRadius: '50%', flexShrink: 0,
                    background: selectedSwimmers.has(sw.id) && !sw.currentSeason ? '#eb5d1e' : '#E5E1DC',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    fontSize: '14px', fontWeight: 700, color: selectedSwimmers.has(sw.id) && !sw.currentSeason ? '#fff' : '#9A9693',
                    fontFamily: '"Instrument Serif", Georgia, serif',
                    transition: 'all 0.2s ease',
                  }}>
                    {sw.name.split(' ').map(n => n[0]).join('').slice(0, 2)}
                  </div>
                  <div style={{ flex: 1 }}>
                    <p style={{ fontSize: '14px', fontWeight: 600, color: '#111111', marginBottom: '2px', fontFamily: '"Plus Jakarta Sans", system-ui, sans-serif' }}>{sw.name}</p>
                    <p style={{ fontSize: '12px', color: '#9A9693', fontFamily: '"JetBrains Mono", monospace' }}>
                      Age {sw.age} · {sw.group ?? 'Unassigned'}
                    </p>
                  </div>
                  {sw.currentSeason && (
                    <span style={{ fontSize: '10px', padding: '2px 8px', background: '#E5E1DC', color: '#9A9693', borderRadius: '20px', fontFamily: '"JetBrains Mono", monospace', fontWeight: 600, flexShrink: 0 }}>
                      ENROLLED
                    </span>
                  )}
                </div>
              </button>
            ))}
          </div>
          {selectedSwimmers.size > 0 && (
            <div style={{ marginTop: '16px', padding: '12px 16px', background: 'rgba(235,93,30,0.06)', border: '1px solid rgba(235,93,30,0.2)', borderRadius: '8px', fontSize: '13px', color: '#111111' }}>
              {selectedSwimmers.size} swimmer{selectedSwimmers.size > 1 ? 's' : ''} selected · Estimated fee: <strong>${totalFee}</strong>
            </div>
          )}
        </div>
      )}

      {/* Step 2: Waivers */}
      {step === 2 && (
        <div>
          <p style={{ fontSize: '14px', color: '#6B6866', marginBottom: '20px', lineHeight: 1.6 }}>
            Review and accept the required agreements. Required items are marked with *.
          </p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            {WAIVERS.map((w) => (
              <div
                key={w.id}
                style={{
                  borderRadius: '10px', overflow: 'hidden', border: `1px solid ${waiverAccepted[w.id] ? 'rgba(22,163,74,0.3)' : '#E5E1DC'}`,
                  background: waiverAccepted[w.id] ? 'rgba(22,163,74,0.04)' : '#FFFFFF', transition: 'all 0.2s ease',
                }}
              >
                <button
                  onClick={() => setExpandedWaiver(expandedWaiver === w.id ? null : w.id)}
                  style={{ width: '100%', padding: '14px 16px', background: 'none', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '12px', textAlign: 'left' }}
                >
                  <div style={{
                    width: '20px', height: '20px', borderRadius: '50%', flexShrink: 0,
                    background: waiverAccepted[w.id] ? '#16a34a' : '#F0EDEA', border: `2px solid ${waiverAccepted[w.id] ? '#16a34a' : '#E5E1DC'}`,
                    display: 'flex', alignItems: 'center', justifyContent: 'center', transition: 'all 0.2s',
                  }}>
                    {waiverAccepted[w.id] && (
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth={3} strokeLinecap="round" strokeLinejoin="round">
                        <polyline points="20 6 9 17 4 12" />
                      </svg>
                    )}
                  </div>
                  <div style={{ flex: 1 }}>
                    <p style={{ fontSize: '14px', fontWeight: 600, color: '#111111', fontFamily: '"Plus Jakarta Sans", system-ui, sans-serif' }}>
                      {w.title} {w.required && <span style={{ color: '#dc2626' }}>*</span>}
                    </p>
                  </div>
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#9A9693" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0, transform: expandedWaiver === w.id ? 'rotate(180deg)' : 'rotate(0)', transition: 'transform 0.2s' }}>
                    <polyline points="6 9 12 15 18 9" />
                  </svg>
                </button>

                {expandedWaiver === w.id && (
                  <div style={{ padding: '0 16px 16px' }}>
                    <div style={{ padding: '12px', background: '#F7F4F0', borderRadius: '8px', marginBottom: '12px' }}>
                      <p style={{ fontSize: '13px', color: '#6B6866', lineHeight: 1.7, margin: 0 }}>{w.body}</p>
                    </div>
                    <label style={{ display: 'flex', alignItems: 'flex-start', gap: '10px', cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={!!waiverAccepted[w.id]}
                        onChange={(e) => setWaiverAccepted((prev) => ({ ...prev, [w.id]: e.target.checked }))}
                        style={{ marginTop: '2px', accentColor: '#eb5d1e', width: '16px', height: '16px', cursor: 'pointer' }}
                      />
                      <span style={{ fontSize: '13px', color: '#111111', lineHeight: 1.5 }}>
                        I have read and agree to the <strong>{w.title}</strong>
                        {!w.required && <span style={{ color: '#9A9693' }}> (optional)</span>}
                      </span>
                    </label>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Step 3: Confirm */}
      {step === 3 && (
        <div>
          <p style={{ fontSize: '14px', color: '#6B6866', marginBottom: '20px', lineHeight: 1.6 }}>
            Review your registration summary before submitting.
          </p>
          <div style={{ background: '#FFFFFF', border: '1px solid #E5E1DC', borderRadius: '12px', overflow: 'hidden', marginBottom: '20px' }}>
            <div style={{ padding: '16px 18px', borderBottom: '1px solid #E5E1DC', background: '#F7F4F0' }}>
              <p style={{ fontSize: '11px', fontFamily: '"JetBrains Mono", monospace', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.06em' }}>Program</p>
              <p style={{ fontSize: '15px', fontWeight: 600, color: '#111111', marginTop: '2px' }}>{season?.name}</p>
            </div>
            <div style={{ padding: '16px 18px', borderBottom: '1px solid #E5E1DC' }}>
              <p style={{ fontSize: '11px', fontFamily: '"JetBrains Mono", monospace', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '8px' }}>Swimmers</p>
              {[...selectedSwimmers].map((id) => {
                const sw = MY_SWIMMERS.find(s => s.id === id)
                return sw ? (
                  <div key={id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                    <span style={{ fontSize: '13px', color: '#111111' }}>{sw.name}</span>
                    <span style={{ fontFamily: '"JetBrains Mono", monospace', fontSize: '13px', color: '#6B6866' }}>${season?.fee}</span>
                  </div>
                ) : null
              })}
            </div>
            <div style={{ padding: '14px 18px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <p style={{ fontSize: '13px', fontWeight: 600, color: '#111111' }}>Total Due</p>
              <p style={{ fontFamily: '"Instrument Serif", Georgia, serif', fontSize: '20px', color: '#111111' }}>${totalFee}</p>
            </div>
          </div>
          <div style={{ padding: '12px 14px', background: 'rgba(37,99,235,0.06)', border: '1px solid rgba(37,99,235,0.15)', borderRadius: '8px', fontSize: '12px', color: '#2563eb' }}>
            ℹ Your registration will be reviewed by the club. An invoice will be issued once approved.
          </div>
        </div>
      )}

      {/* Nav buttons */}
      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '28px' }}>
        <button
          onClick={() => step > 0 ? setStep(step - 1) : router.push('/portal/registrations')}
          style={{ padding: '11px 22px', background: '#FFFFFF', border: '1px solid #E5E1DC', borderRadius: '10px', color: '#6B6866', fontSize: '13px', cursor: 'pointer', fontFamily: '"Plus Jakarta Sans", system-ui, sans-serif', display: 'flex', alignItems: 'center', gap: '7px' }}
        >
          {step === 0 ? 'Cancel' : '← Back'}
        </button>
        <button
          onClick={() => step < 3 ? setStep(step + 1) : handleSubmit()}
          disabled={!canProceed() || submitting}
          style={{
            padding: '11px 28px', borderRadius: '10px', border: 'none',
            background: canProceed() && !submitting ? '#eb5d1e' : '#E5E1DC',
            color: canProceed() && !submitting ? '#fff' : '#9A9693',
            fontSize: '13px', fontWeight: 600, cursor: canProceed() && !submitting ? 'pointer' : 'not-allowed',
            fontFamily: '"Plus Jakarta Sans", system-ui, sans-serif', display: 'flex', alignItems: 'center', gap: '8px',
          }}
        >
          {submitting ? 'Submitting…' : step < 3 ? 'Continue →' : 'Submit Registration'}
        </button>
      </div>
    </div>
  )
}
