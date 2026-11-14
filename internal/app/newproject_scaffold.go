package app

// File › New Project's generators after OpenIDEA v0.1.44: the backend's
// ide-scaffold catalog drives the dialog (Java, Spring Boot, Kotlin, Go, …),
// its fields are rendered with their {{template}} defaults, and
// scaffold_create streams the steps while it writes the project. The local
// empty-folder path stays as the fallback when no backend answers.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
)

type scaffoldOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type scaffoldKind struct {
	Type        string           `json:"type"`
	Options     []scaffoldOption `json:"options"`
	SDK         string           `json:"sdk"`
	Pattern     *string          `json:"pattern"`
	Placeholder string           `json:"placeholder"`
	Segmented   bool             `json:"segmented"`
}

type scaffoldField struct {
	ID      string              `json:"id"`
	Label   string              `json:"label"`
	Default string              `json:"default"`
	Hint    string              `json:"hint"`
	Kind    scaffoldKind        `json:"kind"`
	Visible map[string][]string `json:"visible"`
}

type scaffoldGenerator struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Group       string          `json:"group"`
	Git         bool            `json:"git"`
	Dynamic     bool            `json:"dynamic"`
	Order       int             `json:"order"`
	Fields      []scaffoldField `json:"fields"`
}

// loadScaffoldCatalog asks the backend for the generators and selects the
// Java one, as OpenIDEA's New Project opens on it.
func (m *Model) loadScaffoldCatalog() {
	if m.ctx == nil || m.mgr == nil {
		return
	}
	st := &m.newProject
	seq := st.seq
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, "goidea", "scaffold_catalog", nil)
		var list []scaffoldGenerator
		if err == nil {
			err = json.Unmarshal(raw, &list)
		}
		m.post(func() {
			if m.newProject.seq != seq || !m.newProject.open {
				return
			}
			if err != nil {
				m.newProject.catalogErr = "读取项目模板失败：" + err.Error()
				m.invalidate()
				return
			}
			m.newProject.catalog = list
			pick := ""
			for _, g := range list {
				if g.ID == "java" {
					pick = g.ID
				}
			}
			if pick == "" && len(list) > 0 {
				pick = list[0].ID
			}
			if pick != "" {
				m.SelectNewProjectGenerator(pick)
			}
		})
	}()
}

func (m *Model) scaffoldGenerator(id string) *scaffoldGenerator {
	for i := range m.newProject.catalog {
		if m.newProject.catalog[i].ID == id {
			return &m.newProject.catalog[i]
		}
	}
	return nil
}

// SelectNewProjectGenerator switches the wizard's template and renders its
// fields; a dynamic generator (Spring Initializr, Quarkus…) asks the backend
// for the extra fields its service offers.
func (m *Model) SelectNewProjectGenerator(id string) {
	g := m.scaffoldGenerator(id)
	if g == nil {
		return
	}
	st := &m.newProject
	st.generator, st.values, st.steps = id, map[string]string{}, nil
	st.fields = append([]scaffoldField(nil), g.Fields...)
	st.gitInit = st.gitInit && g.Git
	m.invalidate()
	m.refreshScaffoldOptions()
}

// scaffoldValue is a field's value: the user's edit, else its rendered
// default over the fields before it.
func (st *newProjectState) scaffoldValue(id string) string {
	if v, ok := st.values[id]; ok {
		return v
	}
	for _, f := range st.fields {
		if f.ID == id {
			return renderScaffoldTemplate(f.Default, func(name string) string {
				if name == "name" {
					return strings.TrimSpace(st.name)
				}
				return st.scaffoldValue(name)
			})
		}
	}
	return ""
}

// setScaffoldValue edits one field.
func (m *Model) setScaffoldValue(id, value string) {
	if m.newProject.values == nil {
		m.newProject.values = map[string]string{}
	}
	m.newProject.values[id] = value
	// A dynamic generator's fields depend on its server URL; ask again when it
	// changes, as NewProject.tsx does.
	if id == "server" && m.scaffoldGenerator(m.newProject.generator) != nil &&
		m.scaffoldGenerator(m.newProject.generator).Dynamic {
		m.refreshScaffoldOptions()
	}
	m.invalidate()
}

// refreshScaffoldOptions refetches a dynamic generator's fields for the
// current server URL, replacing the ones it added before.
func (m *Model) refreshScaffoldOptions() {
	st := &m.newProject
	g := m.scaffoldGenerator(st.generator)
	if g == nil || !g.Dynamic || m.ctx == nil || m.mgr == nil {
		return
	}
	st.fields = append([]scaffoldField(nil), g.Fields...)
	seq, id := st.seq, st.generator
	request := m.nextRequest()
	values := map[string]string{"server": st.scaffoldValue("server")}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, "goidea", "scaffold_options", map[string]any{
			"generator": id, "values": values, "request": request,
		})
		var fields []scaffoldField
		if err == nil {
			err = json.Unmarshal(raw, &fields)
		}
		m.post(func() {
			if m.newProject.seq != seq || m.newProject.generator != id {
				return
			}
			if err == nil {
				m.newProject.fields = append(m.newProject.fields, fields...)
			}
			m.invalidate()
		})
	}()
}

// scaffoldFieldError checks a text field's pattern.
func (st *newProjectState) scaffoldFieldError(f scaffoldField) string {
	if f.Kind.Type != "text" || f.Kind.Pattern == nil || *f.Kind.Pattern == "" {
		return ""
	}
	re, err := regexp.Compile(*f.Kind.Pattern)
	if err != nil {
		return ""
	}
	v := st.scaffoldValue(f.ID)
	if v == "" {
		return ""
	}
	if re.MatchString(v) {
		return ""
	}
	return "Invalid value"
}

// scaffoldVisible reports whether a field applies to the current values, as
// filters.ts' isVisible does (a C++ standard field only under the C++ choice).
func (st *newProjectState) scaffoldVisible(f scaffoldField) bool {
	for id, allowed := range f.Visible {
		v := st.scaffoldValue(id)
		ok := false
		for _, a := range allowed {
			if a == v {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// scaffoldReady reports whether the backend generators drive the dialog.
func (st *newProjectState) scaffoldReady() bool {
	return len(st.catalog) > 0 && st.generator != "" && !st.busy
}

// createScaffoldProject begins scaffold_create in the background.
func (m *Model) createScaffoldProject() {
	st := &m.newProject
	values := map[string]string{}
	for _, f := range st.fields {
		if !st.scaffoldVisible(f) {
			continue
		}
		if v := st.scaffoldValue(f.ID); v != "" || f.Kind.Type == "text" || f.Kind.Type == "sdk" {
			values[f.ID] = v
		}
	}
	st.busy, st.err, st.steps = true, "", nil
	st.seq++
	seq := st.seq
	channel := m.nextChannel()
	request := m.nextRequest()
	st.channel, st.request = channel, request
	m.invalidate()
	req := map[string]any{
		"generator": st.generator,
		"location":  strings.TrimSpace(st.location),
		"name":      strings.TrimSpace(st.name),
		"git":       st.gitInit,
		"values":    values,
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Minute)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, "goidea", "scaffold_create", map[string]any{
			"req": req, "request": request, "onEvent": map[string]any{"$channel": channel},
		})
		var dir string
		if err == nil {
			err = json.Unmarshal(raw, &dir)
		}
		m.post(func() {
			if m.newProject.seq != seq {
				return
			}
			m.newProject.busy = false
			m.newProject.channel, m.newProject.request = 0, 0
			if err != nil {
				m.newProject.err = "创建失败：" + err.Error()
				m.invalidate()
				return
			}
			m.newProject.open = false
			m.newProject.seq++
			m.openProject(dir, nil)
			m.invalidate()
		})
	}()
}

// acceptScaffoldEvent appends the backend's creation steps.
func (m *Model) acceptScaffoldEvent(data json.RawMessage) {
	var ev struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(data, &ev) != nil || ev.Text == "" {
		return
	}
	st := &m.newProject
	st.steps = append(st.steps, ev.Text)
	if len(st.steps) > 200 {
		st.steps = st.steps[len(st.steps)-200:]
	}
	m.invalidate()
}

// newProjectScaffoldPanel draws the generator list and the selected
// template's fields. It is empty (and the dialog keeps its local fields) when
// the backend did not answer.
func (m *Model) newProjectScaffoldPanel(c *ui.Context, p *ui.Theme) {
	st := &m.newProject
	if len(st.catalog) == 0 {
		if st.catalogErr != "" {
			ui.Text(c, st.catalogErr).FontSize(10.5).TextColor(p.Danger).SingleLine().Ellipsis("…")
		}
		return
	}
	ui.Row(c).Gap(12).Children(func() {
		ui.Column(c).Width(210).Height(190).Children(func() {
			ui.Scroll(c).Height(190).Children(func() {
				ui.Column(c).Children(func() {
					group := ""
					for _, g := range st.catalog {
						g := g
						if g.Group != group {
							group = g.Group
							ui.Text(c, group).FontSize(10).TextColor(p.TextMuted).Padding(6, 4).SingleLine()
						}
						on := st.generator == g.ID
						r := ui.Row(c.Key("scaffold:"+g.ID)).Height(22).Padding(0, 6).
							AlignItems(ui.Center).Cursor(ui.CursorPointer)
						if on {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.Children(func() {
							tone := p.Text
							if !on {
								tone = p.TextMuted
							}
							ui.Text(c, g.Name).FontSize(12).TextColor(tone).SingleLine()
						})
						r.OnClick(func() { m.SelectNewProjectGenerator(g.ID) })
					}
				})
			})
		})
		ui.Column(c).Grow(1).Gap(6).Children(func() {
			for _, f := range st.fields {
				f := f
				if !st.scaffoldVisible(f) {
					continue
				}
				switch f.Kind.Type {
				case "choice":
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, f.Label).FontSize(11).TextColor(p.TextMuted).Width(70).SingleLine()
						cur := st.scaffoldValue(f.ID)
						for _, o := range f.Kind.Options {
							o := o
							on := cur == o.Value
							b := ui.ButtonBase(c.Key("scaffold:"+f.ID+":"+o.Value)).Height(20).
								Padding(0, 6).Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
							if on {
								b.Background(p.SurfaceHover)
								b.BorderWidth(1, 1, 1, 1).BorderColor(p.Accent)
							} else if b.Hovered() {
								b.Background(p.SurfaceHover)
							}
							b.Children(func() {
								ui.Text(c, o.Label).FontSize(10.5).TextColor(p.Text).SingleLine()
							})
							b.OnClick(func() { m.setScaffoldValue(f.ID, o.Value) })
						}
					})
				case "checkbox":
					on := st.scaffoldValue(f.ID) == "true"
					if ui.Checkbox(c, &on, f.Label).Changed() {
						m.setScaffoldValue(f.ID, fmt.Sprint(on))
					}
				default:
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, f.Label).FontSize(11).TextColor(p.TextMuted).Width(70).SingleLine()
						v := st.scaffoldValue(f.ID)
						in := ui.TextInput(c, &v)
						in.OnChange(func() { m.setScaffoldValue(f.ID, v) })
						if f.Kind.Placeholder != "" {
							in.Placeholder(f.Kind.Placeholder)
						}
						if errText := st.scaffoldFieldError(f); errText != "" {
							ui.Text(c, errText).FontSize(10).TextColor(p.Danger).SingleLine()
						}
					})
				}
			}
			if len(st.steps) > 0 {
				last := st.steps[len(st.steps)-1]
				ui.Text(c, last).FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			}
		})
	})
}

// renderScaffoldTemplate renders `{{field|filter}}` over a value lookup, as
// ide-scaffold's template.rs does.
func renderScaffoldTemplate(tmpl string, lookup func(string) string) string {
	return scaffoldTemplateRE.ReplaceAllStringFunc(tmpl, func(m string) string {
		expr := strings.TrimSuffix(strings.TrimPrefix(m, "{{"), "}}")
		parts := strings.Split(expr, "|")
		name := strings.TrimSpace(parts[0])
		v := lookup(name)
		for _, f := range parts[1:] {
			v = scaffoldFilter(strings.TrimSpace(f), v)
		}
		return v
	})
}

var scaffoldTemplateRE = regexp.MustCompile(`\{\{[^}]*\}\}`)

func scaffoldFilter(filter, value string) string {
	switch filter {
	case "path":
		return strings.ReplaceAll(value, ".", "/")
	case "ident":
		var b strings.Builder
		for _, r := range strings.ToLower(value) {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
				b.WriteRune(r)
			}
		}
		s := b.String()
		if s != "" && s[0] >= '0' && s[0] <= '9' {
			s = "_" + s
		}
		return s
	case "kebab", "snake":
		sep := "-"
		if filter == "snake" {
			sep = "_"
		}
		words := scaffoldWords(value)
		for i := range words {
			words[i] = strings.ToLower(words[i])
		}
		return strings.Join(words, sep)
	case "pascal":
		words := scaffoldWords(value)
		for i, w := range words {
			lower := strings.ToLower(w)
			if lower != "" {
				words[i] = strings.ToUpper(lower[:1]) + lower[1:]
			}
		}
		return strings.Join(words, "")
	case "lower":
		return strings.ToLower(value)
	case "json":
		b, _ := json.Marshal(value)
		return strings.Trim(string(b), `"`)
	}
	return value
}

// scaffoldWords splits an identifier into its words.
func scaffoldWords(s string) []string {
	var out []string
	var word []rune
	prevLower := false
	flush := func() {
		if len(word) > 0 {
			out = append(out, string(word))
			word = nil
		}
	}
	for _, c := range s {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			flush()
			prevLower = false
			continue
		}
		upper := unicode.IsUpper(c)
		if upper && prevLower {
			flush()
		}
		prevLower = !upper || unicode.IsDigit(c)
		word = append(word, c)
	}
	flush()
	return out
}

// SmokeScaffoldProject drives File › New Project's generator path headlessly:
// it opens the dialog, waits for the catalog, selects the Java generator and
// creates a project in dir, then waits until the dialog has opened it.
func (m *Model) SmokeScaffoldProject(ctx context.Context, dir string) (map[string]any, error) {
	out := map[string]any{}
	m.post(func() { m.NewProject() })
	deadline := time.Now().Add(30 * time.Second)
	for {
		var n int
		m.post(func() { n = len(m.newProject.catalog) })
		if n > 0 || time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	var catalogErr string
	m.post(func() {
		catalogErr = m.newProject.catalogErr
		m.SelectNewProjectGenerator("java")
		m.newProject.name = "ScaffoldDemo"
		m.newProject.location = dir
		m.newProject.gitInit = true
	})
	if catalogErr != "" {
		return out, fmt.Errorf("%s", catalogErr)
	}
	var n int
	m.post(func() { n = len(m.newProject.catalog) })
	out["generators"] = n
	if n == 0 {
		return out, fmt.Errorf("项目模板清单为空")
	}
	m.post(func() { m.CreateNewProject() })
	deadline = time.Now().Add(15 * time.Minute)
	for {
		var busy, open bool
		var errText, gen string
		var steps int
		m.post(func() {
			busy, open = m.newProject.busy, m.newProject.open
			errText, gen, steps = m.newProject.err, m.newProject.generator, len(m.newProject.steps)
		})
		if errText != "" {
			out["generator"], out["steps"] = gen, steps
			return out, fmt.Errorf("%s", errText)
		}
		if !busy && !open {
			break
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("创建项目超时")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	out["generator"], out["steps"] = "java", 0
	created := dir + "/ScaffoldDemo"
	out["dir"] = created
	if st, err := os.Stat(created); err == nil && st.IsDir() {
		out["created"] = true
	}
	if _, err := os.Stat(created + "/pom.xml"); err == nil {
		out["pom"] = true
	}
	if _, err := os.Stat(created + "/build.gradle"); err == nil {
		out["gradle"] = true
	}
	if _, err := os.Stat(created + "/.git"); err == nil {
		out["git"] = true
	}
	return out, nil
}
