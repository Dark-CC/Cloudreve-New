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

mkdir -p "$ROOT/application/statics"
cd "$WEB"
rm -f "$ROOT/application/statics/assets.zip"
zip -r - assets/build > "$ROOT/application/statics/assets.zip"
