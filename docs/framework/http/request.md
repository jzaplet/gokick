---
layout: 'page'
uri: '/framework/request'
position: 10
slug: 'framework-request'
parent: 'framework-http'
navTitle: 'Request'
title: 'Request lifecycle'
description: 'Cesta HTTP requestu od socketu k handleru — globální middleware chain (trace, IP, recovery, security, CORS, CSRF, log) a per-route rate limit / JWT.'
---

# Request lifecycle

Než se request dostane k HTTP handleru, projde **globálním middleware chainem**. Ten se sestaví jednou při startu v `Server.buildMiddlewareChain` a vyřídí vše, co je potřeba zpracovat **dřív, než se dostane k aplikační logice** — korelaci, klientskou IP, recovery, bezpečnostní hlavičky, CORS, CSRF a access log. Vlastní autorizace, transakce a audit běží až **uvnitř busu** ([Command](/framework/command) / [Query](/framework/query)).

> Přehled toku. Detaily k jednotlivým tématům: `/gk-hardening` (CSRF, bezpečnostní hlavičky), `/gk-auth` (login/session), `/gk-rate-limiting`, `/gk-feature` (endpoint napříč vrstvami).


## K čemu to je

Tady se requestu nastaví vše, na co se zbytek systému spoléhá: `trace_id` pro korelaci logů, klientská IP (jeden zdroj pro rate limit, audit i log), per-request Sentry scope, bezpečnostní hlavičky a ochrana CORS/CSRF. Handler pak dostane `ctx`, který tyto hodnoty nese.


## Jak to teče

Middleware se uplatní v pořadí, v jakém jsou v seznamu — **první obaluje všechny ostatní**:

1. **Trace** — `X-Trace-Id` (validovaný), nebo nové UUID → `ctx` + response hlavička.
2. **IP** — klientská IP přes `IPExtractor` → `ctx` (výchozí `RemoteAddr`; proxy hlavičky jen při `APP_TRUST_PROXY_HEADERS`).
3. **ReportScope** — per-request Sentry hub + napojení na frontendovou trace.
4. **Recovery** — `recover()` zachytí paniku → log + Sentry → generický 500 (stack trace se ke klientovi nedostane).
5. **Security** — CSP, HSTS (na HTTPS), `X-Frame-Options: DENY`, … (cíl A+ na securityheaders.com).
6. **CORS** — `Access-Control-*` podle `APP_CORS_ORIGIN`, preflight `OPTIONS` → 204.
7. **CSRF** — `http.CrossOriginProtection` (Go 1.25 stdlib) přes `Sec-Fetch-Site`; `APP_CORS_ORIGIN` je registrovaný jako důvěryhodný origin (`AddTrustedOrigin`), takže CORS-povolený klient projde i zápisem.
8. **Logging** — po doběhnutí jeden access řádek: status, bytes, `duration_ms`.
9. **Per-route** — `RateLimit` (login/refresh) a `JWT Auth` se vážou jen na vybrané endpointy.

Trace, IP a ReportScope jen zapisují do `ctx` a nikdy neselžou — proto běží **před** Recovery, aby každý report nesl `trace_id` i klientskou IP.


## Příklad

Handler dělá málo — dekóduje JSON a předá command nebo query busu; `r.Context()` už nese `trace_id`, IP, případné `claims` i Sentry hub:

```go
cmd := authcmd.LoginCommand{Nickname: body.Nickname, Password: body.Password}

result, err := bus.Dispatch(r.Context(), h.commandBus, "Login", cmd,
    func(ctx context.Context) (authcmd.IssuedSession, error) {
        return h.login.Handle(ctx, cmd)
    },
)
if err != nil {
    h.resp.HandleError(r.Context(), w, err) // doménová chyba → HTTP status
    return
}
```


## Související

- [Command](/framework/command) / [Query](/framework/query) — bus chain za handlerem.
- [Configuration](/framework/configuration) — `APP_CORS_ORIGIN`, `APP_COOKIE_SECURE`, `APP_TRUST_PROXY_HEADERS`, rate limit, Sentry.
- Skilly: `/gk-hardening`, `/gk-auth`, `/gk-rate-limiting`, `/gk-feature`.
