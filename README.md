# GoIDEA

A native Java and Spring Boot IDE. GoIDEA is built as a fork of
[Mayoi](https://github.com/sorrycc/Mayoi), so Mayoi's application source and
Git history are part of this repository: the window and every control are drawn
by [MyGo](https://github.com/egoist/mygo) (Go, GPU); the Java project model, JDT
language server, Maven/Gradle, run, debug, VCS, terminal, database and agent
services come from [OpenIDEA](https://github.com/l0ng-ai/openidea)'s Rust
backend, reached over JSON-RPC on `openidea-server --stdio`.

There is no Electron, Tauri, WebView, React, HTML or CSS in GoIDEA's own UI,
and no JPS: the Java ecosystem is not reimplemented here.

## Status

Version 0.1.0. Working today: document bridge with UTF-16 mapping and
undo/redo, Java syntax highlighting, diagnostics, completion, hover,
definitions, intentions, rename, usages, symbols, format, project search, a
project tree that follows the disk (Mayoi's `internal/workspace`),
Maven/Gradle build and run, DAP debugging, Git changes and diff, terminal,
database console, themes and font settings, command palette and quick open.

Later increments: table editing and DDL/history/explain in the database sheet,
HTTP client, agent panel, plugin management, remote development, benchmarks,
and the remaining docs.

## Build

Requirements: Go 1.27.3 or newer; macOS 13 or newer for the app bundle.
The Rust server is not compiled here: `scripts/fetch-server.sh` downloads the
pinned official `openidea-server` release and verifies its SHA-256.

Clone with the submodules; `third_party/openidea` pins the OpenIDEA fork's
source at the commit GoIDEA ships against:

```sh
git clone --recurse-submodules https://github.com/misswell/GoIdea
cd GoIdea
scripts/fetch-server.sh          # resources/openidea-server
CGO_ENABLED=0 go build -o build/goidea ./cmd/goidea
GOIDEA_SERVER=resources/openidea-server ./build/goidea
```

If the repository is already cloned, `git submodule update --init --recursive`
does the same.

Run the tests and the offscreen checks:

```sh
gofmt -l .
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
scripts/check-licenses.sh
```

CI runs these on `ubuntu-latest`. The `macos-14` job is gated behind the
`ENABLE_MACOS_CI` repository variable because GitHub hosted macOS runners are
not available to this account; the release itself was built, signed and
notarized on macOS with `scripts/release.sh`.

The app reads `GOIDEA_OPEN_PROJECT` to open a folder at startup and
`GOIDEA_HEADLESS=1` to stay offscreen. `GOIDEA_SMOKE_*` flags drive the
checks in `cmd/goidea/main.go` (edit round-trip, run, debug, search,
terminal, code intelligence, LSP, diff, commands, watch, VCS, settings,
database); each writes its findings to `GOIDEA_SMOKE_OUT`.

## Release

```sh
scripts/release.sh               # build, sign with Developer ID, notarize
```

The script restores `mygo.json` afterwards and verifies the app and the disk
image with `codesign`, `spctl` and `stapler`. Pushing a `v*` tag whose version
matches `mygo.json` runs the same script on GitHub Actions and publishes the
DMG (see `.github/workflows/release.yml`).

## Upstreams

GoIDEA follows three upstreams and inherits their work instead of forking away
from them:

| Upstream | Use | Pin |
| --- | --- | --- |
| [Mayoi](https://github.com/sorrycc/Mayoi) (fork [misswell/Mayoi](https://github.com/misswell/Mayoi)) | repository base; app, editor, terminal, Git, plugins | merged history, `UPSTREAMS.yml` |
| [MyGo](https://github.com/egoist/mygo) (fork [sorrycc/mygo](https://github.com/sorrycc/mygo)) | native UI framework and editor | `go.mod` + `UPSTREAMS.yml` |
| [OpenIDEA](https://github.com/l0ng-ai/openidea) (fork [misswell/openidea](https://github.com/misswell/openidea)) | Rust backend and `openidea-server` | submodule `third_party/openidea` + `UPSTREAMS.yml` |

`UPSTREAMS.yml` is the only pin file. `scripts/upstream-check.sh` reports what
moved, `scripts/upstream-bump.sh` moves the pins on a branch, and the Upstream
workflow runs the report every day and opens a tested pull request every week.
`UPSTREAM_FEATURE_MATRIX.md` records, per upstream feature, whether GoIDEA has
integrated it — code adopted is not the same as a feature working here.

## Licenses

GoIDEA's own code is MIT (`LICENSE`), and the fork keeps Mayoi's MIT copyright
because Mayoi's source is included. The shipped server is OpenIDEA's
Apache-2.0 build, redistributed unmodified with its `NOTICE`; the UI framework
is MIT. See `THIRD_PARTY_LICENSES.md`; `scripts/check-licenses.sh` enforces it.
