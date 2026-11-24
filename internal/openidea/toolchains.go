package openidea

import (
	"context"
	"encoding/json"
)

// SdkKind is one family of runtimes the backend can discover. The string
// values are the IPC spelling, including the camel-cased typeScript value.
type SdkKind string

const (
	SdkPython         SdkKind = "python"
	SdkNode           SdkKind = "node"
	SdkPhp            SdkKind = "php"
	SdkPackageManager SdkKind = "packageManager"
	SdkRust           SdkKind = "rust"
	SdkGo             SdkKind = "go"
	SdkTypeScript     SdkKind = "typeScript"
	SdkJDK            SdkKind = "jdk"
	SdkMaven          SdkKind = "maven"
	SdkRuby           SdkKind = "ruby"
)

// ProjectInterpreter is one interpreter another trusted recent project uses.
type ProjectInterpreter struct {
	Project string `json:"project"`
	SDK     Sdk    `json:"sdk"`
}

// LocatedTool is a bare command the backend resolved for run-this-file
// support. Path is nil when the command is missing.
type LocatedTool struct {
	Name    string  `json:"name"`
	Path    *string `json:"path"`
	Version *string `json:"version"`
}

// LocatedTools is the reply to sdk_locate_tools.
type LocatedTools struct {
	Tools      []LocatedTool `json:"tools"`
	CompileDir string        `json:"compileDir"`
}

// PhpLanguageLevel is the PHP version the backend selected and where it came
// from: settings, composer platform, or composer requirements.
type PhpLanguageLevel struct {
	Version string `json:"version"`
	Source  string `json:"source"`
}

// PhpLanguageLevels is the effective and Auto PHP level for a project member.
type PhpLanguageLevels struct {
	Effective *PhpLanguageLevel `json:"effective"`
	Auto      *PhpLanguageLevel `json:"auto"`
	Member    *string           `json:"member"`
}

// PhpXdebug is a validated PHP executable and the Xdebug version it loads.
type PhpXdebug struct {
	PHP        string  `json:"php"`
	PHPVersion string  `json:"phpVersion"`
	Xdebug     *string `json:"xdebug"`
}

// PythonPackage is one package installed in a Python environment.
type PythonPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// DetectSDKs lists installed toolchains, project-local ones first.
func (m *Manager) DetectSDKs(ctx context.Context, window string, kind SdkKind) ([]Sdk, error) {
	raw, err := m.Invoke(ctx, window, "sdk_detect", map[string]any{"kind": kind})
	if err != nil {
		return nil, err
	}
	var list []Sdk
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []Sdk{}
	}
	return list, nil
}

// PythonProjectInterpreters lists interpreters used by other trusted recent
// projects. The open project is returned by DetectSDKs instead.
func (m *Manager) PythonProjectInterpreters(ctx context.Context, window string) ([]ProjectInterpreter, error) {
	raw, err := m.Invoke(ctx, window, "sdk_python_project_interpreters", nil)
	if err != nil {
		return nil, err
	}
	var list []ProjectInterpreter
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []ProjectInterpreter{}
	}
	return list, nil
}

// PythonInterpreter resolves an executable or virtualenv directory as an SDK.
// Nil means the path does not run as Python.
func (m *Manager) PythonInterpreter(ctx context.Context, window, path string) (*Sdk, error) {
	raw, err := m.Invoke(ctx, window, "sdk_python_interpreter", map[string]any{"path": path})
	if err != nil {
		return nil, err
	}
	var sdk *Sdk
	if err := json.Unmarshal(raw, &sdk); err != nil {
		return nil, err
	}
	return sdk, nil
}

// PythonEffective is the interpreter a project file uses. Nil means no
// project or virtualenv is available.
func (m *Manager) PythonEffective(ctx context.Context, window, near string) (*string, error) {
	var path any
	if near != "" {
		path = near
	}
	raw, err := m.Invoke(ctx, window, "sdk_python_effective", map[string]any{"near": path})
	if err != nil {
		return nil, err
	}
	var interpreter *string
	if err := json.Unmarshal(raw, &interpreter); err != nil {
		return nil, err
	}
	return interpreter, nil
}

// PhpEffective is the configured PHP executable. Nil means php on PATH.
func (m *Manager) PhpEffective(ctx context.Context, window string) (*string, error) {
	raw, err := m.Invoke(ctx, window, "sdk_php_effective", nil)
	if err != nil {
		return nil, err
	}
	var php *string
	if err := json.Unmarshal(raw, &php); err != nil {
		return nil, err
	}
	return php, nil
}

// PhpXdebug validates a PHP executable and reports the loaded Xdebug version.
func (m *Manager) PhpXdebug(ctx context.Context, window, php, cwd string) (PhpXdebug, error) {
	args := map[string]any{"php": php}
	if cwd != "" {
		args["cwd"] = cwd
	}
	raw, err := m.Invoke(ctx, window, "sdk_php_xdebug", args)
	if err != nil {
		return PhpXdebug{}, err
	}
	var result PhpXdebug
	if err := json.Unmarshal(raw, &result); err != nil {
		return PhpXdebug{}, err
	}
	return result, nil
}

// PhpLanguageLevel returns the effective PHP language level for path and the
// member whose settings supplied it.
func (m *Manager) PhpLanguageLevel(ctx context.Context, window, path string) (PhpLanguageLevels, error) {
	raw, err := m.Invoke(ctx, window, "sdk_php_language_level", map[string]any{"path": path})
	if err != nil {
		return PhpLanguageLevels{}, err
	}
	var result PhpLanguageLevels
	if err := json.Unmarshal(raw, &result); err != nil {
		return PhpLanguageLevels{}, err
	}
	return result, nil
}

// LocateTools resolves bare command names, preferring node_modules/.bin above
// near and then the search path.
func (m *Manager) LocateTools(ctx context.Context, window string, names []string, near string) (LocatedTools, error) {
	args := map[string]any{"names": names}
	if near != "" {
		args["near"] = near
	}
	raw, err := m.Invoke(ctx, window, "sdk_locate_tools", args)
	if err != nil {
		return LocatedTools{}, err
	}
	var result LocatedTools
	if err := json.Unmarshal(raw, &result); err != nil {
		return LocatedTools{}, err
	}
	if result.Tools == nil {
		result.Tools = []LocatedTool{}
	}
	return result, nil
}

// CreatePythonEnvironment creates a venv, uv, Poetry or Conda environment and
// streams one output line per channel message.
func (m *Manager) CreatePythonEnvironment(
	ctx context.Context,
	window, tool, base, location string,
	request, channel uint32,
) (Sdk, error) {
	raw, err := m.Invoke(ctx, window, "sdk_python_create_venv", map[string]any{
		"tool":     tool,
		"base":     base,
		"location": location,
		"request":  request,
		"onOutput": map[string]any{"$channel": channel},
	})
	if err != nil {
		return Sdk{}, err
	}
	var sdk Sdk
	if err := json.Unmarshal(raw, &sdk); err != nil {
		return Sdk{}, err
	}
	return sdk, nil
}

// PythonPackages lists the packages in a Python environment.
func (m *Manager) PythonPackages(
	ctx context.Context,
	window, python string,
	request uint32,
) ([]PythonPackage, error) {
	raw, err := m.Invoke(ctx, window, "sdk_python_packages", map[string]any{
		"python": python, "request": request,
	})
	if err != nil {
		return nil, err
	}
	var list []PythonPackage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []PythonPackage{}
	}
	return list, nil
}

// PythonPip installs, upgrades or uninstalls a package, streaming output on
// channel. Callers cancel it with cancel_request(request).
func (m *Manager) PythonPip(
	ctx context.Context,
	window, python, requirement string,
	uninstall, upgrade bool,
	request, channel uint32,
) error {
	_, err := m.Invoke(ctx, window, "sdk_python_pip", map[string]any{
		"python":      python,
		"requirement": requirement,
		"uninstall":   uninstall,
		"upgrade":     upgrade,
		"request":     request,
		"onOutput":    map[string]any{"$channel": channel},
	})
	return err
}
