#!/usr/bin/env bash
# Builds, signs and notarizes GoIDEA, then verifies the artifacts. The
# Developer ID comes from GOIDEA_SIGN_IDENTITY, and the notary credential from
# GOIDEA_NOTARY_PROFILE (this machine's shared Apple-ID profile), or from
# GOIDEA_APPLE_ID + GOIDEA_APPLE_PASSWORD + GOIDEA_TEAM_ID.
#
# The notarization settings live in a temporary mygo.json: the committed file
# stays ad-hoc friendly for anyone building locally. The original is restored
# even when the build fails, and the script fails if it could not restore it.
#
#   scripts/release.sh                 # darwin/arm64
#   scripts/release.sh darwin/universal
set -euo pipefail
cd "$(dirname "$0")/.."

platform="${1:-darwin/arm64}"
identity="${GOIDEA_SIGN_IDENTITY:-Developer ID Application: Guofeng Liu (U8U443D7ZL)}"
profile="${GOIDEA_NOTARY_PROFILE:-octoshrink-notary}"

[[ -f resources/icon.png ]] || { echo "resources/icon.png is missing" >&2; exit 1; }
[[ -x resources/openidea-server ]] || scripts/fetch-server.sh
# Nothing is signed or notarized before the whole regression passes: the
# release gate covers the unit tests, the offline flow, the toolchain install,
# JDT language services and the debugger against the real server.
scripts/gate.sh release

cp mygo.json mygo.json.release-backup
restore() {
  mv mygo.json.release-backup mygo.json 2>/dev/null || true
  rm -rf resources/licenses
}
trap restore EXIT

jq --arg identity "$identity" --arg profile "$profile" \
  '.macos.signingIdentity = $identity
   | .macos.notarize = {keychainProfile: $profile}' \
  mygo.json > mygo.json.release-new
mv mygo.json.release-new mygo.json

# The licenses travel with the app: Apache-2.0 and MIT both require the
# notices to accompany the binary. Only license texts, never the pinned
# source trees (third_party/openidea is a 57 MB submodule).
mkdir -p resources/licenses
cp LICENSE NOTICE THIRD_PARTY_LICENSES.md resources/licenses/
while IFS= read -r f; do
  mkdir -p "resources/licenses/$(dirname "$f")"
  cp "$f" "resources/licenses/$f"
done < <(find third_party -maxdepth 2 -type f \
  \( -iname 'LICENSE*' -o -iname 'NOTICE*' -o -iname 'PATENTS*' -o -iname 'COPYING*' \))

# mygo signs the bundle here; notarization waits until the shipped server's
# sidecar is fixed (seal_shipped_server below), because the app's preflight
# compares that sidecar with the binary beside it and the image must be built
# from the fixed app.
go tool mygo build -platform "$platform" -skip-notarize

restore
if ! git diff --quiet -- mygo.json; then
  echo "mygo.json was not restored" >&2
  exit 1
fi

app="$(find build -maxdepth 3 -name GoIDEA.app -type d | head -1)"
dmg="$(find build -maxdepth 3 -name 'goidea-*.dmg' | head -1)"
[[ -n "$app" && -n "$dmg" ]] || { echo "build did not produce an app and a dmg" >&2; exit 1; }

# The shipped server's sidecar is fixed before the image is built: signing the
# bundle rewrites the server, and the app refuses a sidecar that disagrees with
# the binary beside it. See scripts/seal-shipped-server.sh for the details.
# build_mac_dmg rebuilds the image from the app as it now stands, keeping
# mygo's Finder layout: the .DS_Store and the volume icon its image carried.
build_mac_dmg() {
  local app="$1" out="$2" work src mnt volume
  volume="$(/usr/bin/python3 -c 'import json;c=json.load(open("mygo.json"));print(c.get("macos",{}).get("dmgTitle") or c["name"])')"
  work="$(mktemp -d)"
  src="$work/src"
  mnt="$work/mnt"
  mkdir -p "$src" "$mnt"
  if hdiutil attach -nobrowse -readonly -mountpoint "$mnt" "$out" >/dev/null 2>&1; then
    cp "$mnt/.DS_Store" "$src/.DS_Store" 2>/dev/null || true
    cp "$mnt/.VolumeIcon.icns" "$src/.VolumeIcon.icns" 2>/dev/null || true
    hdiutil detach "$mnt" >/dev/null 2>&1 || hdiutil detach -force "$mnt" >/dev/null 2>&1 || true
  fi
  ditto --noclone "$app" "$src/$(basename "$app")"
  ln -s /Applications "$src/Applications"
  rm -f "$out"
  hdiutil create -ov -volname "$volume" -srcfolder "$src" \
    -fs HFS+ -fsargs "-c c=64,a=16,e=16" -format ULMO "$out" >/dev/null
  codesign --force --timestamp --sign "$identity" "$out" >/dev/null
  rm -rf "$work"
}

# notarize_mac_dmg submits the image, then staples the ticket to it and to the
# app, as mygo does: updates install the bare app.
notarize_mac_dmg() {
  local dmg="$1" app="$2"
  if ! xcrun notarytool submit "$dmg" --keychain-profile "$profile" --wait --output-format json >/dev/null; then
    echo "notarization failed; run 'xcrun notarytool log <id> --keychain-profile $profile' for why" >&2
    exit 1
  fi
  xcrun stapler staple "$dmg" >/dev/null
  xcrun stapler staple "$app" >/dev/null
}

echo "==> Sealing the shipped server"
scripts/seal-shipped-server.sh "$app" "$identity"
echo "==> Rebuilding $dmg from the sealed app"
build_mac_dmg "$app" "$dmg"
echo "==> Notarizing $dmg"
notarize_mac_dmg "$dmg" "$app"

echo "==> Verifying $app"
codesign --verify --deep --strict --verbose=2 "$app"
# The app refuses its own server when the two disagree, so the pair is
# asserted here too: a signing change must not ship a stale sidecar again.
shipped_server="$app/Contents/Resources/openidea-server"
if [[ -f "$shipped_server" ]]; then
  want="$(shasum -a 256 "$shipped_server" | awk '{print $1}')"
  have="$(awk '{print $1}' "$shipped_server.sha256" 2>/dev/null || true)"
  [[ "$want" == "$have" ]] || { echo "the shipped openidea-server and its sidecar disagree" >&2; exit 1; }
fi
spctl --assess --type execute --verbose=2 "$app"
xcrun stapler validate "$app"
echo "==> Verifying $dmg"
xcrun stapler validate "$dmg"
spctl --assess --type open --context context:primary-signature --verbose=2 "$dmg"

shasum -a 256 "$dmg" | tee "$dmg.sha256"
echo "  App: $app"
echo "  DMG: $dmg"
