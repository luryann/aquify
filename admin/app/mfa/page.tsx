'use client'

import { useState, useRef, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { getCSRFCookieValue, useSession } from '@/lib/session'
import { destinationForRole } from '@/lib/auth-routing'

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
`

export default function MfaPage() {
  const router = useRouter()
  const { refreshSession, session } = useSession()
  const [mounted, setMounted] = useState(false)
  const [mode, setMode] = useState<'totp' | 'backup'>('totp')
  const [digits, setDigits] = useState<string[]>(Array(6).fill(''))
  const [backupCode, setBackupCode] = useState('')
  const [factorId, setFactorId] = useState('')
  const [totpFactorId, setTotpFactorId] = useState('')
  const [backupFactorId, setBackupFactorId] = useState('')
  const [loading, setLoading] = useState(false)
  const [loadingFactors, setLoadingFactors] = useState(true)
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
    // Focus first input on mount
    setTimeout(() => inputRefs.current[0]?.focus(), 300)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function loadFactors() {
      try {
        const res = await fetch('/api/v1/auth/mfa/factors', {
          method: 'GET',
          credentials: 'include',
          cache: 'no-store',
        })
        if (!res.ok) {
          throw new Error('Unable to load your MFA factors.')
        }
        const payload = await res.json() as {
          factors: Array<{ id: string; factor_type: string; label?: string }>
        }
        const totp = payload.factors.find((item) => item.factor_type === 'totp')?.id ?? ''
        const backup = payload.factors.find((item) => item.factor_type === 'backup_code')?.id ?? ''
        if (!cancelled) {
          setTotpFactorId(totp)
          setBackupFactorId(backup)
          setFactorId(totp || backup)
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Unable to load MFA factors.')
        }
      } finally {
        if (!cancelled) setLoadingFactors(false)
      }
    }

    loadFactors()
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    setFactorId(mode === 'totp' ? totpFactorId : backupFactorId)
  }, [mode, totpFactorId, backupFactorId])

  function handleDigitChange(idx: number, val: string) {
    if (!/^\d?$/.test(val)) return
    const next = [...digits]
    next[idx] = val
    setDigits(next)
    setError('')
    if (val && idx < 5) {
      inputRefs.current[idx + 1]?.focus()
    }
  }

  function handleDigitKeyDown(idx: number, e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Backspace' && !digits[idx] && idx > 0) {
      inputRefs.current[idx - 1]?.focus()
    }
    if (e.key === 'ArrowLeft' && idx > 0) inputRefs.current[idx - 1]?.focus()
    if (e.key === 'ArrowRight' && idx < 5) inputRefs.current[idx + 1]?.focus()
  }

  function handleDigitPaste(e: React.ClipboardEvent) {
    e.preventDefault()
    const text = e.clipboardData.getData('text').replace(/\D/g, '').slice(0, 6)
    if (!text) return
    const next = [...digits]
    for (let i = 0; i < text.length; i++) next[i] = text[i]
    setDigits(next)
    const focusIdx = Math.min(text.length, 5)
    inputRefs.current[focusIdx]?.focus()
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault()
    const code = mode === 'totp' ? digits.join('') : backupCode.trim()
    if (!factorId) {
      setError('No enrolled factor is available for this verification method.')
      triggerShake()
      return
    }
    if ((mode === 'totp' && code.length < 6) || (mode === 'backup' && !code)) {
      setError('Please enter a complete code.')
      triggerShake()
      return
    }
    setLoading(true)
    setError('')
    try {
      const csrf = getCSRFCookieValue()
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
      }
      if (csrf) headers['X-CSRF-Token'] = csrf

      const res = await fetch('/api/v1/auth/mfa/challenge', {
        method: 'POST',
        headers,
        credentials: 'include',
        body: JSON.stringify({
          factor_id: factorId,
          challenge_code: code,
        }),
      })

      if (!res.ok) {
        let message = 'Verification failed.'
        try {
          const payload = await res.json() as { message?: string; error?: string }
          message = payload.message ?? payload.error ?? message
        } catch {
          // Use default message.
        }
        throw new Error(message)
      }

      const next = await refreshSession()
      setLoading(false)
      router.replace(destinationForRole(next?.user.role ?? session?.user.role ?? 'admin'))
    } catch (err) {
      setLoading(false)
      setError(err instanceof Error ? err.message : 'Verification failed.')
      triggerShake()
    }
  }

  function triggerShake() {
    setShake(true)
    setTimeout(() => setShake(false), 600)
  }

  const totpComplete = digits.every((d) => d !== '')

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        background: 'var(--bg)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontFamily: 'var(--font-body)',
        overflow: 'hidden',
      }}
    >
      {/* Geometric background */}
      <div aria-hidden style={{ position: 'absolute', inset: 0, pointerEvents: 'none' }}>
        {/* Grid lines */}
        <div style={{
          position: 'absolute', inset: 0,
          backgroundImage: 'linear-gradient(rgba(255,255,255,0.02) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.02) 1px, transparent 1px)',
          backgroundSize: '48px 48px',
        }} />
        {/* Accent radial */}
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

      <div
        style={{
          position: 'relative',
          width: '100%',
          maxWidth: '420px',
          padding: '0 24px',
          opacity: mounted ? 1 : 0,
          transform: mounted ? 'translateY(0)' : 'translateY(20px)',
          transition: 'opacity 0.4s var(--ease-out), transform 0.4s var(--ease-out)',
        }}
      >
        {/* Card */}
        <div
          style={{
            background: 'var(--bg-surface)',
            border: '1px solid var(--border)',
            borderRadius: '16px',
            padding: '40px',
            boxShadow: '0 24px 64px rgba(0,0,0,0.4), 0 1px 0 rgba(255,255,255,0.04) inset',
            animation: shake ? 'mfaShake 0.5s ease' : 'none',
          }}
        >
          {/* Shield icon */}
          <div
            style={{
              width: '48px',
              height: '48px',
              borderRadius: '12px',
              background: 'var(--accent-subtle)',
              border: '1px solid var(--accent-border)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              marginBottom: '20px',
            }}
          >
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
              stroke="var(--accent)" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
              <path d="M9 12l2 2 4-4" />
            </svg>
          </div>

          <h1
            style={{
              fontFamily: 'var(--font-display)',
              fontSize: '26px',
              fontWeight: 400,
              color: 'var(--text)',
              marginBottom: '6px',
              letterSpacing: '-0.01em',
            }}
          >
            Two-factor verification
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)', marginBottom: '28px', lineHeight: 1.6 }}>
            {mode === 'totp'
              ? 'Enter the 6-digit code from your authenticator app.'
              : 'Enter one of your saved backup codes.'}
          </p>

          {loadingFactors && (
            <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '18px' }}>
              Loading your enrolled verification methods…
            </p>
          )}

          {/* Mode tabs */}
          <div
            style={{
              display: 'flex',
              gap: '4px',
              background: 'var(--bg-elevated)',
              borderRadius: '8px',
              padding: '3px',
              marginBottom: '24px',
            }}
          >
            {(['totp', 'backup'] as const).map((m) => (
              <button
                key={m}
                onClick={() => { setMode(m); setError('') }}
                style={{
                  flex: 1,
                  padding: '6px 12px',
                  borderRadius: '6px',
                  border: 'none',
                  background: mode === m ? 'var(--bg-surface)' : 'transparent',
                  color: mode === m ? 'var(--text)' : 'var(--text-muted)',
                  fontSize: '12px',
                  fontWeight: mode === m ? 600 : 400,
                  fontFamily: 'var(--font-body)',
                  cursor: 'pointer',
                  transition: 'all 0.15s ease',
                  boxShadow: mode === m ? 'var(--shadow-xs)' : 'none',
                }}
              >
                {m === 'totp' ? 'Authenticator App' : 'Backup Code'}
              </button>
            ))}
          </div>

          <form onSubmit={handleVerify}>
            {mode === 'totp' ? (
              /* ── TOTP digit inputs ── */
              <div
                style={{
                  display: 'flex',
                  gap: '8px',
                  justifyContent: 'center',
                  marginBottom: '20px',
                }}
                onPaste={handleDigitPaste}
              >
                {digits.map((d, idx) => (
                  <input
                    key={idx}
                    ref={(el) => { inputRefs.current[idx] = el }}
                    type="text"
                    inputMode="numeric"
                    maxLength={1}
                    value={d}
                    onChange={(e) => handleDigitChange(idx, e.target.value)}
                    onKeyDown={(e) => handleDigitKeyDown(idx, e)}
                    style={{
                      width: '46px',
                      height: '54px',
                      textAlign: 'center',
                      fontSize: '22px',
                      fontFamily: 'var(--font-mono)',
                      fontWeight: 600,
                      background: 'var(--bg-elevated)',
                      border: `1px solid ${d ? 'var(--accent-border)' : 'var(--border)'}`,
                      borderRadius: '10px',
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
            ) : (
              /* ── Backup code input ── */
              <input
                type="text"
                placeholder="xxxx-xxxx-xxxx"
                value={backupCode}
                onChange={(e) => { setBackupCode(e.target.value); setError('') }}
                style={{
                  width: '100%',
                  padding: '12px 14px',
                  background: 'var(--bg-elevated)',
                  border: `1px solid ${error ? 'var(--error-border)' : 'var(--border)'}`,
                  borderRadius: '10px',
                  color: 'var(--text)',
                  fontSize: '16px',
                  fontFamily: 'var(--font-mono)',
                  letterSpacing: '0.08em',
                  textAlign: 'center',
                  outline: 'none',
                  marginBottom: '20px',
                  transition: 'border-color 0.15s ease',
                }}
                onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
              />
            )}

            {/* Error message */}
            {error && (
              <p style={{
                fontSize: '12px', color: 'var(--error)', marginBottom: '12px',
                display: 'flex', alignItems: 'center', gap: '6px',
              }}>
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                  strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                  <circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" />
                  <line x1="12" y1="16" x2="12.01" y2="16" />
                </svg>
                {error}
              </p>
            )}

            {/* Submit button */}
            <button
              type="submit"
              disabled={loading || (mode === 'totp' && !totpComplete)}
              style={{
                width: '100%',
                padding: '12px 20px',
                background: (mode === 'totp' && !totpComplete) || loading
                  ? 'var(--bg-elevated)' : 'var(--accent)',
                border: '1px solid ' + ((mode === 'totp' && !totpComplete) || loading
                  ? 'var(--border)' : 'transparent'),
                borderRadius: '10px',
                color: (mode === 'totp' && !totpComplete) || loading ? 'var(--text-muted)' : '#fff',
                fontSize: '14px',
                fontWeight: 600,
                fontFamily: 'var(--font-body)',
                cursor: ((mode === 'totp' && !totpComplete) || loading) ? 'not-allowed' : 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: '8px',
                transition: 'all 0.15s ease',
                letterSpacing: '-0.01em',
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
              ) : 'Verify Code'}
            </button>
          </form>

          {/* Footer note */}
          <div style={{ marginTop: '20px', textAlign: 'center' }}>
            <button
              onClick={() => router.push('/login')}
              style={{
                background: 'none', border: 'none', cursor: 'pointer',
                fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)',
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
        </div>

        {/* Bottom hint */}
        <p style={{
          textAlign: 'center', marginTop: '16px', fontSize: '11px',
          fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.04em',
        }}>
          MFA required for admin and privileged roles
        </p>
      </div>
    </div>
  )
}
