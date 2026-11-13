#!/usr/bin/env bash
# Measures the budgets in PERFORMANCE.md on this machine, offscreen, against an
# already built binary: cold start to first frame, backend ready, idle CPU, RSS,
# a 1M-line file, a 10k-file tree, 100 tabs and 20 concurrent searches. The app
# and the server it spawns are sampled by PID; the app is always started with
# GOIDEA_HEADLESS=1 in a fixed data directory.
#
# Usage: scripts/bench.sh
#   GOIDEA_BENCH_BIN=<path>   use an already built binary (default build/goidea)
#   GOIDEA_BENCH_DATA=<dir>   data directory (default build/bench-data)
#   GOIDEA_BENCH_ENFORCE=1    exit 1 when a measured value is over budget
#   GOIDEA_BENCH_KEEP=1       keep the generated projects for inspection
# Everything lands in build/bench/<timestamp>/: per-scenario JSON and logs,
# metrics.tsv, and a summary.md with the budget verdicts.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="${GOIDEA_BENCH_BIN:-$ROOT/build/goidea}"
SERVER="${GOIDEA_SERVER:-$ROOT/resources/openidea-server}"
DATA="${GOIDEA_BENCH_DATA:-$ROOT/build/bench-data}"
ENFORCE="${GOIDEA_BENCH_ENFORCE:-0}"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT="$ROOT/build/bench/$STAMP"

say() { printf '\n==> %s\n' "$*"; }

[ -x "$BIN" ] || { echo "缺少 $BIN，先运行 CGO_ENABLED=0 go build -o build/goidea ./cmd/goidea" >&2; exit 1; }
[ -x "$SERVER" ] || { echo "缺少 openidea-server：$SERVER（先运行 scripts/fetch-server.sh）" >&2; exit 1; }
command -v python3 >/dev/null || { echo "缺少 python3" >&2; exit 1; }
mkdir -p "$DATA" "$OUT"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/goidea-bench.XXXXXX")"
TRACKED=()
cleanup() {
    status=$?
    for pid in "${TRACKED[@]:-}"; do
        [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && kill -TERM "$pid" 2>/dev/null || true
    done
    if [ "$status" -ne 0 ] || [ "${GOIDEA_BENCH_KEEP:-0}" = "1" ]; then
        echo "kept benchmark projects in $WORK" >&2
    else
        rm -rf "$WORK"
    fi
    exit $status
}
trap cleanup EXIT INT TERM

# cpu_seconds sums the CPU time a process and its descendants have used; two
# samples around a sleep give the idle percentage. cputime is [[dd-]hh:]mm:ss.
cpu_seconds() {
    local pid total=0 pid_list
    pid_list="$pid $(descendants "$pid")"
    for pid in $pid_list; do
        local t
        t="$(ps -o cputime= -p "$pid" 2>/dev/null | awk '{
            t=$1; d=0
            if (index(t,"-")>0) { split(t,a,"-"); d=a[1]; t=a[2] }
            n=split(t,b,":")
            if (n==3) s=b[1]*3600+b[2]*60+b[3]
            else if (n==2) s=b[1]*60+b[2]
            else s=b[1]
            printf "%.2f", d*86400+s
        }')"
        if [ -n "$t" ]; then
            total="$(awk -v a="$total" -v b="$t" 'BEGIN{printf "%.2f", a+b}')"
        fi
    done
    printf '%s\n' "$total"
}

descendants() {
    local child
    for child in $(pgrep -P "$1" 2>/dev/null || true); do
        printf '%s\n' "$child"
        descendants "$child"
    done
}

rss_kb() {
    local pid total=0 pid_list
    pid_list="$pid $(descendants "$pid")"
    for pid in $pid_list; do
        local kb
        kb="$(ps -o rss= -p "$pid" 2>/dev/null | tr -d ' ')"
        if [ -n "$kb" ]; then total=$((total + kb)); fi
    done
    printf '%s\n' "$total"
}

server_pid() {
    pgrep -P "$1" 2>/dev/null | head -1 || true
}

# run_scenario <name> <project> <files> <idle-seconds> <settle> <window> [env…]
# Starts the hidden app, samples CPU/RSS for app+server while it is idle, waits
# for it to quit and records whether the server outlived it.
run_scenario() {
    local name="$1" proj="$2" files="$3" idle="$4" settle="$5" window="$6"
    shift 6
    local out="$OUT/$name.json" log="$OUT/$name.log"
    local app srv c1 c2 app_rss srv_rss cpu orphan=0
    say "场景 $name（idle ${idle}s）"
    env GOIDEA_HEADLESS=1 GOIDEA_BENCH_IDLE="$idle" \
        GOIDEA_DATA_DIR="$DATA" GOIDEA_SERVER="$SERVER" \
        GOIDEA_OPEN_PROJECT="$proj" GOIDEA_OPEN_FILES="$files" \
        GOIDEA_SMOKE_OUT="$out" "$@" "$BIN" >"$log" 2>&1 &
    app=$!
    # A benchmark that hangs must fail the run, not block it forever.
    ( sleep $((idle + 60)); kill -TERM "$app" 2>/dev/null ) &
    watchdog=$!
    sleep "$settle"
    srv="$(server_pid "$app")"
    if [ -n "$srv" ]; then TRACKED+=("$srv"); fi
    c1="$(cpu_seconds "$app")"
    app_rss="$(ps -o rss= -p "$app" 2>/dev/null | tr -d ' ')"
    sleep "$window"
    c2="$(cpu_seconds "$app")"
    app_rss="$(awk -v a="$app_rss" -v b="$(ps -o rss= -p "$app" 2>/dev/null | tr -d ' ')" 'BEGIN{print (a>b?a:b)}')"
    cpu="$(awk -v a="$c1" -v b="$c2" -v w="$window" 'BEGIN{d=b-a; if (d<0) d=0; printf "%.2f", d*100/w}')"
    if [ -n "$srv" ]; then
        srv_rss="$(rss_kb "$srv")"
    else
        srv_rss=0
    fi
    wait "$app" || { echo "$name 退出异常，见 $log" >&2; tail -20 "$log" >&2; exit 1; }
    kill "$watchdog" 2>/dev/null || true
    if [ -n "$srv" ]; then
        for _ in 1 2 3 4 5 6; do
            kill -0 "$srv" 2>/dev/null || break
            sleep 0.5
        done
        if kill -0 "$srv" 2>/dev/null; then orphan=1; fi
    fi
    [ -s "$out" ] || { echo "$name 没有写出 $out，见 $log" >&2; tail -20 "$log" >&2; exit 1; }
    printf '%s\t%s\t%s\t%s\t%s\n' "$name" "$app_rss" "$srv_rss" "$cpu" "$orphan" >>"$OUT/metrics.tsv"
    printf '    firstFrame=%sms backendReady=%sms tabs=%s idleCpu=%s%% rss=%dMB orphan=%s\n' \
        "$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("firstFrameMs"))' "$out")" \
        "$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("backendReadyMs"))' "$out")" \
        "$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("tabs"))' "$out")" \
        "$cpu" "$(((app_rss + srv_rss) / 1024))" "$orphan"
}

# 1. Cold start and idle, with a 10 000-line file open next to the Java class.
PROJ="$WORK/start"
cp -R "$ROOT/testdata/java-maven" "$PROJ"
rm -rf "$PROJ/target"
python3 - "$PROJ/Lines10k.txt" <<'PY'
import sys
with open(sys.argv[1], "w", encoding="utf-8") as f:
    f.write("".join(f"line {i}\n" for i in range(1, 10001)))
PY
JAVA="$PROJ/src/main/java/com/example/App.java"
FILES="$JAVA:$PROJ/Lines10k.txt"
# A first launch warms the page cache and the backend data directory; the
# measured launch is the one with warm caches.
say "预热（不计入结果）"
env GOIDEA_HEADLESS=1 GOIDEA_BENCH_IDLE=6 GOIDEA_DATA_DIR="$DATA" GOIDEA_SERVER="$SERVER" \
    GOIDEA_OPEN_PROJECT="$PROJ" GOIDEA_OPEN_FILES="$FILES" "$BIN" >"$OUT/warmup.log" 2>&1
run_scenario start "$PROJ" "$FILES" 16 6 5

# 2. One million lines.
PROJ1M="$WORK/largefile"
cp -R "$ROOT/testdata/java-maven" "$PROJ1M"
rm -rf "$PROJ1M/target"
python3 - "$PROJ1M/big.txt" <<'PY'
import sys
with open(sys.argv[1], "w", encoding="utf-8") as f:
    for i in range(1, 1000001):
        f.write(f"line {i}\n")
PY
run_scenario largefile "$PROJ1M" "$PROJ1M/big.txt" 10 4 3

# 3. A 10 000-file source tree.
PROJT="$WORK/tree10k"
cp -R "$ROOT/testdata/java-maven" "$PROJT"
rm -rf "$PROJT/target"
python3 - "$PROJT" <<'PY'
import os, sys
root = os.path.join(sys.argv[1], "src", "gen")
for d in range(100):
    path = os.path.join(root, f"pkg{d:03d}")
    os.makedirs(path, exist_ok=True)
    for n in range(100):
        with open(os.path.join(path, f"Gen{d:03d}_{n:03d}.java"), "w", encoding="utf-8") as f:
            f.write(f"package gen.pkg{d:03d};\npublic class Gen{d:03d}_{n:03d} {{}}\n")
PY
run_scenario tree10k "$PROJT" "$PROJT/src/main/java/com/example/App.java" 10 4 3

# 4. One hundred open tabs.
PROJB="$WORK/tabs100"
cp -R "$ROOT/testdata/java-maven" "$PROJB"
rm -rf "$PROJB/target"
TABS=""
for i in $(seq 1 100); do
    f="$PROJB/src/main/java/com/example/Tab$i.java"
    printf 'package com.example;\npublic class Tab%s {}\n' "$i" >"$f"
    TABS="${TABS:+$TABS:}$f"
done
run_scenario tabs100 "$PROJB" "$TABS" 12 5 3

# 5. Twenty concurrent backend searches on the cold-start project.
run_scenario tasks "$PROJ" "$JAVA" 4 1 1 GOIDEA_BENCH_TASKS=20

say "汇总"
python3 - "$OUT" "$STAMP" "$BIN" "$SERVER" "$ENFORCE" <<'PY'
import json, os, platform, subprocess, sys

out, stamp, binary, server, enforce = sys.argv[1:6]
metrics = {}
path = os.path.join(out, "metrics.tsv")
if os.path.exists(path):
    for line in open(path, encoding="utf-8"):
        name, app_rss, srv_rss, cpu, orphan = line.rstrip("\n").split("\t")
        metrics[name] = dict(appRssKb=int(app_rss), serverRssKb=int(srv_rss), idleCpuPct=float(cpu), orphan=orphan == "1")

def load(name):
    p = os.path.join(out, f"{name}.json")
    return json.load(open(p, encoding="utf-8")) if os.path.exists(p) else {}

def mb(kb):
    return kb / 1024

rows = []
failed = []
def row(label, budget, value, ok):
    rows.append((label, budget, value, "PASS" if ok else "FAIL"))
    if not ok:
        failed.append(label)

start = load("start")
large = load("largefile")
tree = load("tree10k")
tabs = load("tabs100")
tasks = load("tasks")
m = metrics.get("start", {})

row("冷启动到首帧（缓存已热）", "≤ 1500 ms", f"{start.get('firstFrameMs')} ms", start.get("firstFrameMs", 1 << 30) <= 1500)
row("启动到后端就绪", "≤ 2000 ms", f"{start.get('backendReadyMs')} ms", 0 < start.get("backendReadyMs", 0) <= 2000)
row("空闲 CPU（窗口打开，无输入）", "≤ 1.00 %", f"{m.get('idleCpuPct')} %", m.get("idleCpuPct", 101) <= 1.0)
rss = mb(m.get("appRssKb", 1 << 30) + m.get("serverRssKb", 1 << 30))
row("常驻内存（含一个 10000 行文件，app+server）", "≤ 400 MB", f"{rss:.0f} MB", rss <= 400)
row("打开 1 000 000 行文件到首帧", "≤ 2000 ms", f"{large.get('firstFrameMs')} ms", large.get("firstFrameMs", 1 << 30) <= 2000)
row("1M 行文件已在标签页打开", "1 tab", f"{large.get('tabs')} tab", large.get("tabs") == 1)
row("10 000 文件项目，后端就绪", "≤ 10 000 ms", f"{tree.get('backendReadyMs')} ms", 0 < tree.get("backendReadyMs", 0) <= 10000)
row("100 个标签页全部打开", "100 tabs", f"{tabs.get('tabs')} tabs", tabs.get("tabs") == 100)
row("100 个标签页后无孤儿 server", "none", "none" if not metrics.get("tabs100", {}).get("orphan") else "orphan", not metrics.get("tabs100", {}).get("orphan", True))
t = tasks.get("tasks") or {}
row("20 个并发任务全部完成", "20/20", f"{t.get('done')}/{t.get('tasks')}", t.get("tasks") == 20 and t.get("done") == 20 and t.get("failed") == 0 and t.get("peak") == 20)
row("每个场景退出后无孤儿 server", "none", "none" if not any(v.get("orphan") for v in metrics.values()) else "orphan", not any(v.get("orphan") for v in metrics.values()))

def sha256(path):
    import hashlib
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()

try:
    goversion = subprocess.run(["go", "version"], capture_output=True, text=True).stdout.strip()
except OSError:
    goversion = "?"
env = [
    ("机器", f"{platform.machine()} / {platform.system()} {platform.release()}"),
    ("Go", goversion),
    ("二进制", os.path.relpath(binary, os.getcwd())),
    ("server sha256", sha256(server) if os.path.exists(server) else "?"),
]
lines = [f"# GoIDEA 性能测量 {stamp}", ""]
lines += [f"- {k}: `{v}`" for k, v in env]
lines += ["", "| 指标 | 预算 | 实测 | 结果 |", "| --- | --- | --- | --- |"]
lines += [f"| {a} | {b} | {c} | {d} |" for a, b, c, d in rows]
lines += ["", "未测量：键入到缓冲区延迟、同步往返、标签切换耗时（需要可见窗口与真实输入，屏外钩子不注入事件）。"]
text = "\n".join(lines) + "\n"
open(os.path.join(out, "summary.md"), "w", encoding="utf-8").write(text)
print(text)
if failed and enforce == "1":
    print("超出预算：" + ", ".join(failed), file=sys.stderr)
    sys.exit(1)
PY

say "结果在 $OUT"
