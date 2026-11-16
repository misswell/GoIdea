#!/usr/bin/env bash
# Makes a signed macOS app's openidea-server checksum sidecar describe the
# binary the app actually ships.
#
# Signing a bundle with `codesign --deep` also signs openidea-server among its
# resources, and a signature is appended to the file: the sidecar
# fetch-server.sh wrote (the digest of the upstream download) then disagrees
# with the file beside it. The app's preflight compares the two before it
# starts the server, so a signed app with the upstream digest refuses its own
# server:
#
#   goidea: server sha256: <shipped>, want <upstream>
#
# Provenance stays at download time (fetch-server.sh verifies the pinned
# digest); the bundle carries the digest of the bytes it runs, which is what
# the preflight can check. The sidecar is a sealed resource, so the bundle is
# sealed again after the edit (without --deep: the nested code keeps the
# signature the packager gave it).
#
# Usage: scripts/seal-shipped-server.sh <path/to/App.app> [signing identity]
# Exits non-zero when the pair cannot be made to agree or the app does not
# verify, so a release can never ship the mismatch.
set -euo pipefail

app="${1:-}"
identity="${2:-${GOIDEA_SIGN_IDENTITY:-Developer ID Application: Guofeng Liu (U8U443D7ZL)}}"
sign_args=()
if [[ -z "$app" || ! -d "$app" ]]; then
  echo "usage: scripts/seal-shipped-server.sh <App.app> [identity]" >&2
  exit 1
fi

server="$app/Contents/Resources/openidea-server"
sidecar="$server.sha256"
if [[ ! -f "$server" ]]; then
  # A bundle without the server (a non-Mac build, or one that expects
  # GOIDEA_SERVER) has nothing to seal.
  exit 0
fi

hash="$(shasum -a 256 "$server" | awk '{print $1}')"
shipped=""
[[ -f "$sidecar" ]] && shipped="$(awk '{print $1}' "$sidecar")"

if [[ "$hash" != "$shipped" ]]; then
  printf '%s  openidea-server\n' "$hash" > "$sidecar"
  if [[ "$(uname -s)" == "Darwin" ]]; then
    # An ad-hoc build keeps an ad-hoc seal; a Developer ID build keeps the
    # hardened runtime and a secure timestamp, as the packager signed it.
    sign_args=(--force --sign "$identity")
    [[ "$identity" != "-" ]] && sign_args+=(--options runtime --timestamp)
    codesign "${sign_args[@]}" --preserve-metadata=entitlements,flags "$app" >/dev/null
  fi
fi

if [[ "$(uname -s)" == "Darwin" ]]; then
  codesign --verify --deep --strict "$app" >/dev/null 2>&1 || {
    echo "seal-shipped-server: $app does not verify after sealing the sidecar" >&2
    exit 1
  }
fi
[[ "$hash" == "$(awk '{print $1}' "$sidecar")" ]] || {
  echo "seal-shipped-server: the shipped openidea-server and its sidecar disagree" >&2
  exit 1
}
echo "openidea-server sha256 $hash (as shipped)"
