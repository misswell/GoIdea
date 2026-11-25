package app

// Add Dependency is GoIDEA's native Maven adaptation. It edits the shared
// pom.xml document through docbridge, saves it, and lets build_sync resolve the
// new tree. The edit is version-guarded so unsaved user text is never replaced.

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
)

type addMavenDependencyDialog struct {
	open       bool
	pom        string
	group      string
	artifact   string
	version    string
	scope      string
	optional   bool
	err        string
	busy       bool
	generation uint64
}

type mavenDependencyInput struct {
	group    string
	artifact string
	version  string
	scope    string
	optional bool
}

func (m *Model) OpenAddMavenDependency(pom string) {
	pom = strings.TrimSpace(pom)
	var project *mavenProject
	if pom != "" {
		project = m.mavenProjectByPom(pom)
	}
	if project == nil && m.maven.sel.project != nil {
		project = m.maven.sel.project
	}
	if project == nil {
		project = m.mavenRootProject()
	}
	if project != nil && pom == "" {
		pom = project.Pom
	}
	if pom == "" {
		m.status = "Select a Maven module before adding a dependency"
		m.invalidate()
		return
	}
	m.maven.addDependency = &addMavenDependencyDialog{
		open: true, pom: pom, scope: "compile",
	}
	m.invalidate()
}

func (m *Model) CloseAddMavenDependency() {
	a := m.maven.addDependency
	if a != nil {
		a.generation++
	}
	m.maven.addDependency = nil
	m.invalidate()
}

func (m *Model) addMavenDependencyDialog(c *ui.Context, p *ui.Theme) {
	a := m.maven.addDependency
	if a == nil {
		return
	}
	open := a.open
	dialog := nativeModal(c, &open, func() {
		ui.Column(c).Width(500).Gap(8).Children(func() {
			ui.Text(c, "Add Maven Dependency").FontSize(14).Bold().TextColor(p.Text).SingleLine()
			ui.Text(c, a.pom).FontSize(11).TextColor(p.TextMuted).SingleLine().Ellipsis("…")
			ui.Text(c, "Group ID").FontSize(11).TextColor(p.TextMuted)
			group := ui.TextInput(c, &a.group).Placeholder("org.example").AutoFocus()
			group.OnChange(func() { a.err = "" })
			ui.Text(c, "Artifact ID").FontSize(11).TextColor(p.TextMuted)
			artifact := ui.TextInput(c, &a.artifact).Placeholder("example-lib")
			artifact.OnChange(func() { a.err = "" })
			ui.Text(c, "Version").FontSize(11).TextColor(p.TextMuted)
			version := ui.TextInput(c, &a.version).Placeholder("1.0.0 (optional when managed)")
			version.OnChange(func() { a.err = "" })
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Scope").FontSize(11).TextColor(p.TextMuted)
				scope := a.scope
				scopes := []string{"compile", "provided", "runtime", "test"}
				if ui.Select(c, &scope, scopes).Changed() {
					a.scope = scope
				}
				ui.Checkbox(c, &a.optional, "Optional")
			})
			if a.err != "" {
				ui.Text(c, a.err).FontSize(11).TextColor(p.Danger).MaxWidth(480)
			}
			ui.Row(c).Gap(8).Children(func() {
				cancel := ui.Button(c, "Cancel")
				cancel.Disabled(a.busy)
				cancel.OnClick(m.CloseAddMavenDependency)
				add := ui.Button(c, "Add")
				add.Disabled(a.busy)
				add.OnClick(func() { m.saveMavenDependencyDialog(a) })
			})
		})
	})
	if dialog.Dismissed() || !open {
		m.CloseAddMavenDependency()
	}
}

func (m *Model) saveMavenDependencyDialog(a *addMavenDependencyDialog) {
	if a == nil || a.busy || m.maven.addDependency != a {
		return
	}
	input := mavenDependencyInput{
		group: strings.TrimSpace(a.group), artifact: strings.TrimSpace(a.artifact),
		version: strings.TrimSpace(a.version), scope: strings.TrimSpace(a.scope),
		optional: a.optional,
	}
	if input.group == "" || input.artifact == "" {
		a.err = "Group ID and Artifact ID are required"
		m.invalidate()
		return
	}
	if strings.ContainsAny(input.group+input.artifact+input.version+input.scope, "<>&") {
		a.err = "A dependency field cannot contain XML markup"
		m.invalidate()
		return
	}
	project := m.mavenProjectByPom(a.pom)
	if project == nil {
		a.err = "The selected Maven module is no longer linked"
		m.invalidate()
		return
	}
	path := m.mavenPOMPath(project)
	if path == "" {
		a.err = "The module has no pom.xml path"
		m.invalidate()
		return
	}
	a.busy = true
	a.err = ""
	a.generation++
	generation := a.generation
	m.invalidate()
	if tab := m.tabForPath(path); tab >= 0 {
		m.applyMavenDependencyEdit(a, generation, m.tabs[tab], input)
		return
	}
	if m.reg == nil {
		a.busy = false
		a.err = "The document registry is unavailable"
		m.invalidate()
		return
	}
	m.openFileThen(path, func(tab *Tab) {
		if m.maven.addDependency != a || a.generation != generation {
			return
		}
		m.applyMavenDependencyEdit(a, generation, tab, input)
	})
}

func (m *Model) applyMavenDependencyEdit(
	a *addMavenDependencyDialog,
	generation uint64,
	tab *Tab,
	input mavenDependencyInput,
) {
	if a == nil || tab == nil || tab.doc == nil || m.maven.addDependency != a || a.generation != generation {
		return
	}
	if tab.doc.ReadOnly() {
		a.busy = false
		a.err = "pom.xml is read-only"
		m.invalidate()
		return
	}
	text := tab.doc.Buffer().Snapshot().String()
	at, inserted, err := insertMavenDependency(text, input)
	if err != nil {
		a.busy = false
		a.err = err.Error()
		m.invalidate()
		return
	}
	version := tab.doc.Buffer().Snapshot().Version()
	doc := tab.doc
	pom := a.pom
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		err := doc.ApplyEditsIfVersion(ctx, version, editor.Edit{
			Start: at, End: at, Text: inserted,
		})
		if err == nil {
			err = doc.Save(ctx)
		}
		m.post(func() {
			if m.maven.addDependency != a || a.generation != generation {
				return
			}
			a.busy = false
			if err != nil {
				if errors.Is(err, docbridge.ErrChanged) {
					a.err = "pom.xml changed while this dialog was open. Review the file and try again."
				} else {
					a.err = "Cannot add the dependency: " + err.Error()
				}
				m.invalidate()
				return
			}
			m.maven.addDependency = nil
			m.status = "Added " + input.group + ":" + input.artifact + " to " + pom
			m.SyncMaven()
			m.invalidate()
		})
	}()
}

func insertMavenDependency(text string, input mavenDependencyInput) (int, string, error) {
	if text == "" {
		return 0, "", fmt.Errorf("pom.xml is empty")
	}
	dependenciesStartAfter := -1
	dependenciesClose := -1
	var stack []string
	decoder := xml.NewDecoder(strings.NewReader(text))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, "", fmt.Errorf("pom.xml is not valid XML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			stack = append(stack, value.Name.Local)
			if value.Name.Local == "dependencies" && parent == "project" && dependenciesStartAfter < 0 {
				dependenciesStartAfter = int(decoder.InputOffset())
			}
		case xml.EndElement:
			if value.Name.Local == "dependencies" && dependenciesStartAfter >= 0 && dependenciesClose < 0 {
				end := int(decoder.InputOffset())
				segment := text[dependenciesStartAfter:end]
				if index := strings.LastIndex(segment, "</dependencies"); index >= 0 {
					dependenciesClose = dependenciesStartAfter + index
				}
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if mavenDependencyAlreadyDeclared(text, input.group, input.artifact) {
		return 0, "", fmt.Errorf("%s:%s is already declared in pom.xml", input.group, input.artifact)
	}
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	if dependenciesStartAfter >= 0 && dependenciesClose >= 0 {
		openingLine := lineStartAt(text, dependenciesStartAfter)
		indent := leadingWhitespace(text[openingLine:dependenciesStartAfter])
		childIndent := indent + "    "
		fieldIndent := childIndent + "    "
		closeLine := lineStartAt(text, dependenciesClose)
		closeOnOwnLine := strings.TrimSpace(text[closeLine:dependenciesClose]) == ""
		insertAt := dependenciesClose
		if closeOnOwnLine {
			insertAt = closeLine
		} else {
			insertAt = dependenciesClose
		}
		var block strings.Builder
		if !closeOnOwnLine {
			block.WriteString(newline)
		}
		writeMavenDependencyXML(&block, input, newline, childIndent, fieldIndent)
		block.WriteString(newline)
		if !closeOnOwnLine {
			block.WriteString(indent)
		}
		return insertAt, block.String(), nil
	}
	projectClose := strings.LastIndex(text, "</project>")
	if projectClose < 0 {
		return 0, "", fmt.Errorf("pom.xml has no closing </project>")
	}
	projectCloseLine := lineStartAt(text, projectClose)
	projectIndent := leadingWhitespace(text[projectCloseLine:])
	dependenciesIndent := projectIndent + "    "
	childIndent := dependenciesIndent + "    "
	fieldIndent := childIndent + "    "
	closeOnOwnLine := strings.TrimSpace(text[projectCloseLine:projectClose]) == ""
	insertAt := projectClose
	if closeOnOwnLine {
		insertAt = projectCloseLine
	}
	var block strings.Builder
	if !closeOnOwnLine {
		block.WriteString(newline)
	}
	block.WriteString(dependenciesIndent + "<dependencies>" + newline)
	writeMavenDependencyXML(&block, input, newline, childIndent, fieldIndent)
	block.WriteString(newline)
	block.WriteString(dependenciesIndent + "</dependencies>" + newline)
	if !closeOnOwnLine {
		block.WriteString(projectIndent)
	}
	return insertAt, block.String(), nil
}

func writeMavenDependencyXML(
	builder *strings.Builder,
	input mavenDependencyInput,
	newline, dependencyIndent, fieldIndent string,
) {
	builder.WriteString(dependencyIndent + "<dependency>" + newline)
	builder.WriteString(fieldIndent + "<groupId>" + input.group + "</groupId>" + newline)
	builder.WriteString(fieldIndent + "<artifactId>" + input.artifact + "</artifactId>" + newline)
	if input.version != "" {
		builder.WriteString(fieldIndent + "<version>" + input.version + "</version>" + newline)
	}
	if input.scope != "" && input.scope != "compile" {
		builder.WriteString(fieldIndent + "<scope>" + input.scope + "</scope>" + newline)
	}
	if input.optional {
		builder.WriteString(fieldIndent + "<optional>true</optional>" + newline)
	}
	builder.WriteString(dependencyIndent + "</dependency>")
}

func mavenDependencyAlreadyDeclared(text, group, artifact string) bool {
	decoder := xml.NewDecoder(strings.NewReader(text))
	var stack []string
	var currentGroup, currentArtifact string
	inProjectDependencies := false
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		switch value := token.(type) {
		case xml.StartElement:
			stack = append(stack, value.Name.Local)
			if value.Name.Local == "dependency" &&
				len(stack) == 3 &&
				stack[0] == "project" &&
				stack[1] == "dependencies" {
				inProjectDependencies = true
				currentGroup, currentArtifact = "", ""
			}
		case xml.CharData:
			if !inProjectDependencies || len(stack) < 2 {
				continue
			}
			parent := ""
			if len(stack) > 1 {
				parent = stack[len(stack)-2]
			}
			if parent == "dependency" {
				switch stack[len(stack)-1] {
				case "groupId":
					currentGroup = strings.TrimSpace(string(value))
				case "artifactId":
					currentArtifact = strings.TrimSpace(string(value))
				}
			}
		case xml.EndElement:
			if value.Name.Local == "dependency" && inProjectDependencies &&
				currentGroup == strings.TrimSpace(group) &&
				currentArtifact == strings.TrimSpace(artifact) {
				return true
			}
			if value.Name.Local == "dependency" && inProjectDependencies {
				inProjectDependencies = false
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func lineStartAt(text string, at int) int {
	if at <= 0 {
		return 0
	}
	if at > len(text) {
		at = len(text)
	}
	if index := strings.LastIndex(text[:at], "\n"); index >= 0 {
		return index + 1
	}
	return 0
}

func leadingWhitespace(text string) string {
	index := 0
	for index < len(text) && (text[index] == ' ' || text[index] == '\t') {
		index++
	}
	return text[:index]
}
