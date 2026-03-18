# STEP 18: API Contract and Docs — Implementation Plan

## Goal
Produce a comprehensive OpenAPI 3.1 spec covering all `v1` endpoints, wire the spec into the `/api/v1/openapi` route, add a lint config, and gate CI on schema validity.

## Deliverables
1. `/docs/api/openapi.yaml` — Full OpenAPI 3.1 spec
2. `/docs/api/.spectral.yaml` — Spectral lint ruleset
3. Updated `server/internal/app/handlers/openapi.go` — Serve real spec file
4. Updated `Makefile` — `make lint-openapi` target + wired into `make lint`
5. `TODO.md` — All STEP 18 tasks marked done

## Endpoint Inventory (all groups)

### Auth (public + protected)
- POST /api/v1/auth/oidc/callback
- POST /api/v1/auth/sessions/issue
- POST /api/v1/auth/sessions/refresh
- POST /api/v1/auth/sessions/revoke
- POST /api/v1/auth/mfa/enroll (protected)
- POST /api/v1/auth/mfa/challenge (protected)
- POST /api/v1/auth/roles/assign (protected, owner/admin + MFA)

### Health (public)
- GET /api/v1/health/live
- GET /api/v1/health/ready

### Members (protected)
- POST /api/v1/members/households
- PATCH /api/v1/members/households/{householdID}
- POST /api/v1/members/households/{householdID}/contacts
- POST /api/v1/members/swimmers
- PATCH /api/v1/members/swimmers/{swimmerID}
- POST /api/v1/members/swimmers/{swimmerID}/groups
- GET /api/v1/members/swimmers

### Groups / Eligibility / Registrations (protected)
- POST /api/v1/groups
- PATCH /api/v1/groups/{groupID}
- GET /api/v1/groups/{groupID}
- GET /api/v1/groups
- POST /api/v1/groups/{groupID}/eligibility-rules
- GET /api/v1/groups/{groupID}/eligibility/{swimmerID}
- POST /api/v1/registrations
- POST /api/v1/registrations/{registrationID}/transition
- POST /api/v1/registrations/{registrationID}/waivers

### Meets (protected)
- POST /api/v1/meets
- GET /api/v1/meets
- GET /api/v1/meets/{meetID}
- POST /api/v1/meets/{meetID}/sessions
- GET /api/v1/meets/{meetID}/sessions
- POST /api/v1/meets/{meetID}/events
- GET /api/v1/meets/{meetID}/events
- POST /api/v1/meets/{meetID}/transition
- POST /api/v1/meets/{meetID}/commitment-request
- GET /api/v1/meets/{meetID}/commitments
- POST /api/v1/meets/{meetID}/commitments/{commitmentID}/respond
- POST /api/v1/meets/{meetID}/commitments/{commitmentID}/event-selections
- GET /api/v1/meets/{meetID}/commitments/{commitmentID}/event-selections
- DELETE /api/v1/meets/{meetID}/commitments/{commitmentID}/event-selections/{selectionID}
- POST /api/v1/meets/{meetID}/lock-commitments
- GET /api/v1/meets/{meetID}/entries
- POST /api/v1/meets/{meetID}/entries
- POST /api/v1/meets/{meetID}/entries/{entryID}/override
- POST /api/v1/meets/{meetID}/entries/{entryID}/approve
- POST /api/v1/meets/{meetID}/exports
- GET /api/v1/meets/{meetID}/exports

### Imports (protected)
- POST /api/v1/imports/upload-url
- GET /api/v1/imports
- GET /api/v1/imports/{importID}
- POST /api/v1/imports/{importID}/finalize
- GET /api/v1/imports/{importID}/conflicts
- POST /api/v1/imports/{importID}/conflicts/{conflictID}/resolve
- GET /api/v1/imports/{importID}/report

### Exports (protected)
- GET /api/v1/exports/{exportID}
- GET /api/v1/exports/{exportID}/download

### Performance (protected)
- POST /api/v1/results
- GET /api/v1/swimmers/{swimmerID}/best-times
- POST /api/v1/swimmers/{swimmerID}/best-times/override
- GET /api/v1/swimmers/{swimmerID}/progression
- GET /api/v1/swimmers/{swimmerID}/cut-qualification
- GET /api/v1/time-standards
- POST /api/v1/time-standards
- GET /api/v1/time-standards/{standardID}
- PUT /api/v1/time-standards/{standardID}
- DELETE /api/v1/time-standards/{standardID}

### Billing (protected)
- POST /api/v1/invoices
- GET /api/v1/invoices/{invoiceID}
- POST /api/v1/invoices/{invoiceID}/items
- DELETE /api/v1/invoices/{invoiceID}/items/{itemID}
- GET /api/v1/invoices/{invoiceID}/items
- POST /api/v1/invoices/{invoiceID}/issue
- POST /api/v1/invoices/{invoiceID}/void
- POST /api/v1/invoices/{invoiceID}/write-off
- POST /api/v1/invoices/{invoiceID}/payments
- GET /api/v1/invoices/{invoiceID}/payments
- POST /api/v1/invoices/{invoiceID}/refunds
- GET /api/v1/invoices/{invoiceID}/refunds
- GET /api/v1/households/{householdID}/invoices
- GET /api/v1/ledger-events

### Stripe (protected + public webhook)
- POST /api/v1/stripe/payment-intents
- POST /api/v1/stripe/checkout-sessions
- GET /api/v1/stripe/webhook-events
- GET /api/v1/stripe/reconciliation
- POST /api/v1/webhooks/stripe (public)

### Communications (protected)
- POST /api/v1/announcements
- GET /api/v1/announcements
- GET /api/v1/announcements/{announcementID}
- POST /api/v1/announcements/{announcementID}/publish
- POST /api/v1/announcements/{announcementID}/archive
- POST /api/v1/email-campaigns
- GET /api/v1/email-campaigns
- GET /api/v1/email-campaigns/{campaignID}
- POST /api/v1/email-campaigns/{campaignID}/send
- GET /api/v1/email-campaigns/{campaignID}/deliveries
- POST /api/v1/suppression
- GET /api/v1/suppression
- DELETE /api/v1/suppression/{suppressionID}

### Audit (protected, owner/admin only)
- GET /api/v1/audit-events

### Compliance (protected)
- POST /api/v1/compliance/data-export-requests
- GET /api/v1/compliance/data-export-requests
- GET /api/v1/compliance/data-export-requests/{requestID}
- POST /api/v1/compliance/data-deletion-requests
- GET /api/v1/compliance/data-deletion-requests
- GET /api/v1/compliance/data-deletion-requests/{requestID}

### Jobs Admin (protected, owner/admin only)
- GET /api/v1/jobs
- GET /api/v1/jobs/queues
- GET /api/v1/jobs/metrics
- GET /api/v1/jobs/dlq
- POST /api/v1/jobs/dlq/{taskID}/retry
- DELETE /api/v1/jobs/dlq/{taskID}

## Security Scheme
- `BearerAuth`: HTTP Bearer JWT (Authorization: Bearer <token>)
- `OrgScope`: API key in header (X-Org-Id)

## Common Patterns
- Error shape: `{"error": {"code": "...", "message": "...", "details": {...}}}`
- Most protected routes require both BearerAuth + OrgScope
- Pagination: `cursor` + `limit` (audit), `count` + array (others)
- IDs are UUIDs (string format: uuid)

## Implementation Status
- [x] Planning file created
- [ ] openapi.yaml created
- [ ] .spectral.yaml created
- [ ] openapi.go updated to serve file
- [ ] Makefile updated with lint-openapi target
- [ ] TODO.md updated
