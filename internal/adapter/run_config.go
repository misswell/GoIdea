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
	"gradle": "Gradle", "maven": "Maven", "go": "Go", "python": "Python",
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

// ValidateConfig is the port of upstream validateConfig's fields GoIDEA
// edits: the name is required and unique, a command is required, environment
// variable names cannot be empty or carry `=`, whitespace or NUL, and a
// working directory cannot carry NUL.
func ValidateConfig(config *openidea.RunConfig, all []*openidea.RunConfig) ConfigErrors {
	errs := ConfigErrors{}
	if strings.TrimSpace(config.Name) == "" {
		errs["name"] = "名称不能为空"
	} else {
		for _, c := range all {
			if c.ID != config.ID && strings.TrimSpace(c.Name) == strings.TrimSpace(config.Name) {
				errs["name"] = "另一个配置已使用此名称"
				break
			}
		}
	}
	if strings.TrimSpace(config.Command) == "" {
		errs["command"] = "命令不能为空"
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

// SaveConfigs is commands.run_save_configs: user configurations persist in
// `.openidea/run.toml`, `removedDetectedIds` stops the backend regenerating
// detected ones the user deleted. The payload starts from the
// configuration's raw object, so fields GoIDEA does not model survive, and
// the fields GoIDEA edits overlay it.
func SaveConfigs(ctx context.Context, backend Backend, user []*openidea.RunConfig, removedDetectedIDs []string) error {
	payload := make([]map[string]any, 0, len(user))
	for _, c := range user {
		src := c.Raw()
		if src == nil {
			var err error
			src, err = json.Marshal(c)
			if err != nil {
				return err
			}
		}
		obj := map[string]any{}
		if err := json.Unmarshal(src, &obj); err != nil {
			return err
		}
		// The fields the editor touches win over the raw object.
		obj["id"], obj["name"], obj["kind"], obj["command"] = c.ID, c.Name, c.Kind, c.Command
		if c.Args != nil {
			obj["args"] = c.Args
		}
		if c.Cwd != nil {
			obj["cwd"] = *c.Cwd
		}
		if c.Env != nil {
			obj["env"] = c.Env
		}
		if c.Module != nil {
			obj["module"] = *c.Module
		}
		if c.BuildPolicy != nil {
			obj["buildPolicy"] = *c.BuildPolicy
		}
		if c.EnvFiles != nil {
			obj["envFiles"] = c.EnvFiles
		}
		obj["isTest"], obj["detected"] = c.IsTest, false
		if c.AllowParallel {
			obj["allowParallel"] = true
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
