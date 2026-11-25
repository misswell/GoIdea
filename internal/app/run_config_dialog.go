package app

// Run › Edit Configurations… and the "Process is running" rerun confirm,
// ports of OpenIDEA's Run/Debug Configurations dialog (ConfigForm.tsx) and
// confirmRerun (dialogs.tsx): the same fields, validation, override semantics
// (editing a detected config produces a user copy with its id) and save flow
// (run_save_configs with the removed detected ids).

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

// runConfigDlg is Run › Edit Configurations… while it is open.
type runConfigDlg struct {
	open bool
	// items are the working copies of the project's configurations, in the
	// merged order the backend sent.
	items []*openidea.RunConfig
	sel   int
	// OpenIDEA groups the tree by type, then by named folders. Empty folders
	// remain in extraFolders until the dialog closes.
	folderKind      string
	folderName      string
	folderDraft     string
	extraFolders    []runConfigFolder
	collapsedGroups map[string]bool
	collapsedFolder map[string]bool
	// removedDetected are the ids of detected configurations the user deleted;
	// run_save_configs stops the backend from regenerating them.
	removedDetected []string

	// The form's fields, as the editor shows them.
	name         string
	args         string
	cwd          string
	env          string
	envFiles     string
	stdinFile    string
	outputFile   string
	module       string
	policy       string
	runOn        string
	dockerFields map[string]*string
	typeParams   map[string]string
	// temporary preserves the session-only context configurations the chooser
	// and gutter can create. An untouched one stays temporary after OK/Apply.
	temporary map[string]bool
	// baseline and templatesBaseline make Apply/OK no-ops until something
	// actually changed, as OpenIDEA leaves detected configurations detected.
	baseline          map[string]string
	templatesBaseline string
	// selLabel and policyLabel are the Select bindings; both write back on
	// Changed().
	selLabel    string
	policyLabel string
	runOnLabel  string

	errs    adapter.ConfigErrors
	saveErr string

	importing    bool
	importFile   string
	importNotice []string
	importErr    string

	templates      []*openidea.RunConfig
	templatesReady bool
	templateMode   bool
	templateKind   string
	templatesDirty bool
	templateErr    string
	saving         bool
	launchAfter    string
}

// runConfigBaseline stores the ordered raw-preserving form of the
// configurations as the dialog opened. Detection is deliberately omitted:
// applying the untouched form creates a candidate user copy, and this
// comparison decides whether that candidate really differs.
type runConfigBaseline map[string]string

// rerunAsk is the "Process 'x' Is Running" confirm of a rerun.
type rerunAsk struct {
	open    bool
	name    string
	dontAsk bool
	raw     json.RawMessage
}

// OpenRunConfigs opens Run › Edit Configurations… with the project's current
// configurations.
func (m *Model) OpenRunConfigs() {
	items := make([]*openidea.RunConfig, 0, len(m.run.configs)+len(m.run.temporaries)+1)
	present := map[string]bool{}
	for i := range m.run.configs {
		if cfg := cloneRunConfig(&m.run.configs[i]); cfg != nil {
			items = append(items, cfg)
			present[cfg.ID] = true
		}
	}
	temporary := map[string]bool{}
	appendTemporary := func(cfg *openidea.RunConfig) {
		if cfg == nil || present[cfg.ID] {
			return
		}
		if clone := cloneRunConfig(cfg); clone != nil {
			clone.Detected = false
			items = append(items, clone)
			present[clone.ID] = true
			temporary[clone.ID] = true
		}
	}
	for _, cfg := range m.run.temporaries {
		appendTemporary(cfg)
	}
	appendTemporary(m.run.inline)
	appendTemporary(m.run.pendingInline)
	selectedID := m.run.selectedID()
	if len(items) > 0 && selectedID == "" {
		selectedID = items[0].ID
	}
	d := &runConfigDlg{
		open:            true,
		items:           items,
		sel:             0,
		temporary:       temporary,
		collapsedGroups: map[string]bool{},
		collapsedFolder: map[string]bool{},
	}
	if selectedID != "" {
		for i, cfg := range d.items {
			if cfg.ID == selectedID {
				d.sel = i
				break
			}
		}
	}
	if d.sel < 0 || d.sel >= len(d.items) {
		d.sel = 0
	}
	d.baseline = makeRunConfigBaseline(d.items)
	m.runCfgDlg = d
	d.loadForm()
	m.loadRunTemplates(d)
	m.ensureRunTargetSettingsLoaded()
	m.loadRegisteredRunTargets(false)
	m.invalidate()
}

// OpenRunConfigsFor is run.editConfigs with an optional configuration id.
func (m *Model) OpenRunConfigsFor(arg any) {
	m.OpenRunConfigs()
	id, _ := arg.(string)
	if strings.TrimSpace(id) == "" || m.runCfgDlg == nil {
		return
	}
	for i, cfg := range m.runCfgDlg.items {
		if cfg != nil && cfg.ID == id {
			m.runCfgDlg.sel = i
			m.runCfgDlg.loadForm()
			m.invalidate()
			return
		}
	}
	m.status = "Run configuration not found: " + id
	m.invalidate()
}

// loadRunTemplates reads the per-type defaults shown by "Edit configuration
// templates…" and used by Add New Configuration.
func (m *Model) loadRunTemplates(d *runConfigDlg) {
	if d == nil || m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	root := m.Root()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "run_templates", nil)
		var templates []openidea.RunConfig
		if err == nil {
			templates, err = openidea.DecodeRunConfigs(raw)
		}
		items := make([]*openidea.RunConfig, len(templates))
		for i := range templates {
			items[i] = &templates[i]
		}
		m.post(func() {
			if m.runCfgDlg != d || m.Root() != root {
				return
			}
			d.templatesReady = true
			d.templates = items
			d.templatesBaseline = runTemplatesBaseline(items)
			if err != nil {
				d.templateErr = err.Error()
			}
			m.invalidate()
		})
	}()
}

// closeRunConfigs drops the dialog without saving.
func (m *Model) closeRunConfigs() {
	m.runCfgDlg = nil
	m.invalidate()
}

// selected returns the configuration the form edits.
func (d *runConfigDlg) selected() *openidea.RunConfig {
	if d.sel < 0 || d.sel >= len(d.items) {
		return nil
	}
	return d.items[d.sel]
}

func (d *runConfigDlg) selectedID() string {
	if cfg := d.selected(); cfg != nil {
		return cfg.ID
	}
	return ""
}

func makeRunConfigBaseline(configs []*openidea.RunConfig) runConfigBaseline {
	out := runConfigBaseline{}
	ids := make([]string, 0, len(configs))
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		ids = append(ids, cfg.ID)
		out[cfg.ID] = normalizedRunConfig(cfg)
	}
	out["\x00order"] = strings.Join(ids, "\x00")
	return out
}

func runTemplatesBaseline(templates []*openidea.RunConfig) string {
	ids := make([]string, 0, len(templates))
	values := make([]string, 0, len(templates))
	for _, cfg := range templates {
		if cfg == nil {
			continue
		}
		ids = append(ids, cfg.ID)
		values = append(values, normalizedRunConfig(cfg))
	}
	return strings.Join(ids, "\x00") + "\x01" + strings.Join(values, "\x00")
}

func normalizedRunConfig(cfg *openidea.RunConfig) string {
	if cfg == nil {
		return ""
	}
	if form := adapter.BuildToolForm(cfg); form != nil {
		cfg = adapter.ApplyBuildToolForm(cfg, form)
	}
	obj, err := adapter.ConfigObject(cfg)
	if err != nil {
		raw := cfg.Raw()
		if raw == nil {
			raw, _ = json.Marshal(cfg)
		}
		return string(raw)
	}
	delete(obj, "detected")
	raw, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (d *runConfigDlg) configChangedFromBaseline(cfg *openidea.RunConfig) bool {
	if cfg == nil || d.baseline == nil {
		return true
	}
	before, ok := d.baseline[cfg.ID]
	return !ok || before != normalizedRunConfig(cfg)
}

// dirty reports whether OK/Apply has anything to send. The form must be
// applied to the selected draft before this is called.
func (d *runConfigDlg) dirty() bool {
	if d.templatesDirty {
		return true
	}
	current := makeRunConfigBaseline(d.items)
	if current["\x00order"] != d.baseline["\x00order"] {
		return true
	}
	for id, value := range current {
		if id == "\x00order" {
			continue
		}
		if before, ok := d.baseline[id]; !ok || before != value {
			return true
		}
	}
	for id := range d.baseline {
		if id != "\x00order" {
			if _, ok := current[id]; !ok {
				return true
			}
		}
	}
	return false
}

// loadForm fills the fields from the selected configuration.
func (d *runConfigDlg) loadForm() {
	cfg := d.selected()
	if cfg == nil {
		d.name, d.args, d.cwd, d.env, d.envFiles = "", "", "", "", ""
		d.stdinFile, d.outputFile, d.module, d.policy = "", "", "", "automatic"
		d.runOn = runTargetDefaultChoice
		d.selLabel, d.policyLabel, d.runOnLabel = "", "", ""
		d.typeParams = nil
		return
	}
	d.loadDockerFields(cfg)
	d.typeParams = adapter.BuildToolForm(cfg)
	d.name = cfg.Name
	d.args = adapter.FormatArgs(cfg.Args)
	d.cwd = cfg.CwdOr("")
	d.env = adapter.FormatEnv(cfg.Env)
	d.envFiles = strings.Join(cfg.EnvFiles, ";")
	d.stdinFile = ""
	if cfg.StdinFile != nil {
		d.stdinFile = *cfg.StdinFile
	}
	d.outputFile = ""
	if cfg.OutputFile != nil {
		d.outputFile = *cfg.OutputFile
	}
	d.module = ""
	if cfg.Module != nil {
		d.module = *cfg.Module
	}
	d.policy = "automatic"
	if cfg.BuildPolicy != nil && *cfg.BuildPolicy != "" {
		d.policy = *cfg.BuildPolicy
	}
	d.runOn = runTargetChoiceForConfig(cfg.Target)
	d.policyLabel = policyLabelOf(d.policy, cfg.Kind)
	d.selLabel = adapter.KindLabel(cfg.Kind) + " · " + cfg.Name
	if cfg.Detected {
		d.selLabel += " (detected)"
	}
	d.errs = nil
	d.saveErr = ""
}

// policyLabelOf is the label of a configuration's build policy as the Build
// select shows it.
func policyLabelOf(policy, kind string) string {
	for _, o := range adapter.BuildPolicyOptions(kind) {
		if o.Value == policy {
			return o.Label
		}
	}
	return adapter.BuildPolicyOptions(kind)[0].Label
}

// applyForm writes the fields into the selected configuration, as an edit of
// it: a detected config becomes a user copy with its id.
func (d *runConfigDlg) applyForm() {
	cfg := d.selected()
	if cfg == nil {
		return
	}
	edited := *cfg
	edited.Name = d.name
	edited.Args = adapter.ParseArgs(d.args)
	if c := strings.TrimSpace(d.cwd); c == "" {
		edited.Cwd = nil
	} else {
		edited.Cwd = &c
	}
	edited.Env = adapter.ParseEnv(d.env)
	edited.EnvFiles = splitSemicolon(d.envFiles)
	edited.StdinFile = optionalTrimmed(d.stdinFile)
	edited.OutputFile = optionalTrimmed(d.outputFile)
	edited.Target = runTargetChoicePointer(d.runOn)
	if d.typeParams != nil {
		if applied := adapter.ApplyBuildToolForm(&edited, d.typeParams); applied != nil {
			edited = *applied
		}
	}
	if mod := strings.TrimSpace(d.module); mod == "" {
		edited.Module = nil
	} else {
		edited.Module = &mod
	}
	if adapter.BuildPolicyApplies(edited.Kind) && d.policy != "" && d.policy != "automatic" {
		edited.BuildPolicy = &d.policy
	} else {
		edited.BuildPolicy = nil
	}
	if isDockerRunKind(edited.Kind) {
		var err error
		edited, err = d.applyDockerFields(&edited)
		if err != nil {
			d.saveErr = err.Error()
		}
	}
	if d.configChangedFromBaseline(&edited) {
		d.items = adapter.UpsertConfig(d.items, &edited)
	} else {
		edited.Detected = cfg.Detected
		d.items[d.sel] = &edited
	}
	for i, c := range d.items {
		if c.ID == edited.ID {
			d.sel = i
			break
		}
	}
}

// AddRunConfig creates a Shell configuration from its saved template.
func (m *Model) AddRunConfig() {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	m.addRunConfigOfType("shell", d.newConfigFolder("shell"))
}

// AddRunConfigOfType is Add New Configuration's picker action: the selected
// type's saved template (or its blank defaults) becomes a new configuration.
func (m *Model) AddRunConfigOfType(kind string) {
	d := m.runCfgDlg
	if d == nil || kind == "" {
		return
	}
	m.addRunConfigOfType(kind, d.newConfigFolder(kind))
}

func (m *Model) addRunConfigOfType(kind, folder string) {
	d := m.runCfgDlg
	if d == nil || kind == "" {
		return
	}
	d.applyForm()
	cfg, err := adapter.NewConfigOfType(kind, d.items, d.templates, folder)
	if err != nil {
		d.saveErr = err.Error()
		m.invalidate()
		return
	}
	d.items = append(d.items, cfg)
	d.sel = len(d.items) - 1
	d.folderKind, d.folderName = "", ""
	d.templateMode = false
	d.loadForm()
	m.invalidate()
}

// OpenRunConfigTemplates switches the sidebar to per-type configuration
// templates without closing the dialog.
func (m *Model) OpenRunConfigTemplates() {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	d.applyForm()
	d.templateMode = true
	types := adapter.RunConfigTypes(d.items)
	d.templateKind = ""
	if cfg := d.selected(); cfg != nil && cfg.Kind != "compound" {
		d.templateKind = cfg.Kind
	}
	if d.templateKind == "" {
		for _, typ := range types {
			if typ.ID != "compound" {
				d.templateKind = typ.ID
				break
			}
		}
	}
	d.loadTemplateForm()
	m.invalidate()
}

// CloseRunConfigTemplates returns from the template list to configurations.
func (m *Model) CloseRunConfigTemplates() {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	d.applyTemplateForm()
	d.templateMode = false
	d.loadForm()
	m.invalidate()
}

// SelectRunConfigTemplate opens one type's template.
func (m *Model) SelectRunConfigTemplate(kind string) {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	d.applyTemplateForm()
	d.templateKind = kind
	d.loadTemplateForm()
	m.invalidate()
}

func (d *runConfigDlg) currentTemplate() *openidea.RunConfig {
	for _, template := range d.templates {
		if template != nil && template.Kind == d.templateKind {
			return template
		}
	}
	return nil
}

func (d *runConfigDlg) loadTemplateForm() {
	template := d.currentTemplate()
	if template == nil {
		blank, err := adapter.BlankTemplate(d.templateKind)
		if err != nil {
			d.saveErr = err.Error()
			return
		}
		template = blank
		d.templates = append(d.templates, template)
	}
	d.loadDockerFields(template)
	d.typeParams = adapter.BuildToolForm(template)
	d.name = template.Name
	d.args = adapter.FormatArgs(template.Args)
	d.cwd = template.CwdOr("")
	d.env = adapter.FormatEnv(template.Env)
	d.envFiles = strings.Join(template.EnvFiles, ";")
	d.stdinFile = ""
	if template.StdinFile != nil {
		d.stdinFile = *template.StdinFile
	}
	d.outputFile = ""
	if template.OutputFile != nil {
		d.outputFile = *template.OutputFile
	}
	d.module = ""
	if template.Module != nil {
		d.module = *template.Module
	}
	d.policy = "automatic"
	if template.BuildPolicy != nil && *template.BuildPolicy != "" {
		d.policy = *template.BuildPolicy
	}
	d.runOn = runTargetChoiceForConfig(template.Target)
	d.policyLabel = policyLabelOf(d.policy, template.Kind)
	d.selLabel = adapter.KindLabel(template.Kind)
	d.errs = nil
	d.saveErr = ""
}

func (d *runConfigDlg) applyTemplateForm() {
	if !d.templateMode || d.templateKind == "" {
		return
	}
	template := d.currentTemplate()
	if template == nil {
		blank, err := adapter.BlankTemplate(d.templateKind)
		if err != nil {
			d.saveErr = err.Error()
			return
		}
		template = blank
	}
	edited := *template
	edited.ID = adapter.TemplateID(d.templateKind)
	edited.Kind = d.templateKind
	edited.Name = adapter.KindLabel(d.templateKind)
	edited.Detected = false
	edited.Args = adapter.ParseArgs(d.args)
	if c := strings.TrimSpace(d.cwd); c == "" {
		edited.Cwd = nil
	} else {
		edited.Cwd = &c
	}
	edited.Env = adapter.ParseEnv(d.env)
	edited.EnvFiles = splitSemicolon(d.envFiles)
	edited.StdinFile = optionalTrimmed(d.stdinFile)
	edited.OutputFile = optionalTrimmed(d.outputFile)
	edited.Target = runTargetChoicePointer(d.runOn)
	if mod := strings.TrimSpace(d.module); mod == "" {
		edited.Module = nil
	} else {
		edited.Module = &mod
	}
	if adapter.BuildPolicyApplies(edited.Kind) && d.policy != "" && d.policy != "automatic" {
		edited.BuildPolicy = &d.policy
	} else {
		edited.BuildPolicy = nil
	}
	if isDockerRunKind(edited.Kind) {
		var err error
		edited, err = d.applyDockerFields(&edited)
		if err != nil {
			d.saveErr = err.Error()
		}
	}
	if d.typeParams != nil {
		if applied := adapter.ApplyBuildToolForm(&edited, d.typeParams); applied != nil {
			edited = *applied
		}
	}
	found := false
	for i, existing := range d.templates {
		if existing.Kind == edited.Kind {
			d.templates[i] = &edited
			found = true
			break
		}
	}
	if !found {
		d.templates = append(d.templates, &edited)
	}
	d.templatesDirty = runTemplatesBaseline(d.templates) != d.templatesBaseline
}

func splitSemicolon(text string) []string {
	var out []string
	for _, item := range strings.Split(text, ";") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func optionalTrimmed(text string) *string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return &text
}

func (d *runConfigDlg) buildToolTextField(
	c *ui.Context,
	p *ui.Theme,
	label, key, placeholder string,
) {
	value := d.typeParams[key]
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
		input := ui.TextInput(c, &value).Width(520).Placeholder(placeholder)
		if input.Changed() {
			d.typeParams[key] = value
		}
	})
}

func (d *runConfigDlg) renderBuildToolFields(c *ui.Context, p *ui.Theme, kind string) {
	tasksLabel := "Goals"
	tasksPlaceholder := "e.g. spring-boot:run"
	if kind == "gradle" {
		tasksLabel = "Tasks"
		tasksPlaceholder = "e.g. bootRun"
	}
	d.buildToolTextField(c, p, tasksLabel, "tasks", tasksPlaceholder)
	if e := d.errs["tasks"]; e != "" {
		ui.Text(c, e).FontSize(12).TextColor(p.Danger).SingleLine()
	}
	d.buildToolTextField(c, p, "Arguments", "arguments", "Optional build arguments")
	tasks := d.typeParams["tasks"]
	if adapter.BuildToolShowsVMOptions(kind, tasks) {
		vmOptions := d.typeParams["vmOptions"]
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "VM options").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			area := ui.TextArea(c, &vmOptions).Width(520).Height(52).Font("monospace").
				Placeholder("-Xmx1g -Dkey=value")
			if area.Changed() {
				d.typeParams["vmOptions"] = vmOptions
			}
		})
	}
	if adapter.BuildToolShowsActiveProfiles(kind, tasks) {
		d.buildToolTextField(c, p, "Active profiles", "activeProfiles", "e.g. dev,local")
	}
	wrapper := d.typeParams["wrapper"] == "true"
	label := "Use the Maven wrapper"
	if kind == "gradle" {
		label = "Use the Gradle wrapper"
	}
	if check := ui.Checkbox(c, &wrapper, label); check.Changed() {
		d.typeParams["wrapper"] = fmt.Sprintf("%t", wrapper)
	}
}

// DuplicateRunConfig copies the selected configuration with a fresh id.
func (m *Model) DuplicateRunConfig() {
	d := m.runCfgDlg
	if d == nil || d.selected() == nil {
		return
	}
	d.applyForm()
	cfg, err := adapter.DuplicateConfig(d.selected(), d.items)
	if err != nil {
		d.saveErr = err.Error()
		m.invalidate()
		return
	}
	d.items = append(d.items, cfg)
	d.sel = len(d.items) - 1
	d.loadForm()
	m.invalidate()
}

// RemoveRunConfig deletes the selected configuration; a detected one is
// remembered so the backend stops regenerating it.
func (m *Model) RemoveRunConfig() {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	if d.folderKind != "" {
		m.RemoveRunConfigFolder()
		return
	}
	if d.selected() == nil {
		return
	}
	cfg := d.selected()
	if cfg.Detected {
		d.removedDetected = append(d.removedDetected, cfg.ID)
	}
	id := cfg.ID
	isTemporary := d.temporary[id]
	d.items = adapter.RemoveConfig(d.items, id)
	delete(d.temporary, id)
	if isTemporary {
		m.removeRunTemporary(id)
	}
	if d.sel >= len(d.items) {
		d.sel = len(d.items) - 1
	}
	d.loadForm()
	m.invalidate()
}

// SaveRunConfigs validates the edited configuration, persists the user
// configurations and reloads the merged list from the backend.
func (m *Model) SaveRunConfigs() {
	m.saveRunConfigs(true)
}

// RunSelectedRunConfig applies the dialog, closes it and starts the selected
// configuration, as the Run button in OpenIDEA's Edit Configurations footer.
func (m *Model) RunSelectedRunConfig() {
	m.launchSelectedRunConfig("run")
}

// DebugSelectedRunConfig is the footer's Debug button.
func (m *Model) DebugSelectedRunConfig() {
	m.launchSelectedRunConfig("debug")
}

func (m *Model) launchSelectedRunConfig(action string) {
	d := m.runCfgDlg
	if d == nil || d.saving || d.templateMode || d.selected() == nil {
		return
	}
	d.launchAfter = action
	m.saveRunConfigs(true)
}

// ApplyRunConfigs keeps the editor open after saving, as OpenIDEA's Apply.
func (m *Model) ApplyRunConfigs() {
	m.saveRunConfigs(false)
}

func (m *Model) saveRunConfigs(andClose bool) {
	d := m.runCfgDlg
	if d == nil || d.saving {
		return
	}
	if d.templateMode {
		d.applyTemplateForm()
	} else {
		d.applyForm()
	}
	launchAfter := d.launchAfter
	d.launchAfter = ""
	launchID := ""
	if launchAfter != "" && !d.templateMode && d.selected() != nil {
		launchID = d.selectedID()
	}
	if !d.dirty() && !d.templatesDirty {
		if andClose {
			m.closeRunConfigs()
		}
		launchCfg := runConfigLaunchCopy(d.selected())
		if launchCfg != nil && launchAfter != "" {
			m.applyDialogConfigs(d, false)
			m.loadRunConfigs()
			m.launchRunConfigAfterSave(launchAfter, launchCfg)
		}
		return
	}
	for i, cfg := range d.items {
		if cfg.Detected {
			continue
		}
		errs := adapter.ValidateConfig(cfg, d.items)
		if isDockerRunKind(cfg.Kind) {
			delete(errs, "command")
			validateDockerFields(cfg, errs)
		}
		if errs.HasErrors() {
			d.templateMode = false
			d.sel = i
			d.loadForm()
			d.errs = errs
			d.launchAfter = ""
			m.invalidate()
			return
		}
	}
	d.errs = nil
	d.saveErr = ""
	user := adapter.UserConfigs(d.items)
	if len(d.temporary) > 0 {
		filtered := user[:0]
		for _, cfg := range user {
			if d.temporary[cfg.ID] && !d.configChangedFromBaseline(cfg) {
				continue
			}
			filtered = append(filtered, cfg)
		}
		user = filtered
	}
	removed := d.removedDetected
	var templates []map[string]any
	var err error
	if d.templatesDirty {
		templates, err = adapter.TemplatePayload(d.templates)
		if err != nil {
			d.saveErr = err.Error()
			m.invalidate()
			return
		}
	}
	d.saving = true
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		err := adapter.SaveUserConfigs(ctx, javaBackend{m}, user, removed)
		if err == nil && d.templatesDirty {
			_, err = m.invoke(ctx, "run_save_templates", map[string]any{"templates": templates})
		}
		m.post(func() {
			if m.runCfgDlg != d {
				return
			}
			if err != nil {
				d.saveErr = err.Error()
				d.saving = false
				m.invalidate()
				return
			}
			var launchCfg *openidea.RunConfig
			if launchID != "" {
				for _, cfg := range user {
					if cfg.ID == launchID {
						launchCfg = runConfigLaunchCopy(cfg)
						break
					}
				}
				if launchCfg == nil {
					for _, cfg := range d.items {
						if cfg.ID == launchID {
							launchCfg = runConfigLaunchCopy(cfg)
							break
						}
					}
				}
			}
			d.saving = false
			d.templatesDirty = false
			d.templatesBaseline = runTemplatesBaseline(d.templates)
			removed = nil
			d.removedDetected = nil
			for id := range d.temporary {
				savedTemporary := false
				for _, cfg := range user {
					if cfg.ID == id {
						savedTemporary = true
						break
					}
				}
				if savedTemporary {
					delete(d.temporary, id)
					m.removeRunTemporary(id)
				}
			}
			if andClose {
				if launchAfter != "" && launchCfg != nil {
					m.applyDialogConfigs(d, false)
					m.selectRunConfig(launchCfg.ID)
				}
				m.closeRunConfigs()
				m.run.loaded = false
				m.loadRunConfigs()
				if launchAfter != "" && launchCfg != nil {
					m.launchRunConfigAfterSave(launchAfter, launchCfg)
				}
				return
			}
			d.baseline = makeRunConfigBaseline(d.items)
			m.applyDialogConfigs(d, true)
			m.status = "Run configurations applied"
			m.invalidate()
		})
	}()
	m.invalidate()
}

func runConfigLaunchCopy(cfg *openidea.RunConfig) *openidea.RunConfig {
	if cfg == nil {
		return nil
	}
	obj, err := adapter.ConfigObject(cfg)
	if err != nil {
		return nil
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil
	}
	out, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		return nil
	}
	return out
}

func cloneRunConfig(cfg *openidea.RunConfig) *openidea.RunConfig {
	if cfg == nil {
		return nil
	}
	raw := append(json.RawMessage(nil), cfg.Raw()...)
	if len(raw) == 0 {
		var err error
		raw, err = json.Marshal(cfg)
		if err != nil {
			return nil
		}
	}
	out, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		return nil
	}
	return out
}

func (m *Model) applyDialogConfigs(d *runConfigDlg, selectCurrent bool) {
	if d == nil {
		return
	}
	m.run.configs = m.run.configs[:0]
	m.run.temporaries = nil
	selectedID := d.selectedID()
	for _, cfg := range d.items {
		if cfg == nil {
			continue
		}
		if d.temporary[cfg.ID] {
			clone := cloneRunConfig(cfg)
			if clone != nil {
				clone.Detected = false
				m.run.temporaries = append(m.run.temporaries, clone)
			}
		} else {
			m.run.configs = append(m.run.configs, *cfg)
		}
	}
	if selectCurrent {
		if selectedID != "" {
			m.selectRunConfig(selectedID)
		}
	}
	m.run.loaded = true
	m.persistRunTemporaries()
}

func (m *Model) launchRunConfigAfterSave(action string, cfg *openidea.RunConfig) {
	if cfg == nil {
		return
	}
	if action == "debug" {
		m.DebugRunConfig(cfg)
		return
	}
	m.startRunWith(append(json.RawMessage(nil), cfg.Raw()...), cfg.Name)
}

// runConfigForm draws the right side of Edit Configurations. The same form
// edits saved configurations and per-type templates, as upstream's ConfigForm.
func (m *Model) runConfigForm(
	c *ui.Context,
	p *ui.Theme,
	cfg *openidea.RunConfig,
	template bool,
) {
	if cfg == nil {
		return
	}
	fieldError := func(key string) {
		if text := dError(m.runCfgDlg, key); text != "" {
			ui.Text(c, text).FontSize(12).TextColor(p.Danger).SingleLine()
		}
	}

	if !template && cfg.Detected {
		ui.Text(c, "Detected from the project. Changes are saved as a user configuration that overrides it.").
			FontSize(12).TextColor(p.TextMuted).MaxWidth(650)
	}
	if !template {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Name").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			ui.TextInput(c, &m.runCfgDlg.name).Width(330)
			storeAsProject := !cfg.Local
			check := ui.Checkbox(c, &storeAsProject, "Store as project file")
			if check.Changed() {
				cfg.Local = !storeAsProject
			}
		})
		fieldError("name")
	}

	if cfg.Kind != "compound" {
		choices := m.remote.runTargets.choices(m.runCfgDlg.runOn)
		labels := make([]string, 0, len(choices))
		for _, choice := range choices {
			labels = append(labels, choice.Label)
		}
		selected := runTargetChoiceDisplay(choices, m.runCfgDlg.runOn)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Run on").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			if pick := ui.Select(c, &selected, labels); pick.Changed() {
				for _, choice := range choices {
					if choice.Label == selected {
						m.runCfgDlg.runOn = choice.Value
						break
					}
				}
			}
		})
		if m.remote.runTargets.pluginsLoading || m.remote.runTargets.loading {
			ui.Text(c, "Loading run targets…").FontSize(11).TextColor(p.TextMuted).SingleLine()
		}
	}

	ui.Text(c, adapter.KindLabel(cfg.Kind)).FontSize(12).Bold().TextColor(p.Text).SingleLine()
	switch {
	case cfg.Kind == "compound":
		m.runConfigCompoundMembers(c, p, cfg)
	case isDockerRunKind(cfg.Kind):
		m.runCfgDlg.renderDockerFields(c, cfg.Kind, p)
	case cfg.Kind == "maven" || cfg.Kind == "gradle":
		m.runCfgDlg.renderBuildToolFields(c, p, cfg.Kind)
	default:
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Command").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			input := ui.TextInput(c, &cfg.Command).Width(520).Font("monospace").
				Placeholder("e.g. ./gradlew or cargo")
			if input.Hovered() {
				input.Tooltip("Use Command line in the run widget to launch a one-off command")
			}
		})
		fieldError("command")
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Arguments").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			ui.TextInput(c, &m.runCfgDlg.args).Width(520).Font("monospace").
				Placeholder(`Space separated; quote with ' or "`)
		})
	}

	if cfg.Kind != "compound" {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Working directory").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			ui.Text(c, m.Root()+"/").FontSize(12).TextColor(p.TextMuted).SingleLine()
			ui.TextInput(c, &m.runCfgDlg.cwd).Width(360).Font("monospace").
				Placeholder("Project root")
		})
		fieldError("cwd")
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Environment variables").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			ui.TextInput(c, &m.runCfgDlg.env).Width(520).Font("monospace").
				Placeholder("VAR=value; VAR1=value1")
		})
		fieldError("env")
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, `Paths to ".env" files`).FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			ui.TextInput(c, &m.runCfgDlg.envFiles).Width(520).Font("monospace").
				Placeholder(".env; .env.local")
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Redirect input from").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			ui.TextInput(c, &m.runCfgDlg.stdinFile).Width(520).Font("monospace").
				Placeholder("File relative to the project root")
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Save console output to file").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			ui.TextInput(c, &m.runCfgDlg.outputFile).Width(520).Font("monospace").
				Placeholder("File relative to the project root")
		})
		passSystemEnv := cfg.IncludeSystemEnv == nil || *cfg.IncludeSystemEnv
		check := ui.Checkbox(c, &passSystemEnv, "Pass system environment variables")
		if check.Changed() {
			value := passSystemEnv
			cfg.IncludeSystemEnv = &value
		}
		if adapter.IsJVMApplication(cfg.Kind) {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Module").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
				ui.TextInput(c, &m.runCfgDlg.module).Width(330)
				ui.Text(c, "Empty: detected when saving").FontSize(12).TextColor(p.TextMuted).SingleLine()
			})
		}
		m.runConfigDebugAdapter(c, p, cfg)
	}

	if adapter.BuildPolicyApplies(cfg.Kind) {
		opts := adapter.BuildPolicyOptions(cfg.Kind)
		labels := make([]string, 0, len(opts))
		for _, option := range opts {
			labels = append(labels, option.Label)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Build").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
			if selection := ui.Select(c, &m.runCfgDlg.policyLabel, labels); selection.Changed() {
				for _, option := range opts {
					if option.Label == m.runCfgDlg.policyLabel {
						m.runCfgDlg.policy = option.Value
						break
					}
				}
			}
		})
	}

	m.runConfigBeforeLaunch(c, p, cfg)
	m.runConfigOptions(c, cfg)

	if cfg.Kind != "compound" && !template {
		command := strings.TrimSpace(cfg.Command)
		if len(cfg.Args) > 0 {
			command += " " + adapter.FormatArgs(cfg.Args)
		}
		if command == "" {
			command = "—"
		}
		ui.Column(c).Gap(3).Padding(8, 10).Background(oiPaletteOf(p).island).
			Border(1, oiPaletteOf(p).border).Radius(4).Children(func() {
			ui.Text(c, "Command line").FontSize(11).TextColor(p.TextMuted).SingleLine()
			ui.Text(c, command).FontSize(12).Font("monospace").TextColor(p.Text).MaxWidth(650)
		})
	}
}

func dError(d *runConfigDlg, key string) string {
	if d == nil {
		return ""
	}
	return d.errs[key]
}

func (m *Model) runConfigCompoundMembers(
	c *ui.Context,
	p *ui.Theme,
	cfg *openidea.RunConfig,
) {
	ui.Text(c, "Before launch").FontSize(12).TextColor(p.TextMuted).SingleLine()
	if len(cfg.Compound) == 0 {
		ui.Text(c, "No configurations in this compound.").FontSize(12).TextColor(p.TextMuted).SingleLine()
	}
	for index, id := range append([]string(nil), cfg.Compound...) {
		name := id
		for _, candidate := range m.runCfgDlg.items {
			if candidate != nil && candidate.ID == id {
				name = candidate.Name
				break
			}
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, name).FontSize(12).TextColor(p.Text).Grow(1).SingleLine().Ellipsis("…")
			ui.Button(c, "−").Tooltip("Remove from compound").OnClick(func() {
				cfg.Compound = append(cfg.Compound[:index], cfg.Compound[index+1:]...)
				m.invalidate()
			})
		})
	}
	add := ui.Button(c, "Add configuration")
	add.Disabled(len(m.runCfgDlg.items) < 2).
		Menu(func(menu *ui.Menu) { m.compoundMemberMenu(cfg, menu) })
	if errorText := dError(m.runCfgDlg, "compound"); errorText != "" {
		ui.Text(c, errorText).FontSize(12).TextColor(p.Danger).SingleLine()
	}
}

func (m *Model) runConfigDebugAdapter(
	c *ui.Context,
	p *ui.Theme,
	cfg *openidea.RunConfig,
) {
	adapters := []runTargetChoice{
		{Value: "", Label: "None"},
		{Value: "netcoredbg", Label: "netcoredbg (.NET)"},
		{Value: "java-debug", Label: "java-debug (Java)"},
		{Value: "kotlin-debug-adapter", Label: "kotlin-lsp or kotlin-debug-adapter (Kotlin)"},
		{Value: "debugpy", Label: "debugpy (Python)"},
		{Value: "js-debug", Label: "js-debug (Node.js / TypeScript)"},
		{Value: "delve", Label: "Delve (Go)"},
		{Value: "codelldb", Label: "CodeLLDB (Rust, C, C++)"},
		{Value: "lldb-dap", Label: "lldb-dap (C, C++, Swift)"},
		{Value: "gdb", Label: "GDB (C, C++)"},
		{Value: "php-debug", Label: "php-debug (PHP)"},
		{Value: "rdbg", Label: "rdbg (Ruby)"},
		{Value: "bun-dap", Label: "bun-dap (Bun)"},
	}
	current := ""
	if cfg.DebugAdapter != nil {
		current = *cfg.DebugAdapter
	}
	labels := make([]string, 0, len(adapters)+1)
	found := false
	for _, adapter := range adapters {
		labels = append(labels, adapter.Label)
		found = found || adapter.Value == current
	}
	if current != "" && !found {
		adapters = append(adapters, runTargetChoice{Value: current, Label: current + " (no launch template)"})
		labels = append(labels, adapters[len(adapters)-1].Label)
	}
	selected := runTargetChoiceDisplay(adapters, current)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Debug adapter").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
		if pick := ui.Select(c, &selected, labels); pick.Changed() {
			for _, adapter := range adapters {
				if adapter.Label == selected {
					cfg.DebugAdapter = optionalTrimmed(adapter.Value)
					break
				}
			}
		}
	})
}

func (m *Model) runConfigBeforeLaunch(
	c *ui.Context,
	p *ui.Theme,
	cfg *openidea.RunConfig,
) {
	ui.Row(c).Width(680).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Before launch").FontSize(12).Bold().TextColor(p.Text).SingleLine().Grow(1)
		add := ui.Button(c, "Add task")
		add.Menu(func(menu *ui.Menu) { m.beforeLaunchMenu(cfg, menu) })
	})
	if len(cfg.BeforeLaunch) == 0 {
		ui.Text(c, "No tasks").FontSize(12).TextColor(p.TextMuted).SingleLine()
		return
	}
	for index := range cfg.BeforeLaunch {
		task := &cfg.BeforeLaunch[index]
		ui.Row(c).Key(fmt.Sprintf("before-launch:%d:%s", index, task.Kind)).
			Gap(5).AlignItems(ui.Center).Children(func() {
			if task.Kind == "command" {
				ui.TextInput(c, &task.Target).Grow(1).MinWidth(240).Font("monospace").
					Placeholder("Shell command, e.g. make generate")
			} else {
				ui.Text(c, beforeLaunchTaskLabel(*task, m.runCfgDlg.items, m.prefs.ExternalTools)).
					FontSize(12).TextColor(p.Text).Grow(1).SingleLine().Ellipsis("…")
			}
			ui.Button(c, "↑").Tooltip("Move Up").Disabled(index == 0).OnClick(func() {
				moveBeforeLaunchTask(cfg, index, -1)
				m.invalidate()
			})
			ui.Button(c, "↓").Tooltip("Move Down").Disabled(index == len(cfg.BeforeLaunch)-1).OnClick(func() {
				moveBeforeLaunchTask(cfg, index, 1)
				m.invalidate()
			})
			ui.Button(c, "−").Tooltip("Remove Task").OnClick(func() {
				removeBeforeLaunchTask(cfg, index)
				m.invalidate()
			})
		})
	}
	if errorText := dError(m.runCfgDlg, "beforeLaunch"); errorText != "" {
		ui.Text(c, errorText).FontSize(12).TextColor(p.Danger).SingleLine()
	}
}

func (m *Model) runConfigOptions(c *ui.Context, cfg *openidea.RunConfig) {
	ui.Row(c).Gap(14).Wrap().Children(func() {
		allowParallel := cfg.AllowParallel
		if ui.Checkbox(c, &allowParallel, "Allow multiple instances").Changed() {
			cfg.AllowParallel = allowParallel
		}
		emulateTerminal := cfg.EmulateTerminal
		if ui.Checkbox(c, &emulateTerminal, "Emulate terminal in output console").Changed() {
			cfg.EmulateTerminal = emulateTerminal
		}
		showOnStdout := cfg.ShowOnStdout
		if ui.Checkbox(c, &showOnStdout, "Show console on stdout").Changed() {
			cfg.ShowOnStdout = showOnStdout
		}
		showOnStderr := cfg.ShowOnStderr
		if ui.Checkbox(c, &showOnStderr, "Show console on stderr").Changed() {
			cfg.ShowOnStderr = showOnStderr
		}
		openToolWindow := cfg.OpenToolWindow == nil || *cfg.OpenToolWindow
		if ui.Checkbox(c, &openToolWindow, "Open run/debug tool window").Changed() {
			value := openToolWindow
			cfg.OpenToolWindow = &value
		}
		focusToolWindow := cfg.FocusToolWindow
		if ui.Checkbox(c, &focusToolWindow, "Focus run/debug tool window").Changed() {
			cfg.FocusToolWindow = focusToolWindow
		}
		isTest := cfg.IsTest
		if ui.Checkbox(c, &isTest, "Show results in the test runner").Changed() {
			cfg.IsTest = isTest
		}
	})
}

// runConfigsDialog draws Run › Edit Configurations….
func (m *Model) runConfigsDialog(c *ui.Context, p *ui.Theme) {
	d := m.runCfgDlg
	if d == nil {
		return
	}
	open := d.open
	modal := nativeModal(c, &open, func() {
		ui.Column(c).Width(880).Gap(8).Children(func() {
			ui.Text(c, "Run/Debug Configurations").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			if !d.templateMode {
				if len(d.items) == 0 {
					ui.Text(c, "The project has no run configurations yet.").FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Button(c, "Import from .vscode/launch.json").
						Disabled(d.importing || m.run.busy || m.run.running).
						OnClick(func() { m.ImportRunConfigurations(openidea.RunImportVSCode) })
					ui.Button(c, "Import from JetBrains").
						Disabled(d.importing || m.run.busy || m.run.running).
						OnClick(func() { m.ImportRunConfigurations(openidea.RunImportJetBrains) })
					if !d.templatesReady {
						ui.Text(c, "Loading templates…").FontSize(12).TextColor(p.TextMuted).SingleLine()
					}
				})
				if len(d.importNotice) > 0 {
					ui.Column(c).Gap(3).Padding(8, 10).Background(oiPaletteOf(p).island).
						Border(1, oiPaletteOf(p).border).Radius(4).Children(func() {
						title := "Imported from " + d.importFile
						if d.importFile == "" {
							title = "Run configuration import"
						}
						ui.Text(c, title).FontSize(12).Bold().TextColor(p.Text).SingleLine()
						for _, line := range d.importNotice {
							ui.Text(c, line).FontSize(11).TextColor(p.TextMuted).MaxWidth(700)
						}
					})
				}
				if d.importErr != "" {
					ui.Text(c, d.importErr).FontSize(12).TextColor(p.Danger).MaxWidth(700)
				}
			}

			if d.templateMode {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					types := adapter.RunConfigTypes(d.items)
					labels := make([]string, 0, len(types))
					for _, typ := range types {
						labels = append(labels, typ.Label)
					}
					templateLabel := adapter.KindLabel(d.templateKind)
					if sel := ui.Select(c, &templateLabel, labels); sel.Changed() {
						for _, typ := range types {
							if typ.Label == templateLabel {
								m.SelectRunConfigTemplate(typ.ID)
								break
							}
						}
					}
					ui.Button(c, "Back to configurations").OnClick(m.CloseRunConfigTemplates)
					if d.templateErr != "" {
						ui.Text(c, "Templates unavailable").FontSize(12).TextColor(p.Danger).SingleLine().
							Tooltip(d.templateErr)
					}
				})
				ui.Row(c).Height(510).Gap(10).AlignItems(ui.Stretch).Children(func() {
					ui.Column(c).Width(255).Shrink(0).MinHeight(0).
						Border(1, oiPaletteOf(p).border).Radius(4).Background(oiPaletteOf(p).island).
						Children(func() {
							ui.Text(c, "Templates").FontSize(12).Bold().TextColor(p.Text).
								Padding(8, 10).SingleLine()
							ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
								ui.Column(c).Padding(4, 0, 4, 4).Children(func() {
									for _, typ := range adapter.RunConfigTypes(d.items) {
										if typ.ID == "compound" {
											continue
										}
										row := ui.Row(c).Height(27).Padding(6, 8).Radius(3).
											AlignItems(ui.Center).Gap(5).Cursor(ui.CursorPointer)
										if d.templateKind == typ.ID {
											row.Background(oiPaletteOf(p).hover)
										}
										row.Children(func() {
											ui.Text(c, typ.Label).FontSize(12).TextColor(p.Text).SingleLine()
										})
										if row.Clicked() {
											m.SelectRunConfigTemplate(typ.ID)
										}
									}
								})
							})
							ui.Button(c, "Back to configurations").OnClick(m.CloseRunConfigTemplates)
						})
					ui.Scroll(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
						ui.Column(c).Gap(6).Children(func() {
							ui.Text(c, "New configurations of this type start with these values.").
								FontSize(12).TextColor(p.TextMuted).SingleLine()
							cfg := d.currentTemplate()
							if cfg == nil {
								ui.Text(c, "Select a configuration type").FontSize(12).
									TextColor(p.TextMuted).SingleLine()
								return
							}
							m.runConfigForm(c, p, cfg, true)
						})
					})
				})
			} else {
				ui.Row(c).Height(510).Gap(10).AlignItems(ui.Stretch).Children(func() {
					m.runConfigSidebar(c, p)
					ui.Scroll(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
						ui.Column(c).Gap(6).Children(func() {
							if d.folderKind != "" {
								ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
									ui.Text(c, "Folder name").FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
									ui.TextInput(c, &d.folderDraft).Width(330)
									ui.Button(c, "Rename").Disabled(strings.TrimSpace(d.folderDraft) == "").
										OnClick(func() { m.RenameRunConfigFolder(d.folderDraft) })
								})
								return
							}
							cfg := d.selected()
							if cfg == nil {
								ui.Text(c, "Select a configuration").FontSize(12).
									TextColor(p.TextMuted).SingleLine()
								return
							}
							m.runConfigForm(c, p, cfg, false)
						})
					})
				})
			}
			if d.saveErr != "" {
				ui.Text(c, d.saveErr).FontSize(12).TextColor(p.Danger).SingleLine()
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Run").Disabled(d.saving || d.templateMode || d.selected() == nil).
					OnClick(m.RunSelectedRunConfig)
				ui.Button(c, "Debug").Disabled(d.saving || d.templateMode || d.selected() == nil).
					OnClick(m.DebugSelectedRunConfig)
				ui.Button(c, "OK").Disabled(d.saving || d.templateMode && !d.templatesDirty).
					OnClick(m.SaveRunConfigs)
				ui.Button(c, "Apply").Disabled(d.saving).
					OnClick(m.ApplyRunConfigs)
				ui.Button(c, "Cancel").OnClick(m.closeRunConfigs)
			})
		})
	})
	if modal.Dismissed() || !open {
		m.closeRunConfigs()
	}
}

// RerunFailedTests is run.rerunFailed (store.ts rerunFailed): reruns only
// the failed tests of the pane's last run, in the same tab; the process
// must have exited and a derived configuration must be supported.
func (m *Model) RerunFailedTests() {
	if m.run.busy || m.run.running || m.run.awaiting {
		return
	}
	raw, _ := m.rerunTarget()
	if raw == nil {
		m.notifyInfo("run-rerun-failed", "No failed tests to rerun", "No configuration has run in this session")
		m.invalidate()
		return
	}
	base, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		m.notifyInfo("run-rerun-failed", "No failed tests to rerun", err.Error())
		m.invalidate()
		return
	}
	if m.run.tests == nil {
		m.notifyInfo("run-rerun-failed", "No failed tests to rerun", "This configuration's output is not a test report")
		m.invalidate()
		return
	}
	paths := adapter.FailedTestPaths(m.run.tests)
	if configID := pluginTestConfigID(base); configID != "" {
		if len(paths) == 0 {
			m.notifyInfo("run-rerun-failed", "No failed tests to rerun", "There are no failed tests to rerun")
			m.invalidate()
			return
		}
		m.selectPluginFailedTests(base, configID, paths)
		return
	}
	derived := adapter.RerunFailedConfig(base, paths)
	if derived == nil {
		m.notifyInfo("run-rerun-failed", "No failed tests to rerun", "There are no failed tests, or the configuration's framework does not support test filters")
		m.invalidate()
		return
	}
	m.askOrStart(derived.Raw(), derived.Name)
}

// askOrStart starts a configuration, applying the rerun confirm when it is
// already running: upstream startRun's confirm-and-stop flow.
func (m *Model) askOrStart(raw json.RawMessage, name string) {
	live := m.run.running || m.run.awaiting
	if !live {
		m.startRunWith(raw, name)
		return
	}
	if m.confirmRerunEnabled() {
		m.rerunAsk = &rerunAsk{open: true, name: name, raw: append(json.RawMessage(nil), raw...)}
		m.invalidate()
		return
	}
	m.stopAndRerun(raw, name)
}

// rerunTarget is what a rerun launches: the configuration the pane started,
// as saved now (upstream rerunConfig), or the one it ran with when it was
// deleted or never saved (a Run Anything line).
func (m *Model) rerunTarget() (json.RawMessage, string) {
	if id := m.run.lastID; id != "" {
		for i := range m.run.configs {
			if m.run.configs[i].ID == id {
				return m.run.configs[i].Raw(), m.run.configs[i].Name
			}
		}
		if cfg := m.runTemporaryByID(id); cfg != nil {
			return cfg.Raw(), cfg.Name
		}
	}
	if m.run.lastRaw != nil {
		return m.run.lastRaw, m.run.lastName
	}
	return nil, ""
}

// Rerun starts the current configuration again, as OpenIDEA's run.rerun does:
// the configuration as saved now, and — unless it allows parallel runs — a
// confirm before its running instance is stopped and waited for.
func (m *Model) Rerun() {
	if m.run.busy || m.run.loading {
		return
	}
	raw, name := m.rerunTarget()
	if raw == nil {
		return
	}
	m.askOrStart(raw, name)
}

// stopAndRerun stops the running process, waits for its exit, and starts the
// configuration again; a still-busy session gets a second stop, which kills.
func (m *Model) stopAndRerun(raw json.RawMessage, name string) {
	m.StopRun()
	go func() {
		deadline := time.Now().Add(30 * time.Second)
		for {
			var done bool
			m.post(func() {
				done = !m.run.busy && !m.run.running && !m.run.awaiting
			})
			if done {
				break
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		m.post(func() {
			if m.run.busy || m.run.running || m.run.awaiting {
				// The first stop did not finish in time: the second one kills.
				m.StopRun()
			}
			m.startRunWith(raw, name)
		})
	}()
}

// ConfirmRerun answers the "Process is running" dialog with Stop and Rerun.
func (m *Model) ConfirmRerun() {
	ask := m.rerunAsk
	if ask == nil {
		return
	}
	if ask.dontAsk {
		m.setRunRuntimeBool("run.confirmRerun", false)
	}
	raw, name := ask.raw, ask.name
	m.rerunAsk = nil
	m.invalidate()
	m.stopAndRerun(raw, name)
}

// CancelRerun answers the confirm with Cancel: nothing is stopped.
func (m *Model) CancelRerun() {
	m.rerunAsk = nil
	m.invalidate()
}

// rerunConfirmDialog draws the "Process 'x' Is Running" confirm.
func (m *Model) rerunConfirmDialog(c *ui.Context, p *ui.Theme) {
	ask := m.rerunAsk
	if ask == nil {
		return
	}
	open := ask.open
	modal := nativeModal(c, &open, func() {
		ui.Column(c).Width(440).Gap(10).Children(func() {
			ui.Text(c, "Process '"+ask.name+"' Is Running").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, "'"+ask.name+"' does not allow parallel runs.\nStop the running process and run it again?").
				FontSize(12).TextColor(p.Text)
			dontAsk := ask.dontAsk
			if ui.Checkbox(c, &dontAsk, "Don't ask again").Changed() {
				ask.dontAsk = dontAsk
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "Stop and Rerun").OnClick(m.ConfirmRerun)
				ui.Button(c, "Cancel").OnClick(m.CancelRerun)
			})
		})
	})
	if modal.Dismissed() || !open {
		m.CancelRerun()
	}
}
