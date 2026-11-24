package app

// Gradle has no command-line switch for test events. OpenIDEA writes a stable
// init script on the machine that runs the build and passes it to Gradle; the
// script emits TeamCity service messages consumed by the native test tree.

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

//go:embed gradle_tests.gradle
var gradleTestsScript string

const gradleTestsScriptName = "goidea-gradle-tests"

func isGradleCommand(config *openidea.RunConfig) bool {
	if config == nil {
		return false
	}
	if config.Kind == "gradle" {
		return true
	}
	name := strings.ToLower(filepath.Base(config.Command))
	return name == "gradle" || name == "gradlew" || name == "gradlew.bat"
}

func hasInitScript(args []string, path string) bool {
	for i, arg := range args {
		if arg == path {
			return true
		}
		if arg == "--init-script" && i+1 < len(args) && args[i+1] == path {
			return true
		}
	}
	return false
}

// prepareGradleJVMLaunch applies VM options and Spring Boot profiles through
// the backend's Gradle init-script generator. Upstream refuses these fields on
// a remote run target, where the generated local script cannot be mounted.
func (m *Model) prepareGradleJVMLaunch(
	ctx context.Context,
	raw json.RawMessage,
	config *openidea.RunConfig,
) (json.RawMessage, error) {
	jvmArgs := adapter.GradleForkedJVMArgs(config)
	if len(jvmArgs) == 0 {
		return raw, nil
	}
	if config.Target != nil && strings.TrimSpace(*config.Target) != "" {
		return nil, fmt.Errorf("VM options of Gradle configurations are not applied on run targets")
	}
	reply, err := m.invoke(ctx, "run_gradle_jvm_args_script", map[string]any{
		"jvmArgs": jvmArgs,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot prepare Gradle VM options: %w", err)
	}
	var script string
	if err := json.Unmarshal(reply, &script); err != nil {
		return nil, fmt.Errorf("cannot read the Gradle VM options script: %w", err)
	}
	if strings.TrimSpace(script) == "" {
		return nil, fmt.Errorf("the Gradle VM options script is empty")
	}
	args := append([]string{"--init-script", script}, config.Args...)
	patched, ok := patchRunConfigArgs(raw, args)
	if !ok {
		return nil, fmt.Errorf("cannot update the Gradle configuration arguments")
	}
	return patched, nil
}

// prepareGradleTestLaunch adds OpenIDEA's reporting init script to a Gradle
// test configuration. A script-writing failure leaves the launch usable,
// matching upstream gradleTestLaunch.
func (m *Model) prepareGradleTestLaunch(ctx context.Context, raw json.RawMessage, config *openidea.RunConfig) json.RawMessage {
	if config == nil || !config.IsTest || !isGradleCommand(config) {
		return raw
	}
	reply, err := m.invoke(ctx, "run_gradle_init_script", map[string]any{
		"name": gradleTestsScriptName,
		"text": gradleTestsScript,
	})
	if err != nil {
		return raw
	}
	var script string
	if err := json.Unmarshal(reply, &script); err != nil || strings.TrimSpace(script) == "" {
		return raw
	}
	if hasInitScript(config.Args, script) {
		return raw
	}
	args := append([]string{"--init-script", script}, config.Args...)
	patched, ok := patchRunConfigArgs(raw, args)
	if !ok {
		return raw
	}
	return patched
}

// gradleInitScriptArgs is separated for tests: it returns the exact prefix
// upstream withTestEvents uses.
func gradleInitScriptArgs(script string, args []string) []string {
	return append([]string{"--init-script", script}, args...)
}

func gradleInitScriptError(err error) error {
	return fmt.Errorf("cannot prepare the Gradle test init script: %w", err)
}
