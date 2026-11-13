#!/usr/bin/env bash
# Runs the end-to-end regression against real Java projects and the real
# openidea-server: editing, search, Git, settings, Maven build/run, the Java
# toolchain install, JDT language services and the debugger, all through the
# offscreen smoke hooks of a hidden window.
#
# Usage: scripts/e2e.sh
#   GOIDEA_E2E_OFFLINE=1   skip the phases that need a download (tools, LSP,
#                          debug) and say so; the build/search/Git/Run phases
#                          still run.
#   GOIDEA_E2E_FIXTURES=1  also build the JUnit, multi-module, Lombok and
#                          Spring Boot fixtures with the real mvn.
#   GOIDEA_E2E_DATA=<dir>  reuse a data directory (default build/e2e-data), so
#                          jdtls/java-debug are downloaded once.
#   GOIDEA_E2E_BIN=<path>  test an already built binary.
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

# smoke runs one offscreen phase in a hidden window and leaves its JSON at
# $1; the remaining arguments are GOIDEA_SMOKE_* variables for that phase.
smoke() {
    local out="$1"
    shift
    env GOIDEA_HEADLESS=1 GOIDEA_DATA_DIR="$DATA" GOIDEA_SERVER="$SERVER" \
        GOIDEA_OPEN_PROJECT="$PROJ" GOIDEA_OPEN_FILES="$JAVA" \
        GOIDEA_SMOKE_OUT="$out" "$@" "$BIN" >"${out%.json}.log" 2>&1 ||
        { echo "goidea 退出异常，日志 ${out%.json}.log：" >&2; tail -30 "${out%.json}.log" >&2; return 1; }
    [ -s "$out" ] || { echo "冒烟没有写出 $out" >&2; tail -30 "${out%.json}.log" >&2; return 1; }
}

command -v mvn >/dev/null || { echo "缺少 mvn：本测试用真实 Maven 构建 Java 项目" >&2; exit 1; }

say "阶段 1/4：编辑、搜索、Git、设置、外观、Maven 构建运行"
BASE="$WORK/base.json"
smoke "$BASE" \
    GOIDEA_SMOKE_EDIT=1 GOIDEA_SMOKE_CODEINTEL=1 GOIDEA_SMOKE_SEARCH=1 \
    GOIDEA_SMOKE_DIFF=1 GOIDEA_SMOKE_COMMANDS=1 GOIDEA_SMOKE_SETTINGS=1 \
    GOIDEA_SMOKE_WATCH=1 GOIDEA_SMOKE_RUN=1 GOIDEA_SMOKE_BUILD=1 GOIDEA_SMOKE_LOG=1 GOIDEA_SMOKE_COMMIT=1 GOIDEA_SMOKE_BRANCH=1
python3 "$CHECKS" base "$BASE"

if [ "$OFFLINE" = "1" ]; then
    skip "Java 工具安装、JDT 语言服务与调试（GOIDEA_E2E_OFFLINE=1）"
else
    say "阶段 2/4：安装 jdtls 与 java-debug"
    TOOLS="$WORK/tools.json"
    smoke "$TOOLS" GOIDEA_SMOKE_TOOLS=install
    python3 "$CHECKS" tools "$TOOLS"

    say "阶段 3/4：JDT 补全、悬停、跳转、格式化、用法、重命名、诊断"
    LSP="$WORK/lsp.json"
    smoke "$LSP" GOIDEA_SMOKE_LSP=1
    python3 "$CHECKS" lsp "$LSP"

    say "阶段 4/4：断点、变量、求值、步过、运行结束"
    DEBUG="$WORK/debug.json"
    smoke "$DEBUG" GOIDEA_SMOKE_DEBUG=1
    python3 "$CHECKS" debug "$DEBUG"
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
