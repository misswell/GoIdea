package adapter

// The async half of the javaLaunch.ts port: module resolution through the
// backend's indexed file search (never a local WalkDir guess) and the shared,
// cancellable launch preparation Run and Debug both go through. Every
// failure is returned as an error — a failed preparation never hands an
// unresolved configuration to run_start.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"mayoi/internal/openidea"
)

// Backend is the subset of the OpenIDEA RPC surface a Java launch needs.
// internal/app wires it to the live server; tests inject fakes, which is the
// seam that proves run_start is never called after a failed preparation.
type Backend interface {
	// FindFiles is the backend's indexed file search (commands.findFiles).
	FindFiles(ctx context.Context, query string, limit int) ([]openidea.FileMatch, error)
	// ReadText is commands.fsReadText.
	ReadText(ctx context.Context, path string) (string, error)
	// RunConfigs is commands.runConfigs.
	RunConfigs(ctx context.Context) ([]openidea.RunConfig, error)
	// ResolveLaunch is commands.javaResolveLaunch: the raw reply stays raw so
	// a malformed one is an error, not a zero value.
	ResolveLaunch(ctx context.Context, module string, test bool, policy string, request uint32) (json.RawMessage, error)
}

// JavaBuildFailedError is a build stopped the launch: the message is ready
// for the run console and the notification.
type JavaBuildFailedError struct {
	Message string
}

func (e *JavaBuildFailedError) Error() string { return e.Message }

// The package statement of a source file, matched per line like upstream's
// /^\s*package\s+([\w.]+)\s*[;\r\n]/m.
var packageDecl = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+)\s*[;\r\n]`)

// The jdtls verdicts that mean the module's Java project does not exist (yet)
// in the language server, matched like upstream prepareJavaLaunch does.
var notImported = regexp.MustCompile(`(?i)jdtls:.*(?:does not belong to any java project|references non-existing project|not a java project)`)

// FindJavaModule recovers the classpath module of a configuration: the module
// it names, else the module of the source file the main class comes from,
// found through the backend's indexed search and the source's package
// statement. "" is a valid answer (the project root); ok=false when the
// module cannot be determined.
func FindJavaModule(ctx context.Context, backend Backend, config *openidea.RunConfig) (string, bool, error) {
	if config.Module != nil {
		return *config.Module, true, nil
	}
	main := ParseJavaArgs(config.Args).MainClass
	if main == "" {
		return "", false, nil
	}
	// A binary name's outer class: TopLevel$Nested runs from TopLevel.java.
	outer := main
	if i := strings.Index(main, "$"); i >= 0 {
		outer = main[:i]
	}
	simple := outer[strings.LastIndex(outer, ".")+1:]
	pkg := ""
	if i := strings.LastIndex(outer, "."); i >= 0 {
		pkg = outer[:i]
	}
	names := []string{simple + ".java", simple + ".kt"}
	if strings.HasSuffix(simple, "Kt") {
		names = append(names, strings.TrimSuffix(simple, "Kt")+".kt")
	}
	var matches []openidea.FileMatch
	for _, name := range names {
		found, err := backend.FindFiles(ctx, name, 1000)
		if err != nil {
			return "", false, err
		}
		for _, f := range found {
			base := f.RelPath[strings.LastIndex(f.RelPath, "/")+1:]
			for _, want := range names {
				if base == want {
					matches = append(matches, f)
					break
				}
			}
		}
	}
	configs, err := backend.RunConfigs(ctx)
	if err != nil {
		return "", false, err
	}
	modules := map[string]bool{}
	for _, file := range matches {
		text, err := backend.ReadText(ctx, file.Path)
		if err != nil {
			return "", false, err
		}
		sourcePackage := packageDecl.FindStringSubmatch(text)
		if sourcePackage == nil || sourcePackage[1] != pkg {
			continue
		}
		modules[JVMModuleDir(file.RelPath, configs, nil)] = true
	}
	if len(modules) != 1 {
		return "", false, nil
	}
	for module := range modules {
		return module, true, nil
	}
	return "", false, nil
}

// ResolveJavaModule resolves the module a configuration launches from, or
// fails with the message upstream resolveJavaModule throws.
func ResolveJavaModule(ctx context.Context, backend Backend, config *openidea.RunConfig) (string, error) {
	module, ok, err := FindJavaModule(ctx, backend, config)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("无法确定主类 '%s' 所在的模块",
			ParseJavaArgs(config.Args).MainClass)
	}
	return module, nil
}

// Preparation is the shared, cancellable build and classpath preparation Run
// and Debug both go through.
type Preparation struct {
	// Request identifies the backend request so the user can cancel it.
	Request uint32
	// Report writes into the run console: kind is "system" or "stderr".
	Report func(text, kind string)
	// Policy overrides the configuration's build policy; unset reads it
	// (default: automatic/incremental).
	Policy string
	// SaveAll saves open documents first, as IDEA's Before launch does.
	SaveAll func() error
}

// PreparedJavaLaunch is what a successful preparation hands to the caller.
type PreparedJavaLaunch struct {
	Module   string
	Info     openidea.JavaLaunchInfo
	Parsed   JavaCommandLine
	Command  string
	Resolved []string
}

// throwIfAborted is the Go reading of upstream signal.throwIfAborted(): the
// launch stops as soon as its context is cancelled.
func throwIfAborted(ctx context.Context) error { return ctx.Err() }

// PrepareJavaModule saves open documents, then resolves the launch module.
func PrepareJavaModule(ctx context.Context, backend Backend, config *openidea.RunConfig, preparation *Preparation) (string, error) {
	if err := throwIfAborted(ctx); err != nil {
		return "", err
	}
	if preparation != nil && preparation.SaveAll != nil {
		if err := preparation.SaveAll(); err != nil {
			return "", err
		}
	}
	if err := throwIfAborted(ctx); err != nil {
		return "", err
	}
	module, err := ResolveJavaModule(ctx, backend, config)
	if err != nil {
		return "", err
	}
	if err := throwIfAborted(ctx); err != nil {
		return "", err
	}
	return module, nil
}

// PrepareJavaLaunch runs the shared preparation: build policy, module, build
// and classpath through java_resolve_launch, then the resolved command line.
// It returns an error for every condition upstream refuses to launch from,
// and the caller must not start anything on an error.
func PrepareJavaLaunch(ctx context.Context, backend Backend, config *openidea.RunConfig, preparation *Preparation) (*PreparedJavaLaunch, error) {
	policy := BuildPolicyOf(config)
	if preparation != nil && preparation.Policy != "" {
		policy = preparation.Policy
	}
	report := func(text, kind string) {
		if preparation != nil && preparation.Report != nil {
			preparation.Report(text, kind)
		}
	}
	if err := throwIfAborted(ctx); err != nil {
		return nil, err
	}
	parsed := ParseJavaArgs(config.Args)
	if parsed.MainClass == "" {
		return nil, fmt.Errorf("配置 '%s' 没有主类", config.Name)
	}
	report(fmt.Sprintf("正在构建并解析 %s 的 classpath…\n", parsed.MainClass), "system")
	module, err := PrepareJavaModule(ctx, backend, config, preparation)
	if err != nil {
		return nil, err
	}
	rawInfo, err := backend.ResolveLaunch(ctx, module, parsed.Classpath == "test", policy, preparationRequest(preparation))
	if err != nil {
		// The jdtls verdicts that mean the module has no Java project (yet):
		// say what to wait for instead of relaying the raw RPC text.
		if openidea.ErrorKind(err) != "cancelled" && notImported.MatchString(err.Error()) {
			name := module
			if name == "" {
				name = "."
			}
			return nil, fmt.Errorf("Java 模块 '%s' 尚未导入。请等待 Java 项目导入完成；若仍然失败，检查运行配置的 module 是否正确、是否被父级 Maven 或 Gradle 构建包含，或直接把该目录作为项目打开", name)
		}
		return nil, err
	}
	if err := throwIfAborted(ctx); err != nil {
		return nil, err
	}
	var info openidea.JavaLaunchInfo
	if err := json.Unmarshal(rawInfo, &info); err != nil {
		return nil, fmt.Errorf("java_resolve_launch 返回的数据无法解析：%w", err)
	}
	if errors := FormatCompileErrors(&info); errors != "" {
		report(errors, "stderr")
	}
	// IDEA's default: a failed build (or one with compile errors) does not
	// start the program; only "Build, no error check" continues with the last
	// compiled classes.
	if info.Build != nil && (*info.Build == "failed" || *info.Build == "withErrors") && policy != "makeAndContinueOnErrors" {
		reason := "构建有编译错误；未启动 '" + config.Name + "'。"
		if *info.Build == "failed" {
			reason = "构建失败；未启动 '" + config.Name + "'。"
		}
		return nil, &JavaBuildFailedError{Message: reason}
	}
	if parsed.Classpath != "" && len(info.ClassPaths) == 0 {
		return nil, fmt.Errorf("classpath 为空：该模块是否已被 Java 语言服务导入？")
	}
	command, args := ResolveJavaCommand(config.Kind, config.Command, config.Args, &info)
	return &PreparedJavaLaunch{
		Module:   module,
		Info:     info,
		Parsed:   parsed,
		Command:  command,
		Resolved: args,
	}, nil
}

func preparationRequest(preparation *Preparation) uint32 {
	if preparation == nil {
		return 0
	}
	return preparation.Request
}
