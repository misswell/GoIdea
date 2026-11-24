package app

// Gradle version catalog editor contribution, after
// build-tools/versionCatalog.ts. Build scripts navigate from `libs.*`
// accessors to the catalog entry, and catalog files navigate from
// `version.ref` and bundle members to the entries they name. Catalog text is
// read through OpenIDEA's fs_read_text and published catalogs through
// gradle_published_catalog, so remote projects use the same backend path.

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

type gradleCatalogEntry struct {
	Section string
	Alias   string
	Line    int
	Col     int
	Value   string
}

type gradleVersionCatalog struct {
	Name    string
	Path    string
	Entries []gradleCatalogEntry
}

type gradleCatalogDeclaration struct {
	Name     string
	File     string
	Artifact string
}

type gradleVersionCatalogChain struct {
	From     int
	To       int
	Segments []string
}

type gradleVersionCatalogHit struct {
	Catalog gradleVersionCatalog
	Entry   gradleCatalogEntry
	From    int
	To      int
}

type gradleVersionCatalogDownload struct {
	Root     string
	Catalogs []gradleVersionCatalog
	Ready    bool
	Loading  bool
	Gen      uint64
	Err      string
	Waiters  []func(*gradleVersionCatalogDownload)
}

type gradleVersionCatalogState struct {
	generation uint64
	downloads  map[string]*gradleVersionCatalogDownload
}

var (
	gradleCatalogHeaderRE = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*(#.*)?$`)
	gradleCatalogKeyRE    = regexp.MustCompile(
		`^(\s*)(?:"([^"]+)"|'([^']+)'|([A-Za-z0-9_-]+))[^=]*?=\s*(.*)$`,
	)
	gradleCatalogDeclarationRE = regexp.MustCompile(
		`create\(\s*["'](\w+)["']\s*\)\s*\{[^}]*?from\(\s*(?:files\(\s*["']([^"']+)["']|["']([^"':]+:[^"':]+:[^"']+)["'])`,
	)
	gradleCatalogStringRE   = regexp.MustCompile(`"([^"]*)"|'([^']*)'`)
	gradleCatalogRefRE      = regexp.MustCompile(`\bref\s*=\s*$`)
	gradleCatalogEntryRefRE = regexp.MustCompile(
		`\bref\s*=\s*["']([^"']+)["']`,
	)
)

func isGradleVersionCatalogPath(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".versions.toml")
}

// gradleVersionCatalogPath also recognizes generated catalogs such as
// platform-catalog-1.4.0.toml after the backend has resolved them.
func (m *Model) gradleVersionCatalogPath(value string) bool {
	if isGradleVersionCatalogPath(value) {
		return true
	}
	if m == nil {
		return false
	}
	for _, download := range m.versionCatalogs.downloads {
		if download == nil {
			continue
		}
		for _, catalog := range download.Catalogs {
			if catalog.Path == value {
				return true
			}
		}
	}
	return false
}

func isGradleBuildScriptPath(value string) bool {
	value = strings.ToLower(strings.ReplaceAll(value, `\`, "/"))
	return strings.HasSuffix(value, ".gradle") || strings.HasSuffix(value, ".gradle.kts")
}

func gradleCatalogSourceChanged(event openidea.FsChangedEvent) bool {
	for _, value := range uniqueChangedPaths(
		event.Created,
		event.Modified,
		event.Removed,
	) {
		value = strings.ReplaceAll(value, `\`, "/")
		base := path.Base(value)
		if isGradleVersionCatalogPath(value) ||
			base == "settings.gradle" ||
			base == "settings.gradle.kts" {
			return true
		}
	}
	return false
}

func parseGradleVersionCatalog(name, path, text string) gradleVersionCatalog {
	catalog := gradleVersionCatalog{Name: name, Path: path}
	section := ""
	depth := 0
	for lineNumber, raw := range strings.Split(text, "\n") {
		if depth == 0 {
			if match := gradleCatalogHeaderRE.FindStringSubmatch(raw); match != nil {
				section = strings.TrimSpace(match[1])
				switch section {
				case "versions", "libraries", "bundles", "plugins":
				default:
					section = ""
				}
				continue
			}
		}
		if section != "" && depth == 0 {
			if match := gradleCatalogKeyRE.FindStringSubmatch(raw); match != nil {
				alias := match[2]
				if alias == "" {
					alias = match[3]
				}
				if alias == "" {
					alias = match[4]
				}
				col := len(match[1])
				if match[4] == "" {
					col++
				}
				if alias != "" && !gradleCatalogHasEntry(catalog, section, alias) {
					catalog.Entries = append(catalog.Entries, gradleCatalogEntry{
						Section: section,
						Alias:   alias,
						Line:    lineNumber,
						Col:     col,
						Value:   stripGradleCatalogComment(match[5]),
					})
				}
			}
		}
		depth += gradleCatalogDepthDelta(raw)
		if depth < 0 {
			depth = 0
		}
	}
	return catalog
}

func gradleCatalogHasEntry(catalog gradleVersionCatalog, section, alias string) bool {
	for _, entry := range catalog.Entries {
		if entry.Section == section && entry.Alias == alias {
			return true
		}
	}
	return false
}

func stripGradleCatalogComment(value string) string {
	quoted := byte(0)
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '"', '\'':
			if quoted == 0 {
				quoted = value[i]
			} else if quoted == value[i] {
				quoted = 0
			}
		case '#':
			if quoted == 0 {
				return strings.TrimSpace(value[:i])
			}
		}
	}
	return strings.TrimSpace(value)
}

func gradleCatalogDepthDelta(line string) int {
	delta := 0
	quote := byte(0)
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"', '\'':
			if quote == 0 {
				quote = line[i]
			} else if quote == line[i] {
				quote = 0
			}
		case '#':
			if quote == 0 {
				return delta
			}
		case '[', '{':
			if quote == 0 {
				delta++
			}
		case ']', '}':
			if quote == 0 {
				delta--
			}
		}
	}
	return delta
}

func gradleCatalogAccessorSegments(alias string) []string {
	return strings.FieldsFunc(alias, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
}

func gradleCatalogEntryFor(
	catalog gradleVersionCatalog,
	segments []string,
) (gradleCatalogEntry, bool) {
	if len(segments) == 0 {
		return gradleCatalogEntry{}, false
	}
	section := "libraries"
	entryPath := segments
	switch segments[0] {
	case "versions", "bundles", "plugins":
		section = segments[0]
		entryPath = segments[1:]
	}
	for length := len(entryPath); length > 0; length-- {
		want := strings.Join(entryPath[:length], ".")
		for _, entry := range catalog.Entries {
			if entry.Section == section &&
				strings.Join(gradleCatalogAccessorSegments(entry.Alias), ".") == want {
				return entry, true
			}
		}
	}
	return gradleCatalogEntry{}, false
}

func gradleCatalogChainAt(line string, pos int) (gradleVersionCatalogChain, bool) {
	if pos < 0 || pos > len(line) {
		return gradleVersionCatalogChain{}, false
	}
	isWord := func(value byte) bool {
		return value == '_' ||
			value >= 'a' && value <= 'z' ||
			value >= 'A' && value <= 'Z' ||
			value >= '0' && value <= '9'
	}
	from := pos
	for from > 0 {
		before := line[from-1]
		if isWord(before) {
			from--
			continue
		}
		if before == '.' && from >= 2 && isWord(line[from-2]) {
			from--
			continue
		}
		break
	}
	to := pos
	for to < len(line) {
		at := line[to]
		if isWord(at) {
			to++
			continue
		}
		if at == '.' && to+1 < len(line) && isWord(line[to+1]) {
			to++
			continue
		}
		break
	}
	if to <= from || !(line[from] == '_' ||
		line[from] >= 'a' && line[from] <= 'z' ||
		line[from] >= 'A' && line[from] <= 'Z') {
		return gradleVersionCatalogChain{}, false
	}
	segments := strings.Split(line[from:to], ".")
	if len(segments) < 2 {
		return gradleVersionCatalogChain{}, false
	}
	return gradleVersionCatalogChain{From: from, To: to, Segments: segments}, true
}

func gradleCatalogNameCandidate(name string) bool {
	name = strings.ToLower(name)
	return name == "libs" || strings.HasSuffix(name, "libs")
}

// gradleCatalogCompletionAt accepts the incomplete accessor typed while the
// user is editing, including a trailing dot. The completed replacement always
// spans the whole catalog chain so a custom catalog name works unchanged.
func gradleCatalogCompletionAt(
	line string,
	pos int,
) (from, to int, name, typed string, ok bool) {
	if pos < 0 {
		return 0, 0, "", "", false
	}
	pos = min(pos, len(line))
	isWord := func(value byte) bool {
		return value == '_' ||
			value >= 'a' && value <= 'z' ||
			value >= 'A' && value <= 'Z' ||
			value >= '0' && value <= '9'
	}
	from = pos
	for from > 0 {
		before := line[from-1]
		if isWord(before) {
			from--
			continue
		}
		if before == '.' && from >= 2 && isWord(line[from-2]) {
			from--
			continue
		}
		break
	}
	to = pos
	for to < len(line) {
		at := line[to]
		if isWord(at) {
			to++
			continue
		}
		if at == '.' && to+1 < len(line) && isWord(line[to+1]) {
			to++
			continue
		}
		break
	}
	raw := line[from:pos]
	segments := strings.Split(raw, ".")
	if len(segments) < 2 || segments[0] == "" ||
		!(segments[0][0] == '_' ||
			segments[0][0] >= 'a' && segments[0][0] <= 'z' ||
			segments[0][0] >= 'A' && segments[0][0] <= 'Z') ||
		!gradleCatalogNameCandidate(segments[0]) {
		return 0, 0, "", "", false
	}
	return from, to, segments[0], strings.Join(segments[1:], "."), true
}

func declaredGradleVersionCatalogs(settings string) []gradleCatalogDeclaration {
	var out []gradleCatalogDeclaration
	for _, match := range gradleCatalogDeclarationRE.FindAllStringSubmatch(settings, -1) {
		declaration := gradleCatalogDeclaration{Name: match[1]}
		if match[2] != "" {
			declaration.File = match[2]
		} else {
			declaration.Artifact = match[3]
		}
		out = append(out, declaration)
	}
	return out
}

func gradleCatalogJoin(base, rel string) string {
	rel = strings.ReplaceAll(rel, `\`, "/")
	if strings.HasPrefix(rel, "/") || len(rel) >= 3 &&
		((rel[0] >= 'A' && rel[0] <= 'Z') || (rel[0] >= 'a' && rel[0] <= 'z')) &&
		rel[1] == ':' && rel[2] == '/' {
		return path.Clean(rel)
	}
	rel = strings.TrimPrefix(rel, "./")
	return path.Clean(path.Join(strings.ReplaceAll(base, `\`, "/"), rel))
}

func gradleCatalogContains(root, candidate string) bool {
	root = strings.TrimSuffix(strings.ReplaceAll(path.Clean(root), `\`, "/"), "/")
	candidate = strings.ReplaceAll(path.Clean(candidate), `\`, "/")
	return candidate == root || strings.HasPrefix(candidate, root+"/")
}

type gradleCatalogReader func(string) (string, bool, error)
type gradleCatalogPublisher func(string) (string, bool, error)

func readGradleVersionCatalogs(
	script, root string,
	read gradleCatalogReader,
	published gradleCatalogPublisher,
) ([]gradleVersionCatalog, bool, error) {
	script = strings.ReplaceAll(path.Clean(script), `\`, "/")
	root = strings.ReplaceAll(path.Clean(root), `\`, "/")
	dir := path.Dir(script)
	settings := ""
	settingsFound := false
	for {
		for _, name := range []string{"settings.gradle.kts", "settings.gradle"} {
			text, ok, err := read(path.Join(dir, name))
			if err != nil {
				return nil, false, err
			}
			if ok {
				settings = text
				settingsFound = true
				break
			}
		}
		if settingsFound || dir == root || !gradleCatalogContains(root, dir) {
			break
		}
		parent := path.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	buildRoot := root
	if settingsFound {
		buildRoot = dir
	}
	declarations := declaredGradleVersionCatalogs(settings)
	hasLibs := false
	for _, declaration := range declarations {
		if declaration.Name == "libs" {
			hasLibs = true
			break
		}
	}
	if !hasLibs {
		declarations = append(declarations, gradleCatalogDeclaration{
			Name: "libs",
			File: "gradle/libs.versions.toml",
		})
	}

	catalogs := make([]gradleVersionCatalog, 0, len(declarations))
	complete := true
	for _, declaration := range declarations {
		catalogPath := ""
		if declaration.File != "" {
			catalogPath = gradleCatalogJoin(buildRoot, declaration.File)
		} else if published != nil {
			var err error
			catalogPath, _, err = published(declaration.Artifact)
			if err != nil {
				return nil, false, err
			}
		}
		if catalogPath == "" {
			complete = false
			continue
		}
		text, ok, err := read(catalogPath)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			complete = false
			continue
		}
		catalogs = append(catalogs, parseGradleVersionCatalog(
			declaration.Name,
			catalogPath,
			text,
		))
	}
	return catalogs, complete, nil
}

func gradleCatalogSectionAt(lines []string, line int) string {
	for at := min(line, len(lines)-1); at >= 0; at-- {
		if match := gradleCatalogHeaderRE.FindStringSubmatch(lines[at]); match != nil {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}

func gradleCatalogReferenceAt(
	name, path, text string,
	offset int,
) (gradleVersionCatalogHit, bool) {
	if offset < 0 || offset > len(text) {
		return gradleVersionCatalogHit{}, false
	}
	lineStart := strings.LastIndex(text[:offset], "\n") + 1
	lineEnd := strings.IndexByte(text[offset:], '\n')
	if lineEnd < 0 {
		lineEnd = len(text)
	} else {
		lineEnd += offset
	}
	lineText := text[lineStart:lineEnd]
	column := offset - lineStart
	for _, match := range gradleCatalogStringRE.FindAllStringIndex(lineText, -1) {
		if column <= match[0] || column >= match[1] {
			continue
		}
		raw := lineText[match[0]:match[1]]
		alias := strings.Trim(raw, `"'`)
		lines := strings.Split(text, "\n")
		line := strings.Count(text[:lineStart], "\n")
		section := gradleCatalogSectionAt(lines, line)
		ref := gradleCatalogRefRE.MatchString(lineText[:match[0]])
		inBundle := section == "bundles"
		if !ref && !inBundle {
			return gradleVersionCatalogHit{}, false
		}
		targetSection := "libraries"
		if ref {
			targetSection = "versions"
		}
		catalog := parseGradleVersionCatalog(name, path, text)
		for _, entry := range catalog.Entries {
			if entry.Section == targetSection && entry.Alias == alias {
				return gradleVersionCatalogHit{
					Catalog: catalog,
					Entry:   entry,
					From:    lineStart + match[0],
					To:      lineStart + match[1],
				}, true
			}
		}
		return gradleVersionCatalogHit{}, false
	}
	return gradleVersionCatalogHit{}, false
}

func gradleCatalogHoverText(hit gradleVersionCatalogHit) string {
	lines := []string{fmt.Sprintf("%s = %s", hit.Entry.Alias, hit.Entry.Value)}
	if match := gradleCatalogEntryRefRE.FindStringSubmatch(hit.Entry.Value); match != nil {
		for _, entry := range hit.Catalog.Entries {
			if entry.Section == "versions" && entry.Alias == match[1] {
				lines = append(lines, fmt.Sprintf(
					"%s = %s",
					match[1],
					strings.Trim(entry.Value, `"'`),
				))
				break
			}
		}
	}
	lines = append(lines, path.Base(hit.Catalog.Path))
	return strings.Join(lines, "\n\n")
}

func (m *Model) gradleVersionCatalogDefinition(t *Tab, offset int) bool {
	if t == nil || t.doc == nil || !m.tabOpen(t) {
		return false
	}
	value := t.doc.Path()
	if m.gradleVersionCatalogPath(value) {
		hit, ok := gradleCatalogReferenceAt(
			"",
			value,
			t.doc.Buffer().Snapshot().String(),
			offset,
		)
		if !ok {
			return false
		}
		m.revealRange(
			value,
			hit.Entry.Line,
			hit.Entry.Col,
			hit.Entry.Line,
			hit.Entry.Col+len(hit.Entry.Alias),
		)
		return true
	}
	if !isGradleBuildScriptPath(value) {
		return false
	}
	source := t.doc.Buffer().Snapshot().String()
	if offset < 0 || offset > len(source) {
		return false
	}
	lineStart := strings.LastIndex(source[:offset], "\n") + 1
	lineEnd := strings.IndexByte(source[offset:], '\n')
	if lineEnd < 0 {
		lineEnd = len(source)
	} else {
		lineEnd += offset
	}
	chain, ok := gradleCatalogChainAt(source[lineStart:lineEnd], offset-lineStart)
	if !ok {
		return false
	}
	download := m.gradleVersionCatalogDownload(value, chain.Segments[0])
	if download == nil || !download.Ready {
		if !gradleCatalogNameCandidate(chain.Segments[0]) {
			return false
		}
		version := t.doc.Buffer().Version()
		m.waitForGradleVersionCatalogs(value, func(download *gradleVersionCatalogDownload) {
			if !m.tabOpen(t) || t.doc.Path() != value ||
				t.doc.Buffer().Version() != version {
				return
			}
			hit, ok := gradleCatalogHitForChain(download, chain.Segments)
			if !ok {
				return
			}
			m.revealRange(
				hit.Catalog.Path,
				hit.Entry.Line,
				hit.Entry.Col,
				hit.Entry.Line,
				hit.Entry.Col+len(hit.Entry.Alias),
			)
		})
		return true
	}
	if !downloadCoversGradleCatalog(download, chain.Segments[0]) {
		return false
	}
	hit, ok := gradleCatalogHitForChain(download, chain.Segments)
	if !ok {
		return false
	}
	m.revealRange(
		hit.Catalog.Path,
		hit.Entry.Line,
		hit.Entry.Col,
		hit.Entry.Line,
		hit.Entry.Col+len(hit.Entry.Alias),
	)
	return true
}

func (m *Model) gradleVersionCatalogHover(t *Tab, req editor.HoverRequest) bool {
	if t == nil || t.doc == nil || !m.tabOpen(t) || req.Offset < 0 {
		return false
	}
	value := t.doc.Path()
	if m.gradleVersionCatalogPath(value) {
		hit, ok := gradleCatalogReferenceAt(
			"",
			value,
			t.doc.Buffer().Snapshot().String(),
			req.Offset,
		)
		if !ok {
			return false
		}
		t.state.ShowHover(hit.From, hit.To, gradleCatalogHoverText(hit), nil)
		m.invalidate()
		return true
	}
	if !isGradleBuildScriptPath(value) {
		return false
	}
	source := t.doc.Buffer().Snapshot().String()
	if req.Offset > len(source) {
		return false
	}
	lineStart := strings.LastIndex(source[:req.Offset], "\n") + 1
	lineEnd := strings.IndexByte(source[req.Offset:], '\n')
	if lineEnd < 0 {
		lineEnd = len(source)
	} else {
		lineEnd += req.Offset
	}
	chain, ok := gradleCatalogChainAt(source[lineStart:lineEnd], req.Offset-lineStart)
	if !ok {
		return false
	}
	download := m.gradleVersionCatalogDownload(value, chain.Segments[0])
	if download == nil {
		return false
	}
	show := func(download *gradleVersionCatalogDownload) {
		if !m.tabOpen(t) || t.doc.Path() != value ||
			t.doc.Buffer().Version() != req.Version ||
			t.state.Cursor() != req.Offset ||
			!downloadCoversGradleCatalog(download, chain.Segments[0]) {
			return
		}
		hit, ok := gradleCatalogHitForChain(download, chain.Segments)
		if !ok {
			return
		}
		t.state.ShowHover(
			lineStart+chain.From,
			lineStart+chain.To,
			gradleCatalogHoverText(hit),
			nil,
		)
		m.invalidate()
	}
	if !download.Ready {
		if !gradleCatalogNameCandidate(chain.Segments[0]) {
			return false
		}
		m.waitForGradleVersionCatalogs(value, show)
		return true
	}
	if !downloadCoversGradleCatalog(download, chain.Segments[0]) {
		return false
	}
	hit, ok := gradleCatalogHitForChain(download, chain.Segments)
	if !ok {
		return false
	}
	t.state.ShowHover(lineStart+chain.From, lineStart+chain.To, gradleCatalogHoverText(hit), nil)
	m.invalidate()
	return true
}

func downloadCoversGradleCatalog(
	download *gradleVersionCatalogDownload,
	name string,
) bool {
	for _, catalog := range download.Catalogs {
		if catalog.Name == name {
			return true
		}
	}
	return false
}

func gradleCatalogHitForChain(
	download *gradleVersionCatalogDownload,
	segments []string,
) (gradleVersionCatalogHit, bool) {
	for _, catalog := range download.Catalogs {
		if catalog.Name != segments[0] {
			continue
		}
		entry, ok := gradleCatalogEntryFor(catalog, segments[1:])
		if ok {
			return gradleVersionCatalogHit{Catalog: catalog, Entry: entry}, true
		}
	}
	return gradleVersionCatalogHit{}, false
}

func gradleCatalogCompletionItems(
	download *gradleVersionCatalogDownload,
	catalogName, typed string,
	explicit bool,
) []editor.Completion {
	if download == nil {
		return nil
	}
	type candidate struct {
		path   string
		detail string
	}
	var candidates []candidate
	for _, catalog := range download.Catalogs {
		if catalog.Name != catalogName {
			continue
		}
		for _, entry := range catalog.Entries {
			path := strings.Join(gradleCatalogAccessorSegments(entry.Alias), ".")
			if entry.Section != "libraries" {
				path = entry.Section + "." + path
			}
			candidates = append(candidates, candidate{
				path: path,
				detail: fmt.Sprintf(
					"%s · %s",
					entry.Section,
					strings.Trim(entry.Value, `"'`),
				),
			})
		}
	}
	lowerTyped := strings.ToLower(typed)
	seen := map[string]bool{}
	items := make([]editor.Completion, 0, len(candidates))
	for _, candidate := range candidates {
		if !explicit && !strings.HasPrefix(strings.ToLower(candidate.path), lowerTyped) {
			continue
		}
		insert := catalogName + "." + candidate.path
		if seen[insert] {
			continue
		}
		seen[insert] = true
		last := candidate.path
		if dot := strings.LastIndexByte(last, '.'); dot >= 0 {
			last = last[dot+1:]
		}
		items = append(items, editor.Completion{
			Label:      insert,
			Detail:     candidate.detail,
			Kind:       editor.CompletionField,
			Insert:     insert,
			FilterText: last,
		})
	}
	return items
}

func (m *Model) requestGradleVersionCatalogCompletion(
	t *Tab,
	req editor.CompletionRequest,
) bool {
	if t == nil || t.doc == nil || !m.tabOpen(t) ||
		!isGradleBuildScriptPath(t.doc.Path()) {
		return false
	}
	source := t.doc.Buffer().Snapshot().String()
	if req.Offset < 0 || req.Offset > len(source) {
		return false
	}
	lineStart := strings.LastIndex(source[:req.Offset], "\n") + 1
	lineEnd := strings.IndexByte(source[req.Offset:], '\n')
	if lineEnd < 0 {
		lineEnd = len(source)
	} else {
		lineEnd += req.Offset
	}
	from, to, catalogName, typed, ok := gradleCatalogCompletionAt(
		source[lineStart:lineEnd],
		req.Offset-lineStart,
	)
	if !ok {
		return false
	}
	script := t.doc.Path()
	version := t.doc.Buffer().Version()
	show := func(download *gradleVersionCatalogDownload) {
		if !m.tabOpen(t) || t.doc.Path() != script ||
			t.doc.Buffer().Version() != version {
			return
		}
		items := gradleCatalogCompletionItems(
			download,
			catalogName,
			typed,
			req.Explicit,
		)
		if len(items) == 0 {
			return
		}
		start := lineStart + from
		end := lineStart + to
		infos := make(map[string]completionInfo, len(items))
		for _, item := range items {
			infos[completionTrackKey(item)] = completionInfo{
				Replace: &completionReplacement{From: start, To: end},
			}
		}
		m.showCompletionResults(t, req, start, items, infos)
	}
	download := m.gradleVersionCatalogDownload(script, catalogName)
	if download == nil {
		return true
	}
	if download.Ready {
		show(download)
	} else if download.Err == "" {
		m.waitForGradleVersionCatalogs(script, show)
	}
	return true
}

func (m *Model) gradleVersionCatalogDownload(
	script, catalogName string,
) *gradleVersionCatalogDownload {
	download := m.versionCatalogs.downloads[script]
	if download != nil && download.Ready {
		return download
	}
	if download == nil || !download.Loading || download.Root != m.Root() {
		if m.ctx == nil || (m.mgr == nil && m.invokeHook == nil) {
			return download
		}
		m.loadGradleVersionCatalogs(script)
		download = m.versionCatalogs.downloads[script]
	}
	if download != nil && download.Err != "" && catalogName != "" {
		m.status = "Gradle version catalogs are unavailable: " + download.Err
		m.invalidate()
	}
	return download
}

func (m *Model) loadGradleVersionCatalogs(script string) {
	if m.versionCatalogs.downloads == nil {
		m.versionCatalogs.downloads = map[string]*gradleVersionCatalogDownload{}
	}
	if previous := m.versionCatalogs.downloads[script]; previous != nil && previous.Loading {
		return
	}
	m.versionCatalogs.generation++
	gen := m.versionCatalogs.generation
	root := m.Root()
	download := &gradleVersionCatalogDownload{
		Root:    root,
		Loading: true,
		Gen:     gen,
	}
	m.versionCatalogs.downloads[script] = download
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, lspTimeout)
		defer cancel()
		read := func(value string) (string, bool, error) {
			return m.readGradleCatalogText(ctx, value)
		}
		published := func(coordinates string) (string, bool, error) {
			raw, err := m.invoke(ctx, "gradle_published_catalog", map[string]any{
				"coordinates": coordinates,
			})
			if err != nil {
				if openidea.ErrorKind(err) == "notFound" {
					return "", false, nil
				}
				return "", false, err
			}
			if strings.TrimSpace(string(raw)) == "null" {
				return "", false, nil
			}
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return "", false, err
			}
			return value, value != "", nil
		}
		catalogs, _, err := readGradleVersionCatalogs(script, root, read, published)
		m.post(func() {
			current := m.versionCatalogs.downloads[script]
			if current != download || download.Gen != gen ||
				m.versionCatalogs.generation != gen || m.Root() != root {
				return
			}
			current.Loading = false
			if err != nil {
				current.Err = err.Error()
			} else {
				current.Catalogs = catalogs
				current.Ready = true
			}
			waiters := current.Waiters
			current.Waiters = nil
			for _, run := range waiters {
				run(current)
				if current != m.versionCatalogs.downloads[script] ||
					m.versionCatalogs.generation != gen {
					break
				}
			}
			m.invalidate()
		})
	}()
}

func (m *Model) waitForGradleVersionCatalogs(
	script string,
	run func(*gradleVersionCatalogDownload),
) {
	download := m.versionCatalogs.downloads[script]
	if download == nil {
		return
	}
	if download.Ready {
		run(download)
		return
	}
	if download.Err != "" {
		m.status = "Gradle version catalogs are unavailable: " + download.Err
		m.invalidate()
		return
	}
	download.Waiters = append(download.Waiters, run)
}

func (m *Model) readGradleCatalogText(
	ctx context.Context,
	value string,
) (string, bool, error) {
	raw, err := m.invoke(ctx, "fs_read_text", map[string]any{"path": value})
	if err != nil {
		if openidea.ErrorKind(err) == "notFound" {
			return "", false, nil
		}
		return "", false, err
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", false, fmt.Errorf("read %s: %w", value, err)
	}
	return text, true, nil
}

func (m *Model) invalidateGradleVersionCatalogs() {
	m.versionCatalogs.generation++
	m.versionCatalogs.downloads = nil
}
