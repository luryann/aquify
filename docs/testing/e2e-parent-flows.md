# E2E Test Stubs — Parent Flows (No Swimmer Login Required)

## Policy
Per launch policy (DECISIONS.md): **No launch-critical parent flow may require a swimmer account.**
These e2e tests verify that assertion at the UI level.

## Test Environment
- Framework: Playwright (recommended) or Cypress
- Auth stub: `AUTH_ENABLE_DEV_STUB_TOKENS=true`, token `stub-parent` → role `parent`
- Pre-seeded test data: household with 2 swimmers, no swimmer accounts created

---

## Test Suite 1: Registration Without Swimmer Login

```typescript
// e2e/parent-registration.spec.ts
test('parent can complete registration without swimmer account', async ({ page }) => {
  // 1. Authenticate as parent (no swimmer account exists)
  await page.goto('/login')
  await page.evaluate(() => localStorage.setItem('dare_session', JSON.stringify({
    token: 'stub-parent',
    user: { id: 'usr-parent-001', name: 'Sarah Chen', role: 'parent', avatarInitials: 'SC' }
  })))

  // 2. Navigate to registrations
  await page.goto('/portal/registrations')
  await expect(page.locator('h1')).toContainText('Registration')

  // 3. Assert swimmers are visible without swimmer login
  await expect(page.locator('[data-testid="swimmer-card"]').first()).toBeVisible()

  // 4. Assert no "swimmer login required" error state
  await expect(page.locator('[data-testid="swimmer-login-required"]')).not.toBeVisible()

  // 5. Assert waiver status is visible
  await expect(page.locator('[data-testid="waiver-status"]').first()).toBeVisible()

  // 6. Assert registration season is visible
  await expect(page.locator('[data-testid="registration-season"]').first()).toBeVisible()
})
```

---

## Test Suite 2: Commitment Without Swimmer Login

```typescript
// e2e/parent-commitment.spec.ts
test('parent can submit meet commitment without swimmer account', async ({ page }) => {
  // 1. Authenticate as parent
  // ... (same auth setup)

  // 2. Navigate to meets
  await page.goto('/portal/meets')
  await expect(page.locator('h1')).toContainText('Meet Center')

  // 3. Assert commitment requests visible
  await expect(page.locator('[data-testid="needs-action-section"]')).toBeVisible()

  // 4. Open meet detail
  await page.goto('/portal/meets/meet-001')

  // 5. Assert swimmer commitment tabs visible
  await expect(page.locator('[data-testid="swimmer-tab"]').first()).toBeVisible()

  // 6. Assert yes/no commitment buttons present
  await expect(page.locator('[data-testid="commit-yes"]')).toBeVisible()
  await expect(page.locator('[data-testid="commit-no"]')).toBeVisible()

  // 7. Click commit yes
  await page.click('[data-testid="commit-yes"]')

  // 8. Assert moved to step 2 (event selection)
  await expect(page.locator('[data-testid="event-selection"]')).toBeVisible()

  // 9. Assert no swimmer login required at any step
  await expect(page.locator('[data-testid="swimmer-login-required"]')).not.toBeVisible()
})
```

---

## Test Suite 3: Payment Without Swimmer Login

```typescript
// e2e/parent-payment.spec.ts
test('parent can view and initiate payment without swimmer account', async ({ page }) => {
  // 1. Authenticate as parent
  // ...

  // 2. Navigate to billing
  await page.goto('/portal/billing')
  await expect(page.locator('h1')).toContainText('Billing')

  // 3. Assert outstanding balance visible
  await expect(page.locator('[data-testid="outstanding-balance"]')).toBeVisible()

  // 4. Assert invoice list visible
  await expect(page.locator('[data-testid="invoice-row"]').first()).toBeVisible()

  // 5. Navigate to invoice detail
  await page.goto('/portal/billing/invoices/inv-001')

  // 6. Assert pay button visible
  await expect(page.locator('[data-testid="pay-button"]')).toBeVisible()

  // 7. Assert no swimmer login required
  await expect(page.locator('[data-testid="swimmer-login-required"]')).not.toBeVisible()

  // 8. Assert line items visible
  await expect(page.locator('[data-testid="line-item"]').first()).toBeVisible()
})
```

---

## Test Suite 4: Policy Acceptance Tests

```typescript
// e2e/policy-no-swimmer-account.spec.ts
test.describe('No launch-critical flow requires swimmer account', () => {
  test('parent dashboard loads with no swimmer accounts', async ({ page }) => {
    // Stub: parent authenticated, swimmers exist but have NO swimmer-role accounts
    await page.goto('/portal/dashboard')
    await expect(page.locator('h1')).toBeTruthy()
    // Swimmer cards must still appear
    await expect(page.locator('[data-testid="swimmer-card"]').first()).toBeVisible()
    // No error about missing swimmer accounts
    await expect(page.locator('text=swimmer account required')).not.toBeVisible()
  })

  test('parent can view swimmer best times without swimmer login', async ({ page }) => {
    await page.goto('/portal/household/swimmers/sw-001')
    await expect(page.locator('[data-testid="best-times-table"]')).toBeVisible()
    await expect(page.locator('text=swimmer account required')).not.toBeVisible()
  })

  test('parent can access all 6 nav sections without swimmer account', async ({ page }) => {
    const navPaths = [
      '/portal/dashboard',
      '/portal/meets',
      '/portal/billing',
      '/portal/documents',
      '/portal/communications',
      '/portal/calendar',
    ]
    for (const path of navPaths) {
      await page.goto(path)
      // Should not redirect to login or show unauthorized
      await expect(page).not.toHaveURL('/login')
      await expect(page.locator('[data-testid="unauthorized"]')).not.toBeVisible()
    }
  })
})
```

---

## Implementation Notes
- Add `data-testid` attributes to relevant elements as the UI is hardened for testing
- Run against a seeded test database with no swimmer-role user accounts
- These tests must pass in CI before any STEP 20E merge
- Add to `make test` as: `npm run test:e2e` in `admin/`
