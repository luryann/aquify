# STEP 22 Post-Implementation Audit Failure

Date: 2026-03-18
Step: 22
Status: Failed audit

## 1) What went wrong

### A. Public session minting is still exposed

`/api/v1/auth/sessions/issue` is still registered as a public route in [server/internal/app/handlers/auth.go](/Users/ryan/Documents/aquify/server/internal/app/handlers/auth.go#L40). The handler in [server/internal/app/handlers/auth.go](/Users/ryan/Documents/aquify/server/internal/app/handlers/auth.go#L104) accepts caller-supplied `user_id`, `org_id`, `roles`, and `mfa_authenticated` values and immediately issues a session plus Secure cookies.

That means an unauthenticated caller can mint an arbitrary admin or owner session. This is a clear security issue, and because Step 22 touched the same auth surface, condition 3 is not met.

### B. Malware-scan “required” mode still accepts the built-in stub scanner

The import service still defaults to `stubMalwareScanner{}` in [server/internal/app/imports/service.go](/Users/ryan/Documents/aquify/server/internal/app/imports/service.go#L168), and that stub always returns a clean result with provider `"stub"` in [server/internal/app/imports/service.go](/Users/ryan/Documents/aquify/server/internal/app/imports/service.go#L618).

When `SECURITY_UPLOAD_REQUIRE_MALWARE_SCAN_HOOK=true`, finalize only checks that the provider string is non-empty in [server/internal/app/imports/service.go](/Users/ryan/Documents/aquify/server/internal/app/imports/service.go#L315). Because the stub provider is non-empty, “required” mode still passes without a real malware integration. That means the Step 22 checkbox is only barely satisfied, not well-met.

### C. Full verification could not be completed in this environment

Targeted tests passed, but `go test ./...` in `server/` failed because `server/internal/app/authn` tests use `httptest.NewServer`, and this sandbox blocks local port binding. The failure observed was:

- `httptest: failed to listen on a port: listen tcp6 [::1]:0: bind: operation not permitted`

Because the full suite could not run here, condition 1 cannot be confidently certified as well-met.

## 2) How to fix it

### Fix A. Lock down session issuance

- Remove `/sessions/issue` from `RegisterPublicRoutes`, or gate it behind an explicit dev-only/internal-only config flag.
- Reject client-supplied role and MFA state in any non-development path.
- Add tests proving unauthenticated callers cannot issue sessions outside approved local-dev mode.

### Fix B. Make malware-scan enforcement real

- Do not default to a clean stub scanner when malware scanning is marked required.
- Fail startup or fail finalize if no non-stub scanner is configured while `SECURITY_UPLOAD_REQUIRE_MALWARE_SCAN_HOOK=true`.
- Treat `"stub"` as non-compliant in required mode.
- Add tests for:
  - required mode with no scanner configured
  - required mode with stub scanner
  - infected result
  - clean result from a real scanner implementation

### Fix C. Re-run the full verification pass

- Re-run `go test ./...` in an environment that allows localhost listeners.
- Keep the targeted security tests, but also confirm the full server suite is green before marking Step 22 complete.

## 3) What counts as passing

Step 22 should count as passing only if all of the following are true:

- No unauthenticated route can mint arbitrary sessions or elevate caller role/MFA state.
- `SECURITY_UPLOAD_REQUIRE_MALWARE_SCAN_HOOK=true` cannot succeed with a stub or missing malware scanner.
- The full relevant automated test suite runs successfully, not just a targeted subset.
- The security hardening claims in [TODO.md](/Users/ryan/Documents/aquify/TODO.md) and [CLAUDE.md](/Users/ryan/Documents/aquify/CLAUDE.md) are supported by implementation, not placeholders.

## 4) Master list of files edited in STEP 22

- [/.github/workflows/ci.yml](/Users/ryan/Documents/aquify/.github/workflows/ci.yml)
- [/CLAUDE.md](/Users/ryan/Documents/aquify/CLAUDE.md)
- [/TODO.md](/Users/ryan/Documents/aquify/TODO.md)
- [/docs/api/openapi.yaml](/Users/ryan/Documents/aquify/docs/api/openapi.yaml)
- [/docs/planning/STEP22-security-hardening-plan.md](/Users/ryan/Documents/aquify/docs/planning/STEP22-security-hardening-plan.md)
- [/docs/security/threat-model-v1.md](/Users/ryan/Documents/aquify/docs/security/threat-model-v1.md)
- [/server/internal/app/config.go](/Users/ryan/Documents/aquify/server/internal/app/config.go)
- [/server/internal/app/handlers/auth.go](/Users/ryan/Documents/aquify/server/internal/app/handlers/auth.go)
- [/server/internal/app/handlers/billing.go](/Users/ryan/Documents/aquify/server/internal/app/handlers/billing.go)
- [/server/internal/app/handlers/imports.go](/Users/ryan/Documents/aquify/server/internal/app/handlers/imports.go)
- [/server/internal/app/imports/service.go](/Users/ryan/Documents/aquify/server/internal/app/imports/service.go)
- [/server/internal/app/middleware/auth.go](/Users/ryan/Documents/aquify/server/internal/app/middleware/auth.go)
- [/server/internal/app/middleware/auth_test.go](/Users/ryan/Documents/aquify/server/internal/app/middleware/auth_test.go)
- [/server/internal/app/middleware/csrf.go](/Users/ryan/Documents/aquify/server/internal/app/middleware/csrf.go)
- [/server/internal/app/middleware/impersonation.go](/Users/ryan/Documents/aquify/server/internal/app/middleware/impersonation.go)
- [/server/internal/app/middleware/permissions.go](/Users/ryan/Documents/aquify/server/internal/app/middleware/permissions.go)
- [/server/internal/app/middleware/rate_limit.go](/Users/ryan/Documents/aquify/server/internal/app/middleware/rate_limit.go)
- [/server/internal/app/middleware/security_headers.go](/Users/ryan/Documents/aquify/server/internal/app/middleware/security_headers.go)
- [/server/internal/app/middleware/session_cookies.go](/Users/ryan/Documents/aquify/server/internal/app/middleware/session_cookies.go)
- [/server/internal/app/server.go](/Users/ryan/Documents/aquify/server/internal/app/server.go)
- [/server/internal/app/server_billing_test.go](/Users/ryan/Documents/aquify/server/internal/app/server_billing_test.go)
- [/server/internal/app/server_imports_test.go](/Users/ryan/Documents/aquify/server/internal/app/server_imports_test.go)
- [/server/internal/app/server_security_test.go](/Users/ryan/Documents/aquify/server/internal/app/server_security_test.go)
