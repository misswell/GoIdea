package app

// Scratch Files (Cmd+Shift+N) after OpenIDEA's feature: throwaway files of a
// chosen language in <data>/scratches, outside any project, openable like a
// normal document. The chooser lists recently used languages first and filters
// by name, fuzzy prefix or extension.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"

	"github.com/alecthomas/chroma/v2/lexers"
)

// maxScratchRecent is how many languages the chooser remembers, as upstream's
// MAX_RECENT.
const maxScratchRecent = 5

// scratchLanguage is a language a scratch file can be created for: the first
// extension it lists, and its scratch template where the definition has one.
type scratchLanguage struct {
	id, name, ext, template string
}

// scratchChooser is the New Scratch File popup's state.
type scratchChooser struct {
	open      bool
	loading   bool
	err       string
	languages []scratchLanguage
	filter    string
	active    int
}

// OpenScratchChooser is File › New Scratch File…: the chooser reads the
// backend language registry, including OpenIDEA's per-language scratch
// templates, and keeps the local lexer list visible while it loads.
func (m *Model) OpenScratchChooser() {
	v := &scratchChooser{
		open:      true,
		loading:   m.mgr != nil || m.invokeHook != nil,
		languages: scratchLanguagesLocal(),
	}
	m.scratch = v
	m.invalidate()
	if !v.loading {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		languages, err := m.toolLanguages(ctx)
		var scratchLanguages []scratchLanguage
		if err == nil {
			scratchLanguages = scratchLanguagesFromBackend(languages)
		}
		m.post(func() {
			if m.scratch != v {
				return
			}
			v.loading = false
			if err != nil {
				if len(v.languages) == 0 {
					v.err = "Could not read languages: " + err.Error()
				}
			} else if len(scratchLanguages) > 0 {
				v.languages = scratchLanguages
				v.active = 0
			}
			m.invalidate()
		})
	}()
}

// closeScratchChooser drops the popup without creating anything.
func (m *Model) closeScratchChooser() {
	m.scratch = nil
	m.invalidate()
}

// scratchLanguagesLocal is the chooser's list: each language this editor can
// highlight, with its first plain extension, sorted by name.
func scratchLanguagesLocal() []scratchLanguage {
	seen := make(map[string]bool)
	out := make([]scratchLanguage, 0, 64)
	for _, l := range lexers.GlobalLexerRegistry.Lexers {
		cfg := l.Config()
		ext := scratchExtension(cfg.Filenames)
		if ext == "" || seen[ext] {
			continue
		}
		seen[ext] = true
		name := highlight.DisplayName(l)
		if name == "" {
			name = cfg.Name
		}
		out = append(out, scratchLanguage{id: strings.ToLower(cfg.Name), name: name, ext: ext})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].name) < strings.ToLower(out[j].name)
	})
	return out
}

// scratchExtension is the first plain "*.ext" pattern of a lexer's filenames.
func scratchExtension(patterns []string) string {
	for _, p := range patterns {
		if !strings.HasPrefix(p, "*.") {
			continue
		}
		ext := strings.TrimPrefix(p, "*.")
		if ext == "" || strings.ContainsAny(ext, "*?[]") {
			continue
		}
		return ext
	}
	return ""
}

// scratchLanguagesFromBackend maps the backend's UI language catalog to the
// chooser rows. The registry is authoritative for ids and templates, just as
// OpenIDEA's `languages()` command is.
func scratchLanguagesFromBackend(languages []openIDEALanguageInfo) []scratchLanguage {
	out := make([]scratchLanguage, 0, len(languages))
	for _, language := range languages {
		ext := ""
		for _, extension := range language.Extensions {
			if ext = strings.TrimPrefix(strings.TrimSpace(extension), "."); ext != "" {
				break
			}
		}
		if language.ID == "" || language.Name == "" || ext == "" {
			continue
		}
		out = append(out, scratchLanguage{
			id:       language.ID,
			name:     language.Name,
			ext:      ext,
			template: language.ScratchTemplate,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].name) < strings.ToLower(out[j].name)
	})
	return out
}

// scratchRows is the chooser's list: recently used languages first when the
// filter is empty; otherwise prefix matches, then fuzzy ones, then matches on
// the extension or id.
func (m *Model) scratchRows() []scratchLanguage {
	v := m.scratch
	if v == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(v.filter))
	if q == "" {
		byID := make(map[string]scratchLanguage, len(v.languages))
		for _, l := range v.languages {
			byID[l.id] = l
		}
		seen := make(map[string]bool, len(v.languages))
		out := make([]scratchLanguage, 0, len(v.languages))
		for _, id := range m.prefs.ScratchRecent {
			if l, ok := byID[id]; ok && !seen[id] {
				out = append(out, l)
				seen[id] = true
			}
		}
		for _, l := range v.languages {
			if !seen[l.id] {
				out = append(out, l)
			}
		}
		return out
	}
	var prefix, fuzzy, ext []scratchLanguage
	for _, l := range v.languages {
		name := strings.ToLower(l.name)
		switch {
		case strings.HasPrefix(name, q):
			prefix = append(prefix, l)
		case isSubsequence(name, q):
			fuzzy = append(fuzzy, l)
		case strings.HasPrefix(strings.ToLower(l.ext), q), strings.HasPrefix(strings.ToLower(l.id), q):
			ext = append(ext, l)
		}
	}
	return append(append(prefix, fuzzy...), ext...)
}

// isSubsequence reports whether q's characters appear in s in order.
func isSubsequence(s, q string) bool {
	if q == "" {
		return true
	}
	at := 0
	for _, r := range s {
		if r == rune(q[at]) {
			at++
			if at == len(q) {
				return true
			}
		}
	}
	return false
}

// SetScratchFilter filters the chooser's list and puts the highlight back at
// its top.
func (m *Model) SetScratchFilter(filter string) {
	if m.scratch == nil {
		return
	}
	m.scratch.filter = filter
	m.scratch.active = 0
	m.invalidate()
}

// stepScratch moves the chooser's highlight, wrapping at both ends.
func (m *Model) stepScratch(dir int) {
	v := m.scratch
	if v == nil {
		return
	}
	rows := m.scratchRows()
	if len(rows) == 0 {
		v.active = 0
		return
	}
	v.active = (v.active + dir + len(rows)) % len(rows)
	m.invalidate()
}

// CreateScratchSelected creates the highlighted language's scratch file.
func (m *Model) CreateScratchSelected() {
	rows := m.scratchRows()
	if len(rows) == 0 {
		return
	}
	active := m.scratch.active
	if active < 0 || active >= len(rows) {
		active = 0
	}
	m.CreateScratchLanguage(rows[active].id)
}

// CreateScratchLanguage creates scratch[_n].<ext> for the language, remembers
// the choice and opens the file with the caret where its template puts it.
func (m *Model) CreateScratchLanguage(id string) {
	var lang scratchLanguage
	found := false
	if m.scratch != nil {
		for _, l := range m.scratch.languages {
			if l.id == id {
				lang, found = l, true
				break
			}
		}
	}
	if !found {
		return
	}
	m.scratch = nil
	m.rememberScratchLanguage(id)
	path, line, col, err := m.createScratchFile(lang)
	if err != nil {
		m.status = "Could not create the scratch file: " + err.Error()
		m.invalidate()
		return
	}
	m.status = "Created " + filepath.Base(path)
	m.revealAt(path, line, col)
	m.invalidate()
}

// scratchDir is <data>/scratches, beside the settings file.
func (m *Model) scratchDir() (string, error) {
	if m.cfg.SettingsPath == "" {
		return "", fmt.Errorf("No data directory is available")
	}
	return filepath.Join(filepath.Dir(m.cfg.SettingsPath), "scratches"), nil
}

// createScratchFile writes the language's scratch file and returns its path
// with the caret position its template asks for.
func (m *Model) createScratchFile(lang scratchLanguage) (string, int, int, error) {
	dir, err := m.scratchDir()
	if err != nil {
		return "", 0, 0, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", 0, 0, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	path := filepath.Join(dir, scratchFileName(names, lang.ext))
	text, line, col := scratchContent(lang.template)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", 0, 0, err
	}
	return path, line, col, nil
}

// scratchFileName is IDEA's naming: scratch.<ext>, then scratch_1.<ext>,
// scratch_2.<ext>…
func scratchFileName(existing []string, ext string) string {
	taken := make(map[string]bool, len(existing))
	for _, n := range existing {
		taken[strings.ToLower(n)] = true
	}
	suffix := ""
	if ext != "" {
		suffix = "." + ext
	}
	if !taken[strings.ToLower("scratch"+suffix)] {
		return "scratch" + suffix
	}
	for i := 1; ; i++ {
		name := fmt.Sprintf("scratch_%d%s", i, suffix)
		if !taken[strings.ToLower(name)] {
			return name
		}
	}
}

// scratchContent is a new file's text and caret from its template, where $END$
// marks the caret.
func scratchContent(template string) (string, int, int) {
	const caret = "$END$"
	at := strings.Index(template, caret)
	if at < 0 {
		return template, 0, 0
	}
	before := template[:at]
	text := before + strings.ReplaceAll(template[at+len(caret):], caret, "")
	lines := strings.Split(before, "\n")
	last := lines[len(lines)-1]
	return text, len(lines) - 1, int(utf16Len(last))
}

// rememberScratchLanguage puts the language first in the chooser's recent list
// and stores it, as the page keeps its recent languages across starts.
func (m *Model) rememberScratchLanguage(id string) {
	recent := []string{id}
	for _, r := range m.prefs.ScratchRecent {
		if r != id && len(recent) < maxScratchRecent {
			recent = append(recent, r)
		}
	}
	m.prefs.ScratchRecent = recent
	m.savePreferences()
}

// scratchChooserDialog draws the New Scratch File popup: the filter, the
// language list and its empty state.
func (m *Model) scratchChooserDialog(c *ui.Context, p *ui.Theme) {
	v := m.scratch
	if v == nil || !v.open {
		return
	}
	open := v.open
	d := nativeModal(c, &open, func() {
		ui.Column(c).Width(420).Gap(8).Children(func() {
			ui.Text(c, "New Scratch File").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &v.filter).AutoFocus()
			if in.Changed() {
				m.SetScratchFilter(v.filter)
			}
			if in.Submitted() {
				m.CreateScratchSelected()
			}
			switch {
			case v.loading:
				ui.Text(c, "Reading the languages…").FontSize(12).TextColor(p.TextMuted).SingleLine()
			case v.err != "":
				ui.Text(c, v.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			default:
				rows := m.scratchRows()
				if len(rows) == 0 {
					ui.Text(c, "No matching languages").FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
				first := v.active - 5
				if first < 0 {
					first = 0
				}
				if first > len(rows)-10 {
					first = max(0, len(rows)-10)
				}
				last := min(len(rows), first+10)
				ui.Column(c).Gap(0).Children(func() {
					for i := first; i < last; i++ {
						i, lang := i, rows[i]
						row := ui.Row(c.Key("scratch:"+lang.id)).Height(24).Padding(0, 6).
							Gap(8).AlignItems(ui.Center)
						if i == v.active {
							row = row.Background(p.SurfaceHover)
						}
						row.Children(func() {
							ui.Text(c, lang.name).FontSize(12).TextColor(p.Text).
								SingleLine().Grow(1)
							ui.Text(c, "."+lang.ext).FontSize(12).TextColor(p.TextMuted).SingleLine()
						})
						if row.Clicked() {
							m.CreateScratchLanguage(lang.id)
						}
					}
				})
			}
		})
	})
	if (d.Dismissed() || !open) && !v.loading {
		m.closeScratchChooser()
	}
}

// SmokeScratch opens the chooser, selects Java when the backend registry
// provides it, and checks the backend template, caret and opened document.
func (m *Model) SmokeScratch(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	var origPath string
	m.post(func() {
		if t := m.tab(); t != nil {
			origPath = t.doc.Path()
		}
		m.OpenScratchChooser()
	})
	deadline := time.Now().Add(60 * time.Second)
	for {
		ready, failed := false, ""
		var count int
		m.post(func() {
			if v := m.scratch; v != nil {
				ready = !v.loading
				failed = v.err
				count = len(v.languages)
			}
		})
		if failed != "" {
			out["error"] = failed
			return out, nil
		}
		if ready {
			out["languages"] = count
			break
		}
		if time.Now().After(deadline) {
			return out, errors.New("Reading the language list timed out")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	var selected scratchLanguage
	m.post(func() {
		if m.scratch == nil {
			return
		}
		for _, language := range m.scratch.languages {
			if language.id == "java" {
				selected = language
				break
			}
		}
		if selected.id == "" && len(m.scratch.languages) > 0 {
			selected = m.scratch.languages[0]
		}
	})
	if selected.id == "" {
		out["error"] = "No language is available"
		return out, nil
	}
	out["language"] = selected.id
	out["extension"] = selected.ext
	out["template"] = selected.template != ""
	expectedText, expectedLine, expectedCol := scratchContent(selected.template)
	expectedCaret := strings.Index(selected.template, "$END$")
	if expectedCaret < 0 {
		expectedCaret = 0
	}
	m.post(func() { m.CreateScratchLanguage(selected.id) })
	dir, _ := m.scratchDir()
	wantDir := realPath(dir)
	deadline = time.Now().Add(60 * time.Second)
	for {
		var path string
		m.post(func() {
			if t := m.tab(); t != nil {
				path = t.doc.Path()
			}
		})
		if path != "" && strings.HasPrefix(realPath(path), wantDir) {
			out["path"] = path
			out["name"] = filepath.Base(path)
			out["opened"] = true
			out["inScratchDir"] = true
			if raw, err := os.ReadFile(path); err == nil {
				out["templateUsed"] = string(raw) == expectedText
			}
			out["line"] = expectedLine
			out["column"] = expectedCol
			break
		}
		if time.Now().After(deadline) {
			out["opened"] = false
			if path != "" {
				out["path"] = path
				out["name"] = filepath.Base(path)
			}
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	if out["opened"] == true {
		deadline = time.Now().Add(5 * time.Second)
		for {
			var caret int
			m.post(func() {
				if t := m.tab(); t != nil {
					caret = t.state.Selection().Head
				}
			})
			out["caret"] = caret
			out["caretExpected"] = expectedCaret
			out["caretUsed"] = caret == expectedCaret
			if caret == expectedCaret || time.Now().After(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	// Put the active tab back, so the phases after this one see the file the
	// harness opened.
	if origPath != "" {
		want := realPath(origPath)
		m.post(func() { m.openFile(origPath) })
		deadline = time.Now().Add(30 * time.Second)
		for {
			var path string
			m.post(func() {
				if t := m.tab(); t != nil {
					path = t.doc.Path()
				}
			})
			if realPath(path) == want || time.Now().After(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(150 * time.Millisecond):
			}
		}
	}
	out["recent"] = LoadPreferences(m.cfg.SettingsPath).ScratchRecent
	return out, nil
}
