# Releasing wt

Releases are built by GoReleaser (`.goreleaser.yaml`) via
`.github/workflows/release.yml`, which triggers on `v*` tags
(or manually via workflow dispatch).

## Steps

1. Make sure everything passes locally:

   ```bash
   go build ./... && go vet ./... && gofmt -l . && go test ./...
   ```

2. Tag the release and push the tag:

   ```bash
   git tag -a v1.0.0 -m "wt v1.0.0" && git push origin v1.0.0
   ```

3. The release workflow runs tests, cross-compiles binaries
   (darwin/linux/windows × amd64/arm64), and creates a GitHub release
   with archives and `checksums.txt`.

Version information is injected at build time via ldflags
(`main.version`, `main.commit` in `cmd/wt/main.go`) and shown by
`wt --version`.

## Verifying locally before tagging

```bash
goreleaser check                     # validate .goreleaser.yaml
goreleaser release --snapshot --clean  # full local build, no publish
```
