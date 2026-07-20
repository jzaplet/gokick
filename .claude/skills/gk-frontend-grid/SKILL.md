---
layout: 'page'
uri: '/skills/gk-frontend-grid'
position: 40
slug: 'skills-gk-frontend-grid'
parent: 'skills-frontend'
navTitle: 'gk-frontend-grid'
title: 'GK — Frontend DataGrid (server-side)'
description: 'Server-side DataGrid — stránkování, filtry, řazení, výběr řádků a bulk akce; headless createGridState na FE + whitelisted/clamped list query na BE. Use when stavíš stránkovanou tabulku, řešíš filtry/pagination/sort, výběr + bulk akce, nebo jak grid stav bezpečně zpracovat na backendu.'
name: 'gk-frontend-grid'
---

# GK — Frontend DataGrid (server-side)

Stránkovaná, filtrovaná a řazená tabulka, kde **stránkuje, filtruje i řadí backend** — frontend drží jen stav a vykreslí jednu stránku.

## What & when

- Sáhni sem, když: stavíš **stránkovanou tabulku** (list uživatelů, objednávek…), přidáváš **filtry / řazení / výběr řádků + bulk akce**, nebo řešíš, **jak grid stav (page/sort/filtry) teče na BE a jak ho BE bezpečně zpracuje** (whitelist sortu, clamp stránky, filtered total).
- NEtýká se: obecné FE struktury a `app-ui` konvencí (`/gk-frontend-ui`), fetch/guard mechaniky (`/gk-frontend-fetch`), formulářů pro create/edit řádku (`/gk-frontend-forms`). Query jako obecný doménový vzor je `/gk-queries`, repo detaily `/gk-repositories`.

## For non-tech / juniors

Grid je tabulka s vyhledáváním a stránkami. **„Server-side"** znamená, že data i počet stránek počítá server, ne prohlížeč — tabulka může mít miliony řádků, ale stáhne se vždy jen jedna stránka. Když napíšeš do filtru „Anna", pošle se to na server, ten vrátí jen Anny a řekne, kolik jich je → z toho se poskládají stránky.

Srdcem je `createGridState` — „mozek" tabulky. Drží číslo stránky, řazení, filtry a výběr řádků, ale **neví nic o HTTP ani o vykreslování**: dostane funkci `load()`, kterou zavolá, kdykoli se má načíst nová stránka. Vykreslování obstará `DataGrid.vue`, samotné volání API tvůj view. Díky tomu je stejný „mozek" použitelný pro libovolnou tabulku.

## How it works

### FE — headless state (`assets/app-ui/DataGrid/createGridState.ts`)

`createGridState<F>({ defaultSort, filters, load, perPage?, syncUrl?, debounceMs? })` vrací stav (readonly refs `page`/`perPage`/`sort`/`total`/`isLoading`, reactive `filters`) + akce (`init`/`reload`/`handleSort`/`handlePageChange`/`clearFilters` + výběr):

- **`load(args)` je injektovaný** — grid neví o HTTP; view v něm zavolá `authFetch` a vrátí `{ ok: true, total } | { ok: false }`. Chybu si toastem řeší view, grid drží poslední stav.
- **Filtry jsou debounced** (default 400 ms) — psaní do search boxu nepálí request per úhoz; při změně filtru se stránka resetuje na 1.
- **Tri-state sort** — `handleSort(col)`: ASC → DESC → zpět na default.
- **Volitelný URL sync** (`syncUrl: true`) — page/sort/filtry se zrcadlí do query stringu přes `window.history.replaceState` (NE `router.replace` — je to zrcadlo stavu, ne navigace: žádný history záznam, žádný re-run guardu). Deep-link přežije reload i sdílení.
- **Dual-mode výběr** — `selected` (množina id) NEBO `allFiltered` = „všechny řádky, co matchují filtr" bez vyjmenování id (bulk pak pošle filter set, ne obří seznam id).
- **Clamp** — po `load` clampne out-of-range page (bulk delete vyprázdní poslední stránku, `?page=999` deep-link degraduje) a přefetchne.

### FE — prezentace (`assets/app-ui/`)

`DataGrid.vue` (hlavička se sort ikonkami, loading řádek, `ScrollShadow`; **řádky jsou konzumentovy přes `#rows` slot** — grid nikdy nediktuje markup buňky), `FilterPanel.vue`, `Pagination.vue`, `BulkActions/BulkActionBar.vue`. Kartu (border + Pagination uvnitř) obaluje view.

### BE — grid stav na drátě → bezpečná query

Wire → query → doména → repo, referenční vzor `admin/users`:

1. **Handler** (`app/presentation/http/handler/admin_users.go`) — `listUsersQueryFromRequest(r)` čte `?page`/`?per_page`/`?sort_by`/`?sort_dir`/`?<filtry>` jako **raw** hodnoty do `ListUsersQuery`; `bus.Query` → DTO response `{ items, total }` (gkts-anotovaný, viz `/gk-queries`).
2. **Query handler** (`app/application/user/query/list_users.go`) — `Handle` normalizuje raw stav na doménová criteria (`SortColumnFrom`, `SortDirectionFrom`, `ListCriteria{…}.Normalize()`) a zavolá `repo.FindPage`.
3. **Doména** (`app/domain/user/list.go`) — sort je **WHITELIST** (`SortColumn` konsty + `SortColumnFrom` s fallbackem na nickname), `Normalize()` clampuje page/perPage (`ListPerPageMax = 100`) a direction. Neznámý sort ani out-of-range page **nevrací 400** — je to UX preference, ne business vstup (stará deep-link se nesmí rozbít).
4. **Repo** (`app/infrastructure/sqlite/user/list.go`) — `FindPage`: tenant-scoped base + LIKE filtry (`listFilterWhere`), sort přes **mapu `listSortSQL`** (mapa JE injection guard — wire hodnota se nikdy neinterpoluje), povinný **tie-break `, id ASC`**, `LIMIT/OFFSET`; vrací `ListPage{ Items, Total }`, kde `Total` je **filtered count ze stejného WHERE** (jeden snapshot pro pager).

### BE — bulk (dual-mode)

`BulkSelection{ IDs | (AllFiltered + Filters), ExcludeID }` (`app/domain/user/list.go`) zrcadlí FE výběr: buď výčet id, nebo filter set. `ExcludeID` = aktér, **vždy vyloučen** (self-protection). Endpointy vrací `{ affected }` (`BulkResult`) — klient rozezná „N změněno" od „výběr se scvrknul na řádky, co server ušetřil".

## Recipe: nová server-side grid tabulka

1. **Doména** — `SortColumn` whitelist + `SortColumnFrom`, `ListFilters`, `ListCriteria` + `Normalize()`, `ListPage{Items,Total}` (vzor `app/domain/user/list.go`).
2. **Repo** — `FindPage(ctx, criteria) (ListPage, error)`: `sortSQL` mapa, LIKE/exact filtry, tie-break `, id ASC`, `LIMIT/OFFSET`, filtered `COUNT(*)`. Tenant scoping přes `r.Tenant(ctx)` (viz `/gk-repositories`, `/gk-multitenancy`).
3. **Query** — `List<X>Query` (raw wire pole) + `Handle` → `Normalize` → `FindPage`; `RequiredPermission()`.
4. **Handler** — `...QueryFromRequest(r)` → `bus.Query` → gkts-anotovaný `{items,total}` DTO; `make ts-gen`.
5. **FE view** — `createGridState({ defaultSort, filters, syncUrl: true, load })`; v `load` složit `URLSearchParams` (`page`, `per_page`, `sort_by`, `sort_dir` + neprázdné filtry) a zavolat `authFetch<Resp>('GET', url, { validate })` → `{ ok: true, total }`.
6. **FE template** — `columns: GridColumn[]`; `<FilterPanel>` s `Input`/`Select` bound na `grid.filters.*`; `<DataGrid :columns :sort :is-loading @sort>` s `#rows` slotem (doménový `XxxRow.vue`); `<Pagination>`; `onMounted(() => grid.init())`. Vzor: `assets/app/Admin/Views/AdminUsersView.vue`.
7. **(volitelně) bulk** — `<BulkActionBar>` + composable (vzor `assets/app-ui/BulkActions/createUsersBulk.ts`), bulk endpointy s `BulkSelection`, každá akce přes `ConfirmModal`.

## Invariants & pitfalls

- **Sort/paging nikdy nevrací 400** — neznámý sort → fallback (`SortColumnFrom`), out-of-range page → clamp (`Normalize` + FE reload clamp). UX preference, ne business vstup.
- **Sort výhradně přes whitelist mapu** (`listSortSQL`), nikdy interpolací wire hodnoty; i `SortDir` přes `SortDirectionFrom` (interpoluje se do `ORDER BY`). Tohle je injection guard.
- **Tie-break `, id ASC` je povinný** — bez sekundárního klíče mají řádky se stejnou primární hodnotou nedefinované pořadí přes hranici `LIMIT/OFFSET` → řádek se objeví na dvou stránkách nebo na žádné.
- **Filtered total v jednom snapshotu** — `COUNT(*)` i `SELECT` sdílí stejné WHERE; pager total potřebuje.
- **Prázdný filtr = vypnutý**; `active` je tri-state (`""`/`"1"`/`"0"`) — bool neumí „nezáleží".
- **Destruktivní bulk: neznámá `active` = hard error** (`ValidActiveFilter`) — tiché zahození podmínky rozšíří „smaž inactive" na „smaž všechny". Bulk **vždy vylučuje aktéra** (`ExcludeID`); `allFiltered` posílá filter set, ne výčet id.
- **FE výběr se při změně filtru čistí SYNCHRONNĚ** (ne až po debounce) — jinak `selectedCount` mluví za filtr, který už neplatí (modal „smazat 3" pak pošle prázdný filtr = „smazat vše"). V `allFiltered` módu per-row toggle **zahodí eskalaci** (clear), ne „all except" (id nejsou vyjmenované).
- **URL sync = `history.replaceState`, ne `router.replace`**; `init()` potlačí filter watcher (`urlSyncing`), ať deep-link fetchne 1× a drží stránku.
- **Žádné raw permission stringy na FE** (obecný invariant) — grid read i bulk endpointy jsou `Permissioned` na BE.

## Related

- `/gk-frontend-fetch` (`authFetch` + runtime guard), `/gk-frontend-ui` (`app-ui` struktura, přísný lint), `/gk-frontend-forms` (formuláře create/edit řádku)
- `/gk-queries` (query pattern + normalizace), `/gk-repositories` (`FindPage`, `r.Conn`/`r.Tenant`), `/gk-multitenancy` (tenant scoping u list/bulk)
- Kód: `assets/app-ui/DataGrid/`, `assets/app-ui/FilterPanel/`, `assets/app-ui/Pagination/`, `assets/app-ui/BulkActions/`, `assets/app/Admin/Views/AdminUsersView.vue`, `app/domain/user/list.go`, `app/application/user/query/list_users.go`, `app/infrastructure/sqlite/user/list.go`, `app/presentation/http/handler/admin_users.go`
