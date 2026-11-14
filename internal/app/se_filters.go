package app

// Search Everywhere's funnel, after filters.ts: excluded symbol kinds on the
// Classes and Symbols tabs and a file mask on Text. Upstream's language half
// needs a language list the backend does not expose, so it is left out rather
// than faked; the kinds and the mask work on the results already loaded.

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// seFilters is the funnel's stored state, per tab.
type seFilters struct {
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
	return tab == seTabClasses || tab == seTabSymbols || tab == seTabText
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
	return len(f.kinds(tab)) > 0 || (tab == seTabText && strings.TrimSpace(f.Text) != "")
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

// clearTab removes one tab's filter, as Reset Filter does.
func (f seFilters) clearTab(tab seTab) seFilters {
	switch tab {
	case seTabClasses:
		f.Classes = nil
	case seTabSymbols:
		f.Symbols = nil
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
		_ = m.prefs.Save(m.cfg.SettingsPath)
	}
}

// seFilterPanel draws the funnel's panel under the tab row: a checkbox per
// symbol kind, or the Text tab's mask field.
func (m *Model) seFilterPanel(c *ui.Context, p *ui.Theme, s *sePane) {
	col := ui.Column(c).Gap(2).Padding(6, 4).Radius(6).Background(p.Surface)
	col.Children(func() {
		if s.tab == seTabText {
			ui.Text(c, "File mask").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			in := ui.TextInput(c, &s.filters.Text)
			in.OnChange(func() {
				m.seFilters = s.filters
				m.saveSearchFilters()
				m.refreshSearchEverywhere()
			})
		} else {
			ui.Text(c, "Show symbol kinds").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			for _, kind := range seFilterKinds(s.tab) {
				kind := kind
				on := !s.filters.kindExcluded(s.tab, kind)
				b := ui.ButtonBase(c.Key("se:filter:"+string(s.tab)+":"+kind)).Height(22).
					Padding(0, 6).Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if b.Hovered() {
					b.Background(p.SurfaceHover)
				}
				b.Children(func() {
					box := "☐"
					if on {
						box = "☑"
					}
					ui.Text(c, box+" "+seKindLabels[kind]).FontSize(11.5).TextColor(p.Text).SingleLine()
				})
				b.OnClick(func() {
					s.filters = s.filters.toggleKind(s.tab, kind)
					m.seFilters = s.filters
					m.saveSearchFilters()
					m.refreshSearchEverywhere()
				})
			}
		}
		if s.filters.active(s.tab) {
			r := ui.Row(c.Key("se:filter:reset")).Height(22).Padding(0, 6).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if r.Hovered() {
				r.Background(p.SurfaceHover)
			}
			r.Children(func() {
				ui.Text(c, "Reset Filter").FontSize(11.5).TextColor(p.TextMuted).SingleLine()
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
