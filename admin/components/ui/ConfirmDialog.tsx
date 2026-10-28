'use client'

import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { Modal } from './Modal'

export type ConfirmOptions = {
  title: string
  /** Plain-language consequence of the action. */
  message: string
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
  /** Number of items affected; shown in the body. */
  count?: number
  countNoun?: string
  /** If set, the user must type this phrase to enable the confirm button. */
  typedPhrase?: string
  /** If set, shows a textarea; confirm is returned via useConfirmReason. */
  reasonLabel?: string
  reasonRequired?: boolean
}

type DialogProps = ConfirmOptions & {
  open: boolean
  onConfirm: (reason?: string) => void
  onCancel: () => void
}

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  destructive,
  count,
  countNoun = 'item',
  typedPhrase,
  reasonLabel,
  reasonRequired,
  onConfirm,
  onCancel,
}: DialogProps) {
  const [typed, setTyped] = useState('')
  const [reason, setReason] = useState('')
  const ok = (!typedPhrase || typed.trim() === typedPhrase) && (!reasonLabel || !reasonRequired || reason.trim().length > 0)
  return (
    <Modal
      open={open}
      onClose={onCancel}
      title={title}
      width={440}
      footer={
        <>
          <button type="button" className="btn btn-secondary" onClick={onCancel} data-autofocus={!typedPhrase ? '' : undefined}>
            {cancelLabel}
          </button>
          <button
            type="button"
            className={`btn ${destructive ? 'btn-danger' : 'btn-primary'}`}
            disabled={!ok}
            onClick={() => {
              const r = reason.trim()
              setTyped('')
              setReason('')
              onConfirm(r)
            }}
            style={destructive ? { background: 'var(--danger, #b42318)', color: '#fff', borderColor: 'transparent' } : undefined}
          >
            {confirmLabel}
          </button>
        </>
      }
    >
      <div className="stack">
        <p className="u-fs-fs-base u-lh-1_5" style={{ color: 'var(--text)' }}>{message}</p>
        {typeof count === 'number' && (
          <p className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
            This affects {count} {countNoun}
            {count === 1 ? '' : 's'}.
          </p>
        )}
        {reasonLabel && (
          <div>
            <label htmlFor="confirm-reason" className="form-label">
              {reasonLabel}{reasonRequired ? '' : ' (optional)'}
            </label>
            <textarea
              id="confirm-reason"
              className="form-input"
              rows={3}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              data-autofocus=""
              style={{ width: '100%' }}
            />
          </div>
        )}
        {typedPhrase && (
          <div>
            <label htmlFor="confirm-typed" className="form-label">
              Type {typedPhrase} to continue
            </label>
            <input
              id="confirm-typed"
              className="form-input"
              autoComplete="off"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              data-autofocus=""
              style={{ width: '100%' }}
            />
          </div>
        )}
      </div>
    </Modal>
  )
}

type Ctx = (opts: ConfirmOptions) => Promise<boolean>
type ReasonCtx = (opts: ConfirmOptions) => Promise<string | null>
const ConfirmContext = createContext<{ confirm: Ctx; confirmReason: ReasonCtx } | null>(null)

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [opts, setOpts] = useState<ConfirmOptions | null>(null)
  const resolver = useRef<((v: string | null) => void) | null>(null)

  const confirmReason = useCallback<ReasonCtx>((o) => {
    return new Promise<string | null>((resolve) => {
      resolver.current?.(null)
      resolver.current = resolve
      setOpts(o)
    })
  }, [])
  const confirm = useCallback<Ctx>((o) => confirmReason(o).then((v) => v !== null), [confirmReason])
  const value = useMemo(() => ({ confirm, confirmReason }), [confirm, confirmReason])

  const finish = (v: string | null) => {
    resolver.current?.(v)
    resolver.current = null
    setOpts(null)
  }

  return (
    <ConfirmContext.Provider value={value}>
      {children}
      {opts && <ConfirmDialog {...opts} open onConfirm={(r) => finish(r ?? '')} onCancel={() => finish(null)} />}
    </ConfirmContext.Provider>
  )
}

/** Returns an async confirm(): `if (!(await confirm({...}))) return`. */
export function useConfirm(): Ctx {
  const ctx = useContext(ConfirmContext)
  if (!ctx) throw new Error('useConfirm must be used inside ConfirmProvider')
  return ctx.confirm
}

/** Like useConfirm but resolves to the entered reason (may be '' if optional), or null if cancelled. */
export function useConfirmReason(): ReasonCtx {
  const ctx = useContext(ConfirmContext)
  if (!ctx) throw new Error('useConfirmReason must be used inside ConfirmProvider')
  return ctx.confirmReason
}
