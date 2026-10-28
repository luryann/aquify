'use client'

import { useState, useRef, useEffect, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { getCSRFCookieValue, useSession } from '@/lib/session'
import { destinationForRole } from '@/lib/auth-routing'
import { resolveMfaBootstrapState, type MfaFactorSummary } from '@/lib/mfa-flow'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

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
    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '24px' }}>
      {[1, 2, 3].map((n) => {
        const done = n < step
        const active = n === step
        return (
          <div key={n} style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
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
    <div style={{
      display: 'flex',
      justifyContent: 'center',
      marginBottom: '20px',
    }}>
      <div style={{
        padding: '12px',
        background: '#ffffff',
        borderRadius: '12px',
        boxShadow: '0 0 0 1px rgba(255,255,255,0.08), 0 8px 24px rgba(0,0,0,0.3)',
        display: 'inline-block',
        lineHeight: 0,
      }}>
        {renderFailed ? (
          <div style={{
            width: '180px',
            minHeight: '180px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            textAlign: 'center',
            color: 'var(--text-muted)',
            fontSize: 'var(--fs-sm)',
            lineHeight: 1.5,
            padding: '16px',
          }}>
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
      style={{ display: 'flex', gap: '8px', justifyContent: 'center', marginBottom: '20px' }}
      onPaste={onPaste}
    >
      {digits.map((d, idx) => (
        <input
          key={idx}
          ref={(el) => { inputRefs.current[idx] = el }}
          type="text"
          inputMode="numeric"
          maxLength={1}
          value={d}
          onChange={(e) => onChange(idx, e.target.value)}
          onKeyDown={(e) => onKeyDown(idx, e)}
          style={{
            width: '46px', height: '54px',
            textAlign: 'center',
            fontSize: 'var(--fs-2xl)',
            fontFamily: 'var(--font-body)',
            fontWeight: 600,
            background: 'var(--bg-elevated)',
            border: `1px solid ${d ? 'var(--accent-border)' : 'var(--border)'}`,
            borderRadius: '12px',
            color: 'var(--text)',
            outline: 'none',
            transition: 'border-color 0.15s ease',
            caretColor: 'var(--accent)',
          }}
          onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
          onBlur={(e) => { e.currentTarget.style.borderColor = d ? 'var(--accent-border)' : 'var(--border)' }}
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
    const csrf = getCSRFCookieValue()
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (csrf) headers['X-CSRF-Token'] = csrf

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
    const csrf = getCSRFCookieValue()
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (csrf) headers['X-CSRF-Token'] = csrf

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
      const csrf = getCSRFCookieValue()
      const headers: Record<string, string> = { 'Content-Type': 'application/json' }
      if (csrf) headers['X-CSRF-Token'] = csrf

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
    <p style={{
      fontSize: 'var(--fs-sm)', color: 'var(--error)', marginBottom: '12px',
      display: 'flex', alignItems: 'center', gap: '6px',
    }}>
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
    <div style={{ marginTop: '20px', textAlign: 'center' }}>
      <button
        onClick={handleCancel}
        style={{
          background: 'none', border: 'none', cursor: 'pointer',
          fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)',
          textDecoration: 'underline', textDecorationColor: 'transparent',
          transition: 'color 0.15s, text-decoration-color 0.15s',
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.color = 'var(--text-subtle)'
          e.currentTarget.style.textDecorationColor = 'var(--text-subtle)'
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.color = 'var(--text-muted)'
          e.currentTarget.style.textDecorationColor = 'transparent'
        }}
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
    <div style={{
      position: 'fixed', inset: 0,
      background: 'var(--bg)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      fontFamily: 'var(--font-body)',
      overflow: 'hidden',
    }}>
      {/* Geometric background */}
      <div aria-hidden style={{ position: 'absolute', inset: 0, pointerEvents: 'none' }}>
        <div style={{
          position: 'absolute', inset: 0,
          backgroundImage: 'linear-gradient(rgba(255,255,255,0.02) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.02) 1px, transparent 1px)',
          backgroundSize: '48px 48px',
        }} />
        <div style={{
          position: 'absolute', top: '-10%', right: '-5%',
          width: '500px', height: '500px', borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(235,93,30,0.08) 0%, transparent 65%)',
        }} />
        <div style={{
          position: 'absolute', bottom: '-15%', left: '-8%',
          width: '400px', height: '400px', borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(235,93,30,0.05) 0%, transparent 65%)',
        }} />
      </div>

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
          <div style={{
            background: 'var(--bg-surface)', border: '1px solid var(--border)',
            borderRadius: '12px', padding: '40px',
            display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '16px',
          }}>
            <div style={{
              width: '12px', height: '12px', borderRadius: '50%',
              background: 'var(--accent)',
              animation: 'mfaPulse 1.4s ease-in-out infinite',
            }} />
            <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', letterSpacing: '0.04em' }}>
              Checking your account…
            </p>
          </div>
        )}

        {/* ── Error (init failed) ── */}
        {stage === 'error' && (
          <div style={{
            background: 'var(--bg-surface)', border: '1px solid var(--border)',
            borderRadius: '12px', padding: '40px',
            display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center', gap: '12px',
          }}>
            <div style={{
              width: '48px', height: '48px', borderRadius: '12px',
              background: 'var(--error-bg)', border: '1px solid var(--error-border)',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              marginBottom: '8px',
            }}>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
                stroke="var(--error)" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="10" />
                <line x1="12" y1="8" x2="12" y2="12" />
                <line x1="12" y1="16" x2="12.01" y2="16" />
              </svg>
            </div>
            <p style={{ fontSize: 'var(--fs-lg)', fontFamily: 'var(--font-display)', color: 'var(--text)', fontWeight: 'var(--display-weight)' }}>
              Something went wrong
            </p>
            <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', lineHeight: 1.6, maxWidth: '300px' }}>
              {error || 'Couldn’t load your two-factor authentication settings. Try again.'}
            </p>
            <button
              onClick={() => { setStage('loading'); setError(''); window.location.reload() }}
              style={{
                marginTop: '8px', padding: '12px 24px',
                background: 'var(--accent)', border: 'none', borderRadius: '8px',
                color: '#fff', fontSize: 'var(--fs-base)', fontWeight: 600,
                fontFamily: 'var(--font-body)', cursor: 'pointer',
              }}
            >
              Try again
            </button>
            {cancelLink}
          </div>
        )}

        {/* ── Setup: Scan QR ── */}
        {stage === 'setup-scan' && (
          <div style={{
            background: 'var(--bg-surface)', border: '1px solid var(--border)',
            borderRadius: '12px', padding: '40px 40px',
          }}>
            <SetupSteps step={1} />

            <h1 style={{
              fontFamily: 'var(--font-display)', fontSize: 'var(--fs-2xl)', fontWeight: 'var(--display-weight)',
              color: 'var(--text)', marginBottom: '6px', letterSpacing: 'var(--display-tracking)',
            }}>
              Set up two-factor auth
            </h1>
            <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', marginBottom: '24px', lineHeight: 1.6 }}>
              Scan this QR code with your authenticator app, then continue.
            </p>

            {provisioningUri ? (
              <QRCanvas uri={provisioningUri} />
            ) : (
              <div style={{
                height: '204px', display: 'flex', alignItems: 'center', justifyContent: 'center',
                marginBottom: '20px',
              }}>
                <div style={{
                  width: '10px', height: '10px', borderRadius: '50%',
                  background: 'var(--accent)', animation: 'mfaPulse 1.4s ease-in-out infinite',
                }} />
              </div>
            )}

            {/* Recommended apps */}
            <div style={{
              display: 'flex', gap: '6px', justifyContent: 'center',
              flexWrap: 'wrap', marginBottom: '20px',
            }}>
              {['Google Authenticator', 'Authy', 'Apple Passwords', 'Bitwarden'].map((app) => (
                <span key={app} style={{
                  fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)',
                  color: 'var(--text-muted)', letterSpacing: '0.03em',
                  background: 'var(--bg-elevated)', border: '1px solid var(--border)',
                  borderRadius: '4px', padding: '2px 8px',
                }}>
                  {app}
                </span>
              ))}
            </div>

            {/* Manual key toggle */}
            <div style={{ marginBottom: '24px' }}>
              <button
                type="button"
                onClick={() => setShowManualKey((v) => !v)}
                style={{
                  background: 'none', border: 'none', cursor: 'pointer',
                  fontSize: 'var(--fs-xs)', color: 'var(--text-muted)',
                  fontFamily: 'var(--font-body)', padding: 0,
                  display: 'flex', alignItems: 'center', gap: '4px',
                  width: '100%', justifyContent: 'center',
                  transition: 'color 0.15s',
                }}
                onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--text-subtle)' }}
                onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--text-muted)' }}
              >
                <svg width="11" height="11" viewBox="0 0 16 16" fill="none"
                  style={{ transform: showManualKey ? 'rotate(180deg)' : 'none', transition: 'transform 0.2s', flexShrink: 0 }}>
                  <path d="M4 6l4 4 4-4" stroke="currentColor" strokeWidth={1.4} strokeLinecap="round" />
                </svg>
                {showManualKey ? 'Hide' : "Can't scan? Enter the key manually"}
              </button>

              {showManualKey && totpSecret && (
                <div style={{
                  marginTop: '12px', padding: '12px 12px',
                  background: 'var(--bg-elevated)',
                  border: '1px solid var(--border)',
                  borderRadius: '8px',
                  display: 'flex', alignItems: 'center', gap: '12px',
                }}>
                  <span style={{
                    flex: 1, fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)',
                    color: 'var(--text)', letterSpacing: '0.12em', wordBreak: 'break-all',
                  }}>
                    {totpSecret.match(/.{1,4}/g)?.join(' ') ?? totpSecret}
                  </span>
                  <CopyButton text={totpSecret} />
                </div>
              )}
            </div>

            <button
              type="button"
              onClick={() => { setDigits(Array(6).fill('')); setError(''); setStage('setup-verify') }}
              style={{
                width: '100%', padding: '12px 20px',
                background: 'var(--accent)', border: 'none',
                borderRadius: '12px', color: '#fff',
                fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)',
                cursor: 'pointer', letterSpacing: '-0.01em',
                display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px',
                transition: 'opacity 0.15s',
              }}
              onMouseEnter={(e) => { e.currentTarget.style.opacity = '0.88' }}
              onMouseLeave={(e) => { e.currentTarget.style.opacity = '1' }}
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

            <h1 style={{
              fontFamily: 'var(--font-display)', fontSize: 'var(--fs-2xl)', fontWeight: 'var(--display-weight)',
              color: 'var(--text)', marginBottom: '6px', letterSpacing: 'var(--display-tracking)',
            }}>
              Confirm your setup
            </h1>
            <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', marginBottom: '24px', lineHeight: 1.6 }}>
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

            <div style={{ marginTop: '16px', textAlign: 'center' }}>
              <button
                type="button"
                onClick={() => { setError(''); setStage('setup-scan') }}
                style={{
                  background: 'none', border: 'none', cursor: 'pointer',
                  fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)',
                  textDecoration: 'underline', textDecorationColor: 'transparent',
                  transition: 'color 0.15s, text-decoration-color 0.15s',
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.color = 'var(--text-subtle)'
                  e.currentTarget.style.textDecorationColor = 'var(--text-subtle)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.color = 'var(--text-muted)'
                  e.currentTarget.style.textDecorationColor = 'transparent'
                }}
              >
                ← Back to QR code
              </button>
            </div>
          </div>
        )}

        {/* ── Setup: Save backup codes ── */}
        {stage === 'setup-backup' && (
          <div style={{
            background: 'var(--bg-surface)', border: '1px solid var(--border)',
            borderRadius: '12px', padding: '40px 40px',
          }}>
            <SetupSteps step={3} />

            <h1 style={{
              fontFamily: 'var(--font-display)', fontSize: 'var(--fs-2xl)', fontWeight: 'var(--display-weight)',
              color: 'var(--text)', marginBottom: '6px', letterSpacing: 'var(--display-tracking)',
            }}>
              Save your backup codes
            </h1>
            <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', marginBottom: '20px', lineHeight: 1.6 }}>
              Store these codes somewhere safe. Each can be used once if you lose access to your authenticator app.
            </p>

            {backupCodes.length > 0 ? (
              <>
                <div style={{
                  background: 'var(--bg-elevated)',
                  border: '1px solid var(--border)',
                  borderRadius: '12px',
                  padding: '16px',
                  marginBottom: '12px',
                  overflow: 'hidden',
                }}>
                  <div style={{
                    display: 'grid',
                    gridTemplateColumns: '1fr 1fr',
                    gap: '8px',
                  }}>
                    {backupCodes.map((code, i) => (
                      <div key={i} style={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        padding: '6px 12px',
                        background: 'var(--bg-surface)',
                        border: '1px solid var(--border)',
                        borderRadius: '6px',
                        gap: '8px',
                        minWidth: 0,
                      }}>
                        <span style={{
                          fontSize: 'var(--fs-sm)',
                          fontFamily: 'var(--font-body)',
                          color: 'var(--text)',
                          letterSpacing: '0.08em',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                          flexShrink: 1,
                        }}>
                          {code}
                        </span>
                        <CopyButton text={code} />
                      </div>
                    ))}
                  </div>
                </div>

                <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '20px' }}>
                  <CopyButton text={backupCodes.join('\n')} />
                </div>
              </>
            ) : (
              <div style={{
                padding: '16px',
                background: 'rgba(235,93,30,0.08)',
                border: '1px solid rgba(235,93,30,0.4)',
                borderRadius: '12px',
                marginBottom: '20px',
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                gap: '12px',
              }}>
                <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', margin: 0, textAlign: 'center', lineHeight: 1.5 }}>
                  Backup codes could not be generated. You must save backup codes before finishing setup.
                </p>
                <button
                  type="button"
                  onClick={retryGenerateBackupCodes}
                  disabled={backupRetrying}
                  style={{
                    padding: '8px 20px',
                    background: 'var(--accent)',
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
                style={{ marginTop: '2px', accentColor: 'var(--accent)', cursor: 'pointer', flexShrink: 0 }}
              />
              <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', lineHeight: 1.5 }}>
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

            <h1 style={{
              fontFamily: 'var(--font-display)', fontSize: 'var(--fs-3xl)', fontWeight: 'var(--display-weight)',
              color: 'var(--text)', marginBottom: '6px', letterSpacing: 'var(--display-tracking)',
            }}>
              Two-factor verification
            </h1>
            <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', marginBottom: '24px', lineHeight: 1.6 }}>
              {mode === 'totp'
                ? 'Enter the 6-digit code from your authenticator app.'
                : 'Enter one of your saved backup codes.'}
            </p>

            {/* Mode tabs — only show backup tab when a backup factor is enrolled */}
            {availableVerifyModes.length > 1 && (
              <div style={{
                display: 'flex', gap: '4px',
                background: 'var(--bg-elevated)', borderRadius: '8px',
                padding: '4px', marginBottom: '24px',
              }}>
                {availableVerifyModes.map((m) => (
                  <button key={m} onClick={() => { setMode(m); setError('') }} style={{
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
                  placeholder="xxxx-xxxx-xxxx"
                  value={backupCode}
                  onChange={(e) => { setBackupCode(e.target.value); setError('') }}
                  style={{
                    width: '100%', padding: '12px 16px',
                    background: 'var(--bg-elevated)',
                    border: `1px solid ${error ? 'var(--error-border)' : 'var(--border)'}`,
                    borderRadius: '12px', color: 'var(--text)',
                    fontSize: 'var(--fs-lg)', fontFamily: 'var(--font-body)',
                    letterSpacing: '0.08em', textAlign: 'center', outline: 'none',
                    marginBottom: '20px', transition: 'border-color 0.15s ease',
                  }}
                  onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                  onBlur={(e) => { e.currentTarget.style.borderColor = error ? 'var(--error-border)' : 'var(--border)' }}
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
