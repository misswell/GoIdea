package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

type mergePane string

const (
	mergePaneYours   mergePane = "left"
	mergePaneResult  mergePane = "result"
	mergePaneTheirs  mergePane = "right"
	mergeConfirmNone           = ""
)

type mergeEditorState struct {
	open     bool
	path     string
	loading  bool
	busy     bool
	resolved bool
	err      string
	status   string

	versions mergeVersions
	model    *mergeEditorModel
	result   string
	pane     mergePane
	loadID   uint64

	selection       map[mergePane][2]int
	pendingSelect   map[mergePane][2]int
	pendingCaret    map[mergePane]int
	currentConflict int
	touched         bool
	confirm         string
	menu            string
}

type mergeResolveAction struct {
	kind    string
	side    mergeSide
	resolve bool
}

type mergeChangeArg struct {
	Indexes []int              `json:"indexes"`
	Action  mergeResolveAction `json:"action"`
}

func newMergeEditorState(path string) *mergeEditorState {
	return &mergeEditorState{
		open: true, path: realPath(path), loading: true, pane: mergePaneResult,
		selection: map[mergePane][2]int{
			mergePaneYours:  {0, 0},
			mergePaneResult: {0, 0},
			mergePaneTheirs: {0, 0},
		},
		pendingSelect:   map[mergePane][2]int{},
		pendingCaret:    map[mergePane]int{},
		currentConflict: -1,
	}
}

// OpenMergeEditor reads the three conflict stages and opens OpenIDEA's native
// merge sheet. The file itself is only written by Apply or Accept Yours/Theirs.
func (m *Model) OpenMergeEditor(path string) {
	if path == "" || m.Root() == "" {
		return
	}
	path = realPath(path)
	if m.merge != nil && m.merge.path == path && m.merge.open {
		m.CloseVCSMore()
		m.invalidate()
		return
	}
	state := newMergeEditorState(path)
	m.merge = state
	m.closeVCSMore()
	m.invalidate()
	m.loadMergeEditor(state)
}

func (m *Model) loadMergeEditor(state *mergeEditorState) {
	if state == nil || m.ctx == nil && m.invokeHook == nil && m.mgr == nil {
		return
	}
	state.loadID++
	loadID := state.loadID
	state.loading = true
	state.resolved = false
	state.err = ""
	state.status = ""
	state.model = nil
	state.result = ""
	state.touched = false
	state.confirm = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_conflict_versions", map[string]any{"path": state.path})
		var versions mergeVersions
		if err == nil {
			err = json.Unmarshal(raw, &versions)
		}
		if err == nil && versions.Base == nil && versions.Ours == nil && versions.Theirs == nil {
			err = errors.New("the backend returned no conflict versions")
		}
		if err == nil && (mergeTextContainsBinaryPointer(versions.Base) ||
			mergeTextContainsBinaryPointer(versions.Ours) ||
			mergeTextContainsBinaryPointer(versions.Theirs)) {
			err = errors.New("Binary files cannot be resolved in the merge editor")
		}
		m.post(func() {
			current := m.merge
			if current != state || state.loadID != loadID {
				return
			}
			if err != nil {
				state.err = mergeErrorText(err)
				state.loading = false
				m.invalidate()
				return
			}
			state.versions = versions
			left := mergeVersionText(versions.Ours)
			base := mergeVersionText(versions.Base)
			right := mergeVersionText(versions.Theirs)
			state.model = newMergeEditorModel(state.path, left, base, right)
			state.model.leftMissing = versions.Ours == nil
			state.model.rightMissing = versions.Theirs == nil
			state.model.baseMissing = versions.Base == nil
			state.model.auto = m.prefs.VCS.Merge.AutoApply
			state.result = state.model.result
			state.currentConflict = -1
			state.status = mergeSummaryText(state.model)
			if state.model.auto {
				state.model.applyNonConflicting(mergeSide("base"), true)
				state.result = state.model.result
			}
			state.loading = false
			m.invalidate()
		})
	}()
}

func (m *Model) mergeContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func mergeTextContainsBinaryPointer(text *string) bool {
	return text != nil && mergeTextContainsBinary(*text)
}

func mergeVersionText(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}

func mergeErrorText(err error) string {
	var oe *openidea.Error
	if errors.As(err, &oe) && strings.Contains(strings.ToLower(oe.Message), "not a git repository") {
		return "The current project is not a Git repository"
	}
	return "Could not load the conflict versions: " + err.Error()
}

func mergeSummaryText(model *mergeEditorModel) string {
	if model == nil {
		return ""
	}
	pending := len(model.unresolved())
	conflicts := model.conflictCount()
	switch {
	case pending == 0:
		return "All changes processed"
	case conflicts == 0:
		return fmt.Sprintf("%d %s left", pending, plural(pending, "change", "changes"))
	default:
		return fmt.Sprintf("%d %s left, %d %s",
			pending, plural(pending, "change", "changes"),
			conflicts, plural(conflicts, "conflict", "conflicts"))
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// mergeTitles follows OpenIDEA's operation-aware pane names instead of
// reducing every conflict to a generic "ours" and "theirs".
func mergeTitles(sides mergeSides) (string, string) {
	branch, commit, other := sides.Branch, sides.Commit, sides.OtherBranch
	value := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	switch sides.Operation {
	case "rebase":
		yours := "Your changes"
		if commit != nil && branch != nil {
			yours = "Rebasing " + value(commit) + " from " + value(branch)
		}
		theirs := "Already rebased commits"
		if other != nil {
			theirs += " and commits from " + value(other)
		}
		return yours, theirs
	case "merge":
		yours := "Your version"
		if branch != nil {
			yours = "Changes from " + value(branch)
		}
		theirs := "Changes from diverging branches"
		switch {
		case other != nil:
			theirs = "Changes from " + value(other)
		case commit != nil:
			theirs = "Changes from " + value(commit)
		}
		return yours, theirs
	case "unstash":
		yours := "Your uncommitted changes"
		theirs := "Changes from remote"
		if branch != nil {
			theirs = "Changes from " + value(branch)
		}
		return yours, theirs
	case "cherry-pick":
		yours := "Local Changes"
		theirs := "Changes from cherry-pick"
		if commit != nil {
			theirs += " " + value(commit)
		}
		return yours, theirs
	default:
		yours := "Your version"
		if branch != nil {
			yours += ", branch " + value(branch)
		}
		theirs := "Theirs (incoming)"
		if commit != nil {
			theirs = "Changes from " + value(commit)
		}
		return yours, theirs
	}
}

func (m *Model) CloseVCSMore() {
	m.closeVCSMore()
}

// RequestCloseMerge mirrors the editor's Cancel action: it asks before
// discarding decisions the user made in this sheet.
func (m *Model) RequestCloseMerge() {
	state := m.merge
	if state == nil || !state.open {
		return
	}
	if state.touched && !state.busy {
		state.confirm = "cancel"
		m.invalidate()
		return
	}
	m.closeMergeEditor()
}

func (m *Model) closeMergeEditor() {
	if m.merge == nil {
		return
	}
	m.merge = nil
	m.invalidate()
}

func (m *Model) confirmMergeAction() {
	state := m.merge
	if state == nil {
		return
	}
	action := state.confirm
	state.confirm = ""
	switch action {
	case "apply":
		m.applyMergeText(state.model.outputText())
	case "yours":
		m.acceptMergeWholeSide(mergeSide("left"))
	case "theirs":
		m.acceptMergeWholeSide(mergeSide("right"))
	case "cancel":
		m.closeMergeEditor()
	case "reload":
		m.loadMergeEditor(state)
	}
}

func (m *Model) discardMergeConfirmation() {
	if m.merge == nil {
		return
	}
	m.merge.confirm = ""
	m.invalidate()
}

func (m *Model) ApplyMerge() {
	state := m.mergeActionState()
	if state == nil {
		return
	}
	state.result = state.model.result
	if len(state.model.unresolved()) > 0 {
		state.confirm = "apply"
		m.invalidate()
		return
	}
	m.applyMergeText(state.model.outputText())
}

func (m *Model) AcceptMergeYours() {
	m.requestAcceptMergeWholeSide(mergeSide("left"))
}

func (m *Model) AcceptMergeTheirs() {
	m.requestAcceptMergeWholeSide(mergeSide("right"))
}

func (m *Model) requestAcceptMergeWholeSide(side mergeSide) {
	state := m.mergeActionState()
	if state == nil {
		return
	}
	if state.touched {
		if side == mergeSide("left") {
			state.confirm = "yours"
		} else {
			state.confirm = "theirs"
		}
		m.invalidate()
		return
	}
	m.acceptMergeWholeSide(side)
}

func (m *Model) acceptMergeWholeSide(side mergeSide) {
	state := m.mergeActionState()
	if state == nil {
		return
	}
	if side == mergeSide("left") {
		if state.versions.Ours == nil {
			m.applyMergeTextPointer(nil, "yours")
			return
		}
		m.applyMergeTextPointer(state.versions.Ours, "yours")
		return
	}
	if state.versions.Theirs == nil {
		m.applyMergeTextPointer(nil, "theirs")
		return
	}
	m.applyMergeTextPointer(state.versions.Theirs, "theirs")
}

func (m *Model) applyMergeText(text string) {
	m.applyMergeTextPointer(&text, "merge result")
}

func (m *Model) applyMergeTextPointer(text *string, source string) {
	state := m.mergeActionState()
	if state == nil {
		return
	}
	state.busy = true
	state.err = ""
	state.status = "Saving " + filepath.Base(state.path) + "…"
	m.invalidate()
	path := state.path
	var tab *Tab
	for _, candidate := range m.tabs {
		if candidate != nil && candidate.kind == "" && candidate.doc != nil &&
			filepath.Clean(candidate.doc.Path()) == path {
			tab = candidate
			break
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.mergeContext(), 30*time.Second)
		defer cancel()
		var err error
		if text == nil {
			err = os.Remove(path)
			if os.IsNotExist(err) {
				err = nil
			}
		} else if tab == nil {
			err = writeFileAtomic(path, []byte(*text))
		} else {
			err = tab.doc.ApplyEdits(ctx, editorEdit(0, len(tab.doc.Text()), *text))
			if err == nil {
				err = tab.doc.Sync(ctx)
			}
			if err == nil {
				err = tab.doc.Save(ctx)
			}
		}
		if err == nil {
			_, err = m.invoke(ctx, "vcs_stage", map[string]any{"paths": []string{path}})
		}
		m.post(func() {
			if m.merge != state {
				return
			}
			state.busy = false
			if err != nil {
				state.err = "Could not resolve " + filepath.Base(path) + ": " + err.Error()
				state.status = ""
				m.invalidate()
				return
			}
			state.resolved = true
			state.status = filepath.Base(path) + " was resolved and staged"
			if tab != nil {
				tab.synced = tab.doc.Buffer().Version()
				tab.noteDisk()
				if text == nil {
					m.closeTabThen(tab, nil)
				}
			}
			m.status = filepath.Base(path) + ": conflicts resolved"
			m.RefreshVCS()
			// OpenIDEA closes the merge editor after the staged write succeeds.
			m.closeMergeEditor()
		})
	}()
}

func editorEdit(start, end int, text string) editor.Edit {
	return editor.Edit{Start: start, End: end, Text: text}
}

func (m *Model) ReloadMerge() {
	state := m.mergeActionState()
	if state == nil {
		return
	}
	if state.touched {
		state.confirm = "reload"
		m.invalidate()
		return
	}
	m.loadMergeEditor(state)
}

func (m *Model) ToggleMergeAutoApply() {
	state := m.mergeActionState()
	if state == nil {
		return
	}
	m.setMergeAutoApply(!m.prefs.VCS.Merge.AutoApply)
}

func (m *Model) setMergeAutoApply(on bool) {
	state := m.mergeActionState()
	if state == nil {
		return
	}
	was := m.prefs.VCS.Merge.AutoApply
	m.prefs.VCS.Merge.AutoApply = on
	state.model.auto = on
	m.savePreferences()
	if on && !was {
		count := state.model.applyNonConflicting(mergeSide("base"), true)
		state.result = state.model.result
		state.status = fmt.Sprintf("Applied %d non-conflicting %s",
			count, plural(count, "change", "changes"))
	}
	m.invalidate()
}

func (m *Model) ToggleMergeCompareMenu() {
	if state := m.mergeActionState(); state != nil {
		state.menu = "compare"
		m.invalidate()
	}
}

func (m *Model) ToggleMergeSettingsMenu() {
	if state := m.mergeActionState(); state != nil {
		state.menu = "settings"
		m.invalidate()
	}
}

func (m *Model) ApplyMergeNonConflicting(side mergeSide) {
	state := m.mergeActionState()
	if state == nil || state.model == nil {
		return
	}
	all := side == mergeSide("base")
	if !state.model.canApplyNonConflicting(side, all) {
		return
	}
	count := state.model.applyNonConflicting(side, all)
	state.result = state.model.result
	state.touched = true
	state.status = fmt.Sprintf("Applied %d non-conflicting %s",
		count, plural(count, "change", "changes"))
	m.revealNextMergeChange()
	m.invalidate()
}

func (m *Model) ResolveSimpleMergeConflicts() {
	state := m.mergeActionState()
	if state == nil || state.model == nil {
		return
	}
	count := state.model.resolveSimple()
	if count == 0 {
		return
	}
	state.result = state.model.result
	state.touched = true
	state.status = fmt.Sprintf("Resolved %d simple %s",
		count, plural(count, "change", "changes"))
	m.revealNextMergeChange()
	m.invalidate()
}

func (m *Model) RevertMergeResolution() {
	state := m.mergeActionState()
	if state == nil || state.model == nil {
		return
	}
	count := state.model.revertResolved()
	if count == 0 {
		return
	}
	state.result = state.model.result
	state.touched = true
	state.status = fmt.Sprintf("Reverted %d resolved %s",
		count, plural(count, "change", "changes"))
	m.invalidate()
}

func (m *Model) MergeAcceptSelected(side mergeSide, resolve bool) {
	m.applySelectedMergeAction(mergeResolveAction{
		kind: "accept", side: side, resolve: resolve,
	})
}

func (m *Model) MergeIgnoreSelected() {
	m.applySelectedMergeAction(mergeResolveAction{kind: "ignore"})
}

func (m *Model) MergeResolveSelectedAutomatically() {
	m.applySelectedMergeAction(mergeResolveAction{kind: "auto"})
}

func (m *Model) applySelectedMergeAction(action mergeResolveAction) {
	state := m.mergeActionState()
	if state == nil || state.model == nil {
		return
	}
	indexes := state.selectedChangeIndexes()
	if len(indexes) == 0 {
		return
	}
	m.applyMergeActionTo(indexes, action)
}

func (m *Model) applyMergeActionTo(indexes []int, action mergeResolveAction) {
	state := m.mergeActionState()
	if state == nil || state.model == nil {
		return
	}
	changed := 0
	for _, index := range indexes {
		var ok bool
		switch action.kind {
		case "accept":
			ok = state.model.acceptChange(index, action.side, action.resolve)
		case "ignore":
			side := action.side
			if action.resolve || state.pane == mergePaneResult {
				side = ""
			}
			if side == "" {
				ok = state.model.ignoreWholeChange(index)
			} else {
				ok = state.model.ignoreChange(index, side)
			}
		case "auto":
			ok = state.model.resolveAutomatically(index)
		}
		if ok {
			changed++
		}
	}
	if changed == 0 {
		return
	}
	state.result = state.model.result
	state.touched = true
	state.status = mergeSummaryText(state.model)
	m.invalidate()
}

func (m *Model) RunMergeChange(arg any) {
	parsed, ok := parseMergeChangeArg(arg)
	if !ok {
		return
	}
	m.applyMergeActionTo(parsed.Indexes, parsed.Action)
}

func parseMergeChangeArg(arg any) (mergeChangeArg, bool) {
	switch value := arg.(type) {
	case mergeChangeArg:
		return value, len(value.Indexes) > 0
	case *mergeChangeArg:
		if value == nil {
			return mergeChangeArg{}, false
		}
		return *value, len(value.Indexes) > 0
	case map[string]any:
		raw, err := json.Marshal(value)
		if err != nil {
			return mergeChangeArg{}, false
		}
		var shape struct {
			Indexes []int `json:"indexes"`
			Action  struct {
				Kind    string `json:"kind"`
				Type    string `json:"type"`
				Side    string `json:"side"`
				Resolve bool   `json:"resolve"`
			} `json:"action"`
		}
		if json.Unmarshal(raw, &shape) != nil || len(shape.Indexes) == 0 {
			return mergeChangeArg{}, false
		}
		out := mergeChangeArg{Indexes: shape.Indexes}
		out.Action.kind = shape.Action.Kind
		if out.Action.kind == "" {
			out.Action.kind = shape.Action.Type
		}
		out.Action.side = mergeSide(shape.Action.Side)
		out.Action.resolve = shape.Action.Resolve
		if out.Action.kind == "" {
			out.Action.kind = fmt.Sprint(value["kind"])
		}
		if out.Action.side == "" {
			switch fmt.Sprint(value["side"]) {
			case "left", "yours":
				out.Action.side = mergeSide("left")
			case "right", "theirs":
				out.Action.side = mergeSide("right")
			}
		}
		return out, true
	default:
		return mergeChangeArg{}, false
	}
}

func (m *Model) StepMergeChange(delta int) {
	state := m.mergeActionState()
	if state == nil || state.model == nil {
		return
	}
	open := state.model.unresolved()
	if len(open) == 0 {
		return
	}
	current := state.currentConflict
	index := 0
	if delta > 0 {
		for _, candidate := range open {
			if candidate > current {
				index = candidate
				goto selected
			}
		}
		index = open[0]
	} else {
		index = open[len(open)-1]
		for i := len(open) - 1; i >= 0; i-- {
			if open[i] < current {
				index = open[i]
				break
			}
		}
	}
selected:
	state.currentConflict = index
	offset := state.model.changes[index].resultFrom
	state.pendingCaret[mergePaneResult] = utf8.RuneCountInString(state.model.result[:min(max(0, offset), len(state.model.result))])
	state.pane = mergePaneResult
	m.invalidate()
}

func (m *Model) revealNextMergeChange() {
	state := m.merge
	if state == nil || state.model == nil {
		return
	}
	open := state.model.unresolved()
	if len(open) == 0 {
		state.currentConflict = -1
		return
	}
	next := open[0]
	for _, candidate := range open {
		if candidate > state.currentConflict {
			next = candidate
			break
		}
	}
	state.currentConflict = next
	offset := state.model.changes[next].resultFrom
	state.pendingCaret[mergePaneResult] = utf8.RuneCountInString(state.model.result[:min(max(0, offset), len(state.model.result))])
	state.pane = mergePaneResult
}

func (m *Model) MergeCut() {
	state := m.mergeActionState()
	if state == nil || state.model == nil || state.pane != mergePaneResult {
		return
	}
	text := state.paneText()
	start, end := runeRange(state.selection[mergePaneResult], text)
	if start == end {
		return
	}
	mygo.Clipboard.WriteText(text[start:end])
	state.replaceResultRunes(state.selection[mergePaneResult], "")
}

func (m *Model) MergeCopy() {
	state := m.mergeActionState()
	if state == nil {
		return
	}
	text := state.paneText()
	start, end := runeRange(state.selection[state.pane], text)
	if start < end {
		mygo.Clipboard.WriteText(text[start:end])
		m.status = "Copied"
	}
}

func (m *Model) MergePaste() {
	state := m.mergeActionState()
	if state == nil || state.model == nil || state.pane != mergePaneResult {
		return
	}
	data, err := mygo.Clipboard.Read(transfer.Text)
	if err != nil {
		state.status = "The clipboard has no text"
		m.invalidate()
		return
	}
	raw, err := data.Read(transfer.Text)
	if err != nil {
		state.status = "The clipboard has no text"
		m.invalidate()
		return
	}
	state.replaceResultRunes(state.selection[mergePaneResult], string(raw))
}

func (m *Model) MergeSelectAll() {
	state := m.mergeActionState()
	if state == nil {
		return
	}
	text := state.paneText()
	n := utf8.RuneCountInString(text)
	state.selection[state.pane] = [2]int{0, n}
	state.pendingSelect[state.pane] = [2]int{0, n}
	m.invalidate()
}

func (state *mergeEditorState) replaceResultRunes(selection [2]int, insert string) {
	if state == nil || state.model == nil {
		return
	}
	r := []rune(state.model.result)
	start, end := selection[0], selection[1]
	if start > end {
		start, end = end, start
	}
	start = min(max(start, 0), len(r))
	end = min(max(end, start), len(r))
	next := string(r[:start]) + insert + string(r[end:])
	state.model.setResultText(next)
	state.result = state.model.result
	state.touched = true
	caret := start + utf8.RuneCountInString(insert)
	state.selection[mergePaneResult] = [2]int{caret, caret}
	state.pendingSelect[mergePaneResult] = [2]int{caret, caret}
	state.status = mergeSummaryText(state.model)
}

func runeRange(selection [2]int, text string) (int, int) {
	r := []rune(text)
	start, end := selection[0], selection[1]
	if start > end {
		start, end = end, start
	}
	start = min(max(start, 0), len(r))
	end = min(max(end, start), len(r))
	return len(string(r[:start])), len(string(r[:end]))
}

func (m *Model) ShowMergeCompare(command string) {
	state := m.mergeActionState()
	if state == nil || state.model == nil {
		return
	}
	yoursTitle, theirsTitle := mergeTitles(state.versions.Sides)
	type version struct {
		text  string
		label string
	}
	versions := map[string]version{
		"yours":  {mergeVersionText(state.versions.Ours), yoursTitle},
		"base":   {mergeVersionText(state.versions.Base), "Base"},
		"result": {state.model.result, "Result"},
		"theirs": {mergeVersionText(state.versions.Theirs), theirsTitle},
	}
	var leftKey, rightKey string
	switch command {
	case "compareYoursResult":
		leftKey, rightKey = "yours", "result"
	case "compareResultTheirs":
		leftKey, rightKey = "result", "theirs"
	case "compareYoursTheirs":
		leftKey, rightKey = "yours", "theirs"
	case "compareBaseYours":
		leftKey, rightKey = "base", "yours"
	case "compareBaseResult":
		leftKey, rightKey = "base", "result"
	case "compareBaseTheirs":
		leftKey, rightKey = "base", "theirs"
	default:
		return
	}
	left, right := versions[leftKey], versions[rightKey]
	d := &diffView{
		open: true, path: state.path, rightPath: state.path, inline: true,
		leftLabel: left.label, rightLabel: right.label, loading: true,
		options: m.diffPrefs(),
	}
	m.diff = d
	m.finishCompare(d, left.text, right.text, left.label+" is identical to "+right.label)
}

func (m *Model) AnnotateMergePane() {
	state := m.mergeActionState()
	if state == nil || state.pane == mergePaneResult {
		return
	}
	revision := "HEAD"
	if state.pane == mergePaneTheirs && state.versions.Sides.Commit != nil {
		revision = *state.versions.Sides.Commit
	}
	text := state.paneText()
	m.ShowAnnotateVersion(state.path, text, revision)
}

func (m *Model) mergeActionState() *mergeEditorState {
	state := m.merge
	if state == nil || !state.open || state.loading || state.busy || state.resolved ||
		state.err != "" || state.model == nil {
		return nil
	}
	return state
}

func (state *mergeEditorState) paneText() string {
	if state == nil || state.model == nil {
		return ""
	}
	switch state.pane {
	case mergePaneYours:
		return state.model.left
	case mergePaneTheirs:
		return state.model.right
	default:
		return state.model.result
	}
}

func (state *mergeEditorState) selectedChangeIndexes() []int {
	if state == nil || state.model == nil {
		return nil
	}
	text := state.model.result
	from, to := runeRange(state.selection[mergePaneResult], text)
	if state.pane != mergePaneResult {
		text = state.paneText()
		from, to = runeRange(state.selection[state.pane], text)
	}
	return mergeSelectedChanges(state.model, state.pane, from, to)
}

func mergeSelectedChanges(model *mergeEditorModel, pane mergePane, from, to int) []int {
	if model == nil {
		return nil
	}
	start, end := mergeLineSpan(model.result, from, to)
	if pane != mergePaneResult {
		text := model.left
		if pane == mergePaneTheirs {
			text = model.right
		}
		start, end = mergeLineSpan(text, from, to)
	}
	var indexes []int
	for i, change := range model.changes {
		var a, b int
		switch pane {
		case mergePaneYours:
			if !change.changesSide(mergeSide("left")) {
				continue
			}
			a, b = change.leftFrom, change.leftTo
		case mergePaneTheirs:
			if !change.changesSide(mergeSide("right")) {
				continue
			}
			a, b = change.rightFrom, change.rightTo
		default:
			a, b = change.resultFrom, change.resultTo
		}
		hit := a >= start && (a < end || a == mergeDocLength(model, pane))
		if b > a {
			hit = a < end && b > start
		}
		if hit {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func mergeDocLength(model *mergeEditorModel, pane mergePane) int {
	switch pane {
	case mergePaneYours:
		return len(model.left)
	case mergePaneTheirs:
		return len(model.right)
	default:
		return len(model.result)
	}
}

func mergeLineSpan(text string, from, to int) (int, int) {
	from = min(max(from, 0), len(text))
	to = min(max(to, from), len(text))
	start := strings.LastIndexByte(text[:from], '\n') + 1
	end := to
	if next := strings.IndexByte(text[to:], '\n'); next >= 0 {
		end = to + next + 1
	} else {
		end = len(text)
	}
	return start, end
}

func (m *Model) mergeDialog(c *ui.Context, p *ui.Theme) {
	state := m.merge
	if state == nil || !state.open {
		return
	}
	open := state.open
	dialog := nativeModal(c.Key("merge-editor"), &open, func() {
		ui.Column(c).Width(1320).Gap(8).Children(func() {
			if state.confirm != "" {
				m.drawMergeConfirmation(c, p, state)
				return
			}
			m.drawMergeHeader(c, p, state)
			if state.err != "" {
				ui.Text(c, state.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			} else if state.loading {
				ui.Text(c, "Reading the three conflict versions…").FontSize(12).
					TextColor(p.TextMuted).SingleLine()
			} else if state.resolved {
				ui.Text(c, state.status).FontSize(12).TextColor(p.Success).SingleLine()
			} else if state.model != nil {
				m.drawMergeToolbar(c, p, state)
				m.drawMergePanes(c, p, state)
				m.drawMergeFooter(c, p, state)
			}
		})
	})
	if !open || dialog.Dismissed() {
		m.RequestCloseMerge()
	}
}

func (m *Model) drawMergeHeader(c *ui.Context, p *ui.Theme, state *mergeEditorState) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		title := "Merge · " + filepath.Base(state.path)
		ui.Text(c, title).FontSize(14).Bold().TextColor(p.Text).SingleLine().
			Ellipsis("…")
		ui.Spacer(c)
		if state.status != "" && !state.loading {
			ui.Text(c, state.status).FontSize(12).TextColor(p.TextMuted).SingleLine()
		}
	})
	if rel, err := filepath.Rel(m.Root(), state.path); err == nil && !strings.HasPrefix(rel, "..") {
		ui.Text(c, filepath.ToSlash(rel)).FontSize(12).TextColor(p.TextMuted).
			SingleLine().Ellipsis("…")
	}
}

func (m *Model) drawMergeToolbar(c *ui.Context, p *ui.Theme, state *mergeEditorState) {
	model := state.model
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "‹ Previous").Disabled(len(model.unresolved()) == 0).
			OnClick(func() { m.StepMergeChange(-1) })
		ui.Button(c, "Next ›").Disabled(len(model.unresolved()) == 0).
			OnClick(func() { m.StepMergeChange(1) })
		ui.Button(c, "Apply Left").Disabled(!model.canApplyNonConflicting(mergeSide("left"), false)).
			OnClick(func() { m.ApplyMergeNonConflicting(mergeSide("left")) })
		ui.Button(c, "Apply All").Disabled(!model.canApplyNonConflicting(mergeSide("base"), true)).
			OnClick(func() { m.ApplyMergeNonConflicting(mergeSide("base")) })
		ui.Button(c, "Apply Right").Disabled(!model.canApplyNonConflicting(mergeSide("right"), false)).
			OnClick(func() { m.ApplyMergeNonConflicting(mergeSide("right")) })
		ui.Button(c, "Resolve Simple").Disabled(!model.canResolveSimple()).
			OnClick(m.ResolveSimpleMergeConflicts)
		ui.Button(c, "Revert").Disabled(!model.canRevert()).
			OnClick(m.RevertMergeResolution)
		ui.Spacer(c)
		ui.Button(c, "Compare").OnClick(m.ToggleMergeCompareMenu)
		ui.Button(c, "Reload").OnClick(m.ReloadMerge)
		ui.Button(c, "Settings").OnClick(m.ToggleMergeSettingsMenu)
	})
	switch state.menu {
	case "compare":
		m.drawMergeCompareMenu(c, p)
	case "settings":
		m.drawMergeSettingsMenu(c, p)
	}
}

func (m *Model) drawMergeCompareMenu(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Wrap().Gap(5).Padding(6).Radius(5).Background(p.Surface).
		BorderWidth(1).BorderColor(p.Border).Children(func() {
		for _, item := range []struct {
			label   string
			command string
		}{
			{"Yours and Result", "compareYoursResult"},
			{"Result and Theirs", "compareResultTheirs"},
			{"Yours and Theirs", "compareYoursTheirs"},
			{"Base and Yours", "compareBaseYours"},
			{"Base and Result", "compareBaseResult"},
			{"Base and Theirs", "compareBaseTheirs"},
		} {
			label, command := item.label, item.command
			ui.Button(c, label).OnClick(func() { m.ShowMergeCompare(command) })
		}
	})
}

func (m *Model) drawMergeSettingsMenu(c *ui.Context, p *ui.Theme) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(6).Radius(5).Background(p.Surface).
		BorderWidth(1).BorderColor(p.Border).Children(func() {
		on := m.prefs.VCS.Merge.AutoApply
		if ui.Checkbox(c, &on,
			"Automatically apply non-conflicting changes").Changed() {
			m.setMergeAutoApply(on)
		}
		ui.Text(c, "Applied at once when the merge editor opens.").
			FontSize(11).TextColor(p.TextMuted)
	})
}

func (m *Model) drawMergePanes(c *ui.Context, p *ui.Theme, state *mergeEditorState) {
	yoursTitle, theirsTitle := mergeTitles(state.versions.Sides)
	ui.Row(c).Gap(6).Children(func() {
		m.drawMergePane(c, p, state, mergePaneYours, yoursTitle)
		m.drawMergePane(c, p, state, mergePaneResult, "Result")
		m.drawMergePane(c, p, state, mergePaneTheirs, theirsTitle)
	})
}

func (m *Model) drawMergePane(c *ui.Context, p *ui.Theme, state *mergeEditorState,
	pane mergePane, title string) {
	ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(12).Bold().TextColor(p.Text).SingleLine().
				Ellipsis("…")
			ui.Spacer(c)
			switch pane {
			case mergePaneYours:
				if state.versions.Ours == nil {
					ui.Text(c, "Deleted").FontSize(11).TextColor(p.Danger)
				}
			case mergePaneTheirs:
				if state.versions.Theirs == nil {
					ui.Text(c, "Deleted").FontSize(11).TextColor(p.Danger)
				}
			case mergePaneResult:
				ui.Text(c, mergeSummaryText(state.model)).FontSize(11).
					TextColor(p.TextMuted).SingleLine()
			}
		})
		var text string
		switch pane {
		case mergePaneYours:
			text = state.model.left
		case mergePaneTheirs:
			text = state.model.right
		default:
			text = state.model.result
			state.result = text
		}
		ui.Row(c).Gap(2).Height(500).MinWidth(0).Children(func() {
			area := ui.TextArea(c.Key("merge-pane:"+string(pane)), &text).
				Grow(1).Basis(0).Height(500).MinWidth(0).
				Font(oiFontMono).FontSize(12).ReadOnly(pane != mergePaneResult)
			area.ContextMenu(func(menu *ui.Menu) { m.mergePaneMenu(menu, pane) })
			if area.FocusWithin() {
				state.pane = pane
			}
			start, end := area.TextSelection()
			state.selection[pane] = [2]int{start, end}
			if pending, ok := state.pendingSelect[pane]; ok {
				area.SetTextSelection(pending[0], pending[1])
				delete(state.pendingSelect, pane)
			} else if caret, ok := state.pendingCaret[pane]; ok {
				area.SetTextSelection(caret, caret)
				delete(state.pendingCaret, pane)
			}
			if pane == mergePaneResult && text != state.model.result {
				state.model.setResultText(text)
				state.result = state.model.result
				state.touched = true
				state.status = mergeSummaryText(state.model)
			}
			overviewText := state.model.result
			if pane == mergePaneYours {
				overviewText = state.model.left
			} else if pane == mergePaneTheirs {
				overviewText = state.model.right
			}
			overviewLane(c, p, "merge-overview:"+string(pane),
				"Merge overview "+string(pane), nil, overviewText,
				mergeOverviewMarks(state.model, pane), 500)
		})
	})
}

func (m *Model) mergePaneMenu(menu *ui.Menu, pane mergePane) {
	if m.merge == nil {
		return
	}
	m.merge.pane = pane
	if pane == mergePaneResult {
		if menu.Item("Resolve Automatically").Chosen() {
			m.MergeResolveSelectedAutomatically()
		}
		if menu.Item("Ignore").Chosen() {
			m.MergeIgnoreSelected()
		}
		menu.Separator()
		if menu.Item("Cut").Chosen() {
			m.MergeCut()
		}
	} else {
		if menu.Item("Accept").Chosen() {
			m.MergeAcceptSelected(mergeSide(pane), false)
		}
		if menu.Item("Resolve Using Yours").Chosen() && pane == mergePaneYours {
			m.MergeAcceptSelected(mergeSide("left"), true)
		}
		if menu.Item("Resolve Using Theirs").Chosen() && pane == mergePaneTheirs {
			m.MergeAcceptSelected(mergeSide("right"), true)
		}
		if menu.Item("Ignore").Chosen() {
			m.MergeIgnoreSelected()
		}
		menu.Separator()
		if menu.Item("Annotate with Git Blame").Chosen() {
			m.AnnotateMergePane()
		}
	}
	if menu.Item("Copy").Chosen() {
		m.MergeCopy()
	}
	if pane == mergePaneResult && menu.Item("Paste").Chosen() {
		m.MergePaste()
	}
	if menu.Item("Select All").Chosen() {
		m.MergeSelectAll()
	}
}

func (m *Model) drawMergeFooter(c *ui.Context, p *ui.Theme, state *mergeEditorState) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "Accept Yours").Disabled(state.busy).OnClick(m.AcceptMergeYours)
		ui.Button(c, "Accept Theirs").Disabled(state.busy).OnClick(m.AcceptMergeTheirs)
		ui.Spacer(c)
		ui.Button(c, "Cancel").Disabled(state.busy).OnClick(m.RequestCloseMerge)
		ui.Button(c, "Apply").Disabled(state.busy).OnClick(m.ApplyMerge)
	})
}

func (m *Model) drawMergeConfirmation(c *ui.Context, p *ui.Theme, state *mergeEditorState) {
	title := "Unresolved Changes"
	message := ""
	action := "Mark Resolved"
	switch state.confirm {
	case "cancel":
		title = "Cancel Merge"
		message = "Exit without applying the changes made in the result?"
		action = "Exit"
	case "yours":
		title = "Accept Yours"
		message = "The result was changed. Discard those changes and take Your version as it is?"
		action = "Discard Changes"
	case "theirs":
		title = "Accept Theirs"
		message = "The result was changed. Discard those changes and take Their version as it is?"
		action = "Discard Changes"
	case "reload":
		title = "Reload from Disk"
		message = "Discard the decisions made in the result and reload all three versions?"
		action = "Reload"
	default:
		pending := len(state.model.unresolved())
		message = fmt.Sprintf(
			"%d unresolved %s keep the base text. Save and mark the file as resolved anyway?",
			pending, plural(pending, "change", "changes"))
	}
	ui.Column(c).Gap(12).Padding(12).Children(func() {
		ui.Text(c, title).FontSize(15).Bold().TextColor(p.Text).SingleLine()
		ui.Text(c, message).FontSize(12).TextColor(p.TextMuted)
		if state.err != "" {
			ui.Text(c, state.err).FontSize(12).TextColor(p.Danger)
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.Spacer(c)
			ui.Button(c, "Back").OnClick(m.discardMergeConfirmation)
			ui.Button(c, action).OnClick(m.confirmMergeAction)
		})
	})
}

// SmokeMerge opens a conflict, reports the model state and closes it without
// writing the working tree. It is the headless check for the native editor.
func (m *Model) SmokeMerge(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	raw, err := m.invoke(ctx, "vcs_status", nil)
	if err != nil {
		return out, err
	}
	var files []openidea.FileStatus
	if err := json.Unmarshal(raw, &files); err != nil {
		return out, err
	}
	path := ""
	for _, file := range files {
		if effectiveVCSKind(file) == "conflicted" {
			path = file.Path
			break
		}
	}
	if path == "" {
		return out, errors.New("There are no conflicted files")
	}
	out["path"] = path
	mygo.RunOnMain(func() { m.OpenMergeEditor(path) })
	deadline := time.Now().Add(60 * time.Second)
	for {
		var loading bool
		var errText, status string
		var changes, pending, conflicts int
		mygo.RunOnMain(func() {
			state := m.merge
			if state == nil {
				loading = true
				return
			}
			loading = state.loading
			errText, status = state.err, state.status
			if state.model != nil {
				changes = len(state.model.changes)
				pending = len(state.model.unresolved())
				conflicts = state.model.conflictCount()
			}
		})
		if !loading {
			out["changes"] = changes
			out["pending"] = pending
			out["conflicts"] = conflicts
			out["status"] = status
			if errText != "" {
				out["error"] = errText
			}
			break
		}
		if time.Now().After(deadline) {
			out["error"] = "Opening the merge timed out"
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	mygo.RunOnMain(m.closeMergeEditor)
	return out, nil
}
