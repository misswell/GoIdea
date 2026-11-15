package adapter

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func TestParseJavaArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		main      string
		vmArgs    []string
		program   []string
		classpath string
		explicit  string
		jar       string
	}{
		{
			name:      "runtime classpath placeholder",
			args:      []string{"-cp", "${classpath}", "demo.App"},
			main:      "demo.App",
			classpath: "runtime",
		},
		{
			name:      "test classpath placeholder",
			args:      []string{"-cp", "${testClasspath}", "demo.AppTest"},
			main:      "demo.AppTest",
			classpath: "test",
		},
		{
			name:     "vm options with value-taking flags, program args",
			args:     []string{"-Xmx1g", "--add-opens", "java.base/java.lang=ALL-UNNAMED", "a.Main", "--port", "1"},
			main:     "a.Main",
			vmArgs:   []string{"-Xmx1g", "--add-opens", "java.base/java.lang=ALL-UNNAMED"},
			program:  []string{"--port", "1"},
			explicit: "",
		},
		{
			name:     "explicit classpath kept apart from placeholders",
			args:     []string{"-classpath", "out:lib/x.jar", "a.Main", "run"},
			main:     "a.Main",
			explicit: "out:lib/x.jar",
			program:  []string{"run"},
		},
		{
			name:    "jar launch has no main class",
			args:    []string{"-jar", "app.jar", "serve"},
			jar:     "app.jar",
			program: []string{"serve"},
		},
		{
			name:     "no args",
			args:     nil,
			explicit: "",
		},
		{
			name:   "module path flag consumes its value",
			args:   []string{"-p", "mods", "a.Main"},
			main:   "a.Main",
			vmArgs: []string{"-p", "mods"},
		},
		{
			name:   "trailing value-taking flag keeps an empty value",
			args:   []string{"--add-modules"},
			vmArgs: []string{"--add-modules", ""},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseJavaArgs(c.args)
			if got.MainClass != c.main {
				t.Fatalf("MainClass = %q, want %q", got.MainClass, c.main)
			}
			if got.Classpath != c.classpath {
				t.Fatalf("Classpath = %q, want %q", got.Classpath, c.classpath)
			}
			if got.ExplicitClasspath != c.explicit {
				t.Fatalf("ExplicitClasspath = %q, want %q", got.ExplicitClasspath, c.explicit)
			}
			if got.JarPath != c.jar {
				t.Fatalf("JarPath = %q, want %q", got.JarPath, c.jar)
			}
			if strings.Join(got.VMArgs, "\x00") != strings.Join(c.vmArgs, "\x00") {
				t.Fatalf("VMArgs = %v, want %v", got.VMArgs, c.vmArgs)
			}
			if strings.Join(got.ProgramArgs, "\x00") != strings.Join(c.program, "\x00") {
				t.Fatalf("ProgramArgs = %v, want %v", got.ProgramArgs, c.program)
			}
		})
	}
}

func TestConsoleVMArgs(t *testing.T) {
	if got := ConsoleVMArgs("spring-boot", []string{"-cp", "${classpath}", "a.App"}); len(got) != 1 || got[0] != "-Dspring.output.ansi.enabled=always" {
		t.Fatalf("spring-boot launch must add the ANSI property, got %v", got)
	}
	// The configuration sets the property itself: kept out.
	if got := ConsoleVMArgs("spring-boot", []string{"-Dspring.output.ansi.enabled=never", "-cp", "${classpath}", "a.App"}); got != nil {
		t.Fatalf("own ANSI property must suppress the default, got %v", got)
	}
	// Only the VM options decide: the same text in a program argument does not.
	if got := ConsoleVMArgs("spring-boot", []string{"-cp", "${classpath}", "a.App", "-Dspring.output.ansi.enabled=never"}); got == nil {
		t.Fatalf("a program argument must not suppress the console property")
	}
	if got := ConsoleVMArgs("application", []string{"-cp", "${classpath}", "a.App"}); got != nil {
		t.Fatalf("plain applications get no console options, got %v", got)
	}
}

func TestResolveJavaCommand(t *testing.T) {
	javaExec := "/Library/Java/bin/java"
	info := &openidea.JavaLaunchInfo{
		ClassPaths:        []string{"/p/app/target/classes", "/p/lib/dep.jar"},
		ModulePaths:       []string{"/p/mod/target/classes"},
		PathListSeparator: ":",
		JavaExec:          &javaExec,
	}
	command, args := ResolveJavaCommand("application", "java", []string{"-Xmx1g", "-cp", "${classpath}", "a.Main", "serve"}, info)
	if command != javaExec {
		t.Fatalf("command = %q, want the resolved javaExec", command)
	}
	want := []string{"--module-path", "/p/mod/target/classes", "-Xmx1g", "-cp", "/p/app/target/classes:/p/lib/dep.jar", "a.Main", "serve"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %v, want %v", args, want)
	}

	// A configuration that sets its own module path keeps it.
	_, args = ResolveJavaCommand("application", "java", []string{"--module-path", "own", "a.Main"}, info)
	if args[0] != "--module-path" || args[1] != "own" {
		t.Fatalf("own module path must be kept, got %v", args)
	}

	// Spring Boot gets the console property first, like IDEA's configurations.
	command, args = ResolveJavaCommand("spring-boot", "java", []string{"-cp", "${testClasspath}", "a.App"}, &openidea.JavaLaunchInfo{
		ClassPaths: []string{"/p/test"}, PathListSeparator: ":",
	})
	if command != "java" {
		t.Fatalf("command = %q, want java when no exec is resolved", command)
	}
	want = []string{"-Dspring.output.ansi.enabled=always", "-cp", "/p/test", "a.App"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func TestFormatCompileErrors(t *testing.T) {
	if got := FormatCompileErrors(&openidea.JavaLaunchInfo{}); got != "" {
		t.Fatalf("no errors must format to \"\", got %q", got)
	}
	info := &openidea.JavaLaunchInfo{
		Errors: []openidea.JavaCompileError{
			{Path: "app/src/main/java/a/App.java", Line: 3, Column: 8, Message: "cannot find symbol"},
		},
		ErrorCount: 3,
		Hints:      []string{"Maven 模型未同步，先刷新项目"},
	}
	got := FormatCompileErrors(info)
	want := "app/src/main/java/a/App.java:3:8: error: cannot find symbol\n" +
		"… and 2 more errors (see the Problems tool window)\n" +
		"Maven 模型未同步，先刷新项目\n"
	if got != want {
		t.Fatalf("FormatCompileErrors =\n%q\nwant\n%q", got, want)
	}
}

func TestJoinJavaArgs(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"-Xmx1g", "a.Main"}, "-Xmx1g a.Main"},
		{[]string{"", "a"}, `"" a`},
		{[]string{"two words", `say "hi"`, `back\slash`}, `"two words" "say \"hi\"" back\slash`},
	}
	for _, c := range cases {
		if got := JoinJavaArgs(c.in); got != c.want {
			t.Fatalf("JoinJavaArgs(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestJVMModuleDir(t *testing.T) {
	// The source layout decides when no build model is known.
	if got := JVMModuleDir("src/main/java/a/App.java", nil, nil); got != "" {
		t.Fatalf("root module = %q, want \"\"", got)
	}
	if got := JVMModuleDir("services/api/src/test/kotlin/a/T.kt", nil, nil); got != "services/api" {
		t.Fatalf("test kotlin module = %q, want services/api", got)
	}
	maven := []openidea.RunConfig{{Kind: "maven", Cwd: strPtr("odd")}}
	if got := JVMModuleDir("odd/App.java", maven, nil); got != "odd" {
		t.Fatalf("maven cwd fallback = %q, want odd", got)
	}

	// The innermost module the build model knows wins, like IDEA's file index.
	modules := []string{"", "/w/billing", "/w/billing/modules/core", "legacy"}
	// `sourceSets.main.java.srcDirs = ['src']`: no src/main/java to guess from.
	if got := JVMModuleDir("legacy/src/a/App.java", nil, modules); got != "legacy" {
		t.Fatalf("known module = %q, want legacy", got)
	}
	if got := JVMModuleDir("/w/billing/modules/core/src/main/java/a/App.java", nil, modules); got != "/w/billing/modules/core" {
		t.Fatalf("innermost module = %q, want /w/billing/modules/core", got)
	}
	// A module the model does not know yet (before the first sync): layout.
	if got := JVMModuleDir("services/api/src/main/java/a/App.java", nil, modules); got != "services/api" {
		t.Fatalf("layout module = %q, want services/api", got)
	}
}

func TestBuildPolicyOf(t *testing.T) {
	if got := BuildPolicyOf(&openidea.RunConfig{}); got != "automatic" {
		t.Fatalf("default policy = %q, want automatic", got)
	}
	if got := BuildPolicyOf(&openidea.RunConfig{BuildPolicy: strPtr("makeAndContinueOnErrors")}); got != "makeAndContinueOnErrors" {
		t.Fatalf("policy = %q, want makeAndContinueOnErrors", got)
	}
}

func strPtr(s string) *string { return &s }
