package app

// The startup readiness state machine of the goal's section 8: opening a
// project is not the same as having it imported, and operations that need a
// state name the first missing one instead of failing silently.
//
//	Backend Connected → Project Opened → Project Importing → Java Ready →
//	Build Model Ready → Run Ready
//
// The states derive from the live signals the app already has (the manager's
// phase, project_current, the lsp_servers status of jdtls, the build model
// and the run pane); refreshReadiness caches them for the UI, and the e2e
// readiness phase records when each state was first reached.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"time"
)

// jsonUnmarshalInto is json.Unmarshal with the awkward error ignored for
// convenience reads.
func jsonUnmarshalInto(raw json.RawMessage, v any) error {
	return json.Unmarshal(raw, v)
}

// samePath compares two paths after resolving symlinks (/tmp on macOS).
func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b) || filepath.Clean(realPath(a)) == filepath.Clean(realPath(b))
}

// hasLanguage reports whether the languages list carries lang.
func hasLanguage(languages []string, lang string) bool {
	return slices.Contains(languages, lang)
}

// Readiness is the six-state snapshot (section 8). Importing is the
// in-between state: the project is open while its Java import is still
// running.
type Readiness struct {
	Backend    bool   `json:"backend"`
	Project    bool   `json:"project"`
	Importing  bool   `json:"importing"`
	Java       bool   `json:"java"`
	BuildModel bool   `json:"buildModel"`
	Run        bool   `json:"run"`
	JavaDetail string `json:"javaDetail,omitempty"`
	At         string `json:"at"`
}

// readinessNow computes the snapshot on the caller's goroutine; it talks to
// the backend, so only goroutines may call it.
func (m *Model) readinessNow(ctx context.Context) Readiness {
	var r Readiness
	r.At = time.Now().Format(time.RFC3339)

	st := m.BackendState()
	r.Backend = st.Running && st.Phase == "running"
	if !r.Backend {
		return r
	}

	root := m.Root()
	if root == "" {
		return r
	}
	raw, err := m.mgr.Invoke(ctx, Window, "project_current", nil)
	if err == nil {
		var info struct {
			Root string `json:"root"`
		}
		if jsonUnmarshalInto(raw, &info) == nil && info.Root != "" && samePath(info.Root, root) {
			r.Project = true
		}
	}
	if !r.Project {
		return r
	}

	// Java Ready is the language server's verdict; Project Importing is the
	// in-between.
	var servers []lspServerState
	_ = m.smokeLSPCall(ctx, "lsp_servers", nil, &servers)
	for _, s := range servers {
		if !hasLanguage(s.Languages, "java") {
			continue
		}
		if s.Status == "ready" {
			r.Java = true
		} else {
			r.JavaDetail = s.Status
		}
	}
	if !r.Java {
		r.Importing = r.JavaDetail == "" || r.JavaDetail == "starting" || r.JavaDetail == "busy"
	}

	// Build Model Ready: the Maven/Gradle window's model, loaded with the
	// project. It is independent of the optional language server: Maven runs
	// and compile actions work while Java language services are absent.
	var loading bool
	var modelErr string
	m.post(func() {
		loading = m.maven.loading
		modelErr = m.maven.err
	})
	r.BuildModel = !loading && modelErr == ""

	// Run Ready: the project is trusted and its configurations are loaded. The
	// three outcomes stay independent so the UI can name all missing startup
	// stages instead of stopping at the first optional tool.
	var trusted, loaded bool
	var n int
	m.post(func() {
		trusted, loaded = m.run.trusted, m.run.loaded
		n = len(m.run.configs)
	})
	r.Run = trusted && loaded
	_ = n
	return r
}

// readinessReason names the first missing state, for operations that need it
// (section 8: 未就绪时，相关操作显示准确原因).
func (r Readiness) readinessReason() string {
	switch {
	case !r.Backend:
		return "The backend is not connected"
	case !r.Project:
		return "The project is not open yet"
	case !r.Java && r.Importing:
		return "The Java project is importing; wait for it to finish"
	case !r.Java:
		return fmt.Sprintf("Java language services are not ready (%s)", r.JavaDetail)
	case !r.BuildModel:
		return "The build model is not loaded yet"
	case !r.Run:
		return "Run configurations are not loaded (project untrusted or still loading)"
	}
	return ""
}

// refreshReadiness caches the snapshot for the UI and status bar.
func (m *Model) refreshReadiness() {
	if m.mgr == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		r := m.readinessNow(ctx)
		m.post(func() {
			if m.Root() == "" {
				return
			}
			m.ready = &r
			m.invalidate()
		})
	}()
}

// readyFor reports startup progress for the Run tool window.
func (m *Model) readyFor() string {
	if m.ready == nil {
		// The snapshot has not been taken yet (no project open): the
		// operation's own checks report their reasons.
		return ""
	}
	return m.readiness().readinessReason()
}

// readiness returns the cached snapshot; an empty one reads as not ready.
func (m *Model) readiness() Readiness {
	if m.ready != nil {
		return *m.ready
	}
	return Readiness{}
}
