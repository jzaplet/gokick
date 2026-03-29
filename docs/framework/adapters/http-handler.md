---
layout: 'page'
uri: '/framework/adapters/http-handler'
position: 2
slug: 'framework-adapters-http-handler'
parent: 'framework-adapters'
navTitle: 'HTTP Handlery'
title: 'HTTP Handlery'
description: 'Balíček handler/ – deserializace, bus dispatch, error handling.'
---

# HTTP Handlery

Balíček `handler/`. Přijmou HTTP request, deserializují vstup, zavolají command/query přes bus.

Neimportují `sqlite/` ani `security/` – autorizace probíhá v bus middleware.


## Příklad

```go
// handler/admin_users.go

type AdminUsersHandler struct {
    commandBus *bus.Bus
    queryBus   *bus.Bus
    createUser *command.CreateUserHandler
    listUsers  *query.ListUsersHandler
}

func (h *AdminUsersHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
    var cmd command.CreateUserCommand
    if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
        response.HandleError(w, err)
        return
    }

    err := bus.ExecVoid(r.Context(), h.commandBus, "CreateUser", cmd, func(ctx context.Context) error {
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
    users, err := bus.Exec[[]domain.User](r.Context(), h.queryBus, "ListUsers", q, func(ctx context.Context) ([]domain.User, error) {
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
