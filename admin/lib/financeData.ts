import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { ApiError, rawRequestAuthHeaders, type ApiClient, orgHeaders } from './api.ts'
import { fetchAllPages } from './adminData.ts'

// ─── Types (mirror server/internal/app/billing finance contracts) ────────────

export type FeeScheduleKind = 'dues' | 'registration'
export type FeeFrequency = 'monthly' | 'quarterly' | 'annual' | 'one_time'
export type FeeProration = 'none' | 'daily' | 'half' | 'waive'

export type FeeSchedule = {
  id: string
  name: string
  kind: FeeScheduleKind
  group_code?: string
  season?: string
  frequency: FeeFrequency
  amount_cents: number
  start_date: string
  end_date?: string
  proration: FeeProration
  auto_bill: boolean
  active: boolean
  description?: string
}

export type FeeScheduleInput = Omit<FeeSchedule, 'id' | 'active'> & { active?: boolean }

export type DiscountRule = {
  id: string
  name: string
  applies_to: 'dues' | 'registration' | 'all'
  min_position: number
  max_position?: number | null
  discount_type: 'percent' | 'fixed'
  percent_bps: number
  amount_cents: number
  active: boolean
}

export type DiscountRuleInput = Omit<DiscountRule, 'id' | 'active'> & { active?: boolean }

export type BillingAdjustment = {
  id: string
  household_id: string
  swimmer_id?: string
  amount_cents: number
  reason: string
  status: 'pending' | 'applied' | 'voided'
  applied_invoice_id?: string
  created_at: string
}

export type BillingSettings = {
  late_fee_enabled: boolean
  late_fee_grace_days: number
  late_fee_type: 'flat' | 'percent'
  late_fee_flat_cents: number
  late_fee_percent_bps: number
  late_fee_max_cents: number
  auto_run_enabled: boolean
  auto_run_day: number
  auto_run_invoice_status: 'draft' | 'issued'
  auto_run_due_days: number
  meet_fee_per_event_cents: number
  meet_fee_flat_cents: number
  meet_fee_invoice_status: 'draft' | 'issued'
  meet_fee_due_days: number
}

export type RunInput = {
  as_of: string
  schedule_ids?: string[]
  invoice_status?: 'draft' | 'issued'
  due_date?: string
  include_adjustments?: boolean
  expected_fingerprint?: string
}

export type RunPreviewLine = {
  kind: 'dues' | 'registration' | 'discount' | 'adjustment'
  description: string
  swimmer_name?: string
  amount_cents: number
  sibling_position?: number
  note?: string
}

export type RunPreviewHousehold = {
  household_id: string
  household_name: string
  swimmer_count: number
  lines: RunPreviewLine[]
  gross_cents: number
  discount_cents: number
  adjustment_cents: number
  total_cents: number
  credit_to_apply_cents: number
  amount_due_cents: number
}

export type RunSkip = {
  household_id: string
  household_name?: string
  swimmer_name?: string
  schedule_name?: string
  reason: string
}

export type RunPreview = {
  as_of: string
  invoice_status: string
  due_date: string
  households: RunPreviewHousehold[]
  skipped: RunSkip[]
  household_count: number
  swimmer_count: number
  charge_count: number
  gross_cents: number
  discount_cents: number
  adjustment_cents: number
  net_cents: number
  credit_to_apply_cents: number
  fingerprint: string
}

export type BillingRun = {
  id: string
  as_of: string
  invoice_status: string
  trigger: 'manual' | 'scheduler'
  household_count: number
  invoice_count: number
  charge_count: number
  skipped_count: number
  gross_cents: number
  discount_cents: number
  adjustment_cents: number
  credit_applied_cents: number
  net_cents: number
  created_at: string
  created_by?: string
}

export type RunResult = {
  run: BillingRun
  invoices: { household_name: string; invoice_id: string; invoice_number: string; total_cents: number; status: string }[]
  skipped: RunSkip[]
}

export type AgingBuckets = {
  current_cents: number
  days_1_30_cents: number
  days_31_60_cents: number
  days_61_90_cents: number
  days_90_plus_cents: number
  total_cents: number
}

export type AgingReport = {
  as_of: string
  rows: (AgingBuckets & { household_id: string; household_name: string; invoice_count: number; oldest_due_date?: string })[]
  totals: AgingBuckets
}

export type RevenueTotal = { key: string; label: string; amount_cents: number }

export type RevenueReport = {
  from: string
  to: string
  by_month: RevenueTotal[]
  by_category: RevenueTotal[]
  by_group: RevenueTotal[]
  total_cents: number
}

export type DepositsReport = {
  rows: { period_start: string; period_end: string; payment_count: number; gross_cents: number; refunds_cents: number; net_cents: number }[]
  gross_cents: number
  refunds_cents: number
  net_cents: number
}

export type StatementLine = {
  date: string
  type: string
  description: string
  reference?: string
  amount_cents: number
  display_amount_cents: number
  balance_cents: number
  informational: boolean
}

export type Statement = {
  household_id: string
  opening_balance_cents: number
  closing_balance_cents: number
  charges_cents: number
  payments_cents: number
  credits_cents: number
  credit_balance_cents: number
  open_invoices_due_cents: number
  lines: StatementLine[]
}

export type UpcomingCharges = {
  through: string
  charges: { schedule_name: string; swimmer_name: string; period_start: string; period_end: string; gross_cents: number; discount_cents: number; net_cents: number }[]
  total_cents: number
  credit_balance_cents: number
}

// ─── Fetchers ────────────────────────────────────────────────────────────────

export async function fetchFeeSchedules(api: ApiClient, orgId: string) {
  const res = await api.get<{ fee_schedules: FeeSchedule[] }>('/api/v1/billing/fee-schedules', undefined, orgHeaders(orgId))
  return res.fee_schedules ?? []
}

export async function saveFeeSchedule(api: ApiClient, orgId: string, input: FeeScheduleInput, id?: string) {
  const res = id
    ? await api.put<{ fee_schedule: FeeSchedule }>(`/api/v1/billing/fee-schedules/${id}`, input, orgHeaders(orgId))
    : await api.post<{ fee_schedule: FeeSchedule }>('/api/v1/billing/fee-schedules', input, orgHeaders(orgId))
  return res.fee_schedule
}

export async function archiveFeeSchedule(api: ApiClient, orgId: string, id: string) {
  await api.delete(`/api/v1/billing/fee-schedules/${id}`, orgHeaders(orgId))
}

export async function fetchDiscountRules(api: ApiClient, orgId: string) {
  const res = await api.get<{ discount_rules: DiscountRule[] }>('/api/v1/billing/discount-rules', undefined, orgHeaders(orgId))
  return res.discount_rules ?? []
}

export async function saveDiscountRule(api: ApiClient, orgId: string, input: DiscountRuleInput, id?: string) {
  const res = id
    ? await api.put<{ discount_rule: DiscountRule }>(`/api/v1/billing/discount-rules/${id}`, input, orgHeaders(orgId))
    : await api.post<{ discount_rule: DiscountRule }>('/api/v1/billing/discount-rules', input, orgHeaders(orgId))
  return res.discount_rule
}

export async function archiveDiscountRule(api: ApiClient, orgId: string, id: string) {
  await api.delete(`/api/v1/billing/discount-rules/${id}`, orgHeaders(orgId))
}

export async function fetchAdjustments(api: ApiClient, orgId: string, status = 'pending') {
  return fetchAllPages<BillingAdjustment>(api, `/api/v1/billing/adjustments?status=${encodeURIComponent(status)}`, 'adjustments', orgId)
}

export async function createAdjustment(api: ApiClient, orgId: string, input: { household_id: string; amount_cents: number; reason: string }) {
  const res = await api.post<{ adjustment: BillingAdjustment }>('/api/v1/billing/adjustments', input, orgHeaders(orgId))
  return res.adjustment
}

export async function voidAdjustment(api: ApiClient, orgId: string, id: string) {
  await api.post(`/api/v1/billing/adjustments/${id}/void`, undefined, orgHeaders(orgId))
}

export async function fetchBillingSettings(api: ApiClient, orgId: string) {
  return (await api.get<{ settings: BillingSettings }>('/api/v1/billing/settings', undefined, orgHeaders(orgId))).settings
}

export async function saveBillingSettings(api: ApiClient, orgId: string, settings: BillingSettings) {
  return (await api.put<{ settings: BillingSettings }>('/api/v1/billing/settings', settings, orgHeaders(orgId))).settings
}

export async function previewBillingRun(api: ApiClient, orgId: string, input: RunInput) {
  return (await api.post<{ preview: RunPreview }>('/api/v1/billing/runs/preview', input, orgHeaders(orgId))).preview
}

export async function commitBillingRun(api: ApiClient, orgId: string, input: RunInput) {
  return api.post<RunResult>('/api/v1/billing/runs', input, orgHeaders(orgId))
}

export async function fetchBillingRuns(api: ApiClient, orgId: string) {
  return fetchAllPages<BillingRun>(api, '/api/v1/billing/runs', 'runs', orgId)
}

export async function applyLateFeesNow(api: ApiClient, orgId: string) {
  return (await api.post<{ result: { applied_count: number; total_cents: number; enabled: boolean } }>('/api/v1/billing/late-fees/apply', undefined, orgHeaders(orgId))).result
}

export async function fetchAgingReport(api: ApiClient, orgId: string, asOf: string) {
  return (await api.get<{ report: AgingReport }>(`/api/v1/billing/reports/aging?as_of=${asOf}`, undefined, orgHeaders(orgId))).report
}

export async function fetchRevenueReport(api: ApiClient, orgId: string, from: string, to: string) {
  return (await api.get<{ report: RevenueReport }>(`/api/v1/billing/reports/revenue?from=${from}&to=${to}`, undefined, orgHeaders(orgId))).report
}

export async function fetchDepositsReport(api: ApiClient, orgId: string, from: string, to: string, period: string) {
  return (await api.get<{ report: DepositsReport }>(`/api/v1/billing/reports/deposits?from=${from}&to=${to}&period=${period}`, undefined, orgHeaders(orgId))).report
}

export async function fetchStatement(api: ApiClient, orgId: string, householdId: string) {
  return (await api.get<{ statement: Statement }>(`/api/v1/households/${householdId}/statement`, undefined, orgHeaders(orgId))).statement
}

export async function fetchUpcomingCharges(api: ApiClient, orgId: string, householdId: string) {
  return (await api.get<{ upcoming: UpcomingCharges }>(`/api/v1/households/${householdId}/upcoming-charges?days=90`, undefined, orgHeaders(orgId))).upcoming
}

/** Download a CSV report through a raw fetch (keeps CSRF/auth headers). */
export async function downloadFinanceCsv(token: string | undefined, orgId: string, path: string, filename: string) {
  const res = await fetch(path, {
    method: 'GET',
    credentials: 'include',
    headers: { 'X-Org-Id': orgId, ...rawRequestAuthHeaders(token, 'GET') },
  })
  if (!res.ok) throw new ApiError(res.status, 'download_failed', 'The export could not be generated.')
  const url = URL.createObjectURL(await res.blob())
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

// ─── Pure helpers (unit-tested) ──────────────────────────────────────────────

export function formatCents(cents: number | undefined | null): string {
  const value = typeof cents === 'number' ? cents : 0
  const sign = value < 0 ? '−' : ''
  return `${sign}$${(Math.abs(value) / 100).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

/** Parse a dollar string ("12.50", "$1,200") into integer cents; null if invalid. */
export function parseDollarsToCents(input: string): number | null {
  const cleaned = input.replace(/[$,\s]/g, '')
  if (!/^-?\d+(\.\d{0,2})?$/.test(cleaned)) return null
  const negative = cleaned.startsWith('-')
  const [whole, frac = ''] = cleaned.replace('-', '').split('.')
  const cents = Number(whole) * 100 + Number((frac + '00').slice(0, 2))
  return negative ? -cents : cents
}

export function describeDiscount(rule: Pick<DiscountRule, 'discount_type' | 'percent_bps' | 'amount_cents'>): string {
  if (rule.discount_type === 'percent') return `${(rule.percent_bps / 100).toFixed(rule.percent_bps % 100 ? 2 : 0)}% off`
  return `${formatCents(rule.amount_cents)} off`
}

export function describeTier(rule: Pick<DiscountRule, 'min_position' | 'max_position'>): string {
  const ord = (n: number) => {
    const suffix = n % 100 >= 11 && n % 100 <= 13 ? 'th' : ({ 1: 'st', 2: 'nd', 3: 'rd' } as Record<number, string>)[n % 10] ?? 'th'
    return `${n}${suffix}`
  }
  if (rule.max_position == null) return `${ord(rule.min_position)}+ swimmer`
  if (rule.max_position === rule.min_position) return `${ord(rule.min_position)} swimmer`
  return `${ord(rule.min_position)}–${ord(rule.max_position)} swimmers`
}

export const AGING_BUCKETS: { key: keyof Omit<AgingBuckets, 'total_cents'>; label: string }[] = [
  { key: 'current_cents', label: 'Current' },
  { key: 'days_1_30_cents', label: '1–30 days' },
  { key: 'days_31_60_cents', label: '31–60 days' },
  { key: 'days_61_90_cents', label: '61–90 days' },
  { key: 'days_90_plus_cents', label: '90+ days' },
]

/** Share (0–1) of each aging bucket; zero total yields all zeros. */
export function agingShares(totals: AgingBuckets): number[] {
  return AGING_BUCKETS.map((b) => (totals.total_cents > 0 ? totals[b.key] / totals.total_cents : 0))
}

export function financeErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    if (error.code === 'nothing_to_bill') return 'Every eligible charge for this date is already billed. Nothing new to invoice.'
    if (error.code === 'preview_stale') return 'The roster or schedules changed since this preview. Review the refreshed preview, then commit again.'
    return uiErrorMessage(error, fallback)
  }
  return fallback
}
