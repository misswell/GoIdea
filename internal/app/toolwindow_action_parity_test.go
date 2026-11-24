package app

import "testing"

func TestOpenIDEAToolWindowActionsUseTheNativeCommandPath(t *testing.T) {
	tests := []struct {
		id   string
		open func(*Model) bool
	}{
		{
			id: "toolwindow.project",
			open: func(m *Model) bool {
				return m.sideMode == "" && !m.leftHidden
			},
		},
		{
			id: "toolwindow.find",
			open: func(m *Model) bool {
				return m.dockWindowShown("find")
			},
		},
		{
			id: "toolwindow.run",
			open: func(m *Model) bool {
				return m.dockWindowShown("run")
			},
		},
		{
			id: "toolwindow.debug",
			open: func(m *Model) bool {
				return m.dockWindowShown("debug")
			},
		},
		{
			id: "toolwindow.git",
			open: func(m *Model) bool {
				return m.sideMode == "vcs" && !m.leftHidden
			},
		},
		{
			id: "toolwindow.terminal",
			open: func(m *Model) bool {
				return m.dockWindowShown("term")
			},
		},
		{
			id: "toolwindow.commit",
			open: func(m *Model) bool {
				return m.commit != nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			m := toolModel()
			m.setRoot(t.TempDir())
			m.sideMode = "vcs"
			m.leftHidden = true

			if !m.RunAction(tt.id) {
				t.Fatalf("%s was not available", tt.id)
			}
			if !tt.open(m) {
				t.Fatalf("%s did not open its native window", tt.id)
			}
		})
	}
}
