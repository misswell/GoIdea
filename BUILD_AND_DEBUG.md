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

## Gaps (deliberately not claimed)

- Conditional, hit-count and log breakpoints are not sent (the protocol types
  exist upstream).
- Attach mode, exception breakpoints, a thread chooser, a scope chooser,
  variable child expansion and multi-session debugging have no UI yet.
- Breakpoint verification state from `breakpointsChanged` is ignored.
- Run and debug share the backend's resolver, but there is no client-side
  launch-configuration editor.
