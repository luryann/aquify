SHELL := /bin/zsh

.PHONY: dev dev-api dev-admin test lint lint-openapi infra-up infra-down infra-reset db-up db-down db-status

COMPOSE_FILE ?= docker-compose.yml
DOCKER_COMPOSE ?= docker compose
DOCKER_ENV_FILE ?= .env.local
MIGRATIONS_DIR ?= db/migrations
DATABASE_URL_DEFAULT ?= postgres://postgres:postgres@localhost:5432/aquify?sslmode=disable
GOOSE_CMD ?= go run github.com/pressly/goose/v3/cmd/goose@v3.24.1

dev:
	@echo "Starting API (:8080) and admin (:3000)"
	@(cd server && go run ./cmd/api) & \
	API_PID=$$!; \
	(cd admin && npm run dev) & \
	ADMIN_PID=$$!; \
	trap 'kill $$API_PID $$ADMIN_PID 2>/dev/null || true' INT TERM EXIT; \
	wait

dev-api:
	cd server && go run ./cmd/api

dev-admin:
	cd admin && npm run dev

test:
	@echo "Running meetfiles parsing library tests"
	@cd internal/meetfiles && go test ./...
	@echo "Running server unit/integration tests"
	@cd server && go test ./...
	@echo "Running admin production build check"
	@cd admin && npm run build

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
	@echo "Infrastructure is running (Postgres, Redis, MinIO, Mailpit)"

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
