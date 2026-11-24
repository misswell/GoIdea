package app

import "testing"

func ptrString(value string) *string { return &value }

func TestHTTPBodyKindAndPrettyPrinting(t *testing.T) {
	if got := httpBodyKind(ptrString("application/json"), `{"a":1}`); got != httpBodyJSON {
		t.Fatalf("JSON kind = %q", got)
	}
	if got := httpBodyKind(nil, `<root><a>x</a><b/></root>`); got != httpBodyXML {
		t.Fatalf("XML kind = %q", got)
	}
	if got := httpBodyKind(nil, `<!doctype html><html><body>Hello</body></html>`); got != httpBodyHTML {
		t.Fatalf("HTML kind = %q", got)
	}
	if got := httpBodyKind(nil, `not json or markup`); got != httpBodyText {
		t.Fatalf("text kind = %q", got)
	}
	if got, want := httpPrettyJSON(`{"a":1,"b":[2,3]}`),
		"{\n  \"a\": 1,\n  \"b\": [\n    2,\n    3\n  ]\n}"; got != want {
		t.Fatalf("pretty JSON:\n%s\nwant:\n%s", got, want)
	}
	if got, want := httpPrettyMarkup(`<root><a>x</a><b/></root>`, false, "  "),
		"<root>\n  <a>x</a>\n  <b/>\n</root>"; got != want {
		t.Fatalf("pretty XML:\n%s\nwant:\n%s", got, want)
	}
	if got, want := httpPrettyMarkup(`<html><body><p>Hi</p><script>if (a < b) {}</script></body></html>`,
		true, "  "),
		"<html>\n  <body>\n    <p>Hi</p>\n    <script>if (a < b) {}</script>\n  </body>\n</html>"; got != want {
		t.Fatalf("pretty HTML:\n%s\nwant:\n%s", got, want)
	}
}

func TestHTTPByteFormattingMatchesTheServiceView(t *testing.T) {
	for _, test := range []struct {
		size float64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1536, "1.5 KB"},
		{2 * 1024 * 1024, "2.0 MB"},
	} {
		if got := formatHTTPBytes(test.size); got != test.want {
			t.Errorf("formatHTTPBytes(%v) = %q, want %q", test.size, got, test.want)
		}
	}
}
