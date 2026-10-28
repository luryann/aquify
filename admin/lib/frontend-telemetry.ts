import { ApiError } from './api.ts'

export type UIErrorEventDetail = {
  scope: string
  message: string
  status?: number
  code?: string
  meta?: Record<string, unknown>
}

export const NETWORK_ERROR_MESSAGE = "Can't reach the server. Check your connection and try again."
export const SESSION_EXPIRED_MESSAGE = 'Your session has expired. Sign in again to continue.'
export const PERMISSION_DENIED_MESSAGE = "You don't have permission to do this."

// Backend text that names fields, headers, or internals rather than describing
// the problem to a person. Anything matching falls back to the caller's copy.
const TECHNICAL_MESSAGE = /\b[a-z]+_[a-z_]+\b|json|request body|header|x-org-id|csrf|context|scope|null|undefined|panic|stack|^http \d{3}|insufficient (role|permissions)|service is not available|not configured|failed to (resolve|persist|build|parse)|\bID (and [\w ]+ )?(is|are) required/i

function isNetworkFailure(error: unknown): boolean {
  return error instanceof TypeError && /fetch|network|load failed/i.test(error.message)
}

/**
 * The one way the UI turns a thrown error into a sentence. Human messages from
 * the API (validation, conflicts, not found) pass through; auth, permission,
 * network, rate-limit, and server failures get consistent copy; anything that
 * reads like internals falls back to the caller's `fallback`.
 */
export function uiErrorMessage(error: unknown, fallback: string): string {
  if (isNetworkFailure(error)) return NETWORK_ERROR_MESSAGE
  if (error instanceof ApiError) {
    if (error.status === 401) return SESSION_EXPIRED_MESSAGE
    if (error.status === 429) return 'Too many requests. Wait a moment, then try again.'
    if (error.status >= 500) return fallback
    const message = error.message.trim()
    if (error.status === 403 && (!message || TECHNICAL_MESSAGE.test(message) || /^forbidden\.?$/i.test(message))) {
      return PERMISSION_DENIED_MESSAGE
    }
    if (!message || TECHNICAL_MESSAGE.test(message)) return fallback
    return message
  }
  if (error instanceof Error) {
    const message = error.message.trim()
    if (message && !TECHNICAL_MESSAGE.test(message)) return message
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
