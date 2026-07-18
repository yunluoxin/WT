# Releasing wt

Releases are built by GoReleaser.

## Which workflow runs?

This directory is its own git repository. When it's pushed on its own
(or moved to a dedicated GitHub repo), the workflows in
`go/.github/workflows/` take over:

- `test.yml` — tests on push/PR (3 OS × 2 Go versions, race detector)
- `release.yml` — GoReleaser on `v*` tags

While `go/` lives inside the claude-worktree repository, the copies at
`<repo-root>/.github/workflows/go-test.yml` and `go-release.yml` are the
ones that actually run (GitHub Actions only reads workflow files from
the outer repository root). There, tags are namespaced `wt-v*` to avoid
colliding with the Python package's `v*` release flow.

## Steps

1. Make sure tests pass: `go test ./...`
2. Tag the release:

   ```bash
   # Inside the claude-worktree repo (root workflows):
   git tag wt-v0.1.0 && git push origin wt-v0.1.0

   # When go/ is its own repo (go/.github workflows):
   git tag v0.1.0 && git push origin v0.1.0
   ```

3. The release workflow runs tests, cross-compiles binaries
   (darwin/linux/windows × amd64/arm64), and creates a GitHub release
   with archives and checksums.

Version information is injected at build time via ldflags
(`main.version`, `main.commit`) and shown by `wt --version`.
