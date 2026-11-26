package highlight

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/BurntSushi/toml"
)

type contributingLanguage struct {
	ID           string   `json:"id" toml:"id"`
	Name         string   `json:"name" toml:"name"`
	Extensions   []string `json:"extensions" toml:"extensions"`
	Filenames    []string `json:"filenames" toml:"filenames"`
	UIMode       string   `json:"uiMode" toml:"ui-mode"`
	LineComment  string   `json:"lineComment" toml:"line-comment"`
	BlockComment []string `json:"blockComment" toml:"block-comment"`
	Indent       struct {
		TabSize      int  `json:"tabSize" toml:"tab-size"`
		InsertSpaces bool `json:"insertSpaces" toml:"insert-spaces"`
	} `json:"indent" toml:"indent"`
	TreeSitter      string `json:"treeSitter" toml:"tree-sitter"`
	LanguageServers []struct {
		Name string `json:"name" toml:"name"`
	} `json:"languageServers" toml:"language-servers"`
	Debugger map[string]any `json:"debugger" toml:"debugger"`
}

func TestLanguageCatalogMatchesOpenIDEAContributions(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "third_party", "openidea", "languages")
	paths, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 82 {
		t.Fatalf("read %d OpenIDEA language contributions, want at least 82", len(paths))
	}
	want := make(map[string]Language, len(paths))
	for _, path := range paths {
		var definition contributingLanguage
		if _, err := toml.DecodeFile(path, &definition); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		blockComment := [2]string{}
		if len(definition.BlockComment) == 2 {
			copy(blockComment[:], definition.BlockComment)
		}
		var servers []string
		for _, server := range definition.LanguageServers {
			servers = append(servers, server.Name)
		}
		want[definition.ID] = Language{
			ID:              definition.ID,
			Name:            definition.Name,
			Extensions:      definition.Extensions,
			Filenames:       definition.Filenames,
			UIMode:          definition.UIMode,
			LineComment:     definition.LineComment,
			BlockComment:    blockComment,
			TabSize:         definition.Indent.TabSize,
			InsertSpaces:    definition.Indent.InsertSpaces,
			TreeSitter:      definition.TreeSitter,
			LanguageServers: servers,
			Debugger:        definition.Debugger != nil,
		}
	}
	got := make(map[string]Language, len(languageCatalog))
	for _, definition := range Languages() {
		if _, exists := got[definition.ID]; exists {
			t.Errorf("%s is duplicated", definition.ID)
		}
		got[definition.ID] = definition
	}
	for id := range want {
		if _, exists := got[id]; !exists {
			t.Errorf("%s is missing", id)
		}
	}
	for id := range want {
		expected, actual := want[id], got[id]
		if actual.ID == "" {
			continue
		}
		actual.Core = ""
		if len(actual.Extensions) == 0 {
			actual.Extensions = nil
		}
		if len(actual.Filenames) == 0 {
			actual.Filenames = nil
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s metadata differs\n got: %#v\nwant: %#v", id, actual, expected)
		}
	}
}

func TestOpenIDEALanguageDetection(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{"Main.java", "java"},
		{"App.tsx", "typescriptreact"},
		{"index.d.ts", "typescript"},
		{"Dockerfile.debug", "dockerfile"},
		{"tsconfig.build.json", "jsonc"},
		{"go.mod", "go.mod"},
		{"go.work", "go.work"},
		{".env.local", "dotenv"},
		{".gitattributes", "gitattributes"},
		{"view.html.erb", "erb"},
		{"main.tf", "terraform"},
		{"values.tfvars", "terraform-vars"},
		{"CMakeLists.txt", "cmake"},
		{"Cargo.toml", "toml"},
		{"component.vue", "vue"},
		{"App.jsx", "javascriptreact"},
		{"script.ps1", "powershell"},
		{"schema.prisma", "prisma"},
		{"main.mli", "ocaml.interface"},
	} {
		language := LanguageFor(test.path)
		if language == nil || language.ID != test.want {
			t.Errorf("%s: %#v, want %s", test.path, language, test.want)
		}
	}
}

func TestLanguageDetectionUsesLongestCompoundExtension(t *testing.T) {
	if language := LanguageFor("query.graphqls"); language == nil || language.ID != "graphql" {
		t.Fatalf("graphqls: %#v", language)
	}
	if language := LanguageFor("schema.json.map"); language == nil || language.ID != "json" {
		t.Fatalf("json.map: %#v", language)
	}
	if language := LanguageFor("module.tftest.hcl"); language == nil || language.ID != "terraform" {
		t.Fatalf("tftest.hcl: %#v", language)
	}
}

func TestLanguageCatalogIncludesAllMatrixGaps(t *testing.T) {
	missingAndPartial := []string{
		"arduino", "asciidoc", "astro", "bat", "c", "clojure", "cmake", "cpp",
		"csharp", "css", "d", "dart", "diff", "django-html", "dockerfile", "dotenv",
		"elixir", "erb", "erlang", "fsharp", "gdscript", "gdshader", "gitattributes",
		"go", "go.mod", "go.work", "godot-resource", "graphql", "groovy", "haskell",
		"hcl", "html", "http", "ignore", "ini", "javascript", "javascriptreact",
		"json", "jsonc", "jsp", "julia", "kotlin", "latex", "less", "log", "lua",
		"makefile", "markdown", "nix", "objective-c", "objective-cpp", "ocaml",
		"ocaml.interface", "perl", "php", "plaintext", "powershell", "prisma",
		"properties", "proto", "python", "r", "restructuredtext", "ruby", "rust",
		"sass", "scala", "scss", "shellscript", "sql", "svelte", "swift", "terraform",
		"terraform-vars", "toml", "typescript", "typescriptreact", "vue", "xml",
		"yaml", "zig",
	}
	if len(missingAndPartial) != 81 {
		t.Fatalf("matrix gap list has %d rows, want 81", len(missingAndPartial))
	}
	for _, id := range missingAndPartial {
		if language := LanguageByID(id); language == nil {
			t.Errorf("%s is not registered", id)
		}
	}
}

func TestMyGoLanguageCarriesNativeBehavior(t *testing.T) {
	language := MyGoLanguage(LanguageByID("scala"))
	if language == nil || language.Name != "Scala" {
		t.Fatalf("Scala language = %#v", language)
	}
	if language.LineComment != "//" || language.BlockComment != [2]string{"/*", "*/"} {
		t.Fatalf("Scala comments = %q %#v", language.LineComment, language.BlockComment)
	}
	if !slicesContains(language.Extensions, ".scala") || !slicesContains(language.Extensions, ".sbt") {
		t.Fatalf("Scala extensions = %v", language.Extensions)
	}
	plain := MyGoLanguage(LanguageByID("plaintext"))
	if plain == nil || plain.Tokenizer != nil || plain.Highlighter != nil {
		t.Fatalf("plain text claimed syntax support: %#v", plain)
	}
}

func TestEveryContributionUsesBackendLanguageID(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "highlight" {
		t.Fatal("unexpected test working directory")
	}
	for _, language := range Languages() {
		if language.ID == "" || language.Name == "" {
			t.Errorf("incomplete contribution: %#v", language)
		}
		if got := LanguageByID(language.ID); got == nil || got.Name != language.Name {
			t.Errorf("%s lookup = %#v", language.ID, got)
		}
		if len(language.Extensions) > 0 {
			path := "file." + language.Extensions[0]
			if got := LanguageFor(path); got == nil {
				t.Errorf("%s: no language for %s", language.ID, path)
			}
		}
	}
}

func TestLanguageSyntaxRendererInventory(t *testing.T) {
	for _, language := range Languages() {
		native := MyGoLanguage(&language)
		if native == nil || native.Tokenizer == nil && native.Highlighter == nil {
			t.Logf("no native syntax renderer: %s (%s)", language.ID, language.Name)
		}
	}
}

func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
