export type PeriodPreset = 'all' | '12m' | 'ytd'

function iso(d: Date) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** Resolves a preset time window to inclusive YYYY-MM-DD bounds ("" = open). */
export function periodRange(preset: PeriodPreset, today: Date): { from: string; to: string } {
  switch (preset) {
    case '12m': {
      const from = new Date(today)
      from.setFullYear(from.getFullYear() - 1)
      return { from: iso(from), to: iso(today) }
    }
    case 'ytd':
      return { from: `${today.getFullYear()}-01-01`, to: iso(today) }
    default:
      return { from: '', to: '' }
  }
}

/** Default progress-report comparison: the 12 months before the last 12 months vs. the last 12 months. */
export function defaultComparison(today: Date) {
  const bEnd = new Date(today)
  const bStart = new Date(today)
  bStart.setFullYear(bStart.getFullYear() - 1)
  bStart.setDate(bStart.getDate() + 1)
  const aEnd = new Date(today)
  aEnd.setFullYear(aEnd.getFullYear() - 1)
  const aStart = new Date(today)
  aStart.setFullYear(aStart.getFullYear() - 2)
  aStart.setDate(aStart.getDate() + 1)
  return { from_a: iso(aStart), to_a: iso(aEnd), from_b: iso(bStart), to_b: iso(bEnd) }
}

/** Goal progress bar fill (0–100) and tone. */
export function goalTone(goal: { achieved: boolean; overdue: boolean; progress_percent?: number }): { fill: number; tone: 'success' | 'warning' | 'accent' } {
  if (goal.achieved) return { fill: 100, tone: 'success' }
  const fill = Math.max(0, Math.min(100, goal.progress_percent ?? 0))
  return { fill, tone: goal.overdue ? 'warning' : 'accent' }
}
