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
}

export type EntryRecord = {
  id: string
  meet_id: string
  swimmer_id: string
  status: string
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

function withOrgHeaders(orgId: string) {
  return { 'X-Org-Id': orgId }
}

export async function fetchHouseholds(api: ApiClient, orgId: string): Promise<HouseholdRecord[]> {
  const res = await api.get<{ households?: HouseholdRecord[] }>('/api/v1/members/households', undefined, withOrgHeaders(orgId))
  return res.households ?? []
}

export async function fetchSwimmers(api: ApiClient, orgId: string): Promise<SwimmerRecord[]> {
  const res = await api.get<{ swimmers?: SwimmerRecord[] }>(`/api/v1/members/swimmers`, undefined, withOrgHeaders(orgId))
  return res.swimmers ?? []
}

export async function fetchSwimmerGroups(api: ApiClient, orgId: string): Promise<SwimmerGroupLinkRecord[]> {
  const res = await api.get<{ swimmer_groups?: SwimmerGroupLinkRecord[] }>('/api/v1/members/swimmer-groups', undefined, withOrgHeaders(orgId))
  return res.swimmer_groups ?? []
}

export async function fetchGroups(api: ApiClient, orgId: string): Promise<GroupRecord[]> {
  const res = await api.get<{ groups?: GroupRecord[] }>('/api/v1/groups', undefined, withOrgHeaders(orgId))
  return res.groups ?? []
}

export async function fetchRegistrations(api: ApiClient, orgId: string): Promise<RegistrationRecord[]> {
  const res = await api.get<{ registrations?: RegistrationRecord[] }>('/api/v1/registrations', undefined, withOrgHeaders(orgId))
  return res.registrations ?? []
}

export async function fetchMeets(api: ApiClient, orgId: string): Promise<MeetRecord[]> {
  const res = await api.get<{ meets?: MeetRecord[] }>('/api/v1/meets', undefined, withOrgHeaders(orgId))
  return res.meets ?? []
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

export async function fetchExportsForMeet(api: ApiClient, orgId: string, meetId: string): Promise<ExportRecord[]> {
  const res = await api.get<{ exports?: ExportRecord[] }>(`/api/v1/meets/${meetId}/exports`, undefined, withOrgHeaders(orgId))
  return res.exports ?? []
}

export async function fetchInvoicesForHousehold(api: ApiClient, orgId: string, householdId: string): Promise<InvoiceRecord[]> {
  const res = await api.get<{ invoices?: InvoiceRecord[] }>(`/api/v1/households/${householdId}/invoices`, undefined, withOrgHeaders(orgId))
  return res.invoices ?? []
}

export async function fetchLedgerEvents(api: ApiClient, orgId: string): Promise<LedgerEventRecord[]> {
  const res = await api.get<{ events?: LedgerEventRecord[] }>('/api/v1/ledger-events', undefined, withOrgHeaders(orgId))
  return res.events ?? []
}
