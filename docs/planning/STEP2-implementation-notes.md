# STEP 2 Implementation Notes

Date: 2026-03-15
Owner: Codex
Purpose: execute TODO.md section `2) Local infrastructure and environments`.

## Execution Checklist
- [x] Add `docker-compose.yml` with Postgres, Redis, and mail test service
- [x] Add healthcheck for Postgres service
- [x] Add healthcheck for Redis service
- [x] Add named volume for Postgres data
- [x] Add named volume for Redis data
- [x] Add local object storage service (S3-compatible) in compose
- [x] Add `.env.local.example` for local development
- [x] Add script to wait for DB readiness
- [x] Add `make infra-up` target
- [x] Add `make infra-down` target
- [x] Add `make infra-reset` target
- [x] Document local ports and credentials in README

## Notes
- Local infra stack includes Postgres 16, Redis 7, MinIO (S3-compatible), and Mailpit for email sandbox testing.
- `scripts/wait-for-db.sh` supports both `pg_isready` and `nc` checks to avoid hard dependency on a single host tool.
- Infrastructure make targets call Docker Compose using `.env.local`, source `.env.local` for DB readiness checks, and keep DB migration targets untouched for STEP 3.
