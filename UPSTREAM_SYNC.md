# Upstream sync

GoIDEA is a product of three upstreams, not a copy of them. This document says
where each pin lives, how it moves, and what must be verified before it moves.

## Pins

`UPSTREAMS.yml` is the only pin file.

| Upstream | How it is included | Pin keys |
| --- | --- | --- |
| Mayoi | source and Git history merged into this repository | `upstreams.mayoi.commit`, `version`, `synced_at`; `local_changes` |
| MyGo | Go module with a `replace` to the tested build | `upstreams.mygo.module_version`; `go.mod` |
| OpenIDEA | fork submodule plus the shipped server release | `upstreams.openidea.commit`, `submodule`, `server{release, release_commit, asset, sha256_file}` |

The user's own checkouts of Mayoi and OpenIDEA are never modified: this
repository is the Mayoi fork, and `third_party/openidea` is a submodule of the
OpenIDEA fork. `resources/openidea-server` is not committed; it is downloaded
from the pinned release by `scripts/fetch-server.sh` and verified against the
release's SHA-256.

## Daily report

`.github/workflows/upstream.yml` runs at 06:00 UTC. `scripts/upstream-check.sh`
uses the GitHub API to compare each pin with upstream HEAD and reports:

- commits since the pin, the newest 60 subjects and up to 60 changed files;
- a classification of message prefixes (`feat`, `fix`, `perf`, `breaking`);
- whether the change reaches GoIDEA's seams — `crates/ide-host/src/ops`,
  `crates/ide-jsonrpc`, `crates/ide-core/src/rpc` (RPC), `ui/` and `cmd/mygo/`
  (native UI), `src-tauri/` (the original desktop shell, not used by GoIDEA);
- submodule-versus-pin drift, the `go.mod` module pin, the latest OpenIDEA
  release, and the shipped server version;
- the decision table: files, features, fixes, performance, API changes,
  breaking changes, expected effect on unit tests and `GOIDEA_SMOKE_*`, and a
  recommendation to inherit, adapt or defer.

The report is posted as one issue per day; it is not a merge.

## Weekly sync

Mondays at 07:00 UTC the same workflow runs mode `sync`:

1. `scripts/upstream-bump.sh` moves the Mayoi commit, the MyGo module version
   (`go mod edit -replace`), the OpenIDEA submodule checkout, and the server
   release fields; `synced_at` is updated.
2. The report is regenerated; if nothing changed, the run stops.
3. `go vet`, `go test`, `go build` and `scripts/check-licenses.sh` run.
4. The branch `upstream/sync-YYYY-MM-DD` is pushed and a PR is opened or
   updated. CI never merges it.

A sync PR is only merged after:

- the PR gate is green;
- the main gate (`scripts/gate.sh main`) passes on the release machine,
  including the offline end-to-end run against the real server;
- for an OpenIDEA RPC change, the `lsp`, `debug` and `tools` phases pass too
  (`scripts/gate.sh release`);
- `UPSTREAM_FEATURE_MATRIX.md` has a row for every new feature, with its real
  status, and `REGRESSION_INDEX.json` covers the checks that prove it.

## Feature inheritance rules

A row in `UPSTREAM_FEATURE_MATRIX.md` starts at `DETECTED` when the report sees
the upstream change. It moves to `INTEGRATED` only when GoIDEA's own UI exposes
it, a smoke or test exercises it, the menu/command palette entry exists, and
this documentation mentions it. Code adopted is not the same as a feature
working here.

Preferred route: a native Go UI over an existing RPC command. If the backend
lacks the command, the change goes through the OpenIDEA fork first (submodule
bump), never by patching `third_party/openidea` in place. `local_changes` in
`UPSTREAMS.yml` must stay `none-to-upstream`; a local patch needs its own fork
commit and a documented re-base plan.

## Conflict and rollback

- Conflicts with the Mayoi merge are resolved here, in the fork; shared code
  (`cmd/mayoi`, `internal/*` inherited packages) is kept compatible and GoIDEA's
  differences live in `cmd/goidea` and `internal/app`.
- To roll back a bad sync: `git revert` the merge commit (the pins return to
  their previous values), or restore `UPSTREAMS.yml`, `go.mod` and the
  submodule pointer from the last good release tag and run
  `git submodule update --init`.
- A bad server release is rolled back by restoring the previous `server`
  block, running `scripts/fetch-server.sh` again, and re-running the release
  gate before shipping.
- Rollbacks are recorded in the sync PR and in `CHANGELOG.md` when they affect
  a shipped version.

## Server releases

`scripts/fetch-server.sh [platform]` downloads `openidea-server-<platform>.gz`
and its `.sha256` from the pinned release with `gh`, verifies the digest, and
installs it at `resources/openidea-server`. The release workflow for GoIDEA
uses the macOS aarch64 asset; other platforms are tracked in `PERFORMANCE.md`
and `RELEASE.md` until their assets are verified.
