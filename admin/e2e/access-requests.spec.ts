import type { Page } from '@playwright/test'

import { expect, test } from './support/test'

import { expectLoaded, getUser, newAuthedContext } from './support/app'
import { scenarioFixture } from './support/fixtures'

async function submitCoachRegistration(page: Page, orgId: string, input: { firstName: string; lastName: string; email: string }, extra?: () => Promise<void>) {
  await page.goto(`/register?org_id=${orgId}`)
  await page.getByLabel('First name').fill(input.firstName)
  await page.getByLabel('Last name').fill(input.lastName)
  await page.getByLabel('Email address').fill(input.email)
  await page.getByRole('button', { name: /Coach/ }).click()
  await extra?.()

  const registerResponsePromise = page.waitForResponse((response) =>
    response.url().endsWith('/api/v1/register') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: 'Submit access request' }).click()
  await expect(page.getByText('Request received.')).toBeVisible()
  const registerResponse = await registerResponsePromise
  return await registerResponse.json() as { user_id: string }
}

test('public registration requests can be approved from the admin approvals queue', async ({ browser, page }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId
  const email = fixture.email('approve-user')

  const registerPayload = await submitCoachRegistration(page, orgId, { firstName: 'Avery', lastName: 'Approval', email }, async () => {
    await page.getByLabel(/Club code/).fill('E2E-CLUB')
    await page.getByLabel(/Note to administrator/).fill('Needs coach access for the fall group.')
  })

  const adminContext = await newAuthedContext(browser, {
    userId: fixture.userId('admin-approve'),
    orgId,
    roles: ['admin'],
    mfaAuthenticated: true,
    name: 'Approval Admin',
  })

  const adminPage = await adminContext.newPage()
  await adminPage.goto('/members/approvals')
  await expectLoaded(adminPage, 'Pending Approvals')
  const pendingCard = adminPage.locator('.card').filter({ hasText: email }).first()
  await expect(pendingCard).toContainText('Coach')
  await pendingCard.getByRole('button', { name: 'Approve' }).click()
  await expect(pendingCard).toHaveCount(0)

  const approvedUser = await getUser(adminContext, orgId, registerPayload.user_id)
  expect(approvedUser.user.enabled).toBe(true)
  expect(approvedUser.user.attributes?.registration_status?.[0]).toBe('active')
  expect(approvedUser.user.attributes?.requested_role?.[0]).toBe('coach')

})

test('public registration requests can be rejected from the admin approvals queue', async ({ browser, page }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId
  const email = fixture.email('reject-user')

  const registerPayload = await submitCoachRegistration(page, orgId, { firstName: 'Riley', lastName: 'Reject', email })

  const adminContext = await newAuthedContext(browser, {
    userId: fixture.userId('admin-reject'),
    orgId,
    roles: ['admin'],
    mfaAuthenticated: true,
    name: 'Reject Admin',
  })

  const adminPage = await adminContext.newPage()
  await adminPage.goto('/members/approvals')
  await expectLoaded(adminPage, 'Pending Approvals')
  const pendingCard = adminPage.locator('.card').filter({ hasText: email }).first()
  await pendingCard.getByRole('button', { name: 'Reject' }).click()
  await pendingCard.getByPlaceholder('Optional note for internal records…').fill('Missing verification from club staff.')
  await pendingCard.getByRole('button', { name: 'Confirm Reject' }).click()
  await expect(pendingCard).toHaveCount(0)

  const rejectedUser = await getUser(adminContext, orgId, registerPayload.user_id)
  expect(rejectedUser.user.enabled).toBe(false)
  expect(rejectedUser.user.attributes?.registration_status?.[0]).toBe('rejected')

})
