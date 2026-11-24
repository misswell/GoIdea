package app

import (
	"reflect"
	"testing"

	"mayoi/internal/openidea"
)

func TestDependencyManifestNamesMatchOpenIDEA(t *testing.T) {
	for _, path := range []string{
		"Cargo.toml",
		"/workspace/Cargo.toml",
		"go.mod",
		`C:\workspace\package.json`,
		"/workspace/package.json",
	} {
		if !isDependencyManifest(path) {
			t.Fatalf("%q is not recognized as a dependency manifest", path)
		}
	}
	for _, path := range []string{"Cargo.lock", "go.sum", "package-lock.json", "pom.xml"} {
		if isDependencyManifest(path) {
			t.Fatalf("%q should not use dependency version completion", path)
		}
	}
}

func TestDependencyCompletionPreservesOperatorAndFiltersTypedVersion(t *testing.T) {
	text := "serde = \"^1.2\"\n"
	from := len("serde = \"")
	to := from + len("^1.2")
	cursor := from + len("^1")
	found := []openidea.DependencyVersions{{
		Dependency: openidea.ManifestDependency{
			Package: "serde", Requirement: "^1.2",
			From: byteToUTF16Offset(text, from), To: byteToUTF16Offset(text, to),
		},
		Versions: []string{"1.10.0", "2.0.0", "1.2.3"},
	}}

	start, items := dependencyCompletionItems(
		text, byteToUTF16Offset(text, cursor), false, found)
	if start != from+1 {
		t.Fatalf("completion start = %d, want %d", start, from+1)
	}
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	if want := []string{"1.10.0", "1.2.3"}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	for _, item := range items {
		if item.Insert != item.Label || item.Detail != "serde" {
			t.Fatalf("completion = %+v", item)
		}
	}
}

func TestDependencyCompletionExplicitShowsEveryVersionInBackendOrder(t *testing.T) {
	text := "crate = \">= 2\""
	from := len("crate = \"")
	to := from + len(">= 2")
	found := []openidea.DependencyVersions{{
		Dependency: openidea.ManifestDependency{
			Package: "crate", Requirement: ">= 2",
			From: byteToUTF16Offset(text, from), To: byteToUTF16Offset(text, to),
		},
		Versions: []string{"9.0.0", "2.1.0", "3.0.0"},
	}}
	start, items := dependencyCompletionItems(
		text, byteToUTF16Offset(text, to), true, found)
	if start != from+len(">= ") {
		t.Fatalf("completion start = %d", start)
	}
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
		if item.Insert != item.Label {
			t.Fatalf("insert = %q for %q", item.Insert, item.Label)
		}
	}
	if want := found[0].Versions; !reflect.DeepEqual(labels, want) {
		t.Fatalf("explicit labels = %v, want %v", labels, want)
	}
}

func TestDependencyCompletionUsesUTF16OffsetsAroundAstralText(t *testing.T) {
	text := "notes = \"😀\"\nserde = \"1.2\"\n"
	from := len("notes = \"😀\"\nserde = \"")
	to := from + len("1.2")
	found := []openidea.DependencyVersions{{
		Dependency: openidea.ManifestDependency{
			Package: "serde", Requirement: "1.2",
			From: byteToUTF16Offset(text, from), To: byteToUTF16Offset(text, to),
		},
		Versions: []string{"1.2.3"},
	}}
	start, items := dependencyCompletionItems(
		text, byteToUTF16Offset(text, from+1), false, found)
	if start != from || len(items) != 1 || items[0].Label != "1.2.3" {
		t.Fatalf("completion = start %d items %+v", start, items)
	}
}

func TestDependencyCompletionRejectsRangeWithoutDependency(t *testing.T) {
	text := "name = \"crate\"\nversion = \"1.2\""
	found := []openidea.DependencyVersions{{
		Dependency: openidea.ManifestDependency{
			Package: "crate", Requirement: "1.2",
			From: byteToUTF16Offset(text, 26), To: byteToUTF16Offset(text, 29),
		},
		Versions: []string{"1.2.3"},
	}}
	if start, items := dependencyCompletionItems(text, 2, false, found); start != -1 || len(items) != 0 {
		t.Fatalf("outside completion = start %d items %+v", start, items)
	}
}

func TestDependencyIntentBuildsCargoUpdateQuickFix(t *testing.T) {
	text := "serde = \"^1.0\"\n"
	from := len("serde = \"")
	to := from + len("^1.0")
	found := []openidea.DependencyVersions{{
		Dependency: openidea.ManifestDependency{
			Package: "serde", Requirement: "^1.0",
			From: byteToUTF16Offset(text, from), To: byteToUTF16Offset(text, to),
		},
		Update: &openidea.DependencyUpdate{
			Latest: "1.0.219", Replacement: "^1.0.219",
		},
	}}
	items := dependencyIntentItems(text, from+2, 17, found)
	if len(items) != 1 {
		t.Fatalf("dependency intents = %+v", items)
	}
	item := items[0]
	if item.title != "Update to ^1.0.219" || item.kind != "quickfix" ||
		item.dependency == nil {
		t.Fatalf("quick fix = %+v", item)
	}
	edit := item.dependency
	if edit.from != from || edit.to != to ||
		edit.replacement != "^1.0.219" || edit.latest != "1.0.219" ||
		edit.version != 17 {
		t.Fatalf("dependency edit = %+v", edit)
	}
	if items := dependencyIntentItems(text, 0, 17, found); len(items) != 0 {
		t.Fatalf("quick fix outside the dependency = %+v", items)
	}
}
