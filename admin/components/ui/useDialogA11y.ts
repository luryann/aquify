'use client'

import { useEffect, type RefObject } from 'react'

/**
 * Escape-to-close, Tab focus trap, initial focus and focus restore for
 * hand-built overlays (drawers, sheets). Modal.tsx covers the centered case.
 */
export function useDialogA11y(ref: RefObject<HTMLElement | null>, onClose: () => void, active = true) {
  useEffect(() => {
    if (!active) return
    const returnTo = document.activeElement as HTMLElement | null
    const focusables = () =>
      Array.from(
        ref.current?.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ) ?? [],
      )
    const t = window.setTimeout(() => {
      const auto = ref.current?.querySelector<HTMLElement>('[data-autofocus]')
      ;(auto ?? focusables().find((el) => el.getAttribute('aria-label') !== 'Close') ?? ref.current)?.focus()
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
      const current = document.activeElement
      if (e.shiftKey && (current === first || !ref.current?.contains(current))) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && (current === last || !ref.current?.contains(current))) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      window.clearTimeout(t)
      returnTo?.focus?.()
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [ref, onClose, active])
}
