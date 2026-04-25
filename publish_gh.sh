#!/usr/bin/env bash
set -euo pipefail

# --- Configuration ---
MODULE_DIR="$(cd "$(dirname "$0")" && pwd)"
DIST_DIR="$MODULE_DIR/dist"
TAG="${1:-}"

# --- Helpers ---
die() { echo "ERROR: $*" >&2; exit 1; }
info() { echo "==> $*"; }

# --- Pre-flight checks ---
[[ -n "$TAG" ]] || die "Usage: $0 <tag>  (e.g. v0.1.0)"
command -v gh    >/dev/null 2>&1 || die "'gh' CLI not found — install it first"
command -v go    >/dev/null 2>&1 || die "'go' not found"
command -v zip   >/dev/null 2>&1 || die "'zip' not found — install it first"

# Make sure working tree is clean
[[ -z "$(git -C "$MODULE_DIR" status --porcelain)" ]] || die "Working tree is not clean. Commit or stash changes first."

# --- Build ---
info "Formatting Go files..."
gofmt -w "$MODULE_DIR"

info "Linting..."
(cd "$MODULE_DIR" && go vet ./...)

info "Testing..."
(cd "$MODULE_DIR" && go test ./...)

info "Building Windows resources..."
(cd "$MODULE_DIR" && go run github.com/tc-hib/go-winres@latest make \
    --in build/windows/winres.json --out cmd/rsrc)

info "Building turing-display.exe..."
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"
(cd "$MODULE_DIR" && GOOS=windows GOARCH=amd64 go build \
    -ldflags="-H windowsgui" -o dist/turing-display.exe ./cmd)

info "Copying config..."
cp "$MODULE_DIR/config/config.json" "$DIST_DIR/config.json"

info "Zipping release..."
cd "$DIST_DIR" && zip -r "$MODULE_DIR/dist/turing-display.zip" .

info "Build complete: $DIST_DIR"
ls -lh "$DIST_DIR"

# --- Confirm before publishing ---
read -rp "Tested the exe? Push to GitHub? [y/N] " answer
[[ "$answer" =~ ^[Yy]$ ]] || { info "Aborted — test the build first."; exit 0; }

# --- Tag & Push ---
info "Creating tag $TAG..."
git -C "$MODULE_DIR" tag -a "$TAG" -m "Release $TAG"
info "Pushing tag..."
git -C "$MODULE_DIR" push origin "$TAG"

# --- GitHub Release ---
info "Creating GitHub release..."
gh release create "$TAG" \
    --title "$TAG" \
    --notes "Release $TAG" \
    "$DIST_DIR/turing-display.zip"

info "Done! Release published at:"
gh release view "$TAG" --json url --jq '.url'
