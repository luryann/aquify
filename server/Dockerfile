# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1: Build the Go binary
# ---------------------------------------------------------------------------

# Go version must satisfy server/go.mod's `go` directive; official images run
# with GOTOOLCHAIN=local, so an older base image fails the build outright.
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Build context is the repository root (see .github/workflows/deploy-staging.yml):
# server/go.mod replaces github.com/dareaquatics/aquify/meetfiles with
# ../internal/meetfiles, so both trees must be laid out as in the repo.
# server/Dockerfile.dockerignore keeps the context to just those two trees.
WORKDIR /src

# Download dependencies first (cached layer when source changes but go.mod does not)
COPY internal/meetfiles ./internal/meetfiles
COPY server/go.mod server/go.sum ./server/
WORKDIR /src/server
RUN go mod download && go mod verify

# Copy source
COPY server/ ./

# Build arguments for embedding version metadata
ARG BUILD_SHA=unknown
ARG BUILD_REF=unknown

# Build a statically linked binary with version info embedded
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w \
      -X main.BuildSHA=${BUILD_SHA} \
      -X main.BuildRef=${BUILD_REF}" \
    -o /dare-api \
    ./cmd/api

# ---------------------------------------------------------------------------
# Stage 2: Minimal runtime image
# ---------------------------------------------------------------------------

FROM gcr.io/distroless/static-debian12

# Import timezone data and CA certs from builder so the binary can make
# outbound TLS calls (Stripe, Postmark, OIDC) and handle time zones correctly.
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /dare-api /dare-api

# Run as the built-in nonroot user (uid 65532) — never root
USER nonroot:nonroot

EXPOSE 8080

# Default entrypoint: run the API server.
# Pass "worker" as CMD to start the background worker process instead.
ENTRYPOINT ["/dare-api"]
