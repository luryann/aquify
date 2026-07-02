import type { SwimmerCutQualificationCard } from './swimmer-cut-qualifications.ts'

export type SwimmerCutsViewModel = {
  state: 'loading' | 'error' | 'empty' | 'ready'
  title: string
  message: string
  qualifiedCount: number
  totalCount: number
}

export type CutQualificationStatus = {
  badge: string
  detail: string
  tone: 'success' | 'warning' | 'muted'
}

export function buildSwimmerCutsViewModel(input: {
  loading: boolean
  error?: string | null
  cards: SwimmerCutQualificationCard[]
}): SwimmerCutsViewModel {
  if (input.loading) {
    return {
      state: 'loading',
      title: 'Loading cut qualifications',
      message: 'Fetching active time standards and your best-time progress.',
      qualifiedCount: 0,
      totalCount: 0,
    }
  }

  if (input.error) {
    return {
      state: 'error',
      title: 'Unable to load cut qualifications',
      message: input.error,
      qualifiedCount: 0,
      totalCount: 0,
    }
  }

  if (input.cards.length === 0) {
    return {
      state: 'empty',
      title: 'No applicable cut standards',
      message: 'There are no active standards matching your age, gender, and current season setup yet.',
      qualifiedCount: 0,
      totalCount: 0,
    }
  }

  const qualifiedCount = input.cards.filter((card) => card.qualification.qualified).length
  return {
    state: 'ready',
    title: qualifiedCount > 0 ? 'Qualified standards' : 'Standards to chase',
    message: qualifiedCount > 0
      ? `You have qualified for ${qualifiedCount} of ${input.cards.length} active standard${input.cards.length === 1 ? '' : 's'}.`
      : `You have ${input.cards.length} active standard${input.cards.length === 1 ? '' : 's'} available to track.`,
    qualifiedCount,
    totalCount: input.cards.length,
  }
}

export function describeCutQualification(card: SwimmerCutQualificationCard): CutQualificationStatus {
  const qualification = card.qualification
  if (qualification.best_time_ms == null) {
    return {
      badge: 'No Time Yet',
      detail: 'No recorded best time is available for this event yet.',
      tone: 'muted',
    }
  }

  if (qualification.qualified) {
    const marginMs = qualification.margin_ms ?? 0
    return {
      badge: 'Qualified',
      detail: `Under the cut by ${formatMarginMs(marginMs)}.`,
      tone: 'success',
    }
  }

  const marginMs = qualification.margin_ms == null ? null : Math.abs(qualification.margin_ms)
  return {
    badge: 'Not Yet Qualified',
    detail: marginMs == null
      ? 'A recorded best time exists, but the qualification margin is unavailable.'
      : `${formatMarginMs(marginMs)} away from the cut.`,
    tone: 'warning',
  }
}

function formatMarginMs(value: number): string {
  const totalSeconds = value / 1000
  if (totalSeconds < 60) {
    return `${totalSeconds.toFixed(2)}s`
  }
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = (totalSeconds % 60).toFixed(2).padStart(5, '0')
  return `${minutes}:${seconds}`
}
