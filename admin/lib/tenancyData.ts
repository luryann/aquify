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
