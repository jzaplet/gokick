.PHONY: install build serve dev di install-tools go-deps lint format format-check test arch-check \
        e2e e2e-crash-recovery e2e-at-least-once e2e-sigterm-drain e2e-terminal-failure \
        fe-deps fe-dev fe-build fe-clean \
        migrate-create migrate-up migrate-down migrate-status \
        docker-build \
        documan documan-import documan-lint documan-fix documan-vectorize

# Tools installed via `go install` land in $GOPATH/bin. We resolve them by
# absolute path so recipes work even when the user hasn't added that dir to
# their shell PATH (and to sidestep GNU Make 3.81's broken `export PATH`,
# which Apple still ships on macOS).
GOBIN_DIR := $(shell go env GOPATH)/bin
WIRE := $(GOBIN_DIR)/wire
GOLINES := $(GOBIN_DIR)/golines
GOLANGCI_LINT := $(GOBIN_DIR)/golangci-lint
GOOSE := $(GOBIN_DIR)/goose
GO_ARCH_LINT := $(GOBIN_DIR)/go-arch-lint

# Pin the Go toolchain that BUILDS our dev tools to the project's go.mod version.
# `go install pkg@v` keys off the TOOL's go.mod, not ours, so on a machine whose
# PATH `go` is older than go.mod (multiple Go versions on one box is common) the
# tool gets built with that older toolchain — and golangci-lint then refuses to
# run ("the Go language version go1.X used to build golangci-lint is lower than
# the targeted go1.Y"). golines / go-arch-lint / wire share the same latent
# parse/type-check coupling. GOTOOLCHAIN=go<gomod>+auto floors the build
# toolchain at our go.mod version (Go fetches it on demand, cached), upgrading
# only if a tool itself needs newer. Derived from go.mod, so every gokick-based
# project pins to its own version automatically.
GO_VERSION := $(shell awk '/^go /{print $$2; exit}' go.mod)
TOOL_GOTOOLCHAIN := go$(GO_VERSION)+auto

# Tool versions are pinned (not @latest) so every machine and CI install the same
# linter set — @latest means two devs a month apart get different golangci-lint
# findings (and golangci retires linters across minors). Bumping go.mod's `go`
# may require bumping GOLANGCI_LINT_VERSION to a release built with that Go.
WIRE_VERSION := v0.7.0
GOLINES_VERSION := v0.13.0
GOLANGCI_LINT_VERSION := v2.12.2
GOOSE_VERSION := v3.27.1
GO_ARCH_LINT_VERSION := v1.15.0

# Release version stamped into the binary (-X main.release) and the SPA bundle
# (VITE_SENTRY_RELEASE) — both feed the Sentry release so issues group by
# deployed version. Derived from the latest git tag locally; CI / the Docker
# build override it with the release tag. Falls back to the short commit SHA.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)

# Install
install: go-deps install-tools fe-deps

go-deps:
	go mod download && go mod tidy

install-tools:
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/google/wire/cmd/wire@$(WIRE_VERSION)
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/segmentio/golines@$(GOLINES_VERSION)
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/fe3dback/go-arch-lint@$(GO_ARCH_LINT_VERSION)

# Build — frontend first (Vite → public/), then Go (embeds public/)
build: di fe-build
	go build -ldflags="-s -w -X main.release=$(VERSION)" -o bin/app ./cmd/

# Format — frontend (ESLint Stylistic) + backend (golines) + docs
format:
	yarn format
	$(GOLINES) -w .
	$(MAKE) documan-fix

# Lint — frontend (ESLint strict) + backend (golangci-lint + arch rules +
# golines format check) + docs
lint:
	yarn lint
	yarn type-check
	yarn knip
	$(GOLANGCI_LINT) run ./app/... ./cmd/...
	$(MAKE) arch-check
	$(MAKE) format-check
	$(MAKE) ts-check
	$(MAKE) documan-lint

# Fail if any Go file is not golines-formatted. golines is not covered by
# golangci-lint, so without this gate `make format` drift slips in unnoticed
# (it runs only via `make format`, never in CI otherwise). Fix with `make format`.
format-check:
	@unformatted="$$($(GOLINES) -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "golines: the following files are not formatted (run 'make format'):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

# Development
dev: di
	go build -o bin/app ./cmd/

serve:
	./bin/app serve

# DI
di:
	cd app/infrastructure/di && $(WIRE)

# Go->TS type parity (F-082). Generate the frontend request/response types from
# the annotated Go DTOs (//gkts:<Name> <path> directives) — mirrors `make di` for
# wire. Run after changing a DTO. `ts-check` (wired into `make lint`) fails CI if
# the committed TS has drifted from the Go source.
ts-gen:
	cd tools/gk && go run . tsgen generate

ts-check:
	cd tools/gk && go run . tsgen check

# Migrations
migrate-create:
	$(GOOSE) -dir migrations create $(NAME) sql

migrate-up:
	$(GOOSE) -dir migrations sqlite3 $(shell grep APP_DB_PATH .env | cut -d= -f2) up

migrate-down:
	$(GOOSE) -dir migrations sqlite3 $(shell grep APP_DB_PATH .env | cut -d= -f2) down

migrate-status:
	$(GOOSE) -dir migrations sqlite3 $(shell grep APP_DB_PATH .env | cut -d= -f2) status

# Frontend
fe-deps:
	yarn install

fe-dev:
	yarn dev

fe-build:
	VITE_SENTRY_RELEASE=$(VERSION) yarn build

fe-clean:
	rm -rf public/assets public/index.html

# Quality — app + cmd, FE vitest, and the dev-tooling module (tsgen golden
# tests pin the generator's exact emission; regenerate deliberately with
# `cd tools/gk && go test ./tsgen -update`).
# No output filtering here on purpose: piping go test through grep makes the
# recipe's exit status grep's, and grep exits 0 whenever FAIL lines pass the
# filter — test failures could never fail the target.
test:
	yarn test
	go test ./app/... ./cmd/...
	cd tools/gk && go test ./...

# Local durable-run E2E — process-lifecycle guarantees an in-process test can't reach
# (kill -9 / SIGTERM + persistent SQLite). Each builds bin/app and spawns real serve
# processes; needs jq (at-least-once also sqlite3). NOT part of `test` — run on demand.
# See tests/e2e/README.md.
e2e: e2e-crash-recovery e2e-at-least-once e2e-sigterm-drain e2e-terminal-failure

e2e-crash-recovery:      ## kill -9 mid-run → cold restart resumes from checkpoint
	./tests/e2e/run_crash_recovery.sh
e2e-at-least-once:       ## crash after a side-effect, before complete → effect re-fires
	./tests/e2e/at_least_once.sh
e2e-sigterm-drain:       ## graceful stop abandons cleanly (attempts=0) → reclaim + resume
	./tests/e2e/sigterm_drain.sh
e2e-terminal-failure:    ## a failed run fires the Sentry terminal path (local half only)
	./tests/e2e/terminal_failure.sh

arch-check:
	$(GO_ARCH_LINT) check

# Production image — multi-stage Dockerfile builds Vite SPA, Go binary, and
# a minimal Alpine runtime. Self-contained (no `make build` prerequisite).
docker-build:
	docker build --build-arg VERSION=$(VERSION) -f docker/production/Dockerfile -t gokick:latest .

# Documan
# Each target ensures the container is up (docker compose up -d is idempotent),
# then execs the documan CLI inside it. First invocation builds the image and
# runs the lint as part of the build (per docker/documan/Dockerfile).
# The served docs UI is at https://docs.gokick.local (OrbStack domain via the
# dev.orbstack.* labels in docker-compose.yml — no published host port, so it
# never collides with another project's documan). `make documan` brings it up.
#
# In CI / containerless environments set SKIP_DOCUMAN=1 to make these targets
# no-ops (e.g. `SKIP_DOCUMAN=1 make lint`). Doc validation in CI is handled by
# the dedicated `.github/workflows/documan.yml` workflow which builds the
# Documan Dockerfile directly — no docker compose needed.
documan:
	docker compose --progress=plain build documan && docker compose up -d documan

documan-import:
ifdef SKIP_DOCUMAN
	@echo "documan-import: skipped (SKIP_DOCUMAN=$(SKIP_DOCUMAN))"
else
	@docker compose up -d documan >/dev/null
	docker compose exec -t documan /documan/bin/documan import
endif

documan-lint:
ifdef SKIP_DOCUMAN
	@echo "documan-lint: skipped (SKIP_DOCUMAN=$(SKIP_DOCUMAN))"
else
	@docker compose up -d documan >/dev/null
	docker compose exec -t documan /documan/bin/documan lint
endif

documan-fix:
ifdef SKIP_DOCUMAN
	@echo "documan-fix: skipped (SKIP_DOCUMAN=$(SKIP_DOCUMAN))"
else
	@docker compose up -d documan >/dev/null
	docker compose exec -t documan /documan/bin/documan fix
endif

documan-vectorize:
ifdef SKIP_DOCUMAN
	@echo "documan-vectorize: skipped (SKIP_DOCUMAN=$(SKIP_DOCUMAN))"
else
	@docker compose up -d documan >/dev/null
	docker compose exec -t documan /documan/bin/documan vectorize
endif
