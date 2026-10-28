// API client for DARE backend — wraps fetch with auth, error handling, and token refresh
import { useMemo } from 'react'

export class ApiError extends Error {
  public readonly status: number
  public readonly code: string

  constructor(
    status: number,
    code: string,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }

  isUnauthorized()    { return this.status === 401 }
  isForbidden()       { return this.status === 403 }
  isNotFound()        { return this.status === 404 }
  isConflict()        { return this.status === 409 }
  isValidation()      { return this.status === 422 }
  isServerError()     { return this.status >= 500 }

  static async fromResponse(res: Response): Promise<ApiError> {
    return parseResponseError(res)
  }
}

export type ApiRequestOptions = {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  headers?: Record<string, string>
  signal?: AbortSignal
}

async function parseResponseError(res: Response): Promise<ApiError> {
  let code = 'unknown_error'
  let message = `HTTP ${res.status}: ${res.statusText}`
  try {
    const body = await res.json() as {
      code?: unknown
      message?: unknown
      error?: unknown
    }
    const nestedError = typeof body.error === 'object' && body.error !== null
      ? body.error as { code?: unknown; message?: unknown }
      : null

    if (typeof nestedError?.code === 'string') code = nestedError.code
    else if (typeof body.code === 'string') code = body.code

    if (typeof nestedError?.message === 'string') message = nestedError.message
    else if (typeof body.message === 'string') message = body.message
    else if (typeof body.error === 'string') message = body.error
  } catch {
    // Response wasn't JSON — use defaults
  }
  return new ApiError(res.status, code, message)
}

function getCSRFToken(): string | null {
  if (typeof document === 'undefined') return null
  const entry = document.cookie.split('; ').find((c) => c.startsWith('aquify_csrf_token='))
  if (!entry) return null
  const [, value = ''] = entry.split('=')
  return decodeURIComponent(value) || null
}

/**
 * Auth headers for raw `fetch` calls that can't go through apiFetch (multipart
 * uploads, file downloads): bearer token when present, otherwise the CSRF
 * header that cookie-authenticated mutations require.
 */
export function rawRequestAuthHeaders(token: string | undefined, method: string): Record<string, string> {
  if (token) return { Authorization: `Bearer ${token}` }
  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase())) {
    const csrf = getCSRFToken()
    if (csrf) return { 'X-CSRF-Token': csrf }
  }
  return {}
}

// Core fetch function
export async function apiFetch<T = unknown>(
  path: string,
  token: string | undefined,
  options: ApiRequestOptions = {},
): Promise<T> {
  const { method = 'GET', body, headers: extraHeaders = {}, signal } = options

  const headers: Record<string, string> = { ...extraHeaders }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  // When using cookie-based auth (no bearer token), include CSRF token for mutations
  const isMutation = ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)
  if (isMutation && !token) {
    const csrf = getCSRFToken()
    if (csrf) headers['X-CSRF-Token'] = csrf
  }

  const res = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'include',
    signal,
  })

  if (!res.ok) {
    throw await parseResponseError(res)
  }

  // No content
  if (res.status === 204) return undefined as T

  const ct = res.headers.get('content-type') ?? ''
  if (ct.includes('application/json')) {
    return res.json() as Promise<T>
  }
  return res.text() as unknown as T
}

// Typed client factory bound to a token getter
export type ApiClient = ReturnType<typeof createApiClient>

export function createApiClient(getToken: () => string | undefined) {
  const t = () => getToken()

  return {
    get<T = unknown>(path: string, signal?: AbortSignal, headers?: Record<string, string>) {
      return apiFetch<T>(path, t(), { method: 'GET', signal, headers })
    },
    post<T = unknown>(path: string, body?: unknown, headers?: Record<string, string>) {
      return apiFetch<T>(path, t(), { method: 'POST', body, headers })
    },
    put<T = unknown>(path: string, body?: unknown, headers?: Record<string, string>) {
      return apiFetch<T>(path, t(), { method: 'PUT', body, headers })
    },
    patch<T = unknown>(path: string, body?: unknown, headers?: Record<string, string>) {
      return apiFetch<T>(path, t(), { method: 'PATCH', body, headers })
    },
    delete<T = unknown>(path: string, headers?: Record<string, string>) {
      return apiFetch<T>(path, t(), { method: 'DELETE', headers })
    },
  }
}

// Convenience hook-compatible factory — memoized so the returned object is
// referentially stable across renders (safe to use in useEffect deps).
export function useApiClient(token: string | undefined): ApiClient {
  return useMemo(() => createApiClient(() => token), [token])
}

// Pagination helpers
export type PaginatedResponse<T> = {
  data: T[]
  total: number
  page: number
  per_page: number
  cursor?: string
}

export type CursorResponse<T> = {
  data: T[]
  next_cursor?: string
  has_more: boolean
}

// Common query param builder
export function buildQuery(params: Record<string, string | number | boolean | undefined | null>): string {
  const qs = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== '') {
      qs.set(key, String(value))
    }
  }
  const str = qs.toString()
  return str ? `?${str}` : ''
}
