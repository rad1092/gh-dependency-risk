# AGENTS

- mission: on-demand pull request dependency risk summary
- dependency analysis principle: GitHub Dependency Review API first, local
  fallback second only when Dependency Review is unavailable
- local fallback scope: support npm, pnpm, Yarn Classic, Yarn Berry / modern
  Yarn, Bun text `bun.lock`, Python direct declarations with Poetry and
  `uv.lock` direct enrichment, and Go modules `go.mod` with `go.sum` checksum
  evidence only
- unsupported local fallback scope: no resolver, no full transitive graph, no
  registry metadata lookup expansion, no `.pnp.cjs` parsing, and no binary
  `bun.lockb` parsing
- keep GitHub I/O in `internal/github`
- keep orchestration in `internal/app`
- keep deterministic logic in `internal/analysis`, `internal/npm`,
  `internal/python`, and `internal/gomod`
- keep rendering in `internal/render`
- keep fixtures in `testdata`
- marker comment is `<!-- gh-dep-risk -->`
- PR timeline comments must use issue comments, never review comments
- `--comment` must maintain exactly one marker comment owned by the authenticated user
- if multiple own marker comments exist, update the newest and delete older own duplicates
- never edit or delete another author's marker comment
- never build a server or web UI
- add tests whenever parser, scoring, rendering, or comment-upsert rules change
