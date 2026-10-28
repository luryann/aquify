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
      className="u-m-0-auto u-gap-12px u-p-12px-12px u-ff-font-body u-fs-fs-md" style={{ position: 'fixed', left: 16, right: 16, bottom: 16, zIndex: 900, maxWidth: 420, display: 'flex', alignItems: 'center', borderRadius: 12, background: 'var(--bg-elevated)', color: 'var(--text)', border: '1px solid var(--border)', boxShadow: 'var(--shadow)' }}
    >
      <span style={{ flex: 1 }}>Add Aquify to your home screen.</span>
      <button
        type="button"
        onClick={install}
        className="u-p-8px-16px u-fw-600 u-ff-inherit" style={{ background: 'var(--accent)', color: '#fff', border: 0, borderRadius: 8, cursor: 'pointer' }}
      >
        Install app
      </button>
      <button
        type="button"
        onClick={dismiss}
        aria-label="Dismiss"
        className="u-fs-fs-xl u-lh-1 u-p-4px-6px" style={{ background: 'transparent', border: 0, color: 'var(--text-muted)', cursor: 'pointer' }}
      >
        ×
      </button>
    </div>
  )
}
