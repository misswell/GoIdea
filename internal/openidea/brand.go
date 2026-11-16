package openidea

// The backend is OpenIDEA's engine, and a few of its messages still call the
// product by its upstream name. The interface says GoIDEA, so text that comes
// back to the client as IDE wording is normalised here, at the one funnel
// every remote error passes through. File and identifier names are kept as
// they are: `openidea-server`, `.openidea`, `openidea-api`,
// `openidea.repositories` and paths that contain them must stay usable.

import (
	"regexp"
	"strings"
)

// brandWordRE matches the product name in either case.
var brandWordRE = regexp.MustCompile(`(?i)\bopenidea\b`)

// anBrandRE fixes the article a renamed product needs: the sentence reads
// "an GoIDEA plugin" otherwise.
var anBrandRE = regexp.MustCompile(`\ban GoIDEA\b`)

// NormalizeBrand rewrites the upstream product name in backend-authored text to
// GoIDEA, leaving identifiers and paths alone.
func NormalizeBrand(s string) string {
	locs := brandWordRE.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, loc := range locs {
		var before, after byte
		if loc[0] > 0 {
			before = s[loc[0]-1]
		}
		if loc[1] < len(s) {
			after = s[loc[1]]
		}
		// Names that really exist keep their spelling: the backend binary and
		// its log, the project's .openidea directory, the plugin API, Maven's
		// tracking file, and any path segment that happens to be called
		// openidea.
		word := s[loc[0]:loc[1]]
		identifier := after == '-' || after == '_' || after == '/' ||
			before == '.' || before == '/' || before == '\\'
		if identifier || (word == "openidea" && after == '.') {
			continue
		}
		b.WriteString(s[last:loc[0]])
		b.WriteString("GoIDEA")
		last = loc[1]
	}
	b.WriteString(s[last:])
	return anBrandRE.ReplaceAllString(b.String(), "a GoIDEA")
}
