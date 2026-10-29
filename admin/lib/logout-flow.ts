// Shared sign-out helpers for /logout and the MFA cancel path.
//
// The logout endpoint returns an IdP `logout_url` (ends the Keycloak SSO session)
// and a `redirect_url`. Both come from the server, so they are validated before
// the browser navigates to them.

export type LogoutTarget = {
  /** Where to send the browser next. */
  target: string
  /** True when `target` is an absolute (IdP) URL that needs a full navigation. */
  topLevel: boolean
}

type SafeTargetOptions = {
  /** Current page origin; same-origin absolute URLs are always allowed. */
  origin?: string
  /** Optional comma-separated allowlist of IdP origins (NEXT_PUBLIC_IDP_ORIGINS). */
  allowedOrigins?: string
}

const LOCAL_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]'])

function allowlist(raw?: string): string[] {
  return (raw ?? '').split(',').map((o) => o.trim().replace(/\/+$/, '')).filter(Boolean)
}

/**
 * Accepts an app-relative path or an https (http only on localhost) URL whose
 * origin is the current origin or on the configured IdP allowlist; anything
 * else (javascript:, data:, protocol-relative, unknown hosts) falls back.
 */
export function safeRedirectTarget(raw: string | undefined | null, opts: SafeTargetOptions = {}, fallback = '/login'): LogoutTarget {
  const value = (raw ?? '').trim()
  const safeFallback: LogoutTarget = { target: fallback, topLevel: false }
  if (!value) return safeFallback
  if (value.startsWith('/')) {
    return value.startsWith('//') || value.startsWith('/\\') ? safeFallback : { target: value, topLevel: false }
  }
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return safeFallback
  }
  const secure = url.protocol === 'https:' || (url.protocol === 'http:' && LOCAL_HOSTS.has(url.hostname))
  if (!secure) return safeFallback
  if (opts.origin && url.origin === opts.origin) return { target: value, topLevel: true }
  const allowed = allowlist(opts.allowedOrigins)
  if (allowed.length > 0 && !allowed.includes(url.origin)) return safeFallback
  return { target: value, topLevel: true }
}

type LogoutPayload = {
  logout_url?: string
  redirect_url?: string
  message?: string
  error?: string | { message?: string }
}

/**
 * Revokes the cookie-backed session (sending the CSRF header cookie-auth
 * mutations require) and resolves where to go next. Throws an Error carrying the
 * server's message when the request is rejected.
 */
export async function requestLogout(
  deps: { fetchImpl?: typeof fetch; csrfToken?: string | null; origin?: string; allowedOrigins?: string } = {},
): Promise<LogoutTarget> {
  const doFetch = deps.fetchImpl ?? fetch
  const headers: Record<string, string> = {}
  if (deps.csrfToken) headers['X-CSRF-Token'] = deps.csrfToken
  const res = await doFetch('/api/v1/auth/sessions/logout', { method: 'POST', credentials: 'include', headers })

  if (!res.ok) {
    let message = 'Couldn’t revoke your session. Try again.'
    try {
      const payload = (await res.json()) as LogoutPayload
      const errField = payload.error
      message = payload.message ?? (typeof errField === 'string' ? errField : errField?.message) ?? message
    } catch {
      // Keep the default message.
    }
    throw new Error(message)
  }

  const payload = (await res.json()) as LogoutPayload
  const opts = { origin: deps.origin, allowedOrigins: deps.allowedOrigins }
  const idp = payload.logout_url?.trim() ? safeRedirectTarget(payload.logout_url, opts, '') : null
  if (idp && idp.target) return idp
  return safeRedirectTarget(payload.redirect_url, opts)
}
