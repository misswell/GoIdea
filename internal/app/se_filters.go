package app

// Search Everywhere's funnel, after filters.ts: excluded file types and symbol
// kinds on the relevant tabs and a file mask on Text.

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

// seFilters is the funnel's stored state, per tab.
type seFilters struct {
	// Files, Classes and Symbols list excluded language ids.
	FilesLanguages   []string `json:"filesLanguages,omitempty"`
	ClassesLanguages []string `json:"classesLanguages,omitempty"`
	SymbolsLanguages []string `json:"symbolsLanguages,omitempty"`
	// Classes and Symbols list the excluded symbol kinds, so a newly known
	// kind shows by default.
	Classes []string `json:"classes,omitempty"`
	Symbols []string `json:"symbols,omitempty"`
	// Text is the comma-separated glob mask; empty includes every file.
	Text string `json:"text,omitempty"`
}

// seClassKinds and seSymbolKinds are the kinds the menu offers, in IDEA's
// order, as CLASS_KINDS and SYMBOL_KINDS.
var seClassKinds = []string{"class", "interface", "struct", "enum", "trait", "typeAlias"}

var seSymbolKinds = append(append([]string{}, seClassKinds...),
	"function", "method", "constructor", "field", "property", "variable",
	"constant", "enumMember", "module", "namespace", "macro")

var seKindLabels = map[string]string{
	"class":       "Classes",
	"interface":   "Interfaces",
	"struct":      "Structs",
	"enum":        "Enums",
	"trait":       "Traits",
	"typeAlias":   "Type Aliases",
	"function":    "Functions",
	"method":      "Methods",
	"constructor": "Constructors",
	"field":       "Fields",
	"property":    "Properties",
	"variable":    "Variables",
	"constant":    "Constants",
	"enumMember":  "Enum Members",
	"module":      "Modules",
	"namespace":   "Namespaces",
	"macro":       "Macros",
}

// seFilterKinds is the kinds a tab offers, empty for the tabs without a kind
// filter.
func seFilterKinds(tab seTab) []string {
	switch tab {
	case seTabClasses:
		return seClassKinds
	case seTabSymbols:
		return seSymbolKinds
	}
	return nil
}

// seFilterTab reports whether the tab has a funnel at all.
func seFilterTab(tab seTab) bool {
	return tab == seTabFiles || tab == seTabClasses || tab == seTabSymbols || tab == seTabText
}

func (f seFilters) languages(tab seTab) []string {
	switch tab {
	case seTabFiles:
		return f.FilesLanguages
	case seTabClasses:
		return f.ClassesLanguages
	case seTabSymbols:
		return f.SymbolsLanguages
	}
	return nil
}

func (f seFilters) kinds(tab seTab) []string {
	switch tab {
	case seTabClasses:
		return f.Classes
	case seTabSymbols:
		return f.Symbols
	}
	return nil
}

// active reports whether the tab's filter would drop anything.
func (f seFilters) active(tab seTab) bool {
	return len(f.languages(tab)) > 0 || len(f.kinds(tab)) > 0 ||
		(tab == seTabText && strings.TrimSpace(f.Text) != "")
}

// languageExcluded drops paths whose longest registered suffix belongs to an
// excluded language. Unknown paths stay visible, as OpenIDEA's passesFilter.
func (f seFilters) languageExcluded(tab seTab, path string, languages []openidea.LanguageInfo) bool {
	id := seLanguageOf(path, languages)
	if id == "" {
		return false
	}
	for _, excluded := range f.languages(tab) {
		if excluded == id {
			return true
		}
	}
	return false
}

// seLanguageOf matches the longest suffix first, so d.ts wins over ts.
func seLanguageOf(path string, languages []openidea.LanguageInfo) string {
	name := strings.ToLower(filepath.Base(path))
	for dot := strings.IndexByte(name, '.'); dot >= 0; {
		suffix := name[dot+1:]
		for _, language := range languages {
			for _, extension := range language.Extensions {
				if strings.TrimPrefix(strings.ToLower(extension), ".") == suffix {
					return language.ID
				}
			}
		}
		next := strings.IndexByte(name[dot+1:], '.')
		if next < 0 {
			break
		}
		dot += next + 1
	}
	return ""
}

// kindExcluded reports whether a symbol kind is filtered out. Kinds the menu
// does not list fall under the closest listed one, as filters.ts buckets them.
func (f seFilters) kindExcluded(tab seTab, kind string) bool {
	kind = seKindBucket(kind)
	for _, k := range f.kinds(tab) {
		if k == kind {
			return true
		}
	}
	return false
}

func seKindBucket(kind string) string {
	switch kind {
	case "package":
		return "module"
	case "typeParameter":
		return "typeAlias"
	}
	return kind
}

// toggleKind includes or excludes one kind of a tab.
func (f seFilters) toggleKind(tab seTab, kind string) seFilters {
	list := f.kinds(tab)
	kept := make([]string, 0, len(list)+1)
	found := false
	for _, k := range list {
		if k == kind {
			found = true
		} else {
			kept = append(kept, k)
		}
	}
	if !found {
		kept = append(kept, kind)
	}
	switch tab {
	case seTabClasses:
		f.Classes = kept
	case seTabSymbols:
		f.Symbols = kept
	}
	return f
}

func (f seFilters) toggleLanguage(tab seTab, id string) seFilters {
	list := f.languages(tab)
	kept := make([]string, 0, len(list)+1)
	found := false
	for _, language := range list {
		if language == id {
			found = true
		} else {
			kept = append(kept, language)
		}
	}
	if !found {
		kept = append(kept, id)
	}
	switch tab {
	case seTabFiles:
		f.FilesLanguages = kept
	case seTabClasses:
		f.ClassesLanguages = kept
	case seTabSymbols:
		f.SymbolsLanguages = kept
	}
	return f
}

// clearTab removes one tab's filter, as Reset Filter does.
func (f seFilters) clearTab(tab seTab) seFilters {
	switch tab {
	case seTabFiles:
		f.FilesLanguages = nil
	case seTabClasses:
		f.Classes = nil
		f.ClassesLanguages = nil
	case seTabSymbols:
		f.Symbols = nil
		f.SymbolsLanguages = nil
	case seTabText:
		f.Text = ""
	}
	return f
}

// seTextMaskMatches reports whether a path passes the Text tab's mask:
// comma-separated globs, "!" excluding, matched against the file name or the
// whole path. An empty mask includes every file; only negative globs include
// everything they do not name.
func seTextMaskMatches(mask, path string) bool {
	mask = strings.TrimSpace(mask)
	if mask == "" {
		return true
	}
	include, exclude, positive := false, false, false
	for _, part := range strings.Split(mask, ",") {
		p := strings.TrimSpace(part)
		neg := strings.HasPrefix(p, "!")
		if neg {
			p = strings.TrimSpace(strings.TrimPrefix(p, "!"))
		}
		if p == "" {
			continue
		}
		if !neg {
			positive = true
		}
		if seGlobMatches(p, path) {
			if neg {
				exclude = true
			} else {
				include = true
			}
		}
	}
	if exclude {
		return false
	}
	if positive {
		return include
	}
	return true
}

func seGlobMatches(pattern, path string) bool {
	if ok, err := filepath.Match(pattern, filepath.Base(path)); err == nil && ok {
		return true
	}
	ok, err := filepath.Match(pattern, path)
	return err == nil && ok
}

// saveSearchFilters stores the funnel in the settings, as OpenIDEA keeps it
// across starts.
func (m *Model) saveSearchFilters() {
	f := m.seFilters
	m.prefs.SearchFilters = &f
	if m.cfg.SettingsPath != "" {
		_ = m.savePreferences()
	}
}

// seFilterPanel draws file types and symbol kinds, or the Text mask.
func (m *Model) seFilterPanel(c *ui.Context, p *ui.Theme, s *sePane) {
	col := ui.Column(c).Gap(2).Padding(6, 4).Radius(6).Background(p.Surface)
	col.Children(func() {
		if s.tab == seTabText {
			ui.Text(c, "File mask").FontSize(12).TextColor(p.TextMuted).SingleLine()
			in := ui.TextInput(c, &s.filters.Text)
			in.OnChange(func() {
				m.seFilters = s.filters
				m.saveSearchFilters()
				m.refreshSearchEverywhere()
			})
		} else {
			switch {
			case !s.languagesDone && s.languagesBusy:
				ui.Text(c, "Loading file types…").FontSize(12).TextColor(p.TextMuted).SingleLine()
			case s.languagesError != "":
				ui.Text(c, s.languagesError).FontSize(12).TextColor(p.TextMuted).SingleLine()
			case len(s.languages) == 0:
				ui.Text(c, "No file types are available.").FontSize(12).TextColor(p.TextMuted).SingleLine()
			default:
				ui.Text(c, "File types").FontSize(12).TextColor(p.TextMuted).SingleLine()
				for _, language := range s.languages {
					language := language
					id := language.ID
					on := true
					for _, excluded := range s.filters.languages(s.tab) {
						if excluded == id {
							on = false
							break
						}
					}
					m.seFilterCheckbox(c, p, s, "language:"+id, seLanguageLabel(language), on, func() {
						s.filters = s.filters.toggleLanguage(s.tab, id)
					})
				}
			}
			if kinds := seFilterKinds(s.tab); len(kinds) > 0 {
				ui.Text(c, "Show symbol kinds").FontSize(12).TextColor(p.TextMuted).SingleLine()
				for _, kind := range kinds {
					kind := kind
					label := seKindLabels[kind]
					on := !s.filters.kindExcluded(s.tab, kind)
					m.seFilterCheckbox(c, p, s, "kind:"+kind, label, on, func() {
						s.filters = s.filters.toggleKind(s.tab, kind)
					})
				}
			}
		}
		if s.filters.active(s.tab) {
			r := ui.Row(c.Key("se:filter:reset")).Height(22).Padding(0, 6).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if r.Hovered() {
				r.Background(p.SurfaceHover)
			}
			r.Children(func() {
				ui.Text(c, "Reset Filter").FontSize(12).TextColor(p.TextMuted).SingleLine()
			})
			r.OnClick(func() {
				s.filters = s.filters.clearTab(s.tab)
				m.seFilters = s.filters
				m.saveSearchFilters()
				m.refreshSearchEverywhere()
			})
		}
	})
}

func seLanguageLabel(language openidea.LanguageInfo) string {
	if strings.TrimSpace(language.Name) != "" {
		return language.Name
	}
	return language.ID
}

func (m *Model) seFilterCheckbox(c *ui.Context, p *ui.Theme, s *sePane, id, label string, on bool, run func()) {
	b := ui.ButtonBase(c.Key("se:filter:"+string(s.tab)+":"+id)).Height(22).
		Padding(0, 6).Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(p.SurfaceHover)
	}
	b.Children(func() {
		box := "☐"
		if on {
			box = "☑"
		}
		ui.Text(c, box+" "+label).FontSize(12).TextColor(p.Text).SingleLine()
	})
	b.OnClick(func() {
		run()
		m.seFilters = s.filters
		m.saveSearchFilters()
		m.refreshSearchEverywhere()
	})
}
