package app

import (
	"context"
	"fmt"
	"path/filepath"

	"mayoi/internal/openidea"
)

type resolvedCommitPlan struct {
	stage    []string
	exclude  []string
	included []string
	conflict []string
	partial  []openidea.PartialCommitFile
}

// commitRepositoryForPath assigns an included file to one backend repository.
// Unknown paths fall back to the selected repository, or to the sole known
// repository in a single-repository project.
func (m *Model) commitRepositoryForPath(c *commitState, path string) string {
	repository := vcsRepositoryForPath(path, m.vcs.repositories)
	if repository != "" {
		return repository
	}
	if c != nil && c.repo != "" {
		return filepath.Clean(c.repo)
	}
	if len(m.vcs.repositories) == 1 {
		return filepath.Clean(m.vcs.repositories[0].Root)
	}
	return ""
}

// commitRepositories returns every repository with an included change in
// first-seen order, with the selected repository last. With no included files,
// Amend still targets the selected repository.
func (m *Model) commitRepositories(c *commitState, plan resolvedCommitPlan) []string {
	repositories := make([]string, 0)
	seen := map[string]bool{}
	for _, path := range plan.included {
		repository := m.commitRepositoryForPath(c, path)
		key := vcsRepositoryKey(repository)
		if seen[key] {
			continue
		}
		seen[key] = true
		repositories = append(repositories, repository)
	}
	if len(repositories) == 0 {
		if c != nil && c.amend {
			return []string{m.selectedCommitRepository(c)}
		}
		return nil
	}
	selected := m.selectedCommitRepository(c)
	selectedKey := vcsRepositoryKey(selected)
	if selectedKey == "" {
		return repositories
	}
	for i, repository := range repositories {
		if vcsRepositoryKey(repository) != selectedKey {
			continue
		}
		copy(repositories[i:], repositories[i+1:])
		repositories[len(repositories)-1] = selected
		return repositories
	}
	return repositories
}

func (m *Model) selectedCommitRepository(c *commitState) string {
	if c != nil && c.repo != "" {
		return filepath.Clean(c.repo)
	}
	return filepath.Clean(m.vcsRepositoryRoot())
}

func (m *Model) commitOptionsForRepository(
	c *commitState,
	plan resolvedCommitPlan,
	repository string,
) openidea.CommitOptions {
	inRepository := func(path string) bool {
		return vcsRepositoryKey(m.commitRepositoryForPath(c, path)) ==
			vcsRepositoryKey(repository)
	}
	options := openidea.CommitOptions{
		Amend: c.amend, SignOff: c.signOff, RunHooks: c.runHooks,
	}
	for _, path := range plan.stage {
		if inRepository(path) {
			options.Stage = append(options.Stage, path)
		}
	}
	for _, path := range plan.exclude {
		if inRepository(path) {
			options.Exclude = append(options.Exclude, path)
		}
	}
	for _, file := range plan.partial {
		if inRepository(file.Path) {
			options.Partial = append(options.Partial, file)
		}
	}
	return options
}

// resolvePartialCommitPlan refreshes HEAD and disk after saves and
// before-commit modifications. Stable hunk identities keep a selection when
// unrelated lines moved, while a changed selected block refuses the commit.
func (m *Model) resolvePartialCommitPlan(
	ctx context.Context,
	plan commitPlan,
) (resolvedCommitPlan, error) {
	resolved := resolvedCommitPlan{
		stage:    append([]string(nil), plan.stage...),
		exclude:  append([]string(nil), plan.exclude...),
		included: append([]string(nil), plan.included...),
		conflict: append([]string(nil), plan.conflict...),
	}
	for _, selected := range plan.partial {
		_, disk, hunks, err := m.commitHunks(ctx, selected.Path)
		if err != nil {
			return resolvedCommitPlan{}, err
		}
		keys := commitHunkKeys(disk, hunks)
		wanted := make(map[string]bool, len(selected.Keys))
		for _, key := range selected.Keys {
			wanted[key] = true
		}
		found := make(map[string]bool, len(selected.Keys))
		chosen := make([]openidea.HunkRange, 0, len(selected.Keys))
		for i, hunk := range hunks {
			if !wanted[keys[i]] {
				continue
			}
			found[keys[i]] = true
			chosen = append(chosen, openidea.HunkRange{
				Start: hunk.Start, Lines: hunk.Lines,
				OldStart: hunk.OldStart, OldLines: hunk.OldLines,
			})
		}
		if len(found) != len(wanted) {
			return resolvedCommitPlan{}, fmt.Errorf(
				"The selected changes in %s changed on disk; open Hunks and review them again",
				filepath.Base(selected.Path),
			)
		}
		if len(chosen) == len(hunks) {
			resolved.stage = append(resolved.stage, selected.Path)
			continue
		}
		if len(chosen) == 0 {
			return resolvedCommitPlan{}, fmt.Errorf(
				"No selected line changes remain in %s",
				filepath.Base(selected.Path),
			)
		}
		resolved.partial = append(resolved.partial, openidea.PartialCommitFile{
			Path: selected.Path, Hunks: chosen,
		})
	}
	return resolved, nil
}

func (m *Model) commitHunks(
	ctx context.Context,
	path string,
) (head, disk string, hunks []openidea.LineHunk, err error) {
	raw, err := m.invoke(ctx, "vcs_file_at", map[string]any{
		"path": path, "rev": "HEAD",
	})
	if err != nil {
		return "", "", nil, err
	}
	var headText *string
	if err = decodeOpenIDEAResponse(raw, &headText); err != nil {
		return "", "", nil, err
	}
	if headText == nil {
		return "", "", nil, fmt.Errorf("%s is not in HEAD", filepath.Base(path))
	}
	head = *headText
	raw, err = m.invoke(ctx, "fs_read_text", map[string]any{"path": path})
	if err != nil {
		return "", "", nil, err
	}
	if err = decodeOpenIDEAResponse(raw, &disk); err != nil {
		return "", "", nil, err
	}
	raw, err = m.invoke(ctx, "vcs_diff_hunks", map[string]any{
		"oldText": head, "newText": disk,
	})
	if err != nil {
		return "", "", nil, err
	}
	if err = decodeOpenIDEAResponse(raw, &hunks); err != nil {
		return "", "", nil, err
	}
	return head, disk, hunks, nil
}
