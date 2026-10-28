import { ApiError, type ApiClient } from '#lib/api'

import type { InvoiceRecord } from './adminData.ts'
import { createInvoiceCheckoutSession } from './adminData.ts'

export function buildPortalInvoiceCheckoutURLs(origin: string, invoiceId: string) {
  return {
    success_url: `${origin}/portal/billing/invoices/${invoiceId}?checkout=success`,
    cancel_url: `${origin}/portal/billing/invoices/${invoiceId}?checkout=cancelled`,
  }
}

export function isPortalInvoiceCheckoutDisabled(invoice: InvoiceRecord | null, checkingOut: boolean): boolean {
  if (checkingOut || !invoice) {
    return true
  }
  return invoice.amount_due_cents <= 0 || !['issued', 'partially_paid'].includes(invoice.status)
}

export async function launchPortalInvoiceCheckout(params: {
  api: ApiClient
  orgId: string
  invoice: InvoiceRecord
  origin: string
  redirect: (url: string) => void
}): Promise<void> {
  const { api, orgId, invoice, origin, redirect } = params
  const urls = buildPortalInvoiceCheckoutURLs(origin, invoice.id)
  const checkout = await createInvoiceCheckoutSession(api, orgId, {
    invoice_id: invoice.id,
    amount_cents: invoice.amount_due_cents,
    description: invoice.notes ?? invoice.invoice_number,
    success_url: urls.success_url,
    cancel_url: urls.cancel_url,
    idempotency_key: `invoice-checkout:${invoice.id}:${invoice.amount_due_cents}`,
  })
  if (!checkout.url) {
    throw new Error('Checkout session did not return a redirect URL.')
  }
  redirect(checkout.url)
}

export function portalInvoiceCheckoutErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return error.message
  }
  return 'Couldn’t start Stripe Checkout. Try again.'
}
