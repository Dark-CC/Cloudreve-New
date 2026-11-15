#!/bin/bash
set -e
export NODE_OPTIONS="--max-old-space-size=8192"

# Build the frontend located in ../web and embed it into the Go binary.
# Usage: ./build-assets.sh <version>

VERSION="${1:-}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WEB="$(cd "$ROOT/../web" && pwd)"

if [ -z "$VERSION" ]; then
  VERSION="$(node -p "require('$WEB/package.json').version")"
fi

cd "$WEB"
rm -rf build
if [ -f pnpm-lock.yaml ]; then
  pnpm install --frozen-lockfile || pnpm install --no-frozen-lockfile
  npm_package_name="cloudreve-frontend" npm_package_version="$VERSION" pnpm build
else
  yarn install --network-timeout 1000000
  yarn version --new-version "$VERSION" --no-git-tag-version
  yarn run build
fi

if [ ! -f "$WEB/build/index.html" ]; then
  echo "frontend build output not found at $WEB/build" >&2
  exit 1
fi

# The server reads the bundle through fs.Sub(statics, "assets/build"), so the
# archive must keep that prefix. Stage it instead of zipping build/ directly.
STAGE="$(mktemp -d)"
mkdir -p "$STAGE/assets"
cp -R "$WEB/build" "$STAGE/assets/build"

mkdir -p "$ROOT/application/statics"
rm -f "$ROOT/application/statics/assets.zip"
(
  cd "$STAGE"
  zip -q -r - assets/build
) > "$ROOT/application/statics/assets.zip"
rm -rf "$STAGE"

echo "assets.zip contains $(unzip -l "$ROOT/application/statics/assets.zip" | tail -1)"

# goreleaser archives live in src/, so the license files must be here too.
cp -f "$ROOT/../LICENSE" "$ROOT/LICENSE"
cp -f "$ROOT/../NOTICE" "$ROOT/NOTICE"
