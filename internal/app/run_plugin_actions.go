package app

import (
	"encoding/json"
	"fmt"

	"mayoi/internal/openidea"
)

func init() {
	hiddenActionIDs["run.pluginStart"] = true
	_ = registerAction(&Action{
		ID:       "run.pluginStart",
		Title:    "Run Plugin Configuration",
		Category: "Run",
		When:     func(m *Model) bool { return m.Root() != "" },
		RunWithArg: func(m *Model, arg any) {
			m.RunPluginStart(arg)
		},
	})
}

type pluginStartRequest struct {
	raw    json.RawMessage
	debug  bool
	onExit func(code *int32, stopped bool)
}

// RunPluginStart is the hidden run.pluginStart bridge used by plugin hosts.
// It delegates debug launches to debug.pluginStart and otherwise passes the
// exact plugin configuration to run_start, settling onExit exactly once.
func (m *Model) RunPluginStart(arg any) bool {
	request, callback, err := decodePluginStartRequest(arg)
	if err != nil {
		if callback != nil {
			callback(nil, false)
		}
		m.status = "Cannot start the plugin configuration: " + err.Error()
		m.invalidate()
		return false
	}
	if m.Root() == "" {
		if callback != nil {
			callback(nil, false)
		}
		return false
	}
	if request.debug {
		m.DebugPluginStart(map[string]any{
			"config": request.raw,
			"onExit": request.onExit,
		})
		return true
	}
	config, err := openidea.DecodeRunConfig(request.raw)
	if err != nil {
		m.finishPluginCallback(callback, nil, false)
		m.status = "Cannot read the plugin run configuration: " + err.Error()
		m.invalidate()
		return false
	}
	if m.ctx == nil {
		m.finishPluginCallback(callback, nil, false)
		m.status = "The run backend is not connected"
		m.invalidate()
		return false
	}
	m.beginRunPlugin(config.ID, callback)
	if !m.startRunWith(request.raw, config.Name) {
		m.failRunPlugin()
		return false
	}
	return true
}

func (m *Model) finishPluginCallback(
	callback func(code *int32, stopped bool),
	code *int32,
	stopped bool,
) {
	if callback != nil {
		callback(code, stopped)
	}
}

func decodePluginStartRequest(arg any) (pluginStartRequest, func(*int32, bool), error) {
	var request pluginStartRequest
	var callback func(*int32, bool)
	object, ok := arg.(map[string]any)
	if !ok {
		if raw, rawOK := arg.(json.RawMessage); rawOK {
			if err := json.Unmarshal(raw, &object); err != nil {
				return request, nil, err
			}
			ok = object != nil
		} else if raw, rawOK := arg.([]byte); rawOK {
			if err := json.Unmarshal(raw, &object); err != nil {
				return request, nil, err
			}
			ok = object != nil
		}
	}
	if !ok {
		var decode map[string]json.RawMessage
		raw, err := json.Marshal(arg)
		if err != nil {
			return request, nil, err
		}
		if err := json.Unmarshal(raw, &decode); err != nil {
			return request, nil, err
		}
		object = make(map[string]any, len(decode))
		for key, value := range decode {
			object[key] = value
		}
	}
	if fn, ok := object["onExit"].(func(*int32, bool)); ok {
		callback = fn
	}
	config, hasConfig := object["config"]
	if !hasConfig {
		return request, callback, fmt.Errorf("the plugin run configuration is missing")
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return request, callback, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return request, callback, fmt.Errorf("the plugin run configuration is missing")
	}
	request.raw = raw
	request.onExit = callback
	request.debug, _ = object["debug"].(bool)
	return request, callback, nil
}
