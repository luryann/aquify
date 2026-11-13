// API helpers for tenancy features: the club's Stripe account, sign-up invites,
// and the platform operator console. Org scoping always comes from the session.
import { orgHeaders, type ApiClient } from './api'

// ── Club payment account (Stripe Connect) ────────────────────────────────────

export type PaymentAccountStatus = {
  connected: boolean
  required: boolean
  can_take_payments: boolean
  charges_enabled: boolean
  payouts_enabled: boolean
  details_submitted: boolean
  stripe_account_id?: string
}

export function fetchPaymentAccount(api: ApiClient, orgId: string): Promise<PaymentAccountStatus> {
  return api.get<PaymentAccountStatus>('/api/v1/billing/payment-account', undefined, orgHeaders(orgId))
}

/** Starts or resumes hosted Stripe onboarding; returns the URL to send the admin to. */
export async function startPaymentOnboarding(api: ApiClient, orgId: string, returnUrl: string): Promise<string> {
  const res = await api.post<{ url: string }>(
    '/api/v1/billing/payment-account/onboarding',
    { return_url: returnUrl },
    orgHeaders(orgId),
  )
  return res.url
}

/** One plain-language line describing where onboarding stands. */
export function paymentAccountSummary(s: PaymentAccountStatus): string {
  if (!s.connected) return s.required ? 'Not connected. Families can’t pay online until you connect Stripe.' : 'Not connected. Payments use the platform’s Stripe account.'
  if (s.charges_enabled && s.payouts_enabled) return 'Connected. Families can pay online and payouts go to your bank account.'
  if (s.charges_enabled) return 'Connected. Families can pay online. Stripe still needs details before it can send payouts.'
  if (!s.details_submitted) return 'Connected, but setup isn’t finished. Continue in Stripe to turn on online payments.'
  return 'Stripe is reviewing your details. Online payments turn on once it approves them.'
}

// ── Sign-up invites ──────────────────────────────────────────────────────────

export type InviteRole = 'parent' | 'swimmer' | 'coach'

export type InviteRecord = {
  id: string
  label: string
  role: InviteRole
  max_uses: number
  uses: number
  expires_at: string
  created_at: string
  revoked_at?: string
}

export type InviteState = 'active' | 'revoked' | 'expired' | 'used_up'

export function inviteState(i: InviteRecord, now: number = Date.now()): InviteState {
  if (i.revoked_at) return 'revoked'
  if (Date.parse(i.expires_at) <= now) return 'expired'
  if (i.uses >= i.max_uses) return 'used_up'
  return 'active'
}

export async function fetchInvites(api: ApiClient, orgId: string): Promise<InviteRecord[]> {
  const res = await api.get<{ items: InviteRecord[] }>('/api/v1/invites', undefined, orgHeaders(orgId))
  return res.items ?? []
}

/** Creates an invite. The plaintext code is returned once and cannot be read again. */
export function createInvite(
  api: ApiClient,
  orgId: string,
  input: { role: InviteRole; label?: string; max_uses?: number; ttl_days?: number },
): Promise<{ invite: InviteRecord; code: string }> {
  return api.post('/api/v1/invites', input, orgHeaders(orgId))
}

export async function revokeInvite(api: ApiClient, orgId: string, id: string): Promise<void> {
  await api.delete(`/api/v1/invites/${encodeURIComponent(id)}`, orgHeaders(orgId))
}

// ── Platform operator console ────────────────────────────────────────────────

export type OrgStatus = 'provisioning' | 'active' | 'suspended' | 'offboarding' | 'deleted'

export type PlatformOrg = {
  id: string
  slug?: string
  name: string
  status: OrgStatus
  plan?: string
  region?: string
  suspended_at?: string
  offboarding_at?: string
  created_at: string
}

export async function fetchPlatformOrgs(api: ApiClient, platformOrgId: string): Promise<PlatformOrg[]> {
  const res = await api.get<{ items: PlatformOrg[] }>('/api/v1/platform/orgs', undefined, orgHeaders(platformOrgId))
  return res.items ?? []
}

export type ProvisionInput = {
  name: string
  slug: string
  admin_email: string
  admin_first_name: string
  admin_last_name: string
  team_code?: string
}

export function provisionOrg(api: ApiClient, platformOrgId: string, input: ProvisionInput): Promise<{ org: PlatformOrg; admin_user_id: string; resumed: boolean }> {
  return api.post('/api/v1/platform/orgs', input, orgHeaders(platformOrgId))
}

export function setOrgSuspended(api: ApiClient, platformOrgId: string, orgId: string, suspended: boolean): Promise<PlatformOrg> {
  return api.post(`/api/v1/platform/orgs/${encodeURIComponent(orgId)}/${suspended ? 'suspend' : 'resume'}`, undefined, orgHeaders(platformOrgId))
}

export type DomainChallenge = { domain: string; verified: boolean; challenge: { type: string; name: string; value: string } }

export function addOrgDomain(api: ApiClient, platformOrgId: string, orgId: string, domain: string): Promise<DomainChallenge> {
  return api.post(`/api/v1/platform/orgs/${encodeURIComponent(orgId)}/domains`, { domain }, orgHeaders(platformOrgId))
}

export async function verifyOrgDomain(api: ApiClient, platformOrgId: string, orgId: string, domain: string): Promise<void> {
  await api.post(`/api/v1/platform/orgs/${encodeURIComponent(orgId)}/domains/${encodeURIComponent(domain)}/verify`, undefined, orgHeaders(platformOrgId))
}

/** Starts closing a club: access stops now and the cooling-off period begins. */
export function startOffboarding(api: ApiClient, platformOrgId: string, orgId: string): Promise<PlatformOrg> {
  return api.post(`/api/v1/platform/orgs/${encodeURIComponent(orgId)}/offboard`, undefined, orgHeaders(platformOrgId))
}

export function cancelOffboarding(api: ApiClient, platformOrgId: string, orgId: string): Promise<PlatformOrg> {
  return api.post(`/api/v1/platform/orgs/${encodeURIComponent(orgId)}/cancel-offboarding`, undefined, orgHeaders(platformOrgId))
}

/** Writes the club's data export and returns a 24-hour download link. */
export async function exportOrgData(api: ApiClient, platformOrgId: string, orgId: string): Promise<string> {
  const res = await api.post<{ download_url: string }>(`/api/v1/platform/orgs/${encodeURIComponent(orgId)}/export`, undefined, orgHeaders(platformOrgId))
  return res.download_url
}

export function purgeOrg(api: ApiClient, platformOrgId: string, orgId: string, confirmSlug: string): Promise<{ files_deleted: number; users_deleted: number }> {
  return api.post(`/api/v1/platform/orgs/${encodeURIComponent(orgId)}/purge`, { confirm_slug: confirmSlug }, orgHeaders(platformOrgId))
}

/** Whole days left before a closing club's data may be deleted (0 = eligible now). */
export function coolingOffDaysLeft(offboardingAt: string | undefined, coolingDays = 14, now: number = Date.now()): number {
  if (!offboardingAt) return coolingDays
  const eligibleAt = Date.parse(offboardingAt) + coolingDays * 86_400_000
  return Math.max(0, Math.ceil((eligibleAt - now) / 86_400_000))
}

const SLUG_RE = /^[a-z0-9-]{3,40}$/

/** Suggests a URL handle from a club name ("Blue Fish Aquatics" -> "blue-fish-aquatics"). */
export function suggestSlug(name: string): string {
  return name.toLowerCase().normalize('NFKD').replace(/[̀-ͯ]/g, '').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40).replace(/-+$/g, '')
}

export function isValidSlug(slug: string): boolean {
  return SLUG_RE.test(slug)
}

export function orgStatusLabel(s: OrgStatus): string {
  return { provisioning: 'Setting up', active: 'Active', suspended: 'Suspended', offboarding: 'Closing', deleted: 'Deleted' }[s]
}

// ── Club support (operator console) ──────────────────────────────────────────

export type ClubDomain = { domain: string; verified: boolean }

export type ClubOverview = {
  org: PlatformOrg
  domains: ClubDomain[]
  role_counts: Partial<Record<'admin' | 'coach' | 'parent' | 'swimmer', number>>
  payments: { connected: boolean; charges_enabled: boolean; payouts_enabled: boolean; details_submitted: boolean }
}

export type ClubUser = {
  id: string
  email: string
  first_name: string
  last_name: string
  enabled: boolean
  email_verified: boolean
  mfa_enrolled: boolean
  roles: string[]
}

export type ClubActivityEvent = {
  id: string
  action: string
  actor_user_id: string
  occurred_at: string
  resource_id: string
  metadata?: Record<string, string>
}

export type SupportAction = 'password-reset' | 'resend-verification' | 'sign-out' | 'reset-mfa' | 'disable' | 'enable'

const clubPath = (orgId: string, rest = '') => `/api/v1/platform/orgs/${encodeURIComponent(orgId)}${rest}`

export function fetchClubOverview(api: ApiClient, platformOrgId: string, orgId: string): Promise<ClubOverview> {
  return api.get<ClubOverview>(clubPath(orgId, '/overview'), undefined, orgHeaders(platformOrgId))
}

export const CLUB_USERS_PAGE_SIZE = 200

export async function fetchClubUsers(api: ApiClient, platformOrgId: string, orgId: string, opts: { search?: string; role?: string; offset?: number } = {}): Promise<ClubUser[]> {
  const q = new URLSearchParams({ limit: String(CLUB_USERS_PAGE_SIZE) })
  if (opts.offset) q.set('offset', String(opts.offset))
  if (opts.search?.trim()) q.set('search', opts.search.trim())
  if (opts.role) q.set('role', opts.role)
  const res = await api.get<{ items: ClubUser[] }>(clubPath(orgId, `/users?${q}`), undefined, orgHeaders(platformOrgId))
  return res.items ?? []
}

export async function fetchClubActivity(api: ApiClient, platformOrgId: string, orgId: string): Promise<ClubActivityEvent[]> {
  const res = await api.get<{ items: ClubActivityEvent[] }>(clubPath(orgId, '/activity'), undefined, orgHeaders(platformOrgId))
  return res.items ?? []
}

export function updatePlatformOrg(api: ApiClient, platformOrgId: string, orgId: string, input: { name?: string; plan?: string; reason: string }): Promise<PlatformOrg> {
  return api.patch(clubPath(orgId), input, orgHeaders(platformOrgId))
}

export function runSupportAction(api: ApiClient, platformOrgId: string, orgId: string, userId: string, action: SupportAction, reason: string): Promise<void> {
  return api.post(clubPath(orgId, `/users/${encodeURIComponent(userId)}/${action}`), { reason }, orgHeaders(platformOrgId)).then(() => undefined)
}

export function addClubAdmin(
  api: ApiClient, platformOrgId: string, orgId: string,
  input: { email: string; first_name: string; last_name: string; reason: string },
): Promise<{ user_id: string; promoted: boolean }> {
  return api.post(clubPath(orgId, '/admins'), input, orgHeaders(platformOrgId))
}

/** What each support action does, in the words the operator confirms. */
export const SUPPORT_ACTIONS: Record<SupportAction, { label: string; consequence: string; done: string; danger?: boolean }> = {
  'password-reset': { label: 'Send password reset', consequence: 'Emails them a link to choose a new password. Their current password keeps working until they use it.', done: 'Password reset email sent' },
  'resend-verification': { label: 'Resend verification', consequence: 'Emails them a fresh link to confirm their address.', done: 'Verification email sent' },
  'sign-out': { label: 'Sign out everywhere', consequence: 'Ends their open aquify sessions. They can sign back in straight away, and it doesn’t sign them out of the identity provider.', done: 'Signed out everywhere' },
  'reset-mfa': { label: 'Reset MFA', consequence: 'Removes their authenticator and backup codes and signs them out. They set up MFA again at their next sign-in.', done: 'MFA reset', danger: true },
  disable: { label: 'Disable account', consequence: 'Signs them out and blocks sign-in until you enable the account again. If this is the club’s only administrator, add another one first.', done: 'Account disabled', danger: true },
  enable: { label: 'Enable account', consequence: 'Lets them sign in again.', done: 'Account enabled' },
}

/** The actions that make sense for this account right now, in display order. */
export function supportActionsFor(u: Pick<ClubUser, 'enabled' | 'email_verified' | 'mfa_enrolled'>): SupportAction[] {
  if (!u.enabled) return ['enable']
  const out: SupportAction[] = ['password-reset']
  if (!u.email_verified) out.push('resend-verification')
  out.push('sign-out')
  if (u.mfa_enrolled) out.push('reset-mfa')
  out.push('disable')
  return out
}

export const REASON_MIN = 3
export const REASON_MAX = 500

/** The audit trail needs a real reason: 3 to 500 characters once trimmed. */
export function isValidReason(reason: string): boolean {
  const n = [...reason.trim()].length
  return n >= REASON_MIN && n <= REASON_MAX
}

const ACTIVITY_LABELS: Record<string, string> = {
  'platform.org.create': 'Club created',
  'platform.org.update': 'Club details changed',
  'platform.org.suspend': 'Club paused',
  'platform.org.resume': 'Club resumed',
  'platform.org.offboard': 'Closing started',
  'platform.org.offboard_cancel': 'Closing cancelled',
  'platform.org.export': 'Data export created',
  'platform.org.purge': 'Club data deleted',
  'platform.org.restore': 'Club data restored',
  'platform.org.domain_add': 'Web address added',
  'platform.org.domain_verify': 'Web address verified',
  'platform.user.password_reset': 'Password reset sent',
  'platform.user.resend_verification': 'Verification email resent',
  'platform.user.sign_out': 'User signed out everywhere',
  'platform.user.mfa_reset': 'MFA reset',
  'platform.user.disable': 'Account disabled',
  'platform.user.enable': 'Account enabled',
  'platform.user.admin_add': 'Administrator added',
}

export function activityLabel(action: string): string {
  return ACTIVITY_LABELS[action] ?? action.replace(/^platform\./, '').replace(/[._]/g, ' ')
}

export function userDisplayName(u: Pick<ClubUser, 'first_name' | 'last_name' | 'email'>): string {
  return `${u.first_name} ${u.last_name}`.trim() || u.email
}

/** One plain line on where the club's Stripe account stands (operator view). */
export function operatorPaymentSummary(p: ClubOverview['payments']): string {
  if (!p.connected) return 'Not connected'
  if (p.charges_enabled && p.payouts_enabled) return 'Taking payments, payouts on'
  if (p.charges_enabled) return 'Taking payments, payouts waiting on Stripe'
  if (!p.details_submitted) return 'Connected, setup unfinished'
  return 'Stripe is reviewing details'
}
