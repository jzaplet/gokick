# Contributing

Three conventions keep the history readable and the releases automatic: how you
**name a branch**, how you **write a commit**, and how a **release** is cut. All
three are enforced — locally by git hooks and again in CI — so you can't half-adopt
them by accident. Run `make install` once; it installs the hooks (via `lefthook`)
along with the rest of the toolchain.

## Branches

Cut every branch off `main` with one of these prefixes:

| Prefix       | For                                             |
|--------------|-------------------------------------------------|
| `feature/`   | a new capability                                |
| `fix/`       | a bug fix                                        |
| `hotfix/`    | an urgent production fix                         |
| `refactor/`  | a behaviour-preserving change                   |
| `chore/`     | tooling, deps, config                           |
| `docs/`      | documentation only                              |
| `test/`      | tests only                                       |
| `perf/`      | a performance change                            |
| `ci/`        | CI / workflow changes                           |
| `release/`   | a release-prep branch (rarely needed here)      |

Example: `git switch -c feature/tenant-usage-ledger`.

The `pre-push` hook rejects anything else before it leaves your machine; `main` is
exempt (you may push it after a local merge, though the ruleset blocks a *direct*
push server-side). `main` is the only long-lived branch — this is trunk-based
development, not Gitflow: short-lived branches, PR into `main`, merge, delete.

## Commits

Every commit message is a [Conventional Commit](https://www.conventionalcommits.org):

```
<type>(<optional scope>): <subject>

<optional body — the WHY, wrapped prose>

<optional footer — BREAKING CHANGE: …, refs>
```

Allowed types: `feat` `fix` `refactor` `style` `docs` `chore` `test` `ci` `build`
`perf` `revert`. The scope is optional and lowercase (`fix(platform):`, `feat(ui):`).

**The type is release input, not decoration** — it decides the next version:

| Commit                              | Version effect            |
|-------------------------------------|---------------------------|
| `feat: …`                           | minor bump (1.2.0 → 1.3.0) |
| `fix:` / `perf:`                    | patch bump (1.2.0 → 1.2.1) |
| `feat!:` or a `BREAKING CHANGE:` footer | major bump (1.2.0 → 2.0.0) |
| `chore` / `docs` / `refactor` / `test` / … | no bump (still in the changelog if not hidden) |

The `commit-msg` hook lints your message as you commit; the **Commitlint** CI job
re-lints every commit in a PR, so `git commit --no-verify` only defers the failure
to CI. The ruleset lives in [`commitlint.config.js`](commitlint.config.js).

## Releases

Releases are automated by [release-please](https://github.com/googleapis/release-please)
— you never hand-edit a version or a changelog.

1. Merge `feat:` / `fix:` PRs into `main` as usual.
2. release-please keeps a standing **“chore(main): release X.Y.Z” PR** open,
   recomputing the next version and rewriting `CHANGELOG.md` from the commits since
   the last release. Leave it open until you want to ship.
3. **Merging that release PR is the release.** release-please tags `vX.Y.Z`, creates
   the GitHub Release, and the tag drives the production image build (via
   `release-please.yml` → `release.yml`).

Nothing to run by hand. The version flows from the tag into the binary
(`-X main.release`) and the SPA bundle (`VITE_SENTRY_RELEASE`) → the Sentry release,
exactly as before.

**Manual escape hatch:** cutting a `v*` tag by hand off a green `main` still builds a
release image (`release.yml` also triggers `on: push: tags`) — use it only if you
need a release outside the release-please flow.

The deep mechanics (single-binary build, the multi-stage Dockerfile, GHCR publish
gating, Sentry stamping) live in the `/gk-deploy` skill.
