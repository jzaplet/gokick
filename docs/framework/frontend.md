---
layout: 'page'
uri: '/framework/frontend'
position: 9
slug: 'framework-frontend'
parent: 'framework'
navTitle: 'Frontend'
title: 'Frontend'
description: 'Vue 3 SPA – Vite, TypeScript, Tailwind, router, composables.'
---

# Frontend

Vue 3 SPA embedovaná do Go binárky.


## Stack

| Komponenta | Knihovna |
|---|---|
| Framework | `vue@^3` |
| Routing | `vue-router@^4` |
| Build tool | `vite` |
| CSS | `tailwindcss@^4` + `@tailwindcss/vite` |
| TypeScript | `typescript` + `vue-tsc` |
| Linting | `eslint` + `oxlint` |


## Adresářová struktura

```
assets/
├── app.ts                    # Entry point
├── tailwind.css              # Tailwind konfigurace
└── vue/
    ├── App.vue               # Root komponenta
    ├── router/               # Vue Router
    ├── views/                # Stránkové komponenty
    ├── components/           # UI komponenty
    ├── composables/          # useAuth, sdílený stav
    ├── services/             # apiFetch, API klienti
    └── types/                # TypeScript typy
```


## Entry point

`index.html` → `assets/app.ts` → mountuje Vue s routerem.


## Vue Router

```typescript
createRouter({
    history: createWebHistory('/'),
    routes: [
        { path: '/login', component: LoginView },
        { path: '/', component: HomeView, meta: { requiresAuth: true } },
        // ...
    ]
})
```

Backend SPA fallback zajistí, že přímý přístup na jakoukoliv URL vrátí `index.html`.


## Stav

Žádná externí state management knihovna. Sdílený stav přes composables (`ref`, `computed`, `provide/inject`).

- **`useAuth()`** – auth stav, login/logout/refresh
- **`apiFetch()`** – fetch wrapper s auth a auto-refresh

Detaily viz [Autentizace](/framework/auth).


## Embedding do binárky

Vite builduje do `public/`. Go embeduje přes `//go:embed *`:

```go
// public/embed.go
package public

import "embed"

//go:embed *
var FS embed.FS
```


## Makefile

```bash
make fe-deps    # Install závislostí
make fe-build   # Vite produkční build → public/
make fe-dev     # Vite dev server s HMR
make fe-clean   # Smazání build artefaktů
```
