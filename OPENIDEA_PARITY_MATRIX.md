# OpenIDEA parity matrix

OpenIDEA is the product specification, Mayoi/MyGo is the frontend base and the
OpenIDEA Rust core is the engine. This document enumerates everything the pinned
OpenIDEA source registers and maps each item to GoIDEA. It is generated from
`55142826`; `scripts/openidea-parity.py --check` fails when it is stale.

## How to read the status

| Status | Meaning |
| --- | --- |
| `integrated` | GoIDEA has the feature and a gate check proves it. |
| `partial` | Part of the feature exists in GoIDEA, with evidence. |
| `engine-only` | The engine behind it is reachable over stdio, but no GoIDEA UI proof exists. |
| `blocked` | It needs an OpenIDEA change (an `ops` module, or a Host/crate change) first. |
| `missing` | No GoIDEA implementation is recorded. |

`missing` is the default and is not a claim that no code exists anywhere: it means
no evidence has been recorded. The only place a status is raised is
`scripts/openidea-parity-status.json`, and every raised status must name its
evidence (a regression check id, a doc or a source path). Nothing is hand-picked
out of the inventory: the item lists below are generated from the source, and a
new action, tool window, page or command appears automatically.

## Inventory

| Kind | Items | Integrated | Partial | Engine | Blocked | Missing |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Feature folders | 43 | 0 | 11 | 0 | 0 | 32 |
| Actions | 647 | 31 | 64 | 0 | 1 | 551 |
| Keymap presets | 6 | 0 | 0 | 0 | 0 | 6 |
| Tool windows | 30 | 8 | 5 | 0 | 0 | 17 |
| Overlays | 56 | 1 | 0 | 0 | 0 | 55 |
| Editor contributions | 35 | 0 | 0 | 0 | 0 | 35 |
| Editor kinds | 4 | 0 | 0 | 0 | 0 | 4 |
| File editor providers | 4 | 0 | 0 | 0 | 0 | 4 |
| Settings pages | 63 | 0 | 1 | 0 | 0 | 62 |
| Settings keys | 445 | 0 | 0 | 0 | 0 | 445 |
| Languages | 80 | 1 | 0 | 0 | 0 | 79 |
| Backend crates | 38 | 0 | 0 | 31 | 0 | 7 |
| Events | 30 | 0 | 0 | 0 | 0 | 30 |
| Plugin worlds | 37 | 0 | 0 | 0 | 0 | 37 |
| Plugin exports | 21 | 0 | 0 | 0 | 0 | 21 |
| IPC commands | 624 | - | - | - | - | see [OPENIDEA_IPC.md](OPENIDEA_IPC.md) |

## Frontend feature folders

Every directory under `src/features` (43). Counts are the contributions that
directory registers.

| Feature | Actions | Tool windows | Overlays | Editor | Settings pages | Status |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| `agents` | 7 | 0 | 0 | 0 | 0 | missing |
| `analysis` | 6 | 2 | 1 | 0 | 0 | missing |
| `app` | 2 | 0 | 6 | 0 | 0 | missing |
| `build` | 6 | 1 | 0 | 0 | 0 | missing |
| `build-tools` | 21 | 2 | 1 | 2 | 0 | missing |
| `code-insight` | 6 | 3 | 1 | 0 | 0 | partial - lsp.references.count, lsp.symbols |
| `color-preview` | 0 | 0 | 0 | 1 | 0 | missing |
| `coverage` | 6 | 1 | 1 | 1 | 0 | missing |
| `database` | 40 | 1 | 1 | 1 | 0 | partial - UPSTREAM_FEATURE_MATRIX.md database console; db_* commands |
| `debugger` | 32 | 1 | 1 | 1 | 0 | partial - debug.paused, debug.step.paused, debug.vars |
| `dependencies` | 0 | 0 | 0 | 1 | 0 | missing |
| `deployment` | 11 | 1 | 1 | 0 | 0 | missing |
| `dotnet` | 11 | 1 | 0 | 0 | 0 | missing |
| `github` | 6 | 1 | 1 | 1 | 1 | missing |
| `graphql` | 6 | 0 | 1 | 1 | 0 | missing |
| `http-client` | 10 | 0 | 0 | 3 | 0 | missing |
| `libraries` | 1 | 0 | 0 | 0 | 0 | missing |
| `logcat` | 0 | 1 | 0 | 0 | 0 | missing |
| `macros` | 5 | 0 | 1 | 1 | 0 | missing |
| `mcp` | 0 | 0 | 1 | 0 | 0 | missing |
| `notebook` | 0 | 0 | 0 | 0 | 0 | missing |
| `perf` | 0 | 0 | 1 | 0 | 0 | missing |
| `plugins` | 10 | 0 | 2 | 5 | 0 | missing |
| `profiler` | 2 | 1 | 0 | 0 | 0 | missing |
| `project-tree` | 31 | 1 | 3 | 0 | 0 | partial - internal/workspace tree tests; base.tabs |
| `project-trust` | 5 | 0 | 1 | 0 | 1 | missing |
| `remote-dev` | 4 | 1 | 1 | 0 | 0 | missing |
| `run` | 27 | 1 | 5 | 2 | 0 | partial - base.run.config, base.run.exit, base.run.build |
| `run-anything` | 1 | 0 | 1 | 0 | 0 | partial - internal/app/run_anything.go Run Anything popup with Recent and Run Configurations sections, Clear History, per-project history and shell command lines; gear menu action:runAnything.show; Ctrl Ctrl keymap deferred, MyGo has no KeyControl constant; internal/app/run_anything_test.go |
| `scratch` | 1 | 0 | 1 | 0 | 0 | missing |
| `search` | 24 | 3 | 6 | 2 | 0 | partial - base.search.done, base.search.files, base.search.matches |
| `services` | 20 | 1 | 1 | 2 | 0 | missing |
| `settings` | 11 | 0 | 3 | 0 | 56 | partial - base.settings.opened, base.settings.closed, base.settings.theme |
| `ssh` | 2 | 0 | 1 | 0 | 0 | missing |
| `templates` | 5 | 0 | 3 | 1 | 3 | missing |
| `terminal` | 14 | 1 | 0 | 0 | 0 | partial - internal/app/term.go tab groups over backend PTYs, SplitTerm/UnsplitTerm, ShowTermFind scrollback search, per-tab channel routing and exit by id; base.term.output, base.term.tabs, base.term.split, base.term.find; UPSTREAM_FEATURE_MATRIX.md Terminal |
| `todo` | 2 | 1 | 0 | 1 | 1 | missing |
| `tools` | 4 | 0 | 0 | 0 | 0 | partial - tools.jdtls, tools.java-debug |
| `vcs` | 33 | 3 | 2 | 2 | 1 | partial - base.vcs.changes, base.diff.rows, base.commit.id, base.log.commits, base.log.details, base.log.changes; internal/app/branch.go branchLabel and branchesDialog, base.branch.head, base.branch.list, base.branch.checkout; internal/app/vcs.go vcsStaged, vcsUnstaged, StageChange, UnstageChange, AskRollback, base.stage.roundtrip, base.stage.discard |
| `viewers` | 0 | 0 | 0 | 1 | 0 | missing |
| `vim` | 3 | 0 | 0 | 1 | 0 | missing |
| `web` | 2 | 0 | 0 | 0 | 0 | missing |
| `welcome` | 2 | 0 | 1 | 0 | 0 | missing |

Infrastructure folders (`src/editor`, `src/workbench`, `src/platform`, `src/ui`)
are not product features; the contributions they host are listed below.

## Actions

455 registered action ids, plus 192 ids that only presets bind (374 in total).
Registered ids are listed by feature folder; keymap-only ids follow.

### agents

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `agents.addAgent` | More Agents… | `src/features/agents/index.ts` | missing |
| `agents.findInConversation` | Find in Conversation | `src/features/agents/index.ts` | missing |
| `agents.mcpServers` | External MCP Servers… | `src/features/agents/index.ts` | missing |
| `agents.newTask` | New Agent Task | `src/features/agents/index.ts` | missing |
| `agents.openSampleConversations` | Open Sample Conversations | `src/features/agents/index.ts` | missing |
| `agents.sendToAgent` | Send to Agent | `src/features/agents/index.ts` | missing |
| `agents.showToolWindow` | Agents | `src/features/agents/index.ts` | missing |

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
| `app.restart` | Restart IDE | `src/features/app/InvalidateCaches.tsx` | missing |
| `file.invalidateCaches` | Invalidate Caches… | `src/features/app/InvalidateCaches.tsx` | missing |

### build

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `build.buildArtifacts` | Build Artifacts | `src/features/build/index.ts` | partial - internal/app/build.go BuildArtifacts (build_run kind=artifacts) |
| `build.buildModule` | Build Module | `src/features/build/index.ts` | partial - internal/app/build.go BuildModule; base.build.success |
| `build.buildProject` | Build Project | `src/features/build/index.ts` | integrated - base.build.success, base.build.lines, base.build.console |
| `build.compileFile` | Compile File | `src/features/build/index.ts` | blocked - build_compile_file is not served by the pinned v0.1.41 server; OPENIDEA_IPC.md |
| `build.rebuildProject` | Rebuild Project | `src/features/build/index.ts` | partial - internal/app/build.go RebuildProject (build_run kind=rebuild) |
| `build.stop` | Stop Build | `src/features/build/index.ts` | partial - internal/app/build.go StopBuild (cancel_request) |

### build-tools

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `gradle.analyzeDependencies` | Analyze Dependencies… | `src/features/build-tools/index.ts` | missing |
| `gradle.cancelReload` | Stop Reloading Gradle Projects | `src/features/build-tools/index.ts` | missing |
| `gradle.collapseAll` | Collapse All | `src/features/build-tools/index.ts` | missing |
| `gradle.executeTask` | Execute Gradle Task… | `src/features/build-tools/index.ts` | missing |
| `gradle.expandAll` | Expand All | `src/features/build-tools/index.ts` | missing |
| `gradle.linkProjects` | Link Gradle Projects | `src/features/build-tools/index.ts` | missing |
| `gradle.reload` | Reload All Gradle Projects | `src/features/build-tools/index.ts` | missing |
| `gradle.toggleOffline` | Toggle Offline Mode | `src/features/build-tools/index.ts` | missing |
| `gradle.unlinkProjects` | Unlink Gradle Project | `src/features/build-tools/index.ts` | missing |
| `maven.addProjects` | Add Maven Projects | `src/features/build-tools/index.ts` | missing |
| `maven.analyzeDependencies` | Analyze Dependencies… | `src/features/build-tools/index.ts` | missing |
| `maven.cancelReload` | Stop Reloading Maven Projects | `src/features/build-tools/index.ts` | missing |
| `maven.collapseAll` | Collapse All | `src/features/build-tools/index.ts` | missing |
| `maven.executeGoal` | Execute Maven Goal… | `src/features/build-tools/index.ts` | missing |
| `maven.expandAll` | Expand All | `src/features/build-tools/index.ts` | missing |
| `maven.generateSources` | Generate Sources and Update Folders For All Projects | `src/features/build-tools/index.ts` | missing |
| `maven.reload` | Reload All Maven Projects | `src/features/build-tools/index.ts` | missing |
| `maven.settings` | Maven Settings… | `src/features/build-tools/index.ts` | missing |
| `maven.toggleOffline` | Toggle Offline Mode | `src/features/build-tools/index.ts` | missing |
| `maven.toggleSkipTests` | Toggle 'Skip Tests' Mode | `src/features/build-tools/index.ts` | missing |
| `maven.unlinkProjects` | Unlink Maven Projects | `src/features/build-tools/index.ts` | missing |

### code-insight

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `find.showUsageResults` | Show Usage Results | `src/features/code-insight/index.ts` | missing |
| `find.showUsages` | Show Usages | `src/features/code-insight/index.ts` | missing |
| `find.usages` | Find Usages | `src/features/code-insight/index.ts` | integrated - lsp.references.count, lsp.references.done |
| `navigate.callHierarchy` | Call Hierarchy | `src/features/code-insight/index.ts` | missing |
| `navigate.typeHierarchy` | Type Hierarchy | `src/features/code-insight/index.ts` | missing |
| `usages.rerun` | Rerun Find Usages | `src/features/code-insight/index.ts` | missing |

### coverage

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `coverage.hide` | Hide Coverage | `src/features/coverage/index.ts` | missing |
| `coverage.importReport` | Import a Coverage Report… | `src/features/coverage/index.ts` | missing |
| `coverage.manageReports` | Manage Coverage Reports… | `src/features/coverage/index.ts` | missing |
| `coverage.show` | Show Coverage | `src/features/coverage/index.ts` | missing |
| `run.coverage` | Run with Coverage | `src/features/coverage/index.ts` | missing |
| `run.coverageContext` | Run Context Configuration with Coverage | `src/features/coverage/index.ts` | missing |

### database

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `db.collapseAll` | Collapse All | `src/features/database/treeActions.ts` | missing |
| `db.copyReference` | Copy Reference | `src/features/database/treeActions.ts` | missing |
| `db.ddlToClipboard` | Generate DDL to Clipboard | `src/features/database/treeActions.ts` | missing |
| `db.ddlToConsole` | Generate DDL to Query Console | `src/features/database/treeActions.ts` | missing |
| `db.disconnect` | Deactivate | `src/features/database/treeActions.ts` | missing |
| `db.drop` | Drop… | `src/features/database/treeActions.ts` | missing |
| `db.duplicateSource` | Duplicate | `src/features/database/treeActions.ts` | missing |
| `db.editData` | Edit Data | `src/features/database/treeActions.ts` | missing |
| `db.execute` | Execute SQL Statement | `src/features/database/index.ts` | missing |
| `db.explainPlan` | Explain Plan | `src/features/database/index.ts` | missing |
| `db.findInValue` | Find | `src/features/database/valueEditorActions.ts` | missing |
| `db.goToColumn` | Go to Column | `src/features/database/index.ts` | missing |
| `db.goToDdl` | Go to DDL | `src/features/database/treeActions.ts` | missing |
| `db.goToReferencedTable` | Go to Referenced Table | `src/features/database/treeActions.ts` | missing |
| `db.importData` | Import Data from File… | `src/features/database/treeActions.ts` | missing |
| `db.jumpToConsole` | Jump to Query Console… | `src/features/database/treeActions.ts` | missing |
| `db.modifyTable` | Modify Table… | `src/features/database/treeActions.ts` | missing |
| `db.new` | New… | `src/features/database/treeActions.ts` | missing |
| `db.newColumn` | Column | `src/features/database/treeActions.ts` | missing |
| `db.newDataSource` | New Data Source… | `src/features/database/index.ts` | missing |
| `db.newTable` | Table | `src/features/database/treeActions.ts` | missing |
| `db.openConsole` | Query Console | `src/features/database/index.ts` | missing |
| `db.openValueWindow` | Open in Window | `src/features/database/valueEditorActions.ts` | missing |
| `db.pluginConnect` | Connect Plugin Data Source | `src/features/database/index.ts` | missing |
| `db.properties` | Data Source Properties… | `src/features/database/treeActions.ts` | missing |
| `db.queryHistory` | Browse Query History | `src/features/database/index.ts` | missing |
| `db.quickDoc` | Quick Documentation | `src/features/database/treeActions.ts` | missing |
| `db.reformatValue` | Reformat | `src/features/database/valueEditorActions.ts` | missing |
| `db.refresh` | Refresh | `src/features/database/treeActions.ts` | missing |
| `db.rename` | Rename… | `src/features/database/treeActions.ts` | missing |
| `db.revertChanges` | Revert | `src/features/database/index.ts` | missing |
| `db.runSqlScript` | Run SQL Script… | `src/features/database/treeActions.ts` | missing |
| `db.separateProceduresAndFunctions` | Separate Procedures and Functions | `src/features/database/treeActions.ts` | missing |
| `db.setCellDefault` | Set DEFAULT | `src/features/database/cellActions.ts` | missing |
| `db.setCellNull` | Set NULL | `src/features/database/cellActions.ts` | missing |
| `db.showToolWindow` | Database | `src/features/database/index.ts` | missing |
| `db.sqlScript` | SQL Script Template | `src/features/database/treeActions.ts` | missing |
| `db.submitChanges` | Submit | `src/features/database/index.ts` | missing |
| `db.toggleBooleanCell` | Toggle Boolean Cell | `src/features/database/cellActions.ts` | missing |
| `db.truncate` | Truncate… | `src/features/database/treeActions.ts` | missing |

### debugger

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `debug.attach` | Attach Debugger | `src/features/debugger/actions.ts` | missing |
| `debug.attachToProcess` | Attach to Process… | `src/features/debugger/actions.ts` | missing |
| `debug.browseConsoleHistory` | Browse Console History | `src/features/debugger/actions.ts` | missing |
| `debug.cancelHotSwap` | Cancel Hot Swap Build | `src/features/debugger/actions.ts` | missing |
| `debug.evaluate` | Evaluate Expression… | `src/features/debugger/actions.ts` | missing |
| `debug.forceRunToCursor` | Force Run to Cursor | `src/features/debugger/actions.ts` | missing |
| `debug.forceStepInto` | Force Step Into | `src/features/debugger/actions.ts` | missing |
| `debug.forceStepOver` | Force Step Over | `src/features/debugger/actions.ts` | missing |
| `debug.hotReload` | Hot Reload | `src/features/debugger/actions.ts` | missing |
| `debug.hotRestart` | Hot Restart | `src/features/debugger/actions.ts` | missing |
| `debug.jumpToSource` | Jump to Source | `src/features/debugger/actions.ts` | missing |
| `debug.muteBreakpoints` | Mute Breakpoints | `src/features/debugger/actions.ts` | missing |
| `debug.pause` | Pause Program | `src/features/debugger/actions.ts` | missing |
| `debug.pluginStart` | Debug Plugin Configuration | `src/features/debugger/actions.ts` | missing |
| `debug.quickEvaluate` | Quick Evaluate Expression | `src/features/debugger/actions.ts` | missing |
| `debug.reloadChangedClasses` | Reload Changed Classes | `src/features/debugger/actions.ts` | missing |
| `debug.removeAllBreakpoints` | Remove All Breakpoints | `src/features/debugger/actions.ts` | missing |
| `debug.rerun` | Rerun | `src/features/debugger/actions.ts` | missing |
| `debug.resetFrame` | Reset Frame | `src/features/debugger/actions.ts` | missing |
| `debug.resume` | Resume Program | `src/features/debugger/actions.ts` | missing |
| `debug.runToCursor` | Run to Cursor | `src/features/debugger/actions.ts` | missing |
| `debug.setValue` | Set Value… | `src/features/debugger/actions.ts` | missing |
| `debug.showExecutionPoint` | Show Execution Point | `src/features/debugger/actions.ts` | missing |
| `debug.showToolWindow` | Show Debug Tool Window | `src/features/debugger/actions.ts` | missing |
| `debug.smartStepInto` | Smart Step Into | `src/features/debugger/actions.ts` | missing |
| `debug.startConfig` | Debug… | `src/features/debugger/actions.ts` | missing |
| `debug.stop` | Stop | `src/features/debugger/actions.ts` | missing |
| `debug.toggleBreakpoint` | Toggle Line Breakpoint | `src/features/debugger/actions.ts` | missing |
| `debug.toggleFieldWatchpoint` | Toggle Field Watchpoint | `src/features/debugger/actions.ts` | missing |
| `debug.toggleMethodBreakpoint` | Toggle Method Breakpoint | `src/features/debugger/actions.ts` | missing |
| `debug.toggleTemporaryBreakpoint` | Toggle Temporary Line Breakpoint | `src/features/debugger/actions.ts` | missing |
| `debug.viewBreakpoints` | View Breakpoints… | `src/features/debugger/actions.ts` | missing |

### deployment

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `deployment.browseRemoteHost` | Browse Remote Host | `src/features/deployment/actions.tsx` | missing |
| `deployment.configure` | Deployment Configuration… | `src/features/deployment/actions.tsx` | missing |
| `deployment.deleteFromServer` | Delete from Server | `src/features/deployment/actions.tsx` | missing |
| `deployment.downloadFrom` | Download from… | `src/features/deployment/actions.tsx` | missing |
| `deployment.downloadFromDefault` | Download from Default Server | `src/features/deployment/actions.tsx` | missing |
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
| `github.pullRequest.reviewMode.showDiff` | Show Review Diff | `src/features/github/reviewMode/index.ts` | missing |

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

### plugins

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `plugins.content.action` | Run Content Editor Action | `src/features/plugins/views/contentActions.ts` | missing |
| `plugins.content.find` | Find in Editor | `src/features/plugins/views/contentActions.ts` | missing |
| `plugins.importInstalledIntellijTheme` | Import Theme from JetBrains IDE… | `src/features/plugins/index.ts` | missing |
| `plugins.importIntellijTheme` | Import IntelliJ Theme from File… | `src/features/plugins/index.ts` | missing |
| `plugins.installFromDisk` | Install Plugin from Disk… | `src/features/plugins/index.ts` | missing |
| `plugins.log.${operation}` |  | `src/features/plugins/views/logActions.ts` | missing |
| `plugins.log.toggleArgument` | Toggle Log Option | `src/features/plugins/views/logActions.ts` | missing |
| `plugins.reloadAll` | Reload Plugins | `src/features/plugins/index.ts` | missing |
| `plugins.settings` | Plugins… | `src/features/plugins/index.ts` | missing |
| `plugins.updateAll` | Update All Plugins | `src/features/plugins/index.ts` | missing |

### profiler

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `run.profile` | Profile | `src/features/profiler/index.ts` | missing |
| `run.profileContext` | Profile Context Configuration | `src/features/profiler/index.ts` | missing |

### project-tree

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `file.new` | New… | `src/features/project-tree/actions.ts` | missing |
| `navigate.selectIn` | Select In… | `src/features/project-tree/actions.ts` | partial - Alt+F1 reveals the active file in the project view; the Select In popup's other target is Reveal in Finder, which GoIDEA leaves out; base.files.select |
| `navigate.selectInProjectView` | Select in Project View | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go SelectInProjectView reveals the active tab and marks its row; base.files.select |
| `navigate.showFilePath` | File Path | `src/features/project-tree/actions.ts` | missing |
| `project.collapseAll` | Collapse All | `src/features/project-tree/actions.ts` | partial - internal/app/tree.go CollapseAll leaves only the root row; base.files.expand |
| `project.compareWith` | Compare With… | `src/features/project-tree/actions.ts` | missing |
| `project.copy` | Copy | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go SetTreeClip; base.files.menu, base.files.copy; internal/app/tree_ops_test.go |
| `project.copyElement` | Copy… | `src/features/project-tree/actions.ts` | missing |
| `project.copyPath` | Copy Path | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go CopyTreePath with absolute, file name and content-root paths; base.files.menu |
| `project.cut` | Cut | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go SetTreeClip; base.files.menu, base.files.move; internal/app/tree_ops_test.go |
| `project.delete` | Delete… | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go deleteTreeEntry over fs_delete after a confirmation; base.files.delete; internal/app/tree_ops_test.go |
| `project.expandAll` | Expand All | `src/features/project-tree/actions.ts` | partial - internal/app/tree.go ExpandAll opens each folder as its read arrives; base.files.expand |
| `project.findInFolder` | Find in Folder… | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go treeActFindInFolder -> internal/app/search.go ShowSearchInFolder scopes search_start to the folder; base.files.findInFolder; replace in folder is missing |
| `project.findUsages` | Find Usages of File | `src/features/project-tree/actions.ts` | missing |
| `project.move` | Move File… | `src/features/project-tree/actions.ts` | missing |
| `project.newFile` | New File | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go treeMenuItems OpenNewTreeEntry; base.files.menu, base.files.new; internal/app/tree_ops_test.go |
| `project.newFolder` | New Directory | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go treeMenuItems OpenNewTreeEntry; base.files.menu, base.files.new; internal/app/tree_ops_test.go |
| `project.newInThisDirectory` | New in This Directory… | `src/features/project-tree/actions.ts` | missing |
| `project.open` | Jump to Source | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go runTreeAct treeActJump -> openFile; base.files.menu; internal/app/tree_ops_test.go |
| `project.openInTerminal` | Open in Terminal | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go treeActTerminal -> OpenTerminalAt in internal/app/term.go; base.files.menu, base.term.output |
| `project.optimizeImports` | Optimize Imports in File | `src/features/project-tree/actions.ts` | missing |
| `project.paste` | Paste | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go PasteTreeEntry over fs_copy and fs_rename; base.files.copy, base.files.move |
| `project.reformatCode` | Reformat File | `src/features/project-tree/actions.ts` | missing |
| `project.refresh` | Reload Project Tree from Disk | `src/features/project-tree/actions.ts` | integrated - internal/app/tree_ops.go ReloadProjectTree -> internal/app/tree.go Refresh -> workspace.Tree.Refresh rereads every folder read so far, from the palette's 从磁盘重新加载项目树; base.files.expand |
| `project.reloadFromDisk` | Reload from Disk | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go ReloadPathFromDisk -> internal/app/tree.go RefreshDir -> workspace.Tree.RefreshDir rereads the selected folder and the read folders below it, from the popup and Cmd+Alt+Y; base.files.reload; re-reading a selected file's open document is missing |
| `project.rename` | Rename File… | `src/features/project-tree/actions.ts` | partial - internal/app/tree_ops.go ConfirmTreeOp over fs_rename, moving bookmarks and closing clean tabs; base.files.rename; internal/app/tree_ops_test.go |
| `project.renameInline` | Rename File Inline | `src/features/project-tree/actions.ts` | missing |
| `project.reveal` | Reveal in Project View | `src/features/project-tree/actions.ts` | partial - internal/app/tree.go reveal plus Select; base.files.select |
| `project.revealInFinder` |  | `src/features/project-tree/actions.ts` | missing |
| `project.showHistory` | Local History: Show History | `src/features/project-tree/actions.ts` | partial - internal/app/history.go ShowHistory over history_list_dir and RestoreHistoryEntry over history_restore; base.files.historyRestore |
| `project.undoMove` | Undo Move / Rename | `src/features/project-tree/actions.ts` | missing |

### project-trust

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `project.dontOpen` | Don't Open | `src/features/project-trust/index.tsx` | missing |
| `project.previewRestricted` | Preview in Restricted Mode | `src/features/project-trust/index.tsx` | missing |
| `project.removeTrustedLocation` | Remove Trusted Location | `src/features/project-trust/index.tsx` | missing |
| `project.revokeTrust` | Revoke Trust | `src/features/project-trust/index.tsx` | missing |
| `project.trust` | Trust Project | `src/features/project-trust/index.tsx` | missing |

### remote-dev

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `remote.connect` | Remote Development… | `src/features/remote-dev/index.ts` | missing |
| `remote.disconnect` | Disconnect from Remote Host | `src/features/remote-dev/index.ts` | missing |
| `remote.reloadEnvironment` | Reload Environment | `src/features/remote-dev/index.ts` | missing |
| `remote.reopenInContainer` | Reopen in Container | `src/features/remote-dev/index.ts` | missing |

### run

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `analyze.stackTrace` | Analyze Stack Trace or Thread Dump… | `src/features/run/stackTrace.tsx` | missing |
| `go.generate.${scope}` |  | `src/features/run/goActions.ts` | missing |
| `php.composerInstall` | Composer Install | `src/features/run/composerInstall.ts` | missing |
| `run.chooseConfig` | Select Run/Debug Configuration… | `src/features/run/index.ts` | missing |
| `run.chooseDebugConfig` | Debug… | `src/features/run/index.ts` | missing |
| `run.createConfig` | Create Run Configuration | `src/features/run/index.ts` | missing |
| `run.debug` | Debug | `src/features/run/index.ts` | integrated - debug.paused, debug.step.paused |
| `run.debugConfig` | Debug Configuration | `src/features/run/index.ts` | missing |
| `run.debugContext` | Debug Context Configuration | `src/features/run/index.ts` | missing |
| `run.editConfigs` | Edit Configurations… | `src/features/run/index.ts` | missing |
| `run.importJetbrains` | Import Run Configurations from JetBrains (.run, .idea) | `src/features/run/index.ts` | missing |
| `run.importVscodeLaunch` | Import Run Configurations from .vscode/launch.json | `src/features/run/index.ts` | missing |
| `run.modifyContext` | Modify Run Configuration… | `src/features/run/index.ts` | missing |
| `run.pluginStart` | Run Plugin Configuration | `src/features/run/index.ts` | missing |
| `run.reloadConfigs` | Reload Run Configurations | `src/features/run/index.ts` | missing |
| `run.rerun` | Rerun | `src/features/run/index.ts` | missing |
| `run.rerunFailed` | Rerun Failed Tests | `src/features/run/index.ts` | missing |
| `run.run` | Run | `src/features/run/index.ts` | integrated - base.run.exit, base.run.build |
| `run.runConfig` | Run Configuration | `src/features/run/index.ts` | missing |
| `run.runContext` | Run Context Configuration | `src/features/run/index.ts` | missing |
| `run.sampleOutput` | Stream Sample Output | `src/features/run/index.ts` | missing |
| `run.services.addType` | Add Service | `src/features/run/services.tsx` | missing |
| `run.services.hideConfig` | Remove from Services | `src/features/run/services.tsx` | missing |
| `run.stop` | Stop | `src/features/run/index.ts` | missing |
| `run.stopProcess` | Stop | `src/features/run/index.ts` | missing |
| `run.updateApplication` | Update Application | `src/features/run/index.ts` | missing |
| `run.withExecutor` | Run with Executor | `src/features/run/index.ts` | missing |

### run-anything

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `runAnything.show` | Run Anything… | `src/features/run-anything/index.ts` | integrated - internal/app/commands.go runAnything.show opens the popup; gear menu first item in internal/app/menu.go; internal/app/run_anything_test.go TestRunAnythingDialogRenders runs the action through the registry |

### scratch

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `file.newScratchFile` | New Scratch File… | `src/features/scratch/index.ts` | missing |

### search

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `bookmarks.newList` | New Bookmark List… | `src/features/search/index.ts` | partial - internal/app/bookmarks.go addBookmarkList, renameBookmarkList, removeBookmarkList and newListDialog; tool-window chips 重命名/删除; the move chooser's 新建列表… creates a list and moves the bookmark into it; cmd/goidea/main.go 新建书签列表…; base.bookmark.list, base.bookmark.listOps, base.bookmark.move; internal/app/bookmarks_test.go |
| `bookmarks.toggleFileBookmark` | Toggle File Bookmark | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleFileBookmark and ToggleFolderBookmark with PathBookmark.Dir, pathBookmarkLabel; project tree rows chip 书签 in internal/app/view.go; cmd/goidea/main.go 切换文件书签; base.bookmark.file, base.bookmark.folder; internal/app/bookmarks_test.go |
| `find.inPath` | Find in Files… | `src/features/search/index.ts` | missing |
| `find.replaceInPath` | Replace in Files… | `src/features/search/index.ts` | missing |
| `find.rerun` | Rerun Search | `src/features/search/index.ts` | missing |
| `navigate.fileStructure` | File Structure | `src/features/search/index.ts` | missing |
| `navigate.findAction` | Find Action… | `src/features/search/index.ts` | integrated - base.commands |
| `navigate.gotoBookmark${d}` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go JumpBookmarkMnemonic bound to Ctrl+digit for 0-9; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoClass` | Go to Class… | `src/features/search/index.ts` | missing |
| `navigate.gotoFile` | Go to File… | `src/features/search/index.ts` | integrated - internal/app/quickopen_test.go |
| `navigate.gotoSymbol` | Go to Symbol… | `src/features/search/index.ts` | missing |
| `navigate.nextBookmark` | Next Bookmark | `src/features/search/index.ts` | partial - internal/app/bookmarks.go NextBookmark and stepBookmark wrap; base.bookmark.next; internal/app/bookmarks_test.go |
| `navigate.prevBookmark` | Previous Bookmark | `src/features/search/index.ts` | partial - internal/app/bookmarks.go PrevBookmark and stepBookmark wrap; base.bookmark.prev; internal/app/bookmarks_test.go |
| `navigate.recentFiles` | Recent Files | `src/features/search/index.ts` | missing |
| `navigate.recentLocations` | Recent Locations | `src/features/search/index.ts` | missing |
| `navigate.searchEverywhere` | Search Everywhere | `src/features/search/index.ts` | missing |
| `navigate.searchText` | Search Text Everywhere | `src/features/search/index.ts` | integrated - base.search.done, base.search.files, base.search.matches |
| `navigate.showBookmarks` | Show Bookmarks | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ShowBookmarks popup groups files and lines by list with jump and delete; base.bookmark.popup; Cmd+F3 in internal/app/app.go; internal/app/bookmarks_test.go |
| `navigate.switcher` | Switcher | `src/features/search/index.ts` | missing |
| `navigate.switcherBack` | Switcher (Backward) | `src/features/search/index.ts` | missing |
| `navigate.toggleBookmark` | Toggle Bookmark | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmark at the caret and gutter dots; F3 in internal/app/app.go; base.bookmark.added, base.bookmark.gutter, base.bookmark.remove; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark${d}` |  | `src/features/search/index.ts` | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic bound to Ctrl+Shift+digit for 0-9; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmarkWithMnemonic` | Toggle Bookmark with Mnemonic | `src/features/search/index.ts` | partial - internal/app/bookmarks.go OpenBookmarkMnemonics Alt+F3 chooser over digits and A-Z; internal/app/app.go shortcut; internal/app/bookmarks_test.go |
| `search.editScopes` | Edit Scopes… | `src/features/search/index.ts` | missing |

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
| `services.show` | Services | `src/features/services/index.ts` | missing |

### settings

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `editor.colorScheme.${name}` |  | `src/features/settings/pages/editor/colorScheme.tsx` | missing |
| `file.overrideFileType` | Override File Type | `src/features/settings/fileTypesRuntime.ts` | missing |
| `file.projectStructure` | Project Structure… | `src/features/settings/index.ts` | missing |
| `file.revertFileTypeOverride` | Revert File Type Override | `src/features/settings/fileTypesRuntime.ts` | missing |
| `php.languageLevelPopup` | PHP Language Level | `src/features/settings/languages/PhpStatus.tsx` | missing |
| `php.selectLanguageLevel` | Select PHP Language Level | `src/features/settings/languages/PhpStatus.tsx` | missing |
| `python.addInterpreter` | Add New Interpreter… | `src/features/settings/languages/PythonStatus.tsx` | missing |
| `python.interpreterPopup` | Python Interpreter | `src/features/settings/languages/PythonStatus.tsx` | missing |
| `python.interpreterSettings` | Interpreter Settings… | `src/features/settings/languages/PythonStatus.tsx` | missing |
| `python.selectInterpreter` | Select Python Interpreter | `src/features/settings/languages/PythonStatus.tsx` | missing |
| `settings.assignShortcut` | Assign Shortcut… | `src/features/settings/index.ts` | missing |

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
| `templates.newFromTemplate` | New from Template… | `src/features/templates/index.ts` | missing |

### terminal

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `terminal.clear` | Clear Terminal Buffer | `src/features/terminal/index.ts` | missing |
| `terminal.closeSession` | Close Terminal Session | `src/features/terminal/index.ts` | missing |
| `terminal.find` | Find in Terminal | `src/features/terminal/index.ts` | missing |
| `terminal.new` | New Terminal Tab | `src/features/terminal/index.ts` | missing |
| `terminal.newPredefinedSession` | New Predefined Session | `src/features/terminal/index.ts` | missing |
| `terminal.openAt` | Open in Terminal | `src/features/terminal/index.ts` | missing |
| `terminal.pluginSession` | Open Plugin Terminal Session | `src/features/terminal/index.ts` | missing |
| `terminal.runCommand` | Run Command in Terminal | `src/features/terminal/index.ts` | missing |
| `terminal.sampleTui` | Stream Sample TUI | `src/features/terminal/index.ts` | missing |
| `terminal.splitAndMoveDown` | Split and Move Down | `src/features/terminal/index.ts` | missing |
| `terminal.splitAndMoveRight` | Split and Move Right | `src/features/terminal/index.ts` | missing |
| `terminal.splitDown` | Split Down | `src/features/terminal/index.ts` | missing |
| `terminal.splitRight` | Split Right | `src/features/terminal/index.ts` | missing |
| `terminal.unsplit` | Unsplit | `src/features/terminal/index.ts` | missing |

### todo

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `todo.refresh` | Refresh TODO Items | `src/features/todo/index.ts` | missing |
| `todo.showCurrentFile` | Show TODO Items in Current File | `src/features/todo/index.ts` | missing |

### tools

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `java.downloadJdk` | Download JDK… | `src/features/tools/jdk.ts` | missing |
| `tools.install` | Install Language Server / Debugger | `src/features/tools/index.ts` | missing |
| `tools.languageServers` | Language Servers… | `src/features/tools/index.ts` | missing |
| `tools.promptInstall` | Install Missing Tool | `src/features/tools/index.ts` | missing |

### vcs

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `diff.applyLeftSide` | Accept Left Side | `src/features/vcs/compare.tsx` | missing |
| `diff.applyRightSide` | Accept Right Side | `src/features/vcs/compare.tsx` | missing |
| `diff.compareDirectories` | Compare Directories | `src/features/vcs/compare.tsx` | integrated - internal/app/dircompare.go OpenDirCompare and ConfirmDirCompare over fs_compare_directories with its onProgress channel, rows opening internal/app/diff.go ShowFileDiff; base.files.compareDirs |
| `diff.compareFiles` | Compare Files | `src/features/vcs/compare.tsx` | missing |
| `diff.compareTexts` | Compare Texts | `src/features/vcs/compare.tsx` | missing |
| `diff.compareWith` | Compare With… | `src/features/vcs/compare.tsx` | missing |
| `diff.compareWithClipboard` | Compare with Clipboard | `src/features/vcs/compare.tsx` | missing |
| `diff.jumpToSource` | Jump to Source | `src/features/vcs/compare.tsx` | missing |
| `diff.nextDifference` | Next Difference | `src/features/vcs/compare.tsx` | missing |
| `diff.nextFile` | Compare Next File | `src/features/vcs/compare.tsx` | missing |
| `diff.prevDifference` | Previous Difference | `src/features/vcs/compare.tsx` | missing |
| `diff.prevFile` | Compare Previous File | `src/features/vcs/compare.tsx` | missing |
| `diff.showSettings` | Diff Settings | `src/features/vcs/compare.tsx` | missing |
| `localHistory.putLabel` | Local History: Put Label… | `src/features/vcs/localHistoryActions.ts` | partial - internal/app/history.go ConfirmHistoryLabel over history_put_label; base.files.historyLabel |
| `localHistory.recentChanges` | Recent Changes | `src/features/vcs/localHistoryActions.ts` | missing |
| `merge.${command}` |  | `src/features/vcs/mergeActions.ts` | missing |
| `vcs.log.branches.delete` | Delete Branch | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.branches.navigate` | Navigate Log to Selected Branch | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.branches.new` | New Branch… | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.branches.toggleFavorite` | Mark/Unmark As Favorite | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.branches.update` | Update Selected | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.changes.compareBeforeWithLocal` | Compare Before with Local | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.changes.compareWithLocal` | Compare with Local | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.changes.diffPreview` | Show Diff Preview | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.changes.getFromRevision` | Get from Revision | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.changes.groupByDirectory` | Group by Directory | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.changes.jumpToSource` | Jump to Source | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.changes.revert` | Revert Selected Changes | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.changes.showDiff` | Show Diff | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.collapseLinear` | Collapse Linear Branches | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.expandLinear` | Expand Linear Branches | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.log.showBranches` | Show Branches | `src/features/vcs/logPaneActions.ts` | missing |
| `vcs.openDiff` | Open Diff | `src/features/vcs/compare.tsx` | missing |

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
| `file.openRecent` | Open Recent… | `src/features/welcome/index.ts` | missing |
| `welcome.recentProjects` | Recent Projects… | `src/features/welcome/index.ts` | missing |

### infrastructure

| Action | Title | Source | Status |
| --- | --- | --- | --- |
| `code.applyPastedImports` | Add Imports | `src/editor/lsp/importsOnPaste.ts` | missing |
| `code.cleanupFile` | Code Cleanup in File | `src/editor/lsp/actions.ts` | missing |
| `code.optimizeImportsFile` | Optimize Imports in File | `src/editor/lsp/actions.ts` | missing |
| `code.reformatFile` | Reformat File | `src/editor/lsp/actions.ts` | missing |
| `editor.addOrRemoveCaret` | Add or Remove Caret | `src/editor/core/actions.ts` | missing |
| `editor.cancelSourceDownload` | Cancel | `src/editor/core/librarySources.ts` | missing |
| `editor.changeSplitOrientation` | Change Splitter Orientation | `src/editor/core/actions.ts` | missing |
| `editor.chooseSources` | Choose Sources… | `src/editor/core/librarySources.ts` | missing |
| `editor.closeAllTabs` | Close All Tabs | `src/editor/core/actions.ts` | partial - internal/app/tabs.go CloseAllTabs with Close All Tabs and Close All but Pinned in the tab context menu; internal/app/tabs_test.go |
| `editor.closeOtherTabs` | Close Other Tabs | `src/editor/core/actions.ts` | partial - internal/app/tabs.go CloseOtherTabs, CloseTabsToLeft/Right and CloseUnmodifiedTabs; internal/app/workbench.go tabMenu; internal/app/tabs_test.go |
| `editor.closeTab` | Close Tab | `src/editor/core/actions.ts` | missing |
| `editor.copyPath` | Absolute Path | `src/editor/core/actions.ts` | missing |
| `editor.copyPathReference` | Copy Path/Reference… | `src/editor/core/actions.ts` | missing |
| `editor.copyPathWithLineNumber` | Copy Path with Line Number | `src/editor/core/actions.ts` | missing |
| `editor.copyReference` | Copy Reference | `src/editor/core/actions.ts` | missing |
| `editor.decreaseFontSize` | Decrease Font Size | `src/editor/core/actions.ts` | missing |
| `editor.downloadSources` | Download Sources | `src/editor/core/librarySources.ts` | missing |
| `editor.formatDocument` | Reformat Code | `src/platform/actions.ts` | missing |
| `editor.increaseFontSize` | Increase Font Size | `src/editor/core/actions.ts` | missing |
| `editor.inlineCompletion.trigger` | Call Inline Completion | `src/editor/lsp/inlineCompletion.ts` | missing |
| `editor.moveToOppositeGroup` | Move to Opposite Group | `src/editor/core/actions.ts` | missing |
| `editor.nextSplitter` | Go to Next Splitter | `src/editor/core/actions.ts` | missing |
| `editor.nextTab` | Select Next Tab | `src/editor/core/actions.ts` | missing |
| `editor.openInOppositeGroup` | Open in Opposite Group | `src/editor/core/actions.ts` | missing |
| `editor.pasteHistory.paste` | Paste | `src/editor/core/view/PasteHistoryDialog.tsx` | missing |
| `editor.pasteHistory.remove` | Delete | `src/editor/core/view/PasteHistoryDialog.tsx` | missing |
| `editor.pinTab` | Pin / Unpin Tab | `src/editor/core/actions.ts` | partial - internal/app/tabs.go PinActiveTab/placeTab and the tab context menu in internal/app/workbench.go; pinned tabs in Session.Pinned; internal/app/tabs_test.go |
| `editor.prevSplitter` | Go to Previous Splitter | `src/editor/core/actions.ts` | missing |
| `editor.prevTab` | Select Previous Tab | `src/editor/core/actions.ts` | missing |
| `editor.reopenClosedTab` | Reopen Closed Tab | `src/editor/core/actions.ts` | missing |
| `editor.resetFontSize` | Reset Font Size in All Editors | `src/editor/core/actions.ts` | missing |
| `editor.showHint` | Show Editor Hint | `src/editor/lsp/actions.ts` | missing |
| `editor.splitDown` | Split Down | `src/editor/core/actions.ts` | missing |
| `editor.splitRight` | Split Right | `src/editor/core/actions.ts` | missing |
| `editor.toggleColumnSelection` | Column Selection Mode | `src/editor/core/actions.ts` | missing |
| `editor.toggleSoftWrap` | Soft-Wrap | `src/editor/core/actions.ts` | missing |
| `editor.unsplit` | Unsplit | `src/editor/core/actions.ts` | missing |
| `editor.unsplitAll` | Unsplit All | `src/editor/core/actions.ts` | missing |
| `file.closeProject` | Close Project | `src/workbench/shellActions.ts` | missing |
| `file.openProject` | Open… | `src/workbench/shellActions.ts` | missing |
| `file.reloadAllFromDisk` | Reload All from Disk | `src/editor/core/actions.ts` | missing |
| `file.reloadFromDisk` | Reload from Disk | `src/editor/core/actions.ts` | missing |
| `file.save` | Save All | `src/editor/core/actions.ts` | missing |
| `file.toggleReadOnly` | Toggle Read-only Attribute | `src/editor/core/actions.ts` | missing |
| `find.field.newLine` | New Line in Search Field | `src/workbench/shellActions.ts` | missing |
| `find.field.recentSearches` | Recent Searches | `src/workbench/shellActions.ts` | missing |
| `find.inFile` | Find | `src/editor/core/actions.ts` | missing |
| `find.next` | Find Next Occurrence | `src/editor/core/actions.ts` | missing |
| `find.openInWindow` | Open Results in Find Window | `src/editor/core/actions.ts` | missing |
| `find.previous` | Find Previous Occurrence | `src/editor/core/actions.ts` | missing |
| `find.replaceInFile` | Replace | `src/editor/core/actions.ts` | missing |
| `find.scrollToResults` | Scroll to Results During Typing | `src/editor/core/actions.ts` | missing |
| `navigate.back` | Back | `src/editor/core/actions.ts` | missing |
| `navigate.forward` | Forward | `src/editor/core/actions.ts` | missing |
| `navigate.gotoLine` | Go to Line:Column… | `src/editor/core/actions.ts` | missing |
| `navigate.lastEditLocation` | Last Edit Location | `src/editor/core/actions.ts` | missing |
| `navigate.navigationBar` | Jump to Navigation Bar | `src/editor/core/actions.ts` | missing |
| `navigate.nextEditLocation` | Next Edit Location | `src/editor/core/actions.ts` | missing |
| `navigate.nextOccurrence` | Next Occurrence | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/problems.go StepProblem; internal/app/toolwindow_test.go |
| `navigate.prevOccurrence` | Previous Occurrence | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/problems.go StepProblem; internal/app/toolwindow_test.go |
| `refactor.thisMenu` | Refactor This… | `src/editor/lsp/actions.ts` | missing |
| `toolbar.customize` | Customize Main Toolbar… | `src/workbench/shellActions.ts` | missing |
| `toolwindow.${id}` |  | `src/workbench/toolwindows/actions.ts` | missing |
| `toolwindow.closeContent` | Close Tool Window Tab | `src/workbench/toolwindows/actions.ts` | missing |
| `toolwindow.focusEditor` | Focus Editor | `src/workbench/toolwindows/actions.ts` | missing |
| `toolwindow.hideActive` | Hide Active Tool Window | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow.go HideActiveToolWindow; internal/app/toolwindow_test.go |
| `toolwindow.hideAll` | Hide All Tool Windows | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow.go HideAllToolWindows; internal/app/toolwindow_test.go |
| `toolwindow.jumpToLast` | Jump to Last Tool Window | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow.go JumpToLastToolWindow; internal/app/toolwindow_test.go |
| `toolwindow.maximize` | Maximize Tool Window | `src/workbench/toolwindows/actions.ts` | integrated - internal/app/toolwindow.go ToggleMaximizeToolWindow; internal/app/app.go View; internal/app/maximize_test.go |
| `toolwindow.resize${dir[0]!.toUpperCase()}${dir.slice(1)}` |  | `src/workbench/toolwindows/actions.ts` | missing |
| `view.backgroundTasks` | Background Tasks | `src/workbench/shellActions.ts` | missing |
| `view.notifications` | Notifications | `src/workbench/shellActions.ts` | missing |
| `view.quickSwitchScheme` | Quick Switch Scheme… | `src/workbench/shellActions.ts` | missing |
| `view.toggleStatusBarWidget` | Status Bar Widget | `src/workbench/shellActions.ts` | missing |
| `view.toggleTheme` | Toggle Light/Dark Theme | `src/workbench/shellActions.ts` | missing |
| `workbench.stopBackgroundProcesses` | Stop Background Processes… | `src/workbench/shellActions.ts` | missing |

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
| `code.optimizeImports` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.overrideMethods` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.parameterInfo` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `code.pasteAsPlainText` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `code.pasteFromHistory` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.quickDefinition` | intellij-macos, intellij-windows, vscode-macos | missing |
| `code.quickDocumentation` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
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
| `debug.stepInto` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `debug.stepOut` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `debug.stepOver` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
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
| `editor.toggleZen` | intellij-macos, intellij-windows, vscode-macos | missing |
| `file.settings` | intellij-macos, intellij-windows, vscode-macos | missing |
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
| `layout.restoreDefault` | intellij-macos, intellij-windows, vscode-macos | missing |
| `markdown.editorAndPreview` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `markdown.editorOnly` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `markdown.previewOnly` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
| `navigate.customRegion` | intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.declaration` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.gotoBookmark0` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark1` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark2` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark3` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark4` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark5` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark6` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark7` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark8` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoBookmark9` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go JumpBookmarkMnemonic via Ctrl+digit; base.bookmark.mnemonic, base.bookmark.mnemonicUnique; internal/app/bookmarks_test.go |
| `navigate.gotoTest` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.implementation` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.nextError` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.nextHighlightedUsage` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.nextMethod` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.prevError` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.prevHighlightedUsage` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.previousMethod` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.relatedSymbol` | vscode-macos | missing |
| `navigate.superMethod` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `navigate.toggleBookmark0` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark1` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark2` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark3` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark4` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark5` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark6` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark7` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark8` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.toggleBookmark9` | intellij-macos, intellij-windows, vscode-macos | partial - internal/app/bookmarks.go ToggleBookmarkMnemonic via Ctrl+Shift+digit; base.bookmark.mnemonic; internal/app/bookmarks_test.go |
| `navigate.typeDeclaration` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `project.copyAbsolutePath` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |
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
| `refactor.safeDelete` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | missing |
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
| `vcs.addToVcs` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.annotate` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.branches` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.changelist.edit` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.changelist.move` | eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.commit` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | integrated - base.commit.id, base.commit.staged, base.commit.clean |
| `vcs.commitAndPush` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.localHistory` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.log.copyRevision` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.log.editMessage` | eclipse-windows | missing |
| `vcs.log.goToRef` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.newBranch` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.nextChange` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.operations` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.prevChange` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.push` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.rollback` | eclipse-macos, intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.shelf.rename` | eclipse-windows | missing |
| `vcs.showDiff` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.showMessageHistory` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.toggleAmend` | intellij-macos, intellij-windows, vscode-macos | missing |
| `vcs.update` | eclipse-macos, eclipse-windows, intellij-macos, intellij-windows, vscode-macos | missing |
| `view.resetZoom` | intellij-macos, intellij-windows, vscode-macos | missing |
| `view.zoomIn` | intellij-macos, intellij-windows, vscode-macos | missing |
| `view.zoomOut` | intellij-macos, intellij-windows, vscode-macos | missing |
| `window.next` | intellij-macos, intellij-windows, vscode-macos | missing |
| `window.nextTab` | intellij-macos, intellij-windows, vscode-macos | missing |
| `window.previous` | intellij-macos, intellij-windows, vscode-macos | missing |
| `window.previousTab` | intellij-macos, intellij-windows, vscode-macos | missing |
| `window.toggleFullScreen` | intellij-macos, intellij-windows, vscode-macos, vscode-windows | missing |

## Keymaps

| Preset | Bindings | Status |
| --- | ---: | --- |
| `eclipse-macos` | 146 | missing |
| `eclipse-windows` | 147 | missing |
| `intellij-macos` | 368 | missing |
| `intellij-windows` | 368 | missing |
| `vscode-macos` | 370 | missing |
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
| `coverage` | Coverage | right | `src/features/coverage/index.ts` | missing |
| `database` | Database | right | `src/features/database/index.ts` | partial - UPSTREAM_FEATURE_MATRIX.md database console; db_* commands |
| `debug` | Debug | bottom | `src/features/debugger/index.ts` | integrated - debug.paused, debug.vars, debug.step.paused, debug.watch |
| `documentation` | Documentation | right | `src/editor/lsp/index.ts` | missing |
| `dotnet` | .NET | right | `src/features/dotnet/index.ts` | missing |
| `duplicates` | Duplicates | bottom | `src/features/analysis/index.ts` | missing |
| `find` | Find | bottom | `src/features/search/index.ts` | integrated - base.search.done, base.search.files, base.search.matches; internal/app/search.go searchPanel; bottom anchor in internal/app/anchor_test.go |
| `git` | Git | bottom | `src/features/vcs/index.ts` | partial - base.vcs.changes, base.diff.rows, internal/app/log.go logPanel, base.log.commits, base.log.details, base.log.changes; internal/app/branch.go branchLabel, base.branch.head, base.branch.list, base.branch.checkout; internal/app/vcs.go vcsRows staging groups, StageChange, UnstageChange, AskRollback, base.stage.roundtrip, base.stage.discard |
| `gradle` | Gradle | right | `src/features/build-tools/index.ts` | missing |
| `hierarchy` | Hierarchy | right | `src/features/code-insight/index.ts` | missing |
| `inspection` | Inspection Results | bottom | `src/features/analysis/index.ts` | missing |
| `logcat` | Logcat | bottom | `src/features/logcat/index.ts` | missing |
| `maven` | Maven | right | `src/features/build-tools/index.ts` | missing |
| `notifications` | Notifications | right | `src/workbench/activate.ts` | missing |
| `problems` | Problems | bottom | `src/features/code-insight/index.ts` | integrated - internal/app/problems.go problemsPanel; lsp.problems.errors, lsp.problems.rows, lsp.problems.visible; internal/app/problems_test.go |
| `profiler` | Profiler | bottom | `src/features/profiler/index.ts` | missing |
| `project` | Project | left | `src/features/project-tree/index.ts` | integrated - internal/workspace tree tests; base.tabs |
| `pullRequests` | Pull Requests | left | `src/features/github/index.ts` | missing |
| `remote.ports` | Ports | bottom | `src/features/remote-dev/index.ts` | missing |
| `remoteHost` | Remote Host | right | `src/features/deployment/index.ts` | missing |
| `run` | Run | bottom | `src/features/run/index.ts` | integrated - base.run.config, base.run.exit, base.run.build |
| `services` | Services | bottom | `src/features/services/index.ts` | missing |
| `structure` | Structure | left | `src/features/search/index.ts` | integrated - internal/app/structure.go structureRowsUI; lsp.structure.rows, lsp.structure.first, lsp.structure.visible; internal/app/structure_test.go |
| `terminal` | Terminal | bottom | `src/features/terminal/index.ts` | partial - internal/app/term.go TermPane tab groups, OpenTermTab Cmd+T, SplitTerm/UnsplitTerm with termPaneBody in view.go, ShowTermFind/FindTermStep Cmd+F find bar, CloseTermTab hides the empty window; base.term.output, base.term.tabs, base.term.split, base.term.find; internal/app/term_test.go |
| `todo` | TODO | bottom | `src/features/todo/index.ts` | missing |
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
| `code-insight.usagesPopup` | `src/features/code-insight/index.ts` | missing |
| `coverage.manageReports` | `src/features/coverage/index.ts` | missing |
| `database.dialogs` | `src/features/database/index.ts` | missing |
| `debugger` | `src/features/debugger/index.ts` | missing |
| `deployment.dialogs` | `src/features/deployment/index.ts` | missing |
| `editor-lsp.hierarchyDialog` | `src/editor/lsp/index.ts` | missing |
| `editor-lsp.popups` | `src/editor/lsp/index.ts` | missing |
| `editor.gotoLine` | `src/editor/core/index.ts` | missing |
| `editor.navigationBar` | `src/editor/core/index.ts` | missing |
| `editor.pasteHistory` | `src/editor/core/index.ts` | missing |
| `github.dialogs` | `src/features/github/index.ts` | missing |
| `graphql.environment` | `src/features/graphql/index.ts` | missing |
| `macros.dialogs` | `src/features/macros/index.ts` | missing |
| `mcp.confirm` | `src/features/mcp/index.ts` | missing |
| `perf.overlay` | `src/features/perf/index.ts` | missing |
| `plugins.consent` | `src/features/plugins/index.ts` | missing |
| `plugins.intellijThemes` | `src/features/plugins/index.ts` | missing |
| `project.fileRefactor` | `src/features/project-tree/index.ts` | missing |
| `project.newPopup` | `src/features/project-tree/index.ts` | missing |
| `project.refactoringPreview` | `src/features/project-tree/index.ts` | missing |
| `project.trustDialog` | `src/features/project-trust/index.tsx` | missing |
| `remote.connect` | `src/features/remote-dev/index.ts` | missing |
| `run.chooser` | `src/features/run/index.ts` | missing |
| `run.dialogs` | `src/features/run/index.ts` | missing |
| `run.editConfigs` | `src/features/run/index.ts` | missing |
| `run.serviceImport` | `src/features/run/index.ts` | missing |
| `run.stackTraceDialog` | `src/features/run/stackTrace.tsx` | missing |
| `runAnything.popup` | `src/features/run-anything/index.ts` | integrated - internal/app/run_anything.go runAnythingDialog, drawn by View in internal/app/app.go; internal/app/run_anything_test.go |
| `scratch.chooser` | `src/features/scratch/index.ts` | missing |
| `search.bookmarks` | `src/features/search/index.ts` | missing |
| `search.everywhere` | `src/features/search/index.ts` | missing |
| `search.fileStructure` | `src/features/search/index.ts` | missing |
| `search.findInFiles` | `src/features/search/index.ts` | missing |
| `search.recent` | `src/features/search/index.ts` | missing |
| `search.scopes` | `src/features/search/index.ts` | missing |
| `services.dialogs` | `src/features/services/index.ts` | missing |
| `settings.assignShortcut` | `src/features/settings/index.ts` | missing |
| `settings.dialog` | `src/features/settings/index.ts` | missing |
| `settings.projectStructure` | `src/features/settings/index.ts` | missing |
| `ssh.askpass` | `src/features/ssh/index.ts` | missing |
| `templates.chooser` | `src/features/templates/index.ts` | missing |
| `templates.emmetUpdateTag` | `src/features/templates/index.ts` | missing |
| `templates.newFile` | `src/features/templates/index.ts` | missing |
| `vcs.branches` | `src/features/vcs/index.ts` | missing |
| `vcs.dialogs` | `src/features/vcs/index.ts` | missing |
| `welcome.recentProjects` | `src/features/welcome/index.ts` | missing |
| `workbench.backgroundTasks` | `src/workbench/activate.ts` | missing |
| `workbench.customizeToolbar` | `src/workbench/activate.ts` | missing |

## Editor contributions and file editor types

### Editor contributions

| Contribution | Source | Status |
| --- | --- | --- |
| `build-tools.pomNavigation` | `src/features/build-tools/index.ts` | missing |
| `build-tools.versionCatalog` | `src/features/build-tools/index.ts` | missing |
| `colorPreview.gutter` | `src/features/color-preview/index.ts` | missing |
| `coverage.gutter` | `src/features/coverage/index.ts` | missing |
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
| `run.lens` | `src/editor/api.ts` | missing |
| `run.npmScripts` | `src/features/run/index.ts` | missing |
| `run.targets` | `src/features/run/index.ts` | missing |
| `search.bookmarks` | `src/features/search/index.ts` | missing |
| `search.tracking` | `src/features/search/common/tracking.ts` | missing |
| `services.composeEditor` | `src/features/services/index.ts` | missing |
| `services.dockerGutter` | `src/features/services/index.ts` | missing |
| `templates` | `src/features/templates/index.ts` | missing |
| `todo.highlight` | `src/features/todo/index.ts` | missing |
| `vcs.changeMarkers` | `src/features/vcs/index.ts` | missing |
| `vcs.marks` | `src/editor/api.ts` | missing |
| `viewers.markdownLive` | `src/features/viewers/markdown/live.ts` | missing |
| `vim` | `src/features/vim/index.ts` | missing |

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
| `agents.chat` | Chat | Agents | `src/features/settings/pages/tools.tsx` | missing |
| `agents.mcpServer` | IDE MCP Server | Agents | `src/features/settings/pages/mcpServer.tsx` | missing |
| `appearance` | Appearance |  | `src/features/settings/pages/behavior.tsx` | partial - base.settings.theme |
| `build.cmake` | CMake |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.console` | Console |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.debugger` | Debugger |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.debugger.dataViews` | Data Views |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.debugger.stepping` | Stepping |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.deployment` | Deployment |  | `src/features/settings/pages/deployment.tsx` | missing |
| `build.deployment.options` | Options |  | `src/features/settings/pages/deployment.tsx` | missing |
| `build.gradle` | Gradle |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.maven` | Maven |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.pythonDebugger` | Python Debugger |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.run` | Run/Debug |  | `src/features/settings/pages/execution.tsx` | missing |
| `build.runTargets` | Run Targets |  | `src/features/settings/pages/runTargets.tsx` | missing |
| `editor.codeStyle` | Code Style | Editor | `src/features/settings/pages/editor/codeStyle.tsx` | missing |
| `editor.colorScheme.debugger` | Debugger |  | `src/features/settings/pages/editor/colorScheme.tsx` | missing |
| `editor.colorScheme.general` | General |  | `src/features/settings/pages/editor/colorScheme.tsx` | missing |
| `editor.colorScheme.language-defaults` | Language Defaults |  | `src/features/settings/pages/editor/colorScheme.tsx` | missing |
| `editor.fileTemplates` | File and Code Templates | Editor | `src/features/templates/index.ts` | missing |
| `editor.files` | File Types |  | `src/features/settings/pages/builtins.tsx` | missing |
| `editor.general.postfixCompletion` | Postfix Completion | Editor | `src/features/templates/index.ts` | missing |
| `editor.inspections` | Inspections | Editor | `src/features/settings/pages/editor/inspections.tsx` | missing |
| `editor.liveTemplates` | Live Templates | Editor | `src/features/templates/index.ts` | missing |
| `editor.naturalLanguages` | Natural Languages | Editor | `src/features/settings/pages/editor/spelling.tsx` | missing |
| `editor.naturalLanguages.spelling` | Spelling | Editor | `src/features/settings/pages/editor/spelling.tsx` | missing |
| `editor.todo` | TODO | Editor | `src/features/todo/index.ts` | missing |
| `general` | System Settings |  | `src/features/settings/pages/behavior.tsx` | missing |
| `general.dateFormats` | Date Formats |  | `src/features/settings/pages/behavior.tsx` | missing |
| `general.httpProxy` | HTTP Proxy |  | `src/features/settings/pages/httpProxy.tsx` | missing |
| `general.language` | Language and Region |  | `src/features/settings/pages/behavior.tsx` | missing |
| `general.updates` | Updates |  | `src/features/settings/pages/updates.tsx` | missing |
| `keymap` | Keymap |  | `src/features/settings/pages/builtins.tsx` | missing |
| `languages.go` | Go |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.go.buildTags` | Build Tags |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.go.modules` | Go Modules |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.java` | Java |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.markdown` | Markdown |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.node` | Node.js |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.oxfmt` | oxfmt |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.php` | PHP |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.prettier` | Prettier |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.ruby` | Ruby |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.ruff` | Ruff |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.rust` | Rust |  | `src/features/settings/pages/languages.tsx` | missing |
| `languages.typescript` | TypeScript |  | `src/features/settings/pages/languages.tsx` | missing |
| `notifications` | Notifications |  | `src/features/settings/pages/behavior.tsx` | missing |
| `projectStructure` | Project Structure |  | `src/features/settings/pages/languages.tsx` | missing |
| `projectTrust` | Project Trust | Security | `src/features/project-trust/index.tsx` | missing |
| `python.interpreter` | Python Interpreter |  | `src/features/settings/pages/languages.tsx` | missing |
| `tools.actionsOnSave` | Actions on Save |  | `src/features/settings/pages/tools.tsx` | missing |
| `tools.externalTools` | External Tools |  | `src/features/settings/pages/tools.tsx` | missing |
| `tools.sshConfigurations` | SSH Configurations |  | `src/features/settings/pages/ssh.tsx` | missing |
| `tools.terminal` | Terminal |  | `src/features/settings/pages/tools.tsx` | missing |
| `vcs` | Version Control |  | `src/features/settings/pages/vcs.tsx` | missing |
| `vcs.commit` | Commit |  | `src/features/settings/pages/vcs.tsx` | missing |
| `vcs.confirmation` | Confirmation |  | `src/features/settings/pages/vcs.tsx` | missing |
| `vcs.git` | Git |  | `src/features/settings/pages/vcs.tsx` | missing |
| `vcs.github` | GitHub |  | `src/features/github/settingsPages.ts` | missing |
| `vcs.issueNavigation` | Issue Navigation |  | `src/features/settings/pages/vcs.tsx` | missing |
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
| `agents` |  | missing |
| `agents.defaults` |  | missing |
| `agents.expandToolOutput` | agents.chat | missing |
| `agents.foldTurns` | agents.chat | missing |
| `agents.mcpServers` |  | missing |
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
| `appearance.alwaysShowMainMenu` |  | missing |
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
| `editor.appearance.lineNumbers` |  | missing |
| `editor.appearance.methodSeparators` |  | missing |
| `editor.appearance.rainbowIndentGuides` |  | missing |
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
| `editor.encodings.global` |  | missing |
| `editor.encodings.nativeToAscii` |  | missing |
| `editor.encodings.project` |  | missing |
| `editor.encodings.properties` |  | missing |
| `editor.encodings.utf8Bom` |  | missing |
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
| `editor.inlay.codeVisionUsages` |  | missing |
| `editor.inlay.disabledLanguages` |  | missing |
| `editor.inlay.other` |  | missing |
| `editor.inlay.parameters` |  | missing |
| `editor.inlay.types` |  | missing |
| `editor.inlayHints` |  | missing |
| `editor.inlineCompletion.disabledLanguages` |  | missing |
| `editor.inlineCompletion.enabled` |  | missing |
| `editor.inlineCompletion.onlyOnPause` |  | missing |
| `editor.inlineCompletion.pauseMs` |  | missing |
| `editor.insertSpaces` |  | missing |
| `editor.ligatures` |  | missing |
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
| `editor.tabSize` |  | missing |
| `editor.tabs.activateOnClose` |  | missing |
| `editor.tabs.closeButtonPosition` |  | missing |
| `editor.tabs.closingPolicy` |  | missing |
| `editor.tabs.markModified` |  | missing |
| `editor.tabs.multipleRows` |  | missing |
| `editor.tabs.oneRowOverflow` |  | missing |
| `editor.tabs.openAtEnd` |  | missing |
| `editor.tabs.pinnedSeparateRow` |  | missing |
| `editor.tabs.placement` |  | missing |
| `editor.tabs.previewTab` |  | missing |
| `editor.tabs.showCloseButton` |  | missing |
| `editor.tabs.showDirectoryForNonUnique` |  | missing |
| `editor.tabs.showExtension` |  | missing |
| `editor.tabs.showFileIcon` |  | missing |
| `editor.tabs.showFullPathOnHover` |  | missing |
| `editor.tabs.sortAlphabetically` |  | missing |
| `editor.tabs.tabLimit` |  | missing |
| `externalTools` |  | missing |
| `files` |  | missing |
| `files.autoSave` | general | missing |
| `files.composeFileOverrides` |  | missing |
| `files.ensureFinalNewline` |  | missing |
| `files.exclude` |  | missing |
| `files.fileTypeOverrides` |  | missing |
| `files.fileTypes` |  | missing |
| `files.ignoredNames` |  | missing |
| `files.localHistoryDays` | general | missing |
| `files.localHistoryMaxMb` | general | missing |
| `files.syncOnFocus` | general | missing |
| `files.trimTrailingWhitespace` |  | missing |
| `general` |  | missing |
| `general.autoCheckUpdates` | general.updates | missing |
| `general.closeTabsWithWindow` | general | missing |
| `general.closeWithoutWelcome` | general | missing |
| `general.confirmExit` | general | missing |
| `general.dateFormat.overrideSystem` |  | missing |
| `general.dateFormat.pattern` |  | missing |
| `general.dateFormat.pretty` |  | missing |
| `general.dateFormat.use24Hour` |  | missing |
| `general.defaultProjectDirectory` |  | missing |
| `general.idleSeconds` |  | missing |
| `general.openProjectIn` |  | missing |
| `general.powerSaveMode` |  | missing |
| `general.projectName` |  | missing |
| `general.reopenLastProject` | general | missing |
| `general.saveOnIdle` |  | missing |
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
| `languages.python.interpreter` |  | missing |
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
| `mcpServer.enabled` | agents.mcpServer | missing |
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
| `vcs.issueNavigation` |  | missing |
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

All 624 commands the desktop shell registers. `stdio` is whether the pinned
`openidea-server` serves the command; `GoIDEA` is whether GoIDEA calls it.
Local-only commands and the per-command argument lists are in
[OPENIDEA_IPC.md](OPENIDEA_IPC.md).

| Command | Module | stdio | GoIDEA | Status |
| --- | --- | --- | --- | --- |
| `agent_archive` | agent | yes |  | missing |
| `agent_base_text` | agent | yes |  | missing |
| `agent_cancel` | agent | yes |  | missing |
| `agent_changes` | agent | yes |  | missing |
| `agent_commands` | agent | yes |  | missing |
| `agent_create` | agent | yes |  | missing |
| `agent_delete` | agent | yes |  | missing |
| `agent_entries` | agent | yes |  | missing |
| `agent_external_sessions` | agent | yes |  | missing |
| `agent_fork` | agent | yes |  | missing |
| `agent_import_session` | agent | yes |  | missing |
| `agent_mcp_import` | agent | yes |  | missing |
| `agent_mcp_list` | agent | yes |  | missing |
| `agent_mcp_remove` | agent | yes |  | missing |
| `agent_mcp_save` | agent | yes |  | missing |
| `agent_mcp_set_enabled` | agent | yes |  | missing |
| `agent_probe` | agent | yes |  | missing |
| `agent_prompt` | agent | yes |  | missing |
| `agent_providers` | agent | yes |  | missing |
| `agent_registry_install` | agent | yes |  | missing |
| `agent_registry_list` | agent | yes |  | missing |
| `agent_registry_remove` | agent | yes |  | missing |
| `agent_rename` | agent | yes |  | missing |
| `agent_respond` | agent | yes |  | missing |
| `agent_rollback` | agent | yes |  | missing |
| `agent_rollback_changes` | agent | yes |  | missing |
| `agent_send_queued_now` | agent | yes |  | missing |
| `agent_set_command` | agent | yes |  | missing |
| `agent_set_config` | agent | yes |  | missing |
| `agent_set_env` | agent | yes |  | missing |
| `agent_stop_background` | agent | yes |  | missing |
| `agent_subscribe` | agent | yes |  | missing |
| `agent_unarchive` | agent | yes |  | missing |
| `agent_unqueue` | agent | yes |  | missing |
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
| `app_restart` | caches | no |  | missing |
| `app_update_announcement` | window | no |  | missing |
| `app_update_status` | window | no |  | missing |
| `app_update_window_ready` | window | no |  | missing |
| `apply_workspace_edit` | lsp | yes | yes | missing |
| `build_compile_file` | build | yes |  | missing |
| `build_model` | build | yes |  | missing |
| `build_run` | build | yes | yes | missing |
| `build_sync` | build | yes |  | missing |
| `caches_schedule_invalidation` | caches | no |  | missing |
| `cancel_request` | app | yes | yes | missing |
| `cancel_task` | app | yes |  | missing |
| `clear_caches` | perf | no |  | missing |
| `clipboard_file_list` | clipboard | no |  | missing |
| `clipboard_has_files` | clipboard | no |  | missing |
| `clipboard_image` | clipboard | no |  | missing |
| `clipboard_set_file_list` | clipboard | no |  | missing |
| `clipboard_write_text` | clipboard | no |  | missing |
| `close_project` | app | yes |  | missing |
| `coverage_collect` | coverage | yes |  | missing |
| `coverage_import` | coverage | yes |  | missing |
| `coverage_prepare` | coverage | yes |  | missing |
| `db_apply_edits` | db | yes |  | missing |
| `db_completion` | db | yes |  | missing |
| `db_connect` | db | yes | yes | missing |
| `db_connected` | db | yes | yes | missing |
| `db_console_path` | db | yes |  | missing |
| `db_databases` | db | yes |  | missing |
| `db_ddl` | db | yes |  | missing |
| `db_disconnect` | db | yes | yes | missing |
| `db_execute` | db | yes | yes | missing |
| `db_explain` | db | yes |  | missing |
| `db_export_file` | db | no |  | missing |
| `db_finish_transaction` | db | yes |  | missing |
| `db_history` | db | yes |  | missing |
| `db_object_ddl` | db | yes |  | missing |
| `db_remove_history` | db | yes |  | missing |
| `db_save_sources` | db | yes | yes | missing |
| `db_schema_objects` | db | yes |  | missing |
| `db_schemas` | db | yes | yes | missing |
| `db_set_manual_commit` | db | yes |  | missing |
| `db_set_password` | db | yes |  | missing |
| `db_set_ssh_secret` | db | yes |  | missing |
| `db_sources` | db | yes | yes | missing |
| `db_statements` | db | yes | yes | missing |
| `db_table_details` | db | yes |  | missing |
| `db_table_page` | db | yes |  | missing |
| `db_tables` | db | yes | yes | missing |
| `db_test` | db | yes |  | missing |
| `db_trust_ssh_host` | db | yes |  | missing |
| `debug_attachable_processes` | debug | yes |  | missing |
| `debug_breakpoint_states` | debug | yes |  | missing |
| `debug_capabilities` | debug | yes |  | missing |
| `debug_completions` | debug | yes |  | missing |
| `debug_data_breakpoint_info` | debug | yes |  | missing |
| `debug_evaluate` | debug | yes | yes | missing |
| `debug_evaluate_in` | debug | yes |  | missing |
| `debug_exception_filters` | debug | yes |  | missing |
| `debug_force_step_over` | debug | yes |  | missing |
| `debug_free_port` | debug | yes |  | missing |
| `debug_hot_reload` | debug | no |  | missing |
| `debug_hot_restart` | debug | no |  | missing |
| `debug_listen_for_jvm` | debug | yes |  | missing |
| `debug_pause` | debug | yes | yes | missing |
| `debug_redefine_classes` | debug | yes |  | missing |
| `debug_restart_frame` | debug | yes |  | missing |
| `debug_resume` | debug | yes | yes | missing |
| `debug_run_to` | debug | yes |  | missing |
| `debug_scopes` | debug | yes | yes | missing |
| `debug_send_input` | debug | yes |  | missing |
| `debug_sessions` | debug | yes |  | missing |
| `debug_set_breakpoints` | debug | yes | yes | missing |
| `debug_set_data_breakpoints` | debug | yes |  | missing |
| `debug_set_exception_breakpoints` | debug | yes |  | missing |
| `debug_set_function_breakpoints` | debug | yes |  | missing |
| `debug_set_variable` | debug | yes |  | missing |
| `debug_stack_trace` | debug | yes | yes | missing |
| `debug_start` | debug | yes | yes | missing |
| `debug_step` | debug | yes | yes | missing |
| `debug_step_in_target` | debug | yes |  | missing |
| `debug_step_in_targets` | debug | yes |  | missing |
| `debug_stop` | debug | yes | yes | missing |
| `debug_threads` | debug | yes | yes | missing |
| `debug_variables` | debug | yes | yes | missing |
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
| `doc_close` | doc | yes | yes | missing |
| `doc_edit` | doc | yes | yes | missing |
| `doc_folding` | doc | yes |  | missing |
| `doc_highlights` | doc | yes |  | missing |
| `doc_open` | doc | yes | yes | missing |
| `doc_open_virtual` | doc | yes |  | missing |
| `doc_outline` | doc | yes | yes | missing |
| `doc_save` | doc | yes | yes | missing |
| `doc_save_all` | doc | yes |  | missing |
| `doc_selection_ranges` | doc | yes |  | missing |
| `doc_set_encoding` | doc | yes |  | missing |
| `doc_set_line_ending` | doc | yes |  | missing |
| `doc_snapshot` | doc | yes | yes | missing |
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
| `env_reload` | env | yes |  | missing |
| `file_dialog_needs_gtk` | file_dialog | no |  | missing |
| `find_dirs` | nav | yes |  | missing |
| `find_files` | nav | yes | yes | missing |
| `find_files_named` | nav | yes |  | missing |
| `find_symbols` | nav | yes |  | missing |
| `fs_compare_directories` | fs | yes | yes | missing |
| `fs_copy` | fs | yes | yes | missing |
| `fs_copy_merge` | fs | yes |  | missing |
| `fs_create_dir` | fs | yes | yes | missing |
| `fs_create_file` | fs | yes | yes | missing |
| `fs_create_file_bytes` | fs | yes |  | missing |
| `fs_delete` | fs | yes | yes | missing |
| `fs_entry_kind` | fs | yes |  | missing |
| `fs_probe` | bytes | yes |  | missing |
| `fs_read_dir` | fs | yes |  | missing |
| `fs_read_image` | bytes | yes |  | missing |
| `fs_read_range` | bytes | yes |  | missing |
| `fs_read_text` | fs | yes |  | missing |
| `fs_rename` | fs | yes | yes | missing |
| `fs_resolve_reference` | nav | yes |  | missing |
| `fs_set_writable` | fs | yes |  | missing |
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
| `gradle_link_projects` | build | yes |  | missing |
| `gradle_published_catalog` | build | yes |  | missing |
| `gradle_unlink_projects` | build | yes |  | missing |
| `gtk_pick_paths` | file_dialog | no |  | missing |
| `history_content` | doc | yes |  | missing |
| `history_list` | doc | yes |  | missing |
| `history_list_dir` | history | yes | yes | missing |
| `history_put_label` | history | yes | yes | missing |
| `history_restore` | history | yes | yes | missing |
| `http_curl` | http | no |  | missing |
| `http_environment_file` | http | no |  | missing |
| `http_environments` | http | no |  | missing |
| `http_execute` | http | no |  | missing |
| `http_from_curl` | http | no |  | missing |
| `http_history` | http | no |  | missing |
| `http_history_clear` | http | no |  | missing |
| `http_history_get` | http | no |  | missing |
| `http_requests` | http | no |  | missing |
| `index_file_extensions` | nav | yes |  | missing |
| `index_status` | nav | yes |  | missing |
| `inspect_code` | analysis | yes |  | missing |
| `java_build_workspace` | java | yes |  | missing |
| `java_debug_server` | java | yes | yes | missing |
| `java_resolve_launch` | java | yes | yes | missing |
| `jdk_download` | tools | yes |  | missing |
| `jdk_download_location` | tools | yes |  | missing |
| `jdk_download_releases` | tools | yes |  | missing |
| `jdk_download_vendors` | tools | yes |  | missing |
| `jvm_file_presentations` | syntax | yes |  | missing |
| `languages` | app | no |  | missing |
| `light_edit_open` | app | no |  | missing |
| `log_frontend` | app | no |  | missing |
| `lsp_apply_code_action` | lsp | yes | yes | missing |
| `lsp_call_hierarchy` | lsp | yes |  | missing |
| `lsp_can_download_sources` | lsp | yes |  | missing |
| `lsp_code_action_query` | lsp | yes | yes | missing |
| `lsp_code_actions` | lsp | yes |  | missing |
| `lsp_code_lens` | lsp | yes |  | missing |
| `lsp_completion` | lsp | yes | yes | missing |
| `lsp_document_highlights` | lsp | yes |  | missing |
| `lsp_document_links` | lsp | yes |  | missing |
| `lsp_execute_code_action` | lsp | yes |  | missing |
| `lsp_execute_command` | lsp | yes |  | missing |
| `lsp_format` | lsp | yes | yes | missing |
| `lsp_goto` | lsp | yes | yes | missing |
| `lsp_goto_implementation` | lsp | yes |  | missing |
| `lsp_hierarchy_members` | lsp | yes |  | missing |
| `lsp_hierarchy_refactor` | lsp | yes |  | missing |
| `lsp_hover` | lsp | yes | yes | missing |
| `lsp_implementations` | lsp | yes |  | missing |
| `lsp_inlay_hints` | lsp | yes |  | missing |
| `lsp_inline_completion` | lsp | yes |  | missing |
| `lsp_library_sources` | lsp | yes |  | missing |
| `lsp_prepare_rename` | lsp | yes | yes | missing |
| `lsp_references` | lsp | yes | yes | missing |
| `lsp_rename` | lsp | yes | yes | missing |
| `lsp_resend_diagnostics` | lsp | yes |  | missing |
| `lsp_resolve_class_file` | lsp | yes |  | missing |
| `lsp_resolve_code_action` | lsp | yes |  | missing |
| `lsp_resolve_command` | lsp | yes |  | missing |
| `lsp_resolve_completion` | lsp | yes |  | missing |
| `lsp_restart` | lsp | yes |  | missing |
| `lsp_restart_named` | lsp | no |  | missing |
| `lsp_semantic_tokens` | lsp | yes |  | missing |
| `lsp_servers` | lsp | yes | yes | missing |
| `lsp_signature_help` | lsp | yes |  | missing |
| `lsp_type_hierarchy` | lsp | yes |  | missing |
| `lsp_virtual_document` | lsp | yes |  | missing |
| `lsp_workspace_symbols` | lsp | yes | yes | missing |
| `mark_recent` | nav | yes | yes | missing |
| `maven_dependency_declaration` | build | yes |  | missing |
| `maven_exclude_dependency` | build | yes |  | missing |
| `maven_goto_declaration` | build | yes |  | missing |
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
| `open_project` | app | yes | yes | missing |
| `path_completions` | nav | yes |  | missing |
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
| `plugin_prepare_debug_run` | plugin | yes |  | missing |
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
| `profiler_prepare` | profiler | no |  | missing |
| `project_current` | app | yes | yes | missing |
| `project_rename` | app | no |  | missing |
| `project_roots` | app | yes |  | missing |
| `project_trust_add_location` | app | no |  | missing |
| `project_trust_list` | app | yes |  | missing |
| `project_trust_remove_location` | app | yes |  | missing |
| `project_trust_set` | app | yes | yes | missing |
| `project_trust_status` | app | yes | yes | missing |
| `proxy_check_connection` | http | no |  | missing |
| `recent_files` | nav | yes |  | missing |
| `recent_project_groups` | app | no |  | missing |
| `recent_projects` | app | no |  | missing |
| `redo_workspace_edit` | lsp | yes | yes | missing |
| `refactor_can_undo_file_move` | refactor | yes |  | missing |
| `refactor_file_move_edits` | refactor | yes |  | missing |
| `refactor_file_move_text_edits` | refactor | yes |  | missing |
| `refactor_move_files` | refactor | yes |  | missing |
| `refactor_undo_file_move` | refactor | yes |  | missing |
| `remote_bootstrap_list` | remote_dev | no |  | missing |
| `remote_connect` | remote_dev | no |  | missing |
| `remote_current` | remote_dev | no |  | missing |
| `remote_delete` | deploy | yes |  | missing |
| `remote_disconnect` | remote_dev | no |  | missing |
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
| `remove_recent` | nav | yes |  | missing |
| `remove_recent_project` | app | no |  | missing |
| `replace_in_files` | search | yes |  | missing |
| `replace_preview` | search | yes |  | missing |
| `run_application_build` | run | yes |  | missing |
| `run_application_publish` | run | yes |  | missing |
| `run_apply_settings` | run | yes |  | missing |
| `run_close_stdin` | run | yes |  | missing |
| `run_configs` | run | yes | yes | missing |
| `run_dotnet_resolve` | run | yes |  | missing |
| `run_env_files` | run | yes |  | missing |
| `run_gradle_init_script` | run | yes |  | missing |
| `run_gradle_jvm_args_script` | run | yes |  | missing |
| `run_import_configs` | run | yes |  | missing |
| `run_import_sources` | run | yes |  | missing |
| `run_list` | run | yes |  | missing |
| `run_resize` | run | yes |  | missing |
| `run_save_configs` | run | yes |  | missing |
| `run_save_templates` | run | yes |  | missing |
| `run_start` | run | yes | yes | missing |
| `run_stdin` | run | yes |  | missing |
| `run_stop` | run | yes | yes | missing |
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
| `sdk_detect` | sdk | yes |  | missing |
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
| `search_start` | search | yes | yes | missing |
| `search_validate` | search | yes |  | missing |
| `set_recent_project_groups` | app | no |  | missing |
| `settings_export` | app | no |  | missing |
| `settings_get` | app | yes |  | missing |
| `settings_get_global` | app | no |  | missing |
| `settings_import` | app | no |  | missing |
| `settings_list_jetbrains_ides` | app | no |  | missing |
| `settings_read_jetbrains` | app | no |  | missing |
| `settings_set` | app | yes |  | missing |
| `spell_check` | spell | yes |  | missing |
| `spell_suggest` | spell | yes |  | missing |
| `spring_route_sources` | nav | no |  | missing |
| `ssh_askpass_answer` | ssh | no |  | missing |
| `ssh_connected` | ssh | no |  | missing |
| `ssh_disconnect` | ssh | no |  | missing |
| `ssh_needs_secret` | ssh | no |  | missing |
| `ssh_set_secret` | ssh | no |  | missing |
| `ssh_terminal_command` | ssh | no |  | missing |
| `ssh_test` | ssh | no |  | missing |
| `ssh_trust_host` | ssh | no |  | missing |
| `structural_languages` | analysis | yes |  | missing |
| `structural_search` | analysis | yes |  | missing |
| `structural_validate` | analysis | yes |  | missing |
| `syntax_errors` | syntax | yes |  | missing |
| `syntax_expression_at` | syntax | yes |  | missing |
| `syntax_identifiers` | syntax | yes |  | missing |
| `syntax_identifiers_in_files` | syntax | yes |  | missing |
| `syntax_locals` | syntax | yes |  | missing |
| `syntax_node_at` | syntax | yes |  | missing |
| `syntax_structural_ranges` | syntax | yes |  | missing |
| `syntax_support` | syntax | yes |  | missing |
| `system_font_families` | fonts | no |  | missing |
| `term_available_shells` | term | yes |  | missing |
| `term_default_shell` | term | yes | yes | missing |
| `term_is_busy` | term | yes |  | missing |
| `term_kill` | term | yes | yes | missing |
| `term_resize` | term | yes |  | missing |
| `term_spawn` | term | yes | yes | missing |
| `term_store_image` | term | yes |  | missing |
| `term_write` | term | yes | yes | missing |
| `tools_install` | tools | yes | yes | missing |
| `tools_list` | tools | yes | yes | missing |
| `tools_uninstall` | tools | yes | yes | missing |
| `undo_workspace_edit` | lsp | yes | yes | missing |
| `vcs_add_to_ignore` | vcs | yes |  | missing |
| `vcs_add_worktree` | vcs | yes |  | missing |
| `vcs_apply_patch` | vcs | yes |  | missing |
| `vcs_blame` | vcs | yes |  | missing |
| `vcs_block_history` | vcs | yes |  | missing |
| `vcs_branch_info` | vcs | yes | yes | missing |
| `vcs_branches` | vcs | yes | yes | missing |
| `vcs_changelists_load` | vcs | yes |  | missing |
| `vcs_changelists_save` | vcs | yes |  | missing |
| `vcs_checkout` | vcs | yes | yes | missing |
| `vcs_clone` | vcs | yes |  | missing |
| `vcs_commit` | vcs | yes | yes | missing |
| `vcs_commit_files` | vcs | yes |  | missing |
| `vcs_commits_changes` | vcs | yes | yes | missing |
| `vcs_compare_paths` | vcs | yes |  | missing |
| `vcs_compare_scope` | vcs | yes |  | missing |
| `vcs_config_get` | vcs | yes |  | missing |
| `vcs_conflict_versions` | vcs | yes |  | missing |
| `vcs_create_branch` | vcs | yes | yes | missing |
| `vcs_create_patch` | vcs | yes |  | missing |
| `vcs_create_tag` | vcs | yes |  | missing |
| `vcs_delete_branch` | vcs | yes | yes | missing |
| `vcs_delete_tag` | vcs | yes |  | missing |
| `vcs_detected_git` | vcs | yes |  | missing |
| `vcs_diff_hunks` | vcs | yes | yes | missing |
| `vcs_diff_numstat` | vcs | yes |  | missing |
| `vcs_diff_revisions` | vcs | yes |  | missing |
| `vcs_diff_text` | vcs | yes |  | missing |
| `vcs_diff_with_worktree` | vcs | yes |  | missing |
| `vcs_discard` | vcs | yes | yes | missing |
| `vcs_edit_remote` | vcs | yes |  | missing |
| `vcs_file_at` | vcs | yes | yes | missing |
| `vcs_git_version` | vcs | yes |  | missing |
| `vcs_has_hook` | vcs | yes |  | missing |
| `vcs_init` | vcs | yes |  | missing |
| `vcs_line_hunks` | vcs | yes |  | missing |
| `vcs_log` | vcs | yes | yes | missing |
| `vcs_log_details` | vcs | yes | yes | missing |
| `vcs_log_details_batch` | vcs | yes |  | missing |
| `vcs_log_refs` | vcs | yes |  | missing |
| `vcs_log_users` | vcs | yes |  | missing |
| `vcs_merge` | vcs | yes |  | missing |
| `vcs_merge_base` | vcs | yes |  | missing |
| `vcs_merge_with` | vcs | yes |  | missing |
| `vcs_move_worktree` | vcs | yes |  | missing |
| `vcs_open_external_log` | vcs | yes |  | missing |
| `vcs_operation` | vcs | yes |  | missing |
| `vcs_patch_preview` | vcs | yes |  | missing |
| `vcs_rebase` | vcs | yes |  | missing |
| `vcs_rebase_with` | vcs | yes |  | missing |
| `vcs_remote` | vcs | yes |  | missing |
| `vcs_remotes` | vcs | yes |  | missing |
| `vcs_remove_worktree` | vcs | yes |  | missing |
| `vcs_rename_branch` | vcs | yes |  | missing |
| `vcs_restore_branch` | vcs | yes |  | missing |
| `vcs_restore_head` | vcs | yes |  | missing |
| `vcs_restore_tag` | vcs | yes |  | missing |
| `vcs_revision_children` | vcs | yes |  | missing |
| `vcs_revision_files` | vcs | yes |  | missing |
| `vcs_reword` | vcs | yes |  | missing |
| `vcs_roots` | vcs | yes |  | missing |
| `vcs_shelf_delete` | vcs | yes |  | missing |
| `vcs_shelf_diff` | vcs | yes |  | missing |
| `vcs_shelf_import` | vcs | yes |  | missing |
| `vcs_shelf_patch` | vcs | yes |  | missing |
| `vcs_shelf_rename` | vcs | yes |  | missing |
| `vcs_shelve` | vcs | yes |  | missing |
| `vcs_shelves` | vcs | yes |  | missing |
| `vcs_stage` | vcs | yes | yes | missing |
| `vcs_stage_hunks` | vcs | yes |  | missing |
| `vcs_stage_patch` | vcs | yes |  | missing |
| `vcs_stash_apply` | vcs | yes |  | missing |
| `vcs_stash_drop` | vcs | yes |  | missing |
| `vcs_stash_push` | vcs | yes |  | missing |
| `vcs_stashes` | vcs | yes |  | missing |
| `vcs_status` | vcs | yes | yes | missing |
| `vcs_tags` | vcs | yes |  | missing |
| `vcs_undo_commit` | vcs | yes |  | missing |
| `vcs_unshelve` | vcs | yes |  | missing |
| `vcs_unstage` | vcs | yes | yes | missing |
| `vcs_worktrees` | vcs | yes |  | missing |
| `vcs_write_patch` | vcs | yes |  | missing |
| `web_browsers` | web | no |  | missing |
| `web_goto` | nav | yes |  | missing |
| `web_open` | web | no |  | missing |
| `web_unresolved_references` | nav | yes |  | missing |
| `web_url` | web | no |  | missing |
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
| `workspace_attach` | workspace | no |  | missing |
| `workspace_create` | workspace | no |  | missing |
| `workspace_discover_projects` | workspace | no |  | missing |
| `workspace_update` | workspace | no |  | missing |
| `wsl_distros` | wsl | no |  | missing |
| `wsl_resolve` | wsl | no |  | missing |

446 commands are served over stdio and 85 are called by GoIDEA today. A command
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

The desktop shell forwards these as one Tauri event (`ide://event`). Over stdio
they are `event` notifications; high-volume streams use `channel` notifications
instead (RPC_PROTOCOL.md).

### Streaming channels

| Command | Streaming arguments | Status |
| --- | --- | --- |
| `agent_registry_install` | `onEvent` | missing |
| `agent_subscribe` | `onEvent` | missing |
| `build_compile_file` | `onEvent` | missing |
| `build_run` | `onEvent` | missing |
| `build_sync` | `onProgress` | missing |
| `fs_compare_directories` | `onProgress` | missing |
| `inspect_code` | `onEvent` | missing |
| `jdk_download` | `onEvent` | missing |
| `lsp_code_action_query` | `only` | missing |
| `lsp_code_actions` | `only` | missing |
| `lsp_goto_implementation` | `onPartial` | missing |
| `lsp_references` | `onPartial` | missing |
| `notebook_execute` | `onOutput` | missing |
| `plugin_service_log` | `on`, `onLine` | missing |
| `plugin_terminal_session_spawn` | `onOutput` | missing |
| `replace_in_files` | `onEvent` | missing |
| `replace_preview` | `only` | missing |
| `run_start` | `onOutput` | missing |
| `sdk_project_libraries` | `onGroups` | missing |
| `sdk_python_create_venv` | `onOutput` | missing |
| `sdk_python_pip` | `onOutput` | missing |
| `search_start` | `onEvent` | missing |
| `structural_search` | `onEvent` | missing |
| `term_spawn` | `onOutput` | missing |
| `tools_install` | `onEvent` | missing |
| `vcs_clone` | `onOutput` | missing |
| `vcs_operation` | `onOutput` | missing |
| `vcs_rebase` | `onto` | missing |
| `vcs_remote` | `onOutput` | missing |

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
| `ide-host` | The IDE backend without a UI toolkit: projects per window, terminals, processes, and the JSON-RPC server for remote development | yes | engine-only - linked into ide-host |
| `ide-http` | HTTP Client: .http request files, environments, execution and response history | yes | engine-only - linked into ide-host |
| `ide-index` | Background project index: files and symbols, persisted in redb, fuzzy queried with nucleo | yes | engine-only - linked into ide-host |
| `ide-jsonrpc` | Content-Length framed transport and JSON-RPC 2.0 peer shared by LSP and DAP clients | yes | engine-only - linked into ide-host |
| `ide-language` | Language registry (languages/*.toml), detection, and tree-sitter powered outline/folding/symbol tags | yes | engine-only - linked into ide-host |
| `ide-lsp` | Language server manager: lifecycle, document sync, feature requests mapped to IDE types | yes | engine-only - linked into ide-host |
| `ide-mcp` | The IDE's MCP server: Streamable HTTP on 127.0.0.1 for AI agents outside the IDE, its discovery file and the stdio bridge | yes | engine-only - linked into ide-host |
| `ide-notebook` | Lossless native notebook documents and owned Jupyter kernels | yes | engine-only - linked into ide-host |
| `ide-plugin` | WebAssembly runtime for plugins (wasmtime component model + WASI preview 2) | yes | engine-only - linked into ide-host |
| `ide-profiler` | Run with Profiler: sampling-profiler launch plans (Node.js --cpu-prof, py-spy, async-profiler, Linux perf) and a common call-tree model from .cpuprofile, collapsed stacks and perf script output | no | missing |
| `ide-remote` | Remote work over SSH: SFTP deployment (mappings, upload, download, sync) and run targets (SSH, Docker, Compose) | yes | engine-only - linked into ide-host |
| `ide-run` | Run configurations: detection, persistence, process lifecycle and output streaming | yes | engine-only - linked into ide-host |
| `ide-sdk` | SDKs / toolchains: interpreter discovery (Python, Node.js, Rust, Go, TypeScript), virtualenvs, pip, and applying the selected toolchain to language servers, run configurations and terminals | yes | engine-only - linked into ide-host |
| `ide-search` | Find / Replace in Files built on the ripgrep engine crates | yes | engine-only - linked into ide-host |
| `ide-spell` | Spell checking (Proofreading › Typo): word extraction from comments, strings and declared names, Hunspell en_US / en_GB dictionary, programming terms, user dictionaries | yes | engine-only - linked into ide-host |
| `ide-ssh` | SSH connections: configurations, host keys, authentication, remote commands, SFTP, port forwarding and askpass | yes | engine-only - linked into ide-host |
| `ide-syntax` | Tree-sitter structure service: node/expression lookup, syntax errors, scopes & locals, identifier occurrences, structural ranges (LSP fallback) | yes | engine-only - linked into ide-host |
| `ide-terminal` | Integrated terminal PTY sessions | yes | engine-only - linked into ide-host |
| `ide-text` | Rope-backed documents, change sets and UTF-16/byte coordinate conversion | yes | engine-only - linked into ide-host |
| `ide-tools` | Installer for language servers and debug adapters: registry, detection, download/extract, managed tools dir | yes | engine-only - linked into ide-host |
| `ide-vcs` | Git integration: gitoxide for fast reads, git CLI for writes | yes | engine-only - linked into ide-host |
| `ide-vfs` | File system access: directory listing, text loading/saving with encoding detection, watching, project walking | yes | engine-only - linked into ide-host |
| `ide-web` | Web browsers: the built-in static web server that serves project files on 127.0.0.1 (IDEA's built-in server) and detection / launching of installed browsers | no | missing |
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

