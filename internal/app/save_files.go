package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui/editor"
)

// saveText is the text a save writes after Editor › General › Save Files'
// cleanups, as IntelliJ applies them to the document before writing it. disk
// is the text the file held at the last open or save: only lines that differ
// from it count as modified.
func saveText(text, disk string, p SavePrefs) string {
	if p.TrimTrailing {
		lines := strings.Split(text, "\n")
		diskLines := strings.Split(disk, "\n")
		for i, line := range lines {
			// A line too short for the disk's is new or cut, hence modified.
			if !p.TrimAllLines && i < len(diskLines) && diskLines[i] == line {
				continue
			}
			lines[i] = strings.TrimRight(line, " \t")
		}
		text = strings.Join(lines, "\n")
	}
	if p.DropTrailingBlankLines {
		text = trimTrailingBlankLines(text)
	}
	if p.EnsureFinalNewline && text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text
}

// trimTrailingBlankLines drops the whitespace-only lines at the end of text
// and leaves one line break there, as Remove trailing blank lines does.
func trimTrailingBlankLines(text string) string {
	cut := len(text)
	for cut > 0 {
		start := strings.LastIndexByte(text[:cut], '\n') + 1
		if strings.TrimRight(text[start:cut], " \t") != "" {
			break
		}
		if start == 0 {
			return ""
		}
		cut = start - 1
	}
	if cut == len(text) {
		return text
	}
	return text[:cut] + "\n"
}

// saveEdits is the one edit that turns text into what the save writes: the
// bytes saveText changed, from the first to the last, so the editor's undo
// takes the cleanup back in one step.
func saveEdits(text, disk string, p SavePrefs) []editor.Edit {
	want := saveText(text, disk, p)
	if want == text {
		return nil
	}
	pre := 0
	for pre < len(text) && pre < len(want) && text[pre] == want[pre] {
		pre++
	}
	suf := 0
	for suf < len(text)-pre && suf < len(want)-pre && text[len(text)-1-suf] == want[len(want)-1-suf] {
		suf++
	}
	return []editor.Edit{{Start: pre, End: len(text) - suf, Text: want[pre : len(want)-suf]}}
}

// cleanForSave applies Save Files' cleanups to a document before it is saved;
// a document with nothing to clean is left alone, unsaved text included.
func cleanForSave(ctx context.Context, doc interface {
	Buffer() *editor.Buffer
	SavedText() string
	ApplyEdits(context.Context, ...editor.Edit) error
}, p SavePrefs) error {
	if p.empty() {
		return nil
	}
	edits := saveEdits(doc.Buffer().String(), doc.SavedText(), p)
	if len(edits) == 0 {
		return nil
	}
	return doc.ApplyEdits(ctx, edits...)
}
