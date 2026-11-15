// Package adapter ports the OpenIDEA frontend's client-side flows to Go,
// function by function: the same parameters, state transitions, error
// handling and async behaviour as the TypeScript originals, without
// re-implementing anything the OpenIDEA backend already does. The UI package
// (internal/app) drives these flows; the backend RPC surface stays in
// internal/openidea.
//
// java_launch.go is the port of src/platform/javaLaunch.ts at the pinned
// OpenIDEA revision: Java "Application" run configurations shared by Run and
// Debug. The command line is
//
//	java [vm options] -cp ${classpath} <main class> [program args]
//
// and ${classpath} (or ${testClasspath}) and the `java` launcher are resolved
// through jdtls when the configuration starts.
package adapter

import (
	"fmt"
	"regexp"
	"strings"

	"mayoi/internal/openidea"
)

// The classpath placeholders a run configuration's command line may hold.
const (
	Classpath     = "${classpath}"
	TestClasspath = "${testClasspath}"
)

// JVMApplicationKinds are the run configuration kinds launched as a JVM main
// class.
var JVMApplicationKinds = []string{"application", "spring-boot"}

// IsJVMApplication reports whether the configuration kind is launched as a
// JVM main class.
func IsJVMApplication(kind string) bool {
	for _, k := range JVMApplicationKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// withValue lists `java` options whose value is the next argument.
var withValue = map[string]bool{
	"-cp":                    true,
	"-classpath":             true,
	"--class-path":           true,
	"-p":                     true,
	"--module-path":          true,
	"--upgrade-module-path":  true,
	"--add-modules":          true,
	"--add-opens":            true,
	"--add-exports":          true,
	"--add-reads":            true,
	"--patch-module":         true,
	"--limit-modules":        true,
	"--enable-native-access": true,
}

// JavaCommandLine splits `java` arguments into VM options, main class and
// program arguments, the port of upstream parseJavaArgs.
type JavaCommandLine struct {
	// VMArgs are the VM options, without the classpath.
	VMArgs []string
	// MainClass is the binary name of the main class, "" for -jar or none.
	MainClass string
	// JarPath is the argument of -jar, "" without it.
	JarPath string
	// ProgramArgs follow the main class (or the jar path).
	ProgramArgs []string
	// Classpath is which placeholder the configuration uses: "runtime",
	// "test", or "" for an explicit classpath or none.
	Classpath string
	// ExplicitClasspath is an explicit -cp value other than a placeholder.
	ExplicitClasspath string
}

// ParseJavaArgs splits `java` arguments into VM options, main class and
// program arguments.
func ParseJavaArgs(args []string) JavaCommandLine {
	var parsed JavaCommandLine
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-jar" {
			if i+1 < len(args) {
				parsed.JarPath = args[i+1]
			}
			parsed.ProgramArgs = append([]string(nil), args[min(i+2, len(args)):]...)
			return parsed
		}
		if a == "-cp" || a == "-classpath" || a == "--class-path" {
			i++
			value := ""
			if i < len(args) {
				value = args[i]
			}
			switch value {
			case Classpath:
				parsed.Classpath = "runtime"
			case TestClasspath:
				parsed.Classpath = "test"
			default:
				parsed.ExplicitClasspath = value
			}
			continue
		}
		if withValue[a] {
			i++
			if i < len(args) {
				parsed.VMArgs = append(parsed.VMArgs, a, args[i])
			} else {
				parsed.VMArgs = append(parsed.VMArgs, a, "")
			}
			continue
		}
		if strings.HasPrefix(a, "-") {
			parsed.VMArgs = append(parsed.VMArgs, a)
			continue
		}
		parsed.MainClass = a
		parsed.ProgramArgs = append([]string(nil), args[i+1:]...)
		return parsed
	}
	return parsed
}

// ConsoleVMArgs are the VM options a launch adds for the console, as IDEA's
// Spring Boot configurations do: the console renders ANSI colours, but Spring
// only detects a terminal, and the process runs on pipes. Kept out when the
// configuration sets the property itself.
func ConsoleVMArgs(kind string, args []string) []string {
	if kind != "spring-boot" {
		return nil
	}
	for _, a := range ParseJavaArgs(args).VMArgs {
		if strings.HasPrefix(a, "-Dspring.output.ansi.enabled=") {
			return nil
		}
	}
	return []string{"-Dspring.output.ansi.enabled=always"}
}

// ResolveJavaCommand returns the command line with the placeholders and the
// launcher resolved, the port of upstream resolveJavaCommand.
func ResolveJavaCommand(kind, command string, args []string, info *openidea.JavaLaunchInfo) (string, []string) {
	cp := strings.Join(info.ClassPaths, info.PathListSeparator)
	resolved := append([]string(nil), ConsoleVMArgs(kind, args)...)
	if len(info.ModulePaths) > 0 && !hasFlag(args, "-p", "--module-path") {
		resolved = append(resolved, "--module-path", strings.Join(info.ModulePaths, info.PathListSeparator))
	}
	for _, a := range args {
		if a == Classpath || a == TestClasspath {
			a = cp
		}
		resolved = append(resolved, a)
	}
	if command == "java" && info.JavaExec != nil && *info.JavaExec != "" {
		command = *info.JavaExec
	}
	return command, resolved
}

func hasFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

// FormatCompileErrors renders a launch's compile errors
// (`path:line:col: error: …`) and their likely causes for the console;
// "" when there are none.
func FormatCompileErrors(info *openidea.JavaLaunchInfo) string {
	var lines []string
	for _, e := range info.Errors {
		lines = append(lines, fmt.Sprintf("%s:%d:%d: error: %s", e.Path, e.Line, e.Column, e.Message))
	}
	if more := info.ErrorCount - len(info.Errors); more > 0 {
		s := "s"
		if more == 1 {
			s = ""
		}
		lines = append(lines, fmt.Sprintf("… and %d more error%s (see the Problems tool window)", more, s))
	}
	lines = append(lines, info.Hints...)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// JoinJavaArgs joins arguments into one string for adapters that take
// `args` / `vmArgs` as text (java-debug).
func JoinJavaArgs(args []string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		switch {
		case a == "":
			parts = append(parts, `""`)
		case strings.ContainsAny(a, " \t\""):
			parts = append(parts, `"`+strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(a)+`"`)
		default:
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

// moduleSourceLayout matches the source layout of a JVM source file:
// the part before src/<source set>/(java|kotlin|scala|groovy)/.
var moduleSourceLayout = regexp.MustCompile(`^(?:(.*)/)?src/(?:main|test|[\w-]+)/(?:java|kotlin|scala|groovy)/`)

// JVMModuleDir is the module directory (relative to the project root, "" for
// the root; absolute for a module outside it) of a JVM source file. The port
// of upstream jvmModuleDir: like IDEA's ProjectFileIndex.getModuleForFile, the
// innermost Maven / Gradle module folder holding the file, as the build model
// knows them (`modules`). Before a model knows a deeper one: the part before
// src/{main,test,…}, else the working directory of the closest Maven / Gradle
// configuration.
func JVMModuleDir(relPath string, configs []openidea.RunConfig, modules []string) string {
	owner := ""
	for _, d := range modules {
		if d != "" && strings.HasPrefix(relPath, d+"/") {
			if len(d) > len(owner) {
				owner = d
			}
		}
	}
	if owner != "" {
		return owner
	}
	if m := moduleSourceLayout.FindStringSubmatch(relPath); m != nil {
		return m[1]
	}
	var cwds []string
	for _, c := range configs {
		if c.Kind != "maven" && c.Kind != "gradle" {
			continue
		}
		if cwd := normalizeRelCwd(c.CwdOr("")); cwd == "" || strings.HasPrefix(relPath, cwd+"/") {
			cwds = append(cwds, cwd)
		}
	}
	if len(cwds) == 0 {
		return ""
	}
	longest := cwds[0]
	for _, cwd := range cwds[1:] {
		if len(cwd) > len(longest) {
			longest = cwd
		}
	}
	return longest
}

// BuildPolicyOf is the build the launch would do: the configuration's policy,
// else incremental ("automatic").
func BuildPolicyOf(config *openidea.RunConfig) string {
	if config != nil && config.BuildPolicy != nil && *config.BuildPolicy != "" {
		return *config.BuildPolicy
	}
	return "automatic"
}

// normalizeRelCwd strips the leading "./" (or ".") and the trailing slashes a
// stored working directory may carry, as upstream jvmModuleDir does with
// /^\.\/?/ and /\/+$/.
func normalizeRelCwd(cwd string) string {
	cwd = strings.TrimPrefix(cwd, "./")
	cwd = strings.TrimPrefix(cwd, ".")
	return strings.TrimRight(cwd, "/")
}
