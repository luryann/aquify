import { expect, test } from '@playwright/test'

import { expectLoaded, getUser, newAuthedContext } from './support/app'
import { scenarioFixture } from './support/fixtures'

test('public registration requests can be approved from the admin approvals queue', async ({ browser, page }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId
  const email = fixture.email('approve-user')

  await page.goto(`/register?org_id=${orgId}`)
  await page.getByLabel('First name').fill('Avery')
  await page.getByLabel('Last name').fill('Approval')
  await page.getByLabel('Email address').fill(email)
  await page.getByRole('button', { name: /Coach/ }).click()
  await page.getByLabel(/Club code/).fill('E2E-CLUB')
  await page.getByLabel(/Note to administrator/).fill('Needs coach access for the fall group.')

  const registerResponsePromise = page.waitForResponse((response) =>
    response.url().endsWith('/api/v1/register') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: 'Submit access request' }).click()
  await expect(page.getByText('Request received.')).toBeVisible()
  const registerResponse = await registerResponsePromise
  const registerPayload = await registerResponse.json() as { user_id: string }

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

  await adminContext.close()
})

test('public registration requests can be rejected from the admin approvals queue', async ({ browser, page }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId
  const email = fixture.email('reject-user')

  await page.goto(`/register?org_id=${orgId}`)
  await page.getByLabel('First name').fill('Riley')
  await page.getByLabel('Last name').fill('Reject')
  await page.getByLabel('Email address').fill(email)
  await page.getByRole('button', { name: /Coach/ }).click()

  const registerResponsePromise = page.waitForResponse((response) =>
    response.url().endsWith('/api/v1/register') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: 'Submit access request' }).click()
  await expect(page.getByText('Request received.')).toBeVisible()
  const registerResponse = await registerResponsePromise
  const registerPayload = await registerResponse.json() as { user_id: string }

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

  await adminContext.close()
})
