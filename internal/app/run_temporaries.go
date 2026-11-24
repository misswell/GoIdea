package app

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

const maxRunTemporaries = 5

func (m *Model) runTemporaryByID(id string) *openidea.RunConfig {
	if m == nil || id == "" {
		return nil
	}
	for _, cfg := range m.run.temporaries {
		if cfg != nil && cfg.ID == id {
			return cfg
		}
	}
	if m.run.inline != nil && m.run.inline.ID == id {
		return m.run.inline
	}
	return nil
}

func (m *Model) isRunTemporaryID(id string) bool {
	return m.runTemporaryByID(id) != nil ||
		m.run.pendingInline != nil && m.run.pendingInline.ID == id
}

func (m *Model) runTemporaryIDs() map[string]bool {
	ids := make(map[string]bool, len(m.run.temporaries)+1)
	for _, cfg := range m.run.temporaries {
		if cfg != nil {
			ids[cfg.ID] = true
		}
	}
	if m.run.inline != nil {
		ids[m.run.inline.ID] = true
	}
	if m.run.pendingInline != nil {
		ids[m.run.pendingInline.ID] = true
	}
	return ids
}

func runConfigValues(configs []*openidea.RunConfig) []openidea.RunConfig {
	out := make([]openidea.RunConfig, 0, len(configs))
	for _, cfg := range configs {
		if clone := cloneRunConfig(cfg); clone != nil {
			out = append(out, *clone)
		}
	}
	return out
}

func removeRunConfigValueID(configs []openidea.RunConfig, id string) []openidea.RunConfig {
	out := configs[:0]
	for _, cfg := range configs {
		if cfg.ID != id {
			out = append(out, cfg)
		}
	}
	return out
}

func (m *Model) persistRunSelection() {
	if m == nil || m.Root() == "" {
		return
	}
	project := m.runProjectPrefsState()
	project.Selected = m.run.selectedID()
	m.savePreferences()
}

func (m *Model) persistRunTemporaries() {
	if m == nil {
		return
	}
	root := m.Root()
	if root == "" {
		return
	}
	m.run.temporaryPersistGeneration++
	generation := m.run.temporaryPersistGeneration
	configs := runConfigValues(m.run.temporaries)
	project := m.runProjectPrefsState()
	project.Temporary = configs
	project.Selected = m.run.selectedID()
	m.savePreferences()
	if len(configs) == 0 || m.invokeHook == nil && m.mgr == nil {
		return
	}

	go func() {
		ctx := m.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "run_without_derived", map[string]any{
			"configs": runConfigPointers(configs),
		})
		if err != nil {
			// The full snapshot written above remains usable if an older
			// server cannot strip derived toolchain values.
			return
		}
		kept, err := openidea.DecodeRunConfigs(raw)
		if err != nil {
			return
		}
		m.post(func() {
			if generation != m.run.temporaryPersistGeneration || m.Root() != root {
				return
			}
			project := m.runProjectPrefsState()
			project.Temporary = kept
			m.savePreferences()
		})
	}()
}

func (m *Model) storedRunTemporaries() ([]openidea.RunConfig, string) {
	if m == nil || m.Root() == "" {
		return nil, ""
	}
	project := m.runProjectPrefsState()
	configs := make([]openidea.RunConfig, 0, len(project.Temporary))
	for i := range project.Temporary {
		if clone := cloneRunConfig(&project.Temporary[i]); clone != nil {
			configs = append(configs, *clone)
		}
	}
	return configs, project.Selected
}

func (m *Model) replaceRunTemporaries(configs []openidea.RunConfig, selectedID string) {
	m.run.temporaries = nil
	for i := range configs {
		if clone := cloneRunConfig(&configs[i]); clone != nil {
			m.run.temporaries = append(m.run.temporaries, clone)
		}
	}
	m.run.inline = nil
	m.run.sel = -1
	m.run.currentFile = false
	if selectedID == "current-file" {
		m.run.currentFile = true
	} else if cfg := m.runTemporaryByID(selectedID); cfg != nil {
		m.run.inline = cfg
	}
	m.persistRunTemporaries()
}

func sameRunApplicationConfig(a, b *openidea.RunConfig) bool {
	if a == nil || b == nil || !isJVMApplicationKind(a.Kind) || !isJVMApplicationKind(b.Kind) {
		return false
	}
	ma, mb := adapter.ParseJavaArgs(a.Args).MainClass, adapter.ParseJavaArgs(b.Args).MainClass
	if ma == "" || ma != mb {
		return false
	}
	return runApplicationModulePath(a) == runApplicationModulePath(b)
}

func isJVMApplicationKind(kind string) bool {
	return kind == "application" || kind == "spring-boot"
}

func runApplicationModulePath(cfg *openidea.RunConfig) string {
	if cfg == nil {
		return ""
	}
	if cfg.Module != nil {
		return *cfg.Module
	}
	if cfg.Cwd != nil {
		return *cfg.Cwd
	}
	return ""
}

func (m *Model) runTemporaryBusy(id string) bool {
	if m == nil || id == "" {
		return false
	}
	if m.chooserConfigSession(id).running {
		return true
	}
	if m.debug.starting && m.debug.lastID == id {
		return true
	}
	for _, child := range m.run.children {
		if child != nil && !child.finished && child.groupID == id {
			return true
		}
	}
	return false
}

func (m *Model) adoptRunTemporary(cfg openidea.RunConfig) *openidea.RunConfig {
	for i := range m.run.configs {
		if m.run.configs[i].ID == cfg.ID {
			m.run.pendingInline = nil
			m.selectRunConfig(cfg.ID)
			return &m.run.configs[i]
		}
	}
	for i := range m.run.configs {
		if sameRunApplicationConfig(&m.run.configs[i], &cfg) {
			m.run.pendingInline = nil
			m.selectRunConfig(m.run.configs[i].ID)
			return &m.run.configs[i]
		}
	}

	cfg.Detected = false
	next := make([]*openidea.RunConfig, 0, len(m.run.temporaries)+1)
	for _, current := range m.run.temporaries {
		if current != nil && current.ID != cfg.ID {
			next = append(next, current)
		}
	}
	clone := cloneRunConfig(&cfg)
	if clone == nil {
		return nil
	}
	next = append(next, clone)
	for len(next) > maxRunTemporaries {
		evict := 0
		for i, current := range next {
			if current.ID != cfg.ID && !m.runTemporaryBusy(current.ID) {
				evict = i
				break
			}
		}
		next = append(next[:evict], next[evict+1:]...)
	}
	m.run.temporaries = next
	m.run.inline = clone
	m.run.pendingInline = clone
	m.run.currentFile = false
	m.persistRunTemporaries()
	return clone
}

func (m *Model) removeRunTemporary(id string) {
	if m == nil || id == "" {
		return
	}
	next := m.run.temporaries[:0]
	for _, cfg := range m.run.temporaries {
		if cfg != nil && cfg.ID != id {
			next = append(next, cfg)
		}
	}
	m.run.temporaries = next
	droppedSelected := false
	if m.run.inline != nil && m.run.inline.ID == id {
		droppedSelected = true
		m.run.inline = nil
	}
	if m.run.pendingInline != nil && m.run.pendingInline.ID == id {
		m.run.pendingInline = nil
	}
	if droppedSelected {
		m.run.sel = -1
		m.run.currentFile = false
	}
	m.persistRunTemporaries()
}

func (m *Model) setRunTemporarySelection(cfg *openidea.RunConfig) {
	m.run.inline = cfg
	m.run.currentFile = false
	m.persistRunSelection()
}

func (m *Model) dropStaleRunTemporaries(removed []string) {
	if m == nil || len(removed) == 0 || len(m.run.temporaries) == 0 {
		return
	}
	gone := map[string]bool{}
	for _, file := range removed {
		rel := relPathUnder(m.Root(), file)
		gone[filepath.ToSlash(rel)] = true
	}
	stale := func(cfg *openidea.RunConfig) bool {
		if cfg == nil {
			return false
		}
		var rest string
		switch {
		case strings.HasPrefix(cfg.ID, "context:file:"):
			rest = strings.TrimPrefix(cfg.ID, "context:file:")
		case strings.HasPrefix(cfg.ID, "context:build:"):
			rest = strings.TrimPrefix(cfg.ID, "context:build:")
		default:
			return false
		}
		if at := strings.LastIndex(rest, "#"); at >= 0 {
			rest = rest[:at]
		}
		return gone[filepath.ToSlash(rest)]
	}
	next := m.run.temporaries[:0]
	droppedSelected := false
	for _, cfg := range m.run.temporaries {
		if stale(cfg) {
			if m.run.inline != nil && m.run.inline.ID == cfg.ID {
				droppedSelected = true
			}
			continue
		}
		next = append(next, cfg)
	}
	if len(next) == len(m.run.temporaries) {
		if stale(m.run.pendingInline) {
			m.run.pendingInline = nil
			m.persistRunTemporaries()
		}
		return
	}
	m.run.temporaries = next
	if droppedSelected {
		m.run.inline = nil
		m.run.sel = -1
		m.run.currentFile = false
	}
	if stale(m.run.pendingInline) {
		m.run.pendingInline = nil
	}
	m.persistRunTemporaries()
}
