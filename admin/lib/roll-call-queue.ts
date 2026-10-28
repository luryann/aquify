// Offline-tolerant write queue for pool-deck roll call. Each mark is keyed by
// practice+swimmer so only the latest tap is sent; failed writes stay queued
// (persisted to localStorage) and retry with backoff until the API accepts them.
import type { AttendanceStatus } from './adminData'

export type QueuedMark = {
  practiceId: string
  swimmerId: string
  status: AttendanceStatus
  queuedAt: number
  attempts: number
  nextAttemptAt: number
  lastError?: string
}

export type RollCallQueue = Record<string, QueuedMark>

export const ROLL_CALL_STORAGE_KEY = 'aquify:roll-call-queue:v1'
const MAX_BACKOFF_MS = 60_000

export function markKey(practiceId: string, swimmerId: string): string {
  return `${practiceId}:${swimmerId}`
}

/** Adds or replaces a mark; a newer tap resets its retry state. */
export function enqueueMark(queue: RollCallQueue, practiceId: string, swimmerId: string, status: AttendanceStatus, now: number): RollCallQueue {
  return { ...queue, [markKey(practiceId, swimmerId)]: { practiceId, swimmerId, status, queuedAt: now, attempts: 0, nextAttemptAt: now } }
}

export function dueMarks(queue: RollCallQueue, now: number): QueuedMark[] {
  return Object.values(queue).filter((m) => m.nextAttemptAt <= now).sort((a, b) => a.queuedAt - b.queuedAt)
}

export function backoffMs(attempts: number): number {
  return Math.min(MAX_BACKOFF_MS, 2000 * 2 ** Math.max(0, attempts - 1))
}

/** Removes a mark once sent, unless a newer tap replaced it meanwhile. */
export function settleMark(queue: RollCallQueue, sent: QueuedMark): RollCallQueue {
  const key = markKey(sent.practiceId, sent.swimmerId)
  const current = queue[key]
  if (!current || current.queuedAt !== sent.queuedAt) return queue
  const next = { ...queue }
  delete next[key]
  return next
}

export function failMark(queue: RollCallQueue, sent: QueuedMark, error: string, now: number): RollCallQueue {
  const key = markKey(sent.practiceId, sent.swimmerId)
  const current = queue[key]
  if (!current || current.queuedAt !== sent.queuedAt) return queue
  const attempts = current.attempts + 1
  return { ...queue, [key]: { ...current, attempts, lastError: error, nextAttemptAt: now + backoffMs(attempts) } }
}

/** True for errors worth retrying (network/5xx/429); 4xx validation is permanent. */
export function isRetryable(status?: number): boolean {
  return status === undefined || status === 0 || status === 408 || status === 429 || status >= 500
}

export function loadQueue(storage: Pick<Storage, 'getItem'> | undefined): RollCallQueue {
  try {
    const raw = storage?.getItem(ROLL_CALL_STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as RollCallQueue
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

export function saveQueue(storage: Pick<Storage, 'setItem'> | undefined, queue: RollCallQueue): void {
  try {
    storage?.setItem(ROLL_CALL_STORAGE_KEY, JSON.stringify(queue))
  } catch {
    // Storage full or blocked: the in-memory queue still retries this session.
  }
}

/** Status shown for a swimmer: a pending local tap wins over the server value. */
export function effectiveStatus(queue: RollCallQueue, practiceId: string, swimmerId: string, server?: AttendanceStatus): AttendanceStatus | undefined {
  return queue[markKey(practiceId, swimmerId)]?.status ?? server
}

export function pendingForPractice(queue: RollCallQueue, practiceId: string): QueuedMark[] {
  return Object.values(queue).filter((m) => m.practiceId === practiceId)
}
