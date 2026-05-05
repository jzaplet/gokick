---
name: documan
description: Documentation helper for creating and editing Documan markdown files in docs/. Use when working with project documentation, creating new pages/sections, or editing existing docs.
---

# Documan Documentation Helper

You help create and edit project documentation in Documan format. Write clearly and concisely. **Always write documentation content in Czech**, regardless of the language the user communicates in.

## Workflow

### Before any edit
1. Check Documan container is running: `docker compose ps documan`
2. If not running: `make documan` (builds and starts the service)

### After every MD file change
1. `make documan-import` — syncs changes to Documan DB (runs lint internally)
2. After import, ask the user if they want to run `make documan-fix` (auto-formatting)
3. `make documan-fix` should always be run before committing

### Moving/renaming a page
1. Update the file's frontmatter: `uri`, `slug`, and `parent` to match the new location
2. If child pages exist, update their `parent` slug too
3. Search all `docs/**/*.md` for links to the old URI and update them
4. Run the standard post-edit workflow (import, then ask about fix).

### Deleting a page
1. Delete the `.md` file
2. If it's a `.list.md`, move or delete all child pages first
3. Search all `docs/**/*.md` for links to the deleted URI and remove/update them
4. Run the standard post-edit workflow (import, then ask about fix).

### Troubleshooting: duplicate URI error
If lint/import reports a duplicate URI but only one `.md` file with that URI actually exists, the Docker volume has stale data. Fix: `make documan` (full rebuild from scratch).

### Searching existing docs
Use Documan MCP tools (load via ToolSearch if needed):
1. `list_documentation_structure` — browse topic tree
2. `search_in_documentation` — semantic search
3. `read_documentation_section` — read specific section (preferred over full file)

## Frontmatter Rules

### Field order (mandatory)
```yaml
---
layout: 'page'              # 'page' for content, 'list' for .list.md index files
uri: '/section/page-name'   # absolute path, matches file path without docs/ prefix
position: 1                 # numeric ordering among siblings
slug: 'section-page-name'   # uri with hyphens instead of slashes (no leading slash)
parent: 'section'           # slug of parent .list.md (omit for root-level pages)
navTitle: 'Short Name'      # sidebar navigation label
title: 'Full Page Title'    # MUST match the # H1 heading exactly
description: 'Optional.'    # page description
---
```

### Rules
- `uri` = file path without `docs/` prefix, with leading `/`
- `slug` = uri with `-` instead of `/` (no leading hyphen)
- `parent` = slug of the parent category's `.list.md`
- `title` and `# H1` heading MUST be identical
- `.list.md` files use `layout: 'list'`, regular pages use `layout: 'page'`
- Position: use sequential integers (1, 2, 3) with gaps for future insertions
- Use **two blank lines** between major sections (Documan renders inline otherwise)
- **Links between docs:** Always use Documan URIs from frontmatter (`uri` field), never direct paths to `.md` files. Example: `[Page Title](/section/page-name)`, not `[Page Title](../section/page-name.md)`
- **Public URL:** All docs are available at `https://docs.yourdomain.dev/`. Any frontmatter `uri` maps to `https://docs.yourdomain.dev{uri}`. Use this when referencing docs outside of markdown (e.g. in chat, Jira, Slack). Only content merged to `develop` branch is published — local changes and open MRs are not visible.
- All files must be UTF-8

## Templates

### New section (folder + index)
File: `docs/my-section/.list.md`
```yaml
---
layout: 'list'
uri: '/my-section'
position: 5
slug: 'my-section'
navTitle: 'My Section'
title: 'My Section'
---

# My Section
```

### New page in section
File: `docs/my-section/my-page.md`
```yaml
---
layout: 'page'
uri: '/my-section/my-page'
position: 1
slug: 'my-section-my-page'
parent: 'my-section'
navTitle: 'My Page'
title: 'My Page Title'
description: 'What this page covers.'
---

# My Page Title

Content here...
```

### Nested subsection
File: `docs/my-section/sub-section/.list.md`
```yaml
---
layout: 'list'
uri: '/my-section/sub-section'
position: 1
slug: 'my-section-sub-section'
parent: 'my-section'
navTitle: 'Sub Section'
title: 'Sub Section'
---

# Sub Section
```

## Page Length
- If a page has more than ~5 major sections or becomes hard to scan, suggest splitting into a subsection (folder + `.list.md`) with separate pages per topic.
- Prefer many short, focused pages over one long monolithic page.

## When to Document (project guidelines)
- Complex business logic needing deeper context
- Integration knowledge not obvious from code
- Business domain complexity beyond simple code comments
- Processes useful for AI tools

## When NOT to Document
- Quality third-party docs exist (link instead)
- Content would be identical to AI-generated output
- Simple/obvious code patterns
