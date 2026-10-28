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
      className="u-m-0-auto u-gap-12px u-p-12px-16px u-ff-font-body u-fs-fs-md" style={{ position: 'fixed', left: 16, right: 16, bottom: 16, zIndex: 1000, maxWidth: 420, display: 'flex', alignItems: 'center', borderRadius: 12, background: 'var(--bg-elevated, #242426)', color: 'var(--text, #fff)', border: '1px solid var(--border, rgba(255,255,255,.12))', boxShadow: 'var(--shadow)' }}
    >
      <span style={{ flex: 1 }}>New version available.</span>
      <button
        type="button"
        onClick={() => waiting.postMessage({ type: 'SKIP_WAITING' })}
        className="u-p-8px-16px u-fw-600 u-ff-inherit" style={{ background: 'var(--accent, #eb5d1e)', color: '#fff', border: 0, borderRadius: 8, cursor: 'pointer' }}
      >
        Reload
      </button>
    </div>
  )
}
