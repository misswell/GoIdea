package app

import (
	"context"
	"fmt"
	"path/filepath"
)

func (m *Model) applyActionsOnSave(ctx context.Context, tab *Tab) error {
	optimize := m.nativeRuntimeBool(
		settingsPageActionsOnSave,
		"actionsOnSave.optimizeImports",
	)
	cleanup := m.nativeRuntimeBool(
		settingsPageActionsOnSave,
		"actionsOnSave.cleanup",
	)
	if !optimize && !cleanup {
		return nil
	}
	if tab == nil || tab.doc == nil || tab.state == nil || tab.state.ReadOnly {
		return nil
	}
	if tab.formatting {
		return fmt.Errorf(
			"Cannot save %s while another refactoring is running",
			filepath.Base(tab.doc.Path()),
		)
	}
	tab.formatting = true
	defer func() { tab.formatting = false }()
	if optimize {
		m.status = "Optimizing imports in " + filepath.Base(tab.doc.Path()) + "…"
		if err := m.applyAnalysisSourceAction(ctx, tab, "source.organizeImports"); err != nil {
			return fmt.Errorf("Optimize imports failed: %w", err)
		}
	}
	if cleanup {
		m.status = "Cleaning " + filepath.Base(tab.doc.Path()) + "…"
		if err := m.applyAnalysisSourceAction(ctx, tab, "source.fixAll"); err != nil {
			return fmt.Errorf("Code cleanup failed: %w", err)
		}
	}
	return nil
}
