package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProjectStructureSettingsNormalizeAndDiff(t *testing.T) {
	var before projectStructureSnapshot
	before.Structure = normalizeProjectStructure(projectStructureSettings{
		Libraries: []projectLibrary{{
			Name:  "legacy",
			Scope: "invalid",
		}},
		Facets: []facetConfig{
			{Kind: "invalid"},
			{Kind: "web"},
		},
		Artifacts: []artifactConfig{{
			Kind: "war",
		}},
	})
	after := before
	after.JDTLSJDK = "/jdk/21"
	after.Structure.CompilerOutput = "out"
	after.Structure.Dependencies = []string{"/workspace/shared"}

	if before.Structure.Libraries[0].Scope != "compile" {
		t.Fatalf("library scope = %q", before.Structure.Libraries[0].Scope)
	}
	if len(before.Structure.Facets) != 1 || before.Structure.Facets[0].Module != "." {
		t.Fatalf("facets = %+v", before.Structure.Facets)
	}
	if got := before.Structure.Artifacts[0].Module; got != "." {
		t.Fatalf("artifact module = %q", got)
	}

	writes := projectStructureWrites(before, after)
	var keys []string
	for _, write := range writes {
		keys = append(keys, write.key)
	}
	want := []string{
		"projectStructure.dependencies",
		"projectStructure.compilerOutput",
		"languages.java.jdtlsJdk",
	}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("writes = %v, want %v", keys, want)
	}
}

func TestProjectStructureRootMarksOverrideDetectionAndRestore(t *testing.T) {
	m := &Model{projectStructure: projectStructureState{
		draft: projectStructureSnapshot{Structure: defaultProjectStructureSettings()},
		image: projectStructureImage{Detected: []detectedProjectRoot{{
			Path: "src/main/java", Kind: "source", DetectedFrom: "maven",
		}}},
	}}

	m.MarkProjectStructureFolder("src/main/java", "")
	if got := m.projectStructure.draft.Structure.Unmarked; !reflect.DeepEqual(got, []string{"src/main/java"}) {
		t.Fatalf("unmarked = %v", got)
	}
	if got := m.projectStructureRoleFor("src/main/java"); got != "" {
		t.Fatalf("unmarked role = %q", got)
	}
	m.RestoreDetectedProjectRoots()
	if got := m.projectStructure.draft.Structure.Unmarked; len(got) != 0 {
		t.Fatalf("restored unmarked = %v", got)
	}

	m.MarkProjectStructureFolder("src/test/java", "test")
	if got := m.projectStructureRoleFor("src/test/java"); got != "test" {
		t.Fatalf("test role = %q", got)
	}
	m.MarkProjectStructureFolder("src/test/java", "")
	if got := m.projectStructureRoleFor("src/test/java"); got != "" {
		t.Fatalf("cleared role = %q", got)
	}
}

func TestProjectStructureModulesDependenciesLibrariesAndFacets(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "service")
	dependency := filepath.Join(filepath.Dir(root), "shared")
	if err := os.MkdirAll(module, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dependency, 0o755); err != nil {
		t.Fatal(err)
	}
	m := &Model{projectStructure: projectStructureState{
		draft: projectStructureSnapshot{Structure: defaultProjectStructureSettings()},
	}}
	m.root = root

	if !m.AddProjectStructureModule(module) {
		t.Fatal("AddProjectStructureModule rejected a child directory")
	}
	if m.projectStructure.moduleKey != "service" {
		t.Fatalf("selected module = %q", m.projectStructure.moduleKey)
	}
	m.SetProjectStructureModuleField("jdk", "/jdk/21")
	if got := m.projectStructure.draft.Structure.Modules["service"].JDK; got != "/jdk/21" {
		t.Fatalf("module JDK = %q", got)
	}
	m.RemoveProjectStructureModule()
	if _, ok := m.projectStructure.draft.Structure.Modules["service"]; ok {
		t.Fatal("module was not removed")
	}

	m.ToggleProjectStructureDependency(dependency, true)
	m.ToggleProjectStructureDependency(dependency, true)
	if got := m.projectStructure.draft.Structure.Dependencies; !reflect.DeepEqual(got, []string{dependency}) {
		t.Fatalf("dependencies = %v", got)
	}
	m.ToggleProjectStructureDependency(dependency, false)
	if got := m.projectStructure.draft.Structure.Dependencies; len(got) != 0 {
		t.Fatalf("unchecked dependencies = %v", got)
	}

	m.ToggleProjectStructureMemberDependency(root, dependency, true)
	m.ToggleProjectStructureMemberDependency(root, dependency, true)
	if got := m.projectStructure.draft.Structure.MemberDependencies[root]; !reflect.DeepEqual(got, []string{dependency}) {
		t.Fatalf("member dependencies = %v", got)
	}
	m.ToggleProjectStructureMemberDependency(root, dependency, false)
	if _, ok := m.projectStructure.draft.Structure.MemberDependencies[root]; ok {
		t.Fatal("empty member dependency entry was retained")
	}

	library := filepath.Join(root, "lib", "demo.jar")
	m.AddProjectStructureLibrary(library, false)
	libraryState := m.projectStructure.draft.Structure.Libraries[0]
	if libraryState.Name != "demo" || !reflect.DeepEqual(libraryState.Classes, []string{"lib/demo.jar"}) {
		t.Fatalf("library = %+v", libraryState)
	}
	m.AppendProjectStructureLibraryPath("classes", library)
	m.AppendProjectStructureLibraryPath("sources", filepath.Join(root, "lib", "demo-sources.jar"))
	if got := len(m.projectStructure.draft.Structure.Libraries[0].Classes); got != 1 {
		t.Fatalf("library classes = %d", got)
	}
	m.SetProjectStructureLibraryField("scope", "provided")
	if got := m.projectStructure.draft.Structure.Libraries[0].Scope; got != "provided" {
		t.Fatalf("library scope = %q", got)
	}

	m.AddProjectStructureFacet("web")
	if got := m.projectStructure.draft.Structure.Facets[0]; got.Module != "." || got.WebRoot == "" {
		t.Fatalf("facet = %+v", got)
	}
	m.SetProjectStructureFacetField("kind", "spring")
	if got := m.projectStructure.draft.Structure.Facets[0].Kind; got != "spring" {
		t.Fatalf("facet kind = %q", got)
	}
	m.RemoveProjectStructureFacet()
	if got := len(m.projectStructure.draft.Structure.Facets); got != 0 {
		t.Fatalf("facets = %d", got)
	}
}

func TestProjectStructureArtifactIncludesLibraries(t *testing.T) {
	root := t.TempDir()
	m := &Model{projectStructure: projectStructureState{
		draft: projectStructureSnapshot{Structure: defaultProjectStructureSettings()},
	}}
	m.root = filepath.Join(root, "demo")
	m.projectStructure.draft.Structure.Libraries = []projectLibrary{{
		Name: "core", Classes: []string{}, Sources: []string{}, Javadocs: []string{}, Scope: "compile",
	}}
	m.AddProjectStructureArtifact("warExploded")
	artifact := m.projectStructure.draft.Structure.Artifacts[0]
	if artifact.Kind != "warExploded" || artifact.Module != "." ||
		!reflect.DeepEqual(artifact.IncludeLibraries, []string{"core"}) {
		t.Fatalf("artifact = %+v", artifact)
	}
	m.ToggleProjectStructureArtifactLibrary("core", false)
	m.SetProjectStructureArtifactField("mainClass", "demo.Main")
	m.SetProjectStructureArtifactField("outputDirectory", filepath.Join(m.Root(), "out", "app"))
	artifact = m.projectStructure.draft.Structure.Artifacts[0]
	if len(artifact.IncludeLibraries) != 0 || artifact.MainClass != "demo.Main" ||
		artifact.OutputDirectory != "out/app" {
		t.Fatalf("edited artifact = %+v", artifact)
	}
	m.RemoveProjectStructureArtifact()
	if got := len(m.projectStructure.draft.Structure.Artifacts); got != 0 {
		t.Fatalf("artifacts = %d", got)
	}
}
