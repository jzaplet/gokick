---
layout: 'page'
uri: '/guides/build-deploy'
position: 3
slug: 'guides-build-deploy'
parent: 'guides'
navTitle: 'Build & Deploy'
title: 'Build & Deploy'
description: 'Single-binary build, Makefile, Docker.'
---

# Build & Deploy


## Single-binary

1. Vite → `public/` (JS, CSS)
2. `public/embed.go` → `//go:embed *`
3. `migrations/embed.go` → embeduje SQL
4. Go build → jedna binárka


## Makefile

```makefile
make install        # go-deps + fe-deps + install-tools
make build          # di + fe-build + go-build
make build-all      # Cross-platform
make dev            # Go binárka (dev)
make fe-dev         # Vite HMR
make serve          # ./bin/app serve
make lint           # go-fmt + fe-lint + go-lint
make test           # vitest + go test (app/ + cmd/)
make arch-check     # go-arch-lint
make go-fix         # go fix ./... (Go 1.26)
make check          # Kompletní CI
make di             # Wire
make migrate-create # NAME=xxx
make migrate-up / down / status
```


## Docker

```dockerfile
FROM alpine:latest
WORKDIR /app
RUN mkdir -p /app/bin /app/db
COPY app /app/bin/app
ENV APP_DB_PATH=/app/db/app.db
ENV APP_HTTP_PORT=3000
EXPOSE 3000
CMD ["/app/bin/app", "serve"]
```

- Container-aware GOMAXPROCS (Go 1.25+)
- Green Tea GC (Go 1.26) – GC overhead -10-40 %
- SQLite volume: `/app/db/`
