---
layout: 'page'
uri: '/framework/overview/lifecycle'
position: 3
slug: 'framework-overview-lifecycle'
parent: 'framework-overview'
navTitle: 'Životní cyklus'
title: 'Životní cyklus'
description: 'Tok requestu, start aplikace, error flow.'
---

# Životní cyklus


## Start aplikace

```
main.go
  → di_container.CreateApplication()       Wire DI vytvoří vše
    → env.LoadConfig()                     Načtení .env
    → database.NewSqliteManager()          Připojení k SQLite
    → database.MigrationManager.RunUp()    Automatické migrace
    → bus.New(middlewares...)               CommandBus, QueryBus, EventBus
    → server.New(handlers, middlewares)     HTTP server
    → console.NewRootCommand()             Cobra CLI
  → application.Run()
    → rootCmd.Execute()                    Cobra parsuje "serve"
      → server.Start()                    Naslouchá na portu
```


## Tok requestu (command)

`POST /api/v1/admin/users` – vytvoření uživatele:

```
1. HTTP Request → net/http ServeMux

2. HTTP Middleware:
   CORS → Logging → JWT Auth (claims do context)

3. HTTP Handler:
   json.Decode → CreateUserCommand
   bus.ExecVoid(ctx, commandBus, "CreateUser", cmd, fn)

4. Bus Middleware (CommandBus):
   Recovery → Logging → Authorize → Transaction → DispatchEvents
   │
   ├─ Authorize: cmd.(Permissioned) → PermissionChecker.Check()
   ├─ Transaction: BEGIN
   └─ → handler:

5. Command Handler:
   NewNickname() → NewRole() → repo.FindByNickname() → password.Hash()
   → NewUser() → repo.Save()

6. Bus post-handler:
   Transaction → COMMIT
   DispatchEvents → flush EventCollector → async goroutiny

7. HTTP Handler: response.JSON(w, 201, nil)
```


## Tok requestu (query)

`GET /api/v1/admin/users`:

```
HTTP Request → CORS → Logging → JWT Auth
  → Handler → bus.Exec[[]domain.User](ctx, queryBus, "ListUsers", q, fn)
    → Recovery → Logging → Authorize → Query Handler → repo.FindAll()
  → response.JSON(w, 200, users)
```


## Tok requestu (veřejný)

`POST /api/v1/auth/login`:

```
HTTP Request → CORS → Logging (bez JWT Auth)
  → Handler → bus.Exec[*LoginResult](ctx, commandBus, "Login", cmd, fn)
    → Recovery → Logging → Authorize (SkipPermission → skip)
      → Transaction → Login Handler
  → http.SetCookie(refreshToken)
  → response.JSON(w, 200, {access_token, user})
```


## Tok requestu (SPA)

`GET /dashboard`:

```
HTTP Request → SPA Fallback
  → public.FS → soubor existuje? → vrátí ho
                soubor neexistuje? → index.html (Vue Router)
```


## Error flow

```
Command Handler vrátí error
  ↓
Bus: Transaction → ROLLBACK, DispatchEvents → eventy zahozeny
  ↓
HTTP Handler: response.HandleError(w, err)
  → ValidationError → 400
  → AuthError → 403
  → jiný error → 500
```
