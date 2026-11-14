package app

// pom.xml go to declaration, after v0.1.44's pomNavigation.ts: on a
// ${property} reference, or inside a <dependency> / <plugin> / <parent>
// element, the backend (maven_goto_declaration) resolves the declaration
// along the parent chain — a <properties> entry, a <dependencyManagement> /
// <pluginManagement> entry, or the artifact's own POM, from the project or
// the local repository. GoIDEA asks the same RPC from the editor's definition
// hook and only for those spans, as upstream does; anything else falls back
// to the language server.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

type pomGoto struct {
	Path  string                `json:"path"`
	Range openidea.OutlineRange `json:"range"`
	Kind  string                `json:"kind"`
	Label string                `json:"label"`
}

var (
	depOpen     = regexp.MustCompile(`<dependency[\s>]`)
	depClose    = regexp.MustCompile(`</dependency\s*>`)
	pluginOpen  = regexp.MustCompile(`<plugin[\s>]`)
	pluginClose = regexp.MustCompile(`</plugin\s*>`)
	parentOpen  = regexp.MustCompile(`<parent[\s>]`)
	parentClose = regexp.MustCompile(`</parent\s*>`)
)

// propertyRefAt is the ${property} reference covering at, if any (clicking
// the $ counts), as upstream's eligibility test computes it.
func propertyRefAt(text string, at int) (int, int, bool) {
	if at < 0 || at > len(text) {
		return 0, 0, false
	}
	open := -1
	if strings.HasPrefix(text[at:], "${") {
		open = at
	} else {
		// lastIndexOf("${", at-1): a reference whose $ is at at-1 still counts.
		end := at + 1
		if end > len(text) {
			end = len(text)
		}
		open = strings.LastIndex(text[:end], "${")
	}
	if open < 0 {
		return 0, 0, false
	}
	rel := strings.IndexByte(text[open+2:], '}')
	if rel < 0 {
		return 0, 0, false
	}
	close := open + 2 + rel
	if close < at {
		return 0, 0, false
	}
	return open, close + 1, true
}

// insideElement reports whether an opener of name before at has no closer
// yet; POM elements do not nest.
func insideElement(text string, at int, open, close *regexp.Regexp) bool {
	before := text[:at]
	return len(open.FindAllStringIndex(before, -1)) > len(close.FindAllStringIndex(before, -1))
}

// pomGotoEligible is whether at may resolve to a declaration: a property
// reference or inside a dependency, plugin or parent element.
func pomGotoEligible(text string, at int) bool {
	if _, _, ok := propertyRefAt(text, at); ok {
		return true
	}
	return insideElement(text, at, depOpen, depClose) ||
		insideElement(text, at, pluginOpen, pluginClose) ||
		insideElement(text, at, parentOpen, parentClose)
}

// pomGotoDeclaration asks the backend where the POM declaration at off is
// and selects it. It reports whether it took the request; false leaves the
// position to the language server.
func (m *Model) pomGotoDeclaration(t *Tab, off int) bool {
	if m.mgr == nil || m.ctx == nil || !m.tabOpen(t) {
		return false
	}
	text := t.doc.Buffer().Snapshot().String()
	if off < 0 || off > len(text) || !pomGotoEligible(text, off) {
		return false
	}
	pom := t.doc.Path()
	pos := t.doc.UTF16Pos(off)
	version := t.doc.Buffer().Version()
	t.defGen++
	gen := t.defGen
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		raw, err := m.mgr.Invoke(ctx, Window, "maven_goto_declaration", map[string]any{
			"pom": pom, "pos": lspPos(pos),
		})
		var target *pomGoto
		if err == nil {
			if strings.TrimSpace(string(raw)) != "null" {
				err = json.Unmarshal(raw, &target)
			}
		}
		m.post(func() {
			if t.defGen != gen || !m.tabOpen(t) || t.doc.Path() != pom {
				return
			}
			if t.doc.Buffer().Version() != version {
				return
			}
			switch {
			case err != nil:
				m.status = "跳转失败：" + err.Error()
			case target == nil:
				m.status = "未找到声明"
			default:
				m.revealRange(target.Path,
					int(target.Range.Start.Line), int(target.Range.Start.Col),
					int(target.Range.End.Line), int(target.Range.End.Col))
				return
			}
			m.invalidate()
		})
	}()
	return true
}

// SmokePomGoto opens the project's POM, runs the definition at a property
// reference (or at the first one when the caller names no needle) and reports
// where the navigation landed, for the real-server acceptance run.
func (m *Model) SmokePomGoto(ctx context.Context, relPom, needle string) (map[string]any, error) {
	out := map[string]any{}
	if relPom == "" {
		relPom = "pom.xml"
	}
	pom := filepath.Join(m.Root(), relPom)
	text, err := os.ReadFile(pom)
	if err != nil {
		return out, err
	}
	src := string(text)
	off := -1
	if needle != "" {
		off = strings.Index(src, needle)
		if off < 0 {
			return out, fmt.Errorf("needle %q not found in %s", needle, pom)
		}
	} else {
		for at := 0; at < len(src); at++ {
			if pomGotoEligible(src, at) {
				off = at
				break
			}
		}
		if off < 0 {
			return out, fmt.Errorf("no go-to-declaration position in %s", pom)
		}
	}
	out["pom"] = filepath.Base(pom)
	out["needle"] = strings.TrimSpace(src[off : off+min(40, len(src)-off)])
	m.post(func() { m.openFile(pom) })
	deadline := time.Now().Add(60 * time.Second)
	for {
		var tab *Tab
		m.post(func() { tab = m.tab() })
		if tab != nil && tab.doc.Path() == pom {
			break
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("POM did not open")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	requested := false
	for {
		var status, target, selected string
		var landed bool
		m.post(func() {
			if !requested {
				if tab := m.tab(); tab != nil && tab.doc.Path() == pom {
					m.gotoDefinition(tab, off)
					requested = true
				}
				return
			}
			status = m.status
			if tab := m.tab(); tab != nil {
				target = tab.doc.Path()
				selected = tab.state.SelectedText()
			}
			landed = m.reveal == nil && (status != "" || target != pom || selected != "")
		})
		if requested && landed {
			if rel, err := filepath.Rel(m.Root(), target); err == nil {
				target = rel
			}
			out["target"] = target
			out["selected"] = strings.TrimSpace(selected)
			out["status"] = status
			return out, nil
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("go to declaration did not finish")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
