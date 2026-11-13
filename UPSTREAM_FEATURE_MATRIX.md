# Upstream feature matrix

What the three upstreams offer, and where GoIDEA stands on each. A feature is
`INTEGRATED` only when it works in GoIDEA's native UI, not when the upstream
code that implements it has been adopted. Update this table with every upstream
sync; the status words are fixed:

`NOT_AVAILABLE` · `DETECTED` · `ANALYZING` · `ADAPTING` · `IMPLEMENTED` ·
`TESTING` · `INTEGRATED` · `BLOCKED`

Last reviewed: 2026-11-13, against the pins in `UPSTREAMS.yml`.

## Mayoi (application patterns, editor, mygo tooling)

| Feature | Status | Where in GoIDEA |
| --- | --- | --- |
| Native window, menus, dialogs, theme | INTEGRATED | `mygo` app + `cmd/goidea` menu |
| Editor buffer, selections, undo/redo | INTEGRATED | `ui/editor` state per tab; `internal/docbridge` syncs it |
| Syntax highlighting | INTEGRATED | mygo editor over the backend's grammar |
| Quick-open files (Cmd+P) | INTEGRATED | `internal/app/quickopen.go` |
| Command palette | INTEGRATED | `internal/app/commands.go` |
| Project-wide search UI | INTEGRATED | `internal/app/search.go` |
| Settings sheet and stored preferences | INTEGRATED | `internal/app/settings_ui.go` |
| Sticky scroll | NOT_AVAILABLE | planned |
| Vim mode | NOT_AVAILABLE | planned |
| mygo bundling (`.app`, dmg, signing) | INTEGRATED | `mygo.json`, `scripts/release.sh` |
| Signed updates (mygo.Updater) | NOT_AVAILABLE | updates need the updater key; defer |

## MyGo (native UI framework and editor component)

| Feature | Status | Where in GoIDEA |
| --- | --- | --- |
| GPU renderer, window system, controls | INTEGRATED | every view in `internal/app` |
| Text input, buttons, scroll, modals | INTEGRATED | sheets (diff, commands, database, settings) |
| Editor: text, selections, line numbers | INTEGRATED | every open tab |
| Editor: completion, hover, go-to-definition | INTEGRATED | callbacks wired to backend RPC |
| Editor: increment search, multi-caret | DETECTED | upstream exists; not yet exposed in GoIDEA |
| Editor: IME and grapheme handling | INTEGRATED | inherited from mygo |
| `mygo build` packaging and icons | INTEGRATED | release pipeline |

## OpenIDEA (Rust project model and services)

| Feature | Status | Where in GoIDEA |
| --- | --- | --- |
| Document open/save/close, disk state | INTEGRATED | `internal/docbridge`, smoke: edit round-trip |
| UTF-16 position mapping | INTEGRATED | `internal/position`, unit + bridge tests |
| Java analysis (JDT LS) | INTEGRATED | diagnostics, completion, hover, definition |
| Intentions and quick fixes | INTEGRATED | Alt+Enter sheet |
| Rename, usages, symbols, format | INTEGRATED | code menu and pickers |
| Find/replace in project | INTEGRATED | search view |
| Maven/Gradle build and run | INTEGRATED | run window, smoke: `GOIDEA_SMOKE_RUN` |
| Debugger (DAP) | INTEGRATED | breakpoints, stepping, variables, watch |
| Git changes, diff against HEAD | INTEGRATED | changes tree, compare view |
| Terminal | INTEGRATED | terminal panel |
| Database console | INTEGRATED | database sheet (sources, schemas, tables, SQL, results) |
| Table editing, DDL view, history, explain | IMPLEMENTED (backend) / ADAPTING (UI) | next UI increment |
| HTTP client UI | ADAPTING | backend ops exist, no sheet yet |
| Agent UI | ADAPTING | backend ops exist, no panel yet |
| Plugin management UI | ADAPTING | backend ops exist, no sheet yet |
| Remote development and containers | NOT_AVAILABLE | `openidea-server --connect` supported; no UI |
| Java toolchain download on first use | INTEGRATED | backend installs JDT LS into GoIDEA's own data dir |

## Rule for a new upstream feature

1. Add a row here as `DETECTED` when the upstream commit or release that adds it
   is seen by `scripts/upstream-check.sh`.
2. Decide the route: native Go UI over existing RPC (preferred), new RPC on the
   pinned server (needs an upstream change), or `BLOCKED` with the reason.
3. Only flip to `INTEGRATED` after a smoke or test exercises the feature in
   GoIDEA, the command palette/menu entries exist where they belong, and the
   docs mention it.
