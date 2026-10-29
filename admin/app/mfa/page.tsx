'use client'

import { AuthBackdrop } from '@/components/auth/AuthBackdrop'
import { useState, useRef, useEffect, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { useSession } from '@/lib/session'
import { destinationForRole } from '@/lib/auth-routing'
import { resolveMfaBootstrapState, type MfaFactorSummary } from '@/lib/mfa-flow'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import './page.inline.css'
import { rawRequestAuthHeaders } from '@/lib/api'

// ─── Types ────────────────────────────────────────────────────────────────────

type Stage = 'loading' | 'setup-scan' | 'setup-verify' | 'setup-backup' | 'verify' | 'error'

// ─── Keyframes ────────────────────────────────────────────────────────────────

const KEYFRAMES = `
@keyframes mfaFadeUp {
  from { opacity: 0; transform: translateY(16px); }
  to   { opacity: 1; transform: translateY(0); }
}
@keyframes mfaSpin {
  to { transform: rotate(360deg); }
}
@keyframes mfaShake {
  0%, 100% { transform: translateX(0); }
  20%      { transform: translateX(-8px); }
  40%      { transform: translateX(8px); }
  60%      { transform: translateX(-5px); }
  80%      { transform: translateX(5px); }
}
@keyframes mfaPulse {
  0%, 100% { opacity: 0.4; transform: scale(0.95); }
  50%      { opacity: 1;   transform: scale(1.05); }
}
`

// ─── Step indicator ───────────────────────────────────────────────────────────

const STEP_LABELS: Record<number, string> = { 1: 'SCAN', 2: 'VERIFY', 3: 'BACKUP' }

function SetupSteps({ step }: { step: 1 | 2 | 3 }) {
  return (
    <div className="is-b91a204">
      {[1, 2, 3].map((n) => {
        const done = n < step
        const active = n === step
        return (
          <div key={n} className="is-3088da1">
            <div style={{
              width: '24px', height: '24px', borderRadius: '50%',
              background: done ? 'var(--accent)' : active ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
              border: `1px solid ${done || active ? 'var(--accent)' : 'var(--border)'}`,
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              transition: 'all 0.2s ease',
            }}>
              {done ? (
                <svg width="11" height="11" viewBox="0 0 12 12" fill="none">
                  <path d="M2 6l3 3 5-5" stroke="#fff" strokeWidth={1.8}
                    strokeLinecap="round" strokeLinejoin="round" />
                </svg>
              ) : (
                <span style={{
                  fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', fontWeight: 700,
                  color: active ? 'var(--accent)' : 'var(--text-muted)',
                }}>{n}</span>
              )}
            </div>
            <span style={{
              fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)',
              color: active ? 'var(--text)' : done ? 'var(--text-muted)' : 'var(--text-muted)',
              letterSpacing: '0.04em',
              fontWeight: active ? 600 : 400,
            }}>
              {STEP_LABELS[n]}
            </span>
            {n < 3 && (
              <div style={{
                width: '24px', height: '1px',
                background: done ? 'var(--accent)' : 'var(--border)',
                transition: 'background 0.3s ease',
              }} />
            )}
          </div>
        )
      })}
    </div>
  )
}

// ─── QR canvas ────────────────────────────────────────────────────────────────

function QRCanvas({ uri }: { uri: string }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [renderFailed, setRenderFailed] = useState(false)

  useEffect(() => {
    if (!uri || !canvasRef.current) return
    let cancelled = false
    setRenderFailed(false)
    import('qrcode').then((QRCode) => {
      if (cancelled || !canvasRef.current) return
      return QRCode.toCanvas(canvasRef.current, uri, {
        width: 180,
        margin: 2,
        color: { dark: '#09090b', light: '#ffffff' },
      })
    }).catch(() => {
      if (!cancelled) {
        setRenderFailed(true)
      }
    })
    return () => { cancelled = true }
  }, [uri])

  return (
    <div className="is-81b8ec2">
      <div className="is-1b7f04f">
        {renderFailed ? (
          <div className="is-deb28fe">
            QR rendering failed. Use the manual setup key below instead.
          </div>
        ) : (
          <canvas ref={canvasRef} style={{ display: 'block', borderRadius: '4px' }} />
        )}
      </div>
    </div>
  )
}

// ─── Digit inputs (shared between setup-verify and verify) ────────────────────

function DigitInputs({
  digits,
  inputRefs,
  onChange,
  onKeyDown,
  onPaste,
}: {
  digits: string[]
  inputRefs: React.MutableRefObject<(HTMLInputElement | null)[]>
  onChange: (idx: number, val: string) => void
  onKeyDown: (idx: number, e: React.KeyboardEvent<HTMLInputElement>) => void
  onPaste: (e: React.ClipboardEvent) => void
}) {
  return (
    <div
      className="is-85bbadf"
      onPaste={onPaste}
    >
      {digits.map((d, idx) => (
        <input
          key={idx}
          ref={(el) => { inputRefs.current[idx] = el }}
          type="text"
          inputMode="numeric"
          aria-label={`Digit ${idx + 1} of ${digits.length}`}
          maxLength={1}
          value={d}
          onChange={(e) => onChange(idx, e.target.value)}
          onKeyDown={(e) => onKeyDown(idx, e)}
          style={{ width: '46px',
            textAlign: 'center',
            fontWeight: 600,
            caretColor: 'var(--accent)' }}
        />
      ))}
    </div>
  )
}

// ─── Copy button ──────────────────────────────────────────────────────────────

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      type="button"
      onClick={() => {
        navigator.clipboard.writeText(text).then(() => {
          setCopied(true)
          setTimeout(() => setCopied(false), 1800)
        })
      }}
      style={{
        background: copied ? 'rgba(52,211,153,0.1)' : 'var(--bg-elevated)',
        border: `1px solid ${copied ? 'rgba(52,211,153,0.3)' : 'var(--border)'}`,
        borderRadius: '6px',
        padding: '4px 12px',
        fontSize: 'var(--fs-xs)',
        fontFamily: 'var(--font-body)',
        color: copied ? '#34d399' : 'var(--text-muted)',
        cursor: 'pointer',
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
        transition: 'all 0.2s ease',
        flexShrink: 0,
      }}
    >
      {copied ? (
        <>
          <svg width="10" height="10" viewBox="0 0 12 12" fill="none">
            <path d="M2 6l3 3 5-5" stroke="currentColor" strokeWidth={1.8}
              strokeLinecap="round" strokeLinejoin="round" />
          </svg>
          copied
        </>
      ) : (
        <>
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor"
            strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
            <rect x="9" y="9" width="13" height="13" rx="2" />
            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
          </svg>
          copy
        </>
      )}
    </button>
  )
}

// ─── Main page ────────────────────────────────────────────────────────────────

export default function MfaPage() {
  const router = useRouter()
  const { refreshSession, session, signOut } = useSession()
  const [mounted, setMounted] = useState(false)
  const [stage, setStage] = useState<Stage>('loading')
  const [mode, setMode] = useState<'totp' | 'backup'>('totp')
  const [digits, setDigits] = useState<string[]>(Array(6).fill(''))
  const [backupCode, setBackupCode] = useState('')
  const [factorId, setFactorId] = useState('')
  const [totpFactorId, setTotpFactorId] = useState('')
  const [backupFactorId, setBackupFactorId] = useState('')
  const [provisioningUri, setProvisioningUri] = useState('')
  const [totpSecret, setTotpSecret] = useState('')
  const [showManualKey, setShowManualKey] = useState(false)
  const [backupCodes, setBackupCodes] = useState<string[]>([])
  const [backupSaved, setBackupSaved] = useState(false)
  const [backupGenFailed, setBackupGenFailed] = useState(false)
  const [backupRetrying, setBackupRetrying] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [shake, setShake] = useState(false)
  const inputRefs = useRef<(HTMLInputElement | null)[]>([])

  useEffect(() => {
    setMounted(true)
    const id = 'dare-mfa-kf'
    if (!document.getElementById(id)) {
      const s = document.createElement('style')
      s.id = id
      s.textContent = KEYFRAMES
      document.head.appendChild(s)
    }
  }, [])

  // Auto-focus digit inputs when entering a verify stage
  useEffect(() => {
    if (stage === 'verify' || stage === 'setup-verify') {
      setTimeout(() => inputRefs.current[0]?.focus(), 300)
    }
  }, [stage])

  const enrollTotpFactor = useCallback(async () => {
    const headers = { 'Content-Type': 'application/json', ...rawRequestAuthHeaders(undefined, 'POST') }

    const res = await fetch('/api/v1/auth/mfa/enroll', {
      method: 'POST',
      headers,
      credentials: 'include',
      body: JSON.stringify({ factor_type: 'totp', label: 'Authenticator App' }),
    })
    if (!res.ok) throw new Error('Couldn’t set up two-factor authentication. Try again.')
    const payload = await res.json() as {
      factor: {
        id: string
        provisioning_uri?: string
        challenge_hint?: string
      }
    }

    setFactorId(payload.factor.id)
    setTotpFactorId(payload.factor.id)
    setBackupFactorId('')
    setBackupCodes([])
    setBackupSaved(false)
    setBackupGenFailed(false)
    setBackupRetrying(false)
    setMode('totp')

    const uri = payload.factor.provisioning_uri ?? ''
    if (!uri) {
      throw new Error('Couldn’t generate the QR code. Refresh the page and try again.')
    }

    setProvisioningUri(uri)
    const secretMatch = uri.match(/secret=([^&]+)/)
    if (secretMatch) {
      setTotpSecret(secretMatch[1])
    } else if (payload.factor.challenge_hint) {
      setTotpSecret(payload.factor.challenge_hint)
    } else {
      setTotpSecret('')
    }

    setStage('setup-scan')
  }, [])

  const enrollBackupFactor = useCallback(async () => {
    const headers = { 'Content-Type': 'application/json', ...rawRequestAuthHeaders(undefined, 'POST') }

    const res = await fetch('/api/v1/auth/mfa/enroll', {
      method: 'POST',
      headers,
      credentials: 'include',
      body: JSON.stringify({ factor_type: 'backup_code', label: 'Backup Codes' }),
    })
    if (!res.ok) {
      setBackupGenFailed(true)
      setStage('setup-backup')
      return false
    }

    const payload = await res.json() as {
      factor: { id: string; codes?: string[] }
    }
    const codes = payload.factor.codes ?? []

    if (payload.factor.id) setBackupFactorId(payload.factor.id)
    setMode('totp')
    setBackupCodes(codes)
    setBackupSaved(false)
    setBackupGenFailed(codes.length === 0)
    setBackupRetrying(false)
    setStage('setup-backup')

    return codes.length > 0
  }, [])

  // Load existing factors on mount; if none found, auto-enroll TOTP
  useEffect(() => {
    let cancelled = false

    async function init() {
      // 1. Check for existing factors
      try {
        const res = await fetch('/api/v1/auth/mfa/factors', {
          method: 'GET',
          credentials: 'include',
          cache: 'no-store',
        })
        if (res.status === 401) {
          // No valid session cookie — this isn't "zero factors enrolled,"
          // it's "not actually signed in." Send back to login instead of
          // dead-ending on a generic error the user can't act on.
          if (!cancelled) router.replace('/login')
          return
        }
        if (!res.ok) throw new Error('Couldn’t load verification methods. Try again.')
        const payload = await res.json() as {
          factors: MfaFactorSummary[]
        }

        if (cancelled) return

        const next = resolveMfaBootstrapState(payload.factors)

        if (next.kind === 'verify') {
          setTotpFactorId(next.totpFactorId ?? '')
          setBackupFactorId(next.backupFactorId ?? '')
          setMode(next.mode)
          setFactorId(next.mode === 'backup' ? next.backupFactorId ?? '' : next.totpFactorId ?? next.backupFactorId ?? '')
          setStage('verify')
          return
        }

        if (next.kind === 'setup-backup') {
          setTotpFactorId(next.totpFactorId)
          setFactorId(next.totpFactorId)
          await enrollBackupFactor()
          return
        }
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load verification methods. Try again.'))
          setStage('error')
        }
        return
      }

      try {
        await enrollTotpFactor()
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t set up two-factor authentication. Refresh the page and try again.'))
          setStage('error')
        }
      }
    }

    init()
    return () => { cancelled = true }
  }, [enrollBackupFactor, enrollTotpFactor, router])

  useEffect(() => {
    setFactorId(mode === 'totp' ? totpFactorId : backupFactorId)
  }, [mode, totpFactorId, backupFactorId])

  // ── Digit input handlers ──────────────────────────────────────────────────

  const handleDigitChange = useCallback((idx: number, val: string) => {
    if (!/^\d?$/.test(val)) return
    const next = [...digits]
    next[idx] = val
    setDigits(next)
    setError('')
    if (val && idx < 5) inputRefs.current[idx + 1]?.focus()
  }, [digits])

  const handleDigitKeyDown = useCallback((idx: number, e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Backspace' && !digits[idx] && idx > 0) inputRefs.current[idx - 1]?.focus()
    if (e.key === 'ArrowLeft' && idx > 0) inputRefs.current[idx - 1]?.focus()
    if (e.key === 'ArrowRight' && idx < 5) inputRefs.current[idx + 1]?.focus()
  }, [digits])

  const handleDigitPaste = useCallback((e: React.ClipboardEvent) => {
    e.preventDefault()
    const text = e.clipboardData.getData('text').replace(/\D/g, '').slice(0, 6)
    if (!text) return
    const next = [...digits]
    for (let i = 0; i < text.length; i++) next[i] = text[i]
    setDigits(next)
    inputRefs.current[Math.min(text.length, 5)]?.focus()
  }, [digits])

  // ── Verify / challenge ────────────────────────────────────────────────────

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault()
    const code = mode === 'totp' ? digits.join('') : backupCode.trim()
    if (!factorId) {
      setError('No enrolled factor available.')
      triggerShake()
      return
    }
    if ((mode === 'totp' && code.length < 6) || (mode === 'backup' && !code)) {
      setError('Enter the full code.')
      triggerShake()
      return
    }
    setLoading(true)
    setError('')
    try {
      const headers = { 'Content-Type': 'application/json', ...rawRequestAuthHeaders(undefined, 'POST') }

      const res = await fetch('/api/v1/auth/mfa/challenge', {
        method: 'POST',
        headers,
        credentials: 'include',
        body: JSON.stringify({ factor_id: factorId, challenge_code: code }),
      })
      if (!res.ok) {
        const payload = await res.json().catch(() => ({})) as { message?: string }
        throw new Error(payload.message ?? 'Verification failed.')
      }

      // During setup-verify, generate backup codes before redirecting
      if (stage === 'setup-verify') {
        try {
          await enrollBackupFactor()
        } catch {
          setBackupGenFailed(true)
          setStage('setup-backup')
        }
        setLoading(false)
        return
      }

      const next = await refreshSession()
      setLoading(false)
      router.replace(destinationForRole(next?.user.roles ?? session?.user.roles ?? ['admin']))
    } catch (err) {
      setLoading(false)
      setError(uiErrorMessage(err, 'Verification failed.'))
      triggerShake()
    }
  }

  async function handleFinishSetup() {
    try {
      const next = await refreshSession()
      router.replace(destinationForRole(next?.user.roles ?? session?.user.roles ?? ['admin']))
    } catch {
      setError('Your session couldn’t be refreshed. Sign in again.')
    }
  }

  async function retryGenerateBackupCodes() {
    setBackupRetrying(true)
    setBackupGenFailed(false)
    try {
      await enrollBackupFactor()
    } catch {
      setBackupGenFailed(true)
    }
    setBackupRetrying(false)
  }

  function triggerShake() {
    setShake(true)
    setTimeout(() => setShake(false), 600)
  }

  const totpComplete = digits.every((d) => d !== '')
  const availableVerifyModes = ([
    totpFactorId ? 'totp' : null,
    backupFactorId ? 'backup' : null,
  ].filter(Boolean) as Array<'totp' | 'backup'>)

  // ── Shared layout pieces ──────────────────────────────────────────────────

  const errorRow = error && (
    <p className="is-c80a2df">
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor"
        strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
        <circle cx="12" cy="12" r="10" />
        <line x1="12" y1="8" x2="12" y2="12" />
        <line x1="12" y1="16" x2="12.01" y2="16" />
      </svg>
      {error}
    </p>
  )

  const handleCancel = async () => {
    // Best-effort: terminate the cookie-backed session, but always clear local
    // state and navigate away so AuthGuard won't redirect back to /mfa.
    try {
      await fetch('/api/v1/auth/sessions/logout', { method: 'POST', credentials: 'include' })
    } catch {
      // Ignore — local sign-out proceeds regardless.
    }
    signOut()
    router.replace('/login')
  }

  const cancelLink = (
    <div className="is-c749c16">
      <button type="button"
        onClick={handleCancel}
        className="is-208f141 hv-ce78c9"
      >
        Cancel and return to sign in
      </button>
    </div>
  )

  const submitBtn = (label: string, disabled: boolean) => (
    <button
      type="submit"
      disabled={disabled || loading}
      style={{
        width: '100%', padding: '12px 20px',
        background: disabled || loading ? 'var(--bg-elevated)' : 'var(--accent)',
        border: `1px solid ${disabled || loading ? 'var(--border)' : 'transparent'}`,
        borderRadius: '12px',
        color: disabled || loading ? 'var(--text-muted)' : '#fff',
        fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)',
        cursor: (disabled || loading) ? 'not-allowed' : 'pointer',
        display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '8px',
        transition: 'all 0.15s ease', letterSpacing: '-0.01em',
      }}
    >
      {loading ? (
        <>
          <div style={{
            width: '14px', height: '14px', borderRadius: '50%',
            border: '2px solid var(--border)', borderTopColor: 'var(--text-subtle)',
            animation: 'mfaSpin 0.8s linear infinite',
          }} />
          Verifying…
        </>
      ) : label}
    </button>
  )

  // ── Render ────────────────────────────────────────────────────────────────

  return (
    <div className="is-bce601c">
      <AuthBackdrop />

      <div style={{
        position: 'relative',
        width: '100%',
        maxWidth: (stage === 'setup-scan' || stage === 'setup-backup') ? '460px' : (stage === 'error' ? '400px' : '420px'),
        padding: '0 24px',
        opacity: mounted ? 1 : 0,
        transform: mounted ? 'translateY(0)' : 'translateY(20px)',
        transition: 'opacity 0.4s var(--ease-out), transform 0.4s var(--ease-out), max-width 0.3s ease',
      }}>

        {/* ── Loading ── */}
        {stage === 'loading' && (
          <div className="is-53d877f">
            <div style={{
              width: '12px', height: '12px', borderRadius: '50%',
              background: 'var(--accent)',
              animation: 'mfaPulse 1.4s ease-in-out infinite',
            }} />
            <p className="is-552bbb8">
              Checking your account…
            </p>
          </div>
        )}

        {/* ── Error (init failed) ── */}
        {stage === 'error' && (
          <div className="is-1d9cfb1">
            <div className="is-6a996f7">
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
                stroke="var(--error)" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="10" />
                <line x1="12" y1="8" x2="12" y2="12" />
                <line x1="12" y1="16" x2="12.01" y2="16" />
              </svg>
            </div>
            <p className="is-2f5b248">
              Something went wrong
            </p>
            <p className="is-471f9eb">
              {error || 'Couldn’t load your two-factor authentication settings. Try again.'}
            </p>
            <button type="button"
              onClick={() => { setStage('loading'); setError(''); window.location.reload() }}
              className="is-295e6d6"
            >
              Try again
            </button>
            {cancelLink}
          </div>
        )}

        {/* ── Setup: Scan QR ── */}
        {stage === 'setup-scan' && (
          <div className="is-c5cf717">
            <SetupSteps step={1} />

            <h1 className="is-0faea01">
              Set up two-factor auth
            </h1>
            <p className="is-6f544b3">
              Scan this QR code with your authenticator app, then continue.
            </p>

            {provisioningUri ? (
              <QRCanvas uri={provisioningUri} />
            ) : (
              <div className="is-d2b051a">
                <div style={{
                  width: '10px', height: '10px', borderRadius: '50%',
                  background: 'var(--accent)', animation: 'mfaPulse 1.4s ease-in-out infinite',
                }} />
              </div>
            )}

            {/* Recommended apps */}
            <div className="is-8915ff3">
              {['Google Authenticator', 'Authy', 'Apple Passwords', 'Bitwarden'].map((app) => (
                <span key={app} className="is-c9ded42">
                  {app}
                </span>
              ))}
            </div>

            {/* Manual key toggle */}
            <div className="is-8d70334">
              <button
                type="button"
                onClick={() => setShowManualKey((v) => !v)}
                className="is-b26264f hv-5b662a"
              >
                <svg width="11" height="11" viewBox="0 0 16 16" fill="none"
                  style={{ transform: showManualKey ? 'rotate(180deg)' : 'none', transition: 'transform 0.2s', flexShrink: 0 }}>
                  <path d="M4 6l4 4 4-4" stroke="currentColor" strokeWidth={1.4} strokeLinecap="round" />
                </svg>
                {showManualKey ? 'Hide' : "Can't scan? Enter the key manually"}
              </button>

              {showManualKey && totpSecret && (
                <div className="is-5c0465d">
                  <span className="is-09dcabf">
                    {totpSecret.match(/.{1,4}/g)?.join(' ') ?? totpSecret}
                  </span>
                  <CopyButton text={totpSecret} />
                </div>
              )}
            </div>

            <button
              type="button"
              onClick={() => { setDigits(Array(6).fill('')); setError(''); setStage('setup-verify') }}
              className="is-10570e9 hv-669692"
            >
              I&apos;ve scanned it — Continue
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                <path d="M6 4l4 4-4 4" stroke="currentColor" strokeWidth={1.6} strokeLinecap="round" />
              </svg>
            </button>

            {cancelLink}
          </div>
        )}

        {/* ── Setup: Verify new TOTP ── */}
        {stage === 'setup-verify' && (
          <div style={{
            background: 'var(--bg-surface)', border: '1px solid var(--border)',
            borderRadius: '12px', padding: '40px',
            animation: shake ? 'mfaShake 0.5s ease' : 'none',
          }}>
            <SetupSteps step={2} />

            <h1 className="is-0faea01">
              Confirm your setup
            </h1>
            <p className="is-6f544b3">
              Enter the 6-digit code shown in your authenticator app to confirm.
            </p>

            <form onSubmit={handleVerify}>
              <DigitInputs
                digits={digits}
                inputRefs={inputRefs}
                onChange={handleDigitChange}
                onKeyDown={handleDigitKeyDown}
                onPaste={handleDigitPaste}
              />
              {errorRow}
              {submitBtn('Confirm & Activate', !totpComplete)}
            </form>

            <div className="is-d1b3fe6">
              <button
                type="button"
                onClick={() => { setError(''); setStage('setup-scan') }}
                className="is-208f141 hv-ce78c9"
              >
                ← Back to QR code
              </button>
            </div>
          </div>
        )}

        {/* ── Setup: Save backup codes ── */}
        {stage === 'setup-backup' && (
          <div className="is-c5cf717">
            <SetupSteps step={3} />

            <h1 className="is-0faea01">
              Save your backup codes
            </h1>
            <p className="is-6a60b9a">
              Store these codes somewhere safe. Each can be used once if you lose access to your authenticator app.
            </p>

            {backupCodes.length > 0 ? (
              <>
                <div className="is-ff5fcc0">
                  <div className="is-283d81b">
                    {backupCodes.map((code, i) => (
                      <div key={i} className="is-3c66856">
                        <span className="is-b4ef7c6">
                          {code}
                        </span>
                        <CopyButton text={code} />
                      </div>
                    ))}
                  </div>
                </div>

                <div className="is-341a398">
                  <CopyButton text={backupCodes.join('\n')} />
                </div>
              </>
            ) : (
              <div className="is-4f752e5">
                <p className="is-5c2b859">
                  Backup codes could not be generated. You must save backup codes before finishing setup.
                </p>
                <button
                  type="button"
                  onClick={retryGenerateBackupCodes}
                  disabled={backupRetrying}
                  style={{
                    padding: '8px 20px',
                    background: 'var(--accent-fill, #c9460f)',
                    border: 'none',
                    borderRadius: '8px',
                    color: '#fff',
                    fontSize: 'var(--fs-base)',
                    fontWeight: 600,
                    fontFamily: 'var(--font-body)',
                    cursor: backupRetrying ? 'not-allowed' : 'pointer',
                    opacity: backupRetrying ? 0.6 : 1,
                  }}
                >
                  {backupRetrying ? 'Retrying…' : 'Retry'}
                </button>
              </div>
            )}

            {/* Acknowledgment checkbox — only shown when codes were successfully generated */}
            <label style={{
              display: backupCodes.length > 0 ? 'flex' : 'none', alignItems: 'flex-start', gap: '12px',
              cursor: 'pointer', marginBottom: '20px',
            }}>
              <input
                type="checkbox"
                checked={backupSaved}
                onChange={(e) => setBackupSaved(e.target.checked)}
                className="is-3c6539b"
              />
              <span className="is-4831fd9">
                I&apos;ve saved my backup codes in a secure location and understand each code can only be used once.
              </span>
            </label>

            <button
              type="button"
              disabled={!backupSaved || backupGenFailed || backupCodes.length === 0}
              onClick={handleFinishSetup}
              style={{
                width: '100%', padding: '12px 20px',
                background: (backupSaved && !backupGenFailed && backupCodes.length > 0) ? 'var(--accent)' : 'var(--bg-elevated)',
                border: `1px solid ${(backupSaved && !backupGenFailed && backupCodes.length > 0) ? 'transparent' : 'var(--border)'}`,
                borderRadius: '12px',
                color: (backupSaved && !backupGenFailed && backupCodes.length > 0) ? '#fff' : 'var(--text-muted)',
                fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)',
                cursor: (backupSaved && !backupGenFailed && backupCodes.length > 0) ? 'pointer' : 'not-allowed',
                display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px',
                transition: 'all 0.15s ease', letterSpacing: '-0.01em',
              }}
            >
              Finish setup
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                <path d="M6 4l4 4-4 4" stroke="currentColor" strokeWidth={1.6} strokeLinecap="round" />
              </svg>
            </button>
          </div>
        )}

        {/* ── Verify existing factors ── */}
        {stage === 'verify' && (
          <div style={{
            background: 'var(--bg-surface)', border: '1px solid var(--border)',
            borderRadius: '12px', padding: '40px',
            animation: shake ? 'mfaShake 0.5s ease' : 'none',
          }}>

            <h1 className="is-3ca9302">
              Two-factor verification
            </h1>
            <p className="is-6f544b3">
              {mode === 'totp'
                ? 'Enter the 6-digit code from your authenticator app.'
                : 'Enter one of your saved backup codes.'}
            </p>

            {/* Mode tabs — only show backup tab when a backup factor is enrolled */}
            {availableVerifyModes.length > 1 && (
              <div className="is-c2428bb">
                {availableVerifyModes.map((m) => (
                  <button type="button" key={m} onClick={() => { setMode(m); setError('') }} style={{
                    flex: 1, padding: '6px 12px', borderRadius: '6px', border: 'none',
                    background: mode === m ? 'var(--bg-surface)' : 'transparent',
                    color: mode === m ? 'var(--text)' : 'var(--text-muted)',
                    fontSize: 'var(--fs-sm)', fontWeight: mode === m ? 600 : 400,
                    fontFamily: 'var(--font-body)', cursor: 'pointer',
                    transition: 'all 0.15s ease',
                    boxShadow: mode === m ? 'var(--shadow-xs)' : 'none',
                  }}>
                    {m === 'totp' ? 'Authenticator App' : 'Backup Code'}
                  </button>
                ))}
              </div>
            )}

            <form onSubmit={handleVerify}>
              {mode === 'totp' ? (
                <DigitInputs
                  digits={digits}
                  inputRefs={inputRefs}
                  onChange={handleDigitChange}
                  onKeyDown={handleDigitKeyDown}
                  onPaste={handleDigitPaste}
                />
              ) : (
                <input
                  type="text"
                  aria-label="Backup code"
                  placeholder="xxxx-xxxx-xxxx"
                  value={backupCode}
                  onChange={(e) => { setBackupCode(e.target.value); setError('') }}
                  style={{ width: '100%',
                    letterSpacing: '0.08em', textAlign: 'center',
                    marginBottom: '20px' }}
                />
              )}
              {errorRow}
              {submitBtn('Verify Code', mode === 'totp' ? !totpComplete : backupCode.trim().length === 0)}
            </form>

            {cancelLink}
          </div>
        )}

      </div>
    </div>
  )
}
