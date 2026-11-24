package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"mayoi/internal/openidea"
)

func init() {
	hiddenActionIDs["run.withExecutor"] = true
	_ = registerAction(&Action{
		ID:       "run.withExecutor",
		Title:    "Run with Executor",
		Category: "Run",
		When:     func(m *Model) bool { return m.Root() != "" },
		RunWithArg: func(m *Model, arg any) {
			m.RunWithExecutor(arg)
		},
	})
}

type runExecutorRequest struct {
	Executor   string
	Config     any
	Context    any
	HasConfig  bool
	HasContext bool
}

// RunWithExecutor dispatches OpenIDEA's run executors. The four ids match the
// executors GoIDEA ships: coverage, CPU profiling, allocation profiling and
// live-object profiling.
func (m *Model) RunWithExecutor(arg any) bool {
	request, ok := decodeRunExecutorRequest(arg)
	if !ok || m.Root() == "" {
		return false
	}
	if request.HasContext {
		cfg, err := m.configFromExecutorContext(request.Context)
		if err != nil {
			m.status = err.Error()
			m.invalidate()
			return false
		}
		return m.runConfigWithExecutor(cfg, request.Executor)
	}
	if !request.HasConfig && m.run.currentFile {
		cfg, err := m.configFromExecutorContext(true)
		if err != nil {
			m.status = err.Error()
			m.invalidate()
			return false
		}
		return m.runConfigWithExecutor(cfg, request.Executor)
	}

	var cfg *openidea.RunConfig
	if request.HasConfig {
		decoded, _, ok := runConfigFromActionArg(request.Config)
		if !ok {
			return false
		}
		cfg = &decoded
	} else {
		cfg = m.run.current()
	}
	if cfg == nil {
		m.status = "Choose a run configuration"
		m.DockShow("run")
		if !m.run.loaded {
			m.loadRunConfigs()
		}
		m.invalidate()
		return false
	}
	return m.runConfigWithExecutor(cfg, request.Executor)
}

func decodeRunExecutorRequest(arg any) (runExecutorRequest, bool) {
	var request runExecutorRequest
	switch value := arg.(type) {
	case map[string]any:
		request.Executor, _ = value["executor"].(string)
		request.Config, request.HasConfig = value["config"]
		request.Context, request.HasContext = value["context"]
	case json.RawMessage:
		if !decodeRunExecutorRequestJSON(value, &request) {
			return request, false
		}
	case []byte:
		if !decodeRunExecutorRequestJSON(value, &request) {
			return request, false
		}
	default:
		raw, err := json.Marshal(arg)
		if err != nil || !decodeRunExecutorRequestJSON(raw, &request) {
			return request, false
		}
	}
	request.Executor = strings.TrimSpace(request.Executor)
	return request, request.Executor != ""
}

func decodeRunExecutorRequestJSON(raw []byte, request *runExecutorRequest) bool {
	if json.Unmarshal(raw, request) != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	_, request.HasConfig = fields["config"]
	context, hasContext := fields["context"]
	request.HasContext = hasContext
	if hasContext && string(context) != "null" {
		request.Context = context
	}
	return true
}

func (m *Model) configFromExecutorContext(arg any) (*openidea.RunConfig, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("No project is open")
	}
	if arg == true || arg == nil {
		if tab := m.tab(); tab != nil && tab.doc != nil {
			cfg, err := contextRunConfigFor(root, tab.doc.Path(), tab.doc.Text(), m.run.configs)
			if err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, errors.New("Open a source file first")
	}
	object, ok := arg.(map[string]any)
	if !ok {
		raw, err := json.Marshal(arg)
		if err != nil || json.Unmarshal(raw, &object) != nil {
			return nil, errors.New("The executor context is invalid")
		}
	}
	path, _ := object["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("The executor context has no file")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	content, _ := object["content"].(string)
	if content == "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		content = string(data)
	}
	return contextRunConfigFor(root, filepath.Clean(path), content, m.run.configs)
}

func (m *Model) runConfigWithExecutor(
	cfg *openidea.RunConfig,
	executor string,
) bool {
	if cfg == nil {
		return false
	}
	if !m.run.trusted {
		m.status = "The project is not trusted: accept the prompt before running"
		m.run.pendingTrust = true
		m.trustAsk = true
		m.trustFolder = false
		m.invalidate()
		return false
	}
	switch executor {
	case "coverage":
		raw := append(json.RawMessage(nil), cfg.Raw()...)
		if len(raw) == 0 {
			var err error
			raw, err = json.Marshal(cfg)
			if err != nil {
				return false
			}
		}
		return m.startRunWithCoverage(raw, cfg.Name, &coverageExecution{
			name: cfg.Name,
			root: m.Root(),
		})
	case "profiler", "profilerAllocations", "profilerLiveObjects":
		mode := openidea.ProfileCPU
		switch executor {
		case "profilerAllocations":
			mode = openidea.ProfileAllocations
		case "profilerLiveObjects":
			mode = openidea.ProfileLiveObjects
		}
		m.prepareProfilerConfig(cfg, mode)
		return true
	default:
		return false
	}
}
