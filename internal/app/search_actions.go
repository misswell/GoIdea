package app

// Search and navigation actions live outside the static command table because
// Find in Files and Switcher take OpenIDEA action arguments.
func init() {
	action := func(id, title string, run func(*Model, any)) *Action {
		return &Action{
			ID: id, Title: title, Category: "Find",
			Keywords:   "find search replace files path switcher structure scope",
			RunWithArg: run,
		}
	}
	for _, item := range []*Action{
		action("find.inPath", "Find in Files…", func(m *Model, arg any) {
			m.ShowFind(false, arg)
		}),
		action("find.replaceInPath", "Replace in Files…", func(m *Model, arg any) {
			m.ShowFind(true, arg)
		}),
		action("find.rerun", "Rerun Search", func(m *Model, _ any) {
			m.RerunSearch()
		}),
		action("navigate.switcher", "Switcher", func(m *Model, _ any) {
			m.CycleSwitcher(1)
		}),
		action("navigate.switcherBack", "Switcher (Backward)", func(m *Model, _ any) {
			m.CycleSwitcher(-1)
		}),
		action("navigate.fileStructure", "File Structure", func(m *Model, _ any) {
			m.OpenFileStructure()
		}),
		action("search.editScopes", "Edit Scopes…", func(m *Model, _ any) {
			m.OpenSearchScopes()
		}),
	} {
		_ = registerAction(item)
	}
}
