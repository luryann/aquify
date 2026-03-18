# STEP 20A — Post-Implementation Audit Failure

**Audit Date:** 2026-03-17
**Step:** STEP 20A — Admin UI Screens (Imports, Exports, Invoices)

---

## Conditions Evaluated

| # | Condition | Result |
|---|-----------|--------|
| 1 | No bugs, potential issues, or edge cases | ❌ FAIL |
| 2 | No regressions in code quality | ✅ PASS |
| 3 | No clear code security issues | ✅ PASS |
| 4 | All directions followed and completed fully | ❌ FAIL |

---

## Condition 1 — Bugs and Issues Found

### Bug 1 (Critical): "Total Issued" stat in invoices/page.tsx is miscalculated

**File:** `admin/app/(admin)/invoices/page.tsx`, line 326

**Code:**
```tsx
const totalAmount    = MOCK_INVOICES.filter((i) => ['issued','partially_paid'].includes(i.status)).reduce((s, i) => s + i.totalCents, 0)
const outstandingAmt = MOCK_INVOICES.filter((i) => ['issued','partially_paid'].includes(i.status)).reduce((s, i) => s + (i.totalCents - i.paidCents), 0)

{ label: 'Total Issued',  value: formatCents(totalAmount + outstandingAmt), ... }
```

**What went wrong:** `totalAmount` is the sum of `totalCents` for issued/partially_paid invoices. `outstandingAmt` is the sum of `(totalCents - paidCents)` for the **same** set of invoices. Adding them together double-counts: for an invoice with `totalCents=120000` and `paidCents=0`, `totalAmount` includes 120000 and `outstandingAmt` also includes 120000, yielding 240000 for a single invoice — clearly wrong.

With the mock data:
- `totalAmount` = 120000 + 72000 = **192,000**
- `outstandingAmt` = 120000 + 36000 = **156,000**
- "Total Issued" = 192000 + 156000 = **$3,480.00** — incorrect

**How to fix:**
```tsx
// "Total Issued" should simply be the sum of totalCents for all non-draft non-void invoices:
const totalIssued = MOCK_INVOICES
  .filter((i) => !['draft','voided','written_off'].includes(i.status))
  .reduce((s, i) => s + i.totalCents, 0)

{ label: 'Total Issued', value: formatCents(totalIssued), ... }
```

**What counts as passing:** The "Total Issued" stat must show the total face value of all invoices that have been issued (i.e., not draft/voided/written-off), without any double-counting.

---

### Bug 2 (Lint Error): Unused variable `importId` in imports/[id]/page.tsx

**File:** `admin/app/(admin)/imports/[id]/page.tsx`, line 385

```tsx
const { id: importId } = useParams() as { id: string }
```

`importId` is declared but never referenced in the component body (the mock data is static). The ESLint config extends `next/typescript`, which includes `@typescript-eslint/no-unused-vars` as an error. This will cause `make lint` to fail.

**How to fix:** Either use `importId` in the component (e.g., to fetch data) or rename it to `_importId`, or destructure with just `id`:
```tsx
const params = useParams()
// use params.id wherever needed
```
Or simply:
```tsx
const { id: _importId } = useParams() as { id: string }
```
Once real API integration is wired in, `importId` should be the fetch key.

---

### Bug 3 (Lint Error): Unused variable `invoiceId` in invoices/[id]/page.tsx

**File:** `admin/app/(admin)/invoices/[id]/page.tsx`, line 318

```tsx
const { id: invoiceId } = useParams() as { id: string }
```

Same issue as above — `invoiceId` is declared but never used. Will fail `@typescript-eslint/no-unused-vars`.

**How to fix:** Same pattern as Bug 2.

---

### Bug 4 (Lint Error): Unused function `formatDateTime` in exports/page.tsx

**File:** `admin/app/(admin)/exports/page.tsx`, lines 148–150

```tsx
function formatDateTime(iso: string) {
  return new Date(iso).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}
```

This function is defined but never called in the file — only `formatDate` is used in the column definitions. The `next/typescript` ESLint config will flag this as a lint error.

**How to fix:** Remove the `formatDateTime` function from `exports/page.tsx` since it is not needed. If the `completedAt` column ever needs a time-aware display, it can be re-added.

---

## Condition 4 — Incomplete Scope

### Missing Screen 1: Commitments Review Screen

**From TODO.md, STEP 20A:**
```
- [ ] Build commitments review screen.
```

This screen was **not built**. The meet detail page (`/meets/[id]`) contains a Commitments tab, but there is no standalone `/commitments` admin screen where coaches/admins can see a cross-meet view of all pending commitments, filter by meet/status, and take bulk actions.

**How to fix:** Build `admin/app/(admin)/commitments/page.tsx` — a standalone screen that lists all commitments across meets, with meet filter, commitment status filter (invited/yes/no/locked), swimmer counts, and a link into the relevant meet's detail page.

---

### Missing Screen 2: Entries Approval Screen

**From TODO.md, STEP 20A:**
```
- [ ] Build entries approval screen.
```

This screen was **not built**. The meet detail page contains an Entries tab, but there is no standalone `/entries` admin screen where coaches can review all pending entry approvals across meets in a single queue view.

**How to fix:** Build `admin/app/(admin)/entries/page.tsx` — a standalone screen listing all entries across meets pending approval, with meet filter, status filter (draft/submitted/approved), and quick-approve/reject actions.

---

### Missing: TODO.md Not Updated

All STEP 20A task items remain unchecked (`[ ]`) in `TODO.md`. Even the four screens that were successfully built (imports list, import detail/conflict resolution, exports list, invoices list/detail) were not marked complete.

**How to fix:** Update `TODO.md` lines 335–341 to mark completed items with `[x]` and leave commitments/entries items as `[ ]`.

---

## What Counts as Passing

For all 4 conditions to be met:

1. **Condition 1 passes when:**
   - "Total Issued" in invoices list uses a correct, non-double-counting formula
   - No unused variables or functions that would cause `make lint` to fail
   - `make lint` (running `npm run lint` in `admin/`) exits with code 0

2. **Condition 4 passes when:**
   - Either commitments review and entries approval screens are built as standalone admin routes, OR the TODO.md is updated to explicitly defer/scope them out with a clear rationale
   - `TODO.md` STEP 20A section is updated to mark the 4 completed screens as `[x]`

---

## Master List of Files Edited/Created in STEP 20A

| File | Action |
|------|--------|
| `admin/app/(admin)/imports/page.tsx` | Created |
| `admin/app/(admin)/imports/[id]/page.tsx` | Created |
| `admin/app/(admin)/exports/page.tsx` | Created |
| `admin/app/(admin)/invoices/page.tsx` | Created |
| `admin/app/(admin)/invoices/[id]/page.tsx` | Created |
| `CLAUDE.md` | Modified (STEP 20A completion notes added) |

> **Note:** `TODO.md` was NOT updated to reflect completed items — this is itself flagged as a gap under Condition 4.
