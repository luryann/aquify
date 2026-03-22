'use client'

import type { ApiClient } from './api'

export type HouseholdRecord = {
  id: string
  org_id: string
  household_name: string
  primary_email?: string
  primary_phone_e164?: string
  status: string
  notes?: string
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

export type ContactRecord = {
  id: string
  org_id: string
  household_id: string
  relationship: string
  first_name: string
  last_name: string
  email?: string
  phone_e164?: string
  is_primary: boolean
  is_emergency_contact: boolean
  can_pick_up: boolean
  emergency_priority?: number
  notes?: string
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

export type SwimmerRecord = {
  id: string
  org_id: string
  household_id: string
  first_name: string
  last_name: string
  preferred_name?: string
  date_of_birth: string
  gender: string
  swimmer_external_id?: string
  registration_status: string
  medical_flags?: string[]
  medical_flags_redacted?: boolean
  notes?: string
}

export type SwimmerGroupLinkRecord = {
  id: string
  org_id: string
  swimmer_id: string
  group_code: string
  is_primary: boolean
  assigned_at?: string
}

export type GroupRecord = {
  id: string
  org_id: string
  code: string
  name: string
  description?: string
  type: string
  min_age?: number
  max_age?: number
  coach_user_id?: string
  is_active: boolean
  created_at: string
}

export type RegistrationRecord = {
  id: string
  org_id: string
  household_id: string
  swimmer_id: string
  group_id: string
  season: string
  status: string
  registration_date: string
  notes?: string
}

export type EligibilityRuleRecord = {
  id: string
  org_id: string
  group_id: string
  rule_type: string
  parameters: Record<string, unknown>
  is_active: boolean
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

export type MeetRecord = {
  id: string
  org_id: string
  name: string
  meet_type: string
  course: string
  start_date?: string
  end_date?: string
  entry_deadline?: string
  commitment_deadline?: string
  host_org?: string
  location?: string
  status: string
}

export type CommitmentRecord = {
  id: string
  meet_id: string
  swimmer_id: string
  status: string
}

export type MeetEventRecord = {
  id: string
  meet_id: string
  session_id?: string
  event_number?: number
  gender?: string
  age_group?: string
  distance?: number
  stroke?: string
  entry_limit?: number
  requires_qualification?: boolean
  qualifying_time_cents?: number
}

export type EntryRecord = {
  id: string
  meet_id: string
  swimmer_id: string
  status: string
  meet_event_id?: string
  seed_time_cents?: number
  entry_time_cents?: number
  override_reason?: string
}

export type MeetSessionRecord = {
  id: string
  org_id: string
  meet_id: string
  name: string
  session_order: number
  scheduled_date?: string
  warmup_at?: string
  start_at?: string
  created_at: string
  updated_at: string
}

export type ImportRecord = {
  id: string
  org_id: string
  meet_id?: string
  filename: string
  file_type: string
  status: string
  row_count: number
  error_count: number
  conflict_count: number
  declared_size_bytes?: number
  created_at: string
  created_by: string
}

export type ImportErrorRecord = {
  row_number?: number
  error_code: string
  error_message: string
}

export type ImportReportRecord = {
  total_rows: number
  matched_rows: number
  error_rows: number
  conflict_rows: number
  errors?: ImportErrorRecord[]
}

export type ImportConflictRecord = {
  id: string
  import_id: string
  org_id: string
  row_number?: number
  raw_data?: string
  status: string
  conflict_type: string
  candidate_swimmer_ids: string[]
  resolved_swimmer_id?: string
  resolver_id?: string
  resolution_note?: string
  resolved_at?: string
  created_at: string
  updated_at: string
}

export type ExportRecord = {
  id: string
  org_id: string
  meet_id: string
  export_type: string
  status: string
  filename: string
  size_bytes?: number
  download_url?: string
  expires_at?: string
  last_error?: string
  created_at: string
  created_by: string
}

export type InvoiceRecord = {
  id: string
  org_id: string
  household_id?: string
  invoice_number: string
  status: string
  total_cents: number
  amount_paid_cents: number
  amount_due_cents: number
  due_date?: string
  issued_at?: string
  created_at: string
  notes?: string
}

export type InvoiceItemRecord = {
  id: string
  org_id: string
  invoice_id: string
  description: string
  quantity: number
  unit_amount_cents: number
  total_cents: number
  category: string
  swimmer_id?: string
  meet_id?: string
  created_at: string
  updated_at: string
}

export type PaymentRecord = {
  id: string
  org_id: string
  invoice_id: string
  amount_cents: number
  method: string
  status: string
  stripe_payment_intent_id?: string
  paid_at?: string
  failed_at?: string
  external_ref?: string
  notes?: string
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

export type RefundRecord = {
  id: string
  org_id: string
  invoice_id: string
  payment_id: string
  amount_cents: number
  reason: string
  status: string
  stripe_refund_id?: string
  processed_at?: string
  rejected_at?: string
  requested_by?: string
  approved_by?: string
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

export type BestTimeRecord = {
  id: string
  org_id: string
  swimmer_id: string
  event_distance: number
  stroke: string
  course: string
  best_time_ms?: number | null
  result_id?: string
  recorded_at?: string
  is_manual_override: boolean
  override_reason?: string
  override_by?: string
  created_at: string
  updated_at: string
}

export type ProgressionResultRecord = {
  id: string
  org_id: string
  swimmer_id: string
  meet_id?: string
  meet_event_id?: string
  import_id?: string
  event_number?: string
  distance: number
  stroke: string
  course: string
  gender: string
  fin_time_ms?: number | null
  is_official: boolean
  is_manual_override: boolean
  override_reason?: string
  created_at: string
  updated_at: string
}

export type RoleAssignmentRecord = {
  org_id: string
  target_user_id: string
  role: string
  scope_type: string
  scope_id?: string
  reason: string
  actor_user_id: string
  assigned_at: string
}

export type MFAFactorRecord = {
  id: string
  user_id: string
  org_id: string
  factor_type: string
  label: string
  is_verified: boolean
  created_at: string
  last_challenged_at?: string
}

export type RuntimeSettingsRecord = {
  app_env?: string
  auth?: Record<string, unknown>
  workers?: Record<string, unknown>
  compliance?: Record<string, unknown>
  storage?: Record<string, unknown>
  stripe?: Record<string, unknown>
  postmark?: Record<string, unknown>
  telemetry?: Record<string, unknown>
}

export type LedgerEventRecord = {
  id: string
  org_id: string
  invoice_id?: string
  payment_id?: string
  refund_id?: string
  event_type: string
  amount_cents: number
  description?: string
  actor_id?: string
  created_at: string
}

export type ReportArtifactRecord = {
  id: string
  org_id: string
  kind: string
  filename: string
  content_type: string
  size_bytes: number
  storage_key: string
  download_url?: string
  expires_at: string
  created_at: string
}

export type MembershipStatusReportRow = {
  household_id: string
  household_name: string
  household_status: string
  primary_email?: string
  primary_phone_e164?: string
  swimmer_id: string
  swimmer_name: string
  preferred_name?: string
  registration_status: string
  primary_group_code?: string
  primary_group_name?: string
  group_count: number
  last_profile_update_at: string
}

export type MembershipStatusReportRecord = {
  generated_at: string
  row_count: number
  rows: MembershipStatusReportRow[]
}

export type MeetFeesReportRow = {
  invoice_id: string
  invoice_number: string
  invoice_status: string
  household_id?: string
  meet_id?: string
  meet_name?: string
  swimmer_id?: string
  swimmer_name?: string
  line_item_id: string
  line_description: string
  quantity: number
  unit_amount_cents: number
  total_cents: number
  due_date?: string
  issued_at?: string
}

export type MeetFeesReportRecord = {
  generated_at: string
  row_count: number
  total_fees_cents: number
  rows: MeetFeesReportRow[]
}

export type InvoiceAgingReportRow = {
  invoice_id: string
  invoice_number: string
  household_id: string
  status: string
  due_date: string
  days_past_due: number
  aging_bucket: string
  total_cents: number
  amount_paid_cents: number
  amount_due_cents: number
  is_outstanding: boolean
}

export type InvoiceAgingReportRecord = {
  generated_at: string
  as_of_date: string
  row_count: number
  outstanding_count: number
  total_outstanding_cents: number
  rows: InvoiceAgingReportRow[]
}

export type PaymentReconciliationReportRow = {
  invoice_id: string
  invoice_number: string
  payment_id: string
  method: string
  payment_status: string
  payment_amount_cents: number
  refunded_cents: number
  net_collected_cents: number
  ledger_payment_cents: number
  ledger_refund_cents: number
  is_reconciled: boolean
  reconciliation_notes: string
}

export type PaymentReconciliationReportRecord = {
  generated_at: string
  row_count: number
  payment_total_cents: number
  refund_total_cents: number
  net_collected_cents: number
  unreconciled_row_count: number
  stripe_reported_cents?: number
  stripe_mismatch_cents?: number
  stripe_alert_triggered?: boolean
  rows: PaymentReconciliationReportRow[]
}

export type RegistrationStatusReportRow = {
  registration_id: string
  season: string
  status: string
  household_id: string
  household_name: string
  swimmer_id: string
  swimmer_name: string
  group_id: string
  group_code?: string
  group_name?: string
  registration_date: string
  waiver_accepted: boolean
}

export type RegistrationStatusReportRecord = {
  generated_at: string
  row_count: number
  rows: RegistrationStatusReportRow[]
}

export type AuditEventRecord = {
  id: string
  org_id: string
  actor_user_id: string
  actor_roles: string[]
  action: string
  resource_type: string
  resource_id: string
  before_state?: unknown
  after_state?: unknown
  metadata?: Record<string, string>
  occurred_at: string
}

export type WorkerTaskRecord = {
  id: string
  type: string
  queue: string
  status: string
  enqueued_at: string
  process_at?: string
  completed_at?: string
  retried: number
  max_retries: number
  last_error?: string
}

export type QueueStatsRecord = {
  queue: string
  pending: number
  active: number
  retrying: number
  completed: number
  failed: number
  dead_letter: number
}

export type JobMetricsTypeRecord = {
  enqueued: number
  succeeded: number
  failed: number
  dlq: number
}

export type JobExecutionRecord = {
  task_id: string
  task_type: string
  queue: string
  status: string
  started_at: string
  finished_at?: string
  error?: string
}

export type AnnouncementRecord = {
  id: string
  org_id: string
  title: string
  body: string
  status: string
  audience: string
  group_id?: string
  published_at?: string
  archived_at?: string
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

export type EmailCampaignRecord = {
  id: string
  org_id: string
  name: string
  subject: string
  body_html: string
  body_text?: string
  from_name: string
  from_email: string
  audience: string
  group_id?: string
  status: string
  sent_at?: string
  total_recipients: number
  sent_count: number
  failed_count: number
  suppressed_count: number
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

export type EmailDeliveryRecord = {
  id: string
  org_id: string
  campaign_id?: string
  recipient_email: string
  recipient_user_id?: string
  status: string
  provider_message_id?: string
  error_message?: string
  sent_at?: string
  delivered_at?: string
  bounced_at?: string
  created_at: string
  updated_at: string
}

export type SendCampaignRecipientInput = {
  email: string
  user_id?: string
}

export type SuppressionRecord = {
  id: string
  org_id: string
  email: string
  reason: string
  created_at: string
  created_by: string
}

export type DataExportRequestRecord = {
  id: string
  org_id: string
  household_id: string
  requested_by: string
  status: string
  download_url?: string
  requested_at: string
  completed_at?: string
  expires_at?: string
  created_at: string
  updated_at: string
}

export type DataDeletionRequestRecord = {
  id: string
  org_id: string
  household_id: string
  requested_by: string
  status: string
  reason?: string
  review_notes?: string
  requested_at: string
  completed_at?: string
  expires_at?: string
  created_at: string
  updated_at: string
}

export type TimeStandardRecord = {
  id: string
  org_id: string
  name: string
  season_year: number
  standard_code: string
  description?: string
  event_distance: number
  stroke: string
  course: string
  gender: string
  age_min?: number
  age_max?: number
  time_ms: number
  is_active: boolean
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

function withOrgHeaders(orgId: string) {
  return { 'X-Org-Id': orgId }
}

export async function fetchHouseholds(api: ApiClient, orgId: string): Promise<HouseholdRecord[]> {
  const res = await api.get<{ households?: HouseholdRecord[] }>('/api/v1/members/households', undefined, withOrgHeaders(orgId))
  return res.households ?? []
}

export async function fetchHousehold(api: ApiClient, orgId: string, householdId: string): Promise<HouseholdRecord> {
  const res = await api.get<{ household: HouseholdRecord }>(`/api/v1/members/households/${householdId}`, undefined, withOrgHeaders(orgId))
  return res.household
}

export async function fetchHouseholdContacts(api: ApiClient, orgId: string, householdId: string): Promise<ContactRecord[]> {
  const res = await api.get<{ contacts?: ContactRecord[] }>(`/api/v1/members/households/${householdId}/contacts`, undefined, withOrgHeaders(orgId))
  return res.contacts ?? []
}

export async function fetchSwimmers(api: ApiClient, orgId: string): Promise<SwimmerRecord[]> {
  const res = await api.get<{ swimmers?: SwimmerRecord[] }>(`/api/v1/members/swimmers`, undefined, withOrgHeaders(orgId))
  return res.swimmers ?? []
}

export async function fetchSwimmer(api: ApiClient, orgId: string, swimmerId: string): Promise<SwimmerRecord> {
  const res = await api.get<{ swimmer: SwimmerRecord }>(`/api/v1/members/swimmers/${swimmerId}`, undefined, withOrgHeaders(orgId))
  return res.swimmer
}

export async function fetchSwimmerGroups(api: ApiClient, orgId: string): Promise<SwimmerGroupLinkRecord[]> {
  const res = await api.get<{ swimmer_groups?: SwimmerGroupLinkRecord[] }>('/api/v1/members/swimmer-groups', undefined, withOrgHeaders(orgId))
  return res.swimmer_groups ?? []
}

export async function fetchGroups(api: ApiClient, orgId: string): Promise<GroupRecord[]> {
  const res = await api.get<{ groups?: GroupRecord[] }>('/api/v1/groups', undefined, withOrgHeaders(orgId))
  return res.groups ?? []
}

export async function fetchGroup(api: ApiClient, orgId: string, groupId: string): Promise<GroupRecord> {
  const res = await api.get<{ group: GroupRecord }>(`/api/v1/groups/${groupId}`, undefined, withOrgHeaders(orgId))
  return res.group
}

export async function fetchGroupEligibilityRules(api: ApiClient, orgId: string, groupId: string): Promise<EligibilityRuleRecord[]> {
  const res = await api.get<{ rules?: EligibilityRuleRecord[] }>(`/api/v1/groups/${groupId}/eligibility-rules`, undefined, withOrgHeaders(orgId))
  return res.rules ?? []
}

export async function fetchRegistrations(api: ApiClient, orgId: string): Promise<RegistrationRecord[]> {
  const res = await api.get<{ registrations?: RegistrationRecord[] }>('/api/v1/registrations', undefined, withOrgHeaders(orgId))
  return res.registrations ?? []
}

export async function fetchMeets(api: ApiClient, orgId: string): Promise<MeetRecord[]> {
  const res = await api.get<{ meets?: MeetRecord[] }>('/api/v1/meets', undefined, withOrgHeaders(orgId))
  return res.meets ?? []
}

export async function fetchMeet(api: ApiClient, orgId: string, meetId: string): Promise<MeetRecord> {
  const res = await api.get<{ meet: MeetRecord }>(`/api/v1/meets/${meetId}`, undefined, withOrgHeaders(orgId))
  return res.meet
}

export async function fetchMeetSessions(api: ApiClient, orgId: string, meetId: string): Promise<MeetSessionRecord[]> {
  const res = await api.get<{ sessions?: MeetSessionRecord[] }>(`/api/v1/meets/${meetId}/sessions`, undefined, withOrgHeaders(orgId))
  return res.sessions ?? []
}

export async function fetchMeetCommitments(api: ApiClient, orgId: string, meetId: string): Promise<CommitmentRecord[]> {
  const res = await api.get<{ commitments?: CommitmentRecord[] }>(`/api/v1/meets/${meetId}/commitments`, undefined, withOrgHeaders(orgId))
  return res.commitments ?? []
}

export async function fetchMeetEvents(api: ApiClient, orgId: string, meetId: string): Promise<MeetEventRecord[]> {
  const res = await api.get<{ events?: MeetEventRecord[] }>(`/api/v1/meets/${meetId}/events`, undefined, withOrgHeaders(orgId))
  return res.events ?? []
}

export async function fetchMeetEntries(api: ApiClient, orgId: string, meetId: string): Promise<EntryRecord[]> {
  const res = await api.get<{ entries?: EntryRecord[] }>(`/api/v1/meets/${meetId}/entries`, undefined, withOrgHeaders(orgId))
  return res.entries ?? []
}

export async function fetchImports(api: ApiClient, orgId: string): Promise<ImportRecord[]> {
  const res = await api.get<{ imports?: ImportRecord[] }>('/api/v1/imports', undefined, withOrgHeaders(orgId))
  return res.imports ?? []
}

export async function fetchImport(api: ApiClient, orgId: string, importId: string): Promise<ImportRecord> {
  const res = await api.get<{ import: ImportRecord }>(`/api/v1/imports/${importId}`, undefined, withOrgHeaders(orgId))
  return res.import
}

export async function fetchImportConflicts(api: ApiClient, orgId: string, importId: string): Promise<ImportConflictRecord[]> {
  const res = await api.get<{ conflicts?: ImportConflictRecord[] }>(`/api/v1/imports/${importId}/conflicts`, undefined, withOrgHeaders(orgId))
  return res.conflicts ?? []
}

export async function fetchImportReport(api: ApiClient, orgId: string, importId: string): Promise<ImportReportRecord | null> {
  const res = await api.get<{ import?: { import_report?: ImportReportRecord | null } }>(`/api/v1/imports/${importId}/report`, undefined, withOrgHeaders(orgId))
  return res.import?.import_report ?? null
}

export async function fetchExportsForMeet(api: ApiClient, orgId: string, meetId: string): Promise<ExportRecord[]> {
  const res = await api.get<{ exports?: ExportRecord[] }>(`/api/v1/meets/${meetId}/exports`, undefined, withOrgHeaders(orgId))
  return res.exports ?? []
}

export async function fetchInvoicesForHousehold(api: ApiClient, orgId: string, householdId: string): Promise<InvoiceRecord[]> {
  const res = await api.get<{ invoices?: InvoiceRecord[] }>(`/api/v1/households/${householdId}/invoices`, undefined, withOrgHeaders(orgId))
  return res.invoices ?? []
}

export async function fetchInvoices(api: ApiClient, orgId: string, params: Record<string, string> = {}): Promise<InvoiceRecord[]> {
  const query = new URLSearchParams(params).toString()
  const res = await api.get<{ invoices?: InvoiceRecord[] }>(`/api/v1/invoices${query ? `?${query}` : ''}`, undefined, withOrgHeaders(orgId))
  return res.invoices ?? []
}

export async function fetchInvoice(api: ApiClient, orgId: string, invoiceId: string): Promise<InvoiceRecord> {
  const res = await api.get<{ invoice: InvoiceRecord }>(`/api/v1/invoices/${invoiceId}`, undefined, withOrgHeaders(orgId))
  return res.invoice
}

export async function fetchInvoiceItems(api: ApiClient, orgId: string, invoiceId: string): Promise<InvoiceItemRecord[]> {
  const res = await api.get<{ items?: InvoiceItemRecord[] }>(`/api/v1/invoices/${invoiceId}/items`, undefined, withOrgHeaders(orgId))
  return res.items ?? []
}

export async function fetchInvoicePayments(api: ApiClient, orgId: string, invoiceId: string): Promise<PaymentRecord[]> {
  const res = await api.get<{ payments?: PaymentRecord[] }>(`/api/v1/invoices/${invoiceId}/payments`, undefined, withOrgHeaders(orgId))
  return res.payments ?? []
}

export async function fetchPayments(api: ApiClient, orgId: string, params: Record<string, string> = {}): Promise<PaymentRecord[]> {
  const query = new URLSearchParams(params).toString()
  const res = await api.get<{ payments?: PaymentRecord[] }>(`/api/v1/payments${query ? `?${query}` : ''}`, undefined, withOrgHeaders(orgId))
  return res.payments ?? []
}

export async function fetchInvoiceRefunds(api: ApiClient, orgId: string, invoiceId: string): Promise<RefundRecord[]> {
  const res = await api.get<{ refunds?: RefundRecord[] }>(`/api/v1/invoices/${invoiceId}/refunds`, undefined, withOrgHeaders(orgId))
  return res.refunds ?? []
}

export async function fetchSwimmerBestTimes(api: ApiClient, orgId: string, swimmerId: string): Promise<BestTimeRecord[]> {
  const res = await api.get<{ best_times?: BestTimeRecord[] }>(`/api/v1/swimmers/${swimmerId}/best-times`, undefined, withOrgHeaders(orgId))
  return res.best_times ?? []
}

export async function fetchSwimmerProgression(api: ApiClient, orgId: string, swimmerId: string): Promise<ProgressionResultRecord[]> {
  const res = await api.get<{ results?: ProgressionResultRecord[] }>(`/api/v1/swimmers/${swimmerId}/progression`, undefined, withOrgHeaders(orgId))
  return res.results ?? []
}

export async function fetchPerformanceBestTimes(api: ApiClient, orgId: string, params: Record<string, string> = {}): Promise<BestTimeRecord[]> {
  const query = new URLSearchParams(params).toString()
  const res = await api.get<{ best_times?: BestTimeRecord[] }>(`/api/v1/performance/best-times${query ? `?${query}` : ''}`, undefined, withOrgHeaders(orgId))
  return res.best_times ?? []
}

export async function fetchPerformanceProgression(api: ApiClient, orgId: string, params: Record<string, string> = {}): Promise<ProgressionResultRecord[]> {
  const query = new URLSearchParams(params).toString()
  const res = await api.get<{ results?: ProgressionResultRecord[] }>(`/api/v1/performance/progression${query ? `?${query}` : ''}`, undefined, withOrgHeaders(orgId))
  return res.results ?? []
}

export async function fetchLedgerEvents(api: ApiClient, orgId: string): Promise<LedgerEventRecord[]> {
  const res = await api.get<{ events?: LedgerEventRecord[] }>('/api/v1/ledger-events', undefined, withOrgHeaders(orgId))
  return res.events ?? []
}

export async function fetchLedgerEventsForInvoice(api: ApiClient, orgId: string, invoiceId: string): Promise<LedgerEventRecord[]> {
  const query = new URLSearchParams({ invoice_id: invoiceId }).toString()
  const res = await api.get<{ events?: LedgerEventRecord[] }>(`/api/v1/ledger-events?${query}`, undefined, withOrgHeaders(orgId))
  return res.events ?? []
}

export async function fetchRoleAssignments(api: ApiClient, orgId: string): Promise<RoleAssignmentRecord[]> {
  const res = await api.get<{ assignments?: RoleAssignmentRecord[] }>('/api/v1/auth/roles/assignments', undefined, withOrgHeaders(orgId))
  return res.assignments ?? []
}

export async function assignRole(api: ApiClient, orgId: string, input: {
  target_user_id: string
  role: string
  reason: string
  scope_type?: string
  scope_id?: string
}): Promise<RoleAssignmentRecord> {
  const res = await api.post<{ assignment: RoleAssignmentRecord }>('/api/v1/auth/roles/assign', {
    org_id: orgId,
    ...input,
  }, withOrgHeaders(orgId))
  return res.assignment
}

export async function fetchMFAFactors(api: ApiClient): Promise<MFAFactorRecord[]> {
  const res = await api.get<{ factors?: MFAFactorRecord[] }>('/api/v1/auth/mfa/factors')
  return res.factors ?? []
}

export async function fetchRuntimeSettings(api: ApiClient, orgId: string): Promise<RuntimeSettingsRecord> {
  const res = await api.get<{ settings: RuntimeSettingsRecord }>('/api/v1/settings/runtime', undefined, withOrgHeaders(orgId))
  return res.settings
}

export async function fetchMembershipStatusReport(api: ApiClient, orgId: string): Promise<MembershipStatusReportRecord> {
  const res = await api.get<{ report: MembershipStatusReportRecord }>('/api/v1/reports/membership-status', undefined, withOrgHeaders(orgId))
  return res.report
}

export async function fetchMeetFeesReport(api: ApiClient, orgId: string): Promise<MeetFeesReportRecord> {
  const res = await api.get<{ report: MeetFeesReportRecord }>('/api/v1/reports/meet-fees', undefined, withOrgHeaders(orgId))
  return res.report
}

export async function fetchInvoiceAgingReport(api: ApiClient, orgId: string, asOf?: string): Promise<InvoiceAgingReportRecord> {
  const query = asOf ? `?${new URLSearchParams({ as_of: asOf }).toString()}` : ''
  const res = await api.get<{ report: InvoiceAgingReportRecord }>(`/api/v1/reports/invoice-aging${query}`, undefined, withOrgHeaders(orgId))
  return res.report
}

export async function fetchPaymentReconciliationReport(api: ApiClient, orgId: string, stripeReportedCents?: number): Promise<PaymentReconciliationReportRecord> {
  const params = new URLSearchParams()
  if (typeof stripeReportedCents === 'number') params.set('stripe_reported_cents', String(stripeReportedCents))
  const query = params.toString()
  const res = await api.get<{ report: PaymentReconciliationReportRecord }>(`/api/v1/reports/payment-reconciliation${query ? `?${query}` : ''}`, undefined, withOrgHeaders(orgId))
  return res.report
}

export async function fetchRegistrationStatusReport(api: ApiClient, orgId: string): Promise<RegistrationStatusReportRecord> {
  const res = await api.get<{ report: RegistrationStatusReportRecord }>('/api/v1/reports/registration-status', undefined, withOrgHeaders(orgId))
  return res.report
}

export async function downloadReportArtifact(api: ApiClient, orgId: string, path: string): Promise<ReportArtifactRecord | null> {
  const res = await api.get<{ artifact?: ReportArtifactRecord }>(path, undefined, withOrgHeaders(orgId))
  return res.artifact ?? null
}

export async function fetchAuditEvents(api: ApiClient, orgId: string, params: Record<string, string> = {}): Promise<{ events: AuditEventRecord[]; next_cursor?: string }> {
  const query = new URLSearchParams(params).toString()
  return api.get<{ events: AuditEventRecord[]; next_cursor?: string }>(`/api/v1/audit-events${query ? `?${query}` : ''}`, undefined, withOrgHeaders(orgId))
}

export async function fetchJobs(api: ApiClient): Promise<WorkerTaskRecord[]> {
  const res = await api.get<{ tasks?: WorkerTaskRecord[] }>('/api/v1/jobs')
  return res.tasks ?? []
}

export async function fetchQueueStats(api: ApiClient): Promise<QueueStatsRecord[]> {
  const res = await api.get<{ queues?: QueueStatsRecord[] }>('/api/v1/jobs/queues')
  return res.queues ?? []
}

export async function fetchJobMetrics(api: ApiClient): Promise<{ by_type: Record<string, JobMetricsTypeRecord>; recent_executions: JobExecutionRecord[] }> {
  return api.get<{ by_type: Record<string, JobMetricsTypeRecord>; recent_executions: JobExecutionRecord[] }>('/api/v1/jobs/metrics')
}

export async function fetchDLQTasks(api: ApiClient): Promise<WorkerTaskRecord[]> {
  const res = await api.get<{ dead_letter_tasks?: WorkerTaskRecord[] }>('/api/v1/jobs/dlq')
  return res.dead_letter_tasks ?? []
}

export async function retryDLQTask(api: ApiClient, taskId: string): Promise<void> {
  await api.post(`/api/v1/jobs/dlq/${taskId}/retry`)
}

export async function deleteDLQTask(api: ApiClient, taskId: string): Promise<void> {
  await api.delete(`/api/v1/jobs/dlq/${taskId}`)
}

export async function fetchAnnouncements(api: ApiClient, orgId: string, status?: string): Promise<AnnouncementRecord[]> {
  const query = status ? `?${new URLSearchParams({ status }).toString()}` : ''
  const res = await api.get<{ announcements?: AnnouncementRecord[] }>(`/api/v1/announcements${query}`, undefined, withOrgHeaders(orgId))
  return res.announcements ?? []
}

export async function fetchAnnouncement(api: ApiClient, orgId: string, announcementId: string): Promise<AnnouncementRecord> {
  return api.get<AnnouncementRecord>(`/api/v1/announcements/${announcementId}`, undefined, withOrgHeaders(orgId))
}

export async function createAnnouncement(api: ApiClient, orgId: string, input: {
  title: string
  body: string
  audience: string
  group_id?: string
}): Promise<AnnouncementRecord> {
  return api.post<AnnouncementRecord>('/api/v1/announcements', input, withOrgHeaders(orgId))
}

export async function publishAnnouncement(api: ApiClient, orgId: string, announcementId: string): Promise<AnnouncementRecord> {
  return api.post<AnnouncementRecord>(`/api/v1/announcements/${announcementId}/publish`, undefined, withOrgHeaders(orgId))
}

export async function archiveAnnouncement(api: ApiClient, orgId: string, announcementId: string): Promise<AnnouncementRecord> {
  return api.post<AnnouncementRecord>(`/api/v1/announcements/${announcementId}/archive`, undefined, withOrgHeaders(orgId))
}

export async function fetchCampaigns(api: ApiClient, orgId: string): Promise<EmailCampaignRecord[]> {
  const res = await api.get<{ campaigns?: EmailCampaignRecord[] }>('/api/v1/email-campaigns', undefined, withOrgHeaders(orgId))
  return res.campaigns ?? []
}

export async function fetchCampaign(api: ApiClient, orgId: string, campaignId: string): Promise<EmailCampaignRecord> {
  return api.get<EmailCampaignRecord>(`/api/v1/email-campaigns/${campaignId}`, undefined, withOrgHeaders(orgId))
}

export async function createCampaign(api: ApiClient, orgId: string, input: {
  name: string
  subject: string
  body_html: string
  body_text?: string
  from_name: string
  from_email: string
  audience: string
  group_id?: string
}): Promise<EmailCampaignRecord> {
  return api.post<EmailCampaignRecord>('/api/v1/email-campaigns', input, withOrgHeaders(orgId))
}

export async function sendCampaign(api: ApiClient, orgId: string, campaignId: string, input: {
  recipients: SendCampaignRecipientInput[]
}): Promise<EmailCampaignRecord> {
  return api.post<EmailCampaignRecord>(`/api/v1/email-campaigns/${campaignId}/send`, input, withOrgHeaders(orgId))
}

export async function fetchCampaignDeliveries(api: ApiClient, orgId: string, campaignId: string): Promise<EmailDeliveryRecord[]> {
  const res = await api.get<{ deliveries?: EmailDeliveryRecord[] }>(`/api/v1/email-campaigns/${campaignId}/deliveries`, undefined, withOrgHeaders(orgId))
  return res.deliveries ?? []
}

export async function fetchSuppression(api: ApiClient, orgId: string): Promise<SuppressionRecord[]> {
  const res = await api.get<{ suppression?: SuppressionRecord[] }>('/api/v1/suppression', undefined, withOrgHeaders(orgId))
  return res.suppression ?? []
}

export async function createSuppression(api: ApiClient, orgId: string, input: { email: string; reason: string }): Promise<SuppressionRecord> {
  return api.post<SuppressionRecord>('/api/v1/suppression', input, withOrgHeaders(orgId))
}

export async function removeSuppression(api: ApiClient, orgId: string, suppressionId: string): Promise<void> {
  await api.delete(`/api/v1/suppression/${suppressionId}`, withOrgHeaders(orgId))
}

export async function fetchDataExportRequests(api: ApiClient, orgId: string): Promise<DataExportRequestRecord[]> {
  const res = await api.get<{ requests?: DataExportRequestRecord[] }>('/api/v1/compliance/data-export-requests', undefined, withOrgHeaders(orgId))
  return res.requests ?? []
}

export async function createDataExportRequest(api: ApiClient, orgId: string, householdId: string): Promise<DataExportRequestRecord> {
  return api.post<DataExportRequestRecord>('/api/v1/compliance/data-export-requests', { household_id: householdId }, withOrgHeaders(orgId))
}

export async function fetchDataDeletionRequests(api: ApiClient, orgId: string): Promise<DataDeletionRequestRecord[]> {
  const res = await api.get<{ requests?: DataDeletionRequestRecord[] }>('/api/v1/compliance/data-deletion-requests', undefined, withOrgHeaders(orgId))
  return res.requests ?? []
}

export async function createDataDeletionRequest(api: ApiClient, orgId: string, input: { household_id: string; reason: string }): Promise<DataDeletionRequestRecord> {
  return api.post<DataDeletionRequestRecord>('/api/v1/compliance/data-deletion-requests', input, withOrgHeaders(orgId))
}

export async function fetchTimeStandards(api: ApiClient, orgId: string, params: Record<string, string> = {}): Promise<TimeStandardRecord[]> {
  const query = new URLSearchParams(params).toString()
  const res = await api.get<{ time_standards?: TimeStandardRecord[] }>(`/api/v1/time-standards${query ? `?${query}` : ''}`, undefined, withOrgHeaders(orgId))
  return res.time_standards ?? []
}

export async function createTimeStandard(api: ApiClient, orgId: string, input: {
  name: string
  season_year: number
  standard_code: string
  description?: string
  event_distance: number
  stroke: string
  course: string
  gender: string
  age_min?: number
  age_max?: number
  time_ms: number
  is_active: boolean
}): Promise<TimeStandardRecord> {
  const res = await api.post<{ time_standard: TimeStandardRecord }>('/api/v1/time-standards', input, withOrgHeaders(orgId))
  return res.time_standard
}

export async function updateTimeStandard(api: ApiClient, orgId: string, standardId: string, input: {
  name?: string
  description?: string
  time_ms?: number
  is_active?: boolean
}): Promise<TimeStandardRecord> {
  const res = await api.put<{ time_standard: TimeStandardRecord }>(`/api/v1/time-standards/${standardId}`, input, withOrgHeaders(orgId))
  return res.time_standard
}

export async function deleteTimeStandard(api: ApiClient, orgId: string, standardId: string): Promise<void> {
  await api.delete(`/api/v1/time-standards/${standardId}`, withOrgHeaders(orgId))
}
