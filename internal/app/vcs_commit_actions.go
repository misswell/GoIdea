package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

type vcsCommitModifications struct {
	optimizeImports bool
	cleanup         bool
	reformat        bool
}

func (m *Model) vcsCommitModifications() vcsCommitModifications {
	return vcsCommitModifications{
		optimizeImports: m.nativeRuntimeBool(
			settingsPageVCSCommit,
			"vcs.commit.optimizeImports",
		),
		cleanup: m.nativeRuntimeBool(
			settingsPageVCSCommit,
			"vcs.commit.cleanup",
		),
		reformat: m.nativeRuntimeBool(
			settingsPageVCSCommit,
			"vcs.commit.reformat",
		),
	}
}

func (m vcsCommitModifications) enabled() bool {
	return m.optimizeImports || m.cleanup || m.reformat
}

func commitFilesForPlan(files []openidea.FileStatus, plan commitPlan) []openidea.FileStatus {
	included := make(map[string]bool, len(plan.included))
	for _, path := range plan.included {
		included[filepath.Clean(path)] = true
	}
	out := make([]openidea.FileStatus, 0, len(plan.included))
	for _, file := range files {
		if included[filepath.Clean(file.Path)] {
			out = append(out, file)
		}
	}
	return out
}

func commitExistingPaths(files []openidea.FileStatus) []string {
	seen := make(map[string]bool, len(files))
	out := make([]string, 0, len(files))
	for _, file := range files {
		if effectiveVCSKind(file) == "deleted" {
			continue
		}
		path := filepath.Clean(file.Path)
		if path == "" || seen[path] {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

// modifyCommitFiles applies OpenIDEA's before-commit modifications in its
// order: optimize imports, cleanup, then reformat. Opening a changed file that
// has no editor tab is temporary, and a language server that cannot handle the
// file only skips that modification instead of blocking the commit.
func (m *Model) modifyCommitFiles(
	ctx context.Context,
	tabs []*Tab,
	paths []string,
	modifications vcsCommitModifications,
) {
	if !modifications.enabled() || len(paths) == 0 {
		return
	}
	openTabs := make(map[string]*Tab, len(tabs))
	for _, tab := range tabs {
		if tab == nil || tab.doc == nil || tab.kind != "" {
			continue
		}
		openTabs[filepath.Clean(tab.doc.Path())] = tab
	}
	for _, path := range paths {
		path = filepath.Clean(path)
		tab := openTabs[path]
		temporary := false
		if tab == nil {
			if m.reg == nil {
				continue
			}
			doc, err := m.reg.Open(ctx, path)
			if err != nil {
				continue
			}
			tab = &Tab{
				doc:   doc,
				state: &editor.State{Buffer: doc.Buffer()},
			}
			temporary = true
		}
		m.applyCommitModifications(ctx, tab, modifications)
		if temporary {
			_ = m.reg.Close(ctx, path)
		}
	}
}

func (m *Model) applyCommitModifications(
	ctx context.Context,
	tab *Tab,
	modifications vcsCommitModifications,
) {
	if tab == nil || tab.doc == nil {
		return
	}
	if modifications.optimizeImports {
		_ = m.applyAnalysisSourceAction(ctx, tab, "source.organizeImports")
	}
	if modifications.cleanup {
		_ = m.applyAnalysisSourceAction(ctx, tab, "source.fixAll")
	}
	if modifications.reformat {
		version := tab.doc.Buffer().Version()
		_, stale, err := m.applyLanguageFormat(
			ctx,
			tab,
			tab.doc.ID(),
			version,
			false,
			m.formatGuardForDocument(),
		)
		if err != nil || stale {
			return
		}
	}
	if err := tab.doc.Sync(ctx); err != nil {
		return
	}
	if err := tab.doc.Save(ctx); err != nil {
		m.post(func() {
			m.status = fmt.Sprintf(
				"Before-commit changes to %s could not be saved: %v",
				filepath.Base(tab.doc.Path()),
				err,
			)
			m.invalidate()
		})
	}
}
