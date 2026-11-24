package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	vcsDefaultSubjectLimit = 72
	vcsDefaultBodyLimit    = 72
	vcsDefaultHistoryLimit = 1000
	vcsDefaultLargeFileMB  = 50
)

var (
	vcsIncomingFirstDelay = 20 * time.Second
	vcsIncomingInterval   = 15 * time.Minute
)

func (m *Model) vcsCommitNonModal() bool {
	return m.nativeRuntimeBool(settingsPageVCSCommit, "vcs.commit.nonModal")
}

func (m *Model) vcsCommitClearInitialMessage() bool {
	return m.nativeRuntimeBool(settingsPageVCSCommit, "vcs.commit.clearInitialMessage")
}

func (m *Model) vcsConfirmationMode(created bool) string {
	key := "vcs.confirmation.onDelete"
	if created {
		key = "vcs.confirmation.onCreate"
	}
	mode := m.nativeRuntimeString(settingsPageVCSConfirmation, key)
	switch mode {
	case "silently", "never":
		return mode
	default:
		return "ask"
	}
}

func (m *Model) vcsShowDirtyDirectories() bool {
	return m.nativeRuntimeBool(settingsPageVCSConfirmation, "vcs.showDirtyDirectories")
}

func (m *Model) vcsPatchCreatedMode() string {
	mode := m.nativeRuntimeString(settingsPageVCSConfirmation, "vcs.confirmation.onPatchCreated")
	switch mode {
	case "no", "reveal":
		return mode
	default:
		return "ask"
	}
}

func (m *Model) vcsHistoryLimit() int {
	if !m.nativeRuntimeBool(settingsPageVCSConfirmation, "vcs.limitHistory") {
		return 0
	}
	limit := int(m.nativeRuntimeNumber(settingsPageVCSConfirmation, "vcs.historyLimit"))
	if limit <= 0 {
		limit = vcsDefaultHistoryLimit
	}
	return limit
}

func (m *Model) vcsLogRequestLimit(path string) int {
	if strings.TrimSpace(path) == "" {
		return logLimit
	}
	if limit := m.vcsHistoryLimit(); limit > 0 {
		return limit
	}
	return logLimit
}

func (m *Model) ensureVCSIncomingCheck() {
	mode := m.vcsGitString("vcs.git.incomingCheck")
	root := vcsRepositoryKey(m.vcs.current)
	if mode == "never" || root == "" || (m.mgr == nil && m.invokeHook == nil) {
		if m.vcs.incomingCancel != nil {
			m.vcs.incomingCancel()
			m.vcs.incomingCancel = nil
			m.vcs.incomingRoot = ""
		}
		return
	}
	if m.vcs.incomingCancel != nil && m.vcs.incomingRoot == root &&
		!(m.vcs.incomingFailed && mode == "always") {
		return
	}
	if m.vcs.incomingCancel != nil {
		m.vcs.incomingCancel()
	}
	ctx, cancel := context.WithCancel(m.mergeContext())
	m.vcs.incomingCancel = cancel
	m.vcs.incomingRoot = root
	m.vcs.incomingFailed = false
	go func() {
		if !waitVCSIncoming(ctx, vcsIncomingFirstDelay) {
			return
		}
		for {
			var currentRoot, currentMode, upstream string
			m.post(func() {
				currentRoot = vcsRepositoryKey(m.vcs.current)
				currentMode = m.vcsGitString("vcs.git.incomingCheck")
				if m.branch.upstream != nil {
					upstream = strings.TrimSpace(*m.branch.upstream)
				}
			})
			if currentRoot != root {
				return
			}
			if currentMode == "never" {
				return
			}
			if upstream != "" {
				request := m.nextRequest()
				requestCtx, requestCancel := context.WithTimeout(ctx, 2*time.Minute)
				_, err := m.invoke(requestCtx, "vcs_remote", map[string]any{
					"repo": vcsRepositoryArgFor(root), "request": request,
					"op": map[string]any{"op": "fetch"},
				})
				requestCancel()
				if err != nil {
					m.post(func() {
						if vcsRepositoryKey(m.vcs.current) == root {
							m.vcs.incomingFailed = true
						}
					})
					if currentMode == "auto" {
						return
					}
				} else {
					m.post(func() {
						m.RefreshBranch()
						m.RefreshVCS()
					})
				}
			}
			if !waitVCSIncoming(ctx, vcsIncomingInterval) {
				return
			}
		}
	}()
}

func waitVCSIncoming(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (m *Model) vcsGitBool(key string) bool {
	return m.nativeRuntimeBool(settingsPageVCSGit, key)
}

func (m *Model) vcsGitString(key string) string {
	return m.nativeRuntimeString(settingsPageVCSGit, key)
}

func (m *Model) vcsGitNumber(key string, fallback int) int {
	value := int(m.nativeRuntimeNumber(settingsPageVCSGit, key))
	if value <= 0 {
		return fallback
	}
	return value
}

func (m *Model) vcsGitUpdateMethod() string {
	if m.vcsGitString("vcs.git.updateMethod") == "rebase" {
		return "rebase"
	}
	return "merge"
}

func (m *Model) vcsShelfRemoveApplied() bool {
	return m.nativeRuntimeBool(settingsPageVCSShelf, "vcs.shelf.removeApplied")
}

func (m *Model) vcsProtectedBranch(branch string) bool {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false
	}
	patterns := m.vcsGitString("vcs.git.protectedBranches")
	for _, pattern := range strings.Split(patterns, ";") {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" && wildcardFileNameMatch(pattern, branch) {
			return true
		}
	}
	return false
}

func (m *Model) vcsCurrentBranch() string {
	repo := m.currentVCSRepository()
	if repo == nil || repo.Head == nil {
		return ""
	}
	return strings.TrimSpace(*repo.Head)
}

func (m *Model) vcsPushDialogNeeded(branch string) bool {
	if strings.TrimSpace(branch) == "" {
		return true
	}
	if !m.vcsGitBool("vcs.git.showPushDialog") {
		return false
	}
	return !m.vcsGitBool("vcs.git.pushDialogOnlyProtected") ||
		m.vcsProtectedBranch(branch)
}

func vcsPushWasRejected(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{
		"(non-fast-forward)",
		"(fetch first)",
		"tip of your current branch is behind",
		"remote contains work that you do",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func (m *Model) afterPatchSaved(path string) {
	switch m.vcsPatchCreatedMode() {
	case "no":
		return
	case "reveal":
		revealPath(path)
	default:
		m.vcsMore = vcsMoreState{
			open: true, kind: vcsMoreConfirm, action: "patch.saved",
			title: "Create Patch",
			hint:  "Patch file created successfully. Show it in " + vcsFileManagerName() + "?",
			input: path,
		}
		m.invalidate()
	}
}

func vcsFileManagerName() string {
	switch runtime.GOOS {
	case "darwin":
		return "Finder"
	case "windows":
		return "Explorer"
	default:
		return "Files"
	}
}

func (m *Model) rememberVCSSetting(page, key string, value any) {
	state := m.nativeSettings.pages[page]
	if state.applied == nil {
		state.applied = map[string]any{}
	}
	if state.draft == nil {
		state.draft = map[string]any{}
	}
	state.applied[key] = value
	state.draft[key] = value
	m.nativeSettings.pages[page] = state
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 8*time.Second)
		defer cancel()
		if err := m.invokeSettingsSet(ctx, "project", key, value); err != nil {
			m.post(func() {
				m.status = "Could not save " + key + ": " + err.Error()
				m.invalidate()
			})
		}
	}()
}

type vcsCommitMessageSettings struct {
	blankLineAfterSubject bool
	limitSubject          bool
	subjectLimit          int
	limitBody             bool
	bodyLimit             int
}

func (m *Model) vcsCommitMessageSettings() vcsCommitMessageSettings {
	return vcsCommitMessageSettings{
		blankLineAfterSubject: m.nativeRuntimeBool(
			settingsPageVCSCommit, "vcs.commit.blankLineAfterSubject"),
		limitSubject: m.nativeRuntimeBool(settingsPageVCSCommit, "vcs.commit.limitSubject"),
		subjectLimit: m.vcsCommitLimit(
			"vcs.commit.subjectLimit", vcsDefaultSubjectLimit),
		limitBody: m.nativeRuntimeBool(settingsPageVCSCommit, "vcs.commit.limitBody"),
		bodyLimit: m.vcsCommitLimit(
			"vcs.commit.bodyLimit", vcsDefaultBodyLimit),
	}
}

func (m *Model) vcsCommitLimit(key string, fallback int) int {
	value := int(m.nativeRuntimeNumber(settingsPageVCSCommit, key))
	if value <= 0 {
		return fallback
	}
	return value
}

func vcsCommitMessageProblems(message string, settings vcsCommitMessageSettings) []string {
	lines := strings.Split(message, "\n")
	var problems []string
	if settings.limitSubject && utf8.RuneCountInString(lines[0]) > settings.subjectLimit {
		problems = append(problems, "Subject line is longer than "+
			strconv.Itoa(settings.subjectLimit)+" characters")
	}
	if settings.blankLineAfterSubject && len(lines) > 1 &&
		strings.TrimSpace(lines[1]) != "" {
		problems = append(problems, "Add a blank line between the subject and body")
	}
	if settings.limitBody {
		for index, line := range lines[1:] {
			if utf8.RuneCountInString(line) > settings.bodyLimit {
				problems = append(problems, "Body line "+strconv.Itoa(index+2)+" is longer than "+
					strconv.Itoa(settings.bodyLimit)+" characters")
				break
			}
		}
	}
	return problems
}

func wrapVCSCommitMessageAt(message string, limit int) string {
	if limit <= 0 || !strings.Contains(message, "\n") {
		return message
	}
	lines := strings.Split(message, "\n")
	var wrapped []string
	wrapped = append(wrapped, lines[0])
	for _, line := range lines[1:] {
		rest := line
		for utf8.RuneCountInString(rest) > limit {
			runes := []rune(rest)
			cut := -1
			for index := min(limit, len(runes)-1); index > 0; index-- {
				if runes[index] == ' ' {
					cut = index
					break
				}
			}
			if cut <= 0 {
				break
			}
			wrapped = append(wrapped, strings.TrimRight(string(runes[:cut]), " "))
			rest = strings.TrimLeft(string(runes[cut:]), " ")
		}
		wrapped = append(wrapped, rest)
	}
	return strings.Join(wrapped, "\n")
}

func (m *Model) vcsCommitMessageProblems(message string) []string {
	return vcsCommitMessageProblems(message, m.vcsCommitMessageSettings())
}

func (m *Model) vcsCommitLargeFileMB() int {
	return m.vcsGitNumber("vcs.git.largeFileMb", vcsDefaultLargeFileMB)
}

func (m *Model) vcsCommitCandidates() []vcsCommitCandidate {
	if m.commit == nil {
		return nil
	}
	plan := m.planCommit(m.commit)
	candidates := make([]vcsCommitCandidate, 0, len(plan.included))
	for _, path := range plan.included {
		candidates = append(candidates, vcsCommitCandidate{
			Path: path,
			Rel:  m.vcsRelativePath(path),
		})
	}
	return candidates
}

type vcsCommitCandidate struct {
	Path string
	Rel  string
}

func (m *Model) vcsCommitCheckProblems(c *commitState, plan commitPlan) []string {
	var problems []string
	repo := c.repo
	if repo == "" {
		repo = m.vcsRepositoryRoot()
	}
	if m.vcsGitBool("vcs.git.warnDetachedHead") {
		for _, candidate := range m.vcs.repositories {
			if filepath.Clean(candidate.Root) != filepath.Clean(repo) {
				continue
			}
			if candidate.Head == nil {
				problems = append(problems,
					"HEAD is detached. The commit will not belong to a branch and may be lost after checking out another branch.")
			} else if candidate.Operation != nil && *candidate.Operation == "rebase" {
				problems = append(problems,
					"A rebase is in progress. The commit will become part of the rebased history; usually continue the rebase instead.")
			}
			break
		}
	}

	statusByPath := make(map[string]string, len(m.vcs.files))
	for _, file := range m.vcs.files {
		statusByPath[filepath.Clean(file.Path)] = effectiveVCSKind(file)
	}
	var largeFiles []string
	var largeBytes int64
	largeLimit := int64(m.vcsCommitLargeFileMB()) * 1024 * 1024
	var todoFiles []string
	for _, path := range plan.included {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if m.vcsGitBool("vcs.git.warnLargeFiles") && info.Size() > largeLimit {
			largeFiles = append(largeFiles, filepath.Base(path))
			largeBytes += info.Size()
		}
		if m.vcsGitBool("vcs.git.warnCrlf") {
			if file, err := os.Open(path); err == nil {
				buffer := make([]byte, 64*1024)
				n, _ := file.Read(buffer)
				_ = file.Close()
				if bytes.Contains(buffer[:n], []byte("\r\n")) {
					problems = append(problems,
						"CRLF line separators are about to be committed in "+filepath.Base(path)+".")
				}
			}
		}
		kind := statusByPath[filepath.Clean(path)]
		if m.vcsGitBool("vcs.git.warnBadFileNames") &&
			(kind == "added" || kind == "untracked" || kind == "renamed") {
			if name, reserved := vcsWindowsBadFileName(m.vcsRelativePath(path)); name != "" {
				if reserved {
					problems = append(problems,
						"Some files use Windows reserved names: '"+name+"'. The repository could not be checked out on Windows.")
				} else {
					problems = append(problems,
						"Some file names contain characters that Windows does not allow: '"+name+"'.")
				}
			}
		}
		if m.nativeRuntimeBool(settingsPageVCSCommit, "vcs.commit.checkTodo") {
			data, err := os.ReadFile(path)
			if err == nil && len(data) <= 2*1024*1024 {
				for index, line := range strings.Split(string(data), "\n") {
					upper := strings.ToUpper(line)
					if strings.Contains(upper, "TODO") || strings.Contains(upper, "FIXME") {
						todoFiles = append(todoFiles, fmt.Sprintf("%s:%d", filepath.Base(path), index+1))
						break
					}
				}
			}
		}
	}
	if len(largeFiles) > 0 {
		sort.Strings(largeFiles)
		mb := (largeBytes + 1024*1024 - 1) / (1024 * 1024)
		if len(largeFiles) == 1 {
			problems = append(problems, fmt.Sprintf(
				"You are about to commit a large file: %s (%d MB). Consider Git LFS.",
				largeFiles[0], mb))
		} else {
			problems = append(problems, fmt.Sprintf(
				"You are about to commit %d large files with a total size of %d MB. Consider Git LFS.",
				len(largeFiles), mb))
		}
	}
	if len(todoFiles) > 0 {
		if len(todoFiles) > 4 {
			todoFiles = append(todoFiles[:4], "…")
		}
		problems = append(problems,
			"TODO and FIXME items were found in changed files: "+strings.Join(todoFiles, ", "))
	}
	if m.nativeRuntimeBool(settingsPageVCSCommit, "vcs.commit.analyzeCode") {
		errors, warnings := 0, 0
		for _, path := range plan.included {
			for _, problem := range m.problemsOf(path) {
				switch problem.Severity {
				case "error":
					errors++
				case "warning":
					warnings++
				}
			}
		}
		if errors+warnings > 0 {
			problems = append(problems, fmt.Sprintf(
				"Analyze code found %d errors and %d warnings in the files selected for commit.",
				errors, warnings))
		}
	}
	return problems
}

func vcsWindowsBadFileName(relative string) (string, bool) {
	reserved := []string{
		"CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
	}
	reservedSet := make(map[string]bool, len(reserved))
	for _, name := range reserved {
		reservedSet[name] = true
	}
	for _, name := range strings.Split(filepath.ToSlash(relative), "/") {
		if name == "" {
			continue
		}
		stem := name
		if dot := strings.LastIndex(stem, "."); dot > 0 {
			stem = stem[:dot]
		}
		if reservedSet[strings.ToUpper(stem)] {
			return name, true
		}
		for _, char := range name {
			if char < 0x20 || strings.ContainsRune(`<>:"\|?*`, char) {
				return name, false
			}
		}
	}
	return "", false
}

func (m *Model) vcsRelativePath(path string) string {
	repo := vcsRepositoryContainingPath(path, m.vcs.repositories)
	if repo == "" {
		repo = m.Root()
	}
	rel, err := filepath.Rel(repo, path)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return filepath.ToSlash(rel)
}
