package app

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

const (
	navigationHistoryLimit = 100
	navigationMergeLines   = 4
	editHistoryLimit       = 60
	editMergeLines         = 3
)

// navigationLocation is one place in Back / Forward or Last / Next Edit
// Location. Columns are UTF-16 so the history agrees with OpenIDEA even when
// the line contains non-BMP characters.
type navigationLocation struct {
	path string
	pos  editor.Pos
	at   time.Time
}

func sameNavigationPlace(left, right navigationLocation) bool {
	return left.path == right.path &&
		lineDistance(left.pos.Line, right.pos.Line) < navigationMergeLines
}

func lineDistance(left, right int) int {
	if left < right {
		return right - left
	}
	return left - right
}

// navigationHistory is OpenIDEA's NavigationHistory: a normal caret move
// updates the current place, while a navigation to another line or file
// leaves the old place on the back stack and clears the forward stack.
type navigationHistory struct {
	entries []navigationLocation
	index   int
}

func (h *navigationHistory) update(place navigationLocation, navigation bool) {
	if len(h.entries) == 0 {
		h.entries = []navigationLocation{place}
		h.index = 0
		return
	}
	current := h.entries[h.index]
	switched := current.path != place.path
	if !switched && (!navigation || current.pos.Line == place.pos.Line) {
		h.entries[h.index] = place
		return
	}
	h.entries = h.entries[:h.index+1]
	if h.index > 0 && sameNavigationPlace(h.entries[h.index-1], current) {
		h.entries = slices.Delete(h.entries, h.index-1, h.index)
	}
	h.entries = append(h.entries, place)
	if len(h.entries) > navigationHistoryLimit {
		h.entries = slices.Delete(h.entries, 0, len(h.entries)-navigationHistoryLimit)
	}
	h.index = len(h.entries) - 1
}

func (h *navigationHistory) canGoBack() bool {
	return h.index > 0 && h.index < len(h.entries)
}

func (h *navigationHistory) canGoForward() bool {
	return h.index >= 0 && h.index < len(h.entries)-1
}

func (h *navigationHistory) back() (navigationLocation, bool) {
	if !h.canGoBack() {
		return navigationLocation{}, false
	}
	h.index--
	return h.entries[h.index], true
}

func (h *navigationHistory) forward() (navigationLocation, bool) {
	if !h.canGoForward() {
		return navigationLocation{}, false
	}
	h.index++
	return h.entries[h.index], true
}

func (h *navigationHistory) removePath(path string) {
	h.removeTree(path, false)
}

func (h *navigationHistory) removeTree(path string, includeChildren bool) {
	if len(h.entries) == 0 {
		return
	}
	current := h.entries[h.index]
	h.entries = slices.DeleteFunc(h.entries, func(place navigationLocation) bool {
		return navigationPathUnder(place.path, path, includeChildren)
	})
	if len(h.entries) == 0 {
		h.index = 0
		return
	}
	if current.path != path {
		for i, place := range h.entries {
			if place == current {
				h.index = i
				return
			}
		}
	}
	h.index = len(h.entries) - 1
}

// editHistory is OpenIDEA's EditHistory. Repeated Last Edit Location calls
// walk older entries, and Next Edit Location walks back toward the newest.
type editHistory struct {
	entries []navigationLocation
	index   int
}

func (h *editHistory) record(place navigationLocation) {
	last := len(h.entries) - 1
	if last >= 0 && h.entries[last].path == place.path &&
		lineDistance(h.entries[last].pos.Line, place.pos.Line) <= editMergeLines {
		h.entries[last] = place
		h.index = len(h.entries)
		return
	}
	h.entries = slices.DeleteFunc(h.entries, func(entry navigationLocation) bool {
		return entry.path == place.path && lineDistance(entry.pos.Line, place.pos.Line) <= editMergeLines
	})
	h.entries = append(h.entries, place)
	if len(h.entries) > editHistoryLimit {
		h.entries = slices.Delete(h.entries, 0, len(h.entries)-editHistoryLimit)
	}
	h.index = len(h.entries)
}

func (h *editHistory) back(caret *navigationLocation) (navigationLocation, bool) {
	i := min(h.index, len(h.entries)) - 1
	if i >= 0 && h.index == len(h.entries) && caret != nil &&
		h.entries[i].path == caret.path && h.entries[i].pos.Line == caret.pos.Line {
		i--
	}
	if i < 0 {
		return navigationLocation{}, false
	}
	h.index = i
	return h.entries[i], true
}

func (h *editHistory) forward() (navigationLocation, bool) {
	if h.index >= len(h.entries)-1 {
		return navigationLocation{}, false
	}
	h.index++
	return h.entries[h.index], true
}

func (h *editHistory) removePath(path string) {
	h.removeTree(path, false)
}

func (h *editHistory) removeTree(path string, includeChildren bool) {
	before := len(h.entries)
	h.entries = slices.DeleteFunc(h.entries, func(place navigationLocation) bool {
		return navigationPathUnder(place.path, path, includeChildren)
	})
	if len(h.entries) != before {
		h.index = len(h.entries)
	}
}

func navigationPathUnder(path, root string, includeChildren bool) bool {
	if path == root {
		return true
	}
	return includeChildren && strings.HasPrefix(path, root+string(filepath.Separator))
}
