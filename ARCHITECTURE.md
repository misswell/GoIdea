# Architecture

GoIDEA is one native application process plus one backend process. The window,
editor, and every control are drawn by MyGo (Go, GPU); the Java project model,
language server, builds, debugger, VCS, terminal and database come from
`openidea-server` (OpenIDEA, Rust) over JSON-RPC on stdio. There is no WebView,
Electron, Tauri, React, HTML or CSS in GoIDEA's own UI, and no JPS.

```
cmd/goidea (GoIDEA.app)
├── internal/app          window, panels, editor over MyGo   — main thread
├── internal/docbridge    shared buffers, ChangeSets, merge
├── internal/position     UTF-16 ↔ bytes, line index
├── internal/openidea     JSON-RPC client and process manager
└── openidea-server --stdio (child process, stdout = RPC only)
```

## Process model

- `cmd/goidea` starts, resolves its data directory, and launches
  `resources/openidea-server --stdio` with `OPENIDEA_SERVER_HOME` pointing at
  `<dataDir>/openidea-server` (`internal/openidea/process.go`). The user's
  OpenIDEA installation and its `jdtls` processes are never reused or touched.
- stdout of the server carries framed JSON-RPC and nothing else; stderr is
  copied into GoIDEA's log file verbatim (`process.go:106-108`).
- Files, settings and session state live under the mygo data directory, which
  `GOIDEA_DATA_DIR` overrides for tests and smoke runs. The backend's managed
  tools (JDT LS, java-debug) live under `<dataDir>/openidea-server/data/tools`.
- Shutdown closes stdin, waits 3 s, sends SIGTERM to the process group, waits
  2 s, then SIGKILL (`process_unix.go`). No orphan server survives a normal
  quit.

## Startup

1. mygo creates the window; the app shows it unless `GOIDEA_HEADLESS=1`.
2. `openidea.Manager.Ensure` launches the server (3 attempts, 250 ms × attempt)
   and performs `initialize {version: 2}`; a protocol mismatch is fatal
   (`internal/openidea/process.go:59-144`).
3. The app sends `open_project` for the startup folder or the last session's
   root, then polls `project_current` every 60 ms for up to 60 s until the
   backend reports the project (`internal/app/app.go:594-613`).
4. `project_trust_status` is checked; the trust dialog gates every build, run,
   debug and tool install (`internal/app/run.go:60-143`).
5. Session files are reopened: only paths inside the resolved project root are
   restored (`inProject`, symlink and separator tolerant), then `doc_open`
   seeds each buffer.

## Threads and mutation

The model lives on the main thread. Background work (RPC, disk, dialogs) hands
results back through `mygo.RunOnMain` (`Model.post`), so view fields are mutated
from one goroutine. `syncLoop` is a separate ticker that pushes buffer changes
even when no frames are being built (window hidden or minimized)
(`internal/app/app.go:283-294`).

## Packages

| Package | Responsibility |
| --- | --- |
| `internal/openidea` | process launch, framing, request/notification dispatch, reconnect, typed RPC calls |
| `internal/docbridge` | one `Document` per path shared by tabs; buffer ↔ backend sync, conflict merge, save |
| `internal/position` | implicit-treap line index; UTF-16 offsets ↔ byte offsets |
| `internal/app` | the window: tree, tabs, editor callbacks, search, diff, VCS, run, debug, terminal, database, settings, command palette |

## Shared editor state

All tabs for a path share one `docbridge.Document`; closing a tab does not drop
unsaved text (the document stays registered until its buffer is discarded after
save or explicit revert). Truly separate copies of the same file never exist in
GoIDEA, which is what makes the ChangeSet protocol sound.

## Backend restarts

`Manager.watch` restarts an unexpectedly closed server up to 5 times with
`attempt² × 200 ms` backoff (`internal/openidea/manager.go:222-249`). On a
restart, `OnRestart` reopens the project and every open document, preserving
unsaved buffer text, and resets run/debug/terminal views to a clean disconnected
state (`internal/app/app.go:212-250`).

## Test surface

Unit tests run with `CGO_ENABLED=0 go test ./...`. End-to-end coverage drives a
hidden window with `GOIDEA_HEADLESS=1` and `GOIDEA_SMOKE_*` flags against the
real server and real Maven projects (`scripts/e2e.sh`); every check is listed in
`REGRESSION_INDEX.json`.
