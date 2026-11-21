package adapter

// .NET test reports, ported from src/features/run/testing/dotnet.ts. VSTest
// writes TRX files and Microsoft Testing Platform reports them directly.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"mayoi/internal/openidea"
)

var dotnetTrxPathRe = regexp.MustCompile(`(?i)(?:[A-Za-z]:[\\/]|/).*\.trx\s*$`)
var dotnetLocationRe = regexp.MustCompile(`\bin (.+\.(?:cs|fs|vb)):(?:line )?(\d+)`)

// DotnetReports tracks the reports produced by one .NET test launch.
type DotnetReports struct {
	prefix string
	mtp    bool
	trx    bool
	paths  map[string]bool
	order  []string
}

// NewDotnetReports creates the report collector of a configuration.
func NewDotnetReports(config *openidea.RunConfig) *DotnetReports {
	var random [8]byte
	_, _ = rand.Read(random[:])
	prefix := "goidea-" + hex.EncodeToString(random[:])
	runner, _ := config.Params["runner"].(string)
	trx := true
	if raw, ok := config.Params["trx"].(string); ok {
		trx = raw != "false"
	}
	return &DotnetReports{
		prefix: prefix,
		mtp:    runner == "mtp" || runner == "mtp-legacy",
		trx:    trx,
		paths:  map[string]bool{},
	}
}

// Prefix is the unique LogFilePrefix embedded in this launch's TRX files.
func (r *DotnetReports) Prefix() string { return r.prefix }

// Args adds the TRX report option to a dotnet test command line.
func (r *DotnetReports) Args(config *openidea.RunConfig) []string {
	args := append([]string(nil), config.Args...)
	runner, _ := config.Params["runner"].(string)
	if r.mtp {
		if !r.trx {
			return args
		}
		if runner == "mtp-legacy" && !containsArg(args, "--") {
			args = append(args, "--")
		}
		if !containsArg(args, "--report-trx") {
			args = append(args, "--report-trx")
		}
		return args
	}
	at := len(args)
	if boundary := indexOfArg(args, "--"); boundary >= 0 {
		at = boundary
	}
	out := append([]string(nil), args[:at]...)
	out = append(out, "--logger", "trx;LogFilePrefix="+r.prefix)
	out = append(out, args[at:]...)
	return out
}

func containsArg(args []string, want string) bool {
	return indexOfArg(args, want) >= 0
}

func indexOfArg(args []string, want string) int {
	for i, arg := range args {
		if arg == want {
			return i
		}
	}
	return -1
}

// Feed remembers a TRX path printed by VSTest.
func (r *DotnetReports) Feed(line string) {
	path := strings.TrimSpace(dotnetTrxPathRe.FindString(line))
	if path == "" {
		return
	}
	base := filepath.Base(strings.ReplaceAll(path, `\`, `/`))
	if r.mtp || strings.HasPrefix(base, r.prefix) {
		if !r.paths[path] {
			r.paths[path] = true
			r.order = append(r.order, path)
		}
	}
}

// Paths returns the reports seen so far.
func (r *DotnetReports) Paths() []string {
	return append([]string(nil), r.order...)
}

// ReadReports parses every report using readText.
func (r *DotnetReports) ReadReports(readText func(string) (string, error)) ([]TestEvent, error) {
	var events []TestEvent
	var failures []error
	for _, path := range r.Paths() {
		text, err := readText(path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		parsed, err := TrxEvents(text)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		events = append(events, parsed...)
	}
	return events, errors.Join(failures...)
}

type trxNode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Text    string     `xml:",chardata"`
	Nodes   []trxNode  `xml:",any"`
}

func trxAttr(node trxNode, name string) string {
	for _, attr := range node.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func trxDescendants(node *trxNode, name string) []*trxNode {
	var out []*trxNode
	var visit func(*trxNode)
	visit = func(current *trxNode) {
		for i := range current.Nodes {
			child := &current.Nodes[i]
			if child.XMLName.Local == name {
				out = append(out, child)
			}
			visit(child)
		}
	}
	visit(node)
	return out
}

func trxFirst(node *trxNode, name string) *trxNode {
	if items := trxDescendants(node, name); len(items) > 0 {
		return items[0]
	}
	return nil
}

func trxText(node *trxNode) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(node.Text)
	for i := range node.Nodes {
		b.WriteString(trxText(&node.Nodes[i]))
	}
	return b.String()
}

func trxDuration(value string) *float64 {
	m := regexp.MustCompile(`^(\d+):(\d+):(\d+(?:\.\d+)?)$`).FindStringSubmatch(value)
	if m == nil {
		return nil
	}
	hours, _ := strconv.ParseFloat(m[1], 64)
	minutes, _ := strconv.ParseFloat(m[2], 64)
	seconds, _ := strconv.ParseFloat(m[3], 64)
	ms := (hours*3600 + minutes*60 + seconds) * 1000
	return &ms
}

// TrxEvents converts a VSTest TRX report into test events.
func TrxEvents(text string) ([]TestEvent, error) {
	var root trxNode
	if err := xml.Unmarshal([]byte(text), &root); err != nil {
		return nil, err
	}
	if root.XMLName.Local != "TestRun" {
		return nil, errors.New("Invalid TRX report")
	}
	definitions := map[string]*trxNode{}
	for _, unit := range trxDescendants(&root, "UnitTest") {
		if id := trxAttr(*unit, "id"); id != "" {
			definitions[id] = trxFirst(unit, "TestMethod")
		}
	}
	var events []TestEvent
	for _, result := range trxDescendants(&root, "UnitTestResult") {
		method := definitions[trxAttr(*result, "testId")]
		if method == nil {
			continue
		}
		className := strings.SplitN(trxAttr(*method, "className"), ",", 2)[0]
		name := strings.TrimSpace(regexp.MustCompile(`(?s)\(.*$`).ReplaceAllString(trxAttr(*method, "name"), ""))
		source := strings.ReplaceAll(trxAttr(*method, "codeBase"), `\`, `/`)
		if className == "" || name == "" || source == "" {
			continue
		}
		display := trxAttr(*result, "testName")
		if display == "" {
			display = name
		}
		display = strings.TrimPrefix(display, className+".")
		path := []string{source, className, name}
		if display != name {
			path = append(path, display)
		}
		outcome := trxAttr(*result, "outcome")
		status := TestFailed
		switch {
		case outcome == "Passed":
			status = TestPassed
		case regexp.MustCompile(`^(NotExecuted|Inconclusive|Pending)$`).MatchString(outcome):
			status = TestSkipped
		}
		events = append(events, TestEvent{
			Kind: "result", Path: path, Status: status,
			DurationMs: trxDuration(trxAttr(*result, "duration")),
		})
		for _, tag := range []string{"Message", "StackTrace", "StdOut", "StdErr"} {
			for _, node := range trxDescendants(result, tag) {
				lines := strings.Split(strings.ReplaceAll(trxText(node), "\r\n", "\n"), "\n")
				for i, line := range lines {
					if i >= 200 {
						break
					}
					events = append(events, TestEvent{Kind: "output", Path: path, Line: line})
					if loc := dotnetLocationRe.FindStringSubmatch(line); loc != nil {
						lineNo, _ := strconv.Atoi(loc[2])
						events = append(events, TestEvent{
							Kind: "location", Path: path,
							Location: &TestLocation{Path: loc[1], Line: lineNo, Resolve: "path"},
						})
					}
				}
			}
		}
	}
	return events, nil
}

// DotnetFilter builds the VSTest filter for failed TRX test paths.
func DotnetFilter(paths [][]string) string {
	seen := map[string]bool{}
	var names []string
	for _, path := range paths {
		if len(path) < 3 {
			return ""
		}
		name := path[1] + "." + path[2]
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	escaped := make([]string, 0, len(names))
	for _, name := range names {
		escaped = append(escaped, "FullyQualifiedName="+regexp.MustCompile(`[\\()&|=!~]`).
			ReplaceAllStringFunc(name, func(s string) string { return `\` + s }))
	}
	return strings.Join(escaped, "|")
}

// DotnetArgs replaces VSTest's --filter, preserving runner arguments after --.
func DotnetArgs(args []string, filter string) []string {
	before, after := args, []string(nil)
	if boundary := indexOfArg(args, "--"); boundary >= 0 {
		before, after = args[:boundary], args[boundary+1:]
	}
	var out []string
	for i := 0; i < len(before); i++ {
		arg := before[i]
		switch {
		case arg == "--filter":
			i++
		case strings.HasPrefix(arg, "--filter="):
		default:
			out = append(out, arg)
		}
	}
	out = append(out, "--filter", filter)
	if after != nil {
		out = append(out, "--")
		out = append(out, after...)
	}
	return out
}
