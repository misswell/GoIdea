package docbridge

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"goidea/internal/openidea"
	"goidea/internal/position"
)

type fakeDoc struct {
	id      uint32
	path    string
	text    string
	version uint32
	mapper  *position.Mapper
	dirty   bool
	readOn  bool
}

func newFakeDoc(id uint32, path, text string) *fakeDoc {
	return &fakeDoc{id: id, path: path, text: lf(text), version: 1, mapper: position.NewMapper(lf(text))}
}

func (d *fakeDoc) setText(s string) {
	d.text = s
	d.mapper = position.NewMapper(s)
}

func (d *fakeDoc) apply(cs position.ChangeSet) error {
	total := position.UTF16Length(d.text)
	prevTo := uint32(0)
	type byteEdit struct {
		start, end int
		text       string
	}
	edits := make([]byteEdit, 0, len(cs.Changes))
	for i, c := range cs.Changes {
		if c.From > c.To || c.To > uint32(total) {
			return fmt.Errorf("change %d out of range", i)
		}
		if c.From < prevTo {
			return fmt.Errorf("change %d overlaps the previous change", i)
		}
		prevTo = c.To
		start := d.mapper.UTF16OffsetToByteOffset(position.StringText(d.text), int(c.From))
		end := d.mapper.UTF16OffsetToByteOffset(position.StringText(d.text), int(c.To))
		edits = append(edits, byteEdit{start: start, end: end, text: lf(c.Insert)})
	}
	var b strings.Builder
	last := 0
	for _, e := range edits {
		b.WriteString(d.text[last:e.start])
		b.WriteString(e.text)
		last = e.end
	}
	b.WriteString(d.text[last:])
	d.setText(b.String())
	d.version++
	d.dirty = true
	return nil
}

type fakeBackend struct {
	mu       sync.Mutex
	docs     map[uint32]*fakeDoc
	byPath   map[string]uint32
	nextID   uint32
	failNext int
	disk     map[string]string
	opens    int
	closes   int
	saves    int
	lastSet  position.ChangeSet
	lastBase uint32
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		docs:   make(map[uint32]*fakeDoc),
		byPath: make(map[string]uint32),
		disk:   make(map[string]string),
	}
}

func (b *fakeBackend) add(path, text string) *fakeDoc {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	d := newFakeDoc(b.nextID, path, text)
	b.docs[d.id] = d
	b.byPath[path] = d.id
	b.disk[path] = lf(text)
	return d
}

func (b *fakeBackend) doc(id uint32) *fakeDoc {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.docs[id]
}

func (b *fakeBackend) text(id uint32) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.docs[id].text
}

func (b *fakeBackend) DocOpen(_ context.Context, _, path string) (openidea.DocInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.opens++
	id, ok := b.byPath[path]
	if !ok {
		b.nextID++
		d := newFakeDoc(b.nextID, path, b.disk[path])
		b.docs[d.id] = d
		b.byPath[path] = d.id
		id = d.id
	}
	return b.infoLocked(b.docs[id]), nil
}

func (b *fakeBackend) DocEdit(_ context.Context, _ string, doc uint32, baseVersion uint32, changes position.ChangeSet) (uint32, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := b.docs[doc]
	if d == nil {
		return 0, &openidea.Error{Kind: "notFound", Message: "no such document"}
	}
	if b.failNext > 0 {
		b.failNext--
		return 0, fmt.Errorf("fake backend: edit failed")
	}
	if baseVersion != d.version {
		return 0, &openidea.Error{Kind: "outOfSync", Message: fmt.Sprintf("at %d, edit for %d", d.version, baseVersion)}
	}
	b.lastSet = changes
	b.lastBase = baseVersion
	if err := d.apply(changes); err != nil {
		return 0, &openidea.Error{Kind: "invalid", Message: err.Error()}
	}
	return d.version, nil
}

func (b *fakeBackend) DocSnapshot(_ context.Context, _ string, doc uint32) (openidea.DocInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := b.docs[doc]
	if d == nil {
		return openidea.DocInfo{}, &openidea.Error{Kind: "notFound", Message: "no such document"}
	}
	return b.infoLocked(d), nil
}

func (b *fakeBackend) DocSave(_ context.Context, _ string, doc uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := b.docs[doc]
	if d == nil {
		return &openidea.Error{Kind: "notFound", Message: "no such document"}
	}
	b.saves++
	b.disk[d.path] = d.text
	d.dirty = false
	return nil
}

func (b *fakeBackend) DocClose(_ context.Context, _ string, doc uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closes++
	if d := b.docs[doc]; d != nil {
		delete(b.docs, doc)
		delete(b.byPath, d.path)
	}
	return nil
}

func (b *fakeBackend) infoLocked(d *fakeDoc) openidea.DocInfo {
	return openidea.DocInfo{
		ID:         d.id,
		Path:       d.path,
		Text:       d.text,
		Version:    d.version,
		Language:   "java",
		ReadOnly:   d.readOn,
		LineEnding: "lf",
	}
}

// edit replaces the UTF-16 range [from, to) of a document in the backend, as a
// language-server edit would, and returns the patch an event would carry.
func (b *fakeBackend) edit(id uint32, from, to int, insert string) Patch {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := b.docs[id]
	base := d.version
	t := position.StringText(d.text)
	start := d.mapper.UTF16OffsetToByteOffset(t, from)
	end := d.mapper.UTF16OffsetToByteOffset(t, to)
	d.setText(d.text[:start] + lf(insert) + d.text[end:])
	d.version++
	d.dirty = true
	return Patch{
		BaseVersion: base,
		Version:     d.version,
		Changes:     []position.TextChange{{From: uint32(from), To: uint32(to), Insert: insert}},
	}
}

// restart forgets in-memory documents, as a backend crash does; documents come
// back from the last saved text when opened.
func (b *fakeBackend) restart() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.docs = make(map[uint32]*fakeDoc)
	b.byPath = make(map[string]uint32)
}

const sampleJava = "package demo;\n\nclass 世界 {\n\tString s = \"😀\";\n}\n"

func openSample(t *testing.T) (*fakeBackend, *Document, uint32) {
	t.Helper()
	b := newFakeBackend()
	d := b.add("/tmp/demo/World.java", sampleJava)
	doc, err := Open(context.Background(), b, "w1", d.path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return b, doc, d.id
}

func requireMapper(t *testing.T, doc *Document) {
	t.Helper()
	text := doc.Text()
	want := position.NewMapper(text)
	got := doc.Mapper()
	if got.LineCount() != want.LineCount() {
		t.Fatalf("mapper lines: got %d, want %d", got.LineCount(), want.LineCount())
	}
	for off := 0; off <= len(text); off++ {
		if got.ByteOffsetToUTF16Offset(position.StringText(text), off) != want.ByteOffsetToUTF16Offset(position.StringText(text), off) {
			t.Fatalf("mapper at byte %d differs from a fresh mapper", off)
		}
	}
}

func TestApplyEditsSyncsCJKAndEmoji(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	text := doc.Text()
	at := strings.Index(text, "\tString")
	if at < 0 {
		t.Fatal("fixture")
	}
	if err := doc.ApplyEdits(ctx, editorEdit(at, at, "int 数字 = 1;\n")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := text[:at] + "int 数字 = 1;\n" + text[at:]
	if got := doc.Text(); got != want {
		t.Fatalf("buffer:\n%q\nwant\n%q", got, want)
	}
	if got := b.text(id); got != want {
		t.Fatalf("backend:\n%q\nwant\n%q", got, want)
	}
	if doc.Version() != 2 {
		t.Fatalf("version %d, want 2", doc.Version())
	}
	if len(b.lastSet.Changes) != 1 {
		t.Fatalf("changes %d, want 1", len(b.lastSet.Changes))
	}
	wantFrom := position.UTF16Length(text[:at])
	if got := int(b.lastSet.Changes[0].From); got != wantFrom {
		t.Fatalf("change from %d, want %d (UTF-16)", got, wantFrom)
	}
	requireMapper(t, doc)
}

func TestMultiEditOneUndoStep(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	text := doc.Text()
	head := strings.Index(text, "class")
	tail := strings.Index(text, "String")
	err := doc.ApplyEdits(ctx,
		editorEdit(head, head, "// header\n"),
		editorEdit(tail, tail, "public "),
	)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	edited := doc.Text()
	if b.text(id) != edited {
		t.Fatal("backend and buffer differ after the batch")
	}
	requireMapper(t, doc)

	if ok, err := doc.Undo(ctx); !ok || err != nil {
		t.Fatalf("undo: %v %v", ok, err)
	}
	if got := doc.Text(); got != text {
		t.Fatalf("undo text:\n%q\nwant\n%q", got, text)
	}
	if got := b.text(id); got != text {
		t.Fatalf("backend after undo:\n%q\nwant\n%q", got, text)
	}
	requireMapper(t, doc)

	if ok, err := doc.Redo(ctx); !ok || err != nil {
		t.Fatalf("redo: %v %v", ok, err)
	}
	if got := doc.Text(); got != edited {
		t.Fatalf("redo text differs")
	}
	if got := b.text(id); got != edited {
		t.Fatalf("backend after redo differs")
	}
	requireMapper(t, doc)
}

// Server text edits carry UTF-16 columns; applying them through ByteEdits must
// land on the right bytes even after text the editor and protocol count
// differently (an astral emoji is two UTF-16 units, four bytes).
func TestServerTextEditsConvertFromUTF16(t *testing.T) {
	_, doc, id := openSample(t)
	ctx := context.Background()
	text := doc.Text()
	// Line 3 is "\tString s = "😀";".
	edits := []openidea.TextEdit{
		{Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 3, Col: 8},
			End:   openidea.OutlinePos{Line: 3, Col: 9},
		}, NewText: "name"},
		{Range: openidea.OutlineRange{
			Start: openidea.OutlinePos{Line: 3, Col: 15},
			End:   openidea.OutlinePos{Line: 3, Col: 15},
		}, NewText: "!"},
	}
	be := doc.ByteEdits(edits)
	if len(be) != 2 {
		t.Fatalf("byte edits %d, want 2", len(be))
	}
	if err := doc.ApplyEdits(ctx, be...); err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := strings.Replace(text, `String s = "😀";`, `String name = "😀!";`, 1)
	if got := doc.Text(); got != want {
		t.Fatalf("buffer:\n%q\nwant\n%q", got, want)
	}
	if ok, err := doc.Undo(ctx); !ok || err != nil {
		t.Fatalf("undo: %v %v", ok, err)
	}
	if got := doc.Text(); got != text {
		t.Fatalf("undo text:\n%q\nwant\n%q", got, text)
	}
	requireMapper(t, doc)
	_ = id
}

func TestPatchAppliesAndUndoes(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	text := doc.Text()
	at := strings.Index(text, "世界")
	from := position.UTF16Length(text[:at])
	p := b.edit(id, from, from+2, "地球")
	if err := doc.ApplyPatch(ctx, p); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if !strings.Contains(doc.Text(), "地球") {
		t.Fatalf("buffer did not take the patch: %q", doc.Text())
	}
	if doc.Version() != p.Version {
		t.Fatalf("version %d, want %d", doc.Version(), p.Version)
	}
	requireMapper(t, doc)
	if ok, err := doc.Undo(ctx); !ok || err != nil {
		t.Fatalf("undo patch: %v %v", ok, err)
	}
	if !strings.Contains(doc.Text(), "世界") || !strings.Contains(b.text(id), "世界") {
		t.Fatal("undo did not revert the patch on both sides")
	}
}

func TestWorkspaceEditIsNotAFileLocalUndoStep(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	if err := doc.ApplyEdits(ctx, editorEdit(0, 0, "// typed\n")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	typed := doc.Text()

	// A grouped backend edit, as a rename or a refactor emits.
	p := b.edit(id, 0, 0, "// refactor\n")
	p.WorkspaceEdit = true
	if err := doc.ApplyPatch(ctx, p); err != nil {
		t.Fatalf("workspace patch: %v", err)
	}
	want := "// refactor\n" + typed
	if got := doc.Text(); got != want {
		t.Fatalf("buffer:\n%q\nwant\n%q", got, want)
	}
	if got := b.text(id); got != want {
		t.Fatalf("backend:\n%q\nwant\n%q", got, want)
	}
	if doc.Version() != p.Version {
		t.Fatalf("version %d, want %d", doc.Version(), p.Version)
	}
	requireMapper(t, doc)
	if doc.Buffer().CanUndo() {
		t.Fatal("the workspace edit entered the buffer's undo history")
	}
	if ok, err := doc.Undo(ctx); ok || err != nil {
		t.Fatalf("undo after a workspace edit: %v %v", ok, err)
	}
	if doc.Text() != want || b.text(id) != want {
		t.Fatal("undo changed a workspace edit")
	}
}

func TestPatchWaitsForUnsyncedBufferEdits(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	// Typing the sync pump has not sent yet: the patch's offsets describe the
	// base text, not the buffer, so it must merge through a resync.
	doc.Buffer().Edit(editorEdit(0, 0, "// typed\n"))
	p := b.edit(id, position.UTF16Length(sampleJava), position.UTF16Length(sampleJava), "// end\n")
	if err := doc.ApplyPatch(ctx, p); err != nil {
		t.Fatalf("patch over unsynced edits: %v", err)
	}
	want := "// typed\n" + sampleJava + "// end\n"
	if got := doc.Text(); got != want {
		t.Fatalf("buffer:\n%q\nwant\n%q", got, want)
	}
	if got := b.text(id); got != want {
		t.Fatalf("backend:\n%q\nwant\n%q", got, want)
	}
	requireMapper(t, doc)
}

func TestOutOfSyncMergesSeparateChanges(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	text := doc.Text()

	// The backend reformats the package line while the user types later.
	at := strings.Index(text, "package")
	_ = b.edit(id, position.UTF16Length(text[:at]), position.UTF16Length(text[:at+7]), "package ")
	// The edit at the tail is made against the stale version and triggers a
	// resync, which must keep both changes.
	tail := strings.Index(text, "}")
	err := doc.ApplyEdits(ctx, editorEdit(tail, tail, "// tail\n"))
	if err != nil {
		t.Fatalf("apply after concurrent backend change: %v", err)
	}
	got := doc.Text()
	if !strings.Contains(got, "package  demo;") {
		t.Fatalf("backend change lost: %q", got)
	}
	if !strings.Contains(got, "// tail\n}") {
		t.Fatalf("local change lost: %q", got)
	}
	if b.text(id) != got {
		t.Fatalf("backend and buffer differ after merge:\nbackend %q\nbuffer  %q", b.text(id), got)
	}
	if doc.Version() != 3 {
		t.Fatalf("version %d, want 3", doc.Version())
	}
	requireMapper(t, doc)
}

func TestConflictKeepsLocalAndResolves(t *testing.T) {
	for _, ours := range []bool{true, false} {
		t.Run(fmt.Sprintf("ours=%v", ours), func(t *testing.T) {
			b, doc, id := openSample(t)
			ctx := context.Background()
			text := doc.Text()
			at := strings.Index(text, "class")
			from := position.UTF16Length(text[:at])
			remote := b.edit(id, from, from+5, "record")
			err := doc.ApplyEdits(ctx, editorEdit(at, at+5, "enum  "))
			if err == nil {
				t.Fatal("expected a conflict")
			}
			var ce *ConflictError
			if !asConflict(err, &ce) {
				t.Fatalf("error %v is not a ConflictError", err)
			}
			if !strings.HasPrefix(doc.Text(), "package demo;\n\nenum  ") {
				t.Fatalf("local text lost: %q", doc.Text())
			}
			if err := doc.Resolve(ctx, ours); err != nil {
				t.Fatalf("resolve: %v", err)
			}
			got := doc.Text()
			if ours {
				if !strings.Contains(got, "enum  ") || b.text(id) != got {
					t.Fatalf("ours not pushed: %q vs %q", got, b.text(id))
				}
			} else {
				if got != remoteText(remote, text) {
					t.Fatalf("theirs not taken: %q", got)
				}
				if b.text(id) != got {
					t.Fatalf("backend differs from resolved buffer")
				}
			}
			requireMapper(t, doc)
		})
	}
}

func remoteText(p Patch, base string) string {
	c := p.Changes[0]
	from := byteForUTF16(base, int(c.From))
	to := byteForUTF16(base, int(c.To))
	return base[:from] + c.Insert + base[to:]
}

func byteForUTF16(s string, u int) int {
	return position.NewMapper(s).UTF16OffsetToByteOffset(position.StringText(s), u)
}

func asConflict(err error, out **ConflictError) bool {
	ce, ok := err.(*ConflictError)
	if ok {
		*out = ce
	}
	return ok
}

func TestFailedPushSendsWholeDifferenceLater(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	text := doc.Text()
	b.mu.Lock()
	b.failNext = 1
	b.mu.Unlock()
	tail := strings.Index(text, "}")
	if err := doc.ApplyEdits(ctx, editorEdit(tail, tail, "// one\n")); err == nil {
		t.Fatal("expected the failing edit to return an error")
	}
	if b.text(id) != text {
		t.Fatal("backend changed although the edit failed")
	}
	if err := doc.ApplyEdits(ctx, editorEdit(tail, tail, "// two\n")); err != nil {
		t.Fatalf("second edit: %v", err)
	}
	if b.text(id) != doc.Text() {
		t.Fatalf("backend %q, buffer %q", b.text(id), doc.Text())
	}
	// Both edits target the same offset, so their order depends on the buffer;
	// what matters is that neither is lost and the backend matches.
	if got := doc.Text(); !strings.Contains(got, "// one\n") || !strings.Contains(got, "// two\n") {
		t.Fatalf("both local edits not kept: %q", got)
	}
	requireMapper(t, doc)
}

func TestSyncSendsDirectBufferEdits(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	text := doc.Text()
	at := strings.Index(text, "\tString")

	// A widget edits the shared buffer itself, as the MyGo editor does, then
	// asks the bridge to push the difference.
	doc.Buffer().Edit(editor.Edit{Start: at, End: at, Text: "\tint n = 0;\n"})
	want := text[:at] + "\tint n = 0;\n" + text[at:]
	if b.text(id) != text {
		t.Fatal("backend changed before Sync")
	}
	if err := doc.Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := b.text(id); got != want {
		t.Fatalf("backend:\n%q\nwant\n%q", got, want)
	}
	if !doc.Dirty() {
		t.Fatal("document not marked dirty after a synced edit")
	}
	requireMapper(t, doc)

	// A second sync with no new edits sends nothing.
	before := b.lastBase
	if err := doc.Sync(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if b.lastBase != before {
		t.Fatal("a clean sync sent a change set")
	}

	// Undo through the editor's buffer is also pushed.
	if !doc.Buffer().Undo() {
		t.Fatal("buffer undo failed")
	}
	if err := doc.Sync(ctx); err != nil {
		t.Fatalf("sync after undo: %v", err)
	}
	if got := b.text(id); got != text {
		t.Fatalf("undo not synced:\n%q\nwant\n%q", got, text)
	}
	requireMapper(t, doc)
}

func TestSaveSyncsThenWrites(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	text := doc.Text()
	b.mu.Lock()
	b.failNext = 1
	b.mu.Unlock()
	if err := doc.ApplyEdits(ctx, editorEdit(0, 0, "// saved first\n")); err == nil {
		t.Fatal("expected the edit to fail")
	}
	if err := doc.Save(ctx); err != nil {
		t.Fatalf("save: %v", err)
	}
	want := "// saved first\n" + text
	if b.disk["/tmp/demo/World.java"] != want {
		t.Fatalf("disk %q, want %q", b.disk["/tmp/demo/World.java"], want)
	}
	if b.text(id) != want || doc.Text() != want {
		t.Fatal("buffer, backend and disk differ after save")
	}
	if doc.Dirty() {
		t.Fatal("document still dirty after save")
	}
	if b.saves != 1 {
		t.Fatalf("saves %d, want 1", b.saves)
	}
	requireMapper(t, doc)
}

func TestReopenKeepsUnsavedEdits(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	if err := doc.ApplyEdits(ctx, editorEdit(0, 0, "// unsaved\n")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := doc.Text()
	b.restart()
	if err := doc.Reopen(ctx); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if doc.Text() != want {
		t.Fatal("reopen changed the buffer")
	}
	if doc.ID() == id {
		t.Fatal("reopen kept the old document id")
	}
	if got := b.text(doc.ID()); got != want {
		t.Fatalf("reopened backend %q, want %q", got, want)
	}
	requireMapper(t, doc)
}

func TestConcurrentEditsStayConsistent(t *testing.T) {
	b, doc, id := openSample(t)
	ctx := context.Background()
	const n = 16
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			line := fmt.Sprintf("// %d\n", i)
			if err := doc.ApplyEdits(ctx, editorEdit(0, 0, line)); err != nil {
				t.Errorf("edit %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if doc.Text() != b.text(id) {
		t.Fatal("backend and buffer differ after concurrent edits")
	}
	if doc.Version() != 1+n {
		t.Fatalf("version %d, want %d", doc.Version(), 1+n)
	}
	requireMapper(t, doc)
}

func TestReadOnlyRejectsEdits(t *testing.T) {
	b := newFakeBackend()
	d := b.add("/tmp/demo/Locked.java", "class Locked {}\n")
	d.readOn = true
	doc, err := Open(context.Background(), b, "w1", d.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.ApplyEdits(context.Background(), editorEdit(0, 0, "x")); err != ErrReadOnly {
		t.Fatalf("error %v, want ErrReadOnly", err)
	}
}

func editorEdit(start, end int, text string) editor.Edit {
	return editor.Edit{Start: start, End: end, Text: text}
}

func TestModifiedFromSavedFollowsTextNotVersions(t *testing.T) {
	_, doc, _ := openSample(t)
	ctx := context.Background()
	if doc.ModifiedFromSaved() {
		t.Fatal("fresh document reports modified")
	}
	at := strings.Index(doc.Text(), "class")
	if err := doc.ApplyEdits(ctx, editorEdit(at, at, "// edited\n")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !doc.ModifiedFromSaved() {
		t.Fatal("edit not reported as modified")
	}
	if err := doc.Save(ctx); err != nil {
		t.Fatalf("save: %v", err)
	}
	if doc.ModifiedFromSaved() {
		t.Fatal("saved document reports modified")
	}
	// A direct buffer edit pushed by Sync, then undone: the text is back to
	// the saved one, so the prompt must not say there are changes.
	doc.Buffer().Edit(editorEdit(at, at, "// again\n"))
	if err := doc.Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !doc.ModifiedFromSaved() {
		t.Fatal("direct edit not reported as modified")
	}
	if !doc.Buffer().Undo() {
		t.Fatal("undo did nothing")
	}
	if doc.ModifiedFromSaved() {
		t.Fatal("undo back to the saved text still reports modified")
	}
	// The pump pushes the undone text as it would after typing.
	if err := doc.Sync(ctx); err != nil {
		t.Fatalf("sync after undo: %v", err)
	}
	requireMapper(t, doc)
}

func TestProtocolCoordinatesOnASText(t *testing.T) {
	_, doc, _ := openSample(t)
	text := doc.Text()

	// "class 世界 {": the CJK name is six bytes but one UTF-16 unit per
	// character, the case a byte column gets wrong.
	at := strings.Index(text, "世界")
	if p := doc.UTF16Pos(at); p != (openidea.OutlinePos{Line: 2, Col: 6}) {
		t.Fatalf("start of name: %+v, want line 2 col 6", p)
	}
	if p := doc.UTF16Pos(at + len("世界")); p != (openidea.OutlinePos{Line: 2, Col: 8}) {
		t.Fatalf("end of name: %+v, want line 2 col 8", p)
	}
	start, end := doc.ByteRange(openidea.OutlineRange{
		Start: openidea.OutlinePos{Line: 2, Col: 6},
		End:   openidea.OutlinePos{Line: 2, Col: 8},
	})
	if text[start:end] != "世界" {
		t.Fatalf("range maps to %q, want 世界", text[start:end])
	}

	// The emoji is four bytes and two UTF-16 units: a surrogate pair.
	emoji := strings.Index(text, "😀")
	if p := doc.UTF16Pos(emoji); p != (openidea.OutlinePos{Line: 3, Col: 13}) {
		t.Fatalf("start of emoji: %+v, want line 3 col 13", p)
	}
	if p := doc.UTF16Pos(emoji + len("😀")); p != (openidea.OutlinePos{Line: 3, Col: 15}) {
		t.Fatalf("end of emoji: %+v, want line 3 col 15", p)
	}
	if off := doc.ByteOffset(openidea.OutlinePos{Line: 3, Col: 15}); off != emoji+len("😀") {
		t.Fatalf("col 15 of line 3 is byte %d, want %d", off, emoji+len("😀"))
	}
	lineStart := strings.LastIndex(text[:emoji], "\n") + 1
	if line, col := doc.BytePos(3, 15); line != 3 || col != emoji+len("😀")-lineStart {
		t.Fatalf("byte position of 3:15 is %d:%d, want 3:%d", line, col, emoji+len("😀")-lineStart)
	}
}

func TestCoordinatesCatchUpAfterWidgetEdit(t *testing.T) {
	_, doc, _ := openSample(t)
	// A widget edits the shared buffer directly; the mapper only learns of it
	// on the next look, which these conversions must trigger themselves.
	doc.Buffer().Edit(editor.Edit{Start: 0, End: 0, Text: "// 世界\n"})
	text := doc.Text()
	at := strings.LastIndex(text, "世界")
	if p := doc.UTF16Pos(at); p != (openidea.OutlinePos{Line: 3, Col: 6}) {
		t.Fatalf("name after widget edit: %+v, want line 3 col 6", p)
	}
	if off := doc.ByteOffset(openidea.OutlinePos{Line: 3, Col: 8}); off != at+len("世界") {
		t.Fatalf("col 8 of line 3 is byte %d, want %d", off, at+len("世界"))
	}
	// Conversions must not have edited anything.
	if doc.Text() != text {
		t.Fatal("coordinate conversion changed the buffer")
	}
	requireMapper(t, doc)
}
