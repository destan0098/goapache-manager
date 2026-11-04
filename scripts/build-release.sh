#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

APP_NAME="goapache-manager"
GOOS_TARGET="linux"
GOARCH_TARGET="amd64"

VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo 0.1.0)}"
VERSION="${VERSION#v}"

RELEASE_NAME="${APP_NAME}-${GOOS_TARGET}-${GOARCH_TARGET}"
DIST_DIR="$ROOT_DIR/dist"
STAGING_DIR="$DIST_DIR/$RELEASE_NAME"
ARCHIVE="$DIST_DIR/${RELEASE_NAME}.tar.gz"

log() {
    echo
    echo "[$(date '+%H:%M:%S')] $*"
}

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

cleanup() {
    if [[ -n "${STAGING_DIR:-}" && -d "$STAGING_DIR" ]]; then
        rm -rf -- "$STAGING_DIR"
    fi
}

trap cleanup EXIT

log "Checking build dependencies..."

for cmd in go npm node tar sha256sum; do
    command -v "$cmd" >/dev/null 2>&1 ||
        fail "Required command not found: $cmd"
done

[[ -f go.mod ]] || fail "go.mod not found."
[[ -f frontend/package-lock.json ]] || fail "Frontend package-lock.json not found."
[[ -f scripts/install.sh ]] || fail "Installer not found."
[[ -f packaging/systemd/goapache.service ]] || fail "Backend systemd unit not found."
[[ -f packaging/systemd/goapache-helper.service ]] || fail "Helper systemd unit not found."

if [[ "$(go env GOOS)" != "linux" ]]; then
    fail "Build must run on Linux."
fi

if [[ "$(go env GOARCH)" != "amd64" ]]; then
    fail "This script currently supports Linux AMD64 only."
fi

if ! command -v gcc >/dev/null 2>&1; then
    fail "GCC is required because the project uses CGO. Install it with: sudo apt install build-essential"
fi

log "Release version: $VERSION"
log "Target: $GOOS_TARGET/$GOARCH_TARGET"

log "Preparing release directory..."

mkdir -p "$DIST_DIR"
rm -rf -- "$STAGING_DIR"
mkdir -p "$STAGING_DIR"

log "Building Go backend..."

CGO_ENABLED=1 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o "$STAGING_DIR/goapache" \
    ./cmd/server

log "Building privileged helper..."

CGO_ENABLED=1 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o "$STAGING_DIR/goapache-helper" \
    ./cmd/helper

chmod 0755 "$STAGING_DIR/goapache" "$STAGING_DIR/goapache-helper"

log "Installing frontend dependencies..."

(
    cd frontend
    npm ci
)

log "Building frontend..."

(
    cd frontend
    npm run build
)

[[ -f frontend/dist/index.html ]] ||
    fail "Frontend build did not produce frontend/dist/index.html"

log "Copying application files..."

mkdir -p "$STAGING_DIR/frontend"
cp -a frontend/dist "$STAGING_DIR/frontend/"

mkdir -p "$STAGING_DIR/scripts"
cp scripts/install.sh "$STAGING_DIR/scripts/install.sh"
chmod 0755 "$STAGING_DIR/scripts/install.sh"

mkdir -p "$STAGING_DIR/packaging/systemd"
cp packaging/systemd/*.service "$STAGING_DIR/packaging/systemd/"

cp README.md "$STAGING_DIR/README.md"
cp LICENSE "$STAGING_DIR/LICENSE"

if [[ -f configs/config.example.yaml ]]; then
    mkdir -p "$STAGING_DIR/configs"
    cp configs/config.example.yaml "$STAGING_DIR/configs/"
fi

cat > "$STAGING_DIR/RELEASE" <<RELEASE_INFO
Application: GoApache Manager
Version: $VERSION
Operating System: Linux
Architecture: amd64
Build Date: $(date -u '+%Y-%m-%dT%H:%M:%SZ')
Git Commit: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)
RELEASE_INFO

log "Checking generated binaries..."

file "$STAGING_DIR/goapache"
file "$STAGING_DIR/goapache-helper"

log "Creating release archive..."

tar -czf "$ARCHIVE" \
    -C "$DIST_DIR" \
    "$RELEASE_NAME"

log "Generating SHA256 checksum..."

(
    cd "$DIST_DIR"
    sha256sum "$(basename "$ARCHIVE")" > "$(basename "$ARCHIVE").sha256"
)

rm -rf -- "$STAGING_DIR"

log "Release build completed successfully."

echo
echo "========== RELEASE OUTPUT =========="
ls -lh "$ARCHIVE" "$ARCHIVE.sha256"

echo
echo "========== SHA256 =========="
cat "$ARCHIVE.sha256"

echo
echo "========== ARCHIVE CONTENTS =========="
tar -tzf "$ARCHIVE"

echo
echo "Release ready: $ARCHIVE"
