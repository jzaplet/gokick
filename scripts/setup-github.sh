#!/usr/bin/env bash
#
# One-time GitHub bootstrap for a repo created from the gokick template.
#
# The template carries every FILE (workflows, hooks config, release-please
# config), but two things are repo SETTINGS and don't copy — and release-please
# is dead until they're set. This applies both, idempotently:
#
#   1. Actions permissions — GITHUB_TOKEN write access + "allow Actions to create
#      PRs". Without it release-please can't open its release PR or push tags.
#   2. Branch ruleset on the default branch — require a PR, block force-push and
#      deletion. The "lighter" profile: NO required status checks, on purpose, so
#      the release PR (opened by GITHUB_TOKEN, which can't trigger checks) is never
#      stuck waiting on one.
#
# Optionally resets .release-please-manifest.json to a starting version (the
# template ships gokick's own version; a fresh project wants its own baseline).
#
# Prereq: the GitHub CLI, authenticated with admin on the repo — `gh auth login`.
# Run once, from inside the new repo's checkout: `make setup-github`.
set -euo pipefail

assume_yes=0
force=0
reset_version=""

usage() {
    cat <<'USAGE'
Usage: scripts/setup-github.sh [--yes] [--force] [--reset-version X.Y.Z]

  --yes                  skip the confirmation prompt
  --force                run even if the repo is itself a template (guards against
                         reconfiguring the gokick template by accident)
  --reset-version X.Y.Z  also rewrite .release-please-manifest.json to this version
                         (commit it via a PR afterwards — main is protected)
USAGE
}

while [ $# -gt 0 ]; do
    case "$1" in
        --yes | -y) assume_yes=1 ;;
        --force) force=1 ;;
        --reset-version) reset_version="${2:-}"; shift ;;
        --reset-version=*) reset_version="${1#*=}" ;;
        -h | --help) usage; exit 0 ;;
        *) echo "unknown flag: $1" >&2; usage >&2; exit 2 ;;
    esac
    shift
done

command -v gh >/dev/null 2>&1 || { echo "✖ the GitHub CLI (gh) is required — https://cli.github.com" >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "✖ not logged in — run: gh auth login" >&2; exit 1; }

repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
is_template="$(gh repo view --json isTemplate --jq .isTemplate)"
default_branch="$(gh repo view --json defaultBranchRef --jq .defaultBranchRef.name)"
# UPPERCASE: PUBLIC | PRIVATE | INTERNAL. Drives the deploy-pull hint at the end:
# a non-public repo publishes a GHCR package that isn't anonymously pullable.
visibility="$(gh repo view --json visibility --jq .visibility)"

echo "Repo:           $repo"
echo "Default branch: $default_branch"
echo

# The template itself is marked isTemplate:true; a repo created FROM it is not.
# Refuse on a template so `make setup-github` can't clobber the origin's config.
if [ "$is_template" = "true" ] && [ "$force" -ne 1 ]; then
    echo "✖ $repo is itself a template repository — this script is for a repo CREATED"
    echo "  from the template, not the template. Pass --force if you really mean it."
    exit 1
fi

if [ "$assume_yes" -ne 1 ]; then
    printf 'Apply Actions permissions + branch ruleset to %s? [y/N] ' "$repo"
    read -r ans
    case "$ans" in
        y | Y | yes | YES) ;;
        *) echo "aborted."; exit 1 ;;
    esac
fi

echo "→ Enabling Actions write + create-PR permissions…"
gh api --method PUT "repos/$repo/actions/permissions/workflow" \
    -f default_workflow_permissions=write \
    -F can_approve_pull_request_reviews=true >/dev/null
echo "  ✓ done"

echo "→ Setting merge strategy (squash-only, PR title = commit)…"
# Squash-only keeps release-please's changelog clean: each PR lands as exactly
# ONE conventional commit (its title), so nothing is double-counted the way a
# merge commit that carries the PR title in its body is. title=PR_TITLE makes
# that commit deterministic; message=COMMIT_MESSAGES preserves the bodies +
# Co-authored-by trailers. delete_branch_on_merge tidies merged branches.
# The PR title is now the release-critical Conventional Commit — commitlint.yml
# lints it (in addition to the commits).
gh api --method PATCH "repos/$repo" \
    -F allow_squash_merge=true \
    -F allow_merge_commit=false \
    -F allow_rebase_merge=false \
    -f squash_merge_commit_title=PR_TITLE \
    -f squash_merge_commit_message=COMMIT_MESSAGES \
    -F delete_branch_on_merge=true >/dev/null
echo "  ✓ done"

if gh api "repos/$repo/rulesets" --jq '.[].name' 2>/dev/null | grep -qx "main"; then
    echo "→ Branch ruleset 'main' already exists — skipping."
else
    echo "→ Creating branch ruleset 'main' (require PR, block force-push + deletion)…"
    gh api --method POST "repos/$repo/rulesets" --input - >/dev/null <<'JSON'
{
  "name": "main",
  "target": "branch",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["~DEFAULT_BRANCH"], "exclude": [] } },
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": false
      }
    }
  ]
}
JSON
    echo "  ✓ done"
fi

if [ -n "$reset_version" ]; then
    echo "→ Resetting .release-please-manifest.json to $reset_version…"
    printf '{\n  ".": "%s"\n}\n' "$reset_version" > .release-please-manifest.json
    echo "  ✓ file updated locally — commit it via a PR (main is now protected)."
fi

echo
echo "Done. Remaining, if you haven't:"
echo "  • Hooks    — run 'make install' to wire the commit-msg + pre-push hooks locally."
echo "  • Merges   — PRs are squash-only now; the PR TITLE becomes the commit on main,"
echo "               so write it as a Conventional Commit (feat: … / fix: …) — its type"
echo "               drives the release. See CONTRIBUTING.md."
if [ -n "$reset_version" ]; then
    echo "  • Version  — manifest set to $reset_version; commit it via a PR."
else
    # Loud on purpose: the silent failure mode is running this without
    # --reset-version and shipping your first release on gokick's version line.
    current_version="$(sed -n 's/.*"\.": *"\([^"]*\)".*/\1/p' .release-please-manifest.json 2>/dev/null || true)"
    echo "  • Version  — ⚠  NOT reset: .release-please-manifest.json still reads \"${current_version:-?}\""
    echo "               (gokick's version). Your first release would continue that line."
    echo "               Re-run with ARGS=\"--reset-version 0.1.0\", or edit the file + commit via a PR."
fi
echo "  • Publish  — to push release images to GHCR: gh variable set RELEASE_PUSH --body true"
echo "               (optional Sentry source maps: SENTRY_ORG / SENTRY_PROJECT vars +"
echo "               SENTRY_AUTH_TOKEN secret — full recipe in the /gk-sentry skill)."
if [ "$visibility" != "PUBLIC" ]; then
    # Push side needs no secret (GITHUB_TOKEN has packages:write); the pull side does.
    echo "  • Deploy   — ⚠  this repo is $visibility → the GHCR image package it publishes is NOT"
    echo "               anonymously pullable. Your deploy target (e.g. Dokploy) needs registry auth:"
    echo "               either flip the package public (Package → Package settings → visibility), or"
    echo "               give the target a GitHub PAT with read:packages. Full recipe: /gk-deploy."
fi
