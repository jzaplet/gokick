// Conventional Commits enforcement — the format release-please reads to compute
// the next SemVer bump and the CHANGELOG. A `feat:` is a minor bump, a `fix:` a
// patch, a `feat!:`/`BREAKING CHANGE:` a major; anything else (chore, docs,
// refactor, style, test, ci, build, perf) rides along without moving the version.
// So the commit type is not bookkeeping here — it is the release input.
//
// Enforced in two places: locally by the commit-msg hook (lefthook.yml, fast
// feedback) and in CI on every PR commit (.github/workflows/commitlint.yml, the
// gate `--no-verify` can't dodge). ESM because package.json is `type: module`.
//
// Merge commits and release-please's own commits are conventional or ignored by
// commitlint's defaults, so this needs no special-casing for them.
export default {
    extends: ['@commitlint/config-conventional'],
    rules: {
        // The type set is config-conventional's default (feat, fix, refactor,
        // style, docs, chore, test, ci, build, perf, revert) — exactly the types
        // already in this repo's history, restated so the allowed list is visible
        // at the config rather than inherited silently.
        'type-enum': [
            2,
            'always',
            ['feat', 'fix', 'refactor', 'style', 'docs', 'chore', 'test', 'ci', 'build', 'perf', 'revert'],
        ],
        // Bodies here carry the WHY at length — invariants, traps, failure
        // scenarios — and routinely quote identifiers and paths that blow past
        // 100 columns. The house style is long, wrapped prose, so the line-length
        // cap is off rather than fighting it; the subject cap below still holds.
        'body-max-line-length': [0, 'always'],
        'footer-max-line-length': [0, 'always'],
    },
};
