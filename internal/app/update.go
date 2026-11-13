package app

// Check for Updates, as OpenIDEA's Help › Check for Updates does: ask the
// release feed in the background, then show the newest version or that this
// is the latest one, with a way to open the release page.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var updateFeedURL = "https://api.github.com/repos/misswell/GoIdea/releases/latest"

// updateHTTP is the client the check uses; tests point it at a local feed.
var updateHTTP = &http.Client{Timeout: 15 * time.Second}

// updateState is the Check for Updates dialog: the outcome of the check, or
// the request while it is out.
type updateState struct {
	Open   bool
	Busy   bool
	Status string
	Latest string
	URL    string
	Err    string
	Newer  bool
}

// CheckForUpdates runs the check off the UI thread; a second click while one
// is out does nothing, as OpenIDEA joins a running check.
func (m *Model) CheckForUpdates() {
	if m.update.Busy {
		m.update.Open = true
		m.invalidate()
		return
	}
	m.update = updateState{Open: true, Busy: true, Status: "Checking for updates…"}
	m.invalidate()
	go func() {
		latest, url, err := fetchLatestRelease(updateFeedURL)
		mygo.RunOnMain(func() {
			current := mygo.App.Version()
			m.update.Busy = false
			switch {
			case err != nil:
				m.update.Err = err.Error()
				m.update.Status = "Could not check for updates."
			case compareVersions(latest, current) > 0:
				m.update.Latest, m.update.URL = latest, url
				m.update.Newer = true
				m.update.Status = fmt.Sprintf("Version %s is available.", latest)
			default:
				m.update.Latest = current
				m.update.Status = fmt.Sprintf("You are running the latest version (%s).", current)
			}
			m.invalidate()
		})
	}()
}

func fetchLatestRelease(feed string) (string, string, error) {
	req, err := http.NewRequest(http.MethodGet, feed, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "GoIDEA/"+mygo.App.Version())
	resp, err := updateHTTP.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("release feed returned %s", resp.Status)
	}
	var release struct {
		Tag  string `json:"tag_name"`
		Page string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(release.Tag) == "" {
		return "", "", fmt.Errorf("release feed has no version")
	}
	return strings.TrimPrefix(release.Tag, "v"), release.Page, nil
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

// updateDialog draws Check for Updates.
func (m *Model) updateDialog(c *ui.Context, p *ui.Theme) {
	if !m.update.Open {
		return
	}
	open := m.update.Open
	dlg := ui.Modal(c, &open, func() {
		ui.Column(c).Width(440).Gap(10).Children(func() {
			ui.Text(c, "Check for Updates").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, m.update.Status).FontSize(12).TextColor(p.Text).SingleLine()
			if m.update.Err != "" {
				ui.Text(c, m.update.Err).FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			}
			if m.update.Newer && m.update.URL != "" {
				ui.Text(c, "Download the new version from the release page.").
					FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if m.update.Newer && m.update.URL != "" {
					url := m.update.URL
					ui.Button(c, "Open Release Page").OnClick(func() { openReleasePage(url) })
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
