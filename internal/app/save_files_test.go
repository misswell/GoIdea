package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestSaveTextCleansTheDocument(t *testing.T) {
	trimModified := SavePrefs{TrimTrailing: true}
	trimAll := SavePrefs{TrimTrailing: true, TrimAllLines: true}
	cases := []struct {
		name string
		text string
		disk string
		p    SavePrefs
		want string
	}{
		{"off", "a \n\n \t\n", "a \n", SavePrefs{}, "a \n\n \t\n"},
		{"modified lines", "one \ntwo  \nthree\t\n", "one\ntwo\nthree\n", trimModified, "one\ntwo\nthree\n"},
		{"untouched lines keep spaces", "keep \nnew  \n", "keep \nold\n", trimModified, "keep \nnew\n"},
		{"added lines count as modified", "keep \nnew  \n", "keep \n", trimModified, "keep \nnew\n"},
		{"all lines", "keep \nnew  \n", "keep \nold\n", trimAll, "keep\nnew\n"},
		{"blank lines drop", "a\n\n \t\n", "a\n", SavePrefs{DropTrailingBlankLines: true}, "a\n"},
		{"blank file stays empty", " \n\t\n", "", SavePrefs{DropTrailingBlankLines: true}, ""},
		{"final newline added", "a", "", SavePrefs{EnsureFinalNewline: true}, "a\n"},
		{"final newline kept", "a\n", "", SavePrefs{EnsureFinalNewline: true}, "a\n"},
		{"empty stays empty", "", "", SavePrefs{EnsureFinalNewline: true}, ""},
		{
			"all together", "a \nb  \n\n\n",
			"a\nb\n", SavePrefs{TrimTrailing: true, DropTrailingBlankLines: true, EnsureFinalNewline: true},
			"a\nb\n",
		},
	}
	for _, c := range cases {
		if got := saveText(c.text, c.disk, c.p); got != c.want {
			t.Errorf("%s: saveText(%q, %q) = %q, want %q", c.name, c.text, c.disk, got, c.want)
		}
	}
}

func TestSaveEditsTurnTheTextIntoTheSavedOne(t *testing.T) {
	p := SavePrefs{TrimTrailing: true}
	text, disk := "one \ntwo  \n", "one\ntwo\n"
	want := "one\ntwo\n"
	edits := saveEdits(text, disk, p)
	if len(edits) != 1 {
		t.Fatalf("edits = %+v, want one", edits)
	}
	e := edits[0]
	got := text[:e.Start] + e.Text + text[e.End:]
	if got != want {
		t.Fatalf("applying %+v gives %q, want %q", e, got, want)
	}
	if e.Start != 3 || e.End != 10 {
		t.Fatalf("edit range = %d..%d, want the changed bytes 3..10", e.Start, e.End)
	}
	if edits := saveEdits(want, disk, p); edits != nil {
		t.Fatalf("clean text makes edits %+v", edits)
	}
}

type saveDoc struct {
	buf     *editor.Buffer
	saved   string
	applied []editor.Edit
}

func (d *saveDoc) Buffer() *editor.Buffer { return d.buf }

func (d *saveDoc) SavedText() string { return d.saved }

func (d *saveDoc) ApplyEdits(_ context.Context, edits ...editor.Edit) error {
	d.applied = append(d.applied, edits...)
	text := d.buf.String()
	for _, e := range edits {
		text = text[:e.Start] + e.Text + text[e.End:]
	}
	d.buf.Reset(text)
	return nil
}

func TestCleanForSaveLeavesTheDocumentReady(t *testing.T) {
	doc := &saveDoc{buf: editor.NewBuffer("a \nb  \n"), saved: "a\nb\n"}
	p := SavePrefs{TrimTrailing: true}
	if err := cleanForSave(context.Background(), doc, p); err != nil {
		t.Fatalf("cleanForSave: %v", err)
	}
	if got := doc.buf.String(); got != "a\nb\n" {
		t.Fatalf("buffer = %q, want the cleaned text", got)
	}
	if len(doc.applied) != 1 {
		t.Fatalf("applied %+v, want one undoable step", doc.applied)
	}

	doc.applied = nil
	doc.buf.Reset("a  \nb  \n")
	if err := cleanForSave(context.Background(), doc, SavePrefs{}); err != nil {
		t.Fatalf("cleanForSave off: %v", err)
	}
	if doc.buf.String() != "a  \nb  \n" || doc.applied != nil {
		t.Fatalf("off prefs touched the document")
	}

	doc.buf.Reset("a\nb\n")
	doc.applied = nil
	if err := cleanForSave(context.Background(), doc, p); err != nil {
		t.Fatalf("cleanForSave clean: %v", err)
	}
	if doc.applied != nil {
		t.Fatalf("clean document took edits %+v", doc.applied)
	}
}

func TestSettingsSaveFilesStageAndApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := &Model{cfg: Config{SettingsPath: path}}
	m.applyPreferences()
	m.OpenSettingsPage(settingsPageGeneral)
	if d := m.settingsDraft; d.trimTrailing || d.trimAllLines || d.dropTrailingBlankLines || d.ensureFinalNewline {
		t.Fatalf("defaults stage %+v", d)
	}

	m.settingsSetTrimTrailing(true)
	m.settingsSetTrimAllLines(true)
	m.settingsSetDropTrailingBlankLines(true)
	m.settingsSetEnsureFinalNewline(true)
	if m.prefs.SaveFiles != (SavePrefs{}) {
		t.Fatalf("staging wrote the prefs: %+v", m.prefs.SaveFiles)
	}
	if !m.settingsDirtyPage(settingsPageGeneral) || !m.settingsPending() {
		t.Fatalf("staged Save Files did not dirty its page")
	}
	m.closeSettings()
	m.OpenSettingsPage(settingsPageGeneral)
	if m.settingsDraft.savePrefs() != (SavePrefs{}) {
		t.Fatalf("cancel kept the staged Save Files: %+v", m.settingsDraft)
	}

	m.settingsSetTrimTrailing(true)
	m.settingsSetTrimAllLines(true)
	m.settingsSetDropTrailingBlankLines(true)
	m.settingsSetEnsureFinalNewline(true)
	m.applySettings()
	want := SavePrefs{
		TrimTrailing: true, TrimAllLines: true,
		DropTrailingBlankLines: true, EnsureFinalNewline: true,
	}
	if m.prefs.SaveFiles != want || LoadPreferences(path).SaveFiles != want {
		t.Fatalf("stored Save Files = %+v / %+v", m.prefs.SaveFiles, LoadPreferences(path).SaveFiles)
	}

	// The All lines choice only counts while trailing spaces are trimmed.
	m.settingsSetTrimTrailing(false)
	if m.settingsDraft.savePrefs().TrimAllLines {
		t.Fatalf("All lines survived the trim switch being off")
	}
	if !m.settingsDirtyPage(settingsPageGeneral) {
		t.Fatalf("turning the trim off is not dirty")
	}

	m.settingsSelectPage(settingsPageGeneral)
	m.settingsResetPage()
	d := m.settingsDraft
	if d.trimTrailing || d.trimAllLines || d.dropTrailingBlankLines || d.ensureFinalNewline {
		t.Fatalf("reset Save Files = %+v", d)
	}
}
