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

**终端** (`Alt+F12`) is a bottom window as well. It runs the login shell in a
PTY owned by `openidea-server` (`term_spawn`, with output on a streaming
channel) and renders that text with ANSI colours removed, so full-screen
programs stay out of scope. Each tab is its own shell: `Cmd+T` or **新建**
opens one, the tab strip switches between them, and the tab's **×** ends that
shell; closing the last tab hides the window, as OpenIDEA's Close Terminal
Session does (`term.go`, `termPanel` in `view.go`).

**拆分** puts a second tab group beside the active one, as OpenIDEA's Split
Right does, and **拆分↓** stacks it below; either pane takes typing when its
tab strip is clicked, keeps its own tabs, and its **+** opens a tab in that
pane. Splitting again only turns the divider, **合并** folds the second group's
tabs into the first with their shells running, and emptying a split group in
the pane merges it away (`SplitTerm`, `UnsplitTerm`, `termPaneBody`).

`Cmd+F` in the terminal opens its find bar, bounded to the terminal's focus so
the editor keeps its own Find: the query marks every matching line and
**上一个**/**下一个** (or Enter for the next) cycle through them, wrapping at
both ends and scrolling the chosen line into view (`ShowTermFind`,
`FindTermStep`). The scrollback follows new output while it shows the end and
keeps its place once the user scrolls up.

## Structure

`Cmd+7` opens OpenIDEA's Structure window in the sidebar: the active file's
outline from `doc_outline`, indented by depth, with the symbol kind on the
right and a click that jumps to the declaration. The filter keeps a symbol that
matches or has a matching descendant, and containers it only keeps to reach a
match are dimmed. The caret collapses a container; a filter expands a collapsed
container so its matches stay visible (`structure.go`). The window shares the
outline the breadcrumbs already load, so opening it after a file has been open
does not ask the server again.

## Project view

Project tree rows open OpenIDEA's ProjectViewPopupMenu on right-click: **新建**
(File…, Directory…) on a folder, **剪切**, **复制**, **复制路径/引用**
(Absolute Path, File Name, Path From Content Root), **粘贴** while the
clipboard holds a copied or cut entry, **跳到源码** on a file,
**在文件夹中查找…** and **在终端中打开** on a folder, **重命名…** and
**删除…**. Entries that cannot apply are
disabled, and the project root itself cannot be cut, renamed or deleted
(`treeMenuItems` in `tree_ops.go`, tested by `tree_ops_test.go`).

New, Rename and Delete ask in a small sheet first; the name is refused when it
is empty or carries a path separator, and an existing entry is refused with
已存在同名文件或目录. Delete moves the entry to the trash after
**把“X”移到废纸篓？**. All four run through the backend's `fs_create_file`,
`fs_create_dir`, `fs_rename` and `fs_delete`, and Paste runs `fs_copy` or, for
a cut, `fs_rename`. A tab under an affected path that has unsaved text blocks
Rename and Delete; otherwise its clean tabs close, and bookmarks move with a
rename or drop with a delete. Paste skips an entry that would collide.
**在终端中打开** opens a terminal tab whose shell starts in that folder
(`OpenTerminalAt` in `term.go`). `SmokeFiles` drives every operation against
the real backend and the base-phase e2e checks the results.

**Cmd+=** expands every folder and **Cmd+-** folds the tree back to its root.
A folder is read when it first opens, so Expand All marks the root as the pass
to finish and each rebuild opens what the last read revealed, until nothing
below the root is closed (`Tree.ExpandAll`, `openUnder`, `expandSettled`); a
manual toggle ends the pass. **Cmd+Alt+Y** rereads the tree from disk.
**Alt+F1** (Select in Project View) opens the folders leading to the active
file and marks its row, which is drawn in the selection colour; a click marks
its row too (`SelectInProjectView`, `Tree.Select`).

**在文件夹中查找…** opens the Find window limited to that folder: `RunSearch`
then sends `scope: {kind: "directory", path}` instead of the project scope, and
the window shows 范围：<folder> until a project-wide Find (Cmd+Shift+F or
Cmd+3) widens it again (`ShowSearchInFolder` in `search.go`). The palette's
在文件夹中查找… uses the folder selected in the tree, or the whole project
when the selection is not a folder.

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
| Staged and unstaged change groups | `internal/app/vcs.go`; 已暂存/未暂存 split, per-row `vcs_stage` / `vcs_unstage` / `vcs_discard` with a rollback confirmation |
| Branch widget and Branches popup | `internal/app/branch.go`; title bar `vcs_branch_info`, popup `vcs_branches` / `vcs_checkout` |
| Line bookmarks | `internal/app/bookmarks.go`; F3 toggles the caret line, Cmd+F3 lists files and lines grouped by list with F2 description editing, next/previous cycle, gutter dots, saved in the session; Alt+F3 assigns a unique mnemonic, Ctrl+Shift+digit toggles and Ctrl+digit jumps; Cmd+2 opens the Bookmarks tool window; Toggle File Bookmark marks the active file, project tree rows mark files and folders; named lists group the window, can be renamed or deleted, and rows move between lists |
| Project view menu | `internal/app/tree_ops.go`; New File/Directory, Cut, Copy, Paste, Copy Path, Jump to Source, Find in Folder, Open in Terminal, Rename and Delete over `fs_*`, with a confirmation sheet and unsaved-tab guards |
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
