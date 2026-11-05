# MinIO is built from a pinned source revision because the project no longer publishes
# images (minio/minio is gone from Docker Hub and Quay) and dl.min.io returns 410.
# The upstream OSS repository is archived: this image receives no upstream security
# fixes. Treat it as a known risk and plan a migration to another S3-compatible store
# (see docs/production-readiness-tracker.md, T-29).
#
#   docker compose build minio        # run by `compose up` when the image is missing
#
# Bump MINIO_REV / MC_REV deliberately; both are Go pseudo-versions of the default branch.
FROM golang:1.26.10-alpine AS build
ARG MINIO_REV=v0.0.0-20260212201848-7aac2a2c5b7c
ARG MC_REV=v0.0.0-20251106162529-77f82e18b540
ENV CGO_ENABLED=0 GOBIN=/out
RUN apk add --no-cache git \
 && go install github.com/minio/minio@${MINIO_REV} \
 && go install github.com/minio/mc@${MC_REV}

FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl && adduser -D -u 10001 minio && mkdir /data && chown minio /data
COPY --from=build /out/minio /out/mc /usr/local/bin/
# The compose health check runs `mc ready local`.
ENV MC_HOST_local=http://localhost:9000
USER minio
EXPOSE 9000 9001
ENTRYPOINT ["minio"]
CMD ["server", "/data"]
