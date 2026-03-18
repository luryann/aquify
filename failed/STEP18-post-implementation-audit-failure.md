# STEP 18 Post-Implementation Audit Failure

**Date:** 2026-03-16
**Step:** STEP 18 — API Contract and Docs
**Verdict:** FAILED — 3 of 4 conditions not well-met

---

## Condition Results

| Condition | Result |
|-----------|--------|
| 1. No bugs, potential issues, or edge cases | **FAILED** |
| 2. No regressions in code quality | **FAILED** |
| 3. No clear code security issues | **FAILED** |
| 4. All directions followed and completed fully | **FAILED** |

---

## Issue 1 (CRITICAL) — `runtime.Caller` breaks the primary STEP 18 deliverable in production

**What went wrong:**
`server/internal/app/handlers/openapi.go` uses `runtime.Caller(0)` to resolve the path to `docs/api/openapi.yaml`. This function returns the *source file path at compile time*. In a compiled Go binary deployed anywhere outside the original source tree — which includes every staging, CI, and production environment — the resolved path points to a directory that does not exist on the target host. The fallback to a relative path `docs/api/openapi.yaml` is also unreliable because it depends on the process's working directory at runtime. The result is that `GET /api/v1/openapi` — the primary deliverable of STEP 18 — will return `404 OpenAPI spec not found` in any compiled deployment.

**How to fix:**
Use Go's `embed` package to embed the spec at build time. Example:

```go
// In handlers/openapi.go
import _ "embed"

//go:embed ../../../../docs/api/openapi.yaml
var openapiSpec []byte

func (h *OpenAPIHandlers) ServeSpec(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/yaml")
    w.Header().Set("Cache-Control", "public, max-age=300")
    w.WriteHeader(http.StatusOK)
    w.Write(openapiSpec)
}
```

Alternatively, if embedding is not desired, inject the spec path as a constructor argument populated from an env var (`OPENAPI_SPEC_PATH`), defaulting to `docs/api/openapi.yaml`, and document that the binary must be run from the repo root or with the env var set.

**What counts as passing:**
`GET /api/v1/openapi` returns HTTP 200 with the full YAML content in a compiled binary when run outside the source tree (e.g., `go build ./... && ./api`).

---

## Issue 2 (CRITICAL) — Email campaign send endpoint has an unfixed security vulnerability

**What went wrong:**
`server/internal/app/handlers/comms.go` lines 307–315 contain an explicit `SECURITY TODO` comment acknowledging that the `/email-campaigns/{id}/send` endpoint accepts a caller-supplied recipient list without server-side validation that recipients are current org members. Any principal with `coach` or higher role can send org-branded email to arbitrary external addresses. The CLAUDE.md Definition of Done requires a feature flag or rollback plan for every task. Neither exists. The issue was identified but not remediated.

**How to fix:**
Implement one of the following:
1. Server-side validation: resolve the recipient list from database records (household contacts, swimmer emails) rather than accepting raw email addresses. Return 422 if any provided address is not found in the org's member directory.
2. Feature flag guard: add env var `COMMS_ALLOW_ARBITRARY_RECIPIENTS=false` (default false) and return 403 when the flag is disabled and non-member addresses are in the request.
3. At minimum: document the rollback plan — which feature flag, which env var, and which deployment step disables this — in `docs/planning/STEP18-implementation-notes.md`.

**What counts as passing:**
Either (a) the endpoint rejects recipient addresses not found in the org's member directory, or (b) an explicit feature flag is implemented and documented, or (c) the rollback plan is formally documented in a planning artifact per the Definition of Done.

---

## Issue 3 (IMPORTANT) — `nullable: true` is invalid in OpenAPI 3.1

**What went wrong:**
`docs/api/openapi.yaml` declares `openapi: 3.1.0` but uses `nullable: true` extensively throughout the schema definitions. `nullable` is an OpenAPI 3.0 extension keyword; it is not part of JSON Schema draft 2020-12, which OAS 3.1 is based on. The Spectral ruleset in `.spectral.yaml` sets `oas3-schema: error`. When Spectral is installed and `make lint-openapi` is run, these occurrences will produce errors, breaking the lint gate that STEP 18 was meant to establish.

**How to fix:**
Replace all `nullable: true` occurrences with the OAS 3.1-compliant nullable pattern. For a field that can be a string or null:

```yaml
# OAS 3.0 (wrong for 3.1):
my_field:
  type: string
  nullable: true

# OAS 3.1 (correct):
my_field:
  type: [string, "null"]
```

For `$ref` types that can be null:
```yaml
my_field:
  oneOf:
    - $ref: '#/components/schemas/MyType'
    - type: "null"
```

Run a global search-replace for `nullable: true` in `openapi.yaml` and replace each with the appropriate OAS 3.1 pattern.

**What counts as passing:**
`make lint-openapi` (with Spectral installed via `npx @stoplight/spectral-cli`) exits with zero errors for `oas3-schema` violations.

---

## Issue 4 (IMPORTANT) — `no-$ref-siblings` is not a valid built-in Spectral OAS rule

**What went wrong:**
`docs/api/.spectral.yaml` line 27 references `no-$ref-siblings: error`. This is not a built-in rule in `@stoplight/spectral-oas`. Depending on the Spectral version, this either silently ignores the unknown rule name (no enforcement) or produces a warning about an unknown rule. Either outcome means the intent of this guard is not enforced.

**How to fix:**
Remove `no-$ref-siblings` from the ruleset. In OpenAPI 3.1 with JSON Schema draft 2020-12, `$ref` siblings are natively supported (a resolved `$ref` merges with sibling keywords), so enforcing this restriction may not even be desirable. If you want to keep a strict no-sibling-with-`$ref` policy, use a custom Spectral function rule rather than a non-existent built-in.

**What counts as passing:**
`.spectral.yaml` contains no unknown rule names. Running `npx @stoplight/spectral-cli lint docs/api/openapi.yaml` produces no "unknown rule" warnings.

---

## Issue 5 (IMPORTANT) — `GET /authz/ping` is missing from the OpenAPI spec

**What went wrong:**
The router registers `GET /api/v1/authz/ping` (confirmed in `server/internal/app/server.go`). This endpoint is not documented anywhere in `docs/api/openapi.yaml`. The STEP 18 plan explicitly states the spec must cover ALL v1 endpoints.

**How to fix:**
Add a path entry for `/authz/ping` in `openapi.yaml`:
```yaml
/authz/ping:
  get:
    summary: Authorization ping (RBAC smoke test)
    tags: [Auth]
    security:
      - BearerAuth: []
    responses:
      '200':
        description: Caller is authenticated and has a valid role
        content:
          application/json:
            schema:
              type: object
              properties:
                status:
                  type: string
                  example: ok
      '401':
        $ref: '#/components/responses/Unauthorized'
      '403':
        $ref: '#/components/responses/Forbidden'
```

**What counts as passing:**
Every path registered in the router appears in the OpenAPI spec.

---

## Issue 6 (IMPORTANT) — Billing handlers lack `io.LimitReader` and `DisallowUnknownFields`

**What went wrong:**
`server/internal/app/handlers/billing.go` `decodeAndValidate` (lines 93–101) and the Stripe handler decode request bodies using bare `json.NewDecoder(r.Body).Decode()` with no body size limit and no unknown field rejection. The auth handler (`handlers/auth.go` line 349) uses `io.LimitReader(r.Body, 1<<20)` and `DisallowUnknownFields()`. This inconsistency exposes billing and Stripe endpoints to unbounded request bodies (potential DoS/memory exhaustion) and silently accepts extra JSON fields that should be rejected.

**How to fix:**
Update `billing.go`'s `decodeAndValidate` to match the auth handler pattern:
```go
func decodeAndValidate(r *http.Request, dst interface{}) error {
    dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
    dec.DisallowUnknownFields()
    if err := dec.Decode(dst); err != nil {
        return err
    }
    return validate.Struct(dst)
}
```
Apply the same fix to the Stripe handler's body decoding.

**What counts as passing:**
All handler packages that decode request bodies use `io.LimitReader` with a 1 MiB (or similar) cap and `DisallowUnknownFields()`, consistent with the auth handler pattern.

---

## Issue 7 (IMPORTANT) — `POST /invoices/{invoiceID}/write-off` missing 409 response in the spec

**What went wrong:**
The billing handler returns HTTP 409 via `mapBillingError` for `ErrInvalidTransition` (when the invoice is not in a write-off-eligible state). The OpenAPI spec for `POST /invoices/{invoiceID}/write-off` documents 200, 403, and 404 responses but omits 409. API consumers have no way to programmatically distinguish a state-machine conflict from other errors.

**How to fix:**
Add a 409 response to the write-off path in `openapi.yaml`:
```yaml
'409':
  $ref: '#/components/responses/Conflict'
```
Also verify that a `Conflict` response component exists in `components/responses`; if not, define it with an appropriate error schema.

**What counts as passing:**
All billing state-machine endpoints that can return 409 (`/issue`, `/void`, `/write-off`) document the 409 response in the spec.

---

## Master List of Files Edited in STEP 18

The following files were created or modified during STEP 18:

1. `docs/api/openapi.yaml` — Created: full OpenAPI 3.1 spec
2. `docs/api/.spectral.yaml` — Created: Spectral lint ruleset
3. `server/internal/app/handlers/openapi.go` — Modified: updated from stub to serve real spec file
4. `Makefile` — Modified: added `lint-openapi` target, wired into `make lint`
5. `TODO.md` — Modified: all STEP 18 tasks marked `[x]`
6. `docs/planning/STEP18-implementation-plan.md` — Created: implementation plan artifact

---

## What Counts as Passing (Summary)

All four conditions below must be well-met:

1. **No bugs/edge cases:** `GET /api/v1/openapi` must return HTTP 200 with YAML content in a compiled binary run outside the source tree. The `nullable: true` OAS 3.1 violations must be fixed. All handler 4xx responses that can actually occur must be documented in the spec.

2. **No regressions in code quality:** All handler packages must use `io.LimitReader` and `DisallowUnknownFields` consistently. The `nightly-integrity` job should be on a semantically appropriate queue.

3. **No security issues:** The email campaign send endpoint must not accept arbitrary external recipient addresses without server-side org-member validation or an explicit feature flag + documented rollback plan.

4. **Directions fully followed:** Every router-registered endpoint must appear in `openapi.yaml`. The `.spectral.yaml` ruleset must contain only valid, known Spectral rule names. `make lint-openapi` must pass with zero errors when Spectral is installed.
