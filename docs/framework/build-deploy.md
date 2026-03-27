---
layout: 'page'
uri: '/framework/build-deploy'
position: 10
slug: 'framework-build-deploy'
parent: 'framework'
navTitle: 'Build & Deploy'
title: 'Build & Deploy'
description: 'Single-binary build, Makefile, Docker deployment.'
---

# Build & Deploy


## Single-binary princip

1. **Vite** zkompiluje Vue SPA do `public/`
2. `public/embed.go` – `//go:embed *` zabalí assets do binárky
3. `migrations/embed.go` embeduje SQL migrace
4. **Go build** vytvoří jednu spustitelnou binárku


## Makefile

```makefile
# Instalace
make install          # go-deps + fe-deps + install-tools
make install-tools    # wire, golines, golangci-lint, goose, go-arch-lint

# Vývoj
make dev              # Go binárka bez optimalizací
make fe-dev           # Vite dev server s HMR
make serve            # ./bin/filmshes serve

# Build
make build            # di + fe-build + go-build (produkční)
make build-all        # Cross-platform (linux/darwin/windows)

# Kvalita
make lint             # go-fmt + fe-lint + go-lint
make test             # go test ./...
make arch-check       # go-arch-lint dependency pravidla
make go-fix           # go fix ./... – automatická modernizace kódu (Go 1.26)
make check            # Kompletní CI pipeline

# DI a migrace
make di               # Wire generování
make migrate-create   # Nová migrace: NAME=xxx
make migrate-up       # Aplikuj migrace
make migrate-down     # Rollback
```


## Docker

### Dockerfile

```dockerfile
FROM alpine:latest
WORKDIR /filmshes
RUN mkdir -p /filmshes/bin /filmshes/db
COPY filmshes /filmshes/bin/filmshes
ENV FILMSHES_DB_PATH=/filmshes/db/filmshes.db
ENV FILMSHES_HTTP_PORT=3000
EXPOSE 3000
CMD ["/filmshes/bin/filmshes", "serve"]
```

### Docker Compose

```yaml
services:
  filmshes:
    image: filmshes:latest
    ports:
      - "3000:3000"
    volumes:
      - filmshes-db:/filmshes/db
    restart: unless-stopped

volumes:
  filmshes-db:
```

### Deploy flow

```bash
make build-all                                              # → bin/filmshes-linux-amd64
docker build -f docker/release/Dockerfile.release -t filmshes:latest .
docker compose up -d
```

- Minimální Alpine image, žádný runtime
- SQLite persistuje přes Docker volume
- Single process = celá aplikace
- Go 1.25+ runtime automaticky detekuje cgroup CPU limity (container-aware GOMAXPROCS) – není potřeba manuální nastavení
- Go 1.26 používá Green Tea GC jako výchozí – snížení GC overhead o 10-40 %
