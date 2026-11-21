package app

// Run configuration parity actions after OpenIDEA's features/run: create a
// configuration from another feature, and import VS Code or JetBrains
// configurations into the native Run/Debug Configurations draft.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

func parityRunCommands() []appCommand {
	return []appCommand{
		{
			"run.importVscodeLaunch",
			"Import Run Configurations from .vscode/launch.json",
			"Run", "",
			"launch.json vscode visual studio code debug configuration import",
			func(m *Model) { m.ImportRunConfigurations(openidea.RunImportVSCode) },
		},
		{
			"run.importJetbrains",
			"Import Run Configurations from JetBrains (.run, .idea)",
			"Run", "",
			"pycharm webstorm intellij run.xml workspace.xml run configuration import",
			func(m *Model) { m.ImportRunConfigurations(openidea.RunImportJetBrains) },
		},
		{
			"run.createConfig",
			"Create Run Configuration",
			"Run", "",
			"create save run configuration launch",
			func(m *Model) {},
		},
	}
}

func parityRunActionState(a *Action) {
	switch a.ID {
	case "run.importVscodeLaunch", "run.importJetbrains":
		a.When = func(m *Model) bool { return m.Root() != "" }
	case "run.createConfig":
		a.RunWithArg = func(m *Model, arg any) {
			m.CreateRunConfiguration(arg)
		}
		a.When = func(m *Model) bool { return m.Root() != "" }
	}
}

// ImportRunConfigurations opens Run/Debug Configurations with the file's
// translations added to its draft. The configurations are saved only by the
// dialog's Save action, as upstream does.
func (m *Model) ImportRunConfigurations(source openidea.RunImportSource) {
	if m.Root() == "" || m.importingRunConfiguration() {
		return
	}
	if !m.run.trusted {
		m.status = "Trust the project before importing run configurations"
		m.invalidate()
		return
	}

	dialog := m.runCfgDlg
	var draft []*openidea.RunConfig
	var baseConfigs []openidea.RunConfig
	if dialog != nil {
		dialog.applyForm()
		draft = append([]*openidea.RunConfig(nil), dialog.items...)
		dialog.importing = true
		dialog.importErr = ""
	} else {
		m.run.loading = true
	}
	m.invalidate()

	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()

		sourcesRaw, err := m.invoke(ctx, "run_import_sources", nil)
		if err != nil {
			m.finishRunImport(dialog, root, nil, openidea.RunImport{}, err, false)
			return
		}
		var sources []openidea.RunImportSource
		if err := json.Unmarshal(sourcesRaw, &sources); err != nil {
			m.finishRunImport(dialog, root, nil, openidea.RunImport{}, err, false)
			return
		}
		if !slices.Contains(sources, source) {
			m.finishRunImport(dialog, root, nil, openidea.RunImport{Source: source}, nil, true)
			return
		}

		if dialog == nil {
			raw, err := m.invoke(ctx, "run_configs", nil)
			if err != nil {
				m.finishRunImport(nil, root, nil, openidea.RunImport{}, err, false)
				return
			}
			configs, err := openidea.DecodeRunConfigs(raw)
			if err != nil {
				m.finishRunImport(nil, root, nil, openidea.RunImport{}, err, false)
				return
			}
			baseConfigs = configs
			draft = runConfigPointers(configs)
		}

		existing, err := runConfigPayload(draft)
		if err != nil {
			m.finishRunImport(dialog, root, baseConfigs, openidea.RunImport{}, err, false)
			return
		}
		raw, err := m.invoke(ctx, "run_import_configs", map[string]any{
			"source": source, "existing": existing,
		})
		if err != nil {
			m.finishRunImport(dialog, root, baseConfigs, openidea.RunImport{}, err, false)
			return
		}
		var result openidea.RunImport
		if err := json.Unmarshal(raw, &result); err != nil {
			m.finishRunImport(dialog, root, baseConfigs, openidea.RunImport{}, err, false)
			return
		}
		m.finishRunImport(dialog, root, baseConfigs, result, nil, false)
	}()
}

func (m *Model) importingRunConfiguration() bool {
	return m.runCfgDlg != nil && m.runCfgDlg.importing
}

func (m *Model) finishRunImport(
	dialog *runConfigDlg,
	root string,
	baseConfigs []openidea.RunConfig,
	result openidea.RunImport,
	err error,
	missing bool,
) {
	m.post(func() {
		if m.Root() != root {
			return
		}
		if dialog != nil && m.runCfgDlg == dialog {
			dialog.importing = false
		} else {
			m.run.loading = false
		}
		if missing {
			m.notifyInfo("run-import", runImportMissingText(result.Source), "")
			m.status = runImportMissingText(result.Source)
			m.invalidate()
			return
		}
		if err != nil {
			if dialog != nil && m.runCfgDlg == dialog {
				dialog.importErr = "Cannot import run configurations: " + err.Error()
			} else {
				m.notifyError("run-import", "Cannot import run configurations", err.Error())
				m.status = "Cannot import run configurations: " + err.Error()
			}
			m.invalidate()
			return
		}

		imported, decodeErr := decodeImportedRunConfigs(result.Imported)
		if decodeErr != nil {
			m.notifyError("run-import", "Cannot read imported run configurations", decodeErr.Error())
			m.status = "Cannot read imported run configurations: " + decodeErr.Error()
			m.invalidate()
			return
		}
		notice := runImportNotice(result)
		if dialog != nil && m.runCfgDlg == dialog {
			appendRunImportToDialog(dialog, imported, result, notice)
		} else {
			m.run.configs = append(append([]openidea.RunConfig(nil), baseConfigs...), imported...)
			m.run.loaded = true
			m.OpenRunConfigs()
			if m.runCfgDlg != nil {
				appendRunImportToDialog(m.runCfgDlg, imported, result, notice)
			}
		}
		level := "info"
		if len(result.Skipped) > 0 || len(imported) == 0 {
			level = "warning"
		}
		m.notifyUser("run-import", level, runImportTitle(result, len(imported)), strings.Join(notice, "\n"))
		m.status = runImportTitle(result, len(imported))
		m.invalidate()
	})
}

func decodeImportedRunConfigs(imported []openidea.ImportedRunConfig) ([]openidea.RunConfig, error) {
	out := make([]openidea.RunConfig, 0, len(imported))
	for _, item := range imported {
		cfg, err := openidea.DecodeRunConfig(item.Config)
		if err != nil {
			return nil, err
		}
		cfg.Detected = false
		out = append(out, *cfg)
	}
	return out, nil
}

func appendRunImportToDialog(
	d *runConfigDlg,
	imported []openidea.RunConfig,
	result openidea.RunImport,
	notice []string,
) {
	first := len(d.items)
	for i := range imported {
		cfg := imported[i]
		cfg.Detected = false
		d.items = adapter.UpsertConfig(d.items, &cfg)
	}
	if len(imported) > 0 {
		d.sel = min(first, len(d.items)-1)
		d.loadForm()
	}
	d.importFile = result.File
	d.importNotice = append([]string(nil), notice...)
	d.importErr = ""
}

func runConfigPointers(configs []openidea.RunConfig) []*openidea.RunConfig {
	out := make([]*openidea.RunConfig, len(configs))
	for i := range configs {
		out[i] = &configs[i]
	}
	return out
}

func runConfigPayload(configs []*openidea.RunConfig) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(configs))
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		raw := cfg.Raw()
		if len(raw) == 0 {
			var err error
			raw, err = json.Marshal(cfg)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, append(json.RawMessage(nil), raw...))
	}
	return out, nil
}

func runImportMissingText(source openidea.RunImportSource) string {
	switch source {
	case openidea.RunImportVSCode:
		return "This project has no .vscode/launch.json"
	case openidea.RunImportJetBrains:
		return "This project has no JetBrains run configurations (.run, .idea)"
	}
	return "This project has no run configurations to import"
}

func runImportTitle(result openidea.RunImport, imported int) string {
	if imported == 0 {
		return "No run configurations imported from " + result.File
	}
	if imported == 1 {
		return "Imported 1 run configuration from " + result.File
	}
	return fmt.Sprintf("Imported %d run configurations from %s", imported, result.File)
}

func runImportNotice(result openidea.RunImport) []string {
	var out []string
	for _, skipped := range result.Skipped {
		name := skipped.Name
		if name == "" {
			name = "Unnamed"
		}
		out = append(out, fmt.Sprintf(
			"Skipped '%s': %s", name, runImportSkipReason(skipped.Reason)))
	}
	for _, imported := range result.Imported {
		if len(imported.Ignored) > 0 {
			var name string
			var cfg struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(imported.Config, &cfg)
			name = cfg.Name
			out = append(out, fmt.Sprintf(
				"Left out of '%s': %s", name, strings.Join(imported.Ignored, ", ")))
		}
	}
	if len(result.Imported) > 0 {
		out = append(out, "Click Save to keep the imported configurations.")
	}
	return out
}

func runImportSkipReason(reason map[string]any) string {
	kind, _ := reason["kind"].(string)
	value := func(key string) string {
		text, _ := reason[key].(string)
		return text
	}
	switch kind {
	case "nameTaken":
		return "A configuration with this name already exists"
	case "unsupportedType":
		return fmt.Sprintf("Type '%s' has no equivalent here", value("type"))
	case "unsupportedRequest":
		return fmt.Sprintf("Request '%s' is not supported", value("request"))
	case "unsupportedAttach":
		return "Only attaching to a host and port is supported"
	case "variable":
		return fmt.Sprintf("Uses %s, which is known only when VS Code starts it", value("variable"))
	case "nothingToRun":
		return "No program, module or command to run"
	case "pathVariable":
		return fmt.Sprintf("Uses %s, a path variable with no value here", value("variable"))
	case "severalModules":
		return fmt.Sprintf("Uses %s, and the project has several modules", value("variable"))
	case "missingFile":
		return fmt.Sprintf("%s is not in the project", value("file"))
	case "invalidFile":
		return "The file cannot be read: " + value("detail")
	case "missingMember":
		return fmt.Sprintf("Its configuration '%s' is not imported", value("member"))
	case "invalid":
		return "Not a configuration with a name"
	}
	return "The source entry has no equivalent here"
}

// CreateRunConfiguration saves the supplied configuration as a user
// configuration and opens it in the native editor, matching
// run.createConfig's hidden integration action.
func (m *Model) CreateRunConfiguration(arg any) {
	if m.Root() == "" || arg == nil {
		return
	}
	raw, err := json.Marshal(arg)
	if err != nil {
		m.status = "Cannot create the run configuration: " + err.Error()
		m.invalidate()
		return
	}
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		m.status = "Cannot create the run configuration: " + err.Error()
		m.invalidate()
		return
	}
	cfg.Detected = false
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		configRaw, err := m.invoke(ctx, "run_configs", nil)
		if err != nil {
			m.post(func() {
				m.notifyError("run-create", "Cannot create the run configuration", err.Error())
				m.status = "Cannot create the run configuration: " + err.Error()
				m.invalidate()
			})
			return
		}
		configs, err := openidea.DecodeRunConfigs(configRaw)
		if err != nil {
			m.post(func() {
				m.notifyError("run-create", "Cannot create the run configuration", err.Error())
				m.invalidate()
			})
			return
		}
		items := adapter.UpsertConfig(runConfigPointers(configs), cfg)
		err = adapter.SaveUserConfigs(ctx, javaBackend{m}, adapter.UserConfigs(items), nil)
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				m.notifyError("run-create", "Cannot create the run configuration", err.Error())
				m.status = "Cannot create the run configuration: " + err.Error()
				m.invalidate()
				return
			}
			m.run.configs = m.run.configs[:0]
			for _, item := range items {
				m.run.configs = append(m.run.configs, *item)
			}
			m.run.loaded = true
			m.OpenRunConfigs()
			d := m.runCfgDlg
			if d == nil {
				return
			}
			for i, item := range d.items {
				if item.ID == cfg.ID {
					d.sel = i
					d.loadForm()
					break
				}
			}
			m.status = "Created run configuration " + cfg.Name
			m.invalidate()
		})
	}()
	m.invalidate()
}
