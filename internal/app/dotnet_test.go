package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func TestDotnetActionsAndToolWindowAreRegistered(t *testing.T) {
	m := &Model{ctx: context.Background()}
	root := t.TempDir()
	m.setRoot(root)
	tests := []struct {
		id       string
		title    string
		category string
		hidden   bool
		enabled  bool
	}{
		{"dotnet.show", ".NET Workspace", "Build", false, true},
		{"dotnet.reload", "Reload .NET Workspace", "Build", false, true},
		{"dotnet.cancel", "Cancel .NET Operation", "Build", false, true},
		{"dotnet.select", "Select .NET Project", "Build", true, true},
		{"dotnet.build", "Build .NET Project", "Build", false, true},
		{"dotnet.restore", "Restore .NET Project", "Build", false, true},
		{"dotnet.clean", "Clean .NET Project", "Build", false, true},
		{"dotnet.packages", "Refresh NuGet Packages", "Build", false, true},
		{"dotnet.search", "Search NuGet Packages", "Build", true, true},
		{"dotnet.packageChange", "Change NuGet Package", "Build", true, true},
		{"dotnet.discover", "Discover .NET Tests", "Run", false, true},
		{"dotnet.test", "Run .NET Tests", "Run", false, true},
		{"dotnet.installTrx", "Install TRX reporting", "Build", false, true},
	}
	for _, test := range tests {
		action, ok := actionGet(test.id)
		if !ok {
			t.Fatalf("%s is not registered", test.id)
		}
		if action.Title != test.title || action.Category != test.category ||
			hiddenActionIDs[test.id] != test.hidden {
			t.Fatalf("%s = %#v hidden=%v", test.id, action, hiddenActionIDs[test.id])
		}
		if m.ActionEnabled(test.id) != test.enabled {
			t.Fatalf("%s enabled = %v, want %v", test.id, m.ActionEnabled(test.id), test.enabled)
		}
	}
	spec := dockSpec("dotnet")
	if spec == nil || spec.title != ".NET" || spec.anchor != dockRight || spec.stripe {
		t.Fatalf(".NET tool window = %#v", spec)
	}
}

func TestDotnetSolutionPathsReadsLegacyAndSlnxLayouts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	legacy := "\xef\xbb\xbfMicrosoft Visual Studio Solution File, Format Version 12.00\n" +
		`Project("{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}") = "App", "src\App.csproj", "{11111111-1111-1111-1111-111111111111}"` + "\n" +
		`Project("{F2A71F9B-5D33-465A-A702-920D77279786}") = "Tests", "tests\Tests.fsproj", "{22222222-2222-2222-2222-222222222222}"` + "\n" +
		`Project("{E24C65DC-7377-472B-9ABA-BC803B73C61A}") = "Web", "apps\Web.vbproj", "{33333333-3333-3333-3333-333333333333}"` + "\n" +
		`Project("{2150E333-8FDC-42A3-9474-1A3956D46DE8}") = "Solution", "Solution\", "{44444444-4444-4444-4444-444444444444}"` + "\n"
	got := dotnetSolutionProjectPaths(legacy, false, root)
	want := []string{
		filepath.Join(root, "src", "App.csproj"),
		filepath.Join(root, "tests", "Tests.fsproj"),
		filepath.Join(root, "apps", "Web.vbproj"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy solution paths = %#v, want %#v", got, want)
	}
	slnx := `<Solution><Project Path="src/App.csproj"/><Project Path="tests/Tests.csproj"/>` +
		`<Project Path="README.md"/><Project Path="src/../src/App.csproj"/></Solution>`
	got = dotnetSolutionProjectPaths(slnx, true, root)
	want = []string{
		filepath.Join(root, "src", "App.csproj"),
		filepath.Join(root, "tests", "Tests.csproj"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(".slnx paths = %#v, want %#v", got, want)
	}
}

func TestDotnetGlobalRunnerAcceptsJSONCAndIgnoreSkippedDirectories(t *testing.T) {
	text := `{
	  // The URL text below must survive string handling.
	  "sdk": { "version": "10.0.100" },
	  "description": "https://example.com/*",
	  "test": { "runner": "Microsoft.Testing.Platform", },
	}`
	runner, err := dotnetGlobalRunner(text)
	if err != nil {
		t.Fatal(err)
	}
	if runner != dotnetRunnerMTP {
		t.Fatalf("runner = %q, want mtp", runner)
	}
	for _, name := range []string{".git", ".idea", "bin", "obj", "node_modules", "target", "packages"} {
		if !dotnetSkipDirectory(name, false) {
			t.Fatalf("%s should be skipped", name)
		}
	}
	if dotnetSkipDirectory("src", false) || !dotnetSkipDirectory("src", true) {
		t.Fatal("ordinary and excluded directories were classified incorrectly")
	}
}

func TestDotnetPackagesAndSearchParsersPreservePinnedRules(t *testing.T) {
	output := "MSBuild noise\n" + `{
	  "projects": [{
	    "frameworks": [{
	      "topLevelPackages": [
	        {"id":"Newtonsoft.Json","requestedVersion":"13.0.3","resolvedVersion":"13.0.4","latestVersion":"13.1.0"}
	      ],
	      "transitivePackages": [
	        {"id":"Newtonsoft.Json","resolvedVersion":"13.0.4"},
	        {"id":"System.Memory","resolvedVersion":"4.5.5"}
	      ]
	    }]
	  }]
	}` + "\nDone"
	packages, err := dotnetPackagesFromOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 2 || packages[0].ID != "Newtonsoft.Json" ||
		packages[0].Version != "13.0.4" || packages[0].Transitive ||
		packages[1].ID != "System.Memory" || !packages[1].Transitive {
		t.Fatalf("packages = %#v", packages)
	}

	search := `{"searchResult":[{"packages":[{"id":"A","latestVersion":"2.0.0"}]},` +
		`{"packages":[{"id":"A","version":"1.0.0"},{"id":"B","latestVersion":"3.0.0"}]}]}`
	found, err := dotnetSearchFromOutput(search, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{found[0].ID, found[1].Version, found[2].ID}; !reflect.DeepEqual(got, []string{"A", "1.0.0", "B"}) {
		t.Fatalf("found = %#v", found)
	}
	found, err = dotnetSearchFromOutput(search, true)
	if err != nil {
		t.Fatal(err)
	}
	if found[0].ID != "B" || found[2].ID != "A" {
		t.Fatalf("exact results were not reversed: %#v", found)
	}
	if _, err := dotnetSearchFromOutput(`{"problems":[{"text":"feed failed"}]}`, false); err == nil ||
		!strings.Contains(err.Error(), "feed failed") {
		t.Fatalf("search problems error = %v", err)
	}
}

func TestDotnetPinnedProjectDetectionRules(t *testing.T) {
	for _, id := range []string{
		"Microsoft.NET.Test.Sdk",
		"mstest",
		"NUnit",
		"xunit",
		"xunit.v3",
		"xunit.v3.mtp-v1",
		"xunit.v3.mtp-v2",
	} {
		if !dotnetTestPackageRE.MatchString(id) {
			t.Fatalf("%s should identify a test project", id)
		}
	}
	for _, id := range []string{
		"MSTest.TestAdapter",
		"NUnit3TestAdapter",
		"xunit.runner.visualstudio",
		"xunit.v3.mtp-v3",
		"Microsoft.NET.Test.Sdk.Extension",
	} {
		if dotnetTestPackageRE.MatchString(id) {
			t.Fatalf("%s should not match the pinned exact package list", id)
		}
	}
	for _, path := range []string{
		`C:\repo\src\App.csproj`,
		`C:/repo/src/App.csproj`,
	} {
		if !dotnetIsAbsolute(path) {
			t.Fatalf("%s should be treated as an absolute path", path)
		}
	}
	if dotnetIsAbsolute(`C:repo\App.csproj`) {
		t.Fatal("a drive-relative path was treated as absolute")
	}
}

func TestDotnetTestDiscoveryAndCommandBuilders(t *testing.T) {
	output := "\x1b[33mThe following Tests are available:\x1b[0m\n" +
		"  Namespace.OneTests.Passes\n" +
		"  Namespace.TwoTests.Fails(System.String)\n" +
		"\n" +
		"Test run summary:\nTotal: 2\n"
	if got := dotnetDiscoveredTests(output); !reflect.DeepEqual(got, []string{
		"Namespace.OneTests.Passes",
		"Namespace.TwoTests.Fails(System.String)",
	}) {
		t.Fatalf("discovered tests = %#v", got)
	}
	project := dotnetProject{Path: "/repo/tests/Tests.csproj"}
	project.Runner = dotnetRunnerVSTest
	if got := dotnetTestArgs(project, "net10.0", []string{"--list-tests"}, "Debug"); !reflect.DeepEqual(got, []string{
		"test", project.Path, "--configuration", "Debug", "--framework", "net10.0", "--list-tests",
	}) {
		t.Fatalf("VSTest args = %#v", got)
	}
	project.Runner = dotnetRunnerMTP
	if got := dotnetTestArgs(project, "", nil, "Release"); !reflect.DeepEqual(got, []string{
		"test", "--project", project.Path, "--configuration", "Release",
	}) {
		t.Fatalf("MTP args = %#v", got)
	}
	project.Runner = dotnetRunnerMTPLegacy
	if got := dotnetTestArgs(project, "net9.0", []string{"--list-tests"}, "Debug"); !reflect.DeepEqual(got, []string{
		"test", project.Path, "--configuration", "Debug", "--framework", "net9.0",
		"-p:TestingPlatformCaptureOutput=false", "-p:TestingPlatformDotnetTestSupport=true",
		"--", "--list-tests",
	}) {
		t.Fatalf("MTP-legacy args = %#v", got)
	}
}

func TestDotnetTestCommandFiltersMethodsAndPreservesRunnerArguments(t *testing.T) {
	params := map[string]any{
		"project": "/repo/Tests.csproj", "framework": "net10.0",
		"configuration": "Debug", "runner": "vstest", "adapter": "mstest",
		"programArgs": `--filter "Name=Old" --no-restore`,
		"testMethods": `["Suite.Case","Has(x|y)"]`,
	}
	command, args, err := dotnetTestCommandLine(params)
	if err != nil {
		t.Fatal(err)
	}
	if command != "dotnet" {
		t.Fatalf("command = %q", command)
	}
	want := []string{
		"test", "/repo/Tests.csproj", "--framework", "net10.0", "--configuration", "Debug",
		"--no-restore", "--filter",
		`FullyQualifiedName=Suite.Case|Name=Has\(x\|y\)`,
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	params["runner"] = "mtp-legacy"
	params["adapter"] = "xunit"
	params["programArgs"] = `--verbosity normal custom --filter-method Old`
	params["testMethods"] = `["Case"]`
	_, args, err = dotnetTestCommandLine(params)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{
		"test", "/repo/Tests.csproj", "--framework", "net10.0", "--configuration", "Debug",
		"-p:TestingPlatformCaptureOutput=false", "-p:TestingPlatformDotnetTestSupport=true",
		"--verbosity", "normal", "--",
		"custom", "--filter-method", "*.Case",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("MTP-legacy args = %#v, want %#v", args, want)
	}
	params["programArgs"] = `--no-build -- custom --filter-method Old`
	_, args, err = dotnetTestCommandLine(params)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{
		"test", "/repo/Tests.csproj", "--framework", "net10.0", "--configuration", "Debug",
		"-p:TestingPlatformCaptureOutput=false", "-p:TestingPlatformDotnetTestSupport=true",
		"--no-build", "--", "custom", "--filter-method", "*.Case",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("MTP-legacy explicit boundary args = %#v, want %#v", args, want)
	}
}

func TestDotnetRunConfigAndTestConfigMatchUpstreamShape(t *testing.T) {
	task := dotnetRunConfigObject([]string{"build", "/repo/App.csproj"}, "/repo")
	if task["name"] != "dotnet build" || task["kind"] != "shell" ||
		task["command"] != "dotnet" || task["cwd"] != "/repo" ||
		task["isTest"] != false || task["detected"] != true {
		t.Fatalf("task config = %#v", task)
	}
	env, _ := task["env"].(map[string]string)
	if env["DOTNET_CLI_UI_LANGUAGE"] != "en" {
		t.Fatalf("env = %#v", env)
	}
	project := dotnetProject{
		Path: "/repo/Tests.csproj", Name: "Tests",
		Runner: dotnetRunnerVSTest, Adapter: "mstest",
	}
	params := map[string]any{
		"project": project.Path, "framework": "net10.0",
		"configuration": "Debug", "runner": project.Runner,
		"adapter": project.Adapter, "programArgs": "",
		"testMethods": `["Suite.Case"]`,
	}
	config := dotnetTestRunConfigObject(project, "Suite.Case", params)
	if config["kind"] != "dotnet-test" || config["isTest"] != true ||
		config["debugAdapter"] != nil || config["id"] != "dotnet:/repo/Tests.csproj:tests" {
		t.Fatalf("test config = %#v", config)
	}
	if got := dotnetTestMethod("Suite.Case(System.String)"); got != "Suite.Case" {
		t.Fatalf("test method = %q", got)
	}
}

func TestDotnetRunResolvedMaterializesProfileArguments(t *testing.T) {
	raw := json.RawMessage(`{
	  "id":"run",
	  "name":"App",
	  "kind":"dotnet",
	  "command":"dotnet",
	  "args":[],
	  "params":{
	    "project":"/repo/src/App.csproj",
	    "framework":"net10.0",
	    "configuration":"Debug",
	    "launchProfile":"local web",
	    "programArgs":"",
	    "profileArgs":"--name 'hello world'"
	  }
	}`)
	config, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := (&Model{}).dotnetRunResolved(config)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(resolved, &object); err != nil {
		t.Fatal(err)
	}
	args := toStrings(object["args"])
	want := []string{
		"run", "--project", "/repo/src/App.csproj", "--framework", "net10.0",
		"--configuration", "Debug", "--no-launch-profile", "--", "--name", "hello world",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("resolved args = %#v, want %#v", args, want)
	}
}

func TestDotnetRunSessionMatchesChannelAndCatchesEarlyExit(t *testing.T) {
	session := newDotnetRunSession(9)
	session.accept(openidea.Notification{
		Method: "channel",
		Params: json.RawMessage(`{"id":10,"data":{"kind":"stdout","text":"wrong\n"}}`),
	})
	session.accept(openidea.Notification{
		Method: "channel",
		Params: json.RawMessage(`{"id":9,"data":{"kind":"stdout","text":"build\n"}}`),
	})
	code := int32(0)
	window := Window
	event, _ := json.Marshal(openidea.WindowEvent{
		Window: &window,
		Event:  json.RawMessage(`{"type":"runExited","run":42,"code":0}`),
	})
	session.accept(openidea.Notification{Method: "event", Params: event})
	session.setRunID(42)
	text, exit, finished, limited, readErr := session.snapshot()
	if text != "build\n" || exit == nil || *exit != code || !finished || limited || readErr != nil {
		t.Fatalf("session snapshot = %q %v %v %v %v", text, exit, finished, limited, readErr)
	}
}

func TestDotnetPendingCancellationStopsReturnedRun(t *testing.T) {
	taskCtx, cancelTask := context.WithCancel(context.Background())
	startEntered := make(chan struct{})
	launchCanceled := make(chan bool, 1)
	stopIDs := make(chan uint32, 2)
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(callCtx context.Context, command string, input any) (json.RawMessage, error) {
			switch command {
			case "run_start":
				close(startEntered)
				<-taskCtx.Done()
				launchCanceled <- callCtx.Err() != nil
				return json.RawMessage(`{"id":42}`), nil
			case "run_stop":
				data, _ := json.Marshal(input)
				var request struct {
					ID uint32 `json:"id"`
				}
				if err := json.Unmarshal(data, &request); err != nil {
					return nil, err
				}
				stopIDs <- request.ID
				return json.RawMessage(`null`), nil
			default:
				return nil, fmt.Errorf("unexpected command %s", command)
			}
		},
	}
	type runResult struct {
		stop func()
		err  error
	}
	resultCh := make(chan runResult, 1)
	go func() {
		stop, err := m.startDotnetRun(
			taskCtx,
			newDotnetRunSession(7),
			7,
			[]string{"build", "/repo/App.csproj"},
			"/repo",
		)
		resultCh <- runResult{stop: stop, err: err}
	}()

	select {
	case <-startEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("run_start was not called")
	}
	cancelTask()

	var result runResult
	select {
	case result = <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not finish the run_start handshake")
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	if launchCanceled := <-launchCanceled; launchCanceled {
		t.Fatal("run_start inherited the canceled .NET task context")
	}
	select {
	case id := <-stopIDs:
		if id != 42 {
			t.Fatalf("run_stop id = %d, want 42", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the pending run was not stopped after run_start returned its id")
	}
	result.stop()
	select {
	case id := <-stopIDs:
		t.Fatalf("run_stop was repeated for id %d", id)
	default:
	}
}

func TestDotnetTaskRequiresMatchingGenerationAndRoot(t *testing.T) {
	m := &Model{}
	m.setRoot("/repo")
	task := &dotnetTask{generation: 4, root: "/repo"}
	m.dotnet = dotnetPane{root: "/repo", generation: 4, task: task}
	if !m.dotnetTaskCurrent(task) {
		t.Fatal("current .NET task was rejected")
	}
	m.dotnet.generation++
	if m.dotnetTaskCurrent(task) {
		t.Fatal("stale generation was accepted")
	}
	m.dotnet.generation = task.generation
	m.setRoot("/other")
	if m.dotnetTaskCurrent(task) {
		t.Fatal("stale root was accepted")
	}
}

func TestDotnetPanelRendersWorkspacePackagesAndTestsOffscreen(t *testing.T) {
	root := t.TempDir()
	m := &Model{ctx: context.Background()}
	m.setRoot(root)
	projectPath := filepath.Join(root, "src", "App.csproj")
	reference := filepath.Join(root, "src", "Library", "Library.csproj")
	solution := filepath.Join(root, "GoIdea.sln")
	m.dotnet = dotnetPane{
		root:          root,
		generation:    2,
		selected:      projectPath,
		configuration: "Debug",
		framework:     "net10.0",
		query:         "Newtonsoft.Json",
		model: dotnetWorkspace{
			Projects: []dotnetProject{{
				Path: projectPath, Name: "App", Frameworks: []string{"net10.0", "net9.0"},
				Configurations: []string{"Debug", "Release"},
				References:     []string{reference},
				Test:           true, Runner: dotnetRunnerMTP, Adapter: "xunit",
			}},
			Solutions: []dotnetSolution{{Path: solution, Projects: []string{projectPath}}},
			Errors:    []string{reference + ": missing SDK"},
		},
		packages: []dotnetPackage{
			{ID: "Newtonsoft.Json", Version: "13.0.3"},
			{ID: "System.Memory", Version: "4.5.5", Transitive: true},
		},
		found: []dotnetPackage{
			{ID: "Newtonsoft.Json", Version: "13.0.4"},
			{ID: "Newtonsoft.Json", Version: "13.0.3"},
		},
		tests:    []string{"App.Tests.Passes", "App.Tests.Fails"},
		output:   "Build succeeded",
		expanded: map[string]bool{solution: true, "output": true},
		versions: map[string]string{"Newtonsoft.Json": "13.0.4"},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.dotnetPanel(c, c.Theme())
	}, 760, 900)
	tester.Frame()
	for _, text := range []string{
		"Reload .NET Workspace",
		"Project evaluation failed",
		"GoIdea.sln",
		".NET project",
		"Configuration",
		"Target framework",
		"Build",
		"Restore",
		"Clean",
		"NuGet packages",
		"Refresh packages",
		"Newtonsoft.Json",
		"Install",
		"System.Memory",
		"Transitive",
		"Tests",
		"Discover .NET Tests",
		"Run all tests",
		"Install TRX reporting",
		"App.Tests.Passes",
		"Output",
	} {
		if !tester.HasText(text) {
			t.Fatalf("%q is missing from the .NET panel: %q", text, tester.Texts())
		}
	}
}
