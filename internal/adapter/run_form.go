package adapter

import (
	"encoding/json"
	"reflect"
	"strings"

	"mayoi/internal/openidea"
)

// RunTypeForm returns the editable values of OpenIDEA's non-build-tool run
// configuration types. Configurations saved before params were available are
// represented from their command line when it can be reversed without losing
// information.
func RunTypeForm(config *openidea.RunConfig) map[string]string {
	if config == nil {
		return nil
	}
	if form := BuildToolForm(config); form != nil {
		return form
	}
	if !hasRunTypeForm(config.Kind) {
		return nil
	}
	out := stringParams(config.Params)
	hasParams := len(config.Params) > 0 && (config.Kind != "go" || out["goKind"] != "")
	if hasParams {
		return out
	}
	switch config.Kind {
	case "shell":
		if out["mode"] == "" {
			out["mode"] = "command"
			out["command"] = config.Command
			out["args"] = FormatArgs(config.Args)
		}
	case "program":
		if len(config.Args) == 0 && strings.ContainsAny(config.Command, " \t\r\n") {
			return nil
		}
		if out["program"] == "" {
			out["program"] = config.Command
			out["arguments"] = FormatArgs(config.Args)
		}
	case "godot":
		if !isGodotCommand(config.Command) {
			return nil
		}
		if out["projectDir"] == "" {
			out["projectDir"] = config.CwdOr("")
		}
		if out["executable"] == "" && commandName(config.Command) != "godot" {
			out["executable"] = config.Command
		}
		if out["scene"] == "" {
			var rest []string
			for i := 0; i < len(config.Args); i++ {
				if config.Args[i] == "--path" {
					i++
					continue
				}
				if strings.EqualFold(extension(config.Args[i]), ".tscn") ||
					strings.EqualFold(extension(config.Args[i]), ".scn") {
					out["scene"] = config.Args[i]
					continue
				}
				rest = append(rest, config.Args[i])
			}
			out["arguments"] = FormatArgs(rest)
		}
	case "npm":
		if _, ok := reversePackageManagerForm(config, out, []string{"npm", "pnpm", "yarn", "bun"}, []string{
			"run", "test", "start", "install", "ci", "publish", "pack",
		}); !ok {
			return nil
		}
	case "bun":
		if _, ok := reversePackageManagerForm(config, out, []string{"bun"}, []string{
			"run", "test",
		}); !ok {
			return nil
		}
	case "pytest":
		if out["command"] == "" {
			command, runner, target, options, ok := reversePytestArgs(config)
			if !ok {
				return nil
			}
			out["command"] = command
			out["runnerArgs"] = FormatArgs(runner)
			out["target"] = target
			out["options"] = FormatArgs(options)
		}
	case "go", "go-test":
		if out["goKind"] == "" {
			goKind, scope, target, runAfterBuild, toolArgs, programArgs, ok := reverseGoArgs(config)
			if !ok {
				return nil
			}
			out["goKind"] = goKind
			out["scope"] = scope
			out["target"] = target
			out["runAfterBuild"] = boolString(runAfterBuild)
			out["toolArgs"] = FormatArgs(toolArgs)
			out["programArgs"] = FormatArgs(programArgs)
			out["module"] = config.CwdOr("")
			out["executable"] = config.Command
		}
		if config.Kind == "go-test" {
			out["goKind"] = "test"
		}
	default:
		return nil
	}
	if config.Kind != "go" && config.Kind != "go-test" {
		derived := ApplyRunTypeForm(config, out)
		if derived.Command != config.Command || !reflect.DeepEqual(nonNilArgs(derived.Args), nonNilArgs(config.Args)) ||
			(RunTypeOwnsWorkingDirectory(config.Kind) && derived.CwdOr("") != config.CwdOr("")) {
			return nil
		}
	}
	return out
}

func nonNilArgs(args []string) []string {
	if args == nil {
		return []string{}
	}
	return args
}

// ApplyRunTypeForm writes a non-build-tool type form and the command line it
// derives. Unknown params survive so fields outside the native form are not
// dropped.
func ApplyRunTypeForm(config *openidea.RunConfig, form map[string]string) *openidea.RunConfig {
	if config == nil {
		return config
	}
	if config.Kind == "maven" || config.Kind == "gradle" {
		return ApplyBuildToolForm(config, form)
	}
	if !hasRunTypeForm(config.Kind) {
		return config
	}

	next := *config
	next.Params = mergedStringParams(config.Params, form)
	switch config.Kind {
	case "shell":
		applyShellForm(&next, form)
	case "program":
		next.Command = strings.TrimSpace(form["program"])
		next.Args = ParseArgs(form["arguments"])
	case "godot":
		command := strings.TrimSpace(form["executable"])
		if command == "" {
			command = "godot"
		}
		args := []string{"--path", "."}
		if scene := strings.TrimSpace(form["scene"]); scene != "" {
			args = append(args, scene)
		}
		args = append(args, ParseArgs(form["arguments"])...)
		next.Command, next.Args = command, args
		setOptionalText(&next.Cwd, form["projectDir"])
	case "npm":
		applyPackageManagerForm(&next, form, "npm")
	case "bun":
		applyPackageManagerForm(&next, form, "bun")
	case "pytest":
		applyPytestForm(&next, form)
	case "go", "go-test":
		applyGoForm(&next, form)
		argv, _ := json.Marshal(ParseArgs(form["programArgs"]))
		if string(argv) == "null" {
			argv = []byte("[]")
		}
		next.Params["programArgv"] = string(argv)
		next.IsTest = form["goKind"] == "test" || config.Kind == "go-test"
		if config.Kind == "go-test" && !strings.HasPrefix(config.ID, "template:") {
			next.Kind = "go"
		}
	}
	return &next
}

func hasRunTypeForm(kind string) bool {
	switch kind {
	case "shell", "program", "godot", "npm", "bun", "pytest", "go", "go-test":
		return true
	default:
		return false
	}
}

func RunTypeOwnsWorkingDirectory(kind string) bool {
	switch kind {
	case "npm", "bun", "godot":
		return true
	default:
		return false
	}
}

func stringParams(params map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range params {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return out
}

func mergedStringParams(params map[string]any, form map[string]string) map[string]any {
	if len(params) == 0 && len(form) == 0 {
		return nil
	}
	out := make(map[string]any, len(params)+len(form))
	for key, value := range params {
		out[key] = value
	}
	for key, value := range form {
		if value = strings.TrimSpace(value); value != "" {
			out[key] = value
		} else {
			delete(out, key)
		}
	}
	return out
}

func setOptionalText(target **string, value string) {
	if value = strings.TrimSpace(value); value == "" {
		*target = nil
		return
	}
	*target = &value
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func extension(path string) string {
	name := strings.ReplaceAll(path, `\`, "/")
	if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
		name = name[slash+1:]
	}
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		return name[dot:]
	}
	return ""
}

func isGodotCommand(command string) bool {
	name := commandName(command)
	name = strings.ToLower(name)
	if !strings.HasPrefix(name, "godot") {
		return false
	}
	return strings.Trim(name[len("godot"):], "abcdefghijklmnopqrstuvwxyz0123456789_.-") == ""
}

func applyShellForm(config *openidea.RunConfig, form map[string]string) {
	switch strings.TrimSpace(form["mode"]) {
	case "text":
		config.Command = form["script"]
		config.Args = nil
	case "file":
		path := strings.TrimSpace(form["path"])
		interpreter := strings.TrimSpace(form["interpreter"])
		if interpreter == "" {
			config.Command = path
			config.Args = ParseArgs(form["options"])
			break
		}
		config.Command = interpreter
		config.Args = append(ParseArgs(form["interpreterOptions"]), path)
		config.Args = append(config.Args, ParseArgs(form["options"])...)
	default:
		config.Command = strings.TrimSpace(form["command"])
		config.Args = ParseArgs(form["args"])
	}
}

func reversePackageManagerForm(
	config *openidea.RunConfig,
	out map[string]string,
	managers []string,
	commands []string,
) (map[string]string, bool) {
	if out["command"] != "" {
		return out, true
	}
	manager := commandName(config.Command)
	if !containsString(managers, manager) || len(config.Args) == 0 || !containsString(commands, config.Args[0]) {
		return nil, false
	}
	command := config.Args[0]
	rest := append([]string(nil), config.Args[1:]...)
	script := ""
	if command == "run" && len(rest) > 0 {
		script, rest = rest[0], rest[1:]
	}
	out["packageManager"] = manager
	out["command"] = command
	out["script"] = script
	out["arguments"] = FormatArgs(rest)
	out["packageDir"] = config.CwdOr("")
	if config.Command != manager {
		out["executable"] = config.Command
	}
	return out, true
}

func applyPackageManagerForm(config *openidea.RunConfig, form map[string]string, fallback string) {
	manager := strings.TrimSpace(form["packageManager"])
	if manager == "" {
		manager = fallback
	}
	command := strings.TrimSpace(form["command"])
	if command == "" {
		command = "run"
	}
	args := []string{command}
	if command == "run" {
		args = append(args, form["script"])
	}
	args = append(args, ParseArgs(form["arguments"])...)
	executable := strings.TrimSpace(form["executable"])
	if executable == "" || commandName(executable) != manager {
		executable = manager
	}
	config.Command, config.Args = executable, args
	setOptionalText(&config.Cwd, form["packageDir"])
}

func reversePytestArgs(config *openidea.RunConfig) (string, []string, string, []string, bool) {
	runner := -1
	switch {
	case len(config.Args) >= 2 && config.Args[0] == "run" && config.Args[1] == "pytest":
		runner = 2
	default:
		for i := 0; i+1 < len(config.Args); i++ {
			if config.Args[i] == "-m" && config.Args[i+1] == "pytest" {
				runner = i + 2
				break
			}
		}
		if runner < 0 && commandName(config.Command) == "pytest" {
			runner = 0
		}
	}
	if runner < 0 {
		return "", nil, "", nil, false
	}
	rest := append([]string(nil), config.Args[runner:]...)
	target := ""
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		target, rest = rest[0], rest[1:]
	}
	return config.Command, append([]string(nil), config.Args[:runner]...), target, rest, true
}

func applyPytestForm(config *openidea.RunConfig, form map[string]string) {
	command := strings.TrimSpace(form["command"])
	if command == "" {
		command = "pytest"
	}
	args := ParseArgs(form["runnerArgs"])
	if target := strings.TrimSpace(form["target"]); target != "" {
		args = append(args, ParseArgs(target)...)
	}
	args = append(args, ParseArgs(form["options"])...)
	config.Command, config.Args = command, args
}

func reverseGoArgs(config *openidea.RunConfig) (string, string, string, bool, []string, []string, bool) {
	if len(config.Args) == 0 {
		return "", "", "", false, nil, nil, false
	}
	op := config.Args[0]
	switch op {
	case "run":
		if len(config.Args) == 2 && !strings.HasPrefix(config.Args[1], "-") {
			return "build", "package", config.Args[1], true, nil, nil, true
		}
	case "build", "test":
		if len(config.Args) == 2 && !strings.HasPrefix(config.Args[1], "-") {
			goKind := op
			if goKind == "run" {
				goKind = "build"
			}
			return goKind, "package", config.Args[1], op == "run", nil, nil, true
		}
	}
	return "", "", "", false, nil, nil, false
}

func applyGoForm(config *openidea.RunConfig, form map[string]string) {
	goKind := strings.TrimSpace(form["goKind"])
	if config.Kind == "go-test" {
		goKind = "test"
	}
	if goKind == "" {
		goKind = "build"
	}
	toolArgs := ParseArgs(form["toolArgs"])
	target := strings.TrimSpace(form["target"])
	if target == "" {
		target = "."
	}
	targets := []string{target}
	if strings.TrimSpace(form["scope"]) == "file" {
		targets = nil
		for _, item := range strings.Split(target, "|") {
			if item = strings.TrimSpace(item); item != "" {
				targets = append(targets, item)
			}
		}
	}
	args := []string{"build"}
	if goKind == "test" {
		args = []string{"test"}
		args = append(args, toolArgs...)
		if pattern := strings.TrimSpace(form["pattern"]); pattern != "" {
			args = append(args, "-run", pattern)
		}
		args = append(args, targets...)
		if program := strings.TrimSpace(form["programArgs"]); program != "" {
			args = append(args, "-args")
			args = append(args, ParseArgs(program)...)
		}
	} else {
		args = append(args, toolArgs...)
		args = append(args, targets...)
	}
	command := strings.TrimSpace(form["executable"])
	if command == "" {
		command = "go"
	}
	config.Command, config.Args = command, args
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
