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
		{"src/app.tsx", "TypeScript"},
		{"config/settings.json", "JSON"},
		{"src/app.py", "Python"},
		{"src/lib.rs", "Rust"},
		{"scripts/build.sh", "Shell"},
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
		{"src/service.proto", "Protobuf"},
		{"src/query.graphql", "GraphQL"},
		{"src/main.zig", "Zig"},
		{"build.gradle", ""},
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
