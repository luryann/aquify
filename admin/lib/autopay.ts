import { ApiError } from './api.ts'
import type { HouseholdAutopayStatusRecord, HouseholdRecord } from './adminData.ts'

export type AutopayFormState = {
  paymentMethodType: 'card' | 'ach'
  paymentMethodBrand: string
  paymentMethodLast4: string
  paymentMethodLabel: string
  paymentMethodExternalRef: string
  paymentMethodExpMonth: string
  paymentMethodExpYear: string
  triggerDaysBeforeDue: 0 | 3 | 7
}

export type HouseholdAutopayRow = {
  householdId: string
  householdName: string
  enabled: boolean
  methodLabel: string
  nextChargeDate?: string
  nextChargeAmountCents?: number
  triggerDaysBeforeDue: 0 | 3 | 7
  status: HouseholdAutopayStatusRecord
}

export const DEFAULT_AUTOPAY_FORM: AutopayFormState = {
  paymentMethodType: 'card',
  paymentMethodBrand: '',
  paymentMethodLast4: '',
  paymentMethodLabel: '',
  paymentMethodExternalRef: '',
  paymentMethodExpMonth: '',
  paymentMethodExpYear: '',
  triggerDaysBeforeDue: 0,
}

export function formatAutopayMethod(status?: Partial<HouseholdAutopayStatusRecord> | null): string {
  if (!status) return 'No payment method saved'
  if (status.payment_method_label?.trim()) return status.payment_method_label.trim()
  if (status.payment_method_type === 'ach') return status.payment_method_brand?.trim() || 'ACH transfer'
  const brand = status.payment_method_brand?.trim()
  const last4 = status.payment_method_last4?.trim()
  if (brand && last4) return `${brand} •••• ${last4}`
  if (brand) return brand
  if (last4) return `Card •••• ${last4}`
  return 'No payment method saved'
}

export function autopayFormFromStatus(status?: HouseholdAutopayStatusRecord | null): AutopayFormState {
  if (!status) return { ...DEFAULT_AUTOPAY_FORM }
  return {
    paymentMethodType: status.payment_method_type === 'ach' ? 'ach' : 'card',
    paymentMethodBrand: status.payment_method_brand ?? '',
    paymentMethodLast4: status.payment_method_last4 ?? '',
    paymentMethodLabel: status.payment_method_label ?? '',
    paymentMethodExternalRef: status.payment_method_external_ref ?? '',
    paymentMethodExpMonth: status.payment_method_exp_month ? String(status.payment_method_exp_month) : '',
    paymentMethodExpYear: status.payment_method_exp_year ? String(status.payment_method_exp_year) : '',
    triggerDaysBeforeDue: normalizeTriggerDays(status.trigger_days_before_due),
  }
}

export function autopayPayloadFromForm(form: AutopayFormState) {
  return {
    payment_method_type: form.paymentMethodType,
    payment_method_brand: form.paymentMethodBrand.trim(),
    payment_method_last4: form.paymentMethodLast4.trim(),
    payment_method_label: form.paymentMethodLabel.trim(),
    payment_method_external_ref: form.paymentMethodExternalRef.trim(),
    payment_method_exp_month: parseOptionalInt(form.paymentMethodExpMonth),
    payment_method_exp_year: parseOptionalInt(form.paymentMethodExpYear),
    trigger_days_before_due: normalizeTriggerDays(form.triggerDaysBeforeDue),
  }
}

export function autopayErrorMessage(error: unknown, fallback = 'Unable to update autopay right now.'): string {
  if (error instanceof ApiError) return error.message
  return fallback
}

export function mergeHouseholdAutopayRows(
  households: HouseholdRecord[],
  statuses: HouseholdAutopayStatusRecord[],
): HouseholdAutopayRow[] {
  const statusByHousehold = new Map(statuses.map((status) => [status.household_id, status]))
  return households.map((household) => {
    const status = statusByHousehold.get(household.id) ?? {
      household_id: household.id,
      enabled: false,
      trigger_days_before_due: 0,
    }
    return {
      householdId: household.id,
      householdName: household.household_name,
      enabled: status.enabled,
      methodLabel: formatAutopayMethod(status),
      nextChargeDate: status.next_charge_date,
      nextChargeAmountCents: status.next_charge_amount_cents,
      triggerDaysBeforeDue: normalizeTriggerDays(status.trigger_days_before_due),
      status,
    }
  })
}

export function normalizeTriggerDays(value: number | string | undefined | null): 0 | 3 | 7 {
  const numeric = typeof value === 'string' ? Number.parseInt(value, 10) : value
  return numeric === 3 || numeric === 7 ? numeric : 0
}

function parseOptionalInt(value: string): number | undefined {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  const parsed = Number.parseInt(trimmed, 10)
  return Number.isFinite(parsed) ? parsed : undefined
}
