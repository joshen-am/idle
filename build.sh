#!/bin/sh
# Fabrica las apps: dist/Idle-Windows.zip (.exe) y dist/Idle-Mac.zip (.app universal)
# Uso: ./build.sh [versión] [usuario/repositorio-de-github]
set -e
cd "$(dirname "$0")"
VER=${1:-1.2.0}
REPO=${2:-$(cat build/repo.txt 2>/dev/null || true)}
LDF="-s -w -X main.version=$VER -X main.updateRepo=$REPO"
rm -rf dist && mkdir -p dist

# ---- Windows ----
go run github.com/tc-hib/go-winres@v0.3.3 simply --icon build/icon-256.png --manifest gui \
  --product-name "Idle" --file-description "Idle" --product-version $VER --file-version $VER --arch amd64
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H=windowsgui $LDF" -o "dist/Idle.exe" .
rm -f rsrc_windows_*.syso
(cd dist && zip -q Idle-Windows.zip Idle.exe && rm Idle.exe)

# ---- Mac (Apple Silicon + Intel en un solo programa) ----
APP="dist/Idle.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$LDF" -o dist/mj-arm64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$LDF" -o dist/mj-amd64 .
go run github.com/konoui/lipo@v0.9.4 -output "$APP/Contents/MacOS/idle" -create dist/mj-arm64 dist/mj-amd64
rm dist/mj-arm64 dist/mj-amd64
chmod +x "$APP/Contents/MacOS/idle"
cp build/icon.icns "$APP/Contents/Resources/icon.icns"
sed "s/@VER@/$VER/g" build/Info.plist > "$APP/Contents/Info.plist"
# firma ad-hoc (sin cuenta de Apple developer)
if [ "$(uname)" = Darwin ]; then codesign --force --deep -s - "$APP"; else rcodesign sign "$APP" >/dev/null; fi
(cd dist && zip -qry Idle-Mac.zip Idle.app && rm -rf Idle.app)
ls -la dist
