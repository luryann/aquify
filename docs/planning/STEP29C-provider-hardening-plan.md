# STEP 29C Provider Hardening Plan

## Goal

Finish the production provider work for payments and outbound email so the app no longer depends on stub runtime integrations for Stripe and can accurately track Postmark delivery/suppression state.

## Scope For This Execution Pass

- Wire a real Stripe SDK-backed client behind the existing `server/internal/stripe` interface.
- Update server runtime wiring so Stripe uses the real client when production credentials are configured and only falls back to the stub in development/test scenarios.
- Expand Postmark support beyond outbound sends by handling provider webhooks for bounce/delivery/spam complaint updates and syncing suppression state.
- Add tests covering the new runtime/provider code paths that are executable locally.
- Update project memory (`CLAUDE.md`) with any new runtime/env requirements.

## Explicit Non-Goals

- Live staging verification against Stripe/Postmark production credentials.
- Final operator execution against real provider dashboards.

Those remain real-environment validation tasks and can only be completed once staging credentials and external access are available.

## Notes

- The repo already has a real Postmark HTTP send client, but no inbound webhook handling.
- The repo currently defaults Stripe to `NewStubClient()` even when a real secret key is present.
- Step 29C in `TODO.md` should only be checked off when code and real-environment verification are both complete.
