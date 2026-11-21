#!/usr/bin/env python3
"""Generate OPENIDEA_PARITY_MATRIX.md from the pinned OpenIDEA source.

OpenIDEA is the product specification. This script enumerates what the source
actually registers - feature folders, actions, keymaps, tool windows, overlays,
editor contributions, file editor types, settings pages and keys, languages,
IPC commands, events, streaming channels, backend crates and plugin extension
points - and maps every item to GoIDEA.

The matrix is complete by construction: every item found in the source gets a
row. GoIDEA status defaults to `missing`; a claim is made only in
scripts/openidea-parity-status.json, where each entry carries evidence (a
regression check id, a doc, or a source path). The gate runs this script with
--check, so a new action or command cannot land without appearing here.

Usage:
  scripts/openidea-parity.py                 # rewrite OPENIDEA_PARITY_MATRIX.md
  scripts/openidea-parity.py --check         # fail if the document is stale
"""

import importlib.util
import json
import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OPENIDEA = ROOT / "third_party" / "openidea"
DOC = ROOT / "OPENIDEA_PARITY_MATRIX.md"
STATUS_FILE = ROOT / "scripts" / "openidea-parity-status.json"

STATUS_ORDER = ["integrated", "partial", "engine", "blocked", "missing"]
STATUS_LABEL = {
    "integrated": "integrated",
    "partial": "partial",
    "engine": "engine-only",
    "blocked": "blocked",
    "missing": "missing",
}


def load_ipc():
    spec = importlib.util.spec_from_file_location("openidea_ipc", ROOT / "scripts" / "openidea-ipc.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def read(path):
    return Path(path).read_text(encoding="utf-8", errors="replace")


def sources(subdir="src", suffix=(".ts", ".tsx")):
    base = OPENIDEA / subdir
    for path in sorted(base.rglob("*")):
        if path.suffix not in suffix or not path.is_file():
            continue
        rel = path.relative_to(OPENIDEA).as_posix()
        if ".test." in rel or ".spec." in rel or "/node_modules/" in rel:
            continue
        yield rel, read(path)


def code_mask(text):
    """Blank strings and comments while preserving indices and line breaks."""
    out = list(text)
    i = 0
    state = "code"
    quote = ""
    while i < len(text):
        ch = text[i]
        nxt = text[i + 1] if i + 1 < len(text) else ""
        if state == "code":
            if ch == "/" and nxt == "/":
                out[i] = out[i + 1] = " "
                state, i = "line", i + 2
                continue
            if ch == "/" and nxt == "*":
                out[i] = out[i + 1] = " "
                state, i = "block", i + 2
                continue
            if ch == "/" and regex_can_start(text, i):
                out[i] = " "
                state, i = "regex", i + 1
                continue
            if ch in ("'", '"', "`"):
                quote = ch
                out[i] = " "
                state, i = "string", i + 1
                continue
        elif state == "line":
            if ch == "\n":
                state = "code"
            else:
                out[i] = " "
            i += 1
            continue
        elif state == "regex":
            if ch == "\\" and i + 1 < len(text):
                out[i] = " "
                if text[i + 1] != "\n":
                    out[i + 1] = " "
                i += 2
                continue
            if ch == "/":
                out[i] = " "
                state = "code"
            elif ch != "\n":
                out[i] = " "
            i += 1
            continue
        elif state == "block":
            if ch == "*" and nxt == "/":
                out[i] = out[i + 1] = " "
                state, i = "code", i + 2
                continue
            if ch != "\n":
                out[i] = " "
            i += 1
            continue
        elif state == "string":
            if ch == "\\" and i + 1 < len(text):
                out[i] = " "
                if text[i + 1] != "\n":
                    out[i + 1] = " "
                i += 2
                continue
            if ch == quote:
                out[i] = " "
                state = "code"
            elif ch != "\n":
                out[i] = " "
            i += 1
            continue
        i += 1
    return "".join(out)


def regex_can_start(text, slash):
    """Heuristic for a JavaScript regular-expression literal."""
    i = slash - 1
    while i >= 0 and text[i].isspace():
        i -= 1
    if i < 0:
        return True
    # `</Tag>` in TSX is a closing tag, not a regular-expression literal.
    # Treating `<` as an expression-start token masks the rest of the file
    # whenever a JSX node appears before an action or settings-page table.
    if text[i] in "([{=,:;!?&|+-*%^~>":
        return True
    word = re.search(r"([A-Za-z_]\w*)$", text[max(0, i - 16) : i + 1])
    return bool(
        word
        and word.group(1)
        in {
            "return",
            "case",
            "throw",
            "typeof",
            "instanceof",
            "in",
            "of",
            "yield",
            "await",
        }
    )


def call_blocks(text, name):
    """The argument text of every top-level `name(...)` call."""
    out = []
    mask = code_mask(text)
    for m in re.finditer(r"\b" + re.escape(name) + r"\s*\(", mask):
        start = m.end() - 1
        depth = 0
        i = start
        while i < len(text):
            ch = mask[i]
            if ch == "(":
                depth += 1
            elif ch == ")":
                depth -= 1
                if depth == 0:
                    break
            i += 1
        if depth == 0:
            out.append(text[start + 1 : i])
    return out


def objects_in(block):
    """Top-level `{...}` objects in an argument, so arrays of contributions split."""
    out = []
    depth = 0
    start = None
    mask = code_mask(block)
    for i, ch in enumerate(block):
        token = mask[i]
        if token == "{":
            if depth == 0:
                start = i
            depth += 1
        elif token == "}":
            depth -= 1
            if depth == 0 and start is not None:
                out.append(block[start : i + 1])
                start = None
    return out


def object_bodies(text):
    """Every balanced brace body, including objects nested in function bodies."""
    out = []
    stack = []
    mask = code_mask(text)
    for i, ch in enumerate(mask):
        if ch == "{":
            stack.append(i)
        elif ch == "}" and stack:
            start = stack.pop()
            out.append(text[start + 1 : i])
    return out


def top_level_match(pattern, body):
    """The first match at an object's own field depth, not inside a callback."""
    mask = code_mask(body)
    depth_at = [0] * len(body)
    depth = 0
    for i, ch in enumerate(mask):
        depth_at[i] = depth
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
    for match in pattern.finditer(body):
        if depth_at[match.start()] == 0:
            return match
    return None


ID_RE = re.compile(r"\bid\s*:\s*(?:\"([^\"]+)\"|'([^']+)'|`([^`]+)`)")
TITLE_RE = re.compile(r"\btitle\s*:\s*(?:\"([^\"]+)\"|'([^']+)'|`([^`]+)`)")
KIND_RE = re.compile(r"\bkind\s*:\s*(?:\"([^\"]+)\"|'([^']+)'|`([^`]+)`)")
ANCHOR_RE = re.compile(r"\banchor\s*:\s*\"([^\"]+)\"")
GROUP_RE = re.compile(r"\bgroup\s*:\s*\"([^\"]+)\"")


def val(match):
    if not match:
        return ""
    return next((g for g in match.groups() if g is not None), "")


_CONSTS = None


def const_maps():
    """String constants per file and globally, so `id: WINDOW` is not dropped."""
    global _CONSTS
    if _CONSTS is None:
        per_file, everywhere = {}, {}
        for rel, text in sources():
            found = dict(
                re.findall(
                    r"const\s+([A-Za-z_]\w*)\s*(?::\s*string\s*)?=\s*\"([^\"]*)\"", text
                )
            )
            per_file[rel] = found
            for name, value in found.items():
                everywhere.setdefault(name, set()).add(value)
        _CONSTS = (per_file, everywhere)
    return _CONSTS


def ident_value(raw, rel):
    per_file, everywhere = const_maps()
    if raw in per_file.get(rel, {}):
        return per_file[rel][raw]
    values = everywhere.get(raw, set())
    return next(iter(values)) if len(values) == 1 else ""


def string_field(obj, field, rel):
    literal = re.search(
        r"\b"
        + re.escape(field)
        + r"\s*:\s*(?:\"([^\"]*)\"|'([^']*)'|`([^`]*)`)",
        obj,
    )
    if literal:
        return val(literal)
    ref = re.search(r"\b" + re.escape(field) + r"\s*:\s*([A-Za-z_]\w*)\s*[,}]", obj)
    return ident_value(ref.group(1), rel) if ref else ""


def id_of(obj, rel):
    return string_field(obj, "id", rel)


def registrations(call, fields=("id",)):
    """(field values, source) for every contribution object passed to a call."""
    found = []
    for rel, text in sources():
        for name in (call,):
            for block in call_blocks(text, name):
                objs = objects_in(block) or [block]
                for obj in objs:
                    item = {"source": rel}
                    for field in fields:
                        if field == "id":
                            item["id"] = id_of(obj, rel)
                        elif field == "title":
                            item["title"] = string_field(obj, "title", rel)
                        elif field == "kind":
                            item["kind"] = val(KIND_RE.search(obj))
                        elif field == "anchor":
                            item["anchor"] = val(ANCHOR_RE.search(obj))
                        elif field == "group":
                            item["group"] = string_field(obj, "group", rel)
                    key = item.get(fields[0], "")
                    if key:
                        found.append(item)
    return found


def dedupe(items, key):
    seen = {}
    for item in items:
        seen.setdefault(item[key], item)
    return list(seen.values())


def feature_of(rel):
    parts = rel.split("/")
    if len(parts) >= 3 and parts[0] == "src" and parts[1] == "features":
        return parts[2]
    return None


def labeled_tuples(path, declaration):
    """`[key, label]` tuples from a typed tuple-array constant."""
    text = read(path)
    match = re.search(
        r"(?:export\s+)?const\s+" + re.escape(declaration) + r"[^=]*=\s*\[",
        text,
        re.S,
    )
    if not match:
        return []
    array = delimited(text, match.end() - 1, "[", "]")
    return re.findall(
        r'\[\s*"([^"]+)"\s*,\s*"([^"]+)"\s*\]',
        array,
    )


def expand_dynamic_action(item):
    """Concrete OpenIDEA ids for template actions whose domains are constants."""
    aid = item["id"]
    feature = OPENIDEA / "src" / "features"
    if aid == "project.option.${key}":
        values = labeled_tuples(
            feature / "project-tree" / "HeaderActions.tsx", "OPTION_LABELS"
        )
        return [
            {
                **item,
                "id": "project.option." + key,
                "title": "Project View: " + label,
            }
            for key, label in values
        ]
    if aid == "project.viewMode.${mode}":
        values = labeled_tuples(
            feature / "project-tree" / "store.ts", "VIEW_MODES"
        )
        return [
            {
                **item,
                "id": "project.viewMode." + mode,
                "title": "Project View: Show " + label,
            }
            for mode, label in values
        ]
    if aid == "project.sortBy.${sortBy}":
        values = labeled_tuples(
            feature / "project-tree" / "HeaderActions.tsx", "SORT_LABELS"
        )
        return [
            {
                **item,
                "id": "project.sortBy." + sort,
                "title": "Project View: " + label,
            }
            for sort, label in values
        ]
    if aid == "merge.${command}":
        text = read(feature / "vcs" / "mergeActions.ts")
        match = re.search(
            r"(?:export\s+)?const\s+COMMANDS\s*=\s*\[(.*?)\]\s*as\s+const",
            text,
            re.S,
        )
        if not match:
            return [item]
        commands = re.findall(
            r'\[\s*"([^"]+)"\s*,\s*"([^"]+)"(?:\s*,\s*"[^"]+")?\s*\]',
            match.group(1),
        )
        return [
            {
                **item,
                "id": "merge." + command,
                "title": title,
            }
            for command, title in commands
        ]
    if aid in ("navigate.toggleBookmark${d}", "navigate.gotoBookmark${d}"):
        return [
            {
                **item,
                "id": aid.replace("${d}", str(digit)),
                "title": item["title"].replace("${d}", str(digit)),
            }
            for digit in range(10)
        ]
    return [item]


def actions():
    items = []
    for rel, text in sources():
        # Read the action object that owns each id independently of the rest of
        # the file. This remains reliable in large TSX modules where JSX text,
        # regular-expression literals and nested callbacks can otherwise throw
        # off whole-file delimiter matching.
        for match in ID_RE.finditer(text):
            if obvious_comment(text, match.start()):
                continue
            obj = enclosing_braces(text, match.start())
            if not obj:
                continue
            if not re.search(r"\btitle\s*:", obj):
                continue
            if not re.search(r"\b(?:run|toggled)\s*:", obj):
                continue
            aid = val(match)
            if not action_id(aid):
                continue
            items.append(
                {
                    "id": aid,
                    "title": val(TITLE_RE.search(obj)),
                    "source": rel,
                }
            )
        for match in re.finditer(
            r"\bid\s*:\s*([A-Za-z_]\w*)\s*(?:,|})", text
        ):
            if obvious_comment(text, match.start()):
                continue
            obj = enclosing_braces(text, match.start())
            if not obj:
                continue
            if not re.search(r"\btitle\s*:", obj):
                continue
            if not re.search(r"\b(?:run|toggled)\s*:", obj):
                continue
            aid = ident_value(match.group(1), rel)
            if not action_id(aid):
                continue
            items.append(
                {
                    "id": aid,
                    "title": val(TITLE_RE.search(obj)),
                    "source": rel,
                }
            )

        for name in ("registerAction", "registerActions"):
            for block in call_blocks(text, name):
                for obj in objects_in(block) or [block]:
                    aid = id_of(obj, rel)
                    if not action_id(aid):
                        continue
                    items.append(
                        {"id": aid, "title": val(TITLE_RE.search(obj)), "source": rel}
                    )
        # Several features declare their action array first and pass the
        # variable to registerActions later. Treat the declaration as part of
        # the registration so task/reply menus and other dynamic actions stay
        # in the parity inventory.
        for m in re.finditer(
            r"(?:export\s+)?const\s+\w+\s*:\s*Action\[\]\s*=\s*\[", text
        ):
            array = delimited(text, m.end() - 1, "[", "]")
            for obj in objects_in(array):
                aid = id_of(obj, rel)
                if not action_id(aid):
                    continue
                items.append(
                    {"id": aid, "title": val(TITLE_RE.search(obj)), "source": rel}
                )

        # Actions commonly live in typed factory return values rather than in a
        # registerAction call's inline argument. Scan every object body that has
        # the required Action fields. The run/toggled test keeps settings and UI
        # descriptors with an unrelated id out of the action inventory.
        for obj in object_bodies(text):
            action_field = top_level_match(
                re.compile(r"\b(?:run|toggled)\s*:"), obj
            )
            if not action_field:
                continue
            title_field = top_level_match(re.compile(r"\btitle\s*:"), obj)
            if not title_field:
                continue
            id_match = top_level_match(ID_RE, obj)
            aid = val(id_match)
            if not aid:
                ref = top_level_match(
                    re.compile(r"\bid\s*:\s*([A-Za-z_]\w*)\s*[,}]"), obj
                )
                aid = ident_value(ref.group(1), rel) if ref else ""
            if not action_id(aid):
                continue
            items.append(
                {
                    "id": aid,
                    "title": val(top_level_match(TITLE_RE, obj)),
                    "source": rel,
                }
            )

        # Some modules define a local `edit`, `editorAction` or similar factory
        # whose Action object uses an id parameter. Recover the literal id
        # argument from its call sites.
        for factory, params, decl in factory_declarations(text):
            if "Action[]" in decl or "Action" not in decl:
                continue
            param_names = [param_name(p) for p in split_top_level(params, ",")]
            param_names = [name for name in param_names if name]
            id_param = factory_id_param(text, decl, params)
            if id_param not in param_names:
                continue
            id_index = param_names.index(id_param)
            for block in call_blocks(text, factory):
                args = split_top_level(block, ",")
                aid = literal_value(args[id_index]) if id_index < len(args) else ""
                if action_id(aid):
                    items.append({"id": aid, "title": "", "source": rel})
    expanded = []
    for item in items:
        expanded.extend(expand_dynamic_action(item))
    return dedupe(expanded, "id")


def action_id(value):
    """Global action ids are dotted. Templates are kept only after a namespace."""
    if not value or value.startswith("${"):
        return False
    return "." in value


def obvious_comment(text, pos):
    start = text.rfind("\n", 0, pos) + 1
    line = text[start:pos].lstrip()
    return line.startswith(("*", "//", "/*"))


def enclosing_braces(text, pos):
    """The nearest balanced `{...}` around pos, returned without its outer braces."""
    depth = 0
    open_at = -1
    for i in range(pos - 1, -1, -1):
        ch = text[i]
        if ch == "}":
            depth += 1
        elif ch == "{":
            if depth == 0:
                open_at = i
                break
            depth -= 1
    if open_at < 0:
        return ""
    depth = 0
    for i in range(open_at, len(text)):
        ch = text[i]
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return text[open_at + 1 : i]
    return ""


def factory_declarations(text):
    """(name, params, declaration prefix) for local functions returning Action."""
    out = []
    patterns = (
        r"(?:export\s+)?(?:const|let)\s+(\w+)\s*=\s*(?:async\s*)?\(([^)]*)\)\s*:\s*Action\s*=>",
        r"(?:export\s+)?function\s+(\w+)\s*\(([^)]*)\)\s*:\s*Action\s*\{",
    )
    for pattern in patterns:
        for m in re.finditer(pattern, text, re.S):
            out.append((m.group(1), m.group(2), m.group(0)))
    return out


def function_body_after(text, start):
    """A small bounded window after a declaration, enough to inspect its body."""
    return text[start : start + 1800]


def factory_id_param(text, decl, params):
    """The parameter used for the Action object's id field."""
    body = function_body_after(text, text.find(decl) + len(decl))
    for obj in object_bodies(body):
        if not top_level_match(re.compile(r"\b(?:run|toggled)\s*:"), obj):
            continue
        ref = top_level_match(
            re.compile(r"\bid\s*:\s*([A-Za-z_]\w*)\s*(?:,|$)"), obj
        )
        if ref and ref.group(1) in params:
            return ref.group(1)
        if re.search(r"(?:^|[,\n])\s*id\s*,", obj):
            for candidate in split_top_level(params, ","):
                if param_name(candidate) == "id":
                    return "id"
    return ""


def param_name(param):
    param = param.strip()
    if not param:
        return ""
    param = param.split("=", 1)[0].strip()
    return param.split(":", 1)[0].strip().split()[-1]


def split_top_level(text, separator):
    """Split source text on a one-character separator outside brackets."""
    parts = []
    start = 0
    depth = 0
    mask = code_mask(text)
    pairs = {"(": ")", "[": "]", "{": "}", "<": ">"}
    closes = set(pairs.values())
    for i, ch in enumerate(mask):
        if ch in pairs:
            depth += 1
        elif ch in closes and depth:
            depth -= 1
        elif ch == separator and depth == 0:
            parts.append(text[start:i])
            start = i + 1
    parts.append(text[start:])
    return parts


def literal_value(text):
    """A plain quoted string, rejecting templates with interpolation."""
    text = text.strip()
    match = re.fullmatch(r"\"([^\"]*)\"|'([^']*)'|`([^`]*)`", text, re.S)
    if not match:
        return ""
    value = val(match)
    return "" if "${" in value else value


def keymap_presets():
    presets = {}
    for rel, text in sources("src/platform/keymaps"):
        if "keymap" in rel and "test" in rel:
            continue
        name = Path(rel).stem
        bindings = re.findall(r"^\s*\"([^\"]+)\"\s*:\s*\[", text, re.M)
        presets[name] = sorted(set(bindings))
    return presets


def settings_keys():
    text = read(OPENIDEA / "src" / "bindings.ts")
    m = re.search(r"export type Settings_Serialize = \{(.*?)\n\};", text, re.S)
    if not m:
        return []
    keys = []
    for line in m.group(1).splitlines():
        line = line.strip()
        km = re.match(r"([A-Za-z_][A-Za-z0-9_]*)\??:", line)
        if km:
            keys.append(km.group(1))
    return keys


def delimited(text, start, opener, closer):
    """Text inside the bracket pair that starts at `start`."""
    depth = 0
    mask = code_mask(text)
    for i in range(start, len(text)):
        if mask[i] == opener:
            depth += 1
        elif mask[i] == closer:
            depth -= 1
            if depth == 0:
                return text[start + 1 : i]
    return ""


def camel(name):
    return re.sub(r"_([a-z0-9])", lambda m: m.group(1).upper(), name)


def rust_settings_keys():
    """Every fixed settings key reachable from `Settings`, flattened to dotted paths."""
    structs = {}
    for path in sorted((OPENIDEA / "crates" / "ide-core" / "src").glob("*settings*.rs")):
        text = read(path)
        for m in re.finditer(r"pub struct (\w+)\s*\{(.*?)\n\}", text, re.S):
            name, body = m.group(1), m.group(2)
            head = text[max(0, m.start() - 240) : m.start()]
            use_camel = 'rename_all = "camelCase"' in head
            fields = []
            pending_rename = None
            for line in body.splitlines():
                rm = re.match(r'\s*#\[serde\(rename\s*=\s*"([^"]+)"\)\]', line)
                if rm:
                    pending_rename = rm.group(1)
                    continue
                fm = re.match(r"\s*pub (\w+)\s*:\s*(.+?),\s*$", line)
                if not fm:
                    continue
                fname, ftype = fm.group(1), fm.group(2).strip()
                key = pending_rename or (camel(fname) if use_camel else fname)
                pending_rename = None
                fields.append((key, ftype))
            structs[name] = fields

    def base(t):
        t = t.strip()
        while True:
            m = re.match(r"(?:Option|Box|Arc|Rc)<(.+)>$", t)
            if not m:
                return t
            t = m.group(1).strip()

    def collection(t):
        return bool(re.match(r"(?:Vec|VecDeque|BTreeMap|HashMap|BTreeSet|HashSet|IndexMap)<", base(t)))

    leaves = set()

    def walk(name, prefix, depth, seen):
        if depth > 8 or name in seen:
            return
        for key, ftype in structs.get(name, []):
            path = key if not prefix else prefix + "." + key
            bt = base(ftype)
            if collection(ftype):
                leaves.add(path)
            elif bt in structs:
                walk(bt, path, depth + 1, seen | {name})
            else:
                leaves.add(path)

    walk("Settings", "", 0, frozenset())
    return sorted(leaves)


def settings_pages():
    """Every settings page: declarative PageSpec arrays and direct registrations."""
    items = []
    for rel, text in sources():
        if "PageSpec" not in text and "registerSettingsPage" not in text and "editorPage(" not in text:
            continue
        for m in re.finditer(
            r"(?:export\s+)?const\s+\w+\s*:\s*PageSpec(?:\[\])?\s*=\s*\[", text
        ):
            arr = delimited(text, m.end() - 1, "[", "]")
            for obj in objects_in(arr):
                items.append(page_from(obj, rel))
        for m in re.finditer(r"(?:export\s+)?const\s+\w+\s*:\s*PageSpec\s*=\s*\{", text):
            obj = delimited(text, m.end() - 1, "{", "}")
            items.append(page_from(obj, rel))
        for block in call_blocks(text, "registerSettingsPage"):
            for obj in objects_in(block) or [block]:
                items.append(page_from(obj, rel))
        for block in call_blocks(text, "editorPage"):
            for obj in objects_in(block) or [block]:
                items.append(page_from(obj, rel))
    return dedupe([i for i in items if i["id"]], "id")


def page_from(obj, rel):
    keys = []
    keys += re.findall(r"\bkey\s*:\s*\"([^\"]+)\"", obj)
    keys += re.findall(r"\buseSetting\s*(?:<[^>]*>)?\s*\(\s*\"([^\"]+)\"", obj)
    pid = id_of(obj, rel)
    if "${" in pid:
        pid = ""
    return {
        "id": pid,
        "title": string_field(obj, "title", rel),
        "group": string_field(obj, "group", rel),
        "source": rel,
        "keys": sorted(set(keys)),
    }


def languages():
    out = []
    base = OPENIDEA / "languages"
    for path in sorted(base.glob("*.toml")):
        text = read(path)
        lid = re.search(r'^\s*id\s*=\s*"([^"]+)"', text, re.M)
        name = re.search(r'^\s*name\s*=\s*"([^"]+)"', text, re.M)
        ts = re.search(r'^\s*tree-sitter\s*=\s*"([^"]+)"', text, re.M)
        dbg = re.search(r'^\s*\[debugger\]', text, re.M)
        out.append(
            {
                "id": lid.group(1) if lid else path.stem,
                "name": name.group(1) if name else path.stem,
                "tree_sitter": ts.group(1) if ts else "",
                "debugger": bool(dbg),
            }
        )
    return out


def events():
    text = read(OPENIDEA / "crates" / "ide-core" / "src" / "event.rs")
    out = []
    for enum in ("IdeEvent", "McpUiRequest"):
        m = re.search(r"pub enum " + enum + r"\s*\{(.*?)\n\}", text, re.S)
        if not m:
            continue
        for vm in re.finditer(r"^\s{4}([A-Z][A-Za-z0-9_]*)\s*(?:\{|\()", m.group(1), re.M):
            variant = vm.group(1)
            out.append(
                {
                    "enum": enum,
                    "variant": variant,
                    "wire": variant[0].lower() + variant[1:],
                }
            )
    return out


def crates(host_deps):
    out = []
    for path in sorted((OPENIDEA / "crates").glob("ide-*")):
        cargo = path / "Cargo.toml"
        if not cargo.exists():
            continue
        text = read(cargo)
        desc = re.search(r'^description\s*=\s*"([^"]*)"', text, re.M)
        name = path.name
        out.append(
            {
                "name": name,
                "dir": path.relative_to(OPENIDEA).as_posix(),
                "description": desc.group(1) if desc else "",
                "linked": name == "ide-host" or name in host_deps,
            }
        )
    return out


def host_dependencies():
    text = read(OPENIDEA / "crates" / "ide-host" / "Cargo.toml")
    return set(re.findall(r"^\s*(ide-[a-z0-9-]+)", text, re.M))


def plugin_surface():
    worlds = []
    for path in sorted((OPENIDEA / "wit").glob("*.wit")):
        text = read(path)
        for m in re.finditer(r"world\s+([a-z0-9-]+)\s*\{(.*?)\}", text, re.S):
            body = m.group(2)
            worlds.append(
                {
                    "name": m.group(1),
                    "file": path.relative_to(OPENIDEA).as_posix(),
                    "includes": re.findall(r"include\s+([a-z0-9-]+)\s*;", body),
                    "imports": re.findall(r"import\s+([a-z0-9-]+)\s*;", body),
                    "exports": re.findall(r"export\s+([a-z0-9-]+)\s*;", body),
                }
            )
    exports = sorted({e for w in worlds for e in w["exports"]})
    imports = sorted({i for w in worlds for i in w["imports"]})
    return worlds, imports, exports


def channel_commands(remote):
    out = []
    for cmd, args in remote.items():
        streaming = [a for a in args if a in ("channel", "sender") or a.startswith("on")]
        if streaming:
            out.append((cmd, streaming))
    return sorted(out)


def load_status():
    if not STATUS_FILE.exists():
        return {}
    data = json.loads(read(STATUS_FILE))
    return data.get("items", {})


def goidea_action_aliases():
    """OpenIDEA action ids redirected by GoIDEA's action registry."""
    text = read(ROOT / "internal" / "app" / "action.go")
    match = re.search(r"var actionAliases = map\[string\]string\{(.*?)\n\}", text, re.S)
    if not match:
        return {}
    return dict(re.findall(r'"([^"]+)"\s*:\s*"([^"]+)"', match.group(1)))


def goidea_registered_actions():
    """Ids registered by the native action/command tables, including aliases."""
    found = set()
    app_dir = ROOT / "internal" / "app"
    for path in app_dir.glob("*.go"):
        if path.name.endswith("_test.go"):
            continue
        text = read(path)
        found.update(
            match.group(1)
            for match in re.finditer(
                r'\{\s*"([A-Za-z][A-Za-z0-9_]*'
                r'(?:\.[A-Za-z][A-Za-z0-9_]*)+)"\s*,',
                text,
            )
        )
        found.update(
            match.group(1)
            for match in re.finditer(
                r'\baction\(\s*"([A-Za-z][A-Za-z0-9_]*'
                r'(?:\.[A-Za-z][A-Za-z0-9_]*)+)"\s*,',
                text,
            )
        )
        found.update(
            match.group(1)
            for match in re.finditer(
                r'\bID\s*:\s*"([A-Za-z][A-Za-z0-9_]*'
                r'(?:\.[A-Za-z][A-Za-z0-9_]*)+)"\s*,',
                text,
            )
        )
        # Several tables use a file-local constructor which takes the action id
        # as its first string. The literal itself is not in an ID: field, so
        # track those constructor names and their calls as real registrations.
        helpers = set()
        for match in re.finditer(r"(\w+)\s*:=\s*func\b", text):
            brace = text.find("{", match.end())
            if brace < 0:
                continue
            depth = 0
            end = -1
            for index in range(brace, len(text)):
                if text[index] == "{":
                    depth += 1
                elif text[index] == "}":
                    depth -= 1
                    if depth == 0:
                        end = index + 1
                        break
            if end < 0:
                continue
            body = text[brace:end]
            if "ID: id" in body.replace("\t", " ").replace("\n", " ") or re.search(
                r"ID\s*:\s*id\s*,", body
            ):
                helpers.add(match.group(1))
        for helper_name in helpers:
            helper_name = re.escape(helper_name)
            found.update(
                match.group(1)
                for match in re.finditer(
                    r"\b" + helper_name + r'\(\s*"([A-Za-z][A-Za-z0-9_]*'
                    r'(?:\.[A-Za-z][A-Za-z0-9_]*)+)"\s*,',
                    text,
                )
            )
    return found


def action_status(items, aid, aliases, registered):
    """An upstream id has the state of its canonical GoIDEA action."""
    status, evidence = status_doc(items, "action:" + aid, items)
    if status != "missing":
        return status, evidence
    target = aid
    seen = set()
    while target not in seen:
        seen.add(target)
        next_id = aliases.get(target, "")
        if not next_id:
            break
        target = next_id
    if target != aid:
        status, evidence = status_doc(items, "action:" + target, items)
        if status != "missing":
            return status, "OpenIDEA id `{}` aliases GoIDEA action `{}`; {}".format(
                aid, target, evidence
            )
    if aid in registered or target in registered:
        native_id = aid if aid in registered else target
        return (
            "partial",
            "internal/app registers the native action `{}`; interaction-level "
            "acceptance is pending".format(native_id),
        )
    return status, evidence


def status_doc(items, key, status):
    entry = items.get(key)
    if not entry:
        return "missing", ""
    st = entry.get("status", "missing")
    ev = entry.get("evidence", "").strip()
    if st != "missing" and not ev:
        raise SystemExit("{}: status {} needs evidence".format(key, st))
    return st, ev


def commit():
    try:
        out = subprocess.check_output(
            ["git", "-C", str(OPENIDEA), "rev-parse", "HEAD"], text=True
        ).strip()
        return out
    except Exception:
        return "unknown"


def cell(text):
    return str(text).replace("|", "\\|")


def status_cell(st, ev):
    label = STATUS_LABEL.get(st, st)
    if ev:
        return "{} - {}".format(label, cell(ev))
    if st == "missing":
        return "missing"
    return label


def render():
    ipc = load_ipc()
    remote = ipc.remote_commands()
    used = ipc.goidea_used(remote)
    tauri = ipc.tauri_commands()
    pin = ipc.upstream_pin()
    status = load_status()
    aliases = goidea_action_aliases()
    registered_actions = goidea_registered_actions()

    acts = actions()
    presets = keymap_presets()
    tools = dedupe(registrations("registerToolWindow", ("id", "title", "anchor")), "id")
    overlays = dedupe(registrations("registerOverlay", ("id",)), "id")
    contributions = dedupe(registrations("registerEditorContribution", ("id",)), "id")
    kinds = dedupe(registrations("registerEditorKind", ("kind",)), "kind")
    providers = dedupe(registrations("registerFileEditorProvider", ("id",)), "id")
    pages = settings_pages()
    engine_keys = rust_settings_keys()
    setting_keys = settings_keys()
    option_keys = {}
    for page in pages:
        for key in page["keys"]:
            option_keys.setdefault(key, []).append(page["id"])
    langs = languages()
    evs = events()
    host_deps = host_dependencies()
    crate_list = crates(host_deps)
    worlds, plugin_imports, plugin_exports = plugin_surface()
    channel_cmds = channel_commands(remote)

    feature_dirs = sorted(
        p.name for p in (OPENIDEA / "src" / "features").iterdir() if p.is_dir()
    )
    feature_stats = {}
    for fd in feature_dirs:
        feature_stats[fd] = {
            "actions": sum(1 for a in acts if feature_of(a["source"]) == fd),
            "toolwindows": sum(1 for t in tools if feature_of(t["source"]) == fd),
            "overlays": sum(1 for o in overlays if feature_of(o["source"]) == fd),
            "contributions": sum(1 for c in contributions if feature_of(c["source"]) == fd),
            "pages": sum(1 for p in pages if feature_of(p["source"]) == fd),
        }

    keymap_union = sorted({b for bs in presets.values() for b in bs})
    action_ids = {a["id"] for a in acts}
    keymap_only = [k for k in keymap_union if k not in action_ids]

    # Every inventory row shares one status table, keyed by kind:name.
    rows = []
    for fd in feature_dirs:
        rows.append(("feature:" + fd, status_doc(status, "feature:" + fd, status)))
    for a in acts:
        rows.append(
            (
                "action:" + a["id"],
                action_status(status, a["id"], aliases, registered_actions),
            )
        )
    for k in keymap_only:
        rows.append(
            (
                "action:" + k,
                action_status(status, k, aliases, registered_actions),
            )
        )
    for name in presets:
        rows.append(("keymap-preset:" + name, status_doc(status, "keymap-preset:" + name, status)))
    for t in tools:
        rows.append(("toolwindow:" + t["id"], status_doc(status, "toolwindow:" + t["id"], status)))
    for o in overlays:
        rows.append(("overlay:" + o["id"], status_doc(status, "overlay:" + o["id"], status)))
    for c in contributions:
        rows.append(("editor-contribution:" + c["id"], status_doc(status, "editor-contribution:" + c["id"], status)))
    for k in kinds:
        rows.append(("editor-kind:" + k["kind"], status_doc(status, "editor-kind:" + k["kind"], status)))
    for p in providers:
        rows.append(("file-editor:" + p["id"], status_doc(status, "file-editor:" + p["id"], status)))
    for p in pages:
        rows.append(("settings-page:" + p["id"], status_doc(status, "settings-page:" + p["id"], status)))
    all_setting_keys = sorted(set(engine_keys) | set(setting_keys) | set(option_keys))
    for k in all_setting_keys:
        rows.append(("setting:" + k, status_doc(status, "setting:" + k, status)))
    for l in langs:
        rows.append(("language:" + l["id"], status_doc(status, "language:" + l["id"], status)))
    for c in crate_list:
        st, ev = status_doc(status, "crate:" + c["name"], status)
        if st == "missing" and c["linked"]:
            st, ev = "engine", "linked into ide-host"
        rows.append(("crate:" + c["name"], (st, ev)))
    for e in evs:
        rows.append(("event:" + e["enum"] + "." + e["variant"], status_doc(status, "event:" + e["enum"] + "." + e["variant"], status)))
    for w in worlds:
        rows.append(("plugin-world:" + w["name"], status_doc(status, "plugin-world:" + w["name"], status)))
    for ex in plugin_exports:
        rows.append(("plugin-export:" + ex, status_doc(status, "plugin-export:" + ex, status)))
    for i in plugin_imports:
        rows.append(("plugin-import:" + i, status_doc(status, "plugin-import:" + i, status)))
    for _module, name in tauri:
        rows.append(("command:" + name, status_doc(status, "command:" + name, status)))

    known = {key for key, _ in rows}
    unknown = sorted(set(status) - known)
    if unknown:
        raise SystemExit(
            "scripts/openidea-parity-status.json names items that are not in the source: "
            + ", ".join(unknown)
        )

    counts = {}
    for key, (st, _) in rows:
        kind = key.split(":", 1)[0]
        counts.setdefault(kind, {}).setdefault(st, 0)
        counts[kind][st] += 1

    def totals(kind):
        c = counts.get(kind, {})
        return {s: c.get(s, 0) for s in STATUS_ORDER if s in c or s == "missing"}

    w = []
    add = w.append

    add("# OpenIDEA parity matrix")
    add("")
    add(
        "OpenIDEA is the product specification, Mayoi/MyGo is the frontend base and the\n"
        "OpenIDEA Rust core is the engine. This document enumerates everything the pinned\n"
        "OpenIDEA source registers and maps each item to GoIDEA. It is generated from\n"
        "`{}`; `scripts/openidea-parity.py --check` fails when it is stale."
        .format(commit())
    )
    add("")
    add("## How to read the status")
    add("")
    add("| Status | Meaning |")
    add("| --- | --- |")
    add("| `integrated` | GoIDEA has the feature and a gate check proves it. |")
    add("| `partial` | Part of the feature exists in GoIDEA, with evidence. |")
    add("| `engine-only` | The engine behind it is reachable over stdio, but no GoIDEA UI proof exists. |")
    add("| `blocked` | It needs an OpenIDEA change (an `ops` module, or a Host/crate change) first. |")
    add("| `missing` | No GoIDEA implementation is recorded. |")
    add("")
    add(
        "`missing` is the default and is not a claim that no code exists anywhere: it means\n"
        "no evidence has been recorded. A registered action with an otherwise undeclared\n"
        "status appears as `partial`; other raised statuses come from\n"
        "`scripts/openidea-parity-status.json`, where evidence names a regression check id,\n"
        "a doc or a source path. Nothing is hand-picked out of the inventory: the item\n"
        "lists below are generated from the source, and a new action, tool window, page or\n"
        "command appears automatically."
    )
    add("")
    add("## Inventory")
    add("")
    add("| Kind | Items | Integrated | Partial | Engine | Blocked | Missing |")
    add("| --- | ---: | ---: | ---: | ---: | ---: | ---: |")
    for kind, label in [
        ("feature", "Feature folders"),
        ("action", "Actions"),
        ("keymap-preset", "Keymap presets"),
        ("toolwindow", "Tool windows"),
        ("overlay", "Overlays"),
        ("editor-contribution", "Editor contributions"),
        ("editor-kind", "Editor kinds"),
        ("file-editor", "File editor providers"),
        ("settings-page", "Settings pages"),
        ("setting", "Settings keys"),
        ("language", "Languages"),
        ("crate", "Backend crates"),
        ("event", "Events"),
        ("plugin-world", "Plugin worlds"),
        ("plugin-export", "Plugin exports"),
    ]:
        c = totals(kind)
        total = sum(counts.get(kind, {}).values())
        add(
            "| {} | {} | {} | {} | {} | {} | {} |".format(
                label,
                total,
                c.get("integrated", 0),
                c.get("partial", 0),
                c.get("engine", 0),
                c.get("blocked", 0),
                c.get("missing", 0),
            )
        )
    add(
        "| IPC commands | {} | - | - | - | - | see [OPENIDEA_IPC.md](OPENIDEA_IPC.md) |".format(
            len(tauri)
        )
    )
    add("")

    # 1. Feature folders
    add("## Frontend feature folders")
    add("")
    add(
        "Every directory under `src/features` ({}). Counts are the contributions that\n"
        "directory registers.".format(len(feature_dirs))
    )
    add("")
    add("| Feature | Actions | Tool windows | Overlays | Editor | Settings pages | Status |")
    add("| --- | ---: | ---: | ---: | ---: | ---: | --- |")
    for fd in feature_dirs:
        s = feature_stats[fd]
        st, ev = status_doc(status, "feature:" + fd, status)
        add(
            "| `{}` | {} | {} | {} | {} | {} | {} |".format(
                fd,
                s["actions"],
                s["toolwindows"],
                s["overlays"],
                s["contributions"],
                s["pages"],
                status_cell(st, ev),
            )
        )
    add("")
    add(
        "Infrastructure folders (`src/editor`, `src/workbench`, `src/platform`, `src/ui`)\n"
        "are not product features; the contributions they host are listed below."
    )
    add("")

    # 2. Actions
    add("## Actions")
    add("")
    add(
        "{} registered action ids, plus {} ids that only presets bind ({} in total).\n"
        "Registered ids are listed by feature folder; keymap-only ids follow.".format(
            len(acts), len(keymap_only), len(keymap_union)
        )
    )
    add("")
    by_feature = {}
    for a in acts:
        by_feature.setdefault(feature_of(a["source"]) or "infrastructure", []).append(a)
    for fd in feature_dirs + ["infrastructure"]:
        group = sorted(by_feature.get(fd, []), key=lambda a: a["id"])
        if not group:
            continue
        add("### {}".format(fd))
        add("")
        add("| Action | Title | Source | Status |")
        add("| --- | --- | --- | --- |")
        for a in group:
            st, ev = action_status(
                status, a["id"], aliases, registered_actions
            )
            add(
                "| `{}` | {} | `{}` | {} |".format(
                    a["id"], cell(a["title"]), a["source"], status_cell(st, ev)
                )
            )
        add("")

    add("### Keymap-only ids")
    add("")
    add(
        "Ids bound by a preset but not registered by an action in the source: they are\n"
        "expected to come from a plugin or a hidden registration."
    )
    add("")
    add("| Action id | Bound in presets | Status |")
    add("| --- | --- | --- |")
    for k in keymap_only:
        bound = ", ".join(name for name, bs in presets.items() if k in bs)
        st, ev = action_status(status, k, aliases, registered_actions)
        add("| `{}` | {} | {} |".format(k, bound, status_cell(st, ev)))
    add("")

    # 3. Keymaps
    add("## Keymaps")
    add("")
    add("| Preset | Bindings | Status |")
    add("| --- | ---: | --- |")
    for name in sorted(presets):
        st, ev = status_doc(status, "keymap-preset:" + name, status)
        add("| `{}` | {} | {} |".format(name, len(presets[name]), status_cell(st, ev)))
    add("")
    add(
        "Counts are the bindings written literally in a preset file. A preset may spread\n"
        "another (`vscode-windows` translates and extends `vscode-macos`), so its effective\n"
        "map is larger; the union above and the keymap-only ids below are complete.\n"
        "GoIDEA's key handling is its own: a preset id does not imply that GoIDEA ships the\n"
        "same bindings until a check records it."
    )
    add("")

    # 4. Tool windows
    add("## Tool windows")
    add("")
    add("| Tool window | Title | Anchor | Source | Status |")
    add("| --- | --- | --- | --- | --- |")
    for t in sorted(tools, key=lambda t: t["id"]):
        st, ev = status_doc(status, "toolwindow:" + t["id"], status)
        add(
            "| `{}` | {} | {} | `{}` | {} |".format(
                t["id"], cell(t.get("title", "")), t.get("anchor", ""), t["source"], status_cell(st, ev)
            )
        )
    add("")

    # 5. Overlays
    add("## Overlays")
    add("")
    add("| Overlay | Source | Status |")
    add("| --- | --- | --- |")
    for o in sorted(overlays, key=lambda o: o["id"]):
        st, ev = status_doc(status, "overlay:" + o["id"], status)
        add("| `{}` | `{}` | {} |".format(o["id"], o["source"], status_cell(st, ev)))
    add("")

    # 6. Editor surface
    add("## Editor contributions and file editor types")
    add("")
    add("### Editor contributions")
    add("")
    add("| Contribution | Source | Status |")
    add("| --- | --- | --- |")
    for c in sorted(contributions, key=lambda c: c["id"]):
        st, ev = status_doc(status, "editor-contribution:" + c["id"], status)
        add("| `{}` | `{}` | {} |".format(c["id"], c["source"], status_cell(st, ev)))
    add("")
    add("### Editor kinds (file editor types)")
    add("")
    add(
        "The plain text editor is the workbench default and is not registered through\n"
        "`registerEditorKind`; the kinds below are the non-text inputs."
    )
    add("")
    add("| Kind | Source | Status |")
    add("| --- | --- | --- |")
    for k in sorted(kinds, key=lambda k: k["kind"]):
        st, ev = status_doc(status, "editor-kind:" + k["kind"], status)
        add("| `{}` | `{}` | {} |".format(k["kind"], k["source"], status_cell(st, ev)))
    add("")
    add("### File editor providers")
    add("")
    add("| Provider | Source | Status |")
    add("| --- | --- | --- |")
    for p in sorted(providers, key=lambda p: p["id"]):
        st, ev = status_doc(status, "file-editor:" + p["id"], status)
        add("| `{}` | `{}` | {} |".format(p["id"], p["source"], status_cell(st, ev)))
    add("")

    # 7. Settings
    add("## Settings")
    add("")
    add("### Pages")
    add("")
    add("| Page | Title | Group | Source | Status |")
    add("| --- | --- | --- | --- | --- |")
    for p in sorted(pages, key=lambda p: p["id"]):
        st, ev = status_doc(status, "settings-page:" + p["id"], status)
        add(
            "| `{}` | {} | {} | `{}` | {} |".format(
                p["id"], cell(p.get("title", "")), p.get("group", ""), p["source"], status_cell(st, ev)
            )
        )
    add("")
    add("### Keys")
    add("")
    add(
        "Every fixed key reachable from the engine's `Settings` type (`crates/ide-core`),\n"
        "flattened to dotted camelCase paths, plus keys an option on a page edits.\n"
        "Collection keys (`Vec`, maps) are one item: their entries are data, not fixed\n"
        "settings."
    )
    add("")
    add("| Key | Edited on | Status |")
    add("| --- | --- | --- |")
    for k in all_setting_keys:
        st, ev = status_doc(status, "setting:" + k, status)
        add(
            "| `{}` | {} | {} |".format(
                k, ", ".join(sorted(option_keys.get(k, []))), status_cell(st, ev)
            )
        )
    add("")

    # 8. Languages
    add("## Languages")
    add("")
    add("| Language | Id | Tree-sitter | Debugger | Status |")
    add("| --- | --- | --- | --- | --- |")
    for l in sorted(langs, key=lambda l: l["id"]):
        st, ev = status_doc(status, "language:" + l["id"], status)
        add(
            "| {} | `{}` | {} | {} | {} |".format(
                cell(l["name"]),
                l["id"],
                ("`{}`".format(l["tree_sitter"]) if l["tree_sitter"] else ""),
                ("yes" if l["debugger"] else ""),
                status_cell(st, ev),
            )
        )
    add("")

    # 9. IPC commands
    add("## IPC commands")
    add("")
    add(
        "All {} commands the desktop shell registers. `stdio` is whether the pinned\n"
        "`openidea-server` serves the command; `GoIDEA` is whether GoIDEA calls it.\n"
        "Local-only commands and the per-command argument lists are in\n"
        "[OPENIDEA_IPC.md](OPENIDEA_IPC.md).".format(len(tauri))
    )
    add("")
    add("| Command | Module | stdio | GoIDEA | Status |")
    add("| --- | --- | --- | --- | --- |")
    for module, name in sorted(tauri, key=lambda p: p[1]):
        served = "yes" if name in remote else "no"
        calls = "yes" if name in used else ""
        st, ev = status_doc(status, "command:" + name, status)
        add("| `{}` | {} | {} | {} | {} |".format(name, module, served, calls, status_cell(st, ev)))
    add("")
    add(
        "{} commands are served over stdio and {} are called by GoIDEA today. A command\n"
        "that is registered only in the desktop shell needs an `ops` module before stdio\n"
        "can serve it; a GoIDEA check counts only when the UI path works end to end.".format(
            len(remote), len(used)
        )
    )
    add("")

    # 10. Events and channels
    add("## Events and channels")
    add("")
    add("### Backend events")
    add("")
    add("| Enum | Variant | Wire type | Status |")
    add("| --- | --- | --- | --- |")
    for e in evs:
        st, evd = status_doc(status, "event:" + e["enum"] + "." + e["variant"], status)
        add("| {} | `{}` | `{}` | {} |".format(e["enum"], e["variant"], e["wire"], status_cell(st, evd)))
    add("")
    add(
        "The desktop shell forwards these as one Tauri event (`ide://event`). Over stdio\n"
        "they are `event` notifications; high-volume streams use `channel` notifications\n"
        "instead (RPC_PROTOCOL.md)."
    )
    add("")
    add("### Streaming channels")
    add("")
    add("| Command | Streaming arguments | Status |")
    add("| --- | --- | --- |")
    for cmd, args in channel_cmds:
        st, ev = status_doc(status, "command:" + cmd, status)
        add("| `{}` | {} | {} |".format(cmd, ", ".join("`{}`".format(a) for a in args), status_cell(st, ev)))
    add("")

    # 11. Backend crates
    add("## Backend feature modules (crates)")
    add("")
    add("| Crate | Role | Linked into `ide-host` (stdio reachable) | Status |")
    add("| --- | --- | --- | --- |")
    for c in crate_list:
        st, ev = status_doc(status, "crate:" + c["name"], status)
        if st == "missing" and c["linked"]:
            st, ev = "engine", "linked into ide-host"
        add(
            "| `{}` | {} | {} | {} |".format(
                c["name"], cell(c["description"]), "yes" if c["linked"] else "no", status_cell(st, ev)
            )
        )
    add("")
    add(
        "`engine-only` means the crate is linked into the stdio host, so its logic can be\n"
        "reached; it does not mean GoIDEA has a UI for it. A crate that is not linked\n"
        "needs an OpenIDEA change before any GoIDEA work can call it."
    )
    add("")

    # 12. Plugins
    add("## Plugin extension points")
    add("")
    add(
        "Plugins are sandboxed WebAssembly components (`wit/`, interface version 1.5.0).\n"
        "The base world imports the host API and exports three interfaces; optional worlds\n"
        "add exports. GoIDEA does not load plugins yet, so every entry starts as `missing`."
    )
    add("")
    add("### Host API imported by plugins ({})".format(len(plugin_imports)))
    add("")
    add("| Interface | Status |")
    add("| --- | --- |")
    for i in plugin_imports:
        st, ev = status_doc(status, "plugin-import:" + i, status)
        add("| `{}` | {} |".format(i, status_cell(st, ev)))
    add("")
    add("### Exported by plugins ({})".format(len(plugin_exports)))
    add("")
    add("| Export | Worlds | Status |")
    add("| --- | --- | --- |")
    for ex in plugin_exports:
        ws = sorted(w["name"] for w in worlds if ex in w["exports"])
        st, ev = status_doc(status, "plugin-export:" + ex, status)
        add("| `{}` | {} | {} |".format(ex, ", ".join(ws), status_cell(st, ev)))
    add("")
    add("### Worlds")
    add("")
    add("| World | File | Includes | Exports | Status |")
    add("| --- | --- | --- | --- | --- |")
    for wd in worlds:
        st, ev = status_doc(status, "plugin-world:" + wd["name"], status)
        add(
            "| `{}` | `{}` | {} | {} | {} |".format(
                wd["name"],
                wd["file"],
                ", ".join(wd["includes"]),
                ", ".join(wd["exports"]),
                status_cell(st, ev),
            )
        )
    add("")

    add("## Dynamic registrations")
    add("")
    add(
        "Call sites whose id is computed at run time (plugin views, parameterised actions).\n"
        "They cannot be enumerated from source; the product item is the call site itself."
    )
    add("")
    add("| Register | Source | Example |")
    add("| --- | --- | --- |")
    for call in (
        "registerAction",
        "registerActions",
        "registerToolWindow",
        "registerOverlay",
        "registerEditorContribution",
        "registerEditorKind",
        "registerFileEditorProvider",
        "registerSettingsPage",
    ):
        for rel, text in sources():
            for block in call_blocks(text, call):
                objs = objects_in(block) or [block]
                if any(id_of(o, rel) or KIND_RE.search(o) for o in objs):
                    continue
                snippet = " ".join(block.split())[:70]
                add("| `{}` | `{}` | `{}` |".format(call, rel, cell(snippet)))
    add("")

    add("## What this means for GoIDEA")
    add("")
    add(
        "The matrix is the port backlog, ordered by product surface rather than by what\n"
        "Mayoi happens to contain. An action, tool window or page is not ported because a\n"
        "similar Mayoi one exists; it is ported when the OpenIDEA interaction behind it\n"
        "works in GoIDEA and a gate check proves it. Backend work goes upstream through the\n"
        "OpenIDEA fork and a submodule bump, never by patching the submodule in place."
    )
    add("")
    return "\n".join(w) + "\n"


def main():
    check = "--check" in sys.argv[1:]
    text = render()
    try:
        current = DOC.read_text(encoding="utf-8")
    except FileNotFoundError:
        current = None
    if check:
        if current != text:
            sys.exit("OPENIDEA_PARITY_MATRIX.md is stale: run scripts/openidea-parity.py")
        print("OPENIDEA_PARITY_MATRIX.md is current")
        return
    DOC.write_text(text, encoding="utf-8")
    print("wrote {}".format(DOC.relative_to(ROOT)))


if __name__ == "__main__":
    main()
