package app

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	mermaidpkg "mayoi/internal/mermaid"
)

func TestAgentMarkdownParsesBlocksAndInlineRuns(t *testing.T) {
	source := strings.Join([]string{
		"# Result",
		"",
		"Payments retry **three times** with `backoff`.",
		"",
		"- [OrderService.kt](src/OrderService.kt:42)",
		"- keeps ~~declined~~ cards",
		"",
		"| Case | Result |",
		"|---|---|",
		"| Timeout | retried |",
		"",
		"```kotlin",
		"retry(times = 3)",
		"```",
	}, "\n")
	blocks := agentMarkdownBlocks(source)
	var kinds []string
	for _, block := range blocks {
		kinds = append(kinds, block.Kind)
	}
	want := []string{"heading", "paragraph", "list", "table", "code"}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("blocks = %v, want %v", kinds, want)
	}
	if blocks[0].Level != 1 || blocks[0].Text != "Result" {
		t.Fatalf("heading = %+v", blocks[0])
	}
	if blocks[4].Lang != "kotlin" || blocks[4].Text != "retry(times = 3)" {
		t.Fatalf("code = %+v", blocks[4])
	}

	runs := agentMarkdownRuns("Payments retry **three times** with `backoff` and [the test](src/Test.kt:8).")
	var bold, code, href string
	for _, run := range runs {
		if run.Bold {
			bold += run.Text
		}
		if run.Code {
			code += run.Text
		}
		if run.Href != "" {
			href = run.Href
		}
	}
	if bold != "three times" || code != "backoff" || href != "src/Test.kt:8" {
		t.Fatalf("inline runs = %+v", runs)
	}

	runs = agentMarkdownRuns(`![QR code](images/code.png "Scan to sign in")`)
	if len(runs) != 1 || runs[0].Text != "QR code" ||
		runs[0].Image != "images/code.png" || runs[0].Title != "Scan to sign in" {
		t.Fatalf("image run = %+v", runs)
	}
}

func TestAgentMarkdownImageTargetsMatchOpenIDEA(t *testing.T) {
	for _, test := range []struct {
		source, cwd, url, path string
	}{
		{"images/code.png", "/repo", "", "/repo/images/code.png"},
		{"../code.png?v=2#preview", "/repo", "", "/repo/../code.png"},
		{"/tmp/截图%20one%23.png", "/repo", "", "/tmp/截图 one#.png"},
		{"file://localhost/tmp/code.png", "/repo", "", "/tmp/code.png"},
		{"C:/shots/code.png", "C:/repo", "", "C:/shots/code.png"},
		{`C:\shots\code.png`, "C:/repo", "", "C:/shots/code.png"},
		{`images\code.png`, `C:\repo`, "", "C:/repo/images/code.png"},
		{`\\server\share\code.png`, "C:/repo", "", `//server/share/code.png`},
		{"file://server/share/code.png", "C:/repo", "", "//server/share/code.png"},
		{"file:///C:/shots/code%20one.png", "C:/repo", "", "C:/shots/code one.png"},
		{"//example.com/code.png", "/repo", "https://example.com/code.png", ""},
		{"https://example.com/code.png", "/repo", "https://example.com/code.png", ""},
		{"data:image/gif;base64,R0lGODlhAQABAAAAACw=", "/repo",
			"data:image/gif;base64,R0lGODlhAQABAAAAACw=", ""},
	} {
		target, ok := agentMarkdownImageTarget(test.source, test.cwd)
		if !ok || target.URL != test.url || target.Path != test.path {
			t.Errorf("image target %q in %q = %+v, %v", test.source, test.cwd, target, ok)
		}
	}
	for _, source := range []string{
		"", "#section", "javascript:alert(1)", "JaVaScRiPt:alert(1)",
		"data:text/html,test", "ftp://example.com/x.png", "/tmp/%ZZ.png",
		"/tmp/code%00.png", "file:///tmp/%ZZ.png", "https://",
		"https://example.com/\ncode.png",
	} {
		if target, ok := agentMarkdownImageTarget(source, "/repo"); ok {
			t.Errorf("unsafe image target %q = %+v", source, target)
		}
	}
}

func TestAgentMarkdownRawHTMLImagesUseOnlySafeSourceAndIgnoreScripts(t *testing.T) {
	runs := agentMarkdownRuns(
		`<picture><source srcset="/wrong.png"><img alt="QR" src="/tmp/code.png" srcset="/wrong.png 2x" onerror="alert(1)"></picture>`)
	if len(runs) != 1 || runs[0].Text != "QR" || runs[0].Image != "/tmp/code.png" {
		t.Fatalf("raw HTML image runs = %+v", runs)
	}
	runs = agentMarkdownRuns(`before <style>body{display:none}</style><b>bold</b> after`)
	var text strings.Builder
	for _, run := range runs {
		text.WriteString(run.Text)
	}
	if strings.Contains(text.String(), "display:none") ||
		!strings.Contains(text.String(), "before ") || !strings.Contains(text.String(), "bold after") {
		t.Fatalf("sanitized raw HTML text = %q", text.String())
	}
}

func TestAgentMarkdownInlineCodeResolvesExistingFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "App.java")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("class App {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, line, column := agentMarkdownInlineCodeTarget(root, "src/App.java:12:5")
	if got != path || line != 11 || column != 4 {
		t.Fatalf("inline target = %q:%d:%d", got, line, column)
	}
	got, line, column = agentMarkdownInlineCodeTarget(root, path+"#L7")
	if got != path || line != 6 || column != 0 {
		t.Fatalf("absolute inline target = %q:%d:%d", got, line, column)
	}
	for _, text := range []string{"cargo test", "foo()", "missing.go", "1.5", "https://x/y.go"} {
		if got, _, _ := agentMarkdownInlineCodeTarget(root, text); got != "" {
			t.Errorf("inline text %q resolved to %q", text, got)
		}
	}
}

func TestAgentMarkdownLoadsDataImageWithoutNetworkConsent(t *testing.T) {
	const pixel = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"
	m := &Model{postHook: func(fn func()) { fn() }}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentMarkdown(c, c.Theme(), oiPaletteOf(c.Theme()), "![QR]("+pixel+")", "/repo", nil)
	}, 520, 320)
	if tester.HasText("Load image from") {
		t.Fatal("an inline data image asked for network consent")
	}
	waitAgentState(t, "data image", func() bool {
		for _, state := range m.agents.markdownImages {
			if state.Source != nil {
				return true
			}
		}
		return false
	})
}

func TestAgentMarkdownHoldsRemoteImagesUntilConsent(t *testing.T) {
	m := &Model{}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentMarkdown(c, c.Theme(), oiPaletteOf(c.Theme()),
			"![QR](https://example.com/code.png)", "/repo", nil)
	}, 520, 320)
	if !tester.HasText("Load image from example.com") {
		t.Fatalf("remote image did not show consent: %q", tester.Texts())
	}
	if len(m.agents.markdownImages) != 0 {
		t.Fatal("remote image was loaded before consent")
	}
}

func TestAgentMarkdownRendersMermaidFenceNatively(t *testing.T) {
	posted := make(chan func(), 1)
	m := &Model{postHook: func(fn func()) { posted <- fn }}
	source := "```mermaid\nflowchart LR\nA[Start] --> B[Done]\n```"
	render := func() *ui.Tester {
		return ui.NewTester(func(c *ui.Context) {
			m.agentMarkdown(c, c.Theme(), oiPaletteOf(c.Theme()), source, "/repo", nil)
		}, 720, 480)
	}

	tester := render()
	if !tester.HasText("Mermaid") || !tester.HasText("Rendering diagram…") {
		t.Fatalf("initial Mermaid state = %q", tester.Texts())
	}
	select {
	case apply := <-posted:
		apply()
	case <-time.After(10 * time.Second):
		t.Fatal("Mermaid render did not settle")
	}
	tester = render()
	if !tester.HasText("Mermaid diagram") {
		t.Fatalf("rendered Mermaid state = %q", tester.Texts())
	}
}

func TestAgentMarkdownMermaidErrorsStayVisible(t *testing.T) {
	posted := make(chan func(), 1)
	m := &Model{postHook: func(fn func()) { posted <- fn }}
	source := "```mermaid\nnot a diagram\n```"
	render := func() *ui.Tester {
		return ui.NewTester(func(c *ui.Context) {
			m.agentMarkdown(c, c.Theme(), oiPaletteOf(c.Theme()), source, "/repo", nil)
		}, 720, 320)
	}
	_ = render()
	select {
	case apply := <-posted:
		apply()
	case <-time.After(10 * time.Second):
		t.Fatal("invalid Mermaid diagram did not settle")
	}
	tester := render()
	if !tester.HasText("Unable to draw Mermaid:") {
		t.Fatalf("invalid Mermaid state = %q", tester.Texts())
	}
}

func TestAgentMarkdownMermaidCacheUsesSourceAndTheme(t *testing.T) {
	style := mermaidpkg.Style{
		BG: ui.Hex("#1e1f22"), FG: ui.Hex("#dfe1e5"),
		Muted: ui.Hex("#9da0a8"), Line: ui.Hex("#9da0a8"),
		Surface: ui.Hex("#2b2d30"), SurfaceAlt: ui.Hex("#393b40"),
		Border: ui.Hex("#4b4d51"), Accent: ui.Hex("#3574f0"),
		Danger: ui.Hex("#db5c5c"), Mono: "monospace", MonoWeight: 400,
	}
	const source = "flowchart LR\nA --> B"
	first := agentMarkdownDiagramKey(source, style)
	if second := agentMarkdownDiagramKey(source, style); second != first {
		t.Fatal("the same diagram and theme used different cache keys")
	}
	style.BG = ui.Hex("#ffffff")
	if agentMarkdownDiagramKey(source, style) == first {
		t.Fatal("a theme change reused the old diagram")
	}
	if agentMarkdownDiagramKey(source+"\nB --> C", mermaidpkg.Style{
		BG: style.BG, FG: style.FG, Muted: style.Muted, Line: style.Line,
		Surface: style.Surface, SurfaceAlt: style.SurfaceAlt, Border: style.Border,
		Accent: style.Accent, Danger: style.Danger, Mono: style.Mono, MonoWeight: style.MonoWeight,
	}) == agentMarkdownDiagramKey(source, style) {
		t.Fatal("different Mermaid source reused the old diagram")
	}

	m := &Model{postHook: func(func()) {}}
	m.agents.markdownDiagrams = make(map[string]*agentMarkdownDiagramState)
	for index := 0; index < agentMarkdownDiagramCount; index++ {
		key := "cached-" + string(rune('a'+index))
		m.agents.markdownDiagrams[key] = &agentMarkdownDiagramState{}
		m.agents.markdownDiagramOrder = append(m.agents.markdownDiagramOrder, key)
	}
	oldest := m.agents.markdownDiagramOrder[0]
	m.requestAgentMarkdownDiagram("flowchart LR\nOld --> New", style)
	if len(m.agents.markdownDiagrams) != agentMarkdownDiagramCount {
		t.Fatalf("diagram cache grew to %d", len(m.agents.markdownDiagrams))
	}
	if m.agents.markdownDiagrams[oldest] != nil {
		t.Fatal("the least recently used diagram stayed cached")
	}
}

func TestAgentMarkdownLoadsLocalImageAndCachesByFileVersion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "image.gif")
	data, err := base64.StdEncoding.DecodeString(
		"R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{postHook: func(fn func()) { fn() }}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentMarkdown(c, c.Theme(), oiPaletteOf(c.Theme()), "![Pixel](image.gif)", root, nil)
	}, 520, 320)
	_ = tester
	waitAgentState(t, "local image", func() bool {
		for _, state := range m.agents.markdownImages {
			if state.Source != nil {
				return true
			}
		}
		return false
	})
	target, ok := agentMarkdownImageTarget("image.gif", root)
	if !ok {
		t.Fatal("local image target was rejected")
	}
	first := agentMarkdownImageKey(target, "image.gif")
	if second := agentMarkdownImageKey(target, "image.gif"); first != second {
		t.Fatal("the same local image version used different cache keys")
	}
}

func TestAgentMarkdownLocalTargetResolvesLineAndColumn(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "App.java")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("class App {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, line, column := agentMarkdownLocalTarget(root, "src/App.java:12:7")
	if got != path || line != 12 || column != 7 {
		t.Fatalf("target = %q:%d:%d", got, line, column)
	}
	got, line, column = agentMarkdownLocalTarget(root, "src/App.java#L9")
	if got != path || line != 9 || column != 1 {
		t.Fatalf("anchor target = %q:%d:%d", got, line, column)
	}
	if target, _, _ := agentMarkdownLocalTarget(root, "https://example.com/a.java"); target != "" {
		t.Fatalf("remote URL became a local target: %q", target)
	}
}

func TestAgentMarkdownRendersContentInConversation(t *testing.T) {
	m := &Model{agents: AgentsPane{
		selected:      "a",
		conversation:  true,
		entries:       map[string][]agentEntry{"a": {{Type: "agent", Text: "# Done\n\nChanged **three files**.\n\n```go\nfmt.Println(\"ok\")\n```"}}},
		entriesLoaded: map[string]bool{"a": true},
		entriesBusy:   map[string]bool{},
		entriesErr:    map[string]string{},
		taskBusy:      map[string]bool{},
		tasks:         map[string]agentTask{"a": agentTestTask("a", "Markdown", "ready", 1)},
	}}
	tester := ui.NewTester(func(c *ui.Context) {
		m.agentsConversation(c, c.Theme(), oiPaletteOf(c.Theme()))
	}, 720, 640)
	for _, text := range []string{"Done", "Changed three files.", "go", "fmt.Println(\"ok\")", "Copy"} {
		if !tester.HasText(text) {
			t.Fatalf("Markdown reply lacks %q: %q", text, tester.Texts())
		}
	}
}

func TestAgentTimelineHonorsThoughtAndToolFoldSettings(t *testing.T) {
	render := func(m *Model) *ui.Tester {
		return ui.NewTester(func(c *ui.Context) {
			m.agentsConversation(c, c.Theme(), oiPaletteOf(c.Theme()))
		}, 720, 640)
	}
	newModel := func(status string, entries []agentEntry, prefs AgentPrefs) *Model {
		return &Model{
			prefs: Preferences{Agents: &prefs},
			agents: AgentsPane{
				selected:      "a",
				conversation:  true,
				entries:       map[string][]agentEntry{"a": entries},
				entriesLoaded: map[string]bool{"a": true},
				entriesBusy:   map[string]bool{},
				entriesErr:    map[string]string{},
				taskBusy:      map[string]bool{},
				tasks:         map[string]agentTask{"a": agentTestTask("a", "Timeline", status, 1)},
			},
		}
	}

	thought := agentEntry{Type: "thought", Text: "Reasoning preview\nPrivate reasoning body"}
	m := newModel("running", []agentEntry{thought}, AgentPrefs{})
	if !render(m).HasText("Private reasoning body") {
		t.Fatal("running reasoning is folded under the automatic setting")
	}
	m = newModel("running", []agentEntry{thought}, AgentPrefs{Thinking: "collapsed"})
	tester := render(m)
	if tester.HasText("Private reasoning body") {
		t.Fatal("collapsed reasoning rendered its body")
	}
	if !tester.HasText("Thinking") {
		t.Fatal("collapsed reasoning lost its live summary")
	}
	m = newModel("ready", []agentEntry{thought}, AgentPrefs{Thinking: "expanded"})
	if !render(m).HasText("Private reasoning body") {
		t.Fatal("expanded reasoning did not render its body")
	}

	output := "tool output only when opened"
	tool := agentEntry{
		Type: "tool", Title: "Read App.java", Kind: "read",
		Status: "completed", Output: &output,
	}
	m = newModel("ready", []agentEntry{tool}, AgentPrefs{})
	tester = render(m)
	if tester.HasText(output) {
		t.Fatal("completed tool output was open by default")
	}
	m = newModel("ready", []agentEntry{tool}, AgentPrefs{ExpandToolOutput: true})
	if !render(m).HasText(output) {
		t.Fatal("expandToolOutput did not open the output")
	}
	m = newModel("running", []agentEntry{{
		Type: "tool", Title: "Run tests", Kind: "execute",
		Status: "running", Output: &output,
	}}, AgentPrefs{})
	if !render(m).HasText(output) {
		t.Fatal("live tool output was not open")
	}
}

func TestAgentRevealPacingAndUnicodeBoundaries(t *testing.T) {
	if agentRevealStep(100, 0) != 1 ||
		agentRevealStep(100, agentRevealHorizon/2) != 50 ||
		agentRevealStep(100, agentRevealHorizon) != 100 {
		t.Fatal("reveal pacing does not drain the backlog over the horizon")
	}
	if got := agentTextPrefix("A😀中", 2); got != "A😀" {
		t.Fatalf("prefix after two runes = %q", got)
	}
	if got := agentWithoutBrokenTail("flag \U0001F1E8"); got != "flag " {
		t.Fatalf("half flag was painted: %q", got)
	}
	if agentRevealStep(5, -time.Second) != 1 {
		t.Fatal("negative elapsed must still make progress")
	}
}

func TestAgentImageCacheKeyDoesNotEmbedImageData(t *testing.T) {
	data := strings.Repeat("A", 10_000)
	first := agentImageCacheKey("image/png", data)
	second := agentImageCacheKey("image/png", data)
	if first != second || len(first) != 64 || strings.Contains(first, "AAAA") {
		t.Fatalf("cache key = %q, %q", first, second)
	}
	if first == agentImageCacheKey("image/jpeg", data) {
		t.Fatal("MIME type is not part of the image cache key")
	}
}
