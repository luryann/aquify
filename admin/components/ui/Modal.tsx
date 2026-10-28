'use client'

import { useEffect, useRef, type ReactNode } from 'react'
import { Icon } from './icons'

type ModalProps = {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children: ReactNode
  footer?: ReactNode
  width?: number
}

export function Modal({ open, onClose, title, description, children, footer, width = 480 }: ModalProps) {
  const panelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const returnTo = document.activeElement as HTMLElement | null
    const focusables = () =>
      Array.from(
        panelRef.current?.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ) ?? [],
      )
    const t = window.setTimeout(() => {
      const auto = panelRef.current?.querySelector<HTMLElement>('[data-autofocus]')
      ;(auto ?? focusables().find((el) => el.getAttribute('aria-label') !== 'Close') ?? panelRef.current)?.focus()
    }, 0)
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onClose()
        return
      }
      if (e.key !== 'Tab') return
      const els = focusables()
      if (els.length === 0) return
      const first = els[0]
      const last = els[els.length - 1]
      const active = document.activeElement
      if (e.shiftKey && (active === first || !panelRef.current?.contains(active))) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && (active === last || !panelRef.current?.contains(active))) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      window.clearTimeout(t)
      returnTo?.focus?.()
      document.removeEventListener('keydown', onKeyDown)
      document.body.style.overflow = prevOverflow
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      role="presentation"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 1000,
        display: 'flex',
        alignItems: 'flex-start',
        justifyContent: 'center',
        padding: '10vh 20px 40px',
        background: 'var(--bg-overlay)',
        animation: 'modalOverlayIn var(--dur) var(--ease-out)',
      }}
    >
      <div
        ref={panelRef}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        style={{
          width: '100%',
          maxWidth: `${width}px`,
          maxHeight: '78vh',
          display: 'flex',
          flexDirection: 'column',
          background: 'var(--bg-elevated)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--r-xl)',
          boxShadow: 'var(--shadow-lg)',
          animation: 'modalPanelIn var(--dur-slow) var(--ease-out)',
        }}
      >
        <div
          className="u-gap-12px" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', padding: '20px 24px 16px', borderBottom: '1px solid var(--border)' }}
        >
          <div>
            <h2
              id="modal-title"
              className="u-ff-font-display u-fs-fs-xl u-fw-display-weight" style={{ color: 'var(--text)' }}
            >
              {title}
            </h2>
            {description && (
              <p className="u-fs-fs-base u-mt-4px u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>
                {description}
              </p>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="btn-icon u-p-4px"
            style={{ flexShrink: 0, background: 'none', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', borderRadius: 'var(--r-sm)', display: 'flex' }}
          >
            <Icon name="x" size={18} />
          </button>
        </div>

        <div className="u-p-20px-24px" style={{ overflowY: 'auto', flex: 1 }}>{children}</div>

        {footer && (
          <div
            className="u-gap-12px u-p-16px-24px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', borderTop: '1px solid var(--border)' }}
          >
            {footer}
          </div>
        )}
      </div>

      <style>{`
        @keyframes modalOverlayIn {
          from { opacity: 0; }
          to   { opacity: 1; }
        }
        @keyframes modalPanelIn {
          from { opacity: 0; transform: translateY(-10px) scale(0.98); }
          to   { opacity: 1; transform: translateY(0) scale(1); }
        }
        @media (prefers-reduced-motion: reduce) {
          @keyframes modalPanelIn {
            from { opacity: 0; }
            to   { opacity: 1; }
          }
        }
      `}</style>
    </div>
  )
}
