# Data Handling and Subprocessors

Status: engineering description of what the platform does today. It is the factual basis for the public
Privacy policy (`admin/app/(legal)/privacy/page.tsx`) and for the USA Swimming API Access Program application.
It is not legal advice; counsel must review the policy and terms before launch (see "Open items").

## Data we hold

| Category | Fields | Sensitivity |
|---|---|---|
| Accounts | name, email, phone, role, MFA/session metadata | personal |
| Swimmer profile | name, preferred name, DOB, gender, USA Swimming ID, registration status, groups, bio (<= 400 chars) | **children's data** |
| Health and safety | emergency contact, allergies, medical flags, waivers, uploaded documents | **children's data, sensitive** |
| Swimming activity | commitments, entries, results/times, attendance, goals, training, volunteer hours, wellness check-ins | **children's data** |
| Billing | invoices, payments, credits, autopay settings. Card numbers are never stored (Stripe) | financial |
| Technical | IP, user agent, session records (hashed tokens), audit events | security |

## Children (COPPA)

- Parent account required for every household-critical workflow; swimmer accounts are optional (DECISIONS.md).
- A swimmer **under 13** cannot be linked to a sign-in, and cannot sign in, without recorded parental consent:
  - Consent fields on `swimmers`: `guardian_consent_at`, `guardian_consent_by`, `guardian_consent_version`
    (migration `20260725000100`).
  - Only a **parent of the household** can grant (`PATCH /members/swimmers/{id}` `{guardian_consent:true}`);
    staff and the swimmer cannot. A parent or staff can withdraw, which also unlinks the sign-in.
  - Enforcement: `LinkSwimmerToUser` returns `guardian_consent_required` (409); `SwimmerPortalDisabled` fails
    closed for unconsented under-13 swimmers, so already-linked accounts are blocked until a parent consents.
  - `GuardianConsentVersion` (members/service.go) must equal `LEGAL_VERSION` (`admin/lib/legal.ts`).
- No advertising, no sale of data, no public swimmer profiles or results. The public club site is a boilerplate
  page: club name, about text, contact details, and join/sign-in links. Team records and swimmer names were
  removed from the public payload.
- Retention and deletion: see below. Parents can review (portal), withdraw consent (portal), and request
  deletion (compliance data-deletion request, admin approval + MFA, enqueues `compliance:data_deletion`).

## Subprocessors

| Provider | Purpose | Data involved |
|---|---|---|
| Stripe | payments, checkout, refunds | payer details, amounts, invoice metadata |
| Postmark | transactional and announcement email | recipient email/name, message content |
| Cloudflare | edge, WAF, TLS termination | request metadata, traffic |
| VPS/hosting + object storage (US West) | compute, database, files, backups | all categories |
| USA Swimming (SWIMS vendor API), only when a club enables it | roster compare, registration, member and time verification | member IDs, names, DOB, registration details |

Keycloak (identity) is self-hosted and is not a third party.

## Retention

| Data | Retention |
|---|---|
| Account and swimmer records | while the household is active with the club |
| After approved deletion request | deleted or de-identified, except the rows below |
| Invoices, payments, ledger | kept as accounting/tax law requires |
| Audit events | `AUDIT_RETENTION_DAYS`, default 2555 (~7 years) |
| Encrypted backups | ~30 days (backup-restore-runbook.md) |
| Export/report artifacts | expire via the `purge expired files` worker job |

## Security controls

TLS via Cloudflare with an origin certificate; Cloudflare WAF; deny-by-default RBAC with central org scoping and
household scoping; MFA on sensitive admin actions; hashed session tokens; third-party credentials AES-GCM
encrypted (`INTEGRATIONS_ENCRYPTION_KEY`, required at startup outside development); audit events for privileged
actions; GPG-encrypted offsite backups; breach-response runbook (`breach-response-runbook.md`).

Not in place today: application-level encryption of `medical_flags`/`allergies` (protection relies on host and
database controls), and upload malware scanning.

## USA Swimming application: suggested answers

**Section 3, how this supports USA Swimming's mission.** aquify gives member clubs one system for registration,
meet entries and results. Registration status and member IDs are checked against USA Swimming's records instead
of being re-keyed, and meet entries are exported in the standard host file formats, which reduces data-entry
errors for clubs and meet hosts.

**Section 5, data handling.** Data is held in PostgreSQL and S3-compatible object storage in the US, behind
Cloudflare. Access is limited by role and household, administrators use MFA for sensitive actions, and privileged
actions are audit-logged. USA Swimming data is requested only when a club enables the integration and is used only
to display roster and membership status, submit registrations, and verify members and times for that club. It is
not used for marketing and is not shared with other parties. Backups are encrypted and rotate within about 30
days. Swimmers under 13 can only have an account with recorded parental consent.

**Data protection laws.** We comply with COPPA for children's data (parental consent, review and withdrawal, no
advertising or sale, published retention) and honor CCPA-style access, correction and deletion requests. The
service is directed to US clubs and is not offered to people in the EEA/UK, so GDPR is not expected to apply;
choose "Yes" for COPPA-aligned practices only after counsel has signed off on the policy.

**Shared with third parties.** Yes: processors only, as listed above (Stripe, Postmark, Cloudflare, hosting). No
sale, no advertising.

## Open items (before relying on the above)

1. Counsel reviews `/privacy` and `/terms`, including the governing-law clause (deliberately generic).
2. Confirm the contact mailbox in `LEGAL_CONTACT_EMAIL` (default `privacy@aquify.club`, override with
   `NEXT_PUBLIC_PRIVACY_CONTACT_EMAIL`) exists and is monitored.
3. Confirm the retention behavior of the data-deletion worker matches the table above.
4. Decide on field-level encryption for medical data and upload scanning, or accept the residual risk in writing.
5. Check the policy statement that medical details are limited to staff and the swimmer's household against the
   read paths for every role.

## Roles when several clubs share the platform (added 2026-11-03)

Each club is the **controller** of its members' data; Aquify operates the platform as the club's **processor**. Consequences implemented in the product:

- A club's data is addressed only by its own `org_id`; clubs cannot see or reach one another's data (cross-org tests run in CI).
- Each club takes payments on its **own Stripe account**; Aquify does not hold the club's funds.
- A club can be closed by a platform operator: access stops immediately, the club receives a full data export, and after a cooling-off period (default 14 days) its identity-provider users, stored files and database rows are deleted. The audit trail is retained as a processing record; the registry row is kept as a tombstone (no personal data).
- The platform privacy policy and the guardian-consent version (`LEGAL_VERSION`) remain platform-level documents; a club that needs its own terms must publish them on its public site.
- Sub-processors are unchanged (Keycloak, Postgres, object storage, Stripe, Postmark); Stripe Connect adds Stripe as processor for each club's connected account.
