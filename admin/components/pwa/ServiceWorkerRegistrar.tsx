'use client'

import { useEffect, useState } from 'react'

const ENABLED =
  process.env.NODE_ENV === 'production' || process.env.NEXT_PUBLIC_ENABLE_SW === 'true'

export function ServiceWorkerRegistrar() {
  const [waiting, setWaiting] = useState<ServiceWorker | null>(null)

  useEffect(() => {
    if (!ENABLED || !('serviceWorker' in navigator)) return
    let reloading = false
    const onControllerChange = () => {
      if (reloading) return
      reloading = true
      window.location.reload()
    }
    navigator.serviceWorker.addEventListener('controllerchange', onControllerChange)

    navigator.serviceWorker
      .register('/sw.js')
      .then((reg) => {
        const check = (w: ServiceWorker | null) => {
          // Only prompt when an older worker is already controlling the page.
          if (w && navigator.serviceWorker.controller) setWaiting(w)
        }
        check(reg.waiting)
        reg.addEventListener('updatefound', () => {
          const w = reg.installing
          w?.addEventListener('statechange', () => {
            if (w.state === 'installed') check(w)
          })
        })
      })
      .catch(() => {})

    return () => navigator.serviceWorker.removeEventListener('controllerchange', onControllerChange)
  }, [])

  if (!waiting) return null
  return (
    <div
      role="status"
      style={{
        position: 'fixed', left: 16, right: 16, bottom: 16, zIndex: 1000,
        maxWidth: 420, margin: '0 auto', display: 'flex', alignItems: 'center', gap: 12,
        padding: '12px 16px', borderRadius: 12, background: 'var(--bg-elevated, #242426)',
        color: 'var(--text, #fff)', border: '1px solid var(--border, rgba(255,255,255,.12))',
        boxShadow: '0 8px 30px rgba(0,0,0,.35)', fontFamily: 'var(--font-body)', fontSize: 'var(--fs-md)',
      }}
    >
      <span style={{ flex: 1 }}>New version available.</span>
      <button
        type="button"
        onClick={() => waiting.postMessage({ type: 'SKIP_WAITING' })}
        style={{
          background: 'var(--accent, #eb5d1e)', color: '#fff', border: 0, borderRadius: 8,
          padding: '8px 16px', fontWeight: 600, cursor: 'pointer', fontFamily: 'inherit',
        }}
      >
        Reload
      </button>
    </div>
  )
}
