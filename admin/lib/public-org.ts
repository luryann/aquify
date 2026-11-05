import { LOCAL_DEV_ORG_ID } from './localDev.ts'

// Local-dev org resolution (localhost -> demo org) is disabled in production builds.
const IS_PRODUCTION_BUILD = process.env.NEXT_PUBLIC_APP_ENV?.trim() === 'production'
const DEFAULT_OIDC_ORG_ID = process.env.NEXT_PUBLIC_DEFAULT_OIDC_ORG_ID?.trim() ?? ''

function parseStringMap(raw: string | undefined): Record<string, string> {
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return normalizedMap(parsed as Record<string, unknown>)
  } catch {
    return {}
  }
}

const CLUB_CODE_ORG_MAP = parseStringMap(process.env.NEXT_PUBLIC_CLUB_CODE_ORG_MAP)
const HOST_ORG_MAP = parseStringMap(process.env.NEXT_PUBLIC_HOST_ORG_MAP)

export type PublicOrgResolutionSource =
  | 'query_org_id'
  | 'query_club_code'
  | 'form_club_code'
  | 'host'
  | 'default'
  | 'local_dev'
  | 'none'

export type PublicOrgResolution = {
  orgId: string
  clubCode: string
  source: PublicOrgResolutionSource
}

type SearchParamsLike = Pick<URLSearchParams, 'get'>
type StringMap = Record<string, string>

type PublicOrgResolverConfig = {
  clubCodeOrgMap?: StringMap | null
  hostOrgMap?: StringMap | null
  defaultOrgId?: string | null
  /** Overrides NEXT_PUBLIC_APP_ENV === 'production' (tests). */
  production?: boolean | null
}

function normalized(input: string | null | undefined): string {
  return input?.trim() ?? ''
}

function normalizedCode(input: string | null | undefined): string {
  return normalized(input).toUpperCase()
}

function normalizedHost(input: string | null | undefined): string {
  return normalized(input).toLowerCase()
}

function isLocalHost(host: string): boolean {
  return host === 'localhost' || host === '127.0.0.1' || host === '::1'
}

/** Lower-cases and trims keys, trims string values, and drops entries that end up empty. */
function normalizedMap(input: Record<string, unknown> | null | undefined): StringMap {
  // Null-prototype so lookups of keys like 'constructor' or '__proto__' never hit inherited members.
  const out: StringMap = Object.create(null)
  for (const [key, value] of Object.entries(input ?? {})) {
    const normalizedKey = normalizedHost(key)
    const normalizedValue = typeof value === 'string' ? value.trim() : ''
    if (normalizedKey && normalizedValue) out[normalizedKey] = normalizedValue
  }
  return out
}

function own(map: StringMap, key: string): string {
  return Object.hasOwn(map, key) ? map[key] : ''
}

function isUnsafeSharedDefaultOrgID(orgID: string): boolean {
  const normalizedOrgID = normalizedHost(orgID)
  return normalizedOrgID === 'org-demo' || normalizedOrgID === normalizedHost(LOCAL_DEV_ORG_ID)
}

export function resolvePublicOrg(input: {
  searchParams?: SearchParamsLike | null
  host?: string | null
  clubCode?: string | null
  config?: PublicOrgResolverConfig | null
} = {}): PublicOrgResolution {
  const clubCodeOrgMap = normalizedMap(input.config?.clubCodeOrgMap ?? CLUB_CODE_ORG_MAP)
  const hostOrgMap = normalizedMap(input.config?.hostOrgMap ?? HOST_ORG_MAP)
  const defaultOrgID = normalized(input.config?.defaultOrgId ?? DEFAULT_OIDC_ORG_ID)
  const production = input.config?.production ?? IS_PRODUCTION_BUILD
  // A raw ?org_id= from the address bar is a dev convenience only. In production
  // the tenant is resolved by the server from the club slug or a verified domain.
  const explicitOrgID = production ? '' : normalized(input.searchParams?.get('org_id'))
  if (explicitOrgID) {
    return { orgId: explicitOrgID, clubCode: normalizedCode(input.clubCode), source: 'query_org_id' }
  }

  const queryClubCode = normalizedCode(input.searchParams?.get('club_code'))
  if (queryClubCode) {
    const mappedOrgID = own(clubCodeOrgMap, queryClubCode.toLowerCase())
    if (mappedOrgID) {
      return { orgId: mappedOrgID, clubCode: queryClubCode, source: 'query_club_code' }
    }
  }

  const formClubCode = normalizedCode(input.clubCode)
  if (formClubCode) {
    const mappedOrgID = own(clubCodeOrgMap, formClubCode.toLowerCase())
    if (mappedOrgID) {
      return { orgId: mappedOrgID, clubCode: formClubCode, source: 'form_club_code' }
    }
  }

  const clubCode = formClubCode || queryClubCode
  const host = normalizedHost(input.host)
  if (host) {
    const mappedOrgID = own(hostOrgMap, host)
    if (mappedOrgID) {
      return { orgId: mappedOrgID, clubCode, source: 'host' }
    }
  }

  if (isLocalHost(host) && !(input.config?.production ?? IS_PRODUCTION_BUILD)) {
    return { orgId: LOCAL_DEV_ORG_ID, clubCode, source: 'local_dev' }
  }

  if (defaultOrgID && !isUnsafeSharedDefaultOrgID(defaultOrgID)) {
    return { orgId: defaultOrgID, clubCode, source: 'default' }
  }

  return { orgId: '', clubCode, source: 'none' }
}
