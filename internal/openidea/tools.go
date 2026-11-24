package openidea

import (
	"context"
	"encoding/json"
)

// ToolInfo is one language server, debug adapter or plugin the backend knows,
// with where it is and whether the backend can install it.
type ToolInfo struct {
	Name          string   `json:"name"`
	DisplayName   string   `json:"displayName"`
	Kind          string   `json:"kind"`
	Command       string   `json:"command"`
	Languages     []string `json:"languages"`
	Status        string   `json:"status"` // installed | onPath | custom | missing
	Path          *string  `json:"path"`
	Version       *string  `json:"version"`
	InstallMethod *string  `json:"installMethod"`
	CanInstall    bool     `json:"canInstall"`
	// UnavailableReason says why install is impossible, e.g. no npm.
	UnavailableReason *string `json:"unavailableReason"`
	CanUninstall      bool    `json:"canUninstall"`
	InstallHint       *string `json:"installHint"`
	Homepage          *string `json:"homepage"`
	CustomError       *string `json:"customError"`
	// DefaultArgs is a language server's arguments from its language
	// definition. DefaultInitializationOptions is its default initialization
	// options as JSON text. Alternatives lists interchangeable servers in
	// preference order, including this server when it has fallbacks.
	DefaultArgs                  []string `json:"defaultArgs"`
	DefaultInitializationOptions *string  `json:"defaultInitializationOptions"`
	Alternatives                 []string `json:"alternatives"`
}

// LanguageInfo is one language the backend can assign to a custom server.
type LanguageInfo struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Extensions []string `json:"extensions"`
	Filenames  []string `json:"filenames,omitempty"`
}

// InstallEvent is one message of tools_install: a line of installer output or
// a download progress report.
type InstallEvent struct {
	Type     string   `json:"type"` // line | progress
	Text     string   `json:"text"`
	Fraction *float64 `json:"fraction"`
	Message  string   `json:"message"`
}

// JdkVendor is one JDK distribution the backend's Download JDK dialog offers.
type JdkVendor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// JdkReleases is a vendor's Java feature-version catalogue.
type JdkReleases struct {
	Versions    []uint32 `json:"versions"`
	LTS         []uint32 `json:"lts"`
	Recommended uint32   `json:"recommended"`
}

// Sdk is one JDK the backend discovered or downloaded.
type Sdk struct {
	Path    string  `json:"path"`
	Name    string  `json:"name"`
	Version *string `json:"version"`
	Source  string  `json:"source"`
	Home    *string `json:"home"`
	Extra   *string `json:"extra"`
}

// ToolsList returns every known tool with its state in window's project. The
// list is empty until the window has a project, because tools are resolved
// against its settings.
func (m *Manager) ToolsList(ctx context.Context, window string) ([]ToolInfo, error) {
	raw, err := m.Invoke(ctx, window, "tools_list", nil)
	if err != nil {
		return nil, err
	}
	var list []ToolInfo
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// ToolLanguages lists the languages the Settings › Language Servers picker
// offers for a custom server.
func (m *Manager) ToolLanguages(ctx context.Context, window string) ([]LanguageInfo, error) {
	raw, err := m.Invoke(ctx, window, "languages", nil)
	if err != nil {
		return nil, err
	}
	var list []LanguageInfo
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// ToolInstall installs a tool into the backend's own data directory, streaming
// progress on channel id events. request, which cancel_request cancels, must be
// nonzero; the call returns the tool's new state.
func (m *Manager) ToolInstall(ctx context.Context, window, name string, request, channel uint32) (ToolInfo, error) {
	raw, err := m.Invoke(ctx, window, "tools_install", map[string]any{
		"name":    name,
		"request": request,
		"onEvent": map[string]any{"$channel": channel},
	})
	if err != nil {
		return ToolInfo{}, err
	}
	var info ToolInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return ToolInfo{}, err
	}
	return info, nil
}

// ToolUninstall removes a tool the backend installed.
func (m *Manager) ToolUninstall(ctx context.Context, window, name string) (ToolInfo, error) {
	raw, err := m.Invoke(ctx, window, "tools_uninstall", map[string]any{"name": name})
	if err != nil {
		return ToolInfo{}, err
	}
	var info ToolInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return ToolInfo{}, err
	}
	return info, nil
}

// JdkDownloadVendors lists the distributions the backend can download.
func (m *Manager) JdkDownloadVendors(ctx context.Context, window string) ([]JdkVendor, error) {
	raw, err := m.Invoke(ctx, window, "jdk_download_vendors", nil)
	if err != nil {
		return nil, err
	}
	var list []JdkVendor
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// JdkDownloadReleases lists vendor feature versions, cancellable by request.
func (m *Manager) JdkDownloadReleases(ctx context.Context, window, vendor string, request uint32) (JdkReleases, error) {
	raw, err := m.Invoke(ctx, window, "jdk_download_releases", map[string]any{
		"vendor": vendor, "request": request,
	})
	if err != nil {
		return JdkReleases{}, err
	}
	var releases JdkReleases
	if err := json.Unmarshal(raw, &releases); err != nil {
		return JdkReleases{}, err
	}
	return releases, nil
}

// JdkDownloadLocation is the backend's default JDK destination.
func (m *Manager) JdkDownloadLocation(ctx context.Context, window string) (string, error) {
	raw, err := m.Invoke(ctx, window, "jdk_download_location", nil)
	if err != nil {
		return "", err
	}
	var location string
	if err := json.Unmarshal(raw, &location); err != nil {
		return "", err
	}
	return location, nil
}

// JdkDownload downloads the latest build of vendor/version and streams
// progress on channel.
func (m *Manager) JdkDownload(
	ctx context.Context,
	window, vendor string,
	version uint32,
	location string,
	request, channel uint32,
) (Sdk, error) {
	var target any
	if location != "" {
		target = location
	}
	raw, err := m.Invoke(ctx, window, "jdk_download", map[string]any{
		"vendor": vendor, "version": version, "location": target,
		"request": request, "onEvent": map[string]any{"$channel": channel},
	})
	if err != nil {
		return Sdk{}, err
	}
	var sdk Sdk
	if err := json.Unmarshal(raw, &sdk); err != nil {
		return Sdk{}, err
	}
	return sdk, nil
}

// JdtlsRequiredJava is the Java feature version the jdtls binary this window
// would launch needs, or zero while it is absent or does not report one.
func (m *Manager) JdtlsRequiredJava(ctx context.Context, window string) (uint32, error) {
	raw, err := m.Invoke(ctx, window, "jdtls_required_java", nil)
	if err != nil {
		return 0, err
	}
	var version *uint32
	if err := json.Unmarshal(raw, &version); err != nil {
		return 0, err
	}
	if version == nil {
		return 0, nil
	}
	return *version, nil
}
