#!/usr/bin/env bash
# Fails when a module in GoIDEA's build graph is missing from
# THIRD_PARTY_LICENSES.md, when its license is not allowed, or when the
# license texts the release ships are missing. Run before every release.
set -euo pipefail
cd "$(dirname "$0")/.."

allowed='MIT License|Apache License|Redistribution and use in source and binary forms|Permission to use, copy, modify, and/or distribute this software|UNLICENSE|This is free and unencumbered software released into the public domain'
fail=0

echo "==> License texts"
for f in LICENSE NOTICE THIRD_PARTY_LICENSES.md \
         third_party/openidea/LICENSE third_party/openidea/NOTICE \
         third_party/mygo/LICENSE third_party/purego/LICENSE \
         third_party/go-text/LICENSE third_party/x-image/LICENSE \
         third_party/x-net/LICENSE third_party/x-text/LICENSE; do
  if [[ -f "$f" ]]; then echo "  ok  $f"; else echo "  missing $f"; fail=1; fi
done

echo "==> Build graph"
mods="$(go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}@{{.Version}}{{end}}{{end}}' ./... | sort -u | sed '/^$/d')"
replaced_dir() { # <module path> <version> → module cache dir, following go.mod
  local path="$1" version="$2" repl
  if [[ "$path" == github.com/egoist/mygo ]]; then
    repl="$(awk '/^replace github.com\/egoist\/mygo/ { print $4 "@" $5 }' go.mod)"
    echo "$(go env GOMODCACHE)/${repl%%@*}@${repl##*@}"
  else
    echo "$(go env GOMODCACHE)/${path}@${version}"
  fi
}
for m in $mods; do
  path="${m%@*}"
  dir="$(replaced_dir "$path" "${m##*@}")"
  license="$(find "$dir" -maxdepth 1 \( -iname 'LICENSE*' -o -iname 'COPYING*' \) 2>/dev/null | head -1 || true)"
  if [[ -z "$license" ]]; then
    echo "  unknown license: $m (no LICENSE file in $dir)"; fail=1; continue
  fi
  if ! head -40 "$license" | grep -Eq "$allowed"; then
    echo "  disallowed license: $m ($(basename "$license"))"; fail=1; continue
  fi
  listed="$path"
  [[ "$path" == github.com/egoist/mygo ]] && listed="sorrycc/mygo"
  if ! grep -q "$listed" THIRD_PARTY_LICENSES.md; then
    echo "  not listed in THIRD_PARTY_LICENSES.md: $m"; fail=1; continue
  fi
  echo "  ok  $m"
done

echo "==> Shipped server"
if [[ -x resources/openidea-server ]]; then
  echo "  ok  resources/openidea-server ($(file -b resources/openidea-server | cut -d, -f1))"
else
  echo "  note resources/openidea-server is not fetched (release.sh fetches it)"
fi

test "$fail" = 0 || { echo "license check failed" >&2; exit 1; }
echo "license check passed"
