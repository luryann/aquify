'use client'

import { useEffect, useState } from 'react'
import { rememberInstallDismissed, safeLocalStorage, shouldOfferInstall } from '@/lib/install-prompt'

type BeforeInstallPromptEvent = Event & {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

const SESSION_KEY = 'aquify.install-prompt.shown'

export function InstallPrompt() {
  const [evt, setEvt] = useState<BeforeInstallPromptEvent | null>(null)

  useEffect(() => {
    // "Home screen" only means something on touch devices, and the banner
    // shows at most once per browser session, to returning users only.
    if (!window.matchMedia('(pointer: coarse)').matches) return
    try {
      if (sessionStorage.getItem(SESSION_KEY)) return
    } catch { /* storage unavailable: fall through to the persistent checks */ }
    if (!shouldOfferInstall(safeLocalStorage())) return
    const onPrompt = (e: Event) => {
      e.preventDefault()
      try { sessionStorage.setItem(SESSION_KEY, '1') } catch { /* ignore */ }
      setEvt(e as BeforeInstallPromptEvent)
    }
    const onInstalled = () => setEvt(null)
    window.addEventListener('beforeinstallprompt', onPrompt)
    window.addEventListener('appinstalled', onInstalled)
    return () => {
      window.removeEventListener('beforeinstallprompt', onPrompt)
      window.removeEventListener('appinstalled', onInstalled)
    }
  }, [])

  if (!evt) return null

  const dismiss = () => {
    rememberInstallDismissed(safeLocalStorage())
    setEvt(null)
  }
  const install = async () => {
    try {
      await evt.prompt()
      await evt.userChoice
    } catch {
      /* ignore */
    }
    dismiss()
  }

  return (
    <div
      role="region"
      aria-label="Install app"
      style={{
        position: 'fixed', left: 16, right: 16, bottom: 16, zIndex: 900, maxWidth: 420,
        margin: '0 auto', display: 'flex', alignItems: 'center', gap: 12, padding: '12px 12px',
        borderRadius: 12, background: 'var(--bg-elevated)', color: 'var(--text)',
        border: '1px solid var(--border)', boxShadow: '0 8px 30px rgba(0,0,0,.18)',
        fontFamily: 'var(--font-body)', fontSize: 'var(--fs-md)',
      }}
    >
      <span style={{ flex: 1 }}>Add Aquify to your home screen.</span>
      <button
        type="button"
        onClick={install}
        style={{
          background: 'var(--accent)', color: '#fff', border: 0, borderRadius: 8,
          padding: '8px 16px', fontWeight: 600, cursor: 'pointer', fontFamily: 'inherit',
        }}
      >
        Install app
      </button>
      <button
        type="button"
        onClick={dismiss}
        aria-label="Dismiss"
        style={{
          background: 'transparent', border: 0, color: 'var(--text-muted)', fontSize: 'var(--fs-xl)',
          lineHeight: 1, cursor: 'pointer', padding: '4px 6px',
        }}
      >
        ×
      </button>
    </div>
  )
}
