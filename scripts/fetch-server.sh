#!/usr/bin/env bash
# Downloads the OpenIDEA server GoIDEA ships, at the pin in UPSTREAMS.yml, and
# verifies it against the release's SHA-256 before unpacking it into
# resources/. release.sh calls this; run it alone to refresh the binary.
#
#   scripts/fetch-server.sh                        # this Mac's platform
#   scripts/fetch-server.sh macos-aarch64          # a specific release asset
#   scripts/fetch-server.sh linux-x86_64 --out DIR # elsewhere (cross-build.sh)
set -euo pipefail
cd "$(dirname "$0")/.."

platform="${1:-macos-$(uname -m | sed 's/arm64/aarch64/;s/x86_64/x86_64/')}"
out="resources/openidea-server"
if [[ "${2:-}" == "--out" ]]; then
    test -n "${3:-}" || { echo "fetch-server.sh: --out needs a path" >&2; exit 1; }
    out="$3"
fi
mkdir -p "$(dirname "$out")"
field() { # <key> in the openidea.server block
  awk -v key="$1" '
    /^    server:/ { s=1; next }
    s && /^    [a-z_]+:/ { s=0 }
    s && $1 == key":" { print $2; exit }
  ' UPSTREAMS.yml
}
release="$(field release)"
asset="$(field asset)"
if [[ -n "$1" ]]; then
  asset="openidea-server-${platform}.gz"
fi
test -n "$release" && test -n "$asset" || { echo "UPSTREAMS.yml has no openidea.server pin" >&2; exit 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
gh release download "$release" -R l0ng-ai/openidea -p "$asset*" -D "$tmp" --clobber
want="$(awk '{print $1}' "$tmp/$asset.sha256")"
got="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"
test "$want" = "$got" || { echo "$asset: sha256 $got, release says $want" >&2; exit 1; }
gunzip -c "$tmp/$asset" > "$out.tmp"
chmod 755 "$out.tmp"
mv "$out.tmp" "$out"
echo "$out: $release $asset (sha256 $got)"
