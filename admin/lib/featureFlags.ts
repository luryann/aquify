/**
 * Feature flags for the DARE platform frontend.
 *
 * Flags are read from environment variables at build/runtime.
 * The swimmer portal is behind a flag because swimmer accounts
 * are optional per platform policy — no launch-critical parent
 * flow should require a swimmer account.
 */

/** Whether the optional swimmer portal (/swim/*) is enabled. */
export const SWIMMER_PORTAL_ENABLED =
  process.env.NEXT_PUBLIC_SWIMMER_PORTAL_ENABLED === 'true'

/** Whether dev stub tokens are active (never true in production). */
export const DEV_STUB_TOKENS_ENABLED =
  process.env.NEXT_PUBLIC_APP_ENV !== 'production' &&
  process.env.NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS !== 'false'
