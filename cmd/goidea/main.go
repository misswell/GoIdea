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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/app"
)

func main() {
	started := time.Now()
	mygo.App.SetName("GoIDEA")

	// GOIDEA_DATA_DIR keeps a run's session, settings and server home apart
	// from the user's: the E2E harness uses a temporary one.
	if dir := os.Getenv("GOIDEA_DATA_DIR"); dir != "" {
		mygo.App.SetPath(mygo.PathUserData, dir)
	}
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

	cfg := app.Config{
		ServerPath:       serverPath,
		Home:             home,
		SessionPath:      filepath.Join(dataDir, "session.json"),
		SettingsPath:     filepath.Join(dataDir, "settings.json"),
		Log:              logFile,
		ServerMinVersion: serverMinVersion,
		ServerSHA256:     serverChecksum(serverPath),
	}
	// File › Invalidate Caches… scheduled what the backend must forget; the
	// caches go before the server starts and takes them into use.
	app.ApplyPendingInvalidation(cfg)
	model := app.New(cfg)
	if serverErr != nil {
		model.Notify(serverErr.Error())
	}
	if dir := os.Getenv("GOIDEA_OPEN_PROJECT"); dir != "" {
		model.OpenAtStartup(dir)
		if files := os.Getenv("GOIDEA_OPEN_FILES"); files != "" {
			model.OpenFilesAtStartup(filepath.SplitList(files))
		}
	}
	headless := os.Getenv("GOIDEA_HEADLESS") == "1"
	if headless {
		// An accessory app has no Dock icon and takes no focus.
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	}

	mygo.App.WhenReady(func() {
		readyMs := time.Since(started).Milliseconds()
		var firstFrame atomic.Int64
		content := ui.View(func(c *ui.Context) {
			if firstFrame.Load() == 0 {
				firstFrame.CompareAndSwap(0, time.Since(started).Milliseconds())
			}
			model.View(c)
		})
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "GoIDEA",
			Width:     1280,
			Height:    820,
			MinWidth:  900,
			MinHeight: 560,
			StateKey:  "main",
			Hidden:    headless,
			Content:   content,
		})
		model.Attach(win)
		mygo.App.SetMenu(model.MainMenu())
		model.Start()
		// Benchmark mode owns the window: the smoke driver would quit the app
		// before an idle sample could be taken.
		idle := benchIdle()
		var backendReady atomic.Int64
		if headless && idle == 0 {
			go smoke(model)
		}
		if idle > 0 {
			go benchWaitBackend(model, started, &backendReady)
			go benchWait(model, readyMs, &firstFrame, &backendReady, idle)
		}
	})
	mygo.App.OnBeforeQuit(model.RequestQuit)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// benchIdle is GOIDEA_BENCH_IDLE: keep the hidden window open for that many
// seconds after startup, so the benchmark can sample it, then quit.
func benchIdle() time.Duration {
	n, err := strconv.Atoi(os.Getenv("GOIDEA_BENCH_IDLE"))
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// benchWaitBackend records when the backend became running, for the startup
// budget. It polls the main thread rather than reading the model directly.
func benchWaitBackend(m *app.Model, started time.Time, ready *atomic.Int64) {
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		var up bool
		mygo.RunOnMain(func() { up = m.BackendState().Running })
		if up {
			ready.CompareAndSwap(0, time.Since(started).Milliseconds())
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// benchWait writes the startup numbers and the loaded session to
// GOIDEA_SMOKE_OUT once the idle window is over.
func benchWait(m *app.Model, readyMs int64, firstFrame *atomic.Int64, backendReady *atomic.Int64, idle time.Duration) {
	time.Sleep(idle)
	var root string
	var tabs int
	var backend bool
	mygo.RunOnMain(func() { root, tabs, backend = m.Snapshot() })
	fields := map[string]any{
		"pid":            os.Getpid(),
		"root":           root,
		"tabs":           tabs,
		"backend":        backend,
		"windowReadyMs":  readyMs,
		"firstFrameMs":   firstFrame.Load(),
		"backendReadyMs": backendReady.Load(),
		"idleSeconds":    int64(idle / time.Second),
	}
	if n, err := strconv.Atoi(os.Getenv("GOIDEA_BENCH_TASKS")); err == nil && n > 0 {
		fields["tasks"] = m.BenchConcurrentSearches(root, n)
	}
	if out, err := json.MarshalIndent(fields, "", "  "); err == nil {
		if path := os.Getenv("GOIDEA_SMOKE_OUT"); path != "" {
			_ = os.WriteFile(path, out, 0o644)
		}
	}
	mygo.RunOnMain(m.QuitImmediately)
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
	var phase string
	mygo.RunOnMain(func() { phase = m.Phase() })
	fields := map[string]any{
		"root": root, "tabs": tabs, "backend": backend, "pid": os.Getpid(), "phase": phase,
	}
	if mode := os.Getenv("GOIDEA_SMOKE_TOOLS"); mode != "" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		tools, toolsErr := m.SmokeTools(ctx, mode == "install")
		cancel()
		fields["tools"] = tools
		if toolsErr != nil {
			fields["toolsError"] = toolsErr.Error()
		}
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
	if os.Getenv("GOIDEA_SMOKE_BUILD") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		build, buildErr := m.SmokeBuild(ctx)
		cancel()
		fields["build"] = build
		if buildErr != nil {
			fields["buildError"] = buildErr.Error()
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
	if os.Getenv("GOIDEA_SMOKE_TODO") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		todo, todoErr := m.SmokeTodo(ctx)
		cancel()
		fields["todo"] = todo
		if todoErr != nil {
			fields["todoError"] = todoErr.Error()
		}
	}
	if other := os.Getenv("GOIDEA_SMOKE_SWITCH_TO"); other != "" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		sw, swErr := m.SmokeSwitch(ctx, other)
		cancel()
		fields["switch"] = sw
		if swErr != nil {
			fields["switchError"] = swErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_TRUST") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		tr, trErr := m.SmokeTrust(ctx)
		cancel()
		fields["trust"] = tr
		if trErr != nil {
			fields["trustError"] = trErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_SCAFFOLD") == "1" && root != "" {
		dir, err := os.MkdirTemp("", "goidea-scaffold-")
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			sc, scErr := m.SmokeScaffoldProject(ctx, dir)
			cancel()
			fields["scaffold"] = sc
			if scErr != nil {
				fields["scaffoldError"] = scErr.Error()
			}
			_ = os.RemoveAll(dir)
		}
	}
	if os.Getenv("GOIDEA_SMOKE_RECOVERY") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		rec, recErr := m.SmokeRecovery(ctx)
		cancel()
		fields["recovery"] = rec
		if recErr != nil {
			fields["recoveryError"] = recErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_DOCK") == "1" && root != "" {
		fields["dock"] = m.SmokeDock()
	}
	if os.Getenv("GOIDEA_SMOKE_SCRATCH") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		scratch, scratchErr := m.SmokeScratch(ctx)
		cancel()
		fields["scratch"] = scratch
		if scratchErr != nil {
			fields["scratchError"] = scratchErr.Error()
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
	if os.Getenv("GOIDEA_SMOKE_HIERARCHY") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		hierarchy, hierarchyErr := m.SmokeHierarchy(ctx)
		cancel()
		fields["hierarchy"] = hierarchy
		if hierarchyErr != nil {
			fields["hierarchyError"] = hierarchyErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_SE") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		se := m.SmokeSearchEverywhere(ctx, "App")
		cancel()
		fields["searchEverywhere"] = se
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
	if os.Getenv("GOIDEA_SMOKE_NOTIFY") == "1" {
		fields["notifications"] = m.SmokeNotifications()
	}
	if os.Getenv("GOIDEA_SMOKE_GOLDENS") == "1" {
		fields["goldens"] = m.SmokeGoldens(os.Getenv("GOIDEA_SHOTS_DIR"),
			os.Getenv("GOIDEA_GOLDEN_DIR"), os.Getenv("GOIDEA_GOLDEN_DIFF_DIR"))
	}
	if os.Getenv("GOIDEA_SMOKE_NEWPROJECT") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		fields["newProject"] = m.SmokeNewProject(ctx)
		cancel()
	}
	if os.Getenv("GOIDEA_SMOKE_SETTINGS") == "1" {
		fields["settings"] = m.SmokeSettings()
	}
	if os.Getenv("GOIDEA_SMOKE_DB") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		db, dbErr := m.SmokeDatabase(ctx)
		cancel()
		fields["database"] = db
		if dbErr != nil {
			fields["databaseError"] = dbErr.Error()
		}
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
	if os.Getenv("GOIDEA_SMOKE_FILES") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		files, filesErr := m.SmokeFiles(ctx)
		cancel()
		fields["files"] = files
		if filesErr != nil {
			fields["filesError"] = filesErr.Error()
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
	if os.Getenv("GOIDEA_SMOKE_BRANCH") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		branch, branchErr := m.SmokeBranch(ctx)
		cancel()
		fields["branch"] = branch
		if branchErr != nil {
			fields["branchError"] = branchErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_STAGE") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		stage, stageErr := m.SmokeStage(ctx)
		cancel()
		fields["stage"] = stage
		if stageErr != nil {
			fields["stageError"] = stageErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_BOOKMARK") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		bookmark, bookmarkErr := m.SmokeBookmark(ctx)
		cancel()
		fields["bookmark"] = bookmark
		if bookmarkErr != nil {
			fields["bookmarkError"] = bookmarkErr.Error()
		}
	}
	if os.Getenv("GOIDEA_SMOKE_LOG") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		logInfo, logErr := m.SmokeLog(ctx)
		cancel()
		fields["log"] = logInfo
		if logErr != nil {
			fields["logError"] = logErr.Error()
		}
	}
	// The commit goes last: the checks above read the working tree the project
	// started with, and committing changes it.
	if os.Getenv("GOIDEA_SMOKE_COMMIT") == "1" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		commit, commitErr := m.SmokeCommit(ctx)
		cancel()
		fields["commit"] = commit
		if commitErr != nil {
			fields["commitError"] = commitErr.Error()
		}
	}
	if dir := os.Getenv("GOIDEA_SMOKE_SHOTS"); dir != "" && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		shots, shotsErr := m.SmokeShots(ctx, dir)
		cancel()
		if files, ok := shots["shots"]; ok {
			fields["shots"] = files
		}
		if e, ok := shots["shotErrors"]; ok {
			fields["shotErrors"] = e
		}
		if shotsErr != nil {
			fields["shotsError"] = shotsErr.Error()
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
// executable, then among the bundled resources, then on PATH. The app still
// opens without it, and says why.
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
	// The app bundle ships the server under Contents/Resources.
	if dir, err := mygo.App.Path(mygo.PathResources); err == nil {
		p := filepath.Join(dir, "openidea-server")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	if p, err := exec.LookPath("openidea-server"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("未找到 openidea-server：请设置 GOIDEA_SERVER 或把它放到可执行文件旁")
}

// serverMinVersion is the OpenIDEA release this client speaks to; it moves
// with the pin in UPSTREAMS.yml and the shipped server.
const serverMinVersion = "0.1.44"

// serverChecksum reads the release checksum shipped beside the binary, when
// fetch-server.sh wrote one; a hand-placed server skips the comparison.
func serverChecksum(path string) string {
	data, err := os.ReadFile(path + ".sha256")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func openLog(path string) *os.File {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	return f
}
