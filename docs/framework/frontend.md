---
layout: 'page'
uri: '/framework/frontend'
position: 60
slug: 'framework-frontend'
parent: 'framework'
navTitle: 'Frontend'
title: 'Frontend'
description: 'Vue 3 SPA – Vite, TypeScript, Tailwind, embedding.'
---

# Frontend

Vue 3 SPA embedovaná do Go binárky.


## Stack

| Komponenta | Knihovna |
|---|---|
| Framework | `vue@^3` |
| Routing | `vue-router@^4` |
| Build | `vite` |
| CSS | `tailwindcss@^4` + `@tailwindcss/vite` |
| TypeScript | `typescript` + `vue-tsc` |
| Linting | `eslint` + `oxlint` |


## Struktura

```
assets/
├── app.ts              # Entry point
├── tailwind.css
└── vue/
    ├── App.vue
    ├── router/
    ├── views/
    ├── components/
    ├── composables/    # useAuth
    ├── services/       # apiFetch
    └── types/
```


## Embedding

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
make fe-deps    # Install
make fe-build   # Produkční build
make fe-dev     # Dev server s HMR
make fe-clean   # Smazání artefaktů
```
