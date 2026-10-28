import { expect, test } from '@playwright/test'

import {
  createGroup,
  createLinkedSwimmerUser,
  createHousehold,
  createMeet,
  createPerformanceResult,
  createTimeStandard,
  createSwimmer,
  expectLoaded,
  getCommunicationPreferences,
  getHouseholdAutopay,
  listCommitmentEventSelections,
  listMeetEvents,
  listRegistrations,
  newAuthedContext,
  overrideBestTime,
  publishCommitmentRequest,
  respondToCommitment,
  selectCommitmentEvent,
  transitionMeet,
} from './support/app'
import { scenarioFixture } from './support/fixtures'

test('parent portal registration wizard creates live registrations for multiple swimmers', async ({ browser }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId
  const adminContext = await newAuthedContext(browser, {
    userId: fixture.userId('admin-portal'),
    orgId,
    roles: ['admin'],
    name: 'Portal Admin',
  })

  const household = await createHousehold(adminContext, orgId, fixture.text('Household Portal'))
  const swimmerOne = await createSwimmer(adminContext, orgId, household.id, {
    firstName: 'Mia',
    lastName: 'Example',
    dateOfBirth: '2012-04-15',
  })
  const swimmerTwo = await createSwimmer(adminContext, orgId, household.id, {
    firstName: 'Noah',
    lastName: 'Example',
    dateOfBirth: '2010-09-20',
  })
  const group = await createGroup(adminContext, orgId, {
    code: fixture.code('group'),
    name: 'Senior Bronze',
  })

  const parentContext = await newAuthedContext(browser, {
    userId: fixture.userId('parent-portal'),
    orgId,
    householdId: household.id,
    roles: ['parent'],
    name: 'Portal Parent',
  })

  const parentPage = await parentContext.newPage()
  await parentPage.goto('/portal/registrations/new')
  await expectLoaded(parentPage, 'New Registration')

  await parentPage.getByRole('button', { name: new RegExp(group.name, 'i') }).click()
  await parentPage.getByRole('button', { name: /Continue/ }).click()

  await parentPage.getByRole('button', { name: new RegExp(`${swimmerOne.first_name} ${swimmerOne.last_name}`, 'i') }).click()
  await parentPage.getByRole('button', { name: new RegExp(`${swimmerTwo.first_name} ${swimmerTwo.last_name}`, 'i') }).click()
  await parentPage.getByRole('button', { name: /Continue/ }).click()

  await parentPage.getByRole('button', { name: /Continue/ }).click()
  await parentPage.getByRole('button', { name: 'Submit Registration' }).click()

  await expect(parentPage.getByText('Registration submitted')).toBeVisible()
  await parentPage.getByRole('button', { name: 'Back to Registrations' }).click()

  await expectLoaded(parentPage, 'Registration Center')
  await expect(parentPage.getByText(`${swimmerOne.first_name} ${swimmerOne.last_name}`)).toBeVisible()
  await expect(parentPage.getByText(`${swimmerTwo.first_name} ${swimmerTwo.last_name}`)).toBeVisible()
  await expect(parentPage.getByText(group.name, { exact: true })).toBeVisible()

  const registrationPayload = await listRegistrations(parentContext, orgId)
  const registrations = registrationPayload.registrations ?? []
  expect(registrations.filter((registration) => registration.group_id === group.id).map((registration) => registration.swimmer_id).sort()).toEqual(
    [swimmerOne.id, swimmerTwo.id].sort(),
  )

  await parentContext.close()
  await adminContext.close()
})

test('swimmer portal bio edits persist after refresh', async ({ browser }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId
  const adminContext = await newAuthedContext(browser, {
    userId: fixture.userId('admin-swimmer'),
    orgId,
    roles: ['admin'],
    name: 'Swimmer Admin',
  })

  const household = await createHousehold(adminContext, orgId, fixture.text('Household Swim'))
  const swimmer = await createSwimmer(adminContext, orgId, household.id, {
    firstName: 'Sky',
    lastName: 'Lane',
    dateOfBirth: '2011-01-11',
    notes: 'Original bio',
  })

  const swimmerUserId = await createLinkedSwimmerUser(adminContext, orgId, swimmer.id, fixture.email('swimmer'), 'Sky', 'Lane')
  const swimmerContext = await newAuthedContext(browser, {
    userId: swimmerUserId,
    orgId,
    householdId: household.id,
    roles: ['swimmer'],
    name: 'Sky Lane',
  })

  const swimmerPage = await swimmerContext.newPage()
  await swimmerPage.goto('/swim/profile')
  await expectLoaded(swimmerPage, /Sky Lane/)

  const nextBio = fixture.text('Updated bio')
  const bioField = swimmerPage.getByPlaceholder("Tell your team about yourself. Your goals, favorite events, or anything you'd like to share...")
  await expect(bioField).toHaveValue('Original bio')
  await bioField.fill(nextBio)
  const saveResponsePromise = swimmerPage.waitForResponse((response) =>
    response.url().endsWith(`/api/v1/members/swimmers/${swimmer.id}`) && response.request().method() === 'PATCH',
  )
  await swimmerPage.getByRole('button', { name: 'Save Bio' }).click()
  const saveResponse = await saveResponsePromise
  expect(saveResponse.ok()).toBe(true)
  expect(saveResponse.request().postDataJSON()).toEqual({ notes: nextBio })
  expect((await saveResponse.json()).swimmer.notes).toBe(nextBio)
  await expect(swimmerPage.getByText('Saved', { exact: true })).toBeVisible()

  await swimmerPage.reload()
  await expect(bioField).toHaveValue(nextBio)
  const savedBio = swimmerPage.getByRole('paragraph').filter({ hasText: nextBio })
  await expect(savedBio).toHaveText(nextBio)
  await expect(savedBio).toBeVisible()

  await swimmerContext.close()
  await adminContext.close()
})

test('parent portal communication preferences persist after refresh', async ({ browser }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId

  const adminContext = await newAuthedContext(browser, {
    userId: fixture.userId('admin-comms'),
    orgId,
    roles: ['admin'],
    name: 'Communications Admin',
  })

  const household = await createHousehold(adminContext, orgId, fixture.text('Household Comms'))
  const parentContext = await newAuthedContext(browser, {
    userId: fixture.userId('parent-comms'),
    orgId,
    householdId: household.id,
    roles: ['parent'],
    name: 'Portal Parent',
  })

  const parentPage = await parentContext.newPage()
  await parentPage.goto('/portal/communications')
  await expectLoaded(parentPage, 'Communications')
  await parentPage.getByRole('button', { name: 'Preferences' }).click()

  const pushToggle = parentPage.getByRole('switch', { name: 'Club Announcements push' })
  await pushToggle.click()
  await parentPage.getByRole('button', { name: 'Save Preferences' }).click()
  await expect(parentPage.getByText('Saved', { exact: true })).toBeVisible()

  const prefsPayload = await getCommunicationPreferences(parentContext, orgId)
  const announcementPreference = prefsPayload.preferences?.find((preference) => preference.event_id === 'announcement')
  expect(announcementPreference?.push_enabled).toBe(true)

  await parentPage.reload()
  await expectLoaded(parentPage, 'Communications')
  await parentPage.getByRole('button', { name: 'Preferences' }).click()
  await expect(pushToggle).toBeChecked()

  await parentContext.close()
  await adminContext.close()
})

test('parent portal autopay settings persist after refresh', async ({ browser }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId

  const adminContext = await newAuthedContext(browser, {
    userId: fixture.userId('admin-billing'),
    orgId,
    roles: ['admin'],
    name: 'Billing Admin',
  })

  const household = await createHousehold(adminContext, orgId, fixture.text('Household Billing'))
  const parentContext = await newAuthedContext(browser, {
    userId: fixture.userId('parent-billing'),
    orgId,
    householdId: household.id,
    roles: ['parent'],
    name: 'Billing Parent',
  })

  const parentPage = await parentContext.newPage()
  await parentPage.goto('/portal/billing')
  await expectLoaded(parentPage, 'Billing')
  await parentPage.getByRole('button', { name: 'Autopay' }).click()

  const paymentLabel = fixture.text('Family Visa')
  await parentPage.getByLabel('Saved Label').fill(paymentLabel)
  await parentPage.getByLabel('Card Brand').fill('Visa')
  await parentPage.getByLabel('Last 4').fill('1111')
  await parentPage.getByLabel('Expiry Month').fill('09')
  await parentPage.getByLabel('Expiry Year').fill('2029')
  await parentPage.getByRole('button', { name: '7 days early' }).click()
  await parentPage.getByRole('button', { name: 'Enable autopay' }).click()
  await expect(parentPage.getByText('Autopay settings saved.')).toBeVisible()
  await expect(parentPage.getByText('ACTIVE')).toBeVisible()

  const autopayPayload = await getHouseholdAutopay(parentContext, orgId, household.id)
  expect(autopayPayload.autopay.enabled).toBe(true)
  expect(autopayPayload.autopay.trigger_days_before_due).toBe(7)
  expect(autopayPayload.autopay.payment_method_label).toBe(paymentLabel)

  await parentPage.reload()
  await expectLoaded(parentPage, 'Billing')
  await parentPage.getByRole('button', { name: 'Autopay' }).click()
  await expect(parentPage.getByText('ACTIVE')).toBeVisible()
  await expect(parentPage.getByLabel('Saved Label')).toHaveValue(paymentLabel)
  await expect(parentPage.getByRole('button', { name: 'Update autopay' })).toBeVisible()

  await parentContext.close()
  await adminContext.close()
})

test('swimmer portal cut qualifications render live time standard results', async ({ browser }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId

  const adminContext = await newAuthedContext(browser, {
    userId: fixture.userId('admin-performance'),
    orgId,
    roles: ['admin'],
    name: 'Performance Admin',
  })

  const household = await createHousehold(adminContext, orgId, fixture.text('Household Performance'))
  const swimmer = await createSwimmer(adminContext, orgId, household.id, {
    firstName: 'Taylor',
    lastName: 'Fast',
    dateOfBirth: '2012-07-08',
  })

  const standard = await createTimeStandard(adminContext, orgId, {
    name: fixture.text('AAA 100 Free'),
    seasonYear: 2026,
    standardCode: 'AAA',
    description: 'E2E qualifying standard',
    eventDistance: 100,
    stroke: 'free',
    course: 'SCY',
    gender: 'F',
    ageMin: 10,
    ageMax: 18,
    timeMs: 60000,
  })
  await overrideBestTime(adminContext, orgId, swimmer.id, {
    eventDistance: 100,
    stroke: 'free',
    course: 'SCY',
    bestTimeMs: 58000,
    reason: 'E2E seeded performance result',
  })

  const swimmerUserId = await createLinkedSwimmerUser(adminContext, orgId, swimmer.id, fixture.email('swimmer'), 'Taylor', 'Fast')
  const swimmerContext = await newAuthedContext(browser, {
    userId: swimmerUserId,
    orgId,
    householdId: household.id,
    roles: ['swimmer'],
    name: 'Taylor Fast',
  })

  const swimmerPage = await swimmerContext.newPage()
  await swimmerPage.goto('/swim/performance')
  await expectLoaded(swimmerPage, 'Performance')
  await swimmerPage.getByRole('button', { name: 'Cut qualification' }).click()

  await expect(swimmerPage.getByText('Qualified standards')).toBeVisible()
  await expect(swimmerPage.getByText(standard.standard_code)).toBeVisible()
  await expect(swimmerPage.getByText('Qualified').first()).toBeVisible()
  await expect(swimmerPage.getByText('Under the cut by 2.00s.')).toBeVisible()

  await swimmerContext.close()
  await adminContext.close()
})

test('parent portal meet event selections reload from live commitments', async ({ browser }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId

  const adminContext = await newAuthedContext(browser, {
    userId: fixture.userId('admin-meet-selection'),
    orgId,
    roles: ['admin'],
    name: 'Meet Admin',
  })

  const household = await createHousehold(adminContext, orgId, fixture.text('Household Meet Selection'))
  const swimmer = await createSwimmer(adminContext, orgId, household.id, {
    firstName: 'Jordan',
    lastName: 'Relay',
    dateOfBirth: '2012-03-04',
  })
  const meet = await createMeet(adminContext, orgId, {
    name: fixture.text('Selection Invite'),
    eventNumber: fixture.code('event', 4),
  })
  await transitionMeet(adminContext, orgId, meet.id, 'published')
  const commitmentPayload = await publishCommitmentRequest(adminContext, orgId, meet.id)
  const commitment = commitmentPayload.commitments.find((item) => item.swimmer_id === swimmer.id)
  expect(commitment).toBeTruthy()
  const events = await listMeetEvents(adminContext, orgId, meet.id)
  expect(events).toHaveLength(1)

  const parentContext = await newAuthedContext(browser, {
    userId: fixture.userId('parent-meet-selection'),
    orgId,
    householdId: household.id,
    roles: ['parent'],
    name: 'Meet Parent',
  })

  await respondToCommitment(parentContext, orgId, meet.id, commitment!.id, 'committed_yes')
  await selectCommitmentEvent(parentContext, orgId, meet.id, commitment!.id, events[0].id)
  const persistedSelections = await listCommitmentEventSelections(parentContext, orgId, meet.id, commitment!.id)
  expect(persistedSelections.map((selection) => selection.meet_event_id)).toContain(events[0].id)

  const parentPage = await parentContext.newPage()
  await parentPage.goto(`/portal/meets/${meet.id}`)
  await expectLoaded(parentPage, meet.name)
  await expect(parentPage.getByText('Going', { exact: true })).toBeVisible()
  await expect(parentPage.getByText('Pick events', { exact: true })).toBeVisible()
  const eventRow = parentPage.getByText(`#${events[0].event_number}`, { exact: true }).locator('..')
  await expect(eventRow).toBeVisible()
  await expect(eventRow.locator('svg')).toBeVisible()

  await parentPage.reload()
  await expectLoaded(parentPage, meet.name)
  await expect(parentPage.getByText('Going', { exact: true })).toBeVisible()
  await expect(eventRow).toBeVisible()
  await expect(eventRow.locator('svg')).toBeVisible()

  await parentContext.close()
  await adminContext.close()
})

test('parent portal swimmer activity history renders live backend activity', async ({ browser }) => {
  const fixture = scenarioFixture(test.info())
  const orgId = fixture.orgId

  const adminContext = await newAuthedContext(browser, {
    userId: fixture.userId('admin-history'),
    orgId,
    roles: ['admin'],
    name: 'History Admin',
  })

  const household = await createHousehold(adminContext, orgId, fixture.text('Household History'))
  const swimmer = await createSwimmer(adminContext, orgId, household.id, {
    firstName: 'Casey',
    lastName: 'History',
    dateOfBirth: '2011-08-09',
  })
  await createPerformanceResult(adminContext, orgId, swimmer.id, {
    eventDistance: 100,
    stroke: 'free',
    course: 'SCY',
    gender: 'F',
    finTimeMs: 61_250,
  })

  const parentContext = await newAuthedContext(browser, {
    userId: fixture.userId('parent-history'),
    orgId,
    householdId: household.id,
    roles: ['parent'],
    name: 'History Parent',
  })

  const parentPage = await parentContext.newPage()
  await parentPage.goto(`/portal/household/swimmers/${swimmer.id}`)
  await expectLoaded(parentPage, /Casey History/)
  await parentPage.getByRole('button', { name: 'History' }).click()

  await expect(parentPage.getByText('100 SCY Freestyle')).toBeVisible()
  await expect(parentPage.getByText('Finished in 1:01.25.')).toBeVisible()

  await parentPage.reload()
  await expectLoaded(parentPage, /Casey History/)
  await parentPage.getByRole('button', { name: 'History' }).click()
  await expect(parentPage.getByText('100 SCY Freestyle')).toBeVisible()
  await expect(parentPage.getByText('Finished in 1:01.25.')).toBeVisible()

  await parentContext.close()
  await adminContext.close()
})
