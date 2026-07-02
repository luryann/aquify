import test from 'node:test'
import assert from 'node:assert/strict'

import { ApiError } from './api.ts'
import {
  DEFAULT_AUTOPAY_FORM,
  autopayErrorMessage,
  autopayFormFromStatus,
  autopayPayloadFromForm,
  formatAutopayMethod,
  mergeHouseholdAutopayRows,
  normalizeTriggerDays,
} from './autopay.ts'

test('formatAutopayMethod prefers explicit saved labels and falls back to method details', () => {
  assert.equal(formatAutopayMethod({ payment_method_label: 'Visa •••• 4242' }), 'Visa •••• 4242')
  assert.equal(formatAutopayMethod({ payment_method_type: 'ach', payment_method_brand: 'Bank account' }), 'Bank account')
  assert.equal(formatAutopayMethod({ payment_method_brand: 'Mastercard', payment_method_last4: '4444' }), 'Mastercard •••• 4444')
  assert.equal(formatAutopayMethod(), 'No payment method saved')
})

test('autopay form helpers normalize persisted status into API payloads', () => {
  const form = autopayFormFromStatus({
    household_id: 'hh_1',
    enabled: true,
    payment_method_type: 'card',
    payment_method_brand: 'Visa',
    payment_method_last4: '4242',
    payment_method_label: 'Visa •••• 4242',
    payment_method_external_ref: 'pm_123',
    payment_method_exp_month: 9,
    payment_method_exp_year: 2028,
    trigger_days_before_due: 7,
  })

  assert.deepEqual(form, {
    paymentMethodType: 'card',
    paymentMethodBrand: 'Visa',
    paymentMethodLast4: '4242',
    paymentMethodLabel: 'Visa •••• 4242',
    paymentMethodExternalRef: 'pm_123',
    paymentMethodExpMonth: '9',
    paymentMethodExpYear: '2028',
    triggerDaysBeforeDue: 7,
  })
  assert.deepEqual(autopayPayloadFromForm(form), {
    payment_method_type: 'card',
    payment_method_brand: 'Visa',
    payment_method_last4: '4242',
    payment_method_label: 'Visa •••• 4242',
    payment_method_external_ref: 'pm_123',
    payment_method_exp_month: 9,
    payment_method_exp_year: 2028,
    trigger_days_before_due: 7,
  })
  assert.deepEqual(autopayFormFromStatus(null), DEFAULT_AUTOPAY_FORM)
})

test('mergeHouseholdAutopayRows overlays status records onto household lists', () => {
  const rows = mergeHouseholdAutopayRows(
    [
      { id: 'hh_1', org_id: 'org_1', household_name: 'Henderson', status: 'active', created_at: '', updated_at: '', created_by: '', updated_by: '' },
      { id: 'hh_2', org_id: 'org_1', household_name: 'Park', status: 'active', created_at: '', updated_at: '', created_by: '', updated_by: '' },
    ],
    [
      {
        household_id: 'hh_1',
        enabled: true,
        payment_method_label: 'Visa •••• 4242',
        trigger_days_before_due: 3,
        next_charge_date: '2026-05-30',
        next_charge_amount_cents: 6400,
      },
    ],
  )

  assert.equal(rows[0]?.enabled, true)
  assert.equal(rows[0]?.methodLabel, 'Visa •••• 4242')
  assert.equal(rows[1]?.enabled, false)
  assert.equal(rows[1]?.methodLabel, 'No payment method saved')
  assert.equal(rows[1]?.triggerDaysBeforeDue, 0)
})

test('autopayErrorMessage and trigger normalization preserve API messages', () => {
  assert.equal(normalizeTriggerDays('3'), 3)
  assert.equal(normalizeTriggerDays(99), 0)
  assert.equal(
    autopayErrorMessage(new ApiError(422, 'autopay_payment_method_required', 'Payment method details are required to enable autopay.')),
    'Payment method details are required to enable autopay.',
  )
  assert.equal(autopayErrorMessage(new Error('boom')), 'Unable to update autopay right now.')
})
