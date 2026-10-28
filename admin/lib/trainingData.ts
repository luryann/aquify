// Structured workouts, season periodization and training analytics.
// Backend: server/internal/app/training + handlers/training_workouts.go.
import type { ApiClient } from '#lib/api'
import type { PracticeRecord } from './adminData.ts'

const org = (orgId: string) => ({ 'X-Org-Id': orgId })
const qs = (params: Record<string, string>) => `?${new URLSearchParams(params).toString()}`

export type Course = 'SCY' | 'SCM' | 'LCM'
export type SectionKind = 'warmup' | 'pre_set' | 'main' | 'kick' | 'pull' | 'drill' | 'cooldown' | 'custom'
export type Zone = 'REC' | 'EN1' | 'EN2' | 'EN3' | 'SP1' | 'SP2' | 'SP3'

export type WorkoutSet = {
  repeats: number
  distance?: number
  stroke?: string
  interval_seconds?: number
  rest_seconds?: number
  zone?: Zone | ''
  equipment?: string[]
  notes?: string
  sets?: WorkoutSet[]
}

export type WorkoutSection = { kind: SectionKind; title?: string; notes?: string; sets: WorkoutSet[] }

export type WorkoutTotals = {
  distance: number
  unit: 'yd' | 'm'
  distance_yards: number
  lengths: number
  by_stroke: Record<string, number>
  by_zone: Record<string, number>
  by_section: { index: number; kind: SectionKind; title?: string; distance: number; estimated_seconds: number }[]
  estimated_seconds: number
  warnings: string[]
}

export type Workout = {
  id: string
  title: string
  description: string
  course: Course
  is_template: boolean
  tags: string[]
  sections: WorkoutSection[]
  source_workout_id?: string
  totals: WorkoutTotals
  updated_at: string
}

export type WorkoutInput = {
  title: string
  description?: string
  course: Course
  is_template?: boolean
  tags?: string[]
  sections: WorkoutSection[]
}

export type ParseError = { line: number; text: string; message: string }
export type ParsedWorkout = { course: Course; sections: WorkoutSection[]; errors: ParseError[]; totals: WorkoutTotals }

export type TrainingPractice = PracticeRecord & { workout_id?: string }
export type ScheduleItem = { practice: TrainingPractice; workout: Workout | null }

export async function fetchWorkouts(api: ApiClient, orgId: string, params: { q?: string; tag?: string } = {}): Promise<Workout[]> {
  const query: Record<string, string> = { scope: 'templates', limit: '200' }
  if (params.q) query.q = params.q
  if (params.tag) query.tag = params.tag
  const res = await api.get<{ workouts?: Workout[] }>(`/api/v1/workouts${qs(query)}`, undefined, org(orgId))
  return res.workouts ?? []
}

export async function fetchWorkout(api: ApiClient, orgId: string, id: string): Promise<{ workout: Workout; text: string }> {
  return api.get<{ workout: Workout; text: string }>(`/api/v1/workouts/${id}`, undefined, org(orgId))
}

export async function parseWorkoutText(api: ApiClient, orgId: string, text: string, course: Course): Promise<ParsedWorkout> {
  return api.post<ParsedWorkout>('/api/v1/workouts/parse', { text, course }, org(orgId))
}

export async function createWorkout(api: ApiClient, orgId: string, body: WorkoutInput): Promise<Workout> {
  return (await api.post<{ workout: Workout }>('/api/v1/workouts', body, org(orgId))).workout
}

export async function updateWorkout(api: ApiClient, orgId: string, id: string, body: WorkoutInput): Promise<Workout> {
  return (await api.put<{ workout: Workout }>(`/api/v1/workouts/${id}`, body, org(orgId))).workout
}

export async function deleteWorkout(api: ApiClient, orgId: string, id: string): Promise<void> {
  await api.delete(`/api/v1/workouts/${id}`, org(orgId))
}

export async function duplicateWorkout(api: ApiClient, orgId: string, id: string, body: { title?: string; is_template?: boolean } = {}): Promise<Workout> {
  return (await api.post<{ workout: Workout }>(`/api/v1/workouts/${id}/duplicate`, body, org(orgId))).workout
}

export async function assignWorkout(api: ApiClient, orgId: string, id: string, body: { practice_id?: string; date?: string; group_id?: string }): Promise<{ practice: TrainingPractice; workout: Workout }> {
  return api.post<{ practice: TrainingPractice; workout: Workout }>(`/api/v1/workouts/${id}/assign`, body, org(orgId))
}

export async function savePracticeWorkout(api: ApiClient, orgId: string, practiceId: string, body: WorkoutInput): Promise<{ practice: TrainingPractice; workout: Workout }> {
  return api.put<{ practice: TrainingPractice; workout: Workout }>(`/api/v1/practices/${practiceId}/workout`, { ...body, is_template: false }, org(orgId))
}

export async function clearPracticeWorkout(api: ApiClient, orgId: string, practiceId: string): Promise<void> {
  await api.delete(`/api/v1/practices/${practiceId}/workout`, org(orgId))
}

export async function fetchTrainingSchedule(api: ApiClient, orgId: string, from: string, to: string): Promise<ScheduleItem[]> {
  const res = await api.get<{ items?: ScheduleItem[] }>(`/api/v1/training/schedule${qs({ from, to })}`, undefined, org(orgId))
  return res.items ?? []
}

// ── Analytics ────────────────────────────────────────────────────────────────

type Counts = { present: number; absent: number; excused: number; attendance_pct: number | null }

export type GroupWeek = Counts & { week_start: string; group_id: string; practices: number; planned_yards: number; swimmer_yards: number }
export type GroupSummary = Counts & { group_id: string; practices: number; planned_yards: number; swimmer_yards: number }

export type TrainingAnalytics = {
  from: string
  to: string
  weeks: GroupWeek[]
  groups: GroupSummary[]
  zone_yards: Record<string, number>
  stroke_yards: Record<string, number>
  total_practices: number
  structured_practices: number
}

export type SwimmerTrainingAnalytics = Counts & {
  swimmer_id: string
  yards: number
  scheduled: number
  zone_yards: Record<string, number>
  weeks: (Counts & { week_start: string; yards: number; scheduled: number })[]
}

export async function fetchTrainingAnalytics(api: ApiClient, orgId: string, from: string, to: string): Promise<TrainingAnalytics> {
  return api.get<TrainingAnalytics>(`/api/v1/training/analytics${qs({ from, to })}`, undefined, org(orgId))
}

export async function fetchSwimmerTrainingAnalytics(api: ApiClient, orgId: string, swimmerId: string, from: string, to: string): Promise<SwimmerTrainingAnalytics> {
  return api.get<SwimmerTrainingAnalytics>(`/api/v1/training/analytics/swimmers/${swimmerId}${qs({ from, to })}`, undefined, org(orgId))
}

// ── Periodization ────────────────────────────────────────────────────────────

export type PhaseKind = 'base' | 'build' | 'peak' | 'taper' | 'recovery'

export type TrainingPhase = {
  id: string
  season_id: string
  group_id: string
  phase: PhaseKind
  name: string
  start_date: string
  end_date: string
  weekly_yardage_target: number
  focus: string
}

export type TrainingPhaseInput = Omit<TrainingPhase, 'id' | 'season_id'>

export type PlanWeek = {
  week_start: string
  week_end: string
  group_id: string
  phase_id?: string
  phase?: PhaseKind
  target_yards: number
  planned_yards: number
  practices: number
}

export type SeasonPlan = { season_id: string; start_date: string; end_date: string; phases: TrainingPhase[]; weeks: PlanWeek[] }

export async function fetchSeasonPlan(api: ApiClient, orgId: string, seasonId: string, groupId?: string): Promise<SeasonPlan> {
  const res = await api.get<{ plan: SeasonPlan }>(`/api/v1/seasons/${seasonId}/training-plan${groupId ? qs({ group_id: groupId }) : ''}`, undefined, org(orgId))
  return res.plan
}

export async function createTrainingPhase(api: ApiClient, orgId: string, seasonId: string, body: TrainingPhaseInput): Promise<TrainingPhase> {
  return (await api.post<{ phase: TrainingPhase }>(`/api/v1/seasons/${seasonId}/training-phases`, body, org(orgId))).phase
}

export async function deleteTrainingPhase(api: ApiClient, orgId: string, phaseId: string): Promise<void> {
  await api.delete(`/api/v1/training-phases/${phaseId}`, org(orgId))
}
