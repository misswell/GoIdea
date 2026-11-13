# Java support

Java support is OpenIDEA's: project model, JDT language server, Maven/Gradle,
javac diagnostics and the java-debug adapter. GoIDEA contributes the native UI,
the toolchain installer, and the RPC glue under `internal/app`. Nothing here is
a mock: the end-to-end gate builds a real Maven project with the local `mvn`.

## Opening a Java project

`open_project` takes the folder; the backend detects Maven, Gradle and plain
source layouts. GoIDEA polls `project_current` until the backend reports it
(60 s budget), then asks for trust. Build, run, debug and tool installation are
refused until the user trusts the folder (`internal/app/run.go:60-143`).

Supported project shapes are whatever the backend detects; the repository ships
fixtures that are exercised:

| Fixture | Tool | Command |
| --- | --- | --- |
| `testdata/java-maven` | Maven, JDK 17 | `mvn -o package` |
| `testdata/java-junit` | Maven + JUnit 5 (surefire 3.5.6) | `mvn -o test` |
| `testdata/java-multimodule` | Maven reactor | `mvn -o package` |
| `testdata/java-lombok` | Maven + Lombok 1.18.38, JDK 11 | `mvn -o package` |
| `testdata/java-spring-boot` | Spring Boot 2.7.18 | `mvn package -DskipTests` |
| `testdata/java-gradle` / `java-gradle-kts` | Gradle Groovy / Kotlin DSL | `gradle build` |

`GOIDEA_E2E_FIXTURES=1 scripts/e2e.sh` builds them with the real toolchain.

## Toolchain install

JDT LS and java-debug are not bundled. The backend manages them under
`<dataDir>/openidea-server/data/tools/<name>`; GoIDEA's job is to list, install,
show progress and reload servers:

- Settings → *Java 语言服务与调试器* (`internal/app/settings_ui.go`), command
  palette → *安装 Java 语言服务与调试器*, or the missing-tool status line.
- `tools_list` reports `installed | onPath | custom | missing`; only missing
  tools are installed (`internal/app/tools.go`).
- `tools_install {name, request, onEvent:{$channel}}` streams `line` and
  `progress` events. The sheet shows the fraction and the last log lines; the
  install is cancellable through `cancel_request`.
- A failed download is retried once after 2 s (the Eclipse mirror occasionally
  breaks HTTP/2 streams). A second failure is reported, not hidden.
- After a successful install the app calls `reg.ReopenAll` so open documents are
  served by the new JDT LS without restarting GoIDEA.
- `GOIDEA_SMOKE_TOOLS=install` performs the same flow offscreen and fails when
  a tool is still missing afterwards.

The install never writes to the user's OpenIDEA installation or its `jdtls`
processes; `OPENIDEA_SERVER_HOME` keeps the state inside GoIDEA's data dir.

## Language services

`GOIDEA_SMOKE_LSP=1` and the `lsp` phase of `scripts/e2e.sh` exercise, on the
real fixture:

- completion (50+ items on `App.java`), hover documentation, go to definition
  resolving to `App.java`;
- formatting, find usages with completion of the request, rename applied and
  undone through the workspace-edit undo stack;
- go to symbol (type) and diagnostics published by JDT.

The UI exposes the same calls: completion/hover/definition popups in the
editor, Alt+Enter intentions, Alt+F7 usages, Shift+F6 rename, Cmd+Shift+O
symbols, Edit → Format, and the Problems list fed by diagnostics.

## Multi-module, JUnit, Lombok

- The backend resolves modules from `src/main/java` and `src/test/java`; the
  client sends the module path to `java_resolve_launch` and asks for the DAP
  server per module (`internal/app/debug.go:238-269`).
- JUnit tests are build/run targets of the backend's Maven/Gradle detection;
  GoIDEA does not yet have a dedicated test-results UI.
  `testdata/java-junit` proves the real Surefire run.
- Lombok is annotation processing inside JDT and javac; the fixture builds with
  JDK 11. JDT LS must run on a JDK it supports — the installer reports the
  version it chose.

## Multi-JDK

The backend picks the project JDK and reports `javaExec`; the debugger falls
back to `$JAVA_HOME/bin/java` (`debug.go:373-380`). A per-project JDK picker is
not implemented in GoIDEA's UI yet, and is listed as `ADAPTING` in
`UPSTREAM_FEATURE_MATRIX.md` rather than claimed as working.

## Known gaps (not claimed as integrated)

- Spring Boot application run/stop is not yet covered by the regression gate;
  the gate currently proves a Maven goal run and a plain Java main-class debug
  session.
- Separate Build / Rebuild / Clean actions are not in the UI; run and debug
  trigger the backend's build.
- Conditional breakpoints, attach mode, thread/scope choosers and variable
  child expansion are backend features without UI yet.
