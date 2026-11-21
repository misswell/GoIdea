package app

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

func TestRerunUsagesRepeatsTheOriginalSymbolAndRefreshesResults(t *testing.T) {
	const text = "class A { int value; void run() { value++; value++; } }"
	offset := strings.Index(text, "value") + 1
	m := gotoLineTestModel(text, offset)
	m.ctx = context.Background()

	type request struct {
		docID uint32
		pos   map[string]any
	}
	var (
		mu       sync.Mutex
		requests []request
	)
	done := make(chan struct{}, 2)
	m.postHook = func(fn func()) {
		fn()
		done <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		switch command {
		case "cancel_request":
			return json.RawMessage(`null`), nil
		case "lsp_references":
			input := args.(map[string]any)
			pos, ok := input["pos"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("reference position = %#v", input["pos"])
			}
			mu.Lock()
			requests = append(requests, request{
				docID: input["doc"].(uint32),
				pos:   pos,
			})
			count := len(requests)
			mu.Unlock()
			locations := make([]openidea.Location, 0, count)
			for i := 0; i < count; i++ {
				locations = append(locations, usageTestLocation(
					"/test.java", 0, uint32(28+i*8), 0, uint32(33+i*8),
				))
			}
			raw, err := json.Marshal(openidea.References{Locations: locations})
			return raw, err
		default:
			return nil, fmt.Errorf("unexpected RPC %q", command)
		}
	}

	wait := func(label string) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not finish", label)
		}
	}

	m.FindUsages()
	wait("Find Usages")
	if m.usages.path != "/test.java" || m.usages.offset != offset ||
		len(m.usages.locs) != 1 || !m.ActionEnabled("usages.rerun") {
		t.Fatalf("first result = %+v", m.usages)
	}

	// Rerun follows the selected Find Usages tab, not a new caret position in
	// the editor.
	m.tab().state.GoTo(editor.Pos{Line: 0, Col: 0})
	m.RerunUsages()
	wait("Rerun Find Usages")

	mu.Lock()
	gotRequests := append([]request(nil), requests...)
	mu.Unlock()
	if len(gotRequests) != 2 {
		t.Fatalf("reference requests = %d, want 2", len(gotRequests))
	}
	if gotRequests[0].docID != gotRequests[1].docID ||
		!reflect.DeepEqual(gotRequests[0].pos, gotRequests[1].pos) {
		t.Fatalf("rerun changed the query: first=%+v second=%+v", gotRequests[0], gotRequests[1])
	}
	if m.usages.offset != offset || len(m.usages.locs) != 2 || !m.usages.done {
		t.Fatalf("rerun result = %+v", m.usages)
	}
	if len(m.usages.tabs) != 1 {
		t.Fatalf("rerun created %d tabs, want the same tab refreshed", len(m.usages.tabs))
	}
}

func TestUsageTabsKeepSearchesPinnedAndCloseOthers(t *testing.T) {
	m := &Model{ctx: context.Background()}
	first := usageTestLocation("/p/A.java", 1, 2, 1, 7)
	second := usageTestLocation("/p/B.java", 3, 4, 3, 9)

	m.showUsageResults("first", []openidea.Location{first}, nil, "")
	firstID := m.usages.activeTab
	m.showUsageResults("second", []openidea.Location{second}, nil, "")
	secondID := m.usages.activeTab
	if len(m.usages.tabs) != 2 || firstID == secondID {
		t.Fatalf("tabs = %+v", m.usages.tabs)
	}

	m.PinUsagesTab(firstID, true)
	m.SelectUsagesTab(firstID)
	if m.usages.symbol != "first" || len(m.usages.locs) != 1 ||
		m.usages.locs[0] != first || !m.usages.tabs[0].pinned {
		t.Fatalf("first tab state = %+v", m.usages.tabs[0])
	}
	m.SelectUsagesTab(secondID)
	if m.usages.symbol != "second" || m.usages.locs[0] != second {
		t.Fatalf("second tab state = %+v", m.usages.tabs[1])
	}

	m.CloseOtherUsagesTabs(secondID)
	if len(m.usages.tabs) != 1 || m.usages.activeTab != secondID ||
		m.usages.tabs[0].state.symbol != "second" {
		t.Fatalf("after close others = %+v", m.usages.tabs)
	}
	m.CloseUsagesTab(secondID)
	if len(m.usages.tabs) != 0 || m.usages.symbol != "" || m.usages.locs != nil {
		t.Fatalf("after closing the last tab = %+v", m.usages)
	}
}

func TestUsageAccessFiltersAndExcludedRowsStayVisible(t *testing.T) {
	m := &Model{ctx: context.Background()}
	read := usageTestLocation("/p/A.java", 1, 2, 1, 7)
	write := usageTestLocation("/p/A.java", 2, 2, 2, 7)
	m.showUsageResults("value", []openidea.Location{read, write}, []openidea.ClassifiedReference{
		{Location: read, Access: "read", Kind: "field"},
		{Location: write, Access: "write", Kind: "field"},
	}, "")
	if rows := m.usageVisibleRows(); len(rows) != 2 {
		t.Fatalf("default access rows = %d, want 2", len(rows))
	}

	m.prefs.Usages.HideRead = true
	rows := m.usageVisibleRows()
	if len(rows) != 1 || rows[0].source != write {
		t.Fatalf("read filter left %+v", rows)
	}

	m.prefs.Usages.HideRead = false
	writeRow := rows[0]
	m.toggleUsageExcluded(writeRow)
	rows = m.usageVisibleRows()
	if len(rows) != 2 || !m.usageRowExcluded(writeRow) {
		t.Fatalf("excluded rows = %+v, want the excluded row kept for Include", rows)
	}
	m.toggleUsageExcluded(writeRow)
	if rows = m.usageVisibleRows(); len(rows) != 2 || m.usageRowExcluded(writeRow) {
		t.Fatalf("including the excluded row left %+v", rows)
	}
}
