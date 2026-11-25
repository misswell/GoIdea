package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

const (
	runTargetDefaultChoice = "__goidea_default_run_target__"
	runTargetLocalChoice   = "__goidea_local_run_target__"
)

type remoteRunTargetState struct {
	loaded   bool
	loading  bool
	err      string
	settings openidea.RunTargetSettings
	itemText map[string]runTargetItemText

	plugins        []openidea.RegisteredRunTarget
	pluginsLoaded  bool
	pluginsLoading bool
	pluginsErr     string

	dockerConnections []openidea.DockerConnection
	dockerLoaded      bool
	dockerLoading     bool
	dockerErr         string

	editorOpen     bool
	editorBusy     bool
	editorDirty    bool
	editorErr      string
	selectedTarget int
	draft          openidea.RunTargetSettings

	remedyOpen        bool
	remedyTrust       bool
	remedyTitle       string
	remedyMessage     string
	remedyFingerprint string
	remedyValue       string
	remedySave        bool
	remedyErr         string
	remedyBusy        bool
	remedyConfigID    string
	remedyServer      string
	remedyRetry       func()
}

type runTargetItemText struct {
	ComposeFiles string
	Ports        string
}

func init() {
	_ = registerAction(&Action{
		ID:          "runTargets.configure",
		Title:       "Run Targets…",
		Category:    "Run",
		Description: "Edits SSH, Docker and Compose targets used by run configurations.",
		Keywords:    "run target ssh docker compose remote container interpreter sync",
		When:        remoteProjectOpen,
		Run:         func(m *Model) { m.OpenRunTargetSettings() },
	})
}

func (m *Model) loadRunTargetSettings(ctx context.Context) (openidea.RunTargetSettings, error) {
	raw, err := m.invoke(ctx, "settings_get", nil)
	if err != nil {
		return openidea.RunTargetSettings{}, err
	}
	settings, err := openidea.DecodeRunTargetSettings(raw)
	if err != nil {
		return openidea.RunTargetSettings{}, fmt.Errorf("read run targets: %w", err)
	}
	return settings, nil
}

// ensureRunTargetSettingsLoaded reads project Run Targets once. It is safe to
// call from the Run configuration dialog, the Run Targets editor and the
// Remote Development window.
func (m *Model) ensureRunTargetSettingsLoaded() {
	state := &m.remote.runTargets
	if state.loaded || state.loading || (m.invokeHook == nil && m.mgr == nil) {
		return
	}
	root := m.Root()
	state.loading = true
	state.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		settings, err := m.loadRunTargetSettings(ctx)
		m.post(func() {
			state.loading = false
			if m.Root() != root {
				return
			}
			if err != nil {
				state.err = err.Error()
				m.invalidate()
				return
			}
			state.loaded = true
			state.settings = settings
			if state.editorOpen && !state.editorDirty {
				state.draft = cloneRunTargetSettings(settings)
				state.selectedTarget = firstEditableRunTarget(settings.Targets)
				state.resetItemText()
			}
			m.invalidate()
		})
	}()
}

// loadRegisteredRunTargets refreshes the read-only targets contributed by
// project plugins. Their selectors are opaque and owned by the plugin host.
func (m *Model) loadRegisteredRunTargets(force bool) {
	state := &m.remote.runTargets
	if state.pluginsLoading || (!force && (state.pluginsLoaded || state.pluginsErr != "")) ||
		(m.invokeHook == nil && m.mgr == nil) {
		return
	}
	root := m.Root()
	state.pluginsLoading = true
	state.pluginsErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "plugin_run_targets", nil)
		var targets []openidea.RegisteredRunTarget
		if err == nil {
			targets, err = openidea.DecodeRegisteredRunTargets(raw)
		}
		m.post(func() {
			state.pluginsLoading = false
			if m.Root() != root {
				return
			}
			state.pluginsLoaded = true
			if err != nil {
				state.pluginsErr = err.Error()
			} else {
				state.plugins = targets
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadRunTargetDockerConnections(force bool) {
	state := &m.remote.runTargets
	if state.dockerLoading || (!force && state.dockerLoaded) ||
		(m.invokeHook == nil && m.mgr == nil) {
		return
	}
	root := m.Root()
	state.dockerLoading = true
	state.dockerErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 20*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "docker_settings", nil)
		var settings openidea.DockerSettings
		if err == nil {
			err = json.Unmarshal(raw, &settings)
		}
		m.post(func() {
			state.dockerLoading = false
			if m.Root() != root {
				return
			}
			state.dockerLoaded = true
			if err != nil {
				state.dockerErr = err.Error()
			} else {
				state.dockerConnections = append([]openidea.DockerConnection(nil), settings.Connections...)
			}
			m.invalidate()
		})
	}()
}

func firstEditableRunTarget(targets []openidea.RunTarget) int {
	if len(targets) == 0 {
		return -1
	}
	return 0
}

// OpenRunTargetSettings opens the native equivalent of Settings › Build,
// Execution, Deployment › Run Targets.
func (m *Model) OpenRunTargetSettings() {
	state := &m.remote.runTargets
	state.editorOpen = true
	state.editorErr = ""
	state.editorDirty = false
	if state.loaded {
		state.draft = cloneRunTargetSettings(state.settings)
		state.selectedTarget = firstEditableRunTarget(state.settings.Targets)
		state.resetItemText()
	}
	m.ensureRunTargetSettingsLoaded()
	m.loadRegisteredRunTargets(false)
	m.loadRunTargetDockerConnections(false)
	m.loadRemoteChoices()
	m.invalidate()
}

func (m *Model) closeRunTargetSettings() {
	state := &m.remote.runTargets
	if state.editorBusy {
		return
	}
	state.editorOpen = false
	state.editorErr = ""
	state.editorDirty = false
}

func (m *Model) runTargetSSHConfigForRaw(
	ctx context.Context,
	raw json.RawMessage,
) (string, error) {
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil || cfg == nil {
		return "", err
	}
	targetID := ""
	if cfg.Target != nil {
		targetID = strings.TrimSpace(*cfg.Target)
		if targetID == "" {
			return "", nil
		}
	}
	settings, err := m.loadRunTargetSettings(ctx)
	if err != nil {
		return "", err
	}
	if cfg.Target == nil {
		targetID = strings.TrimSpace(settings.DefaultTarget)
	}
	target := configuredRunTargetByID(settings, targetID)
	if target == nil || target.Kind != openidea.RunTargetSSH {
		return "", nil
	}
	return strings.TrimSpace(target.SSHConfig), nil
}

func (m *Model) offerRunTargetAuth(
	message string,
	configID string,
	retry func(),
) bool {
	configID = strings.TrimSpace(configID)
	if configID == "" || retry == nil {
		return false
	}
	state := &m.remote.runTargets
	state.remedyBusy = false
	state.remedyErr = ""
	state.remedyRetry = retry
	state.remedyConfigID = configID
	state.remedyValue = ""
	switch {
	case sshHostKeyNeedsTrust(message):
		state.remedyOpen = true
		state.remedyTrust = true
		state.remedyTitle = "Trust SSH Host"
		state.remedyMessage = "Trust this exact host key for run target " + configID + "?"
		state.remedyFingerprint = sshHostKeyFingerprint(message)
		if state.remedyFingerprint == "" {
			state.remedyErr = "The SSH host key fingerprint was not included. Run the configuration again."
		}
	case sshAuthenticationNeedsSecret(message):
		state.remedyOpen = true
		state.remedyTrust = false
		state.remedyTitle = "SSH Authentication"
		state.remedyMessage = "Enter the password or key passphrase for " + configID + "."
		state.remedyFingerprint = ""
		state.remedySave = false
	default:
		state.remedyRetry = nil
		return false
	}
	m.invalidate()
	return true
}

func (m *Model) retryRunTargetAuth() {
	state := &m.remote.runTargets
	if state.remedyBusy || state.remedyRetry == nil {
		return
	}
	if state.remedyTrust {
		if state.remedyFingerprint == "" {
			state.remedyErr = "The SSH host key fingerprint is unavailable."
			m.invalidate()
			return
		}
	} else if strings.TrimSpace(state.remedyValue) == "" {
		state.remedyErr = "Enter the password or passphrase."
		m.invalidate()
		return
	}
	state.remedyBusy = true
	state.remedyErr = ""
	configID := state.remedyConfigID
	secret := state.remedyValue
	save := state.remedySave
	trust := state.remedyTrust
	fingerprint := state.remedyFingerprint
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		command, args := "ssh_set_secret", map[string]any{
			"id": configID, "secret": secret, "save": save,
		}
		if trust {
			command, args = "ssh_trust_host", map[string]any{
				"id": configID, "fingerprint": fingerprint,
			}
		}
		_, err := m.invoke(ctx, command, args)
		m.post(func() {
			state.remedyBusy = false
			if err != nil {
				state.remedyErr = err.Error()
				m.invalidate()
				return
			}
			retry := state.remedyRetry
			state.remedyOpen = false
			state.remedyValue = ""
			state.remedyFingerprint = ""
			state.remedyRetry = nil
			m.invalidate()
			retry()
		})
	}()
}

func (m *Model) cancelRunTargetAuth() {
	state := &m.remote.runTargets
	if state.remedyBusy {
		return
	}
	state.remedyOpen = false
	state.remedyValue = ""
	state.remedyFingerprint = ""
	state.remedyErr = ""
	state.remedyRetry = nil
	m.invalidate()
}

func defaultRunTarget() openidea.RunTarget {
	return openidea.RunTarget{
		Kind:          openidea.RunTargetSSH,
		ID:            adapter.UID(),
		SyncBeforeRun: true,
	}
}

func cloneRunTarget(target openidea.RunTarget) openidea.RunTarget {
	target.ComposeFiles = append([]string(nil), target.ComposeFiles...)
	target.Ports = append([]string(nil), target.Ports...)
	return target
}

func cloneRunTargetSettings(settings openidea.RunTargetSettings) openidea.RunTargetSettings {
	out := openidea.RunTargetSettings{DefaultTarget: settings.DefaultTarget}
	out.Targets = make([]openidea.RunTarget, len(settings.Targets))
	for i := range settings.Targets {
		out.Targets[i] = cloneRunTarget(settings.Targets[i])
	}
	return out
}

func (s *remoteRunTargetState) resetItemText() {
	s.itemText = make(map[string]runTargetItemText, len(s.draft.Targets))
	for _, target := range s.draft.Targets {
		s.itemText[target.ID] = runTargetItemText{
			ComposeFiles: strings.Join(target.ComposeFiles, " "),
			Ports:        strings.Join(target.Ports, " "),
		}
	}
}

func (s *remoteRunTargetState) itemTextFor(target openidea.RunTarget) runTargetItemText {
	if s.itemText == nil {
		s.itemText = map[string]runTargetItemText{}
	}
	text, ok := s.itemText[target.ID]
	if !ok {
		text = runTargetItemText{
			ComposeFiles: strings.Join(target.ComposeFiles, " "),
			Ports:        strings.Join(target.Ports, " "),
		}
		s.itemText[target.ID] = text
	}
	return text
}

func (s *remoteRunTargetState) applyItemText() {
	for i := range s.draft.Targets {
		target := &s.draft.Targets[i]
		text, ok := s.itemText[target.ID]
		if !ok {
			continue
		}
		target.ComposeFiles = parseRunTargetList(text.ComposeFiles)
		target.Ports = parseRunTargetList(text.Ports)
	}
}

func parseRunTargetList(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

func (m *Model) selectRunTarget(index int) {
	state := &m.remote.runTargets
	if index < 0 || index >= len(state.draft.Targets) {
		return
	}
	state.selectedTarget = index
	state.editorErr = ""
	m.invalidate()
}

type runTargetChoice struct {
	Value string
	Label string
}

func configuredRunTargetByID(settings openidea.RunTargetSettings, id string) *openidea.RunTarget {
	for i := range settings.Targets {
		if settings.Targets[i].ID == id {
			return &settings.Targets[i]
		}
	}
	return nil
}

func registeredRunTargetByID(targets []openidea.RegisteredRunTarget, id string) *openidea.RegisteredRunTarget {
	for i := range targets {
		if targets[i].ID == id {
			return &targets[i]
		}
	}
	return nil
}

func (s remoteRunTargetState) choices(current string) []runTargetChoice {
	counts := map[string]int{}
	for _, target := range s.settings.Targets {
		counts[target.Label()]++
	}
	for _, target := range s.plugins {
		label := strings.TrimSpace(target.Target.Name)
		if label == "" {
			label = target.Target.ID
		}
		counts[label+" ("+target.PluginName+")"]++
	}
	out := []runTargetChoice{
		{Value: runTargetDefaultChoice, Label: "Default run target"},
		{Value: runTargetLocalChoice, Label: "Local machine"},
	}
	for _, target := range s.settings.Targets {
		label := target.Label()
		if counts[label] > 1 {
			label += " (" + target.ID + ")"
		}
		out = append(out, runTargetChoice{Value: target.ID, Label: label})
	}
	for _, target := range s.plugins {
		label := strings.TrimSpace(target.Target.Name)
		if label == "" {
			label = target.Target.ID
		}
		label += " (" + target.PluginName + ")"
		if counts[label] > 1 && !strings.Contains(label, target.ID) {
			label += " (" + target.ID + ")"
		}
		out = append(out, runTargetChoice{Value: target.ID, Label: label})
	}
	if current != "" && current != runTargetDefaultChoice && current != runTargetLocalChoice &&
		!runTargetChoiceExists(out, current) {
		out = append(out, runTargetChoice{Value: current, Label: current + " (missing)"})
	}
	return out
}

func runTargetChoiceExists(choices []runTargetChoice, value string) bool {
	for _, choice := range choices {
		if choice.Value == value {
			return true
		}
	}
	return false
}

func runTargetChoiceForConfig(target *string) string {
	if target == nil {
		return runTargetDefaultChoice
	}
	if strings.TrimSpace(*target) == "" {
		return runTargetLocalChoice
	}
	return *target
}

func runTargetChoicePointer(choice string) *string {
	switch choice {
	case "", runTargetDefaultChoice:
		return nil
	case runTargetLocalChoice:
		local := ""
		return &local
	default:
		value := choice
		return &value
	}
}

func runTargetChoiceDisplay(choices []runTargetChoice, value string) string {
	for _, choice := range choices {
		if choice.Value == value {
			return choice.Label
		}
	}
	if value == "" {
		return "Local machine"
	}
	return value + " (missing)"
}

func runTargetIDFromRaw(raw json.RawMessage) string {
	var object struct {
		Target *string `json:"target"`
	}
	if json.Unmarshal(raw, &object) != nil || object.Target == nil {
		return ""
	}
	return strings.TrimSpace(*object.Target)
}

func (m *Model) addRunTarget(copyCurrent bool) {
	state := &m.remote.runTargets
	target := defaultRunTarget()
	target.Name = fmt.Sprintf("Target %d", len(state.draft.Targets)+1)
	if copyCurrent && state.selectedTarget >= 0 && state.selectedTarget < len(state.draft.Targets) {
		target = cloneRunTarget(state.draft.Targets[state.selectedTarget])
		target.ID = adapter.UID()
		target.Name += " (copy)"
	} else if len(m.remote.sshConfigs) > 0 {
		target.SSHConfig = m.remote.sshConfigs[0].ID
	}
	state.draft.Targets = append(state.draft.Targets, target)
	state.selectedTarget = len(state.draft.Targets) - 1
	if state.itemText == nil {
		state.itemText = map[string]runTargetItemText{}
	}
	state.itemText[target.ID] = runTargetItemText{
		ComposeFiles: strings.Join(target.ComposeFiles, " "),
		Ports:        strings.Join(target.Ports, " "),
	}
	state.editorDirty = true
	state.editorErr = ""
	m.invalidate()
}

func (m *Model) removeRunTarget() {
	state := &m.remote.runTargets
	i := state.selectedTarget
	if i < 0 || i >= len(state.draft.Targets) {
		return
	}
	removed := state.draft.Targets[i].ID
	state.draft.Targets = append(state.draft.Targets[:i], state.draft.Targets[i+1:]...)
	delete(state.itemText, removed)
	if state.draft.DefaultTarget == removed {
		state.draft.DefaultTarget = ""
	}
	if i >= len(state.draft.Targets) {
		i = len(state.draft.Targets) - 1
	}
	state.selectedTarget = i
	state.editorDirty = true
	state.editorErr = ""
	m.invalidate()
}

func (m *Model) moveRunTarget(delta int) {
	state := &m.remote.runTargets
	i := state.selectedTarget
	j := i + delta
	if i < 0 || i >= len(state.draft.Targets) || j < 0 || j >= len(state.draft.Targets) {
		return
	}
	state.draft.Targets[i], state.draft.Targets[j] = state.draft.Targets[j], state.draft.Targets[i]
	state.selectedTarget = j
	state.editorDirty = true
	m.invalidate()
}

func validateRunTargetSettings(settings openidea.RunTargetSettings) error {
	ids := map[string]bool{}
	for i := range settings.Targets {
		target := &settings.Targets[i]
		target.ID = strings.TrimSpace(target.ID)
		target.Name = strings.TrimSpace(target.Name)
		if target.ID == "" {
			return fmt.Errorf("every run target needs an id")
		}
		if ids[target.ID] {
			return fmt.Errorf("run target id %q is duplicated", target.ID)
		}
		ids[target.ID] = true
		if target.Name == "" {
			return fmt.Errorf("run target %s needs a name", target.ID)
		}
		switch target.Kind {
		case openidea.RunTargetSSH:
			if strings.TrimSpace(target.SSHConfig) == "" {
				return fmt.Errorf("%s needs an SSH configuration", target.Name)
			}
		case openidea.RunTargetDocker:
			if strings.TrimSpace(target.Image) == "" {
				return fmt.Errorf("%s needs an image", target.Name)
			}
		case openidea.RunTargetCompose:
			if strings.TrimSpace(target.Service) == "" {
				return fmt.Errorf("%s needs a Compose service", target.Name)
			}
		default:
			return fmt.Errorf("%s has an unsupported target type", target.Name)
		}
	}
	return nil
}

func (m *Model) saveRunTargetSettings() {
	state := &m.remote.runTargets
	if state.editorBusy || !state.loaded {
		return
	}
	state.applyItemText()
	if err := validateRunTargetSettings(state.draft); err != nil {
		state.editorErr = err.Error()
		m.invalidate()
		return
	}
	targets := state.draft.Targets
	if targets == nil {
		targets = []openidea.RunTarget{}
	}
	targetsJSON, err := json.Marshal(targets)
	if err != nil {
		state.editorErr = err.Error()
		m.invalidate()
		return
	}
	defaultJSON, err := json.Marshal(strings.TrimSpace(state.draft.DefaultTarget))
	if err != nil {
		state.editorErr = err.Error()
		m.invalidate()
		return
	}
	state.editorBusy = true
	state.editorErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		_, err := m.invoke(ctx, "settings_set", map[string]any{
			"scope": "project", "key": "runTargets", "value": string(targetsJSON),
		})
		if err == nil {
			_, err = m.invoke(ctx, "settings_set", map[string]any{
				"scope": "project", "key": "run.defaultTarget", "value": string(defaultJSON),
			})
		}
		var saved openidea.RunTargetSettings
		if err == nil {
			saved, err = m.loadRunTargetSettings(ctx)
		}
		m.post(func() {
			state.editorBusy = false
			if err != nil {
				state.editorErr = "Could not save Run Targets: " + err.Error()
				m.invalidate()
				return
			}
			state.loaded = true
			state.err = ""
			state.settings = saved
			state.draft = cloneRunTargetSettings(saved)
			state.selectedTarget = firstEditableRunTarget(saved.Targets)
			state.resetItemText()
			state.editorDirty = false
			state.editorOpen = false
			m.status = "Run Targets saved"
			m.invalidate()
		})
	}()
}

func (m *Model) runTargetAuthDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote.runTargets
	if !state.remedyOpen {
		return
	}
	open := state.remedyOpen
	dialog := nativeModal(c.Key("run-target-auth"), &open, func() {
		ui.Column(c).Width(580).Padding(18).Gap(10).Children(func() {
			ui.Text(c, state.remedyTitle).FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, state.remedyMessage).FontSize(12).TextColor(p.TextMuted).MaxWidth(540)
			if state.remedyTrust {
				ui.Text(c, state.remedyFingerprint).FontSize(11).TextColor(p.Text).
					SingleLine().Ellipsis("…")
				ui.Text(c, "Compare this fingerprint with the host administrator before trusting it.").
					FontSize(11).TextColor(p.TextMuted).MaxWidth(540)
			} else {
				input := ui.TextInput(c, &state.remedyValue).Height(28).Password().
					Disabled(state.remedyBusy).AutoFocus()
				input.OnSubmit(m.retryRunTargetAuth)
				ui.Checkbox(c, &state.remedySave, "Save for this GoIDEA server").
					Disabled(state.remedyBusy)
				ui.Text(c, "The secret is sent only to the backend's SSH credential store and is never added to run settings.").
					FontSize(10).TextColor(p.TextMuted).MaxWidth(540)
			}
			if state.remedyErr != "" {
				errorState(c, p, state.remedyErr)
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				label := "Connect and Run"
				if state.remedyTrust {
					label = "Trust and Run"
				}
				disabled := state.remedyBusy
				if state.remedyTrust {
					disabled = disabled || state.remedyFingerprint == ""
				} else {
					disabled = disabled || strings.TrimSpace(state.remedyValue) == ""
				}
				ui.Button(c, label).Disabled(disabled).OnClick(m.retryRunTargetAuth)
				ui.Button(c, "Cancel").Disabled(state.remedyBusy).
					OnClick(m.cancelRunTargetAuth)
			})
		})
	})
	if !open || dialog.Dismissed() {
		m.cancelRunTargetAuth()
	}
}

func (m *Model) runTargetSettingsDialog(c *ui.Context, p *ui.Theme) {
	state := &m.remote.runTargets
	if !state.editorOpen {
		return
	}
	open := state.editorOpen
	dialog := nativeModal(c.Key("run-target-settings"), &open, func() {
		ui.Column(c).Width(900).MaxHeight(740).Padding(18).Gap(10).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Run Targets").FontSize(14).Bold().TextColor(p.Text).SingleLine()
				ui.Spacer(c)
				if state.loading {
					ui.Text(c, "Loading…").FontSize(11).TextColor(p.TextMuted).SingleLine()
				}
			})
			m.runTargetDefaultEditor(c, p)
			ui.Row(c).Grow(1).Gap(12).Children(func() {
				m.runTargetListEditor(c, p)
				m.runTargetDetailEditor(c, p)
			})
			if state.editorErr != "" {
				errorState(c, p, state.editorErr)
			}
			if state.err != "" {
				errorState(c, p, state.err)
			}
			ui.Text(c, "Choose a target under “Run on” in a run configuration. "+
				"Interpreters left empty use the target's PATH.").
				FontSize(11).TextColor(p.TextMuted).MaxWidth(840)
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Save").Disabled(state.editorBusy || !state.loaded).
					OnClick(m.saveRunTargetSettings)
				ui.Button(c, "Cancel").Disabled(state.editorBusy).
					OnClick(m.closeRunTargetSettings)
			})
		})
	})
	if !open || dialog.Dismissed() {
		m.closeRunTargetSettings()
	}
}

func (m *Model) runTargetDefaultEditor(c *ui.Context, p *ui.Theme) {
	state := &m.remote.runTargets
	choices := state.choices(state.draft.DefaultTarget)
	labels := make([]string, len(choices))
	for i := range choices {
		labels[i] = choices[i].Label
	}
	selected := runTargetChoiceDisplay(choices, state.draft.DefaultTarget)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Default run target").FontSize(11).TextColor(p.Text).Width(140)
		if selectField := ui.Select(c, &selected, labels); selectField.Changed() {
			for _, choice := range choices {
				if choice.Label == selected {
					state.draft.DefaultTarget = choice.Value
					state.editorDirty = true
					break
				}
			}
		}
	})
}

func (m *Model) runTargetListEditor(c *ui.Context, p *ui.Theme) {
	state := &m.remote.runTargets
	ui.Column(c).Width(250).MinWidth(0).Gap(4).Children(func() {
		ui.Row(c).Gap(4).Children(func() {
			ui.Button(c, "+").Disabled(!state.loaded || state.editorBusy).
				OnClick(func() { m.addRunTarget(false) })
			ui.Button(c, "Copy").Disabled(state.selectedTarget < 0 || state.editorBusy).
				OnClick(func() { m.addRunTarget(true) })
			ui.Button(c, "−").Disabled(state.selectedTarget < 0 || state.editorBusy).
				OnClick(m.removeRunTarget)
			ui.Button(c, "Up").Disabled(state.selectedTarget <= 0 || state.editorBusy).
				OnClick(func() { m.moveRunTarget(-1) })
			ui.Button(c, "Down").Disabled(state.selectedTarget < 0 ||
				state.selectedTarget >= len(state.draft.Targets)-1 || state.editorBusy).
				OnClick(func() { m.moveRunTarget(1) })
		})
		if len(state.draft.Targets) == 0 && len(state.plugins) == 0 {
			emptyState(c, p, "No run targets")
			ui.Text(c, "Add a target with +.").FontSize(11).TextColor(p.TextMuted).SingleLine()
		}
		for _, target := range state.plugins {
			name := strings.TrimSpace(target.Target.Name)
			if name == "" {
				name = target.Target.ID
			}
			ui.Text(c, name).FontSize(11).Bold().TextColor(p.Text).MaxWidth(240).SingleLine()
			ui.Text(c, target.PluginName+" · Read-only").FontSize(10).
				TextColor(p.TextMuted).MaxWidth(240).SingleLine()
		}
		for i, target := range state.draft.Targets {
			i, target := i, target
			label := target.Label()
			if target.ID == state.draft.DefaultTarget {
				label += " · Default"
			}
			ui.Button(c, label).OnClick(func() { m.selectRunTarget(i) })
			ui.Text(c, runTargetSummary(target)).FontSize(10).TextColor(p.TextMuted).
				MaxWidth(240).SingleLine()
		}
		if state.pluginsLoading {
			ui.Text(c, "Loading plugin targets…").FontSize(10).TextColor(p.TextMuted).SingleLine()
		}
		if state.pluginsErr != "" {
			ui.Text(c, state.pluginsErr).FontSize(10).TextColor(p.Danger).MaxWidth(240)
		}
	})
}

func (m *Model) runTargetDetailEditor(c *ui.Context, p *ui.Theme) {
	state := &m.remote.runTargets
	ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
		i := state.selectedTarget
		if i < 0 || i >= len(state.draft.Targets) {
			emptyState(c, p, "Select a run target")
			ui.Text(c, "Plugin-provided targets are read-only and are chosen under Run on.").
				FontSize(11).TextColor(p.TextMuted).MaxWidth(540)
			return
		}
		target := &state.draft.Targets[i]
		ui.Scroll(c).Height(500).Children(func() {
			ui.Column(c).Gap(6).Children(func() {
				runTargetTextField(c, p, state, "Name", &target.Name, "")
				kind := string(target.Kind)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Type").FontSize(11).TextColor(p.Text).Width(140)
					if field := ui.Select(c, &kind,
						[]string{"SSH", "Docker", "Docker Compose"}); field.Changed() {
						switch kind {
						case "Docker":
							target.Kind = openidea.RunTargetDocker
						case "Docker Compose":
							target.Kind = openidea.RunTargetCompose
						default:
							target.Kind = openidea.RunTargetSSH
						}
						state.editorDirty = true
					}
				})
				if target.Kind == openidea.RunTargetSSH {
					m.runTargetSSHFields(c, p, state, target)
				} else {
					m.runTargetContainerFields(c, p, state, target)
				}
				runTargetTextField(c, p, state, "Python interpreter", &target.Python, "python3")
				runTargetTextField(c, p, state, "Node.js interpreter", &target.Node, "node")
				runTargetTextField(c, p, state, "Java executable", &target.Java, "java")
			})
		})
	})
}

func (m *Model) runTargetSSHFields(
	c *ui.Context,
	p *ui.Theme,
	state *remoteRunTargetState,
	target *openidea.RunTarget,
) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "SSH configuration").FontSize(11).TextColor(p.Text).Width(140)
		if len(m.remote.sshConfigs) == 0 {
			if field := ui.TextInput(c, &target.SSHConfig).Grow(1).
				Placeholder("Configuration id"); field.Changed() {
				state.editorDirty = true
			}
			return
		}
		labels := remoteSSHConfigurationChoices(m.remote.sshConfigs)
		selected := target.SSHConfig
		if index := indexOfSSHConfig(m.remote.sshConfigs, target.SSHConfig); index >= 0 {
			selected = labels[index]
		} else if target.SSHConfig != "" {
			labels = append(labels, target.SSHConfig+" (missing)")
			selected = target.SSHConfig + " (missing)"
		}
		if field := ui.Select(c, &selected, labels).Grow(1); field.Changed() {
			if index := slicesIndex(labels, selected); index >= 0 &&
				index < len(m.remote.sshConfigs) {
				target.SSHConfig = m.remote.sshConfigs[index].ID
			} else {
				target.SSHConfig = strings.TrimSuffix(selected, " (missing)")
			}
			state.editorDirty = true
		}
	})
	runTargetTextField(c, p, state, "Project folder", &target.ProjectRoot, "~/project")
	ui.Text(c, "Files are uploaded here and working-directory paths are mapped into it.").
		FontSize(10).TextColor(p.TextMuted).MaxWidth(540)
	if field := ui.Checkbox(c, &target.SyncBeforeRun,
		"Upload changed project files before each run"); field.Changed() {
		state.editorDirty = true
	}
}

func (m *Model) runTargetContainerFields(
	c *ui.Context,
	p *ui.Theme,
	state *remoteRunTargetState,
	target *openidea.RunTarget,
) {
	dockerChoices := runTargetDockerChoices(state.dockerConnections, target.DockerConnection)
	dockerLabels := make([]string, len(dockerChoices))
	for i := range dockerChoices {
		dockerLabels[i] = dockerChoices[i].Label
	}
	selectedDocker := runTargetChoiceDisplay(dockerChoices, target.DockerConnection)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Docker connection").FontSize(11).TextColor(p.Text).Width(140)
		if field := ui.Select(c, &selectedDocker, dockerLabels).Grow(1); field.Changed() {
			for _, choice := range dockerChoices {
				if choice.Label == selectedDocker {
					target.DockerConnection = choice.Value
					state.editorDirty = true
					break
				}
			}
		}
	})
	ui.Text(c, "Only daemons on this machine can mount the project.").
		FontSize(10).TextColor(p.TextMuted).MaxWidth(540)
	if state.dockerLoading {
		ui.Text(c, "Loading Docker connections…").FontSize(10).
			TextColor(p.TextMuted).SingleLine()
	} else if state.dockerErr != "" {
		ui.Text(c, "Docker connections unavailable: "+state.dockerErr).FontSize(10).
			TextColor(p.Danger).MaxWidth(540)
	}
	if target.Kind == openidea.RunTargetCompose {
		text := state.itemTextFor(*target)
		runTargetListField(c, p, state, "Compose files", &text.ComposeFiles,
			"compose.yaml")
		state.itemText[target.ID] = text
		runTargetTextField(c, p, state, "Service", &target.Service, "web")
	} else {
		runTargetTextField(c, p, state, "Image", &target.Image, "python:3.12")
	}
	runTargetTextField(c, p, state, "Container folder", &target.ContainerRoot,
		"/workspaces/project")
	text := state.itemTextFor(*target)
	runTargetListField(c, p, state, "Published ports", &text.Ports, "8000:8000")
	state.itemText[target.ID] = text
	runTargetTextField(c, p, state, "Run options", &target.RunOptions, "--network host")
	if field := ui.Checkbox(c, &target.ImageUser,
		"Run as the image user, not the project folder owner"); field.Changed() {
		state.editorDirty = true
	}
}

func runTargetTextField(
	c *ui.Context,
	p *ui.Theme,
	state *remoteRunTargetState,
	label string,
	value *string,
	placeholder string,
) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(p.Text).Width(140)
		field := ui.TextInput(c, value).Grow(1)
		if placeholder != "" {
			field = field.Placeholder(placeholder)
		}
		if field.Changed() {
			state.editorDirty = true
		}
	})
}

func runTargetListField(
	c *ui.Context,
	p *ui.Theme,
	state *remoteRunTargetState,
	label string,
	value *string,
	placeholder string,
) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(p.Text).Width(140)
		field := ui.TextInput(c, value).Grow(1).Placeholder(placeholder)
		if field.Changed() {
			state.editorDirty = true
		}
	})
}

func runTargetSummary(target openidea.RunTarget) string {
	switch target.Kind {
	case openidea.RunTargetSSH:
		value := target.SSHConfig
		if root := strings.TrimSpace(target.ProjectRoot); root != "" {
			value += " · " + root
		}
		return strings.Trim(value, " ·")
	case openidea.RunTargetDocker:
		return "docker · " + target.Image
	case openidea.RunTargetCompose:
		return "compose · " + target.Service
	default:
		return ""
	}
}

func runTargetDockerChoices(
	connections []openidea.DockerConnection,
	current string,
) []runTargetChoice {
	counts := map[string]int{}
	for _, connection := range connections {
		label := strings.TrimSpace(connection.Name)
		if label == "" {
			label = connection.ID
		}
		counts[label]++
	}
	out := []runTargetChoice{{Value: "", Label: "Docker CLI default"}}
	for _, connection := range connections {
		label := strings.TrimSpace(connection.Name)
		if label == "" {
			label = connection.ID
		}
		if counts[label] > 1 {
			label += " (" + connection.ID + ")"
		}
		out = append(out, runTargetChoice{Value: connection.ID, Label: label})
	}
	if current != "" && !runTargetChoiceExists(out, current) {
		out = append(out, runTargetChoice{Value: current, Label: current + " (missing)"})
	}
	return out
}
