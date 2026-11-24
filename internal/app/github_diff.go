package app

import (
	"strconv"
	"strings"
)

type githubDiffRow struct {
	Kind        string
	Text        string
	OldLine     int
	NewLine     int
	CommentSide string
	CommentLine int
}

// parseGitHubPatch turns GitHub's unified REST patch into rows with
// commentable old/new line numbers.
func parseGitHubPatch(patch string) []githubDiffRow {
	lines := strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n")
	rows := make([]githubDiffRow, 0, len(lines))
	oldLine, newLine := 0, 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@@ ") {
			oldLine, newLine = githubHunkLines(line)
			rows = append(rows, githubDiffRow{Kind: "hunk", Text: line})
			continue
		}
		if strings.HasPrefix(line, `\ No newline at end of file`) {
			continue
		}
		prefix := line[0]
		text := line[min(1, len(line)):]
		row := githubDiffRow{Text: text}
		switch prefix {
		case '+':
			row.Kind = "add"
			row.NewLine = newLine
			row.CommentSide, row.CommentLine = "RIGHT", newLine
			newLine++
		case '-':
			row.Kind = "remove"
			row.OldLine = oldLine
			row.CommentSide, row.CommentLine = "LEFT", oldLine
			oldLine++
		default:
			row.Kind = "context"
			row.OldLine, row.NewLine = oldLine, newLine
			row.CommentSide, row.CommentLine = "RIGHT", newLine
			oldLine++
			newLine++
		}
		rows = append(rows, row)
	}
	return rows
}

func githubHunkLines(header string) (int, int) {
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return 1, 1
	}
	oldStart := githubHunkStart(fields[1], '-')
	newStart := githubHunkStart(fields[2], '+')
	if oldStart <= 0 {
		oldStart = 1
	}
	if newStart <= 0 {
		newStart = 1
	}
	return oldStart, newStart
}

func githubHunkStart(value string, prefix byte) int {
	value = strings.TrimPrefix(value, string(prefix))
	if comma := strings.IndexByte(value, ','); comma >= 0 {
		value = value[:comma]
	}
	line, _ := strconv.Atoi(value)
	return line
}
