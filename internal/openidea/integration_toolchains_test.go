package openidea_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func recordToolchainParityEvidence(t *testing.T, evidence map[string]any) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("GOIDEA_PARITY_REPORT"))
	if path == "" {
		return
	}
	report := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatalf("read parity report: %v", err)
			}
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("read parity report: %v", err)
	}
	merged := map[string]any{}
	if existing, ok := report["toolchains"].(map[string]any); ok {
		for key, value := range existing {
			merged[key] = value
		}
	}
	for key, value := range evidence {
		merged[key] = value
	}
	report["toolchains"] = merged
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	temp := path + ".toolchains.tmp"
	if err := os.WriteFile(temp, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write toolchain evidence: %v", err)
	}
	if err := os.Rename(temp, path); err != nil {
		t.Fatalf("replace parity report: %v", err)
	}
}

// TestIntegrationLanguageToolchainsAcceptance exercises the pinned server's
// real detector and Python environment RPCs. It is opt-in because it creates a
// virtualenv and starts language discovery against the host's installed tools.
//
//	GOIDEA_SERVER=$PWD/resources/openidea-server GOIDEA_SDK_ACCEPTANCE=1 \
//	  CGO_ENABLED=0 go test ./internal/openidea -run LanguageToolchainsAcceptance -v
func TestIntegrationLanguageToolchainsAcceptance(t *testing.T) {
	if os.Getenv("GOIDEA_SDK_ACCEPTANCE") != "1" {
		t.Skip("GOIDEA_SDK_ACCEPTANCE is not set")
	}
	m := launchForTest(t)
	requiredCommands := []string{
		"sdk_detect", "sdk_python_project_interpreters", "sdk_python_interpreter",
		"sdk_python_effective", "sdk_php_effective", "sdk_php_xdebug",
		"sdk_php_language_level", "sdk_locate_tools", "sdk_python_create_venv",
	}
	for _, command := range requiredCommands {
		if !m.Server().HasCommand(command) {
			t.Fatalf("pinned backend does not offer %s", command)
		}
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte("print('toolchain')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "__main__.py"), []byte("print('app')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{
		"require":{"php":">=8.1"}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.php"), []byte("<?php echo PHP_VERSION;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const window = "goidea-sdk-it"
	openProject(t, m, window, root)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if _, err := m.Invoke(ctx, window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": false,
	}); err != nil {
		t.Fatalf("trust project: %v", err)
	}

	sdkKinds := []openidea.SdkKind{
		openidea.SdkPython, openidea.SdkNode, openidea.SdkPhp,
		openidea.SdkPackageManager, openidea.SdkRust, openidea.SdkGo,
		openidea.SdkTypeScript, openidea.SdkJDK, openidea.SdkMaven, openidea.SdkRuby,
	}
	for _, kind := range sdkKinds {
		if _, err := m.DetectSDKs(ctx, window, kind); err != nil {
			t.Fatalf("sdk_detect %s: %v", kind, err)
		}
	}
	if _, err := m.PythonProjectInterpreters(ctx, window); err != nil {
		t.Fatalf("sdk_python_project_interpreters: %v", err)
	}

	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	python, err := m.PythonInterpreter(ctx, window, pythonPath)
	if err != nil {
		t.Fatalf("sdk_python_interpreter: %v", err)
	}
	if python == nil || python.Path == "" {
		t.Fatalf("python3 did not validate as an interpreter: %s", pythonPath)
	}

	venv := filepath.Join(root, ".venv")
	created, err := m.CreatePythonEnvironment(
		ctx, window, "venv", python.Path, venv, 701, 702,
	)
	if err != nil {
		t.Fatalf("sdk_python_create_venv: %v", err)
	}
	if !strings.HasPrefix(created.Path, venv+string(filepath.Separator)) {
		t.Fatalf("created interpreter = %q, want under %q", created.Path, venv)
	}
	if info, err := os.Stat(created.Path); err != nil || info.IsDir() {
		t.Fatalf("created Python executable %q: info=%v err=%v", created.Path, info, err)
	}

	encoded, _ := json.Marshal(created.Path)
	if _, err := m.Invoke(ctx, window, "settings_set", map[string]any{
		"scope": "project", "key": "languages.python.interpreter", "value": string(encoded),
	}); err != nil {
		t.Fatalf("persist Python interpreter: %v", err)
	}
	effective, err := m.PythonEffective(ctx, window, filepath.Join(root, "main.py"))
	if err != nil {
		t.Fatalf("sdk_python_effective: %v", err)
	}
	if effective == nil || *effective != created.Path {
		t.Fatalf("effective Python = %v, want %q", effective, created.Path)
	}

	rawConfigs, err := m.Invoke(ctx, window, "run_configs", nil)
	if err != nil {
		t.Fatalf("run_configs: %v", err)
	}
	configs, err := openidea.DecodeRunConfigs(rawConfigs)
	if err != nil {
		t.Fatalf("decode run_configs: %v", err)
	}
	foundPython := false
	for _, config := range configs {
		if config.Kind != "python" {
			continue
		}
		foundPython = true
		if config.Command != created.Path {
			t.Fatalf("Python run command = %q, want settings interpreter %q", config.Command, created.Path)
		}
	}
	if !foundPython {
		t.Fatalf("no Python run configuration detected: %+v", configs)
	}

	located, err := m.LocateTools(ctx, window, []string{"node", "php", "ruby"}, filepath.Join(root, "main.py"))
	if err != nil {
		t.Fatalf("sdk_locate_tools: %v", err)
	}
	if len(located.Tools) != 3 || located.CompileDir == "" {
		t.Fatalf("sdk_locate_tools = %+v", located)
	}
	for _, tool := range located.Tools {
		if path, err := exec.LookPath(tool.Name); err == nil && tool.Path == nil {
			t.Fatalf("%s is on PATH at %q but sdk_locate_tools returned nil", tool.Name, path)
		}
	}

	if phpPath, err := exec.LookPath("php"); err == nil {
		if _, err := m.PhpXdebug(ctx, window, phpPath, root); err != nil {
			t.Fatalf("sdk_php_xdebug: %v", err)
		}
	}
	levels, err := m.PhpLanguageLevel(ctx, window, filepath.Join(root, "main.php"))
	if err != nil {
		t.Fatalf("sdk_php_language_level: %v", err)
	}
	if levels.Auto == nil || levels.Auto.Version == "" {
		t.Fatalf("Composer language level was not detected: %+v", levels)
	}

	recordToolchainParityEvidence(t, map[string]any{
		"rpcCommands": len(requiredCommands),
		"sdkKinds":    len(sdkKinds),
		"python":      true,
		"runConfig":   true,
		"tools":       true,
		"php":         true,
	})
}
