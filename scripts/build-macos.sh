#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
test "$(uname -s)" = Darwin || { echo "Packaging requires macOS." >&2; exit 1; }
version="$(/usr/bin/plutil -extract version raw -o - mygo.json)"
tag="${1:-v$version}"
go run ./cmd/release prepare -tag "$tag"
version="${tag#v}"
output="$PWD/dist/release/$tag"
mkdir -p "$output"
# One Universal executable; MyGo compiles both architectures without cgo.
go run github.com/egoist/mygo/cmd/mygo build -platform darwin/universal -skip-installer -skip-dmg -o dist/macos
app="$PWD/dist/macos/darwin-universal/TextViewer.app"
if [[ ! -d "$app" ]]; then
  echo "Expected MyGo bundle missing: $app" >&2
  exit 1
fi
go run ./cmd/macbundle "$app"
/usr/bin/lipo "$app/Contents/MacOS/TextViewer" -verify_arch arm64 x86_64
/usr/bin/plutil -lint "$app/Contents/Info.plist"
identity="${MACOS_SIGNING_IDENTITY:--}"
if [[ "$identity" == "-" ]]; then
  /usr/bin/codesign --force --deep --sign - "$app"
else
  /usr/bin/codesign --force --deep --options runtime --timestamp --sign "$identity" "$app"
fi
/usr/bin/codesign --verify --deep --strict "$app"
package="$output/TextViewer-Setup-$version-macos-universal.pkg"
staging="$(mktemp -d "${TMPDIR:-/tmp}/textviewer-pkg.XXXXXX")"
trap 'rm -rf "$staging"' EXIT
mkdir "$staging/payload"
/usr/bin/ditto "$app" "$staging/payload/TextViewer.app"
/usr/bin/pkgbuild --analyze --root "$staging/payload" "$staging/components.plist"
/usr/libexec/PlistBuddy -c 'Set :0:BundleIsRelocatable false' "$staging/components.plist"
/usr/libexec/PlistBuddy -c 'Set :0:BundleHasStrictIdentifier true' "$staging/components.plist"
if ! /usr/libexec/PlistBuddy -c 'Set :0:BundleOverwriteAction upgrade' "$staging/components.plist" 2>/dev/null; then
  /usr/libexec/PlistBuddy -c 'Add :0:BundleOverwriteAction string upgrade' "$staging/components.plist"
fi
set --
if [[ -n "${MACOS_INSTALLER_IDENTITY:-}" ]]; then
  set -- --sign "$MACOS_INSTALLER_IDENTITY" --timestamp
fi
# A component package replaces the existing /Applications/TextViewer.app on upgrade.
/usr/bin/pkgbuild --root "$staging/payload" --component-plist "$staging/components.plist" --install-location /Applications \
  --identifier local.oranjekop.textviewer --version "$version" \
  "$@" "$package"
if [[ -n "${MACOS_NOTARY_PROFILE:-}" ]]; then
  /usr/bin/xcrun notarytool submit "$package" --keychain-profile "$MACOS_NOTARY_PROFILE" --wait
  /usr/bin/xcrun stapler staple "$package"
  /usr/bin/xcrun stapler validate "$package"
fi
(
  cd "$output"
  /usr/bin/shasum -a 256 "$(basename "$package")" > SHA256SUMS-macos.txt
)
