package openidea

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCompletionItemRoundTripsResolveFields(t *testing.T) {
	raw := []byte(`{
		"label":"sum",
		"labelDetail":"(int a, int b)",
		"labelDescription":null,
		"detail":"int sum(int a, int b)",
		"kind":"method",
		"documentation":null,
		"sortText":"0001",
		"filterText":"sum",
		"preselect":true,
		"deprecated":false,
		"insertText":"sum(${1:a}, ${2:b})",
		"isSnippet":true,
		"replaceRange":null,
		"replaceRangeTab":null,
		"additionalEdits":[],
		"commitCharacters":[],
		"command":null,
		"data":{"resolve":"sum"},
		"server":9
	}`)
	var item CompletionItem
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	if item.Server != 9 || string(item.Data) != `{"resolve":"sum"}` {
		t.Fatalf("resolve fields = %+v", item)
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"sortText":"0001"`, `"preselect":true`, `"additionalEdits":[]`} {
		if !bytes.Contains(encoded, []byte(field)) {
			t.Fatalf("resolve payload lost %s: %s", field, encoded)
		}
	}
}
