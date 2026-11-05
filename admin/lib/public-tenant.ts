// Resolves which club a public page (sign-up, sign-in) belongs to without ever
// handling an org id in the browser. The server owns that mapping: the club
// slug comes from the club's "Join" link (?club=) or from a verified custom
// domain, and POST /register resolves the slug to the org itself.

export type TenantFetch = (input: string, init?: RequestInit) => Promise<Pick<Response, 'ok' | 'json'>>

type SearchParamsLike = Pick<URLSearchParams, 'get'>

const SLUG_RE = /^[a-z0-9-]{3,40}$/

export function normalizeClubSlug(raw: string | null | undefined): string {
  const slug = (raw ?? '').trim().toLowerCase()
  return SLUG_RE.test(slug) ? slug : ''
}

/**
 * Returns the club slug for this page, or '' when none can be determined.
 * Order: explicit ?club= link, then the host's verified custom domain.
 */
export async function resolveClubSlug(input: {
  searchParams?: SearchParamsLike | null
  host?: string | null
  fetchImpl?: TenantFetch
}): Promise<string> {
  const fromLink = normalizeClubSlug(input.searchParams?.get('club'))
  if (fromLink) return fromLink

  const host = (input.host ?? '').trim().toLowerCase()
  if (!host || host === 'localhost' || /^[\d.:[\]]+$/.test(host)) return ''
  const doFetch: TenantFetch = input.fetchImpl ?? ((url, init) => fetch(url, init))
  try {
    const res = await doFetch(`/api/v1/public/tenant?host=${encodeURIComponent(host)}`, { headers: { Accept: 'application/json' } })
    if (!res.ok) return ''
    const body = (await res.json()) as { slug?: unknown }
    return typeof body.slug === 'string' ? normalizeClubSlug(body.slug) : ''
  } catch {
    return ''
  }
}
