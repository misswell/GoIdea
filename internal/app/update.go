package app

// Check for Updates, as OpenIDEA's Help › Check for Updates and its update
// balloon do: the app asks the release feed in the background (at startup and
// every few hours) and in a manual check, offers an available version with its
// release notes, and downloads and installs it in place — verifying the
// release's SHA-256 — then restarts. What's New shows the notes of the version
// that was just installed.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var updateFeedURL = "https://api.github.com/repos/misswell/GoIdea/releases/latest"

// updateHTTP fetches the feed, the notes and the release assets; requests get
// their own deadlines.
var updateHTTP = &http.Client{}

const (
	updateCheckInterval = 6 * time.Hour
	updateFirstCheck    = 5 * time.Second
)

// updateRelease is the release the feed offers: its version, page, notes and
// the DMG and checksum assets of this Mac.
type updateRelease struct {
	Version string
	Page    string
	Notes   string
	DMG     string
	SHA     string
}

// updateState is the Check for Updates dialog and the install it runs.
type updateState struct {
	Open   bool
	Busy   bool
	Status string
	Latest string
	URL    string
	Notes  string
	Err    string
	Newer  bool

	Installing   bool
	Progress     float64 // 0..1, <0 while unknown
	ProgressText string
	// What's New shows the notes of the version just installed, not a release
	// the user can still choose.
	whatsNew bool

	cancel context.CancelFunc
	gen    uint64
}

// updatePrefs remembers which version was announced or skipped, and the notes
// the next launch shows as What's New.
type updatePrefs struct {
	Skipped        string `json:"skipped,omitempty"`
	Announced      string `json:"announced,omitempty"`
	PendingVersion string `json:"pendingVersion,omitempty"`
	PendingNotes   string `json:"pendingNotes,omitempty"`
}

func updatePrefsPath(sessionPath string) string {
	if sessionPath != "" {
		return filepath.Join(filepath.Dir(sessionPath), "update.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "goidea", "update.json")
}

func loadUpdatePrefs(path string) updatePrefs {
	var p updatePrefs
	if path == "" {
		return p
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	_ = json.Unmarshal(raw, &p)
	return p
}

func saveUpdatePrefs(path string, p updatePrefs) {
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	raw, err := json.Marshal(p)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o644)
}

// startUpdateChecks asks the feed shortly after startup and then periodically,
// and shows what the last install left as What's New.
func (m *Model) startUpdateChecks() {
	m.post(m.showWhatsNewIfPending)
	go func() {
		select {
		case <-time.After(updateFirstCheck):
		case <-m.ctx.Done():
			return
		}
		for {
			m.post(func() { m.checkForUpdates(false) })
			select {
			case <-time.After(updateCheckInterval):
			case <-m.ctx.Done():
				return
			}
		}
	}()
}

// CheckForUpdates is Help › Check for Updates.
func (m *Model) CheckForUpdates() { m.checkForUpdates(true) }

func (m *Model) checkForUpdates(manual bool) {
	if m.update.Busy || m.update.Installing {
		if manual {
			m.update.Open = true
			m.invalidate()
		}
		return
	}
	m.update.gen++
	gen := m.update.gen
	if manual {
		m.update.Open = true
		m.update.Busy = true
		m.update.Err = ""
		m.update.whatsNew = false
		m.update.Status = "Checking for updates…"
		m.invalidate()
	}
	go func() {
		rel, err := fetchLatestRelease(updateFeedURL)
		m.post(func() {
			if m.update.gen != gen {
				return
			}
			m.update.Busy = false
			current := mygo.App.Version()
			switch {
			case err != nil:
				if manual {
					m.update.Err = err.Error()
					m.update.Status = "Could not check for updates."
					m.invalidate()
				}
			case compareVersions(rel.Version, current) > 0:
				m.update.Latest, m.update.URL, m.update.Notes = rel.Version, rel.Page, rel.Notes
				m.update.Newer = true
				if manual {
					m.update.Status = fmt.Sprintf("Version %s is available.", rel.Version)
					m.invalidate()
				}
				m.announceUpdate(rel)
			default:
				if manual {
					m.update.Newer = false
					m.update.Latest = current
					m.update.Status = fmt.Sprintf("You are running the latest version (%s).", current)
					m.invalidate()
				}
			}
		})
	}()
}

// announceUpdate shows the update balloon once per version unless the user
// skipped it, as upstream's appUpdateAnnouncement.
func (m *Model) announceUpdate(rel updateRelease) {
	path := updatePrefsPath(m.cfg.SessionPath)
	prefs := loadUpdatePrefs(path)
	if rel.Version == "" || prefs.Skipped == rel.Version || prefs.Announced == rel.Version {
		return
	}
	prefs.Announced = rel.Version
	saveUpdatePrefs(path, prefs)
	summary := updateSummary(rel.Notes)
	m.notifyInfo("update.available", "Update available: v"+rel.Version, summary,
		notifyAction{label: "Install and Restart", run: func(m *Model) { m.InstallUpdate() }},
		notifyAction{label: "Release Notes", run: func(m *Model) {
			m.update.Open = true
			m.update.Latest = rel.Version
			m.update.Notes = rel.Notes
			m.update.whatsNew = false
			m.update.Status = fmt.Sprintf("Version %s is available.", rel.Version)
			m.invalidate()
		}},
		notifyAction{label: "Skip This Version", run: func(m *Model) {
			prefs := loadUpdatePrefs(path)
			prefs.Skipped = rel.Version
			saveUpdatePrefs(path, prefs)
		}},
	)
}

// updateSummary is the first paragraph of the notes, as the balloon shows it.
func updateSummary(notes string) string {
	text := strings.TrimSpace(notes)
	if text == "" {
		return ""
	}
	paragraph := strings.TrimSpace(strings.SplitN(text, "\n\n", 2)[0])
	if paragraph == "" {
		paragraph = text
	}
	if runes := []rune(paragraph); len(runes) > 300 {
		paragraph = string(runes[:300]) + "…"
	}
	return paragraph
}

// skipUpdateVersion drops the balloon for version.
func (m *Model) skipUpdateVersion(version string) {
	path := updatePrefsPath(m.cfg.SessionPath)
	prefs := loadUpdatePrefs(path)
	prefs.Skipped = version
	saveUpdatePrefs(path, prefs)
}

// InstallUpdate downloads the offered release beside its checksum, verifies
// it, replaces this app once it quits and reopens it. Outside a macOS app
// bundle it opens the release page instead.
func (m *Model) InstallUpdate() {
	if m.update.Installing {
		m.update.Open = true
		m.invalidate()
		return
	}
	target := appBundlePath()
	if runtime.GOOS != "darwin" || target == "" {
		if m.update.URL != "" {
			openReleasePage(m.update.URL)
		}
		return
	}
	want := m.update.Latest
	ctx, cancel := context.WithCancel(m.ctx)
	m.update.Open = true
	m.update.Installing = true
	m.update.Err = ""
	m.update.whatsNew = false
	m.update.Progress = -1
	m.update.ProgressText = "Preparing the download…"
	m.update.Status = fmt.Sprintf("Installing GoIDEA %s…", want)
	m.update.cancel = cancel
	m.invalidate()
	go func() {
		rel, err := fetchLatestRelease(updateFeedURL)
		if err == nil && (rel.Version != want || rel.DMG == "" || rel.SHA == "") {
			err = fmt.Errorf("the release assets of %s are not available yet", want)
		}
		var dmg string
		if err == nil {
			dmg, err = downloadUpdate(ctx, m, rel)
		}
		if err != nil {
			cancel()
			m.post(func() {
				if !m.update.Installing {
					return
				}
				m.update.Installing = false
				m.update.Err = err.Error()
				m.update.Status = "The update could not be installed."
				if ctx.Err() != nil {
					m.update.Status = "The update was cancelled."
				}
				m.invalidate()
			})
			return
		}
		script := installerScript(installWaitPID(os.Getpid()), dmg, target)
		cmd := exec.Command("/bin/sh", "-c", script)
		cmd.Stdout, cmd.Stderr = nil, nil
		if err := cmd.Start(); err != nil {
			_ = os.Remove(dmg)
			m.post(func() {
				m.update.Installing = false
				m.update.Err = "Starting the installer failed: " + err.Error()
				m.update.Status = "The update could not be installed."
				m.invalidate()
			})
			return
		}
		// The next launch shows these notes as What's New.
		path := updatePrefsPath(m.cfg.SessionPath)
		prefs := loadUpdatePrefs(path)
		prefs.PendingVersion, prefs.PendingNotes = rel.Version, rel.Notes
		saveUpdatePrefs(path, prefs)
		m.post(func() {
			m.status = fmt.Sprintf("Installing GoIDEA %s and restarting…", rel.Version)
			m.invalidate()
			m.finishQuit(true)
		})
	}()
}

// downloadUpdate streams the DMG to a temporary file, reporting progress, and
// verifies it against the release's .sha256 asset.
func downloadUpdate(ctx context.Context, m *Model, rel updateRelease) (string, error) {
	f, err := os.CreateTemp("", "goidea-update-*.dmg")
	if err != nil {
		return "", err
	}
	path := f.Name()
	fail := func(err error) (string, error) {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	resp, err := httpGetWith(ctx, rel.DMG)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	h := sha256.New()
	buf := make([]byte, 256*1024)
	var done, total int64 = 0, resp.ContentLength
	last := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			done += int64(n)
			_, _ = h.Write(buf[:n])
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fail(werr)
			}
			if time.Since(last) > 200*time.Millisecond {
				last = time.Now()
				m.reportDownload(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fail(rerr)
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	want, err := fetchChecksum(ctx, rel.SHA)
	if err != nil {
		return fail(err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(want, got) {
		return fail(fmt.Errorf("the download's SHA-256 does not match the release"))
	}
	m.reportDownload(done, done)
	return path, nil
}

// reportDownload posts the download's progress to the dialog.
func (m *Model) reportDownload(done, total int64) {
	m.post(func() {
		if !m.update.Installing {
			return
		}
		megabytes := func(n int64) string { return fmt.Sprintf("%.1f", float64(n)/(1024*1024)) }
		if total > 0 {
			m.update.Progress = float64(done) / float64(total)
			m.update.ProgressText = fmt.Sprintf("Downloading %s of %s MB…", megabytes(done), megabytes(total))
		} else {
			m.update.Progress = -1
			m.update.ProgressText = fmt.Sprintf("Downloading %s MB…", megabytes(done))
		}
		m.invalidate()
	})
}

// cancelUpdateInstall stops a download in progress.
func (m *Model) cancelUpdateInstall() {
	if m.update.cancel != nil {
		m.update.cancel()
	}
}

// httpGetWith performs a GET that honours ctx and redirects.
func httpGetWith(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GoIDEA/"+mygo.App.Version())
	resp, err := updateHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download returned %s", resp.Status)
	}
	return resp, nil
}

// fetchChecksum reads the hex digest from a release's .sha256 asset.
func fetchChecksum(ctx context.Context, url string) (string, error) {
	resp, err := httpGetWith(ctx, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	return parseChecksum(string(body))
}

// parseChecksum takes the digest from shasum output ("<hex>  <file>") or a
// bare digest.
func parseChecksum(text string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", fmt.Errorf("the checksum asset is empty")
	}
	digest := fields[0]
	if len(digest) != 64 {
		return "", fmt.Errorf("the checksum asset has no SHA-256")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", fmt.Errorf("the checksum asset has no SHA-256")
	}
	return digest, nil
}

// fetchLatestRelease reads the newest release: its version, page, notes and
// the DMG and checksum assets of this Mac.
func fetchLatestRelease(feed string) (updateRelease, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed, nil)
	if err != nil {
		return updateRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "GoIDEA/"+mygo.App.Version())
	resp, err := updateHTTP.Do(req)
	if err != nil {
		return updateRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return updateRelease{}, fmt.Errorf("release feed returned %s", resp.Status)
	}
	var release struct {
		Tag    string `json:"tag_name"`
		Page   string `json:"html_url"`
		Notes  string `json:"body"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return updateRelease{}, err
	}
	version := strings.TrimPrefix(strings.TrimSpace(release.Tag), "v")
	if version == "" {
		return updateRelease{}, fmt.Errorf("release feed has no version")
	}
	out := updateRelease{Version: version, Page: release.Page, Notes: release.Notes}
	names := make([]updateAssetName, 0, len(release.Assets))
	for _, a := range release.Assets {
		names = append(names, updateAssetName{Name: a.Name, URL: a.URL})
	}
	out.DMG, out.SHA = pickUpdateAsset(names, version, runtime.GOARCH)
	return out, nil
}

// updateAssetName is one release asset, for pickUpdateAsset.
type updateAssetName struct {
	Name string
	URL  string
}

// pickUpdateAsset finds the DMG for version on arch and its .sha256.
func pickUpdateAsset(assets []updateAssetName, version, arch string) (dmg, sha string) {
	want := fmt.Sprintf("goidea-%s-%s.dmg", version, arch)
	for _, a := range assets {
		switch a.Name {
		case want:
			dmg = a.URL
		case want + ".sha256":
			sha = a.URL
		}
	}
	if dmg != "" && sha != "" {
		return dmg, sha
	}
	// A release built for this Mac under another name still installs.
	prefix := "goidea-" + version + "-"
	for _, a := range assets {
		if dmg == "" && strings.HasPrefix(a.Name, prefix) && strings.HasSuffix(a.Name, "-"+arch+".dmg") {
			dmg = a.URL
			want = a.Name
		}
	}
	if dmg != "" {
		for _, a := range assets {
			if a.Name == want+".sha256" {
				sha = a.URL
			}
		}
	}
	return dmg, sha
}

// appBundlePath is the running .app bundle, "" for a binary outside one.
func appBundlePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	p := exe
	for i := 0; i < 5; i++ {
		if strings.HasSuffix(p, ".app") {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return ""
}

// installWaitPID waits for the running app to quit before replacing it.
func installWaitPID(pid int) string {
	return fmt.Sprintf("while kill -0 %d 2>/dev/null; do sleep 0.2; done", pid)
}

// installerScript mounts dmg after wait ran, copies GoIDEA.app over target and
// reopens it; a target that cannot be replaced opens the image in Finder so
// the user can move the app by hand.
func installerScript(wait, dmg, target string) string {
	q := shellQuote
	return "set -e\n" + wait + "\n" +
		"mnt=\"$(mktemp -d /tmp/goidea-update.XXXXXX)\"\n" +
		"hdiutil attach -nobrowse -readonly " + q(dmg) + " -mountpoint \"$mnt\" >/dev/null\n" +
		"src=\"$(find \"$mnt\" -maxdepth 2 -name GoIDEA.app -type d -print -quit)\"\n" +
		"if [ -z \"$src\" ]; then hdiutil detach \"$mnt\" >/dev/null 2>&1; open " + q(dmg) + "; exit 1; fi\n" +
		"rm -rf " + q(target+".new") + "\n" +
		"ditto \"$src\" " + q(target+".new") + "\n" +
		"hdiutil detach \"$mnt\" >/dev/null 2>&1 || true\n" +
		"rmdir \"$mnt\" 2>/dev/null || true\n" +
		"if ! rm -rf " + q(target) + "; then rm -rf " + q(target+".new") + "; open " + q(dmg) + "; exit 1; fi\n" +
		"mv " + q(target+".new") + " " + q(target) + "\n" +
		"rm -f " + q(dmg) + "\n" +
		"open " + q(target) + "\n"
}

// shellQuote single-quotes s for /bin/sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// showWhatsNewIfPending opens the notes of the version this install replaced.
func (m *Model) showWhatsNewIfPending() {
	path := updatePrefsPath(m.cfg.SessionPath)
	prefs := loadUpdatePrefs(path)
	current := mygo.App.Version()
	if prefs.PendingVersion == "" || prefs.PendingVersion != current {
		return
	}
	m.update.Open = true
	m.update.Latest = prefs.PendingVersion
	m.update.Notes = prefs.PendingNotes
	m.update.whatsNew = true
	m.update.Status = fmt.Sprintf("What's new in GoIDEA %s", current)
	m.invalidate()
	prefs.PendingVersion, prefs.PendingNotes = "", ""
	saveUpdatePrefs(path, prefs)
}

// compareVersions orders dotted versions by their numeric parts, ignoring a
// prerelease or build suffix, so v1.2.0 is newer than 1.1.9 and equal to
// 1.2.0-nightly.1.
func compareVersions(a, b string) int {
	pa := versionParts(a)
	pb := versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		switch {
		case x > y:
			return 1
		case x < y:
			return -1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil
	}
	fields := strings.Split(v, ".")
	parts := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			n = 0
		}
		parts = append(parts, n)
	}
	return parts
}

// openReleasePage shows url in the user's browser.
func openReleasePage(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// updateDialog draws Check for Updates and What's New.
func (m *Model) updateDialog(c *ui.Context, p *ui.Theme) {
	if !m.update.Open {
		return
	}
	open := m.update.Open
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(520).Gap(10).Children(func() {
			title := "Check for Updates"
			if m.update.whatsNew {
				title = "What's New"
			}
			ui.Text(c, title).FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, m.update.Status).FontSize(12).TextColor(p.Text).SingleLine().Ellipsis("…")
			if m.update.Err != "" {
				ui.Text(c, m.update.Err).FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			}
			if m.update.Installing {
				ui.Text(c, m.update.ProgressText).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
				frac := m.update.Progress
				if frac < 0 {
					frac = 0
				}
				if frac > 1 {
					frac = 1
				}
				ui.Row(c).Width(500).Height(6).Background(p.Border).Radius(3).Children(func() {
					ui.Row(c).Width(float32(500 * frac)).Height(6).Background(p.Accent).Radius(3)
				})
			}
			if m.update.Notes != "" {
				ui.Scroll(c).Height(240).Children(func() {
					ui.Column(c).Gap(4).Children(func() {
						for _, line := range strings.Split(strings.TrimRight(m.update.Notes, "\n"), "\n") {
							ui.Text(c, line).FontSize(11.5).TextColor(p.Text).SingleLine().Ellipsis("…")
						}
					})
				})
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if m.update.Installing {
					ui.Button(c, "Cancel").OnClick(func() { m.cancelUpdateInstall() })
				} else if m.update.Newer && m.update.Latest != "" {
					canInstall := runtime.GOOS == "darwin" && appBundlePath() != ""
					if canInstall {
						ui.Button(c, "Skip This Version").OnClick(func() {
							m.skipUpdateVersion(m.update.Latest)
							m.update.Open = false
							open = false
						})
						ui.Button(c, "Install and Restart").OnClick(func() { m.InstallUpdate() })
					} else if m.update.URL != "" {
						url := m.update.URL
						ui.Button(c, "Download").OnClick(func() { openReleasePage(url) })
					}
				}
				ui.Button(c, "Close").OnClick(func() {
					m.update.Open = false
					open = false
				})
			})
		})
	})
	if dlg.Dismissed() || !open {
		m.update.Open = false
	}
}
