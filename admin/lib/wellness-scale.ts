// Wellness readiness helpers shared by the swimmer form and coach views.
// Mirrors server/internal/app/wellness.Readiness.
import type { WellnessMetrics } from './engagementData'

export function readiness(m: Pick<WellnessMetrics, 'sleep_quality' | 'soreness' | 'energy' | 'stress' | 'mood'>): number {
  const sum = m.sleep_quality + m.energy + m.mood + (6 - m.soreness) + (6 - m.stress)
  return Math.round((sum / 5) * 10) / 10
}

/** Bucket a 1–5 readiness score into the 5-step diverging scale. */
export function readinessStep(score: number | null | undefined): 1 | 2 | 3 | 4 | 5 | null {
  if (score === null || score === undefined || Number.isNaN(score)) return null
  if (score < 1.8) return 1
  if (score < 2.6) return 2
  if (score < 3.4) return 3
  if (score < 4.2) return 4
  return 5
}

export const READINESS_STEP_LABELS = { 1: 'Very low', 2: 'Low', 3: 'Steady', 4: 'Good', 5: 'Great' } as const

export type ScaleField = 'sleep_quality' | 'soreness' | 'energy' | 'stress' | 'mood'

export const SCALE_QUESTIONS: { field: ScaleField; label: string; low: string; high: string }[] = [
  { field: 'sleep_quality', label: 'How well did you sleep?', low: 'Badly', high: 'Great' },
  { field: 'energy', label: 'Energy', low: 'Drained', high: 'Charged' },
  { field: 'soreness', label: 'Muscle soreness', low: 'None', high: 'Very sore' },
  { field: 'stress', label: 'Stress', low: 'Calm', high: 'Very stressed' },
  { field: 'mood', label: 'Mood', low: 'Low', high: 'Great' },
]

/** Today's date in the viewer's timezone (check-ins are per local day). */
export function localToday(now = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${now.getFullYear()}-${p(now.getMonth() + 1)}-${p(now.getDate())}`
}
