package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

// SmokeExternalLibraries trusts the project, runs the real SDK/dependency
// discovery, expands one archive through fs_read_dir and reports the native
// Project View state.
func (m *Model) SmokeExternalLibraries(ctx context.Context) (map[string]any, error) {
	root := m.Root()
	if root == "" {
		return nil, errors.New("No project is open")
	}
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return nil, err
	}
	m.post(func() {
		m.run.trusted = true
		m.RefreshExternalLibraries()
	})

	var groups []openidea.LibraryGroup
	var loadErr string
	for {
		m.post(func() {
			if m.externalLibraries != nil {
				groups = append([]openidea.LibraryGroup(nil), m.externalLibraries.groups...)
				loadErr = m.externalLibraries.err
			}
		})
		if len(groups) > 0 || loadErr != "" {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if loadErr != "" {
		return nil, errors.New(loadErr)
	}
	if len(groups) == 0 {
		return nil, errors.New("External Libraries returned no SDK or dependency groups")
	}

	var kinds []string
	var roots []string
	var archiveRoot string
	for _, group := range groups {
		kinds = append(kinds, group.Kind)
		for _, libraryRoot := range group.Roots {
			roots = append(roots, libraryRoot.Path)
			if archiveRoot == "" && strings.HasSuffix(libraryRoot.Path, "!") {
				archiveRoot = libraryRoot.Path
			}
		}
	}

	var expanded bool
	var archiveEntries int
	var rowNames []string
	var allReadOnly = true
	m.post(func() {
		if m.tree == nil {
			return
		}
		m.tree.Toggle(externalLibrariesRootID)
		for _, group := range groups {
			m.tree.Toggle(externalLibraryGroupID(group))
		}
		if archiveRoot != "" {
			m.tree.Toggle(archiveRoot)
		}
	})
	if archiveRoot != "" {
		for {
			var loading bool
			m.post(func() {
				if m.tree != nil {
					loading = m.tree.externalLoading[archiveRoot]
					archiveEntries = len(m.tree.externalEntries[archiveRoot])
				}
			})
			if !loading {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	m.post(func() {
		if m.tree == nil {
			return
		}
		for _, row := range m.tree.Rows() {
			if !row.member {
				rowNames = append(rowNames, row.name)
			}
			if row.external && !row.readOnly {
				allReadOnly = false
			}
		}
		expanded = m.tree.externalRoot != nil && m.tree.open[externalLibrariesRootID]
	})
	if archiveRoot != "" && archiveEntries == 0 {
		return nil, fmt.Errorf("the archive root %s returned no entries", archiveRoot)
	}
	return map[string]any{
		"groups":         len(groups),
		"kinds":          kinds,
		"roots":          roots,
		"archiveRoot":    archiveRoot,
		"archiveEntries": archiveEntries,
		"expanded":       expanded,
		"rows":           rowNames,
		"readOnly":       allReadOnly,
		"action":         m.ActionEnabled("libraries.reload"),
	}, nil
}
