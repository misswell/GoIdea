# Editor

The editor is MyGo's `ui/editor` over buffers owned by `internal/docbridge`.
Draw and input belong to MyGo; text, positions and backend synchronization
belong to GoIDEA.

## Documents and tabs

- One `docbridge.Document` exists per path; several tabs can show the same
  document (`internal/docbridge/registry.go:12-92`).
- `doc_open` seeds the buffer from the backend, normalized to LF. The backend
  version and a base snapshot are kept, plus a hash of the saved text
  (`document.go:66-135`).
- Tab dirty state is true while syncing, saving, the buffer is unsynced, or the
  backend reports the document dirty (`internal/app/app.go:876-878`).
- Closing a tab keeps the buffer; text is only discarded after save or an
  explicit revert.

## Buffer → backend

`syncLoop` ticks every 120 ms (`app.go:283-294`). `syncPump` debounces 220 ms
after the last buffer-version change, then syncs off the UI thread
(`app.go:838-872`).

A `ChangeSet` is an ordered list of `{from,to,insert}` in **UTF-16 offsets into
the pre-change text** (`internal/position/protocol.go:15-27`), built by diffing
the base snapshot against the current buffer (`document.go:607-632`). The app
sends `doc_edit {path, version, changes}`; there is no `doc_change` command.

UTF-16 is what the backend and the LSP protocol use; the Go buffer counts
bytes. `position.PositionMapper` keeps an implicit treap of per-line byte and
UTF-16 counts (`position/index.go`), updated incrementally by `ApplyEdit`
(`mapper.go:24-31`). It exposes byte↔UTF-16 offset and position conversions,
range→byte offsets, `ChangeSetToByteEdits` and `TextEditsToByteEdits`
(`mapper.go:41-189`). A position inside a surrogate pair snaps to the rune
start (`mapper.go:54-59`).

## Backend → buffer

- `docPatched` notifications are applied as edits when the buffer still matches
  the base. A stale base or a dirty buffer triggers a resync instead of blindly
  applying offsets (`document.go:313-346`).
- Resync reads `doc_snapshot`, diffs base→remote and base→local, and merges
  non-overlapping hunks. Overlapping hunks raise `ConflictError`, are stored as
  `pending`, and the UI asks the user which side to keep; `Resolve` pushes the
  local text at the remote version or drops the local edit
  (`document.go:476-723`). After three failed attempts it reports
  `ErrResync` rather than looping.
- Workspace edits reload the buffer and drop local undo history, because the
  offsets no longer refer to the same text (`document.go:327-336`).
- Undo and redo go through the buffer and then sync, so the backend's history
  and the visible text agree (`document.go:376-409`). An editor edit is applied
  as one transaction, which is one undo step (`document.go:728-742`).

## Saving and external changes

Save syncs pending changes, calls `doc_save`, clears dirty and records the text
hash (`document.go:413-441`). Saving is asynchronous and bounded so a slow disk
cannot block the UI (`app.go:783-817`). Run, debug and tool installs call
`saveAll` first (`app.go:819-836`, `run.go:280`, `debug.go:474`).

A 2 s stat watch notices external edits (`internal/app/watch.go:17,55-72`).
Clean buffers reload the file; buffers above 4 MiB are left alone. Dirty
buffers keep their text and show a banner instead of silently overwriting
either side (`watch.go:75-106`).

## Problems

The backend publishes a file's diagnostics per producer; `diagnostics.go` keeps
the newest per file path and producer, so a file's problems stay after its tab
closes. The status bar counts errors, warnings and information and opens the
Problems tool window, which shows the active file or the whole project, groups
by file with files that have errors first, filters by severity, and jumps to a
row's line and column (`problems.go`).

`Cmd+6` opens the window and a second press from the editor closes it. The
tool-window actions follow OpenIDEA's keymap: `Shift+Escape` hides the active
window, `Cmd+Shift+F12` hides every window and a second press restores the ones
that were visible, `F12` reopens the last window used, even after it was hidden,
and `Cmd+0` opens Commit (`toolwindow.go`). `Cmd+Alt+Down` and `Cmd+Alt+Up`
step through the problem rows and wrap at both ends.

The bottom side has two areas, as OpenIDEA's does: its primary area and its
split area sit side by side at half the width each, and each shows one window.
Opening Problems takes the primary area from the Run output while a window in
the split area stays, and the replaced window keeps its contents, so the
terminal's shell and a run's output are still there when it comes back. **视图 ›
移动到底部右侧** moves the active window into the split area, replacing the
window shown there, and **移动到底部** moves it back (`occupyBottom`,
`toolArea` and `moveActiveTool` in `toolwindow.go`, `bottomSlot` in `view.go`).

`Cmd+Shift+'` maximizes the slot over the workbench and a second press gives
the editor back; hiding the slot or opening a file also restores it.
`Ctrl+Alt+Up` and `Ctrl+Alt+Down` move the slot's edge by 24 pixels, no smaller
than 80 and no taller than the window less 160, and the arrows along the edge
do nothing, as `resizeDelta` returns 0 for them. The slot keeps one height for
whichever window it shows, as OpenIDEA keeps one size per side
(`bottomSize` in `toolwindow.go`).

Find (`Cmd+3`) and Find Usages (`Alt+F7`) are bottom windows too, as OpenIDEA
anchors them, so their results no longer take the sidebar
(`searchPanel`, `usagesPanel` in `view.go`). The sidebar keeps the windows
OpenIDEA anchors on the left: Project, Commit's changes and Structure.

## Structure

`Cmd+7` opens OpenIDEA's Structure window in the sidebar: the active file's
outline from `doc_outline`, indented by depth, with the symbol kind on the
right and a click that jumps to the declaration. The filter keeps a symbol that
matches or has a matching descendant, and containers it only keeps to reach a
match are dimmed. The caret collapses a container; a filter expands a collapsed
container so its matches stay visible (`structure.go`). The window shares the
outline the breadcrumbs already load, so opening it after a file has been open
does not ask the server again.

## Features wired to the editor

| Feature | Where |
| --- | --- |
| Syntax highlighting | MyGo language chosen per document in `docbridge` |
| Diagnostics, gutter marks | `internal/app/diagnostics.go`, `codeintel.go` |
| Problems tool window, status-bar counts | `internal/app/problems.go` |
| Structure tool window (Cmd+7) | `internal/app/structure.go` |
| Tool window maximize and resize | `internal/app/toolwindow.go`, `maximize_test.go` |
| Bottom split area, Move to Bottom Right | `internal/app/toolwindow.go`, `split_test.go` |
| Find and Find Usages anchored at the bottom | `internal/app/search.go`, `usages.go`, `anchor_test.go` |
| Completion, hover, definition popups | MyGo callbacks → `internal/app/lsp.go` |
| Intentions / quick fixes (Alt+Enter) | `internal/app/intents.go` |
| Rename (Shift+F6) with undo | `internal/app/rename.go` |
| Find usages (Alt+F7) | `internal/app/usages.go` |
| Go to symbol (Cmd+Shift+O) | `internal/app/symbols.go` |
| Formatting | Edit menu → `lsp_format` |
| Project search | `internal/app/search.go` |
| Diff against HEAD | `internal/app/diff.go` |
| Git changes and Git log | `internal/app/vcs.go`, `internal/app/log.go`; commit in `commit.go` |
| Quick open (Cmd+P) / command palette | `quickopen.go`, `commands.go` |
| Vim mode, sticky scroll | Mayoi's implementations are in-tree but not wired into `cmd/goidea` (see `UPSTREAM_FEATURE_MATRIX.md`) |

## Tests

`internal/position` has the offset and edit conversion tests. `internal/docbridge`
covers round-trip edits, resync conflicts and version handling.
`internal/app/problems_test.go` proves severity mapping, ordering, filters and
counts.
`GOIDEA_SMOKE_EDIT` proves an edit and an undo reach the backend;
`GOIDEA_SMOKE_WATCH` proves an external edit is reloaded;
`GOIDEA_SMOKE_LSP` proves format/rename/format-through-JDT on a real project,
and that JDT's error reaches the Problems tool window (`lsp.problems.*`).
