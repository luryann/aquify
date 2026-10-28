import type { RelayLock } from './performanceData'

const MEDLEY_LEGS = ['Back', 'Breast', 'Fly', 'Free']

/** Label for a relay leg: stroke for medley, swim order for free. */
export function relayLegLabel(relayType: string, leg: number): string {
  if (relayType === 'medley') return MEDLEY_LEGS[leg - 1] ?? `Leg ${leg}`
  return leg === 1 ? 'Lead-off' : leg === 4 ? 'Anchor' : `Leg ${leg}`
}

/** Toggles a lock: removes it if this exact lock exists, otherwise replaces any
 *  lock on the same slot or the same swimmer (a swimmer holds one leg). */
export function toggleLock(locks: RelayLock[], lock: RelayLock): RelayLock[] {
  const exists = locks.some((l) => l.team === lock.team && l.leg === lock.leg && l.swimmer_id === lock.swimmer_id)
  if (exists) return locks.filter((l) => !(l.team === lock.team && l.leg === lock.leg))
  return [...locks.filter((l) => !(l.team === lock.team && l.leg === lock.leg) && l.swimmer_id !== lock.swimmer_id), lock]
}
