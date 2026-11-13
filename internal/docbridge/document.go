// Package docbridge keeps one shared editor buffer in step with OpenIDEA's
// documents: local edits travel to the backend as UTF-16 change sets, backend
// edits come back as patches, and undo, redo, save and out-of-sync recovery
// keep the two texts equal without losing unsaved work.
package docbridge

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui/editor"
	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

// Backend is the document half of the OpenIDEA protocol.
type Backend interface {
	DocOpen(ctx context.Context, window, path string) (openidea.DocInfo, error)
	DocEdit(ctx context.Context, window string, doc uint32, baseVersion uint32, changes position.ChangeSet) (uint32, error)
	DocSnapshot(ctx context.Context, window string, doc uint32) (openidea.DocInfo, error)
	DocSave(ctx context.Context, window string, doc uint32) error
	DocClose(ctx context.Context, window string, doc uint32) error
}

var (
	// ErrClosed is returned by a document the registry closed.
	ErrClosed = errors.New("docbridge: document is closed")
	// ErrReadOnly is returned for edits of a read-only document.
	ErrReadOnly = errors.New("docbridge: document is read-only")
	// ErrResync gives up when the backend keeps changing under a resync.
	ErrResync = errors.New("docbridge: backend versions keep moving during resync")
)

// ConflictError reports that a backend change and unsaved local edits touched
// the same place. The buffer keeps the local text; Resolve picks a side.
type ConflictError struct {
	Path   string
	Local  []Hunk
	Remote []Hunk
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("docbridge: %s: local and backend edits conflict", e.Path)
}

// Patch is a backend-originated change: Changes are UTF-16 into BaseVersion
// and make Version.
type Patch struct {
	BaseVersion   uint32
	Version       uint32
	Changes       []position.TextChange
	WorkspaceEdit bool
}

// Document is one file open in the backend with one shared editor buffer, its
// mapper for UTF-16 conversion, and the version bookkeeping of the sync.
//
// All edits must go through the document (ApplyEdits, ApplyPatch, Undo, Redo,
// Resync): it maintains the buffer, the mapper and the backend together.
type Document struct {
	mu       sync.Mutex
	backend  Backend
	window   string
	id       uint32
	path     string
	info     openidea.DocInfo
	buf      *editor.Buffer
	mapper   *position.Mapper
	indexed  uint64 // buffer version the mapper describes
	version  uint32 // backend version of base
	base     editor.Snapshot
	baseBuf  uint64 // buffer version of base
	unsynced bool
	dirty    bool
	saved    uint64 // hash of the text on disk at the last open or save
	closed   bool
	pending  *pendingRemote
	wake     chan struct{}
}

type pendingRemote struct {
	text    string
	version uint32
	local   []Hunk
	remote  []Hunk
}

// Open opens path in the backend and returns its document, with a buffer
// initialized from the backend text.
func Open(ctx context.Context, backend Backend, window, path string) (*Document, error) {
	info, err := backend.DocOpen(ctx, window, path)
	if err != nil {
		return nil, err
	}
	return newDocument(backend, window, info), nil
}

func newDocument(backend Backend, window string, info openidea.DocInfo) *Document {
	text := lf(info.Text)
	buf := editor.NewBuffer(text)
	d := &Document{
		backend: backend,
		window:  window,
		id:      info.ID,
		path:    info.Path,
		info:    info,
		buf:     buf,
		mapper:  position.NewMapper(text),
		version: info.Version,
		base:    buf.Snapshot(),
		dirty:   info.Dirty,
		saved:   textHash(text),
		wake:    make(chan struct{}, 1),
	}
	d.baseBuf = d.base.Version()
	d.indexed = d.base.Version()
	return d
}

// lf normalizes line endings the way the backend and the editor both store
// text, so offsets agree.
func lf(s string) string {
	if strings.Contains(s, "\r\n") {
		return strings.ReplaceAll(s, "\r\n", "\n")
	}
	return s
}

// Path, ID, Info, Version, Dirty and ReadOnly report the document's state.
func (d *Document) Path() string { return d.path }

func (d *Document) ID() uint32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.id
}

func (d *Document) Info() openidea.DocInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.info
}

func (d *Document) Version() uint32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.version
}

func (d *Document) Dirty() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dirty
}

func (d *Document) ReadOnly() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.info.ReadOnly
}

// Buffer returns the shared editor buffer, for editors and views to read.
func (d *Document) Buffer() *editor.Buffer { return d.buf }

// Mapper returns the byte/UTF-16 mapper of the buffer's current text.
func (d *Document) Mapper() *position.Mapper { return d.mapper }

// UTF16Pos converts a byte offset of the buffer to a protocol position.
func (d *Document) UTF16Pos(off int) openidea.OutlinePos {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur := d.buf.Snapshot()
	d.catchUpLocked(cur)
	p := d.mapper.ByteOffsetToUTF16Position(snapText{cur}, off)
	return openidea.OutlinePos{Line: p.Line, Col: p.Character}
}

// ByteOffset converts a protocol position to a byte offset of the buffer.
func (d *Document) ByteOffset(p openidea.OutlinePos) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur := d.buf.Snapshot()
	d.catchUpLocked(cur)
	return d.mapper.UTF16PositionToByteOffset(snapText{cur}, position.Position{
		Line: p.Line, Character: p.Col,
	})
}

// BytePos converts a protocol line and UTF-16 column to a line and byte
// column, as the editor takes them.
func (d *Document) BytePos(line, col int) (int, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur := d.buf.Snapshot()
	d.catchUpLocked(cur)
	return d.mapper.UTF16PositionToBytePosition(snapText{cur}, position.Position{
		Line: uint32(line), Character: uint32(col),
	})
}

// ByteRange converts a protocol range to byte offsets of the buffer.
func (d *Document) ByteRange(r openidea.OutlineRange) (start, end int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur := d.buf.Snapshot()
	d.catchUpLocked(cur)
	return d.mapper.RangeToByteOffsets(snapText{cur}, position.Range{
		Start: position.Position{Line: r.Start.Line, Character: r.Start.Col},
		End:   position.Position{Line: r.End.Line, Character: r.End.Col},
	})
}

// ByteEdits converts server text edits, whose ranges are UTF-16 positions of
// the current text, to buffer byte edits, so a server's formatting can be
// applied with ApplyEdits as one undo step.
func (d *Document) ByteEdits(edits []openidea.TextEdit) []editor.Edit {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur := d.buf.Snapshot()
	d.catchUpLocked(cur)
	out := make([]editor.Edit, 0, len(edits))
	for _, e := range edits {
		start, end := d.mapper.RangeToByteOffsets(snapText{cur}, position.Range{
			Start: position.Position{Line: e.Range.Start.Line, Character: e.Range.Start.Col},
			End:   position.Position{Line: e.Range.End.Line, Character: e.Range.End.Col},
		})
		out = append(out, editor.Edit{Start: start, End: end, Text: e.NewText})
	}
	return out
}

// catchUpLocked moves the mapper to the buffer's text when a widget edited the
// shared buffer since the mapper last moved. The mapper describes the base in
// that state (every other path moves it with the buffer as it edits), so the
// diff from the base is what it missed.
func (d *Document) catchUpLocked(cur editor.Snapshot) {
	if d.indexed == cur.Version() || d.indexed != d.baseBuf {
		return
	}
	d.applyHunksToIndexLocked(snapText{d.base}, snapText{cur}, diffTexts(snapText{d.base}, snapText{cur}))
	d.indexed = cur.Version()
}

// Changes signals that the buffer's text changed, from any goroutine. Slow
// consumers keep one pending signal.
func (d *Document) Changes() <-chan struct{} { return d.wake }

func (d *Document) signal() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Text returns the buffer's text.
func (d *Document) Text() string { return d.buf.String() }

// ApplyEdits applies editor edits, whose offsets are bytes of the current
// buffer text before any of them, as one undo step, then sends them to the
// backend. Overlapping edits are clipped in order, as the editor does.
func (d *Document) ApplyEdits(ctx context.Context, edits ...editor.Edit) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if d.info.ReadOnly {
		return ErrReadOnly
	}
	before := d.buf.Snapshot()
	norm := normalizeEdits(edits, before)
	if len(norm) == 0 {
		return nil
	}

	changes := make([]position.TextChange, 0, len(norm))
	for _, e := range norm {
		changes = append(changes, position.TextChange{
			From:   uint32(d.mapper.ByteOffsetToUTF16Offset(before, e.Start)),
			To:     uint32(d.mapper.ByteOffsetToUTF16Offset(before, e.End)),
			Insert: e.Text,
		})
	}

	d.applyBufferEditsLocked(norm)
	d.signal()
	if d.pending != nil {
		return d.conflictErrorLocked()
	}
	if d.unsynced {
		return d.syncLocked(ctx)
	}
	return d.pushLocked(ctx, position.ChangeSet{Changes: changes})
}

func (d *Document) conflictErrorLocked() error {
	return &ConflictError{Path: d.path, Local: d.pending.local, Remote: d.pending.remote}
}

// ApplyPatch applies a backend-originated patch. A patch whose base is not the
// current version, or that arrives while the buffer is ahead of the base,
// triggers a resync instead of being guessed at: its offsets describe the base
// text, not what the user has typed since.
func (d *Document) ApplyPatch(ctx context.Context, p Patch) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	if p.Version <= d.version {
		return nil
	}
	cur := d.buf.Snapshot()
	if p.BaseVersion != d.version || d.unsynced || cur.Version() != d.baseBuf {
		return d.resyncLocked(ctx, 0)
	}
	edits := d.mapper.ChangeSetToByteEdits(cur, position.ChangeSet{Changes: p.Changes})
	if p.WorkspaceEdit && len(edits) > 0 {
		// A grouped backend edit (rename, refactor, build) must not become a
		// file-local undo step: Ctrl+Z would send the refactor back as a user
		// edit. The buffer cannot apply text without recording history, so it
		// is reloaded, which drops this document's undo history and leaves the
		// backend's grouped transaction in charge of undoing it.
		next := textAfter(cur, edits)
		d.buf.Reset(next)
		d.mapper = position.NewMapper(next)
		d.indexed = d.buf.Version()
	} else {
		d.applyByteChangesLocked(edits)
	}
	d.version = p.Version
	d.base = d.buf.Snapshot()
	d.baseBuf = d.base.Version()
	d.dirty = true
	d.signal()
	return nil
}

// textAfter returns t with changes applied in one pass, for reloading a
// buffer. Changes may come in any order; as in the buffer, each is clipped to
// start where the previous one ended.
func textAfter(t editor.Snapshot, changes []position.ByteChange) string {
	sorted := append([]position.ByteChange(nil), changes...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	var b strings.Builder
	b.Grow(t.Len())
	last := 0
	for _, c := range sorted {
		start := max(last, min(c.Start, t.Len()))
		end := max(start, min(c.End, t.Len()))
		b.WriteString(t.Slice(last, start))
		b.WriteString(c.Text)
		last = end
	}
	b.WriteString(t.Slice(last, t.Len()))
	return b.String()
}

// SetDirty records the backend's dirty flag.
func (d *Document) SetDirty(dirty bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dirty = dirty
}

// Undo takes back the buffer's last step and syncs the result to the backend.
func (d *Document) Undo(ctx context.Context) (bool, error) {
	return d.undoRedo(ctx, true)
}

// Redo reapplies the step Undo took back and syncs it.
func (d *Document) Redo(ctx context.Context) (bool, error) {
	return d.undoRedo(ctx, false)
}

func (d *Document) undoRedo(ctx context.Context, undo bool) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return false, ErrClosed
	}
	if d.info.ReadOnly {
		return false, ErrReadOnly
	}
	before := d.buf.Snapshot()
	var ok bool
	if undo {
		ok = d.buf.Undo()
	} else {
		ok = d.buf.Redo()
	}
	if !ok {
		return false, nil
	}
	after := d.buf.Snapshot()
	d.applyHunksToIndexLocked(snapText{before}, snapText{after}, diffTexts(snapText{before}, snapText{after}))
	d.indexed = after.Version()
	d.signal()
	return true, d.syncLocked(ctx)
}

// Save syncs pending edits and writes the document. A clean document's save is
// a no-op on the backend, as documents.save leaves an unmodified file alone.
func (d *Document) Save(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if err := d.syncLocked(ctx); err != nil {
		return err
	}
	if err := d.backend.DocSave(ctx, d.window, d.id); err != nil {
		return err
	}
	d.dirty = false
	// base is the text the backend kept, which is what the file holds now.
	d.saved = textHash(d.base.String())
	return nil
}

// ModifiedFromSaved reports whether the buffer differs from the file as of the
// last open or save. Undoing an edit back to the saved text makes it false
// again, however many buffer versions passed in between.
func (d *Document) ModifiedFromSaved() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return false
	}
	return textHash(d.buf.Snapshot().String()) != d.saved
}

// textHash hashes text for comparing a buffer with the saved file without
// holding both in memory.
func textHash(s string) uint64 {
	h := fnv.New64a()
	_, _ = io.WriteString(h, s)
	return h.Sum64()
}

// Sync sends buffer text changed outside ApplyEdits (a widget editing the
// shared buffer directly) to the backend. It is cheap when nothing changed.
func (d *Document) Sync(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	return d.syncLocked(ctx)
}

// Resync rereads the backend text and merges changes it made behind the
// buffer's back without sending anything. Use it for external file changes.
func (d *Document) Resync(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	return d.resyncLocked(ctx, 0)
}

// Resolve settles a ConflictError: ours keeps the buffer's local text and
// pushes it over the backend's; otherwise the buffer takes the backend text and
// the local edits are dropped, as the user chose.
func (d *Document) Resolve(ctx context.Context, ours bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	return d.resolveLocked(ctx, ours)
}

func (d *Document) resolveLocked(ctx context.Context, ours bool) error {
	if d.pending == nil {
		return d.resyncLocked(ctx, 0)
	}
	remote := d.pending.text
	version := d.pending.version
	if ours {
		local := d.buf.Snapshot()
		hunks := diffTexts(position.StringText(remote), snapText{local})
		cs := d.changesFor(position.StringText(remote), snapText{local}, hunks)
		v, err := d.backend.DocEdit(ctx, d.window, d.id, version, cs)
		if err != nil {
			if openidea.IsOutOfSync(err) {
				return d.resyncLocked(ctx, 1)
			}
			d.unsynced = true
			return err
		}
		d.version = v
		d.base = local
		d.baseBuf = local.Version()
		d.unsynced = false
		d.pending = nil
		d.signal()
		return nil
	}
	target := position.StringText(remote)
	cur := d.buf.Snapshot()
	hunks := diffTexts(snapText{cur}, target)
	d.applyByteChangesLocked(byteChanges(target, hunks))
	d.version = version
	d.base = d.buf.Snapshot()
	d.baseBuf = d.base.Version()
	d.unsynced = false
	d.pending = nil
	d.signal()
	return nil
}

// Reopen re-opens the document after a backend restart, which forgets ids and
// unsaved text. The buffer keeps the user's edits, which are pushed over the
// text the new backend read from disk, so nothing typed during the outage is
// lost and the document gets a fresh id.
func (d *Document) Reopen(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	info, err := d.backend.DocOpen(ctx, d.window, d.path)
	if err != nil {
		return err
	}
	disk := lf(info.Text)
	cur := d.buf.Snapshot()
	d.id = info.ID
	d.info = info
	d.version = info.Version
	if textEquals(cur, disk) {
		d.markSynced()
		return nil
	}
	hunks := diffTexts(position.StringText(disk), snapText{cur})
	d.base = scratchSnapshot(disk)
	d.baseBuf = 0
	d.unsynced = true
	cs := d.changesFor(position.StringText(disk), snapText{cur}, hunks)
	v, err := d.backend.DocEdit(ctx, d.window, d.id, d.version, cs)
	if err != nil {
		if openidea.IsOutOfSync(err) {
			return d.resyncLocked(ctx, 0)
		}
		return err
	}
	d.version = v
	d.markSynced()
	d.dirty = true
	d.signal()
	return nil
}

// Close releases the document in the backend. The buffer stays readable, so
// views that still show it do not see a half-closed document.
func (d *Document) Close(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	return d.backend.DocClose(ctx, d.window, d.id)
}

// pushLocked sends changes made against d.version, advances the base on
// success, recovers from a stale version, and remembers a failed push so the
// next sync sends the whole difference.
func (d *Document) pushLocked(ctx context.Context, cs position.ChangeSet) error {
	if len(cs.Changes) == 0 {
		d.markSynced()
		return nil
	}
	v, err := d.backend.DocEdit(ctx, d.window, d.id, d.version, cs)
	if err == nil {
		d.version = v
		d.markSynced()
		d.dirty = true
		return nil
	}
	if openidea.IsOutOfSync(err) {
		return d.resyncLocked(ctx, 0)
	}
	d.unsynced = true
	return err
}

func (d *Document) markSynced() {
	d.base = d.buf.Snapshot()
	d.baseBuf = d.base.Version()
	d.unsynced = false
	d.pending = nil
}

// syncLocked sends whatever the buffer has that the backend does not.
func (d *Document) syncLocked(ctx context.Context) error {
	if d.pending != nil {
		return d.conflictErrorLocked()
	}
	cur := d.buf.Snapshot()
	if !d.unsynced && cur.Version() == d.baseBuf {
		return nil
	}
	hunks := diffTexts(snapText{d.base}, snapText{cur})
	if d.indexed != cur.Version() {
		// The buffer was edited directly by a widget since the mapper last
		// moved; catch the line index up before converting the change set.
		d.applyHunksToIndexLocked(snapText{d.base}, snapText{cur}, hunks)
		d.indexed = cur.Version()
	}
	cs := d.changesFor(snapText{d.base}, snapText{cur}, hunks)
	if len(cs.Changes) == 0 {
		d.base = cur
		d.baseBuf = cur.Version()
		d.unsynced = false
		d.pending = nil
		return nil
	}
	return d.pushLocked(ctx, cs)
}

// resyncLocked reconciles the buffer with a fresh backend snapshot. Backend and
// local changes in separate places are merged; overlapping ones keep the local
// text and wait for Resolve, so unsaved work is never overwritten silently.
func (d *Document) resyncLocked(ctx context.Context, attempt int) error {
	if attempt >= 3 {
		return ErrResync
	}
	info, err := d.backend.DocSnapshot(ctx, d.window, d.id)
	if err != nil {
		d.unsynced = true
		return err
	}
	remote := lf(info.Text)
	local := d.buf.Snapshot()

	if textEquals(local, remote) {
		d.version = info.Version
		d.base = local
		d.baseBuf = local.Version()
		d.unsynced = false
		d.pending = nil
		return nil
	}

	baseText := snapText{d.base}
	remoteText := position.StringText(remote)
	localText := snapText{local}
	hunksRemote := diffTexts(baseText, remoteText)
	hunksLocal := diffTexts(baseText, localText)
	if conflicts(hunksRemote, hunksLocal) {
		d.pending = &pendingRemote{text: remote, version: info.Version, local: hunksLocal, remote: hunksRemote}
		d.unsynced = true
		return d.conflictErrorLocked()
	}
	if d.indexed != local.Version() {
		// The buffer was edited directly by a widget since the mapper last
		// moved; its byte offsets must describe the text the merged hunks
		// are applied to.
		d.applyHunksToIndexLocked(baseText, localText, hunksLocal)
		d.indexed = local.Version()
	}

	d.applyRemoteLocked(hunksLocal, hunksRemote, remote)
	d.version = info.Version
	d.base = scratchSnapshot(remote)
	d.baseBuf = 0 // the buffer now differs from the backend base by the local hunks
	merged := d.buf.Snapshot()
	mergedHunks := diffTexts(remoteText, snapText{merged})
	cs := d.changesFor(remoteText, snapText{merged}, mergedHunks)
	if len(cs.Changes) == 0 {
		d.markSynced()
		return nil
	}
	v, err := d.backend.DocEdit(ctx, d.window, d.id, d.version, cs)
	if err != nil {
		if openidea.IsOutOfSync(err) {
			return d.resyncLocked(ctx, attempt+1)
		}
		d.unsynced = true
		return err
	}
	d.version = v
	d.markSynced()
	d.dirty = true
	d.signal()
	return nil
}

// applyRemoteLocked applies the backend's hunks to the buffer, moved to where
// the local hunks left room: they do not overlap, so each backend hunk simply
// shifts by the local edits before it.
func (d *Document) applyRemoteLocked(hunksLocal, hunksRemote []Hunk, remote string) {
	if len(hunksRemote) == 0 {
		return
	}
	edits := make([]position.ByteChange, 0, len(hunksRemote))
	li, delta := 0, 0
	for _, rh := range hunksRemote {
		for li < len(hunksLocal) && hunksLocal[li].OldEnd <= rh.OldStart {
			delta += hunksLocal[li].newLen() - hunksLocal[li].oldLen()
			li++
		}
		edits = append(edits, position.ByteChange{
			Start: rh.OldStart + delta,
			End:   rh.OldEnd + delta,
			Text:  remote[rh.NewStart:rh.NewEnd],
		})
	}
	d.applyByteChangesLocked(edits)
}

// applyBufferEditsLocked applies ascending, non-overlapping edits as one undo
// step, from the last to the first so every edit's offsets stay valid, and
// keeps the mapper in step with each intermediate text.
func (d *Document) applyBufferEditsLocked(edits []editor.Edit) {
	d.buf.Transact(func() {
		for i := len(edits) - 1; i >= 0; i-- {
			e := edits[i]
			before := d.buf.Snapshot()
			d.buf.Edit(e)
			after := d.buf.Snapshot()
			if after.Version() == before.Version() {
				continue
			}
			d.mapper.ApplyEdit(snapText{before}, snapText{after}, e.Start, e.End, e.Start+len(e.Text))
		}
	})
	d.indexed = d.buf.Version()
}

// applyByteChangesLocked applies byte changes of the current text as one undo
// step, in reverse so that earlier coordinates stay valid.
func (d *Document) applyByteChangesLocked(changes []position.ByteChange) {
	if len(changes) == 0 {
		return
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Start < changes[j].Start })
	edits := make([]editor.Edit, len(changes))
	for i, c := range changes {
		edits[i] = editor.Edit{Start: c.Start, End: c.End, Text: c.Text}
	}
	d.applyBufferEditsLocked(edits)
}

// applyHunksToIndexLocked moves the mapper from before to after by applying the
// hunks in order. Between two hunks the text is the old one after the earlier
// hunks and the new one before the later ones, which concatText expresses
// without copying either text.
func (d *Document) applyHunksToIndexLocked(before, after textReader, hunks []Hunk) {
	beforeLen := before.Len()
	for _, h := range hunks {
		inter := concatText{a: after.Slice(0, h.NewStart), b: before.Slice(h.OldStart, beforeLen)}
		d.mapper.ApplyEdit(inter, after, h.NewStart, h.NewStart+h.oldLen(), h.NewEnd)
	}
}

// changesFor converts hunks between oldText and the current text into a change
// set in old text coordinates. The mapper describes the current text, and the
// two texts share the prefix of each hunk, so a running UTF-16 delta converts
// every hunk's offsets back.
func (d *Document) changesFor(oldText, newText textReader, hunks []Hunk) position.ChangeSet {
	changes := make([]position.TextChange, 0, len(hunks))
	delta := 0
	for _, h := range hunks {
		newUnits := d.mapper.ByteOffsetToUTF16Offset(newText, h.NewStart)
		deleted := oldText.Slice(h.OldStart, h.OldEnd)
		insert := newText.Slice(h.NewStart, h.NewEnd)
		deletedUnits := position.UTF16Length(deleted)
		from := newUnits - delta
		changes = append(changes, position.TextChange{
			From:   uint32(from),
			To:     uint32(from + deletedUnits),
			Insert: insert,
		})
		delta += position.UTF16Length(insert) - deletedUnits
	}
	return position.ChangeSet{Changes: changes}
}

// scratchSnapshot returns a snapshot of text without connecting it to the
// document's buffer.
func scratchSnapshot(text string) editor.Snapshot {
	return editor.NewBuffer(text).Snapshot()
}

// snapText adapts an editor snapshot to the read side of a text.
type snapText struct{ s editor.Snapshot }

func (t snapText) Len() int                    { return t.s.Len() }
func (t snapText) Slice(start, end int) string { return t.s.Slice(start, end) }

// textEquals reports whether t and s are the same bytes, without copying t.
func textEquals(t editor.Snapshot, s string) bool {
	if t.Len() != len(s) {
		return false
	}
	for i := 0; i < len(s); i += diffChunk {
		end := min(i+diffChunk, len(s))
		if t.Slice(i, end) != s[i:end] {
			return false
		}
	}
	return true
}

// byteChanges turns hunks into replacements of the old text's bytes, taking
// the new bytes from newText.
func byteChanges(newText textReader, hunks []Hunk) []position.ByteChange {
	out := make([]position.ByteChange, 0, len(hunks))
	for _, h := range hunks {
		out = append(out, position.ByteChange{
			Start: h.OldStart,
			End:   h.OldEnd,
			Text:  newText.Slice(h.NewStart, h.NewEnd),
		})
	}
	return out
}

// normalizeEdits sorts, clips and drops the no-ops of a batch the way the
// editor buffer does, so the change set describes what the buffer will hold.
func normalizeEdits(edits []editor.Edit, text editor.Snapshot) []editor.Edit {
	if len(edits) == 0 {
		return nil
	}
	out := make([]editor.Edit, len(edits))
	for i, e := range edits {
		out[i] = editor.Edit{Start: e.Start, End: e.End, Text: lf(e.Text)}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	n := 0
	last := 0
	for _, e := range out {
		start := max(last, min(e.Start, text.Len()))
		end := max(start, min(e.End, text.Len()))
		last = end
		if start == end && e.Text == "" || end-start == len(e.Text) && text.Slice(start, end) == e.Text {
			continue
		}
		out[n] = editor.Edit{Start: start, End: end, Text: e.Text}
		n++
	}
	return out[:n]
}
