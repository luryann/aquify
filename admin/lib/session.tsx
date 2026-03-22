'use client'

import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  useMemo,
  type ReactNode,
} from 'react'
import type { Role } from './roles'
import { ROLE_HIERARCHY } from './roles'
import { DEV_STUB_TOKENS_ENABLED } from './featureFlags'

export type User = {
  id: string
  email: string
  name: string
  role: Role
  orgId: string
  householdId?: string
  mfaAuthenticated?: boolean
  avatarInitials?: string
}

export type Session = {
  token: string
  refreshToken?: string
  expiresAt: number
  user: User
}

type SessionContextValue = {
  session: Session | null
  loading: boolean
  setSession: (s: Session | null) => void
  signOut: () => void
  refreshSession: () => Promise<Session | null>
}

type CurrentSessionResponse = {
  status: string
  user: {
    user_id: string
    email?: string
    name?: string
    org_id: string
    household_id?: string
    roles: string[]
    mfa_authenticated?: boolean
  }
}

const SessionContext = createContext<SessionContextValue | null>(null)
const DEV_TOKEN_KEY = 'dare:dev_token'

export const DEV_SESSIONS: Record<string, Session> = {
  'stub-owner': {
    token: 'stub-owner',
    expiresAt: Infinity,
    user: { id: 'dev-owner', email: 'owner@dare.local', name: 'Dev Owner', role: 'owner', orgId: 'org-dev', avatarInitials: 'DO', mfaAuthenticated: true },
  },
  'stub-admin': {
    token: 'stub-admin',
    expiresAt: Infinity,
    user: { id: 'dev-admin', email: 'admin@dare.local', name: 'Dev Admin', role: 'admin', orgId: 'org-dev', avatarInitials: 'DA' },
  },
  'stub-coach': {
    token: 'stub-coach',
    expiresAt: Infinity,
    user: { id: 'dev-coach', email: 'coach@dare.local', name: 'Dev Coach', role: 'coach', orgId: 'org-dev', avatarInitials: 'DC' },
  },
  'stub-parent': {
    token: 'stub-parent',
    expiresAt: Infinity,
    user: { id: 'dev-parent', email: 'parent@dare.local', name: 'Dev Parent', role: 'parent', orgId: 'org-dev', avatarInitials: 'DP' },
  },
  'stub-billing': {
    token: 'stub-billing',
    expiresAt: Infinity,
    user: { id: 'dev-billing', email: 'billing@dare.local', name: 'Dev Billing', role: 'billing', orgId: 'org-dev', avatarInitials: 'DB' },
  },
  'stub-readonly': {
    token: 'stub-readonly',
    expiresAt: Infinity,
    user: { id: 'dev-readonly', email: 'readonly@dare.local', name: 'Dev Read-Only', role: 'read-only', orgId: 'org-dev', avatarInitials: 'DR' },
  },
  'stub-swimmer': {
    token: 'stub-swimmer',
    expiresAt: Infinity,
    user: { id: 'dev-swimmer', email: 'swimmer@dare.local', name: 'Emma Chen', role: 'swimmer', orgId: 'org-dev', avatarInitials: 'EC' },
  },
}

function deriveInitials(name: string, email: string, userID: string): string {
  const source = name || email || userID
  const parts = source.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return `${parts[0][0] ?? ''}${parts[1][0] ?? ''}`.toUpperCase()
  return source.slice(0, 2).toUpperCase() || 'DA'
}

function resolvePrimaryRole(roles: string[]): Role {
  const knownRoles = roles.filter((role): role is Role => role in ROLE_HIERARCHY)
  if (knownRoles.length === 0) return 'parent'
  return [...knownRoles].sort((a, b) => ROLE_HIERARCHY[b] - ROLE_HIERARCHY[a])[0]
}

function sessionFromCurrentResponse(payload: CurrentSessionResponse): Session {
  const role = resolvePrimaryRole(payload.user.roles)
  const name = payload.user.name?.trim() || payload.user.email?.trim() || payload.user.user_id
  const email = payload.user.email?.trim() || ''
  return {
    token: '',
    expiresAt: Date.now() + 15 * 60 * 1000,
    user: {
      id: payload.user.user_id,
      email,
      name,
      role,
      orgId: payload.user.org_id,
      householdId: payload.user.household_id,
      mfaAuthenticated: payload.user.mfa_authenticated,
      avatarInitials: deriveInitials(name, email, payload.user.user_id),
    },
  }
}

export function getCSRFCookieValue(): string | null {
  if (typeof document === 'undefined') return null
  const cookie = document.cookie
    .split('; ')
    .find((entry) => entry.startsWith('aquify_csrf_token='))
  if (!cookie) return null
  const [, value = ''] = cookie.split('=')
  return decodeURIComponent(value) || null
}

async function attemptRefresh(): Promise<boolean> {
  const csrf = getCSRFCookieValue()
  if (!csrf) return false

  const res = await fetch('/api/v1/auth/sessions/refresh', {
    method: 'POST',
    headers: {
      'X-CSRF-Token': csrf,
    },
    credentials: 'include',
  })

  return res.ok
}

async function fetchCurrentSession(allowRefresh: boolean): Promise<Session | null> {
  const res = await fetch('/api/v1/auth/session', {
    method: 'GET',
    credentials: 'include',
    cache: 'no-store',
  })

  if (res.ok) {
    return sessionFromCurrentResponse(await res.json() as CurrentSessionResponse)
  }

  if (res.status === 401 && allowRefresh && await attemptRefresh()) {
    return fetchCurrentSession(false)
  }

  if (res.status === 401 || res.status === 403) {
    return null
  }

  throw new Error(`session bootstrap failed with status ${res.status}`)
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSessionState] = useState<Session | null>(null)
  const [loading, setLoading] = useState(true)

  const setSession = useCallback((s: Session | null) => {
    setSessionState(s)
    if (typeof window === 'undefined') return

    if (!DEV_STUB_TOKENS_ENABLED) {
      localStorage.removeItem(DEV_TOKEN_KEY)
      return
    }

    if (s && s.token in DEV_SESSIONS) {
      localStorage.setItem(DEV_TOKEN_KEY, s.token)
      return
    }

    localStorage.removeItem(DEV_TOKEN_KEY)
  }, [])

  const signOut = useCallback(() => {
    setSessionState(null)
    if (typeof window !== 'undefined') {
      localStorage.removeItem(DEV_TOKEN_KEY)
    }
  }, [])

  const refreshSession = useCallback(async () => {
    try {
      const next = await fetchCurrentSession(true)
      setSessionState(next)
      return next
    } catch {
      setSessionState(null)
      return null
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    async function bootstrap() {
      try {
        if (DEV_STUB_TOKENS_ENABLED) {
          const storedId = localStorage.getItem(DEV_TOKEN_KEY)
          if (storedId && DEV_SESSIONS[storedId]) {
            if (!cancelled) setSessionState(DEV_SESSIONS[storedId])
            return
          }
        }

        const next = await fetchCurrentSession(true)
        if (!cancelled) setSessionState(next)
      } catch {
        if (!cancelled) setSessionState(null)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    bootstrap()
    return () => {
      cancelled = true
    }
  }, [])

  const value = useMemo<SessionContextValue>(
    () => ({ session, loading, setSession, signOut, refreshSession }),
    [session, loading, setSession, signOut, refreshSession],
  )

  return (
    <SessionContext.Provider value={value}>
      {children}
    </SessionContext.Provider>
  )
}

export function useSession(): SessionContextValue {
  const ctx = useContext(SessionContext)
  if (!ctx) throw new Error('useSession must be used within SessionProvider')
  return ctx
}

export function useRequiredSession(): Session {
  const { session } = useSession()
  if (!session) throw new Error('No active session')
  return session
}
