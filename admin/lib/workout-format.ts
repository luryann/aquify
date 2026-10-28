// Pure rendering helpers for structured workouts (shared by the admin
// library, the pool-deck view and the swimmer schedule).
import type { SectionKind, WorkoutSet, Zone } from './trainingData.ts'

export const ZONES: Zone[] = ['REC', 'EN1', 'EN2', 'EN3', 'SP1', 'SP2', 'SP3']

export const ZONE_LABELS: Record<Zone, string> = {
  REC: 'Recovery',
  EN1: 'Aerobic',
  EN2: 'Threshold',
  EN3: 'VO2 / overload',
  SP1: 'Race pace',
  SP2: 'Lactate',
  SP3: 'Sprint',
}

// Zones are an ordinal intensity scale, so they share one sequential ramp
// (the brand accent mixed into the surface, light → dark) rather than seven
// unrelated hues. Every use pairs the swatch with the zone code as text.
const ZONE_STRENGTH: Record<Zone, number> = { REC: 18, EN1: 32, EN2: 46, EN3: 60, SP1: 74, SP2: 87, SP3: 100 }

export function zoneColor(zone: string | undefined): string {
  if (!zone || !(zone in ZONE_STRENGTH)) return 'var(--border-strong)'
  return `color-mix(in oklab, var(--accent) ${ZONE_STRENGTH[zone as Zone]}%, var(--bg-surface))`
}

/** Zones whose swatch is dark enough to need on-accent text. */
export function zoneNeedsLightText(zone: string | undefined): boolean {
  return !!zone && (ZONE_STRENGTH[zone as Zone] ?? 0) >= 60
}

export const SECTION_LABELS: Record<SectionKind, string> = {
  warmup: 'Warmup',
  pre_set: 'Pre-set',
  main: 'Main set',
  kick: 'Kick',
  pull: 'Pull',
  drill: 'Drill',
  cooldown: 'Cooldown',
  custom: 'Set',
}

const STROKE_LABELS: Record<string, string> = {
  free: 'free', back: 'back', breast: 'breast', fly: 'fly', im: 'IM',
  choice: 'choice', kick: 'kick', pull: 'pull', drill: 'drill',
}

const EQUIPMENT_LABELS: Record<string, string> = {
  pull_buoy: 'buoy', drag_suit: 'drag suit', tempo_trainer: 'tempo trainer', kickboard: 'board',
}

/** 90 → "1:30", 45 → ":45". */
export function formatClock(seconds: number | undefined): string {
  const s = Math.max(0, Math.round(seconds ?? 0))
  if (s < 60) return `:${String(s).padStart(2, '0')}`
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

/** 4500 → "1h 15m", 1800 → "30m". */
export function formatDuration(seconds: number): string {
  const mins = Math.round(seconds / 60)
  if (mins < 60) return `${mins}m`
  const h = Math.floor(mins / 60)
  const m = mins % 60
  return m ? `${h}h ${m}m` : `${h}h`
}

export function formatDistance(distance: number, unit: 'yd' | 'm'): string {
  return `${distance.toLocaleString('en-US')} ${unit}`
}

/** The set's prescription line, without nested children: "8 × 100 free". */
export function setHeadline(set: WorkoutSet): string {
  const stroke = set.stroke ? ` ${STROKE_LABELS[set.stroke] ?? set.stroke}` : ''
  if (set.sets && set.sets.length > 0) {
    return `${set.repeats} ${set.repeats === 1 ? 'round' : 'rounds'}${stroke}`
  }
  const head = set.repeats > 1 ? `${set.repeats} × ${set.distance ?? 0}` : `${set.distance ?? 0}`
  return `${head}${stroke}`
}

/** Send-off / rest / equipment / notes as short fragments. */
export function setDetails(set: WorkoutSet): string[] {
  const out: string[] = []
  if (set.interval_seconds) out.push(`@ ${formatClock(set.interval_seconds)}`)
  if (set.rest_seconds) out.push(`${formatClock(set.rest_seconds)} rest`)
  for (const e of set.equipment ?? []) out.push(EQUIPMENT_LABELS[e] ?? e)
  if (set.notes) out.push(set.notes)
  return out
}

/** One-line plain text for a set, recursing into rounds. */
export function formatSetText(set: WorkoutSet): string {
  const details = setDetails(set)
  const zone = set.zone ? ` ${set.zone}` : ''
  const tail = details.length ? ` ${details.join(' ')}` : ''
  if (set.sets && set.sets.length > 0) {
    return `${set.repeats} × {${set.sets.map(formatSetText).join('; ')}}${zone}${tail}`
  }
  return `${setHeadline(set)}${zone}${tail}`
}

/** Distance of one execution of a set (repeats included). */
export function setDistance(set: WorkoutSet): number {
  if (set.sets && set.sets.length > 0) {
    return set.repeats * set.sets.reduce((sum, child) => sum + setDistance(child), 0)
  }
  return set.repeats * (set.distance ?? 0)
}

/** Zones present in a totals map, intensity order, with their share (0–1). */
export function zoneShares(byZone: Record<string, number>): { zone: string; distance: number; share: number }[] {
  const total = Object.values(byZone).reduce((a, b) => a + b, 0)
  if (total <= 0) return []
  const order = [...ZONES, 'unassigned']
  return order
    .filter((z) => (byZone[z] ?? 0) > 0)
    .map((z) => ({ zone: z, distance: byZone[z], share: byZone[z] / total }))
}

/** "a, b , A" → ["a", "b"] (lower-cased, de-duplicated). */
export function parseTags(raw: string): string[] {
  return [...new Set(raw.split(',').map((t) => t.trim().toLowerCase()).filter(Boolean))]
}

/** Monday (YYYY-MM-DD) of the week containing an ISO date, UTC-safe. */
export function weekStartOf(date: string): string {
  const d = new Date(`${date}T00:00:00Z`)
  const offset = (d.getUTCDay() + 6) % 7
  d.setUTCDate(d.getUTCDate() - offset)
  return d.toISOString().slice(0, 10)
}

export function addDaysISO(date: string, days: number): string {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}
