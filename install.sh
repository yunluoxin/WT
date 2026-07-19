#!/bin/sh
# Build wt with stripped symbols (-s -w) and install into $GOBIN (or $GOPATH/bin).
set -eu

cd "$(dirname "$0")"

version=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
commit=$(git rev-parse --short HEAD 2>/dev/null || echo "none")

GOBIN=${GOBIN:-$(go env GOBIN)}
if [ -z "$GOBIN" ]; then
  GOBIN="$(go env GOPATH)/bin"
fi

echo "building wt ${version} (${commit}) -> ${GOBIN}/wt"
go build -trimpath -ldflags="-s -w -X main.version=${version} -X main.commit=${commit}" -o "$GOBIN/wt" ./cmd/wt
ls -lh "$GOBIN/wt"
