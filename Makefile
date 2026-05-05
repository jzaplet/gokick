.PHONY: install build serve dev di install-tools go-deps lint format test arch-check \
        fe-deps fe-dev fe-build fe-clean \
        migrate-create migrate-up migrate-down migrate-status \
        docker-build \
        documan documan-import documan-lint documan-fix documan-vectorize

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
	go build -ldflags="-s -w" -o bin/app ./cmd/

# Format — frontend (ESLint Stylistic) + backend (golines) + docs
format:
	yarn format
	golines -w .
	$(MAKE) documan-fix

# Lint — frontend (ESLint strict) + backend (golangci-lint + arch rules) + docs
lint:
	yarn lint
	yarn type-check
	golangci-lint run ./app/... ./cmd/...
	$(MAKE) arch-check
	$(MAKE) documan-lint

# Development
dev: di
	go build -o bin/app ./cmd/

serve:
	./bin/app serve

# DI
di:
	cd app/infrastructure/di && wire

# Migrations
migrate-create:
	goose -dir migrations create $(NAME) sql

migrate-up:
	goose -dir migrations sqlite3 $(shell grep APP_DB_PATH .env | cut -d= -f2) up

migrate-down:
	goose -dir migrations sqlite3 $(shell grep APP_DB_PATH .env | cut -d= -f2) down

migrate-status:
	goose -dir migrations sqlite3 $(shell grep APP_DB_PATH .env | cut -d= -f2) status

# Frontend
fe-deps:
	yarn install

fe-dev:
	yarn dev

fe-build:
	yarn build

fe-clean:
	rm -rf public/assets public/index.html

# Quality
test:
	yarn test
	go test ./app/... ./cmd/... 2>&1 | grep -v '\[no test files\]'

arch-check:
	go-arch-lint check

# Production image — multi-stage Dockerfile builds Vite SPA, Go binary, and
# a minimal Alpine runtime. Self-contained (no `make build` prerequisite).
docker-build:
	docker build -f docker/production/Dockerfile -t gokick:latest .

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
