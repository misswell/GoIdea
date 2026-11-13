#!/usr/bin/env python3
"""Writes OPENIDEA_IPC.md: every IPC command OpenIDEA's Tauri shell registers,
which of them openidea-server serves over --stdio, and which GoIDEA already
calls.

The inventory is generated, never edited by hand: rerun it after a submodule
bump. `--check` fails when the committed document is stale, and
`--server PATH` also compares the source with a live `commands` reply so a
shipped server that predates the submodule shows up.

Usage:
    scripts/openidea-ipc.py                # rewrite OPENIDEA_IPC.md
    scripts/openidea-ipc.py --check        # verify it is current
    scripts/openidea-ipc.py --server resources/openidea-server
"""

import argparse
import glob
import json
import os
import re
import select
import subprocess
import sys
import tempfile
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OPENIDEA = os.path.join(ROOT, "third_party", "openidea")
DOC = os.path.join(ROOT, "OPENIDEA_IPC.md")

# Tauri command modules, in the order of commands/mod.rs.
TAURI_MODULES = os.path.join(OPENIDEA, "src-tauri", "src", "commands")
HOST_SRC = os.path.join(OPENIDEA, "crates", "ide-host", "src")

DOMAIN = {
    "agent": "AI agent sessions",
    "analysis": "Code analysis",
    "android": "Android devices",
    "app": "Application and projects",
    "build": "Maven/Gradle builds",
    "bytes": "Binary file reads",
    "caches": "Caches and restart",
    "clipboard": "Clipboard",
    "coverage": "Coverage",
    "db": "Database",
    "debug": "Debugger",
    "dependencies": "Dependencies",
    "deploy": "Deployment",
    "doc": "Documents",
    "docker": "Docker",
    "env": "Environment",
    "file_dialog": "File dialogs",
    "fonts": "Fonts",
    "fs": "File system",
    "github": "GitHub",
    "history": "Local history",
    "http": "HTTP client",
    "java": "Java",
    "lsp": "Language services",
    "mcp": "MCP",
    "nav": "Navigation and index",
    "notebook": "Notebooks",
    "perf": "Performance",
    "plugin": "Plugins",
    "profiler": "Profiler",
    "refactor": "Refactoring",
    "remote_dev": "Remote development",
    "run": "Run configurations",
    "sdk": "SDKs",
    "search": "Search",
    "spell": "Spelling",
    "ssh": "SSH",
    "syntax": "Syntax trees",
    "term": "Terminal",
    "tools": "Toolchains",
    "vcs": "Version control",
    "web": "Web preview",
    "window": "Window and menu",
    "window_drag": "Window drag",
    "window_group": "Window groups",
    "workspace": "Workspaces",
    "wsl": "WSL",
}


def read(path):
    with open(path, encoding="utf-8") as f:
        return f.read()


def tauri_commands():
    """(module, name) of every command collect_commands! registers."""
    text = read(os.path.join(TAURI_MODULES, "mod.rs"))
    m = re.search(r"collect_commands!\[(.*?)\]\s*\)", text, re.S)
    if not m:
        sys.exit("commands/mod.rs has no collect_commands! block")
    pairs = re.findall(r"\b(\w+)::(\w+),", m.group(1))
    return pairs


def tauri_bodies():
    """name -> source of its #[tauri::command] fn, for the local-only classes."""
    bodies = {}
    for path in sorted(glob.glob(os.path.join(TAURI_MODULES, "*.rs"))):
        text = read(path)
        for part in re.split(r"(?=#\[tauri::command\])", text):
            m = re.search(r"\bfn\s+(\w+)\s*[(<]", part)
            if not m:
                continue
            body = part.replace("#[tauri::command]", "").replace(
                "#[allow(clippy::too_many_arguments)]", ""
            )
            bodies[m.group(1)] = body.split("\n#[tauri::command]")[0]
    return bodies


def remote_commands():
    """name -> argument names, from what the stdio dispatcher registers.

    The server adds most commands as `d.add("name", ...)` and a few under a
    constant (plugin_sync_*); the constants are file-scoped, so they are
    resolved across every host source.
    """
    files = {}
    consts = {}
    for path in glob.glob(os.path.join(HOST_SRC, "**", "*.rs"), recursive=True):
        files[path] = read(path)
        consts.update(re.findall(r'const\s+([A-Z_][A-Z0-9_]*)\s*:\s*&str\s*=\s*"([^"]+)"', files[path]))
    regs = {}
    for path, text in files.items():
        marks = list(re.finditer(r'\.add\(\s*(?:"([a-zA-Z0-9_]+)"|([A-Z_][A-Z0-9_]*))', text))
        for i, mark in enumerate(marks):
            name = mark.group(1) or consts.get(mark.group(2))
            if not name:
                continue
            end = marks[i + 1].start() if i + 1 < len(marks) else len(text)
            block = text[mark.end():end]
            args = sorted(set(re.findall(r'c\.(?:arg|sender|channel)\(\s*"([^"]+)"', block)))
            regs[name] = args
    return regs


def release_remote(release_commit):
    """The commands the pinned release's server registered, from git history."""
    if not release_commit:
        return None
    files = subprocess.run(
        ["git", "-C", OPENIDEA, "ls-tree", "-r", "--name-only", release_commit, "crates/ide-host/src"],
        capture_output=True, text=True,
    ).stdout.split()
    consts, sources = {}, {}
    for path in files:
        if not path.endswith(".rs"):
            continue
        shown = subprocess.run(
            ["git", "-C", OPENIDEA, "show", "{}:{}".format(release_commit, path)],
            capture_output=True, text=True,
        )
        if shown.returncode == 0:
            sources[path] = shown.stdout
            consts.update(re.findall(r'const\s+([A-Z_][A-Z0-9_]*)\s*:\s*&str\s*=\s*"([^"]+)"', shown.stdout))
    names = set()
    for text in sources.values():
        for mark in re.finditer(r'\.add\(\s*(?:"([a-zA-Z0-9_]+)"|([A-Z_][A-Z0-9_]*))', text):
            name = mark.group(1) or consts.get(mark.group(2))
            if name:
                names.add(name)
    return names or None


def upstream_pin():
    """(submodule commit, version, release, release commit) from UPSTREAMS.yml."""
    text = read(os.path.join(ROOT, "UPSTREAMS.yml"))
    block = text.split("openidea:", 1)[1]
    def one(key):
        m = re.search(r"^\s*{}:\s*(\S+)\s*$".format(key), block, re.M)
        return m.group(1) if m else ""
    return one("commit"), one("version"), one("release"), one("release_commit")


def goidea_used(remote):
    """Remote command names GoIDEA's non-test Go sources name.

    Both an `Invoke` call and a wrapper that passes the name (`Docs` open/save,
    the smoke LSP helper) count; a literal anywhere in product code is the
    union that catches both without following the wrappers.
    """
    used = set()
    for path in glob.glob(os.path.join(ROOT, "internal", "**", "*.go"), recursive=True) + glob.glob(
        os.path.join(ROOT, "cmd", "**", "*.go"), recursive=True
    ):
        if path.endswith("_test.go"):
            continue
        text = read(path)
        used.update(set(re.findall(r'"([a-z][a-z0-9_]+)"', text)) & set(remote))
        for m in re.finditer(r"\.Invoke\(|smokeLSPCall\(", text):
            lit = re.search(r'"([a-z][a-z0-9_]+)"', text[m.end():m.end() + 300])
            if lit and lit.group(1) in remote:
                used.add(lit.group(1))
    return used


def classify_local(body):
    if body is None:
        return "desktop"
    stripped = body.replace("#[specta::specta]", "")
    if any(k in stripped for k in ("WebviewWindow", "AppHandle", "tauri::", "Window", "Clipboard", "rfd")):
        return "desktop"
    if "ops::" in stripped:
        return "shared-op"
    if re.search(r"\b(host|state\.host|state\()", stripped):
        return "host"
    return "crate"


def live_commands(server):
    """`commands` over --stdio, the way a desktop client asks."""
    home = tempfile.mkdtemp(prefix="goidea-ipc-")
    env = dict(os.environ, OPENIDEA_SERVER_HOME=home)
    p = subprocess.Popen(
        [server, "--stdio"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL, env=env,
    )

    def send(obj):
        body = json.dumps(obj).encode()
        p.stdin.write(b"Content-Length: %d\r\n\r\n" % len(body) + body)
        p.stdin.flush()

    def recv(timeout=60):
        buf, end = b"", time.time() + timeout
        while b"\r\n\r\n" not in buf:
            r, _, _ = select.select([p.stdout], [], [], max(0, end - time.time()))
            if not r:
                raise TimeoutError("no reply header")
            buf += os.read(p.stdout.fileno(), 1 << 16)
        head, _, rest = buf.partition(b"\r\n\r\n")
        size = int(re.search(rb"Content-Length:\s*(\d+)", head, re.I).group(1))
        while len(rest) < size:
            r, _, _ = select.select([p.stdout], [], [], max(0, end - time.time()))
            if not r:
                raise TimeoutError("short reply body")
            rest += os.read(p.stdout.fileno(), 1 << 16)
        return json.loads(rest[:size])

    try:
        send({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"version": 2}})
        recv()
        send({"jsonrpc": "2.0", "id": 2, "method": "commands"})
        return set(recv()["result"])
    finally:
        p.stdin.close()
        p.kill()
        p.wait()


def render(server=None):
    pairs = tauri_commands()
    bodies = tauri_bodies()
    remote = remote_commands()
    used = goidea_used(remote)
    names = [name for _, name in pairs]
    modules = {name: mod for mod, name in pairs}
    local = {name: classify_local(bodies.get(name)) for name in names if name not in remote}

    commit = subprocess.run(
        ["git", "-C", OPENIDEA, "rev-parse", "--short", "HEAD"], capture_output=True, text=True
    ).stdout.strip()
    pin_commit, version, release, release_commit = upstream_pin()
    released = release_remote(release_commit)

    out = []
    w = out.append
    w("# OpenIDEA IPC inventory\n")
    w(
        "Generated by [scripts/openidea-ipc.py](scripts/openidea-ipc.py) from the\n"
        "`third_party/openidea` submodule ({}), not edited by hand. Rerun it after\n"
        "every submodule bump; `scripts/openidea-ipc.py --check` fails when this file\n"
        "is stale.\n".format(commit or "unknown")
    )
    w(
        "OpenIDEA is the product specification, so this is the list of what a GoIDEA\n"
        "frontend must eventually reach. The desktop shell registers **{} IPC\n"
        "commands**; `openidea-server --stdio` answers **{}** of them. The rest are\n"
        "either desktop-shell work (windows, menus, dialogs, clipboard) that a native\n"
        "Go frontend implements itself, or host logic that is not yet exposed over\n"
        "stdio and needs an OpenIDEA change before GoIDEA can call it.\n".format(
            len(names), len(remote)
        )
    )
    w("## Protocol\n")
    w(
        "JSON-RPC 2.0 over `Content-Length` framing. The server answers `initialize`\n"
        "(protocol 2), `commands` (this list) and `invoke {window, cmd, args}`.\n"
        "Streaming arguments are `{\"$channel\": id}`; the server sends `channel`\n"
        "notifications, `event` notifications for a window's `IdeEvent`s, and\n"
        "`plugin` / `plugin_secret` for remote code plugins. GoIDEA's client side is\n"
        "described in [RPC_PROTOCOL.md](RPC_PROTOCOL.md).\n"
    )
    if released is not None:
        w("## The shipped server\n")
        w(
            "The pin in `UPSTREAMS.yml` ships `{}` (commit `{}`); the submodule is at\n"
            "`{}`, which may be ahead of it. Only a command in both the source and the\n"
            "release can be called today.\n".format(release or "the release", release_commit[:8], pin_commit[:8])
        )
        w(
            "- Source-only: {}\n- Release-only: {}\n".format(
                ", ".join("`{}`".format(c) for c in sorted(set(remote) - released)) or "none",
                ", ".join("`{}`".format(c) for c in sorted(released - set(remote))) or "none",
            )
        )
        if set(remote) - released:
            w(
                "A source-only command needs the next `openidea-server` release before\n"
                "GoIDEA can rely on it; code that calls it must check `commands` first.\n"
            )

    domains = {}
    for name in names:
        d = domains.setdefault(modules[name], {"remote": 0, "local": 0, "used": 0})
        if name in remote:
            d["remote"] += 1
        else:
            d["local"] += 1
        if name in used:
            d["used"] += 1
    w("## By domain\n")
    w("| Domain | Module | Remote | Local | GoIDEA uses |")
    w("| --- | --- | ---: | ---: | ---: |")
    for mod in sorted(domains):
        d = domains[mod]
        w("| {} | `{}` | {} | {} | {} |".format(DOMAIN.get(mod, mod), mod, d["remote"], d["local"], d["used"]))
    w("")

    w("## Served by `openidea-server --stdio`\n")
    w(
        "{} commands. `GoIDEA` marks the ones a non-test Go source invokes today\n"
        "({} of {}).\n".format(len(remote), len(used), len(remote))
    )
    w("| Command | Domain | Arguments | GoIDEA |")
    w("| --- | --- | --- | --- |")
    for name in sorted(remote):
        args = ", ".join("`{}`".format(a) for a in remote[name]) or "—"
        w("| `{}` | {} | {} | {} |".format(
            name, DOMAIN.get(modules.get(name, "?"), modules.get(name, "?")), args,
            "yes" if name in used else "—",
        ))
    w("")
    missing_used = sorted(set(used) - set(remote))
    if missing_used:
        w("GoIDEA invokes commands the dispatcher does not register: {}.\n".format(
            ", ".join("`{}`".format(c) for c in missing_used)))

    w("## Desktop-only IPC\n")
    w(
        "{} commands the desktop shell answers but stdio does not. `Kind` says what\n"
        "GoIDEA needs to do for each:\n\n"
        "- **desktop** — a native Go frontend implements it (window, menu, dialog,\n"
        "  clipboard, notifications, updater); no backend change.\n"
        "- **shared-op** — an `ide_host::ops` implementation already exists and only\n"
        "  needs registering in the stdio dispatcher.\n"
        "- **host** — the logic works against the `Host` (or `AppState`) but is not in\n"
        "  an `ops` module: it needs a small OpenIDEA change.\n"
        "- **crate** — implemented in the Tauri shell against an `ide-*` crate; it\n"
        "  needs an `ops` module before it can be served remotely.\n".format(len(local))
    )
    w("| Command | Domain | Kind |")
    w("| --- | --- | --- |")
    for name in sorted(local):
        mod = modules[name]
        w("| `{}` | {} | {} |".format(name, DOMAIN.get(mod, mod), local[name]))
    w("")

    counts = {}
    for kind in local.values():
        counts[kind] = counts.get(kind, 0) + 1
    w("## What this means for GoIDEA\n")
    w(
        "{} remote commands are available today; GoIDEA uses {}. The parity work is\n"
        "first to cover the remote surface OpenIDEA's own frontend uses, then to move\n"
        "`shared-op`/`host`/`crate` commands onto stdio through the OpenIDEA fork\n"
        "(a submodule bump and a server release), and to implement `desktop` commands\n"
        "natively in Go. A feature is not ported because a similar Mayoi one exists:\n"
        "it is ported when the OpenIDEA interaction behind it works in GoIDEA.\n".format(
            len(remote), len(used)
        )
    )
    w(
        "Local-only kinds: {}.\n".format(
            ", ".join("{} {}".format(counts[k], k) for k in sorted(counts))
        )
    )
    w(
        "The gaps that need OpenIDEA work are the {} `shared-op`, `host` and `crate`\n"
        "commands above, in that order: the first already have an `ops`\n"
        "implementation, the second work against the host, and the third must move\n"
        "out of the Tauri shell first.\n".format(
            counts.get("shared-op", 0) + counts.get("host", 0) + counts.get("crate", 0)
        )
    )
    return "\n".join(out) + "\n"


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--check", action="store_true", help="fail if OPENIDEA_IPC.md is stale")
    ap.add_argument("--server", help="also verify a live server's `commands` reply")
    args = ap.parse_args()
    if args.server:
        if not os.access(args.server, os.X_OK):
            sys.exit("{} is not executable".format(args.server))
        live = live_commands(os.path.abspath(args.server))
        source = set(remote_commands())
        if live != source:
            print(
                "server differs from source: only in source {}; only in server {}".format(
                    sorted(source - live) or "none", sorted(live - source) or "none"
                ),
                file=sys.stderr,
            )
        else:
            print("server matches the source: {} commands".format(len(live)))
    text = render()
    try:
        current = read(DOC)
    except FileNotFoundError:
        current = None
    if args.check:
        if current != text:
            sys.exit("OPENIDEA_IPC.md is stale: run scripts/openidea-ipc.py")
        print("OPENIDEA_IPC.md is current")
        return
    with open(DOC, "w", encoding="utf-8") as f:
        f.write(text)
    print("wrote {}".format(os.path.relpath(DOC, ROOT)))


if __name__ == "__main__":
    main()
