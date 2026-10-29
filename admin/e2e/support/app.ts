import type { scenarioFixture } from './fixtures'
import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test'

type ScenarioFixture = ReturnType<typeof scenarioFixture>

const API_ORIGIN = `http://localhost:${process.env.PLAYWRIGHT_API_PORT ?? '8080'}`
const ADMIN_ORIGIN = `http://localhost:${process.env.PLAYWRIGHT_ADMIN_PORT ?? '3000'}`

type SessionInput = {
  userId: string
  orgId: string
  roles: string[]
  householdId?: string
  email?: string
  name?: string
  mfaAuthenticated?: boolean
}

type HouseholdRecord = {
  id: string
  household_name: string
}

type SwimmerRecord = {
  id: string
  first_name: string
  last_name: string
  notes?: string
}

type GroupRecord = {
  id: string
  code: string
  name: string
}

type MeetRecord = {
  id: string
  name: string
  status: string
}

type MeetEventRecord = {
  id: string
  event_number: string
  distance: number
  stroke: string
}

type CommitmentRecord = {
  id: string
  swimmer_id: string
  status: string
}

type EventSelectionRecord = {
  id: string
  meet_event_id: string
}

type PerformanceResultRecord = {
  id: string
  swimmer_id: string
  fin_time_ms?: number | null
}

type CommunicationPreferenceRecord = {
  event_id: string
  email_enabled: boolean
  push_enabled: boolean
}

type HouseholdAutopayStatusRecord = {
  enabled: boolean
  trigger_days_before_due: number
  payment_method_label?: string
}

type TimeStandardRecord = {
  id: string
  standard_code: string
}

function buildEmail(userId: string, orgId: string) {
  return `${userId}.${orgId}@example.test`
}

export async function newAuthedContext(browser: Browser, input: SessionInput): Promise<BrowserContext> {
  const context = await browser.newContext({ baseURL: ADMIN_ORIGIN })
  await issueSession(context, input)
  return context
}

/** Authed admin context for the scenario's org. */
export function newAdminContext(browser: Browser, fixture: ScenarioFixture, label: string, name: string) {
  return newAuthedContext(browser, { userId: fixture.userId(label), orgId: fixture.orgId, roles: ['admin'], name })
}

/** Authed parent context scoped to one household. */
export function newParentContext(browser: Browser, fixture: ScenarioFixture, label: string, householdId: string, name: string) {
  return newAuthedContext(browser, { userId: fixture.userId(label), orgId: fixture.orgId, householdId, roles: ['parent'], name })
}

export async function issueSession(context: BrowserContext, input: SessionInput) {
  const response = await context.request.post(`${API_ORIGIN}/api/v1/auth/sessions/issue`, {
    data: {
      user_id: input.userId,
      email: input.email ?? buildEmail(input.userId, input.orgId),
      name: input.name ?? input.userId,
      org_id: input.orgId,
      household_id: input.householdId,
      roles: input.roles,
      mfa_authenticated: input.mfaAuthenticated ?? false,
    },
  })

  if (!response.ok()) {
    // Carry the status and body so an intermittent failure is diagnosable from the report.
    throw new Error(`Session issue failed: HTTP ${response.status()} ${(await response.text()).slice(0, 300)}`)
  }
}

async function csrfToken(context: BrowserContext) {
  const cookies = await context.cookies(API_ORIGIN)
  const match = cookies.find((cookie) => cookie.name === 'aquify_csrf_token')
  if (!match) {
    throw new Error('Missing aquify_csrf_token cookie after issuing a session.')
  }
  return decodeURIComponent(match.value)
}

async function requestWithOrg<T>(
  context: BrowserContext,
  method: 'GET' | 'POST' | 'PUT' | 'DELETE',
  path: string,
  orgId: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {
    'X-Org-Id': orgId,
  }

  if (method !== 'GET') {
    headers['X-CSRF-Token'] = await csrfToken(context)
  }

  const response = await context.request.fetch(`${API_ORIGIN}${path}`, {
    method,
    data: body,
    headers,
  })

  if (!response.ok()) {
    throw new Error(`${method} ${path} failed: HTTP ${response.status()} ${(await response.text()).slice(0, 300)}`)
  }
  return await response.json() as T
}

export async function createHousehold(context: BrowserContext, orgId: string, householdName: string) {
  const response = await requestWithOrg<{ household: HouseholdRecord }>(context, 'POST', '/api/v1/members/households', orgId, {
    org_id: orgId,
    household_name: householdName,
    primary_email: `${householdName.toLowerCase().replace(/[^a-z0-9]+/g, '-')}@example.test`,
    status: 'active',
  })
  return response.household
}

export async function createSwimmer(
  context: BrowserContext,
  orgId: string,
  householdId: string,
  swimmer: { firstName: string; lastName: string; dateOfBirth: string; notes?: string },
) {
  const response = await requestWithOrg<{ swimmer: SwimmerRecord }>(context, 'POST', '/api/v1/members/swimmers', orgId, {
    org_id: orgId,
    household_id: householdId,
    first_name: swimmer.firstName,
    last_name: swimmer.lastName,
    date_of_birth: swimmer.dateOfBirth,
    gender: 'female',
    registration_status: 'active',
    notes: swimmer.notes,
  })
  return response.swimmer
}

export async function createGroup(
  context: BrowserContext,
  orgId: string,
  input: { code: string; name: string; type?: string },
) {
  const response = await requestWithOrg<{ group: GroupRecord }>(context, 'POST', '/api/v1/groups', orgId, {
    code: input.code,
    name: input.name,
    type: input.type ?? 'competitive',
  })
  return response.group
}

export async function createMeet(
  context: BrowserContext,
  orgId: string,
  input: { name: string; eventNumber?: string; selfSignupEnabled?: boolean },
) {
  const response = await requestWithOrg<{ meet: MeetRecord }>(context, 'POST', '/api/v1/meets', orgId, {
    name: input.name,
    meet_type: 'invitational',
    course: 'SCY',
    start_date: '2026-07-10',
    end_date: '2026-07-10',
    commitment_deadline: '2026-07-09',
    sessions: [{ session_number: 1, name: 'Morning Session', session_date: '2026-07-10' }],
    events: [{
      session_number: 1,
      event_number: input.eventNumber ?? '1',
      stroke: 'freestyle',
      distance: 100,
      course: 'SCY',
      gender: 'F',
    }],
    signup_settings: {
      self_signup_enabled: input.selfSignupEnabled ?? true,
      max_entries_per_swimmer: 3,
    },
  })
  return response.meet
}

export async function transitionMeet(context: BrowserContext, orgId: string, meetId: string, status: string) {
  const response = await requestWithOrg<{ meet: MeetRecord }>(
    context,
    'POST',
    `/api/v1/meets/${meetId}/transition`,
    orgId,
    { status },
  )
  return response.meet
}

export async function publishCommitmentRequest(context: BrowserContext, orgId: string, meetId: string) {
  const response = await requestWithOrg<{ meet: MeetRecord; commitments: CommitmentRecord[] }>(
    context,
    'POST',
    `/api/v1/meets/${meetId}/commitment-request`,
    orgId,
    {},
  )
  return response
}

export async function listMeetEvents(context: BrowserContext, orgId: string, meetId: string) {
  const response = await requestWithOrg<{ events?: MeetEventRecord[] }>(
    context,
    'GET',
    `/api/v1/meets/${meetId}/events`,
    orgId,
  )
  return response.events ?? []
}

export async function respondToCommitment(
  context: BrowserContext,
  orgId: string,
  meetId: string,
  commitmentId: string,
  responseValue: 'committed_yes' | 'committed_no',
) {
  const response = await requestWithOrg<{ commitment: CommitmentRecord }>(
    context,
    'POST',
    `/api/v1/meets/${meetId}/commitments/${commitmentId}/respond`,
    orgId,
    { response: responseValue, notes: '' },
  )
  return response.commitment
}

export async function selectCommitmentEvent(
  context: BrowserContext,
  orgId: string,
  meetId: string,
  commitmentId: string,
  meetEventId: string,
) {
  const response = await requestWithOrg<{ selection: EventSelectionRecord }>(
    context,
    'POST',
    `/api/v1/meets/${meetId}/commitments/${commitmentId}/event-selections`,
    orgId,
    { meet_event_id: meetEventId },
  )
  return response.selection
}

export async function listCommitmentEventSelections(
  context: BrowserContext,
  orgId: string,
  meetId: string,
  commitmentId: string,
) {
  const response = await requestWithOrg<{ selections?: EventSelectionRecord[] }>(
    context,
    'GET',
    `/api/v1/meets/${meetId}/commitments/${commitmentId}/event-selections`,
    orgId,
  )
  return response.selections ?? []
}

export async function createLinkedSwimmerUser(
  context: BrowserContext,
  orgId: string,
  swimmerId: string,
  email: string,
  firstName: string,
  lastName: string,
) {
  const { user_id: userId } = await requestWithOrg<{ user_id: string }>(context, 'POST', '/api/v1/users', orgId, {
    email,
    first_name: firstName,
    last_name: lastName,
    password: 'E2E-test-password-123!',
    role: 'swimmer',
  })
  await requestWithOrg(context, 'POST', `/api/v1/users/${userId}/swimmer/link`, orgId, { swimmer_id: swimmerId })
  return userId
}

export async function getUser(context: BrowserContext, orgId: string, userId: string) {
  return requestWithOrg<{
    user: {
      id: string
      enabled: boolean
      attributes?: Record<string, string[]>
    }
  }>(context, 'GET', `/api/v1/users/${userId}`, orgId)
}

export async function listRegistrations(context: BrowserContext, orgId: string) {
  return requestWithOrg<{ registrations?: Array<{ swimmer_id: string; group_id: string; season: string }> }>(
    context,
    'GET',
    '/api/v1/registrations',
    orgId,
  )
}

export async function getCommunicationPreferences(context: BrowserContext, orgId: string) {
  return requestWithOrg<{ preferences?: CommunicationPreferenceRecord[] }>(
    context,
    'GET',
    '/api/v1/communication-preferences',
    orgId,
  )
}

export async function getHouseholdAutopay(context: BrowserContext, orgId: string, householdId: string) {
  return requestWithOrg<{ autopay: HouseholdAutopayStatusRecord }>(
    context,
    'GET',
    `/api/v1/households/${householdId}/autopay`,
    orgId,
  )
}

export async function createTimeStandard(
  context: BrowserContext,
  orgId: string,
  input: {
    name: string
    seasonYear: number
    standardCode: string
    description?: string
    eventDistance: number
    stroke: string
    course: string
    gender: string
    ageMin?: number
    ageMax?: number
    timeMs: number
    isActive?: boolean
  },
) {
  const response = await requestWithOrg<{ time_standard: TimeStandardRecord }>(context, 'POST', '/api/v1/time-standards', orgId, {
    name: input.name,
    season_year: input.seasonYear,
    standard_code: input.standardCode,
    description: input.description ?? '',
    event_distance: input.eventDistance,
    stroke: input.stroke,
    course: input.course,
    gender: input.gender,
    age_min: input.ageMin,
    age_max: input.ageMax,
    time_ms: input.timeMs,
    is_active: input.isActive ?? true,
  })
  return response.time_standard
}

export async function overrideBestTime(
  context: BrowserContext,
  orgId: string,
  swimmerId: string,
  input: {
    eventDistance: number
    stroke: string
    course: string
    bestTimeMs: number
    reason: string
  },
) {
  return requestWithOrg<{ best_time: { best_time_ms?: number | null } }>(
    context,
    'POST',
    `/api/v1/swimmers/${swimmerId}/best-times/override`,
    orgId,
    {
      event_distance: input.eventDistance,
      stroke: input.stroke,
      course: input.course,
      best_time_ms: input.bestTimeMs,
      reason: input.reason,
    },
  )
}

export async function createPerformanceResult(
  context: BrowserContext,
  orgId: string,
  swimmerId: string,
  input: {
    eventDistance: number
    stroke: string
    course: string
    gender: string
    finTimeMs: number
  },
) {
  const response = await requestWithOrg<{ result: PerformanceResultRecord }>(
    context,
    'POST',
    '/api/v1/results',
    orgId,
    {
      swimmer_id: swimmerId,
      distance: input.eventDistance,
      stroke: input.stroke,
      course: input.course,
      gender: input.gender,
      fin_time_ms: input.finTimeMs,
      is_official: true,
    },
  )
  return response.result
}

export async function expectLoaded(page: Page, heading: RegExp | string) {
  await expect(page.getByRole('heading', { name: heading })).toBeVisible()
}
