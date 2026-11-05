/**
 * Single source of truth for the legal documents. Bump LEGAL_VERSION whenever the
 * privacy policy changes in a way that affects children's data; it must match
 * GuardianConsentVersion in server/internal/app/members/service.go so recorded
 * parental consent maps to the text the parent actually saw.
 */
export const LEGAL_VERSION = '2026-11-03'
export const LEGAL_EFFECTIVE_DATE = 'November 3, 2026'
export const LEGAL_CONTACT_EMAIL = process.env.NEXT_PUBLIC_PRIVACY_CONTACT_EMAIL ?? 'privacy@aquify.club'
export const PRIVACY_PATH = '/privacy'
export const TERMS_PATH = '/terms'
