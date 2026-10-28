// Display helpers for meet events. The API stores long stroke names
// (`freestyle`, `individual_medley`, `medley_relay`); imported/legacy data and
// best-time records use short codes (`free`, `im`). Accept both.

const STROKE_LABELS: Record<string, string> = {
  freestyle: 'Freestyle', free: 'Freestyle',
  backstroke: 'Backstroke', back: 'Backstroke',
  breaststroke: 'Breaststroke', breast: 'Breaststroke',
  butterfly: 'Butterfly', fly: 'Butterfly',
  individual_medley: 'IM', im: 'IM',
  freestyle_relay: 'Free Relay',
  medley_relay: 'Medley Relay', medley: 'Medley',
}

export function formatStroke(stroke?: string | null): string {
  if (!stroke) return ''
  const key = stroke.trim().toLowerCase()
  return STROKE_LABELS[key] ?? key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}

export function formatAgeGroup(minAge?: number | null, maxAge?: number | null, fallback?: string | null): string {
  if (fallback) return fallback
  if (minAge != null && maxAge != null) return minAge === maxAge ? `${minAge}` : `${minAge}-${maxAge}`
  if (minAge != null) return `${minAge}&O`
  if (maxAge != null) return `${maxAge}&U`
  return 'Open'
}

type EventLike = {
  distance?: number | null
  stroke?: string | null
  gender?: string | null
  min_age?: number | null
  max_age?: number | null
  age_group?: string | null
}

/** e.g. "50 Freestyle (F 11-12)" */
export function formatEventLabel(ev: EventLike | undefined | null): string {
  if (!ev) return '—'
  const name = [ev.distance ?? '', formatStroke(ev.stroke)].filter(Boolean).join(' ')
  const qualifier = [ev.gender ?? '', formatAgeGroup(ev.min_age, ev.max_age, ev.age_group)].filter(Boolean).join(' ')
  return qualifier ? `${name} (${qualifier})` : name
}
