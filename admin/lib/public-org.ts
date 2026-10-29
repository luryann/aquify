import { LOCAL_DEV_ORG_ID } from './localDev.ts'

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
  return Object.fromEntries(
    Object.entries(input ?? {}).flatMap(([key, value]) => {
      const normalizedKey = normalizedHost(key)
      const normalizedValue = typeof value === 'string' ? value.trim() : ''
      return normalizedKey && normalizedValue ? [[normalizedKey, normalizedValue]] : []
    }),
  )
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
  const explicitOrgID = normalized(input.searchParams?.get('org_id'))
  if (explicitOrgID) {
    return { orgId: explicitOrgID, clubCode: normalizedCode(input.clubCode), source: 'query_org_id' }
  }

  const queryClubCode = normalizedCode(input.searchParams?.get('club_code'))
  if (queryClubCode) {
    const mappedOrgID = clubCodeOrgMap[queryClubCode.toLowerCase()] ?? ''
    if (mappedOrgID) {
      return { orgId: mappedOrgID, clubCode: queryClubCode, source: 'query_club_code' }
    }
  }

  const formClubCode = normalizedCode(input.clubCode)
  if (formClubCode) {
    const mappedOrgID = clubCodeOrgMap[formClubCode.toLowerCase()] ?? ''
    if (mappedOrgID) {
      return { orgId: mappedOrgID, clubCode: formClubCode, source: 'form_club_code' }
    }
  }

  const clubCode = formClubCode || queryClubCode
  const host = normalizedHost(input.host)
  if (host) {
    const mappedOrgID = hostOrgMap[host] ?? ''
    if (mappedOrgID) {
      return { orgId: mappedOrgID, clubCode, source: 'host' }
    }
  }

  if (isLocalHost(host)) {
    return { orgId: LOCAL_DEV_ORG_ID, clubCode, source: 'local_dev' }
  }

  if (defaultOrgID && !isUnsafeSharedDefaultOrgID(defaultOrgID)) {
    return { orgId: defaultOrgID, clubCode, source: 'default' }
  }

  return { orgId: '', clubCode, source: 'none' }
}
