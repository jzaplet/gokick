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
#      deletion, and allow only rebase merges. Two profiles:
#        - default ("lighter"): NO required status checks. What a GITHUB_TOKEN-created
#          PR does is currently unpredictable — GitHub is rolling out "bot-created PRs
#          can run workflows if approved", and gokick has measured BOTH outcomes with
#          the same token (release PRs #39/#41 got runs in the approval-required state;
#          #43/#55 got none). A required context therefore either blocks the release PR
#          forever or costs a click on every release, with no way to tell in advance.
#        - --release-token PROFILE: stores the token as the RELEASE_PLEASE_TOKEN repo
#          secret so release-please's PR reliably gets checks, and THEN adds required
#          status checks + the strict (branch-up-to-date) policy. Use this on a real
#          project; it is what makes "other PRs must rebase and re-run before merging"
#          actually enforced.
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
release_token=""
want_release_token=0

usage() {
    cat <<'USAGE'
Usage: scripts/setup-github.sh [--yes] [--force] [--reset-version X.Y.Z]
                              [--release-token[=TOKEN]]

  --yes                  skip the confirmation prompt
  --force                run even if the repo is itself a template (guards against
                         reconfiguring the gokick template by accident)
  --reset-version X.Y.Z  also rewrite .release-please-manifest.json to this version
                         (commit it via a PR afterwards — main is protected)
  --release-token[=TOK]  store TOK as the RELEASE_PLEASE_TOKEN repo secret and add
                         required status checks (strict) to the ruleset. Without a
                         value, reads $RELEASE_PLEASE_TOKEN or prompts (hidden input).
                         The token needs repo permissions Contents / Pull requests /
                         Issues: read+write (classic PAT: scope `repo`).
                         NOTE: this is NOT the GHCR pull token your deploy target
                         uses — that one is separate, lives on the deploy host, and
                         only needs `read:packages`.
USAGE
}

while [ $# -gt 0 ]; do
    case "$1" in
        --yes | -y) assume_yes=1 ;;
        --force) force=1 ;;
        --reset-version) reset_version="${2:-}"; shift ;;
        --reset-version=*) reset_version="${1#*=}" ;;
        --release-token) want_release_token=1 ;;
        --release-token=*) want_release_token=1; release_token="${1#*=}" ;;
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

echo "→ Setting merge strategy (rebase-only)…"
# Rebase-only keeps release-please's changelog clean AND preserves every commit:
# a PR's commits replay onto main individually (no squash — full history for
# bisect/blame), and there is no merge commit to carry the PR title into its body
# and get double-counted. So each commit's type drives the release — keep them
# Conventional. delete_branch_on_merge tidies merged branches.
# (Why not merge commits: GitHub forbids merge_commit_title=MERGE_MESSAGE with
# message=BLANK, and every allowed combo leaves a Conventional commit in the merge
# commit that release-please counts a second time. Rebase sidesteps that entirely.)
gh api --method PATCH "repos/$repo" \
    -F allow_rebase_merge=true \
    -F allow_merge_commit=false \
    -F allow_squash_merge=false \
    -F delete_branch_on_merge=true >/dev/null
echo "  ✓ done"

# Resolve the release-please token BEFORE the ruleset is written: required status
# checks are only safe once the release PR can actually produce them.
if [ "$want_release_token" -eq 1 ] && [ -z "$release_token" ]; then
    release_token="${RELEASE_PLEASE_TOKEN:-}"
fi
if [ "$want_release_token" -eq 1 ] && [ -z "$release_token" ]; then
    # Hidden input — never echoed, never a shell-history argument.
    printf 'release-please token (Contents/PRs/Issues read+write; input hidden): '
    stty -echo 2>/dev/null || true
    read -r release_token
    stty echo 2>/dev/null || true
    printf '\n'
fi
if [ "$want_release_token" -eq 1 ] && [ -z "$release_token" ]; then
    echo "✖ --release-token given but no token supplied." >&2
    exit 2
fi

if [ -n "$release_token" ]; then
    echo "→ Storing RELEASE_PLEASE_TOKEN secret…"
    # --body -, so the token reaches gh over stdin instead of argv (argv is visible
    # in `ps` to every local user).
    printf '%s' "$release_token" | gh secret set RELEASE_PLEASE_TOKEN --repo "$repo" --body - >/dev/null
    echo "  ✓ done — release-please will use it (release-please.yml falls back to"
    echo "    GITHUB_TOKEN when the secret is absent)."
fi

if gh api "repos/$repo/rulesets" --jq '.[].name' 2>/dev/null | grep -qx "main"; then
    echo "→ Branch ruleset 'main' already exists — skipping."
else
    echo "→ Creating branch ruleset 'main' (require PR, block force-push + deletion)…"

    # allowed_merge_methods mirrors the repo settings set above. Both places, on
    # purpose: repo settings are editable by the Maintain role, rulesets only by
    # Admin — so on an org repo the ruleset is the higher bar, and merge strategy is
    # the one setting whose drift silently doubles release-please's changelog entries.
    # Extra rule block, added ONLY when a release-please token is configured: the four
    # contexts must actually be reportable on the release PR, or every release blocks.
    # strict = "branch must be up to date", which is what forces an open PR to rebase
    # and re-run after someone else merges. Contexts are the JOB `name:` values.
    checks_rule=""
    if [ -n "$release_token" ]; then
        echo "  + required status checks (strict) — release-please token is configured"
        checks_rule=',
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": true,
        "do_not_enforce_on_create": false,
        "required_status_checks": [
          { "context": "lint + test + build" },
          { "context": "durable-run E2E" },
          { "context": "Lint docs" },
          { "context": "conventional commits" }
        ]
      }
    }'
    fi

    # allowed_merge_methods mirrors the repo settings set above. Both places, on
    # purpose: repo settings are editable by the Maintain role, rulesets only by
    # Admin — so on an org repo the ruleset is the higher bar, and merge strategy is
    # the one setting whose drift silently doubles release-please's changelog entries.
    gh api --method POST "repos/$repo/rulesets" --input - >/dev/null <<JSON
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
        "required_review_thread_resolution": false,
        "allowed_merge_methods": ["rebase"]
      }
    }$checks_rule
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
echo "  • Merges   — PRs are rebase-only now; every commit lands on main as-is, so keep"
echo "               each one a Conventional Commit (feat: … / fix: …) — its type drives"
echo "               the release. See CONTRIBUTING.md."
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
