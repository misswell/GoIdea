#!/usr/bin/env python3
"""Assertions over the JSON a GoIDEA offscreen smoke run wrote.

Usage: e2e-checks.py <phase> <smoke.json> [<smoke.json> ...]
       e2e-checks.py --list
Phases: base | tools | lsp | debug
Every check has an id. REGRESSION_INDEX.json names the same ids, and --list
lets scripts/check-regression-index.py prove the two never drift apart, so a
check cannot be deleted without the index noticing. Every failed assertion is
printed and the exit status is 1, so a gate never passes on a partial build.
"""

import json
import sys

def get(data, dotted, default=None):
    cur = data
    for part in dotted.split("."):
        if not isinstance(cur, dict) or part not in cur:
            return default
        cur = cur[part]
    return cur


def absent(key, where=""):
    return f"{where}{key} is absent", lambda d: (key not in d, d.get(key))


CHECKS = {
    "base": [
        ("backend", "backend is up", lambda d: (d.get("backend") is True, d.get("backend"))),
        ("tabs", "a file is open in a tab", lambda d: ((d.get("tabs") or 0) >= 1, d.get("tabs"))),
        ("edited", "edit reached the backend", lambda d: (d.get("edited") is True, d.get("edited"))),
        ("undone", "undo round-tripped", lambda d: (d.get("undone") is True, d.get("undone"))),
        ("outline", "outline names the Java class", lambda d: (bool(get(d, "codeIntel.outlineFirst")), get(d, "codeIntel.outlineFirst"))),
        ("search.done", "project search finished", lambda d: (get(d, "search.done") is True, get(d, "search.done"))),
        ("search.files", "project search scanned the sources", lambda d: ((get(d, "search.files") or 0) >= 2, get(d, "search.files"))),
        ("search.matches", "project search found matches", lambda d: ((get(d, "search.matches") or 0) >= 1, get(d, "search.matches"))),
        ("searchError", *absent("searchError")),
        ("diff.rows", "Git diff has rows", lambda d: ((get(d, "diff.rows") or 0) > 0, get(d, "diff.rows"))),
        ("diffError", *absent("diffError")),
        ("commands", "command palette is populated", lambda d: ((get(d, "commands.all") or 0) >= 30, get(d, "commands.all"))),
        ("settings.opened", "settings sheet opened", lambda d: (get(d, "settings.opened") is True, get(d, "settings.opened"))),
        ("settings.closed", "settings sheet closed", lambda d: (get(d, "settings.closed") is True, get(d, "settings.closed"))),
        ("settings.theme", "theme choice reached the disk", lambda d: (get(d, "settings.storedTheme") == "dark", get(d, "settings.storedTheme"))),
        ("watch.reloaded", "external edit was reloaded", lambda d: (get(d, "watch.reloaded") is True, get(d, "watch.reloaded"))),
        ("vcs.changes", "Git status reports the change", lambda d: ((d.get("vcsChanges") or 0) >= 1, d.get("vcsChanges"))),
        ("log.commits", "vcs_log returned commits", lambda d: ((get(d, "log.count") or 0) >= 1, get(d, "log.count"))),
        ("log.details", "vcs_log_details returned the message and a branch", lambda d: (bool(get(d, "log.message")) and (get(d, "log.branches") or 0) >= 1, {"message": get(d, "log.message"), "branches": get(d, "log.branches")})),
        ("log.changes", "vcs_commits_changes returned the commit's files", lambda d: ((get(d, "log.changes") or 0) >= 1, {"changes": get(d, "log.changes"), "files": get(d, "log.changedFiles")})),
        ("logError", *absent("logError")),
        ("run.config", "a run configuration was detected", lambda d: (bool(d.get("runConfig")), d.get("runConfig"))),
        ("runError", *absent("runError")),
        ("run.exit", "run exited 0", lambda d: (d.get("exitCode") == 0, d.get("exitCode"))),
        ("run.build", "Maven reported BUILD SUCCESS", lambda d: ("BUILD SUCCESS" in (d.get("runOutput") or ""), d.get("runOutput"))),
        ("build.success", "build_run succeeded", lambda d: (get(d, "build.success") is True, get(d, "build.success"))),
        ("build.lines", "build streamed console lines", lambda d: ((get(d, "build.lines") or 0) >= 2, get(d, "build.lines"))),
        ("build.console", "build console shows the engine's output", lambda d: (any(s in (get(d, "build.console") or "") for s in ("BUILD SUCCESS", "Up to date")), get(d, "build.console"))),
        ("buildError", *absent("buildError")),
        ("commit.id", "vcs_commit returned a commit id", lambda d: (len(get(d, "commit.id") or "") >= 7, get(d, "commit.shortId"))),
        ("commit.staged", "the included unstaged file was staged", lambda d: ((get(d, "commit.staged") or 0) >= 1, get(d, "commit.staged"))),
        ("commit.clean", "the committed change left the status", lambda d: ((get(d, "commit.after") or 0) < (get(d, "commit.before") or 0) and "Greeter.java" not in (get(d, "commit.remaining") or []), {"before": get(d, "commit.before"), "after": get(d, "commit.after"), "remaining": get(d, "commit.remaining")})),
        ("commitError", *absent("commitError")),
    ],
    "tools": [
        ("toolsError", *absent("toolsError")),
        ("jdtls", "jdtls is installed", lambda d: (get(d, "tools.after.jdtls") in ("installed", "onPath", "custom"), get(d, "tools.after.jdtls"))),
        ("java-debug", "java-debug is installed", lambda d: (get(d, "tools.after.java-debug") in ("installed", "onPath", "custom"), get(d, "tools.after.java-debug"))),
    ],
    "lsp": [
        ("lspError", *absent("lspError")),
        ("jdtls.ready", "jdtls is ready", lambda d: (any(s.get("name") == "jdtls" and s.get("status") == "ready" for s in (get(d, "lsp.servers") or [])), get(d, "lsp.servers"))),
        ("completion.items", "completion returned items", lambda d: ((get(d, "lsp.completionItems") or 0) > 0, get(d, "lsp.completionItems"))),
        ("completion.first", "completion has a first label", lambda d: (bool(get(d, "lsp.completionFirst")), get(d, "lsp.completionFirst"))),
        ("hover", "hover returned documentation", lambda d: (bool(get(d, "lsp.hoverText")), get(d, "lsp.hoverText"))),
        ("definition", "definition resolved to App.java", lambda d: ((get(d, "lsp.definitionPath") or "").endswith("App.java"), get(d, "lsp.definitionPath"))),
        ("format", "format changed the buffer", lambda d: (get(d, "lsp.format.changed") is True, get(d, "lsp.format.changed"))),
        ("references.count", "find usages returned references", lambda d: ((get(d, "lsp.references.count") or 0) > 0, get(d, "lsp.references.count"))),
        ("references.done", "find usages finished", lambda d: (get(d, "lsp.references.done") is True, get(d, "lsp.references.done"))),
        ("rename", "rename was applied and undone", lambda d: (get(d, "lsp.rename.undone") is True, get(d, "lsp.rename.undone"))),
        ("symbols", "go to symbol found the class", lambda d: ((get(d, "lsp.symbols.count") or 0) > 0, get(d, "lsp.symbols.count"))),
        ("diagnostics", "JDT published diagnostics", lambda d: (bool(get(d, "lsp.diagnostics")), get(d, "lsp.diagnostics"))),
        ("problems.errors", "problems view counted the error", lambda d: ((get(d, "lsp.diagnostics.problems.errors") or 0) >= 1, get(d, "lsp.diagnostics.problems.errors"))),
        ("problems.rows", "problems view built a row", lambda d: ((get(d, "lsp.diagnostics.problems.rows") or 0) >= 1, get(d, "lsp.diagnostics.problems.rows"))),
        ("problems.visible", "problems tool window opened", lambda d: (get(d, "lsp.diagnostics.problems.visible") is True, get(d, "lsp.diagnostics.problems.visible"))),
    ],
    "debug": [
        ("debugError", *absent("debugError")),
        ("paused", "debugger paused at the breakpoint", lambda d: (get(d, "debug.paused") is True, get(d, "debug.paused"))),
        ("breakpoint", "breakpoint was set", lambda d: ((get(d, "debug.breakpoint") or 0) > 0, get(d, "debug.breakpoint"))),
        ("mainClass", "main class resolved", lambda d: (get(d, "debug.mainClass") == "com.example.App", get(d, "debug.mainClass"))),
        ("vars", "variables were read", lambda d: ((get(d, "debug.vars") or 0) > 0, get(d, "debug.vars"))),
        ("watch", "watch expression evaluated", lambda d: (get(d, "debug.watch") == "4", get(d, "debug.watch"))),
        ("step.paused", "step over paused again", lambda d: (get(d, "debug.afterStepPaused") is True, get(d, "debug.afterStepPaused"))),
        ("step.line", "step moved to the next line", lambda d: ((get(d, "debug.afterStepLine") or 0) > (get(d, "debug.frameLine") or 0), [get(d, "debug.frameLine"), get(d, "debug.afterStepLine")])),
        ("ended", "program ran to the end", lambda d: (get(d, "debug.ended") is True, get(d, "debug.ended"))),
        ("errors", "project compiles with no errors", lambda d: ((get(d, "debug.errorCount") or 0) == 0, get(d, "debug.errorCount"))),
        ("output", "program output was captured", lambda d: ("Hello, GoIDEA!" in (get(d, "debug.output") or ""), get(d, "debug.output"))),
    ],
}


def run(phase, paths):
    failures = []
    passes = 0
    for path in paths:
        print(f"{phase}: {path}")
        try:
            with open(path, encoding="utf-8") as f:
                data = json.load(f)
        except (OSError, json.JSONDecodeError) as e:
            print(f"  FAIL cannot read smoke output: {e}")
            failures.append(f"cannot read smoke output: {e}")
            continue
        for cid, label, check in CHECKS[phase]:
            try:
                ok, got = check(data)
            except Exception as e:  # a broken assertion must fail the gate, not crash it
                ok, got = False, f"assertion error: {e}"
            if ok:
                passes += 1
                print(f"  ok   {label}")
            else:
                failures.append(f"{phase}.{cid}: {label} (got {got!r})")
                print(f"  FAIL {label} (got {got!r})")
    if failures:
        print(f"\n{len(failures)} check(s) failed:", file=sys.stderr)
        for f in failures:
            print(f"  - {f}", file=sys.stderr)
        return 1
    print(f"\n{passes} checks passed")
    return 0


def main():
    if len(sys.argv) == 2 and sys.argv[1] == "--list":
        for phase, checks in CHECKS.items():
            for cid, label, _ in checks:
                print(f"{phase}.{cid}\t{label}")
        return 0
    if len(sys.argv) < 3 or sys.argv[1] not in CHECKS:
        print(__doc__, file=sys.stderr)
        return 2
    return run(sys.argv[1], sys.argv[2:])


if __name__ == "__main__":
    sys.exit(main())
