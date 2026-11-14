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


# The states every run renders; the right-hierarchy shot needs the language
# server, so only the online phases add it.
SHOT_STATES = {
    "workbench", "editor", "bottom-todo", "split", "settings", "palette",
    "popup-quick-files", "search-everywhere", "balloons", "notifications",
    "left-structure", "dialog", "welcome-empty", "welcome",
    "welcome-customize", "welcome-plugins",
}


def absent(key, where=""):
    return f"{where}{key} is absent", lambda d: (key not in d, d.get(key))


CHECKS = {
    "shots": [
        ("count", "every visual-regression state was rendered", lambda d: (len(get(d, "shots") or {}) >= 10, sorted((get(d, "shots") or {}).keys()))),
        ("bytes", "each PNG is a real capture", lambda d: (all(v >= 15000 for v in (get(d, "shots") or {}).values()), {k: v for k, v in (get(d, "shots") or {}).items() if v < 15000})),
        ("states", "the baseline names every expected state", lambda d: (
            sorted(SHOT_STATES - set((get(d, "shots") or {}).keys())) == [],
            sorted(SHOT_STATES - set((get(d, "shots") or {}).keys())),
        )),
        ("errors", "no shot failed", lambda d: (get(d, "shotErrors") is None and get(d, "shotsError") is None, {"errors": get(d, "shotErrors"), "error": get(d, "shotsError")})),
    ],
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
        ("notify.count", "notifications recorded every level", lambda d: ((get(d, "notifications.count") or 0) >= 3, get(d, "notifications.count"))),
        ("notify.balloons", "the balloon stack held three", lambda d: (get(d, "notifications.balloons") == 3, get(d, "notifications.balloons"))),
        ("notify.sticky", "errors stayed until dismissed", lambda d: ((get(d, "notifications.sticky") or 0) >= 1, get(d, "notifications.sticky"))),
        ("notify.replace", "a keyed notification replaced the earlier one", lambda d: (get(d, "notifications.afterReplace") == 3, get(d, "notifications.afterReplace"))),
        ("notify.window", "the Notifications window opened on the right", lambda d: (get(d, "notifications.window") is True, get(d, "notifications.window"))),
        ("notify.read", "opening the list marked it read", lambda d: (get(d, "notifications.unreadAfterOpen") == 0, get(d, "notifications.unreadAfterOpen"))),
        ("notify.cleared", "Clear All emptied the history", lambda d: (get(d, "notifications.cleared") == 0, get(d, "notifications.cleared"))),
        ("commands", "command palette is populated", lambda d: ((get(d, "commands.all") or 0) >= 30, get(d, "commands.all"))),
        ("commands.invalidate", "the Invalidate Caches dialog opens, stages an option and closes", lambda d: (get(d, "commands.invalidateOpen") is True and get(d, "commands.invalidateStaged") is True and get(d, "commands.invalidateClosed") is True, {"open": get(d, "commands.invalidateOpen"), "staged": get(d, "commands.invalidateStaged"), "closed": get(d, "commands.invalidateClosed")})),
        ("settings.opened", "settings sheet opened", lambda d: (get(d, "settings.opened") is True, get(d, "settings.opened"))),
        ("settings.closed", "settings sheet closed", lambda d: (get(d, "settings.closed") is True, get(d, "settings.closed"))),
        ("settings.theme", "theme choice reached the disk", lambda d: (get(d, "settings.storedTheme") == "dark", get(d, "settings.storedTheme"))),
        ("watch.reloaded", "external edit was reloaded", lambda d: (get(d, "watch.reloaded") is True, get(d, "watch.reloaded"))),
        ("watch.reloadAll", "Reload All from Disk reread the clean buffer, and restoring it kept the file", lambda d: (get(d, "watch.reloadAll") is True and get(d, "watch.reloadAllRestored") is True, {"reloadAll": get(d, "watch.reloadAll"), "restored": get(d, "watch.reloadAllRestored")})),
        ("vcs.changes", "Git status reports the change", lambda d: ((d.get("vcsChanges") or 0) >= 1, d.get("vcsChanges"))),
        ("branch.head", "vcs_branch_info names the branch", lambda d: (bool(get(d, "branch.head")), get(d, "branch.head"))),
        ("branch.list", "vcs_branches lists the current branch", lambda d: ((get(d, "branch.branches") or 0) >= 1 and get(d, "branch.current") == get(d, "branch.head"), {"branches": get(d, "branch.branches"), "current": get(d, "branch.current")})),
        ("branch.checkout", "checkout switched branches and came back", lambda d: (get(d, "branch.checkedOut") == "goidea-smoke" and get(d, "branch.restored") is True, {"checkedOut": get(d, "branch.checkedOut"), "restored": get(d, "branch.restored")})),
        ("branchError", *absent("branchError")),
        ("stage.roundtrip", "vcs_stage staged the scratch file and vcs_unstage took it back", lambda d: (get(d, "stage.staged") is True and get(d, "stage.unstaged") is True, {"staged": get(d, "stage.staged"), "unstaged": get(d, "stage.unstaged")})),
        ("stage.discard", "vcs_discard removed the scratch file", lambda d: (get(d, "stage.discarded") is True, get(d, "stage.discarded"))),
        ("stageError", *absent("stageError")),
        ("bookmark.added", "F3 marked two lines", lambda d: (get(d, "bookmark.added") == 2, get(d, "bookmark.added"))),
        ("bookmark.gutter", "bookmarks paint gutter marks", lambda d: (get(d, "bookmark.gutterMarks") == 2, get(d, "bookmark.gutterMarks"))),
        ("bookmark.next", "Next Bookmark stepped to line 2 and then 5", lambda d: (get(d, "bookmark.nextFirst") == 2 and get(d, "bookmark.nextSecond") == 5, {"first": get(d, "bookmark.nextFirst"), "second": get(d, "bookmark.nextSecond")})),
        ("bookmark.prev", "Previous Bookmark stepped back to line 2", lambda d: (get(d, "bookmark.prev") == 2, get(d, "bookmark.prev"))),
        ("bookmark.remove", "F3 removed both marks", lambda d: (get(d, "bookmark.afterRemove") == 1 and get(d, "bookmark.cleared") == 0, {"afterRemove": get(d, "bookmark.afterRemove"), "cleared": get(d, "bookmark.cleared")})),
        ("bookmark.mnemonic", "Alt+F3 key reached its line and jumped there", lambda d: (get(d, "bookmark.mnemonicSet") == 2 and get(d, "bookmark.mnemonicJump") == 2, {"set": get(d, "bookmark.mnemonicSet"), "jump": get(d, "bookmark.mnemonicJump")})),
        ("bookmark.mnemonicUnique", "assigning a key took it from the other line", lambda d: (get(d, "bookmark.mnemonicMoved") == 5, get(d, "bookmark.mnemonicMoved"))),
        ("bookmark.window", "Cmd+2 shows every bookmark in the sidebar", lambda d: (get(d, "bookmark.windowMode") == "bookmarks" and get(d, "bookmark.windowRows") == 2, {"mode": get(d, "bookmark.windowMode"), "rows": get(d, "bookmark.windowRows")})),
        ("bookmark.file", "Toggle File Bookmark marked and unmarked the file", lambda d: (get(d, "bookmark.fileMarked") == 1 and get(d, "bookmark.fileCleared") == 0, {"marked": get(d, "bookmark.fileMarked"), "cleared": get(d, "bookmark.fileCleared")})),
        ("bookmark.list", "a new list received the new bookmarks", lambda d: (get(d, "bookmark.lists") == 2 and get(d, "bookmark.listAssigned") == 2, {"lists": get(d, "bookmark.lists"), "assigned": get(d, "bookmark.listAssigned")})),
        ("bookmark.listOps", "a list renamed and its deletion took its bookmarks", lambda d: (get(d, "bookmark.renamed") == "Temp 2" and get(d, "bookmark.listsAfterDelete") == (get(d, "bookmark.listsBeforeDelete") or 0) - 1 and get(d, "bookmark.marksAfterDelete") == 0, {"renamed": get(d, "bookmark.renamed"), "before": get(d, "bookmark.listsBeforeDelete"), "lists": get(d, "bookmark.listsAfterDelete"), "marks": get(d, "bookmark.marksAfterDelete")})),
        ("bookmark.popup", "Cmd+F3 lists files and lines by list", lambda d: (get(d, "bookmark.popupItems") == 3, get(d, "bookmark.popupItems"))),
        ("bookmark.description", "F2 in the popup edited the line's description", lambda d: (get(d, "bookmark.described") == "入口", get(d, "bookmark.described"))),
        ("bookmark.move", "the move chooser sent the line to another list, including a new one", lambda d: (get(d, "bookmark.movedTo") is True and get(d, "bookmark.movedAgain") is True and get(d, "bookmark.movedToNew") is True and get(d, "bookmark.moveKeptDefault") is True, {"to": get(d, "bookmark.movedTo"), "again": get(d, "bookmark.movedAgain"), "new": get(d, "bookmark.movedToNew")})),
        ("bookmark.folder", "a project folder was bookmarked and removed", lambda d: (get(d, "bookmark.folderMarked") is True and get(d, "bookmark.folderListed") == 1 and get(d, "bookmark.folderCleared") == 0, {"marked": get(d, "bookmark.folderMarked"), "listed": get(d, "bookmark.folderListed"), "cleared": get(d, "bookmark.folderCleared")})),
        ("bookmarkError", *absent("bookmarkError")),
        ("todo.done", "TODO scan finished", lambda d: (get(d, "todo.done") is True, get(d, "todo.done"))),
        ("todo.items", "TODO scan found the marker comment", lambda d: ((get(d, "todo.items") or 0) >= 1 and get(d, "todo.firstKeyword") == "TODO", {"items": get(d, "todo.items"), "keyword": get(d, "todo.firstKeyword")})),
        ("todo.openFiles", "open-files scope returned the current file's item", lambda d: ((get(d, "todo.openFilesItems") or 0) >= 1, get(d, "todo.openFilesItems"))),
        ("todo.visible", "the TODO window stayed open", lambda d: (get(d, "todo.visible") is True, get(d, "todo.visible"))),
        ("todo.settings", "the editor.todo page opens, applies a pattern and restores the defaults", lambda d: (get(d, "todo.settingsPage") is True and get(d, "todo.settingsDefaults") == 4 and get(d, "todo.settingsApplied") is True and get(d, "todo.settingsRestored") is True, {"page": get(d, "todo.settingsPage"), "defaults": get(d, "todo.settingsDefaults"), "applied": get(d, "todo.settingsApplied"), "restored": get(d, "todo.settingsRestored")})),
        ("todo.highlight", "the editor tinted the open file's TODO spans", lambda d: ((get(d, "todo.highlightSpans") or 0) >= 1, get(d, "todo.highlightSpans"))),
        ("todoError", *absent("todoError")),
        ("scratch.languages", "the scratch chooser listed the backend's languages", lambda d: ((get(d, "scratch.languages") or 0) >= 1, get(d, "scratch.languages"))),
        ("scratch.opened", "a scratch file was written in the data directory and opened", lambda d: (get(d, "scratch.opened") is True and get(d, "scratch.inScratchDir") is True and (get(d, "scratch.name") or "").startswith("scratch"), {"opened": get(d, "scratch.opened"), "inDir": get(d, "scratch.inScratchDir"), "name": get(d, "scratch.name")})),
        ("scratchError", *absent("scratchError")),
        ("term.output", "a real shell ran echo in the terminal", lambda d: ("GOIDEA_TERM_OK" in (get(d, "term.output") or ""), (get(d, "term.output") or "")[-300:])),
        ("term.tabs", "a second terminal tab opened and closed", lambda d: (get(d, "term.tabs") == 2 and get(d, "term.tabsAfterClose") == 1, {"tabs": get(d, "term.tabs"), "afterClose": get(d, "term.tabsAfterClose")})),
        ("term.split", "split right opened a second shell and unsplit merged it back", lambda d: (get(d, "term.split") == "right" and get(d, "term.splitGroups") == 2 and get(d, "term.splitReady") is True and get(d, "term.unsplitAxis") == "" and get(d, "term.unsplitGroups") == 1 and (get(d, "term.unsplitTabs") or 0) >= 2, {"split": get(d, "term.split"), "groups": get(d, "term.splitGroups"), "ready": get(d, "term.splitReady"), "unsplit": get(d, "term.unsplitGroups"), "tabs": get(d, "term.unsplitTabs")})),
        ("term.find", "find in the terminal marked the line with the query", lambda d: ((get(d, "term.findHits") or 0) >= 1 and get(d, "term.findAt") is not None and get(d, "term.findAt") >= 0 and bool(get(d, "term.findLabel")), {"hits": get(d, "term.findHits"), "at": get(d, "term.findAt"), "label": get(d, "term.findLabel")})),
        ("termError", *absent("termError")),
        ("files.menu", "the project view menu names New, Rename, Delete and Clipboard actions", lambda d: (any(s.startswith("新建/文件…/目录…") for s in (get(d, "files.menu") or [])) and any(s.startswith("重命名…") for s in (get(d, "files.menu") or [])), get(d, "files.menu"))),
        ("files.new", "fs_create_dir and fs_create_file made a folder and a file", lambda d: (get(d, "files.newDir") is True and get(d, "files.newFile") is True, {"dir": get(d, "files.newDir"), "file": get(d, "files.newFile")})),
        ("files.rename", "fs_rename renamed the file", lambda d: (get(d, "files.renamed") is True, get(d, "files.renamed"))),
        ("files.copy", "fs_copy pasted the file into another folder", lambda d: (get(d, "files.copied") is True, get(d, "files.copied"))),
        ("files.move", "a cut and paste moved the file with fs_rename", lambda d: (get(d, "files.moved") is True, get(d, "files.moved"))),
        ("files.delete", "fs_delete removed the file and the folder", lambda d: (get(d, "files.deletedFile") is True and get(d, "files.deletedDir") is True, {"file": get(d, "files.deletedFile"), "dir": get(d, "files.deletedDir")})),
        ("files.expand", "Collapse All folded the tree and Expand All reopened every folder", lambda d: ((get(d, "files.collapseRows") or 0) >= 1 and (get(d, "files.expandRows") or 0) >= (get(d, "files.collapseRows") or 0) + 2 and (get(d, "files.reloadRows") or 0) >= (get(d, "files.expandRows") or 0), {"collapse": get(d, "files.collapseRows"), "expand": get(d, "files.expandRows"), "reload": get(d, "files.reloadRows")})),
        ("files.select", "Select in Project View marked the active file", lambda d: (get(d, "files.selected") == "one.txt", get(d, "files.selected"))),
        ("files.findInFolder", "Find in Folder searched only the chosen folder", lambda d: ((get(d, "files.folderFiles") or 0) >= 1 and get(d, "files.folderUnder") == get(d, "files.folderFiles") and get(d, "files.folderEmpty") == 0, {"files": get(d, "files.folderFiles"), "under": get(d, "files.folderUnder"), "emptyFolder": get(d, "files.folderEmpty")})),
        ("files.reload", "Reload from Disk showed a file written behind the tree", lambda d: (get(d, "files.reloadSeen") is True and get(d, "files.reloadStatus") == "已从磁盘重新加载", {"seen": get(d, "files.reloadSeen"), "status": get(d, "files.reloadStatus")})),
        ("files.historyRestore", "Local History restored the older snapshot of a saved file", lambda d: (get(d, "files.historyRestored") is True and (get(d, "files.historyEntries") or 0) >= 1, {"entries": get(d, "files.historyEntries"), "restored": get(d, "files.historyRestored")})),
        ("files.historyLabel", "Put Label listed a named history point", lambda d: (get(d, "files.historyLabeled") is True and get(d, "files.historyLabelListed") is True, {"labeled": get(d, "files.historyLabeled"), "listed": get(d, "files.historyLabelListed")})),
        ("files.compareDirs", "Compare Directories classified both folders and diffed a pair", lambda d: (get(d, "files.compareDiffer") == 1 and get(d, "files.compareLeft") == 1 and get(d, "files.compareRight") == 1 and get(d, "files.compareEqual") == 1 and get(d, "files.compareDiffAdded") == 1 and get(d, "files.compareDiffGone") == 1, {"different": get(d, "files.compareDiffer"), "left": get(d, "files.compareLeft"), "right": get(d, "files.compareRight"), "equal": get(d, "files.compareEqual"), "added": get(d, "files.compareDiffAdded"), "gone": get(d, "files.compareDiffGone")})),
        ("filesError", *absent("filesError")),
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
        ("structure.rows", "structure view built rows", lambda d: ((get(d, "lsp.structure.rows") or 0) > 0, get(d, "lsp.structure.rows"))),
        ("structure.first", "structure view named the first symbol", lambda d: (bool(get(d, "lsp.structure.first")), get(d, "lsp.structure.first"))),
        ("structure.visible", "structure tool window opened", lambda d: (get(d, "lsp.structure.visible") is True, get(d, "lsp.structure.visible"))),
        ("hierarchy.rows", "call hierarchy listed a level", lambda d: ((get(d, "hierarchy.rows") or 0) >= 1, get(d, "hierarchy.rows"))),
        ("hierarchy.visible", "the hierarchy window showed on the right", lambda d: (get(d, "hierarchy.visible") is True and get(d, "hierarchy.mode") == "call", {"visible": get(d, "hierarchy.visible"), "mode": get(d, "hierarchy.mode")})),
        ("hierarchyError", *absent("hierarchyError")),
        ("se.files", "Search Everywhere listed files", lambda d: ((get(d, "searchEverywhere.files") or 0) >= 1, get(d, "searchEverywhere.files"))),
        ("se.symbols", "Search Everywhere listed symbols from the language server", lambda d: ((get(d, "searchEverywhere.symbols") or 0) >= 1, get(d, "searchEverywhere.symbols"))),
        ("se.actions", "Search Everywhere listed actions", lambda d: ((get(d, "searchEverywhere.actions") or 0) >= 1, get(d, "searchEverywhere.actions"))),
        ("se.text", "Search Everywhere listed text matches", lambda d: ((get(d, "searchEverywhere.text") or 0) >= 1, get(d, "searchEverywhere.text"))),
        ("se.sections", "the All tab carried the OpenIDEA sections", lambda d: (all(s in (get(d, "searchEverywhere.allSections") or []) for s in ("files", "symbols", "actions", "text")), get(d, "searchEverywhere.allSections"))),
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
