package app

// Check for Updates, after OpenIDEA's features/app/updates.tsx: both the check
// and the install are background tasks in the status bar (the Welcome screen
// has no status bar, so their balloon shows the progress there), and their
// results arrive as notifications rather than a dialog. A check that finds a
// newer version posts a sticky "is available" balloon with Install and
// Restart; the install downloads with progress and Cancel, verifies the
// release's SHA-256, replaces this app and reopens it. What's New is a page in
// the editor area with the notes of the version the update installed.

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
)

var updateFeedURL = "https://api.github.com/repos/misswell/GoIdea/releases/latest"

// updateHTTP fetches the feed, the notes and the release assets; requests get
// their own deadlines.
var updateHTTP = &http.Client{}

const (
	updateCheckInterval = 4 * time.Hour
	updateFirstCheck    = 5 * time.Second

	// updateKey is the one key every update notification shares: the newest
	// replaces the earlier one, as upstream's UPDATE_KEY does.
	updateKey           = "app.update"
	updateCheckTaskID   = "app.update.check"
	updateInstallTaskID = "app.update.install"
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

// updateState is the check and install in flight. A repeated request joins
// the running one instead of starting another.
type updateState struct {
	checking   bool
	installing bool
	// latest is the release the last check found newer than this build.
	latest updateRelease
	// gen drops a check's answer that a Cancel superseded.
	gen    uint64
	cancel context.CancelFunc
}

// updatePrefs remembers which version was announced, the notes the next
// launch shows as What's New, the version this start last ran, and the
// nightly prompt's last time.
type updatePrefs struct {
	Announced         string `json:"announced,omitempty"`
	PendingVersion    string `json:"pendingVersion,omitempty"`
	PendingNotes      string `json:"pendingNotes,omitempty"`
	LastRunVersion    string `json:"lastRunVersion,omitempty"`
	NightlyPromptedAt int64  `json:"nightlyPromptedAt,omitempty"`
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

// UpdatePrefs are Settings › Updates: the automatic check and the What's New
// page stay on unless turned off, and LastChecked is when the feed was asked.
type UpdatePrefs struct {
	NoAutoCheck bool  `json:"noAutoCheck,omitempty"`
	NoWhatsNew  bool  `json:"noWhatsNew,omitempty"`
	LastChecked int64 `json:"lastChecked,omitempty"`
}

func (m *Model) autoCheckUpdates() bool {
	return m.prefs.Updates == nil || !m.prefs.Updates.NoAutoCheck
}

func (m *Model) showWhatsNewAfterUpdate() bool {
	return m.prefs.Updates == nil || !m.prefs.Updates.NoWhatsNew
}

// lastUpdateCheck is when the feed was last asked, zero before the first.
func (m *Model) lastUpdateCheck() time.Time {
	if m.prefs.Updates == nil || m.prefs.Updates.LastChecked == 0 {
		return time.Time{}
	}
	return time.UnixMilli(m.prefs.Updates.LastChecked)
}

// setUpdatePrefs changes one setting and stores it, as the Updates page's
// controls do.
func (m *Model) setUpdatePrefs(change func(*UpdatePrefs)) {
	p := UpdatePrefs{}
	if m.prefs.Updates != nil {
		p = *m.prefs.Updates
	}
	change(&p)
	m.prefs.Updates = &p
	m.savePreferences()
	m.invalidate()
}

func (m *Model) setUpdateLastChecked(t time.Time) {
	m.setUpdatePrefs(func(p *UpdatePrefs) { p.LastChecked = t.UnixMilli() })
}

// SetAutoCheckUpdates is Settings › Updates' automatic check.
func (m *Model) SetAutoCheckUpdates(on bool) {
	m.setUpdatePrefs(func(p *UpdatePrefs) { p.NoAutoCheck = !on })
}

// SetShowWhatsNewAfterUpdate is Settings › Updates' What's New page.
func (m *Model) SetShowWhatsNewAfterUpdate(on bool) {
	m.setUpdatePrefs(func(p *UpdatePrefs) { p.NoWhatsNew = !on })
}

// startUpdateChecks asks the feed shortly after startup and then every four
// hours while the app runs, unless Settings turned the automatic check off,
// and opens what the last update left as What's New.
func (m *Model) startUpdateChecks() {
	m.post(m.showWhatsNewIfPending)
	go func() {
		select {
		case <-time.After(updateFirstCheck):
		case <-m.ctx.Done():
			return
		}
		for {
			m.post(func() {
				if m.autoCheckUpdates() {
					m.checkForUpdates(false)
				}
			})
			select {
			case <-time.After(updateCheckInterval):
			case <-m.ctx.Done():
				return
			}
		}
	}()
}

// CheckForUpdates is Help › Check for Updates: a manual check whose answer
// pops up even with balloon notifications turned off.
func (m *Model) CheckForUpdates() { m.checkForUpdates(true) }

func (m *Model) checkForUpdates(manual bool) {
	if m.update.installing {
		if manual {
			m.status = "GoIDEA is installing an update"
			m.invalidate()
		}
		return
	}
	if m.update.checking {
		return
	}
	m.update.checking = true
	m.update.gen++
	gen := m.update.gen
	title := "Checking for updates…"
	m.upsertTask(BackgroundTask{ID: updateCheckTaskID, Title: title, Fraction: -1,
		Cancel: func() {
			// The check only reads: cancelling drops its answer, as upstream's
			// abandoned promise does, while the request finishes on its own.
			m.post(func() {
				if m.update.gen != gen {
					return
				}
				m.update.gen++
				m.update.checking = false
				m.removeTask(updateCheckTaskID)
			})
		}})
	go func() {
		rel, err := fetchLatestRelease(updateFeedURL)
		m.post(func() {
			if m.update.gen != gen {
				return
			}
			m.update.checking = false
			m.setUpdateLastChecked(time.Now())
			m.upsertTask(BackgroundTask{ID: updateCheckTaskID, Title: title, Fraction: 1, Done: true})
			current := mygo.App.Version()
			switch {
			case err != nil:
				m.notifyUser(updateKey, "error", "Update check failed", err.Error())
			case compareVersions(rel.Version, current) > 0:
				m.update.latest = rel
				m.announceUpdate(rel, manual)
			default:
				m.notifyUser(updateKey, "info", "GoIDEA "+current+" is up to date", "")
			}
		})
	}()
}

// announceUpdate offers the newer release once per version, as upstream's
// announcement claim does; a manual check always shows it.
func (m *Model) announceUpdate(rel updateRelease, manual bool) {
	if !manual {
		if !m.claimUpdateAnnouncement(rel.Version) {
			return
		}
		if strings.Contains(rel.Version, "-nightly.") && !m.nightlyPromptDue() {
			return
		}
	}
	m.notifySticky(updateKey, "info", "GoIDEA "+rel.Version+" is available",
		updateSummary(rel.Notes), manual,
		notifyAction{label: "Install and Restart", run: func(m *Model) { m.InstallUpdate() }})
}

// claimUpdateAnnouncement reports whether the automatic check may announce
// version: each one is offered once, across starts.
func (m *Model) claimUpdateAnnouncement(version string) bool {
	path := updatePrefsPath(m.cfg.SessionPath)
	prefs := loadUpdatePrefs(path)
	if version == "" || prefs.Announced == version {
		return false
	}
	prefs.Announced = version
	saveUpdatePrefs(path, prefs)
	return true
}

// nightlyPromptDue throttles a nightly build's prompt to one a day, since
// they come out more often than anyone wants to be asked.
func (m *Model) nightlyPromptDue() bool {
	path := updatePrefsPath(m.cfg.SessionPath)
	prefs := loadUpdatePrefs(path)
	now := time.Now().UnixMilli()
	if now-prefs.NightlyPromptedAt < int64(24*time.Hour/time.Millisecond) {
		return false
	}
	prefs.NightlyPromptedAt = now
	saveUpdatePrefs(path, prefs)
	return true
}

// updateSummary is the first paragraph of the notes, as the balloon shows it;
// the whole notes belong to the What's New page.
func updateSummary(notes string) string {
	text := strings.TrimSpace(notes)
	if text == "" {
		return ""
	}
	paragraph := strings.TrimSpace(strings.SplitN(text, "\n\n", 2)[0])
	if paragraph == "" {
		paragraph = text
	}
	// Release notes are Markdown; the balloon draws plain text, so drop the
	// heading and list markers and the emphasis the notes carry.
	paragraph = strings.TrimLeft(paragraph, "# \t")
	paragraph = strings.TrimPrefix(paragraph, "- ")
	paragraph = strings.NewReplacer("**", "", "__", "", "`", "").Replace(paragraph)
	paragraph = strings.Join(strings.Fields(paragraph), " ")
	if runes := []rune(paragraph); len(runes) > 300 {
		paragraph = string(runes[:300]) + "…"
	}
	return paragraph
}

// InstallUpdate downloads the offered release beside its checksum, verifies
// it, replaces this app once it quits and reopens it. Outside a macOS app
// bundle it opens the release page instead. A second call joins the install.
func (m *Model) InstallUpdate() {
	if m.update.installing {
		return
	}
	rel := m.update.latest
	if runtime.GOOS != "darwin" || appBundlePath() == "" {
		if rel.Page != "" {
			openReleasePage(rel.Page)
		} else if rel.DMG != "" {
			openReleasePage(rel.DMG)
		}
		return
	}
	if rel.Version == "" {
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.update.installing = true
	m.update.cancel = cancel
	title := "Downloading GoIDEA " + rel.Version
	m.upsertTask(BackgroundTask{ID: updateInstallTaskID, Title: title, Fraction: -1, Cancel: cancel})
	m.notifySticky(updateKey, "info", title, "Preparing the download…", true,
		notifyAction{label: "Cancel", run: func(m *Model) { m.cancelUpdateInstall() }})
	go m.installWorker(ctx, rel)
}

// installWorker downloads and starts the installer; its failures and its
// completion replace the progress notification.
func (m *Model) installWorker(ctx context.Context, want updateRelease) {
	title := "Downloading GoIDEA " + want.Version
	failed := func(err error) {
		cancelled := ctx.Err() != nil
		m.post(func() {
			m.update.installing = false
			m.update.cancel = nil
			m.upsertTask(BackgroundTask{ID: updateInstallTaskID, Title: title, Fraction: 1, Done: true})
			notifyUpdateFailure(m, cancelled, err)
		})
	}
	rel, err := fetchLatestRelease(updateFeedURL)
	if err == nil && (rel.Version != want.Version || rel.DMG == "" || rel.SHA == "") {
		err = fmt.Errorf("the release assets of %s are not available yet", want.Version)
	}
	var dmg string
	if err == nil {
		dmg, err = downloadUpdate(ctx, m, rel)
	}
	if err != nil {
		failed(err)
		return
	}
	target := appBundlePath()
	script := installerScript(installWaitPID(os.Getpid()), dmg, target)
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		_ = os.Remove(dmg)
		failed(err)
		return
	}
	// The next launch shows these notes as What's New.
	path := updatePrefsPath(m.cfg.SessionPath)
	prefs := loadUpdatePrefs(path)
	prefs.PendingVersion, prefs.PendingNotes = rel.Version, rel.Notes
	saveUpdatePrefs(path, prefs)
	m.post(func() {
		m.update.installing = false
		m.update.cancel = nil
		m.upsertTask(BackgroundTask{ID: updateInstallTaskID, Title: title, Fraction: 1, Done: true})
		// Stays if the restart does not happen: the update applies on the
		// next start, as upstream's installed notification says.
		m.notifySticky(updateKey, "info", "GoIDEA "+rel.Version+" is installed",
			"It is used after GoIDEA restarts.", true,
			notifyAction{label: "Restart", run: func(m *Model) { m.finishQuit(true) }})
		m.status = fmt.Sprintf("Installing GoIDEA %s and restarting…", rel.Version)
		m.invalidate()
		// The installer waits for this process to quit, replaces the bundle
		// and opens it again.
		m.finishQuit(true)
	})
}

// notifyUpdateFailure replaces the download's progress with why it ended:
// a cancelled download offers Retry, a failure offers it with the reason.
func notifyUpdateFailure(m *Model, cancelled bool, err error) {
	retry := notifyAction{label: "Retry", run: func(m *Model) { m.InstallUpdate() }}
	if cancelled {
		m.notifyUser(updateKey, "info", "Update cancelled", "", retry)
		return
	}
	m.notifyUser(updateKey, "error", "Could not install the update", err.Error(), retry)
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
				m.reportDownload(rel.Version, done, total)
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
	m.reportDownload(rel.Version, done, done)
	return path, nil
}

// reportDownload posts the download's progress to the status bar's task and
// the balloon, in the megabytes upstream shows.
func (m *Model) reportDownload(version string, done, total int64) {
	bytes := func(n int64) string { return fmt.Sprintf("%.1f", float64(n)/(1024*1024)) }
	message := bytes(done) + " MB"
	fraction := -1.0
	if total > 0 {
		message = bytes(done) + " MB of " + bytes(total) + " MB"
		fraction = float64(done) / float64(total)
	}
	m.post(func() {
		if !m.update.installing {
			return
		}
		m.upsertTask(BackgroundTask{ID: updateInstallTaskID,
			Title: "Downloading GoIDEA " + version, Message: message,
			Fraction: fraction, Cancel: m.update.cancel})
		m.updateNotificationBody(updateKey, message)
	})
}

// cancelUpdateInstall stops a download in progress; the install step itself
// is one move that must not be cut short.
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

// showWhatsNewIfPending opens the notes of a version this start runs for the
// first time, as upstream's checkAfterUpdate does: one start of a different
// version opens the page, unless the setting turned it off.
func (m *Model) showWhatsNewIfPending() {
	path := updatePrefsPath(m.cfg.SessionPath)
	prefs := loadUpdatePrefs(path)
	current := mygo.App.Version()
	if current == "" || prefs.LastRunVersion == current {
		return
	}
	previous := prefs.LastRunVersion
	prefs.LastRunVersion = current
	notes := ""
	if prefs.PendingVersion == current {
		notes, prefs.PendingVersion, prefs.PendingNotes = prefs.PendingNotes, "", ""
	}
	saveUpdatePrefs(path, prefs)
	// The first run has nothing to compare against.
	if previous == "" || !m.showWhatsNewAfterUpdate() {
		return
	}
	m.OpenWhatsNew(current, notes)
}

// OpenWhatsNew is the page of one version, with its notes when the install
// recorded them.
func (m *Model) OpenWhatsNew(version, notes string) {
	if version == "" {
		version = mygo.App.Version()
	}
	m.openWhatsNewPage(version, notes)
	if notes != "" {
		return
	}
	// An update installed outside the app recorded no notes: read the release
	// the page belongs to, as upstream's page does.
	if t := m.whatsNewTab(); t != nil {
		t.pageLoading = true
	}
	m.invalidate()
	go func() {
		found := fetchReleaseNotes(version)
		m.post(func() {
			t := m.whatsNewTab()
			if t == nil || t.pageVersion != version {
				return
			}
			t.pageLoading = false
			if found != "" {
				t.pageNotes = found
			}
			m.invalidate()
		})
	}()
}

// OpenCurrentWhatsNew is Help › What's New.
func (m *Model) OpenCurrentWhatsNew() { m.OpenWhatsNew(mygo.App.Version(), "") }

// fetchReleaseNotes reads the notes of one version's release, "" when the
// feed has none.
func fetchReleaseNotes(version string) string {
	feed := "https://api.github.com/repos/misswell/GoIdea/releases/tags/v" + version
	rel, err := fetchLatestRelease(feed)
	if err != nil || rel.Version != version {
		return ""
	}
	return rel.Notes
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
