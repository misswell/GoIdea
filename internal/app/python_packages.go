package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const pythonPackageLogLimit = 400

type pythonPackageJob struct {
	op         string
	target     string
	request    uint32
	channel    uint32
	generation uint64
}

type pythonPackagesState struct {
	root        string
	configured  string
	interpreter string
	packages    []openidea.PythonPackage
	loading     bool
	loaded      bool
	gen         uint64
	err         string
	job         *pythonPackageJob
	jobGen      uint64
	jobErr      string
	log         []string
	query       string
	requirement string
	installOpen bool
	frozen      string
}

// ensurePythonPackages resolves the same interpreter run configurations use,
// then loads its installed packages through the pinned SDK RPC. A stale reply
// is ignored after a project or interpreter switch.
func (m *Model) ensurePythonPackages(force bool) {
	root := m.Root()
	if root == "" {
		return
	}
	configured := strings.TrimSpace(nativeSettingsString(
		m,
		settingsPagePythonDeps,
		"languages.python.interpreter",
	))
	state := m.languageSDKs.packages
	if !force && state.root == root && state.configured == configured &&
		(state.loading || state.loaded || state.err != "") {
		return
	}
	state.root = root
	state.configured = configured
	state.loading = true
	state.loaded = false
	state.err = ""
	state.interpreter = ""
	state.gen++
	generation := state.gen
	m.languageSDKs.packages = state
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.err = "The GoIDEA backend is not connected"
		m.languageSDKs.packages = state
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "sdk_python_effective", map[string]any{"near": root})
		var interpreter *string
		if err == nil {
			err = json.Unmarshal(raw, &interpreter)
		}
		var packages []openidea.PythonPackage
		request := m.nextRequest()
		if err == nil && interpreter != nil && strings.TrimSpace(*interpreter) != "" {
			raw, err = m.invoke(ctx, "sdk_python_packages", map[string]any{
				"python": *interpreter, "request": request,
			})
			if err == nil {
				err = json.Unmarshal(raw, &packages)
			}
		}
		m.post(func() {
			current := m.languageSDKs.packages
			if current.gen != generation || m.Root() != root ||
				strings.TrimSpace(nativeSettingsString(
					m,
					settingsPagePythonDeps,
					"languages.python.interpreter",
				)) != configured {
				return
			}
			current.loading = false
			current.loaded = err == nil
			if err != nil {
				current.err = err.Error()
			} else if interpreter != nil {
				current.interpreter = strings.TrimSpace(*interpreter)
				current.packages = append([]openidea.PythonPackage(nil), packages...)
			}
			m.languageSDKs.packages = current
			m.invalidate()
		})
	}()
}

func filterPythonPackages(packages []openidea.PythonPackage, query string) []openidea.PythonPackage {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]openidea.PythonPackage(nil), packages...)
	}
	out := make([]openidea.PythonPackage, 0, len(packages))
	for _, pkg := range packages {
		if strings.Contains(strings.ToLower(pkg.Name), query) {
			out = append(out, pkg)
		}
	}
	return out
}

// freezePythonRequirements formats the installed package snapshot as
// requirements.txt. The pinned backend exposes `pip list --format=json`; the
// generated lines are deterministic name==version pins.
func freezePythonRequirements(packages []openidea.PythonPackage) string {
	sorted := append([]openidea.PythonPackage(nil), packages...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})
	var lines []string
	for _, pkg := range sorted {
		name := strings.TrimSpace(pkg.Name)
		version := strings.TrimSpace(pkg.Version)
		if name == "" {
			continue
		}
		if version == "" {
			lines = append(lines, name)
		} else {
			lines = append(lines, name+"=="+version)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m *Model) startPythonPackageJob(op, target string) {
	state := m.languageSDKs.packages
	root := m.Root()
	target = strings.TrimSpace(target)
	if root == "" || state.interpreter == "" || target == "" || state.job != nil {
		return
	}
	request, channel := m.nextRequest(), m.nextChannel()
	state.jobGen++
	job := &pythonPackageJob{
		op: op, target: target, request: request, channel: channel, generation: state.jobGen,
	}
	state.job = job
	state.jobErr = ""
	state.log = nil
	state.frozen = ""
	m.languageSDKs.packages = state
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 30*time.Minute)
		defer cancel()
		_, err := m.invoke(ctx, "sdk_python_pip", map[string]any{
			"python":      state.interpreter,
			"requirement": target,
			"uninstall":   op == "uninstall",
			"upgrade":     op == "upgrade",
			"request":     request,
			"onOutput":    map[string]any{"$channel": channel},
		})
		m.post(func() {
			current := m.languageSDKs.packages
			if current.jobGen != job.generation || current.job != job || m.Root() != root {
				return
			}
			current.job = nil
			switch {
			case err != nil && (ctx.Err() == context.Canceled || errors.Is(err, context.Canceled)):
				current.jobErr = ""
			case err != nil:
				current.jobErr = err.Error()
			case op == "install":
				current.requirement = ""
				current.installOpen = false
			}
			m.languageSDKs.packages = current
			if err == nil {
				m.ensurePythonPackages(true)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) cancelPythonPackageJob() {
	state := m.languageSDKs.packages
	if state.job == nil || state.job.request == 0 {
		return
	}
	request := state.job.request
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"request": request})
	}()
}

func (m *Model) acceptPythonPackageChunk(channel uint32, data json.RawMessage) bool {
	state := m.languageSDKs.packages
	if channel == 0 || state.job == nil || state.job.channel != channel {
		return false
	}
	var line string
	if json.Unmarshal(data, &line) != nil {
		return true
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	state.log = append(state.log, line)
	if len(state.log) > pythonPackageLogLimit {
		state.log = append([]string(nil), state.log[len(state.log)-pythonPackageLogLimit:]...)
	}
	m.languageSDKs.packages = state
	m.invalidate()
	return true
}

func (m *Model) freezePythonPackages() {
	state := m.languageSDKs.packages
	state.frozen = freezePythonRequirements(state.packages)
	m.languageSDKs.packages = state
	if state.frozen == "" {
		m.status = "No installed packages to freeze"
	}
	m.invalidate()
}

func (m *Model) savePythonRequirements() {
	state := m.languageSDKs.packages
	if state.frozen == "" {
		return
	}
	root := m.Root()
	parent := m.win
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:            parent,
			Title:             "Save Frozen Requirements",
			ButtonLabel:       "Save",
			DefaultPath:       filepath.Join(root, "requirements.txt"),
			CreateDirectories: true,
			Filters: []mygo.FileFilter{{
				Name: "Requirements", Extensions: []string{"txt"},
			}},
		})
		if err != nil || path == "" {
			return
		}
		err = os.WriteFile(path, []byte(state.frozen), 0o644)
		m.post(func() {
			if m.Root() != root {
				return
			}
			if err != nil {
				m.status = "Could not save requirements: " + err.Error()
			} else {
				m.status = "Saved " + filepath.Base(path)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) renderPythonPackages(c *ui.Context, oi oiColors) {
	m.ensurePythonPackages(false)
	state := m.languageSDKs.packages
	settingsSectionTitle(c, oi, "Packages")
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
		label := state.interpreter
		if label == "" {
			label = "No Python interpreter selected"
		}
		ui.Text(c, label).FontSize(11).TextColor(oi.textMuted).Grow(1).SingleLine().Ellipsis("…")
		ui.Button(c, "Reload").OnClick(func() {
			m.ensurePythonPackages(true)
		})
		ui.Button(c, "Install…").OnClick(func() {
			state := m.languageSDKs.packages
			state.installOpen = !state.installOpen
			m.languageSDKs.packages = state
			m.invalidate()
		})
	})
	if state.installOpen {
		ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
			requirement := state.requirement
			input := ui.TextInput(c, &requirement).Grow(1).
				Placeholder("Package or requirement, for example requests or django==5.1")
			if input.Changed() {
				state := m.languageSDKs.packages
				state.requirement = requirement
				m.languageSDKs.packages = state
			}
			ui.Button(c, "Install Package").OnClick(func() {
				m.startPythonPackageJob("install", strings.TrimSpace(requirement))
			})
		})
	}
	if state.loading {
		ui.Text(c, "Loading installed packages…").FontSize(11).TextColor(oi.textMuted).
			PaddingStart(20).SingleLine()
	}
	if state.err != "" {
		ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
			ui.Text(c, state.err).FontSize(11).TextColor(oi.danger).Grow(1).SingleLine().Ellipsis("…")
			ui.Button(c, "Retry").OnClick(func() { m.ensurePythonPackages(true) })
		})
	}
	if state.job != nil {
		ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
			ui.Text(c, fmt.Sprintf("%s %s…", titleWord(state.job.op), state.job.target)).
				FontSize(11).TextColor(oi.warning).Grow(1).SingleLine().Ellipsis("…")
			ui.Button(c, "Cancel").OnClick(func() { m.cancelPythonPackageJob() })
		})
	}
	if state.jobErr != "" {
		ui.Text(c, state.jobErr).FontSize(11).TextColor(oi.danger).PaddingStart(20).SingleLine().Ellipsis("…")
	}
	if len(state.log) > 0 {
		start := max(0, len(state.log)-8)
		for _, line := range state.log[start:] {
			ui.Text(c, line).FontSize(10).TextColor(oi.textMuted).PaddingStart(20).
				SingleLine().Ellipsis("…")
		}
	}
	query := state.query
	ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		input := ui.TextInput(c, &query).Grow(1).Placeholder("Search packages")
		if input.Changed() {
			state := m.languageSDKs.packages
			state.query = query
			m.languageSDKs.packages = state
		}
		ui.Button(c, "Freeze").OnClick(func() { m.freezePythonPackages() })
	})
	shown := filterPythonPackages(state.packages, query)
	if len(shown) == 0 && state.loaded && state.err == "" {
		message := "No packages installed"
		if strings.TrimSpace(query) != "" {
			message = "No packages match the search"
		}
		ui.Text(c, message).FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine()
	}
	if len(shown) > 0 {
		ui.Row(c).MinHeight(24).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
			ui.Text(c, "Package").FontSize(10).Bold().TextColor(oi.textMuted).Grow(1).SingleLine()
			ui.Text(c, "Version").FontSize(10).Bold().TextColor(oi.textMuted).Width(150).SingleLine()
		})
	}
	for _, pkg := range shown {
		pkg := pkg
		ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
			ui.Text(c, pkg.Name).FontSize(11).TextColor(oi.text).Grow(1).SingleLine().Ellipsis("…")
			ui.Text(c, pkg.Version).FontSize(10).TextColor(oi.textMuted).Width(150).SingleLine().
				Ellipsis("…")
			ui.Button(c, "Upgrade").Width(72).OnClick(func() {
				m.startPythonPackageJob("upgrade", pkg.Name)
			})
			ui.Button(c, "Uninstall").Width(76).OnClick(func() {
				m.startPythonPackageJob("uninstall", pkg.Name)
			})
		})
	}
	if state.frozen == "" {
		return
	}
	settingsSectionTitle(c, oi, "Frozen Requirements")
	ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		ui.Button(c, "Copy").OnClick(func() {
			mygo.Clipboard.WriteText(state.frozen)
			m.status = "Copied frozen requirements"
		})
		ui.Button(c, "Save…").OnClick(func() { m.savePythonRequirements() })
	})
	for _, line := range strings.Split(strings.TrimRight(state.frozen, "\n"), "\n") {
		ui.Text(c, line).FontSize(10).TextColor(oi.textMuted).PaddingStart(20).
			SingleLine().Ellipsis("…")
	}
}

func titleWord(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
