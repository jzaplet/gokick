---
layout: 'page'
uri: '/skills/documan'
position: 10
slug: 'skills-documan'
parent: 'skills'
navTitle: 'documan'
title: 'Documan — pomocník pro dokumentaci'
description: 'Pomocník pro tvorbu a úpravu Documan markdown souborů v docs/. Use when pracuješ s dokumentací projektu, zakládáš nové stránky/sekce nebo upravuješ existující docs.'
name: 'documan'
---

# Documan — pomocník pro dokumentaci

Pomáháš vytvářet a upravovat dokumentaci projektu ve formátu Documan. Piš jasně a stručně. **Obsah dokumentace piš vždy česky**, bez ohledu na to, jakým jazykem mluví uživatel.

## Workflow

### Před každou úpravou
1. Ověř, že běží Documan container: `docker compose ps documan`
2. Když neběží: `make documan` (sestaví a nastartuje službu)

### Po každé změně MD souboru
1. `make documan-import` — promítne změny do Documan DB (uvnitř spustí lint)
2. Po importu se zeptej uživatele, jestli chce spustit `make documan-fix` (auto-formátování)
3. `make documan-fix` se má vždy spustit před commitem

### Přesun / přejmenování stránky
1. Uprav frontmatter souboru: `uri`, `slug` a `parent` podle nového umístění
2. Pokud má stránka potomky, uprav i jejich `parent` slug
3. Prohledej všechny `docs/**/*.md` na odkazy na staré URI a uprav je
4. Spusť standardní postup po úpravě (import, pak nabídni fix).

### Smazání stránky
1. Smaž `.md` soubor
2. Pokud je to `.list.md`, nejdřív přesuň nebo smaž všechny potomky
3. Prohledej všechny `docs/**/*.md` na odkazy na smazané URI a odeber/uprav je
4. Spusť standardní postup po úpravě (import, pak nabídni fix).

### Troubleshooting: duplicitní URI
Když lint/import hlásí duplicitní URI, ale na disku existuje jen jeden `.md` soubor s tím URI, má Docker volume zastaralá data. Řešení: `make documan` (kompletní rebuild od nuly).

### Hledání v existujících docs
Použij Documan MCP nástroje (v případě potřeby je načti přes ToolSearch):
1. `list_documentation_structure` — procházení stromu témat
2. `search_in_documentation` — sémantické hledání
3. `read_documentation_section` — přečtení konkrétní sekce (lepší než celý soubor)

## Pravidla frontmatteru

### Pořadí polí (povinné)
```yaml
---
layout: 'page'              # 'page' pro obsah, 'list' pro .list.md indexové soubory
uri: '/section/page-name'   # absolutní cesta, odpovídá cestě souboru bez prefixu docs/
position: 1                 # číselné řazení mezi sourozenci
slug: 'section-page-name'   # uri s pomlčkami místo lomítek (bez úvodní pomlčky)
parent: 'section'           # slug nadřazeného .list.md (u root stránek vynech)
navTitle: 'Krátký název'    # popisek v navigaci (sidebar)
title: 'Celý název stránky' # MUSÍ přesně odpovídat nadpisu # H1
description: 'Volitelné.'   # popis stránky
---
```

### Pravidla
- `uri` = cesta souboru bez prefixu `docs/`, s úvodním `/`
- `slug` = uri s `-` místo `/` (bez úvodní pomlčky)
- `parent` = slug nadřazené kategorie (`.list.md`)
- `title` a nadpis `# H1` MUSÍ být identické
- `.list.md` soubory mají `layout: 'list'`, běžné stránky `layout: 'page'`
- Position: postupná celá čísla (1, 2, 3) s mezerami pro budoucí vkládání
- Mezi hlavními sekcemi nech **dva prázdné řádky** (jinak je Documan vyrenderuje inline)
- **Odkazy mezi docs:** vždy přes Documan URI z frontmatteru (pole `uri`), nikdy přímou cestou na `.md` soubor. Příklad: `[Page Title](/section/page-name)`, ne `[Page Title](../section/page-name.md)`
- **Veřejná URL:** všechny docs jsou na `https://docs.yourdomain.dev/`. Každé `uri` z frontmatteru se mapuje na `https://docs.yourdomain.dev{uri}`. Použij to, když na docs odkazuješ mimo markdown (např. v chatu, Jira, Slacku). Publikuje se jen obsah mergnutý do větve `develop` — lokální změny a otevřené MR vidět nejsou.
- Všechny soubory musí být v UTF-8

## Šablony

### Nová sekce (složka + index)
Soubor: `docs/moje-sekce/.list.md`
```yaml
---
layout: 'list'
uri: '/moje-sekce'
position: 5
slug: 'moje-sekce'
navTitle: 'Moje sekce'
title: 'Moje sekce'
---

# Moje sekce
```

### Nová stránka v sekci
Soubor: `docs/moje-sekce/moje-stranka.md`
```yaml
---
layout: 'page'
uri: '/moje-sekce/moje-stranka'
position: 1
slug: 'moje-sekce-moje-stranka'
parent: 'moje-sekce'
navTitle: 'Moje stránka'
title: 'Název mé stránky'
description: 'Co stránka pokrývá.'
---

# Název mé stránky

Obsah stránky…
```

### Vnořená podsekce
Soubor: `docs/moje-sekce/podsekce/.list.md`
```yaml
---
layout: 'list'
uri: '/moje-sekce/podsekce'
position: 1
slug: 'moje-sekce-podsekce'
parent: 'moje-sekce'
navTitle: 'Podsekce'
title: 'Podsekce'
---

# Podsekce
```

## Délka stránky
- Pokud má stránka víc než ~5 hlavních sekcí nebo se v ní špatně orientuje, navrhni rozdělení do podsekce (složka + `.list.md`) se samostatnými stránkami po tématech.
- Radši hodně krátkých, zaměřených stránek než jednu dlouhou monolitickou.

## Narativní vzor stránky (motivace ke čtení)
Každá obsahová stránka začíná **hookem** — 1–3 věty hned pod `# H1`, **bez nadpisu** — které řeknou *co to je a co čtenáři dá, když bude číst dál*. „Proč" patří **sem**, do hooku.
- **Nikdy bare `## Proč`** (čtenář se ptá „proč co?"). Když sekce opravdu vysvětluje důvod, pojmenuj konkrétní věc: `## Proč audit mimo transakci`, `## Proč WAL` — nikdy jen `## Proč`.
- Nadpisy jsou samovysvětlující a pojmenovávají svůj obsah, ne nějaký abstraktní meta-popisek.
- Dva tvary stránek:
  - **Reference** (installation, configuration): hook → tématické sekce (tabulky proměnných, příkazy).
  - **Koncept / flow** (architecture, `*-flow`): hook → `## K čemu to je` (proč/kdy po tom sáhneš) → `## Jak to teče` / tělo → `## Příklad` → `## Související`.
- Každou stránku zakonči `## Související` — kam dál (další stránky + relevantní `/gk-*` skilly).

## Kdy dokumentovat (zásady projektu)
- Složitá business logika, která potřebuje hlubší kontext
- Integrační znalosti, které z kódu nejsou zřejmé
- Složitost business domény nad rámec prostých komentářů v kódu
- Postupy užitečné pro AI nástroje

## Kdy NEdokumentovat
- Existuje kvalitní dokumentace třetí strany (radši na ni odkaž)
- Obsah by byl identický s tím, co vygeneruje AI
- Jednoduché/zřejmé vzory v kódu
