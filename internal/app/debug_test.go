package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"goidea/internal/openidea"
)

func TestJavaTargetForMavenLayout(t *testing.T) {
	root := "/p"
	text := "package demo;\n\npublic class Hello {\n"
	target, ok := javaTargetFor(root, "/p/src/main/java/demo/Hello.java", text)
	if !ok {
		t.Fatal("main class not found")
	}
	if target.module != "" || target.test {
		t.Fatalf("module = %q, test = %v", target.module, target.test)
	}
	if target.mainClass != "demo.Hello" {
		t.Fatalf("mainClass = %q", target.mainClass)
	}
}

func TestJavaTargetForModuleAndTest(t *testing.T) {
	main, ok := javaTargetFor("/p", "/p/app/src/main/java/demo/App.java", "package demo;\nclass App {}\n")
	if !ok || main.module != "app" || main.test {
		t.Fatalf("main = %+v, ok = %v", main, ok)
	}
	test, ok := javaTargetFor("/p", "/p/app/src/test/java/demo/AppTest.java", "package demo;\nclass AppTest {}\n")
	if !ok || test.module != "app" || !test.test {
		t.Fatalf("test = %+v, ok = %v", test, ok)
	}
}

func TestJavaTargetRejectsNonJava(t *testing.T) {
	if _, ok := javaTargetFor("/p", "/p/README.md", "hello"); ok {
		t.Fatal("a markdown file is not a Java target")
	}
}

func TestJavaPackage(t *testing.T) {
	cases := map[string]string{
		"package demo.app;\n":        "demo.app",
		"// x\npackage a ;\n":        "a",
		"class NoPackage {}":         "",
		"  package   spaced.name;  ": "spaced.name",
	}
	for text, want := range cases {
		if got := javaPackage(text); got != want {
			t.Errorf("javaPackage(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestJavaProjectName(t *testing.T) {
	dir := t.TempDir()
	pom := `<project><modelVersion>4.0.0</modelVersion>
  <parent><groupId>x</groupId><artifactId>parent-id</artifactId></parent>
  <artifactId>child-id</artifactId>
</project>`
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pom), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := javaProjectName(dir); got != "child-id" {
		t.Fatalf("artifactId = %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ".project"), []byte("<projectDescription><name>eclipse-name</name></projectDescription>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := javaProjectName(dir); got != "eclipse-name" {
		t.Fatalf(".project name = %q", got)
	}
	empty := t.TempDir()
	if got := javaProjectName(empty); got != filepath.Base(empty) {
		t.Fatalf("folder name = %q", got)
	}
}

func TestJoinJavaArgs(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"-Xmx1g", "--add-opens", "java.base/java.lang=ALL-UNNAMED"}, "-Xmx1g --add-opens java.base/java.lang=ALL-UNNAMED"},
		{[]string{"a b", "", `c"d`, `e\f`}, `"a b" "" "c\"d" e\f`},
		{[]string{`p"q\r`}, `"p\"q\\r"`},
	}
	for _, c := range cases {
		if got := joinJavaArgs(c.in); got != c.want {
			t.Errorf("joinJavaArgs(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildJavaLaunch(t *testing.T) {
	exec := "/jdk/bin/java"
	info := openidea.JavaLaunchInfo{
		ClassPaths:        []string{"/p/target/classes"},
		ModulePaths:       nil,
		PathListSeparator: ":",
		JavaExec:          &exec,
	}
	target := javaTarget{module: "", mainClass: "demo.Hello"}
	launch, err := buildJavaLaunch("demo.Hello", target, "/p", info, 5005)
	if err != nil {
		t.Fatal(err)
	}
	if launch.Adapter != "java-debug" || launch.Request != "launch" || launch.Cwd != "/p" {
		t.Fatalf("launch = %+v", launch)
	}
	if launch.ExtraJSON == nil {
		t.Fatal("extraJson is empty")
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(*launch.ExtraJSON), &extra); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"debugServer":        float64(5005),
		"mainClass":          "demo.Hello",
		"classPaths":         []any{"/p/target/classes"},
		"modulePaths":        []any{},
		"javaExec":           "/jdk/bin/java",
		"shortenCommandLine": "none",
		"console":            "internalConsole",
		"stopOnEntry":        false,
	}
	for k, v := range want {
		if !reflect.DeepEqual(extra[k], v) {
			t.Errorf("extraJson[%q] = %#v, want %#v", k, extra[k], v)
		}
	}
	if extra["projectName"] != "p" {
		t.Errorf("projectName = %v", extra["projectName"])
	}
}

func TestToggledLines(t *testing.T) {
	lines, found := toggledLines(nil, 4)
	if found || !reflect.DeepEqual(lines, []int{4}) {
		t.Fatalf("add: %v, %v", lines, found)
	}
	lines, found = toggledLines([]int{2, 4, 7}, 4)
	if !found || !reflect.DeepEqual(lines, []int{2, 7}) {
		t.Fatalf("remove: %v, %v", lines, found)
	}
	lines, _ = toggledLines([]int{2, 7}, 4)
	if !reflect.DeepEqual(lines, []int{2, 4, 7}) {
		t.Fatalf("sorted insert: %v", lines)
	}
}

func TestBreakpointLinesCopiesAndSorts(t *testing.T) {
	p := DebugPane{bps: map[string][]int{"/p/A.java": {9, 3, 6}}}
	got := p.breakpointLines("/p/A.java")
	if !reflect.DeepEqual(got, []int{3, 6, 9}) {
		t.Fatalf("lines = %v", got)
	}
	got[0] = 100
	if p.bps["/p/A.java"][0] != 9 {
		t.Fatal("breakpointLines aliases the stored list")
	}
	if lines := p.breakpointLines("/p/Other.java"); lines != nil {
		t.Fatalf("unknown file = %v", lines)
	}
}

func TestAddWatchTrimsAndDedupes(t *testing.T) {
	m := &Model{}
	if !m.addWatch("  count  ") {
		t.Fatalf("watch was rejected")
	}
	if !m.addWatch("count") {
		t.Fatalf("repeat watch was rejected")
	}
	if len(m.debug.watches) != 1 || m.debug.watches[0].Expr != "count" {
		t.Fatalf("watches = %+v", m.debug.watches)
	}
	if m.addWatch("   ") {
		t.Fatalf("blank watch was accepted")
	}
}

func TestAddWatchCapsThePanel(t *testing.T) {
	m := &Model{}
	for i := 0; i < maxWatches; i++ {
		if !m.addWatch(fmt.Sprintf("v%d", i)) {
			t.Fatalf("watch %d was rejected", i)
		}
	}
	if m.addWatch("one-too-many") {
		t.Fatalf("watch past the cap was accepted")
	}
	if m.debug.err == "" {
		t.Fatalf("cap did not explain itself")
	}
}

func TestRemoveWatch(t *testing.T) {
	m := &Model{debug: DebugPane{watches: []debugWatch{{Expr: "a"}, {Expr: "b"}}}}
	m.RemoveWatch(0)
	if len(m.debug.watches) != 1 || m.debug.watches[0].Expr != "b" {
		t.Fatalf("watches = %+v", m.debug.watches)
	}
	m.RemoveWatch(9)
	if len(m.debug.watches) != 1 {
		t.Fatalf("out-of-range remove changed the panel")
	}
}

func TestSubmitWatchNeedsPause(t *testing.T) {
	m := &Model{debug: DebugPane{evalText: "count"}}
	m.SubmitWatch()
	if len(m.debug.watches) != 0 || !strings.Contains(m.debug.err, "暂停") {
		t.Fatalf("watches = %+v, err = %q", m.debug.watches, m.debug.err)
	}
}
