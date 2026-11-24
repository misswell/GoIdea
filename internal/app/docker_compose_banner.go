package app

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

// composeDetectionBanner is OpenIDEA's per-editor notice for YAML whose
// contents look like Docker Compose. Named files and remembered choices do not
// show it.
func (m *Model) composeDetectionBanner(c *ui.Context, p *ui.Theme, tab *Tab) {
	if tab == nil || tab.doc == nil {
		return
	}
	path := tab.doc.Path()
	if !isYAMLFile(path) || isNamedComposeFile(path) {
		return
	}
	cleanPath := cleanDockerComposePath(path)
	if m.services.docker.composeOverrides[cleanPath] != "" {
		return
	}
	text := tab.doc.Text()
	if !isComposeContent(text) {
		return
	}

	oi := oiPaletteOf(p)
	ui.Row(c.Key("docker-compose-auto-detect:"+cleanPath)).Shrink(0).
		Padding(6, 10).Gap(8).AlignItems(ui.Center).
		Background(oi.hoverTint).BorderWidth(0, 0, 1, 0).BorderColor(oi.border).
		Children(func() {
			oiIconEl(c, "status/info", 16, oi, p.Accent)
			ui.Text(c, "This file was auto-detected as a Docker Compose configuration based on its content").
				FontSize(12).TextColor(oi.textStrong).SingleLine().Ellipsis("…").Grow(1)
			ui.Button(c, "Not a Compose file").OnClick(func() {
				m.setDockerComposeFileOverride(map[string]any{
					"path": cleanPath, "status": "excluded",
				})
			})
			ui.Button(c, "Keep as Docker Compose").OnClick(func() {
				m.setDockerComposeFileOverride(map[string]any{
					"path": cleanPath, "status": "kept",
				})
			})
		})
}

func cleanDockerComposePath(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func dockerComposeAutoDetected(path, text string, overrides map[string]string) bool {
	if !isYAMLFile(path) || isNamedComposeFile(path) {
		return false
	}
	if overrides[cleanDockerComposePath(path)] != "" {
		return false
	}
	return isComposeContent(text)
}
