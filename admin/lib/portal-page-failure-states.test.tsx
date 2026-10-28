import test, { before, after } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { JSDOM } from 'jsdom'

import { SessionProvider } from './session.tsx'

const require = createRequire(import.meta.url)
const previousCSSLoader = require.extensions['.css']
// Next loads global CSS in the browser. These DOM tests exercise page behavior
// in Node, where extracted stylesheets need no execution.
before(() => { require.extensions['.css'] = () => {} })
after(() => {
  if (previousCSSLoader) require.extensions['.css'] = previousCSSLoader
  else delete require.extensions['.css']
})

type FetchHandler = (input: string, init?: RequestInit) => Response | Promise<Response>

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

function installDom() {
  const dom = new JSDOM('<!doctype html><html><body></body></html>', {
    url: 'http://localhost/portal',
  })
  const { window } = dom

  Object.defineProperty(globalThis, 'window', { configurable: true, value: window })
  Object.defineProperty(globalThis, 'self', { configurable: true, value: window })
  Object.defineProperty(globalThis, 'document', { configurable: true, value: window.document })
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: window.navigator })
  Object.defineProperty(globalThis, 'HTMLElement', { configurable: true, value: window.HTMLElement })
  Object.defineProperty(globalThis, 'Node', { configurable: true, value: window.Node })
  Object.defineProperty(globalThis, 'Event', { configurable: true, value: window.Event })
  Object.defineProperty(globalThis, 'CustomEvent', { configurable: true, value: window.CustomEvent })
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: window.localStorage })
  Object.defineProperty(globalThis, 'IS_REACT_ACT_ENVIRONMENT', { configurable: true, value: true })
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1280, writable: true })
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: () => ({
      matches: false,
      media: '',
      onchange: null,
      addListener() {},
      removeListener() {},
      addEventListener() {},
      removeEventListener() {},
      dispatchEvent() { return false },
    }),
  })

  return () => {
    dom.window.close()
    // @ts-expect-error cleanup test globals
    delete globalThis.window
    // @ts-expect-error cleanup test globals
    delete globalThis.self
    // @ts-expect-error cleanup test globals
    delete globalThis.document
    // @ts-expect-error cleanup test globals
    delete globalThis.navigator
    // @ts-expect-error cleanup test globals
    delete globalThis.HTMLElement
    // @ts-expect-error cleanup test globals
    delete globalThis.Node
    // @ts-expect-error cleanup test globals
    delete globalThis.Event
    // @ts-expect-error cleanup test globals
    delete globalThis.CustomEvent
    // @ts-expect-error cleanup test globals
    delete globalThis.localStorage
    // @ts-expect-error cleanup test globals
    delete globalThis.IS_REACT_ACT_ENVIRONMENT
  }
}

async function renderPortalPage(
  element: React.ReactElement,
  fetchHandler: FetchHandler,
) {
  const cleanupDom = installDom()
  const previousFetch = globalThis.fetch
  const previousConsoleError = console.error
  Object.defineProperty(globalThis, 'fetch', {
    configurable: true,
    value: (input: string | URL | Request, init?: RequestInit) => {
      const path = typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.toString()
          : input.url
      if (path.includes('/api/v1/auth/session')) {
        return Promise.resolve(jsonResponse(200, {
          status: 'ok',
          user: {
            user_id: 'test-parent',
            email: 'parent@dare.local',
            name: 'Test Parent',
            org_id: '00000000-0000-0000-0000-00000000a001',
            household_id: 'household_123',
            swimmer_id: 'swimmer_123',
            roles: ['parent'],
          },
        }))
      }
      return Promise.resolve(fetchHandler(path, init))
    },
  })
  console.error = () => {}

  localStorage.removeItem('dare:dev_token')

  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(<SessionProvider>{element}</SessionProvider>)
  })

  const cleanup = async () => {
    await act(async () => {
      root.unmount()
    })
    container.remove()
    Object.defineProperty(globalThis, 'fetch', { configurable: true, value: previousFetch })
    console.error = previousConsoleError
    cleanupDom()
  }

  return { container, cleanup }
}

async function waitFor(assertion: () => void, timeoutMs = 3000) {
  const start = Date.now()
  let lastError: unknown
  while (Date.now() - start < timeoutMs) {
    try {
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 25))
      })
      assertion()
      return
    } catch (error) {
      lastError = error
    }
  }
  throw lastError
}

function mockNextNavigation(overrides: {
  useParams?: () => Record<string, string>
  useSearchParams?: () => URLSearchParams
}) {
  const modulePath = require.resolve('next/navigation')
  const previousModule = require.cache[modulePath]
  require.cache[modulePath] = {
    exports: {
      useParams: overrides.useParams ?? (() => ({})),
      useSearchParams: overrides.useSearchParams ?? (() => new URLSearchParams()),
    },
  } as NodeModule

  return () => {
    if (previousModule) {
      require.cache[modulePath] = previousModule
      return
    }
    delete require.cache[modulePath]
  }
}

test('calendar page shows a load failure instead of a fake empty upcoming state', { concurrency: false }, async () => {
  const { default: CalendarPage } = await import('../app/(portal)/portal/calendar/page.tsx')
  const { container, cleanup } = await renderPortalPage(
    <CalendarPage />,
    (input) => {
      if (input.includes('/api/v1/meets')) {
        return jsonResponse(503, { message: 'Calendar feed is temporarily unavailable.' })
      }
      if (input.includes('/api/v1/practices')) {
        return jsonResponse(503, { message: 'Practice feed is temporarily unavailable.' })
      }
      return jsonResponse(404, { message: `Unhandled fetch: ${input}` })
    },
  )

  try {
    await waitFor(() => {
      const text = container.textContent ?? ''
      assert.match(text, /We could not load this page\./)
      assert.match(text, /Retry/)
      assert.doesNotMatch(text, /No upcoming events/)
    })
  } finally {
    await cleanup()
  }
})

test('communications page shows a load failure instead of a fake empty announcements state', { concurrency: false }, async () => {
  const { default: CommunicationsPage } = await import('../app/(portal)/portal/communications/page.tsx')
  const { container, cleanup } = await renderPortalPage(
    <CommunicationsPage />,
    (input) => {
      if (input.includes('/api/v1/announcements?status=published')) {
        return jsonResponse(503, { message: 'Announcements backend unavailable.' })
      }
      if (input.includes('/api/v1/communication-preferences')) {
        return jsonResponse(200, { preferences: [] })
      }
      return jsonResponse(404, { message: `Unhandled fetch: ${input}` })
    },
  )

  try {
    await waitFor(() => {
      const text = container.textContent ?? ''
      assert.match(text, /Announcements are unavailable/)
      assert.match(text, /Couldn’t load announcements\. Try again\./)
      assert.match(text, /Retry/)
      assert.doesNotMatch(text, /No announcements/)
    })
  } finally {
    await cleanup()
  }
})

test('invoice detail page shows a load failure instead of a fake not-found state', { concurrency: false }, async () => {
  const restoreNavigation = mockNextNavigation({
    useParams: () => ({ id: 'inv_123' }),
    useSearchParams: () => new URLSearchParams(),
  })
  const { default: InvoiceDetailPage } = await import('../app/(portal)/portal/billing/invoices/[id]/page.tsx')
  const { container, cleanup } = await renderPortalPage(
    <InvoiceDetailPage />,
    (input) => {
      if (input.includes('/api/v1/invoices/inv_123')) {
        return jsonResponse(503, { message: 'Invoice backend unavailable.' })
      }
      return jsonResponse(404, { message: `Unhandled fetch: ${input}` })
    },
  )

  try {
    await waitFor(() => {
      const text = container.textContent ?? ''
      assert.match(text, /Invoice details are unavailable right now\./)
      assert.match(text, /Retry/)
      assert.doesNotMatch(text, /Invoice not found/)
    })
  } finally {
    await cleanup()
    restoreNavigation()
  }
})

test('meet detail page shows a load failure instead of a fake not-found state', { concurrency: false }, async () => {
  const restoreNavigation = mockNextNavigation({
    useParams: () => ({ id: 'meet_123' }),
  })
  const { default: MeetDetailPage } = await import('../app/(portal)/portal/meets/[id]/page.tsx')
  const { container, cleanup } = await renderPortalPage(
    <MeetDetailPage />,
    (input) => {
      if (input.includes('/api/v1/meets/meet_123')) {
        return jsonResponse(503, { message: 'Meet backend unavailable.' })
      }
      if (input.includes('/api/v1/members/swimmers')) {
        return jsonResponse(503, { message: 'Swimmer backend unavailable.' })
      }
      return jsonResponse(404, { message: `Unhandled fetch: ${input}` })
    },
  )

  try {
    await waitFor(() => {
      const text = container.textContent ?? ''
      assert.match(text, /Meet details are unavailable right now\./)
      assert.match(text, /Retry/)
      assert.doesNotMatch(text, /Meet not found/)
    })
  } finally {
    await cleanup()
    restoreNavigation()
  }
})

test('registrations page shows a load failure instead of a fake empty household state', { concurrency: false }, async () => {
  const { default: RegistrationsPage } = await import('../app/(portal)/portal/registrations/page.tsx')
  const { container, cleanup } = await renderPortalPage(
    <RegistrationsPage />,
    (input) => {
      if (input.includes('/api/v1/registrations')) {
        return jsonResponse(503, { message: 'Registrations backend unavailable.' })
      }
      if (input.includes('/api/v1/members/swimmers')) {
        return jsonResponse(200, { swimmers: [] })
      }
      if (input.includes('/api/v1/groups')) {
        return jsonResponse(200, { groups: [] })
      }
      if (input.includes('/api/v1/document-types')) {
        return jsonResponse(200, { document_types: [] })
      }
      if (input.includes('/api/v1/document-submissions')) {
        return jsonResponse(200, { submissions: [] })
      }
      return jsonResponse(404, { message: `Unhandled fetch: ${input}` })
    },
  )

  try {
    await waitFor(() => {
      const text = container.textContent ?? ''
      assert.match(text, /Registration data is unavailable/)
      assert.match(text, /Couldn’t load registrations\. Try again\./)
      assert.match(text, /Retry/)
      assert.doesNotMatch(text, /No registrations found for your household\./)
    })
  } finally {
    await cleanup()
  }
})

test('portal meets page shows a load failure instead of a fake empty meet list', { concurrency: false }, async () => {
  const { default: MeetsPage } = await import('../app/(portal)/portal/meets/page.tsx')
  const { container, cleanup } = await renderPortalPage(
    <MeetsPage />,
    (input) => {
      if (input.includes('/api/v1/meets')) {
        return jsonResponse(503, { message: 'Meets backend unavailable.' })
      }
      if (input.includes('/api/v1/members/swimmers')) {
        return jsonResponse(200, { swimmers: [] })
      }
      return jsonResponse(404, { message: `Unhandled fetch: ${input}` })
    },
  )

  try {
    await waitFor(() => {
      const text = container.textContent ?? ''
      assert.match(text, /Meet data is unavailable/)
      assert.match(text, /Couldn’t load meet schedules\. Try again\./)
      assert.match(text, /Retry/)
      assert.doesNotMatch(text, /No meets scheduled yet\./)
    })
  } finally {
    await cleanup()
  }
})

test('swimmer meet detail page shows a load failure instead of a fake not-found state', { concurrency: false }, async () => {
  const restoreNavigation = mockNextNavigation({
    useParams: () => ({ id: 'meet_456' }),
  })
  const { default: SwimmerMeetDetailPage } = await import('../app/(swimmer)/swim/meets/[id]/page.tsx')
  const { container, cleanup } = await renderPortalPage(
    <SwimmerMeetDetailPage />,
    (input) => {
      if (input.includes('/api/v1/meets/meet_456')) {
        return jsonResponse(503, { message: 'Meet detail backend unavailable.' })
      }
      return jsonResponse(404, { message: `Unhandled fetch: ${input}` })
    },
  )

  try {
    await waitFor(() => {
      const text = container.textContent ?? ''
      assert.match(text, /We could not load this page\./)
      assert.match(text, /Couldn’t load meet details\. Try again\./)
      assert.match(text, /Retry/)
      assert.doesNotMatch(text, /Meet not found/)
    })
  } finally {
    await cleanup()
    restoreNavigation()
  }
})

test('billing page shows a load failure instead of fake empty invoice or payment states', { concurrency: false }, async () => {
  const { default: BillingPage } = await import('../app/(portal)/portal/billing/page.tsx')
  const { container, cleanup } = await renderPortalPage(
    <BillingPage />,
    (input) => {
      if (input.includes('/api/v1/households/household_123/invoices')) {
        return jsonResponse(503, { message: 'Billing backend unavailable.' })
      }
      if (input.includes('/api/v1/households/household_123/autopay')) {
        return jsonResponse(200, { status: null })
      }
      return jsonResponse(404, { message: `Unhandled fetch: ${input}` })
    },
  )

  try {
    await waitFor(() => {
      const text = container.textContent ?? ''
      assert.match(text, /Couldn’t load billing activity\. Try again\./)
      assert.match(text, /Retry/)
      assert.doesNotMatch(text, /No invoices found\./)
      assert.doesNotMatch(text, /No payment history yet\./)
    })
  } finally {
    await cleanup()
  }
})

test('performance page shows a load failure instead of fake empty best-time state', { concurrency: false }, async () => {
  const { default: PerformancePage } = await import('../app/(swimmer)/swim/performance/page.tsx')
  const { container, cleanup } = await renderPortalPage(
    <PerformancePage />,
    (input) => {
      if (input.includes('/api/v1/members/swimmers')) {
        return jsonResponse(200, {
          swimmers: [{
            id: 'swimmer_123',
            household_id: 'household_123',
            first_name: 'Test',
            last_name: 'Swimmer',
            gender: 'female',
          }],
        })
      }
      if (input.includes('/best-times')) {
        return jsonResponse(503, { message: 'Best times backend unavailable.' })
      }
      if (input.includes('/time-standards')) {
        return jsonResponse(200, { standards: [] })
      }
      return jsonResponse(404, { message: `Unhandled fetch: ${input}` })
    },
  )

  try {
    await waitFor(() => {
      const text = container.textContent ?? ''
      assert.match(text, /We could not load this page\./)
      assert.match(text, /Couldn’t load swimmer performance\. Try again\./)
      assert.match(text, /Retry/)
      assert.doesNotMatch(text, /No best times recorded yet\./)
    })
  } finally {
    await cleanup()
  }
})
