package adapter

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mayoi/internal/openidea"
)

func TestBuildPolicyOptions(t *testing.T) {
	all := []string{"automatic", "make", "rebuild", "noBuild", "makeAndContinueOnErrors"}
	values := func(kind string) []string {
		var out []string
		for _, o := range BuildPolicyOptions(kind) {
			out = append(out, o.Value)
		}
		return out
	}
	if got := values("application"); strings.Join(got, ",") != strings.Join(all, ",") {
		t.Fatalf("application options = %v, want all five", got)
	}
	if got := values("maven"); strings.Join(got, ",") != "automatic,make,rebuild,makeAndContinueOnErrors" {
		t.Fatalf("maven options = %v, want No build dropped", got)
	}
	if got := values("gradle"); strings.Join(got, ",") != "automatic,make,rebuild" {
		t.Fatalf("gradle options = %v, want No build and no-error-check dropped", got)
	}
	for _, kind := range []string{"application", "spring-boot", "maven", "gradle"} {
		if !BuildPolicyApplies(kind) {
			t.Fatalf("%s must show the Build select", kind)
		}
	}
	if BuildPolicyApplies("shell") {
		t.Fatalf("shell must not show the Build select")
	}
}

func TestApplyBuildPolicyToCommand(t *testing.T) {
	rebuild := "rebuild"
	// A Maven rebuild prepends clean.
	mvn := &openidea.RunConfig{Kind: "maven", Args: []string{"package"}, BuildPolicy: &rebuild}
	out := ApplyBuildPolicyToCommand(mvn)
	if strings.Join(out.Args, " ") != "clean package" {
		t.Fatalf("args = %v, want clean prepended", out.Args)
	}
	// A command that already carries clean is left alone.
	mvn.Args = []string{"clean", "package"}
	if got := ApplyBuildPolicyToCommand(mvn); got != mvn {
		t.Fatalf("an existing clean must not be duplicated")
	}
	// Maven's "Build, no error check" adds the compiler flag once.
	cont := "makeAndContinueOnErrors"
	mvn = &openidea.RunConfig{Kind: "maven", Args: []string{"package"}, BuildPolicy: &cont}
	out = ApplyBuildPolicyToCommand(mvn)
	if out.Args[len(out.Args)-1] != "-Dmaven.compiler.failOnError=false" {
		t.Fatalf("args = %v, want the failOnError flag appended", out.Args)
	}
	if got := ApplyBuildPolicyToCommand(out); got != out {
		t.Fatalf("the flag must not be appended twice")
	}
	// Gradle keeps its own command for no-error-check; a Gradle rebuild cleans.
	gradle := &openidea.RunConfig{Kind: "gradle", Args: []string{"build"}, BuildPolicy: &cont}
	if got := ApplyBuildPolicyToCommand(gradle); got != gradle {
		t.Fatalf("a gradle no-error-check must not rewrite the command")
	}
	gradle.BuildPolicy = &rebuild
	if got := ApplyBuildPolicyToCommand(gradle); got.Args[0] != "clean" {
		t.Fatalf("args = %v, want clean prepended", got.Args)
	}
	// No policy: untouched.
	if got := ApplyBuildPolicyToCommand(&openidea.RunConfig{Kind: "maven", Args: []string{"package"}}); got.Args[0] != "package" {
		t.Fatalf("no policy must not rewrite the command")
	}
}

func TestUpsertConfigOverridesDetected(t *testing.T) {
	detected := &openidea.RunConfig{ID: "maven:test", Name: "test", Kind: "maven", Detected: true}
	configs := []*openidea.RunConfig{detected}
	edited := &openidea.RunConfig{ID: detected.ID, Name: "test (edited)", Kind: "maven"}
	out := UpsertConfig(configs, edited)
	if len(out) != 1 || out[0].Detected {
		t.Fatalf("editing a detected config must produce a user copy with its id: %+v", out)
	}
	if out[0].Name != "test (edited)" {
		t.Fatalf("name = %q", out[0].Name)
	}
	added := &openidea.RunConfig{ID: "user:new", Name: "new", Kind: "shell"}
	if out := UpsertConfig(configs, added); len(out) != 2 {
		t.Fatalf("a new config is appended: %d", len(out))
	}
}

func TestValidateConfig(t *testing.T) {
	a := &openidea.RunConfig{ID: "a", Name: "Same", Kind: "maven", Command: "mvn"}
	b := &openidea.RunConfig{ID: "b", Name: "Same", Kind: "maven", Command: "mvn"}
	errs := ValidateConfig(a, []*openidea.RunConfig{a, b})
	if errs["name"] == "" {
		t.Fatalf("a duplicate name must be reported")
	}
	empty := &openidea.RunConfig{ID: "c", Name: "  ", Kind: "shell", Command: ""}
	errs = ValidateConfig(empty, nil)
	if errs["name"] == "" || errs["command"] == "" {
		t.Fatalf("an empty name and command must be reported: %v", errs)
	}
	badEnv := &openidea.RunConfig{ID: "d", Name: "ok", Kind: "shell", Command: "sh",
		Env: map[string]string{"A B": "1"}}
	if errs := ValidateConfig(badEnv, nil); errs["env"] == "" {
		t.Fatalf("an environment name with whitespace must be reported")
	}
	gradle := &openidea.RunConfig{
		ID: "g", Name: "gradle", Kind: "gradle", Command: "./gradlew",
		Params: map[string]any{"wrapper": "true"},
	}
	if errs := ValidateConfig(gradle, nil); errs["tasks"] == "" {
		t.Fatalf("a Gradle configuration without tasks must be reported: %v", errs)
	}
}

func TestBuildToolFormAppliesTasksWorkingDirectoryAndForkedJVMArgs(t *testing.T) {
	maven := &openidea.RunConfig{
		ID: "m", Name: "Spring", Kind: "maven", Command: "mvn",
		Params: map[string]any{"tasks": "spring-boot:run", "wrapper": "false"},
	}
	form := BuildToolForm(maven)
	form["vmOptions"] = `-Xmx1g -Dname="my app"`
	form["activeProfiles"] = "dev,local"
	form["projectDir"] = "service"
	form["arguments"] = "-q"
	edited := ApplyBuildToolForm(maven, form)
	if edited.Cwd == nil || *edited.Cwd != "service" {
		t.Fatalf("working directory = %#v", edited.Cwd)
	}
	want := []string{
		"spring-boot:run", "-q",
		`-Dspring-boot.run.jvmArguments=-Xmx1g "-Dname=my app"`,
		"-Dspring-boot.run.profiles=dev,local",
	}
	if strings.Join(edited.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", edited.Args, want)
	}
	if edited.Params["tasks"] != "spring-boot:run" || edited.Params["activeProfiles"] != "dev,local" {
		t.Fatalf("params = %#v", edited.Params)
	}

	gradle := &openidea.RunConfig{
		ID: "g", Name: "Boot", Kind: "gradle", Command: "./gradlew",
		Params: map[string]any{"tasks": "bootRun", "wrapper": "true", "vmOptions": "-Xmx512m"},
	}
	edited = ApplyBuildToolForm(gradle, BuildToolForm(gradle))
	if strings.Join(edited.Args, " ") != "bootRun" {
		t.Fatalf("Gradle VM options must stay in params for the init-script launch: %v", edited.Args)
	}
	if edited.Params["vmOptions"] != "-Xmx512m" {
		t.Fatalf("Gradle params = %#v", edited.Params)
	}
	payload, err := ConfigObject(edited)
	if err != nil {
		t.Fatal(err)
	}
	savedParams, ok := payload["params"].(map[string]any)
	if !ok || savedParams["vmOptions"] != "-Xmx512m" {
		t.Fatalf("edited build-tool params were not written into the save payload: %#v", payload)
	}
}

func TestGradleForkedJVMArgsFollowTheSelectedTasks(t *testing.T) {
	config := &openidea.RunConfig{
		Kind: "gradle",
		Params: map[string]any{
			"tasks":          "clean :service:bootRun",
			"vmOptions":      `-Xmx512m -Dname="two words"`,
			"activeProfiles": "dev,local",
		},
	}
	want := []string{"-Xmx512m", "-Dname=two words", "-Dspring.profiles.active=dev,local"}
	if got := GradleForkedJVMArgs(config); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Gradle forked JVM args = %#v, want %#v", got, want)
	}
	config.Params["tasks"] = "assemble"
	if got := GradleForkedJVMArgs(config); len(got) != 0 {
		t.Fatalf("a task without a forked JVM received args: %#v", got)
	}
	if !BuildToolShowsVMOptions("maven", "verify") ||
		!BuildToolShowsActiveProfiles("maven", "org.example:spring-boot-maven-plugin:3.3:run") {
		t.Fatal("Maven Spring Boot/test fields were hidden")
	}
}

func TestRunConfigTypesAndNewConfigOfTypeUseTemplates(t *testing.T) {
	plugin := &openidea.RunConfig{ID: "plugin:one", Name: "Plugin", Kind: "kotlin"}
	types := RunConfigTypes([]*openidea.RunConfig{plugin})
	seenPlugin := false
	for _, typ := range types {
		if typ.ID == "kotlin" && typ.Label == "Kotlin Script" {
			seenPlugin = true
		}
	}
	if !seenPlugin {
		t.Fatalf("plugin configuration kind was not added: %+v", types)
	}

	templateRaw, _ := json.Marshal(map[string]any{
		"id": "template:maven", "name": "Maven", "kind": "maven",
		"command": "custom-mvn", "args": []string{"verify"},
		"env":          map[string]string{"MAVEN_OPTS": "-Xmx1g"},
		"beforeLaunch": []map[string]string{{"kind": "build"}},
	})
	template, err := openidea.DecodeRunConfig(templateRaw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := NewConfigOfType("maven", nil, []*openidea.RunConfig{template}, "Services")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Command != "custom-mvn" || strings.Join(cfg.Args, " ") != "verify" ||
		cfg.Env["MAVEN_OPTS"] != "-Xmx1g" || len(cfg.BeforeLaunch) != 1 {
		t.Fatalf("the type template was not inherited: %+v", cfg)
	}
	if cfg.ID == "template:maven" || cfg.Name != "Unnamed" || cfg.Detected ||
		!cfg.Local || cfg.Folder == nil || *cfg.Folder != "Services" {
		t.Fatalf("the new configuration did not receive identity fields: %+v", cfg)
	}
}

func TestCompoundCycleAndFlattenedChildren(t *testing.T) {
	a := &openidea.RunConfig{ID: "a", Name: "A", Kind: "compound", Compound: []string{"b", "c"}}
	b := &openidea.RunConfig{ID: "b", Name: "B", Kind: "compound", Compound: []string{"run1"}}
	c := &openidea.RunConfig{ID: "c", Name: "C", Kind: "compound", Compound: []string{"run2", "run1"}}
	run1 := &openidea.RunConfig{ID: "run1", Name: "One", Kind: "shell", Command: "one"}
	run2 := &openidea.RunConfig{ID: "run2", Name: "Two", Kind: "shell", Command: "two"}
	all := []*openidea.RunConfig{a, b, c, run1, run2}
	children := CompoundChildren(a, all)
	if len(children) != 2 || children[0].ID != "run1" || children[1].ID != "run2" {
		t.Fatalf("nested Compound children = %+v", children)
	}
	if CompoundCycle(a, all) {
		t.Fatal("an acyclic nested Compound was reported as a cycle")
	}
	b.Compound = append(b.Compound, "a")
	if !CompoundCycle(a, all) {
		t.Fatal("the reachable cycle was not detected")
	}
}

func TestValidateBeforeLaunchAndConfigObjectRoundTrip(t *testing.T) {
	self := &openidea.RunConfig{
		ID: "self", Name: "Self", Kind: "shell", Command: "echo",
		BeforeLaunch: []openidea.BeforeLaunchTask{{Kind: "run", Target: "self"}},
	}
	if errs := ValidateConfig(self, []*openidea.RunConfig{self}); errs["beforeLaunch"] == "" {
		t.Fatalf("a before-launch self cycle was accepted: %v", errs)
	}
	missing := &openidea.RunConfig{
		ID: "missing", Name: "Missing", Kind: "shell", Command: "echo",
		BeforeLaunch: []openidea.BeforeLaunchTask{{Kind: "run", Target: "gone"}},
	}
	if errs := ValidateConfig(missing, []*openidea.RunConfig{missing}); errs["beforeLaunch"] == "" {
		t.Fatalf("a missing before-launch target was accepted: %v", errs)
	}

	raw, _ := json.Marshal(map[string]any{
		"id": "app", "name": "App", "kind": "application", "command": "java",
		"args": []string{"App"}, "detected": false,
		"beforeLaunch": []map[string]string{{"kind": "build"}},
		"compound":     []string{"one", "two"},
		"showOnStdout": true, "showOnStderr": true,
		"focusToolWindow": true, "emulateTerminal": true,
		"target": "staging", "extraField": map[string]any{"kept": true},
	})
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := ConfigObject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(obj["beforeLaunch"].([]openidea.BeforeLaunchTask)) != 1 ||
		len(obj["compound"].([]string)) != 2 ||
		obj["showOnStdout"] != true || obj["showOnStderr"] != true ||
		obj["focusToolWindow"] != true || obj["emulateTerminal"] != true ||
		obj["target"] != "staging" {
		t.Fatalf("the run configuration fields were not written back: %v", obj)
	}
	if _, ok := obj["extraField"]; !ok {
		t.Fatal("an unmodelled field was dropped")
	}
}

func TestSaveUserConfigsResolvesTheModuleThenSaves(t *testing.T) {
	// An application configuration without a module is resolved through the
	// backend's indexed search before the save.
	backend := &fakeBackend{
		files: map[string][]openidea.FileMatch{
			"App.java": {{Path: "/p/svc/src/main/java/a/App.java", RelPath: "svc/src/main/java/a/App.java"}},
		},
		texts: map[string]string{"/p/svc/src/main/java/a/App.java": "package a;\npublic class App {}"},
	}
	rawCfg := openidea.RunConfig{
		ID: "maven:test", Name: "test", Kind: "maven", Command: "mvn", Args: []string{"package"},
	}
	base, err := json.Marshal(rawCfg)
	if err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{}
	if err := json.Unmarshal(base, &obj); err != nil {
		t.Fatal(err)
	}
	// A field GoIDEA does not model survives the round trip.
	obj["extraField"] = "kept"
	raw, _ := json.Marshal(obj)
	edited, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	app := openidea.RunConfig{
		ID: "user:app", Name: "App", Kind: "application", Command: "java",
		Args: []string{"-cp", "${classpath}", "a.App"},
	}
	appRaw, _ := json.Marshal(app)
	appCfg, _ := openidea.DecodeRunConfig(appRaw)
	if err := SaveUserConfigs(context.Background(), backend, []*openidea.RunConfig{edited, appCfg}, nil); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if len(backend.saved) != 1 {
		t.Fatalf("one run_save_configs call expected, got %d", len(backend.saved))
	}
	saved := backend.saved[0].configs
	if len(saved) != 2 {
		t.Fatalf("two configurations expected, got %d", len(saved))
	}
	if saved[0]["detected"] != false {
		t.Fatalf("saved configurations are user configs")
	}
	if saved[0]["extraField"] != "kept" {
		t.Fatalf("a field GoIDEA does not model must survive: %v", saved[0]["extraField"])
	}
	if saved[1]["module"] != "svc" {
		t.Fatalf("the JVM application's module must be resolved at save: %v", saved[1]["module"])
	}
}

func TestParseArgsFormatArgsRoundTrip(t *testing.T) {
	cases := [][]string{
		{"clean", "package"},
		{"-Dtest=App#method", "-Dsurefire.failIfNoSpecifiedTests=false"},
		{"two words", "", `he said 'hi'`, `back\slash`, "$PATH", "a`b"},
		{`"quoted"`},
	}
	for _, args := range cases {
		got := ParseArgs(FormatArgs(args))
		if strings.Join(got, "\x00") != strings.Join(args, "\x00") {
			t.Fatalf("round trip of %v gave %v", args, got)
		}
	}
	if got := ParseArgs(`mvn -q -Dx="a b" c`); strings.Join(got, "\x00") != "mvn\x00-q\x00-Dx=a b\x00c" {
		t.Fatalf("ParseArgs = %v", got)
	}
}

func TestFormatEnvParseEnv(t *testing.T) {
	env := map[string]string{"B": "2", "A": "1"}
	text := FormatEnv(env)
	if text != "A=1\nB=2" {
		t.Fatalf("FormatEnv = %q, want a stable sorted list", text)
	}
	got := ParseEnv("A=1\nB=2\nNOEQUALS\n")
	if got["A"] != "1" || got["B"] != "2" || len(got) != 2 {
		t.Fatalf("ParseEnv = %v", got)
	}
}

func TestDockerRunConfigTypesAndDefaults(t *testing.T) {
	want := map[string]string{"docker-dockerfile": "Dockerfile", "docker-image": "Docker Image", "docker-compose": "Docker Compose"}
	for _, typ := range RunConfigTypes(nil) {
		if label, ok := want[typ.ID]; ok {
			if typ.Label != label || typ.Group != "Docker" {
				t.Fatalf("unexpected Docker type: %#v", typ)
			}
			delete(want, typ.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing Docker types: %v", want)
	}
	for kind, key := range map[string]string{"docker-dockerfile": "dockerfile", "docker-compose": "files", "docker-image": "connection"} {
		cfg, err := BlankTemplate(kind)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Params[key] == nil {
			t.Fatalf("missing %s in %#v", key, cfg.Params)
		}
	}
}
