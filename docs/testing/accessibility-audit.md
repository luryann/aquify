# WCAG 2.2 AA Accessibility Audit — DARE Platform

**Version:** 1.0
**Last Updated:** 2026-03-23
**Standard:** WCAG 2.2 Level AA
**Owner:** Product (Accessibility)

---

## Scope

This audit covers the critical user flows in the DARE admin portal and parent portal. The swimmer portal is included for core read flows.

### Critical Flows

**Admin Portal (`admin.dare.club`)**
1. Login and MFA
2. Create meet (multi-step form)
3. View household and swimmer detail
4. Approve meet entry
5. Process refund

**Parent Portal (`/portal/`)**
1. Login
2. View commitment status and select events
3. Pay invoice (Stripe)
4. Upload document

**Swimmer Portal (`/swim/`)**
1. View schedule
2. View best times

---

## Testing Tools

| Tool | Purpose | How to Run |
|---|---|---|
| axe-core (browser extension) | Automated WCAG violation detection | Install [axe DevTools](https://www.deque.com/axe/), run on each flow |
| Keyboard-only navigation | Verify full operability without mouse | Tab through all interactive elements |
| NVDA + Chrome | Screen reader testing (Windows) | Enable NVDA, navigate with arrow keys |
| VoiceOver + Safari | Screen reader testing (macOS/iOS) | Cmd+F5, navigate with VO keys |
| Chrome DevTools Lighthouse | Accessibility score and audit | DevTools → Lighthouse → Accessibility |
| Color contrast analyzer | Verify 4.5:1 minimum ratio | [Colour Contrast Analyser](https://www.tpgi.com/color-contrast-checker/) |

---

## Audit Checklist

Status values: **Pass** | **Fail** | **Partial** | **N/A** | **Not Tested**

---

### Perceivable

#### 1.1 Text Alternatives

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 1.1.1 Non-text Content | All images, icons, and graphics have descriptive `alt` text or `aria-label` | Not Tested | Not Tested | Not Tested | Audit icon buttons (sidebar, table actions) for `aria-label` |

#### 1.2 Time-based Media

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 1.2.1 Audio-only / Video-only | Transcript or alternative provided | N/A | N/A | N/A | No audio/video content in v1 |
| 1.2.2 Captions | Live/recorded video has captions | N/A | N/A | N/A | |

#### 1.3 Adaptable

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 1.3.1 Info and Relationships | Structure conveyed through semantic HTML (headings, lists, tables) | Not Tested | Not Tested | Not Tested | Verify data tables use `<th>` with scope; forms use `<label>` |
| 1.3.2 Meaningful Sequence | DOM reading order matches visual order | Not Tested | Not Tested | Not Tested | Check slide-over panels and modal stacking order |
| 1.3.3 Sensory Characteristics | Instructions don't rely solely on shape, color, or location | Not Tested | Not Tested | Not Tested | Check status badges use text not only color |
| 1.3.4 Orientation | Content not restricted to one orientation | Not Tested | Not Tested | Not Tested | Test portrait + landscape on iPad |
| 1.3.5 Identify Input Purpose | Form inputs have `autocomplete` attributes where applicable | Not Tested | Not Tested | N/A | Login form: `autocomplete="email"`, `autocomplete="current-password"` |

#### 1.4 Distinguishable

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 1.4.1 Use of Color | Color not the sole means of conveying information | Not Tested | Not Tested | Not Tested | Status badges (committed/not) must include text label |
| 1.4.2 Audio Control | No auto-playing audio | N/A | N/A | N/A | |
| 1.4.3 Contrast (Minimum) | Text contrast ratio ≥ 4.5:1 (3:1 for large text) | Not Tested | Not Tested | Not Tested | Brand accent `#eb5d1e` on white needs verification |
| 1.4.4 Resize Text | Text resizable to 200% without loss of content | Not Tested | Not Tested | Not Tested | Test at browser 200% zoom |
| 1.4.5 Images of Text | Avoid images of text where possible | Not Tested | N/A | N/A | Check logo/wordmark |
| 1.4.10 Reflow | Content reflows at 320px width without horizontal scroll | Not Tested | Not Tested | Not Tested | Critical for mobile parents |
| 1.4.11 Non-text Contrast | UI components and focus indicators have 3:1 contrast | Not Tested | Not Tested | Not Tested | Check input borders, checkbox, toggle |
| 1.4.12 Text Spacing | No loss of content when letter/word/line spacing is adjusted | Not Tested | Not Tested | Not Tested | |
| 1.4.13 Content on Hover/Focus | Tooltips dismissible, hoverable, persistent | Not Tested | Not Tested | N/A | Check tooltip components |

---

### Operable

#### 2.1 Keyboard Accessible

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 2.1.1 Keyboard | All functionality operable via keyboard | Not Tested | Not Tested | Not Tested | Priority: sidebar nav, data tables, slide-over panels |
| 2.1.2 No Keyboard Trap | Focus not trapped in a component | Not Tested | Not Tested | Not Tested | Check modal/drawer components (shadcn Dialog) |
| 2.1.3 Keyboard (No Exception) | No exceptions for keyboard operation | Not Tested | Not Tested | Not Tested | |
| 2.1.4 Character Key Shortcuts | Single-character shortcuts can be disabled or remapped | N/A | N/A | N/A | No single-key shortcuts in v1 |

#### 2.2 Enough Time

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 2.2.1 Timing Adjustable | Session timeouts can be extended or disabled | Not Tested | Not Tested | N/A | Session expiry toast should offer "Stay logged in" action |
| 2.2.2 Pause, Stop, Hide | Moving content (loading spinners, skeleton loaders) can be paused | N/A | N/A | N/A | Skeletons are static — acceptable |

#### 2.3 Seizures and Physical Reactions

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 2.3.1 Three Flashes | No content flashes more than 3 times per second | Not Tested | Not Tested | Not Tested | Check page load animations |

#### 2.4 Navigable

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 2.4.1 Bypass Blocks | Skip link to main content provided | Not Tested | Not Tested | Not Tested | Add `<a href="#main-content">Skip to main content</a>` to layout |
| 2.4.2 Page Titled | Each page has a descriptive `<title>` | Not Tested | Not Tested | Not Tested | Verify Next.js `metadata.title` on all routes |
| 2.4.3 Focus Order | Focus order is logical | Not Tested | Not Tested | Not Tested | Tab through login → dashboard → detail flows |
| 2.4.4 Link Purpose | Link text describes the destination | Not Tested | Not Tested | Not Tested | Check "View" and "Edit" links — add `aria-label` if ambiguous |
| 2.4.5 Multiple Ways | Multiple ways to find a page | Not Tested | N/A | N/A | Admin: nav + search; parent: nav |
| 2.4.6 Headings and Labels | Headings and labels describe topic/purpose | Not Tested | Not Tested | Not Tested | |
| 2.4.7 Focus Visible | Keyboard focus indicator visible | Not Tested | Not Tested | Not Tested | Check CSS focus-visible ring on all interactive elements |
| 2.4.11 Focus Not Obscured (Min) | Focused component not fully hidden | Not Tested | Not Tested | Not Tested | Check sticky header / footer overlap |
| 2.4.12 Focus Not Obscured (Enh) | Focused component fully visible | Not Tested | Not Tested | Not Tested | |

#### 2.5 Input Modalities

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 2.5.1 Pointer Gestures | Multipoint gestures have single-pointer alternative | N/A | N/A | N/A | |
| 2.5.2 Pointer Cancellation | Click actions fire on mouseup not mousedown | Not Tested | Not Tested | Not Tested | |
| 2.5.3 Label in Name | Visible label text contained in accessible name | Not Tested | Not Tested | Not Tested | Icon-only buttons must have `aria-label` matching visible tooltip |
| 2.5.4 Motion Actuation | Functionality not only operable by device motion | N/A | N/A | N/A | |
| 2.5.7 Dragging Movements | Drag actions have single-pointer alternative | Not Tested | N/A | N/A | Sidebar resize: verify keyboard width adjustment |
| 2.5.8 Target Size (Minimum) | Touch targets ≥ 24×24 CSS pixels | Not Tested | Not Tested | Not Tested | Critical for parent portal on mobile |

---

### Understandable

#### 3.1 Readable

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 3.1.1 Language of Page | `<html lang="en">` present | Not Tested | Not Tested | Not Tested | Check Next.js layout |
| 3.1.2 Language of Parts | Language changes marked with `lang` attribute | N/A | N/A | N/A | English-only v1 |

#### 3.2 Predictable

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 3.2.1 On Focus | Focus does not trigger unexpected context change | Not Tested | Not Tested | Not Tested | |
| 3.2.2 On Input | Input does not trigger unexpected context change | Not Tested | Not Tested | Not Tested | Check select/combobox components |
| 3.2.3 Consistent Navigation | Navigation is consistent across pages | Not Tested | Not Tested | Not Tested | Sidebar nav order must not change per route |
| 3.2.4 Consistent Identification | Components with same function have same label | Not Tested | Not Tested | Not Tested | |
| 3.2.6 Consistent Help | Help links/support contact in consistent location | Not Tested | Not Tested | N/A | |

#### 3.3 Input Assistance

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 3.3.1 Error Identification | Errors identified in text, not just color | Not Tested | Not Tested | N/A | Form validation messages must be descriptive |
| 3.3.2 Labels or Instructions | Labels and instructions provided for inputs | Not Tested | Not Tested | N/A | Check Zod/React Hook Form error messages |
| 3.3.3 Error Suggestion | Error messages suggest how to correct | Not Tested | Not Tested | N/A | |
| 3.3.4 Error Prevention | Reversible, checked, or confirmed for legal/financial actions | Not Tested | Not Tested | N/A | Refund confirmation dialog; payment confirmation |
| 3.3.7 Redundant Entry | Previously entered info not re-required in same session | Not Tested | Not Tested | N/A | |
| 3.3.8 Accessible Authentication | No cognitive function test required to authenticate | Not Tested | Not Tested | N/A | MFA must support authenticator app (not just text puzzle) |

---

### Robust

#### 4.1 Compatible

| Criterion | Requirement | Admin | Parent | Swimmer | Notes |
|---|---|---|---|---|---|
| 4.1.1 Parsing | No duplicate IDs; well-formed HTML | Not Tested | Not Tested | Not Tested | Run HTML validator on rendered output |
| 4.1.2 Name, Role, Value | All UI components have accessible name, role, value | Not Tested | Not Tested | Not Tested | Priority: custom comboboxes, data tables, toggle switches |
| 4.1.3 Status Messages | Status messages programmatically determinable without focus | Not Tested | Not Tested | Not Tested | Toast/snackbar notifications need `role="status"` or `aria-live` |

---

## Known Issues (Pre-Launch)

The following are common accessibility gaps in Next.js + shadcn/ui applications that should be addressed before launch:

1. **Icon-only buttons** — shadcn `Button` with only an icon child renders no accessible name. Add `aria-label` to all icon-only buttons (edit pencil, delete trash, close X, sidebar toggle).

2. **Data table sort headers** — sort column headers need `aria-sort="ascending|descending|none"` to convey sort state to screen readers.

3. **Slide-over panels / Drawers** — confirm shadcn `Sheet` component traps focus correctly and restores focus to the trigger on close.

4. **Toast notifications** — ensure `Sonner` or the notification system uses `role="status"` for informational toasts and `role="alert"` for error toasts so screen readers announce them.

5. **Color contrast — brand accent** — the brand accent `#eb5d1e` on white background is approximately 3.0:1, which fails AA for normal-sized body text (requires 4.5:1). Acceptable for large text (≥ 18pt) and decorative elements only. Do not use as text color on white backgrounds.

6. **Form validation** — React Hook Form + Zod error messages should be associated with their fields via `aria-describedby` pointing to the error element ID.

7. **Loading skeletons** — skeleton placeholders should have `aria-busy="true"` on the container so screen readers announce the loading state.

8. **Skip link** — `admin/app/layout.tsx` and portal layout do not currently include a skip-to-main-content link. Add as first focusable element in each layout.

9. **Page titles** — verify every Next.js route has a unique, descriptive `metadata.title`. Generic titles ("DARE" or "Dashboard") do not distinguish pages for screen reader users.

10. **Sidebar resize drag handle** — new drag handle in `Sidebar.tsx` needs a keyboard alternative (e.g., arrow keys when focused) and `aria-label="Resize sidebar"`.

---

## Remediation Priority

| Priority | Issues | Target Completion |
|---|---|---|
| P0 (launch blocking) | Icon-only buttons without `aria-label`; missing skip link; form error association | Before launch |
| P1 (high) | Toast `aria-live` regions; focus trap in drawers; page titles | Before launch |
| P2 (medium) | Color contrast for accent text; sort header `aria-sort`; loading state `aria-busy` | First sprint post-launch |
| P3 (low) | Keyboard drag handle; redundant entry; motion preference `prefers-reduced-motion` | Roadmap |
