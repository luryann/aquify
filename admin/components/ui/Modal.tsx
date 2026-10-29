'use client'

import { useEffect, useRef, type ReactNode } from 'react'
import { Icon } from './icons'
import { useDialogA11y } from './useDialogA11y'
import './Modal.inline.css'

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
  useDialogA11y(panelRef, onClose, open)

  // Lock page scroll while the dialog is open.
  useEffect(() => {
    if (!open) return
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prevOverflow
    }
  }, [open])

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

    </div>
  )
}
