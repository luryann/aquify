## STEP34B / STEP34C execution notes

### Scope for this pass
- Remove hardcoded org selection from public auth/registration entry points.
- Make self-registration submit the actual requested role and request metadata.
- Persist the request metadata on the user identity record so admin review operates on real request state.
- Use the stored requested role during approval instead of always defaulting to `parent`.

### Constraints
- Current backend registration flow persists through Keycloak user creation plus user attributes.
- There is no dedicated seasons catalog API yet, so full portal registration wizard replacement may require a second pass.

### Verification targets
- Frontend tenant/org resolution has unit coverage.
- Self-registration API stores request attributes and approval consumes the requested role.
- Admin approval UI exposes request details from live data.
