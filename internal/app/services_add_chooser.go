package app

import (
	"github.com/egoist/mygo/ui"

	"mayoi/internal/adapter"
)

type serviceAddChooserState struct {
	open bool
}

// serviceAddChooserDialog is the command-palette and action-registry path for
// OpenIDEA's services.addService. The tree's + button opens the same choices
// as a menu; this native sheet makes the action usable without a pointer.
func (m *Model) serviceAddChooserDialog(c *ui.Context, p *ui.Theme) {
	state := m.services.addChooser
	if state == nil {
		return
	}
	nativeModal(c.Key("services:add-service-chooser"), &state.open, func() {
		ui.Column(c).Width(460).Gap(10).Children(func() {
			ui.Text(c, "Add Service").FontSize(14).Bold().TextColor(p.Text)
			ui.Text(c, "Choose a run configuration type or a Docker provider entry.").
				FontSize(12).TextColor(p.TextMuted)
			ui.Scroll(c).Height(360).Children(func() {
				ui.Column(c).Gap(6).Children(func() {
					project, rest := m.serviceAddKinds()
					for _, group := range append(project, rest...) {
						if len(group.kinds) == 0 {
							continue
						}
						ui.Text(c, group.name).FontSize(11).Bold().TextColor(p.TextMuted)
						for _, kind := range group.kinds {
							kind := kind
							if ui.Button(c, "Add "+adapter.KindLabel(kind)).Clicked() {
								state.open = false
								m.services.addChooser = nil
								m.beginServiceImport(kind)
							}
						}
					}
					ui.Text(c, "Docker").FontSize(11).Bold().TextColor(p.TextMuted)
					if ui.Button(c, "Add Docker Connection…").Clicked() {
						state.open = false
						m.services.addChooser = nil
						m.editDockerConnection(nil)
					}
					if ui.Button(c, "Add Docker Registry…").Clicked() {
						state.open = false
						m.services.addChooser = nil
						m.editDockerRegistry(nil)
					}
				})
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Close").Clicked() {
					state.open = false
				}
			})
		})
	})
	if !state.open {
		m.services.addChooser = nil
		m.invalidate()
	}
}
