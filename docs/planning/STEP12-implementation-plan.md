# STEP 12: Best Times and Performance History — Implementation Plan

## Overview
Implement the performance history module: database tables, service layer, and HTTP endpoints for results, splits, best times, time standards, and cut qualification.

## TODO Tasks
- [x] Create `results` table migration
- [x] Create `splits` table migration
- [x] Create `best_times` table migration
- [x] Create `time_standards` table migration
- [x] Service: upsert result record from parsed import
- [x] Service: recompute best time for swimmer/event/course
- [x] Service: update future entry seed times from results import
- [x] Endpoint: list swimmer best times
- [x] Endpoint: list swimmer progression timeline
- [x] Endpoint: manage time standards (CRUD)
- [x] Endpoint: evaluate cut qualification
- [x] Audit event for manual best-time override
- [x] Register routes in server.go
- [x] Write server tests
- [x] Update CLAUDE.md

## Schema Design

### results
- id, org_id, swimmer_id (FK swimmers), meet_id (FK meets nullable), meet_event_id (FK meet_events nullable)
- import_id (FK file_imports nullable — source import job)
- event_number, distance INT, stroke TEXT, course TEXT (SCY/SCM/LCM), gender TEXT (M/F)
- heat INT nullable, lane INT nullable, place INT nullable
- fin_time_ms INT nullable (null = NT or DQ), dq BOOL default false, dq_code TEXT, dq_note TEXT
- seed_time_ms INT nullable
- is_official BOOL default true, is_manual_override BOOL default false
- override_reason TEXT, override_by (FK users nullable)
- status TEXT CHECK (active | voided | archived)
- standard audit columns + deleted_at/deleted_by

### splits
- id, org_id, result_id (FK results), split_number INT, split_distance INT
- split_time_ms INT, cumulative_time_ms INT
- standard audit columns

### best_times
- id, org_id, swimmer_id (FK swimmers)
- event_distance INT, stroke TEXT, course TEXT
- best_time_ms INT nullable (null = NT), result_id (FK results nullable)
- is_manual_override BOOL default false, override_reason TEXT
- override_by (FK users nullable)
- recorded_at TIMESTAMPTZ nullable
- UNIQUE (org_id, swimmer_id, event_distance, stroke, course)
- standard audit columns

### time_standards
- id, org_id, name TEXT, season_year INT
- event_distance INT, stroke TEXT, course TEXT, gender TEXT
- age_min INT nullable, age_max INT nullable
- time_ms INT (cut time in ms), standard_code TEXT (AA/A/BB/B/etc)
- description TEXT, is_active BOOL default true
- standard audit columns + deleted_at/deleted_by

## API Endpoints

### Results/Best Times
- `POST   /results`                              — upsert result from import (admin/coach)
- `GET    /swimmers/{swimmerID}/best-times`      — list best times
- `POST   /swimmers/{swimmerID}/best-times/override` — manual BT override with audit
- `GET    /swimmers/{swimmerID}/progression`     — progression timeline (event/stroke/course query params)
- `GET    /swimmers/{swimmerID}/cut-qualification` — evaluate cut quals (standard_id param)

### Time Standards
- `GET    /time-standards`          — list
- `POST   /time-standards`          — create (admin only)
- `GET    /time-standards/{id}`     — get
- `PUT    /time-standards/{id}`     — update (admin only)
- `DELETE /time-standards/{id}`     — delete (admin only)

## Audit Events
- `best_time.manual_override` — actor, swimmer_id, before/after values, reason
- `result.upserted` — actor, swimmer_id, result details

## Role Checks
- Results upsert: admin, coach
- Best time override: admin only (requires reason)
- Time standards manage: admin only
- Read endpoints: owner, admin, coach, parent (own household)
