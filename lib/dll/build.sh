#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"

mkdir -p "$REPO_ROOT/.dist"

CGO_ENABLED=1 GOOS=linux GOARCH="$GOARCH_VALUE" \
	go build -buildvcs=false -buildmode=c-shared -o "$REPO_ROOT/.dist/libtsunamidb.so" "$SCRIPT_DIR"

echo "Linux shared library build completed: .dist/libtsunamidb.so"
