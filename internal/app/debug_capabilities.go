package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

func (m *Model) debugSessionSupports(id uint32, capability func(openidea.DebugCapabilities) bool) bool {
	state := m.debugSessionState(id)
	return state != nil && state.capsKnown && capability(state.caps)
}

func (m *Model) refreshDebugCapabilities(id uint32) {
	var request uint64
	if !m.updateDebugSession(id, func(state *debugSessionState) {
		state.capsRequest++
		request = state.capsRequest
	}) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "debug_capabilities", map[string]any{"session": id})
		var caps openidea.DebugCapabilities
		if err == nil {
			err = json.Unmarshal(raw, &caps)
		}
		m.post(func() {
			m.updateDebugSession(id, func(state *debugSessionState) {
				if state.capsRequest != request || state.session == 0 || state.ended {
					return
				}
				if err != nil {
					state.err = "Could not read the debugger capabilities: " + err.Error()
					return
				}
				state.caps = caps
				state.capsKnown = true
				if !caps.JVMInstances {
					state.memory.invalidateInspection()
				}
			})
			m.invalidate()
		})
	}()
}

func (m *Model) acceptDebugCapabilities(id uint32) {
	m.refreshDebugCapabilities(id)
}

func (m *Model) acceptJVMBackend(id uint32, backend openidea.JvmBackendState) {
	backend.Kind = strings.ToLower(strings.TrimSpace(backend.Kind))
	m.updateDebugSession(id, func(state *debugSessionState) {
		state.jvmBackend = backend
		switch backend.Kind {
		case "degraded", "closed":
			state.memory.invalidateInspection()
		}
		if state.session != 0 {
			state.status = debugJVMBackendStatus(backend)
		}
	})
	m.invalidate()
}

func debugJVMBackendStatus(state openidea.JvmBackendState) string {
	switch state.Kind {
	case "ready":
		return "JVM object inspection is ready"
	case "unsupported":
		if state.Detail != "" {
			return "JVM object inspection is unavailable: " + state.Detail
		}
		return "JVM object inspection is unavailable for this session"
	case "degraded":
		if state.Detail != "" {
			return "JVM object inspection is limited: " + state.Detail
		}
		return "JVM object inspection is temporarily unavailable"
	case "closed":
		return "JVM object inspection closed"
	default:
		return ""
	}
}

func (m *Model) decodeDebugSessionInfo(raw json.RawMessage) ([]openidea.DebugSessionInfo, error) {
	var sessions []openidea.DebugSessionInfo
	err := json.Unmarshal(raw, &sessions)
	return sessions, err
}
