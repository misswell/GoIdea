package app

import (
	"reflect"
	"testing"
)

func TestNewProjectsSettingsForceEveryPageToGlobalScope(t *testing.T) {
	m := &Model{
		root: "/project",
		settingsDraft: settingsDraft{
			newProjects: true,
		},
	}
	def, ok := nativeSettingsDefinition(settingsPageCodeStyle)
	if !ok {
		t.Fatal("Code Style settings page is missing")
	}
	if scopes := nativeSettingsPageScopes(m, def); !reflect.DeepEqual(scopes, []string{"global"}) {
		t.Fatalf("new-project scopes = %v, want only global", scopes)
	}

	key := def.sections[0].fields[0].key
	m.nativeSettings.pages = map[string]nativeSettingsPageState{
		def.page.id: {
			loaded: true,
			applied: map[string]any{
				key: false,
			},
			draft: map[string]any{
				key: true,
			},
		},
	}
	writes := m.pendingNativeSettingsWrites()
	if len(writes) != 1 || writes[0].key != key || writes[0].scope != "global" {
		t.Fatalf("new-project writes = %+v", writes)
	}
}
