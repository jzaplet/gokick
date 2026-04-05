.PHONY: install dev build serve di install-tools go-deps arch-check

# Instalace
install: go-deps install-tools

go-deps:
	go mod download && go mod tidy

install-tools:
	go install github.com/google/wire/cmd/wire@latest
	go install github.com/segmentio/golines@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest
	go install github.com/fe3dback/go-arch-lint@latest

# Vývoj
dev: di
	go build -o bin/app ./cmd/

build: di
	go build -ldflags="-s -w" -o bin/app ./cmd/

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

# Architektura
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
