# STEP 20D Post-Implementation Audit Failure

**Date:** 2026-03-17
**Step:** STEP 20D — Parent Portal Feature Pages
**Auditor:** Claude Code (automated)
**Verdict:** FAIL — Condition 1 not met (build-breaking duplicate function declaration)

---

## Conditions Evaluated

| # | Condition | Result |
|---|-----------|--------|
| 1 | No bugs, potential issues, or edge cases | ❌ FAIL |
| 2 | No regressions in code quality | ✅ PASS |
| 3 | No clear code security issues | ✅ PASS |
| 4 | All directions followed and completed fully | ✅ PASS |

---

## Condition 1 — Failures

### 🔴 CRITICAL: Duplicate Function Declaration in `meets/page.tsx`

**File:** `admin/app/(portal)/portal/meets/page.tsx`
**Lines:** 94 and 560

The `DeadlineCountdown` function is declared **twice** at module scope with separate bodies:

```typescript
// Line 94 — first declaration
function DeadlineCountdown({ days }: { days: number }) {
  if (days < 0) return null
  const urgent = days <= 3
  const warning = days <= 7
  const color = urgent ? '#dc2626' : warning ? '#d97706' : '#6B6866'
  const bg    = urgent ? '#fef2f2' : warning ? '#fffbeb' : 'rgba(0,0,0,0.04)'
  return (
    <span ...>
      {urgent && <Icon name="alert-circle" size={12} style={{ color }} />}
      {days === 0 ? 'Due today' : days === 1 ? '1 day left' : `${days} days left`}
    </span>
  )
}

// Line 560 — duplicate declaration (different return text, slight style difference)
function DeadlineCountdown({ days }: { days: number }) {
  if (days < 0) return null
  const urgent = days <= 3
  const warning = days <= 7
  const color = urgent ? '#dc2626' : warning ? '#d97706' : '#6B6866'
  const bg    = urgent ? '#fef2f2' : warning ? '#fffbeb' : 'rgba(0,0,0,0.04)'
  return (
    <span ...>
      {urgent && <Icon name="alert-circle" size={11} style={{ color }} />}
      {days === 0 ? 'Due today' : days === 1 ? '1 day left' : `${days} days`}
    </span>
  )
}
```

**Impact:** TypeScript will throw `error TS2393: Duplicate function implementation.` This causes `npm run build` to fail and `npm run lint` to report an error. The page is unrenderable in production.

**Root cause:** The `MeetList` helper component (defined at line 508) uses a local `DeadlineCountdown`, but instead of inlining it or renaming it, a full second declaration was placed after the already-existing top-level one. The two variants differ in icon `size` (12 vs 11) and the `days` display format (`${days} days left` vs `${days} days`).

---

### 🟡 MINOR: `published` Meets Appear in Two Sections

**File:** `admin/app/(portal)/portal/meets/page.tsx`
**Lines:** 128 and 413

```typescript
// Line 128 — activeMeets includes 'published'
const activeMeets = MEETS.filter((m) => ['commitment_open', 'published'].includes(m.status))

// Line 413 — "Upcoming" section also includes 'published'
MEETS.filter((m) => m.status === 'published' || m.status === 'draft')
```

A `published` meet (commitment not yet open) appears in both the **"Needs Action"** section and the **"Upcoming"** section simultaneously. This creates duplicate cards and confuses parents about what action (if any) is required.

Additionally, `draft` meets (not yet published to parents) are shown in the "Upcoming" section. Parents should not see draft meets at all.

---

### 🟡 MINOR: Registration Detail Links Use Wrong ID

**File:** `admin/app/(portal)/portal/registrations/page.tsx`
**Line:** 417

```typescript
<Link href={`/portal/household/swimmers/${reg.id}`} ...>
```

`reg.id` is the registration ID (e.g., `reg-001`), not the swimmer ID (e.g., `sw-001`). The swimmer profile page at `/portal/household/swimmers/[id]` expects a swimmer ID. When real API data replaces stubs, this link would route to the wrong resource. The stub will work accidentally only because both pages use hardcoded data.

---

## How to Fix

### Fix 1 — Duplicate DeadlineCountdown (CRITICAL)

Remove the second `DeadlineCountdown` declaration at line 560 (and its companion at the bottom of `meets/page.tsx`). Update the `MeetList` component to use the already-declared top-level `DeadlineCountdown`. The minor display differences (icon size 12→11, `days left` vs `days`) are negligible and the top-level version is slightly more descriptive.

**File:** `admin/app/(portal)/portal/meets/page.tsx`
**Action:** Delete lines 560–586 (the second `function DeadlineCountdown` block).

### Fix 2 — Section Overlap Logic (MINOR)

Restructure the section filters so `published` meets appear in exactly one section. Recommended approach:

- `activeMeets` = `commitment_open` only (requires action, parents can respond)
- `upcomingMeets` = `entry_finalized` only (confirmed, no action needed)
- New "Announced" section = `published` only (opens soon, informational)
- Remove `draft` from parent-visible sections entirely

### Fix 3 — Registration Link ID (MINOR)

Change `reg.id` to `reg.swimmer.id` (or a dedicated `swimmerId` field) in the link href:

```typescript
// Before (wrong)
href={`/portal/household/swimmers/${reg.id}`}

// After (correct)
href={`/portal/household/swimmers/${reg.swimmer.id}`}
```

Since stub data doesn't include `swimmer.id`, add an `id` field to the swimmer objects in `REGISTRATIONS` (e.g., `{ id: 'sw-001', name: 'Emma Chen', ... }`).

---

## What Counts as Passing

For Condition 1 to be met:
1. **No TypeScript compile errors** — `npm run build` in `admin/` must succeed without error
2. **No duplicate function declarations** — Every named function at module scope must appear exactly once
3. **No duplicate UI sections** — Each meet should appear in at most one section on the meets list page
4. **Navigation links use correct ID type** — Links from registration cards to swimmer profiles must use swimmer IDs

---

## Master File List — Files Edited in STEP 20D

The following files were created or modified during STEP 20D:

| File | Action |
|------|--------|
| `admin/app/(portal)/portal/household/swimmers/[id]/page.tsx` | Created |
| `admin/app/(portal)/portal/registrations/page.tsx` | Created |
| `admin/app/(portal)/portal/meets/page.tsx` | Created — **CONTAINS CRITICAL BUG** |
| `admin/app/(portal)/portal/meets/[id]/page.tsx` | Created |
| `admin/app/(portal)/portal/billing/page.tsx` | Created |
| `admin/app/(portal)/portal/billing/invoices/[id]/page.tsx` | Created |
| `admin/app/(portal)/portal/documents/page.tsx` | Created |
| `admin/components/ui/icons.tsx` | Modified — added `map-pin`, `edit`, `x-circle`, `check`, `credit-card` icons |
