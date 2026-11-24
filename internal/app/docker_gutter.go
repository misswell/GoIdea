package app

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type dockerGutterAction struct {
	title  string
	config openidea.RunConfig
}

func dockerTargetTitle(target dockerTarget, file string) string {
	switch target.kind {
	case "build":
		if target.stage != "" && !target.last {
			return "Build Stage '" + target.stage + "'"
		}
		return "Run '" + dockerBase(file) + "'"
	case "composeAll":
		return "Compose Up (All Services)"
	case "composeService":
		return "Compose Up '" + target.service + "'"
	default:
		return "Run"
	}
}

func dockerProjectPath(root, file string) string {
	file = filepath.Clean(file)
	if strings.TrimSpace(root) == "" {
		return filepath.ToSlash(file)
	}
	rel, err := filepath.Rel(filepath.Clean(root), file)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}

func dockerGutterConfig(
	kind, id, name string,
	params map[string]string,
) (openidea.RunConfig, bool) {
	values := make(map[string]any, len(params))
	for key, value := range params {
		values[key] = value
	}
	cfg := openidea.RunConfig{
		ID:       id,
		Name:     name,
		Kind:     kind,
		Args:     []string{},
		Env:      map[string]string{},
		Detected: false,
		Params:   values,
	}
	prepared, err := dockerPrepareRunConfig(&cfg)
	if err != nil {
		return openidea.RunConfig{}, false
	}
	return *prepared, true
}

// dockerGutterActions mirrors OpenIDEA's gutterActions: every Dockerfile FROM
// has a temporary build configuration, the final stage has both Run and Build,
// and Compose exposes all services or the service on that line.
func (m *Model) dockerGutterActions(target dockerTarget, file string) []dockerGutterAction {
	projectFile := dockerProjectPath(m.Root(), file)
	folder := path.Dir(projectFile)
	if folder == "" {
		folder = "."
	}
	connection := "default"
	if len(m.services.docker.settings.Connections) > 0 {
		if id := strings.TrimSpace(m.services.docker.settings.Connections[0].ID); id != "" {
			connection = id
		}
	}
	switch target.kind {
	case "build":
		base := map[string]string{
			"connection": connection,
			"dockerfile": projectFile,
			"context":    folder,
			"tag":        dockerImageTag(file),
		}
		if target.stage != "" && !target.last {
			params := cloneDockerParams(base)
			params["target"] = target.stage
			cfg, ok := dockerGutterConfig(
				"docker-dockerfile",
				"docker:build:"+projectFile+":"+target.stage,
				projectFile+" ["+target.stage+"]",
				params,
			)
			if !ok {
				return nil
			}
			return []dockerGutterAction{{
				title:  dockerTargetTitle(target, file),
				config: cfg,
			}}
		}
		runParams := cloneDockerParams(base)
		runParams["runImage"] = "true"
		run, ok := dockerGutterConfig(
			"docker-dockerfile",
			"docker:dockerfile:"+projectFile,
			projectFile,
			runParams,
		)
		if !ok {
			return nil
		}
		build, ok := dockerGutterConfig(
			"docker-dockerfile",
			"docker:build:"+projectFile,
			"Build "+projectFile,
			base,
		)
		if !ok {
			return nil
		}
		return []dockerGutterAction{
			{title: dockerTargetTitle(target, file), config: run},
			{title: "Build Image '" + base["tag"] + "'", config: build},
		}
	case "composeAll":
		cfg, ok := dockerGutterConfig(
			"docker-compose",
			"docker:compose:"+projectFile,
			projectFile+": Compose Deployment",
			map[string]string{"connection": connection, "files": projectFile},
		)
		if !ok {
			return nil
		}
		return []dockerGutterAction{{title: dockerTargetTitle(target, file), config: cfg}}
	case "composeService":
		cfg, ok := dockerGutterConfig(
			"docker-compose",
			"docker:compose:"+projectFile+":"+target.service,
			projectFile+": "+target.service,
			map[string]string{
				"connection": connection,
				"files":      projectFile,
				"services":   target.service,
			},
		)
		if !ok {
			return nil
		}
		return []dockerGutterAction{{title: dockerTargetTitle(target, file), config: cfg}}
	}
	return nil
}

func cloneDockerParams(values map[string]string) map[string]string {
	out := make(map[string]string, len(values)+1)
	for key, value := range values {
		out[key] = value
	}
	return out
}

func dockerGutterIcon(target dockerTarget) string {
	if target.kind == "composeAll" {
		return "actions/runAll"
	}
	return "gutter/run"
}

// dockerGutterMenu is the menu opened by a Dockerfile or Compose gutter icon.
// Run starts a temporary run configuration. Modify persists the first one and
// opens it in the native Run/Debug Configurations dialog.
func (m *Model) dockerGutterMenu(menu *ui.Menu, actions []dockerGutterAction) {
	for _, action := range actions {
		action := action
		if menu.Item(action.title).Chosen() {
			m.RunActionWithArg("run.runConfig", map[string]any{
				"config":    action.config,
				"temporary": true,
			})
		}
	}
	if len(actions) == 0 {
		return
	}
	menu.Separator()
	if menu.Item("Modify Run Configuration…").Chosen() {
		m.RunActionWithArg("run.createConfig", actions[0].config)
	}
}

// dockerGutter paints OpenIDEA's run icons over the native editor gutter. MyGo
// exposes line geometry and scrolling but not custom gutter drawings, so these
// remain native menu-button elements positioned from that public geometry.
func (m *Model) dockerGutter(
	c *ui.Context,
	p *ui.Theme,
	tab *Tab,
	editorRow, view ui.Element,
) {
	if tab == nil || tab.doc == nil || tab.state == nil || tab.state.Buffer == nil {
		return
	}
	file := tab.doc.Path()
	text := tab.state.Buffer.Snapshot().String()
	targets := m.dockerFileTargets(file, text)
	if len(targets) == 0 {
		return
	}
	viewBounds, rowBounds := view.Bounds(), editorRow.Bounds()
	if viewBounds.W <= 0 || viewBounds.H <= 0 {
		return
	}
	first, last := tab.state.VisibleLines()
	first = max(0, first)
	last = min(last, tab.state.Buffer.LineCount()-1)
	oi := oiPaletteOf(p)
	for _, target := range targets {
		if target.line < first || target.line > last {
			continue
		}
		top := float32(tab.state.LineTop(target.line) - tab.state.ScrollY())
		height := float32(tab.state.LineTop(target.line+1) - tab.state.LineTop(target.line))
		if height <= 0 {
			height = tab.state.Font.Size * tab.state.LineHeight
		}
		if top+height <= 0 || top >= viewBounds.H {
			continue
		}
		actions := m.dockerGutterActions(target, file)
		if len(actions) == 0 {
			continue
		}
		title := dockerTargetTitle(target, file)
		localX := viewBounds.X - rowBounds.X + 2
		localY := viewBounds.Y - rowBounds.Y + top + max(0, (height-16)/2)
		button := ui.Row(c.Key(fmt.Sprintf("docker-gutter:%s:%d", file, target.line))).
			Absolute().Left(localX).Top(localY).Width(16).Height(16).
			AlignItems(ui.Center).Justify(ui.Center).
			Radius(3).Label(title).Tooltip(title)
		if button.Hovered() {
			button.Background(oi.hoverTint)
		}
		button.Menu(func(menu *ui.Menu) { m.dockerGutterMenu(menu, actions) })
		button.Children(func() {
			oiIconEl(c, dockerGutterIcon(target), 14, oi, oi.textMuted)
		})
	}
}
