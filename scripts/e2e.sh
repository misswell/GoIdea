#!/usr/bin/env bash
# Runs the end-to-end regression against real Java projects and the real
# openidea-server: editing, search, Git, settings, Maven build/run, the Java
# toolchain install, JDT language services and the debugger, all through the
# offscreen smoke hooks of a hidden window.
#
# Usage: scripts/e2e.sh
#   GOIDEA_E2E_OFFLINE=1   skip the phases that need a download (tools, LSP,
#                          debug, extended toolchains) and say so; the
#                          build/search/Git/Run phases still run.
#   GOIDEA_E2E_FIXTURES=1  also build the JUnit, multi-module, Lombok and
#                          Spring Boot fixtures with the real mvn.
#   GOIDEA_E2E_DATA=<dir>  reuse a data directory (default build/e2e-data), so
#                          jdtls/java-debug are downloaded once.
#   GOIDEA_E2E_BIN=<path>  test an already built binary.
#   GOIDEA_GRADLE_HOME=<dir>  Gradle 8.10.2 for the published catalog phase.
#   GOIDEA_GRADLE=<path>      the Gradle 8.10.2 executable, when not on PATH.
# Exits 0 only when every run passed its assertions; a missing download or
# toolchain fails the run instead of being counted as a pass.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="${GOIDEA_E2E_BIN:-$ROOT/build/goidea}"
SERVER="${GOIDEA_SERVER:-$ROOT/resources/openidea-server}"
DATA="${GOIDEA_E2E_DATA:-$ROOT/build/e2e-data}"
CHECKS="$ROOT/scripts/e2e-checks.py"
OFFLINE="${GOIDEA_E2E_OFFLINE:-0}"

say() { printf '\n==> %s\n' "$*"; }
skip() { printf 'SKIP: %s\n' "$*"; }

[ -x "$SERVER" ] || {
    echo "缺少 openidea-server：$SERVER（先运行 scripts/fetch-server.sh）" >&2
    exit 1
}
if [ -z "${GOIDEA_E2E_BIN:-}" ]; then
    say "构建 build/goidea"
    CGO_ENABLED=0 go build -o "$BIN" ./cmd/goidea
fi
mkdir -p "$DATA"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/goidea-e2e.XXXXXX")"
cleanup() {
    status=$?
    if [ $status -ne 0 ] && [ "${GOIDEA_E2E_KEEP:-0}" = "1" ]; then
        echo "kept smoke output in $WORK" >&2
    else
        rm -rf "$WORK"
    fi
    exit $status
}
trap cleanup EXIT INT TERM

# The project is a copy: the smoke edits, formats and renames files, and the
# fixture in testdata must stay pristine. An uncommitted change gives the Git
# diff something to show.
PROJ="$WORK/java-maven"
cp -R "$ROOT/testdata/java-maven" "$PROJ"
rm -rf "$PROJ/target"
git -C "$PROJ" init -q
git -C "$PROJ" add -A
git -C "$PROJ" -c user.name=e2e -c user.email=e2e@goidea.invalid commit -qm baseline
printf '\n// e2e change\n' >> "$PROJ/src/main/java/com/example/Greeter.java"
JAVA="$PROJ/src/main/java/com/example/App.java"
BASE_PROJ="$PROJ"
BASE_JAVA="$JAVA"
# The TODO scan needs one marker in the open file; the base phase edits and
# restores this copy, so the line survives to the scan.
printf '\n// TODO: e2e marker\n' >> "$JAVA"

# smoke runs one offscreen phase in a hidden window and leaves its JSON at
# $1; the remaining arguments are GOIDEA_SMOKE_* variables for that phase.
smoke() {
    local out="$1"
    shift
    env GOIDEA_HEADLESS=1 GOIDEA_DATA_DIR="${GOIDEA_SMOKE_DATA:-$DATA}" GOIDEA_SERVER="$SERVER" \
        GOIDEA_OPEN_PROJECT="$PROJ" GOIDEA_OPEN_FILES="$JAVA" \
        GOIDEA_SMOKE_OUT="$out" "$@" "$BIN" >"${out%.json}.log" 2>&1 ||
        { echo "goidea 退出异常，日志 ${out%.json}.log：" >&2; tail -30 "${out%.json}.log" >&2; return 1; }
    [ -s "$out" ] || { echo "冒烟没有写出 $out" >&2; tail -30 "${out%.json}.log" >&2; return 1; }
}

command -v mvn >/dev/null || { echo "缺少 mvn：本测试用真实 Maven 构建 Java 项目" >&2; exit 1; }

say "阶段 0：两个原生项目窗口、独立后端与单窗关闭隔离"
WINDOW_PROJ="$WORK/java-maven-window"
cp -R "$ROOT/testdata/java-maven" "$WINDOW_PROJ"
rm -rf "$WINDOW_PROJ/target"
git -C "$WINDOW_PROJ" init -q
git -C "$WINDOW_PROJ" add -A
git -C "$WINDOW_PROJ" -c user.name=e2e -c user.email=e2e@goidea.invalid commit -qm baseline
WINDOW_JAVA="$WINDOW_PROJ/src/main/java/com/example/App.java"
WINDOWS_JSON="$WORK/windows.json"
smoke "$WINDOWS_JSON" GOIDEA_SMOKE_WINDOWS=1 \
    GOIDEA_SMOKE_WINDOW_PROJECT="$WINDOW_PROJ" \
    GOIDEA_SMOKE_WINDOW_FILES="$WINDOW_JAVA"
python3 "$CHECKS" windows "$WINDOWS_JSON"

say "阶段 0a：failed project switch 保持项目、recent 与窗口工作区"
ROLLBACK_JSON="$WORK/rollback.json"
smoke "$ROLLBACK_JSON" GOIDEA_SMOKE_ROLLBACK=1
python3 "$CHECKS" rollback "$ROLLBACK_JSON"

say "阶段 1：编辑、搜索、Git、设置、外观、Maven 构建运行"
BASE="$WORK/base.json"
COVERAGE="$WORK/coverage.info"
COVERAGE_SOURCE="$WORK/coverage-source"
ln -s "$PROJ" "$COVERAGE_SOURCE"
printf 'TN:\nSF:%s\nDA:3,1\nDA:4,0\nBRDA:3,0,0,1\nBRDA:3,0,1,0\nend_of_record\n' \
    "$COVERAGE_SOURCE/src/main/java/com/example/Greeter.java" > "$COVERAGE"
smoke "$BASE" \
    GOIDEA_SMOKE_EDIT=1 GOIDEA_SMOKE_MCP=1 GOIDEA_SMOKE_CODEINTEL=1 GOIDEA_SMOKE_SEARCH=1 GOIDEA_SMOKE_TODO=1 \
    GOIDEA_SMOKE_SCRATCH=1 GOIDEA_SMOKE_DIFF=1 GOIDEA_SMOKE_COMMANDS=1 GOIDEA_SMOKE_KEYMAP=1 GOIDEA_SMOKE_SETTINGS=1 GOIDEA_SMOKE_NOTIFY=1 GOIDEA_SMOKE_NEWPROJECT=1 \
    GOIDEA_SMOKE_DOCK=1 GOIDEA_SMOKE_COVERAGE="$COVERAGE" GOIDEA_SMOKE_WORKSPACE=1 \
    GOIDEA_SMOKE_WATCH=1 GOIDEA_SMOKE_RUN=1 GOIDEA_SMOKE_RUNCFG=1 GOIDEA_SMOKE_BUILD=1 GOIDEA_SMOKE_STAGE=1 GOIDEA_SMOKE_BOOKMARK=1 GOIDEA_SMOKE_LOG=1 GOIDEA_SMOKE_COMMIT=1 GOIDEA_SMOKE_BRANCH=1 GOIDEA_SMOKE_TERM=1 GOIDEA_SMOKE_FILES=1
python3 "$CHECKS" base "$BASE"

say "阶段 1a：Database 数据页、内联编辑与 db_apply_edits（真实 SQLite）"
DATABASE_JSON="$WORK/database.json"
smoke "$DATABASE_JSON" GOIDEA_SMOKE_DB=1
python3 "$CHECKS" database "$DATABASE_JSON"

say "阶段 1a：External Libraries（真实 openidea-server SDK 与归档）"
LIBRARIES_JSON="$WORK/libraries.json"
smoke "$LIBRARIES_JSON" GOIDEA_SMOKE_LIBRARIES=1
python3 "$CHECKS" libraries "$LIBRARIES_JSON"

say "阶段 1a：Project Structure（七个分区与真实设置保存）"
PROJECT_STRUCTURE_JSON="$WORK/project-structure.json"
smoke "$PROJECT_STRUCTURE_JSON" GOIDEA_SMOKE_PROJECT_STRUCTURE=1
python3 "$CHECKS" projectstructure "$PROJECT_STRUCTURE_JSON"

say "阶段 1a：File Types 精确路径覆盖（真实 openidea-server 设置）"
FILE_TYPES_JSON="$WORK/file-types.json"
smoke "$FILE_TYPES_JSON" GOIDEA_SMOKE_FILE_TYPES=1
python3 "$CHECKS" filetypes "$FILE_TYPES_JSON"

say "阶段 1a：Agents 订阅、Provider 与外部会话（真实 openidea-server）"
AGENTS_JSON="$WORK/agents.json"
smoke "$AGENTS_JSON" GOIDEA_SMOKE_AGENTS=1
python3 "$CHECKS" agents "$AGENTS_JSON"

say "阶段 1a：GraphQL、HTTP Client 与 Deployment（真实 openidea-server）"
FEATURE_PARITY_JSON="$WORK/feature-parity.json"
FEATURE_PARITY_LOG="$WORK/feature-parity.log"
if ! GOIDEA_SERVER="$SERVER" GOIDEA_PARITY_REPORT="$FEATURE_PARITY_JSON" \
    CGO_ENABLED=0 go test ./internal/app \
    -run '^(TestIntegrationGraphQLAndHTTPClientAgainstLocalFixtures|TestIntegrationDeploymentLocalFilesystemRoundTrip)$' \
    -count=1 >"$FEATURE_PARITY_LOG" 2>&1; then
    tail -80 "$FEATURE_PARITY_LOG" >&2
    exit 1
fi

say "阶段 1a：Gradle 8.10.2 真实 Maven publication 与 version catalog"
GRADLE_BIN="${GOIDEA_GRADLE:-}"
if [ -z "$GRADLE_BIN" ] && [ -n "${GOIDEA_GRADLE_HOME:-}" ]; then
    GRADLE_BIN="$GOIDEA_GRADLE_HOME/bin/gradle"
fi
if [ -z "$GRADLE_BIN" ]; then
    GRADLE_BIN="$(command -v gradle || true)"
fi
if [ ! -x "$GRADLE_BIN" ]; then
    echo "缺少 Gradle 8.10.2：设置 GOIDEA_GRADLE_HOME 或 GOIDEA_GRADLE" >&2
    exit 1
fi
GRADLE_CATALOG_LOG="$WORK/gradle-version-catalog.log"
if ! GOIDEA_SERVER="$SERVER" GOIDEA_GRADLE="$GRADLE_BIN" GOIDEA_PARITY_REPORT="$FEATURE_PARITY_JSON" \
    CGO_ENABLED=0 go test ./internal/app \
    -run '^TestIntegrationGradlePublishedVersionCatalog$' \
    -count=1 -timeout 20m >"$GRADLE_CATALOG_LOG" 2>&1; then
    tail -100 "$GRADLE_CATALOG_LOG" >&2
    exit 1
fi
python3 "$CHECKS" featureparity "$FEATURE_PARITY_JSON"

say "阶段 1a：Language SDK detection and Python environment (real openidea-server)"
TOOLCHAIN_LOG="$WORK/toolchains.log"
if ! GOIDEA_SERVER="$SERVER" GOIDEA_SDK_ACCEPTANCE=1 GOIDEA_PARITY_REPORT="$FEATURE_PARITY_JSON" \
    CGO_ENABLED=0 go test ./internal/openidea \
    -run '^TestIntegrationLanguageToolchainsAcceptance$' -count=1 >"$TOOLCHAIN_LOG" 2>&1; then
    tail -80 "$TOOLCHAIN_LOG" >&2
    exit 1
fi
python3 "$CHECKS" toolchains "$FEATURE_PARITY_JSON"

if [ "$OFFLINE" = "1" ]; then
    skip "Prettier、oxfmt、Ruff、uv、Poetry、Conda 与 Gradle 扩展验收（GOIDEA_E2E_OFFLINE=1）"
else
    say "阶段 1a：真实格式化器、隔离 Python 工具与 Gradle（release）"
    EXTENDED_TOOLCHAIN_CACHE="$DATA/extended-toolchains"
    mkdir -p "$EXTENDED_TOOLCHAIN_CACHE"
    EXTENDED_TOOLCHAIN_LOG="$WORK/extended-toolchains.log"
    if ! GOIDEA_SERVER="$SERVER" GOIDEA_EXTENDED_TOOLCHAIN_ACCEPTANCE=1 \
        GOIDEA_EXTENDED_TOOLCHAIN_CACHE="$EXTENDED_TOOLCHAIN_CACHE" \
        GOIDEA_PARITY_REPORT="$FEATURE_PARITY_JSON" \
        CGO_ENABLED=0 go test ./internal/openidea \
        -run '^TestIntegrationExtendedToolchainAcceptance$' \
        -count=1 -timeout 30m >"$EXTENDED_TOOLCHAIN_LOG" 2>&1; then
        tail -100 "$EXTENDED_TOOLCHAIN_LOG" >&2
        exit 1
    fi
    python3 "$CHECKS" extendedToolchains "$FEATURE_PARITY_JSON"
fi

say "阶段 1b：原生 Git 操作（真实本地远端）"
VCSOPS="$WORK/java-vcs-operations"
cp -R "$ROOT/testdata/java-maven" "$VCSOPS"
rm -rf "$VCSOPS/target"
git -C "$VCSOPS" init -q
git -C "$VCSOPS" add -A
git -C "$VCSOPS" -c user.name=e2e -c user.email=e2e@goidea.invalid commit -qm baseline
VCSOPS_JSON="$WORK/vcs-operations.json"
PROJ="$VCSOPS"
JAVA=""
smoke "$VCSOPS_JSON" GOIDEA_SMOKE_VCS_OPERATIONS=1
python3 "$CHECKS" vcsoperations "$VCSOPS_JSON"
PROJ="$BASE_PROJ"
JAVA="$BASE_JAVA"

say "阶段 1c：离屏视觉截图"
SHOTS="$WORK/shots"
mkdir -p "$SHOTS"
SHOTS_JSON="$WORK/shots.json"
grep -q "TODO: visual shot" "$JAVA" || printf '\n// TODO: visual shot\n' >> "$JAVA"
smoke "$SHOTS_JSON" GOIDEA_SMOKE_SHOTS="$SHOTS"
python3 "$CHECKS" shots "$SHOTS_JSON"

say "阶段 1c2：测试结果管线与重新运行失败的测试（真实 surefire 输出）"
REFAIL="$WORK/java-junit-failed"
cp -R "$ROOT/testdata/java-junit-failed" "$REFAIL"
rm -rf "$REFAIL/target"
say "用真实 Maven 预热 JUnit 夹具依赖"
(cd "$REFAIL" && mvn -q test >/dev/null 2>&1 || true)
REFAIL_JSON="$WORK/rerunfail.json"
PROJ_BASE="$PROJ"; JAVA_BASE="$JAVA"
PROJ="$REFAIL"
JAVA=""
smoke "$REFAIL_JSON" GOIDEA_SMOKE_RERUNFAIL=1
python3 "$CHECKS" rerunfail "$REFAIL_JSON"

say "阶段 1b4：cargo 测试管线与重新运行失败的测试"
CARUN="$WORK/cargo-app"
cp -R "$ROOT/testdata/cargo-app" "$CARUN"
rm -rf "$CARUN/target"
say "用真实 cargo 预构建夹具"
(cd "$CARUN" && cargo test >/dev/null 2>&1 || true)
CARGORUN_JSON="$WORK/cargorun.json"
PROJ="$CARUN"
JAVA=""
smoke "$CARGORUN_JSON" GOIDEA_SMOKE_CARGORUN=1
python3 "$CHECKS" cargorun "$CARGORUN_JSON"

say "阶段 1b5：pytest 测试管线与重新运行失败的测试"
if ! command -v pytest >/dev/null 2>&1; then
    say "准备 pytest：优先 venv（本机 python 可能 externally-managed）"
    if [ ! -x "$HOME/.goidea-pytest/bin/pytest" ]; then
        python3 -m venv "$HOME/.goidea-pytest" && "$HOME/.goidea-pytest/bin/pip" install --quiet pytest
    fi
    export PATH="$HOME/.goidea-pytest/bin:$PATH"
    export GOIDEA_PYTEST="$HOME/.goidea-pytest/bin/pytest"
fi
PYRUN="$WORK/pytest-app"
cp -R "$ROOT/testdata/pytest-app" "$PYRUN"
rm -rf "$PYRUN/__pycache__" "$PYRUN/.pytest_cache"
PYRUN_JSON="$WORK/pyrerun.json"
PROJ="$PYRUN"
JAVA=""
smoke "$PYRUN_JSON" GOIDEA_SMOKE_PYRERUN=1
python3 "$CHECKS" pyrerun "$PYRUN_JSON"
unset GOIDEA_PYTEST
python3 "$CHECKS" pyrerun "$PYRUN_JSON"

say "阶段 1b6：jest 测试管线与重新运行失败的测试"
JSRUN="$WORK/node-app"
cp -R "$ROOT/testdata/node-app" "$JSRUN"
rm -rf "$JSRUN/node_modules"
say "用 npm 安装 jest（真实框架安装）"
(cd "$JSRUN" && npm install --silent --no-audit --no-fund jest >/dev/null 2>&1)
JSRUN_JSON="$WORK/jsrerun.json"
PROJ="$JSRUN"
JAVA=""
smoke "$JSRUN_JSON" GOIDEA_SMOKE_JSRERUN=1
python3 "$CHECKS" jsrerun "$JSRUN_JSON"

say "阶段 1b7：Run with Profiler（真实 Node.js CPU profile）"
PROFILER_JSON="$WORK/profiler.json"
PROJ="$JSRUN"
JAVA=""
smoke "$PROFILER_JSON" GOIDEA_SMOKE_PROFILER=1
python3 "$CHECKS" profiler "$PROFILER_JSON"
if [ -n "${ASYNC_PROFILER_HOME:-}" ] &&
    [ -f "$ASYNC_PROFILER_HOME/lib/libasyncProfiler.dylib" ]; then
    say "阶段 1b8：Run with Profiler（真实 JVM allocations 与 live objects）"
    PROFILER_ALLOCATIONS_JSON="$WORK/profiler-allocations.json"
    smoke "$PROFILER_ALLOCATIONS_JSON" \
        GOIDEA_SMOKE_PROFILER=1 GOIDEA_SMOKE_PROFILER_MODE=allocations
    python3 "$CHECKS" profilerallocations "$PROFILER_ALLOCATIONS_JSON"

    PROFILER_LIVE_JSON="$WORK/profiler-live.json"
    smoke "$PROFILER_LIVE_JSON" \
        GOIDEA_SMOKE_PROFILER=1 GOIDEA_SMOKE_PROFILER_MODE=liveObjects
    python3 "$CHECKS" profilerlive "$PROFILER_LIVE_JSON"
else
    skip "JVM allocation/live-object profiler (set ASYNC_PROFILER_HOME)"
fi
PROJ="$PROJ_BASE"; JAVA="$JAVA_BASE"

READINESS_JSON="$WORK/readiness.json"

run_preparation_negatives() {
    say "阶段 1c：运行准备负向回归（任何准备失败都不得调用 run_start）"
    # A: a data directory without the Java toolchain — the application
    # configuration's preparation must fail with the real reason.
    NEGDATA="$WORK/neg-data"
    mkdir -p "$NEGDATA"
    NEGJDTLS="$WORK/negjdtls.json"
    export GOIDEA_SMOKE_DATA="$NEGDATA"
    smoke "$NEGJDTLS" GOIDEA_SMOKE_NEGJDTLS=1
    unset GOIDEA_SMOKE_DATA
    python3 "$CHECKS" negjdtls "$NEGJDTLS"
    # B: switch the project while a real Maven run is in flight.
    PROJB="$WORK/java-maven-b"
    cp -R "$ROOT/testdata/java-maven" "$PROJB"
    rm -rf "$PROJB/target"
    git -C "$PROJB" init -q
    git -C "$PROJB" add -A
    git -C "$PROJB" -c user.name=e2e -c user.email=e2e@goidea.invalid commit -qm baseline
    RUNSWITCH="$WORK/runswitch.json"
    smoke "$RUNSWITCH" GOIDEA_SMOKE_RUNSWITCH=1 GOIDEA_SMOKE_SWITCH_TO="$PROJB"
    python3 "$CHECKS" runswitch "$RUNSWITCH"
    # C: the server dies in the middle of the preparation.
    RUNKILL="$WORK/runkill.json"
    smoke "$RUNKILL" GOIDEA_SMOKE_RUNKILL=1 GOIDEA_SMOKE_RUNKILL_OFFLINE="$OFFLINE"
    python3 "$CHECKS" runkill "$RUNKILL"
}

if [ "$OFFLINE" = "1" ]; then
    say "阶段 1b3：离线启动状态机（后端、项目、构建模型、运行）"
    smoke "$READINESS_JSON" GOIDEA_SMOKE_READINESS=1 \
        GOIDEA_SMOKE_READINESS_JAVA_OPTIONAL=1
    python3 "$CHECKS" readiness "$READINESS_JSON"
    run_preparation_negatives
    skip "Java 工具安装、JDT 语言服务与调试（GOIDEA_E2E_OFFLINE=1）"
else
    say "阶段 2：安装 jdtls 与 java-debug"
    TOOLS="$WORK/tools.json"
    smoke "$TOOLS" GOIDEA_SMOKE_TOOLS=install
    python3 "$CHECKS" tools "$TOOLS"

    say "阶段 2a：安装工具链后的六态启动状态机（第八节）"
    smoke "$READINESS_JSON" GOIDEA_SMOKE_READINESS=1 \
        GOIDEA_SMOKE_READINESS_JAVA_OPTIONAL=0
    python3 "$CHECKS" readiness "$READINESS_JSON"

    run_preparation_negatives

    say "阶段 2b：Debug 所选配置并附加到真实 JDWP JVM"
    DBGCFG="$WORK/dbgcfg.json"
    smoke "$DBGCFG" GOIDEA_SMOKE_DBGCFG=1 GOIDEA_SMOKE_ATTACH=1
    python3 "$CHECKS" dbgcfg "$DBGCFG"
    python3 "$CHECKS" attach "$DBGCFG"

    say "阶段 2c：全新项目在导入期间启动 Debug"
    DEBUGIMPORT_PROJ="$WORK/java-maven-debug-import"
    cp -R "$ROOT/testdata/java-maven" "$DEBUGIMPORT_PROJ"
    rm -rf "$DEBUGIMPORT_PROJ/target"
    git -C "$DEBUGIMPORT_PROJ" init -q
    git -C "$DEBUGIMPORT_PROJ" add -A
    git -C "$DEBUGIMPORT_PROJ" -c user.name=e2e -c user.email=e2e@goidea.invalid commit -qm baseline
    DEBUGIMPORT_JSON="$WORK/debug-import.json"
    PROJ="$DEBUGIMPORT_PROJ"
    JAVA="$DEBUGIMPORT_PROJ/src/main/java/com/example/App.java"
    smoke "$DEBUGIMPORT_JSON" GOIDEA_SMOKE_DEBUG_IMPORT=1
    python3 "$CHECKS" debugimport "$DEBUGIMPORT_JSON"
    PROJ="$BASE_PROJ"
    JAVA="$BASE_JAVA"

    if [ -n "${GOIDEA_GOLDEN_DIR:-}" ]; then
        say "阶段 2d：OpenIDEA Golden 视觉差异"
        GOLD="$WORK/goldens.json"
        smoke "$GOLD" GOIDEA_SMOKE_GOLDENS=1 GOIDEA_SHOTS_DIR="$SHOTS" \
            GOIDEA_GOLDEN_DIR="$GOIDEA_GOLDEN_DIR" GOIDEA_GOLDEN_DIFF_DIR="$WORK/diffs"
        python3 "$CHECKS" goldens "$GOLD"
    else
        say "SKIP：OpenIDEA Golden 截图未提供（设置 GOIDEA_GOLDEN_DIR）"
    fi

    say "阶段 2e：干净环境开箱即用（第八节全链路）"
    FRESH="$WORK/fresh-data"
    mkdir -p "$FRESH"
    FRESH_JSON="$WORK/fresh.json"
    PROJ="$PROJ_BASE"
    JAVA="$JAVA_BASE"
    smoke "$FRESH_JSON" GOIDEA_SMOKE_DATA="$FRESH" GOIDEA_SMOKE_FRESH=1
    unset GOIDEA_SMOKE_DATA
    python3 "$CHECKS" fresh "$FRESH_JSON"

    say "阶段 3：JDT 补全、悬停、跳转、格式化、用法、重命名、诊断"
    LSP="$WORK/lsp.json"
    smoke "$LSP" GOIDEA_SMOKE_LSP=1 GOIDEA_SMOKE_HIERARCHY=1 GOIDEA_SMOKE_SE=1
    python3 "$CHECKS" lsp "$LSP"

    say "阶段 3a：Java 分析（检查、重复、结构搜索/替换、清理）"
    ANALYSIS="$WORK/analysis.json"
    smoke "$ANALYSIS" GOIDEA_SMOKE_ANALYSIS=1
    python3 "$CHECKS" analysis "$ANALYSIS"

    say "阶段 4：断点、变量、求值、步过、运行结束"
    DEBUG="$WORK/debug.json"
    JAVA17="${GOIDEA_E2E_JAVA17:-}"
    if [ -z "$JAVA17" ] && [ -x /usr/libexec/java_home ]; then
        JAVA17="$(/usr/libexec/java_home -v 17 2>/dev/null || true)"
    fi
    [ -x "$JAVA17/bin/java" ] || {
        echo "Debug heap capture needs a real JDK 17+; set GOIDEA_E2E_JAVA17." >&2
        exit 1
    }
    smoke "$DEBUG" GOIDEA_SMOKE_DEBUG=1 GOIDEA_SMOKE_DEBUG_JDK="$JAVA17"
    python3 "$CHECKS" debug "$DEBUG"

    say "阶段 4b：HotSwap（重新加载变更的类）"
    HOTSWAP="$WORK/hotswap.json"
    smoke "$HOTSWAP" GOIDEA_SMOKE_HOTSWAP=1
    python3 "$CHECKS" hotswap "$HOTSWAP"

    say "阶段 5：多模块 Maven 的 Spring Boot 运行"
    SPRING="$WORK/java-spring-multimodule"
    cp -R "$ROOT/testdata/java-spring-multimodule" "$SPRING"
    say "用真实 Maven 预构建 Spring 多模块夹具"
    (cd "$SPRING" && mvn -q package -DskipTests) || { echo "Spring 多模块夹具构建失败" >&2; exit 1; }
    SPRING_JSON="$WORK/spring.json"
    PROJ="$SPRING"
    JAVA=""
    smoke "$SPRING_JSON" GOIDEA_SMOKE_SPRING=1
    python3 "$CHECKS" spring "$SPRING_JSON"
fi

if [ "${GOIDEA_E2E_FIXTURES:-0}" = "1" ]; then
    say "附加：JUnit、多模块、Lombok、Spring Boot 与 Gradle 夹具"
    java11="$(/usr/libexec/java_home -v 11 2>/dev/null || true)"
    run_fixture() { # name, mvn args...
        local name="$1"
        shift
        local dir="$WORK/$name"
        cp -R "$ROOT/testdata/$name" "$dir"
        if (cd "$dir" && mvn -q "$@"); then
            printf '  ok   %s\n' "$name"
        else
            printf '  FAIL %s\n' "$name"
            exit 1
        fi
    }
    run_fixture java-junit -o test
    run_fixture java-multimodule -o package
    if [ -n "$java11" ]; then
        JAVA_HOME="$java11" run_fixture java-lombok -o package
    else
        skip "Lombok 夹具（没有 JDK 11）"
    fi
    if [ "$OFFLINE" = "1" ]; then
        skip "Spring Boot 夹具（GOIDEA_E2E_OFFLINE=1）"
    else
        run_fixture java-spring-boot package -DskipTests
    fi
    GRADLE=""
    if [ -x "$ROOT/testdata/java-gradle/gradlew" ]; then
        GRADLE=./gradlew
    elif command -v gradle >/dev/null; then
        GRADLE=gradle
    fi
    if [ -z "$GRADLE" ]; then
        skip "Gradle 夹具（没有 gradle 也没有 wrapper）"
    else
        dir="$WORK/java-gradle"
        cp -R "$ROOT/testdata/java-gradle" "$dir"
        (cd "$dir" && "$GRADLE" -q --no-daemon build)
        printf '  ok   java-gradle\n'
    fi
fi

say "端到端回归通过"
