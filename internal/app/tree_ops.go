package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// treeTarget is one selection of the project view, the subject of its
// context menu.
type treeTarget struct {
	path string
	dir  bool
	root bool
	// readOnly marks SDK and dependency rows contributed by External
	// Libraries. Their files can be opened and inspected, but project file
	// operations do not edit them.
	readOnly bool
	// git says the project's Git status loaded, which shows the Git submenu;
	// kind is the change status of a file target, and rollback says its
	// working-tree side can be discarded.
	git      bool
	kind     string
	rollback bool
	// workspaceMember marks a linked project root. workspaceAttachment says
	// the manifest's first project is the window root; OpenIDEA calls removing
	// one of its other members "Detach Project".
	workspaceMember      bool
	workspaceCurrentRoot bool
	workspaceAttachment  bool
}

// folder is the directory an action works in: the target itself for a
// folder, its parent for a file.
func (t treeTarget) folder() string {
	if t.dir {
		return t.path
	}
	return filepath.Dir(t.path)
}

func (m *Model) treeTargetFolder(target treeTarget) string {
	if target.dir {
		return target.path
	}
	return m.projectTreePathDir(target.path)
}

// selectedTreeEntry describes the project-view selection for commands that sit
// only in the main menu or palette. The root is protected from rename exactly
// as its context menu does.
func (m *Model) selectedTreeEntry() (treeTarget, bool) {
	if m.tree == nil {
		return treeTarget{}, false
	}
	selected := m.tree.Selected()
	if selected == "" {
		return treeTarget{}, false
	}
	return treeTarget{
		path: selected, dir: m.tree.IsDir(selected),
		readOnly:             m.tree.ReadOnly(selected),
		root:                 selected == m.Root() || m.workspaceContains(selected),
		workspaceMember:      m.workspaceContains(selected),
		workspaceCurrentRoot: workspacePathEqual(selected, m.Root()),
		workspaceAttachment:  m.workspaceIsAttachment(),
	}, true
}

// selectedTreeTargets describes every selected project-view row without
// collapsing nested paths. Comparison actions operate on these exact rows.
func (m *Model) selectedTreeTargets() []treeTarget {
	if m.tree == nil {
		return nil
	}
	seen := map[string]bool{}
	targets := make([]treeTarget, 0, len(m.tree.SelectedPaths()))
	for _, raw := range m.tree.SelectedPaths() {
		path := m.projectTreePathClean(raw)
		if path == "" || path == "." {
			continue
		}
		key := path
		if caseInsensitiveFileSystem() {
			key = strings.ToLower(path)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		readOnly := m.tree.ReadOnly(path)
		if !m.tree.IsDir(path) && !m.projectTreeEntryKnown(path) && !readOnly {
			continue
		}
		targets = append(targets, treeTarget{
			path: path, dir: m.tree.IsDir(path),
			readOnly:             readOnly,
			root:                 path == m.Root() || m.workspaceContains(path),
			workspaceMember:      m.workspaceContains(path),
			workspaceCurrentRoot: workspacePathEqual(path, m.Root()),
			workspaceAttachment:  m.workspaceIsAttachment(),
		})
	}
	return targets
}

func selectedTreeComparisonPaths(
	selection []treeTarget,
	fallback treeTarget,
	directories bool,
) []string {
	if len(selection) == 0 && fallback.path != "" {
		selection = []treeTarget{fallback}
	}
	if len(selection) != 2 {
		return nil
	}
	paths := make([]string, 0, 2)
	for _, target := range selection {
		if target.path == "" || target.dir != directories {
			return nil
		}
		paths = append(paths, target.path)
	}
	return paths
}

// RevealSelectedInFinder shows the selected file or folder in the platform's
// file manager.
func (m *Model) RevealSelectedInFinder() {
	if target, ok := m.selectedTreeEntry(); ok {
		if m.projectTreeUsesRemotePaths() {
			m.status = "Reveal is unavailable for a remote project."
			m.invalidate()
			return
		}
		revealPath(target.path)
	}
}

// OpenSelectedTreeEntry opens the selected file in the editor. OpenIDEA binds
// this Project View command to project.open and F4.
func (m *Model) OpenSelectedTreeEntry() {
	if target, ok := m.selectedTreeEntry(); ok && !target.dir {
		m.openFile(target.path)
		m.invalidate()
	}
}

// RenameSelectedTreeEntry opens OpenIDEA's Rename refactoring.
func (m *Model) RenameSelectedTreeEntry() {
	if target, ok := m.selectedTreeEntry(); ok && !target.root {
		m.OpenFileRenameDialog(target.path)
	}
}

// CopySelectedTreeElement opens the Copy File/Directory dialog for the
// project-view selection.
func (m *Model) CopySelectedTreeElement() {
	paths := m.selectedMutableTreePaths()
	if len(paths) > 0 {
		m.OpenFileCopyDialog(paths, m.projectTreePathDir(paths[0]))
	}
}

// DeleteSelectedTreeEntry opens the guarded Delete confirmation for the
// project-view selection.
func (m *Model) DeleteSelectedTreeEntry() {
	paths := m.selectedMutableTreePaths()
	if len(paths) > 0 {
		m.AskTreeDeletePaths(paths)
	}
}

// OpenSelectedTreeInTerminal starts a terminal at the selected folder, or at
// the file's parent folder.
func (m *Model) OpenSelectedTreeInTerminal() {
	if target, ok := m.selectedTreeEntry(); ok {
		m.OpenTerminalAt(m.treeTargetFolder(target))
	}
}

// CompareSelectedTreeFile opens the selected file and asks for its comparison
// target, matching the editor's Compare With… action.
func (m *Model) CompareSelectedTreeFile() {
	target, ok := m.selectedTreeEntry()
	if !ok || target.dir {
		return
	}
	m.compareTreeFile(target.path)
}

func (m *Model) compareTreeFile(path string) {
	m.openFileThen(path, func(tab *Tab) {
		m.PickCompareFile(tab)
	})
}

// FindUsagesOfSelectedTreeFile opens the file and runs Find Usages at the
// current caret.
func (m *Model) FindUsagesOfSelectedTreeFile() {
	target, ok := m.selectedTreeEntry()
	if !ok || target.dir {
		return
	}
	m.findUsagesOfTreeFile(target.path)
}

func (m *Model) findUsagesOfTreeFile(path string) {
	m.openFileThen(path, func(*Tab) { m.FindUsages() })
}

// FormatSelectedTreeFile opens the selected source and formats it through the
// active language service.
func (m *Model) FormatSelectedTreeFile() {
	target, ok := m.selectedTreeEntry()
	if !ok || target.dir {
		return
	}
	m.formatTreeFile(target.path)
}

func (m *Model) formatTreeFile(path string) {
	m.openFileThen(path, func(*Tab) { m.FormatActive() })
}

// OptimizeImportsOfSelectedTreeFile opens the selected source and runs the
// server's source.organizeImports action, matching OpenIDEA's project variant.
func (m *Model) OptimizeImportsOfSelectedTreeFile() {
	target, ok := m.selectedTreeEntry()
	if !ok || target.dir {
		return
	}
	m.optimizeImportsTreeFile(target.path)
}

func (m *Model) optimizeImportsTreeFile(path string) {
	m.openFileThen(path, func(tab *Tab) { m.optimizeImportsTab(tab) })
}

// treeAct is what choosing a project view menu item does.
type treeAct int

const (
	treeActNone treeAct = iota
	treeActSeparator
	treeActNewFile
	treeActNewDir
	treeActCut
	treeActCopy
	treeActPaste
	treeActCopyPath
	treeActJump
	treeActFindUsages
	treeActFindInFolder
	treeActRename
	treeActMove
	treeActCopyElement
	treeActDelete
	treeActTerminal
	treeActRevealFinder
	treeActOpenInBrowser
	treeActTogglePreview
	treeActSendToAgent
	treeActReformat
	treeActOptimizeImports
	treeActGoGenerateFile
	treeActGoGeneratePackage
	treeActShowHistory
	treeActPutLabel
	treeActCompareDirs
	treeActCompareFiles
	treeActCompareWith
	treeActCompareClipboard
	treeActReload
	treeActGitCommit
	treeActGitAdd
	treeActGitIgnoreShared
	treeActGitIgnoreLocal
	treeActGitAnnotate
	treeActGitDiff
	treeActGitCompare
	treeActGitCurrentRevision
	treeActGitHistorySelection
	treeActGitHistory
	treeActGitRollback
	treeActGitBranches
	treeActGitUpdate
	treeActGitPush
	treeActGitFetch
	treeActGitMerge
	treeActGitRebase
	treeActGitCheckout
	treeActGitNewBranch
	treeActGitNewTag
	treeActGitWorktrees
	treeActGitNewWorktree
	treeActGitRemotes
	treeActGitResolveConflicts
	treeActGitStash
	treeActGitUnstash
	treeActGitReset
	treeActProjectDetach
	treeActWorkspaceRemove
)

// treeMenuItem is one row of the project view menu: an action, a separator or
// a submenu, as OpenIDEA's ProjectViewPopupMenu has them.
type treeMenuItem struct {
	Act     treeAct
	Label   string
	Items   []treeMenuItem
	Enabled bool
}

// treeMenuItems is the menu of target t, as OpenIDEA orders it: New, the
// clipboard group, Jump to Source, Open in Terminal, Local History and the Git
// group, then Rename and Delete. Items that cannot apply to the selection are
// disabled.
func treeMenuItems(t treeTarget, clipboard bool, selection ...treeTarget) []treeMenuItem {
	item := func(act treeAct, label string, on bool) treeMenuItem {
		return treeMenuItem{Act: act, Label: label, Enabled: on}
	}
	moveLabel, copyLabel := "Move File…", "Copy File…"
	if t.dir {
		moveLabel, copyLabel = "Move Directory…", "Copy Directory…"
	}
	if len(selection) == 0 && t.path != "" {
		selection = []treeTarget{t}
	}
	count := len(selection)
	if count > 1 {
		moveLabel, copyLabel = "Move…", "Copy…"
	}
	writable := !t.readOnly && count > 0
	sep := treeMenuItem{Act: treeActSeparator}
	items := make([]treeMenuItem, 0, 48)
	if t.workspaceMember && !t.workspaceCurrentRoot {
		act, label := treeActWorkspaceRemove, "Remove from Workspace"
		if t.workspaceAttachment {
			act, label = treeActProjectDetach, "Detach Project"
		}
		items = append(items,
			item(act, label, true),
			sep,
		)
	}
	items = append(items,
		treeMenuItem{Label: "New", Items: []treeMenuItem{
			item(treeActNewFile, "File…", !t.readOnly),
			item(treeActNewDir, "Directory…", !t.readOnly),
		}},
		sep,
		item(treeActCut, "Cut", !t.root && writable),
		item(treeActCopy, "Copy", !t.root && count > 0),
		treeMenuItem{Label: "Copy Path/Reference", Items: []treeMenuItem{
			item(treeActCopyPath, "Absolute Path", true),
			item(treeActCopyPath, "File Name", true),
			item(treeActCopyPath, "Path From Content Root", true),
		}},
		item(treeActPaste, "Paste", clipboard && !t.readOnly),
		sep,
		item(treeActJump, "Jump to Source", !t.dir),
		item(treeActFindUsages, "Find Usages", !t.dir),
		item(treeActFindInFolder, "Find in Folder…", t.dir),
		sep,
		treeMenuItem{Label: "Refactor", Items: []treeMenuItem{
			item(treeActRename, "Rename…", !t.root && writable && count == 1),
			item(treeActMove, moveLabel, !t.root && writable),
			item(treeActCopyElement, copyLabel, !t.root && writable),
		}},
		item(treeActReformat, "Reformat Code", !t.dir && !t.readOnly),
		item(treeActOptimizeImports, "Optimize Imports", !t.dir && !t.readOnly),
	)
	if count == 1 && !t.dir && !t.readOnly && strings.EqualFold(filepath.Ext(t.path), ".go") {
		items = append(items, treeMenuItem{Label: "Go Generate", Items: []treeMenuItem{
			item(treeActGoGenerateFile, "Go Generate File", true),
			item(treeActGoGeneratePackage, "Go Generate Package", true),
		}})
	}
	openInItems := []treeMenuItem{
		item(treeActRevealFinder, "Reveal in Finder", true),
		item(treeActTerminal, "Terminal", true),
	}
	if count == 1 && !t.dir && isWebPath(t.path) {
		browserItems := []treeMenuItem{
			item(treeActOpenInBrowser, "Default Browser", true),
		}
		if isHTMLPath(t.path) {
			browserItems = append(browserItems, item(treeActTogglePreview, "Built-in Preview", true))
		}
		openInItems = append(openInItems, treeMenuItem{Label: "Browser", Items: browserItems})
	}
	items = append(items,
		item(treeActDelete, "Delete…", !t.root && !t.readOnly),
		sep,
		treeMenuItem{Label: "Open In", Items: openInItems},
		item(treeActSendToAgent, "Send to Agent", !t.root),
		treeMenuItem{Label: "Local History", Items: []treeMenuItem{
			item(treeActShowHistory, "Show History…", true),
			item(treeActPutLabel, "Put Label…", true),
		}},
	)
	if t.git {
		items = append(items, treeGitMenu(t))
	}
	if count == 1 && !t.dir && isWebPath(t.path) {
		items = append(items,
			item(treeActOpenInBrowser, "Open '"+filepath.Base(t.path)+"' in Browser", true),
		)
	}
	switch treeComparisonAction(selection) {
	case treeActCompareDirs:
		items = append(items, item(treeActCompareDirs, "Compare Directories…", true))
	case treeActCompareFiles:
		items = append(items, item(treeActCompareFiles, "Compare Files", true))
	case treeActCompareWith:
		items = append(items, item(treeActCompareWith, "Compare With…", true))
		items = append(items, item(treeActCompareClipboard, "Compare with Clipboard", true))
	}
	return append(items, item(treeActReload, "Reload from Disk", true))
}

// treeComparisonAction follows OpenIDEA's selection rules: two directories
// compare recursively, two files compare directly, and one row asks for its
// counterpart. Mixed or larger selections have no comparison action.
func treeComparisonAction(selection []treeTarget) treeAct {
	switch len(selection) {
	case 1:
		if selection[0].path != "" {
			return treeActCompareWith
		}
	case 2:
		if selection[0].path == "" || selection[1].path == "" ||
			selection[0].dir != selection[1].dir {
			return treeActSeparator
		}
		if selection[0].dir {
			return treeActCompareDirs
		}
		return treeActCompareFiles
	}
	return treeActSeparator
}

// treeGitMenu is the project view's Git submenu, as OpenIDEA orders it.
func treeGitMenu(t treeTarget) treeMenuItem {
	item := func(act treeAct, label string, on bool) treeMenuItem {
		return treeMenuItem{Act: act, Label: label, Enabled: on}
	}
	commit := "Commit File…"
	if t.dir {
		commit = "Commit…"
	}
	items := []treeMenuItem{
		item(treeActGitCommit, commit, t.dir || t.kind != "conflicted"),
		item(treeActGitAdd, "Add", t.kind == "untracked"),
		{Label: "Add to .gitignore", Items: []treeMenuItem{
			item(treeActGitIgnoreShared, ".gitignore", !t.root),
			item(treeActGitIgnoreLocal, ".git/info/exclude", !t.root),
		}},
		{Act: treeActSeparator},
		item(treeActGitAnnotate, "Annotate with Git Blame", !t.dir),
		item(treeActGitDiff, "Show Diff", !t.dir),
		item(treeActGitCompare, "Compare with Branch or Revision…", true),
	}
	if !t.dir {
		items = append(items,
			item(treeActGitCompare, "Compare with Revision…", true),
		)
	}
	items = append(items, item(treeActGitHistory, "Show History", true))
	if !t.dir {
		items = append(items,
			item(treeActGitHistorySelection, "Show History for Selection", true),
			item(treeActGitCurrentRevision, "Show Current Revision", true),
		)
	}
	items = append(items,
		item(treeActGitRollback, "Rollback…", t.rollback),
		treeMenuItem{Act: treeActSeparator},
		item(treeActGitBranches, "Branches…", true),
		item(treeActGitUpdate, "Pull…", true),
		item(treeActGitPush, "Push…", true),
		item(treeActGitFetch, "Fetch", true),
		item(treeActGitMerge, "Merge…", true),
		item(treeActGitRebase, "Rebase…", true),
		item(treeActGitCheckout, "Checkout Tag or Revision…", true),
		item(treeActGitNewBranch, "New Branch…", true),
		item(treeActGitNewTag, "New Tag…", true),
		item(treeActGitWorktrees, "Worktrees…", true),
		item(treeActGitNewWorktree, "New Worktree…", true),
		item(treeActGitRemotes, "Manage Remotes…", true),
		item(treeActGitResolveConflicts, "Resolve Conflicts…", true),
		item(treeActGitStash, "Stash Changes…", true),
		item(treeActGitUnstash, "Unstash Changes…", true),
		item(treeActGitReset, "Reset HEAD…", true),
	)
	return treeMenuItem{Label: "Git", Items: items}
}

// treeMenuLabels flattens the menu to its labels, the separators dropped and
// submenus joined with a slash, for the offscreen check.
func treeMenuLabels(items []treeMenuItem) []string {
	var out []string
	for _, it := range items {
		if it.Act == treeActSeparator {
			continue
		}
		if len(it.Items) > 0 {
			out = append(out, it.Label+"/"+strings.Join(treeMenuLabels(it.Items), "/"))
			continue
		}
		out = append(out, it.Label)
	}
	return out
}

// treeMenu draws target t's context menu.
func (m *Model) treeMenu(c *ui.Context, menu *ui.Menu, t treeTarget) {
	selection := m.selectedTreeTargets()
	items := treeMenuItems(t, m.treeClip != nil, selection...)
	if len(selection) == 1 && !selection[0].dir {
		m.buildTreeMenu(c, menu, items, t)
		if len(m.fileTypeMenuLanguages()) > 0 {
			menu.Separator()
			m.fileTypeOverrideMenu(menu, selection[0].path)
		}
	} else {
		m.buildTreeMenu(c, menu, items, t)
	}
	if !t.dir {
		m.compileFileMenu(menu, t.path)
	}
}

func (m *Model) buildTreeMenu(c *ui.Context, menu *ui.Menu, items []treeMenuItem, t treeTarget) {
	for _, it := range items {
		it := it
		switch {
		case it.Act == treeActSeparator:
			menu.Separator()
		case len(it.Items) > 0:
			menu.Submenu(it.Label, func(sub *ui.Menu) { m.buildTreeMenu(c, sub, it.Items, t) })
		case menu.Item(it.Label).Disabled(!it.Enabled).Chosen():
			m.runTreeAct(c, it, t)
		}
	}
}

func (m *Model) runTreeAct(c *ui.Context, it treeMenuItem, t treeTarget) {
	folder := m.treeTargetFolder(t)
	switch {
	case it.Act == treeActNewFile:
		m.OpenProjectNew(folder)
	case it.Act == treeActNewDir:
		m.OpenNewTreeEntry(folder, true)
	case it.Act == treeActCut:
		m.CutSelectedTreeEntry()
	case it.Act == treeActCopy:
		m.CopySelectedTreeEntry()
	case it.Act == treeActPaste:
		m.PasteTreeEntry(folder)
	case it.Act == treeActCopyPath:
		m.CopyTreePath(c, t.path, it.Label)
	case it.Act == treeActJump:
		m.openFile(t.path)
		m.invalidate()
	case it.Act == treeActFindUsages:
		m.findUsagesOfTreeFile(t.path)
	case it.Act == treeActFindInFolder:
		m.ShowSearchInFolder(folder)
	case it.Act == treeActRename:
		m.OpenFileRenameDialog(t.path)
	case it.Act == treeActMove:
		m.moveTreeEntry(t)
	case it.Act == treeActCopyElement:
		paths := m.selectedMutableTreePaths()
		if len(paths) == 0 {
			paths = []string{t.path}
		}
		m.OpenFileCopyDialog(paths, m.projectTreePathDir(paths[0]))
	case it.Act == treeActDelete:
		paths := m.selectedMutableTreePaths()
		if len(paths) == 0 {
			paths = []string{t.path}
		}
		m.AskTreeDeletePaths(paths)
	case it.Act == treeActTerminal:
		m.OpenTerminalAt(folder)
	case it.Act == treeActRevealFinder:
		revealPath(t.path)
	case it.Act == treeActOpenInBrowser:
		m.OpenWebInBrowser(t.path)
	case it.Act == treeActTogglePreview:
		m.ToggleWebPreview(t.path)
	case it.Act == treeActSendToAgent:
		m.SendPathsToAgent([]string{t.path})
	case it.Act == treeActReformat:
		m.formatTreeFile(t.path)
	case it.Act == treeActOptimizeImports:
		m.optimizeImportsTreeFile(t.path)
	case it.Act == treeActGoGenerateFile:
		m.RunCommandWithArg("go.generate.file", map[string]any{"path": t.path})
	case it.Act == treeActGoGeneratePackage:
		m.RunCommandWithArg("go.generate.package", map[string]any{"path": t.path})
	case it.Act == treeActShowHistory:
		m.ShowHistory(t.path)
	case it.Act == treeActPutLabel:
		m.OpenHistoryLabel()
	case it.Act == treeActCompareDirs:
		paths := selectedTreeComparisonPaths(m.selectedTreeTargets(), t, true)
		if len(paths) == 2 {
			m.CompareDirectoriesArg(paths)
		}
	case it.Act == treeActCompareFiles:
		paths := selectedTreeComparisonPaths(m.selectedTreeTargets(), t, false)
		if len(paths) == 2 {
			m.CompareFiles(paths)
		}
	case it.Act == treeActCompareWith:
		m.ComparePathWith(t.path)
	case it.Act == treeActCompareClipboard:
		m.ComparePathWithClipboard(t.path)
	case it.Act == treeActReload:
		m.ReloadPathFromDisk(t.path)
	case it.Act == treeActGitCommit:
		if t.dir {
			m.OpenCommitDir(t.path)
		} else {
			m.OpenCommitFile(t.path)
		}
	case it.Act == treeActGitAdd:
		if f, ok := m.statusFor(realPath(t.path)); ok {
			m.StageChange(f)
		}
	case it.Act == treeActGitIgnoreShared:
		m.AddToIgnore(t.path, "gitignore")
	case it.Act == treeActGitIgnoreLocal:
		m.AddToIgnore(t.path, "localExclude")
	case it.Act == treeActGitAnnotate:
		m.ShowAnnotate(t.path)
	case it.Act == treeActGitDiff:
		m.ShowDiff(t.path)
	case it.Act == treeActGitCompare:
		m.OpenCompareWithRevision(map[string]any{"path": t.path})
	case it.Act == treeActGitCurrentRevision:
		m.ShowCurrentRevision(map[string]any{"path": t.path})
	case it.Act == treeActGitHistorySelection:
		m.ShowHistoryForFile(map[string]any{"path": t.path})
	case it.Act == treeActGitHistory:
		m.ShowHistory(t.path)
	case it.Act == treeActGitRollback:
		if f, ok := m.statusFor(realPath(t.path)); ok {
			m.AskRollback(f)
		}
	case it.Act == treeActGitBranches:
		m.ShowBranches()
	case it.Act == treeActGitUpdate:
		m.OpenGitPull()
	case it.Act == treeActGitPush:
		m.OpenGitPush()
	case it.Act == treeActGitFetch:
		m.GitFetch()
	case it.Act == treeActGitMerge:
		m.OpenGitMerge()
	case it.Act == treeActGitRebase:
		m.OpenGitRebase()
	case it.Act == treeActGitCheckout:
		m.OpenCheckoutRevision("")
	case it.Act == treeActGitNewBranch:
		m.OpenNewGitBranch()
	case it.Act == treeActGitNewTag:
		m.OpenNewGitTag()
	case it.Act == treeActGitWorktrees:
		m.OpenVCSWorktrees()
	case it.Act == treeActGitNewWorktree:
		m.OpenNewVCSWorktree()
	case it.Act == treeActGitRemotes:
		m.OpenGitRemotes()
	case it.Act == treeActGitResolveConflicts:
		m.OpenResolveConflicts()
	case it.Act == treeActGitStash:
		m.OpenGitStash()
	case it.Act == treeActGitUnstash:
		m.OpenGitUnstash()
	case it.Act == treeActGitReset:
		m.OpenGitReset()
	case it.Act == treeActProjectDetach, it.Act == treeActWorkspaceRemove:
		m.RemoveWorkspaceProject(t.path)
	}
}

// treeClipState is what Copy or Cut last put on the project view's own
// clipboard, waiting for Paste.
type treeClipState struct {
	path  string
	paths []string
	cut   bool
}

// treeOpKind is the prompt the project view shows.
type treeOpKind int

const (
	treeOpNewFile treeOpKind = iota
	treeOpNewDir
	treeOpRename
	treeOpCopy
	treeOpDelete
)

// treeOpState is a New, Rename, Copy or Delete prompt of the project view.
type treeOpState struct {
	kind  treeOpKind
	path  string   // the folder to create in, or the first entry to rename, copy or delete
	paths []string // the top-level selection for a multi-item Delete
	dirs  []string // folders in a multi-item Delete
	dir   bool     // path is a folder
	name  string
	open  bool
	err   string
	busy  bool
	// safeDelete and safeSearchable are the Delete dialog's remembered
	// checkbox and whether the selection can be searched at all. safeRun owns
	// the in-flight usage search or its detected-usages choices.
	safeDelete     bool
	safeSearchable bool
	safeRun        *treeSafeDeleteRun
	// projectTemplate carries a file template from the chooser through the
	// name prompt. projectTarget and projectPackage are the live preview.
	projectTemplate     *projectNewTemplate
	projectIncludes     map[string]string
	projectKind         int
	projectTarget       string
	projectDir          string
	projectPackage      string
	projectPackageStyle string
}

// SetTreeClip remembers path for Paste, cut or copy.
func (m *Model) SetTreeClip(path string, cut bool) {
	if path == "" || path == m.Root() {
		return
	}
	m.SetTreeClipPaths([]string{path}, cut)
}

// SetTreeClipPaths remembers the project selection for Paste, cut or copy.
func (m *Model) SetTreeClipPaths(paths []string, cut bool) {
	paths = m.mutableTreePaths(paths)
	if len(paths) == 0 {
		return
	}
	m.treeClip = &treeClipState{
		path: paths[0], paths: append([]string(nil), paths...), cut: cut,
	}
	m.writeTreeClipboardFiles(paths)
	word := "Copied "
	if cut {
		word = "Cut "
	}
	if len(paths) == 1 {
		m.status = word + m.projectTreePathBase(paths[0])
	} else {
		m.status = fmt.Sprintf("%s %d items", strings.TrimSpace(word), len(paths))
	}
	m.invalidate()
}

// OpenNewTreeEntry asks for the name of a file or folder to create in dir.
func (m *Model) OpenNewTreeEntry(dir string, asDir bool) {
	if dir == "" {
		return
	}
	m.projectNew = nil
	kind := treeOpNewFile
	if asDir {
		kind = treeOpNewDir
	}
	m.treeOp = &treeOpState{kind: kind, path: dir, dir: true, open: true}
	m.invalidate()
}

// OpenTreeRename asks for path's new name, which keeps it in its folder.
func (m *Model) OpenTreeRename(path string) {
	if path == "" || path == m.Root() {
		return
	}
	if m.projectTreeUsesRemotePaths() {
		if !m.projectTreeEntryKnown(path) {
			m.status = "Rename failed: the remote entry is no longer available"
			m.invalidate()
			return
		}
	} else if _, err := os.Stat(path); err != nil {
		m.status = "Rename failed: " + err.Error()
		m.invalidate()
		return
	}
	m.treeOp = &treeOpState{
		kind: treeOpRename, path: path, dir: m.projectTreeIsDir(path),
		name: m.projectTreePathBase(path), open: true,
	}
	m.invalidate()
}

// OpenTreeCopy opens OpenIDEA's Copy File/Directory prompt with Finder-style
// non-conflicting text. A name typed there becomes the copy's name.
func (m *Model) OpenTreeCopy(path string) {
	if path == "" || path == m.Root() {
		return
	}
	if m.projectTreeUsesRemotePaths() {
		if !m.projectTreeEntryKnown(path) {
			m.status = "Copy failed: the remote entry is no longer available"
			m.invalidate()
			return
		}
	} else if _, err := os.Stat(path); err != nil {
		m.status = "Copy failed: " + err.Error()
		m.invalidate()
		return
	}
	paths := m.selectedMutableTreePaths()
	if len(paths) == 0 {
		paths = []string{path}
	}
	m.OpenFileCopyDialog(paths, m.projectTreePathDir(paths[0]))
}

// uniqueTreeCopyName is OpenIDEA's Finder-style copy name:
// Foo.kt, Foo copy.kt, Foo copy 2.kt.
func (m *Model) uniqueTreeCopyName(dir, name string, isDir bool) string {
	taken := map[string]bool{}
	for _, name := range m.projectTreeEntryNames(dir) {
		taken[m.projectTreeNameKey(name)] = true
	}
	return uniqueTreeNameCase(name, isDir, taken, m.projectTreeCaseInsensitive())
}

// AskTreeDelete opens the confirmation that drops path.
func (m *Model) AskTreeDelete(path string, dir bool) {
	if path == "" || path == m.Root() {
		return
	}
	m.AskTreeDeletePaths([]string{path})
}

// AskTreeDeletePaths opens OpenIDEA's one confirmation for a top-level
// project-view selection.
func (m *Model) AskTreeDeletePaths(paths []string) {
	paths = m.mutableTreePaths(paths)
	if len(paths) == 0 {
		return
	}
	dirs := make([]string, 0, len(paths))
	files := make([]string, 0, len(paths))
	for _, path := range paths {
		if m.projectTreeIsDir(path) {
			dirs = append(dirs, path)
		} else if m.projectTreeEntryKnown(path) {
			files = append(files, path)
		}
	}
	dir := len(paths) == 1 && len(dirs) == 1
	m.treeOp = &treeOpState{
		kind: treeOpDelete, path: paths[0],
		paths: append([]string(nil), paths...), dirs: dirs,
		dir: dir, open: true,
		safeDelete:     m.prefs.ProjectTree.searchesUsagesOnDelete(),
		safeSearchable: len(files) > 0 && len(files) <= maxSafeDeleteFiles,
	}
	m.invalidate()
}

// CloseTreeOp cancels the prompt.
func (m *Model) CloseTreeOp() {
	if m.treeOp != nil {
		m.cancelTreeSafeDelete(m.treeOp)
	}
	m.projectNew = nil
	m.treeOp = nil
	m.invalidate()
}

// treeOpName is the trimmed name the prompt typed, with the one path
// separator a name cannot hold reported before any disk write.
func treeOpName(v *treeOpState) (string, error) {
	name := strings.TrimSpace(v.name)
	switch {
	case name == "":
		return "", errors.New("The name cannot be empty")
	case name == "." || name == "..":
		return "", errors.New("The name is not valid")
	case strings.ContainsAny(name, `/\`):
		return "", errors.New("The name cannot contain a path separator")
	}
	return name, nil
}

// ConfirmTreeOp applies the prompt.
func (m *Model) ConfirmTreeOp() {
	v := m.treeOp
	if v == nil || v.busy || m.Root() == "" {
		return
	}
	if v.projectTemplate != nil {
		m.confirmProjectNewFile(v)
		return
	}
	if v.kind == treeOpDelete {
		m.deleteTreeEntry(v)
		return
	}
	name, err := treeOpName(v)
	if err != nil {
		v.err = err.Error()
		m.invalidate()
		return
	}
	from, to := v.path, ""
	command := "fs_create_file"
	args := map[string]any{"path": m.projectTreePathJoin(v.path, name)}
	switch v.kind {
	case treeOpNewDir:
		command = "fs_create_dir"
	case treeOpRename:
		from = v.path
		to = m.projectTreePathJoin(m.projectTreePathDir(v.path), name)
		command = "fs_rename"
		args = map[string]any{"from": from, "to": to}
		if to == from {
			m.CloseTreeOp()
			return
		}
	case treeOpCopy:
		from = v.path
		to = m.projectTreePathJoin(m.projectTreePathDir(v.path), name)
		command = "fs_copy"
		args = map[string]any{"from": from, "to": to}
	}
	target, _ := args["path"].(string)
	if target == "" {
		target = to
	}
	if m.projectTreeEntryKnown(target) {
		v.err = "A file or directory with that name already exists"
		m.invalidate()
		return
	}
	if v.kind == treeOpRename {
		if bad := m.treeDirtyUnder(from); bad != "" {
			v.err = "Save or close it in the editor first: " + bad
			m.invalidate()
			return
		}
	}
	v.busy = true
	v.err = ""
	root := m.Root()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, command, args)
		m.post(func() {
			v.busy = false
			if m.Root() != root {
				return
			}
			if err != nil {
				v.err = "The operation failed: " + err.Error()
				m.invalidate()
				return
			}
			m.treeOp = nil
			switch v.kind {
			case treeOpNewFile:
				m.status = "Created " + name
				m.refreshTree(m.projectTreePathDir(target))
				m.notifyVCSFileOperation("created", []string{target})
				m.openFile(target)
			case treeOpNewDir:
				m.status = "Created directory " + name
				m.refreshTree(m.projectTreePathDir(target))
			case treeOpRename:
				m.closedTreeTabs(from)
				m.moveBookmarkPaths(from, to)
				m.refreshTree(m.projectTreePathDir(to))
				m.status = "Renamed to " + name
				if m.tree != nil {
					m.tree.Select(to)
				}
			case treeOpCopy:
				m.refreshTree(m.projectTreePathDir(to))
				m.status = "Copied " + m.projectTreePathBase(from) + " to " + name
				m.notifyVCSFileOperation("created", []string{to})
				if m.tree != nil {
					m.tree.Select(to)
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) confirmProjectNewFile(v *treeOpState) {
	plan, err := projectNewPlanFile(v.projectTemplate, v.projectKind, v.path, v.name)
	if err != nil {
		v.err = err.Error()
		m.invalidate()
		return
	}
	if m.projectTreeEntryKnown(plan.path) {
		v.err = "A file or directory with that name already exists"
		m.invalidate()
		return
	}
	root := m.Root()
	projectName := m.projectName
	if strings.TrimSpace(projectName) == "" {
		projectName = filepath.Base(root)
	}
	style, pkg := projectNewPackage(v.projectTemplate, v.projectKind, plan.directory, root)
	text, caret := projectNewRenderedTextWithIncludes(
		v.projectTemplate, v.projectKind, plan, pkg, projectName, root, time.Now(), v.projectIncludes,
	)
	line, col := projectNewCaretPosition(text, caret)
	v.projectTarget = plan.path
	v.projectDir = plan.directory
	v.projectPackageStyle = style
	v.projectPackage = pkg
	v.busy = true
	v.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "fs_create_file_bytes", map[string]any{
			"path": plan.path,
			"data": projectNewByteValues(text),
		})
		m.post(func() {
			v.busy = false
			if m.Root() != root {
				return
			}
			if err != nil {
				v.err = "The operation failed: " + err.Error()
				m.invalidate()
				return
			}
			m.treeOp = nil
			m.status = "Created " + plan.fileName
			m.refreshTree(plan.directory)
			m.notifyVCSFileOperation("created", []string{plan.path})
			m.openFileThen(plan.path, func(tab *Tab) {
				tab.state.GoTo(editor.Pos{Line: line, Col: col})
			})
			m.invalidate()
		})
	}()
}

// treeSelectionAfterDelete keeps the next surviving visible row, else the
// previous one, as OpenIDEA's Project view does after Delete.
func treeSelectionAfterDelete(rows []*treeNode, deleted []string) string {
	return treeSelectionAfterDeleteWith(rows, deleted, underTreePath)
}

func (m *Model) treeSelectionAfterDelete(rows []*treeNode, deleted []string) string {
	return treeSelectionAfterDeleteWith(rows, deleted, m.projectTreeUnderPath)
}

func treeSelectionAfterDeleteWith(
	rows []*treeNode,
	deleted []string,
	under func(string, string) bool,
) string {
	gone := func(path string) bool {
		for _, removed := range deleted {
			if under(path, removed) {
				return true
			}
		}
		return false
	}
	last := -1
	for index, row := range rows {
		for _, removed := range deleted {
			if row.path == removed {
				last = index
			}
		}
	}
	if last < 0 {
		return ""
	}
	for index := last + 1; index < len(rows); index++ {
		if !rows[index].member && !gone(rows[index].path) {
			return rows[index].path
		}
	}
	for index := last - 1; index >= 0; index-- {
		if !rows[index].member && !gone(rows[index].path) {
			return rows[index].path
		}
	}
	return ""
}

// deleteTreeEntry moves the selected top-level entries to the Trash and keeps
// the snapshots Local History needs for one-step Undo.
func (m *Model) deleteTreeEntry(v *treeOpState) {
	if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		v.err = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	if v.safeSearchable {
		m.prefs.ProjectTree.SafeDelete = boolPointer(v.safeDelete)
		m.savePreferences()
	}
	if v.safeDelete && v.safeSearchable {
		m.startTreeSafeDelete(v)
		return
	}
	m.performTreeDelete(v)
}

// performTreeDelete executes a confirmed Delete after the optional usage
// search. It keeps OpenIDEA's partial-success behavior and one undo record.
func (m *Model) performTreeDelete(v *treeOpState) {
	paths := v.paths
	if len(paths) == 0 {
		paths = []string{v.path}
	}
	for _, path := range paths {
		if bad := m.treeDirtyUnder(path); bad != "" {
			v.err = "Save or close it in the editor first: " + bad
			m.invalidate()
			return
		}
	}
	rows := append([]*treeNode(nil), m.tree.Rows()...)
	next := m.treeSelectionAfterDelete(rows, paths)
	dirs := append([]string(nil), v.dirs...)
	v.busy = true
	v.err = ""
	root := m.Root()
	m.status = "Deleting…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()
		var deleted []string
		var failures []string
		for _, path := range paths {
			_, err := m.invoke(ctx, "fs_delete", map[string]any{"path": path})
			if err != nil {
				failures = append(failures, m.projectTreePathBase(path)+": "+err.Error())
				continue
			}
			deleted = append(deleted, path)
		}
		m.post(func() {
			v.busy = false
			if m.Root() != root {
				return
			}
			m.treeOp = nil
			for _, path := range deleted {
				m.closedTreeTabs(path)
				m.removeNavigationTree(path)
				m.dropBookmarksUnder(path)
				m.refreshTree(m.projectTreePathDir(path))
			}
			if len(deleted) > 0 {
				m.treeLastMove = &treeMoveRecord{
					Kind: "delete", Seq: treeFileOperationSequence(),
					DeletePaths: append([]string(nil), deleted...),
					DeleteDirs:  append([]string(nil), dirs...),
				}
				m.notifyVCSFileOperation("deleted", deleted)
			}
			if m.tree != nil {
				if next != "" && !m.underAnyTreePath(next, deleted) {
					m.tree.Select(next)
				} else if len(deleted) > 0 {
					m.tree.Select("")
				}
			}
			if len(deleted) == 1 {
				m.status = "Deleted " + m.projectTreePathBase(deleted[0])
			} else if len(deleted) > 1 {
				m.status = fmt.Sprintf("Deleted %d items", len(deleted))
			} else {
				m.status = ""
			}
			if len(failures) > 0 {
				m.notifyError("project.delete", "Some items could not be deleted",
					strings.Join(failures, "\n"))
			}
			m.invalidate()
		})
	}()
}

func underAnyTreePath(path string, roots []string) bool {
	return underAnyTreePathWith(path, roots, underTreePath)
}

func (m *Model) underAnyTreePath(path string, roots []string) bool {
	return underAnyTreePathWith(path, roots, m.projectTreeUnderPath)
}

func underAnyTreePathWith(
	path string,
	roots []string,
	under func(string, string) bool,
) bool {
	for _, root := range roots {
		if under(path, root) {
			return true
		}
	}
	return false
}

// PasteTreeEntry opens OpenIDEA's Copy or Move dialog for the internal
// project-view clipboard. Files copied in Finder paste immediately as copies.
func (m *Model) PasteTreeEntry(dir string) {
	clip := m.treeClip
	if dir == "" || m.Root() == "" {
		return
	}
	external, clipboardErr := m.readTreeClipboardFiles()
	var internalPaths []string
	if clip != nil {
		internalPaths = clip.paths
		if len(internalPaths) == 0 && clip.path != "" {
			internalPaths = []string{clip.path}
		}
	}
	if len(external) > 0 && !m.sameTreeClipboardFiles(external, internalPaths) {
		m.pasteExternalTreeFiles(external, dir)
		return
	}
	if clip == nil || len(internalPaths) == 0 {
		if clipboardErr != nil {
			m.notifyError("project.paste", "Cannot access the clipboard", clipboardErr.Error())
		} else {
			m.notifyInfo("project.paste", "No files on the clipboard", "")
		}
		m.status = ""
		m.invalidate()
		return
	}
	if !clip.cut {
		m.OpenFileCopyDialog(internalPaths, dir)
		return
	}
	paths := make([]string, 0, len(internalPaths))
	for _, path := range internalPaths {
		if m.projectTreePathDir(path) != m.projectTreePathClean(dir) {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		m.status = "It is already in that directory"
		m.invalidate()
		return
	}
	m.OpenFileMoveDialogFor(paths, dir, true)
}

func (m *Model) pasteExternalTreeFiles(paths []string, dir string) {
	if m.projectTreeUsesRemotePaths() {
		m.importLocalTreeFiles(paths, dir)
		return
	}
	paths = m.mutableTreePaths(paths)
	if len(paths) == 0 {
		m.notifyInfo("project.paste", "No files on the clipboard", "")
		return
	}
	d := &fileRefactorDialogState{
		kind: "copy", path: paths[0], paths: paths, target: m.displayRefactorTarget(dir),
	}
	if len(paths) == 1 {
		d.dir = m.projectTreeIsDir(paths[0])
		d.name = m.uniqueTreeCopyName(dir, m.projectTreePathBase(paths[0]), d.dir)
	}
	copies, err := m.copyDialogPlan(d)
	if err != nil {
		m.notifyError("project.paste", "Some items could not be copied", err.Error())
		return
	}
	m.runTreeFileCopies(copies, dir, false, nil)
}

// CopyTreePath puts the absolute path, the file name or the path from the
// project root on the system clipboard.
func (m *Model) CopyTreePath(c *ui.Context, path, mode string) {
	if c == nil || path == "" {
		return
	}
	text := path
	switch mode {
	case "File Name":
		text = m.projectTreePathBase(path)
	case "Path From Content Root":
		if rel, err := m.projectTreePathRelative(m.Root(), path); err == nil {
			text = rel
		}
	}
	c.WriteClipboard(text)
	m.status = "Copied " + text
	m.invalidate()
}

// refreshTree rereads the project view and opens dir so a new entry shows.
func (m *Model) refreshTree(dir string) {
	if m.tree == nil {
		return
	}
	m.tree.Refresh()
	m.tree.reveal(dir)
}

// treeTabsUnder lists the open tabs of path or of a file inside it.
func (m *Model) treeTabsUnder(path string) []*Tab {
	var out []*Tab
	for _, t := range m.tabs {
		if m.projectTreeUnderPath(t.doc.Path(), path) {
			out = append(out, t)
		}
	}
	return out
}

// treeDirtyUnder names the first unsaved tab of path, "" when none.
func (m *Model) treeDirtyUnder(path string) string {
	for _, t := range m.treeTabsUnder(path) {
		if t.unsaved() {
			return m.projectTreePathBase(t.doc.Path())
		}
	}
	return ""
}

// closedTreeTabs closes the open tabs of path after it moved or went away.
// The caller has already refused when one of them was unsaved.
func (m *Model) closedTreeTabs(path string) {
	for _, t := range m.treeTabsUnder(path) {
		m.removeTab(t)
	}
}

// underTreePath reports whether p is path or inside it.
func underTreePath(p, path string) bool {
	if caseInsensitiveFileSystem() {
		p, path = strings.ToLower(p), strings.ToLower(path)
	}
	separator := string(filepath.Separator)
	if strings.HasPrefix(p, "/") || strings.HasPrefix(path, "/") {
		separator = "/"
	}
	return p == path || strings.HasPrefix(p, path+separator)
}

// moveBookmarkPaths follows a rename, so bookmarks of the old path still
// reach the entry.
func (m *Model) moveBookmarkPaths(from, to string) {
	if from == to {
		return
	}
	follow := func(p string) string {
		if m.projectTreeSamePath(p, from) {
			return to
		}
		if m.projectTreeUnderPath(p, from) {
			return to + p[len(from):]
		}
		return p
	}
	changed := false
	for i := range m.bookmarks {
		if next := follow(m.bookmarks[i].Path); next != m.bookmarks[i].Path {
			m.bookmarks[i].Path = next
			changed = true
		}
	}
	for i := range m.pathBookmarks {
		if next := follow(m.pathBookmarks[i].Path); next != m.pathBookmarks[i].Path {
			m.pathBookmarks[i].Path = next
			changed = true
		}
	}
	if changed {
		m.saveSession()
		m.refreshDecorations()
	}
}

// dropBookmarksUnder forgets the bookmarks of a deleted entry.
func (m *Model) dropBookmarksUnder(path string) {
	marks := m.bookmarks[:0]
	for _, b := range m.bookmarks {
		if !m.projectTreeUnderPath(b.Path, path) {
			marks = append(marks, b)
		}
	}
	paths := m.pathBookmarks[:0]
	for _, pb := range m.pathBookmarks {
		if !m.projectTreeUnderPath(pb.Path, path) {
			paths = append(paths, pb)
		}
	}
	m.bookmarks, m.pathBookmarks = marks, paths
	m.saveSession()
	m.refreshDecorations()
}

// SmokeFiles drives the project view's New, Rename, Copy, Cut and Delete
// against the real backend, on a folder it removes at the end.
func (m *Model) SmokeFiles(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("No project is open")
	}
	base := filepath.Join(root, "goidea-tree-smoke")
	_ = os.RemoveAll(base)
	out := map[string]any{
		"menu": treeMenuLabels(treeMenuItems(treeTarget{path: root, dir: true, root: true}, false)),
	}
	run := func(fn func()) {
		done := make(chan struct{})
		m.post(func() { defer close(done); fn() })
		<-done
	}
	wait := func(path string, want bool, what string) error {
		deadline := time.Now().Add(30 * time.Second)
		for {
			_, err := os.Stat(path)
			if (err == nil) == want {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("%s timed out", what)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	create := func(dir, name string, asDir bool) error {
		run(func() {
			m.OpenNewTreeEntry(dir, asDir)
			if m.treeOp != nil {
				m.treeOp.name = name
			}
			m.ConfirmTreeOp()
		})
		return wait(filepath.Join(dir, name), true, "new "+name)
	}
	rename := func(path, name string) error {
		run(func() {
			m.OpenTreeRename(path)
			if m.treeOp != nil {
				m.treeOp.name = name
			}
			m.ConfirmTreeOp()
		})
		if err := wait(path, false, "the name before renaming "+name); err != nil {
			return err
		}
		return wait(filepath.Join(filepath.Dir(path), name), true, "renaming "+name)
	}
	paste := func(from, dir string, cut bool) error {
		run(func() {
			m.SetTreeClip(from, cut)
			m.PasteTreeEntry(dir)
			// OpenIDEA's Paste opens the Copy or Move dialog rather than
			// writing immediately. This smoke covers the filesystem move; the
			// reference-aware path has its own backend regression test.
			if m.fileRefactor != nil {
				if cut {
					m.fileRefactor.references = false
					m.fileRefactor.textOccurrences = false
				}
				m.ConfirmFileRefactor()
			}
		})
		return wait(filepath.Join(dir, filepath.Base(from)), true, "pasting "+filepath.Base(from))
	}
	drop := func(path string, dir bool) error {
		run(func() {
			m.AskTreeDelete(path, dir)
			if m.treeOp != nil {
				m.treeOp.safeDelete = false
			}
			m.ConfirmTreeOp()
		})
		return wait(path, false, "deleting "+filepath.Base(path))
	}
	rows := func() int {
		n := 0
		run(func() {
			if m.tree != nil {
				n = len(m.tree.Rows())
			}
		})
		return n
	}
	waitRows := func(want int) (int, error) {
		deadline := time.Now().Add(30 * time.Second)
		for {
			n := rows()
			if n >= want {
				return n, nil
			}
			if time.Now().After(deadline) {
				return n, errors.New("Expand All timed out")
			}
			select {
			case <-ctx.Done():
				return n, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
	}

	if err := create(root, filepath.Base(base), true); err != nil {
		return out, err
	}
	out["newDir"] = true
	one := filepath.Join(base, "one.txt")
	if err := create(base, "one.txt", false); err != nil {
		return out, err
	}
	out["newFile"] = true
	if err := create(base, "nested", true); err != nil {
		return out, err
	}
	// The new file's tab arrives after its read, and Select in Project View
	// acts on the active tab.
	deadline := time.Now().Add(30 * time.Second)
	for {
		open := false
		run(func() {
			for _, t := range m.tabs {
				if t.doc.Path() == realPath(one) {
					open = true
				}
			}
		})
		if open {
			break
		}
		if time.Now().After(deadline) {
			return out, errors.New("Opening one.txt timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	run(func() {
		m.SelectInProjectView()
		if m.tree != nil {
			out["selected"] = filepath.Base(m.tree.Selected())
		}
	})
	run(func() { m.CollapseProjectAll() })
	out["collapseRows"] = rows()
	run(func() { m.ExpandProjectAll() })
	expanded, err := waitRows(4)
	if err != nil {
		return out, err
	}
	out["expandRows"] = expanded
	run(func() { m.ReloadProjectTree() })
	out["reloadRows"] = rows()
	// Reload from Disk rereads the selected folder, so a file written behind
	// the tree's back shows after it.
	late := filepath.Join(base, "reloaded.txt")
	if err := os.WriteFile(late, []byte("late\n"), 0o644); err != nil {
		return out, err
	}
	run(func() {
		m.tree.Select(base)
		m.ReloadSelectedFromDisk()
	})
	seen := false
	reloadDeadline := time.Now().Add(30 * time.Second)
	for {
		run(func() {
			seen = false
			if m.tree == nil {
				return
			}
			for _, r := range m.tree.Rows() {
				if filepath.Base(r.path) == "reloaded.txt" {
					seen = true
				}
			}
		})
		if seen {
			break
		}
		if time.Now().After(reloadDeadline) {
			return out, errors.New("Reload from disk timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["reloadSeen"] = seen
	run(func() { out["reloadStatus"] = m.status })
	if err := os.Remove(late); err != nil {
		return out, err
	}
	// Find in Folder searches only the chosen folder: the fixture's hits come
	// from src/main/java, and the smoke folder's empty files give none.
	searchIn := func(dir, query string) (int, []string, error) {
		var rels []string
		run(func() {
			m.ShowSearchInFolder(dir)
			m.search.query = query
			m.RunSearch()
		})
		deadline := time.Now().Add(90 * time.Second)
		for {
			var done, running bool
			m.post(func() {
				done, running = m.search.done, m.search.running
				rels = rels[:0]
				for _, f := range m.search.files {
					rels = append(rels, f.RelPath)
				}
			})
			if done || (!running && len(rels) == 0) {
				return len(rels), rels, nil
			}
			if time.Now().After(deadline) {
				return len(rels), rels, errors.New("Find in Folder timed out")
			}
			select {
			case <-ctx.Done():
				return len(rels), rels, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	folderFiles, rels, err := searchIn(filepath.Join(root, "src", "main", "java"), "class ")
	if err != nil {
		return out, err
	}
	under := 0
	for _, rel := range rels {
		if strings.HasPrefix(rel, "src/main/java/") {
			under++
		}
	}
	out["folderFiles"] = folderFiles
	out["folderUnder"] = under
	emptyFiles, _, err := searchIn(base, "class ")
	if err != nil {
		return out, err
	}
	out["folderEmpty"] = emptyFiles
	// Local History records a snapshot on each save; the marker write is the
	// second revision, so restoring the oldest snapshot must remove it.
	hist := filepath.Join(base, "hist.txt")
	if err := create(base, "hist.txt", false); err != nil {
		return out, err
	}
	var histTab *Tab
	tabDeadline := time.Now().Add(30 * time.Second)
	for histTab == nil {
		run(func() {
			for _, t := range m.tabs {
				if t.doc.Path() == realPath(hist) {
					histTab = t
				}
			}
		})
		if histTab != nil {
			break
		}
		if time.Now().After(tabDeadline) {
			return out, errors.New("Opening hist.txt timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	save := func(text string) error {
		run(func() {
			for i, t := range m.tabs {
				if t == histTab {
					m.active = i
				}
			}
			histTab.doc.Buffer().Edit(editor.Edit{Start: 0, End: 0, Text: text})
			m.invalidate()
		})
		run(func() { m.SaveActive() })
		deadline := time.Now().Add(30 * time.Second)
		for {
			data, readErr := os.ReadFile(hist)
			var saving bool
			run(func() { saving = histTab.saving })
			if readErr == nil && !saving && strings.Contains(string(data), strings.TrimSpace(text)) {
				return nil
			}
			if time.Now().After(deadline) {
				return errors.New("Saving hist.txt timed out")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	if err := save("history base\n"); err != nil {
		return out, err
	}
	if err := save("history-marker\n"); err != nil {
		return out, err
	}
	run(func() { m.ShowHistory(hist) })
	var entries []historyEntry
	histDeadline := time.Now().Add(60 * time.Second)
	for {
		var loading bool
		var histErr string
		run(func() {
			h := m.history
			if h == nil {
				loading = true
				return
			}
			loading = h.loading
			histErr = h.err
			entries = append(entries[:0], h.entries...)
		})
		if !loading {
			if histErr != "" {
				return out, errors.New(histErr)
			}
			break
		}
		if time.Now().After(histDeadline) {
			return out, errors.New("Reading hist.txt's history timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["historyEntries"] = len(entries)
	oldest := -1
	for i, e := range entries {
		if e.Kind == "label" || e.ID == "" {
			continue
		}
		if oldest < 0 || e.Ms < entries[oldest].Ms {
			oldest = i
		}
	}
	if oldest < 0 {
		return out, errors.New("hist.txt has no restorable history")
	}
	run(func() {
		m.history.sel = oldest
		m.RestoreHistoryEntry()
	})
	run(func() { m.RestoreHistoryEntry() })
	restored := false
	restoreDeadline := time.Now().Add(60 * time.Second)
	for {
		data, readErr := os.ReadFile(hist)
		if readErr == nil && !strings.Contains(string(data), "history-marker") {
			restored = true
			break
		}
		if time.Now().After(restoreDeadline) {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["historyRestored"] = restored
	run(func() {
		m.OpenHistoryLabel()
		if m.history != nil {
			m.history.label = "e2e label"
		}
		m.ConfirmHistoryLabel()
	})
	labeled := false
	deadline = time.Now().Add(30 * time.Second)
	for {
		run(func() { labeled = m.history == nil && strings.Contains(m.status, "Put label e2e label") })
		if labeled {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["historyLabeled"] = labeled
	listed := false
	run(func() { m.ShowHistory(hist) })
	deadline = time.Now().Add(60 * time.Second)
	for {
		var loading bool
		run(func() {
			h := m.history
			if h == nil {
				loading = true
				return
			}
			loading = h.loading
			for _, e := range h.entries {
				if e.Kind == "label" && e.Label == "e2e label" {
					listed = true
				}
			}
		})
		if !loading || listed {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["historyLabelListed"] = listed
	run(func() {
		m.closeHistory()
		for i, t := range m.tabs {
			if t == histTab {
				m.removeTabNow(m.tabs[i])
				break
			}
		}
	})
	// Compare Directories hashes both folders in the backend: one file differs,
	// one exists on each side alone and one is identical.
	left := filepath.Join(base, "compare-a")
	right := filepath.Join(base, "compare-b")
	if err := os.MkdirAll(filepath.Join(left, "sub"), 0o755); err != nil {
		return out, err
	}
	if err := os.MkdirAll(right, 0o755); err != nil {
		return out, err
	}
	files := map[string]string{
		filepath.Join(left, "a.txt"):      "one\n",
		filepath.Join(right, "a.txt"):     "two\n",
		filepath.Join(left, "both.txt"):   "same\n",
		filepath.Join(right, "both.txt"):  "same\n",
		filepath.Join(left, "onlya.txt"):  "only left\n",
		filepath.Join(right, "onlyb.txt"): "only right\n",
	}
	for path, text := range files {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return out, err
		}
	}
	run(func() {
		m.OpenDirCompare(left)
		m.dirCompare.other = right
		m.ConfirmDirCompare()
	})
	var compareErr string
	var cmpEntries []dirCompareEntry
	deadline = time.Now().Add(60 * time.Second)
	for {
		var loading bool
		run(func() {
			if d := m.dirCompare; d != nil {
				loading, cmpEntries, compareErr = d.loading, d.entries, d.err
			}
		})
		if compareErr != "" || !loading {
			break
		}
		if time.Now().After(deadline) {
			return out, errors.New("Compare Directories timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if compareErr != "" {
		return out, errors.New(compareErr)
	}
	counts := dirCompareCounts(cmpEntries)
	out["compareDiffer"] = counts["different"]
	out["compareLeft"] = counts["leftOnly"]
	out["compareRight"] = counts["rightOnly"]
	out["compareEqual"] = counts["equal"]
	run(func() { m.closeDirCompare() })
	var diffAdded, diffGone int
	run(func() {
		m.ShowFileDiff(filepath.Join(left, "a.txt"), filepath.Join(right, "a.txt"))
	})
	deadline = time.Now().Add(30 * time.Second)
	for {
		var loading bool
		run(func() {
			if d := m.diff; d != nil {
				loading, diffAdded, diffGone = d.loading, d.added, d.gone
			}
		})
		if !loading {
			break
		}
		if time.Now().After(deadline) {
			return out, errors.New("Comparing the two files timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["compareDiffAdded"] = diffAdded
	out["compareDiffGone"] = diffGone
	run(func() { m.closeDiff() })
	if err := os.RemoveAll(left); err != nil {
		return out, err
	}
	if err := os.RemoveAll(right); err != nil {
		return out, err
	}
	two := filepath.Join(base, "two.txt")
	if err := rename(one, "two.txt"); err != nil {
		return out, err
	}
	out["renamed"] = true
	if err := paste(two, root, false); err != nil {
		return out, err
	}
	out["copied"] = true
	if err := drop(filepath.Join(root, "two.txt"), false); err != nil {
		return out, err
	}
	out["deletedFile"] = true
	if err := paste(two, root, true); err != nil {
		return out, err
	}
	out["moved"] = true
	if err := paste(filepath.Join(root, "two.txt"), base, true); err != nil {
		return out, err
	}
	if err := drop(two, false); err != nil {
		return out, err
	}
	if err := drop(base, true); err != nil {
		return out, err
	}
	out["deletedDir"] = true
	return out, nil
}

// ExpandProjectAll opens every folder in the project view, as OpenIDEA's
// Expand All (Cmd+=) does.
func (m *Model) ExpandProjectAll() {
	if m.tree == nil || m.Root() == "" {
		return
	}
	switch m.tree.Mode() {
	case "scratches":
		if m.tree.scratchRoot != "" {
			m.tree.ExpandAll(m.tree.scratchRoot)
		}
	case "openFiles", "changedFiles":
		for path := range m.tree.virtualDirs {
			m.tree.open[path] = true
			delete(m.tree.collapsed, path)
		}
		m.tree.rebuild()
		m.tree.expansionChanged()
	default:
		m.tree.ExpandAll(m.Root())
		if m.tree.Mode() == "project" && m.tree.scratchRoot != "" {
			m.tree.open[m.tree.scratchRoot] = true
		}
	}
	m.status = "Expanded All"
	m.invalidate()
}

// CollapseProjectAll closes every folder but the root, as OpenIDEA's Collapse
// All (Cmd+-) does.
func (m *Model) CollapseProjectAll() {
	if m.tree == nil {
		return
	}
	m.tree.CollapseAll()
	m.status = "Collapsed All"
	m.invalidate()
}

// ReloadProjectTree rereads every folder the project view has read, as
// OpenIDEA's Reload Project Tree from Disk does.
func (m *Model) ReloadProjectTree() {
	if m.tree == nil {
		return
	}
	m.tree.Refresh()
	m.status = "Reloaded from Disk"
	m.invalidate()
}

// ReloadPathFromDisk rereads path's folder, as OpenIDEA's Reload from Disk
// does for a selected folder or file.
func (m *Model) ReloadPathFromDisk(path string) {
	if m.tree == nil || path == "" {
		return
	}
	dir := path
	if !m.tree.IsDir(path) {
		dir = m.projectTreePathDir(path)
	}
	m.tree.RefreshDir(dir)
	m.status = "Reloaded from Disk"
	m.invalidate()
}

// ReloadSelectedFromDisk reloads the project view's selection, or the root
// when nothing is selected.
func (m *Model) ReloadSelectedFromDisk() {
	if m.tree == nil {
		return
	}
	path := m.tree.Selected()
	if path == "" {
		path = m.Root()
	}
	m.ReloadPathFromDisk(path)
}

// SelectInProjectView reveals the active file in the project view and marks
// it, as OpenIDEA's Select in Project View does.
func (m *Model) SelectInProjectView() {
	if m.tree == nil || m.Root() == "" {
		return
	}
	var t *Tab
	if m.active >= 0 && m.active < len(m.tabs) {
		t = m.tabs[m.active]
	}
	if t == nil || t.doc.Path() == "" {
		m.status = "Open a file first"
		m.invalidate()
		return
	}
	m.selectInProjectView(t.doc.Path())
}

// selectInProjectView reveals path in the project view and marks it.
func (m *Model) selectInProjectView(path string) {
	if m.tree == nil || m.Root() == "" || path == "" {
		return
	}
	m.sideMode = ""
	m.leftHidden = false
	m.tree.reveal(path)
	m.tree.Select(path)
	m.status = "Selected in the Project view: " + m.projectTreePathBase(path)
	m.rememberLayout()
	m.invalidate()
}

// treeOpDialog draws the project view's New, Rename or Delete prompt.
func (m *Model) treeOpDialog(c *ui.Context, p *ui.Theme) {
	v := m.treeOp
	if v == nil {
		return
	}
	if v.kind == treeOpDelete && v.safeRun != nil {
		m.treeSafeDeleteDialog(c, p, v)
		return
	}
	open := v.open
	title, hint, action := "New File", "The file is created in the Project view and opened here.", "Create"
	if v.projectTemplate != nil {
		title = "New " + v.projectTemplate.name
		hint = "Type a name; folders use / and dotted JVM packages use ."
		if len(v.projectTemplate.kinds) > 1 {
			hint += " Up/Down chooses kind."
		}
	}
	switch v.kind {
	case treeOpNewDir:
		title, hint, action = "New Directory", "The directory is created in the Project view.", "Create"
	case treeOpRename:
		title, hint, action = "Rename", "The new name replaces the old one; the entry stays in the same directory.", "Rename"
	case treeOpCopy:
		title, hint, action = "Copy", "The copy is created beside the original.", "Copy"
	case treeOpDelete:
		title = "Delete"
	}
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if v.kind == treeOpDelete {
				paths := v.paths
				if len(paths) == 0 {
					paths = []string{v.path}
				}
				question := ""
				if len(paths) == 1 {
					if len(v.dirs) > 0 {
						question = fmt.Sprintf("Delete '%s' and its contents?",
							m.projectTreePathBase(paths[0]))
					} else {
						question = fmt.Sprintf("Delete '%s'?", m.projectTreePathBase(paths[0]))
					}
				} else if len(v.dirs) > 0 {
					question = fmt.Sprintf("Delete %d items (including folder contents)?",
						len(paths))
				} else {
					question = fmt.Sprintf("Delete %d items?", len(paths))
				}
				ui.Text(c, question).FontSize(12).TextColor(p.TextMuted)
				ui.Text(c, "Items are moved to the Trash.").
					FontSize(12).TextColor(p.TextMuted)
				if v.safeSearchable {
					ui.Checkbox(c, &v.safeDelete, "Safe delete (with usage search)").
						Disabled(v.busy)
				}
			} else {
				if v.projectTemplate != nil && len(v.projectTemplate.kinds) > 1 {
					m.treeOpProjectKindChoices(c, p, v)
				}
				in := ui.TextInput(c, &v.name).AutoFocus()
				in.OnChange(func() {
					if v.projectTemplate != nil && v.projectTemplate.fixedName != "" {
						v.name = v.projectTemplate.fixedName
					}
					v.err = ""
					m.refreshProjectNewTarget(v)
				})
				if v.projectTemplate != nil {
					in.OnShortcut(0, ui.KeyDown, func() { m.StepTreeOpProjectKind(1) })
					in.OnShortcut(0, ui.KeyUp, func() { m.StepTreeOpProjectKind(-1) })
				}
				if in.Submitted() {
					m.ConfirmTreeOp()
				}
				ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted).SingleLine()
				if v.projectTemplate != nil {
					if v.projectTarget != "" {
						ui.Text(c, "Target  "+projectNewRelative(m.Root(), v.projectTarget)).
							FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
					}
					if v.projectPackageStyle != "" {
						value := v.projectPackage
						if value == "" {
							value = "not detected"
						}
						ui.Text(c, v.projectPackageStyle+"  "+value).
							FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
					}
				}
			}
			if v.err != "" {
				ui.Text(c, v.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() { m.CloseTreeOp() })
				if v.kind == treeOpDelete {
					ui.Button(c, "Delete").Disabled(v.busy).OnClick(func() { m.ConfirmTreeOp() })
				} else {
					ui.Button(c, action).Disabled(v.busy).OnClick(func() { m.ConfirmTreeOp() })
				}
			})
		})
	})
	if d.Dismissed() || !open {
		m.CloseTreeOp()
	}
}

func (m *Model) treeOpProjectKindChoices(c *ui.Context, p *ui.Theme, v *treeOpState) {
	kinds := v.projectTemplate.kinds
	if len(kinds) == 0 {
		return
	}
	v.projectKind = max(0, min(v.projectKind, len(kinds)-1))
	ui.Text(c, "Kind").FontSize(11).TextColor(p.TextMuted).SingleLine()
	ui.Scroll(c).Height(float32(min(144, len(kinds)*24))).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			for i, kind := range kinds {
				rowIndex := i
				r := ui.Row(c.Key(fmt.Sprintf("projectNewKind:%d", i))).Height(22).Padding(0, 7).
					Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if i == v.projectKind {
					r.Background(p.SurfaceHover)
				}
				r.Children(func() {
					color := p.TextMuted
					if i == v.projectKind {
						color = p.Accent
					}
					ui.Text(c, kind.label).FontSize(12).TextColor(color).SingleLine()
				})
				if r.Clicked() {
					v.projectKind = rowIndex
					m.refreshProjectNewTarget(v)
					m.invalidate()
				}
			}
		})
	})
}
