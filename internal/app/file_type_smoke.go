package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SmokeFileTypeOverride proves an exact-path override reaches the pinned
// backend and immediately changes the active editor. It restores the project's
// original override before returning.
func (m *Model) SmokeFileTypeOverride(ctx context.Context) (map[string]any, error) {
	var path string
	var originalOverride string
	m.post(func() {
		if tab := m.tab(); tab != nil && tab.doc != nil {
			path = tab.doc.Path()
			originalOverride = m.currentFileTypeOverride(path)
		}
	})
	if path == "" {
		return nil, fmt.Errorf("file type smoke needs an open file")
	}
	languages, err := m.toolLanguages(ctx)
	if err != nil {
		return nil, fmt.Errorf("read backend languages: %w", err)
	}

	var target openIDEALanguageInfo
	var targetLanguageName string
	for _, preferred := range []string{"markdown", "plaintext", "text"} {
		for _, language := range languages {
			if language.ID == preferred && language.ID != originalOverride {
				if target = language; mygoLanguageForID(language.ID) != nil {
					targetLanguageName = mygoLanguageForID(language.ID).Name
					break
				}
			}
		}
		if target.ID != "" {
			break
		}
	}
	if target.ID == "" {
		for _, language := range languages {
			if language.ID == originalOverride {
				continue
			}
			if resolved := mygoLanguageForID(language.ID); resolved != nil {
				target = language
				targetLanguageName = resolved.Name
				break
			}
		}
	}
	if target.ID == "" {
		return nil, fmt.Errorf("the backend has no language that MyGo can render")
	}

	key := fileTypeOverrideKey(path, m.Root())
	m.SetFileTypeOverride(path, target.ID)
	if err := waitFileTypeSmoke(ctx, func() bool {
		var current, language string
		m.post(func() {
			current = m.currentFileTypeOverride(path)
			if tab := m.tab(); tab != nil && tab.state != nil && tab.state.Buffer != nil {
				if got := tab.state.Buffer.Language(); got != nil {
					language = got.Name
				}
			}
		})
		return current == target.ID && language == targetLanguageName
	}); err != nil {
		return nil, fmt.Errorf("apply %s override: %w", target.ID, err)
	}

	overrides, err := m.readFileTypeOverrides(ctx, "project")
	if err != nil {
		return nil, fmt.Errorf("read saved override: %w", err)
	}
	if overrides[key] != target.ID {
		return nil, fmt.Errorf("saved override = %q, want %q", overrides[key], target.ID)
	}

	m.SetFileTypeOverride(path, originalOverride)
	if err := waitFileTypeSmoke(ctx, func() bool {
		var current string
		m.post(func() { current = m.currentFileTypeOverride(path) })
		return current == originalOverride
	}); err != nil {
		return nil, fmt.Errorf("restore override: %w", err)
	}
	overrides, err = m.readFileTypeOverrides(ctx, "project")
	if err != nil {
		return nil, fmt.Errorf("read restored override: %w", err)
	}

	out := map[string]any{
		"path":         projectRelative(m.Root(), path),
		"languages":    len(languages),
		"override":     target.ID,
		"language":     targetLanguageName,
		"persisted":    true,
		"restored":     overrides[key] == originalOverride,
		"original":     originalOverride,
		"menuLanguage": len(m.fileTypeMenuLanguages()),
	}
	filters, err := m.smokeProjectTreeFilters(ctx)
	if err != nil {
		return out, err
	}
	for key, value := range filters {
		out[key] = value
	}
	return out, nil
}

func (m *Model) smokeProjectTreeFilters(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" || m.tree == nil {
		return nil, fmt.Errorf("File Types smoke needs a Project View")
	}
	raw, err := m.invokeSettingsScope(ctx, "project")
	if err != nil {
		return nil, fmt.Errorf("read File Types filters: %w", err)
	}
	original, err := decodeFileTypesSettings(raw)
	if err != nil {
		return nil, fmt.Errorf("decode File Types filters: %w", err)
	}
	const (
		ignoredPattern = "*.goidea-ignored-smoke"
		excludePattern = "**/goidea-excluded-smoke"
	)
	ignoredPath := filepath.Join(root, "filter.goidea-ignored-smoke")
	excludedDir := filepath.Join(root, "goidea-excluded-smoke")
	excludedPath := filepath.Join(excludedDir, "data.txt")
	if err := os.MkdirAll(excludedDir, 0o755); err != nil {
		return nil, fmt.Errorf("create exclude fixture: %w", err)
	}
	if err := os.WriteFile(ignoredPath, []byte("ignored\n"), 0o644); err != nil {
		return nil, fmt.Errorf("create ignored fixture: %w", err)
	}
	if err := os.WriteFile(excludedPath, []byte("excluded\n"), 0o644); err != nil {
		return nil, fmt.Errorf("create excluded fixture: %w", err)
	}
	defer os.RemoveAll(excludedDir)
	defer os.Remove(ignoredPath)

	originalOptions := m.tree.options
	defer m.post(func() {
		if m.tree != nil {
			m.tree.SetOptions(originalOptions)
			m.tree.RefreshDir(root)
		}
	})
	m.smokeSetTreeFilterVisibility(true, true)
	if err := waitFileTypeSmoke(ctx, func() bool {
		return m.smokeTreeHasRow(ignoredPath) && m.smokeTreeHasRow(excludedDir)
	}); err != nil {
		return nil, fmt.Errorf("show filter fixtures: %w", err)
	}

	ignoredNames := append(cloneStrings(original.IgnoredNames), ignoredPattern)
	exclude := append(cloneStrings(original.Exclude), excludePattern)
	restored := false
	restore := func(cleanupCtx context.Context) error {
		if err := m.invokeSettingsSet(cleanupCtx, "project", "files.ignoredNames", original.IgnoredNames); err != nil {
			return err
		}
		if err := m.invokeSettingsSet(cleanupCtx, "project", "files.exclude", original.Exclude); err != nil {
			return err
		}
		m.refreshFileTypeRuntime(cleanupCtx)
		restored = true
		return nil
	}
	defer func() {
		if restored {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = restore(cleanupCtx)
	}()
	if err := m.invokeSettingsSet(ctx, "project", "files.ignoredNames", ignoredNames); err != nil {
		return nil, fmt.Errorf("save ignored names: %w", err)
	}
	if err := m.invokeSettingsSet(ctx, "project", "files.exclude", exclude); err != nil {
		return nil, fmt.Errorf("save exclude patterns: %w", err)
	}
	m.refreshFileTypeRuntime(ctx)
	m.smokeSetTreeFilterVisibility(false, true)
	if err := waitFileTypeSmoke(ctx, func() bool {
		return !m.smokeTreeHasRow(ignoredPath) &&
			m.smokeTreeHasRow(excludedDir) &&
			m.smokeTreeRowExcluded(excludedDir)
	}); err != nil {
		return nil, fmt.Errorf("apply ignored names and dim exclusions: %w", err)
	}
	m.smokeSetTreeFilterVisibility(false, false)
	if err := waitFileTypeSmoke(ctx, func() bool { return !m.smokeTreeHasRow(excludedDir) }); err != nil {
		return nil, fmt.Errorf("hide exclude patterns: %w", err)
	}
	if err := restore(ctx); err != nil {
		return nil, fmt.Errorf("restore File Types filters: %w", err)
	}
	restoredRaw, err := m.invokeSettingsScope(ctx, "project")
	if err != nil {
		return nil, fmt.Errorf("read restored File Types filters: %w", err)
	}
	restoredSettings, err := decodeFileTypesSettings(restoredRaw)
	if err != nil {
		return nil, fmt.Errorf("decode restored File Types filters: %w", err)
	}
	if !sameStrings(restoredSettings.IgnoredNames, original.IgnoredNames) ||
		!sameStrings(restoredSettings.Exclude, original.Exclude) {
		return nil, fmt.Errorf("File Types filters were not restored")
	}
	m.post(func() {
		if m.tree != nil {
			m.tree.SetOptions(originalOptions)
			m.tree.RefreshDir(root)
		}
	})
	if err := waitFileTypeSmoke(ctx, func() bool {
		return m.smokeTreeHasRow(ignoredPath) && m.smokeTreeHasRow(excludedDir)
	}); err != nil {
		return nil, fmt.Errorf("show restored filter fixtures: %w", err)
	}
	return map[string]any{
		"ignoredHidden":   true,
		"excludedDimmed":  true,
		"excludedHidden":  true,
		"filtersRestored": true,
	}, nil
}

func (m *Model) smokeSetTreeFilterVisibility(showHidden, showExcluded bool) {
	m.post(func() {
		if m.tree == nil {
			return
		}
		options := m.tree.options
		options.showHidden = showHidden
		options.showExcluded = showExcluded
		m.tree.SetOptions(options)
		m.tree.RefreshDir(m.Root())
	})
}

func (m *Model) smokeTreeHasRow(value string) bool {
	value = realPath(value)
	found := false
	m.post(func() {
		if m.tree == nil {
			return
		}
		for _, row := range m.tree.Rows() {
			if realPath(row.path) == value {
				found = true
				return
			}
		}
	})
	return found
}

func (m *Model) smokeTreeRowExcluded(value string) bool {
	value = realPath(value)
	found := false
	m.post(func() {
		if m.tree == nil {
			return
		}
		for _, row := range m.tree.Rows() {
			if realPath(row.path) == value {
				found = row.excluded
				return
			}
		}
	})
	return found
}

func waitFileTypeSmoke(ctx context.Context, done func() bool) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if done() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
