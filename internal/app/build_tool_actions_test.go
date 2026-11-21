package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenIDEABuildToolActionsAreRegistered(t *testing.T) {
	for _, id := range []string{
		"maven.addProjects",
		"maven.importExternal",
		"maven.unlinkProjects",
		"maven.executeGoal",
		"maven.generateSources",
		"gradle.linkProjects",
		"gradle.unlinkProjects",
		"gradle.executeTask",
	} {
		if _, ok := Command(id); !ok {
			t.Errorf("missing OpenIDEA build-tool action %q", id)
		}
	}
}

func TestClassifyMavenPOMsUsesContentRootAndAbsoluteExternalPaths(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "modules", "api", "pom.xml")
	outside := filepath.Join(filepath.Dir(root), "shared", "pom.xml")
	notPOM := filepath.Join(root, "build.xml")
	link, rejected := classifyMavenPOMs(root, []string{inside, outside, notPOM, inside})
	want := []string{"modules/api/pom.xml", filepath.ToSlash(filepath.Clean(outside))}
	if !reflect.DeepEqual(link, want) {
		t.Fatalf("linked POMs = %v, want %v", link, want)
	}
	if !reflect.DeepEqual(rejected, []string{notPOM}) {
		t.Fatalf("rejected paths = %v, want %s", rejected, notPOM)
	}
}

func TestImportMavenModuleAcceptsAPOMOrDirectory(t *testing.T) {
	root := t.TempDir()
	pom := filepath.Join(root, "pom.xml")
	if err := os.WriteFile(pom, []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, picked := range []string{root, pom} {
		got, ok := mavenPOMForImport(picked)
		if !ok || got != pom {
			t.Fatalf("mavenPOMForImport(%q) = %q, %v, want %q", picked, got, ok, pom)
		}
	}
	if _, ok := mavenPOMForImport(t.TempDir()); ok {
		t.Fatal("a directory without pom.xml was accepted")
	}
}

func TestNextMavenLinkSettingsMirrorsOpenIDEALinkage(t *testing.T) {
	projects := []mavenProject{
		{Pom: "pom.xml"},
		{Pom: "modules/api/pom.xml"},
	}
	unlinked, linked := nextMavenLinkSettings(
		[]string{"modules/api/pom.xml"},
		nil,
		projects,
		[]string{"modules/api/pom.xml", "../shared/pom.xml"},
		true,
	)
	if len(unlinked) != 0 {
		t.Fatalf("linked module stayed unlinked: %v", unlinked)
	}
	if !reflect.DeepEqual(linked, []string{"../shared/pom.xml"}) {
		t.Fatalf("linked external POMs = %v", linked)
	}

	unlinked, linked = nextMavenLinkSettings(
		nil,
		[]string{"../shared/pom.xml"},
		projects,
		[]string{"pom.xml", "../shared/pom.xml"},
		false,
	)
	if !reflect.DeepEqual(linked, []string{}) {
		t.Fatalf("unlinked external POMs = %v", linked)
	}
	if !reflect.DeepEqual(unlinked, []string{"pom.xml"}) {
		t.Fatalf("discovered POM was not unlinked: %v", unlinked)
	}

	unlinked, linked = nextMavenLinkSettings(
		[]string{"pom.xml"},
		nil,
		projects,
		[]string{"pom.xml"},
		false,
	)
	if !reflect.DeepEqual(unlinked, []string{"pom.xml"}) || len(linked) != 0 {
		t.Fatalf("idempotent unlink = %v, %v", unlinked, linked)
	}
}

func TestBuildToolArgumentsAcceptOpenIDEAShapes(t *testing.T) {
	cases := []any{
		"pom.xml",
		map[string]any{"pom": "pom.xml"},
		[]byte(`{"pom":"pom.xml"}`),
	}
	for _, arg := range cases {
		if got := buildToolArgString(arg, "pom"); got != "pom.xml" {
			t.Fatalf("buildToolArgString(%#v) = %q", arg, got)
		}
	}
}
