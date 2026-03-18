'use client'

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  useMemo,
  type ReactNode,
} from 'react'
import { Icon } from './icons'

// ─────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────

export type ToastVariant = 'success' | 'error' | 'warning' | 'info'

export type Toast = {
  id: string
  variant: ToastVariant
  title: string
  description?: string
  duration?: number  // ms, 0 = persistent
}

export type ToastOptions = Omit<Toast, 'id'>

type ToastContextValue = {
  toasts: Toast[]
  addToast: (opts: ToastOptions) => string
  removeToast: (id: string) => void
  success: (title: string, description?: string) => void
  error: (title: string, description?: string) => void
  warning: (title: string, description?: string) => void
  info: (title: string, description?: string) => void
}

// ─────────────────────────────────────────────
// Context
// ─────────────────────────────────────────────

const ToastContext = createContext<ToastContextValue | null>(null)

let toastIdCounter = 0

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const addToast = useCallback(
    (opts: ToastOptions): string => {
      const id = `toast-${++toastIdCounter}`
      const toast: Toast = { id, duration: 4500, ...opts }
      setToasts((prev) => [...prev.slice(-4), toast]) // max 5 at once
      return id
    },
    [],
  )

  const shorthand = useCallback(
    (variant: ToastVariant) =>
      (title: string, description?: string) =>
        addToast({ variant, title, description }),
    [addToast],
  )

  const value = useMemo<ToastContextValue>(
    () => ({
      toasts,
      addToast,
      removeToast,
      success: shorthand('success'),
      error:   shorthand('error'),
      warning: shorthand('warning'),
      info:    shorthand('info'),
    }),
    [toasts, addToast, removeToast, shorthand],
  )

  return (
    <ToastContext.Provider value={value}>
      {children}
      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </ToastContext.Provider>
  )
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx
}

// ─────────────────────────────────────────────
// Toast item
// ─────────────────────────────────────────────

const VARIANT_CONFIG = {
  success: { icon: 'check-circle' as const, color: 'var(--success)', bg: 'var(--success-bg)', border: 'var(--success-border)' },
  error:   { icon: 'alert-circle'  as const, color: 'var(--error)',   bg: 'var(--error-bg)',   border: 'var(--error-border)' },
  warning: { icon: 'alert-triangle' as const, color: 'var(--warning)', bg: 'var(--warning-bg)', border: 'var(--warning-border)' },
  info:    { icon: 'info'          as const, color: 'var(--info)',    bg: 'var(--info-bg)',    border: 'var(--info-border)' },
}

function ToastItem({ toast, onRemove }: { toast: Toast; onRemove: (id: string) => void }) {
  const { variant, title, description, duration = 4500, id } = toast
  const cfg = VARIANT_CONFIG[variant]
  const [exiting, setExiting] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const dismiss = useCallback(() => {
    setExiting(true)
    setTimeout(() => onRemove(id), 300)
  }, [id, onRemove])

  useEffect(() => {
    if (duration <= 0) return
    timerRef.current = setTimeout(dismiss, duration)
    return () => { if (timerRef.current) clearTimeout(timerRef.current) }
  }, [dismiss, duration])

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'flex-start',
        gap: '10px',
        background: 'var(--bg-elevated)',
        border: `1px solid ${cfg.border}`,
        borderLeft: `3px solid ${cfg.color}`,
        borderRadius: 'var(--r-lg)',
        padding: '12px 14px',
        boxShadow: 'var(--shadow-toast)',
        width: '320px',
        maxWidth: '90vw',
        position: 'relative',
        overflow: 'hidden',
        animation: exiting
          ? 'toastSlideOut 0.3s var(--ease-in) forwards'
          : 'toastSlideIn 0.35s var(--ease-out) forwards',
      }}
    >
      {/* Icon */}
      <div style={{ color: cfg.color, flexShrink: 0, marginTop: '1px' }}>
        <Icon name={cfg.icon} size={16} />
      </div>

      {/* Content */}
      <div style={{ flex: 1, minWidth: 0 }}>
        <p style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', lineHeight: 1.4 }}>
          {title}
        </p>
        {description && (
          <p style={{ fontSize: '12px', color: 'var(--text-subtle)', marginTop: '2px', lineHeight: 1.5 }}>
            {description}
          </p>
        )}
      </div>

      {/* Dismiss */}
      <button
        onClick={dismiss}
        aria-label="Dismiss"
        style={{
          flexShrink: 0,
          background: 'none',
          border: 'none',
          padding: '2px',
          color: 'var(--text-muted)',
          cursor: 'pointer',
          borderRadius: 'var(--r-xs)',
          display: 'flex',
          alignItems: 'center',
          transition: 'color var(--dur-fast)',
          marginTop: '1px',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--text)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}
      >
        <Icon name="x" size={14} />
      </button>

      {/* Progress bar */}
      {duration > 0 && (
        <div
          style={{
            position: 'absolute',
            bottom: 0,
            left: 0,
            right: 0,
            height: '2px',
            background: cfg.color,
            opacity: 0.35,
            transformOrigin: 'left center',
            animation: `toastProgress ${duration}ms linear forwards`,
          }}
        />
      )}

      <style>{`
        @keyframes toastSlideIn {
          from { opacity: 0; transform: translateX(24px); }
          to   { opacity: 1; transform: translateX(0); }
        }
        @keyframes toastSlideOut {
          from { opacity: 1; transform: translateX(0); max-height: 200px; margin-bottom: 8px; }
          to   { opacity: 0; transform: translateX(24px); max-height: 0; margin-bottom: 0; padding-top: 0; padding-bottom: 0; }
        }
        @keyframes toastProgress {
          from { transform: scaleX(1); }
          to   { transform: scaleX(0); }
        }
      `}</style>
    </div>
  )
}

// ─────────────────────────────────────────────
// Container
// ─────────────────────────────────────────────

function ToastContainer({
  toasts,
  onRemove,
}: {
  toasts: Toast[]
  onRemove: (id: string) => void
}) {
  if (toasts.length === 0) return null

  return (
    <div
      aria-live="polite"
      aria-label="Notifications"
      style={{
        position: 'fixed',
        bottom: '24px',
        right: '24px',
        zIndex: 9999,
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
        pointerEvents: 'none',
      }}
    >
      {toasts.map((t) => (
        <div key={t.id} style={{ pointerEvents: 'auto' }}>
          <ToastItem toast={t} onRemove={onRemove} />
        </div>
      ))}
    </div>
  )
}
