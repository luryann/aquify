# Step 30F: Self-Service Flows — Closeout Notes

## Goal
Verify and close out the self-service registration and approval workflow for Keycloak-backed user onboarding.

## Verified Implementation
- Public registration page exists at `admin/app/register/page.tsx` and submits to `POST /api/v1/register`.
- Backend self-registration endpoint exists in `server/internal/app/handlers/users.go` and creates a Keycloak user with `registration_status=pending_approval`.
- Admin approvals queue exists at `admin/app/(admin)/members/approvals/page.tsx`.
- Approve and reject endpoints exist at `POST /api/v1/users/{id}/approve` and `POST /api/v1/users/{id}/reject`.
- Audit actions are defined in `server/internal/app/audit/service.go` and emitted for self-register, approve, and reject flows.
- In-app admin notification is implemented as a live pending-approval badge in the admin sidebar (`admin/components/layout/Sidebar.tsx`).

## Verification Performed
- `cd server && go test ./internal/app -run 'Step30F|SelfRegister|ApproveUser|RejectUser|PendingApproval'`
- `cd admin && npm run lint`

## Notes
- The repo does not currently include an `ABOUT.txt`; project context was taken from `CLAUDE.md` and the step-specific tests.
- `STEP 30F` is complete based on the current code, tests, and admin UI wiring.
