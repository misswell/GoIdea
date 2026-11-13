#!/usr/bin/env python3
"""Keeps REGRESSION_INDEX.json and the regression scripts honest.

Fails when a check in scripts/e2e-checks.py is missing from the index (an
undocumented regression), when the index names a check the script no longer
runs (a stale claim), when a check points at a doc that does not exist, or when
scripts/e2e.sh stops exercising a phase the gates rely on.
"""

import json
import os
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
INDEX = os.path.join(ROOT, "REGRESSION_INDEX.json")
CHECKS = os.path.join(ROOT, "scripts", "e2e-checks.py")
E2E = os.path.join(ROOT, "scripts", "e2e.sh")

problems = []


def fail(msg):
    problems.append(msg)


def load_index():
    with open(INDEX, encoding="utf-8") as f:
        return json.load(f)


def script_checks():
    out = subprocess.run(
        [sys.executable, CHECKS, "--list"], capture_output=True, text=True, check=True
    ).stdout
    checks = {}
    for line in out.splitlines():
        cid, label = line.split("\t", 1)
        checks[cid] = label
    return checks


def check_index(index, scripted):
    seen = set()
    ids = set()
    for c in index.get("checks", []):
        cid = c.get("id", "")
        if not cid:
            fail("a check has no id")
            continue
        if cid in ids:
            fail(f"{cid}: listed twice")
        ids.add(cid)
        seen.add(cid)
        if cid not in scripted:
            fail(f"{cid}: in the index but scripts/e2e-checks.py no longer runs it")
            continue
        if c.get("label") != scripted[cid]:
            fail(f"{cid}: index label {c.get('label')!r} != script label {scripted[cid]!r}")
        for field in ("feature", "area", "gate", "doc"):
            if not c.get(field):
                fail(f"{cid}: missing {field}")
        if c.get("gate") not in ("main", "release"):
            fail(f"{cid}: gate {c.get('gate')!r} is not main or release")
        doc = c.get("doc")
        if doc and not os.path.exists(os.path.join(ROOT, doc)):
            fail(f"{cid}: doc {doc} does not exist")
    for cid in sorted(set(scripted) - seen):
        fail(f"{cid}: runs in scripts/e2e-checks.py but is missing from the index")


def check_gates(index):
    gates = index.get("gates") or {}
    for name in ("pr", "main", "release"):
        g = gates.get(name)
        if not g:
            fail(f"gate {name} is not defined")
            continue
        if not g.get("when") or not g.get("steps") or not g.get("command"):
            fail(f"gate {name} needs when, steps and command")
        cmd = (g.get("command") or "").split()
        if len(cmd) == 2 and cmd[0] == "scripts/gate.sh":
            path = os.path.join(ROOT, cmd[0])
            if not os.access(path, os.X_OK):
                fail(f"gate {name}: {cmd[0]} is missing or not executable")
            else:
                with open(path, encoding="utf-8") as f:
                    body = f.read()
                if cmd[1] not in body:
                    fail(f"gate {name}: {cmd[0]} has no {cmd[1]} mode")
    release = gates.get("release") or {}
    if "scripts/e2e.sh" not in " ".join(release.get("steps") or []):
        fail("release gate does not run scripts/e2e.sh")


def check_e2e():
    with open(E2E, encoding="utf-8") as f:
        body = f.read()
    for phase, env in (
        ("base", "GOIDEA_SMOKE_EDIT=1"),
        ("tools", "GOIDEA_SMOKE_TOOLS=install"),
        ("lsp", "GOIDEA_SMOKE_LSP=1"),
        ("debug", "GOIDEA_SMOKE_DEBUG=1"),
    ):
        if env not in body:
            fail(f"scripts/e2e.sh no longer runs the {phase} phase ({env})")
    if "GOIDEA_E2E_OFFLINE" not in body:
        fail("scripts/e2e.sh lost the offline switch")


def main():
    try:
        index = load_index()
    except (OSError, json.JSONDecodeError) as e:
        print(f"cannot read {INDEX}: {e}", file=sys.stderr)
        return 1
    check_index(index, script_checks())
    check_gates(index)
    check_e2e()
    if problems:
        print(f"{len(problems)} regression index problem(s):", file=sys.stderr)
        for p in problems:
            print(f"  - {p}", file=sys.stderr)
        return 1
    print(f"regression index: {len(index.get('checks', []))} checks agree with e2e-checks.py")
    return 0


if __name__ == "__main__":
    sys.exit(main())
