package app

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// VCSPane is the sidebar's Git view: the changes `git status` reports for every
// repository of the project, loaded through openidea-server.
type VCSPane struct {
	loading bool
	loaded  bool
	files   []openidea.FileStatus
	err     string
}

func (m *Model) resetVCS() { m.vcs = VCSPane{} }

// RefreshVCS reloads the project's changes in the background.
func (m *Model) RefreshVCS() {
	root := m.Root()
	if root == "" || m.vcs.loading {
		return
	}
	m.vcs.loading = true
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "vcs_status", nil)
		var files []openidea.FileStatus
		if err == nil {
			err = json.Unmarshal(raw, &files)
		}
		m.post(func() {
			m.vcs.loading = false
			if m.Root() != root {
				return
			}
			m.vcs.loaded = true
			if err != nil {
				m.vcs.err = vcsErrorText(err)
				m.vcs.files = nil
				m.invalidate()
				return
			}
			m.vcs.err = ""
			sort.SliceStable(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
			m.vcs.files = files
			m.invalidate()
		})
	}()
}

// ShowVCS switches the sidebar to the changes view (and reloads it).
func (m *Model) ShowVCS() {
	m.sideMode = "vcs"
	m.RefreshVCS()
	m.invalidate()
}

// SmokeVCS reports the project's Git changes through the backend: the
// offscreen check of the changes view.
func (m *Model) SmokeVCS(ctx context.Context) (int, string, error) {
	raw, err := m.mgr.Invoke(ctx, Window, "vcs_status", nil)
	if err != nil {
		return 0, "", err
	}
	var files []openidea.FileStatus
	if err := json.Unmarshal(raw, &files); err != nil {
		return 0, "", err
	}
	sample := ""
	if len(files) > 0 {
		letter, word, _ := vcsBadge(files[0])
		sample = letter + " " + word + " " + files[0].RelPath
	}
	return len(files), sample, nil
}

// vcsErrorText turns the backend's refusal for a project without Git into the
// sentence the sidebar shows; other failures keep their own message.
func vcsErrorText(err error) string {
	var oe *openidea.Error
	if errors.As(err, &oe) && oe.Kind == "unavailable" && strings.Contains(oe.Message, "not a git repository") {
		return "当前项目不是 Git 仓库"
	}
	return "读取 Git 状态失败：" + err.Error()
}

// vcsBadge is the label of a status row: a letter as IDEA shows it, the
// change in words, and a tone for the colour. The working tree kind wins when
// both sides changed, since that is the state the file is in now.
func vcsBadge(f openidea.FileStatus) (letter, word, tone string) {
	kind := ""
	if f.Unstaged != nil {
		kind = *f.Unstaged
	} else if f.Staged != nil {
		kind = *f.Staged
	}
	switch kind {
	case "added":
		return "A", "新增", "add"
	case "modified":
		return "M", "已修改", "mod"
	case "deleted":
		return "D", "已删除", "del"
	case "renamed":
		return "R", "已重命名", "add"
	case "conflicted":
		return "!", "冲突", "del"
	case "untracked":
		return "U", "未跟踪", "mod"
	case "ignored":
		return "I", "已忽略", "muted"
	}
	return "?", "已更改", "mod"
}
