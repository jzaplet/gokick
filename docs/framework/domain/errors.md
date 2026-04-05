---
layout: 'page'
uri: '/framework/domain/errors'
position: 4
slug: 'framework-domain-errors'
parent: 'framework-domain'
navTitle: 'Error Types'
title: 'Error Types'
description: 'Doménové error typy – ValidationError, AuthError, HTTPError pattern.'
---

# Error Types

Doménové errory implementují `HTTPError` interface z `response/` balíčku **implicitně** (Go duck typing – žádný import mezi domain a response).


## ValidationError

Vstupní validace a business pravidla. HTTP status 400.

```go
// domain/errors.go

type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string   { return e.Message }
func (e *ValidationError) HTTPStatus() int { return 400 }
```


## AuthError

Nedostatečná oprávnění. HTTP status 403.

```go
type AuthError struct {
    Message string
}

func (e *AuthError) Error() string   { return e.Message }
func (e *AuthError) HTTPStatus() int { return 403 }
```


## HTTPError interface (response balíček)

`response/` definuje interface, domain ho implementuje implicitně:

```go
// response/json.go

type HTTPError interface {
    error
    HTTPStatus() int
}

func HandleError(w http.ResponseWriter, err error) {
    var httpErr HTTPError
    if errors.As(err, &httpErr) {
        Error(w, httpErr.HTTPStatus(), err)
    } else {
        Error(w, http.StatusInternalServerError, err)
    }
}
```

| Error | Status | Kdy |
|---|---|---|
| `ValidationError` | 400 | Value object validace, business pravidla |
| `AuthError` | 403 | Bus AuthorizeMiddleware, permission check |
| Ostatní | 500 | Systémové chyby |
