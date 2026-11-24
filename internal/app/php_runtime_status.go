package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type phpRuntimeStatus struct {
	root       string
	target     string
	configured string
	key        string
	gen        uint64
	loading    bool
	loaded     bool
	effective  *string
	located    *openidea.LocatedTool
	xdebug     *openidea.PhpXdebug
	xdebugErr  string
	levels     openidea.PhpLanguageLevels
	levelErr   string
	err        string
}

func phpLanguageLevelOrigin(level *openidea.PhpLanguageLevel) string {
	if level == nil {
		return "No PHP version in Settings or composer.json"
	}
	switch level.Source {
	case "settings":
		return "PHP " + level.Version + ", set in Settings"
	case "platform":
		return "PHP " + level.Version + ", from config.platform.php in composer.json"
	case "require":
		return "PHP " + level.Version + ", the lowest require.php in composer.json"
	default:
		return "PHP " + level.Version
	}
}

func phpShortVersion(level *openidea.PhpLanguageLevel) string {
	if level == nil {
		return ""
	}
	parts := strings.Split(level.Version, ".")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, ".")
}

func phpStatusNearPath(m *Model) string {
	if path := languageTabPath(m, "php"); path != "" {
		return path
	}
	root := m.Root()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "main.php")
}

func phpRuntimeKey(root, target, configured string) string {
	return strings.Join([]string{
		languageSDKPathKey(root),
		pythonInterpreterPathKey(target, root),
		strings.TrimSpace(configured),
	}, "\x00")
}

// ensurePHPRuntimeStatus resolves the configured/default PHP executable,
// checks its Xdebug extension, and reads the effective language level. Xdebug
// and language-level failures are retained separately so the interpreter can
// still be shown when one optional probe fails.
func (m *Model) ensurePHPRuntimeStatus(target string, force bool) {
	root := m.Root()
	target = strings.TrimSpace(target)
	if root == "" || target == "" {
		return
	}
	configured := strings.TrimSpace(nativeSettingsString(
		m,
		settingsPagePHP,
		"languages.php.interpreter",
	))
	key := phpRuntimeKey(root, target, configured)
	state := m.languageSDKs.php
	if !force && state.key == key &&
		(state.loading || state.loaded || state.err != "" ||
			state.xdebugErr != "" || state.levelErr != "") {
		return
	}
	state.root, state.target = root, target
	state.configured, state.key = configured, key
	state.loading = true
	state.loaded = false
	state.err, state.xdebugErr, state.levelErr = "", "", ""
	state.gen++
	generation := state.gen
	m.languageSDKs.php = state
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.loaded = true
		state.err = "The GoIDEA backend is not connected"
		m.languageSDKs.php = state
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 45*time.Second)
		defer cancel()
		raw, effectiveErr := m.invoke(ctx, "sdk_php_effective", nil)
		var effective *string
		if effectiveErr == nil {
			effectiveErr = json.Unmarshal(raw, &effective)
		}
		var located *openidea.LocatedTool
		phpPath := ""
		if effectiveErr == nil {
			if effective != nil {
				phpPath = strings.TrimSpace(*effective)
			}
			if phpPath == "" {
				raw, locateErr := m.invoke(ctx, "sdk_locate_tools", map[string]any{
					"names": []string{"php"}, "near": root,
				})
				var result openidea.LocatedTools
				if locateErr == nil {
					locateErr = json.Unmarshal(raw, &result)
				}
				if locateErr == nil && len(result.Tools) > 0 {
					candidate := result.Tools[0]
					located = &candidate
					if candidate.Path != nil {
						phpPath = strings.TrimSpace(*candidate.Path)
					}
				}
			}
		}
		var xdebug openidea.PhpXdebug
		var xdebugErr error
		if effectiveErr == nil && phpPath != "" {
			raw, xdebugErr = m.invoke(ctx, "sdk_php_xdebug", map[string]any{
				"php": phpPath, "cwd": root,
			})
			if xdebugErr == nil {
				xdebugErr = json.Unmarshal(raw, &xdebug)
			}
		}
		levelPath := target
		raw, levelErr := m.invoke(ctx, "sdk_php_language_level", map[string]any{"path": levelPath})
		var levels openidea.PhpLanguageLevels
		if levelErr == nil {
			levelErr = json.Unmarshal(raw, &levels)
		}
		m.post(func() {
			current := m.languageSDKs.php
			if current.gen != generation || current.key != key || m.Root() != root {
				return
			}
			current.loading = false
			current.loaded = true
			current.effective = effective
			current.located = located
			current.xdebug = nil
			current.levels = levels
			if effectiveErr != nil {
				current.err = effectiveErr.Error()
			}
			if xdebugErr == nil && phpPath != "" {
				current.xdebug = &xdebug
			} else if xdebugErr != nil {
				current.xdebugErr = xdebugErr.Error()
			}
			if levelErr != nil {
				current.levelErr = levelErr.Error()
			}
			m.languageSDKs.php = current
			m.invalidate()
		})
	}()
}

func phpRuntimeExecutableLabel(status phpRuntimeStatus) string {
	if status.xdebug != nil && strings.TrimSpace(status.xdebug.PHP) != "" {
		label := status.xdebug.PHP + " · PHP " + status.xdebug.PHPVersion
		if status.xdebug.Xdebug != nil && strings.TrimSpace(*status.xdebug.Xdebug) != "" {
			label += " · Xdebug " + strings.TrimSpace(*status.xdebug.Xdebug)
		} else {
			label += " · Xdebug not installed"
		}
		return label
	}
	if status.effective != nil && strings.TrimSpace(*status.effective) != "" {
		return *status.effective
	}
	if status.located != nil && status.located.Path != nil {
		label := *status.located.Path
		if status.located.Version != nil && strings.TrimSpace(*status.located.Version) != "" {
			label += " · PHP " + strings.TrimSpace(*status.located.Version)
		}
		return label
	}
	return "php on PATH"
}

func phpLanguageLevelLabel(levels openidea.PhpLanguageLevels) string {
	if levels.Effective != nil {
		label := "PHP " + levels.Effective.Version + " · " + phpLanguageLevelOrigin(levels.Effective)
		if levels.Auto != nil && levels.Effective.Source != "settings" {
			label += " · Auto"
		}
		if levels.Member != nil && strings.TrimSpace(*levels.Member) != "" {
			label += " · " + filepath.Base(*levels.Member)
		}
		return label
	}
	if levels.Auto != nil {
		return "Auto selects " + phpLanguageLevelOrigin(levels.Auto)
	}
	return phpLanguageLevelOrigin(nil)
}

func (m *Model) renderPHPStatus(c *ui.Context, oi oiColors) {
	target := filepath.Join(m.Root(), "main.php")
	m.ensurePHPRuntimeStatus(target, false)
	status := m.languageSDKs.php
	ui.Row(c).MinHeight(30).Gap(8).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		ui.Text(c, phpRuntimeExecutableLabel(status)).FontSize(11).TextColor(oi.textMuted).
			Grow(1).SingleLine().Ellipsis("…")
		ui.Button(c, "Re-check").OnClick(func() {
			m.ensurePHPRuntimeStatus(target, true)
		})
	})
	if status.err != "" {
		ui.Text(c, "Effective PHP: "+status.err).FontSize(11).TextColor(oi.danger).
			PaddingStart(20).SingleLine().Ellipsis("…")
	}
	if status.xdebugErr != "" {
		ui.Text(c, "Xdebug check failed: "+status.xdebugErr).FontSize(11).TextColor(oi.warning).
			PaddingStart(20).SingleLine().Ellipsis("…")
	}
	if status.levelErr != "" {
		ui.Text(c, "Language level check failed: "+status.levelErr).FontSize(11).TextColor(oi.warning).
			PaddingStart(20).SingleLine().Ellipsis("…")
	} else if status.levels.Effective != nil || status.levels.Auto != nil {
		ui.Text(c, phpLanguageLevelLabel(status.levels)).FontSize(11).TextColor(oi.textMuted).
			PaddingStart(20).SingleLine().Ellipsis("…")
	}
}

func init() {
	registerStatusItem(&StatusItem{
		ID: "php.languageLevel", Title: "PHP Language Level", Side: "right", Order: 89,
		Visible: func(m *Model) bool {
			if m.Root() == "" {
				return false
			}
			return languageTabPath(m, "php") != ""
		},
		Render: func(m *Model, c *ui.Context, p *ui.Theme, oi oiColors) {
			target := phpStatusNearPath(m)
			if target == "" {
				return
			}
			m.ensurePHPRuntimeStatus(target, false)
			status := m.languageSDKs.php
			label := "PHP"
			if version := phpShortVersion(status.levels.Effective); version != "" {
				label = "PHP " + version
			}
			tooltip := phpLanguageLevelOrigin(status.levels.Effective)
			tk := oiTokensFor(m, p)
			button := ui.ButtonBase(c.Key("status:php:languageLevel")).
				Height(tk.StatusBarHeight-tk.SpaceL).Padding(0, tk.SpaceM).
				Radius(tk.RadiusS).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if button.Hovered() {
				button.Background(oi.toolbarHover)
			}
			button.Tooltip(tooltip)
			button.Children(func() {
				ui.Text(c, label).FontSize(tk.FontSizeM).TextColor(oi.textMuted).SingleLine()
			})
			button.Menu(m.phpLanguageLevelMenu)
		},
	})
}
