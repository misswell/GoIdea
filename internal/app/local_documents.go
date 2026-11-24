package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

const firstLocalDocID uint32 = 1 << 31

// hybridDocs routes project documents to OpenIDEA and standalone files to a
// small local backend. The latter is the native equivalent of LightEdit: the
// same editor, save lifecycle and buffer semantics, without pretending that a
// project exists.
type hybridDocs struct {
	remote docbridge.Backend
	local  *localDocsBackend
	root   func() string
	// remotePath lets SDK and dependency roots outside the project use the
	// backend's VFS for ordinary folders and archive entries.
	remotePath func(string) bool

	mu      sync.RWMutex
	sources map[uint32]docSource

	ensureEncoding func(context.Context)
	encodingFor    func(string) string
}

type docSource uint8

const (
	docSourceRemote docSource = iota + 1
	docSourceLocal
)

func newHybridDocs(remote docbridge.Backend, root func() string) *hybridDocs {
	return &hybridDocs{
		remote:  remote,
		local:   newLocalDocsBackend(),
		root:    root,
		sources: make(map[uint32]docSource),
	}
}

func (h *hybridDocs) SetRemotePathMatcher(match func(string) bool) {
	h.remotePath = match
}

func (h *hybridDocs) SetEncodingRuntime(
	ensure func(context.Context),
	fallbackFor func(string) string,
) {
	h.ensureEncoding = ensure
	h.encodingFor = fallbackFor
}

func (h *hybridDocs) usesRemote(filePath string) bool {
	if root := h.root(); root != "" && inProject(root, filePath) {
		return true
	}
	return h.remotePath != nil && h.remotePath(filePath)
}

func (h *hybridDocs) DocOpen(ctx context.Context, window, path string) (openidea.DocInfo, error) {
	if h.usesRemote(path) {
		info, err := h.remote.DocOpen(ctx, window, path)
		if err == nil {
			h.remember(info.ID, docSourceRemote)
		}
		return info, err
	}
	if h.ensureEncoding != nil {
		h.ensureEncoding(ctx)
	}
	fallback := "utf-8"
	if h.encodingFor != nil {
		fallback = h.encodingFor(path)
	}
	info, err := h.local.docOpen(ctx, window, path, fallback)
	if err == nil {
		h.remember(info.ID, docSourceLocal)
	}
	return info, err
}

func (h *hybridDocs) DocEdit(
	ctx context.Context,
	window string,
	doc uint32,
	baseVersion uint32,
	changes position.ChangeSet,
) (uint32, error) {
	switch h.source(doc) {
	case docSourceLocal:
		return h.local.DocEdit(ctx, window, doc, baseVersion, changes)
	case docSourceRemote:
		return h.remote.DocEdit(ctx, window, doc, baseVersion, changes)
	default:
		return 0, fmt.Errorf("goidea: unknown document %d", doc)
	}
}

func (h *hybridDocs) DocSnapshot(ctx context.Context, window string, doc uint32) (openidea.DocInfo, error) {
	switch h.source(doc) {
	case docSourceLocal:
		return h.local.DocSnapshot(ctx, window, doc)
	case docSourceRemote:
		return h.remote.DocSnapshot(ctx, window, doc)
	default:
		return openidea.DocInfo{}, fmt.Errorf("goidea: unknown document %d", doc)
	}
}

func (h *hybridDocs) DocSave(ctx context.Context, window string, doc uint32) error {
	switch h.source(doc) {
	case docSourceLocal:
		return h.local.DocSave(ctx, window, doc)
	case docSourceRemote:
		return h.remote.DocSave(ctx, window, doc)
	default:
		return fmt.Errorf("goidea: unknown document %d", doc)
	}
}

func (h *hybridDocs) DocClose(ctx context.Context, window string, doc uint32) error {
	source := h.source(doc)
	defer h.forget(doc)
	switch source {
	case docSourceLocal:
		return h.local.DocClose(ctx, window, doc)
	case docSourceRemote:
		return h.remote.DocClose(ctx, window, doc)
	default:
		return nil
	}
}

func (h *hybridDocs) remember(doc uint32, source docSource) {
	h.mu.Lock()
	h.sources[doc] = source
	h.mu.Unlock()
}

func (h *hybridDocs) source(doc uint32) docSource {
	h.mu.RLock()
	source := h.sources[doc]
	h.mu.RUnlock()
	return source
}

func (h *hybridDocs) forget(doc uint32) {
	h.mu.Lock()
	delete(h.sources, doc)
	h.mu.Unlock()
}

type localDocsBackend struct {
	mu    sync.Mutex
	next  uint32
	docs  map[uint32]*localDocument
	paths map[string]uint32
}

type localDocument struct {
	id         uint32
	path       string
	text       string
	version    uint32
	mode       os.FileMode
	encoding   string
	hasBOM     bool
	lineEnding string
	readOnly   bool
}

func newLocalDocsBackend() *localDocsBackend {
	return &localDocsBackend{
		next:  firstLocalDocID,
		docs:  make(map[uint32]*localDocument),
		paths: make(map[string]uint32),
	}
}

func (b *localDocsBackend) DocOpen(_ context.Context, _ string, path string) (openidea.DocInfo, error) {
	return b.docOpen(context.Background(), "", path, "utf-8")
}

func (b *localDocsBackend) docOpen(_ context.Context, _ string, path, fallback string) (openidea.DocInfo, error) {
	clean := realPath(path)
	b.mu.Lock()
	if id, ok := b.paths[clean]; ok {
		doc := b.docs[id]
		if doc == nil {
			b.mu.Unlock()
			return openidea.DocInfo{}, fmt.Errorf("goidea: local document %s disappeared", path)
		}
		info := doc.info()
		b.mu.Unlock()
		return info, nil
	}

	info, err := os.Stat(clean)
	if err != nil {
		b.mu.Unlock()
		return openidea.DocInfo{}, err
	}
	if info.IsDir() {
		b.mu.Unlock()
		return openidea.DocInfo{}, fmt.Errorf("goidea: %s is a directory", path)
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		b.mu.Unlock()
		return openidea.DocInfo{}, err
	}
	text, encoding, hasBOM, lineEnding := decodeLocalText(data, fallback)
	id := b.nextLocalIDLocked()
	doc := &localDocument{
		id:         id,
		path:       clean,
		text:       text,
		mode:       info.Mode().Perm(),
		encoding:   encoding,
		hasBOM:     hasBOM,
		lineEnding: lineEnding,
		readOnly:   info.Mode().Perm()&0o222 == 0,
	}
	b.docs[id] = doc
	b.paths[clean] = id
	b.mu.Unlock()
	return doc.info(), nil
}

func (b *localDocsBackend) DocEdit(
	_ context.Context,
	_ string,
	id uint32,
	baseVersion uint32,
	changes position.ChangeSet,
) (uint32, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc := b.docs[id]
	if doc == nil {
		return 0, fmt.Errorf("goidea: unknown local document %d", id)
	}
	if doc.readOnly {
		return 0, errors.New("goidea: the file is read-only")
	}
	if baseVersion != doc.version {
		return 0, &openidea.Error{
			Code:    openidea.CodeInternalError,
			Kind:    "outOfSync",
			Message: fmt.Sprintf("document version %d is not %d", baseVersion, doc.version),
		}
	}
	text, err := applyLocalChanges(doc.text, changes)
	if err != nil {
		return 0, err
	}
	doc.text = text
	doc.version++
	return doc.version, nil
}

func (b *localDocsBackend) DocSnapshot(
	_ context.Context,
	_ string,
	id uint32,
) (openidea.DocInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc := b.docs[id]
	if doc == nil {
		return openidea.DocInfo{}, fmt.Errorf("goidea: unknown local document %d", id)
	}
	return doc.info(), nil
}

func (b *localDocsBackend) DocSave(_ context.Context, _ string, id uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc := b.docs[id]
	if doc == nil {
		return fmt.Errorf("goidea: unknown local document %d", id)
	}
	if doc.readOnly {
		return errors.New("goidea: the file is read-only")
	}
	data, err := encodeLocalText(doc.text, doc.encoding, doc.hasBOM, doc.lineEnding)
	if err != nil {
		return err
	}
	return writeFileAtomically(doc.path, data, doc.mode)
}

func (b *localDocsBackend) DocClose(_ context.Context, _ string, id uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if doc := b.docs[id]; doc != nil {
		delete(b.paths, doc.path)
		delete(b.docs, id)
	}
	return nil
}

func (b *localDocsBackend) nextLocalIDLocked() uint32 {
	for {
		id := b.next
		b.next++
		if b.next < firstLocalDocID {
			b.next = firstLocalDocID
		}
		if id >= firstLocalDocID && b.docs[id] == nil {
			return id
		}
	}
}

func (d *localDocument) info() openidea.DocInfo {
	return openidea.DocInfo{
		ID:         d.id,
		Path:       d.path,
		Text:       d.text,
		Version:    d.version,
		Language:   "",
		Encoding:   d.encoding,
		HasBOM:     d.hasBOM,
		LineEnding: d.lineEnding,
		ReadOnly:   d.readOnly,
		Dirty:      false,
		URI:        "file://" + filepath.ToSlash(d.path),
	}
}

func applyLocalChanges(text string, cs position.ChangeSet) (string, error) {
	total := position.UTF16Length(text)
	mapper := position.NewMapper(text)
	type byteEdit struct {
		start int
		end   int
		text  string
	}
	edits := make([]byteEdit, 0, len(cs.Changes))
	var previousTo uint32
	for i, change := range cs.Changes {
		if change.From > change.To || change.To > uint32(total) {
			return "", fmt.Errorf("goidea: change %d is outside the document", i)
		}
		if i > 0 && change.From < previousTo {
			return "", fmt.Errorf("goidea: change %d overlaps the previous change", i)
		}
		previousTo = change.To
		start := mapper.UTF16OffsetToByteOffset(position.StringText(text), int(change.From))
		end := mapper.UTF16OffsetToByteOffset(position.StringText(text), int(change.To))
		edits = append(edits, byteEdit{start: start, end: end, text: strings.ReplaceAll(change.Insert, "\r\n", "\n")})
	}
	var out strings.Builder
	out.Grow(len(text))
	last := 0
	for _, edit := range edits {
		out.WriteString(text[last:edit.start])
		out.WriteString(edit.text)
		last = edit.end
	}
	out.WriteString(text[last:])
	return out.String(), nil
}

func writeFileAtomically(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".goidea-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
