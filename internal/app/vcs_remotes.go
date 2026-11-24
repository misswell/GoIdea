package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// vcsRemotesPane is owned by the VCS operations overlay. Keeping its identity
// across async requests prevents replies from an old dialog replacing a new one.
type vcsRemotesPane struct {
	items    []openidea.Remote
	selected string
	loading  bool
	busy     bool
	err      string
	mode     string
	name     string
	url      string
	original string
}

// OpenGitRemotes shows the current repository's configured remotes.
func (m *Model) OpenGitRemotes() {
	if m.Root() == "" {
		return
	}
	state := &vcsRemotesPane{}
	m.vcsOps = VCSOperationPane{kind: "remotes", open: true, remotes: state}
	m.loadGitRemotes(state)
}

func (m *Model) loadGitRemotes(state *vcsRemotesPane) {
	root := m.Root()
	repoArg := m.vcsRepositoryArg()
	state.loading = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_remotes", map[string]any{"repo": repoArg})
		var items []openidea.Remote
		if err == nil {
			err = json.Unmarshal(raw, &items)
		}
		m.post(func() {
			if m.Root() != root || m.vcsOps.remotes != state || !m.vcsOps.open {
				return
			}
			state.loading = false
			if err != nil {
				state.err = "Could not read remotes: " + err.Error()
				m.invalidate()
				return
			}
			state.items = items
			found := false
			for _, item := range items {
				if item.Name == state.selected {
					found = true
					break
				}
			}
			if !found {
				state.selected = ""
				if len(items) > 0 {
					state.selected = items[0].Name
				}
			}
			m.invalidate()
		})
	}()
}

func validateRemoteName(name, original string, items []openidea.Remote) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Enter a remote name"
	}
	if strings.HasPrefix(name, "-") || strings.Contains(name, "..") ||
		strings.ContainsAny(name, "~^:?*[\\") || strings.IndexFunc(name, unicode.IsSpace) >= 0 {
		return "Invalid remote name"
	}
	for _, item := range items {
		if item.Name == name && item.Name != original {
			return fmt.Sprintf("Remote '%s' already exists", name)
		}
	}
	return ""
}

func (s *vcsRemotesPane) selectedRemote() *openidea.Remote {
	for i := range s.items {
		if s.items[i].Name == s.selected {
			return &s.items[i]
		}
	}
	return nil
}

func (s *vcsRemotesPane) editRemote(remote *openidea.Remote) {
	s.mode, s.name, s.url, s.original, s.err = "add", "", "", "", ""
	if remote != nil {
		s.mode, s.name, s.url, s.original = "edit", remote.Name, remote.URL, remote.Name
		return
	}
	if validateRemoteName("origin", "", s.items) == "" {
		s.name = "origin"
	}
}

func (m *Model) submitGitRemote(state *vcsRemotesPane) {
	if state.busy || state.loading {
		return
	}
	if state.mode != "add" && state.mode != "edit" && state.mode != "remove" {
		return
	}
	edit := map[string]any{"op": state.mode}
	if state.mode == "remove" {
		if state.selectedRemote() == nil {
			return
		}
		edit["name"] = state.selected
	} else {
		name, url := strings.TrimSpace(state.name), strings.TrimSpace(state.url)
		if message := validateRemoteName(name, state.original, state.items); message != "" {
			state.err = message
			m.invalidate()
			return
		}
		if url == "" {
			state.err = "Enter the remote URL"
			m.invalidate()
			return
		}
		edit["name"], edit["url"] = name, url
		if state.mode == "edit" {
			edit["name"], edit["new_name"] = state.original, name
		}
	}
	root := m.Root()
	repoArg := m.vcsRepositoryArg()
	state.busy, state.err = true, ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		_, err := m.mgr.Invoke(ctx, Window, "vcs_edit_remote", map[string]any{"repo": repoArg, "edit": edit})
		m.post(func() {
			if m.Root() != root || m.vcsOps.remotes != state || !m.vcsOps.open {
				return
			}
			state.busy = false
			if err != nil {
				state.err = "Could not update remote: " + err.Error()
			} else {
				if name, ok := edit["new_name"].(string); ok {
					state.selected = name
				} else if edit["op"] == "add" {
					state.selected = edit["name"].(string)
				}
				state.mode = ""
			}
			// Rename can succeed before setting the URL fails. Always re-read.
			m.loadGitRemotes(state)
			m.RefreshBranch()
			if m.branch.open && !m.branch.listing {
				m.branch.listing = true
				m.loadBranches()
			}
		})
	}()
}

func (m *Model) vcsRemotesDialog(c *ui.Context, p *ui.Theme, state *vcsRemotesPane) {
	open := true
	dialog := ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).Gap(10).Children(func() {
			switch state.mode {
			case "add", "edit":
				title := "Define Remote"
				if state.mode == "edit" {
					title = "Edit Remote"
				}
				ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text)
				ui.Text(c, "Name").FontSize(12).TextColor(p.TextMuted)
				if in := ui.TextInput(c, &state.name).AutoFocus(); in.Submitted() {
					m.submitGitRemote(state)
				}
				ui.Text(c, "URL").FontSize(12).TextColor(p.TextMuted)
				if in := ui.TextInput(c, &state.url).Font(oiFontMono); in.Submitted() {
					m.submitGitRemote(state)
				}
				if state.err != "" {
					ui.Text(c, state.err).FontSize(12).TextColor(p.Danger)
				}
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c, "OK").Disabled(state.busy || state.loading).OnClick(func() { m.submitGitRemote(state) })
					ui.Button(c, "Cancel").Disabled(state.busy).OnClick(func() { state.mode, state.err = "", "" })
				})
			case "remove":
				ui.Text(c, "Remove Remote").FontSize(13).Bold().TextColor(p.Text)
				ui.Text(c, fmt.Sprintf("Remove remote '%s'? Its remote-tracking branches are deleted.", state.selected)).
					FontSize(12).TextColor(p.Text)
				if state.err != "" {
					ui.Text(c, state.err).FontSize(12).TextColor(p.Danger)
				}
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c, "Remove").Disabled(state.busy || state.loading).OnClick(func() { m.submitGitRemote(state) })
					ui.Button(c, "Cancel").Disabled(state.busy).OnClick(func() { state.mode, state.err = "", "" })
				})
			default:
				ui.Text(c, "Git Remotes").FontSize(13).Bold().TextColor(p.Text)
				ui.Row(c).Gap(6).Children(func() {
					ui.Button(c, "Add…").Disabled(state.loading || state.busy).OnClick(func() { state.editRemote(nil) })
					ui.Button(c, "Remove").Disabled(state.loading || state.busy || state.selectedRemote() == nil).
						OnClick(func() { state.mode, state.err = "remove", "" })
					ui.Button(c, "Edit…").Disabled(state.loading || state.busy || state.selectedRemote() == nil).
						OnClick(func() { state.editRemote(state.selectedRemote()) })
					ui.Button(c, "Refresh").Disabled(state.loading || state.busy).OnClick(func() {
						state.err = ""
						m.loadGitRemotes(state)
					})
				})
				ui.Scroll(c).Height(220).Children(func() {
					ui.Column(c).Children(func() {
						if state.loading {
							ui.Text(c, "Loading remotes…").FontSize(12).TextColor(p.TextMuted)
						} else if len(state.items) == 0 {
							ui.Text(c, "No remotes").FontSize(12).TextColor(p.TextMuted)
						}
						for _, remote := range state.items {
							remote := remote
							row := ui.Row(c.Key("git-remote:"+remote.Name)).Height(28).Padding(0, 8).
								Gap(12).AlignItems(ui.Center)
							if state.selected == remote.Name || row.Hovered() {
								row.Background(p.SurfaceHover)
							}
							row.Children(func() {
								ui.Text(c, remote.Name).FontSize(13).TextColor(p.Text).SingleLine()
								ui.Text(c, remote.URL).Font(oiFontMono).FontSize(12).TextColor(p.TextMuted).
									SingleLine().Ellipsis("…").Grow(1)
							})
							if row.DoubleClicked() && !state.busy && !state.loading {
								state.selected = remote.Name
								state.editRemote(&remote)
							} else if row.Clicked() && !state.busy {
								state.selected = remote.Name
							}
						}
					})
				})
				if state.err != "" {
					ui.Text(c, state.err).FontSize(12).TextColor(p.Danger)
				}
				ui.Button(c, "OK").Disabled(state.busy).OnClick(m.closeVCSOperation)
			}
		})
	})
	if (dialog.Dismissed() || !open) && !state.busy {
		if state.mode != "" {
			state.mode, state.err = "", ""
		} else {
			m.closeVCSOperation()
		}
	}
}
