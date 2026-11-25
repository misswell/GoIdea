package app

// The action registry after OpenIDEA's platform/actions: one record per user
// invocation, with the same Run behind the menu, palette, Search Everywhere,
// toolbar, context menus, tool window headers and keymap. Registration is
// explicit so features and plugins contribute actions instead of the palette
// switching on ids.

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// Action is one user invocation.
type Action struct {
	ID          string
	Title       string
	Category    string
	Description string
	Keywords    string
	Icon        *ui.SVG
	// When and KeyWhen gate the action by context; Enabled the action's own
	// state. A missing function means "always".
	When    func(m *Model) bool
	KeyWhen func(m *Model) bool
	Enabled func(m *Model) bool
	Toggled func(m *Model) bool
	Running func(m *Model) bool
	Run     func(m *Model)
	// RunWithArg is OpenIDEA's Action.run({arg}) path. Actions that ignore
	// their argument keep Run and are still callable with one.
	RunWithArg func(m *Model, arg any)
	// Builtin marks an action registered from the shipped command list; the
	// keymap's defaults are only known for those.
	Builtin bool
}

// usable reports the action's availability in the current context.
func (a *Action) usable(m *Model) bool {
	if a.When != nil && !a.When(m) {
		return false
	}
	if a.Enabled != nil && !a.Enabled(m) {
		return false
	}
	return true
}

// actionRegistry holds the registered actions in registration order.
type actionRegistry struct {
	byID map[string]*Action
	list []*Action
}

var actions = actionRegistry{byID: map[string]*Action{}}

// actionAliases keeps the upstream action ids callable without adding duplicate
// rows to the command palette. This matters for keymaps, plugin contributions
// and automation written against OpenIDEA: the same visible action can be
// invoked by its OpenIDEA id while GoIDEA keeps its historical id for state.
var actionAliases = map[string]string{
	"debug.startConfig":                   "run.debug",
	"debug.attachToProcess":               "debug.attach",
	"debug.toggleBreakpoint":              "run.breakpoint",
	"debug.resume":                        "run.continue",
	"debug.pause":                         "run.pause",
	"debug.stepOver":                      "run.stepOver",
	"debug.stepInto":                      "run.stepInto",
	"debug.stepOut":                       "run.stepOut",
	"debug.runToCursor":                   "run.runToCursor",
	"debug.forceRunToCursor":              "run.forceRunToCursor",
	"debug.showToolWindow":                "view.debug",
	"run.stopProcess":                     "run.stop",
	"run.debugConfig":                     "run.debug",
	"run.run":                             "run.start",
	"navigate.selectIn":                   "navigate.selectInProjectView",
	"navigate.toggleBookmark":             "bookmarks.toggle",
	"navigate.showBookmarks":              "bookmarks.show",
	"navigate.nextBookmark":               "bookmarks.next",
	"navigate.prevBookmark":               "bookmarks.prev",
	"navigate.toggleBookmarkWithMnemonic": "bookmarks.mnemonic",
	"bookmarks.toggleFileBookmark":        "bookmarks.file",
	"find.usages":                         "code.usages",
	"find.showUsages":                     "code.showUsages",
	"find.showUsageResults":               "code.showUsageResults",
	"code.showIntentions":                 "code.intentions",
	"code.reformat":                       "code.format",
	"refactor.rename":                     "code.rename",
	"editor.closeAllTabs":                 "editor.closeAll",
	"editor.closeOtherTabs":               "editor.closeOthers",
	"editor.closeTab":                     "file.close",
	"editor.formatDocument":               "code.format",
	"editor.prevSplitter":                 "editor.previousSplitter",
	"file.closeProject":                   "project.close",
	"project.new":                         "file.newProject",
	"file.reloadFromDisk":                 "project.reloadFromDisk",
	"run.trust":                           "project.trust",
	"toolwindow.hideAll":                  "view.hideAll",
	"toolwindow.hideActive":               "view.hideActive",
	"toolwindow.jumpToLast":               "view.jumpLast",
	"toolwindow.maximize":                 "view.maximize",
	"toolwindow.project":                  "view.project",
	"toolwindow.bookmarks":                "bookmarks.window",
	"toolwindow.find":                     "view.find",
	"toolwindow.run":                      "view.run",
	"toolwindow.debug":                    "view.debug",
	"toolwindow.problems":                 "view.problems",
	"toolwindow.structure":                "view.structure",
	"toolwindow.git":                      "vcs.show",
	"toolwindow.commit":                   "view.commit",
	"toolwindow.terminal":                 "view.terminal",
	"toolwindow.resizeLeft":               "view.resizeLeft",
	"toolwindow.resizeRight":              "view.resizeRight",
	"toolwindow.resizeUp":                 "view.resizeUp",
	"toolwindow.resizeDown":               "view.resizeDown",
	"view.resetLayout":                    "layout.restoreDefault",
	"terminal.clear":                      "term.clear",
	"terminal.closeSession":               "term.close",
	"terminal.find":                       "term.find",
	"terminal.new":                        "term.new",
	"terminal.unsplit":                    "term.unsplit",
	"services.show":                       "view.services",
	"toolwindow.services":                 "view.services",
	"services.addService":                 "run.services.addType",
	"navigate.nextError":                  "problems.next",
	"navigate.prevError":                  "problems.prev",
	"build.maven":                         "view.maven",
}

// registerAction adds an action. An empty or duplicate id is an error, as
// upstream's registry treats it; a feature registers each action once.
func registerAction(a *Action) error {
	return actions.register(a)
}

func (r *actionRegistry) register(a *Action) error {
	if a == nil || a.ID == "" {
		return fmt.Errorf("action needs an id")
	}
	if a.Run == nil && a.RunWithArg == nil {
		return fmt.Errorf("action %q needs a run function", a.ID)
	}
	if _, ok := r.byID[a.ID]; ok {
		return fmt.Errorf("action %q is already registered", a.ID)
	}
	r.byID[a.ID] = a
	r.list = append(r.list, a)
	return nil
}

// unregisterAction removes an action, as a plugin's teardown does. It reports
// whether the id was registered.
func unregisterAction(id string) bool {
	return actions.unregister(id)
}

func (r *actionRegistry) unregister(id string) bool {
	if _, ok := r.byID[id]; !ok {
		return false
	}
	delete(r.byID, id)
	for i, a := range r.list {
		if a.ID == id {
			r.list = append(r.list[:i], r.list[i+1:]...)
			break
		}
	}
	return true
}

func actionGet(id string) (*Action, bool) {
	id = canonicalActionID(id)
	a, ok := actions.byID[id]
	if ok {
		return a, true
	}
	if a, ok := dynamicActionGet(id); ok {
		return a, true
	}
	return editorSchemeActionGet(id)
}

func actionGetForModel(m *Model, id string) (*Action, bool) {
	a, ok := actionGet(id)
	if ok {
		return a, true
	}
	if a, ok := dynamicActionGetForModel(m, id); ok {
		return a, true
	}
	return editorSchemeActionGetForModel(m, id)
}

func canonicalActionID(id string) string {
	seen := map[string]bool{}
	for {
		if seen[id] {
			return id
		}
		seen[id] = true
		next, ok := actionAliases[id]
		if !ok || next == "" {
			return id
		}
		id = next
	}
}

// actionList is the registry in registration order.
func actionList() []*Action {
	return append([]*Action(nil), actions.list...)
}

// RunAction runs an action when it is available in this context.
func (m *Model) RunAction(id string) bool {
	return m.RunActionWithArg(id, nil)
}

// RunActionWithArg runs an action with the argument supplied by a menu,
// plugin or automation. It is equivalent to OpenIDEA's executeAction(id, arg).
func (m *Model) RunActionWithArg(id string, arg any) bool {
	a, ok := actionGetForModel(m, id)
	if !ok || !a.usable(m) {
		return false
	}
	if a.RunWithArg != nil {
		a.RunWithArg(m, arg)
	} else if a.Run != nil {
		a.Run(m)
	} else {
		return false
	}
	m.recordMacroAction(id, arg)
	return true
}

// ActionEnabled reports whether an action can run now; a missing action is
// not enabled.
func (m *Model) ActionEnabled(id string) bool {
	a, ok := actionGetForModel(m, id)
	return ok && a.usable(m)
}

// ActionToggled is the checkmark state a menu or toolbar shows for an action.
func (m *Model) ActionToggled(id string) bool {
	if a, ok := actionGetForModel(m, id); ok && a.Toggled != nil {
		return a.Toggled(m)
	}
	return false
}

// ActionRunning is the busy indicator some toolbar widgets show.
func (m *Model) ActionRunning(id string) bool {
	if a, ok := actionGetForModel(m, id); ok && a.Running != nil {
		return a.Running(m)
	}
	return false
}
