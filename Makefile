.PHONY: install build serve dev di install-tools go-deps lint format format-check test arch-check \
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
	go install github.com/google/wire/cmd/wire@latest
	go install github.com/segmentio/golines@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest
	go install github.com/fe3dback/go-arch-lint@latest

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
	$(GOLANGCI_LINT) run ./app/... ./cmd/...
	$(MAKE) arch-check
	$(MAKE) format-check
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

# Quality
test:
	yarn test
	go test ./app/... ./cmd/... 2>&1 | grep -v '\[no test files\]'

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
