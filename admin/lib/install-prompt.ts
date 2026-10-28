export const INSTALL_DISMISS_KEY = 'aquify.install-prompt.dismissed-at'
export const INSTALL_DISMISS_DAYS = 90
export const INSTALL_VISITS_KEY = 'aquify.install-prompt.visit-days'
/** Distinct days a user must have opened the portal before we offer install. */
export const INSTALL_MIN_VISIT_DAYS = 3

type StorageLike = Pick<Storage, 'getItem' | 'setItem'>

/** True when the user dismissed the install banner within the snooze window. */
export function isInstallDismissed(storage: StorageLike | null, now: number = Date.now()): boolean {
  try {
    const raw = storage?.getItem(INSTALL_DISMISS_KEY)
    if (!raw) return false
    const at = Number(raw)
    if (!Number.isFinite(at)) return false
    return now - at < INSTALL_DISMISS_DAYS * 86_400_000
  } catch {
    return false
  }
}

export function rememberInstallDismissed(storage: StorageLike | null, now: number = Date.now()): void {
  try {
    storage?.setItem(INSTALL_DISMISS_KEY, String(now))
  } catch {
    /* storage unavailable; banner may reappear next visit */
  }
}

/**
 * Records today as a visit day and returns how many distinct days (most recent
 * ten) the user has opened the portal on.
 */
export function recordVisitDay(storage: StorageLike | null, now: number = Date.now()): number {
  try {
    const today = new Date(now).toISOString().slice(0, 10)
    const raw = storage?.getItem(INSTALL_VISITS_KEY)
    const parsed: unknown = raw ? JSON.parse(raw) : []
    const days = Array.isArray(parsed) ? parsed.filter((d): d is string => typeof d === 'string') : []
    if (!days.includes(today)) days.push(today)
    const recent = days.slice(-10)
    storage?.setItem(INSTALL_VISITS_KEY, JSON.stringify(recent))
    return recent.length
  } catch {
    return 0
  }
}

/** Offer install only to returning users who haven't recently said no. */
export function shouldOfferInstall(storage: StorageLike | null, now: number = Date.now()): boolean {
  if (isInstallDismissed(storage, now)) return false
  return recordVisitDay(storage, now) >= INSTALL_MIN_VISIT_DAYS
}

export function safeLocalStorage(): Storage | null {
  try {
    return typeof window === 'undefined' ? null : window.localStorage
  } catch {
    return null
  }
}
