package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/app"
)

const maxNativeProjectWindows = 12

type nativeProjectWindow struct {
	id         string
	root       string
	group      int
	model      *app.Model
	win        *mygo.Window
	log        *os.File
	serverHome string
}

type nativeWindowHost struct {
	base        app.Config
	headless    bool
	serverErr   error
	startupErrs []error
	startedAt   time.Time
	firstFrame  atomic.Int64

	windows []*nativeProjectWindow
	active  *app.Model
	nextID  int

	workspacePath  string
	restoreWindows []app.ProjectWindowRecord
	restoreActive  string
	restoring      bool
	nextGroup      int
	selectedWindow map[int]string
	workspaceErr   error

	closingProjectTabs bool
	preparingQuit      bool
	shuttingDown       bool
	allowQuit          bool
	relaunch           bool
	immediate          bool
	prepareOrder       []*nativeProjectWindow
	prepareIndex       int
	saveChoices        map[*app.Model]bool
	shutdownLeft       int
}

func newNativeWindowHost(
	base app.Config,
	headless bool,
	serverErr error,
	startupErrs []error,
	startedAt time.Time,
) *nativeWindowHost {
	return &nativeWindowHost{
		base:           base,
		headless:       headless,
		serverErr:      serverErr,
		startupErrs:    append([]error(nil), startupErrs...),
		startedAt:      startedAt,
		workspacePath:  app.ProjectWindowWorkspacePath(base.SessionPath),
		nextID:         1,
		nextGroup:      1,
		selectedWindow: map[int]string{},
	}
}

func (h *nativeWindowHost) addInitialWindow(root string, files []string) *nativeProjectWindow {
	if h.restoring {
		for _, record := range h.restoreWindows {
			if record.ID != "main" {
				continue
			}
			root, files = record.Root, nil
			entry := h.addWindow("main", root, files, root == "")
			if entry != nil {
				entry.group = record.Group
			}
			return entry
		}
		root, files = "", nil
		return h.addWindow("main", root, files, true)
	}
	return h.addWindow("main", root, files, false)
}

func (h *nativeWindowHost) addWindow(id, root string, files []string, fresh bool) *nativeProjectWindow {
	if h.preparingQuit || h.shuttingDown ||
		len(h.windows) >= maxNativeProjectWindows || !mygo.App.IsReady() {
		return nil
	}
	if id == "" {
		for {
			h.nextID++
			id = fmt.Sprintf("window-%d", h.nextID)
			if h.windowByID(id) == nil {
				break
			}
		}
	}
	cfg := h.base
	var logFile *os.File
	if id == "main" {
		logFile = h.base.Log
	} else {
		dir := filepath.Join(h.base.DataDir, "windows", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil
		}
		cfg = secondaryProjectWindowConfig(h.base, dir, fresh)
		seedWindowSession(h.base.SessionPath, cfg.SessionPath)
		logFile = openLog(filepath.Join(dir, "openidea-server.log"))
		cfg.Log = logFile
	}
	cfg.FreshSession = fresh

	model := app.New(cfg)
	model.SetWorkspaceHost(h)
	if root != "" {
		model.OpenAtStartup(root)
		model.OpenFilesAtStartup(append([]string(nil), files...))
	} else if len(files) > 0 {
		model.OpenFilesAtStartup(append([]string(nil), files...))
	}
	if h.serverErr != nil {
		model.Notify(h.serverErr.Error())
	}
	for _, err := range h.startupErrs {
		model.Notify(err.Error())
	}
	content := ui.View(func(c *ui.Context) {
		if h.firstFrame.Load() == 0 {
			h.firstFrame.CompareAndSwap(0, time.Since(h.startedAt).Milliseconds())
		}
		model.View(c)
	})
	win := mygo.NewWindow(goideaWindowOptions(id, h.headless, content))
	if win.IsDestroyed() {
		if logFile != nil && id != "main" {
			_ = logFile.Close()
		}
		return nil
	}
	entry := &nativeProjectWindow{
		id:         id,
		root:       root,
		model:      model,
		win:        win,
		log:        logFile,
		serverHome: cfg.Home,
	}
	h.windows = append(h.windows, entry)
	win.OnClosed(func() { h.windowClosed(entry) })
	model.Attach(win)
	h.WindowFocused(model)
	model.Start()
	return entry
}

// OpenIDEA runs every project window against one application data root. Keep
// each window's session and log separate, but share the backend's installed
// tools and global settings so JDT is downloaded once instead of once per
// project window.
func secondaryProjectWindowConfig(base app.Config, dir string, fresh bool) app.Config {
	cfg := base
	cfg.SessionPath = filepath.Join(dir, "session.json")
	cfg.FreshSession = fresh
	return cfg
}

// migrateLegacyWindowToolData moves tools installed by older GoIDEA versions
// from each window's private server home into the shared OpenIDEA-style home.
// Existing shared entries win, so startup never replaces a newer installation.
func migrateLegacyWindowToolData(dataDir, sharedHome string) error {
	windowRoot := filepath.Join(dataDir, "windows")
	entries, err := os.ReadDir(windowRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if entry.IsDir() {
			legacyData := filepath.Join(windowRoot, entry.Name(), "openidea-server", "data")
			for _, name := range []string{"tools", "jdks"} {
				if err := mergeLegacyDirectory(
					filepath.Join(legacyData, name),
					filepath.Join(sharedHome, "data", name),
				); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}

func mergeLegacyDirectory(source, destination string) error {
	entries, err := os.ReadDir(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(destination, entry.Name())
		info, err := os.Lstat(to)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Rename(from, to); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if info.IsDir() && entry.IsDir() {
			if err := mergeLegacyDirectory(from, to); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func goideaWindowOptions(id string, hidden bool, content mygo.Content) mygo.WindowOptions {
	return mygo.WindowOptions{
		Title:     "GoIDEA",
		Width:     1280,
		Height:    820,
		MinWidth:  900,
		MinHeight: 560,
		StateKey:  id,
		Hidden:    hidden,
		// GoIDEA draws OpenIDEA's 40 DIP workbench bar in its content. Hiding
		// AppKit's title bar removes the extra native row while keeping the
		// traffic lights over the start of that bar.
		TitleBarStyle:        mygo.TitleBarHidden,
		TitleBarHeight:       40,
		TrafficLightPosition: &mygo.Point{X: 14, Y: 13},
		Content:              content,
	}
}

func seedWindowSession(source, target string) {
	if source == "" || target == "" {
		return
	}
	if _, err := os.Stat(target); err == nil {
		return
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(target, raw, 0o600)
}

// prepareWorkspaceRestore keeps the saved order and group membership until
// the main and secondary windows have been created.
func (h *nativeWindowHost) prepareWorkspaceRestore(
	workspace app.ProjectWindowWorkspace,
) bool {
	if len(workspace.Windows) == 0 {
		return false
	}
	records := make([]app.ProjectWindowRecord, 0, len(workspace.Windows))
	for _, record := range workspace.Windows {
		if record.Root != "" {
			info, err := os.Stat(record.Root)
			if err != nil || !info.IsDir() {
				continue
			}
		}
		records = append(records, record)
	}
	hasMain := false
	for _, record := range records {
		hasMain = hasMain || record.ID == "main"
	}
	if !hasMain {
		records = append([]app.ProjectWindowRecord{{ID: "main"}}, records...)
	}
	if len(records) == 0 {
		return false
	}
	h.restoreWindows = records
	h.restoreActive = workspace.Active
	h.restoring = true
	return true
}

// restoreAdditionalWindows recreates every secondary window after the main
// window exists, then installs groups without touching their backends.
func (h *nativeWindowHost) restoreAdditionalWindows() {
	if !h.restoring {
		return
	}
	for _, record := range h.restoreWindows {
		if record.ID == "main" {
			continue
		}
		entry := h.addWindow(record.ID, record.Root, nil, record.Root == "")
		if entry != nil {
			entry.group = record.Group
		}
	}

	ordered := make([]*nativeProjectWindow, 0, len(h.windows))
	for _, record := range h.restoreWindows {
		if entry := h.windowByID(record.ID); entry != nil {
			ordered = append(ordered, entry)
		}
	}
	h.windows = ordered

	counts := map[int]int{}
	for _, entry := range h.windows {
		if entry.group != 0 {
			counts[entry.group]++
		}
	}
	for group, count := range counts {
		if group >= h.nextGroup {
			h.nextGroup = group + 1
		}
		if count < 2 {
			for _, entry := range h.windows {
				if entry.group == group {
					entry.group = 0
				}
			}
			delete(h.selectedWindow, group)
			continue
		}
		selected := h.windowByID(h.restoreActive)
		if selected == nil || selected.group != group {
			selected = h.firstWindowInGroup(group)
		}
		if selected != nil {
			h.selectedWindow[group] = selected.id
		}
	}

	h.restoring = false
	for _, entry := range h.windows {
		if entry.group == 0 {
			continue
		}
		if h.selectedWindow[entry.group] == entry.id {
			if !h.headless {
				entry.win.Show()
			}
		} else {
			entry.win.Hide()
		}
	}
	if selected := h.windowByID(h.restoreActive); selected != nil {
		h.WindowFocused(selected.model)
		if !h.headless {
			selected.win.Show()
			selected.win.Focus()
		}
	}
	for _, entry := range h.windows {
		entry.model.RefreshWorkspaceWindows()
	}
}

func (h *nativeWindowHost) windowByID(id string) *nativeProjectWindow {
	for _, entry := range h.windows {
		if entry.id == id {
			return entry
		}
	}
	return nil
}

func (h *nativeWindowHost) windowForModel(model *app.Model) *nativeProjectWindow {
	for _, entry := range h.windows {
		if entry.model == model {
			return entry
		}
	}
	return nil
}

func (h *nativeWindowHost) firstWindowInGroup(group int) *nativeProjectWindow {
	for _, entry := range h.windows {
		if entry.group == group {
			return entry
		}
	}
	return nil
}

func (h *nativeWindowHost) groupedWindows(group int) []*nativeProjectWindow {
	if group == 0 {
		return nil
	}
	var windows []*nativeProjectWindow
	for _, entry := range h.windows {
		if entry.group == group {
			windows = append(windows, entry)
		}
	}
	return windows
}

func (h *nativeWindowHost) projectWindows() []*nativeProjectWindow {
	var windows []*nativeProjectWindow
	for _, entry := range h.windows {
		if entry.model == nil || entry.model.Root() == "" {
			continue
		}
		entry.root = entry.model.Root()
		windows = append(windows, entry)
	}
	return windows
}

func (h *nativeWindowHost) showGroupWindow(entry *nativeProjectWindow) {
	if entry == nil || entry.group == 0 {
		return
	}
	previous := h.windowByID(h.selectedWindow[entry.group])
	if !h.headless && previous != nil && previous != entry {
		previous.win.Hide()
	}
	h.selectedWindow[entry.group] = entry.id
	if !h.headless {
		entry.win.Show()
		entry.win.Focus()
	}
	h.WindowFocused(entry.model)
	for _, member := range h.groupedWindows(entry.group) {
		member.model.RefreshWorkspaceWindows()
	}
	h.saveWorkspace()
}

func (h *nativeWindowHost) MergeProjectWindows(anchor *app.Model) bool {
	projectWindows := h.projectWindows()
	if h.closingProjectTabs || h.preparingQuit || h.shuttingDown || anchor == nil || len(projectWindows) < 2 {
		return false
	}
	anchorWindow := h.windowForModel(anchor)
	if anchorWindow == nil || anchorWindow.model.Root() == "" {
		return false
	}
	group := h.nextGroup
	h.nextGroup++
	for _, entry := range projectWindows {
		delete(h.selectedWindow, entry.group)
		entry.group = group
	}
	h.selectedWindow[group] = anchorWindow.id
	for _, entry := range projectWindows {
		if !h.headless && entry != anchorWindow {
			entry.win.Hide()
		}
	}
	if !h.headless {
		anchorWindow.win.Show()
		anchorWindow.win.Focus()
	}
	h.WindowFocused(anchor)
	for _, entry := range projectWindows {
		entry.model.RefreshWorkspaceWindows()
	}
	h.saveWorkspace()
	return true
}

// DetachWorkspaceWindow reveals the same live native window outside the group.
func (h *nativeWindowHost) DetachWorkspaceWindow(model *app.Model) bool {
	entry := h.windowForModel(model)
	if h.closingProjectTabs || h.preparingQuit || h.shuttingDown || entry == nil || entry.group == 0 || h.selectedWindow[entry.group] != entry.id {
		return false
	}
	group := entry.group
	members := h.groupedWindows(group)
	if len(members) < 2 {
		return false
	}
	next := members[0]
	for i, w := range members {
		if w == entry {
			if i+1 < len(members) {
				next = members[i+1]
			} else {
				next = members[i-1]
			}
			break
		}
	}
	entry.group = 0
	if len(members) == 2 {
		next.group = 0
		delete(h.selectedWindow, group)
	} else {
		h.selectedWindow[group] = next.id
	}
	if !h.headless {
		next.win.Show()
		entry.win.Show()
		entry.win.Focus()
	}
	h.WindowFocused(model)
	for _, w := range members {
		w.model.RefreshWorkspaceWindows()
	}
	h.saveWorkspace()
	return true
}

func (h *nativeWindowHost) ReorderWorkspaceWindows(model *app.Model, ids []string) bool {
	entry := h.windowForModel(model)
	if h.closingProjectTabs || h.preparingQuit || h.shuttingDown || entry == nil || entry.group == 0 {
		return false
	}
	members := h.groupedWindows(entry.group)
	if len(ids) != len(members) {
		return false
	}
	byID := make(map[string]*nativeProjectWindow, len(members))
	for _, w := range members {
		byID[w.id] = w
	}
	ordered := make([]*nativeProjectWindow, 0, len(ids))
	for _, id := range ids {
		w := byID[id]
		if w == nil {
			return false
		}
		ordered = append(ordered, w)
		delete(byID, id)
	}
	i := 0
	for slot, w := range h.windows {
		if w.group == entry.group {
			h.windows[slot] = ordered[i]
			i++
		}
	}
	for _, w := range ordered {
		w.model.RefreshWorkspaceWindows()
	}
	h.saveWorkspace()
	return true
}

// CloseWorkspaceWindows presents each project's own guard before advancing.
func (h *nativeWindowHost) CloseWorkspaceWindows(model *app.Model, keep string, ids []string) {
	entry := h.windowForModel(model)
	if h.closingProjectTabs || h.preparingQuit || h.shuttingDown || entry == nil || entry.group == 0 || len(ids) == 0 {
		return
	}
	group := entry.group
	if keep != "" {
		survivor := h.windowByID(keep)
		if survivor == nil || survivor.group != group {
			return
		}
	} else if len(ids) != 1 {
		return
	}
	pending := make([]*nativeProjectWindow, 0, len(ids))
	for _, id := range ids {
		target := h.windowByID(id)
		if target == nil || target.group != group || target.id == keep || slices.Contains(pending, target) {
			return
		}
		pending = append(pending, target)
	}
	h.closingProjectTabs = true
	var next func(bool)
	next = func(ok bool) {
		if !ok || h.shuttingDown || h.preparingQuit {
			h.closingProjectTabs = false
			return
		}
		if len(pending) == 0 {
			h.closingProjectTabs = false
			if w := h.windowByID(keep); w != nil && w.group == group {
				h.showGroupWindow(w)
			}
			return
		}
		target := pending[0]
		pending = pending[1:]
		if h.windowByID(target.id) != target || target.group != group {
			h.closingProjectTabs = false
			return
		}
		h.showGroupWindow(target)
		target.model.CloseWorkspaceTab(next)
	}
	next(true)
}

func (h *nativeWindowHost) FocusWorkspaceWindow(id string) bool {
	target := h.windowByID(id)
	active := h.windowForModel(h.active)
	if target == nil || active == nil || target.group == 0 || target.group != active.group {
		return false
	}
	h.showGroupWindow(target)
	return true
}

func (h *nativeWindowHost) WorkspaceWindows(model *app.Model) []app.WorkspaceWindow {
	current := h.windowForModel(model)
	if current == nil || current.group == 0 {
		return nil
	}
	windows := make([]app.WorkspaceWindow, 0, len(h.windows))
	for _, entry := range h.windows {
		if entry.group != current.group {
			continue
		}
		selected := entry.group != 0 && h.selectedWindow[entry.group] == entry.id
		windows = append(windows, app.WorkspaceWindow{
			ID:       entry.id,
			Title:    entry.model.WorkspaceTitle(),
			Root:     entry.model.Root(),
			Branch:   entry.model.WorkspaceBranch(),
			Group:    entry.group,
			Selected: selected,
			Dirty:    entry.model.WorkspaceDirty(),
		})
	}
	return windows
}

func (h *nativeWindowHost) OpenProjects(_ *app.Model) []app.WorkspaceWindow {
	windows := make([]app.WorkspaceWindow, 0, len(h.windows))
	for _, entry := range h.windows {
		if entry == nil || entry.model == nil || entry.model.Root() == "" {
			continue
		}
		selected := entry.model == h.active
		windows = append(windows, app.WorkspaceWindow{
			ID: entry.id, Title: entry.model.WorkspaceTitle(),
			Root: entry.model.Root(), Branch: entry.model.WorkspaceBranch(),
			Selected: selected,
		})
	}
	return windows
}

func (h *nativeWindowHost) FocusOpenProject(_ *app.Model, path string) bool {
	clean := filepath.Clean(path)
	for _, entry := range h.windows {
		if entry == nil || entry.model == nil || filepath.Clean(entry.model.Root()) != clean {
			continue
		}
		if entry.group != 0 {
			h.showGroupWindow(entry)
			return true
		}
		if !h.headless {
			entry.win.Show()
			entry.win.Focus()
		}
		h.WindowFocused(entry.model)
		for _, member := range h.groupedWindows(entry.group) {
			member.model.RefreshWorkspaceWindows()
		}
		return true
	}
	return false
}

func (h *nativeWindowHost) WindowStateChanged(model *app.Model) {
	if h.restoring || h.shuttingDown {
		return
	}
	entry := h.windowForModel(model)
	if entry == nil {
		return
	}
	root := model.Root()
	changed := entry.root != root
	entry.root = root
	if changed || entry.group != 0 {
		h.saveWorkspace()
	}
}

func (h *nativeWindowHost) workspaceSnapshot() app.ProjectWindowWorkspace {
	workspace := app.ProjectWindowWorkspace{
		Version: 1,
		Windows: make([]app.ProjectWindowRecord, 0, len(h.windows)),
	}
	for _, entry := range h.windows {
		root := entry.model.Root()
		entry.root = root
		workspace.Windows = append(workspace.Windows, app.ProjectWindowRecord{
			ID:    entry.id,
			Root:  root,
			Group: entry.group,
		})
		if entry.model == h.active {
			workspace.Active = entry.id
		}
	}
	if workspace.Active == "" && len(workspace.Windows) > 0 {
		workspace.Active = workspace.Windows[0].ID
	}
	return workspace
}

func (h *nativeWindowHost) saveWorkspace() {
	if h.workspacePath == "" || h.restoring || h.allowQuit {
		return
	}
	workspace := h.workspaceSnapshot()
	if len(workspace.Windows) == 0 {
		return
	}
	h.workspaceErr = workspace.Save(h.workspacePath)
}

func (h *nativeWindowHost) OpenProjectInNewWindow(path string, files []string) bool {
	if filepath.Clean(path) == "." || filepath.Clean(path) == "" {
		return false
	}
	entry := h.addWindow("", filepath.Clean(path), files, false)
	if entry == nil {
		return false
	}
	h.saveWorkspace()
	return true
}

func (h *nativeWindowHost) OpenNewWindow() bool {
	entry := h.addWindow("", "", nil, true)
	if entry == nil {
		return false
	}
	h.saveWorkspace()
	return true
}

// DetachTabToNewWindow restores a moved file in a fresh backend. The source
// tab is removed only after that backend has accepted the full unsaved text.
func (h *nativeWindowHost) DetachTabToNewWindow(
	snapshot app.TabSnapshot,
	done func(bool) bool,
) bool {
	target := h.addWindow("", snapshot.Root, []string{snapshot.Path}, false)
	if target == nil {
		return false
	}
	h.saveWorkspace()
	go func() {
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			var opened, failed bool
			mygo.RunOnMain(func() {
				_, opened = target.model.CaptureWindowTab(snapshot.Path)
				failed = target.model.Phase() == "failed"
			})
			if opened {
				break
			}
			if failed {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		var opened bool
		mygo.RunOnMain(func() {
			_, opened = target.model.CaptureWindowTab(snapshot.Path)
		})
		if !opened {
			mygo.RunOnMain(func() {
				done(false)
				h.closeTransferredWindow(target)
			})
			return
		}
		mygo.RunOnMain(func() {
			target.model.RestoreWindowTab(snapshot, func(err error) {
				if err != nil {
					done(false)
					h.closeTransferredWindow(target)
					return
				}
				if !done(true) {
					h.closeTransferredWindow(target)
					return
				}
				h.saveWorkspace()
			})
		})
	}()
	return true
}

func (h *nativeWindowHost) closeTransferredWindow(entry *nativeProjectWindow) {
	if entry == nil || entry.win.IsDestroyed() {
		return
	}
	entry.model.CloseWindowImmediately()
}

func (h *nativeWindowHost) FocusProjectWindow(previous bool) bool {
	candidates := h.focusCandidates()
	if len(candidates) < 2 {
		return false
	}
	index := -1
	for i, window := range candidates {
		if window.model == h.active {
			index = i
			break
		}
	}
	if index < 0 {
		index = 0
	}
	if previous {
		index = (index + len(candidates) - 1) % len(candidates)
	} else {
		index = (index + 1) % len(candidates)
	}
	if candidates[index].group != 0 {
		h.showGroupWindow(candidates[index])
	} else {
		h.WindowFocused(candidates[index].model)
		if !h.headless {
			candidates[index].win.Focus()
		}
	}
	return true
}

func (h *nativeWindowHost) focusCandidates() []*nativeProjectWindow {
	candidates := make([]*nativeProjectWindow, 0, len(h.windows))
	seenGroups := map[int]bool{}
	for _, entry := range h.windows {
		if entry.group == 0 {
			candidates = append(candidates, entry)
			continue
		}
		if seenGroups[entry.group] {
			continue
		}
		seenGroups[entry.group] = true
		selected := h.windowByID(h.selectedWindow[entry.group])
		if selected == nil || selected.group != entry.group {
			selected = h.firstWindowInGroup(entry.group)
		}
		if selected != nil {
			candidates = append(candidates, selected)
		}
	}
	return candidates
}

func (h *nativeWindowHost) WindowCount() int {
	return len(h.windows)
}

func (h *nativeWindowHost) WindowFocused(model *app.Model) {
	h.active = model
	if mygo.App.IsReady() {
		mygo.App.SetMenu(model.MainMenu())
	}
}

func (h *nativeWindowHost) WindowMenuChanged(model *app.Model) {
	mygo.RunOnMain(func() {
		if model != h.active || !mygo.App.IsReady() {
			return
		}
		mygo.App.SetMenu(model.MainMenu())
	})
}

func (h *nativeWindowHost) windowClosed(entry *nativeProjectWindow) {
	var next *nativeProjectWindow
	if entry.group != 0 && !h.shuttingDown {
		members := h.groupedWindows(entry.group)
		for i, member := range members {
			if member == entry {
				if i+1 < len(members) {
					next = members[i+1]
				} else if i > 0 {
					next = members[i-1]
				}
				break
			}
		}
	}
	for i, window := range h.windows {
		if window != entry {
			continue
		}
		h.windows = append(h.windows[:i], h.windows[i+1:]...)
		break
	}
	if entry.log != nil && entry.id != "main" {
		_ = entry.log.Close()
	}
	if h.active == entry.model {
		h.active = nil
		if next != nil && !next.win.IsDestroyed() {
			h.active = next.model
			if !h.headless {
				next.win.Show()
				next.win.Focus()
			}
		} else if len(h.windows) > 0 {
			h.active = h.windows[len(h.windows)-1].model
			if h.active != nil {
				mygo.App.SetMenu(h.active.MainMenu())
			}
		}
	}
	if entry.group != 0 {
		members := h.groupedWindows(entry.group)
		if len(members) < 2 {
			for _, member := range members {
				member.group = 0
			}
			delete(h.selectedWindow, entry.group)
		} else if h.selectedWindow[entry.group] == entry.id {
			h.selectedWindow[entry.group] = members[0].id
		}
		for _, member := range members {
			member.model.RefreshWorkspaceWindows()
		}
	}
	if h.active != nil {
		if group := h.activeGroup(); group != 0 &&
			h.selectedWindow[group] != h.activeID() {
			h.selectedWindow[group] = h.activeID()
		}
		if mygo.App.IsReady() {
			mygo.App.SetMenu(h.active.MainMenu())
		}
	}
	if !h.shuttingDown {
		h.saveWorkspace()
	}
}

func (h *nativeWindowHost) activeGroup() int {
	if entry := h.windowForModel(h.active); entry != nil {
		return entry.group
	}
	return 0
}

func (h *nativeWindowHost) activeID() string {
	if entry := h.windowForModel(h.active); entry != nil {
		return entry.id
	}
	return ""
}

func (h *nativeWindowHost) RequestApplicationQuit() {
	h.beginQuit(false)
}

func (h *nativeWindowHost) RequestRelaunch() {
	h.beginQuit(true)
}

// BeforeQuit is the single application-level gate. MyGo calls it before any
// window is destroyed, which lets every model finish its own prompts first.
func (h *nativeWindowHost) BeforeQuit(event *mygo.QuitEvent) {
	if h.allowQuit {
		return
	}
	event.PreventDefault()
	h.beginQuit(h.relaunch)
}

func (h *nativeWindowHost) beginQuit(relaunch bool) {
	if h.allowQuit {
		return
	}
	if relaunch {
		h.relaunch = true
	}
	if h.closingProjectTabs || h.preparingQuit || h.shuttingDown {
		return
	}
	if len(h.windows) == 0 {
		h.finishQuit()
		return
	}
	h.preparingQuit = true
	h.prepareOrder = h.quitOrder()
	h.prepareIndex = 0
	h.saveChoices = make(map[*app.Model]bool, len(h.prepareOrder))
	h.prepareNext()
}

func (h *nativeWindowHost) quitOrder() []*nativeProjectWindow {
	order := make([]*nativeProjectWindow, 0, len(h.windows))
	for _, window := range h.windows {
		if window.model == h.active {
			order = append(order, window)
			break
		}
	}
	for _, window := range h.windows {
		if len(order) > 0 && window == order[0] {
			continue
		}
		order = append(order, window)
	}
	return order
}

func (h *nativeWindowHost) prepareNext() {
	if h.prepareIndex >= len(h.prepareOrder) {
		h.confirmQuit()
		return
	}
	entry := h.prepareOrder[h.prepareIndex]
	entry.model.PrepareApplicationQuit(func(save, ok bool) {
		if !ok {
			h.cancelQuit()
			return
		}
		h.saveChoices[entry.model] = save
		h.prepareIndex++
		h.prepareNext()
	})
}

func (h *nativeWindowHost) confirmQuit() {
	model := h.active
	if model == nil && len(h.prepareOrder) > 0 {
		model = h.prepareOrder[0].model
	}
	if model == nil {
		h.commitQuit()
		return
	}
	model.ConfirmApplicationExit(func(ok bool) {
		if !ok {
			h.cancelQuit()
			return
		}
		h.commitQuit()
	})
}

func (h *nativeWindowHost) cancelQuit() {
	for _, entry := range h.prepareOrder {
		entry.model.CancelApplicationQuitPreparation()
	}
	h.preparingQuit = false
	h.prepareOrder = nil
	h.prepareIndex = 0
	h.saveChoices = nil
	h.relaunch = false
}

func (h *nativeWindowHost) commitQuit() {
	h.saveWorkspace()
	h.preparingQuit = false
	h.shuttingDown = true
	h.shutdownLeft = len(h.windows)
	if h.shutdownLeft == 0 {
		h.finishQuit()
		return
	}
	for _, entry := range append([]*nativeProjectWindow(nil), h.windows...) {
		save := h.saveChoices[entry.model]
		entry.model.ShutdownForApplicationQuit(save, h.windowShutdownDone)
	}
}

func (h *nativeWindowHost) windowShutdownDone() {
	h.shutdownLeft--
	if h.shutdownLeft > 0 {
		return
	}
	h.finishQuit()
}

func (h *nativeWindowHost) finishQuit() {
	h.shuttingDown = false
	h.allowQuit = true
	if h.relaunch {
		mygo.App.Relaunch()
		return
	}
	mygo.App.Quit()
}

func (h *nativeWindowHost) QuitImmediately() {
	if h.shuttingDown && h.immediate {
		return
	}
	h.immediate = true
	h.shuttingDown = true
	h.shutdownLeft = len(h.windows)
	if h.shutdownLeft == 0 {
		h.finishQuit()
		return
	}
	for _, entry := range append([]*nativeProjectWindow(nil), h.windows...) {
		entry.model.ShutdownForApplicationQuit(false, h.windowShutdownDone)
	}
}
