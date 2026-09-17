# syntax=docker/dockerfile:1.7
# Build stage: compile a fully-static Go binary.
FROM --platform=linux/arm64 golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO disabled; statically link everything so the runtime image needs no libc.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/api ./cmd/api

# Runtime stage: distroless static + nonroot UID 65532.
# Replaces FROM scratch so the API can perform socket(), DNS lookups, and TLS
# handshakes against Postgres and S3-compatible storage without shipping a shell,
# package manager, or dynamic linker. The :nonroot tag drops the need to add an
# explicit USER directive — the container starts as UID 65532 by default.
# Pinned by digest (Plan C.4) — gcr.io/distroless static-debian12:nonroot @ arm64.
# Re-resolve via gcr.io/v2/distroless/static-debian12/manifests/nonroot if the pin breaks.
FROM --platform=linux/arm64 gcr.io/distroless/static-debian12:nonroot@sha256:06c3c14b7fa252d9f4285242d2d665ce3b51f74b02d3c529464f220f2912aecb AS runtime
COPY --from=builder /out/api /api
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# The binary serves /api/v1/health. Docker Compose uses [-healthcheck] to
# probe it without an in-container shell. Entrypoint form (JSON array) is
# required so the distroless runtime can exec /api directly.
ENTRYPOINT ["/api"]
