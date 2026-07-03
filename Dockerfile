# syntax=docker/dockerfile:1
# Conway — single static Go binary that serves the SPA (app/) + the game/auth API.
# Build context is the repo root.

FROM golang:1.26 AS build
WORKDIR /src
# deps are vendored (server/vendor) so the build is hermetic — no module proxy
# needed at build time (the corp TLS proxy can block it).
COPY server/ ./server/
WORKDIR /src/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/conway .
# stage the SPA and normalize perms so the non-root runtime user can always read
# every file (Go's FileServer 403s on an unreadable file — e.g. a stray 0600).
COPY app/ /out/app
RUN chmod -R a+rX /out/app

# Minimal runtime from Docker Hub (gcr.io is blocked by the corp TLS proxy on
# the build host). The Go binary is fully static (CGO disabled), so Alpine — or
# even scratch — is enough; it makes no outbound TLS, so no CA bundle is needed.
# Runs as uid 65532 to match the pod securityContext (runAsNonRoot/fsGroup).
FROM alpine:3.20
WORKDIR /
COPY --from=build /out/conway /conway
COPY --from=build /out/app /app
ENV CONWAY_ADDR=:8741 \
    CONWAY_APP_DIR=/app \
    CONWAY_STORE=/data/store.json
EXPOSE 8741
USER 65532:65532
ENTRYPOINT ["/conway"]
