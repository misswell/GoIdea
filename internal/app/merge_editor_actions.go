package app

// mergeCommands is the complete action surface of OpenIDEA's merge editor.
// Every command has a real RunWithArg path so menus, the command palette,
// keymaps and automation invoke the same operation as the toolbar.
var mergeCommands = []struct {
	id      string
	title   string
	icon    string
	enabled func(*Model) bool
}{
	{"applyLeft", "Apply Non-Conflicting Changes from the Left Side", "diff-apply-left",
		func(m *Model) bool {
			s := m.mergeActionState()
			return s != nil && s.model.canApplyNonConflicting(mergeSide("left"), false)
		}},
	{"applyAll", "Apply All Non-Conflicting Changes", "diff-apply-all",
		func(m *Model) bool {
			s := m.mergeActionState()
			return s != nil && s.model.canApplyNonConflicting(mergeSide("base"), true)
		}},
	{"applyRight", "Apply Non-Conflicting Changes from the Right Side", "diff-apply-right",
		func(m *Model) bool {
			s := m.mergeActionState()
			return s != nil && s.model.canApplyNonConflicting(mergeSide("right"), false)
		}},
	{"resolveSimple", "Resolve Simple Conflicts", "diff-magic-toolbar",
		func(m *Model) bool {
			s := m.mergeActionState()
			return s != nil && s.model.canResolveSimple()
		}},
	{"revert", "Revert Conflict Resolution", "diff-revert",
		func(m *Model) bool {
			s := m.mergeActionState()
			return s != nil && s.model.canRevert()
		}},
	{"acceptYours", "Accept Yours", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"acceptTheirs", "Accept Theirs", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"apply", "Apply Merge Result", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"cancel", "Cancel Merge", "",
		func(m *Model) bool { return mergeStateOpen(m) && !m.merge.busy && !m.merge.resolved }},
	{"reload", "Reload from Disk", "refresh",
		func(m *Model) bool { return mergeStateOpen(m) && !m.merge.busy && !m.merge.resolved }},
	{"compare", "Compare Contents", "diff",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"settings", "Merge Settings", "settings",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"autoApply", "Automatically Apply Non-Conflicting Changes", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"compareYoursResult", "Yours and Result", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"compareResultTheirs", "Result and Theirs", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"compareYoursTheirs", "Yours and Theirs", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"compareBaseYours", "Base and Yours", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"compareBaseResult", "Base and Result", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"compareBaseTheirs", "Base and Theirs", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"accept", "Accept", "",
		func(m *Model) bool {
			s := mergeSelectedActionState(m)
			return s != nil && s.pane != mergePaneResult
		}},
	{"resolveUsingYours", "Resolve Using Yours", "",
		func(m *Model) bool {
			s := mergeSelectedActionState(m)
			return s != nil && s.pane == mergePaneYours
		}},
	{"resolveUsingTheirs", "Resolve Using Theirs", "",
		func(m *Model) bool {
			s := mergeSelectedActionState(m)
			return s != nil && s.pane == mergePaneTheirs
		}},
	{"ignore", "Ignore", "diff-remove",
		func(m *Model) bool {
			s := mergeSelectedActionState(m)
			return s != nil && len(s.selectedChangeIndexes()) > 0
		}},
	{"resolveAutomatically", "Resolve Automatically", "diff-magic",
		func(m *Model) bool {
			s := mergeSelectedActionState(m)
			return s != nil && len(s.selectedChangeIndexes()) > 0
		}},
	{"annotate", "Annotate with Git Blame", "changelist",
		func(m *Model) bool {
			s := mergeSelectedActionState(m)
			return s != nil && s.pane != mergePaneResult
		}},
	{"cut", "Cut", "",
		func(m *Model) bool {
			s := mergeSelectedActionState(m)
			return s != nil && s.pane == mergePaneResult &&
				s.selection[mergePaneResult][0] != s.selection[mergePaneResult][1]
		}},
	{"copy", "Copy", "copy",
		func(m *Model) bool {
			s := mergeSelectedActionState(m)
			return s != nil && s.selection[s.pane][0] != s.selection[s.pane][1]
		}},
	{"paste", "Paste", "",
		func(m *Model) bool {
			s := mergeSelectedActionState(m)
			return s != nil && s.pane == mergePaneResult
		}},
	{"selectAll", "Select All", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
	{"change", "Resolve Merge Change", "",
		func(m *Model) bool { return mergeCommandReady(m) }},
}

func init() {
	for _, command := range mergeCommands {
		command := command
		_ = registerAction(&Action{
			ID:          "merge." + command.id,
			Title:       command.title,
			Category:    "Merge",
			Keywords:    "merge conflict " + command.id,
			Description: "Operate on the focused native merge editor",
			When: func(m *Model) bool {
				return m.merge != nil && m.merge.open
			},
			Enabled: command.enabled,
			Toggled: func(m *Model) bool {
				return command.id == "autoApply" && m.prefs.VCS.Merge.AutoApply
			},
			Run: func(m *Model) {
				m.runMergeCommand(command.id, nil)
			},
			RunWithArg: func(m *Model, arg any) {
				m.runMergeCommand(command.id, arg)
			},
		})
	}
}

func mergeStateOpen(m *Model) bool {
	return m != nil && m.merge != nil && m.merge.open
}

func mergeCommandReady(m *Model) bool {
	return m.mergeActionState() != nil
}

func mergeSelectedActionState(m *Model) *mergeEditorState {
	state := m.mergeActionState()
	if state == nil || len(state.selectedChangeIndexes()) == 0 {
		return nil
	}
	return state
}

func (m *Model) runMergeCommand(command string, arg any) {
	switch command {
	case "applyLeft":
		m.ApplyMergeNonConflicting(mergeSide("left"))
	case "applyAll":
		m.ApplyMergeNonConflicting(mergeSide("base"))
	case "applyRight":
		m.ApplyMergeNonConflicting(mergeSide("right"))
	case "resolveSimple":
		m.ResolveSimpleMergeConflicts()
	case "revert":
		m.RevertMergeResolution()
	case "acceptYours":
		m.AcceptMergeYours()
	case "acceptTheirs":
		m.AcceptMergeTheirs()
	case "apply":
		m.ApplyMerge()
	case "cancel":
		m.RequestCloseMerge()
	case "reload":
		m.ReloadMerge()
	case "compare":
		m.ToggleMergeCompareMenu()
	case "settings":
		m.ToggleMergeSettingsMenu()
	case "autoApply":
		m.ToggleMergeAutoApply()
	case "compareYoursResult", "compareResultTheirs", "compareYoursTheirs",
		"compareBaseYours", "compareBaseResult", "compareBaseTheirs":
		m.ShowMergeCompare(command)
	case "accept":
		if state := m.mergeActionState(); state != nil && state.pane != mergePaneResult {
			m.MergeAcceptSelected(mergeSide(state.pane), false)
		}
	case "resolveUsingYours":
		m.MergeAcceptSelected(mergeSide("left"), true)
	case "resolveUsingTheirs":
		m.MergeAcceptSelected(mergeSide("right"), true)
	case "ignore":
		m.MergeIgnoreSelected()
	case "resolveAutomatically":
		m.MergeResolveSelectedAutomatically()
	case "annotate":
		m.AnnotateMergePane()
	case "cut":
		m.MergeCut()
	case "copy":
		m.MergeCopy()
	case "paste":
		m.MergePaste()
	case "selectAll":
		m.MergeSelectAll()
	case "change":
		m.RunMergeChange(arg)
	}
}
