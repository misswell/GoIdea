package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

func TestVCSFileOperationMatchingAddsFoldersAndUnityMetadata(t *testing.T) {
	untracked := "untracked"
	staged := "added"
	files := []openidea.FileStatus{
		{Path: "/p/Assets/New.cs", Unstaged: &untracked},
		{Path: "/p/Assets/New.cs.meta", Unstaged: &untracked},
		{Path: "/p/Staged.cs", Staged: &staged},
		{Path: "/p/Assets/Folder/Child.cs", Unstaged: &untracked},
	}
	got := vcsMatchingFileOperationPaths(
		files,
		[]string{"/p/Assets/New.cs", "/p/Assets/Folder"},
		"created",
		true,
	)
	want := []string{
		"/p/Assets/New.cs",
		"/p/Assets/New.cs.meta",
		"/p/Assets/Folder/Child.cs",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("created paths = %v, want %v", got, want)
	}
}

func TestVCSFileOperationMatchingUsesDeletedWorkingTreeRows(t *testing.T) {
	deleted := "deleted"
	untracked := "untracked"
	files := []openidea.FileStatus{
		{Path: "/p/src/Main.java", Unstaged: &deleted},
		{Path: "/p/src/Other.java", Unstaged: &untracked},
	}
	got := vcsMatchingFileOperationPaths(
		files,
		[]string{"/p/src"},
		"deleted",
		false,
	)
	if !reflect.DeepEqual(got, []string{"/p/src/Main.java"}) {
		t.Fatalf("deleted paths = %v", got)
	}
}

func TestVCSCommitDraftPersistsPerProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{
		cfg: Config{SettingsPath: path},
		prefs: Preferences{
			VCS: VCSPrefs{CommitDrafts: map[string]string{}},
		},
	}
	root := filepath.Join(t.TempDir(), "project")
	m.keepCommitDraft(root, " fix a bug ")
	if got := m.loadCommitDraft(root); got != "fix a bug" {
		t.Fatalf("draft = %q", got)
	}
	stored := LoadPreferences(path).VCS.CommitDrafts[filepath.Clean(root)]
	if stored != "fix a bug" {
		t.Fatalf("stored draft = %q", stored)
	}
	m.keepCommitDraft(root, "")
	if got := LoadPreferences(path).VCS.CommitDrafts[filepath.Clean(root)]; got != "" {
		t.Fatalf("cleared draft = %q", got)
	}
}

func TestVCSCommitModificationsRunInOpenIDEAOrder(t *testing.T) {
	doc := docbridge.Virtual("/project/App.java", "package demo;\n")
	tab := &Tab{
		doc:   doc,
		state: &editor.State{Buffer: doc.Buffer()},
	}
	var calls []string
	m := &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, input any) (json.RawMessage, error) {
			args := input.(map[string]any)
			switch command {
			case "lsp_code_action_query":
				only := args["only"].([]string)
				calls = append(calls, "query:"+only[0])
				return json.RawMessage(`{"actions":[{"title":"Action","kind":"` + only[0] + `"}]}`), nil
			case "lsp_apply_code_action":
				var action struct {
					Kind string `json:"kind"`
				}
				if err := analysisJSON(args["action"].(json.RawMessage), &action); err != nil {
					t.Fatal(err)
				}
				calls = append(calls, "apply:"+action.Kind)
				doc.Buffer().Edit(editor.Edit{Start: 0, End: 0, Text: " "})
				return json.RawMessage(`null`), nil
			case "lsp_format":
				calls = append(calls, "format")
				return json.RawMessage(`[]`), nil
			default:
				t.Fatalf("command = %q", command)
				return nil, nil
			}
		},
	}
	m.applyCommitModifications(context.Background(), tab, vcsCommitModifications{
		optimizeImports: true,
		cleanup:         true,
		reformat:        true,
	})
	want := []string{
		"query:source.organizeImports",
		"apply:source.organizeImports",
		"query:source.fixAll",
		"apply:source.fixAll",
		"format",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("before-commit modifications = %v, want %v", calls, want)
	}
}

func TestInlineBlameFormattingAndMergedInlay(t *testing.T) {
	now := timeForInlineBlameTest()
	line := openidea.BlameLine{
		Commit: "abc123", Author: "Ada", TimeMs: float64(now.Add(-2 * time.Hour).UnixMilli()),
		Summary: "Fix  the\nparser",
	}
	if got := inlineBlameText(line, now); got != "Ada, 2 hours ago · Fix the parser" {
		t.Fatalf("inline blame = %q", got)
	}
	state := &inlayState{
		inlineBlameHasLine: true,
		inlineBlameInlay:   editor.Inlay{Offset: 4, Text: "blame"},
	}
	if got := mergedEditorInlays(state); len(got) != 1 || got[0].Text != "blame" {
		t.Fatalf("merged inline blame = %+v", got)
	}
}

func timeForInlineBlameTest() time.Time {
	return time.Unix(1_700_000_000, 0)
}

func TestVCSFileOperationNotificationHonorsNeverWithoutContactingBackend(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := &Model{root: root}
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		settingsPageVCSConfirmation: {
			loaded: true,
			applied: map[string]any{
				"vcs.confirmation.onCreate": "never",
			},
			draft: map[string]any{
				"vcs.confirmation.onCreate": "never",
			},
		},
	}
	m.invokeHook = func(context.Context, string, any) (json.RawMessage, error) {
		t.Fatal("Never mode contacted the VCS backend")
		return nil, nil
	}
	m.notifyVCSFileOperation("created", []string{filepath.Join(root, "New.java")})
}
