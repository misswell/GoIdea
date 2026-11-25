package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	maxSafeDeleteFiles       = 20
	safeDeleteTimeout        = 15 * time.Second
	safeDeleteProgressDelay  = 400 * time.Millisecond
	maxSafeDeleteSymbolsFile = 60
)

type treeSafeDeletePhase int

const (
	treeSafeDeleteSearching treeSafeDeletePhase = iota
	treeSafeDeleteDetected
	treeSafeDeleteComplete
)

// treeSafeDeleteRun coordinates one project-view usage search. The request
// and cancellation fields are shared with the worker, while the phase and
// locations are changed only on the UI thread.
type treeSafeDeleteRun struct {
	phase     treeSafeDeletePhase
	progress  bool
	files     []string
	locations []openidea.Location
	timer     *time.Timer

	mu      sync.Mutex
	request uint32
	cancel  context.CancelFunc
}

func (r *treeSafeDeleteRun) setCancel(cancel context.CancelFunc) {
	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()
}

func (r *treeSafeDeleteRun) setRequest(request uint32) {
	r.mu.Lock()
	r.request = request
	r.mu.Unlock()
}

func (r *treeSafeDeleteRun) clearRequest(request uint32) {
	r.mu.Lock()
	if r.request == request {
		r.request = 0
	}
	r.mu.Unlock()
}

func (r *treeSafeDeleteRun) stop() uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	request := r.request
	r.request = 0
	return request
}

func treeSafeDeleteFiles(paths []string) []string {
	files := make([]string, 0, len(paths))
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files = append(files, path)
		}
	}
	return files
}

// startTreeSafeDelete runs OpenIDEA's bounded file usage search. If it finds
// nothing, cannot run, or the user chooses Delete Without Search, the same
// confirmed Delete continues.
func (m *Model) startTreeSafeDelete(v *treeOpState) {
	files := treeSafeDeleteFiles(v.paths)
	if len(files) == 0 || len(files) > maxSafeDeleteFiles {
		m.performTreeDelete(v)
		return
	}
	ctx, cancel := context.WithTimeout(m.ctx, safeDeleteTimeout)
	run := &treeSafeDeleteRun{
		phase: treeSafeDeleteSearching,
		files: append([]string(nil), files...),
	}
	run.setCancel(cancel)
	v.safeRun = run
	v.busy = true
	v.err = ""
	m.status = "Looking for usages…"
	m.invalidate()

	run.timer = time.AfterFunc(safeDeleteProgressDelay, func() {
		m.post(func() {
			if m.treeOp != v || v.safeRun != run ||
				run.phase != treeSafeDeleteSearching || run.progress {
				return
			}
			run.progress = true
			m.invalidate()
		})
	})

	go func() {
		locations, searched, err := m.fileUsagesForSafeDelete(ctx, files, run)
		m.post(func() {
			if run.timer != nil {
				run.timer.Stop()
			}
			if m.treeOp != v || v.safeRun != run {
				return
			}
			v.busy = false
			if err != nil {
				cancel()
				m.CloseTreeOp()
				m.notifyWarning(
					"project.safeDelete",
					"Usage search did not finish; nothing was deleted",
					err.Error(),
				)
				return
			}
			if !searched || len(locations) == 0 {
				run.phase = treeSafeDeleteComplete
				m.performTreeDelete(v)
				return
			}
			run.locations = append([]openidea.Location(nil), locations...)
			run.phase = treeSafeDeleteDetected
			run.progress = true
			m.status = ""
			m.invalidate()
		})
	}()
}

func (m *Model) cancelTreeSafeDelete(v *treeOpState) {
	if v == nil || v.safeRun == nil {
		return
	}
	run := v.safeRun
	v.safeRun = nil
	if run.timer != nil {
		run.timer.Stop()
	}
	if request := run.stop(); request != 0 {
		m.cancelRequest(request)
	}
}

// fileUsagesForSafeDelete mirrors OpenIDEA's fileUsages. It searches the
// top-level declarations of each file, excludes every file being deleted, and
// reports unsupported when no server answered any symbol.
func (m *Model) fileUsagesForSafeDelete(
	ctx context.Context,
	paths []string,
	run *treeSafeDeleteRun,
) ([]openidea.Location, bool, error) {
	deleted := make(map[string]bool, len(paths))
	for _, path := range paths {
		deleted[filepath.Clean(path)] = true
	}
	found := map[string]openidea.Location{}
	var foundOrder []string
	searched := false
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, searched, err
		}
		searchErr := func() error {
			doc, closeDoc, err := m.openTreeSafeDeleteDocument(ctx, path)
			if err != nil {
				return nil
			}
			defer closeDoc()

			raw, outlineErr := m.invoke(ctx, "doc_outline", map[string]any{"doc": doc})
			if outlineErr != nil {
				return ctx.Err()
			}
			var outline []openidea.OutlineItem
			if json.Unmarshal(raw, &outline) != nil {
				return nil
			}
			symbols := safeDeleteDeclaredSymbols(outline)
			if len(symbols) > maxSafeDeleteSymbolsFile {
				symbols = symbols[:maxSafeDeleteSymbolsFile]
			}
			for _, symbol := range symbols {
				if err := ctx.Err(); err != nil {
					return err
				}
				request := m.nextRequest()
				channel := m.nextChannel()
				run.setRequest(request)
				raw, err := m.invoke(ctx, "lsp_references", map[string]any{
					"request": request, "doc": doc, "pos": symbol.SelectionRange.Start,
					"includeDeclaration": false, "projectWide": false, "libraries": false,
					"onPartial": map[string]any{"$channel": channel},
				})
				run.clearRequest(request)
				if err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					continue
				}
				var refs openidea.References
				if json.Unmarshal(raw, &refs) != nil {
					continue
				}
				searched = true
				for _, location := range refs.Locations {
					path := filepath.Clean(location.Path)
					if deleted[path] {
						continue
					}
					key := fmt.Sprintf("%s:%d:%d", path,
						location.Range.Start.Line, location.Range.Start.Col)
					if _, exists := found[key]; !exists {
						foundOrder = append(foundOrder, key)
					}
					found[key] = location
				}
			}
			return nil
		}()
		if searchErr != nil {
			return nil, searched, searchErr
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, searched, err
	}
	locations := make([]openidea.Location, 0, len(found))
	for _, key := range foundOrder {
		locations = append(locations, found[key])
	}
	return locations, searched, nil
}

// safeDeleteDeclaredSymbols removes package and namespace containers, whose
// members stand in for them, as OpenIDEA's declaredSymbols does.
func safeDeleteDeclaredSymbols(items []openidea.OutlineItem) []openidea.OutlineItem {
	var out []openidea.OutlineItem
	for _, item := range items {
		if item.Kind == "package" || item.Kind == "namespace" {
			out = append(out, safeDeleteDeclaredSymbols(item.Children)...)
			continue
		}
		out = append(out, item)
	}
	return out
}

func (m *Model) openTreeSafeDeleteDocument(
	ctx context.Context,
	path string,
) (uint32, func(), error) {
	if m.reg != nil {
		doc, err := m.reg.Open(ctx, path)
		if err != nil {
			return 0, func() {}, err
		}
		if err := doc.Sync(ctx); err != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = m.reg.Close(closeCtx, path)
			return 0, func() {}, err
		}
		return doc.ID(), func() {
			closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = m.reg.Close(closeCtx, path)
		}, nil
	}

	raw, err := m.invoke(ctx, "doc_open", map[string]any{"path": path})
	if err != nil {
		return 0, func() {}, err
	}
	var info openidea.DocInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return 0, func() {}, err
	}
	return info.ID, func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = m.invoke(closeCtx, "doc_close", map[string]any{"doc": info.ID})
	}, nil
}

func (m *Model) treeSafeDeleteSkip(v *treeOpState) {
	if v == nil || v.safeRun == nil {
		return
	}
	m.cancelTreeSafeDelete(v)
	v.busy = false
	m.performTreeDelete(v)
}

func (m *Model) treeSafeDeleteViewUsages(v *treeOpState) {
	if v == nil || v.safeRun == nil {
		return
	}
	locations := append([]openidea.Location(nil), v.safeRun.locations...)
	name := filepath.Base(v.path)
	if len(v.safeRun.files) > 1 {
		name = fmt.Sprintf("%d files", len(v.safeRun.files))
	}
	m.cancelTreeSafeDelete(v)
	m.treeOp = nil
	m.showUsageResults(name, locations, nil, "")
}

func (m *Model) treeSafeDeleteAnyway(v *treeOpState) {
	if v == nil || v.safeRun == nil {
		return
	}
	m.cancelTreeSafeDelete(v)
	v.busy = false
	m.performTreeDelete(v)
}

func (m *Model) treeSafeDeleteDialog(c *ui.Context, p *ui.Theme, v *treeOpState) {
	run := v.safeRun
	if run == nil || run.phase == treeSafeDeleteComplete {
		return
	}
	if run.phase == treeSafeDeleteSearching && !run.progress {
		return
	}

	open := v.open
	title := "Delete"
	if run.phase == treeSafeDeleteDetected {
		title = "Usages Detected"
	}
	d := nativeModal(c.Key("project-tree-safe-delete"), &open, func() {
		ui.Column(c).Width(480).Gap(10).Children(func() {
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			switch run.phase {
			case treeSafeDeleteSearching:
				ui.Text(c, "Looking for usages…").FontSize(12).TextColor(p.TextMuted)
				ui.Text(c, "The search stops after 15 seconds.").
					FontSize(12).TextColor(p.TextMuted)
			case treeSafeDeleteDetected:
				n := len(run.locations)
				files := len(groupUsages(run.locations))
				message := fmt.Sprintf("%d usages found", n)
				if n == 1 {
					message = "1 usage found"
				}
				where := ""
				if files > 1 {
					where = fmt.Sprintf(" (in %d files)", files)
				}
				subject := fmt.Sprintf("File '%s'", filepath.Base(v.path))
				if len(run.files) > 1 {
					subject = fmt.Sprintf("The %d files", len(run.files))
				}
				detail := fmt.Sprintf("%s have %d usages that are not safe to delete%s.",
					subject, n, where)
				if len(run.files) == 1 {
					detail = fmt.Sprintf("%s has %d usages that are not safe to delete%s.",
						subject, n, where)
				}
				if n == 1 {
					detail = fmt.Sprintf("%s has 1 usage that is not safe to delete%s.",
						subject, where)
					if len(run.files) > 1 {
						detail = fmt.Sprintf("%s have 1 usage that is not safe to delete%s.",
							subject, where)
					}
				}
				ui.Text(c, message).FontSize(12).TextColor(p.Warning)
				ui.Text(c, detail).FontSize(12).TextColor(p.TextMuted)
			}
			ui.Row(c).Gap(8).Children(func() {
				switch run.phase {
				case treeSafeDeleteSearching:
					ui.Button(c, "Delete Without Search").OnClick(func() {
						m.treeSafeDeleteSkip(v)
					})
					ui.Button(c, "Cancel").OnClick(func() { m.CloseTreeOp() })
				case treeSafeDeleteDetected:
					ui.Button(c, "View Usages").OnClick(func() {
						m.treeSafeDeleteViewUsages(v)
					})
					ui.Button(c, "Delete Anyway").OnClick(func() {
						m.treeSafeDeleteAnyway(v)
					})
					ui.Button(c, "Cancel").OnClick(func() { m.CloseTreeOp() })
				}
			})
		})
	})
	if d.Dismissed() || !open {
		m.CloseTreeOp()
	}
}
