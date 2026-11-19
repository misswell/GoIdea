package app

// The shipped command list registers itself into the action registry, so the
// built-ins go through the same lifecycle as a feature's or plugin's actions.

var builtinShortcuts = map[string]string{}
var builtinCategories = map[string]string{}

func init() {
	for _, c := range builtinCommands() {
		builtinShortcuts[c.id] = c.shortcut
		builtinCategories[c.id] = c.group
		// A malformed built-in id would be a programming error; the tests
		// check the whole list instead of failing the app at start.
		a := &Action{
			ID:       c.id,
			Title:    c.title,
			Category: c.group,
			Keywords: c.keywords,
			Run:      c.run,
			Builtin:  true,
		}
		switch c.id {
		case "build.compileFile":
			a.When = func(m *Model) bool {
				t := m.tab()
				return t != nil && t.kind == "" && t.doc != nil && m.canCompileFile(t.doc.Path())
			}
		case "navigate.recentFiles", "navigate.recentLocations", "analyze.stackTrace",
			"navigate.gotoClass", "navigate.gotoFile", "navigate.gotoSymbol", "navigate.findAction":
			a.When = func(m *Model) bool { return m.Root() != "" && !m.showingWelcome }
		}
		_ = registerAction(a)
	}
}
