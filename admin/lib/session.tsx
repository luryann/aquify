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

export type User = {
  id: string
  email: string
  name: string
  role: Role
  orgId: string
  avatarInitials?: string
}

// Session holds the bearer token in-memory only — it is NEVER written to localStorage.
//
// Production token storage model:
//   The bearer token is an HttpOnly; Secure; SameSite=Strict cookie set by the backend
//   at POST /api/v1/auth/sessions/issue. The browser attaches it automatically on every
//   fetch() call. The frontend never has direct access to the real token value.
//   Only non-sensitive display metadata (user profile, expiry) is stored in localStorage
//   under `dare:session_meta` to allow the UI to hydrate without a round-trip on reload.
//
// Development stub token model:
//   Stub token IDs (e.g. "stub-owner") are fixture identifiers, not real credentials.
//   They are stored separately under `dare:dev_token` and are only active when
//   AUTH_ENABLE_DEV_STUB_TOKENS is enabled on the server (development only).
export type Session = {
  token: string
  refreshToken?: string
  expiresAt: number
  user: User
}

// Only non-sensitive display metadata is persisted to localStorage in production.
type SessionMeta = {
  user: User
  expiresAt: number
}

type SessionContextValue = {
  session: Session | null
  loading: boolean
  setSession: (s: Session | null) => void
  signOut: () => void
}

const SessionContext = createContext<SessionContextValue | null>(null)

// Production: stores only display metadata (name, role, orgId) — no token.
const META_KEY = 'dare:session_meta'
// Development: stores the stub token ID (a fixture string, not a real credential).
const DEV_TOKEN_KEY = 'dare:dev_token'

// Dev stub sessions — only active when AUTH_ENABLE_DEV_STUB_TOKENS is enabled on the server.
export const DEV_SESSIONS: Record<string, Session> = {
  'stub-owner': {
    token: 'stub-owner',
    expiresAt: Infinity,
    user: { id: 'dev-owner', email: 'owner@dare.local', name: 'Dev Owner', role: 'owner', orgId: 'org-dev', avatarInitials: 'DO' },
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

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSessionState] = useState<Session | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    try {
      if (process.env.NODE_ENV === 'development') {
        // Dev path: restore session from the stub token ID stored in dare:dev_token.
        // Stub IDs are fixture strings — not real credentials.
        const storedId = localStorage.getItem(DEV_TOKEN_KEY)
        if (storedId && DEV_SESSIONS[storedId] && DEV_SESSIONS[storedId].expiresAt > Date.now()) {
          setSessionState(DEV_SESSIONS[storedId])
          setLoading(false)
          return
        }
        // Clean up expired or unknown dev token, then auto-seed default stub.
        localStorage.removeItem(DEV_TOKEN_KEY)
        const devSession = DEV_SESSIONS['stub-owner']
        setSessionState(devSession)
        localStorage.setItem(DEV_TOKEN_KEY, 'stub-owner')
        setLoading(false)
        return
      }

      // Production path: restore only non-sensitive display metadata from localStorage.
      // The bearer token is an HttpOnly cookie — the frontend never reads or writes it.
      const raw = localStorage.getItem(META_KEY)
      if (raw) {
        const meta = JSON.parse(raw) as SessionMeta
        if (meta.expiresAt > Date.now()) {
          // token is intentionally empty string here; the real token is an HttpOnly cookie
          // forwarded automatically by the browser on every API request.
          setSessionState({ token: '', expiresAt: meta.expiresAt, user: meta.user })
        } else {
          localStorage.removeItem(META_KEY)
        }
      }
    } catch {
      localStorage.removeItem(META_KEY)
      localStorage.removeItem(DEV_TOKEN_KEY)
    }
    setLoading(false)
  }, [])

  const setSession = useCallback((s: Session | null) => {
    setSessionState(s)
    if (!s) {
      localStorage.removeItem(META_KEY)
      if (process.env.NODE_ENV === 'development') {
        localStorage.removeItem(DEV_TOKEN_KEY)
      }
      return
    }
    if (process.env.NODE_ENV === 'development') {
      // Store only known stub IDs — never persist arbitrary token strings to localStorage.
      // If a real OIDC token is passed in dev mode, it is NOT persisted (session resets on reload).
      if (s.token in DEV_SESSIONS) {
        localStorage.setItem(DEV_TOKEN_KEY, s.token)
      }
    } else {
      // Store only non-sensitive display metadata; never store the bearer token.
      const meta: SessionMeta = { user: s.user, expiresAt: s.expiresAt }
      localStorage.setItem(META_KEY, JSON.stringify(meta))
    }
  }, [])

  const signOut = useCallback(() => {
    setSession(null)
  }, [setSession])

  const value = useMemo<SessionContextValue>(
    () => ({ session, loading, setSession, signOut }),
    [session, loading, setSession, signOut],
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
