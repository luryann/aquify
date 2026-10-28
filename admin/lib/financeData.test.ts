import test from 'node:test'
import assert from 'node:assert/strict'

import { ApiError } from './api.ts'
import { agingShares, describeDiscount, describeTier, financeErrorMessage, formatCents, parseDollarsToCents } from './financeData.ts'

test('parseDollarsToCents converts dollar strings to integer cents', () => {
  assert.equal(parseDollarsToCents('12.5'), 1250)
  assert.equal(parseDollarsToCents('$1,200'), 120000)
  assert.equal(parseDollarsToCents('-10.05'), -1005)
  assert.equal(parseDollarsToCents('0.1'), 10)
  assert.equal(parseDollarsToCents('abc'), null)
  assert.equal(parseDollarsToCents('1.234'), null)
})

test('formatCents renders signed currency', () => {
  assert.equal(formatCents(123456), '$1,234.56')
  assert.equal(formatCents(-500), '−$5.00')
  assert.equal(formatCents(undefined), '$0.00')
})

test('discount tier descriptions', () => {
  assert.equal(describeDiscount({ discount_type: 'percent', percent_bps: 1000, amount_cents: 0 }), '10% off')
  assert.equal(describeDiscount({ discount_type: 'percent', percent_bps: 1250, amount_cents: 0 }), '12.50% off')
  assert.equal(describeDiscount({ discount_type: 'fixed', percent_bps: 0, amount_cents: 2500 }), '$25.00 off')
  assert.equal(describeTier({ min_position: 2, max_position: 2 }), '2nd swimmer')
  assert.equal(describeTier({ min_position: 3, max_position: null }), '3rd+ swimmer')
})

test('agingShares handles empty and populated totals', () => {
  const empty = { current_cents: 0, days_1_30_cents: 0, days_31_60_cents: 0, days_61_90_cents: 0, days_90_plus_cents: 0, total_cents: 0 }
  assert.deepEqual(agingShares(empty), [0, 0, 0, 0, 0])
  const shares = agingShares({ ...empty, current_cents: 300, days_90_plus_cents: 100, total_cents: 400 })
  assert.deepEqual(shares, [0.75, 0, 0, 0, 0.25])
})

test('financeErrorMessage maps run conflicts to guidance', () => {
  assert.match(financeErrorMessage(new ApiError(422, 'nothing_to_bill', 'x'), 'f'), /already billed/)
  assert.match(financeErrorMessage(new ApiError(409, 'preview_stale', 'x'), 'f'), /changed since this preview/)
  assert.equal(financeErrorMessage(new Error('boom'), 'fallback'), 'fallback')
})
