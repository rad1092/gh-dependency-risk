<!-- gh-dep-risk -->
## gh-dep-risk
- Repository: `owner/repo`
- PR: [#123](https://github.com/owner/repo/pull/123) Update dependencies
- Score: `48` (`high`)
- Blast radius: `medium`
- Dependency review available: `false`
- Why risky: left-pad crosses a major version boundary and declares an install script.

### Summary
- 1 dependency changes were detected.
- Dependency Review was unavailable, so local fallback analysis was used.
- Top risk signals: major version bump, install script.

### Notes
- Dependency review API was unavailable, so local fallback analysis was used.

### Targets
- `root` (root, ecosystem=npm, score `48`, level `high`, blast `medium`)
  - `left-pad 1.0.0 -> 2.0.0` (updated/runtime, score `48`)
    - The dependency crosses a major version boundary.
    - The package declares an install script.
  - Dependency review API was unavailable, so local fallback analysis was used.

### Recommended actions
- Inspect install scripts and published tarballs for left-pad before merging.
- Check release notes and migration guidance for left-pad before merging.

### Quick commands
- `npm ls left-pad`

