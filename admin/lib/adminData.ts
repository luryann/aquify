'use client'

import { ApiError, type ApiClient } from '#lib/api'

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
  waiver_accepted?: boolean
  waiver_count?: number
  waiver_types?: string[]
  last_waiver_accepted_at?: string
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
  self_signup_enabled?: boolean
  max_entries_per_swimmer?: number
  status: string
}

export type CommitmentRecord = {
  id: string
  meet_id: string
  swimmer_id: string
  status: string
  note?: string
}

export type MeetEventRecord = {
  id: string
  meet_id: string
  session_id?: string
  event_number?: string
  gender?: string
  age_group?: string
  min_age?: number
  max_age?: number
  distance?: number
  stroke?: string
  course?: string
  entry_fee_cents?: number
  is_relay?: boolean
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
  session_number: number
  session_order?: number
  session_date?: string
  scheduled_date?: string
  warmup_time?: string
  warmup_at?: string
  start_time?: string
  start_at?: string
  notes?: string
  created_at: string
  updated_at: string
}

export type ParsedMeetSessionDraft = {
  session_number: number
  section: string
  name: string
  session_date?: string
  warmup_time?: string
  start_time?: string
}

export type ParsedMeetEventDraftRecord = {
  session_number?: number
  event_number: string
  stroke: string
  distance: number
  course?: string
  gender?: string
  min_age?: number
  max_age?: number
  is_relay: boolean
}

export type ParsedMeetSignupSettingsRecord = {
  self_signup_enabled: boolean
  max_entries_per_swimmer: number
}

export type ParsedMeetFileRecord = {
  file_type: string
  source_file: string
  meet_name?: string
  course?: string
  location?: string
  start_date?: string
  end_date?: string
  sessions: ParsedMeetSessionDraft[]
  events: ParsedMeetEventDraftRecord[]
  signup_settings: ParsedMeetSignupSettingsRecord
  parse_warnings?: string[]
}

export type ImportRecord = {
  id: string
  org_id: string
  meet_id?: string
  filename: string
  file_type: string
  status: string
  storage_key?: string
  upload_url?: string
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

export type HouseholdAutopayStatusRecord = {
  household_id: string
  enabled: boolean
  payment_method_type?: string
  payment_method_brand?: string
  payment_method_last4?: string
  payment_method_label?: string
  payment_method_external_ref?: string
  payment_method_exp_month?: number
  payment_method_exp_year?: number
  trigger_days_before_due: number
  next_charge_date?: string
  next_charge_amount_cents?: number
  next_invoice_id?: string
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

export type SwimmerActivityRecord = {
  id: string
  type: string
  occurred_at: string
  title: string
  detail?: string
  status?: string
  source_id?: string
  meet_id?: string
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

export type DocumentTypeRecord = {
  id: string
  org_id: string
  name: string
  category: string
  description?: string
  is_required: boolean
  applies_to: string
  expiry_months?: number
  is_active: boolean
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

export type DocumentSubmissionRecord = {
  id: string
  org_id: string
  document_type_id: string
  document_type_name?: string
  household_id: string
  swimmer_id?: string
  status: string
  file_object_key?: string
  file_name?: string
  file_size_bytes?: number
  file_mime?: string
  notes?: string
  review_notes?: string
  submitted_at?: string
  verified_at?: string
  verified_by?: string
  expires_at?: string
  reminder_sent_at?: string
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
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

export type CommunicationPreferenceRecord = {
  event_id: string
  label: string
  description: string
  email_enabled: boolean
  push_enabled: boolean
}

export type CutQualificationRecord = {
  swimmer_id: string
  standard_id: string
  standard_code: string
  event_distance: number
  stroke: string
  course: string
  gender: string
  cut_time_ms: number
  best_time_ms?: number
  qualified: boolean
  margin_ms?: number
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
  try {
    const res = await api.get<{ swimmers?: SwimmerRecord[] }>(`/api/v1/members/swimmers`, undefined, withOrgHeaders(orgId))
    return res.swimmers ?? []
  } catch (error) {
    // Swimmer-role sessions are not allowed to list org-wide swimmers.
    // Treat that specific permission denial as "no scoped swimmers" so
    // swimmer portal pages can continue rendering.
    if (error instanceof ApiError && error.status === 403 && error.code === 'forbidden') {
      return []
    }
    throw error
  }
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

export async function transitionMeet(api: ApiClient, orgId: string, meetId: string, status: string): Promise<MeetRecord> {
  const res = await api.post<{ meet: MeetRecord }>(`/api/v1/meets/${meetId}/transition`, { status }, withOrgHeaders(orgId))
  return res.meet
}

export async function openMeetCommitments(api: ApiClient, orgId: string, meetId: string): Promise<MeetRecord> {
  const res = await api.post<{ meet: MeetRecord }>(`/api/v1/meets/${meetId}/commitment-request`, {}, withOrgHeaders(orgId))
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

export async function createMeetEntry(api: ApiClient, orgId: string, meetId: string, input: {
  swimmer_id: string
  meet_event_id: string
  commitment_id?: string
  seed_time_ms?: number
  is_relay?: boolean
}): Promise<EntryRecord> {
  const res = await api.post<{ entry: EntryRecord }>(
    `/api/v1/meets/${meetId}/entries`,
    input,
    withOrgHeaders(orgId),
  )
  return res.entry
}

export async function respondToMeetCommitment(api: ApiClient, orgId: string, meetId: string, commitmentId: string, input: {
  response: 'committed_yes' | 'committed_no'
  notes?: string
}): Promise<CommitmentRecord> {
  const res = await api.post<{ commitment: CommitmentRecord }>(
    `/api/v1/meets/${meetId}/commitments/${commitmentId}/respond`,
    input,
    withOrgHeaders(orgId),
  )
  return res.commitment
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

export async function fetchHouseholdAutopay(api: ApiClient, orgId: string, householdId: string): Promise<HouseholdAutopayStatusRecord> {
  const res = await api.get<{ autopay: HouseholdAutopayStatusRecord }>(
    `/api/v1/households/${householdId}/autopay`,
    undefined,
    withOrgHeaders(orgId),
  )
  return res.autopay
}

export async function fetchHouseholdAutopayStatuses(
  api: ApiClient,
  orgId: string,
  householdIds: string[] = [],
): Promise<HouseholdAutopayStatusRecord[]> {
  const params = new URLSearchParams()
  for (const householdId of householdIds) {
    if (householdId) params.append('household_id', householdId)
  }
  const query = params.toString()
  const res = await api.get<{ autopay?: HouseholdAutopayStatusRecord[] }>(
    `/api/v1/autopay/households${query ? `?${query}` : ''}`,
    undefined,
    withOrgHeaders(orgId),
  )
  return res.autopay ?? []
}

export async function upsertHouseholdAutopay(
  api: ApiClient,
  orgId: string,
  householdId: string,
  input: {
    payment_method_type: 'card' | 'ach'
    payment_method_brand?: string
    payment_method_last4?: string
    payment_method_label: string
    payment_method_external_ref?: string
    payment_method_exp_month?: number
    payment_method_exp_year?: number
    trigger_days_before_due: 0 | 3 | 7
  },
): Promise<HouseholdAutopayStatusRecord> {
  const res = await api.put<{ autopay: HouseholdAutopayStatusRecord }>(
    `/api/v1/households/${householdId}/autopay`,
    input,
    withOrgHeaders(orgId),
  )
  return res.autopay
}

export async function optOutHouseholdAutopay(api: ApiClient, orgId: string, householdId: string): Promise<HouseholdAutopayStatusRecord> {
  const res = await api.post<{ autopay: HouseholdAutopayStatusRecord }>(
    `/api/v1/households/${householdId}/autopay/opt-out`,
    undefined,
    withOrgHeaders(orgId),
  )
  return res.autopay
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

export async function createInvoiceCheckoutSession(
  api: ApiClient,
  orgId: string,
  input: {
    invoice_id: string
    amount_cents: number
    description?: string
    success_url: string
    cancel_url: string
    idempotency_key: string
  },
): Promise<{ id: string; url: string }> {
  const res = await api.post<{ checkout_session: { id: string; url: string } }>(
    '/api/v1/stripe/checkout-sessions',
    {
      invoice_id: input.invoice_id,
      amount_cents: input.amount_cents,
      description: input.description ?? '',
      success_url: input.success_url,
      cancel_url: input.cancel_url,
    },
    {
      ...withOrgHeaders(orgId),
      'Idempotency-Key': input.idempotency_key,
    },
  )
  return res.checkout_session
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

export async function fetchSwimmerActivity(api: ApiClient, orgId: string, swimmerId: string): Promise<SwimmerActivityRecord[]> {
  const res = await api.get<{ activities?: SwimmerActivityRecord[] }>(`/api/v1/swimmers/${swimmerId}/activity`, undefined, withOrgHeaders(orgId))
  return res.activities ?? []
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

export async function fetchJobs(api: ApiClient, orgId: string): Promise<WorkerTaskRecord[]> {
  const res = await api.get<{ tasks?: WorkerTaskRecord[] }>('/api/v1/jobs', undefined, withOrgHeaders(orgId))
  return res.tasks ?? []
}

export async function fetchQueueStats(api: ApiClient, orgId: string): Promise<QueueStatsRecord[]> {
  const res = await api.get<{ queues?: QueueStatsRecord[] }>('/api/v1/jobs/queues', undefined, withOrgHeaders(orgId))
  return res.queues ?? []
}

export async function fetchJobMetrics(api: ApiClient, orgId: string): Promise<{ by_type: Record<string, JobMetricsTypeRecord>; recent_executions: JobExecutionRecord[] }> {
  return api.get<{ by_type: Record<string, JobMetricsTypeRecord>; recent_executions: JobExecutionRecord[] }>('/api/v1/jobs/metrics', undefined, withOrgHeaders(orgId))
}

export async function fetchDLQTasks(api: ApiClient, orgId: string): Promise<WorkerTaskRecord[]> {
  const res = await api.get<{ dead_letter_tasks?: WorkerTaskRecord[] }>('/api/v1/jobs/dlq', undefined, withOrgHeaders(orgId))
  return res.dead_letter_tasks ?? []
}

export async function retryDLQTask(api: ApiClient, orgId: string, taskId: string): Promise<void> {
  await api.post(`/api/v1/jobs/dlq/${taskId}/retry`, undefined, withOrgHeaders(orgId))
}

export async function deleteDLQTask(api: ApiClient, orgId: string, taskId: string): Promise<void> {
  await api.delete(`/api/v1/jobs/dlq/${taskId}`, withOrgHeaders(orgId))
}

export async function fetchAnnouncements(api: ApiClient, orgId: string, status?: string): Promise<AnnouncementRecord[]> {
  const query = status ? `?${new URLSearchParams({ status }).toString()}` : ''
  const res = await api.get<{ announcements?: AnnouncementRecord[] }>(`/api/v1/announcements${query}`, undefined, withOrgHeaders(orgId))
  return res.announcements ?? []
}

export async function fetchCommunicationPreferences(api: ApiClient, orgId: string): Promise<CommunicationPreferenceRecord[]> {
  const res = await api.get<{ preferences?: CommunicationPreferenceRecord[] }>(
    '/api/v1/communication-preferences',
    undefined,
    withOrgHeaders(orgId),
  )
  return res.preferences ?? []
}

export async function updateCommunicationPreferences(
  api: ApiClient,
  orgId: string,
  preferences: Array<{
    event_id: string
    email_enabled: boolean
    push_enabled: boolean
  }>,
): Promise<CommunicationPreferenceRecord[]> {
  const res = await api.put<{ preferences?: CommunicationPreferenceRecord[] }>(
    '/api/v1/communication-preferences',
    { preferences },
    withOrgHeaders(orgId),
  )
  return res.preferences ?? []
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

export async function sendCampaign(api: ApiClient, orgId: string, campaignId: string): Promise<EmailCampaignRecord> {
  return api.post<EmailCampaignRecord>(`/api/v1/email-campaigns/${campaignId}/send`, undefined, withOrgHeaders(orgId))
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

export async function fetchSwimmerCutQualification(api: ApiClient, orgId: string, swimmerId: string, standardId: string): Promise<CutQualificationRecord> {
  const query = new URLSearchParams({ standard_id: standardId }).toString()
  const res = await api.get<{ qualification: CutQualificationRecord }>(
    `/api/v1/swimmers/${swimmerId}/cut-qualification?${query}`,
    undefined,
    withOrgHeaders(orgId),
  )
  return res.qualification
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

// ── Document Center ───────────────────────────────────────────────────────────

export async function fetchDocumentTypes(api: ApiClient, orgId: string, activeOnly = true): Promise<DocumentTypeRecord[]> {
  const res = await api.get<{ document_types?: DocumentTypeRecord[] }>(`/api/v1/document-types?active_only=${activeOnly}`, undefined, withOrgHeaders(orgId))
  return res.document_types ?? []
}

export async function createDocumentType(api: ApiClient, orgId: string, input: {
  name: string
  category: string
  description?: string
  is_required: boolean
  applies_to?: string
  expiry_months?: number
  is_active: boolean
}): Promise<DocumentTypeRecord> {
  const res = await api.post<{ document_type: DocumentTypeRecord }>('/api/v1/document-types', input, withOrgHeaders(orgId))
  return res.document_type
}

export async function updateDocumentType(api: ApiClient, orgId: string, typeId: string, input: {
  name?: string
  category?: string
  description?: string
  is_required?: boolean
  applies_to?: string
  expiry_months?: number
  is_active?: boolean
}): Promise<DocumentTypeRecord> {
  const res = await api.put<{ document_type: DocumentTypeRecord }>(`/api/v1/document-types/${typeId}`, input, withOrgHeaders(orgId))
  return res.document_type
}

export async function deleteDocumentType(api: ApiClient, orgId: string, typeId: string): Promise<void> {
  await api.delete(`/api/v1/document-types/${typeId}`, withOrgHeaders(orgId))
}

export async function fetchDocumentSubmissions(api: ApiClient, orgId: string, householdId?: string): Promise<DocumentSubmissionRecord[]> {
  const q = householdId ? `?household_id=${householdId}` : ''
  const res = await api.get<{ documents?: DocumentSubmissionRecord[] }>(`/api/v1/documents${q}`, undefined, withOrgHeaders(orgId))
  return res.documents ?? []
}

export async function createDocumentSubmission(api: ApiClient, orgId: string, input: {
  document_type_id: string
  household_id: string
  swimmer_id?: string
}): Promise<DocumentSubmissionRecord> {
  const res = await api.post<{ document: DocumentSubmissionRecord }>('/api/v1/documents', input, withOrgHeaders(orgId))
  return res.document
}

export async function getDocumentSubmission(api: ApiClient, orgId: string, submissionId: string): Promise<DocumentSubmissionRecord> {
  const res = await api.get<{ document: DocumentSubmissionRecord }>(`/api/v1/documents/${submissionId}`, undefined, withOrgHeaders(orgId))
  return res.document
}

export async function attachDocumentFile(api: ApiClient, orgId: string, submissionId: string, input: {
  file_object_key: string
  file_name: string
  file_mime?: string
  file_size_bytes?: number
}): Promise<DocumentSubmissionRecord> {
  const res = await api.post<{ document: DocumentSubmissionRecord }>(`/api/v1/documents/${submissionId}/attach`, input, withOrgHeaders(orgId))
  return res.document
}

export async function requestDocumentUploadUrl(api: ApiClient, orgId: string, submissionId: string, input: {
  file_name: string
  file_mime: string
  file_size_bytes: number
}): Promise<{ upload_url: string; object_key: string; upload_expires_at: string }> {
  return api.post<{ upload_url: string; object_key: string; upload_expires_at: string }>(
    `/api/v1/documents/${submissionId}/upload-url`,
    input,
    withOrgHeaders(orgId),
  )
}

export async function verifyDocumentSubmission(api: ApiClient, orgId: string, submissionId: string, reviewNotes?: string): Promise<DocumentSubmissionRecord> {
  const res = await api.post<{ document: DocumentSubmissionRecord }>(`/api/v1/documents/${submissionId}/verify`, { review_notes: reviewNotes ?? '' }, withOrgHeaders(orgId))
  return res.document
}

export async function rejectDocumentSubmission(api: ApiClient, orgId: string, submissionId: string, reviewNotes?: string): Promise<DocumentSubmissionRecord> {
  const res = await api.post<{ document: DocumentSubmissionRecord }>(`/api/v1/documents/${submissionId}/reject`, { review_notes: reviewNotes ?? '' }, withOrgHeaders(orgId))
  return res.document
}

// ── Training / Practices ─────────────────────────────────────────────────────

export type PracticeRecord = {
  id: string
  org_id: string
  date: string // YYYY-MM-DD
  group_id?: string
  type: 'water' | 'dryland' | 'video' | 'rest'
  description: string
  yardage_target: number
  attendance?: number
  strokes: { free: number; back: number; breast: number; fly: number; im: number }
  notes?: string
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

export async function fetchPractices(api: ApiClient, orgId: string): Promise<PracticeRecord[]> {
  const res = await api.get<{ practices?: PracticeRecord[] }>('/api/v1/practices', undefined, withOrgHeaders(orgId))
  return res.practices ?? []
}

export async function createPractice(api: ApiClient, orgId: string, body: {
  date: string
  group_id?: string
  type: string
  description?: string
  yardage_target?: number
  attendance?: number
  strokes?: { free: number; back: number; breast: number; fly: number; im: number }
  notes?: string
}): Promise<PracticeRecord> {
  const res = await api.post<{ practice: PracticeRecord }>('/api/v1/practices', body, withOrgHeaders(orgId))
  return res.practice
}

export async function updatePractice(api: ApiClient, orgId: string, practiceId: string, body: Partial<{
  date: string
  group_id: string
  type: string
  description: string
  yardage_target: number
  attendance: number
  strokes: { free: number; back: number; breast: number; fly: number; im: number }
  notes: string
}>): Promise<PracticeRecord> {
  const res = await api.put<{ practice: PracticeRecord }>(`/api/v1/practices/${practiceId}`, body, withOrgHeaders(orgId))
  return res.practice
}

export async function deletePractice(api: ApiClient, orgId: string, practiceId: string): Promise<void> {
  await api.delete(`/api/v1/practices/${practiceId}`, withOrgHeaders(orgId))
}

// ── Seasons ───────────────────────────────────────────────────────────────────

export type SeasonRecord = {
  id: string
  org_id: string
  name: string
  start_date: string // YYYY-MM-DD, '' when unset
  end_date: string // YYYY-MM-DD, '' when unset
  status: 'active' | 'upcoming' | 'completed'
  goals: string
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

export async function fetchSeasons(api: ApiClient, orgId: string): Promise<SeasonRecord[]> {
  const res = await api.get<{ seasons?: SeasonRecord[] }>('/api/v1/seasons', undefined, withOrgHeaders(orgId))
  return res.seasons ?? []
}

export async function createSeason(api: ApiClient, orgId: string, body: {
  name: string
  start_date?: string
  end_date?: string
  status: string
  goals?: string
}): Promise<SeasonRecord> {
  const res = await api.post<{ season: SeasonRecord }>('/api/v1/seasons', body, withOrgHeaders(orgId))
  return res.season
}

export async function updateSeason(api: ApiClient, orgId: string, seasonId: string, body: Partial<{
  name: string
  start_date: string
  end_date: string
  status: string
  goals: string
}>): Promise<SeasonRecord> {
  const res = await api.put<{ season: SeasonRecord }>(`/api/v1/seasons/${seasonId}`, body, withOrgHeaders(orgId))
  return res.season
}

export async function deleteSeason(api: ApiClient, orgId: string, seasonId: string): Promise<void> {
  await api.delete(`/api/v1/seasons/${seasonId}`, withOrgHeaders(orgId))
}

// ── Commitment Event Selections ───────────────────────────────────────────────

export type CommitmentEventSelectionRecord = {
  id: string
  org_id: string
  commitment_id: string
  meet_event_id: string
  seed_time_ms?: number
  created_at: string
  created_by: string
}

export async function fetchCommitmentEventSelections(
  api: ApiClient,
  orgId: string,
  meetId: string,
  commitmentId: string,
): Promise<CommitmentEventSelectionRecord[]> {
  const res = await api.get<{ selections?: CommitmentEventSelectionRecord[] }>(
    `/api/v1/meets/${meetId}/commitments/${commitmentId}/event-selections`,
    undefined,
    withOrgHeaders(orgId),
  )
  return res.selections ?? []
}

export async function addCommitmentEventSelection(
  api: ApiClient,
  orgId: string,
  meetId: string,
  commitmentId: string,
  meetEventId: string,
): Promise<CommitmentEventSelectionRecord> {
  const res = await api.post<{ selection: CommitmentEventSelectionRecord }>(
    `/api/v1/meets/${meetId}/commitments/${commitmentId}/event-selections`,
    { meet_event_id: meetEventId },
    withOrgHeaders(orgId),
  )
  return res.selection
}

export async function deleteCommitmentEventSelection(
  api: ApiClient,
  orgId: string,
  meetId: string,
  commitmentId: string,
  selectionId: string,
): Promise<void> {
  await api.delete(
    `/api/v1/meets/${meetId}/commitments/${commitmentId}/event-selections/${selectionId}`,
    withOrgHeaders(orgId),
  )
}

export async function approveEntry(
  api: ApiClient,
  orgId: string,
  meetId: string,
  entryId: string,
): Promise<EntryRecord> {
  const res = await api.post<{ entry: EntryRecord }>(
    `/api/v1/meets/${meetId}/entries/${entryId}/approve`,
    undefined,
    withOrgHeaders(orgId),
  )
  return res.entry
}

export async function lockMeetCommitments(
  api: ApiClient,
  orgId: string,
  meetId: string,
): Promise<void> {
  await api.post(
    `/api/v1/meets/${meetId}/lock-commitments`,
    undefined,
    withOrgHeaders(orgId),
  )
}

// ── Create / Mutate helpers ───────────────────────────────────────────────────

export async function createHousehold(
  api: ApiClient,
  orgId: string,
  input: {
    household_name: string
    primary_email?: string
    primary_phone_e164?: string
    status?: string
    notes?: string
  },
): Promise<HouseholdRecord> {
  const res = await api.post<{ household: HouseholdRecord }>(
    '/api/v1/members/households',
    { org_id: orgId, ...input },
    withOrgHeaders(orgId),
  )
  return res.household
}

export async function deleteHousehold(
  api: ApiClient,
  orgId: string,
  householdId: string,
): Promise<void> {
  await api.delete(`/api/v1/members/households/${householdId}`, withOrgHeaders(orgId))
}

export async function deleteSwimmer(
  api: ApiClient,
  orgId: string,
  swimmerId: string,
): Promise<void> {
  await api.delete(`/api/v1/members/swimmers/${swimmerId}`, withOrgHeaders(orgId))
}

export async function updateHousehold(
  api: ApiClient,
  orgId: string,
  householdId: string,
  input: {
    household_name?: string
    primary_email?: string
    primary_phone_e164?: string
    status?: string
    notes?: string
  },
): Promise<HouseholdRecord> {
  const res = await api.patch<{ household: HouseholdRecord }>(
    `/api/v1/members/households/${householdId}`,
    input,
    withOrgHeaders(orgId),
  )
  return res.household
}

export async function createSwimmer(
  api: ApiClient,
  orgId: string,
  input: {
    household_id: string
    first_name: string
    last_name: string
    preferred_name?: string
    date_of_birth: string
    gender: string
    swimmer_external_id?: string
    registration_status?: string
    notes?: string
  },
): Promise<SwimmerRecord> {
  const res = await api.post<{ swimmer: SwimmerRecord }>(
    '/api/v1/members/swimmers',
    { org_id: orgId, ...input },
    withOrgHeaders(orgId),
  )
  return res.swimmer
}

export async function createGroup(
  api: ApiClient,
  orgId: string,
  input: {
    code: string
    name: string
    description?: string
    type?: string
    min_age?: number
    max_age?: number
    coach_user_id?: string
  },
): Promise<GroupRecord> {
  const res = await api.post<{ group: GroupRecord }>(
    '/api/v1/groups',
    input,
    withOrgHeaders(orgId),
  )
  return res.group
}

export async function createRegistration(
  api: ApiClient,
  orgId: string,
  input: {
    household_id: string
    swimmer_id: string
    group_id: string
    season: string
    registration_date?: string
    notes?: string
  },
): Promise<RegistrationRecord> {
  const res = await api.post<{ registration: RegistrationRecord }>(
    '/api/v1/registrations',
    input,
    withOrgHeaders(orgId),
  )
  return res.registration
}

export async function acceptRegistrationWaiver(
  api: ApiClient,
  orgId: string,
  registrationId: string,
  input: {
    waiver_type: string
    ip_address?: string
  },
): Promise<{
  id: string
  org_id: string
  registration_id: string
  waiver_type: string
  accepted_at: string
  accepted_by_user_id: string
  ip_address?: string
  created_at: string
}> {
  const res = await api.post<{ waiver: {
    id: string
    org_id: string
    registration_id: string
    waiver_type: string
    accepted_at: string
    accepted_by_user_id: string
    ip_address?: string
    created_at: string
  } }>(
    `/api/v1/registrations/${registrationId}/waivers`,
    input,
    withOrgHeaders(orgId),
  )
  return res.waiver
}

export async function assignSwimmerToGroup(
  api: ApiClient,
  orgId: string,
  swimmerId: string,
  input: {
    group_code: string
    is_primary?: boolean
  },
): Promise<SwimmerGroupLinkRecord> {
  const res = await api.post<{ assignment: SwimmerGroupLinkRecord }>(
    `/api/v1/members/swimmers/${swimmerId}/groups`,
    input,
    withOrgHeaders(orgId),
  )
  return res.assignment
}

export async function updateSwimmer(
  api: ApiClient,
  orgId: string,
  swimmerId: string,
  input: {
    preferred_name?: string
    notes?: string
  },
): Promise<SwimmerRecord> {
  const res = await api.patch<{ swimmer: SwimmerRecord }>(
    `/api/v1/members/swimmers/${swimmerId}`,
    input,
    withOrgHeaders(orgId),
  )
  return res.swimmer
}

export async function createMeetSession(
  api: ApiClient,
  orgId: string,
  meetId: string,
  input: {
    session_number: number
    name: string
    session_date?: string
    warmup_time?: string
    start_time?: string
  },
): Promise<MeetSessionRecord> {
  const res = await api.post<{ session: MeetSessionRecord }>(
    `/api/v1/meets/${meetId}/sessions`,
    input,
    withOrgHeaders(orgId),
  )
  return res.session
}

export async function createMeet(
  api: ApiClient,
  orgId: string,
  input: {
    name: string
    meet_type?: string
    course?: string
    start_date?: string
    end_date?: string
    entry_deadline?: string
    commitment_deadline?: string
    host_org?: string
    location?: string
    sessions?: Array<{
      session_number: number
      name: string
      session_date?: string
      warmup_time?: string
      start_time?: string
      notes?: string
    }>
    events?: Array<{
      session_number?: number
      event_number: string
      stroke: string
      distance: number
      course?: string
      gender?: string
      min_age?: number
      max_age?: number
      entry_fee_cents?: number
      is_relay?: boolean
    }>
    signup_settings?: {
      self_signup_enabled: boolean
      max_entries_per_swimmer: number
    }
  },
): Promise<MeetRecord> {
  const res = await api.post<{ meet: MeetRecord }>(
    '/api/v1/meets',
    input,
    withOrgHeaders(orgId),
  )
  return res.meet
}

export async function parseMeetEntryFile(
  token: string | undefined,
  orgId: string,
  file: File,
): Promise<ParsedMeetFileRecord> {
  const form = new FormData()
  form.append('file', file, file.name)

  const headers: Record<string, string> = {
    'X-Org-Id': orgId,
  }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  const res = await fetch('/api/v1/meets/parse-entry-file', {
    method: 'POST',
    body: form,
    headers,
    credentials: 'include',
  })
  if (!res.ok) {
    throw await ApiError.fromResponse(res)
  }
  const payload = await res.json() as { parsed: ParsedMeetFileRecord }
  return payload.parsed
}

export async function createInvoice(
  api: ApiClient,
  orgId: string,
  input: {
    household_id?: string
    invoice_number: string
    due_date?: string
    notes?: string
    external_ref?: string
  },
): Promise<InvoiceRecord> {
  const res = await api.post<{ invoice: InvoiceRecord }>(
    '/api/v1/invoices',
    input,
    withOrgHeaders(orgId),
  )
  return res.invoice
}

export async function requestImportUploadUrl(
  api: ApiClient,
  orgId: string,
  input: {
    filename: string
    size_bytes: number
    mime_type: string
    meet_id?: string
  },
): Promise<{ import_id: string; upload_url: string; storage_key: string }> {
  const res = await api.post<{ import: ImportRecord }>(
    '/api/v1/imports/upload-url',
    input,
    withOrgHeaders(orgId),
  )
  if (!res.import?.id || !res.import.upload_url || !res.import.storage_key) {
    throw new ApiError(500, 'invalid_import_upload_response', 'Upload URL response was missing import metadata.')
  }
  return {
    import_id: res.import.id,
    upload_url: res.import.upload_url,
    storage_key: res.import.storage_key,
  }
}

export async function finalizeImport(
  api: ApiClient,
  orgId: string,
  importId: string,
): Promise<ImportRecord> {
  const res = await api.post<{ import: ImportRecord }>(
    `/api/v1/imports/${importId}/finalize`,
    undefined,
    withOrgHeaders(orgId),
  )
  return res.import
}

export async function generateExport(
  api: ApiClient,
  orgId: string,
  meetId: string,
  input: {
    export_type: string
    filename?: string
  },
): Promise<ExportRecord> {
  const res = await api.post<{ export: ExportRecord }>(
    `/api/v1/meets/${meetId}/exports`,
    input,
    withOrgHeaders(orgId),
  )
  return res.export
}

// ── User Management (Keycloak) ─────────────────────────────────────────────────

export type UserRecord = {
  id: string
  username: string
  email: string
  firstName: string
  lastName: string
  enabled: boolean
  emailVerified: boolean
  roles?: string[]
  lastLoginAt?: string
  householdId?: string
  attributes?: Record<string, string[]>
}

export type UserListSource = 'stub' | 'keycloak'

export type UserRoleRecord = {
  id: string
  name: string
  description?: string
}

export async function fetchUsers(
  api: ApiClient,
  orgId: string,
  params?: { search?: string; role?: string; status?: string; limit?: number; offset?: number },
): Promise<{ users: UserRecord[]; count: number; source: UserListSource; isStubbed: boolean }> {
  const qs = new URLSearchParams()
  if (params?.search) qs.set('search', params.search)
  if (params?.role) qs.set('role', params.role)
  if (params?.status) qs.set('status', params.status)
  if (params?.limit) qs.set('limit', String(params.limit))
  if (params?.offset) qs.set('offset', String(params.offset))
  const query = qs.toString() ? `?${qs.toString()}` : ''
  const res = await api.get<{ status: string; count: number; source?: UserListSource; is_stubbed?: boolean; users: UserRecord[] }>(
    `/api/v1/users${query}`,
    undefined,
    withOrgHeaders(orgId),
  )
  const source = res.source === 'stub' ? 'stub' : 'keycloak'
  return { users: res.users ?? [], count: res.count ?? 0, source, isStubbed: res.is_stubbed ?? source === 'stub' }
}

export async function fetchUser(
  api: ApiClient,
  orgId: string,
  userId: string,
): Promise<{ user: UserRecord; roles: UserRoleRecord[] }> {
  const res = await api.get<{ status: string; user: UserRecord; roles: UserRoleRecord[] }>(
    `/api/v1/users/${userId}`,
    undefined,
    withOrgHeaders(orgId),
  )
  return { user: res.user, roles: res.roles ?? [] }
}

export async function fetchUserRoles(
  api: ApiClient,
  orgId: string,
  userId: string,
): Promise<UserRoleRecord[]> {
  const res = await api.get<{ status: string; roles: UserRoleRecord[] }>(
    `/api/v1/users/${userId}/roles`,
    undefined,
    withOrgHeaders(orgId),
  )
  return res.roles ?? []
}

export async function createUser(
  api: ApiClient,
  orgId: string,
  input: { first_name: string; last_name: string; email: string; password: string; role: string; temporary_password?: boolean; create_household?: boolean },
): Promise<{ user_id: string }> {
  const res = await api.post<{ status: string; user_id: string }>(
    '/api/v1/users',
    input,
    withOrgHeaders(orgId),
  )
  return { user_id: res.user_id }
}

export async function inviteUser(
  api: ApiClient,
  orgId: string,
  input: { first_name: string; last_name: string; email: string; role: string; create_household?: boolean },
): Promise<{ user_id: string }> {
  const res = await api.post<{ status: string; user_id: string }>(
    '/api/v1/users/invite',
    input,
    withOrgHeaders(orgId),
  )
  return { user_id: res.user_id }
}

export async function updateUser(
  api: ApiClient,
  orgId: string,
  userId: string,
  input: { first_name?: string; last_name?: string; email?: string },
): Promise<void> {
  await api.put(`/api/v1/users/${userId}`, input, withOrgHeaders(orgId))
}

export async function disableUser(api: ApiClient, orgId: string, userId: string): Promise<void> {
  await api.post(`/api/v1/users/${userId}/disable`, {}, withOrgHeaders(orgId))
}

export async function enableUser(api: ApiClient, orgId: string, userId: string): Promise<void> {
  await api.post(`/api/v1/users/${userId}/enable`, {}, withOrgHeaders(orgId))
}

export async function deleteUser(api: ApiClient, orgId: string, userId: string): Promise<void> {
  await api.delete(`/api/v1/users/${userId}`, withOrgHeaders(orgId))
}

export async function assignUserRole(
  api: ApiClient,
  orgId: string,
  userId: string,
  role: string,
): Promise<void> {
  await api.post(`/api/v1/users/${userId}/roles`, { role }, withOrgHeaders(orgId))
}

export async function removeUserRole(
  api: ApiClient,
  orgId: string,
  userId: string,
  role: string,
): Promise<void> {
  await api.delete(`/api/v1/users/${userId}/roles/${role}`, withOrgHeaders(orgId))
}

export async function sendPasswordReset(
  api: ApiClient,
  orgId: string,
  userId: string,
): Promise<void> {
  await api.post(`/api/v1/users/${userId}/password-reset`, {}, withOrgHeaders(orgId))
}
