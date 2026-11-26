package docbridge

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/openidea"
)

// TestDocumentLanguageCoverage checks that an opened file carries the
// language MyGo knows it by, so the editor colors it and comments, closes and
// indents its text: the bridge sets it once, on every document.
func TestDocumentLanguageCoverage(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"src/main/java/com/example/App.java", "Java"},
		{"src/main/kotlin/App.kt", "Kotlin"},
		{"src/main.c", "C"},
		{"src/main.cpp", "C++"},
		{"src/App.scala", "Scala"},
		{"src/App.swift", "Swift"},
		{"src/App.dart", "Dart"},
		{"src/App.cs", "C#"},
		{"src/app.js", "JavaScript"},
		{"src/app.tsx", "TypeScript JSX"},
		{"config/settings.json", "JSON with Comments"},
		{"src/app.py", "Python"},
		{"src/lib.rs", "Rust"},
		{"scripts/build.sh", "Shell Script"},
		{"src/query.sql", "SQL"},
		{"src/analysis.r", "R"},
		{"src/main.lua", "Lua"},
		{"src/index.php", "PHP"},
		{"src/site.css", "CSS"},
		{"src/index.html", "HTML"},
		{"src/service.xml", "XML"},
		{"README.md", "Markdown"},
		{"fix.patch", "Diff"},
		{"Dockerfile", "Dockerfile"},
		{"Makefile", "Makefile"},
		{"src/service.proto", "Protocol Buffers"},
		{"src/query.graphql", "GraphQL"},
		{"src/main.zig", "Zig"},
		{"build.gradle", "Groovy"},
		{"src/main/resources/application.yaml", "YAML"},
		{"Cargo.toml", "TOML"},
		{"src/index.rb", "Ruby"},
	}
	for _, c := range cases {
		doc := newDocument(nil, "w", openidea.DocInfo{Path: c.path, Text: "class A {}\n", Version: 1})
		lang := doc.Buffer().Language()
		if c.want == "" {
			if lang != nil {
				t.Errorf("%s: language %q, want plain text", c.path, lang.Name)
			}
			continue
		}
		if lang == nil || lang.Name != c.want {
			t.Errorf("%s: language %v, want %s", c.path, lang, c.want)
			continue
		}
		if lang.Tokenizer == nil && lang.Highlighter == nil &&
			lang.LineComment == "" && lang.BlockComment == [2]string{} {
			t.Errorf("%s: language has no editor behavior", c.path)
		}
	}
}

// TestJavaTextIsTokenized checks that the Java language the bridge attaches
// tokenizes Java: keywords are keywords, so the colors follow the code.
func TestJavaTextIsTokenized(t *testing.T) {
	doc := newDocument(nil, "w", openidea.DocInfo{
		Path:    "src/main/java/App.java",
		Text:    "public class App {}\n",
		Version: 1,
	})
	lang := doc.Buffer().Language()
	if lang == nil || lang.Tokenizer == nil {
		t.Fatal("no Java tokenizer")
	}
	tokens, _ := lang.Tokenizer.Tokenize("public class App {}", 0, nil)
	if len(tokens) == 0 {
		t.Fatal("no tokens")
	}
	if tokens[0].Kind != editor.TokenKeyword || tokens[0].End != len("public") {
		t.Fatalf("first token = %+v, want the keyword public", tokens[0])
	}
}

func TestDocumentUsesBackendLanguageIDOverFilename(t *testing.T) {
	doc := newDocument(nil, "w", openidea.DocInfo{
		Path: "templates/account.html", Text: "{% block body %}Hello{% endblock %}\n",
		Language: "django-html", Version: 1,
	})
	language := doc.Buffer().Language()
	if language == nil || language.Name != "Django Template" {
		t.Fatalf("language = %#v, want Django Template", language)
	}
	if language.BlockComment != [2]string{"{#", "#}"} {
		t.Fatalf("Django comments = %#v", language.BlockComment)
	}
	doc = newDocument(nil, "w", openidea.DocInfo{
		Path: "source.unknown", Text: "package main\n", Language: "go", Version: 1,
	})
	if language := doc.Buffer().Language(); language == nil || language.Name != "Go" {
		t.Fatalf("backend-language override = %#v, want Go", language)
	}
}

func TestDocumentLanguageAssociationCoverage(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{"templates/view.html.erb", "ERB"},
		{"src/App.jsx", "JavaScript JSX"},
		{"src/App.tsx", "TypeScript JSX"},
		{"tsconfig.build.json", "JSON with Comments"},
		{".env.local", ".env"},
		{".gitattributes", "Git Attributes"},
		{"go.mod", "Go Module"},
		{"go.work", "Go Workspace"},
		{"infra/main.tf", "Terraform"},
		{"infra/values.tfvars", "Terraform Variables"},
		{"CMakeLists.txt", "CMake"},
		{"schema.prisma", "Prisma"},
		{"module.mli", "OCaml Interface"},
		{"tool.ps1", "PowerShell"},
	} {
		doc := newDocument(nil, "w", openidea.DocInfo{Path: test.path, Text: "", Version: 1})
		language := doc.Buffer().Language()
		if language == nil || language.Name != test.want {
			t.Errorf("%s: %v, want %s", test.path, language, test.want)
		}
	}
}

func TestLanguageCommentsAndIndentationFollowBackend(t *testing.T) {
	for _, test := range []struct {
		path      string
		line      string
		block     [2]string
		tabSize   int
		hardTabs  bool
		tokenized bool
	}{
		{"main.go", "//", [2]string{"/*", "*/"}, 4, true, true},
		{"build.gradle", "//", [2]string{"/*", "*/"}, 4, false, true},
		{"module.mli", "", [2]string{"(*", "*)"}, 2, false, true},
		{"style.scss", "//", [2]string{"/*", "*/"}, 2, false, true},
		{"index.htm", "", [2]string{"<!--", "-->"}, 2, false, true},
	} {
		doc := newDocument(nil, "w", openidea.DocInfo{Path: test.path, Text: "", Version: 1})
		language := doc.Buffer().Language()
		if language == nil {
			t.Errorf("%s: no language", test.path)
			continue
		}
		if language.LineComment != test.line || language.BlockComment != test.block {
			t.Errorf("%s comments = %q %#v", test.path, language.LineComment, language.BlockComment)
		}
		if language.TabSize != test.tabSize || language.HardTabs != test.hardTabs {
			t.Errorf("%s indentation = %d hard=%v", test.path, language.TabSize, language.HardTabs)
		}
		if test.tokenized && language.Tokenizer == nil && language.Highlighter == nil {
			t.Errorf("%s has no syntax behavior", test.path)
		}
	}
}
