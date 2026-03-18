# STEP 5 Post-Implementation Audit Failure

Date: 2026-03-15  
Auditor: Codex session  
Status: **FAILED** (conditions not fully met)

## 1) What Went Wrong

At least one high-impact issue was found in STEP 5 implementation:

1. **Cross-org role assignment scope gap (security issue)**  
   In `POST /api/v1/auth/roles/assign`, the handler validates `body.org_id == X-Org-Id`, but it does **not** enforce `principal.OrgID == X-Org-Id` (or `principal.OrgID == body.org_id`).  
   Impact: a caller with a valid privileged token from one org can submit a different org in both header/body and assign roles outside their tenant boundary.

Other audit findings:

2. **Deny-by-default is only partially covered by tests**  
   Tests cover unauthorized and some forbidden cases, but do not verify successful-path invariants and principal/org consistency on protected role-assignment flow.

3. **Condition quality threshold not met**  
   Because of item (1), the implementation cannot be considered free of bugs/potential issues, free of clear security issues, or fully compliant with resource-scoping intent in planning docs.

## 2) How To Fix It

1. Enforce principal org scope in protected auth endpoints:
   - In role assignment handler, reject when `principal.OrgID` does not match `X-Org-Id`.
   - Keep existing `body.org_id == X-Org-Id` check.
   - Net requirement: `principal.OrgID == X-Org-Id == body.org_id`.

2. Add explicit tests:
   - Reject role assignment when principal org differs from `X-Org-Id` (`403`, code e.g. `scope_violation`).
   - Allow role assignment only when all three org values align and caller has required role + MFA.

3. Add regression guardrails:
   - Add one table-driven authz test suite for scope/authn/authz permutations.
   - Keep deny-by-default tests, plus a positive-path test for authorized request behavior.

## 3) What Counts As Passing

STEP 5 passes only when all are true:

1. **No known bugs/edge-case failures** in implemented STEP 5 endpoints, including org-scope invariants.
2. **No code-quality regressions**, demonstrated by passing tests and clear endpoint validation paths.
3. **No clear security issues**, especially cross-tenant authorization bypasses.
4. **Directions fully met**, including practical deny-by-default behavior and enforced resource scope for privileged actions.

Minimum objective checks:

1. `go test ./...` passes in `server/`.
2. New tests prove cross-org assignment is blocked.
3. Successful assignment requires: privileged role, MFA, valid org match, and valid payload.

## 4) Master List of Files Edited by Codex in STEP 5

The following list is based on STEP 5 implementation notes and file modification evidence in this workspace:

1. `server/internal/app/authn/service.go`
2. `server/internal/app/handlers/auth.go`
3. `server/internal/app/handlers/authz_ping.go`
4. `server/internal/app/middleware/auth.go`
5. `server/internal/app/middleware/permissions.go`
6. `server/internal/app/server.go`
7. `server/internal/app/server_authz_test.go`
8. `db/migrations/20260315000800_seed_default_roles.sql`
9. `docs/security/api-key-secret-storage-strategy.md`
10. `docs/planning/STEP5-implementation-notes.md`
11. `TODO.md`
12. `CLAUDE.md`

Note: this workspace is not a git repository (`.git` missing), so the edited-file list was reconstructed from timestamps and STEP 5 notes rather than git history.
