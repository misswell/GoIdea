package app

import (
	"context"
	"encoding/json"
	"time"
)

const (
	nativeToAsciiActionID  = "editor.encodings.toggleNativeToAscii"
	nativeToAsciiSettingID = "editor.encodings.nativeToAscii"
)

func init() {
	action, ok := actionGet(nativeToAsciiActionID)
	if !ok {
		action = &Action{ID: nativeToAsciiActionID}
	}
	action.Title = "Transparent native-to-ascii conversion"
	action.Category = "Editor"
	action.Keywords = "properties encoding Unicode"
	action.When = nil
	action.KeyWhen = nil
	action.Run = func(m *Model) {
		m.toggleNativeToAscii(nil)
	}
	action.RunWithArg = func(m *Model, arg any) {
		m.toggleNativeToAscii(arg)
	}
	action.Toggled = func(m *Model) bool {
		state := m.nativeSettings.pages[settingsPageFileEncodings]
		return state.loaded && nativeSettingsBool(
			m,
			settingsPageFileEncodings,
			nativeToAsciiSettingID,
		)
	}
	if !ok {
		_ = registerAction(action)
	}
}

// ToggleNativeToASCII keeps the public action entry point used by existing
// callers while Settings staging goes through toggleNativeToAscii.
func (m *Model) ToggleNativeToASCII() {
	if m != nil {
		m.toggleNativeToAscii(nil)
	}
}

func (m *Model) toggleNativeToAscii(arg any) {
	if m == nil {
		return
	}
	if stageNativeToAsciiValue(m, arg) {
		return
	}

	scope := "global"
	root := m.Root()
	if root != "" {
		scope = "project"
	}
	if m.mgr == nil && m.invokeHook == nil {
		m.status = "The GoIDEA backend is not connected"
		m.invalidate()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		raw, err := m.invokeSettingsScope(ctx, scope)
		current := false
		if err == nil {
			var values map[string]any
			if json.Unmarshal(raw, &values) == nil {
				current, _ = nativeNestedSetting(
					values, "editor", "encodings", "nativeToAscii",
				).(bool)
			}
		}
		desired := !current
		if err == nil {
			err = m.invokeSettingsSet(ctx, scope, nativeToAsciiSettingID, desired)
		}
		m.post(func() {
			if scope == "project" && m.Root() != root {
				return
			}
			if err != nil {
				m.status = "Could not update native-to-ascii conversion: " + err.Error()
				m.invalidate()
				return
			}
			state := m.nativeSettings.pages[settingsPageFileEncodings]
			if state.loaded && state.scope == scope {
				if state.applied == nil {
					state.applied = map[string]any{}
				}
				if state.draft == nil {
					state.draft = map[string]any{}
				}
				state.applied[nativeToAsciiSettingID] = desired
				state.draft[nativeToAsciiSettingID] = desired
				m.nativeSettings.pages[settingsPageFileEncodings] = state
			}
			if desired {
				m.status = "Transparent native-to-ascii conversion on"
			} else {
				m.status = "Transparent native-to-ascii conversion off"
			}
			m.invalidate()
		})
	}()
}

func stageNativeToAsciiValue(m *Model, arg any) bool {
	staged, ok := arg.(map[string]any)
	if !ok {
		return false
	}
	pageID, _ := staged["pageId"].(string)
	scope, _ := staged["scope"].(string)
	value, valueOK := staged["value"].(bool)
	if pageID != settingsPageFileEncodings || scope == "" || !valueOK {
		return false
	}
	m.setNativeSettingsValue(settingsPageFileEncodings, nativeToAsciiSettingID, value)
	return true
}
