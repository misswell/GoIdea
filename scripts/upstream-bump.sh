#!/usr/bin/env bash
# Moves the pins in UPSTREAMS.yml to the upstream heads, and the mygo module to
# the fork's head. The upstream workflow runs this on a branch, tests the
# result and opens a pull request; running it on main is a mistake.
set -euo pipefail
cd "$(dirname "$0")/.."

today="$(date -u +%Y-%m-%d)"
set_field() { # <section> <key> <value>
  awk -v sec="$1" -v key="$2" -v val="$3" '
    /^  [a-zA-Z0-9_-]+:$/ { cur=$1; sub(":","",cur) }
    cur==sec && $1 == key":" { $2=val }
    { print }
  ' UPSTREAMS.yml > UPSTREAMS.yml.new
  mv UPSTREAMS.yml.new UPSTREAMS.yml
}
set_server_field() { # <key> <value> in openidea.server
  awk -v key="$1" -v val="$2" '
    /^    server:/ { s=1; next }
    s && /^    [a-z_]+:/ { s=0 }
    s && $1 == key":" { $2=val }
    { print }
  ' UPSTREAMS.yml > UPSTREAMS.yml.new
  mv UPSTREAMS.yml.new UPSTREAMS.yml
}
head_of() { gh api "repos/$1/commits/$2" --jq .sha; }

echo "==> Mayoi"
mayoi="$(head_of sorrycc/Mayoi main)"
set_field mayoi commit "$mayoi"
set_field mayoi synced_at "$today"
echo "  $mayoi"

echo "==> MyGo"
mygo="$(head_of sorrycc/mygo main)"
set_field mygo commit "$mygo"
set_field mygo synced_at "$today"
old="$(awk '/^replace github.com\/egoist\/mygo/ { print $4 }' go.mod)"
go mod edit -replace "github.com/egoist/mygo=github.com/sorrycc/mygo@$mygo"
if go mod tidy >/dev/null 2>&1; then
  version="$(awk '/^replace github.com\/egoist\/mygo/ { print $4 }' go.mod)"
  set_field mygo module_version "$version"
  echo "  $mygo ($version)"
else
  go mod edit -replace "github.com/egoist/mygo=$old"
  echo "  $mygo: go mod tidy failed, module pin left at $old" >&2
fi

echo "==> OpenIDEA"
openidea="$(head_of misswell/openidea main)"
upstream="$(head_of l0ng-ai/openidea main)"
git -C third_party/openidea fetch --quiet origin main
git -C third_party/openidea checkout --quiet "$openidea"
release="$(gh api repos/l0ng-ai/openidea/releases/latest --jq .tag_name)"
release_commit="$(gh api "repos/l0ng-ai/openidea/git/ref/tags/$release" --jq .object.sha)"
set_field openidea commit "$openidea"
set_field openidea version "${release#v}"
set_field openidea synced_at "$today"
set_server_field release "$release"
set_server_field release_commit "$release_commit"
echo "  source $openidea (upstream $upstream), server $release ($release_commit)"
if [[ "$openidea" != "$upstream" ]]; then
  echo "  note: the fork is not at upstream main; merge l0ng-ai/openidea into the fork to inherit those commits" >&2
fi

git diff --stat
