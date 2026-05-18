# STEP 29G — Document Center and Household Compliance

## Status: COMPLETE (2026-03-22)

## Goal
Implement the real document center backend (document types catalog + household
submissions) and wire it to the admin and parent portal UIs.

## Scope

### 1. DB Migrations
- `20260322000100_create_document_types.sql` — org-scoped catalog of document
  types (waivers, medical, insurance, release, registration, other) with
  expiry, category, required flag, and applies_to scope.
- `20260322000200_create_document_submissions.sql` — household/swimmer document
  submission records with status machine, file storage ref, review notes,
  expiry, and reminder tracking.

### 2. Backend — documents service package
- `server/internal/app/documents/service.go` — core logic for document type
  management and submission workflow.
- `server/internal/app/documents/repository.go` — pgx/v5 DB-backed repository
  with in-memory fallback.

### 3. HTTP Handlers
- `server/internal/app/handlers/documents.go`
  - `GET  /api/v1/document-types`            — list active types
  - `POST /api/v1/document-types`            — create (owner/admin)
  - `PUT  /api/v1/document-types/{typeID}`   — update (owner/admin)
  - `DELETE /api/v1/document-types/{typeID}` — soft delete (owner/admin)
  - `GET  /api/v1/documents`                 — list submissions (admin: all; parent: own household)
  - `POST /api/v1/documents`                 — submit/create record (parent/admin)
  - `GET  /api/v1/documents/{id}`            — get single submission
  - `POST /api/v1/documents/{id}/verify`     — admin verify
  - `POST /api/v1/documents/{id}/reject`     — admin reject
  - `POST /api/v1/documents/{id}/upload-url` — get presigned upload URL

### 4. Wire into server.go

### 5. Frontend — adminData.ts
- Add DocumentTypeRecord and DocumentSubmissionRecord types
- Add CRUD fetch functions for document types and submissions

### 6. Admin UI
- `/compliance/documents/page.tsx` — manage document types, view submissions

### 7. Parent portal
- Update `/portal/documents/page.tsx` to use live API instead of stub data

### 8. Tests
- `server/internal/app/server_documents_test.go`

## Status Checklist
- [x] DB migrations created
- [x] documents service created
- [x] HTTP handlers created
- [x] server.go wired
- [x] adminData.ts updated
- [x] admin UI page created (/compliance/documents)
- [x] parent portal wired to live API
- [x] tests written (server_documents_test.go — 5 tests, all pass)
- [x] build passes (Go + Next.js)
