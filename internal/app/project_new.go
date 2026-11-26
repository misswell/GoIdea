package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
)

// projectNewState is the Project view's searchable template chooser. The
// chosen template then uses the regular New file dialog for its name and kind.
type projectNewState struct {
	open    bool
	dir     string
	anchor  ui.Rect
	query   string
	matches []projectNewMatch
	sel     int
}

type projectNewMatch struct {
	entry      projectNewEntry
	highlights []int
	score      int
}

type projectNewEntry struct {
	key           string
	label         string
	keywords      []string
	templateID    string
	templateIndex int
	action        string
	separator     bool
}

type projectNewTemplate struct {
	id             string
	name           string
	keywords       string
	extension      string
	icon           string
	nameStyle      string
	packageStyle   string
	dottedPackages bool
	defaultName    string
	fixedName      string
	enabled        bool
	reformat       bool
	preferencesSet bool
	kinds          []projectNewKind
}

type projectNewKind struct {
	id          string
	label       string
	body        string
	extension   string
	description string
}

type projectNewPlan struct {
	path      string
	directory string
	name      string
	fileName  string
}

type projectNewVars struct {
	name        string
	fileName    string
	pkg         string
	projectName string
	user        string
	now         time.Time
}

var projectNewIncludes = map[string]string{
	"File Header":               "",
	"JUnit5 Test Method":        "@Test\nvoid test() {\n    $END$\n}\n",
	"Kotlin JUnit5 Test Method": "@Test\nfun test() {\n    $END$\n}\n",
	"Go Test Function":          "func Test$NAME$(t *testing.T) {\n\t$END$\n}\n",
	"Rust Test Function":        "#[test]\nfn it_works() {\n    $END$\n}\n",
	"Vitest Test": `it("works", () => {
  $END$
});
`,
	"PHPUnit Test Method":  "public function testSomething(): void\n{\n    $END$\n}\n",
	"pytest Test Function": "def test_$NAME$():\n    $END$\n",
}

var projectNewTemplates = []projectNewTemplate{
	{
		id: "file", name: "File", keywords: "empty plain text",
		nameStyle: "asis",
		kinds:     []projectNewKind{{id: "file", label: "File", body: "$END$"}},
	},
	{
		id: "rust", name: "Rust File", keywords: "rust cargo module struct test",
		extension: "rs", nameStyle: "snake",
		kinds: []projectNewKind{
			{id: "module", label: "Module", body: "$END$\n"},
			{id: "struct", label: "Struct", body: "pub struct $NAME$ {\n    $END$\n}\n"},
			{id: "test", label: "Test module", body: "#[cfg(test)]\nmod tests {\n    use super::*;\n\n    #parse(\"Rust Test Function\")\n}\n"},
		},
	},
	{
		id: "typescript", name: "TypeScript File", keywords: "typescript ts class test vitest",
		extension: "ts", nameStyle: "asis",
		kinds: []projectNewKind{
			{id: "file", label: "File", body: "$END$\n"},
			{id: "class", label: "Class", body: "export class $NAME$ {\n  $END$\n}\n"},
			{id: "test", label: "Test", body: "import { describe, expect, it } from \"vitest\";\n\ndescribe(\"$NAME$\", () => {\n  #parse(\"Vitest Test\")\n});\n"},
		},
	},
	{
		id: "tsx", name: "React Component", keywords: "react jsx tsx component stylesheet css module",
		extension: "tsx", nameStyle: "pascal",
		kinds: []projectNewKind{
			{id: "component", label: "Function component", body: "export interface $NAME$Props {}\n\nexport function $NAME$(props: $NAME$Props) {\n  return <div>$END$</div>;\n}\n"},
			{id: "styled", label: "Component with CSS module", body: "import styles from \"./$NAME$.module.css\";\n\nexport function $NAME$() {\n  return <div className={styles.root}>$END$</div>;\n}\n"},
		},
	},
	{
		id: "javascript", name: "JavaScript File", keywords: "javascript js script",
		extension: "js", nameStyle: "asis",
		kinds: []projectNewKind{{id: "file", label: "File", body: "$END$\n"}},
	},
	{
		id: "python", name: "Python File", keywords: "python py module class pytest test script",
		extension: "py", nameStyle: "snake",
		kinds: []projectNewKind{
			{id: "module", label: "Module", body: "\"\"\"$NAME$ module.\"\"\"\n\n$END$\n"},
			{id: "class", label: "Class", body: "class $NAME$:\n    def __init__(self):\n        $END$\n"},
			{id: "test", label: "Test (pytest)", body: "#parse(\"pytest Test Function\")\n"},
			{id: "script", label: "Script", body: "def main():\n    $END$\n\n\nif __name__ == \"__main__\":\n    main()\n"},
		},
	},
	{
		id: "go", name: "Go File", keywords: "go golang package testing main test",
		extension: "go", nameStyle: "snake", packageStyle: "go",
		kinds: []projectNewKind{
			{id: "file", label: "Empty file", body: "package $PACKAGE$\n\n$END$\n"},
			{id: "test", label: "Test file", body: "package $PACKAGE$\n\nimport \"testing\"\n\n#parse(\"Go Test Function\")\n"},
			{id: "main", label: "Main", body: "package main\n\nfunc main() {\n\t$END$\n}\n"},
		},
	},
	{
		id: "go-mod", name: "Go Module", keywords: "go module mod go.mod",
		extension: "mod", nameStyle: "asis", defaultName: "go",
		kinds: []projectNewKind{{id: "module", label: "Module", body: "module $PROJECT_NAME$\n\ngo 1.20\n"}},
	},
	{
		id: "php", name: "PHP File", keywords: "php file script",
		extension: "php", nameStyle: "asis",
		kinds: []projectNewKind{{id: "file", label: "File", body: "<?php\n\n$END$\n"}},
	},
	{
		id: "php-class", name: "PHP Class", keywords: "php class interface trait enum phpunit test namespace",
		extension: "php", nameStyle: "pascal", packageStyle: "php",
		kinds: []projectNewKind{
			{id: "class", label: "Class", body: "<?php\n\nnamespace $PACKAGE$;\n\n#parse(\"File Header\")\nclass $NAME$\n{\n    $END$\n}\n"},
			{id: "interface", label: "Interface", body: "<?php\n\nnamespace $PACKAGE$;\n\n#parse(\"File Header\")\ninterface $NAME$\n{\n    $END$\n}\n"},
			{id: "trait", label: "Trait", body: "<?php\n\nnamespace $PACKAGE$;\n\n#parse(\"File Header\")\ntrait $NAME$\n{\n    $END$\n}\n"},
			{id: "enum", label: "Enum", body: "<?php\n\nnamespace $PACKAGE$;\n\n#parse(\"File Header\")\nenum $NAME$\n{\n    $END$\n}\n"},
			{id: "test", label: "PHPUnit Test", body: "<?php\n\nnamespace $PACKAGE$;\n\nuse PHPUnit\\Framework\\TestCase;\n\n#parse(\"File Header\")\nfinal class $NAME$ extends TestCase\n{\n    #parse(\"PHPUnit Test Method\")\n}\n"},
		},
	},
	{
		id: "java", name: "Java Class", keywords: "java class interface enum record annotation junit test package",
		extension: "java", nameStyle: "pascal", packageStyle: "jvm", dottedPackages: true,
		kinds: []projectNewKind{
			{id: "class", label: "Class", body: "package $PACKAGE$;\n\n#parse(\"File Header\")\npublic class $NAME$ {\n    $END$\n}\n"},
			{id: "interface", label: "Interface", body: "package $PACKAGE$;\n\n#parse(\"File Header\")\npublic interface $NAME$ {\n    $END$\n}\n"},
			{id: "enum", label: "Enum", body: "package $PACKAGE$;\n\n#parse(\"File Header\")\npublic enum $NAME$ {\n    $END$\n}\n"},
			{id: "record", label: "Record", body: "package $PACKAGE$;\n\n#parse(\"File Header\")\npublic record $NAME$($END$) {\n}\n"},
			{id: "annotation", label: "Annotation", body: "package $PACKAGE$;\n\n#parse(\"File Header\")\npublic @interface $NAME$ {\n    $END$\n}\n"},
			{id: "test", label: "JUnit 5 Test", body: "package $PACKAGE$;\n\nimport org.junit.jupiter.api.Test;\n\nimport static org.junit.jupiter.api.Assertions.*;\n\n#parse(\"File Header\")\nclass $NAME$ {\n\n    #parse(\"JUnit5 Test Method\")\n}\n"},
		},
	},
	{
		id: "kotlin", name: "Kotlin Class/File", keywords: "kotlin kt class file interface data enum object junit test package",
		extension: "kt", nameStyle: "pascal", packageStyle: "jvm", dottedPackages: true,
		kinds: []projectNewKind{
			{id: "class", label: "Class", body: "package $PACKAGE$\n\n#parse(\"File Header\")\nclass $NAME$ {\n    $END$\n}\n"},
			{id: "file", label: "File", body: "package $PACKAGE$\n\n$END$\n"},
			{id: "interface", label: "Interface", body: "package $PACKAGE$\n\n#parse(\"File Header\")\ninterface $NAME$ {\n    $END$\n}\n"},
			{id: "data", label: "Data class", body: "package $PACKAGE$\n\n#parse(\"File Header\")\ndata class $NAME$($END$)\n"},
			{id: "enum", label: "Enum class", body: "package $PACKAGE$\n\n#parse(\"File Header\")\nenum class $NAME$ {\n    $END$\n}\n"},
			{id: "object", label: "Object", body: "package $PACKAGE$\n\n#parse(\"File Header\")\nobject $NAME$ {\n    $END$\n}\n"},
			{id: "test", label: "JUnit 5 Test", body: "package $PACKAGE$\n\nimport org.junit.jupiter.api.Test\nimport org.junit.jupiter.api.Assertions.*\n\n#parse(\"File Header\")\nclass $NAME$ {\n\n    #parse(\"Kotlin JUnit5 Test Method\")\n}\n"},
		},
	},
	{
		id: "html", name: "HTML File", keywords: "html web page fragment",
		extension: "html", nameStyle: "asis",
		kinds: []projectNewKind{
			{id: "html5", label: "HTML 5 file", body: "<!doctype html>\n<html lang=\"en\">\n<head>\n  <meta charset=\"UTF-8\">\n  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n  <title>$NAME$</title>\n</head>\n<body>\n$END$\n</body>\n</html>\n"},
			{id: "fragment", label: "Fragment", body: "$END$\n"},
		},
	},
	{
		id: "markdown", name: "Markdown File", keywords: "markdown md readme document",
		extension: "md", nameStyle: "asis",
		kinds: []projectNewKind{{id: "md", label: "Markdown file", body: "# $NAME$\n\n$END$\n"}},
	},
	{
		id: "stylesheet", name: "Stylesheet", keywords: "css scss sass less stylesheet",
		extension: "css", nameStyle: "asis",
		kinds: []projectNewKind{
			{id: "css", label: "CSS File", body: "$END$\n"},
			{id: "scss", label: "SCSS File", body: "$END$\n", extension: "scss"},
			{id: "sass", label: "Sass File", body: "$END$\n", extension: "sass"},
			{id: "less", label: "Less File", body: "$END$\n", extension: "less"},
		},
	},
	{
		id: "json", name: "JSON File", keywords: "json config object data",
		extension: "json", nameStyle: "asis",
		kinds: []projectNewKind{{id: "json", label: "JSON file", body: "{\n  $END$\n}\n"}},
	},
	{
		id: "yaml", name: "YAML File", keywords: "yaml yml config workflow",
		extension: "yaml", nameStyle: "asis",
		kinds: []projectNewKind{{id: "yaml", label: "YAML file", body: "$END$\n"}},
	},
	{
		id: "http-request", name: "HTTP Request", keywords: "http client rest scratch curl",
		extension: "http", nameStyle: "asis", defaultName: "requests",
		kinds: []projectNewKind{{
			id: "request", label: "HTTP Request",
			body: "### Example request\n# Variables come from http-client.env.json / http-client.private.env.json\n# next to this file or in the project root: {{host}}\nGET https://example.com/\nAccept: application/json\n\n###\n",
		}},
	},
	{
		id: "toml", name: "TOML File", keywords: "toml cargo config",
		extension: "toml", nameStyle: "asis",
		kinds: []projectNewKind{{id: "toml", label: "TOML file", body: "$END$\n"}},
	},
	{
		id: "shell", name: "Shell Script", keywords: "shell sh bash zsh script terminal",
		extension: "sh", nameStyle: "asis",
		kinds: []projectNewKind{
			{id: "bash", label: "Bash script", body: "#!/usr/bin/env bash\nset -euo pipefail\n\n$END$\n"},
			{id: "sh", label: "POSIX sh script", body: "#!/bin/sh\n\n$END$\n"},
			{id: "zsh", label: "Zsh script", body: "#!/usr/bin/env zsh\n\n$END$\n", extension: "zsh"},
		},
	},
	{
		id: "dockerfile", name: "Dockerfile", keywords: "docker container image build",
		nameStyle: "asis", defaultName: "Dockerfile", fixedName: "Dockerfile",
		kinds: []projectNewKind{{id: "dockerfile", label: "Dockerfile", body: "FROM $END$\n"}},
	},
	{
		id: "editorconfig", name: "EditorConfig File", keywords: "editorconfig formatting style",
		nameStyle: "asis", defaultName: ".editorconfig", fixedName: ".editorconfig",
		kinds: []projectNewKind{
			{id: "default", label: "Default properties", body: "root = true\n\n[*]\ncharset = utf-8\nend_of_line = lf\nindent_style = space\nindent_size = 4\ninsert_final_newline = true\ntrim_trailing_whitespace = true\n$END$"},
			{id: "root", label: "Root only", body: "root = true\n\n[*]\n$END$\n"},
		},
	},
	{
		id: "gitignore", name: ".gitignore File", keywords: "git ignore ignorefile",
		nameStyle: "asis", defaultName: ".gitignore", fixedName: ".gitignore",
		kinds: []projectNewKind{{id: "gitignore", label: ".gitignore file", body: "$END$\n"}},
	},
	{
		id: "tomcat-context", name: "Tomcat Context Descriptor", keywords: "tomcat xml context java servlet",
		extension: "xml", nameStyle: "asis", defaultName: "context",
		kinds: []projectNewKind{{id: "context", label: "Tomcat Context Descriptor (context.xml)", body: "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Context antiJARLocking=\"true\" path=\"$END$\">\n</Context>\n"}},
	},
}

// OpenProjectNew opens OpenIDEA's anchored New popup for dir.
func (m *Model) OpenProjectNew(dir string) {
	anchor := ui.Rect{}
	if m.projectTreeFocus {
		anchor = m.projectTreeAnchor
	} else if m.usagePopupAnchor.W > 0 {
		anchor = m.usagePopupAnchor
	}
	m.OpenProjectNewAt(dir, anchor)
}

// OpenProjectNewAt opens the New popup at anchor. A zero anchor uses the
// fallback near the Project View's top-left, matching the headless surface.
func (m *Model) OpenProjectNewAt(dir string, anchor ui.Rect) {
	if dir == "" {
		return
	}
	m.projectNew = &projectNewState{open: true, dir: dir, anchor: anchor}
	m.refreshProjectNewMatches()
	m.invalidate()
}

func (m *Model) CloseProjectNew() {
	m.projectNew = nil
	m.invalidate()
}

func (m *Model) refreshProjectNewMatches() {
	v := m.projectNew
	if v == nil {
		return
	}
	query := strings.TrimSpace(v.query)
	found := make([]projectNewMatch, 0, len(projectNewEntries()))
	var keywordMatches []projectNewMatch
	for _, entry := range m.projectNewEntries() {
		if query == "" {
			found = append(found, projectNewMatch{entry: entry})
			continue
		}
		labelMatches, score, ok := fuzzySubsequence(query, entry.label)
		if ok {
			found = append(found, projectNewMatch{
				entry: entry, highlights: labelMatches, score: score,
			})
		} else if projectNewKeywordMatch(entry, query) {
			keywordMatches = append(keywordMatches, projectNewMatch{entry: entry})
		}
	}
	v.matches = append(found, keywordMatches...)
	if v.sel >= len(v.matches) {
		v.sel = max(0, len(v.matches)-1)
	}
}

func projectNewKeywordMatch(entry projectNewEntry, query string) bool {
	query = strings.ToLower(strings.TrimPrefix(query, "."))
	if query == "" {
		return false
	}
	for _, keyword := range entry.keywords {
		keyword = strings.ToLower(strings.TrimPrefix(keyword, "."))
		if keyword != "" && !projectNewGenericKeyword(keyword) && strings.HasPrefix(keyword, query) {
			return true
		}
	}
	return false
}

func projectNewGenericKeyword(keyword string) bool {
	switch keyword {
	case "new", "create", "template", "file":
		return true
	default:
		return false
	}
}

func projectNewEntries() []projectNewEntry {
	scheme := defaultFileTemplateScheme()
	return projectNewEntriesForCatalog(scheme.Templates)
}

func (m *Model) projectNewEntries() []projectNewEntry {
	if m == nil {
		return projectNewEntries()
	}
	scheme := m.effectiveFileTemplateScheme()
	return projectNewEntriesForCatalog(scheme.Templates)
}

func projectNewEntriesForCatalog(templates []projectNewTemplate) []projectNewEntry {
	entries := []projectNewEntry{
		{key: "new.file", label: "File", templateID: "file", templateIndex: 0, action: "file"},
		{key: "new.dir", label: "Directory", templateIndex: -1, action: "directory"},
		{
			key: "scratch.new", label: "Scratch File", templateIndex: -1, action: "scratch",
			keywords: []string{"scratch", "temporary", "buffer", "language"},
		},
	}
	firstTemplate := true
	for index := range templates {
		template := &templates[index]
		if template.id == "file" || template.id == "http-request" || !template.enabled {
			continue
		}
		entries = append(entries, projectNewEntry{
			key: "new.template." + template.id, label: template.name,
			keywords:   projectNewTemplateKeywords(template),
			templateID: template.id, templateIndex: index, action: "template", separator: firstTemplate,
		})
		firstTemplate = false
	}
	for index := range templates {
		template := &templates[index]
		if template.id != "http-request" || !template.enabled {
			continue
		}
		entries = append(entries, projectNewEntry{
			key: "http.newRequest", label: template.name,
			keywords:   []string{"http client", "rest", "scratch", "curl"},
			templateID: template.id, templateIndex: index, action: "template", separator: true,
		})
		break
	}
	entries = append(entries, projectNewEntry{
		key: "new.fromTemplate", label: "From Template…",
		keywords:      []string{"new", "file", "create", "class"},
		templateIndex: 0, action: "fromTemplate", separator: true,
	})
	return entries
}

func projectNewTemplateKeywords(template *projectNewTemplate) []string {
	keywords := []string{
		"new", "create", "template", template.extension, template.icon,
		template.nameStyle, template.packageStyle, template.defaultName, template.keywords,
	}
	for _, kind := range template.kinds {
		keywords = append(keywords, kind.extension, kind.label, kind.description)
	}
	seen := make(map[string]bool, len(keywords))
	out := keywords[:0]
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		key := strings.ToLower(keyword)
		if keyword == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, keyword)
	}
	return out
}

func (m *Model) StepProjectNew(delta int) {
	v := m.projectNew
	if v == nil || len(v.matches) == 0 {
		return
	}
	v.sel = (v.sel + delta) % len(v.matches)
	if v.sel < 0 {
		v.sel += len(v.matches)
	}
	m.invalidate()
}

// SelectProjectNewTemplate chooses a template by its index in
// projectNewTemplates. The chooser is closed and the name step opens.
func (m *Model) SelectProjectNewTemplate(index int) {
	scheme := m.effectiveFileTemplateScheme()
	templates := scheme.Templates
	v := m.projectNew
	if v == nil || index < 0 || index >= len(templates) {
		return
	}
	t := &templates[index]
	if !t.enabled {
		return
	}
	name := t.defaultName
	m.projectNew = nil
	m.treeOp = &treeOpState{
		kind: treeOpNewFile, path: v.dir, dir: true, name: name, open: true,
		projectTemplate: t, projectIncludes: templateSchemeFragments(scheme),
	}
	m.refreshProjectNewTarget(m.treeOp)
	m.invalidate()
}

func templateSchemeFragments(scheme FileTemplateScheme) map[string]string {
	out := make(map[string]string, len(projectNewIncludes)+len(scheme.Includes)+len(scheme.Code))
	for name, body := range projectNewIncludes {
		out[name] = body
	}
	for name, body := range scheme.Includes {
		out[name] = body
	}
	for name, body := range scheme.Code {
		out[name] = body
	}
	return out
}

func (m *Model) selectCurrentProjectNewTemplate() {
	v := m.projectNew
	if v == nil || len(v.matches) == 0 {
		return
	}
	m.chooseProjectNewMatch(v.matches[max(0, min(v.sel, len(v.matches)-1))])
}

func (m *Model) chooseProjectNewMatch(match projectNewMatch) {
	v := m.projectNew
	if v == nil {
		return
	}
	switch match.entry.action {
	case "directory":
		m.projectNew = nil
		m.OpenNewTreeEntry(v.dir, true)
	case "scratch":
		m.projectNew = nil
		m.OpenScratchChooser()
	case "file", "fromTemplate":
		m.projectNew = nil
		m.OpenNewTreeEntry(v.dir, false)
	default:
		m.SelectProjectNewTemplate(match.entry.templateIndex)
	}
}

func (m *Model) projectNewDialog(c *ui.Context, p *ui.Theme) {
	v := m.projectNew
	if v == nil {
		return
	}
	oi := oiPaletteOf(p)
	width, height := c.Size()
	popupWidth := float32(280)
	rowCount := len(v.matches)
	separatorCount := 0
	if strings.TrimSpace(v.query) == "" {
		for _, match := range v.matches {
			if match.entry.separator {
				separatorCount++
			}
		}
	}
	popupHeight := min(float32(420), float32(43+rowCount*24+separatorCount*9))
	if rowCount == 0 {
		popupHeight = 73
	}
	x, y := width/2-popupWidth/2, height/5
	if v.anchor.W > 0 {
		x = v.anchor.X + 24
		y = v.anchor.Y + v.anchor.H
	} else if m.projectTreeAnchor.W > 0 {
		x = m.projectTreeAnchor.X + 24
		y = m.projectTreeAnchor.Y + m.projectTreeAnchor.H
	}
	x = max(8, min(x, width-popupWidth-8))
	if y+popupHeight > height-8 {
		if v.anchor.Y > popupHeight+8 {
			y = v.anchor.Y - popupHeight
		} else {
			y = max(8, height-popupHeight-8)
		}
	}
	ui.Overlay(c, func() {
		box := ui.Column(c.Key("project:new-popup")).Role(ui.RolePopup).Label("New").
			Absolute().Left(x).Top(y).Width(popupWidth).Height(popupHeight).
			Background(oi.popup).Border(1, oi.borderStrong).Radius(7).
			Shadow(0, 10, 24, 0, ui.RGBA(0, 0, 0, 0.28)).Clip().Children(func() {
			ui.Row(c).Height(36).Shrink(0).Padding(0, 10).Gap(6).
				AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).
				BorderColor(oi.border).Children(func() {
				oiIconEl(c, "general/search", 14, oi, oi.textMuted)
				in := ui.TextInput(c, &v.query).Grow(1).AutoFocus().
					Placeholder("Search for file type")
				in.OnChange(func() {
					v.sel = 0
					m.refreshProjectNewMatches()
				})
				in.OnShortcut(0, ui.KeyUp, func() { m.StepProjectNew(-1) })
				in.OnShortcut(0, ui.KeyDown, func() { m.StepProjectNew(1) })
				in.OnShortcut(0, ui.KeyPageUp, func() { m.StepProjectNew(-8) })
				in.OnShortcut(0, ui.KeyPageDown, func() { m.StepProjectNew(8) })
				in.OnShortcut(0, ui.KeyEscape, m.CloseProjectNew)
				if in.Submitted() {
					m.selectCurrentProjectNewTemplate()
				}
			})
			ui.Row(c).Height(3).Shrink(0)
			if len(v.matches) == 0 {
				ui.Text(c, "Nothing found").FontSize(11).TextColor(oi.textFaint).
					Padding(6, 10).SingleLine()
				return
			}
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Padding(0, 4).Children(func() {
					for row := range v.matches {
						row := row
						match := v.matches[row]
						if match.entry.separator && strings.TrimSpace(v.query) == "" {
							ui.Row(c).Height(9).Padding(0, 8).Children(func() {
								ui.Row(c).Height(1).Grow(1).Background(oi.border)
							})
						}
						item := ui.Row(c.Key("project-new:"+match.entry.key)).
							Height(24).Padding(0, 8).Gap(8).Radius(4).
							AlignItems(ui.Center).Cursor(ui.CursorPointer)
						if row == v.sel || item.Hovered() {
							item.Background(oi.selected)
						}
						item.Children(func() {
							if match.entry.action == "directory" {
								folderIconEl(c, oi, "plain", 16, oi.textMuted)
							} else {
								name := match.entry.label
								scheme := m.effectiveFileTemplateScheme()
								if match.entry.templateIndex >= 0 &&
									match.entry.templateIndex < len(scheme.Templates) {
									ext := projectNewTemplateExtension(
										scheme.Templates[match.entry.templateIndex], 0,
									)
									if ext != "" {
										name = "file." + ext
									}
								}
								fileIconEl(c, oi, name, 16, oi.textMuted)
							}
							spans := projectNewHighlightSpans(match.entry.label, match.highlights, oi)
							ui.RichText(c, spans...).FontSize(12).SingleLine().
								Ellipsis("…").Grow(1)
						})
						if item.Hovered() && v.sel != row {
							v.sel = row
						}
						if item.Clicked() {
							v.sel = row
							m.chooseProjectNewMatch(match)
						}
					}
				})
			})
		})
		if box.PressedOutside() {
			m.CloseProjectNew()
		}
	})
}

func projectNewHighlightSpans(text string, indices []int, oi oiColors) []ui.Span {
	if len(indices) == 0 {
		return []ui.Span{{Text: text, Color: oi.textStrong}}
	}
	matched := make(map[int]bool, len(indices))
	for _, index := range indices {
		matched[index] = true
	}
	spans := make([]ui.Span, 0, len(text))
	start := 0
	active := matched[0]
	for i := 1; i < len(text); i++ {
		if matched[i] == active {
			continue
		}
		color := oi.textStrong
		weight := 400
		background := ui.RGBA(0, 0, 0, 0)
		if active {
			color = oi.textStrong
			weight = 700
			background = oi.selected
		}
		spans = append(spans, ui.Span{
			Text: text[start:i], Color: color, Weight: weight,
			Background: background,
		})
		start = i
		active = matched[i]
	}
	color := oi.textStrong
	weight := 400
	background := ui.RGBA(0, 0, 0, 0)
	if active {
		weight = 700
		background = oi.selected
	}
	spans = append(spans, ui.Span{
		Text: text[start:], Color: color, Weight: weight,
		Background: background,
	})
	return spans
}

func projectNewRelative(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func projectNewTemplateExtension(t projectNewTemplate, kindIndex int) string {
	if kindIndex >= 0 && kindIndex < len(t.kinds) && t.kinds[kindIndex].extension != "" {
		return t.kinds[kindIndex].extension
	}
	return t.extension
}

func projectNewKindFor(t *projectNewTemplate, index int) projectNewKind {
	if t == nil || len(t.kinds) == 0 {
		return projectNewKind{}
	}
	index = max(0, min(index, len(t.kinds)-1))
	return t.kinds[index]
}

func projectNewPlanFile(t *projectNewTemplate, kindIndex int, directory, input string) (projectNewPlan, error) {
	if t == nil {
		return projectNewPlan{}, errors.New("Choose a file template")
	}
	kind := projectNewKindFor(t, kindIndex)
	extension := strings.TrimPrefix(projectNewTemplateExtension(*t, kindIndex), ".")
	if t.fixedName != "" {
		if err := projectNewValidateFileName(t.fixedName); err != nil {
			return projectNewPlan{}, err
		}
		dot := strings.LastIndexByte(t.fixedName, '.')
		name := t.fixedName
		if dot > 0 {
			name = t.fixedName[:dot]
		}
		return projectNewPlan{
			path: filepath.Join(directory, t.fixedName), directory: directory,
			name: name, fileName: t.fixedName,
		}, nil
	}
	if kind.id == "" {
		return projectNewPlan{}, errors.New("Choose a file kind")
	}
	raw := strings.TrimSpace(input)
	raw = strings.ReplaceAll(raw, "\\", "/")
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return projectNewPlan{}, errors.New("Enter a name")
	}
	if extension != "" && strings.HasSuffix(strings.ToLower(raw), "."+strings.ToLower(extension)) {
		raw = raw[:len(raw)-len(extension)-1]
	}
	parts := strings.Split(raw, "/")
	clean := parts[:0]
	for _, part := range parts {
		if part != "" {
			clean = append(clean, part)
		}
	}
	parts = clean
	if t.dottedPackages && len(parts) == 1 && strings.Contains(parts[0], ".") {
		parts = strings.Split(parts[0], ".")
	}
	if len(parts) == 0 {
		return projectNewPlan{}, errors.New("Enter a name")
	}
	for _, part := range parts {
		if part == "." || part == ".." || strings.ContainsAny(part, `<>:"|?*`) {
			return projectNewPlan{}, errors.New("The name or folder contains characters that are not allowed")
		}
	}
	leaf := parts[len(parts)-1]
	name := projectNewStyleName(leaf, t.nameStyle)
	if name == "" {
		return projectNewPlan{}, errors.New("Enter a valid name")
	}
	if err := projectNewValidateFileName(name); err != nil {
		return projectNewPlan{}, err
	}
	fileName := name
	if extension != "" {
		fileName += "." + extension
	}
	dir := directory
	if len(parts) > 1 {
		dir = filepath.Join(append([]string{directory}, parts[:len(parts)-1]...)...)
	}
	return projectNewPlan{
		path: filepath.Join(dir, fileName), directory: dir, name: name, fileName: fileName,
	}, nil
}

func projectNewValidateFileName(name string) error {
	if name == "" {
		return errors.New("Enter a name")
	}
	if strings.TrimSpace(name) != name || strings.HasSuffix(name, ".") {
		return errors.New("The name is not valid")
	}
	if strings.ContainsAny(name, `\/<>:"|?*`) || len([]byte(name)) > 255 {
		return errors.New("The name or folder contains characters that are not allowed")
	}
	for _, r := range name {
		if r < 32 || (r >= 127 && r <= 159) {
			return errors.New("The name contains a control character")
		}
	}
	stem := name
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	switch strings.ToLower(stem) {
	case "con", "prn", "aux", "nul", "com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
		"lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return errors.New("That name is reserved by the operating system")
	}
	return nil
}

func projectNewStyleName(name, style string) string {
	switch style {
	case "snake":
		words := projectNewWords(name)
		if len(words) == 0 {
			return name
		}
		for i := range words {
			words[i] = strings.ToLower(words[i])
		}
		return strings.Join(words, "_")
	case "pascal":
		if regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`).MatchString(name) {
			return name
		}
		words := projectNewWords(name)
		if len(words) == 0 {
			return name
		}
		for i, word := range words {
			runes := []rune(strings.ToLower(word))
			if len(runes) > 0 {
				runes[0] = unicode.ToUpper(runes[0])
			}
			words[i] = string(runes)
		}
		return strings.Join(words, "")
	default:
		return name
	}
}

func projectNewWords(value string) []string {
	var words []string
	var word []rune
	runes := []rune(value)
	flush := func() {
		if len(word) > 0 {
			words = append(words, string(word))
			word = word[:0]
		}
	}
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
				flush()
			}
		}
		word = append(word, r)
	}
	flush()
	return words
}

func (m *Model) refreshProjectNewTarget(v *treeOpState) {
	if v == nil || v.projectTemplate == nil {
		return
	}
	plan, err := projectNewPlanFile(v.projectTemplate, v.projectKind, v.path, v.name)
	if err != nil {
		v.projectTarget = ""
		v.projectDir = v.path
		v.projectPackage = ""
		v.projectPackageStyle = ""
		return
	}
	style, pkg := projectNewPackage(v.projectTemplate, v.projectKind, plan.directory, m.Root())
	v.projectTarget = plan.path
	v.projectDir = plan.directory
	v.projectPackageStyle = style
	v.projectPackage = pkg
}

func (m *Model) StepTreeOpProjectKind(delta int) {
	v := m.treeOp
	if v == nil || v.projectTemplate == nil || len(v.projectTemplate.kinds) == 0 {
		return
	}
	v.projectKind = (v.projectKind + delta) % len(v.projectTemplate.kinds)
	if v.projectKind < 0 {
		v.projectKind += len(v.projectTemplate.kinds)
	}
	m.refreshProjectNewTarget(v)
	m.invalidate()
}

func projectNewPackage(t *projectNewTemplate, kindIndex int, directory, root string) (string, string) {
	if t == nil {
		return "", ""
	}
	switch t.packageStyle {
	case "jvm":
		return "package", projectNewInferJVMPackage(directory, root)
	case "go":
		return "package", projectNewInferGoPackage(directory, root)
	case "php":
		return "namespace", projectNewInferPHPNamespace(directory, root)
	default:
		return "", ""
	}
}

func projectNewInferJVMPackage(directory, root string) string {
	rel := directory
	if root != "" {
		if r, err := filepath.Rel(root, directory); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			rel = r
		}
	}
	segs := strings.FieldsFunc(filepath.ToSlash(rel), func(r rune) bool { return r == '/' })
	src := -1
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i] == "src" {
			src = i
			break
		}
	}
	var pkg []string
	if src >= 0 {
		rootIdx := -1
		for i := src + 1; i < len(segs); i++ {
			switch segs[i] {
			case "java", "kotlin", "scala", "groovy":
				rootIdx = i
			}
		}
		if rootIdx >= 0 {
			pkg = segs[rootIdx+1:]
		} else {
			pkg = segs[src+1:]
			if len(pkg) > 0 && (pkg[0] == "main" || pkg[0] == "test" || pkg[0] == "commonMain" ||
				pkg[0] == "commonTest" || pkg[0] == "jvmMain" || pkg[0] == "jvmTest" ||
				pkg[0] == "androidMain" || pkg[0] == "androidTest") {
				pkg = pkg[1:]
			}
		}
	}
	for i := range pkg {
		pkg[i] = strings.ReplaceAll(pkg[i], "-", "_")
		if !projectNewIdentifier(pkg[i]) {
			return ""
		}
	}
	return strings.Join(pkg, ".")
}

func projectNewIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if r != '_' && r != '$' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && r != '$' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func projectNewInferGoPackage(directory, root string) string {
	if entries, err := os.ReadDir(directory); err == nil {
		name := ""
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			if name == "" {
				name = entry.Name()
			}
			if !strings.HasSuffix(entry.Name(), "_test.go") {
				name = entry.Name()
				break
			}
		}
		if name != "" {
			if data, err := os.ReadFile(filepath.Join(directory, name)); err == nil {
				if pkg := projectNewParseGoPackage(string(data)); pkg != "" {
					pkg = strings.TrimSuffix(pkg, "_test")
					return pkg
				}
			}
		}
	}
	rel := directory
	if root != "" {
		if r, err := filepath.Rel(root, directory); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			rel = r
		}
	}
	segs := strings.FieldsFunc(filepath.ToSlash(rel), func(r rune) bool { return r == '/' })
	if len(segs) > 0 {
		for _, seg := range segs[:len(segs)-1] {
			if seg == "cmd" {
				return "main"
			}
		}
	}
	base := strings.ToLower(filepath.Base(directory))
	var cleaned strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			cleaned.WriteRune(r)
		}
	}
	pkg := cleaned.String()
	if pkg == "" || pkg[0] >= '0' && pkg[0] <= '9' {
		return "main"
	}
	return pkg
}

var projectNewGoPackageRE = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_][A-Za-z0-9_]*)`)

func projectNewParseGoPackage(text string) string {
	if match := projectNewGoPackageRE.FindStringSubmatch(text); len(match) > 1 {
		return match[1]
	}
	return ""
}

func projectNewInferPHPNamespace(directory, root string) string {
	dir := directory
	for dir != "" {
		if data, err := os.ReadFile(filepath.Join(dir, "composer.json")); err == nil {
			return projectNewPSR4Namespace(directory, dir, data)
		}
		if root != "" && filepath.Clean(dir) == filepath.Clean(root) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func projectNewPSR4Namespace(directory, composerDir string, data []byte) string {
	var manifest struct {
		Autoload    map[string]json.RawMessage `json:"autoload"`
		AutoloadDev map[string]json.RawMessage `json:"autoload-dev"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return ""
	}
	rel, err := filepath.Rel(composerDir, directory)
	if err != nil {
		return ""
	}
	relSlash := strings.Trim(filepath.ToSlash(rel), "./")
	bestPrefix, bestDir := "", ""
	for _, section := range []map[string]json.RawMessage{manifest.Autoload, manifest.AutoloadDev} {
		raw, ok := section["psr-4"]
		if !ok {
			continue
		}
		var mappings map[string]json.RawMessage
		if json.Unmarshal(raw, &mappings) != nil {
			continue
		}
		for prefix, values := range mappings {
			var one string
			var many []string
			if json.Unmarshal(values, &one) == nil {
				many = []string{one}
			} else if json.Unmarshal(values, &many) != nil {
				continue
			}
			for _, mapped := range many {
				mapped = strings.Trim(filepath.ToSlash(mapped), "./")
				if (mapped == "" || relSlash == mapped || strings.HasPrefix(relSlash, mapped+"/")) && len(mapped) >= len(bestDir) {
					bestPrefix, bestDir = prefix, mapped
				}
			}
		}
	}
	if bestPrefix == "" && bestDir == "" {
		return ""
	}
	parts := strings.FieldsFunc(bestPrefix, func(r rune) bool { return r == '\\' })
	if bestDir != "" && relSlash != bestDir {
		parts = append(parts, strings.Split(strings.TrimPrefix(relSlash, bestDir+"/"), "/")...)
	} else if bestDir != "" && relSlash == bestDir {
		parts = strings.FieldsFunc(bestPrefix, func(r rune) bool { return r == '\\' })
	}
	for _, part := range parts {
		if !projectNewIdentifier(part) {
			return ""
		}
	}
	return strings.Join(parts, "\\")
}

func projectNewExpandedBody(body string) string {
	return projectNewExpandedBodyWithIncludes(body, projectNewIncludes)
}

func projectNewExpandedBodyWithIncludes(body string, includes map[string]string) string {
	return projectNewExpandIncludes(strings.ReplaceAll(body, "\r\n", "\n"), includes, 0)
}

var projectNewParseLineRE = regexp.MustCompile(`^([ \t]*)#parse\(\s*"([^"]+)"\s*\)[ \t]*$`)
var projectNewParseInlineRE = regexp.MustCompile(`#parse\(\s*"([^"]+)"\s*\)`)

func projectNewExpandIncludes(body string, includes map[string]string, depth int) string {
	if depth > 4 || !strings.Contains(body, "#parse") {
		return body
	}
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if match := projectNewParseLineRE.FindStringSubmatch(line); match != nil {
			include, ok := includes[match[2]]
			if !ok {
				out = append(out, line)
				continue
			}
			include = strings.TrimSuffix(strings.ReplaceAll(include, "\r\n", "\n"), "\n")
			include = projectNewExpandIncludes(include, includes, depth+1)
			if include == "" {
				continue
			}
			for _, includeLine := range strings.Split(include, "\n") {
				if includeLine != "" {
					includeLine = match[1] + includeLine
				}
				out = append(out, includeLine)
			}
			continue
		}
		line = projectNewParseInlineRE.ReplaceAllStringFunc(line, func(directive string) string {
			match := projectNewParseInlineRE.FindStringSubmatch(directive)
			include, ok := includes[match[1]]
			if !ok {
				return directive
			}
			include = strings.TrimSuffix(strings.ReplaceAll(include, "\r\n", "\n"), "\n")
			return projectNewExpandIncludes(include, includes, depth+1)
		})
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func renderFileTemplate(body string, vars projectNewVars) (string, int) {
	return renderFileTemplateWithIncludes(body, vars, projectNewIncludes)
}

func renderFileTemplateWithIncludes(body string, vars projectNewVars, includes map[string]string) (string, int) {
	if vars.now.IsZero() {
		vars.now = time.Now()
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = projectNewExpandedBodyWithIncludes(body, includes)
	lines := strings.Split(body, "\n")
	if vars.pkg == "" {
		clean := make([]string, 0, len(lines))
		for i := 0; i < len(lines); i++ {
			if strings.Contains(lines[i], "$PACKAGE$") || strings.Contains(lines[i], "${PACKAGE}") ||
				strings.Contains(lines[i], "$PACKAGE_NAME$") || strings.Contains(lines[i], "${PACKAGE_NAME}") {
				if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
					i++
				}
				continue
			}
			clean = append(clean, lines[i])
		}
		lines = clean
	}
	values := map[string]string{
		"NAME": vars.name, "FILE_NAME": vars.fileName, "PACKAGE": vars.pkg,
		"PACKAGE_NAME": vars.pkg, "PROJECT_NAME": vars.projectName, "USER": vars.user,
		"DATE": vars.now.Format("2006-01-02"), "TIME": vars.now.Format("15:04"),
		"YEAR": strconv.Itoa(vars.now.Year()),
	}
	src := strings.Join(lines, "\n")
	var out strings.Builder
	caret := -1
	for i := 0; i < len(src); {
		if src[i] != '$' {
			out.WriteByte(src[i])
			i++
			continue
		}
		if i+1 < len(src) && src[i+1] == '$' {
			out.WriteByte('$')
			i += 2
			continue
		}
		braced := i+1 < len(src) && src[i+1] == '{'
		close := -1
		if braced {
			if end := strings.IndexByte(src[i+2:], '}'); end >= 0 {
				close = i + 2 + end
			}
		} else if end := strings.IndexByte(src[i+1:], '$'); end >= 0 {
			close = i + 1 + end
		}
		if close < 0 {
			out.WriteByte(src[i])
			i++
			continue
		}
		keyStart := i + 1
		if braced {
			keyStart++
		}
		key := src[keyStart:close]
		if key == "END" {
			if caret < 0 {
				caret = out.Len()
			}
			i = close + 1
			continue
		}
		if value, ok := values[key]; ok {
			out.WriteString(value)
		} else {
			out.WriteString(src[i : close+1])
		}
		i = close + 1
	}
	text := out.String()
	if caret < 0 {
		caret = len(text)
	}
	return text, caret
}

func projectNewUserName(root string) string {
	if match := regexp.MustCompile(`^/(?:Users|home)/([^/]+)`).FindStringSubmatch(filepath.ToSlash(root)); len(match) > 1 {
		return match[1]
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Base(home)
	}
	return ""
}

func projectNewRenderedText(t *projectNewTemplate, kindIndex int, plan projectNewPlan, packageName, projectName, root string, now time.Time) (string, int) {
	return projectNewRenderedTextWithIncludes(t, kindIndex, plan, packageName, projectName, root, now, projectNewIncludes)
}

func projectNewRenderedTextWithIncludes(t *projectNewTemplate, kindIndex int, plan projectNewPlan, packageName, projectName, root string, now time.Time, includes map[string]string) (string, int) {
	kind := projectNewKindFor(t, kindIndex)
	return renderFileTemplateWithIncludes(kind.body, projectNewVars{
		name: plan.name, fileName: plan.fileName, pkg: packageName,
		projectName: projectName, user: projectNewUserName(root), now: now,
	}, includes)
}

func projectNewCaretPosition(text string, offset int) (line, col int) {
	offset = max(0, min(offset, len(text)))
	before := text[:offset]
	line = strings.Count(before, "\n")
	if nl := strings.LastIndexByte(before, '\n'); nl >= 0 {
		col = offset - nl - 1
	} else {
		col = offset
	}
	return line, col
}

func projectNewByteValues(text string) []any {
	out := make([]any, len(text))
	for i := 0; i < len(text); i++ {
		out[i] = int(text[i])
	}
	return out
}
