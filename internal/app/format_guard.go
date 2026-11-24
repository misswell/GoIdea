package app

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mayoi/internal/openidea"
	"mayoi/internal/position"
)

type formatTextChange struct {
	from   int
	to     int
	insert string
}

type formatGuard struct {
	doNotFormat          []string
	formatterTagsEnabled bool
	formatterOffTag      string
	formatterOnTag       string
	formatterTagsRegexp  bool
}

func (m *Model) formatGuardForDocument() formatGuard {
	return formatGuard{
		doNotFormat: m.nativeRuntimeStringList(
			settingsPageCodeStyle, "editor.codeStyle.doNotFormat",
		),
		formatterTagsEnabled: m.nativeRuntimeBool(
			settingsPageCodeStyle, "editor.codeStyle.formatterTagsEnabled",
		),
		formatterOffTag: m.nativeRuntimeString(
			settingsPageCodeStyle, "editor.codeStyle.formatterOffTag",
		),
		formatterOnTag: m.nativeRuntimeString(
			settingsPageCodeStyle, "editor.codeStyle.formatterOnTag",
		),
		formatterTagsRegexp: m.nativeRuntimeBool(
			settingsPageCodeStyle, "editor.codeStyle.formatterTagsRegexp",
		),
	}
}

func formatPathExcluded(path, root string, globs []string) bool {
	rel := path
	if root != "" && strings.HasPrefix(path, filepath.Clean(root)+string(filepath.Separator)) {
		rel, _ = filepath.Rel(filepath.Clean(root), path)
	}
	rel = filepath.ToSlash(rel)
	segments := strings.Split(strings.Trim(rel, "/"), "/")
	valid := segments[:0]
	for _, segment := range segments {
		if segment != "" && segment != "." {
			valid = append(valid, segment)
		}
	}
	segments = valid
	for _, raw := range globs {
		glob := strings.TrimSpace(filepath.ToSlash(raw))
		glob = strings.TrimPrefix(glob, "./")
		if glob == "" {
			continue
		}
		if !strings.Contains(glob, "/") {
			re := formatGlobRegexp(glob)
			for _, segment := range segments {
				if re.MatchString(segment) {
					return true
				}
			}
			continue
		}
		glob = strings.Trim(glob, "/")
		re := formatGlobRegexp(glob)
		for i := len(segments); i > 0; i-- {
			if re.MatchString(strings.Join(segments[:i], "/")) {
				return true
			}
		}
	}
	return false
}

func formatGlobRegexp(glob string) *regexp.Regexp {
	var expression strings.Builder
	expression.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch glob[i] {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				expression.WriteString(".*")
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++
				}
			} else {
				expression.WriteString(`[^/]*`)
			}
		case '?':
			expression.WriteString(`[^/]`)
		default:
			expression.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	expression.WriteString("$")
	return regexp.MustCompile(expression.String())
}

func formatMarkerRegexp(tag string, regularExpression bool) *regexp.Regexp {
	if strings.TrimSpace(tag) == "" {
		return nil
	}
	if !regularExpression {
		tag = regexp.QuoteMeta(tag)
	}
	re, err := regexp.Compile(tag)
	if err != nil {
		return nil
	}
	return re
}

var formatCommentBefore = regexp.MustCompile(`(//|#|/\*|<!--|--|;|\*|%|\{-|\(\*|')`)

func formatMarkerLines(text string, re *regexp.Regexp) []formatTextChange {
	if re == nil {
		return nil
	}
	matches := re.FindAllStringIndex(text, -1)
	var out []formatTextChange
	lastStart := -1
	for _, match := range matches {
		if match[0] == match[1] {
			continue
		}
		start := strings.LastIndex(text[:match[0]], "\n") + 1
		end := strings.Index(text[match[1]:], "\n")
		if end < 0 {
			end = len(text)
		} else {
			end += match[1]
		}
		if start == lastStart || !formatCommentBefore.MatchString(text[start:match[0]]) {
			continue
		}
		out = append(out, formatTextChange{from: start, to: end})
		lastStart = start
	}
	return out
}

func formatProtectedRegions(text string, guard formatGuard) []formatTextChange {
	if !guard.formatterTagsEnabled {
		return nil
	}
	off := formatMarkerRegexp(guard.formatterOffTag, guard.formatterTagsRegexp)
	on := formatMarkerRegexp(guard.formatterOnTag, guard.formatterTagsRegexp)
	if off == nil {
		return nil
	}
	offs := formatMarkerLines(text, off)
	ons := formatMarkerLines(text, on)
	regions := make([]formatTextChange, 0, len(offs))
	position := 0
	for _, marker := range offs {
		if marker.from < position {
			continue
		}
		end := len(text)
		for _, close := range ons {
			if close.from > marker.from {
				end = close.to
				break
			}
		}
		regions = append(regions, formatTextChange{from: marker.from, to: end})
		position = end
		if end == len(text) {
			break
		}
	}
	return regions
}

func formatTouches(change, region formatTextChange) bool {
	if change.from == change.to {
		return change.from > region.from && change.from < region.to
	}
	return change.from < region.to && change.to > region.from
}

func narrowFormatChange(text string, change formatTextChange) *formatTextChange {
	if change.from < 0 {
		change.from = 0
	}
	if change.to < change.from {
		return nil
	}
	if change.to > len(text) {
		change.to = len(text)
	}
	old := text[change.from:change.to]
	if old == change.insert {
		return nil
	}
	maxShared := min(len(old), len(change.insert))
	start := 0
	for start < maxShared && old[start] == change.insert[start] {
		start++
	}
	oldEnd, insertEnd := len(old), len(change.insert)
	for oldEnd > start && insertEnd > start && old[oldEnd-1] == change.insert[insertEnd-1] {
		oldEnd--
		insertEnd--
	}
	narrowed := formatTextChange{
		from: change.from + start, to: change.from + oldEnd,
		insert: change.insert[start:insertEnd],
	}
	return &narrowed
}

func guardFormatChanges(text string, changes []formatTextChange, guard formatGuard) []formatTextChange {
	regions := formatProtectedRegions(text, guard)
	if len(regions) == 0 {
		return append([]formatTextChange(nil), changes...)
	}
	var out []formatTextChange
	for _, raw := range changes {
		narrowed := narrowFormatChange(text, raw)
		if narrowed == nil {
			continue
		}
		change := *narrowed
		var hits []formatTextChange
		for _, region := range regions {
			if formatTouches(change, region) {
				hits = append(hits, region)
			}
		}
		if len(hits) == 0 {
			out = append(out, change)
			continue
		}
		for _, region := range hits {
			if region.from < change.from || region.to > change.to {
				hits = nil
				break
			}
		}
		if hits == nil {
			continue
		}
		local := formatProtectedRegions(change.insert, guard)
		if len(local) != len(hits) {
			continue
		}
		oldPosition, newPosition := change.from, 0
		for i := 0; i <= len(hits); i++ {
			oldEnd := change.to
			newEnd := len(change.insert)
			if i < len(hits) {
				oldEnd, newEnd = hits[i].from, local[i].from
			}
			piece := formatTextChange{
				from: oldPosition, to: oldEnd,
				insert: change.insert[newPosition:newEnd],
			}
			if narrowed := narrowFormatChange(text, piece); narrowed != nil {
				out = append(out, *narrowed)
			}
			if i < len(hits) {
				oldPosition, newPosition = hits[i].to, local[i].to
			}
		}
	}
	return out
}

func guardFormatTextEdits(text string, edits []openidea.TextEdit, guard formatGuard) []openidea.TextEdit {
	if len(formatProtectedRegions(text, guard)) == 0 {
		return append([]openidea.TextEdit(nil), edits...)
	}
	mapper := position.NewMapper(text)
	changes := make([]formatTextChange, 0, len(edits))
	for _, edit := range edits {
		from := mapper.UTF16PositionToByteOffset(position.StringText(text), position.Position{
			Line: edit.Range.Start.Line, Character: edit.Range.Start.Col,
		})
		to := mapper.UTF16PositionToByteOffset(position.StringText(text), position.Position{
			Line: edit.Range.End.Line, Character: edit.Range.End.Col,
		})
		changes = append(changes, formatTextChange{from: from, to: to, insert: edit.NewText})
	}
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].from == changes[j].from {
			return changes[i].to < changes[j].to
		}
		return changes[i].from < changes[j].from
	})
	guarded := guardFormatChanges(text, changes, guard)
	out := make([]openidea.TextEdit, 0, len(guarded))
	for _, change := range guarded {
		start := mapper.ByteOffsetToUTF16Position(position.StringText(text), change.from)
		end := mapper.ByteOffsetToUTF16Position(position.StringText(text), change.to)
		out = append(out, openidea.TextEdit{
			Range: openidea.OutlineRange{
				Start: openidea.OutlinePos{Line: start.Line, Col: start.Character},
				End:   openidea.OutlinePos{Line: end.Line, Col: end.Character},
			},
			NewText: change.insert,
		})
	}
	return out
}
