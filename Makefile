.PHONY: install build serve dev di install-tools go-deps hooks setup-github lint format format-check test test-pg arch-check nosqlite-check \
        db-up db-down db-reset db-psql \
        ts-gen ts-check boundary-check errfields-check docpaths-check \
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
# Git-hook runner (commit-msg → commitlint, pre-push → branch-name guard). A
# single Go binary, so it installs alongside the other pinned tools rather than
# pulling in a Node hook manager. `make install` runs `lefthook install` after
# to wire .git/hooks. See lefthook.yml + CONTRIBUTING.md.
LEFTHOOK_VERSION := v1.13.6

# Release version stamped into the binary (-X main.release) and the SPA bundle
# (VITE_SENTRY_RELEASE) — both feed the Sentry release so issues group by
# deployed version. Derived from the latest git tag locally; CI / the Docker
# build override it with the release tag. Falls back to the short commit SHA.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)

# Install — Go deps + tools + frontend deps, then wire the git hooks. `hooks`
# runs last because the commit-msg hook shells out to commitlint, which lives in
# node_modules (installed by fe-deps).
install: go-deps install-tools fe-deps hooks

go-deps:
	go mod download && go mod tidy

install-tools:
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/google/wire/cmd/wire@$(WIRE_VERSION)
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/segmentio/golines@$(GOLINES_VERSION)
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/fe3dback/go-arch-lint@$(GO_ARCH_LINT_VERSION)
	GOTOOLCHAIN=$(TOOL_GOTOOLCHAIN) go install github.com/evilmartians/lefthook@$(LEFTHOOK_VERSION)

# Wire .git/hooks to lefthook. Idempotent — safe to re-run; a fresh clone gets
# its hooks from `make install`. Skips gracefully outside a git checkout (CI
# archive / vendored source) so it can't break a build that has no .git.
hooks:
	@if [ -d .git ]; then lefthook install; else echo "hooks: no .git, skipping"; fi

# One-time GitHub bootstrap for a repo created FROM the gokick template: applies
# the two repo settings the template can't carry (Actions write + create-PR
# permissions, and the branch ruleset) so release-please and branch protection
# work. Idempotent; needs `gh` logged in. See scripts/setup-github.sh + /gk-init.
# Pass args through: `make setup-github ARGS="--yes --reset-version 0.1.0"`.
setup-github:
	./scripts/setup-github.sh $(ARGS)

# Build — frontend first (Vite → public/), then Go (embeds public/). db-up first:
# with APP_DB_DRIVER=postgres in .env the Postgres container comes up on its own.
build: db-up di fe-build
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
	$(MAKE) nosqlite-check
	$(MAKE) format-check
	$(MAKE) ts-check
	$(MAKE) boundary-check
	$(MAKE) errfields-check
	$(MAKE) i18n-check
	$(MAKE) docpaths-check
	$(MAKE) documan-lint

# The adapter switch, compile half: built with -tags nosqlite, neither the app
# nor any test links SQLite (the adapter or the ncruces driver), yet every
# package still compiles and lints clean. The static half is
# app/zz_nosqlite_test.go.
nosqlite-check:
	$(GOLANGCI_LINT) run --build-tags nosqlite ./app/... ./cmd/...
	@linked="$$(go list -tags nosqlite -test -deps ./app/... ./cmd/... | grep -E 'ncruces|gokick/app/infrastructure/sqlite' || true)"; \
	if [ -n "$$linked" ]; then \
		echo "nosqlite-check: a -tags nosqlite build still links SQLite:"; \
		echo "$$linked"; \
		exit 1; \
	fi

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

serve: db-up
	./bin/app serve

# Postgres (APP_DB_DRIVER=postgres) — the db service of docker-compose.yml, built
# from docker/postgres/Dockerfile. It publishes no port: on OrbStack the host
# reaches db.$APP_DOMAIN directly (the APP_DB_*_URL DSNs in .env). db-up is a no-op
# unless .env selects postgres, so a SQLite project never needs Docker; with
# postgres it is idempotent — a running container returns at once, otherwise it is
# built and started and the recipe waits for its healthcheck. The migrations then
# run at application startup, as on SQLite.
# The environment wins over .env, as it does for the app (godotenv never overrides
# a variable that is already set); in .env the value may be quoted or exported.
DB_DRIVER := $(or $(APP_DB_DRIVER),$(shell sed -nE \
	's/^[[:space:]]*(export[[:space:]]+)?APP_DB_DRIVER[[:space:]]*=[[:space:]]*["'\'']?([a-z]*).*/\2/p' \
	.env 2>/dev/null | tail -n 1))

db-up:
ifeq ($(DB_DRIVER),postgres)
	docker compose up -d --wait db
endif

db-down:
	docker compose stop db db-test

# Wipes the Postgres data: stops the project's containers (the app and documan
# too) and deletes its named volumes — pgdata is the only one. The next db-up
# starts from an empty cluster (roles re-created, schema re-migrated at startup).
db-reset:
	docker compose --profile postgres --profile test down --volumes

# psql inside the container, as the schema owner — no port needed.
db-psql:
	docker compose exec db psql -U gokick_owner gokick

# DI
di:
	cd app/infrastructure/di && $(WIRE)

# Go->TS type parity (F-082). Generate the frontend request/response types from
# the annotated Go DTOs (path-first //gkts:<path> <TSName> directives) — mirrors
# `make di` for wire. Run after changing a DTO. `ts-check` (wired into `make
# lint`) fails CI if the committed TS has drifted from the Go source.
ts-gen:
	cd tools/gk && go run . tsgen generate

ts-check:
	cd tools/gk && go run . tsgen check

# Wire-boundary gate (closes the tsgen opt-in gap): every Responder.JSON payload
# and DecodeJSON target must be a named struct with a //gkts: directive, or carry
# a call-site `//gkts:ignore <reason>` (non-SPA endpoints only).
boundary-check:
	cd tools/gk && go run . boundary

# Error-key parity (the static half of follow-up ④): every Go
# ValidationError{Field: "..."} literal must have a home key in some FE
# *Errors type and vice versa (general is the conventional catch-all).
# Escape: //gkerrf:exempt <reason> for fields that never render in a form.
errfields-check:
	cd tools/gk && go run . errfields

# Translation-key parity. Generate app/domain/shared/msgkey AND the
# frontend catalogs (assets/app-ui/I18n/catalog/{en,cs}.ts) from the single
# locale/messages.<lang>.json namespace (en is canonical) — mirrors `make
# ts-gen`. Run after editing a catalog. The check (wired into `make lint`) also
# gates catalog quality (key/param/pluralness parity, valid CLDR forms,
# lowercase-first params), artifact freshness, dead keys (unused in BOTH Go and
# frontend source), Go call-site params, and the supported-language mirrors.
i18n-gen:
	cd tools/gk && go run . i18n generate

i18n-check:
	cd tools/gk && go run . i18n check

# Every repo path and /gk-* skill link the docs and skills cite must resolve —
# the first gate on prose. It checks that pointers land, not that the prose is
# true, so it catches the rot every drift sweep re-found (a squashed migration,
# a renamed skill, a moved file) and nothing subtler.
# Escape: <!-- gkdoc:ignore <path> — <reason> --> for fictional examples.
docpaths-check:
	cd tools/gk && go run . docpaths

# Migrations
# Every migration exists once per dialect under the SAME version (the twin gate
# in app/zz_migrations_test.go enforces it), so this creates the pair with one
# timestamp: fill in migrations/sqlite/<v>_<name>.sql AND migrations/postgres/….
# migrate-up / -down / -status below drive the SQLite file only; on Postgres the
# application migrates at startup (make serve), as the schema owner.
migrate-create:
	@test -n "$(NAME)" || { echo "usage: make migrate-create NAME=add_x_table"; exit 1; }
	@v="$$(date -u +%Y%m%d%H%M%S)"; \
	for d in sqlite postgres; do \
		f="migrations/$$d/$${v}_$(NAME).sql"; \
		printf -- '-- +goose Up\n\n-- +goose Down\n' > "$$f"; \
		echo "created $$f"; \
	done

migrate-up:
	$(GOOSE) -dir migrations/sqlite sqlite3 $(shell grep APP_DB_PATH .env | cut -d= -f2) up

migrate-down:
	$(GOOSE) -dir migrations/sqlite sqlite3 $(shell grep APP_DB_PATH .env | cut -d= -f2) down

migrate-status:
	$(GOOSE) -dir migrations/sqlite sqlite3 $(shell grep APP_DB_PATH .env | cut -d= -f2) status

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

# The Postgres adapter's tests, against the db-test service (a throwaway cluster
# in RAM, reached by its container IP — no published port). Built with -tags
# nosqlite, so not a line of SQLite is compiled in. Set APP_TEST_DB_URL (a
# superuser DSN) to run against another cluster instead of the container. Until
# the Postgres repositories land (phase 4 of the Postgres adapter plan) this covers
# the adapter's own packages; then it becomes the whole suite.
PG_TEST_PKGS := ./app/infrastructure/postgres/...
PG_GO_TEST := APP_DB_DRIVER=postgres go test -tags nosqlite $(PG_TEST_PKGS)

test-pg:
ifdef APP_TEST_DB_URL
	$(PG_GO_TEST)
else
	docker compose up -d --wait db-test
	@ip="$$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' \
		"$$(docker compose ps -q db-test)")"; \
	test -n "$$ip" || { echo "test-pg: cannot resolve the db-test container IP"; exit 1; }; \
	url="postgres://postgres:postgres@$$ip:5432/postgres?sslmode=disable"; \
	echo "APP_TEST_DB_URL=$$url $(PG_GO_TEST)"; \
	APP_TEST_DB_URL="$$url" $(PG_GO_TEST)
endif

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
