# STEP 32D Remove Remaining UI Stub Flows Plan

## Goal

Finish `TODO.md` step `32D)` by replacing the last two UI-only stub flows with
real backend-backed behavior:

- portal document uploads must use a real presigned object-storage upload and
  then attach only a validated object key to the submission
- admin meet creation must replace the fake timer/mock-session flow with a real
  backend parse request that returns derived session drafts

## Scope

1. Add backend support for document upload handoff:
   - request a presigned upload URL for a document submission
   - validate that attached object keys match the submission scope and exist in
     object storage
2. Add backend support for meet-file parsing:
   - accept a real meet file payload from the admin flow
   - parse with the existing `internal/meetfiles` module
   - derive session drafts from parsed meet metadata instead of UI mock data
3. Update the portal/admin UIs to call the live APIs.
4. Add focused regression coverage for both flows.

## Notes

- Keep DB-backed document submissions and meet sessions as the source of truth.
- Avoid adding new process-local state; upload/parse flows should be request
  driven.
- Session derivation can be conservative when the file lacks explicit schedule
  details; the key requirement is that the backend, not the UI, produces the
  session drafts.

## Checklist

- [x] Add document upload request/validation backend support
- [x] Wire parent portal document upload to presign + PUT + attach
- [x] Add meet-file parse endpoint/service logic
- [x] Wire admin meet creation to live parse response
- [x] Add regression tests for upload and meet-parse live backend flows
- [x] Update `CLAUDE.md` with new project conventions
- [x] Final diff review against `TODO.md` step `32D`

## Verification Notes

- Targeted Go tests pass with `GOCACHE=/tmp/go-build`:
  - `TestDocumentUploadURLAndAttachValidateObjectStorage`
  - `TestDocumentAttachRejectsObjectKeyOutsideSubmissionScope`
  - `TestMeetParseEntryFileDerivesSessionsFromBackendParser`
- `npm --prefix /Users/ryan/Documents/aquify/admin test` passes, including the
  portal document action regression coverage in
  `admin/lib/document-submissions.test.ts`.
