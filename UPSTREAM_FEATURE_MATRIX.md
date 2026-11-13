# Upstream feature matrix

What the three upstreams offer, and where GoIDEA stands on each. A feature is
`INTEGRATED` only when it works in GoIDEA's native UI, not when the upstream
code that implements it has been adopted. Update this table with every upstream
sync; the status words are fixed:

`NOT_AVAILABLE` · `DETECTED` · `ANALYZING` · `ADAPTING` · `IMPLEMENTED` ·
`TESTING` · `INTEGRATED` · `BLOCKED`

Last reviewed: 2026-11-13, against the pins in `UPSTREAMS.yml`.

## Mayoi (repository base: application, editor, terminal, Git, plugins)

Mayoi's source and Git history are merged into this repository (see
`UPSTREAMS.yml`), so every row below can be adopted by porting code rather than
reimplementing it. A row is `INTEGRATED` only when the GoIDEA product
(`cmd/goidea`) exposes it; Mayoi's own window (`cmd/mayoi`) does not count.

| Feature | Status | Where in GoIDEA |
| --- | --- | --- |
| Native window, menus, dialogs, theme | INTEGRATED | `mygo` app + `internal/app/menu.go` as the native menu bar; OpenIDEA's palette in both appearances (`internal/app/theme.go`) |
| Welcome screen with recent projects | INTEGRATED | `internal/app/welcome.go` mirrors WelcomeView.tsx: brand rail, Projects/Customize/Plugins, search and actions, recent projects with branch and menus, clone and remove dialogs; offscreen render in `welcome_render_test.go` |
| Plugin manager page (Welcome › Plugins) | INTEGRATED | `internal/app/plugins.go` over `internal/plugin`: Available/Installed lists, details, enable state under `plugins.<name>`, Git install, reload and remove; tests in `plugins_test.go` |
| Editor buffer, selections, undo/redo | INTEGRATED | `ui/editor` state per tab; `internal/docbridge` syncs it |
| Syntax highlighting | INTEGRATED | mygo editor's language for the file, set per document in `internal/docbridge` (Java, Kotlin, XML, YAML…) |
| Quick-open files (Cmd+P) | INTEGRATED | `internal/app/quickopen.go` |
| Command palette | INTEGRATED | `internal/app/commands.go` |
| Project-wide search UI | INTEGRATED | `internal/app/search.go`; OpenIDEA's Find window, bottom-anchored |
| Settings dialog and stored preferences | INTEGRATED | `internal/app/settings_window.go`, `settings_pages.go`; OpenIDEA's 980 DIP Settings dialog: page tree built as `model/pages.ts` builds it (dotted ids nest, group folders, search keeps ancestors and selects the first match), breadcrumbs, Back/Forward history, Reset, and Cancel/Apply/OK staging that previews theme and font; pages are Appearance (Theme, Sync with OS), Keymap, Editor › General (Soft-wrap files with file patterns, Show virtual space at file bottom), Editor › Appearance (Show line numbers, Show whitespaces, Show indent guides and distraction free width, driving the editor gutter and whitespace), Editor › Breadcrumbs (Show breadcrumbs with Top/Bottom placement, driving the editor's breadcrumb row), Editor › Editor Tabs (Tab placement, pinned tabs row, tab labels, close button side, sort, open at end, tab limit), Editor › Code Folding (Show code folding outline), Editor › Font (size) and Languages › Java (toolchain); `internal/app/settings_test.go` |
| mygo bundling (`.app`, dmg, signing) | INTEGRATED | `mygo.json`, `scripts/release.sh` |
| Workspace file tree: lazy reads, live disk updates | INTEGRATED | `internal/workspace` under `internal/app/tree.go`; tests in `tree_test.go` |
| Sticky scroll | DETECTED | source in `cmd/mayoi`; not wired into `cmd/goidea` |
| Vim mode | DETECTED | source in `cmd/mayoi`; not wired into `cmd/goidea` |
| Terminal UI and PTY lifecycle | DETECTED | Mayoi's terminal is in-tree; GoIDEA uses the backend terminal |
| Git changes/history UI | DETECTED | Mayoi's SCM is in-tree; GoIDEA uses the backend VCS |
| JS/TS plugin engine (moejs) | DETECTED | source and plugin runtime in-tree; the management page uses `internal/plugin`, but plugin code is not started in `cmd/goidea` |
| Signed updates (mygo.Updater) | NOT_AVAILABLE | updates need the updater key; defer |

## MyGo (native UI framework and editor component)

| Feature | Status | Where in GoIDEA |
| --- | --- | --- |
| GPU renderer, window system, controls | INTEGRATED | every view in `internal/app` |
| Text input, buttons, scroll, modals | INTEGRATED | sheets (diff, commands, database, settings) |
| Editor: text, selections, line numbers | INTEGRATED | every open tab |
| Editor tabs: pin, close others/all/left/right/unmodified | INTEGRATED | pinned group at the left with a pin glyph, OpenIDEA's tab context menu and guarded closes that stop on cancel; `internal/app/tabs.go`, `internal/app/tabs_test.go` |
| Editor: completion, hover, go-to-definition | INTEGRATED | callbacks wired to backend RPC |
| Editor lines: comment, duplicate, delete, join, move | INTEGRATED | IntelliJ macOS keys from `src/platform/keymaps/intellij-macos.ts` via `editor.State.Bindings`, the same commands in the **Code** menu and registry; statement moves run on whole lines; `internal/app/editor_keys.go`, `internal/app/editor_keys_test.go` |
| Editor: increment search, multi-caret | DETECTED | upstream exists; not yet exposed in GoIDEA |
| Editor: IME and grapheme handling | INTEGRATED | inherited from mygo |
| `mygo build` packaging and icons | INTEGRATED | release pipeline |

## OpenIDEA (Rust project model and services)

| Feature | Status | Where in GoIDEA |
| --- | --- | --- |
| Document open/save/close, disk state | INTEGRATED | `internal/docbridge`, smoke: edit round-trip |
| UTF-16 position mapping | INTEGRATED | `internal/position`, unit + bridge tests |
| Java analysis (JDT LS) | INTEGRATED | diagnostics, completion, hover, definition |
| Problems tool window (File / Project) | INTEGRATED | `internal/app/problems.go`; per-file diagnostics, status-bar counts, `lsp.problems.*` in the release e2e |
| Structure tool window (Cmd+7) | INTEGRATED | `internal/app/structure.go`; opens in the left side's split island below Project, filter, collapse, click-to-jump, `lsp.structure.*` in the release e2e |
| Tool window maximize (Cmd+Shift+'), resize (Ctrl+Alt+arrows) | INTEGRATED | `internal/app/toolwindow.go`; one height per bottom slot, editor restored on navigation; `internal/app/maximize_test.go` |
| Bottom split area, Move to Bottom Right | INTEGRATED | `internal/app/toolwindow.go`; each area shows one window at half the width; `internal/app/split_test.go` |
| Left split area, Structure and Bookmarks below Project | INTEGRATED | `internal/app/workbench.go`; the left side's split island shows Structure or Bookmarks under the Project window, one at a time, and a lone island fills the side; the 6px gap between the islands drags the split with a row resize cursor, keeping 0.1–0.9 of the side and 60 DIP on the smaller area as OpenIDEA's SplitHandle does, and each project keeps its ratio; `internal/app/layout_test.go`, `internal/app/workbench_render_test.go` |
| Left tool window resize (drag the gap) | INTEGRATED | `internal/app/workbench.go`; the 6px gap on the left side's inner edge drags its width with a column resize cursor, keeping 140 DIP for the side and 200 for the editor, and a release past half the minimum closes both left areas as OpenIDEA's GapHandle does; the width is stored per project within 80–4000; `internal/app/layout_test.go`, `internal/app/workbench_render_test.go` |
| Bottom tool window resize (drag the gap) | INTEGRATED | `internal/app/workbench.go`; the 6px gap above the bottom area drags its height with a row resize cursor, keeping 90 DIP for the side and 60 for the editor, the height stored per project, and a release past half the minimum closes every bottom window as OpenIDEA's GapHandle does; `internal/app/workbench_render_test.go` |
| Main menu (menu.rs structure) | INTEGRATED | `internal/app/menu.go`; File, Edit, View, Navigate, Code, Refactor, Build, Run, Git, Tools and Window with OpenIDEA's labels and order, every item running a registry command, installed as the native menu bar by `cmd/goidea`; `internal/app/menu_test.go` |
| Title bar project, VCS and gear widgets | INTEGRATED | `internal/app/workbench.go`; the project menu's Open… and Manage Projects…, Version Control ▾ while no VCS widget exists (Clone…, Show Git Log), and the gear menu in TitleBar.tsx order: Run Anything…, Settings, Plugins, Theme, Keymap, View Mode, Customize Main Toolbar, Check for Updates…; `internal/app/gear_test.go` |
| Title bar Run widget | INTEGRATED | `internal/app/run_widget.go`; OpenIDEA's RunWidget: the chooser (selected configuration, every configuration, Reload Run Configurations), Run and Debug, Resume, Rerun and Stop while a session is live, and the More Actions menu, every control running a registry command; `internal/app/run_widget_test.go` |
| Run Anything popup | INTEGRATED | `internal/app/run_anything.go`; `runAnything.show` in the gear menu and palette opens OpenIDEA's 620px popup: Recent and Run Configurations sections, matching by words with prefix first, Clear History, per-project history, arrow/Page/Tab/Enter keys, Enter runs the chosen row or the typed line in the project shell through `run_start`; `internal/app/run_anything_test.go` |
| Main toolbar customization | INTEGRATED | `internal/app/toolbar.go`; OpenIDEA's mainToolbar settings: the saved left/right/hidden layout in the preferences, widgets registered since the save added at their default side, and MainToolbarDialog's draft: Left/Right lists, Add… picker over hidden widgets and actions, Remove, Move Up/Down, Move to the other side, Restore Defaults, OK applies, Cancel discards; `internal/app/toolbar_test.go`, `internal/app/gear_test.go` |
| Distraction Free Mode (View Mode) | INTEGRATED | `internal/app/zen.go`; `editor.toggleZen` hides the tool windows, stripes, toolbar, status bar, tabs and gutter, centres an 80-column editor with IDEA's compact 30px header and exit hint, persists the mode and width (20–500) in the preferences, and applies the gutter to tabs opened while on; `internal/app/gear_test.go` |
| Check for Updates | INTEGRATED | `internal/app/update.go`; `app.checkForUpdates` asks the GitHub releases feed in the background, compares its tag with `mygo.App.Version()` and reports available/latest/failure in a dialog with a release-page link; signed download and install stay NOT_AVAILABLE; `internal/app/gear_test.go` |
| Per-project tool window layout | INTEGRATED | `internal/app/toolwindow.go`, `settings.go`; left window, left split window, its split ratio, the left side's width, bottom windows, bottom split dock, height and last-used window stored per project root in the session and restored on open, **恢复默认布局** merges the split area; `internal/app/layout_test.go` |
| Intentions and quick fixes | INTEGRATED | Alt+Enter sheet |
| Rename, usages, symbols, format | INTEGRATED | code menu and pickers; Find Usages is OpenIDEA's bottom window, `internal/app/usages.go` |
| Find/replace in project | INTEGRATED | search view |
| Project view menu: New, Cut/Copy/Paste, Copy Path, Jump to Source, Find in Folder, Rename, Delete, Open in Terminal | INTEGRATED | right-click menu in `internal/app/tree_ops.go` over `fs_create_file`, `fs_create_dir`, `fs_rename`, `fs_copy`, `fs_delete` and `search_start` with a directory scope, with a prompt sheet and unsaved-tab guards; base-phase e2e `base.files.*` |
| Project view Expand All (Cmd+=), Collapse All (Cmd+-), Reload from Disk (Cmd+Alt+Y), Reload Project Tree, Select in Project View (Alt+F1) | INTEGRATED | `internal/app/tree.go` opens folders as their reads arrive, `internal/app/tree_ops.go` reveals and selects the active file, and `workspace.Tree.RefreshDir` rereads just the selected subtree; base-phase e2e `base.files.expand`, `base.files.select`, `base.files.reload` |
| Local History: Show History, restore a snapshot, Put Label | INTEGRATED | project view submenu and palette over `history_list_dir`, `history_restore` and `history_put_label` in `internal/app/history.go`; base-phase e2e `base.files.historyRestore`, `base.files.historyLabel` |
| Compare Directories | INTEGRATED | project view **比较目录…** and palette over `fs_compare_directories` in `internal/app/dircompare.go`, listing 仅左侧/仅右侧/内容不同/相同, with a row opening the two files' compare view (`ShowFileDiff`); base-phase e2e `base.files.compareDirs` |
| Maven/Gradle build and run | INTEGRATED | run window, smoke: `GOIDEA_SMOKE_RUN` |
| Debugger (DAP) | INTEGRATED | breakpoints, stepping, variables, watch |
| Git changes, diff against HEAD | INTEGRATED | changes tree, compare view |
| Commit with file inclusion, amend, sign-off, hooks | INTEGRATED | `Cmd+K` commit dialog → `vcs_commit`; base-phase e2e |
| Git log with details and changed files | INTEGRATED | Git log panel → `vcs_log`, `vcs_log_details`, `vcs_commits_changes`; base-phase e2e |
| Branch widget and Branches popup | INTEGRATED | title bar shows the head with ahead/behind and operation, popup checks out → `vcs_branch_info`, `vcs_branches`, `vcs_checkout`; `internal/app/branch.go`; base-phase e2e |
| Staged change groups and change actions | INTEGRATED | 已暂存/未暂存 groups, rows stage, unstage and roll back with a confirmation → `vcs_stage`, `vcs_unstage`, `vcs_discard`; `internal/app/vcs.go`; base-phase e2e |
| Line bookmarks | INTEGRATED | F3 toggles the caret line, Cmd+F3 opens the Bookmarks popup with files and lines grouped by list and F2 description editing, Cmd+2 opens the Bookmarks tool window in the left split area below Project, Toggle File Bookmark marks the active file and project tree rows mark files and folders, named bookmark lists group the tool window and can be renamed, deleted, or moved between, next/previous cycle with wrap, gutter dots, session persistence, Alt+F3 mnemonic chooser, Ctrl+Shift+digit toggle and Ctrl+digit jump with unique mnemonics; `internal/app/bookmarks.go`; base-phase e2e |
| Terminal | INTEGRATED | bottom **终端** window, one tab per login shell in a backend PTY (`term_spawn`, `term_write`, `term_kill`), `Cmd+T` opens a tab and closing the last hides the window, **拆分**/**拆分↓** split into a second tab group and **合并** merges it, `Cmd+F` finds in the scrollback; `internal/app/term.go`; base-phase e2e |
| Database console | INTEGRATED | database sheet (sources, schemas, tables, SQL, results) |
| Table editing, DDL view, history, explain | IMPLEMENTED (backend) / ADAPTING (UI) | next UI increment |
| HTTP client UI | ADAPTING | backend ops exist, no sheet yet |
| Agent UI | ADAPTING | backend ops exist, no panel yet |
| Plugin management UI | ADAPTING | backend ops exist, no sheet yet |
| Remote development and containers | NOT_AVAILABLE | `openidea-server --connect` supported; no UI |
| Java toolchain download on first use | INTEGRATED | settings sheet + palette install JDT LS/java-debug with progress, cancel and retry into GoIDEA's own data dir; `GOIDEA_SMOKE_TOOLS` |
| Spring Boot application run/stop | IMPLEMENTED (backend) / ADAPTING (coverage) | backend detects `spring-boot` configs and GoIDEA prefers application configs; no gate runs one yet |
| Build / rebuild / clean actions | INTEGRATED | Build Project, Build Module, Rebuild and Build Artifacts through `build_run`, with Stop Build, a Build window and base-phase e2e; no message tree yet |
| Multi-JDK project selection | ADAPTING | backend reports `javaExec`; no per-project JDK picker in the UI |

## Rule for a new upstream feature

1. Add a row here as `DETECTED` when the upstream commit or release that adds it
   is seen by `scripts/upstream-check.sh`.
2. Decide the route: native Go UI over existing RPC (preferred), new RPC on the
   pinned server (needs an upstream change), or `BLOCKED` with the reason.
3. Only flip to `INTEGRATED` after a smoke or test exercises the feature in
   GoIDEA, the command palette/menu entries exist where they belong, and the
   docs mention it.
