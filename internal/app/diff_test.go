package app

import (
	"strings"
	"testing"

	"mayoi/internal/openidea"
	"mayoi/internal/textdiff"
)

func TestShowDiffNeedsAProject(t *testing.T) {
	m := &Model{}
	m.ShowDiff("/tmp/x")
	if m.diff != nil {
		t.Fatalf("compare view opened without a project")
	}
}

func TestCloseDiffDropsView(t *testing.T) {
	m := &Model{diff: &diffView{open: true}}
	m.closeDiff()
	if m.diff != nil {
		t.Fatalf("compare view stayed open")
	}
}

func TestDiffErrorTextNamesMissingGit(t *testing.T) {
	err := &openidea.Error{Kind: "unavailable", Message: "not a git repository"}
	if got := diffErrorText(err); got != "当前项目不是 Git 仓库" {
		t.Fatalf("diffErrorText = %q", got)
	}
	if got := diffErrorText(textdiffError{}); !strings.HasPrefix(got, "读取差异失败：") {
		t.Fatalf("diffErrorText = %q", got)
	}
}

type textdiffError struct{}

func (textdiffError) Error() string { return "boom" }

func TestDiffRowText(t *testing.T) {
	row := textdiff.Row{Kind: textdiff.Add, Old: 4, New: 5, Text: "x"}
	if got := diffRowText(row); got != "+ 4:5 x" {
		t.Fatalf("diffRowText = %q", got)
	}
}
