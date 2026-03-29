---
layout: 'page'
uri: '/framework/adapters/response'
position: 6
slug: 'framework-adapters-response'
parent: 'framework-adapters'
navTitle: 'Response'
title: 'Response'
description: 'Balíček response/ – JSON helpery, HTTPError interface, HandleError.'
---

# Response

Balíček `response/`. JSON response helpery a centralizovaný error handling. **Žádné závislosti** – jen stdlib.


## API

```go
// response/json.go

func JSON(w http.ResponseWriter, status int, data any)
func Error(w http.ResponseWriter, status int, err error)
func HandleError(w http.ResponseWriter, err error)
```


## HTTPError interface

```go
type HTTPError interface {
    error
    HTTPStatus() int
}
```

`HandleError` kontroluje zda error implementuje `HTTPError`:
- Ano → použije `HTTPStatus()` (400, 403, ...)
- Ne → vrátí 500

Domain typy (`ValidationError`, `AuthError`) implementují `HTTPError` implicitně (duck typing). Žádný import mezi `response/` a `domain/`.
