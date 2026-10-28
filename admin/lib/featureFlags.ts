/**
 * Feature flags for the DARE platform frontend.
 *
 * Flags are read from environment variables at build/runtime.
 */

/** Whether dev stub tokens are active (never true in production). */
export const DEV_STUB_TOKENS_ENABLED =
  process.env.NEXT_PUBLIC_APP_ENV !== 'production' &&
  process.env.NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS !== 'false'
