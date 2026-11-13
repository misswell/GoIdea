#!/usr/bin/env bash
# Compares the pins in UPSTREAMS.yml with the upstreams, and writes a report
# answering the questions a sync decision needs: what changed, what it is,
# whether it reaches GoIDEA, and what to do about it. The report is a draft:
# the inherit/adapt/defer decisions stay with a person.
#
#   scripts/upstream-check.sh > UPSTREAM_REPORT.md
set -uo pipefail
cd "$(dirname "$0")/.."

field() { # <upstream> <key>
  awk -v up="$1" -v key="$2" '
    /^  [a-zA-Z0-9_-]+:$/ { sec=$1; sub(":","",sec); next }
    sec==up && $1 == key":" { print $2; exit }
  ' UPSTREAMS.yml
}
server_field() { # <key> in openidea.server
  awk -v key="$1" '
    /^    server:/ { s=1; next }
    s && /^    [a-z_]+:/ { s=0 }
    s && $1 == key":" { print $2; exit }
  ' UPSTREAMS.yml
}
latest_commit() { gh api "repos/$1/commits/$2" --jq .sha 2>/dev/null; }
latest_release() { gh api "repos/$1/releases/latest" --jq '.tag_name' 2>/dev/null; }

head_section() { printf '## %s\n\n' "$1"; }
compare_section() { # <repo> <pin> <label>
  local repo="$1" pin="$2" label="$3"
  local json commits
  json="$(gh api "repos/$repo/compare/$pin...HEAD" 2>/dev/null)"
  if [[ -z "$json" || "$json" == *"Not Found"* ]]; then
    echo "- ${label}: could not compare $pin with upstream HEAD (pin not found or the API failed)"
    return
  fi
  commits="$(jq -r '.commits | length' <<<"$json")"
  echo "- ${label}: **${commits} commits** since the pin"
  if (( commits > 0 )); then
    echo
    echo "### Commits (newest first)"
    echo
    jq -r '.commits | reverse | .[] | "- `\(.sha[0:8])` \(.commit.message | split("\n")[0])"' <<<"$json" | head -60
    echo
    echo "### Changed files"
    echo
    jq -r '[.files[]?.filename] | .[0:60][] | "- `\(.)`"' <<<"$json"
    local total
    total="$(jq -r '.files | length' <<<"$json")"
    (( total > 60 )) && echo "- … and $((total - 60)) more"
    echo
    echo "### Classification"
    echo
    jq -r '
      .commits[].commit.message | split("\n")[0] as $m
      | if ($m | test("^feat")) then "- feature: \($m)"
        elif ($m | test("^fix")) then "- bug fix: \($m)"
        elif ($m | test("^perf")) then "- performance: \($m)"
        elif ($m | test("breaking|\\w+!:")) then "- breaking change (review the protocol): \($m)"
        else empty end' <<<"$json" | head -60
    echo
    echo "### Reach into GoIDEA"
    echo
    jq -r '[.files[]?.filename] | unique | .[]' <<<"$json" | awk '
      /crates\/ide-host\/src\/ops|crates\/ide-jsonrpc|crates\/ide-core\/src\/rpc/ {
        print "- RPC surface touched: `" $0 "` — run the RPC contract tests before adopting"; rpc=1; next }
      /^ui\/|^cmd\/mygo\// {
        print "- native UI surface touched: `" $0 "` — check the widgets GoIDEA uses"; ui=1; next }
      /^src-tauri\// {
        print "- desktop shell only (Tauri), does not reach GoIDEA: `" $0 "`"; next }
      { other++ }
      END {
        if (other > 0) print "- " other " other files do not touch the RPC or UI surface GoIDEA uses"
        if (!rpc) print "- RPC surface: unchanged"
        if (!ui) print "- native UI surface: unchanged"
      }'
  fi
}

echo "# Upstream report"
echo
echo "Generated $(date -u '+%Y-%m-%d %H:%M UTC') from the pins in \`UPSTREAMS.yml\`."
echo

head_section "Mayoi"
pin="$(field mayoi commit)"
latest="$(latest_commit sorrycc/Mayoi main)"
echo "- Pinned: \`${pin:0:8}\` · upstream main: \`${latest:0:8}\`"
echo "- Latest release: \`$(latest_release sorrycc/Mayoi)\`"
compare_section sorrycc/Mayoi "$pin" "Mayoi"

head_section "MyGo (sorrycc fork)"
pin="$(field mygo commit)"
latest="$(latest_commit sorrycc/mygo main)"
official="$(latest_commit egoist/mygo main)"
echo "- Pinned fork commit: \`${pin:0:8}\` · fork main: \`${latest:0:8}\`"
echo "- Official egoist/mygo main: \`${official:0:8}\`"
echo "- Module pin in go.mod: \`$(field mygo module_version)\`"
compare_section sorrycc/mygo "$pin" "MyGo fork"

head_section "OpenIDEA"
pin="$(field openidea commit)"
latest="$(latest_commit l0ng-ai/openidea main)"
echo "- Pinned source: \`${pin:0:8}\` · upstream main: \`${latest:0:8}\`"
echo "- Latest release: \`$(latest_release l0ng-ai/openidea)\`"
echo "- Shipped server: release \`$(server_field release)\`, commit \`$(server_field release_commit)\`"
compare_section l0ng-ai/openidea "$pin" "OpenIDEA"

head_section "Decision"
echo
echo "| Question | Answer |"
echo "| --- | --- |"
echo "| 1. Files changed | above, per upstream |"
echo "| 2. New features | the \`feature:\` lines above |"
echo "| 3. Bug fixes | the \`bug fix:\` lines above |"
echo "| 4. Performance | the \`performance:\` lines above |"
echo "| 5. API changes | the RPC surface lines above |"
echo "| 6. Breaking changes | the breaking-change lines above |"
echo "| 7. Effect on GoIDEA | run \`CGO_ENABLED=0 go test ./...\` against a candidate server, then the offscreen smokes (\`GOIDEA_SMOKE_*\`) |"
echo "| 8. Inherit directly | needs a person: copy the rows to \`UPSTREAM_FEATURE_MATRIX.md\` |"
echo "| 9. Needs adaptation | needs a person: native UI or protocol glue |"
echo "| 10. Defer, and why | needs a person: record the reason in the matrix |"
