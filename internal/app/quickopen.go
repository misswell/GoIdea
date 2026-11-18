package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

const (
	fileQueryTimeout = 30 * time.Second
	// maxFileResults bounds the picker; a one-letter query matches most of a
	// large project.
	maxFileResults = 200
	// fileDebounce keeps typing from querying the index on every keystroke.
	fileDebounce = 120 * time.Millisecond
)

// filePicker is the Quick Open prompt: the typed name and the indexed files it
// matched. An empty query lists recently opened files.
type filePicker struct {
	open  bool
	query string
	items []openidea.FileMatch
	sel   int
	busy  bool
	done  bool
	err   string

	truncated bool
	seq       int
	request   uint32
}

// OpenQuickFiles opens Quick Open over the window.
func (m *Model) OpenQuickFiles() {
	if m.files != nil || m.Root() == "" {
		return
	}
	m.files = &filePicker{open: true}
	m.queryFiles()
}

// fileListHeight is the Quick Open list's height: its 24px rows, capped so a
// long index scrolls instead of growing the card.
func fileListHeight(rows int) float32 {
	h := float32(rows) * 24
	if h > 330 {
		h = 330
	}
	return h
}

// closeQuickFiles drops the picker and cancels a lookup still in flight.
func (m *Model) closeQuickFiles() {
	if m.files == nil {
		return
	}
	m.cancelFileQuery()
	m.files = nil
	m.invalidate()
}

// queryFiles asks the index for files matching the typed name. An answer to a
// query the user has typed past is dropped.
func (m *Model) queryFiles() {
	f := m.files
	if f == nil {
		return
	}
	f.seq++
	seq := f.seq
	query := strings.TrimSpace(f.query)
	m.cancelFileQuery()
	f.err = ""
	f.busy = true
	f.done = false
	req := m.nextRequest()
	f.request = req
	m.invalidate()
	go func() {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(fileDebounce):
		}
		var stale bool
		mygo.RunOnMain(func() { stale = m.files != f || f.seq != seq })
		if stale {
			return
		}
		ctx, cancel := context.WithTimeout(m.ctx, fileQueryTimeout)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "find_files", map[string]any{
			"query": query, "limit": maxFileResults + 1,
		})
		var files []openidea.FileMatch
		if err == nil {
			err = json.Unmarshal(raw, &files)
		}
		m.post(func() {
			if m.files != f || f.seq != seq {
				return
			}
			f.busy = false
			f.request = 0
			f.done = true
			if err != nil {
				if openidea.ErrorKind(err) != "cancelled" {
					f.err = "Finding files failed: " + err.Error()
				}
				f.items = nil
				f.sel = 0
				m.invalidate()
				return
			}
			f.truncated = len(files) > maxFileResults
			if f.truncated {
				files = files[:maxFileResults]
			}
			f.items = files
			f.sel = 0
			m.invalidate()
		})
	}()
}

func (m *Model) cancelFileQuery() {
	f := m.files
	if f == nil || f.request == 0 {
		return
	}
	id := f.request
	f.request = 0
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.mgr.Invoke(ctx, Window, "cancel_request", map[string]any{"id": id})
	}()
}

// stepFile moves the chosen row by delta, stopping at the ends.
func (m *Model) stepFile(delta int) {
	f := m.files
	if f == nil || len(f.items) == 0 {
		return
	}
	f.sel = clampSymbolSel(f.sel+delta, len(f.items))
	m.invalidate()
}

// openSelectedFile opens the chosen file.
func (m *Model) openSelectedFile() {
	f := m.files
	if f == nil {
		return
	}
	if len(f.items) == 0 {
		m.invalidate()
		return
	}
	fm := f.items[clampSymbolSel(f.sel, len(f.items))]
	m.closeQuickFiles()
	m.openFile(fm.Path)
}

// markRecent tells the index a file was shown, so Quick Open's empty query
// lists it.
func (m *Model) markRecent(path string) {
	if path == "" || m.Root() == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		_, _ = m.mgr.Invoke(ctx, Window, "mark_recent", map[string]any{"path": path})
	}()
}

// filesDialog draws Quick Open and its results.
func (m *Model) filesDialog(c *ui.Context, p *ui.Theme) {
	f := m.files
	if f == nil {
		return
	}
	open := f.open
	d := ui.Modal(c, &open, func() {
		ui.Column(c).Width(600).Gap(8).Children(func() {
			ui.Text(c, "Quick Open").FontSize(13).Bold().TextColor(p.Text).SingleLine()
			in := ui.TextInput(c, &f.query).AutoFocus()
			in.OnChange(func() { m.queryFiles() })
			in.OnShortcut(0, ui.KeyDown, func() { m.stepFile(1) })
			in.OnShortcut(0, ui.KeyUp, func() { m.stepFile(-1) })
			if in.Submitted() {
				m.openSelectedFile()
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				text := "Type a file name; Enter opens it"
				switch {
				case f.busy:
					text = "Searching…"
				case f.err != "":
					text = ""
				case strings.TrimSpace(f.query) == "":
					if f.done && len(f.items) == 0 {
						text = "No recently opened files yet"
					} else if len(f.items) > 0 {
						text = "Recently opened"
					}
				case f.done && len(f.items) == 0:
					text = "No matching files"
				case len(f.items) > 0:
					text = count(len(f.items), "result", "results")
					if f.truncated {
						text = fmt.Sprintf("Too many results; showing the first %d", maxFileResults)
					}
				}
				if text != "" {
					ui.Text(c, text).FontSize(12).TextColor(p.TextMuted).SingleLine()
				}
			})
			if f.err != "" {
				ui.Text(c, f.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			if len(f.items) == 0 {
				return
			}
			// The list hugs its rows up to the cap: a fixed 330px box left a
			// mostly empty card when Quick Open had one or two matches.
			ui.Scroll(c).Height(fileListHeight(len(f.items))).Children(func() {
				ui.Column(c).Children(func() {
					for i, fm := range f.items {
						i, fm := i, fm
						r := ui.Row(c.Key(fmt.Sprintf("file:%d", i))).Height(24).Padding(0, 8).
							Gap(8).AlignItems(ui.Center)
						if i == f.sel {
							r.Background(p.SurfaceHover)
						} else if r.Hovered() {
							r.Background(p.SurfaceHover)
						}
						r.Children(func() {
							fileIconEl(c, oiPaletteOf(p), fm.Path, 16, p.TextMuted)
							name, dir := filePathParts(fm.RelPath)
							ui.Text(c, name).FontSize(12).Bold().TextColor(p.Text).
								SingleLine().Ellipsis("…")
							if dir != "" {
								ui.Text(c, dir).FontSize(12).TextColor(p.TextMuted).
									SingleLine().Ellipsis("…")
							}
						})
						if r.Clicked() {
							m.closeQuickFiles()
							m.openFile(fm.Path)
						}
					}
				})
			})
		})
	})
	if d.Dismissed() || !open {
		m.closeQuickFiles()
	}
}

// filePathParts splits a relative path into its base name and the directory
// before it, as Quick Open shows them.
func filePathParts(rel string) (string, string) {
	rel = strings.TrimPrefix(rel, "./")
	i := strings.LastIndexAny(rel, "/\\")
	if i < 0 {
		return rel, ""
	}
	return rel[i+1:], rel[:i]
}
