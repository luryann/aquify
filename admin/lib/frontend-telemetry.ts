import { ApiError } from './api.ts'

export type UIErrorEventDetail = {
  scope: string
  message: string
  status?: number
  code?: string
  meta?: Record<string, unknown>
}

export function uiErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    return error.message
  }
  if (error instanceof Error && error.message.trim()) {
    return error.message
  }
  return fallback
}

export function reportUIError(scope: string, error: unknown, fallback: string, meta?: Record<string, unknown>): string {
  const detail: UIErrorEventDetail = {
    scope,
    message: uiErrorMessage(error, fallback),
    meta,
  }

  if (error instanceof ApiError) {
    detail.status = error.status
    detail.code = error.code
  }

  if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function' && typeof CustomEvent !== 'undefined') {
    window.dispatchEvent(new CustomEvent<UIErrorEventDetail>('aquify:ui-error', { detail }))
  }

  if (typeof console !== 'undefined' && typeof console.error === 'function') {
    console.error(`[ui-error] ${scope}`, detail, error)
  }

  return detail.message
}
