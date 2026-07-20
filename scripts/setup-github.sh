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

cat <<EOF

Done. Remaining, if you haven't:
  • Hooks    — run 'make install' to wire the commit-msg + pre-push hooks locally.
  • Version  — ${reset_version:+manifest set to $reset_version; commit it. }${reset_version:-edit .release-please-manifest.json to your starting version and commit it (it ships gokick's).}
  • Publish  — to push release images to GHCR: gh variable set RELEASE_PUSH --body true
               (optional Sentry: SENTRY_ORG / SENTRY_PROJECT vars + SENTRY_AUTH_TOKEN secret).
EOF
