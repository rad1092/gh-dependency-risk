# Changelog

All notable changes to `gh-dep-risk` will be documented in this file.

## Unreleased

- Hardened Bun local fallback edge cases around JSONC parsing, direct lockfile
  matching, binary lockfile handling, checksum gating, and source protocols.
- Added narrow Bun local fallback for direct `package.json` declarations
  matched to text `bun.lock` entries when Dependency Review is unavailable.
- Hardened Yarn Berry local fallback edge cases around ambiguous direct
  lockfile matching, protocol/source notes, checksum/nodeLinker note gating,
  and descriptor parsing.
- Added narrow Yarn Berry / modern Yarn local fallback for direct
  `package.json` declarations matched to modern `yarn.lock` entries when
  Dependency Review is unavailable.
- Hardened Go modules local fallback edge cases around direct/indirect-only
  changes, version-specific `replace` directives, and `go.sum` evidence notes.
- Added Go modules local fallback for static `go.mod` `require`/`replace`
  changes, with `go.sum` checksum evidence notes only when Dependency Review is
  unavailable.
- Hardened `uv.lock` local fallback edge cases around direct resolved-version
  and source updates and unsupported source shapes.
- Added PEP 621 `pyproject.toml` local fallback enrichment from matching
  direct `uv.lock` resolved versions and source metadata when Dependency Review
  is unavailable.
- Hardened Poetry local fallback edge cases around direct `poetry.lock` resolved
  version updates and transitive-only lockfile package changes.
- Added Poetry `pyproject.toml` local fallback with optional `poetry.lock`
  direct resolved-version enrichment when Dependency Review is unavailable.
- Added Python direct dependency local fallback for `requirements.txt` and PEP
  621 `pyproject.toml` declarations when Dependency Review is unavailable.
- Updated public badge, smoke-test, and release documentation to distinguish
  the current `rad1092/gh-dependency-risk` repository slug from the stable
  `rad1092/gh-dep-risk` install path that keeps the command as `gh dep-risk`.
- Documented the owned live smoke matrix for npm, pnpm workspace, Yarn
  standalone, local comment-upsert verification against
  `rad1092/gh-dep-risk-smoke-comments`, and remote comment-upsert verification
  from the comment smoke repository's own workflow.
- Added repository hygiene coverage so workflow install commands keep the stable
  `gh dep-risk` command and badges keep the current repository slug.
- Kept the manual workflow on repository-scoped `GITHUB_TOKEN` and made it
  refuse cross-repo comment mode instead of requiring a PAT secret.
- Allowed comment mode to recognize the GitHub Actions integration identity as
  `github-actions[bot]` when `GITHUB_TOKEN` cannot call the `/user` endpoint.
- Replaced the older live-smoke target with dedicated matrix and comment smoke
  repositories: `gh-dep-risk-smoke-matrix` and `gh-dep-risk-smoke-comments`.
- Aligned the public smoke fixture README/About surfaces and marked the older
  `dep-risk-live-e2e` repository as superseded by the dedicated smoke repos.

## v0.1.8

- Fixed local fallback against large repository lockfiles fetched from the
  GitHub contents API by following blob-backed `encoding: none` responses for
  files such as large `yarn.lock` and `pnpm-lock.yaml`.
- Marked local npm and Yarn file/workspace packages consistently during local
  fallback so local targets are not treated like registry packages for target
  traversal or publish-age lookups.
- Tightened the public scope wording to distinguish GitHub dependency-review
  ecosystem coverage from the narrower local fallback support matrix.

## v0.1.7

- Expanded the dependency-review path to surface mixed-ecosystem pull requests
  for Cargo, Composer, Go modules, Maven, npm, pip, pnpm, Poetry, RubyGems,
  Swift Package Manager, and Yarn without silently narrowing results to npm or
  pnpm.
- Added narrow Yarn local fallback support for classic `yarn.lock` projects,
  workspaces, and nested standalone targets while failing honestly for likely
  Yarn Berry / Plug'n'Play cases that cannot be analyzed faithfully in this
  release.
- Introduced an explicit ecosystem-aware normalization layer for dependency
  review data and ecosystem-aware target selection without changing the stable
  JSON schema, CLI shape, or exit code meanings.

## v0.1.6

- Added repo-local `.gh-dep-risk.yml` config support for `gh dep-risk pr` with
  explicit CLI-over-config precedence and repeatable `path` handling.
- Improved reviewer-facing human and markdown output with clearer `Why risky`
  summaries and more operational recommended actions.
- Added pnpm support for root projects, shared-lockfile workspaces, and nested
  standalone targets while preserving the stable JSON schema and existing exit
  code behavior.
- Clarified actual workflow behavior for cross-repo private targets: a manual
  workflow run can fail before comment upsert if `GITHUB_TOKEN` cannot read the
  target PR repository.

## v0.1.4

- Replaced the outdated `npm-only MVP` wording with `npm-only scope` across the
  repo guidance and primary product docs.
- Reviewed the Markdown docs against the current release/install behavior and
  kept the existing release history intact.

## v0.1.3

- Restored the `gh-` repository path in docs and the `install-smoke` workflow
  so remote `gh extension install` continues to work.
- Polished the release and smoke-test documentation to match the current
  install, workflow, and verification flow.

## v0.1.2

- Hardened large-repo target discovery so truncated Git tree responses no
  longer silently drop npm targets.
- Fixed dependency-review auth classification and workflow input handling so
  auth and permission failures surface consistently and safely.
- Deduplicated aggregate transitive dependency counts across workspace targets
  that share a root lockfile.

## v0.1.1

- Added the cross-platform `install-smoke` workflow to validate released
  extension install and execution on Linux, macOS, and Windows.
- Extended install smoke coverage to run `gh dep-risk pr` against a public test
  PR instead of checking version output only.
- Documented that workflow-driven cross-repo comment mode can be blocked by the
  scope of `GITHUB_TOKEN`, which surfaces as exit code `4`.

## v0.1.0

- Added `gh dep-risk pr` for on-demand npm dependency risk summaries on GitHub
  pull requests.
- Added human, JSON, and markdown output formats with Korean as the default
  language.
- Added `--comment` marker-comment upsert behavior using PR timeline issue
  comments.
- Added `--fail-level` support with deterministic exit codes for CI and workflow
  gating.
- Added best-effort npm registry publish-age checks with `--no-registry` opt
  out.
- Added npm workspace and nested standalone subproject support with `--path`
  and `--list-targets`.
- Added reusable output bundle generation and a manual GitHub Actions workflow
  for no-local-install runs.
- Added precompiled release workflow support for GitHub CLI extension installs.
