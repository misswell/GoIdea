package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

const runContextContentLimit = 400_000

type phpComposerInstallState struct {
	Root   string `json:"root"`
	Reason string `json:"reason"`
}

type runContextModifyRequest struct {
	Path    string
	Content string
}

// runDynamicCommands are the concrete actions OpenIDEA derives from run
// scopes and tool settings. External tools add their own entries when settings
// change; these entries are stable and registered with the other built-ins.
func runDynamicCommands() []appCommand {
	commands := make([]appCommand, 0, 5)
	for _, scope := range []string{"file", "package"} {
		scope := scope
		title := "Go Generate File"
		if scope == "package" {
			title = "Go Generate Package"
		}
		commands = append(commands, appCommand{
			id:       "go.generate." + scope,
			title:    title,
			group:    "Go",
			keywords: "go generate code generation directives package file",
			run: func(m *Model) {
				m.RunGoGenerate(scope, nil)
			},
		})
	}
	return append(commands,
		appCommand{
			id:       "php.composerInstall",
			title:    "Composer Install",
			group:    "PHP",
			keywords: "composer install dependencies vendor lock php",
			run:      func(m *Model) { m.ComposerInstall(nil) },
		},
		appCommand{
			id:       "run.modifyContext",
			title:    "Modify Run Configuration…",
			group:    "Run",
			keywords: "modify edit context run configuration temporary",
			run:      func(m *Model) { m.ModifyRunContext(nil) },
		},
		appCommand{
			id:       "run.sampleOutput",
			title:    "Stream Sample Output",
			group:    "Run",
			keywords: "sample output performance console stream",
			run:      func(m *Model) { m.StreamSampleOutput(nil) },
		},
	)
}

func runActionPath(m *Model, arg any) string {
	path := strings.TrimSpace(projectPathArg(arg))
	if path == "" {
		if tab := m.tab(); tab != nil && tab.doc != nil {
			path = tab.doc.Path()
		}
	}
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.Root(), path)
	}
	return filepath.Clean(path)
}

func goGenerateConfig(path, scope, directive string) (*openidea.RunConfig, error) {
	path = filepath.Clean(path)
	if !strings.HasSuffix(strings.ToLower(path), ".go") ||
		strings.ContainsRune(path, '\x00') ||
		(scope != "file" && scope != "package") {
		return nil, errors.New("Go Generate needs one Go file")
	}
	file := filepath.Base(path)
	target := file
	if scope == "package" {
		target = "."
	} else if strings.HasPrefix(file, "-") {
		target = "./" + file
	}
	pattern := ""
	if directive = strings.TrimSpace(directive); directive != "" {
		pattern = "^" + regexp.QuoteMeta(directive) + "$"
	}
	args := []string{"generate"}
	if pattern != "" {
		args = append(args, "-run", pattern)
	}
	args = append(args, target)
	cwd := filepath.Dir(path)
	return &openidea.RunConfig{
		ID:           fmt.Sprintf("go:generate:%s:%s:%s", scope, path, pattern),
		Name:         "go generate " + target,
		Kind:         "go",
		Command:      "go",
		Args:         args,
		Cwd:          &cwd,
		Env:          map[string]string{},
		Detected:     true,
		DebugAdapter: nil,
		Params:       map[string]any{"useTags": "true"},
	}, nil
}

// RunGoGenerate ports go.generate.file/package. The backend starts the
// resulting Go run configuration, so output, exit status and Stop all use the
// same native Run window as an ordinary `go generate`.
func (m *Model) RunGoGenerate(scope string, arg any) {
	root := m.Root()
	if root == "" {
		return
	}
	path := runActionPath(m, arg)
	if path == "" {
		m.run.err = "Open a Go file or select one in Project View"
		m.status = m.run.err
		m.invalidate()
		return
	}
	if !inProject(root, path) {
		m.run.err = "Go Generate only runs files in the open project"
		m.status = m.run.err
		m.invalidate()
		return
	}
	cfg, err := goGenerateConfig(path, scope, "")
	if err != nil {
		m.run.err = err.Error()
		m.status = err.Error()
		m.invalidate()
		return
	}
	if !m.startDynamicRunConfig(cfg, cfg.Name, true) && !m.trustAsk {
		m.notifyError("go-generate", "Go Generate was not started", m.run.err)
	}
}

// ComposerInstall queries OpenIDEA for the owning Composer project, applies the
// configured PHP to the run command, then starts `composer install` through the
// backend. The SDK lookup is advisory, as upstream: a lookup failure falls back
// to the project root.
func (m *Model) ComposerInstall(arg any) {
	root := m.Root()
	if root == "" {
		return
	}
	path := runActionPath(m, arg)
	if path == "" || !inProject(root, path) {
		path = root
	}
	baseCtx := m.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	m.run.err = ""
	m.status = "Finding Composer project…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(baseCtx, 60*time.Second)
		defer cancel()
		installRoot := root
		if raw, err := m.invoke(ctx, "sdk_php_composer_install", map[string]any{
			"path": path,
		}); err == nil && len(raw) > 0 && string(raw) != "null" {
			var state phpComposerInstallState
			if json.Unmarshal(raw, &state) == nil && state.Root != "" &&
				(filepath.IsAbs(state.Root) && inProject(root, state.Root)) {
				installRoot = filepath.Clean(state.Root)
			}
		}
		cfg, err := composerInstallConfig(installRoot)
		if err == nil {
			_, err = json.Marshal(cfg)
			if err == nil {
				var applied []openidea.RunConfig
				var appliedRaw json.RawMessage
				appliedRaw, err = m.invoke(ctx, "run_apply_settings", map[string]any{
					"configs": []*openidea.RunConfig{cfg},
				})
				if err == nil {
					applied, err = openidea.DecodeRunConfigs(appliedRaw)
				}
				if err == nil && len(applied) > 0 {
					cfg = &applied[0]
				}
			}
		}
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				switch {
				case errors.Is(ctx.Err(), context.Canceled):
					m.run.err = "Composer install was cancelled before it started"
				default:
					m.run.err = "Cannot prepare Composer install: " + err.Error()
				}
				m.status = m.run.err
				m.notifyError("composer-install", "Composer Install was not started", m.run.err)
				m.invalidate()
				return
			}
			if !m.startDynamicRunConfig(cfg, cfg.Name, true) && !m.trustAsk {
				m.notifyError("composer-install", "Composer Install was not started", m.run.err)
			}
		})
	}()
}

func composerInstallConfig(root string) (*openidea.RunConfig, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("The project root is missing")
	}
	root = filepath.Clean(root)
	return &openidea.RunConfig{
		ID:       "composer:install:" + root,
		Name:     "composer install",
		Kind:     "composer",
		Command:  "composer",
		Args:     []string{"install"},
		Cwd:      &root,
		Env:      map[string]string{},
		Detected: true,
	}, nil
}

func (m *Model) startDynamicRunConfig(cfg *openidea.RunConfig, name string, show bool) bool {
	if cfg == nil {
		return false
	}
	if m.run.busy || m.run.running || m.run.awaiting {
		m.run.visible = true
		m.occupyBottom("run")
		m.run.err = "Another process is already running"
		m.status = m.run.err
		m.invalidate()
		return false
	}
	if !m.run.trusted {
		m.run.visible = true
		m.occupyBottom("run")
		m.run.pendingInline = cloneRunConfig(cfg)
		m.run.pendingTrust = true
		m.trustAsk = true
		m.trustFolder = false
		m.status = "Trust this project to run " + name
		m.invalidate()
		return false
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		m.run.err = "Cannot prepare " + name + ": " + err.Error()
		m.status = m.run.err
		m.invalidate()
		return false
	}
	wasVisible := m.run.visible
	previousTool := m.visibleTool()
	if !m.startRunWith(raw, name) {
		if m.run.err == "" {
			m.run.err = "Cannot start " + name
		}
		m.status = m.run.err
		m.invalidate()
		return false
	}
	if !show && !wasVisible {
		m.run.visible = false
		if previousTool != "" {
			m.activateTool(previousTool)
		}
	}
	return true
}

// ModifyRunContext derives the active file's would-be temporary configuration
// and opens it in Run/Debug Configurations, matching run.modifyContext.
func (m *Model) ModifyRunContext(arg any) {
	if m.Root() == "" {
		return
	}
	path := runActionPath(m, arg)
	if path == "" {
		m.run.err = "Open a source file first"
		m.status = m.run.err
		m.invalidate()
		return
	}
	if !inProject(m.Root(), path) {
		m.run.err = "Run configuration editing only works for files in the open project"
		m.status = m.run.err
		m.invalidate()
		return
	}
	if content, ok := runContextArgContent(m, path, arg); ok {
		m.finishModifyRunContext(runContextModifyRequest{Path: path, Content: content})
		return
	}
	baseCtx := m.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(baseCtx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "fs_read_text", map[string]any{"path": path})
		var content string
		if err == nil {
			err = json.Unmarshal(raw, &content)
		}
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				m.run.err = "Cannot read the source file: " + err.Error()
				m.status = m.run.err
				m.notifyError("run-modify-context", "Cannot modify this run configuration", m.run.err)
				m.invalidate()
				return
			}
			m.finishModifyRunContext(runContextModifyRequest{Path: path, Content: content})
		})
	}()
}

func runContextArgContent(m *Model, path string, arg any) (string, bool) {
	if params, ok := arg.(map[string]any); ok {
		if content, ok := params["content"].(string); ok {
			return content, true
		}
	}
	if tab := m.tab(); tab != nil && tab.doc != nil &&
		realPath(tab.doc.Path()) == realPath(path) {
		return tab.doc.Text(), true
	}
	if data, err := os.ReadFile(path); err == nil {
		return limitRunContextContent(string(data)), true
	}
	return "", false
}

func limitRunContextContent(text string) string {
	runes := []rune(text)
	if len(runes) <= runContextContentLimit {
		return text
	}
	return string(runes[:runContextContentLimit])
}

func (m *Model) finishModifyRunContext(req runContextModifyRequest) {
	if !m.run.loaded {
		m.run.pendingModifyContext = &req
		m.loadRunConfigs()
		m.invalidate()
		return
	}
	cfg, err := contextRunConfigFor(m.Root(), req.Path, req.Content, m.run.configs)
	if err != nil {
		m.run.err = err.Error()
		m.status = err.Error()
		m.notifyInfo("run-modify-context", "Nothing to edit here", err.Error())
		m.invalidate()
		return
	}
	cfg = m.adoptRunTemporary(*cfg)
	if cfg == nil {
		return
	}
	m.run.err = ""
	m.OpenRunConfigs()
	m.status = "Editing the run configuration for " + filepath.Base(req.Path)
	m.invalidate()
}

// StreamSampleOutput ports the hidden run.sampleOutput performance action.
// It fills the native Run console in bounded chunks and uses the same Stop
// command for cancellation as a real process.
func (m *Model) StreamSampleOutput(arg any) {
	lines, perFrame := 20_000, 200
	if params, ok := arg.(map[string]any); ok {
		lines = intArg(params["lines"], lines)
		perFrame = intArg(params["perFrame"], perFrame)
	}
	if lines < 1 {
		lines = 1
	}
	if perFrame < 1 {
		perFrame = 1
	}
	if m.run.busy || m.run.running || m.run.awaiting || m.run.sampleOutputBusy {
		m.run.err = "Stop the current process before streaming sample output"
		m.status = m.run.err
		m.invalidate()
		return
	}
	baseCtx := m.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(baseCtx)
	m.run.sampleOutputGeneration++
	generation := m.run.sampleOutputGeneration
	m.run.sampleOutputCancel = cancel
	m.run.sampleOutputBusy = true
	m.run.output = nil
	m.run.truncated = false
	m.run.err = ""
	m.run.exitCode = nil
	m.run.visible = true
	m.occupyBottom("run")
	m.status = "Running: Sample output"
	m.invalidate()

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		out := 0
		for out < lines {
			select {
			case <-ctx.Done():
				m.post(func() {
					if m.run.sampleOutputGeneration != generation {
						return
					}
					m.run.sampleOutputBusy = false
					m.run.sampleOutputCancel = nil
					m.status = "Sample output stopped"
					m.invalidate()
				})
				return
			case <-ticker.C:
				end := min(lines, out+perFrame)
				var text strings.Builder
				for i := out; i < end; i++ {
					text.WriteString(runSampleLine(i))
				}
				out = end
				chunk := openidea.OutputChunk{Kind: "stdout", Text: text.String()}
				m.post(func() {
					if m.run.sampleOutputGeneration != generation {
						return
					}
					m.run.append(chunk)
					m.invalidate()
				})
			}
		}
		m.post(func() {
			if m.run.sampleOutputGeneration != generation {
				return
			}
			m.run.sampleOutputBusy = false
			m.run.sampleOutputCancel = nil
			m.status = fmt.Sprintf("Sample output finished (%d lines)", lines)
			m.invalidate()
		})
	}()
}

func intArg(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
	}
	return fallback
}

func runSampleLine(index int) string {
	switch {
	case index%20 == 19:
		return fmt.Sprintf("\tat com.acme.orders.service.OrderService.process(OrderService.kt:%d)\n",
			index%300+1)
	case index%7 == 6:
		return fmt.Sprintf(
			"2026-11-05 10:42:01.%03d DEBUG 4242 --- [           main] "+
				"o.s.web.servlet.DispatcherServlet        : GET \"/api/orders/%d\", parameters={}\n",
			index%1000, index,
		)
	default:
		return fmt.Sprintf(
			"2026-11-05 10:42:%02d.%03d  INFO 4242 --- [nio-8080-exec-%d] "+
				"c.a.orders.service.OrderService  : processed order #%d in %d ms\n",
			index%60, index%1000, index%10, index, index%97,
		)
	}
}
