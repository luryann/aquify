'use client'

import { AuthBackdrop } from '@/components/auth/AuthBackdrop'
import { Suspense, useState, useCallback, type FormEvent } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { resolvePublicOrg } from '@/lib/public-org'
import './register.css'

type Role = 'parent' | 'coach'

const ROLE_OPTIONS: { value: Role; label: string; desc: string }[] = [
  { value: 'parent',  label: 'Parent / Guardian', desc: 'Manage swimmers, meets, billing' },
  { value: 'coach',   label: 'Coach',              desc: 'Access training tools and entries' },
]

type FormState = {
  firstName: string
  lastName: string
  email: string
  role: Role | ''
  clubCode: string
  message: string
  swimmerName: string
}

type FieldError = Partial<Record<keyof FormState, string>>
type SubmitStatus = 'idle' | 'loading' | 'conflict' | 'error'

function validate(f: FormState): FieldError {
  const e: FieldError = {}
  if (!f.firstName.trim())     e.firstName = 'Required'
  if (!f.lastName.trim())      e.lastName  = 'Required'
  if (!f.email.trim())         e.email     = 'Required'
  else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(f.email)) e.email = 'Enter a valid email'
  if (!f.role)                 e.role      = 'Select a role'
  return e
}

export default function RegisterPage() {
  return (
    <Suspense fallback={null}>
      <RegisterPageContent />
    </Suspense>
  )
}

function RegisterPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [form, setForm] = useState<FormState>({
    firstName: '', lastName: '', email: '', role: '', clubCode: '', message: '', swimmerName: '',
  })
  const [errors, setErrors] = useState<FieldError>({})
  const [submitStatus, setSubmitStatus] = useState<SubmitStatus>('idle')
  const [apiError, setApiError] = useState('')
  const [submitted, setSubmitted] = useState(false)

  const set = useCallback((k: keyof FormState) => (
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
      setForm(f => ({ ...f, [k]: e.target.value }))
      setErrors(er => ({ ...er, [k]: undefined }))
      setSubmitStatus('idle')
    }
  ), [])

  const setRole = useCallback((r: Role) => {
    setForm(f => ({ ...f, role: r }))
    setErrors(er => ({ ...er, role: undefined }))
    setSubmitStatus('idle')
  }, [])

  const handleSubmit = useCallback(async (e: FormEvent) => {
    e.preventDefault()
    const errs = validate(form)
    if (Object.keys(errs).length > 0) { setErrors(errs); return }
    setSubmitStatus('loading')
    setApiError('')
    try {
      const resolved = resolvePublicOrg({
        searchParams,
        host: typeof window === 'undefined' ? '' : window.location.hostname,
        clubCode: form.clubCode,
      })
      if (!resolved.orgId) {
        setApiError('We could not determine your club. Use the registration link from your club or enter a valid club code.')
        setSubmitStatus('error')
        return
      }
      const res = await fetch('/api/v1/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          first_name: form.firstName.trim(),
          last_name: form.lastName.trim(),
          email: form.email.trim().toLowerCase(),
          org_id: resolved.orgId,
          requested_role: form.role,
          club_code: resolved.clubCode || undefined,
          note: form.message.trim() || undefined,
          swimmer_name: form.swimmerName.trim() || undefined,
        }),
      })
      if (res.status === 201 || res.status === 200) {
        setSubmitStatus('idle')
        setSubmitted(true)
        return
      }
      if (res.status === 409) {
        setSubmitStatus('conflict')
        return
      }
      const data = await res.json().catch(() => ({}))
      const message = (data as { message?: string; error?: { message?: string } }).error?.message
        ?? (data as { message?: string }).message
      setApiError(message ?? 'Couldn’t send your request. Try again.')
      setSubmitStatus('error')
    } catch {
      setApiError('Couldn’t connect. Check your connection and try again.')
      setSubmitStatus('error')
    }
  }, [form, searchParams])

  const submitting = submitStatus === 'loading'

  // ── Success state ────────────────────────────────────────────────────────────
  if (submitted) {
    return (
      <>
        <div className="rp-root rp-success-root">
          <AuthBackdrop />
          <div className="rp-success-card">
            <div className="rp-success-icon">
              <svg viewBox="0 0 24 24" fill="none" width="28" height="28" aria-hidden="true">
                <path d="M4 12l5 5L20 7" stroke="var(--accent)" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </div>
            <h2 className="rp-success-title">Request received.</h2>
            <p className="rp-success-body">
              Your access request has been submitted to your club administrator.
              {"You'll receive an email at"} <strong>{form.email}</strong> when your account is approved.
              {form.role === 'parent' && (
                <>
                  {' '}This creates your account only — a club administrator will follow up
                  separately about registering
                  {form.swimmerName.trim() ? <> <strong>{form.swimmerName.trim()}</strong></> : ' your swimmer'} for the season.
                </>
              )}
            </p>
            <div className="rp-success-actions">
              <button type="button" className="rp-btn-ghost" onClick={() => router.push('/login')}>
                ← Back to sign in
              </button>
            </div>
          </div>
        </div>
        </>
    )
  }

  // ── Form ─────────────────────────────────────────────────────────────────────
  return (
    <>
      <div className="rp-root">
        {/* Atmospheric background */}
        <AuthBackdrop />

        {/* Back link */}
        <a href="/login" className="rp-back">
          <svg viewBox="0 0 16 16" fill="none" width="13" height="13" aria-hidden="true">
            <path d="M10 3L5 8l5 5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
          Back to sign in
        </a>

        {/* Card */}
        <div className="rp-card">
          {/* Brand */}
          <div className="rp-brand">
            <svg width="30" height="30" viewBox="0 0 30 30" fill="none" aria-hidden="true">
              <rect width="30" height="30" rx="8" fill="rgba(235,93,30,0.12)" />
              <path d="M7 22V8h6.5c3.87 0 7 3.13 7 7s-3.13 7-7 7H7z" fill="var(--accent)" />
              <path d="M11 18v-6h2.5c1.66 0 3 1.34 3 3s-1.34 3-3 3H11z" fill="#040C16" />
            </svg>
            <span className="rp-brandname">aquify</span>
          </div>

          <div className="rp-heading">
            <h1 className="rp-title">Request access.</h1>
            <p className="rp-subtitle">
              A club administrator will review and approve your request.
            </p>
          </div>

          <form className="rp-form" onSubmit={(e) => void handleSubmit(e)} noValidate>
            {/* API error alerts */}
            {submitStatus === 'error' && (
              <div className="rp-api-alert rp-api-alert-error" role="alert">
                {apiError || 'Couldn’t send your request. Try again.'}
              </div>
            )}
            {submitStatus === 'conflict' && (
              <div className="rp-api-alert rp-api-alert-conflict" role="alert">
                An account with that email already exists.{' '}
                <a href="/login" className="rp-link">Sign in instead →</a>
              </div>
            )}
            {/* Name row */}
            <div className="rp-row">
              <div className="rp-field">
                <label htmlFor="rp-fname" className="rp-label">First name</label>
                <input
                  id="rp-fname"
                  type="text"
                  className={`rp-input${errors.firstName ? ' rp-input-err' : ''}`}
                  placeholder="Jane"
                  value={form.firstName}
                  onChange={set('firstName')}
                  autoComplete="given-name"
                  disabled={submitting}
                />
                {errors.firstName && <span className="rp-err">{errors.firstName}</span>}
              </div>
              <div className="rp-field">
                <label htmlFor="rp-lname" className="rp-label">Last name</label>
                <input
                  id="rp-lname"
                  type="text"
                  className={`rp-input${errors.lastName ? ' rp-input-err' : ''}`}
                  placeholder="Chen"
                  value={form.lastName}
                  onChange={set('lastName')}
                  autoComplete="family-name"
                  disabled={submitting}
                />
                {errors.lastName && <span className="rp-err">{errors.lastName}</span>}
              </div>
            </div>

            {/* Email */}
            <div className="rp-field">
              <label htmlFor="rp-email" className="rp-label">Email address</label>
              <input
                id="rp-email"
                type="email"
                className={`rp-input${errors.email ? ' rp-input-err' : ''}`}
                placeholder="jane@example.com"
                value={form.email}
                onChange={set('email')}
                autoComplete="email"
                disabled={submitting}
              />
              {errors.email && <span className="rp-err">{errors.email}</span>}
            </div>

            {/* Role selection */}
            <div className="rp-field">
              <span className="rp-label">I am a…</span>
              {errors.role && <span className="rp-err">{errors.role}</span>}
              <div className="rp-roles">
                {ROLE_OPTIONS.map(r => (
                  <button
                    key={r.value}
                    type="button"
                    className={`rp-role-chip${form.role === r.value ? ' rp-role-active' : ''}`}
                    aria-pressed={form.role === r.value}
                    onClick={() => setRole(r.value)}
                    disabled={submitting}
                  >
                    <span className="rp-role-label">{r.label}</span>
                    <span className="rp-role-desc">{r.desc}</span>
                  </button>
                ))}
              </div>
            </div>

            {/* Swimmer name (parent only, optional) */}
            {form.role === 'parent' && (
              <div className="rp-field">
                <label htmlFor="rp-swimmer" className="rp-label">
                  Swimmer&rsquo;s name <span className="rp-optional">(optional)</span>
                </label>
                <input
                  id="rp-swimmer"
                  type="text"
                  className="rp-input"
                  placeholder="Emma Chen"
                  value={form.swimmerName}
                  onChange={set('swimmerName')}
                  autoComplete="off"
                  disabled={submitting}
                  maxLength={120}
                />
                <span className="rp-hint-text">
                  Tell us who you&rsquo;re swimming for so staff can follow up.
                </span>
              </div>
            )}

            {/* Club code (optional) */}
            <div className="rp-field">
              <label htmlFor="rp-code" className="rp-label">
                Club code <span className="rp-optional">(optional)</span>
              </label>
              <input
                id="rp-code"
                type="text"
                className="rp-input"
                placeholder="AQUIFY2026"
                value={form.clubCode}
                onChange={set('clubCode')}
                autoComplete="off"
                disabled={submitting}
                maxLength={20}
              />
              <span className="rp-hint-text">
                If your administrator gave you a code, enter it here.
              </span>
            </div>

            {/* Message (optional) */}
            <div className="rp-field">
              <label htmlFor="rp-msg" className="rp-label">
                Note to administrator <span className="rp-optional">(optional)</span>
              </label>
              <textarea
                id="rp-msg"
                className="rp-textarea"
                placeholder="E.g. I'm the parent of swimmer Emma Chen on the Senior group."
                value={form.message}
                onChange={set('message')}
                rows={3}
                disabled={submitting}
                maxLength={500}
              />
            </div>

            <button
              type="submit"
              className={`rp-btn${submitting ? ' rp-btn-loading' : ''}`}
              disabled={submitting}
            >
              {submitting ? (
                <>
                  <span className="rp-spinner" />
                  Submitting request…
                </>
              ) : 'Submit access request'}
            </button>
          </form>

          <div className="rp-footer-note">
            Already have an account?{' '}
            <a href="/login" className="rp-link">Sign in →</a>
          </div>
        </div>
      </div>
    </>
  )
}
