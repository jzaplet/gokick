# A doc whose every pointer resolves

Prose citing a real file: `docs/other.md`.

A line anchor inside that file: `docs/other.md:1`.

A subtree pattern: `docs/**` and a Go package pattern `docs/...`.

A brace set of siblings: `docs/{page,other}.md`.

A symbol anchor: `docs/other.md:someFunction`.

A cross-reference to a skill that exists: `/gk-real`.

Shapes that are NOT path citations and must not be checked:
`FindAll/FindByIDAcrossTenants` (prose shorthand), `/api/v1/admin/users` (a URL),
`@/app-ui/Auth` (a TS alias), `app/domain/<ctx>/` (a placeholder),
`migrations/…_create_runs.sql` (an elided name), `https://example.com/x` (a link).

A deliberately fictional path, properly marked:

<!-- gkdoc:ignore docs/imaginary-section/.list.md — the "add your own section" tutorial invents it -->

Create `docs/imaginary-section/.list.md` to start a section.
