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
database console, themes and font settings, command palette and quick open,
plus the Java toolchain installer (JDT LS and java-debug) in the settings sheet.

Later increments: table editing and DDL/history/explain in the database sheet,
HTTP client, agent panel, plugin management, remote development, separate
Build/Rebuild/Clean actions, Spring Boot application run/stop coverage, and the
v0.2.0 performance bench table.

## Docs

- `ARCHITECTURE.md` — processes, packages, threads, restart and startup.
- `RPC_PROTOCOL.md` — framing, `invoke` commands, channels, cancellation,
  errors, backpressure, timeouts.
- `OPENIDEA_IPC.md` — every IPC command OpenIDEA registers, which ones
  `openidea-server` serves, and which ones GoIDEA calls (generated).
- `OPENIDEA_PARITY_MATRIX.md` — every OpenIDEA feature folder, action, keymap,
  tool window, overlay, editor type, setting, language, IPC command, event and
  plugin extension point, mapped to GoIDEA (generated).
- `EDITOR.md` — documents, ChangeSets, UTF-16 mapping, undo, save, conflicts.
- `JAVA_SUPPORT.md` — project detection, toolchain install, LSP, fixtures.
- `BUILD_AND_DEBUG.md` — run configurations, run/stop, DAP pipeline, gaps.
- `UPSTREAM_SYNC.md` — pins, daily report, weekly sync, rollback.
- `PERFORMANCE.md` — budgets, mechanisms, how to measure.
- `RELEASE.md` — version, gates, signing, notarization, verification.
- `REGRESSION_INDEX.json` — every end-to-end check and the gate it blocks.

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
scripts/gate.sh pr        # format, vet, unit tests, build, licenses, index
scripts/gate.sh main      # + offline e2e against the real server and Maven
scripts/gate.sh release   # + toolchain install, JDT language services, debugger
```

The end-to-end run copies `testdata/java-maven` to a temporary folder, builds
it with the local `mvn`, edits it over the real backend, installs jdtls and
java-debug into `build/e2e-data`, and drives completion, rename, a Maven build
and a DAP session in a hidden window. Every assertion is listed in
`REGRESSION_INDEX.json` and enforced by `scripts/check-regression-index.py`.

CI runs the PR gate on `ubuntu-latest`. The `e2e` job (`scripts/gate.sh main`)
and the `macos-14` unit tests are gated behind the `ENABLE_MACOS_CI` repository
variable because GitHub hosted macOS runners are not available to this account;
the release itself was built, signed and notarized on macOS with
`scripts/release.sh`.

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
