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

// escapeRegExp escapes a string for use in a filter regular expression.
func escapeRegExp(s string) string {
	return regexp.MustCompile(`[.*+?^${}()|[\]\\]`).ReplaceAllString(s, "\\$&")
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

// FrameworkFor is the testing framework of a configuration (testing/index.ts
// frameworkFor's jvm branch, the one GoIDEA parses today).
func FrameworkFor(config *openidea.RunConfig) string {
	switch config.Kind {
	case "maven", "gradle":
		return "jvm"
	}
	cmd := config.Command + " " + strings.Join(config.Args, " ")
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
	if FrameworkFor(config) != "jvm" || len(paths) == 0 {
		return nil
	}
	args := jvmArgs(config, paths)
	if args == nil {
		return nil
	}
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
