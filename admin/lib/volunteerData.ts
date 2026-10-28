// API helpers and types for the volunteer jobs & service-hours domain
// (/api/v1/volunteer/*). Kept out of adminData.ts to avoid growing it.
import type { ApiClient } from './api'
import type { ComplianceStatus, VolunteerRole } from './volunteer-state'

const base = '/api/v1/volunteer'
const org = (orgId: string) => ({ 'X-Org-Id': orgId })

export type VolunteerJob = {
  id: string
  org_id: string
  meet_id?: string
  event_name?: string
  season_id?: string
  role: VolunteerRole
  title: string
  description: string
  location: string
  starts_at: string
  ends_at: string
  capacity: number
  hour_credit: number
  min_age?: number
  requires_certification: boolean
}

export type VolunteerSignup = {
  id: string
  job_id: string
  household_id: string
  volunteer_name: string
  swimmer_id?: string
  status: 'confirmed' | 'cancelled' | 'worked' | 'no_show'
  hours_credited?: number
  notes: string
  created_at: string
  household_name?: string
  effective_hours: number
  can_cancel: boolean
  job?: VolunteerJob
}

export type VolunteerJobView = VolunteerJob & {
  meet_name?: string
  effective_season_id?: string
  filled_count: number
  open_spots: number
  signups?: VolunteerSignup[]
}

export type VolunteerJobSummary = {
  job_count: number
  total_capacity: number
  filled_spots: number
  open_spots: number
  fill_rate: number
}

export type VolunteerBuyout = {
  id: string
  status: 'pending' | 'billed'
  hours_short: number
  amount_cents: number
  invoice_id?: string
}

export type VolunteerLedgerRow = {
  season_id: string
  season_name: string
  season_status: string
  season_start_date?: string
  season_end_date?: string
  household_id: string
  household_name: string
  required_hours: number
  worked_hours: number
  credited_hours: number
  earned_hours: number
  scheduled_hours: number
  remaining_hours: number
  buyout_cents_per_hour: number
  buyout_due_cents: number
  requirement_source: 'household' | 'season' | 'org_default'
  status: ComplianceStatus
  buyout?: VolunteerBuyout
}

export type VolunteerSettings = {
  default_required_hours: number
  default_buyout_cents_per_hour: number
  cancellation_cutoff_hours: number
  reminders_enabled: boolean
  timezone: string
}

export type VolunteerRequirement = {
  id: string
  season_id: string
  household_id?: string
  required_hours: number
  buyout_cents_per_hour: number
}

export type VolunteerTemplateRole = {
  role: VolunteerRole
  title: string
  description: string
  capacity: number
  min_age?: number
  requires_certification: boolean
}

export type VolunteerTemplate = { id: string; name: string; roles: VolunteerTemplateRole[] }

async function listAll<T>(api: ApiClient, path: string, key: string, orgId: string): Promise<{ items: T[]; raw: Record<string, unknown> }> {
  const items: T[] = []
  const sep = path.includes('?') ? '&' : '?'
  let offset = 0
  let raw: Record<string, unknown> = {}
  for (;;) {
    raw = await api.get<Record<string, unknown>>(`${path}${sep}limit=200&offset=${offset}`, undefined, org(orgId))
    const page = (raw[key] as T[] | undefined) ?? []
    items.push(...page)
    offset += page.length
    if (page.length === 0 || typeof raw.total !== 'number' || offset >= raw.total) break
  }
  return { items, raw }
}

export async function fetchVolunteerJobs(
  api: ApiClient,
  orgId: string,
  filter: { meetId?: string; seasonId?: string; upcoming?: boolean } = {},
): Promise<{ jobs: VolunteerJobView[]; summary: VolunteerJobSummary | null }> {
  const qs = new URLSearchParams()
  if (filter.meetId) qs.set('meet_id', filter.meetId)
  if (filter.seasonId) qs.set('season_id', filter.seasonId)
  if (filter.upcoming) qs.set('upcoming', 'true')
  const q = qs.toString()
  const { items, raw } = await listAll<VolunteerJobView>(api, `${base}/jobs${q ? `?${q}` : ''}`, 'jobs', orgId)
  return { jobs: items, summary: (raw.summary as VolunteerJobSummary | undefined) ?? null }
}

export async function fetchVolunteerJob(api: ApiClient, orgId: string, jobId: string): Promise<VolunteerJobView> {
  const res = await api.get<{ job: VolunteerJobView }>(`${base}/jobs/${encodeURIComponent(jobId)}`, undefined, org(orgId))
  return res.job
}

export type VolunteerJobInput = {
  meet_id?: string
  event_name?: string
  role: VolunteerRole
  title: string
  description?: string
  location?: string
  starts_at: string
  ends_at: string
  capacity: number
  hour_credit?: number
  min_age?: number
  requires_certification?: boolean
}

export async function createVolunteerJob(api: ApiClient, orgId: string, body: VolunteerJobInput): Promise<VolunteerJob> {
  const res = await api.post<{ job: VolunteerJob }>(`${base}/jobs`, body, org(orgId))
  return res.job
}

export async function updateVolunteerJob(api: ApiClient, orgId: string, jobId: string, body: Partial<VolunteerJobInput>): Promise<VolunteerJob> {
  const res = await api.patch<{ job: VolunteerJob }>(`${base}/jobs/${encodeURIComponent(jobId)}`, body, org(orgId))
  return res.job
}

export async function deleteVolunteerJob(api: ApiClient, orgId: string, jobId: string): Promise<void> {
  await api.delete(`${base}/jobs/${encodeURIComponent(jobId)}`, org(orgId))
}

export async function bulkCreateVolunteerJobs(api: ApiClient, orgId: string, body: {
  meet_id?: string
  event_name?: string
  location?: string
  template_id?: string
  shifts: Array<{ starts_at: string; ends_at: string; label?: string }>
}): Promise<number> {
  const res = await api.post<{ count: number }>(`${base}/jobs/bulk`, body, org(orgId))
  return res.count
}

export async function fetchVolunteerTemplates(api: ApiClient, orgId: string): Promise<VolunteerTemplate[]> {
  const res = await api.get<{ templates?: VolunteerTemplate[] }>(`${base}/templates`, undefined, org(orgId))
  return res.templates ?? []
}

export async function fetchVolunteerSignups(api: ApiClient, orgId: string, filter: { householdId?: string; meetId?: string } = {}): Promise<VolunteerSignup[]> {
  const qs = new URLSearchParams()
  if (filter.householdId) qs.set('household_id', filter.householdId)
  if (filter.meetId) qs.set('meet_id', filter.meetId)
  const q = qs.toString()
  const { items } = await listAll<VolunteerSignup>(api, `${base}/signups${q ? `?${q}` : ''}`, 'signups', orgId)
  return items
}

export async function signUpForVolunteerJob(api: ApiClient, orgId: string, jobId: string, body: {
  volunteer_name: string
  household_id?: string
  swimmer_id?: string
  certified?: boolean
  notes?: string
}): Promise<VolunteerSignup> {
  const res = await api.post<{ signup: VolunteerSignup }>(`${base}/jobs/${encodeURIComponent(jobId)}/signups`, body, org(orgId))
  return res.signup
}

export async function cancelVolunteerSignup(api: ApiClient, orgId: string, signupId: string, reason?: string): Promise<void> {
  await api.post(`${base}/signups/${encodeURIComponent(signupId)}/cancel`, reason ? { reason } : {}, org(orgId))
}

export async function checkInVolunteerSignup(api: ApiClient, orgId: string, signupId: string, status: 'worked' | 'no_show' | 'confirmed', hours?: number): Promise<void> {
  await api.post(`${base}/signups/${encodeURIComponent(signupId)}/check-in`, hours === undefined ? { status } : { status, hours }, org(orgId))
}

export async function fetchVolunteerSettings(api: ApiClient, orgId: string): Promise<VolunteerSettings> {
  const res = await api.get<{ settings: VolunteerSettings }>(`${base}/settings`, undefined, org(orgId))
  return res.settings
}

export async function updateVolunteerSettings(api: ApiClient, orgId: string, body: Partial<VolunteerSettings>): Promise<VolunteerSettings> {
  const res = await api.put<{ settings: VolunteerSettings }>(`${base}/settings`, body, org(orgId))
  return res.settings
}

export async function fetchVolunteerRequirements(api: ApiClient, orgId: string, seasonId: string): Promise<VolunteerRequirement[]> {
  const res = await api.get<{ requirements?: VolunteerRequirement[] }>(`${base}/requirements?season_id=${encodeURIComponent(seasonId)}`, undefined, org(orgId))
  return res.requirements ?? []
}

export async function upsertVolunteerRequirement(api: ApiClient, orgId: string, body: {
  season_id: string
  household_id?: string
  required_hours: number
  buyout_cents_per_hour: number
}): Promise<VolunteerRequirement> {
  const res = await api.put<{ requirement: VolunteerRequirement }>(`${base}/requirements`, body, org(orgId))
  return res.requirement
}

export async function addVolunteerCredit(api: ApiClient, orgId: string, body: {
  season_id: string
  household_id: string
  hours: number
  reason: string
}): Promise<void> {
  await api.post(`${base}/credits`, body, org(orgId))
}

export async function fetchSeasonLedger(api: ApiClient, orgId: string, seasonId: string): Promise<VolunteerLedgerRow[]> {
  const res = await api.get<{ ledger?: VolunteerLedgerRow[] }>(`${base}/ledger?season_id=${encodeURIComponent(seasonId)}`, undefined, org(orgId))
  return res.ledger ?? []
}

export async function fetchHouseholdLedger(api: ApiClient, orgId: string, householdId: string): Promise<VolunteerLedgerRow[]> {
  const res = await api.get<{ ledger?: VolunteerLedgerRow[] }>(`${base}/households/${encodeURIComponent(householdId)}/ledger`, undefined, org(orgId))
  return res.ledger ?? []
}

export async function billVolunteerBuyout(api: ApiClient, orgId: string, body: {
  season_id: string
  household_id: string
  issue: boolean
  due_date?: string
}): Promise<{ invoice_id: string; invoice_number: string; invoice_status: string }> {
  const res = await api.post<{ invoice: { invoice_id: string; invoice_number: string; invoice_status: string } }>(`${base}/buyouts`, body, org(orgId))
  return res.invoice
}
