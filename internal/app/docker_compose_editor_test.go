package app

import (
	"strings"
	"testing"
)

func TestDockerComposeEditorCompletesAndResolvesReferences(t *testing.T) {
	text := `services:
  db:
    image: postgres
  api:
    depends_on:
      db:
        condition: service_started
    extends:
      service: db
    networks:
      - frontend
networks:
  frontend: {}
`

	for _, test := range []struct {
		name     string
		needle   string
		want     string
		notWant  string
		explicit bool
	}{
		{
			name: "extends service", needle: "      service: ", want: "db", notWant: "api", explicit: true,
		},
		{
			name: "network list", needle: "      - ", want: "frontend", explicit: true,
		},
		{
			name: "network mapping", needle: "    networks:\n      ", want: "frontend", explicit: true,
		},
		{
			name: "network list after typing", needle: "      - fr", want: "frontend", explicit: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			at := strings.Index(text, test.needle)
			if at < 0 {
				t.Fatalf("fixture lacks %q", test.needle)
			}
			pos := at + len(test.needle)
			start, items, ok := composeCompletionsAt(text, pos, test.explicit)
			if !ok || len(items) == 0 {
				t.Fatalf("completion at %d = %d, %v, %v", pos, start, items, ok)
			}
			var labels []string
			for _, item := range items {
				labels = append(labels, item.Label)
			}
			if !containsStringFold(labels, test.want) {
				t.Fatalf("completion labels = %v, want %q", labels, test.want)
			}
			if test.notWant != "" && containsStringFold(labels, test.notWant) {
				t.Fatalf("completion labels = %v, must exclude %q", labels, test.notWant)
			}
		})
	}

	serviceAt := strings.Index(text, "service: db") + len("service: ")
	declaration, ok := composeReferenceAt(text, serviceAt)
	if !ok || declaration.name != "db" {
		t.Fatalf("compose reference at %d = %+v, %v", serviceAt, declaration, ok)
	}

	comment := strings.Replace(text, "      service: db", "      service: db # keep", 1)
	commentAt := strings.Index(comment, "# keep") + len("# ")
	if _, _, ok := composeCompletionsAt(comment, commentAt, true); ok {
		t.Fatal("compose completion ran inside a comment")
	}
}
