# syntax=docker/dockerfile:1
#
# Production image for the gokick boilerplate.
# Three stages: Vite SPA build → Go binary build (embeds the SPA) →
# minimal Alpine runtime that just runs `serve`.
#
# wire_gen.go is committed (no Wire run inside the image).
# CGO_ENABLED=0 (pure-Go SQLite) → fully static binary.
#
# Production usage: mount a volume at /data for the SQLite file.
#   docker run -p 3000:3000 -v /host/data:/data -e APP_JWT_SECRET=… gokick

###############################################################################
# Stage 1: Build the Vue 3 SPA with Vite + Tailwind.
###############################################################################
FROM node:24-alpine AS frontend
WORKDIR /build

RUN corepack enable

COPY package.json yarn.lock .yarnrc.yml ./
RUN yarn install --immutable

COPY assets ./assets
COPY index.html vite.config.ts tsconfig.json env.d.ts ./
COPY public/embed.go public/favicon.ico ./public/

# Release version → baked into the SPA bundle as the Sentry release. Empty when
# not passed (e.g. `make docker-build` without a tag), which Sentry treats as
# no release. (ARG must be re-declared per stage — a pre-FROM ARG doesn't reach here.)
ARG VERSION=""
ENV VITE_SENTRY_RELEASE=${VERSION}
RUN yarn build


###############################################################################
# Stage 2: Compile the Go backend with the built SPA embedded.
###############################################################################
FROM golang:1.26-alpine AS backend
WORKDIR /build

# Release version → stamped into the binary as main.release (the Sentry
# release). Re-declared here because ARGs don't cross stages.
ARG VERSION=""

COPY go.mod go.sum ./
RUN go mod download

COPY app ./app
COPY cmd ./cmd
COPY migrations ./migrations
COPY --from=frontend /build/public ./public

ENV CGO_ENABLED=0 GOOS=linux
RUN go build \
    -trimpath \
    -ldflags="-s -w -X main.release=${VERSION}" \
    -o /out/app \
    ./cmd


###############################################################################
# Stage 3: Minimal Alpine runtime — small image, has wget for healthcheck,
# nonroot user, persistent /data volume for the SQLite database.
###############################################################################
FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 1000 gokick \
    && adduser  -S -D -u 1000 -G gokick -h /gokick gokick \
    && mkdir -p /data \
    && chown -R gokick:gokick /data /gokick

WORKDIR /gokick
COPY --from=backend --chown=gokick:gokick /out/app /gokick/app

USER gokick:gokick

ENV APP_HTTP_PORT=3000 \
    APP_DB_PATH=/data/app.db \
    APP_COOKIE_SECURE=true

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:3000/health || exit 1

ENTRYPOINT ["/gokick/app"]
CMD ["serve"]
