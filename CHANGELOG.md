# Changelog

All notable changes to `gh-dep-risk` will be documented in this file.

## Unreleased

- Updated public install, badge, smoke-test, and release documentation to use
  the current `rad1092/gh-dependency-risk` repository slug while keeping the
  installed command as `gh dep-risk`.
- Documented the owned live smoke matrix for npm, pnpm workspace, Yarn
  standalone, and comment-upsert verification against `rad1092/dep-risk-live-e2e`.
- Added repository hygiene coverage so old remote install paths do not drift
  back into public docs or release workflows.

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
