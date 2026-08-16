#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT/wails-app"

# Go 1.25+ requires macOS 12 or newer. This is lower than the requested macOS 13 floor.
export MACOSX_DEPLOYMENT_TARGET=12.0

go test ./...
WAILS_BIN="${WAILS_BIN:-$(go env GOPATH)/bin/wails}"
"$WAILS_BIN" build -clean -platform darwin/universal -nocolour

APP="build/bin/VideoHtmlDownloader.app"
codesign --force --deep --sign - "$APP"

mkdir -p "$ROOT/dist-macos"
ditto -c -k --sequesterRsrc --keepParent "$APP" \
  "$ROOT/dist-macos/VideoHtmlDownloader-macOS-12-universal.zip"

echo "Built: $ROOT/dist-macos/VideoHtmlDownloader-macOS-12-universal.zip"
