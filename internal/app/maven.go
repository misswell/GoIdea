package app

// The Maven/Gradle tool window after OpenIDEA: the build model the backend
// already computes (build_model) — reactor modules, their coordinates and
// dependency trees — shown in a tree. Nothing is rescanned here; Refresh asks
// the backend again.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
)

type mavenDependency struct {
	GroupID    string            `json:"groupId"`
	ArtifactID string            `json:"artifactId"`
	Version    string            `json:"version"`
	Scope      string            `json:"scope"`
	Type       string            `json:"type"`
	Optional   bool              `json:"optional"`
	Children   []mavenDependency `json:"children"`
}

type mavenProject struct {
	Dir          string            `json:"dir"`
	GroupID      string            `json:"groupId"`
	ArtifactID   string            `json:"artifactId"`
	Version      string            `json:"version"`
	Packaging    string            `json:"packaging"`
	DisplayName  string            `json:"displayName"`
	Dependencies []mavenDependency `json:"dependencies"`
}

type mavenModel struct {
	Projects []mavenProject `json:"projects"`
}

type gradleModel struct {
	Dir      string            `json:"dir"`
	Projects []json.RawMessage `json:"projects"`
	Error    string            `json:"error"`
}

type buildModel struct {
	Maven  *mavenModel   `json:"maven"`
	Gradle []gradleModel `json:"gradle"`
}

// mavenPane is the tool window's state.
type mavenPane struct {
	visible  bool
	loading  bool
	err      string
	model    buildModel
	expanded map[string]bool
	gen      uint64
}

// ShowMaven opens the Maven window and loads the build model.
func (m *Model) ShowMaven() {
	m.maven.visible = true
	m.DockShow("maven")
	m.loadBuildModel()
}

// CloseMaven hides it without side effects.
func (m *Model) CloseMaven() { m.maven.visible = false; m.DockHide("maven") }

// ToggleMaven is the Window menu and palette action.
func (m *Model) ToggleMaven() { m.maven.visible = !m.maven.visible; m.DockToggle("maven") }

// loadBuildModel asks the backend for the reactor and Gradle builds.
func (m *Model) loadBuildModel() {
	if m.maven.loading || m.ctx == nil || m.mgr == nil || m.Root() == "" {
		return
	}
	m.maven.loading = true
	m.maven.err = ""
	m.maven.gen++
	gen := m.maven.gen
	root := m.Root()
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "build_model", nil)
		var model buildModel
		if err == nil {
			err = json.Unmarshal(raw, &model)
		}
		m.post(func() {
			if m.maven.gen != gen {
				return
			}
			m.maven.loading = false
			if m.Root() != root {
				m.maven.model = buildModel{}
				m.maven.err = ""
				m.invalidate()
				return
			}
			if err != nil {
				m.maven.err = "读取构建模型失败：" + err.Error()
			} else {
				m.maven.model = model
			}
			m.invalidate()
		})
	}()
}

// mavenToggle expands or collapses a module's dependencies.
func (m *Model) mavenToggle(dir string) {
	if m.maven.expanded == nil {
		m.maven.expanded = map[string]bool{}
	}
	m.maven.expanded[dir] = !m.maven.expanded[dir]
	m.invalidate()
}

// mavenPanel draws the reactor and dependency trees.
func (m *Model) mavenPanel(c *ui.Context, p *ui.Theme) {
	col := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.Surface).Children(func() {
		ui.Row(c).Height(28).Padding(0, 8).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "Maven").FontSize(12).Bold().TextColor(p.Text).SingleLine()
			if n := len(m.maven.model.MavenProjects()); n > 0 {
				ui.Text(c, fmt.Sprintf("%d modules", n)).FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			}
			ui.Spacer(c)
			if m.maven.loading {
				ui.Text(c, "读取中…").FontSize(10.5).TextColor(p.TextMuted).SingleLine()
			}
			b := ui.ButtonBase(c.Key("maven:refresh")).Height(20).Padding(0, 6).Radius(4).
				AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if b.Hovered() {
				b.Background(p.SurfaceHover)
			}
			b.Children(func() { ui.Text(c, "刷新").FontSize(10.5).TextColor(p.TextMuted).SingleLine() })
			b.OnClick(func() { m.loadBuildModel() })
		})
		if m.maven.err != "" {
			ui.Text(c, m.maven.err).FontSize(11).TextColor(p.Danger).Padding(6, 8).SingleLine().Ellipsis("…")
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(4, 4).Children(func() {
				for _, proj := range m.maven.model.MavenProjects() {
					proj := proj
					mavenProjectRow(c, p, m, proj)
				}
				for _, g := range m.maven.model.Gradle {
					label := g.Dir
					if label == "" {
						label = "Gradle build"
					}
					ui.Row(c).Height(22).Padding(0, 6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, label+" (Gradle)").FontSize(11.5).TextColor(p.Text).SingleLine().Ellipsis("…")
					})
					if g.Error != "" {
						ui.Text(c, g.Error).FontSize(10).TextColor(p.Danger).Padding(0, 16).SingleLine().Ellipsis("…")
					}
				}
				if m.maven.model.Maven == nil && len(m.maven.model.Gradle) == 0 && !m.maven.loading && m.maven.err == "" {
					ui.Text(c, "没有 Maven/Gradle 构建").FontSize(11).TextColor(p.TextMuted).Padding(8, 8).SingleLine()
				}
			})
		})
	})
	_ = col
}

// MavenProjects lists the build model's Maven modules.
func (b *buildModel) MavenProjects() []mavenProject {
	if b.Maven == nil {
		return nil
	}
	return b.Maven.MavenProjects()
}

// MavenProjects lists the reactor's modules in a stable order.
func (m *mavenModel) MavenProjects() []mavenProject {
	if m == nil || len(m.Projects) == 0 {
		return nil
	}
	out := append([]mavenProject(nil), m.Projects...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out
}

func mavenProjectRow(c *ui.Context, p *ui.Theme, m *Model, proj mavenProject) {
	key := proj.Dir
	if key == "" {
		key = proj.ArtifactID
	}
	open := m.maven.expanded[key]
	row := ui.Row(c.Key("maven:"+key)).Height(22).Padding(0, 6).Gap(6).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(p.SurfaceHover)
	}
	row.Children(func() {
		glyph := "▸"
		if open {
			glyph = "▾"
		}
		ui.Text(c, glyph).FontSize(10).TextColor(p.TextMuted)
		ui.Text(c, proj.ArtifactID).FontSize(11.5).TextColor(p.Text).SingleLine().Ellipsis("…")
		if proj.Version != "" {
			ui.Text(c, proj.Version).FontSize(10).TextColor(p.TextMuted).SingleLine()
		}
		ui.Spacer(c)
		ui.Text(c, proj.Packaging).FontSize(10).TextColor(p.TextMuted).SingleLine()
	})
	row.OnClick(func() { m.mavenToggle(key) })
	if !open || len(proj.Dependencies) == 0 {
		return
	}
	for _, dep := range proj.Dependencies {
		mavenDependencyRow(c, p, dep, 1)
	}
}

func mavenDependencyRow(c *ui.Context, p *ui.Theme, dep mavenDependency, depth int) {
	if depth > 8 {
		return
	}
	ui.Row(c).Height(20).Padding(0, float32(6+depth*14)).Gap(6).AlignItems(ui.Center).Children(func() {
		label := dep.GroupID + ":" + dep.ArtifactID
		if dep.Version != "" {
			label += ":" + dep.Version
		}
		ui.Text(c, label).FontSize(10.5).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
		if dep.Scope != "" && dep.Scope != "compile" {
			ui.Text(c, dep.Scope).FontSize(9.5).TextColor(p.TextMuted).SingleLine()
		}
	})
	for _, child := range dep.Children {
		mavenDependencyRow(c, p, child, depth+1)
	}
}

// SmokeMaven loads the build model for the open project and reports its size,
// for the real-server acceptance run.
func (m *Model) SmokeMaven(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	m.post(func() { m.ShowMaven() })
	deadline := time.Now().Add(60 * time.Second)
	for {
		var loading bool
		var errText string
		var n, deps, gradle int
		var visible bool
		m.post(func() {
			loading, errText = m.maven.loading, m.maven.err
			visible = m.maven.visible
			n = len(m.maven.model.MavenProjects())
			gradle = len(m.maven.model.Gradle)
			for _, p := range m.maven.model.MavenProjects() {
				deps += len(p.Dependencies)
			}
		})
		if errText != "" {
			return out, fmt.Errorf("%s", errText)
		}
		if !loading {
			out["modules"], out["dependencies"] = n, deps
			out["gradle"] = gradle
			out["visible"] = visible
			return out, nil
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("读取构建模型超时")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
