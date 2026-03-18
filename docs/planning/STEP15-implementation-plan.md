# STEP 15: Communications Module — Implementation Plan

## Overview
Implement the communications module: announcements (org-wide broadcast) and email campaigns (segmented Postmark sends) with delivery tracking and suppression list enforcement.

## TODO Coverage
- [ ] Create `announcements` table migration
- [ ] Create `email_campaigns` table migration
- [ ] Create `email_deliveries` table migration
- [ ] Create `suppression_list` table migration
- [ ] Integrate Postmark send client (interface + stub, no external SDK)
- [ ] Implement comms service (in-memory, consistent with prior steps)
- [ ] Implement comms handlers
- [ ] Wire comms into server.go + config.go
- [ ] Add server_comms_test.go
- [ ] Update TODO.md and CLAUDE.md

## Schema Design

### announcements
- id UUID PK, org_id UUID FK
- title TEXT NOT NULL, body TEXT NOT NULL
- status TEXT: draft | published | archived
- audience TEXT: all | group
- group_id UUID FK groups nullable (scopes audience to a group)
- published_at TIMESTAMPTZ nullable, archived_at TIMESTAMPTZ nullable
- created_at, updated_at, created_by, updated_by

### email_campaigns
- id UUID PK, org_id UUID FK
- name TEXT NOT NULL, subject TEXT NOT NULL
- body_html TEXT NOT NULL, body_text TEXT nullable
- from_name TEXT NOT NULL, from_email TEXT NOT NULL
- audience TEXT: all | group
- group_id UUID FK groups nullable
- status TEXT: draft | sending | sent | failed | cancelled
- sent_at TIMESTAMPTZ nullable
- total_recipients INT default 0, sent_count INT default 0, failed_count INT default 0
- created_at, updated_at, created_by, updated_by

### email_deliveries
- id UUID PK, org_id UUID FK
- campaign_id UUID FK email_campaigns nullable
- recipient_email TEXT NOT NULL
- recipient_user_id UUID FK users nullable
- status TEXT: pending | sent | delivered | bounced | failed | suppressed
- provider_message_id TEXT nullable (Postmark message ID)
- error_message TEXT nullable
- sent_at, delivered_at, bounced_at TIMESTAMPTZ nullable
- created_at, updated_at

### suppression_list
- id UUID PK, org_id UUID FK
- email TEXT NOT NULL (lowercased), reason TEXT: bounce | complaint | manual
- created_at, created_by
- UNIQUE(org_id, email)

## Postmark Client Interface
Follows StubClient/Client pattern from `server/internal/stripe/client.go`.
- `Client` interface: `SendEmail(ctx, msg SendEmailRequest) (SendEmailResponse, error)`
- `StubClient`: in-memory; records sent messages in a slice for test assertions
- Real HTTP client can be substituted without touching service/handler code
- Config env vars: `POSTMARK_SERVER_TOKEN`, `POSTMARK_FROM_EMAIL`, `POSTMARK_FROM_NAME`

## Endpoints
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| POST | /announcements | owner,admin,coach | Create announcement |
| POST | /announcements/{id}/publish | owner,admin,coach | Publish announcement |
| POST | /announcements/{id}/archive | owner,admin | Archive announcement |
| GET | /announcements | all authenticated | List announcements |
| GET | /announcements/{id} | all authenticated | Get announcement |
| POST | /email-campaigns | owner,admin,coach | Create campaign draft |
| GET | /email-campaigns | owner,admin,coach | List campaigns |
| GET | /email-campaigns/{id} | owner,admin,coach | Get campaign + stats |
| POST | /email-campaigns/{id}/send | owner,admin,coach | Send campaign (suppression enforced) |
| GET | /email-campaigns/{id}/deliveries | owner,admin,coach | View delivery details |
| POST | /suppression | owner,admin | Add email to suppression list |
| GET | /suppression | owner,admin | List suppression entries |
| DELETE | /suppression/{id} | owner,admin | Remove from suppression list |

## Suppression Enforcement
At send time:
1. Resolve recipient email list from audience (all users in org, or group members).
2. Strip all emails that appear in the org's suppression_list.
3. Create email_deliveries with status=suppressed for stripped addresses.
4. Send remaining via Postmark; create email_deliveries with status=sent or failed.
5. Update campaign sent_count, failed_count, total_recipients.

## New Files
1. `server/internal/postmark/client.go` — Client interface, types, StubClient
2. `server/internal/app/comms/service.go` — in-memory comms service
3. `server/internal/app/handlers/comms.go` — HTTP handlers
4. `server/internal/app/server_comms_test.go` — tests
5. DB migrations 004100–004400

## Modified Files
1. `server/internal/app/server.go` — wire comms service + handlers
2. `server/internal/app/config.go` — add PostmarkConfig
3. `TODO.md` — mark tasks complete
4. `CLAUDE.md` — update project status
