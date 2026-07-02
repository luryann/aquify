# STEP35H Autopay Implementation Plan

## Goal

Replace the remaining demo-only autopay flows with a real shared backend-backed implementation for both the admin autopay page and the parent portal billing autopay tab.

## Scope

- Add billing-domain persistence for household autopay enrollment state and payment-method metadata.
- Expose API routes for:
  - listing household autopay statuses for admins/billing staff
  - reading a single household autopay profile
  - enrolling/updating autopay for a household
  - opting out of autopay for a household
- Compute the next charge preview from real open invoices rather than hardcoded UI values.
- Wire `admin/app/(admin)/billing/autopay/page.tsx` and `admin/app/(portal)/portal/billing/page.tsx` to the live API.
- Add audit coverage for autopay mutations and regression tests for parent/admin scope rules.

## Notes

- Parent access must remain restricted to the authenticated `household_id`.
- Admin/billing access can manage org households but should still reuse the same billing service source of truth.
- Payment method storage in this slice is metadata-only; no raw card data should be stored.
