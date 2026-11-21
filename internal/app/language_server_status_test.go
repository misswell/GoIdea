package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func languageServerStatusFixture() map[uint32]languageServerInfo {
	return map[uint32]languageServerInfo{
		1: {
			ID: 1, Name: "Eclipse JDT Language Server", Tool: "jdtls",
			Status: "ready", Languages: []string{"java"}, Root: "/project",
		},
		2: {
			ID: 2, Name: "TypeScript Language Server", Tool: "typescript-language-server",
			Status: "ready", Languages: []string{"TypeScript"}, Root: "/project",
		},
		3: {
			ID: 3, Name: "vtsls", Tool: "vtsls", Status: "missing",
			Languages: []string{"TypeScript"}, Root: "/project",
		},
		4: {
			ID: 4, Name: "jdtls", Tool: "jdtls", Status: "misconfigured",
			Message: "jdtls needs JDK 25 or newer to start", Languages: []string{"java"},
		},
		5: {
			ID: 5, Name: "ESLint", Tool: "eslint", Status: "missing",
			Languages: []string{"JavaScript"}, Root: "/project",
		},
	}
}

func TestLanguageServerSummaryMatchesOpenIDEAServedStates(t *testing.T) {
	m := &Model{languageServers: languageServerStatusFixture()}
	servers := m.languageServerList()
	text, tone := languageServerSummary(servers, m.languageServerServedByOther)
	if text != "Eclipse JDT Language Server +4" || tone != "error" {
		t.Fatalf("summary = %q %q", text, tone)
	}
	if !m.languageServerServedByOther(m.languageServers[3]) {
		t.Fatal("vtsls should be served by typescript-language-server")
	}
}

func TestLanguageServerEventsAndSnapshotKeepTheNewestState(t *testing.T) {
	m := &Model{}
	m.ctx = context.Background()
	root := t.TempDir()
	m.setRoot(root)
	done := make(chan struct{}, 1)
	m.postHook = func(fn func()) {
		fn()
		done <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "lsp_servers" || args != nil {
			return nil, fmt.Errorf("unexpected RPC %s(%v)", command, args)
		}
		return json.RawMessage(`[
			{"id":1,"name":"jdtls","status":"ready","languages":["java"],"root":"/project"},
			{"id":2,"name":"gopls","status":"starting","languages":["go"],"root":"/project"}
		]`), nil
	}
	m.RefreshLanguageServers()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("language server snapshot did not finish")
	}
	if len(m.languageServers) != 2 || m.languageServers[1].Status != "ready" {
		t.Fatalf("snapshot = %+v", m.languageServers)
	}

	label := "Eclipse JDTLS"
	m.acceptLanguageServerStatus(openidea.LanguageServerEvent{
		Server: 1, Name: "jdtls", Label: &label, Status: "ready",
	})
	if got := m.languageServerJavaReady[1]; got != "/project" {
		t.Fatalf("Java ready root = %q, want /project", got)
	}
	wantsPrompt := m.acceptLanguageServerStatus(openidea.LanguageServerEvent{
		Server: 1, Name: "jdtls", Label: &label, Status: "busy",
	})
	if wantsPrompt {
		t.Fatal("busy event asked for a missing install prompt")
	}
	if got := m.languageServerJavaReady[1]; got != "/project" {
		t.Fatalf("busy background work cleared Java ready at %q", got)
	}
	// jdtls was ready and busy is delayed by 500 ms in the renderer test's
	// terms. A different lifecycle state is visible at once.
	m.acceptLanguageServerStatus(openidea.LanguageServerEvent{
		Server: 1, Name: "jdtls", Label: &label, Status: "crashed",
		Message: stringPointer("the server exited"),
	})
	if got := m.languageServers[1]; got.Name != label || got.Status != "crashed" ||
		got.Message != "the server exited" || len(got.Languages) != 1 {
		t.Fatalf("updated server = %+v", got)
	}
	if _, ok := m.languageServerJavaReady[1]; ok {
		t.Fatal("a crashed server stayed Java ready")
	}

	m.acceptLanguageServerRemoved(openidea.LanguageServerRemovedEvent{Server: 2})
	if _, ok := m.languageServers[2]; ok {
		t.Fatal("removed server stayed in the snapshot")
	}
	m.languageServerRemovedAt[2] = m.languageServerSeq
	if m.languageServerServedByOther(languageServerInfo{
		ID: 3, Status: "missing", Languages: []string{"go"},
	}) {
		t.Fatal("a removed server still serves fallback languages")
	}
}

func TestLanguageServicesStatusRendersSnapshotAndRepairActions(t *testing.T) {
	m := &Model{
		root:               "/project",
		languageServers:    languageServerStatusFixture(),
		languageServerOpen: true,
	}
	p := openideaTheme(false)
	tester := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(p)
		m.languageServersStatus(c, p, oiPaletteOf(p))
	}, 760, 420)
	tester.Frame()
	for _, text := range []string{
		"Eclipse JDT Language Server", "Running", "vtsls", "Not installed",
		"ESLint", "jdtls", "Not started", "jdtls needs JDK 25 or newer to start",
		"Install", "Download JDK 25", "Restart",
	} {
		if !tester.HasText(text) {
			t.Fatalf("Language Services status is missing %q: %v", text, tester.Texts())
		}
	}
	ids := statusIDs(m.statusSide("right"))
	if indexOf(ids, "code-insight.servers") < 0 {
		t.Fatalf("Language Services status item is absent: %v", ids)
	}
}
