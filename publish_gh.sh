#!/usr/bin/env bash
set -euo pipefail

# --- Configuration ---
MODULE_DIR="$(cd "$(dirname "$0")" && pwd)"
DIST_DIR="$MODULE_DIR/dist"
TAG="${1:-}"

# --- Helpers ---
die() { echo "ERROR: $*" >&2; exit 1; }
info() { echo "==> $*"; }

latest_git_tag() {
    git -C "$MODULE_DIR" tag --list 'v*' --sort=-v:refname | head -n 1
}

bump_tag() {
    local tag="${1#v}"
    local -a parts
    local i last_index

    IFS='.' read -r -a parts <<< "$tag"
    (( ${#parts[@]} > 0 )) || return 1

    for part in "${parts[@]}"; do
        [[ "$part" =~ ^[0-9]+$ ]] || return 1
    done

    last_index=$((${#parts[@]} - 1))
    parts[$last_index]=$((10#${parts[$last_index]} + 1))

    (IFS=.; printf 'v%s' "${parts[*]}")
}

resolve_next_tag() {
    local latest
    latest="$(latest_git_tag)"
    if [[ -z "$latest" ]]; then
        printf '%s' 'v0.1'
        return 0
    fi

    bump_tag "$latest"
}

# --- Pre-flight checks ---
if [[ -z "$TAG" ]]; then
    info "Resolving next tag..."
    TAG="$(resolve_next_tag)" || die "Unable to compute next tag from git tags"
fi
[[ -n "$TAG" ]] || die "Usage: $0 [tag]  (auto-generates the next tag when omitted)"
[[ -t 0 ]] || die "Refusing to publish non-interactively."
command -v gh    >/dev/null 2>&1 || die "'gh' CLI not found — install it first"
command -v go    >/dev/null 2>&1 || die "'go' not found"
command -v zip   >/dev/null 2>&1 || die "'zip' not found — install it first"

# Make sure working tree is clean
[[ -z "$(git -C "$MODULE_DIR" status --porcelain)" ]] || die "Working tree is not clean. Commit or stash changes first."

# --- Confirm release version ---
info "About to publish version $TAG"
read -rp "Publish $TAG to GitHub? [y/N] " answer
[[ "$answer" =~ ^[Yy]$ ]] || { info "Aborted — release cancelled."; exit 0; }

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
