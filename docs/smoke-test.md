# Smoke Test

These smoke tests are designed for release validation. They assume you are in
the repository root and have a built binary or an installed extension.

On Windows PowerShell, use `.\gh-dep-risk.exe` instead of `./gh-dep-risk`.
If you want isolated extension-install testing, set a temporary `GH_CONFIG_DIR`
before running the install commands.

## 1. Local CLI run against a real PR

```bash
gh auth login
export GH_REPO=OWNER/REPO
go build -o gh-dep-risk .
./gh-dep-risk pr 123
```

Verify:

- the command exits `0`, `2`, `3`, or `4` as documented
- the report includes repo, PR, score, blast radius, and recommended actions
- before release, run at least one smaller PR and one larger PR instead of
  relying only on fixture-backed tests
- if the target repository has a very large lockfile, verify the run does not
  fail on a GitHub contents API `encoding: none` response

### Read-only analysis smoke

Use the owned matrix repository for repeatable read-only analysis checks. These
commands must not write PR comments:

```bash
gh dep-risk pr 3 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 1 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 2 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 4 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 5 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 6 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 7 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 8 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 9 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
gh dep-risk pr 10 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
```

Verify:

- supported fixture reports render for npm, pnpm workspace, Yarn Classic,
  Python, Poetry, uv, Go modules, Yarn Berry, and Bun text lockfiles
- JSON output includes `score`, `level`, and `dependency_review_available`
- `dependency_review_available=false` is expected for local fallback fixtures

The Bun binary lockfile fixture is unsupported-only by design:

```bash
gh dep-risk pr 11 --repo rad1092/gh-dep-risk-smoke-matrix --lang en --format json --no-registry
```

For PR `#11`, exit code `2` is expected because `bun.lockb` is not parsed and
no scored dependency change is invented.

Current owned live read-only fixture coverage:

| Repo | PR | Coverage |
| --- | --- | --- |
| `rad1092/gh-dep-risk-smoke-matrix` | `#3` | npm `package.json` + `package-lock.json` |
| `rad1092/gh-dep-risk-smoke-matrix` | `#1` | pnpm `package.json` + `pnpm-lock.yaml` with workspace discovery |
| `rad1092/gh-dep-risk-smoke-matrix` | `#2` | Yarn Classic `package.json` + `yarn.lock` |
| `rad1092/gh-dep-risk-smoke-matrix` | `#4` | Python `requirements.txt` |
| `rad1092/gh-dep-risk-smoke-matrix` | `#5` | Python PEP 621 `pyproject.toml` |
| `rad1092/gh-dep-risk-smoke-matrix` | `#6` | Poetry `pyproject.toml` + `poetry.lock` |
| `rad1092/gh-dep-risk-smoke-matrix` | `#7` | uv `pyproject.toml` + `uv.lock` |
| `rad1092/gh-dep-risk-smoke-matrix` | `#8` | Go modules `go.mod` + `go.sum` checksum evidence |
| `rad1092/gh-dep-risk-smoke-matrix` | `#9` | Yarn Berry / modern Yarn `package.json` + `yarn.lock` |
| `rad1092/gh-dep-risk-smoke-matrix` | `#10` | Bun `package.json` + text `bun.lock` |
| `rad1092/gh-dep-risk-smoke-matrix` | `#11` | Bun `bun.lockb` unsupported-only behavior |

Phase 6A validated the expanded fixtures with a local binary built from
`a8252d281537968aef65e680583e5208df4d79ce`. Installed extensions older than
that commit may not validate the newer fixture PRs until a release catches up.
For unreleased validation, use the same arguments with `./gh-dep-risk` or the
platform-specific local binary path instead of `gh dep-risk`.

### Local fallback support smoke matrix

This matrix is for release checklist coverage. Dependency Review remains the
primary path; local fallback is only used when Dependency Review is unavailable.

| Area | Local fallback smoke expectation |
| --- | --- |
| npm package-lock | Direct dependency changes from `package.json` and `package-lock.json` are reported. |
| pnpm lockfile/workspaces | `pnpm-lock.yaml` and `pnpm-workspace.yaml` discovery select root/workspace targets. |
| Yarn Classic | Classic `yarn.lock` direct dependency changes are reported. |
| Python `requirements.txt` | Direct declarations are parsed; unsupported entries stay notes-only. |
| Python PEP 621 `pyproject.toml` | `[project].dependencies` and `[project.optional-dependencies]` direct declarations are parsed. |
| Poetry | Poetry direct declarations are parsed and matching `poetry.lock` entries enrich direct resolved versions/source. |
| uv | Matching `uv.lock` entries enrich PEP 621 direct dependencies only. |
| Go modules | `go.mod` `require`/`replace` changes are reported; `go.sum` is checksum evidence only. |
| Yarn Berry / modern Yarn | Direct `package.json` declarations are matched to modern `yarn.lock`; `.yarnrc.yml` is detection/nodeLinker note-only. |
| Bun text lockfile | Direct `package.json` declarations are matched to text `bun.lock`. |
| Bun binary lockfile | `bun.lockb` is unsupported and must not create scored dependency changes. |

### Comment smoke

Comment smoke intentionally writes PR timeline issue comments. Keep it separate
from read-only analysis smoke:

```bash
./gh-dep-risk pr 1 --repo rad1092/gh-dep-risk-smoke-comments --lang en --comment --no-registry
```

Verify:

- comment mode is used only on `rad1092/gh-dep-risk-smoke-comments`
- PR `#1` has exactly one `<!-- gh-dep-risk -->` marker comment for the current
  authenticated user; marker comments owned by other users or bots are left
  untouched

## 2. Workflow dispatch run

```bash
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=123
gh run watch
```

For owned live smoke runs:

```bash
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=3 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=1 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=2 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=4 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=5 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=6 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=7 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=8 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=9 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=10 -f repo=rad1092/gh-dep-risk-smoke-matrix -f no_registry=true
gh run watch
```

PR `#11` can also be dispatched as an unsupported-only check; exit code `2` is
expected for that fixture.

For comment smoke, run the target smoke repository's own workflow:

```bash
gh workflow run comment-smoke.yml --repo rad1092/gh-dep-risk-smoke-comments -f pr=1 -f source_ref=main -f no_registry=true
gh run watch --repo rad1092/gh-dep-risk-smoke-comments
```

Verify:

- the workflow summary contains the aggregate markdown output
- the artifact includes `dep-risk-human.txt`, `dep-risk.json`, `dep-risk.md`,
  `metadata.json`
- if multiple targets changed, the artifact also includes `targets/...`
- for private cross-repo targets, verify the workflow token can read the target
  PR repository; otherwise the run can fail before artifact upload
- remote comment smoke runs inside `rad1092/gh-dep-risk-smoke-comments` so its
  repository-scoped `GITHUB_TOKEN` can write its own PR timeline comment
- the main repository manual workflow intentionally refuses cross-repo comment
  mode; it is for read-only cross-repo analysis and same-repository comments

## 3. Remote install smoke

```bash
gh extension install rad1092/gh-dep-risk --force
gh dep-risk version
gh dep-risk version --json
```

Verify:

- the install succeeds from the published release assets
- the reported version matches the latest tag
- the command does not report only `dev`
- the installed command is `gh dep-risk`, not `gh dependency-risk`

## 4. Comment mode

```bash
./gh-dep-risk pr 123 --comment
```

Verify:

- exactly one marker comment owned by the current authenticated user remains
- older duplicate comments owned by the same user are removed
- marker comments from other authors are not edited or deleted

## 5. Fail-level mode

```bash
./gh-dep-risk pr 123 --fail-level high
```

Verify:

- exit code `3` is returned only when the final score meets or exceeds the
  threshold
- the report still renders before the exit code is surfaced

## 6. Monorepo target selection

```bash
./gh-dep-risk pr 123 --list-targets
./gh-dep-risk pr 123 --path apps/web
./gh-dep-risk pr 123 --path package.json --bundle-dir ./out
```

Verify:

- `--list-targets` exits `0` and prints detected targets with clear ecosystem
  and manager context
- `--path` restricts analysis to the selected target or targets by exact
  manifest path or by owning directory when that is unambiguous
- aggregate and per-target bundle files are written when `--bundle-dir` is set
