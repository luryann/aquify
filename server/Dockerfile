# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1: Build the Go binary
# ---------------------------------------------------------------------------

FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Download dependencies first (cached layer when source changes but go.mod does not)
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source
COPY . .

# Build arguments for embedding version metadata
ARG BUILD_SHA=unknown
ARG BUILD_REF=unknown

# Build a statically linked binary with version info embedded
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w \
      -X main.BuildSHA=${BUILD_SHA} \
      -X main.BuildRef=${BUILD_REF}" \
    -o dare-api \
    ./cmd/api

# ---------------------------------------------------------------------------
# Stage 2: Minimal runtime image
# ---------------------------------------------------------------------------

FROM gcr.io/distroless/static-debian12

# Import timezone data and CA certs from builder so the binary can make
# outbound TLS calls (Stripe, Postmark, OIDC) and handle time zones correctly.
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /app/dare-api /dare-api

# Run as the built-in nonroot user (uid 65532) — never root
USER nonroot:nonroot

EXPOSE 8080

# Default entrypoint: run the API server.
# Pass "worker" as CMD to start the background worker process instead.
ENTRYPOINT ["/dare-api"]
