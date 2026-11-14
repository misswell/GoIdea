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

// registerAction adds an action. An empty or duplicate id is an error, as
// upstream's registry treats it; a feature registers each action once.
func registerAction(a *Action) error {
	return actions.register(a)
}

func (r *actionRegistry) register(a *Action) error {
	if a == nil || a.ID == "" {
		return fmt.Errorf("action needs an id")
	}
	if a.Run == nil {
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
	a, ok := actions.byID[id]
	return a, ok
}

// actionList is the registry in registration order.
func actionList() []*Action {
	return append([]*Action(nil), actions.list...)
}

// RunAction runs an action when it is available in this context.
func (m *Model) RunAction(id string) bool {
	a, ok := actionGet(id)
	if !ok || !a.usable(m) || a.Run == nil {
		return false
	}
	a.Run(m)
	return true
}

// ActionEnabled reports whether an action can run now; a missing action is
// not enabled.
func (m *Model) ActionEnabled(id string) bool {
	a, ok := actionGet(id)
	return ok && a.usable(m)
}

// ActionToggled is the checkmark state a menu or toolbar shows for an action.
func (m *Model) ActionToggled(id string) bool {
	if a, ok := actionGet(id); ok && a.Toggled != nil {
		return a.Toggled(m)
	}
	return false
}

// ActionRunning is the busy indicator some toolbar widgets show.
func (m *Model) ActionRunning(id string) bool {
	if a, ok := actionGet(id); ok && a.Running != nil {
		return a.Running(m)
	}
	return false
}
