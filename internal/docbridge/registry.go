package docbridge

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"

	"mayoi/internal/openidea"
)

// Registry owns the open documents of one backend window: one Document per
// file however many tabs show it, looked up by path and by backend document
// id, and fed the backend's document events.
type Registry struct {
	mu      sync.Mutex
	backend Backend
	window  string
	byPath  map[string]*entry
	byID    map[uint32]*entry
	onError func(path string, err error)
	onDiags func(ev openidea.DiagnosticsEvent)
}

type entry struct {
	doc  *Document
	refs int
}

// NewRegistry returns a registry of documents in window.
func NewRegistry(backend Backend, window string) *Registry {
	return &Registry{
		backend: backend,
		window:  window,
		byPath:  make(map[string]*entry),
		byID:    make(map[uint32]*entry),
	}
}

// SetErrorHandler receives failures that happen while handling backend events,
// such as a failed resync, so the UI can report them.
func (r *Registry) SetErrorHandler(fn func(path string, err error)) {
	r.mu.Lock()
	r.onError = fn
	r.mu.Unlock()
}

// SetDiagnosticsHandler receives the diagnostics the backend publishes for a
// file, on the goroutine that feeds the registry.
func (r *Registry) SetDiagnosticsHandler(fn func(ev openidea.DiagnosticsEvent)) {
	r.mu.Lock()
	r.onDiags = fn
	r.mu.Unlock()
}

// Open returns the document for path, opening it in the backend on first use.
// The registry holds one reference per call; Close releases one.
func (r *Registry) Open(ctx context.Context, path string) (*Document, error) {
	key := canonical(path)
	r.mu.Lock()
	if e := r.byPath[key]; e != nil {
		e.refs++
		r.mu.Unlock()
		return e.doc, nil
	}
	r.mu.Unlock()

	doc, err := Open(ctx, r.backend, r.window, path)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	infoKey := canonical(doc.Info().Path)
	e := r.byPath[infoKey]
	if e == nil {
		e = r.byPath[key]
	}
	if e != nil {
		// Another tab opened the same file first; keep its document.
		e.refs++
		r.mu.Unlock()
		_ = doc.Close(ctx)
		return e.doc, nil
	}
	e = &entry{doc: doc, refs: 1}
	r.byPath[key] = e
	r.byPath[infoKey] = e
	r.byID[doc.ID()] = e
	r.mu.Unlock()
	return doc, nil
}

// Get returns the open document for path, or nil.
func (r *Registry) Get(path string) *Document {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.byPath[canonical(path)]; e != nil {
		return e.doc
	}
	return nil
}

// ByID returns the open document with backend id, or nil.
func (r *Registry) ByID(id uint32) *Document {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.byID[id]; e != nil {
		return e.doc
	}
	return nil
}

// Close releases one reference to the document for path and closes it in the
// backend when the last tab lets go.
func (r *Registry) Close(ctx context.Context, path string) error {
	r.mu.Lock()
	e := r.byPath[canonical(path)]
	if e == nil {
		r.mu.Unlock()
		return nil
	}
	e.refs--
	if e.refs > 0 {
		r.mu.Unlock()
		return nil
	}
	for k, v := range r.byPath {
		if v == e {
			delete(r.byPath, k)
		}
	}
	delete(r.byID, e.doc.ID())
	r.mu.Unlock()
	return e.doc.Close(ctx)
}

// Documents returns the open documents.
func (r *Registry) Documents() []*Document {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := make(map[*Document]bool, len(r.byID))
	out := make([]*Document, 0, len(r.byID))
	for _, e := range r.byID {
		if !seen[e.doc] {
			seen[e.doc] = true
			out = append(out, e.doc)
		}
	}
	return out
}

// ReopenAll re-opens every document after the backend restarted, so the
// project and its unsaved edits survive the crash.
func (r *Registry) ReopenAll(ctx context.Context) error {
	docs := r.Documents()
	var first error
	for _, d := range docs {
		oldID := d.ID()
		if err := d.Reopen(ctx); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		r.mu.Lock()
		if e := r.byID[oldID]; e != nil {
			delete(r.byID, oldID)
			r.byID[d.ID()] = e
		}
		r.mu.Unlock()
	}
	return first
}

// CloseAll saves nothing and closes every document, as when a window closes.
func (r *Registry) CloseAll(ctx context.Context) {
	for _, d := range r.Documents() {
		_ = d.Close(ctx)
	}
	r.mu.Lock()
	r.byPath = make(map[string]*entry)
	r.byID = make(map[uint32]*entry)
	r.mu.Unlock()
}

// HandleNotification applies the document events of one manager notification
// and reports whether it was one.
func (r *Registry) HandleNotification(ctx context.Context, n openidea.Notification) bool {
	if n.Method != "event" {
		return false
	}
	var we openidea.WindowEvent
	if err := json.Unmarshal(n.Params, &we); err != nil {
		return true
	}
	if we.Window != nil && *we.Window != r.window {
		return true
	}
	switch openidea.EventType(we.Event) {
	case "docPatched":
		ev, err := openidea.DecodeDocPatched(we.Event)
		if err != nil {
			return true
		}
		doc := r.ByID(ev.Doc)
		if doc == nil {
			return true
		}
		p := Patch{
			BaseVersion:   ev.BaseVersion,
			Version:       ev.Version,
			Changes:       ev.Changes,
			WorkspaceEdit: ev.WorkspaceEdit,
		}
		if err := doc.ApplyPatch(ctx, p); err != nil {
			r.report(doc.Path(), err)
		}
		return true
	case "docDirty":
		var ev openidea.DocDirtyEvent
		if err := json.Unmarshal(we.Event, &ev); err != nil {
			return true
		}
		if doc := r.ByID(ev.Doc); doc != nil {
			doc.SetDirty(ev.Dirty)
		}
		return true
	case "diagnostics":
		var ev openidea.DiagnosticsEvent
		if err := json.Unmarshal(we.Event, &ev); err != nil {
			return true
		}
		r.mu.Lock()
		fn := r.onDiags
		r.mu.Unlock()
		if fn != nil {
			fn(ev)
		}
		return true
	}
	return true
}

// Run consumes a notification stream until ctx or the stream ends, keeping
// documents current with backend edits.
func (r *Registry) Run(ctx context.Context, events <-chan openidea.Notification) {
	for {
		select {
		case <-ctx.Done():
			return
		case n, ok := <-events:
			if !ok {
				return
			}
			r.HandleNotification(ctx, n)
		}
	}
}

func (r *Registry) report(path string, err error) {
	r.mu.Lock()
	fn := r.onError
	r.mu.Unlock()
	if fn != nil {
		fn(path, err)
	}
}

func canonical(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}
