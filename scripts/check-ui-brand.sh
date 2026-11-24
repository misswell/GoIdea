#!/usr/bin/env bash
# The interface says GoIDEA. This check fails when a Go string literal the app
# can render still contains "openidea" — except the names that really exist on
# disk: the backend binary, its log, the project's .openidea directory, the
# environment variable the client sets for it, and the vendored icon asset
# paths under assets/icons/openidea. The IDE reporter's @@OPENIDEA@@ wire
# marker is protocol data from OpenIDEA and never appears in the interface.
#
# The scan covers the whole client boundary, not only internal/app: the
# adapter composes the text Run and Debug show, and internal/openidea turns
# remote errors into the sentences the status bar and notifications draw.
# Code comments and docs may cite the upstream by name; only string literals
# are scanned, and tests are skipped (they name the upstream in assertions).
set -euo pipefail

cd "$(dirname "$0")/.."

allowed='^"(@@OPENIDEA@@|X-OpenIDEA-Project|Agents name this project with the X-OpenIDEA-Project header or the \?project= query\.|[A-Za-z0-9_./$*+=-]*openidea[A-Za-z0-9_./$*+=-]*|-Dopenidea\.run\.[A-Za-z]+=true|OPENIDEA_[A-Za-z0-9_]+(=.*)?)"$'

# The client boundary: the UI package, the entry point, the ports of
# OpenIDEA's run/debug logic, and the protocol package that normalises what
# the backend sends back.
files=(internal/app/*.go cmd/goidea/*.go internal/adapter/*.go internal/openidea/*.go)

offenders=$(
  for f in "${files[@]}"; do
    [[ "$f" == *_test.go ]] && continue
    # Only string literals are scanned: comments may cite the upstream.
    grep -ohE '"[^"]*"' "$f" 2>/dev/null | grep -iE 'openidea' | grep -vE "$allowed" | sed "s|^|$f: |" || true
  done
)

if [[ -n "$offenders" ]]; then
  echo "界面文案仍用 openidea 命名（应为 GoIDEA）:" >&2
  echo "$offenders" >&2
  exit 1
fi
echo "brand check passed: the interface says GoIDEA"
