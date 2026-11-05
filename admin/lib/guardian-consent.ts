/** COPPA: swimmers under 13 need a parent's recorded consent before they can have their own account. */

export const GUARDIAN_CONSENT_AGE = 13

/**
 * True when the swimmer is under 13 on `now` (UTC calendar date). Mirrors
 * swimmerNeedsGuardianConsent in server/internal/app/members/service.go; an
 * unparseable date of birth counts as under 13 so the UI fails closed.
 */
export function isUnderConsentAge(dateOfBirth: string | undefined | null, now: Date = new Date()): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(dateOfBirth ?? '')
  if (!match) return true
  const [, y, m, d] = match
  const turns13 = Date.UTC(Number(y) + GUARDIAN_CONSENT_AGE, Number(m) - 1, Number(d))
  const today = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate())
  return turns13 > today
}

export type ConsentState = 'not-needed' | 'needed' | 'given'

export function guardianConsentState(
  swimmer: { date_of_birth?: string | null; guardian_consent_at?: string | null },
  now: Date = new Date(),
): ConsentState {
  if (swimmer.guardian_consent_at) return 'given'
  return isUnderConsentAge(swimmer.date_of_birth, now) ? 'needed' : 'not-needed'
}
