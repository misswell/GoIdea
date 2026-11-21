# OpenIDEA parity matrix

OpenIDEA is the product specification, Mayoi/MyGo is the frontend base and the
OpenIDEA Rust core is the engine. This document enumerates everything the pinned
OpenIDEA source registers and maps each item to GoIDEA. It is generated from
`203871f1250ba5054bf976de2ce3267963c6011e`; `scripts/openidea-parity.py --check` fails when it is stale.

## How to read the status

| Status | Meaning |
| --- | --- |
| `integrated` | GoIDEA has the feature and a gate check proves it. |
| `partial` | Part of the feature exists in GoIDEA, with evidence. |
| `engine-only` | The engine behind it is reachable over stdio, but no GoIDEA UI proof exists. |
| `blocked` | It needs an OpenIDEA change (an `ops` module, or a Host/crate change) first. |
| `missing` | No GoIDEA implementation is recorded. |

`missing` is the default and is not a claim that no code exists anywhere: it means
no evidence has been recorded. A registered action with an otherwise undeclared
status appears as `partial`; other raised statuses come from
`scripts/openidea-parity-status.json`, where evidence names a regression check id,
a doc or a source path. Nothing is hand-picked out of the inventory: the item
lists below are generated from the source, and a new action, tool window, page or
command appears automatically.

## Inventory

| Kind | Items | Integrated | Partial | Engine | Blocked | Missing |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Feature folders | 44 | 7 | 13 | 0 | 0 | 24 |
| Actions | 888 | 225 | 391 | 0 | 0 | 272 |
| Keymap presets | 6 | 0 | 0 | 0 | 0 | 6 |
| Tool windows | 30 | 11 | 10 | 0 | 1 | 8 |
| Overlays | 56 | 11 | 5 | 0 | 0 | 40 |
| Editor contributions | 35 | 1 | 1 | 0 | 0 | 33 |
| Editor kinds | 4 | 0 | 0 | 0 | 0 | 4 |
| File editor providers | 4 | 0 | 0 | 0 | 0 | 4 |
| Settings pages | 88 | 10 | 9 | 0 | 0 | 69 |
| Settings keys | 470 | 6 | 1 | 0 | 0 | 463 |
| Languages | 82 | 1 | 0 | 0 | 0 | 81 |
| Backend crates | 43 | 0 | 0 | 35 | 0 | 8 |
| Events | 31 | 0 | 0 | 0 | 0 | 31 |
| Plugin worlds | 37 | 0 | 0 | 0 | 0 | 37 |
| Plugin exports | 21 | 0 | 0 | 0 | 0 | 21 |
| IPC commands | 668 | - | - | - | - | see [OPENIDEA_IPC.md](OPENIDEA_IPC.md) |

## Frontend feature folders

Every directory under `src/features` (44). Counts are the contributions that
directory registers.

| Feature | Actions | Tool windows | Overlays | Editor | Settings pages | Status |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| `agents` | 32 | 0 | 0 | 0 | 3 | integrated - internal/app/agents.go, agents_view.go, agent_composer.go, agent_editor.go, agent_markdown.go, agent_workspace.go and agent_registry_settings.go implement OpenIDEA's task home, editor/tool-window placement, groups, selection, search, conversation timeline, composer, changes, registry, policy and MCP settings over the real agent RPCs; agents_test.go, agent_*_test.go and the agents e2e phase cover the surface |
| `analysis` | 6 | 2 | 1 | 0 | 0 | missing |
| `app` | 47 | 0 | 6 | 0 | 0 | partial - internal/app/update.go, tasks.go and notifications.go cover update checks, background download, cancellation and install/restart; update_flow_test.go; internal/app/invalidate.go covers cache invalidation and restart; other app overlays still have pending scope |
| `build` | 6 | 1 | 0 | 0 | 0 | missing |
| `build-tools` | 23 | 2 | 1 | 3 | 0 | partial - internal/app/maven.go, pom_completion.go and pom_goto.go; Maven window and POM completion/navigation; internal/app/maven_test.go; see separate Maven and Gradle rows |
| `code-insight` | 6 | 3 | 1 | 0 | 0 | partial - lsp.references.count, lsp.symbols |
| `color-preview` | 0 | 0 | 0 | 1 | 0 | missing |
| `coverage` | 6 | 1 | 1 | 1 | 0 | integrated - internal/app/coverage.go, coverage_run.go and coverage_view.go implement report import and suites, Run with Coverage, context coverage, show/hide state, project-tree badges, editor gutter marks that follow edits, and OpenIDEA gutter tooltips; coverage_test.go covers import, suite management, action enablement, rendering, the short-lived run reply race, edit remapping and tooltip titles; TestContextRunWithCoveragePreparesLaunchesAndCollects covers the context executor |
| `database` | 46 | 1 | 1 | 1 | 0 | partial - UPSTREAM_FEATURE_MATRIX.md database console; db_* commands |
| `debugger` | 43 | 1 | 1 | 1 | 0 | partial - debug.paused, debug.step.paused, debug.vars; internal/app/debug.go builds the file's application config and calls adapter.PrepareJavaLaunch before java_debug_server + debug_start; waitForJavaDebugReady holds launch through jdtls starting/busy and Stop cancels it, proven by debugimport.*; debug_view.go renders the session tab, toolbar and Console and debug.consoleEval exercises REPL evaluation; debug_memory.go reads live JVM class counts, tested by debug.memoryRows; instances, heap and concurrent sessions remain; java-debug missing offers one-click install; HotSwap uses java_build_workspace and debug_redefine_classes, proven by hotswap.* |
| `dependencies` | 0 | 0 | 0 | 1 | 0 | missing |
| `deployment` | 12 | 1 | 1 | 0 | 0 | missing |
| `dotnet` | 11 | 1 | 0 | 0 | 0 | missing |
| `github` | 9 | 1 | 1 | 1 | 1 | missing |
| `graphql` | 6 | 0 | 1 | 1 | 0 | missing |
| `http-client` | 10 | 0 | 0 | 3 | 0 | missing |
| `libraries` | 1 | 0 | 0 | 0 | 0 | missing |
| `logcat` | 0 | 1 | 0 | 0 | 0 | missing |
| `macros` | 5 | 0 | 1 | 1 | 0 | missing |
| `mcp` | 0 | 0 | 1 | 0 | 0 | missing |
| `notebook` | 0 | 0 | 0 | 0 | 0 | missing |
| `perf` | 4 | 0 | 1 | 0 | 0 | missing |
| `plugins` | 11 | 0 | 2 | 5 | 0 | missing |
| `profiler` | 4 | 1 | 0 | 0 | 0 | missing |
| `project-tree` | 55 | 1 | 3 | 0 | 0 | integrated - internal/app/tree.go, project_tree_actions.go, tree_ops.go and project_view.go implement OpenIDEA's project view, linked roots, selection, inline operations, menu options, View Modes and sort orders over workspace and fs RPCs; tree_test.go, project_tree_actions_test.go, tree_ops_test.go, project_view_test.go and base.files.* cover the surface |
| `project-trust` | 5 | 0 | 1 | 0 | 1 | integrated - internal/app/run.go implements OpenIDEA's Trust and Open Project? dialog, Restricted Mode, trust decisions and folder scope; internal/app/project_trust_settings.go implements Settings › Security › Project Trust over project_trust_list, project_trust_set, project_trust_add_location and project_trust_remove_location; internal/app/trust_test.go, internal/app/project_trust_settings_test.go, the trust screenshot and the ide-host RPC test prove the complete flow |
| `remote-dev` | 4 | 1 | 1 | 0 | 0 | missing |
| `run` | 29 | 1 | 5 | 2 | 0 | partial - base.run.config, base.run.exit, base.run.build; internal/adapter/java_launch.go and java_module.go port OpenIDEA's platform/javaLaunch.ts and Run and Debug share it; spring.* release-gate checks run the detected Spring Boot configuration of testdata/java-spring-multimodule; internal/adapter/run_config.go ports configs.ts (upsert/merge/validate/unique), buildCommand.ts (applyBuildPolicyToCommand) and store.ts saveUserConfigs, and internal/app/run_config_dialog.go carries Edit Configurations (action:run.editConfigs) and Rerun with the confirm dialog (action:run.rerun); base.runcfg.* proves the save/policy/run/remove round trip; Build Before Launch is buildCommand.ts's applyBuildPolicyToCommand (internal/adapter/run_config.go), applied at launch for build-tool configs and through BuildPolicyOf for JVM ones, proven by base.runcfg.policy; the testing pipeline port (internal/adapter/testing_*.go) folds surefire/Gradle output into the results tree and run.rerunFailed derives the -Dtest=/--tests filter (rerunfail.* main-gate checks); go test is parsed too (testing_go.go, gorerun.* main-gate checks); the cargo test pipeline (internal/adapter/testing_cargo.go ports cargo.ts: libtest/nextest parsing, panic locations) folds cargo test output into the tree and cargorun.* main-gate checks prove the --exact derivation reruns only the failed test; pytest is parsed too (testing_pytest.go, pyrerun.* main-gate checks); the js/phpunit/ruby/dotnet parsers are not ported yet (node/php/ruby toolchains exist but jest/phpunit/rspec need framework installs; dotnet is absent) |
| `run-anything` | 1 | 0 | 1 | 0 | 0 | partial - internal/app/run_anything.go Run Anything popup with Recent and Run Configurations sections, Clear History, per-project history and shell command lines; gear menu action:runAnything.show; Ctrl Ctrl keymap deferred, MyGo has no KeyControl constant; internal/app/run_anything_test.go |
| `scratch` | 1 | 0 | 1 | 0 | 0 | integrated - internal/app/commands.go and scratch.go; File menu and Cmd+Shift+N open the language chooser and create a scratch tab; internal/app/scratch_test.go and base.scratch checks |
| `search` | 42 | 3 | 6 | 2 | 0 | partial - base.search.done, base.search.files, base.search.matches and searchEverywhere.*; internal/app/se.go Search Everywhere tabs, search history and recent files/actions; se_filters.go and se_filters_test.go cover the filter funnel; recent_navigation.go adds Recent Files, Recent Locations and an open-file Switcher with guarded Delete; double-Shift and Switcher Ctrl-release behavior are tested. The Endpoints section still depends on plugin endpoint enumeration |
| `services` | 20 | 1 | 1 | 2 | 0 | missing |
| `settings` | 26 | 0 | 3 | 0 | 77 | partial - internal/app/settings_window.go; OpenIDEA's 980 DIP sheet: page tree built like model/pages.ts with group folders, search filtering with ancestors, breadcrumbs, Back/Forward history, Reset and Cancel/Apply/OK staging; internal/app/settings_test.go |
| `ssh` | 2 | 0 | 1 | 0 | 0 | missing |
| `templates` | 6 | 0 | 3 | 1 | 3 | partial - internal/app/project_new.go ports OpenIDEA's built-in file-template catalog, fuzzy chooser, name and kind prompt, JVM/Go/PSR-4 package inference, include and variable expansion and $END$ caret placement; project_new_test.go covers the catalog and rendering. The caret-anchored template chooser, live/postfix/Emmet templates and File and Code Templates settings remain missing |
| `terminal` | 14 | 1 | 0 | 0 | 0 | partial - internal/app/term.go tab groups over backend PTYs, SplitTerm/UnsplitTerm, ShowTermFind scrollback search, per-tab channel routing and exit by id; base.term.output, base.term.tabs, base.term.split, base.term.find; UPSTREAM_FEATURE_MATRIX.md Terminal |
| `todo` | 2 | 1 | 0 | 1 | 1 | missing |
| `tools` | 4 | 0 | 0 | 0 | 0 | partial - tools.jdtls, tools.java-debug |
| `vcs` | 161 | 3 | 2 | 2 | 1 | partial - base.vcs.changes, base.diff.rows, base.commit.id, base.log.*, base.branch.* and base.stage.*; internal/app/vcs_operations.go supplies branch/tag, fetch, pull, push, merge, rebase, stash/unstash, reset, cherry-pick/revert and continue/abort/skip through one native sheet and the OpenIDEA VCS Operations quick list; vcsoperations.* runs tag, branch, stash round trip, fetch, push, pull, rebase, merge and reset against a real local remote; remote management and worktrees remain |
| `viewers` | 3 | 0 | 0 | 1 | 0 | missing |
| `vim` | 3 | 0 | 0 | 1 | 1 | missing |
| `web` | 2 | 0 | 0 | 1 | 0 | missing |
| `welcome` | 2 | 0 | 1 | 0 | 0 | integrated - internal/app/welcome.go, welcome_recents.go and recent_projects.go implement OpenIDEA's Welcome rail, Recent Projects, groups, search, remove, Customize and Plugins pages; internal/app/welcome_test.go and the welcome-empty, welcome, welcome-customize, welcome-plugins and recent-projects screenshots cover the surface |
| `whats-new` | 1 | 0 | 0 | 0 | 0 | integrated - internal/app/whatsnew.go; Help menu and app.whatsNew open release notes with the update preference; TestWhatsNewIsAStandalonePage |

Infrastructure folders (`src/editor`, `src/workbench`, `src/platform`, `src/ui`)
are not product features; the contributions they host are listed below.

## Actions

756 registered action ids, plus 132 ids that only presets bind (376 in total).
Registered ids are listed by feature folder; keymap-only ids follow.

### agents

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `agents.addAgent` | More Agents… | `src/features/agents/index.ts` | integrated - internal/app/commands.go opens Settings › Agents › Coding Agents (agents.list) over AgentRegistryPage; internal/app/agent_registry_settings.go and TestAgentRegistryPageShowsProvidersAndLaunchSettings |
| `agents.archiveTasks` | Archive Agent Tasks | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go and agent_workspace.go archive the selected tasks sequentially and refresh with the backend archive RPC; TestAgentArchiveManyIsSequentialAndAggregatesErrors |
| `agents.closeAndNewTask` | Close and New Agent Task | `src/features/agents/index.ts` | integrated - internal/app/commands.go and agents.go close the active conversation and open New Task; internal/app/agents_test.go |
| `agents.closeTask` | Close Agent Task | `src/features/agents/index.ts` | integrated - internal/app/commands.go and agents.go close the active conversation while keeping its backend task; internal/app/agents_test.go |
| `agents.copyReplyMarkdown` | Copy Markdown | `src/features/agents/replyActions.ts` | integrated - internal/app/agent_actions.go copies the selected reply's original Markdown; TestAgentCompatibilityActionsAreRegistered and TestAgentActionsAcceptOpenIDEAArguments |
| `agents.deleteTasks` | Delete Agent Tasks | `src/features/agents/taskActions.ts` | integrated - internal/app/agents.go and agent_workspace.go require confirmation, delete selected tasks and close their tabs; internal/app/agent_organize_test.go and agent_editor_test.go |
| `agents.dockTask` | Open Agent Task in Tool Window | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go opens the task in the Agents tool window; TestAgentTaskToolWindowDoesNotCreateEditorTabs and TestAgentActionsAcceptOpenIDEAArguments |
| `agents.findInConversation` | Find in Conversation | `src/features/agents/index.ts` | integrated - internal/app/agents.go implements the conversation find bar and match count; TestAgentFindCountsEveryVisiblePartOfConversation and TestAgentFindBarTypesStepsAndCloses |
| `agents.forkReply` | Fork Conversation from Here | `src/features/agents/replyActions.ts` | integrated - internal/app/agent_actions.go forks the conversation at a reply through the backend; TestAgentCompatibilityActionsAreRegistered and TestAgentActionsAcceptOpenIDEAArguments |
| `agents.forkTask` | Fork Agent Task | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go and agents.go fork a task and add the returned task to the home; internal/app/agent_actions_test.go |
| `agents.jumpToLatest` | Jump to Latest | `src/features/agents/replyActions.ts` | integrated - internal/app/agent_actions.go scrolls the active conversation to its newest entry; TestAgentCompatibilityActionsAreRegistered |
| `agents.mcpServers` | External MCP Servers… | `src/features/agents/index.ts` | integrated - internal/app/commands.go opens Settings › Agents › External MCP Servers (agents.mcpServers); internal/app/agent_mcp_settings.go |
| `agents.newTask` | New Agent Task | `src/features/agents/index.ts` | integrated - internal/app/commands.go and agents.go open OpenIDEA's New Agent Task dialog; TestAgentCreatePromptAndPermissionRPCArguments |
| `agents.newTaskGroup` | New Agent Task Group | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go and agent_workspace.go create a named task group; internal/app/agent_organize_test.go |
| `agents.openSampleConversations` | Open Sample Conversations | `src/features/agents/index.ts` | integrated - internal/app/agent_actions.go and agent_samples.go open deterministic long sample conversations without backend state; TestAgentSampleConversationsOpenLongTabsWithoutBackend |
| `agents.openTask` | Open Agent Task | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go opens a task in its configured editor or tool-window location; TestAgentTaskDefaultsToEditorAndReusesItsTab and TestAgentActionsAcceptOpenIDEAArguments |
| `agents.openTaskInEditor` | Open Agent Task in Editor | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_editor.go opens and reuses the task's editor tab; TestAgentTaskDefaultsToEditorAndReusesItsTab |
| `agents.pinTasks` | Pin Agent Tasks | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go and agent_workspace.go pin the selected tasks and persist their order; internal/app/agent_organize_test.go |
| `agents.quoteReply` | Quote Reply | `src/features/agents/replyActions.ts` | integrated - internal/app/agent_actions.go inserts a quoted reply into the active composer; TestAgentCompatibilityActionsAreRegistered and TestAgentActionsAcceptOpenIDEAArguments |
| `agents.regenerateReply` | Regenerate Response | `src/features/agents/replyActions.ts` | integrated - internal/app/agent_actions.go retracts and regenerates the selected response through the backend; internal/app/agent_actions_test.go |
| `agents.removeTaskGroup` | Remove Agent Task Group | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_workspace.go requires confirmation before removing a group; TestAgentRemoveTaskGroupRequiresConfirmation |
| `agents.renameTask` | Rename Agent Task | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go and agents.go rename the selected task through the backend; internal/app/agent_actions_test.go |
| `agents.renameTaskGroup` | Rename Agent Task Group | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_workspace.go renames a task group and persists the organization; internal/app/agent_organize_test.go |
| `agents.retractMessage` | Retract Message | `src/features/agents/replyActions.ts` | integrated - internal/app/agent_actions.go opens the message editor and retracts the selected prompt through the backend; internal/app/agent_actions_test.go |
| `agents.retryConversation` | Retry Loading Conversation | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go reloads a failed conversation; internal/app/agent_actions_test.go |
| `agents.sendToAgent` | Send to Agent | `src/features/agents/index.ts` | integrated - internal/app/commands.go and agents.go send the editor selection and references to the active agent; internal/app/agent_composer_test.go |
| `agents.showToolWindow` | Agents | `src/features/agents/index.ts` | integrated - internal/app/commands.go and agents.go open the right-side Agents window; TestAgentsToolWindowRendersOnTheRight |
| `agents.unarchiveTasks` | Unarchive Agent Tasks | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go and agent_workspace.go unarchive the selected tasks through the backend; internal/app/agent_actions_test.go |
| `agents.undockTask` | Open Agent Task in Editor | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go moves the task into an editor tab; TestAgentTaskDefaultsToEditorAndReusesItsTab |
| `agents.unpinTasks` | Unpin Agent Tasks | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_actions.go and agent_workspace.go unpin selected tasks through the backend; internal/app/agent_actions_test.go |
| `agents.useEditor` | Chat in Editor | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_editor.go moves the active task into the editor and preserves its composer draft; TestAgentSessionLocationPreservesComposerDraft |
| `agents.useToolWindow` | Chat in Tool Window | `src/features/agents/taskActions.ts` | integrated - internal/app/agent_editor.go moves the active task into the right Agents window; TestAgentTaskToolWindowDoesNotCreateEditorTabs |

### analysis

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `analyze.inspectCode` | Inspect Code… | `src/features/analysis/index.ts` | missing |
| `analyze.inspectFile` | Inspect Current File | `src/features/analysis/index.ts` | missing |
| `analyze.locateDuplicates` | Locate Duplicates… | `src/features/analysis/index.ts` | missing |
| `code.cleanup` | Code Cleanup… | `src/features/analysis/index.ts` | missing |
| `find.replaceStructural` | Replace Structurally… | `src/features/analysis/index.ts` | missing |
| `find.structural` | Search Structurally… | `src/features/analysis/index.ts` | missing |

### app

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `app.about` |  | `src/features/app/appActions.ts` | partial - internal/app registers the native action `app.about`; interaction-level acceptance is pending |
| `app.checkForUpdates` |  | `src/features/app/appActions.ts` | partial - internal/app registers the native action `app.checkForUpdates`; interaction-level acceptance is pending |
| `app.installCli` |  | `src/features/app/cliCommand.ts` | partial - internal/app registers the native action `app.installCli`; interaction-level acceptance is pending |
| `app.quit` |  | `src/features/app/appActions.ts` | partial - internal/app registers the native action `app.quit`; interaction-level acceptance is pending |
| `app.restart` | Restart IDE | `src/features/app/InvalidateCaches.tsx` | integrated - internal/app/commands.go app.restart -> internal/app/invalidate.go RestartIDE, which writes an empty invalidation marker and restarts through the quit flow, so unsaved buffers still prompt; ApplyPendingInvalidation deletes nothing for it; File menu Restart IDE; internal/app/invalidate_test.go TestRestartScheduleDeletesNothing |
| `app.uninstallCli` |  | `src/features/app/cliCommand.ts` | partial - internal/app registers the native action `app.uninstallCli`; interaction-level acceptance is pending |
| `edit.copy` |  | `src/features/app/appActions.ts` | integrated - internal/app/commands.go registers OpenIDEA's edit.copy; runEditorCommand executes editor.Copy through MyGo's native clipboard service; TestBasicEditorActionsRunThroughTheRegistry and TestEditorActionsRespectEditorAvailability |
| `edit.cut` |  | `src/features/app/appActions.ts` | integrated - internal/app/commands.go registers OpenIDEA's edit.cut; runEditorCommand executes editor.Cut with read-only gating; TestBasicEditorActionsRunThroughTheRegistry and TestEditorActionsRespectEditorAvailability |
| `edit.paste` |  | `src/features/app/appActions.ts` | integrated - internal/app/commands.go registers OpenIDEA's edit.paste; runEditorCommand executes editor.Paste with read-only gating; TestBasicEditorActionsRunThroughTheRegistry and TestEditorActionsRespectEditorAvailability |
| `edit.redo` |  | `src/features/app/appActions.ts` | integrated - internal/app/commands.go registers OpenIDEA's edit.redo; runEditorCommand executes editor.Redo on the active native Buffer and actions_builtin.go enables it from CanRedo; TestBasicEditorActionsRunThroughTheRegistry |
| `edit.selectAll` |  | `src/features/app/appActions.ts` | integrated - internal/app/commands.go registers OpenIDEA's edit.selectAll; runEditorCommand executes editor.SelectAll on the active native editor; TestBasicEditorActionsRunThroughTheRegistry and TestEditorActionsRespectEditorAvailability |
| `edit.undo` |  | `src/features/app/appActions.ts` | integrated - internal/app/commands.go registers OpenIDEA's edit.undo; runEditorCommand executes editor.Undo on the active native Buffer and actions_builtin.go enables it from CanUndo; TestBasicEditorActionsRunThroughTheRegistry |
| `file.exportSettings` | Export Settings… | `src/features/app/projectActions.ts` | missing |
| `file.importJetBrainsSettings` | Import Settings from JetBrains IDE… | `src/features/app/projectActions.ts` | missing |
| `file.importSettings` | Import Settings… | `src/features/app/projectActions.ts` | missing |
| `file.invalidateCaches` | Invalidate Caches… | `src/features/app/InvalidateCaches.tsx` | integrated - internal/app/commands.go file.invalidateCaches -> internal/app/invalidate.go OpenInvalidateCaches, the dialog with the three optional caches (Local History, language server workspaces, build tool caches) and Invalidate and Restart / Just Restart / Cancel; it writes <data>/invalidate.json and restarts through the quit flow (mygo App.Relaunch), and ApplyPendingInvalidation deletes the chosen openidea-server caches (data/history, data/lsp/jdtls, data/cache) before the next server start, logging a failed deletion; internal/app/invalidate_test.go TestInvalidationScheduleAppliesAtStart, TestRestartScheduleDeletesNothing; base.commands.invalidate |
| `file.newProject` | New Project… | `src/features/app/projectActions.ts` | partial - internal/app registers the native action `file.newProject`; interaction-level acceptance is pending |
| `file.openFile` |  | `src/features/app/windows.ts` | partial - internal/app registers the native action `file.openFile`; interaction-level acceptance is pending |
| `file.openInNewWindow` |  | `src/features/app/windows.ts` | missing |
| `file.openPaths` | Open Paths | `src/features/app/windows.ts` | partial - internal/app registers the native action `file.openPaths`; interaction-level acceptance is pending |
| `file.openProjectPath` | Open Project Path | `src/features/app/windows.ts` | partial - internal/app registers the native action `file.openProjectPath`; interaction-level acceptance is pending |
| `file.openRecentProject` |  | `src/features/app/windows.ts` | partial - internal/app registers the native action `file.openRecentProject`; interaction-level acceptance is pending |
| `file.renameProject` | Rename Project… | `src/features/app/projectActions.ts` | missing |
| `file.togglePowerSave` | Power Save Mode | `src/features/app/projectActions.ts` | missing |
| `help.keymapReference` |  | `src/features/app/appActions.ts` | partial - internal/app registers the native action `help.keymapReference`; interaction-level acceptance is pending |
| `help.showLog` |  | `src/features/app/appActions.ts` | partial - internal/app registers the native action `help.showLog`; interaction-level acceptance is pending |
| `lightEdit.openProject` |  | `src/features/app/windows.ts` | partial - internal/app registers the native action `lightEdit.openProject`; interaction-level acceptance is pending |
| `project.attach` |  | `src/features/app/windows.ts` | integrated - internal/app/project_workspace.go AttachProject, attachWorkspaceProject and the project.reveal selection; base.workspace.attached runs the real Attach Project flow and internal/app/project_workspace_test.go covers trust and the retained editor root |
| `project.detach` | Detach Project | `src/features/app/windows.ts` | integrated - internal/app/project_workspace.go RemoveWorkspaceProject confirms a non-primary attachment and calls workspace_update; base.workspace.detached and internal/app/project_workspace_test.go |
| `window.closeOtherTabs` | Close Other Project Tabs | `src/features/app/windows.ts` | partial - internal/app registers the native action `window.closeOtherTabs`; interaction-level acceptance is pending |
| `window.closeTab` | Close Project Tab | `src/features/app/windows.ts` | partial - internal/app registers the native action `window.closeTab`; interaction-level acceptance is pending |
| `window.closeTabsToLeft` | Close Project Tabs to the Left | `src/features/app/windows.ts` | partial - internal/app registers the native action `window.closeTabsToLeft`; interaction-level acceptance is pending |
| `window.closeTabsToRight` | Close Project Tabs to the Right | `src/features/app/windows.ts` | partial - internal/app registers the native action `window.closeTabsToRight`; interaction-level acceptance is pending |
| `window.detachTab` | Move Tab to New Window | `src/features/app/windows.ts` | missing |
| `window.mergeAllWindows` | Merge All Project Windows | `src/features/app/windows.ts` | missing |
| `window.new` |  | `src/features/app/windows.ts` | missing |
| `window.next` |  | `src/features/app/windows.ts` | missing |
| `window.nextTab` | Next Project Tab | `src/features/app/windows.ts` | partial - internal/app registers the native action `window.nextTab`; interaction-level acceptance is pending |
| `window.previous` |  | `src/features/app/windows.ts` | missing |
| `window.previousTab` | Previous Project Tab | `src/features/app/windows.ts` | partial - internal/app registers the native action `window.previousTab`; interaction-level acceptance is pending |
| `window.reorderTabs` | Reorder Project Tabs | `src/features/app/windows.ts` | partial - internal/app registers the native action `window.reorderTabs`; interaction-level acceptance is pending |
| `window.selectTab` | Select Project Tab | `src/features/app/windows.ts` | partial - internal/app registers the native action `window.selectTab`; interaction-level acceptance is pending |
| `window.toggleFullScreen` |  | `src/features/app/windows.ts` | partial - internal/app/zen.go ToggleFullScreen through mygo Window.ToggleFullScreen; the native full-screen transition is not exercised headless |
| `workspace.create` | New Workspace… | `src/features/app/windows.ts` | integrated - internal/app/project_workspace.go OpenNewWorkspace, saveWorkspaceDialog and workspace_create; base.workspace.created opens the real container and internal/app/project_workspace_test.go covers the request |
| `workspace.manage` | Manage Workspace… | `src/features/app/windows.ts` | integrated - internal/app/project_workspace.go OpenManageWorkspace and workspace_update preserve the container; base.workspace.managed renames it and links a third real project |
| `workspace.open` | Open Workspace… | `src/features/app/windows.ts` | partial - internal/app/project_workspace.go OpenWorkspace selects a folder and uses the shared project-switch path; internal/app/project_workspace_test.go covers workspace RPC lifecycle, but the native folder-picker callback has no automated gate |
| `workspace.removeProject` | Remove from Workspace | `src/features/app/windows.ts` | integrated - internal/app/project_workspace.go RemoveWorkspaceProject and updateWorkspaceProjects keep the container state root; base.workspace.removed and internal/app/project_workspace_test.go |

### build

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `build.buildArtifacts` | Build Artifacts | `src/features/build/index.ts` | partial - internal/app/build.go BuildArtifacts (build_run kind=artifacts) |
| `build.buildModule` | Build Module | `src/features/build/index.ts` | partial - internal/app/build.go BuildModule; base.build.success |
| `build.buildProject` | Build Project | `src/features/build/index.ts` | integrated - base.build.success, base.build.lines, base.build.console |
| `build.compileFile` | Compile File | `src/features/build/index.ts` | integrated - internal/app/build.go compiles one Java source over build_compile_file; Build menu, action registry, editor, tab and Project view context menus; base.compileFile.success runs real javac in the Maven fixture |
| `build.rebuildProject` | Rebuild Project | `src/features/build/index.ts` | partial - internal/app/build.go RebuildProject (build_run kind=rebuild) |
| `build.stop` | Stop Build | `src/features/build/index.ts` | partial - internal/app/build.go StopBuild (cancel_request) |

### build-tools

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `gradle.analyzeDependencies` | Analyze Dependencies… | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `gradle.analyzeDependencies`; interaction-level acceptance is pending |
| `gradle.cancelReload` | Stop Reloading Gradle Projects | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `gradle.cancelReload`; interaction-level acceptance is pending |
| `gradle.collapseAll` | Collapse All | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `gradle.collapseAll`; interaction-level acceptance is pending |
| `gradle.executeTask` | Execute Gradle Task… | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `gradle.executeTask`; interaction-level acceptance is pending |
| `gradle.expandAll` | Expand All | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `gradle.expandAll`; interaction-level acceptance is pending |
| `gradle.linkProjects` | Link Gradle Projects | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `gradle.linkProjects`; interaction-level acceptance is pending |
| `gradle.reload` | Reload All Gradle Projects | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `gradle.reload`; interaction-level acceptance is pending |
| `gradle.toggleOffline` | Toggle Offline Mode | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `gradle.toggleOffline`; interaction-level acceptance is pending |
| `gradle.toggleSkipTests` | Toggle 'Skip Tests' Mode | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `gradle.toggleSkipTests`; interaction-level acceptance is pending |
| `gradle.unlinkProjects` | Unlink Gradle Project | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `gradle.unlinkProjects`; interaction-level acceptance is pending |
| `maven.addProjects` | Add Maven Projects | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.addProjects`; interaction-level acceptance is pending |
| `maven.analyzeDependencies` | Analyze Dependencies… | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.analyzeDependencies`; interaction-level acceptance is pending |
| `maven.cancelReload` | Stop Reloading Maven Projects | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.cancelReload`; interaction-level acceptance is pending |
| `maven.collapseAll` | Collapse All | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.collapseAll`; interaction-level acceptance is pending |
| `maven.executeGoal` | Execute Maven Goal… | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.executeGoal`; interaction-level acceptance is pending |
| `maven.expandAll` | Expand All | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.expandAll`; interaction-level acceptance is pending |
| `maven.generateSources` | Generate Sources and Update Folders For All Projects | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.generateSources`; interaction-level acceptance is pending |
| `maven.importExternal` | Import Module from Existing Sources | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.importExternal`; interaction-level acceptance is pending |
| `maven.reload` | Reload All Maven Projects | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.reload`; interaction-level acceptance is pending |
| `maven.settings` | Maven Settings… | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.settings`; interaction-level acceptance is pending |
| `maven.toggleOffline` | Toggle Offline Mode | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.toggleOffline`; interaction-level acceptance is pending |
| `maven.toggleSkipTests` | Toggle 'Skip Tests' Mode | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.toggleSkipTests`; interaction-level acceptance is pending |
| `maven.unlinkProjects` | Unlink Maven Projects | `src/features/build-tools/index.ts` | partial - internal/app registers the native action `maven.unlinkProjects`; interaction-level acceptance is pending |

### code-insight

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `find.showUsageResults` | Show Usage Results | `src/features/code-insight/index.ts` | integrated - internal/app/commands.go registers OpenIDEA's find.showUsageResults and the code.showUsageResults alias; internal/app/action.go dispatches it to the same exact-result transfer used by Show Usages, while lsp.usagesPopup.transfer proves the native popup hands all rows to Find Usages without issuing a second reference search |
| `find.showUsages` | Show Usages | `src/features/code-insight/index.ts` | integrated - internal/app/usage_popup.go implements the native Show Usages popup behind OpenIDEA's find.showUsages action and ⌥⌘F7, with streamed references, filename and source-line speed search, scope widening, access filters, source preview and transfer to Find Usages; lsp.usagesPopup.open and lsp.usagesPopup.transfer pass against real JDT, and usage_popup_test.go covers navigation, filtering, persistence, stale responses, early streaming and rendering |
| `find.usages` | Find Usages | `src/features/code-insight/index.ts` | integrated - lsp.references.count, lsp.references.done |
| `navigate.callHierarchy` | Call Hierarchy | `src/features/code-insight/index.ts` | integrated - internal/app/commands.go navigate.callHierarchy Ctrl+Alt+H; internal/app/hierarchy.go OpenCallHierarchy over lsp_call_hierarchy; Navigate menu Call Hierarchy |
| `navigate.typeHierarchy` | Type Hierarchy | `src/features/code-insight/index.ts` | integrated - internal/app/commands.go navigate.typeHierarchy Ctrl+H; internal/app/hierarchy.go OpenTypeHierarchy over lsp_type_hierarchy; Navigate menu Type Hierarchy |
| `usages.rerun` | Rerun Find Usages | `src/features/code-insight/index.ts` | partial - internal/app registers the native action `usages.rerun`; interaction-level acceptance is pending |

### coverage

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `coverage.hide` | Hide Coverage | `src/features/coverage/index.ts` | integrated - internal/app/coverage.go SetCoverageVisible removes every active-suite decoration while retaining reports; TestCoverageActionsFollowOpenIDEAContextState verifies enabled state and results |
| `coverage.importReport` | Import a Coverage Report… | `src/features/coverage/index.ts` | integrated - base.coverage.report |
| `coverage.manageReports` | Manage Coverage Reports… | `src/features/coverage/index.ts` | integrated - internal/app/coverage.go and coverage_view.go render OpenIDEA's Manage Coverage Reports overlay with suite selection, imported-report removal and source-report preservation; TestCoverageReportsRestoreWithoutDeletingSourceReport and TestCoverageActionsFollowOpenIDEAContextState cover the native flow |
| `coverage.show` | Show Coverage | `src/features/coverage/index.ts` | integrated - internal/app/coverage.go SetCoverageVisible restores the active suite's editor and tree decorations and opens the Coverage window; TestCoverageActionsFollowOpenIDEAContextState verifies enabled state and results |
| `run.coverage` | Run with Coverage | `src/features/coverage/index.ts` | integrated - internal/app/coverage_run.go prepares the selected configuration, runs it through the normal Run lifecycle, waits for exit and collects the report; TestCoverageCollectsAnExitBeforeTheRunStartReply covers the short-lived process reply race |
| `run.coverageContext` | Run Context Configuration with Coverage | `src/features/coverage/index.ts` | partial - internal/app registers the native action `run.coverageContext`; interaction-level acceptance is pending |

### database

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `db.closeAllResultTabs` | Close All | `src/features/database/resultTabActions.ts` | partial - internal/app registers the native action `db.closeAllResultTabs`; interaction-level acceptance is pending |
| `db.closeOtherResultTabs` | Close Others | `src/features/database/resultTabActions.ts` | partial - internal/app registers the native action `db.closeOtherResultTabs`; interaction-level acceptance is pending |
| `db.closeResultTab` | Close | `src/features/database/resultTabActions.ts` | partial - internal/app registers the native action `db.closeResultTab`; interaction-level acceptance is pending |
| `db.collapseAll` | Collapse All | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.collapseAll`; interaction-level acceptance is pending |
| `db.copyReference` | Copy Reference | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.copyReference`; interaction-level acceptance is pending |
| `db.ddlToClipboard` | Generate DDL to Clipboard | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.ddlToClipboard`; interaction-level acceptance is pending |
| `db.ddlToConsole` | Generate DDL to Query Console | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.ddlToConsole`; interaction-level acceptance is pending |
| `db.disconnect` | Deactivate | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.disconnect`; interaction-level acceptance is pending |
| `db.drop` | Drop… | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.drop`; interaction-level acceptance is pending |
| `db.duplicateSource` | Duplicate | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.duplicateSource`; interaction-level acceptance is pending |
| `db.editData` | Edit Data | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.editData`; interaction-level acceptance is pending |
| `db.execute` | Execute SQL Statement | `src/features/database/index.ts` | partial - internal/app registers the native action `db.execute`; interaction-level acceptance is pending |
| `db.executeScript` | Execute SQL Script | `src/features/database/index.ts` | partial - internal/app registers the native action `db.executeScript`; interaction-level acceptance is pending |
| `db.explainPlan` | Explain Plan | `src/features/database/index.ts` | partial - internal/app registers the native action `db.explainPlan`; interaction-level acceptance is pending |
| `db.findInValue` | Find | `src/features/database/valueEditorActions.ts` | partial - internal/app registers the native action `db.findInValue`; interaction-level acceptance is pending |
| `db.goToColumn` | Go to Column | `src/features/database/index.ts` | partial - internal/app registers the native action `db.goToColumn`; interaction-level acceptance is pending |
| `db.goToDdl` | Go to DDL | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.goToDdl`; interaction-level acceptance is pending |
| `db.goToReferencedTable` | Go to Referenced Table | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.goToReferencedTable`; interaction-level acceptance is pending |
| `db.importData` | Import Data from File… | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.importData`; interaction-level acceptance is pending |
| `db.introspect` | Introspect All Databases | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.introspect`; interaction-level acceptance is pending |
| `db.jumpToConsole` | Jump to Query Console… | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.jumpToConsole`; interaction-level acceptance is pending |
| `db.modifyTable` | Modify Table… | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.modifyTable`; interaction-level acceptance is pending |
| `db.new` | New… | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.new`; interaction-level acceptance is pending |
| `db.newColumn` | Column | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.newColumn`; interaction-level acceptance is pending |
| `db.newDataSource` | New Data Source… | `src/features/database/index.ts` | partial - internal/app registers the native action `db.newDataSource`; interaction-level acceptance is pending |
| `db.newTable` | Table | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.newTable`; interaction-level acceptance is pending |
| `db.openConsole` | Query Console | `src/features/database/index.ts` | partial - internal/app registers the native action `db.openConsole`; interaction-level acceptance is pending |
| `db.openValueWindow` | Open in Window | `src/features/database/valueEditorActions.ts` | partial - internal/app registers the native action `db.openValueWindow`; interaction-level acceptance is pending |
| `db.pluginConnect` | Connect Plugin Data Source | `src/features/database/index.ts` | partial - internal/app registers the native action `db.pluginConnect`; interaction-level acceptance is pending |
| `db.properties` | Data Source Properties… | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.properties`; interaction-level acceptance is pending |
| `db.queryHistory` | Browse Query History | `src/features/database/index.ts` | partial - internal/app registers the native action `db.queryHistory`; interaction-level acceptance is pending |
| `db.quickDoc` | Quick Documentation | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.quickDoc`; interaction-level acceptance is pending |
| `db.reformatValue` | Reformat | `src/features/database/valueEditorActions.ts` | partial - internal/app registers the native action `db.reformatValue`; interaction-level acceptance is pending |
| `db.refresh` | Refresh | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.refresh`; interaction-level acceptance is pending |
| `db.rename` | Rename… | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.rename`; interaction-level acceptance is pending |
| `db.renameResultTab` | Rename Tab | `src/features/database/resultTabActions.ts` | partial - internal/app registers the native action `db.renameResultTab`; interaction-level acceptance is pending |
| `db.revertChanges` | Revert | `src/features/database/index.ts` | partial - internal/app registers the native action `db.revertChanges`; interaction-level acceptance is pending |
| `db.runSqlScript` | Run SQL Script… | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.runSqlScript`; interaction-level acceptance is pending |
| `db.separateProceduresAndFunctions` | Separate Procedures and Functions | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.separateProceduresAndFunctions`; interaction-level acceptance is pending |
| `db.setCellDefault` | Set DEFAULT | `src/features/database/cellActions.ts` | partial - internal/app registers the native action `db.setCellDefault`; interaction-level acceptance is pending |
| `db.setCellNull` | Set NULL | `src/features/database/cellActions.ts` | partial - internal/app registers the native action `db.setCellNull`; interaction-level acceptance is pending |
| `db.showToolWindow` | Database | `src/features/database/index.ts` | partial - internal/app registers the native action `db.showToolWindow`; interaction-level acceptance is pending |
| `db.sqlScript` | SQL Script Template | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.sqlScript`; interaction-level acceptance is pending |
| `db.submitChanges` | Submit | `src/features/database/index.ts` | partial - internal/app registers the native action `db.submitChanges`; interaction-level acceptance is pending |
| `db.toggleBooleanCell` | Toggle Boolean Cell | `src/features/database/cellActions.ts` | partial - internal/app registers the native action `db.toggleBooleanCell`; interaction-level acceptance is pending |
| `db.truncate` | Truncate… | `src/features/database/treeActions.ts` | partial - internal/app registers the native action `db.truncate`; interaction-level acceptance is pending |

### debugger

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `debug.attach` | Attach Debugger | `src/features/debugger/actions.ts` | partial - internal/app/attach.go ports attach.ts and java.ts's javaAttachConfig: debug_attachable_processes enumeration, the Java (JDWP) group with the word filter, and the attach request (cwd, javaProjectName, hostName, port, 30 s timeout) with the suspend=y auto-resume; attach.* release-gate checks attach to a real suspended JDWP JVM and require the program's output in the run pane; limits: GoIDEA ships java-debug only, so the popup lists the Java group, and there is no listen-mode remote debug |
| `debug.attachToProcess` | Attach to Process… | `src/features/debugger/actions.ts` | partial - OpenIDEA id `debug.attachToProcess` aliases GoIDEA action `debug.attach`; internal/app/attach.go ports attach.ts and java.ts's javaAttachConfig: debug_attachable_processes enumeration, the Java (JDWP) group with the word filter, and the attach request (cwd, javaProjectName, hostName, port, 30 s timeout) with the suspend=y auto-resume; attach.* release-gate checks attach to a real suspended JDWP JVM and require the program's output in the run pane; limits: GoIDEA ships java-debug only, so the popup lists the Java group, and there is no listen-mode remote debug |
| `debug.browseConsoleHistory` | Browse Console History | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.browseConsoleHistory`; interaction-level acceptance is pending |
| `debug.cancelHotSwap` | Cancel Hot Swap Build | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.cancelHotSwap`; interaction-level acceptance is pending |
| `debug.evaluate` | Evaluate Expression… | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.evaluate`; interaction-level acceptance is pending |
| `debug.forceRunToCursor` | Force Run to Cursor | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `run.forceRunToCursor`; interaction-level acceptance is pending |
| `debug.forceStepInto` | Force Step Into | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.forceStepInto`; interaction-level acceptance is pending |
| `debug.forceStepOver` | Force Step Over | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.forceStepOver`; interaction-level acceptance is pending |
| `debug.heap.capture` | Capture Heap Snapshot | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.heap.capture`; interaction-level acceptance is pending |
| `debug.heap.open` | Open Heap Dump… | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.heap.open`; interaction-level acceptance is pending |
| `debug.heap.saveDump` | Save Heap Dump As… | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.heap.saveDump`; interaction-level acceptance is pending |
| `debug.hotReload` | Hot Reload | `src/features/debugger/actions.ts` | partial - OpenIDEA id `debug.hotReload` aliases GoIDEA action `debug.reloadChangedClasses`; internal/app/hotswap.go HotSwapReload ports hotswap.ts: saveAll, java_build_workspace over the session's cwd, debug_redefine_classes into the session, hotSwapMessage wording (nothing to reload / N classes reloaded / obsolete methods warning / failure refuses), and the hotCodeReplace BUILD_COMPLETE event offers the reload (IDEA's Ask); hotswap.* release-gate checks pause at a breakpoint, reload the edited Greeter class and require the NEW text in the program's output; limits: no per-session caps gate, no Reset Frame action, no cancel-build UI |
| `debug.hotRestart` | Hot Restart | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.hotRestart`; interaction-level acceptance is pending |
| `debug.jumpToSource` | Jump to Source | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.jumpToSource`; interaction-level acceptance is pending |
| `debug.memory.closeInstances` | Close Instances | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.memory.closeInstances`; interaction-level acceptance is pending |
| `debug.memory.jumpToSource` | Jump to Source | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.memory.jumpToSource`; interaction-level acceptance is pending |
| `debug.memory.refresh` | Refresh Memory View | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.memory.refresh`; interaction-level acceptance is pending |
| `debug.memory.showInstances` | Show Instances | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.memory.showInstances`; interaction-level acceptance is pending |
| `debug.memory.showReferrers` | Show Referrers | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.memory.showReferrers`; interaction-level acceptance is pending |
| `debug.muteBreakpoints` | Mute Breakpoints | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.muteBreakpoints`; interaction-level acceptance is pending |
| `debug.pause` | Pause Program | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `run.pause`; interaction-level acceptance is pending |
| `debug.pluginStart` | Debug Plugin Configuration | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.pluginStart`; interaction-level acceptance is pending |
| `debug.quickEvaluate` | Quick Evaluate Expression | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.quickEvaluate`; interaction-level acceptance is pending |
| `debug.reloadChangedClasses` | Reload Changed Classes | `src/features/debugger/actions.ts` | partial - internal/app/hotswap.go HotSwapReload ports hotswap.ts: saveAll, java_build_workspace over the session's cwd, debug_redefine_classes into the session, hotSwapMessage wording (nothing to reload / N classes reloaded / obsolete methods warning / failure refuses), and the hotCodeReplace BUILD_COMPLETE event offers the reload (IDEA's Ask); hotswap.* release-gate checks pause at a breakpoint, reload the edited Greeter class and require the NEW text in the program's output; limits: no per-session caps gate, no Reset Frame action, no cancel-build UI |
| `debug.removeAllBreakpoints` | Remove All Breakpoints | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.removeAllBreakpoints`; interaction-level acceptance is pending |
| `debug.rerun` | Rerun | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.rerun`; interaction-level acceptance is pending |
| `debug.resetFrame` | Reset Frame | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.resetFrame`; interaction-level acceptance is pending |
| `debug.resume` | Resume Program | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `run.continue`; interaction-level acceptance is pending |
| `debug.runToCursor` | Run to Cursor | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `run.runToCursor`; interaction-level acceptance is pending |
| `debug.setValue` | Set Value… | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.setValue`; interaction-level acceptance is pending |
| `debug.showExecutionPoint` | Show Execution Point | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.showExecutionPoint`; interaction-level acceptance is pending |
| `debug.showToolWindow` | Show Debug Tool Window | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `view.debug`; interaction-level acceptance is pending |
| `debug.smartStepInto` | Smart Step Into | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.smartStepInto`; interaction-level acceptance is pending |
| `debug.startConfig` | Debug… | `src/features/debugger/actions.ts` | partial - OpenIDEA id `debug.startConfig` aliases GoIDEA action `run.debug`; internal/app/debug.go Debug/DebugRunConfig: debug the SELECTED run configuration over the shared PrepareJavaLaunch + javaDebugLaunch (F9, run.debug); non-JVM kinds and unshipped adapters (kotlin-debug-adapter) are refused with the real reason; negatives.dbgcfg.session debugs the detected application configuration to its end on the real server; limits: gradle/maven debugging delegates are not ported |
| `debug.stepInto` |  | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `run.stepInto`; interaction-level acceptance is pending |
| `debug.stepOut` |  | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `run.stepOut`; interaction-level acceptance is pending |
| `debug.stepOver` |  | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `run.stepOver`; interaction-level acceptance is pending |
| `debug.stop` | Stop | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.stop`; interaction-level acceptance is pending |
| `debug.toggleBreakpoint` | Toggle Line Breakpoint | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `run.breakpoint`; interaction-level acceptance is pending |
| `debug.toggleFieldWatchpoint` | Toggle Field Watchpoint | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.toggleFieldWatchpoint`; interaction-level acceptance is pending |
| `debug.toggleMethodBreakpoint` | Toggle Method Breakpoint | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.toggleMethodBreakpoint`; interaction-level acceptance is pending |
| `debug.toggleTemporaryBreakpoint` | Toggle Temporary Line Breakpoint | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.toggleTemporaryBreakpoint`; interaction-level acceptance is pending |
| `debug.viewBreakpoints` | View Breakpoints… | `src/features/debugger/actions.ts` | partial - internal/app registers the native action `debug.viewBreakpoints`; interaction-level acceptance is pending |

### deployment

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `deployment.browseRemoteHost` | Browse Remote Host | `src/features/deployment/actions.tsx` | missing |
| `deployment.configure` | Deployment Configuration… | `src/features/deployment/actions.tsx` | missing |
| `deployment.deleteFromServer` | Delete from Server | `src/features/deployment/actions.tsx` | missing |
| `deployment.downloadFrom` | Download from… | `src/features/deployment/actions.tsx` | missing |
| `deployment.downloadFromDefault` | Download from Default Server | `src/features/deployment/actions.tsx` | missing |
| `deployment.downloadRemoteInto` | Download Remote Files Into Folder | `src/features/deployment/actions.tsx` | missing |
| `deployment.openInBrowser` | Open in Browser (Deployed) | `src/features/deployment/actions.tsx` | missing |
| `deployment.syncWith` | Sync with Deployed to… | `src/features/deployment/actions.tsx` | missing |
| `deployment.syncWithDefault` | Sync with Deployed to Default Server | `src/features/deployment/actions.tsx` | missing |
| `deployment.toggleAutoUpload` | Automatic Upload | `src/features/deployment/actions.tsx` | missing |
| `deployment.uploadTo` | Upload to… | `src/features/deployment/actions.tsx` | missing |
| `deployment.uploadToDefault` | Upload to Default Server | `src/features/deployment/actions.tsx` | missing |

### dotnet

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `dotnet.${op}` |  | `src/features/dotnet/index.ts` | missing |
| `dotnet.cancel` | Cancel .NET Operation | `src/features/dotnet/index.ts` | missing |
| `dotnet.discover` | Discover .NET Tests | `src/features/dotnet/index.ts` | missing |
| `dotnet.installTrx` | Install TRX reporting | `src/features/dotnet/index.ts` | missing |
| `dotnet.packageChange` | Change NuGet Package | `src/features/dotnet/index.ts` | missing |
| `dotnet.packages` | Refresh NuGet Packages | `src/features/dotnet/index.ts` | missing |
| `dotnet.reload` | Reload .NET Workspace | `src/features/dotnet/index.ts` | missing |
| `dotnet.search` | Search NuGet Packages | `src/features/dotnet/index.ts` | missing |
| `dotnet.select` | Select .NET Project | `src/features/dotnet/index.ts` | missing |
| `dotnet.show` | .NET Workspace | `src/features/dotnet/index.ts` | missing |
| `dotnet.test` | Run .NET Tests | `src/features/dotnet/index.ts` | missing |

### github

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `github.pullRequest.diff.addComment` | Add Review Comment | `src/features/github/diff/index.ts` | missing |
| `github.pullRequest.diff.addMultilineComment` | Add Multiline Review Comment | `src/features/github/diff/index.ts` | missing |
| `github.pullRequest.diff.nextComment` | Next Comment | `src/features/github/diff/index.ts` | missing |
| `github.pullRequest.diff.previousComment` | Previous Comment | `src/features/github/diff/index.ts` | missing |
| `github.pullRequest.diff.refreshReview` | Refresh Review Data | `src/features/github/diff/index.ts` | missing |
| `github.pullRequest.reviewMode` | Review Mode | `src/features/github/index.ts` | missing |
| `github.pullRequest.reviewMode.showDiff` | Show Review Diff | `src/features/github/reviewMode/index.ts` | missing |
| `github.pullRequest.showInToolWindow` | Show Pull Request in the Tool Window | `src/features/github/index.ts` | missing |
| `github.pullRequest.updateBranch` | Update to Enable Review Mode… | `src/features/github/index.ts` | missing |

### graphql

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `graphql.addConfiguration` | Add Schema Configuration | `src/features/graphql/index.ts` | missing |
| `graphql.editConfiguration` | Open Configuration File | `src/features/graphql/index.ts` | missing |
| `graphql.introspect` | Rerun Introspection Query | `src/features/graphql/index.ts` | missing |
| `graphql.restartDiscovery` | Restart Schema Discovery | `src/features/graphql/index.ts` | missing |
| `graphql.runQuery` | Execute GraphQL | `src/features/graphql/index.ts` | missing |
| `graphql.showToolWindow` | GraphQL | `src/features/graphql/index.ts` | missing |

### http-client

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `http.addPrivateEnvironment` | Add Environment to Private File | `src/features/http-client/index.ts` | missing |
| `http.addPublicEnvironment` | Add Environment to Public File | `src/features/http-client/index.ts` | missing |
| `http.copyAsCurl` | Convert to cURL and Copy | `src/features/http-client/index.ts` | missing |
| `http.generateSpringControllerRequests` | Generate Requests for Controller | `src/features/http-client/index.ts` | missing |
| `http.generateSpringRequest` | Generate Request in HTTP Client | `src/features/http-client/index.ts` | missing |
| `http.newRequest` | New HTTP Request | `src/features/http-client/index.ts` | missing |
| `http.openEndpoint` | Open in HTTP Client | `src/features/http-client/index.ts` | missing |
| `http.runAll` | Run All HTTP Requests in File | `src/features/http-client/index.ts` | missing |
| `http.runRequest` | Run HTTP Request at Caret | `src/features/http-client/index.ts` | missing |
| `http.showToolWindow` | HTTP Client | `src/features/http-client/index.ts` | missing |

### libraries

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `libraries.reload` | Reload External Libraries | `src/features/libraries/index.ts` | missing |

### macros

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `macro.edit` | Edit Macros… | `src/features/macros/index.ts` | missing |
| `macro.playBack` | Play Back Last Macro | `src/features/macros/index.ts` | missing |
| `macro.playSaved` | Play Saved Macros… | `src/features/macros/index.ts` | missing |
| `macro.startRecording` | Start Macro Recording | `src/features/macros/index.ts` | missing |
| `macro.stopRecording` | Stop Macro Recording | `src/features/macros/index.ts` | missing |

### perf

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `perf.clearCaches` | Clear Caches | `src/features/perf/actions.ts` | missing |
| `perf.showMemory` | Show Memory Indicator Details | `src/features/perf/actions.ts` | missing |
| `perf.toggleMemoryIndicator` | Memory Indicator | `src/features/perf/actions.ts` | missing |
| `perf.toggleOverlay` | Toggle Performance Overlay (Dev) | `src/features/perf/actions.ts` | missing |

### plugins

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `plugin.${pluginId}.${decl.id}` |  | `src/features/plugins/views/serviceProvider.tsx` | missing |
| `plugins.content.action` | Run Content Editor Action | `src/features/plugins/views/contentActions.ts` | missing |
| `plugins.content.find` | Find in Editor | `src/features/plugins/views/contentActions.ts` | missing |
| `plugins.importInstalledIntellijTheme` | Import Theme from JetBrains IDE… | `src/features/plugins/index.ts` | missing |
| `plugins.importIntellijTheme` | Import IntelliJ Theme from File… | `src/features/plugins/index.ts` | missing |
| `plugins.installFromDisk` | Install Plugin from Disk… | `src/features/plugins/index.ts` | missing |
| `plugins.log.${operation}` |  | `src/features/plugins/views/logActions.ts` | missing |
| `plugins.log.toggleArgument` | Toggle Log Option | `src/features/plugins/views/logActions.ts` | missing |
| `plugins.reloadAll` | Reload Plugins | `src/features/plugins/index.ts` | missing |
| `plugins.settings` | Plugins… | `src/features/plugins/index.ts` | partial - internal/app/commands.go plugins.settings -> internal/app/welcome.go plugins page over internal/app/plugins.go registry; internal/app/menu_test.go |
| `plugins.updateAll` | Update All Plugins | `src/features/plugins/index.ts` | missing |

### profiler

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `run.profile` | Profile | `src/features/profiler/index.ts` | missing |
| `run.profileAllocations` | Profile Allocations | `src/features/profiler/index.ts` | missing |
| `run.profileContext` | Profile Context Configuration | `src/features/profiler/index.ts` | missing |
| `run.profileLiveObjects` | Profile Live Objects | `src/features/profiler/index.ts` | missing |

### project-tree

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `file.new` | New… | `src/features/project-tree/actions.ts` | integrated - internal/app/commands.go file.new; internal/app/project_tree_actions.go NewFileAtProjectSelection opens the templates.newFile chooser in internal/app/project_new.go; TestProjectNewTemplateSearchAndOpenState and TestFileTemplateCreateUsesBytesRPC |
| `navigate.selectIn` | Select In… | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go and tree_ops.go implement Alt+F1 Select In; base.files.select and TestProjectTreeOpenIDEAActionsAreRegistered |
| `navigate.selectInProjectView` | Select in Project View | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go reveals and selects the active file; base.files.select and TestProjectTreeOpenIDEAActionsAreRegistered |
| `navigate.showFilePath` | File Path | `src/features/project-tree/actions.ts` | integrated - internal/app/project_tree_actions.go opens OpenIDEA's path notification; TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.collapseAll` | Collapse All | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go collapses every project root; TestProjectViewOpenIDEAActionsAndOptions and TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.compareWith` | Compare With… | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go and dircompare.go compare a selected tree entry with another path through the native compare view; base.files.compareDirs and TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.copy` | Copy | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go copies a file or directory through fs_copy with unsaved-buffer guards; TestProjectTreeOpenIDEAActionsAreRegistered and TestTreeMenuLabelsFlattenSubmenus |
| `project.copyAbsolutePath` |  | `src/features/project-tree/actions.ts` | integrated - internal/app/commands.go project.copyAbsolutePath; internal/app/project_tree_actions.go CopySelectedTreePath |
| `project.copyElement` | Copy… | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go provides the OpenIDEA element-copy command and target; TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.copyFileName` |  | `src/features/project-tree/actions.ts` | integrated - internal/app/commands.go project.copyFileName; internal/app/project_tree_actions.go CopySelectedTreePath |
| `project.copyPath` | Copy Path | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go copies absolute, relative and source-root paths; TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.copyRelativePath` |  | `src/features/project-tree/actions.ts` | integrated - internal/app/commands.go project.copyRelativePath; internal/app/project_tree_actions.go CopySelectedTreePath computes the content-root relative path |
| `project.copySourceRootPath` |  | `src/features/project-tree/actions.ts` | integrated - internal/app/commands.go CopySelectedTreePath("source") and tree_ops.go resolve the source root; TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.cut` | Cut | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go cuts a selected file or directory into the tree clipboard; TestTreeMenuFollowsTheSelection and TestTreeMenuLabelsFlattenSubmenus |
| `project.delete` | Delete… | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go confirms deletion, closes affected tabs and calls fs_delete; TestProjectTreeOpenIDEAActionsAreRegistered and tree_ops_test.go |
| `project.expandAll` | Expand All | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go expands loaded folders; base.files.expand and TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.findInFolder` | Find in Folder… | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go opens project search scoped to the selected directory; TestProjectTreeOpenIDEAActionsAreRegistered and search_actions_test.go |
| `project.findUsages` | Find Usages of File | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go findUsagesOfTreeFile opens the file and runs FindUsages over the language service |
| `project.move` | Move File… | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go moves files and directories and updates open references; TestReferenceAwareTreeMoveUsesOpenIDEARPC and TestProjectTreeNewAndClipboardUseTheSelection |
| `project.newFile` | New File | `src/features/project-tree/actions.ts` | integrated - internal/app/project_tree_actions.go NewFileAtProjectSelection opens internal/app/project_new.go's anchored project.newPopup; choosing File or a template opens templates.newFile; TestProjectNewTemplateSearchAndOpenState, TestProjectNewPopupRendersOpenIDEAFilterAndActions and TestFileTemplateCreateUsesBytesRPC |
| `project.newFolder` | New Directory | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go creates a directory through fs_create_dir; TestProjectTreeNewAndClipboardUseTheSelection |
| `project.newInThisDirectory` | New in This Directory… | `src/features/project-tree/actions.ts` | integrated - internal/app/project_tree_actions.go NewInThisDirectory; internal/app/commands.go project.newInThisDirectory; internal/app/project_tree_actions_test.go |
| `project.open` | Jump to Source | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go opens the selected file and honours the project view's single-click and preview-tab options; tree_test.go and project_view_test.go |
| `project.openInTerminal` | Open in Terminal | `src/features/project-tree/actions.ts` | integrated - internal/app/project_tree_actions.go and terminal_actions.go open a shell at the selected directory; TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.optimizeImports` | Optimize Imports in File | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go optimizeImportsTreeFile and OptimizeImportsOfSelectedTreeFile run the language service source.organizeImports action |
| `project.option.autoscrollFromSource` | Project View: Always Select Opened File | `src/features/project-tree/actions.ts` | integrated - internal/app/project_view.go and tree.go keep the active editor file selected; TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.option.autoscrollToSource` | Project View: Autoscroll to Source | `src/features/project-tree/actions.ts` | integrated - internal/app/project_view.go opens the selected tree file as the editor focus; TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.option.compact` | Project View: Compact Middle Packages | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go packageChain and internal/app/project_view_test.go TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.option.foldersFirst` | Project View: Sort Folders First | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go sorts folders first according to OpenIDEA's option; TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.option.previewTab` | Project View: Enable Preview Tab | `src/features/project-tree/actions.ts` | integrated - internal/app/project_view.go and tabs.go route single-click opens through the preview tab; TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.option.showExcluded` | Project View: Show Excluded Files | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go entryVisible; internal/app/project_view_test.go TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.option.showHidden` | Project View: Show Hidden Files | `src/features/project-tree/actions.ts` | integrated - internal/app/project_view.go SetProjectTreeOption; internal/workspace Tree.IncludeHidden and SortEntries refresh; internal/app/project_view_test.go TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.option.showMembers` | Project View: Show Members | `src/features/project-tree/actions.ts` | integrated - internal/app/project_view.go projectTree members loading and internal/app/tree.go appendMembers; internal/app/project_view_test.go TestProjectTreeOpenChangedScratchAndMembers |
| `project.option.showRootPaths` | Project View: Show Root Paths | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go renders root paths from the persisted project-view option; TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.option.singleClickExpand` | Project View: Expand Directories with Single Click | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go expands a directory on a single click when enabled; TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.option.singleClickOpen` | Project View: Open Files with Single Click | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go opens a file on a single click when enabled; TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.paste` | Paste | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go pastes copied or cut entries through fs_copy and refreshes affected tabs; TestProjectTreeNewAndClipboardUseTheSelection |
| `project.reformatCode` | Reformat File | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go formatTreeFile and FormatSelectedTreeFile open the file and run FormatActive |
| `project.refresh` | Reload Project Tree from Disk | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go ReloadProjectTree -> internal/app/tree.go Refresh -> workspace.Tree.Refresh rereads every folder read so far, from the palette's 从磁盘重新加载项目树; base.files.expand |
| `project.reloadFromDisk` | Reload from Disk | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go refreshes the selected subtree from disk; base.files.reload and TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.rename` | Rename File… | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go renames through fs_rename and updates open documents; TestProjectTreeOpenIDEAActionsAreRegistered and TestReferenceAwareTreeMoveUsesOpenIDEARPC |
| `project.renameInline` | Rename File Inline | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go opens the native inline rename path and shares the same guarded RPC; TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.reveal` | Reveal in Project View | `src/features/project-tree/actions.ts` | integrated - internal/app/project_tree_actions.go reveals the selected path in the system file manager; TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.revealInFinder` |  | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go RevealSelectedInFinder and revealPath; Project View Open In submenu |
| `project.showHistory` | Local History: Show History | `src/features/project-tree/actions.ts` | integrated - internal/app/history.go lists and restores folder history; base.files.historyRestore and TestProjectTreeOpenIDEAActionsAreRegistered |
| `project.sortBy.modified` | Project View: Sort by Modification Time | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go pathLess; internal/app/project_view_test.go TestProjectTreeSortsByModificationTime |
| `project.sortBy.name` | Project View: Sort by Name | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go default Name sorting and natural numeric comparison; internal/app/project_view_test.go |
| `project.sortBy.type` | Project View: Sort by Type | `src/features/project-tree/actions.ts` | integrated - internal/app/project_view.go and workspace tree sort by type; TestProjectTreeViewOptionsFilterSortAndCompact |
| `project.undoMove` | Undo Move / Rename | `src/features/project-tree/actions.ts` | integrated - internal/app/project_tree_actions.go UndoLastTreeMove over refactor_undo_file_move; internal/app/project_tree_actions_test.go |
| `project.viewMode.changedFiles` | Project View: Show All Changed Files | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go SetMode changedFiles and internal/app/vcs.go filtered-file sync; internal/app/project_view_test.go |
| `project.viewMode.openFiles` | Project View: Show Open Files | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go SetMode openFiles and SetFilteredFiles; internal/app/project_view_test.go TestProjectTreeOpenChangedScratchAndMembers |
| `project.viewMode.project` | Project View: Show Project | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go SetMode project; internal/app/project_view_test.go OpenIDEA action registry test |
| `project.viewMode.projectFiles` | Project View: Show Project Files | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go SetMode projectFiles; internal/app/project_view_test.go TestProjectTreeOpenChangedScratchAndMembers |
| `project.viewMode.scratches` | Project View: Show Scratches and Consoles | `src/features/project-tree/actions.ts` | integrated - internal/app/tree.go SetScratchRoot and Scratches mode; internal/app/project_view_test.go TestProjectTreeOpenChangedScratchAndMembers |
| `project.viewMode.unity` | Project View: Show Unity | `src/features/project-tree/actions.ts` | integrated - internal/app/project_view.go switches the tree to OpenIDEA's Unity view; TestProjectTreeOpenChangedScratchAndMembers |

### project-trust

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `project.dontOpen` | Don't Open | `src/features/project-trust/index.tsx` | integrated - internal/app/run.go closes project.trustDialog without changing the stored decision and leaves no project attached; TestProjectTrustDialogCancelKeepsTheProjectClosed |
| `project.previewRestricted` | Preview in Restricted Mode | `src/features/project-trust/index.tsx` | integrated - internal/app/run.go TrustProjectPreview records the restricted decision, opens the project without allowing build/run/debug RPCs, and is rendered as Preview in Restricted Mode in project.trustDialog; TestProjectTrustPreviewClearsFolderScope |
| `project.removeTrustedLocation` | Remove Trusted Location | `src/features/project-trust/index.tsx` | integrated - internal/app/project_trust_settings.go exposes Remove for every trusted location through project_trust_remove_location, then reloads project_trust_list; TestProjectTrustSettingsPageMatchesOpenIDEA |
| `project.revokeTrust` | Revoke Trust | `src/features/project-trust/index.tsx` | integrated - internal/app/project_trust_settings.go exposes Revoke Trust for every trusted project through project_trust_set(path,false,false), then reloads project_trust_list; TestProjectTrustSettingsPageMatchesOpenIDEA |
| `project.trust` | Trust Project | `src/features/project-trust/index.tsx` | integrated - internal/app/run.go and actions_builtin.go expose OpenIDEA's project.trust action; TrustProject persists the current-project decision or the selected parent-folder scope and refreshes Restricted Mode; TestRunningAnUntrustedProjectAsksToTrust and TestProjectTrustPromptMatchesOpenIDEA |

### remote-dev

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `remote.connect` | Remote Development… | `src/features/remote-dev/index.ts` | partial - internal/app/remote.go opens the native SSH, WSL and Dev Container chooser and states that this build's backend has no remote orchestration RPC; TestRemoteConnectAndReopenDialogsExplainBackendGap |
| `remote.disconnect` | Disconnect from Remote Host | `src/features/remote-dev/index.ts` | partial - internal/app/remote.go confirms the action and leaves the project open because the backend has no remote teardown RPC; TestRemoteDisconnectConfirmsWithoutClosingProject |
| `remote.reloadEnvironment` | Reload Environment | `src/features/remote-dev/index.ts` | partial - internal/app/remote.go calls env_reload and renders changed names, devcontainer.json and login-shell state; TestRemoteEnvironmentReloadSuccess and TestRemoteEnvironmentReloadErrorClearsBusyState |
| `remote.reopenInContainer` | Reopen in Container | `src/features/remote-dev/index.ts` | partial - internal/app/remote.go shows a native unavailable notice without closing or reopening the project; TestRemoteConnectAndReopenDialogsExplainBackendGap |

### run

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `analyze.stackTrace` | Analyze Stack Trace or Thread Dump… | `src/features/run/stackTrace.tsx` | partial - Code menu and action registry open internal/app/stack_trace.go; clipboard detection, Run trace tabs, clickable JVM/Python/path source frames and file index resolution; stack_trace_test.go; remaining console link patterns and upstream Run session integration are pending |
| `externalTools.${t.id}` |  | `src/features/run/externalTools.ts` | missing |
| `go.generate.${scope}` |  | `src/features/run/goActions.ts` | missing |
| `php.composerInstall` | Composer Install | `src/features/run/composerInstall.ts` | missing |
| `run.agentStart` | Run Agent Command | `src/features/run/index.ts` | missing |
| `run.chooseConfig` | Select Run/Debug Configuration… | `src/features/run/index.ts` | partial - internal/app/run_config_chooser.go runChooserDialog (Ctrl+Alt+R, run.chooseConfig in internal/app/commands.go): the ConfigChooser modes ported - run (Enter starts as saved now through askOrStart with the rerun confirm, Shift+Enter debugs), debug (run.chooseDebugConfig Ctrl+Alt+D), select (the widget's chooser); the current-file row debugs the context; limits: no temporary-configuration Save row, no mnemonics, no recent/pinned grouping |
| `run.chooseDebugConfig` | Debug… | `src/features/run/index.ts` | partial - internal/app/run_config_chooser.go chooser in debug mode (Ctrl+Alt+D): Enter debugs the chosen configuration through DebugRunConfig over the shared adapter.PrepareJavaLaunch |
| `run.createConfig` | Create Run Configuration | `src/features/run/index.ts` | partial - internal/app registers the native action `run.createConfig`; interaction-level acceptance is pending |
| `run.debug` | Debug | `src/features/run/index.ts` | partial - internal/app/debug.go Debug/DebugRunConfig: debug the SELECTED run configuration over the shared PrepareJavaLaunch + javaDebugLaunch (F9, run.debug); non-JVM kinds and unshipped adapters (kotlin-debug-adapter) are refused with the real reason; negatives.dbgcfg.session debugs the detected application configuration to its end on the real server; limits: gradle/maven debugging delegates are not ported |
| `run.debugConfig` | Debug Configuration | `src/features/run/index.ts` | partial - OpenIDEA id `run.debugConfig` aliases GoIDEA action `run.debug`; internal/app/debug.go Debug/DebugRunConfig: debug the SELECTED run configuration over the shared PrepareJavaLaunch + javaDebugLaunch (F9, run.debug); non-JVM kinds and unshipped adapters (kotlin-debug-adapter) are refused with the real reason; negatives.dbgcfg.session debugs the detected application configuration to its end on the real server; limits: gradle/maven debugging delegates are not ported |
| `run.debugContext` | Debug Context Configuration | `src/features/run/index.ts` | partial - internal/app/debug.go DebugContext (Ctrl+Shift+D): the active file's applicationConfig over the shared PrepareJavaLaunch; the debug e2e phase proves the flow (debug.paused, debug.vars, debug.watch) |
| `run.editConfigs` | Edit Configurations… | `src/features/run/index.ts` | partial - internal/app/run_config_dialog.go runConfigsDialog (Ctrl+Alt+Shift+R, run.editConfigs): the ConfigForm fields GoIDEA edits (name, command, program args via adapter.ParseArgs/FormatArgs, working directory, environment, module with save-time resolution, Build select with per-kind options), configs.ts validation and upsertConfig override semantics, saveUserConfigs through run_save_configs with removedDetectedIds; base.runcfg.* proves the round trip on the real server; limits: no per-template Add menu, no before-launch task editor, no compound editor |
| `run.importJetbrains` | Import Run Configurations from JetBrains (.run, .idea) | `src/features/run/index.ts` | partial - internal/app registers the native action `run.importJetbrains`; interaction-level acceptance is pending |
| `run.importVscodeLaunch` | Import Run Configurations from .vscode/launch.json | `src/features/run/index.ts` | partial - internal/app registers the native action `run.importVscodeLaunch`; interaction-level acceptance is pending |
| `run.modifyContext` | Modify Run Configuration… | `src/features/run/index.ts` | missing |
| `run.pluginStart` | Run Plugin Configuration | `src/features/run/index.ts` | missing |
| `run.reloadConfigs` | Reload Run Configurations | `src/features/run/index.ts` | partial - internal/app registers the native action `run.reloadConfigs`; interaction-level acceptance is pending |
| `run.rerun` | Rerun | `src/features/run/index.ts` | partial - internal/app/run_config_dialog.go Rerun over the run widget's rerun button (⌘R, run.rerun in internal/app/commands.go): rerunConfig semantics (the configuration as saved now), the Process 'x' Is Running confirm (dialogs.tsx confirmRerun) with 不再询问 persisted under run.confirmRerun in internal/app/settings.go RunPrefs, stop + wait for exit + restart; limits: allowParallel restarts instead of a second instance (single run session) |
| `run.rerunFailed` | Rerun Failed Tests | `src/features/run/index.ts` | partial - internal/adapter/testing_results.go, testing_jvm.go and testing_rerun.go port the testing pipeline's results.ts, jvm.ts (surefire 2/3, Gradle logging, TeamCity init-script messages) and rerun.ts's jvmArgs; internal/app/run.go folds the run output into the tree, and the Run pane renders the results TREE (internal/app/view.go: status marks, durations, per-node output); run.rerunFailed derives the -Dtest=/--tests filter; rerunfail.* checks prove the derivation and the Tests run: 1 rerun; limits: other frameworks' parsers and Gradle's init-script wiring are not ported; the go test pipeline (internal/adapter/testing_go.go ports go.ts: plain/-v/-json parsing, benchmarks, locations) folds go test -json records into the tree and gorerun.* main-gate checks prove the derivation anchors -run ^TestFailsOnPurpose$ and the rerun executes; limits: the backend's quoting of the $ anchor in the rerun's filter is a suspected upstream-backend issue, so the go rerun's final one-test assertion stays out until verified against an OpenIDEA change; the pytest pipeline (internal/adapter/testing_pytest.go ports pytest.ts: -v verbose lines, -rA summaries, progress dots, failure sections with buffered resolution) folds pytest output into the tree and pyrerun.* main-gate checks prove the node-id derivation reruns only the failed test; the fixture's venv interpreter is set through the configuration's command (Edit Configurations semantics); the jest pipeline (internal/adapter/testing_javascript.go ports js.ts: jest/vitest verbose lines, describe indentation, fail headers, vitest console blocks) and jsrerun.* main-gate checks prove the anchored -t derivation and the rerun of only the failed test with a real npm-installed jest; the parser's rune decoding and fail-header path are covered by internal/adapter/testing_javascript_test.go; the phpunit/ruby/dotnet parsers are not ported yet |
| `run.run` | Run | `src/features/run/index.ts` | integrated - base.run.exit, base.run.build |
| `run.runConfig` | Run Configuration | `src/features/run/index.ts` | partial - internal/app/run_anything.go runAnythingLine executes a run configuration by its exact name (upstream runConfigsRunAnything.execute) with the same multi-word filter and prefix-first ordering as upstream matchingConfigs (runAnythingConfigs); limits: the chooser's debug executor and RunWidget per-kind icons differ |
| `run.runContext` | Run Context Configuration | `src/features/run/index.ts` | partial - internal/app registers the native action `run.runContext`; interaction-level acceptance is pending |
| `run.sampleOutput` | Stream Sample Output | `src/features/run/index.ts` | missing |
| `run.services.addType` | Add Service | `src/features/run/services.tsx` | missing |
| `run.services.hideConfig` | Remove from Services | `src/features/run/services.tsx` | missing |
| `run.stop` | Stop | `src/features/run/index.ts` | partial - internal/app registers the native action `run.stop`; interaction-level acceptance is pending |
| `run.stopProcess` | Stop | `src/features/run/index.ts` | partial - internal/app registers the native action `run.stop`; interaction-level acceptance is pending |
| `run.updateApplication` | Update Application | `src/features/run/index.ts` | missing |
| `run.withExecutor` | Run with Executor | `src/features/run/index.ts` | missing |

### run-anything

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `runAnything.show` | Run Anything… | `src/features/run-anything/index.ts` | integrated - internal/app/commands.go runAnything.show opens the popup; gear menu first item in internal/app/menu.go; internal/app/run_anything_test.go TestRunAnythingDialogRenders runs the action through the registry |

### scratch

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `file.newScratchFile` | New Scratch File… | `src/features/scratch/index.ts` | partial - internal/app/commands.go file.newScratchFile, Cmd+Shift+N, -> internal/app/scratch.go OpenScratchChooser: the New Scratch File popup lists the languages this editor highlights (Chroma's lexers with their first plain extension), recent first and filtered by name/prefix/subsequence/extension; CreateScratchLanguage writes scratch[_n].<ext> into <data>/scratches, places the caret from the template's $END$ when one exists, opens it and remembers the language in Preferences.ScratchRecent; internal/app/scratch_test.go; base.scratch.languages, base.scratch.opened; the backend's desktop-only languages list (and with it the per-language scratch templates) is not reachable over RPC, so templates are empty, and the Scratches project view root and New popup menu group are not ported |

### search

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `bookmarks.newList` | New Bookmark List… | `src/features/search/index.ts` | integrated - internal/app/bookmarks.go addBookmarkList, renameBookmarkList, removeBookmarkList and newListDialog; tool-window chips 重命名/删除; the move chooser's 新建列表… creates a list and moves the bookmark into it; cmd/goidea/main.go 新建书签列表…; base.bookmark.list, base.bookmark.listOps, base.bookmark.move; internal/app/bookmarks_test.go |
| `bookmarks.toggleFileBookmark` | Toggle File Bookmark | `src/features/search/index.ts` | integrated - internal/app/bookmarks.go ToggleFileBookmark and ToggleFolderBookmark with PathBookmark.Dir, pathBookmarkLabel; project tree rows chip 书签 in internal/app/view.go; cmd/goidea/main.go 切换文件书签; base.bookmark.file, base.bookmark.folder; internal/app/bookmarks_test.go |
| `find.inPath` | Find in Files… | `src/features/search/index.ts` | partial - internal/app registers the native action `find.inPath`; interaction-level acceptance is pending |
| `find.replaceInPath` | Replace in Files… | `src/features/search/index.ts` | partial - internal/app registers the native action `find.replaceInPath`; interaction-level acceptance is pending |
| `find.rerun` | Rerun Search | `src/features/search/index.ts` | partial - internal/app registers the native action `find.rerun`; interaction-level acceptance is pending |
| `navigate.fileStructure` | File Structure | `src/features/search/index.ts` | partial - internal/app registers the native action `navigate.fileStructure`; interaction-level acceptance is pending |
| `navigate.findAction` | Find Action… | `src/features/search/index.ts` | integrated - lsp.se.navigation, lsp.se.actions |
| `navigate.gotoBookmark0` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark1` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark2` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark3` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark4` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark5` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark6` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark7` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark8` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark9` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoClass` | Go to Class… | `src/features/search/index.ts` | integrated - lsp.se.navigation; Classes uses the real workspace symbol query |
| `navigate.gotoFile` | Go to File… | `src/features/search/index.ts` | integrated - lsp.se.navigation, lsp.se.files |
| `navigate.gotoSymbol` | Go to Symbol… | `src/features/search/index.ts` | integrated - lsp.se.navigation, lsp.se.symbols |
| `navigate.nextBookmark` | Next Bookmark | `src/features/search/index.ts` | partial - internal/app/bookmarks.go NextBookmark and stepBookmark wrap; base.bookmark.next; internal/app/bookmarks_test.go |
| `navigate.prevBookmark` | Previous Bookmark | `src/features/search/index.ts` | partial - internal/app/bookmarks.go PrevBookmark and stepBookmark wrap; base.bookmark.prev; internal/app/bookmarks_test.go |
| `navigate.recentFiles` | Recent Files | `src/features/search/index.ts` | integrated - internal/app/recent_navigation.go and recent_navigation_view.go implement Navigate or Cmd+E, previous-file selection, speed search, tool windows, single-click activation, Show edited only, guarded Delete close plus remove_recent, Shift/Cmd multi-selection, Enter in list order, Shift+Enter opening to the side, and OpenIDEA's Ctrl+Tab Switcher. The Switcher has no speed-search field, lists open files plus Agent Session/Home editors, releases Ctrl to activate, accepts native Enter and navigation keys, and walks from files into tool windows across both columns; TestRecentFilesShowsPreviousAndEditedFiles, TestRecentFilesDeleteForgetsTheSelectedFile, TestRecentFilesKeyboardRangeAndModifierClicks, TestRecentFilesEnterOpensSelectionInListOrder, TestRecentFilesShiftEnterOpensInSideGroup, TestRecentFilesShiftEnterCopiesTheOnlyFocusedTab, TestSwitcherListsOnlyOpenFilesInActivationOrder, TestSwitcherListsAgentEditorsAndFiltersTheirTitles, TestSwitcherNavigationWrapsAcrossFilesAndTools, TestSwitcherHasNoSpeedSearchAndAcceptsNativeKeys and TestSwitcherOpensSelectionWhenControlIsReleased cover the native UI |
| `navigate.recentLocations` | Recent Locations | `src/features/search/index.ts` | partial - internal/app/recent_navigation.go and recent_navigation_view.go; Navigate or Cmd+Shift+E, bounded caret history, code snippets, changed filter, dismiss and jump; internal/app/recent_navigation_test.go; syntax-highlighted snippets, symbol breadcrumbs and opening to the side are pending |
| `navigate.searchEverywhere` | Search Everywhere | `src/features/search/index.ts` | integrated - internal/app/se.go OpenSearchEverywhere; internal/app/commands.go navigate.searchEverywhere; Navigate menu and the title bar search button; e2e searchEverywhere.files/symbols/actions/text/allSections; internal/app/se_test.go |
| `navigate.searchText` | Search Text Everywhere | `src/features/search/index.ts` | integrated - base.search.done, base.search.files, base.search.matches |
| `navigate.showBookmarks` | Show Bookmarks | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ShowBookmarks popup groups files and lines by list with jump and delete; base.bookmark.popup; Cmd+F3 in internal/app/app.go; internal/app/bookmarks_test.go |
| `navigate.switcher` | Switcher | `src/features/search/index.ts` | partial - internal/app registers the native action `navigate.switcher`; interaction-level acceptance is pending |
| `navigate.switcherBack` | Switcher (Backward) | `src/features/search/index.ts` | partial - internal/app registers the native action `navigate.switcherBack`; interaction-level acceptance is pending |
| `navigate.toggleBookmark` | Toggle Bookmark | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmark at the caret and gutter dots; F3 in internal/app/app.go; base.bookmark.added, base.bookmark.gutter, base.bookmark.remove; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark0` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark1` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark2` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark3` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark4` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark5` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark6` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark7` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark8` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark9` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmarkWithMnemonic` | Toggle Bookmark with Mnemonic | `src/features/search/index.ts` | partial - internal/app/bookmarks.go OpenBookmarkMnemonics Alt+F3 chooser over digits and A-Z; internal/app/app.go shortcut; internal/app/bookmarks_test.go |
| `search.editScopes` | Edit Scopes… | `src/features/search/index.ts` | partial - internal/app registers the native action `search.editScopes`; interaction-level acceptance is pending |

### services

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `docker.composeUp` | Compose Up | `src/features/services/index.ts` | missing |
| `docker.execShell` | Create Terminal in Container | `src/features/services/index.ts` | missing |
| `docker.inspect` | Inspect Docker Object | `src/features/services/index.ts` | missing |
| `docker.openLogInEditor` | Open Container Log in Editor | `src/features/services/index.ts` | missing |
| `docker.prune` | Prune Docker… | `src/features/services/index.ts` | missing |
| `docker.pullImage` | Pull Docker Image… | `src/features/services/index.ts` | missing |
| `docker.refresh` | Refresh Docker | `src/features/services/index.ts` | missing |
| `docker.remove` | Delete Docker Object… | `src/features/services/index.ts` | missing |
| `docker.restart` | Restart Container | `src/features/services/index.ts` | missing |
| `docker.setComposeFileOverride` | Set Docker Compose File Detection | `src/features/services/index.ts` | missing |
| `docker.showDashboard` | Show Docker Dashboard | `src/features/services/index.ts` | missing |
| `docker.showLayers` | Show Image Layers | `src/features/services/index.ts` | missing |
| `docker.showLog` | Show Container Log | `src/features/services/index.ts` | missing |
| `docker.showProcesses` | Show Container Processes | `src/features/services/index.ts` | missing |
| `docker.start` | Start Container | `src/features/services/index.ts` | missing |
| `docker.stop` | Stop Container | `src/features/services/index.ts` | missing |
| `docker.toggleStoppedContainers` | Toggle Stopped Containers | `src/features/services/index.ts` | missing |
| `docker.toggleUntaggedImages` | Toggle Untagged Images | `src/features/services/index.ts` | missing |
| `services.addService` | Add Service | `src/features/services/index.ts` | missing |
| `services.show` | Services | `src/features/services/index.ts` | partial - internal/app registers the native action `view.services`; interaction-level acceptance is pending |

### settings

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `editor.colorScheme.${name}` |  | `src/features/settings/pages/editor/colorScheme.tsx` | missing |
| `editor.encodings.toggleNativeToAscii` | Transparent native-to-ascii conversion | `src/features/settings/nativeToAscii.ts` | missing |
| `file.newProjectsSettings` | Settings for New Projects… | `src/features/settings/index.ts` | missing |
| `file.overrideFileType` | Override File Type | `src/features/settings/fileTypesRuntime.ts` | missing |
| `file.projectStructure` | Project Structure… | `src/features/settings/index.ts` | missing |
| `file.revertFileTypeOverride` | Revert File Type Override | `src/features/settings/fileTypesRuntime.ts` | missing |
| `file.settings` | Settings… | `src/features/settings/index.ts` | partial - internal/app/commands.go file.settings -> internal/app/settings_window.go + settings_pages.go, OpenIDEA's Settings dialog over Appearance, Keymap, Editor Appearance/Font and Java; OpenIDEA's remaining settings pages are not ported yet |
| `php.languageLevelPopup` | PHP Language Level | `src/features/settings/languages/PhpStatus.tsx` | missing |
| `php.selectLanguageLevel` | Select PHP Language Level | `src/features/settings/languages/PhpStatus.tsx` | missing |
| `python.addInterpreter` | Add New Interpreter… | `src/features/settings/languages/PythonStatus.tsx` | missing |
| `python.interpreterPopup` | Python Interpreter | `src/features/settings/languages/PythonStatus.tsx` | missing |
| `python.interpreterSettings` | Interpreter Settings… | `src/features/settings/languages/PythonStatus.tsx` | missing |
| `python.selectInterpreter` | Select Python Interpreter | `src/features/settings/languages/PythonStatus.tsx` | missing |
| `settings.appearance` | Appearance… | `src/features/settings/index.ts` | partial - internal/app registers the native action `settings.appearance`; interaction-level acceptance is pending |
| `settings.assignShortcut` | Assign Shortcut… | `src/features/settings/index.ts` | missing |
| `settings.keymap` | Keymap… | `src/features/settings/index.ts` | partial - internal/app registers the native action `settings.keymap`; interaction-level acceptance is pending |
| `view.navigationBarHidden` |  | `src/features/settings/appearanceRuntime.ts` | integrated - internal/app/navigation_bar.go persists Don't Show as hidden and removes the bar from both the top and status-bar placements; View › Appearance › Navigation Bar › Don't Show exposes the checked action; TestNavigationBarPlacementAndStatusWidgetPersistence |
| `view.navigationBarStatusBar` |  | `src/features/settings/appearanceRuntime.ts` | integrated - internal/app/navigation_bar.go defaults to OpenIDEA's statusBar placement, renders it through the Navigation Bar status item and restores it through View › Appearance › Navigation Bar › Status Bar; navigationBarPosition normalizes malformed settings to statusBar and TestNavigationBarPlacementAndStatusWidgetPersistence proves persistence |
| `view.navigationBarTop` |  | `src/features/settings/appearanceRuntime.ts` | integrated - internal/app/navigation_bar.go SetNavigationBarPosition("top") persists the global setting and renders the same project › directory › file › symbol bar above the editor while leaving the status widget independent; View › Appearance › Navigation Bar › Top and the action registry use the upstream id; TestNavigationBarPlacementAndStatusWidgetPersistence |
| `view.resetZoom` | Reset IDE Zoom | `src/features/settings/appearanceRuntime.ts` | partial - internal/app registers the native action `view.resetZoom`; interaction-level acceptance is pending |
| `view.toggleCompactMode` |  | `src/features/settings/appearanceRuntime.ts` | integrated - internal/app/appearance_runtime.go persists Compact Mode and internal/app/tokens.go applies OpenIDEA's :root[data-compact] chrome, header, tab, row, button and input sizes; View › Appearance exposes a checked toggle and keeping the toolbar off still renders the compact document header; TestCompactModeChangesChromeAndKeepsCompactHeaderWithToolbarOff |
| `view.toggleStatusBar` |  | `src/features/settings/appearanceRuntime.ts` | partial - internal/app registers the native action `view.toggleStatusBar`; interaction-level acceptance is pending |
| `view.toggleToolWindowBars` |  | `src/features/settings/appearanceRuntime.ts` | partial - internal/app registers the native action `view.toggleToolWindowBars`; interaction-level acceptance is pending |
| `view.toggleToolbar` |  | `src/features/settings/appearanceRuntime.ts` | partial - internal/app registers the native action `view.toggleToolbar`; interaction-level acceptance is pending |
| `view.zoomIn` | Zoom In IDE | `src/features/settings/appearanceRuntime.ts` | partial - internal/app registers the native action `view.zoomIn`; interaction-level acceptance is pending |
| `view.zoomOut` | Zoom Out IDE | `src/features/settings/appearanceRuntime.ts` | partial - internal/app registers the native action `view.zoomOut`; interaction-level acceptance is pending |

### ssh

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `ssh.configurations` | SSH Configurations… | `src/features/ssh/index.ts` | missing |
| `ssh.startSession` | Start SSH Session… | `src/features/ssh/index.ts` | missing |

### templates

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `code.insertLiveTemplate` | Insert Live Template… | `src/features/templates/index.ts` | missing |
| `code.surroundWith` | Surround With… | `src/features/templates/index.ts` | missing |
| `code.surroundWithLiveTemplate` | Surround with Live Template… | `src/features/templates/index.ts` | missing |
| `emmet.updateTag` | Update Tag with Emmet | `src/features/templates/index.ts` | missing |
| `templates.new.${t.id}` |  | `src/features/templates/index.ts` | missing |
| `templates.newFromTemplate` | New from Template… | `src/features/templates/index.ts` | missing |

### terminal

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `terminal.clear` | Clear Terminal Buffer | `src/features/terminal/index.ts` | partial - internal/app registers the native action `term.clear`; interaction-level acceptance is pending |
| `terminal.closeSession` | Close Terminal Session | `src/features/terminal/index.ts` | partial - internal/app registers the native action `term.close`; interaction-level acceptance is pending |
| `terminal.find` | Find in Terminal | `src/features/terminal/index.ts` | partial - internal/app registers the native action `term.find`; interaction-level acceptance is pending |
| `terminal.new` | New Terminal Tab | `src/features/terminal/index.ts` | partial - internal/app registers the native action `term.new`; interaction-level acceptance is pending |
| `terminal.newPredefinedSession` | New Predefined Session | `src/features/terminal/index.ts` | partial - internal/app registers the native action `terminal.newPredefinedSession`; interaction-level acceptance is pending |
| `terminal.openAt` | Open in Terminal | `src/features/terminal/index.ts` | partial - internal/app registers the native action `terminal.openAt`; interaction-level acceptance is pending |
| `terminal.pluginSession` | Open Plugin Terminal Session | `src/features/terminal/index.ts` | partial - internal/app registers the native action `terminal.pluginSession`; interaction-level acceptance is pending |
| `terminal.runCommand` | Run Command in Terminal | `src/features/terminal/index.ts` | partial - internal/app registers the native action `terminal.runCommand`; interaction-level acceptance is pending |
| `terminal.sampleTui` | Stream Sample TUI | `src/features/terminal/index.ts` | missing |
| `terminal.splitAndMoveDown` | Split and Move Down | `src/features/terminal/index.ts` | partial - internal/app registers the native action `terminal.splitAndMoveDown`; interaction-level acceptance is pending |
| `terminal.splitAndMoveRight` | Split and Move Right | `src/features/terminal/index.ts` | partial - internal/app registers the native action `terminal.splitAndMoveRight`; interaction-level acceptance is pending |
| `terminal.splitDown` | Split Down | `src/features/terminal/index.ts` | partial - internal/app registers the native action `terminal.splitDown`; interaction-level acceptance is pending |
| `terminal.splitRight` | Split Right | `src/features/terminal/index.ts` | partial - internal/app registers the native action `terminal.splitRight`; interaction-level acceptance is pending |
| `terminal.unsplit` | Unsplit | `src/features/terminal/index.ts` | partial - internal/app registers the native action `term.unsplit`; interaction-level acceptance is pending |

### todo

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `todo.refresh` | Refresh TODO Items | `src/features/todo/index.ts` | integrated - internal/app/todo.go RunTodo; internal/app/commands.go todo.refresh; Tools menu Refresh TODO Items |
| `todo.showCurrentFile` | Show TODO Items in Current File | `src/features/todo/index.ts` | integrated - internal/app/todo.go ShowTodoCurrentFile; internal/app/commands.go todo.showCurrentFile; Tools menu Show TODO Items in Current File |

### tools

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `java.downloadJdk` | Download JDK… | `src/features/tools/jdk.ts` | partial - internal/app registers the native action `java.downloadJdk`; interaction-level acceptance is pending |
| `tools.install` | Install Language Server / Debugger | `src/features/tools/index.ts` | partial - internal/app registers the native action `tools.install`; interaction-level acceptance is pending |
| `tools.languageServers` | Language Servers… | `src/features/tools/index.ts` | partial - internal/app registers the native action `tools.languageServers`; interaction-level acceptance is pending |
| `tools.promptInstall` | Install Missing Tool | `src/features/tools/index.ts` | partial - internal/app registers the native action `tools.promptInstall`; interaction-level acceptance is pending |

### vcs

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `diff.applyLeftSide` | Accept Left Side | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `diff.applyLeftSide`; interaction-level acceptance is pending |
| `diff.applyRightSide` | Accept Right Side | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `diff.applyRightSide`; interaction-level acceptance is pending |
| `diff.compareDirectories` | Compare Directories | `src/features/vcs/compare.tsx` | integrated - internal/app/dircompare.go OpenDirCompare and ConfirmDirCompare over fs_compare_directories with its onProgress channel, rows opening internal/app/diff.go ShowFileDiff; base.files.compareDirs |
| `diff.compareFiles` | Compare Files | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `diff.compareFiles`; interaction-level acceptance is pending |
| `diff.compareTexts` | Compare Texts | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `diff.compareTexts`; interaction-level acceptance is pending |
| `diff.compareWith` | Compare With… | `src/features/vcs/compare.tsx` | partial - internal/app/workbench.go tabMenu's Compare With… over internal/app/diff.go PickCompareFile (mygo.Dialog.Open from the tab's folder) then CompareTabWithFile, which compares the chosen file with the tab's buffer text so unsaved edits count, through diffRows and finishCompare (a guarded m.post, so a newer compare wins) into the read-only diffDialog titled by diffTitle; the tab menu's Git submenu is present alongside this entry, proven by diff_test.go TestDiffRowsCountsBothSides, TestDiffTitleNamesEachCompare and TestCompareWithoutATabOpensNothing; limits: a row list rather than OpenIDEA's editable two-pane diff editor and the project tree's Compare With… is not ported |
| `diff.compareWithClipboard` | Compare with Clipboard | `src/features/vcs/compare.tsx` | partial - internal/app/workbench.go tabMenu's Compare with Clipboard over internal/app/diff.go CompareTabWithClipboard (mygo.Clipboard.Read(transfer.Text), with a 剪贴板中没有文本 status when the clipboard holds no text) then ShowTextCompare, which diffs the tab's buffer against the clipboard text and titles the read-only diffDialog 剪贴板 by diffTitle; proven by diff_test.go TestDiffTitleNamesEachCompare and TestCompareWithoutATabOpensNothing; limits: the row list is not OpenIDEA's editable two-pane diff editor |
| `diff.jumpToSource` | Jump to Source | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `diff.jumpToSource`; interaction-level acceptance is pending |
| `diff.nextDifference` | Next Difference | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `diff.nextDifference`; interaction-level acceptance is pending |
| `diff.nextFile` | Compare Next File | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `diff.nextFile`; interaction-level acceptance is pending |
| `diff.prevDifference` | Previous Difference | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `diff.prevDifference`; interaction-level acceptance is pending |
| `diff.prevFile` | Compare Previous File | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `diff.prevFile`; interaction-level acceptance is pending |
| `diff.showSettings` | Diff Settings | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `diff.showSettings`; interaction-level acceptance is pending |
| `localHistory.putLabel` | Local History: Put Label… | `src/features/vcs/localHistoryActions.ts` | partial - internal/app/history.go ConfirmHistoryLabel over history_put_label; base.files.historyLabel |
| `localHistory.recentChanges` | Recent Changes | `src/features/vcs/localHistoryActions.ts` | partial - internal/app registers the native action `localHistory.recentChanges`; interaction-level acceptance is pending |
| `merge.accept` | Accept | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor_actions.go registers the full OpenIDEA merge command list; internal/app/merge_editor.go runMergeCommand and mergePaneMenu dispatch Accept through the selected side change; TestMergeCommandsAreAllRegistered and TestMergeEditorRendersOpenIDEAControls |
| `merge.acceptTheirs` | Accept Theirs | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go AcceptMergeTheirs and applyMergeTextPointer write and stage the complete Theirs version; the full command registration and footer are covered by TestMergeCommandsAreAllRegistered and TestMergeEditorRendersOpenIDEAControls |
| `merge.acceptYours` | Accept Yours | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go AcceptMergeYours and applyMergeTextPointer write and stage the complete Yours version, including deletion when the side is absent; TestMergeMissingSideDeletesAndStagesTheFile |
| `merge.annotate` | Annotate with Git Blame | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go AnnotateMergePane opens native blame for the focused side using HEAD or the merge commit; internal/app/merge_editor_actions.go registers the side-only action and mergePaneMenu exposes it |
| `merge.apply` | Apply Merge Result | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ApplyMerge confirms unresolved changes, writes the result, calls vcs_stage, refreshes VCS and closes the editor; TestMergeApplyWritesStagesAndClosesTheEditor and TestMergeUnresolvedApplyWaitsForConfirmation |
| `merge.applyAll` | Apply All Non-Conflicting Changes | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ApplyMergeNonConflicting handles OpenIDEA's apply-all operation and merge summary state; internal/app/merge_model_test.go TestMergeActionsAutoIgnoreAndRevert and the registered toolbar in TestMergeEditorRendersOpenIDEAControls |
| `merge.applyLeft` | Apply Non-Conflicting Changes from the Left Side | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ApplyMergeNonConflicting applies only left non-conflicting changes; internal/app/merge_model_test.go TestMergeTextChangesKeepsIndependentChangesApart and TestMergeActionsAutoIgnoreAndRevert |
| `merge.applyRight` | Apply Non-Conflicting Changes from the Right Side | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ApplyMergeNonConflicting applies only right non-conflicting changes; internal/app/merge_model_test.go TestMergeTextChangesKeepsIndependentChangesApart and TestMergeActionsAutoIgnoreAndRevert |
| `merge.autoApply` | Automatically Apply Non-Conflicting Changes | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go setMergeAutoApply persists the OpenIDEA setting and applies non-conflicting changes without recording the initial application as an edit; TestMergeAutoApplyPreferencePersists and TestMergeAutoApplyOnLoadDoesNotMakeTheEditorDirty |
| `merge.cancel` | Cancel Merge | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go RequestCloseMerge and drawMergeConfirmation discard only after confirmation when the result was touched; the footer and registered action are covered by TestMergeEditorRendersOpenIDEAControls |
| `merge.change` | Resolve Merge Change | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go RunMergeChange parses OpenIDEA's {indexes, action} payload and applies side accept, ignore, auto resolve and manual result edits; TestMergeEditorLoadsVersionsAndResolvesSelectedChange and internal/app/merge_model_test.go |
| `merge.compare` | Compare Contents | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ToggleMergeCompareMenu opens the six native version comparisons; TestMergeEditorRendersOpenIDEAControls and TestMergeCompareUsesAllSixOpenIDEAVersionPairs |
| `merge.compareBaseResult` | Base and Result | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ShowMergeCompare opens Base and Result through the shared native diff view; TestMergeCompareUsesAllSixOpenIDEAVersionPairs |
| `merge.compareBaseTheirs` | Base and Theirs | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ShowMergeCompare opens Base and Theirs through the shared native diff view; TestMergeCompareUsesAllSixOpenIDEAVersionPairs |
| `merge.compareBaseYours` | Base and Yours | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ShowMergeCompare opens Base and Yours through the shared native diff view; TestMergeCompareUsesAllSixOpenIDEAVersionPairs |
| `merge.compareResultTheirs` | Result and Theirs | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ShowMergeCompare opens Result and Theirs through the shared native diff view; TestMergeCompareUsesAllSixOpenIDEAVersionPairs |
| `merge.compareYoursResult` | Yours and Result | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ShowMergeCompare opens Yours and Result through the shared native diff view; TestMergeCompareUsesAllSixOpenIDEAVersionPairs |
| `merge.compareYoursTheirs` | Yours and Theirs | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ShowMergeCompare opens Yours and Theirs through the shared native diff view; TestMergeCompareUsesAllSixOpenIDEAVersionPairs |
| `merge.copy` | Copy | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go MergeCopy copies the focused pane selection; the result context menu and complete action registry are covered by TestMergeEditorRendersOpenIDEAControls and TestMergeCommandsAreAllRegistered |
| `merge.cut` | Cut | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go MergeCut copies the result selection and replaceResultRunes removes it with undo-safe change mapping; the result context menu and action registry are covered by TestMergeEditorRendersOpenIDEAControls |
| `merge.ignore` | Ignore | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go MergeIgnoreSelected handles whole-change and side-specific ignore through mergeEditorModel.ignoreChange and ignoreWholeChange; internal/app/merge_model_test.go TestMergeActionsAutoIgnoreAndRevert |
| `merge.paste` | Paste | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go MergePaste inserts text at the result selection with the same change remapping used by direct editing; the result context menu and action registry are covered by TestMergeEditorRendersOpenIDEAControls |
| `merge.reload` | Reload from Disk | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ReloadMerge reloads all three versions and asks before discarding touched decisions; the toolbar action is covered by TestMergeEditorRendersOpenIDEAControls |
| `merge.resolveAutomatically` | Resolve Automatically | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ResolveMergeSelectedAutomatically calls the word-level auto resolver for the selected changes; internal/app/merge_model_test.go TestMergeActionsAutoIgnoreAndRevert |
| `merge.resolveSimple` | Resolve Simple Conflicts | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ResolveSimpleMergeConflicts applies every simple automatic resolution and reveals what remains; internal/app/merge_model_test.go TestMergeActionsAutoIgnoreAndRevert |
| `merge.resolveUsingTheirs` | Resolve Using Theirs | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go MergeAcceptSelected resolves the selected conflict using Theirs; internal/app/merge_editor_actions.go gates it to the Theirs pane and mergePaneMenu exposes it |
| `merge.resolveUsingYours` | Resolve Using Yours | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go MergeAcceptSelected resolves the selected conflict using Yours; internal/app/merge_editor_actions.go gates it to the Yours pane and mergePaneMenu exposes it |
| `merge.revert` | Revert Conflict Resolution | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go RevertMergeResolution restores the base and clears every decision and manual marker; internal/app/merge_model_test.go TestMergeActionsAutoIgnoreAndRevert |
| `merge.selectAll` | Select All | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go MergeSelectAll selects the focused pane and updates the native text selection on the next frame; the result context menu and action registry are covered by TestMergeEditorRendersOpenIDEAControls |
| `merge.settings` | Merge Settings | `src/features/vcs/mergeActions.ts` | integrated - internal/app/merge_editor.go ToggleMergeSettingsMenu renders OpenIDEA's automatic non-conflicting-change setting; TestMergeEditorRendersOpenIDEAControls and TestMergeAutoApplyPreferencePersists |
| `vcs.acceptBoth` | Accept Both (Conflict at Caret) | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.acceptBoth`; interaction-level acceptance is pending |
| `vcs.acceptTheirs` | Accept Theirs (Conflict at Caret) | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.acceptTheirs`; interaction-level acceptance is pending |
| `vcs.acceptYours` | Accept Yours (Conflict at Caret) | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.acceptYours`; interaction-level acceptance is pending |
| `vcs.addToVcs` | Add to VCS | `src/features/vcs/actions.tsx` | partial - internal/app/workbench.go tabGitMenu's Add stages the tab's unversioned file through internal/app/vcs.go StageChange over vcs_stage, and the project view's Git › Add stages the selected unversioned file the same way via statusFor, proven by commit_test.go TestStatusForFindsAChangedFile and the base-phase stage checks; limits: the changes view offers Stage/Unstage rather than a separate Add action elsewhere |
| `vcs.annotate` | Annotate with Git Blame | `src/features/vcs/actions.tsx` | integrated - internal/app/annotate.go ShowAnnotate toggles a per-editor vcs_blame gutter with commit tints, scroll/fold/soft-wrap geometry, same-commit hover highlighting and an 800 ms edit refresh; clicking opens the commit and the context menu offers Show Commit in Git Log, Copy Revision Number and Close Annotations. Hovering lazily loads vcs_log and shows full author, email, formatted date and complete commit message with Issue Navigation links. annotate_test.go covers grouping, toggling, rich commit loading and the native author/date gutter; offscreen WORKBENCH_SHOT verifies the complete dynamic-width columns |
| `vcs.applyPatch` | Apply Patch… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.applyPatch`; interaction-level acceptance is pending |
| `vcs.applyPatchFromClipboard` | Apply Patch from Clipboard… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.applyPatchFromClipboard`; interaction-level acceptance is pending |
| `vcs.branches` | Branches… | `src/features/vcs/actions.tsx` | integrated - internal/app/branch.go ToggleBranches and ShowBranches render a native branch list from vcs_branch_info/vcs_branches with checkout, create, update, push, merge, rebase, compare, working-tree diff, history, scoped favorite, rename, local delete + Restore/View Commits/Delete Tracked Branch notification and confirmed remote delete; rename uses vcs_rename_branch, local delete vcs_delete_branch and restore vcs_restore_branch; branch_test.go covers validation, prompts, restore actions, favorites and the native rename dialog; SmokeBranch exercises the real backend |
| `vcs.changelist.commit` | Commit Changelist… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.changelist.commit`; interaction-level acceptance is pending |
| `vcs.changelist.delete` | Delete Changelist | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.changelist.delete`; interaction-level acceptance is pending |
| `vcs.changelist.edit` | Edit Changelist… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.changelist.edit`; interaction-level acceptance is pending |
| `vcs.changelist.move` | Move to Another Changelist… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.changelist.move`; interaction-level acceptance is pending |
| `vcs.changelist.new` | New Changelist… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.changelist.new`; interaction-level acceptance is pending |
| `vcs.changelist.setActive` | Set Active Changelist | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.changelist.setActive`; interaction-level acceptance is pending |
| `vcs.checkoutRevision` | Checkout Tag or Revision… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.checkoutRevision`; interaction-level acceptance is pending |
| `vcs.cherryPick.abort` | Abort Cherry-Pick | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.cherryPick.abort`; interaction-level acceptance is pending |
| `vcs.cherryPick.continue` | Continue Cherry-Pick | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.cherryPick.continue`; interaction-level acceptance is pending |
| `vcs.clone` | Get from Version Control… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.clone`; interaction-level acceptance is pending |
| `vcs.commit` | Commit… | `src/features/vcs/actions.tsx` | integrated - base.commit.id, base.commit.staged, base.commit.clean; the tab menu's Commit File… opens the same dialog with commitState.only, which keeps every other change out and includes a new file, proven by commit_test.go TestCommitFileIncludesOnlyItsFile; the project view's Git › Commit… opens the dialog with commitState.scope so only changes under the directory are listed and committed, proven by commit_test.go TestCommitDirKeepsChangesUnderTheDirectory |
| `vcs.commitAndPush` | Commit and Push… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.commitAndPush`; interaction-level acceptance is pending |
| `vcs.compareWithBranch` | Compare with Branch or Revision… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.compareWithBranch`; interaction-level acceptance is pending |
| `vcs.compareWithRevision` | Compare with Revision… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.compareWithRevision`; interaction-level acceptance is pending |
| `vcs.createPatch` | Create Patch… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.createPatch`; interaction-level acceptance is pending |
| `vcs.fetch` | Fetch | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.fetch`; interaction-level acceptance is pending |
| `vcs.localHistory` | Show Local History | `src/features/vcs/actions.tsx` | integrated - internal/app/history.go ShowHistory loads file snapshots through history_list and labels/deleted paths through history_list_dir, previews one revision against the worktree or two snapshots through history_content, and restores through history_restore; historyDialog renders the native timeline, deleted-path styling, multi-revision comparison and label-time group restore; branch tests cover scoped favorites and deleted-branch restore, history_test.go covers revisionRange, folder labels, content loading, merged file/list_dir data and the native dialog; base.files.historyRestore and base.files.historyLabel run on the real server |
| `vcs.log.branches.delete` | Delete Branch | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.branches.delete`; interaction-level acceptance is pending |
| `vcs.log.branches.navigate` | Navigate Log to Selected Branch | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.branches.navigate`; interaction-level acceptance is pending |
| `vcs.log.branches.new` | New Branch… | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.branches.new`; interaction-level acceptance is pending |
| `vcs.log.branches.toggleFavorite` | Mark/Unmark As Favorite | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.branches.toggleFavorite`; interaction-level acceptance is pending |
| `vcs.log.branches.update` | Update Selected | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.branches.update`; interaction-level acceptance is pending |
| `vcs.log.changes.compareBeforeWithLocal` | Compare Before with Local | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.changes.compareBeforeWithLocal`; interaction-level acceptance is pending |
| `vcs.log.changes.compareWithLocal` | Compare with Local | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.changes.compareWithLocal`; interaction-level acceptance is pending |
| `vcs.log.changes.diffPreview` | Show Diff Preview | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.changes.diffPreview`; interaction-level acceptance is pending |
| `vcs.log.changes.getFromRevision` | Get from Revision | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.changes.getFromRevision`; interaction-level acceptance is pending |
| `vcs.log.changes.groupByDirectory` | Group by Directory | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.changes.groupByDirectory`; interaction-level acceptance is pending |
| `vcs.log.changes.jumpToSource` | Jump to Source | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.changes.jumpToSource`; interaction-level acceptance is pending |
| `vcs.log.changes.revert` | Revert Selected Changes | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.changes.revert`; interaction-level acceptance is pending |
| `vcs.log.changes.showDiff` | Show Diff | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.changes.showDiff`; interaction-level acceptance is pending |
| `vcs.log.checkoutRevision` | Checkout Revision | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.checkoutRevision`; interaction-level acceptance is pending |
| `vcs.log.cherryPick` | Cherry-Pick | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.cherryPick`; interaction-level acceptance is pending |
| `vcs.log.collapseLinear` | Collapse Linear Branches | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.collapseLinear`; interaction-level acceptance is pending |
| `vcs.log.compareVersions` | Compare Versions | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.compareVersions`; interaction-level acceptance is pending |
| `vcs.log.compareWithLocal` | Compare with Local | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.compareWithLocal`; interaction-level acceptance is pending |
| `vcs.log.copyRevision` | Copy Revision Number | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.copyRevision`; interaction-level acceptance is pending |
| `vcs.log.createPatch` | Create Patch… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.createPatch`; interaction-level acceptance is pending |
| `vcs.log.dropCommit` | Drop Commit | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.dropCommit`; interaction-level acceptance is pending |
| `vcs.log.editMessage` | Edit Commit Message… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.editMessage`; interaction-level acceptance is pending |
| `vcs.log.expandLinear` | Expand Linear Branches | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.expandLinear`; interaction-level acceptance is pending |
| `vcs.log.fixup` | Fixup… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.fixup`; interaction-level acceptance is pending |
| `vcs.log.goToChild` | Go to Child Commit | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.goToChild`; interaction-level acceptance is pending |
| `vcs.log.goToParent` | Go to Parent Commit | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.goToParent`; interaction-level acceptance is pending |
| `vcs.log.goToRef` | Go to Hash/Branch/Tag | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.goToRef`; interaction-level acceptance is pending |
| `vcs.log.interactiveRebase` | Interactively Rebase from Here… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.interactiveRebase`; interaction-level acceptance is pending |
| `vcs.log.newBranch` | New Branch… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.newBranch`; interaction-level acceptance is pending |
| `vcs.log.newTag` | New Tag… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.newTag`; interaction-level acceptance is pending |
| `vcs.log.openExternal` | Open External Git Log… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.openExternal`; interaction-level acceptance is pending |
| `vcs.log.openInBrowser` | View in Browser | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.openInBrowser`; interaction-level acceptance is pending |
| `vcs.log.pushUpToHere` | Push All up to Here… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.pushUpToHere`; interaction-level acceptance is pending |
| `vcs.log.rebaseOntoCommit` | Rebase Current Branch onto Selected Commit | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.rebaseOntoCommit`; interaction-level acceptance is pending |
| `vcs.log.resetToHere` | Reset Current Branch to Here… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.resetToHere`; interaction-level acceptance is pending |
| `vcs.log.revealBookmark` | Show Commit Bookmark | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.revealBookmark`; interaction-level acceptance is pending |
| `vcs.log.revert` | Revert Commit | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.revert`; interaction-level acceptance is pending |
| `vcs.log.showBranches` | Show Branches | `src/features/vcs/logPaneActions.ts` | partial - internal/app registers the native action `vcs.log.showBranches`; interaction-level acceptance is pending |
| `vcs.log.squashCommits` | Squash Commits… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.squashCommits`; interaction-level acceptance is pending |
| `vcs.log.squashInto` | Squash Into… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.squashInto`; interaction-level acceptance is pending |
| `vcs.log.undoCommit` | Undo Commit… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.log.undoCommit`; interaction-level acceptance is pending |
| `vcs.manageRemotes` | Manage Remotes… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.manageRemotes`; interaction-level acceptance is pending |
| `vcs.merge` | Merge… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.merge`; interaction-level acceptance is pending |
| `vcs.merge.abort` | Abort Merge | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.merge.abort`; interaction-level acceptance is pending |
| `vcs.newBranch` | New Branch… | `src/features/vcs/actions.tsx` | integrated - internal/app/vcs_operations.go OpenNewGitBranch and native revision/name sheet; Git menu, project tree, tab menu, palette and VCS Operations quick list; vcsoperations.branch proves a real branch create and checkout |
| `vcs.newTag` | New Tag… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.newTag`; interaction-level acceptance is pending |
| `vcs.newWorktree` | New Worktree… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.newWorktree`; interaction-level acceptance is pending |
| `vcs.nextChange` | Next Change | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.nextChange`; interaction-level acceptance is pending |
| `vcs.nextConflict` | Next Conflict Marker | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.nextConflict`; interaction-level acceptance is pending |
| `vcs.openDiff` | Open Diff | `src/features/vcs/compare.tsx` | partial - internal/app registers the native action `vcs.openDiff`; interaction-level acceptance is pending |
| `vcs.openOnRemote` | Open on Git Remote | `src/features/vcs/openOnRemote.ts` | partial - internal/app registers the native action `vcs.openOnRemote`; interaction-level acceptance is pending |
| `vcs.operations` | VCS Operations… | `src/features/vcs/actions.tsx` | integrated - internal/app/vcs_operations.go OpenVCSOperations Ctrl+V quick list with OpenIDEA action ids; Git menu and palette entry; TestVCSOperationsQuickListFiltersAndDispatches and vcsoperations.* |
| `vcs.prevChange` | Previous Change | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.prevChange`; interaction-level acceptance is pending |
| `vcs.prevConflict` | Previous Conflict Marker | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.prevConflict`; interaction-level acceptance is pending |
| `vcs.pull` | Pull… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.pull`; interaction-level acceptance is pending |
| `vcs.push` | Push… | `src/features/vcs/actions.tsx` | integrated - internal/app/vcs_operations.go OpenGitPush and native remote, target, force-with-lease, set-upstream and no-verify choices; vcsoperations.push publishes to the real local remote |
| `vcs.rebase` | Rebase… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.rebase`; interaction-level acceptance is pending |
| `vcs.rebase.abort` | Abort Rebase | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.rebase.abort`; interaction-level acceptance is pending |
| `vcs.rebase.continue` | Continue Rebase | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.rebase.continue`; interaction-level acceptance is pending |
| `vcs.rebase.skip` | Skip Commit | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.rebase.skip`; interaction-level acceptance is pending |
| `vcs.refresh` | Refresh VCS Status | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.refresh`; interaction-level acceptance is pending |
| `vcs.resetHead` | Reset HEAD… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.resetHead`; interaction-level acceptance is pending |
| `vcs.resolveConflicts` | Resolve Conflicts… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.resolveConflicts`; interaction-level acceptance is pending |
| `vcs.revert.abort` | Abort Revert | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.revert.abort`; interaction-level acceptance is pending |
| `vcs.revert.continue` | Continue Revert | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.revert.continue`; interaction-level acceptance is pending |
| `vcs.rollback` | Rollback… | `src/features/vcs/actions.tsx` | integrated - internal/app/vcs.go AskRollback and ConfirmRollback over vcs_discard with a confirmation, from the changes view, the tab menu's Git › Rollback… and the project view's Git › Rollback…; SmokeStage |
| `vcs.shelf.compareLocal` | Compare Shelved Changes with Local | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.shelf.compareLocal`; interaction-level acceptance is pending |
| `vcs.shelf.copyPatch` | Copy as Patch to Clipboard | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.shelf.copyPatch`; interaction-level acceptance is pending |
| `vcs.shelf.createPatch` | Create Patch from Shelved Changes… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.shelf.createPatch`; interaction-level acceptance is pending |
| `vcs.shelf.delete` | Delete Shelved Changes | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.shelf.delete`; interaction-level acceptance is pending |
| `vcs.shelf.importPatches` | Import Patches… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.shelf.importPatches`; interaction-level acceptance is pending |
| `vcs.shelf.refresh` | Refresh Shelf | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.shelf.refresh`; interaction-level acceptance is pending |
| `vcs.shelf.rename` | Rename Shelved Changes… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.shelf.rename`; interaction-level acceptance is pending |
| `vcs.shelf.showDiff` | Show Shelved Diff | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.shelf.showDiff`; interaction-level acceptance is pending |
| `vcs.shelf.showDiffNewTab` | Show Diff in a New Tab | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.shelf.showDiffNewTab`; interaction-level acceptance is pending |
| `vcs.shelve` | Shelve Changes… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.shelve`; interaction-level acceptance is pending |
| `vcs.showCommit` | Show Commit in Git Log | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.showCommit`; interaction-level acceptance is pending |
| `vcs.showCurrentRevision` | Show Current Revision | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.showCurrentRevision`; interaction-level acceptance is pending |
| `vcs.showDiff` | Show Diff | `src/features/vcs/actions.tsx` | integrated - internal/app/diff.go ShowDiff over vcs_file_at with the buffer text, opened from the changes view, the tab menu's Git › Show Diff and the project view's Git › Show Diff; SmokeDiff |
| `vcs.showDiffInNewWindow` | Show Diff in New Window | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.showDiffInNewWindow`; interaction-level acceptance is pending |
| `vcs.showHistory` | Show History for File | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.showHistory`; interaction-level acceptance is pending |
| `vcs.showHistoryForSelection` | Show History for Selection | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.showHistoryForSelection`; interaction-level acceptance is pending |
| `vcs.showLog` | Show Git Log | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.showLog`; interaction-level acceptance is pending |
| `vcs.showMessageHistory` | Show Commit Message History | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.showMessageHistory`; interaction-level acceptance is pending |
| `vcs.showShelf` | Show Shelf | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.showShelf`; interaction-level acceptance is pending |
| `vcs.stash` | Stash Changes… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.stash`; interaction-level acceptance is pending |
| `vcs.toggleAmend` | Amend Commit | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.toggleAmend`; interaction-level acceptance is pending |
| `vcs.unshelve` | Unshelve… | `src/features/vcs/historyActions.tsx` | partial - internal/app registers the native action `vcs.unshelve`; interaction-level acceptance is pending |
| `vcs.unstash` | Unstash Changes… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.unstash`; interaction-level acceptance is pending |
| `vcs.update` | Update Project… | `src/features/vcs/actions.tsx` | integrated - internal/app/vcs_operations.go OpenGitPull and OpenGitFetch; native remote and strategy choices stream vcs_remote output; vcsoperations.fetch and vcsoperations.pull prove fetch and pull against a real local remote |
| `vcs.worktrees` | Worktrees… | `src/features/vcs/actions.tsx` | partial - internal/app registers the native action `vcs.worktrees`; interaction-level acceptance is pending |

### viewers

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `markdown.editorAndPreview` |  | `src/features/viewers/index.ts` | missing |
| `markdown.editorOnly` |  | `src/features/viewers/index.ts` | missing |
| `markdown.previewOnly` |  | `src/features/viewers/index.ts` | missing |

### vim

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `vim.reloadRc` | Reload .ideavimrc | `src/features/vim/index.ts` | missing |
| `vim.settings` | Vim Settings… | `src/features/vim/index.ts` | missing |
| `vim.toggle` | Vim | `src/features/vim/index.ts` | missing |

### web

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `web.openInBrowser` | Open in Browser | `src/features/web/index.ts` | missing |
| `web.togglePreview` | Built-in Preview | `src/features/web/index.ts` | missing |

### welcome

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `file.openRecent` | Open Recent… | `src/features/welcome/index.ts` | integrated - internal/app/menu.go exposes File › Open Recent › Recent Projects…, commands.go registers file.openRecent, and internal/app/recent_projects.go opens OpenIDEA's compact welcome.recentProjects chooser; internal/app/recent_projects_test.go and the recent-projects screenshot prove it |
| `welcome.recentProjects` | Recent Projects… | `src/features/welcome/index.ts` | integrated - internal/app/recent_projects.go filters the current project, searches, moves with arrows and PageUp/PageDown, opens with Enter or a single click, removes with Delete or Cmd+Backspace, and exposes row menus; internal/app/recent_projects_test.go and the recent-projects screenshot |

### whats-new

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `app.whatsNew` | What's New | `src/features/whats-new/index.ts` | integrated - internal/app/commands.go app.whatsNew; Help menu; internal/app/whatsnew.go and TestWhatsNewIsAStandalonePage |

### infrastructure

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `code.applyPastedImports` | Add Imports | `src/editor/lsp/importsOnPaste.ts` | missing |
| `code.cleanupFile` | Code Cleanup in File | `src/editor/lsp/actions.ts` | missing |
| `code.optimizeImportsFile` | Optimize Imports in File | `src/editor/lsp/actions.ts` | partial - internal/app registers the native action `code.optimizeImportsFile`; interaction-level acceptance is pending |
| `code.reformatFile` | Reformat File | `src/editor/lsp/actions.ts` | partial - internal/app registers the native action `code.reformatFile`; interaction-level acceptance is pending |
| `editor.addOrRemoveCaret` | Add or Remove Caret | `src/editor/core/actions.ts` | partial - internal/app registers the native action `editor.addOrRemoveCaret`; interaction-level acceptance is pending |
| `editor.cancelSourceDownload` | Cancel | `src/editor/core/librarySources.ts` | missing |
| `editor.changeSplitOrientation` | Change Splitter Orientation | `src/editor/core/actions.ts` | partial - internal/app/editor_split.go ChangeSplitterOrientation flips Model.splitDown between side by side and stacked; Window › Editor Tabs and the tab context menu show it only while the editor has two groups; internal/app/commands.go editor.changeSplitOrientation |
| `editor.chooseSources` | Choose Sources… | `src/editor/core/librarySources.ts` | missing |
| `editor.closeAllTabs` | Close All Tabs | `src/editor/core/actions.ts` | partial - internal/app/tabs.go CloseAllTabs with Close All Tabs and Close All but Pinned in the tab context menu; internal/app/tabs_test.go |
| `editor.closeOtherTabs` | Close Other Tabs | `src/editor/core/actions.ts` | partial - internal/app/tabs.go CloseOtherTabs, CloseTabsToLeft/Right and CloseUnmodifiedTabs; internal/app/workbench.go tabMenu; internal/app/tabs_test.go |
| `editor.closeTab` | Close Tab | `src/editor/core/actions.ts` | partial - internal/app registers the native action `file.close`; interaction-level acceptance is pending |
| `editor.copyPath` | Absolute Path | `src/editor/core/actions.ts` | partial - internal/app/editor_menu.go editorMenu: Copy / Paste Special › Copy Path/Reference › Absolute Path copies the active editor's path through copyEditorText and reports it in the status bar, in OpenIDEA's context-menu order after MyGo's editing commands. internal/app/editor_menu_test.go. OpenIDEA also binds the action to a shortcut, while GoIDEA reaches it from the context menu only |
| `editor.copyPathReference` | Copy Path/Reference… | `src/editor/core/actions.ts` | partial - internal/app/editor_menu.go editorMenu Copy / Paste Special › Copy Path/Reference submenu with Absolute Path, File Name, Path From Content Root and Path with line number over editorPathFlavours/projectRelative; internal/app/editor_menu_test.go. OpenIDEA opens a caret popup that also lists Reference, resolved through the language server; GoIDEA's submenu neither pops up at the caret nor offers the LSP reference row |
| `editor.copyPathWithLineNumber` | Copy Path with Line Number | `src/editor/core/actions.ts` | partial - internal/app/editor_menu.go editorMenu Copy Path/Reference › Path with line number builds the project-relative path and the caret's line from editor.State.CursorPos; internal/app/editor_menu_test.go. There is no command-palette entry or shortcut for it |
| `editor.copyReference` | Copy Reference | `src/editor/core/actions.ts` | partial - internal/app registers the native action `editor.copyReference`; interaction-level acceptance is pending |
| `editor.decreaseFontSize` | Decrease Font Size | `src/editor/core/actions.ts` | integrated - internal/app/settings_ui.go stepEditorFontZoom applies Alt+Shift+, across every open editor within OpenIDEA's shared -6..36 zoom; TestEditorFontActionsUseSharedRuntimeZoom |
| `editor.downloadSources` | Download Sources | `src/editor/core/librarySources.ts` | missing |
| `editor.increaseFontSize` | Increase Font Size | `src/editor/core/actions.ts` | integrated - internal/app/settings_ui.go stepEditorFontZoom applies Alt+Shift+. across every open editor within OpenIDEA's shared -6..36 zoom; TestEditorFontActionsUseSharedRuntimeZoom |
| `editor.inlineCompletion.trigger` | Call Inline Completion | `src/editor/lsp/inlineCompletion.ts` | missing |
| `editor.moveToOppositeGroup` | Move to Opposite Group | `src/editor/core/actions.ts` | partial - internal/app/editor_split.go MoveToOppositeGroup/moveToGroup takes the active tab to the other window group and focuses it there; Window › Editor Tabs and the tab context menu; internal/app/commands.go editor.moveToOppositeGroup; internal/app/editor_groups_test.go TestRemoveFromGroupKeepsItsActiveTabValid |
| `editor.nextSplitter` | Go to Next Splitter | `src/editor/core/actions.ts` | partial - internal/app/editor_split.go GoToNextSplitter focuses the other window group; Alt+Tab in internal/app/app.go and the Window › Editor Tabs entry report it; internal/app/commands.go editor.nextSplitter; internal/app/editor_groups_test.go TestFocusGroupSwapsThePanesAndTheirPositions. Two groups make next and previous the same step |
| `editor.nextTab` | Select Next Tab | `src/editor/core/actions.ts` | integrated - internal/app/commands.go editor.nextTab; internal/app/tabs.go cycleEditorTab wraps within the focused group, and the IntelliJ bindings are Ctrl+Right and Cmd+Shift+]; TestCycleTabIndexWrapsInBothDirections and TestEditorTabActionsHaveOpenIDEAIdentityAndBindings |
| `editor.openInOppositeGroup` | Open in Opposite Group | `src/editor/core/actions.ts` | partial - internal/app/editor_split.go OpenInOppositeGroup opens a copy of the active tab in the other group over the shared document, splitting first when the editor has one group; Window › Editor Tabs and the tab context menu; internal/app/commands.go editor.openInOppositeGroup |
| `editor.pasteHistory.paste` | Paste | `src/editor/core/view/PasteHistoryDialog.tsx` | missing |
| `editor.pasteHistory.remove` | Delete | `src/editor/core/view/PasteHistoryDialog.tsx` | missing |
| `editor.pinTab` | Pin / Unpin Tab | `src/editor/core/actions.ts` | partial - internal/app/tabs.go PinActiveTab/placeTab and the tab context menu in internal/app/workbench.go; pinned tabs in Session.Pinned; internal/app/tabs_test.go |
| `editor.prevSplitter` | Go to Previous Splitter | `src/editor/core/actions.ts` | partial - internal/app/editor_split.go GoToPreviousSplitter; Alt+Shift+Tab in internal/app/app.go and the Window › Editor Tabs entry report it; internal/app/commands.go editor.previousSplitter. Two groups make previous and next the same step |
| `editor.prevTab` | Select Previous Tab | `src/editor/core/actions.ts` | integrated - internal/app/commands.go editor.prevTab; internal/app/tabs.go cycleEditorTab wraps within the focused group, and the IntelliJ bindings are Ctrl+Left and Cmd+Shift+[; TestCycleTabIndexWrapsInBothDirections and TestEditorTabActionsHaveOpenIDEAIdentityAndBindings |
| `editor.reopenClosedTab` | Reopen Closed Tab | `src/editor/core/actions.ts` | integrated - internal/app/tabs.go closedTab/rememberClosed/nextClosed/insertTabAt (maxClosedTabs 50, newest poppable last, open files skipped) and ReopenClosedTab restore the original position, pin state, selections, scroll offset and shared undo history while retaining the document; internal/app/app.go removeTab records and removeTabNow skips the trimmed preview; internal/app/commands.go Reopen Closed Tab has no default key, as OpenIDEA's IntelliJ macOS keymap leaves Shift+Cmd+T to Go to Test; internal/app/tabs_test.go TestClosedTabsKeepTheNewestAndReopenNewestFirst, TestReopenClosedTabRestoresSelectionsAndUndoHistory, TestInsertTabAtFollowsThePinnedGroup and TestTabForPathIgnoresTabsWithoutADocument cover the native behavior |
| `editor.resetFontSize` | Reset Font Size in All Editors | `src/editor/core/actions.ts` | integrated - internal/app/settings_ui.go resetEditorFontZoom resets shared runtime zoom without changing Settings; TestEditorFontActionsUseSharedRuntimeZoom |
| `editor.showHint` | Show Editor Hint | `src/editor/lsp/actions.ts` | missing |
| `editor.splitDown` | Split Down | `src/editor/core/actions.ts` | partial - internal/app/editor_split.go SplitEditorDown/splitEditor with Model.splitDown drawing the stacked split; Window › Editor Tabs and the tab context menu in internal/app/menu.go and internal/app/workbench.go; internal/app/commands.go editor.splitDown; internal/app/editor_groups_test.go. The IntelliJ macOS keymap leaves editor.splitDown unbound, so GoIDEA has no default key |
| `editor.splitRight` | Split Right | `src/editor/core/actions.ts` | partial - internal/app/editor_split.go SplitEditorRight/splitEditor copies the active tab into the second window group over the shared docbridge document, so both panes edit one buffer and one undo history; Cmd+\ in internal/app/app.go; Window › Editor Tabs in internal/app/menu.go and the tab context menu's Split Right with its shortcut label in internal/app/workbench.go; internal/app/commands.go editor.splitRight; internal/app/editor_groups_test.go TestFocusGroupSwapsThePanesAndTheirPositions. GoIDEA keeps at most one other window group, so a further split adds its copy beside the first; dragging a tab to the editor edge to split is not ported |
| `editor.toggleColumnSelection` | Column Selection Mode | `src/editor/core/actions.ts` | missing |
| `editor.toggleRainbowBrackets` |  | `src/editor/core/actions.ts` | missing |
| `editor.toggleRenderedDocs` | Render Documentation Comments | `src/editor/core/actions.ts` | partial - internal/app registers the native action `editor.toggleRenderedDocs`; interaction-level acceptance is pending |
| `editor.toggleSemanticHighlighting` |  | `src/editor/core/actions.ts` | missing |
| `editor.toggleSoftWrap` | Soft-Wrap | `src/editor/core/actions.ts` | integrated - internal/app/commands.go editor.toggleSoftWrap toggles the active MyGo editor's effective wrapping and exposes it through ActionToggled; TestSoftWrapActionTogglesTheActiveTabAndState |
| `editor.toggleZen` |  | `src/workbench/distractionFree.ts` | integrated - internal/app/zen.go ToggleZen/StepZenWidth/zenWorkbench hints and compact header; prefs Zen/ZenWidth persisted; internal/app/gear_test.go TestViewModeTitleFollowsTheMode, TestZenWidthClamps |
| `editor.unsplit` | Unsplit | `src/editor/core/actions.ts` | partial - internal/app/editor_split.go Unsplit closes the focused group's tabs through the dirty guard and leaves the other group as the editor; Window › Editor Tabs and the tab context menu; internal/app/commands.go editor.unsplit; internal/app/editor_groups_test.go TestSettleGroupsCollapsesAnEmptyPane |
| `editor.unsplitAll` | Unsplit All | `src/editor/core/actions.ts` | partial - internal/app/editor_split.go UnsplitAll closes the other group's tabs through the dirty guard and keeps the active group; Window › Editor Tabs and the tab context menu; internal/app/commands.go editor.unsplitAll; internal/app/editor_groups_test.go |
| `file.closeProject` | Close Project | `src/workbench/shellActions.ts` | partial - internal/app registers the native action `project.close`; interaction-level acceptance is pending |
| `file.openProject` | Open… | `src/workbench/shellActions.ts` | partial - internal/app registers the native action `file.openProject`; interaction-level acceptance is pending |
| `file.reloadAllFromDisk` | Reload All from Disk | `src/editor/core/actions.ts` | integrated - internal/app/commands.go file.reloadAllFromDisk -> internal/app/watch.go ReloadAllFromDisk, which rereads every clean buffer through reloadTab (a shared ApplyEdits against the disk text), skips a buffer with unsaved edits and says so in the banner, and reports how many files it is reloading; File menu Reload All from Disk; base.watch.reloadAll |
| `file.reloadFromDisk` | Reload from Disk | `src/editor/core/actions.ts` | integrated - OpenIDEA id `file.reloadFromDisk` aliases GoIDEA action `project.reloadFromDisk`; internal/app/tree_ops.go refreshes the selected subtree from disk; base.files.reload and TestProjectTreeOpenIDEAActionsAreRegistered |
| `file.save` | Save All | `src/editor/core/actions.ts` | partial - internal/app registers the native action `file.save`; interaction-level acceptance is pending |
| `file.toggleReadOnly` | Toggle Read-only Attribute | `src/editor/core/actions.ts` | integrated - internal/app/editor_file_actions.go calls OpenIDEA fs_set_writable and updates every tab sharing the document; internal/app/commands.go and menu.go expose the action; TestToggleReadOnlyRequiresARealFileDocument, TestSetFileWritableUpdatesEveryTabSharingTheDocument and TestSetFileWritableFailureLeavesTheDocumentEditable |
| `find.field.newLine` | New Line in Search Field | `src/workbench/shellActions.ts` | missing |
| `find.field.recentSearches` | Recent Searches | `src/workbench/shellActions.ts` | missing |
| `find.inFile` | Find | `src/editor/core/actions.ts` | integrated - internal/app/commands.go runs the native MyGo editor Find bar, action.go keeps it distinct from view.find, and the Edit menu exposes it; TestFileFindActionsUseTheNativeFindBar and TestFileFindActionsKeepTheirOpenIDEAIdentity |
| `find.next` | Find Next Occurrence | `src/editor/core/actions.ts` | integrated - internal/app/commands.go runs editor.FindNext and the IntelliJ macOS Cmd+G binding is registered; TestFileFindActionsUseTheNativeFindBar and TestFileFindActionsKeepTheirOpenIDEAIdentity |
| `find.openInWindow` | Open Results in Find Window | `src/editor/core/actions.ts` | missing |
| `find.previous` | Find Previous Occurrence | `src/editor/core/actions.ts` | integrated - internal/app/commands.go runs editor.FindPrevious and the IntelliJ macOS Cmd+Shift+G binding is registered; TestFileFindActionsUseTheNativeFindBar and TestFileFindActionsKeepTheirOpenIDEAIdentity |
| `find.replaceInFile` | Replace | `src/editor/core/actions.ts` | integrated - internal/app/editor_keys.go binds Cmd+R to editor.FindReplace and commands.go runs it through the action registry; TestIntellijEditorBindingsMatchOpenIDEA and TestFileFindActionsUseTheNativeFindBar |
| `find.scrollToResults` | Scroll to Results During Typing | `src/editor/core/actions.ts` | missing |
| `layout.delete` | Delete Tool Window Layout… | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow_layouts.go registers OpenIDEA's layout.delete, renders the saved-layout chooser and confirmation in MyGo, and clears the active check when the active layout is deleted; TestNamedToolWindowLayoutsSaveRestoreRenameDeleteAndPersist |
| `layout.rename` | Rename Tool Window Layout… | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow_layouts.go implements Rename Layout… with chooser, name prompt, blank and duplicate validation, active-name migration and session persistence; TestNamedToolWindowLayoutsSaveRestoreRenameDeleteAndPersist |
| `layout.restore` | Restore Tool Window Layout | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow_layouts.go exposes layout.restore with an argument or native chooser, applies every dock, size, split, placement and view mode, exits Distraction Free Mode and marks the restored name active; TestNamedToolWindowLayoutsSaveRestoreRenameDeleteAndPersist and TestNamedToolWindowLayoutDialogRendersAndRestores |
| `layout.restoreDefault` | Restore Default Layout | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow.go applies OpenIDEA's fresh-project snapshot, and internal/app/toolwindow_layouts_test.go verifies the upstream id, view.resetLayout alias and Shift+F12 binding; TestRestoreDefaultLayoutMatchesOpenIDEAsFreshLayout and TestLayoutRestoreDefaultUsesOpenIDEAIDAndShortcut |
| `layout.save` | Save Current Layout as… | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow_layouts.go implements Save Current Layout as… with the active name prefilled, blank validation, replace-on-same-name, active state and shared session persistence; TestNamedToolWindowLayoutsSaveRestoreRenameDeleteAndPersist and TestNamedLayoutPersistenceIsSharedAcrossProjects |
| `navigate.back` | Back | `src/editor/core/actions.ts` | partial - internal/app registers the native action `navigate.back`; interaction-level acceptance is pending |
| `navigate.forward` | Forward | `src/editor/core/actions.ts` | partial - internal/app registers the native action `navigate.forward`; interaction-level acceptance is pending |
| `navigate.gotoLine` | Go to Line:Column… | `src/editor/core/actions.ts` | integrated - internal/app/goto_line.go accepts one-based line and UTF-16 column, with clamping; Navigate, Cmd+L and the status bar; internal/app/goto_line_test.go and TestRecentNavigationAndGotoLineRenderOffscreen |
| `navigate.lastEditLocation` | Last Edit Location | `src/editor/core/actions.ts` | partial - internal/app registers the native action `navigate.lastEditLocation`; interaction-level acceptance is pending |
| `navigate.navigationBar` | Jump to Navigation Bar | `src/editor/core/actions.ts` | integrated - internal/app/navigation_bar.go opens the native project › directory › file › symbol popup from Navigate and registers both Cmd+Up and Alt+Home; left/right traverse and clamp across every segment, Up/Down move child rows, Enter descends, and layered Escape closes the child list before the bar; parity_shell_actions_test.go verifies placement and bindings and navigation_bar_test.go TestNavigationBarKeyboardTraversalAndChildList verifies the native flow |
| `navigate.nextEditLocation` | Next Edit Location | `src/editor/core/actions.ts` | partial - internal/app registers the native action `navigate.nextEditLocation`; interaction-level acceptance is pending |
| `navigate.nextOccurrence` | Next Occurrence | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/problems.go StepProblem; internal/app/toolwindow_test.go |
| `navigate.prevOccurrence` | Previous Occurrence | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/problems.go StepProblem; internal/app/toolwindow_test.go |
| `refactor.thisMenu` | Refactor This… | `src/editor/lsp/actions.ts` | missing |
| `toolbar.customize` | Customize Main Toolbar… | `src/workbench/shellActions.ts` | integrated - internal/app/toolbar.go OpenToolbarCustomize/toolbarCustomizeDialog draft with Add/Remove/Move/side switch and Restore Defaults; internal/app/gear_test.go TestToolbarDraftEdits |
| `toolwindow.${id}` |  | `src/workbench/toolwindows/actions.ts` | missing |
| `toolwindow.closeContent` | Close Tool Window Tab | `src/workbench/toolwindows/actions.ts` | partial - internal/app/toolwindow.go closes the active terminal content tab before hiding the tool window, synchronizes the dock when the last tab closes, and carries OpenIDEA's Cmd+W and Ctrl+Shift+F4 bindings; TestBackgroundTasksOpenEmptyStopAllAndToolWindowCloseContent and TestOpenIDEAShellShortcuts; Find and Run do not yet expose multiple content tabs of their own |
| `toolwindow.focusEditor` | Focus Editor | `src/workbench/toolwindows/actions.ts` | partial - internal/app registers the native action `toolwindow.focusEditor`; interaction-level acceptance is pending |
| `toolwindow.hideActive` | Hide Active Tool Window | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow.go HideActiveToolWindow; internal/app/toolwindow_test.go |
| `toolwindow.hideAll` | Hide All Tool Windows | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow.go HideAllToolWindows; internal/app/toolwindow_test.go |
| `toolwindow.jumpToLast` | Jump to Last Tool Window | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow.go JumpToLastToolWindow; internal/app/toolwindow_test.go |
| `toolwindow.maximize` | Maximize Tool Window | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow.go ToggleMaximizeToolWindow; internal/app/app.go View; internal/app/maximize_test.go |
| `toolwindow.resize${dir[0]!.toUpperCase()}${dir.slice(1)}` |  | `src/workbench/toolwindows/actions.ts` | missing |
| `toolwindow.viewMode.${mode}` |  | `src/workbench/toolwindows/actions.ts` | missing |
| `view.backgroundTasks` | Background Tasks | `src/workbench/shellActions.ts` | integrated - internal/app/tasks.go ShowBackgroundTasks opens the registered workbench.backgroundTasks native popup regardless of status-bar visibility and renders No background tasks when the queue is empty; TestBackgroundTasksOpenEmptyStopAllAndToolWindowCloseContent |
| `view.notifications` | Notifications | `src/workbench/shellActions.ts` | partial - internal/app registers the native action `view.notifications`; interaction-level acceptance is pending |
| `view.quickSwitchScheme` | Quick Switch Scheme… | `src/workbench/shellActions.ts` | partial - internal/app registers the native action `view.quickSwitchScheme`; interaction-level acceptance is pending |
| `view.toggleStatusBarWidget` | Status Bar Widget | `src/workbench/shellActions.ts` | integrated - internal/app/statuscontrib.go resolves a named StatusItem, persists one appearance.statusBarWidgets override and invalidates the bar; internal/app/menu.go lists named built-in and plugin widgets under View › Appearance › Status Bar Widgets, and the status-bar context menu uses the same rows; TestNavigationBarPlacementAndStatusWidgetPersistence and TestStatusBarContextMenuTogglesWidgets |
| `view.toggleTheme` | Toggle Light/Dark Theme | `src/workbench/shellActions.ts` | partial - internal/app registers the native action `view.toggleTheme`; interaction-level acceptance is pending |
| `workbench.stopBackgroundProcesses` | Stop Background Processes… | `src/workbench/shellActions.ts` | integrated - internal/app/tasks.go StopBackgroundProcesses cancels one task directly, opens a chooser for several cancellable tasks with Stop All, and reports status when none exist; the action carries OpenIDEA's Cmd+Shift+F2 binding; TestBackgroundTasksOpenEmptyStopAllAndToolWindowCloseContent and TestOpenIDEAShellShortcuts |

### Keymap-only ids

Ids bound by a preset but not registered by an action in the source: they are
expected to come from a plugin or a hidden registration.

| Action id | Bound in presets | Status |
| --- | --- | --- |
| `code.addCaretNextOccurrence` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.addCaretsToLineEnds` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.applyQuickFix` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.autoIndentLines` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.collapseBlock` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.collapseRecursively` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.commentBlock` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/editor_keys.go intellijEditorBindings Cmd+Alt+/ and Ctrl+Shift+/; internal/app/editor_keys_test.go |
| `code.commentLine` | intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/editor_keys.go intellijEditorBindings Cmd+/ and code.commentLine in internal/app/commands.go and the Code menu; internal/app/editor_keys_test.go |
| `code.completeStatement` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.completion` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.cyclicExpandWord` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.cyclicExpandWordBackward` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.deleteLine` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/editor_keys.go intellijEditorBindings Cmd+Backspace to editor.DeleteLine; internal/app/editor_keys_test.go |
| `code.duplicateLine` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/editor_keys.go intellijEditorBindings Cmd+D to editor.DuplicateSelection (line when nothing is selected); internal/app/editor_keys_test.go |
| `code.expandAllToLevel1` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expandAllToLevel2` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expandAllToLevel3` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expandAllToLevel4` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expandAllToLevel5` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expandRecursively` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expandToLevel1` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expandToLevel2` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expandToLevel3` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expandToLevel4` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expandToLevel5` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.expressionTypeInfo` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.extendSelection` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.fold` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.foldAll` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.foldSelection` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.generate` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.implementMethods` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.joinLines` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | integrated - internal/app/editor_keys.go Ctrl+Shift+J to editor.JoinLines; internal/app/editor_keys_test.go |
| `code.moveElementLeft` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.moveElementRight` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.moveLineDown` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/editor_keys.go Alt+Shift+Down to editor.MoveLinesDown; internal/app/editor_keys_test.go |
| `code.moveLineUp` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/editor_keys.go Alt+Shift+Up to editor.MoveLinesUp; internal/app/editor_keys_test.go |
| `code.moveStatementDown` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | partial - internal/app/editor_keys.go Cmd+Shift+Down runs editor.MoveLinesDown; moves whole lines, not the statement tree IntelliJ parses |
| `code.moveStatementUp` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | partial - internal/app/editor_keys.go Cmd+Shift+Up runs editor.MoveLinesUp; moves whole lines, not the statement tree IntelliJ parses |
| `code.optimizeImports` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | partial - internal/app registers the native action `code.optimizeImports`; interaction-level acceptance is pending |
| `code.overrideMethods` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.parameterInfo` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.pasteAsPlainText` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.pasteFromHistory` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.quickDefinition` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app registers the native action `code.quickDefinition`; interaction-level acceptance is pending |
| `code.quickDocumentation` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | partial - internal/app/app.go routes F1 and Ctrl+J to Quick Documentation |
| `code.reformat` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.reformatDialog` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.reverseLines` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.selectAllOccurrences` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.showErrorDescription` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.showIntentions` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.shrinkSelection` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.sortLines` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.splitLine` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.startNewLine` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.startNewLineBefore` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.toggleCase` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.unfold` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.unfoldAll` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.unselectOccurrence` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.unwrap` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.cloneCaretAbove` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.cloneCaretBelow` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.codeBlockEnd` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.codeBlockEndWithSelection` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.codeBlockStart` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.codeBlockStartWithSelection` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.contextInfo` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.inlineCompletion.accept` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.inlineCompletion.acceptWord` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.inlineCompletion.dismiss` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.inlineCompletion.next` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.inlineCompletion.previous` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.matchBrace` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.scrollToCenter` | intellij-macos, intellij-windows, vscode-macos | missing |
| `editor.toggleInsertState` | intellij-macos, intellij-windows, vscode-macos | missing |
| `find.highlightUsages` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `find.popup.nextResult` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `find.popup.openResult` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `find.popup.previousResult` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `find.popup.recentSearches` | intellij-macos, intellij-windows, vscode-macos | missing |
| `github.createPullRequest` | intellij-macos, intellij-windows, vscode-macos | missing |
| `github.pullRequest.refresh` | intellij-macos, intellij-windows, vscode-macos | missing |
| `github.pullRequest.timeline.refresh` | intellij-macos, intellij-windows, vscode-macos | missing |
| `github.pullRequest.toggleViewed` | intellij-macos, intellij-windows, vscode-macos | missing |
| `github.refresh` | intellij-macos, intellij-windows, vscode-macos | missing |
| `image.actualSize` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `image.fitToWindow` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `image.toggleChessboard` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `image.zoomIn` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `image.zoomOut` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `navigate.customRegion` | intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.declaration` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.gotoTest` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.implementation` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | partial - internal/app registers the native action `navigate.implementation`; interaction-level acceptance is pending |
| `navigate.nextError` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.nextHighlightedUsage` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.nextMethod` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.prevError` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.prevHighlightedUsage` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.previousMethod` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.relatedSymbol` | vscode-macos | missing |
| `navigate.superMethod` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.typeDeclaration` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | partial - internal/app registers the native action `navigate.typeDeclaration`; interaction-level acceptance is pending |
| `refactor.changeSignature` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.extractConstant` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.extractField` | intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.extractMethod` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.extractParameter` | intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.extractVariable` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.inline` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.move` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.pullMembersUp` | intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.pushMembersDown` | intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.rename` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `refactor.safeDelete` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/editor_safe_delete.go mirrors OpenIDEA's editor refactor.safeDelete with definition/outline resolution, JDT references, Find Usages transfer, Delete Anyway, comment/string preview, remembered search text and stale-edit cancellation; the command, Refactor menu, editor context menu, command palette and Cmd+Delete entry are wired through internal/app/commands.go, menu.go, editor_menu.go and overlay_registry.go; lsp.safeDelete.usages, lsp.safeDelete.view, lsp.safeDelete.anyway, lsp.safeDelete.text and lsp.safeDelete.editCancel pass against real JDT; editor_safe_delete_test.go covers local declarations and refusals |
| `toolwindow.bookmarks` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/app.go Cmd+2 ToggleBookmarksWindow; cmd/goidea/main.go 显示书签; internal/app/view.go sidebar tab; base.bookmark.window |
| `toolwindow.commit` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/commit.go OpenCommit; Cmd+0 in cmd/goidea/main.go |
| `toolwindow.debug` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/debug.go ShowDebug; cmd/goidea/main.go 视图 menu |
| `toolwindow.find` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | partial - internal/app/search.go ShowSearch; cmd/goidea/main.go 视图 menu |
| `toolwindow.git` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/vcs.go ShowVCS; cmd/goidea/main.go 视图 menu |
| `toolwindow.problems` | intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/problems.go ToggleProblems; lsp.problems.visible |
| `toolwindow.project` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | partial - internal/app/app.go ShowProject; cmd/goidea/main.go 视图 menu |
| `toolwindow.resizeDown` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/toolwindow.go ResizeBottomToolWindow; internal/app/maximize_test.go |
| `toolwindow.resizeLeft` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/toolwindow.go ResizeBottomToolWindow no-ops sideways on a bottom window; internal/app/maximize_test.go |
| `toolwindow.resizeRight` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/toolwindow.go ResizeBottomToolWindow no-ops sideways on a bottom window; internal/app/maximize_test.go |
| `toolwindow.resizeUp` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/toolwindow.go ResizeBottomToolWindow; internal/app/maximize_test.go |
| `toolwindow.run` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/run.go ShowRun; cmd/goidea/main.go 视图 menu |
| `toolwindow.services` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `toolwindow.structure` | intellij-macos, intellij-windows, vscode-macos | integrated - internal/app/structure.go ToggleStructure; lsp.structure.visible; internal/app/structure_test.go |
| `toolwindow.terminal` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/term.go ShowTerm; UPSTREAM_FEATURE_MATRIX.md Terminal |

## Keymaps

| Preset | Bindings | Status |
| --- | ---: | --- |
| `eclipse-macos` | 147 | missing |
| `eclipse-windows` | 148 | missing |
| `intellij-macos` | 370 | missing |
| `intellij-windows` | 370 | missing |
| `vscode-macos` | 372 | missing |
| `vscode-windows` | 41 | missing |

Counts are the bindings written literally in a preset file. A preset may spread
another (`vscode-windows` translates and extends `vscode-macos`), so its effective
map is larger; the union above and the keymap-only ids below are complete.
GoIDEA's key handling is its own: a preset id does not imply that GoIDEA ships the
same bindings until a check records it.

## Tool windows

| Tool window | Title | Anchor | Source | Status |
| --- | --- | --- | --- | --- |
| `bookmarks` | Bookmarks | left | `src/features/search/index.ts` | integrated - internal/app/bookmarks.go bookmarkWindowRows, ShowBookmarksWindow Cmd+2, move chooser 移动 with OpenMoveBookmark/OpenNewBookmarkListForMove and folder rows; Cmd+F3 popup edit description (F2) in bookmarksDialog; internal/app/view.go sidebar tab and project tree 书签 chip; base.bookmark.window, base.bookmark.popup, base.bookmark.move, base.bookmark.folder |
| `build` | Build | bottom | `src/features/build/index.ts` | partial - base.build.console, base.build.lines; internal/app/view.go buildPanel |
| `commit` | Commit | left | `src/features/vcs/index.ts` | partial - internal/app/commit.go commitDialog; base.commit.id |
| `compare` | Compare | left | `src/features/vcs/compareTool.tsx` | missing |
| `coverage` | Coverage | right | `src/features/coverage/index.ts` | integrated - internal/app/coverage_view.go renders Coverage suite tabs, compact package/file rollups and OpenIDEA line/branch percentages; TestCoverageWindowRendersLineAndBranchMetrics, TestCoverageMetricsRollUpThroughCompactDirectories and TestCoverageEmptyStateMatchesOpenIDEA cover the native window |
| `database` | Database | right | `src/features/database/index.ts` | partial - UPSTREAM_FEATURE_MATRIX.md database console; db_* commands |
| `debug` | Debug | bottom | `src/features/debugger/index.ts` | integrated - debug.paused, debug.vars, debug.step.paused, debug.watch |
| `documentation` | Documentation | right | `src/editor/lsp/index.ts` | partial - lsp.documentation and TestDocumentationFocusAndKeyboardDock; completion and dependency-editor triggers are pending |
| `dotnet` | .NET | right | `src/features/dotnet/index.ts` | missing |
| `duplicates` | Duplicates | bottom | `src/features/analysis/index.ts` | missing |
| `find` | Find | bottom | `src/features/search/index.ts` | integrated - base.search.done, base.search.files, base.search.matches; internal/app/search.go searchPanel; bottom anchor in internal/app/anchor_test.go |
| `git` | Git | bottom | `src/features/vcs/index.ts` | partial - base.vcs.changes, base.log.*, base.branch.* and base.stage.* cover the native changes, log, branch and staging views; internal/app/vcs_operations.go adds fetch, pull, push, merge, rebase, tag, branch, stash and reset and vcsoperations.* exercises the real backend; remote management, worktrees and compare views remain |
| `gradle` | Gradle | right | `src/features/build-tools/index.ts` | partial - internal/app/maven.go uses build_model to list Gradle build directories and errors within the Maven window; separate Gradle window, project/task tree and real Gradle acceptance pending |
| `hierarchy` | Hierarchy | right | `src/features/code-insight/index.ts` | integrated - internal/app/hierarchy.go hierarchyRightView: right-anchored window with Call/Type direction tabs, lazy children per node and recursion leaves; lsp_call_hierarchy, lsp_type_hierarchy; e2e hierarchy.rows, hierarchy.visible, hierarchyError; internal/app/hierarchy_test.go; internal/app/workbench_render_test.go TestWorkbenchHierarchyRightWindowRenders |
| `inspection` | Inspection Results | bottom | `src/features/analysis/index.ts` | missing |
| `logcat` | Logcat | bottom | `src/features/logcat/index.ts` | missing |
| `maven` | Maven | right | `src/features/build-tools/index.ts` | partial - internal/app/maven.go and dockregistry.go; right-side Maven reactor, lifecycle and plugin goals, profiles, dependencies, and reload; internal/app/maven_test.go and UPSTREAM_FEATURE_MATRIX.md Maven row; dependency navigation, add and exclude are pending |
| `notifications` | Notifications | right | `src/workbench/activate.ts` | partial - internal/app/notifications.go balloons (3 newest, 10 s fade, sticky errors, hover pause, keyed replacement, actions) and notificationsView history with unread badge and Clear All; build results and failures notify; plugin notifications preserve their verbatim multiline body, grouping, actions and 20-second timeout; base.notify.* in the e2e; Settings › Appearance & Behavior › Notifications with the global balloon switch, per-group Balloon/Sticky/No popup and Log, and the balloon's Don't Show Again; sound remains pending |
| `problems` | Problems | bottom | `src/features/code-insight/index.ts` | integrated - internal/app/problems.go problemsPanel; lsp.problems.errors, lsp.problems.rows, lsp.problems.visible; internal/app/problems_test.go |
| `profiler` | Profiler | bottom | `src/features/profiler/index.ts` | missing |
| `project` | Project | left | `src/features/project-tree/index.ts` | integrated - internal/workspace tree tests; base.tabs |
| `pullRequests` | Pull Requests | left | `src/features/github/index.ts` | missing |
| `remote.ports` | Ports | bottom | `src/features/remote-dev/index.ts` | blocked - OpenIDEA registers Ports only after an SSH or dev-container session; remote_forwarded_ports, remote_forward_port and remote_stop_forwarding exist only in the pinned desktop shell, not in openidea-server --stdio. internal/app/remote.go renders the unavailable native state until the OpenIDEA fork exposes matching RPCs |
| `remoteHost` | Remote Host | right | `src/features/deployment/index.ts` | missing |
| `run` | Run | bottom | `src/features/run/index.ts` | integrated - base.run.config, base.run.exit, base.run.build |
| `services` | Services | bottom | `src/features/services/index.ts` | partial - internal/app/services.go: build system detection, run configurations and tool settings rows that run their action; internal/app/services_test.go; missing the compose/plugin service model and per-service logs |
| `structure` | Structure | left | `src/features/search/index.ts` | integrated - internal/app/structure.go structureRowsUI; lsp.structure.rows, lsp.structure.first, lsp.structure.visible; internal/app/structure_test.go |
| `terminal` | Terminal | bottom | `src/features/terminal/index.ts` | partial - internal/app/term.go TermPane tab groups, OpenTermTab Cmd+T, SplitTerm/UnsplitTerm with termPaneBody in view.go, ShowTermFind/FindTermStep Cmd+F find bar, CloseTermTab hides the empty window; base.term.output, base.term.tabs, base.term.split, base.term.find; internal/app/term_test.go |
| `todo` | TODO | bottom | `src/features/todo/index.ts` | integrated - internal/app/todo.go todoPanel with 项目/当前文件/按范围/更改 tabs, keyword chips and scope bar over search_start; base.todo.done, base.todo.items, base.todo.openFiles, base.todo.visible; internal/app/todo_test.go |
| `usages` | Find Usages | bottom | `src/features/code-insight/index.ts` | integrated - internal/app/usages.go showUsages and usagesPanel; lsp.references.count, lsp.references.done; bottom anchor in internal/app/anchor_test.go |

## Overlays

| Overlay | Source | Status |
| --- | --- | --- |
| `analysis.dialogs` | `src/features/analysis/index.ts` | missing |
| `app.about` | `src/features/app/index.ts` | missing |
| `app.invalidateCaches` | `src/features/app/index.ts` | missing |
| `app.keymapReference` | `src/features/app/index.ts` | missing |
| `app.newProject` | `src/features/app/index.ts` | missing |
| `app.termination` | `src/features/app/index.ts` | missing |
| `app.workspace` | `src/features/app/index.ts` | missing |
| `build-tools.execute` | `src/features/build-tools/index.ts` | missing |
| `code-insight.usagesPopup` | `src/features/code-insight/index.ts` | integrated - internal/app/overlay_registry.go registers code-insight.usagesPopup; internal/app/usage_popup.go renders the caret-anchored native speed-search popup with scope, read/write filters, bounded scrolling and a 60 ms-debounced source preview; TestUsagePopupRendersAndSpeedSearches, TestEditorCaretAnchorUsesScrolledLineTop and TestUsagePopupPreviewLoadIsDebounced cover the native interaction, while lsp.usagesPopup.open and lsp.usagesPopup.transfer prove the real JDT path |
| `coverage.manageReports` | `src/features/coverage/index.ts` | integrated - internal/app/coverage.go opens and closes the native report-management overlay and coverage_view.go renders suite selection, import metadata and removal; TestCoverageReportsRestoreWithoutDeletingSourceReport and TestCoverageActionsFollowOpenIDEAContextState cover it |
| `database.dialogs` | `src/features/database/index.ts` | missing |
| `debugger` | `src/features/debugger/index.ts` | missing |
| `deployment.dialogs` | `src/features/deployment/index.ts` | missing |
| `editor-lsp.hierarchyDialog` | `src/editor/lsp/index.ts` | missing |
| `editor-lsp.popups` | `src/editor/lsp/index.ts` | missing |
| `editor.gotoLine` | `src/editor/core/index.ts` | integrated - internal/app/goto_line.go and overlay_registry.go; TestRecentNavigationAndGotoLineRenderOffscreen draws the popup; goto_line_test.go covers submission, validation and cancellation |
| `editor.navigationBar` | `src/editor/core/index.ts` | missing |
| `editor.pasteHistory` | `src/editor/core/index.ts` | missing |
| `github.dialogs` | `src/features/github/index.ts` | missing |
| `graphql.environment` | `src/features/graphql/index.ts` | missing |
| `macros.dialogs` | `src/features/macros/index.ts` | missing |
| `mcp.confirm` | `src/features/mcp/index.ts` | missing |
| `perf.overlay` | `src/features/perf/index.ts` | missing |
| `plugins.consent` | `src/features/plugins/index.ts` | missing |
| `plugins.intellijThemes` | `src/features/plugins/index.ts` | missing |
| `project.fileRefactor` | `src/features/project-tree/index.ts` | integrated - internal/app/overlay_registry.go registers project.fileRefactor; internal/app/project_file_refactor.go renders native Rename, Move and Copy dialogs with reference search options; TestProjectFileRefactorOverlaysRenderNativeDialogs and TestProjectMoveDialogValidatesTargetAndUsesRefactoring |
| `project.newPopup` | `src/features/project-tree/index.ts` | integrated - internal/app/overlay_registry.go registers project.newPopup; internal/app/project_new.go renders OpenIDEA's 280 DIP anchored search field, File, Directory, built-in file kinds and From Template row; TestProjectNewPopupRendersOpenIDEAFilterAndActions |
| `project.refactoringPreview` | `src/features/project-tree/index.ts` | integrated - internal/app/overlay_registry.go registers project.refactoringPreview; internal/app/project_file_refactor.go renders included files, occurrence summaries and before/after edits before Do Refactor; TestProjectFileRefactorOverlaysRenderNativeDialogs and TestProjectMoveDialogValidatesTargetAndUsesRefactoring |
| `project.trustDialog` | `src/features/project-trust/index.tsx` | integrated - internal/app/overlay_registry.go registers OpenIDEA id project.trustDialog; internal/app/run.go renders the 560 DIP GoIDEA-logo dialog with Preview in Restricted Mode, Don't Open, Trust Project and Trust projects in parent-folder scope; TestProjectTrustPromptMatchesOpenIDEA and the trust screenshot |
| `remote.connect` | `src/features/remote-dev/index.ts` | partial - internal/app/overlay_registry.go registers remote.connect and internal/app/remote.go renders the native unavailable chooser; TestRemoteConnectAndReopenDialogsExplainBackendGap |
| `run.chooser` | `src/features/run/index.ts` | missing |
| `run.dialogs` | `src/features/run/index.ts` | missing |
| `run.editConfigs` | `src/features/run/index.ts` | partial - internal/app/run_config_dialog.go runConfigsDialog drawn by the overlay registry (internal/app/overlay_registry.go runConfigs) |
| `run.serviceImport` | `src/features/run/index.ts` | missing |
| `run.stackTraceDialog` | `src/features/run/stackTrace.tsx` | partial - internal/app/stack_trace.go provides the native text dialog, clipboard prefill, Analyze and Cancel, and the trace console; internal/app/stack_trace_test.go; remains subject to visual parity verification |
| `runAnything.popup` | `src/features/run-anything/index.ts` | integrated - internal/app/run_anything.go runAnythingDialog, drawn by View in internal/app/app.go; internal/app/run_anything_test.go |
| `scratch.chooser` | `src/features/scratch/index.ts` | integrated - internal/app/scratch.go and overlay_registry.go; internal/app/scratch_test.go and base.scratch checks |
| `search.bookmarks` | `src/features/search/index.ts` | missing |
| `search.everywhere` | `src/features/search/index.ts` | missing |
| `search.fileStructure` | `src/features/search/index.ts` | missing |
| `search.findInFiles` | `src/features/search/index.ts` | missing |
| `search.recent` | `src/features/search/index.ts` | partial - internal/app/recent_navigation_view.go paints the file and tool window columns plus the locations and snippets list; TestRecentNavigationAndGotoLineRenderOffscreen; see the two navigate.recent actions for gaps |
| `search.scopes` | `src/features/search/index.ts` | missing |
| `services.dialogs` | `src/features/services/index.ts` | missing |
| `settings.assignShortcut` | `src/features/settings/index.ts` | missing |
| `settings.dialog` | `src/features/settings/index.ts` | missing |
| `settings.projectStructure` | `src/features/settings/index.ts` | missing |
| `ssh.askpass` | `src/features/ssh/index.ts` | missing |
| `templates.chooser` | `src/features/templates/index.ts` | missing |
| `templates.emmetUpdateTag` | `src/features/templates/index.ts` | missing |
| `templates.newFile` | `src/features/templates/index.ts` | integrated - internal/app/overlay_registry.go registers OpenIDEA's templates.newFile overlay over the native name and kind prompt; TestProjectNewTemplateSearchAndOpenState verifies the overlay opens only after a kind is chosen |
| `vcs.branches` | `src/features/vcs/index.ts` | missing |
| `vcs.dialogs` | `src/features/vcs/index.ts` | missing |
| `welcome.recentProjects` | `src/features/welcome/index.ts` | integrated - internal/app/overlay_registry.go registers OpenIDEA id welcome.recentProjects; internal/app/recent_projects.go renders the 560 DIP chooser with 40 DIP rows, search, path, branch, menus and delete confirmation; internal/app/recent_projects_test.go and the recent-projects screenshot |
| `workbench.backgroundTasks` | `src/workbench/activate.ts` | partial - internal/app/tasks.go shows updater task progress and cancellation from the status bar; registration for other background task producers is pending |
| `workbench.customizeToolbar` | `src/workbench/activate.ts` | missing |

## Editor contributions and file editor types

### Editor contributions

| Contribution | Source | Status |
| --- | --- | --- |
| `build-tools.pomCompletion` | `src/features/build-tools/index.ts` | missing |
| `build-tools.pomNavigation` | `src/features/build-tools/index.ts` | missing |
| `build-tools.versionCatalog` | `src/features/build-tools/index.ts` | missing |
| `colorPreview.gutter` | `src/features/color-preview/index.ts` | missing |
| `coverage.gutter` | `src/features/coverage/index.ts` | integrated - internal/app/coverage.go paints covered and missed line stripes, follows edits until another run and supplies OpenIDEA's line/branch title; TestCoverageGutterMarksFollowEditsUntilAnotherRun and TestCoverageGutterTooltipMatchesOpenIDEATitles cover the contribution |
| `database.console` | `src/features/database/index.ts` | missing |
| `debugger` | `src/features/debugger/index.ts` | missing |
| `dependency-versions` | `src/features/dependencies/index.ts` | missing |
| `github.reviewMode` | `src/features/github/reviewMode/index.ts` | missing |
| `graphql.toolbar` | `src/features/graphql/index.ts` | missing |
| `http.endpoints` | `src/features/http-client/index.ts` | missing |
| `http.requests` | `src/features/http-client/index.ts` | missing |
| `http.spring` | `src/features/http-client/index.ts` | missing |
| `lsp` | `src/editor/lsp/index.ts` | missing |
| `lsp.lineMarkers` | `src/editor/lsp/index.ts` | missing |
| `lsp.parameterTab` | `src/editor/lsp/index.ts` | missing |
| `macros.recorder` | `src/features/macros/index.ts` | missing |
| `merge-conflict-markers` | `src/features/vcs/conflictMarkers.ts` | missing |
| `plugins.decorations` | `src/features/plugins/decorations.ts` | missing |
| `plugins.indexedMarkers` | `src/features/plugins/indexedMarkers.ts` | missing |
| `plugins.languageFeatures` | `src/features/plugins/index.ts` | missing |
| `plugins.lineMarkers` | `src/features/plugins/index.ts` | missing |
| `plugins.navigation` | `src/features/plugins/navigation.ts` | missing |
| `run.npmScripts` | `src/features/run/index.ts` | missing |
| `run.targets` | `src/features/run/index.ts` | missing |
| `search.bookmarks` | `src/features/search/index.ts` | missing |
| `search.tracking` | `src/features/search/common/tracking.ts` | missing |
| `services.composeEditor` | `src/features/services/index.ts` | missing |
| `services.dockerGutter` | `src/features/services/index.ts` | missing |
| `templates` | `src/features/templates/index.ts` | missing |
| `todo.highlight` | `src/features/todo/index.ts` | partial - internal/app/todo.go todoMarksOf and refreshTodoHighlights: the open buffer's TODOs, matched by the same keyword and comment rules and rescanned 400ms after typing pauses, paint as editor.TextBackground spans tinted in the pattern's tone (internal/app/app.go View), proven by base.todo.highlight; OpenIDEA colours and italicises the comment text instead, which the MyGo editor has no foreground-span API for, so the tint is the native equivalent |
| `vcs.changeMarkers` | `src/features/vcs/index.ts` | missing |
| `viewers.markdownLive` | `src/features/viewers/markdown/live.ts` | missing |
| `vim` | `src/features/vim/index.ts` | missing |
| `web.djangoContent` | `src/features/web/index.ts` | missing |

### Editor kinds (file editor types)

The plain text editor is the workbench default and is not registered through
`registerEditorKind`; the kinds below are the non-text inputs.

| Kind | Source | Status |
| --- | --- | --- |
| `git-log` | `src/features/vcs/index.ts` | missing |
| `git-merge-diff` | `src/features/vcs/index.ts` | missing |
| `git-revision` | `src/features/vcs/index.ts` | missing |
| `notebook` | `src/features/notebook/index.ts` | missing |

### File editor providers

| Provider | Source | Status |
| --- | --- | --- |
| `notebook` | `src/features/notebook/index.ts` | missing |
| `plugin.${pluginId}.${decl.id}` | `src/features/plugins/uiPoints.tsx` | missing |
| `viewers.hex` | `src/features/viewers/index.ts` | missing |
| `viewers.image` | `src/features/viewers/index.ts` | missing |

## Settings

### Pages

| Page | Title | Group | Source | Status |
| --- | --- | --- | --- | --- |
| `agents.chat` | Chat | Agents | `src/features/settings/pages/tools.tsx` | integrated - internal/app/settings_pages.go renders OpenIDEA's Chat settings over the persisted agent preferences; internal/app/settings_test.go and agent_composer_test.go |
| `agents.list` | Coding Agents | Agents | `src/features/agents/index.ts` | integrated - internal/app/settings_window.go registers Coding Agents with OpenIDEA's exact agents.list id and internal/app/agent_registry_settings.go renders providers, registry and launch settings; TestAgentRegistryPageShowsProvidersAndLaunchSettings |
| `agents.mcpServer` | IDE MCP Server | Agents | `src/features/settings/pages/mcpServer.tsx` | missing |
| `agents.mcpServers` | External MCP Servers | Agents | `src/features/agents/index.ts` | integrated - internal/app/settings_window.go registers External MCP Servers with OpenIDEA's exact agents.mcpServers id and internal/app/agent_mcp_settings.go renders user/workspace servers; internal/app/agent_registry_test.go |
| `agents.policy` | Command Permissions | Agents | `src/features/agents/index.ts` | integrated - internal/app/agent_policy_settings.go implements OpenIDEA's Command Permissions page and policy validation; internal/app/settings_test.go and agent_registry_test.go |
| `appearance` | Appearance | Appearance & Behavior | `src/features/settings/pages/behavior.tsx` | partial - base.settings.theme; internal/app/settings_pages.go settingsAppearanceBody Theme row (Select + Sync with OS checkbox); the preferred-themes gear is not ported |
| `build.cmake` | CMake | Build, Execution, Deployment | `src/features/settings/pages/execution.tsx` | missing |
| `build.console` | Console | Build, Execution, Deployment | `src/features/settings/pages/execution.tsx` | missing |
| `build.debugger` | Debugger | Build, Execution, Deployment | `src/features/settings/pages/execution.tsx` | missing |
| `build.debugger.dataViews` | Data Views |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.debugger.stepping` | Stepping |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.deployment` | Deployment | Build, Execution, Deployment | `src/features/settings/pages/deployment.tsx` | missing |
| `build.deployment.options` | Options |  | `src/features/settings/pages/deployment.tsx` | missing |
| `build.gradle` | Gradle | Build, Execution, Deployment | `src/features/settings/pages/execution.tsx` | missing |
| `build.maven` | Maven | Build, Execution, Deployment | `src/features/settings/pages/execution.tsx` | missing |
| `build.pythonDebugger` | Python Debugger | Build, Execution, Deployment | `src/features/settings/pages/execution.tsx` | missing |
| `build.run` | Run/Debug | Build, Execution, Deployment | `src/features/settings/pages/execution.tsx` | missing |
| `build.runTargets` | Run Targets | Build, Execution, Deployment | `src/features/settings/pages/runTargets.tsx` | missing |
| `editor.codeEditing` | Code Editing |  | `src/features/settings/pages/editor/general.tsx` | missing |
| `editor.codeStyle` | Code Style | Editor | `src/features/settings/pages/editor/codeStyle.tsx` | missing |
| `editor.codeStyle.other` | Other File Types |  | `src/features/settings/pages/editor/codeStyle.tsx` | missing |
| `editor.colorScheme` | Color Scheme |  | `src/features/settings/pages/editor/colorScheme.tsx` | missing |
| `editor.colorScheme.console-colors` | Console Colors |  | `src/features/settings/pages/editor/colorScheme.tsx` | missing |
| `editor.colorScheme.debugger` | Debugger |  | `src/features/settings/pages/editor/colorScheme.tsx` | missing |
| `editor.colorScheme.general` | General |  | `src/features/settings/pages/editor/colorScheme.tsx` | missing |
| `editor.colorScheme.language-defaults` | Language Defaults |  | `src/features/settings/pages/editor/colorScheme.tsx` | missing |
| `editor.fileEncodings` | File Encodings |  | `src/features/settings/pages/editor/codeStyle.tsx` | missing |
| `editor.fileTemplates` | File and Code Templates | Editor | `src/features/templates/index.ts` | missing |
| `editor.files` | File Types | Editor | `src/features/settings/pages/builtins.tsx` | missing |
| `editor.font` | Font |  | `src/features/settings/pages/editor/font.tsx` | integrated - internal/app/settings_pages.go settingsFontBody renders OpenIDEA's Editor › Font controls in MyGo: installed-family selector with the bundled JetBrains Mono, monospace-only filter, size, line height, ligatures, main weight, fallback family and a live editable preview; settings_window.go stages every choice and previews tabs before Apply, then persists Preferences.Font and FontSize; TestSettingsFontStagesPreviewAndPersistsEveryTypographyChoice verifies defaults, preview, Apply and the stored file |
| `editor.general` | General |  | `src/features/settings/pages/editor/general.tsx` | partial - internal/app/settings_pages.go settingsGeneralBody: Soft-wrap files over Preferences.Editor.SoftWrap with the File patterns field matched against each tab's base name by EditorAppearance.WrapsFile (default *.md; *.txt; *.rst; *.adoc, * wraps every file) and Show virtual space at file bottom over Preferences.Editor.ScrollPastEnd feeding the MyGo editor, plus the Save Files section (Remove trailing spaces on Modified lines/All lines, Remove trailing blank lines at the end of saved files and Ensure every saved file ends with a line break) over Preferences.SaveFiles, applied by internal/app/save_files.go cleanForSave before every save through docbridge SavedText and a single undoable ApplyEdits step, staged through settingsDraft and proven by settings_test.go TestSettingsGeneralWrapsAndVirtualSpace and save_files_test.go TestSaveTextCleansTheDocument, TestSaveEditsTurnTheTextIntoTheSavedOne, TestCleanForSaveLeavesTheDocumentReady, TestSettingsSaveFilesStageAndApply; Markdown Images, Mouse Control, Scrolling, the soft-wrap indent child, the Keep trailing spaces on caret line child and Reformat code on save are not ported |
| `editor.general.appearance` | Appearance |  | `src/features/settings/pages/editor/general.tsx` | partial - internal/app/settings_pages.go settingsEditorAppearanceBody: Use block caret over Preferences.Editor.BlockCaret feeding the MyGo caret shape (CursorBar / CursorBlock), Show line numbers, Show whitespaces and Show indent guides checkboxes over Preferences.Editor feeding the MyGo editor gutter and whitespace state, plus Distraction free mode text width 20-500 chars over Preferences.ZenWidth, staged through settingsDraft and proven by settings_test.go TestSettingsAppearanceSwitchesReachTheEditor and TestSettingsBlockCaretReachesTheEditor; the caret blinking period, smooth caret blinking/movement, hard wrap guides, method separators, intention bulb, rainbow indent guides and the leading/inner/trailing whitespace children are not ported |
| `editor.general.autoImport` | Auto Import |  | `src/features/settings/pages/editor/general.tsx` | missing |
| `editor.general.breadcrumbs` | Breadcrumbs |  | `src/features/settings/pages/editor/general.tsx` | partial - internal/app/settings_pages.go settingsBreadcrumbsBody: Show breadcrumbs checkbox and Top/Bottom placement radios over Preferences.Breadcrumbs feeding the editor's breadcrumb row in internal/app/view.go, staged through settingsDraft and proven by settings_test.go TestSettingsBreadcrumbsStagePlacement; the Show breadcrumbs for languages chooser is not ported |
| `editor.general.codeCompletion` | Code Completion |  | `src/features/settings/pages/editor/general.tsx` | missing |
| `editor.general.codeFolding` | Code Folding |  | `src/features/settings/pages/editor/general.tsx` | partial - internal/app/settings_pages.go settingsCodeFoldingBody: Show code folding outline checkbox over Preferences.Editor.NoFolding feeding the MyGo editor fold markers, staged through settingsDraft and proven by settings_test.go TestSettingsCodeFoldingStagesAndApplies; the Fold by default file header, imports and doc comment types are not ported |
| `editor.general.editorTabs` | Editor Tabs |  | `src/features/settings/pages/editor/general.tsx` | partial - internal/app/settings_pages.go settingsEditorTabsBody: Tab placement Top/Bottom/Left/Right/None, Show tabs in (one row scrolling the strip sideways, squeezing tabs down to their fixed parts, or wrapping into multiple rows) over Preferences.Tabs.MultipleRows/SqueezeTabs, Show file icon, Show pinned tabs in a separate row, Show file extension, Show directory for non-unique file names, Mark modified (*), Show full path on mouse hover, Close button position Right/Left/None, Sort tabs alphabetically, Open new tabs at the end, Opening Policy's Enable preview tab, Tab limit 1-100 and When the current tab is closed, activate left/right/mru over Preferences.Tabs feeding the workbench tab strip (internal/app/workbench.go tabStrip/tabButtons: a ScrollHorizontal row with a tracked scroll state, a squeeze row whose labels ellipsize to the tab's fixed parts, and a wrapped row; left or right turns it into a 140-260 wide vertical column of full-width 24px scrolling tabs beside the editor with the divider on the editor side and the pinned group in its own column, internal/app/workbench.go verticalTabs; Show tabs in is disabled and its label dimmed for left, right and none; the active tab is OpenIDEA's tinted box with a 1px selected border; activating a tab scrolls it into view; a scrolling strip whose tabs run past its width shows OpenIDEA's chevron at its end, whose Show Hidden Tabs menu lists every tab in the strip with the active one highlighted over tabOverflow/tabMenuRows, internal/app/workbench.go), insertion/sort/limit and the close activation policy (internal/app/tabs.go nextActiveAfterClose, internal/app/app.go removeTab), staged through settingsDraft and proven by settings_test.go TestSettingsEditorTabsStageAndApply, TestSettingsTabOverflowAndFileIconStageAndApply and tabs_test.go TestTabLabelFollowsTheStripSettings, TestInsertTabAfterTheActiveOneOrAtTheEnd, TestSortTabsByTitlePinsFirstAndKeepsTheActiveTab, TestTabLimitCandidatesKeepPinnedActiveAndUnsaved, TestNextActiveAfterCloseFollowsThePolicy, TestTabActivationDefaultsToTheLeft, TestPreviewTabPromotesAndIsNotPinned, TestTabMenuRowsNumberTheStripAndMarkTheActiveTab, TestTabOrderSplicesTheLiftedTabIntoItsDrop, TestReorderTabsMovesTheTabAndFollowsTheActiveOne, TestTabDropIndexTakesTheRowAndTheSlotMidpoint; a tab dragged along its row lifts out of the strip after a 4px threshold, leaves its hover-tinted slot where it would drop while the other tabs make room, and a translucent ghost of it follows the pointer until letting go reorders the strip, over tabDrag, tabDragPress, tabOrder, reorderTabs, tabDropIndex and tabGhost in internal/app/workbench.go, following OpenIDEA's previewOrder splice and dropIndex row/midpoint rule; the tab context menu adds Close with Cmd+W, Keep Tab Open for a preview tab, Open In (Project View, the file manager and a terminal at the file's folder), Local History (Show History and Put Label), Copy Path/Reference (absolute, file name and, inside the project, the path from the content root) and Rename File for a file tab, then Configure Editor Tabs, over tabMenu and tabOpenInMenu in internal/app/workbench.go with tabPathFlavours in internal/app/editor_menu.go, proven by editor_menu_test.go TestTabPathFlavoursCopyInsideAndOutsideTheProject; the file icon is one generic glyph where OpenIDEA draws per-file-type icons, the chevron reads the strip's overflow from the last frame's scroll extent so it can appear a frame after the tabs grow, dragging the vertical strip and dragging a tab out of the strip into a new window group are not ported, the tab menu's Git group is ported for Commit File…, Add, Annotate, Show Diff, Show History, Rollback… and Branches… (its branch and revision compares, push/pull/fetch, merge/rebase, new branch/tag, reset, stash and GitHub entries are not) and Open in New Window is not ported (its Compare With…, Compare with Clipboard, Local History and split groups are, over internal/app/diff.go and internal/app/editor_split.go), its split entries run through internal/app/editor_split.go and Window › Editor Tabs (the two-group subset over shared documents, proven by editor_groups_test.go), and the closing policy radios (GoIDEA never closes unsaved tabs automatically, so both policies pick the same tabs) are not ported |
| `editor.general.gutterIcons` | Gutter Icons |  | `src/features/settings/pages/editor/general.tsx` | missing |
| `editor.general.inlineCompletion` | Inline Completion |  | `src/features/settings/pages/editor/general.tsx` | missing |
| `editor.general.postfixCompletion` | Postfix Completion | Editor | `src/features/templates/index.ts` | missing |
| `editor.general.richTextCopy` | Rich-Text Copy |  | `src/features/settings/pages/editor/general.tsx` | missing |
| `editor.general.smartKeys` | Smart Keys |  | `src/features/settings/pages/editor/general.tsx` | missing |
| `editor.general.stickyLines` | Sticky Lines |  | `src/features/settings/pages/editor/general.tsx` | missing |
| `editor.inlayHints` | Inlay Hints |  | `src/features/settings/pages/editor/general.tsx` | missing |
| `editor.inspections` | Inspections | Editor | `src/features/settings/pages/editor/inspections.tsx` | missing |
| `editor.liveTemplates` | Live Templates | Editor | `src/features/templates/index.ts` | missing |
| `editor.naturalLanguages` | Natural Languages | Editor | `src/features/settings/pages/editor/spelling.tsx` | missing |
| `editor.naturalLanguages.spelling` | Spelling | Editor | `src/features/settings/pages/editor/spelling.tsx` | missing |
| `editor.todo` | TODO | Editor | `src/features/todo/index.ts` | integrated - internal/app/settings_pages.go settingsTodoBody: Case sensitive and Only in comments checkboxes, the Patterns list with each row's enable checkbox, keyword field (whitespace stripped) and Blue/Red/Yellow/Green/Violet/Grey colour select, Add Pattern (NOTE in green), Remove Pattern and Restore Defaults, staged through settingsDraft.todoPrefs into Preferences.Todo and applied to the tool window by internal/app/todo.go applyTodoSettings (rescans); base.todo.settings; internal/app/todo_test.go TestTodoSettingsDefaults, TestPreferencesTodoRoundTrip, TestSettingsTodoPageStagesAndApplies |
| `editor.vim` | Vim | Editor | `src/features/vim/index.ts` | missing |
| `general` | System Settings | Appearance & Behavior | `src/features/settings/pages/behavior.tsx` | partial - internal/app/settings_window.go registers OpenIDEA's System Settings parent and internal/app/settings_pages.go settingsSystemBody exposes Reopen projects on startup over Preferences.Startup, the local data directory and Show in Finder; settings_test.go TestSettingsSystemReopenProjectStagesAndApplies and customize_test.go TestStartupIsWelcomeUnlessReopenChosen cover the staged preference and startup behavior; the remaining Exit, Desktop, Caches and updates choices are not ported |
| `general.dateFormats` | Date Formats |  | `src/features/settings/pages/behavior.tsx` | integrated - internal/app/settings_pages.go settingsDateFormatsBody mirrors behavior.tsx DateFormatRows: Override system date and time format, the date pattern, Use 24-hour time format, Use pretty formatting and a live preview over Preferences.DateFormat; internal/app/dateformat.go follows Intl.DateTimeFormat with MyGo's system region and CLDR formats, and shares the result with Git Log, Local History, Code Vision and Git blame; dateformat_test.go TestSystemDateFormatMatchesIntlRegionalPatterns and TestFormatAppTimestampPrettyAndClock plus settings_test.go TestSettingsDateFormatsStageApplyAndReset prove the page and persistence |
| `general.httpProxy` | HTTP Proxy |  | `src/features/settings/pages/httpProxy.tsx` | missing |
| `general.language` | Language and Region |  | `src/features/settings/pages/behavior.tsx` | missing |
| `general.updates` | Updates |  | `src/features/settings/pages/updates.tsx` | partial - internal/app/settings_pages.go opens Settings › Updates with version, last checked, Check Now, automatic checks and What's New; update source setting is absent |
| `keymap` | Keymap |  | `src/features/settings/pages/builtins.tsx` | integrated - internal/app/settings_pages.go settingsKeymapBody lists every registry action and its bindings; internal/app/settings_keymap.go adds the native Set Keyboard Shortcut recorder, normalized chord capture, conflict warnings, cancel and staged Save, while Remove restores defaults and Settings Apply persists overrides; action_test.go TestKeymapDefaultsAndOverrides and TestKeymapConflicts plus TestSettingsKeymapRecorderStagesAndPersistsABinding cover the flow |
| `languages.go` | Go | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `languages.go.buildTags` | Build Tags | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `languages.go.modules` | Go Modules | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `languages.java` | Java | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | partial - internal/app/settings_ui.go javaToolsSection lists the real JDT LS/java-debug state with install and cancel; JDK selection is not ported |
| `languages.markdown` | Markdown | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `languages.node` | Node.js | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `languages.oxfmt` | oxfmt | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `languages.php` | PHP | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `languages.prettier` | Prettier | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `languages.ruby` | Ruby | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `languages.rust` | Rust | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `languages.typescript` | TypeScript | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `notifications` | Notifications | Appearance & Behavior | `src/features/settings/pages/behavior.tsx` | missing |
| `projectTrust` | Project Trust | Security | `src/features/project-trust/index.tsx` | integrated - internal/app/settings_window.go registers Project Trust under Security and internal/app/project_trust_settings.go renders trusted projects and locations plus Revoke Trust, Remove and Add folder…; internal/app/project_trust_settings_test.go and the settings-project-trust screenshot |
| `python` | Python | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `python.docstrings` | Docstrings | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `python.interpreter` | Dependencies | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `python.interpreters` | All Interpreters | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `python.ruff` | Ruff | Languages & Frameworks | `src/features/settings/pages/languages.tsx` | missing |
| `tools.actionsOnSave` | Actions on Save | Tools | `src/features/settings/pages/tools.tsx` | missing |
| `tools.externalTools` | External Tools | Tools | `src/features/settings/pages/tools.tsx` | missing |
| `tools.sshConfigurations` | SSH Configurations | Tools | `src/features/settings/pages/ssh.tsx` | missing |
| `tools.terminal` | Terminal | Tools | `src/features/settings/pages/tools.tsx` | missing |
| `vcs` | Version Control |  | `src/features/settings/pages/vcs.tsx` | missing |
| `vcs.commit` | Commit |  | `src/features/settings/pages/vcs.tsx` | missing |
| `vcs.confirmation` | Confirmation |  | `src/features/settings/pages/vcs.tsx` | missing |
| `vcs.git` | Git |  | `src/features/settings/pages/vcs.tsx` | missing |
| `vcs.github` | GitHub |  | `src/features/github/settingsPages.ts` | missing |
| `vcs.issueNavigation` | Issue Navigation |  | `src/features/settings/pages/vcs.tsx` | integrated - internal/app/settings_window.go registers Version Control › Issue Navigation and internal/app/settings_issue_navigation.go renders OpenIDEA's Issue ID/Issue Link table, Add, Remove, Edit, Enter to commit, Escape to cancel, invalid-regex feedback and JIRA, YouTrack and GitHub presets over Preferences.VCS.IssueNavigation; internal/app/issue_links.go links matching issue ids in Git Log, commit details and blame, while issue_links_test.go and settings_test.go TestSettingsIssueNavigationStagesAppliesAndEdits and TestIssueNavigationEditorCommitsWithEnterAndCancelsWithEscape prove the flow |
| `vcs.log` | Log |  | `src/features/vcs/index.ts` | missing |
| `vcs.mappings` | Directory Mappings |  | `src/features/settings/pages/vcs.tsx` | missing |
| `vcs.shelf` | Shelf |  | `src/features/settings/pages/vcs.tsx` | missing |

### Keys

Every fixed key reachable from the engine's `Settings` type (`crates/ide-core`),
flattened to dotted camelCase paths, plus keys an option on a page edits.
Collection keys (`Vec`, maps) are one item: their entries are data, not fixed
settings.

| Key | Edited on | Status |
| --- | --- | --- |
| `actionsOnSave` |  | missing |
| `actionsOnSave.cleanup` |  | missing |
| `actionsOnSave.optimizeImports` |  | missing |
| `agentSecurity` |  | missing |
| `agentSecurity.allowedCommands` |  | missing |
| `agentSecurity.blockedCommands` |  | missing |
| `agents` |  | missing |
| `agents.defaults` |  | missing |
| `agents.expandToolOutput` | agents.chat | missing |
| `agents.foldTurns` | agents.chat | missing |
| `agents.mcpServers` |  | missing |
| `agents.newTaskApprovals` | agents.chat | missing |
| `agents.sendWith` | agents.chat | missing |
| `agents.showContextUsage` | agents.chat | missing |
| `agents.showCost` | agents.chat | missing |
| `agents.showUsageLimits` | agents.chat | missing |
| `agents.showWorkTime` | agents.chat | missing |
| `agents.smoothText` | agents.chat | missing |
| `agents.thinking` | agents.chat | missing |
| `agents.toolCalls` | agents.chat | missing |
| `agents.whileRunning` | agents.chat | missing |
| `appearance` |  | missing |
| `appearance.accent` |  | missing |
| `appearance.compactMode` |  | missing |
| `appearance.contrastScrollbars` | appearance | missing |
| `appearance.differentToolWindowBackground` |  | missing |
| `appearance.dndWithAltOnly` |  | missing |
| `appearance.editorAntialiasing` | appearance | missing |
| `appearance.fileIconTheme` |  | missing |
| `appearance.fileIconThemeOptions` |  | missing |
| `appearance.fullPathInTitle` |  | missing |
| `appearance.ideAntialiasing` | appearance | missing |
| `appearance.locale` | general.language | missing |
| `appearance.mainMenuAlt` |  | missing |
| `appearance.mainMenuStyle` |  | missing |
| `appearance.mainToolbar.hidden` |  | missing |
| `appearance.mainToolbar.left` |  | missing |
| `appearance.mainToolbar.right` |  | missing |
| `appearance.menuIcons` |  | missing |
| `appearance.navigationBar` | appearance | missing |
| `appearance.preferredDarkTheme` |  | missing |
| `appearance.preferredLightTheme` |  | missing |
| `appearance.projectColorsInToolbar` |  | missing |
| `appearance.redGreenVision` | appearance | missing |
| `appearance.showMainToolbar` | appearance | missing |
| `appearance.showStatusBar` | appearance | missing |
| `appearance.showToolWindowBadges` |  | missing |
| `appearance.showToolWindowBars` |  | missing |
| `appearance.showToolWindowNames` |  | missing |
| `appearance.smallTreeIndents` |  | missing |
| `appearance.statusBarWidgets` |  | missing |
| `appearance.syncWithOs` |  | missing |
| `appearance.theme` |  | missing |
| `appearance.uiFontFallback` |  | missing |
| `appearance.uiFontFamily` |  | missing |
| `appearance.uiFontSize` |  | missing |
| `appearance.useCustomFont` |  | missing |
| `appearance.widescreenToolWindows` |  | missing |
| `appearance.zoom` |  | missing |
| `cmake` |  | missing |
| `cmake.profiles` |  | missing |
| `console` |  | missing |
| `console.bufferKb` |  | missing |
| `console.encoding` | build.console | missing |
| `console.foldPatterns` |  | missing |
| `console.logColors.debug` |  | missing |
| `console.logColors.enabled` |  | missing |
| `console.logColors.error` |  | missing |
| `console.logColors.info` |  | missing |
| `console.logColors.trace` |  | missing |
| `console.logColors.warn` |  | missing |
| `console.overrideBuffer` |  | missing |
| `console.softWrap` | build.console | missing |
| `customKeymaps` |  | missing |
| `debugger` |  | missing |
| `debugger.breakpointsAnyFile` | build.debugger | missing |
| `debugger.focusAppOnBreakpoint` | build.debugger | missing |
| `debugger.hideOnTermination` | build.debugger | missing |
| `debugger.inlineValues` | build.debugger.dataViews | missing |
| `debugger.python.attachSubprocess` | build.pythonDebugger | missing |
| `debugger.python.gevent` | build.pythonDebugger | missing |
| `debugger.runToCursorInlay` | build.debugger | missing |
| `debugger.showOnBreakpoint` | build.debugger | missing |
| `debugger.skipLibraryCode` | build.debugger.stepping | missing |
| `debugger.sortValues` | build.debugger.dataViews | missing |
| `debugger.valueTooltip` | build.debugger.dataViews | missing |
| `debugger.valueTooltipDelay` |  | missing |
| `debugger.valueTooltipOnSelection` | build.debugger.dataViews | missing |
| `deployment` |  | missing |
| `deployment.autoUpload` | build.deployment.options | missing |
| `deployment.defaultServer` |  | missing |
| `deployment.deleteRemote` | build.deployment.options | missing |
| `deployment.exclude` |  | missing |
| `deployment.preserveTimestamps` | build.deployment.options | missing |
| `deployment.servers` |  | missing |
| `editor` |  | missing |
| `editor.appearance.blockCaret` |  | missing |
| `editor.appearance.caretBlinkPeriod` |  | missing |
| `editor.appearance.caretBlinking` |  | missing |
| `editor.appearance.caretMovementStyle` |  | missing |
| `editor.appearance.distractionFreeWidth` |  | missing |
| `editor.appearance.hardWrapGuides` |  | missing |
| `editor.appearance.indentGuides` |  | missing |
| `editor.appearance.intentionBulb` |  | missing |
| `editor.appearance.lineNumberMode` |  | missing |
| `editor.appearance.lineNumbers` |  | missing |
| `editor.appearance.methodSeparators` |  | missing |
| `editor.appearance.rainbowIndentGuides` |  | missing |
| `editor.appearance.renderDocComments` |  | missing |
| `editor.appearance.smoothCaretBlinking` |  | missing |
| `editor.appearance.smoothCaretMovement` |  | missing |
| `editor.appearance.whitespaceInner` |  | missing |
| `editor.appearance.whitespaceLeading` |  | missing |
| `editor.appearance.whitespaceTrailing` |  | missing |
| `editor.autoImport.addUnambiguous` |  | missing |
| `editor.autoImport.importsOnPaste` |  | missing |
| `editor.autoImport.optimizeOnTheFly` |  | missing |
| `editor.boldFontWeight` |  | missing |
| `editor.breadcrumbs.disabledLanguages` |  | missing |
| `editor.breadcrumbs.placement` |  | missing |
| `editor.breadcrumbs.show` |  | missing |
| `editor.codeEditing.highlightBraces` |  | missing |
| `editor.codeEditing.highlightSelection` |  | missing |
| `editor.codeEditing.highlightUsages` |  | missing |
| `editor.codeEditing.nextErrorErrorsOnly` |  | missing |
| `editor.codeEditing.quickDocDelay` |  | missing |
| `editor.codeEditing.quickDocOnHover` |  | missing |
| `editor.codeEditing.stickyLinesMax` |  | missing |
| `editor.codeStyle.detectIndents` |  | missing |
| `editor.codeStyle.doNotFormat` |  | missing |
| `editor.codeStyle.editorconfig` |  | missing |
| `editor.codeStyle.formatterOffTag` |  | missing |
| `editor.codeStyle.formatterOnTag` |  | missing |
| `editor.codeStyle.formatterTagsEnabled` |  | missing |
| `editor.codeStyle.formatterTagsRegexp` |  | missing |
| `editor.codeStyle.hardWrapAt` |  | missing |
| `editor.codeStyle.languages` |  | missing |
| `editor.codeStyle.lineSeparator` |  | missing |
| `editor.codeStyle.visualGuides` |  | missing |
| `editor.colorOverrides` |  | missing |
| `editor.colorScheme` |  | missing |
| `editor.completion.autoInsertSingle` |  | missing |
| `editor.completion.autoPopup` |  | missing |
| `editor.completion.caseSensitivity` |  | missing |
| `editor.completion.documentationDelay` |  | missing |
| `editor.completion.insertOnSpecialKeys` |  | missing |
| `editor.completion.insertParentheses` |  | missing |
| `editor.completion.matchCase` |  | missing |
| `editor.completion.parameterInfo` |  | missing |
| `editor.completion.parameterInfoDelay` |  | missing |
| `editor.completion.showDocumentation` |  | missing |
| `editor.completion.sortAlphabetically` |  | missing |
| `editor.customSchemes` |  | missing |
| `editor.docstringFormat` |  | missing |
| `editor.encodings.global` | editor.fileEncodings | missing |
| `editor.encodings.nativeToAscii` | editor.fileEncodings | missing |
| `editor.encodings.project` | editor.fileEncodings | missing |
| `editor.encodings.properties` | editor.fileEncodings | missing |
| `editor.encodings.utf8Bom` | editor.fileEncodings | missing |
| `editor.fallbackFont` |  | missing |
| `editor.folding.docComments` |  | missing |
| `editor.folding.fileHeader` |  | missing |
| `editor.folding.imports` |  | missing |
| `editor.folding.showOutline` |  | missing |
| `editor.fontFamily` |  | missing |
| `editor.fontSize` |  | missing |
| `editor.fontWeight` |  | missing |
| `editor.formatOnSave` |  | missing |
| `editor.general.caretScroll` |  | missing |
| `editor.general.dragAndDrop` |  | missing |
| `editor.general.keepTrailingOnCaretLine` |  | missing |
| `editor.general.markdownImageFolder` |  | missing |
| `editor.general.markdownImageNamePattern` |  | missing |
| `editor.general.removeTrailingBlankLines` |  | missing |
| `editor.general.richTextCopy` |  | missing |
| `editor.general.scrollPastEnd` |  | missing |
| `editor.general.softWrapFiles` |  | missing |
| `editor.general.softWrapIndent` |  | missing |
| `editor.general.stripTrailingScope` |  | missing |
| `editor.general.wheelZoom` |  | missing |
| `editor.gutterIconsDisabled` |  | missing |
| `editor.inlay.codeVisionAuthor` |  | missing |
| `editor.inlay.codeVisionAuthorPosition` |  | missing |
| `editor.inlay.codeVisionNoUsages` |  | missing |
| `editor.inlay.codeVisionUsages` |  | missing |
| `editor.inlay.codeVisionUsagesPosition` |  | missing |
| `editor.inlay.disabledLanguages` |  | missing |
| `editor.inlay.other` |  | missing |
| `editor.inlay.parameters` |  | missing |
| `editor.inlay.types` |  | missing |
| `editor.inlayHints` |  | missing |
| `editor.inlineCompletion.disabledLanguages` |  | missing |
| `editor.inlineCompletion.enabled` |  | missing |
| `editor.inlineCompletion.onlyOnPause` |  | missing |
| `editor.inlineCompletion.pauseMs` |  | missing |
| `editor.insertSpaces` | editor.codeStyle.other | missing |
| `editor.ligatures` | editor.font | missing |
| `editor.lineHeight` |  | missing |
| `editor.rainbowBrackets` | editor.colorScheme.language-defaults | missing |
| `editor.semanticHighlighting` | editor.colorScheme.language-defaults | missing |
| `editor.showGutterIcons` |  | missing |
| `editor.showWhitespace` |  | missing |
| `editor.smartKeys.backspaceUnindent` |  | missing |
| `editor.smartKeys.camelHumps` |  | missing |
| `editor.smartKeys.escapeOnPaste` |  | missing |
| `editor.smartKeys.insertDocStub` |  | missing |
| `editor.smartKeys.pairBrackets` |  | missing |
| `editor.smartKeys.pairQuotes` |  | missing |
| `editor.smartKeys.reformatOnPaste` |  | missing |
| `editor.smartKeys.smartHome` |  | missing |
| `editor.smartKeys.smartIndent` |  | missing |
| `editor.smartKeys.surroundSelection` |  | missing |
| `editor.smartKeys.tabOutOfPair` |  | missing |
| `editor.softWrap` |  | missing |
| `editor.stickyScroll` |  | missing |
| `editor.tabSize` | editor.codeStyle.other | missing |
| `editor.tabs.activateOnClose` |  | missing |
| `editor.tabs.closeButtonPosition` |  | missing |
| `editor.tabs.closingPolicy` |  | missing |
| `editor.tabs.markModified` |  | missing |
| `editor.tabs.multipleRows` |  | missing |
| `editor.tabs.oneRowOverflow` |  | missing |
| `editor.tabs.openAtEnd` |  | missing |
| `editor.tabs.pinnedSeparateRow` |  | missing |
| `editor.tabs.placement` |  | missing |
| `editor.tabs.previewTab` |  | partial - internal/app/app.go openPreview and openFileWith with internal/app/tabs.go previewIndex, replacePreviewTab, dropPreviewTabs and promoteTab: a click in a results list opens one italic preview tab that the next such click replaces in place, an edit (internal/app/app.go syncPump), a pin (internal/app/tabs.go setTabPinned) or an explicit open promotes it, a regular open closes it, and Preferences.Tabs.NoPreviewTab turns it off from Editor > Editor Tabs Opening Policy (internal/app/settings_pages.go, internal/app/settings_window.go); the entry points are Problems rows and file headers (internal/app/problems.go), Find hits (internal/app/view.go) and Find Usages rows (internal/app/usages.go), and a double-click opens the regular tab; proven by tabs_test.go TestPreviewTabPromotesAndIsNotPinned and settings_test.go TestSettingsEditorTabsStageAndApply; the saved session does not carry which tab was a preview, so after a restart it opens as a regular tab |
| `editor.tabs.showCloseButton` |  | missing |
| `editor.tabs.showDirectoryForNonUnique` |  | missing |
| `editor.tabs.showExtension` |  | missing |
| `editor.tabs.showFileIcon` |  | missing |
| `editor.tabs.showFullPathOnHover` |  | missing |
| `editor.tabs.sortAlphabetically` |  | missing |
| `editor.tabs.tabLimit` |  | missing |
| `externalTools` |  | missing |
| `files` |  | missing |
| `files.associatedApplications` |  | missing |
| `files.autoSave` | general | missing |
| `files.composeFileOverrides` |  | missing |
| `files.ensureFinalNewline` | editor.general | missing |
| `files.exclude` |  | missing |
| `files.fileTypeOverrides` |  | missing |
| `files.fileTypes` |  | missing |
| `files.ignoredNames` |  | missing |
| `files.localHistoryDays` | general | missing |
| `files.localHistoryMaxMb` | general | missing |
| `files.syncOnFocus` | general | missing |
| `files.trimTrailingWhitespace` | editor.general | missing |
| `general` |  | missing |
| `general.autoCheckUpdates` | general.updates | missing |
| `general.closeTabsWithWindow` | general | missing |
| `general.closeWithoutWelcome` | general | missing |
| `general.confirmExit` | general | missing |
| `general.dateFormat.overrideSystem` |  | integrated - internal/app/settings_pages.go settingsDateFormatsBody toggles Preferences.DateFormat.OverrideSystem; internal/app/dateformat.go switches between the custom pattern and MyGo's system region; dateformat_test.go and settings_test.go TestSettingsDateFormatsStageApplyAndReset |
| `general.dateFormat.pattern` |  | integrated - internal/app/settings_pages.go settingsDateFormatsBody edits Preferences.DateFormat.Pattern with OpenIDEA's yyyy, yy, MMMM, MMM, MM, M, dd and d placeholders; internal/app/dateformat.go formatDatePattern and TestDateFormatPatternUsesOpenIDEAPlaceholders |
| `general.dateFormat.pretty` |  | integrated - internal/app/settings_pages.go settingsDateFormatsBody toggles Preferences.DateFormat.Pretty; internal/app/dateformat.go formatAppTimestamp supplies Moments ago, N min ago, Today and Yesterday; TestFormatAppTimestampPrettyAndClock and TestSettingsDateFormatsStageApplyAndReset |
| `general.dateFormat.use24Hour` |  | integrated - internal/app/settings_pages.go settingsDateFormatsBody toggles Preferences.DateFormat.Use24Hour; internal/app/dateformat.go formatAppClock writes 12- or 24-hour times and formatSystemClock follows the system hour cycle without an override; TestFormatAppTimestampPrettyAndClock and TestSystemDateFormatMatchesIntlRegionalPatterns |
| `general.defaultProjectDirectory` |  | missing |
| `general.idleSeconds` |  | missing |
| `general.lastRunVersion` |  | missing |
| `general.openProjectIn` |  | missing |
| `general.powerSaveMode` |  | missing |
| `general.projectName` |  | missing |
| `general.reopenLastProject` | general | integrated - internal/app/settings_pages.go settingsSystemBody stages Reopen projects on startup; internal/app/settings_window.go persists Preferences.Startup and customize_test.go TestStartupIsWelcomeUnlessReopenChosen verifies the effective startup choice; settings_test.go TestSettingsSystemReopenProjectStagesAndApplies covers Apply and Reset |
| `general.saveOnIdle` |  | missing |
| `general.showWhatsNewAfterUpdate` | general.updates | missing |
| `general.updateSource` | general.updates | missing |
| `inspections` |  | missing |
| `inspections.default` |  | missing |
| `inspections.ideProfiles` |  | missing |
| `inspections.profile` |  | missing |
| `inspections.project` |  | missing |
| `inspections.projectProfiles` |  | missing |
| `keymap` |  | missing |
| `keymapOverrides` |  | missing |
| `languageServers` |  | missing |
| `languages` |  | missing |
| `languages.arduino.clangd` |  | missing |
| `languages.arduino.cli` |  | missing |
| `languages.arduino.cliConfig` |  | missing |
| `languages.arduino.fqbn` |  | missing |
| `languages.arduino.languageServer` |  | missing |
| `languages.go.buildTags.arch` |  | missing |
| `languages.go.buildTags.cgo` |  | missing |
| `languages.go.buildTags.compiler` |  | missing |
| `languages.go.buildTags.custom` |  | missing |
| `languages.go.buildTags.experiments` |  | missing |
| `languages.go.buildTags.os` |  | missing |
| `languages.go.buildTags.version` |  | missing |
| `languages.go.gopath` |  | missing |
| `languages.go.goroot` |  | missing |
| `languages.go.modules.autoVendor` |  | missing |
| `languages.go.modules.env` |  | missing |
| `languages.java.gradle.distribution` |  | missing |
| `languages.java.gradle.home` |  | missing |
| `languages.java.gradle.jvm` |  | missing |
| `languages.java.gradle.linkedProjects` |  | missing |
| `languages.java.gradle.runUsing` |  | missing |
| `languages.java.gradle.userHome` |  | missing |
| `languages.java.jdk` |  | missing |
| `languages.java.jdtlsJdk` |  | missing |
| `languages.java.jdtlsShared` |  | missing |
| `languages.java.jdtlsVmArgs` |  | missing |
| `languages.java.languageLevel` |  | missing |
| `languages.java.maven.activeProfiles` |  | missing |
| `languages.java.maven.daemon` |  | missing |
| `languages.java.maven.delegateBuild` |  | missing |
| `languages.java.maven.home` |  | missing |
| `languages.java.maven.linkedProjects` |  | missing |
| `languages.java.maven.localRepository` |  | missing |
| `languages.java.maven.mvndHome` |  | missing |
| `languages.java.maven.runnerJre` |  | missing |
| `languages.java.maven.threads` |  | missing |
| `languages.java.maven.toolchains` |  | missing |
| `languages.java.maven.unlinkedProjects` |  | missing |
| `languages.java.maven.userSettings` |  | missing |
| `languages.markdown.previewFontSize` |  | missing |
| `languages.node.interpreter` |  | missing |
| `languages.node.packageManager` |  | missing |
| `languages.node.runTarget` |  | missing |
| `languages.oxfmt.configurationDirectory` |  | missing |
| `languages.oxfmt.languages` |  | missing |
| `languages.oxfmt.path` |  | missing |
| `languages.oxfmt.prefer` |  | missing |
| `languages.oxfmt.runOnSave` |  | missing |
| `languages.php.includePaths` |  | missing |
| `languages.php.interpreter` |  | missing |
| `languages.php.languageLevel` |  | missing |
| `languages.php.phpstanConfig` |  | missing |
| `languages.php.runTarget` |  | missing |
| `languages.php.stubs` |  | missing |
| `languages.pluginFormatters` |  | missing |
| `languages.prettier.configurationDirectory` |  | missing |
| `languages.prettier.languages` |  | missing |
| `languages.prettier.path` |  | missing |
| `languages.prettier.prefer` |  | missing |
| `languages.prettier.runOnSave` |  | missing |
| `languages.python.activateVirtualenv` |  | missing |
| `languages.python.hiddenInterpreters` |  | missing |
| `languages.python.hideOtherProjectInterpreters` |  | missing |
| `languages.python.interpreter` |  | missing |
| `languages.python.interpreters` |  | missing |
| `languages.python.showInterpreterPaths` |  | missing |
| `languages.ruby.interpreter` |  | missing |
| `languages.ruff.configurationDirectory` |  | missing |
| `languages.ruff.languages` |  | missing |
| `languages.ruff.path` |  | missing |
| `languages.ruff.prefer` |  | missing |
| `languages.ruff.runOnSave` |  | missing |
| `languages.rust.checkCommand` |  | missing |
| `languages.rust.checkOnSave` |  | missing |
| `languages.rust.rustfmtArgs` |  | missing |
| `languages.rust.stdlib` |  | missing |
| `languages.rust.toolchain` |  | missing |
| `languages.typescript.version` |  | missing |
| `mcpServer` |  | missing |
| `mcpServer.agentArchiveLimit` | agents.mcpServer | missing |
| `mcpServer.agentTabLimit` | agents.mcpServer | missing |
| `mcpServer.enabled` | agents.mcpServer | missing |
| `mcpServer.runWithoutConfirmation` | agents.mcpServer | missing |
| `notifications` |  | missing |
| `notifications.balloons` | notifications | missing |
| `notifications.groups` |  | missing |
| `notifications.sound` | notifications | missing |
| `plugins` |  | missing |
| `projectStructure` |  | missing |
| `projectStructure.artifacts` |  | missing |
| `projectStructure.compilerOutput` |  | missing |
| `projectStructure.contentRoots` |  | missing |
| `projectStructure.dependencies` |  | missing |
| `projectStructure.excludeFiles` |  | missing |
| `projectStructure.excluded` |  | missing |
| `projectStructure.facets` |  | missing |
| `projectStructure.libraries` |  | missing |
| `projectStructure.memberDependencies` |  | missing |
| `projectStructure.modules` |  | missing |
| `projectStructure.resourceRoots` |  | missing |
| `projectStructure.sourceRoots` |  | missing |
| `projectStructure.testResourceRoots` |  | missing |
| `projectStructure.testRoots` |  | missing |
| `projectStructure.unmarked` |  | missing |
| `proxy` |  | missing |
| `proxy.authenticate` |  | missing |
| `proxy.host` |  | missing |
| `proxy.kind` |  | missing |
| `proxy.login` |  | missing |
| `proxy.mode` |  | missing |
| `proxy.noProxy` |  | missing |
| `proxy.password` |  | missing |
| `proxy.port` |  | missing |
| `run` |  | missing |
| `run.confirmRerun` | build.run | missing |
| `run.confirmTerminate` | build.run | missing |
| `run.defaultTarget` |  | missing |
| `run.showOnStart` | build.run | missing |
| `run.terminationTimeoutMs` | build.run | missing |
| `runTargets` |  | missing |
| `spelling` |  | missing |
| `spelling.customDictionaries` |  | missing |
| `spelling.ideWords` |  | missing |
| `spelling.projectWords` |  | missing |
| `ssh` |  | missing |
| `ssh.configs` |  | missing |
| `terminal` |  | missing |
| `terminal.audibleBell` |  | missing |
| `terminal.closeOnExit` |  | missing |
| `terminal.columnWidth` |  | missing |
| `terminal.copyOnSelection` |  | missing |
| `terminal.cursorBlink` |  | missing |
| `terminal.cursorShape` |  | missing |
| `terminal.env` |  | missing |
| `terminal.escapeToEditor` |  | missing |
| `terminal.fallbackFontFamily` |  | missing |
| `terminal.fontFamily` |  | missing |
| `terminal.fontSize` |  | missing |
| `terminal.highlightHyperlinks` |  | missing |
| `terminal.highlightSelection` |  | missing |
| `terminal.lineHeight` |  | missing |
| `terminal.mouseReporting` |  | missing |
| `terminal.optionAsMeta` |  | missing |
| `terminal.overrideIdeShortcuts` |  | missing |
| `terminal.pasteOnMiddleClick` |  | missing |
| `terminal.shellIntegration` |  | missing |
| `terminal.shellPath` |  | missing |
| `terminal.startDirectory` |  | missing |
| `terminal.tabName` | tools.terminal | missing |
| `terminal.useEditorFont` |  | missing |
| `tools` |  | missing |
| `vcs` |  | missing |
| `vcs.commit.analyzeCode` |  | missing |
| `vcs.commit.blankLineAfterSubject` | vcs.commit | missing |
| `vcs.commit.bodyLimit` |  | missing |
| `vcs.commit.checkTodo` |  | missing |
| `vcs.commit.cleanup` |  | missing |
| `vcs.commit.clearInitialMessage` | vcs.commit | missing |
| `vcs.commit.disabledPluginChecks` |  | missing |
| `vcs.commit.doubleClickOpensDiff` |  | missing |
| `vcs.commit.limitBody` |  | missing |
| `vcs.commit.limitSubject` |  | missing |
| `vcs.commit.nonModal` | vcs.commit | missing |
| `vcs.commit.optimizeImports` |  | missing |
| `vcs.commit.reformat` |  | missing |
| `vcs.commit.showRightMargin` |  | missing |
| `vcs.commit.subjectLimit` |  | missing |
| `vcs.commit.wrapAtMargin` |  | missing |
| `vcs.confirmation.confirmDropCommits` | vcs.confirmation | missing |
| `vcs.confirmation.onCreate` |  | missing |
| `vcs.confirmation.onDelete` |  | missing |
| `vcs.confirmation.onPatchCreated` | vcs.confirmation | missing |
| `vcs.confirmation.showUpdateOptions` | vcs.confirmation | missing |
| `vcs.git.autoUpdateOnPushRejected` | vcs.git | missing |
| `vcs.git.combineStashAndShelf` | vcs.git | missing |
| `vcs.git.executable` |  | missing |
| `vcs.git.incomingCheck` |  | missing |
| `vcs.git.inlineBlame` | vcs.git | missing |
| `vcs.git.largeFileMb` |  | missing |
| `vcs.git.protectedBranches` |  | missing |
| `vcs.git.pushDialogOnlyProtected` |  | missing |
| `vcs.git.showBranchInProjectView` | vcs.git | missing |
| `vcs.git.showPushDialog` |  | missing |
| `vcs.git.signOff` | vcs.git | missing |
| `vcs.git.updateMethod` |  | missing |
| `vcs.git.warnBadFileNames` | vcs.git | missing |
| `vcs.git.warnCrlf` | vcs.git | missing |
| `vcs.git.warnDetachedHead` | vcs.git | missing |
| `vcs.git.warnLargeFiles` |  | missing |
| `vcs.historyLimit` |  | missing |
| `vcs.issueNavigation` |  | integrated - internal/app/settings_issue_navigation.go and Preferences.VCS.IssueNavigation carry OpenIDEA's Issue ID regular expressions and Issue Link templates; internal/app/settings_window.go stages, applies, persists and resets them, while internal/app/issue_links.go applies earliest-match-wins capture expansion and HTTP(S) links in Git Log, commit details and blame; issue_links_test.go and settings_test.go TestSettingsIssueNavigationStagesAppliesAndEdits and TestIssueNavigationEditorCommitsWithEnterAndCancelsWithEscape |
| `vcs.limitHistory` |  | missing |
| `vcs.shelf.removeApplied` | vcs.shelf | missing |
| `vcs.showDirtyDirectories` | vcs.confirmation | missing |

## Languages

| Language | Id | Tree-sitter | Debugger | Status |
| --- | --- | --- | --- | --- |
| Arduino | `arduino` | `cpp` |  | missing |
| AsciiDoc | `asciidoc` |  |  | missing |
| Astro | `astro` |  |  | missing |
| Batch | `bat` |  |  | missing |
| C | `c` | `c` | yes | missing |
| Clojure | `clojure` |  |  | missing |
| CMake | `cmake` | `cmake` |  | missing |
| C++ | `cpp` | `cpp` | yes | missing |
| C# | `csharp` | `csharp` | yes | missing |
| CSS | `css` | `css` |  | missing |
| D | `d` |  |  | missing |
| Dart | `dart` | `dart` | yes | missing |
| Diff | `diff` |  |  | missing |
| Django Template | `django-html` | `html` |  | missing |
| Dockerfile | `dockerfile` |  |  | missing |
| .env | `dotenv` |  |  | missing |
| Elixir | `elixir` | `elixir` | yes | missing |
| ERB | `erb` |  |  | missing |
| Erlang | `erlang` |  |  | missing |
| F# | `fsharp` |  | yes | missing |
| GDScript | `gdscript` | `gdscript` | yes | missing |
| Godot Shader | `gdshader` |  |  | missing |
| Git Attributes | `gitattributes` |  |  | missing |
| Go | `go` | `go` | yes | missing |
| Go Module | `go.mod` |  |  | missing |
| Go Workspace | `go.work` |  |  | missing |
| Godot Resource | `godot-resource` |  |  | missing |
| GraphQL | `graphql` | `graphql` |  | missing |
| Groovy | `groovy` |  |  | missing |
| Haskell | `haskell` | `haskell` |  | missing |
| HCL | `hcl` | `hcl` |  | missing |
| HTML | `html` | `html` |  | missing |
| HTTP Request | `http` |  |  | missing |
| Ignore File | `ignore` |  |  | missing |
| INI | `ini` |  |  | missing |
| Java | `java` | `java` | yes | integrated - lsp.jdtls.ready, lsp.completion.items, lsp.hover, lsp.rename, debug.paused |
| JavaScript | `javascript` | `javascript` | yes | missing |
| JavaScript JSX | `javascriptreact` | `javascript` | yes | missing |
| JSON | `json` | `json` |  | missing |
| JSON with Comments | `jsonc` | `json` |  | missing |
| JSP | `jsp` |  |  | missing |
| Julia | `julia` |  |  | missing |
| Kotlin | `kotlin` | `kotlin` | yes | missing |
| LaTeX | `latex` |  |  | missing |
| Less | `less` |  |  | missing |
| Log | `log` |  |  | missing |
| Lua | `lua` | `lua` |  | missing |
| Makefile | `makefile` | `make` |  | missing |
| Markdown | `markdown` | `markdown` |  | missing |
| Nix | `nix` | `nix` |  | missing |
| Objective-C | `objective-c` |  | yes | missing |
| Objective-C++ | `objective-cpp` |  | yes | missing |
| OCaml | `ocaml` | `ocaml` | yes | missing |
| OCaml Interface | `ocaml.interface` | `ocaml_interface` |  | missing |
| Perl | `perl` |  |  | missing |
| PHP | `php` | `php` | yes | missing |
| Plain Text | `plaintext` |  |  | missing |
| PowerShell | `powershell` |  |  | missing |
| Prisma | `prisma` | `prisma` |  | missing |
| Properties | `properties` |  |  | missing |
| Protocol Buffers | `proto` | `proto` |  | missing |
| Python | `python` | `python` | yes | missing |
| R | `r` |  |  | missing |
| reStructuredText | `restructuredtext` |  |  | missing |
| Ruby | `ruby` | `ruby` | yes | missing |
| Rust | `rust` | `rust` | yes | missing |
| Sass | `sass` |  |  | missing |
| Scala | `scala` | `scala` |  | missing |
| SCSS | `scss` |  |  | missing |
| Shell Script | `shellscript` | `bash` | yes | missing |
| SQL | `sql` |  |  | missing |
| Svelte | `svelte` |  |  | missing |
| Swift | `swift` | `swift` | yes | missing |
| Terraform | `terraform` | `hcl` |  | missing |
| Terraform Variables | `terraform-vars` | `hcl` |  | missing |
| TOML | `toml` | `toml` |  | missing |
| TypeScript | `typescript` | `typescript` | yes | missing |
| TypeScript JSX | `typescriptreact` | `tsx` | yes | missing |
| Vue | `vue` |  |  | missing |
| XML | `xml` | `xml` |  | missing |
| YAML | `yaml` | `yaml` |  | missing |
| Zig | `zig` | `zig` | yes | missing |

## IPC commands

All 668 commands the desktop shell registers. `stdio` is whether the pinned
`openidea-server` serves the command; `GoIDEA` is whether GoIDEA calls it.
Local-only commands and the per-command argument lists are in
[OPENIDEA_IPC.md](OPENIDEA_IPC.md).

| Command | Module | stdio | GoIDEA | Status |
| --- | --- | --- | --- | --- |
| `agent_accept_changes` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_archive` | agent | yes | yes | integrated - internal/app/agent_workspace.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_base_text` | agent | yes | yes | integrated - internal/app/agent_diff.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_cancel` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_changes` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_commands` | agent | yes | yes | integrated - internal/app/agent_composer.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_create` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_custom_import` | agent | no |  | integrated - internal/app/agent_registry_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_custom_remove` | agent | no |  | integrated - internal/app/agent_registry_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_custom_save` | agent | no |  | integrated - internal/app/agent_registry_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_delete` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_entries` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_external_sessions` | agent | yes | yes | integrated - internal/app/agents.go, internal/app/agents_smoke.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_fork` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_grouped_changes` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_import_session` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_mcp_import` | agent | yes | yes | integrated - internal/app/agent_mcp_settings.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_mcp_list` | agent | yes | yes | integrated - internal/app/agent_mcp_settings.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_mcp_remove` | agent | yes | yes | integrated - internal/app/agent_mcp_settings.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_mcp_save` | agent | yes | yes | integrated - internal/app/agent_mcp_settings.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_mcp_set_enabled` | agent | yes | yes | integrated - internal/app/agent_mcp_settings.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_probe` | agent | yes |  | missing |
| `agent_prompt` | agent | yes | yes | integrated - internal/app/agents.go, internal/app/agents_view.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_providers` | agent | yes | yes | integrated - internal/app/agents.go, internal/app/agents_smoke.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_registry_install` | agent | yes | yes | integrated - internal/app/agent_registry_settings.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_registry_list` | agent | yes | yes | integrated - internal/app/agent_registry_settings.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_registry_remove` | agent | yes | yes | integrated - internal/app/agent_registry_settings.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_rename` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_respond` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_rollback` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_rollback_changes` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_send_queued_now` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_set_command` | agent | yes | yes | integrated - internal/app/agent_registry_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_set_config` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_set_env` | agent | yes | yes | integrated - internal/app/agent_registry_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_stop_background` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_subscribe` | agent | yes | yes | integrated - internal/app/agents.go, internal/app/agents_smoke.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_unarchive` | agent | yes | yes | integrated - internal/app/agent_workspace.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_unqueue` | agent | yes | yes | integrated - internal/app/agents.go invokes the OpenIDEA RPC through the native Go adapter |
| `analysis_locate_duplicates` | analysis | yes |  | missing |
| `android_devices` | android | no |  | missing |
| `android_logcat` | android | no |  | missing |
| `app_build_info` | window | no |  | missing |
| `app_cancel_quit` | window | no |  | missing |
| `app_check_update` | window | no |  | missing |
| `app_cli_install` | window | no |  | missing |
| `app_cli_uninstall` | window | no |  | missing |
| `app_info` | app | no |  | missing |
| `app_install_update` | window | no |  | missing |
| `app_log_location` | window | no |  | missing |
| `app_metrics` | perf | no |  | missing |
| `app_quit` | window | no |  | missing |
| `app_release_notes` | window | no |  | missing |
| `app_restart` | caches | no |  | missing |
| `app_update_announcement` | window | no |  | missing |
| `app_update_status` | window | no |  | missing |
| `app_update_window_ready` | window | no |  | missing |
| `apply_workspace_edit` | lsp | yes | yes | integrated - internal/app/rename.go invokes the OpenIDEA RPC through the native Go adapter |
| `build_compile_file` | build | yes | yes | integrated - internal/app/build.go invokes the OpenIDEA RPC through the native Go adapter |
| `build_model` | build | yes | yes | integrated - internal/app/maven.go invokes the OpenIDEA RPC through the native Go adapter |
| `build_run` | build | yes | yes | integrated - internal/app/build.go invokes the OpenIDEA RPC through the native Go adapter |
| `build_sync` | build | yes | yes | integrated - internal/app/maven.go invokes the OpenIDEA RPC through the native Go adapter |
| `caches_local_data_info` | caches | no |  | missing |
| `caches_schedule_invalidation` | caches | no |  | missing |
| `caches_set_local_data_dir` | caches | no |  | missing |
| `cancel_request` | app | yes | yes | integrated - internal/app/intents.go, internal/app/build.go, internal/app/vcs_operations.go and 11 more invokes the OpenIDEA RPC through the native Go adapter |
| `cancel_task` | app | yes |  | missing |
| `clear_caches` | perf | no |  | missing |
| `clipboard_file_list` | clipboard | no |  | missing |
| `clipboard_has_files` | clipboard | no |  | missing |
| `clipboard_image` | clipboard | no |  | missing |
| `clipboard_set_file_list` | clipboard | no |  | missing |
| `clipboard_text` | clipboard | no |  | missing |
| `clipboard_write_text` | clipboard | no |  | missing |
| `close_project` | app | yes | yes | integrated - internal/app/app.go invokes the OpenIDEA RPC through the native Go adapter |
| `coverage_collect` | coverage | yes | yes | integrated - internal/app/coverage_run.go calls coverage_collect with the prepared report after process exit and handles early exit replies through a bounded queue; TestCoverageCollectsAnExitBeforeTheRunStartReply covers it |
| `coverage_import` | coverage | yes | yes | integrated - base.coverage.report |
| `coverage_prepare` | coverage | yes | yes | integrated - internal/app/coverage_run.go calls coverage_prepare, validates ready/missingTool/unsupported replies and preserves the returned launch and report JSON; TestContextRunWithCoveragePreparesLaunchesAndCollects covers the flow |
| `db_apply_edits` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_completion` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_connect` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_connected` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_console_path` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_database_catalog` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_databases` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_ddl` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_disconnect` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_execute` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_explain` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_export_file` | db | no |  | missing |
| `db_finish_transaction` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_history` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_object_ddl` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_remove_history` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_retain_catalog` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_save_sources` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_saved_catalog` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_schema_objects` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_schemas` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_session_context` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_set_manual_commit` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_set_password` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_set_ssh_secret` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_sources` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_statements` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_table_details` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_table_page` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_tables` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_test` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `db_trust_ssh_host` | db | yes | yes | integrated - internal/openidea/db.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_attachable_processes` | debug | yes | yes | integrated - internal/app/attach.go, internal/app/negatives.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_breakpoint_states` | debug | yes |  | missing |
| `debug_capabilities` | debug | yes |  | missing |
| `debug_completions` | debug | yes |  | missing |
| `debug_data_breakpoint_info` | debug | yes |  | missing |
| `debug_evaluate` | debug | yes | yes | integrated - internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_evaluate_in` | debug | yes | yes | integrated - internal/app/debug_view.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_exception_filters` | debug | yes |  | missing |
| `debug_force_step_over` | debug | yes | yes | integrated - internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_free_port` | debug | yes |  | missing |
| `debug_heap_capture` | heap | yes | yes | integrated - internal/app/debug_heap.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_heap_classes` | heap | yes | yes | integrated - internal/app/debug_heap.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_heap_close` | heap | yes | yes | integrated - internal/app/debug_heap.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_heap_compare` | heap | yes |  | missing |
| `debug_heap_dominators` | heap | yes |  | missing |
| `debug_heap_open` | heap | yes | yes | integrated - internal/app/debug_heap.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_heap_path` | heap | yes |  | missing |
| `debug_heap_save` | heap | yes | yes | integrated - internal/app/debug_heap.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_hot_reload` | debug | no |  | missing |
| `debug_hot_restart` | debug | no |  | integrated - internal/app/debug_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_jvm_memory_classes` | debug | yes | yes | integrated - internal/app/debug_memory.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_jvm_memory_fields` | debug | yes | yes | integrated - internal/app/debug_memory_instances.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_jvm_memory_instances` | debug | yes | yes | integrated - internal/app/debug_memory_instances.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_jvm_memory_referrers` | debug | yes | yes | integrated - internal/app/debug_memory_instances.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_listen_for_jvm` | debug | yes |  | missing |
| `debug_pause` | debug | yes | yes | integrated - internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_redefine_classes` | debug | yes | yes | integrated - internal/app/hotswap.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_restart_frame` | debug | yes | yes | integrated - internal/app/debug_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_resume` | debug | yes | yes | integrated - internal/app/attach.go, internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_run_to` | debug | yes | yes | integrated - internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_scopes` | debug | yes | yes | integrated - internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_send_input` | debug | yes |  | missing |
| `debug_sessions` | debug | yes |  | missing |
| `debug_set_breakpoints` | debug | yes | yes | integrated - internal/app/hotswap_smoke.go, internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_set_data_breakpoints` | debug | yes | yes | integrated - internal/app/debug_actions.go, internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_set_exception_breakpoints` | debug | yes |  | missing |
| `debug_set_function_breakpoints` | debug | yes | yes | integrated - internal/app/debug_actions.go, internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_set_variable` | debug | yes | yes | integrated - internal/app/debug_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_stack_trace` | debug | yes | yes | integrated - internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_start` | debug | yes | yes | integrated - internal/app/attach.go, internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_step` | debug | yes | yes | integrated - internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_step_in_target` | debug | yes | yes | integrated - internal/app/debug_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_step_in_targets` | debug | yes | yes | integrated - internal/app/debug_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_stop` | debug | yes | yes | integrated - internal/app/attach.go, internal/app/app.go, internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_threads` | debug | yes | yes | integrated - internal/app/attach.go, internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_variables` | debug | yes | yes | integrated - internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `dependency_versions` | dependencies | yes |  | missing |
| `deploy_compare` | deploy | yes |  | missing |
| `deploy_coverage` | deploy | yes |  | missing |
| `deploy_delete_remote` | deploy | yes |  | missing |
| `deploy_download` | deploy | yes |  | missing |
| `deploy_info` | deploy | yes |  | missing |
| `deploy_server_root` | deploy | yes |  | missing |
| `deploy_sync` | deploy | yes |  | missing |
| `deploy_test_connection` | deploy | yes |  | missing |
| `deploy_upload` | deploy | yes |  | missing |
| `doc_close` | doc | yes | yes | integrated - internal/openidea/docs.go, internal/app/documentation.go invokes the OpenIDEA RPC through the native Go adapter |
| `doc_edit` | doc | yes | yes | integrated - internal/openidea/docs.go invokes the OpenIDEA RPC through the native Go adapter |
| `doc_folding` | doc | yes |  | missing |
| `doc_highlights` | doc | yes |  | missing |
| `doc_open` | doc | yes | yes | integrated - internal/openidea/docs.go, internal/app/documentation.go invokes the OpenIDEA RPC through the native Go adapter |
| `doc_open_virtual` | doc | yes | yes | integrated - internal/app/documentation.go invokes the OpenIDEA RPC through the native Go adapter |
| `doc_outline` | doc | yes | yes | integrated - internal/app/project_view.go, internal/app/codeintel.go, internal/app/copy_reference.go invokes the OpenIDEA RPC through the native Go adapter |
| `doc_save` | doc | yes | yes | integrated - internal/openidea/docs.go invokes the OpenIDEA RPC through the native Go adapter |
| `doc_save_all` | doc | yes |  | missing |
| `doc_selection_ranges` | doc | yes |  | missing |
| `doc_set_bom` | doc | yes |  | missing |
| `doc_set_encoding` | doc | yes |  | missing |
| `doc_set_line_ending` | doc | yes |  | missing |
| `doc_snapshot` | doc | yes | yes | integrated - internal/openidea/docs.go invokes the OpenIDEA RPC through the native Go adapter |
| `doc_sync_from_disk` | doc | yes |  | missing |
| `doc_ui_mode` | doc | yes |  | missing |
| `docker_compose_op` | docker | no |  | missing |
| `docker_container_op` | docker | no |  | missing |
| `docker_container_stats` | docker | no |  | missing |
| `docker_container_top` | docker | no |  | missing |
| `docker_events` | docker | no |  | missing |
| `docker_image_history` | docker | no |  | missing |
| `docker_inspect` | docker | no |  | missing |
| `docker_logs` | docker | no |  | missing |
| `docker_prune` | docker | no |  | missing |
| `docker_pull` | docker | no |  | missing |
| `docker_registry_login` | docker | no |  | missing |
| `docker_remove` | docker | no |  | missing |
| `docker_run_env` | docker | yes |  | missing |
| `docker_save_settings` | docker | no |  | missing |
| `docker_set_registry_password` | docker | no |  | missing |
| `docker_settings` | docker | no |  | missing |
| `docker_snapshot` | docker | no |  | missing |
| `env_reload` | env | yes | yes | missing |
| `file_dialog_needs_gtk` | file_dialog | no |  | missing |
| `find_dirs` | nav | yes | yes | integrated - internal/app/agent_composer.go invokes the OpenIDEA RPC through the native Go adapter |
| `find_files` | nav | yes | yes | integrated - internal/app/agent_composer.go, internal/app/run.go, internal/app/stack_trace.go and 3 more invokes the OpenIDEA RPC through the native Go adapter |
| `find_files_named` | nav | yes |  | missing |
| `find_symbols` | nav | yes |  | missing |
| `fs_compare_directories` | fs | yes | yes | integrated - internal/app/dircompare.go invokes the OpenIDEA RPC through the native Go adapter |
| `fs_copy` | fs | yes | yes | integrated - internal/app/tree_ops.go invokes the OpenIDEA RPC through the native Go adapter |
| `fs_copy_merge` | fs | yes |  | missing |
| `fs_create_dir` | fs | yes | yes | integrated - internal/app/newproject.go, internal/app/tree_ops.go invokes the OpenIDEA RPC through the native Go adapter |
| `fs_create_file` | fs | yes | yes | integrated - internal/app/tree_ops.go invokes the OpenIDEA RPC through the native Go adapter |
| `fs_create_file_bytes` | fs | yes | yes | missing |
| `fs_delete` | fs | yes | yes | integrated - internal/app/tree_ops.go, internal/app/vcs_log_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `fs_entry_kind` | fs | yes |  | missing |
| `fs_open_with_system` | fs | no |  | missing |
| `fs_probe` | bytes | yes |  | missing |
| `fs_read_dir` | fs | yes | yes | integrated - internal/app/newproject.go invokes the OpenIDEA RPC through the native Go adapter |
| `fs_read_image` | bytes | yes |  | missing |
| `fs_read_range` | bytes | yes |  | missing |
| `fs_read_text` | fs | yes | yes | integrated - internal/app/java_run.go invokes the OpenIDEA RPC through the native Go adapter |
| `fs_rename` | fs | yes | yes | integrated - internal/app/tree_ops.go invokes the OpenIDEA RPC through the native Go adapter |
| `fs_resolve_reference` | nav | yes |  | missing |
| `fs_set_writable` | fs | yes | yes | integrated - internal/app/editor_file_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `gh_account_repos` | github | no |  | missing |
| `gh_api` | github | no |  | missing |
| `gh_availability` | github | no |  | missing |
| `gh_pr_checkout` | github | no |  | missing |
| `gh_pr_comment` | github | no |  | missing |
| `gh_pr_create` | github | no |  | missing |
| `gh_pr_list` | github | no |  | missing |
| `gh_pr_prepare_diff` | github | no |  | missing |
| `gh_pr_review` | github | no |  | missing |
| `gh_pr_review_comments` | github | no |  | missing |
| `gh_pr_view` | github | no |  | missing |
| `git_credential_forget` | ssh | no |  | missing |
| `git_credentials` | ssh | no |  | missing |
| `gradle_dependencies` | build | yes |  | missing |
| `gradle_link_projects` | build | yes | yes | integrated - internal/app/build_tool_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `gradle_published_catalog` | build | yes |  | missing |
| `gradle_unlink_projects` | build | yes | yes | integrated - internal/app/build_tool_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `gtk_pick_paths` | file_dialog | no |  | missing |
| `history_content` | doc | yes | yes | integrated - internal/app/history.go invokes the OpenIDEA RPC through the native Go adapter for revision previews |
| `history_list` | doc | yes | yes | integrated - internal/app/history.go invokes the OpenIDEA RPC through the native Go adapter |
| `history_list_dir` | history | yes | yes | integrated - internal/app/history.go invokes the OpenIDEA RPC through the native Go adapter |
| `history_put_label` | history | yes | yes | integrated - internal/app/history.go invokes the OpenIDEA RPC through the native Go adapter |
| `history_restore` | history | yes | yes | integrated - internal/app/history.go invokes the OpenIDEA RPC through the native Go adapter |
| `http_curl` | http | no |  | missing |
| `http_environment_file` | http | no |  | missing |
| `http_environments` | http | no |  | missing |
| `http_execute` | http | no |  | missing |
| `http_from_curl` | http | no |  | missing |
| `http_history` | http | no |  | missing |
| `http_history_clear` | http | no |  | missing |
| `http_history_get` | http | no |  | missing |
| `http_requests` | http | no |  | missing |
| `http_variable_definitions` | http | no |  | missing |
| `index_file_extensions` | nav | yes |  | missing |
| `index_status` | nav | yes |  | missing |
| `inspect_code` | analysis | yes |  | missing |
| `java_build_workspace` | java | yes | yes | integrated - internal/app/hotswap.go invokes the OpenIDEA RPC through the native Go adapter |
| `java_debug_server` | java | yes | yes | integrated - internal/app/attach.go, internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `java_resolve_launch` | java | yes | yes | integrated - internal/app/java_run.go invokes the OpenIDEA RPC through the native Go adapter |
| `jdk_download` | tools | yes | yes | integrated - internal/openidea/tools.go, internal/app/jdk_download.go invokes the OpenIDEA RPC through the native Go adapter |
| `jdk_download_location` | tools | yes | yes | integrated - internal/openidea/tools.go, internal/app/jdk_download.go invokes the OpenIDEA RPC through the native Go adapter |
| `jdk_download_releases` | tools | yes | yes | integrated - internal/openidea/tools.go, internal/app/jdk_download.go invokes the OpenIDEA RPC through the native Go adapter |
| `jdk_download_vendors` | tools | yes | yes | integrated - internal/openidea/tools.go, internal/app/jdk_download.go invokes the OpenIDEA RPC through the native Go adapter |
| `jdtls_required_java` | tools | yes | yes | integrated - internal/openidea/tools.go invokes the OpenIDEA RPC through the native Go adapter |
| `jvm_file_presentations` | syntax | yes |  | missing |
| `languages` | app | no |  | integrated - internal/openidea/tools.go, internal/app/scratch.go, internal/app/build_tool_actions.go and 4 more invokes the OpenIDEA RPC through the native Go adapter |
| `light_edit_open` | app | no |  | missing |
| `log_frontend` | app | no |  | missing |
| `lsp_apply_code_action` | lsp | yes | yes | integrated - internal/app/intents.go, internal/app/source_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_call_hierarchy` | lsp | yes | yes | integrated - internal/app/hierarchy.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_can_download_sources` | lsp | yes |  | missing |
| `lsp_code_action_query` | lsp | yes | yes | integrated - internal/app/intents.go, internal/app/source_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_code_actions` | lsp | yes |  | missing |
| `lsp_code_lens` | lsp | yes |  | missing |
| `lsp_completion` | lsp | yes | yes | integrated - internal/app/lsp.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_document_highlights` | lsp | yes |  | missing |
| `lsp_document_links` | lsp | yes |  | missing |
| `lsp_execute_code_action` | lsp | yes |  | missing |
| `lsp_execute_command` | lsp | yes |  | missing |
| `lsp_format` | lsp | yes | yes | integrated - internal/app/format.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_goto` | lsp | yes | yes | integrated - internal/app/documentation.go, internal/app/lsp.go, internal/app/copy_reference.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_goto_implementation` | lsp | yes | yes | integrated - internal/app/navigation.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_hierarchy_members` | lsp | yes |  | missing |
| `lsp_hierarchy_refactor` | lsp | yes |  | missing |
| `lsp_hover` | lsp | yes | yes | integrated - internal/app/documentation.go, internal/app/lsp.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_implementations` | lsp | yes |  | missing |
| `lsp_inlay_hints` | lsp | yes | yes | integrated - internal/app/inlay.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_inline_completion` | lsp | yes |  | missing |
| `lsp_library_sources` | lsp | yes |  | missing |
| `lsp_prepare_rename` | lsp | yes | yes | integrated - internal/app/rename.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_references` | lsp | yes | yes | integrated - internal/app/codevision.go, internal/app/usages.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_rename` | lsp | yes | yes | integrated - internal/app/rename.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_resend_diagnostics` | lsp | yes |  | missing |
| `lsp_resolve_class_file` | lsp | yes |  | missing |
| `lsp_resolve_code_action` | lsp | yes |  | missing |
| `lsp_resolve_command` | lsp | yes |  | missing |
| `lsp_resolve_completion` | lsp | yes |  | missing |
| `lsp_restart` | lsp | yes | yes | missing |
| `lsp_restart_named` | lsp | no |  | missing |
| `lsp_semantic_tokens` | lsp | yes |  | missing |
| `lsp_servers` | lsp | yes | yes | integrated - internal/app/debug.go, internal/app/lsp.go, internal/app/readiness.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_signature_help` | lsp | yes |  | missing |
| `lsp_type_hierarchy` | lsp | yes | yes | integrated - internal/app/hierarchy.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_virtual_document` | lsp | yes | yes | integrated - internal/app/documentation.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_workspace_symbols` | lsp | yes | yes | integrated - internal/app/symbols.go, internal/app/debug_memory.go, internal/app/se.go invokes the OpenIDEA RPC through the native Go adapter |
| `mark_recent` | nav | yes | yes | integrated - internal/app/quickopen.go invokes the OpenIDEA RPC through the native Go adapter |
| `maven_dependency_completion` | build | yes | yes | integrated - internal/app/pom_completion.go invokes the OpenIDEA RPC through the native Go adapter |
| `maven_dependency_declaration` | build | yes |  | missing |
| `maven_exclude_dependency` | build | yes |  | missing |
| `maven_goto_declaration` | build | yes | yes | integrated - internal/app/pom_goto.go invokes the OpenIDEA RPC through the native Go adapter |
| `mcp_agent_run_closed` | mcp | no |  | missing |
| `mcp_report_problems` | mcp | yes |  | missing |
| `mcp_respond` | mcp | no |  | missing |
| `mcp_server_status` | mcp | no |  | missing |
| `mcp_set_focused_editor` | mcp | yes |  | missing |
| `mcp_set_open_editors` | mcp | yes |  | missing |
| `mcp_ui_ready` | mcp | no |  | missing |
| `menu_configure` | window | no |  | missing |
| `menu_model` | window | no |  | missing |
| `menu_refresh` | window | no |  | missing |
| `menu_set_disabled` | window | no |  | missing |
| `nav_index_definitions` | nav | yes |  | missing |
| `nav_resolve_module` | nav | yes |  | missing |
| `notebook_cell_docs` | notebook | yes |  | missing |
| `notebook_close` | notebook | yes |  | missing |
| `notebook_edit` | notebook | yes |  | missing |
| `notebook_execute` | notebook | yes |  | missing |
| `notebook_get` | notebook | yes |  | missing |
| `notebook_interrupt` | notebook | yes |  | missing |
| `notebook_open` | notebook | yes |  | missing |
| `notebook_redo` | notebook | yes |  | missing |
| `notebook_reload` | notebook | yes |  | missing |
| `notebook_restart` | notebook | yes |  | missing |
| `notebook_save` | notebook | yes |  | missing |
| `notebook_undo` | notebook | yes |  | missing |
| `open_project` | app | yes | yes | integrated - internal/app/app.go invokes the OpenIDEA RPC through the native Go adapter |
| `path_completions` | nav | yes | yes | integrated - internal/app/agent_composer.go invokes the OpenIDEA RPC through the native Go adapter |
| `plugin_activation_event` | plugin | no |  | missing |
| `plugin_answer_command` | plugin | yes |  | missing |
| `plugin_answer_commit` | plugin | yes |  | missing |
| `plugin_answer_prompt` | plugin | yes |  | missing |
| `plugin_answer_run_target` | plugin | yes |  | missing |
| `plugin_assets` | plugin | no |  | missing |
| `plugin_cancel_task` | plugin | yes |  | missing |
| `plugin_catalog` | plugin | no |  | missing |
| `plugin_choose_program` | plugin | yes |  | missing |
| `plugin_color_presentations` | lsp | yes |  | missing |
| `plugin_commit_checks` | plugin | yes |  | missing |
| `plugin_content_tab_action` | plugin | yes |  | missing |
| `plugin_context_keys` | plugin | yes |  | missing |
| `plugin_decoration_sets` | plugin | yes |  | missing |
| `plugin_endpoints` | plugin | yes |  | missing |
| `plugin_event_kinds` | plugin | yes |  | missing |
| `plugin_finish_operation` | plugin | yes |  | missing |
| `plugin_formatters` | lsp | yes |  | missing |
| `plugin_grant` | plugin | no |  | missing |
| `plugin_install_dir` | plugin | no |  | missing |
| `plugin_installed_intellij_themes` | plugin | no |  | missing |
| `plugin_language_visuals` | lsp | yes |  | missing |
| `plugin_line_markers` | plugin | yes |  | missing |
| `plugin_link` | plugin | no |  | missing |
| `plugin_list` | plugin | no |  | missing |
| `plugin_log_tab_spawn` | plugin | no |  | missing |
| `plugin_market` | plugin | no |  | missing |
| `plugin_market_download` | plugin | no |  | missing |
| `plugin_market_url` | plugin | no |  | missing |
| `plugin_navigation_files` | plugin | yes |  | missing |
| `plugin_navigation_goto` | plugin | yes |  | missing |
| `plugin_navigation_index` | plugin | yes |  | missing |
| `plugin_navigation_invalidate` | plugin | yes |  | missing |
| `plugin_navigation_markers` | plugin | yes |  | missing |
| `plugin_parse_test_results` | plugin | yes |  | missing |
| `plugin_post_events` | plugin | yes |  | missing |
| `plugin_prepare_debug_run` | plugin | yes | yes | integrated - internal/app/debug_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `plugin_preview_install` | plugin | no |  | missing |
| `plugin_program` | plugin | yes |  | missing |
| `plugin_read_intellij_theme` | plugin | no |  | missing |
| `plugin_reload` | plugin | no |  | missing |
| `plugin_revoke_net_host` | plugin | no |  | missing |
| `plugin_roll_back` | plugin | no |  | missing |
| `plugin_run_command` | plugin | yes |  | missing |
| `plugin_run_targets` | plugin | yes |  | missing |
| `plugin_secrets` | plugin | no |  | missing |
| `plugin_select_tests` | plugin | yes |  | missing |
| `plugin_service_detail` | plugin | yes |  | missing |
| `plugin_service_log` | plugin | yes |  | missing |
| `plugin_set_active_file` | plugin | yes |  | missing |
| `plugin_set_capability` | plugin | no |  | missing |
| `plugin_set_enabled` | plugin | no |  | missing |
| `plugin_set_market_url` | plugin | no |  | missing |
| `plugin_set_secret` | plugin | no |  | missing |
| `plugin_set_theme` | plugin | no |  | missing |
| `plugin_set_theme_frame` | plugin | no |  | missing |
| `plugin_set_ui_locale` | plugin | yes |  | missing |
| `plugin_stage_package` | plugin | no |  | missing |
| `plugin_status_items` | plugin | yes |  | missing |
| `plugin_subscribe` | plugin | no |  | missing |
| `plugin_terminal_session_close` | plugin | yes |  | missing |
| `plugin_terminal_session_spawn` | plugin | yes |  | missing |
| `plugin_uninstall` | plugin | no |  | missing |
| `plugin_unlink` | plugin | no |  | missing |
| `plugin_unpack_package` | plugin | no |  | missing |
| `plugin_view_event` | plugin | yes |  | missing |
| `plugin_view_render` | plugin | yes |  | missing |
| `plugin_web_page` | plugin | no |  | missing |
| `plugin_web_pty_spawn` | plugin | no |  | missing |
| `plugin_web_read_file` | plugin | no |  | missing |
| `plugin_web_save_file` | plugin | no |  | missing |
| `prepare_workspace_edit` | lsp | yes |  | missing |
| `profiler_collect` | profiler | no |  | missing |
| `profiler_gc_before_stop` | profiler | no |  | missing |
| `profiler_prepare` | profiler | no |  | missing |
| `project_current` | app | yes | yes | integrated - internal/app/recovery.go, internal/app/app.go, internal/app/project_workspace.go and 2 more invokes the OpenIDEA RPC through the native Go adapter |
| `project_rename` | app | no |  | missing |
| `project_roots` | app | yes |  | missing |
| `project_trust_add_location` | app | yes | yes | integrated - OpenIDEA crates/ide-host/src/rpc.rs exposes project_trust_add_location; internal/app/project_trust_settings.go opens the native Add Trusted Location picker and refreshes the page; crates/ide-host/tests/rpc.rs round-trips add/list/remove |
| `project_trust_list` | app | yes | yes | integrated - internal/app/project_trust_settings.go loads project_trust_list into Settings › Security › Project Trust and refreshes it after every mutation; internal/app/project_trust_settings_test.go |
| `project_trust_remove_location` | app | yes | yes | integrated - internal/app/run.go and project_trust_settings.go remove trusted locations through project_trust_remove_location; crates/ide-host/tests/rpc.rs round-trips add/list/remove |
| `project_trust_set` | app | yes | yes | integrated - internal/app/run.go and project_trust_settings.go persist trust and Restricted Mode decisions through project_trust_set; internal/app/trust_test.go and internal/openidea integration tests |
| `project_trust_status` | app | yes | yes | integrated - internal/app/run.go and project_workspace.go call project_trust_status before project commands and when an untrusted project opens; internal/app/trust_test.go and internal/openidea integration tests |
| `proxy_check_connection` | http | no |  | missing |
| `recent_files` | nav | yes | yes | integrated - internal/app/recent_navigation.go invokes the OpenIDEA RPC through the native Go adapter |
| `recent_project_groups` | app | no |  | missing |
| `recent_projects` | app | no |  | missing |
| `redo_workspace_edit` | lsp | yes | yes | integrated - internal/app/refactor.go invokes the OpenIDEA RPC through the native Go adapter |
| `refactor_can_undo_file_move` | refactor | yes |  | missing |
| `refactor_file_move_edits` | refactor | yes | yes | integrated - internal/app/project_tree_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `refactor_file_move_text_edits` | refactor | yes | yes | missing |
| `refactor_move_files` | refactor | yes | yes | integrated - internal/app/project_tree_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `refactor_undo_file_move` | refactor | yes | yes | integrated - internal/app/project_tree_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `remote_bootstrap_list` | remote_dev | no |  | missing |
| `remote_connect` | remote_dev | no |  | missing |
| `remote_current` | remote_dev | no |  | missing |
| `remote_delete` | deploy | yes |  | missing |
| `remote_disconnect` | remote_dev | no |  | missing |
| `remote_download_into` | deploy | yes |  | missing |
| `remote_forward_port` | remote_dev | no |  | missing |
| `remote_forwarded_ports` | remote_dev | no |  | missing |
| `remote_import_local` | remote_dev | no |  | missing |
| `remote_invoke` | remote_dev | no |  | missing |
| `remote_list` | deploy | yes |  | missing |
| `remote_mkdir` | deploy | yes |  | missing |
| `remote_of` | deploy | yes |  | missing |
| `remote_open` | deploy | yes |  | missing |
| `remote_rename` | deploy | yes |  | missing |
| `remote_stop_forwarding` | remote_dev | no |  | missing |
| `remote_trust_set` | remote_dev | no |  | missing |
| `remote_trust_status` | remote_dev | no |  | missing |
| `remove_recent` | nav | yes | yes | integrated - internal/app/recent_navigation.go invokes the OpenIDEA RPC through the native Go adapter |
| `remove_recent_project` | app | no |  | missing |
| `replace_in_files` | search | yes | yes | integrated - internal/app/search.go invokes the OpenIDEA RPC through the native Go adapter |
| `replace_preview` | search | yes |  | missing |
| `run_application_build` | run | yes |  | missing |
| `run_application_publish` | run | yes |  | missing |
| `run_apply_settings` | run | yes |  | missing |
| `run_close_stdin` | run | yes |  | missing |
| `run_configs` | run | yes | yes | integrated - internal/app/run_parity_actions.go, internal/app/run.go, internal/app/java_run.go and 2 more invokes the OpenIDEA RPC through the native Go adapter |
| `run_dotnet_resolve` | run | yes |  | missing |
| `run_env_files` | run | yes | yes | integrated - internal/app/debug.go invokes the OpenIDEA RPC through the native Go adapter |
| `run_gradle_init_script` | run | yes |  | missing |
| `run_gradle_jvm_args_script` | run | yes |  | missing |
| `run_import_configs` | run | yes | yes | integrated - internal/app/run_parity_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `run_import_sources` | run | yes | yes | integrated - internal/app/run_parity_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `run_list` | run | yes | yes | integrated - internal/app/run.go, internal/app/negatives.go invokes the OpenIDEA RPC through the native Go adapter |
| `run_resize` | run | yes |  | missing |
| `run_save_configs` | run | yes | yes | integrated - internal/app/java_run.go, internal/app/negatives.go invokes the OpenIDEA RPC through the native Go adapter |
| `run_save_templates` | run | yes |  | missing |
| `run_start` | run | yes | yes | integrated - internal/app/run.go invokes the OpenIDEA RPC through the native Go adapter |
| `run_stdin` | run | yes |  | missing |
| `run_stop` | run | yes | yes | integrated - internal/app/run.go, internal/app/app.go invokes the OpenIDEA RPC through the native Go adapter |
| `run_system_env` | run | yes |  | missing |
| `run_templates` | run | yes |  | missing |
| `run_test_reports` | run | yes |  | missing |
| `run_tomcat_artifacts` | run | yes |  | missing |
| `run_tomcat_build` | run | yes |  | missing |
| `run_tomcat_home` | run | yes |  | missing |
| `run_tomcat_installations` | run | yes |  | missing |
| `run_tomcat_sync` | run | yes |  | missing |
| `run_tomcat_update` | run | yes |  | missing |
| `run_without_derived` | run | yes |  | missing |
| `scaffold_catalog` | scaffold | yes | yes | integrated - internal/app/newproject_scaffold.go invokes the OpenIDEA RPC through the native Go adapter |
| `scaffold_create` | scaffold | yes | yes | integrated - internal/app/newproject_scaffold.go invokes the OpenIDEA RPC through the native Go adapter |
| `scaffold_options` | scaffold | yes | yes | integrated - internal/app/newproject_scaffold.go invokes the OpenIDEA RPC through the native Go adapter |
| `scaffold_requirements` | scaffold | yes |  | missing |
| `sdk_detect` | sdk | yes | yes | missing |
| `sdk_locate_tools` | sdk | yes |  | missing |
| `sdk_php_composer_install` | sdk | yes |  | missing |
| `sdk_php_effective` | sdk | yes |  | missing |
| `sdk_php_language_level` | sdk | yes |  | missing |
| `sdk_php_xdebug` | sdk | yes |  | missing |
| `sdk_project_libraries` | sdk | yes |  | missing |
| `sdk_python_create_venv` | sdk | yes |  | missing |
| `sdk_python_effective` | sdk | yes |  | missing |
| `sdk_python_interpreter` | sdk | yes |  | missing |
| `sdk_python_packages` | sdk | yes |  | missing |
| `sdk_python_pip` | sdk | yes |  | missing |
| `sdk_python_project_interpreters` | sdk | yes |  | missing |
| `search_start` | search | yes | yes | integrated - internal/app/search.go, internal/app/bench.go, internal/app/se.go and 1 more invokes the OpenIDEA RPC through the native Go adapter |
| `search_validate` | search | yes | yes | integrated - internal/app/search.go invokes the OpenIDEA RPC through the native Go adapter |
| `set_recent_project_groups` | app | no |  | missing |
| `settings_export` | app | no |  | missing |
| `settings_get` | app | yes | yes | integrated - internal/app/build_tool_actions.go, internal/app/jdk_download.go invokes the OpenIDEA RPC through the native Go adapter |
| `settings_get_global` | app | no |  | integrated - internal/app/agent_settings_backend.go invokes the OpenIDEA RPC through the native Go adapter |
| `settings_import` | app | no |  | missing |
| `settings_list_jetbrains_ides` | app | no |  | missing |
| `settings_read_jetbrains` | app | no |  | missing |
| `settings_set` | app | yes | yes | integrated - internal/app/build_tool_actions.go, internal/app/agent_settings_backend.go, internal/app/jdk_download.go and 1 more invokes the OpenIDEA RPC through the native Go adapter |
| `spell_check` | spell | yes |  | missing |
| `spell_suggest` | spell | yes |  | missing |
| `spring_route_sources` | nav | no |  | missing |
| `ssh_askpass_answer` | ssh | no |  | missing |
| `ssh_connected` | ssh | no |  | missing |
| `ssh_disconnect` | ssh | no |  | missing |
| `ssh_needs_secret` | ssh | no |  | missing |
| `ssh_open_terminal` | ssh | no |  | missing |
| `ssh_set_secret` | ssh | no |  | missing |
| `ssh_test` | ssh | no |  | missing |
| `ssh_trust_host` | ssh | no |  | missing |
| `structural_languages` | analysis | yes |  | missing |
| `structural_search` | analysis | yes |  | missing |
| `structural_validate` | analysis | yes |  | missing |
| `syntax_errors` | syntax | yes |  | missing |
| `syntax_expression_at` | syntax | yes |  | missing |
| `syntax_identifiers` | syntax | yes | yes | missing |
| `syntax_identifiers_in_files` | syntax | yes |  | missing |
| `syntax_locals` | syntax | yes | yes | missing |
| `syntax_node_at` | syntax | yes |  | missing |
| `syntax_structural_ranges` | syntax | yes | yes | missing |
| `syntax_support` | syntax | yes |  | missing |
| `system_font_families` | fonts | no |  | missing |
| `system_probe_hotkeys` | hotkeys | no |  | missing |
| `term_available_shells` | term | yes | yes | integrated - internal/app/term.go invokes the OpenIDEA RPC through the native Go adapter |
| `term_default_shell` | term | yes | yes | integrated - internal/app/term.go invokes the OpenIDEA RPC through the native Go adapter |
| `term_is_busy` | term | yes |  | missing |
| `term_kill` | term | yes | yes | integrated - internal/app/term.go, internal/app/app.go invokes the OpenIDEA RPC through the native Go adapter |
| `term_resize` | term | yes |  | missing |
| `term_spawn` | term | yes | yes | integrated - internal/app/term.go invokes the OpenIDEA RPC through the native Go adapter |
| `term_store_image` | term | yes |  | missing |
| `term_write` | term | yes | yes | integrated - internal/app/term.go invokes the OpenIDEA RPC through the native Go adapter |
| `tools_install` | tools | yes | yes | integrated - internal/openidea/tools.go invokes the OpenIDEA RPC through the native Go adapter |
| `tools_list` | tools | yes | yes | integrated - internal/openidea/tools.go invokes the OpenIDEA RPC through the native Go adapter |
| `tools_uninstall` | tools | yes | yes | integrated - internal/openidea/tools.go invokes the OpenIDEA RPC through the native Go adapter |
| `undo_workspace_edit` | lsp | yes | yes | integrated - internal/app/refactor.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_add_to_ignore` | vcs | yes | yes | integrated - internal/app/vcs_more.go, internal/app/vcs.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_add_worktree` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_apply_patch` | vcs | yes | yes | integrated - internal/app/vcs_more.go, internal/app/vcs_log_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_blame` | vcs | yes | yes | integrated - internal/app/annotate.go, internal/app/codevision.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_block_history` | vcs | yes |  | missing |
| `vcs_branch_info` | vcs | yes | yes | integrated - internal/openidea/types.go, internal/app/branch.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_branches` | vcs | yes | yes | integrated - internal/openidea/types.go, internal/app/branch.go, internal/app/vcs_log_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_changelists_load` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_changelists_save` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_checkout` | vcs | yes | yes | integrated - internal/app/vcs_operations.go, internal/app/branch.go, internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_clone` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_commit` | vcs | yes | yes | integrated - internal/app/vcs_operations.go, internal/app/commit.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_commit_files` | vcs | yes |  | missing |
| `vcs_commits_changes` | vcs | yes | yes | integrated - internal/app/log.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_compare_paths` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_compare_scope` | vcs | yes |  | missing |
| `vcs_config_get` | vcs | yes | yes | integrated - internal/app/vcs_more.go, internal/app/vcs_log_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_conflict_versions` | vcs | yes | yes | integrated - internal/app/vcs_more.go, internal/app/merge_editor.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_create_branch` | vcs | yes | yes | integrated - internal/app/vcs_operations.go, internal/app/branch.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_create_patch` | vcs | yes | yes | integrated - internal/app/vcs_more.go, internal/app/vcs_log_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_create_tag` | vcs | yes | yes | integrated - internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_delete_branch` | vcs | yes | yes | integrated - internal/app/vcs_operations.go, internal/app/branch.go, internal/app/vcs_log_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_delete_tag` | vcs | yes | yes | integrated - internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_detected_git` | vcs | yes |  | missing |
| `vcs_diff_hunks` | vcs | yes | yes | integrated - internal/app/codeintel.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_diff_numstat` | vcs | yes |  | missing |
| `vcs_diff_revisions` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_diff_text` | vcs | yes |  | missing |
| `vcs_diff_with_worktree` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_discard` | vcs | yes | yes | integrated - internal/app/vcs_operations.go, internal/app/vcs.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_edit_remote` | vcs | yes | yes | integrated - internal/app/vcs_remotes.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_file_at` | vcs | yes | yes | integrated - internal/app/codeintel.go, internal/app/agent_diff.go, internal/app/vcs_more.go and 2 more invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_file_mode` | vcs | yes |  | missing |
| `vcs_git_version` | vcs | yes |  | missing |
| `vcs_has_hook` | vcs | yes |  | missing |
| `vcs_init` | vcs | yes | yes | integrated - internal/app/newproject.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_line_hunks` | vcs | yes |  | missing |
| `vcs_log` | vcs | yes | yes | integrated - internal/app/log.go, internal/app/vcs_more.go, internal/app/vcs_log_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_log_details` | vcs | yes | yes | integrated - internal/app/log.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_log_details_batch` | vcs | yes |  | missing |
| `vcs_log_refs` | vcs | yes |  | missing |
| `vcs_log_users` | vcs | yes |  | missing |
| `vcs_merge` | vcs | yes |  | missing |
| `vcs_merge_base` | vcs | yes |  | missing |
| `vcs_merge_with` | vcs | yes | yes | integrated - internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_move_worktree` | vcs | yes |  | missing |
| `vcs_open_external_log` | vcs | yes | yes | integrated - internal/app/agent_diff.go, internal/app/vcs_log_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_operation` | vcs | yes | yes | integrated - internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_patch_preview` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_rebase` | vcs | yes |  | missing |
| `vcs_rebase_with` | vcs | yes | yes | integrated - internal/app/vcs_operations.go, internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_remote` | vcs | yes | yes | integrated - internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_remotes` | vcs | yes | yes | integrated - internal/openidea/types.go, internal/app/vcs_remotes.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_remove_worktree` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_rename_branch` | vcs | yes | yes | missing |
| `vcs_restore_branch` | vcs | yes | yes | missing |
| `vcs_restore_head` | vcs | yes |  | missing |
| `vcs_restore_tag` | vcs | yes |  | missing |
| `vcs_revision_children` | vcs | yes |  | missing |
| `vcs_revision_files` | vcs | yes |  | missing |
| `vcs_reword` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_roots` | vcs | yes |  | missing |
| `vcs_shelf_delete` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_shelf_diff` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_shelf_import` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_shelf_patch` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_shelf_rename` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_shelve` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_shelves` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_stage` | vcs | yes | yes | integrated - internal/app/vcs_operations.go, internal/app/vcs_more.go, internal/app/merge_editor.go and 1 more invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_stage_hunks` | vcs | yes |  | missing |
| `vcs_stage_patch` | vcs | yes |  | missing |
| `vcs_stash_apply` | vcs | yes | yes | integrated - internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_stash_drop` | vcs | yes |  | missing |
| `vcs_stash_push` | vcs | yes | yes | integrated - internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_stashes` | vcs | yes | yes | integrated - internal/openidea/types.go, internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_status` | vcs | yes | yes | integrated - internal/app/commit.go, internal/app/merge_editor.go, internal/app/diff.go and 1 more invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_tags` | vcs | yes | yes | integrated - internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_undo_commit` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_unshelve` | vcs | yes | yes | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_unstage` | vcs | yes | yes | integrated - internal/app/vcs.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_worktrees` | vcs | yes | yes | integrated - internal/openidea/types.go, internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_write_patch` | vcs | yes |  | missing |
| `web_browsers` | web | no |  | missing |
| `web_goto` | nav | yes |  | missing |
| `web_open` | web | no |  | missing |
| `web_unresolved_references` | nav | yes |  | missing |
| `web_url` | web | yes |  | missing |
| `window_close_context` | window | no |  | missing |
| `window_complete_launch` | window | no |  | missing |
| `window_cycle` | window | no |  | missing |
| `window_flash_taskbar` | window | no |  | missing |
| `window_group_cancel_close` | window_group | no |  | missing |
| `window_group_close` | window_group | no |  | missing |
| `window_group_close_tabs` | window_group | no |  | missing |
| `window_group_current` | window_group | no |  | missing |
| `window_group_detach` | window_group | no |  | missing |
| `window_group_merge` | window_group | no |  | missing |
| `window_group_reorder` | window_group | no |  | missing |
| `window_group_restore` | window_group | no |  | missing |
| `window_group_select` | window_group | no |  | missing |
| `window_is_fullscreen` | window | no |  | missing |
| `window_new` | window | no |  | missing |
| `window_open_comparison` | window | no |  | missing |
| `window_open_deployment` | window | no |  | missing |
| `window_open_diff` | window | no |  | missing |
| `window_open_file_in_new_window` | window | no |  | missing |
| `window_open_files` | window | no |  | missing |
| `window_open_project` | window | no |  | missing |
| `window_open_projects` | window | no |  | missing |
| `window_open_remote` | window | no |  | missing |
| `window_open_settings` | window | no |  | missing |
| `window_set_titlebar_height` | window | no |  | missing |
| `window_start_drag` | window_drag | no |  | missing |
| `window_take_launch` | window | no |  | missing |
| `window_toggle_fullscreen` | window | no |  | missing |
| `window_toggle_zoom` | window | no |  | missing |
| `workspace_attach` | workspace | yes | yes | integrated - internal/app/project_workspace.go invokes the OpenIDEA RPC through the native Go adapter |
| `workspace_create` | workspace | yes | yes | integrated - internal/app/project_workspace.go invokes the OpenIDEA RPC through the native Go adapter |
| `workspace_discover_projects` | workspace | yes | yes | integrated - internal/app/project_workspace.go invokes the OpenIDEA RPC through the native Go adapter |
| `workspace_update` | workspace | yes | yes | integrated - internal/app/project_workspace.go invokes the OpenIDEA RPC through the native Go adapter |
| `wsl_distros` | wsl | no |  | missing |
| `wsl_resolve` | wsl | no |  | missing |

481 commands are served over stdio and 268 are called by GoIDEA today. A command
that is registered only in the desktop shell needs an `ops` module before stdio
can serve it; a GoIDEA check counts only when the UI path works end to end.

## Events and channels

### Backend events

| Enum | Variant | Wire type | Status |
| --- | --- | --- | --- |
| IdeEvent | `McpRequest` | `mcpRequest` | missing |
| IdeEvent | `McpRequestClosed` | `mcpRequestClosed` | missing |
| IdeEvent | `ProjectTrustChanged` | `projectTrustChanged` | missing |
| IdeEvent | `FsChanged` | `fsChanged` | missing |
| IdeEvent | `ExcludedDirsChanged` | `excludedDirsChanged` | missing |
| IdeEvent | `DocPatched` | `docPatched` | missing |
| IdeEvent | `DocDirty` | `docDirty` | missing |
| IdeEvent | `WorkspaceEditApplied` | `workspaceEditApplied` | missing |
| IdeEvent | `DocLanguage` | `docLanguage` | missing |
| IdeEvent | `Diagnostics` | `diagnostics` | missing |
| IdeEvent | `Task` | `task` | missing |
| IdeEvent | `IndexState` | `indexState` | missing |
| IdeEvent | `VcsChanged` | `vcsChanged` | missing |
| IdeEvent | `LanguageServer` | `languageServer` | missing |
| IdeEvent | `LanguageServerWarning` | `languageServerWarning` | missing |
| IdeEvent | `LanguageServerRemoved` | `languageServerRemoved` | missing |
| IdeEvent | `LspRefresh` | `lspRefresh` | missing |
| IdeEvent | `TerminalExited` | `terminalExited` | missing |
| IdeEvent | `RunExited` | `runExited` | missing |
| IdeEvent | `Debug` | `debug` | missing |
| IdeEvent | `Notify` | `notify` | missing |
| IdeEvent | `SettingsChanged` | `settingsChanged` | missing |
| IdeEvent | `UpdateStatusChanged` | `updateStatusChanged` | missing |
| IdeEvent | `UpdateAvailable` | `updateAvailable` | missing |
| IdeEvent | `ProjectRootsChanged` | `projectRootsChanged` | missing |
| IdeEvent | `Askpass` | `askpass` | missing |
| IdeEvent | `AskpassDone` | `askpassDone` | missing |
| IdeEvent | `NotebookKernel` | `notebookKernel` | missing |
| McpUiRequest | `Confirm` | `confirm` | missing |
| McpUiRequest | `OpenFile` | `openFile` | missing |
| McpUiRequest | `AgentRun` | `agentRun` | missing |

The desktop shell forwards these as one Tauri event (`ide://event`). Over stdio
they are `event` notifications; high-volume streams use `channel` notifications
instead (RPC_PROTOCOL.md).

### Streaming channels

| Command | Streaming arguments | Status |
| --- | --- | --- |
| `agent_registry_install` | `onEvent` | integrated - internal/app/agent_registry_settings.go invokes the OpenIDEA RPC through the native Go adapter |
| `agent_subscribe` | `onEvent` | integrated - internal/app/agents.go, internal/app/agents_smoke.go invokes the OpenIDEA RPC through the native Go adapter |
| `build_compile_file` | `onEvent` | integrated - internal/app/build.go invokes the OpenIDEA RPC through the native Go adapter |
| `build_run` | `onEvent` | integrated - internal/app/build.go invokes the OpenIDEA RPC through the native Go adapter |
| `build_sync` | `onProgress` | integrated - internal/app/maven.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_heap_capture` | `onProgress` | integrated - internal/app/debug_heap.go invokes the OpenIDEA RPC through the native Go adapter |
| `debug_heap_open` | `onProgress` | integrated - internal/app/debug_heap.go invokes the OpenIDEA RPC through the native Go adapter |
| `fs_compare_directories` | `onProgress` | integrated - internal/app/dircompare.go invokes the OpenIDEA RPC through the native Go adapter |
| `inspect_code` | `onEvent` | missing |
| `jdk_download` | `onEvent` | integrated - internal/openidea/tools.go, internal/app/jdk_download.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_code_action_query` | `only` | integrated - internal/app/intents.go, internal/app/source_actions.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_code_actions` | `only` | missing |
| `lsp_goto_implementation` | `onPartial` | integrated - internal/app/navigation.go invokes the OpenIDEA RPC through the native Go adapter |
| `lsp_references` | `onPartial` | integrated - internal/app/codevision.go, internal/app/usages.go invokes the OpenIDEA RPC through the native Go adapter |
| `notebook_execute` | `onOutput` | missing |
| `plugin_service_log` | `on`, `onLine` | missing |
| `plugin_terminal_session_spawn` | `onOutput` | missing |
| `replace_in_files` | `onEvent` | integrated - internal/app/search.go invokes the OpenIDEA RPC through the native Go adapter |
| `replace_preview` | `only` | missing |
| `run_start` | `onOutput` | integrated - internal/app/run.go invokes the OpenIDEA RPC through the native Go adapter |
| `scaffold_create` | `onEvent` | integrated - internal/app/newproject_scaffold.go invokes the OpenIDEA RPC through the native Go adapter |
| `sdk_project_libraries` | `onGroups` | missing |
| `sdk_python_create_venv` | `onOutput` | missing |
| `sdk_python_pip` | `onOutput` | missing |
| `search_start` | `onEvent` | integrated - internal/app/search.go, internal/app/bench.go, internal/app/se.go and 1 more invokes the OpenIDEA RPC through the native Go adapter |
| `structural_search` | `onEvent` | missing |
| `term_spawn` | `onOutput` | integrated - internal/app/term.go invokes the OpenIDEA RPC through the native Go adapter |
| `tools_install` | `onEvent` | integrated - internal/openidea/tools.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_clone` | `onOutput` | integrated - internal/app/vcs_more.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_operation` | `onOutput` | integrated - internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |
| `vcs_rebase` | `onto` | missing |
| `vcs_remote` | `onOutput` | integrated - internal/app/vcs_operations.go invokes the OpenIDEA RPC through the native Go adapter |

## Backend feature modules (crates)

| Crate | Role | Linked into `ide-host` (stdio reachable) | Status |
| --- | --- | --- | --- |
| `ide-agent` | Coding agents over the Agent Client Protocol: tasks, sessions, worktrees and their timelines | yes | engine-only - linked into ide-host |
| `ide-analysis` | Project-wide code analysis over syntax trees: duplicate detection (Locate Duplicates) and structural search & replace | yes | engine-only - linked into ide-host |
| `ide-android` | Android devices and logcat for the Logcat tool window, driven through the adb CLI | no | missing |
| `ide-build` | Build tool models for the Maven and Gradle tool windows: POM parsing, Gradle task lists, background sync | yes | engine-only - linked into ide-host |
| `ide-cli` | The `openidea` command: arguments, the request it sends a running app over a local socket, and installing it on PATH | no | missing |
| `ide-clipboard` | The OS clipboard's file list (Explorer <-> project view): Windows CF_HDROP or macOS NSPasteboard file URLs, no window or message loop | no | missing |
| `ide-core` | Shared vocabulary of OpenIDEA: ids, text coordinates, events, errors, settings | yes | engine-only - linked into ide-host |
| `ide-coverage` | Run with Coverage: coverage-tool launch plans (cargo-llvm-cov, vitest / jest / c8, coverage.py, go test, JaCoCo) and report parsers (lcov, Go cover profiles, JaCoCo XML) | yes | engine-only - linked into ide-host |
| `ide-dap` | Debug adapter protocol client: sessions, breakpoints, threads, frames, variables, evaluation | yes | engine-only - linked into ide-host |
| `ide-db` | Database tools: data sources, connections, introspection, query execution and table editing | yes | engine-only - linked into ide-host |
| `ide-docker` | Docker integration for the Services tool window, driven through the docker CLI | yes | engine-only - linked into ide-host |
| `ide-extension` | Plugin manifest, extension points, contribution registry and capability model (no Wasm runtime) | yes | engine-only - linked into ide-host |
| `ide-fonts` | Font families installed on this machine, for the font pickers: CoreText, DirectWrite or fontconfig | no | missing |
| `ide-github` | GitHub pull requests through the gh CLI | no | missing |
| `ide-heap` | HotSpot .hprof parser and heap-snapshot analysis for the debugger's JVM Memory view | yes | engine-only - linked into ide-host |
| `ide-host` | The IDE backend without a UI toolkit: projects per window, terminals, processes, and the JSON-RPC server for remote development | yes | engine-only - linked into ide-host |
| `ide-http` | HTTP Client: .http request files, environments, execution and response history | yes | engine-only - linked into ide-host |
| `ide-index` | Background project index: files and symbols, persisted in redb, fuzzy queried with nucleo | yes | engine-only - linked into ide-host |
| `ide-jdwp` | JDWP codec and a transparent debugger link that counts every object ID it receives, so it can release them exactly | no | missing |
| `ide-jsonrpc` | Content-Length framed transport and JSON-RPC 2.0 peer shared by LSP and DAP clients | yes | engine-only - linked into ide-host |
| `ide-jvm-attach` | HotSpot attach-socket client and class-histogram parser for the debugger's JVM Memory view | yes | engine-only - linked into ide-host |
| `ide-jvm-inspect` | Shared types and the JvmInspector trait for inspecting live JVM instances while a debug session is suspended | no | missing |
| `ide-language` | Language registry (languages/*.toml), detection, and tree-sitter powered outline/folding/symbol tags | yes | engine-only - linked into ide-host |
| `ide-lsp` | Language server manager: lifecycle, document sync, feature requests mapped to IDE types | yes | engine-only - linked into ide-host |
| `ide-mcp` | The IDE's MCP server: Streamable HTTP on 127.0.0.1 for AI agents outside the IDE, its discovery file and the stdio bridge | yes | engine-only - linked into ide-host |
| `ide-notebook` | Lossless native notebook documents and owned Jupyter kernels | yes | engine-only - linked into ide-host |
| `ide-plugin` | WebAssembly runtime for plugins (wasmtime component model + WASI preview 2) | yes | engine-only - linked into ide-host |
| `ide-profiler` | Run with Profiler: sampling-profiler launch plans (Node.js --cpu-prof, py-spy, async-profiler, Linux perf) and a common call-tree model from .cpuprofile, collapsed stacks and perf script output | no | missing |
| `ide-remote` | Remote work over SSH: SFTP deployment (mappings, upload, download, sync) and run targets (SSH, Docker, Compose) | yes | engine-only - linked into ide-host |
| `ide-run` | Run configurations: detection, persistence, process lifecycle and output streaming | yes | engine-only - linked into ide-host |
| `ide-scaffold` | New Project generators: the generator catalog (`generators/*.toml`), project templates, official framework CLIs and initializr-style project services | yes | engine-only - linked into ide-host |
| `ide-sdk` | SDKs / toolchains: interpreter discovery (Python, Node.js, Rust, Go, TypeScript), virtualenvs, pip, and applying the selected toolchain to language servers, run configurations and terminals | yes | engine-only - linked into ide-host |
| `ide-search` | Find / Replace in Files built on the ripgrep engine crates | yes | engine-only - linked into ide-host |
| `ide-spell` | Spell checking (Proofreading › Typo): word extraction from comments, strings and declared names, Hunspell en_US / en_GB dictionary, programming terms, user dictionaries | yes | engine-only - linked into ide-host |
| `ide-ssh` | SSH connections: configurations, host keys, authentication, remote commands, SFTP and FTP files, port forwarding and askpass | yes | engine-only - linked into ide-host |
| `ide-syntax` | Tree-sitter structure service: node/expression lookup, syntax errors, scopes & locals, identifier occurrences, structural ranges (LSP fallback) | yes | engine-only - linked into ide-host |
| `ide-terminal` | Integrated terminal PTY sessions | yes | engine-only - linked into ide-host |
| `ide-text` | Rope-backed documents, change sets and UTF-16/byte coordinate conversion | yes | engine-only - linked into ide-host |
| `ide-tools` | Installer for language servers and debug adapters: registry, detection, download/extract, managed tools dir | yes | engine-only - linked into ide-host |
| `ide-vcs` | Git integration: gitoxide for fast reads, git CLI for writes | yes | engine-only - linked into ide-host |
| `ide-vfs` | File system access: directory listing, text loading/saving with encoding detection, watching, project walking | yes | engine-only - linked into ide-host |
| `ide-web` | Web browsers: the built-in static web server that serves project files on 127.0.0.1 (IDEA's built-in server) and detection / launching of installed browsers | yes | engine-only - linked into ide-host |
| `ide-workspace` | Project model: open documents, settings layering, recent projects, session state, local history | yes | engine-only - linked into ide-host |

`engine-only` means the crate is linked into the stdio host, so its logic can be
reached; it does not mean GoIDEA has a UI for it. A crate that is not linked
needs an OpenIDEA change before any GoIDEA work can call it.

## Plugin extension points

Plugins are sandboxed WebAssembly components (`wit/`, interface version 1.5.0).
The base world imports the host API and exports three interfaces; optional worlds
add exports. GoIDEA does not load plugins yet, so every entry starts as `missing`.

### Host API imported by plugins (27)

| Interface | Status |
| --- | --- |
| `auth` | missing |
| `command-runner` | missing |
| `commit` | missing |
| `database` | missing |
| `decorations` | missing |
| `diagnostics` | missing |
| `editor` | missing |
| `events` | missing |
| `extensions` | missing |
| `fs` | missing |
| `invocation` | missing |
| `ipc` | missing |
| `log` | missing |
| `mcp` | missing |
| `net` | missing |
| `output` | missing |
| `process` | missing |
| `run` | missing |
| `run-targets` | missing |
| `secrets` | missing |
| `settings` | missing |
| `storage` | missing |
| `tasks` | missing |
| `ui` | missing |
| `vcs` | missing |
| `views` | missing |
| `workspace` | missing |

### Exported by plugins (21)

| Export | Worlds | Status |
| --- | --- | --- |
| `commands` | plugin | missing |
| `commit-checker` | commit-checks, plugin-commit-checks | missing |
| `detail-provider` | details, plugin-details | missing |
| `diagnostics-provider` | editor-diagnostics, plugin-diagnostics | missing |
| `document-indexer` | indexing, plugin-indexing | missing |
| `endpoints-provider` | endpoints, plugin-endpoints | missing |
| `event-handler` | plugin | missing |
| `file-actions` | file-actions-provider, plugin-file-actions | missing |
| `hover-provider` | hover, plugin-hover | missing |
| `injections` | language-injections, plugin-injections | missing |
| `inlay-hints-provider` | inlay-hints, plugin-inlay-hints | missing |
| `language-features` | editor-language-features, plugin-language-features | missing |
| `lifecycle` | plugin | missing |
| `mcp-provider` | mcp-tools, plugin-mcp | missing |
| `providers` | editor-providers, plugin-providers | missing |
| `run-configurations` | configurations-provider, plugin-run-configurations | missing |
| `run-target-validation` | plugin-run-target-validation, run-target-validator | missing |
| `semantic-tokens-provider` | plugin-semantic-tokens, semantic-tokens | missing |
| `syntax-provider` | plugin-syntax, syntax | missing |
| `test-results` | plugin-test-results, test-results-provider | missing |
| `view-provider` | plugin-views, views-provider | missing |

### Worlds

| World | File | Includes | Exports | Status |
| --- | --- | --- | --- | --- |
| `plugin` | `wit/plugin.wit` |  | lifecycle, commands, event-handler | missing |
| `plugin-syntax` | `wit/plugin.wit` | plugin | syntax-provider | missing |
| `plugin-providers` | `wit/plugin.wit` | plugin | providers | missing |
| `plugin-views` | `wit/plugin.wit` | plugin | view-provider | missing |
| `plugin-indexing` | `wit/plugin.wit` | plugin | document-indexer | missing |
| `plugin-semantic-tokens` | `wit/plugin.wit` | plugin | semantic-tokens-provider | missing |
| `plugin-hover` | `wit/plugin.wit` | plugin | hover-provider | missing |
| `plugin-inlay-hints` | `wit/plugin.wit` | plugin | inlay-hints-provider | missing |
| `plugin-details` | `wit/plugin.wit` | plugin | detail-provider | missing |
| `plugin-commit-checks` | `wit/plugin.wit` | plugin | commit-checker | missing |
| `plugin-file-actions` | `wit/plugin.wit` | plugin | file-actions | missing |
| `syntax` | `wit/plugin.wit` |  | syntax-provider | missing |
| `editor-providers` | `wit/plugin.wit` |  | providers | missing |
| `views-provider` | `wit/plugin.wit` |  | view-provider | missing |
| `indexing` | `wit/plugin.wit` |  | document-indexer | missing |
| `semantic-tokens` | `wit/plugin.wit` |  | semantic-tokens-provider | missing |
| `hover` | `wit/plugin.wit` |  | hover-provider | missing |
| `inlay-hints` | `wit/plugin.wit` |  | inlay-hints-provider | missing |
| `details` | `wit/plugin.wit` |  | detail-provider | missing |
| `commit-checks` | `wit/plugin.wit` |  | commit-checker | missing |
| `file-actions-provider` | `wit/plugin.wit` |  | file-actions | missing |
| `plugin-endpoints` | `wit/plugin.wit` | plugin | endpoints-provider | missing |
| `endpoints` | `wit/plugin.wit` |  | endpoints-provider | missing |
| `plugin-diagnostics` | `wit/plugin.wit` | plugin | diagnostics-provider | missing |
| `editor-diagnostics` | `wit/plugin.wit` |  | diagnostics-provider | missing |
| `plugin-run-configurations` | `wit/plugin.wit` | plugin | run-configurations | missing |
| `configurations-provider` | `wit/plugin.wit` |  | run-configurations | missing |
| `plugin-test-results` | `wit/plugin.wit` | plugin | test-results | missing |
| `test-results-provider` | `wit/plugin.wit` |  | test-results | missing |
| `plugin-language-features` | `wit/plugin.wit` | plugin | language-features | missing |
| `editor-language-features` | `wit/plugin.wit` |  | language-features | missing |
| `plugin-injections` | `wit/plugin.wit` | plugin | injections | missing |
| `language-injections` | `wit/plugin.wit` |  | injections | missing |
| `plugin-mcp` | `wit/plugin.wit` | plugin | mcp-provider | missing |
| `mcp-tools` | `wit/plugin.wit` |  | mcp-provider | missing |
| `plugin-run-target-validation` | `wit/plugin.wit` | plugin | run-target-validation | missing |
| `run-target-validator` | `wit/plugin.wit` |  | run-target-validation | missing |

## Dynamic registrations

Call sites whose id is computed at run time (plugin views, parameterised actions).
They cannot be enumerated from source; the product item is the call site itself.

| Register | Source | Example |
| --- | --- | --- |
| `registerAction` | `src/features/plugins/points.tsx` | `action` |
| `registerAction` | `src/features/plugins/points.tsx` | `{ ...action, icon: pluginIconName(pluginId, { light: svg, dark: null }` |
| `registerAction` | `src/features/settings/window/bridge.ts` | `proxy` |
| `registerAction` | `src/features/templates/index.ts` | `newFileAction(t)` |
| `registerAction` | `src/platform/actions.ts` | `action: Action` |
| `registerAction` | `src/workbench/distractionFree.ts` | `action(isDistractionFree())` |
| `registerAction` | `src/workbench/distractionFree.ts` | `action(s.active)` |
| `registerActions` | `src/editor/navigation/index.ts` | `[ editorAction("navigate.gotoTest", "Test", gotoTest, { keywords: ["go` |
| `registerActions` | `src/features/app/locale.ts` | `factory()` |
| `registerActions` | `src/features/app/locale.ts` | `factory()` |
| `registerActions` | `src/features/github/index.ts` | `actions().map((a) => ({ ...a, run: a.run ?? (async (c) => (await load(` |
| `registerActions` | `src/features/macros/index.ts` | `saved.map((m): Action => ({ id: playActionId(m.name), title: m.name, c` |
| `registerActions` | `src/features/perf/index.ts` | `perfActions()` |
| `registerActions` | `src/features/run/externalTools.ts` | `toolActions(workflowSettings().externalTools)` |
| `registerActions` | `src/features/run/externalTools.ts` | `toolActions(tools)` |
| `registerActions` | `src/features/settings/index.ts` | `zoomActions()` |
| `registerActions` | `src/features/settings/index.ts` | `appearanceToggleActions()` |
| `registerActions` | `src/features/settings/index.ts` | `navigationBarActions()` |
| `registerActions` | `src/features/settings/index.ts` | `settingsActions((page, newProjects) => void showSettings(page, newProj` |
| `registerActions` | `src/features/settings/index.ts` | `settingsActions((page, newProjects) => relaunch({ page, newProjects: !` |
| `registerActions` | `src/features/vcs/actions.tsx` | `actions` |
| `registerActions` | `src/features/vcs/historyActions.tsx` | `actions` |
| `registerActions` | `src/features/vcs/index.ts` | `openOnRemoteActions` |
| `registerActions` | `src/features/viewers/index.ts` | `[...imageActions(), ...markdownActions()]` |
| `registerActions` | `src/platform/actions.ts` | `actions: Action[]` |
| `registerToolWindow` | `src/features/agents/index.ts` | `{ id: TOOL_WINDOW, title: "Agents", icon: "toolwindow-agents", anchor:` |
| `registerToolWindow` | `src/features/graphql/index.ts` | `{ id: TOOL_WINDOW, title: "GraphQL", icon: "toolwindow-graphql", ancho` |
| `registerToolWindow` | `src/features/plugins/views/viewWindows.tsx` | `{ id, title: first.decl.title, verbatim: true, anchor: first.decl.loca` |
| `registerEditorContribution` | `src/editor/api.ts` | `c: EditorContribution` |
| `registerEditorKind` | `src/features/agents/index.ts` | `{ kind: SESSION_KIND, component: lazyComponent(() => import("./Session` |
| `registerEditorKind` | `src/features/agents/index.ts` | `{ kind: HOME_KIND, component: lazyComponent(() => import("./HomeEditor` |
| `registerEditorKind` | `src/features/build-tools/index.ts` | `{ kind: ANALYZER_KIND, component: lazyComponent(() => import("./Depend` |
| `registerEditorKind` | `src/features/database/index.ts` | `{ kind: TABLE_KIND, component: lazyComponent(() => import("./TableEdit` |
| `registerEditorKind` | `src/features/debugger/index.ts` | `heapEditorKind` |
| `registerEditorKind` | `src/features/github/index.ts` | `{ kind: TIMELINE_KIND, component: lazyComponent(() => import("./timeli` |
| `registerEditorKind` | `src/features/plugins/uiPoints.tsx` | `{ kind: CUSTOM_EDITOR_KIND, component: CustomEditor, title: (input) =>` |
| `registerEditorKind` | `src/features/plugins/uiPoints.tsx` | `{ kind: PLUGIN_LOG_EDITOR_KIND, component: PluginLogEditor, keepAlive:` |
| `registerEditorKind` | `src/features/plugins/uiPoints.tsx` | `{ kind: PLUGIN_CONTENT_EDITOR_KIND, component: PluginContentEditor, ke` |
| `registerEditorKind` | `src/features/services/index.ts` | `{ kind: DOCKER_LOG_EDITOR_KIND, component: DockerLogEditor, title: doc` |
| `registerEditorKind` | `src/features/vcs/index.ts` | `{ kind: DIFF_KIND, standalone: true, component: lazyComponent(() => im` |
| `registerEditorKind` | `src/features/vcs/index.ts` | `{ kind: MERGE_KIND, component: lazyComponent(() => import("./MergeEdit` |
| `registerEditorKind` | `src/features/vcs/index.ts` | `{ kind: LOCAL_HISTORY_KIND, component: lazyComponent(() => import("./L` |
| `registerEditorKind` | `src/features/vcs/index.ts` | `{ kind: SELECTION_HISTORY_KIND, component: lazyComponent(() => import(` |
| `registerEditorKind` | `src/features/viewers/index.ts` | `{ kind: IMAGE_KIND, component: ImageViewer, title: titleOf, icon: icon` |
| `registerEditorKind` | `src/features/viewers/index.ts` | `{ kind: BINARY_KIND, component: HexViewer, title: titleOf, icon: iconO` |
| `registerEditorKind` | `src/features/whats-new/index.ts` | `{ kind: WHATS_NEW_KIND, component: WhatsNewPage, title: () => t("app.w` |
| `registerFileEditorProvider` | `src/workbench/contributions.ts` | `p: FileEditorProvider` |
| `registerSettingsPage` | `src/features/plugins/index.ts` | `{ id: SETTINGS_PAGE_ID, title: "Plugins", // After Editor, as in Intel` |
| `registerSettingsPage` | `src/features/plugins/settings.tsx` | `{ id, title: decl.title, projectScope: true, keywords: ["plugin", plug` |
| `registerSettingsPage` | `src/features/settings/iconThemePages.tsx` | `{ id: page, title: theme.label, keywords: keywords(theme, details), co` |
| `registerSettingsPage` | `src/features/settings/pages/registry.tsx` | `toContribution(spec)` |
| `registerSettingsPage` | `src/features/settings/pages/registry.tsx` | `toContribution(UPDATES_PAGE)` |
| `registerSettingsPage` | `src/features/settings/pages/registry.tsx` | `toContribution(spec)` |
| `registerSettingsPage` | `src/features/tools/index.ts` | `{ id: SETTINGS_PAGE_ID, title: "Language Servers", group: "Languages &` |
| `registerSettingsPage` | `src/workbench/contributions.ts` | `page: SettingsPageContribution` |

## What this means for GoIDEA

The matrix is the port backlog, ordered by product surface rather than by what
Mayoi happens to contain. An action, tool window or page is not ported because a
similar Mayoi one exists; it is ported when the OpenIDEA interaction behind it
works in GoIDEA and a gate check proves it. Backend work goes upstream through the
OpenIDEA fork and a submodule bump, never by patching the submodule in place.

