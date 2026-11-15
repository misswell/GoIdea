# Build, run and debug

Builds and launches are OpenIDEA's. GoIDEA asks the backend for configurations,
shows the output, and runs the DAP session. Everything below was verified with
`scripts/e2e.sh` on a real Maven project and the shipped server.

## Run configurations

- After trust, `run_configs` returns what the backend detected: Maven goals,
  Gradle tasks, and application configurations (`java`, `application`,
  `spring-boot`).
- GoIDEA prefers a non-test configuration with a debug adapter or an
  application kind, otherwise the first non-test one
  (`internal/app/run.go:186-208`). The chosen configuration is shown in the
  status bar and in the run window with its name and command.
- A configuration is the backend's; GoIDEA does not parse `pom.xml` or
  `build.gradle` itself. The Maven/Gradle goal text, JDK and classpath come from
  the backend, which keeps run and debug on one resolver
  (`run_configs` and `java_resolve_launch`).

## Edit Configurations, Re-run and Build Before Launch

- **编辑运行配置…** (`Ctrl+Alt+Shift+R`, `run.editConfigs`) opens the
  Run/Debug Configurations dialog (`internal/app/run_config_dialog.go`), a
  port of OpenIDEA's ConfigForm for the fields GoIDEA edits: name, command,
  program arguments (parsed and formatted like upstream `parseArgs` /
  `formatArgs`), working directory, environment variables, the JVM module
  (left empty, it is resolved through the backend's indexed search at save),
  and the **Build** select whose options follow the kind exactly as
  `ConfigForm.tsx` filters them. Saving validates like `configs.ts`
  (unique name, required command, environment and directory rules), then
  persists the user configurations with `run_save_configs` and the ids of
  deleted detected ones; editing a detected configuration makes a user copy
  with its id, which overrides it (upstream `upsertConfig`).
- **重新运行** (`Cmd+R`, `run.rerun`) launches the configuration the pane
  started as saved now (`rerunConfig`), and when it is already running shows
  OpenIDEA's "Process 'x' Is Running" confirm (`confirmRerun`) with
  **不再询问** stored under `run.confirmRerun`; the answer stops the process,
  waits for its exit and starts it again. A configuration with **Allow
  multiple instances** restarts too, since GoIDEA runs one session per pane.
- **Build Before Launch** is the configuration's `buildPolicy`. JVM
  applications resolve it through the shared preparation
  (`BuildPolicyOf`); Maven and Gradle rewrite their command at launch like
  `buildCommand.ts` (`rebuild` prepends `clean`, Maven's
  `makeAndContinueOnErrors` appends `-Dmaven.compiler.failOnError=false`).

The e2e base phase runs `GOIDEA_SMOKE_RUNCFG=1`: duplicating the detected
Maven configuration into a user copy, saving it, reading it back edited from
`run_configs`, resolving `clean package` from the rebuild policy, running it
for real and deleting it again.

## Running

`run_start {config, onOutput, request:null}` streams output on the `channel`
notification (`internal/app/run.go:271-289`). stdout, stderr and system lines
are tagged, ANSI is stripped, and the view keeps the last 3000 lines
(`run.go:443-504`). `runExited` records the exit code and status
(`run.go:461-481`); `run_stop {id}` has a 15 s budget (`run.go:316-338`).
Before starting, open documents are saved, so the build never sees stale text.

The e2e base phase runs the detected Maven configuration on
`testdata/java-maven`, requires exit code 0 and requires `BUILD SUCCESS` in the
output. This is a real `mvn` process, not a recorded transcript.

## Run-preparation negative regressions (9.3)

The e2e phase 1c proves, on the real server, that a failed preparation never
reaches `run_start`:

- `GOIDEA_SMOKE_NEGJDTLS=1` runs against a **fresh data directory with no
  Java toolchain**: the backend detects no application configuration (its
  model comes from the language server), the smoke persists a real user
  configuration through `run_save_configs`, and launching it is refused with
  the server's real verdict ("openidea: unavailable: jdtls is not installed"),
  no process started, no orphan JVM.
- `GOIDEA_SMOKE_RUNSWITCH=1` starts a real Maven run and **switches the
  project while it is in flight**: the switch stops the process
  (`runningAfterSwitch == 0`), and the new project's run pane is clean. This
  exposed and fixed two real defects: the manager ran its restart-restore
  callback on the first launch too, racing the project being opened
  (a phantom "后端重启后恢复项目失败" that set the app to failed although
  nothing had restarted), and the switch smoke compared an unresolved
  `/tmp` symlink path.
- `GOIDEA_SMOKE_RUNKILL=1` **kills the server with SIGKILL in the middle of
  a launch preparation**: the launch is refused with the real error
  ("启动失败：openidea: connection closed"), nothing starts, the backend
  restarts and restores the project, and the same configuration then runs to
  its exit.

## Choosing and debugging configurations

- **选择运行/调试配置…** (`Ctrl+Alt+R`, `run.chooseConfig`) opens the
  configuration chooser (`internal/app/run_config_chooser.go`), the port of
  upstream ConfigChooser: Enter runs the chosen configuration as saved now
  (through the same stop-and-rerun confirm), **Shift+Enter** debugs it,
  `Ctrl+Alt+D` (`run.chooseDebugConfig`) opens it in debug mode, and the
  title bar widget opens it in select mode. The **当前文件** row debugs the
  active file's own configuration.
- **调试** (`F9`, `run.debug`) debugs the **selected** run configuration;
  **调试当前文件** (`Ctrl+Shift+D`, `run.debugContext`) debugs the active
  file's, as upstream's run.debug / run.debugContext split. Both go through
  the shared `PrepareJavaLaunch` and `javaDebugLaunch`; a Maven or Gradle
  configuration and an unshipped adapter (Kotlin) are refused with the real
  reason instead of silently starting. The e2e negative phase's
  `GOIDEA_SMOKE_DBGCFG=1` debugs the detected application configuration of
  the real Maven project to its end.
- **重新运行失败的测试** (`run.rerunFailed`) is not ported yet: it needs the
  test-results pipeline upstream builds from the run console's test output
  (the `testing` feature's parsers and results tree). It stays listed
  missing in `PARITY_AUDIT.md` until that pipeline exists — no stub entry
  is offered.

## Java launch preparation (shared by Run and Debug)

`internal/adapter` ports OpenIDEA's `platform/javaLaunch.ts` function by
function, and Run and Debug both go through `PrepareJavaLaunch` — the same
shared preparation upstream's `prepareJavaLaunch` gives its Run store and
debugger:

- `ParseJavaArgs` splits VM options, the main class and program arguments, and
  reads which classpath placeholder the configuration uses
  (`${classpath}` vs `${testClasspath}`); the placeholder, not `isTest`,
  decides the test classpath `java_resolve_launch` builds.
- The module comes from the configuration's own `module` field, else from the
  backend's indexed file search (`find_files`) plus the source file's package
  statement — never from a local directory walk. A nested main class resolves
  through its outer class; `FooKt` resolves through `Foo.kt`.
- `java_resolve_launch` builds, resolves runtime/test classpaths and module
  paths, and reports the JDK. Spring Boot launches add the console ANSI
  property like IDEA's configurations do.
- Every failure refuses the launch and reports the real reason in the run
  console: an unimported Java module ("请等待 Java 项目导入完成…"), a failed
  build with the compiler's `path:line:col` errors and hints, an empty
  classpath, an unresolvable module, a malformed reply, an RPC failure, a
  cancelled request. `makeAndContinueOnErrors` (Build, no error check) is the
  only policy that continues with the last compiled classes, exactly as
  upstream does.

The e2e release phase `GOIDEA_SMOKE_SPRING=1` runs the detected Spring Boot
configuration of `testdata/java-spring-multimodule` end to end: it waits for
the Maven model and Java import, requires the nested `service` module (not the
root), a real resolved classpath with no placeholder left, the JVM actually
running, the `GREETING:` startup marker that proves the sibling `lib` module
was on the classpath, a clean `run_stop` and no orphan JVM.

## Building

OpenIDEA's Build actions run through the backend's build engine:

- **构建项目** (`Cmd+F9`, `build.buildProject`) runs `build_run {kind:"build"}`
  for the project.
- **构建模块** (`Cmd+Shift+F9`, `build.buildModule`) passes the active file as
  `file`, so the engine builds that file's module.
- **重新构建项目** (`build.rebuildProject`) uses `kind:"rebuild"`, which cleans
  first; **构建工件** (`build.buildArtifacts`) uses `kind:"artifacts"`.
- **停止构建** (`build.stop`) cancels the request with `cancel_request`.

Each build streams `BuildEvent`s on `onEvent`: `started`, `step`, `output`,
`message` and `progress` (`internal/app/build.go`). The Build window shows the
step commands and console output (last 20,000 lines), the progress text, and at
the end the result with its error/warning counts and duration. Messages are
counted in the console but reach the editors and Problems view as the backend's
`build` diagnostics. Before a build, open documents are saved; the next build
passes the previous build's message files as `clear`. Compiler messages are
those of the engine; GoIDEA does not run `mvn` or `gradle` itself.

**编译文件** (`build.compileFile`) is not ported: the pinned v0.1.44 server
does not serve `build_compile_file` (it exists only in the pinned source), so
the action is recorded `blocked` in `OPENIDEA_PARITY_MATRIX.md` until a server
release or submodule bump ships it.

The e2e base phase runs `GOIDEA_SMOKE_BUILD=1`: `build_run` on
`testdata/java-maven` must succeed, stream at least two console lines and show
`BUILD SUCCESS` (or the engine's up-to-date line when the prior run already
built the project).

Not in the UI yet: the Build view's message tree (messages are counted in the
console and shown as diagnostics instead), the interrupted-build context, and
a dedicated test-run view.

## Debug pipeline

1. The user sets line breakpoints in Java files (`Cmd+F8`). Breakpoints are stored per
   path and pushed wholesale with `debug_set_breakpoints`
   (`internal/app/debug.go:96-196`). The gutter and the paused line are drawn
   from that state.
2. Starting a session saves all documents, builds the temporary Application
   configuration the file belongs to (module from the run configs' modules and
   the source layout, like upstream's `applicationConfig`), and calls the same
   shared `adapter.PrepareJavaLaunch` Run uses — one module, build and
   classpath resolver for both.
3. `java_resolve_launch {module, test, policy}` inside that preparation returns
   the launch plan — main class, classpaths, module paths, project root, JDK,
   `shortenCommandLine`, compile errors and hints. A failed or erroneous build
   stops the debug launch, unless the policy is `makeAndContinueOnErrors`.
4. `java_debug_server {module}` returns the DAP port; when java-debug is
   missing from jdtls the app offers the one-click install and re-raises the
   error.
5. `debug_start` launches with adapter `java-debug`, request `launch`, and the
   resolved `debugServer`, `mainClass`, `classPaths` (the explicit `-cp` value
   when the configuration sets one), `modulePaths`, `projectName` (the imported
   project's name), the configuration's VM and program arguments, javaExec,
   `shortenCommandLine`, console `internalConsole`, `stopOnEntry:false`. The
   equivalent command line is echoed into the debug console, as upstream does,
   because java-debug starts the JVM itself.
6. DAP events (`started`, `stopped`, `continued`, `output`, `terminated`,
   `exited`, `hotCodeReplace`) drive the debug panel. On a stop the app reads
   threads, frames and the first non-expensive scope's variables; frame
   selection reloads variables (`debug.go:633-779`).
7. Step over/into/out, pause/resume and stop map to `debug_step`,
   `debug_pause`, `debug_resume`, `debug_stop`. Watch expressions are pinned
   (max 20) and evaluated per stopped frame with `debug_evaluate`
   (`debug.go:536-618,782-932`).

The JDK is the backend's `javaExec`; when the backend does not resolve one the
launch keeps `java`, as upstream `resolveJavaCommand` does — there is no
client-side fallback.

## What the gate proves

`GOIDEA_SMOKE_DEBUG=1` (the `debug` phase of the e2e run) requires:

- the session paused at a breakpoint on line 7 of `com.example.App`;
- three variables and the watch expression `sum` evaluating to `4`;
- step over pausing again on the next line;
- the program ending with no compile errors and `Hello, GoIDEA!` in the
  captured output.

`GOIDEA_SMOKE_SPRING=1` (the release gate's `spring` phase) requires, on the
real multi-module Maven project `testdata/java-spring-multimodule`:

- a `spring-boot` configuration detected for the nested `service` module
  (never the root) with main class `com.example.service.ServiceApplication`
  and the `-cp ${classpath}` command line the backend detected;
- `java_resolve_launch` resolving a runtime classpath with at least five
  entries and no `${classpath}`/`${testClasspath}` placeholder left;
- a resolved `java` executable, the process really running (`run_list`);
- the `GREETING:Hello, GoIDEA!` marker, which only prints when Spring started
  and the sibling `lib` module was on the resolved classpath;
- `run_stop` ending the process and `pgrep` finding no orphan JVM.

## Test results and rerun failed tests

`internal/adapter/testing_results.go`, `testing_jvm.go` and `testing_rerun.go`
port the testing pipeline's `results.ts`, `jvm.ts` and `rerun.ts` (JVM
branch): Maven Surefire's failure lines and summaries (Surefire 2 and 3
formats), Gradle's test logging and its init script's TeamCity service
messages fold into a results tree (`TestResults`, with aggregated statuses,
counts and the first failure). The run pane feeds the process output to the
parser and finishes the tree with the process. **重新运行失败的测试**
(`run.rerunFailed`, the run pane's button next to Run) derives a
configuration named `Failed: <name>` with Maven's `-Dtest=Class#m1+m2` or
Gradle's `--tests` filter and starts it like any configuration. The e2e
phase 1b2 (`GOIDEA_SMOKE_RERUNFAIL=1`) runs a real failing JUnit method in
`testdata/java-junit-failed`: the tree reports one failure, the derivation
produces `-Dtest=com.example.CalculatorTest#failsOnPurpose`, and the rerun's
surefire reports `Tests run: 1`. Not ported yet: the results **tree view**
(the console shows the failure details), the other frameworks' parsers, and
Gradle's init-script wiring (`run_gradle_jvm_args_script`).

## HotSwap (Reload Changed Classes)

**重新加载变更的类（HotSwap）** (`debug.reloadChangedClasses`,
`internal/app/hotswap.go`) ports `hotswap.ts`: save the open documents,
compile the session's module incrementally with `java_build_workspace`, then
reload through `debug_redefine_classes` into the running java-debug session.
The result wording follows IDEA: nothing to reload, `N` classes reloaded,
"JVM contains obsolete methods" as a warning with the re-entry hint, and a
failed compilation refuses the reload instead of applying stale classes.
jdtls finishing a build on its own (`hotCodeReplace` `BUILD_COMPLETE`) offers
the reload, as IDEA's default "Reload classes after compilation: Ask" does.
The e2e release phase 4b (`GOIDEA_SMOKE_HOTSWAP=1`) pauses at a breakpoint,
edits `Greeter.java` through the document bridge, reloads (1 class) and
requires the resumed program to print the NEW text.

## Gaps (deliberately not claimed)

- Conditional, hit-count and log breakpoints are not sent (the protocol types
  exist upstream).
- Attach mode, exception breakpoints, a thread chooser, a scope chooser,
  variable child expansion and multi-session debugging have no UI yet.
- Breakpoint verification state from `breakpointsChanged` is ignored.
- Run and debug share the backend's resolver, but there is no client-side
  launch-configuration editor.
