SHELL := /bin/zsh

.PHONY: dev dev-full dev-api dev-admin test test-p0 lint lint-openapi infra-up infra-down infra-reset db-up db-down db-status cutover-rehearsal cutover-smoke cutover-certify cutover-billing-check provider-validation-evidence parallel-meet-cycle-evidence stability-report-evidence launch-evidence-bundle verify-launch-evidence keycloak-dist keycloak-local-up keycloak-local-reset

COMPOSE_FILE ?= docker-compose.yml
DOCKER_COMPOSE ?= docker compose
DOCKER_ENV_FILE ?= .env.local
MIGRATIONS_DIR ?= db/migrations
DATABASE_URL_DEFAULT ?= postgres://postgres:postgres@localhost:5432/aquify?sslmode=disable
GOOSE_CMD ?= go run github.com/pressly/goose/v3/cmd/goose@v3.24.1
VERIFY_LAUNCH_EVIDENCE_FLAGS ?=

# Keycloak: all configuration lives in deploy/keycloak/ (realm import + login
# theme). The server distribution is auto-downloaded into .tools/ (gitignored).
# On Windows, use: powershell -ExecutionPolicy Bypass -File scripts/start-keycloak-local.ps1
KEYCLOAK_VERSION ?= 26.5.6
KEYCLOAK_DIR ?= $(shell pwd)/.tools/keycloak-$(KEYCLOAK_VERSION)
KEYCLOAK_REALM_FILE ?= $(shell pwd)/deploy/keycloak/realm-aquify-local.json
KEYCLOAK_THEME_DIR ?= $(shell pwd)/deploy/keycloak/themes/aquify
KEYCLOAK_JAVA_OPTS_APPEND ?= --add-opens java.base/java.lang=ALL-UNNAMED

dev-full: keycloak-dist
	@echo "Starting Keycloak (:8180), API (:8080), and admin (:3000)"
	@mkdir -p $(KEYCLOAK_DIR)/data/import
	@if [ ! -f "$(KEYCLOAK_DIR)/data/h2/keycloakdb.mv.db" ]; then \
		echo "First-time Keycloak setup — importing realm from $(KEYCLOAK_REALM_FILE)"; \
		cp $(KEYCLOAK_REALM_FILE) $(KEYCLOAK_DIR)/data/import/realm-aquify-local.json; \
	else \
		echo "Keycloak data already exists — skipping realm import (passwords preserved)"; \
		rm -f $(KEYCLOAK_DIR)/data/import/realm-aquify-local.json; \
	fi
	@set -a; [ -f $(DOCKER_ENV_FILE) ] && source $(DOCKER_ENV_FILE) || true; set +a; \
	KC_PID=""; \
	if curl -sf http://127.0.0.1:$${KEYCLOAK_PORT:-8180}/realms/master > /dev/null 2>&1; then \
		echo "Keycloak already running on :$${KEYCLOAK_PORT:-8180} — reusing existing instance"; \
		echo "Keycloak ready"; \
	else \
		KC_BOOTSTRAP_ADMIN_USERNAME=$${KEYCLOAK_ADMIN:-admin} KC_BOOTSTRAP_ADMIN_PASSWORD=$${KEYCLOAK_ADMIN_PASSWORD:-admin} JAVA_OPTS_APPEND="$(KEYCLOAK_JAVA_OPTS_APPEND)" \
		$(KEYCLOAK_DIR)/bin/kc.sh start-dev --http-port=$${KEYCLOAK_PORT:-8180} --import-realm & \
		KC_PID=$$!; \
		echo "Waiting for Keycloak..."; \
		until curl -sf http://127.0.0.1:$${KEYCLOAK_PORT:-8180}/realms/master > /dev/null 2>&1; do sleep 2; done; \
		echo "Keycloak ready"; \
	fi; \
	(cd server && go run ./cmd/api) & \
	API_PID=$$!; \
	echo "Waiting for API..."; \
	until curl -sf http://127.0.0.1:8080/api/v1/health/live > /dev/null 2>&1; do \
		if ! kill -0 $$API_PID 2>/dev/null; then echo "API failed to start"; wait $$API_PID; exit 1; fi; \
		sleep 1; \
	done; \
	echo "API ready"; \
	(cd admin && npm run dev) & \
	ADMIN_PID=$$!; \
	trap 'if [ -n "$$KC_PID" ]; then kill $$KC_PID 2>/dev/null || true; fi; kill $$API_PID $$ADMIN_PID 2>/dev/null || true' INT TERM EXIT; \
	wait

dev:
	@echo "Starting API (:8080) and admin (:3000)"
	@[ -f .env.local ] && set -a && source .env.local && set +a; \
	(cd server && go run ./cmd/api) & \
	API_PID=$$!; \
	echo "Waiting for API..."; \
	until curl -sf http://127.0.0.1:8080/api/v1/health/live > /dev/null 2>&1; do \
		if ! kill -0 $$API_PID 2>/dev/null; then echo "API failed to start"; wait $$API_PID; exit 1; fi; \
		sleep 1; \
	done; \
	echo "API ready"; \
	(cd admin && npm run dev) & \
	ADMIN_PID=$$!; \
	trap 'kill $$API_PID $$ADMIN_PID 2>/dev/null || true' INT TERM EXIT; \
	wait

dev-api:
	[ -f .env.local ] && set -a && source .env.local && set +a; cd server && go run ./cmd/api

dev-admin:
	cd admin && npm run dev

test:
	@echo "Running meetfiles parsing library tests"
	@cd internal/meetfiles && go test ./...
	@echo "Running server unit/integration tests"
	@cd server && go test ./...
	@echo "Running admin production build check"
	@cd admin && npm run build

test-p0:
	@echo "Running P0 server happy-path/regression tests"
	@cd server && go test ./internal/app -run 'TestP0'

lint: lint-openapi
	@echo "Running meetfiles static analysis"
	@cd internal/meetfiles && go vet ./...
	@echo "Running server static analysis"
	@cd server && go vet ./...
	@echo "Running admin lint"
	@cd admin && npm run lint

lint-openapi:
	@echo "Validating OpenAPI spec"
	@if command -v spectral >/dev/null 2>&1; then \
		spectral lint docs/api/openapi.yaml --ruleset docs/api/.spectral.yaml; \
	else \
		echo "spectral not installed — skipping OpenAPI lint (install: npm install -g @stoplight/spectral-cli)"; \
	fi

infra-up:
	@$(DOCKER_COMPOSE) --env-file $(DOCKER_ENV_FILE) -f $(COMPOSE_FILE) up -d
	@set -a; source $(DOCKER_ENV_FILE); set +a; ./scripts/wait-for-db.sh
	@echo "Infrastructure is running (Keycloak, Postgres, Redis, MinIO, Mailpit)"
	@echo "Keycloak imports deploy/keycloak/realm-aquify-local.json on first start (admin console: http://localhost:8180/admin)"

# Download the Keycloak distribution into .tools/ (if missing) and sync the
# login theme from deploy/keycloak/themes/aquify.
keycloak-dist:
	@if [ ! -x "$(KEYCLOAK_DIR)/bin/kc.sh" ]; then \
		echo "Downloading Keycloak $(KEYCLOAK_VERSION) into .tools/ (one-time) ..."; \
		mkdir -p .tools; \
		curl -fL -o .tools/keycloak-$(KEYCLOAK_VERSION).zip https://github.com/keycloak/keycloak/releases/download/$(KEYCLOAK_VERSION)/keycloak-$(KEYCLOAK_VERSION).zip; \
		unzip -q .tools/keycloak-$(KEYCLOAK_VERSION).zip -d .tools; \
		rm -f .tools/keycloak-$(KEYCLOAK_VERSION).zip; \
	fi
	@rm -rf $(KEYCLOAK_DIR)/themes/aquify
	@cp -R $(KEYCLOAK_THEME_DIR) $(KEYCLOAK_DIR)/themes/aquify
	@echo "Keycloak dist ready at $(KEYCLOAK_DIR) (theme synced from deploy/keycloak)"

keycloak-local-up: keycloak-dist
	@mkdir -p $(KEYCLOAK_DIR)/data/import
	@if [ ! -f "$(KEYCLOAK_DIR)/data/h2/keycloakdb.mv.db" ]; then \
		echo "First-time Keycloak setup — importing realm from $(KEYCLOAK_REALM_FILE)"; \
		cp $(KEYCLOAK_REALM_FILE) $(KEYCLOAK_DIR)/data/import/realm-aquify-local.json; \
	else \
		echo "Keycloak data already exists — skipping realm import (MFA/passwords preserved)"; \
		rm -f $(KEYCLOAK_DIR)/data/import/realm-aquify-local.json; \
	fi
	@echo "Starting local Keycloak from $(KEYCLOAK_DIR) on http://localhost:$${KEYCLOAK_PORT:-8180}"
	@KC_BOOTSTRAP_ADMIN_USERNAME=$${KEYCLOAK_ADMIN:-admin} KC_BOOTSTRAP_ADMIN_PASSWORD=$${KEYCLOAK_ADMIN_PASSWORD:-admin} JAVA_OPTS_APPEND="$(KEYCLOAK_JAVA_OPTS_APPEND)" \
	$(KEYCLOAK_DIR)/bin/kc.sh start-dev --http-port=$${KEYCLOAK_PORT:-8180} --import-realm

# Wipe local Keycloak data (users, sessions, realm) so the next start
# re-imports deploy/keycloak/realm-aquify-local.json from scratch.
keycloak-local-reset:
	@rm -rf $(KEYCLOAK_DIR)/data
	@echo "Keycloak data wiped — next 'make keycloak-local-up' re-imports the realm"

infra-down:
	@$(DOCKER_COMPOSE) --env-file $(DOCKER_ENV_FILE) -f $(COMPOSE_FILE) down

infra-reset:
	@$(DOCKER_COMPOSE) --env-file $(DOCKER_ENV_FILE) -f $(COMPOSE_FILE) down -v --remove-orphans

db-up:
	@set -a; \
	[ -f $(DOCKER_ENV_FILE) ] && source $(DOCKER_ENV_FILE) || true; \
	[ -f .env ] && source .env || true; \
	set +a; \
	DB_URL="$${DATABASE_URL:-$(DATABASE_URL_DEFAULT)}"; \
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) postgres "$$DB_URL" up

db-down:
	@set -a; \
	[ -f $(DOCKER_ENV_FILE) ] && source $(DOCKER_ENV_FILE) || true; \
	[ -f .env ] && source .env || true; \
	set +a; \
	DB_URL="$${DATABASE_URL:-$(DATABASE_URL_DEFAULT)}"; \
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) postgres "$$DB_URL" down

db-status:
	@set -a; \
	[ -f $(DOCKER_ENV_FILE) ] && source $(DOCKER_ENV_FILE) || true; \
	[ -f .env ] && source .env || true; \
	set +a; \
	DB_URL="$${DATABASE_URL:-$(DATABASE_URL_DEFAULT)}"; \
	$(GOOSE_CMD) -dir $(MIGRATIONS_DIR) postgres "$$DB_URL" status

cutover-rehearsal:
	@./scripts/cutover-rehearsal.sh

cutover-smoke:
	@./scripts/cutover-smoke.sh

cutover-certify:
	@./scripts/cutover-certification.sh

cutover-billing-check:
	@./scripts/cutover-billing-check.sh

provider-validation-evidence:
	@bash ./scripts/provider-validation-evidence.sh

parallel-meet-cycle-evidence:
	@bash ./scripts/parallel-meet-cycle-evidence.sh

stability-report-evidence:
	@bash ./scripts/stability-report-evidence.sh

launch-evidence-bundle:
	@bash ./scripts/launch-evidence-bundle.sh

verify-launch-evidence:
	@cd server && go run ./cmd/evidencecheck -dir "$${EVIDENCE_DIR:?EVIDENCE_DIR is required}" $(VERIFY_LAUNCH_EVIDENCE_FLAGS)
