---
layout: 'page'
uri: '/framework/presentation/http-handler'
position: 2
slug: 'framework-presentation-http-handler'
parent: 'framework-presentation'
navTitle: 'HTTP Handlers'
title: 'HTTP Handlers'
description: 'Balíček presentation/http/handler/ – deserializace, bus dispatch, error handling.'
---

# HTTP Handlers

Balíček `presentation/http/handler/`. Přijmou HTTP request, deserializují vstup, zavolají command/query přes bus.

Neimportují `infrastructure/sqlite/` ani `infrastructure/security/` – autorizace probíhá v bus middleware.


## Příklad

```go
// presentation/http/handler/admin_users.go

type AdminUsersHandler struct {
    commandBus *bus.CommandBus
    queryBus   *bus.QueryBus
    createUser *command.CreateUserHandler
    listUsers  *query.ListUsersHandler
}

func (h *AdminUsersHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
    var cmd command.CreateUserCommand
    if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
        response.HandleError(w, err)
        return
    }

    err := bus.ExecVoid(r.Context(), h.commandBus.Bus, "CreateUser", cmd, func(ctx context.Context) error {
        return h.createUser.Handle(ctx, cmd)
    })
    if err != nil {
        response.HandleError(w, err)
        return
    }
    response.JSON(w, http.StatusCreated, nil)
}

func (h *AdminUsersHandler) HandleList(w http.ResponseWriter, r *http.Request) {
    q := query.ListUsersQuery{}
    users, err := bus.Exec[[]user.User](r.Context(), h.queryBus.Bus, "ListUsers", q, func(ctx context.Context) ([]user.User, error) {
        return h.listUsers.Handle(ctx, q)
    })
    if err != nil {
        response.HandleError(w, err)
        return
    }
    response.JSON(w, http.StatusOK, users)
}
```


## Error handling

`response.HandleError(w, err)` je centralizované – handler se nestará o mapování error→status. Viz [Error typy](/framework/domain/errors).
