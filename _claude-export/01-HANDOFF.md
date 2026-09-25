# HANDOFF — Postgres adaptér pro gokick (čti jako první)

Tento balík (3 soubory v `_claude-export/` v kořeni repa) předává rozdělanou práci
jiné Claude session. Adresář je **dočasný** — po předání ho smažeme (samostatný
commit `chore: remove the claude export`, až o to uživatel požádá). Pořadí čtení:

1. `01-HANDOFF.md` (tento) — cíl, uživatel a jeho pravidla, aktuální stav, co dělat hned.
2. `02-PLAN-STATUS.md` — všechny fáze 0–7: co je hotovo (s commity/PR) a co zbývá, včetně návrhu řešení.
3. `03-TECH-NOTES.md` — architektura adaptéru, poučení/pasti, lokální prostředí, ověřovací příkazy, konvence commitů a PR.

Kanonický plán je **v repu**: `docs/framework/postgres-adapter-plan.md` (česky, ~880 řádků).
Obsahuje nálezy N1–N15, cílovou architekturu, schéma, zámky, RLS, testovací strategii,
fázový plán (sekce 9) a rozhodnutí D1–D10 (sekce 10). Po každé fázi se aktualizuje
(status nahoře, tabulka fází, sekce „Fáze N — co dopadlo jinak"). Tyhle handoff
soubory ho neduplikují — doplňují kontext, který v repu není.

---

## Projekt

- Repo: `jzaplet/gokick` (GitHub). Go 1.26, DDD 4 vrstvy + CQRS, sqlx, goose, Wire,
  go-arch-lint, golangci-lint, golines; frontend Vue 3. Pravidla projektu: `CLAUDE.md`
  v kořeni repa (čti celé) + skilly `.claude/skills/gk-*`.
- Pracovní větev: `feature/kind-planck-fk3df4` (cloud session ji má předepsanou; po
  merge PR se větev znovu založí z `origin/main` pod stejným jménem).

## Cíl celé práce

PostgreSQL 18 adaptér přepínaný `APP_DB_DRIVER=sqlite|postgres` (default `sqlite`),
aplikace **i celá Go test suite** musí běžet na obou DB se stejným chováním.
Postgres přináší zámky na úrovni řádků (místo globálního write-locku SQLite) a
multitenancy vynucenou databází (Row-Level Security).

## Uživatel — pravidla a preference (důležité)

- Komunikuje **česky**. Odpovídej česky, **stručně, bez opakování** (výslovně si
  stěžoval: „opakujeme se 3× a je to moc dlouhé"). Neodbíhej k věcem, na které se
  neptá („Stop, tohle nepatří sem" — když jsem začal číst nesouvisející soubory).
- Kód, komentáře, commity a PR popisy jsou **anglicky**; dokumentace v `docs/` a
  skilly **česky**.
- Tvrdý požadavek: **„Nikde nesmí zůstat žádná sqlite, když přepnu adapter, tak to
  klidně triple checkni hlavně v těch testech."** (Build `-tags nosqlite` nelinkuje
  SQLite vůbec; gaty to hlídají — viz 03-TECH-NOTES.)
- Uživatel sám používá jen `make build && make serve` — to musí pokrýt vše
  (Postgres kontejner se nahodí sám, když je v `.env` `APP_DB_DRIVER=postgres`).
- Rozhodnutí, která udělal (D1–D10 v plánu): UUIDv7 všude; české řazení identicky v
  obou DB; SQLite zůstává default; seedy dávají stejná data; adaptér se volí při
  založení projektu (žádná migrace dat); testy vždy na obou DB (lokálně i v CI);
  testy musí být levné; Docker: `docker/postgres/Dockerfile`, compose v kořeni,
  OrbStack domény, **žádné publikované porty**.
- Workflow: každá fáze = jeden PR s čistými Conventional Commits; uživatel PR
  mergne (nebo řekne „Mergni to" → merge metodou **rebase**) a řekne „pusť se do
  další fáze". Po PR se ho zeptej/informuj; PR zakládej, když o to požádá
  (u fáze 4 řekl „Založ PR").
- Občas spouští `/simplify` a `/code-review` a pak chce opravit nálezy („Oprav co
  není fixnuto").

## Aktuální stav (k 2026-09-25 17:00 UTC)

- Fáze 0–3: hotové a mergnuté (PR #66 mergnutý 2026-09-25 rebase-merge do `main`).
- **Fáze 4: hotová, PR [jzaplet/gokick#67](https://github.com/jzaplet/gokick/pull/67)
  otevřený**, 7 commitů na `feature/kind-planck-fk3df4` (head `773eb14`) + commit
  s tímto exportem. CI na `773eb14` celé zelené (všech 5 jobů, vč. `postgres tests`
  na PG 18), mergeable, bez review komentářů — čeká na merge uživatelem.
- Předchozí session měla na #67 odběr PR událostí a naplánovaný check-in — nová
  session je **nemá**: pokud má nástroje, přihlas se k PR (`subscribe_pr_activity`)
  a zkontroluj CI.

## Co udělat hned v nové session

1. Přečti `CLAUDE.md`, `docs/framework/postgres-adapter-plan.md` (hlavně sekce 5, 6,
   7.1, 7.7, 9) a tyhle 3 soubory.
2. Zkontroluj PR #67: CI zelené? merge conflict? review? Pokud červené → oprav
   (root-cause, ne re-run naslepo), ověř lokálně, push.
3. Až uživatel #67 mergne a řekne pokračovat: `git fetch origin main && git checkout
   -B feature/kind-planck-fk3df4 origin/main` a začni **fázi 5** (viz 02-PLAN-STATUS).
4. Na konci každé fáze: aktualizuj plán (status, tabulka fází, „Fáze N — co dopadlo
   jinak") + CLAUDE.md/skilly, které se změnou zastaraly; ověř každý commit zvlášť;
   push; PR (když o něj požádá).
