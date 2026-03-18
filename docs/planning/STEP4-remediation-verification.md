# STEP 4 Remediation Runtime Verification

Date: 2026-03-15T17:26:54Z
Database URL: postgres://ryan@localhost:5432/aquify?sslmode=disable

## make db-up
2026/03/15 10:26:55 goose: no migrations to run. current version: 20260315000700

## make db-status (after up)
2026/03/15 10:26:56     Applied At                  Migration
2026/03/15 10:26:56     =======================================
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000100_init_extensions.sql
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000200_create_users.sql
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000300_create_roles.sql
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000400_create_user_roles.sql
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000500_create_sessions.sql
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000600_create_mfa_factors.sql
2026/03/15 10:26:56     Sun Mar 15 10:25:45 2026 -- 20260315000700_harden_user_roles_uniqueness.sql

## make db-down
2026/03/15 10:26:56 OK   20260315000700_harden_user_roles_uniqueness.sql (10.62ms)

## make db-status (after down)
2026/03/15 10:26:56     Applied At                  Migration
2026/03/15 10:26:56     =======================================
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000100_init_extensions.sql
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000200_create_users.sql
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000300_create_roles.sql
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000400_create_user_roles.sql
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000500_create_sessions.sql
2026/03/15 10:26:56     Sun Mar 15 09:59:12 2026 -- 20260315000600_create_mfa_factors.sql
2026/03/15 10:26:56     Pending                  -- 20260315000700_harden_user_roles_uniqueness.sql

## post-check restore
2026/03/15 10:26:57 OK   20260315000700_harden_user_roles_uniqueness.sql (3.92ms)
2026/03/15 10:26:57 goose: successfully migrated database to version: 20260315000700
2026/03/15 10:26:57     Applied At                  Migration
2026/03/15 10:26:57     =======================================
2026/03/15 10:26:57     Sun Mar 15 09:59:12 2026 -- 20260315000100_init_extensions.sql
2026/03/15 10:26:57     Sun Mar 15 09:59:12 2026 -- 20260315000200_create_users.sql
2026/03/15 10:26:57     Sun Mar 15 09:59:12 2026 -- 20260315000300_create_roles.sql
2026/03/15 10:26:57     Sun Mar 15 09:59:12 2026 -- 20260315000400_create_user_roles.sql
2026/03/15 10:26:57     Sun Mar 15 09:59:12 2026 -- 20260315000500_create_sessions.sql
2026/03/15 10:26:57     Sun Mar 15 09:59:12 2026 -- 20260315000600_create_mfa_factors.sql
2026/03/15 10:26:57     Sun Mar 15 10:26:57 2026 -- 20260315000700_harden_user_roles_uniqueness.sql
