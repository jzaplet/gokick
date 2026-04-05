---
layout: 'page'
uri: '/framework/presentation/http-server'
position: 1
slug: 'framework-presentation-http-server'
parent: 'framework-presentation'
navTitle: 'HTTP Server'
title: 'HTTP Server'
description: 'Balíček presentation/http/server/ – routing, SPA fallback, Vite proxy.'
---

# HTTP Server

Balíček `presentation/http/server/`. `net/http` stdlib s Go 1.26 routing. Registruje handlery a middleware na `http.ServeMux`.


## Routing

**Veřejné:**

| Metoda | Route | Popis |
|---|---|---|
| GET | `/health` | Health check |
| POST | `/api/v1/auth/login` | Přihlášení (nickname + heslo) |
| POST | `/api/v1/auth/refresh` | Obnovení tokenu |
| GET | `/{path...}` | SPA fallback |

**Chráněné (Bearer):**

| Metoda | Route | Popis |
|---|---|---|
| POST | `/api/v1/auth/logout` | Odhlášení |
| GET | `/api/v1/profile` | Profil |
| PUT | `/api/v1/profile/password` | Změna hesla |

**Admin (Bearer + role):**

| Metoda | Route | Popis |
|---|---|---|
| GET | `/api/v1/admin/users` | Seznam uživatelů |
| POST | `/api/v1/admin/users` | Vytvoření |
| PUT | `/api/v1/admin/users/{id}` | Editace |
| DELETE | `/api/v1/admin/users/{id}` | Smazání |


## SPA fallback

Catch-all `/{path...}` vrací soubory z `public.FS` (embed). Neexistující cesty → `index.html` (Vue Router rozhodne).


## Vite dev proxy

```typescript
// vite.config.ts
server: {
    proxy: {
        '/api': 'http://localhost:3000',
        '/health': 'http://localhost:3000'
    }
}
```
