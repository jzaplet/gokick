---
layout: 'page'
uri: '/framework/gokick-roadmap'
position: 5
slug: 'framework-gokick-roadmap'
parent: 'framework'
navTitle: 'Roadmap (GoKick)'
title: 'Roadmap (GoKick)'
description: 'Aktuální priorita F6 — zapínatelný row-level multitenancy (hotovo) + OpenTelemetry observabilita; bodová cesta k 10/10 v každé disciplíně. Plné hodnocení stacku v PDF reportu.'
---

# Roadmap (GoKick)


## 📊 Hodnocení stacku — 8,5 / 10

> **[⬇ Stáhnout PDF report](../gokick-hodnoceni.pdf)** — nezávislý audit reálného kódu.

<a href="../gokick-hodnoceni.pdf"><img src="../go-vue-cqrs-ddd.png" alt="Hodnocení stacku gokick — PDF report" width="200"></a>

Boilerplate je **production-ready end-to-end**: DDD/CQRS backend, Vue 3 SPA, JWT auth s HttpOnly refresh cookie a detekcí krádeže, admin user CRUD, perzistentní durable engine + scheduler, rate limiting, audit log, brute-force lock, security headers, Sentry (BE i FE), single-binary deploy. Fáze 1–5 jsou hotové; z F6 je **multitenancy + system bus pro CLI + job↔run konvergence hotové** (OTEL zbývá) — rekapitulace v sekci **Hotovo** níže.

Tenhle dokument je **forward-looking**: co zbývá jako aktuální priorita a co konkrétně chybí do plné desítky v každé disciplíně.


## 🚀 Aktuální priorita — Multitenancy + observabilita (F6)

gokick dostává **zapínatelný multitenancy** a **OpenTelemetry** — obojí je široce přenositelné do dalších projektů, proto patří do jádra, ne dodělávat per-projekt.

### ✅ Multitenancy — HOTOVO ([PR #15](https://github.com/jzaplet/gokick/pull/15))

Zapínatelný **row-level** multitenancy jedním přepínačem (`APP_MULTITENANCY`, default vypnuto = dnešní single-tenant chování) + platformní rovina pro autory aplikace. Detail v **`/gk-multitenancy`** skillu.

- **Izolace:** `tenant_id` (NOT NULL FK) na owned tabulkách; resolver tenant **dodá** (z JWT), repo ho **aplikuje** (`r.Tenant(ctx)`). Bez ORM = žádné transparentní `WHERE`, proto izolaci hlídá **per-dotaz conformance gate** (`zz_tenant_test.go`, padá v CI). Flag vybírá fail-open (chybějící tenant → default) vs fail-closed (panika).
- **Worker propagace:** worker obchází bus, takže tenant jede na `runs` řádku a worker ho obnoví do ctx před handlerem.
- **Platformní rovina:** role `superadmin` + `platform:*` (nad admin/user) — cross-tenant dashboard (počty tenantů/uživatelů), přehled tenantů a uživatelů s cross-tenant správou. Superadmin admin sekci nevidí.
- **Operator tooling:** seed s multitenancy on založí adminovi vlastní tenant; CLI `create-tenant`, `create-superadmin`, `create-user --tenant-id/--tenant-name` (s MT on je tenant povinný).
- **Vědomý strop:** transparentní vynucení (tichý read-leak na slepém místě scanneru) dá až **Postgres RLS** — viz disciplína **Škálovatelnost** níže.

### Krok — System command bus pro CLI ✅ HOTOVO

**Hotovo** přes `bus.SystemCommandBus` (4 commity) — detail v `/gk-bus`. Motivace: čtyři CLI commandy (`seed`, `create-user`, `create-superadmin`, `create-tenant`) obcházely bus a volaly handlery napřímo → **žádný audit, žádná atomická transakce** (orphan-tenant třída chyb, jeden výskyt opraven ručně v `create-user`), žádný strukturovaný log, žádné Sentry při neočekávaném pádu. `create-superadmin` na živém serveru navíc nezanechal **žádnou auditní stopu** — přitom je to nejcitlivější operátorská akce vůbec. Bus se obcházel proto, že `Authorize` i `Tenant` jsou claims-driven (čtou principála/tenant z JWT), a CLI žádný JWT nemá.

Řešení = **system bus levně**: druhý provider `provideSystemCommandBus` poskládá **podmnožinu** stávajících (už composable) middlewarů — žádná nová abstrakce, jen jiný subset:

```
Recovery(→Sentry) → Logging → Audit → RunDispatcher → DispatchEvents → Transaction      (vynechán Authorize i Tenant)
```

- [x] **`provideSystemCommandBus`** — vynechává `Authorize` (operator trust; kontrakt `Permissioned`/`SkipPermission` žije *uvnitř* AuthorizeMiddleware, takže vynechání je čisté) i `Tenant` (resolver by přemázl tenant injectnutý přes `ContextWithTenantID`). Pořadí: **Audit vně Transaction**, **DispatchEvents obaluje Transaction**, **RunDispatcher mezi nimi** (stejně jako v CommandChainu — bez něj by durable enqueue z event handleru tiše no-opnul; doplněno dodatečně).
- [x] **4 commandy přepojeny** přes `bus.SystemDispatchVoid`/`bus.SystemDispatch` skrz system bus; ruční transakce v `create-user` zmizela (dává ji `TransactionMiddleware`) → odpadl bespoke `shared.Transactor` wiring. **seed** je teď taky přes bus → **atomický bootstrap** (all-or-nothing).
- [x] **Audit záznamy** doplněny: `create-superadmin`/seed superadmin → `user.created`, `create-tenant`/seed tenant → `tenant.created` (bez ActorUserID — systémová akce). Ověřeno end-to-end přes reálný system bus.
- [x] **Sentry = jen neočekávané** — paniky reportuje `RecoveryMiddleware` zadarmo; očekávané validační chyby do trackeru nejdou (invariant „error reporting is for the unexpected only").

### Krok — Sloučit job + durable run do jednoho primitiva

> Nahrazuje původní „konfigurovatelný job lease + heartbeat" — ten cíl (lease + heartbeat pro dlouhou práci) splnil **durable run** ([PR #21](https://github.com/jzaplet/gokick/pull/21)). Zůstával ale dvojí mechanismus: job (handler v transakci → držel write-lock celou dobu, takže „volání ven v jobu" zamrzlo DB a nešlo to vynutit) a run (mimo tx, vynutitelně). **PR #22 ten footgun odstranil sloučením** — job teď běží mimo tx jako run (viz `[x]` níže).

- [x] **Sloučeno do jednoho `durable task`** ([PR #22](https://github.com/jzaplet/gokick/pull/22)) — jeden engine (tabulka `runs`, jeden worker), mimo tx, idempotentní/at-least-once, **volitelný** checkpoint (s checkpointem = run/`Durable`, bez = job/`FireAndForget`), **timeout** per task. „Job" = „task, který necheckpointuješ". Starý job běžel celý v transakci → držel SQLite write-lock po celou dobu, takže „volání ven v jobu" (SMTP/cizí API) zamrzlo DB a nešlo to vynutit; teď job běží **mimo tx** jako run a outside-tx je vynucené (`ContextForbidTx` + `zz_notx_test.go`). Tabulka `jobs` zahozena (`20260629000001_drop_jobs_table.sql`).

### Krok — OpenTelemetry (až na finálním tvaru)

- [ ] **OTel HTTP middleware + propagace přes bus** — `trace_id` v ctx přejde na `trace.SpanContext`, sladit s `shared.LogKeyTraceID` (traces ↔ logy korelují).
- [ ] **Span per durable task** (run worker, `process()` — **bez** transakce: handler běží outside-tx, takže span obaluje běh handleru, ne tx; checkpointy/heartbeaty jako child-spany nebo span-events) + **SQL viditelnost přes `otelsql`** (span per dotaz) — proto se vědomě nestaví vlastní SQL→breadcrumb most. Worker dnes trace_id nemá (logy korelují přes `run_id`); OTEL ho nasadí přes `shared.LogAttrs(ctx)`.
- [ ] **FE↔BE distributed tracing — full** — light verze hotová ([PR #11](https://github.com/jzaplet/gokick/pull/11)); full přidá `tracesSampleRate > 0` → spany + waterfall (FE klik → API → handler → DB).
- [ ] **Hardening:** `otelsql` + OTEL SDK do depguard allow-listu (`.golangci.yml`); collector endpoint do CSP `connect-src` + `traceparent` přes CORS.


## 🔗 BE↔FE typová parita — vynucení „nemáš kam uhnout" (post-audit follow-up)

Dnes generuje `gk tsgen` (modul `tools/gk`) TypeScript typy z anotovaných Go DTO (direktiva `//gkts:<cesta> <TSName>` → přesná cílová `.ts` cesta per typ; cesta je první schválně — malé písmeno za dvojtečkou z ní dělá pravou gofmt/golines direktivu, takže ji formatter nechá být) a `make ts-check` v `make lint` hlídá, že vygenerované soubory nedriftnou. To řeší drift **anotovaných** typů, ale je to **opt-in** — a tím zůstává díra:

- Zapomenu dát `//gkts:` na nový response/request DTO → generátor mlčí.
- Handler odpoví inline `map[string]any` místo pojmenovaného structu → nikdo to nechytí.
- FE `fetch` pošle neotypované `body` nebo zkonzumuje odpověď jako `any` → parita nevynucená.

Generátor umí říct „tenhle typ driftnul", ne „tady jsi typ zapomněl". **Cílový stav = uzavřená smyčka**, kde `//gkts:` na Go DTO je jediný zdroj pravdy a hranice drátu se vynutí staticky:

- [x] **① Codegen (`gk tsgen`)** — Go DTO → TS, drift = fail v `make lint`. *(hotovo — 12 wire DTO generováno a gate-ováno; `healthResponse` je z tsgen vědomě vyňatý — infra-only endpoint, FE typ by byl dead code)*
- [x] **② BE boundary analyzer** — `gk boundary` (type-checked scan přes `go/packages`, v `make lint` jako `boundary-check`): každý `resp.JSON(ctx, w, _, X)` (metoda na injektovaném `*response.Responder`) a `request.DecodeJSON(w, r, &X)` musí mít `X` = pojmenovaný struct s `//gkts:` (slice/pointer/alias povoleny). Inline mapa / `any` / neanotovaný typ = fail. Escape `//gkts:ignore <reason>` je **jen call-site** (type-level by tsgen přečetl jako cestu), musí stát **sám na řádku nad voláním**, bez důvodu selže a **neaktivní (nic nekryjící) marker je sám violation** — stale escapes nemůžou tiše hnít; dnes ho nesou `/health`, 3 místa `APP_RUN_DEBUG` debug endpointů a embedded SPA shell (`spa.go`). Drift guardy: zero-sites **per kind** (JSON i DecodeJSON zvlášť), arity drift volání = violation, nová `any`-payload metoda na Responderu = violation (gate se musí naučit dřív, než route vznikne). Matchuje se **typ** (receiver `*response.Responder`), ne jméno proměnné — a obchvaty ve wire vrstvě (presentation/http/** mimo response/+request/ plumbing) padají taky: přímé volání `encoding/json`, package-level funkce `response.*` (konstruktory vracející `*Responder` povoleny — a marker tyhle bypassy nikdy neumlčí), **method values** (`send := resp.JSON`) i **raw writes v handlerech** (`w.Write`, `io.WriteString`, `fmt.Fprint*`). Test suite: fixture modul v `tools/gk/boundary/testdata/` kouše každé pravidlo (16 violations + přesné per-kind počty) + drift fixture (změněná signatura → hlasitý fail). Výsledek: **Go response nejde odeslat mimo generovaný DTO.**
- [x] **③ FE typovaný fetch + lint** — `authFetch<Res, Err, Req>` / `apiFetch<Res, Err, Req>`: `FetchOptions<TBody = never>` s **`body?: NoInfer<TBody>`** znamená, že **body bez deklarovaného `Req` typu se nezkompiluje** — `NoInfer` brání TS odvodit `TBody` z argumentu, takže `never` default drží na úrovni kompilátoru pro každý tvar volání (aliasy, namespace importy, re-bindingy — tam name-based lint nevidí). ESLint (`no-restricted-syntax`, scoped na `assets/`) k tomu vynucuje explicitní generiky (čitelnost + review) a zakazuje inline `body` literály (payload teče z typované proměnné). Ověřeno kousnutím všech vrstev. Poctivá mez: že deklarovaný `Req` je **generovaný** typ (a ne ad-hoc lokální), je konvence viditelná v review — kompilátor vynucuje explicitnost, ne původ typu. Výsledek: **fetch nejde poslat s neotypovaným body.** A zpáteční směr taky: tsgen ke každému response typu generuje **runtime guard** (`is<Typ>`, opt-out `noguard` pro request-only DTO) a data-endpointy musí předat `validate:` (vynuceno typem — bez guardu se volání nezkompiluje, guard↔generika je párovaná přes `Guard<TData>`); 2xx tělo porušující kontrakt = `{ general }` failure + Sentry report. Syntetické chyby (network error, rozbité tělo, porušený kontrakt) přestaly předstírat `TError` — failure `data` je poctivá unie `TError | { general: string }`; protože každý `*Errors` typ má `general?: string` (stejný klíč, kterým BE posílá ne-field chyby), zůstává konzument u jednořádkového `errors.value = result.data` bez zužování. Upload/download stojí ke smyčce takhle: `apiUpload` posílá multipart `FormData` (soubory — žádný JSON DTO, request půlka je z principu mimo tsgen; kontrakt jmen polí se u budoucího endpointu přibije golden testem), ale jeho **JSON response v paritě je** — `TData` = generovaný typ a explicit-generics ESLint pravidlo ho kryje; `apiDownload` je binární blob + `Content-Disposition` (žádné JSON na obou stranách, parita se netýká, `DownloadResult` je správně FE-only typ).
- [x] **④ Parita chybových polí (F-077)** — vyřešeno **statikou, bez golden testů** (Jiřího volba): `gk errfields` (v `make lint` jako `errfields-check`) extrahuje všechny Go `ValidationError{Field: "…"}` literály (dekódované přes `strconv.Unquote` — raw string i escape sekvence sedí) a klíče všech FE `*Errors` typů a kontroluje obousměrně — field bez FE domova i fantomový FE klíč = fail; `general` je konvenční catch-all (Field `""` + syntetizovaná selhání); dynamický (ne-literálový) `Field` i **poziční literál** (bez `Field:` klíče) = fail. Escape `//gkerrf:exempt <důvod>` (povinný důvod, **sám na řádku nad literálem**; klíčuje se na začátek composite literalu, takže golines rewrap exempci neutrhne; trailing i nevyužitý marker = violation) — dnes 8× `id` (path-param lookupy → redirect/toast) a tenant `name` (CLI-only). Name-matching drží poctivé **tripwiry**: druhý typ `ValidationError` mimo `domain/shared` nebo konstruktor `NewValidationError` = fail, dokud se tool nenaučí. FE stranu jistí scan na **stray `*Errors` typy** deklarované mimo `*Errors.ts` soubor (parita by je neviděla). Kontrola je záměrně globální, ne per-endpoint (to by chtělo call-graph handler→command→VO — možné pozdější zpřesnění); překlepy a přejmenování — reálnou třídu driftu — chytá spolehlivě. Zero-sites guardy na obou stranách + testy všech violation tříd na fixtures.
- [x] **⑤ Role union přes codegen (F-095)** — **hotovo**: `//gkts:<cesta> <Name> union` na pojmenovaném string typu (`user.Role`) emituje trojici **const objekt + `type` + guard**; `AuthUser.role`, `AdminUser.role`, `PlatformUser.role`, `UserFormData.role` a `PlatformUserFormData.role` už nejsou `string`, ale `Role`. Ruční enum v `assets/app/Auth/enums/roles.ts` je teď **generovaný na téže cestě** se stejným export surface, takže se 7 importérů nedotklo.
  - **Trojice, ne jen union type** — `Role.Admin` se používá jako **hodnota**; samotný `type Role = 'admin' | …` by ruční enum nenahradil.
  - **Dohledání konstanty přes balíčky** — `user.Role` konsty jsou konverze (`Role(shared.RoleSuperAdmin)`), aby žebřík a VO sdílely jednu definici; AST-only tsgen proto resolvuje argument konverze skrz importy volajícího souboru do deklarujícího balíčku. Jištěno fixture `tools/gk/tsgen/testdata/union/` (ten vzor reprodukuje) — ověřeno mutací: bez toho hopu fixture spadne.
  - **Guard je bez castu** (`v === Role.Admin || …`) — cast v „DO NOT EDIT" souboru by shodil no-as ráčnu ⑥.
  - **Zesílený kontrakt:** `isAdminUser` teď roli ověřuje (`isRole` místo `isString`). Bezpečné díky DB `CHECK (role IN (…))` — entita `user.User.Role` je `string`, který sqlx skenuje bez validace, takže garanci drží databáze, ne VO.
  - **Filtry zůstaly `string`** — `bulkDeleteUsersRequest`/`bulkActiveUsersRequest` a platformní dvojčata nesou filtr z gridu, kde `""` = nefiltrovat.
  - **Vedlejší nález:** union označil za mrtvý ruční check `data.user.role === ''` v `assets/app-ui/Auth/state.ts` — `isRole` ho uvnitř guardu odmítne dřív. Codegen tím pohltil ručně psanou sémantickou kontrolu.

- [x] **⑥ no-as ratchet (F-085)** — **hotovo**: `@typescript-eslint/consistent-type-assertions` (`assertionStyle: 'never'`) běží na `assets/**`; `as const` pravidlo nehlásí (ověřeno), takže Role/Permission enumy zůstaly beze změny. Inventář se srazil ze 3 castů na 2: `useToast` localStorage cast nahradil guard (`isStoredToasts`, složený ze stejných primitiv jako drátové guardy — localStorage je stejně nedůvěryhodná hranice, hodnota přežije deploy, takže payload ze starého schématu je reálný vstup, ne hypotéza; dřív prošel a projevil se až jako `undefined` hluboko v renderu). Zbylé dva casty v `parseResponse.ts` jsou **nevyhnutelné** a nesou `eslint-disable` s důvodem (stejná disciplína jako raw-pool výjimky v Go): `null as TData` drží invariant „bez `validate` ⇔ `TData = null`", který žije v overloadech `apiFetch` a generické tělo ho nevidí — a 204 nemá co guardovat; `json as TError` má `TError` od volajícího, takže není co zavolat — `isRecord` nad ním připíná jediné, na co konzumenti spoléhají (je to objekt slučitelný do `errors.value`). Historická poznámka: ⑥ zůstávalo otevřené kvůli rozhodnutí o těch dvou výjimkách, ne kvůli nástroji.

② + ③ dohromady = smyčka zavřená: nový handler ani nový fetch nelze přidat bez typu, který existuje a matchuje na obou stranách. Airtight je BE strana (statická jistota); FE je „velmi těsné" (strukturální typing TS), ale v kombinaci s ② nemá FE co poslat mimo deklarovaný kontrakt.

### Sjednocení dev-tooling adresářů

Vedlejší cíl: roztříštěnost dev nástrojů (`cmd/` vs samostatný modul per nástroj vs `audit/tool`) je matoucí — kam sáhnout, co použít.

- `cmd/` = samotná aplikace (zůstává — to je produkt).
- `audit/tool` = dočasný tracker — **odstraněn** po dopracování backlogu (teardown 2026-07-15; archiv na PR #24).
<!-- gkdoc:ignore tools/tsgen — historické umístění před migrací, kterou tahle odškrtnutá položka popisuje -->
- [x] `tools/tsgen` → **jeden `tools/gk` modul** se sub-příkazy (`gk tsgen generate|check`, `gk boundary`, `gk errfields` — analyzery ② a ④ přibyly přesně takhle: subpackage + case v dispatcheru, žádný další `go.mod`; sdílené kusy žijí v `tools/gk/internal/tool`). Vlastní `go.mod` je nutnost — codegen píše na stdout i do souborů, což lint invarianty hlavního modulu (jediná logovací cesta) zakazují.
- **Ustálený stav = dva moduly:** `cmd/` (app) + `tools/gk` (dev codegen/checks). *(platí už teď)*
- **Make surface se nemění** — codegen se skládá do `make lint` (drift = fail); regen dává ruční `make ts-gen` (mirror `make di`), takže se vynutí sám; 5 hlavních příkazů v README zůstává jedinou plochou. *„Code-gen, na který si nikdy nevzpomeneš, protože se vynutí sám."*

### FE lint gaty (ratchet) — cognitive complexity ✅ HOTOVO

Post-audit zapnul FE gaty `max-lines 300` + `max-depth 4` + `knip` (dead-code, strict) do `make lint`. **Kognitivní complexity gate od 2026-07-17 běží taky** (`sonarjs/cognitive-complexity`, práh **15**) — tím je FE dorovnaný s Go (`gocognit`, práh 20).

- [x] **TS7 blokáda odpadla, gate zapnut.** Tahle položka dlouho zněla „čeká se na TS7": `eslint-plugin-sonarjs` měl `typescript: >=5` jako **přímou** závislost, takže bez horního stropu dotáhl TS7, který `typescript-eslint` nesnese (`<6.1.0`). **Od sonarjs 4.2.0 je strop `>=5 <6.1.0`** — přesně to, co chce `typescript-eslint`, takže konflikt zmizel a **TS7 není potřeba**. Ověřeno instalací i během, ne jen čtením metadat: vnořený TS je 6.0.3, root zůstává 6.0.2, pravidlo kouše (práh 1 → 40 nálezů, práh 15 → 0). Projekt zůstává na **TS 6.0.2**; upgrade na TS7 je teď samostatná otázka bez vazby na tenhle gate.
- **Práh 15** = výchozí hodnota metriky a zároveň strop, kterého kód dosahuje (`parseResponse` sedí přesně na 15, druhá nejvyšší je `createGridState` na 11) — ráčna bez vaty, záměrně.
- **Cyklomatickou vědomě nechceme** (`cyclop` na do-not-enable listu v Go, ESLint `complexity` nezapnutý na FE): účtuje plochý validační řetězec stejně jako hluboké vnoření a tlačí schovávat větve do helperů místo je zjednodušit. Kognitivní metrika trestá vnoření a `switch` počítá jako +1, ne +1 za každý `case`.
- **`max-lines-per-function` taky ne** — zvažováno a zamítnuto: v Go jsou „metody" samostatné top-level funkce, které `funlen` účtuje jednotlivě, kdežto v JS jsou metody factory closure **uvnitř** ní, takže by pravidlo naúčtovalo všechny rodiči. `createGridState` má 149 řádků z ~15 malých closure při kognitivní komplexitě 11 — rozbití by jen rozprášilo stav, kvůli kterému existuje. Tenhle tvar pokrývá `max-lines` (soubor) + kognitivní komplexita; počet řádků na funkci ne.
- **Ostatní sonar metriky zvážené a zamítnuté** (změřeno na našem kódu): `expression-complexity` pálí na **generovaných** tsgen guardech (4×), `max-union-size` trestá `'primary' | 'secondary' | 'danger' | 'ghost'` (4×) — tedy přesně vzor, který ⑤ rozšiřuje, `no-duplicate-string` pálí na Tailwind třídách (3×). `no-identical-functions`, `max-switch-cases`, `no-nested-{switch,functions,template-literals}` a `too-many-break-or-continue-in-loop` mají u nás 0 nálezů → nic by nepřidaly.


## Cesta k 10/10

Co konkrétně chybí do plného skóre v jednotlivých disciplínách. **Jedna disciplína nese ~90 % práce** — škálovatelnost; zbytek představují cílené dílčí úpravy v řádu týdnů. Dokumentace & AI skills je na **10/10** (ADRs jsou součástí šablony, ne tohoto projektu — do hodnocení se nepromítají). Detaily a kontext v [PDF reportu](../gokick-hodnoceni.pdf), kapitola 9.

### 🔴 Škálovatelnost `4 → 10` — sem patří většina práce

Největší (a jediný zásadní) strop: single-node SQLite (single-writer) + scheduler bez leader election. **SQLite je přitom vědomá volba, ne nedopatření** — řeší se adaptérem, ne opuštěním návrhu. A protože perzistence sedí za doménovými `Repository` interface, jde o **výměnu adapteru, ne přepis aplikace**. Preferovaná cesta je **adaptér na Postgres** (i transparentní-enforcement endgame pro multitenancy přes **RLS**); alternativně lze zůstat u distribuovaného SQLite. Dvě cesty:

- **A) Zůstat u SQLite (HA + read-scale):**
  - **Turso / libSQL** — embedded replicas + automatický sync, nově i concurrent writes (MVCC, obchází single-writer bottleneck).
  - **rqlite** — Raft, nejzralejší clustering; **dqlite** (Canonical, Raft).
  - LiteFS funguje, ale je pre-1.0 a Fly.io ho deprioritizoval → pro nové projekty spíš Turso.
- **B) Skutečný write-scale (Postgres):**
  - Přidat `infrastructure/postgres/*` + `wire.Bind` na stávající doménové interface (adapter swap).
  - Frontu durable runů nahradit **River** (Postgres-native, battle-tested) místo custom SQLite queue.
  - **Durable runs (`runs` tabulka):** `ClaimDue` na Postgresu přepsat na `SELECT … FOR UPDATE SKIP LOCKED` místo SQLite single-writer + `UPDATE … RETURNING`. **Vedlejší benefit:** současný SQLite `ClaimDue` obaluje indexovaný `run_at` do `julianday()` (kvůli ms-precision korektnosti proti `strftime('%f')` round-half-up skew), julianday() obalování je na SQLite už vyřešené expression indexem — `idx_runs_claim` keyuje přímo `julianday(run_at)`/`julianday(locked_until)`, takže claim dělá bounded range-seek a čte setříděně (nález z xhigh code-review PR #21, od té doby opraveno). Postgresí seek na nativním `timestamptz` je pak jen přirozenější tvar téhož bez expression indexu.. Postgresí seek na nativním `timestamptz` tohle odstraní bez kompromisu na přesnosti. (Nález z xhigh code-review PR #21.)
  - Scheduler ošetřit **leader election** (Postgres advisory locks) — konec double-runů na víc instancích.
  - Rate-limit stav externalizovat (Redis), aby instance byly stateless.

### 🟡 Testovací pokrytí `7,5 → 10`

- **E2E** (Playwright) — chybí browser flow testy.
- **Load testy** (k6 / vegeta) — žádné výkonové stropy nejsou ověřené.
- **Coverage gate** v CI + **contract** a **mutation** testy.

### 🟢 Bezpečnost `9 → 10`

- **2FA** (TOTP) + **WebAuthn**.
- Refresh token na 256-bit; **EdDSA** (asymetrický) JWT pro multi-service nasazení.
- CSP bez `unsafe-inline` (nonce-based); **gosec + CodeQL + govulncheck** v CI; HIBP breach-check hesel.

### 🟢 Architektura `9 → 10`

- Zapojit dnes prázdné **event/run handler registry** reálnými handlery (vzorové příklady).
- Druhý **bounded context** jako referenční vzor (dnes je doména hlavně auth + users).

### 🟢 Výkon `8,5 → 10`

- **Keyset stránkování** místo `LIMIT/OFFSET` — list dotazy (`FindPage`, `FindPageAcrossTenants`, `OverviewPageAcrossTenants`) dnes stránkují offsetem: hluboká stránka musí naskenovat a zahodit všechny předchozí řádky, a souběžný zápis může řádek na hranici stránky duplikovat nebo úplně vynechat.
- **Cache vrstva** (in-proc / Redis) pro hot reads.
- **Benchmarky + pprof** profily.

### 🟢 Frontend `8,5 → 10`

- **E2E** (Playwright), **a11y** audit (axe), **i18n**.

### 🟢 Tooling / DX `9,5 → 10`

- **Coverage gate + mutation testing** v CI.
- **Dependabot / Renovate** + govulncheck.
- **SBOM** + podepsané release (cosign) + pre-commit hooky.

## Hotovo (F1–F6)

Rekapitulace — detailní záznam (Definition of Done, regresní testy, klíčová rozhodnutí) je v git historii tohoto souboru.

- **F1 — Event flow & graceful shutdown** (2026-05-17) — request-scoped `EventCollector` (konec race), DispatchEvents přesunut ven z transakce (dispatch až po commitu), SIGTERM drain.
- **F2 — In-process scheduler** (2026-05-17) — cron-like goroutiny s tickerem, run-once-then-tick, panic recovery per-tick; první job: cleanup expirovaných refresh tokenů.
- **F3 — Perzistentní job queue (SQLite)** (2026-05-17) — atomický claim přes `UPDATE … RETURNING`, exponenciální backoff, at-least-once, mark-complete v handler tx, worker pool.
- **F4 — Hardening** (2026-05-17) — 3 kritické fixy z auditu + rate limiting, brute-force lock, audit log mimo transakci, HTTP boundary hardening, SQLite concurrency fix (`_txlock=immediate`).
- **F5 — Observability** — strukturované slog atributy se statickým lint-enforcementem + Sentry BE/FE s obohacením eventu a maskováním tajemství (2026-06-10 / 06-14). OTel je teď součástí fáze **F6** (viz **Aktuální priorita** výše).
- **F6 — Multitenancy (částečně)** — zapínatelný row-level multitenancy + platformní rovina (superadmin) v [PR #15](https://github.com/jzaplet/gokick/pull/15) + system bus pro CLI + job↔run konvergence ([PR #22](https://github.com/jzaplet/gokick/pull/22)); OTEL zbývá (viz **Aktuální priorita**). Detail: `/gk-multitenancy`, `/gk-bus`, `/gk-runs`.
