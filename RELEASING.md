# Releasing wt

Releases are built by GoReleaser via `.github/workflows/go-release.yml`.

## Steps

1. Make sure tests pass: `cd go && go test ./...`
2. Tag the release from the repo root (tags are namespaced `wt-v*` to
   coexist with the Python package's `v*` tags):

   ```bash
   git tag wt-v0.1.0
   git push origin wt-v0.1.0
   ```

3. The `Go Release (wt)` workflow runs tests, cross-compiles binaries
   (darwin/linux/windows × amd64/arm64), and creates a GitHub release
   with archives and checksums.

Version information is injected at build time via ldflags
(`main.version`, `main.commit`) and shown by `wt --version`.
