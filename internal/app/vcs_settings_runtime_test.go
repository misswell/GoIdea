package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func vcsSettingsRuntimeModel(t *testing.T, overrides map[string]any) *Model {
	t.Helper()
	m := &Model{ctx: context.Background(), cfg: Config{
		SettingsPath: filepath.Join(t.TempDir(), "settings.json"),
	}}
	m.setRoot(t.TempDir())
	m.postHook = func(fn func()) { fn() }
	m.nativeSettings.pages = map[string]nativeSettingsPageState{}
	for _, def := range vcsNativeSettingsPages() {
		values := nativeTestDefaults(def)
		for key, value := range overrides {
			if _, ok := values[key]; ok {
				values[key] = value
			}
		}
		m.nativeSettings.pages[def.page.id] = nativeSettingsPageState{
			loaded: true, session: true, scope: "project", root: m.Root(),
			applied: nativeSettingsCloneValues(values),
			draft:   nativeSettingsCloneValues(values),
		}
	}
	return m
}

func TestVCSCommitMessageInspections(t *testing.T) {
	m := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.commit.blankLineAfterSubject": true,
		"vcs.commit.limitSubject":          true,
		"vcs.commit.subjectLimit":          float64(5),
		"vcs.commit.limitBody":             true,
		"vcs.commit.bodyLimit":             float64(4),
	})
	got := vcsCommitMessageProblems("subject\nbody line", m.vcsCommitMessageSettings())
	want := []string{
		"Subject line is longer than 5 characters",
		"Add a blank line between the subject and body",
		"Body line 2 is longer than 4 characters",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("message problems = %#v, want %#v", got, want)
	}
	if problems := m.vcsCommitMessageProblems("好\n\n正文"); len(problems) != 0 {
		t.Fatalf("valid Unicode message problems = %#v", problems)
	}
	if got := wrapVCSCommitMessageAt("subject stays long\nbody words should wrap", 10); got !=
		"subject stays long\nbody words\nshould\nwrap" {
		t.Fatalf("wrapped commit message = %q", got)
	}
}

func TestVCSProtectedBranchesAndPushDialogDecision(t *testing.T) {
	m := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.git.showPushDialog":          true,
		"vcs.git.pushDialogOnlyProtected": true,
		"vcs.git.protectedBranches":       "main; release/*; hotfix?",
		"vcs.git.updateMethod":            "rebase",
	})
	for branch, want := range map[string]bool{
		"main":          true,
		"release/1.2":   true,
		"hotfix1":       true,
		"feature/login": false,
	} {
		if got := m.vcsProtectedBranch(branch); got != want {
			t.Errorf("protected(%q) = %v, want %v", branch, got, want)
		}
		if got := m.vcsPushDialogNeeded(branch); got != want {
			t.Errorf("push dialog needed(%q) = %v, want %v", branch, got, want)
		}
	}
	if !m.vcsPushDialogNeeded("") {
		t.Fatal("a detached or unknown target must always show the Push dialog")
	}
	m.nativeSettings.pages[settingsPageVCSGit].applied["vcs.git.showPushDialog"] = false
	if m.vcsPushDialogNeeded("feature/login") {
		t.Fatal("showPushDialog=false still opened the Push dialog")
	}
	if m.vcsGitUpdateMethod() != "rebase" {
		t.Fatalf("update method = %q, want rebase", m.vcsGitUpdateMethod())
	}
}

func TestVCSCommitDefaultsDraftAndNonModalMode(t *testing.T) {
	m := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.commit.nonModal":            true,
		"vcs.commit.clearInitialMessage": false,
		"vcs.git.signOff":                true,
	})
	m.commitInitialMessage = "fix: keep this message"
	m.OpenCommit()
	if m.commit == nil {
		t.Fatal("OpenCommit did not create commit state")
	}
	if !m.vcsCommitNonModal() {
		t.Fatal("non-modal commit setting was not read at runtime")
	}
	if m.commit.message != "fix: keep this message" || !m.commit.signOff {
		t.Fatalf("commit defaults = %#v", m.commit)
	}
	m.commit.message = "fix: committed message"
	m.finishCommit(m.commit, "1234567890", nil)
	if m.commit != nil {
		t.Fatal("successful commit left commit state open")
	}
	if m.commitInitialMessage != "fix: committed message" {
		t.Fatalf("kept commit message = %q", m.commitInitialMessage)
	}

	m.nativeSettings.pages[settingsPageVCSCommit].applied["vcs.commit.clearInitialMessage"] = true
	m.OpenCommit()
	m.commit.message = "fix: clear this message"
	m.finishCommit(m.commit, "0987654321", nil)
	if m.commitInitialMessage != "" {
		t.Fatalf("Clear initial commit message retained %q", m.commitInitialMessage)
	}
}

func TestVCSCommitChecksWarnBeforeCommit(t *testing.T) {
	m := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.git.warnCrlf":         true,
		"vcs.git.warnBadFileNames": true,
		"vcs.git.warnLargeFiles":   true,
		"vcs.git.largeFileMb":      float64(1),
		"vcs.commit.checkTodo":     true,
	})
	path := filepath.Join(m.Root(), "CON.txt")
	content := append([]byte("// TODO fix this\r\n"), make([]byte, 1536*1024)...)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	m.vcs.repositories = []vcsRepository{{Root: m.Root(), Head: vcsStringPointer("main")}}
	m.vcs.current = m.Root()
	m.vcs.files = []openidea.FileStatus{{Path: path, Unstaged: vcsStringPointer("untracked")}}
	m.commit = &commitState{repo: m.Root()}
	problems := m.vcsCommitCheckProblems(m.commit, commitPlan{included: []string{path}})
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"Windows reserved names", "CRLF", "large file", "TODO and FIXME"} {
		if !strings.Contains(joined, want) {
			t.Errorf("commit problems lack %q: %s", want, joined)
		}
	}
}

func TestVCSPatchAndShelfModes(t *testing.T) {
	m := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.confirmation.onPatchCreated": "reveal",
		"vcs.shelf.removeApplied":         true,
	})
	if got := m.vcsPatchCreatedMode(); got != "reveal" {
		t.Fatalf("patch mode = %q", got)
	}
	if !m.vcsShelfRemoveApplied() {
		t.Fatal("shelf removeApplied setting was not read at runtime")
	}
}

func TestVCSPatchAskOpensNativeConfirmation(t *testing.T) {
	m := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.confirmation.onPatchCreated": "ask",
	})
	m.afterPatchSaved("/tmp/example.patch")
	if !m.vcsMore.open || m.vcsMore.action != "patch.saved" ||
		m.vcsMore.input != "/tmp/example.patch" {
		t.Fatalf("patch confirmation state = %#v", m.vcsMore)
	}
	if m.vcsMorePrimaryLabel() != "Show" {
		t.Fatalf("patch confirmation primary = %q", m.vcsMorePrimaryLabel())
	}
}

func TestVCSProjectTreeDecorationsAndCombinedStashTabs(t *testing.T) {
	m := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.showDirtyDirectories":     false,
		"vcs.git.combineStashAndShelf": true,
	})
	source := filepath.Join(m.Root(), "src", "Main.java")
	m.vcs.files = []openidea.FileStatus{{
		Path: source, RelPath: "src/Main.java",
		Unstaged: vcsStringPointer("modified"),
	}}
	m.rebuildVCSStatusDecorations()
	if got := m.projectTreeVCSKind(source, false); got != "modified" {
		t.Fatalf("file status = %q, want modified", got)
	}
	if got := m.projectTreeVCSKind(filepath.Dir(source), true); got != "" {
		t.Fatalf("dirty directory shown while setting is off: %q", got)
	}
	m.nativeSettings.pages[settingsPageVCSConfirmation].applied["vcs.showDirtyDirectories"] = true
	if got := m.projectTreeVCSKind(filepath.Dir(source), true); got != "modified" {
		t.Fatalf("dirty directory status = %q, want modified", got)
	}

	tester := ui.NewTester(func(c *ui.Context) {
		m.vcsContentTabs(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 900, 100)
	tester.Frame()
	if !tester.HasText("Stashes and Shelves") {
		t.Fatalf("combined tabs lack Stashes and Shelves: %q", tester.Texts())
	}
	for _, text := range tester.Texts() {
		if text == "Shelf" || text == "Stash" {
			t.Fatalf("combined tabs still show separate %q: %q", text, tester.Texts())
		}
	}
}

func TestVCSHistoryLimitOnlyCapsPathHistory(t *testing.T) {
	m := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.limitHistory": true,
		"vcs.historyLimit": float64(10),
	})
	if got := m.vcsLogRequestLimit("/p/file.go"); got != 10 {
		t.Fatalf("path history limit = %d, want 10", got)
	}
	if got := m.vcsLogRequestLimit(""); got != logLimit {
		t.Fatalf("unfiltered log limit = %d, want %d", got, logLimit)
	}
	m.nativeSettings.pages[settingsPageVCSConfirmation].applied["vcs.limitHistory"] = false
	if got := m.vcsLogRequestLimit("/p/file.go"); got != logLimit {
		t.Fatalf("unlimited path history request = %d, want %d", got, logLimit)
	}
}

func TestVCSInteractiveRebaseConfirmsDrops(t *testing.T) {
	m := vcsSettingsRuntimeModel(t, map[string]any{
		"vcs.confirmation.confirmDropCommits": true,
	})
	m.vcsMore = vcsMoreState{
		open: true, kind: vcsMoreRebase, action: "interactive.rebase",
		rebaseCommits: []openidea.Commit{
			{ID: "aaaaaaaa", Message: "keep"},
			{ID: "bbbbbbbb", Message: "drop"},
		},
		rebaseActions:  []string{"pick", "drop"},
		rebaseMessages: []string{"keep", "drop"},
	}
	m.submitInteractiveRebase()
	if !m.vcsMore.flag || !strings.Contains(m.vcsMore.err, "will be dropped") {
		t.Fatalf("drop confirmation = flag %v error %q", m.vcsMore.flag, m.vcsMore.err)
	}
	if got := m.vcsMorePrimaryLabel(); got != "Drop Commits and Rebase" {
		t.Fatalf("drop confirmation primary = %q", got)
	}
}

func TestVCSGitCommandsLoadCredentialsAndVersion(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var forgotten []string
	m := vcsSettingsRuntimeModel(t, nil)
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		mu.Lock()
		calls = append(calls, command)
		mu.Unlock()
		switch command {
		case "git_credentials":
			return json.RawMessage(`["github.com","gitlab.com"]`), nil
		case "git_credential_forget":
			request := args.(map[string]any)
			forgotten = append(forgotten, request["key"].(string))
			return json.RawMessage(`null`), nil
		case "vcs_detected_git":
			return json.RawMessage(`"/usr/local/bin/git"`), nil
		case "vcs_git_version":
			return json.RawMessage(`{"version":"git version 2.47.1","path":"/usr/local/bin/git"}`), nil
		default:
			return nil, errors.New("unexpected command " + command)
		}
	}

	m.ensureVCSGitDetected()
	m.ensureVCSGitCredentials()
	m.testVCSGitExecutable("")
	waitVCSRuntimeTest(t, func() bool {
		state := m.nativeSettings.pages[settingsPageVCSGit]
		return state.aux["git.detectedState"] == "ready" &&
			state.aux["git.credentialsState"] == "ready" &&
			state.aux["git.testing"] == ""
	})
	state := m.nativeSettings.pages[settingsPageVCSGit]
	if state.aux["git.detected"] != "/usr/local/bin/git" {
		t.Fatalf("detected Git = %q", state.aux["git.detected"])
	}
	if !strings.Contains(state.aux["git.result"], "Git version is 2.47.1") {
		t.Fatalf("Git version result = %q", state.aux["git.result"])
	}
	tester := ui.NewTester(func(c *ui.Context) {
		m.settingsVCSGitCredentialsRow(c, oiPaletteOf(c.Theme()))
	}, 900, 300)
	tester.Frame()
	for _, text := range []string{"github.com", "gitlab.com", "Forget"} {
		if !tester.HasText(text) {
			t.Fatalf("credentials UI lacks %q: %q", text, tester.Texts())
		}
	}
	if err := tester.Click("Forget"); err != nil {
		t.Fatal(err)
	}
	waitVCSRuntimeTest(t, func() bool {
		state := m.nativeSettings.pages[settingsPageVCSGit]
		return !strings.Contains(state.aux["git.credentials"], "github.com")
	})
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(forgotten, []string{"github.com"}) {
		t.Fatalf("forgotten credentials = %#v", forgotten)
	}
	for _, want := range []string{"vcs_detected_git", "git_credentials", "vcs_git_version", "git_credential_forget"} {
		found := false
		for _, call := range calls {
			if call == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Git commands did not call %s: %#v", want, calls)
		}
	}
}

func TestVCSNativeSettingsSaveWithProjectScope(t *testing.T) {
	var mu sync.Mutex
	var writes []capturedSettingsWrite
	m := vcsSettingsRuntimeModel(t, nil)
	m.backendFormatOnSaveKnown = true
	m.backendFormatOnSave = false
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "settings_set" {
			return json.RawMessage(`null`), nil
		}
		request := args.(map[string]any)
		mu.Lock()
		writes = append(writes, capturedSettingsWrite{
			scope: request["scope"].(string),
			key:   request["key"].(string),
			value: request["value"].(string),
		})
		mu.Unlock()
		return json.RawMessage(`null`), nil
	}
	if def, ok := nativeSettingsDefinition(settingsPageVCSGit); !ok {
		t.Fatal("VCS Git page is not native")
	} else if def.defaultScope != "project" || !def.projectOnly {
		t.Fatalf("VCS Git scope = %q projectOnly=%v", def.defaultScope, def.projectOnly)
	}
	m.setNativeSettingsValue(settingsPageVCSGit, "vcs.git.inlineBlame", true)
	m.setNativeSettingsValue(settingsPageVCSGit, "vcs.git.protectedBranches", "main; release/*")
	m.saveBackendCatalogThenJava(false)
	waitVCSRuntimeTest(t, func() bool {
		return !m.nativeSettings.saving && !m.settingsDirtyPage(settingsPageVCSGit)
	})
	mu.Lock()
	defer mu.Unlock()
	filtered := writes[:0]
	for _, write := range writes {
		if strings.HasPrefix(write.key, "vcs.") {
			filtered = append(filtered, write)
		}
	}
	if len(filtered) != 2 {
		t.Fatalf("settings writes = %#v", writes)
	}
	for _, write := range filtered {
		if write.scope != "project" {
			t.Errorf("VCS setting %s saved to %s, want project", write.key, write.scope)
		}
	}
}

func waitVCSRuntimeTest(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for VCS runtime state")
}

func vcsStringPointer(value string) *string { return &value }
