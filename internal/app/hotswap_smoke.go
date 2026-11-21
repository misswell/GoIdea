package app

// The HotSwap (Reload Changed Classes) acceptance against the real backend:
// the debugger pauses at a breakpoint before the println, the source file
// changes on disk, HotSwapReload compiles with jdtls and redefines the class
// in the java-debug session, and after resuming the program prints the NEW
// text — the reloaded code actually ran.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// SmokeHotSwap debugs App.java, pauses at the println's breakpoint, edits
// the printed text, reloads the changed classes, resumes and requires the
// new text in the program's output.
func (m *Model) SmokeHotSwap(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"root": m.Root()}
	root := m.Root()
	if root == "" {
		return out, errors.New("没有打开项目")
	}
	greeter := root + "/src/main/java/com/example/Greeter.java"
	// The Greeter document joins the tabs, so the edit below reaches jdtls
	// through the document bridge the way a user's typing does.
	m.post(func() { m.openFile(greeter) })
	tabDeadline := time.Now().Add(60 * time.Second)
	opened := false
	for time.Now().Before(tabDeadline) {
		var found bool
		m.post(func() {
			for _, t := range m.tabs {
				if t.doc.Path() == greeter && strings.Contains(t.doc.Text(), `"Hello, " + name + "!"`) {
					found = true
				}
			}
		})
		if found {
			opened = true
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !opened {
		return out, errors.New("60 秒内没有打开 Greeter 文档")
	}
	// The debug context is the active tab: switch back to App.java (the
	// Greeter document stays open for the edit below).
	m.post(func() {
		for i, t := range m.tabs {
			if strings.HasSuffix(t.doc.Path(), "App.java") {
				m.activate(i)
				break
			}
		}
		m.invalidate()
	})
	// The active tab is App.java. Pause on the line that invokes Greeter.greet
	// before the JVM calls the old method; the reloaded class is Greeter.
	path := root + "/src/main/java/com/example/App.java"
	source, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	breakLine := lineContaining(string(source), "greeter.greet()")
	if breakLine < 0 {
		return out, errors.New("App.java has no Greeter.greet() call")
	}
	if _, err := m.mgr.Invoke(ctx, Window, "debug_set_breakpoints", map[string]any{
		"path": path, "breakpoints": []openidea.Breakpoint{{Line: breakLine, Enabled: true}},
	}); err != nil {
		return out, err
	}
	m.post(func() {
		m.debug.visible = true
		m.occupyBottom("debug")
		if m.debug.bps == nil {
			m.debug.bps = map[string][]int{}
		}
		m.debug.bps[path] = []int{breakLine}
		m.invalidate()
	})
	// Debug requires Java Ready (section 8): wait for the import.
	readyDeadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(readyDeadline) {
		if m.readinessNow(ctx).Java {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	m.refreshReadiness()
	time.Sleep(300 * time.Millisecond)
	m.post(func() {
		m.DebugContext()
	})
	deadline := time.Now().Add(4 * time.Minute)
	paused := false
	for time.Now().Before(deadline) {
		m.post(func() { paused = m.debug.paused })
		if paused {
			break
		}
		var dbgErr string
		m.post(func() { dbgErr = m.debug.err })
		if dbgErr != "" {
			return out, fmt.Errorf("调试启动失败：%s", dbgErr)
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !paused {
		return out, errors.New("4 分钟内没有在断点暂停")
	}
	out["paused"] = true
	if !m.waitDebug(ctx, func(d *DebugPane) bool {
		return d.paused && d.stopLine >= 0
	}, 30*time.Second) {
		return out, errors.New("30 秒内没有取得断点所在行")
	}
	var pausedLine int
	m.post(func() { pausedLine = m.debug.stopLine })
	out["breakpointLine"] = breakLine
	out["pausedLine"] = pausedLine
	if pausedLine != breakLine {
		return out, fmt.Errorf("断点停在第 %d 行，期望第 %d 行", pausedLine+1, breakLine+1)
	}

	var sess uint32
	var ended bool
	m.post(func() { sess, ended = m.debug.session, m.debug.ended })
	out["sessionAtHotSwap"] = sess
	out["endedAtHotSwap"] = ended
	// The program is suspended before the println: edit Greeter's buffer the
	// way the editor does, and reload the changed classes (HotSwapReload
	// saves the open documents first).
	edited := false
	m.post(func() {
		for _, t := range m.tabs {
			if t.doc.Path() != greeter {
				continue
			}
			text := t.doc.Text()
			old := `"Hello, " + name + "!"`
			if i := strings.Index(text, old); i >= 0 {
				t.doc.Buffer().Edit(editor.Edit{Start: i, End: i + len(old), Text: `"Hello, HotSwap!"`})
				edited = true
			}
		}
		m.invalidate()
	})
	out["edited"] = edited
	if !edited {
		return out, errors.New("没有找到要修改的 greet 方法")
	}
	m.HotSwapReload()
	reloadDeadline := time.Now().Add(2 * time.Minute)
	reloaded := false
	for time.Now().Before(reloadDeadline) {
		var status, dbgErr, build, stage string
		m.post(func() {
			status, dbgErr, build, stage = m.debug.status, m.debug.err, m.debug.hotSwapBuild, m.debug.hotSwapStage
		})
		out["hotSwapStage"] = stage
		if strings.Contains(status, "Reloaded ") || strings.Contains(dbgErr, "Hot Swap failed") || (build != "" && status != "Compiling the changed classes…") {
			reloaded = strings.Contains(status, "Reloaded ")
			out["reloadStatus"] = status
			out["reloadError"] = dbgErr
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	out["reloaded"] = reloaded
	if !reloaded {
		return out, errors.New("2 分钟内 HotSwap 没有报告类已重新加载")
	}

	// Resuming must print the NEW text: the reloaded code ran.
	m.DebugContinue()
	endDeadline := time.Now().Add(2 * time.Minute)
	endOfSession := false
	var output string
	for time.Now().Before(endDeadline) {
		m.post(func() { endOfSession = m.debug.ended; output = m.debug.text() })
		if endOfSession {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	out["ended"] = endOfSession
	out["hasNewText"] = strings.Contains(output, "Hello, HotSwap!")
	if !endOfSession {
		return out, errors.New("2 分钟内调试会话没有结束")
	}
	if !strings.Contains(output, "Hello, HotSwap!") {
		return out, fmt.Errorf("恢复后没有打印重载后的文本；输出尾部：%s", tailOf(output, 500))
	}
	return out, nil
}

func lineContaining(text, needle string) int {
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}
