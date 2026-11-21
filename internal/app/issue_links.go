package app

import (
	"regexp"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

type issueTextSegment struct {
	Text string
	URL  string
}

type issueMatch struct {
	start int
	end   int
	url   string
}

// linkifyIssueText mirrors OpenIDEA's issueLinks.tsx: every valid configured
// pattern contributes matches, the earliest match wins, and $0…$9 expand to
// the complete match and its capture groups.
func linkifyIssueText(text string, links []IssueLinkPrefs) []issueTextSegment {
	if text == "" {
		return nil
	}
	var matches []issueMatch
	raw := []byte(text)
	for _, link := range links {
		pattern := strings.TrimSpace(link.Pattern)
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		for _, loc := range re.FindAllSubmatchIndex(raw, -1) {
			if loc[0] == loc[1] {
				continue
			}
			url := string(re.Expand(nil, []byte(link.Link), raw, loc))
			matches = append(matches, issueMatch{start: loc[0], end: loc[1], url: url})
		}
	}
	if len(matches) == 0 {
		return []issueTextSegment{{Text: text}}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start != matches[j].start {
			return matches[i].start < matches[j].start
		}
		return matches[i].end > matches[j].end
	})
	out := make([]issueTextSegment, 0, len(matches)*2+1)
	at := 0
	for _, match := range matches {
		if match.start < at {
			continue
		}
		if match.start > at {
			out = append(out, issueTextSegment{Text: text[at:match.start]})
		}
		out = append(out, issueTextSegment{Text: text[match.start:match.end], URL: match.url})
		at = match.end
	}
	if at < len(text) {
		out = append(out, issueTextSegment{Text: text[at:]})
	}
	return out
}

func validIssueLinkURL(url string) bool {
	lower := strings.ToLower(strings.TrimSpace(url))
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")
}

// issueText draws a commit subject or message line with issue ids as native
// links. A segment without a safe URL stays plain text.
func issueText(c *ui.Context, oi oiColors, text string, links []IssueLinkPrefs) ui.Element {
	return issueTextWithWeight(c, oi, text, links, false)
}

func issueSubjectText(c *ui.Context, oi oiColors, text string, links []IssueLinkPrefs) ui.Element {
	return issueTextWithWeight(c, oi, text, links, true)
}

func issueTextWithWeight(c *ui.Context, oi oiColors, text string, links []IssueLinkPrefs, bold bool) ui.Element {
	return ui.Row(c).Wrap().Gap(0).Children(func() {
		for _, segment := range linkifyIssueText(text, links) {
			el := ui.Text(c, segment.Text).FontSize(12).TextColor(oi.text)
			if bold {
				el.Bold()
			}
			if validIssueLinkURL(segment.URL) {
				url := segment.URL
				el.TextColor(oi.link).Cursor(ui.CursorPointer).OnClick(func() { c.OpenURL(url) })
			}
		}
	})
}

func issueLinkPatternInvalid(link IssueLinkPrefs) bool {
	if strings.TrimSpace(link.Pattern) == "" {
		return false
	}
	_, err := regexp.Compile(link.Pattern)
	return err != nil
}
