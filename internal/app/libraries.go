package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"mayoi/internal/openidea"
)

const externalLibrariesRefreshDelay = 500 * time.Millisecond

type externalLibrariesState struct {
	groups []openidea.LibraryGroup
	// rootsMu guards the path snapshot used by document opens on background
	// goroutines. groups is owned by the main thread.
	rootsMu sync.RWMutex
	roots   []string

	request uint32
	channel uint32
	cancel  context.CancelFunc
	seq     uint64
	loading bool
	err     string

	timer    *time.Timer
	schedule uint64
}

var externalLibraryGroupIcons = map[string]string{
	"python":      "fileTypes/python",
	"jdk":         "fileTypes/java",
	"nodeCore":    "fileTypes/javaScript",
	"nodeModules": "openidea/fileTypes/npm",
	"rust":        "fileTypes/rust",
	"cargo":       "fileTypes/cargo",
	"go":          "openidea/fileTypes/go",
	"goModules":   "openidea/fileTypes/goMod",
	"maven":       "maven/mavenProject",
	"gradle":      "fileTypes/gradle",
	"classpath":   "fileTypes/java",
	"ruby":        "openidea/fileTypes/ruby",
}

var externalLibrarySDKGroupKinds = map[string]bool{
	"jdk": true, "python": true, "ruby": true, "go": true, "rust": true,
}

var externalLibraryManifestNames = map[string]bool{
	"pyvenv.cfg": true, "pom.xml": true,
	"build.gradle": true, "build.gradle.kts": true,
	"settings.gradle": true, "settings.gradle.kts": true, "gradle.properties": true,
	"package.json": true, "package-lock.json": true, "yarn.lock": true,
	"pnpm-lock.yaml": true, "pnpm-workspace.yaml": true, "bun.lock": true, "bun.lockb": true,
	"Cargo.toml": true, "Cargo.lock": true,
	"rust-toolchain": true, "rust-toolchain.toml": true,
	"go.mod": true, "go.sum": true, "go.work": true, "go.work.sum": true,
	".classpath": true, "pyproject.toml": true, "setup.py": true, "requirements.txt": true,
	".python-version": true, "Gemfile": true, "Gemfile.lock": true,
	".ruby-version": true, ".tool-versions": true, "mise.toml": true,
}

func init() {
	_ = registerAction(&Action{
		ID:       "libraries.reload",
		Title:    "Reload External Libraries",
		Category: "Go",
		Keywords: "external libraries sdk dependency roots refresh reload",
		Run:      func(m *Model) { m.RefreshExternalLibraries() },
	})
}

func (m *Model) ensureExternalLibraries() *externalLibrariesState {
	if m.externalLibraries == nil {
		m.externalLibraries = &externalLibrariesState{}
	}
	return m.externalLibraries
}

// RefreshExternalLibraries asks the pinned OpenIDEA backend for every SDK and
// dependency root. Partial groups update the native tree as they arrive.
func (m *Model) RefreshExternalLibraries() {
	state := m.ensureExternalLibraries()
	root := m.Root()
	state.seq++
	seq := state.seq
	m.cancelExternalLibrariesRequest(state)
	if root == "" || !m.run.trusted {
		state.loading = false
		state.err = ""
		m.applyExternalLibraries(nil)
		return
	}

	request := m.nextRequest()
	channel := m.nextChannel()
	ctx, cancel := context.WithTimeout(m.externalLibrariesContext(), 2*time.Minute)
	state.request, state.channel, state.cancel = request, channel, cancel
	state.loading = true
	state.err = ""
	state.groups = nil
	m.invalidate()

	go func() {
		defer cancel()
		groups, err := m.projectLibraries(ctx, request, channel)
		m.post(func() {
			if m.externalLibraries == nil || state.seq != seq || m.Root() != root {
				return
			}
			state.request = 0
			state.channel = 0
			state.cancel = nil
			state.loading = false
			if err != nil {
				if !isCancelledError(err) {
					state.err = err.Error()
				}
				m.invalidate()
				return
			}
			m.applyExternalLibraries(groups)
		})
	}()
}

func (m *Model) externalLibrariesContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *Model) projectLibraries(
	ctx context.Context,
	request, channel uint32,
) ([]openidea.LibraryGroup, error) {
	if m.invokeHook != nil {
		raw, err := m.invoke(ctx, "sdk_project_libraries", map[string]any{
			"request":  request,
			"onGroups": map[string]any{"$channel": channel},
		})
		if err != nil {
			return nil, err
		}
		var groups []openidea.LibraryGroup
		if err := json.Unmarshal(raw, &groups); err != nil {
			return nil, err
		}
		return groups, nil
	}
	if m.mgr == nil {
		return nil, fmt.Errorf("the GoIDEA backend is unavailable")
	}
	return m.mgr.ProjectLibraries(ctx, Window, request, channel)
}

func (m *Model) cancelExternalLibrariesRequest(state *externalLibrariesState) {
	if state == nil {
		return
	}
	request := state.request
	cancel := state.cancel
	state.request, state.channel, state.cancel = 0, 0, nil
	if cancel != nil {
		cancel()
	}
	if request == 0 {
		return
	}
	go func() {
		ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
}

func (m *Model) acceptExternalLibrariesChunk(channel uint32, data json.RawMessage) bool {
	state := m.externalLibraries
	if state == nil || channel == 0 || channel != state.channel {
		return false
	}
	var groups []openidea.LibraryGroup
	if err := json.Unmarshal(data, &groups); err != nil {
		state.err = "Could not read the External Libraries reply: " + err.Error()
		m.invalidate()
		return true
	}
	state.groups = mergeLibraryGroups(append(state.groups, groups...))
	m.applyExternalLibraries(state.groups)
	return true
}

func (m *Model) applyExternalLibraries(groups []openidea.LibraryGroup) {
	groups = sortLibraryGroups(mergeLibraryGroups(groups))
	state := m.ensureExternalLibraries()
	state.groups = groups

	roots := make([]string, 0)
	seen := map[string]bool{}
	for _, group := range groups {
		for _, root := range group.Roots {
			if root.Path == "" || seen[root.Path] {
				continue
			}
			seen[root.Path] = true
			roots = append(roots, root.Path)
		}
	}
	sort.Strings(roots)
	state.rootsMu.Lock()
	state.roots = roots
	state.rootsMu.Unlock()

	if m.tree != nil {
		m.tree.SetExternalLibraries(groups, m.externalLibraryLoader())
	}
	for _, tab := range m.allTabs() {
		if tab == nil || tab.doc == nil || !m.isExternalLibraryPath(tab.doc.Path()) {
			continue
		}
		tab.doc.SetReadOnly(true)
		if tab.state != nil {
			tab.state.ReadOnly = true
		}
	}
	m.invalidate()
}

func (m *Model) resetExternalLibraries() {
	state := m.externalLibraries
	if state == nil {
		return
	}
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	state.schedule++
	state.seq++
	m.cancelExternalLibrariesRequest(state)
	state.groups = nil
	state.loading = false
	state.err = ""
	state.rootsMu.Lock()
	state.roots = nil
	state.rootsMu.Unlock()
	if m.tree != nil {
		m.tree.SetExternalLibraries(nil, nil)
	}
}

func (m *Model) externalLibraryRoots() []string {
	if m.externalLibraries == nil {
		return nil
	}
	m.externalLibraries.rootsMu.RLock()
	defer m.externalLibraries.rootsMu.RUnlock()
	return append([]string(nil), m.externalLibraries.roots...)
}

// IsExternalLibraryPath reports whether a document belongs to a read-only SDK
// or dependency root returned by OpenIDEA.
func (m *Model) isExternalLibraryPath(filePath string) bool {
	for _, root := range m.externalLibraryRoots() {
		if libraryPathWithin(root, filePath) {
			return true
		}
	}
	return false
}

func libraryPathWithin(root, candidate string) bool {
	if root == "" || candidate == "" {
		return false
	}
	if strings.HasSuffix(root, "!") {
		return candidate == root || strings.HasPrefix(candidate, root+"/")
	}
	root = strings.TrimRight(root, `/\`)
	candidate = strings.TrimRight(candidate, `/\`)
	if candidate == root {
		return true
	}
	for _, separator := range []string{"/", `\`} {
		if strings.HasPrefix(candidate, root+separator) {
			return true
		}
	}
	return false
}

func mergeLibraryGroups(groups []openidea.LibraryGroup) []openidea.LibraryGroup {
	merged := make([]openidea.LibraryGroup, 0, len(groups))
	index := map[string]int{}
	for _, group := range groups {
		if group.Name == "" {
			continue
		}
		key := group.Kind + "\x00" + group.Name
		at, ok := index[key]
		if !ok {
			index[key] = len(merged)
			copyGroup := group
			copyGroup.Roots = append([]openidea.LibraryRoot(nil), group.Roots...)
			merged = append(merged, copyGroup)
			continue
		}
		for _, root := range group.Roots {
			duplicate := false
			for _, existing := range merged[at].Roots {
				if existing.Path == root.Path {
					duplicate = true
					break
				}
			}
			if !duplicate {
				merged[at].Roots = append(merged[at].Roots, root)
			}
		}
	}
	return merged
}

func sortLibraryGroups(groups []openidea.LibraryGroup) []openidea.LibraryGroup {
	out := append([]openidea.LibraryGroup(nil), groups...)
	sort.SliceStable(out, func(i, j int) bool {
		leftSDK := externalLibrarySDKGroupKinds[out[i].Kind]
		rightSDK := externalLibrarySDKGroupKinds[out[j].Kind]
		if leftSDK != rightSDK {
			return leftSDK
		}
		return compareProjectNames(out[i].Name, out[j].Name) < 0
	})
	return out
}

func externalLibraryGroupLabel(group openidea.LibraryGroup) string {
	if externalLibrarySDKGroupKinds[group.Kind] {
		return "< " + group.Name + " >"
	}
	return group.Name
}

func externalLibraryGroupIcon(kind string) string {
	if icon := externalLibraryGroupIcons[kind]; icon != "" {
		return icon
	}
	return "nodes/library"
}

func externalLibraryRootIcon(root openidea.LibraryRoot) string {
	if strings.HasSuffix(root.Path, "!") {
		return "fileTypes/archive"
	}
	return "nodes/libraryFolder"
}

func touchesExternalLibraryManifest(paths []string) bool {
	for _, filePath := range paths {
		normalized := strings.ReplaceAll(filePath, `\`, "/")
		lower := strings.ToLower(normalized)
		if strings.Contains(lower, "/node_modules/") {
			continue
		}
		if externalLibraryManifestNames[path.Base(normalized)] {
			return true
		}
	}
	return false
}

func (m *Model) noteExternalLibraryManifestChange(event openidea.FsChangedEvent) {
	paths := uniqueChangedPaths(event.Created, event.Modified, event.Removed)
	if touchesExternalLibraryManifest(paths) {
		m.scheduleExternalLibrariesRefresh()
	}
}

func (m *Model) noteProjectRootsChanged(event openidea.ProjectRootsChangedEvent) {
	if event.Root != "" && m.Root() != "" && layoutKey(event.Root) != layoutKey(m.Root()) {
		return
	}
	m.scheduleExternalLibrariesRefresh()
}

func (m *Model) scheduleExternalLibrariesRefresh() {
	state := m.ensureExternalLibraries()
	state.schedule++
	schedule := state.schedule
	root := m.Root()
	if state.timer != nil {
		state.timer.Stop()
	}
	state.timer = time.AfterFunc(externalLibrariesRefreshDelay, func() {
		m.post(func() {
			if m.externalLibraries == nil || state.schedule != schedule || m.Root() != root {
				return
			}
			state.timer = nil
			m.RefreshExternalLibraries()
		})
	})
}

func (m *Model) externalLibraryLoader() func(string, func([]openidea.RemoteDirectoryEntry, error)) {
	return func(dir string, done func([]openidea.RemoteDirectoryEntry, error)) {
		go func() {
			ctx, cancel := context.WithTimeout(m.externalLibrariesContext(), 30*time.Second)
			defer cancel()
			raw, err := m.invoke(ctx, "fs_read_dir", map[string]any{"path": dir})
			if err != nil {
				done(nil, err)
				return
			}
			var entries []struct {
				Name string `json:"name"`
				Path string `json:"path"`
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(raw, &entries); err != nil {
				done(nil, err)
				return
			}
			out := make([]openidea.RemoteDirectoryEntry, 0, len(entries))
			for _, entry := range entries {
				out = append(out, openidea.RemoteDirectoryEntry{
					Name: entry.Name,
					Path: entry.Path,
					IsDir: strings.EqualFold(entry.Kind, "dir") ||
						strings.EqualFold(entry.Kind, "directory"),
				})
			}
			done(out, nil)
		}()
	}
}
