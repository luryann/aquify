# DARE Platform — Quarterly Restore Drill Checklist

## Purpose

This checklist governs the quarterly restore drill, which validates that the backup and restore procedures work end-to-end before an actual incident forces a real restore under pressure. The drill simulates a complete VPS loss by restoring the DARE platform onto a clean staging VM from the latest offsite backup.

**Frequency:** Once per quarter (January, April, July, October)

**Duration:** Allow 2–4 hours for the full drill.

**Who runs it:** On-call engineer (or designated engineer) + engineering lead sign-off.

---

## Pre-Drill Checklist

Complete these items before starting the drill:

- [ ] **Confirm a recent backup exists** and is accessible offsite.
  ```bash
  rclone ls b2:dare-backups-offsite/daily/ | sort | tail -5
  ```
  Expected: at least one backup from within the past 26 hours.

- [ ] **Confirm GPG private key is accessible** on the drill host for decryption.
  ```bash
  gpg --list-secret-keys | grep -i dare
  ```

- [ ] **Confirm rclone is configured** on the drill host.
  ```bash
  rclone listremotes
  rclone lsd b2:dare-backups-offsite/
  ```

- [ ] **Confirm drill VM is available** (clean staging server or a new ephemeral VM). The drill should NOT be run against the production VPS or the live staging environment.

- [ ] **Notify the team** in Slack `#dare-ops`:
  ```
  [RESTORE DRILL] Starting quarterly restore drill at <time UTC>.
  Drill target: <staging VM / ephemeral VM>.
  Operator: @<name>
  Expected duration: 2–4 hours
  ```

- [ ] **Record start time:** ___________

---

## Drill Steps

### Phase 1 — Provision Clean VM

- [ ] Provision a clean VM (Ubuntu 24.04 LTS, minimum 4 vCPU, 8 GB RAM, 50 GB SSD).
- [ ] SSH to the VM and confirm it is a fresh install with no DARE data.
  ```bash
  ls /opt/dare 2>/dev/null && echo "NOT CLEAN" || echo "clean"
  ```
- [ ] Install required dependencies:
  ```bash
  # Docker
  curl -fsSL https://get.docker.com | bash

  # Tools
  apt-get install -y nginx gnupg2 git curl jq python3

  # mc (MinIO client)
  wget -q https://dl.min.io/client/mc/release/linux-amd64/mc -O /usr/local/bin/mc && chmod +x /usr/local/bin/mc

  # rclone
  curl https://rclone.org/install.sh | bash

  # goose (or download binary)
  curl -L https://github.com/pressly/goose/releases/latest/download/goose_linux_x86_64 \
    -o /usr/local/bin/goose && chmod +x /usr/local/bin/goose
  ```
- [ ] Copy configuration from 1Password:
  - [ ] `.env.production` → `/opt/dare/.env.production`
  - [ ] `.env.backup` → `/opt/dare/.env.backup`
  - [ ] rclone config → `~/.config/rclone/rclone.conf`
  - [ ] GPG private key → import with `gpg --import`
- [ ] Clone repository:
  ```bash
  mkdir -p /opt/dare
  git clone git@github.com:your-org/aquify.git /opt/dare
  ```
- [ ] Pull Docker images:
  ```bash
  cd /opt/dare
  docker compose -f docker-compose.production.yml pull
  ```

Record Phase 1 duration: ___________

---

### Phase 2 — Download and Restore Backup

- [ ] Download the latest daily backup from offsite:
  ```bash
  mkdir -p /var/backups/dare
  rclone copy b2:dare-backups-offsite/daily/ /var/backups/dare/ --max-age 26h
  ls -lh /var/backups/dare/
  ```
  Note which timestamp was downloaded: ___________

- [ ] Run the restore script:
  ```bash
  /opt/dare/deploy/scripts/restore.sh <timestamp>
  ```
  Watch for errors. Record any issues encountered: ___________

- [ ] Confirm restore script exits with code 0 (success):
  ```bash
  echo "Exit code: $?"
  ```

Record Phase 2 duration: ___________

---

### Phase 3 — Service Startup

- [ ] Confirm all containers are running:
  ```bash
  docker compose -f /opt/dare/docker-compose.production.yml ps
  ```
  Expected: dare-api, dare-admin, dare-worker, dare-minio, dare-postgres, dare-redis all `Up`.

- [ ] Tail API logs for 60 seconds and check for startup errors:
  ```bash
  docker logs dare-production-dare-api-1 --follow &
  sleep 60
  kill %1
  ```

Record Phase 3 duration: ___________

---

### Phase 4 — Smoke Tests

Run the standard smoke test suite against the drill VM's local ports:

- [ ] Health check:
  ```bash
  curl -sf http://localhost:8080/healthz && echo "PASS" || echo "FAIL"
  ```

- [ ] API session check (requires stub token or test credentials):
  ```bash
  curl -sf -H "Authorization: Bearer stub-admin" \
       -H "X-Org-Id: org-demo" \
       http://localhost:8080/api/v1/auth/session | jq .user_id
  ```
  Expected: a user_id value (not null or error).

- [ ] Member list:
  ```bash
  curl -sf -H "Authorization: Bearer stub-admin" \
       -H "X-Org-Id: org-demo" \
       http://localhost:8080/api/v1/orgs/org-demo/members | jq length
  ```
  Expected: a positive integer matching pre-backup member count.

- [ ] Meets list:
  ```bash
  curl -sf -H "Authorization: Bearer stub-admin" \
       -H "X-Org-Id: org-demo" \
       http://localhost:8080/api/v1/orgs/org-demo/meets | jq length
  ```
  Expected: a positive integer.

- [ ] Invoice list:
  ```bash
  curl -sf -H "Authorization: Bearer stub-admin" \
       -H "X-Org-Id: org-demo" \
       http://localhost:8080/api/v1/orgs/org-demo/invoices | jq length
  ```
  Expected: a positive integer.

- [ ] Import upload endpoint accepts a test file:
  ```bash
  curl -sf -X POST \
       -H "Authorization: Bearer stub-admin" \
       -H "X-Org-Id: org-demo" \
       -F "file=@/opt/dare/server/internal/meetfiles/testdata/fixtures/sample.cl2" \
       http://localhost:8080/api/v1/orgs/org-demo/imports | jq .id
  ```
  Expected: an import job ID.

- [ ] Admin portal loads:
  ```bash
  curl -sf http://localhost:3000 -o /dev/null -w "%{http_code}" && echo ""
  ```
  Expected: 200 or 302.

Record overall smoke test result (PASS / FAIL with notes): ___________

Record Phase 4 duration: ___________

---

### Phase 5 — Row Count Verification

Compare post-restore row counts against the expected baseline (record current production counts before the drill and note them below).

| Table | Expected count | Actual count | Match? |
|---|---|---|---|
| `members.households` | ___ | ___ | [ ] Yes / [ ] No |
| `members.swimmers` | ___ | ___ | [ ] Yes / [ ] No |
| `meets.meets` | ___ | ___ | [ ] Yes / [ ] No |
| `billing.invoices` | ___ | ___ | [ ] Yes / [ ] No |
| `comms.announcements` | ___ | ___ | [ ] Yes / [ ] No |

Any discrepancies: ___________

---

## Pass Criteria

The drill is considered successful if ALL of the following are true:

- [ ] Restore script exited with code 0
- [ ] All smoke test checks returned expected results
- [ ] Row counts match expected baseline within ±1% (small variance from the backup window is acceptable)
- [ ] Admin portal loads
- [ ] API login and member/meet list endpoints return data
- [ ] No ERROR-level log lines related to DB connection or missing data at startup

If any criterion fails, record the failure details below and open a GitHub issue before the next drill.

---

## Results Record

| Field | Value |
|---|---|
| Drill date | |
| Backup timestamp used | |
| Drill VM provider / region | |
| Operator | |
| Total duration | |
| Smoke tests result | PASS / FAIL |
| Row counts verified | PASS / FAIL |
| Issues found | |
| Follow-up actions | |
| Engineering lead sign-off | |
| Sign-off date | |

---

## Post-Drill

- [ ] Destroy the drill VM (do not leave ephemeral VMs running — they incur cost and are a security risk).
  ```bash
  # On your local machine, not the drill VM:
  # Use your VPS provider CLI or dashboard to delete the drill VM
  ```

- [ ] Post results to Slack `#dare-ops`:
  ```
  [RESTORE DRILL COMPLETE] Quarter: <Q1/Q2/Q3/Q4 YYYY>
  Duration: <X min>
  Result: PASS / FAIL
  Issues: <none / brief description>
  Next drill: <approximate date>
  ```

- [ ] If any issues were found:
  - [ ] Open a GitHub issue tagged `dr-drill` with findings.
  - [ ] Assign to the engineering lead for prioritization.
  - [ ] Ensure the issue is resolved before the next drill.

- [ ] Update this checklist file if any steps were unclear or needed modification during the drill.

- [ ] File the completed results table above in the team's shared drive or append it to `docs/ops/drill-history.md`.

---

## Drill History

| Quarter | Date | Operator | Duration | Result | Issues |
|---|---|---|---|---|---|
| | | | | | |
