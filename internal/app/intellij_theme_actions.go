package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func init() {
	for _, action := range []*Action{
		{
			ID:       "plugins.importIntellijTheme",
			Title:    "Import IntelliJ Theme from File…",
			Category: "Plugins",
			Keywords: "intellij jetbrains theme jar zip theme.json color scheme",
			When:     func(m *Model) bool { return m.Root() != "" },
			RunWithArg: func(m *Model, arg any) {
				m.ImportIntellijTheme(pluginPathArg(arg))
			},
		},
		{
			ID:       "plugins.importInstalledIntellijTheme",
			Title:    "Import Theme from JetBrains IDE…",
			Category: "Plugins",
			Keywords: "intellij jetbrains pycharm webstorm android studio theme color scheme migrate",
			When:     func(m *Model) bool { return m.Root() != "" },
			Run:      func(m *Model) { m.ShowIntellijThemes() },
		},
	} {
		_ = registerAction(action)
	}
}

type intellijThemePluginRow struct {
	Key    string
	Plugin string
	IDE    string
	Path   string
	Themes []openidea.InstalledIntellijTheme
	Active bool
}

type intellijThemeChooser struct {
	open     bool
	loading  bool
	err      string
	themes   []openidea.InstalledIntellijTheme
	rows     []intellijThemePluginRow
	selected int
	list     ui.ListState
}

type intellijConsentDialog struct {
	open     bool
	path     string
	preview  openidea.PluginInstallPreview
	accepted map[string]bool
	signer   bool
	busy     bool
	err      string
}

func groupIntellijThemeRows(themes []openidea.InstalledIntellijTheme) []intellijThemePluginRow {
	byKey := map[string]int{}
	rows := make([]intellijThemePluginRow, 0, len(themes))
	for _, theme := range themes {
		key := theme.IDE + "\n" + theme.Path
		index, ok := byKey[key]
		if !ok {
			index = len(rows)
			byKey[key] = index
			rows = append(rows, intellijThemePluginRow{
				Key: key, Plugin: theme.Plugin, IDE: theme.IDE, Path: theme.Path,
			})
		}
		rows[index].Themes = append(rows[index].Themes, theme)
		rows[index].Active = rows[index].Active || theme.Active
	}
	return rows
}

func intellijThemeCountLabel(count int) string {
	if count == 1 {
		return "1 theme"
	}
	return fmt.Sprintf("%d themes", count)
}

func (m *Model) invokeIntellijTheme(cmd string, args any) (json.RawMessage, error) {
	if m == nil || (m.mgr == nil && m.invokeHook == nil) {
		return nil, errors.New("the GoIDEA backend is unavailable")
	}
	parent := m.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	return m.invoke(ctx, cmd, args)
}

// ShowIntellijThemes opens OpenIDEA's chooser of theme plugins found in
// installed JetBrains IDEs. The backend already puts themes in use first.
func (m *Model) ShowIntellijThemes() {
	if m == nil {
		return
	}
	state := &intellijThemeChooser{open: true, loading: true, selected: -1}
	m.intellijThemes = state
	m.status = "Looking for JetBrains themes…"
	m.invalidate()
	go func() {
		raw, err := m.invokeIntellijTheme("plugin_installed_intellij_themes", nil)
		var themes []openidea.InstalledIntellijTheme
		if err == nil {
			err = json.NewDecoder(strings.NewReader(string(raw))).Decode(&themes)
		}
		m.post(func() {
			if m.intellijThemes != state {
				return
			}
			state.loading = false
			if err != nil {
				state.err = err.Error()
				themes = nil
			}
			state.themes = themes
			state.rows = groupIntellijThemeRows(themes)
			if len(state.rows) > 0 {
				state.selected = 0
			} else {
				state.selected = -1
			}
			m.status = ""
			m.invalidate()
		})
	}()
}

func (m *Model) closeIntellijThemes() {
	m.intellijThemes = nil
	m.invalidate()
}

func (m *Model) importChosenIntellijTheme() {
	state := m.intellijThemes
	if state == nil || state.loading || state.selected < 0 || state.selected >= len(state.rows) {
		return
	}
	path := state.rows[state.selected].Path
	m.closeIntellijThemes()
	m.ImportIntellijTheme(path)
}

func (m *Model) chooseIntellijThemeFile() {
	parent := m.win
	start := m.Root()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      parent,
			Title:       "Import IntelliJ Theme",
			ButtonLabel: "Import",
			DefaultPath: start,
			Filters: []mygo.FileFilter{
				{Name: "IntelliJ theme plugin or theme", Extensions: []string{"jar", "zip", "json"}},
			},
		})
		if err != nil {
			m.post(func() { m.fail("", err) })
			return
		}
		if len(paths) == 0 {
			return
		}
		m.post(func() { m.ImportIntellijTheme(paths[0]) })
	}()
}

// ImportIntellijTheme reads the theme source, converts every theme it carries,
// stages the generated package, previews it, and asks for consent when the
// backend reports that it is required. An empty path opens the file picker.
func (m *Model) ImportIntellijTheme(path string) {
	if m == nil {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		m.chooseIntellijThemeFile()
		return
	}
	path = filepath.Clean(path)
	m.status = "Importing IntelliJ theme…"
	m.invalidate()
	go func() {
		sourceRaw, err := m.invokeIntellijTheme("plugin_read_intellij_theme", map[string]any{"path": path})
		if err != nil {
			m.post(func() { m.failIntellijThemeImport(err) })
			return
		}
		var source openidea.IntellijThemeSource
		if err := json.NewDecoder(strings.NewReader(string(sourceRaw))).Decode(&source); err != nil {
			m.post(func() { m.failIntellijThemeImport(err) })
			return
		}
		pkg, err := intellijThemePackageFromSource(source)
		if err != nil {
			m.post(func() { m.failIntellijThemeImport(err) })
			return
		}
		files := make([]openidea.PluginPackageFile, len(pkg.Files))
		for i, file := range pkg.Files {
			files[i] = openidea.PluginPackageFile{Path: file.Path, Text: file.Text}
		}
		stageRaw, err := m.invokeIntellijTheme("plugin_stage_package", map[string]any{
			"name":  pkg.ID,
			"files": files,
		})
		if err != nil {
			m.post(func() { m.failIntellijThemeImport(err) })
			return
		}
		var stagedPath string
		if err := json.NewDecoder(strings.NewReader(string(stageRaw))).Decode(&stagedPath); err != nil {
			m.post(func() { m.failIntellijThemeImport(err) })
			return
		}
		previewRaw, err := m.invokeIntellijTheme("plugin_preview_install", map[string]any{
			"path": filepath.Clean(stagedPath),
		})
		if err != nil {
			m.post(func() { m.failIntellijThemeImport(err) })
			return
		}
		var preview openidea.PluginInstallPreview
		if err := json.NewDecoder(strings.NewReader(string(previewRaw))).Decode(&preview); err != nil {
			m.post(func() { m.failIntellijThemeImport(err) })
			return
		}
		if preview.NeedsConsent {
			m.post(func() {
				m.intellijConsent = newIntellijConsent(stagedPath, preview)
				m.status = ""
				m.invalidate()
			})
			return
		}
		optional := make([]string, 0, len(preview.Capabilities))
		for _, capability := range preview.Capabilities {
			if capability.Optional && capability.Granted {
				optional = append(optional, capability.ID)
			}
		}
		m.startIntellijThemeInstall(stagedPath, openidea.PluginConsent{Optional: optional}, nil)
	}()
}

func (m *Model) failIntellijThemeImport(err error) {
	m.status = ""
	m.fail("", fmt.Errorf("Could not import the IntelliJ theme: %w", err))
}

func newIntellijConsent(path string, preview openidea.PluginInstallPreview) *intellijConsentDialog {
	accepted := map[string]bool{}
	for _, capability := range preview.Capabilities {
		if capability.Optional && (capability.Granted || capability.Risk == "low") {
			accepted[capability.ID] = true
		}
	}
	return &intellijConsentDialog{
		open: true, path: path, preview: preview, accepted: accepted, signer: preview.SignerChanged,
	}
}

func (m *Model) closeIntellijConsent() {
	if m.intellijConsent != nil && m.intellijConsent.busy {
		return
	}
	m.intellijConsent = nil
	m.invalidate()
}

func (m *Model) installIntellijThemeConsent() {
	state := m.intellijConsent
	if state == nil || state.busy {
		return
	}
	capabilities := sortedIntellijCapabilities(state.preview.Capabilities)
	optional := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability.Optional && state.accepted[capability.ID] {
			optional = append(optional, capability.ID)
		}
	}
	state.busy = true
	state.err = ""
	m.invalidate()
	m.startIntellijThemeInstall(state.path, openidea.PluginConsent{
		Optional: optional, SignerChange: state.signer,
	}, func(info openidea.PluginInfo, err error) {
		if m.intellijConsent != state {
			return
		}
		switch {
		case err != nil:
			state.busy = false
			state.err = err.Error()
		case info.Status == "failed":
			state.busy = false
			if info.Error != nil && *info.Error != "" {
				state.err = *info.Error
			} else {
				state.err = "The plugin failed to start."
			}
		default:
			m.intellijConsent = nil
			m.status = fmt.Sprintf("Installed %s %s. Choose it in Settings › Appearance.", info.Name, info.Version)
		}
		m.invalidate()
	})
}

func (m *Model) startIntellijThemeInstall(
	path string,
	consent openidea.PluginConsent,
	done func(openidea.PluginInfo, error),
) {
	go func() {
		raw, err := m.invokeIntellijTheme("plugin_install_dir", map[string]any{
			"path": filepath.Clean(path), "consent": consent,
		})
		var info openidea.PluginInfo
		if err == nil {
			err = json.NewDecoder(strings.NewReader(string(raw))).Decode(&info)
		}
		m.post(func() {
			if done != nil {
				done(info, err)
				return
			}
			switch {
			case err != nil:
				m.failIntellijThemeImport(err)
			case info.Status == "failed":
				message := "the plugin failed to start"
				if info.Error != nil && *info.Error != "" {
					message = *info.Error
				}
				m.failIntellijThemeImport(errors.New(message))
			default:
				m.status = fmt.Sprintf("Installed %s %s. Choose it in Settings › Appearance.", info.Name, info.Version)
				m.invalidate()
			}
		})
	}()
}

func sortedIntellijCapabilities(capabilities []openidea.PluginCapability) []openidea.PluginCapability {
	out := append([]openidea.PluginCapability(nil), capabilities...)
	order := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := order[out[i].Risk], order[out[j].Risk]
		if left != right {
			return left < right
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func intellijCapabilityDescription(capability openidea.PluginCapability) string {
	switch {
	case capability.ID == "database.sources":
		return "Manage its own database sources and store their passwords in trusted projects"
	case capability.ID == "ide.commands":
		return "Run any IDE action or other plugins' commands as if you chose them, including running programs, committing and pushing"
	case capability.ID == "net:ask":
		return "Ask to connect to a host that is not on the list the first time it tries; you allow or refuse each host, and can revoke the allowed ones"
	case strings.HasPrefix(capability.ID, "net.http:"):
		return fmt.Sprintf("Connect to %s over unencrypted HTTP (from the machine the project is on)", strings.TrimPrefix(capability.ID, "net.http:"))
	case strings.HasPrefix(capability.ID, "ipc:"):
		return fmt.Sprintf("Connect to local socket %s — equivalent to running arbitrary code on the project machine",
			strings.TrimPrefix(capability.ID, "ipc:"))
	default:
		return capability.Description
	}
}

func intellijRiskColor(risk string, p *ui.Theme) ui.Color {
	switch risk {
	case "high":
		return p.Danger
	case "medium":
		return p.Warning
	default:
		return p.TextMuted
	}
}

func intellijRiskLabel(risk string) string {
	switch risk {
	case "high":
		return "High"
	case "medium":
		return "Medium"
	default:
		return "Low"
	}
}

func (m *Model) intellijThemesDialog(c *ui.Context, p *ui.Theme) {
	state := m.intellijThemes
	if state == nil {
		return
	}
	open := state.open
	dialog := nativeModal(c, &open, func() {
		ui.Column(c).Width(640).Gap(10).Children(func() {
			ui.Text(c, "Import Theme from JetBrains IDE").FontSize(16).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c,
				"Theme plugins installed in the JetBrains IDEs on this computer. Importing one installs it with all its themes; choose the theme to use in Settings › Appearance.").
				FontSize(12).TextColor(p.TextMuted).Wrap()

			switch {
			case state.loading:
				ui.Text(c, "Looking for JetBrains themes…").FontSize(12).TextColor(p.TextMuted)
			case state.err != "":
				ui.Text(c, state.err).FontSize(12).TextColor(p.Danger).Wrap()
			case len(state.rows) == 0:
				ui.Text(c,
					"No theme plugins found in JetBrains IDEs on this computer. Choose a theme plugin downloaded from the JetBrains Marketplace (.jar or .zip) or a .theme.json file instead.").
					FontSize(12).TextColor(p.TextMuted).Wrap()
			default:
				if state.selected < 0 || state.selected >= len(state.rows) {
					state.selected = 0
				}
				state.list.Selected = &state.selected
				list := ui.List(c, &state.list, len(state.rows), func(i int) {
					row := state.rows[i]
					element := ui.Row(c.Key("plugins.intellijThemes:"+row.Key)).
						Height(46).Padding(4, 10).Gap(8).AlignItems(ui.Center).
						Cursor(ui.CursorPointer)
					if i == state.selected || element.Hovered() {
						element.Background(p.SurfaceHover)
					}
					element.OnClick(func() {
						state.selected = i
						m.invalidate()
					})
					element.Children(func() {
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Text(c, row.Plugin).FontSize(13).Bold().TextColor(p.Text).
									SingleLine().Ellipsis("…")
								if row.Active {
									ui.Text(c, "In use").FontSize(11).TextColor(p.Accent).SingleLine()
								}
							})
							ui.Text(c, intellijThemeCountLabel(len(row.Themes))+" · "+row.IDE).
								FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
						})
						names := make([]string, 0, len(row.Themes))
						for _, theme := range row.Themes {
							names = append(names, theme.ThemeName)
						}
						ui.Text(c, strings.Join(names, ", ")).Width(250).
							FontSize(12).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
					})
				}).Height(300).AutoFocus()
				if list.Submitted() {
					m.importChosenIntellijTheme()
				}
			}

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Choose File…").Disabled(state.loading).Clicked() {
					m.closeIntellijThemes()
					m.chooseIntellijThemeFile()
				}
				if ui.Button(c, "Cancel").Clicked() {
					m.closeIntellijThemes()
				}
				chosen := state.selected >= 0 && state.selected < len(state.rows)
				if ui.Button(c, "Import").Disabled(state.loading || !chosen).Clicked() {
					m.importChosenIntellijTheme()
				}
			})
		})
	})
	if dialog.Dismissed() || !open {
		m.closeIntellijThemes()
	}
}

func (m *Model) intellijConsentDialog(c *ui.Context, p *ui.Theme) {
	state := m.intellijConsent
	if state == nil {
		return
	}
	open := state.open
	capabilities := sortedIntellijCapabilities(state.preview.Capabilities)
	title := fmt.Sprintf("Install %s %s?", state.preview.Name, state.preview.Version)
	dialog := nativeModal(c, &open, func() {
		ui.Column(c).Width(520).Gap(10).Children(func() {
			ui.Text(c, title).FontSize(16).Bold().TextColor(p.Text).SingleLine()
			if state.preview.Unsigned {
				ui.Text(c, "This package is not signed. Only continue if you trust where it came from.").
					FontSize(12).TextColor(p.Warning).Wrap()
			}
			if state.preview.SignerChanged {
				ui.Text(c,
					"This version is signed with a different key than the installed one, and the publisher's earlier key doesn't vouch for it. Only continue if you know the publisher changed keys.").
					FontSize(12).TextColor(p.Warning).Wrap()
			}
			if len(capabilities) == 0 {
				ui.Text(c, "This plugin needs no special permissions.").FontSize(12).TextColor(p.TextMuted)
			} else {
				ui.Scroll(c).Height(260).Children(func() {
					ui.Column(c).Gap(10).Children(func() {
						for _, capability := range capabilities {
							ui.Column(c).Gap(3).Children(func() {
								ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
									if capability.Optional {
										on := state.accepted[capability.ID]
										if ui.Checkbox(c, &on, "").Changed() {
											state.accepted[capability.ID] = on
											m.invalidate()
										}
									} else {
										ui.Text(c, "•").Width(16).TextColor(p.TextMuted)
									}
									ui.Text(c, capability.ID).FontSize(12).TextColor(p.Text).
										SingleLine().Ellipsis("…")
									ui.Text(c, intellijRiskLabel(capability.Risk)).FontSize(11).
										TextColor(intellijRiskColor(capability.Risk, p))
								})
								ui.Text(c, intellijCapabilityDescription(capability)).FontSize(12).
									TextColor(p.TextMuted).Wrap()
							})
						}
					})
				})
				high := false
				for _, capability := range capabilities {
					if capability.Risk == "high" && (!capability.Optional || state.accepted[capability.ID]) {
						high = true
					}
				}
				if high {
					ui.Text(c, "High-risk permissions are equivalent to running arbitrary code on this machine.").
						FontSize(12).TextColor(p.Danger).Wrap()
				}
			}
			for _, warning := range state.preview.Warnings {
				ui.Text(c, warning).FontSize(12).TextColor(p.TextMuted).Wrap()
			}
			if state.err != "" {
				ui.Text(c, state.err).FontSize(12).TextColor(p.Danger).Wrap()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Disabled(state.busy).Clicked() {
					m.closeIntellijConsent()
				}
				if ui.Button(c, "Install").Disabled(state.busy).Clicked() {
					m.installIntellijThemeConsent()
				}
			})
		})
	})
	if dialog.Dismissed() || !open {
		m.closeIntellijConsent()
	}
}
