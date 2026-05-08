# gh-dep-risk

[![test](https://github.com/rad1092/gh-dependency-risk/actions/workflows/test.yml/badge.svg)](https://github.com/rad1092/gh-dependency-risk/actions/workflows/test.yml)
[![install-smoke](https://github.com/rad1092/gh-dependency-risk/actions/workflows/install-smoke.yml/badge.svg)](https://github.com/rad1092/gh-dependency-risk/actions/workflows/install-smoke.yml)

`gh-dep-risk` is a precompiled GitHub CLI extension that reviewers run on
demand to summarize dependency risk in pull requests.

It is an extension instead of a server so it can reuse `gh` authentication,
stay on the reviewer's machine or in a one-off workflow run, and avoid any
webhook, queue, database, or dashboard infrastructure.

![gh-dep-risk animated terminal demo](docs/assets/demo.gif)

The animated terminal capture above comes from a live E2E PR using Yarn local
fallback. An asciinema-compatible recording is also checked in at
[docs/assets/demo.cast](docs/assets/demo.cast).

## Scope

- on-demand pull request dependency review through GitHub Dependency Review
- local fallback support matrix:
  - npm: `package.json`, `package-lock.json`
  - pnpm: `package.json`, `pnpm-lock.yaml`
  - pnpm workspace discovery: `pnpm-workspace.yaml`
  - yarn: `package.json`, `yarn.lock` (narrow Yarn Classic / node_modules fallback only)
  - Python: `requirements.txt`, PEP 621 `pyproject.toml`, and Poetry
    `pyproject.toml` direct dependency declarations; Poetry may use
    `poetry.lock` to enrich direct resolved versions, with no resolver or full
    transitive analysis
- one Go binary
- dependency review API first, local fallback only when dependency review is
  unavailable
- no server, webhook receiver, GitHub App, DB, queue, dashboard, bun, or
  broad non-JS local fallback beyond the narrow Python direct-declaration and
  Poetry direct lockfile fallback in this release

Dependency Review ecosystems surfaced in this release when GitHub provides
them:

- Cargo
- Composer
- Go modules
- Maven
- npm
- pip
- pnpm
- Poetry
- RubyGems
- Swift Package Manager
- Yarn

## Install

### Authenticate first

```bash
gh auth login
```

`go-gh` also respects:

- `GH_TOKEN`
- `GITHUB_TOKEN`
- `GH_REPO`
- `GH_HOST`

`GH_REPO=OWNER/REPO` is useful outside a git checkout. `GH_HOST` is useful for
GitHub Enterprise.

### Install from GitHub

```bash
gh extension install rad1092/gh-dep-risk
```

Upgrade later with:

```bash
gh extension upgrade dep-risk
```

### Install locally from this repo

Linux or macOS:

```bash
go build -o gh-dep-risk .
gh extension install .
```

Windows PowerShell:

```powershell
go build -o gh-dep-risk.exe .
gh extension install .
```

This repo does not install itself automatically. Build the binary at the
repository root first, then run `gh extension install .` manually.

The installed command remains `gh dep-risk`.

The repository itself also needs the `gh-` prefix because GitHub CLI extension
install requires remote extension repositories to start with `gh-`.

The public repository slug is `gh-dependency-risk` for readability, but the
stable install path intentionally remains `rad1092/gh-dep-risk` so GitHub CLI
registers the command as `gh dep-risk`. Installing the readability slug directly
registers the longer command name `gh dependency-risk`.

The checkout directory name must still start with `gh-` for local extension
install to work, so use a local folder such as `gh-dep-risk` when you clone
for extension testing.

## Commands

```bash
gh dep-risk pr 123
gh dep-risk pr https://github.com/OWNER/REPO/pull/123
gh dep-risk pr --format json
gh dep-risk pr 123 --list-targets
gh dep-risk pr 123 --path apps/web
gh dep-risk pr 123 --path package.json --comment
gh dep-risk pr --comment=false
gh dep-risk pr --bundle-dir ./dep-risk-bundle
gh dep-risk pr --comment
gh dep-risk pr --fail-level high
gh dep-risk version
gh dep-risk version --json
```

Typical live checks against owned fixture repositories:

```bash
gh dep-risk pr 3 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 1 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 2 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 1 --repo rad1092/gh-dep-risk-smoke-comments --lang en --comment --no-registry
```

The matrix repository covers read-only npm, pnpm, and Yarn analysis. The
comments repository is the only live example that intentionally writes a PR
timeline comment; it may contain one marker comment per authenticated identity
such as `rad1092` locally and `github-actions[bot]` from Actions.

Command shape:

- `gh dep-risk pr [<number>|<url>]`
- `gh dep-risk version`

If the PR argument is omitted, `gh dep-risk pr` resolves the PR for the current
branch.

## Flags

### `gh dep-risk pr`

- `--repo owner/repo`
- `--format human|json|markdown`
- `--lang ko|en`
- `--comment`
- `--fail-level low|medium|high|critical|none`
- `--no-registry`
- `--bundle-dir <dir>`
- `--path <repo-relative-dir-or-manifest>` repeatable
- `--list-targets`

### `gh dep-risk version`

- `--json`

## Config File

`gh dep-risk pr` also reads a repo-local config file named
`.gh-dep-risk.yml` from the current working directory when it exists.

Supported keys:

- `lang: ko|en`
- `fail_level: none|low|medium|high|critical`
- `comment: true|false`
- `path: apps/web` or `path: [apps/web, package.json]`
- `no_registry: true|false`

Example:

```yaml
lang: en
fail_level: high
comment: true
path:
  - apps/web
  - package.json
no_registry: false
```

Precedence rules:

- CLI flags override config values
- config values override built-in defaults
- an explicit CLI `--path` replaces config `path`
- explicit boolean CLI overrides such as `--comment=false` and
  `--no-registry=false` override a config value of `true`

Unknown config keys are rejected with a clear error that includes the config
file path. A missing config file is ignored.

## What It Looks Like

These examples are checked in under [docs/examples](docs/examples) and are
derived from deterministic fixtures, render tests, and fixture-backed app
tests.

### Human output

```text
Repository: owner/repo
PR: #123 Update dependencies
Score: 48 (high)
Blast radius: medium
Dependency review available: false
Why risky: left-pad crosses a major version boundary and declares an install script.
```

### Markdown comment

```markdown
<!-- gh-dep-risk -->
## gh-dep-risk
- Repository: `owner/repo`
- PR: [#123](https://github.com/owner/repo/pull/123) Update dependencies
- Score: `48` (`high`)
- Why risky: left-pad crosses a major version boundary and declares an install script.
```

### JSON output

```json
{
  "repo": "owner/repo",
  "score": 48,
  "level": "high",
  "blast_radius": "medium",
  "dependency_review_available": false
}
```

## Output Formats

- `human`: concise reviewer-oriented summary
- `json`: stable machine-readable schema with repo, PR metadata, score, level,
  blast radius, dependency review availability, summary bullets, recommended
  actions, notes, detailed changes, and a `targets` array
- `markdown`: comment-ready output that always starts with
  `<!-- gh-dep-risk -->`

English is the default language. Use `--lang ko` for Korean.

`--bundle-dir` writes:

- `dep-risk-human.txt`
- `dep-risk.json`
- `dep-risk.md`
- `metadata.json`

When multiple targets are analyzed, the bundle also includes:

- `targets/<safe-target-name>/dep-risk.json`
- `targets/<safe-target-name>/dep-risk.md`

## Scoring Model

The score model stays heuristic, deterministic, and intentionally auditable.

- each dependency change is scored from named risk drivers with fixed weights
- the overall PR score is the highest single-change score plus a small capped
  bonus for additional risky changes
- this keeps the main driver explainable while still reflecting multi-target
  or multi-change PRs without turning the score into an opaque sum

## Fallback Matrix

| Ecosystem / manager | With GitHub Dependency Review | Without GitHub Dependency Review |
| --- | --- | --- |
| npm | Dependency Review data is used when available. | Local fallback analyzes `package.json` and `package-lock.json`. |
| pnpm | Dependency Review data is used when available. | Local fallback analyzes `package.json`, `pnpm-lock.yaml`, and `pnpm-workspace.yaml` discovery. |
| Yarn Classic | Dependency Review data is used when available. | Local fallback analyzes `package.json` and classic `yarn.lock`. |
| Yarn Berry / PnP | Dependency Review data may be surfaced when GitHub provides it. | Local fallback fails honestly when the lockfile cannot be analyzed faithfully. |
| Python `requirements.txt` / PEP 621 `pyproject.toml` / Poetry `pyproject.toml` + `poetry.lock` | Dependency Review data is used when available. | Local fallback compares direct dependency declarations and can use `poetry.lock` to enrich direct resolved versions. No resolver, broad lockfile support, or full transitive analysis. |
| Cargo, Composer, Go modules, Maven, RubyGems, SwiftPM | Dependency Review data may be surfaced when GitHub provides it. | No local fallback in this release. |

## Behavior

`gh dep-risk pr` resolves the repository from `GH_REPO` or the current git
remote, fetches PR metadata, and prefers GitHub dependency-review data when it
is available. Repository-tree discovery remains in use for local fallback,
`--list-targets`, and path validation.

Supported target shapes:

- dependency-review targets for Cargo, Composer, Go modules, Maven, npm, pip,
  pnpm, Poetry, RubyGems, SwiftPM, and Yarn
- npm root projects with `package.json` and `package-lock.json`
- npm workspaces with a shared root `package-lock.json`
- pnpm root projects with `package.json` and `pnpm-lock.yaml`
- pnpm workspaces with `pnpm-workspace.yaml` and a shared root `pnpm-lock.yaml`
- Yarn root projects with `package.json` and `yarn.lock`
- Yarn workspaces discovered from `package.json` workspaces and a shared root
  `yarn.lock`
- nested standalone subprojects with their own `package.json` and either
  `package-lock.json`, `pnpm-lock.yaml`, or `yarn.lock`
- Python `requirements.txt` direct dependency declarations
- PEP 621 `pyproject.toml` direct dependencies from `[project].dependencies`
  and `[project.optional-dependencies]`
- Poetry `pyproject.toml` direct dependencies from `[tool.poetry.dependencies]`,
  `[tool.poetry.dev-dependencies]`, and `[tool.poetry.group.<name>.dependencies]`,
  with optional `poetry.lock` direct resolved-version enrichment

### Mixed ecosystems and JS workspaces

Default behavior:

- if one supported target changed, `gh-dep-risk` analyzes that target
- if multiple supported targets changed, `gh-dep-risk` analyzes all of them and
  emits one aggregate result plus per-target detail
- if no supported target changed, the command exits with code `2`

Useful examples:

```bash
gh dep-risk pr 123 --list-targets
gh dep-risk pr 123 --path apps/web
gh dep-risk pr 123 --path package.json --comment
gh dep-risk pr 123 --bundle-dir ./out
```

Notes:

- `--path` accepts either an exact manifest path or an owning directory when
  that directory maps to exactly one detected target, and can be repeated
- `--list-targets` prints a readable target list, validates any `--path`
  filters, and exits without running PR file analysis or dependency review
- npm workspaces reuse the shared root `package-lock.json`
- pnpm workspaces reuse the shared root `pnpm-lock.yaml` and use
  `pnpm-workspace.yaml` package globs for discovery
- Yarn local fallback supports classic `yarn.lock` installs only
- likely Yarn Berry / Plug'n'Play lockfiles are detected and reported as an
  unsupported local-fallback case instead of being analyzed inaccurately
- large lockfiles served by the GitHub contents API without inline content are
  still fetched through the corresponding blob object instead of failing early
- if a lockfile-only workspace change cannot be mapped exactly, the report calls
  out that attribution is approximate instead of failing
- if both `package-lock.json` and `pnpm-lock.yaml` exist for the same target
  directory, `gh-dep-risk` will only auto-pick one when exactly one lockfile is
  clearly changed in the PR; otherwise it returns an ambiguity error and tells
  you to narrow the target or remove the unused lockfile
- if dependency review is unavailable and the selected target belongs to an
  ecosystem without local fallback support in this release, the command returns
  a clear actionable error instead of pretending to analyze it
- Python local fallback is declaration-oriented: unsupported requirement
  includes, constraints, editable installs, unsupported Poetry dependency
  shapes, dependency groups outside the Poetry direct subset, and `uv.lock` are
  not resolved in this phase
- `poetry.lock` support is limited to enriching direct Poetry dependencies with
  resolved versions and source metadata; it does not reconstruct a full
  transitive dependency graph
- if a direct Poetry dependency declaration is unchanged but the matching
  `poetry.lock` resolved version changes, local fallback reports that direct
  dependency as updated
- `poetry.lock` package changes without a matching direct dependency declaration
  in `pyproject.toml` are treated as transitive-only and do not create report
  changes
- out of scope for now: bun, `package.json5`, `package.yaml`, pnpm catalogs,
  pnpm branch lockfiles, broad non-JS local fallback, Go module local fallback,
  and full Yarn Plug'n'Play graph resolution

If dependency review returns `403` or `404`, `gh-dep-risk` falls back to
supported local fallback analysis and explicitly reports
`dependency_review_available=false`. Registry publish-age lookups are best
effort and are skipped with `--no-registry`, but API-provided release-age
signals remain available when GitHub already supplies them.

If there is no meaningful supported dependency change, the command exits with
code `2`.

### Comment upsert rules

`--comment` uses PR timeline issue comments, not review comments.

The marker comment is:

```html
<!-- gh-dep-risk -->
```

Behavior:

- exactly one marker comment owned by the authenticated user is maintained
- if multiple own marker comments exist, the newest is updated and older own
  duplicates are deleted
- another author's marker comment is never edited or deleted
- if another author already has a marker comment, `gh-dep-risk` warns on stderr
  and only manages the current user's own comment

## Version Metadata

`gh dep-risk version` prints human-readable build metadata. Release-quality
builds inject:

- `version`
- `commit`
- `date`

Example:

```bash
gh dep-risk version
gh dep-risk version --json
```

Local `go build` still works with safe defaults. Release-quality binaries should
use ldflags, or the provided `Makefile`, so the version command does not report
only `dev`.

## Exit Codes

- `0` success
- `1` general error
- `2` no supported dependency change found
- `3` final score meets or exceeds `--fail-level`
- `4` authentication required or insufficient permissions

## Local Development

Run tests:

```bash
go test ./...
```

Build with local defaults:

```bash
go build -o gh-dep-risk .
./gh-dep-risk version
```

Build with explicit metadata:

```bash
go build -ldflags "-s -w -X gh-dep-risk/cmd.version=dev-local -X gh-dep-risk/cmd.commit=$(git rev-parse --short HEAD) -X gh-dep-risk/cmd.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o gh-dep-risk .
```

Or use the `Makefile`:

```bash
make test
make build VERSION=dev-local
```

Windows PowerShell:

```powershell
$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$commit = git rev-parse --short HEAD
go build -ldflags "-s -w -X gh-dep-risk/cmd.version=dev-local -X gh-dep-risk/cmd.commit=$commit -X gh-dep-risk/cmd.date=$date" -o gh-dep-risk.exe .
.\gh-dep-risk.exe version --json
```

Local extension install remains manual:

```bash
gh extension install .
```

## Run Without Local Install

You can run the existing CLI engine from GitHub Actions without installing the
extension locally.

### From the Actions tab

Use the `dep-risk-manual` workflow and provide:

- `pr`: required PR number or full PR URL
- `repo`: optional repository override
- `lang`
- `fail_level`
- `comment`
- `no_registry`

`comment=true` is limited to PRs in this repository. For remote comment-upsert
smoke, run the target smoke repository's own workflow instead so its
repository-scoped `GITHUB_TOKEN` writes to its own PR.

The workflow file must exist on the default branch for the **Run workflow**
button to appear.

### From GitHub CLI

```bash
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=123
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=https://github.com/OWNER/REPO/pull/123
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=3 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh run watch

gh workflow run comment-smoke.yml --repo rad1092/gh-dep-risk-smoke-comments -f pr=1 -f source_ref=main -f no_registry=true
gh run watch --repo rad1092/gh-dep-risk-smoke-comments
```

### Workflow results

Each manual run:

- builds and tests the repo
- builds `gh-dep-risk` once with workflow metadata
- runs the CLI once
- uploads the output bundle artifact
- appends aggregate markdown output to the workflow job summary

If `comment=true`, comment ownership follows the workflow-authenticated identity.
This workflow uses only its repository-scoped `GITHUB_TOKEN`; in GitHub Actions
that identity is `github-actions[bot]`.

When the workflow is running in a different repository than the target PR,
`GITHUB_TOKEN` may not be allowed to read the target PR at all, especially for
private cross-repo targets. In that case the workflow can fail before artifact
upload. Cross-repo comment mode is intentionally refused by this workflow.

Remote comment smoke is handled by
`rad1092/gh-dep-risk-smoke-comments/.github/workflows/comment-smoke.yml`. That
workflow checks out this repository, builds the requested ref, and comments on
its own fixture PR with its own `GITHUB_TOKEN`, so no PAT secret is needed.

### Self-hosted runners

These workflows use Node 24 based GitHub Actions majors. Keep self-hosted
runners current; GitHub's Node 24 migration guidance uses Actions Runner
`v2.327.1+` as the baseline. If a self-hosted runner rejects `checkout@v5`,
upgrade the runner before using these workflows.

## Release

Push a `v*` tag to trigger `.github/workflows/release.yml`.

The release workflow:

- runs `go test ./...`
- injects version, commit, and build date metadata into binaries
- uses `cli/gh-extension-precompile@v2`
- publishes precompiled binaries for GitHub CLI extension installs

For the exact first release procedure, see [RELEASING.md](RELEASING.md).

## License

This project is licensed under the [MIT License](LICENSE).
