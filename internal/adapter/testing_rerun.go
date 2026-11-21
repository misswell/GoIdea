package adapter

// The port of src/features/run/testing/rerun.ts's JVM branch: the failed
// test paths of a results tree and the configuration that runs only those
// tests, using Maven's `-Dtest=` and Gradle's `--tests` filter syntax.

import (
	"encoding/json"
	"regexp"
	"strings"

	"mayoi/internal/openidea"
)

// failedTestPaths collects the failed tests (rerun.ts): failed leaves, plus
// failed suites without any failed children reported.
func FailedTestPaths(results *TestResults) [][]string {
	var out [][]string
	var visit func(n *TestNode)
	visit = func(n *TestNode) {
		if len(n.Children) == 0 {
			if n.Own == TestFailed {
				out = append(out, n.Path)
			}
			return
		}
		var failedChildren []*TestNode
		for _, c := range n.Children {
			if testNodeStatus(c) == TestFailed {
				failedChildren = append(failedChildren, c)
			}
		}
		if len(failedChildren) == 0 {
			if n.Own == TestFailed {
				out = append(out, n.Path)
			}
			return
		}
		for _, c := range failedChildren {
			visit(c)
		}
	}
	for _, root := range results.Roots {
		visit(root)
	}
	return out
}

// escapeRegExp escapes a string for use in a filter regular expression
// (rerun.ts escapeRegExp: backslash before every metacharacter).
func escapeRegExp(s string) string {
	return regexp.MustCompile(`[.*+?^${}()|[\]\\]`).ReplaceAllStringFunc(s, func(c string) string {
		return "\\" + c
	})
}

// splitDashDash splits args at the first `--` (rerun.ts).
func splitDashDash(args []string) (before []string, after []string, hasAfter bool) {
	for i, a := range args {
		if a == "--" {
			return append([]string(nil), args[:i]...), append([]string(nil), args[i+1:]...), true
		}
	}
	return append([]string(nil), args...), nil, false
}

// dropFlag removes `flag value` / `flag=value` pairs (rerun.ts).
func dropFlag(args []string, flags []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		dropped := false
		for _, f := range flags {
			if a == f {
				i++
				dropped = true
				break
			}
			if strings.HasPrefix(a, f+"=") {
				dropped = true
				break
			}
		}
		if !dropped {
			out = append(out, a)
		}
	}
	return out
}

// jvmArgs derives the arguments that run only `paths`, with Maven Surefire's
// `-Dtest=Class#method1+method2,…` and Gradle's `--tests Class.method`
// (rerun.ts jvmArgs).
func jvmArgs(config *openidea.RunConfig, paths [][]string) []string {
	type testRef struct{ cls, method string }
	var tests []testRef
	var classes []string
	for _, p := range paths {
		if len(p) >= 2 {
			method := strings.TrimSuffix(p[len(p)-1], "()")
			tests = append(tests, testRef{p[0], method})
		} else if len(p) == 1 {
			classes = append(classes, p[0])
		}
	}
	if len(tests) == 0 && len(classes) == 0 {
		return nil
	}
	isMaven := config.Kind == "maven" || regexp.MustCompile(`\bmvnw?\b`).MatchString(config.Command)
	if isMaven {
		byClass := map[string][]string{}
		var order []string
		for _, t := range tests {
			if _, ok := byClass[t.cls]; !ok {
				order = append(order, t.cls)
			}
			byClass[t.cls] = append(byClass[t.cls], t.method)
		}
		var specs []string
		for _, cls := range order {
			seen := map[string]bool{}
			var unique []string
			for _, m := range byClass[cls] {
				if !seen[m] {
					seen[m] = true
					unique = append(unique, m)
				}
			}
			specs = append(specs, cls+"#"+strings.Join(unique, "+"))
		}
		for _, c := range classes {
			if _, ok := byClass[c]; !ok {
				specs = append(specs, c)
			}
		}
		var args []string
		for _, a := range config.Args {
			if !strings.HasPrefix(a, "-Dtest=") {
				args = append(args, a)
			}
		}
		return append(args, "-Dtest="+strings.Join(specs, ","))
	}
	seen := map[string]bool{}
	var filters []string
	for _, t := range tests {
		f := t.cls + "." + t.method
		if !seen[f] {
			seen[f] = true
			filters = append(filters, f)
		}
	}
	for _, c := range classes {
		if !seen[c] {
			seen[c] = true
			filters = append(filters, c)
		}
	}
	var args []string
	args = append(args, dropFlag(config.Args, []string{"--tests"})...)
	for _, f := range filters {
		args = append(args, "--tests", f)
	}
	return args
}

// PytestArgs derives the arguments that run only the selected pytest tests
// (rerun.ts pytestArgs): the node ids appended after the options, with the
// original run's test paths and -k expressions dropped.
func PytestArgs(base []string, paths [][]string) []string {
	seen := map[string]bool{}
	var ids []string
	for _, p := range paths {
		joined := strings.Join(p, "::")
		if joined != "" && !seen[joined] {
			seen[joined] = true
			ids = append(ids, joined)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	kept := dropFlag(base, []string{"-k"})
	dropExt := regexp.MustCompile(`\.py(::|$)`)
	dropDir := regexp.MustCompile(`^tests?/?$`)
	var out []string
	for _, a := range kept {
		if dropExt.MatchString(a) || dropDir.MatchString(a) {
			continue
		}
		out = append(out, a)
	}
	return append(out, ids...)
}

// goSinglePackage returns the one package all paths belong to, "" when they
// span several (rerun.ts's grouping key).
func goSinglePackage(paths [][]string) string {
	pkg := ""
	for _, p := range paths {
		if len(p) == 0 {
			continue
		}
		if pkg == "" {
			pkg = p[0]
		} else if pkg != p[0] {
			return ""
		}
	}
	return pkg
}

// goArgs derives the arguments that run only the selected go tests
// (rerun.ts goArgs): one package per invocation, `-run` with the anchored
// test-name pattern, benchmarks on `-bench`.
func goArgs(base []string, paths [][]string, file string) []string {
	pkgs := map[string]bool{}
	var order []string
	for _, p := range paths {
		if len(p) > 0 && p[0] != "" && !pkgs[p[0]] {
			pkgs[p[0]] = true
			order = append(order, p[0])
		}
	}
	if len(pkgs) != 1 {
		return nil
	}
	parts := GoTestParts(base)
	targets := order
	if file != "" {
		targets = []string{file}
	} else if pkgs["command-line-arguments"] {
		targets = parts.Targets
	}
	onlyPackage := false
	for _, p := range paths {
		if len(p) == 1 {
			onlyPackage = true
		}
	}
	if onlyPackage {
		return append([]string{"test"}, append(parts.Flags, append(targets, parts.Program...)...)...)
	}
	pattern := func(selected [][]string) string {
		seen := map[string]bool{}
		var uniq []string
		for _, path := range selected {
			names := path[1:]
			for i := range names {
				names[i] = "^" + escapeRegExp(names[i]) + "$"
			}
			joined := strings.Join(names, "/")
			if !seen[joined] {
				seen[joined] = true
				uniq = append(uniq, joined)
			}
		}
		return strings.Join(uniq, "|")
	}
	var tests, benchmarks [][]string
	for _, p := range paths {
		if len(p) > 1 && strings.HasPrefix(p[1], "Benchmark") {
			benchmarks = append(benchmarks, p)
		} else {
			tests = append(tests, p)
		}
	}
	args := []string{"test"}
	args = append(args, WithoutGoSelection(parts.Flags)...)
	run := "^$"
	if len(tests) > 0 {
		run = pattern(tests)
	}
	args = append(args, "-run", run)
	if len(benchmarks) > 0 {
		args = append(args, "-bench", pattern(benchmarks))
	}
	args = append(args, targets...)
	args = append(args, WithoutGoSelection(parts.Program)...)
	return args
}

// FrameworkFor is the testing framework of a configuration
// (testing/index.ts frameworkFor).
func FrameworkFor(config *openidea.RunConfig) string {
	switch config.Kind {
	case "maven", "gradle":
		return "jvm"
	case "django-test":
		return "unittest"
	case "python":
		cmd := config.Command + " " + strings.Join(config.Args, " ")
		if regexp.MustCompile(`\bmanage\.py test\b`).MatchString(cmd) {
			return "unittest"
		}
		return "pytest"
	case "go":
		return "go"
	case "cargo":
		return "cargo"
	case "npm", "deno", "bun":
		return "js"
	case "pytest":
		return "pytest"
	case "phpunit":
		return "phpunit"
	case "ruby", "rake", "rspec":
		return "ruby"
	case "dotnet-test":
		return "dotnet"
	}
	cmd := config.Command + " " + strings.Join(config.Args, " ")
	if regexp.MustCompile(`(^|[/\\])dotnet(?:\.exe)?$`).MatchString(config.Command) &&
		len(config.Args) > 0 && config.Args[0] == "test" {
		return "dotnet"
	}
	if regexp.MustCompile(`\bcargo\b`).MatchString(cmd) {
		return "cargo"
	}
	if regexp.MustCompile(`\b(jest|vitest|npm|pnpm|yarn|bun|npx|deno)\b`).MatchString(cmd) {
		return "js"
	}
	if regexp.MustCompile(`\bpytest\b`).MatchString(cmd) {
		return "pytest"
	}
	if regexp.MustCompile(`\bmanage\.py test\b`).MatchString(cmd) {
		return "unittest"
	}
	if regexp.MustCompile(`(^|[/\\\s])(phpunit|pest)(\.phar|\.bat)?(\s|$)`).MatchString(cmd) {
		return "phpunit"
	}
	if regexp.MustCompile(`\bgo\s+test\b`).MatchString(cmd) {
		return "go"
	}
	if regexp.MustCompile(`\b(gradlew?|mvnw?)\b`).MatchString(cmd) {
		return "jvm"
	}
	return ""
}

// RerunFailedConfig derives the configuration that runs only the failed
// tests of `config` (rerun.ts rerunFailedConfig): named "Failed: <name>",
// marked detected so it disappears from the user's list. nil when the
// framework or the paths are not supported.
func RerunFailedConfig(config *openidea.RunConfig, paths [][]string) *openidea.RunConfig {
	var args []string
	switch FrameworkFor(config) {
	case "jvm":
		args = jvmArgs(config, paths)
	case "go":
		// Go filters apply to every package in an invocation, so each
		// package keeps its own selection; a single package reruns directly.
		if pkg := goSinglePackage(paths); pkg != "" {
			var selected [][]string
			for _, p := range paths {
				if len(p) > 0 && p[0] == pkg {
					selected = append(selected, p)
				}
			}
			args = goArgs(config.Args, selected, "")
		}
	case "cargo":
		args = CargoArgs(config.Args, paths)
	case "pytest":
		args = PytestArgs(config.Args, paths)
	case "js":
		args = JsArgs(config, paths)
	case "phpunit":
		args = PHPUnitArgs(config.Args, "", PHPUnitFilter(paths), true)
	case "dotnet":
		filter := DotnetFilter(paths)
		if filter != "" {
			args = DotnetArgs(config.Args, filter)
		}
	case "unittest":
		labels := UnittestLabels(paths)
		args = UnittestArgs(config.Args, labels)
		if args != nil {
			var params map[string]any
			if _, ok := config.Params["target"]; ok {
				params = map[string]any{"target": FormatArgs(labels)}
			}
			return RerunConfigWithArgsAndParams(config, args, params)
		}
	}
	if args == nil {
		return nil
	}
	return RerunConfigWithArgs(config, args)
}

// RerunConfigWithArgs applies the "Failed: <name>" identity and filter
// arguments to a configuration while preserving every unmodelled field.
func RerunConfigWithArgs(config *openidea.RunConfig, args []string) *openidea.RunConfig {
	return RerunConfigWithArgsAndParams(config, args, nil)
}

func RerunConfigWithArgsAndParams(
	config *openidea.RunConfig,
	args []string,
	params map[string]any,
) *openidea.RunConfig {
	// The derived configuration carries the base's raw object with the
	// filter arguments and the "Failed: <name>" identity applied, so
	// run_start gets every field GoIDEA does not model.
	src := config.Raw()
	if src == nil {
		var err error
		src, err = json.Marshal(config)
		if err != nil {
			return nil
		}
	}
	obj := map[string]any{}
	if err := json.Unmarshal(src, &obj); err != nil {
		return nil
	}
	obj["args"] = args
	if params != nil {
		merged := map[string]any{}
		if existing, ok := obj["params"].(map[string]any); ok {
			for key, value := range existing {
				merged[key] = value
			}
		}
		for key, value := range params {
			merged[key] = value
		}
		obj["params"] = merged
	}
	obj["name"] = "Failed: " + strings.TrimPrefix(config.Name, "Failed: ")
	obj["detected"] = false
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil
	}
	derived, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		return nil
	}
	if config.Env != nil {
		env := map[string]string{}
		for k, v := range config.Env {
			env[k] = v
		}
		derived.Env = env
	}
	return derived
}
