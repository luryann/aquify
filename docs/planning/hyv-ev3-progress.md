# HYV / EV3 Create-Meet Progress

## Status
- [x] Copied sample `.hyv` / `.ev3` fixtures into repo testdata.
- [x] Added initial meetfile detection and parser scaffolding for `.hyv` / `.ev3`.
- [x] Extended backend parsed-meet response with events and signup settings.
- [x] Persisted meet signup settings on the meet record.
- [x] Supported atomic create-meet with nested sessions and events.
- [x] Updated create-meet UI to preview parsed events and save the full setup.
- [x] Accepted `.hyv` / `.ev3` in generic import validation and UI.
- [x] Updated OpenAPI/contracts and added focused regression tests.

## Notes
- `.ev3` is the richer event-definition source and should win over `.hyv` inside generic meet ZIP uploads.
- `.hyv` parsing should still create signup-ready events, but session assignment may remain blank when the source file does not provide reliable session structure.
- Focused Go tests for the new parser and create-meet flow are passing; the full server test suite still contains unrelated sandbox-blocked `miniredis` cases.


ORIGINAL PLAN:
# Create-Meet Upload Auto-Populates Signup From `.hyv` / `.ev3`

## Summary
Make the Create Meet upload flow capable of turning a HyTek meet file into a usable signup-ready meet, not just session drafts. Uploading `.hyv` or `.ev3` in Create Meet should prefill meet metadata, schedule, event catalog, and signup settings; saving should persist the meet, sessions, events, and rules so the portal event-selection screen works immediately.

## Key Changes
- Extend `internal/meetfiles` to support standalone `.hyv` and `.ev3`, plus `.zip` archives containing them.
  Use the sample files in [/Users/ryan/Downloads/meetSampleFiles](/Users/ryan/Downloads/meetSampleFiles) as the parser contract and copy repo-safe fixtures into testdata for implementation.
- Expand the meet-file parse result returned by `POST /api/v1/meets/parse-entry-file` to include:
  `sessions`,
  `events`,
  `signup_settings`.
- Parser behavior:
  `.ev3` is the rich source of truth for event catalog plus session/day/start-time mapping.
  `.hyv` provides meet metadata and event catalog but not reliable session assignment; derive sessions from dates as today and create events with no session binding unless a deterministic mapping is available.
  For generic meet ZIP uploads, prefer `.ev3` over `.hyv`, then fall back to the existing `.sd3` / `.cl2` logic.
- Extend Create Meet UI so upload populates:
  Basic Info: meet name, course, dates, host/location when present.
  Schedule: session cards.
  Entry Rules / Signup: inferred max entries per swimmer, self-signup enabled, and imported event count summary.
  Event catalog preview: show the parsed events that will be created on save.
- Persist signup-ready data in one save operation by extending `POST /api/v1/meets` to accept optional nested payloads:
  `sessions[]`,
  `events[]`,
  `signup_settings`.
  When absent, preserve current create-meet behavior.
- Add backend persistence for meet-level signup settings needed by the current UI:
  `self_signup_enabled` boolean,
  `max_entries_per_swimmer` integer.
  Do not add a separate stored `age_groups` field; derive displayed age groups from imported event min/max age ranges.
- On save, create the meet and all child records atomically.
  In DB-backed mode, use one repository transaction so a failed event insert cannot leave a half-created signup setup.
- Auto-created events must populate the existing meet-event model fields used by signup:
  `event_number`, `stroke`, `distance`, `gender`, `min_age`, `max_age`, `course`, `is_relay`, optional `session_id`.
  Leave unsupported HyTek data such as cuts/qualification flags out of scope unless already representable.
- Keep generic imports in sync by accepting `.hyv` and `.ev3` in validation/docs/UI, but do not add real finalize-time parsing there yet.

## Public API / Type Changes
- Add `hyv` and `ev3` to supported file types.
- Extend parsed-meet response with:
  `events: ParsedEventDraft[]`
  `signup_settings: { self_signup_enabled: boolean; max_entries_per_swimmer: number }`
- Extend meet create input with optional:
  `sessions[]`
  `events[]`
  `signup_settings`
- Extend persisted meet model / DB schema with:
  `self_signup_enabled`
  `max_entries_per_swimmer`

## Test Plan
- Detector tests for `.hyv` and `.ev3`, including ZIP precedence.
- Fixture-backed parser tests for standalone `.hyv` and `.ev3`.
- API tests for `/api/v1/meets/parse-entry-file` proving returned sessions, events, and signup settings for both formats.
- API/service tests for extended `POST /api/v1/meets` proving one request creates:
  meet,
  sessions,
  events,
  persisted signup settings.
- Portal-facing tests proving a meet created from parsed upload exposes selectable events in the event-selection flow.
- Import endpoint tests proving `.hyv` / `.ev3` are accepted by upload validation and OpenAPI contract tests stay aligned.

## Assumptions And Defaults
- The intended extension is `.ev3`, not `.en3`.
- “Automatically populate signup” means the meet has real persisted events after save, so the portal selection step can render options without manual event entry.
- `.ev3` is preferred over `.hyv` whenever both are available because it carries better session structure.
- `.hyv` events may be persisted without `session_id` when session mapping is not trustworthy; the signup UI should treat them as ungrouped/main-session events rather than blocking creation.
- Qualification cuts and other event constraints are not added unless they are already supported by the current backend model.

## Primary Touchpoints
- [service.go](/Users/ryan/Documents/aquify/server/internal/app/meets/service.go)
- [page.tsx](/Users/ryan/Documents/aquify/admin/app/(admin)/meets/new/page.tsx)
- [detector.go](/Users/ryan/Documents/aquify/internal/meetfiles/detector.go)
