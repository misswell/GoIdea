package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// runConfigFolder is an empty or populated folder under one configuration
// type. OpenIDEA keeps empty folders for the open dialog even though a run
// configuration is the only object persisted for a populated folder.
type runConfigFolder struct {
	Kind string
	Name string
}

type runConfigTreeRowKind uint8

const (
	runConfigGroupRow runConfigTreeRowKind = iota
	runConfigFolderRow
	runConfigConfigRow
)

// runConfigTreeRow is one row of the native Configurations tree.
type runConfigTreeRow struct {
	Kind   runConfigTreeRowKind
	Key    string
	Label  string
	KindID string
	Folder string
	Index  int
	Depth  int
}

// runConfigTreeRows groups configurations by type, puts named folders before
// loose configurations, and omits children of collapsed groups and folders.
func runConfigTreeRows(d *runConfigDlg) []runConfigTreeRow {
	if d == nil {
		return nil
	}
	var kinds []string
	seenKinds := map[string]bool{}
	for _, cfg := range d.items {
		if cfg == nil || seenKinds[cfg.Kind] {
			continue
		}
		seenKinds[cfg.Kind] = true
		kinds = append(kinds, cfg.Kind)
	}
	for _, folder := range d.extraFolders {
		if folder.Kind == "" || seenKinds[folder.Kind] {
			continue
		}
		seenKinds[folder.Kind] = true
		kinds = append(kinds, folder.Kind)
	}

	rows := make([]runConfigTreeRow, 0, len(d.items)+len(kinds)+len(d.extraFolders))
	for _, kind := range kinds {
		groupKey := "group:" + kind
		rows = append(rows, runConfigTreeRow{
			Kind: runConfigGroupRow, Key: groupKey, Label: adapter.KindLabel(kind), KindID: kind,
		})
		if d.collapsedGroups[groupKey] {
			continue
		}
		var folders []string
		seenFolders := map[string]bool{}
		for _, cfg := range d.items {
			if cfg == nil || cfg.Kind != kind || cfg.Folder == nil || *cfg.Folder == "" ||
				seenFolders[*cfg.Folder] {
				continue
			}
			seenFolders[*cfg.Folder] = true
			folders = append(folders, *cfg.Folder)
		}
		for _, folder := range d.extraFolders {
			if folder.Kind != kind || seenFolders[folder.Name] {
				continue
			}
			seenFolders[folder.Name] = true
			folders = append(folders, folder.Name)
		}
		for _, name := range folders {
			folderKey := "folder:" + kind + ":" + name
			rows = append(rows, runConfigTreeRow{
				Kind: runConfigFolderRow, Key: folderKey, Label: name, KindID: kind, Folder: name, Depth: 1,
			})
			if d.collapsedFolder[folderKey] {
				continue
			}
			for index, cfg := range d.items {
				if cfg != nil && cfg.Kind == kind && cfg.Folder != nil && *cfg.Folder == name {
					rows = append(rows, runConfigTreeRow{
						Kind: runConfigConfigRow, Key: "config:" + cfg.ID, Label: cfg.Name,
						KindID: kind, Folder: name, Index: index, Depth: 2,
					})
				}
			}
		}
		for index, cfg := range d.items {
			if cfg == nil || cfg.Kind != kind || (cfg.Folder != nil && *cfg.Folder != "") {
				continue
			}
			rows = append(rows, runConfigTreeRow{
				Kind: runConfigConfigRow, Key: "config:" + cfg.ID, Label: cfg.Name,
				KindID: kind, Index: index, Depth: 1,
			})
		}
	}
	return rows
}

func (d *runConfigDlg) selectedFolder() (string, string, bool) {
	if d == nil || d.folderKind == "" {
		return "", "", false
	}
	return d.folderKind, d.folderName, true
}

func (d *runConfigDlg) newConfigFolder(kind string) string {
	if kind == "" {
		return ""
	}
	if d.folderKind == kind {
		return d.folderName
	}
	if cfg := d.selected(); cfg != nil && cfg.Kind == kind && cfg.Folder != nil {
		return *cfg.Folder
	}
	return ""
}

func (d *runConfigDlg) selectConfig(index int) {
	if d == nil || index < 0 || index >= len(d.items) {
		return
	}
	if d.folderKind == "" && d.sel == index {
		return
	}
	d.applyForm()
	d.sel = index
	d.folderKind, d.folderName = "", ""
	d.folderDraft = ""
	d.loadForm()
}

func (d *runConfigDlg) selectFolder(kind, name string) {
	if d == nil || kind == "" || name == "" {
		return
	}
	if d.folderKind == kind && d.folderName == name {
		return
	}
	d.applyForm()
	d.sel = -1
	d.folderKind, d.folderName = kind, name
	d.folderDraft = name
	d.loadForm()
	d.errs = nil
	d.saveErr = ""
}

func (d *runConfigDlg) toggleGroup(key string) {
	if d.collapsedGroups == nil {
		d.collapsedGroups = map[string]bool{}
	}
	d.collapsedGroups[key] = !d.collapsedGroups[key]
}

func (d *runConfigDlg) toggleFolder(key string) {
	if d.collapsedFolder == nil {
		d.collapsedFolder = map[string]bool{}
	}
	d.collapsedFolder[key] = !d.collapsedFolder[key]
}

func uniqueRunConfigFolder(kind string, items []*openidea.RunConfig, extra []runConfigFolder) string {
	taken := map[string]bool{}
	for _, cfg := range items {
		if cfg != nil && cfg.Kind == kind && cfg.Folder != nil {
			taken[*cfg.Folder] = true
		}
	}
	for _, folder := range extra {
		if folder.Kind == kind {
			taken[folder.Name] = true
		}
	}
	if !taken["New Folder"] {
		return "New Folder"
	}
	for index := 2; ; index++ {
		name := fmt.Sprintf("New Folder (%d)", index)
		if !taken[name] {
			return name
		}
	}
}

// NewRunConfigFolder creates a folder and, when a configuration is selected,
// moves that configuration into it like IDEA's Create New Folder.
func (m *Model) NewRunConfigFolder() {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	kind := d.folderKind
	if kind == "" {
		if cfg := d.selected(); cfg != nil {
			kind = cfg.Kind
		}
	}
	if kind == "" {
		return
	}
	d.applyForm()
	name := uniqueRunConfigFolder(kind, d.items, d.extraFolders)
	d.extraFolders = append(d.extraFolders, runConfigFolder{Kind: kind, Name: name})
	if cfg := d.selected(); cfg != nil && cfg.Kind == kind {
		cfg.Folder = &name
		cfg.Detected = false
	}
	d.sel = -1
	d.folderKind, d.folderName = kind, name
	d.folderDraft = name
	d.errs = nil
	d.saveErr = ""
	m.invalidate()
}

// RenameRunConfigFolder keeps every member of a folder together and moves the
// folder in the tree to its new name.
func (m *Model) RenameRunConfigFolder(name string) {
	d := m.runCfgDlg
	if d == nil || d.folderKind == "" {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" || name == d.folderName {
		d.folderDraft = d.folderName
		m.invalidate()
		return
	}
	for _, cfg := range d.items {
		if cfg != nil && cfg.Kind == d.folderKind && cfg.Folder != nil &&
			*cfg.Folder != d.folderName && *cfg.Folder == name {
			d.folderDraft = d.folderName
			d.saveErr = "A folder with that name already exists"
			m.invalidate()
			return
		}
	}
	for _, folder := range d.extraFolders {
		if folder.Kind == d.folderKind && folder.Name != d.folderName && folder.Name == name {
			d.folderDraft = d.folderName
			d.saveErr = "A folder with that name already exists"
			m.invalidate()
			return
		}
	}
	old := d.folderName
	for _, cfg := range d.items {
		if cfg != nil && cfg.Kind == d.folderKind && cfg.Folder != nil && *cfg.Folder == old {
			renamed := name
			cfg.Folder = &renamed
			cfg.Detected = false
		}
	}
	for index := range d.extraFolders {
		if d.extraFolders[index].Kind == d.folderKind && d.extraFolders[index].Name == old {
			d.extraFolders[index].Name = name
		}
	}
	d.folderName = name
	d.folderDraft = name
	d.saveErr = ""
	m.invalidate()
}

// RemoveRunConfigFolder keeps its configurations by moving them back to the
// type's top level, as OpenIDEA's Remove Folder does.
func (m *Model) RemoveRunConfigFolder() {
	d := m.runCfgDlg
	if d == nil || d.folderKind == "" {
		return
	}
	kind, name := d.folderKind, d.folderName
	for _, cfg := range d.items {
		if cfg != nil && cfg.Kind == kind && cfg.Folder != nil && *cfg.Folder == name {
			cfg.Folder = nil
			cfg.Detected = false
		}
	}
	folders := d.extraFolders[:0]
	for _, folder := range d.extraFolders {
		if folder.Kind != kind || folder.Name != name {
			folders = append(folders, folder)
		}
	}
	d.extraFolders = folders
	d.sel = -1
	d.folderKind, d.folderName = "", ""
	d.folderDraft = ""
	d.loadForm()
	m.invalidate()
}

// MoveRunConfig moves the selected configuration among siblings of the same
// type and folder.
func (m *Model) MoveRunConfig(delta int) {
	d := m.runCfgDlg
	if d == nil || delta == 0 || d.selected() == nil {
		return
	}
	d.applyForm()
	current := d.selected()
	siblings := make([]int, 0, len(d.items))
	for index, cfg := range d.items {
		if cfg != nil && cfg.Kind == current.Kind && runConfigFolderOf(cfg) == runConfigFolderOf(current) {
			siblings = append(siblings, index)
		}
	}
	at := -1
	for index, item := range siblings {
		if d.items[item] == current {
			at = index
			break
		}
	}
	target := at + delta
	if at < 0 || target < 0 || target >= len(siblings) {
		return
	}
	left, right := siblings[at], siblings[target]
	d.items[left], d.items[right] = d.items[right], d.items[left]
	d.sel = right
	d.loadForm()
	m.invalidate()
}

func runConfigFolderOf(cfg *openidea.RunConfig) string {
	if cfg == nil || cfg.Folder == nil {
		return ""
	}
	return *cfg.Folder
}

// runConfigDropTarget accepts a dragged configuration on a group or folder.
func (m *Model) runConfigDropTarget(row ui.Element, kind, folder string) {
	drag, ok := ui.Drop[runConfigDrag](row)
	if !ok {
		return
	}
	d := m.runCfgDlg
	if d == nil {
		return
	}
	d.applyForm()
	if d.moveConfigToFolder(drag.ID, kind, folder) {
		m.invalidate()
	}
}

func (d *runConfigDlg) moveConfigToFolder(id, kind, folder string) bool {
	if d == nil {
		return false
	}
	for index, cfg := range d.items {
		if cfg == nil || cfg.ID != id || cfg.Kind != kind {
			continue
		}
		if folder == "" {
			cfg.Folder = nil
		} else {
			name := folder
			cfg.Folder = &name
		}
		cfg.Detected = false
		d.selectConfig(index)
		return true
	}
	return false
}

// runConfigDrag identifies a configuration row while the pointer carries it.
type runConfigDrag struct {
	ID string
}

func (m *Model) runConfigSidebar(c *ui.Context, p *ui.Theme) {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	ui.Column(c).Width(255).Shrink(0).MinHeight(0).
		Border(1, oiPaletteOf(p).border).Radius(4).Background(oiPaletteOf(p).island).
		Children(func() {
			ui.Row(c).Height(34).Shrink(0).Padding(0, 5).Gap(2).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(oiPaletteOf(p).border).Children(func() {
				add := ui.Button(c, "+").Tooltip("Add New Configuration")
				add.Menu(func(menu *ui.Menu) { m.addRunConfigTypeMenu(menu) })
				ui.Button(c, "−").Tooltip("Remove Configuration").Disabled(d.selected() == nil && d.folderKind == "").
					OnClick(m.RemoveRunConfig)
				ui.Button(c, "Copy").Tooltip("Copy Configuration").Disabled(d.selected() == nil).
					OnClick(m.DuplicateRunConfig)
				ui.Button(c, "Folder").Tooltip("Create New Folder").
					Disabled(d.selected() == nil && d.folderKind == "").OnClick(m.NewRunConfigFolder)
				ui.Button(c, "↑").Tooltip("Move Up").Disabled(d.selected() == nil).
					OnClick(func() { m.MoveRunConfig(-1) })
				ui.Button(c, "↓").Tooltip("Move Down").Disabled(d.selected() == nil).
					OnClick(func() { m.MoveRunConfig(1) })
			})
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Padding(4, 0, 4, 4).Children(func() {
					for _, row := range runConfigTreeRows(d) {
						m.runConfigTreeRow(c, p, row)
					}
					if len(d.items) == 0 && len(d.extraFolders) == 0 {
						ui.Text(c, "No run configurations added.").FontSize(12).
							TextColor(p.TextMuted).Padding(8, 10)
					}
				})
			})
			ui.Button(c, "Edit configuration templates…").OnClick(m.OpenRunConfigTemplates)
		})
}

func (m *Model) runConfigTreeRow(c *ui.Context, p *ui.Theme, item runConfigTreeRow) {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	paddingLeft := float32(6 + item.Depth*14)
	row := ui.Row(c.Key("run-config-tree:"+item.Key)).Height(27).Padding(0, paddingLeft, 0, 6).
		Gap(5).AlignItems(ui.Center).Radius(3).Cursor(ui.CursorPointer)
	selected := false
	switch item.Kind {
	case runConfigGroupRow:
	case runConfigFolderRow:
		selected = d.folderKind == item.KindID && d.folderName == item.Folder
	case runConfigConfigRow:
		selected = d.folderKind == "" && d.sel == item.Index
	}
	if row.Hovered() || selected {
		row.Background(oiPaletteOf(p).hover)
	}
	row.Children(func() {
		switch item.Kind {
		case runConfigGroupRow:
			collapsed := d.collapsedGroups[item.Key]
			if collapsed {
				ui.Text(c, "›").FontSize(14).TextColor(p.TextMuted)
			} else {
				ui.Text(c, "⌄").FontSize(14).TextColor(p.TextMuted)
			}
			ui.Text(c, item.Label).FontSize(12).Bold().TextColor(p.Text).SingleLine().Ellipsis("…")
		case runConfigFolderRow:
			collapsed := d.collapsedFolder[item.Key]
			if collapsed {
				ui.Text(c, "›").FontSize(14).TextColor(p.TextMuted)
			} else {
				ui.Text(c, "⌄").FontSize(14).TextColor(p.TextMuted)
			}
			ui.Text(c, item.Label).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
		case runConfigConfigRow:
			cfg := d.items[item.Index]
			ui.Text(c, "●").FontSize(8).TextColor(p.Accent)
			ui.Text(c, item.Label).FontSize(12).TextColor(p.Text).Grow(1).
				MinWidth(0).SingleLine().Ellipsis("…")
			if cfg != nil && !cfg.Detected {
				label := "user"
				if cfg.Local {
					label = "local"
				}
				ui.Text(c, label).FontSize(10).TextColor(p.TextMuted).SingleLine()
			}
		}
	})
	switch item.Kind {
	case runConfigGroupRow:
		if row.Clicked() {
			d.toggleGroup(item.Key)
			m.invalidate()
		}
		m.runConfigDropTarget(row, item.KindID, "")
	case runConfigFolderRow:
		if row.Clicked() {
			d.selectFolder(item.KindID, item.Folder)
			m.invalidate()
		}
		if row.DoubleClicked() {
			d.toggleFolder(item.Key)
			m.invalidate()
		}
		row.Menu(func(menu *ui.Menu) {
			if menu.Item("Remove Folder").Chosen() {
				d.selectFolder(item.KindID, item.Folder)
				m.RemoveRunConfigFolder()
			}
		})
		m.runConfigDropTarget(row, item.KindID, item.Folder)
	case runConfigConfigRow:
		if row.Clicked() {
			d.selectConfig(item.Index)
			m.invalidate()
		}
		row.Drag(runConfigDrag{ID: d.items[item.Index].ID})
	}
}
