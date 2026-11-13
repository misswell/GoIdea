#!/usr/bin/env bash
# Fails when a module in GoIDEA's build graph is missing from
# THIRD_PARTY_LICENSES.md, when its license is not allowed, or when the
# license texts the release ships are missing. Run before every release.
set -euo pipefail
cd "$(dirname "$0")/.."

allowed='MIT License|Apache License|Permission is hereby granted|Redistribution and use in source and binary forms|Permission to use, copy, modify, and/or distribute this software|UNLICENSE|This is free and unencumbered software released into the public domain'
fail=0

echo "==> License texts"
for f in LICENSE NOTICE THIRD_PARTY_LICENSES.md \
         third_party/openidea/LICENSE third_party/openidea/NOTICE \
         third_party/mayoi/LICENSE \
         third_party/mygo/LICENSE third_party/purego/LICENSE \
         third_party/go-text/LICENSE third_party/x-image/LICENSE \
         third_party/x-net/LICENSE third_party/x-text/LICENSE \
         third_party/x-sys/LICENSE third_party/x-sys/PATENTS \
         third_party/moejs/LICENSE third_party/moego/LICENSE \
         third_party/chroma/COPYING third_party/regexp2/LICENSE \
         third_party/fsnotify/LICENSE third_party/goldmark/LICENSE; do
  if [[ -f "$f" ]]; then echo "  ok  $f"; else echo "  missing $f"; fail=1; fi
done

# The repository is a Mayoi fork: the original MIT copyright has to stay.
if ! grep -q 'chencheng' LICENSE; then
  echo "  LICENSE lost Mayoi's copyright"; fail=1
fi

echo "==> Build graph"
mods="$(go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}@{{.Version}}{{end}}{{end}}' ./... | sort -u | sed '/^$/d')"
replaced_dir() { # <module path> → module directory, following go.mod
  local path="$1" dir
  dir="$(go list -m -f '{{.Dir}}' "$path" 2>/dev/null)"
  echo "$dir"
}
for m in $mods; do
  path="${m%@*}"
  dir="$(replaced_dir "$path")"
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
