#!/usr/bin/env bash
# Builds GoIDEA for every Go target this machine can package and checks that each
# bundle carries an openidea-server for that target, not the host's binary.
# The server assets come from the pinned OpenIDEA release; a target without one
# is built but reported as BLOCKED rather than shipped with a foreign binary.
#
# Usage: scripts/cross-build.sh
#   GOIDEA_CROSS_OUT=<dir>   output root (default build/cross)
# Downloads are cached in build/servers/<asset>/ and reused.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="${GOIDEA_CROSS_OUT:-build/cross}"
CACHE="build/servers"
HOST_SERVER="resources/openidea-server"
rm -rf "$OUT"
mkdir -p "$OUT" "$CACHE"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/goidea-cross.XXXXXX")"
restore() {
    status=$?
    if [ -f "$WORK/host-server" ]; then
        mv "$WORK/host-server" "$HOST_SERVER"
    fi
    rm -rf "$WORK"
    exit $status
}
trap restore EXIT INT TERM
# The host's own server is what macos-aarch64 ships, so seed the cache with it
# instead of downloading the same 135 MB again.
if [ -f "$HOST_SERVER" ] && [ ! -x "$CACHE/macos-aarch64/openidea-server" ]; then
    mkdir -p "$CACHE/macos-aarch64"
    cp "$HOST_SERVER" "$CACHE/macos-aarch64/openidea-server"
fi
# The host server is lifted out of the way: every package must get the binary
# for its own target, and the last build must not leave a foreign one behind.
if [ -f "$HOST_SERVER" ]; then mv "$HOST_SERVER" "$WORK/host-server"; fi

ROWS="$WORK/rows.tsv"

# with_deadline <seconds> <command…>: kills a stalled download instead of
# letting the whole matrix wait on it.
with_deadline() {
    local secs="$1" code
    shift
    "$@" &
    local pid=$!
    ( sleep "$secs"; kill -TERM "$pid" 2>/dev/null; kill -KILL "$pid" 2>/dev/null ) &
    local guard=$!
    wait "$pid" && code=0 || code=$?
    kill "$guard" 2>/dev/null || true
    return "$code"
}

# server_for <asset>: downloads and verifies once, prints the cached path.
server_for() {
    local asset="$1" dest="$CACHE/$asset/openidea-server" attempt
    if [ ! -x "$dest" ]; then
        for attempt in 1 2; do
            if with_deadline 1800 scripts/fetch-server.sh "$asset" --out "$dest" >&2; then
                break
            fi
            printf '  %s 下载失败（第 %d 次），重试\n' "$asset" "$attempt" >&2
            sleep 5
        done
    fi
    test -x "$dest" || return 1
    printf '%s\n' "$dest"
}

# build_target <os/arch> <asset-or-empty> <arch-regex>
build_target() {
    local target="$1" asset="$2" arch="$3"
    local goos="${target%/*}" goarch="${target#*/}"
    local app server row
    printf '\n==> %s\n' "$target"
    if [ -n "$asset" ]; then
        local cached
        if ! cached="$(server_for "$asset")"; then
            printf '  FAIL 无法取得 %s 的 server asset\n' "$target" >&2
            go tool mygo build -skip-dmg -platform "$target" -o "$OUT"
            printf '%s\n' "$target|FAILED|$asset|download failed" >>"$ROWS"
            return 0
        fi
        cp "$cached" "$HOST_SERVER"
        chmod 755 "$HOST_SERVER"
        go tool mygo build -skip-dmg -platform "$target" -o "$OUT"
        rm -f "$HOST_SERVER"
        if [ "$goos" = "darwin" ]; then
            app="$(find "$OUT" -maxdepth 3 -name GoIDEA.app -type d | head -1)"
            server="$app/Contents/Resources/openidea-server"
        else
            server="$(find "$OUT" -maxdepth 3 -path "*/$goos-$goarch/openidea-server" -type f | head -1)"
        fi
        if [ ! -f "$server" ]; then
            printf '  FAIL %s 没有打包 openidea-server\n' "$target" >&2
            exit 1
        fi
        if ! file -b "$server" | grep -Eiq "$arch"; then
            printf '  FAIL %s 的 server 不是 %s：%s\n' "$target" "$arch" "$(file -b "$server")" >&2
            exit 1
        fi
        row="$target|INTEGRATED|$asset|$(file -b "$server" | cut -d, -f1)"
        printf '  ok   server %s\n' "$(file -b "$server" | cut -d, -f1)"
    else
        go tool mygo build -skip-dmg -platform "$target" -o "$OUT"
        server="$(find "$OUT" -maxdepth 3 -path "*/$goos-$goarch/openidea-server" -type f | head -1)"
        if [ -n "$server" ]; then
            printf '  FAIL %s 打包了不属于它的 server\n' "$target" >&2
            exit 1
        fi
        row="$target|BLOCKED|none|no server asset in the pinned release"
        printf '  blocked: openidea-server 没有 %s 版本\n' "$target"
    fi
    printf '%s\n' "$row" >>"$ROWS"
}

build_target darwin/arm64 macos-aarch64 'arm64'
build_target darwin/amd64 macos-x86_64 'x86_64'
build_target linux/arm64 linux-aarch64 'aarch64'
build_target linux/amd64 linux-x86_64 'x86-64'
build_target windows/amd64 "" ''

{
    echo "# GoIDEA cross-build matrix"
    echo
    echo "| Target | Status | Server asset | Bundled server |"
    echo "| --- | --- | --- | --- |"
    awk -F'|' '{printf "| %s | %s | %s | %s |\n", $1, $2, $3, $4}' "$ROWS"
    echo
    echo "Artifacts:"
    find "$OUT" -maxdepth 2 -type f \( -name '*.dmg' -o -name '*.deb' -o -name '*.tar.gz' -o -name '*.exe' -o -name goidea \) |
        sed "s|$OUT/||" | sort | sed 's/^/  /'
} >"$OUT/MATRIX.md"
cat "$OUT/MATRIX.md"
if grep -q '|FAILED|' "$ROWS"; then
    echo "有目标未能取得对应的 server asset，见 $OUT/MATRIX.md" >&2
    exit 1
fi
