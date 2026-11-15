package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

// fakeBackend is the injectable RPC client: every launch test decides what
// the backend answers, including failures and malformed replies.
type fakeBackend struct {
	files      map[string][]openidea.FileMatch // find_files query -> hits
	texts      map[string]string               // fs_read_text path -> text
	configs    []openidea.RunConfig
	resolve    func(module string, test bool, policy string, request uint32) (json.RawMessage, error)
	findErr    error
	readErr    error
	configsErr error

	resolveCalls []resolveCall
	saved        []savedConfigs
}

type resolveCall struct {
	module  string
	test    bool
	policy  string
	request uint32
}

type savedConfigs struct {
	configs            []map[string]any
	removedDetectedIDs any
}

func (f *fakeBackend) FindFiles(ctx context.Context, query string, limit int) ([]openidea.FileMatch, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.files[query], nil
}

func (f *fakeBackend) ReadText(ctx context.Context, path string) (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	return f.texts[path], nil
}

func (f *fakeBackend) RunConfigs(ctx context.Context) ([]openidea.RunConfig, error) {
	if f.configsErr != nil {
		return nil, f.configsErr
	}
	return f.configs, nil
}

func (f *fakeBackend) ResolveLaunch(ctx context.Context, module string, test bool, policy string, request uint32) (json.RawMessage, error) {
	f.resolveCalls = append(f.resolveCalls, resolveCall{module, test, policy, request})
	if f.resolve == nil {
		return json.Marshal(openidea.JavaLaunchInfo{
			Build: strPtr("succeeded"), ClassPaths: []string{"/p/target/classes"},
			PathListSeparator: ":", JavaExec: strPtr("/usr/bin/java"),
		})
	}
	return f.resolve(module, test, policy, request)
}

func (f *fakeBackend) SaveRunConfigs(ctx context.Context, configs []map[string]any, removedDetectedIDs any) (json.RawMessage, error) {
	f.saved = append(f.saved, savedConfigs{configs, removedDetectedIDs})
	return json.Marshal(nil)
}

func appConfig(args ...string) *openidea.RunConfig {
	return &openidea.RunConfig{
		ID: "user:main", Name: "Main", Kind: "application", Command: "java",
		Args: append([]string{"-cp", "${classpath}"}, args...), Module: strPtr("app"),
	}
}

func TestFindJavaModule(t *testing.T) {
	ctx := context.Background()

	// The configuration's module wins, and "" is a valid answer (the root).
	empty := ""
	if module, ok, _ := FindJavaModule(ctx, &fakeBackend{}, &openidea.RunConfig{Module: &empty, Args: []string{"a.Main"}}); !ok || module != "" {
		t.Fatalf("module \"\" must be kept: %q ok=%v", module, ok)
	}

	// The outer class of a nested main class decides the source file.
	nested := &fakeBackend{
		files: map[string][]openidea.FileMatch{
			"App.java": {{Path: "/p/app/src/main/java/a/App.java", RelPath: "app/src/main/java/a/App.java"}},
		},
		texts: map[string]string{"/p/app/src/main/java/a/App.java": "package a;\npublic class App { class Inner {} }"},
	}
	module, ok, err := FindJavaModule(ctx, nested, &openidea.RunConfig{Args: []string{"-cp", "${classpath}", "a.App$Inner"}})
	if err != nil || !ok || module != "app" {
		t.Fatalf("nested class module = %q ok=%v err=%v", module, ok, err)
	}

	// Kotlin sources, including the FooKt facade of Foo.kt.
	kt := &fakeBackend{
		files: map[string][]openidea.FileMatch{
			"Main.kt": {{Path: "/p/svc/src/main/kotlin/a/Main.kt", RelPath: "svc/src/main/kotlin/a/Main.kt"}},
		},
		texts: map[string]string{"/p/svc/src/main/kotlin/a/Main.kt": "package a\nfun main() {}"},
	}
	module, ok, err = FindJavaModule(ctx, kt, &openidea.RunConfig{Args: []string{"a.MainKt"}})
	if err != nil || !ok || module != "svc" {
		t.Fatalf("kotlin module = %q ok=%v err=%v", module, ok, err)
	}

	// The package statement must match the main class: a same-named file in
	// another package is not the module.
	mismatch := &fakeBackend{
		files: map[string][]openidea.FileMatch{
			"App.java": {{Path: "/p/other/src/main/java/b/App.java", RelPath: "other/src/main/java/b/App.java"}},
		},
		texts: map[string]string{"/p/other/src/main/java/b/App.java": "package b;\npublic class App {}"},
	}
	if _, ok, err := FindJavaModule(ctx, mismatch, &openidea.RunConfig{Args: []string{"a.App"}}); ok || err != nil {
		t.Fatalf("package mismatch must not resolve a module: ok=%v err=%v", ok, err)
	}

	// Two modules holding the same class: ambiguous, refuse.
	ambiguous := &fakeBackend{
		files: map[string][]openidea.FileMatch{
			"App.java": {
				{Path: "/p/app/src/main/java/a/App.java", RelPath: "app/src/main/java/a/App.java"},
				{Path: "/p/web/src/main/java/a/App.java", RelPath: "web/src/main/java/a/App.java"},
			},
		},
		texts: map[string]string{
			"/p/app/src/main/java/a/App.java": "package a;\npublic class App {}",
			"/p/web/src/main/java/a/App.java": "package a;\npublic class App {}",
		},
	}
	if _, ok, _ := FindJavaModule(ctx, ambiguous, &openidea.RunConfig{Args: []string{"a.App"}}); ok {
		t.Fatalf("an ambiguous module must not resolve")
	}
}

func TestResolveJavaModuleRefusesUnknown(t *testing.T) {
	_, err := ResolveJavaModule(context.Background(), &fakeBackend{}, &openidea.RunConfig{Args: []string{"a.Missing"}})
	if err == nil || !strings.Contains(err.Error(), "a.Missing") {
		t.Fatalf("unresolved module must name the main class, got %v", err)
	}
}

func TestPrepareJavaLaunchRefusesToStartWithoutAMainClass(t *testing.T) {
	_, err := PrepareJavaLaunch(context.Background(), &fakeBackend{},
		&openidea.RunConfig{Name: "Jar", Kind: "application", Command: "java", Args: []string{"-jar", "app.jar"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "Jar") {
		t.Fatalf("a configuration without a main class must be refused, got %v", err)
	}
}

func TestPrepareJavaLaunchStopsOnRPCFailure(t *testing.T) {
	boom := errors.New("jdtls: request timed out")
	backend := &fakeBackend{resolve: func(string, bool, string, uint32) (json.RawMessage, error) { return nil, boom }}
	prepared, err := PrepareJavaLaunch(context.Background(), backend, appConfig("a.Main"), nil)
	if prepared != nil || !errors.Is(err, boom) {
		t.Fatalf("an RPC failure must stop the launch with the real error, got %v / %v", prepared, err)
	}
	if len(backend.resolveCalls) != 1 {
		t.Fatalf("resolve must be tried once, got %d", len(backend.resolveCalls))
	}
}

func TestPrepareJavaLaunchExplainsAnUnimportedModule(t *testing.T) {
	backend := &fakeBackend{resolve: func(string, bool, string, uint32) (json.RawMessage, error) {
		return nil, errors.New("jdtls: Module app does not belong to any java project")
	}}
	_, err := PrepareJavaLaunch(context.Background(), backend, appConfig("a.Main"), nil)
	if err == nil || !strings.Contains(err.Error(), "尚未导入") || !strings.Contains(err.Error(), "app") {
		t.Fatalf("an unimported module must be explained, got %v", err)
	}
}

func TestPrepareJavaLaunchStopsOnABuildFailure(t *testing.T) {
	info := openidea.JavaLaunchInfo{
		Build: strPtr("failed"), PathListSeparator: ":",
		Errors:     []openidea.JavaCompileError{{Path: "app/src/main/java/a/App.java", Line: 3, Column: 9, Message: "';' expected"}},
		ErrorCount: 1,
	}
	raw, _ := json.Marshal(info)
	backend := &fakeBackend{resolve: func(string, bool, string, uint32) (json.RawMessage, error) { return raw, nil }}
	var reported []string
	prep := &Preparation{Report: func(text, kind string) { reported = append(reported, kind+":"+text) }}
	prepared, err := PrepareJavaLaunch(context.Background(), backend, appConfig("a.Main"), prep)
	var buildFailed *JavaBuildFailedError
	if prepared != nil || !errors.As(err, &buildFailed) || !strings.Contains(err.Error(), "构建失败") {
		t.Fatalf("a failed build must stop the launch, got %v / %v", prepared, err)
	}
	if len(reported) == 0 ||
		!strings.Contains(reported[0], "a.Main") ||
		!strings.Contains(strings.Join(reported, "\n"), "error: ';' expected") {
		t.Fatalf("the build progress and compile errors must reach the console, got %v", reported)
	}

	// "Build, no error check" continues with the last compiled classes: the
	// classpath still resolves, so the launch goes ahead.
	continued, _ := json.Marshal(openidea.JavaLaunchInfo{
		Build: strPtr("withErrors"), ClassPaths: []string{"/p/target/classes"},
		PathListSeparator: ":", ErrorCount: 1,
	})
	backend.resolve = func(string, bool, string, uint32) (json.RawMessage, error) { return continued, nil }
	prepared, err = PrepareJavaLaunch(context.Background(), backend, &openidea.RunConfig{
		Name: "Main", Kind: "application", Command: "java", Args: []string{"-cp", "${classpath}", "a.Main"},
		Module: strPtr("app"), BuildPolicy: strPtr("makeAndContinueOnErrors"),
	}, nil)
	if err != nil || prepared == nil {
		t.Fatalf("makeAndContinueOnErrors must continue, got %v", err)
	}
}

func TestPrepareJavaLaunchRefusesAnEmptyClasspath(t *testing.T) {
	raw, _ := json.Marshal(openidea.JavaLaunchInfo{Build: strPtr("succeeded"), PathListSeparator: ":"})
	backend := &fakeBackend{resolve: func(string, bool, string, uint32) (json.RawMessage, error) { return raw, nil }}
	_, err := PrepareJavaLaunch(context.Background(), backend, appConfig("a.Main"), nil)
	if err == nil || !strings.Contains(err.Error(), "classpath 为空") {
		t.Fatalf("an empty classpath must be refused, got %v", err)
	}
	// An explicit classpath does not need the resolved one.
	prepared, err := PrepareJavaLaunch(context.Background(), backend, &openidea.RunConfig{
		Name: "Main", Kind: "application", Command: "java", Args: []string{"-cp", "out", "a.Main"}, Module: strPtr("app"),
	}, nil)
	if err != nil || prepared == nil {
		t.Fatalf("an explicit classpath must launch, got %v", err)
	}
}

func TestPrepareJavaLaunchStopsOnMalformedReply(t *testing.T) {
	backend := &fakeBackend{resolve: func(string, bool, string, uint32) (json.RawMessage, error) {
		return json.RawMessage("{not json"), nil
	}}
	_, err := PrepareJavaLaunch(context.Background(), backend, appConfig("a.Main"), nil)
	if err == nil || !strings.Contains(err.Error(), "无法解析") {
		t.Fatalf("a malformed reply must be an error, got %v", err)
	}
}

func TestPrepareJavaLaunchResolvesTheCommandLine(t *testing.T) {
	backend := &fakeBackend{resolve: func(module string, test bool, policy string, request uint32) (json.RawMessage, error) {
		if module != "app" || test || policy != "automatic" {
			return nil, fmt.Errorf("unexpected resolve: module=%s test=%v policy=%s", module, test, policy)
		}
		return json.Marshal(openidea.JavaLaunchInfo{
			Build: strPtr("succeeded"), ProjectRoot: strPtr("/p"),
			ClassPaths: []string{"/p/app/target/classes"}, ModulePaths: []string{"/p/mods"},
			PathListSeparator: ":", JavaExec: strPtr("/jdk/bin/java"),
		})
	}}
	cfg := &openidea.RunConfig{
		Name: "Main", Kind: "spring-boot", Command: "java", IsTest: true,
		Args: []string{"-Xmx512m", "-cp", "${classpath}", "a.App", "serve"}, Module: strPtr("app"),
	}
	prepared, err := PrepareJavaLaunch(context.Background(), backend, cfg, nil)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if prepared.Command != "/jdk/bin/java" {
		t.Fatalf("command = %q, want the resolved javaExec", prepared.Command)
	}
	// The test flag follows the classpath placeholder, not cfg.IsTest: the
	// runtime placeholder resolved the runtime classpath.
	if backend.resolveCalls[0].test {
		t.Fatalf("a runtime placeholder must resolve the runtime classpath")
	}
	want := []string{"-Dspring.output.ansi.enabled=always", "--module-path", "/p/mods", "-Xmx512m", "-cp", "/p/app/target/classes", "a.App", "serve"}
	if strings.Join(prepared.Resolved, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("resolved args = %v, want %v", prepared.Resolved, want)
	}
	if prepared.Parsed.MainClass != "a.App" || strings.Join(prepared.Parsed.ProgramArgs, " ") != "serve" {
		t.Fatalf("parsed = %+v, the main class and program args must survive", prepared.Parsed)
	}
}

func TestPrepareJavaLaunchStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	backend := &fakeBackend{}
	if _, err := PrepareJavaLaunch(ctx, backend, appConfig("a.Main"), nil); err == nil {
		t.Fatalf("a cancelled preparation must stop")
	}
	if len(backend.resolveCalls) != 0 {
		t.Fatalf("a cancelled launch must not reach java_resolve_launch")
	}
}
