package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// vcsRows draws the changes view, which the left island shows.
func (m *Model) vcsRows(c *ui.Context, p *ui.Theme) {
	if m.vcs.err != "" {
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, m.vcs.err).FontSize(12).TextColor(p.Warning)
		})
		return
	}
	if len(m.vcs.files) == 0 {
		hint := "No local changes"
		if !m.vcs.loaded && !m.vcs.loading {
			hint = "Press Refresh to read the Git status"
		}
		if m.vcs.loading {
			hint = "Reading Git status…"
		}
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted)
		})
		return
	}
	if m.vcs.opErr != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, m.vcs.opErr).FontSize(12).TextColor(p.Danger).SingleLine().
				Ellipsis("…")
		})
	}
	// The staging groups OpenIDEA's changes view draws: a file staged and then
	// edited again is listed under both. The rows are inset like a tree's.
	staged, unstaged := vcsStaged(m.vcs.files), vcsUnstaged(m.vcs.files)
	ui.Column(c).Padding(0, 6).Children(func() {
		if len(staged) > 0 {
			m.vcsGroup(c, p, "Staged Changes", len(staged))
			for i, f := range staged {
				m.vcsRow(c, p, "staged", i, f)
			}
		}
		if len(unstaged) > 0 {
			m.vcsGroup(c, p, "Unstaged Changes", len(unstaged))
			for i, f := range unstaged {
				m.vcsRow(c, p, "unstaged", i, f)
			}
		}
	})
}

func (m *Model) vcsGroup(c *ui.Context, p *ui.Theme, label string, n int) {
	oi := oiPaletteOf(p)
	ui.Row(c).Height(24).PaddingStart(4).PaddingEnd(8).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(oi.textStrong).SingleLine()
		ui.Text(c, count(n, "file", "files")).FontSize(13).TextColor(oi.textSubtle).SingleLine()
	})
}

func (m *Model) vcsRow(c *ui.Context, p *ui.Theme, side string, i int, f openidea.FileStatus) {
	_, word, _ := vcsBadge(f)
	oi := oiPaletteOf(p)
	r := ui.Row(c.Key(fmt.Sprintf("vcs:%s:%d", side, i))).Height(24).
		PaddingStart(24).PaddingEnd(8).Gap(4).Radius(5).AlignItems(ui.Center)
	if r.Hovered() {
		r.Background(oi.hoverTint)
	}
	r.Tooltip(word)
	r.Children(func() {
		fileIconEl(c, oi, f.Path, 16, oi.textMuted)
		ui.Text(c, filepath.Base(f.RelPath)).FontSize(13).TextColor(vcsKindColor(p, f)).
			SingleLine().Ellipsis("…")
		ui.Spacer(c)
		// The row actions stay hidden until the row is under the pointer, as
		// OpenIDEA's tree rows carry no visible buttons.
		if r.Hovered() {
			if side == "staged" {
				if m.rowChip(c, p, "vcs:unstage:"+f.Path, "Unstage") {
					m.UnstageChange(f)
				}
			} else {
				if m.rowChip(c, p, "vcs:stage:"+f.Path, "Stage") {
					m.StageChange(f)
				}
				if m.rowChip(c, p, "vcs:rollback:"+f.Path, "Rollback") {
					m.AskRollback(f)
				}
			}
		}
	})
	if r.Clicked() {
		m.ShowDiff(f.Path)
	}
	// A double click takes the row further, to the file itself, as IDEA does
	// from its changes view.
	if r.DoubleClicked() {
		m.closeDiff()
		m.openFile(f.Path)
		m.invalidate()
	}
}

// rowChip is one small action on a list row; it reports a click.
func (m *Model) rowChip(c *ui.Context, p *ui.Theme, key, label string) bool {
	chip := ui.Row(c.Key(key)).Height(16).Padding(0, 5).AlignItems(ui.Center)
	if chip.Hovered() {
		chip.Background(p.SurfacePressed)
	}
	chip.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.TextMuted).SingleLine()
	})
	return chip.Clicked()
}

func (m *Model) searchRows(c *ui.Context, p *ui.Theme) {
	s := &m.search
	ui.Row(c).Padding(6, 8).Children(func() {
		in := ui.TextArea(c, &s.query).Grow(1).Lines(1, 6).
			Placeholder("Search text or pattern").AutoFocus()
		in.OnChange(m.searchQueryChanged)
		in.OnShortcut(ui.Cmd|ui.Shift, ui.KeyEnter, func() {
			m.AddNewLineToFindField()
		})
		in.OnShortcut(ui.Ctrl|ui.Shift, ui.KeyEnter, func() {
			m.AddNewLineToFindField()
		})
		in.OnTextKey(func(k ui.TextKey) bool {
			if k.Key != ui.KeyEnter {
				return false
			}
			if k.Mods == ui.Cmd|ui.Shift || k.Mods == ui.Ctrl|ui.Shift {
				k.SetText(k.Text() + "\n")
				m.searchQueryChanged()
				return true
			}
			if k.Mods&ui.Alt != 0 {
				return false
			}
			m.RunSearch()
			return true
		})
	})
	if s.replaceMode {
		ui.Row(c).Padding(0, 8).Children(func() {
			ui.TextInput(c, &s.replacement).Grow(1).
				Placeholder("Replace with")
		})
	}
	ui.Row(c).Padding(0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		if s.running {
			ui.Button(c, "Stop").OnClick(func() {
				m.cancelSearch()
				m.invalidate()
			})
		} else {
			ui.Button(c, "Search").OnClick(func() { m.RunSearch() })
		}
		if s.replaceMode {
			label := "Stop Replace"
			if !s.replaceRunning {
				label = "Replace All"
			}
			button := ui.Button(c, label).Disabled(!s.replaceRunning && len(s.files) == 0)
			button.OnClick(func() {
				if s.replaceRunning {
					m.CancelReplace()
				} else {
					m.ReplaceSearchAll()
				}
			})
			if len(s.files) > 0 {
				ui.Button(c, "Replace Selected").OnClick(m.ReplaceSearchSelected)
			}
		}
		ui.Button(c, "Options").OnClick(func() {
			s.options = !s.options
			m.invalidate()
		})
		ui.Button(c, "Recent Searches").OnClick(m.ToggleRecentSearchFields)
		scrollLabel := "Scroll to Results"
		if !m.FindScrollToResults() {
			scrollLabel = "Scrolling Off"
		}
		ui.Button(c, scrollLabel).OnClick(m.ToggleFindScrollToResults)
		ui.Spacer(c)
		text := s.summary
		if s.running {
			text = "Searching…"
		}
		if s.cancelled && text != "" {
			text = "Stopped · " + text
		}
		if text != "" {
			ui.Text(c, text).FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
	})
	if s.historyOpen {
		ui.Column(c).Padding(4, 28, 6, 8).Gap(2).Children(func() {
			ui.Text(c, "Recent searches").FontSize(11).TextColor(p.TextMuted).SingleLine()
			if len(s.history) == 0 {
				ui.Text(c, "No recent searches").FontSize(12).TextColor(p.TextMuted).SingleLine()
				return
			}
			for i, query := range s.history {
				i, query := i, query
				label := strings.ReplaceAll(strings.ReplaceAll(query, "\r", ""), "\n", " ↵ ")
				row := ui.Row(c.Key(fmt.Sprintf("find:recent:%d", i))).Height(24).
					Padding(0, 6).AlignItems(ui.Center)
				if row.Hovered() {
					row.Background(oiPaletteOf(p).hoverTint)
				}
				row.Children(func() {
					ui.Text(c, label).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
				})
				if row.Clicked() {
					m.UseRecentSearchField(i)
				}
			}
		})
	}
	ui.Row(c).Padding(0, 8).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Checkbox(c, &s.isRegex, "Regex")
		ui.Checkbox(c, &s.caseSensitive, "Match case")
		ui.Checkbox(c, &s.wholeWord, "Whole words")
		if s.replaceMode {
			ui.Checkbox(c, &s.preserveCase, "Preserve case")
		}
	})
	if s.options {
		ui.Row(c).Padding(0, 8).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Include").Width(52).FontSize(12).TextColor(p.TextMuted)
			ui.TextInput(c, &s.include).Grow(1).Placeholder("*.java, *.ts")
			ui.Text(c, "Exclude").Width(52).FontSize(12).TextColor(p.TextMuted)
			ui.TextInput(c, &s.exclude).Grow(1).Placeholder("build/*, node_modules/*")
		})
		choices := []string{"Project"}
		for _, scope := range m.searchScopes {
			choices = append(choices, scope.Name)
		}
		if s.scopeName == "" {
			s.scopeName = "Project"
		}
		found := false
		for _, choice := range choices {
			if choice == s.scopeName {
				found = true
				break
			}
		}
		if !found {
			s.scopeName = "Project"
		}
		ui.Row(c).Padding(0, 8).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Scope").Width(52).FontSize(12).TextColor(p.TextMuted)
			ui.Select(c, &s.scopeName, choices).Width(220)
			ui.Button(c, "Edit Scopes…").OnClick(m.OpenSearchScopes)
			if s.scopePath != "" {
				if rel, err := filepath.Rel(m.Root(), s.scopePath); err == nil {
					ui.Text(c, "Directory: "+filepath.ToSlash(rel)).FontSize(12).
						TextColor(p.TextMuted).SingleLine().Ellipsis("…")
				}
				ui.Button(c, "Clear Directory").OnClick(func() {
					s.scopePath = ""
					m.invalidate()
				})
			}
		})
	}
	if s.err != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, s.err).FontSize(12).TextColor(p.Warning)
		})
	}
	if s.replaceErr != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, s.replaceErr).FontSize(12).TextColor(p.Warning)
		})
	}
	if s.replaceProgress != "" || s.replaceSummary != "" {
		ui.Row(c).Padding(4, 8).Children(func() {
			text := s.replaceProgress
			if s.replaceSummary != "" {
				text = s.replaceSummary
			}
			ui.Text(c, text).FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		})
	}
	if s.truncated {
		ui.Row(c).Padding(4, 8).Children(func() {
			ui.Text(c, "Too many results; only the first ones are shown").FontSize(12).TextColor(p.TextMuted)
		})
	}
	if len(s.files) == 0 {
		hint := "Type a query and press Enter"
		switch {
		case s.running:
			hint = "Walking the project files…"
		case s.done:
			hint = "No matches"
		}
		ui.Row(c).Padding(8, 8).Children(func() {
			ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted)
		})
		return
	}
	shown := 0
	for fi, f := range s.files {
		f := f
		oi := oiPaletteOf(p)
		ui.Row(c).Height(24).PaddingStart(4).PaddingEnd(8).Gap(6).AlignItems(ui.Center).
			Children(func() {
				fileIconEl(c, oi, f.Path, 16, oi.textMuted)
				ui.Text(c, filepath.Base(f.RelPath)).FontSize(13).TextColor(oi.textStrong).
					SingleLine().Ellipsis("…")
				ui.Spacer(c)
				ui.Text(c, count(len(f.Matches), "match", "matches")).FontSize(12).
					TextColor(oi.textFaint)
			})
		for mi, mm := range f.Matches {
			if mi >= 40 || shown >= 200 {
				break
			}
			mi, mm := mi, mm
			shown++
			r := ui.Row(c.Key(fmt.Sprintf("hit:%d:%d", fi, mi))).Height(24).PaddingStart(26).
				PaddingEnd(8).Gap(6).AlignItems(ui.Center)
			if fi == s.selectedFile && mi == s.selectedMatch {
				r.Background(p.Selection)
				if s.scrollSelected {
					r.ScrollIntoView()
					s.scrollSelected = false
				}
			} else if r.Hovered() {
				r.Background(oi.hoverTint)
			}
			r.Children(func() {
				ui.Text(c, fmt.Sprintf("%d", mm.Line+1)).FontSize(12).Font("monospace").
					TextColor(oi.textFaint)
				code := strings.ReplaceAll(mm.Text, "\t", "    ")
				ui.Text(c, strings.TrimLeft(code, " ")).FontSize(13).Font("monospace").
					TextColor(oi.text).SingleLine().Ellipsis("…")
			})
			col := 0
			if len(mm.Ranges) > 0 {
				col = int(mm.Ranges[0][0])
			}
			if r.Clicked() {
				m.selectSearchHit(fi, mi)
				m.previewAt(f.Path, int(mm.Line), col)
			}
			if r.DoubleClicked() {
				m.revealAt(f.Path, int(mm.Line), col)
			}
		}
	}
}

func (m *Model) treeRow(c *ui.Context, p *ui.Theme, row *treeNode) {
	key := "tree:" + row.path
	if row.member {
		key = fmt.Sprintf("tree-member:%s:%d:%d:%s", row.path, row.line, row.col, row.name)
	}
	r := ui.Row(c.Key(key)).Height(24).
		PaddingStart(4 + float32(row.depth)*m.treeIndentWidth()).PaddingEnd(8).Gap(5).
		Radius(5).AlignItems(ui.Center)
	if r.FocusWithin() {
		m.projectTreeFocus = true
	}
	r.OnShortcut(ui.Cmd, ui.KeyC, m.CopySelectedTreeEntry)
	r.OnShortcut(ui.Ctrl, ui.KeyC, m.CopySelectedTreeEntry)
	r.OnShortcut(ui.Cmd, ui.KeyX, m.CutSelectedTreeEntry)
	r.OnShortcut(ui.Ctrl, ui.KeyX, m.CutSelectedTreeEntry)
	r.OnShortcut(ui.Cmd, ui.KeyV, m.PasteIntoTreeSelection)
	r.OnShortcut(ui.Ctrl, ui.KeyV, m.PasteIntoTreeSelection)
	r.OnShortcut(ui.Cmd, ui.KeyA, m.tree.SelectAll)
	r.OnShortcut(ui.Ctrl, ui.KeyA, m.tree.SelectAll)
	r.OnShortcut(0, ui.KeyEscape, func() {
		m.projectTreeFocus = false
		m.FocusEditor()
	})
	r.OnShortcut(ui.Cmd, ui.KeyZ, m.UndoLastProjectFileOperation)
	r.OnShortcut(0, ui.KeyDelete, m.DeleteSelectedTreeEntry)
	r.OnShortcut(0, ui.KeyBackspace, m.DeleteSelectedTreeEntry)
	r.OnShortcut(ui.Cmd, ui.KeyBackspace, m.DeleteSelectedTreeEntry)
	r.OnShortcut(0, ui.KeyF4, m.OpenSelectedTreeEntry)
	r.OnShortcut(0, ui.KeyF5, m.CopySelectedTreeElement)
	r.OnShortcut(0, ui.KeyF6, m.MoveSelectedTreeEntry)
	r.OnShortcut(ui.Shift, ui.KeyF6, m.RenameSelectedTreeEntry)
	r.OnShortcut(ui.Ctrl, ui.KeyEnter, m.NewFileAtProjectSelection)
	r.OnShortcut(0, ui.KeyUp, func() { m.tree.StepSelection(-1, false) })
	r.OnShortcut(ui.Shift, ui.KeyUp, func() { m.tree.StepSelection(-1, true) })
	r.OnShortcut(0, ui.KeyDown, func() { m.tree.StepSelection(1, false) })
	r.OnShortcut(ui.Shift, ui.KeyDown, func() { m.tree.StepSelection(1, true) })
	r.OnShortcut(0, ui.KeyLeft, func() { m.tree.CollapseSelection() })
	r.OnShortcut(0, ui.KeyRight, func() { m.tree.ExpandSelection() })
	r.OnShortcut(0, ui.KeyEnter, m.OpenSelectedTreeEntry)
	r.OnShortcut(ui.Cmd, ui.KeyDown, m.OpenSelectedTreeEntry)
	m.updateTreeDragTarget(row, r)
	m.handleTreeDrag(c, row, r)
	if !row.member && !row.readOnly {
		if paths := r.DroppedFiles(); len(paths) > 0 {
			target := row.path
			if !row.dir {
				target = filepath.Dir(row.path)
			}
			m.pasteExternalTreeFiles(paths, target)
		}
	}
	oi := oiPaletteOf(p)
	selected := !row.member && m.tree.sels[row.path]
	dropTarget := m.treeDrag != nil && m.treeDrag.active && row.dir &&
		m.projectTreeSamePath(m.treeDrag.targetPath, row.path)
	if dropTarget {
		r.Background(p.Accent.Alpha(0.16))
	} else if r.FileDragOver() {
		r.Background(p.Accent.Alpha(0.16))
	} else if selected {
		r.Background(p.Selection)
	} else if row.excluded {
		r.Background(p.Background.Mix(oi.ignored, 0.14))
	} else if row.dir && row.expanded {
		r.Background(p.Background)
	}
	if r.Hovered() {
		r.Background(p.SurfaceHover)
	}
	bookmarked := false
	for _, pb := range m.pathBookmarks {
		if pb.Path == row.path {
			bookmarked = true
			break
		}
	}
	r.Children(func() {
		label := row.name
		// The chevron slot is reserved on every row, as OpenIDEA's tree does,
		// so files and folders line up at the same depth.
		chevron := ui.ButtonBase(c.Key("tree-chevron:"+key)).Size(16, 16).Shrink(0).
			Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		_, membersLoaded := m.tree.Outline(row.path)
		members, _ := m.tree.Outline(row.path)
		hasMembers := !row.dir && !row.member && m.prefs.ProjectTree.ShowMembers &&
			(!membersLoaded || len(members) > 0)
		if row.dir || row.hasChildren {
			glyph := "general/chevronRight"
			if row.expanded {
				glyph = "general/chevronDown"
			}
			chevron.Children(func() {
				oiIconEl(c, glyph, 16, oi, oi.textMuted)
			})
		} else if hasMembers {
			glyph := "general/chevronRight"
			if row.expanded {
				glyph = "general/chevronDown"
			}
			chevron.Children(func() {
				oiIconEl(c, glyph, 16, oi, oi.textMuted)
			})
		}
		if chevron.Clicked() {
			if row.member {
				m.tree.ToggleMember(row.path, row.line, row.col, row.name)
			} else {
				m.tree.Toggle(row.path)
				if !row.dir && m.prefs.ProjectTree.ShowMembers {
					m.ensureProjectTreeMembers(row.path)
				}
			}
			m.invalidate()
		}
		// OpenIDEA's tree: a module icon on the root and on modules, the
		// package icon on a compacted package, a source folder on a source
		// root, a grey folder on any other directory, and the file type's
		// own icon — labels in the normal text colour.
		switch {
		case row.member:
			if asset := symbolIcon(strings.ToLower(row.kind)); asset != "" {
				oiIconEl(c, asset, 16, oi, oi.textMuted)
			} else {
				oiIconEl(c, "nodes/method", 16, oi, oi.textMuted)
			}
		case row.icon != "":
			oiIconEl(c, row.icon, 16, oi, oi.textSubtle)
		case row.path == m.Root() || row.module:
			folderIconEl(c, oi, "module", 16, oi.textSubtle)
		case row.packageDir:
			folderIconEl(c, oi, "package", 16, oi.textSubtle)
		case row.dir && isSourceRoot(row.path):
			folderIconEl(c, oi, "source", 16, oi.textSubtle)
		case row.dir:
			folderIconEl(c, oi, "plain", 16, oi.textSubtle)
		default:
			fileIconEl(c, oi, row.name, 16, oi.textMuted)
		}
		tone := p.Text
		if row.excluded {
			tone = oi.ignored
		} else if kind := m.projectTreeVCSKind(row.path, row.dir); kind != "" {
			tone = vcsTreeStatusColor(p, oi, kind)
		}
		// No ellipsis: a name wider than the island keeps its whole text and
		// scrolls with the tree, as OpenIDEA's rows do.
		text := ui.Text(c, label).FontSize(13).TextColor(tone).SingleLine()
		if row.module || row.path == m.Root() {
			text.Bold()
		}
		if row.detail != "" {
			ui.Text(c, row.detail).FontSize(12).TextColor(oi.textFaint).
				SingleLine().Ellipsis("…")
		}
		if badge := m.coverageBadge(row.path); badge != "" {
			ui.Text(c, badge).FontSize(12).TextColor(oi.textMuted).SingleLine()
		}
		if !row.member && (bookmarked || r.Hovered()) {
			ui.Spacer(c)
			chip := ui.Row(c.Key("treeBookmark:"+row.path)).Height(16).Padding(0, 5).
				AlignItems(ui.Center)
			if chip.Hovered() {
				chip.Background(p.SurfacePressed)
			}
			chip.Children(func() {
				tone := p.TextMuted
				if bookmarked {
					tone = p.Accent
				}
				ui.Text(c, "Bookmarks").FontSize(12).TextColor(tone).SingleLine()
			})
			if chip.Clicked() {
				m.ToggleFolderBookmark(row.path, row.dir)
			}
		}
	})
	if r.Clicked() {
		r.Focus()
		m.projectTreeFocus = true
		mods := c.Modifiers()
		extend := mods&ui.Shift != 0
		toggle := mods&(ui.Cmd|ui.Ctrl) != 0
		if row.member {
			m.tree.Select(row.path)
			m.revealAt(row.path, row.line, row.col)
			m.invalidate()
			return
		}
		m.tree.clickSelection(row.path, extend, toggle)
		if extend || toggle {
			m.invalidate()
			return
		}
		if row.dir {
			if m.prefs.ProjectTree.SingleClickExpand {
				m.tree.Toggle(row.path)
			}
		} else if m.prefs.ProjectTree.SingleClickOpen {
			m.openFile(row.path)
		} else if m.prefs.ProjectTree.PreviewTab {
			m.openPreview(row.path)
		} else if m.prefs.ProjectTree.AutoscrollToSource {
			if m.prefs.ProjectTree.PreviewTab {
				m.openPreview(row.path)
			} else {
				m.openFile(row.path)
			}
		}
		m.invalidate()
	}
	if r.DoubleClicked() {
		if row.member {
			m.revealAt(row.path, row.line, row.col)
		} else if row.dir {
			if !m.prefs.ProjectTree.SingleClickExpand {
				m.tree.Toggle(row.path)
			}
		} else {
			m.openFile(row.path)
		}
		m.invalidate()
	}
	if !row.member {
		target := treeTarget{
			path: row.path, dir: row.dir,
			root:                 row.path == m.Root() || m.workspaceContains(row.path),
			git:                  m.vcs.loaded && m.vcs.err == "",
			workspaceMember:      m.workspaceContains(row.path),
			workspaceCurrentRoot: workspacePathEqual(row.path, m.Root()),
			workspaceAttachment:  m.workspaceIsAttachment(),
		}
		r.ContextMenu(func(menu *ui.Menu) {
			m.projectTreeFocus = true
			if !m.tree.sels[row.path] {
				m.tree.Select(row.path)
			}
			if f, ok := m.statusFor(row.path); ok {
				target.kind = effectiveVCSKind(f)
				target.rollback = f.Unstaged != nil
			}
			m.treeMenu(c, menu, target)
		})
	}
	if !row.member && m.tree != nil && m.tree.Selected() == row.path {
		m.projectTreeAnchor = r.Bounds()
	}
}

func (m *Model) bannerRow(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Shrink(0).Padding(8, 12).Gap(10).AlignItems(ui.Center).
		Background(p.Surface).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).
		Children(func() {
			ui.Text(c, m.banner).FontSize(12).TextColor(p.Warning).SingleLine().Ellipsis("…").Grow(1)
			if m.bannerPath != "" {
				ui.Button(c, "Resync").OnClick(func() { m.resolveActive(false) })
				ui.Button(c, "Keep Local").OnClick(func() { m.resolveActive(true) })
			}
			ui.Button(c, "Close").OnClick(func() {
				m.banner, m.bannerPath = "", ""
				m.invalidate()
			})
		})
}

func (m *Model) editorArea(c *ui.Context, p *ui.Theme, g groupView) {
	t := g.tab()
	if t == nil {
		// An open project with no editor tab: IDEA's empty editor, the hints
		// that get a file open.
		m.emptyEditor(c, p)
		return
	}
	if t.kind == tabKindWhatsNew {
		m.whatsNewPage(c, p, t)
		return
	}
	if t.kind == tabKindAgentSession {
		m.agentSessionPage(c, p, oiPaletteOf(p), t)
		return
	}
	if t.kind == tabKindAgentHome {
		m.agentHomePage(c, p, oiPaletteOf(p))
		return
	}
	if t.kind == tabKindNotebook {
		m.notebookPage(c, p, t)
		return
	}
	if t.kind == tabKindVCSDiff {
		m.vcsDiffPage(c, p, t)
		return
	}
	if t.kind == tabKindPluginContent {
		m.pluginContentPage(c, p, t)
		return
	}
	if t.kind == tabKindImageViewer {
		m.imageViewerPage(c, p, oiPaletteOf(p), t)
		return
	}
	if t.kind == tabKindHexViewer {
		m.hexViewerPage(c, p, oiPaletteOf(p), t)
		return
	}
	markdownMode := m.markdownModeFor(t)
	if markdownMode == markdownPreviewOnly {
		key := "markdown:preview:" + t.doc.Path()
		if g.ref == groupSplit {
			key += ":split"
		}
		m.markdownPreviewPage(c, p, t, key)
		return
	}
	key := "editor:" + t.doc.Path()
	if g.ref == groupSplit {
		key += ":split"
	}
	area := ui.Column(c.Key(key)).Grow(1).MinHeight(0).MinWidth(0).
		Background(p.Background)
	m.editorBodyRects[int(g.ref)] = area.Bounds()
	area.Children(func() {
		// Breadcrumbs sit above the text unless Settings › Editor ›
		// Breadcrumbs puts them below it, and they are hidden when the page
		// says so or this file's language is disabled.
		showBreadcrumbs := m.showsBreadcrumbs(t)
		top := showBreadcrumbs && !m.crumbs.AtBottom()
		if top {
			m.breadcrumbs(c, p, t, g)
		}
		if t.blame != nil {
			m.refreshTabBlame(t)
		}
		m.djangoTemplateBanner(c, p, t)
		m.httpEditorToolbar(c, p, t)
		m.graphqlEditorToolbar(c, p, t)
		editorRow := ui.Row(c).Grow(1).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch)
		editorRow.Children(func() {
			if t.blame != nil {
				m.blameGutter(c, p, t)
			}
			view := editor.View(c, t.state).Grow(1).MinHeight(0).OnPaste(func(text string) bool {
				return m.editorPaste(t, text)
			})
			t.editorFocused = view.Focused() || view.FocusWithin()
			m.applyDeferredCaretCenter(t)
			if dropped := m.takeEditorDroppedFiles(view); len(dropped) > 0 {
				m.openPaths(dropped)
			}
			m.editorGutterTooltip(c, view, t)
			m.usagePopupAnchor = editorCaretAnchor(view.Bounds(), t.state, m.editorFontSize())
			if t.editorFocused {
				m.projectTreeFocus = false
			}
			m.positionDocumentation(t, view)
			m.templateChooserDialog(c, p)
			// A click in the unfocused pane takes the keyboard back to the main
			// group, whose commands act on m.tabs; applyTab always runs before the
			// next frame, so the queued swap lands between frames.
			switch {
			case g.focused && m.focusApply:
				m.focusApply = false
				view.Focus()
			case !g.focused && view.Focused():
				m.focusQueued = true
			}
			if m.gutterGroupEnabled("run") {
				m.dockerGutter(c, p, t, editorRow, view)
				m.httpGutter(c, p, t, editorRow, view)
				m.graphqlGutter(c, p, t, editorRow, view)
				m.pluginEndpointGutter(c, p, t, editorRow, view)
				m.springGutter(c, p, t, editorRow, view)
			}
			if markdownMode == markdownEditorPreview {
				ui.Row(c).Width(1).Shrink(0).Background(p.Border)
				m.markdownPreviewPage(c, p, t, "markdown:split:"+t.doc.Path())
			}
		})
		if showBreadcrumbs && !top {
			m.breadcrumbs(c, p, t, g)
		}
	})
}

// showsBreadcrumbs reports whether the current file's language allows the
// breadcrumb chain. The stored list is normalized so a hand-edited settings
// file with different case behaves like the checkbox in Settings.
func (m *Model) showsBreadcrumbs(t *Tab) bool {
	if m.crumbs.Hidden || t == nil || t.doc == nil {
		return false
	}
	language := strings.ToLower(strings.TrimSpace(t.doc.Info().Language))
	for _, disabled := range m.crumbs.DisabledLanguages {
		if strings.ToLower(strings.TrimSpace(disabled)) == language {
			return false
		}
	}
	return true
}

// breadcrumbs is the file and symbol chain the caret sits in; a symbol jumps
// to its declaration.
func (m *Model) breadcrumbs(c *ui.Context, p *ui.Theme, t *Tab, g groupView) {
	pos := t.state.CursorPos()
	chain := outlinePath(t.outline, pos.Line)
	ui.Row(c).Height(26).Shrink(0).Padding(0, 10).Gap(4).AlignItems(ui.Center).
		Children(func() {
			ui.Text(c, filepath.Base(t.doc.Path())).FontSize(12).
				TextColor(p.TextMuted).SingleLine()
			for i, it := range chain {
				it := it
				ui.Text(c, "›").FontSize(12).TextColor(p.TextMuted)
				chip := ui.Row(c.Key(fmt.Sprintf("bc:%d:%d:%s", g.ref, i, it.Name))).Padding(2, 4).
					AlignItems(ui.Center)
				if chip.Hovered() {
					chip.Background(p.SurfaceHover)
				}
				chip.Children(func() {
					ui.Text(c, it.Name).FontSize(12).TextColor(p.Text).SingleLine()
				})
				if chip.Clicked() {
					line, col := t.doc.BytePos(
						int(it.SelectionRange.Start.Line), int(it.SelectionRange.Start.Col))
					t.state.GoTo(editor.Pos{Line: line, Col: col})
					if g.ref == groupSplit {
						m.focusQueued = true
					}
					m.invalidate()
				}
			}
		})
}

func (m *Model) runPanel(c *ui.Context, p *ui.Theme) {
	if !m.run.visible {
		return
	}
	oi := oiPaletteOf(p)
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).
		Background(m.toolWindowBackground(p)).Radius(oiIslandRadius).Clip()
	col.Children(func() {
		m.toolWindowHeader(c, p, oi, "run", func() {
			m.runSessionTabs(c, p)
		})
		if m.run.viewChild != nil {
			m.runViewedServicePanel(c, p)
			return
		}
		if !m.hasRunContent() {
			m.runEmptyState(c, p)
			return
		}
		if m.renderStackTraces(c, p) {
			return
		}
		session := m.activeRunSession()
		if session != nil {
			m.runSessionPanel(c, p, session)
		}
	})
}

func (m *Model) buildPanel(c *ui.Context, p *ui.Theme) {
	b := &m.build
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	col.Children(func() {
		m.buildHeader(c, p)
		if b.err != "" {
			ui.Row(c).Height(24).Padding(0, 10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, b.err).FontSize(12).TextColor(p.Warning).SingleLine().Ellipsis("…")
			})
		}
		if len(b.messages) == 0 {
			m.buildConsole(c, p)
			return
		}
		ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			ui.Column(c).Grow(2).MinWidth(220).MinHeight(0).
				BorderWidth(0, 1, 0, 0).BorderColor(p.Border).Children(func() {
				m.buildMessageTree(c, p)
			})
			ui.Column(c).Grow(3).MinWidth(0).MinHeight(0).Children(func() {
				m.buildConsole(c, p)
			})
		})
	})
}

func (m *Model) buildConsole(c *ui.Context, p *ui.Theme) {
	b := &m.build
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(8, 10).Children(func() {
			if !m.run.trusted {
				ui.Text(c, "This project is not trusted: building is disabled.").
					FontSize(12).TextColor(p.TextMuted)
				ui.Row(c).Padding(6, 0).Children(func() {
					ui.Button(c, "Trust Project").OnClick(func() { m.TrustProject() })
				})
				return
			}
			if len(b.output) == 0 {
				ui.Text(c, "No output yet. Press Build Project to start.").
					FontSize(12).TextColor(p.TextMuted)
				return
			}
			if b.truncated {
				ui.Text(c, "(Output truncated: earlier lines were omitted)").
					FontSize(12).TextColor(p.TextMuted)
			}
			m.renderRunOutput(c, p, b.output)
		})
	})
}

func (m *Model) buildMessageTree(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	b := &m.build
	ui.Row(c).Height(28).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		ui.Text(c, "Compiler messages").FontSize(12).Bold().TextColor(oi.textStrong).
			SingleLine().Grow(1)
		ui.Text(c, fmt.Sprintf("%d", len(b.messages))).FontSize(11).
			TextColor(oi.textMuted)
	})
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(4, 4).Gap(1).Children(func() {
			path := ""
			for i := range b.messages {
				message := b.messages[i]
				messagePath := ""
				if message.Path != nil {
					messagePath = *message.Path
				}
				if messagePath != "" && messagePath != path {
					path = messagePath
					ui.Text(c, filepath.Base(path)).FontSize(11).Bold().
						TextColor(oi.textMuted).Padding(14, 5).SingleLine().Ellipsis("…")
				}
				row := ui.Row(c.Key(fmt.Sprintf("build:message:%d", i))).Height(26).
					Padding(4, 6).Gap(6).Radius(oiRadiusS).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				if i == b.selectedMessage {
					row.Background(oi.selected)
				} else if row.Hovered() {
					row.Background(oi.hoverTint)
				}
				row.OnClick(func() {
					b.selectedMessage = i
					if message.Path != nil {
						line, col := 1, 0
						if message.Range != nil {
							line = int(message.Range.Start.Line)
							col = int(message.Range.Start.Col)
						}
						m.previewAt(*message.Path, line, col)
					}
					m.invalidate()
				})
				row.Children(func() {
					icon := "status/info"
					tone := oi.textMuted
					switch strings.ToLower(message.Severity) {
					case "error":
						icon, tone = "status/error", oi.danger
					case "warning":
						icon, tone = "status/warning", oi.warning
					}
					oiIconEl(c, icon, 14, oi, tone)
					ui.Text(c, message.Text).FontSize(12).TextColor(oi.text).
						SingleLine().Ellipsis("…").Grow(1)
					if message.Range != nil {
						ui.Text(c, fmt.Sprintf("%d", message.Range.Start.Line+1)).
							FontSize(10).TextColor(oi.textMuted)
					}
				})
			}
		})
	})
}

func (m *Model) buildHeader(c *ui.Context, p *ui.Theme) {
	b := &m.build
	ui.Row(c).Height(30).Padding(0, 8).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Text(c, "Build").FontSize(12).Bold().TextColor(p.Text)
		title := b.title
		if title == "" {
			title = "Build"
		}
		if b.system != "" {
			title += " · " + b.system
		}
		ui.Text(c, title).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
		ui.Spacer(c)
		summary := m.buildSummaryText()
		if summary != "" {
			ui.Text(c, summary).FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		}
		if b.busy {
			ui.Button(c, "Stop").OnClick(func() { m.StopBuild() })
		} else {
			ui.Button(c, "Build").OnClick(func() { m.BuildProject() })
		}
		ui.Button(c, "Clear").OnClick(func() { m.ClearBuild() })
		ui.Button(c, "Collapse").OnClick(func() { m.ShowBuild() })
	})
}

// buildSummaryText is the header's one-line result: the progress while a
// build runs, then the outcome and counts.
func (m *Model) buildSummaryText() string {
	b := &m.build
	if b.busy {
		if b.progress != "" {
			return b.progress
		}
		return "Building…"
	}
	if b.success == nil {
		return ""
	}
	if *b.success {
		if b.warnings > 0 {
			return fmt.Sprintf("Succeeded · %s · %d warnings", formatBuildDuration(b.duration), b.warnings)
		}
		return "Succeeded · " + formatBuildDuration(b.duration)
	}
	return fmt.Sprintf("Failed · %d errors · %d warnings", b.errors, b.warnings)
}

// renderText is the output as one string; empty lines keep their height.
func (p *RunPane) renderText() string {
	var b strings.Builder
	for i, line := range p.output {
		if i > 0 {
			b.WriteByte('\n')
		}
		if line.text == "" {
			b.WriteByte(' ')
		} else {
			b.WriteString(line.text)
		}
	}
	return b.String()
}

// renderRunOutput draws console output through one tightly grouped RichText.
// Compiler paths and stack frames jump to source, URLs use the system browser,
// and large outputs without links stay one text node rather than thousands.
func (m *Model) renderRunOutput(c *ui.Context, p *ui.Theme, output []outputLine) {
	text := ui.RichText(c).Font(oiFontMono).FontSize(12).TextColor(p.Text).Selectable()
	text.Children(func() {
		var plain strings.Builder
		flush := func() {
			if plain.Len() == 0 {
				return
			}
			ui.Text(c, plain.String()).FontSize(12)
			plain.Reset()
		}
		for i, entry := range output {
			line := entry.text
			if line == "" {
				line = " "
			}
			links := traceLinks(line)
			if len(links) == 0 {
				plain.WriteString(line)
				if i+1 < len(output) {
					plain.WriteByte('\n')
				}
				continue
			}
			pos := 0
			for _, link := range links {
				if link.start > pos {
					plain.WriteString(line[pos:link.start])
					flush()
				}
				label := line[link.start:link.end]
				if link.url != "" {
					ui.Link(c, label, link.url).FontSize(12)
				} else {
					link := link
					ui.Text(c, label).FontSize(12).TextColor(p.Accent).
						Cursor(ui.CursorPointer).OnClick(func() { m.openTraceLink(link, c) })
				}
				pos = link.end
			}
			if pos < len(line) {
				plain.WriteString(line[pos:])
			}
			if i+1 < len(output) {
				plain.WriteByte('\n')
			}
		}
		flush()
	})
}

// termPanel is the integrated terminal. With one group it shows that group's
// tabs, scrollback and input line; a split shows two groups, side by side for
// Split Right or stacked for Split Down, as OpenIDEA splits its terminal.
func (m *Model) termPanel(c *ui.Context, p *ui.Theme) {
	t := &m.term
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	// Find in Terminal, bounded to the focus: Cmd+F in the editor stays the
	// editor's.
	if col.Shortcut(ui.Cmd, ui.KeyF) {
		m.ShowTermFind()
	}
	col.Children(func() {
		ui.Row(c).Height(30).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "Terminal").FontSize(12).Bold().TextColor(p.Text)
			if n := t.total(); n > 0 {
				ui.Text(c, count(n, "session", "sessions")).
					FontSize(12).TextColor(p.TextMuted)
			}
			ui.Spacer(c)
			ui.Button(c, "New").OnClick(func() { m.OpenTermTab() })
			ui.Button(c, "⌄").OnClick(func() { m.OpenTermShellPicker() })
			ui.Button(c, "Split").OnClick(func() { m.SplitTermRight() })
			ui.Button(c, "Split Down").OnClick(func() { m.SplitTermDown() })
			if t.split != "" {
				ui.Button(c, "Unsplit").OnClick(func() { m.UnsplitTerm() })
			}
			if s := t.current(); s == nil || (s.id == 0 && !s.starting) {
				ui.Button(c, "Start").OnClick(func() { m.SpawnTerm() })
			} else {
				ui.Button(c, "Kill").OnClick(func() { m.KillTerm() })
			}
			ui.Button(c, "Clear").OnClick(func() { m.ClearTerm() })
			ui.Button(c, "Collapse").OnClick(func() { m.ShowTerm() })
		})
		if t.err != "" {
			ui.Row(c).Shrink(0).Padding(4, 10).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, t.err).FontSize(12).TextColor(p.Danger).
					SingleLine().Ellipsis("…").Grow(1)
				ui.Button(c, "Close").OnClick(func() {
					t.err = ""
					m.invalidate()
				})
			})
		}
		if s := t.current(); s != nil && s.finding {
			ui.Row(c).Height(30).Shrink(0).Padding(3, 8).Gap(6).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
				ui.Text(c, "Find").FontSize(12).TextColor(p.TextMuted)
				in := ui.TextInput(c, &s.find).Grow(1).AutoFocus()
				if in.Submitted() {
					m.FindTermStep(1)
				}
				if label := m.termFindLabel(); label != "" {
					ui.Text(c, label).FontSize(12).TextColor(p.TextMuted)
				}
				ui.Button(c, "Previous").OnClick(func() { m.FindTermStep(-1) })
				ui.Button(c, "Next").OnClick(func() { m.FindTermStep(1) })
				ui.Button(c, "Close").OnClick(func() { m.HideTermFind() })
			})
		}
		if t.split != "" && len(t.groups) > 1 {
			if t.split == "down" {
				ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
					m.termPaneBody(c, p, 0)
					ui.Row(c).Height(1).Shrink(0).Background(p.Border)
					m.termPaneBody(c, p, 1)
				})
			} else {
				ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
					m.termPaneBody(c, p, 0)
					ui.Column(c).Width(1).Shrink(0).Background(p.Border)
					m.termPaneBody(c, p, 1)
				})
			}
			return
		}
		m.termPaneBody(c, p, 0)
	})
}

// termPaneBody draws one tab group: its own tab strip, its active tab's
// scrollback, and the input line when this group takes typing.
func (m *Model) termPaneBody(c *ui.Context, p *ui.Theme, g int) {
	t := &m.term
	if g < 0 || g >= len(t.groups) {
		return
	}
	gr := &t.groups[g]
	pane := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0)
	pane.Children(func() {
		ui.Row(c).Height(26).Shrink(0).Padding(0, 6).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			for i := range gr.tabs {
				s := &gr.tabs[i]
				label := termTabTitle(s, i)
				selected := t.active == g && gr.active == i
				tabColor := p.TextMuted
				if selected {
					tabColor = p.Text
				}
				tab := ui.Row(c.Key(fmt.Sprintf("termTab:%d", s.channel))).
					Height(22).Padding(0, 6).Gap(2).AlignItems(ui.Center)
				if selected {
					tab.Background(p.SurfacePressed)
				}
				tab.Children(func() {
					ui.Text(c, label).FontSize(12).TextColor(tabColor).
						SingleLine().Ellipsis("…")
					ui.Button(c, "×").OnClick(func() { m.CloseTermTab(g, i) })
				})
				tab.ContextMenu(func(menu *ui.Menu) {
					m.termTabMenu(menu, g, i, tab.Bounds())
				})
				if tab.Clicked() {
					m.ActivateTermTab(g, i)
				}
			}
			if len(gr.tabs) == 0 {
				ui.Text(c, "Not running").FontSize(12).TextColor(p.TextMuted)
			}
			ui.Spacer(c)
			if t.split != "" {
				ui.Button(c, "+").OnClick(func() { m.OpenTermTabIn(g) })
			}
		})
		// The scrollback is one selectable text row per terminal line. Input is
		// handled by the grid itself so navigation keys and full-screen programs
		// share the same PTY path as typed text.
		s := gr.tab()
		scroll := ui.Scroll(c).Grow(1).MinHeight(0)
		if s != nil {
			scroll.TrackScroll(&s.scroll)
		}
		metrics := m.terminalMetrics()
		if s != nil {
			m.reconcileTerminalViewport(s, scroll.Bounds().W, scroll.Bounds().H, metrics)
		}
		scroll.Children(func() {
			grid := ui.Column(c).Padding(terminalPadY, terminalPadX).
				Bind(&gr.focus).Focusable().FocusRing(false).Selectable().
				CopyData(terminalCopyData)
			if s != nil {
				grid.HandleInput(func(ev ui.InputEvent) bool {
					return m.handleTerminalInput(g, s, ev, metrics.cellWidth, metrics.cellHeight)
				})
				caretRow, caretCol := 0, 0
				if s.alternate {
					caretRow, caretCol = s.screenRow, s.screenCol
				} else if len(s.lines) > 0 {
					caretRow = len(s.lines) - 1
					caretCol = terminalTextCellWidth(s.lines[caretRow])
				}
				grid.TextCaret(ui.Rect{
					X: terminalPadX + float32(caretCol)*metrics.cellWidth,
					Y: terminalPadY + float32(caretRow)*metrics.cellHeight,
					W: metrics.cellWidth, H: metrics.cellHeight,
				})
			}
			if (t.active == g || len(t.groups) == 1) &&
				(s == nil || !s.finding) {
				grid.AutoFocus()
			}
			if s != nil {
				m.reportTerminalFocus(s, grid.Focused())
			}
			cursorVisible := s != nil && s.alternate && !s.cursorHidden
			if cursorVisible && m.terminalCursorBlink() {
				cursorVisible = grid.Loop(
					fmt.Sprintf("term-cursor:%d", s.channel),
					terminalCursorBlink, ui.Linear,
				) < 0.5
			}
			grid.Children(func() {
				if s == nil || len(s.lines) == 0 {
					hint := "The terminal's output appears here."
					if s == nil || (s.id == 0 && !s.starting) {
						hint = "Press New to open a login shell."
					}
					ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted)
					return
				}
				if s.truncated {
					ui.Text(c, "(Output truncated: earlier lines were omitted)").
						FontSize(12).TextColor(p.TextMuted)
				}
				var hits map[int]bool
				if s.finding && s.find != "" && t.active == g {
					hits = map[int]bool{}
					for _, line := range termHits(s.lines, s.find) {
						hits[line] = true
					}
				}
				for i, line := range s.lines {
					text := line
					if text == "" {
						text = " "
					}
					cursorCol := -1
					if cursorVisible && i == s.screenRow {
						cursorCol = s.screenCol
					}
					spans := terminalLineSpans(
						text, s.hyperlinks, i, m.terminalHighlightHyperlinks(),
						p.Text, p.Accent, p.Accent, p.Background, metrics.spacing,
						metrics.font, cursorCol, m.terminalCursorShape(), cursorCol >= 0,
					)
					row := ui.RichText(c, spans...).
						Font(metrics.font.Family).FontSize(metrics.font.Size).
						LetterSpacing(metrics.spacing).SingleLine().
						FixedLineHeight(metrics.cellHeight).
						Height(metrics.cellHeight).TextColor(p.Text).
						Key(fmt.Sprintf("termLine:%d:%d", s.channel, i))
					if hits[i] {
						if i == s.findAt {
							row.Background(p.Selection)
						} else {
							row.Background(p.SurfacePressed)
						}
					}
					if s.reveal && i == s.findAt {
						row.ScrollIntoView()
					}
				}
				s.reveal = false
			})
			if s != nil {
				m.updateTerminalCopyOnSelection(s, grid.SelectedTexts())
			}
		})
	})
}

func (m *Model) debugFrames(c *ui.Context, p *ui.Theme) {
	d := &m.debug
	ui.Column(c).Width(260).Shrink(0).MinHeight(0).
		BorderWidth(0, 1, 0, 0).BorderColor(p.Border).Children(func() {
		ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			thread := "Call Stack"
			if len(d.threads) > 0 {
				thread = fmt.Sprintf("Call Stack · %s", d.threads[0].Name)
			}
			ui.Text(c, thread).FontSize(13).Bold().TextColor(p.TextMuted).
				SingleLine().Ellipsis("…")
		})
		ui.Scroll(c).Grow(1).Children(func() {
			if !d.paused || len(d.frames) == 0 {
				ui.Row(c).Padding(8, 8).Children(func() {
					ui.Text(c, d.status).FontSize(12).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
				})
				return
			}
			for i, f := range d.frames {
				i, f := i, f
				row := ui.Row(c.Key(fmt.Sprintf("dframe:%d", i))).Height(24).Padding(0, 8).
					Gap(6).AlignItems(ui.Center)
				if i == d.selFrame {
					row.Background(p.SurfaceHover)
				}
				row.Children(func() {
					ui.Text(c, fmt.Sprintf("%s  :%d", f.Name, f.Line+1)).
						FontSize(13).TextColor(p.Text).SingleLine().Ellipsis("…")
				})
				if row.Clicked() {
					m.SelectFrame(i)
				}
				row.Menu(func(menu *ui.Menu) {
					if menu.Item("Jump to Source").Chosen() {
						m.SelectFrame(i)
						m.DebugJumpToSource()
					}
					item := menu.Item("Reset Frame")
					reason := m.debugResetFrameDisabledReasonAt(i)
					item.Disabled(reason != "")
					if reason == "" && item.Chosen() {
						d.selFrame = i
						m.DebugResetFrame()
					}
				})
			}
		})
	})
}

func (m *Model) debugVars(c *ui.Context, p *ui.Theme) {
	d := &m.debug
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			name := "Variables"
			if len(d.scopes) > 0 {
				name = "Variables · " + d.scopes[0].Name
			}
			ui.Text(c, name).FontSize(13).Bold().TextColor(p.TextMuted).
				SingleLine().Ellipsis("…")
		})
		ui.Scroll(c).Grow(1).Children(func() {
			for i, w := range d.watches {
				i, w := i, w
				row := ui.Row(c.Key(fmt.Sprintf("dwatch:%d", i))).Height(24).Padding(0, 8).
					Gap(6).AlignItems(ui.Center)
				row.Children(func() {
					ui.Text(c, w.Expr).FontSize(13).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
					ui.Text(c, "=").FontSize(13).TextColor(p.TextMuted)
					value, color := w.Value, p.Text
					if w.Err != "" {
						value, color = w.Err, p.Danger
					} else if d.paused && d.evalBusy {
						value = "…"
					} else if value == "" {
						value = "(empty)"
					}
					ui.Text(c, value).FontSize(13).TextColor(color).
						SingleLine().Ellipsis("…").Grow(1)
					if b := ui.Button(c, "×"); b.Clicked() {
						m.RemoveWatch(i)
					}
				})
			}
			if len(d.vars) == 0 {
				if len(d.watches) > 0 {
					return
				}
				hint := "Local variables appear when paused"
				if d.paused {
					hint = "No local variables"
				}
				ui.Row(c).Padding(8, 8).Children(func() {
					ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted)
				})
				return
			}
			for _, i := range sortedDebugVariableIndexes(d.vars, m.prefs.Debugger.SortValues) {
				i, v := i, d.vars[i]
				row := ui.Row(c.Key(fmt.Sprintf("dvar:%d", i))).Height(24).Padding(0, 8).
					Gap(6).AlignItems(ui.Center)
				if i == d.selVar {
					row.Background(p.SurfaceHover)
				}
				row.Children(func() {
					ui.Text(c, v.Name).FontSize(13).TextColor(p.TextMuted).
						SingleLine().Ellipsis("…")
					ui.Text(c, "=").FontSize(13).TextColor(p.TextMuted)
					ui.Text(c, v.Value).FontSize(13).TextColor(p.Text).
						SingleLine().Ellipsis("…").Grow(1)
				})
				if row.Clicked() {
					d.selVar = i
					m.invalidate()
				}
				if row.RowDoubleClicked() {
					d.selVar = i
					m.OpenDebugSetValue()
				}
				row.Menu(func(menu *ui.Menu) {
					if menu.Item("Set Value…").Chosen() {
						d.selVar = i
						m.OpenDebugSetValue()
					}
					if menu.Item("Add to Watches").Disabled(v.EvaluateName == nil).Chosen() {
						m.addWatch(*v.EvaluateName)
					}
					if menu.Item("Jump to Source").Chosen() {
						m.DebugJumpToSource()
					}
				})
			}
		})
		ui.Row(c).Height(32).Shrink(0).Padding(0, 8).Gap(6).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
			if !d.paused {
				ui.Text(c, "Evaluate in the current frame when paused").FontSize(12).
					TextColor(p.TextMuted).SingleLine().Ellipsis("…")
				return
			}
			in := ui.TextInput(c, &d.evalText).Grow(1)
			if in.Submitted() {
				m.SubmitWatch()
			}
			ui.Button(c, "Evaluate").Disabled(d.evalBusy).OnClick(func() { m.SubmitWatch() })
		})
	})
}

// bottomSlot draws the bottom side: the window of its primary area and, when a
// window was moved there, the split area's window beside it, as OpenIDEA lays
// out the two areas of a side. The side keeps one height whichever it shows.
func (m *Model) bottomSlot(c *ui.Context, p *ui.Theme) {
	primary, split := m.primaryTool(), m.splitTool()
	if m.undockedWindow(primary) {
		primary = ""
	}
	if m.undockedWindow(split) {
		split = ""
	}
	if primary == "" && split == "" {
		return
	}
	// A drag of the gap below half the smallest height draws no island until
	// the release closes the windows.
	if !m.maximized && m.bottomDrawHeight() == 0 {
		return
	}
	row := ui.Row(c).Shrink(0).Padding(0, oiIslandGap).Gap(oiIslandGap).
		AlignItems(ui.Stretch)
	if m.maximized {
		row.Grow(1)
	} else {
		row.Height(float32(m.bottomDrawHeight()))
	}
	row.Children(func() {
		if primary != "" {
			m.bottomToolIsland(c, p, primary)
		}
		if split != "" {
			m.bottomToolIsland(c, p, split)
		}
	})
}

func (m *Model) bottomToolIsland(c *ui.Context, p *ui.Theme, id string) {
	island := ui.Column(c).Key("bottom:island:" + id).Grow(1).MinWidth(0).
		MinHeight(0).Background(m.toolWindowBackground(p)).Radius(oiIslandRadius).
		Clip()
	if m.consumeToolWindowFocus(id) {
		island.Focusable().AutoFocus()
	}
	island.Children(func() {
		m.toolPanel(c, p, id)
	})
}

func (m *Model) toolPanel(c *ui.Context, p *ui.Theme, name string) {
	switch name {
	case "run":
		m.runPanel(c, p)
	case "debug":
		m.debugPanel(c, p)
	case "build":
		m.buildPanel(c, p)
	case "term":
		m.termPanel(c, p)
	case "log":
		m.logPanel(c, p)
	case "problems":
		m.problemsPanel(c, p)
	case "todo":
		m.todoPanel(c, p)
	case "find":
		m.searchPanel(c, p)
	case "usages":
		m.usagesPanel(c, p)
	case "services":
		m.servicesPanel(c, p)
	case "graphql":
		m.graphqlPanel(c, p)
	}
}

func (m *Model) searchPanel(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	s := &m.search
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border)
	col.Children(func() {
		ui.Row(c).Key("find:header").Height(oiHeaderHeight).Shrink(0).
			Padding(0, 8, 0, 10).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
			ui.Text(c, "Find").FontSize(13).Bold().TextColor(oi.textStrong).
				SingleLine()
			query := strings.TrimSpace(s.query)
			if query == "" {
				query = "Find in Files"
			}
			ui.Text(c, query).FontSize(12).TextColor(oi.textMuted).
				SingleLine().Ellipsis("…").Grow(1)
			if s.matches > 0 {
				ui.Text(c, count(s.matches, "match", "matches")).FontSize(11).
					TextColor(oi.textFaint)
			}
			m.headerIcon(c, p, oi, "find-hide", "Hide", "general/close",
				func() { m.DockHide("find") })
		})
		if !s.done || len(s.files) == 0 {
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() { m.searchRows(c, p) })
			return
		}
		if s.previewKey == "" && s.previewErr == "" {
			s.refreshPreview()
		}
		ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			ui.Column(c).Grow(2).MinWidth(280).MinHeight(0).
				BorderWidth(0, 1, 0, 0).BorderColor(oi.border).Children(func() {
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					m.searchRows(c, p)
				})
			})
			ui.Column(c).Grow(3).MinWidth(0).MinHeight(0).Children(func() {
				m.searchPreview(c, p)
			})
		})
	})
}

func (m *Model) searchPreview(c *ui.Context, p *ui.Theme) {
	oi := oiPaletteOf(p)
	s := &m.search
	if len(s.files) == 0 {
		emptyState(c, p, "Select a search result")
		return
	}
	file := s.files[s.selectedFile]
	ui.Row(c).Height(28).Shrink(0).Padding(0, 10).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(oi.border).Children(func() {
		fileIconEl(c, oi, file.Path, 15, oi.textMuted)
		ui.Text(c, file.RelPath).FontSize(12).TextColor(oi.textStrong).
			SingleLine().Ellipsis("…").Grow(1)
		ui.Text(c, fmt.Sprintf("%d", s.previewLine+1)).FontSize(11).
			TextColor(oi.textFaint)
	})
	if s.previewErr != "" {
		errorState(c, p, s.previewErr)
		return
	}
	if len(s.previewLines) == 0 {
		loadingState(c, p, "Loading preview…")
		return
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(4, 0).Children(func() {
			for i, line := range s.previewLines {
				lineNumber := s.previewFrom + i
				row := ui.Row(c.Key(fmt.Sprintf("find:preview:%d", lineNumber))).
					Height(20).Shrink(0).Padding(0, 8).Gap(10).AlignItems(ui.Center)
				if lineNumber == s.previewLine {
					row.Background(oi.selected)
				}
				row.Children(func() {
					ui.Text(c, fmt.Sprintf("%d", lineNumber+1)).Width(42).
						FontSize(11).Font(oiFontMono).TextColor(oi.textFaint).
						SingleLine()
					ui.Text(c, strings.ReplaceAll(strings.TrimSuffix(line, "\r"), "\t", "    ")).
						FontSize(12).Font(oiFontMono).TextColor(oi.text).
						SingleLine().Ellipsis("…")
				})
			}
		})
	})
}

func (m *Model) usagesPanel(c *ui.Context, p *ui.Theme) {
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(m.toolWindowBackground(p)).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).
		Focusable().AutoFocus().HandleInput(m.handleUsagesInput)
	col.Children(func() {
		ui.Scroll(c).Grow(1).Children(func() { m.usagesRows(c, p) })
	})
}
