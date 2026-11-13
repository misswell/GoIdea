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
		{"build.gradle", ""},
		{"src/main/resources/application.yaml", "YAML"},
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
		if lang.LineComment == "" {
			t.Errorf("%s: no line comment", c.path)
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
