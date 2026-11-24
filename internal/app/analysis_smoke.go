package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/editor"
)

// SmokeAnalysis exercises the native analysis windows against real Java
// sources, JDT and the pinned OpenIDEA backend.
func (m *Model) SmokeAnalysis(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	root := m.Root()
	if root == "" {
		return out, errors.New("No project is open")
	}
	if m.mgr == nil {
		return out, errors.New("The GoIDEA backend is not connected")
	}
	if _, err := m.mgr.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": true,
	}); err != nil {
		return out, err
	}
	if err := m.reg.ReopenAll(ctx); err != nil {
		out["reopenError"] = err.Error()
	}

	name := fmt.Sprintf("smoke%d", time.Now().UnixNano())
	base := filepath.Join(root, "src", "main", "java", "com", "example", "goidea_analysis", name)
	inspectionDir := filepath.Join(base, "inspection")
	cleanupDir := filepath.Join(base, "cleanup")
	if err := os.MkdirAll(inspectionDir, 0o755); err != nil {
		return out, err
	}
	if err := os.MkdirAll(cleanupDir, 0o755); err != nil {
		return out, err
	}
	defer os.RemoveAll(base)

	packageName := "com.example.goidea_analysis." + name + ".inspection"
	inspectionPath := filepath.Join(inspectionDir, "AnalysisSmoke.java")
	inspectionSource := fmt.Sprintf(`package %s;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

public class AnalysisSmoke {
    private int unusedAnalysisField;

    @Deprecated
    static class LegacySmoke {
    }

    void warningSites() {
        LegacySmoke legacy = new LegacySmoke();
    }

    void brokenSite() {
        missingAnalysisSmokeSymbol();
    }

    List<String> firstCopy(String value) {
        List<String> values = new ArrayList<>();
        values.add(value);
        values.add("alpha");
        values.add("beta");
        values.add("gamma");
        values.add("delta");
        for (String item : values) {
            System.out.println(item);
        }
        return values;
    }

    List<String> secondCopy(String value) {
        List<String> values = new ArrayList<>();
        values.add(value);
        values.add("alpha");
        values.add("beta");
        values.add("gamma");
        values.add("delta");
        for (String item : values) {
            System.out.println(item);
        }
        return values;
    }
}
`, packageName)
	if err := os.WriteFile(inspectionPath, []byte(inspectionSource), 0o644); err != nil {
		return out, err
	}

	cleanupPackage := "com.example.goidea_analysis." + name + ".cleanup"
	cleanupPath := filepath.Join(cleanupDir, "CleanupSmoke.java")
	cleanupSource := fmt.Sprintf(`package %s;

import java.util.List;
import java.util.ArrayList;

public class CleanupSmoke {
  public String  value( ) {
    List<String> items = new ArrayList<>();
    items.add("cleanup");
    return   items.get( 0 );
  }
}
`, cleanupPackage)
	if err := os.WriteFile(cleanupPath, []byte(cleanupSource), 0o644); err != nil {
		return out, err
	}

	mygo.RunOnMain(func() { m.openFile(inspectionPath) })
	if err := m.smokeAnalysisWait(ctx, 60*time.Second, func() bool {
		var open bool
		mygo.RunOnMain(func() { open = m.tabForPath(inspectionPath) >= 0 })
		return open
	}); err != nil {
		return out, fmt.Errorf("opening the analysis fixture: %w", err)
	}
	if err := m.smokeAnalysisWaitJDT(ctx); err != nil {
		return out, err
	}
	out["inspectionPath"] = inspectionPath
	out["cleanupPath"] = cleanupPath

	inspection, err := m.smokeAnalysisInspection(ctx)
	out["inspection"] = inspection
	if err != nil {
		return out, err
	}
	duplicates, err := m.smokeAnalysisDuplicates(ctx)
	out["duplicates"] = duplicates
	if err != nil {
		return out, err
	}
	structural, err := m.smokeAnalysisStructural(ctx)
	out["structural"] = structural
	if err != nil {
		return out, err
	}
	cleanup, err := m.smokeAnalysisCleanup(ctx, cleanupDir, cleanupPath, cleanupSource)
	out["cleanup"] = cleanup
	if err != nil {
		return out, err
	}
	cleanupFileDir := filepath.Join(base, "cleanup-file")
	if err := os.MkdirAll(cleanupFileDir, 0o755); err != nil {
		return out, err
	}
	cleanupFilePath := filepath.Join(cleanupFileDir, "CleanupFileSmoke.java")
	cleanupFilePackage := "com.example.goidea_analysis." + name + ".cleanup_file"
	cleanupFileSource := fmt.Sprintf(`package %s;

import java.util.List;
import java.util.ArrayList;

public class CleanupFileSmoke {
  public String  value( ) {
    List<String> items = new ArrayList<>();
    items.add("cleanup-file");
    return   items.get( 0 );
  }
}
`, cleanupFilePackage)
	if err := os.WriteFile(cleanupFilePath, []byte(cleanupFileSource), 0o644); err != nil {
		return out, err
	}
	cleanupFile, err := m.smokeAnalysisCleanupFile(ctx, cleanupFilePath, cleanupFileSource)
	out["cleanupFile"] = cleanupFile
	if err != nil {
		return out, err
	}

	var tab *Tab
	mygo.RunOnMain(func() {
		if i := m.tabForPath(inspectionPath); i >= 0 {
			tab = m.tabs[i]
		}
	})
	if tab != nil {
		_ = tab.doc.Save(ctx)
		mygo.RunOnMain(func() {
			if i := m.tabForPath(inspectionPath); i >= 0 {
				m.closeTab(i)
			}
		})
	}
	return out, nil
}

func (m *Model) smokeAnalysisInspection(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		state := &m.analysis.inspection
		state.scope.kind = analysisScopeFile
		state.groupBySeverity = true
		state.severity = ""
		state.filter = ""
		m.RunAnalysisInspection()
	})
	var summary *analysisInspectSummary
	var runErr string
	if err := m.smokeAnalysisWait(ctx, 3*time.Minute, func() bool {
		var done bool
		mygo.RunOnMain(func() {
			state := &m.analysis.inspection
			done = !state.running && (state.summary != nil || state.err != "")
			runErr = state.err
			summary = state.summary
		})
		return done
	}); err != nil {
		return out, fmt.Errorf("Inspect Code: %w", err)
	}
	if runErr != "" {
		return out, errors.New(runErr)
	}
	if summary == nil || summary.Problems == 0 {
		return out, errors.New("Inspect Code returned no problems for the real JDT fixture")
	}

	var rawFiles, problemsCount int
	var severityGroups []string
	var filterSeverity string
	var filteredCount, hiddenOtherSeverity int
	var selectedLine int
	var navigationPath string
	mygo.RunOnMain(func() {
		state := &m.analysis.inspection
		rawFiles = len(state.rawFiles)
		problems := m.analysisInspectionProblems()
		problemsCount = len(problems)
		for _, node := range analysisInspectionNodes(problems, true) {
			severityGroups = append(severityGroups, node.Severity)
		}
		filterSeverity = analysisSeverity(problems[0].Diag)
		state.severity = filterSeverity
		for _, problem := range m.analysisInspectionProblems() {
			filteredCount++
			if analysisSeverity(problem.Diag) != filterSeverity {
				hiddenOtherSeverity++
			}
		}
		state.severity = ""
		chosen := problems[0]
		for _, problem := range problems {
			if analysisSeverity(problem.Diag) == "error" {
				chosen = problem
				break
			}
		}
		m.selectAnalysisInspectionProblem(chosen, true)
		if tab := m.tab(); tab != nil {
			navigationPath = tab.doc.Path()
			selectedLine = tab.state.CursorPos().Line
		}
	})
	out["rawFiles"] = rawFiles
	out["problems"] = problemsCount
	out["severityGroups"] = severityGroups
	out["filterSeverity"] = filterSeverity
	out["filteredProblems"] = filteredCount
	out["filterHiddenOtherSeverity"] = hiddenOtherSeverity == 0
	out["navigationPath"] = navigationPath
	out["navigationLine"] = selectedLine

	mygo.RunOnMain(func() {
		state := &m.analysis.inspection
		state.scope.kind = analysisScopeProject
		state.scope.directory = ""
		state.severity = ""
		state.filter = ""
		m.RunAnalysisInspection()
	})
	var projectSummary *analysisInspectSummary
	if err := m.smokeAnalysisWait(ctx, 3*time.Minute, func() bool {
		var done bool
		mygo.RunOnMain(func() {
			state := &m.analysis.inspection
			done = !state.running && (state.summary != nil || state.err != "")
			runErr = state.err
			projectSummary = state.summary
		})
		return done
	}); err != nil {
		return out, fmt.Errorf("project-scope Inspect Code: %w", err)
	}
	if runErr != "" {
		return out, errors.New(runErr)
	}
	if projectSummary == nil || projectSummary.Problems == 0 {
		return out, errors.New("project-scope Inspect Code returned no problems")
	}
	out["projectProblems"] = projectSummary.Problems

	var cancelIssued bool
	var requestAfter uint32
	var runningAfter bool
	mygo.RunOnMain(func() {
		state := &m.analysis.inspection
		state.scope.kind = analysisScopeFile
		m.RunAnalysisInspection()
		hadRequest := state.request != 0
		m.CancelAnalysisInspection()
		cancelIssued = hadRequest && !state.running && state.request == 0
		requestAfter = state.request
		runningAfter = state.running
	})
	out["cancelIssued"] = cancelIssued
	out["cancelRequest"] = requestAfter
	out["cancelRunning"] = runningAfter
	return out, nil
}

func (m *Model) smokeAnalysisDuplicates(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		state := &m.analysis.duplicates
		state.scope.kind = analysisScopeFile
		state.minText = "20"
		state.anonymizeIdentifiers = true
		state.anonymizeLiterals = true
		m.RunAnalysisDuplicates()
	})
	var report *analysisDuplicatesReport
	var runErr string
	if err := m.smokeAnalysisWait(ctx, 2*time.Minute, func() bool {
		var done bool
		mygo.RunOnMain(func() {
			state := &m.analysis.duplicates
			done = !state.running && (state.report != nil || state.err != "")
			report = state.report
			runErr = state.err
		})
		return done
	}); err != nil {
		return out, fmt.Errorf("Locate Duplicates: %w", err)
	}
	if runErr != "" {
		return out, errors.New(runErr)
	}
	if report == nil || len(report.Groups) == 0 {
		return out, errors.New("Locate Duplicates found no group in the real duplicate fixture")
	}
	fragments := 0
	for _, group := range report.Groups {
		fragments += len(group.Fragments)
	}
	var selected int
	mygo.RunOnMain(func() { selected = m.analysis.duplicates.selected })
	out["groups"] = len(report.Groups)
	out["fragments"] = fragments
	out["selected"] = selected
	out["files"] = report.Files
	out["tokens"] = report.Tokens
	return out, nil
}

func (m *Model) smokeAnalysisStructural(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	const pattern = "System.out.println($x$);"
	const replacement = "System.out.print($x$);"

	mygo.RunOnMain(func() {
		state := &m.analysis.structural
		state.languages = nil
		state.languagesLoading = false
		state.languagesErr = ""
		m.loadAnalysisStructuralLanguages()
	})
	var languages int
	var javaLanguage bool
	var languageErr string
	if err := m.smokeAnalysisWait(ctx, time.Minute, func() bool {
		var done bool
		mygo.RunOnMain(func() {
			state := &m.analysis.structural
			languages = len(state.languages)
			languageErr = state.languagesErr
			done = !state.languagesLoading && (languages > 0 || languageErr != "")
		})
		return done
	}); err != nil {
		return out, fmt.Errorf("Structural Search languages: %w", err)
	}
	if languageErr != "" {
		return out, errors.New(languageErr)
	}
	mygo.RunOnMain(func() {
		for _, language := range m.analysis.structural.languages {
			if language.ID == "java" {
				javaLanguage = true
				break
			}
		}
	})
	if !javaLanguage {
		return out, errors.New("Structural Search languages did not include Java")
	}
	out["languages"] = languages
	out["javaLanguage"] = true

	var tab *Tab
	var original string
	mygo.RunOnMain(func() {
		tab = m.tab()
		if tab != nil {
			original = tab.doc.Text()
		}
		state := &m.analysis.structural
		state.scope.kind = analysisScopeFile
		state.pattern = pattern
		state.replacement = replacement
		state.language = "java"
		state.replace = false
		m.RunAnalysisStructural()
	})
	if tab == nil {
		return out, errors.New("The structural analysis fixture is not open")
	}
	if err := waitAnalysisStructural(ctx, m, false); err != nil {
		return out, err
	}
	var searchMatches, searchFiles int
	var resultsShown bool
	var usageCount int
	var validationErr string
	mygo.RunOnMain(func() {
		state := &m.analysis.structural
		if state.summary != nil {
			searchMatches = int(state.summary.Matches)
		}
		searchFiles = len(state.files)
		resultsShown = state.resultsShown
		usageCount = len(m.usages.locs)
		validationErr = state.validationErr
	})
	if validationErr != "" {
		return out, errors.New(validationErr)
	}
	if searchMatches == 0 {
		return out, errors.New("Structural Search found no matches in the real Java fixture")
	}
	out["searchMatches"] = searchMatches
	out["searchFiles"] = searchFiles
	out["findUsages"] = resultsShown && usageCount == searchMatches

	mygo.RunOnMain(func() {
		state := &m.analysis.structural
		state.replace = true
		m.RunAnalysisStructural()
	})
	if err := waitAnalysisStructural(ctx, m, true); err != nil {
		return out, err
	}
	var previewCount int
	mygo.RunOnMain(func() {
		state := &m.analysis.structural
		if state.prepared != nil {
			previewCount = analysisStructuralReplacementCount(m.analysisStructuralEdit())
		}
	})
	if previewCount == 0 {
		return out, errors.New("Replace Structurally prepared no edits")
	}
	out["previewCount"] = previewCount

	stale := original + "\n// structural stale probe\n"
	if err := tab.doc.ApplyEdits(ctx, editor.Edit{
		Start: len(original), End: len(original), Text: "\n// structural stale probe\n",
	}); err != nil {
		return out, err
	}
	mygo.RunOnMain(func() { m.ApplyAnalysisStructuralReplace() })
	if err := m.smokeAnalysisWait(ctx, time.Minute, func() bool {
		var done bool
		mygo.RunOnMain(func() { done = !m.analysis.structural.applying })
		return done
	}); err != nil {
		return out, fmt.Errorf("waiting for stale structural rejection: %w", err)
	}
	var staleErr string
	mygo.RunOnMain(func() { staleErr = m.analysis.structural.err })
	out["staleRejected"] = strings.Contains(staleErr, "files changed since the preview")
	out["staleError"] = staleErr
	if !out["staleRejected"].(bool) {
		return out, fmt.Errorf("stale structural edit was not rejected: %q", staleErr)
	}
	current := tab.doc.Text()
	if err := tab.doc.ApplyEdits(ctx, editor.Edit{
		Start: len(original), End: len(current), Text: "",
	}); err != nil {
		return out, err
	}
	if err := m.smokeAnalysisWait(ctx, 30*time.Second, func() bool {
		return tab.doc.Text() == original
	}); err != nil {
		return out, fmt.Errorf("restoring the structural fixture: %w", err)
	}

	mygo.RunOnMain(func() {
		state := &m.analysis.structural
		state.replace = true
		m.RunAnalysisStructural()
	})
	if err := waitAnalysisStructural(ctx, m, true); err != nil {
		return out, err
	}
	var applyCount int
	mygo.RunOnMain(func() {
		applyCount = analysisStructuralReplacementCount(m.analysisStructuralEdit())
		m.ApplyAnalysisStructuralReplace()
	})
	if err := m.smokeAnalysisWait(ctx, time.Minute, func() bool {
		var done bool
		mygo.RunOnMain(func() {
			state := &m.analysis.structural
			done = !state.applying && state.applyStatus != ""
		})
		return done
	}); err != nil {
		return out, fmt.Errorf("applying the structural replacement: %w", err)
	}
	if err := m.smokeAnalysisWait(ctx, time.Minute, func() bool {
		return strings.Contains(tab.doc.Text(), "System.out.print(")
	}); err != nil {
		return out, fmt.Errorf("waiting for the structural edits in the editor: %w", err)
	}
	out["applyCount"] = applyCount
	out["applied"] = true

	if err := m.smokeAnalysisWait(ctx, time.Minute, func() bool {
		var ready bool
		mygo.RunOnMain(func() { ready = m.refactor != nil })
		return ready
	}); err != nil {
		return out, fmt.Errorf("waiting for structural refactor history: %w", err)
	}
	mygo.RunOnMain(func() { m.UndoRefactor() })
	if err := m.smokeAnalysisWait(ctx, time.Minute, func() bool {
		return tab.doc.Text() == original
	}); err != nil {
		return out, fmt.Errorf("undoing the structural replacement: %w", err)
	}
	out["undone"] = true
	out["staleText"] = stale != original
	return out, nil
}

func waitAnalysisStructural(
	ctx context.Context,
	m *Model,
	wantPrepared bool,
) error {
	var errText string
	waitErr := m.smokeAnalysisWait(ctx, 3*time.Minute, func() bool {
		var done bool
		mygo.RunOnMain(func() {
			state := &m.analysis.structural
			errText = state.err
			if state.validationErr != "" {
				errText = state.validationErr
			}
			done = !state.running && !state.validating && (errText != "" ||
				state.summary != nil && (!wantPrepared || state.prepared != nil))
		})
		return done
	})
	if waitErr != nil {
		var state analysisStructuralState
		mygo.RunOnMain(func() {
			state = m.analysis.structural
		})
		summary := "nil"
		if state.summary != nil {
			summary = fmt.Sprintf(
				"matches=%d files=%d cancelled=%t",
				state.summary.Matches,
				state.summary.Files,
				state.summary.Cancelled,
			)
		}
		return fmt.Errorf(
			"Structural Search: %w (running=%t validating=%t applying=%t request=%d summary=%s prepared=%t preparing=%t err=%q validation=%q status=%q)",
			waitErr,
			state.running,
			state.validating,
			state.applying,
			state.request,
			summary,
			state.prepared != nil,
			state.prepareStarted,
			state.err,
			state.validationErr,
			state.applyStatus,
		)
	}
	if errText != "" {
		return errors.New(errText)
	}
	return nil
}

func (m *Model) smokeAnalysisCleanup(
	ctx context.Context,
	dir, path, original string,
) (map[string]any, error) {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		state := &m.analysis.cleanup
		state.scope.kind = analysisScopeDirectory
		state.scope.directory = dir
		state.fixAll = true
		state.optimizeImports = true
		state.reformat = true
	})
	mygo.RunOnMain(func() { m.RunAnalysisCleanup() })
	var processed int
	var status, runErr string
	if err := m.smokeAnalysisWait(ctx, 3*time.Minute, func() bool {
		var done bool
		mygo.RunOnMain(func() {
			state := &m.analysis.cleanup
			done = !state.running && (state.processed > 0 || state.err != "" || state.status != "")
			processed = state.processed
			status = state.status
			runErr = state.err
		})
		return done
	}); err != nil {
		return out, fmt.Errorf("Code Cleanup: %w", err)
	}
	if runErr != "" {
		return out, errors.New(runErr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	var tabClosed bool
	mygo.RunOnMain(func() { tabClosed = m.tabForPath(path) < 0 })
	out["processed"] = processed
	out["status"] = status
	out["changed"] = string(after) != original
	out["formatted"] = strings.Contains(string(after), "return items.get(0);")
	out["tabClosed"] = tabClosed
	if processed != 1 {
		return out, fmt.Errorf("Code Cleanup processed %d files, want 1", processed)
	}
	if string(after) == original {
		return out, errors.New("Code Cleanup saved unchanged source")
	}
	if !tabClosed {
		return out, errors.New("Code Cleanup left the tab it opened")
	}
	return out, nil
}

func (m *Model) smokeAnalysisCleanupFile(
	ctx context.Context,
	path, original string,
) (map[string]any, error) {
	out := map[string]any{}
	mygo.RunOnMain(func() {
		m.status = ""
		m.CleanupAnalysisFile(map[string]any{
			"path":            path,
			"fixAll":          true,
			"optimizeImports": true,
			"reformat":        true,
		})
	})
	var status string
	var tabClosed bool
	if err := m.smokeAnalysisWait(ctx, time.Minute, func() bool {
		mygo.RunOnMain(func() {
			status = m.status
			tabClosed = m.tabForPath(path) < 0
		})
		return tabClosed && (strings.HasPrefix(status, "Cleaned ") ||
			strings.Contains(status, " failed:"))
	}); err != nil {
		return out, fmt.Errorf("Code Cleanup in File: %w", err)
	}
	if !strings.HasPrefix(status, "Cleaned ") {
		return out, errors.New(status)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	out["status"] = status
	out["changed"] = string(after) != original
	out["formatted"] = strings.Contains(string(after), "return items.get(0);")
	out["tabClosed"] = tabClosed
	if !out["changed"].(bool) {
		return out, errors.New("Code Cleanup in File saved unchanged source")
	}
	if !out["formatted"].(bool) {
		return out, errors.New("Code Cleanup in File did not format the source")
	}
	return out, nil
}

func (m *Model) smokeAnalysisWait(
	ctx context.Context,
	timeout time.Duration,
	done func() bool,
) error {
	deadline := time.Now().Add(timeout)
	for {
		if done() {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("timed out")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (m *Model) smokeAnalysisWaitJDT(ctx context.Context) error {
	deadline := time.Now().Add(3 * time.Minute)
	var states string
	for {
		servers := m.smokeLSPServers(ctx)
		if lspJavaStalled(servers) {
			return errors.New("the Java language server could not start: " + lspJavaStates(servers))
		}
		for _, server := range servers {
			hasJava := false
			for _, language := range server.Languages {
				if language == "java" {
					hasJava = true
					break
				}
			}
			if server.Status == "ready" && hasJava {
				return nil
			}
		}
		states = lspJavaStates(servers)
		if time.Now().After(deadline) {
			return errors.New("waiting for JDT timed out: " + states)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
