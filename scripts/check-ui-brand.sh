#!/usr/bin/env bash
# The interface says GoIDEA. This check fails when a Go string literal the app
# can render still contains "openidea" — except the names that really exist on
# disk: the backend binary, its log, and the project's .openidea directory.
#
# Code comments and docs may cite the upstream by name; only string literals
# are scanned, and tests are skipped (they name the upstream in assertions).
set -euo pipefail

cd "$(dirname "$0")/.."

allowed='^(openidea-server|openidea-server\.log|\.openidea|\.openidea/|openidea-server/)$'

# User-visible surface: the UI package and the entry point.
files=(internal/app/*.go cmd/goidea/*.go)

# Names that really exist on disk (or the Go package) are not product wording:
# the backend binary, its log, the project's .openidea directory, the import.
allowed='^"(openidea-server|openidea-server\.log|\.openidea|mayoi/internal/openidea)"$'

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
