# STEP 32G E2E Harness Plan

## Goal
Add a real browser-driven regression suite in-repo for critical public, parent, admin, and swimmer flows using the live local backend and frontend.

## Scope for first pass
- Public self-registration request submission.
- Admin approvals review and approve/reject actions.
- Parent portal live registration wizard with multi-swimmer submission.
- Swimmer profile bio persistence through the live API.

## Environment strategy
- Start local Postgres via `docker compose up -d postgres`.
- Apply real schema migrations against the local `DATABASE_URL`.
- Start the API in `APP_ENV=development` so the dev session-issue route is available for browser bootstrap.
- Start the Next.js app with `NEXT_PUBLIC_SWIMMER_PORTAL_ENABLED=true`.
- Use Playwright browser contexts plus `/api/v1/auth/sessions/issue` to establish real cookie-backed sessions.

## Test design
- Seed test data through real HTTP APIs, not direct file or in-browser mocks.
- Keep each flow isolated by using unique `org_id` values per test.
- Use live browser navigation for user-facing assertions and API follow-up checks for backend state verification.

## Initial deliverables
- `admin/playwright.config.ts`
- `admin/e2e/*.spec.ts`
- `admin/e2e/support/*.ts`
- `admin/scripts/e2e/*.mjs`
- `admin/package.json` E2E scripts
- CI workflow hook for browser regression execution
