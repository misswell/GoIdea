package adapter

// The port of OpenIDEA's run configuration helpers (src/features/run/configs.ts,
// buildCommand.ts and the save flow of store.ts saveUserConfigs): grouping,
// overriding, validation and the build policy of a launch. The UI package
// drives these; the backend surface (run_save_configs) stays in Backend.

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"mayoi/internal/openidea"
)

// PolicyOption is one entry of the Build select.
type PolicyOption struct {
	Value string
	Label string
}

// buildPolicyOptions is OpenIDEA's Build select for a launch (ConfigForm.tsx
// BUILD_POLICY_OPTIONS).
var buildPolicyOptions = []PolicyOption{
	{"automatic", "Automatic (incremental)"},
	{"make", "Build (Make)"},
	{"rebuild", "Rebuild"},
	{"noBuild", "No build"},
	{"makeAndContinueOnErrors", "Build, no error check"},
}

// BuildPolicyApplies reports whether a configuration's kind shows the Build
// select: JVM applications build through PrepareJavaLaunch; Maven and Gradle
// rewrite their own command.
func BuildPolicyApplies(kind string) bool {
	return IsJVMApplication(kind) || kind == "maven" || kind == "gradle"
}

// BuildPolicyOptions is the Build select of a configuration, with the options
// a kind does not support filtered out (ConfigForm.tsx buildPolicyOptions):
// Gradle always delegates the build to itself, so it drops No build and
// Build-no-error-check; Maven only drops No build.
func BuildPolicyOptions(kind string) []PolicyOption {
	out := make([]PolicyOption, 0, len(buildPolicyOptions))
	for _, o := range buildPolicyOptions {
		if kind == "gradle" {
			if o.Value == "noBuild" || o.Value == "makeAndContinueOnErrors" {
				continue
			}
		} else if kind == "maven" && o.Value == "noBuild" {
			continue
		}
		out = append(out, o)
	}
	return out
}

// ApplyBuildPolicyToCommand ports buildCommand.ts: the build policies of
// configurations whose command is itself a build tool (Maven, Gradle).
// Application / Spring Boot configurations build through PrepareJavaLaunch
// instead; this only rewrites the command of an already chosen build-tool
// launch.
func ApplyBuildPolicyToCommand(config *openidea.RunConfig) *openidea.RunConfig {
	if config == nil {
		return config
	}
	changed := false
	out := *config
	if config.BuildPolicy != nil && *config.BuildPolicy == "rebuild" &&
		!hasArg(config.Args, "clean") && (config.Kind == "maven" || config.Kind == "gradle") {
		// Maven and Gradle both accept the clean task before their other goals.
		out.Args = append([]string{"clean"}, config.Args...)
		changed = true
	}
	if config.BuildPolicy != nil && *config.BuildPolicy == "makeAndContinueOnErrors" && config.Kind == "maven" {
		const flag = "-Dmaven.compiler.failOnError=false"
		if !hasArg(config.Args, flag) {
			out.Args = append(append([]string(nil), config.Args...), flag)
			changed = true
		}
	}
	if !changed {
		return config
	}
	return &out
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// kindLabels is upstream configs.ts's KIND_LABELS: the display name of a run
// configuration kind.
var kindLabels = map[string]string{
	"npm": "npm", "deno": "Deno", "bun": "Bun", "cargo": "Cargo",
	"gradle": "Gradle", "maven": "Maven", "go": "Go Build", "go-test": "Go Test", "godot": "Godot",
	"python": "Python",
	"pytest": "pytest", "phpunit": "PHPUnit", "composer": "Composer",
	"make": "Make", "cmake": "CMake", "just": "Just", "shell": "Shell Script",
	"program": "Program", "compound": "Compound", "application": "Application",
	"jar-application": "JAR Application", "spring-boot": "Spring Boot",
	"node": "Node.js", "java-file": "Java Single File", "rustc": "Rust Single File",
	"kotlinc": "Kotlin Single File", "cc": "C/C++ Single File",
	"cmake-application": "CMake Application", "native": "Native Executable",
	"ruby": "Ruby", "php": "PHP", "perl": "Perl", "lua": "Lua", "r": "R",
	"swift": "Swift", "dart": "Dart", "elixir": "Elixir", "dotnet": ".NET",
	"dotnet-test": ".NET Test", "tomcat": "Tomcat",
	"scala-cli": "Scala CLI", "kotlin": "Kotlin Script",
	"docker-dockerfile": "Dockerfile", "docker-image": "Docker Image", "docker-compose": "Docker Compose",
}

// KindLabel is the display name of a run configuration kind; an unknown kind
// gets its capitalised name, as upstream kindLabel does.
func KindLabel(kind string) string {
	if label := kindLabels[kind]; label != "" {
		return label
	}
	if kind == "" {
		return "Other"
	}
	return strings.ToUpper(kind[:1]) + kind[1:]
}

// RunConfigType is one row of OpenIDEA's Add New Configuration menu.
type RunConfigType struct {
	ID    string
	Label string
	Group string
}

var runConfigTypes = []RunConfigType{
	{ID: "docker-dockerfile", Label: "Dockerfile", Group: "Docker"},
	{ID: "docker-image", Label: "Docker Image", Group: "Docker"},
	{ID: "docker-compose", Label: "Docker Compose", Group: "Docker"},
	{ID: "shell", Label: "Shell Script", Group: "Shell Scripts"},
	{ID: "program", Label: "Program", Group: "Other"},
	{ID: "npm", Label: "npm", Group: "JavaScript and TypeScript"},
	{ID: "deno", Label: "Deno", Group: "JavaScript and TypeScript"},
	{ID: "bun", Label: "Bun", Group: "JavaScript and TypeScript"},
	{ID: "node", Label: "Node.js", Group: "JavaScript and TypeScript"},
	{ID: "gradle", Label: "Gradle", Group: "Build Tools"},
	{ID: "maven", Label: "Maven", Group: "Build Tools"},
	{ID: "go", Label: "Go Build", Group: "Go"},
	{ID: "go-test", Label: "Go Test", Group: "Go"},
	{ID: "python", Label: "Python", Group: "Python"},
	{ID: "pytest", Label: "pytest", Group: "Python"},
	{ID: "django-test", Label: "Django Test", Group: "Python"},
	{ID: "ruby", Label: "Ruby", Group: "Ruby"},
	{ID: "rake", Label: "Rake", Group: "Ruby"},
	{ID: "rspec", Label: "RSpec", Group: "Ruby"},
	{ID: "phpunit", Label: "PHPUnit", Group: "PHP"},
	{ID: "php", Label: "PHP", Group: "PHP"},
	{ID: "dotnet", Label: ".NET", Group: ".NET"},
	{ID: "dotnet-test", Label: ".NET Test", Group: ".NET"},
	{ID: "application", Label: "Application", Group: "Application"},
	{ID: "jar-application", Label: "JAR Application", Group: "Application"},
	{ID: "spring-boot", Label: "Spring Boot", Group: "Application"},
	{ID: "tomcat", Label: "Tomcat", Group: "Application"},
	{ID: "godot", Label: "Godot", Group: "Other"},
	{ID: "compound", Label: "Compound", Group: "Other"},
}

// RunConfigTypes returns the built-in Add New Configuration choices and any
// plugin kinds present in the project.
func RunConfigTypes(configs []*openidea.RunConfig) []RunConfigType {
	out := append([]RunConfigType(nil), runConfigTypes...)
	known := make(map[string]bool, len(out))
	for _, typ := range out {
		known[typ.ID] = true
	}
	for _, c := range configs {
		if c == nil || known[c.Kind] || c.Kind == "" {
			continue
		}
		known[c.Kind] = true
		out = append(out, RunConfigType{ID: c.Kind, Label: KindLabel(c.Kind), Group: "Other"})
	}
	return out
}

// TemplateID is the stable id of a type's configuration template.
func TemplateID(kind string) string { return "template:" + kind }

// DefaultCommand is the command a blank built-in type starts with.
func DefaultCommand(kind string) string {
	switch kind {
	case "gradle":
		if runtime.GOOS == "windows" {
			return "gradlew.bat"
		}
		return "./gradlew"
	case "maven":
		return "mvn"
	case "npm":
		return "npm"
	case "deno":
		return "deno"
	case "bun":
		return "bun"
	case "node":
		return "node"
	case "go", "go-test":
		return "go"
	case "godot":
		return "godot"
	case "python", "django-test":
		return "python"
	case "pytest":
		return "pytest"
	case "ruby":
		return "ruby"
	case "rake", "rspec":
		return "bundle"
	case "phpunit":
		return "phpunit"
	case "php":
		return "php"
	case "dotnet", "dotnet-test":
		return "dotnet"
	case "application", "spring-boot", "jar-application":
		return "java"
	default:
		return ""
	}
}

// BlankTemplate is the base for a type when the project has no saved template.
func BlankTemplate(kind string) (*openidea.RunConfig, error) {
	obj := map[string]any{
		"id": TemplateID(kind), "name": KindLabel(kind), "kind": kind,
		"command": DefaultCommand(kind), "args": []string{},
		"cwd": nil, "env": map[string]string{}, "detected": false,
		"isTest": strings.HasSuffix(kind, "-test") || kind == "pytest" ||
			kind == "phpunit" || kind == "rspec",
		"debugAdapter": nil,
	}
	switch kind {
	case "gradle":
		obj["params"] = map[string]string{"wrapper": "true", "tasks": ""}
	case "maven":
		obj["params"] = map[string]string{"wrapper": "false", "tasks": ""}
	case "shell":
		obj["params"] = map[string]string{"mode": "command"}
	case "program":
		obj["params"] = map[string]string{"program": "", "arguments": ""}
	case "npm":
		obj["params"] = map[string]string{"packageManager": "npm", "command": "run"}
	case "bun":
		obj["params"] = map[string]string{"packageManager": "bun", "command": "run"}
	case "pytest":
		obj["params"] = map[string]string{"command": "pytest"}
	case "godot":
		obj["params"] = map[string]string{"projectDir": "", "scene": "", "arguments": ""}
	case "go":
		obj["params"] = map[string]string{
			"goKind": "build", "scope": "package", "target": ".",
			"runAfterBuild": "true", "useTags": "true",
		}
	case "go-test":
		obj["params"] = map[string]string{
			"goKind": "test", "scope": "package", "target": ".", "useTags": "true",
		}
	case "docker-dockerfile":
		obj["params"] = map[string]string{"connection": "default", "dockerfile": "Dockerfile", "runImage": "true"}
	case "docker-image":
		obj["params"] = map[string]string{"connection": "default"}
	case "docker-compose":
		obj["params"] = map[string]string{"connection": "default", "files": "docker-compose.yml"}
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return openidea.DecodeRunConfig(raw)
}

// BuildToolForm returns the editable Maven/Gradle values OpenIDEA derives
// its command line from. Configurations written before params existed are
// represented from their current command line, so the form stays usable.
func BuildToolForm(config *openidea.RunConfig) map[string]string {
	if config == nil || (config.Kind != "maven" && config.Kind != "gradle") {
		return nil
	}
	out := map[string]string{}
	for key, value := range config.Params {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	if out["tasks"] == "" {
		out["tasks"] = FormatArgs(config.Args)
	}
	if _, ok := out["projectDir"]; !ok {
		out["projectDir"] = config.CwdOr("")
	}
	if _, ok := out["executable"]; !ok {
		out["executable"] = config.Command
	}
	if _, ok := out["wrapper"]; !ok {
		out["wrapper"] = fmt.Sprintf("%t", buildToolUsesWrapper(config.Kind, config.Command))
	}
	return out
}

// ApplyBuildToolForm writes Maven/Gradle params and the command line they
// produce, matching OpenIDEA's withParams for buildToolType.
func ApplyBuildToolForm(config *openidea.RunConfig, form map[string]string) *openidea.RunConfig {
	if config == nil || (config.Kind != "maven" && config.Kind != "gradle") {
		return config
	}
	next := *config
	params := map[string]string{}
	for key, value := range form {
		if value = strings.TrimSpace(value); value != "" {
			params[key] = value
		}
	}
	tasks := ParseArgs(params["tasks"])
	args := append([]string(nil), tasks...)
	args = append(args, ParseArgs(params["arguments"])...)
	if config.Kind == "maven" {
		args = append(args, mavenForkedJVMArgs(tasks, params)...)
	}
	next.Args = args
	next.Command = buildToolCommand(config.Kind, params)
	if projectDir := strings.TrimSpace(params["projectDir"]); projectDir == "" {
		next.Cwd = nil
	} else {
		next.Cwd = &projectDir
	}
	if len(params) == 0 {
		next.Params = nil
	} else {
		next.Params = make(map[string]any, len(params))
		for key, value := range params {
			next.Params[key] = value
		}
	}
	return &next
}

// MissingBuildToolParams reports required type-form values the editor must
// show before this Maven/Gradle configuration can be saved.
func MissingBuildToolParams(config *openidea.RunConfig) ConfigErrors {
	if config == nil || (config.Kind != "maven" && config.Kind != "gradle") || config.Params == nil {
		return ConfigErrors{}
	}
	form := BuildToolForm(config)
	if strings.TrimSpace(form["tasks"]) == "" {
		label := "Goals"
		if config.Kind == "gradle" {
			label = "Tasks"
		}
		return ConfigErrors{"tasks": label + " cannot be empty"}
	}
	return ConfigErrors{}
}

// BuildToolShowsVMOptions reports whether the selected Maven or Gradle tasks
// start another JVM, as OpenIDEA's conditional VM options field does.
func BuildToolShowsVMOptions(kind, tasks string) bool {
	for _, task := range strings.Fields(tasks) {
		switch kind {
		case "maven":
			if isMavenSpringBootRun(task) || isMavenTestGoal(task) {
				return true
			}
		case "gradle":
			if isGradleJVMTask(task) {
				return true
			}
		}
	}
	return false
}

// BuildToolShowsActiveProfiles reports whether a Spring Boot run task can
// consume the Active profiles field.
func BuildToolShowsActiveProfiles(kind, tasks string) bool {
	for _, task := range strings.Fields(tasks) {
		switch kind {
		case "maven":
			if isMavenSpringBootRun(task) {
				return true
			}
		case "gradle":
			if isGradleBootRun(task) {
				return true
			}
		}
	}
	return false
}

// GradleForkedJVMArgs returns the VM options and Spring Boot profile argument
// OpenIDEA's Gradle init script must apply to bootRun, run, test, check or
// integrationTest tasks. Gradle's own process does not receive them.
func GradleForkedJVMArgs(config *openidea.RunConfig) []string {
	if config == nil || config.Kind != "gradle" {
		return nil
	}
	tasks := paramString(config.Params, "tasks")
	if !BuildToolShowsVMOptions("gradle", tasks) {
		return nil
	}
	out := ParseArgs(paramString(config.Params, "vmOptions"))
	if BuildToolShowsActiveProfiles("gradle", tasks) {
		if profiles := strings.TrimSpace(paramString(config.Params, "activeProfiles")); profiles != "" {
			out = append(out, "-Dspring.profiles.active="+profiles)
		}
	}
	return out
}

func paramString(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	value, _ := params[key].(string)
	return value
}

func isMavenSpringBootRun(goal string) bool {
	if goal == "spring-boot:run" {
		return true
	}
	const marker = ":spring-boot-maven-plugin:"
	index := strings.Index(goal, marker)
	if index < 0 {
		return false
	}
	version := strings.TrimSuffix(goal[index+len(marker):], ":run")
	return version != goal[index+len(marker):] && !strings.Contains(version, ":")
}

func isMavenTestGoal(goal string) bool {
	switch goal {
	case "test", "verify", "integration-test", "surefire:test", "failsafe:integration-test":
		return true
	default:
		return false
	}
}

func isGradleJVMTask(task string) bool {
	switch {
	case task == "bootRun", strings.HasSuffix(task, ":bootRun"):
		return true
	case task == "test", strings.HasSuffix(task, ":test"):
		return true
	case task == "run", strings.HasSuffix(task, ":run"):
		return true
	case task == "integrationTest", strings.HasSuffix(task, ":integrationTest"):
		return true
	case task == "check", strings.HasSuffix(task, ":check"):
		return true
	default:
		return false
	}
}

func isGradleBootRun(task string) bool {
	return task == "bootRun" || strings.HasSuffix(task, ":bootRun")
}

func buildToolCommand(kind string, params map[string]string) string {
	wrapper := params["wrapper"] == "true"
	wrapperName := "./mvnw"
	plainName := "mvn"
	if kind == "gradle" {
		wrapperName = "./gradlew"
		plainName = "gradle"
	}
	if runtime.GOOS == "windows" {
		if kind == "gradle" {
			wrapperName = "gradlew.bat"
		} else {
			wrapperName = "mvnw.cmd"
		}
	}
	expected := plainName
	if wrapper {
		expected = commandName(wrapperName)
	}
	executable := strings.TrimSpace(params["executable"])
	if executable != "" && commandName(executable) == expected {
		return executable
	}
	if wrapper {
		return wrapperName
	}
	return plainName
}

func buildToolUsesWrapper(kind, command string) bool {
	name := commandName(command)
	if kind == "gradle" {
		return name == "gradlew"
	}
	return name == "mvnw"
}

// commandName is OpenIDEA commandName: the executable identity ignores a
// Windows extension, while a case-preserving path still has to match.
func commandName(command string) string {
	name := strings.TrimSpace(command)
	name = strings.ReplaceAll(name, `\`, "/")
	if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
		name = name[slash+1:]
	}
	lower := strings.ToLower(name)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".com"} {
		if strings.HasSuffix(lower, ext) {
			return strings.TrimSuffix(lower, ext)
		}
	}
	return name
}

func mavenForkedJVMArgs(tasks []string, params map[string]string) []string {
	vmOptions := ParseArgs(params["vmOptions"])
	profiles := strings.TrimSpace(params["activeProfiles"])
	springBootRun := false
	mavenTest := false
	for _, task := range tasks {
		if isMavenSpringBootRun(task) {
			springBootRun = true
		}
		if isMavenTestGoal(task) {
			mavenTest = true
		}
	}
	var out []string
	if springBootRun {
		if len(vmOptions) > 0 {
			out = append(out, "-Dspring-boot.run.jvmArguments="+joinPropertyArgs(vmOptions))
		}
		if profiles != "" {
			out = append(out, "-Dspring-boot.run.profiles="+profiles)
		}
	} else if mavenTest && len(vmOptions) > 0 {
		out = append(out, "-DargLine="+joinPropertyArgs(vmOptions))
	}
	return out
}

func joinPropertyArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		switch {
		case arg == "":
			quoted = append(quoted, `""`)
		case strings.ContainsAny(arg, " \t\r\n\"'"):
			if strings.Contains(arg, `"`) {
				quoted = append(quoted, "'"+arg+"'")
			} else {
				quoted = append(quoted, `"`+arg+`"`)
			}
		default:
			quoted = append(quoted, arg)
		}
	}
	return strings.Join(quoted, " ")
}

// NewConfigOfType creates a configuration from the type template and the
// selected folder, as upstream newConfigOfType does.
func NewConfigOfType(
	kind string,
	configs []*openidea.RunConfig,
	templates []*openidea.RunConfig,
	folder string,
) (*openidea.RunConfig, error) {
	base, err := BlankTemplate(kind)
	if err != nil {
		return nil, err
	}
	if template := templateFor(kind, templates); template != nil {
		base = cloneConfig(template)
	}
	obj := map[string]any{}
	raw := base.Raw()
	if raw == nil {
		raw, err = json.Marshal(base)
		if err != nil {
			return nil, err
		}
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	obj["id"] = UID()
	obj["name"] = UniqueName("Unnamed", configs)
	obj["kind"] = kind
	obj["detected"] = false
	obj["local"] = true
	if folder == "" {
		delete(obj, "folder")
	} else {
		obj["folder"] = folder
	}
	raw, err = json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return openidea.DecodeRunConfig(raw)
}

func templateFor(kind string, templates []*openidea.RunConfig) *openidea.RunConfig {
	for _, template := range templates {
		if template != nil && template.Kind == kind {
			return template
		}
	}
	return nil
}

func cloneConfig(config *openidea.RunConfig) *openidea.RunConfig {
	if config == nil {
		return nil
	}
	raw := config.Raw()
	if raw == nil {
		raw, _ = json.Marshal(config)
	}
	out, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		cp := *config
		return &cp
	}
	return out
}

// UpsertConfig applies an edit the way upstream configs.ts does: editing a
// detected config produces a user copy with the same id, which then overrides
// the detected one (`run_configs` drops detected entries whose id a user
// config shares).
func UpsertConfig(configs []*openidea.RunConfig, edited *openidea.RunConfig) []*openidea.RunConfig {
	next := *edited
	next.Detected = false
	for i, c := range configs {
		if c.ID == edited.ID {
			out := append([]*openidea.RunConfig(nil), configs...)
			out[i] = &next
			return out
		}
	}
	return append(configs, &next)
}

// RemoveConfig drops a configuration from the working list.
func RemoveConfig(configs []*openidea.RunConfig, id string) []*openidea.RunConfig {
	out := configs[:0:0]
	for _, c := range configs {
		if c.ID != id {
			out = append(out, c)
		}
	}
	return out
}

// UserConfigs are the configs persisted in `.openidea/run.toml` (everything
// not regenerated by detection).
func UserConfigs(configs []*openidea.RunConfig) []*openidea.RunConfig {
	var out []*openidea.RunConfig
	for _, c := range configs {
		if !c.Detected {
			out = append(out, c)
		}
	}
	return out
}

// UniqueName returns a configuration name no other configuration has.
func UniqueName(base string, configs []*openidea.RunConfig) string {
	names := map[string]bool{}
	for _, c := range configs {
		names[c.Name] = true
	}
	if !names[base] {
		return base
	}
	for i := 2; ; i++ {
		n := fmt.Sprintf("%s (%d)", base, i)
		if !names[n] {
			return n
		}
	}
}

var uidSeq int

// UID is upstream uid(): a fresh `user:` id, per session unique.
func UID() string {
	var rnd [2]byte
	if _, err := crand.Read(rnd[:]); err != nil {
		rnd[0], rnd[1] = byte(uidSeq>>8), byte(uidSeq)
	}
	uidSeq++
	return fmt.Sprintf("user:%02x%02x%d", rnd[0], rnd[1], uidSeq)
}

// NewConfig is upstream newConfig: an empty Shell configuration.
func NewConfig(configs []*openidea.RunConfig) (*openidea.RunConfig, error) {
	obj := map[string]any{
		"id": UID(), "name": UniqueName("Unnamed", configs), "kind": "shell",
		"command": "", "args": []string{}, "env": map[string]string{},
		"detected": false, "isTest": false,
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return openidea.DecodeRunConfig(raw)
}

// DuplicateConfig is upstream duplicateConfig: a fresh id, a unique
// `(copy)` name, and every field of the original — the payload starts from
// its raw object so fields GoIDEA does not model survive.
func DuplicateConfig(config *openidea.RunConfig, configs []*openidea.RunConfig) (*openidea.RunConfig, error) {
	src := config.Raw()
	if src == nil {
		var err error
		src, err = json.Marshal(config)
		if err != nil {
			return nil, err
		}
	}
	obj := map[string]any{}
	if err := json.Unmarshal(src, &obj); err != nil {
		return nil, err
	}
	obj["id"] = UID()
	obj["name"] = UniqueName(config.Name+" (copy)", configs)
	obj["detected"] = false
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return openidea.DecodeRunConfig(raw)
}

// ConfigErrors is one error message per invalid form field.
type ConfigErrors map[string]string

// HasErrors reports whether any field is invalid.
func (e ConfigErrors) HasErrors() bool { return len(e) > 0 }

// CompoundCycle reports whether config reaches itself through its members.
func CompoundCycle(config *openidea.RunConfig, all []*openidea.RunConfig) bool {
	if config == nil {
		return false
	}
	byID := make(map[string]*openidea.RunConfig, len(all)+1)
	for _, c := range all {
		if c != nil {
			byID[c.ID] = c
		}
	}
	byID[config.ID] = config
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if seen[id] {
			return false
		}
		seen[id] = true
		c := byID[id]
		if c == nil {
			return false
		}
		for _, child := range c.Compound {
			if child == config.ID || visit(child) {
				return true
			}
		}
		return false
	}
	return visit(config.ID)
}

// CompoundChildren is the flattened launch list: nested compounds are
// expanded and each concrete configuration is returned once.
func CompoundChildren(config *openidea.RunConfig, all []*openidea.RunConfig) []*openidea.RunConfig {
	if config == nil {
		return nil
	}
	byID := make(map[string]*openidea.RunConfig, len(all))
	for _, c := range all {
		if c != nil {
			byID[c.ID] = c
		}
	}
	seen := map[string]bool{config.ID: true}
	var out []*openidea.RunConfig
	var walk func([]string)
	walk = func(ids []string) {
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			c := byID[id]
			if c == nil {
				continue
			}
			if c.Kind == "compound" {
				walk(c.Compound)
				continue
			}
			out = append(out, c)
		}
	}
	walk(config.Compound)
	return out
}

func moduleTaskError(config *openidea.RunConfig, target string) string {
	if config.Module == nil {
		return "请为 Build Module 前置任务选择模块"
	}
	if *config.Module != target {
		return "Build Module 前置任务使用了其他模块，请移除后重新添加"
	}
	return ""
}

// ValidateConfig is the port of upstream validateConfig's fields GoIDEA
// edits: name, command, compound membership, before-launch tasks, environment,
// and working directory.
func ValidateConfig(config *openidea.RunConfig, all []*openidea.RunConfig) ConfigErrors {
	errs := ConfigErrors{}
	if config == nil {
		errs["name"] = "名称不能为空"
		return errs
	}
	if strings.TrimSpace(config.Name) == "" {
		errs["name"] = "名称不能为空"
	} else {
		for _, c := range all {
			if c != nil && c.ID != config.ID && strings.TrimSpace(c.Name) == strings.TrimSpace(config.Name) {
				errs["name"] = "另一个配置已使用此名称"
				break
			}
		}
	}
	if config.Kind == "compound" {
		if len(config.Compound) == 0 {
			errs["compound"] = "至少添加一个配置"
		} else if CompoundCycle(config, all) {
			errs["compound"] = "Compound 配置不能包含自身或形成循环"
		} else {
			for _, id := range config.Compound {
				if !configWithID(all, id) {
					errs["compound"] = "部分配置已不存在"
					break
				}
			}
		}
	} else if strings.TrimSpace(config.Command) == "" {
		errs["command"] = "命令不能为空"
	}
	for key, value := range MissingBuildToolParams(config) {
		errs[key] = value
	}
	for _, task := range config.BeforeLaunch {
		switch task.Kind {
		case "run":
			if task.Target == config.ID || !configWithID(all, task.Target) {
				errs["beforeLaunch"] = "前置任务运行了缺失的配置或当前配置"
			}
		case "build":
			if task.Target != "" && !configWithID(all, task.Target) {
				errs["beforeLaunch"] = "前置任务运行了缺失的配置"
			}
		case "buildModule":
			if e := moduleTaskError(config, task.Target); e != "" {
				errs["beforeLaunch"] = e
			}
		case "buildArtifacts", "dotnetBuild":
		default:
			if strings.TrimSpace(task.Target) == "" {
				errs["beforeLaunch"] = "一个前置任务尚未填写完整"
			}
		}
		if errs["beforeLaunch"] != "" {
			break
		}
	}
	for key := range config.Env {
		if strings.TrimSpace(key) == "" {
			errs["env"] = "环境变量名不能为空"
			break
		}
		if strings.ContainsAny(key, "= \t\r\n\x00") {
			errs["env"] = fmt.Sprintf("无效的环境变量名：%s", key)
			break
		}
	}
	if config.Cwd != nil && strings.Contains(*config.Cwd, "\x00") {
		errs["cwd"] = "无效的目录"
	}
	return errs
}

func configWithID(configs []*openidea.RunConfig, id string) bool {
	for _, config := range configs {
		if config != nil && config.ID == id {
			return true
		}
	}
	return false
}

// SaveConfigs is commands.run_save_configs: user configurations persist in
// `.openidea/run.toml`, `removedDetectedIds` stops the backend regenerating
// detected ones the user deleted. The payload starts from the
// configuration's raw object, so fields GoIDEA does not model survive, and
// the fields GoIDEA edits overlay it.
func SaveConfigs(ctx context.Context, backend Backend, user []*openidea.RunConfig, removedDetectedIDs []string) error {
	payload := make([]map[string]any, 0, len(user))
	for _, c := range user {
		obj, err := ConfigObject(c)
		if err != nil {
			return err
		}
		payload = append(payload, obj)
	}
	var removed any
	if removedDetectedIDs != nil {
		removed = removedDetectedIDs
	}
	_, err := backend.SaveRunConfigs(ctx, payload, removed)
	return err
}

// ConfigObject is the raw-preserving JSON object the editor sends back to
// run_save_configs or run_save_templates.
func ConfigObject(c *openidea.RunConfig) (map[string]any, error) {
	if c == nil {
		return nil, fmt.Errorf("nil run configuration")
	}
	src := c.Raw()
	if src == nil {
		var err error
		src, err = json.Marshal(c)
		if err != nil {
			return nil, err
		}
	}
	obj := map[string]any{}
	if err := json.Unmarshal(src, &obj); err != nil {
		return nil, err
	}
	obj["id"], obj["name"], obj["kind"], obj["command"] = c.ID, c.Name, c.Kind, c.Command
	args := c.Args
	if args == nil {
		args = []string{}
	}
	obj["args"] = args
	if c.Cwd != nil {
		obj["cwd"] = *c.Cwd
	} else {
		delete(obj, "cwd")
	}
	env := c.Env
	if env == nil {
		env = map[string]string{}
	}
	obj["env"] = env
	if c.Module != nil {
		obj["module"] = *c.Module
	} else {
		delete(obj, "module")
	}
	if c.BuildPolicy != nil {
		obj["buildPolicy"] = *c.BuildPolicy
	} else {
		delete(obj, "buildPolicy")
	}
	if c.Params == nil {
		delete(obj, "params")
	} else {
		obj["params"] = c.Params
	}
	envFiles := c.EnvFiles
	if envFiles == nil {
		envFiles = []string{}
	}
	obj["envFiles"] = envFiles
	obj["beforeLaunch"], obj["compound"] = c.BeforeLaunch, c.Compound
	if c.BeforeLaunch == nil {
		obj["beforeLaunch"] = []openidea.BeforeLaunchTask{}
	}
	if c.Compound == nil {
		obj["compound"] = []string{}
	}
	obj["showOnStdout"], obj["showOnStderr"] = c.ShowOnStdout, c.ShowOnStderr
	obj["focusToolWindow"], obj["emulateTerminal"] = c.FocusToolWindow, c.EmulateTerminal
	setOptionalJSONField(obj, "folder", c.Folder)
	setOptionalJSONField(obj, "includeSystemEnv", c.IncludeSystemEnv)
	setOptionalJSONField(obj, "stdinFile", c.StdinFile)
	setOptionalJSONField(obj, "outputFile", c.OutputFile)
	setOptionalJSONField(obj, "openToolWindow", c.OpenToolWindow)
	setOptionalJSONField(obj, "stopCommand", c.StopCommand)
	setOptionalJSONField(obj, "target", c.Target)
	obj["isTest"], obj["detected"] = c.IsTest, false
	if c.AllowParallel {
		obj["allowParallel"] = true
	} else {
		delete(obj, "allowParallel")
	}
	if c.Local {
		obj["local"] = true
	} else {
		delete(obj, "local")
	}
	return obj, nil
}

// TemplatePayload is the run_save_templates argument.
func TemplatePayload(templates []*openidea.RunConfig) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(templates))
	for _, template := range templates {
		obj, err := ConfigObject(template)
		if err != nil {
			return nil, err
		}
		delete(obj, "detected")
		delete(obj, "local")
		out = append(out, obj)
	}
	return out, nil
}

func setOptionalJSONField(obj map[string]any, key string, value any) {
	rv := reflect.ValueOf(value)
	if value == nil || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		delete(obj, key)
		return
	}
	if rv.Kind() == reflect.Pointer {
		obj[key] = rv.Elem().Interface()
		return
	}
	obj[key] = value
}

// SaveUserConfigs persists user configurations and resolves the missing
// module of a JVM application first, exactly as upstream saveUserConfigs
// does. backend then reloads the merged list.
func SaveUserConfigs(ctx context.Context, backend Backend, user []*openidea.RunConfig, removedDetectedIDs []string) error {
	for _, c := range user {
		if IsJVMApplication(c.Kind) && c.Module == nil {
			module, ok, err := FindJavaModule(ctx, backend, c)
			if err != nil {
				return err
			}
			if ok {
				c.Module = &module
			}
		}
	}
	return SaveConfigs(ctx, backend, user, removedDetectedIDs)
}
