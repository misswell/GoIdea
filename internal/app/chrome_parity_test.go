package app

import (
	"encoding/binary"
	"io/fs"
	"strings"
	"testing"
)

// chromeIconNames is every drawing the chrome asks for by name, as the
// OpenIDEA surfaces do: stripes, title bar, tabs, tree, welcome, plugins and
// the build tools.
func chromeIconNames() []string {
	return []string{
		// general
		"general/add", "general/chevronDown", "general/chevronRight",
		"general/close", "general/collapseAll", "general/expandAll",
		"general/moreVertical", "general/pin", "general/refresh",
		"general/runAnything", "general/search", "general/settings", "general/vcs",
		// run
		"run/run", "run/debug", "run/resume", "run/rerun_stroke",
		"run/restartDebug_stroke", "run/showIgnored", "run/stop", "run/stop_stroke",
		// status
		"status/error", "status/info", "status/success", "status/warning",
		// build tools
		"build/dependencyAnalyzer", "build/taskGroup", "build/toggleOfflineMode",
		"externalSystem/task", "maven/mavenGoal", "maven/mavenLoadChanges",
		"maven/mavenModule", "maven/mavenPlugin", "maven/mavenPluginGoal",
		"maven/mavenProfiles", "maven/mavenProject", "nodes/generatedSource",
		// tree and file kinds
		"nodes/folder", "nodes/libraryFolder", "nodes/module", "nodes/package",
		"nodes/sourceRoot", "fileTypes/gradle", "fileTypes/java",
		"fileTypes/unknown",
		// tool windows
		"toolwindows/notifications", "toolwindows/terminal",
	}
}

func TestChromeIconsExistInBothThemes(t *testing.T) {
	for _, name := range chromeIconNames() {
		for _, dark := range []bool{false, true} {
			for _, size := range []float32{16, 20} {
				if oiIconOf(name, size, dark) == nil {
					t.Errorf("%s (%v, dark=%v) does not load", name, size, dark)
				}
			}
		}
	}
}

func TestEveryVendoredIconParses(t *testing.T) {
	n := 0
	err := fs.WalkDir(oiIconAssets, "assets/icons", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".svg") {
			return nil
		}
		src, err := oiIconAssets.ReadFile(p)
		if err != nil {
			return err
		}
		if _, err := oiParseIcon(src, oiIsMono(src)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 900 {
		t.Fatalf("only %d icons are embedded, the upstream set is larger", n)
	}
}

// TestFileIconTablesResolve catches a table entry that names an icon the
// build does not carry: it would silently draw the unknown-file drawing.
func TestFileIconTablesResolve(t *testing.T) {
	used := map[string]bool{"file-unknown": true, "file-code": true}
	for _, icon := range fileIconNames {
		used[icon] = true
	}
	for _, p := range fileIconPatterns {
		used[p.icon] = true
	}
	for _, icon := range fileIconExts {
		used[icon] = true
	}
	for icon := range used {
		p, ok := fileIconPaths[icon]
		if !ok {
			t.Errorf("%s has no vendored drawing", icon)
			continue
		}
		if !oiIconExists(p, 16) {
			t.Errorf("%s maps to %s, which the build does not carry", icon, p)
		}
	}
	for kind, p := range folderIconPaths {
		if !oiIconExists(p, 16) {
			t.Errorf("folder %s maps to %s, which the build does not carry", kind, p)
		}
	}
}

func TestFileIconName(t *testing.T) {
	cases := map[string]string{
		"pom.xml":              "file-maven",
		"mvnw":                 "file-maven",
		"App.java":             "file-java",
		"build.gradle.kts":     "file-gradle-kts",
		"build.gradle":         "file-gradle",
		"package.json":         "file-npm",
		"package-lock.json":    "file-lock",
		"go.sum":               "file-lock",
		"pyproject.toml":       "file-pyproject",
		"Pipfile":              "file-pipenv",
		"requirements-dev.txt": "file-requirements",
		"LICENSE":              "file-license",
		"COPYING.md":           "file-license",
		"docker-compose.yml":   "file-docker",
		".env.local":           "file-dotenv",
		"README.md":            "file-md",
		"Makefile":             "file-makefile",
		"Main.kt":              "file-kotlin",
		"Main.ktx-unknown":     "file-unknown",
		"main.nim":             "file-code",
		"NOTES":                "file-text",
	}
	for name, want := range cases {
		if got := fileIconName(name); got != want {
			t.Errorf("fileIconName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestEditorLineHeight(t *testing.T) {
	if got, want := editorLineHeight(13)*13, float32(22); got != want {
		t.Fatalf("13px code has a %.1fpx line, want %.0f", got, want)
	}
	if got, want := editorLineHeight(14)*14, float32(23.6); got < want-0.01 || got > want+0.01 {
		t.Fatalf("14px code has a %.2fpx line, want %.1f", got, want)
	}
	if got, want := editorLineHeight(0)*13, float32(22); got != want {
		t.Fatalf("an unset size has a %.1fpx line, want %.0f", got, want)
	}
}

func TestOpenIDEAFontsEmbedded(t *testing.T) {
	fonts := map[string][]byte{
		"Inter Regular":       fontInterRegular,
		"Inter Medium":        fontInterMedium,
		"Inter SemiBold":      fontInterSemiBold,
		"Inter Bold":          fontInterBold,
		"JetBrains Mono":      fontJetBrainsRegular,
		"JetBrains Mono Med":  fontJetBrainsMedium,
		"JetBrains Mono Bold": fontJetBrainsBold,
	}
	for name, data := range fonts {
		if len(data) < 100000 {
			t.Errorf("%s is %d bytes, not a static TTF", name, len(data))
			continue
		}
		// A static TrueType face starts with the 1.0 sfnt version.
		if v := binary.BigEndian.Uint32(data[:4]); v != 0x00010000 && string(data[:4]) != "true" {
			t.Errorf("%s does not start with a TrueType header: %x", name, v)
		}
	}
}
