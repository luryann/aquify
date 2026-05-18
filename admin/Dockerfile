# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1: Install dependencies
# ---------------------------------------------------------------------------

FROM node:20-alpine AS deps

RUN apk add --no-cache libc6-compat

WORKDIR /app

# Copy lockfiles and install with clean install (reproducible)
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts

# ---------------------------------------------------------------------------
# Stage 2: Build the Next.js application
# ---------------------------------------------------------------------------

FROM node:20-alpine AS builder

WORKDIR /app

# Bring in node_modules from deps stage
COPY --from=deps /app/node_modules ./node_modules

# Copy application source
COPY . .

# Build-time environment variables
# NEXT_PUBLIC_* values baked in at build time.
# Override these with --build-arg at image build time for each environment.
ARG BUILD_SHA=unknown
ARG NEXT_PUBLIC_APP_ENV=production
ARG NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS=false
# API URL is proxied via Next.js rewrites at runtime — do not bake in a hard URL.
# Set NEXT_PUBLIC_API_URL only if you need to override the rewrite target.

ENV NEXT_PUBLIC_APP_ENV=${NEXT_PUBLIC_APP_ENV}
ENV NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS=${NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS}
ENV NEXT_TELEMETRY_DISABLED=1

# Enable Next.js standalone output (writes a self-contained server to .next/standalone)
# This requires `output: 'standalone'` in next.config.ts.
# If standalone mode is not configured, the runner stage falls back to `next start`.
RUN npm run build

# ---------------------------------------------------------------------------
# Stage 3: Minimal production runner
# ---------------------------------------------------------------------------

FROM node:20-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

# Create a non-root user for the runtime process
RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 --ingroup nodejs nextjs

# Copy the standalone server bundle and static assets.
# If your next.config.ts does NOT set `output: 'standalone'`, replace the
# standalone copy lines with:
#   COPY --from=builder /app/.next ./.next
#   COPY --from=builder /app/node_modules ./node_modules
#   COPY --from=builder /app/package.json ./package.json
# and change CMD to ["npm", "run", "start"].
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public

USER nextjs

EXPOSE 3000

ENV PORT=3000
ENV HOSTNAME=0.0.0.0

# standalone output produces a self-contained `server.js` at the root
CMD ["node", "server.js"]
