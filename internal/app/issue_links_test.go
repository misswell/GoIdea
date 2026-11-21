package app

import "testing"

func TestLinkifyIssueTextUsesEarliestMatchesAndGroups(t *testing.T) {
	links := []IssueLinkPrefs{
		{Pattern: `[A-Z][A-Z0-9]+-\d+`, Link: "https://jira.example/browse/$0"},
		{Pattern: `#(\d+)`, Link: "https://github.example/issues/$1"},
	}
	got := linkifyIssueText("Fix OPENIDEA-12 and #7.", links)
	want := []issueTextSegment{
		{Text: "Fix "},
		{Text: "OPENIDEA-12", URL: "https://jira.example/browse/OPENIDEA-12"},
		{Text: " and "},
		{Text: "#7", URL: "https://github.example/issues/7"},
		{Text: "."},
	}
	if !sameIssueSegments(got, want) {
		t.Fatalf("segments = %+v, want %+v", got, want)
	}
}

func TestLinkifyIssueTextSkipsInvalidPatternsAndUnsafeLinks(t *testing.T) {
	links := []IssueLinkPrefs{
		{Pattern: `(`, Link: "https://invalid.example/$0"},
		{Pattern: `ID-\d+`, Link: "javascript:alert($0)"},
	}
	got := linkifyIssueText("ID-42", links)
	if len(got) != 1 || got[0].Text != "ID-42" || got[0].URL != "javascript:alert(ID-42)" {
		t.Fatalf("segments = %+v", got)
	}
	if validIssueLinkURL(got[0].URL) {
		t.Fatal("unsafe issue link was accepted")
	}
	if !validIssueLinkURL("HTTPS://example.test/issue") || validIssueLinkURL("file:///tmp/a") {
		t.Fatal("issue URL safety check is wrong")
	}
}

func sameIssueSegments(a, b []issueTextSegment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
