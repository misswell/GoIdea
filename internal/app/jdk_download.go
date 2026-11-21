package app

// Download JDK ports OpenIDEA's jdk_download action and DownloadJdkDialog:
// the hidden one-click action downloads a requested Java version in the
// background, while the Java settings dialog selects a vendor, version and
// destination before it starts. Both use the backend as the downloader and
// JDK detector; the native UI only renders their state.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	defaultJDKVendor = "temurin"
	jdkDownloadTask  = "jdk-download"
)

type downloadJdkArgs struct {
	Version        uint32 `json:"version"`
	LanguageServer bool   `json:"languageServer"`
}

// jdkDialogState is Download JDK while it is open. A download itself lives on
// Model so the hidden action can show the same progress in Background Tasks
// without opening the dialog.
type jdkDialogState struct {
	open bool

	vendor      string
	vendorLabel string
	vendors     []openidea.JdkVendor

	versions       []uint32
	lts            []uint32
	recommended    uint32
	initialVersion uint32
	version        uint32
	versionLabel   string
	loading        bool

	location string
	progress *float64
	message  string
	err      string

	seq             uint64
	releasesRequest uint32
}

func downloadJDKCommands() []appCommand {
	return []appCommand{
		{
			"java.downloadJdk", "Download JDK", "Java", "",
			"java jdk download language server toolchain",
			func(m *Model) { m.OpenSettingsPage(settingsPageJava) },
		},
	}
}

func downloadJDKActionState(a *Action) {
	if a.ID != "java.downloadJdk" {
		return
	}
	a.RunWithArg = func(m *Model, arg any) {
		args, ok := parseDownloadJDKArgs(arg)
		if !ok {
			m.OpenSettingsPage(settingsPageJava)
			return
		}
		m.StartJDKDownload(defaultJDKVendor, args.Version, "", args.LanguageServer)
	}
}

// parseDownloadJDKArgs accepts OpenIDEA's {version, languageServer} action
// argument. A version is required for the one-click form; an empty or invalid
// argument belongs to the no-argument Java Settings path instead.
func parseDownloadJDKArgs(arg any) (downloadJdkArgs, bool) {
	if arg == nil {
		return downloadJdkArgs{}, false
	}
	raw, err := json.Marshal(arg)
	if err != nil {
		return downloadJdkArgs{}, false
	}
	var args downloadJdkArgs
	if err := json.Unmarshal(raw, &args); err != nil ||
		args.Version == 0 ||
		!containsJSONObject(arg) {
		return downloadJdkArgs{}, false
	}
	return args, true
}

func containsJSONObject(arg any) bool {
	switch arg.(type) {
	case map[string]any, json.RawMessage:
		return true
	}
	raw, err := json.Marshal(arg)
	if err != nil {
		return false
	}
	return len(raw) > 0 && raw[0] == '{'
}

type jdkRequirement struct {
	version        uint32
	languageServer bool
}

var jdkRequirementPattern = regexp.MustCompile(`needs JDK ([0-9]+) or newer`)

// requiredJDK extracts the feature version and target from the backend's
// launch and sync errors. A language-server failure restarts jdtls on the new
// JDK; a project failure only makes the downloaded SDK available to detection.
func requiredJDK(message string) (jdkRequirement, bool) {
	match := jdkRequirementPattern.FindStringSubmatch(message)
	if len(match) != 2 {
		return jdkRequirement{}, false
	}
	version, err := strconv.ParseUint(match[1], 10, 32)
	if err != nil || version == 0 {
		return jdkRequirement{}, false
	}
	lower := strings.ToLower(message)
	return jdkRequirement{
		version: uint32(version),
		languageServer: strings.Contains(lower, "jdtls") ||
			strings.Contains(lower, "language server jdk"),
	}, true
}

// acceptLanguageServerEvent offers the same one-click JDK repair as
// OpenIDEA's languageServer listener. Prompt keys include the executable name
// and required version, so a retry on the same broken setup stays quiet while
// a newer requirement is still reported.
func (m *Model) acceptLanguageServerEvent(e openidea.LanguageServerEvent) {
	wantsMissingPrompt := m.acceptLanguageServerStatus(e)
	if e.Status == "missing" && wantsMissingPrompt {
		m.promptMissingLanguageTool(e.Name)
	} else if e.Status == "missing" {
		return
	}
	if e.Status != "misconfigured" || e.Message == nil {
		return
	}
	need, ok := requiredJDK(*e.Message)
	if !ok {
		return
	}
	name := strings.TrimSpace(e.Name)
	if name == "" {
		name = "Language server"
	}
	prompt := name + ":" + strconv.FormatUint(uint64(need.version), 10)
	if m.jdkPrompted == nil {
		m.jdkPrompted = make(map[string]bool)
	}
	if m.jdkPrompted[prompt] {
		return
	}
	m.jdkPrompted[prompt] = true
	version := need.version
	languageServer := need.languageServer
	m.notifySticky(
		"jdk-required-"+name,
		"warning",
		fmt.Sprintf("%s needs JDK %d to run", name, version),
		*e.Message,
		false,
		notifyAction{
			label: fmt.Sprintf("Download JDK %d", version),
			run: func(m *Model) {
				m.StartJDKDownload(defaultJDKVendor, version, "", languageServer)
			},
		},
		notifyAction{
			label: "Java Settings…",
			run:   func(m *Model) { m.OpenSettingsPage(settingsPageJava) },
		},
	)
}

// OpenJDKDownloadDialog opens Download JDK from a Java settings JDK field.
// initialVersion preselects the minimum version the language server needs.
func (m *Model) OpenJDKDownloadDialog(initialVersion uint32) {
	m.jdkDialog = jdkDialogState{
		open:           true,
		vendor:         defaultJDKVendor,
		vendorLabel:    "Eclipse Temurin",
		initialVersion: initialVersion,
		version:        initialVersion,
		seq:            m.jdkDialog.seq + 1,
	}
	m.jdkDialog.syncVersionLabel()
	m.loadJDKDialogChoices()
	m.loadJDKReleases(defaultJDKVendor)
	m.invalidate()
}

func (m *Model) closeJDKDownloadDialog() {
	d := &m.jdkDialog
	if !d.open {
		return
	}
	d.open = false
	d.seq++
	if d.releasesRequest != 0 {
		m.cancelJDKRequest(d.releasesRequest)
		d.releasesRequest = 0
	}
	if m.jdkDownloadBusy {
		m.CancelJDKDownload()
	}
	m.invalidate()
}

// loadJDKDialogChoices fills the vendor and location fields independently of
// the version catalogue, as the upstream dialog shows Temurin immediately and
// fills the backend's destination when it answers.
func (m *Model) loadJDKDialogChoices() {
	if m.mgr == nil && m.invokeHook == nil {
		m.jdkDialog.err = "The GoIDEA backend is not connected"
		return
	}
	seq := m.jdkDialog.seq
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()

		var vendors []openidea.JdkVendor
		if raw, err := m.invoke(ctx, "jdk_download_vendors", nil); err == nil {
			_ = json.Unmarshal(raw, &vendors)
		}
		location := ""
		locationErr := error(nil)
		if raw, err := m.invoke(ctx, "jdk_download_location", nil); err != nil {
			locationErr = err
		} else {
			_ = json.Unmarshal(raw, &location)
		}

		m.post(func() {
			d := &m.jdkDialog
			if !d.open || d.seq != seq {
				return
			}
			if len(vendors) > 0 {
				d.vendors = append([]openidea.JdkVendor(nil), vendors...)
				found := false
				for _, vendor := range vendors {
					if vendor.ID == d.vendor {
						d.vendorLabel = vendor.Name
						found = true
						break
					}
				}
				if !found {
					d.vendor = vendors[0].ID
					d.vendorLabel = vendors[0].Name
				}
			}
			if locationErr == nil && d.location == "" {
				d.location = location
			}
			m.invalidate()
		})
	}()
}

func (m *Model) loadJDKReleases(vendor string) {
	if m.mgr == nil && m.invokeHook == nil {
		m.jdkDialog.err = "The GoIDEA backend is not connected"
		return
	}
	if request := m.jdkDialog.releasesRequest; request != 0 {
		m.cancelJDKRequest(request)
	}
	request := m.nextRequest()
	seq := m.jdkDialog.seq
	m.jdkDialog.releasesRequest = request
	m.jdkDialog.loading = true
	m.jdkDialog.err = ""
	m.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "jdk_download_releases", map[string]any{
			"vendor": vendor, "request": request,
		})
		if err != nil {
			if openidea.ErrorKind(err) == "cancelled" {
				return
			}
			m.post(func() {
				d := &m.jdkDialog
				if !d.open || d.seq != seq || d.vendor != vendor || d.releasesRequest != request {
					return
				}
				d.loading = false
				d.err = "Cannot list the Java versions: " + err.Error()
				m.invalidate()
			})
			return
		}
		var releases openidea.JdkReleases
		if err := json.Unmarshal(raw, &releases); err != nil {
			m.post(func() {
				d := &m.jdkDialog
				if !d.open || d.seq != seq || d.vendor != vendor || d.releasesRequest != request {
					return
				}
				d.loading = false
				d.err = "Cannot read the Java versions: " + err.Error()
				m.invalidate()
			})
			return
		}
		m.post(func() {
			d := &m.jdkDialog
			if !d.open || d.seq != seq || d.vendor != vendor || d.releasesRequest != request {
				return
			}
			d.loading = false
			d.releasesRequest = 0
			d.versions = append([]uint32(nil), releases.Versions...)
			d.lts = append([]uint32(nil), releases.LTS...)
			d.recommended = releases.Recommended
			if !jdkHasVersion(d.versions, d.initialVersion) {
				d.initialVersion = 0
			}
			if d.initialVersion != 0 {
				d.version = d.initialVersion
			} else if d.recommended != 0 {
				d.version = d.recommended
			} else if len(d.versions) > 0 {
				d.version = d.versions[0]
			} else {
				d.version = 0
			}
			d.syncVersionLabel()
			m.invalidate()
		})
	}()
}

func jdkHasVersion(versions []uint32, version uint32) bool {
	for _, candidate := range versions {
		if candidate == version {
			return true
		}
	}
	return false
}

func (d *jdkDialogState) vendorChoices() []string {
	if len(d.vendors) == 0 {
		return []string{"Eclipse Temurin"}
	}
	out := make([]string, 0, len(d.vendors))
	for _, vendor := range d.vendors {
		out = append(out, vendor.Name)
	}
	return out
}

func (d *jdkDialogState) vendorFromLabel(label string) string {
	for _, vendor := range d.vendors {
		if vendor.Name == label {
			return vendor.ID
		}
	}
	if strings.TrimSpace(label) != "" {
		return defaultJDKVendor
	}
	return ""
}

func (d *jdkDialogState) versionChoices() []string {
	out := make([]string, 0, len(d.versions))
	for _, version := range d.versions {
		out = append(out, jdkVersionLabel(version, d.lts))
	}
	return out
}

func (d *jdkDialogState) versionFromLabel(label string) uint32 {
	for _, version := range d.versions {
		if jdkVersionLabel(version, d.lts) == label {
			return version
		}
	}
	return 0
}

func (d *jdkDialogState) syncVersionLabel() {
	if d.version == 0 {
		d.versionLabel = ""
		return
	}
	d.versionLabel = jdkVersionLabel(d.version, d.lts)
}

func jdkVersionLabel(version uint32, lts []uint32) string {
	if jdkHasVersion(lts, version) {
		return fmt.Sprintf("%d (LTS)", version)
	}
	return strconv.FormatUint(uint64(version), 10)
}

// BrowseJDKLocation chooses the parent directory the backend unpacks a JDK
// into, matching the folder button beside Location in the upstream dialog.
func (m *Model) BrowseJDKLocation() {
	seq := m.jdkDialog.seq
	start := strings.TrimSpace(m.jdkDialog.location)
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "JDK Location", ButtonLabel: "Choose",
			DefaultPath: start, Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		m.post(func() {
			if !m.jdkDialog.open || m.jdkDialog.seq != seq {
				return
			}
			m.jdkDialog.location = paths[0]
			m.invalidate()
		})
	}()
}

// DownloadJDKFromDialog starts the modal dialog's download.
func (m *Model) DownloadJDKFromDialog() {
	d := &m.jdkDialog
	if !d.open || m.jdkDownloadBusy || d.version == 0 {
		return
	}
	d.err = ""
	d.progress = nil
	d.message = "Resolving the latest build…"
	m.StartJDKDownload(d.vendor, d.version, strings.TrimSpace(d.location), false)
}

// StartJDKDownload downloads a JDK and streams its progress into Background
// Tasks. languageServer selects the downloaded JDK when the user already chose
// a language-server JDK, just as OpenIDEA's chooseForLanguageServer does.
func (m *Model) StartJDKDownload(vendor string, version uint32, location string, languageServer bool) {
	if version == 0 {
		m.OpenSettingsPage(settingsPageJava)
		return
	}
	if m.jdkDownloadBusy {
		m.status = "A JDK download is already running"
		m.invalidate()
		return
	}
	if m.mgr == nil && m.invokeHook == nil {
		m.status = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	if strings.TrimSpace(vendor) == "" {
		vendor = defaultJDKVendor
	}
	location = strings.TrimSpace(location)
	request := m.nextRequest()
	channel := m.nextChannel()
	m.jdkDownloadBusy = true
	m.jdkDownloadReq = request
	m.jdkDownloadCh = channel
	m.jdkDownloadVer = version
	m.jdkDownloadAbort.Store(false)
	m.jdkDialog.progress = nil
	m.jdkDialog.message = "Resolving the latest build…"
	title := fmt.Sprintf("Downloading JDK %d", version)
	vendorName := jdkVendorName(m.jdkDialog.vendors, vendor)
	m.upsertTask(BackgroundTask{
		ID:       jdkDownloadTaskID(version),
		Title:    title,
		Message:  vendorName,
		Fraction: -1,
		Cancel:   m.CancelJDKDownload,
	})
	m.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Minute)
		defer cancel()
		args := map[string]any{
			"vendor": vendor, "version": version, "request": request,
			"onEvent": map[string]any{"$channel": channel},
		}
		if location != "" {
			args["location"] = location
		} else {
			args["location"] = nil
		}
		raw, err := m.invoke(ctx, "jdk_download", args)
		var sdk openidea.Sdk
		if err == nil {
			err = json.Unmarshal(raw, &sdk)
		}
		if err == nil && languageServer {
			err = m.useDownloadedJDKForLanguageServer(ctx, sdk.Path)
		}
		m.post(func() {
			wasCancelled := m.jdkDownloadAbort.Load() || openidea.ErrorKind(err) == "cancelled"
			if m.jdkDownloadReq == request {
				m.jdkDownloadBusy = false
				m.jdkDownloadReq = 0
				m.jdkDownloadCh = 0
				m.jdkDownloadVer = 0
			}
			m.finishJDKTask(version, wasCancelled)
			if err != nil {
				if wasCancelled {
					if m.jdkDialog.open {
						m.jdkDialog.message = ""
						m.jdkDialog.progress = nil
					}
					return
				}
				text := fmt.Sprintf("Failed to download JDK %d: %v", version, err)
				if m.jdkDialog.open {
					m.jdkDialog.err = err.Error()
					m.jdkDialog.message = ""
					m.jdkDialog.progress = nil
				} else {
					m.notifyError(jdkDownloadTaskID(version), "JDK download failed", text)
					m.status = text
				}
				m.invalidate()
				return
			}
			if m.jdkDialog.open {
				m.closeJDKDownloadDialog()
			}
			name := sdk.Name
			if name == "" {
				name = fmt.Sprintf("JDK %d", version)
			}
			m.status = name + " downloaded to " + sdk.Path
			m.notifyInfo(jdkDownloadTaskID(version), name+" downloaded", sdk.Path)
			m.invalidate()
		})
	}()
}

func jdkDownloadTaskID(version uint32) string {
	return fmt.Sprintf("%s:%d", jdkDownloadTask, version)
}

func jdkVendorName(vendors []openidea.JdkVendor, id string) string {
	for _, vendor := range vendors {
		if vendor.ID == id {
			return vendor.Name
		}
	}
	if id == defaultJDKVendor {
		return "Eclipse Temurin"
	}
	return id
}

// CancelJDKDownload stops the visible job and closes its dialog. A direct
// action download has no dialog, so it simply leaves Background Tasks after
// the backend acknowledges the cancellation.
func (m *Model) CancelJDKDownload() {
	request := m.jdkDownloadReq
	if request == 0 {
		if m.jdkDialog.open {
			m.closeJDKDownloadDialog()
		}
		return
	}
	m.jdkDownloadAbort.Store(true)
	m.jdkDialog.progress = nil
	m.jdkDialog.message = ""
	m.jdkDialog.open = false
	m.jdkDialog.seq++
	m.jdkDialog.releasesRequest = 0
	m.cancelJDKRequest(request)
	m.invalidate()
}

func (m *Model) cancelJDKRequest(id uint32) {
	if id == 0 || (m.mgr == nil && m.invokeHook == nil) {
		return
	}
	m.cancelRequest(id)
}

func (m *Model) finishJDKTask(version uint32, cancelled bool) {
	id := jdkDownloadTaskID(version)
	for _, task := range m.tasks {
		if task.ID != id {
			continue
		}
		task.Done = true
		task.Fraction = 1
		task.Message = ""
		task.Cancel = nil
		m.upsertTask(task)
		if cancelled {
			m.removeTask(id)
		}
		return
	}
}

// acceptJDKEvent reports whether the chunk belongs to the current JDK
// download and mirrors it into the dialog and its Background Task.
func (m *Model) acceptJDKEvent(id uint32, data json.RawMessage) bool {
	if id == 0 || id != m.jdkDownloadCh || m.jdkDownloadCh == 0 {
		return false
	}
	var event openidea.InstallEvent
	if json.Unmarshal(data, &event) != nil {
		return true
	}
	message := event.Text
	if event.Type == "progress" {
		message = event.Message
		m.jdkDialog.progress = event.Fraction
	} else if message == "" {
		message = event.Message
	}
	message = strings.TrimSpace(message)
	m.jdkDialog.message = message
	version := m.jdkDownloadVer
	for i := range m.tasks {
		task := &m.tasks[i]
		if version == 0 || task.ID != jdkDownloadTaskID(version) {
			continue
		}
		if message != "" {
			task.Message = message
		}
		if event.Fraction != nil {
			task.Fraction = *event.Fraction
		}
	}
	m.invalidate()
	return true
}

// useDownloadedJDKForLanguageServer follows chooseForLanguageServer: Auto
// stays Auto, while an explicitly selected JDK is moved to the downloaded one.
// Project settings win when a project is open.
func (m *Model) useDownloadedJDKForLanguageServer(ctx context.Context, home string) error {
	home = strings.TrimSpace(home)
	if home == "" {
		return nil
	}
	raw, err := m.invoke(ctx, "settings_get", nil)
	if err != nil {
		return err
	}
	var settings struct {
		Languages struct {
			Java struct {
				JdtlsJdk string `json:"jdtlsJdk"`
			} `json:"java"`
		} `json:"languages"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return err
	}
	chosen := strings.TrimSpace(settings.Languages.Java.JdtlsJdk)
	if chosen == "" || filepath.Clean(chosen) == filepath.Clean(home) {
		return nil
	}
	value, err := json.Marshal(home)
	if err != nil {
		return err
	}
	scope := "global"
	if m.Root() != "" {
		scope = "project"
	}
	_, err = m.invoke(ctx, "settings_set", map[string]any{
		"scope": scope, "key": "languages.java.jdtlsJdk", "value": string(value),
	})
	return err
}

func (m *Model) jdkDownloadDialog(c *ui.Context, p *ui.Theme) {
	d := &m.jdkDialog
	if !d.open {
		return
	}
	oi := oiPaletteOf(p)
	open := d.open
	modal := ui.Modal(c, &open, func() {
		ui.Column(c).Width(560).Padding(18, 20, 16, 20).Gap(12).
			Background(oi.elev).Border(1, oi.border).Radius(6).Children(func() {
			ui.Text(c, "Download JDK").FontSize(14).Bold().
				TextColor(oi.textStrong).SingleLine()
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Vendor:").Width(72).Shrink(0).FontSize(12).
					TextColor(oi.text)
				if choice := ui.Select(c, &d.vendorLabel, d.vendorChoices()).
					Grow(1).Disabled(m.jdkDownloadBusy); choice.Changed() && !m.jdkDownloadBusy {
					vendor := d.vendorFromLabel(d.vendorLabel)
					if vendor != "" && vendor != d.vendor {
						d.vendor = vendor
						m.loadJDKReleases(vendor)
					}
				}
			})
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Version:").Width(72).Shrink(0).FontSize(12).
					TextColor(oi.text)
				versions := d.versionChoices()
				if len(versions) == 0 {
					ui.Text(c, "Loading…").FontSize(12).TextColor(oi.textMuted).SingleLine()
					ui.Spacer(c)
				} else if choice := ui.Select(c, &d.versionLabel, versions).
					Grow(1).Disabled(m.jdkDownloadBusy); choice.Changed() && !m.jdkDownloadBusy {
					d.version = d.versionFromLabel(d.versionLabel)
				}
			})
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Location:").Width(72).Shrink(0).FontSize(12).
					TextColor(oi.text)
				in := ui.TextInput(c, &d.location).Grow(1).
					Disabled(m.jdkDownloadBusy)
				in.OnChange(func() { m.invalidate() })
				if ui.Button(c, "Browse…").Disabled(m.jdkDownloadBusy).Clicked() {
					m.BrowseJDKLocation()
				}
			})
			vendorName := d.vendorLabel
			if vendorName == "" {
				vendorName = jdkVendorName(d.vendors, d.vendor)
			}
			version := ""
			if d.version != 0 {
				version = " " + strconv.FormatUint(uint64(d.version), 10)
			}
			target := d.location
			if target == "" {
				target = "…"
			}
			ui.Text(c, fmt.Sprintf(
				"%s%s is unpacked into a folder of its own in %s, where JDK detection finds it.",
				vendorName, version, target,
			)).FontSize(12).TextColor(oi.textMuted).MaxWidth(500)
			if d.message != "" || m.jdkDownloadBusy {
				ui.Column(c).Gap(5).Children(func() {
					fraction := -1.0
					if d.progress != nil {
						fraction = *d.progress
					}
					m.taskBar(c, p, oi, fraction, 0)
					message := d.message
					if message == "" {
						message = "Resolving the latest build…"
					}
					ui.Text(c, message).FontSize(12).TextColor(oi.textMuted).
						SingleLine().Ellipsis("…")
				})
			}
			if d.err != "" {
				ui.Text(c, d.err).FontSize(12).TextColor(oi.danger).
					MaxWidth(500).Ellipsis("…")
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "Download").
					Disabled(d.version == 0 || m.jdkDownloadBusy).
					AutoFocus().Clicked() {
					m.DownloadJDKFromDialog()
				}
				if ui.Button(c, "Cancel").Clicked() {
					m.closeJDKDownloadDialog()
				}
			})
		})
	})
	if modal.Dismissed() || !open {
		m.closeJDKDownloadDialog()
	}
}
