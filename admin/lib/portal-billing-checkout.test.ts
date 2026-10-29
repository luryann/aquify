import test from 'node:test'
import assert from 'node:assert/strict'

import { ApiError } from './api.ts'
import type { ApiClient } from './api.ts'
import { createInvoiceCheckoutSession, type InvoiceRecord } from './adminData.ts'
import {
  buildPortalInvoiceCheckoutURLs,
  isPortalInvoiceCheckoutDisabled,
  launchPortalInvoiceCheckout,
  portalInvoiceCheckoutErrorMessage,
} from './portal-invoice-checkout.ts'

function makeInvoice(overrides: Partial<InvoiceRecord> = {}): InvoiceRecord {
  return {
    id: 'inv_123',
    org_id: 'org_portal',
    household_id: 'hh_123',
    invoice_number: 'INV-123',
    status: 'issued',
    total_cents: 9500,
    amount_paid_cents: 0,
    amount_due_cents: 9500,
    created_at: '',
    ...overrides,
  }
}

test('createInvoiceCheckoutSession posts invoice checkout payload and idempotency header', async () => {
  let receivedPath = ''
  let receivedBody: unknown
  let receivedHeaders: Record<string, string> | undefined

  const api = {
    post: async (path: string, body?: unknown, headers?: Record<string, string>) => {
      receivedPath = path
      receivedBody = body
      receivedHeaders = headers
      return {
        checkout_session: {
          id: 'cs_test_123',
          url: 'https://checkout.stripe.com/pay/cs_test_123',
        },
      }
    },
  } as unknown as ApiClient

  const result = await createInvoiceCheckoutSession(api, 'org_portal', {
    invoice_id: 'inv_123',
    amount_cents: 9500,
    description: 'Spring dues',
    success_url: 'https://example.com/success',
    cancel_url: 'https://example.com/cancel',
    idempotency_key: 'invoice-checkout:inv_123:9500',
  })

  assert.equal(receivedPath, '/api/v1/stripe/checkout-sessions')
  assert.deepEqual(receivedBody, {
    invoice_id: 'inv_123',
    amount_cents: 9500,
    description: 'Spring dues',
    success_url: 'https://example.com/success',
    cancel_url: 'https://example.com/cancel',
  })
  assert.deepEqual(receivedHeaders, {
    'X-Org-Id': 'org_portal',
    'Idempotency-Key': 'invoice-checkout:inv_123:9500',
  })
  assert.deepEqual(result, {
    id: 'cs_test_123',
    url: 'https://checkout.stripe.com/pay/cs_test_123',
  })
})

test('launchPortalInvoiceCheckout redirects to Stripe checkout URL on success', async () => {
  let redirectedTo = ''
  const api = {
    post: async () => ({
      checkout_session: {
        id: 'cs_live_123',
        url: 'https://checkout.stripe.com/pay/cs_live_123',
      },
    }),
  } as unknown as ApiClient

  await launchPortalInvoiceCheckout({
    api,
    orgId: 'org_portal',
    invoice: makeInvoice(),
    origin: 'https://app.example.com',
    redirect: (url) => {
      redirectedTo = url
    },
  })

  assert.equal(redirectedTo, 'https://checkout.stripe.com/pay/cs_live_123')
})

test('launchPortalInvoiceCheckout uses a fresh idempotency key per attempt', async () => {
  const keys: string[] = []
  const api = {
    post: async (_path: string, _body?: unknown, headers?: Record<string, string>) => {
      keys.push(headers?.['Idempotency-Key'] ?? '')
      return { checkout_session: { id: 'cs_1', url: 'https://checkout.stripe.com/pay/cs_1' } }
    },
  } as unknown as ApiClient
  const run = (attemptId?: string) =>
    launchPortalInvoiceCheckout({ api, orgId: 'org_portal', invoice: makeInvoice(), origin: 'https://app.example.com', redirect: () => {}, attemptId })

  await run()
  await run()
  await run('fixed')
  await run('fixed')
  assert.notEqual(keys[0], keys[1])
  assert.match(keys[0], /^invoice-checkout:inv_123:9500:.+/)
  assert.equal(keys[2], keys[3])
})

test('portal checkout helpers expose redirect-disabled and failure states', async () => {
  const invoice = makeInvoice()

  assert.equal(isPortalInvoiceCheckoutDisabled(invoice, false), false)
  assert.equal(isPortalInvoiceCheckoutDisabled(invoice, true), true)
  assert.equal(
    isPortalInvoiceCheckoutDisabled(makeInvoice({ status: 'paid', amount_due_cents: 0 }), false),
    true,
  )
  assert.equal(
    isPortalInvoiceCheckoutDisabled(makeInvoice({ status: 'issued', amount_due_cents: 0 }), false),
    true,
  )
  assert.deepEqual(buildPortalInvoiceCheckoutURLs('https://app.example.com', 'inv_123'), {
    success_url: 'https://app.example.com/portal/billing/invoices/inv_123?checkout=success',
    cancel_url: 'https://app.example.com/portal/billing/invoices/inv_123?checkout=cancelled',
  })

  const api = {
    post: async () => {
      throw new ApiError(502, 'stripe_error', 'Failed to create checkout session.')
    },
  } as unknown as ApiClient

  await assert.rejects(async () => {
    await launchPortalInvoiceCheckout({
      api,
      orgId: 'org_portal',
      invoice,
      origin: 'https://app.example.com',
      redirect: () => {
        throw new Error('redirect should not be called on failure')
      },
    })
  })
  assert.equal(
    portalInvoiceCheckoutErrorMessage(new ApiError(502, 'stripe_error', 'Failed to create checkout session.')),
    'Failed to create checkout session.',
  )
  assert.equal(
    portalInvoiceCheckoutErrorMessage(new Error('unexpected')),
    'Couldn’t start Stripe Checkout. Try again.',
  )
})
