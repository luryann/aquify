SHELL := /bin/zsh

.PHONY: dev dev-full dev-api dev-admin test test-p0 lint lint-openapi infra-up infra-down infra-reset db-up db-down db-status cutover-rehearsal cutover-smoke cutover-certify cutover-billing-check provider-validation-evidence parallel-meet-cycle-evidence stability-report-evidence launch-evidence-bundle verify-launch-evidence keycloak-bootstrap keycloak-local-sync keycloak-local-up

COMPOSE_FILE ?= docker-compose.yml
DOCKER_COMPOSE ?= docker compose
DOCKER_ENV_FILE ?= .env.local
MIGRATIONS_DIR ?= db/migrations
DATABASE_URL_DEFAULT ?= postgres://postgres:postgres@localhost:5432/aquify?sslmode=disable
GOOSE_CMD ?= go run github.com/pressly/goose/v3/cmd/goose@v3.24.1
VERIFY_LAUNCH_EVIDENCE_FLAGS ?=

KEYCLOAK_DIR ?= $(shell pwd)/keycloak-26.5.6
KEYCLOAK_REALM_FILE ?= $(shell pwd)/keycloak/realm-aquify-local.json
KEYCLOAK_JAVA_OPTS_APPEND ?= --add-opens java.base/java.lang=ALL-UNNAMED

dev-full:
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
		KEYCLOAK_ADMIN=$${KEYCLOAK_ADMIN:-admin} KEYCLOAK_ADMIN_PASSWORD=$${KEYCLOAK_ADMIN_PASSWORD:-admin} JAVA_OPTS_APPEND="$(KEYCLOAK_JAVA_OPTS_APPEND)" \
		$(KEYCLOAK_DIR)/bin/kc.sh start-dev --http-port=$${KEYCLOAK_PORT:-8180} --import-realm & \
		KC_PID=$$!; \
		echo "Waiting for Keycloak..."; \
		until curl -sf http://127.0.0.1:$${KEYCLOAK_PORT:-8180}/realms/master > /dev/null 2>&1; do sleep 2; done; \
		echo "Keycloak ready"; \
	fi; \
	if ! ./scripts/keycloak-bootstrap.sh; then \
		echo "Keycloak bootstrap failed"; \
		if [ -n "$$KC_PID" ]; then \
			kill $$KC_PID 2>/dev/null || true; \
			wait $$KC_PID 2>/dev/null || true; \
		fi; \
		exit 1; \
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
	@set -a; source $(DOCKER_ENV_FILE); set +a; ./scripts/keycloak-bootstrap.sh
	@echo "Infrastructure is running (Keycloak, Postgres, Redis, MinIO, Mailpit)"

keycloak-local-sync:
	@mkdir -p $(KEYCLOAK_DIR)/data/import
	@cp $(KEYCLOAK_REALM_FILE) $(KEYCLOAK_DIR)/data/import/realm-aquify-local.json
	@echo "Copied realm config to $(KEYCLOAK_DIR)/data/import/realm-aquify-local.json"

keycloak-local-up:
	@mkdir -p $(KEYCLOAK_DIR)/data/import
	@if [ ! -f "$(KEYCLOAK_DIR)/data/h2/keycloakdb.mv.db" ]; then \
		echo "First-time Keycloak setup — importing realm from $(KEYCLOAK_REALM_FILE)"; \
		cp $(KEYCLOAK_REALM_FILE) $(KEYCLOAK_DIR)/data/import/realm-aquify-local.json; \
	else \
		echo "Keycloak data already exists — skipping realm import (MFA/passwords preserved)"; \
		rm -f $(KEYCLOAK_DIR)/data/import/realm-aquify-local.json; \
	fi
	@echo "Starting local Keycloak from $(KEYCLOAK_DIR) on http://localhost:$${KEYCLOAK_PORT:-8180}"
	@KEYCLOAK_ADMIN=$${KEYCLOAK_ADMIN:-admin} KEYCLOAK_ADMIN_PASSWORD=$${KEYCLOAK_ADMIN_PASSWORD:-admin} JAVA_OPTS_APPEND="$(KEYCLOAK_JAVA_OPTS_APPEND)" \
	$(KEYCLOAK_DIR)/bin/kc.sh start-dev --http-port=$${KEYCLOAK_PORT:-8180} --import-realm

keycloak-bootstrap:
	@set -a; [ -f $(DOCKER_ENV_FILE) ] && source $(DOCKER_ENV_FILE) || true; set +a; ./scripts/keycloak-bootstrap.sh

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
