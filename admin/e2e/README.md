# Browser E2E tests

Run from `admin/` with Node.js, Go, PostgreSQL client tools (`psql`), and Docker available:

```sh
npm ci
npm run test:e2e:install
npm run test:e2e
```

The harness starts PostgreSQL through the root Compose file, creates and resets
`aquify_e2e`, applies migrations, then starts the API and admin app. API startup
allows five minutes for database setup and a cold Go build; admin startup allows
three minutes. CI downloads Go dependencies and warms the API build cache first.

To use an existing test PostgreSQL server and avoid occupied development ports:

```sh
PLAYWRIGHT_E2E_SKIP_DOCKER=true \
DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/aquify_e2e?sslmode=disable' \
PLAYWRIGHT_API_PORT=18080 \
PLAYWRIGHT_ADMIN_PORT=13000 \
npm run test:e2e
```

`DATABASE_URL` must name a disposable test database: setup resets its contents.
The default API and admin ports are 8080 and 3000. The dedicated API metrics
listener is disabled during E2E runs. Admin browser fixtures use MFA-authenticated
dev sessions when testing privileged screens.
