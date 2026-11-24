package openidea

import (
	"context"
	"encoding/json"
	"path/filepath"
)

// PluginCapability is the permission record returned with an installed plugin
// and shown by OpenIDEA's consent and plugin details pages.
type PluginCapability struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Risk        string `json:"risk"`
	Optional    bool   `json:"optional"`
	Granted     bool   `json:"granted"`
}

// PluginInstallPreview is what the install consent page displays before a
// package changes the installed plugin set.
type PluginInstallPreview struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Version       string                   `json:"version"`
	Description   string                   `json:"description"`
	Authors       []string                 `json:"authors"`
	HasCode       bool                     `json:"hasCode"`
	Capabilities  []PluginCapability       `json:"capabilities"`
	Contributions []PluginContributionInfo `json:"contributions"`
	Warnings      []string                 `json:"warnings"`
	Replaces      *string                  `json:"replaces"`
	Unsigned      bool                     `json:"unsigned"`
	Publisher     *string                  `json:"publisher"`
	SignerChanged bool                     `json:"signerChanged"`
	NeedsConsent  bool                     `json:"needsConsent"`
}

// PluginContributionInfo is a compact description of one package contribution
// on the install page.
type PluginContributionInfo struct {
	Point string `json:"point"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// PluginConsent is the user's answer to a consent prompt.
type PluginConsent struct {
	Optional     []string `json:"optional"`
	SignerChange bool     `json:"signerChange"`
}

// PluginPackageFile is one UTF-8 package file staged by the native UI.
type PluginPackageFile struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// IntellijThemeFile is a theme.json plus its optional editor scheme from one
// IntelliJ theme plugin.
type IntellijThemeFile struct {
	ID           string  `json:"id"`
	Theme        string  `json:"theme"`
	EditorScheme *string `json:"editorScheme"`
}

// IntellijThemeSource is a theme plugin read from a jar, zip, folder or loose
// .theme.json file.
type IntellijThemeSource struct {
	Name     string              `json:"name"`
	PluginID *string             `json:"pluginId"`
	Version  *string             `json:"version"`
	Vendor   *string             `json:"vendor"`
	Themes   []IntellijThemeFile `json:"themes"`
}

// InstalledIntellijTheme is one theme plugin found in a locally installed
// JetBrains IDE.
type InstalledIntellijTheme struct {
	IDE       string `json:"ide"`
	Plugin    string `json:"plugin"`
	Path      string `json:"path"`
	ThemeID   string `json:"themeId"`
	ThemeName string `json:"themeName"`
	Dark      bool   `json:"dark"`
	Active    bool   `json:"active"`
}

// PluginInstalledIntellijThemes lists the theme plugins available from IDEs
// installed on the backend host.
func (m *Manager) PluginInstalledIntellijThemes(ctx context.Context, window string) ([]InstalledIntellijTheme, error) {
	raw, err := m.Invoke(ctx, window, "plugin_installed_intellij_themes", nil)
	if err != nil {
		return nil, err
	}
	var themes []InstalledIntellijTheme
	if err := decodePluginJSON(raw, &themes); err != nil {
		return nil, err
	}
	return themes, nil
}

// PluginReadIntellijTheme reads one theme plugin before staging it.
func (m *Manager) PluginReadIntellijTheme(ctx context.Context, window, path string) (IntellijThemeSource, error) {
	raw, err := m.Invoke(ctx, window, "plugin_read_intellij_theme", map[string]any{
		"path": filepath.Clean(path),
	})
	if err != nil {
		return IntellijThemeSource{}, err
	}
	var source IntellijThemeSource
	if err := decodePluginJSON(raw, &source); err != nil {
		return IntellijThemeSource{}, err
	}
	return source, nil
}

// PluginStagePackage writes an in-memory package to the backend's install
// staging directory and returns its package folder.
func (m *Manager) PluginStagePackage(
	ctx context.Context,
	window, name string,
	files []PluginPackageFile,
) (string, error) {
	raw, err := m.Invoke(ctx, window, "plugin_stage_package", map[string]any{
		"name":  name,
		"files": files,
	})
	if err != nil {
		return "", err
	}
	var path string
	if err := decodePluginJSON(raw, &path); err != nil {
		return "", err
	}
	return path, nil
}

// PluginPreviewInstall validates a package and returns the consent-page data.
func (m *Manager) PluginPreviewInstall(ctx context.Context, window, path string) (PluginInstallPreview, error) {
	raw, err := m.Invoke(ctx, window, "plugin_preview_install", map[string]any{
		"path": filepath.Clean(path),
	})
	if err != nil {
		return PluginInstallPreview{}, err
	}
	var preview PluginInstallPreview
	if err := decodePluginJSON(raw, &preview); err != nil {
		return PluginInstallPreview{}, err
	}
	return preview, nil
}

// PluginInstallDir installs the validated package at path using consent.
func (m *Manager) PluginInstallDir(
	ctx context.Context,
	window, path string,
	consent PluginConsent,
) (PluginInfo, error) {
	raw, err := m.Invoke(ctx, window, "plugin_install_dir", map[string]any{
		"path":    filepath.Clean(path),
		"consent": consent,
	})
	if err != nil {
		return PluginInfo{}, err
	}
	var info PluginInfo
	if err := decodePluginJSON(raw, &info); err != nil {
		return PluginInfo{}, err
	}
	return info, nil
}

// PluginGrant confirms the capabilities of an installed or project plugin.
func (m *Manager) PluginGrant(
	ctx context.Context,
	window, id string,
	consent PluginConsent,
) (PluginInfo, error) {
	raw, err := m.Invoke(ctx, window, "plugin_grant", map[string]any{
		"id":      id,
		"consent": consent,
	})
	if err != nil {
		return PluginInfo{}, err
	}
	var info PluginInfo
	if err := decodePluginJSON(raw, &info); err != nil {
		return PluginInfo{}, err
	}
	return info, nil
}

// PluginSetEnabled changes whether an installed plugin can load.
func (m *Manager) PluginSetEnabled(ctx context.Context, window, id string, enabled bool) error {
	_, err := m.Invoke(ctx, window, "plugin_set_enabled", map[string]any{
		"id":      id,
		"enabled": enabled,
	})
	return err
}

// PluginUninstall removes an installed package.
func (m *Manager) PluginUninstall(ctx context.Context, window, id string) error {
	_, err := m.Invoke(ctx, window, "plugin_uninstall", map[string]any{"id": id})
	return err
}

// PluginContentTabAction forwards a content editor's toolbar action to the
// owning code plugin.
func (m *Manager) PluginContentTabAction(
	ctx context.Context,
	window, pluginID string,
	action map[string]any,
) (json.RawMessage, error) {
	return m.Invoke(ctx, window, "plugin_content_tab_action", map[string]any{
		"pluginId": pluginID,
		"action":   action,
	})
}
