package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestMergeCommandsAreAllRegistered(t *testing.T) {
	if len(mergeCommands) != 30 {
		t.Fatalf("merge commands = %d, want 30", len(mergeCommands))
	}
	for _, command := range mergeCommands {
		if _, ok := actionGet("merge." + command.id); !ok {
			t.Errorf("missing action merge.%s", command.id)
		}
	}
	if !hiddenActionIDs["merge.change"] {
		t.Fatal("merge.change is not hidden from the command palette")
	}
}

func TestMergeEditorLoadsVersionsAndResolvesSelectedChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "conflict.go")
	versions := mergeVersions{
		Base:   textPointer("a\nbase\nz\n"),
		Ours:   textPointer("a\nleft\nz\n"),
		Theirs: textPointer("a\nright\nz\n"),
		Sides: mergeSides{
			Operation:   "merge",
			Branch:      textPointer("main"),
			OtherBranch: textPointer("topic"),
		},
	}
	raw, err := json.Marshal(versions)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "vcs_conflict_versions" {
				t.Fatalf("unexpected command %q", command)
			}
			return raw, nil
		},
	}
	m.setRoot(root)
	m.OpenMergeEditor(path)
	waitMergeReady(t, m)
	if m.merge.model == nil || len(m.merge.model.changes) != 1 ||
		m.merge.model.changes[0].kind != mergeChangeConflict {
		t.Fatalf("merge model = %+v", m.merge.model)
	}
	if !m.ActionEnabled("merge.change") || m.ActionEnabled("merge.applyLeft") {
		t.Fatal("the conflict's action enabled state is wrong")
	}
	if !m.RunActionWithArg("merge.change", map[string]any{
		"indexes": []int{0},
		"action": map[string]any{
			"type": "accept", "side": "left", "resolve": false,
		},
	}) {
		t.Fatal("merge.change did not run")
	}
	if m.merge.model.result != "a\nleft\nz\n" {
		t.Fatalf("result = %q", m.merge.model.result)
	}
	if len(m.merge.model.unresolved()) != 1 || !m.merge.touched {
		t.Fatalf("after accepting one side: unresolved=%v touched=%v",
			m.merge.model.unresolved(), m.merge.touched)
	}
	m.closeMergeEditor()
}

func TestMergeEditorRendersOpenIDEAControls(t *testing.T) {
	model := newMergeEditorModel(
		"/repo/conflict.go",
		"a\nleft\nz\n",
		"a\nbase\nz\n",
		"a\nright\nz\n",
	)
	m := &Model{
		merge: &mergeEditorState{
			open: true, path: "/repo/conflict.go", model: model, result: model.result,
			pane: mergePaneResult,
			versions: mergeVersions{
				Base:   textPointer(model.base),
				Ours:   textPointer(model.left),
				Theirs: textPointer(model.right),
				Sides:  mergeSides{Operation: "merge", Branch: textPointer("main"), OtherBranch: textPointer("topic")},
			},
			selection: map[mergePane][2]int{
				mergePaneYours: {0, 0}, mergePaneResult: {0, 0}, mergePaneTheirs: {0, 0},
			},
			pendingSelect: map[mergePane][2]int{},
			pendingCaret:  map[mergePane]int{},
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.mergeDialog(c, c.Theme())
	}, 1420, 900)
	tester.Frame()
	for _, text := range []string{
		"Merge · conflict.go",
		"Changes from main", "Result", "Changes from topic",
		"Apply Left", "Apply All", "Apply Right", "Resolve Simple", "Revert",
		"Compare", "Reload", "Settings",
		"Accept Yours", "Accept Theirs", "Cancel", "Apply",
	} {
		if !tester.HasText(text) {
			t.Errorf("merge editor does not contain %q", text)
		}
	}
}

func TestMergeAutoApplyPreferencePersists(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	model := newMergeEditorModel("a.go", "left\n", "base\n", "right\n")
	m := &Model{
		cfg:   Config{SettingsPath: settingsPath},
		merge: &mergeEditorState{open: true, path: "a.go", model: model, result: model.result},
	}
	m.setMergeAutoApply(true)
	if !m.prefs.VCS.Merge.AutoApply {
		t.Fatal("the in-memory preference is off")
	}
	if !LoadPreferences(settingsPath).VCS.Merge.AutoApply {
		t.Fatal("the merge auto-apply preference was not stored")
	}
}

func TestMergeAutoApplyOnLoadDoesNotMakeTheEditorDirty(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "conflict.go")
	versions := mergeVersions{
		Base: textPointer(
			"top\nbase-left\nkeep-one\nbase-middle\nkeep-two\nbase-right\nbottom\n",
		),
		Ours: textPointer(
			"top\nleft\nkeep-one\nours-middle\nkeep-two\nbase-right\nbottom\n",
		),
		Theirs: textPointer(
			"top\nbase-left\nkeep-one\ntheirs-middle\nkeep-two\nright\nbottom\n",
		),
	}
	raw, err := json.Marshal(versions)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{
		ctx:      context.Background(),
		postHook: func(fn func()) { fn() },
		prefs: Preferences{
			VCS: VCSPrefs{Merge: MergePrefs{AutoApply: true}},
		},
		invokeHook: func(_ context.Context, command string, _ any) (json.RawMessage, error) {
			if command != "vcs_conflict_versions" {
				t.Fatalf("unexpected command %q", command)
			}
			return raw, nil
		},
	}
	m.setRoot(root)
	m.OpenMergeEditor(path)
	waitMergeReady(t, m)
	state := m.merge
	if state == nil || state.model == nil {
		t.Fatal("the merge editor did not load")
	}
	if state.touched {
		t.Fatal("the initial auto-apply was recorded as a user edit")
	}
	if state.model.result == state.model.base {
		t.Fatal("auto-apply did not apply the non-conflicting changes")
	}
	if len(state.model.unresolved()) != 1 || state.model.conflictCount() != 1 {
		t.Fatalf("unresolved=%v conflicts=%d",
			state.model.unresolved(), state.model.conflictCount())
	}
}

func TestMergeApplyWritesStagesAndClosesTheEditor(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "conflict.go")
	if err := os.WriteFile(path, []byte("a\nbase\nz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := newMergeEditorModel(
		path,
		"a\nleft\nz\n",
		"a\nbase\nz\n",
		"a\nright\nz\n",
	)
	if !model.acceptChange(0, mergeSide("left"), false) ||
		!model.acceptChange(0, mergeSide("right"), false) {
		t.Fatal("the conflict could not be resolved")
	}
	var staged []string
	m, settled := mergeActionModel(path, model, &staged, nil)
	m.ApplyMerge()
	waitMergeSettled(t, settled)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), "a\nleft\nright\nz\n"; got != want {
		t.Fatalf("resolved file = %q, want %q", got, want)
	}
	if len(staged) != 1 || staged[0] != path {
		t.Fatalf("staged paths = %v, want [%s]", staged, path)
	}
	if m.merge != nil {
		t.Fatal("the merge editor stayed open after a successful apply")
	}
	if got, want := m.status, "conflict.go: conflicts resolved"; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
}

func TestMergeMissingSideDeletesAndStagesTheFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "deleted.go")
	if err := os.WriteFile(path, []byte("only theirs remains\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := newMergeEditorModel(path, "", "base\n", "theirs\n")
	var staged []string
	m, settled := mergeActionModel(path, model, &staged, func(state *mergeEditorState) {
		state.versions.Ours = nil
		state.versions.Theirs = textPointer("theirs\n")
	})
	m.AcceptMergeYours()
	waitMergeSettled(t, settled)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the missing side did not delete the file: %v", err)
	}
	if len(staged) != 1 || staged[0] != path {
		t.Fatalf("staged paths = %v, want [%s]", staged, path)
	}
}

func TestMergeUnresolvedApplyWaitsForConfirmation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "pending.go")
	const original = "a\nbase\nz\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	model := newMergeEditorModel(
		path,
		"a\nleft\nz\n",
		original,
		"a\nright\nz\n",
	)
	var staged []string
	m, settled := mergeActionModel(path, model, &staged, nil)
	m.ApplyMerge()
	if got := m.merge.confirm; got != "apply" {
		t.Fatalf("confirmation = %q, want apply", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != original || len(staged) != 0 {
		t.Fatal("Apply wrote or staged the file before confirmation")
	}

	m.confirmMergeAction()
	waitMergeSettled(t, settled)
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != original {
		t.Fatalf("unresolved result = %q, want the base text %q", raw, original)
	}
	if len(staged) != 1 || staged[0] != path {
		t.Fatalf("staged paths = %v, want [%s]", staged, path)
	}
}

func TestMergeNavigationWrapsThroughUnresolvedChanges(t *testing.T) {
	model := newMergeEditorModel(
		"a.go",
		"a\nleft-one\nkeep\nleft-two\nz\n",
		"a\nbase-one\nkeep\nbase-two\nz\n",
		"a\nright-one\nkeep\nright-two\nz\n",
	)
	state := mergeTestState(model)
	m := &Model{merge: state}
	open := model.unresolved()
	if len(open) != 2 {
		t.Fatalf("unresolved changes = %v, want two", open)
	}
	for _, want := range []int{open[0], open[1], open[0]} {
		m.StepMergeChange(1)
		if state.currentConflict != want {
			t.Fatalf("next change = %d, want %d", state.currentConflict, want)
		}
		if state.pane != mergePaneResult {
			t.Fatalf("next focused pane = %q, want result", state.pane)
		}
	}
	m.StepMergeChange(-1)
	if state.currentConflict != open[1] {
		t.Fatalf("previous change = %d, want %d", state.currentConflict, open[1])
	}
}

func TestMergeCompareUsesAllSixOpenIDEAVersionPairs(t *testing.T) {
	model := newMergeEditorModel(
		"/repo/a.go",
		"yours\n",
		"base\n",
		"theirs\n",
	)
	model.setResultText("result\n")
	state := mergeTestState(model)
	m := &Model{merge: state, postHook: func(fn func()) { fn() }}
	tests := []struct {
		command string
		left    string
		right   string
		llabel  string
		rlabel  string
	}{
		{"compareYoursResult", "yours\n", "result\n", "Changes from main", "Result"},
		{"compareResultTheirs", "result\n", "theirs\n", "Result", "Changes from topic"},
		{"compareYoursTheirs", "yours\n", "theirs\n", "Changes from main", "Changes from topic"},
		{"compareBaseYours", "base\n", "yours\n", "Base", "Changes from main"},
		{"compareBaseResult", "base\n", "result\n", "Base", "Result"},
		{"compareBaseTheirs", "base\n", "theirs\n", "Base", "Changes from topic"},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			m.ShowMergeCompare(test.command)
			if m.diff == nil {
				t.Fatal("compare view did not open")
			}
			if m.diff.leftText != test.left || m.diff.rightText != test.right {
				t.Fatalf("texts = %q / %q, want %q / %q",
					m.diff.leftText, m.diff.rightText, test.left, test.right)
			}
			if m.diff.leftLabel != test.llabel || m.diff.rightLabel != test.rlabel {
				t.Fatalf("labels = %q / %q, want %q / %q",
					m.diff.leftLabel, m.diff.rightLabel, test.llabel, test.rlabel)
			}
		})
	}
}

func mergeTestState(model *mergeEditorModel) *mergeEditorState {
	state := newMergeEditorState(model.path)
	state.loading = false
	state.model = model
	state.result = model.result
	state.versions = mergeVersions{
		Base:   textPointer(model.base),
		Ours:   textPointer(model.left),
		Theirs: textPointer(model.right),
		Sides: mergeSides{
			Operation: "merge", Branch: textPointer("main"), OtherBranch: textPointer("topic"),
		},
	}
	return state
}

func mergeActionModel(
	path string,
	model *mergeEditorModel,
	staged *[]string,
	decorate func(*mergeEditorState),
) (*Model, <-chan struct{}) {
	state := mergeTestState(model)
	state.path = path
	if decorate != nil {
		decorate(state)
	}
	settled := make(chan struct{}, 1)
	m := &Model{
		ctx:   context.Background(),
		merge: state,
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "vcs_stage":
				raw, _ := json.Marshal(input)
				var request struct {
					Paths []string `json:"paths"`
				}
				_ = json.Unmarshal(raw, &request)
				*staged = append(*staged, request.Paths...)
				return json.RawMessage(`true`), nil
			case "vcs_status":
				return json.RawMessage(`[]`), nil
			default:
				return nil, os.ErrInvalid
			}
		},
	}
	m.setRoot(filepath.Dir(path))
	m.postHook = func(fn func()) {
		fn()
		if m.merge == nil {
			select {
			case settled <- struct{}{}:
			default:
			}
		}
	}
	return m, settled
}

func waitMergeSettled(t *testing.T, settled <-chan struct{}) {
	t.Helper()
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("the merge editor did not finish the write and stage")
	}
}

func waitMergeReady(t *testing.T, m *Model) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m.merge != nil && !m.merge.loading {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("merge editor did not load")
}
