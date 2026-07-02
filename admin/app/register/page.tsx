'use client'

import { Suspense, useState, useCallback, type FormEvent } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { resolvePublicOrg } from '@/lib/public-org'

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

type Role = 'parent' | 'coach' | 'read_only_staff'

const ROLE_OPTIONS: { value: Role; label: string; desc: string }[] = [
  { value: 'parent',  label: 'Parent / Guardian', desc: 'Manage swimmers, meets, billing' },
  { value: 'coach',   label: 'Coach',              desc: 'Access training tools and entries' },
  { value: 'read_only_staff', label: 'Admin Staff', desc: 'Read-only or administrative access' },
]

type FormState = {
  firstName: string
  lastName: string
  email: string
  role: Role | ''
  clubCode: string
  message: string
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
    firstName: '', lastName: '', email: '', role: '', clubCode: '', message: '',
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
      const res = await fetch(`${API_BASE}/api/v1/register`, {
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
      setApiError(message ?? 'Something went wrong. Please try again.')
      setSubmitStatus('error')
    } catch {
      setApiError('Unable to connect. Please check your connection and try again.')
      setSubmitStatus('error')
    }
  }, [form, searchParams])

  const submitting = submitStatus === 'loading'

  // ── Success state ────────────────────────────────────────────────────────────
  if (submitted) {
    return (
      <>
        <div className="rp-root rp-success-root">
          <div className="rp-success-bg" />
          <div className="rp-success-glow" />
          <div className="rp-success-card">
            <div className="rp-success-icon">
              <svg viewBox="0 0 24 24" fill="none" width="28" height="28" aria-hidden="true">
                <path d="M4 12l5 5L20 7" stroke="#eb5d1e" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </div>
            <h2 className="rp-success-title">Request received.</h2>
            <p className="rp-success-body">
              Your access request has been submitted to your club administrator.
              {"You'll receive an email at"} <strong>{form.email}</strong> when your account is approved.
            </p>
            <div className="rp-success-actions">
              <button className="rp-btn-ghost" onClick={() => router.push('/login')}>
                ← Back to sign in
              </button>
            </div>
          </div>
        </div>
        <style>{SHARED_CSS}</style>
      </>
    )
  }

  // ── Form ─────────────────────────────────────────────────────────────────────
  return (
    <>
      <div className="rp-root">
        {/* Atmospheric background */}
        <div className="rp-bg" />
        <div className="rp-glow-teal" />
        <div className="rp-glow-orange" />
        <div className="rp-lanes" />

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
              <path d="M7 22V8h6.5c3.87 0 7 3.13 7 7s-3.13 7-7 7H7z" fill="#eb5d1e" />
              <path d="M11 18v-6h2.5c1.66 0 3 1.34 3 3s-1.34 3-3 3H11z" fill="#040C16" />
            </svg>
            <span className="rp-brandname">DARE</span>
          </div>

          <div className="rp-heading">
            <h1 className="rp-title">Request access.</h1>
            <p className="rp-subtitle">
              Fill out the form below. A club administrator will review and approve your request.
            </p>
          </div>

          <form className="rp-form" onSubmit={(e) => void handleSubmit(e)} noValidate>
            {/* API error alerts */}
            {submitStatus === 'error' && (
              <div className="rp-api-alert rp-api-alert-error" role="alert">
                {apiError || 'Something went wrong. Please try again.'}
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
                    onClick={() => setRole(r.value)}
                    disabled={submitting}
                  >
                    <span className="rp-role-label">{r.label}</span>
                    <span className="rp-role-desc">{r.desc}</span>
                  </button>
                ))}
              </div>
            </div>

            {/* Club code (optional) */}
            <div className="rp-field">
              <label htmlFor="rp-code" className="rp-label">
                Club code <span className="rp-optional">(optional)</span>
              </label>
              <input
                id="rp-code"
                type="text"
                className="rp-input"
                placeholder="DARE2026"
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
      <style>{SHARED_CSS}</style>
    </>
  )
}

const SHARED_CSS = `
  /* ── Root ── */
  .rp-root {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 40px 24px;
    position: relative;
    background: #040C16;
  }
  .rp-success-root { justify-content: center; }

  /* Atmospheric bg */
  .rp-bg {
    position: fixed;
    inset: 0;
    background: #040C16;
    pointer-events: none;
  }
  .rp-glow-teal {
    position: fixed;
    width: 600px; height: 600px;
    top: -200px; right: -100px;
    background: radial-gradient(circle, rgba(0,190,215,0.14) 0%, transparent 65%);
    filter: blur(70px);
    pointer-events: none;
    animation: rpGlow 10s ease-in-out infinite;
  }
  .rp-glow-orange {
    position: fixed;
    width: 500px; height: 500px;
    bottom: -150px; left: -80px;
    background: radial-gradient(circle, rgba(235,93,30,0.1) 0%, transparent 68%);
    filter: blur(60px);
    pointer-events: none;
    animation: rpGlow 13s ease-in-out infinite reverse;
  }
  .rp-lanes {
    position: fixed;
    inset: 0;
    background-image: repeating-linear-gradient(
      -55deg,
      transparent 0px, transparent 52px,
      rgba(255,255,255,0.013) 52px, rgba(255,255,255,0.013) 54px
    );
    pointer-events: none;
  }
  @keyframes rpGlow {
    0%,100% { opacity: .65; transform: scale(1); }
    50%      { opacity: 1; transform: scale(1.06); }
  }

  /* Back link */
  .rp-back {
    position: relative;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: 6px;
    font-family: var(--font-body);
    font-size: 13px;
    color: rgba(237,233,227,0.4);
    text-decoration: none;
    margin-bottom: 28px;
    align-self: flex-start;
    max-width: 540px;
    width: 100%;
    transition: color .2s;
    animation: rpReveal .5s var(--ease-out) both;
  }
  .rp-back:hover { color: rgba(237,233,227,0.7); }

  /* Card */
  .rp-card {
    position: relative;
    z-index: 1;
    width: 100%;
    max-width: 540px;
    background: rgba(7,18,29,0.92);
    border: 1px solid rgba(255,255,255,0.07);
    border-radius: 18px;
    padding: 48px;
    box-shadow:
      0 32px 80px rgba(0,0,0,0.5),
      0 1px 0 rgba(255,255,255,0.04) inset;
    backdrop-filter: blur(12px);
    animation: rpReveal .55s var(--ease-out) .06s both;
  }
  @media (max-width: 580px) {
    .rp-card { padding: 36px 28px; }
  }

  /* Brand */
  .rp-brand {
    display: flex;
    align-items: center;
    gap: 9px;
    margin-bottom: 40px;
  }
  .rp-brandname {
    font-family: var(--font-display);
    font-style: italic;
    font-size: 21px;
    color: #EDE9E3;
    letter-spacing: -.01em;
  }

  /* Heading */
  .rp-heading { margin-bottom: 36px; }
  .rp-title {
    font-family: var(--font-display);
    font-size: 36px;
    font-weight: 400;
    color: #EDE9E3;
    margin: 0 0 8px;
    letter-spacing: -.022em;
    line-height: 1.12;
  }
  .rp-subtitle {
    font-family: var(--font-body);
    font-size: 13.5px;
    color: rgba(237,233,227,0.38);
    margin: 0;
    line-height: 1.65;
  }

  /* Form */
  .rp-form {
    display: flex;
    flex-direction: column;
    gap: 20px;
  }
  .rp-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 14px;
  }
  @media (max-width: 440px) {
    .rp-row { grid-template-columns: 1fr; }
  }
  .rp-field {
    display: flex;
    flex-direction: column;
    gap: 7px;
  }
  .rp-label {
    font-family: var(--font-body);
    font-size: 11px;
    font-weight: 600;
    color: rgba(237,233,227,0.45);
    letter-spacing: .055em;
    text-transform: uppercase;
  }
  .rp-optional {
    font-weight: 400;
    text-transform: none;
    letter-spacing: 0;
    font-size: 10.5px;
    color: rgba(237,233,227,0.28);
  }
  .rp-input, .rp-textarea {
    width: 100%;
    background: rgba(255,255,255,0.042);
    border: 1px solid rgba(255,255,255,0.09);
    border-radius: 10px;
    color: #EDE9E3;
    font-family: var(--font-body);
    font-size: 14px;
    padding: 12px 15px;
    outline: none;
    transition: border-color .2s, background .2s, box-shadow .2s;
    -webkit-font-smoothing: antialiased;
    resize: none;
  }
  .rp-input::placeholder, .rp-textarea::placeholder {
    color: rgba(237,233,227,0.2);
  }
  .rp-input:focus, .rp-textarea:focus {
    border-color: rgba(235,93,30,0.55);
    background: rgba(235,93,30,0.04);
    box-shadow: 0 0 0 3px rgba(235,93,30,0.09);
  }
  .rp-input:disabled, .rp-textarea:disabled {
    opacity: .45;
    cursor: not-allowed;
  }
  .rp-input-err { border-color: rgba(248,113,113,0.5) !important; }
  .rp-err {
    font-family: var(--font-body);
    font-size: 11px;
    color: #f87171;
    margin: 0;
  }
  .rp-hint-text {
    font-family: var(--font-body);
    font-size: 11px;
    color: rgba(237,233,227,0.28);
    line-height: 1.5;
  }

  /* Role chips */
  .rp-roles {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .rp-role-chip {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    padding: 12px 15px;
    background: rgba(255,255,255,0.03);
    border: 1px solid rgba(255,255,255,0.08);
    border-radius: 10px;
    cursor: pointer;
    text-align: left;
    font-family: var(--font-body);
    transition: background .15s, border-color .15s;
  }
  .rp-role-chip:hover:not(:disabled) {
    background: rgba(235,93,30,0.06);
    border-color: rgba(235,93,30,0.2);
  }
  .rp-role-chip:disabled { opacity: .5; cursor: not-allowed; }
  .rp-role-active {
    background: rgba(235,93,30,0.1) !important;
    border-color: rgba(235,93,30,0.45) !important;
  }
  .rp-role-label {
    font-size: 13.5px;
    font-weight: 600;
    color: rgba(237,233,227,0.85);
  }
  .rp-role-active .rp-role-label { color: #f0795a; }
  .rp-role-desc {
    font-size: 11.5px;
    color: rgba(237,233,227,0.35);
  }

  /* Submit button */
  .rp-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 9px;
    width: 100%;
    padding: 14px 20px;
    background: #eb5d1e;
    border: none;
    border-radius: 10px;
    color: #fff;
    font-family: var(--font-body);
    font-size: 14px;
    font-weight: 600;
    letter-spacing: .01em;
    cursor: pointer;
    margin-top: 4px;
    position: relative;
    overflow: hidden;
    transition: background .2s, transform .15s, box-shadow .2s;
  }
  .rp-btn::after {
    content: '';
    position: absolute;
    inset: 0;
    background: linear-gradient(90deg, transparent 0%, rgba(255,255,255,0.1) 50%, transparent 100%);
    transform: translateX(-100%);
    transition: transform .55s;
  }
  .rp-btn:hover:not(:disabled)::after { transform: translateX(100%); }
  .rp-btn:hover:not(:disabled) {
    background: #d44f14;
    box-shadow: 0 8px 28px rgba(235,93,30,0.32);
    transform: translateY(-1px);
  }
  .rp-btn:active:not(:disabled) { transform: translateY(0); box-shadow: none; }
  .rp-btn:disabled { opacity: .55; cursor: not-allowed; }
  .rp-btn-loading { pointer-events: none; }

  .rp-spinner {
    width: 15px; height: 15px;
    border: 2px solid rgba(255,255,255,0.3);
    border-top-color: #fff;
    border-radius: 50%;
    animation: rpSpin .7s linear infinite;
    flex-shrink: 0;
  }
  @keyframes rpSpin { to { transform: rotate(360deg); } }

  /* Footer note */
  .rp-footer-note {
    margin-top: 24px;
    font-family: var(--font-body);
    font-size: 13px;
    color: rgba(237,233,227,0.36);
    text-align: center;
  }
  .rp-link {
    color: #eb5d1e;
    text-decoration: none;
    font-weight: 500;
    transition: color .2s;
  }
  .rp-link:hover { color: #f5764a; }

  /* ── Success state ── */
  .rp-success-bg {
    position: fixed;
    inset: 0;
    background: #040C16;
  }
  .rp-success-glow {
    position: fixed;
    width: 600px; height: 600px;
    top: 50%; left: 50%;
    transform: translate(-50%,-50%);
    background: radial-gradient(circle, rgba(235,93,30,0.08) 0%, transparent 65%);
    filter: blur(60px);
    animation: rpGlow 8s ease-in-out infinite;
  }
  .rp-success-card {
    position: relative;
    z-index: 1;
    width: 100%;
    max-width: 440px;
    background: rgba(7,18,29,0.95);
    border: 1px solid rgba(255,255,255,0.07);
    border-radius: 18px;
    padding: 52px 44px;
    text-align: center;
    box-shadow: 0 32px 80px rgba(0,0,0,0.5);
    animation: rpReveal .6s var(--ease-out) both;
  }
  .rp-success-icon {
    width: 60px; height: 60px;
    border-radius: 50%;
    background: rgba(235,93,30,0.1);
    border: 1px solid rgba(235,93,30,0.25);
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0 auto 28px;
  }
  .rp-success-title {
    font-family: var(--font-display);
    font-size: 32px;
    font-weight: 400;
    color: #EDE9E3;
    margin: 0 0 16px;
    letter-spacing: -.02em;
  }
  .rp-success-body {
    font-family: var(--font-body);
    font-size: 14px;
    color: rgba(237,233,227,0.45);
    line-height: 1.7;
    margin: 0 0 36px;
  }
  .rp-success-body strong {
    color: rgba(237,233,227,0.7);
    font-weight: 500;
  }
  .rp-success-actions { display: flex; justify-content: center; }
  .rp-btn-ghost {
    background: none;
    border: 1px solid rgba(255,255,255,0.1);
    border-radius: 9px;
    padding: 10px 20px;
    color: rgba(237,233,227,0.5);
    font-family: var(--font-body);
    font-size: 13.5px;
    cursor: pointer;
    transition: border-color .2s, color .2s;
  }
  .rp-btn-ghost:hover {
    border-color: rgba(235,93,30,0.35);
    color: rgba(237,233,227,0.8);
  }

  /* API alert banners */
  .rp-api-alert {
    padding: 11px 14px;
    border-radius: 9px;
    font-family: var(--font-body);
    font-size: 13px;
    line-height: 1.55;
  }
  .rp-api-alert-error {
    background: rgba(248,113,113,0.1);
    border: 1px solid rgba(248,113,113,0.25);
    color: #fca5a5;
  }
  .rp-api-alert-conflict {
    background: rgba(235,93,30,0.08);
    border: 1px solid rgba(235,93,30,0.22);
    color: rgba(237,233,227,0.7);
  }

  /* Reveal animation */
  @keyframes rpReveal {
    from { opacity: 0; transform: translateY(16px); }
    to   { opacity: 1; transform: translateY(0); }
  }
`
