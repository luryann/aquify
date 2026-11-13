import test from 'node:test'
import assert from 'node:assert/strict'

import {
  activityLabel, coolingOffDaysLeft, inviteState, isValidReason, isValidSlug, operatorPaymentSummary, orgStatusLabel, paymentAccountSummary,
  suggestSlug, supportActionsFor, userDisplayName, type InviteRecord, type PaymentAccountStatus,
} from './tenancyData.ts'

const NOW = Date.parse('2026-11-03T12:00:00Z')
const invite = (over: Partial<InviteRecord> = {}): InviteRecord => ({
  id: 'i', label: '', role: 'parent', max_uses: 2, uses: 0, expires_at: '2026-12-01T00:00:00Z', created_at: '2026-11-01T00:00:00Z', ...over,
})

test('invite state reflects revocation, expiry and use', () => {
  assert.equal(inviteState(invite(), NOW), 'active')
  assert.equal(inviteState(invite({ revoked_at: '2026-11-02T00:00:00Z' }), NOW), 'revoked')
  assert.equal(inviteState(invite({ expires_at: '2026-11-03T12:00:00Z' }), NOW), 'expired')
  assert.equal(inviteState(invite({ uses: 2 }), NOW), 'used_up')
  // revoked wins over expired so the list explains the real reason
  assert.equal(inviteState(invite({ revoked_at: 'x', expires_at: '2020-01-01T00:00:00Z' }), NOW), 'revoked')
})

test('slugs are suggested and validated to the server rule', () => {
  assert.equal(suggestSlug('Blue Fish Aquatics'), 'blue-fish-aquatics')
  assert.equal(suggestSlug('  Été  Club!! '), 'ete-club')
  assert.equal(suggestSlug('x'.repeat(60)).length, 40)
  assert.ok(isValidSlug('blue-fish') && !isValidSlug('ab') && !isValidSlug('Has Space') && !isValidSlug('x'.repeat(41)))
})

test('payment account summary explains each stage in plain language', () => {
  const base: PaymentAccountStatus = { connected: false, required: true, can_take_payments: false, charges_enabled: false, payouts_enabled: false, details_submitted: false }
  assert.match(paymentAccountSummary(base), /can’t pay online/)
  assert.match(paymentAccountSummary({ ...base, required: false, can_take_payments: true }), /platform’s Stripe/)
  assert.match(paymentAccountSummary({ ...base, connected: true }), /isn’t finished/)
  assert.match(paymentAccountSummary({ ...base, connected: true, details_submitted: true }), /reviewing/)
  assert.match(paymentAccountSummary({ ...base, connected: true, charges_enabled: true }), /payouts/)
  assert.match(paymentAccountSummary({ ...base, connected: true, charges_enabled: true, payouts_enabled: true }), /bank account/)
})

test('org status labels cover every status', () => {
  for (const s of ['provisioning', 'active', 'suspended', 'offboarding', 'deleted'] as const) assert.ok(orgStatusLabel(s).length > 0)
})

test('cooling-off countdown rounds up and never goes negative', () => {
  const start = Date.parse('2026-11-01T00:00:00Z')
  assert.equal(coolingOffDaysLeft('2026-11-01T00:00:00Z', 14, start), 14)
  assert.equal(coolingOffDaysLeft('2026-11-01T00:00:00Z', 14, start + 13.5 * 86_400_000), 1)
  assert.equal(coolingOffDaysLeft('2026-11-01T00:00:00Z', 14, start + 20 * 86_400_000), 0)
  assert.equal(coolingOffDaysLeft(undefined, 14), 14)
})

test('support actions offered depend on the account state', () => {
  assert.deepEqual(supportActionsFor({ enabled: false, email_verified: true, mfa_enrolled: true }), ['enable'])
  assert.deepEqual(supportActionsFor({ enabled: true, email_verified: true, mfa_enrolled: false }), ['password-reset', 'sign-out', 'disable'])
  assert.deepEqual(supportActionsFor({ enabled: true, email_verified: false, mfa_enrolled: true }), ['password-reset', 'resend-verification', 'sign-out', 'reset-mfa', 'disable'])
})

test('a support reason must be 3 to 500 characters once trimmed', () => {
  assert.equal(isValidReason(''), false)
  assert.equal(isValidReason('  hi  '), false)
  assert.equal(isValidReason(' ok! '), true)
  assert.equal(isValidReason('x'.repeat(500)), true)
  assert.equal(isValidReason('x'.repeat(501)), false)
})

test('activity and user labels read as plain language', () => {
  assert.equal(activityLabel('platform.user.mfa_reset'), 'MFA reset')
  assert.equal(activityLabel('platform.org.something_new'), 'org something new')
  assert.equal(userDisplayName({ first_name: 'Ana', last_name: 'Ruiz', email: 'a@x.org' }), 'Ana Ruiz')
  assert.equal(userDisplayName({ first_name: '', last_name: '', email: 'a@x.org' }), 'a@x.org')
})

test('operator payment summary covers each Stripe state', () => {
  const p = (over: object) => ({ connected: true, charges_enabled: false, payouts_enabled: false, details_submitted: false, ...over })
  assert.equal(operatorPaymentSummary(p({ connected: false })), 'Not connected')
  assert.equal(operatorPaymentSummary(p({ charges_enabled: true, payouts_enabled: true })), 'Taking payments, payouts on')
  assert.equal(operatorPaymentSummary(p({ charges_enabled: true })), 'Taking payments, payouts waiting on Stripe')
  assert.equal(operatorPaymentSummary(p({})), 'Connected, setup unfinished')
  assert.equal(operatorPaymentSummary(p({ details_submitted: true })), 'Stripe is reviewing details')
})
