package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const coverageSuiteLimit = 10
const coverageImportLimit = 20

// The pinned OpenIDEA backend supplies all coverage counts and source paths.
type coverageData struct {
	Runner      string         `json:"runner"`
	HasBranches bool           `json:"hasBranches"`
	Files       []coverageFile `json:"files"`
}

type coverageFile struct {
	Path  string         `json:"path"`
	Lines []coverageLine `json:"lines"`
	coverageCounts
}

type coverageLine struct {
	Line            int `json:"line"`
	Hits            int `json:"hits"`
	BranchesTotal   int `json:"branchesTotal"`
	BranchesCovered int `json:"branchesCovered"`
}

type coverageCounts struct {
	LinesTotal      int `json:"linesTotal"`
	LinesCovered    int `json:"linesCovered"`
	BranchesTotal   int `json:"branchesTotal"`
	BranchesCovered int `json:"branchesCovered"`
}

func (c *coverageCounts) add(n coverageCounts) {
	c.LinesTotal += n.LinesTotal
	c.LinesCovered += n.LinesCovered
	c.BranchesTotal += n.BranchesTotal
	c.BranchesCovered += n.BranchesCovered
}

type coverageSuite struct {
	id          uint64
	name        string
	path        string
	created     time.Time
	runner      string
	hasBranches bool
	totals      coverageCounts
	tree        []*coverageNode
	badges      map[string]string
	lines       map[string][]coverageLine
}

type coverageImport struct {
	Path string    `json:"path"`
	Name string    `json:"name"`
	Time time.Time `json:"importedAt"`
}

type coverageReportEntry struct {
	key, name, detail, path string
	created                 time.Time
	suite                   *coverageSuite
}

type coverageRequest struct {
	root string
}

type coverageState struct {
	open, visible, manage bool
	suites                []*coverageSuite
	active, seq           uint64
	request               *coverageRequest
	message               string
	err                   string
	imports               []coverageImport
	importsLoaded         bool
	reportSelection       string
	outline               ui.OutlineState[*coverageNode]
	sort                  ui.SortOrder
	appliedSort           ui.SortOrder
	selected              int
	reports               ui.ListState
	reportIndex           int
	projectToken          *int
}

func (s *coverageState) activeSuite() *coverageSuite {
	for _, suite := range s.suites {
		if suite.id == s.active {
			return suite
		}
	}
	return nil
}

func (m *Model) addCoverageSuite(root, name, path string, data coverageData) *coverageSuite {
	s := &m.coverage
	s.seq++
	suite := makeCoverageSuite(root, name, path, data, s.seq)
	if path != "" {
		s.suites = slices.DeleteFunc(s.suites, func(other *coverageSuite) bool { return other.path == path })
	}
	s.suites = append([]*coverageSuite{suite}, s.suites...)
	s.suites = s.suites[:min(coverageSuiteLimit, len(s.suites))]
	m.selectCoverageSuite(suite.id)
	return suite
}

func (m *Model) selectCoverageSuite(id uint64) {
	s := &m.coverage
	s.active, s.visible = id, true
	s.resetCoverageTree()
	m.refreshDecorations()
	m.invalidate()
}

func (m *Model) closeCoverageSuite(id uint64) {
	s := &m.coverage
	s.suites = slices.DeleteFunc(s.suites, func(suite *coverageSuite) bool { return suite.id == id })
	if s.active == id {
		s.active = 0
		if len(s.suites) > 0 {
			s.active = s.suites[0].id
		}
		s.visible = s.visible && s.active != 0
		s.resetCoverageTree()
		m.refreshDecorations()
	}
	m.invalidate()
}

// SetCoverageVisible toggles the active suite's decorations in editors and the project tree.
func (m *Model) SetCoverageVisible(visible bool) {
	m.coverage.visible = visible && m.coverage.active != 0
	if m.coverage.visible {
		m.DockShow("coverage")
	}
	m.refreshDecorations()
	m.invalidate()
}

func (m *Model) coverageBadge(path string) string {
	if s := m.coverage.activeSuite(); s != nil && m.coverage.visible {
		return s.badges[filepath.Clean(path)]
	}
	return ""
}

func (m *Model) coverageGutterMarks(path string) []editor.GutterMark {
	suite := m.coverage.activeSuite()
	if suite == nil || !m.coverage.visible {
		return nil
	}
	lines := suite.lines[filepath.Clean(path)]
	if len(lines) == 0 {
		return nil
	}
	oi := oiPaletteOf(m.theme)
	marks := make([]editor.GutterMark, 0, len(lines))
	for _, line := range lines {
		color := gutterAddedColor
		if line.Hits == 0 {
			color = oi.danger
		} else if line.BranchesCovered < line.BranchesTotal {
			color = oi.warning
		}
		marks = append(marks, editor.GutterMark{Line: line.Line, Color: color})
	}
	return marks
}

// ImportCoverageReport asks which local report to import. The backend detects
// the format and resolves paths against the open project.
func (m *Model) ImportCoverageReport() {
	root := m.Root()
	if root == "" || m.win == nil || m.coverage.request != nil {
		return
	}
	req := &coverageRequest{root: root}
	m.coverage.request = req
	parent := m.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Import Coverage Report", ButtonLabel: "Import", DefaultPath: root,
		})
		m.post(func() {
			if !m.coverageRequestCurrent(req) {
				return
			}
			m.coverage.request = nil
			if err != nil {
				m.coverageError("Opening the report picker failed", err)
			} else if len(paths) > 0 {
				m.importCoveragePath(paths[0], false)
			}
			m.invalidate()
		})
	}()
}

func (m *Model) importCoveragePath(path string, closeManager bool) {
	if path == "" || m.Root() == "" || m.ctx == nil || m.coverage.request != nil {
		return
	}
	path = realPath(path)
	req := &coverageRequest{root: m.Root()}
	m.coverage.request = req
	m.coverage.err = ""
	m.coverage.message = "Importing " + filepath.Base(path) + "…"
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		data, err := m.readCoverage(ctx, req.root, "coverage_import", map[string]any{"path": path})
		m.post(func() {
			if !m.coverageRequestCurrent(req) {
				return
			}
			m.coverage.request = nil
			m.coverage.message = ""
			if err != nil {
				m.coverageError("Cannot import "+filepath.Base(path), err)
				return
			}
			suite := m.addCoverageSuite(req.root, filepath.Base(path), path, data)
			m.rememberCoverageImport(suite)
			if closeManager {
				m.coverage.manage = false
			}
			m.DockShow("coverage")
			m.announceCoverage("Imported coverage for "+suite.name, suite)
		})
	}()
}

func (m *Model) coverageRequestCurrent(req *coverageRequest) bool {
	return m.coverage.request == req && m.Root() == req.root
}

func (m *Model) readCoverage(ctx context.Context, root, method string, args any) (coverageData, error) {
	var data coverageData
	raw, err := m.invoke(ctx, method, args)
	if err == nil {
		err = json.Unmarshal(raw, &data)
	}
	if err != nil {
		return coverageData{}, err
	}
	// The report may name the same source through a symlink. Resolve paths
	// here, off the UI thread, so the editor and the suite use the same keys.
	for i := range data.Files {
		path := data.Files[i].Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		data.Files[i].Path = realPath(path)
	}
	return data, nil
}

func (m *Model) coverageError(title string, err error) {
	m.coverage.err = title + ": " + err.Error()
	m.notifyError("run.coverage", title, err.Error())
	m.invalidate()
}

func (m *Model) announceCoverage(title string, suite *coverageSuite) {
	lines := coveragePercent(suite.totals.LinesCovered, suite.totals.LinesTotal)
	detail := lines + " lines covered"
	if len(suite.lines) == 0 {
		detail = "The report covers no files of this project."
	}
	m.notifyInfo("run.coverage", title, detail)
	m.invalidate()
}

func (m *Model) coverageImportsPath() string {
	if m.cfg.SettingsPath == "" || m.Root() == "" {
		return ""
	}
	key := sha256.Sum256([]byte(m.Root()))
	return filepath.Join(filepath.Dir(m.cfg.SettingsPath), "coverage-reports", fmt.Sprintf("%x.json", key))
}

func (m *Model) loadCoverageImports() {
	s := &m.coverage
	if s.importsLoaded {
		return
	}
	s.importsLoaded = true
	if path := m.coverageImportsPath(); path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			var reports []coverageImport
			if json.Unmarshal(raw, &reports) == nil {
				reports = slices.DeleteFunc(reports, func(r coverageImport) bool { return r.Name == "" || r.Path == "" })
				s.imports = reports[:min(len(reports), coverageImportLimit)]
			}
		}
	}
}

func (m *Model) saveCoverageImports() {
	path := m.coverageImportsPath()
	if path == "" {
		return
	}
	raw, err := json.Marshal(m.coverage.imports)
	if err == nil {
		err = writeFileAtomic(path, raw)
	}
	if err != nil {
		m.notifyWarning("run.coverage-history", "Could not save coverage reports", err.Error())
	}
}

func (m *Model) rememberCoverageImport(suite *coverageSuite) {
	m.loadCoverageImports()
	s := &m.coverage
	s.imports = slices.DeleteFunc(s.imports, func(r coverageImport) bool { return r.Path == suite.path })
	s.imports = append([]coverageImport{{Path: suite.path, Name: suite.name, Time: suite.created}}, s.imports...)
	s.imports = s.imports[:min(coverageImportLimit, len(s.imports))]
	m.saveCoverageImports()
}

func (s *coverageState) reportEntries() []coverageReportEntry {
	entries := make([]coverageReportEntry, 0, len(s.suites)+len(s.imports))
	loaded := make(map[string]bool, len(s.suites))
	for _, suite := range s.suites {
		detail := suite.runner
		if suite.path != "" {
			detail = suite.path
			loaded[suite.path] = true
		}
		entries = append(entries, coverageReportEntry{key: fmt.Sprintf("suite:%d", suite.id), name: suite.name,
			detail: detail, path: suite.path, created: suite.created, suite: suite})
	}
	for _, r := range s.imports {
		if !loaded[r.Path] {
			entries = append(entries, coverageReportEntry{key: "report:" + r.Path, name: r.Name,
				detail: r.Path, path: r.Path, created: r.Time})
		}
	}
	return entries
}

func (m *Model) ManageCoverageReports() {
	if m.Root() == "" {
		return
	}
	m.loadCoverageImports()
	m.coverage.manage = true
	m.coverage.reportSelection = fmt.Sprintf("suite:%d", m.coverage.active)
	m.invalidate()
}

func (m *Model) showCoverageReport(entry coverageReportEntry) {
	if entry.suite == nil {
		m.importCoveragePath(entry.path, true)
		return
	}
	m.selectCoverageSuite(entry.suite.id)
	m.coverage.manage = false
	m.DockShow("coverage")
}

func (m *Model) deleteCoverageReport(entry coverageReportEntry) {
	if entry.suite != nil {
		m.closeCoverageSuite(entry.suite.id)
	}
	if entry.path != "" {
		s := &m.coverage
		s.imports = slices.DeleteFunc(s.imports, func(r coverageImport) bool { return r.Path == entry.path })
		m.saveCoverageImports()
	}
	m.invalidate()
}
