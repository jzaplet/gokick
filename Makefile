.PHONY: install build serve dev di install-tools go-deps lint format test arch-check fe-deps fe-dev fe-build fe-clean

# Instalace
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

# Format — frontend (ESLint Stylistic) + backend (golines)
format:
	yarn format
	golines -w .

# Lint — frontend (ESLint strict) + backend (golangci-lint + arch rules)
lint:
	yarn lint
	yarn type-check
	golangci-lint run ./app/... ./cmd/...
	go-arch-lint check

# Vývoj
dev: di
	go build -o bin/app ./cmd/

serve:
	./bin/app serve

# DI
di:
	cd app/infrastructure/di && wire

# Migrace
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

# Kvalita
test:
	yarn test
	go test ./app/... ./cmd/... 2>&1 | grep -v '\[no test files\]'

arch-check:
	go-arch-lint check

# Documan
documan:
	docker compose build --progress=plain documan && docker compose up -d documan

documan-import:
	docker compose exec -t documan /documan/bin/documan import

documan-lint:
	docker compose exec -t documan /documan/bin/documan lint

documan-fix:
	docker compose exec -t documan /documan/bin/documan fix

documan-vectorize:
	docker compose exec -t documan /documan/bin/documan vectorize
