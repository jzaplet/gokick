# GK skill — template & psací pravidla

Tenhle soubor je **vzor a pravidla**, jak psát `gk-*` skills pro tenhle projekt.
Není to spustitelný skill (jen `SKILL.md` jsou skills). Když přidáváš nový
`gk-*` skill, zkopíruj strukturu níže a vyplň ji.

## Kde skill žije

`.claude/skills/gk-<topic>/SKILL.md` — jeden adresář na skill, soubor se vždy
jmenuje `SKILL.md`. Commituje se s repem, takže po `git clone` ho má každý.

## Jazyk

- **Tělo píš česky.** Vysvětlení, věty, recepty → čeština.
- **Nadpisy (`##`), kód, identifikátory a názvy souborů nech anglicky / v originále**
  (`CommandBus`, `app/domain/user/`, `make di`, `BaseRepository`…).
- Žádný žargon bez vysvětlení — skill čte i junior / non-tech člověk.

## Frontmatter (povinný)

```yaml
---
name: gk-<topic>           # = invokace: /gk-<topic>
description: <jedna věta česky — JAKÉ PROBLÉMY skill řeší>. Use when <kdy ho AI/uživatel má sáhnout>.
---
```

- `description` je nejdůležitější řádek: tahá ho rozcestník `/gk` a podle něj se
  skill spouští. Piš ho jako „řeší problém X; sáhni po něm když Y".

## Struktura těla (drž toto pořadí)

```markdown
# GK — <Lidský název konceptu>

<Jednou větou, o čem skill je.>

## What & when
- Kdy přesně po tomhle skillu sáhnout (spouštěcí scénáře, „mám problém …").
- Čeho se NEtýká (odkaž na sousední skill).

## For non-tech / juniors
Vysvětli koncept prostým jazykem, ideálně analogií. Žádné předpoklady. Cíl:
junior po přečtení chápe, PROČ to existuje a K ČEMU to je.

## How it works
Jak to v gokicku reálně funguje — konkrétní vzory, konvence, cesty k souborům
a klíčové symboly. Krátké code snippety, kde pomůžou. Tohle je destilát z docs +
kódu, ne obecná teorie.

## Recipe
Krok-za-krokem „jak udělám X" (číslovaný checklist). Když má skill víc úkolů,
udělej víc receptů (`### Recipe: <úkol>`).

## Invariants & pitfalls
- Co se NESMÍ porušit (invarianty, které drží projekt).
- Časté chyby a jak se jim vyhnout.

## Related
- Sousední skills: `/gk-<x>`, `/gk-<y>`
- Docs: <slim docs stránka, pokud existuje>
- Kód: `app/...`, `assets/...`
```

## Pravidla obsahu

- **Pravdivost > marketing.** Každé tvrzení musí sedět na reálný kód (cituj cestu).
  Nepiš o roadmap/budoucích věcech (OTel zatím není) jako o hotových.
- **Stručnost.** Skill je referenční tahák, ne učebnice. Když roste nad ~6 sekcí,
  zvaž rozdělení na dva skills.
- **Odkazuj, neduplikuj.** Sdílené koncepty zmiň jednou a odkaž (`Related`),
  ať se obsah nerozjíždí mezi skilly.
- **Invarianty z `CLAUDE.md` a `docs/` přenes věrně** — to jsou tvrdá pravidla
  projektu (např. „command/query musí deklarovat permission", „`r.Conn(ctx)`
  v repozitářích", „žádné raw permission stringy na FE").

## Mini-příklad (zkrácený)

```markdown
---
name: gk-scheduler
description: Periodické úlohy uvnitř serveru (cron-like). Use when chceš spustit něco opakovaně na pozadí (cleanup, sync) bez externího cronu.
---

# GK — In-process scheduler

Spouštění periodických úloh přímo v `serve` procesu, bez externího cronu.

## What & when
- Sáhni sem, když potřebuješ něco opakovaně (každou hodinu/minutu) — cleanup,
  sync, housekeeping. Pro práci, co musí přežít restart/crash, použij `/gk-jobs`.

## For non-tech / juniors
Scheduler je „budík", co uvnitř aplikace každých N vteřin/minut zavolá tvoji
funkci. Běží jako součást serveru — žádný OS cron, žádná další služba.

## How it works
`app/infrastructure/scheduler/scheduler.go`: `NewScheduler(logger, []Job{...})`
+ `Run(ctx)`. Každý `Job` (name, interval, fn) běží ve vlastní goroutině,
run-once-then-tick, panika v jednom jobu neshodí ostatní. …

## Recipe
1. Napiš funkci `func(ctx) error`.
2. Zaregistruj `Job{Name, Interval, Fn}` v `provideSchedulerJobs` (DI).
3. `make di` → `make serve`.

## Invariants & pitfalls
- Jména jobů musí být unikátní (constructor to validuje).
- Fn musí být idempotentní — při restartu se spustí hned (run-once-then-tick).

## Related
- `/gk-jobs` (perzistentní fronta), `/gk-config` (DI registrace)
- Kód: `app/infrastructure/scheduler/`
```
