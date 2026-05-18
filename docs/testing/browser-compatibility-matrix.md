# Browser and Device Compatibility Matrix — DARE Platform

**Version:** 1.0
**Last Updated:** 2026-03-23
**Owner:** QA / Engineering

---

## Support Tiers

### Tier 1 — Fully Supported (Launch Blocking)

These browsers must work correctly for all critical flows. Failures are launch-blocking bugs.

| Browser | Minimum Version | Platforms |
|---|---|---|
| Chrome | 120+ | Windows, macOS, Android |
| Safari | 17+ | macOS, iOS |
| Firefox | 120+ | Windows, macOS |
| Edge (Chromium) | 120+ | Windows |
| Safari on iOS | 16+ (iOS 16+) | iPhone, iPad |
| Chrome on Android | 120+ | Android 12+ |

### Tier 2 — Best Effort (Not Launch Blocking)

Test and document issues, but failures do not block launch.

| Browser | Minimum Version | Platforms |
|---|---|---|
| Samsung Internet | 23+ | Android |
| Firefox on Android | 120+ | Android |

### Not Supported

| Browser / Environment | Reason |
|---|---|
| Internet Explorer 11 | EOL; `<1%` market share; no ES module support |
| Opera Mini | Extreme proxy compression breaks API calls |
| Chrome < 100 | Missing CSS features (cascade layers, `has()`) |
| iOS < 15 | Missing `structuredClone`, CSS subgrid |

---

## Feature × Browser Matrix

Status: **Pass** | **Fail** | **Partial** | **Not Tested**

### Admin Portal

| Feature | Chrome 120+ | Firefox 120+ | Edge 120+ | Safari 17+ | Notes |
|---|---|---|---|---|---|
| Login / SSO redirect | Not Tested | Not Tested | Not Tested | Not Tested | Keycloak OIDC redirect |
| Dev stub login | Not Tested | Not Tested | Not Tested | Not Tested | localStorage-based |
| Dashboard (live data) | Not Tested | Not Tested | Not Tested | Not Tested | |
| Sidebar resize (drag) | Not Tested | Not Tested | Not Tested | Not Tested | `pointer events`, `cursor` CSS |
| Create meet (multi-step) | Not Tested | Not Tested | Not Tested | Not Tested | File drag-and-drop |
| File upload (.cl2/.sd3/.hy3) | Not Tested | Not Tested | Not Tested | Not Tested | `FileReader`, `FormData` |
| Meet commitments table | Not Tested | Not Tested | Not Tested | Not Tested | |
| Slide-over panel | Not Tested | Not Tested | Not Tested | Not Tested | shadcn Sheet |
| Invoice / billing table | Not Tested | Not Tested | Not Tested | Not Tested | |
| Process refund (modal) | Not Tested | Not Tested | Not Tested | Not Tested | |
| Email campaign editor | Not Tested | Not Tested | Not Tested | Not Tested | |
| CSV report download | Not Tested | Not Tested | Not Tested | Not Tested | `Content-Disposition` header |
| Audit log (long list) | Not Tested | Not Tested | Not Tested | Not Tested | Virtual scroll / pagination |
| Training page (charts) | Not Tested | Not Tested | Not Tested | Not Tested | CSS bar charts |

### Parent Portal

| Feature | Chrome 120+ | Firefox 120+ | Edge 120+ | Safari 17+ (macOS) | Safari iOS 16+ | Chrome Android | Notes |
|---|---|---|---|---|---|---|---|
| Portal login | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | |
| Dashboard (household) | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | |
| Meet commitment flow | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Multi-step |
| Event selection checkboxes | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | |
| Invoice payment (Stripe) | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Stripe.js |
| Stripe card input | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | iframe |
| Document upload | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | File picker |
| Calendar view | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Date calculations |
| Swimmer profile | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | Not Tested | |

### Swimmer Portal

| Feature | Chrome 120+ | Safari 17+ | Safari iOS 16+ | Chrome Android | Notes |
|---|---|---|---|---|---|
| Schedule view | Not Tested | Not Tested | Not Tested | Not Tested | |
| Best times table | Not Tested | Not Tested | Not Tested | Not Tested | |
| Meet detail | Not Tested | Not Tested | Not Tested | Not Tested | |
| Profile page | Not Tested | Not Tested | Not Tested | Not Tested | |

---

## Mobile Device Test Targets

| Device | Screen Width | OS / Browser | Priority |
|---|---|---|---|
| iPhone 14 Pro | 390 px | iOS 17 / Safari | P0 (most common parent device) |
| iPhone SE (3rd gen) | 375 px | iOS 16 / Safari | P1 |
| iPad Air (5th gen) | 820 px | iPadOS 17 / Safari | P1 |
| Samsung Galaxy S23 | 360 px | Android 13 / Chrome | P1 |
| Google Pixel 7 | 412 px | Android 13 / Chrome | P2 |
| Samsung Galaxy Tab S9 | 800 px | Android 13 / Chrome | P2 |

### Mobile-Specific Test Focus Areas

1. **Touch target size** — all interactive elements must be ≥ 44×44 px (Apple HIG) / ≥ 48×48 dp (Material). This is especially important for the commitment event selection checkboxes.
2. **Viewport meta tag** — `<meta name="viewport" content="width=device-width, initial-scale=1">` must be present.
3. **Sticky header scroll behavior** — verify fixed/sticky headers do not cover content when scrolling.
4. **Form input zoom** — iOS Safari zooms in on `<input>` with `font-size < 16px`. Ensure all inputs use at least 16px.
5. **File picker on iOS** — `.cl2`, `.sd3`, `.hy3` extensions may not appear in the iOS file picker by default. Test file upload on iPhone and verify custom extensions are selectable.
6. **Stripe payment form on iOS** — test Apple Pay and standard card entry in the Stripe payment modal on Safari iOS.
7. **Keyboard avoidance** — on mobile, the soft keyboard should push content up rather than obscure it. Verify in commitment forms and payment forms.

---

## Testing Procedure

### Manual Test Checklist (per browser per release)

For each Tier 1 browser:

1. Open the browser in a private/incognito window with extensions disabled.
2. Navigate to the staging URL.
3. Complete the admin critical flows:
   - [ ] Log in with stub or test credentials
   - [ ] Navigate all sidebar sections without error
   - [ ] Create a test meet with sessions
   - [ ] View a household and swimmer detail
   - [ ] View the commitments table and open a slide-over
   - [ ] View billing invoices
4. Complete the parent portal critical flows:
   - [ ] Log in as parent
   - [ ] View dashboard with swimmer cards
   - [ ] Navigate to a meet and view commitment status
   - [ ] Navigate to billing
5. Note any visual regressions, console errors, or functional failures.
6. Run axe DevTools scan on the login page, dashboard, and one detail page.

### Automated axe Scans

Integrate axe-core into the Playwright e2e test suite:

```typescript
import { checkA11y } from 'axe-playwright';

test('login page is accessible', async ({ page }) => {
  await page.goto('/login');
  await checkA11y(page, undefined, {
    runOnly: ['wcag2a', 'wcag2aa', 'wcag21aa'],
  });
});
```

Run with: `npm run test:e2e:a11y` in `admin/`.

---

## Results Tracking

Update the tables below after each test cycle. Format: `Pass` / `Fail (Issue #)` / `Not Tested`.

### Last Test Cycle: Not yet run

| Browser | Version Tested | Admin Portal | Parent Portal | Swimmer Portal | Tester | Date |
|---|---|---|---|---|---|---|
| Chrome | — | Not Tested | Not Tested | Not Tested | — | — |
| Firefox | — | Not Tested | Not Tested | Not Tested | — | — |
| Edge | — | Not Tested | Not Tested | Not Tested | — | — |
| Safari macOS | — | Not Tested | Not Tested | Not Tested | — | — |
| Safari iOS | — | Not Tested | Not Tested | Not Tested | — | — |
| Chrome Android | — | Not Tested | Not Tested | Not Tested | — | — |
| Samsung Internet | — | Not Tested | N/A | Not Tested | — | — |
| Firefox Android | — | Not Tested | N/A | Not Tested | — | — |

---

## Known Issues and Workarounds

| Issue | Affected Browser | Severity | Status | Workaround |
|---|---|---|---|---|
| iOS file picker may not show `.sd3`/`.cl2` extensions | Safari iOS | Medium | Open | Users can select "All Files" in the iOS document picker |
| Stripe.js iframe may have slight render delay on first load | All | Low | Acceptable | Stripe handles this internally; no action required |
| CSS `subgrid` used in training page layout | Firefox < 71 | Low | N/A | Minimum Firefox version (120) supports subgrid |
| `prefers-reduced-motion` not yet respected in load animations | All | Low | Open | Add `@media (prefers-reduced-motion: reduce)` to globals.css |
| Focus ring may be obscured by sticky sidebar on small viewports | Safari iOS | Medium | Open | Add `scroll-margin` to interactive elements |
