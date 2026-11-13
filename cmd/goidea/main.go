// Command goidea starts GoIDEA: a native window over openidea-server, which
// owns the Java project model, with MyGo's editor over the documents it opens.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"goidea/internal/app"
)

func main() {
	mygo.App.SetName("GoIDEA")

	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Fatalf("goidea: user data directory: %v", err)
	}
	home := filepath.Join(dataDir, "openidea-server")
	if err := os.MkdirAll(home, 0o755); err != nil {
		log.Fatalf("goidea: server home: %v", err)
	}

	serverPath, serverErr := resolveServer()
	logFile := openLog(filepath.Join(dataDir, "openidea-server.log"))

	model := app.New(app.Config{
		ServerPath:  serverPath,
		Home:        home,
		SessionPath: filepath.Join(dataDir, "session.json"),
		Log:         logFile,
	})
	if serverErr != nil {
		model.Notify(serverErr.Error())
	}
	if dir := os.Getenv("GOIDEA_OPEN_PROJECT"); dir != "" {
		model.OpenAtStartup(dir)
	}
	headless := os.Getenv("GOIDEA_HEADLESS") == "1"
	if headless {
		// An accessory app has no Dock icon and takes no focus.
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	}

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "GoIDEA",
			Width:     1280,
			Height:    820,
			MinWidth:  900,
			MinHeight: 560,
			StateKey:  "main",
			Hidden:    headless,
			Content:   ui.View(model.View),
		})
		model.Attach(win)
		mygo.App.SetMenu(menu(model))
		model.Start()
		if headless {
			go smoke(model)
		}
	})
	mygo.App.OnBeforeQuit(model.RequestQuit)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// menu is the app's menu bar.
func menu(m *app.Model) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "打开项目…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { m.ChooseProject() }},
			{Label: "快速打开…", Accelerator: "CmdOrCtrl+P", Click: func(*mygo.MenuItem, *mygo.Window) { m.OpenQuickFiles() }},
			{Label: "命令面板…", Accelerator: "CmdOrCtrl+Shift+P", Click: func(*mygo.MenuItem, *mygo.Window) { m.OpenCommands() }},
			{Label: "保存", Accelerator: "CmdOrCtrl+S", Click: func(*mygo.MenuItem, *mygo.Window) { m.SaveActive() }},
			{Label: "显示 Git 更改", Accelerator: "CmdOrCtrl+Shift+G", Click: func(*mygo.MenuItem, *mygo.Window) { m.ShowVCS() }},
			{Label: "与 HEAD 比较", Accelerator: "CmdOrCtrl+Shift+D", Click: func(*mygo.MenuItem, *mygo.Window) { m.CompareActive() }},
			{Label: "在项目中查找", Accelerator: "CmdOrCtrl+Shift+F", Click: func(*mygo.MenuItem, *mygo.Window) { m.ShowSearch() }},
			mygo.Separator(),
			{Label: "退出 GoIDEA", Accelerator: "CmdOrCtrl+Q", Click: func(*mygo.MenuItem, *mygo.Window) { m.Quit() }},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: "代码", Submenu: []*mygo.MenuItem{
			{Label: "意图操作…", Accelerator: "Alt+Enter", Click: func(*mygo.MenuItem, *mygo.Window) { m.ShowIntentions() }},
			{Label: "转到符号…", Accelerator: "CmdOrCtrl+Shift+O", Click: func(*mygo.MenuItem, *mygo.Window) { m.OpenSymbols() }},
			{Label: "查找用法", Accelerator: "Alt+F7", Click: func(*mygo.MenuItem, *mygo.Window) { m.FindUsages() }},
			{Label: "重命名符号", Accelerator: "Shift+F6", Click: func(*mygo.MenuItem, *mygo.Window) { m.RenameActive() }},
			mygo.Separator(),
			{Label: "撤销重构", Accelerator: "CmdOrCtrl+Alt+Z", Click: func(*mygo.MenuItem, *mygo.Window) { m.UndoRefactor() }},
			{Label: "重做重构", Accelerator: "CmdOrCtrl+Alt+Shift+Z", Click: func(*mygo.MenuItem, *mygo.Window) { m.RedoRefactor() }},
			mygo.Separator(),
			{Label: "格式化代码", Accelerator: "Alt+CmdOrCtrl+L", Click: func(*mygo.MenuItem, *mygo.Window) { m.FormatActive() }},
		}},
		{Label: "运行", Submenu: []*mygo.MenuItem{
			{Label: "运行", Accelerator: "F5", Click: func(*mygo.MenuItem, *mygo.Window) { m.Run() }},
			{Label: "调试", Accelerator: "F9", Click: func(*mygo.MenuItem, *mygo.Window) { m.Debug() }},
			{Label: "停止", Accelerator: "Shift+F5", Click: func(*mygo.MenuItem, *mygo.Window) { m.StopRun(); m.StopDebug() }},
			mygo.Separator(),
			{Label: "切换断点", Accelerator: "CmdOrCtrl+F8", Click: func(*mygo.MenuItem, *mygo.Window) { m.ToggleBreakpoint() }},
			{Label: "步过", Accelerator: "F8", Click: func(*mygo.MenuItem, *mygo.Window) { m.StepOver() }},
			{Label: "步入", Accelerator: "F7", Click: func(*mygo.MenuItem, *mygo.Window) { m.StepInto() }},
			{Label: "步出", Accelerator: "Shift+F8", Click: func(*mygo.MenuItem, *mygo.Window) { m.StepOut() }},
			{Label: "继续", Click: func(*mygo.MenuItem, *mygo.Window) { m.DebugContinue() }},
			{Label: "暂停", Click: func(*mygo.MenuItem, *mygo.Window) { m.DebugPause() }},
			mygo.Separator(),
			{Label: "显示运行窗口", Click: func(*mygo.MenuItem, *mygo.Window) { m.ShowRun() }},
			{Label: "显示调试窗口", Click: func(*mygo.MenuItem, *mygo.Window) { m.ShowDebug() }},
			{Label: "终端", Accelerator: "CmdOrCtrl+J", Click: func(*mygo.MenuItem, *mygo.Window) { m.ShowTerm() }},
			{Label: "信任此项目并启用运行", Click: func(*mygo.MenuItem, *mygo.Window) { m.TrustProject() }},
		}},
		{Role: mygo.RoleWindowMenu},
	})
}

// smoke waits for the project to load in a hidden window, records what it saw
// to GOIDEA_SMOKE_OUT, and quits: the offscreen check of a build.
func smoke(m *app.Model) {
	deadline := time.Now().Add(40 * time.Second)
	var root string
	var tabs int
	var backend bool
	for {
		mygo.RunOnMain(func() { root, tabs, backend = m.Snapshot() })
		if backend && tabs > 0 {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	fields := map[string]any{
		"root": root, "tabs": tabs, "backend": backend, "pid": os.Getpid(),
	}
	if os.Getenv("GOIDEA_SMOKE_EDIT") == "1" && tabs > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		edited, undone := m.SmokeRoundTrip(ctx)
		cancel()
		fields["edited"] = edited
		fields["undone"] = undone
	}
	if os.Getenv("GOIDEA_SMOKE_RUN") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		config, code, output, runErr := m.SmokeRun(ctx)
		cancel()
		fields["runConfig"] = config
		if code != nil {
			fields["exitCode"] = *code
		}
		fields["runOutput"] = tail(output, 4000)
		if runErr != nil {
			fields["runError"] = runErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_DEBUG") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		debug, debugErr := m.SmokeDebug(ctx)
		cancel()
		fields["debug"] = debug
		if debugErr != nil {
			fields["debugError"] = debugErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_SEARCH") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		search, searchErr := m.SmokeSearch(ctx)
		cancel()
		fields["search"] = search
		if searchErr != nil {
			fields["searchError"] = searchErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_TERM") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		term, termErr := m.SmokeTerm(ctx)
		cancel()
		fields["term"] = term
		if termErr != nil {
			fields["termError"] = termErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_CODEINTEL") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		intel, intelErr := m.SmokeCodeIntel(ctx)
		cancel()
		fields["codeIntel"] = intel
		if intelErr != nil {
			fields["codeIntelError"] = intelErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_LSP") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		lsp, lspErr := m.SmokeLSP(ctx)
		cancel()
		fields["lsp"] = lsp
		if lspErr != nil {
			fields["lspError"] = lspErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_DIFF") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		diff, diffErr := m.SmokeDiff(ctx)
		cancel()
		fields["diff"] = diff
		if diffErr != nil {
			fields["diffError"] = diffErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_COMMANDS") == "1" {
		fields["commands"] = m.SmokeCommands()
	}
	if os.Getenv("GOIDEA_SMOKE_WATCH") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		watch, watchErr := m.SmokeWatch(ctx)
		cancel()
		fields["watch"] = watch
		if watchErr != nil {
			fields["watchError"] = watchErr.Error()
		}
	}
	if root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		count, sample, vcsErr := m.SmokeVCS(ctx)
		cancel()
		fields["vcsChanges"] = count
		fields["vcsSample"] = sample
		if vcsErr != nil {
			fields["vcsError"] = vcsErr.Error()
		}
	}
	out, _ := json.MarshalIndent(fields, "", "  ")
	if path := os.Getenv("GOIDEA_SMOKE_OUT"); path != "" {
		_ = os.WriteFile(path, out, 0o644)
	}
	time.Sleep(500 * time.Millisecond)
	mygo.RunOnMain(m.QuitImmediately)
}

// tail is the last at most n bytes of s, cut at a rune boundary.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := len(s) - n
	for cut < len(s) && s[cut]&0xC0 == 0x80 {
		cut++
	}
	return s[cut:]
}

// resolveServer finds openidea-server: GOIDEA_SERVER first, then beside the
// executable, then on PATH. The app still opens without it, and says why.
func resolveServer() (string, error) {
	if p := os.Getenv("GOIDEA_SERVER"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
		return "", fmt.Errorf("GOIDEA_SERVER 指向的文件不存在：%s", p)
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "openidea-server")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	if p, err := exec.LookPath("openidea-server"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("未找到 openidea-server：请设置 GOIDEA_SERVER 或把它放到可执行文件旁")
}

func openLog(path string) *os.File {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	return f
}
