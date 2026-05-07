# Releasing

This document covers the release flow for `gh-dep-risk`.

## 1. Verify a clean tree

```bash
git status --short --branch
```

The tree should be clean before cutting a release tag.

## 2. Run local verification

```bash
go test ./...
go build ./...
git diff --check
```

## 3. Build a release-quality binary locally

Linux or macOS:

```bash
VERSION=vX.Y.Z
COMMIT=$(git rev-parse --short HEAD)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
go build -ldflags "-s -w -X gh-dep-risk/cmd.version=${VERSION} -X gh-dep-risk/cmd.commit=${COMMIT} -X gh-dep-risk/cmd.date=${DATE}" -o gh-dep-risk .
./gh-dep-risk version
./gh-dep-risk version --json
```

Windows PowerShell:

```powershell
$version = "vX.Y.Z"
$commit = git rev-parse --short HEAD
$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
go build -ldflags "-s -w -X gh-dep-risk/cmd.version=$version -X gh-dep-risk/cmd.commit=$commit -X gh-dep-risk/cmd.date=$date" -o gh-dep-risk.exe .
.\gh-dep-risk.exe version
.\gh-dep-risk.exe version --json
```

## 4. Run live CLI checks and optional manual workflow smoke test

Before tagging, run the local binary against real pull requests if you have
GitHub auth available. Prefer at least one smaller PR and one larger PR so the
final release is not validated only against fixtures.

If you have GitHub auth and repository access available, run:

```bash
gh workflow run .github/workflows/dep-risk-manual.yml -f pr=123
gh run watch
```

Then verify:

- the workflow summary contains the markdown report
- the uploaded artifact contains the bundle files
- comment mode behaves correctly if you tested `comment=true`
- for private cross-repo targets, verify the workflow token can read the target
  PR repository before relying on comment mode or artifact generation

If GitHub auth or repository context is unavailable, skip this step and perform
it later from the default branch after pushing.

## 5. Create and push the release tag

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

Do not create the tag until the branch you want to release is already on
`origin/main`.

## 6. Verify release assets

After the `release` workflow finishes:

- open the GitHub release page for the new tag
- verify precompiled binaries are attached for the expected platforms
- verify the generated release notes look sensible
- verify the release title matches the product name and tag

## 7. Verify remote install

Use a clean shell or another machine if possible:

```bash
gh extension install rad1092/gh-dep-risk
gh dep-risk version
gh dep-risk version --json
```

Verify the version matches the new release tag and does not report only `dev`.
The install path intentionally uses `rad1092/gh-dep-risk` so GitHub CLI keeps
the command name `gh dep-risk`; the repository page redirects to the current
`gh-dependency-risk` slug.

If you want to avoid touching your everyday GitHub CLI state, set a temporary
`GH_CONFIG_DIR` before running the install smoke.

## 8. Verify upgrade flow

After the extension is already installed:

```bash
gh extension upgrade dep-risk
gh dep-risk version
```

## 9. Verify install smoke workflow

Run the cross-platform workflow if you want an extra post-release check:

```bash
gh workflow run install-smoke.yml
gh run watch
```

## 10. Self-hosted runner note

These workflows use Node 24 based GitHub Actions majors. Keep self-hosted
runners current; Actions Runner `v2.327.1+` is the practical minimum baseline
for Node 24 based actions, and older runners should be upgraded before release
validation.
