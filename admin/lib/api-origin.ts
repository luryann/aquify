/**
 * Server-side API origin. The localhost fallback is development/test only;
 * production fails closed so a missing runtime env var is a loud config error
 * rather than silently pointing at localhost.
 */
export function apiInternalOrigin(): string {
  const configured = process.env.API_INTERNAL_URL || process.env.NEXT_PUBLIC_API_URL
  if (configured) return configured
  if (process.env.NODE_ENV === 'production') {
    throw new Error('API_INTERNAL_URL is not configured')
  }
  return 'http://localhost:8080'
}
