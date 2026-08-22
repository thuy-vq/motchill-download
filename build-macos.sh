#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"
if [[ "${BUMP_VERSION:-1}" == "1" ]]; then
  node scripts/bump-version.mjs
fi
VERSION="$(tr -d '\r\n' < VERSION)"
cd "$ROOT/wails-app"

# Go 1.25+ requires macOS 12 or newer. This is lower than the requested macOS 13 floor.
export MACOSX_DEPLOYMENT_TARGET=12.0

# main.go embeds frontend/dist, so it must exist before `go test` compiles the package.
# On a clean checkout it does not, and the Wails build that would create it runs later.
if [[ ! -d frontend/dist ]]; then
  echo "frontend/dist not found, building frontend first"
  (
    cd frontend
    if [[ ! -d node_modules ]]; then
      npm install
    fi
    npm run build
  )
fi

go test ./...
WAILS_BIN="${WAILS_BIN:-$(go env GOPATH)/bin/wails}"
"$WAILS_BIN" build -clean -platform darwin/universal -nocolour

APP="build/bin/VideoHtmlDownloader.app"
# Wails names the bundle after "name" in wails.json, not "outputfilename".
if [[ ! -d "$APP" ]]; then
  BUILT="$(find build/bin -maxdepth 1 -name '*.app' -print -quit)"
  if [[ -z "$BUILT" ]]; then
    echo "No .app bundle found in build/bin" >&2
    exit 1
  fi
  rm -rf "$APP"
  mv "$BUILT" "$APP"
fi
codesign --force --deep --sign - "$APP"

mkdir -p "$ROOT/dist-macos"
ditto -c -k --sequesterRsrc --keepParent "$APP" \
  "$ROOT/dist-macos/VideoHtmlDownloader-v${VERSION}-macOS-12-universal.zip"

DMG="$ROOT/dist-macos/VideoHtmlDownloader-v${VERSION}-macOS-12-universal.dmg"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
ditto "$APP" "$STAGE/VideoHtmlDownloader.app"
ln -s /Applications "$STAGE/Applications"
hdiutil create -volname "Video HTML Downloader" -srcfolder "$STAGE" -ov -format UDZO "$DMG"

echo "Built: $ROOT/dist-macos/VideoHtmlDownloader-v${VERSION}-macOS-12-universal.zip"
echo "Built: $DMG"
