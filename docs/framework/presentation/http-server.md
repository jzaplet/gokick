---
layout: 'page'
uri: '/framework/presentation/http-server'
position: 1
slug: 'framework-presentation-http-server'
parent: 'framework-presentation'
navTitle: 'HTTP Server'
title: 'HTTP Server'
description: 'Routing, SPA fallback, Vite proxy, response helpery, HTTPError interface.'
---

# HTTP Server

## Proč

Server je jediný vstupní bod pro HTTP provoz. Centralizuje routing, middleware chain a response format na jednom místě, takže handlery a doména zůstávají čisté.

## Jak

### Routing

Server používá stdlib `net/http.ServeMux` s Go 1.22+ pattern routingem. Routy se registrují v `presentation/http/server/`.

**Veřejné:**

| Metoda | Route | Popis |
|---|---|---|
| GET | `/health` | Health check |
| POST | `/api/v1/auth/login` | Přihlášení (nickname + heslo) |
| POST | `/api/v1/auth/refresh` | Obnovení tokenu |
| GET | `/{path...}` | SPA fallback |

**Chráněné (JWT Bearer):**

| Metoda | Route | Popis |
|---|---|---|
| POST | `/api/v1/auth/logout` | Odhlášení |
| GET | `/api/v1/profile` | Profil |
| PUT | `/api/v1/profile/password` | Změna hesla |

**Admin (JWT Bearer + admin role):**

| Metoda | Route | Popis |
|---|---|---|
| GET | `/api/v1/admin/users` | Seznam uživatelů |
| POST | `/api/v1/admin/users` | Vytvoření |
| PUT | `/api/v1/admin/users/{id}` | Editace |
| DELETE | `/api/v1/admin/users/{id}` | Smazání |

### SPA fallback

Catch-all `GET /{path...}` vrací soubory z embedovaného `public.FS`. Neexistující cesty vrátí `index.html` -- o routing rozhoduje Vue Router na klientu.

### Vite dev proxy

Při vývoji frontend běží na Vite dev serveru. Proxy směruje API cesty na Go server:

```typescript
// vite.config.ts
server: {
    proxy: {
        '/api': 'http://localhost:3000',
        '/health': 'http://localhost:3000'
    }
}
```

### Response helpery

Balíček `presentation/http/response/` poskytuje tři funkce pro jednotný JSON output:

```go
// presentation/http/response/response.go

func JSON(w http.ResponseWriter, status int, data any)
func Error(w http.ResponseWriter, status int, err error)
func HandleError(w http.ResponseWriter, err error)
```

- `JSON()` -- serializuje `data` do JSON a nastaví `Content-Type` + status code.
- `Error()` -- zapíše chybovou odpověď s explicitním status kódem.
- `HandleError()` -- automaticky mapuje error na správný HTTP status.

### HTTPError interface

```go
type HTTPError interface {
    error
    HTTPStatus() int
}
```

`HandleError` kontroluje, zda error implementuje `HTTPError`:

- **Ano** -- použije `HTTPStatus()` (např. 400, 403).
- **Ne** -- vrátí 500 Internal Server Error.

## Detaily

- Domain error typy (`ValidationError`, `AuthError`) implementují `HTTPError` implicitně (duck typing). `ValidationError` vrací 400, `AuthError` vrací 403. Žádný import mezi `response/` a `domain/`.
- Server struct drží `*http.ServeMux`, middleware chain a `Start()` metodu, která spustí `http.ListenAndServe`.
- Response balíček nemá žádné závislosti kromě stdlib.
