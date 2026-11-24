package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// SmokeProjectStructure opens Project Structure over the pinned backend,
// verifies every section, saves one setting and restores the original value.
func (m *Model) SmokeProjectStructure(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("No project is open")
	}
	if m.mgr == nil {
		return nil, errors.New("The GoIDEA backend is not connected")
	}
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return nil, err
	}
	m.post(func() { m.run.trusted = true })
	m.post(func() { m.OpenProjectStructure(projectStructureProject) })

	if err := m.waitForProjectStructure(ctx); err != nil {
		return nil, err
	}

	var sections []string
	var modules int
	var sdkKinds []string
	var libraryGroups int
	var menuAvailable bool
	m.post(func() {
		state := &m.projectStructure
		for _, item := range projectStructureSections {
			sections = append(sections, item.ID)
		}
		modules = len(m.projectStructureModules())
		for _, item := range projectStructureSDKKinds {
			if len(state.image.SDKs[item.Kind]) != 0 {
				sdkKinds = append(sdkKinds, item.Kind)
			}
		}
		libraryGroups = len(state.image.Libraries)
		menuAvailable = m.ActionEnabled("file.projectStructure")
	})
	if modules == 0 {
		return nil, errors.New("Project Structure did not build its module list")
	}
	if !slicesContain(sdkKinds, "jdk") {
		return nil, errors.New("Project Structure did not detect the JDK")
	}
	if !menuAvailable {
		return nil, errors.New("Project Structure action is not registered")
	}

	for _, section := range sections {
		m.post(func() { m.SetProjectStructureSection(section) })
	}

	original, err := m.projectStructureCompilerOutput(ctx)
	if err != nil {
		return nil, err
	}
	testOutput := ".goidea-project-structure-smoke"
	if original == testOutput {
		testOutput = original
	}

	m.post(func() { m.SetProjectStructureField("compilerOutput", testOutput) })
	m.post(func() { m.ProjectStructureApply() })
	if err := m.waitForProjectStructureSave(ctx); err != nil {
		return nil, err
	}

	saved, err := m.projectStructureCompilerOutput(ctx)
	if err != nil {
		return nil, err
	}
	if saved != testOutput {
		return nil, errors.New("Project Structure did not persist compiler output")
	}

	m.post(func() { m.SetProjectStructureField("compilerOutput", original) })
	m.post(func() { m.ProjectStructureApply() })
	if err := m.waitForProjectStructureSave(ctx); err != nil {
		return nil, err
	}

	var open bool
	var structureErr string
	m.post(func() {
		open = m.projectStructure.open
		structureErr = m.projectStructure.err
	})
	return map[string]any{
		"open":          open,
		"sections":      sections,
		"modules":       modules,
		"sdkKinds":      sdkKinds,
		"libraryGroups": libraryGroups,
		"menu":          menuAvailable,
		"saved":         saved,
		"restored":      original,
		"error":         structureErr,
	}, nil
}

func (m *Model) waitForProjectStructure(ctx context.Context) error {
	for {
		var ready bool
		m.post(func() {
			state := &m.projectStructure
			ready = state.open && !state.loading && state.loaded
		})
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m *Model) waitForProjectStructureSave(ctx context.Context) error {
	for {
		var saving bool
		var saveErr string
		m.post(func() {
			state := &m.projectStructure
			saving = state.saving
			saveErr = state.err
		})
		if saveErr != "" {
			return errors.New(saveErr)
		}
		if !saving {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m *Model) projectStructureCompilerOutput(ctx context.Context) (string, error) {
	raw, err := m.invoke(ctx, "settings_get", nil)
	if err != nil {
		return "", err
	}
	var envelope struct {
		ProjectStructure struct {
			CompilerOutput string `json:"compilerOutput"`
		} `json:"projectStructure"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", err
	}
	return envelope.ProjectStructure.CompilerOutput, nil
}
