import test from 'node:test'
import assert from 'node:assert/strict'

import { ApiError, type ApiClient } from './api.ts'
import { agingShares, fetchAdjustments, fetchBillingRuns, describeDiscount, describeTier, financeErrorMessage, formatCents, parseDollarsToCents } from './financeData.ts'

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
  assert.equal(describeTier({ min_position: 1, max_position: 1 }), '1st swimmer')
  assert.equal(describeTier({ min_position: 4, max_position: 12 }), '4th–12th swimmers')
  assert.equal(describeTier({ min_position: 21, max_position: null }), '21st+ swimmer')
  assert.equal(describeTier({ min_position: 11, max_position: 13 }), '11th–13th swimmers')
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

test('fetchAdjustments and fetchBillingRuns page through every row instead of truncating at 200', async () => {
  const paths: string[] = []
  const api = {
    get: async (path: string) => {
      paths.push(path)
      const offset = Number(new URL(`http://x${path}`).searchParams.get('offset'))
      const key = path.includes('/adjustments') ? 'adjustments' : 'runs'
      const all = Array.from({ length: 250 }, (_, i) => ({ id: `r${i}` }))
      return { [key]: all.slice(offset, offset + 200), total: 250 }
    },
  } as unknown as ApiClient

  assert.equal((await fetchAdjustments(api, 'org_1')).length, 250)
  assert.equal((await fetchBillingRuns(api, 'org_1')).length, 250)
  assert.ok(paths.some((p) => p.includes('/adjustments?status=pending&limit=200&offset=200')))
})
