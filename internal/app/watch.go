package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/editor"
)

// watchInterval paces the disk check: a stat of every open file twice a
// second would cost more than it finds.
const watchInterval = 2 * time.Second

// maxReloadSize skips external rewrites too large to pull into a buffer
// without a visible pause.
const maxReloadSize = 4 << 20

// noteDisk records the file as the buffer last matched it.
func (t *Tab) noteDisk() {
	t.diskTime, t.diskSize = diskStamp(t.doc.Path())
}

// changedOutside reports whether another program wrote the file since the
// buffer last matched it.
func (t *Tab) changedOutside() bool {
	mod, size := diskStamp(t.doc.Path())
	return stampChanged(t.diskTime, t.diskSize, mod, size)
}

// diskStamp is the file's modification time and size; zero time when it
// cannot be read, which reads as unchanged.
func diskStamp(path string) (time.Time, int64) {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}, 0
	}
	return fi.ModTime(), fi.Size()
}

func stampChanged(before time.Time, beforeSize int64, now time.Time, nowSize int64) bool {
	if before.IsZero() || now.IsZero() {
		return false
	}
	return !now.Equal(before) || nowSize != beforeSize
}

// watchFiles notices files changed by another program. A clean buffer takes
// the new text; a buffer with unsaved edits keeps them and says so in the
// banner, since reloading would silently drop the user's work.
func (m *Model) watchFiles(now time.Time) {
	if now.Sub(m.watched) < watchInterval {
		return
	}
	m.watched = now
	for _, t := range m.tabs {
		if t.reloading || !t.changedOutside() {
			continue
		}
		if t.unsaved() || t.syncing {
			t.noteDisk()
			m.banner = "文件已在外部修改：" + filepath.Base(t.doc.Path()) + "（保留了编辑器中的内容）"
			m.invalidate()
			continue
		}
		m.reloadTab(t)
	}
}

// ReloadAllFromDisk rereads every clean buffer from disk, as File › Reload All
// from Disk does. A buffer with unsaved edits keeps them and says so, since
// rereading would silently drop the user's work.
func (m *Model) ReloadAllFromDisk() {
	n := 0
	for _, t := range m.tabs {
		if t.reloading || t.syncing {
			continue
		}
		if t.unsaved() {
			m.banner = "文件有未保存的修改，未从磁盘重新加载：" + filepath.Base(t.doc.Path())
			continue
		}
		m.reloadTab(t)
		n++
	}
	if n == 0 {
		m.status = "没有可从磁盘重新加载的文件"
	} else {
		m.status = fmt.Sprintf("正在从磁盘重新加载 %d 个文件…", n)
	}
	m.invalidate()
}

// reloadTab rereads a clean buffer from disk after an external rewrite.
func (m *Model) reloadTab(t *Tab) {
	path := t.doc.Path()
	if fi, err := os.Stat(path); err != nil || fi.Size() > maxReloadSize {
		t.noteDisk()
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	t.reloading = true
	text := string(data)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		err := t.doc.ApplyEdits(ctx, editor.Edit{Start: 0, End: len(t.doc.Text()), Text: text})
		m.post(func() {
			t.reloading = false
			if err != nil {
				m.fail(path, err)
				return
			}
			t.noteDisk()
			t.seen = t.doc.Buffer().Version()
			if m.banner != "" && strings.Contains(m.banner, filepath.Base(path)) {
				m.banner = ""
			}
			m.status = "已重新加载 " + filepath.Base(path)
			m.invalidate()
		})
	}()
}

// SmokeWatch changes an open file behind the app's back and checks that the
// clean buffer picks the new text up, then restores the file: the offscreen
// check of external reloads.
func (m *Model) SmokeWatch(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	root := m.Root()
	if root == "" {
		return out, errors.New("没有打开项目")
	}
	need := false
	mygo.RunOnMain(func() { need = m.tab() == nil })
	if need {
		candidate := findSmokeFile(root)
		if candidate == "" {
			return out, errors.New("项目里没有可打开的文件")
		}
		mygo.RunOnMain(func() { m.openFile(candidate) })
		if !m.waitDebug(ctx, func(*DebugPane) bool { return m.tab() != nil }, 60*time.Second) {
			return out, errors.New("打开文件超时")
		}
	}
	var t *Tab
	var before string
	mygo.RunOnMain(func() {
		t = m.tab()
		if t != nil {
			before = t.doc.Text()
		}
	})
	if t == nil {
		return out, errors.New("没有可检查的文件")
	}
	path := t.doc.Path()
	out["path"] = path
	deadline := time.Now().Add(30 * time.Second)
	for {
		var clean bool
		mygo.RunOnMain(func() { clean = !t.unsaved() })
		if clean {
			break
		}
		if time.Now().After(deadline) {
			return out, errors.New("缓冲区一直没有同步完成")
		}
		time.Sleep(150 * time.Millisecond)
	}
	marker := "\n// goidea-watch-smoke\n"
	if err := os.WriteFile(path, []byte(before+marker), 0o644); err != nil {
		return out, err
	}
	deadline = time.Now().Add(30 * time.Second)
	reloaded := false
	for {
		var text string
		mygo.RunOnMain(func() { text = t.doc.Text() })
		if strings.Contains(text, "goidea-watch-smoke") {
			reloaded = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	out["reloaded"] = reloaded
	out["lengthBefore"] = len(before)

	// Put both the buffer and the file back, so the fixture is as it was.
	rctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	err := t.doc.ApplyEdits(rctx, editor.Edit{Start: 0, End: len(t.doc.Text()), Text: before})
	cancel()
	if err != nil {
		out["restoreError"] = err.Error()
		return out, nil
	}
	mygo.RunOnMain(func() { m.saveTabAsync(t, nil) })
	deadline = time.Now().Add(30 * time.Second)
	for {
		var clean, saving bool
		mygo.RunOnMain(func() {
			clean, saving = !t.unsaved(), t.saving
		})
		if clean && !saving {
			out["restored"] = true
			break
		}
		if time.Now().After(deadline) {
			out["restoreError"] = "保存恢复内容超时"
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	// File › Reload All from Disk rereads the clean buffer from the file.
	marker2 := "\n// goidea-reload-all-smoke\n"
	if err := os.WriteFile(path, []byte(before+marker2), 0o644); err != nil {
		out["reloadAllError"] = err.Error()
		return out, nil
	}
	mygo.RunOnMain(m.ReloadAllFromDisk)
	deadline = time.Now().Add(30 * time.Second)
	reloadedAll := false
	for {
		var text string
		mygo.RunOnMain(func() { text = t.doc.Text() })
		if strings.Contains(text, "goidea-reload-all-smoke") {
			reloadedAll = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	out["reloadAll"] = reloadedAll
	rctx, cancel = context.WithTimeout(m.ctx, 30*time.Second)
	err = t.doc.ApplyEdits(rctx, editor.Edit{Start: 0, End: len(t.doc.Text()), Text: before})
	cancel()
	if err != nil {
		out["reloadAllError"] = err.Error()
		return out, nil
	}
	mygo.RunOnMain(func() { m.saveTabAsync(t, nil) })
	deadline = time.Now().Add(30 * time.Second)
	for {
		var clean, saving bool
		mygo.RunOnMain(func() {
			clean, saving = !t.unsaved(), t.saving
		})
		if clean && !saving {
			out["reloadAllRestored"] = true
			break
		}
		if time.Now().After(deadline) {
			out["reloadAllError"] = "恢复重新加载前的文件超时"
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	return out, nil
}
