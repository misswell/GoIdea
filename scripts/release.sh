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
# notices to accompany the binary.
mkdir -p resources/licenses
cp LICENSE NOTICE THIRD_PARTY_LICENSES.md resources/licenses/
cp -R third_party resources/licenses/

go tool mygo build -platform "$platform"

restore
if ! git diff --quiet -- mygo.json; then
  echo "mygo.json was not restored" >&2
  exit 1
fi

app="$(find build -maxdepth 3 -name GoIDEA.app -type d | head -1)"
dmg="$(find build -maxdepth 3 -name 'goidea-*.dmg' | head -1)"
[[ -n "$app" && -n "$dmg" ]] || { echo "build did not produce an app and a dmg" >&2; exit 1; }

echo "==> Verifying $app"
codesign --verify --deep --strict --verbose=2 "$app"
spctl --assess --type execute --verbose=2 "$app"
xcrun stapler validate "$app"
echo "==> Verifying $dmg"
xcrun stapler validate "$dmg"
spctl --assess --type open --context context:primary-signature --verbose=2 "$dmg"

shasum -a 256 "$dmg" | tee "$dmg.sha256"
echo "  App: $app"
echo "  DMG: $dmg"
