package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const maxVCSOutputLines = 300

const (
	vcsOpNewBranch = "newBranch"
	vcsOpNewTag    = "newTag"
	vcsOpMerge     = "merge"
	vcsOpRebase    = "rebase"
	vcsOpStash     = "stash"
	vcsOpUnstash   = "unstash"
	vcsOpFetch     = "fetch"
	vcsOpPull      = "pull"
	vcsOpPush      = "push"
	vcsOpReset     = "reset"
)

type vcsOperationChoice struct {
	label  string
	value  string
	detail string
}

type vcsQuickItem struct {
	id     string
	title  string
	detail string
}

type vcsQuickList struct {
	open  bool
	query string
	sel   int
}

var vcsQuickItems = []vcsQuickItem{
	{id: "vcs.newBranch", title: "New Branch…", detail: "Create and check out a branch"},
	{id: "vcs.newTag", title: "New Tag…", detail: "Tag the selected revision"},
	{id: "vcs.commit", title: "Commit…", detail: "Review and commit changes"},
	{id: "vcs.fetch", title: "Fetch", detail: "Download remote refs"},
	{id: "vcs.pull", title: "Pull…", detail: "Update the current branch"},
	{id: "vcs.push", title: "Push…", detail: "Publish the current branch"},
	{id: "vcs.merge", title: "Merge…", detail: "Merge a branch or tag"},
	{id: "vcs.rebase", title: "Rebase…", detail: "Rebase onto a branch or tag"},
	{id: "vcs.stash", title: "Stash Changes…", detail: "Store local changes"},
	{id: "vcs.unstash", title: "Unstash Changes…", detail: "Restore stashed changes"},
	{id: "vcs.resetHead", title: "Reset HEAD…", detail: "Move the current branch"},
	{id: "vcs.log", title: "Show Git Log", detail: "Browse commits and changes"},
}

// VCSOperationPane is the native Git operations sheet. It is shared by the
// Git menu, project and editor Git menus, command palette and toolbar.
type VCSOperationPane struct {
	kind  string
	open  bool
	title string
	hint  string
	label string
	name  string

	message          string
	includeUntracked bool
	keepIndex        bool
	rebase           bool
	force            bool
	setUpstream      bool
	resetMode        int

	choices []vcsOperationChoice
	sel     int
	loading bool
	busy    bool
	err     string
	output  []string

	request uint32
	channel uint32
	remotes *vcsRemotesPane
	// preferredStart is the branch/tag selected by a Git Log or Branches pane
	// action before the shared revision chooser finishes loading.
	preferredStart string
}

func (m *Model) resetVCSOperations() { m.vcsOps = VCSOperationPane{} }

// OpenVCSOperations opens OpenIDEA's VCS Operations quick list. The list
// dispatches to the same native commands as the Git menus.
func (m *Model) OpenVCSOperations() {
	if m.Root() == "" {
		return
	}
	if m.vcsQuick.open {
		m.closeVCSQuickList()
		return
	}
	m.vcsQuick = vcsQuickList{open: true}
	m.invalidate()
}

func (m *Model) closeVCSQuickList() {
	m.vcsQuick = vcsQuickList{}
	m.invalidate()
}

func (m *Model) vcsQuickRows() []vcsQuickItem {
	query := strings.ToLower(strings.TrimSpace(m.vcsQuick.query))
	rows := make([]vcsQuickItem, 0, len(vcsQuickItems))
	for _, item := range vcsQuickItems {
		if !m.ActionEnabled(item.id) {
			continue
		}
		haystack := strings.ToLower(item.title + " " + item.detail)
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		rows = append(rows, item)
	}
	return rows
}

func (m *Model) stepVCSQuick(delta int) {
	rows := m.vcsQuickRows()
	if len(rows) == 0 {
		return
	}
	m.vcsQuick.sel = clampSymbolSel(m.vcsQuick.sel+delta, len(rows))
	m.invalidate()
}

func (m *Model) activateVCSQuick() {
	rows := m.vcsQuickRows()
	if len(rows) == 0 {
		return
	}
	item := rows[clampSymbolSel(m.vcsQuick.sel, len(rows))]
	m.closeVCSQuickList()
	m.RunAction(item.id)
}

func (m *Model) vcsQuickListDialog(c *ui.Context, p *ui.Theme) {
	if !m.vcsQuick.open {
		return
	}
	open := m.vcsQuick.open
	rows := m.vcsQuickRows()
	if m.vcsQuick.sel >= len(rows) {
		m.vcsQuick.sel = max(0, len(rows)-1)
	}
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(540).Gap(8).Children(func() {
			ui.Text(c, "VCS Operations").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &m.vcsQuick.query).AutoFocus()
			in.OnShortcut(0, ui.KeyDown, func() { m.stepVCSQuick(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepVCSQuick(-1) })
			if in.Submitted() {
				m.activateVCSQuick()
			}
			ui.Scroll(c).Height(330).Children(func() {
				ui.Column(c).Children(func() {
					if len(rows) == 0 {
						ui.Text(c, "No matching VCS operations").FontSize(12).TextColor(p.TextMuted)
					}
					for i, item := range rows {
						i, item := i, item
						r := ui.Row(c.Key("vcs.quick:"+item.id)).Height(30).Padding(0, 9).
							Gap(12).AlignItems(ui.Center)
						if i == m.vcsQuick.sel {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.OnClick(func() {
							m.vcsQuick.sel = i
							m.activateVCSQuick()
						})
						r.Children(func() {
							ui.Text(c, item.title).FontSize(13).TextColor(p.Text).SingleLine()
							ui.Spacer(c)
							ui.Text(c, item.detail).FontSize(12).TextColor(p.TextMuted).SingleLine()
						})
					}
				})
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Cancel").OnClick(m.closeVCSQuickList)
			})
		})
	})
	if !open {
		m.closeVCSQuickList()
	}
}

func (m *Model) OpenNewGitBranch() {
	m.openVCSOperation(vcsOpNewBranch)
}

// OpenNewGitBranchAt starts the branch chooser at a ref selected outside the
// popup, as the Git Log Branches pane does.
func (m *Model) OpenNewGitBranchAt(ref string) {
	m.openVCSOperation(vcsOpNewBranch)
	m.vcsOps.preferredStart = ref
}

func (m *Model) OpenNewGitTag() {
	m.openVCSOperation(vcsOpNewTag)
}

func (m *Model) OpenGitMerge() {
	m.openVCSOperation(vcsOpMerge)
}

// OpenGitMergeAt opens Merge with a branch selected in the Branches popup.
func (m *Model) OpenGitMergeAt(ref string) {
	m.openVCSOperation(vcsOpMerge)
	m.vcsOps.preferredStart = ref
}

func (m *Model) OpenGitRebase() {
	m.openVCSOperation(vcsOpRebase)
}

// OpenGitRebaseAt opens Rebase with a branch selected in the Branches popup.
func (m *Model) OpenGitRebaseAt(ref string) {
	m.openVCSOperation(vcsOpRebase)
	m.vcsOps.preferredStart = ref
}

func (m *Model) OpenGitStash() {
	m.openVCSOperation(vcsOpStash)
}

func (m *Model) OpenGitUnstash() {
	m.openVCSOperation(vcsOpUnstash)
}

func (m *Model) GitFetch() {
	m.runVCSRemote(vcsOpFetch, "Fetch", map[string]any{"op": "fetch"})
}

func (m *Model) OpenGitPull() {
	m.openVCSOperation(vcsOpPull)
}

func (m *Model) OpenGitPush() {
	m.openVCSOperation(vcsOpPush)
}

func (m *Model) OpenGitReset() {
	m.openVCSOperation(vcsOpReset)
}

func (m *Model) openVCSOperation(kind string) {
	if m.Root() == "" {
		return
	}
	state := VCSOperationPane{
		kind:             kind,
		open:             true,
		sel:              0,
		includeUntracked: true,
		keepIndex:        false,
	}
	switch kind {
	case vcsOpNewBranch:
		state.title = "Create New Branch"
		state.hint = "The new branch starts at the selected revision."
		state.label = "Branch name"
		m.loadVCSStartChoices(&state)
	case vcsOpNewTag:
		state.title = "Create New Tag"
		state.hint = "Create a tag at the current revision."
		state.label = "Tag name"
		state.choices = []vcsOperationChoice{m.selectedCommitChoice()}
	case vcsOpMerge:
		state.title = "Merge"
		state.hint = "Merge the selected branch or tag into the current branch."
		m.loadVCSRevisionChoices(&state, "No branches or tags to merge")
	case vcsOpRebase:
		state.title = "Rebase"
		state.hint = "Rebase the current branch onto the selected branch or tag."
		m.loadVCSRevisionChoices(&state, "No branches or tags to rebase onto")
	case vcsOpStash:
		state.title = "Stash Changes"
		state.hint = "Store local changes and return the working tree to HEAD."
		state.label = "Stash message"
	case vcsOpUnstash:
		state.title = "Unstash Changes"
		state.hint = "Apply a stash and remove it from the stash list."
		m.loadVCSStashes(&state)
	case vcsOpPull:
		state.title = "Pull"
		state.hint = "Update the current branch from its configured upstream."
	case vcsOpPush:
		state.title = "Push"
		state.hint = "Push the current branch to its upstream."
	case vcsOpReset:
		state.title = "Reset Current Branch"
		state.hint = "Move the current branch to the selected revision."
		state.label = "Mode"
		state.choices = []vcsOperationChoice{m.selectedCommitChoice()}
	}
	m.vcsOps = state
	m.invalidate()
}

func (m *Model) closeVCSOperation() {
	if m.vcsOps.busy || m.vcsOps.remotes != nil && m.vcsOps.remotes.busy {
		return
	}
	m.resetVCSOperations()
	m.invalidate()
}

func (m *Model) selectedCommitChoice() vcsOperationChoice {
	if m.log.visible && m.log.selected >= 0 && m.log.selected < len(m.log.commits) {
		c := m.log.commits[m.log.selected]
		return vcsOperationChoice{label: c.ShortID + " " + c.Summary, value: c.ID}
	}
	return vcsOperationChoice{label: "Current revision", value: "HEAD"}
}

func (m *Model) loadVCSStartChoices(state *VCSOperationPane) {
	state.loading = true
	m.vcsOps = *state
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		branches, branchErr := m.branchList(ctx)
		var tags []string
		tagRaw, tagErr := m.invoke(ctx, "vcs_tags", map[string]any{"repo": nil})
		if tagErr == nil {
			tagErr = json.Unmarshal(tagRaw, &tags)
		}
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != vcsOpNewBranch {
				return
			}
			m.vcsOps.loading = false
			if branchErr != nil {
				m.vcsOps.err = "Could not read the starting revisions: " + branchErr.Error()
			} else if tagErr != nil {
				m.vcsOps.err = "Could not read the tags: " + tagErr.Error()
			}
			m.vcsOps.choices = m.vcsRevisionChoices(branches, tags)
			if m.vcsOps.preferredStart != "" {
				for i, choice := range m.vcsOps.choices {
					if choice.value == m.vcsOps.preferredStart {
						m.vcsOps.sel = i
						break
					}
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadVCSRevisionChoices(state *VCSOperationPane, empty string) {
	state.loading = true
	m.vcsOps = *state
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		branches, branchErr := m.branchList(ctx)
		var tags []string
		tagRaw, tagErr := m.invoke(ctx, "vcs_tags", map[string]any{"repo": nil})
		if tagErr == nil {
			tagErr = json.Unmarshal(tagRaw, &tags)
		}
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != state.kind {
				return
			}
			m.vcsOps.loading = false
			if branchErr != nil {
				m.vcsOps.err = "Could not read the revisions: " + branchErr.Error()
			} else if tagErr != nil {
				m.vcsOps.err = "Could not read the tags: " + tagErr.Error()
			}
			m.vcsOps.choices = m.vcsRevisionChoices(branches, tags)
			if m.vcsOps.preferredStart != "" {
				for i, choice := range m.vcsOps.choices {
					if choice.value == m.vcsOps.preferredStart {
						m.vcsOps.sel = i
						break
					}
				}
			}
			if len(m.vcsOps.choices) == 0 && m.vcsOps.err == "" {
				m.vcsOps.err = empty
			}
			m.invalidate()
		})
	}()
}

func (m *Model) vcsRevisionChoices(branches []openidea.Branch, tags []string) []vcsOperationChoice {
	out := make([]vcsOperationChoice, 0, len(branches)+len(tags))
	for _, b := range branches {
		if b.Remote || b.Current {
			continue
		}
		detail := "Branch"
		if b.Upstream != nil {
			detail += " · tracks " + *b.Upstream
		}
		out = append(out, vcsOperationChoice{label: b.Name, value: b.Name, detail: detail})
	}
	for _, tag := range tags {
		out = append(out, vcsOperationChoice{label: tag, value: tag, detail: "Tag"})
	}
	return out
}

func (m *Model) loadVCSStashes(state *VCSOperationPane) {
	state.loading = true
	m.vcsOps = *state
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "vcs_stashes", map[string]any{"repo": nil})
		var stashes []openidea.Stash
		if err == nil {
			err = json.Unmarshal(raw, &stashes)
		}
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != vcsOpUnstash {
				return
			}
			m.vcsOps.loading = false
			if err != nil {
				m.vcsOps.err = "Could not read the stash list: " + err.Error()
				m.invalidate()
				return
			}
			for _, s := range stashes {
				label := s.Message
				if label == "" {
					label = fmt.Sprintf("Stash #%d", s.Index)
				}
				detail := logTime(m.prefs.dateFormatPrefs(), m.systemRegion, s.TimeMs)
				if detail != "" {
					detail = fmt.Sprintf("#%d · %s", s.Index, detail)
				} else {
					detail = fmt.Sprintf("#%d", s.Index)
				}
				m.vcsOps.choices = append(m.vcsOps.choices, vcsOperationChoice{
					label: label, value: fmt.Sprint(s.Index), detail: detail,
				})
			}
			if len(m.vcsOps.choices) == 0 {
				m.vcsOps.err = "There are no stashed changes"
			}
			m.invalidate()
		})
	}()
}

func (m *Model) stepVCSOperationChoice(delta int) {
	state := &m.vcsOps
	if len(state.choices) == 0 || state.kind == vcsOpReset {
		return
	}
	state.sel = clampSymbolSel(state.sel+delta, len(state.choices))
	m.invalidate()
}

func (m *Model) submitVCSOperation() {
	state := &m.vcsOps
	if !state.open || state.busy || state.loading {
		return
	}
	switch state.kind {
	case vcsOpNewBranch:
		name := strings.TrimSpace(state.name)
		if name == "" {
			state.err = "Enter a branch name"
			m.invalidate()
			return
		}
		start := "HEAD"
		if len(state.choices) > 0 {
			start = state.choices[clampSymbolSel(state.sel, len(state.choices))].value
		}
		m.runVCSOperation("Create Branch", "vcs_create_branch", map[string]any{
			"repo": nil, "name": name, "start": start, "checkout": true, "overwrite": nil,
		})
	case vcsOpNewTag:
		name := strings.TrimSpace(state.name)
		if name == "" {
			state.err = "Enter a tag name"
			m.invalidate()
			return
		}
		commit := state.choices[clampSymbolSel(state.sel, len(state.choices))].value
		message := strings.TrimSpace(state.message)
		var annotated any
		if message != "" {
			annotated = message
		}
		m.runVCSOperation("Create Tag", "vcs_create_tag", map[string]any{
			"repo": nil, "name": name, "commit": commit, "message": annotated, "force": false,
		})
	case vcsOpMerge:
		if len(state.choices) == 0 {
			state.err = "Select a branch or tag to merge"
			m.invalidate()
			return
		}
		branch := state.choices[clampSymbolSel(state.sel, len(state.choices))].value
		m.runVCSOperation("Merge", "vcs_merge_with", map[string]any{
			"repo": nil, "branches": []string{branch},
			"options": map[string]any{
				"noFf": false, "ffOnly": false, "squash": false,
				"noCommit": false, "message": nil,
			},
		})
	case vcsOpRebase:
		if len(state.choices) == 0 {
			state.err = "Select a branch or tag to rebase onto"
			m.invalidate()
			return
		}
		upstream := state.choices[clampSymbolSel(state.sel, len(state.choices))].value
		m.runVCSOperation("Rebase", "vcs_rebase_with", map[string]any{
			"repo": nil, "upstream": upstream,
			"options": map[string]any{"onto": nil, "rebaseMerges": false, "keepEmpty": false},
		})
	case vcsOpStash:
		message := strings.TrimSpace(state.message)
		var stashMessage any
		if message != "" {
			stashMessage = message
		}
		m.runVCSOperation("Stash", "vcs_stash_push", map[string]any{
			"repo": nil, "message": stashMessage,
			"includeUntracked": state.includeUntracked, "keepIndex": state.keepIndex,
		})
	case vcsOpUnstash:
		if len(state.choices) == 0 {
			state.err = "Select a stash to apply"
			m.invalidate()
			return
		}
		index := 0
		_, _ = fmt.Sscan(state.choices[clampSymbolSel(state.sel, len(state.choices))].value, &index)
		m.runVCSOperation("Apply Stash", "vcs_stash_apply", map[string]any{
			"repo": nil, "index": index, "pop": true, "reinstateIndex": state.keepIndex, "branch": nil,
		})
	case vcsOpPull:
		m.runVCSRemote(vcsOpPull, "Pull", map[string]any{"op": "pull", "rebase": state.rebase})
	case vcsOpPush:
		m.runVCSRemote(vcsOpPush, "Push", map[string]any{
			"op": "push", "force_with_lease": state.force,
			"set_upstream": state.setUpstream, "tags": nil,
			"force": false, "no_verify": false,
		})
	case vcsOpReset:
		commit := state.choices[clampSymbolSel(state.sel, len(state.choices))].value
		modes := []string{"soft", "mixed", "hard", "keep"}
		mode := modes[clampSymbolSel(state.resetMode, len(modes))]
		if mode == "hard" && !state.force {
			state.err = "Select the confirmation to discard local changes"
			m.invalidate()
			return
		}
		m.runVCSGitOperation("Reset", map[string]any{
			"op": "reset", "commit": commit, "mode": mode,
		})
	}
}

func (m *Model) runVCSOperation(title, command string, args map[string]any) {
	state := &m.vcsOps
	state.busy = true
	state.err = ""
	state.output = nil
	root := m.Root()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, command, args)
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != state.kind {
				return
			}
			m.vcsOps.busy = false
			if err != nil {
				m.vcsOps.err = title + " failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = title + " completed"
			m.refreshAfterVCSWrite()
			m.resetVCSOperations()
			m.invalidate()
		})
	}()
}

func (m *Model) runVCSRemote(kind, title string, op map[string]any) {
	if m.Root() == "" {
		return
	}
	if !m.vcsOps.open || m.vcsOps.kind != kind {
		m.vcsOps = VCSOperationPane{kind: kind, open: true, title: title}
	}
	state := &m.vcsOps
	state.busy = true
	state.err = ""
	state.output = nil
	state.request = m.nextRequest()
	state.channel = m.nextChannel()
	root := m.Root()
	request, channel := state.request, state.channel
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_remote", map[string]any{
			"repo": nil, "request": request, "op": op,
			"onOutput": map[string]any{"$channel": channel},
		})
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.kind != kind || m.vcsOps.request != request {
				return
			}
			m.vcsOps.busy = false
			if err != nil {
				m.vcsOps.err = title + " failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = title + " completed"
			m.refreshAfterVCSWrite()
			m.invalidate()
		})
	}()
}

func (m *Model) runVCSGitOperation(title string, op map[string]any) {
	state := &m.vcsOps
	state.busy = true
	state.err = ""
	state.output = nil
	state.request = m.nextRequest()
	state.channel = m.nextChannel()
	root := m.Root()
	request, channel := state.request, state.channel
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "vcs_operation", map[string]any{
			"repo": nil, "request": request, "op": op,
			"onOutput": map[string]any{"$channel": channel},
		})
		m.post(func() {
			if m.Root() != root || !m.vcsOps.open || m.vcsOps.request != request {
				return
			}
			m.vcsOps.busy = false
			if err != nil {
				m.vcsOps.err = title + " failed: " + err.Error()
				m.invalidate()
				return
			}
			m.status = title + " completed"
			m.refreshAfterVCSWrite()
			m.resetVCSOperations()
			m.invalidate()
		})
	}()
}

func (m *Model) acceptVCSOperationOutput(id uint32, data json.RawMessage) bool {
	state := &m.vcsOps
	if id == 0 || id != state.channel {
		return false
	}
	var line string
	if json.Unmarshal(data, &line) != nil {
		return true
	}
	line = strings.TrimSuffix(line, "\n")
	if line != "" {
		state.output = append(state.output, line)
		if over := len(state.output) - maxVCSOutputLines; over > 0 {
			state.output = append(state.output[:0], state.output[over:]...)
		}
	}
	m.invalidate()
	return true
}

func (m *Model) refreshAfterVCSWrite() {
	m.RefreshVCS()
	m.RefreshBranch()
	if m.log.visible {
		m.RefreshLog()
	}
}

// ContinueGitOperation continues a stopped merge, rebase, cherry-pick or
// revert through vcs_operation.
func (m *Model) ContinueGitOperation() {
	m.runStandaloneVCSGitOperation("Continue", map[string]any{"op": "continue"})
}

// AbortGitOperation aborts the operation currently in progress.
func (m *Model) AbortGitOperation() {
	m.runStandaloneVCSGitOperation("Abort", map[string]any{"op": "abort"})
}

// SkipGitOperation skips the stopped rebase or cherry-pick commit.
func (m *Model) SkipGitOperation() {
	m.runStandaloneVCSGitOperation("Skip", map[string]any{"op": "skip"})
}

func (m *Model) runStandaloneVCSGitOperation(title string, op map[string]any) {
	if m.Root() == "" || m.vcsOps.busy {
		return
	}
	m.vcsOps = VCSOperationPane{kind: "operation", open: true, title: title}
	m.runVCSGitOperation(title, op)
}

func (m *Model) selectedGitCommit() string {
	if m.log.selected >= 0 && m.log.selected < len(m.log.commits) {
		return m.log.commits[m.log.selected].ID
	}
	return ""
}

// CherryPickSelectedCommit applies the commit selected in Git Log.
func (m *Model) CherryPickSelectedCommit() {
	commit := m.selectedGitCommit()
	if commit == "" {
		m.notifyInfo("git-cherry-pick", "Select a commit in the Git log", "")
		return
	}
	m.vcsOps = VCSOperationPane{kind: "operation", open: true, title: "Cherry-Pick"}
	m.runVCSGitOperation("Cherry-Pick", map[string]any{
		"op": "cherryPick", "commits": []string{commit}, "recordOrigin": false,
	})
}

// RevertSelectedCommit applies the inverse of the commit selected in Git Log.
func (m *Model) RevertSelectedCommit() {
	commit := m.selectedGitCommit()
	if commit == "" {
		m.notifyInfo("git-revert", "Select a commit in the Git log", "")
		return
	}
	m.vcsOps = VCSOperationPane{kind: "operation", open: true, title: "Revert Commit"}
	m.runVCSGitOperation("Revert", map[string]any{"op": "revert", "commits": []string{commit}})
}

func (m *Model) vcsOperationDialog(c *ui.Context, p *ui.Theme) {
	state := &m.vcsOps
	if !state.open {
		return
	}
	if state.remotes != nil {
		m.vcsRemotesDialog(c, p, state.remotes)
		return
	}
	open := state.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).Gap(10).Children(func() {
			ui.Text(c, state.title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if state.hint != "" {
				ui.Text(c, state.hint).FontSize(12).TextColor(p.TextMuted)
			}
			if state.loading {
				ui.Text(c, "Reading revisions…").FontSize(12).TextColor(p.TextMuted)
			}
			if state.label != "" && state.kind != vcsOpReset {
				ui.Text(c, state.label).FontSize(12).TextColor(p.TextMuted)
				in := ui.TextInput(c, &state.name).AutoFocus()
				if in.Submitted() {
					m.submitVCSOperation()
				}
			}
			if state.kind == vcsOpReset {
				modes := []string{"Soft", "Mixed", "Hard", "Keep"}
				ui.Text(c, "Mode").FontSize(12).TextColor(p.TextMuted)
				mode := modes[clampSymbolSel(state.resetMode, len(modes))]
				if choice := ui.Select(c, &mode, modes).Width(180); choice.Changed() {
					for i, value := range modes {
						if value == mode {
							state.resetMode = i
							break
						}
					}
					state.err = ""
				}
			}
			if state.kind == vcsOpNewTag || state.kind == vcsOpStash {
				label := "Tag message (optional)"
				if state.kind == vcsOpStash {
					label = "Stash message (optional)"
				}
				ui.Text(c, label).FontSize(12).TextColor(p.TextMuted)
				in := ui.TextInput(c, &state.message).Grow(1)
				if in.Submitted() {
					m.submitVCSOperation()
				}
			}
			if state.kind == vcsOpStash {
				ui.Checkbox(c, &state.includeUntracked, "Include untracked files")
				ui.Checkbox(c, &state.keepIndex, "Keep index")
			}
			if state.kind == vcsOpUnstash {
				ui.Checkbox(c, &state.keepIndex, "Reinstate index")
			}
			if state.kind == vcsOpPull {
				ui.Checkbox(c, &state.rebase, "Rebase incoming changes")
			}
			if state.kind == vcsOpPush {
				ui.Checkbox(c, &state.force, "Force push with lease")
				ui.Checkbox(c, &state.setUpstream, "Set upstream")
			}
			if state.kind == vcsOpReset {
				ui.Checkbox(c, &state.force, "I understand local changes may be discarded")
			}
			if len(state.choices) > 0 && state.kind != vcsOpReset {
				ui.Scroll(c).Height(210).Children(func() {
					ui.Column(c).Children(func() {
						for i, choice := range state.choices {
							i, choice := i, choice
							r := ui.Row(c.Key(fmt.Sprintf("vcs-choice:%s:%d", state.kind, i))).
								Height(26).Padding(0, 8).Gap(8).AlignItems(ui.Center)
							if i == state.sel {
								r.Background(p.SurfaceHover)
							} else if r.Hovered() {
								r.Background(p.SurfaceHover)
							}
							r.Children(func() {
								ui.Text(c, choice.label).FontSize(13).TextColor(p.Text).
									SingleLine().Ellipsis("…")
								ui.Spacer(c)
								if choice.detail != "" {
									ui.Text(c, choice.detail).FontSize(12).TextColor(p.TextMuted).SingleLine()
								}
							})
							if r.Clicked() {
								state.sel = i
							}
						}
					})
				})
			}
			if len(state.output) > 0 {
				ui.Scroll(c).Height(150).Padding(8).Background(p.Background).Children(func() {
					ui.Text(c, strings.Join(state.output, "\n")).FontSize(12).
						Font(oiFontMono).TextColor(p.TextMuted)
				})
			}
			if state.err != "" {
				ui.Text(c, state.err).FontSize(12).TextColor(p.Danger)
			}
			ui.Row(c).Gap(8).Children(func() {
				label := "OK"
				switch state.kind {
				case vcsOpNewBranch:
					label = "Create Branch"
				case vcsOpNewTag:
					label = "Create Tag"
				case vcsOpMerge:
					label = "Merge"
				case vcsOpRebase:
					label = "Rebase"
				case vcsOpStash:
					label = "Stash"
				case vcsOpUnstash:
					label = "Apply Stash"
				case vcsOpPull:
					label = "Pull"
				case vcsOpPush:
					label = "Push"
				case vcsOpReset:
					label = "Reset"
				}
				if state.busy {
					label = "Running…"
				}
				ui.Button(c, label).Disabled(state.loading || state.busy || len(state.err) > 0 && state.kind == vcsOpUnstash).
					OnClick(m.submitVCSOperation)
				if state.busy {
					ui.Button(c, "Stop").OnClick(func() {
						id := state.request
						if id == 0 {
							return
						}
						go func() {
							ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
							defer cancel()
							_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": id})
						}()
					})
				}
				ui.Button(c, "Close").Disabled(state.busy).OnClick(m.closeVCSOperation)
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeVCSOperation()
	}
}

// SmokeVCSOperations proves the Git operations added to the native UI against
// the real backend and a local bare remote. It leaves the repository clean.
func (m *Model) SmokeVCSOperations(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("No project is open")
	}
	out := map[string]any{"root": root}
	if _, err := m.invoke(ctx, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return out, err
	}
	info, err := m.branchInfo(ctx)
	if err != nil {
		return out, err
	}
	if info.Head == nil || info.HeadCommit == nil {
		return out, errors.New("VCS smoke needs a branch with one commit")
	}
	original, branch := *info.HeadCommit, *info.Head
	out["branch"] = branch
	out["head"] = original

	tag := "goidea-smoke-" + fmt.Sprint(time.Now().UnixNano())
	if _, err := m.invoke(ctx, "vcs_create_tag", map[string]any{
		"repo": nil, "name": tag, "commit": original, "message": "GoIDEA smoke tag", "force": false,
	}); err != nil {
		return out, err
	}
	tagRaw, tagErr := m.invoke(ctx, "vcs_tags", map[string]any{"repo": nil})
	var tags []string
	if tagErr == nil {
		tagErr = json.Unmarshal(tagRaw, &tags)
	}
	out["tagCreated"] = tagErr == nil && containsString(tags, tag)
	if tagErr == nil {
		_, _ = m.invoke(ctx, "vcs_delete_tag", map[string]any{"repo": nil, "name": tag})
	}

	file := filepath.Join(root, "goidea-vcs-smoke.txt")
	if err := osWriteFile(file, []byte("stash smoke\n")); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_stash_push", map[string]any{
		"repo": nil, "message": "GoIDEA smoke stash", "includeUntracked": true, "keepIndex": false,
	}); err != nil {
		return out, err
	}
	stashRaw, stashErr := m.invoke(ctx, "vcs_stashes", map[string]any{"repo": nil})
	var stashes []openidea.Stash
	if stashErr == nil {
		stashErr = json.Unmarshal(stashRaw, &stashes)
	}
	out["stashCreated"] = stashErr == nil && len(stashes) > 0
	if stashErr == nil && len(stashes) > 0 {
		index := stashes[0].Index
		if _, err := m.invoke(ctx, "vcs_stash_apply", map[string]any{
			"repo": nil, "index": index, "pop": true, "reinstateIndex": false, "branch": nil,
		}); err != nil {
			return out, err
		}
		restored, err := os.ReadFile(file)
		if err != nil {
			return out, fmt.Errorf("read the unstashed working-tree change: %w", err)
		}
		if string(restored) != "stash smoke\n" {
			return out, fmt.Errorf("unstash restored %q, want the original change", restored)
		}
		out["stashRestored"] = true
		_, _ = m.invoke(ctx, "vcs_discard", map[string]any{"paths": []string{file}})
	}

	remoteDir := filepath.Join(filepath.Dir(root), "goidea-vcs-smoke-remote.git")
	remoteName := "goidea-smoke"
	if err := setupVCSRemote(ctx, root, remoteDir, remoteName, branch); err != nil {
		return out, err
	}
	remoteCh := m.nextChannel()
	remoteRequest := m.nextRequest()
	if _, err := m.invoke(ctx, "vcs_remote", map[string]any{
		"repo": nil, "request": remoteRequest, "op": map[string]any{"op": "fetch"},
		"onOutput": map[string]any{"$channel": remoteCh},
	}); err != nil {
		return out, err
	}
	pushCh := m.nextChannel()
	pushRequest := m.nextRequest()
	if _, err := m.invoke(ctx, "vcs_remote", map[string]any{
		"repo": nil, "request": pushRequest,
		"op": map[string]any{
			"op": "pushBranch", "branch": branch, "revision": nil,
			"force_with_lease": false, "target": branch, "remote": remoteName,
			"force": false, "set_upstream": true, "no_verify": true,
		},
		"onOutput": map[string]any{"$channel": pushCh},
	}); err != nil {
		return out, err
	}
	out["fetched"] = true
	out["pushed"] = true

	if err := commitVCSSmokeRemote(ctx, remoteDir, branch); err != nil {
		return out, err
	}
	pullCh := m.nextChannel()
	pullRequest := m.nextRequest()
	if _, err := m.invoke(ctx, "vcs_remote", map[string]any{
		"repo": nil, "request": pullRequest,
		"op":       map[string]any{"op": "pull", "rebase": false},
		"onOutput": map[string]any{"$channel": pullCh},
	}); err != nil {
		return out, err
	}
	if _, err := os.Stat(filepath.Join(root, "goidea-vcs-pull-smoke.txt")); err != nil {
		return out, fmt.Errorf("pull did not update the worktree: %w", err)
	}
	out["pulled"] = true

	info, err = m.branchInfo(ctx)
	if err != nil {
		return out, err
	}
	if info.HeadCommit == nil {
		return out, errors.New("VCS smoke lost HEAD after pull")
	}
	rebaseStart := *info.HeadCommit
	feature := "goidea-smoke-" + fmt.Sprint(time.Now().UnixNano())
	if _, err := m.invoke(ctx, "vcs_create_branch", map[string]any{
		"repo": nil, "name": feature, "start": rebaseStart, "checkout": true, "overwrite": nil,
	}); err != nil {
		return out, err
	}
	out["branchCreated"] = true
	if err := osWriteFile(file, []byte("merge smoke\n")); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_stage", map[string]any{"paths": []string{file}}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_commit", map[string]any{
		"repo": nil, "message": "VCS operation smoke",
		"options": map[string]any{
			"amend": false, "signOff": false, "author": nil,
			"runHooks": false, "stage": []string{file},
		},
	}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_checkout", map[string]any{
		"repo": nil, "branch": branch, "create": false, "options": nil,
	}); err != nil {
		return out, err
	}
	mainOnly := filepath.Join(root, "goidea-vcs-main-smoke.txt")
	if err := osWriteFile(mainOnly, []byte("main moved after the feature\n")); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_stage", map[string]any{"paths": []string{mainOnly}}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_commit", map[string]any{
		"repo": nil, "message": "VCS rebase smoke on main",
		"options": map[string]any{
			"amend": false, "signOff": false, "author": nil,
			"runHooks": false, "stage": []string{mainOnly},
		},
	}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_checkout", map[string]any{
		"repo": nil, "branch": feature, "create": false, "options": nil,
	}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_rebase_with", map[string]any{
		"repo": nil, "upstream": branch,
		"options": map[string]any{"onto": nil, "rebaseMerges": false, "keepEmpty": false},
	}); err != nil {
		return out, err
	}
	out["rebased"] = true
	if _, err := m.invoke(ctx, "vcs_checkout", map[string]any{
		"repo": nil, "branch": branch, "create": false, "options": nil,
	}); err != nil {
		return out, err
	}
	if _, err := m.invoke(ctx, "vcs_merge_with", map[string]any{
		"repo": nil, "branches": []string{feature},
		"options": map[string]any{
			"noFf": false, "ffOnly": false, "squash": false,
			"noCommit": false, "message": nil,
		},
	}); err != nil {
		return out, err
	}
	out["merged"] = true
	opCh := m.nextChannel()
	opRequest := m.nextRequest()
	if _, err := m.invoke(ctx, "vcs_operation", map[string]any{
		"repo": nil, "request": opRequest,
		"op":       map[string]any{"op": "reset", "commit": original, "mode": "hard"},
		"onOutput": map[string]any{"$channel": opCh},
	}); err != nil {
		return out, err
	}
	info, err = m.branchInfo(ctx)
	if err != nil {
		return out, err
	}
	if info.HeadCommit == nil || *info.HeadCommit != original {
		return out, fmt.Errorf("hard reset left HEAD at %v, want %s", info.HeadCommit, original)
	}
	out["reset"] = true
	_, _ = m.invoke(ctx, "vcs_delete_branch", map[string]any{"repo": nil, "branch": feature})
	_, _ = m.invoke(ctx, "vcs_remote", map[string]any{
		"repo": nil, "request": m.nextRequest(),
		"op":       map[string]any{"op": "deleteBranch", "branch": remoteName + "/" + branch},
		"onOutput": map[string]any{"$channel": m.nextChannel()},
	})
	_ = exec.CommandContext(ctx, "git", "-C", root, "remote", "remove", remoteName).Run()
	if err := os.RemoveAll(remoteDir); err != nil {
		return out, err
	}
	if err := os.RemoveAll(remoteDir + "-work"); err != nil {
		return out, err
	}
	return out, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func setupVCSRemote(ctx context.Context, root, remoteDir, remoteName, branch string) error {
	if err := exec.CommandContext(ctx, "git", "init", "--bare", remoteDir).Run(); err != nil {
		return fmt.Errorf("prepare the VCS smoke remote: %w", err)
	}
	command := exec.CommandContext(ctx, "git", "-C", root, "remote", "add", remoteName, remoteDir)
	if err := command.Run(); err != nil {
		command = exec.CommandContext(ctx, "git", "-C", root, "remote", "set-url", remoteName, remoteDir)
		if err := command.Run(); err != nil {
			return fmt.Errorf("configure the VCS smoke remote: %w", err)
		}
	}
	_ = branch
	return nil
}

// commitVCSSmokeRemote advances the local bare remote through a second clone,
// so vcs_remote Pull has a real upstream commit to fetch and merge.
func commitVCSSmokeRemote(ctx context.Context, remoteDir, branch string) error {
	work := remoteDir + "-work"
	if err := os.RemoveAll(work); err != nil {
		return err
	}
	run := func(dir string, args ...string) error {
		command := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	if err := run(filepath.Dir(remoteDir), "clone", "--branch", branch, remoteDir, work); err != nil {
		return fmt.Errorf("prepare the VCS pull fixture: %w", err)
	}
	if err := run(work, "config", "user.name", "GoIDEA smoke"); err != nil {
		return err
	}
	if err := run(work, "config", "user.email", "goidea-smoke@invalid"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(work, "goidea-vcs-pull-smoke.txt"), []byte("remote update\n"), 0o644); err != nil {
		return err
	}
	if err := run(work, "add", "goidea-vcs-pull-smoke.txt"); err != nil {
		return err
	}
	if err := run(work, "commit", "-m", "VCS pull smoke"); err != nil {
		return err
	}
	if err := run(work, "push", "origin", branch); err != nil {
		return err
	}
	return nil
}
