package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// prepareDotnetLaunch ports run_dotnet_resolve plus the run and test command
// builders from OpenIDEA's .NET run configuration types.
func (m *Model) prepareDotnetLaunch(
	ctx context.Context,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var request map[string]any
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("The run configuration could not be resolved: %w", err)
	}
	resolvedRaw, err := m.invoke(ctx, "run_dotnet_resolve", map[string]any{
		"config": request,
		"debug":  false,
	})
	if err != nil {
		return nil, err
	}
	resolved, err := openidea.DecodeRunConfig(resolvedRaw)
	if err != nil {
		return nil, err
	}
	switch resolved.Kind {
	case "dotnet":
		return m.dotnetRunResolved(resolved)
	case "dotnet-test":
		return m.dotnetTestResolved(ctx, resolved)
	default:
		return nil, fmt.Errorf(".NET launch preparation received kind %q", resolved.Kind)
	}
}

func (m *Model) dotnetRunResolved(
	config *openidea.RunConfig,
) (json.RawMessage, error) {
	params := config.Params
	if dotnetParamString(params, "project") == "" {
		return nil, errors.New("Select a .NET project")
	}
	programArgs := dotnetParamString(params, "programArgs")
	if strings.TrimSpace(programArgs) == "" {
		programArgs = dotnetParamString(params, "profileArgs")
	}
	args := adapter.ParseArgs(programArgs)
	commandParams := make(map[string]any, len(params)+2)
	for key, value := range params {
		commandParams[key] = value
	}
	commandParams["launchProfile"] = ""
	commandParams["programArgs"] = ""
	command, line := dotnetCommand(commandParams, false)
	if len(args) > 0 {
		line = append(line, "--")
		line = append(line, args...)
	}
	return resolvedRunConfig(config.Raw(), command, line)
}

func (m *Model) dotnetTestResolved(
	ctx context.Context,
	config *openidea.RunConfig,
) (json.RawMessage, error) {
	root := m.Root()
	projectValue := dotnetParamString(config.Params, "project")
	if root == "" || strings.TrimSpace(projectValue) == "" {
		return nil, errors.New("Select a .NET project")
	}
	projectPath := dotnetAbsolute(root, projectValue)
	configuration := dotnetParamString(config.Params, "configuration")
	if configuration == "" {
		configuration = "Debug"
	}
	framework := dotnetParamString(config.Params, "framework")
	project, err := dotnetEvaluateProject(
		ctx,
		m,
		projectPath,
		configuration,
		framework,
	)
	if err != nil {
		return nil, err
	}
	if project.Runner != dotnetRunnerVSTest && project.PlatformVersion == "" {
		if _, err := m.runDotnetCommand(
			ctx,
			[]string{"restore", project.Path},
			filepath.Dir(project.Path),
		); err != nil {
			return nil, err
		}
		project, err = dotnetEvaluateProject(
			ctx,
			m,
			project.Path,
			configuration,
			framework,
		)
		if err != nil {
			return nil, err
		}
	}
	params := make(map[string]any, len(config.Params)+3)
	for key, value := range config.Params {
		params[key] = value
	}
	params["project"] = project.Path
	params["runner"] = string(project.Runner)
	params["adapter"] = project.Adapter
	params["trx"] = fmt.Sprintf("%t", project.TRX)
	command, args, err := dotnetTestCommandLine(params)
	if err != nil {
		return nil, err
	}
	return resolvedRunConfigWithParams(config.Raw(), command, args, params)
}

func resolvedRunConfigWithParams(
	raw json.RawMessage,
	command string,
	args []string,
	params map[string]any,
) (json.RawMessage, error) {
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("The run configuration could not be resolved: %w", err)
	}
	object["command"], object["args"], object["params"] = command, args, params
	return json.Marshal(object)
}
