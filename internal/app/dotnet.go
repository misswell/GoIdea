package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mayoi/internal/openidea"
)

const dotnetOutputLimit = 8_000_000

var dotnetTaskSequence atomic.Uint32

type dotnetPane struct {
	root          string
	generation    uint64
	model         dotnetWorkspace
	selected      string
	configuration string
	framework     string
	busy          bool
	error         string
	output        string
	query         string
	packages      []dotnetPackage
	found         []dotnetPackage
	tests         []string
	expanded      map[string]bool
	versions      map[string]string
	task          *dotnetTask
}

type dotnetTask struct {
	generation uint64
	root       string
	cancel     context.CancelFunc
}

type dotnetApply func(*Model)

func init() {
	for _, id := range []string{
		"dotnet.select",
		"dotnet.search",
		"dotnet.packageChange",
	} {
		hiddenActionIDs[id] = true
	}
	projectOpen := func(m *Model) bool { return m.Root() != "" }
	dotnetActions := []*Action{
		{
			ID: "dotnet.show", Title: ".NET Workspace", Category: "Build",
			Builtin: true, When: projectOpen,
			Run: func(m *Model) {
				m.DockShow("dotnet")
				m.ReloadDotnet()
			},
		},
		{
			ID: "dotnet.reload", Title: "Reload .NET Workspace", Category: "Build",
			Builtin: true, When: projectOpen,
			Run: func(m *Model) { m.ReloadDotnet() },
		},
		{
			ID: "dotnet.cancel", Title: "Cancel .NET Operation", Category: "Build",
			Builtin: true,
			Run:     func(m *Model) { m.CancelDotnet() },
		},
		{
			ID: "dotnet.select", Title: "Select .NET Project", Category: "Build",
			Builtin: true,
			RunWithArg: func(m *Model, arg any) {
				request, ok := dotnetSelectArg(arg)
				if ok {
					m.SelectDotnet(request.Path, request.Configuration, request.Framework)
				}
			},
		},
		{
			ID: "dotnet.build", Title: "Build .NET Project", Category: "Build",
			Builtin: true, When: projectOpen,
			Run: func(m *Model) { m.BuildDotnet("build") },
		},
		{
			ID: "dotnet.restore", Title: "Restore .NET Project", Category: "Build",
			Builtin: true, When: projectOpen,
			Run: func(m *Model) { m.BuildDotnet("restore") },
		},
		{
			ID: "dotnet.clean", Title: "Clean .NET Project", Category: "Build",
			Builtin: true, When: projectOpen,
			Run: func(m *Model) { m.BuildDotnet("clean") },
		},
		{
			ID: "dotnet.packages", Title: "Refresh NuGet Packages", Category: "Build",
			Builtin: true, When: projectOpen,
			Run: func(m *Model) { m.RefreshDotnetPackages() },
		},
		{
			ID: "dotnet.search", Title: "Search NuGet Packages", Category: "Build",
			Builtin: true,
			RunWithArg: func(m *Model, arg any) {
				request, ok := dotnetSearchArg(arg)
				if ok {
					m.SearchDotnetPackages(request.Query, request.Exact)
				}
			},
		},
		{
			ID: "dotnet.packageChange", Title: "Change NuGet Package", Category: "Build",
			Builtin: true,
			RunWithArg: func(m *Model, arg any) {
				request, ok := dotnetPackageChangeArg(arg)
				if ok {
					m.ChangeDotnetPackage(request.ID, request.Version)
				}
			},
		},
		{
			ID: "dotnet.discover", Title: "Discover .NET Tests", Category: "Run",
			Builtin: true, When: projectOpen,
			Run: func(m *Model) { m.DiscoverDotnetTests() },
		},
		{
			ID: "dotnet.test", Title: "Run .NET Tests", Category: "Run",
			Builtin: true, When: projectOpen,
			RunWithArg: func(m *Model, arg any) { m.RunDotnetTests(arg) },
		},
		{
			ID: "dotnet.installTrx", Title: "Install TRX reporting", Category: "Build",
			Builtin: true, When: projectOpen,
			Run: func(m *Model) { m.InstallDotnetTRX() },
		},
	}
	for _, action := range dotnetActions {
		_ = registerAction(action)
	}
}

func (m *Model) resetDotnet() {
	generation := m.dotnet.generation + 1
	if m.dotnet.task != nil && m.dotnet.task.cancel != nil {
		m.dotnet.task.cancel()
	}
	m.dotnet = dotnetPane{
		root:          "",
		generation:    generation,
		configuration: "Debug",
		expanded:      map[string]bool{},
		versions:      map[string]string{},
	}
}

func (m *Model) ensureDotnet() {
	if m.dotnet.root == m.Root() &&
		m.dotnet.expanded != nil &&
		m.dotnet.versions != nil {
		if m.dotnet.configuration == "" {
			m.dotnet.configuration = "Debug"
		}
		return
	}
	if m.dotnet.task != nil && m.dotnet.task.cancel != nil {
		m.dotnet.task.cancel()
	}
	generation := m.dotnet.generation + 1
	m.dotnet = dotnetPane{
		root:          m.Root(),
		generation:    generation,
		configuration: "Debug",
		expanded:      map[string]bool{},
		versions:      map[string]string{},
	}
}

func (m *Model) dotnetSelectedProject() *dotnetProject {
	index := dotnetProjectIndex(m.dotnet.model.Projects, m.dotnet.selected)
	if index < 0 {
		return nil
	}
	return &m.dotnet.model.Projects[index]
}

func (m *Model) dotnetTaskCurrent(task *dotnetTask) bool {
	return task != nil &&
		m.dotnet.task == task &&
		m.dotnet.generation == task.generation &&
		m.Root() == task.root
}

func (m *Model) startDotnetTask(
	work func(context.Context) (dotnetApply, error),
) bool {
	m.ensureDotnet()
	root := m.Root()
	if root == "" {
		return false
	}
	if m.dotnet.task != nil && m.dotnet.task.cancel != nil {
		m.dotnet.task.cancel()
	}
	generation := m.dotnet.generation + 1
	ctx, cancel := context.WithCancel(modelContext(m))
	task := &dotnetTask{
		generation: generation,
		root:       root,
		cancel:     cancel,
	}
	m.dotnet.generation = generation
	m.dotnet.task = task
	m.dotnet.busy = true
	m.dotnet.error = ""
	m.invalidate()
	go func() {
		defer cancel()
		apply, err := work(ctx)
		m.post(func() {
			if !m.dotnetTaskCurrent(task) {
				return
			}
			if err == nil && apply != nil {
				apply(m)
			}
			m.dotnet.busy = false
			m.dotnet.task = nil
			if err != nil &&
				!errors.Is(err, context.Canceled) &&
				!errors.Is(err, context.DeadlineExceeded) {
				m.dotnet.error = err.Error()
				m.status = ".NET: " + err.Error()
			}
			m.invalidate()
		})
	}()
	return true
}

func (m *Model) CancelDotnet() {
	if m.dotnet.task == nil || m.dotnet.task.cancel == nil {
		return
	}
	m.dotnet.task.cancel()
	m.status = "Cancelling .NET operation…"
	m.invalidate()
}

func (m *Model) ReloadDotnet() {
	m.ensureDotnet()
	root := m.Root()
	if root == "" {
		return
	}
	previous := m.dotnet.selected
	m.startDotnetTask(func(ctx context.Context) (dotnetApply, error) {
		workspace, err := dotnetDiscoverWorkspace(ctx, m, root)
		if err != nil {
			return nil, err
		}
		return func(m *Model) {
			selected := ""
			for _, project := range workspace.Projects {
				if filepath.Clean(project.Path) == filepath.Clean(previous) {
					selected = project.Path
					break
				}
			}
			if selected == "" && len(workspace.Projects) > 0 {
				selected = workspace.Projects[0].Path
			}
			framework := ""
			if index := dotnetProjectIndex(workspace.Projects, selected); index >= 0 &&
				len(workspace.Projects[index].Frameworks) > 0 {
				framework = workspace.Projects[index].Frameworks[0]
			}
			m.dotnet.model = workspace
			m.dotnet.selected = selected
			m.dotnet.configuration = "Debug"
			m.dotnet.framework = framework
			m.dotnet.packages = nil
			m.dotnet.found = nil
			m.dotnet.tests = nil
			m.dotnet.expanded = map[string]bool{}
		}, nil
	})
}

type dotnetSelectRequest struct {
	Path          string `json:"path"`
	Configuration string `json:"configuration"`
	Framework     string `json:"framework"`
}

func dotnetSelectArg(arg any) (dotnetSelectRequest, bool) {
	var request dotnetSelectRequest
	data, err := json.Marshal(arg)
	if err != nil || json.Unmarshal(data, &request) != nil {
		return request, false
	}
	request.Path = strings.TrimSpace(request.Path)
	return request, request.Path != ""
}

func (m *Model) SelectDotnet(path, configuration, framework string) {
	m.ensureDotnet()
	path = strings.TrimSpace(path)
	if path == "" || m.Root() == "" {
		return
	}
	if !dotnetIsAbsolute(path) {
		path = filepath.Join(m.Root(), path)
	}
	path = filepath.Clean(path)
	if configuration == "" {
		configuration = m.dotnet.configuration
	}
	if configuration == "" {
		configuration = "Debug"
	}
	selectedPath := path
	if !m.startDotnetTask(func(ctx context.Context) (dotnetApply, error) {
		project, err := dotnetEvaluateProject(ctx, m, path, configuration, framework)
		if err != nil {
			return nil, err
		}
		return func(m *Model) {
			projects := append([]dotnetProject(nil), m.dotnet.model.Projects...)
			index := dotnetProjectIndex(projects, selectedPath)
			if index >= 0 {
				projects[index] = project
			} else {
				projects = append(projects, project)
			}
			errorsFound := make([]string, 0, len(m.dotnet.model.Errors))
			for _, message := range m.dotnet.model.Errors {
				if !strings.HasPrefix(message, selectedPath+":") {
					errorsFound = append(errorsFound, message)
				}
			}
			m.dotnet.selected = selectedPath
			m.dotnet.configuration = configuration
			if framework == "" && len(project.Frameworks) > 0 {
				m.dotnet.framework = project.Frameworks[0]
			} else {
				m.dotnet.framework = framework
			}
			m.dotnet.model.Projects = projects
			m.dotnet.model.Errors = errorsFound
			m.dotnet.packages = nil
			m.dotnet.found = nil
			m.dotnet.tests = nil
		}, nil
	}) {
		return
	}
}

func (m *Model) BuildDotnet(operation string) {
	m.ensureDotnet()
	project := m.dotnetSelectedProject()
	if project == nil ||
		(operation != "build" && operation != "restore" && operation != "clean") {
		return
	}
	path := project.Path
	configuration := m.dotnet.configuration
	if configuration == "" {
		configuration = "Debug"
	}
	framework := m.dotnet.framework
	args := []string{operation, path}
	if operation == "restore" {
		args = append(args, "-p:Configuration="+configuration)
	} else {
		args = append(args, "--configuration", configuration)
		if framework != "" {
			args = append(args, "--framework", framework)
		}
	}
	m.startDotnetTask(func(ctx context.Context) (dotnetApply, error) {
		output, err := m.runDotnetCommand(ctx, args, filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		evaluated, err := dotnetEvaluateProject(ctx, m, path, configuration, framework)
		if err != nil {
			return nil, err
		}
		return func(m *Model) {
			m.dotnet.output = output
			if index := dotnetProjectIndex(m.dotnet.model.Projects, path); index >= 0 {
				m.dotnet.model.Projects[index] = evaluated
			}
		}, nil
	})
}

func (m *Model) RefreshDotnetPackages() {
	m.ensureDotnet()
	project := m.dotnetSelectedProject()
	if project == nil {
		return
	}
	path := project.Path
	m.startDotnetTask(func(ctx context.Context) (dotnetApply, error) {
		output, err := m.runDotnetCommand(ctx, []string{
			"list", path, "package", "--include-transitive", "--format", "json",
		}, filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		packages, err := dotnetPackagesFromOutput(output)
		if err != nil {
			return nil, err
		}
		return func(m *Model) {
			m.dotnet.packages = packages
			m.dotnet.versions = map[string]string{}
		}, nil
	})
}

type dotnetSearchRequest struct {
	Query string `json:"query"`
	Exact bool   `json:"exact"`
}

func dotnetSearchArg(arg any) (dotnetSearchRequest, bool) {
	var request dotnetSearchRequest
	data, err := json.Marshal(arg)
	if err != nil || json.Unmarshal(data, &request) != nil {
		return request, false
	}
	request.Query = strings.TrimSpace(request.Query)
	return request, request.Query != ""
}

func (m *Model) SearchDotnetPackages(query string, exact bool) {
	m.ensureDotnet()
	project := m.dotnetSelectedProject()
	query = strings.TrimSpace(query)
	if project == nil || query == "" {
		return
	}
	path := project.Path
	args := []string{"package", "search", query, "--format", "json", "--take", "30"}
	if exact {
		args = append(args, "--exact-match")
	}
	m.dotnet.query = query
	m.startDotnetTask(func(ctx context.Context) (dotnetApply, error) {
		output, err := m.runDotnetCommand(ctx, args, filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		found, err := dotnetSearchFromOutput(output, exact)
		if err != nil {
			return nil, err
		}
		return func(m *Model) {
			m.dotnet.found = found
			m.dotnet.versions = map[string]string{}
		}, nil
	})
}

type dotnetPackageChangeRequest struct {
	ID      string  `json:"id"`
	Version *string `json:"version"`
}

func dotnetPackageChangeArg(arg any) (dotnetPackageChangeRequest, bool) {
	var request dotnetPackageChangeRequest
	data, err := json.Marshal(arg)
	if err != nil || json.Unmarshal(data, &request) != nil {
		return request, false
	}
	request.ID = strings.TrimSpace(request.ID)
	return request, request.ID != ""
}

func (m *Model) ChangeDotnetPackage(id string, version *string) {
	m.ensureDotnet()
	project := m.dotnetSelectedProject()
	id = strings.TrimSpace(id)
	if project == nil || id == "" {
		return
	}
	path := project.Path
	configuration := m.dotnet.configuration
	if configuration == "" {
		configuration = "Debug"
	}
	framework := m.dotnet.framework
	m.startDotnetTask(func(ctx context.Context) (dotnetApply, error) {
		var output string
		changeVersion := version
		if strings.EqualFold(id, "Microsoft.Testing.Extensions.TrxReport") &&
			changeVersion != nil && *changeVersion == "" {
			if _, err := m.runDotnetCommand(ctx,
				[]string{"restore", path}, filepath.Dir(path)); err != nil {
				return nil, err
			}
			restored, err := dotnetEvaluateProject(ctx, m, path, configuration, framework)
			if err != nil {
				return nil, err
			}
			if restored.PlatformVersion == "" {
				return nil, errors.New(
					"Microsoft.Testing.Platform was not found in the restored project",
				)
			}
			value := restored.PlatformVersion
			changeVersion = &value
		}
		args := []string{"add", path, "package", id}
		if changeVersion == nil {
			args = []string{"remove", path, "package", id}
		} else if *changeVersion != "" {
			args = append(args, "--version", *changeVersion)
		}
		var err error
		output, err = m.runDotnetCommand(ctx, args, filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		if changeVersion == nil {
			if _, err := m.runDotnetCommand(ctx,
				[]string{"restore", path}, filepath.Dir(path)); err != nil {
				return nil, err
			}
		}
		evaluated, err := dotnetEvaluateProject(ctx, m, path, configuration, framework)
		if err != nil {
			return nil, err
		}
		listOutput, err := m.runDotnetCommand(ctx, []string{
			"list", path, "package", "--include-transitive", "--format", "json",
		}, filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		packages, err := dotnetPackagesFromOutput(listOutput)
		if err != nil {
			return nil, err
		}
		return func(m *Model) {
			m.dotnet.output = output
			m.dotnet.packages = packages
			m.dotnet.versions = map[string]string{}
			if index := dotnetProjectIndex(m.dotnet.model.Projects, path); index >= 0 {
				m.dotnet.model.Projects[index] = evaluated
			}
		}, nil
	})
}

func (m *Model) DiscoverDotnetTests() {
	m.ensureDotnet()
	project := m.dotnetSelectedProject()
	if project == nil {
		return
	}
	snapshot := *project
	configuration := m.dotnet.configuration
	if configuration == "" {
		configuration = "Debug"
	}
	framework := m.dotnet.framework
	m.startDotnetTask(func(ctx context.Context) (dotnetApply, error) {
		output, err := m.runDotnetCommand(
			ctx,
			dotnetTestArgs(snapshot, framework, []string{"--list-tests"}, configuration),
			filepath.Dir(snapshot.Path),
		)
		if err != nil {
			return nil, err
		}
		tests := dotnetDiscoveredTests(output)
		return func(m *Model) {
			m.dotnet.output = output
			m.dotnet.tests = tests
		}, nil
	})
}

func (m *Model) RunDotnetTests(arg any) {
	m.ensureDotnet()
	project := m.dotnetSelectedProject()
	if project == nil {
		return
	}
	test, _ := arg.(string)
	configuration := m.dotnet.configuration
	if configuration == "" {
		configuration = "Debug"
	}
	params := map[string]any{
		"project":       project.Path,
		"framework":     m.dotnet.framework,
		"configuration": configuration,
		"runner":        project.Runner,
		"adapter":       project.Adapter,
		"programArgs":   "",
	}
	name := project.Name
	if test != "" {
		name = test
	}
	if method := dotnetTestMethod(test); method != "" {
		encoded, _ := json.Marshal([]string{method})
		params["testMethods"] = string(encoded)
	}
	raw, err := json.Marshal(dotnetTestRunConfigObject(*project, name, params))
	if err != nil {
		return
	}
	configurationValue, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		m.status = ".NET test configuration: " + err.Error()
		m.invalidate()
		return
	}
	m.RunActionWithArg("run.runConfig", configurationValue)
}

func dotnetTestRunConfigObject(
	project dotnetProject,
	name string,
	params map[string]any,
) map[string]any {
	object := map[string]any{
		"id":           "dotnet:" + project.Path + ":tests",
		"name":         name,
		"kind":         "dotnet-test",
		"command":      "dotnet",
		"args":         []string{},
		"cwd":          filepath.Dir(project.Path),
		"env":          map[string]string{},
		"detected":     true,
		"isTest":       true,
		"debugAdapter": nil,
		"params":       params,
	}
	return object
}

func (m *Model) InstallDotnetTRX() {
	value := ""
	m.ChangeDotnetPackage("Microsoft.Testing.Extensions.TrxReport", &value)
}

type dotnetRunSession struct {
	channel  string
	runID    uint32
	early    map[uint32]*int32
	output   strings.Builder
	mu       sync.Mutex
	exited   chan struct{}
	changed  chan struct{}
	exit     *int32
	finished bool
	limited  bool
	readErr  error
}

func newDotnetRunSession(channel uint32) *dotnetRunSession {
	return &dotnetRunSession{
		channel: fmt.Sprintf("%d", channel),
		early:   map[uint32]*int32{},
		exited:  make(chan struct{}),
		changed: make(chan struct{}, 1),
	}
}

func (s *dotnetRunSession) notifyChange() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *dotnetRunSession) accept(notification openidea.Notification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch notification.Method {
	case "channel":
		var message openidea.ChannelMessage
		if json.Unmarshal(notification.Params, &message) != nil ||
			fmt.Sprintf("%d", message.ID) != s.channel {
			return
		}
		var chunk openidea.OutputChunk
		if json.Unmarshal(message.Data, &chunk) != nil {
			return
		}
		if s.output.Len()+len(chunk.Text) > dotnetOutputLimit {
			s.limited = true
			s.notifyChange()
			return
		}
		s.output.WriteString(chunk.Text)
		s.notifyChange()
	case "event":
		var envelope openidea.WindowEvent
		if json.Unmarshal(notification.Params, &envelope) != nil {
			return
		}
		if envelope.Window != nil && *envelope.Window != Window {
			return
		}
		if openidea.EventType(envelope.Event) != "runExited" {
			return
		}
		var event openidea.RunExitedEvent
		if json.Unmarshal(envelope.Event, &event) != nil {
			return
		}
		if s.runID == 0 {
			s.early[event.Run] = event.Code
			return
		}
		if event.Run != s.runID || s.finished {
			return
		}
		s.exit = event.Code
		s.finished = true
		close(s.exited)
		s.notifyChange()
	}
}

func (s *dotnetRunSession) setRunID(id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runID = id
	if code, ok := s.early[id]; ok && s.exit == nil {
		s.exit = code
		s.finished = true
		close(s.exited)
		s.notifyChange()
	}
}

func (s *dotnetRunSession) snapshot() (string, *int32, bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output.String(), s.exit, s.finished, s.limited, s.readErr
}

func (s *dotnetRunSession) setReadError(err error) {
	s.mu.Lock()
	if s.readErr == nil {
		s.readErr = err
	}
	s.mu.Unlock()
	s.notifyChange()
}

func (m *Model) runDotnetCommand(
	ctx context.Context,
	args []string,
	cwd string,
) (string, error) {
	if len(args) == 0 {
		return "", errors.New(".NET command is empty")
	}
	if m.mgr == nil {
		return "", errors.New("The GoIDEA backend is required for .NET operations")
	}
	channel := m.nextChannel()
	session := newDotnetRunSession(channel)
	events, unlisten := m.mgr.Listen(8192)
	defer unlisten()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case event, ok := <-events:
				if !ok {
					session.setReadError(errors.New("The backend event stream closed"))
					return
				}
				session.accept(event)
			}
		}
	}()

	stop, err := m.startDotnetRun(ctx, session, channel, args, cwd)
	if err != nil {
		return "", err
	}
	for {
		text, exit, finished, limited, readErr := session.snapshot()
		if readErr != nil {
			return text, readErr
		}
		if limited {
			stop()
			select {
			case <-session.exited:
			case <-time.After(15 * time.Second):
			}
			return text, errors.New(".NET output exceeded the size limit")
		}
		if finished {
			if exit == nil {
				if message := strings.TrimSpace(text); message != "" {
					return text, errors.New(message)
				}
				return text, errors.New("dotnet exited without an exit code")
			}
			if *exit != 0 {
				if message := strings.TrimSpace(text); message != "" {
					return text, errors.New(message)
				}
				return text, fmt.Errorf("dotnet exited with code %d", *exit)
			}
			return text, nil
		}
		select {
		case <-ctx.Done():
			stop()
			select {
			case <-session.exited:
			case <-time.After(15 * time.Second):
			}
			return text, ctx.Err()
		case <-session.changed:
		}
	}
}

func (m *Model) startDotnetRun(
	ctx context.Context,
	session *dotnetRunSession,
	channel uint32,
	args []string,
	cwd string,
) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rawInfo, err := m.invoke(modelContext(m), "run_start", map[string]any{
		"config":   dotnetRunConfigObject(args, cwd),
		"onOutput": map[string]any{"$channel": channel},
		"request":  nil,
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	var info openidea.RunInfo
	if err := json.Unmarshal(rawInfo, &info); err != nil {
		return nil, err
	}
	if info.ID == 0 {
		return nil, errors.New("The backend did not return a .NET process id")
	}
	session.setRunID(info.ID)

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			stopCtx, cancel := context.WithTimeout(modelContext(m), 15*time.Second)
			defer cancel()
			_, _ = m.invoke(stopCtx, "run_stop", map[string]any{"id": info.ID})
		})
	}
	if ctx.Err() != nil {
		stop()
	}
	return stop, nil
}
